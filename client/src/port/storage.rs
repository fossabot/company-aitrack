use crate::domain::model::{InspectRow, Record};

/// Port: persistent storage for edit records.
pub trait StoragePort {
    /// Persist a new edit record. Returns `true` if inserted, `false` if deduped.
    fn save_record(&self, record: &Record) -> rusqlite::Result<bool>;

    /// Count unsynced records for a given token key.
    fn pending_count(&self, token_key: &str) -> i64;

    /// Fetch unsynced records for a given token key up to `limit`.
    fn fetch_unsynced(&self, token_key: &str, limit: i64) -> rusqlite::Result<Vec<Record>>;

    /// Mark a set of record IDs as synced.
    fn mark_synced(&self, ids: &[i64]) -> anyhow::Result<()>;

    /// Increment the retry counter for a set of record IDs.
    fn increment_retry(&self, ids: &[i64]) -> anyhow::Result<()>;

    /// Retrieve recent records for the inspect command.
    fn inspect_records(
        &self,
        limit: i64,
        pending_only: bool,
        token_key: &str,
    ) -> anyhow::Result<Vec<InspectRow>>;
}
