# Workbench 自适应执行 MVP P0B Replay / Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 执行。Steps use checkbox (`- [ ]`) syntax for tracking。
> 只在干净的
> `/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp`
> 工作树执行。

**Goal:** 在不新增迁移、不接 application/ADK/HTTP/UI 的前提下，让 adaptive boundary 对重新提交的
完整 Commit 请求验证持久化 boundary 语义并实现 lost-response 幂等重放，让 recovery target 只通过自身持久化的
`SourceAttemptID + SourceCheckpointID` 精确恢复来源 boundary；任何 payload、mutation、lineage、
Plan 或 Item 漂移都 fail closed。

**Architecture:** `CommitAdaptiveExecutionBoundary` 仍是唯一写入口。它先锁 physical Run 和
Attempt，再按 `(journal_run_id, attempt_id, idempotency_key)` 做 locking/current read；已存在的
exact 请求返回原结果，不存在才校验 lease/cursor 并写入。新增的
`ReadAdaptiveExecutionRecoverySource` 只接收目标 Attempt 身份，repository 自己读取其 Source 指针，
不接受 caller 自报 checkpoint。Checkpoint 的 server-owned metadata 升级到 v2，保存 event
fingerprint 和 1～32 个排序后的 Item refs；这只扩展现有 JSON，不增加表。公共 `ListRunEvents` 和
latest checkpoint 永远不是 authority。

**Tech Stack:** Go、GORM、SQLite、现有 RunEvent/Checkpoint/Plan/PlanItem/Attempt/Run 表、
Workbench execution graph verifier。

---

## 0. Frozen contract

### 0.1 Base and exact scope

- P0A3 implementation HEAD 必须为
  `a1df1789b0b60c0916505711bcc1e5a1fa1410cd`，P0A aggregate evidence 必须 checksum PASS。
- 本文件先作为唯一 docs-only commit 落在 P0A3 HEAD 之上；implementation commit 的 parent 必须
  是该 docs commit。
- P0B 不修改 migration、`repository.go`、`mysql.go`、application、ADK、IDL、frontend、public
  projection、finalizer 或任何模式字段。P0C/P0D/P1M 保持 locked。
- P0D 才做 crash-after-commit、lease takeover、cancel-vs-success 的最终真实 MySQL 四项 machine
  gate（另含 concurrent PlanItem mutation）；P0B 不重复扩张 MySQL fixture。

**Implementation exact-six allowlist:**

1. `backend/domain/agentthread/repository/adaptive_execution.go`
2. `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
3. `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`
4. `docs/superpowers/context/workbench-execution-chain.md`
5. `docs/superpowers/context/workbench-execution-graph.json`
6. `scripts/workbench-execution-graph/contract.mjs`

### 0.2 Exported contract

`adaptive_execution.go` 增加：

```go
var (
	ErrAdaptiveExecutionReplayConflict   = errors.New("adaptive execution replay conflict")
	ErrAdaptiveExecutionRecoveryConflict = errors.New("adaptive execution recovery conflict")
)

type AdaptiveExecutionBoundaryAuthority struct {
	ThreadID            int64
	ExecutionRunID      int64
	ExecutionGeneration uint64
	JournalRunID        int64
	AttemptID           string
	SourceAttemptID     *string
	SourceCheckpointID  *int64
	EventID             int64
	EventSequence       uint64
	IdempotencyKey      string
	CheckpointID        int64
	PlanScopeRunID      int64
	PlanRevision        int64
	PlanItemFingerprint string
}

