/// A full record row from the `records` table.
#[derive(Debug, Clone)]
pub struct Record {
    pub id: i64,
    pub tool: String,
    pub tool_version: Option<String>,
    pub provider: String,
    pub model: Option<String>,
    pub session_id: String,
    pub repo_url: String,
    pub branch: String,
    pub current_sha: String,
    pub file_path: String,
    pub added_lines: i64,
    pub removed_lines: i64,
    pub diff_hunk: Option<String>,
    pub metadata: Option<String>,
    #[allow(dead_code)]
    pub synced: i64,
    #[allow(dead_code)]
    pub synced_at: Option<String>,
    #[allow(dead_code)]
    pub retry_count: i64,
    pub timestamp: String,
    pub token_key: String,
    pub device_id: String,
    pub hostname: String,
    pub record_sig: String,
}

/// A lightweight row returned by the `inspect` command.
#[derive(Debug)]
pub struct InspectRow {
    pub id: i64,
    pub tool: String,
    pub model: Option<String>,
    pub file_path: String,
    pub added_lines: i64,
    pub removed_lines: i64,
    pub synced: i64,
    pub retry_count: i64,
    #[allow(dead_code)]
    pub token_key: String,
    pub timestamp: String,
}
