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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const adkSemanticLoopMaxRepeatedLimit int64 = 20

const (
	adkSemanticLoopDefaultWarnRepeatedToolCalls = 3
	adkSemanticLoopDefaultHardRepeatedToolCalls = 5

	adkSemanticLoopWarningMessage  = "[LOOP DETECTED] You are repeating the same tool calls. Stop calling tools and produce your final answer now. If you cannot complete the task, summarize what you accomplished so far."
	adkSemanticLoopHardStopMessage = "[FORCED STOP] Repeated tool calls exceeded the safety limit. Producing final answer with results collected so far."
)

type ADKSemanticLoopKind string

const (
	ADKSemanticLoopKindToolCalls        ADKSemanticLoopKind = "tool_calls"
	ADKSemanticLoopKindAssistantMessage ADKSemanticLoopKind = "assistant_message"
)

type ADKSemanticLoopConfig struct {
	MaxRepeatedToolCalls         int
	MaxRepeatedAssistantMessages int
	WarnRepeatedToolCalls        int
	HardRepeatedToolCalls        int
}

type ADKSemanticLoopError struct {
	Kind      ADKSemanticLoopKind
	Signature string
	Count     int
	Limit     int
}

func (e *ADKSemanticLoopError) Error() string {
	if e == nil {
		return "semantic loop detected"
	}
	kind := string(e.Kind)
	if kind == "" {
		kind = "unknown"
	}

	return fmt.Sprintf(
		"semantic loop detected: %s repeated %d times (limit %d)",
		kind,
		e.Count,
		e.Limit,
	)
}

type ADKSemanticLoopMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	config          ADKSemanticLoopConfig
	mu              sync.Mutex
	pendingWarnings []string
	warnedTools     map[string]struct{}
}

func NewADKSemanticLoopMiddleware(
	config ADKSemanticLoopConfig,
) *ADKSemanticLoopMiddleware {
	return &ADKSemanticLoopMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		config:                       config,
		warnedTools:                  map[string]struct{}{},
	}
}

func (m *ADKSemanticLoopMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || state == nil {
		return ctx, state, nil
	}
	warnings := m.drainPendingWarnings()
	if len(warnings) == 0 {
		return ctx, state, nil
	}

	next := *state
	next.Messages = cloneADKMessagesForProjection(state.Messages)
	next.Messages = append(next.Messages, &schema.Message{
		Role:    schema.User,
		Name:    "loop_warning",
		Content: strings.Join(deduplicateSemanticLoopWarnings(warnings), "\n\n"),
	})

	return ctx, &next, nil
}

func (m *ADKSemanticLoopMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || state == nil {
		return ctx, state, nil
	}
	signature, count, hasToolLoop := adkConsecutiveSemanticLoopCount(
		state.Messages,
		ADKSemanticLoopKindToolCalls,
	)
	if hasToolLoop {
		if limit := m.config.HardRepeatedToolCalls; limit > 0 && count >= limit {
			return ctx, m.semanticLoopHardStopState(state), nil
		}
		if limit := m.config.MaxRepeatedToolCalls; limit > 0 && count > limit {
			return ctx, state, &ADKSemanticLoopError{
				Kind:      ADKSemanticLoopKindToolCalls,
				Signature: signature,
				Count:     count,
				Limit:     limit,
			}
		}
		if limit := m.config.WarnRepeatedToolCalls; limit > 0 && count >= limit {
			m.queueToolWarning(signature)
		}
	}
	if limit := m.config.MaxRepeatedAssistantMessages; limit > 0 {
		signature, count, ok := adkConsecutiveSemanticLoopCount(
			state.Messages,
			ADKSemanticLoopKindAssistantMessage,
		)
		if ok && count > limit {
			return ctx, state, &ADKSemanticLoopError{
				Kind:      ADKSemanticLoopKindAssistantMessage,
				Signature: signature,
				Count:     count,
				Limit:     limit,
			}
		}
	}

	return ctx, state, nil
}

