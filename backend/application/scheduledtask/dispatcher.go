// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type TaskExecutor interface {
	Execute(context.Context, *entity.Task, *entity.Execution) (repository.ExecutionResult, error)
}

type ExecutionDispatcher struct {
	Repository  repository.Repository
	Executors   map[entity.TargetType]TaskExecutor
	RootContext context.Context
	Timeout     time.Duration
	Now         func() time.Time
}

func (d *ExecutionDispatcher) Dispatch(_ context.Context, task *entity.Task, execution *entity.Execution) error {
	if d == nil || d.Repository == nil {
		return fmt.Errorf("scheduled task dispatcher repository is unavailable")
	}
	if task == nil || execution == nil {
		return fmt.Errorf("scheduled task and execution are required")
	}
	root := d.RootContext
	if root == nil {
		root = context.Background()
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = 24 * time.Hour
	}
	go func() {
		ctx, cancel := context.WithTimeout(root, timeout)
		defer cancel()
		defer func() {
			if recovered := recover(); recovered != nil {
				logs.CtxErrorf(ctx, "[scheduled-task] execution panic task_id=%d execution_id=%d panic=%v stack=%s", task.ID, execution.ID, recovered, debug.Stack())
				if err := d.finishFailure(ctx, execution.ID, "execution_panic"); err != nil {
					if recoverErr := d.markExecutionRecoverable(ctx, task, execution, repository.ExecutionResult{}); recoverErr != nil {
						logs.CtxErrorf(ctx, "[scheduled-task] mark execution recoverable failed task_id=%d execution_id=%d err=%v", task.ID, execution.ID, recoverErr)
					}
				}
			}
		}()
		if err := d.RunExecution(ctx, task, execution); err != nil {
			logs.CtxErrorf(ctx, "[scheduled-task] execution failed task_id=%d execution_id=%d err=%v", task.ID, execution.ID, err)
		}
	}()
	return nil
}

func (d *ExecutionDispatcher) RunExecution(ctx context.Context, task *entity.Task, execution *entity.Execution) error {
	startedAt := d.now().UnixMilli()
	if err := d.Repository.MarkExecutionRunning(ctx, execution.ID, startedAt); err != nil {
		return err
	}
	executor := d.Executors[task.TargetType]
	if executor == nil {
		err := fmt.Errorf("executor for target type %q is unavailable", task.TargetType)
		if finishErr := d.finishFailure(ctx, execution.ID, "execution_failed"); finishErr != nil {
			if recoverErr := d.markExecutionRecoverable(ctx, task, execution, repository.ExecutionResult{}); recoverErr != nil {
				logs.CtxErrorf(ctx, "[scheduled-task] mark execution recoverable failed task_id=%d execution_id=%d err=%v", task.ID, execution.ID, recoverErr)
				return fmt.Errorf("%w; finalize: %v; recover: %v", err, finishErr, recoverErr)
			}
			return fmt.Errorf("%w; finalize: %v", err, finishErr)
		}
		return err
	}
	result, err := executor.Execute(ctx, task, execution)
	if err != nil {
		if finishErr := d.finishFailureResult(ctx, execution.ID, result, "execution_failed"); finishErr != nil {
			if recoverErr := d.markExecutionRecoverable(ctx, task, execution, result); recoverErr != nil {
				logs.CtxErrorf(ctx, "[scheduled-task] mark execution recoverable failed task_id=%d execution_id=%d err=%v", task.ID, execution.ID, recoverErr)
				return fmt.Errorf("execute: %w; record failure: %v; recover: %v", err, finishErr, recoverErr)
			}
			return fmt.Errorf("execute: %w; record failure: %v", err, finishErr)
		}
		return err
	}
	if result.Status == "" {
		result.Status = entity.ExecutionStatusSucceeded
	}
	if result.FinishedAt == 0 {
		result.FinishedAt = d.now().UnixMilli()
	}
	if err := d.Repository.FinalizeExecution(ctx, execution.ID, result); err != nil {
		if recoverErr := d.markExecutionRecoverable(ctx, task, execution, result); recoverErr != nil {
			logs.CtxErrorf(ctx, "[scheduled-task] mark execution recoverable failed task_id=%d execution_id=%d err=%v", task.ID, execution.ID, recoverErr)
			return fmt.Errorf("finalize: %w; recover: %v", err, recoverErr)
		}
		return err
	}
	return nil
}

func (d *ExecutionDispatcher) finishFailure(ctx context.Context, executionID int64, errorCode string) error {
	return d.finishFailureResult(ctx, executionID, repository.ExecutionResult{}, errorCode)
}

func (d *ExecutionDispatcher) finishFailureResult(ctx context.Context, executionID int64, result repository.ExecutionResult, errorCode string) error {
	finishedAt := d.now().UnixMilli()
	result.Status = entity.ExecutionStatusFailed
	if result.ErrorCode == "" {
		result.ErrorCode = errorCode
	}
	result.ErrorMessage = "任务执行失败，请稍后重试"
	if result.FinishedAt == 0 {
		result.FinishedAt = finishedAt
	}
	return d.Repository.FinalizeExecution(ctx, executionID, result)
}

func (d *ExecutionDispatcher) markExecutionRecoverable(ctx context.Context, task *entity.Task, execution *entity.Execution, result repository.ExecutionResult) error {
	if task == nil || execution == nil || execution.TriggerType != "schedule" {
		return nil
	}
	if result.WorkflowExecutionID == 0 {
		result.WorkflowExecutionID = execution.WorkflowExecutionID
	}
	return d.Repository.MarkExecutionRecoverable(ctx, task.ID, execution.ID, execution.ScheduledAt, result)
}

func (d *ExecutionDispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}
