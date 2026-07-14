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
				_ = d.finishFailure(ctx, task.ID, execution.ID, "execution_panic")
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
		_ = d.finishFailure(ctx, task.ID, execution.ID, "execution_failed")
		return err
	}
	result, err := executor.Execute(ctx, task, execution)
	if err != nil {
		if finishErr := d.finishFailure(ctx, task.ID, execution.ID, "execution_failed"); finishErr != nil {
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
	if err := d.Repository.FinishExecution(ctx, execution.ID, result); err != nil {
		return err
	}
	if err := d.Repository.RecordTaskExecution(ctx, task.ID, result.FinishedAt); err != nil {
		return err
	}
	return nil
}

func (d *ExecutionDispatcher) finishFailure(ctx context.Context, taskID, executionID int64, errorCode string) error {
	finishedAt := d.now().UnixMilli()
	if err := d.Repository.FinishExecution(ctx, executionID, repository.ExecutionResult{Status: entity.ExecutionStatusFailed, ErrorCode: errorCode, ErrorMessage: "任务执行失败，请稍后重试", FinishedAt: finishedAt}); err != nil {
		return err
	}
	return d.Repository.RecordTaskExecution(ctx, taskID, finishedAt)
}

func (d *ExecutionDispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}
