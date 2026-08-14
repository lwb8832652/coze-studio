# P1M-C2 Durable Adaptive Bootstrap 设计

## 1. 结论

P1M-C2 为 C1 的 `AdaptiveAdmissionSnapshot` 和 `ExecutionDecision` 增加严格 codec 与一次
原子持久化。它只建立可恢复的 durable fact，不接入 `ADKExecutor.Execute/Resume`，也不改变
当前生产执行行为。

现有 `CommitAdaptiveExecutionBoundary` 不能直接复用：它要求已有 Plan、正 revision 和至少一个
PlanItem mutation。C2 新增独立的 `CommitAdaptiveExecutionBootstrap`，复用它已经验证过的 Run、
Attempt、lease、generation、cancel 和 lost-response fence，但不读取或修改 Plan。

## 2. 范围

C2 完成以下能力：

- 为 admission 和 decision 提供版本化、严格、有界的 canonical JSON codec；
- 在一个 repository transaction 中写入两条 internal typed RunEvent 和一条 control Checkpoint；
- 支持同一幂等操作的 exact replay，以及 durable fact 的只读回读；
- 拒绝不同载荷、部分事实、身份漂移和存量事实被篡改的情况；
- 证明 control fact 不进入公共 RunEvent、Checkpoint、history 或 SSE，也不会被 Eino 恢复选中。

C2 明确不做：

- 不接 `runner.go`、`resume_runner.go`、`ADKExecutor`、runtime factory 或生产依赖注入；
- 不调用 C1 producer，不从 `Run.Config`、`Run.Context` 或 `context.Context` 生成事实；
- 不创建 Plan、PlanItem、Journal projection、progress 或 verification；
- 不修改 IDL、生成 client、前端、migration 或 feature gate；
- repository bootstrap 在 C2 只接受 `source=fresh`；typed inheritance、legacy decoder、多跳 recovery
  和 server-owned Subagent compatibility seam 留给 C3；
- 不修改 whole-Thread DELETE guard，不声明 P1L、P1M 或 AdaptiveGate 已完成。

## 3. 分层

新增纯 domain contract 包 `backend/domain/agentthread/adaptivecontract`。它依赖 entity 类型，只负责
校验、codec、canonical bytes 和 digest，不依赖 application、repository、数据库或时钟。

现有 application 函数名和错误合同保持不变：

- `ValidateAdaptiveAdmissionSnapshot`；
- `ValidateExecutionDecision`；
- `ValidateExecutionDecisionAgainstAdmission`。

它们改为调用 domain contract 包的同义实现，现有 sentinel 通过 alias 保持 `errors.Is` 兼容。
repository 直接依赖 domain contract 包，因此持久化前和回读后使用同一套校验，不复制规则，也不
形成 repository → application 的反向依赖。

## 4. Strict codec

### 4.1 Admission wire

逻辑 schema 仍为 `workbench-adaptive-admission.v1`。对象必须精确包含以下字段：

```text
schema
feature_gate_enabled
source
source_run_id
source_execution_generation
source_config_digest
decoder_version
capabilities {
  plan_allowed
  read_only_tools_allowed
  sandbox_writes_allowed
  human_interaction_allowed
  subagents_allowed
}
limits {
  max_tool_calls
  max_replans
  max_verification_repairs
  max_consecutive_no_progress
  max_active_duration_seconds
}
```

可空 source 字段必须以 JSON `null` 表示；空字符串字段仍必须出现。codec 不使用 `omitempty`，
因此 canonical round-trip 不会把 null、zero 和 missing 混为一谈。

### 4.2 Decision wire

逻辑 schema 仍为 `workbench-adaptive-decision.v1`。对象必须精确包含 C1 `ExecutionDecision` 的
全部字段，字段名使用 snake_case。`plan_scope_run_id` 和 `clarification_question` 使用显式 null；
`deliverables`、`acceptance_checks` 必须是数组且不能是 null。每个 acceptance check 精确包含
`check_id`、`kind`、`target_ref`、`safe_description`。

### 4.3 解码和 canonical 规则

