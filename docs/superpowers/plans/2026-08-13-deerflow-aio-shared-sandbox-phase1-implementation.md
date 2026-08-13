# DeerFlow/AIO Shared Sandbox Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development`
> (recommended) or `executing-plans` to implement this plan task-by-task. Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变现有 Plugin one-shot 执行链的前提下，为 NewX 增加可灰度的
`sandbox_session_v1` Core Session 后端：一个 Runner 部署只运行一个常驻 AIO
容器，按 Thread 分配稳定 UID/GID、持久工作区和独立 Shell Session，并在最低
`2C4G` 主机上以全局权重 `2` 安全运行。

**Architecture:** NewX 控制面继续选择 Provider、签发身份和保存业务状态；Native
Runner 复用现有 Redis 持久公平队列和全局权重槽位，新增独立 Session 队列命名空间、
MySQL Session 元数据以及 AIO generation fencing。生产 Core 操作通过派生 AIO 镜像内
的 NewX `sessiond` 执行；上游 AIO Go SDK 用于锁定版本的能力探针，但不能直接承担
租户隔离，因为已核对的公开 Shell/File 请求没有 UID/GID 字段。本阶段只发布后端能力
和管理配置，不把 Agent、Subagent 或 Plugin 业务流量切到 Session；第二、三阶段另写
实施计划。

**Tech Stack:** Go 1.24、Hertz/net/http、GORM/MySQL、Redis、
`github.com/agent-infra/sandbox-sdk-go` v0.0.5、AIO Sandbox 1.11.0、Linux
UID/GID/进程组、React 18、TypeScript、Vitest、Atlas Community 1.2.3、Docker
Compose、GitHub Actions。

**Execution prerequisite:** 代码实施前必须调用 `test-driven-development`；每个任务通过
后按提交点调用 `verification-before-completion`。Task 12/13 页面和真实本地验收还必须
调用 `browser:control-in-app-browser`。准备合入时调用 `requesting-code-review` 和
`finishing-a-development-branch`。本计划已经在用户指定的当前修复分支编写，不另开
worktree；若实施者另行执行，则先按仓库规则使用 `using-git-worktrees` 建立隔离环境，
且不得带走本工作区的用户未提交改动。

**Approved design:**
`docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`

---

## 实施边界

### 本计划交付

- 锁定并真实探测 AIO `1.11.0` 与 Go SDK `v0.0.5`；
- 新增 `sandbox_session_v1`、Session 身份 v2 和 Core Session Go 合同；
- 新增 `sandbox_runtime_identities`、`sandbox_runtime_sessions` 两张 Phase 1 表，
  `sandbox_runtime_service_leases` 留到 Phase 3；
- 为每个 `deployment + provider + space + user + thread` 分配稳定身份，并保证同一 AIO
  deployment 内 UID/GID 唯一和 Thread 工作区 `0700`；
- 在一个常驻 AIO 容器中运行 NewX `sessiond`，提供显式 Shell Session、安全文件
  操作、取消、清理和 generation 恢复；
- 让 Session Core 操作和现有 one-shot 任务共享总权重 `2`，默认 Core 权重 `1`、
  单用户 active 上限 `1`；
- 增加只在 `APP_ENV=debug`、显式开关和 loopback 下可用的 Host Shell Session；
- 增加系统管理配置、Runner 状态、Compose、镜像构建、发布与回滚合同；
- 用真实容器完成两 Session 并发、跨 Thread 隔离、重启恢复和 `2C4G` 验收。

### 本计划明确不交付

- 不修改 `backend/application/agentthread/**`，不接入 Agent/Subagent、Skill、上传、
  Artifact 或对象存储；
- 不把 Plugin、MCP stdio 或 AppDev 从现有 one-shot Adapter 切到 Session；
- 不开放 Browser、Jupyter、VSCode、Terminal、MCP、VNC、CDP 或预览端口；
- 不创建 `sandbox_runtime_service_leases`；
- 不把 AIO `8080`、NewX `sessiond` 或任何交互端口发布到宿主机；
- 不以 AIO 失败为理由回退 Host Shell；
- 不修改或提交工作区中用户已有的
  `docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md`。

### 单一所有者规则

- `deploy/dev/docker-compose.runner-2c4g.yml` 和 `deploy/dev/deploy.sh` 是 AIO 容器、
  私网、镜像、资源硬限制与持久 volume 的唯一生命周期所有者；
- Runner 不通过 Docker socket 创建、删除或重启 AIO，只监督 AIO/sessiond 健康、校验
  boot fingerprint、管理 generation、Session、队列和 grant；
- Runner 现有 rootless Docker socket 继续只服务 one-shot Adapter，不赋予 AIO 管理权；
- AIO restart policy（`unless-stopped`）恢复容器后，Runner 依据新 boot fingerprint 做
  fencing，不能出现
  Compose 和 Runner 同时争抢同名容器的双控制面。

## 已锁定的上游事实

- AIO 镜像引用固定为
  `ghcr.io/agent-infra/sandbox:1.11.0@sha256:6328d7fd2f0ff0b4c147c3d05b3df1ce331f4a482eb6e550ecd64ed1fcf906e7`；
  该索引同时包含 `linux/amd64` 和 `linux/arm64`，禁止改用 `latest`。
- AIO `1.11.0` 入口是 `/opt/gem/run.sh`，公开端口是 `8080`，默认运行用户语义为
  `gem:1000`，并支持 `JWT_PUBLIC_KEY`；派生镜像必须保留该入口的信号和退出语义。
- Go SDK 固定为 `github.com/agent-infra/sandbox-sdk-go v0.0.5`，客户端必须注入有
  deadline 的 `http.Client` 并设置 `WithMaxAttempts(1)`，禁止对有副作用请求自动重试。
- 上游显式 Shell Session、Shell 取消及 File API 可以用于兼容探针；公开请求类型不含
  UID/GID。生产 Core 操作必须通过 NewX `sessiond` 降权，不能共享 `gem` 或 root。
- 上游仓库不包含可直接修改的完整 AIO 服务实现。本计划使用派生镜像添加最小适配层，
  不 fork 或复制上游服务源码。

## 不可破坏的兼容规则

- `coze.sandbox.execute.v1` 的 URL、请求、响应、签名头和 Redis 记录格式不变；
- 现有 `RunnerScheduler.Accept(ExecuteCommand)`、Plugin Adapter、Provider CRUD 和默认
  Provider 行为保持原样；
- Session 使用独立 HTTP 路径、签名头、Redis key namespace 和持久化表；关闭
  `sandbox_session_v1` 后现有 one-shot 链必须仍可运行；
- Provider 没有同时声明 `sandbox_session_v1` 和 `signed_session_context_v2` 时，控制面
  不得调用 Session endpoint；
- 身份中的 provider、space、user、thread、run 和 profile 全由服务端生成，前端不能
  提交或覆盖；
- 上游 Shell ID、容器 boot ID、UID/GID、物理路径和凭据不进入公共响应、浏览器、普通
  日志或低基数指标；
- Redis、MySQL、AIO、签名密钥或身份代理任一不可用时 fail closed；
- 数据库迁移只允许 additive，不 drop、rename、truncate 或运行时 AutoMigrate；
- Phase 1 不触碰 Workbench 受监控路径；若实施中发现必须修改
  `backend/application/agentthread/**`，立即停止并转入 Phase 2 计划，不扩大本计划范围。

## 默认 `2C4G` Session 配置

Session 配置独立版本化，存入现有 `sandbox_scheduler_settings` 的新增列，避免向当前
严格解析的 `settings_json` 增加字段而破坏旧版本回滚。初始快照为：

```json
{
  "core_enabled": false,
  "interactive_enabled": false,
  "host_shell_enabled": false,
  "core_weight": 1,
  "heavy_weight": 2,
  "per_user_active_limit": 1,
  "idle_session_limit": 20,
  "idle_shell_limit": 4,
  "session_idle_ttl_seconds": 1200,
  "shell_idle_ttl_seconds": 300,
  "command_timeout_seconds": 600,
  "cancel_grace_seconds": 5,
  "aio_cpu_limit": 1.25,
  "aio_memory_limit_mb": 1536,
  "aio_pid_limit": 256,
  "aio_shm_limit_mb": 512,
  "workspace_quota_mb": 2048,
  "uid_min": 20000,
  "uid_max": 59999
}
```

规则：`interactive_enabled` 在 Phase 1 永远校验为 `false`；`host_shell_enabled` 只是期望
配置，运行时仍需满足 debug/loopback 门禁；热更新不允许改变 UID 范围、AIO endpoint、
镜像 digest、JWT/HMAC 密钥或持久卷；这些启动级配置变化必须重启并重新健康检查。

## Phase 1 完成门槛

只有以下条件全部满足才可把计划标记完成：

1. AIO/SDK 锁定探针、Session 单元/合同测试、现有 Sandbox/Plugin 回归全部通过；
2. 同一共享 AIO 容器内两个 Core Session 并发，第三个进入队列而非 500；
3. 同用户不同 Thread、不同用户之间均不能读、列出、写入或软链接逃逸到对方目录；
4. `ps`/文件属主证据证明命令以对应非 root UID/GID 运行；
5. 取消只杀当前 Thread 进程组，后台子进程不泄漏；
6. AIO 重启后 generation 增加、上游 Session 失效、持久工作区保留，命令不自动重放；
7. AIO 容器在 `1536 MiB / 1.25 CPU / 256 PID / 512 MiB shm` 硬限制下通过 Core
   压测，宿主机和 NewX 主服务无 OOM；
8. Session capability 默认关闭，AIO 与 `sessiond` 无宿主机公开端口；
9. 回滚到旧应用版本不读取新 Session 路径，也不要求删除新表或工作区；
10. 当前用户未提交文档仍保持未暂存、内容不变。

---

## Task 0: 冻结基线并建立执行证据目录

**Files:**

- Read: `AGENTS.md`
- Read: `docs/superpowers/context/project-context.md`
- Read: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Read: `docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`
- Create during execution only: `/private/tmp/newx-aio-phase1-evidence/`

- [ ] **Step 1: 核对分支、基线和用户改动**

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse dev
git rev-parse origin/dev
git diff -- docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md
```

Expected: 当前分支为用户指定的 `codex/plugin-trial-run-fix`；计划编写时设计提交为
`a6865a4db`、
`dev`/`origin/dev` 为 `61cb3764d`。若实施时 SHA 已变化，记录新 SHA 并先做差异审计；
不得丢弃或暂存用户已有设计文档改动。

- [ ] **Step 2: 建立本轮临时证据目录**

```bash
mktemp -d /private/tmp/newx-aio-phase1-evidence.XXXXXX
```

Expected: 返回仓库外目录。探针日志、容器 inspect、资源曲线和测试输出只写到该目录，
不提交运行时证据或秘密。

- [ ] **Step 3: 记录初始回归基线**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./pkg/sandboxidentity ./infra/sandbox ./internal/sandboxrunner
```

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-scheduler-card.test.tsx
```

Expected: 当前基线通过；若已有失败，先记录且只修与本计划直接相关的失败，不把历史失败
混入 Session 提交。

---

## Task 1: 锁定 AIO/SDK 合同并建立真实兼容探针

**Files:**

- Modify: `backend/go.mod`
- Modify: `backend/go.sum`
- Create: `backend/cmd/sandbox-aio-compat-probe/main.go`
- Create: `backend/internal/sandboxrunner/aio/upstream_client.go`
- Create: `backend/internal/sandboxrunner/aio/upstream_client_test.go`
- Create: `deploy/sandbox-runner/aio.lock.json`
- Create: `deploy/sandbox-runner/tests/aio_upstream_contract_test.sh`
- Modify after evidence: `docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`

- [ ] **Step 1: 先写 SDK Adapter 失败测试**

测试必须用 `httptest.Server` 固定验证：

- base URL 只能是启动配置给定的私有 AIO origin；
- Authorization 使用 Bearer JWT，不在 URL 或日志中出现；
- `http.Client.Timeout` 有界，所有调用继承 context deadline；
- `WithMaxAttempts(1)`，500/timeout 不自动重放命令或文件写入；
- 上游错误只映射稳定 reason code，不回传正文；
- Create/Exec/View/Wait/Kill/Cleanup 和 File Read/Write/List/Glob/Grep/Replace 的字段
  与 v0.0.5 一致；
- 测试通过反射确认上游 Shell/File 请求没有 UID/GID 字段，并把这个结果作为必须启用
  `sessiond` 的合同，而不是测试失败。

```go
func TestUpstreamSDKDoesNotClaimUnixIdentity(t *testing.T) {
    requestTypes := []reflect.Type{
        reflect.TypeOf(sandboxapi.ShellCreateSessionRequest{}),
        reflect.TypeOf(sandboxapi.ShellExecRequest{}),
        reflect.TypeOf(sandboxapi.FileWriteRequest{}),
    }
    for _, requestType := range requestTypes {
        for _, name := range []string{"UID", "GID", "User", "Username"} {
            _, found := requestType.FieldByName(name)
            require.False(t, found, "%s unexpectedly exposes %s", requestType, name)
        }
    }
}
```

其中 `sandboxapi` 明确导入根模块
`github.com/agent-infra/sandbox-sdk-go`；客户端构造器则使用
`github.com/agent-infra/sandbox-sdk-go/client`，不要凭包目录猜别名。

- [ ] **Step 2: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/aio -run 'TestUpstream'
```

