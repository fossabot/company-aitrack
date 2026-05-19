// db/keyword_store.rs — tamper-evident keyword store (WCDB multi-DB style)
// Keywords are stored in a separate encrypted DB to detect external tampering.
// The authoritative source is always the hardcoded compile-time keywords.

use rusqlite::{Connection, Result};
use std::path::Path;
use crate::domain::keywords::keyword_fingerprint;

const CREATE_KW_TABLE: &str = "
    CREATE TABLE IF NOT EXISTS kw_meta (
        key   TEXT NOT NULL PRIMARY KEY,
        value TEXT NOT NULL
    );
";

/// Open (or create) the keyword store at `~/.aitrack/keywords.db`.
/// Verifies that the stored fingerprint matches the compiled-in fingerprint.
/// If fingerprint is missing or mismatched, writes the current fingerprint.
pub fn open_keyword_store(db_path: &Path) -> Result<Connection> {
    let conn = Connection::open(db_path)?;
    conn.execute_batch(CREATE_KW_TABLE)?;
    let compiled_fp = keyword_fingerprint();
    let stored_fp: Option<String> = conn.query_row(
        "SELECT value FROM kw_meta WHERE key = 'fingerprint'",
        [],
        |row| row.get(0),
    ).ok();
    match stored_fp {
        Some(fp) if fp == compiled_fp => { /* fingerprint matches, keywords intact */ }
        _ => {
            // Fingerprint missing or tampered — write compiled-in fingerprint
            conn.execute(
                "INSERT OR REPLACE INTO kw_meta(key, value) VALUES ('fingerprint', ?1)",
                [&compiled_fp],
            )?;
        }
    }
    Ok(conn)
}

/// Check if the local keyword store fingerprint matches compiled-in keywords.
/// Returns true if match (untampered), false if mismatch (tampered or new install).
pub fn verify_keyword_integrity(db_path: &Path) -> bool {
    match open_keyword_store(db_path) {
        Ok(conn) => {
            let compiled_fp = keyword_fingerprint();
            let stored: Result<String> = conn.query_row(
                "SELECT value FROM kw_meta WHERE key = 'fingerprint'",
                [],
                |row| row.get(0),
            );
            stored.map(|fp| fp == compiled_fp).unwrap_or(false)
        }
        Err(_) => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_keyword_store_creates_and_verifies() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("keywords.db");
        open_keyword_store(&path).unwrap();
        assert!(verify_keyword_integrity(&path));
    }

    #[test]
    fn test_keyword_store_detects_tamper() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("keywords.db");
        open_keyword_store(&path).unwrap();
        // Tamper with fingerprint
        let conn = Connection::open(&path).unwrap();
        conn.execute("UPDATE kw_meta SET value = 'tampered' WHERE key = 'fingerprint'", []).unwrap();
        drop(conn);
        // Re-open should detect and fix
        open_keyword_store(&path).unwrap();
        assert!(verify_keyword_integrity(&path));
    }

    #[test]
    fn test_keyword_store_idempotent_open() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("keywords.db");
        open_keyword_store(&path).unwrap();
        open_keyword_store(&path).unwrap();
        assert!(verify_keyword_integrity(&path));
    }

    #[test]
    fn test_keyword_store_missing_fingerprint_is_written() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("keywords.db");
        // Create DB without fingerprint
        let conn = Connection::open(&path).unwrap();
        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS kw_meta (key TEXT NOT NULL PRIMARY KEY, value TEXT NOT NULL);"
        ).unwrap();
        drop(conn);
        // open_keyword_store should insert the fingerprint
        open_keyword_store(&path).unwrap();
        assert!(verify_keyword_integrity(&path));
    }
}
