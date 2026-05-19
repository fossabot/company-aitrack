# 更新日志

本文档遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.0.0/) 格式。

---

## [v1.6.0] — 2026-05-20

### Changed

- **Sprint 2: Hexagonal architecture refactor across all three components**
  - Rust client: `client/src/` restructured into `domain/` (pure domain logic: model, crypto, diff, keywords), `port/` (StoragePort / UploadPort abstractions), and `adapter/` (sqlite/ + http/ + event/ implementations); legacy `adapters/` and `db/` modules retained for backward compatibility; fixed concurrent race condition via `FLAG_MUTEX: Mutex<()>` in `db/vec.rs`; 291 tests pass, ≥ 90% line coverage
  - Go server: added `domain/port/` (EditRecordPort / HeartbeatPort / TokenPort), `application/` (IngestUsecase / ProfileUsecase / TokenUsecase), `adapter/` (db/ + handler/), and `infrastructure/` (app/ + config/) layers; added `ValidationPolicy` value object; 244 tests pass, 92.1% coverage
  - Java server: added `domain/port/` (DevicePort / EditRecordPort / TokenPort); extracted `ValidationPolicy.java` as a pure domain value object (Spring dependency removed); 218 tests pass, LINE ≥ 90%
  - E2E: `e2e/mock_chain_test.go` — 3 new Phase 4 mock chain tests (no real credentials required)

---

## [v1.5.0] — 2026-05-20

### Added

- **Phase 4: Prompt Capture Pipeline**
  - Client: installs `UserPromptSubmit` hook (Claude Code only) alongside `PostToolUse`; new `prompt-capture` subcommand stores user prompt text (≤512 chars) in local `prompt_context` SQLite table
  - Client: `capture` flow attaches most recent session prompt as optional `prompt_summary` on each edit record
  - DB: new `prompt_context` table (session_id, prompt_text, created_at); `prompt_summary TEXT` column added to `records` via migration
  - Profile API: `prompt_patterns` dimension — keyword intent classification (generate/fix_debug/refactor/explain/test/other) from `prompt_summary` text
  - Profile dimensions redesigned: `scenarios` → `languages` (file-extension based, 23 types) + `depth.comment_density` (ratio of comment lines in diff_hunk added lines)
  - CONTRACT.md updated: `prompt-capture` command, `UserPromptSubmit` hook template, optional `prompt_summary` field, `prompt_patterns`/`languages`/`comment_density` in profile schema
  - Rust 200 tests pass, Java 215 tests pass, Go all packages pass

---

## [v1.4.0] — 2026-05-19

### Added

- **Phase 3: Developer AI Usage Profiles**
  - Java `ProfileController`: `GET /api/v1/ai-track/profiles/{token_key}`, X-Admin-Key auth
  - Java `ProfileService`: on-demand three-dimensional profile (frequency/depth/scenarios/tools), `classifyScenario()` path heuristic
  - Java `ProfileAggregationJob`: `@Scheduled(cron="0 0 2 * * *")` daily warm-up
  - Go `ProfileHandler`: feature-equivalent to Java, same JSON schema
  - `EditRecordRepository.findByTokenKeyAndStatusNot()`, `TokenRepository.findByTokenKeyAndActiveTrue()`
  - `@EnableScheduling` added to `AiTrackServerApplication`
  - 206 Java tests pass; Go total coverage 92.4%

### Docs

- `docs/PRIVACY.md` (both repos): data collection transparency document
- `CONTRACT.md` §5: Phase 3 profile endpoint schema

---

## v1.3.0 — 2026-05-19

### 发布说明

v1.3.0 完成三个 DB 阶段：DB-1 接入 ParadeDB/PostgreSQL 服务端、DB-2 客户端 sqlite-vec 向量扩展、DB-3 语义搜索 API。

### 新增

**Phase DB-1 — ParadeDB / PostgreSQL 服务端支持**
- Java 服务端：`postgres` Spring Profile，通过 `SPRING_PROFILES_ACTIVE=postgres` 激活
- Go 服务端：`DATABASE_URL` 环境变量切换至 PostgreSQL；未设置时回退到嵌入式 SQLite
- `edit_records` 表：新增可空列 `embedding BYTEA/BLOB` 和 `prompt_summary TEXT`
- docker-compose：新增 `paradedb/paradedb:latest` 服务，含 `pg_isready` 健康检查

