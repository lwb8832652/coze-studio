# Workbench Journal Recovery Typed Bootstrap Resume Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在已 enrolled 的 Journal recovery Resume 进入 `buildRuntime` 前，原子提交或精确回放 target 的 typed-inheritance adaptive bootstrap facts。

**Architecture:** 保持现有 C2 bootstrap repository API、表结构、事件类型和 checkpoint namespace 不变。Repository 在 typed target 首次写入时于同一事务锁定并校验 target fence、Attempt lineage、source runtime checkpoint 与 source bootstrap；完整 target replay 只深验 target immutable authority。Application coordinator 先读 target，再从严格 source bootstrap 生成 target revision-1 baseline decision，`ADKExecutor.Resume` 在 runtime/store/factory 构造前注入 target facts。

**Tech Stack:** Go 1.25、GORM、SQLite focused tests、gated disposable MySQL integration test、Eino ADK、Workbench execution graph authority。

---

## Delivery boundary

本计划只交付：

- source admission 为 `fresh` 或 `typed_inheritance` 的 Journal recovery Resume；
- target admission 固定为 gate-off `typed_inheritance`；
- target first-write source attestation、target exact replay、multi-hop typed inheritance；
- `ADKExecutor.Resume` 的 pre-`buildRuntime` fail-closed 接线；
- 同步执行链权威事实与聚焦验证。

本计划不交付 legacy decoder/fallback、Human Attempt rollover、ordinary non-Journal enrollment、迁移、IDL、生成 client、UI、gate-on producer、Plan mutation或新的公共 API。真实 MySQL 凭证缺失时只记录 `NOT_VERIFIED`，不能把 Skip 写成通过，也不能把 P1M 标为 PASS。

## File map

- `backend/domain/agentthread/repository/mysql_adaptive_bootstrap.go`: 扩展 existing C2 bootstrap transaction/reader，使其支持 fresh 与 typed-inheritance 两种封闭 lineage。
- `backend/domain/agentthread/repository/mysql_adaptive_bootstrap_test.go`: SQLite first-write、multi-hop、replay、lineage drift 与零写回归。
- `backend/domain/agentthread/repository/mysql_adaptive_bootstrap_integration_test.go`: gated disposable MySQL typed target race；无 gate 时明确 Skip/NOT_VERIFIED。
- `backend/application/agentthread/adaptive_bootstrap_coordinator.go`: 增加 Resume-specific coordinator contract、target-first replay、source inheritance 与 target commit。
- `backend/application/agentthread/adaptive_bootstrap_coordinator_test.go`: coordinator 的顺序、身份、无 fallback 与 no-op 合同。
- `backend/application/agentthread/adk_executor.go`: 在 Resume input/source validation 后、`buildRuntime` 前调用 coordinator。
- `backend/application/agentthread/adk_executor_test.go`: facts 注入顺序、bootstrap error 不建 store/factory、Execute 不回归。
- `docs/superpowers/context/project-context.md`: 记录 C3h2a 的窄 production Resume edge 与 deferred 边界。
- `docs/superpowers/context/workbench-execution-chain.md`: 同步 Resume 调用顺序和 fail-closed 语义。
- `docs/superpowers/context/workbench-execution-graph.json`: 更新 existing adaptive boundary node/edge/evidence，不接 Human、legacy、IDL/UI。
- `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`: 勾选 C3h2a，但保持 P1M locked、MySQL NOT_VERIFIED、P1L deferred。
- `scripts/workbench-execution-graph/contract.mjs`: 只更新 graph structure digest/required edge（若 graph 增加 Resume edge）。

### Task 1: TDD the typed recovery repository contract

**Files:**
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_bootstrap_test.go`
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_bootstrap.go`

- [ ] **Step 1: Add a recovery fixture that starts from a real fresh source bootstrap**

在测试文件中增加 helper。它先用现有 fixture 提交 source bootstrap，再终结 source Attempt、创建 source Eino checkpoint、target running Run 和带完整 lineage 的 active target Attempt：

