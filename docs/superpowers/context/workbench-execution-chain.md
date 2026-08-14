# Workbench 当前执行链与框架事实

更新时间：2026-08-14
状态：当前生产实现
机器合同：`docs/superpowers/context/workbench-execution-graph.json`

## 结论

Workbench 当前只有一套生产 Run 执行内核：Go `agentthread` 控制面配合 Eino ADK。
主链是：

```text
React UI
  -> page service wrapper
  -> canonicalThreadClient singleton / CanonicalThreadCoreClient
  -> canonical Thread Thrift contract
  -> Hertz canonical HTTP handler
  -> agentthread ApplicationService
  -> agentthread domain service
  -> GORM / MySQL transaction
  -> MySQL pending Run + fenced lease
  -> Go RunWorker / RunProcessor
  -> RuntimeSelector(runtime=eino_adk)
  -> ADKExecutor / Eino Runner / ChatModelAgent
  -> MapADKEvent / EventSink
  -> MySQL RunEvent
  -> Hertz canonical SSE
  -> @coze-arch/fetch-stream / TaskDetail projection
```

这不是两套新旧 Workbench。`ChatTask` 已退役；`legacy` 仅用于读取和恢复历史无
runtime 标记记录。`normalizeNewDeerFlowRunConfig` 是新 Run 策略节点：它拒绝
`legacy`，在持久化前规范化为 `runtime=eino_adk`，随后才由
`RuntimeSelector` 执行。

## 权威规则

- 本文负责解释当前实现；JSON 合同负责稳定节点、显式边、链路顺序、源码锚点
  和测试证据。
- `calls`、`delegates_to`、`persists_via` 等边必须有源码中的直接证据。
- `precedes` 只表示同一已验证流程中的时序，不等同于函数调用。
- Graphify 派生边只补充检索上下文，不能覆盖合同中的显式关系。
- `workbench_execution_v1` profile 由调用者默认强制启用，用完整结构摘要固定
  authority rule、scope、节点属性、边、链、9 类查询、排除语义及其源码证据；
  不得通过删改 profile/authority path、query/chain/exclusion、关键节点属性或
  禁词来自我关闭校验。
- 派生物分为两张同基底图：`query-graph.json` 把每个业务问题建成检索意图节点
  并连接所需事实，`graph.json` 只保留 AST、合同有向边和 `anchored_in` 源码桥。
  完整问题在检索图命中业务节点后，显式边、执行顺序和源码桥必须回到无 overlay
  的执行图独立核验，不能让 `retrieves` 捷径充当执行链证据。
- codebase-memory/CodeGraph 用于实时查调用方和影响范围；最终判断仍回到源码、
  IDL 与测试。
- 本上下文只保存 bounded metadata，不保存 prompt、completion、tool arguments、
  tool results、credentials、object URI、checkpoint bytes 或原始审计载荷。
- 每项排除事实必须引用当前源码、测试、manifest 或独立当前上下文；否定边界不能
  只由 JSON 合同自证。

## 入口链

### Workbench 立即创建

`WorkbenchPage.handleSend` 调用页面适配函数 `createTaskThread`，后者只委托唯一
`canonicalThreadClient.createThread`。canonical client 请求
`POST /api/workbench/threads`，经 `CreateCanonicalThread` handler 进入
`ApplicationService.CreateTaskThread`；应用层调用领域层
`CreateThreadRunMessage`，最终由 MySQL `CreateThreadBundle` 在一个原子聚合中
持久化 Thread、初始 Message、Pending Run 和初始 Event。

源码锚点：

- `frontend/apps/coze-studio/src/pages/workbench/index.tsx`：`handleSend`
- `frontend/apps/coze-studio/src/pages/workbench/service.ts`：`createTaskThread`
- `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client-singleton.ts`：
  唯一 `canonicalThreadClient`
- `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`：
  `CanonicalThreadCoreClient.createThread/createRun/subscribeRunEvents`
- `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`：canonical 生成类型
- `idl/workbench/thread.thrift`：`WorkbenchCanonicalThreadService`
- `backend/api/router/coze/api.go`：52 个 `/api/workbench/threads/**` method/path
  pair 加 2 个 `/api/workbench/journal/settings` method/path pair，合计 54 条
  always-on canonical 路由
- `backend/api/handler/coze/workbench_canonical_thread_service.go`：
  `CreateCanonicalThread`
- `backend/application/agentthread/service.go`：`CreateTaskThread`
- `backend/domain/agentthread/service/service_impl.go`：`CreateThreadRunMessage`
- `backend/domain/agentthread/repository/mysql.go`：`CreateThreadBundle`

### Workbench 带文件创建

文件模式使用同一个 `handleSend`，但时序固定为：

1. `canonicalThreadClient.createThread(defer_start=true)` 只建立 Thread；
2. `canonicalThreadClient.uploadFiles` 上传并取得受限文件元数据；
3. `canonicalThreadClient.createRun` 创建正式 Pending Run。

该顺序由
`frontend/apps/coze-studio/src/pages/workbench/__tests__/workbench.test.tsx` 的
`uploads selected files before starting a new canonical task run` 覆盖。图谱使用
`precedes` 表达三步顺序，不制造 `uploadTaskThreadFiles` 调用 Run API 的假边。

### TaskDetail Follow-up

`sendFollowUpMessage` 只提交当前轮消息，先通过 canonical client 上传文件，再调用
`createRun`。服务端通过 Thread 历史重建权威输入。

源码锚点：

- `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`：
  `sendFollowUpMessage`
- `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-follow-up.test.ts`：
  `uploads files before creating a thread run`

### 其它入口

- 本地 LangGraph-compatible HTTP API：`/api/threads/**` 23 条和
  stateless `/api/runs/**` 10 条路由均已退役，不再是任何生产入口。
  canonical Run SSE 仅在自有 protocol 实现中保留经审核的
  LangGraph SDK-compatible event name 与 payload shape，不保留 backing Thread
  入口，也不导入 LangGraph SDK/runtime。
- Scheduled Task：
  `AgentTaskExecutor.Execute` 根据 `KeepConversation` 和 `ConversationID` 分支；
  `StartNew` 调用 `CreateTaskThread`，`StartInThread` 复用专属会话并调用
  `CreateRun`。
- 飞书消息：
  `backend/application/imchannel/runtime.go` 的 `processEvent` 调用
  `AgentRunner.Execute`；`startRun` 在无有效 session 时调用 `CreateTaskThread`，
  已有 `session.ThreadID` 时调用 `CreateRun`。

