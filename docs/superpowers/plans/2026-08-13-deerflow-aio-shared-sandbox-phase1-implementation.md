# DeerFlow/AIO Shared Sandbox Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development`
> (recommended) or `executing-plans` to implement this plan task-by-task. Steps
> use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在不改变现有 Plugin one-shot 执行链的前提下，为 NewX 增加可灰度的
`sandbox_session_v1` Core Session 后端：一个 Runner 部署只运行一个常驻 AIO
容器，按服务端空间、用户和 Thread 事实映射持久工作区和独立 Shell Session，并在最低
`2C4G` 主机上以全局权重 `2` 安全运行。

**Architecture:** NewX 控制面继续选择 Provider、签发身份和保存业务状态；Native
Runner 复用现有 Redis 持久公平队列和全局权重槽位，新增独立 Session 队列命名空间、
MySQL Session 元数据以及 AIO generation fencing。Runner 通过上游 Go SDK 直连官方
AIO `8080`，将逻辑路径映射到
`/mnt/user-data/<space_id>/<user_id>/<thread_id>/{workspace,uploads,outputs}`；该层只保证
服务端受控请求不会串 Thread，不承诺恶意 Shell 的跨目录硬隔离。本阶段只发布后端能力
和管理配置，不把 Agent、Subagent、Plugin、MCP 或 AppDev 业务流量切到 Session；第二、
三阶段另写实施计划。

**Tech Stack:** Go 1.24、Hertz/net/http、GORM/MySQL、Redis、
`github.com/agent-infra/sandbox-sdk-go` v0.0.5、`ghcr.io/agent-infra/sandbox:latest`、Linux
Shell Session、React 18、TypeScript、Vitest、Atlas Community 1.2.3、Docker Compose、
GitHub Actions。

**Execution prerequisite:** 代码实施前必须调用 `test-driven-development`；每个任务通过
后按提交点调用 `verification-before-completion`，并只做一次主线任务审核、修复和验收，
不增加开发中评审或第二轮审核。Task 12/13 页面和真实本地验收还必须
调用 `browser:control-in-app-browser`。准备合入时调用 `requesting-code-review` 和
`finishing-a-development-branch`。本计划已经在用户指定的当前修复分支编写，不另开
worktree；若实施者另行执行，则先按仓库规则使用 `using-git-worktrees` 建立隔离环境，
且不得带走本工作区的用户未提交改动。

**Approved design:**
`docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`

---

## 实施边界

### 本计划交付

- 使用真正的 GHCR AIO `latest` 并真实探测其与 Go SDK `v0.0.5` 的合同；
- 新增 `sandbox_session_v1`、Session 身份 v2 和 Core Session Go 合同；
- 新增唯一一张 Phase 1 表 `sandbox_runtime_sessions`，`sandbox_runtime_service_leases`
  留到 Phase 3；
- 从服务端 `space_id + user_id + thread_id` 直接派生稳定物理目录，不使用 `thread_key`、
  UID/GID、identity/workspace 表或客户端物理路径；
- 直接运行官方 AIO `latest`，通过 Go SDK 提供显式 Shell Session、受控 File API、取消、
  清理和 generation 恢复；
- 让 Session Core 操作和现有 one-shot 任务共享总权重 `2`，默认 Core 权重 `1`、
  单用户 active 上限 `1`；
- 增加只在 `APP_ENV=debug`、显式开关和 loopback 下可用的 Host Shell Session；
- 增加系统管理配置、Runner 状态、Compose、发布与回滚合同；
- 用真实容器完成两 Session 并发、逻辑路径映射、重启恢复和 `2C4G` 验收。

### 本计划明确不交付

- 不修改 `backend/application/agentthread/**`，不接入 Agent/Subagent、Skill、上传、
  Artifact 或对象存储；
- 不把 Plugin、MCP stdio 或 AppDev 从现有 one-shot Adapter 切到 Session；
- 不开放 Browser、Jupyter、VSCode、Terminal、MCP、VNC、CDP 或预览端口；
- 不创建 `sandbox_runtime_service_leases`；
- 不把 AIO `8080` 或任何交互端口发布到宿主机；
- 不实现 `thread_key`/HMAC 工作区、UID/GID 降权、`sessiond`、grant、`file-helper`、
  `openat2` 代理或派生 AIO 镜像；
- 不以 AIO 失败为理由回退 Host Shell；
- 不修改或提交工作区中用户已有的
  `docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md`。

### 单一所有者规则

- `deploy/dev/docker-compose.runner-2c4g.yml` 和 `deploy/dev/deploy.sh` 是 AIO 容器、
  私网、镜像与持久 volume 的唯一生命周期所有者；
- Runner 不通过 Docker socket 创建、删除或重启 AIO，只监督 raw AIO health、保留
  sentinel、管理 generation、Session 和队列；
- Runner 现有 rootless Docker socket 继续只服务 one-shot Adapter，不赋予 AIO 管理权；
- AIO restart policy（`unless-stopped`）恢复容器后，Runner 依据 reserved sentinel 做
  fencing，不能出现
  Compose 和 Runner 同时争抢同名容器的双控制面。

## 已确认的上游事实

- AIO 镜像直接使用 `ghcr.io/agent-infra/sandbox:latest`，不固定 tag、OCI digest 或平台
  manifest；每次部署和探针都先拉取当时的真实 `latest`。
- 当前实测 `latest` 自报 AIO `1.11.0`，入口是 `/opt/gem/run.sh`，公开端口是 `8080`，
  默认运行用户语义为 `gem:1000`；这些是本轮观测值，不转化为版本锁。
- 官方启动合同要求 `--security-opt seccomp=unconfined`。本地 ARM64 上
  `cryptography 49.0.0` 的 `_rust.abi3.so` 默认导入会以 SIGILL/132 退出；
  `OPENSSL_armcap=0` 后 AIO health、`/v1/ping` 和 `/v1/sandbox` 正常，因此部署必须保留
  该兼容环境变量，并将显式 seccomp 放宽作为已接受的上游风险记录。
- Go SDK 固定为 `github.com/agent-infra/sandbox-sdk-go v0.0.5`，客户端必须注入有
  deadline 的 `http.Client` 并设置 `WithMaxAttempts(1)`，禁止对有副作用请求自动重试。
- 上游 raw AIO 默认不要求认证；SDK Adapter 的 Bearer token 可选，配置 token 时仍只
  通过 `Authorization` header 发送，不把 token 放入 URL 或日志。AIO `8080` 必须仅在
  Runner 专用私网可达，不能把可选 Bearer 描述成默认认证。
- 上游显式 Shell Session、Shell 取消及 File API 可以用于生产 Core；Create 固定
  `exec_dir` 为物理 `workspace` 根，每次 Exec 只接受逻辑 `workspace` 及子目录并映射后
  发送，File Read/Write/Replace 强制 `sudo=false`，业务命令使用相对路径。
- async Session 的真实顺序是 Exec → View → Wait → Kill；Kill 后 Session 可能立即消失，
  也可能保留到 Cleanup，因此只对被 Kill 的 Session 接受 Cleanup 成功或 404，另一个
  活 Session 必须 Cleanup 成功。
- 上游仓库不包含可直接修改的完整 AIO 服务实现。本计划不 fork、不复制上游服务源码，
  也不构建派生 AIO 镜像。

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
- 上游 Shell ID、reserved sentinel ID、物理路径和凭据不进入公共响应、浏览器、普通
  日志或低基数指标；
- Redis、MySQL、AIO 或签名密钥任一不可用时 Session fail closed；
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
  "workspace_quota_mb": 2048
}
```

规则：`interactive_enabled` 在 Phase 1 永远校验为 `false`；`host_shell_enabled` 只是期望
配置，运行时仍需满足 debug/loopback 门禁；AIO endpoint 和持久卷不属于动态快照，
这些启动级配置变化必须重启并重新健康检查。`workspace_quota_mb` 在共享 AIO 中只作为
受控 File API 软准入水位，不得宣称能约束恶意 Shell 的直接磁盘写入。

## Phase 1 完成门槛

只有以下条件全部满足才可把计划标记完成：

1. AIO `latest`/SDK 真实探针、Session 单元/合同测试、现有 Sandbox/Plugin 回归全部通过；
2. 同一共享 AIO 容器内两个 Core Session 并发，第三个进入队列而非 500；
3. Adapter 将三个测试身份映射到各自
   `/mnt/user-data/<space>/<user>/<thread>/{workspace,uploads,outputs}`，受控 File API 不接受
   物理路径或逻辑逃逸；明确不把该结果当作恶意 Shell 硬隔离；
4. Shell Create/Exec 的 `exec_dir`、相对命令、严格目录校验与 File `sudo=false` 合同通过；
5. 同一 Shell 串行，取消只 Kill/Cleanup 当前 Thread Session，其他 Session 继续运行；
6. AIO 重启后 generation 增加、上游 Session 失效、持久工作区保留，命令不自动重放；
7. AIO 采用官网启动参数通过 `2C4G` Core 压测，宿主机和 NewX 主服务无 OOM、无资源
   泄漏；本阶段不要求 AIO 启动时配置 cgroup 资源参数；
8. Session capability 默认关闭，AIO `8080` 无宿主机公开端口；
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

- [x] **Step 1: 核对分支、基线和用户改动**

```bash
git status --short --branch
git rev-parse HEAD
git rev-parse dev
git rev-parse origin/dev
git diff -- docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md
```

执行基线已核对为 `779b68769fe13b739b15cceb53efa5475edb8358`，并在独立分支
`codex/deerflow-aio-shared-sandbox-phase1` 实施。每个 Task 记录完成后的 exact SHA；
`dev`/`origin/dev` 只作为最终集成基准，不在开发中 merge。用户已有 recovery design
不得复制、修改、stage 或 commit。

- [x] **Step 2: 建立本轮临时证据目录**

```bash
mktemp -d /private/tmp/newx-aio-phase1-evidence.XXXXXX
```

Expected: 返回仓库外目录。探针日志、容器 inspect、资源曲线和测试输出只写到该目录，
不提交运行时证据或秘密。

- [x] **Step 3: 记录初始回归基线**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./pkg/sandboxidentity ./infra/sandbox ./internal/sandboxrunner
```

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-scheduler-card.test.tsx
```

Task 0 已完成分支/基线/保护文件和相关 backend/frontend 回归核对；未把历史失败混入
Session 提交。

---

## Task 1: 固化 AIO/SDK API 合同并建立真实兼容探针

**Files:**

- Modify: `backend/go.mod`
- Modify: `backend/go.sum`
- Create: `backend/cmd/sandbox-aio-compat-probe/main.go`
- Create: `backend/internal/sandboxrunner/aio/upstream_client.go`
- Create: `backend/internal/sandboxrunner/aio/upstream_client_test.go`
- Create: `deploy/sandbox-runner/tests/aio_upstream_contract_test.sh`
- Create: `deploy/sandbox-runner/tests/aio_upstream_contract_script_test.sh`
- Modify after evidence: `docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`

- [x] **Step 1: 先写 SDK Adapter 失败测试**

测试必须用 `httptest.Server` 固定验证：

- base URL 只能是启动配置给定的私有 AIO origin；
- Authorization 可选；空 token 不发送 header，非空 token 使用 Bearer 且不在 URL 或日志中出现；
- `http.Client.Timeout` 有界，所有调用继承 context deadline；
- `WithMaxAttempts(1)`，500/timeout 不自动重放命令或文件写入；
- 上游错误只映射稳定 reason code，不回传正文；
- Create/Exec/View/Wait/Kill/Cleanup 和 File Read/Write/List/Glob/Grep/Replace 的字段
  与 v0.0.5 一致；
- 测试通过反射确认上游 Shell/File 请求没有 UID/GID 字段；这固定 Phase 1 只能依赖
  `id`、`exec_dir` 和 File path 做逻辑路由，不能宣称 UID/GID 或恶意命令隔离。

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

- [x] **Step 2: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/aio -run 'TestUpstream'
```