```go
type adaptiveBootstrapRecoveryFixture struct {
	SourceResult     *CommitAdaptiveExecutionBootstrapResult
	TargetRequest    CommitAdaptiveExecutionBootstrapRequest
	SourceAttemptID  string
	SourceCheckpoint int64
	RecoveryKey      string
}

func seedAdaptiveBootstrapRecoveryTargetForTest(t *testing.T, db *gorm.DB) adaptiveBootstrapRecoveryFixture {
	t.Helper()
	repo := NewAdaptiveExecutionRepository(db)
	sourceRequest := newAdaptiveExecutionBootstrapRequestForTest()
	source, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), sourceRequest)
	require.NoError(t, err)

	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
		Updates(map[string]any{"status": string(entity.RunAttemptStatusCompleted), "active_slot": nil, "ended_at": int64(750)}).Error)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", int64(20)).
		Updates(map[string]any{"status": string(entity.RunStatusSucceeded), "ended_at": int64(750)}).Error)

	sourceCheckpointID := int64(9001)
	require.NoError(t, db.Create(&checkpointPO{
		ID: sourceCheckpointID, ThreadID: 10, RunID: 20,
		CheckpointNS: "eino.adk", RuntimeType: "eino_adk", RuntimeKey: "coze-run-20",
		EnvelopeVersion: 2, ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`),
		PendingSends: []byte(`[]`), Metadata: []byte(`{"runtime":"eino_adk"}`), CreatedAt: 740,
	}).Error)

	leaseOwner, leaseToken, recoveryKey := "worker-2", "lease-2", "recover-2"
	leaseExpiresAt := int64(3000)
	require.NoError(t, db.Create(&runPO{
		ID: 21, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusRunning),
		ExecutionGeneration: 4, LeaseOwner: &leaseOwner, LeaseToken: &leaseToken,
		LeaseExpiresAt: &leaseExpiresAt, CreatedAt: 800, UpdatedAt: 800,
	}).Error)
	active := uint8(1)
	sourceAttemptID := "attempt-1"
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 102, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 21,
		AttemptID: "attempt-2", Ordinal: 2, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &active, NextSequence: 1, LastCommittedSequence: 0,
		SourceAttemptID: &sourceAttemptID, SourceCheckpointID: &sourceCheckpointID,
		RecoveryIdempotencyKey: &recoveryKey, EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState: string(entity.JournalProjectionStateHealthy), CreatedAt: 800, UpdatedAt: 800,
	}).Error)

	target := newAdaptiveExecutionBootstrapRequestForTest()
	target.ExecutionRunID, target.AttemptID, target.Generation = 21, "attempt-2", 4
	target.LeaseOwner, target.LeaseToken = leaseOwner, leaseToken
	target.OperationKey = "adaptive-operation:target"
	target.FactCreatedAt, target.Now = 800, 900
	target.AdmissionEventID, target.DecisionEventID, target.CheckpointID = 7101, 7102, 8101
	sourceRunID, sourceGeneration := int64(20), source.Authority.ExecutionGeneration
	target.Admission = source.Admission
	target.Admission.Source = entity.AdaptiveAdmissionSourceTypedInheritance
	target.Admission.SourceRunID = &sourceRunID
	target.Admission.SourceExecutionGeneration = &sourceGeneration
	target.Admission.SourceConfigDigest, target.Admission.DecoderVersion = "", ""
	planScope := int64(21)
	target.Decision = newAdaptiveBootstrapDecisionForTest(t, target.Admission, "decision-target", 21, 30, "attempt-2", 4, planScope, 800)
	return adaptiveBootstrapRecoveryFixture{source, target, sourceAttemptID, sourceCheckpointID, recoveryKey}
}

