// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
)

func TestRepositoryCRUDAndTenantIsolation(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	ctx := context.Background()

	task := taskFixture(10, 100, "morning report")
	require.NoError(t, repo.CreateTask(ctx, task))

	got, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, task.Name, got.Name)
	_, err = repo.GetTask(ctx, 11, task.ID)
	require.Error(t, err)

	tasks, total, err := repo.ListTasks(ctx, ListTaskFilter{
		SpaceID:  10,
		Keyword:  "report",
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, tasks, 1)

	require.NoError(t, repo.SoftDeleteTask(ctx, 10, task.ID))
	_, err = repo.GetTask(ctx, 10, task.ID)
	require.Error(t, err)
	_, total, err = repo.ListTasks(ctx, ListTaskFilter{SpaceID: 10})
	require.NoError(t, err)
	require.Zero(t, total)
}

func TestRepositoryClaimsDueTasksWithLeaseCAS(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC).UnixMilli()

	due := taskFixture(10, 100, "due")
	due.NextExecutionAt = now - 1
	future := taskFixture(10, 100, "future")
	future.NextExecutionAt = now + 60_000
	disabled := taskFixture(10, 100, "disabled")
	disabled.NextExecutionAt = now - 1
	disabled.Status = entity.StatusDisabled

	for _, task := range []*entity.Task{due, future, disabled} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}

	claimed, err := repo.ClaimDueTasks(ctx, now, "worker-a", now+30_000, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, due.ID, claimed[0].ID)
	require.Equal(t, "worker-a", claimed[0].LeaseOwner)

	claimed, err = repo.ClaimDueTasks(ctx, now, "worker-b", now+30_000, 10)
	require.NoError(t, err)
	require.Empty(t, claimed)

	claimed, err = repo.ClaimDueTasks(ctx, now+30_001, "worker-b", now+60_000, 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, "worker-b", claimed[0].LeaseOwner)
}

func TestRepositoryExecutionIdempotencyAndHistory(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	ctx := context.Background()
	task := taskFixture(10, 100, "report")
	require.NoError(t, repo.CreateTask(ctx, task))

	execution := &entity.Execution{
		TaskID:         task.ID,
		SpaceID:        10,
		TriggerType:    "schedule",
		ScheduledAt:    1000,
		Status:         entity.ExecutionStatusQueued,
		IdempotencyKey: "schedule:1000",
	}
	require.NoError(t, repo.CreateExecution(ctx, execution))
	queuedTask, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusQueued, queuedTask.LatestExecutionStatus)
	require.ErrorIs(t, repo.CreateExecution(ctx, &entity.Execution{
		TaskID:         task.ID,
		SpaceID:        10,
		TriggerType:    "schedule",
		ScheduledAt:    1000,
		Status:         entity.ExecutionStatusQueued,
		IdempotencyKey: "schedule:1000",
	}), ErrExecutionAlreadyExists)

	require.NoError(t, repo.MarkExecutionRunning(ctx, execution.ID, 1200))
	runningTask, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusRunning, runningTask.LatestExecutionStatus)
	require.NoError(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{
		Status:     entity.ExecutionStatusSucceeded,
		ThreadID:   21,
		RunID:      22,
		FinishedAt: 1500,
	}))
	finishedTask, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusSucceeded, finishedTask.LatestExecutionStatus)
	require.Equal(t, int64(1), finishedTask.ExecutionCount)
	require.Equal(t, int64(1500), finishedTask.LatestExecutionAt)

	history, total, err := repo.ListExecutions(ctx, 10, task.ID, 1, 20)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, history, 1)
	require.Equal(t, entity.ExecutionStatusSucceeded, history[0].Status)
	require.Equal(t, int64(21), history[0].ThreadID)
	require.Equal(t, int64(22), history[0].RunID)
}