- admission 与 decision 的 raw bytes 和 canonical bytes 各自不得超过 64 KiB；
- 根对象、`capabilities`、`limits` 和 acceptance-check 对象都拒绝未知字段与重复字段；
- 字段名大小写敏感，不做 trim、case-fold、默认值填充或 schema 推断；
- 数字只接受 JSON integer，并执行 Go 目标类型的溢出检查；
- 只接受一个完整 JSON value，拒绝 trailing content；
- `DecodeAdaptiveAdmission` 只运行 admission 结构校验，`DecodeExecutionDecision` 只运行 decision
  结构校验；`ValidateAdaptiveBootstrapPair` 再运行 `ValidateExecutionDecisionAgainstAdmission` 并校验
  repository identity；commit 与 readback 都必须调用 pair validator；
- 校验通过后以固定字段顺序 marshal；
- decode 返回新分配的 slice 和 pointer；encode/decode 不修改输入；
- `goal_summary`、deliverables、`safe_summary`、clarification question 和 acceptance-check
  `safe_description` 拒绝现有公共投影同等级别的 credential、token、URL 和绝对路径模式。
- `decision_id`、`attempt_id`、acceptance-check 的 `check_id`、`kind`、`target_ref` 必须同时满足
  `^[A-Za-z0-9_.:-]+$` 和各自 C1 byte cap，并经过同一敏感模式拒绝；这些字段不允许 raw URL、路径、
  credential 或自由文本。

adaptive contract 使用 `workbench-adaptive-safe-text.v1`，正则字节冻结为：

```text
(?i)(authorization\s*:\s*bearer\s+\S+|bearer\s+[A-Za-z0-9._~+/=-]{8,}|["']?(?:api[_-]?key|access[_-]?token|client[_-]?secret|aws[_-]?secret[_-]?access[_-]?key|secret[_-]?(?:key|token)|private[_-]?key|credential|password|(?:[a-z0-9][a-z0-9_-]*_)?(?:secret|token))["']?\s*[:=]\s*["']?\S+|(?:^|[^A-Za-z0-9])(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{4,}|(?:https?|file|s3|oss|cos|minio)://)
(?i)(?:^|[^A-Za-z0-9._-])(?:/(?:[^/\s]+)(?:/[^\s]*)?|[a-z]:\\[^\s]+|\\\\[^\s]+)
```

它们是 C2 对当前公共投影 `publicSensitivePattern` 与 `publicAbsolutePathPattern` 的固定副本。对上述
每个字段逐值扫描；任一命中就返回 invalid，不 trim、不 redact、不替换。application parity test 用
同一组冻结 safe/credential、bearer/token、http/file/s3/oss/cos/minio URL、Unix/Windows/UNC 绝对
路径语料同时运行公共投影和 C2 helper，结果必须逐项相等。C2 不为复用一个私有正则而重构公共投影。

digest 一律是 canonical bytes 的 lowercase SHA-256。repository 不接受调用方提交 digest。

## 5. Durable facts

一次 bootstrap 写入三个 row：

| row | 固定身份 | 内容 |
| --- | --- | --- |
| Admission RunEvent | `event_type=adaptive.admission` | 完整 canonical admission payload |
| Decision RunEvent | `event_type=adaptive.decision` | 完整 canonical decision payload |
| Control Checkpoint | `runtime_type=workbench_control`、`checkpoint_ns=workbench.adaptive.bootstrap` | 只保存事件引用、身份和 digest |

两条 RunEvent 都满足：

- `visibility=internal`；
- `run_id` 是 physical execution Run，`journal_run_id` 是 logical Journal root；
- 绑定同一 `journal_run_id + attempt_id`；
- `sequence=NULL`，不消费或推进用户 Journal sequence；
- 使用由 operation idempotency key 派生的两个固定 event idempotency key；
- `schema_version` 使用当前 Journal storage schema，payload 自身的 `schema` 才是 typed fact
  discriminator；
- `payload_version` 使用当前 Journal payload version，`status`、parent、action、trace 与
  `journal_payload` 均为空；
- `occurred_at_unix_nano=fact_created_at*1_000_000`、`created_at=fact_created_at`，乘法溢出即
  invalid；
