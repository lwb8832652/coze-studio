# Unified Local Code Sandbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Agent/Workflow Code、Skill Script 和 Code Plugin 在显式本地模式下共用同一个受限 Deno/Pyodide Runner，并在当前本机完成工作流和代码插件真实页面验收，同时不让低可信本地执行解锁代码插件发布。

**Architecture:** `appinfra` 在 application composition 之前解析唯一 Runner 模式；本地模式在 control-plane/legacy/direct 分支之前构造可失败的 `localwasm.Runner`，运行资产由 Go embed 物化到 lock-digest 私有缓存并只允许离线执行。Plugin、Workflow、Skill 接收同一 Runner 实例并保留各自 purpose；Plugin 只有 `control_plane_remote` assurance 才写发布资格，本地 `local_wasm_best_effort` 只返回真实调试结果。

**Tech Stack:** Go、Deno 2、Pyodide、TypeScript、React、Thrift/Hertz、Vitest、testify、Codex in-app browser。

---

## 实施边界

- Worktree：`/Users/liuwenbo/.codex/worktrees/745a/coze-studio`。
- 起始设计 HEAD：`f44bde71c`；每个任务写入前重新核对 status，保留用户后续相关改动。
- 权威设计：`docs/superpowers/specs/2026-08-14-dev-local-code-sandbox-design.md`。
- 不新增 migration，不改变 Provider scope，不给 `local_debug` 增加 plugin scope，不把 Host Shell/direct/remote 失败当本地回退。
- 本地模式不读取 `APP_ENV`；唯一开关为精确 `SANDBOX_LOCAL_CODE_RUNNER_ENABLED=true`，且原始 `SANDBOX_RUNTIME_ROUTING_ENABLED` 必须精确为 `false`。
- 本地成功的 assurance 固定为 `local_wasm_best_effort`，不能调用 `MarkDebuggedCAS`；只有 control-plane remote 成功可以获得发布资格。
- 业务进程只离线预检和执行；联网安装 Deno/缓存依赖只允许显式 prepare 命令。

### Task 1: TDD 冻结 purpose、assurance 与稳定错误合同

**Files:**
- Modify: `backend/infra/coderunner/code.go`
- Create: `backend/infra/coderunner/code_test.go`
- Modify: `backend/infra/coderunner/impl/impl.go`
- Modify: `backend/infra/coderunner/impl/impl_test.go`
- Modify: `backend/infra/coderunner/impl/controlplane/runner.go`
- Modify: `backend/infra/coderunner/impl/controlplane/runner_test.go`
- Modify: `backend/infra/coderunner/impl/direct/runner.go`
- Modify: `backend/infra/coderunner/impl/sandbox/runner.go`

- [ ] **Step 1: 写合同与 purpose RED**

新增测试要求：

```go
const (
    PurposeAgent  Purpose = "agent"
    PurposeSkill  Purpose = "skill"
    PurposePlugin Purpose = "plugin"
)

type Assurance string

const (
    AssuranceControlPlaneRemote  Assurance = "control_plane_remote"
    AssuranceLocalWASMBestEffort Assurance = "local_wasm_best_effort"
    AssuranceLegacyUnverified    Assurance = "legacy_unverified"
)
```

`RunResponse` 必须携带 `Assurance`；新增 `ErrCodeRunnerUnsupportedLanguage`。legacy guard 对空、Agent、Skill 保持兼容，Plugin 仍 unavailable，未知 purpose 仍 invalid。control-plane 的 Skill 必须映射到 Agent scope/workload/entrypoint，成功响应必须是 `control_plane_remote`。legacy direct/sandbox 成功只能标记 `legacy_unverified`。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner ./infra/coderunner/impl ./infra/coderunner/impl/controlplane \
  -run 'Test(RunResponseAssurance|LegacyRunnerPurposeGuardAllowsSkill|ControlPlaneRunner.*(Skill|Assurance))' \
  -count=1
