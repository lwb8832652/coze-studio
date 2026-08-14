# Workbench 自适应执行 MVP P0A2 SQLite Atomic Mutation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 执行。只在干净的
> `/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp` 工作树执行。

**Goal:** 在不新增迁移的前提下，用一个 SQLite transaction 原子提交 authoritative RunEvent、
versioned runtime Checkpoint、1～32 个完整 PlanItem mutation、Plan revision 与 Attempt sequence；
同时证明 initial/recovery/multi-hop plan scope 谱系及所有失败路径零残留。

**Architecture:** 复用 P0A1 的 physical Run 与 active Attempt fence。事务固定按
physical Run → active Attempt → recovery source Attempt/checkpoint/source Run → Plan scope Run →
Plan → sorted PlanItems 加锁；全部行验证完成后写 Event、Checkpoint、PlanItems、Plan，最后以
Attempt 的 active slot、next sequence、last committed sequence 做 CAS。Plan 只持久化稳定的大步骤；
`AgentRunPlanItem.Metadata` 可保存由后续包按需展开的子步骤引用，本包不引入第二套 Plan 表或执行器。

**Files permitted for modification:**

- Modify: `backend/domain/agentthread/repository/adaptive_execution.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`

**Read only:**

- `backend/domain/agentthread/repository/mysql.go:141-193`
- `backend/domain/agentthread/repository/mysql.go:488-519`
- `backend/domain/agentthread/repository/mysql.go:4165-4430`
- `backend/domain/agentthread/repository/mysql.go:5652-5715`
- `backend/domain/agentthread/repository/mysql.go:6220-6298`
- `backend/domain/agentthread/repository/mysql_journal.go:417-559`
- `backend/domain/agentthread/repository/mysql_journal_test.go:1945-2005`
- `backend/domain/agentthread/entity/plan.go`

---

## 0. Frozen contract

### 0.1 Exported types

`adaptive_execution.go` 增加以下 exact contract；不得在本包增加 ADK、HTTP 或 UI 类型：

```go
var (
	ErrAdaptiveExecutionLineageConflict    = errors.New("adaptive execution lineage conflict")
	ErrAdaptiveExecutionPlanScopeConflict  = errors.New("adaptive execution plan scope conflict")
	ErrAdaptiveExecutionCheckpointConflict = errors.New("adaptive execution checkpoint conflict")
	ErrAdaptiveExecutionPlanRevisionConflict = errors.New("adaptive execution plan revision conflict")
	ErrAdaptiveExecutionPlanItemVersionConflict = errors.New("adaptive execution plan item version conflict")
	ErrAdaptiveExecutionSequenceConflict   = errors.New("adaptive execution sequence conflict")
)

type AdaptivePlanItemMutation struct {
	ExpectedVersion int64
	NextItem        *entity.AgentRunPlanItem
}

type AdaptivePlanMutation struct {
	PlanScopeRunID   int64
	ExpectedRevision int64
	NextRevision     int64
	Items            []AdaptivePlanItemMutation
}

type CommitAdaptiveExecutionBoundaryResult struct {
	Event                *entity.RunEvent
	Checkpoint           *entity.Checkpoint
	Plan                 *entity.AgentRunPlan
	Items                []*entity.AgentRunPlanItem // TaskID ascending
	LastCommittedSequence uint64
}

type AdaptiveExecutionRepository interface {
	CommitAdaptiveExecutionBoundary(
		ctx context.Context,
		req CommitAdaptiveExecutionBoundaryRequest,
	) (*CommitAdaptiveExecutionBoundaryResult, error)
}
```

`mysql_adaptive_execution.go` 同时增加：

```go
func NewAdaptiveExecutionRepository(db *gorm.DB) AdaptiveExecutionRepository
```

本包不把该 interface 嵌入 `PersistentRepository`；应用层接线属于后续包。

在既有 `CommitAdaptiveExecutionBoundaryRequest` 末尾只增加：

```go
PlanMutation *AdaptivePlanMutation
```

### 0.2 Validation invariants

- 保留 P0A1 `validateAdaptiveExecutionBoundaryRequest` 的既有语义和测试；新增
  `validateAdaptiveExecutionMutationRequest`，先调用 P0A1 validator，再校验本节 mutation/runtime
  条件。production commit method 只调用新 validator。
- `PlanMutation` 必须非 nil；`PlanScopeRunID > 0`。
- `ExpectedRevision > 0` 且 `NextRevision == ExpectedRevision+1`。
- `Items` 数量为 1～32；item ID 和 TaskID 分别唯一。
- 每项 `ExpectedVersion >= 0`；`NextItem` 非 nil；`NextItem.ID/RunID/TaskID > 0`；
  `NextItem.RunID == PlanScopeRunID`；`NextItem.Version == ExpectedVersion+1`。expected=0 表示首次
  创建且数据库中 `(run_id, task_id)` 必须不存在；expected>0 表示更新且现存 ID/TaskID 必须 exact。
