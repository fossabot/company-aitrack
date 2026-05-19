package handler

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aitrack/server/internal/model"
	"github.com/aitrack/server/internal/service"
	"github.com/go-chi/chi/v5"
)

// ProfileHandler handles GET /api/v1/ai-track/profiles/{token_key}.
type ProfileHandler struct {
	db       *sql.DB
	adminKey string
}

func NewProfileHandler(db *sql.DB, adminKey string) *ProfileHandler {
	return &ProfileHandler{db: db, adminKey: adminKey}
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

// RawRecord holds the columns we need from edit_records.
// Exported so tests can construct values for ComputeCommentDensity and ComputePromptPatterns.
type RawRecord struct {
	Tool          string
	FilePath      string
	AddedLines    int64
	RemovedLines  int64
	DiffHunk      string
	Timestamp     string // Unix epoch as text
	Status        string
	PromptSummary *string
}

func (h *ProfileHandler) computeProfile(r *http.Request, tokenKey string) (*model.ProfileDto, error) {
	ctx := r.Context()

	// Look up owner from tokens table.
	var owner string
	err := h.db.QueryRowContext(ctx,
		`SELECT owner FROM tokens WHERE token_key = ? AND active = 1`, tokenKey,
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
		 WHERE token_key = ? AND status != 'REJECTED'`,
		tokenKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RawRecord
	for rows.Next() {
		var rec RawRecord
		if err := rows.Scan(&rec.Tool, &rec.FilePath, &rec.AddedLines, &rec.RemovedLines, &rec.DiffHunk, &rec.Timestamp, &rec.Status, &rec.PromptSummary); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	generatedAt := now.Format(time.RFC3339)

	// Build profile from records.
	dto := &model.ProfileDto{
		TokenKey:    tokenKey,
		Owner:       owner,
		GeneratedAt: generatedAt,
		Languages:   make(map[string]int64),
		Tools:       make(map[string]int64),
	}

	if len(records) == 0 {
		// Token exists but no records — return 200 with zeros.
		freq := &model.FrequencyStats{
			DailyAvg30d:  0,
			WeeklyAvg12w: 0,
			DailyTrend:   []model.DayCount{},
		}
		depth := &model.DepthStats{}
		dto.Frequency = freq
		dto.Depth = depth
		dto.PromptPatterns = ComputePromptPatterns(records)
		return dto, nil
	}

	// Aggregate totals, tools, scenarios, and timestamps.
	var (
		totalAdded   int64
		totalRemoved int64
		lineTotals   []int64
		minEpoch     int64 = -1
		maxEpoch     int64 = -1

		count30d int64
		count12w int64

		cutoff30d = now.Unix() - 30*86400
		cutoff12w = now.Unix() - 84*86400

		dayBuckets = make(map[string]int64)
	)

	for _, rec := range records {
		totalAdded += rec.AddedLines
		totalRemoved += rec.RemovedLines

		lineTotal := rec.AddedLines + rec.RemovedLines
		lineTotals = append(lineTotals, lineTotal)

		// Tool counts.
		dto.Tools[rec.Tool]++

		// Language detection.
		lang := DetectLanguage(rec.FilePath)
		dto.Languages[lang]++

		// Timestamp-based calculations.
		epochTs, parseErr := strconv.ParseInt(rec.Timestamp, 10, 64)
		if parseErr == nil {
			if minEpoch < 0 || epochTs < minEpoch {
				minEpoch = epochTs
			}
			if epochTs > maxEpoch {
				maxEpoch = epochTs
			}

			if epochTs >= cutoff30d {
				count30d++
				day := time.Unix(epochTs, 0).UTC().Format("2006-01-02")
				dayBuckets[day]++
			}
			if epochTs >= cutoff12w {
				count12w++
			}
		}
	}

	dto.TotalEdits = int64(len(records))
	dto.TotalAddedLines = totalAdded
	dto.TotalRemovedLines = totalRemoved

	if minEpoch >= 0 {
		s := time.Unix(minEpoch, 0).UTC().Format(time.RFC3339)
		dto.FirstSeen = &s
	}
	if maxEpoch >= 0 {
		s := time.Unix(maxEpoch, 0).UTC().Format(time.RFC3339)
		dto.LastSeen = &s
	}

	// Frequency stats.
	dailyTrend := make([]model.DayCount, 0, len(dayBuckets))
	for day, cnt := range dayBuckets {
		dailyTrend = append(dailyTrend, model.DayCount{Date: day, Count: cnt})
	}
	sort.Slice(dailyTrend, func(i, j int) bool {
		return dailyTrend[i].Date < dailyTrend[j].Date
	})
	dto.Frequency = &model.FrequencyStats{
		DailyAvg30d:  float64(count30d) / 30.0,
		WeeklyAvg12w: float64(count12w) / 12.0,
		DailyTrend:   dailyTrend,
	}

	// Depth stats.
	sort.Slice(lineTotals, func(i, j int) bool { return lineTotals[i] < lineTotals[j] })
	n := int64(len(lineTotals))
	var sum int64
	var smallCount, mediumCount, largeCount int64
	for _, v := range lineTotals {
		sum += v
		switch {
		case v < 10:
			smallCount++
		case v <= 100:
			mediumCount++
		default:
			largeCount++
		}
	}
	avgLines := float64(sum) / float64(n)
	p50 := lineTotals[n/2]
	p90 := lineTotals[int(float64(n)*0.9)]

	dto.Depth = &model.DepthStats{
		AvgLines:       avgLines,
		P50Lines:       p50,
		P90Lines:       p90,
		SmallCount:     smallCount,
		MediumCount:    mediumCount,
		LargeCount:     largeCount,
		CommentDensity: ComputeCommentDensity(records),
	}

	dto.PromptPatterns = ComputePromptPatterns(records)

	return dto, nil
}

// ComputePromptPatterns classifies prompt_summary text into intent categories.
func ComputePromptPatterns(records []RawRecord) map[string]int64 {
	patterns := map[string]int64{
		"generate":  0,
		"fix_debug": 0,
		"refactor":  0,
		"explain":   0,
		"test":      0,
		"other":     0,
	}
	for _, r := range records {
		if r.PromptSummary == nil || *r.PromptSummary == "" {
			continue
		}
		lower := strings.ToLower(*r.PromptSummary)
		switch {
		case matchesAny(lower, "generate", "create", "write", "implement", "add"):
			patterns["generate"]++
		case matchesAny(lower, "fix", "debug", "error", "bug", "broken", "failing"):
			patterns["fix_debug"]++
		case matchesAny(lower, "refactor", "clean", "improve", "reorganize", "rename"):
			patterns["refactor"]++
		case matchesAny(lower, "explain", "what", "how", "why", "understand", "describe"):
			patterns["explain"]++
		case matchesAny(lower, "test", "spec", "mock", "assert", "verify"):
			patterns["test"]++
		default:
			patterns["other"]++
		}
	}
	return patterns
}

func matchesAny(s string, keywords ...string) bool {
	for _, kw := range keywords {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}

// DetectLanguage maps a file path to a language name based on its extension.
// Exported for testing.
func DetectLanguage(filePath string) string {
	if filePath == "" {
		return "Other"
	}
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".py":
		return "Python"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".js", ".jsx":
		return "JavaScript"
	case ".java":
		return "Java"
	case ".go":
		return "Go"
	case ".rs":
		return "Rust"
	case ".cpp", ".cc", ".cxx", ".c":
		return "C/C++"
	case ".cs":
		return "C#"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	case ".swift":
		return "Swift"
	case ".kt", ".kts":
		return "Kotlin"
	case ".scala":
		return "Scala"
	case ".vue":
		return "Vue"
	case ".html", ".htm":
		return "HTML"
	case ".css", ".scss", ".sass", ".less":
		return "CSS"
	case ".sql":
		return "SQL"
	case ".sh", ".bash", ".zsh":
		return "Shell"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".xml":
		return "XML"
	case ".md", ".rst", ".txt":
		return "Docs"
	default:
		return "Other"
	}
}

// ComputeCommentDensity calculates the ratio of comment lines to total added lines
// across all records' diff hunks.
func ComputeCommentDensity(records []RawRecord) float64 {
	var totalAdded, commentLines int64
	for _, r := range records {
		if r.DiffHunk == "" {
			continue
		}
		for _, line := range strings.Split(r.DiffHunk, "\n") {
			if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
				continue
			}
			totalAdded++
			trimmed := strings.TrimSpace(line[1:]) // strip leading +
			if strings.HasPrefix(trimmed, "//") ||
				strings.HasPrefix(trimmed, "#") ||
				strings.HasPrefix(trimmed, "/*") ||
				strings.HasPrefix(trimmed, "* ") ||
				strings.HasPrefix(trimmed, "*/") ||
				strings.HasPrefix(trimmed, "/**") ||
				strings.HasPrefix(trimmed, `"""`) ||
				strings.HasPrefix(trimmed, `'''`) ||
				strings.HasPrefix(trimmed, "--") ||
				strings.HasPrefix(trimmed, "<!--") {
				commentLines++
			}
		}
	}
	if totalAdded == 0 {
		return 0.0
	}
	return float64(commentLines) / float64(totalAdded)
}

// percentile returns the value at the given percentile (0.0–1.0) of a sorted slice.
// The slice must be sorted in ascending order.
func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
