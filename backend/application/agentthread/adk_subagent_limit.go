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
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const (
	adkSubagentToolLimitEventType     = "subagent.tool_calls_truncated"
	adkSubagentToolLimitSchema        = "coze.subagent_tool_limit.v1"
	adkSubagentToolLimitMaxEventNames = 16
)

type ADKSubagentLimitMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run       *RunSummary
	toolNames map[string]struct{}
	limit     int
	eventSink RunEventSink
}

func NewADKSubagentLimitMiddleware(
	run *RunSummary,
	toolNames []string,
	limit int,
	eventSink RunEventSink,
) *ADKSubagentLimitMiddleware {
	names := make(map[string]struct{}, len(toolNames))
	for _, rawName := range toolNames {
		name := strings.TrimSpace(rawName)
		if name != "" {
			names[name] = struct{}{}
		}
	}
	if limit < minDeerFlowMaxConcurrentSubagents {
		limit = minDeerFlowMaxConcurrentSubagents
	}
	if limit > maxDeerFlowMaxConcurrentSubagents {
		limit = maxDeerFlowMaxConcurrentSubagents
	}
	return &ADKSubagentLimitMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		run:                          run,
		toolNames:                    names,
		limit:                        limit,
		eventSink:                    eventSink,
	}
}

func (m *ADKSubagentLimitMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || state == nil || len(m.toolNames) == 0 || len(state.Messages) == 0 {
		return ctx, state, nil
	}
	lastIndex := len(state.Messages) - 1
	last := state.Messages[lastIndex]
	if last == nil || last.Role != schema.Assistant || len(last.ToolCalls) == 0 {
		return ctx, state, nil
	}

	requested := 0
	for _, call := range last.ToolCalls {
		if m.isSubagentToolCall(call) {
			requested++
		}
	}
	if requested <= m.limit {
		return ctx, state, nil
	}

	retainedSubagents := 0
	retainedCalls := make([]schema.ToolCall, 0, len(last.ToolCalls))
	droppedNames := make([]string, 0, requested-m.limit)
	for _, call := range last.ToolCalls {
		if !m.isSubagentToolCall(call) {
			retainedCalls = append(retainedCalls, call)
			continue
		}
		if retainedSubagents < m.limit {
			retainedSubagents++
			retainedCalls = append(retainedCalls, call)
			continue
		}
		droppedNames = append(droppedNames, call.Function.Name)
	}

	nextMessages := cloneADKMessagesForProjection(state.Messages)
	nextMessages[lastIndex].ToolCalls = cloneADKToolCalls(retainedCalls)
	nextState := *state
	nextState.Messages = nextMessages
	m.emitTruncationEvent(ctx, requested, retainedSubagents, droppedNames)

	return ctx, &nextState, nil
}

func (m *ADKSubagentLimitMiddleware) isSubagentToolCall(call schema.ToolCall) bool {
	if m == nil {
		return false
	}
	_, ok := m.toolNames[strings.TrimSpace(call.Function.Name)]
	return ok
}

func (m *ADKSubagentLimitMiddleware) emitTruncationEvent(
	ctx context.Context,
	requested int,
	retained int,
	droppedNames []string,
) {
	if m == nil || m.run == nil || len(droppedNames) == 0 {
		return
	}
	emitRunEvent(ctx, m.eventSink, RunEvent{
		ThreadID:  m.run.ThreadID,
		RunID:     m.run.RunID,
		EventType: adkSubagentToolLimitEventType,
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"schema":                   adkSubagentToolLimitSchema,
			"limit":                    m.limit,
			"requested_subagent_calls": requested,
			"retained_subagent_calls":  retained,
			"dropped_subagent_calls":   len(droppedNames),
			"dropped_tool_names":       boundedADKSubagentToolNames(droppedNames),
		}),
	})
}

func boundedADKSubagentToolNames(names []string) []string {
	bounded := make([]string, 0, min(len(names), adkSubagentToolLimitMaxEventNames))
	seen := make(map[string]struct{}, len(names))
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		bounded = append(bounded, name)
		if len(bounded) == adkSubagentToolLimitMaxEventNames {
			break
		}
	}
	return bounded
}
