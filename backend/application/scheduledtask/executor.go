// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/repository"
)

type RunTerminalStatus struct {
	Status       entity.ExecutionStatus
	ErrorCode    string
	ErrorMessage string
}

type AgentRunRequest struct {
	SpaceID     int64
	UserID      int64
	AgentID     int64
	TaskID      int64
	ExecutionID int64
	Message     string
	Variables   map[string]any
}

type AgentRunRef struct {
	ThreadID int64
	RunID    int64
}

type AgentRunner interface {
	StartNew(context.Context, AgentRunRequest) (AgentRunRef, error)
	StartInThread(context.Context, int64, AgentRunRequest) (AgentRunRef, error)
	WaitTerminal(context.Context, int64, int64, int64) (RunTerminalStatus, error)
}

type AgentTaskExecutor struct {
	Runner     AgentRunner
	Repository repository.Repository
}

func (e *AgentTaskExecutor) Execute(ctx context.Context, task *entity.Task, execution *entity.Execution) (repository.ExecutionResult, error) {
	if e == nil || e.Runner == nil || e.Repository == nil {
		return repository.ExecutionResult{}, fmt.Errorf("agent scheduled task executor is unavailable")
	}
	var payload struct {
		Message   string         `json:"message"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return repository.ExecutionResult{}, fmt.Errorf("decode agent task payload: %w", err)
	}
	request := AgentRunRequest{SpaceID: task.SpaceID, UserID: task.CreatorID, AgentID: task.TargetID, TaskID: task.ID, ExecutionID: execution.ID, Message: payload.Message, Variables: payload.Variables}
	var ref AgentRunRef
	var err error
	if task.KeepConversation && task.ConversationID > 0 {
		ref, err = e.Runner.StartInThread(ctx, task.ConversationID, request)
	} else {
		ref, err = e.Runner.StartNew(ctx, request)
		if err == nil && task.KeepConversation {
			if persistErr := e.Repository.SetConversationID(ctx, task.ID, ref.ThreadID); persistErr != nil {
				return repository.ExecutionResult{ThreadID: ref.ThreadID, RunID: ref.RunID}, persistErr
			}
		}
	}
	if err != nil {
		return repository.ExecutionResult{ThreadID: ref.ThreadID, RunID: ref.RunID}, err
	}
	terminal, err := e.Runner.WaitTerminal(ctx, task.CreatorID, ref.ThreadID, ref.RunID)
	result := terminalResult(terminal)
	result.ThreadID = ref.ThreadID
	result.RunID = ref.RunID
	if err != nil {
		return result, err
	}
	if terminal.Status != entity.ExecutionStatusSucceeded {
		return result, fmt.Errorf("agent run ended with status %s", terminal.Status)
	}
	return result, nil
}

type WorkflowRunner interface {
	Start(context.Context, int64, int64, int64, map[string]any) (int64, error)
	WaitTerminal(context.Context, int64, int64) (RunTerminalStatus, error)
}

type WorkflowTaskExecutor struct { Runner WorkflowRunner }

func (e *WorkflowTaskExecutor) Execute(ctx context.Context, task *entity.Task, _ *entity.Execution) (repository.ExecutionResult, error) {
	if e == nil || e.Runner == nil {
		return repository.ExecutionResult{}, fmt.Errorf("workflow scheduled task executor is unavailable")
	}
	inputs := make(map[string]any)
	if err := json.Unmarshal([]byte(task.Payload), &inputs); err != nil {
		return repository.ExecutionResult{}, fmt.Errorf("decode workflow task payload: %w", err)
	}
	executionID, err := e.Runner.Start(ctx, task.SpaceID, task.CreatorID, task.TargetID, inputs)
	if err != nil {
		return repository.ExecutionResult{}, err
	}
	terminal, err := e.Runner.WaitTerminal(ctx, task.TargetID, executionID)
	result := terminalResult(terminal)
	result.WorkflowExecutionID = executionID
	if err != nil {
		return result, err
	}
	if terminal.Status != entity.ExecutionStatusSucceeded {
		return result, fmt.Errorf("workflow execution ended with status %s", terminal.Status)
	}
	return result, nil
}

func terminalResult(status RunTerminalStatus) repository.ExecutionResult {
	result := repository.ExecutionResult{Status: status.Status, ErrorCode: status.ErrorCode}
	if status.Status != entity.ExecutionStatusSucceeded {
		result.ErrorMessage = "任务执行失败，请稍后重试"
	}
	return result
}