- `snapshot_id` 绑定 control Checkpoint 的 server-owned bootstrap authority fingerprint。

先对 trim 后的 operation key 取 lowercase SHA-256。event key 精确为
`adaptive-bootstrap-admission-<digest>` 与 `adaptive-bootstrap-decision-<digest>`，不直接拼接原 key；
相同 operation key 永远得到相同两条 event key。

Control Checkpoint 满足：

- `runtime_key` 精确为 `adaptive-bootstrap-<digest>`；digest 输入为
  `workbench-adaptive-bootstrap.v1\n<journal_run_id base10>\n<attempt_id>` 的 UTF-8 bytes，一个 Attempt
  最多一个 bootstrap；
- `parent_checkpoint_id=0`，不加入 Eino runtime checkpoint parent chain；
- `envelope_version=1`、`runtime_deleted_at=0`；
- `channel_values={}`、`channel_versions={}`、`pending_sends=[]`；
- metadata 根对象精确只含 reserved `adaptive_bootstrap`，总 canonical bytes 不得超过 64 KiB；
- `adaptive_bootstrap` 只含 schema、Thread/Run/Attempt/generation、可空
  recovery source tuple、两个 event ID/key/digest、operation-key digest 和 checkpoint fingerprint；
- metadata 不复制 admission、decision、Plan、tool payload、Run config 或 credential。

bootstrap metadata schema 固定为 `workbench-adaptive-bootstrap.v1`。所有 fingerprint 都覆盖对应
row 的 immutable scalar identity 与 canonical JSON；回读时逐项重算。两个 event content
fingerprint 不含 `snapshot_id`。Checkpoint fingerprint 覆盖其全部 immutable scalar/JSON 以及
metadata 中除自身 `checkpoint_fingerprint` 以外的字段；metadata 保存该 fingerprint，两条 event 的
`snapshot_id` 必须等于它，由此避免自引用 hash。

C2 writer 只允许 `source=fresh`，因此 metadata 的 recovery source tuple 在本切片必须全部为 null，
并要求当前 Attempt 的 `source_attempt_id`、`source_checkpoint_id`、`recovery_idempotency_key` 全为空。
codec 仍支持 C1 的三种 source，以便 C3 在验证真实来源后复用；C2 repository 不接受调用方自行提交
typed/legacy lineage。

## 6. Repository contract

`AdaptiveExecutionRepository` 新增：

```text
CommitAdaptiveExecutionBootstrap(ctx, request) -> result
ReadAdaptiveExecutionBootstrap(ctx, request) -> result
```

commit request 包含：

- Thread、execution Run、Journal root、Attempt identity；
- expected generation、lease owner/token 和用于 lease-expiry 判断的 `now`；
- operation idempotency key；
- admission、decision；
- stable `fact_created_at`；
- 首次插入使用的 admission event、decision event 和 checkpoint candidate ID。

decision 内的 execution Run、Journal Run、Attempt、generation 与 request 必须精确相等，
`decision.created_at` 必须等于 `fact_created_at`。C2 是首次 bootstrap，因此
`decision_revision=1`；非空 `plan_scope_run_id` 必须等于当前 execution Run，不能引用尚未验证的外部
Plan scope。candidate ID 只用于首次插入；exact replay 按 durable
event tuple 和确定性 runtime key 定位事实，忽略新的 candidate ID 与 `now`，并返回原 ID 和原时间。
同载荷的 retry 必须继续提交相同 operation key。三个 candidate ID 都必须为正数，两条 event ID
必须不同；checkpoint 属于独立表，可以与 event 使用相同数值。

result 包含重新从 transaction 读取并 decode 的 admission、decision、两个 event、checkpoint、
immutable authority 和 `Replayed`。禁止直接返回 request 中的 struct、slice 或 pointer。

`AdaptiveExecutionRepository` 是现有窄接口；新增 commit/read 方法只加入这个接口和
`threadRepository` 实现，不加入通用 `ThreadRepository`、domain `ThreadService` 或
`ApplicationService`。因此 C2 没有可被 handler/worker 误用的新 production entry point。

