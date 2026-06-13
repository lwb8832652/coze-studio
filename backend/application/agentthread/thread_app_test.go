/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package agentthread

import (
	"testing"

	"github.com/stretchr/testify/require"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestTaskToThreadSummaryKeepsTaskWordingAndThreadID(t *testing.T) {
	input := `{"message":"整理项目进展"}`
	result := `{"message":"周报已生成"}`
	task := &taskapi.ChatTask{
		ID:        100,
		SpaceID:   1,
		CreatorID: 2,
		Title:     "生成周报",
		Status:    taskapi.TaskStatus_Running,
		Progress:  40,
		Input:     &input,
		Result:    &result,
		CreatedAt: 1717000000000,
		UpdatedAt: 1717000300000,
	}

	summary := TaskToThreadSummary(task)

	require.NotNil(t, summary)
	require.Equal(t, int64(100), summary.ThreadID)
	require.Equal(t, int64(100), summary.LegacyTaskID)
	require.Equal(t, int64(1), summary.SpaceID)
	require.Equal(t, int64(2), summary.CreatorID)
	require.Equal(t, "生成周报", summary.Title)
	require.Equal(t, ThreadStatusRunning, summary.Status)
	require.Equal(t, ThreadSourceWeb, summary.Source)
	require.Equal(t, int32(40), summary.Progress)
	require.Equal(t, "整理项目进展", summary.LastUserMessage)
	require.Equal(t, "周报已生成", summary.LastAgentMessage)
	require.Equal(t, int64(1717000000000), summary.CreatedAt)
	require.Equal(t, int64(1717000300000), summary.UpdatedAt)
}

func TestTaskToThreadSummaryMapsTerminalStatuses(t *testing.T) {
	tests := []struct {
		name   string
		status taskapi.TaskStatus
		want   ThreadStatus
	}{
		{name: "created", status: taskapi.TaskStatus_Created, want: ThreadStatusIdle},
		{name: "queued", status: taskapi.TaskStatus_Queued, want: ThreadStatusRunning},
		{name: "running", status: taskapi.TaskStatus_Running, want: ThreadStatusRunning},
		{name: "succeeded", status: taskapi.TaskStatus_Succeeded, want: ThreadStatusCompleted},
		{name: "failed", status: taskapi.TaskStatus_Failed, want: ThreadStatusFailed},
		{name: "canceling", status: taskapi.TaskStatus_Canceling, want: ThreadStatusRunning},
		{name: "canceled", status: taskapi.TaskStatus_Canceled, want: ThreadStatusCanceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := TaskToThreadSummary(&taskapi.ChatTask{
				ID:     101,
				Title:  "状态映射",
				Status: tt.status,
			})

			require.Equal(t, tt.want, summary.Status)
		})
	}
}

func TestTaskToThreadSummaryReturnsNilForNilTask(t *testing.T) {
	require.Nil(t, TaskToThreadSummary(nil))
}

func TestDomainThreadToSummaryKeepsTaskWording(t *testing.T) {
	thread := &entity.Thread{
		ID:           200,
		LegacyTaskID: 100,
		SpaceID:      1,
		CreatorID:    2,
		Title:        "生成方案",
		Status:       entity.ThreadStatusRunning,
		Source:       entity.ThreadSourceIM,
		CreatedAt:    1717000000000,
		UpdatedAt:    1717000300000,
	}

	summary := DomainThreadToSummary(thread)

	require.NotNil(t, summary)
	require.Equal(t, int64(200), summary.ThreadID)
	require.Equal(t, int64(100), summary.LegacyTaskID)
	require.Equal(t, int64(1), summary.SpaceID)
	require.Equal(t, int64(2), summary.CreatorID)
	require.Equal(t, "生成方案", summary.Title)
	require.Equal(t, ThreadStatusRunning, summary.Status)
	require.Equal(t, ThreadSourceIM, summary.Source)
	require.Equal(t, int64(1717000000000), summary.CreatedAt)
	require.Equal(t, int64(1717000300000), summary.UpdatedAt)
}

func TestDomainThreadToSummaryReturnsNilForNilThread(t *testing.T) {
	require.Nil(t, DomainThreadToSummary(nil))
}

func TestExtractTaskMessageFallsBackToPlainText(t *testing.T) {
	require.Equal(t, "普通输入", extractTaskMessage(" 普通输入 "))
	require.Equal(t, "", extractTaskMessage("  "))
}
