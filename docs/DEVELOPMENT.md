# Development Guide

## Who This Is For

This guide is for engineers contributing to AiTrack. It covers local environment setup, build and test commands for each component, coverage tooling, E2E test execution, and the three-way sync requirement when the protocol changes.

---

## Local Environment Requirements

| Tool | Version | Purpose |
|------|---------|---------|
| Rust / Cargo | stable (1.82+ recommended) | Client build and tests |
| JDK | 17+ | Java server (use Docker build if JDK not installed locally) |
| Maven | 3.8+ | Java server build |
| Go | 1.24+ | Go server build and tests |
| Docker | 20+ | Cross-platform builds, Java builds, E2E tests |
| sqlite3 CLI | any | E2E test verification of local DB |
| git | any | Client git metadata extraction |

**Note**: The Java server (Spring Boot 3.3.8) requires JDK 17. If JDK is not installed locally, all Java-related operations must be done inside Docker (see "Building via Docker" below).

---

## Client (Rust)

```bash
cd client/

# Debug build
cargo build

# Release build
cargo build --release

# Run tests
cargo test

# Coverage measurement (install cargo-llvm-cov first)
cargo install cargo-llvm-cov
cargo llvm-cov --summary-only

# Coverage detail (HTML report)
cargo llvm-cov --open
```

Coverage threshold: LINE ≥ 90%. Docker builds fail if this is not met.

### CLI Commands

| Command | Description |
|---------|-------------|
| `aitrack capture` | Parse a hook event from stdin and record the edit |
| `aitrack prompt-capture` | Record a UserPromptSubmit event from stdin |
| `aitrack heartbeat` | Force-send a heartbeat immediately |
| `aitrack status` | Print config, device_id, and sync stats |
| `aitrack inspect` | Query local records.db |
| `aitrack init` | Initialize config.toml and install hooks |
| `aitrack update` | Download and verify the latest binary (ed25519-signed) |

#### `aitrack update` — Self-Update Command

`aitrack update` fetches the latest release binary and verifies it before replacing the running binary:

```
1. GET <api_url>/api/v1/ai-track/release/latest  → { version, download_url, signature_url }
2. Download binary to <tempfile>
3. Download detached ed25519 signature (.sig file)
4. Verify: ed25519::verify(PUBLIC_KEY_BYTES, sha256(binary), signature)
   → abort with error if verification fails
5. Atomic rename: tempfile → current binary path (via std::fs::rename)
```

The ed25519 public key is compiled into the binary at build time (`include_bytes!`). A tampered binary or mismatched signature causes a hard abort — the old binary is not replaced.

### Rust Client Module Structure (Sprint 2 Hexagonal)

```
client/src/
├── main.rs / cli.rs / config.rs / lib.rs   — command dispatch, config, entry point
├── git.rs / init.rs / uploader.rs / heartbeat.rs / update.rs
│
├── domain/        ← Pure domain logic, zero infrastructure deps
│   ├── model.rs   ← EditRecord, ApiConfig and other core domain models
│   ├── crypto.rs  ← HMAC-SHA256, record_sig, request signing
│   ├── diff.rs    ← Myers/LCS diff (similar crate)
│   └── keywords.rs ← Hardcoded keywords + SHA256 fingerprint
│
├── port/
│   ├── storage.rs ← StoragePort trait
│   └── upload.rs  ← UploadPort trait
│
├── adapter/
│   ├── sqlite/    ← SqliteStorage impl StoragePort
│   │   └── mod.rs / schema.rs / models.rs / queries.rs / vec.rs / keyword_store.rs
│   ├── http/      ← HttpUploader impl UploadPort (real HTTP POST)
│   │   └── mod.rs / upload.rs
│   └── event/     ← claude/codex/cursor adapters
│       └── mod.rs / claude.rs / codex.rs / cursor.rs
│
└── testkit/factories.rs   ← Seed-deterministic test factories
```

