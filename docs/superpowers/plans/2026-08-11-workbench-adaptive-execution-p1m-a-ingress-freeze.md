# Workbench Adaptive Execution P1M-A Ingress Freeze Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 临时硬关闭 canonical 整 Thread 删除，并在 canonical HTTP 与第一方前端冻结七个已退休的执行控制字段，同时保留后端兼容 mode、恢复链和已审核的 runtime/model/resource 配置。

**Architecture:** 删除入口在既有 dependency、workspace 鉴权和 path ID 校验后以编译期常量 fail closed，不进入 application/domain/repository。执行控制使用一个 handler 私有、保留 JSON key 顺序的 raw-body validator，在 typed binder 和任何写入前检查审核路径；前端复用现有 serializer，只删除退休字段与 reasoning 控件。P1L、typed admission、ADK/recovery consumer 退休均不在本计划中。

**Tech Stack:** Go 1.x、Hertz、encoding/json、React/TypeScript、Vitest、Rush、Workbench execution graph tooling。

---

## 实施边界

- 工作目录固定为 `/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp`。
- 当前实施基线为 `0451e6220069639b28a092c812adc7a0b7183d07`；开始写代码前必须重新核对。
- 保留且不修改未完成的 `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p1l-lock-order.md`。
- 不改 IDL、生成 client、application、ADK、recovery、repository、migration。
- 下列 commit 步骤仅在用户明确授权提交当前 exact diff 后执行；未授权时停留在 unstaged handoff。

### Task 1: 冻结基线和删除调用方

**Files:**
- Read: `docs/superpowers/specs/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze-design.md`
- Read: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Read: `backend/application/agentthread/service.go`
- Read: `backend/domain/agentthread/repository/repository.go`

- [ ] **Step 1: 核对工作树**

Run:

```bash
git rev-parse HEAD
git rev-parse origin/dev
git status --short
```

Expected:

```text
0451e6220069639b28a092c812adc7a0b7183d07
6a3086bb8f3e73a5e829701d5b41d76a408ca6ba
?? docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p1l-lock-order.md
?? docs/superpowers/plans/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze.md
?? docs/superpowers/specs/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze-design.md
```

若 tracked 文件已变化，停止并重新绑定计划；不得覆盖或清理用户改动。

- [ ] **Step 2: 用 codebase-memory 冻结删除调用链，再用源码复核**

先对 `DeleteThread` 和 `DeleteThreadIfIdle` 分别执行 inbound `trace_path`，随后运行：

```bash
rg -n '\.(DeleteThread|DeleteThreadIfIdle)\(' backend --glob '*.go'
```

Expected: 唯一网络生产入口是 `DeleteCanonicalThread -> ApplicationService.DeleteThreadIfIdle`；旧 `DeleteThread` 没有生产 handler/job caller。若出现其它生产 caller，停止本包，并把 hard-disable 下沉 application 边界覆盖两条 use case。

### Task 2: TDD 硬关闭整 Thread 删除

**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service.go`

- [ ] **Step 1: 将旧 handler happy-path 测试改成 hard-disable RED**

把 `TestDeleteCanonicalThreadDeletesIdleAndRejectsBusyWithoutCanceling` 改名为 `TestDeleteCanonicalThreadIsTemporarilyDisabledAfterWorkspaceAuthorizationWithoutMutation`，并增加只记录调用、不委托删除的 spy：

```go
import domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"

type canonicalDeleteSpyThreadService struct {
	domainservice.ThreadService
	deleteThreadIfIdleCalls int
}

func (s *canonicalDeleteSpyThreadService) DeleteThreadIfIdle(
	_ context.Context,
	_ *domainservice.DeleteThreadIfIdleRequest,
) (bool, error) {
	s.deleteThreadIfIdleCalls++
	return true, nil
}
```

测试使用 `installAgentThreadTestService(t)` 创建一个真实 idle Thread，保存真实 `ThreadSVC`，用上面的 spy 包装它，并安装现有 `canonicalRecordingWorkspaceAuthorizer`。分别对该 idle Thread 和一个合法但不存在的正整数 ID 发起 DELETE；两者都必须得到相同 503，且每个 subtest 精确断言：

```go
require.Equal(t, consts.StatusServiceUnavailable, recorder.Code)
require.Equal(t, "thread_delete_temporarily_disabled", body.Code)
require.Equal(t, "Thread deletion is temporarily unavailable", body.Detail)
require.False(t, body.Retryable)
require.NotEmpty(t, body.TraceID)
require.NotContains(t, recorder.Body.String(), "error_code")
require.Empty(t, recorder.Header().Get("Retry-After"))
require.Equal(t, 1, workspaceAuthorizer.calls)
require.Equal(t, 0, spy.deleteThreadIfIdleCalls)

