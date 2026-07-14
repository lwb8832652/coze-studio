// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/scheduledtask/service"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

var (
	ErrAccessDenied      = errors.New("scheduled task access denied")
	ErrInvalidRequest    = errors.New("invalid scheduled task request")
	ErrTaskLimitExceeded = errors.New("scheduled task limit exceeded")
	ErrTaskDisabled      = errors.New("scheduled task is disabled")
	ErrTargetUnavailable = errors.New("scheduled task target is unavailable")
)

const (
	defaultTaskLimit  = int64(50)
	maxTaskNameRunes  = 100
	maxTaskPayloadLen = 10_000
)

type SpaceAuthorizer interface {
	IsSpaceMember(context.Context, int64, int64) (bool, error)
}

type UserProfileReader interface {
	MGetUserProfiles(context.Context, []int64) ([]*userentity.User, error)
}

type Target struct {
	ID          int64
	Type        entity.TargetType
	Name        string
	IconURI     string
	Published   bool
	InputSchema string
}

type ListTargetsRequest struct {
	SpaceID  int64
	UserID   int64
	Type     entity.TargetType
	Keyword  string
	Page     int32
	PageSize int32
}

type TargetCatalog interface {
	Resolve(context.Context, int64, entity.TargetType, int64) (*Target, error)
	List(context.Context, ListTargetsRequest) ([]*Target, int64, error)
}

type Dispatcher interface {
	Dispatch(context.Context, *entity.Task, *entity.Execution) error
}

type CreateRequest struct {
	SpaceID          int64
	CreatorID        int64
	Name             string
	TargetType       entity.TargetType
	TargetID         int64
	Schedule         entity.Schedule
	Payload          string
	KeepConversation bool
	MaxExecutions    int64
}

type UpdateRequest struct {
	CreateRequest
	TaskID  int64
	Version int64
}

type ListRequest struct {
	SpaceID    int64
	UserID     int64
	TargetType *entity.TargetType
	Status     *entity.Status
	Keyword    string
	Page       int32
	PageSize   int32
}

type ApplicationService struct {
	Repository      repository.Repository
	Authorizer      SpaceAuthorizer
	Users           UserProfileReader
	Targets         TargetCatalog
	Dispatcher      Dispatcher
	Now             func() time.Time
	ManualKey       func() (string, error)
	MaxTasksPerUser int64
}

var SVC = &ApplicationService{}

func (s *ApplicationService) Create(ctx context.Context, req *CreateRequest) (*entity.Task, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request is required", ErrInvalidRequest)
	}
	if err := s.authorize(ctx, req.SpaceID, req.CreatorID); err != nil {
		return nil, err
	}
	count, err := s.Repository.CountActiveTasks(ctx, req.SpaceID, req.CreatorID)
	if err != nil {
		return nil, err
	}
	limit := s.MaxTasksPerUser
	if limit <= 0 {
		limit = defaultTaskLimit
	}
	if count >= limit {
		return nil, ErrTaskLimitExceeded
	}

	task, err := s.normalizedTask(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.Repository.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ApplicationService) Update(ctx context.Context, req *UpdateRequest) (*entity.Task, error) {
	if req == nil || req.TaskID <= 0 || req.Version <= 0 {
		return nil, fmt.Errorf("%w: task id and version are required", ErrInvalidRequest)
	}
	if err := s.authorize(ctx, req.SpaceID, req.CreatorID); err != nil {
		return nil, err
	}
	existing, err := s.Repository.GetTask(ctx, req.SpaceID, req.TaskID)
	if err != nil {
		return nil, err
	}
	normalized, err := s.normalizedTask(ctx, &req.CreateRequest)
	if err != nil {
		return nil, err
	}
	normalized.ID = existing.ID
	normalized.CreatorID = existing.CreatorID
	normalized.Status = existing.Status
	normalized.ExecutionCount = existing.ExecutionCount
	normalized.LatestExecutionAt = existing.LatestExecutionAt
	normalized.LatestExecutionStatus = existing.LatestExecutionStatus
	normalized.ConversationID = existing.ConversationID
	normalized.Version = existing.Version
	if normalized.Status != entity.StatusEnabled {
		normalized.NextExecutionAt = 0
	}
	if err := s.Repository.UpdateTask(ctx, normalized, req.Version); err != nil {
		return nil, err
	}
	normalized.Version = req.Version + 1
	return normalized, nil
}