func newAdaptiveBootstrapDecisionForTest(
	t *testing.T,
	admission entity.AdaptiveAdmissionSnapshot,
	decisionID string,
	executionRunID, journalRunID int64,
	attemptID string,
	generation uint64,
	planScopeRunID, createdAt int64,
) entity.ExecutionDecision {
	t.Helper()
	decision := newAdaptiveExecutionBootstrapRequestForTest().Decision
	decision.DecisionID = decisionID
	decision.DecisionRevision = 1
	decision.ExecutionRunID = executionRunID
	decision.JournalRunID = journalRunID
	decision.AttemptID = attemptID
	decision.ExecutionGeneration = generation
	decision.PlanScopeRunID = &planScopeRunID
	decision.CreatedAt = createdAt
	err := adaptivecontract.ValidateAdaptiveBootstrapPair(admission, decision, adaptivecontract.BootstrapIdentity{
		ExecutionRunID: executionRunID, JournalRunID: journalRunID,
		AttemptID: attemptID, ExecutionGeneration: generation,
	})
	require.NoError(t, err)
	return decision
}
```

Repository 生产与测试均不得导入 application package，保持 DDD 依赖方向。

- [ ] **Step 2: Write RED tests for typed first-write, strict readback and multi-hop**

新增：

```go
func TestAdaptiveExecutionBootstrapCommitsTypedRecoveryTarget(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	fixture := seedAdaptiveBootstrapRecoveryTargetForTest(t, db)
	repo := NewAdaptiveExecutionRepository(db)

	result, err := repo.CommitAdaptiveExecutionBootstrap(context.Background(), fixture.TargetRequest)
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.Equal(t, entity.AdaptiveAdmissionSourceTypedInheritance, result.Admission.Source)
	require.Equal(t, int64(20), *result.Admission.SourceRunID)
	require.Equal(t, fixture.SourceResult.Authority.ExecutionGeneration, *result.Admission.SourceExecutionGeneration)

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", result.Authority.CheckpointID).First(&checkpoint).Error)
	metadata, err := decodeAdaptiveBootstrapMetadata(checkpoint.Metadata)
	require.NoError(t, err)
	require.Equal(t, fixture.SourceAttemptID, *metadata.SourceAttemptID)
	require.Equal(t, fixture.SourceCheckpoint, *metadata.SourceCheckpointID)
	require.Equal(t, fixture.RecoveryKey, *metadata.RecoveryIdempotencyKey)

	read, err := repo.ReadAdaptiveExecutionBootstrap(context.Background(), ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: 10, ExecutionRunID: 21, JournalRunID: 30, AttemptID: "attempt-2",
	})
	require.NoError(t, err)
	require.Equal(t, result.Admission, read.Admission)
	require.Equal(t, result.Decision, read.Decision)
}
```

再新增 `TestAdaptiveExecutionBootstrapCommitsSecondTypedRecoveryHop`：终结 target-1、创建 Run 22 / Attempt `attempt-3` / Eino checkpoint，并以 target-1 authority 生成第二个 typed admission；断言 source generation 是 4、target generation 是 5，且 target-2 读回成功。

- [ ] **Step 3: Write RED table tests for zero-write conflicts and replay priority**

新增以下真实 DB 用例，每例调用前后比较 `snapshotAdaptiveExecutionDBForTest`：

```go
func TestAdaptiveExecutionBootstrapRejectsInvalidTypedRecoveryLineageWithoutWrites(t *testing.T) {
	tests := []struct {
		name string
		mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBootstrapRequest)
	}{
		{"partial lineage", func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBootstrapRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("attempt_id = ?", "attempt-2").Update("recovery_idempotency_key", nil).Error)
		}},
		{"source run drift", func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBootstrapRequest) { *req.Admission.SourceRunID = 99 }},
		{"source generation drift", func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBootstrapRequest) {
			value := *req.Admission.SourceExecutionGeneration + 1
			req.Admission.SourceExecutionGeneration = &value
		}},
		{"source checkpoint drift", func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBootstrapRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", int64(9001)).Update("run_id", int64(21)).Error)
		}},
		{"self source", func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBootstrapRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("attempt_id = ?", "attempt-2").Update("source_attempt_id", "attempt-2").Error)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			fixture := seedAdaptiveBootstrapRecoveryTargetForTest(t, db)
			test.mutate(t, db, &fixture.TargetRequest)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBootstrap(context.Background(), fixture.TargetRequest)
			require.ErrorIs(t, err, ErrAdaptiveExecutionBootstrapConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}
```

另加 `TestAdaptiveExecutionBootstrapTypedReplaySkipsMutableSourceAndTargetFences`：first commit 后损坏 source checkpoint、把 target Run/Attempt terminal，再提交同 canonical request；断言 `Replayed=true`、authority 相同且无新增写入。再篡改 target metadata lineage，断言 replay/read 都是 `ErrAdaptiveExecutionBootstrapConflict`。

- [ ] **Step 4: Run the repository RED tests**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./domain/agentthread/repository \
  -run '^TestAdaptiveExecutionBootstrap(CommitsTypedRecoveryTarget|CommitsSecondTypedRecoveryHop|RejectsInvalidTypedRecoveryLineageWithoutWrites|TypedReplaySkipsMutableSourceAndTargetFences)$' -count=1
```

Expected: FAIL because typed admission is rejected as invalid/fresh-only, or because typed Attempt lineage is rejected.

- [ ] **Step 5: Implement the two closed lineage shapes**

在 `normalizeAdaptiveExecutionBootstrapRequest` 中只接受 fresh/typed，继续拒绝 legacy：

```go
switch req.Admission.Source {
case entity.AdaptiveAdmissionSourceFresh, entity.AdaptiveAdmissionSourceTypedInheritance:
default:
	return nil, bootstrapInvalidf("bootstrap admission source is unsupported")
}
```

增加封闭 lineage validator，并在 loader 解码 admission 后调用：