Expected: FAIL，因为 package、锁文件和 Adapter 尚不存在。

- [ ] **Step 3: 引入唯一允许的上游 SDK 版本**

```bash
cd backend
go get github.com/agent-infra/sandbox-sdk-go@v0.0.5
go mod tidy
```

只允许 `v0.0.5`；检查 `go.mod`/`go.sum`，拒绝无关依赖升级。Adapter 创建客户端时使用：

```go
headers := make(http.Header)
headers.Set("Authorization", "Bearer "+token)
sdk := sandboxclient.NewClient(
    option.WithBaseURL(baseURL),
    option.WithHTTPClient(httpClient),
    option.WithHTTPHeader(headers),
    option.WithMaxAttempts(1),
)
```

实现不得调用 SDK README 中不存在于 v0.0.5 源码的 `option.WithToken`。

- [ ] **Step 4: 写入不可变锁文件**

`deploy/sandbox-runner/aio.lock.json` 使用严格 JSON：

```json
{
  "schema": "newx.aio.lock.v1",
  "image": "ghcr.io/agent-infra/sandbox:1.11.0@sha256:6328d7fd2f0ff0b4c147c3d05b3df1ce331f4a482eb6e550ecd64ed1fcf906e7",
  "amd64_manifest": "sha256:9a597aaa3716aca2fd42a517ceedc41063e5ceedcef43eb68bf7c059c0128b7a",
  "arm64_manifest": "sha256:5ca2cd5619ee1e18c5479301e740c1e35307ce85d4142a145aec65d459655eee",
  "go_sdk": "github.com/agent-infra/sandbox-sdk-go@v0.0.5",
  "native_uid_gid": false,
  "entrypoint": "/opt/gem/run.sh",
  "private_api_port": 8080
}
```

- [ ] **Step 5: 实现真实容器探针 CLI 和脚本**

探针仅接受环境变量中的临时 JWT 和私有 base URL，执行以下固定序列：

1. 创建 `newx-probe-a`、`newx-probe-b` 两个显式 Shell Session；
2. 并发写入各自 cwd/env marker，交叉读取不得串扰；
3. 前台、后台、View/Wait、Kill、Cleanup；
4. File Write/Read/List/Glob/Grep/Replace；
5. 取消 `sleep 30`，确认子进程消失；
6. 输出版本、能力、耗时和容器资源聚合，不输出 token 或文件正文；
7. 删除 probe Session 和 probe 目录。

脚本必须用锁文件中的 digest 启动临时容器，不发布到非 loopback；交互服务全部关闭：

```bash
docker run --detach --rm --name newx-aio-contract \
  --memory 1536m --cpus 1.25 --pids-limit 256 --shm-size 512m \
  -e DISABLE_BROWSER=true \
  -e DISABLE_JUPYTER=true \
  -e DISABLE_CODE_SERVER=true \
  -e DISABLE_MCP_BROWSER=true \
  -e DISABLE_VNC=true \
  -e DISABLE_NODEJS_REPL=true \
  -e JWT_PUBLIC_KEY="$JWT_PUBLIC_KEY" \
  -p 127.0.0.1::8080 \
  ghcr.io/agent-infra/sandbox:1.11.0@sha256:6328d7fd2f0ff0b4c147c3d05b3df1ce331f4a482eb6e550ecd64ed1fcf906e7
```

测试脚本用 trap 停止临时容器，不删除任何已有容器或 volume。

- [ ] **Step 6: 运行探针与 GREEN 单测**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/aio -run 'TestUpstream'
```

```bash
bash deploy/sandbox-runner/tests/aio_upstream_contract_test.sh
```

Expected: SDK 合同与真实 AIO Core 探针通过；报告明确 `native_uid_gid=false`。如果显式
Session、取消、文件 API 或 `1536m` 启动任一失败，停止 Phase 1，不写绕过补丁。

- [ ] **Step 7: 用真实证据修正文档认证名词**

将设计中“启用 `SANDBOX_API_KEY`”改为“锁定版 AIO 使用 `JWT_PUBLIC_KEY` 和 Runner
签发的短期 JWT；NewX `sessiond` 使用独立 HMAC grant”。只改这一条已核实事实，不能
扩大设计范围。

- [ ] **Step 8: 提交锁定合同**

```bash
git add backend/go.mod backend/go.sum backend/cmd/sandbox-aio-compat-probe backend/internal/sandboxrunner/aio deploy/sandbox-runner/aio.lock.json deploy/sandbox-runner/tests/aio_upstream_contract_test.sh docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md
git commit -m "test: lock AIO core sandbox contract"
```

---

## Task 2: 增加 Session Domain、Core 合同和身份 v2

**Files:**

- Modify: `backend/domain/sandbox/entity.go`
- Create: `backend/domain/sandbox/session.go`
- Create: `backend/domain/sandbox/session_test.go`
- Create: `backend/infra/sandbox/session.go`
- Create: `backend/infra/sandbox/session_test.go`
- Create: `backend/pkg/sandboxidentity/session_context.go`
- Create: `backend/pkg/sandboxidentity/session_context_test.go`
- Modify: `backend/pkg/sandboxidentity/context_test.go`

- [ ] **Step 1: 先写 capability、状态机、输入边界失败测试**

覆盖：

- `sandbox_session_v1` 是可选 Provider feature，旧 feature 列表仍可解析；
- `signed_session_context_v2` 是独立可选 feature，不能把 v1 签名支持等同于 v2；
- Profile 只接受 `core`/`interactive`，Phase 1 admission 只允许 `core`；
- Session 稳定键必须包含合法 deployment、正数 provider/space/user、规范 thread ID
  和 profile；
- Acquire/Get/Release/Destroy/Recover 状态转换幂等；
- Exec 只允许非空 argv 或 command、审核 env、逻辑 cwd、deadline、输出上限；
- File path 只能使用 `/mnt/user-data/workspace|uploads|outputs` 和只读
  `/mnt/skills`，拒绝 NUL、`..`、未规范绝对路径和超长值；
- 公共 String/GoString/Format 不泄露身份、命令、路径或正文。

- [ ] **Step 2: 先写 Session 身份 v2 失败测试**

身份 v2 使用独立 envelope schema 和独立头：

```go
const (
    SessionContextHeader          = "X-Coze-Sandbox-Session-Context"
    SessionContextSignatureHeader = "X-Coze-Sandbox-Session-Context-Signature"
)

type SessionRequest struct {
    ProviderID    int64
    Scope         Scope
    SpaceID       int64
    UserID        int64
    ThreadID      string
    RunID         string
    OperationID   string
    Profile       string
    RequestDigest []byte
}
```

测试签名必须覆盖 schema、provider、scope、space、user、thread、run、operation、
profile、canonical method/path/body digest、iat、exp、nonce、key ID。验证以下负例：旧 v1
头冒充 v2、provider/profile/path 改写、过期、未来签发、未知 key、重复 nonce、digest
不匹配、未知字段和重复 JSON key。

- [ ] **Step 3: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox ./pkg/sandboxidentity -run 'Session|Feature'
```

Expected: FAIL，因为 Session 类型和 feature 尚不存在。

- [ ] **Step 4: 实现最小 Domain 与统一 Go 合同**

`backend/domain/sandbox/session.go` 定义持久化实体和值对象；
`backend/infra/sandbox/session.go` 定义业务可依赖、与 SDK 解耦的合同：

```go
type SandboxSessionManager interface {
    Acquire(context.Context, AcquireSessionRequest) (SandboxSession, error)
    Get(context.Context, domainsandbox.SessionRef) (SandboxSession, error)
    Release(context.Context, domainsandbox.SessionRef) error
    Destroy(context.Context, domainsandbox.SessionRef) error
    Recover(context.Context, domainsandbox.SessionRef) (SandboxSession, error)
}

type SandboxSession interface {
    Ref() domainsandbox.SessionRef
    Exec(context.Context, ExecRequest) (ExecutionStream, error)
    Read(context.Context, ReadRequest) (FileContent, error)
    Write(context.Context, WriteRequest) error
    List(context.Context, ListRequest) ([]FileEntry, error)
    Glob(context.Context, GlobRequest) ([]FileEntry, error)
    Grep(context.Context, GrepRequest) ([]GrepMatch, error)
    Replace(context.Context, ReplaceRequest) error
    Download(context.Context, DownloadRequest) (io.ReadCloser, error)
}
```

`Publish` 不在 Phase 1 实现；Artifact 在 Phase 2 接入。不要用空实现假装支持。

- [ ] **Step 5: 实现身份 v2，保持 v1 字节兼容**

新增 `SessionSigner`/`SessionVerifier` 或在 Keyring 上增加明确命名的方法；不得修改
现有 `Request`、v1 envelope、v1 header 和 canonical JSON。nonce 防重放接口由 Runner
注入 Redis store，单元测试使用内存 fake。

- [ ] **Step 6: 运行 GREEN 与 v1 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox ./pkg/sandboxidentity
```

Expected: PASS；现有 v1 golden/签名测试输出不变。

- [ ] **Step 7: 提交 Session 合同**

```bash
git add backend/domain/sandbox/entity.go backend/domain/sandbox/session.go backend/domain/sandbox/session_test.go backend/infra/sandbox/session.go backend/infra/sandbox/session_test.go backend/pkg/sandboxidentity/session_context.go backend/pkg/sandboxidentity/session_context_test.go backend/pkg/sandboxidentity/context_test.go
git commit -m "feat: add sandbox session core contract"
```

---

## Task 3: 增加向后兼容的 Session 动态配置

**Files:**

- Create: `backend/domain/sandbox/session_settings.go`
- Create: `backend/domain/sandbox/session_settings_test.go`
- Modify: `backend/domain/sandbox/repository.go`
- Modify: `backend/infra/sandbox/mysql_models.go`
- Create: `backend/infra/sandbox/mysql_session_settings_repository.go`
- Create: `backend/infra/sandbox/mysql_session_settings_repository_test.go`
- Create: `backend/infra/sandbox/mysql_session_settings_audit_repository.go`
- Create: `backend/infra/sandbox/mysql_session_settings_audit_repository_test.go`
- Modify: `backend/infra/sandbox/mysql_scheduler_migration_test.go`

- [ ] **Step 1: 写完整快照与热更新边界失败测试**

测试默认 JSON 与本文一致，拒绝部分快照、未知字段、重复 key、NaN/指数 CPU、
`core_weight > total_weight`、interactive=true、UID 范围变化热更新、资源上限高于
`2C4G` 基线、负 TTL、idle shell 大于 idle session、Host Shell 非布尔值。

- [ ] **Step 2: 写旧 Scheduler 回滚兼容测试**

模拟同一行同时存在旧 `settings_json/version` 与新
`session_settings_json/session_settings_version`：

- 旧 `GetSchedulerSettings` 只读旧列且结果不变；
- 旧 `UpdateSchedulerSettingsCAS` 不覆盖 Session 列；
- 新 `UpdateSessionSettingsCAS` 不递增旧 version、不覆盖旧 JSON；
- 两套 CAS 并发更新互不制造伪冲突。

- [ ] **Step 3: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox -run 'SessionSettings|SchedulerMigration'
```

