// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterIncludesScheduledTaskCenterRoutes(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile("api.go")
	require.NoError(t, err)
	text := string(source)

	for _, route := range []string{
		`_workbench.GET("/scheduled_tasks", coze.ListScheduledTasks)`,
		`_workbench.POST("/scheduled_tasks", coze.CreateScheduledTask)`,
		`_scheduled_tasks.GET("/:task_id", coze.GetScheduledTask)`,
		`_scheduled_tasks.PUT("/:task_id", coze.UpdateScheduledTask)`,
		`_scheduled_tasks.DELETE("/:task_id", coze.DeleteScheduledTask)`,
		`_scheduled_tasks.POST("/:task_id/enable", coze.EnableScheduledTask)`,
		`_scheduled_tasks.POST("/:task_id/disable", coze.DisableScheduledTask)`,
		`_scheduled_tasks.POST("/:task_id/execute", coze.ExecuteScheduledTask)`,
		`_scheduled_tasks.GET("/:task_id/executions", coze.ListScheduledTaskExecutions)`,
		`_workbench.GET("/scheduled_task_targets", coze.ListScheduledTaskTargets)`,
		`_workbench.GET("/scheduled_task_cron_presets", coze.ListScheduledTaskCronPresets)`,
	} {
		require.Contains(t, text, route)
	}
}
