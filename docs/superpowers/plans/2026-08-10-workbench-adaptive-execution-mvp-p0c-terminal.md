# Workbench 自适应执行 MVP P0C Verified Success 执行计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用
> `superpowers:executing-plans` 或 `superpowers:subagent-driven-development` 执行；每个行为改动必须先走
> `superpowers:test-driven-development`，最终结论必须走
> `superpowers:verification-before-completion`。每次只展开当前 Task 的 2～5 分钟小步骤，不提前接入
> P0D、P1D 或 P2。

**Goal:** 只扩展现有唯一 `FinalizeRunSuccess` repository transaction，使可选 gate-on 路径在同一事务
中重校已持久化 Decision/Evidence authority，并保证服务端判定为 passed 的 Verification Event 严格先于
Completion Event；终态 Checkpoint、Assistant Message 与 `Run=succeeded` 保留现有相对写序但属于同一事务，任一
漂移或写失败全部回滚。

**Architecture:** P0C 不新增 finalizer、worker、route、migration 或 application/ADK caller。gate-off
继续走当前 `FinalizeRunSuccess` 原路径且不新增 Decision/Evidence/Plan 查询；outer JournalEvent 非 nil 时仍保留
既有 Attempt/Journal projection 行为。gate-on 复用 P0B 的 v2 checkpoint/event
authority，事务锁序固定为 logical Journal root → Execution Run → Thread → Attempt identity → exact Verification
tuple → Decision/Evidence authority → Plan → PlanItems；Verification 与 Completion 在同一 Attempt 中连续分配序列，
一次事务提交。相同 committed request 的 lost-response retry 在 transient fence 前只读回放原结果；真实 MySQL
cancel/crash race 留给 P0D，本包用 SQLite 行为矩阵与 sqlmock 锁序证明语义。

**P0B base:** `54271fa1ae52d70e589482addbdc7f6a1b49d33b`

---

## 0. 冻结合同

### 0.1 Entry gate 与线性 handoff

- P0B implementation commit 必须精确为
  `54271fa1ae52d70e589482addbdc7f6a1b49d33b`，subject 为
  `feat: add adaptive boundary replay recovery`，parent 为 P0B docs-only commit
  `2088e116d7094bca5e8a0d2b402052ef93e5f634`。
- `/private/tmp/workbench-adaptive-mvp-p0b/state.env` 必须包含 `STATUS=PASS`，且
  `evidence.sha256` fresh 校验通过。
- P0C plan 先作为唯一新增文档提交；docs commit subject 精确为
  `docs: add adaptive execution P0C packet`，parent 必须是上述 P0B implementation commit。
- P0C implementation 从该 docs-only commit 的 clean HEAD 开始；不得把 plan 与代码放在同一提交。
- P0B packet 的已提交 subject `docs: add adaptive replay recovery packet` 视为该 packet 自身计划冻结的
  grandfathered exact subject。P0D 聚合按每个 packet 自己冻结的实际 subject/path 逐项核验，不重写历史。

### 0.2 Exact implementation scope

P0C implementation commit 只允许以下八个文件：

1. `backend/domain/agentthread/repository/adaptive_execution.go`
2. `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
3. `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`
4. `backend/domain/agentthread/repository/repository.go`
5. `backend/domain/agentthread/repository/mysql.go`
6. `docs/superpowers/context/workbench-execution-chain.md`
7. `docs/superpowers/context/workbench-execution-graph.json`
8. `scripts/workbench-execution-graph/contract.mjs`

禁止修改：

- `mysql_journal.go`、migration、PO schema、`repository.go` 以外的公共 repository surface；
- application/domain service、runner/resume runner、ADK、IDL、handler、frontend；
- P0B replay/recovery public contract、现有 finalizer 之外的 terminal path；
- `scripts/workbench-execution-graph.test.mjs`，除非 Node contract 的真实 RED 证明固定断言必须同步；若发生，
  先停止并 amend 本计划，不能静默扩成九文件。

### 0.3 Typed gate

在 `adaptive_execution.go` 增加：

```go
var ErrAdaptiveExecutionVerifiedSuccessInvalid = errors.New("adaptive execution verified success invalid")
var ErrAdaptiveExecutionVerifiedSuccessConflict = errors.New("adaptive execution verified success conflict")