- Event ID/type、Checkpoint ID/namespace/runtime key 均非空；Checkpoint runtime type 必须为
  `eino_adk`，`EnvelopeVersion > 0`、`RuntimeDeletedAt == 0`；Event、Checkpoint 的 `CreatedAt`
  均等于 request `Now`。
- `Blocks`、`BlockedBy`、PlanItem `Metadata`、Event payload、Checkpoint channel JSON 均继续由既有
  conversion helper 做合法 JSON 校验；禁止复制 JSON 解析器。
- expected=0 create 由 repository 把 `CreatedAt/UpdatedAt` 都写为 request `Now`；expected>0 update
  保留 locked row 的 `CreatedAt` 并把 `UpdatedAt` 写为 request `Now`。TaskID、ID 和 Plan scope
  不允许修改。

### 0.3 Checkpoint metadata

repository 要求输入 Checkpoint 的 `Metadata` 是 JSON object，保留已有 runtime fields，并在唯一
server-owned `adaptive_execution` namespace 下写 exact versioned object；输入若已带同名 namespace
则拒绝，禁止客户端覆盖：

```json
{
  "runtime_field": "preserved",
  "adaptive_execution": {
    "schema_version": "workbench-adaptive-boundary.v1",
    "event_id": 7001,
    "event_sequence": 7,
    "journal_run_id": 30,
    "attempt_id": "attempt-1",
    "plan_scope_run_id": 20,
    "plan_revision": 2,
    "item_fingerprint": "64-lowercase-hex",
    "execution_run_id": 20,
    "execution_generation": 3
  }
}
```

`item_fingerprint` 对 mutation 后的 items 按 `TaskID ASC` 排序，以固定字段 struct JSON 编码后做
SHA-256；字段精确为 ID、RunID、TaskID、Subject、Description、Status、ActiveForm、Owner、
Blocks、BlockedBy、Metadata、Active、Version、CreatedAt、UpdatedAt。三个 JSON 字段的 canonical
helper 必须用 `json.Decoder.UseNumber` decode、确认第二次 decode 为 EOF，再以 `json.Marshal` 编码；
这样 object key order 不影响 digest，数组顺序和 number lexical form 保留。禁止手写 map 拼接。

### 0.4 Initial and recovery lineage

- target Attempt 除 P0A1 条件外必须 `ActiveSlot == 1`、`NextSequence > 0` 且
  `LastCommittedSequence < NextSequence`，并拒绝 `NextSequence == math.MaxUint64` 防止 `+1` 溢出；
  SQLite 未携带 MySQL CHECK，repository 必须显式拒绝漂移。普通 Journal events 可以只推进
  NextSequence，因此不得错误要求 `NextSequence == LastCommittedSequence+1`。
- initial Attempt：`SourceAttemptID` 与 `SourceCheckpointID` 必须同时 NULL，且
  `PlanScopeRunID == ExecutionRunID`、target `Checkpoint.ParentCheckpointID == 0`。
- recovery Attempt：两个 source 字段必须同时 non-NULL。按
  `(JournalRunID, SourceAttemptID)` 锁 source Attempt，再按 `SourceCheckpointID` 锁 checkpoint；
  target `Checkpoint.ParentCheckpointID` 必须等于 `SourceCheckpointID`；SourceAttemptID 不得等于
  target AttemptID，SourceCheckpointID 不得等于 target Checkpoint ID；只允许
  `sourceAttempt.ExecutionRunID == sourceCheckpoint.RunID`。
- source checkpoint metadata 的 JournalRunID、AttemptID、physical execution Run ID 必须匹配
  source Attempt；checkpoint 必须 `RuntimeDeletedAt == 0`，且
  `event_sequence <= sourceAttempt.LastCommittedSequence`。
- 必须按 metadata `event_id` 读取 source `runEventPO`，并核对其 ThreadID、RunID、JournalRunID、
  AttemptID、Sequence 与 metadata/source Attempt exact；不得信任无 FK 的 checkpoint metadata。
- 再锁 source physical Run；其 ThreadID 必须一致，metadata `execution_generation` 必须等于该 Run
  当前 `execution_generation`。
- target `PlanScopeRunID` 必须等于 source metadata 继承的 `plan_scope_run_id`，metadata
  `plan_revision` 必须等于 mutation `ExpectedRevision`。不得要求 Plan scope 等于 source physical Run。