Expected: FAIL，因为新列、仓储和验证器尚不存在。

- [ ] **Step 4: 实现独立版本配置合同**

新增：

```go
type SessionRuntimeSettings struct {
    CoreEnabled             bool
    InteractiveEnabled      bool
    HostShellEnabled        bool
    CoreWeight              int
    HeavyWeight             int
    PerUserActiveLimit      int
    IdleSessionLimit        int
    IdleShellLimit          int
    SessionIdleTTLSeconds   int
    ShellIdleTTLSeconds     int
    CommandTimeoutSeconds   int
    CancelGraceSeconds      int
    AIOCPULimit             CPUQuotaMilli
    AIOMemoryLimitMB        int
    AIOPIDLimit             int
    AIOSHMSizeMB            int
    WorkspaceQuotaMB        int
    UIDMin                  uint32
    UIDMax                  uint32
    Version                 uint64 `json:"-"`
    UpdatedBy               int64  `json:"-"`
}
```

`SessionSettingsRepository` 与旧 `SchedulerSettingsRepository` 分离。另建
`SessionSettingsAuditRepository`，审计记录仍复用现有 append-only
`sandbox_scheduler_audit_events`，action 使用 `session_settings.*`，不新增第四张
配置/审计表。默认 `CoreEnabled` 为 false，部署和迁移完成不自动接流量。

- [ ] **Step 5: 扩展 PO，但不在此任务运行迁移**

`schedulerSettingsPO` 新增 nullable JSON、独立 version 和 generation 列的映射；新仓储
在列不存在时返回 `ErrConfigurationInvalid`，不调用 AutoMigrate，不静默使用内存默认值。

- [ ] **Step 6: 运行 GREEN 与旧 Scheduler 全量回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox -run 'Scheduler|SessionSettings'
```

- [ ] **Step 7: 提交配置合同**

```bash
git add backend/domain/sandbox/session_settings.go backend/domain/sandbox/session_settings_test.go backend/domain/sandbox/repository.go backend/infra/sandbox/mysql_models.go backend/infra/sandbox/mysql_session_settings_repository.go backend/infra/sandbox/mysql_session_settings_repository_test.go backend/infra/sandbox/mysql_session_settings_audit_repository.go backend/infra/sandbox/mysql_session_settings_audit_repository_test.go backend/infra/sandbox/mysql_scheduler_migration_test.go
git commit -m "feat: add versioned sandbox session settings"
```

---

## Task 4: 创建 additive 迁移和 MySQL Runtime Session 仓储

**Files:**

- Create: `docker/atlas/migrations/20260813000100_sandbox_shared_aio_core.sql`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `backend/infra/sandbox/mysql_models.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_repository.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_repository_test.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_integration_test.go`
- Modify: `backend/domain/sandbox/repository.go`

- [ ] **Step 1: 写迁移结构失败测试**

测试必须解析 migration/schema 并断言：

- 只 `ALTER sandbox_scheduler_settings ADD COLUMN`、`CREATE TABLE`、`INSERT/UPDATE`；
- 没有 `DROP`、`TRUNCATE`、`RENAME`、删除列或重建现有表；
- Phase 1 只创建 `sandbox_runtime_identities`、`sandbox_runtime_sessions`；
- 不提前创建 `sandbox_runtime_service_leases`；
- 两张表的业务唯一键、UID 唯一键、状态/版本/时间列和索引完整；
- Session 配置列与旧 scheduler 列独立；
- `aio_runtime_generation` 初值为 `0`，只能由 Runner 事务递增。

- [ ] **Step 2: 写仓储并发失败测试**

用 `sqlmock` 和真实 MySQL 集成测试覆盖：

- 两个进程并发 Resolve 同一业务身份，只得到一条映射和同一个 UID/GID；
- 两个不同 Thread 不得到同一个 UID；
- 分配在 `[uid_min, uid_max]` 中取最小空闲值，耗尽返回稳定容量错误；
- retired UID 在显式冷却期结束前不复用；
- Session 唯一键为 deployment/provider/space/user/thread/profile；
- identity/session 都绑定 `SANDBOX_RUNNER_DEPLOYMENT_ID`；两个 Provider 指向同一 AIO
  deployment 时也不能分配相同 UID/GID；
- Acquire 幂等，CAS 状态转换拒绝 stale version；
- `NextRuntimeGeneration` 锁定 singleton 行并严格递增；
- `RecoverableSessions` 只返回旧 generation 且非 destroyed 的行；
- MySQL 出错不回退随机 UID、内存仓储或共享 UID。

- [ ] **Step 3: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox -run 'RuntimeSession|SharedAIOMigration'
```

Expected: FAIL，因为迁移和仓储尚不存在。

- [ ] **Step 4: 编写唯一一份 Phase 1 迁移**

迁移必须采用以下结构，不把原始路径或上游 token 落库：

```sql
ALTER TABLE `sandbox_scheduler_settings`
  ADD COLUMN `session_settings_json` JSON NULL AFTER `settings_json`,
  ADD COLUMN `session_settings_version` BIGINT UNSIGNED NOT NULL DEFAULT 1 AFTER `session_settings_json`,
  ADD COLUMN `session_settings_updated_by` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `session_settings_version`,
  ADD COLUMN `session_settings_updated_at` DATETIME(3) NULL AFTER `session_settings_updated_by`,
  ADD COLUMN `aio_runtime_generation` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `session_settings_updated_at`;

CREATE TABLE `sandbox_runtime_identities` (
  `identity_id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `deployment_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `space_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `thread_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `runtime_uid` INT UNSIGNED NOT NULL,
  `runtime_gid` INT UNSIGNED NOT NULL,
  `workspace_key` CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `version` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  `retired_at` DATETIME(3) NULL,
  PRIMARY KEY (`identity_id`),
  UNIQUE KEY `uk_sandbox_runtime_identity_business` (`deployment_id`,`provider_id`,`space_id`,`user_id`,`thread_id`),
  UNIQUE KEY `uk_sandbox_runtime_identity_uid` (`deployment_id`,`runtime_uid`),
  UNIQUE KEY `uk_sandbox_runtime_identity_workspace` (`deployment_id`,`workspace_key`),
  KEY `idx_sandbox_runtime_identity_state_updated` (`deployment_id`,`state`,`updated_at`,`identity_id`),
  CONSTRAINT `fk_sandbox_runtime_identity_provider` FOREIGN KEY (`provider_id`) REFERENCES `sandbox_providers` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sandbox_runtime_sessions` (
  `session_id` CHAR(26) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `identity_id` BIGINT UNSIGNED NOT NULL,
  `deployment_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `space_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `thread_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `profile` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `runtime_generation` BIGINT UNSIGNED NOT NULL,
  `upstream_shell_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `recovery_reason` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  `version` BIGINT UNSIGNED NOT NULL,
  `last_activity_at` DATETIME(3) NOT NULL,
  `expires_at` DATETIME(3) NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`session_id`),
  UNIQUE KEY `uk_sandbox_runtime_session_business` (`deployment_id`,`provider_id`,`space_id`,`user_id`,`thread_id`,`profile`),
  KEY `idx_sandbox_runtime_session_identity` (`identity_id`,`profile`),
  KEY `idx_sandbox_runtime_session_recovery` (`deployment_id`,`runtime_generation`,`state`,`session_id`),
  KEY `idx_sandbox_runtime_session_expiry` (`deployment_id`,`state`,`expires_at`,`session_id`),
  CONSTRAINT `fk_sandbox_runtime_session_identity` FOREIGN KEY (`identity_id`) REFERENCES `sandbox_runtime_identities` (`identity_id`),
  CONSTRAINT `fk_sandbox_runtime_session_provider` FOREIGN KEY (`provider_id`) REFERENCES `sandbox_providers` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

在同一迁移中写入本文“默认 `2C4G` Session 配置”，再把
`session_settings_json` 改成 `NOT NULL`。不要使用 JSON 默认表达式，以兼容目标 MySQL。

- [ ] **Step 5: 实现事务分配和 generation fencing**

`MySQLRepository` 新增的 runtime 方法使用同一数据库事务：

1. `SELECT sandbox_scheduler_settings(id=1) FOR UPDATE` 作为跨进程分配锁；
2. 读取 Session 设置和 UID 范围；
3. 查询同一 `SANDBOX_RUNNER_DEPLOYMENT_ID` 已占用 UID，选择最小可用 UID；
4. 由服务端 HMAC 完整 `deployment + provider + space + user + thread` 键生成不可逆
   `workspace_key`；
5. 插入 identity，唯一冲突后重新读取同一业务键；
6. 创建/读取 Session；
7. commit 后返回 detached domain 值。

`NextRuntimeGeneration` 在同一 singleton 行上用 CAS/行锁加一。不要用 Redis INCR、
Unix 时间或容器 ID 替代 MySQL generation。

`workspace_key` 由专用 `SANDBOX_WORKSPACE_KEY_HASH_KEYS_JSON` HMAC 生成，密钥不复用
Provider credential、queue encryption 或 Session grant。Runner 缺少该 keyring 时 Session
fail closed；旧 one-shot 不受影响。Thread ID 不以明文、可逆编码或裸 SHA 进入物理路径。