type ReadAdaptiveExecutionRecoverySourceRequest struct {
	ThreadID        int64
	JournalRunID    int64
	TargetAttemptID string
}
```

`CommitAdaptiveExecutionBoundaryResult` 增加以下 exact value fields，沿用现有 Journal boundary
约定：首写=false，existing exact replay 和 recovery read=true。

```go
Authority AdaptiveExecutionBoundaryAuthority
Replayed  bool
```

`AdaptiveExecutionRepository` 只新增：

```go
ReadAdaptiveExecutionRecoverySource(
	ctx context.Context,
	req ReadAdaptiveExecutionRecoverySourceRequest,
) (*CommitAdaptiveExecutionBoundaryResult, error)
```

该 read request 只做 identity validation；没有 lease owner/token、没有 checkpoint selector、没有产品
mode 或 shape enum。

### 0.3 Checkpoint metadata v2

P0A primitive 尚未 production wired，因此 P0B 直接把 server namespace 从
`workbench-adaptive-boundary.v1` 升为 `workbench-adaptive-boundary.v2`，不保留无法真实 recovery 的
v1 兼容分支。

```go
type adaptiveExecutionCheckpointItemRef struct {
	ID      int64 `json:"id"`
	TaskID  int64 `json:"task_id"`
	Version int64 `json:"version"`
}
```

现有 metadata 保留，并新增：

```text
source_attempt_id / source_checkpoint_id = required nullable pair copied from the boundary Attempt
event_idempotency_key = non-empty authoritative event key
event_fingerprint = 64 lowercase hex
item_refs = 1..32 refs, TaskID ASC, positive ID/TaskID/Version, unique ID and TaskID
```

- Source pair 两个 JSON key 都必须存在；initial 同为 null，recovery 同为 non-null。Commit 时从 locked
  Attempt 复制，replay/recovery 时与当前 Attempt exact，并要求 checkpoint parent 为 null pair时0、
  non-null pair时等于 source checkpoint；这样单改任一 source pointer都会 fail closed。
- `event_idempotency_key` 与 Event PO 的 tuple key exact，并由 decoder 按 1～191 字节拒空/限长；recovery 用
  `(journal_run_id, attempt_id, event_idempotency_key)` 读取权威 Event，再核 EventID/sequence。
- Event fingerprint 对固定 struct 做 SHA-256：Event ID/ThreadID/RunID/type/canonical payload/created_at
  加 authoritative JournalRunID/AttemptID/sequence/idempotency key。JSON 先用 `UseNumber`、单值 EOF、
  deterministic marshal canonicalize；不能比较数据库原始 JSON bytes。
- Item fingerprint 继续覆盖本次 mutation 的完整 post-image；`item_refs` 只负责定位同一组 rows，不
  冒充历史快照。
- Metadata decoder 继续 `DisallowUnknownFields`，严格拒绝 v1、缺字段、乱序/重复/越界 refs、非法
  digest。
- Plan 或 referenced Items 后续已推进时直接 recovery/replay conflict；零迁移模型没有历史 Plan
  snapshot，禁止返回当前新状态冒充旧结果。

### 0.4 Commit idempotency and lock order

`CommitAdaptiveExecutionBoundary` 固定顺序：

1. pure request validation；
2. transaction 内按 `ExecutionRunID` 锁 physical Run row，但暂不校验 running/lease；
3. 按 `(JournalRunID, AttemptID)` 锁 Attempt row，但暂不校验 active cursor；
4. 在 Run/Attempt 锁之后，对 authoritative event tuple 做 `SELECT ... FOR UPDATE` current read；
5. tuple 存在：直接加载 v2 checkpoint/Plan/refs，比较完整 request 的 Event、Checkpoint base、Plan
   mutation 和 normalized Item post-image；exact 则返回原 result，任一漂移 wrap
   `ErrAdaptiveExecutionReplayConflict`；不再检查旧 lease 或 active status；
6. tuple 不存在：才校验现有 Run lease/cancel、Attempt active/cursor、lineage、Plan/Items，并走 P0A
   原子写；
7. 首写和 replay 共用同一个 PO loader/result builder，返回相同事实与 Authority；只允许
   `Replayed` 从 false 变 true。

Tuple missing 且 target 有 recovery lineage 时，P0B 同时修复现有 source lock inversion：先对 source
Attempt 做一次不加锁的 identity discovery 取得 `ExecutionRunID`，再按 source Run `FOR UPDATE` → source
Attempt `FOR UPDATE` → exact checkpoint → Event 的顺序锁定并重验 discovery。禁止 source Attempt 持锁后
再取 source Run 锁；否则会与同一 source boundary 的 Run→Attempt replay 形成死锁环。

这样 concurrent identical writers 都先竞争 Run row；loser 等待 winner commit 后以 current read 看见
tuple，不会先锁 missing secondary key，也不会落到 stale Plan/duplicate error。禁止 transaction 外
快照、sleep、retry loop、advisory lock、test hook 或全局 failpoint。

### 0.5 Exact replay comparison

- Event tuple 是唯一入口；Event base identity、type、created_at 和 canonical payload 必须与 request
  exact，metadata event fingerprint 必须同时匹配实际 PO。
- Replay 的 durable semantic field-by-field compare 明确排除 `LeaseOwner`/`LeaseToken`：它们仍须通过 pure nonblank
  validation，但 tuple 已存在后不参与 exact compare，也不要求旧 lease 仍有效。Thread/Run/generation、
  Journal/Attempt/key、Now、Event、Checkpoint 和 Plan mutation 全部属于 durable compare。
- Pure validator 对 `IdempotencyKey` 执行 nonblank + `len([]byte(key))<=191`，对 Commit `AttemptID` 和
  recovery `TargetAttemptID` 执行 nonblank + `len([]byte(id))<=64`；invalid tables 必须有边界外案例。
  `TrimSpace` 只判断空值，tuple/metadata/compare 始终使用 caller 的原始 exact string，不静默改 key。
- Checkpoint 只按 request 的 exact ID 读取；Thread/Run/runtime/parent/channel JSON/user metadata 必须
  exact。`ChannelValues`、`ChannelVersions`、`PendingSends` 及剥离 server-owned
  `adaptive_execution` 后的 user Metadata 全部使用与 Event 相同的 UseNumber + single EOF +
  deterministic marshal 做 JSON semantic compare；对象键序/空白等价，数组顺序和值变化不等价。
- Attempt identity、execution Run/generation 和 source pointers 必须与 checkpoint lineage一致；event
  sequence 必须 `<=` Attempt 当前 `LastCommittedSequence`。Result 的 `LastCommittedSequence` 始终返回
  metadata EventSequence（原 boundary 值），不能把 Attempt 后续推进的 cursor 冒充原响应。
- Plan current revision 必须同时等于 metadata 和 request `NextRevision`。
- Items 只按 metadata refs 读取并按 TaskID ASC；每行 ID/RunID/TaskID/Version exact，重算 fingerprint
  必须等于 metadata。Replay 还要把 request NextItem 按 P0A 写入语义规范化：create 的
  CreatedAt/UpdatedAt=`Now`，update 保留已落库 CreatedAt 且 UpdatedAt=`Now`，然后完整比较。
- `GetCheckpoint`、`GetPlan`、`ListPlanItems` 保持兼容读取；禁止调用 public `ListRunEvents` 或扫描
  latest checkpoint。

### 0.6 Recovery source read

`ReadAdaptiveExecutionRecoverySource` 在单个 MySQL REPEATABLE READ consistent-snapshot transaction 中
只做普通 `SELECT`，不取 `FOR UPDATE`：recovery 若先锁 Attempt 再锁 source Run，会与 Commit 的
Run→Attempt 顺序形成锁环；只读 snapshot 要么看到完整旧状态，要么 fail closed 后由 caller 重试。
MySQL 使用 `sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}`；SQLite 测试使用普通只读
transaction，避免 driver 不支持 read-only TxOptions。

1. 按 `(JournalRunID, TargetAttemptID)` 读取目标 Attempt，要求 Thread/Journal exact；目标可 active 或
   terminal；
2. 要求 SourceAttemptID 与 SourceCheckpointID 都非空、非 self；repository 自己取得这两个值；
3. 按同 Journal + SourceAttemptID 读取 source Attempt；
4. 只按目标 Attempt 的 SourceCheckpointID 读取 checkpoint，禁止 ambient/latest；核 Thread、source
   physical Run、RuntimeDeletedAt、parent lineage；
5. strict decode v2；核 source Attempt identity、execution Run/generation、event sequence
   `<= LastCommittedSequence`；
6. 按 metadata 的 event tuple 读取 authoritative event，核 EventID/sequence 与 event fingerprint；
7. 读取 source physical Run、Plan scope Run、Plan 和 item refs；核同 Thread/Space/Creator、current Plan
   revision、refs 与 item fingerprint；
8. 返回来源 boundary 的 Event/Checkpoint/Plan/touched Items/Authority。A→B→C 中 C 的 selector 指向
   B，结果 Authority.AttemptID=B，Authority.Source* 保留 B→A，PlanScopeRunID 继续是 canonical A。

任一 missing/drift wrap `ErrAdaptiveExecutionRecoveryConflict`，数据库六表前后完全不变。

### 0.7 Behavior matrix and authority docs

必须 fresh 证明：

- lost response：同 repository Commit、新 repository Commit 都返回首次 Event/Checkpoint/Plan/Items/
  Authority/LCS，`Replayed=true`，六表零新增；
- payload/mutation drift：同 tuple 改 Event/Checkpoint/Plan/Item 任一事实均 replay conflict；
- duplicate decision：同 key 不同 decision payload conflict；
- duplicate verification：exact retry/repository reload 返回原 verification；
- degraded projection：Attempt degraded、Journal projection columns NULL 时，authoritative tuple 仍 replay；
- exact recovery：存在更晚 decoy checkpoint 仍只读取 target SourceCheckpointID；
- multi-hop A→B→C 保留 canonical Plan scope；
- authority drift：target/source pointer、sequence、generation、event、checkpoint、Plan revision、ref row、
  fingerprint 任一漂移 recovery conflict；
- compatibility readers 与 authoritative result 一致，但没有一个 authority assertion经 public
  `ListRunEvents` 得出。

同步 execution chain 和 graph exclusion：P0A atomic commit + P0B replay/recovery readback 已实现，
仍是 `implemented_not_wired`，不得增加 application/ADK production edge。Graph JSON 变化后按 verifier
actual 更新 `WORKBENCH_PROFILE_STRUCTURE_DIGEST`，不放宽 contract。

---

## 1. Docs-only entry gate

- [ ] **Step 1: 验证 docs commit、P0A evidence 和线性 parent。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test "$(git rev-parse HEAD^)" = a1df1789b0b60c0916505711bcc1e5a1fa1410cd
  test "$(git show -s --format=%s HEAD)" = 'docs: add adaptive replay recovery packet'
  test "$(git diff-tree --no-commit-id --name-status -r HEAD)" = $'A\tdocs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0b-recovery.md'
  test -z "$(git status --porcelain)"
  for packet in p0a1 p0a2 p0a3; do
    (cd "/private/tmp/workbench-adaptive-mvp-${packet}" && shasum -a 256 -c evidence.sha256)
  done
  (cd /private/tmp/workbench-adaptive-mvp-p0a && shasum -a 256 -c evidence.sha256)
  grep -Fx 'STATUS=PASS' /private/tmp/workbench-adaptive-mvp-p0a/state.env
  ```

