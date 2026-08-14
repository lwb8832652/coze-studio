# Workbench Adaptive Execution P1L Thread 删除栅栏设计

## 状态

本设计以已通过全部 P0D 门禁的提交
`d32a8ab1a61f9b1cb3fc4e9ddefd7bb2f56bcefe` 为基线。用户已批准优先处理
repository lock-order blocker；本文件只冻结设计，不授权修改生产代码、迁移、API、IDL、
前端或生产接线。

P0D 已经关闭 `FinalizeRunSuccess` gate-on/gate-off、active-source recovery、
`RequestRunCancellation` 与 `DeleteThreadIfIdle` active-rejection 分支的逐名竞态，但明确没有
证明真正进入删除 cascade 时的全仓安全性。P1L 只解决这一 cross-packet P1。recovery 后同一
Attempt 的第二个 Plan-bearing boundary 仍由后续 P1R 处理，不能在本包中顺便放宽 authority。

推荐依赖顺序保持：

```text
P1L PASS → P1M → P1R PASS → P1D → P2
```

## 问题

`DeleteThread` 与 `DeleteThreadIfIdle` 在允许删除时都会先锁 Thread，再执行
`deleteThreadCascade`。cascade 按外键式 child-first 顺序修改 Snapshot、PlanItem、Plan、
Checkpoint、RunEvent、SideEffectLedger、Attempt、Message、Run 等行，最后删除 Thread。

许多既有写事务没有先取得 Thread 行锁：

- `CreateJournalAttempt` 先锁 logical root Run、distinct execution Run，再插入 Attempt；
- side-effect 与 generic boundary 先锁 Attempt，再锁 Ledger、Event、Checkpoint；
- adaptive boundary 首写和 exact replay 先锁 root/execution Run、Attempt 与 event tuple；
- cancel、lease reconcile 和 terminal `UpdateRunStatus` 先锁 Run，再锁 Attempt/Event；
- direct Plan mutation 先锁 Plan，再锁 PlanItem；
- Snapshot retention claim/complete 在 Snapshot 与 Fragment 之间取锁；
- Message、Event、Checkpoint、Memory、File、Artifact、Token 等单语句写入通常使用
  autocommit，也没有 deletion fence。

这产生两类问题：

1. **死锁**：删除的 child-first 顺序与业务事务的 parent/authority-first 顺序构成反向边；
2. **晚到孤儿**：单表写可能在 cascade 已扫过该表、但 Thread 尚未删除时提交一条新子记录。

仅重排 cascade 无法闭环。logical root 与 execution Run 的语义顺序不等于数值 ID 顺序，
generic boundary、adaptive boundary、Snapshot cleanup 的内部顺序也互不相同。仅增加 1213 重试
同样不能建立删除线性化点，也不能证明没有晚到孤儿。

## 目标

- 为现有 Thread 聚合定义一个所有普通持久化写入都遵守的删除线性化栅栏，并为删除后仍需清理的
  Snapshot/Reservation 冻结窄化 continuation；
- 保留同一 Thread 内正常业务写之间的并行能力；
- 让两个公开删除入口与 Run、Attempt、Journal、adaptive、Plan、Snapshot 及单表子记录写入
  只有可证明的先后关系；
- 保留每个公共方法现有的幂等、replay、typed error、lease fence、authority 与 durable post-image；
- 使用真实 MySQL `performance_schema.data_lock_waits` 保存旧实现 RED 和新实现 GREEN；
- 不依赖 sleep、调度概率、advisory lock、忽略 1213/1205 或无界事务重试。

## 非目标

- 不接线 `AdaptiveGate`，不修改 application、ADK、API、IDL 或前端；
- 不处理 same-recovery-Attempt rolling authority；
- 不新增数据库迁移、Thread 删除状态机、异步 GC 或专用 lock 表；
- 不改变 delete API 的返回合同、cascade 业务顺序或对象存储清理协议；
- 不宣称不同 Thread 之间完全串行，也不把普通只读查询纳入栅栏；
- 不在本包中重写所有细粒度锁的内部顺序。

## 方案比较

### 方案一：Thread 行共享/独占栅栏（采用）

普通 Thread-scoped mutation 在取得任何被 cascade 管理的行锁之前，先对权威 Thread 行执行
`FOR SHARE`；两个删除入口继续以 `FOR UPDATE` 作为第一把锁。多个正常写者可以同时持有共享锁，
删除者必须等待全部写者提交；删除持有独占锁时，新写者必须等待，并在删除提交后因 Thread 不存在
而 fail closed。

