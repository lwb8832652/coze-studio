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
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	agentrunmodel "github.com/coze-dev/coze-studio/backend/crossdomain/agentrun/model"
	crossmessage "github.com/coze-dev/coze-studio/backend/crossdomain/message/model"
	agentrunentity "github.com/coze-dev/coze-studio/backend/domain/conversation/agentrun/entity"
)

func TestRunAgentAppendsStreamEventsAndFinalResult(t *testing.T) {
	stream := newAgentRunStream([]*agentrunentity.AgentRunResponse{
		{
			Event: agentrunentity.RunEventCreated,
			ChunkRunItem: &agentrunentity.ChunkRunItem{
				ID: 77,
			},
		},
		{
			Event: agentrunentity.RunEventMessageDelta,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: "answer",
				Content:     "hello ",
			},
		},
		{
			Event: agentrunentity.RunEventMessageCompleted,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: "answer",
				Content:     "hello world",
				IsFinish:    true,
			},
		},
		{Event: agentrunentity.RunEventStreamDone},
	})
	taskSVC := &recordingWorkbenchTaskApp{}
	agentRun := &fakeAgentRun{stream: stream}
	app := &ApplicationService{
		taskApp:     taskSVC,
		agentRunSVC: agentRun,
	}

	result, err := app.runAgent(context.Background(), agentRequest{
		taskID:         10,
		spaceID:        1,
		conversationID: 20,
		sectionID:      30,
		agentID:        40,
		userID:         "50",
		cozeUID:        50,
		message:        "do work",
		preRetrieveTools: []*agentrunmodel.Tool{
			{
				PluginID: 100,
				ToolID:   200,
				ToolName: "search",
				Type:     agentrunmodel.ToolTypePlugin,
			},
		},
		customVariables: map[string]string{"city": "Shanghai"},
		enableDatabases: []string{"db-a"},
	})

	require.NoError(t, err)
	require.Equal(t, resultTypeAgentTrace, result.ResultType)
	require.Equal(t, executionTypeAgent, result.ExecutionType)
	require.Equal(t, "hello world", result.Message)
	require.NotNil(t, agentRun.req)
	require.Equal(t, int64(20), agentRun.req.ConversationID)
	require.Equal(t, int64(30), agentRun.req.SectionID)
	require.Equal(t, int64(40), agentRun.req.AgentID)
	require.Equal(t, "50", agentRun.req.UserID)
	require.Equal(t, int64(50), agentRun.req.CozeUID)
	require.Len(t, agentRun.req.PreRetrieveTools, 1)
	require.Equal(t, int64(100), agentRun.req.PreRetrieveTools[0].PluginID)
	require.Equal(t, "Shanghai", agentRun.req.CustomVariables["city"])
	require.Equal(t, "db-a", agentRun.req.Ext["enable_databases"])
	require.GreaterOrEqual(t, len(taskSVC.events), 3)
	require.Equal(t, "agent.run_started", taskSVC.events[0].eventType)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(taskSVC.events[len(taskSVC.events)-1].payload), &payload))
	require.Equal(t, "hello world", payload["message"])
}

func TestRunAgentKeepsAnswerWhenVerboseFinishArrivesLater(t *testing.T) {
	stream := newAgentRunStream([]*agentrunentity.AgentRunResponse{
		{
			Event: agentrunentity.RunEventMessageCompleted,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: crossmessage.MessageTypeAnswer,
				Content:     "real answer",
				IsFinish:    true,
			},
		},
		{
			Event: agentrunentity.RunEventMessageCompleted,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: crossmessage.MessageTypeVerbose,
				Content:     `{"msg_type":"generate_answer_finish"}`,
				IsFinish:    true,
			},
		},
		{
			Event: agentrunentity.RunEventMessageCompleted,
			ChunkMessageItem: &agentrunentity.ChunkMessageItem{
				MessageType: crossmessage.MessageTypeFlowUp,
				Content:     "follow up suggestion",
				IsFinish:    true,
			},
		},
		{Event: agentrunentity.RunEventCompleted},
		{Event: agentrunentity.RunEventStreamDone},
	})
	taskSVC := &recordingWorkbenchTaskApp{}
	app := &ApplicationService{
		taskApp:     taskSVC,
		agentRunSVC: &fakeAgentRun{stream: stream},
	}

	result, err := app.runAgent(context.Background(), agentRequest{
		taskID:         10,
		spaceID:        1,
		conversationID: 20,
		sectionID:      30,
		agentID:        40,
		userID:         "50",
		cozeUID:        50,
		message:        "do work",
	})

	require.NoError(t, err)
	require.Equal(t, "real answer", result.Message)
	require.Len(t, taskSVC.events, 1)
	require.Equal(t, "agent.run_completed", taskSVC.events[0].eventType)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(taskSVC.events[0].payload), &payload))
	require.Equal(t, "real answer", payload["message"])
	require.Equal(t, "answer", payload["message_type"])
}

func TestRunAgentReturnsErrorOnStreamErrorEvent(t *testing.T) {
	stream := newAgentRunStream([]*agentrunentity.AgentRunResponse{
		{
			Event: agentrunentity.RunEventError,
			Error: &agentrunentity.RunError{
				Code: 500,
				Msg:  "agent failed",
			},
		},
		{Event: agentrunentity.RunEventStreamDone},
	})
	taskSVC := &recordingWorkbenchTaskApp{}
	app := &ApplicationService{
		taskApp:     taskSVC,
		agentRunSVC: &fakeAgentRun{stream: stream},
	}

	_, err := app.runAgent(context.Background(), agentRequest{
		taskID:         10,
		spaceID:        1,
		conversationID: 20,
		sectionID:      30,
		agentID:        40,
		userID:         "50",
		cozeUID:        50,
		message:        "do work",
	})

	require.ErrorContains(t, err, "agent failed")
	require.Len(t, taskSVC.events, 1)
	require.Equal(t, "agent.run_failed", taskSVC.events[0].eventType)
}

func TestRunAgentRequiresRunnableAgentContext(t *testing.T) {
	app := &ApplicationService{agentRunSVC: &fakeAgentRun{}}

	_, err := app.runAgent(context.Background(), agentRequest{
		taskID:         10,
		spaceID:        1,
		conversationID: 20,
		message:        "do work",
	})

	require.ErrorContains(t, err, "agent_id is required")
}

func newAgentRunStream(chunks []*agentrunentity.AgentRunResponse) *schema.StreamReader[*agentrunentity.AgentRunResponse] {
	sr, sw := schema.Pipe[*agentrunentity.AgentRunResponse](10)
	go func() {
		defer sw.Close()
		for _, chunk := range chunks {
			sw.Send(chunk, nil)
		}
	}()
	return sr
}
