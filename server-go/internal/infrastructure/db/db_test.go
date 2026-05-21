package db_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/aitrack/server/internal/infrastructure/db"
)

func testDSN() string {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		return "postgres://aitrack:aitrack_secret@localhost:5432/aitrack_test?sslmode=disable"
	}
	return dsn
}

func TestMain(m *testing.M) {
	conn, err := sql.Open("pgx", testDSN())
	if err != nil || conn.Ping() != nil {
		fmt.Println("SKIP: TEST_DATABASE_URL not reachable, skipping DB integration tests")
		os.Exit(0) // skip but pass
	}
	conn.Close()
	os.Exit(m.Run())
}

func TestOpen_Connects(t *testing.T) {
	database, err := db.Open(testDSN())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestMigrate_TablesExist(t *testing.T) {
	database, err := db.Open(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	tables := []string{"tokens", "edit_records", "devices"}
	for _, table := range tables {
		var name string
		err := database.QueryRow(
			"SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename=$1", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestMigrate_IndexesExist(t *testing.T) {
	database, err := db.Open(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	indexes := []string{
		"idx_tokens_hash",
		"idx_edit_records_token_key",
		"idx_edit_records_repo_url",
		"idx_edit_records_device_id",
		"idx_edit_records_received_at",
		"idx_devices_device_id",
	}
	for _, idx := range indexes {
		var name string
		err := database.QueryRow(
			"SELECT indexname FROM pg_indexes WHERE schemaname='public' AND indexname=$1", idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %q not found: %v", idx, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	// Open twice — migrate runs both times, must not error
	db1, err := db.Open(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	db1.Close()

	db2, err := db.Open(testDSN())
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	db2.Close()
}

func TestTokensCRUD(t *testing.T) {
	database, err := db.Open(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Clean up before test to ensure idempotency
	database.Exec("DELETE FROM tokens WHERE token_key = 'key-crud-test'")

	_, err = database.Exec(`
		INSERT INTO tokens (token_hash, token_key, hmac_secret, owner, note, active, created_at)
		VALUES ('hash-crud-test', 'key-crud-test', 'secret1', 'alice', 'note', 1, '2026-05-17T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert token: %v", err)
	}

	var hash string
	err = database.QueryRow("SELECT token_hash FROM tokens WHERE token_key = $1", "key-crud-test").Scan(&hash)
	if err != nil {
		t.Fatalf("query token: %v", err)
	}
	if hash != "hash-crud-test" {
		t.Errorf("got hash %q, want hash-crud-test", hash)
	}

	database.Exec("DELETE FROM tokens WHERE token_key = 'key-crud-test'")
}

func TestEditRecordsCRUD(t *testing.T) {
	database, err := db.Open(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	database.Exec("DELETE FROM edit_records WHERE token_key='k-crud-er-test'")

	_, err = database.Exec(`
		INSERT INTO edit_records
		  (token_key, device_id, tool, provider, session_id, repo_url, branch,
		   current_sha, file_path, added_lines, removed_lines, timestamp, record_sig,
		   status, received_at)
		VALUES ('k-crud-er-test','d1','claude','anthropic','s1','repo','main','sha','f.rs',5,2,
		        '2026-05-17T00:00:00Z','sig1','ACCEPTED','2026-05-17T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert edit_record: %v", err)
	}

	var count int64
	database.QueryRow("SELECT COUNT(*) FROM edit_records WHERE token_key='k-crud-er-test'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 edit record, got %d", count)
	}

	database.Exec("DELETE FROM edit_records WHERE token_key='k-crud-er-test'")
}

func TestDevicesCRUD(t *testing.T) {
	database, err := db.Open(testDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	database.Exec("DELETE FROM devices WHERE device_id = 'dev-crud-test'")

	_, err = database.Exec(`
		INSERT INTO devices (device_id, token_key, client_version, created_at)
		VALUES ('dev-crud-test', 'key1', '1.0.0', '2026-05-17T00:00:00Z')
	`)
	if err != nil {
		t.Fatalf("insert device: %v", err)
	}

	// Upsert via ON CONFLICT
	_, err = database.Exec(`
		INSERT INTO devices (device_id, token_key, client_version, created_at)
		VALUES ('dev-crud-test', 'key1', '2.0.0', '2026-05-17T00:00:00Z')
		ON CONFLICT(device_id) DO UPDATE SET client_version = excluded.client_version
	`)
	if err != nil {
		t.Fatalf("upsert device: %v", err)
	}

	var version string
	database.QueryRow("SELECT client_version FROM devices WHERE device_id='dev-crud-test'").Scan(&version)
	if version != "2.0.0" {
		t.Errorf("expected 2.0.0 after upsert, got %s", version)
	}

	database.Exec("DELETE FROM devices WHERE device_id = 'dev-crud-test'")
}
