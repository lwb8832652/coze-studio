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

package workbench

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	appskill "github.com/coze-dev/coze-studio/backend/application/skill"
	apptask "github.com/coze-dev/coze-studio/backend/application/task"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMessageRejectsBlankMessageAsClientError(t *testing.T) {
	_, err := SVC.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: " \t\n ",
		Mode:    chatapi.ChatMode_Auto,
	})

	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestHandleMessageRejectsInvalidModeAsClientError(t *testing.T) {
	_, err := SVC.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: "hello",
		Mode:    chatapi.ChatMode(99),
	})

	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestWorkbenchChatContractHasTaskAndResourceFields(t *testing.T) {
	taskID := int64(100)
	modelType := int64(100002)
	modelName := "deepseek-v4-pro"
	req := &chatapi.WorkbenchChatRequest{
		SpaceID:         1,
		Message:         "hello",
		Mode:            chatapi.ChatMode_Ask,
		TaskID:          &taskID,
		EnableSkills:    []string{"skill-a"},
		EnableMcp:       []string{"mcp-a"},
		EnableKbs:       []string{"kb-a"},
		EnableDatabases: []string{"db-a"},
		ModelType:       &modelType,
		ModelName:       &modelName,
	}

	require.True(t, req.IsSetTaskID())
	require.True(t, req.IsSetEnableSkills())
	require.True(t, req.IsSetEnableMcp())
	require.True(t, req.IsSetEnableKbs())
	require.True(t, req.IsSetEnableDatabases())
	require.True(t, req.IsSetModelType())
	require.True(t, req.IsSetModelName())
	require.Equal(t, int64(100), req.GetTaskID())
	require.Equal(t, []string{"skill-a"}, req.GetEnableSkills())
	require.Equal(t, []string{"mcp-a"}, req.GetEnableMcp())
	require.Equal(t, []string{"kb-a"}, req.GetEnableKbs())
	require.Equal(t, []string{"db-a"}, req.GetEnableDatabases())
	require.Equal(t, int64(100002), req.GetModelType())
	require.Equal(t, "deepseek-v4-pro", req.GetModelName())

	emptyReq := &chatapi.WorkbenchChatRequest{}
	require.False(t, emptyReq.IsSetEnableSkills())
	require.Nil(t, emptyReq.GetEnableSkills())

	resultType := "answer"
	executionType := "Ark"
	data := &chatapi.WorkbenchChatData{
		RouteTarget:   chatapi.RouteTarget_TaskEngine,
		ResultType:    &resultType,
		ExecutionType: &executionType,
	}

	require.Equal(t, "answer", data.GetResultType())
	require.Equal(t, "Ark", data.GetExecutionType())
}

func TestHandleMessageAutoCreatesTaskAndCompletesAnswer(t *testing.T) {
	taskApp := &recordingWorkbenchTaskApp{
		created: &taskapi.ChatTask{ID: 10, SpaceID: 1, Title: "hello", Status: taskapi.TaskStatus_Running},
	}
	var scheduled []func()
	app := &ApplicationService{
		taskApp: taskApp,
		runAsync: func(fn func()) {
			scheduled = append(scheduled, fn)
		},
		chatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 1)
					require.Equal(t, "hello", in[0].Content)
					return schema.AssistantMessage("auto answer", nil), nil
				},
			}, true, nil
		},
	}

	resp, err := app.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: "hello",
		Mode:    chatapi.ChatMode_Auto,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.NotNil(t, resp.Data.Task)
	require.Equal(t, int64(10), resp.Data.Task.ID)
	require.Equal(t, chatapi.RouteTarget_TaskEngine, resp.Data.RouteTarget)
	require.Equal(t, "answer", resp.Data.GetResultType())
	require.Equal(t, "Ark", resp.Data.GetExecutionType())
	require.Nil(t, resp.Data.Answer)
	require.Empty(t, taskApp.completedResult)
	require.Empty(t, taskApp.events)
	require.Len(t, scheduled, 1)

	scheduled[0]()

	require.Contains(t, taskApp.completedResult, `"message":"auto answer"`)
	require.Equal(t, "answer.completed", taskApp.events[len(taskApp.events)-1].eventType)
}

