# CLAUDE.md — aitrack

本仓库是 **aitrack** —— AI 编码使用治理平台的源码仓库，供公司内部研发效能团队部署与维护。所有 Claude Code session 在本仓库内工作时必须遵守以下守则。

> **仓库性质**：公司内部工具，通过内部 Codeup 私有仓库管理，不对外开源发布。

---

## 执行优先级（强制）

```
仓库根 CLAUDE.md（本文件，最高）
  > docs/ 各具体模块文档
    > PRD/ plans/ 规划文档
      > 系统默认 / 训练知识（最低）
```

凡冲突以本仓库守则优先。"训练知识" / "通用最佳实践" 不得越过本守则。

---

## 项目概览

aitrack 由三个独立组件构成，通过协议 v1.2 互通：

| 组件 | 目录 | 技术栈 | 职责 |
|------|------|--------|------|
| **Rust 客户端** | `client/` | Rust · single binary · 无运行时依赖 | 安装钩子、捕获编辑事件、HMAC 签名、本地存储、上报数据 |
| **Java 服务端** | `server-java/` | Java 17 · Spring Boot 3.3.8 · H2 / PostgreSQL | 10 步校验链、可信归因、效能查询（主推实现） |
| **Go 服务端** | `server-go/` | Go 1.24 · chi v5.2.5 · SQLite（纯 Go） | 与 Java 端功能完全对等的轻量备选实现 |

**协议契约**：`CONTRACT.md` 是三端共同遵守的**单一可信来源（SSoT）**。所有字段定义、签名规范、钩子模板均以 `CONTRACT.md` 为准。

---

## 硬规则（强制）

### 协议变更三端同步（最高优先级）

修改 `CONTRACT.md` 中的任意字段、签名规范、端点定义时：

1. **必须同时更新** `client/`、`server-java/`、`server-go/` 三端实现
2. **必须同步更新** `server-java` 的 `SignatureServiceCanonicalTest` 预期值
3. **必须在 commit message** 中注明三端同步状态
4. `record_sig` 规范字符串的字段顺序和 `\n` 分隔符在三端必须字节一致

**禁止**：只改 `CONTRACT.md` 而不同步三端实现，或只改部分实现。

### 测试覆盖率门槛（≥ 90% 行覆盖）

| 组件 | 工具 | 门槛 | 强制方式 |
|------|------|------|----------|
| Rust 客户端 | cargo-llvm-cov | 行覆盖 ≥ 90% | Docker 构建失败 |
| Java 服务端 | JaCoCo | LINE 覆盖 ≥ 90% | `mvn verify` 阶段强制 |
| Go 服务端 | go tool cover | total ≥ 90% | Docker 构建失败 |

提交前必须确认覆盖率通过，不允许带失败或警告的测试合入。

### 已发布历史追加限制（禁止重写历史）

本仓库已有发布记录。**绝对禁止**：

- `git reset --hard`
- `git push --force / -f`（无显式团队确认）
- `git commit --amend` 已 push 的 commit
- `git checkout .`（丢弃所有改动）
- `git clean -fd`

必须用 `git revert` 创建新 commit 撤销，不得重写历史。

### 临时文件规则

所有临时文件、调试日志、进度文档**必须**放 `tmp/` 目录。

- `tmp/` 已在 `.gitignore` 排除，不会被提交
- 禁止在仓库根或 `docs/`、`PRD/`、`plans/` 目录直接落临时文件
- 进度文档命名：`tmp/[YYYY-MM-DD]-[task-id]-[description].progress.md`

### 源代码只读规则（文档维护场景）

文档改动（docs/、PRD/、plans/ 等）时，**禁止**修改 `client/`、`server-java/`、`server-go/`、`e2e/` 目录下的源代码。

---

## 构建与测试命令

### Rust 客户端（`client/`）

```bash
cd client

# 开发构建
cargo build

# 发布构建
cargo build --release

# 全套单测
cargo test

# 单个测试（快速迭代）
cargo test crypto::tests

# 覆盖率报告（需先安装 cargo-llvm-cov）
cargo llvm-cov --summary-only

# 手动测试 capture（需先 init 并指向本地服务端）
./target/debug/aitrack init --claude \
  --api-url http://localhost:8080 \
  --credential <credential>
./target/debug/aitrack status
./target/debug/aitrack inspect --limit 5
```