```

Expected: compile FAIL，仅因 `PurposeSkill`、`Assurance` 或 unsupported sentinel 尚不存在。

- [ ] **Step 3: 写最小合同与映射**

在 `code.go` 定义上述类型；`RunResponse` 改为：

```go
type RunResponse struct {
    Result    map[string]any
    Assurance Assurance
}
```

`codeRunnerRouteForPurpose` 把 `PurposeSkill` 与 `PurposeAgent` 放在同一安全分支；control-plane 只在完整 remote operation 成功后填写 remote assurance。既有 direct/sandbox 只写 legacy assurance，不改变现有选择或权限。

- [ ] **Step 4: 运行 GREEN 与相关回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner ./infra/coderunner/impl ./infra/coderunner/impl/controlplane \
  -count=1
```

Expected: PASS。

### Task 2: TDD 固定 Deno/Pyodide 资产与显式离线 prepare

**Files:**
- Create: `backend/infra/coderunner/impl/localwasm/assets/launcher.ts`
- Create: `backend/infra/coderunner/impl/localwasm/assets/deno.lock`
- Create: `backend/infra/coderunner/impl/localwasm/assets.go`
- Create: `backend/infra/coderunner/impl/localwasm/prepare.go`
- Create: `backend/infra/coderunner/impl/localwasm/prepare_test.go`
- Create: `backend/cmd/local-code-sandbox-prepare/main.go`
- Create: `backend/cmd/local-code-sandbox-prepare/main_test.go`
- Modify: `Makefile`

- [ ] **Step 1: 写 prepare/preflight RED**

冻结 API：

```go
func Prepare(ctx context.Context) error
func NewRunner() (coderunner.Runner, error)
```

包内 tests 用私有 options 注入 Deno 路径、cache root 和 command seam，覆盖：无 Deno、Deno 非规范绝对路径、非 major 2、缺 lock/launcher、lock/launcher digest 漂移、冷缓存、目录非 `0700`、prepare 失败不写 ready manifest、并发 prepare 只产生一套完整缓存。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner/impl/localwasm ./cmd/local-code-sandbox-prepare \
  -run 'Test(Prepare|NewRunnerPreflight|PrepareCommand)' -count=1
```

Expected: compile FAIL，因为 package/API/资产尚不存在。

- [ ] **Step 3: 实现固定资产与原子缓存**

`launcher.ts` 只从 stdin 读取一个 request object，校验 Python/code/params，使用精确
`jsr:@langchain/pyodide-sandbox@0.0.4` 的 `runPython`。params 转成 UTF-8 hex 后在 Python 内
`bytes.fromhex(...).decode()`，避免源码拼接注入；调用用户 `async def main(args)`，只在 stdout
写一个 JSON object。

用 `//go:embed assets/launcher.ts assets/deno.lock` 固定资产。默认根目录为当前用户 cache 下
`coze-studio/local-code-sandbox`，实际 `DENO_DIR` 为 `SHA256(deno.lock)` 子目录；目录 `0700`，
文件 `0600`。prepare 在同文件系统 staging 目录运行：

```text
deno cache --frozen --lock=<absolute managed lock> --no-config --node-modules-dir=none <absolute launcher>
```

成功后原子 rename，再写含 Deno major、lock digest、launcher digest 的 manifest。进程并发使用
Darwin/Linux advisory lock；业务 `NewRunner` 只验证，不补缓存、不联网。

- [ ] **Step 4: 增加显式命令并运行 GREEN**

Make target：

```make
.PHONY: sandbox_local_code_prepare
sandbox_local_code_prepare:
	cd backend && go run ./cmd/local-code-sandbox-prepare
```

重复 Step 2，Expected: PASS。再运行：

```bash
git diff --check
```

### Task 3: TDD 实现 hardened local WASM Runner

**Files:**
- Create: `backend/infra/coderunner/impl/localwasm/runner.go`
- Create: `backend/infra/coderunner/impl/localwasm/runner_test.go`
- Create: `backend/infra/coderunner/impl/localwasm/process_darwin_linux.go`
- Create: `backend/infra/coderunner/impl/localwasm/process_unsupported.go`
- Create: `backend/infra/coderunner/impl/localwasm/process_darwin_linux_test.go`
- Create: `backend/infra/coderunner/impl/localwasm/deno_integration_test.go`

- [ ] **Step 1: 写 wire/purpose/limits RED**

