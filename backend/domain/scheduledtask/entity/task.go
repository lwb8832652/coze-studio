// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package entity

import "fmt"

type TargetType string

const (
	TargetTypeAgent    TargetType = "agent"
	TargetTypeWorkflow TargetType = "workflow"
)

type ScheduleType string

const (
	ScheduleTypeOnce   ScheduleType = "once"
	ScheduleTypeHourly ScheduleType = "hourly"
	ScheduleTypeDaily  ScheduleType = "daily"
	ScheduleTypeWeekly ScheduleType = "weekly"
	ScheduleTypeCron   ScheduleType = "cron"
)

type Status string

const (
	StatusEnabled   Status = "enabled"
	StatusDisabled  Status = "disabled"
	StatusCompleted Status = "completed"
)

type ExecutionStatus string

const (
	ExecutionStatusQueued    ExecutionStatus = "queued"
	ExecutionStatusRunning   ExecutionStatus = "running"
	ExecutionStatusSucceeded ExecutionStatus = "succeeded"
	ExecutionStatusFailed    ExecutionStatus = "failed"
	ExecutionStatusCanceled  ExecutionStatus = "canceled"
)

type Schedule struct {
	Type      ScheduleType
	Timezone  string
	CronExpr  string
	RunOnceAt int64
	Minute    int
	Hour      int
	Weekday   int
}

type Task struct {
	ID                    int64
	SpaceID               int64
	CreatorID             int64
	CreatorName           string
	Name                  string
	TargetType            TargetType
	TargetID              int64
	TargetName            string
	TargetIconURI         string
	Schedule              Schedule
	Payload               string
	KeepConversation      bool
	ConversationID        int64
	Status                Status
	ExecutionCount        int64
	MaxExecutions         int64
	LatestExecutionAt     int64
	LatestExecutionStatus ExecutionStatus
	NextExecutionAt       int64
	LeaseOwner            string
	LeaseExpiresAt        int64
	Version               int64
	CreatedAt             int64
	UpdatedAt             int64
	DeletedAt             int64
}

type Execution struct {
	ID                  int64
	TaskID              int64
	SpaceID             int64
	TriggerType         string
	ScheduledAt         int64
	Status              ExecutionStatus
	Attempt             int32
	IdempotencyKey      string
	ThreadID            int64
	RunID               int64
	WorkflowExecutionID int64
	ErrorCode           string
	ErrorMessage        string
	StartedAt           int64
	FinishedAt          int64
	CreatedAt           int64
	UpdatedAt           int64
}

func (t *Task) Disable() error {
	if t == nil || t.Status != StatusEnabled {
		return fmt.Errorf("only enabled scheduled tasks can be disabled")
	}
	t.Status = StatusDisabled
	return nil
}

func (t *Task) Enable() error {
	if t == nil || t.Status != StatusDisabled {
		return fmt.Errorf("only disabled scheduled tasks can be enabled")
	}
	t.Status = StatusEnabled
	return nil
}

func (t *Task) Complete() error {
	if t == nil || t.Status == StatusCompleted {
		return fmt.Errorf("scheduled task is already completed")
	}
	t.Status = StatusCompleted
	return nil
}
