# System Architecture

## Who This Is For

This document is for developers, operators, and security reviewers who want to understand the overall design of AiTrack. It describes the responsibilities of each component, data flows, protocol versioning, and technology choices.

---

## Component Overview

AiTrack consists of three independent components that communicate over HTTP/JSON. All behavioral rules are governed by `CONTRACT.md` v1.2.

```
┌─────────────────────────────────────────────────────┐
│  AI Coding Tools (Claude Code / Codex CLI / Cursor)  │
│  PostToolUse / afterFileEdit / UserPromptSubmit hooks │
└────────────────────┬────────────────────────────────┘
                     │ stdin JSON
                     ▼
┌─────────────────────────────────────────────────────┐
│  Rust Client  aitrack                               │
│  · adapter parsing   · Myers/LCS diff               │
│  · record_sig        · SQLite local storage         │
│  · flush upload      · throttled heartbeat          │
└────────────────────┬────────────────────────────────┘
                     │ POST /api/v1/ai-track/edits
                     │ POST /api/v1/ai-track/heartbeat
                     ▼
┌────────────────────────────┐  ┌────────────────────────────┐
│  Java Server               │  │  Go Server                 │
│  Spring Boot 3 / H2 / PG  │  │  chi v5 / SQLite (pure Go) │
│  10-step validation chain  │  │  10-step validation chain   │
│                            │  │  (fully equivalent)         │
└────────────────────────────┘  └────────────────────────────┘
```

---

## Client: Rust CLI

**Responsibility**: when an AI tool triggers an edit event, capture, sign, store locally, and report one edit record.

### Module Layout (Hexagonal Architecture, Sprint 2)

```
src/
  main.rs / cli.rs / config.rs / lib.rs   — command dispatch, config, entry point
  git.rs / init.rs / uploader.rs / heartbeat.rs / update.rs

  domain/                   — pure domain logic, no framework dependencies
    mod.rs
    model.rs                — EditRecord, ApiConfig and other core domain models
    crypto.rs               — HMAC-SHA256, record_sig, request signing
    diff.rs                 — Myers/LCS diff (similar crate)
    keywords.rs             — prompt intent classification keywords + SHA-256 fingerprint

  port/                     — output ports (abstract interfaces)
    mod.rs
    storage.rs              — StoragePort (local SQLite read/write)
    upload.rs               — UploadPort (HTTP upload)

  adapter/                  — adapter implementations
    mod.rs
    sqlite/                 — StoragePort SQLite implementation
      mod.rs / schema.rs / models.rs / queries.rs / vec.rs / keyword_store.rs
    http/                   — UploadPort HTTP implementation (real HTTP POST)
      mod.rs / upload.rs
    event/                  — input adapters (hook event parsing)
      mod.rs / claude.rs / codex.rs / cursor.rs

  db/                       — legacy db module (retained for backward compatibility)
  adapters/                 — legacy adapters (retained for backward compatibility)
  testkit/factories.rs      — seed-deterministic test factories
```

### Local Storage

- `~/.aitrack/config.toml` (0600): api_url, credential, device_id
- `~/.aitrack/records.db` (0600): SQLite, stores all captured records

`device_id` is generated as UUIDv4 on first run and persisted; read-only after that.

---

## Data Flow: From Hook Trigger to Database

### Edit Event Flow (PostToolUse / afterFileEdit)

1. AI tool fires PostToolUse/afterFileEdit hook
2. `aitrack capture` reads JSON from stdin
3. Select adapter by `--tool` flag (claude/codex/cursor) and parse payload
4. Call `similar` crate to compute Myers/LCS diff
   → added_lines, removed_lines, diff_hunk
5. Spawn git to get repo metadata
   → repo_url, branch, current_sha
6. Fetch OS hostname
7. Query `prompt_context` table for most recent prompt in current session → prompt_summary (optional, Claude only)
8. Compute record_sig
   → HMAC_SHA256(hmac_secret, canonical_string) (prompt_summary excluded)
9. 2-second deduplication window check (same session_id + file_path)
10. INSERT INTO records (synced=0)
11. flush_unsynced → POST /api/v1/ai-track/edits
    → server 10-step validation chain
    → update synced/retry_count

### Prompt Capture Flow (UserPromptSubmit, Claude Code only)

1. Claude Code fires UserPromptSubmit hook
2. `aitrack prompt-capture` reads JSON from stdin (`{"session_id": "...", "prompt": "..."}`)
3. Truncate to 512 characters
4. INSERT INTO prompt_context (session_id, prompt_text)