func (s *ApplicationService) Get(ctx context.Context, userID, spaceID, taskID int64) (*entity.Task, error) {
	if err := s.authorize(ctx, spaceID, userID); err != nil {
		return nil, err
	}
	return s.Repository.GetTask(ctx, spaceID, taskID)
}

func (s *ApplicationService) List(ctx context.Context, req ListRequest) ([]*entity.Task, int64, error) {
	if err := s.authorize(ctx, req.SpaceID, req.UserID); err != nil {
		return nil, 0, err
	}
	tasks, total, err := s.Repository.ListTasks(ctx, repository.ListTaskFilter{SpaceID: req.SpaceID, TargetType: req.TargetType, Status: req.Status, Keyword: req.Keyword, Page: req.Page, PageSize: req.PageSize})
	if err != nil {
		return nil, 0, err
	}
	s.hydrateCreatorNames(ctx, tasks)
	return tasks, total, nil
}

func (s *ApplicationService) hydrateCreatorNames(ctx context.Context, tasks []*entity.Task) {
	if s.Users == nil || len(tasks) == 0 {
		return
	}
	seen := make(map[int64]struct{}, len(tasks))
	userIDs := make([]int64, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if _, ok := seen[task.CreatorID]; !ok {
			seen[task.CreatorID] = struct{}{}
			userIDs = append(userIDs, task.CreatorID)
		}
	}
	users, err := s.Users.MGetUserProfiles(ctx, userIDs)
	if err != nil {
		return
	}
	names := make(map[int64]string, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		name := strings.TrimSpace(user.Name)
		if name == "" {
			name = strings.TrimSpace(user.UniqueName)
		}
		names[user.UserID] = name
	}
	for _, task := range tasks {
		if task != nil {
			task.CreatorName = names[task.CreatorID]
		}
	}
}

func (s *ApplicationService) Delete(ctx context.Context, userID, spaceID, taskID int64) (*entity.Task, error) {
	task, err := s.Get(ctx, userID, spaceID, taskID)
	if err != nil {
		return nil, err
	}
	if err := s.Repository.SoftDeleteTask(ctx, spaceID, taskID); err != nil {
		return nil, err
	}
	return task, nil
}

func (s *ApplicationService) Disable(ctx context.Context, userID, spaceID, taskID int64) (*entity.Task, error) {
	task, err := s.Get(ctx, userID, spaceID, taskID)
	if err != nil {
		return nil, err
	}
	candidate := *task
	if err := candidate.Disable(); err != nil {
		return nil, err
	}
	if err := s.Repository.SetTaskStatus(ctx, spaceID, taskID, entity.StatusEnabled, entity.StatusDisabled, 0); err != nil {
		return nil, err
	}
	task.Status = candidate.Status
	task.NextExecutionAt = 0
	return task, nil
}