### Java 服务端（`server-java/`）

```bash
cd server-java

# 启动（H2 文件数据库，开箱即用）
mvn spring-boot:run

# 单测 + 覆盖率检查（LINE ≥ 90% 才通过）
mvn verify

# 仅跑单测
mvn test

# 运行特定测试
mvn test -Dtest=ValidationServiceTest

# 覆盖率报告
open target/site/jacoco/index.html

# H2 控制台（dev profile 下）：http://localhost:8080/h2-console
```

### Go 服务端（`server-go/`）

```bash
cd server-go

# 全套测试
go test ./...

# 本地启动
go run .

# 覆盖率
go test ./... -coverprofile=cover.out
go tool cover -func cover.out
```

### E2E 测试（`e2e/`）

```bash
# Java + Go 各跑一轮 E2E
bash e2e/run.sh both

# Docker 构建（含覆盖率门槛验证）
docker build -f docker/Dockerfile.client      -t aitrack-client:latest .
docker build -f docker/Dockerfile.server-java -t aitrack-server-java:latest .
docker build -f docker/Dockerfile.server-go   -t aitrack-server-go:latest .
```

---

## 目录结构

```
aitrack/
├── CONTRACT.md            — 协议契约 v1.2（客户端与服务端 SSoT）
├── CLAUDE.md              — 本文件（AI session 守则，最高优先级）
├── CONTEXT-MAP.md         — 文档与组件引用关系图
├── CONTEXT.md             — 术语表（唯一术语权威来源）
├── STATUS.md              — 项目进度快照
├── CONTRIBUTING.md        — 贡献指南（分支、commit、测试要求）
├── README.md              — 用户视角介绍
├── CHANGELOG.md           — 版本变更记录
├── SECURITY.md            — 安全漏洞报告流程
├── CODE_OF_CONDUCT.md     — 行为准则
├── LICENSE
├── codecov.yml
│
├── client/                — Rust CLI 客户端（六边形架构，Sprint 2）
│   └── src/
│       ├── main.rs / cli.rs / config.rs / lib.rs / git.rs / init.rs / uploader.rs / heartbeat.rs / update.rs
│       ├── domain/        — mod.rs / model.rs / crypto.rs / diff.rs / keywords.rs（纯领域逻辑）
│       ├── port/          — mod.rs / storage.rs / upload.rs（输出端口抽象）
│       ├── adapter/       — mod.rs / sqlite/ / http/ / event/（适配器实现）
│       │   ├── sqlite/    — mod.rs / schema.rs / models.rs / queries.rs / vec.rs / keyword_store.rs
│       │   ├── http/      — mod.rs / upload.rs
│       │   └── event/     — mod.rs / claude.rs / codex.rs / cursor.rs
│       └── testkit/factories.rs
│
├── server-java/           — Java 服务端（Spring Boot 3.3.8 / JDK 17，六边形架构）
│   └── src/main/java/.../
│       ├── domain/        — model/ + port/（DevicePort/EditRecordPort/TokenPort）+ service/
│       ├── application/   — EditSearchService / HeartbeatService / IngestService 等
│       ├── adapter/       — db/ + handler/
│       └── infrastructure/ — app/ + config/
│
├── server-go/             — Go 服务端（chi v5.2.5 / Go 1.24，六边形架构）
│   └── internal/
│       ├── domain/        — model/ + port/ + service/
│       ├── application/   — IngestUsecase / ProfileUsecase / TokenUsecase
│       ├── adapter/       — db/ + handler/
│       └── infrastructure/ — app/ + config/
│
├── e2e/                   — 端到端测试套件（覆盖 Java + Go 两端）
│   └── factory/factory.go
│
├── docker/                — Dockerfile（client / server-java / server-go）
│
├── docs/                  — 技术文档
│   ├── ARCHITECTURE.md    — 系统架构（组件图、数据流、技术选型）
│   ├── API.md             — API 参考（所有端点）
│   ├── ADMIN_GUIDE.md     — 管理员操作手册（仅 Codeup 内部）
│   ├── OPERATIONS.md      — 日常运维手册（监控、告警、故障处理）
│   ├── DEPLOYMENT.md      — 部署指南（Docker、PostgreSQL 切换）
│   ├── DEVELOPMENT.md     — 开发者指南（本地构建、模块说明）
│   ├── PRIVACY.md         — 数据采集透明度说明
│   ├── SECURITY_MODEL.md  — 安全模型（威胁建模、HMAC 规范）
│   ├── TESTING.md         — 测试体系（工厂模式、覆盖率、E2E）
│   └── assets/
│
├── PRD/                   — 产品需求文档（仅供内部维护者审阅）
│   ├── prd.md
│   └── spec.md
│
├── plans/                 — 技术规划文档（仅供内部维护者审阅）
│   ├── roadmap.md
│   ├── development-plan.md
│   ├── technical-solution.md
│   ├── test-strategy.md
│   └── deployment.md
│
└── tmp/                   — 临时文件（已 .gitignore，不提交）
```