- [ ] **Step 6: 更新 Atlas schema 和 hash**

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate hash --dir file:///migrations
```

手工同步 `docker/atlas/opencoze_latest_schema.hcl`，不得使用数据库 dump 覆盖无关 schema。

- [ ] **Step 7: 运行 Atlas 与仓储验证**

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate validate --dir file:///migrations
```

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox -run 'RuntimeSession|SessionSettings|SchedulerMigration'
```

Expected: Atlas hash/validate 和仓储测试通过。集成测试需要显式测试 DSN；没有 DSN 时只
允许 skip 并在 Phase 1 最终闸门使用 dev 同构 MySQL 补跑，不得把 skip 当通过。

- [ ] **Step 8: 提交迁移与仓储**

```bash
git add docker/atlas/migrations/20260813000100_sandbox_shared_aio_core.sql docker/atlas/migrations/atlas.sum docker/atlas/opencoze_latest_schema.hcl backend/domain/sandbox/repository.go backend/infra/sandbox/mysql_models.go backend/infra/sandbox/mysql_runtime_session_repository.go backend/infra/sandbox/mysql_runtime_session_repository_test.go backend/infra/sandbox/mysql_runtime_session_integration_test.go
git commit -m "feat: persist shared AIO runtime sessions"
```

---

## Task 5: 构建 NewX `sessiond` 身份代理和安全文件层

**Files:**

- Create: `backend/cmd/newx-aio-sessiond/main.go`
- Create: `backend/internal/aiosessiond/config.go`
- Create: `backend/internal/aiosessiond/config_test.go`
- Create: `backend/internal/aiosessiond/grant.go`
- Create: `backend/internal/aiosessiond/grant_test.go`
- Create: `backend/internal/aiosessiond/server.go`
- Create: `backend/internal/aiosessiond/server_test.go`
- Create: `backend/internal/aiosessiond/process.go`
- Create: `backend/internal/aiosessiond/process_linux_test.go`
- Create: `backend/internal/aiosessiond/file_helper.go`
- Create: `backend/internal/aiosessiond/path_linux.go`
- Create: `backend/internal/aiosessiond/path_unsupported.go`
- Create: `backend/internal/aiosessiond/path_test.go`
- Create: `deploy/sandbox-runner/Dockerfile.aio`
- Create: `deploy/sandbox-runner/aio/supervisord.newx_sessiond.conf`
- Create: `deploy/sandbox-runner/tests/aio_identity_contract_test.sh`

- [ ] **Step 1: 写 grant 验证和启动门禁失败测试**

`sessiond` 只接受 Runner 签发、最长 30 秒的 HMAC grant。grant 必须绑定：

```go
type Grant struct {
    Schema            string `json:"schema"`
    KeyID             string `json:"key_id"`
    Nonce             string `json:"nonce"`
    IssuedAtUnix      int64  `json:"issued_at_unix"`
    ExpiresAtUnix     int64  `json:"expires_at_unix"`
    ProviderID        int64  `json:"provider_id"`
    SessionID         string `json:"session_id"`
    OperationID       string `json:"operation_id"`
    RuntimeGeneration uint64 `json:"runtime_generation"`
    RuntimeUID        uint32 `json:"runtime_uid"`
    RuntimeGID        uint32 `json:"runtime_gid"`
    WorkspaceKey      string `json:"workspace_key"`
    OperationKind     string `json:"operation_kind"`
    RequestDigest     string `json:"request_digest"`
}
```

测试拒绝：过期/未来、重复 nonce、未知 key、UID 越界、generation=0、workspace key
非 64 hex、kind/path/body 改写、未知/重复 JSON 字段、没有 secret file、secret 文件权限
宽于 `0600`、非 Linux 启动。Bearer service token 只认证 Runner 到 `sessiond` 的私网调用；
grant HMAC 才绑定具体 identity/operation。二者使用不同 key，任一缺失都拒绝。

- [ ] **Step 2: 写进程隔离失败测试**

Linux integration 测试验证：

- `SysProcAttr.Credential{Uid,Gid,NoSetGroups:true}`；
- 每个 Session 独立 process group；
- cwd、HOME、TMPDIR 指向 Thread 目录；
- 清空继承环境，只注入固定 PATH/LANG/HOME/TMPDIR 和审核变量；
- stdout/stderr 单独限长，超限返回稳定 code；
- timeout/cancel 先 TERM 整个负 PGID，grace 后 KILL；
- 后台孙进程也消失；
- 命令进程的 `/proc/<pid>/status` 显示目标非 root UID/GID；
- Session A 的 cancel 不影响 Session B。

stdout、stderr、Read 和 Download 的字节不能写入 Redis。`sessiond` 将每个 operation 的
有界输出写入 root-owned `/run/newx-sessiond/spool/<session>/<operation>`，目录 `0700`、
文件 `0600`，用户进程不可改写；Redis 只保存 size/digest/status/TTL。Compose 为该目录
配置 `64m` tmpfs，单 operation 默认上限 `16 MiB`、全局上限 `64 MiB`。超限时终止当前
operation 并清理 spool。Runner 通过一次性 grant 流式读取，读完或 TTL 到期删除；AIO
重启后未消费的结果变 unavailable/unknown，不自动重跑命令。

- [ ] **Step 3: 写文件逃逸失败测试**

所有文件操作由同一二进制的 `file-helper` 子模式执行，父 `sessiond` 以目标 UID/GID
启动 helper；父进程不能以 root 直接读写用户文件。测试：

- 逻辑目录映射到 `workspace/uploads/outputs/skills`；
- 目录创建后 owner 为目标 UID/GID，Thread 根目录 mode `0700`；
- `openat2` 使用 `RESOLVE_BENEATH|RESOLVE_NO_MAGICLINKS|RESOLVE_NO_SYMLINKS`；
- 拒绝 `..`、绝对物理路径、NUL、procfs、软/硬链接越界、TOCTOU 目录替换；
- read/write/append/list/glob/grep/replace/download 均遵守相同 resolver；
- `/mnt/skills` 只读，workspace quota 和单文件/响应上限有效；
- 每个 Thread 的写/append/replace 使用跨 operation workspace lock；写入前做 fd-relative
  用量扫描，写到同目录临时文件，校验 `current - old + new <= quota` 后 atomic rename，
  防止两个并发写同时越过软配额；
- 不支持 `openat2` 的 Linux 内核 fail closed；非 Linux 构建只提供返回 unavailable 的
  stub，不提供不安全 fallback。

- [ ] **Step 4: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/aiosessiond
```

Expected: FAIL，因为 `sessiond` 尚不存在。

- [ ] **Step 5: 实现最小私有协议**

`sessiond` 只监听启动配置指定的容器私网地址，端点固定为：

- `GET /v1/health`：`boot_id`、AIO version/base digest/image revision、sessiond version、
  generation 支持以及 identity/file/process capability；
- `POST /v1/sessions:prepare`：创建/校验 Thread 目录；
- `POST /v1/sessions/{id}/operations`：以 NDJSON 流返回 start/stdout/stderr/result；
- `POST /v1/sessions/{id}/operations/{operation_id}:cancel`；
- `POST /v1/sessions/{id}:cleanup`。

除只返回低敏健康投影的 `GET /v1/health` 外，请求必须同时通过 Bearer service token、
grant HMAC、nonce、防重放和 body digest；health 仍必须要求 service token，并且不能返回
UID/GID、路径或 secret 状态正文。其他响应也不得出现 raw UID/GID。Session/operation
map 有上限和 TTL，超过上限返回容量错误。

- [ ] **Step 6: 构建派生 AIO 镜像，不覆盖上游服务**

`deploy/sandbox-runner/Dockerfile.aio` 使用 Go builder 后：

```dockerfile
FROM golang:1.24-alpine AS sessiond-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/newx-aio-sessiond ./cmd/newx-aio-sessiond

FROM ghcr.io/agent-infra/sandbox:1.11.0@sha256:6328d7fd2f0ff0b4c147c3d05b3df1ce331f4a482eb6e550ecd64ed1fcf906e7
USER root
ARG NEWX_AIO_IMAGE_REVISION
RUN test -n "${NEWX_AIO_IMAGE_REVISION}"
LABEL org.opencontainers.image.revision=${NEWX_AIO_IMAGE_REVISION}
ENV NEWX_AIO_VERSION=1.11.0 \
    NEWX_AIO_BASE_DIGEST=sha256:6328d7fd2f0ff0b4c147c3d05b3df1ce331f4a482eb6e550ecd64ed1fcf906e7 \
    NEWX_AIO_IMAGE_REVISION=${NEWX_AIO_IMAGE_REVISION}
COPY --from=sessiond-builder /out/newx-aio-sessiond /opt/newx/newx-aio-sessiond
COPY deploy/sandbox-runner/aio/supervisord.newx_sessiond.conf /opt/gem/supervisord/supervisord.newx_sessiond.conf
RUN chmod 0755 /opt/newx/newx-aio-sessiond \
 && chmod 0644 /opt/gem/supervisord/supervisord.newx_sessiond.conf
```

不得改写 `/opt/gem/run.sh`。使用上游约定的 `/opt/gem/supervisord/*.conf` 注册
`sessiond`，配置 `autorestart=true`、`stopasgroup=true`、`killasgroup=true`、
`stopsignal=TERM` 和有限 `stopwaitsecs`，由 AIO 现有 supervisor 负责启动、重启和退出
回收，不使用未经锁定探针证实的自定义 shutdown 环境变量。`sessiond` 每次进程启动时在
容器 tmpfs `/run/newx-aio` 原子生成新 `boot_id`；因此 AIO 或 `sessiond` 重启都会触发
保守 generation fencing。它只从上述非秘密镜像元数据和本地 `boot_id` 形成健康响应，
不信任请求方提交这些字段。镜像构建测试检查 ENTRYPOINT 仍是 `/opt/gem/run.sh`、
supervisor 实际管理 `sessiond`，且 revision 为空时构建必须失败。

- [ ] **Step 7: 运行真实身份合同**

`aio_identity_contract_test.sh` 构建派生镜像后，在临时 volume 中创建 Thread A/B/C，
验证：

- A、B 并发；
- 相同业务身份重复 prepare 幂等；
- A/B/C 各自 UID/GID 不同；
- A 不能 list/read/write B；
- A 的软链接不能逃逸；
- cancel A 不影响 B；
- 容器停止时 `sessiond` 无孤儿进程；
- raw AIO `8080` 和 `sessiond` 端口只在临时私网可达，宿主机无 publish。

```bash
bash deploy/sandbox-runner/tests/aio_identity_contract_test.sh
```

Expected: PASS。若上游 hook 不以允许安全降权的启动身份运行，或必须使用
`seccomp=unconfined` 才能完成 Core 操作，停止实施并回到设计评审；不得给容器增加
`privileged: true`。

- [ ] **Step 8: 运行 Go GREEN**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/aiosessiond ./cmd/newx-aio-sessiond
```

- [ ] **Step 9: 提交身份代理**

```bash
git add backend/cmd/newx-aio-sessiond backend/internal/aiosessiond deploy/sandbox-runner/Dockerfile.aio deploy/sandbox-runner/aio deploy/sandbox-runner/tests/aio_identity_contract_test.sh
git commit -m "feat: add isolated AIO session daemon"
```

---

## Task 6: 实现 Runner AIO 监督、generation 和私有客户端

**Files:**

- Modify: `backend/internal/sandboxrunner/config.go`
- Modify: `backend/internal/sandboxrunner/config_test.go`
- Create: `backend/internal/sandboxrunner/aio/sessiond_client.go`
- Create: `backend/internal/sandboxrunner/aio/sessiond_client_test.go`
- Create: `backend/internal/sandboxrunner/aio/lifecycle.go`
- Create: `backend/internal/sandboxrunner/aio/lifecycle_test.go`
- Create: `backend/internal/sandboxrunner/aio/grant_signer.go`
- Create: `backend/internal/sandboxrunner/aio/grant_signer_test.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition_test.go`

- [ ] **Step 1: 写启动配置失败测试**

Session backend 启用时必须同时存在：

- `SANDBOX_RUNNER_SESSION_ENABLED=true`；
- `SANDBOX_RUNNER_AIO_INTERNAL_URL=http://coze-sandbox-aio:8090`；
- `SANDBOX_RUNNER_AIO_UPSTREAM_URL=http://coze-sandbox-aio:8080`；
- `SANDBOX_RUNNER_AIO_SERVICE_TOKEN_FILE`、JWT private key file、session grant keyring；
- `MYSQL_DSN`、锁定 AIO base digest 和当前部署 AIO image revision；
- 两个 URL 只能是同一专用私网的精确 host/port，无 userinfo/query/fragment；
- secret file 权限不宽于 `0600`；
- Session 关闭时旧 Runner 配置仍能启动，不要求 AIO/MySQL/JWT 新变量。

- [ ] **Step 2: 写共享容器监督失败测试**

用 fake AIO health source 和 fake repository 覆盖：

- Runner 不创建、删除或重启 AIO 容器，AIO 的唯一生命周期所有者是 Compose/部署层；
- Runner 只通过私网 health 读取 AIO `boot_id`、base digest、revision 和 sessiond contract；
- AIO ready 且 boot fingerprint 首次出现/发生变化时才从 MySQL 取下一个 generation；
- Runner 自身重启并连接同一 `boot_id` 不增加 generation；
- AIO `boot_id` 变化触发 generation 增加和旧 Session stale 标记；
- AIO 丢失时 Session unavailable，等待部署层按 restart policy 恢复，不重放 running operation；
- health/grant/JWT/MySQL 任一失败，capability 不 ready。