- target/source Attempt、source Event/checkpoint 必须属于同一 Thread；任一半空、漂移或 ambient
  same-thread 非 canonical source 都 wrap `ErrAdaptiveExecutionLineageConflict`。Plan scope Run/Plan
  exact 条件是 `plan.ThreadID == scopeRun.ThreadID == targetRun.ThreadID`、
  `plan.SpaceID == scopeRun.SpaceID == targetRun.SpaceID`、
  `plan.UserID == scopeRun.CreatorID == targetRun.CreatorID`；漂移 wrap
  `ErrAdaptiveExecutionPlanScopeConflict`。
- A 持有 Plan，B 从 A checkpoint 恢复，C 从 B checkpoint 恢复时，B/C 的 physical Run 各自不同，
  但两次 checkpoint 的 `plan_scope_run_id` 始终为 A。

### 0.5 Atomic write and CAS

- 从 locked target Attempt 的 `NextSequence` 分配 Event sequence；必须大于 0。
- 用 `runEventToPO` 写 authoritative base Event，并无条件写入同一行的
  `journal_run_id`、`attempt_id`、`sequence`、`idempotency_key`；不要求 Journal projection 成功，
  不调用会提前推进 sequence 的 `appendJournalEventLocked`。
- 用 `checkpointToPO` 写合并后的 metadata；当前 schema 只有 checkpoint `id` 是唯一键，因此只有
  primary-key ID 冲突统一 wrap `ErrAdaptiveExecutionCheckpointConflict`。不得虚构 runtime tuple UQ；
  若实现需要该 UQ，立即停止并回设计评审。
- items 按 `TaskID ASC` 锁定。expected=0 只允许完整 INSERT；expected>0 逐行以
  `(id, run_id, task_id, expected_version)` CAS 完整可写内容与 next version；任一存在性或版本漂移
  wrap `ErrAdaptiveExecutionPlanItemVersionConflict`。所有 TaskID 必须 `<= lockedPlan.HighWatermark`；
  boundary 不分配 TaskID，避免与 `ReservePlanTaskID` 的后续分配重复。expected=0 在任何写入前必须
  分别确认 candidate `id` 与 `(run_id, task_id)` 都不存在。
- Plan 以 `(run_id, expected_revision)` CAS 到 `next_revision`；零行更新 wrap
  `ErrAdaptiveExecutionPlanRevisionConflict`，HighWatermark 不变，`UpdatedAt=req.Now`。
- 最后 Attempt 以
  `(id, active_slot, next_sequence, last_committed_sequence)` 为条件，同时把
  `next_sequence` 写为 `sequence+1`、`last_committed_sequence` 写为 `sequence`；零行更新 wrap
  `ErrAdaptiveExecutionSequenceConflict`，成功时 `UpdatedAt=req.Now`。
- 任一错误返回后 transaction 回滚；Event、Checkpoint、Plan、Items、Attempt 必须与调用前逐字段相等。

---

## 1. Entry gate