type AdaptiveVerifiedSuccessGate struct {
    Decision                   AdaptiveExecutionBoundaryAuthority
    Evidence                   AdaptiveExecutionBoundaryAuthority
    DecisionID                 string
    DecisionRevision           int64
    VerificationEvent          *entity.RunEvent
    VerificationIdempotencyKey string
}
```

在 repository `FinalizeRunSuccessRequest` 只增加：

```go
AdaptiveGate *AdaptiveVerifiedSuccessGate
```

`FinalizeRunSuccessResult` 只增加 gate-on 可观测结果：

```go
VerificationEvent *entity.RunEvent
Replayed          bool
```

首写为 `Replayed=false`；exact committed retry 为 `true`。gate-off 保持
`VerificationEvent=nil, Replayed=false`，不改变既有 caller 语义。
gate-on 首写与 replay 返回的 `VerificationEvent` 都必须是已经注入 server-owned fingerprints 的 durable
post-image，不能回显 caller 的未注入 payload。

为使 outbox replay 可判定“已存在”而不是悄悄补写，`NotificationOutboxIntent` 只增加可选 callback：

```go
AppendWithResult func(context.Context, *gorm.DB, domainnotification.Event) (inserted bool, err error)
```

gate-off 继续只用既有 `Append`。持久化后的 Verification payload 必须包含 server-owned required nullable
`outbox_fingerprint`：首写无 outbox 时为 JSON `null`；有 outbox 时是对 canonical notification Event 全部
immutable request 字段计算的 64 位小写 SHA-256：EventID、EventType、AggregateType/ID/Version、OccurredAt
毫秒、ActorID、SpaceID、RecipientPolicy、PayloadSchema 与完整 typed Payload；不含 delivery status/attempt 等
mutable worker state。repository 从 request `OutboxIntent.Event` 自己重算，不能信任 caller 的 digest。

gate-on 的 non-nil outbox 必须提供 `AppendWithResult`：首写要求 `inserted=true`，exact replay 要求
`inserted=false`；重放时缺行会先得到 `inserted=true`，随后返回 typed conflict 使本 transaction 回滚该修复写。
durable payload 的 nullable fingerprint 与 retry request 的 presence/fingerprint 必须双向 exact：首写 non-nil→retry
nil、首写 nil→retry non-nil 都在 callback 前冲突。immutable identity drift 保留 notification idempotency cause；
mutable worker state不参与 equality，也不能被 replay 覆盖。P2 才把 application notification service 的真实
callback 接入；它必须验证上述完整 immutable identity。若现有 infra `AppendInTransactionWithResult` 的
`sameOutboxIdentity` 字段集合更窄，P2 必须先加强/包装，不能直接冒充该合同。

持久化后的 Verification payload 还必须包含 server-owned required `finalize_request_fingerprint`（64 位小写
SHA-256）。request `VerificationEvent.Payload` 必须不携带这两个 server-owned 字段；repository
在首写 title CAS 已确定 `TitleUpdated` 与 selected terminal checkpoint 后、插入 Verification 前，从规范化的
durable semantic projection 自己重算；不能信任 caller 的 digest。projection 必须包含：

- outer RunID、ExecutionGeneration、Now（明确排除 ephemeral LeaseOwner/LeaseToken）；
- 从首写 locked pre-image 应用唯一允许 mutation 后得到的完整 canonical terminal RunPO 与 AttemptPO expected
  post-image；JSON 字段按 UseNumber/canonical value，pointer/SQL NULL 与零值严格区分；
- Decision/Evidence authority、DecisionID/Revision；
- Verification Event identity/tuple/key 与 strict payload（只把 `finalize_request_fingerprint` 自字段固定清空）；
- Completion Event 与 normalized JournalEvent 的完整待落库 identity/payload/tuple；
- Message、nullable TitleEvent、ExpectedThreadTitle、ThreadTitle、`TitleUpdated` 与提交后的 Thread title；
- primary/fallback terminal checkpoints、实际 selected checkpoint 的 ID/完整 canonical body；
- nullable canonical outbox fingerprint。

所有 JSON 用 UseNumber + 单值 EOF + deterministic marshal；server-owned sequence/tuple 先填入再算，然后
repository 把两个摘要注入待持久化 payload。pure validator 按实际 nullable outbox 形态与两个等长 64-byte digest
占位值预留 payload budget，并在第一条 DB 访问前拒绝超限；事务内确定真实摘要后再次 canonical marshal，长度必须
与预检预算一致且不超过 64 KiB，否则作为不变量失败回滚整个 transaction，不能声称在 title CAS 之后仍是“DB 前”
错误。该 digest
把 Verification tuple 外锚到本次 committed request/post-image，禁止 retry 把 Message、Checkpoint、title
precondition 或 outbox presence 换成另一个碰巧存在的 durable row。它不包含 outbox delivery mutable state，也
不把当前 lease/cancel 状态混入 replay equality。

冻结语义：

- `nil`：完全保持 gate-off 行为，不新增 Journal root、Decision/Evidence、Plan 或 adaptive checkpoint 查询；不生成
  Verification Event。outer JournalEvent 为 nil 时可在没有 Attempt/Plan 表的 legacy fixture 成功；非 nil 时仍允许
  既有 terminal Journal helper 读取/投影 Attempt，不能把旧行为误称为零 Attempt 查询。
- 非 nil 的纯结构/payload 错误统一 wrap `ErrAdaptiveExecutionVerifiedSuccessInvalid`；transaction 内 durable
  missing/drift 统一 wrap `ErrAdaptiveExecutionVerifiedSuccessConflict`，并用多 `%w` 保留既有 lease/cancel、
  Attempt、Checkpoint、Plan/Item、Sequence cause。
- gate-on 要求 outer `req.Now > 0`；Verification Event `CreatedAt` 与 payload `created_at` 都必须 exact 等于
  `req.Now`。nil gate 继续保留 legacy `Now=0` 的既有 normalization，不改变旧 caller。
- 为 Verification、Completion 与 terminal `NextSequence` 留足三个 uint64 slot，要求
  `Evidence.EventSequence <= math.MaxUint64-3`；`MaxUint64-2/-1/MaxUint64` 全部 pure fail closed，禁止算术回绕。
- 非 `nil`：Decision/Evidence 都必须是 P0B 返回的完整 authority，而不是 caller 自报的散乱 ID；
  repository 必须从物理 Event/Checkpoint/Attempt/Plan/Item 重新计算，不信任 gate 内容本身。
- `DecisionID` 与 `DecisionRevision` 分别为 1～191 bytes 的非空 opaque ID 和正数 revision。
- `VerificationEvent` 必须属于 outer Run/Thread，`EventType=adaptive.verification`、`CreatedAt=req.Now`；
  idempotency key 为 1～191 bytes，且不得与 Completion Journal key 相同。
- gate-on 若存在 TitleEvent，ID 顺序必须是 `TitleEvent.ID < VerificationEvent.ID < CompletionEvent.ID`；无
  TitleEvent 时仍要求 Verification 严格早于 Completion。
- gate-on 的 outer `JournalEvent` 必须非 nil 且能规范化为 `run.completed`；否则 fail closed，禁止退化为只写
  base Completion 而遗漏 Attempt terminal authority。
- Decision 与 Evidence 的 Thread、Execution Run、generation、Journal Run、Attempt、source pair、
  Plan scope 必须一致；`Decision.EventSequence <= Evidence.EventSequence`。
- `TerminalCheckpoint` 在 gate-on 必须存在，并满足
  `ParentCheckpointID == Evidence.CheckpointID`、`RuntimeDeletedAt == 0`，并与 Evidence checkpoint 的
  CheckpointNS/RuntimeType/RuntimeKey/EnvelopeVersion exact 相同；若提供 title-conflict fallback，它也必须满足
  相同 parent/runtime identity，且仍必须通过现有 identity equality。

### 0.4 Verification payload 的 P0C 窄投影

P0C 不抢占 P1D 的完整 Decision codec 或 P2 的完整 Verification codec，只从持久化 JSON object 读取终态必需
字段；允许完整 codec 的其他字段，
但 required 字段缺失、类型错误、多 JSON 值或 payload 超过 64 KiB 一律 fail closed：

```text
schema = workbench-adaptive-verification.v1
verification_id = 1..191 byte opaque id
execution_run_id / journal_run_id / attempt_id / execution_generation
decision_id / decision_revision
expected_plan_revision
expected_plan_fingerprint
verified_checkpoint_id
evidence_head_event_id
outbox_fingerprint = null | 64hex lowercase
finalize_request_fingerprint = 64hex lowercase
status = passed
created_at
```

Decision Event 的窄投影只读取：

```text
schema = workbench-adaptive-decision.v1
decision_id
decision_revision
```

上表描述的是 durable payload；caller 输入必须缺省两个 server-owned fingerprint。Verification payload 其余字段
必须逐字段绑定 outer Run/generation、gate 的 Decision、Evidence Plan revision、
Evidence checkpoint 与 Evidence event。`failed`、`blocked`、缺字段、旧 decision、旧 Plan 或旧 evidence
都不能 succeeded。`expected_plan_fingerprint` 必须是 64 位小写 hex，并由全部 current PlanItems 按
`task_id ASC` 复用 P0B canonical JSON/fingerprint 算法计算。Verification/Completion payload 与 terminal
checkpoint 的 ChannelValues、ChannelVersions、PendingSends、Metadata 在 replay 时都做 UseNumber + 单值 EOF +
deterministic marshal 的语义比较，不做 raw JSON bytes 比较。replay 比较 caller payload 时先从 durable payload
剥离两个 server-owned fingerprint，再比较其余语义字段；摘要本身必须从 retry request/post-image重算。

通用 `CommitAdaptiveExecutionBoundary` 不拥有 passed capability：当 EventType 是
`adaptive.verification` 且窄投影 `status=passed` 时，必须在纯验证阶段 fail closed 且零写。既有 duplicate
verification replay 用例改为 `failed` 或 `blocked`；只有本节冻结的 `FinalizeRunSuccess` 内部 transaction
可以创建被 adaptive success/replay 接受的 authoritative passed Verification。generic base/journal event 不能
仅凭同名 EventType/payload 被当作 verified-success authority；P2 public projection 必须要求本节的 Attempt terminal
tuple、Evidence anchor 与 Completion coupling，不能按字符串事件名判定 passed。

### 0.5 事务锁序与 authority 重校

gate-on transaction 的读取/锁顺序固定：

1. `JournalRunID` logical root `FOR UPDATE`，校 Thread、top-level Journal root；
2. 若 root ID != outer `RunID`，再锁 Execution Run；相同 ID 复用同一 `runPO`，不二次查询；
3. 以 authority/outer ThreadID 锁 Thread `FOR UPDATE`，校 tenant identity，并把同一锁持有到 title CAS/replay
   返回；这是与 `CreateJournalSnapshot`/reservation 的既有 `Thread → Attempt` 顺序对齐，禁止先锁 Attempt 再补
   Thread；
4. exact `(JournalRunID, AttemptID)` `FOR UPDATE`，先只校 Thread、ExecutionRunID 与 source pair 的 durable
   identity，不提前要求 active slot/status；
5. 按 Verification idempotency tuple 做 exact current read；命中时进入 0.6 的 committed-result replay，不再
   要求旧 lease、running、active slot 或未 cancel；
6. tuple 未命中时才重校 outer generation、lease owner/token、expiry、running、cancel nil，以及 Attempt
   running、active slot=1、cursor/high-watermark；
7. 按稳定 ID 顺序锁 Decision/Evidence exact Checkpoint 与 Event，strict decode v2 metadata，重算
   checkpoint fingerprint、Event `snapshot_id` 外锚与 25/25 Event fingerprint；
8. Decision payload exact；以 `sequence <= Evidence.EventSequence` 为闭区间上界，对同一 Attempt 的
   `adaptive.decision` 做 current/locking read 并按 sequence DESC 取第一行，它必须就是 gate 的 Decision
   EventID/sequence。这样 Evidence 自身即便也是 decision 也不会被 `<` 端点漏掉；
9. Evidence authority 必须是当前高水位：
   `Attempt.LastCommittedSequence == Evidence.EventSequence` 且
   `Attempt.NextSequence == Evidence.EventSequence + 1`；
10. 锁 Evidence `PlanScopeRunID`、Plan，并按 `task_id ASC` 锁全部 current PlanItems；校 tenant、revision，
   `Plan.UpdatedAt == Evidence Event.CreatedAt`，且 `HighWatermark` 不小于最后一个 current Item TaskID（允许
   `ReservePlanTaskID` 已推进但尚未 Upsert 的合法 reservation gap）；逐项核
   metadata refs 的 ID/task/version 与 subset fingerprint，再对全部 current Items 计算 canonical fingerprint，
   绑定 Verification payload 的 `expected_plan_fingerprint`；
11. 验证 Verification payload 与 terminal checkpoint parent；所有验证完成后才允许第一条写。

Decision checkpoint 只验证其 immutable Event/Checkpoint authority、payload 与 latest-decision 身份；不得要求
当前 Plan 回退到 Decision 当时的旧 revision。current Plan/Items 只以 Evidence authority 重校，因为合法 Evidence
本来就可以在 Decision 后推进 Plan。

禁止：

- 在已锁 Execution Run 后再补锁 logical root；
- 调公开 `CommitAdaptiveExecutionBoundary` 开第二个 transaction；
- 用 public `ListRunEvents`/`ListCheckpoints`、latest selector 或 Journal safe projection 当 authority；
- 用数值排序替代上面的语义锁序；
- 增加 retry、sleep、advisory lock、test-only hook 或 global failpoint。

### 0.6 单事务写序

gate-on 必须在现有 `FinalizeRunSuccess` transaction 内完成：

1. 将 `VerificationEvent` 转为 `runEventPO`，强制写入当前 Journal/Attempt、
   `sequence=old Attempt.NextSequence`、Verification idempotency key，并用 Evidence checkpoint fingerprint
   填充 `snapshot_id`；
2. 构造 Completion Event：
   - persisted `projection_state=healthy` 时必须通过 terminal Journal normalization 并写安全投影；任何 request
     normalize、CAS 或 DB 错误都整事务 rollback，不得静默标 degraded/base fallback；
   - 只有进入事务前已经 durable `projection_state=degraded` 时，才直接写带权威 tuple 的 base Completion row；
     safe projection 可缺省但不能丢 JournalRunID/AttemptID/sequence/idempotency key；其他 projection state fail closed；
3. Verification sequence 必须小于 Completion sequence；两者 ID 也要求
   `VerificationEvent.ID < CompletionEvent.ID`；
4. Attempt 以 locked cursor 做一次 terminal CAS：status=completed、active_slot=nil、
   `next_sequence=verification+2`、`last_committed_sequence=verification`、
   `terminal_event_id=completion`、`ended_at=req.Now`、`updated_at=req.Now`；
5. 沿现有 finalizer 写 Run succeeded、Assistant Message、可选 title、选中的 TerminalCheckpoint 与 outbox；
   允许在代码中维持现有相对写序，但 Verification 必须在 Completion 前；gate-on 的 outbox callback 必须是
   Run/Attempt/Message/title/Verification/Completion/selected TerminalCheckpoint 之后的最后一个 durable step，所有写都
   属于同一 transaction；
6. 任一 event duplicate、Attempt CAS、Run fence、message/title、completion、checkpoint 或 outbox 错误都回滚
   Run、Attempt、Message、Thread、Events、Checkpoint、Plan/Items 的本事务变化。

lost-response replay 必须在 root/run/thread/attempt 与 exact Verification tuple current-read 之后、transient fence 之前
分流，但 tuple 命中只跳过 transient fence，不能跳过 authority/post-image 锁。它继续按 0.5 的相同顺序执行
Decision/Evidence checkpoint+event、latest decision、Plan scope/Plan/全部 current PlanItems 的 locking/current reads；
原 step 9 的 active cursor 条件替换为 terminal Attempt post-image（LCS=Verification sequence、Next=Completion
sequence+1、terminal_event_id=Completion）重校。PlanItems 后复用一直持有的 Thread lock，并 current-read exact Message、
可选 TitleEvent、Completion 与 selected TerminalCheckpoint；所有 digest/post-image 比较与返回都在这些锁持有期间完成，
禁止 plain snapshot 读完后无锁返回。

terminal Run/Attempt 不是只核 status。首写从 locked pre-image 克隆完整持久化 PO、应用唯一允许的终态 mutation，
把两份 canonical expected post-image 纳入 durable `finalize_request_fingerprint`；new repository replay 不猜首写
pre-image，而是以当前 locked full PO 重算同一 projection 并与 stored fingerprint exact compare。Run 只允许
status=succeeded、error_code/error_message 为空、
`ended_at=updated_at=req.Now`，并按 `clearRunLeaseUpdates` 清空 worker_id、lease_owner/token/expires、heartbeat 与
cancel_requested_at；ID/Thread/tenant/execution_generation 及其余字段保持 pre-image。Attempt 只允许
status=completed、active_slot=nil、Next/LCS/terminal_event_id、`ended_at=updated_at=req.Now` 改为 0.6 冻结值；
ID/Thread/Journal/Execution/Attempt/Ordinal/source pair/recovery key/enrollment/snapshot/projection/trace/created/started
及其余字段保持 pre-image。任一字段漂移都必须 typed conflict，不能返回 replay。

它先从 retry request 与 durable outcome 重算 `finalize_request_fingerprint` 并与 Verification payload exact
比较，再逐项比较 Verification、Completion、Message、selected TerminalCheckpoint、Decision/Evidence
authority、Plan/Items、TitleEvent/TitleUpdated、Thread title 与 terminal Run/Attempt post-image；OutboxIntent
presence/fingerprint 必须先与 durable Verification payload 双向 exact；只有 stored/request 均 non-nil 时才允许
调用 `AppendWithResult` 做 immutable identity compare，且必须返回 `inserted=false` 并保持 durable outbox row
精确一行；缺行时 callback 的临时 insert 随 typed conflict 一起回滚。整体精确一致时返回
同一 committed result 且零 durable state change，Verification result
标记 replay。任一
命中相同 Verification `(JournalRunID, AttemptID, IdempotencyKey)` selector 后的 request/durable post-image 漂移统一
`ErrAdaptiveExecutionVerifiedSuccessConflict`，不得把当前 Run lease/cancel 或 inactive Attempt 当作 replay 冲突。
若 caller 改掉 selector 本身，它不是 replay candidate，tuple miss 后按正常 transient fence/first-write 规则失败；
不得为了猜测“可能的旧请求”扫描 latest Verification。

gate-off 原样调用现有 `persistTerminalRunEventWithJournal`；gate-on 使用 P0C package-private helper，不能改
其他 terminal caller。

### 0.7 冻结测试矩阵

顶层测试名固定（exact 10）：

1. `TestValidateAdaptiveVerifiedSuccessGate`
2. `TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites`
3. `TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables`
4. `TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically`
5. `TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt`
6. `TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites`
7. `TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure`
8. `TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome`
9. `TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection`
10. `TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites`

pure-invalid table 属于 `TestValidateAdaptiveVerifiedSuccessGate`，统一断言
`ErrAdaptiveExecutionVerifiedSuccessInvalid` 且零 DB access：Evidence sequence
`MaxUint64-2/-1/MaxUint64`、caller verification status/identity/ID/CreatedAt/order、terminal parent、
RuntimeDeletedAt 与结构性 runtime identity 错误都只在这里覆盖，不进入 durable conflict table。

Authority drift table 每个 subtest 使用独立 SQLite DB，先封存全表 snapshot，再调用 public
`FinalizeRunSuccess`，统一断言 `ErrAdaptiveExecutionVerifiedSuccessConflict`（lease/cancel 保留既有 typed
cause）与零写。snapshot 除 Thread/Run/Attempt/Message/Event/Checkpoint/Plan/Items 外，non-nil outbox case
必须包含由 test callback 在同一 transaction 操作的 test outbox rows。至少覆盖：

- Attempt terminal/slot nil/execution drift/cursor stale；
- Decision missing、event/checkpoint tuple/fingerprint/payload drift、单独插入 newer decision；
- Evidence missing、source/generation/event anchor/Plan scope/revision/ref/version/fingerprint drift；
- Evidence 不再是 LCS 或 NextSequence 高水位；
- 预先存在 exact Verification selector，但 durable stored payload 的 server fingerprints、Completion coupling 或
  terminal post-image 与 valid retry request 不一致；
- caller terminal input 本身 valid，但 durable Evidence checkpoint 的 parent/runtime/fingerprint authority 已漂移。

`Decision` 子例必须构造 Decision-1 → Decision-2 → Evidence，再让 gate 指向 Decision-1，证明查询的闭区间
端点与 latest 身份都被执行。`PlanItems` 子例必须直接漂移一个不在 Evidence `ItemRefs` 中的 completed sentinel
Item（保持 Evidence subset fingerprint 自洽），证明 gate 校验的是全部 current PlanItems，而不是只复用 P0B
mutation subset。`Plan` 子例包含 `HighWatermark` 低于最大 TaskID 的腐坏反例；Atomic happy 顶层另含
`ReservedHighWatermarkGapRemainsValid`，证明合法 reservation gap 不被误拒。
`Verification` 使用上述 pre-existing selector 的 durable mismatch；`TerminalCheckpointReference` 使用 valid caller
terminal input + drifted durable Evidence checkpoint，二者都不得拿 pure-invalid request 冒充 conflict RED。

以下六个核心 subtest 名必须逐名 RUN/PASS，不能只靠顶层 PASS：`Plan`、`PlanItems`、`Decision`、
`Verification`、`EvidenceHighWatermark`、`TerminalCheckpointReference`。

lock-order 顶层必须包含并逐名 PASS：`DistinctJournalAndExecutionRuns` 与
`SharedJournalAndExecutionRun`；两者都必须在 exact Attempt 前先出现 Thread `FOR UPDATE`，后者只允许一条
Run `FOR UPDATE`，完整 ordered expectation 为 root/Execution（按是否同 ID 复用）→Thread→Attempt→tuple。

nil-gate 顶层必须逐名 PASS：`WithoutJournalEventDoesNotRequireAdaptiveTables`、
`WithJournalEventPreservesLegacyAttemptProjection`。degraded 顶层必须逐名 PASS：
`DurablyDegradedWritesAuthoritativeBaseTuples`、`HealthyRejectsInvalidJournalProjectionWithoutWrites`。

rollback late-failure 使用 non-nil test outbox 的 `AppendWithResult` 作为事务最后一步：callback 先用收到的同一
`*gorm.DB` current-read 并逐项证明本事务内 Run 已 succeeded、Attempt 已 terminal、Message、Verification、
Completion 与 selected TerminalCheckpoint 都可见，然后返回 frozen sentinel error。public finalizer 必须返回并
保留该 DB cause；事务退出后全表 snapshot 与调用前 exact 相等，以上行都不存在/恢复原值。callback 未观察到任一
预期写入时测试立即失败，禁止预检后提前返回来伪造“晚期回滚”；不得使用 global hook/failpoint，duplicate Message
或预置 checkpoint 冲突也不能作为这个证明。调用 public method 前还必须查询并断言新 Message/Verification/
Completion/selected Checkpoint IDs 全部不存在，禁止用预 seed 行伪造 callback 内可见性。Atomic/degraded/replay happy path 还必须逐项断言 Attempt
`EndedAt==UpdatedAt==req.Now`。

replay 测试必须先完成一次带 title 与 outbox 的真实 public finalizer commit，再把 Run 置于既有 succeeded
post-image、Attempt 保持 terminal/inactive、旧 lease 失效；使用同一 request 由新 repository instance 重试，
断言返回原 Verification/Completion/Message/TitleEvent/TitleUpdated/Checkpoint/Run，outbox 仍精确一行且全库
snapshot 不变。另以独立 fixture 漂移 completion、message、title、selected terminal checkpoint、outbox 或 Plan
revision，统一 typed conflict 与零写；outbox 至少逐名覆盖
`InitialOutboxRetryWithoutOutbox`、`InitialWithoutOutboxRetryWithOutbox` 与
`InitialOutboxRetryMissingDurableRow`。另逐名覆盖 request substitution：`MessageIDOrBody`、
`SelectedCheckpointIDOrBody`、`UnselectedCheckpointIDOrBody`、`ExpectedOrUpdatedTitle`、
`UnpersistedTitleEventIDOrBody`、`OutboxImmutableRequestIdentity`，以及 durable terminal drift：`RunPostImage`、
`AttemptPostImage`。后两例至少分别漂移只能由 durable full-post-image digest 捕获的 Run Metadata/Context 与
Attempt TraceID/EnrollmentVersion，必须证明 full-PO expected post-image compare 生效；每个 request substitution
即使替换后的 durable row 预先存在且内容相同，也必须因
`finalize_request_fingerprint` 不同而 conflict，不能返回 replay。

cancel 语义用两个逐名子例 `CancelFirst`、`SuccessFirst` 的独立 fixture 证明两种串行结果：

- cancel-first：finalizer 返回 `ErrRunCanceled`，仅 canceled terminal 存在；
- success-first：cancel 被现有状态机拒绝，仅 Verification→Completion→succeeded 存在。

P0D 再用真实 MySQL 双连接/barrier 证明竞态；P0C 不伪装成 MySQL race 证据。

---

## 1. Docs-only handoff

### Task 1：创建并提交唯一 P0C packet

**Files:**

- Create: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md`

