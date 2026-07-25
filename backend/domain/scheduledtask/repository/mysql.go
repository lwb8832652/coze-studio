// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type mysqlRepository struct {
	db                 *gorm.DB
	idGen              idgen.IDGenerator
	notificationOutbox NotificationOutboxAppender
}

type NotificationOutboxAppender interface {
	AppendInTransaction(context.Context, *gorm.DB, domainnotification.Event) error
}

type MySQLOption func(*mysqlRepository)

func WithNotificationOutboxAppender(appender NotificationOutboxAppender) MySQLOption {
	return func(r *mysqlRepository) {
		r.notificationOutbox = appender
	}
}

func NewMySQLRepository(db *gorm.DB, idGen idgen.IDGenerator, options ...MySQLOption) Repository {
	repo := &mysqlRepository{db: db, idGen: idGen}
	for _, option := range options {
		if option != nil {
			option(repo)
		}
	}
	return repo
}

type scheduledTaskPO struct {
	ID                    int64          `gorm:"column:id;primaryKey"`
	SpaceID               int64          `gorm:"column:space_id;index:idx_scheduled_tasks_space_status_next"`
	CreatorID             int64          `gorm:"column:creator_id;index:idx_scheduled_tasks_creator"`
	Name                  string         `gorm:"column:name"`
	TargetType            string         `gorm:"column:target_type"`
	TargetID              int64          `gorm:"column:target_id"`
	TargetName            string         `gorm:"column:target_name"`
	TargetIconURI         string         `gorm:"column:target_icon_uri"`
	ScheduleType          string         `gorm:"column:schedule_type"`
	CronExpr              string         `gorm:"column:cron_expr"`
	Timezone              string         `gorm:"column:timezone"`
	RunOnceAt             int64          `gorm:"column:run_once_at"`
	Minute                int            `gorm:"column:minute"`
	Hour                  int            `gorm:"column:hour"`
	Weekday               int            `gorm:"column:weekday"`
	Payload               datatypes.JSON `gorm:"column:payload;type:json"`
	KeepConversation      bool           `gorm:"column:keep_conversation"`
	ConversationID        int64          `gorm:"column:conversation_id"`
	Status                string         `gorm:"column:status;index:idx_scheduled_tasks_space_status_next"`
	ExecutionCount        int64          `gorm:"column:execution_count"`
	MaxExecutions         int64          `gorm:"column:max_executions"`
	LatestExecutionAt     int64          `gorm:"column:latest_execution_at"`
	LatestExecutionStatus string         `gorm:"column:latest_execution_status"`
	NextExecutionAt       int64          `gorm:"column:next_execution_at;index:idx_scheduled_tasks_space_status_next"`
	LeaseOwner            string         `gorm:"column:lease_owner"`
	LeaseExpiresAt        int64          `gorm:"column:lease_expires_at;index:idx_scheduled_tasks_lease"`
	Version               int64          `gorm:"column:version"`
	CreatedAt             int64          `gorm:"column:created_at"`
	UpdatedAt             int64          `gorm:"column:updated_at"`
	DeletedAt             int64          `gorm:"column:deleted_at;index:idx_scheduled_tasks_deleted"`
}

func (scheduledTaskPO) TableName() string { return "scheduled_tasks" }

type scheduledTaskExecutionPO struct {
	ID                  int64  `gorm:"column:id;primaryKey"`
	TaskID              int64  `gorm:"column:task_id;uniqueIndex:uk_scheduled_task_executions_trigger;index:idx_scheduled_task_executions_task_created"`
	SpaceID             int64  `gorm:"column:space_id;index:idx_scheduled_task_executions_space_status"`
	TriggerType         string `gorm:"column:trigger_type"`
	ScheduledAt         int64  `gorm:"column:scheduled_at"`
	Status              string `gorm:"column:status;index:idx_scheduled_task_executions_space_status"`
	Attempt             int32  `gorm:"column:attempt"`
	IdempotencyKey      string `gorm:"column:idempotency_key;uniqueIndex:uk_scheduled_task_executions_trigger"`
	ThreadID            int64  `gorm:"column:thread_id"`
	RunID               int64  `gorm:"column:run_id"`
	WorkflowExecutionID int64  `gorm:"column:workflow_execution_id"`
	ErrorCode           string `gorm:"column:error_code"`
	ErrorMessage        string `gorm:"column:error_message"`
	StartedAt           int64  `gorm:"column:started_at"`
	FinishedAt          int64  `gorm:"column:finished_at"`
	CreatedAt           int64  `gorm:"column:created_at;index:idx_scheduled_task_executions_task_created"`
	UpdatedAt           int64  `gorm:"column:updated_at"`
}

