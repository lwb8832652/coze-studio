// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

func TestCreateRejectsUnauthorizedWorkspace(t *testing.T) {
	t.Parallel()
	service := newTestService()
	service.Authorizer = membershipAuthorizer(false)

	_, err := service.Create(context.Background(), validCreateRequest())

	require.ErrorIs(t, err, ErrAccessDenied)
}

func TestCreateValidatesTargetPayloadAndSchedule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*CreateRequest)
	}{
		{name: "empty name", mutate: func(req *CreateRequest) { req.Name = "" }},
		{name: "invalid payload", mutate: func(req *CreateRequest) { req.Payload = "{" }},
		{name: "missing target", mutate: func(req *CreateRequest) { req.TargetID = 999 }},
		{name: "past one-time", mutate: func(req *CreateRequest) {
			req.Schedule.Type = entity.ScheduleTypeOnce
			req.Schedule.RunOnceAt = fixedNow().Add(-time.Minute).UnixMilli()
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newTestService()
			req := validCreateRequest()
			tt.mutate(req)

			_, err := service.Create(context.Background(), req)

			require.Error(t, err)
		})
	}
}

func TestCreatePersistsNormalizedTask(t *testing.T) {
	t.Parallel()
	service := newTestService()
	req := validCreateRequest()
	req.Schedule = entity.Schedule{
		Type:      entity.ScheduleTypeOnce,
		Timezone:  "Asia/Shanghai",
		RunOnceAt: fixedNow().Add(time.Hour).UnixMilli(),
	}
	req.MaxExecutions = 0

	task, err := service.Create(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, int64(1), task.MaxExecutions)
	require.Equal(t, fixedNow().Add(time.Hour).UnixMilli(), task.NextExecutionAt)
	require.Equal(t, "日报助手", task.TargetName)
	require.Equal(t, entity.StatusEnabled, task.Status)
}

func TestCreateEnforcesPerUserActiveTaskLimit(t *testing.T) {
	t.Parallel()
	service := newTestService()
	service.MaxTasksPerUser = 1
	repo := service.Repository.(*recordingRepository)
	repo.activeCount = 1

	_, err := service.Create(context.Background(), validCreateRequest())

	require.ErrorIs(t, err, ErrTaskLimitExceeded)
}

func TestDisableAndEnableRecalculateSchedule(t *testing.T) {
	t.Parallel()
	service := newTestService()
	repo := service.Repository.(*recordingRepository)
	repo.task = &entity.Task{
		ID: 10, SpaceID: 1, CreatorID: 7, Name: "report",
		Status:   entity.StatusEnabled,
		Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
	}

	disabled, err := service.Disable(context.Background(), 7, 1, 10)
	require.NoError(t, err)
	require.Equal(t, entity.StatusDisabled, disabled.Status)
	require.Zero(t, disabled.NextExecutionAt)

	enabled, err := service.Enable(context.Background(), 7, 1, 10)
	require.NoError(t, err)
	require.Equal(t, entity.StatusEnabled, enabled.Status)
	require.Greater(t, enabled.NextExecutionAt, fixedNow().UnixMilli())
}

func TestExecuteNowCreatesIdempotentExecutionAndDispatches(t *testing.T) {
	t.Parallel()
	service := newTestService()
	repo := service.Repository.(*recordingRepository)
	repo.task = &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, Status: entity.StatusEnabled}
	dispatcher := service.Dispatcher.(*recordingDispatcher)

	execution, err := service.ExecuteNow(context.Background(), 7, 1, 10)

	require.NoError(t, err)
	require.Equal(t, "manual", execution.TriggerType)
	require.Equal(t, entity.ExecutionStatusQueued, execution.Status)
	require.NotEmpty(t, execution.IdempotencyKey)
	require.Same(t, execution, dispatcher.execution)
}

func TestExecuteNowAllowsDisabledTaskForManualTesting(t *testing.T) {
	t.Parallel()
	service := newTestService()
	service.Repository.(*recordingRepository).task = &entity.Task{
		ID: 10, SpaceID: 1, CreatorID: 7, Status: entity.StatusDisabled,
	}

	execution, err := service.ExecuteNow(context.Background(), 7, 1, 10)

	require.NoError(t, err)
	require.NotNil(t, execution)
	require.Equal(t, "manual:manual-key", execution.IdempotencyKey)
}