- [ ] **Step 3: 写私有客户端失败测试**

`sessiond_client` 必须：

- 使用固定 origin、Bearer service token、HMAC grant、body digest；
- 禁止 redirect，禁止代理环境变量，精确限制响应大小；
- Exec NDJSON 流按 sequence 校验，重复/跳号/未知 event fail closed；
- context cancel 调用一次 cancel endpoint，不自动重试原 Exec；
- sessiond 错误只映射稳定 reason code；
- 所有 String/日志对象脱敏。

- [ ] **Step 4: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/... -run 'AIO|SessionConfig|SharedContainer|Generation|Grant'
```

Expected: FAIL，因为 AIO supervisor 和配置尚不存在。

- [ ] **Step 5: 实现窄的 AIO supervisor source**

不要把共享 AIO 强塞进现有 one-shot `Driver.Execute`，也不要让 Runner 和 Compose 成为
两个容器生命周期所有者。新增最小只读接口：

```go
type AIOHealthSource interface {
    UpstreamHealth(context.Context) (UpstreamHealth, error)
    SessiondHealth(context.Context) (SessiondHealth, error)
}
```

health 必须返回由 AIO/sessiond 自报且被锁定合同校验的 `boot_id`、AIO version/base
digest、sessiond version、能力和 readiness。Runner 不接 Docker socket 查询 AIO；部署
合同负责 image/volume/network/resource/revision，Runner 负责运行时身份与 generation。

- [ ] **Step 6: 实现 `AIOLifecycle`**

启动序列：

1. 读取 MySQL Session 设置；
2. 检查 raw AIO SDK health 与 `sessiond /v1/health`；
3. 校验 version/base digest/revision 与 Runner 启动配置；
4. 比较持久化的 boot fingerprint；
5. 首次健康 boot 或 fingerprint 改变时事务递增 generation；
6. 将旧 generation Session 标记 recovering，不触发 Exec；
7. 才把 `sandbox_session_v1` readiness 设为 true。

现有 one-shot lifecycle、execution image 和 Runner readiness 不被 AIO failure 拉成不可用；
health 响应分别报告 `one_shot_ready` 与 `session_core_ready`。

- [ ] **Step 7: 接入 `NewProcessRuntime` 的 MySQL 依赖**

只在 Session 开关为 true 时调用 `mysql.New()` 和
`infrasandbox.NewMySQLRepository(db)`。Runner 专用 secret env 只包含必要的 MySQL、
Redis、AIO 和签名配置，不加载完整 `app.env`。初始化失败时 Session fail closed，但旧
one-shot Runtime 仍按原配置运行。`Runtime` 持有并在 `Run` 退出时关闭新增 SQL pool；
构造中途失败也必须关闭已创建 pool，测试检查连接无泄漏。

- [ ] **Step 8: 运行 GREEN 与旧 Runtime 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/... ./infra/sandbox -run 'AIO|SessionConfig|Supervisor|Generation|Runtime'
```

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner -run 'Lifecycle|WorkloadDispatcher|Server|E2E'
```

Expected: 新测试和原 one-shot Runtime 测试通过。

- [ ] **Step 9: 提交 Runner AIO supervisor**

```bash
git add backend/internal/sandboxrunner/config.go backend/internal/sandboxrunner/config_test.go backend/internal/sandboxrunner/aio backend/internal/sandboxrunner/runtime_composition.go backend/internal/sandboxrunner/runtime_composition_test.go
git commit -m "feat: manage one shared AIO runtime"
```

---

## Task 7: 复用公平队列并增加 Runner Session HTTP 合同

**Files:**

- Modify: `backend/internal/sandboxrunner/scheduler.go`
- Modify: `backend/internal/sandboxrunner/scheduler_test.go`
- Modify: `backend/internal/sandboxrunner/redis_store.go`
- Modify: `backend/internal/sandboxrunner/redis_store_test.go`
- Create: `backend/internal/sandboxrunner/session_store.go`
- Create: `backend/internal/sandboxrunner/session_store_test.go`
- Create: `backend/internal/sandboxrunner/session_protocol.go`
- Create: `backend/internal/sandboxrunner/session_protocol_test.go`
- Create: `backend/internal/sandboxrunner/session_dispatcher.go`
- Create: `backend/internal/sandboxrunner/session_dispatcher_test.go`
- Create: `backend/internal/sandboxrunner/session_server.go`
- Create: `backend/internal/sandboxrunner/session_server_test.go`
- Modify: `backend/internal/sandboxrunner/server.go`
- Modify: `backend/internal/sandboxrunner/server_test.go`
- Modify: `backend/internal/sandboxrunner/runtime_status.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition.go`
- Modify: `backend/internal/sandboxrunner/e2e_test.go`

- [ ] **Step 1: 写 v1 零变化和 Redis namespace 失败测试**

先保存现有 v1 HTTP/Redis golden，新增断言：

- `POST /v1/executions` 的 parse、canonical body、签名、idempotency 和 Redis key 不变；
- Session 操作只写 `sandbox-runner:<deployment>:session-v1:*`；
- 旧 Runner 只读取原 namespace，看不到 Session 记录；
- Session 请求不能进入 `parseExecute` 或 `WorkloadDispatcher`；
- Session operation payload 在 Redis 中加密、有限 TTL，明文命令/路径/正文不可检索；
- Session operation ID 使用 `sop_` 前缀，one-shot execution ID 规则不变。

- [ ] **Step 2: 写共享权重和公平性失败测试**

保持 `RunnerScheduler.Accept(ExecuteCommand)` 现有签名，新增
`AcceptSession(SessionOperationCommand)` 和独立 `SessionExecutionStore`/
`SessionOperationDispatcher`。同一个 scheduler 的 `usedWeight` 管理两类 item。测试矩阵：

| 场景 | 期望 |
| --- | --- |
| 两个 Core Session | 同时运行，权重 `1+1=2` |
| 第三个 Core | accepted/queued，不是 500 |
| 现有 Agent one-shot（权重 2）运行 | Core 不出队 |
| 两个不同空间持续提交 | 空间轮转 |
| 同用户两个 Thread | 仅一个 active，另一个排队 |
| 排队取消/截止 | 只释放对应队列项 |
| 降低总权重 | 不杀运行项，停止新出队 |
| Runner 重启 | accepted Session 恢复；running 标为 unknown/failed，不重放 |

将现有 `isHeavyWorkload(scope)` 改成 item 自带、由服务端配置快照生成的
`SchedulingClass`；现有 Agent/AppDev one-shot 仍是 weight 2，Plugin/MCP stdio 仍按原
设置，Session Core 固定读取 `core_weight=1`。不要借用 `ScopePlugin` 冒充 Core。

- [ ] **Step 3: 写严格 Session 路由失败测试**

Runner 私有协议固定为：

| Method | Path | 语义 |
| --- | --- | --- |
| `POST` | `/v1/sessions:acquire` | 幂等创建/取得稳定 Session |
| `GET` | `/v1/sessions/{session_id}` | 安全状态投影 |
| `POST` | `/v1/sessions/{session_id}:release` | 释放上游 Shell，保留工作区 |
| `POST` | `/v1/sessions/{session_id}:destroy` | 销毁运行资源，默认不删工作区 |
| `POST` | `/v1/sessions/{session_id}:recover` | generation 校验后重建 Session |
| `POST` | `/v1/sessions/{session_id}/operations` | 提交 Exec/File Core 操作 |
| `GET` | `/v1/sessions/{session_id}/operations/{operation_id}` | 状态/有界结果 |
| `GET` | `/v1/sessions/{session_id}/operations/{operation_id}/events` | 有界 NDJSON 事件流 |
| `POST` | `/v1/sessions/{session_id}/operations/{operation_id}:cancel` | 排队或运行取消 |
| `GET` | `/v1/session-configuration` | Session 配置快照 |
| `PUT` | `/v1/session-configuration` | 签名 CAS 应用配置 |

所有 POST/PUT 必须 `application/json`、严格 JSON、请求不超过 1 MiB；download 返回也受
Session 设置与 Provider policy 双重限制。测试 404/405/415/413、trailing slash、
未知字段、重复 key、错误 content length、stream disconnect、身份 v2 缺失/篡改/重放。

- [ ] **Step 4: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner -run 'Session|SharedWeight|Namespace|V1Golden'
```

Expected: FAIL，因为 Session store、scheduler 和 routes 尚不存在。

- [ ] **Step 5: 实现独立 Session command/store**

`SessionOperationCommand` 只保存规范化后的 operation kind、SessionRef、服务端身份、
deadline、配置版本和加密后的 canonical operation envelope；不要同时保留一份明文 raw
body。operation kinds 固定为：

```text
exec read write append list glob grep replace download
```

Phase 1 不接受 publish/browser/jupyter/service。Redis Lua 对 Session 单独执行
idempotent accept、global/space/user queue depth、transition、complete、cancel、recover；
不得改旧 Lua key 顺序和 v1 记录格式。

- [ ] **Step 6: 扩展 scheduler 而不重写 one-shot**

`RunnerSchedulerConfig` 增加可选的 Session store/dispatcher/settings；nil 表示旧行为。
`scheduledItem` 使用 tagged union，dispatch 和 finish 分别调用现有 one-shot 接口或
Session 接口，但空间轮转、权重、内存水位和 `usedWeight` 共用。每个 item 在入队时
捕获 scheduler version、session settings version、weight、queue deadline；热更新不
改已入队快照。

当前 scheduler 的 `runningHeavy/runningLight` 互斥属于已上线 one-shot 语义，必须保留。
tagged item 将 Session Core 判定为 light，因此两个 Core 可以并发；现有 Agent/AppDev
仍是 heavy 并保持独占。新增 `activeSessionUsers map[userID]int` 只对 Session Core 强制
`per_user_active_limit=1`，不改变尚未迁移的 Plugin/MCP/Agent one-shot 单用户语义；这些
业务在 Phase 2 切到 Session 后自然进入同一限制。测试要锁住原 heavy/light 行为和新增
Session Core 行为。

- [ ] **Step 7: 实现 Session dispatcher**

dispatcher 流程：

1. 从 MySQL 重新读取 Session 与 identity，核对 deployment/provider/space/user/thread/profile；
2. 拒绝 stale generation 或 non-active Session；
3. 用当前 identity 和 operation digest 签发一次性 grant；
4. 调用 `sessiond`，严格消费 NDJSON；
5. stdout/stderr/file result 先限长写入 `sessiond` spool，Redis 只保存加密元数据；
6. terminal result 写成功后才释放 scheduler 权重；
7. cancel 终止对应进程组；网络不确定时返回 unknown，不重放 operation。

- [ ] **Step 8: 增加 capability-aware readiness**

`GET /v1/health` 的旧字段保持兼容，新增可选 feature/projection：

```json
{
  "features": ["queue_status_v1", "signed_execution_context_v1"],
  "session": {
    "available": false,
    "feature": "sandbox_session_v1",
    "runtime_generation": 0,
    "reason_code": "SESSION_PROFILE_DISABLED"
  }
}
```

只有 Core 开关、MySQL、Redis、AIO、sessiond、JWT/grant 和 generation 全部 ready 时才
把 `sandbox_session_v1` 与 `signed_session_context_v2` 同时加入顶层 features；任何一个
缺失都不能对外声明 Session 可用。旧 one-shot 健康不能被 AIO unavailable 误判为失败。

- [ ] **Step 9: 运行 GREEN、race 和 v1 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner -run 'Session|SharedWeight|Namespace|V1Golden|Server|Scheduler|RedisStore|E2E'
```

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -race ./internal/sandboxrunner -run 'Session|Scheduler'
```