read request 使用 `thread_id + execution_run_id + journal_run_id + attempt_id`。它不要求来源 Run
仍有 lease，也不要求 Attempt 仍 active；它只读取已提交的 immutable fact，逐项验证 Attempt identity、
checkpoint metadata、event tuple、codec 和 fingerprint。四个 identity 字段必须命中同一个 Attempt；
缺失返回 not-found，部分、跨 Thread/Run 或漂移返回 conflict。

reader 在 MySQL 使用 read-only Repeatable Read transaction，在 SQLite 使用单个 transaction；所有
Attempt、event 和 checkpoint 读取都在该 transaction 内完成，避免 retention 并发时拼接跨快照结果。

`fact_created_at` 与 `decision.created_at` 均为正数 Unix 毫秒。operation key 必须非空、最多 191
bytes 且与 `strings.TrimSpace` 结果逐字相等；request 中的 lease token 和 operation key 永不写入
metadata，持久化的只有 operation-key digest。Attempt ID 同样必须与 trim 结果相等且最多 64 bytes。

## 7. Transaction 与 replay

新提交沿用 P0 已验证的确定顺序。每个 `FOR UPDATE` query 都按下列顺序单独执行；不得把 Run/Attempt
锁合成 `IN (...) ORDER BY id`：

```text
logical Journal root Run
-> distinct execution Run
-> target Attempt
-> admission exact tuple / reserved admission set
-> decision exact tuple / reserved decision set
-> deterministic control Checkpoint
```

在锁住 Run/Attempt 后，先按派生 key 锁两条 exact event，再锁 deterministic control Checkpoint：

- 两条 event 和 checkpoint 都存在：验证完整 durable fact；同 operation、同 canonical payload返回
  `Replayed=true`，即使 lease 已过期或 Attempt 已 terminal；
- 三者均不存在：再查询同 Attempt 的 reserved admission/decision set；只有 set 也为空时，才校验
  running Run、generation、lease、expiry、cancel、fresh Attempt 与 execution lineage，然后写入；
- 只存在一部分、同 Attempt 已有另一 bootstrap、或任一 row 身份/fingerprint 漂移：返回 conflict，
  不修补、不覆盖、不生成第二份事实。

checkpoint 已存在时，以其 metadata 引用的 event ID/key 验证 bootstrap；同 Attempt 后续合法的
Plan-bearing decision revision 不影响旧 bootstrap replay。checkpoint 不存在但发现任何 reserved
admission/decision 时一律视为 partial/conflict，不能据此“补齐”事实。

在 MySQL 上，event absence probe 使用既有
`uk_agent_run_events_attempt_idempotency(journal_run_id,attempt_id,idempotency_key)` 唯一索引的
locking read。Checkpoint 没有唯一 runtime-key 索引：在已锁 Attempt 后按
`thread_id + run_id + runtime_type + runtime_key` 扫描并要求 row count 为 0 或 1，超过 1 立即
conflict；正常 writer 的串行化由 Attempt 行保证，不能声称数据库提供 checkpoint tuple uniqueness。
SQLite 使用相同 predicate 和 0/1 规则。

fresh commit 不读取、锁定或修改 `agent_run_plans`、`agent_run_plan_items`，也不改变 Attempt 的
`next_sequence`/`last_committed_sequence`。两个 event insert、checkpoint insert 和 transaction
readback 任一步失败时全部回滚。

同 operation key 并发时只允许一个 commit，其他调用读取同一 durable fact；不同 operation key
并发到同一 Attempt 时，先提交者成为唯一 bootstrap，另一方返回 conflict。禁止 retry loop、sleep、
advisory lock 或忽略 MySQL 1213/1205。

此锁前缀沿用 P0 的 Journal root → execution Run → Attempt 顺序，不新增 Thread lock。whole-Thread
DELETE 仍由 P1M-A 的编译期 guard 硬禁用；C2 不借机声称 P1L lock-order closure 已完成。

## 8. 错误合同

新增稳定 repository 错误：