```go
func validateAdaptiveBootstrapStoredLineage(
	attempt *runAttemptPO,
	metadata adaptiveBootstrapMetadata,
	admission entity.AdaptiveAdmissionSnapshot,
) error {
	if attempt == nil {
		return bootstrapConflictf("bootstrap attempt is missing")
	}
	sourceAttemptPresent := attempt.SourceAttemptID != nil
	sourceCheckpointPresent := attempt.SourceCheckpointID != nil
	recoveryKeyPresent := attempt.RecoveryIdempotencyKey != nil
	if sourceAttemptPresent != sourceCheckpointPresent || sourceAttemptPresent != recoveryKeyPresent {
		return bootstrapConflictf("bootstrap attempt lineage is partial")
	}
	if !sourceAttemptPresent {
		if admission.Source != entity.AdaptiveAdmissionSourceFresh ||
			metadata.SourceAttemptID != nil || metadata.SourceCheckpointID != nil || metadata.RecoveryIdempotencyKey != nil {
			return bootstrapConflictf("fresh bootstrap lineage drift")
		}
		return nil
	}
	if admission.Source != entity.AdaptiveAdmissionSourceTypedInheritance ||
		!adaptiveExecutionStringPointersEqual(metadata.SourceAttemptID, attempt.SourceAttemptID) ||
		!adaptiveExecutionInt64PointersEqual(metadata.SourceCheckpointID, attempt.SourceCheckpointID) ||
		!adaptiveExecutionStringPointersEqual(metadata.RecoveryIdempotencyKey, attempt.RecoveryIdempotencyKey) {
		return bootstrapConflictf("typed bootstrap lineage drift")
	}
	return nil
}
```

`newAdaptiveExecutionBootstrapRows` 接收 locked target Attempt，并把三项 lineage clone 到 metadata。fresh 保持三项 `null`。

- [ ] **Step 6: Implement typed first-write source attestation inside the existing transaction**

增加 `lockAndValidateAdaptiveBootstrapTypedSource`，复用现有 source discovery/row locks：

```go
func lockAndValidateAdaptiveBootstrapTypedSource(
	tx *gorm.DB,
	normalized *adaptiveExecutionBootstrapNormalizedRequest,
	target *runAttemptPO,
) error {
	req := normalized.request
	if target.SourceAttemptID == nil || target.SourceCheckpointID == nil || target.RecoveryIdempotencyKey == nil {
		return bootstrapConflictf("typed bootstrap lineage is incomplete")
	}
	discovered, err := discoverAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *target.SourceAttemptID)
	if err != nil { return err }
	sourceRun, err := lockAdaptiveExecutionSourceRun(tx, discovered.ExecutionRunID)
	if err != nil { return err }
	sourceAttempt, err := lockAdaptiveExecutionSourceAttempt(tx, req.JournalRunID, *target.SourceAttemptID)
	if err != nil { return err }
	if sourceAttempt.ID != discovered.ID || sourceAttempt.ThreadID != req.ThreadID ||
		sourceAttempt.JournalRunID != req.JournalRunID || sourceAttempt.Ordinal == 0 ||
		target.Ordinal == 0 || sourceAttempt.Ordinal >= target.Ordinal || sourceAttempt.ExecutionRunID == req.ExecutionRunID {
		return bootstrapConflictf("typed bootstrap source attempt drift")
	}
	sourceCheckpoint, err := lockAdaptiveExecutionSourceCheckpoint(tx, *target.SourceCheckpointID)
	if err != nil { return err }
	if sourceCheckpoint.ThreadID != req.ThreadID || sourceCheckpoint.RunID != sourceAttempt.ExecutionRunID ||
		sourceCheckpoint.RuntimeType != "eino_adk" || sourceCheckpoint.RuntimeDeletedAt != 0 {
		return bootstrapConflictf("typed bootstrap source checkpoint drift")
	}
	source, err := loadAdaptiveExecutionBootstrapResult(tx, ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: req.ThreadID, ExecutionRunID: sourceAttempt.ExecutionRunID,
		JournalRunID: req.JournalRunID, AttemptID: sourceAttempt.AttemptID,
	}, nil)
	if err != nil { return bootstrapConflictf("typed bootstrap source facts are invalid: %v", err) }
	if source == nil || source.Authority.ExecutionGeneration != sourceRun.ExecutionGeneration ||
		source.Admission.FeatureGateEnabled ||
		(source.Admission.Source != entity.AdaptiveAdmissionSourceFresh &&
			source.Admission.Source != entity.AdaptiveAdmissionSourceTypedInheritance) ||
		req.Admission.SourceRunID == nil || *req.Admission.SourceRunID != sourceAttempt.ExecutionRunID ||
		req.Admission.SourceExecutionGeneration == nil ||
		*req.Admission.SourceExecutionGeneration != source.Authority.ExecutionGeneration ||
		req.Admission.Capabilities != source.Admission.Capabilities || req.Admission.Limits != source.Admission.Limits {
		return bootstrapConflictf("typed bootstrap inherited policy drift")
	}
	return nil
}
```

在 complete replay/partial/reserved checks 之后保持现有 target Run/Attempt fence；fresh 验证三项 nil，typed 调用上面的 helper。只有 attestation 成功才创建 rows。

