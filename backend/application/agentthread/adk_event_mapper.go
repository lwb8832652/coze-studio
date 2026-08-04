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
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/summarization"
	"github.com/cloudwego/eino/schema"
)

type ADKEventMapping struct {
	RunEvent
	Usage              *AgentTokenUsage
	FinalText          string
	Interrupt          *ADKInterruptMapping
	TerminalToolCallID string
}

type ADKInterruptMapping struct {
	Items []ADKInterruptItem `json:"items"`
}

type ADKInterruptItem struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	Info        any    `json:"info,omitempty"`
	IsRootCause bool   `json:"is_root_cause"`
	ParentID    string `json:"parent_id,omitempty"`
}

type adkEventBasePayload struct {
	AgentName string              `json:"agent_name,omitempty"`
	RunPath   []string            `json:"run_path,omitempty"`
	Subagent  *adkSubagentPayload `json:"subagent,omitempty"`
}

type adkSubagentPayload struct {
	Name       string   `json:"name"`
	RootName   string   `json:"root_name,omitempty"`
	ParentName string   `json:"parent_name,omitempty"`
	StepID     string   `json:"step_id"`
	RunPath    []string `json:"run_path"`
	Depth      int      `json:"depth"`
}

type adkMessagePayload struct {
	adkEventBasePayload
	Role                 schema.RoleType                 `json:"role"`
	Content              string                          `json:"content"`
	ReasoningContent     string                          `json:"reasoning_content,omitempty"`
	ReasoningParts       []adkReasoningPartPayload       `json:"reasoning_parts,omitempty"`
	ContentParts         []adkContentPartPayload         `json:"content_parts,omitempty"`
	FinishReason         string                          `json:"finish_reason,omitempty"`
	FinishClassification *adkFinishClassificationPayload `json:"finish_classification,omitempty"`
	ToolCalls            []schema.ToolCall               `json:"tool_calls,omitempty"`
	ToolName             string                          `json:"tool_name,omitempty"`
	ToolCallID           string                          `json:"tool_call_id,omitempty"`
	PlanTaskID           string                          `json:"plan_task_id,omitempty"`
	ToolError            *ADKToolErrorPayload            `json:"tool_error,omitempty"`
	Media                []adkMediaPayload               `json:"media,omitempty"`
	Usage                *adkUsagePayload                `json:"usage,omitempty"`
}

type adkContentPartPayload struct {
	Type schema.ChatMessagePartType `json:"type"`
	Text string                     `json:"text"`
}