func TestRepositoryOlderExecutionCannotOverwriteLatestStatus(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	ctx := context.Background()
	task := taskFixture(10, 100, "concurrent-report")
	task.UpdatedAt = 5000
	require.NoError(t, repo.CreateTask(ctx, task))
	first := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "manual", Status: entity.ExecutionStatusQueued, IdempotencyKey: "manual:first"}
	second := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "manual", Status: entity.ExecutionStatusQueued, IdempotencyKey: "manual:second"}
	require.NoError(t, repo.CreateExecution(ctx, first))
	require.NoError(t, repo.CreateExecution(ctx, second))

	require.NoError(t, repo.MarkExecutionRunning(ctx, first.ID, 1200))
	latest, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusQueued, latest.LatestExecutionStatus)

	require.NoError(t, repo.FinalizeExecution(ctx, first.ID, ExecutionResult{Status: entity.ExecutionStatusSucceeded, FinishedAt: 1500}))
	latest, err = repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusQueued, latest.LatestExecutionStatus)
	require.Zero(t, latest.LatestExecutionAt)
	require.Equal(t, int64(1), latest.ExecutionCount)
	require.Equal(t, int64(5000), latest.UpdatedAt)
}

func TestRepositoryFinalizeExecutionAppendsOwnerNotificationAtomically(t *testing.T) {
	t.Parallel()
	outbox := &recordingNotificationOutbox{}
	repo := newTestRepositoryWithNotification(t, outbox)
	ctx := context.Background()
	task := taskFixture(10, 100, "notify-owner")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "notify-owner"}
	require.NoError(t, repo.CreateExecution(ctx, execution))
	require.NoError(t, repo.MarkExecutionRunning(ctx, execution.ID, 1200))

	require.NoError(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusSucceeded, ThreadID: 21, RunID: 22, FinishedAt: 1500}))

	got, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.ExecutionCount)
	require.Equal(t, int64(1500), got.LatestExecutionAt)
	require.Equal(t, entity.ExecutionStatusSucceeded, got.LatestExecutionStatus)
	require.Len(t, outbox.events, 1)
	event := outbox.events[0]
	require.Equal(t, "scheduled_execution:"+idString(execution.ID)+":succeeded", event.EventID)
	require.Equal(t, domainnotification.EventScheduledExecutionSucceeded, event.EventType)
	require.Equal(t, "scheduled_task_execution", event.AggregateType)
	require.Equal(t, idString(execution.ID), event.AggregateID)
	require.Equal(t, int64(100), event.ActorID)
	require.Equal(t, int64(10), event.SpaceID)
	require.Equal(t, domainnotification.RecipientActor, event.RecipientPolicy)
	require.Equal(t, idString(task.ID), event.Payload.TargetID)
}

func TestRepositoryFinalizeExecutionDuplicateCallbackDoesNotReappendOrRecount(t *testing.T) {
	t.Parallel()
	outbox := &recordingNotificationOutbox{}
	repo := newTestRepositoryWithNotification(t, outbox)
	ctx := context.Background()
	task := taskFixture(10, 100, "duplicate")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "duplicate"}
	require.NoError(t, repo.CreateExecution(ctx, execution))

	require.NoError(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusFailed, FinishedAt: 1500}))
	require.NoError(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusFailed, FinishedAt: 1600}))

	got, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.ExecutionCount)
	require.Equal(t, int64(1500), got.LatestExecutionAt)
	require.Len(t, outbox.events, 1)
	require.Equal(t, domainnotification.EventScheduledExecutionFailed, outbox.events[0].EventType)
}

func TestRepositoryFinalizeExecutionCASMissTreatsSameTerminalAsIdempotent(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	mysqlRepo := repo.(*mysqlRepository)
	ctx := context.Background()
	task := taskFixture(10, 100, "cas-miss")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "cas-miss"}
	require.NoError(t, repo.CreateExecution(ctx, execution))
	require.NoError(t, mysqlRepo.db.WithContext(ctx).Model(&scheduledTaskExecutionPO{}).Where("id = ?", execution.ID).Update("status", string(entity.ExecutionStatusFailed)).Error)

	require.NoError(t, finalizeExecutionCASMiss(mysqlRepo.db.WithContext(ctx), execution.ID, entity.ExecutionStatusFailed))
	require.ErrorIs(t, finalizeExecutionCASMiss(mysqlRepo.db.WithContext(ctx), execution.ID, entity.ExecutionStatusSucceeded), ErrStateConflict)
}