Expected: FAIL，因为 package 和 Adapter 尚不存在。

- [x] **Step 3: 引入唯一允许的上游 SDK 版本**

```bash
cd backend
go get github.com/agent-infra/sandbox-sdk-go@v0.0.5
go mod tidy
```

只允许 `v0.0.5`；检查 `go.mod`/`go.sum`，拒绝无关依赖升级。Adapter 创建客户端时使用：

```go
headers := make(http.Header)
if token != "" {
    headers.Set("Authorization", "Bearer "+token)
}
sdk := sandboxclient.NewClient(
    option.WithBaseURL(baseURL),
    option.WithHTTPClient(httpClient),
    option.WithHTTPHeader(headers),
    option.WithMaxAttempts(1),
)
```

实现不得调用 SDK README 中不存在于 v0.0.5 源码的 `option.WithToken`。

- [x] **Step 4: 写官网启动合同测试**

脚本合同固定 `ghcr.io/agent-infra/sandbox:latest`、官网要求的
`--security-opt seccomp=unconfined`、ARM64 兼容变量 `OPENSSL_armcap=0` 和随机
loopback 端口。不得固定 digest/manifest，不要求 JWT、资源参数或 `DISABLE_*`，也不得
使用 privileged、host network、宿主 Docker socket 或宿主 bind mount。

- [x] **Step 5: 实现真实容器探针 CLI 和脚本**

探针只要求环境变量中的私有 base URL；Bearer JWT 是可选兼容输入，官网默认模式为空。
执行以下固定序列：

1. 创建 `newx-probe-a`、`newx-probe-b` 两个显式 Shell Session；
2. 并发写入各自 cwd/env marker，交叉读取不得串扰；
3. 前台执行；后台按 Exec → View → Wait → Kill 验证，Kill 后从另一 Session 确认进程消失；
4. File Write/Read/List/Glob/Grep/Replace；
5. 取消 `sleep 30`，确认子进程消失；
6. 输出镜像自报版本、能力和耗时，不输出 token 或文件正文；
7. 删除 probe Session 和 probe 目录。

脚本按官网启动 `latest`，只把端口收窄到 loopback，并应用已验证的 ARM64 兼容变量：

```bash
docker run --detach --rm --name newx-aio-contract \
  --security-opt seccomp=unconfined \
  -e OPENSSL_armcap=0 \
  -p 127.0.0.1::8080 \
  ghcr.io/agent-infra/sandbox:latest
```

测试脚本用 trap 停止临时容器，不删除任何已有容器或 volume。

- [x] **Step 6: 运行探针与 GREEN 单测**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/aio -run 'TestUpstream'
```

```bash
bash deploy/sandbox-runner/tests/aio_upstream_contract_test.sh
```

Expected: SDK 合同与真实 AIO Core 探针通过；报告明确 `native_uid_gid=false`。如果显式
Session、取消或任一文件 API 失败，停止 Phase 1，不写绕过补丁。

- [x] **Step 7: 用真实证据修正文档认证名词**

将设计改为：raw AIO 默认无鉴权但只能处于 Runner 私网；SDK Bearer 可选；Runner 直接
使用 SDK，不增加独立代理协议。同时记录官网 seccomp 和 ARM64 `OPENSSL_armcap=0`
启动事实。

- [x] **Step 8: 提交上游合同**

```bash
git add backend/go.mod backend/go.sum backend/cmd/sandbox-aio-compat-probe backend/internal/sandboxrunner/aio deploy/sandbox-runner/tests/aio_upstream_contract_test.sh deploy/sandbox-runner/tests/aio_upstream_contract_script_test.sh docs/superpowers/plans/2026-08-13-deerflow-aio-shared-sandbox-phase1-implementation.md docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md
git commit -m "test: verify AIO core sandbox contract"
```

已提交为 `c7dd231e3afe88f465982cdd45202ad5dac7ee9f`；真实 official latest 探针 GREEN，
observed AIO 1.11.0 仅作本轮证据，不形成版本锁。

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

- [x] **Step 1: 先写 capability、状态机、输入边界失败测试**

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

- [x] **Step 2: 先写 Session 身份 v2 失败测试**

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

- [x] **Step 3: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox ./pkg/sandboxidentity -run 'Session|Feature'
```

Expected: FAIL，因为 Session 类型和 feature 尚不存在。

- [x] **Step 4: 实现最小 Domain 与统一 Go 合同**

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

- [x] **Step 5: 实现身份 v2，保持 v1 字节兼容**

新增 `SessionSigner`/`SessionVerifier` 或在 Keyring 上增加明确命名的方法；不得修改
现有 `Request`、v1 envelope、v1 header 和 canonical JSON。nonce 防重放接口由 Runner
注入 Redis store，单元测试使用内存 fake。

- [x] **Step 6: 运行 GREEN 与 v1 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox ./pkg/sandboxidentity
```

Expected: PASS；现有 v1 golden/签名测试输出不变。

- [x] **Step 7: 提交 Session 合同**

```bash
git add backend/domain/sandbox/entity.go backend/domain/sandbox/session.go backend/domain/sandbox/session_test.go backend/infra/sandbox/session.go backend/infra/sandbox/session_test.go backend/pkg/sandboxidentity/session_context.go backend/pkg/sandboxidentity/session_context_test.go backend/pkg/sandboxidentity/context_test.go
git commit -m "feat: add sandbox session core contract"
```

已提交为 `c8167aacd6381cfbd67966bebd0558c9858a7b47`；唯一一次主线审核发现的 trusted
env allowlist 与 recovering state 两项 Important 已 TDD 修复并验证，未二次审核。

---

## Task 3: 增加向后兼容的 Session 动态配置

**Files:**

- Create: `backend/domain/sandbox/session_settings.go`
- Create: `backend/domain/sandbox/session_settings_test.go`
- Modify: `backend/domain/sandbox/repository.go`
- Modify: `backend/infra/sandbox/mysql_models.go`
- Modify: `backend/infra/sandbox/mysql_scheduler_repository.go`
- Modify: `backend/infra/sandbox/mysql_scheduler_repository_test.go`
- Create: `backend/infra/sandbox/mysql_session_settings_repository.go`
- Create: `backend/infra/sandbox/mysql_session_settings_repository_test.go`
- Create: `backend/infra/sandbox/mysql_session_settings_audit_repository.go`
- Create: `backend/infra/sandbox/mysql_session_settings_audit_repository_test.go`

- [x] **Step 1: 以 TDD 冻结 13 字段快照和范围**

默认 JSON 与本文一致，拒绝部分快照、未知/重复 key、Interactive=true、权重/用户上限/
idle 数量/TTL/timeout/grace/workspace soft quota 越界。配置不包含 UID/GID、AIO 启动资源、
`thread_key`、endpoint、镜像版本或物理路径；`workspace_quota_mb` 只作受控 File API
软水位。

- [x] **Step 2: 保持旧 Scheduler 回滚兼容**

同一 singleton 行的旧 `settings_json/version/updated_at` 与新
`session_settings_json/session_settings_version/session_settings_updated_at` 完全独立：

- 旧 Get/Update 只投影和更新旧列；
- Session Get/Update 只投影和更新 Session 列，不触碰旧 version/JSON/`updated_at`；
- 两套 CAS 不制造伪冲突；新列缺失时 fail closed，不回退内存默认；
- production repository 不调用 AutoMigrate。

- [x] **Step 3: 复用现有审计表而不破坏 CHECK**

现有数据库 CHECK 只允许 `scheduler_settings.update/update_failed`，因此 Session 审计复用
这两个 action，并以固定第四键 `settings_domain=session` 区分；metadata 其余三键为
previous version、new version、排序后的 changed fields。成功 CAS 与 append-only 审计在
同一事务，并严格绑定 action、actor、版本和当前→目标真实差异；错配或 audit insert 失败
整笔回滚。

- [x] **Step 4: RED/GREEN、唯一一次主线审核与修复**

RED 先证明 Session symbols/repository/审计缺失；后续回归 RED 分别锁定 UID 字段应删除、
审计可伪造与 GORM 自动触碰 legacy `updated_at`。最终修复使用显式 Session columns，
并由事务内 snapshot 校验成功审计。唯一一次主线审核发现的两个 Important 均已 TDD 修复，
修复后未发起第二轮审核。

- [x] **Step 5: 新鲜验证并提交**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -race -count=1 ./domain/sandbox ./infra/sandbox -run 'SessionSettings|SchedulerMigration|SessionAndSchedulerCASUpdateDisjointColumns'
GOCACHE=/private/tmp/coze-go-build go test -count=1 ./domain/sandbox ./infra/sandbox
```

两条验证均通过；完整 infra 测试因 miniredis 需要 loopback 在批准的沙箱外运行。提交：

```text
caac563a1aa28d2be347e3dca25024e438c4051d feat: add sandbox session runtime settings
```

未 merge、push、apply migration 或发布。

## Task 4: 创建 additive 迁移和 MySQL Runtime Session 仓储

**Files:**

- Create: `docker/atlas/migrations/20260813000100_sandbox_shared_aio_core.sql`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Modify: `backend/domain/sandbox/session.go`
- Modify: `backend/domain/sandbox/session_test.go`
- Modify: `backend/infra/sandbox/mysql_models.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_migration_test.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_repository.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_repository_test.go`
- Create: `backend/infra/sandbox/mysql_runtime_session_integration_test.go`
- Modify: `backend/domain/sandbox/repository.go`

