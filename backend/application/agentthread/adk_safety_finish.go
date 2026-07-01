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
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	adkSafetyTerminationSchema = "coze.safety_termination.v1"
)

var adkSafetyOpenAIFinishReasons = map[string]struct{}{
	"content_filter": {},
}

var adkSafetyAnthropicStopReasons = map[string]struct{}{
	"refusal": {},
}

var adkSafetyGeminiFinishReasons = map[string]struct{}{
	"safety":                   {},
	"blocklist":                {},
	"prohibited_content":       {},
	"spii":                     {},
	"recitation":               {},
	"image_safety":             {},
	"image_prohibited_content": {},
	"image_recitation":         {},
}

type ADKSafetyFinishMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run       *RunSummary
	eventSink RunEventSink
}

type adkSafetyTermination struct {
	Detector    string
	ReasonField string
	ReasonValue string
	Extras      map[string]any
}

func NewADKSafetyFinishMiddleware(
	run *RunSummary,
	eventSink RunEventSink,
) *ADKSafetyFinishMiddleware {
	return &ADKSafetyFinishMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		run:                          run,
		eventSink:                    eventSink,
	}
}

func (m *ADKSafetyFinishMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || state == nil || len(state.Messages) == 0 {
		return ctx, state, nil
	}

	lastIndex := len(state.Messages) - 1
	last := state.Messages[lastIndex]
	if last == nil || last.Role != schema.Assistant || len(last.ToolCalls) == 0 {
		return ctx, state, nil
	}

	termination, ok := detectADKSafetyTermination(last)
	if !ok {
		return ctx, state, nil
	}

	nextMessages := cloneADKMessagesForProjection(state.Messages)
	patched := nextMessages[lastIndex]
	if patched == nil {
		return ctx, state, nil
	}

	suppressedNames := adkSafetyToolCallNames(last.ToolCalls)
	patched.ToolCalls = nil
	patched.Content = appendADKSafetyUserMessage(
		patched.Content,
		termination,
	)
	if patched.Extra == nil {
		patched.Extra = map[string]any{}
	}
	patched.Extra["safety_termination"] = map[string]any{
		"detector":                   termination.Detector,
		"reason_field":               termination.ReasonField,
		"reason_value":               termination.ReasonValue,
		"suppressed_tool_call_count": len(suppressedNames),
		"suppressed_tool_call_names": suppressedNames,
		"extras":                     cloneADKAnyMap(termination.Extras),
	}

	nextState := *state
	nextState.Messages = nextMessages
	emitADKSafetyTerminationEvent(
		ctx,
		m.run,
		m.eventSink,
		termination,
		last.ToolCalls,
	)

	return ctx, &nextState, nil
}

func detectADKSafetyTermination(
	message *schema.Message,
) (adkSafetyTermination, bool) {
	if value := adkSafetyMetadataString(message, "finish_reason"); value != "" {
		normalized := normalizeADKFinishReason(value)
		if _, ok := adkSafetyOpenAIFinishReasons[normalized]; ok {
			return adkSafetyTermination{
				Detector:    "openai_compatible_content_filter",
				ReasonField: "finish_reason",
				ReasonValue: value,
				Extras:      adkSafetyTerminationExtras(message),
			}, true
		}
		if _, ok := adkSafetyGeminiFinishReasons[normalized]; ok {
			return adkSafetyTermination{
				Detector:    "gemini_safety",
				ReasonField: "finish_reason",
				ReasonValue: value,
				Extras:      adkSafetyTerminationExtras(message),
			}, true
		}
	}

	if value := adkSafetyMetadataString(message, "stop_reason"); value != "" {
		normalized := normalizeADKFinishReason(value)
		if _, ok := adkSafetyAnthropicStopReasons[normalized]; ok {
			return adkSafetyTermination{
				Detector:    "anthropic_refusal",
				ReasonField: "stop_reason",
				ReasonValue: value,
				Extras:      adkSafetyTerminationExtras(message),
			}, true
		}
	}

	return adkSafetyTermination{}, false
}

func adkSafetyMetadataString(message *schema.Message, field string) string {
	if message == nil {
		return ""
	}
	if field == "finish_reason" && message.ResponseMeta != nil {
		if value := strings.TrimSpace(message.ResponseMeta.FinishReason); value != "" {
			return value
		}
	}
	if message.Extra == nil {
		return ""
	}
	value, ok := message.Extra[field].(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(value)
}

func adkSafetyTerminationExtras(message *schema.Message) map[string]any {
	if message == nil || message.Extra == nil {
		return nil
	}

	extras := map[string]any{}
	for _, key := range []string{"content_filter_results", "safety_ratings"} {
		if value, ok := message.Extra[key]; ok {
			extras[key] = cloneADKJSONCompatibleValue(value)
		}
	}
	if len(extras) == 0 {
		return nil
	}

	return extras
}

func appendADKSafetyUserMessage(
	content string,
	termination adkSafetyTermination,
) string {
	explanation := fmt.Sprintf(
		"The model provider stopped this response with a safety-related signal (%s=%q, detector=%q). Any tool calls produced in this turn were suppressed because their arguments may be truncated and unsafe to execute. Please rephrase the request or ask for a narrower output.",
		termination.ReasonField,
		termination.ReasonValue,
		termination.Detector,
	)
	if strings.TrimSpace(content) == "" {
		return explanation
	}

	return content + "\n\n" + explanation
}

func adkSafetyToolCallNames(calls []schema.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			name = "unknown"
		}
		names = append(names, name)
	}

	return names
}

func adkSafetyToolCallIDs(calls []schema.ToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	ids := make([]string, 0, len(calls))
	for _, call := range calls {
		if id := strings.TrimSpace(call.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	return ids
}

func emitADKSafetyTerminationEvent(
	ctx context.Context,
	run *RunSummary,
	sink RunEventSink,
	termination adkSafetyTermination,
	calls []schema.ToolCall,
) {
	if run == nil || len(calls) == 0 {
		return
	}

	emitRunEvent(ctx, sink, RunEvent{
		ThreadID:  run.ThreadID,
		RunID:     run.RunID,
		EventType: "middleware:safety_termination",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"schema":                       adkSafetyTerminationSchema,
			"detector":                     termination.Detector,
			"reason_field":                 termination.ReasonField,
			"reason_value":                 termination.ReasonValue,
			"suppressed_tool_call_count":   len(calls),
			"suppressed_tool_call_names":   adkSafetyToolCallNames(calls),
			"suppressed_tool_call_ids":     adkSafetyToolCallIDs(calls),
			"extras":                       cloneADKAnyMap(termination.Extras),
			"raw_tool_arguments_persisted": false,
		}),
	})
}
