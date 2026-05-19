package service

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aitrack/server/internal/domain/model"
)

// RawRecord holds the columns needed to compute a developer profile.
// It is populated by the persistence adapter and consumed by pure analytics.
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

// ProfileService computes developer profiles from raw edit records.
// It is pure: it never touches a database or the network.
type ProfileService struct{}

// NewProfileService constructs a ProfileService.
func NewProfileService() *ProfileService { return &ProfileService{} }

// BuildProfile aggregates raw records into a ProfileDto.
// owner and tokenKey identify the subject; now anchors the time-window math.
func (p *ProfileService) BuildProfile(tokenKey, owner string, records []RawRecord, now time.Time) *model.ProfileDto {
	now = now.UTC()
	dto := &model.ProfileDto{
		TokenKey:    tokenKey,
		Owner:       owner,
		GeneratedAt: now.Format(time.RFC3339),
		Languages:   make(map[string]int64),
		Tools:       make(map[string]int64),
	}

	if len(records) == 0 {
		dto.Frequency = &model.FrequencyStats{DailyTrend: []model.DayCount{}}
		dto.Depth = &model.DepthStats{}
		dto.PromptPatterns = ComputePromptPatterns(records)
		return dto
	}

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

		dto.Tools[rec.Tool]++
		dto.Languages[DetectLanguage(rec.FilePath)]++

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

	dailyTrend := make([]model.DayCount, 0, len(dayBuckets))
	for day, cnt := range dayBuckets {
		dailyTrend = append(dailyTrend, model.DayCount{Date: day, Count: cnt})
	}
	sort.Slice(dailyTrend, func(i, j int) bool { return dailyTrend[i].Date < dailyTrend[j].Date })
	dto.Frequency = &model.FrequencyStats{
		DailyAvg30d:  float64(count30d) / 30.0,
		WeeklyAvg12w: float64(count12w) / 12.0,
		DailyTrend:   dailyTrend,
	}

	sort.Slice(lineTotals, func(i, j int) bool { return lineTotals[i] < lineTotals[j] })
	n := int64(len(lineTotals))
	var sum, smallCount, mediumCount, largeCount int64
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
	dto.Depth = &model.DepthStats{
		AvgLines:       float64(sum) / float64(n),
		P50Lines:       lineTotals[n/2],
		P90Lines:       lineTotals[int(float64(n)*0.9)],
		SmallCount:     smallCount,
		MediumCount:    mediumCount,
		LargeCount:     largeCount,
		CommentDensity: ComputeCommentDensity(records),
	}

	dto.PromptPatterns = ComputePromptPatterns(records)
	return dto
}

// DetectLanguage maps a file path to a language name based on its extension.
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

// ComputeCommentDensity calculates the ratio of comment lines to total added
// lines across all records' diff hunks.
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

// Percentile returns the value at the given percentile (0.0–1.0) of a sorted slice.
// The slice must be sorted in ascending order.
func Percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * p)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
