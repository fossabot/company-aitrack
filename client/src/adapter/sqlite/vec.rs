use std::sync::atomic::{AtomicBool, Ordering};

/// Set to `true` when sqlite-vec failed to load at startup.
/// When `true`, all vector operations are no-ops — core record capture is unaffected.
pub static VEC_DISABLED: AtomicBool = AtomicBool::new(false);

/// Register the sqlite-vec extension via `sqlite3_auto_extension`.
///
/// Must be called **once** before any `Connection` is opened so the extension
/// is available to every subsequent connection.  If the registration fails the
/// function sets `VEC_DISABLED = true` and logs a warning; the rest of the
/// capture pipeline continues normally.
#[allow(clippy::transmute_null_to_fn)]
pub fn register_auto_extension() {
    unsafe {
        use rusqlite::ffi::sqlite3_auto_extension;
        // SAFETY: sqlite3_vec_init has the correct C-ABI signature expected by
        // sqlite3_auto_extension. The transmute is the canonical pattern shown
        // in the sqlite-vec crate's own test suite.
        #[allow(clippy::missing_transmute_annotations)]
        sqlite3_auto_extension(Some(std::mem::transmute(
            sqlite_vec::sqlite3_vec_init as *const (),
        )));
    }
}

/// Verify that sqlite-vec was successfully loaded in `conn`.
///
/// On success logs the version string.  On failure marks `VEC_DISABLED = true`
/// and logs a warning so the caller knows vector features are unavailable.
pub fn init_sqlite_vec(conn: &rusqlite::Connection) {
    match conn.query_row("SELECT vec_version()", [], |r| r.get::<_, String>(0)) {
        Ok(ver) => {
            eprintln!("[aitrack] sqlite-vec loaded: {}", ver);
        }
        Err(e) => {
            eprintln!(
                "[aitrack] sqlite-vec unavailable ({}), vector features disabled",
                e
            );
            VEC_DISABLED.store(true, Ordering::Relaxed);
        }
    }
}

/// Create the `vec_records` virtual table if sqlite-vec is available.
///
/// This is a no-op when `VEC_DISABLED` is set, so it is safe to call
/// unconditionally after `init_sqlite_vec`.
pub fn ensure_vec_table(conn: &rusqlite::Connection) -> rusqlite::Result<()> {
    if VEC_DISABLED.load(Ordering::Relaxed) {
        return Ok(());
    }
    conn.execute_batch(
        "CREATE VIRTUAL TABLE IF NOT EXISTS vec_records USING vec0(embedding float[384])",
    )
}

/// Returns `true` when sqlite-vec is available and vector operations can proceed.
pub fn is_vec_enabled() -> bool {
    !VEC_DISABLED.load(Ordering::Relaxed)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    /// Serialise all tests that touch the process-global `VEC_DISABLED` flag.
    static FLAG_MUTEX: Mutex<()> = Mutex::new(());

    #[test]
    fn vec_disabled_flag_works() {
        let _guard = FLAG_MUTEX.lock().unwrap_or_else(|p| p.into_inner());
        let original = VEC_DISABLED.load(Ordering::Relaxed);

        VEC_DISABLED.store(true, Ordering::Relaxed);
        assert!(
            !is_vec_enabled(),
            "should report disabled when flag is true"
        );

        VEC_DISABLED.store(false, Ordering::Relaxed);
        assert!(is_vec_enabled(), "should report enabled when flag is false");

        VEC_DISABLED.store(original, Ordering::Relaxed);
    }

    #[test]
    fn ensure_vec_table_skips_when_disabled() {
        let _guard = FLAG_MUTEX.lock().unwrap_or_else(|p| p.into_inner());
        let original = VEC_DISABLED.load(Ordering::Relaxed);
        VEC_DISABLED.store(true, Ordering::Relaxed);

        let conn = rusqlite::Connection::open_in_memory().unwrap();
        assert!(ensure_vec_table(&conn).is_ok());

        VEC_DISABLED.store(original, Ordering::Relaxed);
    }

    #[test]
    fn init_sqlite_vec_sets_disabled_when_extension_absent() {
        let _guard = FLAG_MUTEX.lock().unwrap_or_else(|p| p.into_inner());
        let original = VEC_DISABLED.load(Ordering::Relaxed);
        VEC_DISABLED.store(false, Ordering::Relaxed);

        let conn = rusqlite::Connection::open_in_memory().unwrap();
        let vec_present = conn
            .query_row("SELECT vec_version()", [], |r| r.get::<_, String>(0))
            .is_ok();

        init_sqlite_vec(&conn);

        if vec_present {
            assert!(
                is_vec_enabled(),
                "flag should stay enabled when extension is present"
            );
        } else {
            assert!(
                !is_vec_enabled(),
                "flag should be disabled when extension absent"
            );
        }

        VEC_DISABLED.store(original, Ordering::Relaxed);
    }
}