这些入口只生产 Run，不拥有 Run 状态机，也不是独立执行器。
`query.integration_ingress` 独立验收 Scheduled 和飞书的
新建/复用 Thread 分支是否都进入共享 `ApplicationService` Run 主链。

P1M-B1 在 public `ApplicationService.CreateTaskThread` 与 `CreateRun` 安装与
canonical raw ingress 同义的七字段结构化 admission，并固定在 runtime 规范化、retry
来源查询和任何持久化之前执行；因此 canonical、Scheduled Task、飞书以及其它直接调用
public Application 用例的新提交不能绕过 ingress freeze。`CreateRun` 的 package-private
`server_owned_subagent` provenance 只供 ADK Subagent recorder 使用，并再次要求非零
`ParentRunID` 与精确 `subagent` Run kind；未知 provenance fail closed。该兼容缝仍允许
服务端子运行生成旧控制字段。Human interaction resume、Subagent retry、Journal recovery
与 lease recovery 不接收新的外部 Config/Context，而是原样继承已持久化来源；P1M-B1
不改写历史 Run，也不代表 mode consumer 或完整 P1M 已退休；后续 C3h2d 已另行完成 production
ADK 的 `requested_policy`/`mode` consumer 退休。

P1M-C1 在此 admission 边界之后定义内部、mode-free 的纯 Go
`AdaptiveAdmissionSnapshot`、`ExecutionDecision` 和 validators。它的
`BaselineDecisionProducer` 无 I/O 且 deterministic；只在 gate-off 生成固定
`execute/multi_step`、安全的服务端摘要和空 deliverables/checks，gate-on 明确拒绝。P1M-C2a 已在
纯 domain `backend/domain/agentthread/adaptivecontract` 实现 strict canonical admission/decision
codec 和 domain validation；application 保持 C1 validator wrapper 与 sentinel alias，兼容既有错误
合同。P1M-C2b 已在 private `AdaptiveExecutionRepository` 提交并只读回放 bootstrap 的
`adaptive.admission`/`adaptive.decision` exact tuple 及 `workbench_control` checkpoint；generic
writer/reader 隔离这些保留事实。P1M-C3a 只增加一条窄生产边：已 enrolled、fresh、顶层 Eino ADK
`Execute` 在输入解析后、`buildRuntime` 前调用 gate-off coordinator；它先读 exact replay，首次才
提交固定 baseline decision。明确 `task` 和既有空 `RunKind` 顶层兼容形式在有效 Execute 身份及
启动依赖已满足时，未 enrolled no-op；logical Journal root 可与 execution Run 不同。
P1M-C3b 在同一链上把回读或提交返回的 server-owned facts 传给 Agent Factory，并只以其
admission/decision 覆盖本地 Plan capability；现有 Todo prompt 与 `ADKMiddlewareAssembler.Build`
中的 Plan backend 因而一致，不改变 middleware order。无 facts 的未接入路径保留历史兼容，
不匹配或 blocked facts 在创建 Agent 前 fail closed。P1M-C3c 再以同一 admission 的
`SubagentsAllowed=false` 同步关闭 local runtime config 的 Subagent prompt/limit middleware，并让标准
Subagent tool provider 在解析 definition、构建 child 前只返回 base tools；一次性私有 disable 信号在
调用 base provider 前已清掉，不会传播到 child。P1M-C3d 再把同一 fresh Execute 的旧
`mode`/`thinking_enabled`/`reasoning_effort` 影响中和为 local thinking false 与空 reasoning effort，
primary/failover model option 和 provider-capability middleware 因而使用同一安全中性请求；未定义新
推理 policy。P1M-C3e 按交付优先只退休 Journal enrollment/completed metrics 的旧 mode consumer：
enrollment 仅接受明确 `runtime=eino_adk` 的 fresh 顶层 task，并保留既有 rollout/kill-switch；completed
完整性指标只以真实 `Enrolled && Completed` 为分母，不再读取 `Mode`。本切片未新增 metrics emitter，
也未定义 server inference policy。P1M-C3f 只收口 public new-write config：`CreateTaskThread`、
`CreateRun` 与 top-level retry 仍先执行 server policy normalization，再在持久化前删除规范化结果顶层的
七个退休执行控制字段；`runtime=eino_adk` 与合法模型、资源、Token Usage、opaque 配置继续保留，
P1M-C3g 又让 public `CreateRun` 拒绝 caller-owned child shape，只有 package-private trusted child seam
可以持久化 exact `ParentRunID > 0 && RunKind=subagent` child；这些 child 的 Config 新写也删除七个退休
字段。异步 child 或 source-child retry 进入同一个 Agent Factory 时，Factory 根据 exact durable child
identity 在本地关闭 Plan、Subagent、thinking 与 reasoning，并在 adaptive facts 之后再次覆盖；因此
旧历史 child Config 仍兼容且不会重新开启这些能力。后续 C3h2d 已让 builtin/single-agent 内存 child
writer 停止写入 `requested_policy`/`mode`，并让 production ADK consumer 忽略这两个顶层旧键；C3g
本身仍不等于该退休完成。
继续按交付优先完成的 P1M-C3h1a 不新增执行边：Human interaction resume、ordinary
non-Journal lease recovery 和 Journal recovery 仍使用各自现有的 Run bundle/create 链，
但目标 Run Config 新写前仅删除来源 Config 顶层的七个退休字段。`runtime`、模型、
资源、Token Usage、opaque 配置和 nested 同名字段保留；Context 与来源历史 Config
均不改写。该 C3h1a 切片本身不新增 Attempt enrollment，不把恢复目标接入
`ADKExecutor.Resume`，不实现 legacy decoder、typed inheritance、IDL/UI；这些历史范围说明已由
后续 C3h2a/C3h2b 的 enrolled Resume typed inheritance 与原子 Attempt rollover 取代。
P1M-C3h2a 只连接 already-enrolled Journal recovery Resume：immediate source 必须具有有效
fresh/typed durable bootstrap，target 在 ADK `buildRuntime` 前提交或 exact replay gate-off
`typed_inheritance` snapshot。该 C3h2a 切片当时不包含 Human rollover；后续 C3h2b 已补齐 enrolled
Human Resume。后续 C3h2c 在同一 production edge 上增加 typed-first、legacy-exact-miss fallback：
target durable replay 仍最优先；只有 source durable bootstrap 精确 NotFound 才读取 source Run 并用
隔离 decoder 严格解析已知 root legacy control，冲突、损坏或其它 repository 错误均 fail closed。
decoder 只生成保守 gate-off admission；baseline decision 由独立 producer 生成，两者连同 source
Run/generation/config digest/decoder version 在 target lease/generation fence 下一次提交或 exact replay。
多跳只继承 durable typed snapshot，不再次解码或回写来源 Config。后续 ordinary MVP 已把相同
bootstrap 边界扩到 disabled/无 Attempt 的 bare recovery；该阶段 gate-on producer 仍 deferred，P1M 未 PASS；
P1L 与 whole-Thread DELETE hard guard 不变。
P1M-C3h2b 把 enrolled Human Resume 接入 full replay 与原子 Attempt rollover。Application 在任何
source status/Attempt/checkpoint 可变读取前查询完整 Run/Message/resolved/source+target
Attempt/terminal aggregate；exact aggregate 直接回放，漂移或半写 fail closed。首次写复用
`CreateRunBundle` 的 Human 专用分支，repository 在同一 Thread-first 事务中追加 source resolved 与
physical terminal、CAS source Attempt 为 `interrupted` 并释放 active slot，最后创建带 source
Attempt/checkpoint lineage 的 pending target Attempt；随后继续走 C3h2a typed
bootstrap-before-buildRuntime。physical helper 不改变公共 RunEvent total/cursor 或 TaskDetail
replay/live。compatible-reader floor 是 `38ddbaf6f`，activation 是 `212546bc`；后续 ordinary MVP
已补齐 disabled/无 Attempt rollover；该阶段 gate-on producer 仍 deferred。dev disposable MySQL 已对 Human rollover
通过 same-key replay、same-key drift conflict、different-key single-winner 三项真实双连接门禁，并对
typed recovery race 与 legacy decoder recovery 通过单写/replay、漂移零增量和 durable readback 门禁。
MySQL JSON 存储归一化由 typed codec canonical readback 后再核对既有 digest/fingerprint；非 MySQL
严格 canonical 规则不变。P1M-C3h2d 已让 production ADK 的 Factory、Middleware、标准 Subagent
provider 与 builtin definition 统一通过 `parseADKRuntimeConfig` 忽略顶层
`requested_policy`/`mode`，builtin/single-agent child writer 也不再写入这两个键；显式 server-owned
Plan/Subagent/reasoning/model 配置仍保留。`b8d1c21b0` 还让 repository 支持同一 recovery Attempt 的
B1→B2→B3 rolling Plan、历史 exact replay 以及 rolling checkpoint 作为下一 Attempt 恢复来源，全部
继续核验 lineage、revision/version、checkpoint/event fingerprint 与 anchor。后续 production writer
已将 `ApplicationADKPlanStore` 的工具写切为 run-scoped overlay；`AfterToolCalls` 内部 cancel 触发真实
Eino v3 checkpoint，`ADKCheckpointStore` 再经 `CommitAdaptiveExecutionBoundary` 一次提交 Plan
high-watermark、PlanItem、追加 Event、checkpoint 与 Attempt cursor。首次 Plan 0→1 初始化，后续
B1/B2/B3 rolling 与当前 head read-first crash replay 共用同一 authority；历史 exact replay 不推进
当前状态。提交成功后 `ADKExecutor` 自动 Resume，外部 cancel 仍沿原取消语义；Plan 与 side-effect
同一 checkpoint boundary 在任何 durable write 前 fail closed。enrolled typed Resume 从 durable
decision 继承 source `PlanScopeRunID`，target checkpoint store/coordinator 使用该 scope，恢复后的
Plan 写仍进入同一 atomic boundary，不回落 legacy writer。repository/application Go 测试已通过；
本轮 dev disposable MySQL rolling gate 因缺少满足安全命名约束的隔离 DSN 保持 `NOT_VERIFIED`。
ordinary MVP 现在把 fresh 顶层 Eino Task 的 Attempt enrollment 固定为 always-on：projection gate
命中时为 healthy，gate off、依赖 nil 或判定错误时为 disabled 且 snapshots off；非 Eino/child 不变。
`recoverExpiredRun` 对 healthy/non-disabled Attempt 继续委托 `RecoverJournal`，对 disabled Attempt 或
无 Attempt source 读取可恢复 checkpoint 后委托 `CreateRunBundle` 的 dedicated ordinary 分支。该分支
在同一 Thread-first transaction 锁定 root/source/Attempt/checkpoint，CAS source lease 为
`interrupted`、只追加 base terminal event，再创建 disabled target Attempt；已有 disabled source
Attempt 同时终结并释放 active slot，bare source 创建 ordinal 1 target。Application 把完整 checkpoint
authority 随请求传入，repository 在锁内逐字段及 JSON 语义复核，防止 checkpoint pre-read 后发生
TOCTOU 漂移。ordinary target 随后进入同一个 `ADKExecutor.Resume -> BootstrapResume` 生产边：bare
lineage 先 exact replay target，miss 后不读 source durable bootstrap，只用 strict legacy decoder 读
source Run，并继承 source `PlanScopeRunID`；同一 source scope 的 bare Plan boundary 支持首次 commit、
read-first replay 与 rolling。ordinary 的真实 dev MySQL gate 本轮未运行，保持 `NOT_VERIFIED`。

