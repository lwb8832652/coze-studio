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

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

const adkSemanticLoopMaxRepeatedLimit int64 = 20

type ADKSemanticLoopKind string

const (
	ADKSemanticLoopKindToolCalls        ADKSemanticLoopKind = "tool_calls"
	ADKSemanticLoopKindAssistantMessage ADKSemanticLoopKind = "assistant_message"
)

type ADKSemanticLoopConfig struct {
	MaxRepeatedToolCalls         int
	MaxRepeatedAssistantMessages int
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
	config ADKSemanticLoopConfig
}

func NewADKSemanticLoopMiddleware(
	config ADKSemanticLoopConfig,
) *ADKSemanticLoopMiddleware {
	return &ADKSemanticLoopMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		config:                       config,
	}
}

func (m *ADKSemanticLoopMiddleware) AfterModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || state == nil {
		return ctx, state, nil
	}
	if limit := m.config.MaxRepeatedToolCalls; limit > 0 {
		signature, count, ok := adkConsecutiveSemanticLoopCount(
			state.Messages,
			ADKSemanticLoopKindToolCalls,
		)
		if ok && count > limit {
			return ctx, state, &ADKSemanticLoopError{
				Kind:      ADKSemanticLoopKindToolCalls,
				Signature: signature,
				Count:     count,
				Limit:     limit,
			}
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
	config := ADKSemanticLoopConfig{}
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

	return config, nil
}

func adkSemanticLoopLimitFromConfig(
	payload map[string]any,
	name string,
	keys ...string,
) (int, error) {
	value, exists, err := firstConfigInt64AllowZero(payload, keys...)
	if err != nil {
		return 0, fmt.Errorf("semantic loop %s is invalid: %w", name, err)
	}
	if !exists {
		return 0, nil
	}
	if value > adkSemanticLoopMaxRepeatedLimit {
		return 0, fmt.Errorf(
			"semantic loop %s must be between 0 and %d",
			name,
			adkSemanticLoopMaxRepeatedLimit,
		)
	}

	return int(value), nil
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