persisted, err := realThreadSVC.GetThread(ctx, thread.ThreadID)
require.NoError(t, err)
require.Equal(t, thread.ThreadID, persisted.ThreadID)
```

不存在 ID 的 subtest 不做 Thread 读取；idle Thread subtest 才使用真实 service 做最终存在性检查。这样同时冻结“不泄露 Thread 是否存在”和“删除路径零调用”。

- [ ] **Step 2: 运行 RED**

Run from `backend/`:

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -run '^TestDeleteCanonicalThreadIsTemporarilyDisabledAfterWorkspaceAuthorizationWithoutMutation$' -count=1
```

Expected: FAIL；旧实现返回 204，且 spy 的 `DeleteThreadIfIdle` 调用数为 1。

- [ ] **Step 3: 加入最小编译期 hard guard**

在 `workbench_canonical_thread_service.go` 定义：

```go
const canonicalWholeThreadDeletionTemporarilyDisabled = true
```

在 `DeleteCanonicalThread` 已完成 dependency、workspace、path ID 和 request-log 设置之后、调用 `SVC.DeleteThreadIfIdle` 之前加入：

```go
if canonicalWholeThreadDeletionTemporarilyDisabled {
	public := newCanonicalError(
		consts.StatusServiceUnavailable,
		"thread_delete_temporarily_disabled",
		"Thread deletion is temporarily unavailable",
		"thread_delete_disabled",
		false,
	)
	writeCanonicalError(ctx, c, public.status, *public)
	return
}
```

同步修正 handler 注释：route 保留，但当前在 workspace 鉴权和 path 校验后返回固定 503；不要改 middleware、route、IDL 或 lower-layer 删除实现。

- [ ] **Step 4: 运行 GREEN 和错误优先级回归**

```bash
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -run '^(TestDeleteCanonicalThreadIsTemporarilyDisabledAfterWorkspaceAuthorizationWithoutMutation|TestCanonicalThreadResourceHandlersFailClosedWithoutApplicationService|TestCanonicalCoreEntrypointsRequireAuthorizedSpaceHeader|TestCanonicalParseIDRejectsZeroNegativeAndNonDecimal|TestCanonicalSpaceIDRequiresAuthorizedHeader)$' -count=1
```

Expected: PASS；缺 dependency、缺/非法 workspace、workspace denied、非法 path ID 仍保持原错误优先级。

- [ ] **Step 5: 经用户授权后提交**

```bash
git add backend/api/handler/coze/workbench_canonical_thread_service.go backend/api/handler/coze/workbench_canonical_thread_service_test.go
git commit -m "fix: temporarily disable whole-thread deletion"
```

### Task 3: TDD 实现 raw execution-control validator

**Files:**
- Create: `backend/api/handler/coze/workbench_canonical_execution_control_validator.go`
- Create: `backend/api/handler/coze/workbench_canonical_execution_control_validator_test.go`

- [ ] **Step 1: 写 validator 单元 RED**

新增两个表驱动测试：

```go
func TestCanonicalExecutionControlIngressValidator(t *testing.T)
func TestCanonicalExecutionControlIngressBudgets(t *testing.T)
```

第一个测试固定以下七字段顺序，并覆盖 root、`config`、`context`、保留容器 `configurable/context`、Create Thread 的 immediate/deferred 路径、mixed-case path、归一化重复 key，以及合法字符串/数组/普通资源对象不扫描：

```go
var canonicalRetiredExecutionControlKeys = [...]string{
	"requested_policy",
	"mode",
	"thinking_enabled",
	"reasoning_effort",
	"is_plan_mode",
	"subagent_enabled",
	"max_concurrent_subagents",
}
```