- [ ] **Step 1: 验证 P0A1 PASS 与 docs-only base。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  test ! -e "$P0A2_EVIDENCE_DIR"
  mkdir -p "$P0A2_EVIDENCE_DIR"
  test -z "$(git status --porcelain)"
  (cd "$P0A1_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  grep -qx 'STATUS=PASS' "$P0A1_EVIDENCE_DIR/state.env"
  P0A1_HEAD_SHA="$(cat "$P0A1_EVIDENCE_DIR/head.sha")"
  grep -qx "HEAD_SHA=$P0A1_HEAD_SHA" "$P0A1_EVIDENCE_DIR/state.env"
  test "$(git rev-parse HEAD^)" = "$P0A1_HEAD_SHA"
  test "$(git log -1 --format=%s)" = 'docs: add adaptive execution P0A2 packet'
  test "$(git diff-tree --no-commit-id --name-only -r HEAD | sed '/^$/d' | wc -l | tr -d ' ')" = 1
  test "$(git diff-tree --no-commit-id --name-only -r HEAD)" = \
    'docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a2-sqlite-atomic.md'
  P0A2_BASE_SHA="$(git rev-parse HEAD)"
  printf '%s\n' "$P0A2_BASE_SHA" > "$P0A2_EVIDENCE_DIR/base.sha"
  printf 'PACKET=P0A2\nSTATUS=active\nBASE_SHA=%s\n' "$P0A2_BASE_SHA" \
    > "$P0A2_EVIDENCE_DIR/state.env"
  ```

  Expected: P0A1 evidence 为 PASS，当前 HEAD 是唯一 P0A2 plan 的 docs-only commit。

- [ ] **Step 2: 运行 P0A1 exact 回归基线。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-entry \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestValidateAdaptiveExecutionBoundaryRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt)$' \
      -count=1 -v 2>&1 | tee "$P0A2_EVIDENCE_DIR/entry-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A2_EVIDENCE_DIR/entry-green.log"; then exit 1; fi
  ```

  Expected: P0A1 三个顶层测试 PASS、零 SKIP/fail。

## 2. Contract RED/GREEN

- [ ] **Step 1: 写 `TestValidateAdaptiveExecutionMutationRequest`。**

  在现有测试文件扩展合法 request，再分别验证 nil mutation、Plan scope 0、revision 不连续、0/33
  items、重复 item ID、重复 TaskID、expected version -1、nil NextItem、NextItem scope drift、
  NextItem version 不连续、
  Event ID 0/空 type、Checkpoint ID 0/空 namespace、Event/Checkpoint timestamp drift、非
  `eino_adk` runtime、空 RuntimeKey、envelope version 0、RuntimeDeletedAt 非 0；每例精确
  `ErrorIs(ErrAdaptiveExecutionBoundaryInvalid)`。另覆盖 Checkpoint Metadata 非 object 与预置
  `adaptive_execution` namespace，均 fail closed。

- [ ] **Step 2: 运行 contract RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  contract_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-contract-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestValidateAdaptiveExecutionMutationRequest$' -count=1 \
      >"$P0A2_EVIDENCE_DIR/contract-red.log" 2>&1 || contract_status=$?
  test "$contract_status" -ne 0
  rg -n 'undefined: (AdaptivePlanMutation|AdaptivePlanItemMutation)' \
    "$P0A2_EVIDENCE_DIR/contract-red.log"
  ```

  Expected: 只因 mutation types 未定义而 RED。

- [ ] **Step 3: 增加 exact types、errors、interface 与纯 validation。**

  只修改 `adaptive_execution.go`；不得开始 transaction、metadata 或数据库 helper。

- [ ] **Step 4: 运行 contract GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-contract-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestValidateAdaptiveExecutionMutationRequest$' -count=1 -v 2>&1 | \
      tee "$P0A2_EVIDENCE_DIR/contract-green.log"
  ```

  Expected: success + 全部 invalid 子例 PASS。

## 3. Initial atomic commit RED/GREEN

- [ ] **Step 1: 扩展 SQLite fixture。**

  新增 test-only `newAdaptiveExecutionRepositoryTestDB`：调用 `newJournalRepositoryTestDB` 后只
  `AutoMigrate(checkpointPO, agentRunPlanPO, agentRunPlanItemPO)`。新增 seed/readback helpers，完整读取
  Run、Attempt、Event、Checkpoint、Plan 和 sorted Items；不得修改共享 journal test helper。

- [ ] **Step 2: 写 `TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON`。**

  两组 item 仅改变 Blocks/BlockedBy/Metadata object key 顺序时 fingerprint 必须相等；改变一个实际
  value、array order 或 item content 时必须不等；invalid/trailing JSON 必须返回 error。

- [ ] **Step 3: 写 `TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically`。**

  seed initial Attempt（两个 source NULL）、Plan revision=1/HighWatermark=2 和 TaskID=1 version=1 item。
  以 **TaskID=2 create 在前、TaskID=1 update 在后** 的逆序 mutation 输入，提交 next revision=2、
  TaskID=1 version=2 完整 update 与 TaskID=2 expected=0/version=1 完整 create；断言 result/数据库 Items
  均按 TaskID ASC 返回，并用测试内独立写死的 64 位小写 hex literal 断言 fingerprint，不得调用生产
  fingerprint helper 生成 expected；
  断言 Event identity
  tuple/sequence/idempotency、Checkpoint 生成 metadata、
  Plan/Items 完整内容、Attempt `NextSequence=2/LastCommittedSequence=1` 与 result 一致。DB
  `runEventPO` 额外精确核 journal tuple；result Event 只按既有 base entity 字段比较，不扩
  `entity.RunEvent`。

- [ ] **Step 4: 写 `TestAdaptiveExecutionCheckpointMetadataRoundTrip`。**

  输入 runtime metadata field 必须原样保留，server `adaptive_execution` exact object 必须可解码并与
  Event/Attempt/Plan/Run 对齐；输入预置 server namespace 已由 contract test 拒绝。

- [ ] **Step 5: 运行 initial RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  initial_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-initial-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically|TestAdaptiveExecutionCheckpointMetadataRoundTrip)$' -count=1 \
      >"$P0A2_EVIDENCE_DIR/initial-red.log" 2>&1 || initial_status=$?
  test "$initial_status" -ne 0
  rg -n 'CommitAdaptiveExecutionBoundary' "$P0A2_EVIDENCE_DIR/initial-red.log"
  ```

  Expected: repository method 未实现而 RED。

- [ ] **Step 6: 实现 lock/normalize/fingerprint helpers。**

  在 `mysql_adaptive_execution.go` 增加 Plan scope Run、Plan、sorted items locks、固定 fingerprint 与
  checkpoint metadata codec；只读并验证，不写行。

- [ ] **Step 7: 实现单 transaction happy path。**

  先定义 package-private `adaptiveExecutionLockedState` 与
  `commitAdaptiveExecutionMutationLocked(tx, req, state)`，严格按 0.5 顺序写 Event、Checkpoint、
  Items、Plan、Attempt CAS；再让 `(*threadRepository).CommitAdaptiveExecutionBoundary` 只负责
  transaction、locks/validation、调用该 helper 与完整 readback。后续 CAS rollback test 直接复用
  此时已经存在的 helper；本步不加 test-only hook、replay、MySQL concurrency 或 finalizer。

- [ ] **Step 8: 运行 initial GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-initial-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically|TestAdaptiveExecutionCheckpointMetadataRoundTrip)$' -count=1 -v 2>&1 | \
      tee "$P0A2_EVIDENCE_DIR/initial-green.log"
  ```

  Expected: 3 tests PASS，canonical fingerprint 与六类行完整 readback 一致。

