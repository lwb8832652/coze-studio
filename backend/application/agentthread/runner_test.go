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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestRunProcessorCompletesClaimedRunWithAssistantMessage(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:        200,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Input:     `{"messages":[{"role":"user","content":"分析客户反馈"}]}`,
				WorkerID:  "worker-a",
				StartedAt: 300,
				UpdatedAt: 301,
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "客户反馈已完成分析",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
			EndedAt:  400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	var executedRun *RunSummary
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		executedRun = run

		return &RunExecutionResult{
			Message:  "客户反馈已完成分析",
			Metadata: `{"source":"agent_harness"}`,
		}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
	})

	err := processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Equal(t, "worker-a", domainSVC.claimRunsReq.WorkerID)
	require.Equal(t, int32(1), domainSVC.claimRunsReq.Limit)
	require.NotNil(t, executedRun)
	require.Equal(t, int64(200), executedRun.RunID)
	require.Equal(t, int64(200), domainSVC.appendReq.RunID)
	require.Equal(t, entity.MessageRoleAssistant, domainSVC.appendReq.Role)
	require.Equal(t, "客户反馈已完成分析", domainSVC.appendReq.Content)
	require.Equal(t, `{"source":"agent_harness"}`, domainSVC.appendReq.Metadata)
	require.Equal(t, int64(200), domainSVC.completeRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.completeRunReq.From)
	require.Equal(t, "worker-a", domainSVC.completeRunReq.WorkerID)
	require.Nil(t, domainSVC.failRunReq)
}

func TestRunProcessorMarksRunFailedWhenExecutorErrors(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				WorkerID: "worker-a",
			},
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "worker-a",
			ErrorCode:    "executor_error",
			ErrorMessage: "model failed",
			EndedAt:      400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return nil, fmt.Errorf("model failed")
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
	})

	err := processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Nil(t, domainSVC.appendReq)
	require.Nil(t, domainSVC.completeRunReq)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, int64(200), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "worker-a", domainSVC.failRunReq.WorkerID)
	require.Equal(t, "executor_error", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "model failed", domainSVC.failRunReq.ErrorMessage)
}