精确断言错误：控制字段为 422/`unsupported_execution_control`/stable lowercase path；归一化重复 key 为 400/`invalid_json`；深度、对象数、路径预算为 422/`invalid_request`。第二个测试直接驱动内部 budget helper，确保 4 hops、64 objects、256-byte path 的边界值通过，超一拒绝。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -run '^(TestCanonicalExecutionControlIngressValidator|TestCanonicalExecutionControlIngressBudgets)$' -count=1
```

Expected: FAIL with undefined validator symbols。

- [ ] **Step 3: 实现 ordered raw-body validator**

新文件公开给同 package 的唯一入口固定为：

```go
type canonicalExecutionControlIngressKind uint8

const (
	canonicalExecutionControlCreateThread canonicalExecutionControlIngressKind = iota
	canonicalExecutionControlRunSubmission
	canonicalExecutionControlRootOnly
)

func validateCanonicalExecutionControlIngress(
	raw []byte,
	kind canonicalExecutionControlIngressKind,
) *canonicalError
```

实现使用 `json.Decoder.Token` 建立保留字段顺序的私有节点，而不是先落 `map[string]any`：

```go
type canonicalOrderedJSONField struct {
	name  string
	value canonicalOrderedJSONValue
}

type canonicalOrderedJSONValue struct {
	fields []canonicalOrderedJSONField
	items  []canonicalOrderedJSONValue
	kind   byte // 'o', 'a', or 's'
}
```

同文件定义并完整实现以下私有 helper：

```go
func decodeCanonicalOrderedJSON(raw []byte) (canonicalOrderedJSONValue, error)
func decodeCanonicalOrderedJSONValue(decoder *json.Decoder) (canonicalOrderedJSONValue, error)
func validateCanonicalExecutionControlValue(value canonicalOrderedJSONValue, path string, hops int, budget *canonicalExecutionControlBudget) *canonicalError
func canonicalExecutionControlField(name string) (string, bool)
func canonicalExecutionControlReservedContainer(name string) bool
func canonicalExecutionControlError(path string) *canonicalError
func canonicalExecutionControlDuplicateKeyError() *canonicalError
```

算法固定为：

1. 语法错误交回现有 binder 处理；只有归一化重复 key 在 validator 返回 `invalid_json`；
2. 每个被审核 object 先按 ASCII lowercase 检测重复 key；
3. 按上面的七字段数组顺序查控制字段，不按 map/token 原始顺序决定错误；
4. `CreateThread` 审核 root，再按 `coze.initial_run`、`coze.deferred_initial_run` 的固定顺序进入其 `config`/`context`；Run 审核 root、`config`、`context`；RootOnly 只审核 root；
5. 进入 `config`/`context` 后，只沿 object-valued `configurable` 和 `context` 递归；不进入数组或其它业务对象；
6. 大小写匹配只做 ASCII fold，错误路径输出 lowercase canonical key；
7. `hops > 4`、`objects > 64` 或 `len(path) > 256` 返回 `canonicalInvalidRequest(...)`；
8. 命中使用 `newCanonicalError(422, "unsupported_execution_control", "Unsupported execution control: "+path, "unsupported_execution_control", false)`。

- [ ] **Step 4: 运行 validator GREEN**

重复 Step 2 命令。

Expected: PASS。

### Task 4: TDD 将 validator 接到全部 canonical HTTP 入口

**Files:**
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_usage_retry_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_usage_retry_service_test.go`

- [ ] **Step 1: 写 route-level RED**

新增以下精确测试：

```go
func TestCreateCanonicalThreadRejectsExecutionControlsBeforeMutation(t *testing.T)
func TestCanonicalRunRoutesRejectExecutionControlsBeforeMutation(t *testing.T)
func TestCanonicalResumeRejectsExecutionControlsBeforeApplication(t *testing.T)
func TestStreamCanonicalRunRejectsExecutionControlsBeforeSSE(t *testing.T)
func TestCanonicalSubagentRunRetryRejectsExecutionControlsBeforeApplication(t *testing.T)
```

Create Thread 表覆盖 immediate 与 deferred；Run 表覆盖 create、wait 和 top-level retry；Stream、Resume、Subagent Retry 各独立覆盖。每例精确断言 422、`unsupported_execution_control`、`retryable=false`、stable path，并用已有 fake/计数查询证明 Thread/Message/Run 写入和 application 调用均为 0。保留或扩展 `TestCanonicalRunDoesNotTreatUserMessageAsRuntimeConfiguration`，证明字符串中的 `requested_policy`/`mode` 不触发 validator；另加合法 `runtime=eino_adk`、model/Skill/MCP/knowledge/database 配置通过例。