- [ ] **Step 2: 初始化 fail-closed evidence 目录。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  test ! -e /private/tmp/workbench-adaptive-mvp-p0b
  mkdir /private/tmp/workbench-adaptive-mvp-p0b
  git rev-parse HEAD > /private/tmp/workbench-adaptive-mvp-p0b/base.sha
  printf '%s\n' 'PACKET=P0B' 'STATUS=active' > /private/tmp/workbench-adaptive-mvp-p0b/state.env
  ```

---

## 2. Task A: v2 metadata and exported contract RED/GREEN

- [ ] **Step 1: 写 contract RED。**

  在 `mysql_adaptive_execution_test.go` 增加或扩展：

  ```go
  func TestAdaptiveExecutionCheckpointMetadataV2RoundTrip(t *testing.T)
  func TestValidateAdaptiveExecutionRecoverySourceRequest(t *testing.T)
  // Extend existing TestValidateAdaptiveExecutionBoundaryRequest.
  ```

  Round-trip 必须覆盖 source nullable pair、event fingerprint、2 个逆序输入后编码为 TaskID ASC 的 refs、所有 invalid
  schema/digest/ref cases；同时把 P0A 既有 metadata 断言从 v1 改为 v2，并把新增字段加入 required
  field drift table。Recovery validator 覆盖 success、zero Thread/Journal、空白及65-byte TargetAttemptID；
  Commit validator补65-byte AttemptID和192-byte IdempotencyKey。

- [ ] **Step 2: 运行 RED。**

  ```zsh
  set -o pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  go test -p 1 ./domain/agentthread/repository \
    -run '^(TestAdaptiveExecutionCheckpointMetadataV2RoundTrip|TestValidateAdaptiveExecutionRecoverySourceRequest|TestValidateAdaptiveExecutionBoundaryRequest)$' \
    -count=1 -v 2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0b/contract-red.log
  contract_status=${pipestatus[1]}
  test "$contract_status" -ne 0
  ! grep -Eq 'panic:|fixture.*failed' /private/tmp/workbench-adaptive-mvp-p0b/contract-red.log
  ```

- [ ] **Step 3: 实现最小 contract、v2 metadata 和 pure fingerprints。**

  只改两个 production files；保留 P0A validators 语义。实现 event fingerprint、refs normalize/strict
  validation、Authority/result mapping 和 recovery request validator。为保持 intermediate package
  GREEN，interface method 同时落一个最小 fail-closed stub：合法请求固定 wrap RecoveryConflict 且零 DB
  访问；Task C 的行为 RED 再驱动真实 reader。尚不实现 Commit replay。

- [ ] **Step 4: 运行同 regex GREEN。**

  ```zsh
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  go test -p 1 ./domain/agentthread/repository \
    -run '^(TestAdaptiveExecutionCheckpointMetadataV2RoundTrip|TestValidateAdaptiveExecutionRecoverySourceRequest|TestValidateAdaptiveExecutionBoundaryRequest)$' \
    -count=1 -v 2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0b/contract-green.log
  test "$(grep -c '^--- PASS: TestAdaptiveExecutionCheckpointMetadataV2RoundTrip ' /private/tmp/workbench-adaptive-mvp-p0b/contract-green.log)" = 1
  test "$(grep -c '^--- PASS: TestValidateAdaptiveExecutionRecoverySourceRequest ' /private/tmp/workbench-adaptive-mvp-p0b/contract-green.log)" = 1
  test "$(grep -c '^--- PASS: TestValidateAdaptiveExecutionBoundaryRequest ' /private/tmp/workbench-adaptive-mvp-p0b/contract-green.log)" = 1
  ! grep -E '^--- (FAIL|SKIP):' /private/tmp/workbench-adaptive-mvp-p0b/contract-green.log
  ```

---

## 3. Task B: Commit lost-response replay RED/GREEN

- [ ] **Step 1: 写行为 RED。**

  ```go
  func TestAdaptiveExecutionBoundaryReplaysLostResponseWithoutWrites(t *testing.T)
  func TestAdaptiveExecutionBoundaryRejectsReplayPayloadAndMutationDrift(t *testing.T)
  func TestAdaptiveExecutionBoundaryDuplicateDecisionConflicts(t *testing.T)
  func TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload(t *testing.T)
  func TestAdaptiveExecutionBoundaryReplaySurvivesDegradedProjection(t *testing.T)
  func TestAdaptiveExecutionBoundaryChecksReplayTupleAfterRunAndAttemptLocks(t *testing.T)
  func TestAdaptiveExecutionBoundaryLocksSourceRunBeforeSourceAttempt(t *testing.T)
  ```

  Lost-response 先 commit，再把 DB Run 推到 terminal、替换/失效 lease、设置 cancel，把 Attempt 推到
  terminal + ActiveSlot nil + LCS高于原event；保存此时六表 snapshot 后，对同 repository、新 repository 各
  Commit 一次；首写 `Replayed=false`、两次 retry=true，返回原 metadata EventSequence 而非新LCS，
  其余结果字段 deep equal，除上述显式状态推进外没有写入。Drift table 至少逐一改变 Event ID/type/payload、Checkpoint ID/channel/user
  metadata、Plan next revision、Item ID/TaskID/subject/version、execution generation。Checkpoint 四个
  JSON 字段各有“等价格式可 replay、语义变化 conflict”覆盖。另断言只改变
  LeaseOwner/LeaseToken 仍可 replay，因为它们是首写 fence而非持久化 boundary 语义。Decision/verification 使用真实 event type。
  Degraded case 核 projection columns NULL、authority tuple完整。
  Lock-order test 用 sqlmock 调 public Commit：`BEGIN` 后第一条 row query 必须是 Run `FOR UPDATE`，
  第二条是 Attempt `FOR UPDATE`，第三条才是 event tuple `FOR UPDATE`；第三条返回 sentinel 后 rollback，
  禁止直接测试 private helper。第二个 sqlmock case 构造 tuple missing + recovery lineage，要求 source
  identity discovery 后第一条 source row lock 是 source Run，再锁 source Attempt。真实 MySQL
  crash-after-commit retry visibility 由 P0D 证明；recovery RR read-only snapshot 由本包 TxOptions 合同和
  只读测试覆盖，不宣称 P0D 额外做 snapshot interleaving stress。

- [ ] **Step 2: 运行 RED。**

  ```zsh
  set -o pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  go test -p 1 ./domain/agentthread/repository \
    -run '^TestAdaptiveExecutionBoundary(ReplaysLostResponseWithoutWrites|RejectsReplayPayloadAndMutationDrift|DuplicateDecisionConflicts|DuplicateVerificationReplaysAfterRepositoryReload|ReplaySurvivesDegradedProjection|ChecksReplayTupleAfterRunAndAttemptLocks|LocksSourceRunBeforeSourceAttempt)$' \
    -count=1 -v 2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0b/replay-red.log
  replay_status=${pipestatus[1]}
  test "$replay_status" -ne 0
  ! grep -Eq 'panic:|build failed' /private/tmp/workbench-adaptive-mvp-p0b/replay-red.log
  ```

- [ ] **Step 3: 实现 authoritative loader 和 Commit recheck。**

  按 0.4/0.5 重构 row lock 与 fence validation，但保留 P0A1 `lockAdaptiveExecutionRun`、
  `lockAdaptiveExecutionAttempt` 的外部测试语义。Tuple missing 使用 package-private sentinel；存在后的任一
  drift 统一 wrap ReplayConflict。首写与 replay 共用 loader/result builder。

- [ ] **Step 4: 运行同 regex GREEN。**

  ```zsh
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  replay_regex='^TestAdaptiveExecutionBoundary(ReplaysLostResponseWithoutWrites|RejectsReplayPayloadAndMutationDrift|DuplicateDecisionConflicts|DuplicateVerificationReplaysAfterRepositoryReload|ReplaySurvivesDegradedProjection|ChecksReplayTupleAfterRunAndAttemptLocks|LocksSourceRunBeforeSourceAttempt)$'
  go test -p 1 ./domain/agentthread/repository -run "$replay_regex" -count=1 -v 2>&1 \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/replay-green.log
  for test_name in \
    TestAdaptiveExecutionBoundaryReplaysLostResponseWithoutWrites \
    TestAdaptiveExecutionBoundaryRejectsReplayPayloadAndMutationDrift \
    TestAdaptiveExecutionBoundaryDuplicateDecisionConflicts \
    TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload \
    TestAdaptiveExecutionBoundaryReplaySurvivesDegradedProjection \
    TestAdaptiveExecutionBoundaryChecksReplayTupleAfterRunAndAttemptLocks \
    TestAdaptiveExecutionBoundaryLocksSourceRunBeforeSourceAttempt; do
    test "$(grep -c "^--- PASS: ${test_name} " /private/tmp/workbench-adaptive-mvp-p0b/replay-green.log)" = 1
  done
  ! grep -E '^--- (FAIL|SKIP):' /private/tmp/workbench-adaptive-mvp-p0b/replay-green.log
  ```

---

## 4. Task C: Exact recovery source RED/GREEN

- [ ] **Step 1: 写 recovery RED。**

  ```go
  func TestAdaptiveExecutionRecoveryReadUsesExactSourceCheckpoint(t *testing.T)
  func TestAdaptiveExecutionRecoveryReadRecoversCanonicalPlanScopeAcrossMultipleHops(t *testing.T)
  func TestAdaptiveExecutionRecoveryReadRejectsAuthorityDriftWithoutWrites(t *testing.T)
  func TestAdaptiveExecutionRecoveryReadMatchesCompatibilityReaders(t *testing.T)
  func TestAdaptiveExecutionRecoveryReadSelectsSafeTransactionOptions(t *testing.T)
  ```

  Exact case 建 A boundary、B target，再插同 Thread 更晚 decoy checkpoint；必须返回 A exact source。
  Multi-hop 建 A→B→C，C selector 返回 B boundary 且 Plan scope仍A。Drift table 覆盖 partial/self source、
  checkpoint missing/cross-thread/deleted/parent drift、仅改 source AttemptID、仅改 source CheckpointID、
  source LCS、source Run generation、event tuple/fingerprint、
  Plan scope/revision、ref missing/task/version、item fingerprint；每例六表完整 snapshot 相等。
  Transaction-options test 要求 MySQL 映射为 `LevelRepeatableRead + ReadOnly=true`，SQLite 映射为 nil；
  production reader 必须调用同一 pure mapping helper。

- [ ] **Step 2: 运行 RED。**

  ```zsh
  set -o pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  go test -p 1 ./domain/agentthread/repository \
    -run '^TestAdaptiveExecutionRecoveryRead(UsesExactSourceCheckpoint|RecoversCanonicalPlanScopeAcrossMultipleHops|RejectsAuthorityDriftWithoutWrites|MatchesCompatibilityReaders|SelectsSafeTransactionOptions)$' \
    -count=1 -v 2>&1 | tee /private/tmp/workbench-adaptive-mvp-p0b/recovery-red.log
  recovery_status=${pipestatus[1]}
  test "$recovery_status" -ne 0
  ! grep -Eq 'panic:|build failed' /private/tmp/workbench-adaptive-mvp-p0b/recovery-red.log
  ```

- [ ] **Step 3: 实现 recovery reader。**

  只实现 0.6 的 strict PO flow；禁止调用 public ListRunEvents/ListCheckpoints 或 latest helper。所有关联
  rows 在同一 consistent snapshot 中普通读取，不得取 row lock，typed error 只暴露 RecoveryConflict。

- [ ] **Step 4: 运行 recovery GREEN 与 P0A regression。**

  先运行 recovery exact GREEN：

  ```zsh
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  recovery_regex='^TestAdaptiveExecutionRecoveryRead(UsesExactSourceCheckpoint|RecoversCanonicalPlanScopeAcrossMultipleHops|RejectsAuthorityDriftWithoutWrites|MatchesCompatibilityReaders|SelectsSafeTransactionOptions)$'
  go test -p 1 ./domain/agentthread/repository -run "$recovery_regex" -count=1 -v 2>&1 \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/recovery-green.log
  for test_name in \
    TestAdaptiveExecutionRecoveryReadUsesExactSourceCheckpoint \
    TestAdaptiveExecutionRecoveryReadRecoversCanonicalPlanScopeAcrossMultipleHops \
    TestAdaptiveExecutionRecoveryReadRejectsAuthorityDriftWithoutWrites \
    TestAdaptiveExecutionRecoveryReadMatchesCompatibilityReaders \
    TestAdaptiveExecutionRecoveryReadSelectsSafeTransactionOptions; do
    test "$(grep -c "^--- PASS: ${test_name} " /private/tmp/workbench-adaptive-mvp-p0b/recovery-green.log)" = 1
  done
  ! grep -E '^--- (FAIL|SKIP):' /private/tmp/workbench-adaptive-mvp-p0b/recovery-green.log
  ```

  再运行 P0A/P0B focused regression：

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  go test -p 1 ./domain/agentthread/repository \
    -run '^(TestValidateAdaptiveExecutionBoundaryRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt|TestValidateAdaptiveExecutionMutationRequest|TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionCheckpointMetadata.*|TestAdaptiveExecutionBoundary(Commits|Recovers|Rejects|RollsBack|Replays|Duplicate|ReplaySurvives|Checks|Locks).*|TestAdaptiveExecutionRecoveryRead.*)$' \
    -count=1 -v | tee /private/tmp/workbench-adaptive-mvp-p0b/sqlite-green.log
  ! grep -E -- '--- (FAIL|SKIP):' /private/tmp/workbench-adaptive-mvp-p0b/sqlite-green.log
  ```