表驱动锁定：只接受 Agent/Skill/Plugin；拒绝空/未知；只支持 Python；源码/params wire 最大
`1 MiB`；stdout 最大 `1 MiB`、stderr 最多 drain `64 KiB` 且不回传；stdout 必须是唯一 JSON
object，允许终止换行但拒 trailing value/bytes；成功 assurance 必须是
`local_wasm_best_effort`；错误和日志不包含 source、argv、temp path、env、stdout/stderr 原文。

- [ ] **Step 2: 写 lifecycle RED**

用 Go test helper process 模拟 Deno，覆盖：并发上限 2，第 3 个立即 capacity；60 秒策略可由测试
私有 seam 缩短；caller cancel/timeout/output overflow 都 kill 完整 process group，唯一 goroutine
`Wait`，reap 后才释放 semaphore；stdin/stdout/stderr 并发流式处理；临时 `0700` 空 cwd 在返回前
清理；并发请求不串 params/result/目录/FD。

- [ ] **Step 3: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner/impl/localwasm \
  -run 'TestRunner(ExecutesPython|Rejects|Limits|StrictJSON|Redacts|Capacity|Timeout|Cancellation|TerminatesProcessGroup|ConcurrentIsolation|Cleans)' \
  -count=1
```

Expected: FAIL，因为 Runner 行为尚未实现。

- [ ] **Step 4: 写最小有界执行器**

固定 application argv：

```text
deno run --cached-only --frozen --lock=<absolute lock> --no-prompt --no-config
  --node-modules-dir=none --v8-flags=--max-old-space-size=128 <absolute launcher>
```

不传任何 `--allow-*`。`Cmd.Env` 从空 slice 构造，只含规范绝对 `DENO_DIR`、固定
`NO_COLOR=1`、`DENO_NO_UPDATE_CHECK=1`。源码只进 stdin。Darwin/Linux 在 Start 前
`Setpgid=true`，取消/超限对负 PGID 发 `SIGKILL` 并等待回收；unsupported OS 构造即失败关闭。

- [ ] **Step 5: 运行 GREEN、race 与真实隔离 RED/GREEN**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./infra/coderunner/impl/localwasm -count=1
GOCACHE=/private/tmp/coze-go-build go test -race -p 1 ./infra/coderunner/impl/localwasm -count=1
```

显式 prepare 后运行真实 gate：

```bash
make sandbox_local_code_prepare
cd backend
SANDBOX_LOCAL_CODE_RUNNER_INTEGRATION=1 \
GOCACHE=/private/tmp/coze-go-build go test -p 1 ./infra/coderunner/impl/localwasm \
  -run '^TestDenoPyodide' -count=1
```

真实 tests 必须覆盖 Python 输入/JSON 输出、离线执行、host file/env/network/run/FFI/dynamic import
探针失败，以及 cancel 后无 Deno child。任一探针未被拒绝则此任务不完成。

### Task 4: TDD 唯一模式解析与 application 启动失败

**Files:**
- Modify: `backend/infra/coderunner/impl/impl.go`
- Modify: `backend/infra/coderunner/impl/impl_test.go`
- Modify: `backend/application/base/appinfra/app_infra.go`
- Modify: `backend/application/base/appinfra/app_infra_test.go`
- Modify: `backend/application/sandbox_wiring.go`
- Modify: `backend/application/sandbox_wiring_test.go`

- [ ] **Step 1: 写完整真值表 RED**

将工厂签名冻结为：

```go
func New(conf *config.BasicConfiguration) (Runner, error)
```

私有 `runnerFactoryDeps` 注入 getenv/newLocal spy。矩阵包含 local 的空/false/true/非法值，routing
的空/false/true/非法值，control-plane true/false。重点断言：local=true 仅 routing 原始值严格
false 成功；`control-plane=true+routing空+local=true` 失败；APP_ENV 任意值不影响；local 分支不调用
legacy/direct/Router/HostShell；local=false 完整保持现有基线。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner/impl ./application/base/appinfra ./application \
  -run 'Test(CodeRunnerModeTruthTable|AppInfraCodeRunnerFailure|SandboxLocalCodeRunner)' -count=1
