# Workbench Human Resume Journal Rollover C3h2b Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以最短可交付路径把 Journal-enrolled Human Resume 从 C3h1b 的 409 fail-closed gate 切换为原子的 source Attempt `interrupted` → target Attempt `pending` rollover，同时保留 non-Journal Resume、C3h2a typed bootstrap 和既有公开 API。

**Architecture:** 先交付 DB/IDL/前后端兼容 reader，producer gate 保持关闭；再在现有 `CreateRunBundle` 增加显式 Human 模式，以 Thread-first 锁序完成 full replay、source resolved/terminal 双事件和 target Attempt 原子写入；最后把 Application/Canonical Resume 接到该模式并移除临时 gate。实现只新增 Human 专用 helper，不泛化 Recovery，不扩路由/UI/ordinary enrollment。

**Tech Stack:** Go 1.24、GORM/MySQL/SQLite、Atlas Community v1.2.3、Thrift/Hertz/IDL2TS、React/TypeScript/Vitest、Eino ADK、Workbench execution graph

---

## 0. MVP 交付纪律

- 冻结规格：`docs/superpowers/specs/2026-08-13-workbench-human-resume-journal-rollover-design.md`（commit `ee5c73bb7`）。实现冲突时以当前源码/迁移/IDL为先，并同步修订规格，不能自行扩义。
- 最高优先级是可用 MVP。只保留数据原子性、幂等、并发单赢家、兼容上线、公开数据隔离和真实运行链硬门；不做泛化重构、额外 UI、普通 non-Journal enrollment、gate-on producer、legacy decoder、P1D/P2。
- Phase 1 只部署 compatible readers + migration，C3h1b gate仍关闭 producer；Phase 2 才提交/启用原子 rollover。首次持久化 `interrupted` 后回滚下限是 phase-1 compatible build，不能回到不识别该字面量的旧二进制/客户端。
- 真实 MySQL 环境缺失只报告 `NOT_VERIFIED`，不阻塞本地实现；P1M exit 前必须补 PASS。测试文件不得以空 body 假 PASS。
- 两份用户拥有的未跟踪计划始终不读、不改、不暂存、不删除、不提交。
- 所有代码任务遵循 RED → 最小 GREEN → focused regression → commit；相互写同一文件的任务串行执行。

## 1. 最小文件职责

| 文件 | 职责 |
| --- | --- |
| `backend/domain/agentthread/entity/journal.go` | `interrupted` Attempt 字面量；terminal truth table只在 compatible-reader gate与Finalize/recovery guards同批启用 |
| `docker/atlas/migrations/20260813000100_agent_run_attempts_interrupted.sql` | 前向重建三个 CHECK；不改历史迁移 |
| `docker/atlas/migrations/atlas.sum` | Atlas v1.2.3 生成 checksum；不手改 |
| `idl/workbench/journal.thrift` | additive `JournalExecutionStatus_Interrupted` 唯一公共来源 |
| `backend/api/model/workbench/journal_contract/journal.go` / `frontend/packages/arch/api-schema/src/idl/workbench/journal.ts` | 生成合同，不手改语义 |
| `backend/domain/agentthread/service/service.go` / `service_impl.go` | Human enrollment option、ID/Attempt/双事件权威构造、公开 Finalize 四态门 |
| `backend/domain/agentthread/repository/repository.go` / `mysql.go` | Human request/replay接口、CreateRunBundle 显式分流、公共 RunEvent 查询过滤 |
| `backend/domain/agentthread/repository/mysql_human_resume.go` | Human-only full replay、锁序、strict dual-view append/CAS、原子写入 |
| `backend/application/agentthread/human_interaction_resume.go` | replay-first、non-Journal兼容、移除 C3h1b gate、错误映射 |
| `backend/api/handler/coze/workbench_canonical_run_service.go` | 只保留授权/不可变身份，移除重复 mutable status/Run-only replay判定 |
| `backend/application/agentthread/public_projection.go` / repository list implementations | physical helper 不进入公共 RunEvents、total/cursor/has_more |
| Journal frontend adapter/reducer/stream/timeline/flow | 识别 `interrupted`、终止重连、中性显示，不增加操作/UI |
| Workbench context/chain/graph/主计划 | 只声明 C3h2b Human rollover；P1M/MySQL remaining gates仍开放 |

