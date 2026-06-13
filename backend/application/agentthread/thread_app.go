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
