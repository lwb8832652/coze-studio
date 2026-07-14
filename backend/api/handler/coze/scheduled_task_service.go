// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"gorm.io/gorm"

	appscheduledtask "github.com/coze-dev/coze-studio/backend/application/scheduledtask"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	scheduledtaskrepo "github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type scheduledTaskID int64

func (v *scheduledTaskID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) || len(data) == 0 {
		*v = 0
		return nil
	}
	data = bytes.Trim(data, `"`)
	parsed, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*v = scheduledTaskID(parsed)
	return nil
}

func (v *scheduledTaskID) UnmarshalText(data []byte) error {
	parsed, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*v = scheduledTaskID(parsed)
	return nil
}

type scheduledTaskMutationRequest struct {
	SpaceID          scheduledTaskID `json:"space_id" query:"space_id"`
	Name             string          `json:"name"`
	TargetType       int32           `json:"target_type"`
	TargetID         scheduledTaskID `json:"target_id"`
	ScheduleType     int32           `json:"schedule_type"`
	CronExpr         string          `json:"cron_expr"`
	Timezone         string          `json:"timezone"`
	RunOnceAt        int64           `json:"run_once_at"`
	Minute           int32           `json:"minute"`
	Hour             int32           `json:"hour"`
	Weekday          int32           `json:"weekday"`
	Payload          string          `json:"payload"`
	KeepConversation bool            `json:"keep_conversation"`
	MaxExecutions    int64           `json:"max_executions"`
	Version          int64           `json:"version"`
}

type scheduledTaskListRequest struct {
	SpaceID    scheduledTaskID `query:"space_id" json:"space_id"`
	TargetType int32           `query:"target_type" json:"target_type"`
	Status     int32           `query:"status" json:"status"`
	Keyword    string          `query:"keyword" json:"keyword"`
	Page       int32           `query:"page" json:"page"`
	PageSize   int32           `query:"page_size" json:"page_size"`
}

type scheduledTaskActionRequest struct {
	SpaceID  scheduledTaskID `query:"space_id" json:"space_id"`
	Page     int32           `query:"page" json:"page"`
	PageSize int32           `query:"page_size" json:"page_size"`
}

type scheduledTaskTargetsRequest struct {
	SpaceID    scheduledTaskID `query:"space_id" json:"space_id"`
	TargetType int32           `query:"target_type" json:"target_type"`
	Keyword    string          `query:"keyword" json:"keyword"`
	Page       int32           `query:"page" json:"page"`
	PageSize   int32           `query:"page_size" json:"page_size"`
}

type scheduledTaskAPI struct {
	ID                    string `json:"id"`
	SpaceID               string `json:"space_id"`
	CreatorID             string `json:"creator_id"`
	CreatorName           string `json:"creator_name,omitempty"`
	Name                  string `json:"name"`
	TargetType            int32  `json:"target_type"`
	TargetID              string `json:"target_id"`
	TargetName            string `json:"target_name"`
	TargetIconURI         string `json:"target_icon_uri,omitempty"`
	ScheduleType          int32  `json:"schedule_type"`
	CronExpr              string `json:"cron_expr,omitempty"`
	Timezone              string `json:"timezone"`
	RunOnceAt             int64  `json:"run_once_at,omitempty"`
	Minute                int32  `json:"minute,omitempty"`
	Hour                  int32  `json:"hour,omitempty"`
	Weekday               int32  `json:"weekday,omitempty"`
	Payload               string `json:"payload"`
	KeepConversation      bool   `json:"keep_conversation"`
	Status                int32  `json:"status"`
	ExecutionCount        int64  `json:"execution_count"`
	MaxExecutions         int64  `json:"max_executions"`
	LatestExecutionAt     int64  `json:"latest_execution_at"`
	LatestExecutionStatus int32  `json:"latest_execution_status,omitempty"`
	NextExecutionAt       int64  `json:"next_execution_at"`
	CreatedAt             int64  `json:"created_at"`
	UpdatedAt             int64  `json:"updated_at"`
	Version               int64  `json:"version"`
}