---

## Server 10-Step Validation Chain

The server executes the following checks on each uploaded batch in order:

| Step | Check | Failure result |
|------|-------|---------------|
| 1 | Bearer token valid | 401, reject entire batch |
| 2 | X-AiTrack-Timestamp within ±300 seconds of server time | 401, reject entire batch |
| 3 | X-AiTrack-Signature HMAC verification | 401, reject entire batch |
| 4 | Per-record record_sig HMAC verification | single record: `rejected: sig_mismatch` |
| 5 | diff_hunk line count consistent with added/removed_lines (±1) | single record: `flagged: diff_inconsistent` |
| 6 | repo_url in allowlist (when enforce=true) | single record: `flagged/rejected: repo_unknown` |
| 7 | file_path sanity check | single record: `flagged: path_mismatch` |
| 8 | added_lines ≤ max_added_lines (default 5000) | single record: `flagged: oversized` |
| 9 | Rate limit: (token, file_path) ≤ 30 per hour | single record: `rejected: rate_limited` |
| 10 | Write accepted + flagged records to database | — |

Flagged records are written to the database and marked for admin review. Rejected records are not written; the client increments retry_count.

---

## Protocol v1.2 Overview

**v1.2 change**: `POST /admin/tokens` now returns a single `credential` field (`<token>-<hmac_secret>` combined string) instead of separate `token` and `hmac_secret` fields. The client's `config.toml` key was merged from `token`/`hmac_secret` into `credential`; the CLI parameter changed to `--credential`.

**v1.1 change**: Added `hostname` field to edit records and heartbeat requests, allowing per-machine attribution when the same token is used from multiple machines.

- `hostname` is not an access control mechanism and does not enforce per-token isolation
- Using the same token from multiple machines is a valid scenario; `hostname` is used for manual review to distinguish sources

Request signing (two types):

```
# Request-level signature (anti-replay)
X-AiTrack-Signature = HMAC_SHA256(hmac_secret, "{unix_ts}\n{sha256_hex(body_bytes)}")

# Record-level signature (anti-tampering / anti-forgery)
record_sig = HMAC_SHA256(hmac_secret, canonical_string)
```

The canonical_string field order is strictly defined in `CONTRACT.md`. Client and server must be byte-identical.

---

## Technology Choices

### Why Rust for the client

- No runtime dependencies; single binary; easy for developers to install
- Hook commands need low latency (default 10-second timeout); Rust startup has no JVM/Node overhead
- The `similar` crate provides a well-tested Myers/LCS diff implementation, preventing naive line-count inflation

### Why two server implementations (Java and Go)

Both implementations are functionally and protocol-equivalent (wire-compatible), offering different operational options:

| Dimension | Java (Spring Boot 3.3.8) | Go (chi v5.2.5) |
|-----------|-------------------------|-----------------|
| Database | H2 (default) / PostgreSQL | SQLite (pure Go, no CGO) |
| Deployment | JRE + jar, suits existing JVM infrastructure | Single binary, distroless image, ideal for minimal containers |
| ORM | Spring Data JPA / Hibernate | Native database/sql, no ORM |
| Best for | Teams with existing Java stack | Lightweight container or JVM-free environments |

Both implementations share the same E2E test suite (`e2e/`) to prove protocol compatibility.

---

## Hexagonal Architecture (Sprint 2)

All three components follow the same hexagonal (ports-and-adapters) pattern:

**Rust client**
```
domain/     — pure domain logic (model.rs, crypto.rs, diff.rs, keywords.rs)
port/       — output port abstractions (storage.rs → StoragePort, upload.rs → UploadPort)
adapter/    — implementations (sqlite/, http/, event/)
```

**Go server**
```
domain/model/    — EditRecord, HeartbeatRecord, Token value objects
domain/port/     — EditRecordPort, HeartbeatPort, TokenPort interfaces
domain/service/  — ValidationPolicy value object and domain services
application/     — IngestUsecase, ProfileUsecase, TokenUsecase
adapter/         — db/ (SQLite impl), handler/ (HTTP handlers)
infrastructure/  — app/ (wiring), config/ (env/flags)
```