### Task 1: 交付兼容状态、迁移与生成合同（producer gate 保持关闭）

**Files:**
- Modify: `backend/domain/agentthread/entity/journal.go`
- Create: `backend/domain/agentthread/entity/journal_attempt_status_test.go`
- Create: `docker/atlas/migrations/20260813000100_agent_run_attempts_interrupted.sql`
- Modify (generated): `docker/atlas/migrations/atlas.sum`
- Modify: `idl/workbench/journal.thrift`
- Modify (generated): `backend/api/model/workbench/journal_contract/journal.go`
- Modify (generated): `frontend/packages/arch/api-schema/src/idl/workbench/journal.ts`
- Modify: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`
- Modify: `backend/domain/agentthread/repository/mysql_journal_test.go`

- [ ] **Step 1: 写 status/migration/codegen RED**

新增 entity truth-table、迁移 contract 与生成合同断言：

```go
func TestRunAttemptStatusInterruptedLiteralIsCompatible(t *testing.T) {
    require.Equal(t, RunAttemptStatus("interrupted"), RunAttemptStatusInterrupted)
    require.False(t, RunAttemptStatusInterrupted.IsActive())
}

func TestJournalInterruptedForwardMigrationRebuildsAllLifecycleChecks(t *testing.T) {
    raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docker", "atlas", "migrations", "20260813000100_agent_run_attempts_interrupted.sql"))
    require.NoError(t, err)
    sql := string(raw)
    for _, name := range []string{"chk_agent_run_attempts_status", "chk_agent_run_attempts_active_slot", "chk_agent_run_attempts_lifecycle"} {
        require.Contains(t, sql, "DROP CHECK `"+name+"`")
        require.Contains(t, sql, "CONSTRAINT `"+name+"`")
    }
    require.GreaterOrEqual(t, strings.Count(sql, "'interrupted'"), 3)
}
```

TS contract test must assert `Interrupted = "interrupted"` and that schema/payload/protocol constants remain `1.1/1.0/1.1`.

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test ./domain/agentthread/entity ./domain/agentthread/repository \
  -run 'TestRunAttemptStatusInterrupted|TestJournalInterruptedForwardMigration' -count=1
cd ../frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
```

Expected: missing status/migration/generated literal failures. Task 1 不改变 `IsTerminal()`；否则 public Finalize 在 Task 2 guards落地前会成为提前 producer。

- [ ] **Step 3: 最小实现状态、IDL 和 forward migration**

```go
const RunAttemptStatusInterrupted RunAttemptStatus = "interrupted"
```

IDL 只 append：

```thrift
const JournalExecutionStatus JournalExecutionStatus_Interrupted = "interrupted"
```

Migration 使用一个 `ALTER TABLE agent_run_attempts`，依次 drop/add status、active-slot、lifecycle 三个 named CHECK；terminal 集合加入 `interrupted`，不新增列/索引，不修改 `20260730000100_agent_run_attempts.sql` 或 HCL。

- [ ] **Step 4: 用受控 codegen 生成 Go/TS，Atlas 固定镜像生成 hash**

Go 生成沿用 `backend/scripts/verify_api_codegen.sh` 的 retained clean-tree 方式：从仓库根运行 `KEEP_API_CODEGEN_TMP=1 bash backend/scripts/verify_api_codegen.sh`（首次因 stale 退出1是预期），从唯一输出行 `API codegen temporary trees retained at <tmp>` 解析目录，先 `cmp <tmp>/run-one/backend/api/model/workbench/journal_contract/journal.go <tmp>/run-two/.../journal.go`，再只复制 run-one 的 `journal.go` 到工作树并重跑 verifier。不得 raw `hz update` 真实树。TS 从 api-schema 运行 `rushx update` 后仅保留 `journal.ts` 目标 diff并恢复仓库统一license header。Atlas：

```bash
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" \
  "$ATLAS_IMAGE" migrate hash --dir file:///migrations
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  "$ATLAS_IMAGE" migrate validate --dir file:///migrations
unset ATLAS_IMAGE
```

