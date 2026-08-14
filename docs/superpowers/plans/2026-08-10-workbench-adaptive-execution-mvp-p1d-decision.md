# Workbench Adaptive Execution MVP P1D Decision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用 server-owned、默认关闭且可稳定按空间放量的 P1D 纵切，让 fresh 顶层 Eino Run 通过真实 model producer 生成 gate-on typed decision，并在既有 fenced bootstrap 中持久化，同时保持 normal/resume 共用同一 authority。

**Architecture:** fresh bootstrap 保持 durable read-first；exact miss 才冻结 eligibility，并把 authoritative `Run.Input` closed-project 为最多 64 KiB/32 条 `user`/`assistant` content 与 attachment bool。Production gate-on 使用显式正十进制 `AGENT_THREAD_ADAPTIVE_DECISION_MODEL_ID`、独立 30 秒 timeout、唯一 forced `adaptive_execution_decision` tool、closed output schema、共享 usage/billing 和窄 durable claim/result operation；provider Generate 总预算为 1。Coordinator 覆盖 candidate 的 durable identity 并 strict validate 后调用既有原子 bootstrap；typed Resume 复制 source durable candidate、绝不调用模型，legacy 永远 gate-off。

**Tech Stack:** Go、Eino ADK、GORM/MySQL、现有 `adaptivecontract` canonical codec、`go test`、Workbench execution graph verifier。

**Status:** `in_progress`；真实 model producer、durable runtime consumer、安全 public v1 投影与
TaskDetail 最新顶层 Run 四态标签已交付，但 clarification Human Interaction consumer 尚未接入，MySQL
exact gate 未在安全 disposable DSN 上验证，holdout、progress/verification 等 P1D 退出门仍未闭合，因此
`P1D NOT PASS`。

---

## Scope and file map

**In scope**

- `backend/application/agentthread/adaptive_eligibility.go`：每次 `Resolve` 严格解析 server-owned 双 env，计算稳定 space eligibility；默认 disabled/`0`，enabled + `10000` 全量开启。
- `backend/application/agentthread/adaptive_decision_producer.go`、`adaptive_decision_model_contract.go`、`adaptive_decision_model_producer.go`：统一 producer interface、authoritative semantic projection、model tool/output contract 与真实 gate-on producer。
- `backend/application/agentthread/adaptive_bootstrap_coordinator.go`：fresh read-first 后 resolver → producer → strict validate → IDs → atomic commit；Resume 继承 typed gate，不重算。
- `backend/application/agentthread/adk_agent_factory.go`：把已有 decision 合同收口成 direct/single-step/multi-step runtime capability；不另建 executor。
- `backend/application/agentthread/adk_middleware.go`、`adk_adaptive_decision_guard.go`、`adk_executor.go`：
  direct 禁止全部工具暴露并拦截模型违规 tool call；clarification 在 bootstrap 后、runtime dependency 前
  fail closed；typed Resume 复用同一 consumer。
- `backend/domain/agentthread/repository/mysql_adaptive_bootstrap.go`、
  `backend/application/agentthread/adaptive_decision_public.go` 与 canonical projection：按 execution Run
  strict 读取 durable pair，只发布 public v1 五字段。
- Workbench canonical adapter 与 TaskDetail loader/header：strict decode optional v1，并在最新 primary
  top-level Run `enabled=true` 时展示四态标签。
- `backend/application/application.go`：构造 env resolver、baseline producer、`ModelAdaptiveDecisionProducer`、共享 usage collector 与 durable operation repository，并显式注入 30 秒 model timeout。
- `backend/domain/agentthread/repository/mysql_adaptive_decision_model.go`：以 internal/unsequenced claim/result events 实现 narrow durable operation；single winner，raw key/token 不落库。
- 对应 `*_test.go`、已有 disposable MySQL bootstrap integration test、两份 Workbench authority context 与 execution graph JSON。

**Out of scope**

- progress、repair/replan、verification、IDL、migration、holdout、acceptance-check registry、Journal
  enrollment 改造、Subagent 开启，以及 clarification 的 P2 Human Interaction consumer。