type scheduledTaskExecutionAPI struct {
	ID                  string `json:"id"`
	TaskID              string `json:"task_id"`
	TriggerType         string `json:"trigger_type"`
	ScheduledAt         int64  `json:"scheduled_at"`
	Status              int32  `json:"status"`
	Attempt             int32  `json:"attempt"`
	ThreadID            string `json:"thread_id,omitempty"`
	RunID               string `json:"run_id,omitempty"`
	WorkflowExecutionID string `json:"workflow_execution_id,omitempty"`
	ErrorCode           string `json:"error_code,omitempty"`
	ErrorMessage        string `json:"error_message,omitempty"`
	StartedAt           int64  `json:"started_at"`
	FinishedAt          int64  `json:"finished_at"`
	CreatedAt           int64  `json:"created_at"`
}

type scheduledTaskTargetAPI struct {
	ID          string `json:"id"`
	Type        int32  `json:"type"`
	Name        string `json:"name"`
	IconURI     string `json:"icon_uri,omitempty"`
	Published   bool   `json:"published"`
	InputSchema string `json:"input_schema,omitempty"`
}

type scheduledTaskCronPresetAPI struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ScheduleType int32  `json:"schedule_type"`
	CronExpr     string `json:"cron_expr,omitempty"`
}

func CreateScheduledTask(ctx context.Context, c *app.RequestContext) {
	var req scheduledTaskMutationRequest
	if err := c.BindAndValidate(&req); err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	createReq, err := scheduledTaskCreateRequest(ctx, req)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	task, err := appscheduledtask.SVC.Create(ctx, createReq)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	scheduledTaskJSONSuccess(c, scheduledTaskToAPI(task))
}

func ListScheduledTasks(ctx context.Context, c *app.RequestContext) {
	var req scheduledTaskListRequest
	if err := c.BindAndValidate(&req); err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	listReq := appscheduledtask.ListRequest{
		SpaceID: int64(req.SpaceID), UserID: workbenchViewerIDFromCtx(ctx), Keyword: strings.TrimSpace(req.Keyword),
		Page: req.Page, PageSize: req.PageSize,
	}
	if req.TargetType != 0 {
		value, ok := scheduledTaskTargetTypeFromAPI(req.TargetType)
		if !ok {
			scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
			return
		}
		listReq.TargetType = &value
	}
	if req.Status != 0 {
		value, ok := scheduledTaskStatusFromAPI(req.Status)
		if !ok {
			scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
			return
		}
		listReq.Status = &value
	}
	tasks, total, err := appscheduledtask.SVC.List(ctx, listReq)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	items := make([]*scheduledTaskAPI, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, scheduledTaskToAPI(task))
	}
	scheduledTaskJSONSuccess(c, map[string]any{"tasks": items, "total": total})
}

func GetScheduledTask(ctx context.Context, c *app.RequestContext) {
	spaceID, taskID, ok := scheduledTaskResourceIDs(ctx, c)
	if !ok {
		return
	}
	task, err := appscheduledtask.SVC.Get(ctx, workbenchViewerIDFromCtx(ctx), spaceID, taskID)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	scheduledTaskJSONSuccess(c, scheduledTaskToAPI(task))
}

func UpdateScheduledTask(ctx context.Context, c *app.RequestContext) {
	var req scheduledTaskMutationRequest
	if err := c.BindAndValidate(&req); err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	taskID, err := scheduledTaskParseID(c.Param("task_id"))
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	createReq, err := scheduledTaskCreateRequest(ctx, req)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	task, err := appscheduledtask.SVC.Update(ctx, &appscheduledtask.UpdateRequest{
		CreateRequest: *createReq,
		TaskID:        taskID,
		Version:       req.Version,
	})
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	scheduledTaskJSONSuccess(c, scheduledTaskToAPI(task))
}

func DeleteScheduledTask(ctx context.Context, c *app.RequestContext) {
	scheduledTaskAction(ctx, c, appscheduledtask.SVC.Delete)
}

func EnableScheduledTask(ctx context.Context, c *app.RequestContext) {
	scheduledTaskAction(ctx, c, appscheduledtask.SVC.Enable)
}

func DisableScheduledTask(ctx context.Context, c *app.RequestContext) {
	scheduledTaskAction(ctx, c, appscheduledtask.SVC.Disable)
}