Expected: Atlas validate PASS；生成 diff 只命中列出的 contract files；router无语义 diff。

- [ ] **Step 5: 运行 GREEN 并提交 phase-1 literal/DB/IDL contract**

```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-go-build go test ./domain/agentthread/entity ./domain/agentthread/repository \
  -run 'TestRunAttemptStatusInterrupted|TestJournal.*Migration' -count=1
cd ../frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
git diff --check
git add backend/domain/agentthread/entity/journal.go \
  backend/domain/agentthread/entity/journal_attempt_status_test.go \
  backend/domain/agentthread/repository/mysql_journal_test.go \
  docker/atlas/migrations/20260813000100_agent_run_attempts_interrupted.sql \
  docker/atlas/migrations/atlas.sum idl/workbench/journal.thrift \
  backend/api/model/workbench/journal_contract/journal.go \
  frontend/packages/arch/api-schema/src/idl/workbench/journal.ts \
  frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts
git commit -m "feat(journal): add interrupted attempt contract"
```

### Task 2: 收紧 public Finalize、generic recovery 与前端兼容读者

**Files:**
- Modify: `backend/domain/agentthread/entity/journal.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_journal.go`
- Modify: `backend/domain/agentthread/repository/mysql_journal_test.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Modify: `backend/domain/agentthread/repository/mysql_canonical.go`
- Modify: `backend/domain/agentthread/repository/mysql_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_canonical_test.go`
- Modify: `backend/application/agentthread/journal_recovery.go`
- Modify: `backend/application/agentthread/journal_recovery_test.go`
- Modify: `backend/application/agentthread/public_projection.go`
- Modify: `backend/application/agentthread/public_projection_test.go`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/journal-types.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/canonical-thread-adapter.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-journal-client.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal-reducer.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal-stream.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal-timeline.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/journal-conversation-flow.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-reducer.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-stream.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-ui.test.tsx`

- [ ] **Step 1: 写双层拒绝与 consumer RED**

```go
func TestFinalizeJournalAttemptRejectsInterrupted(t *testing.T) {
    _, _, err := svc.FinalizeJournalAttempt(ctx, &FinalizeJournalAttemptRequest{
        Status: entity.RunAttemptStatusInterrupted,
        Event: AppendJournalEventRequest{RunID: 10},
    })
    require.ErrorIs(t, err, ErrInvalidArgument)
    require.Zero(t, repo.finalizeCalls)
}

func TestRunAttemptStatusInterruptedBecomesTerminalOnlyWithGuards(t *testing.T) {
    require.True(t, RunAttemptStatusInterrupted.IsTerminal())
    require.False(t, RunAttemptStatusInterrupted.IsActive())
}

func TestSelectJournalRecoverySourceRejectsInterruptedExplicitAndImplicit(t *testing.T) {
    attempt := &domainentity.RunAttempt{AttemptID: "att_interrupted", Ordinal: 9, Status: domainentity.RunAttemptStatusInterrupted}
    require.Nil(t, selectJournalRecoverySourceAttempt([]*domainentity.RunAttempt{attempt}, "att_interrupted"))
    require.Nil(t, selectJournalRecoverySourceAttempt([]*domainentity.RunAttempt{attempt}, ""))
}
```

前端 RED：adapter 接受 literal；reducer/stream 进入 ended；timeline 文案 `已中断`；conversation-flow 有显式 neutral icon/data-status，且不是 failed/timed_out 红色路径。后端 reader RED 同时插入普通 RunEvent 与 physical helper，断言 `ListRunEvents`、`ListRunEventsByCursor` 在 Count/分页前排除 helper，`total/has_more/next cursor` 只计算公开事件，`ProjectPublicRunEvent` 返回 nil；Journal Attempt 查询仍保留同一行的 Journal view。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread \
  -run 'TestFinalizeJournalAttemptRejectsInterrupted|Test.*Recovery.*Interrupted|Test.*JournalAttemptInterrupted|Test.*ListRunEvents.*Interrupted|TestPublicRunEvent.*Interrupted' -count=1
