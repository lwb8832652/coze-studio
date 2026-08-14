# Workbench Adaptive Execution P1M-B1 Application Admission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 application 新输入边界拒绝七个客户端执行控制，同时保留 server-owned ADK child 与历史恢复配置兼容。

**Architecture:** 公开 `CreateTaskThread`/`CreateRun` 在 normalization 和持久化前调用 package 私有结构化 validator；`CreateRun` 主体下沉到私有 core，只有同包 ADK recorder 能选择受约束的 server-owned child provenance。历史 resume/retry/recovery 不经过该 admission，canonical mapper只增加 typed error 映射。

**Tech Stack:** Go、Hertz、encoding/json、testify、Workbench execution graph tooling。

---

## 实施边界

- Worktree：`/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp`。
- Base HEAD：`7366bb4fa740aaf2dccc15f8452673b6f3fa8d1f`；开始写代码前重新核对。
- 保留且不修改未跟踪的 P1L 草稿。
- 不改前端、IDL、repository、migration、runtime consumer 或恢复实现。

### Task 1: TDD application validator 与 typed error

**Files:**
- Create: `backend/application/agentthread/retired_execution_control.go`
- Create: `backend/application/agentthread/retired_execution_control_test.go`

- [ ] **Step 1: 写 validator RED**

新增表驱动测试，逐一覆盖七字段在 `config`、`context` root 和
`configurable/context` 四跳内的拒绝；再覆盖 mixed-case canonical path、普通字符串/数组、
`resource.mode`、`scheduled_task.variables.mode`、合法 runtime/model/resource。
测试调用尚不存在的：

```go
func validateSubmittedExecutionControls(config, runContext string) error
func UnsupportedExecutionControlPath(err error) (string, bool)
```

并要求 `errors.Is(err, ErrUnsupportedExecutionControl)` 为 true。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread -run '^TestSubmittedExecutionControls' -count=1
```

Expected: compile FAIL，仅因上述 symbol 尚未定义。

- [ ] **Step 3: 写最小 validator**

定义：

```go
var ErrUnsupportedExecutionControl = errors.New("unsupported execution control")

type unsupportedExecutionControlError struct { path string }
func (e *unsupportedExecutionControlError) Error() string
func (e *unsupportedExecutionControlError) Unwrap() error
func UnsupportedExecutionControlPath(err error) (string, bool)
func validateSubmittedExecutionControls(config, runContext string) error
```

使用 `parseDeerFlowRuntimePayload` 解析两个 object；按固定字段与保留容器顺序递归，最多四跳，
只扫描 object，不扫描数组/字符串/普通业务 object。非法 JSON 保持既有
`ErrInvalidRuntimeConfig` 语义。

- [ ] **Step 4: 运行 GREEN**

重复 Step 2 命令，Expected: PASS。

### Task 2: TDD 公开 admission 与私有 ADK child 通道

**Files:**
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder_test.go`

- [ ] **Step 1: 写 service RED**

新增表驱动测试：CreateTaskThread immediate/deferred、CreateRun、top-level retry 分别提交
`config.mode` 或 `context.configurable.requested_policy`，要求 typed error 且 recording service
的 Thread/Run/Bundle/Message 调用全部为 nil。CreateRun 测试保留现有 Thread authorization
上下文，证明 admission 位于授权后、normalization/来源查询前。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread -run '^(TestApplication(CreateTaskThread|CreateRun)RejectsSubmittedExecutionControls|TestApplicationTopLevelRetryRejectsSubmittedExecutionControls)$' -count=1
```

Expected: FAIL，因为公开入口仍接受旧字段。

- [ ] **Step 3: 写最小 admission/core**

`CreateTaskThread` 在现有 normalize 调用前执行 validator。将公开 CreateRun 改为：

```go
type createRunProvenance uint8
const (
    createRunSubmitted createRunProvenance = iota
    createRunServerOwnedSubagent
)

