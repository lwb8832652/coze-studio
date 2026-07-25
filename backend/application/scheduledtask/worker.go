// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/scheduledtask/service"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type Worker struct {
	Repository    repository.Repository
	Dispatcher    Dispatcher
	Owner         string
	Now           func() time.Time
	Interval      time.Duration
	LeaseDuration time.Duration
	BatchSize     int32
}

func (w *Worker) Start(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.RunOnce(ctx); err != nil {
					logs.CtxErrorf(ctx, "[scheduled-task] worker tick failed: %v", err)
				}
			}
		}
	}()
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if w == nil || w.Repository == nil || w.Dispatcher == nil {
		return fmt.Errorf("scheduled task worker is not initialized")
	}
	now := w.now()
	leaseDuration := w.LeaseDuration
	if leaseDuration <= 0 {
		leaseDuration = 30 * time.Second
	}
	batchSize := w.BatchSize
	if batchSize <= 0 {
		batchSize = 20
	}
	owner := w.Owner
	if owner == "" {
		owner = defaultWorkerOwner()
	}
	claimed, err := w.Repository.ClaimDueTasks(ctx, now.UnixMilli(), owner, now.Add(leaseDuration).UnixMilli(), batchSize)
	if err != nil {
		return err
	}

	var runErrors []error
	for _, task := range claimed {
		if task == nil {
			continue
		}
		if err := w.processClaim(ctx, task, owner, now); err != nil {
			runErrors = append(runErrors, fmt.Errorf("task %d: %w", task.ID, err))
		}
	}
	return errors.Join(runErrors...)
}

func (w *Worker) processClaim(ctx context.Context, task *entity.Task, owner string, now time.Time) error {
	scheduledAt := task.NextExecutionAt
	idempotencyKey := "schedule:" + strconv.FormatInt(scheduledAt, 10)
	execution := &entity.Execution{TaskID: task.ID, SpaceID: task.SpaceID, TriggerType: "schedule", ScheduledAt: scheduledAt, Status: entity.ExecutionStatusQueued, IdempotencyKey: idempotencyKey, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
	err := w.Repository.CreateExecution(ctx, execution)
	if err != nil && !errors.Is(err, repository.ErrExecutionAlreadyExists) {
		_ = w.Repository.ReleaseClaim(ctx, task.ID, owner)
		return err
	}
	duplicateExecution := errors.Is(err, repository.ErrExecutionAlreadyExists)
	if duplicateExecution {
		execution, err = w.Repository.GetExecutionByTrigger(ctx, task.ID, idempotencyKey)
		if err != nil {
			_ = w.Repository.ReleaseClaim(ctx, task.ID, owner)
			return err
		}
	}

	completed := task.Schedule.Type == entity.ScheduleTypeOnce || (task.MaxExecutions > 0 && task.ExecutionCount+1 >= task.MaxExecutions)
	nextExecutionAt := int64(0)
	if !completed {
		nextExecutionAt, err = domainservice.NextExecutionAt(task.Schedule, now)
		if err != nil {
			_ = w.Repository.ReleaseClaim(ctx, task.ID, owner)
			return err
		}
	}
	if err := w.Repository.AdvanceClaim(ctx, task.ID, owner, nextExecutionAt, completed); err != nil {
		return err
	}
	if duplicateExecution && execution.Status != entity.ExecutionStatusQueued {
		return nil
	}
	if execution.ID == 0 {
		return nil
	}
	if err := w.Dispatcher.Dispatch(ctx, task, execution); err != nil {
		finishedAt := w.now().UnixMilli()
		if finalizeErr := w.Repository.FinalizeExecution(ctx, execution.ID, repository.ExecutionResult{Status: entity.ExecutionStatusFailed, ErrorCode: "dispatch_failed", ErrorMessage: "任务执行失败，请稍后重试", FinishedAt: finishedAt}); finalizeErr != nil {
			if recoverErr := w.Repository.MarkExecutionRecoverable(ctx, task.ID, execution.ID, scheduledAt, repository.ExecutionResult{}); recoverErr != nil {
				logs.CtxErrorf(ctx, "[scheduled-task] mark execution recoverable failed task_id=%d execution_id=%d err=%v", task.ID, execution.ID, recoverErr)
				return fmt.Errorf("dispatch: %w; finalize: %v; recover: %v", err, finalizeErr, recoverErr)
			}
			return fmt.Errorf("dispatch: %w; finalize: %v", err, finalizeErr)
		}
		return err
	}
	return nil
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func defaultWorkerOwner() string {
	hostname, _ := os.Hostname()
	return hostname + ":" + strconv.Itoa(os.Getpid())
}