- 不允许从 `Run.Config`、`Run.Context` 或 Journal 内容推断 gate/decision；模型只能读取审核后的 `Run.Input` semantic projection，不允许把 env 或 producer candidate 作为 durable identity 权威。

**本轮交付状态**

- 本轮已交付 server-owned eligibility 与真实 model-backed gate-on 运行闭环，但不能据此宣称 `P1D PASS`。
- 禁止以关键字、正则、prompt/body/message 扫描或 legacy config/Journals 启发式替代真实 producer。
- runtime consumer 已交付：direct 继续走同一 ADK text path 但没有任何工具暴露，single-step 保留受控
  tools 但 Plan/Subagent off，multi-step Plan on/Subagent off；clarification 暂时 fail closed。
- public v1 与 TaskDetail 四态标签已交付；历史缺失省略、partial/corrupt fail closed，`enabled=false`
  隐藏。
- 剩余 exit gate 必须完成安全 disposable MySQL exact 验证、clarification Human consumer、holdout、
  progress/verification 与其余既定验收；通过前 P1D 保持 `in_progress`。

**Frozen interfaces**

```go
const (
	agentThreadAdaptiveExecutionEnabledEnv = "AGENT_THREAD_ADAPTIVE_EXECUTION_ENABLED"
	agentThreadAdaptiveExecutionRolloutBasisPointsEnv = "AGENT_THREAD_ADAPTIVE_EXECUTION_ROLLOUT_BASIS_POINTS"
	adaptiveExecutionEligibilityFeature = "workbench_adaptive_execution_mvp"
)

type AdaptiveEligibilityRequest struct {
	SpaceID int64
}

type AdaptiveEligibilityResolver interface {
	Resolve(context.Context, AdaptiveEligibilityRequest) (entity.AdaptiveAdmissionSnapshot, error)
}

type AdaptiveDecisionProducer interface {
	Produce(context.Context, AdaptiveDecisionRequest) (AdaptiveDecisionCandidate, error)
}

type AdaptiveDecisionRequest struct {
	Admission     entity.AdaptiveAdmissionSnapshot
	SemanticInput AdaptiveDecisionSemanticInput
}

type AdaptiveDecisionSemanticInput struct {
	Messages       []AdaptiveDecisionSemanticMessage
	HasAttachments bool
}

type AdaptiveDecisionSemanticMessage struct {
	Role    string
	Content string
}

type AdaptiveDecisionCandidate struct {
	GoalSummary           string
	Deliverables          []string
	AcceptanceChecks      []entity.AdaptiveAcceptanceCheck
	Decision              entity.ExecutionDecisionKind
	ExecutionShape        entity.ExecutionShape
	ClarificationQuestion *string
	SafeSummary           string
}
```

`AdaptiveDecisionCandidate` 有意不含 decision ID、revision、Run/Journal/Attempt/generation、Plan scope 或 created-at。Coordinator 必须覆盖这些 server-owned identity 字段；producer request 只携带 admission 与审核后的语义投影，不能携带或伪造 durable authority。

### Task 1: Stable server-owned admission gate

**Files:**

- Create: `backend/application/agentthread/adaptive_eligibility.go`
- Create: `backend/application/agentthread/adaptive_eligibility_test.go`

- [x] **Step 1: 写 resolver RED。** 覆盖双 env 未设置时默认 gate-off；只有 `AGENT_THREAD_ADAPTIVE_EXECUTION_ENABLED=true` 且 rollout `10000` 才对有效 `SpaceID` 全开；中间值对同一 space 重复调用稳定；非法 boolean/integer、负数、`>10000`、无效 space 均 fail closed。断言 bucket 输入只含固定 feature key `workbench_adaptive_execution_mvp` 与 `SpaceID`。
- [x] **Step 2: 运行 RED。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestEnvAdaptiveEligibilityResolver|TestAdaptiveEligibilityBucket' -count=1
  ```

  Expected: FAIL，因为 resolver 与 env parser 尚不存在。

- [x] **Step 3: 最小实现。** 使用 SHA-256 对 `workbench_adaptive_execution_mvp\x00<space_id>` 取 `[0,10000)` bucket；`NewEnvAdaptiveEligibilityResolver()` constructor 只返回无状态 resolver，每次 `Resolve` 都严格解析双 env。fresh snapshot 复用当前 capabilities/limits，只有 `FeatureGateEnabled` 由 eligibility 决定；任一 env 非法时本次 bootstrap 在 producer、ID 与 commit 前 fail closed，不做启动时 fail-fast。
- [x] **Step 4: 运行 GREEN 并随首包提交。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestEnvAdaptiveEligibilityResolver|TestAdaptiveEligibilityBucket' -count=1
  ```

  Expected: PASS；提交只含 resolver 与测试。