- [ ] **Step 1（2 分钟）：核 P0B HEAD、clean 与 evidence。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git rev-parse HEAD)" = 54271fa1ae52d70e589482addbdc7f6a1b49d33b
  test "$(git show -s --format=%s HEAD)" = 'feat: add adaptive boundary replay recovery'
  test "$(git rev-parse HEAD^)" = 2088e116d7094bca5e8a0d2b402052ef93e5f634
  test "$(git show -s --format=%s 2088e116d7094bca5e8a0d2b402052ef93e5f634)" = \
    'docs: add adaptive replay recovery packet'
  test "$(git rev-parse 2088e116d7094bca5e8a0d2b402052ef93e5f634^)" = \
    a1df1789b0b60c0916505711bcc1e5a1fa1410cd
  diff -u <(printf '%s\n' \
    $'M\tbackend/domain/agentthread/repository/adaptive_execution.go' \
    $'M\tbackend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    $'M\tbackend/domain/agentthread/repository/mysql_adaptive_execution_test.go' \
    $'M\tdocs/superpowers/context/workbench-execution-chain.md' \
    $'M\tdocs/superpowers/context/workbench-execution-graph.json' \
    $'M\tscripts/workbench-execution-graph/contract.mjs' | LC_ALL=C sort) \
    <(git diff-tree --no-commit-id --name-status -r HEAD | LC_ALL=C sort)
  (cd /private/tmp/workbench-adaptive-mvp-p0b && shasum -a 256 -c evidence.sha256)
  grep -Fx 'STATUS=PASS' /private/tmp/workbench-adaptive-mvp-p0b/state.env
  ```

- [ ] **Step 2（2 分钟）：核唯一新文件与 whitespace。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git status --porcelain)" = '?? docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md'
  set +e
  git diff --no-index --check /dev/null \
    docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md \
    > /private/tmp/workbench-adaptive-mvp-p0c-plan-diff-check.log 2>&1
  p0c_plan_diff_status=$?
  set -e
  test "$p0c_plan_diff_status" = 1
  test ! -s /private/tmp/workbench-adaptive-mvp-p0c-plan-diff-check.log
  ```

- [ ] **Step 3（3 分钟）：双审 plan，P0/P1 必须清零。**

  Spec reviewer 核 0.1～0.7；quality reviewer 把全部 shell fence 抽出后分别跑 `bash -n` 与
  `zsh -n`，并验证 exact-eight、RED/GREEN test names、evidence manifest、P0D handoff 没有自相矛盾。
  双审通过后执行：

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  shasum -a 256 \
    docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md \
    | awk '{print $1}' \
    > /private/tmp/workbench-adaptive-mvp-p0c-plan-reviewed.sha
  ```

- [ ] **Step 4（2 分钟）：提交 docs-only packet。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git add docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md
  git diff --cached --check
  test "$(git diff --cached --name-only)" = \
    'docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md'
  p0c_parser_package="$(find common/temp/install-run \
    -path '*/node_modules/@babel/parser/package.json' -print -quit)"
  test -n "$p0c_parser_package"
  p0c_node_modules="${p0c_parser_package%/@babel/parser/package.json}"
  NODE_PATH="$p0c_node_modules" git commit -m 'docs: add adaptive execution P0C packet'
  test "$(git show -s --format=%s HEAD)" = 'docs: add adaptive execution P0C packet'
  test "$(git rev-parse HEAD^)" = 54271fa1ae52d70e589482addbdc7f6a1b49d33b
  test "$(git diff-tree --no-commit-id --name-status -r HEAD)" = \
    $'A\tdocs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md'
  git show HEAD:docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md \
    | shasum -a 256 | awk '{print $1}' \
    > /private/tmp/workbench-adaptive-mvp-p0c-plan-committed.sha
  diff -u \
    /private/tmp/workbench-adaptive-mvp-p0c-plan-reviewed.sha \
    /private/tmp/workbench-adaptive-mvp-p0c-plan-committed.sha
  test -z "$(git status --porcelain)"
  ```

---

## 2. Implementation entry gate

### Task 2：初始化 fail-closed evidence

- [ ] **Step 1（2 分钟）：记录 docs base 与 plan hash。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git show -s --format=%s HEAD)" = 'docs: add adaptive execution P0C packet'
  test "$(git rev-parse HEAD^)" = 54271fa1ae52d70e589482addbdc7f6a1b49d33b
  test -z "$(git status --porcelain)"
  test ! -e /private/tmp/workbench-adaptive-mvp-p0c
  mkdir /private/tmp/workbench-adaptive-mvp-p0c
  git rev-parse HEAD | tee /private/tmp/workbench-adaptive-mvp-p0c/base.sha
  shasum -a 256 docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/plan.sha256
  printf '%s\n' 'PACKET=P0C' 'STATUS=active' \
    > /private/tmp/workbench-adaptive-mvp-p0c/state.env
  : > /private/tmp/workbench-adaptive-mvp-p0c/remaining-risk.txt
  ```

- [ ] **Step 2（3 分钟）：fresh baseline。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    go test -p 1 ./domain/agentthread/repository -count=1 -timeout=240s \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/baseline.log
  ```

