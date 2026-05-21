# 系统架构 / System Architecture

## 适用读者 / Who This Is For

本文档面向希望了解 AiTrack 整体设计的开发者、运维人员和安全审查人员。文档描述各组件的职责、数据流、协议版本演进以及技术选型。

---

## 组件概览 / Component Overview

AiTrack 由三个独立组件构成，通过 HTTP/JSON 协议互通。所有行为规范由 `CONTRACT.md` v1.2 约定。

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
│  Spring Boot 3 / H2 / PG  │  │  chi v5 / SQLite / PG      │
│  10-step validation chain  │  │  10-step validation chain   │
│                            │  │  (fully equivalent)         │
└────────────────────────────┘  └────────────────────────────┘
```

---

## 客户端：Rust CLI / Client: Rust CLI

**职责**：当 AI 工具触发编辑事件时，捕获、签名、本地存储并上报一条编辑记录。

### 模块布局（六边形架构 / Hexagonal Architecture，Sprint 2）

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

### 本地存储 / Local Storage

- `~/.aitrack/config.toml`（权限 0600）：api_url、credential、device_id
- `~/.aitrack/records.db`（权限 0600）：SQLite 数据库，存储所有已捕获的记录

`device_id` 首次运行时生成为 UUIDv4 并持久化，之后只读。

---

## 数据流：从钩子触发到写入数据库 / Data Flow: From Hook Trigger to Database

### 编辑事件流（PostToolUse / afterFileEdit）

1. AI 工具触发 PostToolUse/afterFileEdit 钩子
2. `aitrack capture` 从 stdin 读取 JSON
3. 按 `--tool` 参数（claude/codex/cursor）选择适配器并解析 payload
4. 调用 `similar` crate 计算 Myers/LCS diff
   → added_lines、removed_lines、diff_hunk
5. 调用 git 获取仓库元数据
   → repo_url、branch、current_sha
6. 获取操作系统 hostname
7. 查询 `prompt_context` 表，取当前会话最近一条 prompt → prompt_summary（可选，仅 Claude）
8. 计算 record_sig
   → HMAC_SHA256(hmac_secret, canonical_string)（prompt_summary 不计入签名）
9. 2 秒去重窗口检查（相同 session_id + file_path）
10. INSERT INTO records（synced=0）
11. flush_unsynced → POST /api/v1/ai-track/edits
    → 服务端 10 步校验链
    → 更新 synced/retry_count

### Prompt 捕获流（UserPromptSubmit，仅 Claude Code）

1. Claude Code 触发 UserPromptSubmit 钩子
2. `aitrack prompt-capture` 从 stdin 读取 JSON（`{"session_id": "...", "prompt": "..."}`）
3. 截断至 512 字符
4. INSERT INTO prompt_context（session_id, prompt_text）

---

## 服务端 10 步校验链 / Server 10-Step Validation Chain

服务端对每批上传数据依次执行以下校验：

| 步骤 | 校验项 | 失败结果 |
|------|--------|---------|
| 1 | Bearer token 有效 | 401，拒绝整批 |
| 2 | X-AiTrack-Timestamp 与服务器时间偏差 ±300 秒以内 | 401，拒绝整批 |
| 3 | X-AiTrack-Signature HMAC 验证 | 401，拒绝整批 |
| 4 | 每条记录的 record_sig HMAC 验证 | 单条：`rejected: sig_mismatch` |
| 5 | diff_hunk 行数与 added/removed_lines 一致（±1） | 单条：`flagged: diff_inconsistent` |
| 6 | repo_url 在白名单中（enforce=true 时） | 单条：`flagged/rejected: repo_unknown` |
| 7 | file_path 合理性检查 | 单条：`flagged: path_mismatch` |
| 8 | added_lines ≤ max_added_lines（默认 5000） | 单条：`flagged: oversized` |
| 9 | 限流：(token, file_path) 每小时 ≤ 30 条 | 单条：`rejected: rate_limited` |
| 10 | 将已接受和已标记的记录写入数据库 | — |

已标记（flagged）的记录写入数据库并等待管理员审核；被拒绝（rejected）的记录不写入，客户端 retry_count 自增。

---

## 协议 v1.2 概览 / Protocol v1.2 Overview

**v1.2 变更**：`POST /admin/tokens` 现在返回单一 `credential` 字段（`<token>-<hmac_secret>` 合并字符串），不再分别返回 `token` 和 `hmac_secret`。客户端 `config.toml` 中的配置项由 `token`/`hmac_secret` 合并为 `credential`，CLI 参数改为 `--credential`。

**v1.1 变更**：编辑记录和心跳请求中新增 `hostname` 字段，允许在同一 token 跨多台机器使用时按机器归因。

- `hostname` 不是访问控制机制，不强制执行 per-token 隔离
- 同一 token 在多台机器使用是合法场景，`hostname` 用于人工审查时区分来源

请求签名（两类）：

```
# 请求级签名（防重放）
X-AiTrack-Signature = HMAC_SHA256(hmac_secret, "{unix_ts}\n{sha256_hex(body_bytes)}")