### Task 2: Independent deterministic gate-on producer

**Files:**

- Create: `backend/application/agentthread/adaptive_decision_producer.go`
- Create: `backend/application/agentthread/adaptive_decision_producer_test.go`

- [x] **Step 1: 写 producer RED。** 固定统一 interface；baseline adapter 只接受 gate-off 并保持当前 fixed multi-step；deterministic producer 只接受 gate-on 并固定产生保守 `execute/multi_step` candidate。覆盖 gate 不匹配、blocked Plan capability 与 context cancellation；producer request 只能含 Admission，不能观察 durable authority。
- [x] **Step 2: 运行 RED。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestBaselineAdaptiveDecisionProducer|TestDeterministicAdaptiveDecisionProducer|TestAdaptiveDecisionProducerContract' -count=1
  ```

  Expected: FAIL，因为独立 producer contract 与 gate-on implementation 尚不存在。

- [x] **Step 3: 最小实现。** 在 `adaptive_decision_producer.go` 同文件实现 interface、baseline adapter 与 `DeterministicAdaptiveDecisionProducer`；deterministic gate-on producer 固定选择 `execute/multi_step`，不提供 task-aware direct/single-step 分类。Coordinator 的 `produceDecision` 深拷贝 candidate，覆盖全部 identity 与 multi-step PlanScope，并调用 `ValidateAdaptiveBootstrapPair`。producer 不读取 `RunSummary` 或 JSON。
- [x] **Step 4: 运行 GREEN 并随首包提交。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestBaselineAdaptiveDecisionProducer|TestDeterministicAdaptiveDecisionProducer|TestAdaptiveDecisionProducerContract' -count=1
  ```

  Expected: PASS；candidate 无 durable identity，gate-on/off producer 互斥。

### Task 3: Fresh atomic bootstrap and typed Resume inheritance

**Files:**

- Modify: `backend/application/agentthread/adaptive_bootstrap_coordinator.go`
- Modify: `backend/application/agentthread/adaptive_bootstrap_coordinator_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_bootstrap.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_bootstrap_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_bootstrap_integration_test.go`
- Modify: `backend/application/application.go`