## 4. Recovery lineage RED/GREEN

- [ ] **Step 1: 写 `TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops`。**

  seed A physical Run/Plan/Attempt；先用 boundary 提交 A revision 1→2 checkpoint。seed B recovery Attempt
  精确指向 A Attempt/checkpoint，提交同一 A Plan 2→3；再 seed C 精确指向 B Attempt/checkpoint，提交
  A Plan 3→4。断言 B/C checkpoint 的 physical Run/generation 分别为 B/C，Plan scope 始终为 A。

- [ ] **Step 2: 写 `TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage`。**

  table cases 精确覆盖 source 两字段半空、SourceAttemptID 与 checkpoint 不匹配、同 Thread ambient
  Attempt/checkpoint、target parent checkpoint 漂移或 self-source、source checkpoint RuntimeDeletedAt
  非 0、metadata 缺失/unknown schema/逐个 required field 缺失、metadata EventID 不存在或 Event
  Thread/Run/JournalRun/Attempt/Sequence 任一漂移、source checkpoint sequence 大于 source
  LastCommittedSequence、metadata/request Plan scope drift、checkpoint Plan revision drift、跨 Thread；
  每例 `ErrorIs(ErrAdaptiveExecutionLineageConflict)` 并完整比较所有表调用前后不变。

- [ ] **Step 3: 写 `TestAdaptiveExecutionBoundaryRejectsStaleSourceGeneration`。**

  先生成合法 source checkpoint，再只推进 source physical Run `execution_generation`；target recovery
  精确断言 `ErrAdaptiveExecutionLineageConflict`，所有目标表零变化。

- [ ] **Step 4: 运行 lineage RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  lineage_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-lineage-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionBoundary(RecoversCanonicalPlanScopeAcrossMultipleHops|RejectsNonCanonicalRecoveryLineage|RejectsStaleSourceGeneration)$' \
      -count=1 -v >"$P0A2_EVIDENCE_DIR/lineage-red.log" 2>&1 || lineage_status=$?
  test "$lineage_status" -ne 0
  if rg -n 'panic:|undefined:|build failed' "$P0A2_EVIDENCE_DIR/lineage-red.log"; then exit 1; fi
  for name in \
    TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops \
    TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage \
    TestAdaptiveExecutionBoundaryRejectsStaleSourceGeneration
  do
    rg -n -- "=== RUN   $name" "$P0A2_EVIDENCE_DIR/lineage-red.log"
  done
  rg -n -- '--- FAIL: TestAdaptiveExecutionBoundary' "$P0A2_EVIDENCE_DIR/lineage-red.log"
  ```

  Expected: recovery 尚未支持导致测试 RED；不得接受编译错误或 fixture panic。

- [ ] **Step 5: 实现 exact recovery lineage validation。**

  按 0.4 顺序锁 canonical source；解析 metadata 时拒绝 unknown/missing schema 或字段；不得扫描
  “同 Thread 最新 checkpoint”或回退到 ambient Run。

- [ ] **Step 6: 运行 lineage GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-lineage-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionBoundary(RecoversCanonicalPlanScopeAcrossMultipleHops|RejectsNonCanonicalRecoveryLineage|RejectsStaleSourceGeneration)$' \
      -count=1 -v 2>&1 | tee "$P0A2_EVIDENCE_DIR/lineage-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A2_EVIDENCE_DIR/lineage-green.log"; then exit 1; fi
  for name in \
    TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops \
    TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage \
    TestAdaptiveExecutionBoundaryRejectsStaleSourceGeneration
  do
    rg -n -- "--- PASS: $name" "$P0A2_EVIDENCE_DIR/lineage-green.log"
  done
  ```

  Expected: 三个顶层测试全部 PASS、零 SKIP/fail。