**Phase DB-2 — 客户端 sqlite-vec 向量扩展**
- 重构 `client/src/db.rs` → `client/src/db/` 模块（mod / schema / models / queries / vec）
- sqlite-vec 通过 `sqlite3_auto_extension` 注册；`VEC_DISABLED` 标志用于优雅降级
- `records` 表：新增可空列 `embedding BLOB`
- 新增 `vec_records` 虚拟表（`vec0(embedding float[384])`，384 维 MiniLM 空间）

**Phase DB-3 — 语义搜索 API**
- `GET /api/v1/ai-track/edits/search?q=`：ParadeDB BM25 全文检索（`|||` 运算符）
- `POST /api/v1/ai-track/edits/similar`：pgvector HNSW ANN 近似相似度（384 维余弦距离）
- 两个端点均支持可选 `token_key`/`repo` 过滤；H2/SQLite 模式下返回 HTTP 501
- Java `EditSearchController` + `EditSearchService`；Go `SearchHandler` + `SimilarHandler`
- `CONTRACT.md` 新增两个端点的完整请求/响应 schema

### 工具链

- Go 1.24 → **1.25**（pgx v5.9.x 要求）
- JaCoCo **0.8.11 → 0.8.13**（Java 25 字节码支持）
- `pgx/v5` **5.7.2 → 5.9.2**（修复 1 个 Critical + 1 个 Low CVE）
- `golang.org/x/crypto` 升级（修复 1 个 High + 2 个 Medium CVE）

### 覆盖率

| 组件 | 测试数 | 行覆盖率 |
|------|--------|---------|
| Rust 客户端 | 196 | 91.79% |
| Java 服务端 | 186 | 95% |
| Go 服务端 | 70 | 93.2% |

---

## v1.2.0 — 2026-05-18

### 发布说明

v1.2.0 是协议 v1.2 对应的正式版本。核心变更是将 `token` 与 `hmac_secret` 合并为单个 **credential** 字符串（`<token>-<hmac_secret>`），简化了签发与分发流程。同步完成了一批安全加固，覆盖服务端请求体限制、批量上限、HMAC 常量时间比对、H2 控制台禁用，以及运行时版本升级。

### 新增

- **协议 v1.2 合并凭据（credential）**：`POST /admin/tokens` 响应字段由 `token` + `hmac_secret` 合并为单一 `credential` 字段（格式：`<token>-<hmac_secret>`）；客户端 `config.toml` 存储键由 `token`/`hmac_secret` 改为 `credential`；CLI 参数 `--credential` 接收合并字符串。
- 客户端 `init.rs`：`config.toml` 和 `records.db` 改为原子创建，先写临时文件再原子 rename，避免写入中断留下损坏文件。
- 客户端 `capture`：stdin 读取增加上限（防止超大 payload 阻塞进程）。

### 变更

- `CONTRACT.md` 升版至 v1.2，新增 `v1.2 change` 说明段落及 `Credential` 章节，明确 credential 拆分规则（按第一个 `-` 拆分）。
- Java 服务端升级至 Spring Boot **3.3.8**。
- Go 服务端依赖升级：chi **v5.2.5**；Go 工具链要求 **1.24**。
- 服务端请求体上限统一设为 **8 MiB**（Java `spring.servlet.multipart.max-request-size` / Go 中间件 `http.MaxBytesReader`）。
- 服务端单次上报 `edits` 数组上限设为 **500 条**，超出返回 400。
- 服务端 HMAC 比对全部改为**常量时间比较**（Java `MessageDigest.isEqual`，Go `subtle.ConstantTimeCompare`），消除 timing attack 面。
- Java 服务端 H2 Web 控制台在生产 Profile 下**强制禁用**（`spring.h2.console.enabled=false`）。

### 加固点

本版本加固点覆盖 H1–H8（含本版新增的服务端加固项）：