type adkReasoningPartPayload struct {
	Text      string `json:"text,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type adkFinishClassificationPayload struct {
	Category string `json:"category"`
	Reason   string `json:"reason"`
	Safety   bool   `json:"safety"`
	Terminal bool   `json:"terminal"`
}

type adkMediaPayload struct {
	Type          schema.ChatMessagePartType `json:"type"`
	URL           string                     `json:"url,omitempty"`
	MIMEType      string                     `json:"mime_type,omitempty"`
	HasBase64Data bool                       `json:"has_base64_data"`
}

type adkUsagePayload struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	CachedTokens     int `json:"cached_tokens"`
	ReasoningTokens  int `json:"reasoning_tokens"`
}

func MapADKEvent(ctx context.Context, threadID, runID int64, event *adk.AgentEvent) (*ADKEventMapping, error) {
	if event == nil {
		return nil, fmt.Errorf("agent event is required")
	}

	mapped := &ADKEventMapping{
		RunEvent: RunEvent{
			ThreadID: threadID,
			RunID:    runID,
		},
	}
	base := newADKEventBasePayload(event)

	if event.Err != nil {
		eventType, payload, interrupt := mapADKError(base, event.Err)
		if event.Output != nil && event.Output.MessageOutput != nil {
			partial, usage, finalText, err := mapADKMessage(base, event.Output.MessageOutput)
			if err != nil {
				return nil, fmt.Errorf("map partial agent message: %w", err)
			}
			attachADKJournalPlanTask(ctx, &partial)
			payload["partial_message"] = partial
			mapped.Usage = usage
			mapped.FinalText = finalText
		}

		encoded, err := marshalADKPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal %s payload: %w", eventType, err)
		}
		mapped.EventType = eventType
		mapped.Payload = encoded
		mapped.Interrupt = interrupt

		return mapped, nil
	}

	if event.Action != nil && event.Action.Interrupted != nil {
		interrupt := mapADKInterrupts(event.Action.Interrupted.InterruptContexts)
		payload := adkBasePayloadMap(base)
		payload["interrupts"] = interrupt.Items
		if prompts := humanInteractionPromptsFromInterrupts(interrupt.Items); len(prompts) > 0 {
			payload["human_interaction"] = prompts[0]
			payload["human_interactions"] = prompts
		}
		if event.Action.Interrupted.Data != nil {
			payload["data"] = event.Action.Interrupted.Data
		}

		encoded, err := marshalADKPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal run.interrupted payload: %w", err)
		}
		mapped.EventType = "run.interrupted"
		mapped.Payload = encoded
		mapped.Interrupt = interrupt

		return mapped, nil
	}

	if event.Action != nil && event.Action.CustomizedAction != nil {
		action, ok := event.Action.CustomizedAction.(*summarization.CustomizedAction)
		if ok {
			return mapADKSummarizationAction(threadID, runID, base, action)
		}
	}

	if event.Output != nil && event.Output.MessageOutput != nil {
		payload, usage, finalText, err := mapADKMessage(base, event.Output.MessageOutput)
		if err != nil {
			return nil, err
		}
		if isADKPlanCompletionGuardPayload(payload) {
			encoded, err := marshalADKPayload(map[string]any{
				"hidden": true,
				"schema": adkPlanCompletionGuardSchema,
			})
			if err != nil {
				return nil, fmt.Errorf("marshal agent.control payload: %w", err)
			}
			mapped.EventType = "agent.control"
			mapped.Payload = encoded
			return mapped, nil
		}
		attachADKJournalPlanTask(ctx, &payload)

		mapped.EventType = "message.completed"
		if payload.Role == schema.Tool {
			mapped.EventType = "tool.completed"
			mapped.TerminalToolCallID = adkJournalToolBindingKey(
				payload.AgentName,
				payload.ToolCallID,
			)
			if toolError, ok := decodeADKToolErrorResult(payload.Content); ok {
				payload.ToolError = toolError
				mapped.EventType = "tool.failed"
			}
		} else if payload.FinishClassification != nil &&
			payload.FinishClassification.Safety {
			mapped.EventType = "model.safety_finish"
		}
		encoded, err := marshalADKPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal agent message payload: %w", err)
		}
		mapped.Payload = encoded
		mapped.Usage = usage
		mapped.FinalText = finalText

		return mapped, nil
	}

	payload := adkBasePayloadMap(base)
	switch {
	case event.Output != nil && event.Output.CustomizedOutput != nil:
		mapped.EventType = "agent.output"
		payload["customized_output"] = event.Output.CustomizedOutput
	case event.Action != nil:
		mapped.EventType = "agent.action"
		mapADKAction(payload, event.Action)
	default:
		mapped.EventType = "agent.event"
	}

	encoded, err := marshalADKPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", mapped.EventType, err)
	}
	mapped.Payload = encoded

	return mapped, nil
}

func attachADKJournalPlanTask(ctx context.Context, payload *adkMessagePayload) {
	if payload == nil {
		return
	}
	switch payload.Role {
	case schema.Assistant:
		payload.PlanTaskID = activeADKPlanTaskIDFromContext(ctx)
		toolCallIDs := make([]string, 0, len(payload.ToolCalls))
		for _, call := range payload.ToolCalls {
			toolCallIDs = append(toolCallIDs, call.ID)
		}
		if !bindADKJournalScopedToolPlanTasks(
			ctx,
			payload.AgentName,
			toolCallIDs,
			payload.PlanTaskID,
		) {
			payload.PlanTaskID = ""
		}
	case schema.Tool:
		payload.PlanTaskID = boundADKJournalScopedToolPlanTaskIDFromContext(
			ctx,
			payload.AgentName,
			payload.ToolCallID,
		)
	}
}

func mapADKSummarizationAction(
	threadID, runID int64,
	base adkEventBasePayload,
	action *summarization.CustomizedAction,
) (*ADKEventMapping, error) {
	if action == nil {
		return nil, fmt.Errorf("summarization action is required")
	}

	payload := adkBasePayloadMap(base)

	eventType := ""
	switch action.Type {
	case summarization.ActionTypeBeforeSummarize:
		if action.Before == nil {
			return nil, fmt.Errorf("before summarization action is required")
		}
		digest, err := digestADKMessages(action.Before.Messages)
		if err != nil {
			return nil, err
		}
		eventType = "context.summarizing"
		payload["message_count"] = len(action.Before.Messages)
		payload["digest"] = digest
	case summarization.ActionTypeGenerateSummary:
		if action.GenerateSummary == nil {
			return nil, fmt.Errorf("generate summary action is required")
		}
		digest, err := digestADKMessages([]*schema.Message{
			action.GenerateSummary.ModelResponse,
		})
		if err != nil {
			return nil, err
		}
		eventType = "context.summary_model_call"
		payload["attempt"] = action.GenerateSummary.Attempt
		payload["phase"] = action.GenerateSummary.Phase
		payload["digest"] = digest
		if actionErr := action.GenerateSummary.GetError(); actionErr != nil {
			payload["error"] = actionErr.Error()
		}
	case summarization.ActionTypeAfterSummarize:
		if action.After == nil {
			return nil, fmt.Errorf("after summarization action is required")
		}
		digest, err := digestADKMessages(action.After.Messages)
		if err != nil {
			return nil, err
		}
		eventType = "context.summarized"
		payload["message_count"] = len(action.After.Messages)
		payload["digest"] = digest
	default:
		return nil, fmt.Errorf("unsupported summarization action type: %s", action.Type)
	}

	encoded, err := marshalADKPayload(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal %s payload: %w", eventType, err)
	}
	return &ADKEventMapping{
		RunEvent: RunEvent{
			ThreadID:  threadID,
			RunID:     runID,
			EventType: eventType,
			Payload:   encoded,
		},
	}, nil
}

func digestADKMessages(messages []*schema.Message) (string, error) {
	hasher := sha256.New()
	for _, message := range messages {
		if message == nil {
			continue
		}
		raw, err := json.Marshal(struct {
			Role                     schema.RoleType            `json:"role"`
			Content                  string                     `json:"content"`
			Name                     string                     `json:"name,omitempty"`
			ToolCalls                []schema.ToolCall          `json:"tool_calls,omitempty"`
			ToolCallID               string                     `json:"tool_call_id,omitempty"`
			ToolName                 string                     `json:"tool_name,omitempty"`
			ReasoningContent         string                     `json:"reasoning_content,omitempty"`
			UserInputMultiContent    []schema.MessageInputPart  `json:"user_input_multi_content,omitempty"`
			AssistantGenMultiContent []schema.MessageOutputPart `json:"assistant_output_multi_content,omitempty"`
		}{
			Role:                     message.Role,
			Content:                  message.Content,
			Name:                     message.Name,
			ToolCalls:                message.ToolCalls,
			ToolCallID:               message.ToolCallID,
			ToolName:                 message.ToolName,
			ReasoningContent:         message.ReasoningContent,
			UserInputMultiContent:    message.UserInputMultiContent,
			AssistantGenMultiContent: message.AssistantGenMultiContent,
		})
		if err != nil {
			return "", fmt.Errorf("digest adk messages: %w", err)
		}
		_, _ = hasher.Write(raw)
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func mapADKMessage(
	base adkEventBasePayload,
	variant *adk.MessageVariant,
) (adkMessagePayload, *AgentTokenUsage, string, error) {
	if variant == nil {
		return adkMessagePayload{}, nil, "", fmt.Errorf("message variant is required")
	}
	if variant.IsStreaming && variant.MessageStream == nil {
		return adkMessagePayload{}, nil, "", fmt.Errorf("streaming message reader is required")
	}

	message, err := variant.GetMessage()
	if err != nil {
		return adkMessagePayload{}, nil, "", fmt.Errorf("consume agent message: %w", err)
	}
	if message == nil {
		return adkMessagePayload{}, nil, "", fmt.Errorf("agent message is required")
	}

	role := message.Role
	if role == "" {
		role = variant.Role
	}
	toolName := message.ToolName
	if toolName == "" {
		toolName = variant.ToolName
	}

	payload := adkMessagePayload{
		adkEventBasePayload: base,
		Role:                role,
		Content:             message.Content,
		ReasoningContent:    message.ReasoningContent,
		ToolCalls:           message.ToolCalls,
		ToolName:            toolName,
		ToolCallID:          message.ToolCallID,
	}
	payload.ContentParts, payload.ReasoningParts, payload.Media = mapADKOutputParts(message.AssistantGenMultiContent)
	inputContentParts, inputMedia := mapADKInputParts(message.UserInputMultiContent)
	if payload.Content == "" && len(inputContentParts) > 0 {
		payload.Content = joinADKTextContentParts(inputContentParts)
	}
	payload.ContentParts = append(payload.ContentParts, inputContentParts...)
	payload.Media = append(payload.Media, inputMedia...)

	var usage *AgentTokenUsage
	if message.ResponseMeta != nil {
		payload.FinishReason = message.ResponseMeta.FinishReason
		payload.FinishClassification = newADKFinishClassification(
			message.ResponseMeta.FinishReason,
		)
		if message.ResponseMeta.Usage != nil {
			payload.Usage = newADKUsagePayload(message.ResponseMeta.Usage)
			usage, err = newAgentTokenUsage(base, payload.Usage)
			if err != nil {
				return adkMessagePayload{}, nil, "", err
			}
		}
	}

	finalText := ""
	if role == schema.Assistant {
		finalText = message.Content
	}

	return payload, usage, finalText, nil
}

func isADKPlanCompletionGuardPayload(payload adkMessagePayload) bool {
	if payload.Role == schema.Tool &&
		payload.ToolName == adkPlanCompletionGuardToolName {
		return true
	}
	if payload.Role != schema.Assistant || len(payload.ToolCalls) == 0 {
		return false
	}
	for _, call := range payload.ToolCalls {
		if call.Function.Name != adkPlanCompletionGuardToolName {
			return false
		}
	}
	return true
}

func newADKFinishClassification(reason string) *adkFinishClassificationPayload {
	normalized := normalizeADKFinishReason(reason)
	if normalized == "" {
		return nil
	}
	if isADKSafetyFinishReason(normalized) {
		return &adkFinishClassificationPayload{
			Category: "safety",
			Reason:   normalized,
			Safety:   true,
			Terminal: true,
		}
	}

	return nil
}

func mapADKInputParts(parts []schema.MessageInputPart) (
	[]adkContentPartPayload,
	[]adkMediaPayload,
) {
	var content []adkContentPartPayload
	var media []adkMediaPayload

	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			content = append(content, adkContentPartPayload{
				Type: part.Type,
				Text: part.Text,
			})
		case schema.ChatMessagePartTypeImageURL:
			if part.Image != nil {
				media = append(media, newADKMediaPayload(part.Type, part.Image.MessagePartCommon))
			}
		case schema.ChatMessagePartTypeAudioURL:
			if part.Audio != nil {
				media = append(media, newADKMediaPayload(part.Type, part.Audio.MessagePartCommon))
			}
		case schema.ChatMessagePartTypeVideoURL:
			if part.Video != nil {
				media = append(media, newADKMediaPayload(part.Type, part.Video.MessagePartCommon))
			}
		case schema.ChatMessagePartTypeFileURL:
			if part.File != nil {
				media = append(media, newADKMediaPayload(part.Type, part.File.MessagePartCommon))
			}
		}
	}

	return content, media
}

func joinADKTextContentParts(parts []adkContentPartPayload) string {
	var texts []string
	for _, part := range parts {
		if part.Type == schema.ChatMessagePartTypeText {
			texts = append(texts, part.Text)
		}
	}

	return strings.Join(texts, "")
}

func mapADKOutputParts(parts []schema.MessageOutputPart) (
	[]adkContentPartPayload,
	[]adkReasoningPartPayload,
	[]adkMediaPayload,
) {
	var content []adkContentPartPayload
	var reasoning []adkReasoningPartPayload
	var media []adkMediaPayload

	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			content = append(content, adkContentPartPayload{
				Type: part.Type,
				Text: part.Text,
			})
		case schema.ChatMessagePartTypeReasoning:
			if part.Reasoning != nil {
				reasoning = append(reasoning, adkReasoningPartPayload{
					Text:      part.Reasoning.Text,
					Signature: part.Reasoning.Signature,
				})
			}
		case schema.ChatMessagePartTypeImageURL:
			if part.Image != nil {
				media = append(media, newADKMediaPayload(part.Type, part.Image.MessagePartCommon))
			}
		case schema.ChatMessagePartTypeAudioURL:
			if part.Audio != nil {
				media = append(media, newADKMediaPayload(part.Type, part.Audio.MessagePartCommon))
			}
		case schema.ChatMessagePartTypeVideoURL:
			if part.Video != nil {
				media = append(media, newADKMediaPayload(part.Type, part.Video.MessagePartCommon))
			}
		}
	}

	return content, reasoning, media
}

func newADKMediaPayload(partType schema.ChatMessagePartType, common schema.MessagePartCommon) adkMediaPayload {
	payload := adkMediaPayload{
		Type:          partType,
		MIMEType:      common.MIMEType,
		HasBase64Data: common.Base64Data != nil && *common.Base64Data != "",
	}
	if common.URL != nil {
		payload.URL = *common.URL
	}

	return payload
}

func newADKUsagePayload(usage *schema.TokenUsage) *adkUsagePayload {
	return &adkUsagePayload{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.PromptTokenDetails.CachedTokens,
		ReasoningTokens:  usage.CompletionTokensDetails.ReasoningTokens,
	}
}

func newAgentTokenUsage(base adkEventBasePayload, usage *adkUsagePayload) (*AgentTokenUsage, error) {
	rawUsage, err := marshalADKPayload(usage)
	if err != nil {
		return nil, fmt.Errorf("marshal agent token usage: %w", err)
	}

	source := TokenUsageSourceLeadAgent
	if len(base.RunPath) > 1 {
		source = TokenUsageSourceSubagent
	}
	stepID := strings.Join(base.RunPath, "/")
	if stepID == "" {
		stepID = base.AgentName
	}
	stepIndex := 0
	if len(base.RunPath) > 0 {
		stepIndex = len(base.RunPath) - 1
	}
	metadataPayload := adkBasePayloadMap(base)
	metadataPayload["source"] = "eino_event"
	metadataPayload["step_id"] = stepID
	metadataPayload["step_index"] = stepIndex
	metadata, err := marshalADKPayload(metadataPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal agent token usage metadata: %w", err)
	}

	return &AgentTokenUsage{
		Source:       source,
		StepID:       stepID,
		StepIndex:    int32(stepIndex),
		StepName:     base.AgentName,
		InputTokens:  int64(usage.PromptTokens),
		OutputTokens: int64(usage.CompletionTokens),
		TotalTokens:  int64(usage.TotalTokens),
		RawUsage:     rawUsage,
		Metadata:     metadata,
	}, nil
}

func mapADKError(
	base adkEventBasePayload,
	eventErr error,
) (string, map[string]any, *ADKInterruptMapping) {
	payload := adkBasePayloadMap(base)

	var retrying *adk.WillRetryError
	if errors.As(eventErr, &retrying) {
		payload["error"] = retrying.Error()
		payload["retry_attempt"] = retrying.RetryAttempt
		if reason := retrying.RejectReason(); reason != nil {
			payload["reject_reason"] = reason
		}

		return "model.retrying", payload, nil
	}

	var exhausted *adk.RetryExhaustedError
	if errors.As(eventErr, &exhausted) {
		payload["error"] = exhausted.Error()
		payload["total_retries"] = exhausted.TotalRetries
		if exhausted.LastErr != nil {
			payload["last_error"] = exhausted.LastErr.Error()
		}

		return "model.retry_exhausted", payload, nil
	}

	var semanticLoop *ADKSemanticLoopError
	if errors.As(eventErr, &semanticLoop) {
		payload["error"] = semanticLoop.Error()
		payload["kind"] = string(semanticLoop.Kind)
		payload["signature"] = semanticLoop.Signature
		payload["count"] = semanticLoop.Count
		payload["limit"] = semanticLoop.Limit

		return "run.semantic_loop_detected", payload, nil
	}

	var unsupportedCapability *ADKProviderCapabilityError
	if errors.As(eventErr, &unsupportedCapability) {
		payload["error"] = unsupportedCapability.Error()
		payload["capability"] = string(unsupportedCapability.Capability)
		payload["part_type"] = unsupportedCapability.PartType
		payload["count"] = unsupportedCapability.Count

		return "model.unsupported_capability", payload, nil
	}

	var canceled *adk.CancelError
	if errors.As(eventErr, &canceled) {
		payload["error"] = "agent canceled"
		if canceled.Info != nil {
			payload["error"] = canceled.Error()
			payload["mode"] = canceled.Info.Mode
			payload["escalated"] = canceled.Info.Escalated
			payload["timeout"] = canceled.Info.Timeout
		}
		interrupt := mapADKInterrupts(canceled.InterruptContexts)
		if len(interrupt.Items) > 0 {
			payload["interrupts"] = interrupt.Items
		}

		return "run.canceling", payload, interrupt
	}

	if isADKCancellationError(eventErr) {
		payload["error"] = eventErr.Error()
		return "run.canceling", payload, nil
	}

	payload["error"] = eventErr.Error()

	return "run.runtime_error", payload, nil
}

func mapADKInterrupts(contexts []*adk.InterruptCtx) *ADKInterruptMapping {
	mapping := &ADKInterruptMapping{
		Items: make([]ADKInterruptItem, 0, len(contexts)),
	}
	for _, interrupt := range contexts {
		if interrupt == nil {
			continue
		}
		item := ADKInterruptItem{
			ID:          interrupt.ID,
			Address:     interrupt.Address.String(),
			Info:        interrupt.Info,
			IsRootCause: interrupt.IsRootCause,
		}
		if interrupt.Parent != nil {
			item.ParentID = interrupt.Parent.ID
		}
		mapping.Items = append(mapping.Items, item)
	}

	return mapping
}

func mapADKAction(payload map[string]any, action *adk.AgentAction) {
	if action.Exit {
		payload["exit"] = true
	}
	if action.TransferToAgent != nil {
		payload["transfer_to_agent"] = action.TransferToAgent.DestAgentName
	}
	if action.BreakLoop != nil {
		payload["break_loop"] = map[string]any{
			"from":               action.BreakLoop.From,
			"done":               action.BreakLoop.Done,
			"current_iterations": action.BreakLoop.CurrentIterations,
		}
	}
	if action.CustomizedAction != nil {
		payload["customized_action"] = action.CustomizedAction
	}
}

func newADKEventBasePayload(event *adk.AgentEvent) adkEventBasePayload {
	if event == nil {
		return adkEventBasePayload{}
	}
	base := adkEventBasePayload{
		AgentName: strings.TrimSpace(event.AgentName),
		RunPath:   adkRunPath(event.RunPath),
	}
	base.Subagent = adkSubagentPayloadFromRunPath(base.AgentName, base.RunPath)

	return base
}

func adkBasePayloadMap(base adkEventBasePayload) map[string]any {
	payload := map[string]any{}
	if base.AgentName != "" {
		payload["agent_name"] = base.AgentName
	}
	if len(base.RunPath) > 0 {
		payload["run_path"] = base.RunPath
	}
	if base.Subagent != nil {
		payload["subagent"] = base.Subagent
	}

	return payload
}

func adkSubagentPayloadFromRunPath(
	agentName string,
	runPath []string,
) *adkSubagentPayload {
	if len(runPath) <= 1 {
		return nil
	}
	name := strings.TrimSpace(agentName)
	if name == "" {
		name = strings.TrimSpace(runPath[len(runPath)-1])
	}
	stepID := strings.Join(runPath, "/")
	if name == "" || stepID == "" {
		return nil
	}

	return &adkSubagentPayload{
		Name:       name,
		RootName:   strings.TrimSpace(runPath[0]),
		ParentName: strings.TrimSpace(runPath[len(runPath)-2]),
		StepID:     stepID,
		RunPath:    append([]string(nil), runPath...),
		Depth:      len(runPath) - 1,
	}
}

func adkRunPath(runPath []adk.RunStep) []string {
	if len(runPath) == 0 {
		return nil
	}

	path := make([]string, 0, len(runPath))
	for i := range runPath {
		path = append(path, runPath[i].String())
	}

	return path
}

func marshalADKPayload(payload any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}