- [ ] **Step 2: 运行 route RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -run '^(TestCreateCanonicalThreadRejectsExecutionControlsBeforeMutation|TestCanonicalRunRoutesRejectExecutionControlsBeforeMutation|TestCanonicalResumeRejectsExecutionControlsBeforeApplication|TestStreamCanonicalRunRejectsExecutionControlsBeforeSSE|TestCanonicalSubagentRunRetryRejectsExecutionControlsBeforeApplication)$' -count=1
```

Expected: FAIL；旧入口未返回专用控制错误。

- [ ] **Step 3: 在 binder 前接线**

接线位置和代码形状固定为：

```go
// CreateCanonicalThread: body limit 后、decodeCanonicalJSON 前
if public := validateCanonicalExecutionControlIngress(
	c.Request.Body(), canonicalExecutionControlCreateThread,
); public != nil {
	writeCanonicalError(ctx, c, public.status, *public)
	return
}

// parseCanonicalRunSubmission: body limit 后、decodeCanonicalJSON 前
if public := validateCanonicalExecutionControlIngress(
	c.Request.Body(), canonicalExecutionControlRunSubmission,
); public != nil {
	return nil, public
}

// ResumeCanonicalRun: body limit 后、decodeCanonicalJSON 前
if public := validateCanonicalExecutionControlIngress(
	c.Request.Body(), canonicalExecutionControlRootOnly,
); public != nil {
	writeCanonicalError(ctx, c, public.status, *public)
	return
}

// RetryCanonicalSubagentRun: workspace/path 鉴权后，保留普通非空 body 的原错误
if public := canonicalRequestBodyLimit(c, "Run"); public != nil {
	writeCanonicalError(ctx, c, public.status, *public)
	return
}
if public := validateCanonicalExecutionControlIngress(
	c.Request.Body(), canonicalExecutionControlRootOnly,
); public != nil {
	writeCanonicalError(ctx, c, public.status, *public)
	return
}
if public := canonicalRejectNonEmptyBody(c); public != nil {
	writeCanonicalError(ctx, c, public.status, *public)
	return
}
```

Stream 不改生产文件，因为它复用 `parseCanonicalRunSubmission`。不得把 denylist 放进 `validateCanonicalPersistedRunValue`，避免误拒 metadata/资源对象。

- [ ] **Step 4: 运行完整后端 focused GREEN**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -run 'Test(CanonicalExecutionControlIngressValidator|CanonicalExecutionControlIngressBudgets|CreateCanonicalThreadRejectsExecutionControlsBeforeMutation|CanonicalRunRoutesRejectExecutionControlsBeforeMutation|CanonicalResumeRejectsExecutionControlsBeforeApplication|StreamCanonicalRunRejectsExecutionControlsBeforeSSE|CanonicalSubagentRunRetryRejectsExecutionControlsBeforeApplication|DeleteCanonicalThreadIsTemporarilyDisabledAfterWorkspaceAuthorizationWithoutMutation)$' -count=1
```

Expected: PASS。

- [ ] **Step 5: 经用户授权后提交**

```bash
git add backend/api/handler/coze/workbench_canonical_execution_control_validator.go backend/api/handler/coze/workbench_canonical_execution_control_validator_test.go backend/api/handler/coze/workbench_canonical_thread_service.go backend/api/handler/coze/workbench_canonical_thread_service_test.go backend/api/handler/coze/workbench_canonical_run_service.go backend/api/handler/coze/workbench_canonical_run_service_test.go backend/api/handler/coze/workbench_canonical_run_stream_test.go backend/api/handler/coze/workbench_canonical_usage_retry_service.go backend/api/handler/coze/workbench_canonical_usage_retry_service_test.go
git commit -m "feat: freeze canonical execution controls"
```

### Task 5: TDD 清理第一方前端 serializer 与 reasoning 控件

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/types.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/workbench-runtime-settings-control.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-follow-up.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/canonical-frontend-contract.test.ts`

- [ ] **Step 1: 将前端正向旧字段断言改成 RED**

在现有 serializer/new-task/upload/follow-up/retry 测试中统一使用：

```ts
const retiredExecutionControlKeys = [
  'requested_policy',
  'mode',
  'thinking_enabled',
  'reasoning_effort',
  'is_plan_mode',
  'subagent_enabled',
  'max_concurrent_subagents',
] as const;