## 5. Rollback and typed conflicts RED/GREEN

- [ ] **Step 1: 写 `TestAdaptiveExecutionBoundaryRejectsInvalidTargetAttemptCursor`。**

  table cases 覆盖 `ActiveSlot=nil/0/2`、`NextSequence=0`、
  `LastCommittedSequence==NextSequence`、`LastCommittedSequence>NextSequence`、
  `NextSequence=math.MaxUint64`；active-slot case 精确
  `ErrorIs(ErrAdaptiveExecutionAttemptConflict)`，cursor case 精确
  `ErrorIs(ErrAdaptiveExecutionSequenceConflict)`，每例六类表完整不变。

- [ ] **Step 2: 写 `TestAdaptiveExecutionBoundaryRejectsInitialCheckpointParent`。**

  initial Attempt 两个 source 均 NULL，但 target Checkpoint `ParentCheckpointID != 0`；精确
  `ErrorIs(ErrAdaptiveExecutionLineageConflict)`，每类表完整不变。

- [ ] **Step 3: 写 `TestAdaptiveExecutionBoundaryRejectsMalformedMutationJSONWithoutWrites`。**

  table cases 分别破坏 Event payload、Checkpoint ChannelValues/ChannelVersions/PendingSends/Metadata、
  PlanItem Blocks/BlockedBy/Metadata；每例精确 `ErrAdaptiveExecutionBoundaryInvalid`，且六类表完整不变。

- [ ] **Step 4: 写 `TestAdaptiveExecutionBoundaryRollsBackCheckpointConflict`。**

  预置相同 checkpoint primary key；精确 `ErrorIs(ErrAdaptiveExecutionCheckpointConflict)`；Event、
  Plan、Items、Attempt 逐字段不变且 event candidate 不存在。

- [ ] **Step 5: 写 `TestAdaptiveExecutionBoundaryRejectsStalePlanRevision`。**

  request expected revision 比 locked Plan 旧；精确 `ErrAdaptiveExecutionPlanRevisionConflict`，所有表不变。

- [ ] **Step 6: 写 `TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion`。**

  table cases 覆盖两个 mutation 中第二个 update expected version 过期、expected=0 但同
  `(run_id,task_id)` 已存在、expected=0 但 ID 已存在、TaskID 大于 locked Plan HighWatermark；每例精确
  `ErrAdaptiveExecutionPlanItemVersionConflict`，其他 item、Event、Checkpoint、Plan、Attempt 也必须不变。

- [ ] **Step 7: 写 `TestAdaptiveExecutionBoundaryRejectsPlanScopeIdentityDrift`。**

  recovery lineage tuple 保持 canonical，只让 scope Run 的 `ThreadID/SpaceID/CreatorID` 或 Plan 的
  `ThreadID/SpaceID/UserID` 与 target Run 漂移；精确
  `ErrorIs(ErrAdaptiveExecutionPlanScopeConflict)`，所有表不变。

- [ ] **Step 8: 写 `TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict`。**

  通过同文件 test-only transaction 调用 locked write helper，并传入比数据库旧的 Attempt sequence
  snapshot；先执行 Event/Checkpoint/Items/Plan 写，再由最终 CAS 返回
  `ErrAdaptiveExecutionSequenceConflict`；transaction 返回后六类行全部恢复。生产 public method 不暴露
  hook，也不新增全局可变测试开关。