func (scheduledTaskExecutionPO) TableName() string { return "scheduled_task_executions" }

func (r *mysqlRepository) CreateTask(ctx context.Context, task *entity.Task) error {
	if !json.Valid([]byte(task.Payload)) {
		return fmt.Errorf("payload must be valid JSON")
	}
	if task.ID == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		task.ID = id
	}
	now := time.Now().UnixMilli()
	if task.CreatedAt == 0 {
		task.CreatedAt = now
	}
	if task.UpdatedAt == 0 {
		task.UpdatedAt = task.CreatedAt
	}
	if task.Version == 0 {
		task.Version = 1
	}
	return r.db.WithContext(ctx).Create(taskToPO(task)).Error
}

func (r *mysqlRepository) GetTask(ctx context.Context, spaceID, taskID int64) (*entity.Task, error) {
	var po scheduledTaskPO
	err := r.db.WithContext(ctx).Where("id = ? AND space_id = ? AND deleted_at = 0", taskID, spaceID).First(&po).Error
	if err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *mysqlRepository) ListTasks(ctx context.Context, filter ListTaskFilter) ([]*entity.Task, int64, error) {
	page, pageSize := normalizePage(filter.Page, filter.PageSize)
	query := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("space_id = ? AND deleted_at = 0", filter.SpaceID)
	if filter.TargetType != nil {
		query = query.Where("target_type = ?", string(*filter.TargetType))
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR target_name LIKE ?", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var pos []*scheduledTaskPO
	if err := query.Order("created_at DESC, id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&pos).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*entity.Task, 0, len(pos))
	for _, po := range pos {
		items = append(items, po.toEntity())
	}
	return items, total, nil
}

func (r *mysqlRepository) UpdateTask(ctx context.Context, task *entity.Task, expectedVersion int64) error {
	if !json.Valid([]byte(task.Payload)) {
		return fmt.Errorf("payload must be valid JSON")
	}
	updates := taskMutableUpdates(task)
	updates["updated_at"] = time.Now().UnixMilli()
	updates["version"] = gorm.Expr("version + 1")
	result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("id = ? AND space_id = ? AND version = ? AND deleted_at = 0", task.ID, task.SpaceID, expectedVersion).
		Updates(updates)
	return requireUpdated(result, "scheduled task was modified concurrently")
}

func (r *mysqlRepository) SetTaskStatus(ctx context.Context, spaceID, taskID int64, from, to entity.Status, nextExecutionAt int64) error {
	result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("id = ? AND space_id = ? AND status = ? AND deleted_at = 0", taskID, spaceID, string(from)).
		Updates(map[string]any{"status": string(to), "next_execution_at": nextExecutionAt, "updated_at": time.Now().UnixMilli(), "version": gorm.Expr("version + 1")})
	return requireUpdated(result, "scheduled task status changed")
}

func (r *mysqlRepository) SoftDeleteTask(ctx context.Context, spaceID, taskID int64) error {
	now := time.Now().UnixMilli()
	result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("id = ? AND space_id = ? AND deleted_at = 0", taskID, spaceID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now, "lease_owner": "", "lease_expires_at": 0})
	return requireUpdated(result, "scheduled task not found")
}

func (r *mysqlRepository) CountActiveTasks(ctx context.Context, spaceID, creatorID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("space_id = ? AND creator_id = ? AND deleted_at = 0 AND status <> ?", spaceID, creatorID, string(entity.StatusCompleted)).
		Count(&count).Error
	return count, err
}

func (r *mysqlRepository) ClaimDueTasks(ctx context.Context, now int64, owner string, leaseUntil int64, limit int32) ([]*entity.Task, error) {
	if limit <= 0 {
		limit = 20
	}
	var candidates []*scheduledTaskPO
	err := r.db.WithContext(ctx).
		Where("status = ? AND deleted_at = 0 AND next_execution_at > 0 AND next_execution_at <= ? AND (lease_expires_at = 0 OR lease_expires_at < ?)", string(entity.StatusEnabled), now, now).
		Order("next_execution_at ASC, id ASC").Limit(int(limit)).Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	claimed := make([]*entity.Task, 0, len(candidates))
	for _, candidate := range candidates {
		result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
			Where("id = ? AND status = ? AND deleted_at = 0 AND next_execution_at > 0 AND next_execution_at <= ? AND (lease_expires_at = 0 OR lease_expires_at < ?)", candidate.ID, string(entity.StatusEnabled), now, now).
			Updates(map[string]any{"lease_owner": owner, "lease_expires_at": leaseUntil})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			candidate.LeaseOwner = owner
			candidate.LeaseExpiresAt = leaseUntil
			claimed = append(claimed, candidate.toEntity())
		}
	}
	return claimed, nil
}