P1D 首个 deterministic packet 沿用同一 `ADKExecutor.Execute/Resume -> adaptive bootstrap ->
buildRuntime` 生产边，并在 application composition root 安装独立的 env eligibility、gate-off baseline
adapter 与 gate-on deterministic producer。fresh Run 仍先 exact read target；只有 miss 才读取
`AGENT_THREAD_ADAPTIVE_EXECUTION_ENABLED` 与
`AGENT_THREAD_ADAPTIVE_EXECUTION_ROLLOUT_BASIS_POINTS`，二者默认 `false`/`0`，显式非法值 fail closed。
开启后以 `SpaceID` 与冻结 key `workbench_adaptive_execution_mvp` 做稳定 basis-point 分桶，与 Journal
projection enrollment gate 解耦。选中的 producer 只接收 admission，并只返回候选内容/形态；它不能
设置 schema、decision ID/revision、Run/Journal/Attempt identity、execution generation、plan scope 或
created-at。coordinator 补齐上述服务端 authority，再以 strict pair validator 校验，随后才沿现有
`CommitAdaptiveExecutionBootstrap` 一次提交 admission、decision 与 control checkpoint；producer 缺失、
报错或候选非法均在 ID 分配、事务和 Agent 构建前 fail closed。

target exact replay 不调用 resolver/producer。typed Resume 从 durable immediate source 继承 gate、
capabilities、limits 与 plan scope，选择相应 producer 生成 target revision-1 decision，不重新计算 rollout；
legacy decoder 继续只生成 gate-off snapshot。repository 在同一事务中锁定并核对 source gate/policy，
禁止 gate-on→off 漂移。Factory 已能将合法 gate-on `direct`/`single_step` 映射为无 Plan、将
`multi_step` 映射为有 Plan；但当前 production deterministic producer 不扫描任务正文、不调用模型，只
固定生成保守 `execute/multi_step`，因此没有交付真实任务自适应分类或 direct 快速路径。真实 dev MySQL
gate-on fresh replay/typed Resume 测试已存在，但因缺少安全 disposable DSN 本轮仍 `NOT_VERIFIED`。
model-backed producer、holdout、公共 DTO/TaskDetail 与 progress/verification 均未完成，P1D 保持进行中
且不得标记 PASS；historical runtime compatibility 与 package-private server-owned subagent seam 继续
存在。P1L 仍
deferred，whole-Thread DELETE guard 继续 hard-disabled。

