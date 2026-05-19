package model

import "time"

// StatsRow is an aggregated stats bucket for a group key.
type StatsRow struct {
	Group        string     `json:"group"`
	Edits        int64      `json:"edits"`
	AddedLines   int64      `json:"added_lines"`
	RemovedLines int64      `json:"removed_lines"`
	Accepted     int64      `json:"accepted"`
	Flagged      int64      `json:"flagged"`
	Rejected     int64      `json:"rejected"`
	LastActive   *time.Time `json:"last_active"`
}

// DeviceInfo is the API view of a registered device.
type DeviceInfo struct {
	DeviceID      string     `json:"device_id"`
	TokenKey      string     `json:"token_key"`
	Hostname      string     `json:"hostname"`
	ClientVersion string     `json:"client_version"`
	LastHeartbeat *time.Time `json:"last_heartbeat"`
	HooksJSON     string     `json:"hooks_json"`
	Silent        bool       `json:"silent"`
}

// EditQueryResult is a paginated list of edit records.
type EditQueryResult struct {
	Total   int64        `json:"total"`
	Page    int          `json:"page"`
	Size    int          `json:"size"`
	Records []EditRecord `json:"records"`
}

// ProfileDto is returned by GET /api/v1/ai-track/profiles/{token_key}.
type ProfileDto struct {
	TokenKey          string           `json:"token_key"`
	Owner             string           `json:"owner"`
	TotalEdits        int64            `json:"total_edits"`
	TotalAddedLines   int64            `json:"total_added_lines"`
	TotalRemovedLines int64            `json:"total_removed_lines"`
	FirstSeen         *string          `json:"first_seen"`   // ISO-8601 UTC, nil if no records
	LastSeen          *string          `json:"last_seen"`    // ISO-8601 UTC
	GeneratedAt       string           `json:"generated_at"` // ISO-8601 UTC
	Frequency         *FrequencyStats  `json:"frequency"`
	Depth             *DepthStats      `json:"depth"`
	Languages         map[string]int64 `json:"languages"`
	Tools             map[string]int64 `json:"tools"`
	PromptPatterns    map[string]int64 `json:"prompt_patterns"`
}

// FrequencyStats describes how often a token produces edits.
type FrequencyStats struct {
	DailyAvg30d  float64    `json:"daily_avg_30d"`
	WeeklyAvg12w float64    `json:"weekly_avg_12w"`
	DailyTrend   []DayCount `json:"daily_trend"`
}

// DayCount is the edit count for one calendar day.
type DayCount struct {
	Date  string `json:"date"` // "2026-05-19"
	Count int64  `json:"count"`
}

// DepthStats describes the size profile of a token's edits.
type DepthStats struct {
	AvgLines       float64 `json:"avg_lines"`
	P50Lines       int64   `json:"p50_lines"`
	P90Lines       int64   `json:"p90_lines"`
	SmallCount     int64   `json:"small_count"`  // total < 10
	MediumCount    int64   `json:"medium_count"` // 10 <= total <= 100
	LargeCount     int64   `json:"large_count"`  // total > 100
	CommentDensity float64 `json:"comment_density"`
}