---

## 3. Contract TDD

### Task 3：typed gate 与 nil boundary

**Files:**

- Modify: `backend/domain/agentthread/repository/adaptive_execution.go`
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`

- [ ] **Step 1（5 分钟）：只写 validator、passed-capability 与 nil regression tests。**

  新增 `TestValidateAdaptiveVerifiedSuccessGate` 与
  `TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites`、
  `TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables`，覆盖 valid、nil、authority
  identity/source drift、sequence drift、ID/revision/key bounds、caller spoof server-owned fingerprint 的拒绝、durable
  payload 对 nullable outbox/required finalize request fingerprint 的 strict decode、Event/Terminal/Completion
  identity/order、注入两个 64-byte server fields 后的 64 KiB 边界，以及 Evidence sequence
  `MaxUint64-2/-1/MaxUint64` exhaustion；nil test 必须含无 Journal 表分支与
  既有 Journal Attempt projection 分支。通用 boundary
  的 passed verification 零写拒绝；gate-off 不存在 adaptive 表仍成功。既有 duplicate verification replay 改用
  `failed` 或 `blocked`。此步不改 production。

- [ ] **Step 2（2 分钟）：运行 Contract RED。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  set +e
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^(TestValidateAdaptiveVerifiedSuccessGate|TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites|TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables)$' \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/contract-red.log
  p0c_contract_status=$?
  set -e
  test "$p0c_contract_status" -ne 0
  rg -n 'undefined: (AdaptiveVerifiedSuccessGate|validateAdaptiveVerifiedSuccessGate)|FinalizeRunSuccess(Request|Result).*has no field or method (AdaptiveGate|VerificationEvent|Replayed)' \
    /private/tmp/workbench-adaptive-mvp-p0c/contract-red.log
  if rg -n 'panic:|no tests to run|unknown column|fixture' \
    /private/tmp/workbench-adaptive-mvp-p0c/contract-red.log; then
    false
  else
    test "$?" = 1
  fi
  ```