Expected: 新合同通过；原 `server/scheduler/redis/e2e` 测试不需修改 golden 即通过。

- [ ] **Step 10: 提交 Runner Session 协议**

```bash
git add backend/internal/sandboxrunner/scheduler.go backend/internal/sandboxrunner/scheduler_test.go backend/internal/sandboxrunner/redis_store.go backend/internal/sandboxrunner/redis_store_test.go backend/internal/sandboxrunner/session_store.go backend/internal/sandboxrunner/session_store_test.go backend/internal/sandboxrunner/session_protocol.go backend/internal/sandboxrunner/session_protocol_test.go backend/internal/sandboxrunner/session_dispatcher.go backend/internal/sandboxrunner/session_dispatcher_test.go backend/internal/sandboxrunner/session_server.go backend/internal/sandboxrunner/session_server_test.go backend/internal/sandboxrunner/server.go backend/internal/sandboxrunner/server_test.go backend/internal/sandboxrunner/runtime_status.go backend/internal/sandboxrunner/runtime_composition.go backend/internal/sandboxrunner/e2e_test.go
git commit -m "feat: schedule sandbox core sessions"
```

---

## Task 8: 增加 Remote Session Provider、HTTP/HTTPS 安全传输和 Router 选择

**Files:**

- Modify: `backend/infra/sandbox/provider.go`
- Create: `backend/infra/sandbox/session_remote_provider.go`
- Create: `backend/infra/sandbox/session_remote_provider_test.go`
- Modify: `backend/infra/sandbox/endpoint_policy.go`
- Modify: `backend/infra/sandbox/endpoint_policy_test.go`
- Modify: `backend/infra/sandbox/remote_provider.go`
- Modify: `backend/infra/sandbox/remote_provider_test.go`
- Modify: `backend/application/sandbox/router.go`
- Modify: `backend/application/sandbox/router_test.go`
- Modify: `backend/application/sandbox_wiring.go`
- Modify: `backend/application/sandbox_wiring_test.go`

- [ ] **Step 1: 写 Remote Session client 失败测试**

测试实现统一 Go 合同到 Task 7 路由的映射：Acquire/Get/Release/Destroy/Recover、
Exec、Read/Write/List/Glob/Grep/Replace/Download、状态轮询、事件流和 cancel。每次请求：

- Provider ID/key 来自数据库 descriptor；
- Session v2 身份从授权 context 读取并重新计算 canonical digest；
- 使用 Provider credential + Session v2 签名头；
- endpoint path 只能追加到锁定 origin；
- 307/308、跨 origin、DNS 变化、响应过大、未知字段和坏 stream fail closed；
- 有副作用的 POST 不自动重试；
- 错误不泄露 endpoint、credential、Thread、UID、路径或上游正文。

- [ ] **Step 2: 写 HTTP/HTTPS endpoint policy 失败测试**

按照已批准设计，Remote Provider endpoint 可显式使用 HTTP 或 HTTPS，但两者都必须：

- 精确锁定 scheme/host/port；
- 每次拨号重新解析并拒绝 loopback、link-local、multicast、metadata、宿主机网关、Unix
  socket 和非 allowlist 地址；
- 禁止代理环境变量和重定向；
- HTTP 仍要求 credential、签名、nonce、防重放和来源防火墙；
- 只把 `transport_encrypted=false` 投影给系统管理，不在日志重复打印 URL；
- 没有显式 Provider HTTP 选择时不做 HTTPS -> HTTP downgrade。

现有 `safehttp` 是 HTTPS/loopback-debug 通用安全边界，不要全局放开。Sandbox 在
`endpoint_policy.go` 内实现专用 exact-origin transport，复用地址分类与 response limit，
其他 safehttp 调用方行为不变。

- [ ] **Step 3: 写 Router capability 失败测试**

- 只有健康 Provider 同时包含 `sandbox_session_v1` 和 `signed_session_context_v2` 才可
  `ResolveSession`；
- one-shot `Resolve` 的容量 lease 行为不变；
- Session Resolve 不长期占用旧 `RuntimePolicy.MaxConcurrency` lease，实际操作容量由
  Runner scheduler 管理；
- Provider disabled/unhealthy/feature missing 返回稳定错误；
- local_debug 只有 Task 10 门禁通过才实现 Session；
- `SelectedSessionProvider.Release` 只释放选择资源，不销毁业务 Session。

- [ ] **Step 4: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'RemoteSession|PlainHTTP|ResolveSession'
```

Expected: FAIL，因为 Session remote provider 和 router 入口尚不存在。

- [ ] **Step 5: 实现可选 `SessionRuntimeProvider`**

在 `provider.go` 新增可选接口，不给原 `RuntimeProvider` 增加方法：

```go
type SessionRuntimeProvider interface {
    SandboxSessionManager
}
```

`RemoteProvider` 组合一个 `remoteSessionManager`；只有构造配置中有 ProviderID、
Session signer 且健康 feature 支持时，Router 才暴露该接口。旧第三方 Provider 无需修改。

- [ ] **Step 6: 实现 `ProviderRouter.ResolveSession`**

复用 Provider lookup、默认选择、scope 权限、health freshness 和 factory，但使用独立、
短生命周期 resolve guard，不复用 one-shot execution capacity lease。每次 Session operation
仍由 Runner 的 signed identity、Redis admission 和权重槽位控制。

- [ ] **Step 7: 更新 wiring**

`configuredSandboxProviderFactory` 将数据库 `provider.ID` 和 `provider.ProviderKey` 放入
RemoteProvider 配置；加载 Session signer 和 v1 signer 使用同一 keyring material 时也要
生成不同 schema/domain separator。Session signer 未配置时 one-shot signed v1 仍可用，
但 Session feature fail closed。

- [ ] **Step 8: 运行 GREEN 和全部 Provider/Router 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'Remote|Endpoint|Router|Resolve|Session|HTTP'
```

- [ ] **Step 9: 提交 Remote Session Provider**

```bash
git add backend/infra/sandbox/provider.go backend/infra/sandbox/session_remote_provider.go backend/infra/sandbox/session_remote_provider_test.go backend/infra/sandbox/endpoint_policy.go backend/infra/sandbox/endpoint_policy_test.go backend/infra/sandbox/remote_provider.go backend/infra/sandbox/remote_provider_test.go backend/application/sandbox/router.go backend/application/sandbox/router_test.go backend/application/sandbox_wiring.go backend/application/sandbox_wiring_test.go
git commit -m "feat: route signed remote sandbox sessions"
```

---

## Task 9: 增加 Session 配置管理、运行投影和系统管理 UI

**Files:**

- Create: `backend/application/sandbox/session_settings_service.go`
- Create: `backend/application/sandbox/session_settings_service_test.go`
- Modify: `backend/application/sandbox/types.go`
- Modify: `backend/api/handler/coze/admin_sandbox.go`
- Modify: `backend/api/handler/coze/admin_sandbox_test.go`
- Modify: `backend/api/router/coze/admin_sandbox.go`
- Modify: `backend/api/router/coze/admin_sandbox_test.go`
- Modify: `backend/application/sandbox_wiring.go`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-service.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-session-card.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-session-card.module.less`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-session-card.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-management-section.test.tsx`

- [ ] **Step 1: 写后端 admin 合同失败测试**

新增：

- `GET /api/admin/sandboxes/session-settings`；
- `PUT /api/admin/sandboxes/session-settings`，必须 `expected_version`；
- `GET /api/admin/sandboxes/session-runtime-status`。

沿用当前 admin permission、严格 JSON、64 KiB body、trailing slash 404、CAS 409、脱敏
error envelope 和审计约束。更新成功时写独立审计 action：
`session_settings.update`/`session_settings.update_failed`，metadata 只含 previous/new
version 与 changed field names。

- [ ] **Step 2: 写 Runner 应用/失败语义测试**

Service 先事务保存 desired 配置和审计，再调用 Runner 的签名
`PUT /v1/session-configuration`：

- Runner 应用成功 -> `applied=true`；
- Runner unavailable -> 配置保留，`applied=false` + 稳定 reason；
- stale version -> 409，不覆盖；
- UID 范围等启动级字段变化 -> 422，要求维护窗口重启；
- Core enable 请求在 capability/identity/volume/generation 未 ready 时拒绝，不只保存 true；
- Interactive=true 在 Phase 1 拒绝；
- HostShell=true 但环境门禁不满足时保存 desired 可选，但 runtime 投影必须 unavailable，
  不能宣称已应用。

- [ ] **Step 3: 写前端卡片失败测试**

UI 测试：

- 初始显示 desired/applied version、Core/Interactive/Host Shell 三个开关；
- Interactive disabled 并标注“Phase 3 未启用”；
- 显示 generation、AIO/sessiond readiness、队列、active weight、Session 数和 transport
  加密状态；
- HTTP Provider 显示未加密风险，但不显示 endpoint；
- 保存前前端校验范围，后端错误仍是最终事实；
- stale version 自动刷新，不覆盖管理员刚加载的新值；
- Runner unavailable、generation recovering、capacity/identity unavailable 有明确中文状态；
- 不渲染 UID/GID、物理路径、上游 Shell ID、token 或 raw error；
- 原 `SandboxSchedulerCard` 与 Provider CRUD 测试不变。

- [ ] **Step 4: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./application/sandbox ./api/handler/coze ./api/router/coze -run 'SessionSettings|SessionRuntime'
```

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__/sandbox-session-card.test.tsx src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-management-section.test.tsx
```

Expected: FAIL，因为 admin routes 和卡片尚不存在。

- [ ] **Step 5: 实现后端 service/handler/router**

Service 使用现有 `SchedulerService` 的 actor、CAS、审计和 Runner client 风格，但仓储、
version 和 signed endpoint 独立。Runtime DTO 只含聚合：

```go
type SessionRuntimeStatusDTO struct {
    Available             bool   `json:"available"`
    DesiredConfigVersion  uint64 `json:"desired_config_version"`
    AppliedConfigVersion  uint64 `json:"applied_config_version"`
    RuntimeGeneration     uint64 `json:"runtime_generation"`
    CoreEnabled           bool   `json:"core_enabled"`
    InteractiveEnabled    bool   `json:"interactive_enabled"`
    HostShellAvailable    bool   `json:"host_shell_available"`
    QueueDepth            int    `json:"queue_depth"`
    ActiveWeight          int    `json:"active_weight"`
    TotalWeight           int    `json:"total_weight"`
    ActiveSessions        int    `json:"active_sessions"`
    IdleSessions          int    `json:"idle_sessions"`
    IdleShells            int    `json:"idle_shells"`
    TransportEncrypted    bool   `json:"transport_encrypted"`
    ReasonCode            string `json:"reason_code,omitempty"`
}
```

- [ ] **Step 6: 实现前端服务和卡片**

复用现有 card 样式与请求 envelope，不引入新 UI 库。数字输入使用与
`SandboxSchedulerCard` 一致的受控模式；所有按钮保留 loading、disabled、focus、ARIA、
error、refresh 和 readonly 状态。

- [ ] **Step 7: 运行 GREEN 与系统页回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./application/sandbox ./api/handler/coze ./api/router/coze -run 'Sandbox|SessionSettings|SessionRuntime'
```

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__/sandbox-session-card.test.tsx src/pages/system/__tests__/sandbox-scheduler-card.test.tsx src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-management-section.test.tsx src/pages/system/__tests__/sandbox-system-page.test.tsx
```

- [ ] **Step 8: 提交系统管理能力**