cd ../frontend/apps/coze-studio
./node_modules/.bin/vitest --run \
  src/pages/workbench/thread-client/__tests__/workbench-journal-client.test.ts \
  src/pages/tasks/journal/__tests__/journal-reducer.test.ts \
  src/pages/tasks/journal/__tests__/journal-stream.test.ts \
  src/pages/tasks/journal/__tests__/journal-ui.test.tsx
```

- [ ] **Step 3: 最小 GREEN**

在 entity 增 named closed predicate：

```go
func IsLegacyFinalizableRunAttemptStatus(status RunAttemptStatus) bool {
    switch status {
    case RunAttemptStatusCompleted, RunAttemptStatusFailed,
        RunAttemptStatusCancelled, RunAttemptStatusTimedOut:
        return true
    default:
        return false
    }
}
```

同一变更才把 `RunAttemptStatusInterrupted` 加入 `RunAttemptStatus.IsTerminal()`。不得在 Task 1 先改 terminal truth table；本 Task 的 public service/repository Finalize guards、recovery allowlist、public filters与前端 compatible consumers必须作为一个不可拆分的 phase-1 reader commit落下。

public service/repository Finalize 与 application terminal selector都用该四态 allowlist；repository recovery first-write 在无 lease 时要求该四态，在 expired-lease 路径仍只接受 active source，不能把 `IsTerminal()` 全局回退或破坏 C3h2a lease recovery。entity 同时定义唯一 physical helper 常量 `JournalAttemptInterruptedRunEventType = "journal.attempt.interrupted"`，供后续 writer/filter共用。前端 union/strict parser/两 terminal set加 `interrupted`，timeline label中性；conversation flow显式中性分支。保持 lifecycle event non-displayable，不增加卡片/按钮。

同一 compatible-reader slice 在 `ListRunEvents`/`ListRunEventsByCursor` 的 Count 与分页条件中排除 exact physical type `journal.attempt.interrupted`，并让 `ProjectPublicRunEvent` 二次返回 nil。过滤必须在 producer activation 前完成，不能只放在 handler 造成空页或错误 total/cursor。

- [ ] **Step 4: 运行 GREEN 并提交 compatible readers**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/service ./domain/agentthread/repository ./application/agentthread \
  -run 'TestFinalizeJournalAttempt|Test.*Recovery.*Interrupted|Test.*JournalAttemptInterrupted|Test.*ListRunEvents.*Interrupted|TestPublicRunEvent.*Interrupted' -count=1
cd ../frontend/apps/coze-studio
./node_modules/.bin/vitest --run \
  src/pages/workbench/thread-client/__tests__/workbench-journal-client.test.ts \
  src/pages/tasks/journal/__tests__/journal-reducer.test.ts \
  src/pages/tasks/journal/__tests__/journal-stream.test.ts \
  src/pages/tasks/journal/__tests__/journal-ui.test.tsx
git diff --check
git add backend/domain/agentthread/entity/journal.go \
  backend/domain/agentthread/service/service_impl.go \
  backend/domain/agentthread/service/service_impl_test.go \
  backend/domain/agentthread/repository/mysql_journal.go \
  backend/domain/agentthread/repository/mysql_journal_test.go \
  backend/domain/agentthread/repository/mysql.go \
  backend/domain/agentthread/repository/mysql_canonical.go \
  backend/domain/agentthread/repository/mysql_test.go \
  backend/domain/agentthread/repository/mysql_canonical_test.go \
  backend/application/agentthread/journal_recovery.go \
  backend/application/agentthread/journal_recovery_test.go \
  backend/application/agentthread/public_projection.go \
  backend/application/agentthread/public_projection_test.go \
  frontend/apps/coze-studio/src/pages/workbench/thread-client/journal-types.ts \
  frontend/apps/coze-studio/src/pages/workbench/thread-client/adapters/canonical-thread-adapter.ts \
  frontend/apps/coze-studio/src/pages/workbench/thread-client/__tests__/workbench-journal-client.test.ts \
  frontend/apps/coze-studio/src/pages/tasks/journal/journal-reducer.ts \
  frontend/apps/coze-studio/src/pages/tasks/journal/journal-stream.ts \
  frontend/apps/coze-studio/src/pages/tasks/journal/journal-timeline.tsx \
  frontend/apps/coze-studio/src/pages/tasks/journal/journal-conversation-flow.tsx \
  frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-reducer.test.ts \
  frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-stream.test.ts \
  frontend/apps/coze-studio/src/pages/tasks/journal/__tests__/journal-ui.test.tsx
git commit -m "feat(journal): accept interrupted attempt readers"
```