- [ ] **Step 1: 写迁移结构失败测试**

测试必须解析 migration/schema 并断言：

- 只允许对 `sandbox_scheduler_settings` 做 additive `ADD COLUMN`、对新增列做 backfill 后
  `MODIFY ... NOT NULL`、创建一张新表，以及对应的 `INSERT/UPDATE`；
- 没有 `DROP`、`TRUNCATE`、`RENAME`、删除列或重建现有表；
- Phase 1 只创建 `sandbox_runtime_sessions`；
- 不提前创建 `sandbox_runtime_service_leases`；
- 不创建 identity/workspace 表，不存在 UID/GID、`thread_key`、`workspace_key` 或物理路径列；
- runtime Session 的业务唯一键、状态/版本/时间列和索引完整；
- Session 配置列与旧 scheduler 列独立；
- `aio_runtime_generation` 初值为 `0`，只能由 Runner 在 reserved sentinel 缺失后事务递增；
- singleton 同时保存 session-enabled deployment owner 和当前 reserved sentinel ID；二者
  只用于 fencing，不进入公共响应或日志。

- [ ] **Step 2: 写仓储并发失败测试**

用 `sqlmock` 和真实 MySQL 集成测试覆盖：

- 两个进程并发 Acquire 同一业务键只得到一行和同一个 `session_id`；
- 两个不同 Thread 得到不同 Session，物理目录从各自行的 space/user/thread 派生而不落库；
- Session 唯一键为 deployment/provider/space/user/thread/profile；
- Session 绑定 `SANDBOX_RUNNER_DEPLOYMENT_ID`；Provider/space/user/thread/profile 任一
  不同都不能读到另一业务键；
- Acquire 幂等，CAS 状态转换拒绝 stale version；
- `CompareAndReplaceAIOSentinel` 锁定 singleton 行，只有 expected sentinel 与持久值相等时
  才原子替换 sentinel、generation `+1` 并标记旧 Session recovering；CAS 失败返回当前值；
- `RecoverableSessions` 只返回旧 generation 且非 destroyed 的行；
- MySQL 出错不回退内存仓储或未持久化 Session。

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
  ADD COLUMN `aio_runtime_generation` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `session_settings_updated_at`,
  ADD COLUMN `aio_runtime_deployment_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' AFTER `aio_runtime_generation`,
  ADD COLUMN `aio_runtime_sentinel_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' AFTER `aio_runtime_deployment_id`;