```bash
git add backend/application/sandbox/session_settings_service.go backend/application/sandbox/session_settings_service_test.go backend/application/sandbox/types.go backend/api/handler/coze/admin_sandbox.go backend/api/handler/coze/admin_sandbox_test.go backend/api/router/coze/admin_sandbox.go backend/api/router/coze/admin_sandbox_test.go backend/application/sandbox_wiring.go frontend/apps/coze-studio/src/pages/system/sandbox-service.ts frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-service.test.ts frontend/apps/coze-studio/src/pages/system/sandbox-session-card.tsx frontend/apps/coze-studio/src/pages/system/sandbox-session-card.module.less frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-session-card.test.tsx frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-management-section.test.tsx
git commit -m "feat: manage sandbox session runtime settings"
```

---

## Task 10: 实现仅本机 Debug 可用的 Host Shell Session

**Files:**

- Create: `backend/infra/sandbox/host_shell_session.go`
- Create: `backend/infra/sandbox/host_shell_session_test.go`
- Modify: `backend/infra/sandbox/local_debug_provider.go`
- Modify: `backend/infra/sandbox/local_debug_provider_test.go`
- Modify: `backend/application/sandbox_wiring.go`
- Modify: `backend/application/sandbox_wiring_test.go`
- Modify: `backend/application/sandbox/types.go`

- [ ] **Step 1: 写三重门禁失败测试**

Host Shell Session 必须同时满足：

```text
APP_ENV=debug
SANDBOX_HOST_SHELL_SESSION_ENABLED=true
SANDBOX_HOST_SHELL_GATEWAY_ADDR=127.0.0.1:8099 或 [::1]:8099
```

测试 production/test/空环境、非 loopback、`0.0.0.0`、公网地址、AIO/remote unavailable、
配置热更新误开启均返回 unavailable；不得复用旧 `APP_DEV_HOST_RUNTIME_ENABLED` 作为
Session 新开关，避免启用 AppDev 调试时意外开放完整 Host Shell。

- [ ] **Step 2: 写本机执行风险边界测试**

- 工作区仍使用 server-generated workspace key，不拼 Thread 名；
- 只以当前开发用户运行，不宣称 UID/GID 或 cgroup 隔离；
- 新进程组、deadline、输出上限、清理环境、cwd、cancel/kill group 生效；
- 文件路径规范化和逻辑目录边界生效，但 runtime status 必须
  `isolation_level=host_debug_unisolated`；
- 禁止 network/credential 假隔离文案；
- Host Shell 错误不包含本机绝对路径；
- `AIOBackend` 失败时 Router 不选择 HostShellBackend。

- [ ] **Step 3: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'HostShellSession|LocalDebug'
```

Expected: FAIL，因为 Host Shell Session 尚不存在。

- [ ] **Step 4: 实现同一 Core 合同的本机 Adapter**

`HostShellSessionManager` 实现 Task 2 的接口；只支持 Core 操作，不支持 interactive 或
publish。Session state 仅本机 debug 使用内存 map，但工作区文件保留；应用重启后 Session
需重新 Acquire，不恢复运行命令。所有进程以 `Setpgid` 启动，cancel 对负 PGID 发信号。

- [ ] **Step 5: 接入 Router 与管理投影**

`configuredSandboxProviderFactory` 仅在 local_debug Provider、三重门禁和 Session feature
同时满足时返回 Host Shell Session manager。系统管理页显示持续风险提示；远程环境该
feature 永不出现在 health。

- [ ] **Step 6: 运行 GREEN 与 fail-closed 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'HostShellSession|LocalDebug|ResolveSession'
```

- [ ] **Step 7: 提交 Host Shell Adapter**

```bash
git add backend/infra/sandbox/host_shell_session.go backend/infra/sandbox/host_shell_session_test.go backend/infra/sandbox/local_debug_provider.go backend/infra/sandbox/local_debug_provider_test.go backend/application/sandbox_wiring.go backend/application/sandbox_wiring_test.go backend/application/sandbox/types.go
git commit -m "feat: add gated debug host shell sessions"
```

---

## Task 11: 打包单常驻 AIO 容器并补齐 dev 发布/回滚合同

**Files:**

- Modify: `backend/Dockerfile.sandbox-runner`
- Modify: `deploy/dev/docker-compose.runner-2c4g.yml`
- Modify: `deploy/dev/deploy.sh`
- Modify: `deploy/dev/tests/compose_contract_test.sh`
- Modify: `deploy/dev/tests/deploy_test.sh`
- Modify: `deploy/dev/tests/image_contract_test.sh`
- Modify: `.github/workflows/deploy-dev.yml`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md`

- [ ] **Step 1: 写 Compose 失败合同**

测试 `coze-sandbox-aio`：

- 每个 Runner profile 恰好一个 service，不按用户/Thread 动态创建 Compose service；
- 使用构建后的 NewX AIO digest，禁止 `latest`；
- `mem_limit: 1536m`、`cpus: 1.25`、`pids_limit: 256`、`shm_size: 512m`；
- `DISABLE_BROWSER/JUPYTER/CODE_SERVER/MCP_BROWSER/VNC/NODEJS_REPL=true`；
- 只 `expose` 私网 `8080/8090`，没有 `ports`、host network、privileged、Docker socket；
- `restart: unless-stopped`，且 healthcheck 通过带 service token 的容器内检查，不从宿主机
  publish 探活；
- 持久 named volume 只挂 Thread 根目录，skills 只读；
- 上游要求的 `JWT_PUBLIC_KEY` 通过 AIO 专用 env 以 base64 公钥传入；公钥不是秘密，
  但不得把 JWT private key 放入 env；
- JWT private key、sessiond service token 和 grant keyring 通过权限 `0600` 的只读文件
  分别挂载到 Runner/AIO，不写进 Compose、镜像或通用 `app.env`；
- Runner 等待 AIO 与 sessiond 双健康，但 AIO failure 不停止 coze-server/web；
- Runner env file 不复用完整 `app.env`，只含 MySQL/Redis/AIO/签名必需值；
- Core 默认关闭，Interactive 和 Host Shell 在 dev remote profile 都为 false。

- [ ] **Step 2: 写镜像/Workflow 失败合同**

GitHub Actions 新增 `build-sandbox-aio`：

- 使用 `deploy/sandbox-runner/Dockerfile.aio`；
- revision label 等于 target SHA；
- buildx 构建当前部署所需平台；
- verify job 同时校验 server/web/runner/runtime/AIO 五个 immutable candidate；
- 只有全部成功才 promote `coze-sandbox-aio:dev`；
- AIO base digest 必须与 `aio.lock.json` 一致；
- 不把 JWT private key、grant key、DB/Redis secret 作为 build args；
- 旧部署没有启用 runner profile 时，AIO job 可跳过且 server/web 流程不变。

- [ ] **Step 3: 写 deploy/rollback 失败合同**

`deploy.sh` 必须：

- 拉取并核对 AIO candidate revision/digest；
- migration preflight 覆盖 `20260813000100`；`deploy.sh` 本身不 apply，最终仍只允许
  `publish-dev.sh` 在用户确认 exact SHA、实际部署区间和 Atlas status 后执行一次 forward
  apply；
- 验证 runner/aio secret 文件 owner/permission；
- 启动顺序 `aio -> runner -> server/web`；
- health 同时检查 AIO internal、sessiond、Runner Session projection；
- 成功记录 AIO image ref/id、runtime generation 和迁移区间；
- 回滚 tag 恢复旧 AIO/Runner，关闭 Session capability，保留 named volume 和两张表；
- rollback 不执行 `docker volume rm`、`down -v`、drop/truncate 或强制清理用户文件；
- AIO 首次上线失败时回到旧 runner/server/web，volume 可保留空目录。

- [ ] **Step 4: 运行合同确认 RED**

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
```

Expected: 至少 AIO 相关断言 FAIL。

- [ ] **Step 5: 更新镜像和 Compose**

`backend/Dockerfile.sandbox-runner` 保留非 root Runner 用户；现有 `/app/secrets` 已能承载
TLS、JWT 和 keyring 文件，只在镜像合同证明缺少目录时才做最小修改，不制造空 diff。
MySQL CA 也作为只读 secret file 挂载。不要把 sessiond 放入 Runner 镜像。
`coze-sandbox-aio` 使用单独 AIO 镜像，容器内 `sessiond` 的最小 UID/GID 切换 capability
由 Task 5 真实合同决定；禁止为了让测试通过加 `privileged` 或
`seccomp=unconfined`。

- [ ] **Step 6: 更新发布脚本与 Actions**

沿用现有 revision tracking、promotion 和 rollback 函数，增加 AIO 第五镜像，不复制一套
发布逻辑。成功记录同时包含：

```text
SANDBOX_AIO_IMAGE_REF
SANDBOX_AIO_IMAGE_ID
SANDBOX_AIO_BASE_DIGEST
SANDBOX_AIO_RUNTIME_GENERATION
```

- [ ] **Step 7: 写运维手册**

Runbook 必须写清：

- 本地 Debug Host Shell 与本地/共享 AIO 两种启动方式；
- dedicated `sandbox-runner.env` 和 secret 文件清单、权限、轮换；
- 迁移 status/apply 的职责与禁止重刷数据库；
- Core enable 顺序、generation、drain、重启、保留 volume、孤儿进程检查；
- `2C4G` 参数和不得临时提高硬限制的要求；
- HTTP Provider 的来源防火墙和未加密风险；
- 回滚关闭 capability 而不删数据；
- Phase 1 尚未接 Agent/Plugin/Interactive。

- [ ] **Step 8: 运行 GREEN 合同**

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
```

- [ ] **Step 9: 验证 Compose 渲染**

使用临时、无真实秘密的 fixture env 渲染：

```bash
docker compose -f deploy/dev/docker-compose.runner-2c4g.yml config --quiet
```

Expected: PASS；渲染结果无 `ports` for AIO，无 `latest`，且资源硬限制存在。

- [ ] **Step 10: 提交打包和运维合同**

```bash
git add backend/Dockerfile.sandbox-runner deploy/dev/docker-compose.runner-2c4g.yml deploy/dev/deploy.sh deploy/dev/tests/compose_contract_test.sh deploy/dev/tests/deploy_test.sh deploy/dev/tests/image_contract_test.sh .github/workflows/deploy-dev.yml docs/superpowers/runbooks/sandbox-control-plane-operations.md docs/superpowers/runbooks/local-debug-and-test.md
git commit -m "feat: package shared AIO sandbox runtime"
```

---

## Task 12: 完成真实 Core E2E、故障恢复和 `2C4G` 资源闸门

**Files:**

- Create: `backend/internal/sandboxrunner/aio_core_e2e_test.go`
- Create: `deploy/sandbox-runner/tests/aio_core_e2e_test.sh`
- Create: `deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh`
- Modify: `deploy/dev/tests/compose_contract_test.sh`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`

- [ ] **Step 1: 写可重复的 E2E harness**

Harness 使用临时 network、MySQL schema、Redis namespace、volume、JWT/grant keys 和
Provider，绝不连接或清空用户 dev 数据。每次测试生成唯一 deployment ID，trap 只删除
本轮已确认创建的容器/network/schema/Redis namespace；volume 在 persistence case 中跨
容器重启保留，测试结束后只删除本轮命名 volume。

- [ ] **Step 2: 验证 Session/文件完整矩阵**

真实 Runner + AIO + sessiond：

1. 获取同一用户 Thread A/B、另一用户 Thread C；
2. 确认三个稳定 Session、不同 UID/GID、不同 workspace key；
3. 两个 Core 并发运行，第三个 queued；
4. cwd、env、前后台命令、stdout/stderr、exit code、timeout；
5. read/write/append/list/glob/grep/replace/download；
6. `..`、软/硬链接、procfs、另 Thread 路径均拒绝；
7. cancel A 进程树，B/C 继续；
8. release 后文件保留、上游 Shell 释放；reacquire 重建 Shell；
9. destroy 默认不删 workspace；显式测试清理只作用本轮 fixture。