- [ ] **Step 9: 运行 conflict RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  conflict_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-conflict-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionBoundary(RejectsInvalidTargetAttemptCursor|RejectsInitialCheckpointParent|RejectsMalformedMutationJSONWithoutWrites|RollsBackCheckpointConflict|RejectsStalePlanRevision|RejectsStalePlanItemVersion|RejectsPlanScopeIdentityDrift|RollsBackAttemptSequenceCASConflict)$' \
      -count=1 -v >"$P0A2_EVIDENCE_DIR/conflict-red.log" 2>&1 || conflict_status=$?
  test "$conflict_status" -ne 0
  if rg -n 'panic:|undefined:|build failed' "$P0A2_EVIDENCE_DIR/conflict-red.log"; then exit 1; fi
  for name in \
    TestAdaptiveExecutionBoundaryRejectsInvalidTargetAttemptCursor \
    TestAdaptiveExecutionBoundaryRejectsInitialCheckpointParent \
    TestAdaptiveExecutionBoundaryRejectsMalformedMutationJSONWithoutWrites \
    TestAdaptiveExecutionBoundaryRollsBackCheckpointConflict \
    TestAdaptiveExecutionBoundaryRejectsStalePlanRevision \
    TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion \
    TestAdaptiveExecutionBoundaryRejectsPlanScopeIdentityDrift \
    TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict
  do
    rg -n -- "=== RUN   $name" "$P0A2_EVIDENCE_DIR/conflict-red.log"
  done
  rg -n -- '--- FAIL: TestAdaptiveExecutionBoundary' "$P0A2_EVIDENCE_DIR/conflict-red.log"
  ```

  Expected: 尚未映射 typed conflicts 或 CAS rollback 而 RED。

- [ ] **Step 10: 实现 validation/conflict mapping。**

  public method 和 CAS rollback test 复用 initial GREEN 已存在的 locked write helper。把 conversion
  failure wrap 为 boundary invalid；只按 `errors.Is`/driver duplicate classification 映射 checkpoint
  ID conflict；禁止把所有 database error 文本统一吞成 typed conflict。

- [ ] **Step 11: 运行 conflict GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-conflict-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionBoundary(RejectsInvalidTargetAttemptCursor|RejectsInitialCheckpointParent|RejectsMalformedMutationJSONWithoutWrites|RollsBackCheckpointConflict|RejectsStalePlanRevision|RejectsStalePlanItemVersion|RejectsPlanScopeIdentityDrift|RollsBackAttemptSequenceCASConflict)$' \
      -count=1 -v 2>&1 | tee "$P0A2_EVIDENCE_DIR/conflict-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A2_EVIDENCE_DIR/conflict-green.log"; then exit 1; fi
  for name in \
    TestAdaptiveExecutionBoundaryRejectsInvalidTargetAttemptCursor \
    TestAdaptiveExecutionBoundaryRejectsInitialCheckpointParent \
    TestAdaptiveExecutionBoundaryRejectsMalformedMutationJSONWithoutWrites \
    TestAdaptiveExecutionBoundaryRollsBackCheckpointConflict \
    TestAdaptiveExecutionBoundaryRejectsStalePlanRevision \
    TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion \
    TestAdaptiveExecutionBoundaryRejectsPlanScopeIdentityDrift \
    TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict
  do
    rg -n -- "--- PASS: $name" "$P0A2_EVIDENCE_DIR/conflict-green.log"
  done
  ```

  Expected: 八个顶层测试全部 PASS、零 SKIP/fail。

## 6. Exit gate and commit

- [ ] **Step 1: 运行 P0A2 exact suite。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-exact \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestValidateAdaptiveExecutionBoundaryRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt|TestValidateAdaptiveExecutionMutationRequest|TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionCheckpointMetadataRoundTrip|TestAdaptiveExecutionBoundary.*)$' \
      -count=1 -v 2>&1 | tee "$P0A2_EVIDENCE_DIR/exact-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A2_EVIDENCE_DIR/exact-green.log"; then exit 1; fi
  for name in \
    TestValidateAdaptiveExecutionBoundaryRequest \
    TestLockAdaptiveExecutionRun \
    TestLockAdaptiveExecutionAttempt \
    TestValidateAdaptiveExecutionMutationRequest \
    TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON \
    TestAdaptiveExecutionCheckpointMetadataRoundTrip \
    TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically \
    TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops \
    TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage \
    TestAdaptiveExecutionBoundaryRejectsStaleSourceGeneration \
    TestAdaptiveExecutionBoundaryRejectsInvalidTargetAttemptCursor \
    TestAdaptiveExecutionBoundaryRejectsInitialCheckpointParent \
    TestAdaptiveExecutionBoundaryRejectsMalformedMutationJSONWithoutWrites \
    TestAdaptiveExecutionBoundaryRollsBackCheckpointConflict \
    TestAdaptiveExecutionBoundaryRejectsStalePlanRevision \
    TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion \
    TestAdaptiveExecutionBoundaryRejectsPlanScopeIdentityDrift \
    TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict
  do
    rg -n -- "--- PASS: $name" "$P0A2_EVIDENCE_DIR/exact-green.log"
  done
  ```

  Expected: P0A1 + P0A2 全部顶层测试 PASS、零 SKIP/fail。

- [ ] **Step 2: 运行 compile、format 与 allowlist gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a2-compile \
    go test -p 1 -run '^$' ./domain/agentthread/repository -count=1 2>&1 | \
    tee "$P0A2_EVIDENCE_DIR/compile.log"
  cd ..
  for file in \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
  do
    test -f "$file"
  done
  gofmt -d \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go | \
    tee "$P0A2_EVIDENCE_DIR/gofmt.log"
  test ! -s "$P0A2_EVIDENCE_DIR/gofmt.log"
  git diff --check
  git diff --diff-filter=D --exit-code -- \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
  test "$(git diff --name-only | wc -l | tr -d ' ')" = 3
  test -z "$(git diff --name-only | grep -Ev \
    '^(backend/domain/agentthread/repository/adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution_test.go)$' || true)"
  test -z "$(git diff --cached --name-only)"
  ```

  Expected: compile PASS、格式与 whitespace clean、只有 exact 3 files 变化。

