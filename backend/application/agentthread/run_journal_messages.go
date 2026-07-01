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
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type RunJournalMessageType string

const (
	RunJournalMessageTypeHuman RunJournalMessageType = "human"
	RunJournalMessageTypeAI    RunJournalMessageType = "ai"
	RunJournalMessageTypeTool  RunJournalMessageType = "tool"
)

type RunJournalToolCall struct {
	ID   string
	Name string
	Type string
	Args map[string]any
}

type RunJournalMessage struct {
	ID               string
	ThreadID         int64
	RunID            int64
	Type             RunJournalMessageType
	Role             MessageRole
	Content          string
	Name             string
	ToolCallID       string
	ToolCalls        []RunJournalToolCall
	AdditionalKwargs map[string]any
	Usage            map[string]any
	CreatedAt        int64
	SourceEventID    int64
}

type runJournalSortableMessage struct {
	message *RunJournalMessage
	order   int
}

// ProjectRunJournalMessages normalizes Coze persisted messages and ADK run
// events into the message shape DeerFlow uses before getMessageGroups and
// convertToSteps. It intentionally avoids checkpoint history: checkpoint data
// is runtime state, while this projection is user-visible run journal data.
func ProjectRunJournalMessages(
	run *RunSummary,
	persistedMessages []*MessageSummary,
	events []*RunEventSummary,
) []*RunJournalMessage {
	if run == nil {
		return nil
	}

	items := make([]runJournalSortableMessage, 0, len(persistedMessages)+len(events)+1)
	order := 0
	addItem := func(message *RunJournalMessage) {
		if message == nil {
			return
		}
		items = append(items, runJournalSortableMessage{
			message: message,
			order:   order,
		})
		order++
	}

	hasRunUserMessage := false
	persistedAssistantContents := make(map[string]struct{})
	persistedAssistantMessages := make([]*MessageSummary, 0)
	for _, message := range persistedMessages {
		if message == nil || message.RunID != run.RunID {
			continue
		}

		switch message.Role {
		case MessageRoleUser:
			hasRunUserMessage = true
			addItem(runJournalMessageFromPersisted(message, RunJournalMessageTypeHuman))
		case MessageRoleAssistant:
			persistedAssistantMessages = append(persistedAssistantMessages, message)
			content := strings.TrimSpace(message.Content)
			if content != "" {
				persistedAssistantContents[content] = struct{}{}
			}
		}
	}
	if !hasRunUserMessage {
		addItem(runJournalHumanMessageFromRunInput(run))
	}

	orderedEvents := append([]*RunEventSummary(nil), events...)
	sort.SliceStable(orderedEvents, func(left, right int) bool {
		leftEvent, rightEvent := orderedEvents[left], orderedEvents[right]
		if leftEvent == nil {
			return false
		}
		if rightEvent == nil {
			return true
		}
		if leftEvent.CreatedAt != rightEvent.CreatedAt {
			return leftEvent.CreatedAt < rightEvent.CreatedAt
		}
		return leftEvent.EventID < rightEvent.EventID
	})
	for _, event := range orderedEvents {
		message := runJournalMessageFromRunEvent(event, persistedAssistantContents)
		addItem(message)
	}
	for _, message := range persistedAssistantMessages {
		addItem(runJournalMessageFromPersisted(message, RunJournalMessageTypeAI))
	}

	sort.SliceStable(items, func(left, right int) bool {
		leftMessage, rightMessage := items[left].message, items[right].message
		if leftMessage.CreatedAt != rightMessage.CreatedAt {
			return leftMessage.CreatedAt < rightMessage.CreatedAt
		}
		return items[left].order < items[right].order
	})

	result := make([]*RunJournalMessage, 0, len(items))
	for _, item := range items {
		result = append(result, item.message)
	}
	return result
}

