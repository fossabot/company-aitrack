package model

import "time"

// EditRecord is a persisted, validated AI-assisted edit event.
type EditRecord struct {
	ID            int64     `json:"id"`
	TokenKey      string    `json:"token_key"`
	DeviceID      string    `json:"device_id"`
	Hostname      string    `json:"hostname"`
	Tool          string    `json:"tool"`
	ToolVersion   string    `json:"tool_version"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	SessionID     string    `json:"session_id"`
	RepoURL       string    `json:"repo_url"`
	Branch        string    `json:"branch"`
	CurrentSHA    string    `json:"current_sha"`
	FilePath      string    `json:"file_path"`
	AddedLines    int64     `json:"added_lines"`
	RemovedLines  int64     `json:"removed_lines"`
	DiffHunk      string    `json:"diff_hunk"`
	Metadata      string    `json:"metadata"`
	Timestamp     string    `json:"timestamp"`
	RecordSig     string    `json:"record_sig"`
	PromptSummary *string   `json:"prompt_summary,omitempty"`
	Status        string    `json:"status"` // ACCEPTED, FLAGGED, REJECTED
	Flags         string    `json:"flags"`  // comma-separated
	ReceivedAt    time.Time `json:"received_at"`
}

// Device is a registered client device tracked via heartbeats.
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