- [ ] **Step 7: Run focused and existing bootstrap tests GREEN**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' \
  ./domain/agentthread/repository -run '^TestAdaptiveExecutionBootstrap' -count=1
```

Expected: PASS. Existing `TestAdaptiveExecutionBootstrapRejectsNonFreshAdmissionWithoutWrites` must be narrowed to assert legacy remains rejected; typed inheritance moves to the positive recovery tests.

- [ ] **Step 8: Commit the repository slice**

```bash
git add backend/domain/agentthread/repository/mysql_adaptive_bootstrap.go \
  backend/domain/agentthread/repository/mysql_adaptive_bootstrap_test.go
git commit -m "feat: persist typed recovery bootstrap facts"
```

### Task 2: TDD the Resume coordinator

**Files:**
- Modify: `backend/application/agentthread/adaptive_bootstrap_coordinator.go`
- Modify: `backend/application/agentthread/adaptive_bootstrap_coordinator_test.go`

- [ ] **Step 1: Extend the coordinator contract without changing Execute behavior**

接口增加 Resume 方法；函数适配器在 Resume 被误用时明确 fail closed：

```go
type AdaptiveBootstrapCoordinator interface {
	Bootstrap(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error)
	BootstrapResume(context.Context, *RunSummary, *HarnessResumeInput) (*AdaptiveBootstrapFacts, error)
}

func (f AdaptiveBootstrapCoordinatorFunc) BootstrapResume(
	context.Context,
	*RunSummary,
	*HarnessResumeInput,
) (*AdaptiveBootstrapFacts, error) {
	return nil, fmt.Errorf("adaptive resume bootstrap coordinator function is required")
}
```

- [ ] **Step 2: Write RED coordinator tests for target-first replay and typed inheritance**

扩展 repository stub，按 read request queue 返回 target/source 结果并记录调用。新增：

```go
func TestAdaptiveBootstrapCoordinatorResumeReplaysTargetBeforeSourceAndIDs(t *testing.T) {
	run, input, attempt := adaptiveBootstrapRecoveryResumeForTest()
	target := adaptiveBootstrapTypedResultForTest(t, run, input, attempt, 3)
	repo := &adaptiveBootstrapRepositoryStub{readResults: []*repository.CommitAdaptiveExecutionBootstrapResult{target}}
	ids := &adaptiveBootstrapIDGeneratorStub{ids: []int64{101, 102, 103}}
	coordinator := NewAdaptiveBootstrapCoordinator(AdaptiveBootstrapCoordinatorOptions{
		AttemptReader: &adaptiveBootstrapAttemptReaderStub{attempt: attempt},
		Repository: repo, IDGen: ids, Now: func() int64 { return 999 },
	})

	facts, err := coordinator.BootstrapResume(context.Background(), run, input)
	require.NoError(t, err)
	require.Equal(t, target.Admission, facts.Admission)
	require.Len(t, repo.readRequests, 1)
	require.Empty(t, ids.counts)
	require.Empty(t, repo.commitRequests)
}
```

新增 `TestAdaptiveBootstrapCoordinatorResumeInheritsTypedSource`：target read 返回 NotFound，source read 返回 valid fresh/typed result；断言 read 顺序 target then source、target admission 为 typed、SourceRunID/SourceGeneration 来自 source authority、capabilities/limits 完全相同、target decision revision=1 且 identity/PlanScope/CreatedAt 全属于 target、最后才分配 3 IDs 和 commit。

- [ ] **Step 3: Write RED fail-closed matrix**

表驱动覆盖：not enrolled => nil/no-op；partial target lineage；input checkpoint drift；input source Run drift；target/source read Conflict；source NotFound；source gate-on；source legacy；nil source result；nil commit result。每例断言没有 fallback Config 读取、没有不应发生的 ID allocation/commit。

- [ ] **Step 4: Run coordinator RED tests**

Run:

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread \
  -run '^TestAdaptiveBootstrapCoordinatorResume' -count=1
```

Expected: FAIL to compile because `BootstrapResume` does not exist.

- [ ] **Step 5: Implement `BootstrapResume` target-first flow**

核心顺序：

