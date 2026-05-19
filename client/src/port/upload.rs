use crate::domain::model::Record;

/// Port: upload edit records to the remote server.
pub trait UploadPort {
    /// Upload a batch of records to the server endpoint.
    fn upload_batch(&self, records: &[Record]) -> anyhow::Result<()>;
}