func (r *mysqlRepository) AdvanceClaim(ctx context.Context, taskID int64, owner string, nextExecutionAt int64, completed bool) error {
	status := entity.StatusEnabled
	if completed {
		status = entity.StatusCompleted
	}
	result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("id = ? AND lease_owner = ? AND deleted_at = 0", taskID, owner).
		Updates(map[string]any{"status": string(status), "next_execution_at": nextExecutionAt, "lease_owner": "", "lease_expires_at": 0})
	return requireUpdated(result, "scheduled task lease is not owned by worker")
}

func (r *mysqlRepository) ReleaseClaim(ctx context.Context, taskID int64, owner string) error {
	result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).Where("id = ? AND lease_owner = ?", taskID, owner).
		Updates(map[string]any{"lease_owner": "", "lease_expires_at": 0})
	return requireUpdated(result, "scheduled task lease is not owned by worker")
}

func (r *mysqlRepository) CreateExecution(ctx context.Context, execution *entity.Execution) error {
	if execution.ID == 0 {
		id, err := r.idGen.GenID(ctx)
		if err != nil {
			return err
		}
		execution.ID = id
	}
	now := time.Now().UnixMilli()
	if execution.CreatedAt == 0 {
		execution.CreatedAt = now
	}
	if execution.UpdatedAt == 0 {
		execution.UpdatedAt = execution.CreatedAt
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(executionToPO(execution)).Error; err != nil {
			if isDuplicateError(err) {
				return ErrExecutionAlreadyExists
			}
			return err
		}
		return updateTaskLatestExecutionStatus(tx, execution.TaskID, execution.ID, execution.Status)
	})
}

func (r *mysqlRepository) GetExecution(ctx context.Context, executionID int64) (*entity.Execution, error) {
	var po scheduledTaskExecutionPO
	if err := r.db.WithContext(ctx).Where("id = ?", executionID).First(&po).Error; err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *mysqlRepository) GetExecutionByTrigger(ctx context.Context, taskID int64, idempotencyKey string) (*entity.Execution, error) {
	var po scheduledTaskExecutionPO
	if err := r.db.WithContext(ctx).Where("task_id = ? AND idempotency_key = ?", taskID, strings.TrimSpace(idempotencyKey)).First(&po).Error; err != nil {
		return nil, err
	}
	return po.toEntity(), nil
}