```go
func (c *adaptiveBootstrapCoordinator) BootstrapResume(
	ctx context.Context,
	run *RunSummary,
	input *HarnessResumeInput,
) (*AdaptiveBootstrapFacts, error) {
	if run == nil || input == nil || c == nil || c.attemptReader == nil || c.repository == nil || c.idGen == nil || c.now == nil {
		return nil, fmt.Errorf("adaptive resume bootstrap dependencies are required")
	}
	attempt, err := c.attemptReader.GetActiveJournalAttempt(ctx, run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) { return nil, nil }
	if err != nil { return nil, fmt.Errorf("read active journal attempt for adaptive resume bootstrap: %w", err) }
	if err := validateAdaptiveBootstrapRecoveryResume(run, input, attempt); err != nil { return nil, err }

	targetRead := domainrepo.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: run.ThreadID, ExecutionRunID: run.RunID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
	}
	target, err := c.repository.ReadAdaptiveExecutionBootstrap(ctx, targetRead)
	if err == nil { return adaptiveBootstrapRecoveryFactsFromDurableResult(run, input, attempt, target) }
	if !errors.Is(err, domainrepo.ErrAdaptiveExecutionBootstrapNotFound) {
		return nil, fmt.Errorf("read target adaptive execution bootstrap: %w", err)
	}

	source, err := c.repository.ReadAdaptiveExecutionBootstrap(ctx, domainrepo.ReadAdaptiveExecutionBootstrapRequest{
		ThreadID: run.ThreadID, ExecutionRunID: input.SourceRunID,
		JournalRunID: attempt.JournalRunID, AttemptID: *attempt.SourceAttemptID,
	})
	if err != nil { return nil, fmt.Errorf("read source adaptive execution bootstrap: %w", err) }
	admission, err := typedAdaptiveAdmissionFromSource(input.SourceRunID, source)
	if err != nil { return nil, err }
	decision, err := (BaselineDecisionProducer{}).Produce(BaselineDecisionRequest{
		Admission: admission, DecisionID: adaptiveBootstrapStableKey("decision", run, attempt),
		DecisionRevision: 1, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, ExecutionGeneration: run.ExecutionGeneration,
		PlanScopeRunID: run.RunID, CreatedAt: attempt.CreatedAt,
	})
	if err != nil { return nil, fmt.Errorf("produce recovery adaptive decision: %w", err) }
	ids, err := c.idGen.GenMultiIDs(ctx, 3)
	if err != nil { return nil, fmt.Errorf("allocate recovery bootstrap identifiers: %w", err) }
	committed, err := c.repository.CommitAdaptiveExecutionBootstrap(ctx, domainrepo.CommitAdaptiveExecutionBootstrapRequest{
		ThreadID: run.ThreadID, ExecutionRunID: run.RunID, JournalRunID: attempt.JournalRunID,
		AttemptID: attempt.AttemptID, LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		OperationKey: adaptiveBootstrapStableKey("operation", run, attempt), Generation: run.ExecutionGeneration,
		Now: c.now(), FactCreatedAt: attempt.CreatedAt, Admission: admission, Decision: decision,
		AdmissionEventID: ids[0], DecisionEventID: ids[1], CheckpointID: ids[2],
	})
	if err != nil { return nil, fmt.Errorf("commit recovery adaptive bootstrap: %w", err) }
	return adaptiveBootstrapRecoveryFactsFromDurableResult(run, input, attempt, committed)
}
```

Validation requires target Thread/Run/lease/generation, active Attempt, all three lineage fields, `input.ThreadID/RunID`, exact source checkpoint ID and distinct positive source Run. Source accepts only fresh/typed, gate-off, strict pair/authority; no Config access and no legacy fallback.

配套 helper 必须按下面的完整责任实现，不得复用 fresh-only validator：

