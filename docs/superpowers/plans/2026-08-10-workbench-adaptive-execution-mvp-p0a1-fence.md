# Workbench 自适应执行 MVP P0A1 Contract and Fence Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 执行。只在干净的
> `/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp` 工作树执行。

**Goal:** 用最小 typed contract 和 SQLite row locks 证明无效 identity、失效 Run lease、取消请求
及漂移 Attempt 都会被精确拒绝；本包不写 Event、Checkpoint 或 Plan。

**Files permitted for modification:**

- Create: `backend/domain/agentthread/repository/adaptive_execution.go`
- Create: `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
- Create: `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`

**Read only:**

- `backend/domain/agentthread/repository/repository.go:20-45`
- `backend/domain/agentthread/repository/mysql.go:107-193`
- `backend/domain/agentthread/repository/mysql.go:5048-5180`
- `backend/domain/agentthread/repository/mysql_journal_boundary_test.go:112-225`
- `backend/domain/agentthread/repository/mysql_journal_integration_test.go:38-165`

---

## 0. Entry gate

- [ ] **Step 1: 记录 clean base。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  mkdir -p "$P0A1_EVIDENCE_DIR"
  test -z "$(git status --porcelain)"
  P0A1_BASE_SHA="$(git rev-parse HEAD)"
  printf '%s\n' "$P0A1_BASE_SHA" | tee "$P0A1_EVIDENCE_DIR/base.sha"
  printf 'PACKET=P0A1\nSTATUS=active\nBASE_SHA=%s\n' "$P0A1_BASE_SHA" \
    > "$P0A1_EVIDENCE_DIR/state.env"
  ```

  Expected: 工作树 clean，base SHA 非空。

- [ ] **Step 2: 运行现有 SQLite transaction 基线。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-sqlite-baseline \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestJournalBoundaryCommitsTerminalEventCheckpointAndOffsetOnce|TestJournalBoundaryCheckpointFailureRollsBackSucceededState)$' \
      -count=1 -v 2>&1 | tee "$P0A1_EVIDENCE_DIR/sqlite-baseline.log"
  ```

  Expected: 两项 PASS。

- [ ] **Step 3: 在写代码前证明 disposable MySQL 可用。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  test -n "${COZE_AGENTTHREAD_TEST_MYSQL_DSN:-}"
  test "${COZE_AGENTTHREAD_TEST_ALLOW_DDL:-}" = "I_UNDERSTAND_DISPOSABLE_DB"
  case "$COZE_AGENTTHREAD_TEST_MYSQL_DSN" in
    *agentthread_disposable*) ;;
    *) exit 1 ;;
  esac
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-mysql-entry \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestJournalMySQLIntegrationGaplessSequenceAcrossConnections$' \
      -count=1 -v 2>&1 | tee "$P0A1_EVIDENCE_DIR/mysql-entry.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A1_EVIDENCE_DIR/mysql-entry.log"; then exit 1; fi
  ```

  Expected: 真实 MySQL PASS、零 SKIP；否则 P0A1=`BLOCKED` 并停止。

## 1. Typed request validation

- [ ] **Step 1: 写 contract table test。**

  新增 `TestValidateAdaptiveExecutionBoundaryRequest`。基于一个合法 request，分别清空
  `ThreadID`、`ExecutionRunID`、`JournalRunID`、`AttemptID`、`Generation`、`LeaseOwner`、
  `LeaseToken`、`Now`、`IdempotencyKey`、`Event`、`Checkpoint`，并分别制造 Event/Checkpoint
  的 ThreadID、RunID 漂移；每个子例都必须
  `require.ErrorIs(t, err, ErrAdaptiveExecutionBoundaryInvalid)`。