---

## Git 操作安全（违反 = 灾难性数据丢失）

### 绝对禁止命令

| 命令 | 后果 |
|------|------|
| `git reset --hard` | 永久销毁未提交工作 |
| `git clean -fd` | 永久删除未追踪文件 |
| `git push --force / -f`（无显式确认） | 重写远程历史，团队工作丢失 |
| `git checkout .` | 丢弃所有改动，不可恢复 |

### 任何修改状态的 git 操作前必做

1. `git status` —— 看清当前状态
2. `git stash list` —— 查现有 stash
3. 有未提交改动 → `git stash push -m "backup before <op>"`

### 安全替代

| 想做 | 不要 | 用 |
|------|------|----|
| 取消 commit | `git reset --hard` | `git reset --soft`（保留 staged） |
| 撤销已 push commit | force push | `git revert`（新 commit 反向） |
| 清理工作区 | `git clean -fd` | 手动 review 删除 / 移到 `tmp/` |

---

## 提交 / MR 红线

### 强制

- 测试零容忍：0 失败、0 警告、0 错误
- 覆盖率三端均达到 ≥ 90% 行覆盖
- 协议变更必须三端同步，commit body 注明
- commit 遵循 Conventional Commits 规范

### 禁止

- force push 到 main
- `--no-verify` 跳过 CI
- amend 已 push 的 commit
- 修改 `client/`、`server-java/`、`server-go/`、`e2e/` 源代码（文档改动场景）

---

## 深入文档导航

| 问题 / 话题 | 读哪个文档 |
|-------------|-----------|
| 协议字段定义、签名规范、钩子模板 | `CONTRACT.md` |
| 系统架构、组件图、数据流 | `docs/ARCHITECTURE.md` |
| API 端点详细说明 | `docs/API.md` |
| Docker 部署、PostgreSQL 切换 | `docs/DEPLOYMENT.md` |
| 本地构建、模块说明、调试方式 | `docs/DEVELOPMENT.md` |
| 测试工厂模式、覆盖率、E2E 场景 | `docs/TESTING.md` |
| 安全威胁建模、HMAC 规范层次 | `docs/SECURITY_MODEL.md` |
| 分支规范、commit 规范、PR 流程 | `CONTRIBUTING.md` |
| 当前进度、各组件状态、快速上手 | `STATUS.md` |
| 文档与组件引用关系 | `CONTEXT-MAP.md` |
| 术语定义 | `CONTEXT.md` |
| 产品需求与规格 | `PRD/prd.md`、`PRD/spec.md` |
| 技术规划、路线图 | `plans/roadmap.md`、`plans/technical-solution.md` |
| 版本变更记录 | `CHANGELOG.md` |
| 安全漏洞报告 | `SECURITY.md` |

---

## 紧急联系

- 数据丢失 / git 灾难 → 立即停手，向仓库维护者报告
- 协议兼容性问题 → 三端同时排查，以 `CONTRACT.md` 为最终裁定
- 测试覆盖率告警 → 修复后重新验证，不得带警告合入