func ExecuteScheduledTask(ctx context.Context, c *app.RequestContext) {
	spaceID, taskID, ok := scheduledTaskResourceIDs(ctx, c)
	if !ok {
		return
	}
	execution, err := appscheduledtask.SVC.ExecuteNow(ctx, workbenchViewerIDFromCtx(ctx), spaceID, taskID)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	scheduledTaskJSONSuccess(c, scheduledTaskExecutionToAPI(execution))
}

func ListScheduledTaskExecutions(ctx context.Context, c *app.RequestContext) {
	var req scheduledTaskActionRequest
	if err := c.BindAndValidate(&req); err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	taskID, err := scheduledTaskParseID(c.Param("task_id"))
	if err != nil || req.SpaceID <= 0 {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	executions, total, err := appscheduledtask.SVC.ListExecutions(
		ctx, workbenchViewerIDFromCtx(ctx), int64(req.SpaceID), taskID, req.Page, req.PageSize,
	)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	items := make([]*scheduledTaskExecutionAPI, 0, len(executions))
	for _, execution := range executions {
		items = append(items, scheduledTaskExecutionToAPI(execution))
	}
	scheduledTaskJSONSuccess(c, map[string]any{"executions": items, "total": total})
}

func ListScheduledTaskTargets(ctx context.Context, c *app.RequestContext) {
	var req scheduledTaskTargetsRequest
	if err := c.BindAndValidate(&req); err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	targetType, ok := scheduledTaskTargetTypeFromAPI(req.TargetType)
	if !ok || req.SpaceID <= 0 {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return
	}
	targets, total, err := appscheduledtask.SVC.ListTargets(ctx, appscheduledtask.ListTargetsRequest{
		SpaceID: int64(req.SpaceID), UserID: workbenchViewerIDFromCtx(ctx), Type: targetType,
		Keyword: strings.TrimSpace(req.Keyword), Page: req.Page, PageSize: req.PageSize,
	})
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	items := make([]*scheduledTaskTargetAPI, 0, len(targets))
	for _, target := range targets {
		items = append(items, &scheduledTaskTargetAPI{
			ID: strconv.FormatInt(target.ID, 10), Type: scheduledTaskTargetTypeToAPI(target.Type),
			Name: target.Name, IconURI: target.IconURI, Published: target.Published, InputSchema: target.InputSchema,
		})
	}
	scheduledTaskJSONSuccess(c, map[string]any{"targets": items, "total": total})
}

func ListScheduledTaskCronPresets(ctx context.Context, c *app.RequestContext) {
	_ = ctx
	items := make([]*scheduledTaskCronPresetAPI, 0, len(appscheduledtask.CronPresets()))
	for _, preset := range appscheduledtask.CronPresets() {
		items = append(items, &scheduledTaskCronPresetAPI{
			ID: preset.ID, Label: preset.Label,
			ScheduleType: scheduledTaskScheduleTypeToAPI(preset.ScheduleType), CronExpr: preset.CronExpr,
		})
	}
	scheduledTaskJSONSuccess(c, items)
}

func scheduledTaskCreateRequest(ctx context.Context, req scheduledTaskMutationRequest) (*appscheduledtask.CreateRequest, error) {
	targetType, ok := scheduledTaskTargetTypeFromAPI(req.TargetType)
	if !ok || req.SpaceID <= 0 || req.TargetID <= 0 {
		return nil, appscheduledtask.ErrInvalidRequest
	}
	scheduleType, ok := scheduledTaskScheduleTypeFromAPI(req.ScheduleType)
	if !ok {
		return nil, appscheduledtask.ErrInvalidRequest
	}
	return &appscheduledtask.CreateRequest{
		SpaceID: int64(req.SpaceID), CreatorID: workbenchViewerIDFromCtx(ctx), Name: req.Name,
		TargetType: targetType, TargetID: int64(req.TargetID),
		Schedule: entity.Schedule{
			Type: scheduleType, Timezone: req.Timezone, CronExpr: req.CronExpr,
			RunOnceAt: scheduledTaskTimestampToMillis(req.RunOnceAt), Minute: int(req.Minute), Hour: int(req.Hour), Weekday: int(req.Weekday),
		},
		Payload: req.Payload, KeepConversation: req.KeepConversation, MaxExecutions: req.MaxExecutions,
	}, nil
}

func scheduledTaskAction(
	ctx context.Context,
	c *app.RequestContext,
	action func(context.Context, int64, int64, int64) (*entity.Task, error),
) {
	spaceID, taskID, ok := scheduledTaskResourceIDs(ctx, c)
	if !ok {
		return
	}
	task, err := action(ctx, workbenchViewerIDFromCtx(ctx), spaceID, taskID)
	if err != nil {
		scheduledTaskErrorResponse(ctx, c, err)
		return
	}
	scheduledTaskJSONSuccess(c, scheduledTaskToAPI(task))
}

func scheduledTaskResourceIDs(ctx context.Context, c *app.RequestContext) (int64, int64, bool) {
	var req scheduledTaskActionRequest
	if err := c.BindAndValidate(&req); err != nil {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return 0, 0, false
	}
	taskID, err := scheduledTaskParseID(c.Param("task_id"))
	if err != nil || req.SpaceID <= 0 {
		scheduledTaskErrorResponse(ctx, c, appscheduledtask.ErrInvalidRequest)
		return 0, 0, false
	}
	return int64(req.SpaceID), taskID, true
}

func scheduledTaskParseID(raw string) (int64, error) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, appscheduledtask.ErrInvalidRequest
	}
	return value, nil
}