- [ ] **Step 3（5 分钟）：最小 contract GREEN。**

  只增加 0.3 的 error/type、pure validator、repository request/result optional field，并让通用 boundary 在
  pure validation 中拒绝 passed verification；`NotificationOutboxIntent` 只增加 optional `AppendWithResult`。
  validator 不访问 DB；nil 直接返回 nil。暂不实现 gate-on transaction。

- [ ] **Step 4（2 分钟）：运行 Contract GREEN 并逐名断言。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^(TestValidateAdaptiveVerifiedSuccessGate|TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites|TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables)$' \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/contract-green.log
  for p0c_test in \
    TestValidateAdaptiveVerifiedSuccessGate \
    TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites \
    TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables; do
    test "$(rg -c -F -- "--- PASS: ${p0c_test} (" /private/tmp/workbench-adaptive-mvp-p0c/contract-green.log)" = 1
  done
  for p0c_case in WithoutJournalEventDoesNotRequireAdaptiveTables WithJournalEventPreservesLegacyAttemptProjection; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/contract-green.log)" = 1
  done
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed' \
    /private/tmp/workbench-adaptive-mvp-p0c/contract-green.log; then
    false
  else
    test "$?" = 1
  fi
  ```

---

## 4. Happy-path atomic terminal TDD

### Task 4：Verification → Completion → succeeded

**Files:**

- Modify: `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`

- [ ] **Step 1（5 分钟）：写 happy、order、root/thread-lock、late-rollback 与 cancel tests。**

  fixture 用 public P0B boundary 先持久化 Decision/Evidence，保留 active Attempt；Plan 额外 seed 一个已完成、
  不在 Evidence `ItemRefs` 中的 sentinel Item，Verification 的 expected fingerprint 必须覆盖它与全部 current
  Items。`Commits...Atomically` 顶层内增加
  `AllCurrentPlanItemsFingerprintRejectsUnreferencedDrift` 子例并先取得真实行为 RED，同时增加
  `ReservedHighWatermarkGapRemainsValid`；finalizer request 携带
  passed Verification、Completion、TerminalCheckpoint。断言 Run/Message、Verification sequence、Completion
  sequence、Attempt LCS/terminal_event_id/EndedAt/UpdatedAt、Checkpoint parent、Plan/Items readback全部一致；stored
  Verification 的 nullable outbox/finalize-request 两个 server-owned fingerprint 必须分别等于 repository 从 durable
  post-image 重算的值，注入后的 canonical payload 仍在 64 KiB 内。test 必须用手工构造的 typed literal projection +
  canonical marshal + SHA-256 作为独立 oracle，禁止调用 production fingerprint helper 自证。sqlmock 必须从 public `FinalizeRunSuccess`
  证明第一把 Run lock 是 Journal root，第二把才是不同的 Execution Run，随后 Thread `FOR UPDATE` 严格先于 exact
  Attempt；同一顶层 Run 同时作为 Journal root/Execution Run 的 subtest 必须证明只锁一次 Run，再按
  Thread→Attempt。

  同一步先新增 `TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure`：使用 0.7
  的 tx-bound `AppendWithResult` sentinel，callback 必须先看到全部终态写再失败，最终 snapshot exact rollback。
  同时新增 `TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome` 的
  `CancelFirst`/`SuccessFirst` 两个串行 fixture。这样两个 digest 注入、callback 最后一步、rollback 与 terminal
  linearization 都有行为 RED 后才允许 Step 4 实现。

- [ ] **Step 2（2 分钟）：运行 Atomic RED。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  set +e
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^(TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome)$' \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/atomic-red.log
  p0c_atomic_status=$?
  set -e
  test "$p0c_atomic_status" -ne 0
  rg -n -- '--- FAIL: (TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome)' \
    /private/tmp/workbench-adaptive-mvp-p0c/atomic-red.log
  for p0c_test in \
    TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt \
    TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome; do
    test "$(rg -c -F -- "--- FAIL: ${p0c_test} (" /private/tmp/workbench-adaptive-mvp-p0c/atomic-red.log)" = 1
  done
  if rg -n -- '--- SKIP:|panic:|build failed|no tests to run' \
    /private/tmp/workbench-adaptive-mvp-p0c/atomic-red.log; then
    false
  else
    test "$?" = 1
  fi
  ```

- [ ] **Step 3（5 分钟）：只实现 happy-path 所需的 root/run/thread/attempt 与 authority locks。**

  只实现让 Task 4 两个 happy/lock tests 转绿的 identity locks、valid-row decode、全部 current PlanItems
  fingerprint 与最小 post-image readback；Thread lock 必须在 Attempt 前取得并贯穿 title CAS/replay；
  本步骤只实现 persisted projection_state=healthy；durably-degraded 分支必须留到 Task 6 先取得 RED 后实现，
  禁止按全局 0.6 提前顺手补齐；
  暂不实现 newer-decision 扫描、HWM 漂移、payload 其余字段 drift 与细分 cause mapping。
  这些必须留给 Task 5 先取得行为 RED 后再逐类补齐，禁止在本步骤预做而让 Drift RED 失真。

- [ ] **Step 4（5 分钟）：实现单事务 Verification/Completion helper。**

  不调用 public P0B method，不改 `mysql_journal.go`。在 gate-on branch 写两 Event，并用一次 Attempt terminal
  CAS 设置 Next/LCS/status；title CAS 结果确定后、Verification insert 前由 repository 注入 nullable outbox 与
  finalize-request 两个 server-owned fingerprint；finalize projection 必须包含从 locked full Run/Attempt pre-image
  推导出的 expected terminal post-images，再重跑 payload size gate。gate-off 保持原 terminal helper。
  outbox callback 保持最后 durable step；任何 failure 返回 transaction error。