- `ErrAdaptiveExecutionBootstrapInvalid`：request、codec 或 candidate identity 非法；
- `ErrAdaptiveExecutionBootstrapNotFound`：reader 找不到完整 bootstrap；
- `ErrAdaptiveExecutionBootstrapConflict`：不同 operation/payload、部分事实、重复 bootstrap 或 durable
  row 漂移。
- `ErrAdaptiveExecutionReservedFact`：通用 event/checkpoint 写删入口试图伪造或删除 C2 reserved fact。

Run lease、cancel、Attempt 和 lineage 失败继续使用现有 `ErrRunLeaseLost`、`ErrRunCanceled`、
`ErrAdaptiveExecutionAttemptConflict`、`ErrAdaptiveExecutionLineageConflict`。数据库原始错误保留为
cause，但错误消息不得包含 payload、credential、lease token 或完整 idempotency key。

## 9. 公共与 runtime 隔离

`workbench_control` 是 server-only runtime type。C2 的专用 reader 直接读取它，其他通用读取入口
不得返回它：

- generic/canonical RunEvent list 与 cursor 排除 `visibility=internal`；
- `GetCheckpoint`、`GetLatestCheckpoint`、`ListCheckpoints`、`ListCheckpointsBefore` 排除
  `runtime_type=workbench_control`；
- Eino runtime lookup 仍只接受 `runtime_type=eino_adk`，因此不会选中 control Checkpoint；
- public projector 对 `adaptive.admission`、C2 的 `adaptive.decision` 和 `workbench_control` 明确
  fail closed，即使上游过滤被误改也返回 nil；C2 不为 decision 增加公共 allowlist。

共享的 `createBaseRunEvent` 拒绝 `adaptive.admission`、`adaptive.decision`，因此
`CreateRunEvent`、`CreateRunBundle`、`AppendJournalEvent` 和普通 Journal projection 等所有现有通路
都无法伪造它们。新增 package-private reserved writer 只接受这两个精确类型：bootstrap 是
`adaptive.admission` 的唯一调用者；bootstrap 与现有 Plan-bearing adaptive boundary 可以写
`adaptive.decision`。现有 boundary validator 另外明确拒绝 `adaptive.admission`，且不能写
unsequenced bootstrap decision。

共享的 `checkpointToPO` 拒绝 `runtime_type=workbench_control`；bootstrap 使用独立的 package-private
converter，在验证完整 reserved schema 后才生成 PO。这样 CreateCheckpoint、canonical public-state、
Journal boundary、FinalizeRunSuccess 和其他现有 checkpoint 写路都不能伪造 control row。
`DeleteRuntimeCheckpoint` 对 control 返回 `ErrAdaptiveExecutionReservedFact`，
`GetLatestRuntimeCheckpoint` 对 control 返回 `(nil, nil)`；只有 bootstrap transaction 和专用 reader
能创建、读取该 row。`GetCheckpoint` 对 control ID 返回既有 not-found，而不是空投影。

Checkpoint 的通用 read SQL 始终先加 `runtime_type <> 'workbench_control'`，再应用调用方 filter：

- `GetCheckpoint(id)` 命中 control 时返回既有 not-found；
- `GetLatestCheckpoint(thread)` 跳过 control，继续返回最新的非 control row；
- `ListCheckpoints` 即使请求 `RuntimeType=workbench_control` 也返回空集合和 `total=0`；
- `ListCheckpointsBefore` 跳过 control 后再计算 limit、has_more 与 cursor；
- `GetLatestRuntimeCheckpoint(..., workbench_control, ...)` 固定返回 `(nil, nil)`。

这里不扩张 Journal 查询：`GetJournalEvent` 知道 ID 时仍可读取 internal event；
`ListJournalEvents` 继续只列 `visibility=user AND sequence IS NOT NULL`，不会列出 C2 fact。成组回读
只走 dedicated bootstrap reader。禁止的是 generic/canonical/history/SSE 公共路径，不是删除仓库
内部按 ID 审计能力。

过滤后的 total、pagination、has_more 和 cursor 只按可返回 row 计算，不能通过空页或计数泄露
内部 fact。后续公共 decision 投影属于独立切片，不能在 C2 顺手开放。

