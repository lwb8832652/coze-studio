# Workbench 自适应执行 MVP P0A3 MySQL Race Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 执行。只在干净的
> `/Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp` 工作树执行。

**Goal:** 在 disposable MySQL 上证明 P0A2 原子边界对同一 Plan/PlanItem 只有一个完整 winner，
并在接线前统一现行 legacy Plan writer 的锁序，消除 adaptive 与 `UpsertPlanItem`/
`ArchivePlanItem` 之间的反向锁序、1213/1205 和部分残留。

**Architecture:** 所有会同时触碰 Plan 与 PlanItem 的生产写路径统一按 Plan → Item 加锁。
adaptive 继续按 Run → Attempt → lineage → Plan scope Run → Plan → sorted Items；legacy writer
在 transaction 第一条查询就锁 Plan，再锁目标 Item。真实 MySQL 测试使用两个独立连接池、
channel start barrier 和 context deadline，不使用 sleep、`AutoMigrate`、test-only global hook 或
额外迁移。Plan 仍只保存稳定大步骤；按需子步骤继续通过 PlanItem metadata 引用，不增加第二套表。

**Files permitted for code modification:**

- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
- Create: `backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `scripts/workbench-execution-graph/contract.mjs`
- Modify: `scripts/workbench-execution-graph.test.mjs`

**Read only:**

- `backend/domain/agentthread/repository/adaptive_execution.go`
- `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`
- `backend/domain/agentthread/repository/mysql_canonical_integration_test.go`
- `backend/domain/agentthread/repository/mysql_canonical_test.go`
- `backend/domain/agentthread/repository/mysql_journal_integration_test.go`
- `backend/domain/agentthread/repository/mysql_test.go`
- `backend/domain/agentthread/repository/plan.go`
- 本计划 0.2 列出的十二份既有 migration

---

## 0. Frozen contract

### 0.1 Base and package boundary

- P0A2 code HEAD 必须精确为
  `a796262aef605d5477c770b977f09ae065995ca0`，其 evidence 必须 `STATUS=PASS` 且 checksum 全通过。
- 本计划必须先作为唯一 docs-only commit 落在该 HEAD 之上；P0A3 code commit 的 parent 是该
  docs commit，不是直接回到 P0A2 code HEAD。
- 不修改 migration、ADK、IDL、frontend、service API、finalizer 或执行模式代码；不接生产调用链。
- P0A3 只证明真实 MySQL 锁序/线性化与回滚；P0B 及以后继续 locked。

### 0.2 Disposable MySQL and physical schema

- 环境变量精确为 `COZE_AGENTTHREAD_TEST_MYSQL_DSN` 与
  `COZE_AGENTTHREAD_TEST_ALLOW_DDL=I_UNDERSTAND_DISPOSABLE_DB`；DSN 的数据库名必须包含
  `agentthread_disposable`。测试沿用现有 integration 约定：本地缺少显式 gate 时可 `t.Skip`，
  `CI=true` 时必须 `t.Fatalf`；但本包 Entry/JSON gate 会先验证环境并要求本轮零 SKIP，缺失时
  P0A3 必须 `BLOCKED`，不能引用一次普通 package run 的 SKIP 继续。
- test-only schema loader 只读取并按下列顺序执行真实 migration：

  1. `20260613000100_agent_threads.sql`
  2. `20260614000200_agent_runs.sql`
  3. `20260614000300_agent_run_events.sql`
  4. `20260617000100_agent_checkpoints.sql`
  5. `20260619000100_agent_checkpoint_runtime_keys.sql`
  6. `20260620000400_agent_run_plans.sql`
  7. `20260621000100_agent_run_children.sql`
  8. `20260711000100_agent_run_leases.sql`
  9. `20260730000100_agent_run_attempts.sql`
  10. `20260730000200_agent_run_events_journal_columns.sql`
  11. `20260730000210_agent_run_events_journal_indexes.sql`
  12. `20260730000220_agent_run_events_journal_projection.sql`

- 禁止 `AutoMigrate`。loader 对每份已冻结 SQL 以 `;` 分割、`TrimSpace` 并逐 statement `Exec`；
  这十二份 SQL 不含 procedure body 或分号字符串。`20260620000400` 有两条 CREATE，不能依赖 DSN
  的 `multiStatements=true`。
- 每个并发子测试先按
  PlanItems → Plans → Checkpoints → Events → Attempts → Runs → Threads 删除显式表，再重建。
  cleanup 只作用于已通过 disposable gate 的精确表。
- 两个 repository 分别来自两次 `gorm.Open`，各自底层 pool 的 MaxOpen/MaxIdle 都为 1；读取
  `SELECT CONNECTION_ID()` 并断言不同，不能把一个 pool 的两个 goroutine 伪装成两连接。
- running Attempt 必须写 non-NULL `started_at`，Run 的 JSON NOT NULL 字段必须写 `{}`/`[]`，
  fixture 必须真实通过 migration CHECK，不以 SQLite/PO 默认值替代。

### 0.3 Shared Plan lock

- 在 `mysql.go` 增加 package-private `lockAgentRunPlanForUpdate(tx, runID)`；MySQL 以
  `SELECT agent_run_plans ... FOR UPDATE` 锁 exact Plan，SQLite 保持 plain query 兼容现有单测；
  not found 映射为既有 `ErrPlanNotFound`。
- `UpsertPlanItem` 与 `ArchivePlanItem` transaction 的第一条 row-lock query 必须调用该 helper，
  然后才锁/read Item、写 Item、increment Plan revision。
- `lockAdaptiveExecutionPlan` 复用同一 helper；只在 `ErrPlanNotFound` 时映射回既有
  `ErrAdaptiveExecutionPlanScopeConflict`，其他数据库错误原样返回。P0A2 typed contract 不变。
- 不修改 `ReservePlanTaskID`：它只写 Plan、不等待 Item，不构成 Item→Plan 环。
- 不增加 retry loop、sleep、advisory lock、test hook 或全局 failpoint；MySQL 1213/1205 出现即测试失败。

### 0.4 Authority graph reconciliation

- 两个 repository 生产文件都命中 Workbench `monitored_paths`，因此本包同步
  `workbench-execution-chain.md` 与 `workbench-execution-graph.json`，不能以“尚未接线”为由跳过。
- chain 必须写明：P0A1/P0A2 的 `CommitAdaptiveExecutionBoundary` 是已实现、已测试但尚无
  application/ADK 生产 caller 的 repository primitive；P1D 接线前 direct legacy Plan mutation 仍可达，
  P0A3 只统一其锁序，不把该 primitive 伪写成当前生产执行边。
- graph 增加 `exclude.unwired_adaptive_boundary`（`implemented_not_wired`），evidence 锚到 chain 的
  `P0A 当前只提供 repository primitive` 句与 `type AdaptiveExecutionRepository interface {`，而不是
  制造不存在的 production call edge；`REQUIRED_EXCLUSION_IDS` 同步加入该 ID。
- 精确修复当前分支已漂移的 locator：`MapADKEvent(mappingCtx,...)`；canonical route node/edge 都改为
  `require.Len(t, canonicalRouteSnapshot(), 54)`；domain event edge 以 service 的
  `if err := s.persistRunEventWithOptionalJournal(` 与 journal helper 的
  `_, err := projectionRepo.CreateRunEventWithJournalProjection(` 两段证明。既有
  `repository.create_run_event` stable ID 保留，label 改为 MySQL RunEvent persistence，并补 projection
  repository source anchor 与 `TestRunEventJournalProjectionKeepsBaseAndJournalViews` evidence。
- 新增 `middleware.side_effect` 与 `edge.middleware_00`（side_effect→reduction）；middleware chain entry、
  ordered nodes/edges 及 `query.middleware_order` 的 expanded/smoke/required 首项全部同步，terminal 保持
  semantic_loop。chain 文本顺序首插 `side_effect`，更新时间改 2026-08-10，并把旧 47 路由口径更新为
  52 条 `/api/workbench/threads/**` method/path + 2 条 journal settings，共 54 条；同一数字必须同步
  `project-context.md` 与 graph contract test 的 route label 断言，不能让长期上下文/测试继续冻结 47。
- graph JSON 的稳定 projection 变化后，必须用 verifier 报出的 actual SHA-256 更新
  `WORKBENCH_PROFILE_STRUCTURE_DIGEST`；只改该常量及因新增 required ID 必需的精确合同，禁止放宽
  validator。`node --test scripts/workbench-execution-graph.test.mjs` 必须 PASS。
- 最终必须 fresh 通过 `verify --changed-from origin/dev`、`build`、`verify-derived`；派生产物不提交。

### 0.5 Concurrent adaptive writers

`TestAdaptiveExecutionBoundaryMySQL/ConcurrentPlanItemMutationHasSingleWinner`：

- 同一 running Run/active Attempt、Plan revision=1、同一 Item version=1；两个 request 使用不同
  Event ID、Checkpoint ID、idempotency key、payload 与 Item post-image，但相同 expected revision/version。
- 两 goroutine 先向 capacity=2 的 ready channel 报到，再等待关闭 start channel；调用均使用同一
  bounded context，无 `time.Sleep`。
- 精确一个调用成功；另一个必须 `ErrorIs(ErrAdaptiveExecutionPlanRevisionConflict)`，不能是 raw
  duplicate、deadline、1213、1205 或泛 error。
- 最终 Plan revision=2、Item version=2 且内容等于 winner；Attempt next=2/LCS=1；只存在 winner 的
  Event/Checkpoint，loser 两个 ID 均 COUNT=0。成功/失败返回后再完整读取 Run、Attempt、Plan、Item、
  Events、Checkpoints，不能只看 RowsAffected。

### 0.6 Concurrent adaptive and reachable legacy writers

`TestAdaptiveExecutionBoundaryMySQL/ConcurrentAdaptiveAndLegacyUpsertAreLinearizable` 与
`TestAdaptiveExecutionBoundaryMySQL/ConcurrentAdaptiveAndLegacyArchiveAreLinearizable` 分别覆盖
两条 legacy writer。现行生产链
`ApplicationADKPlanStore → PlanSVC → repository.UpsertPlanItem/ArchivePlanItem` 可达，因此不能用
“未来不可达”回避锁序。

两类合法线性化结果：

1. **legacy first:** legacy 成功；adaptive 精确 `ErrAdaptiveExecutionPlanRevisionConflict`；最终
   Plan/Item 为 revision/version=2 的 legacy 结果；adaptive Event/Checkpoint COUNT=0，Attempt
   next=1/LCS=0。
2. **adaptive first:** adaptive 完整成功，legacy 随后基于 Item version=2 成功；两者 error 都为 nil；
   最终 Plan/Item revision/version=3，内容为 legacy 后写结果；legacy `Previous` 必须是 adaptive 的
   version=2 post-image；adaptive Event/Checkpoint 存在，Attempt next=2/LCS=1。

`archive` 的 legacy 最终结果必须 status=`deleted`（除非输入本来 completed）且 active=false；
`upsert` 最终内容必须是 legacy post-image。任何其他结果、1213、1205、deadline、raw duplicate、
多余 Event/Checkpoint 或非目标 Item 改动都失败。

### 0.7 Machine gate

- MySQL 根测试和所有 nested tests 均不得调用 `t.Parallel`；缺少显式 disposable gate 时只可按
  0.2 的 local-SKIP/CI-fatal 约定处理，进入 P0A3 machine gate 后仍要求零 SKIP。
- 两轮独立 `go test -json -count=1`；每轮机器解析 JSON，要求下列 exact Test field 的
  `Action=pass` 各恰好一次：
  - `TestAdaptiveExecutionBoundaryMySQL`
  - `TestAdaptiveExecutionBoundaryMySQL/ConcurrentPlanItemMutationHasSingleWinner`
  - `TestAdaptiveExecutionBoundaryMySQL/ConcurrentAdaptiveAndLegacyUpsertAreLinearizable`
  - `TestAdaptiveExecutionBoundaryMySQL/ConcurrentAdaptiveAndLegacyArchiveAreLinearizable`
- 所有 `TestAdaptiveExecutionBoundaryMySQL` 前缀事件中 `skip/fail` 数量为 0，package `fail` 为 0；
  零匹配、重复 PASS、超时或额外同名 PASS 都失败。

---

## 1. Entry gate

- [ ] **Step 1: 验证 P0A2 evidence、docs-only base 与 clean worktree。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  test ! -e "$P0A3_EVIDENCE_DIR"
  mkdir -p "$P0A3_EVIDENCE_DIR"
  (cd "$P0A2_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  . "$P0A2_EVIDENCE_DIR/state.env"
  test "$PACKET" = P0A2
  test "$STATUS" = PASS
  test "$HEAD_SHA" = a796262aef605d5477c770b977f09ae065995ca0
  P0A3_PACKET_BASE_SHA="$(git rev-parse HEAD)"
  test "$(git rev-parse HEAD^)" = "$HEAD_SHA"
  test "$(git log -1 --format=%s)" = 'docs: add adaptive execution P0A3 packet'
  test "$(git diff-tree --no-commit-id --name-only -r HEAD)" = \
    'docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a3-mysql-race.md'
  test -z "$(git status --porcelain)"
  printf '%s\n' "$P0A3_PACKET_BASE_SHA" > "$P0A3_EVIDENCE_DIR/base.sha"
  printf 'PACKET=P0A3\nSTATUS=active\nBASE_SHA=%s\n' "$P0A3_PACKET_BASE_SHA" \
    > "$P0A3_EVIDENCE_DIR/state.env"
  ```

  Expected: P0A2 PASS、当前 HEAD 是唯一 P0A3 plan 的 docs-only commit、工作树 clean。

- [ ] **Step 2: 运行 P0A2 exact SQLite 回归。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-sqlite-entry \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestValidateAdaptiveExecutionBoundaryRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt|TestValidateAdaptiveExecutionMutationRequest|TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionCheckpointMetadataRoundTrip|TestAdaptiveExecutionBoundary.*)$' \
      -count=1 -v 2>&1 | tee "$P0A3_EVIDENCE_DIR/sqlite-entry.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A3_EVIDENCE_DIR/sqlite-entry.log"; then exit 1; fi
  ```

  Expected: P0A1/P0A2 18 个顶层测试全部 PASS、零 SKIP/fail。

- [ ] **Step 3: 证明 disposable MySQL 与真实 DDL entry 可用。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  test -n "${COZE_AGENTTHREAD_TEST_MYSQL_DSN:-}"
  test "${COZE_AGENTTHREAD_TEST_ALLOW_DDL:-}" = I_UNDERSTAND_DISPOSABLE_DB
  case "$COZE_AGENTTHREAD_TEST_MYSQL_DSN" in
    *agentthread_disposable*) ;;
    *) exit 1 ;;
  esac
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-mysql-entry \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestJournalMySQLIntegrationGaplessSequenceAcrossConnections$' \
      -count=1 -json > "$P0A3_EVIDENCE_DIR/mysql-entry.json"
  jq -e -s '
    ([.[] | select(.Action == "pass" and .Test == "TestJournalMySQLIntegrationGaplessSequenceAcrossConnections")] | length) == 1 and
    ([.[] | select((.Action == "skip" or .Action == "fail") and .Test == "TestJournalMySQLIntegrationGaplessSequenceAcrossConnections")] | length) == 0 and
    ([.[] | select(.Action == "fail" and (.Test == null))] | length) == 0
  ' "$P0A3_EVIDENCE_DIR/mysql-entry.json"
  ```

  Expected: 真实 disposable MySQL entry PASS；否则 P0A3=`BLOCKED` 并停止，不写代码。

## 2. Shared lock order RED/GREEN

- [ ] **Step 1: 写 `TestLegacyPlanItemMutationsLockPlanFirstForUpdate`。**

  在新 integration test 文件复用 `canonicalMySQLMockRepository`。`upsert`/`archive` 两个 case 都
  `ExpectBegin`，第一条 query 必须精确匹配
  `SELECT * FROM agent_run_plans WHERE run_id=? ... FOR UPDATE`，让该 query 返回独立 sentinel，随后
  `ExpectRollback`；公共 repo method 必须 `ErrorIs(sentinel)`。不得直接调用待新增 helper。

- [ ] **Step 2: 运行 lock-order RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  lock_order_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-lock-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestLegacyPlanItemMutationsLockPlanFirstForUpdate$' -count=1 -v \
      > "$P0A3_EVIDENCE_DIR/lock-red.log" 2>&1 || lock_order_status=$?
  test "$lock_order_status" -ne 0
  if rg -n 'panic:|undefined:|build failed' "$P0A3_EVIDENCE_DIR/lock-red.log"; then exit 1; fi
  rg -n -- '=== RUN   TestLegacyPlanItemMutationsLockPlanFirstForUpdate' \
    "$P0A3_EVIDENCE_DIR/lock-red.log"
  rg -n -- '--- FAIL: TestLegacyPlanItemMutationsLockPlanFirstForUpdate' \
    "$P0A3_EVIDENCE_DIR/lock-red.log"
  ```

  Expected: 现有 Item→Plan 首锁与 sqlmock 的 Plan-first 期望不符，行为 RED；不得接受编译错误。

- [ ] **Step 3: 实现共享 Plan lock 并改三条生产路径。**

  只按 0.3 增加 helper；`UpsertPlanItem`、`ArchivePlanItem` transaction 首先调用；adaptive wrapper
  复用并保持原 typed mapping。不得重构其他 Plan API。

- [ ] **Step 4: 运行 lock-order GREEN 与旧 Plan 回归。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-lock-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestLegacyPlanItemMutationsLockPlanFirstForUpdate|TestPlanRepositoryUpsertsAndArchivesCompletedItem)$' \
      -count=1 -v 2>&1 | tee "$P0A3_EVIDENCE_DIR/lock-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A3_EVIDENCE_DIR/lock-green.log"; then exit 1; fi
  ```

  Expected: Plan-first 合同与现有 Plan 行为都 PASS。

## 3. Real MySQL race RED/GREEN

- [ ] **Step 1: 写 `TestAdaptiveExecutionBoundaryMySQL` 与三类并发断言。**

  根测试包含 0.5/0.6 的三个 exact subtest。
  初次只声明测试调用和断言，调用尚未定义的 `adaptiveExecutionMySQLIntegrationRepositories`；
  所有 request、result、readback、MySQL code helpers 都写在 test 文件，不改生产 API。

- [ ] **Step 2: 运行 fixture RED。**

  ```bash
  set -u
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  fixture_status=0
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-fixture-red \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionBoundaryMySQL$' -count=1 \
      > "$P0A3_EVIDENCE_DIR/fixture-red.log" 2>&1 || fixture_status=$?
  test "$fixture_status" -ne 0
  rg -n 'undefined: adaptiveExecutionMySQLIntegrationRepositories' \
    "$P0A3_EVIDENCE_DIR/fixture-red.log"
  ```

  Expected: 只因 test-only real schema/connection helper 未定义而 RED。

- [ ] **Step 3: 实现 test-only migration loader、两连接 fixture 与 bounded runner。**

  严格按 0.2；每个子测试独立重建 schema，断言 connection IDs 不同。runner 用 capacity=2 ready、
  close(start)、两个 goroutine、`context.WithTimeout` 和 result channel；无 sleep、无 production hook。

- [ ] **Step 4: 运行一次 MySQL GREEN。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-mysql-green \
    go test -p 1 ./domain/agentthread/repository \
      -run '^TestAdaptiveExecutionBoundaryMySQL$' -count=1 -timeout=90s -v 2>&1 | \
      tee "$P0A3_EVIDENCE_DIR/mysql-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:|Error 1213|Error 1205' "$P0A3_EVIDENCE_DIR/mysql-green.log"; then exit 1; fi
  ```

  Expected: adaptive single-winner、adaptive/upsert、adaptive/archive 全部 PASS，零 SKIP/fail/deadlock。

## 4. Two-round machine gate and regression

- [ ] **Step 1: 连续运行两轮 JSON race gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  for round in 1 2
  do
    output="$P0A3_EVIDENCE_DIR/mysql-race-round-$round.json"
    GOCACHE="/private/tmp/coze-adaptive-mvp-p0a3-race-$round" \
      go test -p 1 ./domain/agentthread/repository \
        -run '^TestAdaptiveExecutionBoundaryMySQL$' -count=1 -timeout=90s -json > "$output"
    jq -e -s '
      ([.[] | select(.Action == "pass" and .Test == "TestAdaptiveExecutionBoundaryMySQL")] | length) == 1 and
      ([.[] | select(.Action == "pass" and .Test == "TestAdaptiveExecutionBoundaryMySQL/ConcurrentPlanItemMutationHasSingleWinner")] | length) == 1 and
      ([.[] | select(.Action == "pass" and .Test == "TestAdaptiveExecutionBoundaryMySQL/ConcurrentAdaptiveAndLegacyUpsertAreLinearizable")] | length) == 1 and
      ([.[] | select(.Action == "pass" and .Test == "TestAdaptiveExecutionBoundaryMySQL/ConcurrentAdaptiveAndLegacyArchiveAreLinearizable")] | length) == 1 and
      ([.[] | select((.Action == "skip" or .Action == "fail") and (.Test // "" | startswith("TestAdaptiveExecutionBoundaryMySQL")))] | length) == 0 and
      ([.[] | select(.Action == "fail" and (.Test == null))] | length) == 0
    ' "$output"
  done
  ```

  Expected: 两轮四个 exact PASS 各一次，零 skip/fail/package fail。

- [ ] **Step 2: 运行 SQLite/P0A2/legacy/full package 回归。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-focused \
    go test -p 1 ./domain/agentthread/repository \
      -run '^(TestPlanRepositoryUpsertsAndArchivesCompletedItem|TestLegacyPlanItemMutationsLockPlanFirstForUpdate|TestValidateAdaptiveExecutionBoundaryRequest|TestLockAdaptiveExecutionRun|TestLockAdaptiveExecutionAttempt|TestValidateAdaptiveExecutionMutationRequest|TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON|TestAdaptiveExecutionCheckpointMetadataRoundTrip|TestAdaptiveExecutionBoundary.*)$' \
      -count=1 -v 2>&1 | tee "$P0A3_EVIDENCE_DIR/focused-green.log"
  if rg -n -- '--- SKIP:|--- FAIL:' "$P0A3_EVIDENCE_DIR/focused-green.log"; then exit 1; fi
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-package \
    go test -p 1 ./domain/agentthread/repository -count=1 -timeout=240s 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/package-green.log"
  ```

  Expected: focused 与 repository package 全部 PASS。

- [ ] **Step 3: 同步 Workbench 权威执行链源码。**

  按 0.4 更新 chain/JSON/structure digest：修正四个已漂移 locator，补 `side_effect → reduction` middleware
  顺序，并记录 adaptive boundary 是尚未接 application/ADK 的 repository primitive。不得制造
  application→boundary 的假调用边；不得手改 `graphify-out/`。

- [ ] **Step 4: 运行 compile、format、DDL/static 与 allowlist gate。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp/backend
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  GOCACHE=/private/tmp/coze-adaptive-mvp-p0a3-compile \
    go test -p 1 -run '^$' ./domain/agentthread/repository -count=1 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/compile.log"
  cd ..
  for file in \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    docs/superpowers/context/project-context.md \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs \
    scripts/workbench-execution-graph.test.mjs
  do
    test -f "$file"
  done
  gofmt -d \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go | \
    tee "$P0A3_EVIDENCE_DIR/gofmt.log"
  test ! -s "$P0A3_EVIDENCE_DIR/gofmt.log"
  test -z "$(rg -n 'AutoMigrate|time\.Sleep|t\.Parallel' \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go || true)"
  for migration in \
    20260613000100_agent_threads.sql \
    20260614000200_agent_runs.sql \
    20260614000300_agent_run_events.sql \
    20260617000100_agent_checkpoints.sql \
    20260619000100_agent_checkpoint_runtime_keys.sql \
    20260620000400_agent_run_plans.sql \
    20260621000100_agent_run_children.sql \
    20260711000100_agent_run_leases.sql \
    20260730000100_agent_run_attempts.sql \
    20260730000200_agent_run_events_journal_columns.sql \
    20260730000210_agent_run_events_journal_indexes.sql \
    20260730000220_agent_run_events_journal_projection.sql
  do
    test "$(rg -c "$migration" \
      backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go)" = 1
  done
  git diff --check
  git diff --diff-filter=D --exit-code -- \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    docs/superpowers/context/project-context.md \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs \
    scripts/workbench-execution-graph.test.mjs
  worktree_status="$(git status --porcelain=v1)"
  test "$(printf '%s\n' "$worktree_status" | wc -l | tr -d ' ')" = 8
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M backend/domain/agentthread/repository/mysql.go" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M backend/domain/agentthread/repository/mysql_adaptive_execution.go" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == "?? backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M docs/superpowers/context/project-context.md" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M docs/superpowers/context/workbench-execution-chain.md" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M docs/superpowers/context/workbench-execution-graph.json" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M scripts/workbench-execution-graph/contract.mjs" { count++ } END { print count + 0 }')" = 1
  test "$(printf '%s\n' "$worktree_status" | awk \
    '$0 == " M scripts/workbench-execution-graph.test.mjs" { count++ } END { print count + 0 }')" = 1
  test -z "$(git diff --cached --name-only)"
  ```

  Expected: compile/gofmt/static/diff 全部 PASS、真实 migration 精确各一次、只有 exact 8 files 变化。

- [ ] **Step 5: 构建并验证 Workbench 权威执行图。**

  authority 源文件已在 4.3 更新；随后运行：

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-verify.log"
  node scripts/workbench-execution-graph.mjs build 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-build.log"
  node scripts/workbench-execution-graph.mjs verify-derived 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-derived.log"
  node --test scripts/workbench-execution-graph.test.mjs 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-contract-test.log"
  git diff --check
  ```

  Expected: 三个图谱命令与 Node contract test 都 PASS；stable 派生图已更新但不进入 git diff。

- [ ] **Step 6: 请求 spec review 与 quality review。**

  两个 reviewer 独立核 migration closure、连接独立性、start barrier、bounded completion、三类线性化、
  1213/1205、typed loser、完整 readback、锁序、旧 API 语义与 P0A2 回归。任一 P0/P1 必须修复并
  重跑 4.1～4.5；P2 必须明确裁决。

## 5. Commit and evidence freeze

- [ ] **Step 1: 暂存 exact 8 files 并提交。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  git add -- \
    backend/domain/agentthread/repository/mysql.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution.go \
    backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go \
    docs/superpowers/context/project-context.md \
    docs/superpowers/context/workbench-execution-chain.md \
    docs/superpowers/context/workbench-execution-graph.json \
    scripts/workbench-execution-graph/contract.mjs \
    scripts/workbench-execution-graph.test.mjs
  test "$(git diff --cached --name-only | wc -l | tr -d ' ')" = 8
  test -z "$(git diff --cached --name-only | grep -Ev \
    '^(backend/domain/agentthread/repository/mysql.go|backend/domain/agentthread/repository/mysql_adaptive_execution.go|backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go|docs/superpowers/context/project-context.md|docs/superpowers/context/workbench-execution-chain.md|docs/superpowers/context/workbench-execution-graph.json|scripts/workbench-execution-graph/contract.mjs|scripts/workbench-execution-graph.test.mjs)$' || true)"
  git diff --cached --diff-filter=D --exit-code
  git diff --cached --check
  git commit -m 'fix: unify plan mutation lock order'
  test "$(git log -1 --format=%s)" = 'fix: unify plan mutation lock order'
  git rev-parse HEAD > "$P0A3_EVIDENCE_DIR/head.sha"
  node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-postcommit-verify.log"
  node scripts/workbench-execution-graph.mjs build 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-postcommit-build.log"
  node scripts/workbench-execution-graph.mjs verify-derived 2>&1 | \
    tee "$P0A3_EVIDENCE_DIR/graph-postcommit-derived.log"
  test -z "$(git status --porcelain)"
  ```

  Expected: 一个 implementation + authority-context commit，parent 为 P0A3 docs commit。

- [ ] **Step 2: 冻结 P0A3 与 aggregate P0A evidence。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  P0A3_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a3
  P0A_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a
  P0A1_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a1
  P0A2_EVIDENCE_DIR=/private/tmp/workbench-adaptive-mvp-p0a2
  test ! -e "$P0A_EVIDENCE_DIR"
  mkdir -p "$P0A_EVIDENCE_DIR"
  P0A3_BASE_SHA="$(cat "$P0A3_EVIDENCE_DIR/base.sha")"
  P0A3_HEAD_SHA="$(cat "$P0A3_EVIDENCE_DIR/head.sha")"
  test "$(git rev-parse HEAD)" = "$P0A3_HEAD_SHA"
  test "$(git rev-parse HEAD^)" = "$P0A3_BASE_SHA"
  test -z "$(git status --porcelain)"
  test "$(git diff --name-only "$P0A3_BASE_SHA..$P0A3_HEAD_SHA" | wc -l | tr -d ' ')" = 8
  git diff --diff-filter=D --exit-code "$P0A3_BASE_SHA..$P0A3_HEAD_SHA"
  printf 'P0A3 proves migration-backed MySQL adaptive single-winner and adaptive/legacy Plan-first linearizability; ADK coordination and public projection remain later packets.' \
    > "$P0A3_EVIDENCE_DIR/remaining-risk.txt"
  printf 'PACKET=P0A3\nSTATUS=PASS\nBASE_SHA=%s\nHEAD_SHA=%s\n' \
    "$P0A3_BASE_SHA" "$P0A3_HEAD_SHA" > "$P0A3_EVIDENCE_DIR/state.env"
  (cd "$P0A3_EVIDENCE_DIR" && shasum -a 256 \
    sqlite-entry.log mysql-entry.json lock-red.log lock-green.log fixture-red.log mysql-green.log \
    mysql-race-round-1.json mysql-race-round-2.json focused-green.log package-green.log \
    compile.log gofmt.log graph-verify.log graph-build.log graph-derived.log graph-contract-test.log \
    graph-postcommit-verify.log graph-postcommit-build.log graph-postcommit-derived.log \
    base.sha head.sha state.env remaining-risk.txt > evidence.sha256)
  (cd "$P0A3_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  (cd "$P0A1_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  (cd "$P0A2_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  P0A1_BASE_SHA="$(awk -F= '$1 == "BASE_SHA" { print $2 }' "$P0A1_EVIDENCE_DIR/state.env")"
  P0A1_HEAD_SHA="$(awk -F= '$1 == "HEAD_SHA" { print $2 }' "$P0A1_EVIDENCE_DIR/state.env")"
  P0A2_BASE_SHA="$(awk -F= '$1 == "BASE_SHA" { print $2 }' "$P0A2_EVIDENCE_DIR/state.env")"
  P0A2_HEAD_SHA="$(awk -F= '$1 == "HEAD_SHA" { print $2 }' "$P0A2_EVIDENCE_DIR/state.env")"
  test "$(awk -F= '$1 == "PACKET" { print $2 }' "$P0A1_EVIDENCE_DIR/state.env")" = P0A1
  test "$(awk -F= '$1 == "STATUS" { print $2 }' "$P0A1_EVIDENCE_DIR/state.env")" = PASS
  test "$(awk -F= '$1 == "PACKET" { print $2 }' "$P0A2_EVIDENCE_DIR/state.env")" = P0A2
  test "$(awk -F= '$1 == "STATUS" { print $2 }' "$P0A2_EVIDENCE_DIR/state.env")" = PASS
  test "$P0A1_BASE_SHA" = 70c18605f3b6469968584a3289bca17ab1a34a1f
  test "$P0A1_HEAD_SHA" = ce638d88c9e79959f26051fbf4aa14bf6e6d9042
  test "$P0A2_BASE_SHA" = f144229c67790112da901d88a5b9e71413f7b4f4
  test "$P0A2_HEAD_SHA" = a796262aef605d5477c770b977f09ae065995ca0
  test "$(git rev-parse "$P0A1_HEAD_SHA^")" = "$P0A1_BASE_SHA"
  test "$(git rev-parse "$P0A2_BASE_SHA^")" = "$P0A1_HEAD_SHA"
  test "$(git rev-parse "$P0A2_HEAD_SHA^")" = "$P0A2_BASE_SHA"
  test "$(git rev-parse "$P0A3_BASE_SHA^")" = "$P0A2_HEAD_SHA"
  test "$(git diff --name-status "$P0A1_BASE_SHA..$P0A1_HEAD_SHA")" = "$(printf '%b\n' \
    'A	backend/domain/agentthread/repository/adaptive_execution.go' \
    'A	backend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    'A	backend/domain/agentthread/repository/mysql_adaptive_execution_test.go')"
  test "$(git diff --name-status "$P0A2_BASE_SHA..$P0A2_HEAD_SHA")" = "$(printf '%b\n' \
    'M	backend/domain/agentthread/repository/adaptive_execution.go' \
    'M	backend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    'M	backend/domain/agentthread/repository/mysql_adaptive_execution_test.go')"
  test "$(git diff --name-status "$P0A3_BASE_SHA..$P0A3_HEAD_SHA")" = "$(printf '%b\n' \
    'M	backend/domain/agentthread/repository/mysql.go' \
    'M	backend/domain/agentthread/repository/mysql_adaptive_execution.go' \
    'A	backend/domain/agentthread/repository/mysql_adaptive_execution_integration_test.go' \
    'M	docs/superpowers/context/project-context.md' \
    'M	docs/superpowers/context/workbench-execution-chain.md' \
    'M	docs/superpowers/context/workbench-execution-graph.json' \
    'M	scripts/workbench-execution-graph/contract.mjs' \
    'M	scripts/workbench-execution-graph.test.mjs')"
  printf '%s\n' \
    "P0A1 $P0A1_BASE_SHA $P0A1_HEAD_SHA" \
    "P0A2 $P0A2_BASE_SHA $P0A2_HEAD_SHA" \
    "P0A3 $P0A3_BASE_SHA $P0A3_HEAD_SHA" > "$P0A_EVIDENCE_DIR/lineage.txt"
  printf '%s\n' \
    'P0A1: A adaptive_execution.go, A mysql_adaptive_execution.go, A mysql_adaptive_execution_test.go' \
    'P0A2: M adaptive_execution.go, M mysql_adaptive_execution.go, M mysql_adaptive_execution_test.go' \
    'P0A3: M mysql.go, M mysql_adaptive_execution.go, A mysql_adaptive_execution_integration_test.go, M project-context.md, M workbench-execution-chain.md, M workbench-execution-graph.json, M workbench-execution-graph/contract.mjs, M workbench-execution-graph.test.mjs' \
    > "$P0A_EVIDENCE_DIR/allowlist.txt"
  shasum -a 256 "$P0A1_EVIDENCE_DIR/evidence.sha256" | awk '{ print $1 }' \
    > "$P0A_EVIDENCE_DIR/p0a1-evidence-manifest.sha256"
  shasum -a 256 "$P0A2_EVIDENCE_DIR/evidence.sha256" | awk '{ print $1 }' \
    > "$P0A_EVIDENCE_DIR/p0a2-evidence-manifest.sha256"
  shasum -a 256 "$P0A3_EVIDENCE_DIR/evidence.sha256" | awk '{ print $1 }' \
    > "$P0A_EVIDENCE_DIR/p0a3-evidence-manifest.sha256"
  printf '%s\n' \
    'P0A proves fenced atomic persistence and migration-backed MySQL linearizability only.' \
    'P0B still owns public projection/recovery dispatch; P1M/P1D own mode retirement and adaptive producer wiring.' \
    'Before gate-on production wiring, direct legacy Plan mutations must be routed through the adaptive boundary or disabled for adaptive Runs so checkpoints cannot lag a later legacy Plan revision.' \
    > "$P0A_EVIDENCE_DIR/remaining-risk.txt"
  printf 'PACKET=P0A\nSTATUS=PASS\nP0A1_BASE=%s\nP0A1_HEAD=%s\nP0A2_BASE=%s\nP0A2_HEAD=%s\nP0A3_BASE=%s\nP0A3_HEAD=%s\n' \
    "$P0A1_BASE_SHA" "$P0A1_HEAD_SHA" "$P0A2_BASE_SHA" "$P0A2_HEAD_SHA" \
    "$P0A3_BASE_SHA" "$P0A3_HEAD_SHA" > "$P0A_EVIDENCE_DIR/state.env"
  (cd "$P0A_EVIDENCE_DIR" && shasum -a 256 \
    state.env lineage.txt allowlist.txt remaining-risk.txt \
    p0a1-evidence-manifest.sha256 p0a2-evidence-manifest.sha256 \
    p0a3-evidence-manifest.sha256 > evidence.sha256)
  (cd "$P0A_EVIDENCE_DIR" && shasum -a 256 -c evidence.sha256)
  ```

  Expected: P0A3 与 aggregate P0A 都为 PASS，三包形成线性祖先链，工作树 clean。

## 6. Stop boundary

P0A/P0A3 PASS 后才从真实 `P0A3_HEAD_SHA` 生成唯一 P0B plan，并按 master 的 docs-only handoff
单独提交。P0A3 不接 ADK coordinator/finalizer，不修改 IDL/frontend/mode，不发布、不合并 `dev`；
任何需要新 migration、retry loop、第二套 Plan 或 test-only production hook 的方案都立即停止并回设计。