func ProjectThreadRunJournalMessages(
	runs []*RunSummary,
	persistedMessages []*MessageSummary,
	events []*RunEventSummary,
) []*RunJournalMessage {
	runsByID := make(map[int64]*RunSummary, len(runs))
	for _, run := range runs {
		if run == nil || run.RunID <= 0 {
			continue
		}
		runsByID[run.RunID] = run
	}
	if len(runsByID) == 0 {
		return nil
	}

	eventsByRunID := make(map[int64][]*RunEventSummary)
	for _, event := range events {
		if event == nil || event.RunID <= 0 {
			continue
		}
		if _, ok := runsByID[event.RunID]; !ok {
			continue
		}
		eventsByRunID[event.RunID] = append(eventsByRunID[event.RunID], event)
	}

	messagesByRunID := make(map[int64][]*MessageSummary)
	for _, message := range persistedMessages {
		if message == nil || message.RunID <= 0 {
			continue
		}
		if _, ok := runsByID[message.RunID]; !ok {
			continue
		}
		messagesByRunID[message.RunID] = append(messagesByRunID[message.RunID], message)
	}

	items := make([]runJournalSortableMessage, 0, len(persistedMessages)+len(events))
	order := 0
	for _, run := range runs {
		if run == nil || run.RunID <= 0 {
			continue
		}
		messages := ProjectRunJournalMessages(
			run,
			messagesByRunID[run.RunID],
			eventsByRunID[run.RunID],
		)
		for _, message := range messages {
			items = append(items, runJournalSortableMessage{
				message: message,
				order:   order,
			})
			order++
		}
	}

	sort.SliceStable(items, func(left, right int) bool {
		leftMessage, rightMessage := items[left].message, items[right].message
		if leftMessage.CreatedAt != rightMessage.CreatedAt {
			return leftMessage.CreatedAt < rightMessage.CreatedAt
		}
		return items[left].order < items[right].order
	})

	result := make([]*RunJournalMessage, 0, len(items))
	for _, item := range items {
		result = append(result, item.message)
	}
	return result
}

func runJournalMessageFromPersisted(
	message *MessageSummary,
	messageType RunJournalMessageType,
) *RunJournalMessage {
	if message == nil {
		return nil
	}
	content := strings.TrimSpace(message.Content)
	if content == "" {
		return nil
	}

	return &RunJournalMessage{
		ID:        strconv.FormatInt(message.MessageID, 10),
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Type:      messageType,
		Role:      message.Role,
		Content:   content,
		CreatedAt: message.CreatedAt,
	}
}

func runJournalHumanMessageFromRunInput(run *RunSummary) *RunJournalMessage {
	content := latestUserMessageFromRunInput(run.Input)
	if content == "" {
		return nil
	}

	return &RunJournalMessage{
		ID:        fmt.Sprintf("run-%d-input-human", run.RunID),
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		Type:      RunJournalMessageTypeHuman,
		Role:      MessageRoleUser,
		Content:   content,
		CreatedAt: run.CreatedAt,
	}
}

func latestUserMessageFromRunInput(input string) string {
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(input)), &payload); err != nil {
		return ""
	}

	for index := len(payload.Messages) - 1; index >= 0; index-- {
		message := payload.Messages[index]
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role != "user" && role != "human" {
			continue
		}
		if content := strings.TrimSpace(message.Content); content != "" {
			return content
		}
	}
	return ""
}

func runJournalMessageFromRunEvent(
	event *RunEventSummary,
	persistedAssistantContents map[string]struct{},
) *RunJournalMessage {
	if event == nil {
		return nil
	}

	payload := parseRunJournalPayload(event.Payload)
	switch event.EventType {
	case "message.completed":
		return runJournalAIMessageFromEvent(event, payload, persistedAssistantContents)
	case "tool.completed", "tool.failed":
		return runJournalToolMessageFromEvent(event, payload)
	default:
		return nil
	}
}

