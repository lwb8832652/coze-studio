# Workbench Adaptive Execution MVP P1D Decision Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用一个 server-owned、默认关闭且可稳定按空间放量的首包，让 fresh 顶层 Eino Run 原子持久化 gate-on admission 与保守 deterministic multi-step decision，同时冻结现有 runtime factory 对 direct/single-step/multi-step typed decision 的消费合同。

**Architecture:** fresh bootstrap 保持 durable read-first；仅在 exact miss 后由 eligibility resolver 在本次 `Resolve` 中严格读取 `AGENT_THREAD_ADAPTIVE_EXECUTION_ENABLED` 与 `AGENT_THREAD_ADAPTIVE_EXECUTION_ROLLOUT_BASIS_POINTS`，再依据固定 feature key `workbench_adaptive_execution_mvp` 和稳定 space bucket 冻结 gate。Coordinator 选择独立 producer、覆盖 candidate 的全部 durable identity、执行现有 strict pair validation，之后才分配三个 ID 并调用既有原子 bootstrap；typed Resume 只继承 source gate/capability/limits，legacy decoder 永远 gate-off。首包安装同文件内的 deterministic gate-on producer，它只产生保守 server-owned multi-step candidate，不扫描任务正文、legacy config 或 Journal；这不是模型分类器，也不引入第二次模型调用。

**Tech Stack:** Go、Eino ADK、GORM/MySQL、现有 `adaptivecontract` canonical codec、`go test`、Workbench execution graph verifier。

**Status:** `in_progress`；deterministic first packet 已实现、验证并完成一次综合审核，但真实模型 producer exit gate 尚未交付，因此 `P1D NOT PASS`。

---

## Scope and file map

**In scope**

- `backend/application/agentthread/adaptive_eligibility.go`：每次 `Resolve` 严格解析 server-owned 双 env，计算稳定 space eligibility；默认 disabled/`0`，enabled + `10000` 全量开启。
- `backend/application/agentthread/adaptive_decision_producer.go`：统一 producer interface、baseline adapter 与首包 deterministic gate-on producer；deterministic 候选固定为保守 `execute/multi_step`，不读取正文或旧控制字段。
- `backend/application/agentthread/adaptive_bootstrap_coordinator.go`：fresh read-first 后 resolver → producer → strict validate → IDs → atomic commit；Resume 继承 typed gate，不重算。
- `backend/application/agentthread/adk_agent_factory.go`：把已有 decision 合同收口成 direct/single-step/multi-step runtime capability；不另建 executor。
- `backend/application/application.go`：构造无状态 env resolver 和两个 producer 并注入 coordinator；constructor 不解析 env，错误在对应 fresh `Resolve` 时 fail closed。
- 对应 `*_test.go`、已有 disposable MySQL bootstrap integration test、两份 Workbench authority context 与 execution graph JSON。

**Out of scope**

- progress、repair/replan、verification、UI、IDL、migration、公共 DTO、模型调用、正文 classifier、acceptance-check registry、Journal enrollment 改造、Subagent 开启。
- 不允许从 `Run.Input`、`Run.Config`、`Run.Context`、Message/Journal 内容推断 gate 或 decision；不允许把 env 或 producer candidate 作为 durable identity 权威。

**本轮交付状态**

- 本轮只交付 server-owned eligibility、独立 producer seam 和保守 deterministic gate-on 运行闭环；它不是智能分类器，也不是 task-aware classifier，不能据此宣称 `P1D PASS`。
- 禁止以关键字、正则、prompt/body/message 扫描或 legacy config/Journals 启发式替代真实 producer。
- P1D 的后续独立 task/exit gate 必须安装真实模型 producer，并显式覆盖独立 timeout、billing/usage、结构化输出 codec、有限重试、稳定幂等与故障零提交；该 exit gate 通过前，P1D 状态保持 `in_progress`。

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
	Admission entity.AdaptiveAdmissionSnapshot
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

`AdaptiveDecisionCandidate` 有意不含 decision ID、revision、Run/Journal/Attempt/generation、Plan scope 或 created-at。Coordinator 必须覆盖这些 server-owned identity 字段；producer 不能获得或伪造它们。

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
- deterministic 首包只产生保守 multi-step；Factory 已冻结 direct 不开 Plan、single-step 不开 Plan、multi-step 使用已有 Plan 的未来 consumer 合同。首包不声称语义分类质量，不包含 progress/verification/UI/IDL/migration/模型调用。
- 本计划完成只代表 deterministic first packet 交付，不代表 P1D PASS；后续真实模型 producer 的 timeout、billing/usage、结构化输出、有限重试和稳定幂等 exit gate 尚未完成。

### Task 5: Real model producer exit gate（后续，未实现）

**Files:** 实施前必须在 tracked 权威与当前 model execution seam 上重新定位并单独展开，不得复用本首包的 deterministic producer 伪装完成。

- [ ] **Step 1: 冻结独立 typed model producer contract。** 明确结构化输出 schema、单次调用 timeout、billing/usage 归属、稳定 operation/decision idempotency key、有限重试预算和错误 taxonomy；禁止正文扫描启发式作为 fallback。
- [ ] **Step 2: TDD 实现真实模型 producer。** RED 必须证明 timeout、provider error、invalid structured output、重试耗尽和 lost response 全部在 bootstrap ID/commit/runtime 前零副作用；GREEN 才允许替换 production gate-on deterministic producer。
- [ ] **Step 3: 证明幂等、计费与恢复。** 同一 logical bootstrap 不重复计费或重复提交 decision；typed Resume 继承已持久化 decision，不再次调用模型；usage 必须进入现有权威 collector，不自建旁路计费。
- [ ] **Step 4: 完成独立 exit gate。** 运行相关 Go/MySQL/Graphify/Workbench 验证并做一次 combined review；只有该 gate 以及 P1D 其余既定验收全部通过，才可把 `P1D NOT PASS` 改为 PASS。