func adkSemanticLoopConfigFromRun(
	run *RunSummary,
) (ADKSemanticLoopConfig, error) {
	config := ADKSemanticLoopConfig{
		WarnRepeatedToolCalls: adkSemanticLoopDefaultWarnRepeatedToolCalls,
		HardRepeatedToolCalls: adkSemanticLoopDefaultHardRepeatedToolCalls,
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return config, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return config, fmt.Errorf("decode adk semantic loop config: %w", err)
	}
	loopConfig := firstConfigMap(payload, "semantic_loop", "semanticLoop")
	if loopConfig == nil {
		return config, nil
	}

	maxToolCalls, err := adkSemanticLoopLimitFromConfig(
		loopConfig,
		"max_repeated_tool_calls",
		"max_repeated_tool_calls",
		"maxRepeatedToolCalls",
	)
	if err != nil {
		return config, err
	}
	maxAssistantMessages, err := adkSemanticLoopLimitFromConfig(
		loopConfig,
		"max_repeated_assistant_messages",
		"max_repeated_assistant_messages",
		"maxRepeatedAssistantMessages",
	)
	if err != nil {
		return config, err
	}
	config.MaxRepeatedToolCalls = maxToolCalls
	config.MaxRepeatedAssistantMessages = maxAssistantMessages
	warnToolCalls, exists, err := adkSemanticLoopLimitFromConfigIfExists(
		loopConfig,
		"warn_repeated_tool_calls",
		"warn_repeated_tool_calls",
		"warnRepeatedToolCalls",
		"warn_threshold",
		"warnThreshold",
	)
	if err != nil {
		return config, err
	}
	if exists {
		config.WarnRepeatedToolCalls = warnToolCalls
	}
	hardToolCalls, exists, err := adkSemanticLoopLimitFromConfigIfExists(
		loopConfig,
		"hard_repeated_tool_calls",
		"hard_repeated_tool_calls",
		"hardRepeatedToolCalls",
		"hard_limit",
		"hardLimit",
	)
	if err != nil {
		return config, err
	}
	if exists {
		config.HardRepeatedToolCalls = hardToolCalls
	}
	if config.WarnRepeatedToolCalls > 0 &&
		config.HardRepeatedToolCalls > 0 &&
		config.WarnRepeatedToolCalls > config.HardRepeatedToolCalls {
		return config, fmt.Errorf(
			"semantic loop warn_repeated_tool_calls must be less than or equal to hard_repeated_tool_calls",
		)
	}

	return config, nil
}

func adkSemanticLoopLimitFromConfig(
	payload map[string]any,
	name string,
	keys ...string,
) (int, error) {
	value, exists, err := adkSemanticLoopLimitFromConfigIfExists(payload, name, keys...)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	return value, nil
}

func adkSemanticLoopLimitFromConfigIfExists(
	payload map[string]any,
	name string,
	keys ...string,
) (int, bool, error) {
	value, exists, err := firstConfigInt64AllowZero(payload, keys...)
	if err != nil {
		return 0, false, fmt.Errorf("semantic loop %s is invalid: %w", name, err)
	}
	if !exists {
		return 0, false, nil
	}
	if value > adkSemanticLoopMaxRepeatedLimit {
		return 0, false, fmt.Errorf(
			"semantic loop %s must be between 0 and %d",
			name,
			adkSemanticLoopMaxRepeatedLimit,
		)
	}

	return int(value), true, nil
}

func (m *ADKSemanticLoopMiddleware) queueToolWarning(signature string) {
	if strings.TrimSpace(signature) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.warnedTools == nil {
		m.warnedTools = map[string]struct{}{}
	}
	if _, ok := m.warnedTools[signature]; ok {
		return
	}
	m.warnedTools[signature] = struct{}{}
	m.pendingWarnings = append(m.pendingWarnings, adkSemanticLoopWarningMessage)
	if len(m.pendingWarnings) > 4 {
		m.pendingWarnings = m.pendingWarnings[len(m.pendingWarnings)-4:]
	}
}

func (m *ADKSemanticLoopMiddleware) drainPendingWarnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.pendingWarnings) == 0 {
		return nil
	}
	warnings := append([]string(nil), m.pendingWarnings...)
	m.pendingWarnings = nil
	return warnings
}