## 持久化与异步执行

### 原子 Run 创建

`CreateCanonicalRun` handler 调用 `ApplicationService.CreateRun`，应用层完成权限、
提交执行控制 admission、输入、幂等和 runtime 规范化，再调用领域层 `CreateRunBundle`。领域层经
repository `CreateRunBundle` 原子写入 Run、当前轮 Message、初始 Event 和相关
admission 状态。

没有 Message/bundle 的非顶层 Run 走另一条受保护分支：
`ApplicationService.CreateRun` -> `threadService.CreateRun` ->
`CreateRunWithThreadLock`。repository 在插入 Run 前锁定所属 Thread；lease 恢复
建立 resume Run 时也复用该分支。该锁与 `DeleteThreadIfIdle` 使用同一 Thread
行，因此删除与直接 Run 创建只能形成两个可线性化结果：删除成功且 Run 不存在，
或 Run 成功且忙碌 Thread 拒绝删除，不会留下孤立 Run。

上述 repository 能力当前对 whole-Thread canonical DELETE 是 dormant：P1M-A 的 HTTP
编译期 guard 在 workspace 授权与 path 校验后统一返回 503，不调用 application/domain/repository
删除链。它没有改变底层事务语义，也不能作为 idle cascade 已完成锁序闭合的证据。

### P0A/P0B/P0C/P0D 自适应执行事务边界

`P0A/P0B/P0C/P0D 的事务基础仍是 repository primitive 与 named-path evidence；本节随后明确列出
已经接入 production 的 Plan boundary`。P0A
implementation HEAD 是
`a1df1789b0b60c0916505711bcc1e5a1fa1410cd`。P0B 已在同一个 private repository
interface 中实现 `CommitAdaptiveExecutionBoundary` 的 exact-tuple 幂等回放，以及
`ReadAdaptiveExecutionRecoverySource` 的 recovery source 只读恢复；checkpoint 的
server-owned metadata 已升级为 `workbench-adaptive-boundary.v4`，除 event 与 checkpoint
fingerprint 及 bounded PlanItem refs 外，还固定 Plan high-watermark 与逻辑 mutation digest。
Checkpoint fingerprint 覆盖完整物理 checkpoint、canonical user metadata，以及除 event/checkpoint
两个循环摘要外的全部 typed
adaptive authority；它同时写入追加式 boundary Event 的 `snapshot_id` 作为跨行锚点。
Event fingerprint 覆盖完整 event 物理行，回放、恢复和 lineage 都会重算并核对两级摘要，
因此不能通过改写同一 checkpoint 的 refs、source 或 Plan authority 后重算内层摘要来绕过。
写事务统一按 logical Journal root、Execution Run、Attempt、exact event tuple、source lineage、
Plan、PlanItem 的顺序加锁，避免 Attempt 创建与 recovery boundary 形成反向 Run 锁环。两条
读取路径都直接读取私有 PO 中的持久化 event、checkpoint、Attempt lineage、Plan revision/high-watermark
和 PlanItem refs 来重建 authority，不经 public `ListRunEvents`、`ListCheckpoints` 或 latest
checkpoint selector。

Production Plan boundary 复用这一个 repository transaction，而不是另建 writer。Plan middleware
对 eligible `execute/multi_step` 只写 run-scoped overlay，并同步 parity todos；Eino 在每次 Plan 工具
调用后的 `AfterToolCalls` 安全点以内部 `CancelAfterToolCalls` 保存真实 v3 runtime checkpoint。
`ADKCheckpointStore.Set` 先按稳定 tool-call/address identity、runtime key 与 mutation digest 做当前
Attempt head read-first；精确命中直接采用 durable result，只有 typed NotFound 才分配 ID 并提交。
提交同时推进 Plan revision/high-watermark、PlanItem version、Event sequence、checkpoint parent 和
Attempt cursor；首次 Plan 是 0→1，后续 B1/B2/B3 rolling 复用上一 checkpoint。成功后执行器在同一
Runner 上自动 Resume，最多 64 个内部 boundary；未提交 boundary、repository 冲突或 mixed
Plan/side-effect 均 fail closed。enrolled typed Resume 还把 durable source `PlanScopeRunID` 投影到
target store/coordinator，使 recovery Plan mutation 沿用 source Plan identity 与同一 transaction。
repository/application 测试已通过；独立 dev MySQL rolling gate 当前 `NOT_VERIFIED`，不得从既有
Human/typed/legacy recovery MySQL PASS 推导它已通过。

P0C 没有新增第二个 finalizer，只在唯一现有 repository `FinalizeRunSuccess` request 上增加
optional `AdaptiveGate`。nil gate 继续执行原有成功终态路径，返回的
`VerificationEvent=nil`、`Replayed=false`，也不会新增 Decision、Evidence、Plan 或 PlanItem
查询；原有 non-nil JournalEvent 的 Attempt projection 语义保持不变。P0D 将 gate-on 的锁前缀
固定为从 logical Journal root 发现 durable Thread 后按 Thread → logical Journal root → distinct
Execution Run → Attempt identity 加锁；Journal root 与 Execution Run 相同时复用同一已锁 Run，
随后才读取 exact Verification tuple、Decision/Evidence authority、Plan 与全部 current PlanItems。
passed Verification 与 Completion 在同一个 `FinalizeRunSuccess` 事务和同一个 Attempt 的连续
sequence 中依次写入（Verification < Completion），任一 authority drift、写失败或末尾 outbox
callback 失败都会回滚整个终态事务。已提交的 exact retry 在锁内 current-read 并重校原结果；
non-nil outbox 只做 immutable identity compare 且必须返回 `inserted=false`，不会重写终态或补写
缺失行。