func (r *mysqlRepository) MarkExecutionRunning(ctx context.Context, executionID, startedAt int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var execution scheduledTaskExecutionPO
		if err := tx.Select("task_id").Where("id = ?", executionID).First(&execution).Error; err != nil {
			return err
		}
		result := tx.Model(&scheduledTaskExecutionPO{}).
			Where("id = ? AND status = ?", executionID, string(entity.ExecutionStatusQueued)).
			Updates(map[string]any{"status": string(entity.ExecutionStatusRunning), "started_at": startedAt, "attempt": gorm.Expr("attempt + 1"), "updated_at": startedAt})
		if err := requireUpdated(result, "scheduled task execution is not queued"); err != nil {
			return err
		}
		return updateTaskLatestExecutionStatus(tx, execution.TaskID, executionID, entity.ExecutionStatusRunning)
	})
}

func (r *mysqlRepository) SetWorkflowExecutionID(ctx context.Context, executionID, workflowExecutionID int64) error {
	if workflowExecutionID <= 0 {
		return fmt.Errorf("%w: workflow execution id is required", ErrStateConflict)
	}
	now := time.Now().UnixMilli()
	result := r.db.WithContext(ctx).Model(&scheduledTaskExecutionPO{}).
		Where("id = ? AND workflow_execution_id = 0 AND status IN ?", executionID, []string{string(entity.ExecutionStatusQueued), string(entity.ExecutionStatusRunning)}).
		Updates(map[string]any{"workflow_execution_id": workflowExecutionID, "updated_at": now})
	if result.Error != nil || result.RowsAffected > 0 {
		return result.Error
	}
	var execution scheduledTaskExecutionPO
	if err := r.db.WithContext(ctx).Select("workflow_execution_id").Where("id = ?", executionID).First(&execution).Error; err != nil {
		return err
	}
	if execution.WorkflowExecutionID == workflowExecutionID {
		return nil
	}
	return fmt.Errorf("%w: workflow execution id already differs", ErrStateConflict)
}

func (r *mysqlRepository) FinalizeExecution(ctx context.Context, executionID int64, result ExecutionResult) error {
	if !isTerminalExecutionStatus(result.Status) {
		return fmt.Errorf("%w: scheduled task execution status is not terminal", ErrStateConflict)
	}
	if result.FinishedAt == 0 {
		result.FinishedAt = time.Now().UnixMilli()
	}
	updates := map[string]any{"status": string(result.Status), "thread_id": result.ThreadID, "run_id": result.RunID, "workflow_execution_id": result.WorkflowExecutionID, "error_code": result.ErrorCode, "error_message": result.ErrorMessage, "finished_at": result.FinishedAt, "updated_at": result.FinishedAt}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var execution scheduledTaskExecutionPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("task_id", "status").Where("id = ?", executionID).First(&execution).Error; err != nil {
			return err
		}
		if isTerminalExecutionStatus(entity.ExecutionStatus(execution.Status)) {
			if execution.Status == string(result.Status) {
				return nil
			}
			return fmt.Errorf("%w: scheduled task execution has different terminal status", ErrStateConflict)
		}
		if execution.Status != string(entity.ExecutionStatusQueued) && execution.Status != string(entity.ExecutionStatusRunning) {
			return fmt.Errorf("%w: scheduled task execution cannot be finalized from current status", ErrStateConflict)
		}
		var task scheduledTaskPO
		if err := tx.Select("id", "space_id", "creator_id").Where("id = ? AND deleted_at = 0", execution.TaskID).First(&task).Error; err != nil {
			return err
		}
		db := tx.Model(&scheduledTaskExecutionPO{}).
			Where("id = ? AND status IN ?", executionID, []string{string(entity.ExecutionStatusQueued), string(entity.ExecutionStatusRunning)}).
			Updates(updates)
		if db.Error != nil {
			return db.Error
		}
		if db.RowsAffected == 0 {
			return finalizeExecutionCASMiss(tx, executionID, result.Status)
		}
		if err := updateTaskTerminalExecution(tx, execution.TaskID, executionID, result.Status, result.FinishedAt); err != nil {
			return err
		}
		event, err := r.notificationEventForTerminalExecution(task, executionID, result)
		if err != nil {
			return err
		}
		if event == nil {
			return nil
		}
		return r.notificationOutbox.AppendInTransaction(ctx, tx, *event)
	})
}