### Task 3: 定义 Human rollover 与 full replay 闭合合同

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/errors.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`

- [ ] **Step 1: 写 service boundary RED**

覆盖 `TestCreateRunBundleBuildsHumanResumeRolloverBoundary`、`...RejectsRecoveryAndHumanResumeTogether`、`...RejectsHumanResumeWithoutStrictResolvedProjection`，断言 queued/root/reject、User Message、healthy resolved projection、key一致、lineage、TerminalBase/TerminalJournal shared ID。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/service -run 'TestCreateRunBundle.*HumanResume' -count=1
```

- [ ] **Step 3: 添加 exact types 与 service 构造**

```go
type JournalHumanResumeEnrollmentOptions struct {
    JournalRunID, SourceRunID, SourceCheckpointID int64
    SourceAttemptID, IdempotencyKey string
}

type HumanResumeRolloverRequest struct {
    SourceRunID int64
    TerminalBase *entity.RunEvent
    TerminalJournal *entity.JournalEvent
}
```

`JournalEnrollmentOptions.HumanResume` 与 `.Recovery` 互斥。repository request加 `HumanResumeRollover` 与 `ErrHumanResumeRolloverConflict`。定义窄 capability interfaces `HumanResumeRolloverReplayRepository` / `HumanResumeRolloverReplayService`，但不修改宽的 `ThreadRepository` / `ThreadService`；因此不要求全仓 test doubles跟随扩接口。Task 4 concrete repository与Task 5 concrete service分别实现该窄能力。Service Human mode多分配一个 shared terminal ID，固定 physical `journal.attempt.interrupted` payload与 Journal `run.lifecycle/interrupted` envelope，构造 target Attempt lineage；`RecoverySourceLease=nil`，不传第二份 option authority。

- [ ] **Step 4: 运行 GREEN 并提交**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/service -run 'TestCreateRunBundle.*HumanResume' -count=1
git diff --check
git add backend/domain/agentthread/repository/repository.go \
  backend/domain/agentthread/service/service.go \
  backend/domain/agentthread/service/errors.go \
  backend/domain/agentthread/service/service_impl.go \
  backend/domain/agentthread/service/service_impl_test.go
git commit -m "feat(agentthread): define human resume rollover boundary"
```

### Task 4: 实现 full aggregate replay 与原子仓储事务

**Files:**
- Modify: `backend/domain/agentthread/repository/repository.go`
- Modify: `backend/domain/agentthread/repository/mysql.go`
- Create: `backend/domain/agentthread/repository/mysql_human_resume.go`
- Create: `backend/domain/agentthread/repository/mysql_human_resume_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_journal.go`

- [ ] **Step 1: 写核心仓储 RED**

至少冻结这些测试：

```go
func TestHumanResumeRolloverCommitsExactAggregateAndAdjacentSequences(t *testing.T) {}
func TestHumanResumeRolloverExactReplayAfterLifecycleChanges(t *testing.T) {}
func TestHumanResumeRolloverReplayRejectsAggregateDrift(t *testing.T) {}
func TestHumanResumeRolloverRejectsSourceAndCheckpointDriftWithoutWrites(t *testing.T) {}
func TestHumanResumeRolloverRollsBackEveryWriteStage(t *testing.T) {}
func TestHumanResumeRolloverRejectsLateAppendWithoutSequenceMutation(t *testing.T) {}
func TestHumanResumeRolloverDifferentKeyLeavesNoOrphan(t *testing.T) {}
func TestHumanResumeRolloverLocksThreadBeforeAnyConsistentRead(t *testing.T) {}
```

Replay drift table至少改 Message、Run.Command answer/choice/comment、resolved payload/key、terminal parent/link、source/target lineage、operation/fingerprint。Rollback表覆盖 Run、Message、resolved、terminal/CAS、target Attempt。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/repository -run '^TestHumanResumeRollover' -count=1
```