P0D 只证明三条 named repository path 的锁序与线性化结果：gate-on 是 Thread → logical Journal
root → distinct Execution Run → Attempt；gate-off 是从 durable Run identity 发现并锁定 Thread，
再锁 Execution Run，只有既有 non-nil JournalEvent 投影需要时才进入 Attempt，原有 durable write、
返回值和 caller 均不变；active lease recovery 在锁定 source Attempt 前先锁 source Execution Run。
真实 MySQL 门禁已覆盖 cancel 与 verified success、lease recovery admission 与 finalizer/cancel、
`DeleteThreadIfIdle` 的 active-run 拒绝分支与 gate-on/gate-off finalizer，以及 crash-after-commit
exact replay，并验证每组竞态只有一个合法 durable outcome。该证据不扩张为 repository-wide
deadlock-free 结论。

原先 recovery 后同一 Attempt 的第二个 Plan-bearing boundary blocker 已由 rolling high-watermark、
checkpoint parent 与 Application atomic writer 闭合。仍存在的另一跨 packet 问题是：`DeleteThread`
和 `DeleteThreadIfIdle` 真正进入 idle cascade 时，与 historical
Journal、generic boundary 和 exact replay 的完整锁序尚未闭合；P0D 的 active-run 删除竞态不能
证明该分支。P1L 已延期，P1M-A 仅以 canonical HTTP guard 隔离 whole-Thread 删除，因此底层
idle cascade 仍未解锁；该 blocker 至少阻塞删除重新启用，而 P1M-A 也不等于完整 P1M PASS。
P1M-B/C 可在 guard 保持时继续，P1D 仍需完整 P1M。P0A3 只把直接
Plan mutation 与 primitive 的锁顺序统一为先锁 `AgentRunPlan`、再锁 `PlanItem`，现有直接 Plan
mutation 仍然可达。

`FinalizeRunSuccess` 的 `AdaptiveGate` 仍是 implemented-but-unwired repository gate，production caller
继续使用 nil gate；recovery source primitive 也没有 production caller。P1M-C1/C2a 的纯 Go
snapshot/decision、strict canonical codec/domain validation 与 application compatibility wrapper，以及
C2b private durable bootstrap commit/readback 和 generic reserved-fact isolation 已实现。C3a 只让
`ADKExecutor.Execute` 在构建 runtime 前调用 gate-off bootstrap coordinator；明确 `task` 和既有空
`RunKind` 顶层兼容形式在有效 Execute 身份及启动依赖已满足时，未 enrolled no-op，失败不会创建
checkpoint store 或 Agent。C3b 只让该 durable facts 控制同一 fresh Execute 的 Todo prompt 与 Plan
backend；C3c 只让同一 admission 的 `SubagentsAllowed=false` 关闭该 Execute 的 Subagent 工具、prompt
与 limit middleware；C3d 只中和该 Execute 的旧 reasoning controls，不建立新推理 policy；C3e
只让 Journal enrollment 以明确 Eino ADK fresh 顶层 task 为准，并让 completed 完整性指标以真实
enrollment/completion 为分母，不新增 metrics emitter；C3f 只停止 public 新 Run config 写回七个退休
字段并保留 runtime/合法业务配置；C3g 进一步拒绝 public child shape，只允许 package-private trusted seam
持久化无七字段的 durable child，并由 Factory 对 exact child identity 本地强制关闭 Plan、Subagent 与
reasoning；C3h1a 只让 Human resume、ordinary non-Journal lease recovery 和 Journal recovery
的目标 Config 新写删除顶层七字段，保留其余 Config、nested 字段与 Context，来源历史
Config 不改。C3h2b/C3h2c 已让 enrolled Human/Journal Resume 使用原子 Attempt rollover、typed
inheritance 或 source durable exact-miss 时的严格 legacy decoder fallback；dev disposable MySQL 已完成
Human rollover、typed recovery race 与 legacy recovery 门禁。builtin/single-agent 内存 child writer 与
production ADK mode/policy consumer 已由 C3h2d 退休；Application rolling Plan/Checkpoint writer
现已接入上述 atomic boundary。ordinary enrollment/recovery 随后已闭合，P1D 首个 deterministic packet
又接入 gate-on eligibility/producer 与 typed Resume inheritance；但其固定 multi-step，不是 task-aware
producer，frontend/UI 也未改变。P2 仍负责完整 VerificationResult codec、registry、producer、
nullable-Plan authority 分支和其余接线；P1D 仍在进行中且未 PASS。

### Canonical Thread HTTP 契约

`/api/workbench/threads...` 的 Thread create/search/get/patch/delete、state、history
和 messages handler 以及产品扩展合计 52 个 `/api/workbench/threads/**`
method/path pair，再加 2 个 `/api/workbench/journal/settings` method/path pair，
共 54 条 always-on canonical HTTP 路由。不存在 canonical
API 环境开关或运行时路由开关。入口只接受 session
principal；所有 canonical 请求都必须提交 `X-Coze-Space-ID`，服务端先用认证主体校验
workspace。create/search 直接在声明空间内执行；其余资源路由还会从 path Thread/Run
读取服务端归属，把声明空间、资源实际空间和 Thread 授权一起校验。任何路由都拒绝
依赖客户端 body 中的 owner、`user_id` 或 `space_id`。

whole-Thread DELETE route 与 IDL 保持注册，但执行顺序固定为 dependency 检查、workspace
授权、path ID 校验、`503 thread_delete_temporarily_disabled`。合法正整数 ID 无论 Thread
存在与否都返回同一错误；handler 不读取 Thread、不调用 `DeleteThreadIfIdle`，也不进入删除
application/domain/repository。该临时 guard 不影响 Artifact、Upload、Memory 等子资源 DELETE。

P1M-A 还在 raw JSON binder 与任何业务持久化前冻结七个客户端执行控制字段：
`requested_policy`、`mode`、`thinking_enabled`、`reasoning_effort`、`is_plan_mode`、
`subagent_enabled`、`max_concurrent_subagents`。覆盖 Thread create 的
`initial_run/deferred_initial_run`、Run Create/Wait/Stream、专用 Resume 与 Subagent Retry；
只沿审核后的 root、`config`、`context`、`configurable/context` 对象路径检查，不扫描字符串、
数组或普通资源对象。命中返回稳定 `422 unsupported_execution_control`，但
`runtime=eino_adk`、model、Skill、MCP、knowledge/database、附件和可靠性配置仍合法。