---

## 5. Task D: Authority docs and graph

- [ ] **Step 1: 更新两份 authority 文件。**

  Chain 写清 P0A commit + P0B idempotent replay/recovery source read 已实现、metadata v2、direct private PO
  authority、无 public list/latest、仍无 application/ADK caller。Graph 只更新
  `exclude.unwired_adaptive_boundary` reason/evidence；不得加 production edge。

- [ ] **Step 2: 更新 pinned digest 并运行 gate。**

  保留旧 digest 先跑 verify，只从唯一 `canonical_profile_structure_mismatch ... actual` 后的 64 位小写
  hex 取得新值，
  更新 `contract.mjs` 常量，不放宽 validator。然后：

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  node --test scripts/workbench-execution-graph.test.mjs \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-contract.log
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-verify.log
  node scripts/workbench-execution-graph.mjs build \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-build.log
  node scripts/workbench-execution-graph.mjs verify-derived \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-derived.log
  ```

  build 若受 sandbox 限制，只对 exact Node command 请求权限；派生目录不得进入 git status。

---

## 6. Final verification, reviews, commit and evidence

- [ ] **Step 1: fresh package、compile、format 和 static gates。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  export GOCACHE=/private/tmp/workbench-adaptive-mvp-p0b-go-cache
  env -u COZE_AGENTTHREAD_TEST_MYSQL_DSN -u COZE_AGENTTHREAD_TEST_ALLOW_DDL \
    go test -p 1 ./domain/agentthread/repository -count=1 -timeout=240s \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/package-green.log
  go test -p 1 ./domain/agentthread/repository -run '^$' -count=1 \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/compile.log
  gofmt -d \
    domain/agentthread/repository/adaptive_execution.go \
    domain/agentthread/repository/mysql_adaptive_execution.go \
    domain/agentthread/repository/mysql_adaptive_execution_test.go \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/gofmt.log
  test ! -s /private/tmp/workbench-adaptive-mvp-p0b/gofmt.log
  cd ..
  git diff --check
  ! rg -n 'ListRunEvents|ListCheckpoints|latestRecoverableCheckpoint|loadJournalRecoveryCheckpoint' \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go
  ```