func (m *ADKSemanticLoopMiddleware) semanticLoopHardStopState(
	state *adk.ChatModelAgentState,
) *adk.ChatModelAgentState {
	if state == nil || len(state.Messages) == 0 {
		return state
	}
	next := *state
	next.Messages = cloneADKMessagesForProjection(state.Messages)
	last := next.Messages[len(next.Messages)-1]
	if last == nil || last.Role != schema.Assistant || len(last.ToolCalls) == 0 {
		return &next
	}
	last.ToolCalls = nil
	last.Content = appendSemanticLoopText(last.Content, adkSemanticLoopHardStopMessage)
	if last.ResponseMeta != nil {
		meta := *last.ResponseMeta
		if meta.FinishReason == "tool_calls" {
			meta.FinishReason = "stop"
		}
		last.ResponseMeta = &meta
	}
	m.drainPendingWarnings()
	return &next
}

func appendSemanticLoopText(content, addition string) string {
	content = strings.TrimSpace(content)
	addition = strings.TrimSpace(addition)
	if content == "" {
		return addition
	}
	if addition == "" {
		return content
	}
	return content + "\n\n" + addition
}

func deduplicateSemanticLoopWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(warnings))
	deduped := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		if _, ok := seen[warning]; ok {
			continue
		}
		seen[warning] = struct{}{}
		deduped = append(deduped, warning)
	}
	return deduped
}

func adkConsecutiveSemanticLoopCount(
	messages []*schema.Message,
	kind ADKSemanticLoopKind,
) (string, int, bool) {
	signature := ""
	count := 0
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message == nil {
			continue
		}
		if message.Role == schema.Tool {
			if count > 0 {
				continue
			}
			return "", 0, false
		}
		if message.Role != schema.Assistant {
			if count > 0 {
				break
			}
			return "", 0, false
		}

		current, ok := adkSemanticLoopMessageSignature(message, kind)
		if !ok {
			if count > 0 {
				break
			}
			return "", 0, false
		}
		if count == 0 {
			signature = current
			count = 1
			continue
		}
		if current != signature {
			break
		}
		count++
	}

	return signature, count, count > 0
}

func adkSemanticLoopMessageSignature(
	message *schema.Message,
	kind ADKSemanticLoopKind,
) (string, bool) {
	switch kind {
	case ADKSemanticLoopKindToolCalls:
		return adkToolCallsSemanticLoopSignature(message)
	case ADKSemanticLoopKindAssistantMessage:
		return adkAssistantMessageSemanticLoopSignature(message)
	default:
		return "", false
	}
}

type adkToolCallSemanticSignature struct {
	Type      string `json:"type,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

func adkToolCallsSemanticLoopSignature(
	message *schema.Message,
) (string, bool) {
	if message == nil ||
		message.Role != schema.Assistant ||
		len(message.ToolCalls) == 0 {
		return "", false
	}

	items := make([]adkToolCallSemanticSignature, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		items = append(items, adkToolCallSemanticSignature{
			Type:      strings.TrimSpace(call.Type),
			Name:      strings.TrimSpace(call.Function.Name),
			Arguments: adkCanonicalToolArguments(call.Function.Arguments),
		})
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return "", false
	}

	return adkHashSemanticLoopSignature(
		ADKSemanticLoopKindToolCalls,
		string(raw),
	), true
}

func adkAssistantMessageSemanticLoopSignature(
	message *schema.Message,
) (string, bool) {
	if message == nil ||
		message.Role != schema.Assistant ||
		len(message.ToolCalls) > 0 {
		return "", false
	}

	texts := make([]string, 0, 1+len(message.AssistantGenMultiContent))
	if message.Content != "" {
		texts = append(texts, message.Content)
	}
	for _, part := range message.AssistantGenMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	normalized := strings.Join(strings.Fields(strings.Join(texts, "\n")), " ")
	if normalized == "" {
		return "", false
	}

	return adkHashSemanticLoopSignature(
		ADKSemanticLoopKindAssistantMessage,
		normalized,
	), true
}

func adkCanonicalToolArguments(arguments string) string {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return ""
	}

	var decoded any
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return arguments
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return arguments
	}
	raw, err := json.Marshal(decoded)
	if err != nil {
		return arguments
	}

	return string(raw)
}

func adkHashSemanticLoopSignature(
	kind ADKSemanticLoopKind,
	value string,
) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(kind))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(value))

	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}