P1M-C3i1 在这条入口链增加封闭 Typed Submission V2 接受层。IDL 和生成的 Go/TypeScript
合同为 Create Thread 的 `initial_submission_v2`/`deferred_initial_submission_v2`、
Create/Wait/Stream Run 的 `submission_v2` 以及 Resume 的 `response_v2` 提供 typed 字段；
handler 的有向顺序固定为 raw typed validation → deterministic mapping → 既有
application command/idempotency fingerprint。raw 阶段先处理 body budget、重复键、未知字段、
presence/`null`、union、V1/V2 混用和退休执行控制，映射阶段保持附件顺序、composer/resource
选择、retry source lineage、Human response union 与 fingerprint namespace。P1M-C3i2 又把五个
第一方 writer 接到该接受层：Workbench atomic/deferred 创建、deferred upload 后的首轮 turn、
TaskDetail follow-up、top-level retry 与 Human Resume 都经共享 typed serializer 和唯一 canonical
client 发送版本互斥的 V2 字段；deferred/follow-up 保持 upload-before-run，歧义 follow-up、固定 retry
key 与 Human semantic attempt 均不自动旋转或重复写。V1 server reader 与第三方兼容调用继续可用。
C3i2 自身没有增加 repository/runtime 状态机；后续 C3h2b 已补齐 enrolled Human Resume 的原子
rollover 与 full replay，C3h2c 又补齐 enrolled Resume 的 legacy exact-miss fallback；dev disposable
MySQL 已完成 Human rollover、typed recovery race 与 legacy recovery 门禁。ordinary non-Journal
enrollment 后续已闭合；production mode/policy consumer 已由 C3h2d 退休，随后 P1D 首个 deterministic
packet 已接入 gate-on producer seam，但真实任务分类/direct 与 UI 仍未闭合。
Application rolling Plan/Checkpoint writer 后续已接入，独立 dev MySQL rolling gate 保持
`NOT_VERIFIED`。P1D 未 PASS。

P1M-B1 把同一安全边界下沉到 public Application ingress：`CreateTaskThread` 与
`CreateRun` 在 normalization、retry source read 和 mutation 前拒绝同一七字段，typed error
继续由 canonical mapper 输出 `422 unsupported_execution_control`，只公开规范化字段路径。
该防御覆盖 canonical、Scheduled Task、飞书和未来直接 public Application caller；唯一
package-private ADK child provenance 与既有 resume/retry/recovery 继承链继续读取历史
Config/Context，作为 P1M-C 退休 consumer 前的显式兼容边界。

canonical handler 只做严格 SDK 参数、公开投影和稳定错误适配，随后调用同一个
`agentthread.ApplicationService`。create 继续进入既有 `CreateThread` 或
`CreateTaskThread` 事务链；query、patch 和 public-state 则进入 additive
应用/领域/repository use case，不建立第二套 runtime、storage 或双写。public-state 更新
在同一事务内锁定 Thread、选择顶层 Run/父 checkpoint、合并审核后的 `custom`，并写入
隔离的 `canonical_public_state` checkpoint；不会修改或公开 Eino checkpoint bytes、
runtime resume key 或内部事件。metadata 数值过滤接受 MySQL JSON 可表示的全部 finite
number，并让 SQLite 按 MySQL 的存储类型和规范值模拟相同查询语义。

canonical Run create/list/get/wait/join/cancel/resume/events/messages 已接入真实 handler，
并复用同一个 `agentthread.ApplicationService` 的原子 Run 创建、权限校验、取消、恢复和
游标查询用例。请求只接受审核后的 SDK 字段；Run、Event、Message、wait/join values 均经
公开投影，禁止回显原始 input、command、config、context、checkpoint bytes 或 provider
载荷。`command.resume` 与专用 resume route 进入同一 human-interaction 恢复用例，不原地
改写来源 Run。普通 turn 只接受公开 assistant 别名 `agent` 和一个 User Message；普通 turn
以及 Thread create 的 `initial_run/deferred_initial_run` 请求体上限均为 1 MiB，Message 上限为
256 KiB，config/context 的单个持久化字符串上限为 32 KiB，
上传文件最多引用 10 个正整数 ID。handler 不信任客户端文件描述，只在授权后的 path
Thread 中查询文件并重建权威摘要。`Idempotency-Key` 只从 header 接受：同 Thread 重试回放
首个已提交 Run/Message，即使上传文件后来被删除；服务端持久化 operation/payload
fingerprint，同键改 payload、跨 turn/resume 或跨 Thread 复用都返回稳定 `409`，并发唯一键
竞争也由 repository 二次读取执行相同校验。canonical handler 在持久化前把原始 key 与
session principal 组合成稳定摘要，因此同一 workspace 的不同用户互不碰撞，原始 key 不会
进入数据库或日志；operation 仍保存在隐藏 fingerprint metadata 中，保持同一用户跨操作
复用 key 时返回 `409`。Thread create 的 `initial_run` 复用普通 turn 的 assistant、Message、
config/context 和 metadata 安全校验，并参与相同的 operation/fingerprint 回放校验；历史内部
assistant selector 统一投影为公开别名 `agent`。不携带 Run 的空 Thread 和
`deferred_initial_run` 的通用 exactly-once registry 仍是后续能力，不在当前合同内。
Run/User Message 原子 bundle 同时写入不公开的
Message 关联，GET/list 不依赖客户端 metadata 推导 `message_id/attempt_kind/source_run_id`。
应用层、领域层和仓储层的 operation、fingerprint、Message 关联及重放校验开关默认均为空或
`false`，仅 canonical handler 显式启用；历史调用即使已有同名 metadata 也保持原语义。
wait/join 只在 Run 终态返回 `200 values`，不再复用旧事件流的 30 秒定时器；请求 deadline
和 cancel 分别映射为 `504/408`，只有 wait 的 `on_disconnect=cancel` 会触发现有授权取消
用例。终态 Run 或 cancel/complete 竞争统一返回幂等 `204`。resume 的客户端语义错误返回
`422`、状态竞争返回 `409`。Thread/Run message 全局序列投影保留现有精确语义，但以 5000 条
原始记录和 32 MiB 原始字符串为双预算，超限返回 `422 journal_too_large`，不做无界扫描。
生产网关仍必须在读取或缓冲 body 前执行同等或更严格的请求体限制，handler 限制是第二道
契约防线。