- [x] **Step 1: 写 coordinator RED。** fresh exact replay 必须在 resolver/producer/clock/ID 前返回；fresh miss 次序为 resolver → producer → materialize/strict validate → `GenMultiIDs(3)` → existing atomic commit。分别覆盖 gate-off baseline 与 gate-on deterministic；resolver error、缺 producer、producer error、invalid candidate 在分配 ID、commit、runtime 前零副作用。candidate 不持有 identity，全部 durable authority 由 coordinator materialize。typed Resume 从 source admission 继承 gate/capability/limits 并使用匹配 producer，不读取实时 resolver；multi-hop 不重算；legacy decoder 始终 gate-off。
- [x] **Step 2: 运行 RED。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestAdaptiveBootstrapCoordinator.*(Gate|Producer|Replay|Resume|Legacy)' -count=1
  ```

  Expected: FAIL，当前 coordinator 固定 baseline 且拒绝 gate-on typed source。

- [x] **Step 3: 最小实现与 repository 放行。** 给 coordinator options 增加 eligibility resolver、baseline producer、adaptive producer；durable replay 路径不读取实时 eligibility。删除 application/repository 对 typed `FeatureGateEnabled=true` 的硬拒绝，但保持 exact source gate/capability/limits 相等、lineage、lease/generation 和 canonical pair 校验。`application.go` 注入 `NewEnvAdaptiveEligibilityResolver()`、baseline adapter 与 deterministic producer；constructor 不读 env，非法配置在本次 fresh `Resolve` 中零提交 fail closed。
- [x] **Step 4: 添加 dev disposable MySQL 门。** 扩展既有 integration harness，覆盖 gate-on fresh commit/exact replay 与 typed Resume gate inheritance/readback；SQLite 覆盖 gate-on→off 漂移零写入。真实 MySQL 只接受 `COZE_AGENTTHREAD_TEST_MYSQL_DSN` 指向名称含 `agentthread_disposable` 的 dev 隔离库，并要求 frozen exact gate `COZE_AGENTTHREAD_TEST_ALLOW_DDL=I_UNDERSTAND_DISPOSABLE_DB`；缺门禁明确 SKIP/`NOT_VERIFIED`，绝不连接业务 dev 库。
- [x] **Step 5: 运行 GREEN 并提交。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestAdaptiveBootstrapCoordinator' -count=1
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/domain/agentthread/repository -run 'TestAdaptiveExecutionBootstrap' -count=1
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/domain/agentthread/repository -run 'TestAdaptiveExecutionBootstrapMySQLIntegrationGateOnFreshReplayAndTypedResume' -count=1 -v
  # 真机门仅在显式提供 disposable DSN，并设置：
  # COZE_AGENTTHREAD_TEST_ALLOW_DDL=I_UNDERSTAND_DISPOSABLE_DB
  ```

  Expected: unit/repository PASS；无隔离 DSN/DDL gate 时真实 MySQL 用例明确 SKIP，只有具备两项合规 dev disposable 门禁并真实 PASS 后才能把 MySQL 记为 VERIFIED。

### Task 4: Runtime decision consumption, authority sync, and one combined review

**Files:**

- Modify: `backend/application/agentthread/adk_agent_factory_test.go`
- Verify: `backend/application/agentthread/adk_agent_factory.go`
- Verify: `backend/application/agentthread/adk_executor_test.go`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-chat.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`

- [x] **Step 1: 扩展 runtime 合同测试。** 用真实 factory input 覆盖 gate-on direct 关闭 Plan、single-step 关闭 Plan、multi-step 开启既有 Plan；已有测试继续覆盖非法/不匹配 facts fail closed、Execute bootstrap-before-runtime、Resume 使用继承事实，以及 Subagent/thinking/reasoning 关闭。本首包 deterministic producer 只实际产出 multi-step，direct/single-step 仅冻结未来 typed producer consumer 合同。
- [x] **Step 2: 运行 runtime GREEN。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/application/agentthread -run 'TestADKAgentFactoryUsesAdaptiveFacts|TestADKExecutor.*Bootstrap' -count=1
  ```

  Expected: PASS；生产代码已按 `execute/multi_step` 唯一开启 Plan，其它合法 decision shape 关闭 Plan。

- [x] **Step 3: 核实现有 runtime consumer，无需生产修改。** `ApplicationADKAgentFactory.Build` 已只从 durable facts 推导 Plan capability：仅 execute/multi-step 开 Plan；direct 与 execute/single-step 不开 Plan。继续使用现有 ADK runner、middleware、tool provider 与 checkpoint store；不新增 runtime selector/executor，不对 direct 伪造 Plan 或 verification。
- [x] **Step 4: 更新权威。** 记录 exact 双 env、`workbench_adaptive_execution_mvp` feature key、per-Resolve strict fail closed、默认 off/`enabled=true + 10000bp` 全开、fresh read-first、typed Resume gate inheritance、legacy gate-off、deterministic producer 固定保守 multi-step 及 out-of-scope；同步 execution graph 的节点/边/源码锚点。状态必须写 `in_progress` / `P1D NOT PASS`。
- [x] **Step 5: 运行全量验证。**

  ```bash
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./backend/domain/agentthread/entity ./backend/domain/agentthread/adaptivecontract ./backend/domain/agentthread/repository ./backend/application/agentthread -count=1
  go test ./backend/api/handler/coze -run '^$'
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
  node scripts/workbench-execution-graph.mjs build
  node scripts/workbench-execution-graph.mjs verify-derived
  git diff --check
  ```

  Expected: 全部 PASS；若真实 MySQL 未提供合规 DSN，交付报告保留 `NOT_VERIFIED`，不得冒充通过。

