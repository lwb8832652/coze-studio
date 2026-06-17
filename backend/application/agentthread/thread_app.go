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
	"encoding/json"
	"strings"

	taskapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/task"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TaskToThreadSummary(task *taskapi.ChatTask) *ThreadSummary {
	if task == nil {
		return nil
	}

	return &ThreadSummary{
		ThreadID:         task.ID,
		LegacyTaskID:     task.ID,
		SpaceID:          task.SpaceID,
		CreatorID:        task.CreatorID,
		Title:            task.Title,
		Status:           taskStatusToThreadStatus(task.Status),
		Source:           ThreadSourceWeb,
		Progress:         task.Progress,
		LastUserMessage:  extractTaskMessage(task.GetInput()),
		LastAgentMessage: extractTaskMessage(task.GetResult()),
		CreatedAt:        task.CreatedAt,
		UpdatedAt:        task.UpdatedAt,
	}
}

func DomainThreadToSummary(thread *entity.Thread) *ThreadSummary {
	if thread == nil {
		return nil
	}

	return &ThreadSummary{
		ThreadID:     thread.ID,
		LegacyTaskID: thread.LegacyTaskID,
		SpaceID:      thread.SpaceID,
		CreatorID:    thread.CreatorID,
		Title:        thread.Title,
		Status:       ThreadStatus(thread.Status),
		Source:       ThreadSource(thread.Source),
		Metadata:     thread.Metadata,
		CreatedAt:    thread.CreatedAt,
		UpdatedAt:    thread.UpdatedAt,
	}
}

func DomainMessageToSummary(message *entity.Message) *MessageSummary {
	if message == nil {
		return nil
	}

	return &MessageSummary{
		MessageID: message.ID,
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Role:      MessageRole(message.Role),
		Content:   message.Content,
		Metadata:  message.Metadata,
		CreatedAt: message.CreatedAt,
	}
}

func DomainRunToSummary(run *entity.Run) *RunSummary {
	if run == nil {
		return nil
	}

	return &RunSummary{
		RunID:             run.ID,
		ThreadID:          run.ThreadID,
		SpaceID:           run.SpaceID,
		CreatorID:         run.CreatorID,
		AssistantID:       run.AssistantID,
		Status:            RunStatus(run.Status),
		Command:           run.Command,
		Input:             run.Input,
		Config:            run.Config,
		Context:           run.Context,
		Metadata:          run.Metadata,
		StreamMode:        run.StreamMode,
		MultitaskStrategy: run.MultitaskStrategy,
		OnDisconnect:      run.OnDisconnect,
		Durability:        run.Durability,
		IdempotencyKey:    run.IdempotencyKey,
		WorkerID:          run.WorkerID,
		ErrorCode:         run.ErrorCode,
		ErrorMessage:      run.ErrorMessage,
		StartedAt:         run.StartedAt,
		EndedAt:           run.EndedAt,
		CreatedAt:         run.CreatedAt,
		UpdatedAt:         run.UpdatedAt,
	}
}

func DomainRunEventToSummary(event *entity.RunEvent) *RunEventSummary {
	if event == nil {
		return nil
	}

	return &RunEventSummary{
		EventID:   event.ID,
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		EventType: event.EventType,
		Payload:   event.Payload,
		CreatedAt: event.CreatedAt,
	}
}

func DomainCheckpointToSummary(checkpoint *entity.Checkpoint) *CheckpointSummary {
	if checkpoint == nil {
		return nil
	}

	return &CheckpointSummary{
		CheckpointID:       checkpoint.ID,
		ThreadID:           checkpoint.ThreadID,
		RunID:              checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       checkpoint.CheckpointNS,
		ChannelValues:      checkpoint.ChannelValues,
		ChannelVersions:    checkpoint.ChannelVersions,
		PendingSends:       checkpoint.PendingSends,
		Metadata:           checkpoint.Metadata,
		CreatedAt:          checkpoint.CreatedAt,
	}
}

func DomainMemoryToSummary(memory *entity.Memory) *MemorySummary {
	if memory == nil {
		return nil
	}

	return &MemorySummary{
		MemoryID:  memory.ID,
		ThreadID:  memory.ThreadID,
		RunID:     memory.RunID,
		SpaceID:   memory.SpaceID,
		Scope:     MemoryScope(memory.Scope),
		Content:   memory.Content,
		Metadata:  memory.Metadata,
		Score:     memory.Score,
		ExpiresAt: memory.ExpiresAt,
		CreatedAt: memory.CreatedAt,
		UpdatedAt: memory.UpdatedAt,
	}
}

func DomainTokenUsageToSummary(usage *entity.TokenUsage) *TokenUsageSummary {
	if usage == nil {
		return nil
	}

	return &TokenUsageSummary{
		UsageID:      usage.ID,
		ThreadID:     usage.ThreadID,
		RunID:        usage.RunID,
		SpaceID:      usage.SpaceID,
		Source:       TokenUsageSource(usage.Source),
		StepID:       usage.StepID,
		StepIndex:    usage.StepIndex,
		StepName:     usage.StepName,
		ModelName:    usage.ModelName,
		Provider:     usage.Provider,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostMicros:   usage.CostMicros,
		Currency:     usage.Currency,
		Estimated:    usage.Estimated,
		RawUsage:     usage.RawUsage,
		Metadata:     usage.Metadata,
		CreatedAt:    usage.CreatedAt,
	}
}

func DomainTokenUsageAggregateToSummary(aggregate *entity.TokenUsageAggregate) *TokenUsageAggregateSummary {
	if aggregate == nil {
		return &TokenUsageAggregateSummary{}
	}

	return &TokenUsageAggregateSummary{
		InputTokens:      aggregate.InputTokens,
		OutputTokens:     aggregate.OutputTokens,
		TotalTokens:      aggregate.TotalTokens,
		CostMicros:       aggregate.CostMicros,
		CallCount:        aggregate.CallCount,
		LeadAgentTokens:  aggregate.LeadAgentTokens,
		SubagentTokens:   aggregate.SubagentTokens,
		MiddlewareTokens: aggregate.MiddlewareTokens,
		ToolTokens:       aggregate.ToolTokens,
	}
}

func taskStatusToThreadStatus(status taskapi.TaskStatus) ThreadStatus {
	switch status {
	case taskapi.TaskStatus_Queued, taskapi.TaskStatus_Running, taskapi.TaskStatus_Canceling:
		return ThreadStatusRunning
	case taskapi.TaskStatus_Succeeded:
		return ThreadStatusCompleted
	case taskapi.TaskStatus_Failed:
		return ThreadStatusFailed
	case taskapi.TaskStatus_Canceled:
		return ThreadStatusCanceled
	default:
		return ThreadStatusIdle
	}
}

func extractTaskMessage(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return text
	}

	for _, key := range []string{"message", "answer", "title"} {
		if value, ok := payload[key].(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}

	return text
}
