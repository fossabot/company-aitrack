# 系统架构

## 适用对象

这篇面向希望理解 AiTrack 整体设计的开发者、运维人员和安全审查者。它描述三个组件的职责划分、数据流向、协议版本和技术选型理由。

---

## 组件概览

AiTrack 由三个独立组件构成，通过 HTTP/JSON 协议通信，所有行为准则由 `CONTRACT.md` v1.2 统一约束。

```
┌─────────────────────────────────────────────────┐
│  AI 编码工具（Claude Code / Codex CLI / Cursor）  │
│  PostToolUse / afterFileEdit / UserPromptSubmit 钩子│
└────────────────────┬────────────────────────────┘
                     │ stdin JSON
                     ▼
┌─────────────────────────────────────────────────┐
│  Rust 客户端  aitrack                            │
│  · 适配器解析  · Myers/LCS diff                  │
│  · record_sig  · SQLite 本地存储                  │
│  · flush 上传  · 节流心跳                         │
└────────────────────┬────────────────────────────┘
                     │ POST /api/v1/ai-track/edits
                     │ POST /api/v1/ai-track/heartbeat
                     ▼
┌────────────────────────────┐  ┌────────────────────────────┐
│  Java 服务端               │  │  Go 服务端                  │
│  Spring Boot 3 / H2 / PG  │  │  chi v5 / SQLite（纯 Go）  │
│  10 步校验链               │  │  10 步校验链（完全等价）     │
└────────────────────────────┘  └────────────────────────────┘
```

---

## 客户端：Rust CLI

**职责**：在 AI 工具的编辑事件触发时，捕获、签名、本地存储并上报一条编辑记录。

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
    keywords.rs             — prompt intent classification keywords

  port/                     — output ports (abstract interfaces)
    mod.rs
    storage.rs              — StoragePort (local SQLite read/write)
    upload.rs               — UploadPort (HTTP upload)

  adapter/                  — adapter implementations
    mod.rs
    sqlite/                 — StoragePort SQLite implementation
      mod.rs / schema.rs / models.rs / queries.rs / vec.rs / keyword_store.rs
    http/                   — UploadPort HTTP implementation
      mod.rs / upload.rs
    event/                  — input adapters (hook event parsing)
      mod.rs / claude.rs / codex.rs / cursor.rs

  db/                       — legacy db module (retained for backward compatibility)
  adapters/                 — legacy adapters (retained for backward compatibility)
  testkit/factories.rs      — seed-deterministic test factories