- [ ] **Step 2: exact-six status gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git status --porcelain=v1 | LC_ALL=C sort > /private/tmp/workbench-adaptive-mvp-p0b/status.txt
  diff -u <(cat <<'EXPECTED'
   M backend/domain/agentthread/repository/adaptive_execution.go
   M backend/domain/agentthread/repository/mysql_adaptive_execution.go
   M backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
   M docs/superpowers/context/workbench-execution-chain.md
   M docs/superpowers/context/workbench-execution-graph.json
   M scripts/workbench-execution-graph/contract.mjs
  EXPECTED
  ) /private/tmp/workbench-adaptive-mvp-p0b/status.txt
  test -z "$(git diff --cached --name-only)"
  ```

- [ ] **Step 3: 双重只读审查。**

  Spec reviewer 逐条核 0.1～0.7；quality reviewer 独立 fresh 跑 SQLite/package/graph，核 direct PO
  authority、v2 refs、event fingerprint、无 public list/latest、无写 recovery、无新 lock cycle。任一 P0/P1
  必须先修复并重验；P2 记录但不得掩盖 recovery 正确性。

- [ ] **Step 4: stage、cached gate 和 implementation commit。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git add \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs
  git diff --cached --check
  test "$(git diff --cached --name-only | wc -l | tr -d ' ')" = 6
  git commit -m 'feat: add adaptive boundary replay recovery'
  test "$(git show -s --format=%s HEAD)" = 'feat: add adaptive boundary replay recovery'
  ```

  禁止 `--no-verify`。Hook dependency 缺失时只用仓库已有依赖路径，不安装或改 lockfile。