func scheduledTaskTimestampToMillis(value int64) int64 {
	if value > 0 && value < 1_000_000_000_000 {
		return value * 1000
	}
	return value
}

func scheduledTaskToAPI(task *entity.Task) *scheduledTaskAPI {
	if task == nil {
		return nil
	}
	return &scheduledTaskAPI{
		ID: strconv.FormatInt(task.ID, 10), SpaceID: strconv.FormatInt(task.SpaceID, 10),
		CreatorID: strconv.FormatInt(task.CreatorID, 10), CreatorName: task.CreatorName, Name: task.Name,
		TargetType: scheduledTaskTargetTypeToAPI(task.TargetType), TargetID: strconv.FormatInt(task.TargetID, 10),
		TargetName: task.TargetName, TargetIconURI: task.TargetIconURI,
		ScheduleType: scheduledTaskScheduleTypeToAPI(task.Schedule.Type), CronExpr: task.Schedule.CronExpr,
		Timezone: task.Schedule.Timezone, RunOnceAt: task.Schedule.RunOnceAt,
		Minute: int32(task.Schedule.Minute), Hour: int32(task.Schedule.Hour), Weekday: int32(task.Schedule.Weekday),
		Payload: task.Payload, KeepConversation: task.KeepConversation,
		Status: scheduledTaskStatusToAPI(task.Status), ExecutionCount: task.ExecutionCount,
		MaxExecutions: task.MaxExecutions, LatestExecutionAt: task.LatestExecutionAt,
		LatestExecutionStatus: scheduledTaskExecutionStatusToAPI(task.LatestExecutionStatus),
		NextExecutionAt:       task.NextExecutionAt, CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
		Version: task.Version,
	}
}

func scheduledTaskExecutionToAPI(execution *entity.Execution) *scheduledTaskExecutionAPI {
	if execution == nil {
		return nil
	}
	result := &scheduledTaskExecutionAPI{
		ID: strconv.FormatInt(execution.ID, 10), TaskID: strconv.FormatInt(execution.TaskID, 10),
		TriggerType: execution.TriggerType, ScheduledAt: execution.ScheduledAt,
		Status: scheduledTaskExecutionStatusToAPI(execution.Status), Attempt: execution.Attempt,
		ErrorCode: execution.ErrorCode, ErrorMessage: execution.ErrorMessage,
		StartedAt: execution.StartedAt, FinishedAt: execution.FinishedAt, CreatedAt: execution.CreatedAt,
	}
	if execution.ThreadID > 0 {
		result.ThreadID = strconv.FormatInt(execution.ThreadID, 10)
	}
	if execution.RunID > 0 {
		result.RunID = strconv.FormatInt(execution.RunID, 10)
	}
	if execution.WorkflowExecutionID > 0 {
		result.WorkflowExecutionID = strconv.FormatInt(execution.WorkflowExecutionID, 10)
	}
	return result
}

func scheduledTaskJSONSuccess(c *app.RequestContext, data any) {
	c.JSON(http.StatusOK, map[string]any{"code": int64(0), "msg": "success", "data": data})
}