for (const key of retiredExecutionControlKeys) {
  expect(config).not.toHaveProperty(key);
  expect(metadata).not.toHaveProperty(key);
  expect(messageMetadata).not.toHaveProperty(key);
}
expect(config.runtime).toBe('eino_adk');
```

新增 `hides client reasoning controls from runtime settings`：打开“运行设置”，断言“模型推理”及其按钮不存在。扩展 `keeps production task and workbench sources free of retired contracts`，禁止生产 Workbench/Task writer 再出现 `requested_policy` 或序列化 `reasoning_effort`。

- [ ] **Step 2: 运行前端 RED**

Run from `frontend/apps/coze-studio/`:

```bash
rushx test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx src/pages/tasks/__tests__/task-follow-up.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
```

Expected: FAIL；当前 serializer/metadata 仍输出旧字段，reasoning row 仍渲染。

- [ ] **Step 3: 写最小前端实现**

`types.ts` 删除 `WORKBENCH_REQUESTED_POLICY`，让 `createWorkbenchRunConfig` 只返回合法配置：

```ts
return {
  runtime: runtimeSettings.runtime,
  model_type: payload.modelType,
  model_name: payload.modelName,
  memory_retrieval: runtimeSettings.memory_retrieval,
  skills: runtimeSettings.skills,
  mcp_tools: mcpTools,
  web_tools: runtimeSettings.web_tools,
  model_retry: modelRetry,
  model_failover: modelFailover,
  token_usage: runtimeSettings.token_usage,
  enable_skills: payload.enable_skills,
  enable_mcp: payload.enable_mcp,
  enable_kbs: payload.enable_kbs,
  enable_databases: payload.enable_databases,
};
```

以真实现有返回字段为准保留全部合法资源字段，但删除 `requested_policy` 和条件 `reasoning_effort` 分支。`index.tsx` 与 `task-follow-up.ts` 的 metadata 只保留：

```ts
JSON.stringify({ source: 'workbench_new_task' })
JSON.stringify({ source: 'workbench_detail_followup' })
```

`getThreadFollowUpRunMetadata` 删除已无用途的 `payload` 参数，并把调用点改为 `getThreadFollowUpRunMetadata()`。

删除 `WorkbenchReasoningSettingsRow`、`REASONING_OPTIONS`、不再使用的 `WorkbenchReasoningEffort` import 和 panel 中的渲染；内部 `types.ts` reasoning 类型/default 可继续存在，但 UI 不展示、serializer 不读取。

- [ ] **Step 4: 运行前端 GREEN、lint 和 build**

```bash
rushx test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx src/pages/tasks/__tests__/task-follow-up.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
rushx lint
rushx build
```

Expected: 全部 exit 0；上传顺序、导航、follow-up、retry、model/resource 断言继续通过。

- [ ] **Step 5: 经用户授权后提交**

```bash
git add frontend/apps/coze-studio/src/pages/workbench/components/types.ts frontend/apps/coze-studio/src/pages/workbench/components/workbench-runtime-settings-control.tsx frontend/apps/coze-studio/src/pages/workbench/index.tsx frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx frontend/apps/coze-studio/src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/task-follow-up.test.ts frontend/apps/coze-studio/src/pages/tasks/__tests__/task-detail.test.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
git commit -m "refactor: remove retired workbench execution controls"
```

### Task 6: 同步长期事实与执行图

**Files:**
- Modify: `docs/superpowers/context/workbench-chat.md`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Add: `docs/superpowers/specs/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze-design.md`
- Add: `docs/superpowers/plans/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze.md`

- [ ] **Step 1: 更新事实，不扩大完成声明**

文档必须明确写入以下事实：

```text
P1M-A only:
- canonical whole-Thread DELETE is compile-time disabled after workspace auth/path validation;
- canonical HTTP rejects seven retired client execution-control keys before persistence;
- first-party Workbench no longer emits requested_policy/reasoning_effort and hides reasoning selection;
- backend compatibility mode, recovery inheritance and ADK consumers remain;
- P1L is deferred, P1M is not PASS, and P1D remains blocked by full P1M plus P1R;
- re-enabling DELETE requires exact-SHA P1L writer inventory, both cascade paths, real-MySQL races, cleanup and 204/409 regression evidence.
```

删除 `workbench-chat.md` 中“第一方固定发送 requested_policy=auto/可选择 reasoning”的旧事实。执行图更新 canonical Thread/Run HTTP 节点、source locator 和测试证据；route 数量不变。只有结构投影实际变化时才按工具报告更新 `scripts/workbench-execution-graph/contract.mjs` 的单个 digest，不预先修改它。

- [ ] **Step 2: 验证并重建执行图**

Run from worktree root:

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Expected: 三条命令 exit 0。若 build 报 graph/AST/contract mismatch，停止并报告真实错误；不得跳过、伪造 PASS 或重复改 digest 猜值。

- [ ] **Step 3: 经用户授权后提交**

```bash
git add docs/superpowers/context/workbench-chat.md docs/superpowers/context/project-context.md docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md docs/superpowers/specs/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze-design.md docs/superpowers/plans/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze.md
git commit -m "docs: record P1M-A ingress freeze"
```

不要 stage 未完成的 P1L plan。

### Task 7: Fresh 验证与交付

**Files:**
- Verify only: all files listed above

- [ ] **Step 1: 格式化并运行完整相关后端包**

```bash
gofmt -w backend/api/handler/coze/workbench_canonical_execution_control_validator.go backend/api/handler/coze/workbench_canonical_execution_control_validator_test.go backend/api/handler/coze/workbench_canonical_thread_service.go backend/api/handler/coze/workbench_canonical_thread_service_test.go backend/api/handler/coze/workbench_canonical_run_service.go backend/api/handler/coze/workbench_canonical_run_service_test.go backend/api/handler/coze/workbench_canonical_run_stream_test.go backend/api/handler/coze/workbench_canonical_usage_retry_service.go backend/api/handler/coze/workbench_canonical_usage_retry_service_test.go
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./api/handler/coze -count=1
```

Expected: package PASS，无 skip/fail。

- [ ] **Step 2: Fresh 运行前端相关测试、lint 和 build**

```bash
cd frontend/apps/coze-studio
rushx test -- src/pages/workbench/__tests__/workbench.test.tsx src/pages/workbench/components/__tests__/workbench-controls-disabled.test.tsx src/pages/tasks/__tests__/task-follow-up.test.ts src/pages/tasks/__tests__/task-detail.test.tsx src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
rushx lint
rushx build
```

Expected: 全部 exit 0。

- [ ] **Step 3: 运行静态安全检查**

Run from worktree root:

```bash
rg -n '\.(DeleteThread|DeleteThreadIfIdle)\(' backend --glob '*.go'
rg -n 'THREAD_DELETE_ENABLED|WORKBENCH_THREAD_DELETE|os\.LookupEnv|os\.Getenv' backend/api/handler/coze/workbench_canonical_thread_service.go
rg -n 'WORKBENCH_REQUESTED_POLICY|requested_policy' frontend/apps/coze-studio/src/pages/workbench/components/types.ts frontend/apps/coze-studio/src/pages/workbench/index.tsx frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts
git diff --check
git status --short
```

Expected:

- 删除 caller 集合与 Task 1 一致，没有新增生产 caller；
- 删除 handler 没有 env bypass；
- 前端三个生产 writer 扫描无输出（`rg` exit 1），不再包含 `WORKBENCH_REQUESTED_POLICY`/`requested_policy`；
- `git diff --check` 无输出；
- modified/untracked 集合仅为本计划明确文件，加上未触碰的 P1L plan。

- [ ] **Step 4: 最终人工 diff 审查**

```bash
git diff --stat
git diff -- backend/api/handler/coze frontend/apps/coze-studio/src/pages/workbench frontend/apps/coze-studio/src/pages/tasks docs/superpowers/context docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md docs/superpowers/specs/2026-08-11-workbench-adaptive-execution-p1m-a-ingress-freeze-design.md
```

确认：route/IDL 未删；子资源 DELETE 不变；validator 在 binder/application 前；合法 runtime/model/resources 保留；没有 P1M/P1L/P1D 完成宣称。随后交付 fresh 命令结果、exact 文件范围、未运行项和剩余 blocker，等待用户决定是否提交/合并。