- [x] **Step 6: 做一次合并审查并修复。** 只做一次覆盖代码、合同、测试和安全边界的 combined review；修复全部 P0/P1 后重跑 Step 5。除非出现 P0/破坏性变更或用户明确要求，不增加第二轮审核。
- [x] **Step 7: 分离首包代码与权威提交。**

  ```bash
  git add backend/application/agentthread/adaptive_eligibility.go backend/application/agentthread/adaptive_eligibility_test.go backend/application/agentthread/adaptive_decision_producer.go backend/application/agentthread/adaptive_decision_producer_test.go backend/application/agentthread/adaptive_bootstrap_coordinator.go backend/application/agentthread/adaptive_bootstrap_coordinator_test.go backend/application/agentthread/adk_agent_factory_test.go backend/domain/agentthread/repository/mysql_adaptive_bootstrap.go backend/domain/agentthread/repository/mysql_adaptive_bootstrap_test.go backend/domain/agentthread/repository/mysql_adaptive_bootstrap_integration_test.go backend/application/application.go
  git commit -m "feat(agentthread): bootstrap conservative adaptive decisions"
  git add docs/superpowers/context/project-context.md docs/superpowers/context/workbench-chat.md docs/superpowers/context/workbench-execution-chain.md docs/superpowers/context/workbench-execution-graph.json
  git commit -m "docs(workbench): record adaptive decision first packet"
  ```

  Expected: 工作树除用户既有 untracked 文件外无本计划引入的脏改动；不 stage 用户文件，不 merge/push/deploy/apply migration。

## Acceptance boundary

- 双 env 未设置为 gate-off；只有 `AGENT_THREAD_ADAPTIVE_EXECUTION_ENABLED=true` 且 rollout 为 `10000` 时所有有效空间 gate-on；feature key 固定为 `workbench_adaptive_execution_mvp`，空间 bucket 稳定且 server-owned。
- env constructor 不预读配置；每次 fresh `Resolve` 严格解析双 env，非法配置在 producer/ID/commit/runtime 前 fail closed。
- fresh exact replay 不调用 resolver/producer、不分配 ID；fresh miss 只提交一次既有 atomic bootstrap。
- typed Resume 原样继承 gate/capability/limits，实时 env 变化不影响 lineage；legacy 永远 gate-off。
- producer candidate 的 server identity 全被 coordinator 覆盖并 strict validate；任何 dependency/candidate/repository 错误均在 runtime 前 fail closed。
- gate-on 使用真实 model producer；只有唯一 forced tool 的 closed typed candidate 才能进入 materialize。一次 provider Generate attempt 是本阶段的有限预算，timeout、provider/model、usage 或 codec 错误全部 fail closed，禁止 deterministic/正文启发式 fallback。
- durable `direct` 复用同一 ADK text path，但 ToolProvider 不调用、Plan/Subagent 关闭，middleware 不构造
  offload/plan backend，也跳过 Skill/Filesystem/PlanTask/ToolSearch；模型返回任意 tool call 在工具执行前
  fail closed。single-step 保留受控 tools 但 Plan/Subagent off，multi-step Plan on/Subagent off；本轮不
  宣称 single 动作上限或 multi Plan-before-tool 新证据。typed Resume 继承 durable candidate，不重新
  调用模型，并应用同一 consumer。
- clarification 在 Execute/Resume bootstrap 后、任何 runtime store/factory/event 前返回
  `ErrAdaptiveDecisionConsumerUnavailable`；P2 Human Interaction consumer 尚未交付。
