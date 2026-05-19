package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aitrack/server/internal/service"
)

// SearchHandler handles GET /api/v1/ai-track/edits/search (ParadeDB BM25).
type SearchHandler struct {
	db         *sql.DB
	adminKey   string
	isPostgres bool
}

// SimilarHandler handles POST /api/v1/ai-track/edits/similar (pgvector ANN).
type SimilarHandler struct {
	db         *sql.DB
	adminKey   string
	isPostgres bool
}

func NewSearchHandler(db *sql.DB, adminKey string, isPostgres bool) *SearchHandler {
	return &SearchHandler{db: db, adminKey: adminKey, isPostgres: isPostgres}
}

func NewSimilarHandler(db *sql.DB, adminKey string, isPostgres bool) *SimilarHandler {
	return &SimilarHandler{db: db, adminKey: adminKey, isPostgres: isPostgres}
}

// verifyAdminKey checks the X-Admin-Key header. Returns true if valid.
func verifySearchAdminKey(w http.ResponseWriter, r *http.Request, adminKey string) bool {
	if adminKey == "" {
		writeError(w, http.StatusServiceUnavailable,
			"admin-key is not configured; set AITRACK_ADMIN_KEY before using this endpoint")
		return false
	}
	provided := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if !service.ConstantTimeEqual(adminKey, provided) {
		writeError(w, http.StatusForbidden, "invalid X-Admin-Key")
		return false
	}
	return true
}

// searchHit is a single result from a BM25 full-text search.
type searchHit struct {
	RecordID       int64   `json:"record_id"`
	TokenKey       string  `json:"token_key"`
	Repo           string  `json:"repo"`
	FilePath       string  `json:"file_path"`
	DiffHunk       string  `json:"diff_hunk"`
	AILinesAdded   int64   `json:"ai_lines_added"`
	AILinesRemoved int64   `json:"ai_lines_removed"`
	Ts             string  `json:"ts"`
	Score          float64 `json:"score"`
}

// similarHit is a single result from a pgvector ANN search.
type similarHit struct {
	RecordID       int64   `json:"record_id"`
	TokenKey       string  `json:"token_key"`
	Repo           string  `json:"repo"`
	FilePath       string  `json:"file_path"`
	DiffHunk       string  `json:"diff_hunk"`
	AILinesAdded   int64   `json:"ai_lines_added"`
	AILinesRemoved int64   `json:"ai_lines_removed"`
	Ts             string  `json:"ts"`
	Distance       float64 `json:"distance"`
}

// similarRequest is the JSON body for POST /edits/similar.
type similarRequest struct {
	Embedding []float32 `json:"embedding"`
	Limit     int       `json:"limit"`
	TokenKey  string    `json:"token_key"`
	Repo      string    `json:"repo"`
}

// Search handles GET /api/v1/ai-track/edits/search.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	if !verifySearchAdminKey(w, r, h.adminKey) {
		return
	}

	q := r.URL.Query()
	query := strings.TrimSpace(q.Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}

	if !h.isPostgres {
		writeError(w, http.StatusNotImplemented, "requires PostgreSQL/ParadeDB mode")
		return
	}
	limit := clampInt(parseInt(q.Get("limit"), 20), 1, 100)
	tokenKey := q.Get("token_key")
	repo := q.Get("repo")

	// Build WHERE clause with optional filters.
	// ParadeDB BM25 search uses the ||| operator on the diff_hunk field.
	conditions := []string{"diff_hunk ||| $1"}
	args := []interface{}{query}
	argIdx := 2

	if tokenKey != "" {
		conditions = append(conditions, fmt.Sprintf("token_key = $%d", argIdx))
		args = append(args, tokenKey)
		argIdx++
	}
	if repo != "" {
		conditions = append(conditions, fmt.Sprintf("repo_url = $%d", argIdx))
		args = append(args, repo)
		argIdx++
	}

	args = append(args, limit)
	sqlStr := fmt.Sprintf(`
		SELECT id, token_key, repo_url, file_path,
		       COALESCE(diff_hunk, ''), added_lines, removed_lines, received_at,
		       paradedb.score(id) AS score
		FROM edit_records
		WHERE %s
		ORDER BY score DESC
		LIMIT $%d`,
		strings.Join(conditions, " AND "),
		argIdx,
	)

	rows, err := h.db.QueryContext(r.Context(), sqlStr, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search query failed")
		return
	}
	defer rows.Close()

	hits := make([]searchHit, 0)
	for rows.Next() {
		var hit searchHit
		var ts time.Time
		if err := rows.Scan(
			&hit.RecordID, &hit.TokenKey, &hit.Repo, &hit.FilePath,
			&hit.DiffHunk, &hit.AILinesAdded, &hit.AILinesRemoved, &ts, &hit.Score,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		hit.Ts = ts.UTC().Format(time.RFC3339)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "rows error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query": query,
		"total": len(hits),
		"hits":  hits,
	})
}

// Similar handles POST /api/v1/ai-track/edits/similar.
func (h *SimilarHandler) Similar(w http.ResponseWriter, r *http.Request) {
	if !verifySearchAdminKey(w, r, h.adminKey) {
		return
	}

	var req similarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Embedding) != 384 {
		writeError(w, http.StatusBadRequest, "embedding must be exactly 384 dimensions")
		return
	}

	if !h.isPostgres {
		writeError(w, http.StatusNotImplemented, "requires PostgreSQL/ParadeDB mode")
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Format embedding as "[f1,f2,...]" string for pgvector CAST.
	vecStr := formatVectorString(req.Embedding)

	// Build WHERE clause with optional filters.
	conditions := []string{"embedding IS NOT NULL"}
	args := []interface{}{vecStr}
	argIdx := 2

	if req.TokenKey != "" {
		conditions = append(conditions, fmt.Sprintf("token_key = $%d", argIdx))
		args = append(args, req.TokenKey)
		argIdx++
	}
	if req.Repo != "" {
		conditions = append(conditions, fmt.Sprintf("repo_url = $%d", argIdx))
		args = append(args, req.Repo)
		argIdx++
	}

	args = append(args, limit)
	sqlStr := fmt.Sprintf(`
		SELECT id, token_key, repo_url, file_path,
		       COALESCE(diff_hunk, ''), added_lines, removed_lines, received_at,
		       embedding <=> CAST($1 AS vector) AS distance
		FROM edit_records
		WHERE %s
		ORDER BY distance ASC
		LIMIT $%d`,
		strings.Join(conditions, " AND "),
		argIdx,
	)

	rows, err := h.db.QueryContext(r.Context(), sqlStr, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "similarity query failed")
		return
	}
	defer rows.Close()

	hits := make([]similarHit, 0)
	for rows.Next() {
		var hit similarHit
		var ts time.Time
		if err := rows.Scan(
			&hit.RecordID, &hit.TokenKey, &hit.Repo, &hit.FilePath,
			&hit.DiffHunk, &hit.AILinesAdded, &hit.AILinesRemoved, &ts, &hit.Distance,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		hit.Ts = ts.UTC().Format(time.RFC3339)
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "rows error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hits": hits,
	})
}

// formatVectorString formats a float32 slice as the pgvector literal "[f1,f2,...]".
func formatVectorString(v []float32) string {
	sb := strings.Builder{}
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%g", f))
	}
	sb.WriteByte(']')
	return sb.String()
}
