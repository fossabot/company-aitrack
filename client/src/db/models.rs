/// Re-export the canonical model types from the domain layer.
/// All code should prefer `crate::domain::model::*` for new imports;
/// this shim keeps legacy `crate::db::models::*` paths compiling.
pub use crate::domain::model::{InspectRow, Record};