该方案不需要迁移，复用已经存在且生命周期准确的 `agent_threads` 行。它给删除提供唯一线性化点，
同时不把同 Thread 的 Journal/Event/Plan 正常写完全串行化。

### 方案二：专用 per-Thread fence 表

新增 `agent_thread_transaction_locks(thread_id PRIMARY KEY)`，普通写取共享锁、删除取独占锁。
并发语义与方案一相同，却增加 migration、fence 行创建/清理、孤儿 fence 和批处理排序，不能证明比
现有 Thread 行更安全，因此不采用。

### 方案三：全仓细粒度 canonical rank

统一重排 Run、Attempt、Ledger、Event、Checkpoint、Plan、Item、Snapshot、Fragment 等锁等级。
该方案理论并发度最高，但会重写 generic/adaptive/recovery 协议及写序；logical root 与 execution
Run 还需要语义排序，不能用 ID 排序代替。改动和回归面远超当前 blocker，因此不采用。

## 栅栏协议

### 1. 权威 Thread 发现

请求已有 RunID、AttemptID、SnapshotID、Plan scope 或其他 durable parent identity 时，必须先从
该 parent 取得服务端权威 `ThreadID`。这次 discovery 必须在 mutation transaction **之外**，使用
autocommit 普通读完成。禁止在默认 InnoDB RR 的 mutation transaction 内先做 plain discovery：
那会提前建立 consistent-read snapshot，使 exact replay 后续的 Event、Checkpoint、Plan 或 Items
普通读取可能看到取得 Thread fence 之前的旧视图。

新建记录若只有 Thread 本身是 durable parent，可以直接以请求的 ThreadID 查找并锁定 Thread；锁定
行及其服务端 tenant/space 才是 authority。若请求同时带 Run、Attempt 或其他已存在 parent，则必须
从该 durable parent 发现 ThreadID，不能先锁另一个 caller-selected Thread。

发现后启动一个新的 mutation transaction：

```text
autocommit plain durable discovery（如需要）
→ BEGIN mutation transaction
→ agent_threads(id = authoritative ThreadID) FOR SHARE（事务内第一次 managed row access）
→ current-lock 原目标行
→ 重验 ThreadID、tenant、logical root/execution/Attempt identity
→ 原有业务 fence、replay 与写入
```

若 discovery 后删除先获得独占锁并提交，`FOR SHARE` 必须得到 not-found 并返回该公共方法已有的
not-found/conflict cause。若 parent 在 Thread 锁前后发生替换或归属漂移，current-lock 重验必须
fail closed。原始数据库故障仍原样透传；不得把任意 DB error 包成业务冲突。

错误映射按方法逐项冻结：已有 `ErrPlanNotFound`、`ErrJournalNotEnrolled`、lease/conflict 等 domain
cause 的路径继续保留该 cause；目前没有 missing-parent domain cause 的裸 child create，Thread
missing 必须返回一个 `errors.Is(err, gorm.ErrRecordNotFound)` 为真的上下文错误，不在 P1L 新增公共
sentinel。删除入口和 post-delete cleanup 继续保留各自现有的 nil/not-found 与 claim-lost 合同。

已有 `Thread FOR UPDATE` 的 admission、title CAS、public-state、Snapshot create/reserve 和 P0D
finalizer 路径保留更强锁，不降级为共享锁。

### 2. 普通写与删除

- 普通写：`Thread FOR SHARE` 是事务内第一把被管理的行锁；
- `DeleteThread`、`DeleteThreadIfIdle`：`Thread FOR UPDATE` 是事务内第一把锁，并贯穿完整
  cascade；
- inventory 必须为每个 mutation 标注 Thread lock mode。任何后续可能更新/删除 Thread 或需要升级
  为独占锁的路径，从第一步直接使用 `FOR UPDATE`；禁止先持 `FOR SHARE` 再升级，避免两个共享
  持有者互相等待升级；
- `CreateThreadBundle` 只有确认不存在 durable aggregate 的 true first-create 分支可以不预锁 Thread；
  Thread insert 必须是该 mutation transaction 的第一把 managed row lock。existing-idempotency replay
  和 lost-response fallback 必须从 durable Run/idempotency 事务外发现 Thread，再进入 Thread SHARE
  transaction current-lock 并重验完整 replay tuple；