```

### 本地存储

- `~/.aitrack/config.toml`（0600）：api_url、credential、device_id
- `~/.aitrack/records.db`（0600）：SQLite，存储所有捕获记录

`device_id` 在首次运行时生成 UUIDv4 并持久化，后续只读。

---

## 数据流：从钩子触发到入库

### 编辑事件流（PostToolUse / afterFileEdit）

1. AI 工具触发 PostToolUse/afterFileEdit 钩子
2. aitrack capture 从 stdin 读取 JSON
3. 按 --tool 选择适配器（claude/codex/cursor）解析 payload
4. 调用 similar crate 计算 Myers/LCS diff
   → added_lines, removed_lines, diff_hunk
5. spawn git 获取 repo 元数据
   → repo_url, branch, current_sha
6. 获取 OS hostname
7. 从 prompt_context 表查询最近一次提示词 → prompt_summary（可选，仅 claude）
8. 计算 record_sig
   → HMAC_SHA256(hmac_secret, canonical_string)（不包含 prompt_summary）
9. 2 秒去重窗口检查（同 session_id + file_path）
10. INSERT INTO records（synced=0）
11. flush_unsynced → POST /api/v1/ai-track/edits
    → 服务端 10 步校验链
    → 更新 synced/retry_count

### 提示词捕获流（UserPromptSubmit，仅 Claude Code）

1. Claude Code 触发 UserPromptSubmit 钩子
2. aitrack prompt-capture 从 stdin 读取 JSON（{"session_id": "...", "prompt": "..."}）
3. 截断至 512 字符
4. INSERT INTO prompt_context（session_id, prompt_text）

---

## 服务端 10 步校验链

服务端对每批上报数据依次执行：

| 步骤 | 校验内容 | 失败结果 |
|------|----------|----------|
| 1 | Bearer token 有效 | 401 整批拒绝 |
| 2 | X-AiTrack-Timestamp 与服务器时差 ≤ 300 秒 | 401 整批拒绝 |
| 3 | X-AiTrack-Signature HMAC 验证 | 401 整批拒绝 |
| 4 | 每条 record_sig HMAC 验证 | 单条 rejected: sig_mismatch |
| 5 | diff_hunk 行数与 added/removed_lines 一致（±1） | 单条 flagged: diff_inconsistent |
| 6 | repo_url 在白名单内（enforce=true 时） | 单条 flagged/rejected: repo_unknown |
| 7 | file_path 路径合理性校验 | 单条 flagged: path_mismatch |
| 8 | added_lines ≤ max_added_lines（默认 5000） | 单条 flagged: oversized |
| 9 | 速率限制：(token, file_path) 每小时 ≤ 30 条 | 单条 rejected: rate_limited |
| 10 | accepted + flagged 记录写入数据库 | — |

flagged 记录照常入库，标记供管理员审查；rejected 记录不入库，客户端 retry_count+1。

---

## 协议 v1.2 概览

**v1.2 变更**：`POST /admin/tokens` 返回单一 `credential` 字段（`<token>-<hmac_secret>`），替代原先分离的 `token` 与 `hmac_secret`；客户端 `config.toml` 存储键由 `token`/`hmac_secret` 合并为 `credential`，CLI 参数改为 `--credential`。

**v1.1 变更**：在编辑记录和心跳请求中新增 `hostname` 字段，记录上报机器的 OS hostname。

- `hostname` 不是访问控制机制，不做 per-token 隔离
- 同一 token 在多台机器使用是合法场景，`hostname` 用于人工审查时区分来源

请求签名方式（两类）：

```
# 请求级签名（防重放）
X-AiTrack-Signature = HMAC_SHA256(hmac_secret, "{unix_ts}\n{sha256_hex(body_bytes)}")

