// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
)

func TestAgentTaskExecutorCreatesAndReusesConversation(t *testing.T) {
	t.Parallel()

	t.Run("creates dedicated conversation", func(t *testing.T) {
		runner := &recordingAgentRunner{startRef: AgentRunRef{ThreadID: 30, RunID: 40}, status: RunTerminalStatus{Status: entity.ExecutionStatusSucceeded}}
		repo := &conversationRepository{}
		executor := &AgentTaskExecutor{Runner: runner, Repository: repo}
		task := &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, TargetID: 200, KeepConversation: true, Payload: `{"message":"生成日报","variables":{"city":"武汉"}}`}

		result, err := executor.Execute(context.Background(), task, &entity.Execution{ID: 20})

		require.NoError(t, err)
		require.Equal(t, int64(30), result.ThreadID)
		require.Equal(t, int64(40), result.RunID)
		require.Equal(t, int64(30), repo.threadID)
		require.Equal(t, "武汉", runner.newRequest.Variables["city"])
	})

	t.Run("reuses dedicated conversation", func(t *testing.T) {
		runner := &recordingAgentRunner{startRef: AgentRunRef{ThreadID: 30, RunID: 41}, status: RunTerminalStatus{Status: entity.ExecutionStatusSucceeded}}
		executor := &AgentTaskExecutor{Runner: runner, Repository: &conversationRepository{}}
		task := &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, TargetID: 200, KeepConversation: true, ConversationID: 30, Payload: `{"message":"继续生成日报","variables":{}}`}

		result, err := executor.Execute(context.Background(), task, &entity.Execution{ID: 21})

		require.NoError(t, err)
		require.Nil(t, runner.newRequest)
		require.Equal(t, int64(30), runner.existingThreadID)
		require.Equal(t, int64(41), result.RunID)
	})
}

func TestAgentTaskExecutorPropagatesTerminalFailureWithoutRawDetails(t *testing.T) {
	t.Parallel()
	runner := &recordingAgentRunner{
		startRef: AgentRunRef{ThreadID: 30, RunID: 40},
		status: RunTerminalStatus{Status: entity.ExecutionStatusFailed, ErrorCode: "model_failed", ErrorMessage: "raw provider body"},
	}
	executor := &AgentTaskExecutor{Runner: runner, Repository: &conversationRepository{}}

	result, err := executor.Execute(context.Background(), &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, TargetID: 200, Payload: `{"message":"hello"}`}, &entity.Execution{ID: 20})

	require.Error(t, err)
	require.Equal(t, int64(30), result.ThreadID)
	require.Equal(t, int64(40), result.RunID)
	require.NotContains(t, result.ErrorMessage, "raw provider body")
}

func TestWorkflowTaskExecutorRunsPublishedWorkflowToTerminal(t *testing.T) {
	t.Parallel()
	runner := &recordingWorkflowRunner{
		executionID: 50,
		status: RunTerminalStatus{Status: entity.ExecutionStatusSucceeded},
	}
	executor := &WorkflowTaskExecutor{Runner: runner}
	task := &entity.Task{ID: 10, SpaceID: 1, CreatorID: 7, TargetID: 300, Payload: `{"city":"武汉"}`}

	result, err := executor.Execute(context.Background(), task, &entity.Execution{ID: 20})

	require.NoError(t, err)
	require.Equal(t, int64(50), result.WorkflowExecutionID)
	require.Equal(t, "武汉", runner.inputs["city"])
}

type recordingAgentRunner struct {
	newRequest       *AgentRunRequest
	existingThreadID int64
	startRef         AgentRunRef
	status           RunTerminalStatus
}

func (r *recordingAgentRunner) StartNew(_ context.Context, req AgentRunRequest) (AgentRunRef, error) {
	r.newRequest = &req
	return r.startRef, nil
}

func (r *recordingAgentRunner) StartInThread(_ context.Context, threadID int64, _ AgentRunRequest) (AgentRunRef, error) {
	r.existingThreadID = threadID
	return r.startRef, nil
}

func (r *recordingAgentRunner) WaitTerminal(context.Context, int64, int64, int64) (RunTerminalStatus, error) {
	return r.status, nil
}

type conversationRepository struct {
	recordingRepository
	threadID int64
}

func (r *conversationRepository) SetConversationID(_ context.Context, _ int64, threadID int64) error {
	r.threadID = threadID
	return nil
}

type recordingWorkflowRunner struct {
	executionID int64
	inputs      map[string]any
	status      RunTerminalStatus
}

func (r *recordingWorkflowRunner) Start(_ context.Context, _ int64, _ int64, _ int64, inputs map[string]any) (int64, error) {
	r.inputs = inputs
	return r.executionID, nil
}

func (r *recordingWorkflowRunner) WaitTerminal(context.Context, int64, int64) (RunTerminalStatus, error) {
	return r.status, nil
}
