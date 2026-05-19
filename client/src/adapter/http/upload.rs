/// HTTP upload adapter — delegates to the uploader module.
///
/// The `flush_unsynced` function is the primary entry point used by the capture
/// pipeline.  The `HttpUploader` struct implements `UploadPort` for callers that
/// prefer trait-based dispatch.
pub use crate::uploader::flush_unsynced;

use crate::domain::model::Record;
use crate::port::upload::UploadPort;

/// HTTP-backed implementation of `UploadPort`.
///
/// Currently this is a marker type; the async upload path lives in
/// `uploader::flush_unsynced` which takes a `Connection` directly.
/// Future refactoring may move the full async logic here.
pub struct HttpUploader {
    pub api_url: String,
    pub credential: String,
}

impl HttpUploader {
    pub fn new(api_url: String, credential: String) -> Self {
        Self { api_url, credential }
    }
}

impl UploadPort for HttpUploader {
    fn upload_batch(&self, _records: &[Record]) -> anyhow::Result<()> {
        // Async upload is handled by flush_unsynced; this sync variant is a
        // placeholder to satisfy the trait boundary for compile-time checks.
        Ok(())
    }
}