func (r *mysqlRepository) MarkExecutionRecoverable(ctx context.Context, taskID, executionID int64, retryAt int64, result ExecutionResult) error {
	now := time.Now().UnixMilli()
	if retryAt <= 0 {
		retryAt = now
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var execution scheduledTaskExecutionPO
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("status").Where("id = ? AND task_id = ?", executionID, taskID).First(&execution).Error; err != nil {
			return err
		}
		if isTerminalExecutionStatus(entity.ExecutionStatus(execution.Status)) {
			return nil
		}
		executionUpdates := map[string]any{"status": string(entity.ExecutionStatusQueued), "updated_at": now}
		if result.WorkflowExecutionID > 0 {
			executionUpdates["workflow_execution_id"] = gorm.Expr("COALESCE(NULLIF(workflow_execution_id, 0), ?)", result.WorkflowExecutionID)
		}
		executionResult := tx.Model(&scheduledTaskExecutionPO{}).
			Where("id = ? AND task_id = ? AND status IN ?", executionID, taskID, []string{string(entity.ExecutionStatusQueued), string(entity.ExecutionStatusRunning)}).
			Updates(executionUpdates)
		if err := requireUpdated(executionResult, "scheduled task execution cannot be marked recoverable"); err != nil {
			return err
		}
		taskResult := tx.Model(&scheduledTaskPO{}).
			Where("id = ? AND deleted_at = 0 AND status IN ?", taskID, []string{string(entity.StatusEnabled), string(entity.StatusCompleted)}).
			Updates(map[string]any{
				"status":           string(entity.StatusEnabled),
				"next_execution_at": retryAt,
				"lease_owner":      "",
				"lease_expires_at": 0,
				"updated_at":       gorm.Expr("CASE WHEN updated_at > ? THEN updated_at ELSE ? END", now, now),
			})
		return requireUpdated(taskResult, "scheduled task cannot be restored for execution retry")
	})
}

