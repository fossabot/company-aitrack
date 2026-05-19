// Package testapp exposes a public (non-internal) surface for wiring the full
// Go server for integration tests that live outside the server-go module tree.
//
// This package deliberately has no build tag so it is compiled in both test and
// non-test contexts. It is intended only for testing — production code should
// not depend on it.
package testapp

import (
	"net/http"

	"github.com/aitrack/server/internal/infrastructure/app"
	"github.com/aitrack/server/internal/infrastructure/config"
)

// Config is a re-export of the infrastructure config so callers outside the
// internal tree can populate it without importing internal packages directly.
type Config = config.Config

// Build wires all dependencies and returns an http.Handler ready to serve.
// The returned cleanup function must be called when the handler is no longer
// needed (it closes the database).
//
// Pass a *Config with DB.Path = ":memory:" and an empty SecretKey to get an
// in-memory SQLite server running in dev-mode (no real encryption key needed).
func Build(cfg *Config) (http.Handler, func(), error) {
	return app.Build(cfg)
}

// MemoryConfig returns a *Config suitable for integration tests:
//   - In-memory SQLite (no files created)
//   - Dev-mode encryptor (empty SecretKey → plain: prefix storage)
//   - A caller-supplied admin key (used for /admin/tokens and /profiles endpoints)
//   - Permissive rate-limit and size limits
//   - Repo whitelist enforcement disabled
func MemoryConfig(adminKey string) *Config {
	cfg := &Config{}
	cfg.Server.Port = 0
	cfg.DB.Path = ":memory:"
	cfg.DB.DatabaseURL = ""
	cfg.SecretKey = ""       // dev mode: plain-prefix storage
	cfg.AdminKey = adminKey
	cfg.TimestampWindowSeconds = 300
	cfg.RateLimitPerHour = 100
	cfg.MaxAddedLines = 5000
	cfg.RepoWhitelist.Enforce = false
	return cfg
}