- `CreateRunBundle` 的 first-write、existing replay 与 error fallback 都面对既有 Thread。它们必须在
  事务外 discovery 后先取得当前 admission 所需的 Thread UPDATE fence，再 current-lock/replay；禁止
  transaction 内先 `findExistingRunBundle` 建立旧 RR snapshot，或在 rollback 后用无栅栏 plain read
  直接返回 winner；
- 真正只读、不取写锁且不创建 durable child 的方法不需要栅栏；
- autocommit child mutation 必须改为短事务，在共享 Thread 锁后执行原单条写入；
- “只写一个 child 表”不能自动豁免，因为它仍可能在删除扫过后创建孤儿。

### 3. 批处理与 retention

批处理不得先锁 Snapshot、Run 或其他 child，再回头锁 Thread。需要跨多个 Thread 的 claim 按以下
方式处理：

1. 在 mutation transaction 外以稳定 cursor 非锁定 overfetch 候选及其权威 ThreadID；
2. 对去重后的 ThreadID 按升序执行 `FOR SHARE SKIP LOCKED`；已被删除独占锁阻塞的 Thread 本轮
   defer，不能让一个 Thread head-of-line 阻塞整批；
3. 对实际取得的 Thread 锁，在锁内重新执行原 eligibility、lease/token 与 tenant 条件；
4. 再使用既有 `SKIP LOCKED`/row fence 领取 child；
5. 跳过或竞争失败后，以有界 refill 继续扫描，直到达到 BatchSize、候选耗尽或命中冻结的扫描
   上限；只返回实际赢得 current fence 的 claims。

Thread locking read 因 SKIP LOCKED 返回空时不能被解释成 Thread 已删除；它也可能只是正在被删除者
锁住。只有 transaction 外 discovery 已观察到 Thread 不存在，并且 current child 满足下一节的
tombstone 条件，才能进入 post-delete continuation。实施计划必须冻结 overfetch/扫描上限与 cursor，
不能用无界循环维持 batch size。

若现有批处理无法在不改变公共语义的前提下遵守该协议，P1L 必须停止并回到设计评审，不能通过
先锁 child 后补锁 Thread 或放宽 post-state 断言过关。

### 4. 删除后 cleanup continuation

Journal Snapshot 与 staging Reservation 是明确例外：`deleteThreadCascade` 会 tombstone Snapshot、
清空敏感内容、缩短 Reservation expiry 并删除 Thread，但故意保留这些行，供对象存储清理器在
Thread 已不存在后继续 claim、complete 或 release。它们不能一律要求现存 Thread 行。

retention 必须把 live 与 post-delete 两条路径分开：

- Thread 仍存在：按上一节取得共享 Thread fence，在锁内重验 cleanup eligibility 与 current token；
- Thread 已不存在：只允许 cleanup-only 操作；Snapshot 必须 current-lock 后满足
  `deleted_at IS NOT NULL` 且 cleanup state 为 pending/failed/deleting，Reservation 必须 current-lock
  后确认其 Thread 已不存在、expiry/current claim token 与请求完全匹配；
- complete/release 必须依赖当前 claim token 和 current row lock，claim 被 cascade 清除后返回既有
  `ErrJournalRetentionClaimLost`；
- `DeleteExpiredJournalSnapshotReservations` 不使用 claim API，必须单独遵守双态合同：live Thread
  先取得共享 fence；Thread 已不存在时，只能 current-lock 已过期 residual Reservation，并确认
  cleanup claim token 为空后删除，不能抢走另一个清理 worker 已领取的行；
- post-delete continuation 只能清空或删除 tombstoned Snapshot/Fragment/Reservation，不能创建
  Event、Checkpoint、Attempt、Run 或任何普通 child，也不能作为其它 mutation 绕过 Thread fence
  的通用入口。

post-delete mutation transaction 的第一次 managed row access 仍是对原 ThreadID 的 current locking read
`FOR SHARE`。只有该 locking read 明确返回 not-found 后，才可 current-lock residual child；这次缺行
证明不能使用 `SKIP LOCKED`，否则“行不存在”和“行正被删除者锁住”无法区分。缺行 locking read
在 RR 下还必须阻止同 ID Thread 在 cleanup 提交前被重新插入。

若 Thread 删除正在进行但尚未提交，事务外 discovery 仍会看到旧 Thread；随后的共享锁必须等待
删除独占锁。删除提交后，共享锁得到 not-found，retention 才能重新按 current tombstone 进入
post-delete continuation。禁止在同一 RR transaction 内用旧 snapshot 猜测 Thread 已删除。