func TestRepositoryFinalizeExecutionRejectsConflictingTerminalReplay(t *testing.T) {
	t.Parallel()
	outbox := &recordingNotificationOutbox{}
	repo := newTestRepositoryWithNotification(t, outbox)
	ctx := context.Background()
	task := taskFixture(10, 100, "conflict")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "conflict"}
	require.NoError(t, repo.CreateExecution(ctx, execution))

	require.NoError(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusSucceeded, FinishedAt: 1500}))
	require.ErrorIs(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusFailed, FinishedAt: 1600}), ErrStateConflict)

	got, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), got.ExecutionCount)
	require.Equal(t, entity.ExecutionStatusSucceeded, got.LatestExecutionStatus)
	require.Len(t, outbox.events, 1)
}

func TestRepositoryFinalizeExecutionRollsBackWhenNotificationAppendFails(t *testing.T) {
	t.Parallel()
	outbox := &recordingNotificationOutbox{err: errors.New("outbox unavailable")}
	repo := newTestRepositoryWithNotification(t, outbox)
	ctx := context.Background()
	task := taskFixture(10, 100, "rollback")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "rollback"}
	require.NoError(t, repo.CreateExecution(ctx, execution))
	require.NoError(t, repo.MarkExecutionRunning(ctx, execution.ID, 1200))
	require.NoError(t, repo.SetWorkflowExecutionID(ctx, execution.ID, 50))

	require.Error(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusSucceeded, FinishedAt: 1500}))

	gotExecution, err := repo.GetExecution(ctx, execution.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusRunning, gotExecution.Status)
	require.Equal(t, int64(50), gotExecution.WorkflowExecutionID)
	gotTask, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(0), gotTask.ExecutionCount)
	require.Equal(t, entity.ExecutionStatusRunning, gotTask.LatestExecutionStatus)
	require.Len(t, outbox.events, 1)
}

func TestRepositoryFinalizeExecutionFailsWithoutNotificationOutboxForNotifiableStatus(t *testing.T) {
	t.Parallel()
	repo := newTestRepositoryWithoutNotification(t)
	ctx := context.Background()
	task := taskFixture(10, 100, "missing-outbox")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "missing-outbox"}
	require.NoError(t, repo.CreateExecution(ctx, execution))

	require.ErrorIs(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusSucceeded, FinishedAt: 1500}), domainnotification.ErrStorage)

	gotExecution, err := repo.GetExecution(ctx, execution.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusQueued, gotExecution.Status)
	gotTask, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Zero(t, gotTask.ExecutionCount)
}

func TestRepositoryFinalizeExecutionUsesScheduledEventForAgentThreadTerminalResult(t *testing.T) {
	t.Parallel()
	outbox := &recordingNotificationOutbox{}
	repo := newTestRepositoryWithNotification(t, outbox)
	ctx := context.Background()
	task := taskFixture(10, 100, "agent-thread-suppressed")
	require.NoError(t, repo.CreateTask(ctx, task))
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", Status: entity.ExecutionStatusQueued, IdempotencyKey: "agent-thread-suppressed"}
	require.NoError(t, repo.CreateExecution(ctx, execution))

	require.NoError(t, repo.FinalizeExecution(ctx, execution.ID, ExecutionResult{Status: entity.ExecutionStatusSucceeded, ThreadID: 21, RunID: 22, FinishedAt: 1500}))

	require.Len(t, outbox.events, 1)
	require.Equal(t, domainnotification.EventScheduledExecutionSucceeded, outbox.events[0].EventType)
	require.Equal(t, "scheduled_task_execution", outbox.events[0].AggregateType)
	require.NotEqual(t, domainnotification.EventTaskCompleted, outbox.events[0].EventType)
}