- [ ] **Step 3: 实现 Human-only helper，不重构 generic recovery**

`mysql_human_resume.go` 只包含：

```go
func validateHumanResumeRunBundleInput(req CreateRunBundleRequest) error
func findHumanResumeRolloverReplay(db *gorm.DB, req HumanResumeRolloverReplayRequest) (*HumanResumeRolloverReplayResult, bool, error)
func lockHumanResumeRolloverSource(tx *gorm.DB, req ...) (..., error)
func persistHumanResumeResolvedEventStrict(tx *gorm.DB, ...) (*entity.JournalEvent, error)
func persistHumanResumeInterruptedTerminalStrict(tx *gorm.DB, ...) (*entity.JournalEvent, error)
func createHumanResumeRolloverLocked(tx *gorm.DB, ...) (*CreateRunBundleResult, error)
```

`CreateRunBundle` Human分支从 transaction begin 到 `lockThreadForUpdate` 不做任何 plain SELECT；Thread锁后先调用完整 replay，miss后按 root → source Run → source Attempt → checkpoint → admission锁序校验。写序唯一合法：target Run → Message → resolved N → terminal N+1 + CAS/release source → target Attempt。strict helper复用 `validateTerminalJournalEvent`、`journalEventToPOWithBase`、sequence/parent原语，但禁用 fail-soft `persistRunEventWithJournalProjectionTx` 与 `persistTerminalRunEventWithJournal`。

完整 replay 以 `(space_id,idempotency_key)` 找 committed target；本轮预分配 ID不比较、不读 checkpoint、不要求 target仍pending。Run-only不是成功；缺/漂移 enrolled aggregate为 `ErrRunIdempotencyConflict`，true non-Journal为 `ErrJournalNotEnrolled`。Thread锁内与 transaction-exit fallback必须调用同一 validator。

本 Task 同时在 `repository.go` 冻结 replay request/result和窄 `HumanResumeRolloverReplayRepository`，并让 concrete `threadRepository` 实现；不修改宽 `ThreadRepository`。service 的窄 delegator留到 Task 5与Application调用同批落下。

- [ ] **Step 4: 运行 GREEN 与相邻回归并提交**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/repository \
  -run '^(TestHumanResumeRollover|TestCreateRunBundle|TestJournal)' -count=1
git diff --check
git add backend/domain/agentthread/repository/mysql.go \
  backend/domain/agentthread/repository/repository.go \
  backend/domain/agentthread/repository/mysql_human_resume.go \
  backend/domain/agentthread/repository/mysql_human_resume_test.go \
  backend/domain/agentthread/repository/mysql_journal.go
git commit -m "feat(agentthread): add atomic human resume rollover"
```

### Task 5: 接入 Application/Canonical Resume 并复用 C3h2a typed bootstrap

**Files:**
- Modify: `backend/domain/agentthread/service/service.go`
- Modify: `backend/domain/agentthread/service/service_impl.go`
- Modify: `backend/domain/agentthread/service/service_impl_test.go`
- Modify: `backend/application/agentthread/human_interaction_resume.go`
- Modify: `backend/application/agentthread/service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service_test.go`
- Modify: `backend/application/agentthread/adaptive_bootstrap_coordinator_test.go`

- [ ] **Step 1: 写 application/API RED**

覆盖：enrolled creates rollover；exact replay在 source status/Attempt/checkpoint read前；corrupt aggregate conflict；different-key conflict映射 `run_not_resumable`；unknown repo error不重标；non-Journal unchanged；canonical exact replay在source后来变化后仍200；C3h2a target Attempt facts进入 Resume runtime。

- [ ] **Step 2: 运行 RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./application/agentthread ./api/handler/coze \
  -run 'Test(ApplicationResumeHumanInteraction|CanonicalResume.*Rollover|AdaptiveBootstrap.*HumanResume)' -count=1
```

- [ ] **Step 3: 最小接线**