```

- [ ] **Step 3: 实现唯一 selector 与 fail-fast composition**

local=true 在现有 control-plane early return 和 `newLegacyRunner` 前调用 `localwasm.NewRunner()`；
dependency/preflight 错误原样使 `appinfra.Init` 失败。`appinfra` 先构造最终 CodeRunner，再在非 local
模式才构造 legacy `LocalExecutionDelegate`。`initSandboxControlPlaneForApplication` 在
routing=false 时不覆盖 CodeRunner，不创建 Router/runtime binding。Host Shell gate 全开也不能旁路。

- [ ] **Step 4: 运行 GREEN 与 compile regression**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner/impl ./application/base/appinfra ./application \
  -run 'CodeRunner|Sandbox.*Routing|HostShell' -count=1
```

Expected: PASS。

### Task 5: TDD 统一 Plugin、Workflow、Skill 的 Runner 实例与 purpose

**Files:**
- Modify: `backend/application/plugin/init.go`
- Create: `backend/application/plugin/init_test.go`
- Modify: `backend/application/application.go`
- Create: `backend/application/code_runner_wiring_test.go`
- Modify: `backend/application/workflow/init.go`
- Modify: `backend/domain/skill/service/script_executor.go`
- Modify: `backend/domain/skill/service/script_executor_test.go`

- [ ] **Step 1: 写依赖注入 RED**

`plugin.ServiceComponents` 新增 `CodeRunner coderunner.Runner`。测试断言 Plugin 初始化直接保存该值，
不读取全局；`toPluginServiceComponents`、Workflow components、Skill components 观察到同一 pointer；
Plugin 先于 Workflow 初始化仍非 nil。

- [ ] **Step 2: 写 purpose RED**

`ScriptExecutor` 必须发送 `PurposeSkill`；Workflow Code 保持 `PurposeAgent`；Plugin 保持
`PurposePlugin`。内部 legacy local execution delegate 构造 `RunRequest` 时显式写
`PurposeAgent`，不再依赖空 purpose。

- [ ] **Step 3: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' \
  ./application/plugin ./application ./domain/skill/service \
  -run 'Test(PluginInitUsesInjectedCodeRunner|PluginCodeRunnerWiring|ScriptExecutorSendsPurposeSkill)' \
  -count=1
```

- [ ] **Step 4: 写最小注入并运行 GREEN**

`PluginApplicationSVC.codeRunner = components.CodeRunner`，删除 Plugin 初始化对
`coderunner.GetCodeRunner()` 的读取；`toPluginServiceComponents` 传
`CodeRunner: b.infra.CodeRunner`。Workflow 暂保留现有全局兼容写入，但只能写同一实例。

重复 Step 3，Expected: PASS。

### Task 6: TDD Plugin assurance 与发布资格边界

**Files:**
- Modify: `backend/application/plugin/code_plugin.go`
- Modify: `backend/application/plugin/code_plugin_test.go`
- Modify: `idl/plugin/plugin_develop_common.thrift`
- Generated: `backend/api/model/plugin_develop/common/plugin_develop_common.go`
- Generated: `frontend/packages/arch/api-schema/src/idl/plugin/plugin_develop_common.ts`
- Create: `frontend/packages/arch/api-schema/__tests__/plugin-code-contract.test.ts`

- [ ] **Step 1: 写 backend assurance RED**

新增三例：local success 返回真实 result 与 local assurance，但 `MarkDebuggedCAS` 调用 0 次；remote
success 调用 1 次并推进现有资格；empty/unknown assurance 返回结果但不 mark。所有 failure 仍不 mark。
unsupported language 映射到现有 RuntimeError，reason 固定 `current local sandbox only supports Python`。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' \
  ./application/plugin -run 'TestDebugCodePlugin.*(Assurance|UnsupportedLanguage)' -count=1
```

- [ ] **Step 3: 修改 IDL 并安全生成**

在 `CodePluginDebugData` 添加：

```thrift
8: optional string execution_assurance,
```

先运行 retained clean-tree generator：

```bash
KEEP_API_CODEGEN_TMP=1 bash backend/scripts/verify_api_codegen.sh
```

首次预期因真实 generated tree stale 而 exit 1；从脚本报告的 `run-one` 临时树只机械复制
`backend/api/model/plugin_develop/common/plugin_develop_common.go`，不得覆盖手写 handler/router。
然后运行：