func (r *mysqlRepository) ListExecutions(ctx context.Context, spaceID, taskID int64, page, pageSize int32) ([]*entity.Execution, int64, error) {
	page, pageSize = normalizePage(page, pageSize)
	query := r.db.WithContext(ctx).Model(&scheduledTaskExecutionPO{}).Where("space_id = ? AND task_id = ?", spaceID, taskID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var pos []*scheduledTaskExecutionPO
	if err := query.Order("created_at DESC, id DESC").Limit(int(pageSize)).Offset(int((page - 1) * pageSize)).Find(&pos).Error; err != nil {
		return nil, 0, err
	}
	items := make([]*entity.Execution, 0, len(pos))
	for _, po := range pos {
		items = append(items, po.toEntity())
	}
	return items, total, nil
}

func (r *mysqlRepository) SetConversationID(ctx context.Context, taskID, conversationID int64) error {
	result := r.db.WithContext(ctx).Model(&scheduledTaskPO{}).
		Where("id = ? AND deleted_at = 0 AND conversation_id = 0", taskID).
		Updates(map[string]any{"conversation_id": conversationID})
	return requireUpdated(result, "scheduled task conversation was already assigned")
}

func taskToPO(task *entity.Task) *scheduledTaskPO {
	return &scheduledTaskPO{ID: task.ID, SpaceID: task.SpaceID, CreatorID: task.CreatorID, Name: task.Name, TargetType: string(task.TargetType), TargetID: task.TargetID, TargetName: task.TargetName, TargetIconURI: task.TargetIconURI, ScheduleType: string(task.Schedule.Type), CronExpr: task.Schedule.CronExpr, Timezone: task.Schedule.Timezone, RunOnceAt: task.Schedule.RunOnceAt, Minute: task.Schedule.Minute, Hour: task.Schedule.Hour, Weekday: task.Schedule.Weekday, Payload: datatypes.JSON(task.Payload), KeepConversation: task.KeepConversation, ConversationID: task.ConversationID, Status: string(task.Status), ExecutionCount: task.ExecutionCount, MaxExecutions: task.MaxExecutions, LatestExecutionAt: task.LatestExecutionAt, LatestExecutionStatus: string(task.LatestExecutionStatus), NextExecutionAt: task.NextExecutionAt, LeaseOwner: task.LeaseOwner, LeaseExpiresAt: task.LeaseExpiresAt, Version: task.Version, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt, DeletedAt: task.DeletedAt}
}

func (po *scheduledTaskPO) toEntity() *entity.Task {
	return &entity.Task{ID: po.ID, SpaceID: po.SpaceID, CreatorID: po.CreatorID, Name: po.Name, TargetType: entity.TargetType(po.TargetType), TargetID: po.TargetID, TargetName: po.TargetName, TargetIconURI: po.TargetIconURI, Schedule: entity.Schedule{Type: entity.ScheduleType(po.ScheduleType), CronExpr: po.CronExpr, Timezone: po.Timezone, RunOnceAt: po.RunOnceAt, Minute: po.Minute, Hour: po.Hour, Weekday: po.Weekday}, Payload: string(po.Payload), KeepConversation: po.KeepConversation, ConversationID: po.ConversationID, Status: entity.Status(po.Status), ExecutionCount: po.ExecutionCount, MaxExecutions: po.MaxExecutions, LatestExecutionAt: po.LatestExecutionAt, LatestExecutionStatus: entity.ExecutionStatus(po.LatestExecutionStatus), NextExecutionAt: po.NextExecutionAt, LeaseOwner: po.LeaseOwner, LeaseExpiresAt: po.LeaseExpiresAt, Version: po.Version, CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt, DeletedAt: po.DeletedAt}
}

func updateTaskLatestExecutionStatus(db *gorm.DB, taskID, executionID int64, status entity.ExecutionStatus) error {
	result := db.Model(&scheduledTaskPO{}).
		Where("id = ? AND deleted_at = 0 AND NOT EXISTS (SELECT 1 FROM scheduled_task_executions AS newer WHERE newer.task_id = scheduled_tasks.id AND newer.id > ?)", taskID, executionID).
		Update("latest_execution_status", string(status))
	if result.Error != nil || result.RowsAffected > 0 {
		return result.Error
	}
	var count int64
	if err := db.Model(&scheduledTaskPO{}).Where("id = ? AND deleted_at = 0", taskID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("scheduled task not found")
	}
	return nil
}

func updateTaskTerminalExecution(db *gorm.DB, taskID, executionID int64, status entity.ExecutionStatus, finishedAt int64) error {
	newerExecutionMissing := "NOT EXISTS (SELECT 1 FROM scheduled_task_executions AS newer WHERE newer.task_id = scheduled_tasks.id AND newer.id > ?)"
	result := db.Model(&scheduledTaskPO{}).
		Where("id = ? AND deleted_at = 0", taskID).
		Updates(map[string]any{
			"execution_count":         gorm.Expr("execution_count + 1"),
			"latest_execution_at":     gorm.Expr("CASE WHEN "+newerExecutionMissing+" THEN ? ELSE latest_execution_at END", executionID, finishedAt),
			"latest_execution_status": gorm.Expr("CASE WHEN "+newerExecutionMissing+" THEN ? ELSE latest_execution_status END", executionID, string(status)),
			"updated_at":              gorm.Expr("CASE WHEN "+newerExecutionMissing+" THEN CASE WHEN updated_at > ? THEN updated_at ELSE ? END ELSE updated_at END", executionID, finishedAt, finishedAt),
		})
	return requireUpdated(result, "scheduled task not found")
}

func finalizeExecutionCASMiss(db *gorm.DB, executionID int64, status entity.ExecutionStatus) error {
	var execution scheduledTaskExecutionPO
	if err := db.Select("status").Where("id = ?", executionID).First(&execution).Error; err != nil {
		return err
	}
	if execution.Status == string(status) && isTerminalExecutionStatus(entity.ExecutionStatus(execution.Status)) {
		return nil
	}
	return fmt.Errorf("%w: scheduled task execution is terminal", ErrStateConflict)
}

func (r *mysqlRepository) notificationEventForTerminalExecution(task scheduledTaskPO, executionID int64, result ExecutionResult) (*domainnotification.Event, error) {
	eventType := domainnotification.EventType("")
	switch result.Status {
	case entity.ExecutionStatusSucceeded:
		eventType = domainnotification.EventScheduledExecutionSucceeded
	case entity.ExecutionStatusFailed:
		eventType = domainnotification.EventScheduledExecutionFailed
	default:
		return nil, nil
	}
	if r == nil || r.notificationOutbox == nil {
		return nil, domainnotification.ErrStorage
	}
	return &domainnotification.Event{
		EventID:          fmt.Sprintf("scheduled_execution:%d:%s", executionID, result.Status),
		EventType:        eventType,
		AggregateType:    "scheduled_task_execution",
		AggregateID:      strconv.FormatInt(executionID, 10),
		AggregateVersion: 1,
		OccurredAt:       time.UnixMilli(result.FinishedAt),
		ActorID:          task.CreatorID,
		SpaceID:          task.SpaceID,
		RecipientPolicy:  domainnotification.RecipientActor,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			TargetID: strconv.FormatInt(task.ID, 10),
		},
	}, nil
}