- [ ] **Step 5（2 分钟）：Atomic GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^(TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome)$' \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log
  for p0c_test in \
    TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt \
    TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome; do
    test "$(rg -c -F -- "--- PASS: ${p0c_test} (" /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log)" = 1
  done
  for p0c_case in DistinctJournalAndExecutionRuns SharedJournalAndExecutionRun; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log)" = 1
  done
  for p0c_case in CancelFirst SuccessFirst; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log)" = 1
  done
  test "$(rg -c -F -- '--- PASS: TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically/AllCurrentPlanItemsFingerprintRejectsUnreferencedDrift (' \
    /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log)" = 1
  test "$(rg -c -F -- '--- PASS: TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically/ReservedHighWatermarkGapRemainsValid (' \
    /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log)" = 1
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed' \
    /private/tmp/workbench-adaptive-mvp-p0c/atomic-green.log; then
    false
  else
    test "$?" = 1
  fi
  ```

---

## 5. Drift 与 rollback TDD

### Task 5：fail-closed authority matrix

- [ ] **Step 1（5 分钟）：添加 drift table 与完整 snapshot helper。**

  `TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites` 按 0.7 每例独立 DB；snapshot 至少含
  Threads、Runs、Attempts、Messages、Events、Checkpoints、Plans、PlanItems 与 test outbox rows，按稳定主键排序并
  深比较。outbox callback 必须用收到的同一个 `*gorm.DB` transaction 读写 test table，不能用内存计数冒充
  rollback 证据。

- [ ] **Step 2（2 分钟）：运行 Drift RED。**

  `PlanItems` 是 Task 4 已经 RED→GREEN 的全量 fingerprint regression；本步 RED 必须由尚未实现的 Decision、
  Evidence HWM、Verification 或 terminal reference 类别驱动，不能要求该子例再次变红。

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  set +e
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites$' \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/drift-red.log
  p0c_drift_status=$?
  set -e
  test "$p0c_drift_status" -ne 0
  rg -n -- '--- FAIL: TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites' \
    /private/tmp/workbench-adaptive-mvp-p0c/drift-red.log
  if rg -n 'panic:|build failed|no tests to run' \
    /private/tmp/workbench-adaptive-mvp-p0c/drift-red.log; then
    false
  else
    test "$?" = 1
  fi
  ```

- [ ] **Step 3（5 分钟）：逐类补严格重校。**

  每次只处理一个失败类别：Attempt cursor → Decision active → Evidence fingerprint/HWM → Plan/Items →
  Verification payload → terminal ref/order。禁止为测试增加 production hook。

- [ ] **Step 4（2 分钟）：Drift GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites$' \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/drift-green.log
  test "$(rg -c -F -- '--- PASS: TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites (' \
    /private/tmp/workbench-adaptive-mvp-p0c/drift-green.log)" = 1
  for p0c_case in Plan PlanItems Decision Verification EvidenceHighWatermark TerminalCheckpointReference; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/drift-green.log)" = 1
  done
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed' \
    /private/tmp/workbench-adaptive-mvp-p0c/drift-green.log; then
    false
  else
    test "$?" = 1
  fi
  ```

### Task 6：degraded projection 与 committed replay

- [ ] **Step 1（5 分钟）：添加两个顶层行为测试。**

  添加 0.7 的 degraded projection 与 exact committed replay tests。late rollback 与 cancel 已在 Task 4
  先 RED→GREEN，不得在此补测倒置 TDD。degraded
  case 必须证明只有 durable degraded 可写 base authority；healthy + invalid Journal request 必须 rollback，不能动态
  降级。两分支都核 Verification/Completion 的权威 tuple、sequence 和 terminal Attempt。replay case 必须由新
  repository instance 调 public finalizer，并在任何 transient fence 前命中。Task 5 的 tuple-hit mismatch loader
  可能已经自然让 exact replay PASS，因此本节 RED 必须由明确尚未实现的 durably-degraded success 驱动；replay 顶层
  允许恰好一次 PASS 或 FAIL、但不得 SKIP，若 FAIL 才在 Step 3 补齐。

- [ ] **Step 2（2 分钟）：运行 Terminal RED。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  set +e
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^(TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites)$' \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/terminal-red.log
  p0c_terminal_status=$?
  set -e
  test "$p0c_terminal_status" -ne 0
  test "$(rg -c -F -- '--- FAIL: TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection (' \
    /private/tmp/workbench-adaptive-mvp-p0c/terminal-red.log)" = 1
  p0c_replay_red_passes="$(rg -c --include-zero -F -- \
    '--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites (' \
    /private/tmp/workbench-adaptive-mvp-p0c/terminal-red.log || true)"
  p0c_replay_red_failures="$(rg -c --include-zero -F -- \
    '--- FAIL: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites (' \
    /private/tmp/workbench-adaptive-mvp-p0c/terminal-red.log || true)"
  test "$((p0c_replay_red_passes + p0c_replay_red_failures))" = 1
  if rg -n -- '--- SKIP:|panic:|build failed|no tests to run' \
    /private/tmp/workbench-adaptive-mvp-p0c/terminal-red.log; then
    false
  else
    test "$?" = 1
  fi
  ```

- [ ] **Step 3（5 分钟）：最小修复并逐项 GREEN。**

  只修 projection degraded 分支与 committed-result current-read；
  禁止修改 cancellation method或新增 terminal API。

- [ ] **Step 4（2 分钟）：Terminal GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  go test -p 1 ./domain/agentthread/repository -count=1 -v \
    -run '^(TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites)$' \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/terminal-green.log
  for p0c_test in \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites; do
    test "$(rg -c -F -- "--- PASS: ${p0c_test} (" /private/tmp/workbench-adaptive-mvp-p0c/terminal-green.log)" = 1
  done
  for p0c_case in \
    InitialOutboxRetryWithoutOutbox \
    InitialWithoutOutboxRetryWithOutbox \
    InitialOutboxRetryMissingDurableRow \
    MessageIDOrBody \
    SelectedCheckpointIDOrBody \
    UnselectedCheckpointIDOrBody \
    ExpectedOrUpdatedTitle \
    UnpersistedTitleEventIDOrBody \
    OutboxImmutableRequestIdentity \
    RunPostImage \
    AttemptPostImage; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/terminal-green.log)" = 1
  done
  for p0c_case in DurablyDegradedWritesAuthoritativeBaseTuples HealthyRejectsInvalidJournalProjectionWithoutWrites; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/terminal-green.log)" = 1
  done
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed' \
    /private/tmp/workbench-adaptive-mvp-p0c/terminal-green.log; then
    false
  else
    test "$?" = 1
  fi
  ```

---

## 6. Authority docs、final verification 与 commit

### Task 7：同步执行链，但不制造 production edge

- [ ] **Step 1（4 分钟）：更新 chain。**

  把 P0A/P0B 小节扩为 P0A/P0B/P0C：写清 P0C 仍是 unwired repository primitive、唯一
  `FinalizeRunSuccess` optional gate、root→Execution→Thread→Attempt lock、Verification→Completion single transaction、gate-off nil
  unchanged、P0D/P2 owner。不得宣称 application/ADK caller 已存在。

- [ ] **Step 2（4 分钟）：只更新 unwired exclusion。**

  `workbench-execution-graph.json` 只更新 `exclude.unwired_adaptive_boundary` reason/evidence/term（如必要）；
  nodes/edges/chains/count 不变，不新增 finalizer production edge。

- [ ] **Step 3（3 分钟）：更新结构 digest。**

  先保留旧 digest 跑 verify，唯一允许的 RED 是
  `canonical_profile_structure_mismatch ... actual <64hex>`；只把 actual 写入
  `WORKBENCH_PROFILE_STRUCTURE_DIGEST`，不放宽 validator。

- [ ] **Step 4（5 分钟）：串行 graph gates。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  node --test scripts/workbench-execution-graph.test.mjs \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-contract.log
  grep -Fx '# fail 0' /private/tmp/workbench-adaptive-mvp-p0c/graph-contract.log
  grep -Fx '# skipped 0' /private/tmp/workbench-adaptive-mvp-p0c/graph-contract.log
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-verify.log
  node scripts/workbench-execution-graph.mjs build \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-build.log
  node scripts/workbench-execution-graph.mjs verify-derived \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-derived.log
  ```

  `build` 沙箱内若且仅若 `graphify_ast_empty`，按 runbook 以同一 exact Node command 请求沙箱外重跑；
  不并发运行第二个 graph build。

### Task 8：fresh exact suite 与双审