canonical create-stream 和 reconnect-stream 两个 SSE handler 为 always-on 合同。
create-stream 在读取请求体前完成 path Thread/space 授权，再严格解析
submission；随后复用现有原子 Run/Message 创建和 human resume 用例，按 principal 隔离
幂等键，并在 Run 与 User Message 的公共投影通过后返回精确 `Content-Location` 与指向
既有 Run GET stream 的 `Location`，写入 metadata、回放持久化事件并跟随 live 事件。
reconnect-stream 分别校验 query `after_event_id` 和 `Last-Event-ID`，同时存在时使用较大值，
支持受控 `stream_mode` 覆盖；`cancel_on_disconnect` 只接受固定 SDK 的精确 `1|0` 和显式
小写 `true|false`，大小写变体、空白包裹及其他值严格拒绝。`messages-tuple` 只作为
请求 mode，匹配 `message.*`、`llm.*` 及同类公开事件，wire event 固定为 `messages`，data
为二元数组。两条路径均按 `event_id` 顺序输出审核后的公共事件，终态前执行最后一次
event flush，且只在 SSE writer 明确确认断连且请求选择 cancel 时取消 Run；reconnect 的
显式 `true|1` 覆盖 Run 持久化的默认断线策略，`false|0` 不取消。context 结束、流超时和正常
终态都不会触发取消。执行图用独立 canonical Run SSE chain 记录创建、幂等回放、事件查询、
human resume 与断线取消的应用层依赖，不把 SSE handler 伪装成非流式 handler 的调用方。

canonical product resources 共用严格的十进制路径 ID、1 MiB JSON body ceiling、exact
offset pagination 与公开投影 helper。手写 wire projection 必须与 `thread_product.thrift` 的
required/optional presence 一致，所有必填实体 ID 均 fail closed；公开资源将 ID 和时间规范化为
字符串/RFC3339，optional 时间只省略零值，任何非零非法时间均 fail closed；Artifact 和 token usage
先经过 application public projection。非数字
`source_id`/`target_id` 仅在符合公开标识符与敏感值边界时保留；scan worker 只保留稳定哈希引用，
不公开租约或原始错误。完成日志只记录审核后的资源、分页和生命周期字段。普通 Run 创建响应可附带
同一原子 bundle 已提交的 User Message 投影；幂等 POST replay 仍是 create 响应，保留同一公开
`submission_message`，后续 list/get/read 投影不保留该一次性字段。Thread 的 `Source`、`Progress`
与最后消息字段仅兼容透传现有 `ThreadSummary`，不在此层补充数据来源；UI 继续保留 messages/title/status
fallback。

当前 Workbench、任务列表和任务详情 UI 均经唯一 canonical client 使用上述合同。
第一方 writer 已停止向 `config`、`metadata` 和 `message_metadata` 写入上述七字段；运行设置
暂时不渲染“模型推理”，但模型、资源、重试/failover 与 Token 用量设置保持。该 UI/ingress
冻结不改变内部 ADK Execute/Resume、human resume、lease recovery 或 subagent retry 对历史
config 的兼容读取。
`/api/workbench/task_threads/**` 36 条 V1 路由和 `/api/threads/**` 23 条本地
LangGraph Thread 路由以及 stateless `/api/runs/**` 10 条路由已全部不可达。
Scheduled Task 的 11 条路由继续由 `idl/workbench/task.thrift` 拥有。三组已退役
路由都不得作为 UI fallback 或新功能依赖恢复。

### MySQL 队列与 lease

Workbench 没有 Redis、Kafka、RabbitMQ、NATS 或 Asynq Run 队列。队列事实是
MySQL 中的 Pending/Queued Run 行：

- `RunWorker.Start` 使用 Go goroutine 与 `time.Ticker` 周期执行；
- `RunWorker.RunOnce` 调用 `RunProcessor.ProcessPendingRunsWithResult`；
- claim 从 application、domain 下沉到 repository `ClaimPendingRuns`；
- MySQL 使用 `SELECT ... FOR UPDATE SKIP LOCKED`；
- lease fence 由 `lease_owner`、`lease_token` 和 `execution_generation` 组成；
- 执行期间续租，lease 丢失或持久化取消时停止当前执行；
- `RunLeaseRecoveryProcessor` 处理超时 lease，有可恢复 checkpoint 时建立恢复
  路径，否则按规则对旧 Run 做终态协调。

源码锚点：

- `backend/application/agentthread/worker.go`：`RunWorker.Start/RunOnce`
- `backend/application/agentthread/runner.go`：
  `RunProcessor.ProcessPendingRunsWithResult/processRun`
- `backend/domain/agentthread/repository/mysql.go`：`ClaimPendingRuns`、
  `SKIP LOCKED`
- `backend/application/agentthread/run_lease_recovery.go`：
  `RecoverExpiredRunLeases`

## Eino ADK 执行内核

`RuntimeSelector.Execute` 读取持久化 Run config。当前生产标记
`runtime=eino_adk` 选择 `ADKExecutor.Execute`；显式请求 Eino 但策略未启用时
fail closed。

`ADKExecutor.Execute` 的关键步骤：

1. 通过 `ApplicationADKAgentFactory.Build` 解析模型、能力、工具和 middleware；
2. 通过 Eino `adk.NewChatModelAgent` 建立可恢复 Agent；
3. 通过 `adk.NewRunner` 执行并迭代 `AgentEvent`；
4. `MapADKEvent` 把 Eino 内部事件映射为 Coze 公共 RunEvent；
5. `applicationRunEventSink.EmitRunEvent` 调用
   `ApplicationService.AppendRunEvent` 持久化；
6. 最终结果由 `RunProcessor` 以 fence 条件完成 Run，并保存安全投影和 Assistant
   Message。

关键源码：

- `backend/application/agentthread/runtime_selector.go`
- `backend/application/agentthread/adk_executor.go`
- `backend/application/agentthread/adk_agent_factory.go`
- `backend/application/agentthread/adk_event_mapper.go`
- `backend/application/agentthread/event_sink.go`

### Middleware 顺序

`backend/application/agentthread/adk_middleware.go` 的 `adkMiddlewareOrder` 是唯一
顺序事实：

```text
side_effect
reduction
filesystem
uploaded_files
patchtoolcalls
tool_error_normalization
memory
skill
transcript
summarization
plantask
provider_capability
multimodalbudget
toolsearch
parity_state
contextbudget
safety_finish
subagent_limit
semantic_loop
```

装配顺序及 hook/wrapper 行为由
`backend/application/agentthread/adk_middleware_test.go` 的
`TestADKMiddlewareAssemblyPreservesHookAndWrapperOrder` 验证。可选 middleware
可以因能力或模式不适用而跳过，但活跃项必须保持上述相对顺序。

### 工具、MCP 与模型适配

- Eino `tool.BaseTool` 是当前工具合同。
- `mcp-go` 负责 stdio、SSE 和 Streamable HTTP 等 MCP client/transport；
  `adk_mcp_*_eino_runner.go` 将 MCP 工具适配为 Eino tool，并施加 Sandbox、
  安全策略、超时、输出预算和 bounded audit。
- Eino-ext Web 工具当前包含 DuckDuckGo 和 Wikipedia 适配，仍受工具策略和输出
  边界控制。
