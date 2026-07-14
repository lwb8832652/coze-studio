// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"errors"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
)

var (
	ErrExecutionAlreadyExists = errors.New("scheduled task execution already exists")
	ErrStateConflict          = errors.New("scheduled task state conflict")
)

type ListTaskFilter struct {
	SpaceID    int64
	TargetType *entity.TargetType
	Status     *entity.Status
	Keyword    string
	Page       int32
	PageSize   int32
}

type ExecutionResult struct {
	Status              entity.ExecutionStatus
	ThreadID            int64
	RunID               int64
	WorkflowExecutionID int64
	ErrorCode           string
	ErrorMessage        string
	FinishedAt          int64
}

type Repository interface {
	CreateTask(ctx context.Context, task *entity.Task) error
	GetTask(ctx context.Context, spaceID, taskID int64) (*entity.Task, error)
	ListTasks(ctx context.Context, filter ListTaskFilter) ([]*entity.Task, int64, error)
	UpdateTask(ctx context.Context, task *entity.Task, expectedVersion int64) error
	SetTaskStatus(ctx context.Context, spaceID, taskID int64, from, to entity.Status, nextExecutionAt int64) error
	SoftDeleteTask(ctx context.Context, spaceID, taskID int64) error
	CountActiveTasks(ctx context.Context, spaceID, creatorID int64) (int64, error)
	ClaimDueTasks(ctx context.Context, now int64, owner string, leaseUntil int64, limit int32) ([]*entity.Task, error)
	AdvanceClaim(ctx context.Context, taskID int64, owner string, nextExecutionAt int64, completed bool) error
	ReleaseClaim(ctx context.Context, taskID int64, owner string) error
	CreateExecution(ctx context.Context, execution *entity.Execution) error
	GetExecution(ctx context.Context, executionID int64) (*entity.Execution, error)
	MarkExecutionRunning(ctx context.Context, executionID, startedAt int64) error
	FinishExecution(ctx context.Context, executionID int64, result ExecutionResult) error
	ListExecutions(ctx context.Context, spaceID, taskID int64, page, pageSize int32) ([]*entity.Execution, int64, error)
	RecordTaskExecution(ctx context.Context, taskID, latestExecutionAt int64) error
	SetConversationID(ctx context.Context, taskID, conversationID int64) error
}