func (s *ApplicationService) Enable(ctx context.Context, userID, spaceID, taskID int64) (*entity.Task, error) {
	task, err := s.Get(ctx, userID, spaceID, taskID)
	if err != nil {
		return nil, err
	}
	candidate := *task
	if err := candidate.Enable(); err != nil {
		return nil, err
	}
	next, err := domainservice.NextExecutionAt(task.Schedule, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.Repository.SetTaskStatus(ctx, spaceID, taskID, entity.StatusDisabled, entity.StatusEnabled, next); err != nil {
		return nil, err
	}
	task.Status = candidate.Status
	task.NextExecutionAt = next
	return task, nil
}

func (s *ApplicationService) ExecuteNow(ctx context.Context, userID, spaceID, taskID int64) (*entity.Execution, error) {
	task, err := s.Get(ctx, userID, spaceID, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status == entity.StatusCompleted {
		return nil, ErrTaskDisabled
	}
	key, err := s.manualKey()()
	if err != nil {
		return nil, err
	}
	now := s.now().UnixMilli()
	execution := &entity.Execution{TaskID: task.ID, SpaceID: task.SpaceID, TriggerType: "manual", ScheduledAt: now, Status: entity.ExecutionStatusQueued, IdempotencyKey: "manual:" + key, CreatedAt: now, UpdatedAt: now}
	if err := s.Repository.CreateExecution(ctx, execution); err != nil {
		return nil, err
	}
	if s.Dispatcher == nil {
		return nil, fmt.Errorf("scheduled task dispatcher is unavailable")
	}
	if err := s.Dispatcher.Dispatch(ctx, task, execution); err != nil {
		return nil, err
	}
	return execution, nil
}

func (s *ApplicationService) ListExecutions(ctx context.Context, userID, spaceID, taskID int64, page, pageSize int32) ([]*entity.Execution, int64, error) {
	if _, err := s.Get(ctx, userID, spaceID, taskID); err != nil {
		return nil, 0, err
	}
	return s.Repository.ListExecutions(ctx, spaceID, taskID, page, pageSize)
}

func (s *ApplicationService) ListTargets(ctx context.Context, req ListTargetsRequest) ([]*Target, int64, error) {
	if err := s.authorize(ctx, req.SpaceID, req.UserID); err != nil {
		return nil, 0, err
	}
	if s.Targets == nil {
		return nil, 0, ErrTargetUnavailable
	}
	return s.Targets.List(ctx, req)
}

func (s *ApplicationService) normalizedTask(ctx context.Context, req *CreateRequest) (*entity.Task, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || len([]rune(name)) > maxTaskNameRunes {
		return nil, fmt.Errorf("%w: name must contain 1 to %d characters", ErrInvalidRequest, maxTaskNameRunes)
	}
	if req.TargetID <= 0 || (req.TargetType != entity.TargetTypeAgent && req.TargetType != entity.TargetTypeWorkflow) {
		return nil, fmt.Errorf("%w: invalid target", ErrInvalidRequest)
	}
	if err := validatePayload(req.TargetType, req.Payload); err != nil {
		return nil, err
	}
	if s.Targets == nil {
		return nil, ErrTargetUnavailable
	}
	target, err := s.Targets.Resolve(ctx, req.SpaceID, req.TargetType, req.TargetID)
	if err != nil || target == nil || !target.Published {
		return nil, fmt.Errorf("%w: target must exist in the workspace and be published", ErrTargetUnavailable)
	}
	next, err := domainservice.NextExecutionAt(req.Schedule, s.now())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	maxExecutions := req.MaxExecutions
	if req.Schedule.Type == entity.ScheduleTypeOnce {
		maxExecutions = 1
	}
	keepConversation := req.KeepConversation && req.TargetType == entity.TargetTypeAgent
	return &entity.Task{SpaceID: req.SpaceID, CreatorID: req.CreatorID, Name: name, TargetType: req.TargetType, TargetID: req.TargetID, TargetName: target.Name, TargetIconURI: target.IconURI, Schedule: req.Schedule, Payload: req.Payload, KeepConversation: keepConversation, Status: entity.StatusEnabled, MaxExecutions: maxExecutions, NextExecutionAt: next, Version: 1}, nil
}

func (s *ApplicationService) authorize(ctx context.Context, spaceID, userID int64) error {
	if spaceID <= 0 || userID <= 0 || s.Repository == nil || s.Authorizer == nil {
		return ErrAccessDenied
	}
	member, err := s.Authorizer.IsSpaceMember(ctx, spaceID, userID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAccessDenied, err)
	}
	if !member {
		return ErrAccessDenied
	}
	return nil
}

func (s *ApplicationService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *ApplicationService) manualKey() func() (string, error) {
	if s.ManualKey != nil {
		return s.ManualKey
	}
	return func() (string, error) {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return "", err
		}
		return hex.EncodeToString(bytes[:]), nil
	}
}

func validatePayload(targetType entity.TargetType, payload string) error {
	if len(payload) == 0 || len(payload) > maxTaskPayloadLen || !json.Valid([]byte(payload)) {
		return fmt.Errorf("%w: payload must be valid JSON with at most %d bytes", ErrInvalidRequest, maxTaskPayloadLen)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidRequest)
	}
	if targetType == entity.TargetTypeAgent {
		message, _ := value["message"].(string)
		if strings.TrimSpace(message) == "" {
			return fmt.Errorf("%w: agent payload message is required", ErrInvalidRequest)
		}
	}
	return nil
}

type CronPreset struct {
	ID           string
	Label        string
	ScheduleType entity.ScheduleType
	CronExpr     string
}

func CronPresets() []CronPreset {
	return []CronPreset{
		{ID: "hourly", Label: "每小时", ScheduleType: entity.ScheduleTypeHourly},
		{ID: "daily", Label: "每天", ScheduleType: entity.ScheduleTypeDaily},
		{ID: "weekly", Label: "每周", ScheduleType: entity.ScheduleTypeWeekly},
		{ID: "weekdays", Label: "工作日 09:00", ScheduleType: entity.ScheduleTypeCron, CronExpr: "0 9 * * 1-5"},
		{ID: "once", Label: "仅执行一次", ScheduleType: entity.ScheduleTypeOnce},
	}
}
