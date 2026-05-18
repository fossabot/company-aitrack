# 贡献指南

[English](#english)

## 中文

欢迎为 aitrack 提交改进。

### 基本原则

- 改协议字段（`CONTRACT.md`）必须同时更新三端：Rust 客户端、Java 服务端、Go 服务端
- 新改动必须通过测试并满足覆盖率门槛
- 不提交密钥、token、环境文件或私有路径

### 环境要求

| 组件 | 最低版本 |
|------|---------|
| Rust | stable（推荐最新 stable） |
| JDK | 17 |
| Maven | 3.8+ |
| Go | 1.24+ |
| Docker | 用于端到端集成验证 |

### 构建与测试

**Rust 客户端**

```bash
cd client
cargo build --release
cargo test
# 覆盖率（需先安装 cargo-llvm-cov）
cargo llvm-cov --summary-only
```

**Java 服务端**

```bash
cd server-java
# 单测 + 覆盖率检查（LINE ≥ 90% 才通过）
mvn verify
# 仅跑单测
mvn test
```

**Go 服务端**

```bash
cd server-go
go test ./...
```

### 覆盖率门槛

| 组件 | 工具 | 门槛 |
|------|------|------|
| Rust 客户端 | cargo-llvm-cov | 函数覆盖率 ≥ 90% |
| Java 服务端 | JaCoCo | LINE 覆盖率 ≥ 90%（`mvn verify` 强制） |

提交前必须确保覆盖率门槛通过，不允许带失败或警告的测试合入。

### 仓库结构

```
company-aitrack/
├── CONTRACT.md          — 协议单一可信来源（客户端与服务端共同遵守）
├── client/              — Rust CLI 客户端
│   ├── src/
│   │   ├── main.rs      — 命令分发
│   │   ├── cli.rs       — clap 参数定义
│   │   ├── config.rs    — ~/.aitrack/config.toml 读写
│   │   ├── db.rs        — SQLite records 表 CRUD
│   │   ├── crypto.rs    — HMAC-SHA256、record_sig、request_sig
│   │   ├── diff.rs      — Myers/LCS 差分（similar crate）
│   │   ├── git.rs       — 调用 git 获取仓库元数据
│   │   ├── init.rs      — hook 安装与卸载
│   │   ├── uploader.rs  — 刷新未同步记录到服务端
│   │   ├── heartbeat.rs — 节流心跳 POST
│   │   └── adapters/    — Claude / Codex / Cursor payload 解析
│   └── src/testkit/
│       └── factories.rs — 种子确定性测试工厂
├── server-java/         — Java 服务端（Spring Boot 3 / JDK 17）
│   └── src/test/java/com/aitrack/server/
│       └── testkit/     — Java 测试工厂（EditDtoFactory 等）
└── server-go/           — Go 服务端（Go 1.24+）
```

### 协议契约 CONTRACT.md

`CONTRACT.md` 是客户端与所有服务端实现的**单一可信来源**。

改动协议字段前必须理解以下规则：

1. 任何对上报字段（`edits` 数组字段、`record_sig` 规范字符串、`X-AiTrack-*` 头）的增删改都必须同时更新三端。
2. 字段顺序和 `\n` 分隔符在 `record_sig` 规范字符串中是协议规范，不可随意调整——客户端与服务端必须字节对齐。
3. 协议版本变更建议在 `CONTRACT.md` 顶部写明 `vX.X change` 摘要，便于三端同步时对照。
4. 服务端测试 `SignatureServiceCanonicalTest` 专门验证规范字符串与 `CONTRACT.md` 一致，协议变更后必须同步更新该测试的预期值。

### 各组件开发调试

**Rust 客户端**

```bash
cd client

# 开发构建
cargo build

# 运行单个测试（快速迭代）
cargo test crypto::tests

# 完整测试 + 覆盖率
cargo llvm-cov --summary-only

# 手动测试 capture（需要先 init 并指向本地服务端）
./target/debug/aitrack init --claude \
  --api-url http://localhost:8080 \
  --credential <credential>
./target/debug/aitrack status
./target/debug/aitrack inspect --limit 5
```

本地 SQLite 记录存于 `~/.aitrack/records.db`，可用 `sqlite3` 直接查询调试。

**Java 服务端**

```bash
cd server-java

# 启动（H2 文件数据库，开箱即用）
mvn spring-boot:run

# H2 控制台：http://localhost:8080/h2-console
# JDBC URL: jdbc:h2:file:./data/aitrack  密码为空

# 运行特定测试
mvn test -Dtest=ValidationServiceTest

# 覆盖率报告
mvn verify
open target/site/jacoco/index.html
```

切换 PostgreSQL 只需修改 `application.yml` 的 `spring.datasource` 块，无需改业务代码。

**Go 服务端**

```bash
cd server-go

go test ./...
go run .
```

### 测试工厂模式

aitrack 的三端测试都采用工厂模式，原则是：**所有测试数据通过工厂构建，不在测试体内手写字面量**。

**Rust 客户端（`src/testkit/factories.rs`）**

```rust
// 默认有效记录
let record = EditRecordFactory::new(42).build();

// 定制字段
let record = EditRecordFactory::new(42).with_tool("codex").build();

// 负例
let bad = tampered_record_sig(42);        // sig 被篡改
let bad = tampered_oversized_lines(42);   // 行数超限
let bad = malformed_json();               // 语法错误 JSON
```

**Java 服务端（`server-java/src/test/java/.../testkit/`）**

```java
// 默认有效实例
EditDto dto = EditDtoFactory.build();

// 构造器式覆盖（sig 绑定字段变更后须重算 record_sig）
EditDto dto = EditDtoFactory.with(e -> e.setToolVersion("claude-code-v2"));

// 负例
EditDto bad = TamperedFactory.badRecordSig();       // sig_mismatch
EditDto bad = TamperedFactory.oversizedAddedLines(); // oversized flag
EditDto bad = TamperedFactory.nullTool();            // malformed rejection
```

### E2E 端到端测试

全链路 E2E 涉及：Rust 客户端 → Java 或 Go 服务端 → 数据库存储 → 查询接口。

推荐用 Docker Compose 拉起服务端，再用真实的 `aitrack` 二进制执行 `capture` 并验证数据落库：

```bash
# 1. 构建 Rust 客户端
cd client && cargo build --release

# 2. 启动 Java 服务端（Docker 示例）
cd server-java
docker build -t aitrack-server-java .
docker run -p 8080:8080 aitrack-server-java

# 3. 初始化客户端并发起 capture
./client/target/release/aitrack init \
  --claude \
  --api-url http://localhost:8080 \
  --credential <credential>

# 4. 模拟 Claude hook 触发（stdin 传入 PostToolUse payload）
echo '<claude-posttooluse-json>' | \
  ./client/target/release/aitrack capture --tool claude

# 5. 验证记录已上传
./client/target/release/aitrack inspect --limit 5
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/ai-track/edits
```

E2E 运行中注意 `X-AiTrack-Timestamp` 的 300 秒时间窗口——测试环境时钟不同步会导致 `401`。

### 安全相关注意事项

- `config.toml` 和 `records.db` 权限必须是 0600，调试时不要放宽。
- 测试数据中不要使用真实 credential。
- 公开 Issue / PR 中不要粘贴含有真实凭据的日志或请求体。
- `hmac_secret`（credential 的后半部分）在服务端以 AES-256-GCM 加密存储（`HmacSecretEncryptor`），调试时可查看 `application.yml` 中的加密密钥配置。

### 分支规范

- 主干：`main`
- 功能分支：`feat/<short-description>`
- 修复分支：`fix/<short-description>`
- 协议变更：`contract/<version>-<description>`（需注明涉及哪些端）

### Commit 规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/v1.0.0/)：

```
<type>(<scope>): <subject>

[body]
[footer]
```

常用 type：`feat` / `fix` / `refactor` / `test` / `docs` / `chore`

scope 示例：`client` / `server-java` / `server-go` / `contract` / `testkit`

### Pull Request 规范

- PR 描述说明改动范围和测试结论
- 涉及协议变更时，列出三端同步状态
- 不得绕过 CI 检查强制合并

---

## English

Contributions to aitrack are welcome.

### Principles

- Protocol field changes (`CONTRACT.md`) must be applied to all three components simultaneously: Rust client, Java server, Go server
- All changes must pass tests and meet coverage thresholds
- Do not commit secrets, tokens, environment files, or private filesystem paths

### Requirements

| Component | Minimum version |
|-----------|----------------|
| Rust | stable (latest stable recommended) |
| JDK | 17 |
| Maven | 3.8+ |
| Go | 1.24+ |
| Docker | for end-to-end integration verification |

### Build and test

**Rust client**

```bash
cd client
cargo build --release
cargo test
# Coverage (requires cargo-llvm-cov)
cargo llvm-cov --summary-only
```

**Java server**

```bash
cd server-java
# Unit tests + coverage check (LINE >= 90% required)
mvn verify
# Tests only
mvn test
```

**Go server**

```bash
cd server-go
go test ./...
```

### Coverage thresholds

| Component | Tool | Threshold |
|-----------|------|-----------|
| Rust client | cargo-llvm-cov | function coverage >= 90% |
| Java server | JaCoCo | LINE coverage >= 90% (enforced by `mvn verify`) |

Tests must pass with zero failures or warnings before submitting.

### Repository structure

```
company-aitrack/
├── CONTRACT.md          — single source of truth for the protocol (shared by client and servers)
├── client/              — Rust CLI client
│   ├── src/
│   │   ├── main.rs      — command dispatch
│   │   ├── cli.rs       — clap argument definitions
│   │   ├── config.rs    — ~/.aitrack/config.toml read/write
│   │   ├── db.rs        — SQLite records table CRUD
│   │   ├── crypto.rs    — HMAC-SHA256, record_sig, request_sig
│   │   ├── diff.rs      — Myers/LCS diff (similar crate)
│   │   ├── git.rs       — git metadata retrieval
│   │   ├── init.rs      — hook install and uninstall
│   │   ├── uploader.rs  — flush unsynced records to server
│   │   ├── heartbeat.rs — throttled heartbeat POST
│   │   └── adapters/    — Claude / Codex / Cursor payload parsing
│   └── src/testkit/
│       └── factories.rs — seed-based deterministic test factories
├── server-java/         — Java server (Spring Boot 3 / JDK 17)
│   └── src/test/java/com/aitrack/server/
│       └── testkit/     — Java test factories (EditDtoFactory, etc.)
└── server-go/           — Go server (Go 1.24+)
```

### Protocol contract CONTRACT.md

`CONTRACT.md` is the **single source of truth** for the protocol shared between the client and all server implementations.

Rules to understand before modifying protocol fields:

1. Any addition, removal, or change to reported fields (`edits` array fields, `record_sig` canonical string, `X-AiTrack-*` headers) must be applied to all three components simultaneously.
2. Field order and `\n` separators in the `record_sig` canonical string are part of the protocol spec and must not be changed arbitrarily — the client and servers must be byte-aligned.
3. For protocol version changes, add a `vX.X change` summary at the top of `CONTRACT.md` to aid three-way synchronization.
4. The server test `SignatureServiceCanonicalTest` verifies that the canonical string matches `CONTRACT.md`; after protocol changes, update its expected values accordingly.

### Per-component development and debugging

**Rust client**

```bash
cd client

# Development build
cargo build

# Run a single test (fast iteration)
cargo test crypto::tests

# Full tests + coverage
cargo llvm-cov --summary-only

# Manual capture test (requires init pointing to a local server)
./target/debug/aitrack init --claude \
  --api-url http://localhost:8080 \
  --credential <credential>
./target/debug/aitrack status
./target/debug/aitrack inspect --limit 5
```

Local SQLite records are stored at `~/.aitrack/records.db` and can be queried directly with `sqlite3`.

**Java server**

```bash
cd server-java

# Start (H2 file database, works out of the box)
mvn spring-boot:run

# H2 console: http://localhost:8080/h2-console
# JDBC URL: jdbc:h2:file:./data/aitrack  (no password)

# Run a specific test
mvn test -Dtest=ValidationServiceTest

# Coverage report
mvn verify
open target/site/jacoco/index.html
```

To switch to PostgreSQL, modify the `spring.datasource` block in `application.yml` — no business logic changes needed.

**Go server**

```bash
cd server-go

go test ./...
go run .
```

### Test factory pattern

All three components use the factory pattern for test data: **all test data is constructed through factories — no inline literals inside test bodies**.

**Rust client (`src/testkit/factories.rs`)**

```rust
// Default valid record
let record = EditRecordFactory::new(42).build();

// Custom field
let record = EditRecordFactory::new(42).with_tool("codex").build();

// Negative cases
let bad = tampered_record_sig(42);        // tampered sig
let bad = tampered_oversized_lines(42);   // line count exceeded
let bad = malformed_json();               // syntactically invalid JSON
```

**Java server (`server-java/src/test/java/.../testkit/`)**

```java
// Default valid instance
EditDto dto = EditDtoFactory.build();

// Builder-style override (must recompute record_sig after sig-bound field changes)
EditDto dto = EditDtoFactory.with(e -> e.setToolVersion("claude-code-v2"));

// Negative cases
EditDto bad = TamperedFactory.badRecordSig();       // sig_mismatch
EditDto bad = TamperedFactory.oversizedAddedLines(); // oversized flag
EditDto bad = TamperedFactory.nullTool();            // malformed rejection
```

### End-to-end testing

Full-stack E2E covers: Rust client → Java or Go server → database storage → query API.

The recommended approach is to bring up the server with Docker Compose, then run the real `aitrack` binary through a `capture` flow and verify the record is persisted:

```bash
# 1. Build the Rust client
cd client && cargo build --release

# 2. Start the Java server (Docker example)
cd server-java
docker build -t aitrack-server-java .
docker run -p 8080:8080 aitrack-server-java

# 3. Initialize the client and trigger capture
./client/target/release/aitrack init \
  --claude \
  --api-url http://localhost:8080 \
  --credential <credential>

# 4. Simulate a Claude hook trigger (send PostToolUse payload via stdin)
echo '<claude-posttooluse-json>' | \
  ./client/target/release/aitrack capture --tool claude

# 5. Verify the record was uploaded
./client/target/release/aitrack inspect --limit 5
curl -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/ai-track/edits
```

Note the 300-second window enforced by `X-AiTrack-Timestamp` — clock skew in test environments will cause `401` responses.

### Security notes

- `config.toml` and `records.db` must have permissions 0600; do not relax this during debugging.
- Do not use real credentials in test data.
- Do not paste logs or request bodies containing real credentials in public Issues or PRs.
- The `hmac_secret` (the second half of the credential) is stored server-side encrypted with AES-256-GCM (`HmacSecretEncryptor`); see the encryption key configuration in `application.yml` for debugging.

### Branch naming

- Main: `main`
- Feature: `feat/<short-description>`
- Fix: `fix/<short-description>`
- Protocol change: `contract/<version>-<description>` (note which components are affected)

### Commit convention

Follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

Common types: `feat` / `fix` / `refactor` / `test` / `docs` / `chore`

Scope examples: `client` / `server-java` / `server-go` / `contract` / `testkit`

### Pull request requirements

- Describe the change scope and test results in the PR description
- For protocol changes, list the sync status across all three components
- Do not force-merge by bypassing CI checks
