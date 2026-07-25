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
	scheduledAt := now - 1
	repo := &recordingWorkerRepository{
		claimed: []*entity.Task{{
			ID: 10, SpaceID: 1, Status: entity.StatusEnabled,
			NextExecutionAt: scheduledAt,
			Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
		}},
		createErr: repository.ErrExecutionAlreadyExists,
		existingExecution: &entity.Execution{ID: 99, TaskID: 10, SpaceID: 1, TriggerType: "schedule", ScheduledAt: scheduledAt, Status: entity.ExecutionStatusRunning, IdempotencyKey: "schedule:1784015999999"},
	}
	dispatcher := &recordingDispatcher{}
	worker := &Worker{Repository: repo, Dispatcher: dispatcher, Owner: "worker-a", Now: fixedNow}

	require.NoError(t, worker.RunOnce(context.Background()))

	require.Nil(t, dispatcher.execution)
	require.Greater(t, repo.nextExecutionAt, now)
}

func TestWorkerRedispatchesDuplicateQueuedExecutionForRecovery(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	scheduledAt := now - 1000
	existing := &entity.Execution{ID: 33, TaskID: 10, SpaceID: 1, TriggerType: "schedule", ScheduledAt: scheduledAt, Status: entity.ExecutionStatusQueued, IdempotencyKey: "schedule:1784015999000"}
	repo := &recordingWorkerRepository{
		claimed: []*entity.Task{{
			ID: 10, SpaceID: 1, Status: entity.StatusEnabled,
			NextExecutionAt: scheduledAt,
			Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
		}},
		createErr: repository.ErrExecutionAlreadyExists,
		existingExecution: existing,
	}
	dispatcher := &recordingDispatcher{}
	worker := &Worker{Repository: repo, Dispatcher: dispatcher, Owner: "worker-a", Now: fixedNow}

	require.NoError(t, worker.RunOnce(context.Background()))

	require.Same(t, existing, dispatcher.execution)
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
		require.Equal(t, int64(20), repo.finalizedExecutionID)
		require.Equal(t, 1, repo.finalizeCount)
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
		require.Equal(t, int64(20), repo.finalizedExecutionID)
		require.Equal(t, 1, repo.finalizeCount)
	})
}

func TestExecutionDispatcherMarksScheduleExecutionRecoverableWhenFinalizeFails(t *testing.T) {
	t.Parallel()
	repo := &recordingExecutionRepository{finalizeErr: errors.New("outbox unavailable")}
	dispatcher := &ExecutionDispatcher{
		Repository: repo,
		Executors: map[entity.TargetType]TaskExecutor{
			entity.TargetTypeAgent: executorFunc(func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error) {
				return repository.ExecutionResult{Status: entity.ExecutionStatusSucceeded}, nil
			}),
		},
		Now: fixedNow,
	}
	task := &entity.Task{ID: 10, TargetType: entity.TargetTypeAgent}
	execution := &entity.Execution{ID: 20, TaskID: 10, TriggerType: "schedule", ScheduledAt: 1234}

	require.Error(t, dispatcher.RunExecution(context.Background(), task, execution))

	require.Equal(t, int64(10), repo.recoverTaskID)
	require.Equal(t, int64(20), repo.recoverExecutionID)
	require.Equal(t, int64(1234), repo.recoverRetryAt)
}

func TestExecutionDispatcherPreservesWorkflowExecutionIDWhenExecutorReturnsError(t *testing.T) {
	t.Parallel()
	repo := &recordingExecutionRepository{}
	dispatcher := &ExecutionDispatcher{
		Repository: repo,
		Executors: map[entity.TargetType]TaskExecutor{
			entity.TargetTypeWorkflow: executorFunc(func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error) {
				return repository.ExecutionResult{WorkflowExecutionID: 50}, errors.New("workflow terminal failed")
			}),
		},
		Now: fixedNow,
	}

	err := dispatcher.RunExecution(context.Background(), &entity.Task{ID: 10, TargetType: entity.TargetTypeWorkflow}, &entity.Execution{ID: 20})

	require.Error(t, err)
	require.Equal(t, entity.ExecutionStatusFailed, repo.result.Status)
	require.Equal(t, int64(50), repo.result.WorkflowExecutionID)
}