func scheduledTaskErrorResponse(ctx context.Context, c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, appscheduledtask.ErrInvalidRequest):
		c.JSON(http.StatusBadRequest, map[string]any{"code": int64(http.StatusBadRequest), "msg": "任务配置无效"})
	case errors.Is(err, appscheduledtask.ErrAccessDenied):
		c.JSON(http.StatusForbidden, map[string]any{"code": int64(http.StatusForbidden), "msg": "无权访问该工作空间"})
	case errors.Is(err, appscheduledtask.ErrTargetUnavailable):
		c.JSON(http.StatusBadRequest, map[string]any{"code": int64(http.StatusBadRequest), "msg": "执行目标不可用或尚未发布"})
	case errors.Is(err, appscheduledtask.ErrTaskLimitExceeded):
		c.JSON(http.StatusConflict, map[string]any{"code": int64(http.StatusConflict), "msg": "启用中的定时任务已达到上限"})
	case errors.Is(err, appscheduledtask.ErrTaskDisabled):
		c.JSON(http.StatusConflict, map[string]any{"code": int64(http.StatusConflict), "msg": "任务当前不可执行"})
	case errors.Is(err, scheduledtaskrepo.ErrStateConflict):
		c.JSON(http.StatusConflict, map[string]any{"code": int64(http.StatusConflict), "msg": "任务状态已变化，请刷新后重试"})
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, map[string]any{"code": int64(http.StatusNotFound), "msg": "任务不存在或已删除"})
	default:
		logs.CtxErrorf(ctx, "scheduled task request failed: %v", err)
		c.JSON(http.StatusInternalServerError, map[string]any{
			"code": int64(http.StatusInternalServerError), "msg": "任务中心服务暂时不可用",
		})
	}
}

func scheduledTaskTargetTypeFromAPI(value int32) (entity.TargetType, bool) {
	switch value {
	case 1:
		return entity.TargetTypeAgent, true
	case 2:
		return entity.TargetTypeWorkflow, true
	default:
		return "", false
	}
}

func scheduledTaskTargetTypeToAPI(value entity.TargetType) int32 {
	if value == entity.TargetTypeWorkflow {
		return 2
	}
	return 1
}

func scheduledTaskScheduleTypeFromAPI(value int32) (entity.ScheduleType, bool) {
	switch value {
	case 1:
		return entity.ScheduleTypeOnce, true
	case 2:
		return entity.ScheduleTypeHourly, true
	case 3:
		return entity.ScheduleTypeDaily, true
	case 4:
		return entity.ScheduleTypeWeekly, true
	case 5:
		return entity.ScheduleTypeCron, true
	default:
		return "", false
	}
}

func scheduledTaskScheduleTypeToAPI(value entity.ScheduleType) int32 {
	switch value {
	case entity.ScheduleTypeOnce:
		return 1
	case entity.ScheduleTypeHourly:
		return 2
	case entity.ScheduleTypeDaily:
		return 3
	case entity.ScheduleTypeWeekly:
		return 4
	case entity.ScheduleTypeCron:
		return 5
	default:
		return 0
	}
}

func scheduledTaskStatusFromAPI(value int32) (entity.Status, bool) {
	switch value {
	case 1:
		return entity.StatusEnabled, true
	case 2:
		return entity.StatusDisabled, true
	case 3:
		return entity.StatusCompleted, true
	default:
		return "", false
	}
}

func scheduledTaskStatusToAPI(value entity.Status) int32 {
	switch value {
	case entity.StatusEnabled:
		return 1
	case entity.StatusDisabled:
		return 2
	case entity.StatusCompleted:
		return 3
	default:
		return 0
	}
}

func scheduledTaskExecutionStatusToAPI(value entity.ExecutionStatus) int32 {
	switch value {
	case entity.ExecutionStatusQueued:
		return 1
	case entity.ExecutionStatusRunning:
		return 2
	case entity.ExecutionStatusSucceeded:
		return 3
	case entity.ExecutionStatusFailed:
		return 4
	case entity.ExecutionStatusCanceled:
		return 5
	default:
		return 0
	}
}

var _ json.Unmarshaler = (*scheduledTaskID)(nil)