- [ ] **Step 5: post-commit graph gate and clean status。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git rev-parse HEAD > /private/tmp/workbench-adaptive-mvp-p0b/head.sha
  test "$(git rev-parse HEAD^)" = "$(cat /private/tmp/workbench-adaptive-mvp-p0b/base.sha)"
  diff -u <(printf '%s\n' \
    $'M\tbackend/domain/agentthread/repository/adaptive_execution.go' \
    $'M\tbackend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    $'M\tbackend/domain/agentthread/repository/mysql_adaptive_execution_test.go' \
    $'M\tdocs/superpowers/context/workbench-execution-chain.md' \
    $'M\tdocs/superpowers/context/workbench-execution-graph.json' \
    $'M\tscripts/workbench-execution-graph/contract.mjs') \
    <(git diff-tree --no-commit-id --name-status -r HEAD | LC_ALL=C sort)
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-postcommit-verify.log
  node scripts/workbench-execution-graph.mjs build \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-postcommit-build.log
  node scripts/workbench-execution-graph.mjs verify-derived \
    | tee /private/tmp/workbench-adaptive-mvp-p0b/graph-postcommit-derived.log
  test -z "$(git status --porcelain)"
  ```

- [ ] **Step 6: 冻结 evidence。**

  `base.sha` 保持 docs-only base，`head.sha` 已由 Step 5 写入。用实际 SHA 生成 `state.env`：

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  printf '%s\n' \
    'PACKET=P0B' \
    'STATUS=PASS' \
    "P0B_BASE=$(cat /private/tmp/workbench-adaptive-mvp-p0b/base.sha)" \
    "P0B_HEAD=$(cat /private/tmp/workbench-adaptive-mvp-p0b/head.sha)" \
    > /private/tmp/workbench-adaptive-mvp-p0b/state.env
  ```

  `remaining-risk.txt` 至少写：

  ```text
  P0B fails closed after the current Plan or referenced Items advance; zero migration provides no historical Plan snapshot.
  P0B remains an unwired repository primitive; P0C owns verified success, P0D owns final MySQL crash/race proof, and P1M/P1D own mode retirement and production wiring.
  The existing canonical MySQL fixture is not a shared-DDL schema closure; P0B ordinary package validation runs without the DDL integration gate.
  ```

  用 shell 精确写 risk、allowlist 和 manifest：

  ```bash
  set -euo pipefail
  cd /private/tmp/workbench-adaptive-mvp-p0b
  printf '%s\n' \
    'P0B fails closed after the current Plan or referenced Items advance; zero migration provides no historical Plan snapshot.' \
    'P0B remains an unwired repository primitive; P0C owns verified success, P0D owns final MySQL crash/race proof, and P1M/P1D own mode retirement and production wiring.' \
    'The existing canonical MySQL fixture is not a shared-DDL schema closure; P0B ordinary package validation runs without the DDL integration gate.' \
    > remaining-risk.txt
  printf '%s\n' \
    'M backend/domain/agentthread/repository/adaptive_execution.go' \
    'M backend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    'M backend/domain/agentthread/repository/mysql_adaptive_execution_test.go' \
    'M docs/superpowers/context/workbench-execution-chain.md' \
    'M docs/superpowers/context/workbench-execution-graph.json' \
    'M scripts/workbench-execution-graph/contract.mjs' \
    > allowlist.txt
  printf '%s\n' \
    allowlist.txt \
    base.sha \
    compile.log \
    contract-green.log \
    contract-red.log \
    evidence-files.txt \
    gofmt.log \
    graph-build.log \
    graph-contract.log \
    graph-derived.log \
    graph-postcommit-build.log \
    graph-postcommit-derived.log \
    graph-postcommit-verify.log \
    graph-verify.log \
    head.sha \
    package-green.log \
    recovery-green.log \
    recovery-red.log \
    remaining-risk.txt \
    replay-green.log \
    replay-red.log \
    sqlite-green.log \
    state.env \
    status.txt \
    > evidence-files.txt
  diff -u evidence-files.txt <(find . -maxdepth 1 -type f ! -name evidence.sha256 -print | sed 's#^\./##' | LC_ALL=C sort)
  xargs shasum -a 256 < evidence-files.txt > evidence.sha256
  shasum -a 256 -c evidence.sha256
  ```

---

## 7. Exit gate

只有以下全部成立才把 P0B 标为 PASS 并从真实 P0B HEAD 生成 P0C：

- [ ] exact tuple replay 不依赖 public `ListRunEvents`、Journal projection 或 latest checkpoint；
- [ ] lost response、payload/mutation drift、duplicate decision、duplicate verification、exact recovery、
      multi-hop、degraded projection 全部 PASS；
- [ ] v2 event fingerprint + bounded refs 可从 target SourceCheckpointID 重建原 touched Items；
- [ ] Authority 完整包含 execution/journal/attempt/generation/source/Plan/fingerprint；
- [ ] P0A focused regression、普通 package、graph gate fresh PASS；
- [ ] exact-six implementation commit、clean worktree、双审 P0/P1=0、evidence checksum PASS；
- [ ] 无 migration、MySQL fixture、application、ADK、IDL、frontend、public projection、finalizer 或模式代码变化。

P0D 生成最终 aggregate allowlist 时必须把本包与 P0A3 的 chain/graph/contract maintenance 文件单列，
不能用只过滤 packet docs 的旧 product-code allowlist 误判。

任一失败都停止 P0B；不得通过增加表、复制 Plan、扫描 latest checkpoint、放宽 metadata decoder 或
把 application 接线提前来绕过。
