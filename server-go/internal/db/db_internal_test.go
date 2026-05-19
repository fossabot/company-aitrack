package db

import (
	"database/sql"
	"errors"
	"strings"
	"testing"
)

// TestMigrateOnClosedDB exercises the migrate function directly with a closed db.
func TestMigrateOnClosedDB(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	if err := migrate(db, false); err == nil {
		t.Error("expected migrate to fail on closed db")
	}
}

// TestOpen_MigrateError exercises the branch in Open where migrate returns an error.
// We inject a failing migrateFn to hit the db.Close() + return error path.
func TestOpen_MigrateError(t *testing.T) {
	failMigrate := func(*sql.DB) error {
		return errors.New("injected migrate failure")
	}
	_, err := Open(":memory:", failMigrate)
	if err == nil {
		t.Fatal("expected Open to return error when migrate fails")
	}
	if err.Error() == "" {
		t.Error("error should have a message")
	}
}

// TestPostgresDDL verifies the PostgreSQL DDL generator returns a non-empty statement list
// that includes the expected tables and new columns.
func TestPostgresDDL(t *testing.T) {
	stmts := postgresDDL()
	if len(stmts) == 0 {
		t.Fatal("postgresDDL returned no statements")
	}
	joined := strings.Join(stmts, "\n")
	for _, want := range []string{
		"tokens",
		"edit_records",
		"devices",
		"prompt_summary",
		"embedding",
		"BIGSERIAL",
		"BYTEA",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("postgresDDL missing %q", want)
		}
	}
}

// TestSQLiteDDL verifies the SQLite DDL generator returns a non-empty statement list
// that includes the expected tables and new columns.
func TestSQLiteDDL(t *testing.T) {
	stmts := sqliteDDL()
	if len(stmts) == 0 {
		t.Fatal("sqliteDDL returned no statements")
	}
	joined := strings.Join(stmts, "\n")
	for _, want := range []string{
		"tokens",
		"edit_records",
		"devices",
		"prompt_summary",
		"embedding",
		"INTEGER PRIMARY KEY AUTOINCREMENT",
		"BLOB",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("sqliteDDL missing %q", want)
		}
	}
}

// TestOpen_WithDatabaseURL_InvalidURL verifies Open fails gracefully when given an
// invalid PostgreSQL URL (the pgx driver accepts the DSN but fails at query time).
func TestOpen_WithDatabaseURL_InvalidURL(t *testing.T) {
	// sql.Open("pgx", ...) is lazy — it doesn't actually connect until a query is made.
	// When migrate() runs, it will try to execute SQL and fail because the host is unreachable.
	// We just verify that Open returns an error (from migrate) rather than panicking.
	_, err := Open("/ignored/path", WithDatabaseURL("postgres://invalid-host-no-such-server/db?sslmode=disable&connect_timeout=1"))
	if err == nil {
		// In CI or environments where the host resolves, it might time out; that's also fine.
		t.Log("unexpected success connecting to invalid postgres host (may be environment-specific)")
		return
	}
	// Error should be wrapped with "migrate:" prefix.
	if !strings.Contains(err.Error(), "migrate") {
		t.Errorf("expected error to mention migrate, got: %v", err)
	}
}

// TestIsAlterAddColumn verifies the helper correctly identifies ALTER TABLE ADD COLUMN.
func TestIsAlterAddColumn(t *testing.T) {
	cases := []struct {
		stmt string
		want bool
	}{
		{"ALTER TABLE edit_records ADD COLUMN prompt_summary TEXT", true},
		{"ALTER TABLE t ADD COLUMN IF NOT EXISTS x INT", true},
		{"CREATE TABLE IF NOT EXISTS tokens (id INTEGER)", false},
		{"CREATE INDEX IF NOT EXISTS idx ON t(col)", false},
		{"alter table t add column x text", true},
	}
	for _, tc := range cases {
		if got := isAlterAddColumn(tc.stmt); got != tc.want {
			t.Errorf("isAlterAddColumn(%q) = %v, want %v", tc.stmt, got, tc.want)
		}
	}
}

// TestIsDuplicateColumnError verifies the helper detects duplicate column errors.
func TestIsDuplicateColumnError(t *testing.T) {
	if isDuplicateColumnError(nil) {
		t.Error("nil error should not be duplicate column error")
	}
	if isDuplicateColumnError(errors.New("some other error")) {
		t.Error("unrelated error should not be duplicate column error")
	}
	if !isDuplicateColumnError(errors.New("SQL logic error: duplicate column name: prompt_summary (1)")) {
		t.Error("duplicate column name error should be detected")
	}
}
