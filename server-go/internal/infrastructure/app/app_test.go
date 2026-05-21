package app_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/aitrack/server/internal/infrastructure/app"
	"github.com/aitrack/server/internal/infrastructure/config"
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

func TestBuild_OK(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Port = 8080
	cfg.DB.DatabaseURL = testDSN()
	cfg.TimestampWindowSeconds = 300
	cfg.RateLimitPerHour = 30
	cfg.MaxAddedLines = 5000
	cfg.AdminKey = "admin"

	handler, cleanup, err := app.Build(cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	defer cleanup()

	if handler == nil {
		t.Error("handler should not be nil")
	}

	// Smoke test: hit a route
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai-track/stats", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	// No auth → 401
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}
}

func TestBuild_BadEncryptorKey(t *testing.T) {
	cfg := &config.Config{}
	cfg.DB.DatabaseURL = testDSN()
	cfg.SecretKey = "not-valid-base64!!!" // will fail

	_, _, err := app.Build(cfg)
	if err == nil {
		t.Error("expected error for bad secret_key")
	}
}

func TestBuild_MissingDatabaseURL(t *testing.T) {
	cfg := &config.Config{}
	cfg.DB.DatabaseURL = "" // required — must return error

	_, _, err := app.Build(cfg)
	if err == nil {
		t.Error("expected error when DATABASE_URL is empty")
	}
}