func (s *ApplicationService) CreateRun(ctx context.Context, req *CreateRunRequest) (*CreateRunResponse, error) {
    return s.createRun(ctx, req, createRunSubmitted)
}
```

私有 `createRun` 保留原 dependency/nil/authorization 顺序；submitted 在授权后校验，server-owned
分支只允许 `ParentRunID > 0 && RunKind == RunKindSubagent`，unknown provenance fail closed。
ADK recorder 改调用私有 server-owned provenance，不增加 exported bypass。

- [ ] **Step 4: 运行 GREEN 与 ADK compatibility**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread -run '^(TestApplication(CreateTaskThread|CreateRun)RejectsSubmittedExecutionControls|TestApplicationTopLevelRetryRejectsSubmittedExecutionControls|TestApplicationADKSubagentRunRecorder.*)$' -count=1
```

Expected: PASS；ADK child 仍持久化当前 server-owned compatibility config。

### Task 3: 错误映射与历史兼容回归

**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_contract.go`
- Modify: `backend/api/handler/coze/workbench_canonical_contract_test.go`
- Modify only if assertion is missing: existing Human resume, subagent retry, journal recovery and lease recovery tests under `backend/application/agentthread/`

- [ ] **Step 1: 写 mapper RED**

向 `TestMapCanonicalApplicationError` 加入 typed error，要求 HTTP 422、
`code/errorClass=unsupported_execution_control`、`retryable=false`、detail 为
`Unsupported execution control: config.mode`。

- [ ] **Step 2: 运行 RED 并实现映射**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -run '^TestMapCanonicalApplicationError$' -count=1
```

在 `ErrInvalidRuntimeConfig` case 之前处理 `ErrUnsupportedExecutionControl`，通过
`UnsupportedExecutionControlPath` 构造安全 detail；重复命令，Expected: PASS。

- [ ] **Step 3: 冻结历史兼容**

复用或补充既有测试，让 source/parent Run 的 Config 含旧 `mode/requested_policy`，并分别证明
Human resume、subagent retry、journal recovery、lease recovery 创建的目标 Run 原样继承；这些
路径不得调用新 admission。只补缺失断言，不重构恢复代码。

- [ ] **Step 4: 跑 application/handler focused regression**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread ./api/handler/coze -run 'ExecutionControl|ADKSubagentRunRecorder|ResumeHumanInteraction|RetrySubagent|JournalRecovery|RunLeaseRecovery|MapCanonicalApplicationError' -count=1
```

Expected: PASS。

### Task 4: Authority 与最终验证

**Files:**
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify if long-term fact changes: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`

- [ ] **Step 1: 更新事实**

记录 public application admission、私有 ADK child compatibility seam、历史恢复不校验，以及
P1M-B1 不等于 mode retirement/P1M PASS。不得把 P1L 标为完成。

- [ ] **Step 2: 完成 Go 验证**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread ./api/handler/coze -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread ./api/handler/coze -run '^$' -count=1
```

如完整 handler package 命中已知无关 fixture 失败，保存精确失败并以本计划所有 named focused
tests + compile-only 作为 scoped evidence；不得把失败隐瞒为全绿。

- [ ] **Step 3: 完成格式与图谱验证**

```bash
gofmt -w \
  backend/application/agentthread/retired_execution_control.go \
  backend/application/agentthread/retired_execution_control_test.go \
  backend/application/agentthread/service.go \
  backend/application/agentthread/service_test.go \
  backend/application/agentthread/adk_subagent_run_recorder.go \
  backend/application/agentthread/adk_subagent_run_recorder_test.go \
  backend/api/handler/coze/workbench_canonical_contract.go \
  backend/api/handler/coze/workbench_canonical_contract_test.go
git diff --check
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Expected: 全部 exit 0；build 如受 sandbox 权限阻断，只对同一 exact command 申请授权重跑。

- [ ] **Step 4: 独立规格/质量审查并提交**

规格审查确认入口、拒绝顺序、trusted seam、历史兼容与非目标；质量审查确认无 exported bypass、
无 caller-selected provenance、无无关文件。两者 P0/P1=0 后，按业务代码、authority/docs 两个
局部提交保存；不 stage/commit P1L 草稿，不 merge、不 push、不 deploy。