- [ ] **Step 3: 验证 generation 和故障恢复**

- 停止 AIO 容器，保持 volume；
- Runner Session capability 变 unavailable，不回退 Host Shell；
- 重建 AIO 后 generation 严格 `old+1`；
- 旧 Shell ID 不复用，Session Recover 建新上游资源；
- workspace marker 保留；
- 重启前 running operation 状态为 unknown/failed，需要 caller 明确重试，不能自动重放；
- accepted queue item 可从加密 Redis 记录恢复，operation ID 不变；
- Redis 丢失时 MySQL Session 可恢复但命令不重放；
- MySQL unavailable 时新 Acquire fail closed，已有 operation 不伪造成功。

- [ ] **Step 4: 验证 shared failure domain**

人为让 Session A 消耗输出、PID、磁盘和 CPU 到各自软限制，确认：

- A 被终止或拒绝；
- B 仍可完成小命令；
- 容器 cgroup 达硬边界前 Runner 停止新出队；
- PID/磁盘/输出没有无界增长；
- AIO crash 时所有 Session 正确投影为 recovering/unavailable，不返回 500 正文。

- [ ] **Step 5: 运行 2C4G soak**

至少 30 分钟混合执行两个 Core Session：短命令、1–32 MiB 文件、glob/grep、后台进程、
cancel/reacquire。每 5 秒采集容器 memory/CPU/PID、Runner RSS、queue depth、active weight、
进程和 fd 数，只写仓库外 evidence。

Soak 不是唯一的可重复开发测试：先运行 `--duration 2m` smoke，确认采集、清理和断言
稳定；合并闸门再运行 `30m`。CI 只跑 `2m` smoke，`30m` 结果由本轮 exact SHA 的本地/
dev 同构环境提供，避免每次单元 CI 被长压测拖死。

```bash
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 2m
```

```bash
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 30m
```

通过标准：

- AIO 不超过 1536 MiB/1.25 CPU/256 PID/512 MiB shm 硬限制；
- Runner 常驻目标约 128 MiB，绝对不超过 Compose 192 MiB；
- 无 OOMKilled、无持续增长的 PID/fd/session/shell；
- 队列有界且 drain 后归零；
- Host memory reserve 低于水位时停止出队；
- 测试结束不存在 probe 子进程或宿主机公开 AIO 端口。

- [ ] **Step 6: 执行 E2E 和 soak**

```bash
bash deploy/sandbox-runner/tests/aio_core_e2e_test.sh
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 30m
```

Expected: 全部通过。任何隔离逃逸、跨 Session cancel、OOM、自动重放或公开端口都属于
阻断缺陷，不能通过降低断言或提高资源上限放行。

- [ ] **Step 7: 记录运行手册中的实测水位**

只记录聚合最大/稳态内存、CPU、PID、Runner RSS、队列延迟和测试 revision，不记录用户
标识、命令、路径或 token。若实测 Core 无法在预算内稳定运行，保持 Core disabled，并
在计划结论标记 blocked，不进入 Phase 2。

- [ ] **Step 8: 提交 E2E 闸门**

```bash
git add backend/internal/sandboxrunner/aio_core_e2e_test.go deploy/sandbox-runner/tests/aio_core_e2e_test.sh deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh deploy/dev/tests/compose_contract_test.sh docs/superpowers/runbooks/sandbox-control-plane-operations.md
git commit -m "test: gate shared AIO core runtime on 2c4g"
```

---

## Task 13: 全量回归、页面验收和长期事实更新

**Files:**

- Modify only after all gates pass: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`
- Create after Phase 1 acceptance: `docs/superpowers/plans/2026-08-13-deerflow-aio-shared-sandbox-phase2-implementation.md`

- [ ] **Step 1: 运行后端相关全量测试**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./pkg/sandboxidentity ./infra/sandbox ./internal/aiosessiond ./internal/sandboxrunner ./application/sandbox ./api/handler/coze ./api/router/coze
```

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -race ./internal/aiosessiond ./internal/sandboxrunner
```

若任何 package 使用 Mockey，再按仓库要求补跑：

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -gcflags="all=-l -N" ./application/sandbox ./api/handler/coze
```

- [ ] **Step 2: 运行现有 Plugin/AppDev/MCP 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./application/plugin ./application/appdev ./application/agentthread ./infra/coderunner/... ./infra/appdev/...
```

Expected: 未切换流量的 one-shot Plugin、AppDev、MCP stdio 和 Agent 行为不变。这里运行
Agent 测试只是回归；本计划不修改其源码或执行图合同。

- [ ] **Step 3: 运行前端系统管理回归**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__
```

```bash
cd frontend/apps/coze-studio
rushx lint
```

该 project 当前没有独立 typecheck script；Vitest 编译相关 TS，`rushx lint` 做项目级
静态检查。若实施时 package scripts
新增正式 typecheck，再补跑正式命令，不能临时发明全仓 `tsc` 参数。

- [ ] **Step 4: 运行迁移和部署合同**

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate validate --dir file:///migrations
```

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/publish_dev_test.sh
```

- [ ] **Step 5: 在 in-app browser 做本地页面验收**

启动本地系统后使用系统管理员账号，访问：

```text
http://localhost:8080/system/sandbox
```

记录：URL、账号类型、Provider、Session 卡片 desired/applied version、Core 默认关闭、
Interactive disabled、Host Shell 门禁、generation、HTTP 风险、Runner/AIO unavailable 和
刷新/保存/冲突状态。浏览器控制台不得有错误，页面不得出现 UID/GID、物理路径或秘密。

随后在测试配置中启用 Core，验证状态变 ready；关闭 Core 后再次验证现有 Plugin 试运行
仍走 one-shot。Phase 1 没有面向业务的 Session 入口，不在聊天页制造伪验收。

- [ ] **Step 6: 检查代码影响面和用户文件**

使用 codebase-memory `detect_changes` 检查 Session、Runner、Provider、管理页调用方；再
回到真实 diff 核对：

```bash
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git status --short
git diff -- docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md
```

Expected: 没有 `backend/application/agentthread/**` 修改；用户原文档仍是未暂存且未被
Session commits 带入。

- [ ] **Step 7: 只在行为真实上线后更新长期事实**

`project-context.md` 更新为：

- Native Runner 新增默认关闭的 Shared AIO Core Session backend；
- 每个 Runner deployment 一个常驻 AIO，Session 按 Thread UID/GID；
- Plugin 仍 one-shot，Agent/Interactive 尚未迁移；
- remote Provider 可按配置使用 HTTP/HTTPS，HTTP 显示风险；
- Host Shell 仅显式本机 debug；
- 生产依赖缺失 fail closed。

同时删除或改写当前“生产与共享环境只通过 HTTPS remote provider 执行”的旧句，避免
同一长期事实文件同时声明 HTTPS-only 和 HTTP/HTTPS 均可；不得只在段尾追加相反结论。

设计文档把 Phase 1 标为已实施，仅在实际证据通过后填写 revision 和门槛结果；不要提前
把 Phase 2/3 写成已完成。

- [ ] **Step 8: 编写 Phase 2 计划，但不实施**

Phase 2 计划只覆盖 Agent/Subagent/Plugin/Skill/uploads/outputs/Artifact 迁移，必须先读
Workbench 执行链权威文件并安排 graph verify/build/verify-derived。Phase 2 开始前再次由
用户确认范围；本任务不修改业务链代码。

- [ ] **Step 9: 提交事实与 Phase 2 计划**

```bash
git add docs/superpowers/context/project-context.md docs/superpowers/runbooks/sandbox-control-plane-operations.md docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md docs/superpowers/plans/2026-08-13-deerflow-aio-shared-sandbox-phase2-implementation.md
git commit -m "docs: record shared AIO core runtime"
```

---

## Task 14: Phase 1 集成审计与交付选择

**Files:**

- Read: `docs/superpowers/runbooks/dev-integration-audit.md`
- Read: all Phase 1 commits and verification evidence
- No code changes unless audit finds an in-scope defect

- [ ] **Step 1: 建立 exact SHA 清单**

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --name-status origin/dev...HEAD
git status --short
```

确认迁移仅 `20260813000100_sandbox_shared_aio_core.sql`，AIO lock、镜像、Runner、UI、
runbook 和测试范围与本计划一致；用户未提交文档不在任何 commit 中。

- [ ] **Step 2: 运行本轮新鲜的最终验证**

重复 Task 13 的后端、前端、Atlas、deploy contract，并附 Task 12 的真实 E2E/soak
revision。不能复用实现中间的过期“曾经通过”输出。

- [ ] **Step 3: 审查安全与回滚不变量**

逐项确认：

- AIO/sessiond 无公开端口；
- 无 privileged/root Docker socket/seccomp unconfined；
- raw AIO 不能被控制面外调用；
- UID/GID 真实降权、目录 `0700`、openat2 fail closed；
- v1 one-shot wire/Redis 未变化；
- Session 默认关闭，Agent/Plugin 未切换；
- rollback 不删表/volume/工作区；
- HTTP 风险可见且 transport policy 仍阻止 SSRF/rebinding；
- 日志/错误/指标/前端无秘密与内部身份。

- [ ] **Step 4: 让用户确认 exact SHA 后再集成**

按照仓库规则，合并、migration apply、发布和远程分支操作需要用户针对本轮 exact SHA
和范围明确授权。未授权前只报告审计结果，不自行 merge/push/publish。

若用户批准合入 `dev`，按 `dev-integration-audit.md` 执行一次集成检查和 fast-forward；
发布前只做实际 revision、migration 区间、credential 权限和 Atlas status 预检，再按：

```bash
AUDITED_ORIGIN_DEV_SHA=$(git rev-parse origin/dev)
AUDITED_TARGET_DEV_SHA=$(git rev-parse dev)
deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

脚本成功后停止本地流程，不手工 `git push origin dev`，不重复远程部署。

---

## Phase 1 验收矩阵

| 维度 | 必测 | 阻断条件 |
| --- | --- | --- |
| 上游 | AIO 1.11.0 + SDK v0.0.5 真实 Shell/File/Cancel | 任一 Core API 不兼容 |
| Unix 隔离 | 非 root UID/GID、0700、跨 Thread 拒绝 | 共享 gem/root 或可逃逸 |
| Session | acquire/release/destroy/recover、idle shell | 串 Session 或资源泄漏 |
| 调度 | 两 Core、第三排队、与 one-shot 共权重 | 越权重、500、单用户占满 |
| 取消 | 排队取消、进程组 TERM/KILL | 杀错 Thread 或留孙进程 |
| 恢复 | generation、工作区保留、不重放 | generation 不变或重复副作用 |
| 资源 | 1536m/1.25 CPU/256 PID/512m shm | OOM、提上限、无界增长 |
| 传输 | HTTP/HTTPS exact origin、防 SSRF/rebind | downgrade、redirect、特殊地址 |
| 兼容 | v1 one-shot、Plugin/AppDev/MCP/Agent 回归 | 现有链行为变化 |
| 运维 | 默认关闭、健康、审计、rollback | 暴露端口、删表/volume/数据 |

## Phase 2/3 交接条件

- Phase 2 只有在本计划全部通过、用户确认后才写入 Agent/Plugin 业务链；
- Phase 2 必须同步 Workbench execution-chain markdown/JSON 和 Graphify 派生图；
- Phase 3 才创建 `sandbox_runtime_service_leases` 并实现 Browser/Jupyter/VSCode/
  Terminal/MCP/preview；
- Phase 3 Interactive 如无法在 `2C4G` 通过真实压测，只关闭 Interactive，不能提高
  AIO 硬上限挤占 Core 与 NewX 主服务；
- 无论后续采用容器池还是每租户容器，本计划的 SessionRef、身份 v2、generation 和
  Gateway grant 合同保持可迁移。