# 记录级签名（防篡改 / 防伪造）
record_sig = HMAC_SHA256(hmac_secret, canonical_string)
```

canonical_string 的字段顺序在 `CONTRACT.md` 中严格定义。客户端与服务端必须字节完全一致。

---

## 技术选型 / Technology Choices

### 为何选用 Rust 作为客户端

- 无运行时依赖，单一二进制，开发者安装便捷
- 钩子命令要求低延迟（默认 10 秒超时），Rust 启动无 JVM/Node 开销
- `similar` crate 提供经过充分验证的 Myers/LCS diff 实现，避免朴素行数统计导致的虚高

### 为何提供两套服务端实现（Java 和 Go）

两套实现在功能和协议上完全等价（线协议兼容），提供不同的运维选项：

| 维度 | Java（Spring Boot 3.3.8） | Go（chi v5.2.5） |
|------|--------------------------|-----------------|
| 数据库 | H2（默认）/ PostgreSQL | SQLite（纯 Go，无 CGO） |
| 部署 | JRE + jar，适合已有 JVM 基础设施 | 单一二进制，distroless 镜像，适合最小化容器 |
| ORM | Spring Data JPA / Hibernate | 原生 database/sql，无 ORM |
| 适合场景 | 已有 Java 技术栈的团队 | 轻量容器或无 JVM 环境 |

两套实现共享同一套 E2E 测试（`e2e/`），以验证协议兼容性。

---

## 六边形架构（Sprint 2）/ Hexagonal Architecture (Sprint 2)

三个组件均遵循相同的六边形架构（端口与适配器模式）：

**Rust 客户端**
```
domain/     — pure domain logic (model.rs, crypto.rs, diff.rs, keywords.rs)
port/       — output port abstractions (storage.rs → StoragePort, upload.rs → UploadPort)
adapter/    — implementations (sqlite/, http/, event/)
```

**Go 服务端**
```
domain/model/    — EditRecord, HeartbeatRecord, Token value objects
domain/port/     — EditRecordPort, HeartbeatPort, TokenPort interfaces
domain/service/  — ValidationPolicy value object and domain services
application/     — IngestUsecase, ProfileUsecase, TokenUsecase
adapter/         — db/ (PostgreSQL impl + SQLite for E2E tests), handler/ (HTTP handlers)
infrastructure/  — app/ (wiring), config/ (env/flags)
```

**Java 服务端** — 结构与 Go 对应，基于 Spring Boot：
```
domain/model/    — JPA entities and value objects; PageResult<T> replaces Spring Page<T>
domain/port/     — DevicePort, EditRecordPort, TokenPort interfaces
domain/service/  — ValidationPolicy.java (pure, no Spring dependency)
application/     — EditSearchService, HeartbeatService, IngestService, …
adapter/         — db/ (JPA repos), handler/ (Spring MVC controllers)
infrastructure/  — app/ (Spring Boot entry), config/ (profiles)
```

> `PageResult<T>` 是一个普通 Java 泛型类，与 Spring 的 `Page<T>` 形状一致但不引入 `spring-data-commons`，保持领域层框架无关。

### HttpUploader 数据流 / HttpUploader Data Flow

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

`HttpUploader` 实现 `UploadPort`。重试循环位于 `uploader.rs`；`HttpUploader` 本身是无状态的。

### testapp 包（Go，Sprint 2）/ testapp Package (Go, Sprint 2)

`server-go/testapp/` 是一个轻量级装配包，导出两个符号：

```go
// Build wires up the full Go server with a real chi router, real handler chain,
// and the provided config — suitable for in-process integration tests.
func Build(cfg config.Config) (*chi.Mux, func(), error)

