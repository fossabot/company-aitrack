package handler

import (
	"database/sql"
	"net/http"
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

// rawRecord holds the columns we need from edit_records.
type rawRecord struct {
	Tool         string
	FilePath     string
	AddedLines   int64
	RemovedLines int64
	Timestamp    string // Unix epoch as text
	Status       string
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
		`SELECT tool, file_path, added_lines, removed_lines, timestamp, status
		 FROM edit_records
		 WHERE token_key = ? AND status != 'REJECTED'`,
		tokenKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []rawRecord
	for rows.Next() {
		var rec rawRecord
		if err := rows.Scan(&rec.Tool, &rec.FilePath, &rec.AddedLines, &rec.RemovedLines, &rec.Timestamp, &rec.Status); err != nil {
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
		Scenarios:   make(map[string]int64),
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

		// Scenario classification.
		scenario := ClassifyScenario(rec.FilePath)
		dto.Scenarios[scenario]++

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
		AvgLines:    avgLines,
		P50Lines:    p50,
		P90Lines:    p90,
		SmallCount:  smallCount,
		MediumCount: mediumCount,
		LargeCount:  largeCount,
	}

	return dto, nil
}

// ClassifyScenario classifies a file path into a scenario category.
// Exported for testing.
func ClassifyScenario(filePath string) string {
	if strings.TrimSpace(filePath) == "" {
		return "other"
	}
	lp := strings.ToLower(filePath)

	// Test files.
	if strings.Contains(lp, "/test") ||
		strings.HasPrefix(lp, "test") ||
		strings.Contains(lp, "_test.") ||
		strings.Contains(lp, ".test.") ||
		strings.Contains(lp, "/spec") ||
		strings.HasPrefix(lp, "spec") ||
		strings.Contains(lp, "_spec.") ||
		strings.Contains(lp, ".spec.") {
		return "test"
	}

	// Docs files.
	if strings.HasSuffix(lp, ".md") ||
		strings.HasSuffix(lp, ".rst") ||
		strings.HasSuffix(lp, ".txt") ||
		strings.Contains(lp, "/docs/") ||
		strings.Contains(lp, "/doc/") {
		return "docs"
	}

	// Config files.
	if strings.HasSuffix(lp, ".yaml") ||
		strings.HasSuffix(lp, ".yml") ||
		strings.HasSuffix(lp, ".toml") ||
		strings.HasSuffix(lp, ".json") ||
		strings.HasSuffix(lp, ".xml") ||
		strings.HasSuffix(lp, ".ini") ||
		strings.HasSuffix(lp, ".env") ||
		strings.HasSuffix(lp, ".properties") ||
		strings.Contains(lp, "/config/") {
		return "config"
	}

	return "feature"
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