func runJournalAIMessageFromEvent(
	event *RunEventSummary,
	payload map[string]any,
	persistedAssistantContents map[string]struct{},
) *RunJournalMessage {
	role := strings.ToLower(strings.TrimSpace(runJournalString(payload, "role")))
	if role != "" && role != string(MessageRoleAssistant) && role != "ai" {
		return nil
	}

	content := strings.TrimSpace(
		firstRunJournalString(payload, "content", "message"),
	)
	reasoning := strings.TrimSpace(
		firstRunJournalString(payload, "reasoning_content", "reasoning"),
	)
	toolCalls := runJournalToolCalls(payload["tool_calls"])
	if reasoning == "" && len(toolCalls) == 0 && content == "" {
		return nil
	}
	if reasoning == "" && len(toolCalls) == 0 {
		if _, duplicated := persistedAssistantContents[content]; duplicated {
			return nil
		}
	}

	additionalKwargs := map[string]any{}
	if reasoning != "" {
		additionalKwargs["reasoning_content"] = reasoning
	}
	if len(additionalKwargs) == 0 {
		additionalKwargs = nil
	}

	return &RunJournalMessage{
		ID:               fmt.Sprintf("event-%d", event.EventID),
		ThreadID:         event.ThreadID,
		RunID:            event.RunID,
		Type:             RunJournalMessageTypeAI,
		Role:             MessageRoleAssistant,
		Content:          content,
		ToolCalls:        toolCalls,
		AdditionalKwargs: additionalKwargs,
		Usage:            runJournalObject(payload["usage"]),
		CreatedAt:        event.CreatedAt,
		SourceEventID:    event.EventID,
	}
}

func runJournalToolMessageFromEvent(
	event *RunEventSummary,
	payload map[string]any,
) *RunJournalMessage {
	content := strings.TrimSpace(
		firstRunJournalString(payload, "content", "message", "result"),
	)
	name := strings.TrimSpace(
		firstRunJournalString(payload, "tool_name", "name"),
	)
	toolCallID := strings.TrimSpace(runJournalString(payload, "tool_call_id"))
	if content == "" && name == "" && toolCallID == "" {
		return nil
	}

	return &RunJournalMessage{
		ID:            fmt.Sprintf("event-%d", event.EventID),
		ThreadID:      event.ThreadID,
		RunID:         event.RunID,
		Type:          RunJournalMessageTypeTool,
		Role:          MessageRoleTool,
		Content:       content,
		Name:          name,
		ToolCallID:    toolCallID,
		CreatedAt:     event.CreatedAt,
		SourceEventID: event.EventID,
	}
}

func parseRunJournalPayload(raw string) map[string]any {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return map[string]any{}
	}
	return payload
}

func runJournalToolCalls(raw any) []RunJournalToolCall {
	rawCalls, ok := raw.([]any)
	if !ok || len(rawCalls) == 0 {
		return nil
	}

	calls := make([]RunJournalToolCall, 0, len(rawCalls))
	for _, rawCall := range rawCalls {
		call, ok := rawCall.(map[string]any)
		if !ok {
			continue
		}
		function := runJournalObject(call["function"])
		name := firstRunJournalString(call, "name")
		if name == "" {
			name = runJournalString(function, "name")
		}
		if name == "" {
			continue
		}

		callType := runJournalString(call, "type")
		if callType == "" {
			callType = "function"
		}
		calls = append(calls, RunJournalToolCall{
			ID:   runJournalString(call, "id"),
			Name: name,
			Type: callType,
			Args: runJournalToolCallArgs(call, function),
		})
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func runJournalToolCallArgs(call, function map[string]any) map[string]any {
	for _, raw := range []any{
		call["args"],
		call["arguments"],
		function["arguments"],
	} {
		switch value := raw.(type) {
		case map[string]any:
			return value
		case string:
			parsed := parseRunJournalPayload(value)
			if len(parsed) > 0 {
				return parsed
			}
		}
	}
	return map[string]any{}
}

func runJournalObject(raw any) map[string]any {
	value, ok := raw.(map[string]any)
	if !ok || len(value) == 0 {
		return nil
	}
	return value
}

func firstRunJournalString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := runJournalString(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func runJournalString(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