- [ ] **Step 1（4 分钟）：运行 exact 10 tests。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  p0c_regex='^(TestValidateAdaptiveVerifiedSuccessGate|TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites|TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables|TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt|TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites|TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome|TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection|TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites)$'
  go test -p 1 ./domain/agentthread/repository -count=1 -timeout=180s -v -run "$p0c_regex" \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log
  for p0c_test in \
    TestValidateAdaptiveVerifiedSuccessGate \
    TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites \
    TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables \
    TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt \
    TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites \
    TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection \
    TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites; do
    test "$(rg -c -F -- "--- PASS: ${p0c_test} (" /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  for p0c_case in Plan PlanItems Decision Verification EvidenceHighWatermark TerminalCheckpointReference; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  for p0c_case in DistinctJournalAndExecutionRuns SharedJournalAndExecutionRun; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  for p0c_case in CancelFirst SuccessFirst; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  test "$(rg -c -F -- '--- PASS: TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically/AllCurrentPlanItemsFingerprintRejectsUnreferencedDrift (' \
    /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  test "$(rg -c -F -- '--- PASS: TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically/ReservedHighWatermarkGapRemainsValid (' \
    /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  for p0c_case in \
    InitialOutboxRetryWithoutOutbox \
    InitialWithoutOutboxRetryWithOutbox \
    InitialOutboxRetryMissingDurableRow \
    MessageIDOrBody \
    SelectedCheckpointIDOrBody \
    UnselectedCheckpointIDOrBody \
    ExpectedOrUpdatedTitle \
    UnpersistedTitleEventIDOrBody \
    OutboxImmutableRequestIdentity \
    RunPostImage \
    AttemptPostImage; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  for p0c_case in WithoutJournalEventDoesNotRequireAdaptiveTables WithJournalEventPreservesLegacyAttemptProjection; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  for p0c_case in DurablyDegradedWritesAuthoritativeBaseTuples HealthyRejectsInvalidJournalProjectionWithoutWrites; do
    test "$(rg -c -F -- "--- PASS: TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection/${p0c_case} (" \
      /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log)" = 1
  done
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed' \
    /private/tmp/workbench-adaptive-mvp-p0c/exact-green.log; then
    false
  else
    test "$?" = 1
  fi
  ```

- [ ] **Step 2（4 分钟）：package、compile、format、static。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0c-go-cache
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    go test -p 1 ./domain/agentthread/repository -count=1 -timeout=240s \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/package-green.log
  p0c_legacy_regex='^(TestThreadRepositoryFinalizeRunSuccessCommitsMessageTitleAndStatusTogether|TestThreadRepositoryFinalizeRunSuccessCommitsTerminalCheckpointTogether|TestThreadRepositoryFinalizeRunSuccessRollsBackWhenTerminalCheckpointFails|TestThreadRepositoryFinalizeRunSuccessPreservesConcurrentlyChangedTitle|TestThreadRepositoryFinalizeRunSuccessRejectsCancelAndRollsBackWriteFailure|TestThreadRepositoryRequestRunCancellationInvalidatesGenerationAndWritesOneEvent|TestThreadRepositoryRequestRunCancellationCancelsPendingRun|TestThreadRepositoryRequestRunCancellationRejectsMismatchedEventThread|TestThreadRepositoryRequestRunCancellationAppendsOutboxOnlyOnceOnReplay|TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload)$'
  go test -p 1 ./domain/agentthread/repository -count=1 -v -run "$p0c_legacy_regex" \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/legacy-green.log
  for p0c_test in \
    TestThreadRepositoryFinalizeRunSuccessCommitsMessageTitleAndStatusTogether \
    TestThreadRepositoryFinalizeRunSuccessCommitsTerminalCheckpointTogether \
    TestThreadRepositoryFinalizeRunSuccessRollsBackWhenTerminalCheckpointFails \
    TestThreadRepositoryFinalizeRunSuccessPreservesConcurrentlyChangedTitle \
    TestThreadRepositoryFinalizeRunSuccessRejectsCancelAndRollsBackWriteFailure \
    TestThreadRepositoryRequestRunCancellationInvalidatesGenerationAndWritesOneEvent \
    TestThreadRepositoryRequestRunCancellationCancelsPendingRun \
    TestThreadRepositoryRequestRunCancellationRejectsMismatchedEventThread \
    TestThreadRepositoryRequestRunCancellationAppendsOutboxOnlyOnceOnReplay \
    TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload; do
    test "$(rg -c -F -- "--- PASS: ${p0c_test} (" /private/tmp/workbench-adaptive-mvp-p0c/legacy-green.log)" = 1
  done
  if rg -n -- '--- (FAIL|SKIP):|panic:|build failed' \
    /private/tmp/workbench-adaptive-mvp-p0c/legacy-green.log; then
    false
  else
    test "$?" = 1
  fi
  go test -p 1 ./domain/agentthread/repository -run '^$' -count=1 \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/compile.log
  gofmt -d \
    domain/agentthread/repository/adaptive_execution.go \
    domain/agentthread/repository/mysql_adaptive_execution.go \
    domain/agentthread/repository/mysql_adaptive_execution_test.go \
    domain/agentthread/repository/repository.go \
    domain/agentthread/repository/mysql.go \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/gofmt.log
  test ! -s /private/tmp/workbench-adaptive-mvp-p0c/gofmt.log
  cd ..
  git diff --check
  git grep -h -E '^func \(r \*threadRepository\) [A-Za-z0-9]*Finalize[A-Za-z0-9]*\(' \
    "$(cat /private/tmp/workbench-adaptive-mvp-p0c/base.sha)" -- \
    backend/domain/agentthread/repository \
    | LC_ALL=C sort \
    > /private/tmp/workbench-adaptive-mvp-p0c/finalizers-base.txt
  rg --no-filename '^func \(r \*threadRepository\) [A-Za-z0-9]*Finalize[A-Za-z0-9]*\(' \
    backend/domain/agentthread/repository --glob '*.go' \
    | LC_ALL=C sort \
    > /private/tmp/workbench-adaptive-mvp-p0c/finalizers-head.txt
  diff -u \
    /private/tmp/workbench-adaptive-mvp-p0c/finalizers-base.txt \
    /private/tmp/workbench-adaptive-mvp-p0c/finalizers-head.txt \
    > /private/tmp/workbench-adaptive-mvp-p0c/finalizer-diff.log
  diff -u <(printf '%s\n' \
    'func (r *threadRepository) FinalizeJournalAttempt(' \
    'func (r *threadRepository) FinalizeRunSuccess(') \
    /private/tmp/workbench-adaptive-mvp-p0c/finalizers-head.txt
  if rg -n 'AdaptiveGate|AdaptiveVerifiedSuccessGate' \
    backend/application backend/api frontend \
    > /private/tmp/workbench-adaptive-mvp-p0c/production-wiring-scan.log; then
    false
  else
    test "$?" = 1
  fi
  if rg -n '\.CommitAdaptiveExecutionBoundary\(' \
    backend/domain/agentthread/repository --glob '*.go' --glob '!**/*_test.go'; then
    false
  else
    test "$?" = 1
  fi
  ```

- [ ] **Step 3（3 分钟）：exact-eight status gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git status --porcelain=v1 | LC_ALL=C sort \
    | tee /private/tmp/workbench-adaptive-mvp-p0c/status.txt
  diff -u <(printf '%s\n' \
    ' M backend/domain/agentthread/repository/adaptive_execution.go' \
    ' M backend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    ' M backend/domain/agentthread/repository/mysql_adaptive_execution_test.go' \
    ' M backend/domain/agentthread/repository/mysql.go' \
    ' M backend/domain/agentthread/repository/repository.go' \
    ' M docs/superpowers/context/workbench-execution-chain.md' \
    ' M docs/superpowers/context/workbench-execution-graph.json' \
    ' M scripts/workbench-execution-graph/contract.mjs' | LC_ALL=C sort) \
    /private/tmp/workbench-adaptive-mvp-p0c/status.txt
  test -z "$(git diff --cached --name-only)"
  test -z "$(git diff --diff-filter=D --name-only)"
  shasum -a 256 \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/repository.go \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs \
    > /private/tmp/workbench-adaptive-mvp-p0c/implementation-files.sha256
  ```

- [ ] **Step 4（5 分钟）：独立 spec + quality 双审。**

  Spec reviewer 逐条核 0.1～0.7；quality reviewer fresh 跑 exact/package/Node verify，专门反例检查
  TOCTOU、Decision stale、Evidence HWM、全部 PlanItems fingerprint、degraded projection、cancel linearization、
  root lock cycle、nil gate。
  任一 P0/P1 必须先 RED→修复→全量重验；P2 写入 `remaining-risk.txt`。两名 reviewer 分别把绑定
  `base.sha`、当前八文件 SHA、fresh commands 与 P0/P1/P2 计数写入
  `/private/tmp/workbench-adaptive-mvp-p0c/spec-review.txt` 与 `quality-review.txt`；不得只留聊天结论。

### Task 9：implementation commit 与 post-commit evidence

- [ ] **Step 1（3 分钟）：stage + cached gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  shasum -a 256 -c /private/tmp/workbench-adaptive-mvp-p0c/implementation-files.sha256
  git add \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/repository.go \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs
  git diff --cached --check
  test -z "$(git diff --cached --diff-filter=D --name-only)"
  test "$(git diff --cached --name-only | wc -l | tr -d ' ')" = 8
  ```

- [ ] **Step 2（2 分钟）：提交，禁止 `--no-verify`。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  p0c_parser_package="$(find common/temp/install-run \
    -path '*/node_modules/@babel/parser/package.json' -print -quit)"
  test -n "$p0c_parser_package"
  p0c_node_modules="${p0c_parser_package%/@babel/parser/package.json}"
  NODE_PATH="$p0c_node_modules" git commit -m 'feat: gate adaptive verified success atomically'
  test "$(git show -s --format=%s HEAD)" = 'feat: gate adaptive verified success atomically'
  test "$(git rev-parse HEAD^)" = "$(cat /private/tmp/workbench-adaptive-mvp-p0c/base.sha)"
  shasum -a 256 -c /private/tmp/workbench-adaptive-mvp-p0c/implementation-files.sha256
  git rev-parse HEAD | tee /private/tmp/workbench-adaptive-mvp-p0c/head.sha
  ```

- [ ] **Step 3（3 分钟）：post-commit exact-eight 与 graph。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  diff -u <(printf '%s\n' \
    $'M\tbackend/domain/agentthread/repository/adaptive_execution.go' \
    $'M\tbackend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    $'M\tbackend/domain/agentthread/repository/mysql_adaptive_execution_test.go' \
    $'M\tbackend/domain/agentthread/repository/mysql.go' \
    $'M\tbackend/domain/agentthread/repository/repository.go' \
    $'M\tdocs/superpowers/context/workbench-execution-chain.md' \
    $'M\tdocs/superpowers/context/workbench-execution-graph.json' \
    $'M\tscripts/workbench-execution-graph/contract.mjs' | LC_ALL=C sort) \
    <(git diff-tree --no-commit-id --name-status -r HEAD | LC_ALL=C sort)
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-postcommit-verify.log
  node scripts/workbench-execution-graph.mjs build \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-postcommit-build.log
  node scripts/workbench-execution-graph.mjs verify-derived \
    2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0c/graph-postcommit-derived.log
  test -z "$(git status --porcelain)"
  ```

  post-commit `build` 若且仅若沙箱返回 `graphify_ast_empty`，沿用 Task 7 的 runbook：停止所有并发 graph
  命令，以同一 exact Node command 请求沙箱外单 session 重跑，并用成功输出覆盖
  `graph-postcommit-build.log`，再 fresh `verify-derived`。不得保留失败日志冒充成功证据。

- [ ] **Step 4（4 分钟）：冻结 evidence manifest。**

  ```bash
  set -euo pipefail
  p0c_evidence=/private/tmp/workbench-adaptive-mvp-p0c
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  p0c_write_state() {
    printf '%s\n' \
      'PACKET=P0C' \
      "STATUS=$1" \
      "P0C_BASE=$(cat "$p0c_evidence/base.sha")" \
      "P0C_HEAD=$(cat "$p0c_evidence/head.sha")" \
      > "$p0c_evidence/state.env"
  }
  p0c_write_state active
  trap 'p0c_write_state active' EXIT
  git diff-tree --no-commit-id --name-status -r HEAD \
    | LC_ALL=C sort > "$p0c_evidence/allowlist.txt"
  shasum -a 256 /private/tmp/workbench-adaptive-mvp-p0b/evidence.sha256 \
    > "$p0c_evidence/p0b-evidence.sha256"
  p0c_review_risks="$(cat "$p0c_evidence/remaining-risk.txt")"
  printf '%s\n' \
    'P0C proves SQLite transaction semantics and its local root→Execution→Thread→Attempt order; P0D owns real MySQL cancel/lease/crash and cross-API Run↔Thread races.' \
    'P0C remains an unwired repository gate; P2 owns the full VerificationResult codec, registry, producer and application wiring.' \
    'Gate-off FinalizeRunSuccess remains authoritative for baseline Runs and never fabricates adaptive verification.' \
    'P1D only wires admission/decision; P2 must route adaptive top-level success through this gate, while existing child-run CompleteRun remains gate-off.' \
    'P0C proves only Plan-bearing verified success; before routing direct/no-Plan adaptive runs, P2 must add and test the explicit nullable-Plan authority branch.' \
    'Existing recovery admission/DeleteThreadIfIdle use Thread→Run while FinalizeRunSuccess uses Run→Thread; P0C is unwired, but P0D must reproduce/fix both barrier races and P2 wiring is forbidden until they pass without 1213/1205.' \
    'P0B packet subject is grandfathered from its packet-specific plan; P0D validates each packet exact frozen subject rather than rewriting history.' \
    'Zero-migration authority has no historical Plan snapshot; later Plan or Item drift fails closed instead of reconstructing old state.' \
    > "$p0c_evidence/remaining-risk.txt"
  if [ -n "$p0c_review_risks" ]; then
    printf '%s\n' "$p0c_review_risks" >> "$p0c_evidence/remaining-risk.txt"
  fi
  printf '%s\n' \
    allowlist.txt \
    atomic-green.log \
    atomic-red.log \
    base.sha \
    baseline.log \
    compile.log \
    contract-green.log \
    contract-red.log \
    drift-green.log \
    drift-red.log \
    evidence-files.txt \
    exact-green.log \
    finalizer-diff.log \
    finalizers-base.txt \
    finalizers-head.txt \
    gofmt.log \
    graph-build.log \
    graph-contract.log \
    graph-derived.log \
    graph-postcommit-build.log \
    graph-postcommit-derived.log \
    graph-postcommit-verify.log \
    graph-verify.log \
    head.sha \
    implementation-files.sha256 \
    legacy-green.log \
    p0b-evidence.sha256 \
    package-green.log \
    plan.sha256 \
    production-wiring-scan.log \
    quality-review.txt \
    remaining-risk.txt \
    spec-review.txt \
    state.env \
    status.txt \
    terminal-green.log \
    terminal-red.log \
    > "$p0c_evidence/evidence-files.txt"
  cd /private/tmp/workbench-adaptive-mvp-p0c
  diff -u evidence-files.txt \
    <(find . -maxdepth 1 -type f ! -name evidence.sha256 -print | sed 's#^\./##' | LC_ALL=C sort)
  p0c_write_state PASS
  xargs shasum -a 256 < evidence-files.txt > evidence.sha256
  shasum -a 256 -c evidence.sha256
  trap - EXIT
  ```

---

## 7. Exit gate 与 P0D handoff

P0C 只有同时满足以下条件才写 `STATUS=PASS`：

- exact 10 顶层 tests 与逐名 legacy focused tests 全部 fresh PASS、零 SKIP/fail；repository package
  （显式 unset MySQL gate，允许既有真实 MySQL integration tests 按约定 skip）、compile、format、diff、Node
  contract、graph verify/build/derived 全部 fresh PASS；
- Verification 与 Completion 在同一 finalizer transaction，sequence 与 ID 均有序；
- 任一 drift/write failure 不留下 Verification/Completion/Message/Checkpoint/Run/Attempt 部分状态；
- cancel-first 与 success-first 各只有一个 terminal；
- nil gate 不新增 adaptive DB 依赖；无 JournalEvent 子例在无 Attempt/Plan 表时通过，带 JournalEvent 子例保留既有
  Attempt projection，且现有 finalizer tests 全通过；
- 只有一个 repository `FinalizeRunSuccess`，没有 application/ADK wiring、migration、第二 finalizer；
- docs-only P0C packet 与 implementation commit 形成线性 parent/child，implementation exact-eight；
- evidence manifest fresh checksum PASS，最终工作树 clean。

P0C PASS 后才从真实 P0C HEAD 创建：

`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md`

P0D docs subject 冻结为 `docs: add adaptive execution P0D packet`。P0D 必须用 disposable MySQL、
`go test -json` 连续两轮机器断言四个子测试：concurrent PlanItem mutation、lease takeover、cancel vs
verified success、crash-after-commit retry；不得把 P0C SQLite cancel matrix 冒充最终竞态证据。

其中 lease-takeover 顶层必须用 channel barrier 逐名覆盖
`RecoveryAdmissionVsVerifiedFinalizer`（`CreateRunBundle` recovery 的 Thread→Run）与
`DeleteThreadIfIdleVsVerifiedFinalizer`（Thread→active Runs）两个 cross-API 子例，禁止 1213/1205/deadline；旧序若
真实 RED，P0D 必须先统一全局 aggregate lock order 或实现仅针对已识别 deadlock 的有界 whole-transaction retry，
不能靠 skip/降低并发通过。P2 application wiring 硬依赖这两个子例连续两轮 PASS。crash-after-commit 必须由 helper
子进程在 DB commit 后、响应返回前强制退出，再由新进程/new repository replay；单进程普通 lost-response retry
不能冒充 crash 证据。

P0D entry 必须先修真实 MySQL fixture 的物理闭包：seed logical Journal root Run 30；migration loader 加入
`20260614000100_agent_thread_messages.sql`；cleanup 在 Runs 前清 `agent_thread_messages`。这些只属于 P0D 的
`mysql_adaptive_execution_integration_test.go`，不得反向扩张 P0C exact-eight。
