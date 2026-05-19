package model

import "time"

type Token struct {
	ID         int64
	TokenHash  string
	TokenKey   string
	HmacSecret string // encrypted at rest, decrypted in memory for callers
	Owner      string
	Note       string
	Active     bool
	CreatedAt  time.Time
}

type EditRecord struct {
	ID           int64     `json:"id"`
	TokenKey     string    `json:"token_key"`
	DeviceID     string    `json:"device_id"`
	Hostname     string    `json:"hostname"`
	Tool         string    `json:"tool"`
	ToolVersion  string    `json:"tool_version"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	SessionID    string    `json:"session_id"`
	RepoURL      string    `json:"repo_url"`
	Branch       string    `json:"branch"`
	CurrentSHA   string    `json:"current_sha"`
	FilePath     string    `json:"file_path"`
	AddedLines   int64     `json:"added_lines"`
	RemovedLines int64     `json:"removed_lines"`
	DiffHunk     string    `json:"diff_hunk"`
	Metadata     string    `json:"metadata"`
	Timestamp    string    `json:"timestamp"`
	RecordSig    string    `json:"record_sig"`
	Status       string    `json:"status"` // ACCEPTED, FLAGGED, REJECTED
	Flags        string    `json:"flags"`  // comma-separated
	ReceivedAt   time.Time `json:"received_at"`
}

type Device struct {
	ID            int64
	DeviceID      string
	TokenKey      string
	Hostname      string
	ClientVersion string
	LastHeartbeat *time.Time
	HooksJSON     string
	CreatedAt     time.Time
}

// DTOs

type CreateTokenRequest struct {
	Owner string `json:"owner"`
	Note  string `json:"note"`
}

type CreateTokenResponse struct {
	Credential string `json:"credential"`
	TokenKey   string `json:"token_key"`
}

type EditDTO struct {
	Tool         string  `json:"tool"`
	ToolVersion  string  `json:"tool_version"`
	Provider     string  `json:"provider"`
	Model        *string `json:"model"`
	SessionID    string  `json:"session_id"`
	RepoURL      string  `json:"repo_url"`
	Branch       string  `json:"branch"`
	CurrentSHA   string  `json:"current_sha"`
	FilePath     string  `json:"file_path"`
	AddedLines   *int64  `json:"added_lines"`
	RemovedLines *int64  `json:"removed_lines"`
	DiffHunk     *string `json:"diff_hunk"`
	Metadata     *string `json:"metadata"`
	Timestamp    string  `json:"timestamp"`
	DeviceID     string  `json:"device_id"`
	Hostname     string  `json:"hostname"`
	RecordSig    string  `json:"record_sig"`
}

type EditBatchRequest struct {
	DeviceID      string    `json:"device_id"`
	ClientVersion string    `json:"client_version"`
	Edits         []EditDTO `json:"edits"`
}

type IndexedReason struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}

type EditBatchResponse struct {
	Accepted int             `json:"accepted"`
	Rejected []IndexedReason `json:"rejected"`
	Flagged  []IndexedReason `json:"flagged"`
}

type HeartbeatHooks struct {
	Claude bool `json:"claude"`
	Codex  bool `json:"codex"`
	Cursor bool `json:"cursor"`
}

type HeartbeatRequest struct {
	DeviceID       string          `json:"device_id"`
	Hostname       string          `json:"hostname"`
	TokenKeyMasked string          `json:"token_key_masked"`
	ClientVersion  string          `json:"client_version"`
	TS             int64           `json:"ts"`
	Hooks          *HeartbeatHooks `json:"hooks"`
	PendingCount   int             `json:"pending_count"`
}

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

type DeviceInfo struct {
	DeviceID      string     `json:"device_id"`
	TokenKey      string     `json:"token_key"`
	Hostname      string     `json:"hostname"`
	ClientVersion string     `json:"client_version"`
	LastHeartbeat *time.Time `json:"last_heartbeat"`
	HooksJSON     string     `json:"hooks_json"`
	Silent        bool       `json:"silent"`
}

type EditQueryResult struct {
	Total   int64        `json:"total"`
	Page    int          `json:"page"`
	Size    int          `json:"size"`
	Records []EditRecord `json:"records"`
}

// ProfileDto is returned by GET /api/v1/ai-track/profiles/{token_key}
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
	Scenarios         map[string]int64 `json:"scenarios"`
	Tools             map[string]int64 `json:"tools"`
}

type FrequencyStats struct {
	DailyAvg30d  float64    `json:"daily_avg_30d"`
	WeeklyAvg12w float64    `json:"weekly_avg_12w"`
	DailyTrend   []DayCount `json:"daily_trend"`
}

type DayCount struct {
	Date  string `json:"date"`  // "2026-05-19"
	Count int64  `json:"count"`
}

type DepthStats struct {
	AvgLines    float64 `json:"avg_lines"`
	P50Lines    int64   `json:"p50_lines"`
	P90Lines    int64   `json:"p90_lines"`
	SmallCount  int64   `json:"small_count"`  // total < 10
	MediumCount int64   `json:"medium_count"` // 10 <= total <= 100
	LargeCount  int64   `json:"large_count"`  // total > 100
}
