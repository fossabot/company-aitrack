package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/aitrack/server/internal/domain/model"
	"github.com/aitrack/server/internal/domain/service"
	"github.com/go-chi/chi/v5"
)

// ProfileHandler handles GET /api/v1/ai-track/profiles/{token_key}.
type ProfileHandler struct {
	db       *sql.DB
	adminKey string
	profiles *service.ProfileService
}

// NewProfileHandler constructs the profile handler adapter.
func NewProfileHandler(db *sql.DB, adminKey string) *ProfileHandler {
	return &ProfileHandler{db: db, adminKey: adminKey, profiles: service.NewProfileService()}
}

// Profile handles GET /api/v1/ai-track/profiles/{token_key}.
func (h *ProfileHandler) Profile(w http.ResponseWriter, r *http.Request) {
	if h.adminKey == "" {
		writeError(w, http.StatusServiceUnavailable,
			"admin-key is not configured; set AITRACK_ADMIN_KEY before using this endpoint")
		return
	}
	provided := strings.TrimSpace(r.Header.Get("X-Admin-Key"))
	if !service.ConstantTimeEqual(h.adminKey, provided) {
		writeError(w, http.StatusForbidden, "invalid X-Admin-Key")
		return
	}

	tokenKey := chi.URLParam(r, "token_key")
	if tokenKey == "" {
		writeError(w, http.StatusBadRequest, "token_key is required")
		return
	}

	profile, err := h.computeProfile(r, tokenKey)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "profile computation failed")
		return
	}
	if profile == nil {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// computeProfile loads raw records from the database then delegates the
// analytics to the pure domain ProfileService.
func (h *ProfileHandler) computeProfile(r *http.Request, tokenKey string) (*model.ProfileDto, error) {
	ctx := r.Context()

	// Look up owner from tokens table.
	var owner string
	err := h.db.QueryRowContext(ctx,
		`SELECT owner FROM tokens WHERE token_key = $1 AND active = 1`, tokenKey,
	).Scan(&owner)
	if err == sql.ErrNoRows {
		return nil, nil // token not found → 404
	}
	if err != nil {
		return nil, err
	}

	// Query all non-REJECTED records for this token.
	rows, err := h.db.QueryContext(ctx,
		`SELECT tool, file_path, added_lines, removed_lines, diff_hunk, timestamp, status, prompt_summary
		 FROM edit_records
		 WHERE token_key = $1 AND status != 'REJECTED'`,
		tokenKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []service.RawRecord
	for rows.Next() {
		var rec service.RawRecord
		if err := rows.Scan(&rec.Tool, &rec.FilePath, &rec.AddedLines, &rec.RemovedLines, &rec.DiffHunk, &rec.Timestamp, &rec.Status, &rec.PromptSummary); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return h.profiles.BuildProfile(tokenKey, owner, records, time.Now()), nil
}