// MemoryConfig returns a Config pre-populated with an in-memory SQLite DSN
// and a generated adminKey, bypassing Go's `internal` package restriction
// so test files outside server-go/internal/ can construct a live server.
func MemoryConfig(adminKey string) config.Config
```

这样 Go 集成测试无需 Docker 或独立启动的服务进程。`chain_integration_test.go` 使用它针对真实 router 和内存 SQLite 数据库运行 3 个完整链路测试场景。

---

## 架构演进路线 / Architecture Evolution Roadmap

本节描述已交付的功能阶段。开发者画像（Phase 3）和 Prompt 捕获（Phase 4）均已完成。

### Phase 3 — 开发者画像 API（已交付）/ Developer Profile API (Delivered)

- **`GET /api/v1/ai-track/profiles/{token_key}`**：Java + Go 双实现，通过 X-Admin-Key 鉴权
- **多维度画像**：频率（日/周趋势）、深度（行数分布、p50/p90、注释密度）、语言（来自 23 种文件扩展名的编程语言分布）、prompt_patterns（意图分类：generate/fix_debug/refactor/explain/test/other）、工具分布（claude/codex/cursor）
- **每日聚合任务**：Java `ProfileAggregationJob`（`@Scheduled` 每日 02:00 执行）；Go 侧等价 goroutine
- **鉴权**：X-Admin-Key，403/404/200；H2/SQLite 下可用

### Phase 4 — Prompt 捕获（已交付）/ Prompt Capture (Delivered)

- 新增 `UserPromptSubmit` 钩子（仅 Claude Code）：用户提交 prompt 时，aitrack 将 prompt 文本（截断至 512 字符）写入本地 `prompt_context` 表
- `records` 表新增 `prompt_summary TEXT` 列（可为 null）；捕获流程查询当前会话最近一条 prompt 并附加
- `prompt_summary` 不计入 `record_sig` 计算（仅用于画像分析，不影响防篡改机制）
- `prompt_summary` 作为可选字段随每条编辑记录一并上报

### Sprint 2 — 六边形架构重构（2026-05-20，已交付）/ Hexagonal Architecture Refactor (Delivered)

三个组件全部重构为六边形架构。三端测试覆盖率均维持在 ≥ 90%。

完整模块布局见上方[六边形架构](#六边形架构sprint-2--hexagonal-architecture-sprint-2)章节。

Sprint 2 后的测试数量：
- Rust 客户端：291 个测试，**90.71%** 行覆盖
- Go 服务端：244 个测试，**95.3%** 覆盖率
- Java 服务端：218 个测试，**行覆盖 ≥ 90%**

---

## 安全设计原则 / Security Design Principles

- **最小权限（Least privilege）**：客户端配置和数据库以 0600 权限存储
- **防篡改（Anti-tampering）**：每条记录计算 record_sig，服务端重新验证
- **防重放（Anti-replay）**：请求签名包含时间戳，服务端拒绝 300 秒窗口外的请求
- **防伪造（Anti-forgery）**：record_sig 绑定 device_id + token_key，跨设备伪造无效
- **加密存储（Encrypted storage）**：hmac_secret 在服务端使用 AES-256-GCM 加密存储（生产环境）

详见 [SECURITY_MODEL.md](SECURITY_MODEL.md)。
