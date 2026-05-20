# Testing: Unified Test Factories and Coverage

This document describes the test architecture, factory pattern, coverage thresholds, and Docker verification flow for all three aitrack components.

---

## Three-Tier Test Architecture

```
Unit Tests
  ├── Pure functions, business logic, HMAC canonical values
  └── No network/DB dependencies (wiremock / H2 / SQLite in-memory)

Integration Tests
  ├── Full Spring context + H2 in-memory (Java)
  ├── Real SQLite (Go)
  └── HTTP end-to-end (mock server / MockMvc)

E2E Tests
  ├── Runs inside real Docker containers (bash e2e/run.sh)
  ├── One pass each for Java and Go servers
  ├── Covers the full chain from token issuance to stats queries
  └── Real-chain integration tests (chain_integration_test.go): Go router + in-memory SQLite, no Docker
```

---

## Coverage Thresholds (90%)

| Component | Measurement | Failure behavior | Current (v1.6.0) |
|-----------|------------|-----------------|-----------------|
| **Rust client** | `cargo llvm-cov --summary-only` → parse `TOTAL` line | Build fails below 90% | **90.71%** |
| **Java server** | JaCoCo `LINE COVEREDRATIO >= 0.90` (pom.xml `verify` phase) | Build fails below 90% | **LINE ≥ 90%** |
| **Go server** | `go tool cover -func cover.out` → parse `total` line | Build fails below 90% | **95.3%** |

---

## Test Factory Pattern (Three Languages)

All factories follow the same conventions:

1. **Deterministic (seed-based)**: given the same seed, the same data is produced every time
2. **Builder style**: default valid instance + field-level overrides
3. **Negative factories**: explicitly named tamper methods (`tampered_*`, `Tampered*`)
4. **HMAC embedded**: the factory computes the correct `record_sig` internally so default instances pass signature verification

### Rust (`client/src/testkit/factories.rs`)

```rust
// Valid instances
let rec = EditRecordFactory::new(42).with_tool("claude").build();
let cfg = ApiConfigFactory::new(42).with_hmac_secret("secret").build();

// Payload JSON
let json = ClaudeHookPayloadFactory::new(1).build_json();
let json = CodexHookPayloadFactory::new(2).build_json();
let json = CursorHookPayloadFactory::new(3).build_json();

// Negative cases
let bad = tampered_record_sig(1);        // record_sig zeroed
let exp = tampered_expired_timestamp(1); // timestamp = 2000-01-01
let big = tampered_oversized_lines(1);  // added_lines = 99,999,999
```

### Java (`server-java/src/test/java/com/aitrack/server/testkit/`)

```java
// Valid instances
EditDto dto = EditDtoFactory.build();
EditDto dto = EditDtoFactory.with(e -> e.setTool("codex"));
EditDto dto = EditDtoFactory.buildForTool("cursor");

// Negative cases
EditDto bad = TamperedFactory.badRecordSig();
EditDto bad = TamperedFactory.oversizedAddedLines();
EditDto bad = TamperedFactory.nullTool();

// Batch request
EditBatchRequest req = EditBatchRequestFactory.build(dto);

// Token
TokenEntity tok = TokenEntityFactory.build();
```

### Go (`server-go/internal/testkit/factory.go`)

```go
// Valid instances (functional options style)
tok := testkit.BuildToken()
dto := testkit.BuildEditDTO()
req := testkit.BuildUploadRequest(tok, dto)
hb  := testkit.BuildHeartbeatRequest()

// Negative cases
bad := testkit.TamperedEditDTO()
exp := testkit.ExpiredTimestampEditDTO()
big := testkit.OversizedEditDTO()
mal := testkit.MalformedEditDTO()
```

### E2E (`e2e/factory/factory.go`)

```go
// Payload constructed from real code snippets extracted from fixtures/prompts/
p := factory.DefaultEditParams(seed, tok)
body := factory.BuildBatchRequest(deviceID, p.BuildDTO())
hb   := factory.BuildHeartbeatRequest(deviceID, tokenKey, pendingCount)

// Negative cases
tampered := factory.TamperedRecordSig(p)
oversized := factory.OversizedEdit(p)
missing   := factory.MissingFieldEdit(p)

// HMAC canonical string (exactly as defined in CONTRACT.md)
sig := factory.ComputeRecordSig(secret, tokenKey, deviceID, ...)
reqSig := factory.ComputeRequestSig(secret, unixTS, bodyBytes)
```