- [ ] **Step 2: 运行 contract RED 并保存预期失败。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  contract_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-contract-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestValidateAdaptiveExecutionBoundaryRequest$' -count=1 \
      >"$P0A1_EVIDENCE_DIR/contract-red.log" 2>&1 || contract_status=$?
  sed -n '1,120p' "$P0A1_EVIDENCE_DIR/contract-red.log"
  test "$contract_status" -ne 0
  rg -n 'undefined: (CommitAdaptiveExecutionBoundaryRequest|ErrAdaptiveExecutionBoundaryInvalid|validateAdaptiveExecutionBoundaryRequest)' \
    "$P0A1_EVIDENCE_DIR/contract-red.log"
  ```

  Expected: 仅因 contract/validator 未定义而 RED。

- [ ] **Step 3: 定义 typed contract。**

  在 `adaptive_execution.go` 定义 `ErrAdaptiveExecutionBoundaryInvalid`、
  `ErrAdaptiveExecutionAttemptConflict` 和 `CommitAdaptiveExecutionBoundaryRequest`。request 精确包含：

  ```text
  ThreadID int64
  ExecutionRunID int64
  JournalRunID int64
  AttemptID string
  Generation uint64
  LeaseOwner string
  LeaseToken string
  Now int64
  IdempotencyKey string
  Event *entity.RunEvent
  Checkpoint *entity.Checkpoint
  ```

  本包不定义 repository method/result/Plan mutation；这些只在 P0A2 加载。

- [ ] **Step 4: 实现纯 validator。**

  `validateAdaptiveExecutionBoundaryRequest` 只做非空/正数检查，并校验 Event/Checkpoint 的
  `ThreadID`、`RunID` 与 request 一致。任一失败都 wrap
  `ErrAdaptiveExecutionBoundaryInvalid`；不得访问数据库。

- [ ] **Step 5: 运行 contract GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-contract-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestValidateAdaptiveExecutionBoundaryRequest$' -count=1 -v 2>&1 | \
      tee "$P0A1_EVIDENCE_DIR/contract-green.log"
  ```

  Expected: 全部 table cases PASS。

## 2. Run fence

- [ ] **Step 1: 写 Run fence table test。**

  新增 `TestLockAdaptiveExecutionRun`。fixture 是 running Run、generation=3、owner/token exact、
  expiry=`Now+1`、cancel NULL。子例及精确错误：

  ```text
  wrong ThreadID -> ErrRunLeaseLost
  non-running status -> ErrRunLeaseLost
  stale generation -> ErrRunLeaseLost
  wrong owner -> ErrRunLeaseLost
  wrong token -> ErrRunLeaseLost
  expiry == Now -> ErrRunLeaseLost
  cancel requested -> ErrRunCanceled
  ```

  每个子例在独立 transaction 调 `lockAdaptiveExecutionRun`；失败后完整读回 Run，断言未变。

- [ ] **Step 2: 运行 Run fence RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  run_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-run-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestLockAdaptiveExecutionRun$' -count=1 \
      >"$P0A1_EVIDENCE_DIR/run-red.log" 2>&1 || run_status=$?
  sed -n '1,120p' "$P0A1_EVIDENCE_DIR/run-red.log"
  test "$run_status" -ne 0
  rg -n 'undefined: lockAdaptiveExecutionRun' "$P0A1_EVIDENCE_DIR/run-red.log"
  ```

  Expected: 仅因 lock helper 未定义而 RED。

- [ ] **Step 3: 实现 Run lock。**

  在 `mysql_adaptive_execution.go` 实现 `lockAdaptiveExecutionRun(tx, req)`：按 physical
  `ExecutionRunID` `FOR UPDATE`，依次核 `ThreadID`、running、generation、owner、token、
  `lease_expires_at > Now`、cancel NULL；cancel 返回 `ErrRunCanceled`，其余 fence 漂移返回
  `ErrRunLeaseLost`。不更新任何列。

- [ ] **Step 4: 运行 Run fence GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-run-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestLockAdaptiveExecutionRun$' -count=1 -v 2>&1 | \
      tee "$P0A1_EVIDENCE_DIR/run-green.log"
  ```

  Expected: success + 7 conflict cases PASS，数据库基线不变。

## 3. Attempt fence

- [ ] **Step 1: 写 Attempt fence table test。**

  新增 `TestLockAdaptiveExecutionAttempt`。合法行按 `(JournalRunID, AttemptID)` 命中，要求
  `ThreadID`、`ExecutionRunID`、active status 和 non-NULL `ActiveSlot` 完全匹配。terminal status、
  NULL active slot、ExecutionRunID 漂移、跨 Thread 四个子例均精确断言
  `ErrAdaptiveExecutionAttemptConflict`；失败后完整读回 Attempt 不变。

- [ ] **Step 2: 运行 Attempt fence RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  attempt_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-attempt-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestLockAdaptiveExecutionAttempt$' -count=1 \
      >"$P0A1_EVIDENCE_DIR/attempt-red.log" 2>&1 || attempt_status=$?
  sed -n '1,120p' "$P0A1_EVIDENCE_DIR/attempt-red.log"
  test "$attempt_status" -ne 0
  rg -n 'undefined: lockAdaptiveExecutionAttempt' "$P0A1_EVIDENCE_DIR/attempt-red.log"
  ```

  Expected: 仅因 lock helper 未定义而 RED。

- [ ] **Step 3: 实现 Attempt lock。**

  在同一文件实现 `lockAdaptiveExecutionAttempt(tx, req)`：按
  `(journal_run_id, attempt_id)` `FOR UPDATE`，校验 Thread、ExecutionRunID、status active、
  ActiveSlot non-NULL；任一漂移 wrap `ErrAdaptiveExecutionAttemptConflict`。不分配 sequence、
  不更新 Attempt。

- [ ] **Step 4: 运行 Attempt fence GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-attempt-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestLockAdaptiveExecutionAttempt$' -count=1 -v 2>&1 | \
      tee "$P0A1_EVIDENCE_DIR/attempt-green.log"
  ```

  Expected: success + 4 conflict cases PASS，数据库基线不变。

