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
	"encoding/json"
	"strings"
)

const (
	adkToolErrorSchema            = "coze.tool_error.v1"
	adkToolRepairSchema           = "coze.tool_repair.v1"
	adkToolErrorMessageMaxLength  = 2048
	adkToolErrorDefaultMessage    = "tool call failed"
	adkToolRepairReasonMissing    = "missing_tool_result"
	adkToolRepairModelInstruction = "Tool result was missing and has been patched by Coze runtime. Continue with available context or retry the tool if needed."
)

type ADKToolErrorPayload struct {
	Schema       string `json:"schema"`
	Status       string `json:"status"`
	ToolName     string `json:"tool_name,omitempty"`
	ToolCallID   string `json:"tool_call_id,omitempty"`
	ErrorMessage string `json:"error_message"`
	Recoverable  bool   `json:"recoverable"`
	Normalized   bool   `json:"normalized"`
}

type ADKToolRepairPayload struct {
	Schema     string `json:"schema"`
	Status     string `json:"status"`
	ToolName   string `json:"tool_name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	Reason     string `json:"reason"`
	Message    string `json:"message"`
}

func newADKToolErrorPayload(
	toolName string,
	toolCallID string,
	err error,
) ADKToolErrorPayload {
	message := ""
	if err != nil {
		message = err.Error()
	}

	return ADKToolErrorPayload{
		Schema:       adkToolErrorSchema,
		Status:       "failed",
		ToolName:     toolName,
		ToolCallID:   toolCallID,
		ErrorMessage: sanitizeADKToolErrorMessage(message),
		Recoverable:  true,
		Normalized:   true,
	}
}

func encodeADKToolErrorResult(
	toolName string,
	toolCallID string,
	err error,
) string {
	return mustMarshalADKToolResultProtocol(
		newADKToolErrorPayload(toolName, toolCallID, err),
	)
}

func decodeADKToolErrorResult(content string) (*ADKToolErrorPayload, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}

	var payload ADKToolErrorPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil, false
	}
	if payload.Schema != adkToolErrorSchema || payload.Status != "failed" {
		return nil, false
	}

	return &payload, true
}

func newADKToolRepairPayload(
	toolName string,
	toolCallID string,
) ADKToolRepairPayload {
	return ADKToolRepairPayload{
		Schema:     adkToolRepairSchema,
		Status:     "patched",
		ToolName:   toolName,
		ToolCallID: toolCallID,
		Reason:     adkToolRepairReasonMissing,
		Message:    adkToolRepairModelInstruction,
	}
}

func encodeADKToolRepairResult(toolName string, toolCallID string) string {
	return mustMarshalADKToolResultProtocol(
		newADKToolRepairPayload(toolName, toolCallID),
	)
}

func emitADKToolRepairEvent(
	ctx context.Context,
	run *RunSummary,
	sink RunEventSink,
	toolName string,
	toolCallID string,
) {
	if run == nil {
		return
	}

	emitRunEvent(ctx, sink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: "tool.repaired",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"schema":       adkToolRepairSchema,
			"tool_name":    toolName,
			"tool_call_id": toolCallID,
			"reason":       adkToolRepairReasonMissing,
		}),
	})
}

func sanitizeADKToolErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return adkToolErrorDefaultMessage
	}
	runes := []rune(message)
	if len(runes) > adkToolErrorMessageMaxLength {
		message = string(runes[:adkToolErrorMessageMaxLength])
	}

	return message
}

func mustMarshalADKToolResultProtocol(payload any) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"schema":"coze.tool_error.v1","status":"failed","error_message":"tool call failed","recoverable":true,"normalized":true}`
	}

	return string(encoded)
}