`ResumeHumanInteraction` 顺序固定为：normalize（丢弃caller SubmittedAt）→ immutable source auth/identity → key → `s.ThreadSVC.GetHumanResumeRolloverReplay` full replay → replay miss才检查 `source.StatusInterrupted`/active Attempt/checkpoint → server timestamp → strict projection → `JournalEnrollment.HumanResume` bundle。删除 `requireHumanResumeJournalRollover` gate。只把 `ErrHumanResumeRolloverConflict`/`ErrActiveRunExists`包装为 `ErrHumanInteractionResumeConflict`。

本 Task 同批让 concrete `threadService` 实现窄 `HumanResumeRolloverReplayService`：内部把 `s.repo` type-assert为窄 repository capability，不修改宽 `ThreadService`。Application 的 Human Resume 路径把 `s.ThreadSVC` type-assert为该窄 service capability；只有会执行本路径的 test double需要显式实现，普通 Thread stubs不扩散。

Canonical handler删 mutable status pre-gate和重复 `SVC.GetRunByIdempotencyKey`；保留 authorized source lookup，随后只委托 application。不要改 `ADKExecutor.Resume` 的 bootstrap-before-runtime顺序。

- [ ] **Step 4: 完成 producer GREEN，但暂不提交**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./application/agentthread ./api/handler/coze \
  -run 'Test(ApplicationResumeHumanInteraction|CanonicalResume|AdaptiveBootstrap.*Resume|ADKExecutorResume)' -count=1
git diff --check
```

Task 5 不单独 commit。继续执行 Task 6 Steps 1–4；三条 anchored bootstrap/SSE tests、local Human tests和兼容/回滚演练通过后，Task 6 Step 5把 producer与acceptance合成一个 reviewed activation commit。三组合是 new-server+old-frontend（producer gate closed）、old-server+new-frontend、new+new；首次产出后只回滚到 phase-1 compatible build。

### Task 6: 验证 bootstrap/SSE 与真实 MySQL 门禁

**Files:**
- Modify: `backend/application/agentthread/journal_query_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_journal_service_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_stream_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_journal_integration_test.go`

- [ ] **Step 1: 写 bootstrap/MySQL RED**

兼容 reader 的 public filter 已由 Task 2 在 producer 前完成；本任务只补端到端守卫，断言 TaskDetail replay/live无额外step。冻结 exact names：`TestHumanResumeJournalBootstrapSelectsSuccessorAttempt`、`TestCanonicalHumanResumeJournalBootstrapExposesInterruptedSourceAndPendingTarget`、`TestCanonicalHumanResumeJournalStreamEndsSourceAndContinuesSuccessor`。MySQL integration fixture先补 `messagePO`、`checkpointPO` 到 cleanup/models，并按顺序加载 Human path 所需 messages/checkpoints/runtime-key migrations和新的 interrupted forward migration；exact tests 为 `TestHumanResumeRolloverMySQLSameKeyReplay`、`TestHumanResumeRolloverMySQLSameKeyDriftConflict`、`TestHumanResumeRolloverMySQLDifferentKeySingleWinner`。

- [ ] **Step 2: 运行 local RED**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/repository ./application/agentthread ./api/handler/coze \
  -run '^(TestHumanResumeJournalBootstrapSelectsSuccessorAttempt|TestCanonicalHumanResumeJournalBootstrapExposesInterruptedSourceAndPendingTarget|TestCanonicalHumanResumeJournalStreamEndsSourceAndContinuesSuccessor)$' -count=1
```

- [ ] **Step 3: 最小 GREEN**

复用 Task 2 已落的 public filter 与现有 bootstrap/SSE 逻辑，只补端到端断言，不加新协议或UI。

- [ ] **Step 4: 运行 local GREEN；有环境才跑 real MySQL**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/repository ./application/agentthread ./api/handler/coze \
  -run 'Test.*(JournalAttemptInterrupted|HumanResume|CanonicalJournal)' -count=1

if [[ -n "${COZE_AGENTTHREAD_TEST_MYSQL_DSN:-}" && \
      "${COZE_AGENTTHREAD_TEST_ALLOW_DDL:-}" == 'I_UNDERSTAND_DISPOSABLE_DB' ]]; then
  GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
    ./domain/agentthread/repository -run '^TestHumanResumeRolloverMySQL(SameKeyReplay|SameKeyDriftConflict|DifferentKeySingleWinner)$' -count=1