### 5. 内部锁序

Thread 栅栏解决同一聚合的删除反序；栅栏之后保留既有、已验证的语义顺序：

```text
Thread fence
→ logical Journal root Runs
→ distinct execution Runs（root 行复用）
→ Attempts
→ ledger / exact event tuple
→ lineage checkpoint / event
→ Plan scope Run → Plan → PlanItems
→ 其余 child rows与outbox
```

路径不使用的阶段直接省略；该列表不要求 generic boundary 为了形式统一额外锁无关 Run，也不要求
普通写者之间通过独占 Thread 锁完全串行。P1L 的共同前缀是 deletion fence，后续锁仍由各公共合同
决定。

同一事务需要多个 Thread 时按 ThreadID 升序取锁。logical root 必须先于 distinct execution Run，
不能改成纯 RunID 排序。所有 plain discovery 都必须在 current-lock 后重验；plain read 不是 authority
或事务 fence。

### 6. replay 与回调

首写、exact replay、terminal replay 和 lost-response retry 必须走相同 Thread 栅栏，不能让 replay
绕过删除线性化。事务内 outbox callback 保持原位置；栅栏前不得调用外部副作用、生成不可重放随机
结果或提前返回成功。

inventory 的最小单位是“public method × first-write/exact replay/lost-response fallback/terminal 或
post-delete branch”，不是只列函数名。任何 error fallback 读取 committed winner 时，都必须复用同一
fenced replay helper；不得因主事务已经 rollback 就退回无锁读。

## 覆盖面与文件边界

实施计划首先生成机器可复核的 public mutation inventory。已知需要复核或修改的生产文件精确为：