**Java server** — mirrors Go's layering with Spring Boot:
```
domain/model/    — JPA entities and value objects; PageResult<T> replaces Spring Page<T>
domain/port/     — DevicePort, EditRecordPort, TokenPort interfaces
domain/service/  — ValidationPolicy.java (pure, no Spring dependency)
application/     — EditSearchService, HeartbeatService, IngestService, …
adapter/         — db/ (JPA repos), handler/ (Spring MVC controllers)
infrastructure/  — app/ (Spring Boot entry), config/ (profiles)
```

> `PageResult<T>` is a plain Java generic class that mirrors the shape of Spring's `Page<T>` without importing `spring-data-commons`, keeping the domain layer framework-free.

### HttpUploader Data Flow

```
capture → lib.rs
  → uploader::flush_unsynced(&HttpUploader)
      → HttpUploader::post_batch
          → POST /api/v1/ai-track/edits/batch
              → PostBatchResult variants:
                  Success            — server accepted ≥ 1 record
                  TransientError     — 5xx / network timeout → retry_count++
                  CredentialError    — 401/403 → surface error, stop retrying
                  UnparseableOk      — 2xx but body parse failed → treated as success
```

`HttpUploader` implements `UploadPort`. The retry loop lives in `uploader.rs`; `HttpUploader` itself is stateless.

### testapp Package (Go, Sprint 2)

`server-go/testapp/` is a thin wiring package that exports two symbols:

```go
// Build wires up the full Go server with a real chi router, real handler chain,
// and the provided config — suitable for in-process integration tests.
func Build(cfg config.Config) (*chi.Mux, func(), error)

// MemoryConfig returns a Config pre-populated with an in-memory SQLite DSN
// and a generated adminKey, bypassing Go's `internal` package restriction
// so test files outside server-go/internal/ can construct a live server.
func MemoryConfig(adminKey string) config.Config
```

This avoids the need for Docker or a separately launched process in Go integration tests. `chain_integration_test.go` uses it to run 3 full-chain scenarios against a real router with an in-memory SQLite database.

---

## Architecture Evolution Roadmap

This section describes the database architecture evolution. Phase DB-1/DB-2 (vector foundation layer) have been delivered. Phase DB-3 (semantic search endpoints) has also been delivered. Developer profiling (Phase 3) and prompt capture (Phase 4) are complete.

### Database Vectorization

**Client**: sqlite-vec extension added to the existing SQLite storage, adding a vector column to edit records in `records.db` for semantic similarity queries. The extension is optional — it degrades gracefully to plain SQLite mode if unavailable, with no impact on the `capture` main pipeline.