现有 retention 合同不在 C2 改写：active/recoverable Attempt 的 bootstrap 继续受既有保留谓词保护；
Attempt terminal 且超过 cutoff 后，internal event 与 control checkpoint 可由既有 retention 清理。
三 row 全无时专用 reader 返回 not-found，清理中或其它 partial 状态返回 conflict；C2 不把 control
row 变成永久记录，也不扩张 P1L retention 范围。

当前 P0C verified-success 原型只识别带 sequence、Plan authority 的 `adaptive.decision`，不会读取
C2 的 unsequenced bootstrap decision。C2 因而只建立首次 durable authority，不宣称该 gate 已消费
它。C3/P1D 必须让 gate 直接引用同一 bootstrap decision，并在后续有合法 Plan revision 时选择最新
authority；禁止复制成第二条“初始 decision”来伪造接线。

## 10. 测试与完成条件

### 10.1 Codec

- admission 三种 source、decision 三种 kind/两种 execution shape round-trip；
- fixed canonical bytes/digest，重复执行确定；
- unknown、duplicate、missing、wrong type、overflow、trailing、null/empty 和 64 KiB 边界；
- secret/URL/path 拒绝；输入、输出 slice/pointer 无别名且不被修改。

### 10.2 SQLite repository

- 无 Plan 时首次提交成功，三 row 与完整 raw PO 符合合同，Plan/Items 和 Attempt cursor byte/field
  不变；
- typed-inheritance/legacy admission、或带 recovery source tuple 的 Attempt 在 C2 commit 中 fail
  closed 且零写；
- repository reload 后同 payload exact replay；新 candidate ID、now 不改变 durable result；
- 同 key 不同 payload、不同 key、partial row、tampered row 都 conflict 且零写；
- 预占 checkpoint candidate ID 使第三次 insert 失败，证明两条 event 一并回滚；
- reader 返回 durable deep copy，terminal Attempt 可读，缺失/跨 Thread/跨 Run/同名 Attempt
  都 fail closed；
- lease、generation、expiry、cancel、Attempt status/active slot 漂移走现有 typed error。

### 10.3 Real MySQL

- 两个独立 connection 同 operation key 并发，只有一个首次 commit，另一方 exact replay；
- 两个不同 operation key 同 Attempt 并发，只有一个 durable bootstrap，另一方 conflict；
- 无 1213、1205、deadline、duplicate raw error、partial row 或 Plan 变化。

测试使用锁/observer barrier，不使用 `time.Sleep` 或 `t.Parallel`。普通 `go test` 的 skip 不算
证据；JSON 必须逐名证明目标 leaf 各通过一次且零 skip/fail/noise。

### 10.4 隔离与回归

- generic/canonical event 与 checkpoint 查询、history 和 SSE 均看不到 C2 fact，分页仍连续；
- generic event/checkpoint 创建和 runtime checkpoint 删除不能伪造或删除 reserved fact；
- 同 Run 的真实 Eino checkpoint 仍由 `GetLatestRuntimeCheckpoint` 选中；
- 现有 P0 adaptive boundary、recovery、verified success 与 Journal sequence tests 全部通过；
- package compile、`gofmt`、`git diff --check`、Workbench graph verify/build/derived 通过；
- 独立规格与质量审查 P0/P1/P2=0。

## 11. 预计文件面与工期

生产候选只涉及：

- 新增 domain `adaptivecontract` codec/validation；
- application validator 的兼容 wrapper；
- adaptive repository contract 与 MySQL/SQLite 实现；
- generic public query 的 internal/control 过滤；
- 对应 focused tests 和 Workbench authority 文件。

不新增 migration。实施计划拆成两个独立检查点：

- C2a：codec/domain 校验/application wrapper，只跑纯单元与 package 回归，独立审查后提交；
- C2b：repository transaction/readback/public isolation，先 SQLite 再 real MySQL，独立审查后提交并同步
  Workbench authority。

任一检查点的文件 hash 变化都会使其审查和测试证据失效。总工期预计 6–10 小时。任何需要接
`Execute/Resume`、创建 Plan 占位行或扩大 P1L 的发现都立即停止 C2，返回设计修订，不在本包内扩
scope。