- `backend/domain/agentthread/repository/mysql.go`
- `backend/domain/agentthread/repository/mysql_canonical.go`
- `backend/domain/agentthread/repository/mysql_journal.go`
- `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
- `backend/domain/agentthread/repository/mysql_journal_snapshot.go`
- `backend/domain/agentthread/repository/mysql_journal_retention.go`
- `backend/domain/agentthread/repository/memory_audit.go`

inventory 必须从 `deleteThreadCascade` 的每一个 PO/table 反向穷举所有生产 mutator，不能只依赖
函数名或显式 `Transaction` 搜索。当前已知覆盖集合精确包括：

- direct Thread-row mutation：`CreateThread` true-create、`UpdateThreadTitle`、
  `UpdateThreadMetadata`、`PatchThread` 与 `UpdatePublicThreadState`；它们以 Thread INSERT/DML 或
  既有 `FOR UPDATE` 直接取得首个独占锁，不额外先取 SHARE，但必须进入 inventory；
- Run lifecycle、cancel、reconcile、terminal update 与两个 delete 入口；
- `CreateThreadBundle`、`CreateRunBundle` 的 first-write、existing replay 与 lost-response fallback；
- Journal Attempt、side-effect prepare/transition/resolve、generic boundary 首写与 replay、event append、
  projection、finalize；
- adaptive boundary 首写、recovery lineage 与 exact replay；
- direct Plan/PlanItem mutation；
- Snapshot reserve/create/retention claim/complete；
- Snapshot/Reservation claim、complete、release、expired reservation cleanup，以及 expired
  Checkpoint、Event/Ledger、unreferenced Attempt retention；
- pending/queued Run、Memory/Artifact worker claim 与 completion 路径；
- Message、Event、Checkpoint、Memory、MemoryAudit、File、Artifact、Token 等 cascade-managed
  单表 mutation，其中 `CreateMemoryAuditEvent` 位于 `memory_audit.go`。

候选测试文件为一个新建的聚焦真实 MySQL 矩阵文件，加上需要更新的既有 repository 测试。长期
authority 只允许更新：

- `docs/superpowers/context/workbench-execution-chain.md`
- `docs/superpowers/context/workbench-execution-graph.json`
- `scripts/workbench-execution-graph/contract.mjs`

不得修改 repository interface、entity、application/ADK、API/IDL、前端或 migration。若完整
inventory 证明生产修复必须越过上述六个 MySQL 文件与 `memory_audit.go`，实施包立即 FAIL，并先
修订本设计。guardrail/MCP audit 等未被 `deleteThreadCascade` 管理的独立数据域必须在 inventory
中显式列为非目标及其现有删除语义，不能误算为已由本栅栏清理。

## 真实 MySQL 证据

测试只能使用专用 disposable MySQL，禁止连接 dev 数据库。每个 leaf 使用独立 schema lifecycle、
至少两个 participant connection 与一个独立 observer connection；确定性旧锁环另用第四个 external
blocker/orchestrator connection。所有参与连接的 `CONNECTION_ID()` 必须互异。所有等待均由
`performance_schema.data_locks/data_lock_waits` 精确绑定 requester、blocker、schema、table、index
与 lock data，禁止 `time.Sleep`、调度猜测或仅凭超时判断。

### 1. Thread-prefix contract RED/GREEN

冻结 inventory 中每个 mutation 都必须有 table-driven/unit query-order 断言，证明 mutation
transaction 的第一把 managed row lock 是 Thread fence，或精确命中 post-delete cleanup 例外。

真实 MySQL prefix 证据分成两类：

- blocking mutation：先持目标 `agent_threads/PRIMARY` 独占锁，再启动公共 mutation。旧实现证明
  没有首先等待 Thread；新实现证明 mutation transaction 的第一次 managed row access 在该 Thread
  行等待，
  且 discovery 在另一个 autocommit read 中完成；
- batch claim/retention：持有一个 Thread 独占锁后，`FOR SHARE SKIP LOCKED` 必须 defer 该 Thread，
  不得触碰它的 child，且仍能处理其它 Thread。此类路径不得伪造 `data_lock_waits` waiter。

blocking 代表叶包括：

- `CreateJournalAttempt`；
- `CreateThreadBundle` existing/lost-response replay 与 `CreateRunBundle` existing/lost-response replay；
- `CommitExecutionBoundary` exact replay；
- `CommitAdaptiveExecutionBoundary` exact replay；
- `RequestRunCancellation`、`ReconcileExpiredRunLease`、terminal `UpdateRunStatus`；
- `AppendJournalEvent`、direct Plan mutation；
- live-Thread Snapshot/Reservation complete、release；
- Message、Checkpoint、Token、File/Artifact、Memory 单表代表。

skip 代表叶包括 pending/queued Run、Memory/Artifact worker claim、Snapshot/Reservation claim 与
execution retention batch。每个 leaf 都要断言被锁 Thread 的 child 零触碰、其它 Thread 正常领取、
结果无重复，并记录实际 Thread/child lock 对象。

post-delete Snapshot/Reservation continuation 单独测试：Thread 已提交删除、tombstone 仍在时，合法
claim/complete/release 必须继续成功；缺 tombstone、错误 state/token 或仍会创建普通 child 的请求
必须 fail closed。`DeleteExpiredJournalSnapshotReservations` 还必须分别覆盖 live Thread、missing
Thread residual、非空 claim token 与正在删除 Thread 的 defer。它们不能计入“首先等待 Thread”的
单一路径 prefix 集合。

批处理另设双 worker、多 Thread 叶：两个 worker 可发现重叠候选，但 child claim 不重复；一个 Thread
正被删除时，其它未锁 Thread 仍能被领取；所有取得的 Thread 锁顺序严格升序；有界 refill 后没有
无界扫描、整批 deadline 或 head-of-line starvation。

### 2. 确定性旧锁环

以下三组必须保存真实 1213 RED，并在 GREEN 中只允许合法线性化后像：

1. delete × terminal mutation：外部锁 Message，使 delete 已持 Attempt；terminal 持 Run 后等
   Attempt；释放 Message 后 delete 等 Run，形成 `Run ↔ Attempt`；
2. delete × `CommitExecutionBoundary` exact replay：外部锁 SnapshotAccessAudit，使 delete 已持
   Ledger；replay 持 Attempt 后等 Ledger；释放 audit 后 delete 等 Attempt，形成
   `Attempt ↔ Ledger`；
3. delete × adaptive exact replay：外部锁 Ledger，使 delete 已持 Event；replay 持 Run/Attempt
   后等 exact Event；释放 Ledger 后 delete 等 Attempt，形成 `Attempt ↔ Event`。

每组覆盖 `DeleteThread` 与真正进入 idle cascade 的 `DeleteThreadIfIdle`，并覆盖双方先到。若
`DeleteThreadIfIdle` 的 active scan 实际提前串行某条路径，测试必须记录真实锁对象和合法串行结果，
不能伪造 1213。

### 3. 探索性物理锁叶

以下路径必须运行并保存真实锁图，但在 RED 前不得宣称必然形成特定 cycle：

- delete × `CreateJournalAttempt` 的 Attempt next-key insert；
- cascade × direct Plan mutation；
- cascade × Snapshot cleanup claim/complete；
- delete × adaptive first/recovery boundary。

原因是 DML subquery、secondary index 与 RR next-key 的物理加锁顺序必须由当前 MySQL 实证，不能
仅从 Go 语句顺序推断。

### 4. durable post-state

每个 GREEN leaf 必须断言：

- 无 1213、1205、deadline、panic、duplicate 或 fixture/schema noise；
- mutation 先赢时，删除等待其提交后完整删除；
- 删除先赢时，mutation fail closed，不能复活 Thread 或写入 child；
- 删除成功后，机器 inventory 中每一个 hard-delete PO/table 对该 Thread 都必须为零；该集合包含
  Plan/Item、ArtifactScanJob、Artifact、File、Token、MemoryFlushJob、TranscriptSnapshot、
  MemoryAudit、Memory、Checkpoint、RunEvent、SideEffectLedger、SnapshotAccessAudit、Attempt、
  Message 与 Run，禁止用抽样表或手写“等”缩小 post-state；
- 被对象存储清理协议故意保留的 Snapshot、object-backed Fragment 与 Reservation 必须逐字段满足
  tombstone invariant：敏感 inline/metadata/content 已清空、`deleted_at`/cleanup state/expiry 已推进、
  claim token 已清除；随后 post-delete cleanup 在 token fence 下最终收敛，不得把这些合法残留误判
  为 orphan；
- 未删除结果保留完整、唯一、可 replay 的 durable tuple，无 partial write 或 orphan；
- callback/outbox 的 inserted/replayed 语义与原合同一致。

## 回归与发布门

- P1L 硬 timebox 为 2 engineer-days；首个确定行为 RED 前不得改生产代码；
- 若穷举 inventory 或 post-delete continuation 无法在 timebox 内冻结，P1L 以 BLOCKED 退出并重新
  拆包；不得通过删除 mutation 条目、抽样孤儿断言或延长未审实现来维持 PASS 口径；
- 先冻结 inventory、旧源码 hash、测试 preimage 与 RED，再实施最小 GREEN；
- 同一 implementation commit 上连续运行两轮完整 real-MySQL 矩阵；
- 运行 agentthread repository package、P0A/P0B/P0C/P0D exact regressions、compile、gofmt 与
  `git diff --check`；
- Workbench authority 更新后严格串行运行 Node test、verify、唯一 build、verify-derived；
- 两阶段独立 review 均要求 P0/P1/P2 为 0；
- 证据写入 `/private/tmp/workbench-adaptive-mvp-p1l/`，由 exact inventory 与
  `evidence.sha256` 封存；
- docs commit、implementation commit 与 authority maintenance 必须线性、单一范围、无 staged 或
  untracked 漂移。

性能门使用同一 disposable MySQL、同一连接池配置和固定数据集，对 Event、Checkpoint、Token 三条
高频单表写及 Snapshot retention 各运行五轮基线/候选。候选的 p95 中位数不得超过基线
`baseline_p95 + max(0.2 × baseline_p95, 2 ms)`，不得出现 pool acquire timeout；批处理在一个
Thread 被独占锁住时仍须处理其它
Thread。阈值失败即 P1L FAIL，不能通过降低并发或增大连接池掩盖。

P1L PASS 只能声明 Thread deletion fence 在冻结 inventory 上闭合。它不表示 `AdaptiveGate` 已接线，
也不解除 rolling-authority blocker。P1M 只有在 P1L PASS 后才能开始；P1D 还必须等待 P1R PASS。

## 风险与回滚

主要风险是高频 child mutation 增加一次 Thread discovery、短事务和共享行锁。共享锁避免把普通写
完全串行，但会让真实删除等待正在进行的同 Thread 写入；这是删除完整性的预期代价。批量 retention
需要排序 ThreadID 并在锁内重验，可能降低单批吞吐，必须用现有 batch size 做基准而不能通过放宽
栅栏优化。

本设计不改变 schema 或公共合同，因此未进入后续 wiring 前可以回退 P1L implementation commit，
同时保持 P1M/P1D/P2 blocked。进入后续包或部署后，不能只删除栅栏恢复已知 deadlock/orphan 行为；
安全回滚必须同时回退所有依赖 P1L 的后续包，并以单独评审的 fail-closed 措施禁用两个 Thread 删除
入口，直到修复重新通过。无需数据回迁，但不允许在 delete 仍开放时把旧不安全锁图称为已回滚完成。