- `ApplicationADKAgentFactory` 当前直接支持 Eino-ext Ark、Claude、DeepSeek、
  Gemini、OpenAI 和 Qwen model adapter；具体选择取决于 Run/model 配置，它们是
  条件扩展，不是六套 Run 执行器。

## 事件回传

Eino 事件不会原样暴露。`MapADKEvent` 生成公共事件，EventSink 经过 application、
domain 和 MySQL repository 写入 RunEvent。Hertz `StreamCanonicalRun` 创建 Run 并
输出事件，`ReconnectCanonicalRunStream` 按游标重放并跟随 SSE；前端
`useTaskThreadRunEventStream` 委托唯一 client 的 `subscribeRunEvents`，由
`@coze-arch/fetch-stream` 投影 Todo、工具状态、子智能体、澄清卡片和终态。

Hertz/SSE 序列化使用 Sonic。SSE 断线重连、游标去重、取消模式和权限失败由
`backend/api/handler/coze/workbench_canonical_run_stream_test.go` 与前端
`thread-client/__tests__/workbench-run-stream.test.ts` 覆盖。

## 控制与恢复

- Cancel：Hertz `CancelCanonicalRun` -> `ApplicationService.CancelRun` ->
  `threadService.RequestRunCancellation`，先持久化取消事实，再通过
  `ADKCancelRegistry.Cancel` 通知活跃 Eino 执行。
- Human resume：`ResumeCanonicalRun` -> `ResumeHumanInteraction` 先执行 full aggregate replay；
  replay miss 才验证 source Attempt/checkpoint，再由 Human `CreateRunBundle` 分支原子完成 source
  Attempt `interrupted` 与 pending target Attempt rollover。target lineage 随后由
  `ADKExecutor.Resume` 在 `buildRuntime` 前消费为 typed bootstrap；target replay 优先，source typed
  facts 优先，只有 source durable exact NotFound 才进入严格 legacy decoder，并把 admission 与
  baseline decision 在 target fence 下原子提交或回放。
- Subagent retry：`RetryCanonicalSubagentRun` -> `RetrySubagentRun`，根据失败或取消
  的子 Run 创建幂等顶层 retry command bundle。
- Checkpoint resume：`ADKCheckpointStore` 保存 Eino bytes 的内部封装；
  `ADKExecutor.Resume` 使用 resume target 和 Eino Runner 恢复，不向 API/UI 暴露
  checkpoint bytes。
- Multitask rollback：RunProcessor 按持久化 interrupt/rollback 状态清理对应 Eino
  checkpoint，并防止迟到成功覆盖已确定的取消或中断结果。

## 附属数据

- Memory：application/domain/repository 负责增删改、清空、恢复、导入导出和权限；
  middleware 只通过受限 provider 召回或写入事实。
- Artifact：MySQL 保存安全元数据，对象内容通过 Coze object storage abstraction；
  上传、扫描、人工审核、读取、删除和恢复均有权限与审计边界。
- Token Usage：模型回调经 collector 聚合并持久化，公共投影不暴露 provider raw
  payload。
- Guardrail Audit 与 MCP Runtime Audit：仅保存经过清洗的 metadata，支持权限、
  retention 和归档策略，不保存原始请求/响应内容。

## 框架职责与版本来源

版本都来自当前仓库 manifest 或生成文件，不依靠记忆推断：

- UI：React `~18.2.0`、React Router `^6.11.1`、
  `@coze-arch/fetch-stream` workspace package；来源
  `frontend/apps/coze-studio/package.json` 与 canonical client 生产源码。
- API 合同：Thriftgo `0.4.5` 生成模型；来源
  `backend/api/model/workbench/thread_contract/thread.go` 生成头。`task/task.go` 只服务
  Scheduled Task。
- HTTP/SSE：Hertz `v0.10.2`；JSON codec 为 Sonic `v1.15.0`。
- 后端运行：Go `1.24.0`；Run worker 使用 goroutine、context 和 ticker。
- 持久化：GORM `v1.25.11`、GORM MySQL driver `v1.5.7`，本仓库 MySQL
  baseline `8.4.5`。
- Agent 内核：Eino `v0.9.9`。
- 条件模型适配：Ark `v0.1.68`、Claude `v0.1.20`、DeepSeek `v0.1.6`、Gemini
  `v0.1.32`、OpenAI `v0.1.13`、Qwen `v0.1.9`。
- 条件工具适配：DuckDuckGo
  `v2.0.0-20260630024214-84091ffbdce4`、Wikipedia
  `v0.0.0-20260630024214-84091ffbdce4`。
- MCP：mcp-go `v0.43.0`，适配到 Eino tools，不拥有 Run 状态机。
- 可选观测：Prometheus client `v1.20.5`，只观测 worker、Run、模型、MCP、
  memory、artifact 等 bounded metrics。
- 外部入口：robfig cron `v3.0.1` 解析计划；飞书官方 Go SDK `v3.9.9` 负责长连接
  和事件分发。二者最终都进入 agentthread。

## 明确边界

- LangGraph：本地 `/api/threads/**` 与 `/api/runs/**` 兼容 HTTP 路由均已退役；
  canonical SSE 仅保留经审核的 SDK-compatible wire shape，不导入或运行
  LangGraph SDK/runtime。
- DeerFlow：只有 mode、config、指令与行为兼容语义，由当前 Go/Eino 实现；
  不存在 DeerFlow runtime。
- legacy executor：只处理历史无 runtime 标记记录和显式迁移测试；新 Run 拒绝
  `legacy`。
- Runtime Doctor：诊断和建议能力，不拥有状态机。
- Rush、Rsbuild、Vitest、Atlas：构建、测试和迁移工具，不进入生产执行链。
- K2：仍处于设计阶段，不得作为当前节点、执行边或实现依据。
- 旧 `ChatTask`：已退役，不得作为 Workbench 当前实现入口或兼容兜底。

## 更新触发

修改下列任一事实时，必须同步更新本文和 JSON 合同，并重建/校验图谱：

- Workbench/TaskDetail 入口或上传顺序；
- Thrift route、生成 client/model、Hertz route、HTTP handler、公开投影；
- application/domain/repository 调用边界；
- Run admission、claim、lease、worker、runtime selector；
- Eino Runner、Agent、middleware、tool、MCP、checkpoint 或 event mapping；
- cancel/resume/retry/recovery；
- Memory、Artifact、Token、Guardrail/MCP audit；
- 上述框架的 package、版本或 runtime scope。

只改源码而不更新两份权威文件应由图谱校验器拒绝；只改其中一份权威文件也应
拒绝。