# 记录级签名（防篡改/防伪造）
record_sig = HMAC_SHA256(hmac_secret, canonical_string)
```

canonical_string 字段顺序严格定义于 `CONTRACT.md`，客户端与服务端两侧必须字节一致。

---

## 技术选型理由

### 为什么用 Rust 写客户端

- 无运行时依赖，单一二进制，便于开发者安装
- 钩子命令需要低延迟（默认 10 秒超时），Rust 启动无 JVM/Node 开销
- `similar` crate 提供经过验证的 Myers/LCS diff 实现，防止行数统计被朴素算法放大

### 为什么服务端有 Java 和 Go 两套实现

两套实现在功能和协议上完全等价（wire-compatible），提供不同的运维选择：

| 维度 | Java（Spring Boot 3.3.8） | Go（chi v5.2.5）|
|------|-----------------------|-------------|
| 数据库 | H2（默认）/ PostgreSQL | SQLite（纯 Go，无 CGO）|
| 部署模型 | JRE + jar，适合现有 JVM 基础设施 | 单一二进制，distroless 镜像，适合极简容器 |
| ORM | Spring Data JPA / Hibernate | 原生 database/sql，无 ORM |
| 适用场景 | 已有 Java 技术栈的团队 | 偏好轻量容器或无 JVM 环境 |

两套实现共享同一个 e2e 测试套件（`e2e/`），以证明协议兼容性。

---

## 架构演进路线

本节描述数据库架构演进路线。Phase DB-1 / DB-2（向量基础层）已交付；Phase DB-3（语义检索端点）仍在规划中，不影响 v1.2 已发布功能。

### 数据库向量化

**客户端**：在现有 SQLite 基础上引入 `sqlite-vec` 扩展，为 `records.db` 中的编辑记录增加向量列，使本地存储具备语义相关查询能力。扩展为可选加载——不可用时自动降级为纯 SQLite 模式，不影响 `capture` 主流程。

**服务端（Java + Go 双实现）**：将服务端数据库从 PostgreSQL 迁移至 [ParadeDB](https://www.paradedb.com/)。ParadeDB 是基于 PostgreSQL 内核的扩展发行版，集成了 `pg_search`（BM25 全文检索）和 `pgvector`（向量检索）扩展，与 PostgreSQL wire protocol 完全兼容，现有 JPA / pgx 层无需重写。

**迁移一致性**：Java 和 Go 两套服务端同步迁移，共享新增向量列的 schema 定义，通过扩展后的 E2E 测试套件验证协议兼容性。

### Phase DB-1 / DB-2 — Vector Foundation (已交付)

**Status**: Implemented in both servers and client. Search endpoints (Phase DB-3) are still planned.

#### Client — sqlite-vec local embedding storage

The Rust client's database layer (`client/src/db/`) has been refactored into a module:

| File | Responsibility |
|------|----------------|
| `mod.rs` | DB open, auto_extension registration, public re-exports |
| `schema.rs` | DDL constants (`records`, `kv`, `vec_records`) |
| `models.rs` | `Record` and `InspectRow` structs |
| `queries.rs` | All CRUD query functions |
| `vec.rs` | sqlite-vec probe, `VEC_DISABLED` AtomicBool, `ensure_vec_table()` |

sqlite-vec is registered via `sqlite3_auto_extension` at DB open time. If the extension fails to load (e.g., older SQLite without loadable extensions), `VEC_DISABLED` is set to `true` and core capture continues normally. The `vec_records` virtual table uses `vec0(embedding float[384])` (384-dim MiniLM).

New column in `records` table: `embedding BLOB` (nullable, populated in Phase DB-3).

#### Server — ParadeDB / PostgreSQL support (DB-1)

Both Java and Go servers now support PostgreSQL/ParadeDB in addition to their embedded databases.

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

#### Docker Compose

`docker/docker-compose.yml` now includes a `db` service (`paradedb/paradedb:latest`) with a `pgdata` named volume and a `pg_isready` healthcheck. The existing Java and Go server services are unchanged (still default to H2/SQLite); opt in to postgres mode via environment variables.

### Phase DB-3 — Semantic Search API (Delivered)

**Status**: Both endpoints implemented in Java and Go. Embeddings are null until a backfill is run.

#### `GET /edits/search` — BM25 full-text search

Uses ParadeDB `|||` operator against `diff_hunk` and `prompt_summary`. Results ranked by `paradedb.score(id)`.

Both Java (`EditSearchService.searchBm25`) and Go (`SearchHandler`) build the query dynamically with optional `token_key`/`repo` filters and return `{"query", "total", "hits"}`.

#### `POST /edits/similar` — pgvector HNSW ANN

Accepts a 384-dim query vector, casts to `vector` type, and orders by `embedding <=> CAST($1 AS vector)` cosine distance. Only rows with `embedding IS NOT NULL` are considered.

Returns `{"hits": [..., "distance": float]}` where `distance` is in [0, 2] (lower = more similar).

#### H2 / SQLite fallback

Both handlers check `isPostgres()` / `isPostgres` flag at request time and return HTTP 501 when running against embedded databases.

#### Embedding backfill

Embeddings are not populated automatically. To enable ANN search, run the backfill script (`scripts/backfill_embeddings.py`, Phase DB-3.8) or populate `embedding` column directly from the client's sqlite-vec export.

---

### 语义检索扩展

在数据库向量化就绪后，规划开放以下两类检索端点：

- **全文检索**：`GET /api/v1/ai-track/edits/search?q=`，基于 pg_search BM25 对编辑内容做相关性排序
- **向量 ANN 检索**：`POST /api/v1/ai-track/edits/similar`，基于 pgvector HNSW 索引检索语义相似的历史编辑记录

这两类端点与现有结构化查询端点并列存在，不破坏现有 API 兼容性，并在 `CONTRACT.md` 中增加对应 schema 定义。

### Phase 3 Delivered (2026 Q4)

- **Developer Profile API**: `GET /api/v1/ai-track/profiles/{token_key}`, Java + Go dual implementation
- **Multi-dimensional profiling**: frequency (daily/weekly trend), depth (line distribution, p50/p90, comment_density), languages (programming language distribution from 23 file extensions), prompt_patterns (intent classification: generate/fix_debug/refactor/explain/test/other), tool breakdown (claude/codex/cursor)
- **Daily aggregation job**: Java `ProfileAggregationJob` (@Scheduled daily 02:00); Go equivalent
- **Auth**: X-Admin-Key, 403/404/200; no ParadeDB dependency (works with H2/SQLite)

### Sprint 2 — Hexagonal Architecture Refactor (2026-05-20, Delivered)

**Status**: All three components fully refactored. Test coverage remains ≥ 90% across all three.

#### Rust Client

Refactored into `domain/` (pure domain logic), `port/` (output port abstractions), and `adapter/` (implementations) — see Module Layout section above. Fixed concurrent race condition via `FLAG_MUTEX: Mutex<()>` in `db/vec.rs`. 291 tests pass.

#### Go Server

Added `domain/port/` (`EditRecordPort`, `HeartbeatPort`, `TokenPort` interfaces), `domain/service/` (`ValidationPolicy` value object and services), `application/` (`IngestUsecase`, `ProfileUsecase`, `TokenUsecase`), `adapter/` (`db/`, `handler/`), and `infrastructure/` (`app/`, `config/`) layers. 244 tests pass, 92.1% coverage.

#### Java Server

Added `domain/port/` (`DevicePort`, `EditRecordPort`, `TokenPort` interfaces). Extracted `ValidationPolicy.java` as a pure domain value object (Spring dependency removed). 218 tests pass, LINE ≥ 90%.

#### E2E

`e2e/mock_chain_test.go`: 3 new Phase 4 mock chain tests (no real credentials required).

---

### Phase 4 — Prompt Capture (2026-05-19)

**Status**: Implemented in client, Java server, and Go server.

#### 提示词捕获

- 新增 `UserPromptSubmit` 钩子（仅 Claude Code）：claude Code 用户提交 prompt 时触发，aitrack 将 prompt 文本（截断至 512 字符）写入本地 `prompt_context` 表
- `records` 表新增 `prompt_summary TEXT` 列（可空），capture 流程从 `prompt_context` 查询当前 session 的最近一条提示词并附加
- `prompt_summary` 不参与 `record_sig` 计算（仅用于画像分析，不影响防篡改机制）
- 上传时 `prompt_summary` 作为可选字段随 edit 记录上报

#### 画像维度更新

| 维度 | 原实现 | 新实现 |
|------|-------|-------|
| 编程语言 | 无 | `languages`: 按文件扩展名统计（23 种语言） |
| 注释密度 | 无 | `depth.comment_density`: diff_hunk 新增行中注释行比例 |
| 提示词意图 | 无 | `prompt_patterns`: 关键词分类（generate/fix_debug/refactor/explain/test/other） |

---

## 安全设计原则

- **最小权限**：客户端配置和数据库均以 0600 权限存储
- **防篡改**：每条记录计算 record_sig，服务端重新验证
- **防重放**：请求签名包含时间戳，服务端拒绝 300 秒以外的请求
- **防伪造**：record_sig 绑定 device_id + token_key，跨设备伪造无效
- **加密存储**：hmac_secret 在服务端以 AES-256-GCM 加密存储（生产环境）

详见 [SECURITY_MODEL.md](SECURITY_MODEL.md)。