func isTerminalExecutionStatus(status entity.ExecutionStatus) bool {
	switch status {
	case entity.ExecutionStatusSucceeded, entity.ExecutionStatusFailed, entity.ExecutionStatusCanceled:
		return true
	default:
		return false
	}
}

func executionToPO(execution *entity.Execution) *scheduledTaskExecutionPO {
	return &scheduledTaskExecutionPO{ID: execution.ID, TaskID: execution.TaskID, SpaceID: execution.SpaceID, TriggerType: execution.TriggerType, ScheduledAt: execution.ScheduledAt, Status: string(execution.Status), Attempt: execution.Attempt, IdempotencyKey: execution.IdempotencyKey, ThreadID: execution.ThreadID, RunID: execution.RunID, WorkflowExecutionID: execution.WorkflowExecutionID, ErrorCode: execution.ErrorCode, ErrorMessage: execution.ErrorMessage, StartedAt: execution.StartedAt, FinishedAt: execution.FinishedAt, CreatedAt: execution.CreatedAt, UpdatedAt: execution.UpdatedAt}
}

func (po *scheduledTaskExecutionPO) toEntity() *entity.Execution {
	return &entity.Execution{ID: po.ID, TaskID: po.TaskID, SpaceID: po.SpaceID, TriggerType: po.TriggerType, ScheduledAt: po.ScheduledAt, Status: entity.ExecutionStatus(po.Status), Attempt: po.Attempt, IdempotencyKey: po.IdempotencyKey, ThreadID: po.ThreadID, RunID: po.RunID, WorkflowExecutionID: po.WorkflowExecutionID, ErrorCode: po.ErrorCode, ErrorMessage: po.ErrorMessage, StartedAt: po.StartedAt, FinishedAt: po.FinishedAt, CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt}
}

func taskMutableUpdates(task *entity.Task) map[string]any {
	return map[string]any{"name": task.Name, "target_type": string(task.TargetType), "target_id": task.TargetID, "target_name": task.TargetName, "target_icon_uri": task.TargetIconURI, "schedule_type": string(task.Schedule.Type), "cron_expr": task.Schedule.CronExpr, "timezone": task.Schedule.Timezone, "run_once_at": task.Schedule.RunOnceAt, "minute": task.Schedule.Minute, "hour": task.Schedule.Hour, "weekday": task.Schedule.Weekday, "payload": datatypes.JSON(task.Payload), "keep_conversation": task.KeepConversation, "max_executions": task.MaxExecutions, "next_execution_at": task.NextExecutionAt}
}

func normalizePage(page, pageSize int32) (int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func requireUpdated(result *gorm.DB, message string) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("%w: %s", ErrStateConflict, message)
	}
	return nil
}

func isDuplicateError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "unique constraint")
}