```go
func typedAdaptiveAdmissionFromSource(
	sourceRunID int64,
	source *domainrepo.CommitAdaptiveExecutionBootstrapResult,
) (domainentity.AdaptiveAdmissionSnapshot, error) {
	if source == nil || sourceRunID <= 0 || source.Authority.ExecutionRunID != sourceRunID ||
		source.Authority.ExecutionGeneration == 0 || source.Admission.FeatureGateEnabled ||
		(source.Admission.Source != domainentity.AdaptiveAdmissionSourceFresh &&
			source.Admission.Source != domainentity.AdaptiveAdmissionSourceTypedInheritance) {
		return domainentity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("adaptive recovery source facts are invalid")
	}
	if err := ValidateExecutionDecisionAgainstAdmission(source.Admission, source.Decision); err != nil {
		return domainentity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("validate adaptive recovery source facts: %w", err)
	}
	sourceGeneration := source.Authority.ExecutionGeneration
	return domainentity.AdaptiveAdmissionSnapshot{
		Schema: domainentity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled: false,
		Source: domainentity.AdaptiveAdmissionSourceTypedInheritance,
		SourceRunID: &sourceRunID,
		SourceExecutionGeneration: &sourceGeneration,
		Capabilities: source.Admission.Capabilities,
		Limits: source.Admission.Limits,
	}, nil
}

func adaptiveBootstrapRecoveryFactsFromDurableResult(
	run *RunSummary,
	input *HarnessResumeInput,
	attempt *domainentity.RunAttempt,
	result *domainrepo.CommitAdaptiveExecutionBootstrapResult,
) (*AdaptiveBootstrapFacts, error) {
	if result == nil || run == nil || input == nil || attempt == nil ||
		result.Admission.Source != domainentity.AdaptiveAdmissionSourceTypedInheritance ||
		result.Admission.FeatureGateEnabled || result.Admission.SourceRunID == nil ||
		*result.Admission.SourceRunID != input.SourceRunID ||
		result.Authority.ThreadID != run.ThreadID || result.Authority.ExecutionRunID != run.RunID ||
		result.Authority.JournalRunID != attempt.JournalRunID || result.Authority.AttemptID != attempt.AttemptID ||
		result.Authority.ExecutionGeneration != run.ExecutionGeneration ||
		result.Decision.ExecutionRunID != run.RunID || result.Decision.JournalRunID != attempt.JournalRunID ||
		result.Decision.AttemptID != attempt.AttemptID ||
		result.Decision.ExecutionGeneration != run.ExecutionGeneration || result.Decision.DecisionRevision != 1 ||
		result.Decision.PlanScopeRunID == nil || *result.Decision.PlanScopeRunID != run.RunID {
		return nil, fmt.Errorf("adaptive recovery bootstrap facts do not match the current target")
	}
	if err := ValidateExecutionDecisionAgainstAdmission(result.Admission, result.Decision); err != nil {
		return nil, fmt.Errorf("validate adaptive recovery bootstrap facts: %w", err)
	}
	return cloneAdaptiveBootstrapFacts(&AdaptiveBootstrapFacts{Admission: result.Admission, Decision: result.Decision}), nil
}
```

- [ ] **Step 6: Run coordinator focused tests GREEN**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread \
  -run '^TestAdaptiveBootstrapCoordinator(CommitsFreshRootADKRun|ReplaysBeforeAllocatingIDs|Resume.*)$' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit the coordinator slice**

```bash
git add backend/application/agentthread/adaptive_bootstrap_coordinator.go \
  backend/application/agentthread/adaptive_bootstrap_coordinator_test.go
git commit -m "feat: inherit adaptive facts for journal resume"
```

### Task 3: TDD the ADK Resume pre-build seam

**Files:**
- Modify: `backend/application/agentthread/adk_executor.go`
- Modify: `backend/application/agentthread/adk_executor_test.go`

- [ ] **Step 1: Add a test coordinator that records Execute and Resume independently**

```go
type recordingAdaptiveBootstrapCoordinator struct {
	resumeFacts *AdaptiveBootstrapFacts
	resumeErr error
	resumeCalls int
}

func (c *recordingAdaptiveBootstrapCoordinator) Bootstrap(context.Context, *RunSummary) (*AdaptiveBootstrapFacts, error) {
	return nil, nil
}

func (c *recordingAdaptiveBootstrapCoordinator) BootstrapResume(
	context.Context, *RunSummary, *HarnessResumeInput,
) (*AdaptiveBootstrapFacts, error) {
	c.resumeCalls++
	return c.resumeFacts, c.resumeErr
}
```

- [ ] **Step 2: Write RED ordering and error tests**

新增 `TestADKExecutorResumeBootstrapsBeforeBuildingRuntime`：用 valid Resume envelope/input，coordinator 返回 typed target facts；checkpoint-store factory 和 agent factory 分别记录 `store`/`factory`，agent factory 从 context 取 facts；断言顺序 `resume-bootstrap, store, factory`。

新增 `TestADKExecutorResumeStopsBeforeBuildingRuntimeWhenBootstrapFails`：coordinator 返回 sentinel error；断言 result nil、`errors.Is`、store/factory 调用均 0。

- [ ] **Step 3: Run executor RED tests**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread \
  -run '^TestADKExecutorResume(BootstrapsBeforeBuildingRuntime|StopsBeforeBuildingRuntimeWhenBootstrapFails)$' -count=1
```

Expected: FAIL because Resume does not call the coordinator and the factory context has no target facts.

- [ ] **Step 4: Add the Resume coordinator call after existing source-run validation**

在 `ADKExecutor.Resume` 的 `adkAgentRunForResume` 成功之后、parity/buildRuntime 之前：

```go
	if e.adaptiveBootstrapCoordinator != nil {
		facts, bootstrapErr := e.adaptiveBootstrapCoordinator.BootstrapResume(executionCtx, run, input)
		if bootstrapErr != nil {
			return nil, bootstrapErr
		}
		executionCtx = withAdaptiveBootstrapFacts(executionCtx, facts)
	}