func TestRepositoryMarkExecutionRecoverableRestoresAdvancedTaskForRetry(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	ctx := context.Background()
	task := taskFixture(10, 100, "recover")
	task.Schedule = entity.Schedule{Type: entity.ScheduleTypeOnce, Timezone: "UTC", RunOnceAt: 1000}
	task.MaxExecutions = 1
	task.NextExecutionAt = 1000
	require.NoError(t, repo.CreateTask(ctx, task))
	claimed, err := repo.ClaimDueTasks(ctx, 1000, "worker-a", 2000, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	execution := &entity.Execution{TaskID: task.ID, SpaceID: 10, TriggerType: "schedule", ScheduledAt: 1000, Status: entity.ExecutionStatusQueued, IdempotencyKey: "recover"}
	require.NoError(t, repo.CreateExecution(ctx, execution))
	require.NoError(t, repo.MarkExecutionRunning(ctx, execution.ID, 1200))
	require.NoError(t, repo.AdvanceClaim(ctx, task.ID, "worker-a", 0, true))

	require.NoError(t, repo.MarkExecutionRecoverable(ctx, task.ID, execution.ID, 1000, ExecutionResult{WorkflowExecutionID: 50}))

	gotExecution, err := repo.GetExecution(ctx, execution.ID)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusQueued, gotExecution.Status)
	require.Equal(t, int64(50), gotExecution.WorkflowExecutionID)
	gotTask, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, entity.StatusEnabled, gotTask.Status)
	require.Equal(t, int64(1000), gotTask.NextExecutionAt)
	require.Empty(t, gotTask.LeaseOwner)
}

func TestRepositoryAdvancesClaimOnlyForLeaseOwner(t *testing.T) {
	t.Parallel()
	repo := newTestRepository(t)
	ctx := context.Background()
	task := taskFixture(10, 100, "report")
	task.NextExecutionAt = 1000
	require.NoError(t, repo.CreateTask(ctx, task))
	claimed, err := repo.ClaimDueTasks(ctx, 1000, "worker-a", 2000, 1)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	require.Error(t, repo.AdvanceClaim(ctx, task.ID, "worker-b", 3000, false))
	require.NoError(t, repo.AdvanceClaim(ctx, task.ID, "worker-a", 3000, false))

	got, err := repo.GetTask(ctx, 10, task.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3000), got.NextExecutionAt)
	require.Empty(t, got.LeaseOwner)
}

func newTestRepository(t *testing.T) Repository {
	return newTestRepositoryWithNotification(t, &recordingNotificationOutbox{})
}

func newTestRepositoryWithoutNotification(t *testing.T) Repository {
	return newTestRepositoryWithNotification(t, nil)
}

func newTestRepositoryWithNotification(t *testing.T, appender NotificationOutboxAppender) Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&scheduledTaskPO{}, &scheduledTaskExecutionPO{}))
	return NewMySQLRepository(db, &sequenceIDGen{}, WithNotificationOutboxAppender(appender))
}

func taskFixture(spaceID, creatorID int64, name string) *entity.Task {
	return &entity.Task{
		SpaceID:    spaceID,
		CreatorID:  creatorID,
		Name:       name,
		TargetType: entity.TargetTypeAgent,
		TargetID:   200,
		Schedule: entity.Schedule{
			Type:     entity.ScheduleTypeDaily,
			Timezone: "UTC",
			Hour:     9,
		},
		Payload:         `{"message":"hello"}`,
		Status:          entity.StatusEnabled,
		NextExecutionAt: 1000,
		Version:         1,
	}
}

type sequenceIDGen struct{ next atomic.Int64 }

func (g *sequenceIDGen) GenID(context.Context) (int64, error) {
	return g.next.Add(1), nil
}

func (g *sequenceIDGen) GenMultiIDs(_ context.Context, count int) ([]int64, error) {
	ids := make([]int64, count)
	for i := range ids {
		ids[i] = g.next.Add(1)
	}
	return ids, nil
}

type recordingNotificationOutbox struct {
	events []domainnotification.Event
	err    error
}

func (r *recordingNotificationOutbox) AppendInTransaction(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
	r.events = append(r.events, event)
	return r.err
}

func idString(id int64) string {
	return strconv.FormatInt(id, 10)
}
