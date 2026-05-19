// Package db is the infrastructure adapter that opens the database connection
// and runs schema migrations. It supports SQLite (default) and PostgreSQL.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// WithDatabaseURL is a sentinel option used by app.Build to request PostgreSQL mode.
// When non-empty, Open will use the pgx driver instead of SQLite.
type WithDatabaseURL string

// Open opens (or creates) the database and runs schema migrations.
//
// Supported optional arguments (processed in order):
//   - WithDatabaseURL(url): if non-empty, connect to PostgreSQL via pgx driver
//     instead of the SQLite file at path.
//   - func(*sql.DB) error: a custom migrate function (used in tests).
//
// Production callers typically omit all opts (SQLite default).
func Open(path string, opts ...interface{}) (*sql.DB, error) {
	var databaseURL string
	var customMigrateFn func(*sql.DB) error

	for _, opt := range opts {
		switch v := opt.(type) {
		case WithDatabaseURL:
			databaseURL = string(v)
		case func(*sql.DB) error:
			customMigrateFn = v
		}
	}

	var database *sql.DB
	var err error
	isPostgres := databaseURL != ""

	if isPostgres {
		// PostgreSQL / ParadeDB mode
		database, err = sql.Open("pgx", databaseURL)
		if err != nil {
			return nil, fmt.Errorf("open postgres: %w", err)
		}
	} else {
		// SQLite mode (default)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
		database, err = sql.Open("sqlite", path+"?_journal=WAL&_busy_timeout=5000")
		if err != nil {
			return nil, err
		}
		database.SetMaxOpenConns(1) // SQLite write serialization
	}

	fn := func(db *sql.DB) error { return migrate(db, isPostgres) }
	if customMigrateFn != nil {
		fn = customMigrateFn
	}
	if err := fn(database); err != nil {
		database.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return database, nil
}

// migrate runs schema DDL. isPostgres selects the appropriate dialect.
func migrate(db *sql.DB, isPostgres bool) error {
	var stmts []string
	if isPostgres {
		stmts = postgresDDL()
	} else {
		stmts = sqliteDDL()
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			// For SQLite: ALTER TABLE ADD COLUMN does not support IF NOT EXISTS.
			// When the column already exists (e.g. fresh DB with columns in CREATE TABLE),
			// SQLite returns "duplicate column name: <col>". Treat that as a no-op.
			if !isPostgres && isAlterAddColumn(stmt) && isDuplicateColumnError(err) {
				continue
			}
			truncated := stmt
			if len(truncated) > 60 {
				truncated = truncated[:60]
			}
			return fmt.Errorf("migrate stmt %q: %w", truncated, err)
		}
	}
	return nil
}

// isAlterAddColumn reports whether stmt is an ALTER TABLE ... ADD COLUMN statement.
func isAlterAddColumn(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(stmt))
	return strings.HasPrefix(upper, "ALTER TABLE") && strings.Contains(upper, "ADD COLUMN")
}

// isDuplicateColumnError reports whether err indicates a "duplicate column name" from SQLite.
func isDuplicateColumnError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}

func sqliteDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS tokens (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  token_hash  TEXT NOT NULL UNIQUE,
  token_key   TEXT NOT NULL,
  hmac_secret TEXT NOT NULL,
  owner       TEXT NOT NULL,
  note        TEXT,
  active      INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_hash ON tokens(token_hash)`,

		`CREATE TABLE IF NOT EXISTS edit_records (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  token_key     TEXT    NOT NULL,
  device_id     TEXT    NOT NULL,
  hostname      TEXT    NOT NULL DEFAULT '',
  tool          TEXT    NOT NULL,
  tool_version  TEXT,
  provider      TEXT    NOT NULL,
  model         TEXT,
  session_id    TEXT    NOT NULL,
  repo_url      TEXT    NOT NULL,
  branch        TEXT    NOT NULL,
  current_sha   TEXT    NOT NULL,
  file_path     TEXT    NOT NULL,
  added_lines   INTEGER NOT NULL,
  removed_lines INTEGER NOT NULL,
  diff_hunk     TEXT,
  metadata      TEXT,
  timestamp     TEXT    NOT NULL,
  record_sig    TEXT    NOT NULL,
  status        TEXT    NOT NULL,
  flags         TEXT,
  received_at   TEXT    NOT NULL,
  prompt_summary TEXT,
  embedding      BLOB
)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_token_key   ON edit_records(token_key)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_repo_url    ON edit_records(repo_url)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_device_id   ON edit_records(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_received_at ON edit_records(received_at)`,

		`CREATE TABLE IF NOT EXISTS devices (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id      TEXT NOT NULL UNIQUE,
  token_key      TEXT NOT NULL,
  hostname       TEXT NOT NULL DEFAULT '',
  client_version TEXT,
  last_heartbeat TEXT,
  hooks_json     TEXT,
  created_at     TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_device_id ON devices(device_id)`,

		// Additive migrations for existing SQLite databases.
		// SQLite does not support IF NOT EXISTS in ALTER TABLE ADD COLUMN,
		// so "duplicate column name" errors are silently ignored by migrate().
		`ALTER TABLE edit_records ADD COLUMN prompt_summary TEXT`,
		`ALTER TABLE edit_records ADD COLUMN embedding BLOB`,
	}
}

func postgresDDL() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS tokens (
  id          BIGSERIAL PRIMARY KEY,
  token_hash  TEXT NOT NULL UNIQUE,
  token_key   TEXT NOT NULL,
  hmac_secret TEXT NOT NULL,
  owner       TEXT NOT NULL,
  note        TEXT,
  active      INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_hash ON tokens(token_hash)`,

		`CREATE TABLE IF NOT EXISTS edit_records (
  id            BIGSERIAL PRIMARY KEY,
  token_key     TEXT    NOT NULL,
  device_id     TEXT    NOT NULL,
  hostname      TEXT    NOT NULL DEFAULT '',
  tool          TEXT    NOT NULL,
  tool_version  TEXT,
  provider      TEXT    NOT NULL,
  model         TEXT,
  session_id    TEXT    NOT NULL,
  repo_url      TEXT    NOT NULL,
  branch        TEXT    NOT NULL,
  current_sha   TEXT    NOT NULL,
  file_path     TEXT    NOT NULL,
  added_lines   INTEGER NOT NULL,
  removed_lines INTEGER NOT NULL,
  diff_hunk     TEXT,
  metadata      TEXT,
  timestamp     TEXT    NOT NULL,
  record_sig    TEXT    NOT NULL,
  status        TEXT    NOT NULL,
  flags         TEXT,
  received_at   TEXT    NOT NULL,
  prompt_summary TEXT,
  embedding      BYTEA
)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_token_key   ON edit_records(token_key)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_repo_url    ON edit_records(repo_url)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_device_id   ON edit_records(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edit_records_received_at ON edit_records(received_at)`,

		`CREATE TABLE IF NOT EXISTS devices (
  id             BIGSERIAL PRIMARY KEY,
  device_id      TEXT NOT NULL UNIQUE,
  token_key      TEXT NOT NULL,
  hostname       TEXT NOT NULL DEFAULT '',
  client_version TEXT,
  last_heartbeat TEXT,
  hooks_json     TEXT,
  created_at     TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_device_id ON devices(device_id)`,

		// Additive migrations for existing PostgreSQL databases.
		`ALTER TABLE edit_records ADD COLUMN IF NOT EXISTS prompt_summary TEXT`,
		`ALTER TABLE edit_records ADD COLUMN IF NOT EXISTS embedding BYTEA`,
	}
}
