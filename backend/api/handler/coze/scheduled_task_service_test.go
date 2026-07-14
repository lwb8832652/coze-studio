// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
)

func TestScheduledTaskToAPIUsesStableEnumAndStringIDs(t *testing.T) {
	t.Parallel()
	apiTask := scheduledTaskToAPI(&entity.Task{
		ID: 10, SpaceID: 20, CreatorID: 30, Name: "日报",
		TargetType: entity.TargetTypeWorkflow, TargetID: 40, TargetName: "日报流程",
		Schedule: entity.Schedule{Type: entity.ScheduleTypeDaily, Timezone: "Asia/Shanghai", Hour: 9},
		Status:   entity.StatusEnabled, NextExecutionAt: 1000,
		CreatorName: "刘文波", LatestExecutionStatus: entity.ExecutionStatusSucceeded,
	})

	require.Equal(t, "10", apiTask.ID)
	require.Equal(t, int32(2), apiTask.TargetType)
	require.Equal(t, int32(3), apiTask.ScheduleType)
	require.Equal(t, int32(1), apiTask.Status)
	require.Equal(t, "刘文波", apiTask.CreatorName)
	require.Equal(t, int32(3), apiTask.LatestExecutionStatus)
}

func TestListScheduledTaskCronPresetsHandler(t *testing.T) {
	t.Parallel()
	h := server.Default()
	h.GET("/api/workbench/scheduled_task_cron_presets", ListScheduledTaskCronPresets)

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/api/workbench/scheduled_task_cron_presets?timezone=Asia%2FShanghai", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"label":"每小时"`)
	require.Contains(t, body, `"label":"仅执行一次"`)
}

func TestScheduledTaskErrorResponseHidesInternalFailure(t *testing.T) {
	t.Parallel()
	h := server.Default()
	h.GET("/scheduled-task-error", func(ctx context.Context, c *app.RequestContext) {
		scheduledTaskErrorResponse(ctx, c, errors.New("mysql password secret"))
	})

	w := ut.PerformRequest(h.Engine, http.MethodGet, "/scheduled-task-error", nil)
	body := string(w.Result().Body())

	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NotContains(t, body, "mysql password secret")
	require.Contains(t, body, "任务中心服务暂时不可用")
}

func TestScheduledTaskTimestampAcceptsSecondsAndMilliseconds(t *testing.T) {
	t.Parallel()
	require.Equal(t, int64(1720000000000), scheduledTaskTimestampToMillis(1720000000))
	require.Equal(t, int64(1720000000000), scheduledTaskTimestampToMillis(1720000000000))
}