func TestHandleMessageWithTaskIDAppendsUserMessageWithoutCreatingTask(t *testing.T) {
	taskID := int64(20)
	taskApp := &recordingWorkbenchTaskApp{
		got: &taskapi.ChatTask{ID: 20, SpaceID: 1, Title: "existing", Status: taskapi.TaskStatus_Running},
	}
	var scheduled []func()
	app := &ApplicationService{
		taskApp: taskApp,
		runAsync: func(fn func()) {
			scheduled = append(scheduled, fn)
		},
		chatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 1)
					require.Equal(t, "follow up", in[0].Content)
					return schema.AssistantMessage("follow answer", nil), nil
				},
			}, true, nil
		},
	}

	resp, err := app.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		TaskID:  &taskID,
		Message: "follow up",
		Mode:    chatapi.ChatMode_Ask,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Data)
	require.NotNil(t, resp.Data.Task)
	require.Equal(t, int64(20), resp.Data.Task.ID)
	require.Equal(t, 0, taskApp.createCalls)
	require.Len(t, taskApp.events, 1)
	require.Equal(t, "user.message", taskApp.events[0].eventType)
	require.Empty(t, taskApp.completedResult)
	require.Len(t, scheduled, 1)

	scheduled[0]()

	require.GreaterOrEqual(t, len(taskApp.events), 2)
	require.Equal(t, "answer.completed", taskApp.events[len(taskApp.events)-1].eventType)
	require.Contains(t, taskApp.completedResult, `"message":"follow answer"`)
}

func TestHandleMessageWithTaskIDRejectsTaskFromDifferentSpace(t *testing.T) {
	taskID := int64(20)
	taskApp := &recordingWorkbenchTaskApp{
		got: &taskapi.ChatTask{ID: 20, SpaceID: 2, Title: "existing", Status: taskapi.TaskStatus_Running},
	}
	app := &ApplicationService{taskApp: taskApp}

	resp, err := app.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		TaskID:  &taskID,
		Message: "follow up",
		Mode:    chatapi.ChatMode_Ask,
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.Nil(t, resp)
	require.Empty(t, taskApp.events)
}

func TestHandleMessageAgentFailsTaskWhenRuntimeContextMissing(t *testing.T) {
	taskApp := &recordingWorkbenchTaskApp{
		created: &taskapi.ChatTask{ID: 30, SpaceID: 1, Title: "do work", Status: taskapi.TaskStatus_Running},
	}
	var scheduled []func()
	app := &ApplicationService{
		taskApp:     taskApp,
		agentRunSVC: &fakeAgentRun{},
		runAsync: func(fn func()) {
			scheduled = append(scheduled, fn)
		},
	}

	resp, err := app.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: "do work",
		Mode:    chatapi.ChatMode_Agent,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(30), resp.Data.Task.ID)
	require.Zero(t, taskApp.failedID)
	require.Len(t, scheduled, 1)

	scheduled[0]()

	require.Equal(t, int64(30), taskApp.failedID)
	require.Contains(t, taskApp.failError, "agent_id is required")
	require.Equal(t, "turn.failed", taskApp.events[len(taskApp.events)-1].eventType)
}

func TestInitServiceMergesRuntimeComponents(t *testing.T) {
	prevSkillSVC := SVC.skillSVC
	prevTaskSVC := SVC.taskSVC
	prevKnowledgeSVC := SVC.knowledgeSVC
	prevAgentRunSVC := SVC.agentRunSVC
	prevChatModelProvider := SVC.chatModelProvider
	prevTaskApp := SVC.taskApp
	t.Cleanup(func() {
		SVC.skillSVC = prevSkillSVC
		SVC.taskSVC = prevTaskSVC
		SVC.knowledgeSVC = prevKnowledgeSVC
		SVC.agentRunSVC = prevAgentRunSVC
		SVC.chatModelProvider = prevChatModelProvider
		SVC.taskApp = prevTaskApp
	})

	skillSVC := &appskill.ApplicationService{}
	taskSVC := &apptask.ApplicationService{}
	InitService(&ServiceComponents{SkillSVC: skillSVC, TaskSVC: taskSVC})
	InitService(&ServiceComponents{ChatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return nil, false, nil
	}})

	require.Same(t, skillSVC, SVC.skillSVC)
	require.Same(t, taskSVC, SVC.taskSVC)
	require.NotNil(t, SVC.chatModelProvider)
}