- public query 用窄 by-execution-run reader strict 读取 durable pair；Get/List/Search 共用 optional
  `CanonicalRun.coze.adaptive_execution` v1 五字段投影。前端 strict adapter 与 TaskDetail 最新 primary
  top-level Run 四态标签已接入，历史缺失省略，partial/corrupt fail closed，`enabled=false` 隐藏。
- 本计划仍不包含 holdout、progress/verification、clarification Human consumer、IDL 或 migration，
  因此不代表 P1D PASS。

### Task 5: Real model producer exit gate（部分交付，P1D 仍未 PASS）

**Files:** 实施前必须在 tracked 权威与当前 model execution seam 上重新定位并单独展开，不得复用本首包的 deterministic producer 伪装完成。

- [x] **Step 1: 冻结独立 typed model producer contract。** `Run.Input` closed projection 只输出 user/assistant content 与 attachment bool（64 KiB/32 条）；显式正十进制 model ID、唯一 forced tool、strict closed schema/EOF、独立 30 秒 timeout、共享 billing/usage、稳定 operation fingerprint、一次 provider Generate attempt及 closed error code 已冻结；无正文启发式 fallback。
- [x] **Step 2: TDD 实现真实模型 producer。** production gate-on 已替换为 `ModelAdaptiveDecisionProducer`；timeout、provider/model error、invalid structured output、usage error 均 fail closed 并记录 closed failed result，单次调用不重试。focused 与 application package 验证已 GREEN。
- [x] **Step 3: 证明幂等、计费与恢复。** narrow durable operation 以 internal/unsequenced claim/result events 选 single winner，raw operation key/claim token 只保存 digest；completed replay 跳过 provider/billing，claim-only unknown state fail closed。typed Resume 复制 durable candidate，绝不调用模型；不宣称 provider exactly-once。
- [ ] **Step 4: 完成独立 exit gate。** 运行相关 Go/MySQL/Graphify/Workbench 验证并做一次 combined review；只有该 gate 以及 P1D 其余既定验收全部通过，才可把 `P1D NOT PASS` 改为 PASS。

  Evidence：Go focused/package tests 已由实现与验证任务通过；MySQL
  `TestAdaptiveDecisionModelOperationMySQLIntegrationClaimSingleWinnerAndReplay` 已存在，但当前没有满足
  安全命名约束的 disposable DSN，状态为 `NOT_VERIFIED`。80 holdout、progress/verification、
  clarification Human consumer 与完整 P1D verification 仍是剩余 exit gate；P1M 状态不因本 Task 改变。

### Task 6: Durable decision consumer and safe public projection（已交付，P1D 仍未 PASS）

- [x] **Step 1: direct runtime consumer。** durable direct 禁止 ToolProvider、dynamic/subagent tools、Plan、
  offload/plan backend 与 Skill/Filesystem/PlanTask/ToolSearch；模型违规 tool call 在工具执行前 fail
  closed。single-step 保留受控 tools 但 Plan/Subagent off，multi-step Plan on/Subagent off。
- [x] **Step 2: clarification fail-closed boundary。** Execute/Resume 在 durable bootstrap 后、任何
  runtime store/factory/event 前返回 `ErrAdaptiveDecisionConsumerUnavailable`；typed Resume 沿用同一
  durable decision。P2 Human Interaction consumer 明确未交付。
- [x] **Step 3: safe public v1。** 新窄 by-execution-run repository reader strict 读取 durable pair；
  canonical Get/List/Search 只在 public query 显式 hydration，并经 `ProjectPublicRun` 发布 optional
  `coze.adaptive_execution_public.v1` 五字段。历史缺失省略，partial/corrupt fail closed。
- [x] **Step 4: strict frontend and TaskDetail。** adapter 校验 schema、四种 mode 和字段类型；TaskDetail
  只展示最新 primary top-level Run 的四态标签，`enabled=false` 隐藏，不新增 route/page。
- [ ] **Step 5: remaining exit gates。** disposable MySQL、clarification Human consumer、multi-step
  Plan-before-tool、holdout、progress/verification 与完整 P1D 门禁未完成，继续 `P1D NOT PASS`。
