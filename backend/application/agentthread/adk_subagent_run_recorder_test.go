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
	"context"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/stretchr/testify/require"
)

func TestApplicationADKSubagentRunRecorderStartsChildRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdRun: &entity.Run{
			ID:          2001,
			ThreadID:    10,
			ParentRunID: 20,
			AssistantID: "singleagent:1001",
			RunKind:     entity.RunKindSubagent,
			Status:      entity.RunStatusRunning,
		},
	}
	policy := RuntimePolicy{DefaultMode: RuntimeModeEinoADK, EinoADKEnabled: true}
	recorder := NewApplicationADKSubagentRunRecorder(
		&ApplicationService{ThreadSVC: domainSVC, RuntimePolicy: &policy},
	)

	child, err := recorder.StartADKSubagentRun(
		context.Background(),
		ADKSubagentRunStartRequest{
			Parent: &RunSummary{
				RunID:      20,
				ThreadID:   10,
				StreamMode: `["messages"]`,
				Durability: "async",
				Config:     `{"agent_name":"lead"}`,
			},
			ArgumentsInJSON: `{"request":"find context"}`,
			Definition: ADKSubagentDefinition{
				Name:                   "researcher",
				Description:            "Research public information.",
				AgentID:                1001,
				Version:                "v1",
				FullChatHistoryAsInput: true,
				AllowedTools:           []string{"knowledge_lookup"},
				AllowedDynamicTools:    []string{"web_search"},
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2001), child.RunID)
	require.Equal(t, int64(10), domainSVC.createRunReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.createRunReq.ParentRunID)
	require.Equal(t, "singleagent:1001", domainSVC.createRunReq.AssistantID)
	require.Equal(t, entity.RunKindSubagent, domainSVC.createRunReq.RunKind)
	require.Equal(t, entity.RunStatusRunning, domainSVC.createRunReq.Status)
	require.JSONEq(t, `{
		"schema":"coze.subagent_tool_call.v1",
		"tool_name":"researcher",
		"arguments":{"request":"find context"}
	}`, domainSVC.createRunReq.Input)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"requested_policy":"pro",
		"mode":"pro",
		"thinking_enabled":false,
		"reasoning_effort":"medium",
		"is_plan_mode":false,
		"subagent_enabled":false,
		"agent_name":"researcher",
		"agent_description":"Research public information.",
		"single_agent":{"agent_id":1001,"version":"v1","is_draft":false},
		"full_chat_history":true,
		"tool_policy":{
			"allowed_tools":["knowledge_lookup"],
			"allowed_dynamic_tools":["web_search"]
		}
	}`, domainSVC.createRunReq.Config)
	require.Contains(t, domainSVC.createRunReq.Metadata, `"source":"eino_adk_subagent"`)
	require.Contains(t, domainSVC.createRunReq.Metadata, `"parent_run_id":20`)
	require.Contains(t, domainSVC.createRunReq.Metadata, `"name":"researcher"`)
}

func TestApplicationADKSubagentRunRecorderFinishesChildRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		completedRun: &entity.Run{
			ID:     2001,
			Status: entity.RunStatusSucceeded,
		},
		failedRun: &entity.Run{
			ID:           2002,
			Status:       entity.RunStatusFailed,
			ErrorMessage: "tool failed",
		},
	}
	recorder := NewApplicationADKSubagentRunRecorder(
		&ApplicationService{ThreadSVC: domainSVC},
	)

	completed, err := recorder.FinishADKSubagentRun(
		context.Background(),
		ADKSubagentRunFinishRequest{
			Child:  &RunSummary{RunID: 2001},
			Status: RunStatusSucceeded,
		},
	)

	require.NoError(t, err)
	require.Equal(t, RunStatusSucceeded, completed.Status)
	require.Equal(t, int64(2001), domainSVC.completeRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.completeRunReq.From)

	failed, err := recorder.FinishADKSubagentRun(
		context.Background(),
		ADKSubagentRunFinishRequest{
			Child:        &RunSummary{RunID: 2002},
			Status:       RunStatusFailed,
			ErrorCode:    "subagent_timeout",
			ErrorMessage: "[NodeRunError] tool failed\nnode path: [node_1]",
		},
	)

	require.NoError(t, err)
	require.Equal(t, RunStatusFailed, failed.Status)
	require.Equal(t, int64(2002), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "subagent_timeout", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "tool failed", domainSVC.failRunReq.ErrorMessage)
}

func TestApplicationADKSubagentRunRecorderCancelsChildRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		canceledRun: &entity.Run{
			ID:     2001,
			Status: entity.RunStatusCanceled,
		},
	}
	recorder := NewApplicationADKSubagentRunRecorder(
		&ApplicationService{ThreadSVC: domainSVC},
	)

	canceled, err := recorder.FinishADKSubagentRun(
		context.Background(),
		ADKSubagentRunFinishRequest{
			Child:        &RunSummary{RunID: 2001},
			Status:       RunStatusCanceled,
			ErrorCode:    "subagent_canceled",
			ErrorMessage: "context canceled",
		},
	)

	require.NoError(t, err)
	require.Equal(t, RunStatusCanceled, canceled.Status)
	require.Equal(t, int64(2001), domainSVC.cancelRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.cancelRunReq.From)
	require.Equal(t, "subagent_canceled", domainSVC.cancelRunReq.ErrorCode)
	require.Equal(t, "context canceled", domainSVC.cancelRunReq.ErrorMessage)
}