```bash
cd frontend/packages/arch/api-schema
rushx update
cd /Users/liuwenbo/.codex/worktrees/745a/coze-studio
bash backend/scripts/verify_api_codegen.sh
```

Expected: generator verification PASS；TS 字段为 `execution_assurance?: string`。

- [ ] **Step 4: 实现 Plugin 发布分支并运行 GREEN**

只有 `response.Assurance == AssuranceControlPlaneRemote` 才调用 `MarkDebuggedCAS`。公开响应只投影
两个已知 assurance；空/未知省略字段并 fail closed。重复 Step 2，并运行：

```bash
cd frontend/packages/arch/api-schema
rushx test __tests__/plugin-code-contract.test.ts
```

### Task 7: TDD Plugin revision 0、错误分类与本地成功 UI

**Files:**
- Modify: `frontend/packages/agent-ide/bot-plugin/entry/src/pages/plugin-id/code-plugin-workspace.tsx`
- Modify: `frontend/packages/agent-ide/bot-plugin/entry/src/pages/plugin-id/code-plugin-workspace.module.less`
- Modify: `frontend/packages/agent-ide/bot-plugin/entry/src/pages/plugin-id/__tests__/code-plugin-workspace.test.tsx`

- [ ] **Step 1: 写 revision/save RED**

三例：revision 0 即使 clean 也先 Save，再用返回 revision Debug；revision>0 clean 不重复 Save；自动保存
失败、身份变化、卸载或旧 mutation 不发送 Debug。保存错误固定显示“代码草稿保存失败，请稍后重试”。

- [ ] **Step 2: 写 assurance/UI RED**

local assurance：保留 `success=true` 与真实 JSON result，不 reload 成 debug-ready，显示“本地沙箱试运行
通过；发布前需使用远程沙箱验证”，`onPublishReadyChange(false)`；remote assurance 保持既有 reload
与发布资格；缺失/未知 assurance 保留结果但发布 fail closed。覆盖 RuntimeError/Timeout/Capacity/
OutputLimit/Unavailable 文案，JavaScript local unsupported 固定“当前本地沙箱仅支持 Python”。

- [ ] **Step 3: 运行 RED**

```bash
cd frontend/packages/agent-ide/bot-plugin/entry
rushx test src/pages/plugin-id/__tests__/code-plugin-workspace.test.tsx
```

Expected: 新用例 FAIL。

- [ ] **Step 4: 写最小前端分流**

保存条件：

```ts
const mustSave = dirty || revision === 0;
const draft = mustSave ? await saveDraft(true) : undefined;
if (!isCurrentIdentity(debugOwner) || (mustSave && !draft)) {
  return;
}
const debugRevision = draft?.revision ?? revision;
```

remote 才 reload draft 确认资格；local/missing/unknown 不覆盖真实成功结果。提示使用现有 Coze Design
状态组件，并保留 aria-live/status、loading、焦点和禁用行为。

- [ ] **Step 5: 运行 GREEN、typecheck 与 lint**

```bash
cd frontend/packages/agent-ide/bot-plugin/entry
rushx test src/pages/plugin-id/__tests__/code-plugin-workspace.test.tsx
rushx lint -- --no-cache
cd /Users/liuwenbo/.codex/worktrees/745a/coze-studio/frontend/apps/coze-studio
node_modules/.bin/tsc --noEmit -p tsconfig.json
```

Expected: 全部 exit 0。

### Task 8: 文档、配置样例与完整自动化验证

**Files:**
- Modify: `docker/.env.debug.example`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md`
- Modify if target belongs there: `docs/superpowers/runbooks/project-operations.md`

- [ ] **Step 1: 更新长期事实与操作命令**

写清：任何环境都可显式选择 local；默认/共享多租户仍推荐 HTTPS Native Runner；local 状态固定
`local_wasm_best_effort`；不具备容器级/对抗性隔离；必须 routing=false；HostShell/Remote/MCP runtime
不可同时执行；prepare 可联网，应用启动/执行不可联网；回滚只关开关并重启。

样例仅给非秘密配置：

```bash
SANDBOX_LOCAL_CODE_RUNNER_ENABLED=true
SANDBOX_RUNTIME_ROUTING_ENABLED=false
SANDBOX_CONTROL_PLANE_ENABLED=true
```

- [ ] **Step 2: 跑后端相关完整回归**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./infra/coderunner/... ./domain/skill/service ./application/plugin ./application/base/appinfra \
  ./application -count=1
GOCACHE=/private/tmp/coze-go-build go test -race -p 1 \
  ./infra/coderunner/... -count=1
```

