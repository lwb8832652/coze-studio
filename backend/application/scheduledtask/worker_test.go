// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
)

func TestWorkerCreatesExecutionAdvancesAndDispatches(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	repo := &recordingWorkerRepository{claimed: []*entity.Task{{
		ID: 10, SpaceID: 1, CreatorID: 7, Status: entity.StatusEnabled,
		NextExecutionAt: now - 1000,
		Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
	}}}
	dispatcher := &recordingDispatcher{}
	worker := &Worker{Repository: repo, Dispatcher: dispatcher, Owner: "worker-a", Now: fixedNow}

	require.NoError(t, worker.RunOnce(context.Background()))

	require.Len(t, repo.executions, 1)
	require.Equal(t, "schedule", repo.executions[0].TriggerType)
	require.Equal(t, "schedule:1784015999000", repo.executions[0].IdempotencyKey)
	require.Greater(t, repo.nextExecutionAt, now)
	require.False(t, repo.completed)
	require.Same(t, repo.executions[0], dispatcher.execution)
}

func TestWorkerCompletesOneTimeTask(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	repo := &recordingWorkerRepository{claimed: []*entity.Task{{
		ID: 10, SpaceID: 1, Status: entity.StatusEnabled,
		NextExecutionAt: now - 1,
		Schedule: entity.Schedule{Type: entity.ScheduleTypeOnce, Timezone: "UTC", RunOnceAt: now - 1},
		MaxExecutions: 1,
	}}}
	worker := &Worker{Repository: repo, Dispatcher: &recordingDispatcher{}, Owner: "worker-a", Now: fixedNow}

	require.NoError(t, worker.RunOnce(context.Background()))

	require.True(t, repo.completed)
	require.Zero(t, repo.nextExecutionAt)
}

func TestWorkerTreatsDuplicateTriggerAsAlreadyScheduled(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	repo := &recordingWorkerRepository{
		claimed: []*entity.Task{{
			ID: 10, SpaceID: 1, Status: entity.StatusEnabled,
			NextExecutionAt: now - 1,
			Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
		}},
		createErr: repository.ErrExecutionAlreadyExists,
	}
	dispatcher := &recordingDispatcher{}
	worker := &Worker{Repository: repo, Dispatcher: dispatcher, Owner: "worker-a", Now: fixedNow}

	require.NoError(t, worker.RunOnce(context.Background()))

	require.Nil(t, dispatcher.execution)
	require.Greater(t, repo.nextExecutionAt, now)
}

func TestExecutionDispatcherRecordsSuccessAndFailure(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		repo := &recordingExecutionRepository{}
		dispatcher := &ExecutionDispatcher{
			Repository: repo,
			Executors: map[entity.TargetType]TaskExecutor{
				entity.TargetTypeAgent: executorFunc(func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error) {
					return repository.ExecutionResult{Status: entity.ExecutionStatusSucceeded, ThreadID: 30, RunID: 40}, nil
				}),
			},
			Now: fixedNow,
		}
		task := &entity.Task{ID: 10, TargetType: entity.TargetTypeAgent}
		execution := &entity.Execution{ID: 20}

		require.NoError(t, dispatcher.RunExecution(context.Background(), task, execution))
		require.Equal(t, int64(20), repo.runningID)
		require.Equal(t, entity.ExecutionStatusSucceeded, repo.result.Status)
		require.Equal(t, int64(30), repo.result.ThreadID)
		require.Equal(t, int64(40), repo.result.RunID)
		require.Equal(t, int64(10), repo.recordedTaskID)
	})

	t.Run("failure", func(t *testing.T) {
		repo := &recordingExecutionRepository{}
		dispatcher := &ExecutionDispatcher{
			Repository: repo,
			Executors: map[entity.TargetType]TaskExecutor{
				entity.TargetTypeAgent: executorFunc(func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error) {
					return repository.ExecutionResult{}, errors.New("provider secret leaked")
				}),
			},
			Now: fixedNow,
		}

		err := dispatcher.RunExecution(context.Background(), &entity.Task{ID: 10, TargetType: entity.TargetTypeAgent}, &entity.Execution{ID: 20})

		require.Error(t, err)
		require.Equal(t, entity.ExecutionStatusFailed, repo.result.Status)
		require.Equal(t, "execution_failed", repo.result.ErrorCode)
		require.Equal(t, "任务执行失败，请稍后重试", repo.result.ErrorMessage)
		require.Equal(t, int64(10), repo.recordedTaskID)
	})
}

type recordingWorkerRepository struct {
	repository.Repository
	claimed         []*entity.Task
	executions      []*entity.Execution
	createErr       error
	nextExecutionAt int64
	completed       bool
}

func (r *recordingWorkerRepository) ClaimDueTasks(context.Context, int64, string, int64, int32) ([]*entity.Task, error) {
	return r.claimed, nil
}

func (r *recordingWorkerRepository) CreateExecution(_ context.Context, execution *entity.Execution) error {
	if r.createErr != nil { return r.createErr }
	execution.ID = int64(len(r.executions) + 1)
	r.executions = append(r.executions, execution)
	return nil
}

func (r *recordingWorkerRepository) AdvanceClaim(_ context.Context, _ int64, _ string, next int64, completed bool) error {
	r.nextExecutionAt = next
	r.completed = completed
	return nil
}

func (r *recordingWorkerRepository) ReleaseClaim(context.Context, int64, string) error { return nil }

type recordingExecutionRepository struct {
	repository.Repository
	runningID      int64
	result         repository.ExecutionResult
	recordedTaskID int64
}

func (r *recordingExecutionRepository) MarkExecutionRunning(_ context.Context, executionID, _ int64) error {
	r.runningID = executionID
	return nil
}

func (r *recordingExecutionRepository) FinishExecution(_ context.Context, _ int64, result repository.ExecutionResult) error {
	r.result = result
	return nil
}

func (r *recordingExecutionRepository) RecordTaskExecution(_ context.Context, taskID, _ int64) error {
	r.recordedTaskID = taskID
	return nil
}

type executorFunc func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error)

func (fn executorFunc) Execute(ctx context.Context, task *entity.Task, execution *entity.Execution) (repository.ExecutionResult, error) {
	return fn(ctx, task, execution)
}