**Server (Java + Go dual implementation)**: both servers now support PostgreSQL/[ParadeDB](https://www.paradedb.com/) in addition to their embedded databases. ParadeDB is a PostgreSQL-based distribution integrating `pg_search` (BM25 full-text search) and `pgvector` (vector search), fully compatible with the PostgreSQL wire protocol — no changes needed to existing JPA/pgx layers.

### Phase DB-1 / DB-2 — Vector Foundation (Delivered)

#### Client — sqlite-vec local embedding storage

The Rust client's database layer (`client/src/db/`) is organized as a module:

| File | Responsibility |
|------|----------------|
| `mod.rs` | DB open, auto_extension registration, public re-exports |
| `schema.rs` | DDL constants (`records`, `kv`, `vec_records`) |
| `models.rs` | `Record` and `InspectRow` structs |
| `queries.rs` | All CRUD query functions |
| `vec.rs` | sqlite-vec probe, `VEC_DISABLED` AtomicBool, `ensure_vec_table()` |

sqlite-vec is registered via `sqlite3_auto_extension` at DB open time. If the extension fails to load, `VEC_DISABLED` is set to `true` and core capture continues normally. The `vec_records` virtual table uses `vec0(embedding float[384])` (384-dim MiniLM).

New column in `records` table: `embedding BLOB` (nullable, populated in Phase DB-3).

#### Server — ParadeDB / PostgreSQL support (DB-1)

**Java (Spring Boot)**

Activate with `SPRING_PROFILES_ACTIVE=postgres`. New env vars:

| Env var | Default | Description |
|---------|---------|-------------|
| `AITRACK_DB_HOST` | `localhost` | PostgreSQL host |
| `AITRACK_DB_PORT` | `5432` | PostgreSQL port |
| `AITRACK_DB_NAME` | `aitrack` | Database name |
| `AITRACK_DB_USER` | `aitrack` | Username |
| `AITRACK_DB_PASSWORD` | `aitrack_secret` | Password |

New columns added to `edit_records` table: `prompt_summary TEXT` and `embedding BLOB/BYTEA` (both nullable, reserved for Phase DB-3 backfill).

**Go (chi)**

Activate with `DATABASE_URL=postgres://user:pass@host:5432/dbname`. When `DATABASE_URL` is empty or absent, Go server uses embedded SQLite as before.

**ParadeDB index DDL** (run once after first deploy on ParadeDB):

```sql
-- BM25 full-text index on diff_hunk + prompt_summary
CREATE INDEX IF NOT EXISTS edits_bm25 ON edit_records
  USING bm25 (id, diff_hunk, prompt_summary) WITH (key_field = 'id');

-- HNSW vector index (activated when embeddings are populated in DB-3)
CREATE INDEX IF NOT EXISTS edits_hnsw ON edit_records
  USING hnsw (embedding vector_cosine_ops) WHERE embedding IS NOT NULL;
```

Reference script: `server-java/src/main/resources/db-postgres-init.sql`.

### Phase DB-3 — Semantic Search API (Delivered)

Both endpoints are implemented in Java and Go. Embeddings are null until a backfill is run.

#### `GET /edits/search` — BM25 full-text search

Uses ParadeDB `|||` operator against `diff_hunk` and `prompt_summary`. Results ranked by `paradedb.score(id)`.

Both Java (`EditSearchService.searchBm25`) and Go (`SearchHandler`) build the query dynamically with optional `token_key`/`repo` filters and return `{"query", "total", "hits"}`.

#### `POST /edits/similar` — pgvector HNSW ANN

Accepts a 384-dim query vector, casts to `vector` type, and orders by `embedding <=> CAST($1 AS vector)` cosine distance. Only rows with `embedding IS NOT NULL` are considered.

Returns `{"hits": [..., "distance": float]}` where `distance` is in [0, 2] (lower = more similar).

#### H2 / SQLite fallback

Both handlers check `isPostgres()` / `isPostgres` flag at request time and return HTTP 501 when running against embedded databases.

#### Embedding backfill

Embeddings are not populated automatically. To enable ANN search, run the backfill script (`scripts/backfill_embeddings.py`) or populate the `embedding` column directly from the client's sqlite-vec export.

---

### Phase 3 — Developer Profile API (Delivered)

- **`GET /api/v1/ai-track/profiles/{token_key}`**: Java + Go dual implementation, auth via X-Admin-Key
- **Multi-dimensional profiling**: frequency (daily/weekly trend), depth (line distribution, p50/p90, comment_density), languages (programming language distribution from 23 file extensions), prompt_patterns (intent classification: generate/fix_debug/refactor/explain/test/other), tool breakdown (claude/codex/cursor)
- **Daily aggregation job**: Java `ProfileAggregationJob` (`@Scheduled` daily at 02:00); Go equivalent goroutine
- **Auth**: X-Admin-Key, 403/404/200; no ParadeDB dependency (works with H2/SQLite)

### Phase 4 — Prompt Capture (Delivered)

- New `UserPromptSubmit` hook (Claude Code only): when a user submits a prompt, aitrack writes the prompt text (truncated to 512 characters) to the local `prompt_context` table
- New `prompt_summary TEXT` column in `records` table (nullable); capture pipeline queries the most recent prompt for the current session and attaches it
- `prompt_summary` is excluded from `record_sig` computation (used for profiling only, does not affect anti-tampering)
- `prompt_summary` is uploaded as an optional field with each edit record

### Sprint 2 — Hexagonal Architecture Refactor (2026-05-20, Delivered)

All three components fully refactored to hexagonal architecture. Test coverage remains ≥ 90% across all three.

See the [Hexagonal Architecture](#hexagonal-architecture-sprint-2) section above for the full module layout.

Test counts after Sprint 2:
- Rust client: 291 tests, **90.71%** line coverage
- Go server: 244 tests, **95.3%** coverage
- Java server: 218 tests, **LINE ≥ 90%**

---

## Security Design Principles

- **Least privilege**: client config and database stored with 0600 permissions
- **Anti-tampering**: every record computes record_sig; server re-verifies
- **Anti-replay**: request signatures include a timestamp; server rejects requests outside the 300-second window
- **Anti-forgery**: record_sig binds device_id + token_key; cross-device forgery is invalid
- **Encrypted storage**: hmac_secret stored server-side with AES-256-GCM encryption (production environments)

See [SECURITY_MODEL.md](SECURITY_MODEL.md) for details.