### Test Coverage by Module

| Module | Tests (Sprint 2) | Line Coverage |
|--------|---------|---------------|
| `domain/` | — | ≥ 90% |
| `port/` | — | ≥ 90% |
| `adapter/sqlite/`, `adapter/http/`, `adapter/event/` | — | ≥ 90% |
| `config.rs` / `git.rs` / `init.rs` / `uploader.rs` / `update.rs` / ... | — | ≥ 90% |
| **TOTAL** | **291** | **90.71% LINE** |

> After the Sprint 2 hexagonal architecture refactor, tests were reorganized with the domain modules. Total test count increased from 143 to 291. Run `cargo llvm-cov --summary-only` for the latest per-module breakdown.

All tests are `#[cfg(test)]` inline modules. HTTP mocking uses `wiremock`; temporary files use `tempfile`.

#### sqlite-vec (optional vector extension)

The `client/src/db/vec.rs` module registers sqlite-vec via `sqlite3_auto_extension` at DB-open time. If the extension probe (`SELECT vec_version()`) fails, the `VEC_DISABLED` global is set and all vector operations are skipped — the core capture pipeline is unaffected.

To verify sqlite-vec loaded correctly:
```bash
./target/debug/aitrack status   # logs "sqlite-vec loaded: v0.1.x" at DEBUG level
```

The `vec_records` virtual table (`vec0`, `float[384]`) is created automatically when vec is enabled. Embeddings are not populated until Phase DB-3.

### Testkit Factories

`src/testkit/factories.rs` provides seed-deterministic builders:

```rust
// Valid instances
let rec = EditRecordFactory::new(42).with_tool("claude").build();
let cfg = ApiConfigFactory::new(42).with_hmac_secret("secret").build();

// Payload JSON
let json = ClaudeHookPayloadFactory::new(1).build_json();

// Negative cases (for anti-validation tests)
let bad = tampered_record_sig(1);        // record_sig zeroed
let exp = tampered_expired_timestamp(1); // timestamp = 2000-01-01
let big = tampered_oversized_lines(1);  // added_lines = 99,999,999
```

---

## Java Server

```bash
cd server-java/

# Run tests (unit + integration, H2 in-memory)
mvn test

# Run tests + coverage verification (LINE ≥ 90% threshold)
mvn verify

# Start dev server
mvn spring-boot:run
# → http://localhost:8080
# → H2 console: http://localhost:8080/h2-console
```

JaCoCo HTML report: `target/site/jacoco/index.html`

#### PostgreSQL / ParadeDB profile

```bash
# Run with postgres profile (requires ParadeDB running on localhost:5432)
SPRING_PROFILES_ACTIVE=postgres mvn spring-boot:run
```

### Building via Docker (when JDK is not installed locally)

```bash
# Run from project root
docker build -f docker/Dockerfile.server-java -t aitrack-server-java:latest .
```

`mvn verify` is run automatically during the build; the build fails if coverage is insufficient.

### Testkit Factories

```java
// Valid instances
EditDto dto = EditDtoFactory.build();
EditDto dto = EditDtoFactory.with(e -> e.setTool("codex"));
EditDto dto = EditDtoFactory.buildForTool("cursor");

// Negative cases
EditDto bad = TamperedFactory.badRecordSig();
EditDto bad = TamperedFactory.oversizedAddedLines();
EditDto bad = TamperedFactory.nullTool();
```

---

## Go Server

```bash
cd server-go/

# Build
go build ./...

# Run (SQLite stored at ./data/aitrack.db by default, port 8080)
go run .

# Run tests
go test -ldflags=-linkmode=external ./... -cover

# On Linux/Docker (no Darwin dyld issues)
go test ./... -coverprofile=cover.out
go tool cover -func=cover.out | tail -1

# Build via Docker
docker build -f docker/Dockerfile.server-go -t aitrack-server-go:latest .
```