func TestExecutionDispatcherDoesNotRestartWorkflowAfterFinalizeFailureReplay(t *testing.T) {
	t.Parallel()
	repo := &recordingExecutionRepository{finalizeErr: errors.New("outbox unavailable")}
	runner := &recordingWorkflowRunner{executionID: 50, status: RunTerminalStatus{Status: entity.ExecutionStatusSucceeded}}
	dispatcher := &ExecutionDispatcher{
		Repository: repo,
		Executors: map[entity.TargetType]TaskExecutor{
			entity.TargetTypeWorkflow: &WorkflowTaskExecutor{Runner: runner, Repository: repo},
		},
		Now: fixedNow,
	}
	task := &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, TargetType: entity.TargetTypeWorkflow, TargetID: 300, Payload: `{"city":"武汉"}`}
	execution := &entity.Execution{ID: 20, TaskID: 10, TriggerType: "schedule", ScheduledAt: 1234}

	require.Error(t, dispatcher.RunExecution(context.Background(), task, execution))
	require.Equal(t, 1, runner.startCount)
	require.Equal(t, int64(50), repo.workflowExecutionID)
	require.Equal(t, int64(50), execution.WorkflowExecutionID)

	repo.finalizeErr = nil
	require.NoError(t, dispatcher.RunExecution(context.Background(), task, execution))

	require.Equal(t, 1, runner.startCount)
	require.Equal(t, int64(50), runner.waitExecutionID)
}

func TestExecutionDispatcherPersistsWorkflowExecutionIDOnRecoverAfterStartPersistFailure(t *testing.T) {
	t.Parallel()
	repo := &recordingExecutionRepository{
		setWorkflowErr: errors.New("workflow execution id storage unavailable"),
		finalizeErr:    errors.New("outbox unavailable"),
	}
	runner := &recordingWorkflowRunner{executionID: 50, status: RunTerminalStatus{Status: entity.ExecutionStatusSucceeded}}
	dispatcher := &ExecutionDispatcher{
		Repository: repo,
		Executors: map[entity.TargetType]TaskExecutor{
			entity.TargetTypeWorkflow: &WorkflowTaskExecutor{Runner: runner, Repository: repo},
		},
		Now: fixedNow,
	}
	task := &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, TargetType: entity.TargetTypeWorkflow, TargetID: 300, Payload: `{"city":"武汉"}`}
	execution := &entity.Execution{ID: 20, TaskID: 10, TriggerType: "schedule", ScheduledAt: 1234}

	require.Error(t, dispatcher.RunExecution(context.Background(), task, execution))
	require.Equal(t, 1, runner.startCount)
	require.Equal(t, int64(50), repo.workflowExecutionID)
	require.Equal(t, int64(50), repo.recoverResult.WorkflowExecutionID)

	repo.setWorkflowErr = nil
	repo.finalizeErr = nil
	reloaded := &entity.Execution{ID: 20, TaskID: 10, TriggerType: "schedule", ScheduledAt: 1234, WorkflowExecutionID: repo.workflowExecutionID}

	require.NoError(t, dispatcher.RunExecution(context.Background(), task, reloaded))

	require.Equal(t, 1, runner.startCount)
	require.Equal(t, int64(50), runner.waitExecutionID)
}

func TestExecutionDispatcherReturnsRecoverErrorWhenMarkRecoverableFails(t *testing.T) {
	t.Parallel()
	repo := &recordingExecutionRepository{finalizeErr: errors.New("outbox unavailable"), recoverErr: errors.New("recover unavailable")}
	dispatcher := &ExecutionDispatcher{
		Repository: repo,
		Executors: map[entity.TargetType]TaskExecutor{
			entity.TargetTypeAgent: executorFunc(func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error) {
				return repository.ExecutionResult{Status: entity.ExecutionStatusSucceeded}, nil
			}),
		},
		Now: fixedNow,
	}

	err := dispatcher.RunExecution(context.Background(), &entity.Task{ID: 10, TargetType: entity.TargetTypeAgent}, &entity.Execution{ID: 20, TaskID: 10, TriggerType: "schedule", ScheduledAt: 1234})

	require.ErrorContains(t, err, "recover unavailable")
}

func TestWorkerFinalizesDispatchFailure(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	repo := &recordingWorkerRepository{claimed: []*entity.Task{{
		ID: 10, SpaceID: 1, CreatorID: 7, Status: entity.StatusEnabled,
		NextExecutionAt: now - 1000,
		Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
	}}}
	worker := &Worker{Repository: repo, Dispatcher: failingDispatcher{}, Owner: "worker-a", Now: fixedNow}

	require.Error(t, worker.RunOnce(context.Background()))

	require.Equal(t, int64(1), repo.finalizedExecutionID)
	require.Equal(t, entity.ExecutionStatusFailed, repo.finalizedResult.Status)
	require.Equal(t, "dispatch_failed", repo.finalizedResult.ErrorCode)
	require.Equal(t, 1, repo.finalizeCount)
}

func TestWorkerMarksExecutionRecoverableWhenDispatchFailureFinalizationFails(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	scheduledAt := now - 1000
	repo := &recordingWorkerRepository{
		claimed: []*entity.Task{{
			ID: 10, SpaceID: 1, CreatorID: 7, Status: entity.StatusEnabled,
			NextExecutionAt: scheduledAt,
			Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
		}},
		finalizeErr: errors.New("outbox unavailable"),
	}
	worker := &Worker{Repository: repo, Dispatcher: failingDispatcher{}, Owner: "worker-a", Now: fixedNow}

	require.Error(t, worker.RunOnce(context.Background()))

	require.Equal(t, int64(10), repo.recoverTaskID)
	require.Equal(t, int64(1), repo.recoverExecutionID)
	require.Equal(t, scheduledAt, repo.recoverRetryAt)
}