## 4. Commit and machine exit gate

- [ ] **Step 1: 运行 P0A1 exact suite。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-exact \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestValidateAdaptiveExecutionBoundaryRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt)$' \
      -count=1 -v 2>&1 | tee "$P0A1_EVIDENCE_DIR/exact-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A1_EVIDENCE_DIR/exact-green.log"; then exit 1; fi
  for test_name in \
    TestValidateAdaptiveExecutionBoundaryRequest \
    TestLockAdaptiveExecutionRun \
    TestLockAdaptiveExecutionAttempt
  do
    rg -n -- "--- PASS: $test_name" "$P0A1_EVIDENCE_DIR/exact-green.log"
  done
  ```

  Expected: 3 个顶层测试全部 PASS、零 SKIP/fail。

- [ ] **Step 2: 运行 format、package compile 与 diff gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  gofmt -w domain/agentthread/repository/*adaptive_execution*.go
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a1-package \
    go test -p 1 ./domain/agentthread/repository -run '^$' -count=1
  cd ..
  git diff --check
  ```

  Expected: compile PASS，format/diff clean。

- [ ] **Step 3: 提交精确三个文件。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git add -- \
    backend/domain/agentthread/repository/adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_test.go
  test "$(git diff --cached --name-only | wc -l | tr -d ' ')" = "3"
  git diff --cached --check
  git commit -m "feat: fence adaptive execution identity"
  ```

- [ ] **Step 4: 机器验证提交与证据。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  P0A1_BASE_SHA="$(tr -d '\n' < "$P0A1_EVIDENCE_DIR/base.sha")"
  P0A1_HEAD_SHA="$(git rev-parse HEAD)"
  test -n "$P0A1_BASE_SHA"
  git merge-base --is-ancestor "$P0A1_BASE_SHA" "$P0A1_HEAD_SHA"
  git diff --name-only "$P0A1_BASE_SHA..$P0A1_HEAD_SHA" | \
    awk '
      BEGIN { count = 0 }
      {
        count++
        if ($0 != "backend/domain/agentthread/repository/adaptive_execution.go" &&
            $0 != "backend/domain/agentthread/repository/mysql_adaptive_execution.go" &&
            $0 != "backend/domain/agentthread/repository/mysql_adaptive_execution_test.go") bad = 1
      }
      END { exit count != 3 || bad }
    '
  test -z "$(git status --porcelain)"
  printf '%s\n' "$P0A1_HEAD_SHA" | tee "$P0A1_EVIDENCE_DIR/head.sha"
  printf '%s\n' \
    'P0A1 does not yet prove Event/Checkpoint/Plan mutation or MySQL race; P0A2/P0A3 remain locked.' \
    | tee "$P0A1_EVIDENCE_DIR/remaining-risk.txt"
  printf 'PACKET=P0A1\nSTATUS=PASS\nBASE_SHA=%s\nHEAD_SHA=%s\n' \
    "$P0A1_BASE_SHA" "$P0A1_HEAD_SHA" > "$P0A1_EVIDENCE_DIR/state.env"
  shasum -a 256 "$P0A1_EVIDENCE_DIR"/*.log \
    "$P0A1_EVIDENCE_DIR"/base.sha \
    "$P0A1_EVIDENCE_DIR"/head.sha \
    "$P0A1_EVIDENCE_DIR"/state.env \
    "$P0A1_EVIDENCE_DIR"/remaining-risk.txt \
    | tee "$P0A1_EVIDENCE_DIR/evidence.sha256"
  ```

  Expected: clean worktree、精确三文件、base→head 线性、证据 hash 已生成。

P0A1 PASS 后才把 P0A1 记为 `PASS`。随后从 `P0A1_HEAD_SHA` 生成唯一 P0A2 plan 文件，按 master
的 docs-only handoff 单独提交并机器核对；P0A2 的 base 是该 docs commit HEAD。任何未提交或混合
code+docs 的 handoff 都停止；失败时 P0A2～P5 保持 `locked`。