```

不要移动既有 runtime/envelope/runtime-key/source-run validation，不改 Execute seam，不把 facts 写入 Config/Metadata。

- [ ] **Step 5: Run focused Resume and Execute regression tests GREEN**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread \
  -run '^(TestADKExecutor(BootstrapsBeforeBuildingRuntime|StopsBeforeBuildingRuntimeWhenBootstrapFails|ResumeBootstrapsBeforeBuildingRuntime|ResumeStopsBeforeBuildingRuntimeWhenBootstrapFails|InterruptsAndResumesWithTargets))$' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the executor slice**

```bash
git add backend/application/agentthread/adk_executor.go \
  backend/application/agentthread/adk_executor_test.go
git commit -m "feat: gate journal resume before adk runtime build"
```

### Task 4: MySQL gate, authority, verification and final review

**Files:**
- Modify: `backend/domain/agentthread/repository/mysql_adaptive_bootstrap_integration_test.go`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/context/workbench-execution-chain.md`
- Modify: `docs/superpowers/context/workbench-execution-graph.json`
- Modify: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md`
- Modify: `scripts/workbench-execution-graph/contract.mjs`

- [ ] **Step 1: Extend the gated MySQL test with a typed target race**

在 existing fixture 提供真实 disposable MySQL 时，seed source bootstrap + recovery target，用两个独立 repository 并发提交同 canonical target request；断言一个首次写、另一个 replay，authority 完全相同，reserved events=2、control checkpoint=1，且没有 raw 1213/1205/1062。若 fixture gate 缺失，保留明确 Skip；输出中不得打印 DSN。

- [ ] **Step 2: Run the MySQL test and classify evidence honestly**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -json -p 1 -gcflags='all=-l -N' \
  ./domain/agentthread/repository -run '^TestAdaptiveExecutionBootstrapMySQLIntegrationTypedRecoveryRace$' -count=1
```

Expected with configured disposable DSN/DDL gate: leaf test PASS and not Skip. Expected in the current unconfigured environment: Skip, recorded as `NOT_VERIFIED`; do not block the local slice commit and do not claim P1M PASS.

- [ ] **Step 3: Run required Go suites**

```bash
cd backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./domain/agentthread/repository -run '^TestAdaptiveExecutionBootstrap' -count=1
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags='all=-l -N' ./application/agentthread -count=1
```

Expected: PASS. Any sandbox-only loopback bind denial must be rerun with the approved Go test escalation; no unrelated production changes may be used to hide a failure.

- [ ] **Step 4: Update authority without overstating completion**

Authority text must say exactly:

```text
C3h2a connects only an already-enrolled Journal recovery Resume whose immediate source has a valid fresh/typed durable bootstrap. The target commits or exactly replays a gate-off typed-inheritance snapshot before ADK buildRuntime. Legacy fallback, Human rollover, ordinary non-Journal enrollment, IDL/UI and gate-on producer remain deferred. P1M is not PASS; real MySQL concurrency remains NOT_VERIFIED when the gated test skips; P1L and the whole-Thread DELETE hard guard are unchanged.
```

Graph：复用 existing adaptive boundary node；新增/更新 Resume `delegates_to` 与 `precedes buildRuntime` evidence，只锚定实际 Go symbol/tests。不新增 Human/legacy/IDL/UI edge。主计划勾选 C3h2a，但保持 delivery-first 约束和所有 deferred 项。

- [ ] **Step 5: Run graph and diff verification**

```bash
jq -e . docs/superpowers/context/workbench-execution-graph.json >/dev/null
git diff --check
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

Expected: all commands exit 0. Generated/Graphify outputs remain ignored and must not be staged.

- [ ] **Step 6: Review exact scope and commit authority/integration evidence**

Required independent review: first spec compliance against the frozen C3h2a design, then code quality/security review of repository lock/replay ordering and Resume fail-closed ordering. P0/P1 must be zero before commit.

```bash
git status --short
git diff --check
git add backend/domain/agentthread/repository/mysql_adaptive_bootstrap_integration_test.go \
  docs/superpowers/context/project-context.md \
  docs/superpowers/context/workbench-execution-chain.md \
  docs/superpowers/context/workbench-execution-graph.json \
  docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md \
  scripts/workbench-execution-graph/contract.mjs
git commit -m "docs: record typed journal resume bootstrap"
```

Before staging, verify the two user-owned untracked plans are still unmodified and excluded. Do not read, stage, delete or commit them.

## Completion evidence

C3h2a is locally deliverable when:

- repository SQLite bootstrap tests and full application package pass;
- target replay, multi-hop inheritance and runtime-before-build ordering are directly tested;
- execution authority matches the exact production edge;
- graph verify/build/verify-derived pass;
- independent spec and quality reviews report no P0/P1;
- real MySQL is either true PASS or explicitly `NOT_VERIFIED` due missing gated disposable credentials;
- P1M remains locked and deferred scope remains unimplemented.
