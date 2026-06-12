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
		taskID:          10,
		spaceID:         1,
		message:         "do work",
		enableDatabases: []string{"db-a"},
	})

	require.NoError(t, err)
	require.Equal(t, resultTypeAgentTrace, result.ResultType)
	require.Equal(t, executionTypeAgent, result.ExecutionType)
	require.Equal(t, "hello world", result.Message)
	require.NotNil(t, agentRun.req)
	require.Equal(t, "db-a", agentRun.req.Ext["enable_databases"])
	require.GreaterOrEqual(t, len(taskSVC.events), 3)
	require.Equal(t, "agent.run_started", taskSVC.events[0].eventType)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(taskSVC.events[len(taskSVC.events)-1].payload), &payload))
	require.Equal(t, "hello world", payload["message"])
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