func TestListHydratesCreatorNamesFromUserDomain(t *testing.T) {
	t.Parallel()
	service := newTestService()
	service.Repository.(*recordingRepository).task = &entity.Task{
		ID: 10, SpaceID: 1, CreatorID: 7, Name: "report",
	}
	service.Users = userProfileReader{users: []*userentity.User{{UserID: 7, Name: "刘文波"}}}

	tasks, total, err := service.List(context.Background(), ListRequest{SpaceID: 1, UserID: 7})

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "刘文波", tasks[0].CreatorName)
}

func newTestService() *ApplicationService {
	return &ApplicationService{
		Repository: &recordingRepository{},
		Authorizer: membershipAuthorizer(true),
		Targets: &recordingTargetCatalog{target: &Target{
			ID: 200, Type: entity.TargetTypeAgent, Name: "日报助手", Published: true,
		}},
		Dispatcher:      &recordingDispatcher{},
		Now:             fixedNow,
		MaxTasksPerUser: 50,
		ManualKey:       func() (string, error) { return "manual-key", nil },
	}
}

func validCreateRequest() *CreateRequest {
	return &CreateRequest{
		SpaceID:       1,
		CreatorID:     7,
		Name:          "工作日报",
		TargetType:    entity.TargetTypeAgent,
		TargetID:      200,
		Schedule:      entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "UTC", Hour: 9},
		Payload:       `{"message":"生成日报","variables":{}}`,
		MaxExecutions: 0,
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
}

type membershipAuthorizer bool

func (a membershipAuthorizer) IsSpaceMember(context.Context, int64, int64) (bool, error) {
	return bool(a), nil
}

type recordingTargetCatalog struct{ target *Target }

func (c *recordingTargetCatalog) Resolve(_ context.Context, spaceID int64, targetType entity.TargetType, targetID int64) (*Target, error) {
	if c.target == nil || c.target.ID != targetID || c.target.Type != targetType || spaceID != 1 {
		return nil, errors.New("target not found")
	}
	return c.target, nil
}

func (c *recordingTargetCatalog) List(context.Context, ListTargetsRequest) ([]*Target, int64, error) {
	if c.target == nil {
		return nil, 0, nil
	}
	return []*Target{c.target}, 1, nil
}

type recordingDispatcher struct{ execution *entity.Execution }

func (d *recordingDispatcher) Dispatch(_ context.Context, _ *entity.Task, execution *entity.Execution) error {
	d.execution = execution
	return nil
}

type recordingRepository struct {
	repository.Repository
	task        *entity.Task
	activeCount int64
	executions  []*entity.Execution
}

func (r *recordingRepository) CreateTask(_ context.Context, task *entity.Task) error {
	task.ID = 10
	r.task = task
	return nil
}

func (r *recordingRepository) GetTask(_ context.Context, spaceID, taskID int64) (*entity.Task, error) {
	if r.task == nil || r.task.SpaceID != spaceID || r.task.ID != taskID {
		return nil, errors.New("not found")
	}
	return r.task, nil
}

func (r *recordingRepository) CountActiveTasks(context.Context, int64, int64) (int64, error) {
	return r.activeCount, nil
}

func (r *recordingRepository) ListTasks(context.Context, repository.ListTaskFilter) ([]*entity.Task, int64, error) {
	if r.task == nil {
		return nil, 0, nil
	}
	return []*entity.Task{r.task}, 1, nil
}

func (r *recordingRepository) SetTaskStatus(_ context.Context, _ int64, _ int64, from, to entity.Status, next int64) error {
	if r.task.Status != from {
		return errors.New("status conflict")
	}
	r.task.Status = to
	r.task.NextExecutionAt = next
	return nil
}

func (r *recordingRepository) CreateExecution(_ context.Context, execution *entity.Execution) error {
	execution.ID = int64(len(r.executions) + 1)
	r.executions = append(r.executions, execution)
	return nil
}

type userProfileReader struct{ users []*userentity.User }

func (r userProfileReader) MGetUserProfiles(context.Context, []int64) ([]*userentity.User, error) {
	return r.users, nil
}