else
  echo 'NOT_VERIFIED: disposable MySQL DSN/DDL gate is unavailable'
fi
```

Expected: local PASS；无 DSN/DDL 时明确记录 `NOT_VERIFIED` 且不把 skip称PASS。

- [ ] **Step 5: 合并提交 producer activation 与 acceptance**

```bash
git add backend/domain/agentthread/service/service.go \
  backend/domain/agentthread/service/service_impl.go \
  backend/domain/agentthread/service/service_impl_test.go \
  backend/application/agentthread/human_interaction_resume.go \
  backend/application/agentthread/service_test.go \
  backend/application/agentthread/adaptive_bootstrap_coordinator_test.go \
  backend/api/handler/coze/workbench_canonical_run_service.go \
  backend/api/handler/coze/workbench_canonical_run_service_test.go \
  backend/domain/agentthread/repository/mysql_journal_integration_test.go \
  backend/application/agentthread/journal_query_test.go \
  backend/api/handler/coze/workbench_canonical_journal_service_test.go \
  backend/api/handler/coze/workbench_canonical_run_stream_test.go
git commit -m "feat(workbench): activate journal human resume rollover"
```

### Task 7: 同步 authority 并执行最终本地门禁

**Files:**
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-chat.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Modify: `scripts/workbench-execution-graph/contract.mjs`

- [ ] **Step 1: 更新事实，不扩大完成声明**

记录：C3h2b Human enrolled Resume 已 atomic rollover；C3h2a typed inheritance继续复用；physical helper不进公共 RunEvents；兼容回滚底线。明确 P1M仍未PASS、ordinary non-Journal enrollment/gate-on/legacy decoder仍deferred、真实MySQL若缺环境仍 `NOT_VERIFIED`。

- [ ] **Step 2: 运行完整本地验证**

```bash
cd backend
bash scripts/verify_api_codegen.sh
GOCACHE=/private/tmp/coze-go-build go test -p 1 \
  ./domain/agentthread/entity ./domain/agentthread/service \
  ./domain/agentthread/repository ./application/agentthread ./api/handler/coze -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./application/agentthread ./api/handler/coze \
  -run 'Test(ApplicationResumeHumanInteraction|CanonicalResume|AdaptiveBootstrap.*Resume|ADKExecutorResume)' -count=1
cd ../frontend/packages/arch/api-schema
rushx test src/__tests__/workbench-thread-contract.test.ts
cd ../../../apps/coze-studio
./node_modules/.bin/vitest --run \
  src/pages/workbench/thread-client/__tests__/workbench-journal-client.test.ts \
  src/pages/tasks/journal/__tests__/journal-reducer.test.ts \
  src/pages/tasks/journal/__tests__/journal-stream.test.ts \
  src/pages/tasks/journal/__tests__/journal-ui.test.tsx
cd ../../../..
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
git diff --check
```

Expected: 全部 local gates PASS；Graphify build必须真实成功，不复用旧派生图。环境型全包 MySQL/Redis fixture失败需精确区分，但相关 focused packages必须新鲜PASS。

- [ ] **Step 3: 核对 Task 5/6 已记录的兼容矩阵未发生 SHA/范围漂移，并提交 authority**

Task 7 不重新模拟一遍发布；只核对 Task 5/6 的三组合与 rollback rehearsal证据绑定当前 exact SHA/范围。然后：

```bash
git add docs/superpowers/context/project-context.md \
  docs/superpowers/context/workbench-chat.md \
  docs/superpowers/context/workbench-execution-chain.md \
  docs/superpowers/context/workbench-execution-graph.json \
  docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md \
  scripts/workbench-execution-graph/contract.mjs
git commit -m "docs(workbench): record human resume journal rollover"
```

## 2. Completion definition

C3h2b 本地交付仅在以下全部满足时完成：phase-1 reader/DB/前端兼容；公开 Finalize/recovery拒绝 interrupted；full replay与Thread-first原子写入本地PASS；Application/Canonical已切换且non-Journal回归PASS；public helper不泄露；C3h2a Resume facts回归PASS；图谱 build/verify-derived真实PASS。真实 MySQL缺环境可 `NOT_VERIFIED`，但 P1M不得因此声明完成。