func TestWorkerReturnsRecoverErrorWhenMarkRecoverableFails(t *testing.T) {
	t.Parallel()
	now := fixedNow().UnixMilli()
	scheduledAt := now - 1000
	repo := &recordingWorkerRepository{
		claimed: []*entity.Task{{
			ID: 10, SpaceID: 1, CreatorID: 7, Status: entity.StatusEnabled,
			NextExecutionAt: scheduledAt,
			Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
		}},
		finalizeErr: errors.New("outbox unavailable"),
		recoverErr: errors.New("recover unavailable"),
	}
	worker := &Worker{Repository: repo, Dispatcher: failingDispatcher{}, Owner: "worker-a", Now: fixedNow}

	err := worker.RunOnce(context.Background())

	require.ErrorContains(t, err, "recover unavailable")
}

type recordingWorkerRepository struct {
	repository.Repository
	claimed              []*entity.Task
	executions           []*entity.Execution
	createErr            error
	existingExecution    *entity.Execution
	finalizeErr          error
	recoverErr           error
	nextExecutionAt      int64
	completed            bool
	finalizedExecutionID int64
	finalizedResult      repository.ExecutionResult
	finalizeCount        int
	recoverTaskID        int64
	recoverExecutionID   int64
	recoverRetryAt       int64
	recoverResult        repository.ExecutionResult
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

func (r *recordingWorkerRepository) GetExecutionByTrigger(context.Context, int64, string) (*entity.Execution, error) {
	if r.existingExecution == nil {
		return nil, errors.New("execution not found")
	}
	return r.existingExecution, nil
}

func (r *recordingWorkerRepository) AdvanceClaim(_ context.Context, _ int64, _ string, next int64, completed bool) error {
	r.nextExecutionAt = next
	r.completed = completed
	return nil
}

func (r *recordingWorkerRepository) ReleaseClaim(context.Context, int64, string) error { return nil }

func (r *recordingWorkerRepository) FinalizeExecution(_ context.Context, executionID int64, result repository.ExecutionResult) error {
	r.finalizedExecutionID = executionID
	r.finalizedResult = result
	r.finalizeCount++
	return r.finalizeErr
}

func (r *recordingWorkerRepository) MarkExecutionRecoverable(_ context.Context, taskID, executionID int64, retryAt int64, result repository.ExecutionResult) error {
	r.recoverTaskID = taskID
	r.recoverExecutionID = executionID
	r.recoverRetryAt = retryAt
	r.recoverResult = result
	return r.recoverErr
}

type recordingExecutionRepository struct {
	repository.Repository
	runningID            int64
	finalizedExecutionID int64
	result               repository.ExecutionResult
	finalizeCount        int
	finalizeErr          error
	recoverErr           error
	recoverTaskID        int64
	recoverExecutionID   int64
	recoverRetryAt       int64
	recoverResult        repository.ExecutionResult
	workflowExecutionID  int64
	setWorkflowCount     int
	setWorkflowErr       error
}

func (r *recordingExecutionRepository) MarkExecutionRunning(_ context.Context, executionID, _ int64) error {
	r.runningID = executionID
	return nil
}

func (r *recordingExecutionRepository) FinalizeExecution(_ context.Context, executionID int64, result repository.ExecutionResult) error {
	r.finalizedExecutionID = executionID
	r.result = result
	r.finalizeCount++
	return r.finalizeErr
}

func (r *recordingExecutionRepository) MarkExecutionRecoverable(_ context.Context, taskID, executionID int64, retryAt int64, result repository.ExecutionResult) error {
	r.recoverTaskID = taskID
	r.recoverExecutionID = executionID
	r.recoverRetryAt = retryAt
	r.recoverResult = result
	if r.workflowExecutionID == 0 && result.WorkflowExecutionID > 0 {
		r.workflowExecutionID = result.WorkflowExecutionID
	}
	return r.recoverErr
}

func (r *recordingExecutionRepository) SetWorkflowExecutionID(_ context.Context, _ int64, workflowExecutionID int64) error {
	if r.setWorkflowErr != nil {
		r.setWorkflowCount++
		return r.setWorkflowErr
	}
	r.workflowExecutionID = workflowExecutionID
	r.setWorkflowCount++
	return nil
}

type executorFunc func(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error)

func (fn executorFunc) Execute(ctx context.Context, task *entity.Task, execution *entity.Execution) (repository.ExecutionResult, error) {
	return fn(ctx, task, execution)
}

type failingDispatcher struct{}

func (failingDispatcher) Dispatch(context.Context, *entity.Task, *entity.Execution) error {
	return errors.New("dispatch unavailable")
}