---

## E2E Scenario Coverage

| Scenario | Contract requirements covered |
|----------|------------------------------|
| Admin token auth | 403 wrong key / 400 missing field / 200 normal issuance |
| Contract layer validation | 401 no auth / wrong token / expired ts / bad signature; 400 empty edits |
| Full-chain happy path | sign token → POST /edits → accepted=1 → GET → stats → devices |
| Anti-cheat chain | tampered record_sig → rejected; oversized → flagged; missing field → rejected |
| Heartbeat chain | POST /heartbeat → ok=true; devices reflects device |
| Repo allowlist | unknown repo accepted or flagged (not hard-rejected) when enforce=false |

### chain_integration_test.go — In-Process Go Integration Tests (Sprint 2)

`server-go/chain_integration_test.go` runs 3 full end-to-end scenarios against a real Go chi router with an in-memory SQLite database — no Docker, no network sockets required.

**Setup via testapp package:**

```go
import "github.com/your-org/aitrack/server-go/testapp"

func setupServer(t *testing.T) (string, string) {
    adminKey := "integration-test-admin-key-32c"
    cfg := testapp.MemoryConfig(adminKey)  // DSN=":memory:", no file I/O
    handler, cleanup, _ := testapp.Build(cfg)
    t.Cleanup(cleanup)
    srv := httptest.NewServer(handler)
    t.Cleanup(srv.Close)
    return srv.URL, adminKey
}
```

**Three covered scenarios:**

| Scenario | What it proves |
|----------|---------------|
| Happy path | `POST /admin/tokens` → token issued; `POST /edits/batch` → `accepted=1`; `GET /edits` → record visible |
| Tampered record_sig | Same flow but record_sig zeroed → `rejected: sig_mismatch`; record count unchanged |
| Credential error (401) | Request signed with wrong hmac_secret → 401; no records written |

These tests run as part of `go test ./...` with no extra flags and contribute to the 95.3% coverage figure.

---

## Docker Verification Flow

```
┌───────────────────────────────────────────────────────┐
│  Dockerfile.client (rust:1.82)                         │
│    cargo fetch --locked                                 │
│    cargo build --release --locked                       │
│    cargo test --locked                                  │
│    cargo llvm-cov → check TOTAL ≥ 90%                  │
│  → COPY aitrack binary → debian:bookworm-slim           │
└───────────────────────────────────────────────────────┘
┌───────────────────────────────────────────────────────┐
│  Dockerfile.server-java (maven:3.9-eclipse-temurin-17) │
│    mvn dependency:go-offline                            │
│    mvn verify  ← JaCoCo LINE ≥ 90% gate inside here   │
│  → COPY .jar → eclipse-temurin:17-jre                  │
└───────────────────────────────────────────────────────┘
┌───────────────────────────────────────────────────────┐
│  Dockerfile.server-go (golang:1.24)                    │
│    go mod download                                      │
│    go test ./... -coverprofile=cover.out               │
│    go tool cover → check total ≥ 90%                   │
│    CGO_ENABLED=0 go build -o aitrack-server            │
│  → COPY binary → distroless/base-debian12              │
└───────────────────────────────────────────────────────┘
```

### Local verification commands

```bash
# From project root

# Client
docker build -f docker/Dockerfile.client -t aitrack-client:latest . 2>&1 | tail -20

# Java server
docker build -f docker/Dockerfile.server-java -t aitrack-server-java:latest . 2>&1 | tail -20

# Go server
docker build -f docker/Dockerfile.server-go -t aitrack-server-go:latest . 2>&1 | tail -20

# E2E (one pass for Java + Go)
bash e2e/run.sh both

# Real-chain integration tests (no Docker, direct Go test)
cd e2e && go test ./... -run TestReal -v
```

---

## Key Notes

- **Java builds must run inside Docker**: JDK 17/Maven is not required locally; `Dockerfile.server-java` uses the `maven:3.9-eclipse-temurin-17` image for all build and test steps.
- **Go tests have no CGO issues in Linux containers**: `modernc.org/sqlite` is a pure-Go implementation; `CGO_ENABLED=0` builds work cleanly.
- **E2E does not modify real editor configuration**: all operations run in containerized or isolated environments and do not touch `~/.aitrack/`, `~/.claude/`, or similar directories.