| 编号 | 说明 |
|------|------|
| H1 | record_sig HMAC — 防本地 DB 篡改 |
| H2 | record_sig 绑定 device_id+token — 防跨设备伪造 |
| H3 | 心跳 hook 状态上报 — 检测静默卸载 |
| H4 | Myers/LCS 真差分 — 防行数膨胀 |
| H5 | 速率限制 (token, file_path) 每小时 ≤ 30 — 防刷量 |
| H6 | 适配器解析失败记录日志 — 不静默吞错 |
| H7 | repo_url 白名单（enforce=true 时强制拒绝）— 防 repo 伪造 |
| H8 | file_path 合理性校验（无 `..`）— 防路径注入 |

---

## v1.1.0 — 2026-05-17

### 发布说明

v1.1.0 是协议 v1.1 对应的正式版本，核心变更是在上报记录和心跳中引入 `hostname` 字段，使同一 token 在多台机器上的编辑活动可被逐机器追溯。

### 新增

- `hostname` 字段加入 `records` 表 schema 及上报 JSON 结构（`CONTRACT.md` §Upload Request）。
- `record_sig` HMAC 计算绑定 `hostname`，防止跨设备伪造（hardening point H1/H2）。
- 心跳请求中携带 `hostname`，服务端 `devices` 接口可见每台机器的心跳状态。
- Rust 客户端 `capture` 流程第 6 步：从 OS 读取 hostname 并写入本地 SQLite 和上报体。
- Java 服务端 `ValidationService` 新增 hostname 存储与设备去重逻辑。
- Go 服务端同步支持 hostname 字段解析和存储。

### 变更

- `CONTRACT.md` 升版至 v1.1，新增 `v1.1 change` 说明段落，字段顺序文档更新。
- 服务端 `record_sig` 校验规范字符串加入 `hostname` 行（`CONTRACT.md` §Record Signature）。
- `HmacSecretEncryptorTest`、`SignatureServiceCanonicalTest` 同步更新预期值。

### 技术说明

`hostname` 是透明可见性机制，不作为访问控制手段；同一 token 多机使用属正常场景，管理员可通过 `/api/v1/ai-track/devices` 逐机审查。

---

## v1.0.0 — 2026-05-01

### 发布说明

v1.0.0 是 aitrack 的初始正式版本，建立了 Rust 客户端 + Java 服务端的双组件架构，以及完整的加固校验链（hardening points H1–H6）。

### 新增

- Rust CLI 客户端，支持 `init / remove / capture / inspect / stats / status / clean / heartbeat` 全套命令。
- 支持 Claude Code、Codex CLI、Cursor 三种 AI 编码工具的 hook 安装与卸载，操作幂等。
- 本地 SQLite 存储（`~/.aitrack/records.db`，权限 0600），`config.toml`（权限 0600）持久化配置与 `device_id`。
- Myers/LCS 真差分算法（`similar` crate），防止朴素行数统计被刷高（hardening point H4）。
- `record_sig` HMAC-SHA256 签名，绑定 `token_key + device_id + timestamp + tool + file_path + repo_url + current_sha + added_lines + removed_lines + sha256(diff_hunk)`，防止本地记录篡改（hardening point H1/H2）。
- 请求级 HMAC 签名（`X-AiTrack-Signature`），防止重放攻击，时间窗口 300 秒（hardening point H2）。
- 心跳机制：每次 `capture` 结束后节流发送，1 小时内最多一次；`aitrack heartbeat` 可强制发送（hardening point H3）。
- 适配器解析失败写本地日志，不静默吞错（hardening point H6）。
- Java 服务端（Spring Boot 3 / JDK 17 / H2 或 PostgreSQL），10 步校验链，覆盖签名、重放、差分一致性、仓库白名单、路径合理性、行数上限、速率限制。
- `AES-256-GCM` 加密存储 `hmac_secret`（`HmacSecretEncryptor`）。
- 服务端 testkit 工厂模式（`EditDtoFactory`、`TamperedFactory` 等），JaCoCo 覆盖率门槛 ≥ 90%。
- Rust 客户端测试覆盖率：行 87.75%，函数 90.24%，含 `testkit/factories.rs` 种子确定性构建器。
- `CONTRACT.md` 作为客户端与服务端的协议单一可信来源（Single Source of Truth）。

### 技术说明

初始版本以单机自托管为主要场景；H2 内存/文件数据库开箱即用，生产环境可切换 PostgreSQL（`application.yml` 配置切换，无需修改业务代码）。