- [ ] **Step 3: 请求 spec review 与 quality review。**

  两个 reviewer 独立核 frozen contract、lineage、多跳、metadata、锁序、CAS、逐表 rollback、typed errors
  和旧 P0A1 回归；任一 P0/P1 必须修复并重新运行 Step 1～2，P2 必须显式裁决。

- [ ] **Step 4: 暂存 exact 3 files 并提交。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  git add -- \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
  test "$(git diff --cached --name-only | wc -l | tr -d ' ')" = 3
  test -z "$(git diff --cached --name-only | grep -Ev \
    '^(backend/domain/agentthread/repository/adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution_test.go)$' || true)"
  git diff --cached --diff-filter=D --exit-code
  git commit -m 'feat: commit adaptive execution state atomically'
  test "$(git log -1 --format=%s)" = 'feat: commit adaptive execution state atomically'
  git rev-parse HEAD > "$P0A2_EVIDENCE_DIR/head.sha"
  ```

  Expected: 一个 code-only commit，未带 docs、migration、ADK、IDL 或 frontend。

- [ ] **Step 5: 机器冻结 P0A2 evidence。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  P0A2_BASE_SHA="$(cat "$P0A2_EVIDENCE_DIR/base.sha")"
  P0A2_HEAD_SHA="$(cat "$P0A2_EVIDENCE_DIR/head.sha")"
  test "$(git rev-parse HEAD)" = "$P0A2_HEAD_SHA"
  test "$(git rev-parse HEAD^)" = "$P0A2_BASE_SHA"
  test -z "$(git status --porcelain)"
  test "$(git diff --name-only "$P0A2_BASE_SHA..$P0A2_HEAD_SHA" | wc -l | tr -d ' ')" = 3
  git diff --diff-filter=D --exit-code "$P0A2_BASE_SHA..$P0A2_HEAD_SHA"
  test -z "$(git diff --name-only "$P0A2_BASE_SHA..$P0A2_HEAD_SHA" | grep -Ev \
    '^(backend/domain/agentthread/repository/adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution_test.go)$' || true)"
  printf 'P0A2 SQLite proves atomic Event/Checkpoint/PlanItem/Plan/Attempt mutation and canonical multi-hop lineage; MySQL concurrent single-winner remains P0A3.' \
    > "$P0A2_EVIDENCE_DIR/remaining-risk.txt"
  printf 'PACKET=P0A2\nSTATUS=PASS\nBASE_SHA=%s\nHEAD_SHA=%s\n' \
    "$P0A2_BASE_SHA" "$P0A2_HEAD_SHA" > "$P0A2_EVIDENCE_DIR/state.env"
  (cd "$P0A2_EVIDENCE_DIR" && shasum -a 256 \
    entry-green.log contract-red.log contract-green.log initial-red.log initial-green.log \
    lineage-red.log lineage-green.log conflict-red.log conflict-green.log exact-green.log \
    compile.log gofmt.log base.sha head.sha state.env remaining-risk.txt > evidence.sha256)
  (cd "$P0A2_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  ```

  Expected: state=`PASS`、clean HEAD、exact 3-file code diff、全部日志 checksum 通过。

## 7. Stop boundary

P0A2 PASS 后才从真实 `P0A2_HEAD_SHA` 生成唯一 P0A3 plan 文件，并按 master 的 docs-only handoff
单独提交。P0A2 不运行 disposable MySQL race，不修改 migration，不加载 ADK coordinator、finalizer、IDL、
frontend 或执行模式退休代码；任何需要这些能力的实现都立即停止，P0A3～P5 保持 `locked`。