CREATE TABLE `sandbox_runtime_sessions` (
  `session_id` CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
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
  KEY `idx_sandbox_runtime_session_provider` (`provider_id`),
  KEY `idx_sandbox_runtime_session_recovery` (`deployment_id`,`runtime_generation`,`state`,`session_id`),
  KEY `idx_sandbox_runtime_session_expiry` (`deployment_id`,`state`,`expires_at`,`session_id`),
  CONSTRAINT `fk_sandbox_runtime_session_provider` FOREIGN KEY (`provider_id`) REFERENCES `sandbox_providers` (`id`),
  CONSTRAINT `chk_sandbox_runtime_session_profile` CHECK (`profile` IN ('core','interactive')),
  CONSTRAINT `chk_sandbox_runtime_session_state` CHECK (`state` IN ('active','recovering','released','destroyed')),
  CONSTRAINT `chk_sandbox_runtime_session_generation` CHECK (`runtime_generation` > 0),
  CONSTRAINT `chk_sandbox_runtime_session_version` CHECK (`version` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

在同一迁移中写入本文“默认 `2C4G` Session 配置”，再把
`session_settings_json` 改成 `NOT NULL`。不要使用 JSON 默认表达式，以兼容目标 MySQL。

- [ ] **Step 5: 实现事务分配和 generation fencing**

`MySQLRepository` 使用服务端生成的标准 UUID 字符串作为候选 `session_id`，在事务内按完整
业务键创建/读取 Session，唯一冲突后重新读取并返回 detached domain 值。物理目录不落库；
Adapter 只从已验证 Session 的正整数 `space_id`、
正整数 `user_id` 和通过现有安全标识校验的 `thread_id` 派生：

```text
/mnt/user-data/<space_id>/<user_id>/<thread_id>/workspace
/mnt/user-data/<space_id>/<user_id>/<thread_id>/uploads
/mnt/user-data/<space_id>/<user_id>/<thread_id>/outputs
```

由于不再有自报 boot ID，Task 6 使用保留的上游 Shell Session 作为 AIO 进程生命周期
哨兵。仓储实现
`CompareAndReplaceAIOSentinel(deploymentID, expectedID, candidateID)`：在同一 singleton
行上 `SELECT ... FOR UPDATE`，先绑定唯一 session-enabled deployment；若 owner 不同则
fail closed。只有持久 sentinel 等于 `expectedID` 时，才原子写入 `candidateID`、generation
严格 `+1` 并将旧 generation Session 标记 recovering；否则返回当前 sentinel/generation
且不递增。Runner 只在上游 ListSessions 明确证明 expected sentinel 不存在后调用该方法，
CAS 失败后重新探测 winner sentinel，并 best-effort Cleanup 自己的候选哨兵。

首次启用时 expected sentinel 为空，成功写入 candidate 后 generation 从 `0` 变为 `1`。
Runner 重启但同一 AIO 中 sentinel 仍存在时不写数据库、不递增。不得用 Redis INCR、Unix
时间、容器 ID、进程启动次数或 Runner 重启替代。Task 2 的 `RuntimeSession.IdentityID` 在
本 Task 删除；目录直接由 `SessionRef.Key` 派生，不用虚构 workspace identity。

- [ ] **Step 6: 更新 Atlas schema 和 hash**

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate hash --dir file:///migrations
```

手工同步 `docker/atlas/opencoze_latest_schema.hcl`，显式写入 Provider foreign key、状态/
Profile/version/generation checks 和全部索引；不得使用数据库 dump 覆盖无关 schema。

- [ ] **Step 7: 运行 Atlas 与仓储验证**

```bash
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate validate --dir file:///migrations
```

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./infra/sandbox -run 'RuntimeSession|SessionSettings|SchedulerMigration'
```

Expected: Atlas hash/validate 和仓储测试通过。集成测试需要显式测试 DSN；没有 DSN 时只
允许 skip，并在 Phase 1 最终闸门直接使用现有 dev 数据库补跑，不得把 skip 当通过。本
计划不启动独立本地 MySQL/OceanBase 容器、不创建或删除临时 schema；SQLite/sqlmock 只
服务单元测试，真实迁移/仓储集成使用 dev 数据库且不输出 DSN/credential。

- [ ] **Step 8: 完成唯一一次 Task 4 主线审核并提交**

综合审核只检查 additive/rollback 安全、Session 业务唯一键与租户绑定、CAS 线性化、旧
Scheduler 列不被 Session 更新污染、dev 数据保护和错误脱敏。修复发现项后重新运行 Step 7，
不发起第二轮审核。

```bash
git add docker/atlas/migrations/20260813000100_sandbox_shared_aio_core.sql docker/atlas/migrations/atlas.sum docker/atlas/opencoze_latest_schema.hcl backend/domain/sandbox/repository.go backend/domain/sandbox/session.go backend/domain/sandbox/session_test.go backend/infra/sandbox/mysql_models.go backend/infra/sandbox/mysql_runtime_session_migration_test.go backend/infra/sandbox/mysql_runtime_session_repository.go backend/infra/sandbox/mysql_runtime_session_repository_test.go backend/infra/sandbox/mysql_runtime_session_integration_test.go
git commit -m "feat: persist shared AIO runtime sessions"
```

---

## Task 5: 实现 AIO 工作区映射和 Core Session Adapter

**Files:**

- Create: `backend/internal/sandboxrunner/aio/workspace.go`
- Create: `backend/internal/sandboxrunner/aio/workspace_test.go`
- Create: `backend/internal/sandboxrunner/aio/session_adapter.go`
- Create: `backend/internal/sandboxrunner/aio/session_adapter_test.go`
- Modify: `backend/internal/sandboxrunner/aio/upstream_client.go`
- Modify: `backend/internal/sandboxrunner/aio/upstream_client_test.go`
- Modify: `backend/cmd/sandbox-aio-compat-probe/main.go`
- Modify: `backend/cmd/sandbox-aio-compat-probe/main_test.go`
- Modify: `backend/infra/sandbox/session.go`
- Modify: `backend/infra/sandbox/session_test.go`
- Create: `deploy/sandbox-runner/tests/aio_workspace_contract_test.sh`

- [ ] **Step 1: 写工作区映射失败测试**

测试以已规范化的 `SessionKey` 为唯一身份输入，固定验证：

- 根目录严格等于 `/mnt/user-data/<space_id>/<user_id>/<thread_id>`；
- `space_id`、`user_id` 使用正整数规范十进制，`thread_id` 使用服务端 Thread 事实并再次
  通过现有安全标识校验；
- Profile、runtime generation、operation ID 和上游 Shell ID 不进入物理目录；
- 逻辑 `/mnt/user-data/workspace|uploads|outputs[/...]` 映射到当前 Thread 同名子目录；
- `/mnt/skills[/...]` 只允许 Read/List/Glob/Grep/Download，不允许 Write/Replace；
- 调用方提交物理路径、其他 Thread 路径、NUL、反斜杠、`..`、重复斜杠、控制字符、
  非规范 UTF-8 或超长值全部返回 `ErrInvalidInput`；
- 上游响应中的物理路径只能反向映射为当前 Session 的逻辑路径，出现 sibling、父目录或
  未知根立即 fail closed；String/错误和指标不得泄露物理路径。

增加 fuzz property：任意被 resolver 接受的写路径都必须满足
`physical == threadRoot/subroot || strings.HasPrefix(physical, threadRoot/subroot+"/")`。

- [ ] **Step 2: 写 SDK 请求映射失败测试**

用 fake upstream client 捕获请求并覆盖：

- Prepare 使用 Runner 保留的控制 Shell 在 `/mnt/user-data` 下只创建当前
  `<space>/<user>/<thread>/{workspace,uploads,outputs}`；所有命令段来自已规范化标识并做
  固定 shell quoting，不接受自由文本路径；
- Create 使用服务端生成的 opaque upstream ID，`exec_dir` 为物理 workspace 根，
  `preserve_symlinks=false`；
- Exec 只接受 `/mnt/user-data/workspace` 及其子目录作为逻辑 CWD，逐次映射为物理
  `exec_dir`，并显式 `strict=true`、`preserve_symlinks=false`、`hard_timeout`；
- 命令和 argv 中的绝对路径不做字符串替换，Phase 1 调用方必须使用相对路径；
- 同一 upstream Shell 同时只允许一个 operation；另一请求保持排队或返回稳定容量状态，
  Kill 不得误伤同一 Shell 的另一 operation；
- Read/Write/Replace 显式 `sudo=false`，所有 File 请求只携带 resolver 生成的物理路径；
- List/Glob/Grep/Read/Write 的响应路径全部反向映射，丢弃上游 message/hint/raw body；
- context cancel 只 Kill 当前 upstream Shell，不自动重放 Exec/Write/Replace。

- [ ] **Step 3: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/aio -run 'Workspace|SessionAdapter'
```

Expected: FAIL，因为 mapper 和 Session Adapter 尚不存在。

- [ ] **Step 4: 实现最小 mapper 和 Adapter**

`WorkspaceMapper` 只接受 domain 已规范化的 `SessionRef`，不接受裸路径根或客户端 identity。
`SessionAdapter` 位于 `UpstreamClient` 上层并实现 Task 2 的 `SandboxSession`；业务层不得直接
构造上游 SDK request。上游 Shell ID 由 Runner 用加密安全随机数生成并原子绑定到当前
Runtime Session；不得拼接 Session/租户事实，但物理目录始终只由 space/user/thread 派生。

Prepare、Create、Exec 和 File 操作任一返回不符合合同的响应时，映射为稳定 reason code；
不回退到 AIO 默认 cwd、全局 Shell Session、Host Shell 或未经映射的 File path。

- [ ] **Step 5: 增强真实上游探针**

`aio_workspace_contract_test.sh` 继续使用官方 `latest`、官网 seccomp、`OPENSSL_armcap=0` 和
临时 loopback probe port，验证：

1. 两个不同 space/user/thread 得到不同三层物理根和独立 Shell ID；
2. 相对命令的 `pwd` 与文件落点符合各自 workspace；
3. 每类 File 请求实际使用映射后的路径且 `sudo=false`；
4. 取消 A 不影响 B；Cleanup 后持久目录仍在；
5. 记录一个恶意 Shell 可主动使用绝对路径访问共享容器的已接受风险，不把此探针伪装成
   Unix 多租户隔离证明；
6. trap 只删除本轮 Session、probe 目录和临时容器，不清任何既有 volume/数据。

- [ ] **Step 6: 运行 GREEN 和回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/aio ./infra/sandbox -run 'Workspace|SessionAdapter|Session'
```

```bash
bash deploy/sandbox-runner/tests/aio_workspace_contract_test.sh
```

Expected: 新合同通过；若上游 strict cwd、显式 Session、File `sudo=false` 或响应逆映射无法
成立，立即停止，不通过放宽 resolver 或暴露物理路径绕过。

- [ ] **Step 7: 完成唯一一次 Task 5 主线审核并提交**

综合审核只检查本 Task 的 SDK 映射、路径边界、取消粒度、响应脱敏和真实探针。修复发现项
后重新运行 Step 6，不发起第二轮审核。

```bash
git add backend/internal/sandboxrunner/aio backend/cmd/sandbox-aio-compat-probe backend/infra/sandbox/session.go backend/infra/sandbox/session_test.go deploy/sandbox-runner/tests/aio_workspace_contract_test.sh
git commit -m "feat: map shared AIO session workspaces"
```

---

## Task 6: 实现 Runner raw AIO 健康监督、reserved sentinel 与 generation CAS

**Files:**

- Modify: `backend/internal/sandboxrunner/config.go`
- Modify: `backend/internal/sandboxrunner/config_test.go`
- Create: `backend/internal/sandboxrunner/aio/lifecycle.go`
- Create: `backend/internal/sandboxrunner/aio/lifecycle_test.go`
- Modify: `backend/internal/sandboxrunner/aio/upstream_client.go`
- Modify: `backend/internal/sandboxrunner/aio/upstream_client_test.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition_test.go`
- Modify: `backend/internal/sandboxrunner/runtime_status.go`
- Modify: `backend/internal/sandboxrunner/runtime_status_test.go`

- [ ] **Step 1: 写配置与 fail-closed 失败测试**

Session backend 关闭时只保留既有 one-shot readiness，raw AIO 不可达不能影响 one-shot。
开启时必须同时存在合法的 MySQL、Redis、签名/Provider 配置和
`SANDBOX_RUNNER_AIO_UPSTREAM_URL`。生产/共享环境只接受部署层私网地址；local debug
才允许 loopback HTTP。URL 必须精确限制 scheme/host/port，拒绝 userinfo、query、fragment、
redirect 和环境代理。

配置不得要求 internal `8090`、service token、grant key、image revision、container name 或
Docker socket。官方 `latest` 不做版本锁定，image digest/container ID 不参与 readiness 或
generation。

- [ ] **Step 2: 写 raw health 与 sentinel 状态机失败测试**

reserved sentinel 是 Runner 专用的上游 Shell Session，ID 为
`newx-generation-<32-lower-hex>`，`exec_dir=/mnt/user-data`。它不是业务 Session，不进入
调度、公共 API、idle cleanup、用户计数或日志字段。用 fake raw AIO 与事务仓储覆盖：

- raw health timeout、5xx、畸形响应或 transport error 为 unknown，不改 sentinel/generation；
- DB sentinel 在成功 `ListSessions` 中存在时，Runner 重启保持 generation 不变；
- 成功列表确认旧 sentinel 缺失时，先创建并确认 candidate，再做 MySQL CAS；
- 初始 sentinel 为空也走同一 CAS，generation 从 `0` 增到 `1`；
- CAS winner 只把 generation 增加一次，并把旧 generation Session 标为 `recovering`；
- CAS loser 只清理自己的 candidate，重读并采用 winner，不重复增加 generation；
- candidate cleanup 失败只报告稳定内部错误，不得覆盖或清理 winner；
- raw AIO 丢失时 Session unavailable，等待 deploy/Compose 恢复，不重放 running operation；
- Runner 不 create/start/stop/restart/remove AIO 容器。

- [ ] **Step 3: 写 raw SDK/HTTP 客户端失败测试**

只覆盖 Task 0/1 真实探针已经证明的 health、Shell `ListSessions`、Create、View 和 Cleanup
响应。明确 missing 必须来自成功列表缺失或稳定 not-found；timeout/5xx/解码错误都不能
当成 missing。禁止 redirect/环境代理，限制响应和 deadline，错误只映射稳定 reason，
不记录 upstream body、sentinel ID、物理路径或凭据。

- [ ] **Step 4: 运行测试确认 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/... -run 'AIO|RawAIO|Sentinel|Generation|SessionConfig|RuntimeStatus'
```

Expected: FAIL，因为 raw health、sentinel 监督与 generation CAS 编排尚不存在。

- [ ] **Step 5: 实现最小 raw AIO lifecycle supervisor**

执行顺序固定为：

```text
raw health -> read DB sentinel/generation -> confirm present or missing
-> create candidate -> verify candidate -> CAS expected sentinel
-> publish new generation -> mark old sessions recovering
```

候选 ID 使用 crypto-random 16 bytes 编码；sentinel ID 不是 credential，但仍不对外投影。
CAS 在一个 MySQL transaction 内锁定 singleton row、比较 expected sentinel、写 candidate、
generation `+1` 并返回新 generation。单个 DB schema 只允许一个 session-enabled deployment
拥有此 singleton；ownership 不匹配时 fail closed。Runner 只更新 DB/readiness/内存状态，
不依赖 Docker 元数据。

- [ ] **Step 6: 接入 runtime composition 与降级合同**

Core ready 条件固定为：开关开启、配置合法、MySQL/Redis 可用、raw AIO health 通过、
sentinel/generation 达到确定状态。任一 unknown 时 Core fail closed；one-shot 仍可用。
generation 变化只把旧 Session 标为 `recovering`，不自动重放 operation，也不删除 workspace。
reserved sentinel 永久排除业务 acquire、idle eviction、quota 和对外统计。

- [ ] **Step 7: 接入 `NewProcessRuntime` 的 MySQL 依赖**

只在 Session 开关为 true 时调用 `mysql.New()` 和
`infrasandbox.NewMySQLRepository(db)`。Runner 专用 secret env 只包含必要的 MySQL、
Redis、raw AIO 和签名配置，不加载完整 `app.env`。初始化失败时 Core fail closed，但旧
one-shot Runtime 仍按原配置运行。`Runtime` 负责关闭新增 SQL pool；构造中途失败也不能
泄漏连接。

- [ ] **Step 8: 运行 GREEN、旧 Runtime 回归与真实 sentinel 探针**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner/... ./infra/sandbox -run 'AIO|RawAIO|Sentinel|Generation|SessionConfig|RuntimeStatus|Runtime'
```

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner -run 'Lifecycle|WorkloadDispatcher|Server|E2E'
```

使用 deploy/Compose 已启动的 official latest AIO 验证 health、创建 reserved shell、
list/view、Runner 风格复查和 cleanup。若上游合同不符，立即停止并报告脱敏 status code、
response shape 与容器日志；不增加猜测兼容。

- [ ] **Step 9: 完成唯一一次 Task 6 主线审核并提交**

综合审核只检查 Runner 无 Docker 生命周期、missing/unknown 区分、CAS 线性化、sentinel
不泄漏/不参与调度、one-shot 降级。修复后重跑本 Task 全部验证，不发起第二轮审核。

```bash
git add backend/internal/sandboxrunner/config.go backend/internal/sandboxrunner/config_test.go backend/internal/sandboxrunner/aio backend/internal/sandboxrunner/runtime_composition.go backend/internal/sandboxrunner/runtime_composition_test.go backend/internal/sandboxrunner/runtime_status.go backend/internal/sandboxrunner/runtime_status_test.go
git commit -m "feat: supervise shared AIO generation"
```

---

## Task 7: 实现 direct SDK Core Session dispatcher、同 Shell 串行与取消

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
- Modify: `backend/internal/sandboxrunner/runtime_status_test.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition.go`
- Modify: `backend/internal/sandboxrunner/runtime_composition_test.go`
- Create: `backend/internal/sandboxrunner/session_e2e_test.go`

Runner 通过 Task 5 的 adapter 直连 raw AIO SDK/`8080`。每个 Runtime Session 只保存
opaque upstream Shell ID 与 generation；workspace 物理根始终由服务端
`space_id/user_id/thread_id` 派生。本 Task 不引入中间代理、service token、UID/GID、
`thread_key`、派生镜像或 Docker 生命周期管理。

- [ ] **Step 1: 写 Session store、队列恢复与公平调度 RED tests**

覆盖：

- acquire 的 v2 identity 精确绑定 deployment/provider/space/user/thread/profile；客户端不能
  提供或覆盖物理路径；
- 相同 canonical identity 幂等 acquire，并发 acquire 最多绑定一个 active upstream Shell；
- row generation 必须等于 Task 6 当前值，旧 generation 只进入 `recovering`；
- Core 使用独立 Redis namespace，不污染 one-shot key；accepted/queued 可恢复，running 不重放；
- 两个 Core 占满总权重后第三个排队；per-user active limit 生效；
- queued cancel 原子移除；payload 加密、TTL/长度有界，key/value/log 不保存命令正文、File
  body、物理路径或 upstream Shell ID。

- [ ] **Step 2: 写同 upstream Shell 串行与 fencing RED tests**

同一 Runtime Session 的 operation、release、destroy、recover 必须跨 Runner 副本串行，
不能只靠进程内 mutex。使用 Redis 原子 claim/lease，key 绑定 deployment + runtime session，
value 是随机 owner token，具备 TTL、续租与 compare-owner release。测试：

- 同一 Shell 任意时刻最多一个 upstream operation，不同 Shell 可按全局/用户容量并行；
- lease 丢失、续租失败或 owner 不匹配时 fail closed，不能启动第二调用或写成功终态；
- crash 后 running operation 变 unknown/recovering，绝不自动重放；
- late completion 不能覆盖已经确定的 canceled/unknown；
- release/destroy/recover 与 operation 争同一 lease。

- [ ] **Step 3: 写 direct SDK dispatch、路径与取消 RED tests**

dispatcher 在 upstream 调用前重新读 MySQL Session，并严格验证签名 identity、state、
generation 与 opaque Shell ID。首次 acquire 用 Task 5 派生的真实
`/mnt/user-data/<space>/<user>/<thread>/workspace` 创建 Shell；`uploads/outputs` 同步
准备，但不作 Shell cwd。

Exec 只接受逻辑 `/mnt/user-data/workspace` 及子目录，并逐次映射到真实 workspace；
拒绝客户端物理路径、uploads/outputs/skills cwd 与逃逸。command/argv 不重写，Phase 1
调用方使用相对路径。File API 在验证 Session 后映射逻辑路径，mutation 固定
`sudo=false`；structured response 逆映射，越界结果拒绝。Shell stdout/stderr 属用户程序
输出，不宣称可改写其中出现的物理文本。

按 Task 0/1 已证实的 async Exec + View/Wait/Kill 实现 running Exec cancel，只能 Kill row
中的同一 Shell。queued cancel 不调用 upstream；running File 只能 cancel HTTP context，
不能误 Kill Shell。不确定结果标 unknown/recovering。release cleanup Shell 但保留 workspace；
destroy 默认不删 workspace；recover 创建新 opaque Shell、复用同一 workspace、写当前
generation，不恢复旧 operation。

这些检查是服务端逻辑路由，不是 chroot/Unix 对抗性隔离；测试不得声称恶意 Shell 使用
绝对路径或 `..` 时一定无法访问 sibling。

- [ ] **Step 4: 写 Runner private HTTP/NDJSON 协议 RED tests**

冻结以下 private routes；不增加统一 Go 合同中不存在的 renew 或 publish 伪接口：

| Method | Path | 语义 |
| --- | --- | --- |
| `POST` | `/v1/sessions:acquire` | 幂等创建/取得稳定 Session |
| `GET` | `/v1/sessions/{session_id}` | 安全状态投影 |
| `POST` | `/v1/sessions/{session_id}:release` | 释放上游 Shell，保留工作区 |
| `POST` | `/v1/sessions/{session_id}:destroy` | 销毁运行资源，默认不删工作区 |
| `POST` | `/v1/sessions/{session_id}:recover` | generation 校验后重建 Session |
| `POST` | `/v1/sessions/{session_id}/operations` | 提交 `exec/read/write/append/list/glob/grep/replace/download` |
| `GET` | `/v1/sessions/{session_id}/operations/{operation_id}` | 状态和有界结果 |
| `GET` | `/v1/sessions/{session_id}/operations/{operation_id}/events` | 有界 NDJSON 事件流 |
| `POST` | `/v1/sessions/{session_id}/operations/{operation_id}:cancel` | 排队或运行取消 |
| `GET` | `/v1/session-configuration` | Session 配置快照 |
| `PUT` | `/v1/session-configuration` | 签名 CAS 应用配置 |

所有请求校验 Session v2 issuer/audience/expiry/nonce 及完整身份；
method、content-type、body/event/response 大小、deadline 有界；错误不回显 credential、
签名、Shell ID、物理 workspace、Redis metadata、DSN 或 upstream body。event resume
只恢复持久的 bounded accepted/terminal events，不重放 running operation。Core 未 ready
时 fail closed，既有 `/v1/executions` 行为不变。

- [ ] **Step 5: 运行 RED tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner -run 'Session|Core|Scheduler|Redis|Dispatcher|Operation|Cancel|Recover|RuntimeStatus' -count=1
```

Expected: FAIL，因为 Session store、协议、dispatcher 和跨副本 lease 尚未实现。

- [ ] **Step 6: 实现 store、scheduler、lease 与 dispatcher**

固定执行序列：

```text
authenticate -> load Session -> verify identity/state/generation
-> claim same-session lease -> map logical path/cwd -> direct SDK call
-> bound/reverse-map result -> conditional terminal write -> owner-fenced release
```

MySQL 只使用 Task 4 additive schema 与乐观 state/version；不 AutoMigrate，不创建第二张
workspace/identity 表。Redis 只保存有界加密 operation metadata 与随机 lease owner。
acquire 竞争 loser 清理自己创建的 Shell，不能清理 winner。非幂等 Exec/Write/Replace
遇到不确定 transport 结果不自动 retry。进程内锁只能作第二层保护，不能代替 Redis fencing。

- [ ] **Step 7: 接入 runtime composition 与 E2E**

Core readiness 只依赖：显式开关、合法配置、MySQL、Redis、Task 6 raw health 和已确认的
sentinel/generation。E2E 覆盖 acquire -> 同 Shell 串行 exec/file -> cancel -> release；
Runner 重启 queued 恢复/running 不重放；sentinel replacement 后 explicit recover 复用
workspace；File `sudo=false` 和路径正反映射；Core down 时 one-shot 仍可用；多 Runner
竞争只有一个 owner/terminal write。

- [ ] **Step 8: 运行 GREEN、race 与真实 AIO 链路**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner -run 'Session|Core|Scheduler|Redis|Dispatcher|Operation|Cancel|Recover|RuntimeStatus|V1Golden' -count=1
GOCACHE=/private/tmp/coze-go-build go test -race ./internal/sandboxrunner -run 'Session|Scheduler|Lease' -count=1
```

使用 deploy/Compose 已启动的 official latest raw AIO 验证同一 Shell 两次相对路径 Exec、
逻辑子目录 cwd 映射、File `sudo=false`、async cancel/Kill 与 cleanup。若上游行为与
Task 0/1 合同冲突，立即停止并报告脱敏证据，不增加中间代理绕过。

- [ ] **Step 9: 完成唯一一次 Task 7 主线审核并提交**

综合审核只检查身份绑定、跨副本同-Shell 串行/fencing、取消目标、generation 恢复、路径
映射、bounded payload、one-shot 回归与诚实隔离边界。修复后重跑全部验证，不发起第二轮
审核。

```bash
git add backend/internal/sandboxrunner
git commit -m "feat: dispatch shared AIO core sessions"
```

不要 merge、push、发布或切业务流量；提交后自动继续 Task 8。

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

Remote Session Provider 只把统一 Go Session 合同映射到 Task 7 Runner private routes；业务端
不得直连 raw AIO。本 Task 不修改 `backend/application/agentthread/**`，不切
Agent/Subagent/Plugin/MCP/AppDev 流量。

- [ ] **Step 1: 写 Remote Session client RED tests**

覆盖 Acquire/Get/Release/Destroy/Recover、Exec、Read/Write/List/Glob/Grep/
Replace/Download、operation status/events/cancel 的完整映射：

- Provider ID/key 来自数据库 descriptor；space/user/thread/profile 来自服务端授权 context
  与 Thread 事实，客户端不能覆盖；
- 使用 Provider credential 与 Session v2 签名；v1 one-shot header/body/golden 不变；
- path 只能追加到 exact origin；redirect、跨 origin、DNS rebinding、环境代理、超大响应、
  坏 NDJSON/未知字段按冻结合同 fail closed；
- 非幂等 POST 不自动 retry；不确定 transport 结果不得重发 Exec/Write/Replace；
- cancel 只使用返回的 operation ID；Remote Provider 不接受 upstream Shell ID；
- public File DTO 始终是逻辑 `/mnt/user-data/{workspace,uploads,outputs}/...`；
- 错误不泄露 endpoint、credential、签名、Thread 身份、物理 workspace、Shell ID、
  Redis metadata 或 upstream body。

- [ ] **Step 2: 写 HTTP/HTTPS exact-origin policy RED tests**

Remote endpoint 可显式配置 HTTP 或 HTTPS，但必须精确锁定 scheme/host/port；每次拨号
重新解析并拒绝 loopback、link-local、multicast、metadata、宿主网关、Unix socket 和
非 allowlist 地址。禁用环境代理、redirect 与隐式 HTTPS -> HTTP downgrade。HTTP 同样
要求 credential、签名、nonce、防重放与来源防火墙，并只投影
`transport_encrypted=false`；日志不打印 endpoint，header/body/stream 都有上限。

不全局放宽现有 `safehttp`；只在 Sandbox endpoint policy 内实现 exact-origin transport，
复用已有地址分类与 response limit。

- [ ] **Step 3: 写 Router capability RED tests**

- 只有健康/fresh 且同时声明 `sandbox_session_v1` 与
  `signed_session_context_v2` 的 Provider 才能 `ResolveSession`；
- one-shot `Resolve` 和 capacity lease 行为不变；
- Session resolve 只持有短期选择 guard，实际容量与同-Shell 串行由 Runner 管理；
- disabled/unhealthy/feature missing 返回稳定错误，不 fallback one-shot/local runtime；
- `local_debug` 只有 Task 10 三重门禁通过才实现 Session；
- `SelectedSessionProvider.Release` 只释放选择资源，不销毁 Runtime Session。

- [ ] **Step 4: 运行 RED tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'RemoteSession|PlainHTTP|Endpoint|ResolveSession' -count=1
```

Expected: FAIL，因为 Session remote provider 与 router 入口尚不存在。

- [ ] **Step 5: 实现可选 Provider、Router 与 wiring**

新增可选 `SessionRuntimeProvider`，不要给旧 `RuntimeProvider` 增加方法：

```go
type SessionRuntimeProvider interface {
    SandboxSessionManager
}
```

`RemoteProvider` 组合只调用 Task 7 private contract 的 manager，不再写 raw AIO client。
Router 复用 Provider lookup、default、scope permission、health freshness 与 factory，但
Session 使用独立短期 guard。wiring 从数据库 descriptor 注入 Provider ID/key；v1/v2 可
复用 key material，但 schema/domain separator 必须不同。Session signer/feature 缺失时
Session fail closed，one-shot v1 仍可用。wiring 不注入中间代理、Docker metadata、物理根
或客户端身份覆盖字段。

- [ ] **Step 6: 运行 GREEN 与全部 Provider/Router 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'Remote|Endpoint|Router|Resolve|Session|HTTP' -count=1
```

- [ ] **Step 7: 完成唯一一次 Task 8 主线审核并提交**

综合审核只检查 SSRF/exact-origin、签名/防重放、非幂等不重试、服务端身份绑定、feature
negotiation、脱敏与 one-shot 回归。修复后重跑本 Task 验证，不发起第二轮审核。

```bash
git add backend/infra/sandbox/provider.go backend/infra/sandbox/session_remote_provider.go backend/infra/sandbox/session_remote_provider_test.go backend/infra/sandbox/endpoint_policy.go backend/infra/sandbox/endpoint_policy_test.go backend/infra/sandbox/remote_provider.go backend/infra/sandbox/remote_provider_test.go backend/application/sandbox/router.go backend/application/sandbox/router_test.go backend/application/sandbox_wiring.go backend/application/sandbox_wiring_test.go
git commit -m "feat: route signed remote sandbox sessions"
```

不要 merge、push、发布或切业务流量；提交后自动继续 Task 9。

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

Session 管理只控制默认关闭的 Core。Interactive 固定关闭；Host Shell 由 Task 10 独立门禁
控制。运行投影只展示 raw AIO health、generation 与 Session/queue 聚合。

- [ ] **Step 1: 写 backend admin contract RED tests**

新增 GET/PUT `/api/admin/sandboxes/session-settings` 与 GET
`/api/admin/sandboxes/session-runtime-status`。沿用 admin permission、strict JSON、64 KiB、
trailing slash 404、CAS 409、脱敏 envelope。成功/失败审计复用 DB CHECK 已允许的
`scheduler_settings.update/update_failed`，以固定
`settings_domain=session` 区分；metadata 只含 previous/new version 与 changed fields。
仓储必须把成功审计 action/actor/version/真实 changed fields 与 CAS 语义绑定。

- [ ] **Step 2: 写 desired/applied 和 Runner 应用 RED tests**

Service 先事务保存 desired snapshot + audit，再调用签名 Runner configuration route：

- apply 成功才投影 `applied=true` 和精确 applied version；
- Runner unavailable/unknown 时 desired 保留、applied false + 稳定 reason；
- stale version 返回 409，不覆盖；
- enable Core 只有 raw health、sentinel/generation、workspace root、MySQL/Redis 全 ready 才成功；
- disable 即使 AIO unavailable 也能保存并让 Router fail closed；
- Interactive=true 固定 422；
- HostShell desired 不覆盖 Task 10 环境门禁；
- settings 不含 UID/GID、`thread_key`、中间代理配置、镜像版本、客户端 physical root；
- official latest 不做 revision/digest 比较。

dev 集成只使用现有 dev MySQL；测试使用 sqlmock/已有测试设施，不启动 DB 容器、不
AutoMigrate production schema。

- [ ] **Step 3: 写前端 card RED tests**

覆盖 desired/applied version、Core 默认关闭、Interactive disabled、Host Shell、raw AIO
ready/unknown、generation ready/recovering、queue/weight/Session/Shell 聚合与 transport
encrypted。HTTP Provider 显示未加密风险但不显示 endpoint。保留 loading/disabled/readonly/
refresh/error/focus/ARIA；Scheduler card 与 Provider CRUD 不变。页面不得渲染 sentinel/
upstream Shell ID、physical path、Docker/image、secret 或 raw error。

- [ ] **Step 4: 运行 RED tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./application/sandbox ./api/handler/coze ./api/router/coze -run 'SessionSettings|SessionRuntime' -count=1
cd ../frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__/sandbox-session-card.test.tsx src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-management-section.test.tsx
```

Expected: FAIL，因为 admin routes、service 与 card 尚不存在。

- [ ] **Step 5: 实现 backend service/handler/router 与聚合 DTO**

复用 Scheduler Service 的 actor、CAS、审计与 signed Runner client 风格，但 Session
repository/version 独立。Runtime DTO 至少包含 available、desired/applied version、
generation、Core/Interactive/Host Shell、raw AIO readiness、generation state、queue/weight、
active/idle Session/Shell、transport encrypted 和稳定 reason。sentinel ID 不对外投影。
Runner apply 失败不回滚已提交 desired fact，也不能伪造 applied。

- [ ] **Step 6: 实现前端 service/card 并运行 GREEN**

复用现有 card/request envelope 和 `@coze-arch/coze-design`，不引入 UI 库。

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./application/sandbox ./api/handler/coze ./api/router/coze -run 'Sandbox|SessionSettings|SessionRuntime' -count=1
cd ../frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__/sandbox-session-card.test.tsx src/pages/system/__tests__/sandbox-scheduler-card.test.tsx src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-management-section.test.tsx src/pages/system/__tests__/sandbox-system-page.test.tsx
```

- [ ] **Step 7: 完成唯一一次 Task 9 主线审核并提交**

综合审核只检查 admin permission/CAS/audit、desired/applied、默认关闭、generation 投影、
敏感字段、Host Shell/Interactive fail closed、UI 状态与 one-shot 管理回归。修复后重跑
全部验证，不发起第二轮审核。

```bash
git add backend/application/sandbox/session_settings_service.go backend/application/sandbox/session_settings_service_test.go backend/application/sandbox/types.go backend/api/handler/coze/admin_sandbox.go backend/api/handler/coze/admin_sandbox_test.go backend/api/router/coze/admin_sandbox.go backend/api/router/coze/admin_sandbox_test.go backend/application/sandbox_wiring.go frontend/apps/coze-studio/src/pages/system/sandbox-service.ts frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-service.test.ts frontend/apps/coze-studio/src/pages/system/sandbox-session-card.tsx frontend/apps/coze-studio/src/pages/system/sandbox-session-card.module.less frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-session-card.test.tsx frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-management-section.test.tsx
git commit -m "feat: manage sandbox session runtime settings"
```

不要 merge、push、发布或启用业务流量；提交后自动继续 Task 10。

## Task 10: 实现仅本机 Debug 可用的 Host Shell Session

**Files:**

- Create: `backend/infra/sandbox/host_shell_session.go`
- Create: `backend/infra/sandbox/host_shell_session_test.go`
- Modify: `backend/infra/sandbox/local_debug_provider.go`
- Modify: `backend/infra/sandbox/local_debug_provider_test.go`
- Modify: `backend/application/sandbox_wiring.go`
- Modify: `backend/application/sandbox_wiring_test.go`
- Modify: `backend/application/sandbox/types.go`

Host Shell 是独立 local-debug adapter，不是 raw AIO fallback。

- [ ] **Step 1: 写三重门禁 RED tests**

必须同时满足：

```text
APP_ENV=debug
SANDBOX_HOST_SHELL_SESSION_ENABLED=true
SANDBOX_HOST_SHELL_GATEWAY_ADDR=127.0.0.1:8099 或 [::1]:8099
```

覆盖 production/test/空环境、非 loopback、`0.0.0.0`、公网地址、remote Provider、AIO
unavailable 与热更新误开启均 unavailable。不得复用 `APP_DEV_HOST_RUNTIME_ENABLED`，
不得在 Remote/Core unavailable 时 fallback。

- [ ] **Step 2: 写本机风险边界 RED tests**

服务端从可信 space/user/thread 派生 debug physical root；public logical root 与 AIO 相同。
客户端不能提交 physical root。Exec 使用新 process group、deadline、bounded output、最小
环境、受控 workspace cwd；cancel 先 TERM 后按 grace KILL group。command/argv 不改写，
调用方用相对路径。File structured path 复用 mapper 并逆映射。

status 固定 `isolation_level=host_debug_unisolated`，明确没有容器、网络、credential 或
恶意命令隔离。错误/public DTO 不含物理绝对路径、环境、PID 或 command body。应用重启
后内存 Session 消失，running command 不恢复/不重放，workspace 文件可保留。

- [ ] **Step 3: 运行 RED、实现 adapter 并接入 Router**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'HostShellSession|LocalDebug|ResolveSession' -count=1
```

`HostShellSessionManager` 实现 Task 2 Core 接口，不支持 interactive/publish。Session 用
bounded memory map。门禁变化使新 acquire fail closed；已有进程按 shutdown 流程取消，
不切换 AIO。factory 只在 local_debug Provider + 三重门禁 + Session feature 同时满足时
返回 manager；远程 health 永不声明此 feature，UI 持续显示未隔离风险。

- [ ] **Step 4: 运行 GREEN 与 fail-closed 回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./infra/sandbox ./application/sandbox -run 'HostShellSession|LocalDebug|ResolveSession|RemoteSession' -count=1
```

- [ ] **Step 5: 完成唯一一次 Task 10 主线审核并提交**

综合审核只检查三重门禁、无 fallback、process-group cancel、路径/输出脱敏、debug-only
投影和风险文案。修复后重跑全部验证，不发起第二轮审核。

```bash
git add backend/infra/sandbox/host_shell_session.go backend/infra/sandbox/host_shell_session_test.go backend/infra/sandbox/local_debug_provider.go backend/infra/sandbox/local_debug_provider_test.go backend/application/sandbox_wiring.go backend/application/sandbox_wiring_test.go backend/application/sandbox/types.go
git commit -m "feat: add gated debug host shell sessions"
```

不要 merge、push、发布或在 remote 环境开启；提交后自动继续 Task 11。

## Task 11: 部署官方单常驻 AIO latest，并补齐 dev 发布/回滚合同

**Files:**

- Modify: `deploy/dev/docker-compose.runner-2c4g.yml`
- Modify: `deploy/dev/deploy.sh`
- Modify: `deploy/dev/tests/compose_contract_test.sh`
- Modify: `deploy/dev/tests/deploy_test.sh`
- Modify: `deploy/dev/tests/image_contract_test.sh`
- Modify only if current wiring requires it: `.github/workflows/deploy-dev.yml`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md`

直接运行 `ghcr.io/agent-infra/sandbox:latest`，不创建 AIO Dockerfile、不构建/晋级派生
镜像、不锁 digest/version，也不要求 raw AIO JWT 或启动资源参数。AIO/Compose 生命周期
唯一归 deploy；Runner 只监督 raw health、reserved sentinel 与 generation。

- [ ] **Step 1: 写 Compose 和 image/workflow RED contracts**

断言每个 deployment 恰好一个 `coze-sandbox-aio`，image 精确为 official latest。按
Task 0/1 已证实方式启动；当前平台需要时保留 official `seccomp=unconfined` 与 ARM64
`OPENSSL_armcap=0`，不得猜 entrypoint/command。启动不要求 `mem_limit/cpus/pids_limit`、
`DISABLE_*`、JWT 或业务配置。

只在 Compose 私网 `expose: 8080`；没有 host ports/network、privileged、Docker socket
或旧 8090。持久 volume 挂 `/mnt/user-data`，skills 只读挂 `/mnt/skills`，Runner URL
为 `http://coze-sandbox-aio:8080`。Core/Interactive/remote Host Shell 默认 false。
Compose 不增加 MySQL，Runner 使用 dev 已配置 MySQL/Redis。Runner 保留既有 one-shot
rootless Docker 能力，但 Session/AIO supervisor 路径不得调用 Docker CLI/socket 或管理 AIO。

workflow 仍只构建仓库自有镜像，不增加 AIO build/promotion/revision label/build args。
deploy 可记录本次 official image ID/digest 作为证据，但它不是配置锁、readiness 或
generation 来源。

- [ ] **Step 2: 写 deploy/rollback RED contracts**

- lifecycle 命令只在 deploy/Compose；pull/up AIO -> raw health -> up Runner；
- migration preflight 只含 additive `20260813000100`，deploy 不 apply；
- dev 使用现有 DB，不启动/清空/重建本地 DB，禁止 AutoMigrate/drop/truncate；
- health 只有 raw 8080、Runner Core projection/sentinel generation；
- rollback 先关闭 capability、恢复旧应用，保留 session table、scheduler columns、volume；
- 禁止 `down -v`、volume rm 或删除 workspace；
- latest 无版本锁；若无法选择节点缓存的旧 image，保持 Core disabled 并如实报告；
- AIO 首启失败只让 Core unavailable，one-shot/server/web 正常，Runner 不尝试 restart。

- [ ] **Step 3: 运行 RED contracts**

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
```

- [ ] **Step 4: 最小更新 Compose、deploy 与 runbook**

runbook 写清 official latest 直跑、未锁版本的复现/回滚限制、当次 image ID 证据；deploy
拥有 lifecycle，Runner 仅监督；最终 workspace 层级与逻辑路由风险；Core enable/disable、
generation、drain、volume 保留；dev secret 权限和 additive migration；HTTP 风险、
Host Shell debug-only、Plugin/Agent/Interactive 未迁移。不添加中间代理或派生镜像。

- [ ] **Step 5: 运行 GREEN contracts 与 Compose render**

```bash
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
docker compose -f deploy/dev/docker-compose.runner-2c4g.yml config --quiet
```

使用无真实秘密 fixture。若 official image 真实 health 与 Task 0/1 不一致，立即停止并报告
脱敏日志，不通过派生镜像绕过。

- [ ] **Step 6: 完成唯一一次 Task 11 主线审核并提交**

综合审核只检查 official latest、deploy-only lifecycle、无公开端口/派生镜像、本地 DB
禁止、默认关闭、additive rollback 和 secret 范围。修复后重跑全部合同，不发起第二轮审核。

```bash
git add deploy/dev/docker-compose.runner-2c4g.yml deploy/dev/deploy.sh deploy/dev/tests/compose_contract_test.sh deploy/dev/tests/deploy_test.sh deploy/dev/tests/image_contract_test.sh docs/superpowers/runbooks/sandbox-control-plane-operations.md docs/superpowers/runbooks/local-debug-and-test.md
# 仅在真实修改时追加 .github/workflows/deploy-dev.yml
git commit -m "feat: deploy official shared AIO runtime"
```

不要 merge、push、apply migration 或发布；提交后自动继续 Task 12。

## Task 12: 完成真实 Core E2E、generation 恢复和 `2C4G` 资源闸门

**Files:**

- Create: `backend/internal/sandboxrunner/aio_core_e2e_test.go`
- Create: `deploy/sandbox-runner/tests/aio_core_e2e_test.sh`
- Create: `deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh`
- Modify: `deploy/dev/tests/compose_contract_test.sh`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`

真实 E2E 只由 deploy test helper 管理 official latest AIO；Runner 不管理 Docker。数据库
直接使用已配置 dev MySQL，不启动 DB 容器、不新建/删除 schema、不 truncate/reset。
每轮生成唯一 deployment/provider/space/user/thread/session 前缀，只允许精确清理本轮可
证明创建的 row、Redis namespace 和 fixture workspace。

- [ ] **Step 1: 写 dev-data-safe E2E harness**

运行前只读确认 additive migration/Atlas status；测试自身不 AutoMigrate/apply/drop/truncate。
从受控 dev env/secret file 读取 MySQL/Redis/Runner 配置并脱敏；缺配置明确 blocked/skip，
不能启动本地 DB。记录本轮所有主键，cleanup 前再次验证 deployment prefix；禁止无主键
DELETE、flushdb、schema drop、volume clear。

AIO/network/volume 只由 deploy helper 管理。persistence case 跨 AIO recreate 保留 named
volume，结束仅删除本轮 fixture 子目录。记录 code SHA、official ref 和当次 image ID；
image ID 仅证据，不参与 generation。

- [ ] **Step 2: 验证 Session、同-Shell 与 File 完整矩阵**

真实 Runner + raw AIO：

1. 同 user 的 Thread A/B 与另一 user Thread C acquire 三个稳定 Session；物理根严格为
   `/mnt/user-data/<space>/<user>/<thread>/{workspace,uploads,outputs}`，opaque Shell ID
   不对外返回；
2. 同一 Session 两 Exec 严格串行，不同 Session 按 weight 并发，达到 total/per-user 后排队；
3. Shell exec_dir 是真实 workspace；logical cwd 仅 workspace root/subdir，命令用相对路径；
4. 验证 cwd/env、stdout/stderr、exit、timeout、bounded output；
5. 验证 Read/Write/List/Glob/Grep/Replace/Download，mutation `sudo=false`，structured
   response 逆映射；
6. adapter 拒绝客户端 physical path、`..`、错误 logical root、uploads/outputs/skills cwd；
7. cancel A 的 queued/running Exec 只影响 A 的同一个 Shell；File cancel 只 cancel HTTP context；
8. release cleanup Shell 但 marker 保留；reacquire/recover 新 Shell 复用 workspace；destroy
   默认不删 workspace。

明确记录：raw File API 无 Session 字段，exec_dir 不是 chroot，恶意 Shell/symlink/hardlink
跨目录隔离不在 Phase 1 保证内。不得写虚假的对抗性逃逸测试；若产品需要该保证，Core
保持 disabled 并另立隔离方案。

- [ ] **Step 3: 验证 sentinel generation 与恢复**

deploy helper recreate AIO 并保留 volume：

- down 时 Core unavailable/unknown，one-shot ready，不 fallback Host Shell；
- transient health/list error 不 bump；
- confirmed missing 后 CAS 使 generation 恰好 old+1，并发 Runner 只成功一次；
- reserved sentinel 不进入业务 API/统计/idle cleanup；
- 旧 Session recovering；explicit Recover 新 Shell + 当前 generation，workspace marker 保留；
- restart 前 running operation unknown/failed 且不重放；accepted/queued 可恢复；
- Redis/MySQL 故障用 test-scoped fake/proxy，不停止共享 dev dependency。

- [ ] **Step 4: 验证 shared failure domain 的有限资源合同**

Runner total/per-user weight、queue bound、timeout/cancel、输出/HTTP/File payload 上限必须
生效。普通 operation 超限后其他正常 Session 在 shared AIO 尚未整体耗尽时继续。AIO crash
使相关 Session recovering/unavailable，错误不含 raw body。

不宣称单个恶意 Session 的 CPU/PID/disk 被硬隔离；容器级 OOM/PID/disk exhaustion 会影响
同 deployment 所有 Session。若两个普通 Core workload 即可稳定耗尽 2C4G、OOM 或造成
PID/fd/session/shell 无界增长，属于阻断，Core 保持 disabled，不能弱化断言或虚构启动 cgroup。

- [ ] **Step 5: 运行 2m smoke 与 30m soak**

official latest 使用官网参数，不要求 Compose 启动资源限制。混合两个 Core Session 的
短命令、1–32 MiB 文件、glob/grep、cancel/reacquire 与同-Shell operation。每 5 秒采集
CPU/RSS/PID/fd、queue、weight、Session/Shell，只写仓库外 evidence。

```bash
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 2m
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 30m
```

通过：无 OOM、无持续增长；queue drain 后归零；sentinel 不计业务 Shell；Runner RSS
满足既有 192 MiB 上限；AIO 只记录实测水位；两个正常 Core 可稳定完成；低于 host reserve
停止新出队；无公开 AIO port、残留 probe 或非 fixture dev 数据变化。30m evidence 绑定
exact SHA、official image ID 和同构 2C4G 环境；缺环境必须报告 blocked，不能伪造。

- [ ] **Step 6: 执行 E2E、回归并更新实测水位**

```bash
bash deploy/sandbox-runner/tests/aio_core_e2e_test.sh
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 30m
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./internal/sandboxrunner ./infra/sandbox/... -count=1
```

runbook 只记录聚合最大/稳态水位、queue latency、Session/Shell 数、code SHA、image ID 和
环境规格，不记录身份、命令、path、token、DSN。

- [ ] **Step 7: 完成唯一一次 Task 12 主线审核并提交**

综合审核只检查 dev-data-safe fixture、deploy-owned fault injection、同-Shell/cancel、
sentinel CAS、无重放、资源证据与诚实隔离边界。修复后重跑适用验证；若相关代码/镜像/
环境变化，30m 必须重跑。不发起第二轮审核。

```bash
git add backend/internal/sandboxrunner/aio_core_e2e_test.go deploy/sandbox-runner/tests/aio_core_e2e_test.sh deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh deploy/dev/tests/compose_contract_test.sh docs/superpowers/runbooks/sandbox-control-plane-operations.md
git commit -m "test: gate shared AIO core on 2c4g"
```

不要 merge、push、apply migration 或发布；提交后自动继续 Task 13。

## Task 13: 全量回归、页面验收和长期事实更新

**Files:**

- Modify only after all gates pass: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md`
- Create only after Phase 1 acceptance:
  `docs/superpowers/plans/2026-08-13-deerflow-aio-shared-sandbox-phase2-implementation.md`

只有真实代码、dev migration 状态、official latest probe、E2E 与资源闸门全部通过，才更新
长期事实。本 Task 不修改 agentthread，不切任何业务流量，Plugin 继续 one-shot。

- [ ] **Step 1: 运行 backend/未迁移业务相关全量回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/sandbox ./pkg/sandboxidentity ./infra/sandbox/... ./internal/sandboxrunner ./application/sandbox ./api/handler/coze ./api/router/coze -count=1
GOCACHE=/private/tmp/coze-go-build go test -race ./infra/sandbox/... ./internal/sandboxrunner -count=1
GOCACHE=/private/tmp/coze-go-build go test ./application/plugin ./application/appdev ./application/agentthread ./infra/coderunner/... ./infra/appdev/... -count=1
```

使用 Mockey 的相关包按仓库方式补 `-gcflags="all=-l -N"`。这些是回归，不授权修改
`backend/application/agentthread/**`。one-shot Plugin/AppDev/MCP/Agent/Subagent 行为必须
不变。

- [ ] **Step 2: 运行前端、Atlas 与 deploy contracts**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/system/__tests__
rushx lint
cd ../../../..
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e migrate validate --dir file:///migrations
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/publish_dev_test.sh
```

Atlas 只 validate/hash；本 Task 不 apply、不启动 DB、不清空。dev DB 只做已授权 status/结构
只读确认。确认 migration 仅 additive `20260813000100`、一张 runtime session 表和 scheduler
columns；Compose official latest、无 public AIO port、Runner 无 Docker lifecycle。

- [ ] **Step 3: 用 in-app browser 验收系统页**

以管理员访问 `http://localhost:8080/system/sandbox`，记录账号类型、Provider、
desired/applied version、Core 默认关闭、Interactive disabled、Host Shell 门禁、raw AIO、
generation/recovering、HTTP 风险、Runner unavailable、保存/刷新/CAS conflict 与 console。
页面不显示 endpoint、sentinel/Shell ID、租户身份、physical path、Docker/image 或 secret。

共享 dev singleton 未获临时 mutation 授权时，只验默认关闭与失败/冲突状态；enable 转换由
Task 9/12 自动化证据承担。绝不为页面验收切业务流量。

- [ ] **Step 4: 检查影响面、用户文件和 secret**

用 codebase-memory detect_changes 后核真实 diff：

```bash
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git status --short
git diff -- docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md
```

确认无 agentthread 修改、用户 recovery design 未 stage/commit、无 credential/DSN/token/
local env/evidence/cache、无派生 AIO/中间代理/runtime identity/workspace 表/UID/GID/
`thread_key`，migration additive 且业务流量未切。

- [ ] **Step 5: 证据通过后更新长期事实并只规划 Phase 2**

长期事实同步写：每 deployment 一个 deploy-owned official latest AIO；Runner SDK raw 8080
监督 health/sentinel/MySQL CAS generation；服务端完整 identity；仅 runtime session 表；
最终 workspace 层级；Shell logical cwd + File mapping/`sudo=false`；逻辑路由而非 chroot/
恶意多租户隔离；shared failure domain；无版本 pin；Plugin/Agent/MCP/AppDev/Interactive 未
迁移；Host Shell local debug；HTTP/HTTPS exact-origin；rollback 保留表/volume/workspace。

只有所有 gate 通过才创建 Phase 2 计划，且只规划未来业务迁移。若触及 Workbench chain，
必须同步权威 markdown/JSON 与 graph verify/build/verify-derived；本 Task 不实施 Phase 2。

- [ ] **Step 6: 完成唯一一次 Task 13 主线审核并提交**

综合审核只检查证据新鲜、无切流量、文档/源码一致、风险诚实、用户文件、secret 与数据边界。
修复后重跑受影响及必要全量验证，不发起第二轮审核。

```bash
git add docs/superpowers/context/project-context.md docs/superpowers/runbooks/sandbox-control-plane-operations.md docs/superpowers/specs/2026-08-13-deerflow-aio-shared-sandbox-design.md docs/superpowers/plans/2026-08-13-deerflow-aio-shared-sandbox-phase2-implementation.md
git commit -m "docs: record shared AIO core runtime"
```

任一 gate 未通过时不提交“已实施”事实或 Phase 2 plan，保持 Core disabled 并报告证据。
不要 merge、push、apply migration 或发布；通过后自动继续 Task 14。

## Task 14: Phase 1 集成审计与交付选择

**Files:**

- Read: `docs/superpowers/runbooks/dev-integration-audit.md`
- Read: all Phase 1 commits and fresh verification evidence
- No changes unless audit finds an in-scope defect

Task 0–13 已各自完成唯一一次主线审核；Task 14 是整条主线的集成审计，不重复逐 Task
审核，也不授权 merge/push/migration apply/dev publish。

- [ ] **Step 1: 建立 exact SHA、commit、migration 与文件清单**

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --name-status origin/dev...HEAD
git status --short
git rev-parse HEAD
git rev-parse origin/dev
```

确认 migration 只有 additive `20260813000100`、一张 runtime session 表与 scheduler
columns；official AIO 直接 latest；无派生镜像、中间代理、UID/GID、`thread_key`、
Docker lifecycle in Runner；用户 recovery design 不在 commit/diff；无 secret/evidence；
无 agentthread 修改，业务流量未切。

- [ ] **Step 2: 从 current exact HEAD 运行新鲜最终验证**

重跑 Task 13 后端、前端、Atlas/deploy contracts 和 Task 12 E2E。30m soak 必须绑定当前
HEAD、official latest 当次 image ID 与同构 2C4G；相关代码/Compose/image/environment
变化使旧 evidence 失效。dev DB 只作已授权 status/结构只读确认，不启动 DB、不 apply、
不清空。

- [ ] **Step 3: 审查核心不变量**

- AIO 只在 Compose 私网 raw 8080，无 host port/network、privileged、Docker socket；
- lifecycle 仅 deploy；Runner 只做 health/sentinel/MySQL CAS generation；
- Session v2 绑定服务端完整身份，客户端不能提交 physical root/Shell ID；
- workspace 层级、Shell cwd、File mapping/`sudo=false` 真实通过；
- 同 Shell 跨 Runner 串行/fencing，cancel 不杀错，running 不重放；
- transient unknown 与 confirmed missing 分开，CAS 每次只 bump 一代，sentinel 不参与业务；
- 文档/UI 明确逻辑路由与 shared failure domain，不虚称 UID/chroot/cgroup 隔离；
- one-shot wire/Redis/Plugin 不变，Core 默认关闭，Interactive false，Host Shell debug-only；
- HTTP/HTTPS exact-origin、SSRF/rebinding、nonce/signature 通过；
- rollback 保留表/volume/workspace，无 AutoMigrate/drop/truncate/down-v；
- official latest 未锁版本的精确回退限制已披露；
- 日志/error/metric/UI 无 secret、endpoint、sentinel/Shell ID、physical path 或内部身份。

- [ ] **Step 4: 处理审计发现**

发现 in-scope defect 时按 systematic-debugging 补 RED、最小修复并重跑受影响 Task 与最终
验证；不再发起新一轮审核，但必须重新绑定 exact HEAD/evidence。若 raw AIO、generation、
cancel、data safety、资源、公开端口或诚实边界出现重大阻断，立即停止，保持 Core disabled，
报告命令/状态码/脱敏日志，不猜测绕过。

- [ ] **Step 5: 报告 exact SHA 并等待交付选择**

向用户报告 HEAD、origin/dev base、commit/migration/file 范围、新鲜验证、official image
ID、E2E/30m 水位、未验证与残余风险。未经用户对该 exact SHA/范围明确授权，不 merge、
push、apply 或 publish。之后若用户批准 fast-forward local dev，严格按 runbook 执行一次
集成检查；发布仍需确认实际 revision、migration range、credential permission 与 Atlas
status，批准后才运行 `publish-dev.sh`，不手工 push 或重复部署。

---

## Phase 1 验收矩阵

| 维度 | 必测 | 阻断条件 |
| --- | --- | --- |
| 上游 | official GHCR AIO latest + 固定 SDK 的 health/Shell/File/Cancel | 任一 Core API 不兼容 |
| 身份/路由 | 服务端 deployment/provider/space/user/thread/profile + 派生 workspace | 客户端覆盖事实、串 Session 或 physical root 泄漏 |
| Session | acquire/get/release/destroy/recover + opaque Shell | 重复 Shell、资源泄漏或 stale generation 执行 |
| 调度 | 两 Core/第三排队、per-user、同 Shell 跨副本串行 | 超权重、双 owner、lease loss 后写终态 |
| 取消 | queued cancel、running Exec same-Shell Kill、File context cancel | 杀错 Shell、late success、自动重放 |
| 恢复 | sentinel + MySQL CAS generation、workspace 保留 | transient bump、重复/不 bump、running 重放 |
| 路径 | space/user/thread、logical cwd、File map/reverse、sudo=false | 接受客户端 physical root、cwd 越界或 sudo=true |
| 隔离声明 | 逻辑路由 + shared failure domain | 文档/UI/测试虚称对抗性 tenant 隔离 |
| 资源 | normal two-Core 2C4G smoke/30m，queue/PID/fd/session bounded | OOM、持续增长或正常 workload 不稳定 |
| 传输 | HTTP/HTTPS exact origin、防 SSRF/rebind/redirect/downgrade | 特殊地址可达、非幂等重试、签名/nonce 失效 |
| 兼容 | v1 one-shot、Plugin/AppDev/MCP/Agent 回归 | 既有链变化或 agentthread 修改 |
| 运维/数据 | official latest deploy-owned、dev DB、additive rollback | Runner 管 Docker、本地 DB/清空、公开端口、删表/volume |

## Phase 2/3 交接条件

- Phase 2 只在本计划全部 gate 通过且用户再次确认后规划/实施业务迁移；Phase 1 不切流量。
- Phase 2 触及 Workbench chain 时必须同步权威 markdown/JSON 与 graph 验证。
- Phase 3 才考虑 service lease 与 Browser/Jupyter/VSCode/Terminal/MCP/preview。
- 未来若要求恶意多租户隔离，必须另设计 container/namespace/chroot/openat2/per-tenant
  resource boundary，不能把本 Phase 1 目录路由升级为安全声明。
- official latest、SessionRef/identity v2/generation/path mapper 后续变更都需新 spec 和
  compatibility 审核，不能在 Phase 1 隐式漂移。