- [ ] **Step 3: 跑前端与生成合同回归**

```bash
cd frontend/packages/agent-ide/bot-plugin/entry
rushx test src/pages/plugin-id/__tests__/code-plugin-workspace.test.tsx
cd /Users/liuwenbo/.codex/worktrees/745a/coze-studio/frontend/packages/arch/api-schema
rushx test __tests__/plugin-code-contract.test.ts
cd /Users/liuwenbo/.codex/worktrees/745a/coze-studio
bash backend/scripts/verify_api_codegen.sh
git diff --check
```

- [ ] **Step 4: 影响面检查与唯一代码审查**

用 codebase-memory `detect_changes`（若当前 MCP 未暴露则记录并回到真实 diff/源码）核对 caller；确认
无 Workbench monitored path 改动，因此不构建 Graphify。读取 requesting-code-review skill，发起一次独立
只读审查；P0/P1 必须为 0 后才能进入真实验收。

### Task 9: 当前本机依赖、重启与真实页面验收

**Files:**
- External local dependency: Deno v2 under a user-writable path
- External ignored launcher config: current local backend launcher/env only
- No repository source mutation unless a verified defect is found through TDD

- [ ] **Step 1: 安装并准备当前本机依赖**

优先使用已固定的 Deno 2 binary；当前验收记录 exact version/path，但仓库只支持 major 2。运行：

```bash
make sandbox_local_code_prepare
```

然后在无网络执行条件下跑 `TestDenoPyodide*`，不得以 fake/helper 代替。

- [ ] **Step 2: 更新当前 ignored launcher 并重启**

当前后端 launcher 设置 local=true、routing=false、control-plane=true，并确保 Deno absolute path 可被
preflight 发现。先优雅停止旧后端，再启动新进程；不重启数据库、不修改共享 Provider/settings。
前端如仍健康可保留，否则重启当前 `rush dev`。

- [ ] **Step 3: API/日志预检**

确认后端端口、登录态与 `/system/sandbox` 页面可打开；control plane 管理面仍可用但 runtime routing
未启用；日志不含源码、params、stdout/stderr、DENO_DIR 或 temp path。真实调用证明 Plugin、Workflow
均落同一 local runner，且无 Remote/HostShell fallback。

- [ ] **Step 4: Workflow 页面验收**

使用当前账号/空间创建或打开可回滚的测试工作流，Python Code 节点接收固定 JSON，返回固定 JSON；
点击试运行，记录实际输入映射、输出、耗时和浏览器 console。失败时保留证据并回到 TDD，不用手工
改数据库掩盖问题。

- [ ] **Step 5: Code Plugin 页面验收**

打开插件 `7672703696211279872`，Python 代码保持：

```python
async def main(args):
    params = args.params
    return {"result": params}
```

输入 `{"message":"hello"}`。第一次点击必须先 Save，revision>0 后 Debug；页面显示真实 JSON，显示
`local_wasm_best_effort` 提示，不显示“服务不可用”，发布仍 disabled。浏览器 console 与后端日志无新增
未处理错误或敏感值。

- [ ] **Step 6: 关闭/恢复验收**

关 local flag 并重启：Plugin 恢复 unavailable，Workflow/Skill 按既有 legacy 配置，不回退 HostShell/
Remote；再打开 local flag 重启，Workflow 与 Plugin 再次成功，排除前端缓存/stub。最后保留用户当前要
复验的可用 local 配置运行。

- [ ] **Step 7: 最终证据与分支收尾**

读取 verification-before-completion 与 finishing-a-development-branch。报告新鲜命令、页面 URL、账号/
空间、revision、可见结果、console、未验证项和本地依赖路径；不 merge、不 push、不 deploy，等待用户
复验与后续 Git 指令。

