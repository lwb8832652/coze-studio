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

package deerflowparity

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	maxRawEvents       = 4000
	maxRawMessages     = 512
	maxRawTokenEntries = 2048
	maxAssistantBytes  = 16 * 1024 * 1024
)

var safeEventFamilyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,79}$`)

var forbiddenRawKeys = map[string]struct{}{
	"access_token":      {},
	"api_key":           {},
	"authorization":     {},
	"checkpoint":        {},
	"checkpoint_bytes":  {},
	"cookie":            {},
	"credentials":       {},
	"csrf_token":        {},
	"object_uri":        {},
	"password":          {},
	"provider_body":     {},
	"raw_provider_body": {},
	"secret_key":        {},
}

func NormalizeCapture(raw RawCapture) (Observation, error) {
	if raw.Product != ProductDeerFlow && raw.Product != ProductNewX {
		return Observation{}, fmt.Errorf("unsupported product %q", raw.Product)
	}
	if strings.TrimSpace(raw.CaseID) == "" || len(raw.CaseID) > 80 {
		return Observation{}, errors.New("case id is outside bounds")
	}
	if !validMode(raw.Mode) {
		return Observation{}, fmt.Errorf("unsupported mode %q", raw.Mode)
	}
	if len(raw.Events) > maxRawEvents || len(raw.Messages) > maxRawMessages || len(raw.Tokens) > maxRawTokenEntries {
		return Observation{}, errors.New("raw capture exceeds collection bounds")
	}
	if raw.ReconnectDuplicateEvents < 0 || raw.ReconnectDuplicateEvents > maxRawEvents {
		return Observation{}, errors.New("reconnect duplicate count is outside bounds")
	}
	runCount := raw.RunCount
	if runCount == 0 {
		runCount = 1
	}
	if runCount < 0 || runCount > 64 {
		return Observation{}, errors.New("run count is outside bounds")
	}
	if raw.HistoryEntries < 0 || raw.HistoryEntries > 1000 {
		return Observation{}, errors.New("history entry count is outside bounds")
	}
	if err := rejectForbiddenRawValue(raw.State); err != nil {
		return Observation{}, err
	}

	observation := Observation{
		Schema:          ObservationSchemaV1,
		Product:         raw.Product,
		CaseID:          raw.CaseID,
		Mode:            raw.Mode,
		Reconnected:     raw.Reconnected,
		Terminal:        canonicalTerminal(raw.Terminal),
		DuplicateEvents: raw.ReconnectDuplicateEvents,
		RunCount:        runCount,
		StateReloaded:   raw.StateReloaded,
		HistoryEntries:  raw.HistoryEntries,
	}

	seenEventIDs := make(map[string]struct{}, len(raw.Events))
	cancelled := false
	authoritativeCancel := raw.CancelRequested && observation.Terminal == "cancelled"
	for _, event := range raw.Events {
		if err := rejectForbiddenRawValue(event.Payload); err != nil {
			return Observation{}, err
		}
		if event.ID != "" {
			if _, ok := seenEventIDs[event.ID]; ok {
				observation.DuplicateEvents++
				continue
			}
			seenEventIDs[event.ID] = struct{}{}
		}
		family, err := canonicalEventFamily(event)
		if err != nil {
			return Observation{}, err
		}
		if family == "" {
			continue
		}
		observation.EventFamilies = append(observation.EventFamilies, family)
		switch family {
		case "subagent.started":
			observation.ChildRuns++
		case "run.interrupted":
			observation.InterruptPresent = true
			if !authoritativeCancel {
				observation.TerminalEvents++
			}
		case "run.resumed":
			observation.ResumeObserved = true
		case "clarification.requested":
			observation.ClarificationPresent = true
		case "followup.started":
			observation.FollowUpObserved = true
		case "run.cancelled":
			cancelled = true
			observation.TerminalEvents++
		case "run.failed":
			if !authoritativeCancel {
				observation.TerminalEvents++
			}
		case "run.completed":
			observation.TerminalEvents++
			if cancelled {
				observation.SuccessAfterCancel = true
			}
		}
	}

	for _, message := range raw.Messages {
		if message.ReasoningPresent {
			observation.Capabilities.Thinking = true
		}
		if !strings.EqualFold(strings.TrimSpace(message.Role), "assistant") || strings.TrimSpace(message.Content) == "" {
			continue
		}
		observation.AssistantMessagePresent = true
		observation.AssistantMessageBytes += len(message.Content)
		if observation.AssistantMessageBytes > maxAssistantBytes {
			return Observation{}, errors.New("assistant message bytes exceed bounds")
		}
	}
	if raw.CancelRequested && (observation.AssistantMessagePresent || observation.Terminal == "success") {
		observation.SuccessAfterCancel = true
	}
	observation.Todo = normalizeTodo(raw.State["todos"])
	observation.Capabilities.Plan = observation.Todo.Total > 0 || slices.Contains(observation.EventFamilies, "todo.updated")
	observation.Capabilities.Subagent = observation.ChildRuns > 0
	for _, usage := range raw.Tokens {
		if usage.Input < 0 || usage.Output < 0 || usage.Total < 0 {
			return Observation{}, errors.New("token usage must be non-negative")
		}
		observation.Token.Input += usage.Input
		observation.Token.Output += usage.Output
		if usage.Total > 0 {
			observation.Token.Total += usage.Total
		} else {
			observation.Token.Total += usage.Input + usage.Output
		}
	}
	return observation, nil
}

func rejectForbiddenRawValue(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if _, forbidden := forbiddenRawKeys[normalized]; forbidden {
				return fmt.Errorf("raw capture contains forbidden field %q", normalized)
			}
			if err := rejectForbiddenRawValue(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectForbiddenRawValue(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonicalEventFamily(event RawEvent) (string, error) {
	rawType := strings.ToLower(strings.TrimSpace(event.Type))
	if payloadType, ok := event.Payload["event_type"].(string); ok && strings.TrimSpace(payloadType) != "" {
		rawType = strings.ToLower(strings.TrimSpace(payloadType))
	}
	kind, _ := event.Payload["kind"].(string)
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "subagent" {
		switch rawType {
		case "task.started", "run.started", "run.start", "child.started", "subagent.run.started":
			return "subagent.started", nil
		case "task.completed", "run.completed", "run.succeeded", "run.end", "child.completed", "subagent.run.completed", "llm.ai.response":
			return "subagent.completed", nil
		}
	}

	switch rawType {
	case "run.created", "run.started", "run.start", "task.started":
		return "run.started", nil
	case "assistant.completed", "message.completed", "assistant.message.completed":
		return "assistant.completed", nil
	case "llm.ai.response":
		if present, _ := event.Payload["assistant_content_present"].(bool); present {
			return "assistant.completed", nil
		}
		return "runtime.diagnostic", nil
	case "token.usage", "usage.updated", "token_usage.updated":
		return "token.usage", nil
	case "run.completed", "run.succeeded", "run.success", "run.end":
		return "run.completed", nil
	case "todo.updated", "write_todos", "tool.write_todos.completed":
		return "todo.updated", nil
	case "llm.tool.result":
		toolName := strings.TrimSpace(stringValue(event.Payload["tool_name"]))
		if strings.EqualFold(toolName, "write_todos") {
			return "todo.updated", nil
		}
		if strings.EqualFold(toolName, "ask_clarification") {
			return "clarification.requested", nil
		}
		return "runtime.diagnostic", nil
	case "clarification.requested":
		return "clarification.requested", nil
	case "followup.started":
		return "followup.started", nil
	case "followup.completed":
		return "followup.completed", nil
	case "subagent.started", "child.started", "subagent.run.started":
		return "subagent.started", nil
	case "subagent.completed", "child.completed", "subagent.run.completed":
		return "subagent.completed", nil
	case "run.interrupted", "interrupt.created":
		return "run.interrupted", nil
	case "run.resumed", "resume.accepted":
		return "run.resumed", nil
	case "run.cancelled", "run.canceled", "subagent.run.canceled":
		return "run.cancelled", nil
	case "run.failed", "run.error":
		return "run.failed", nil
	case "":
		return "", nil
	default:
		if !safeEventFamilyPattern.MatchString(rawType) {
			return "", fmt.Errorf("event family %q is invalid", rawType)
		}
		return "runtime.diagnostic", nil
	}
}

func canonicalTerminal(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "success", "succeeded", "completed":
		return "success"
	case "cancelled", "canceled":
		return "cancelled"
	case "failed", "error":
		return "failed"
	case "interrupted", "paused":
		return "interrupted"
	case "":
		return ""
	default:
		return "unknown"
	}
}

func normalizeTodo(value any) TodoObservation {
	items, ok := value.([]any)
	if !ok {
		return TodoObservation{}
	}
	result := TodoObservation{Total: len(items)}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			result.Other++
			continue
		}
		status, _ := entry["status"].(string)
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "pending":
			result.Pending++
		case "in_progress", "in-progress":
			result.InProgress++
		case "completed", "done":
			result.Completed++
		default:
			result.Other++
		}
	}
	return result
}