Coverage threshold: total ≥ 90%. Docker builds fail if this is not met. Current coverage: **95.3%**.

### testapp Package — In-Process Integration Testing

`server-go/testapp/` exports a lightweight wiring layer for integration tests that need a live HTTP router without Docker:

```go
import "github.com/your-org/aitrack/server-go/testapp"

func TestChainIntegration(t *testing.T) {
    adminKey := "test-admin-key-32chars-xxxxxxxxxx"
    cfg := testapp.MemoryConfig(adminKey)   // in-memory SQLite, random port
    handler, cleanup, _ := testapp.Build(cfg)
    defer cleanup()

    srv := httptest.NewServer(handler)
    defer srv.Close()
    // ... hit srv.URL endpoints with http.DefaultClient
}
```

`MemoryConfig` returns a `config.Config` with `DSN=":memory:"` and the provided admin key, bypassing Go's `internal` package restriction so test files outside `server-go/internal/` can wire up a real server.

### Testkit Factories

```go
tok := testkit.BuildToken()
dto := testkit.BuildEditDTO()
req := testkit.BuildUploadRequest(tok, dto)
hb  := testkit.BuildHeartbeatRequest()

// Negative cases
bad := testkit.TamperedEditDTO()
exp := testkit.ExpiredTimestampEditDTO()
big := testkit.OversizedEditDTO()
```

#### ParadeDB local dev

To run the Go server against a local ParadeDB instance:
```bash
DATABASE_URL=postgres://aitrack:aitrack_secret@localhost:5432/aitrack go run .
```
Without `DATABASE_URL`, the server falls back to embedded SQLite (default for local dev).

---

## E2E Tests

The E2E test suite lives in `e2e/` and runs one pass against each of the Java and Go implementations, proving protocol compatibility.

### Go runner (simulated client)

```bash
# From project root
bash e2e/run.sh both   # Java + Go
bash e2e/run.sh java   # Java only
bash e2e/run.sh go     # Go only
```

The script automatically builds three Docker images, starts server containers, runs tests, and tears down containers.

### Real Rust Binary E2E

```bash
# Requires: cargo, sqlite3, curl, git, python3, uuidgen installed locally
bash e2e/run-client-e2e.sh both
```

Tests use a temporary `AITRACK_HOME` directory and do not touch `~/.aitrack/` or `~/.claude/`.

### docker-compose E2E (for CI)

```bash
docker compose -f docker/docker-compose.e2e.yml --profile java up --abort-on-container-exit
docker compose -f docker/docker-compose.e2e.yml --profile go up --abort-on-container-exit
```

---

## Coverage Summary

| Component | Tool | Command | Threshold | Current (v1.6.0) |
|-----------|------|---------|-----------|-----------------|
| Rust client | cargo-llvm-cov | `cargo llvm-cov --summary-only` | LINE ≥ 90% | **90.71%** |
| Java server | JaCoCo | `mvn verify` | LINE ≥ 90% | **LINE ≥ 90%** |
| Go server | go cover | `go tool cover -func cover.out` | total ≥ 90% | **95.3%** |

All three component Docker builds embed the coverage check; builds fail if the threshold is not met.

---

## Protocol Change Rules

`CONTRACT.md` is the single source of truth shared by the Rust client, Java server, and Go server. Any protocol change must be synchronized across all three:

1. **Update `CONTRACT.md`**: bump version, describe the change
2. **Update Rust client**: `crypto.rs` (record_sig canonical string), corresponding adapter, uploader
3. **Update Java server**: `SignatureService` (canonical string), `EditDto` (fields), related tests
4. **Update Go server**: `service/signature.go` (canonical string), `model` (fields), related tests
5. **Update E2E factory**: `ComputeRecordSig` in `e2e/factory/factory.go`
6. **Run the E2E suite** to verify three-way compatibility

The field order and `\n` separators in the `record_sig` canonical string must be byte-identical across all three components. See the Record Signature section in `CONTRACT.md`.
