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
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/coze-dev/coze-studio/backend/bizpkg/llm/modelbuilder"
)

type ChatModelProvider func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error)

type ModelExecutor struct {
	provider ChatModelProvider
}

func DefaultChatModelProvider(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
	if modelID > 0 {
		chatModel, _, err := modelbuilder.BuildModelByID(ctx, modelID, nil)
		if err != nil {
			return nil, false, err
		}

		return chatModel, true, nil
	}

	return modelbuilder.GetBuiltinChatModel(ctx, "AGENT_THREAD_")
}

func NewModelExecutor(provider ChatModelProvider) *ModelExecutor {
	if provider == nil {
		provider = DefaultChatModelProvider
	}

	return &ModelExecutor{provider: provider}
}

func (e *ModelExecutor) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
	if e == nil {
		return nil, fmt.Errorf("model executor is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}

	cfg, err := parseModelExecutorConfig(run.Config)
	if err != nil {
		return nil, err
	}

	messages, err := parseModelExecutorMessages(run.Input, cfg.SystemPrompt)
	if err != nil {
		return nil, err
	}

	provider := e.provider
	if provider == nil {
		provider = DefaultChatModelProvider
	}
	chatModel, configured, err := provider(ctx, cfg.ModelID)
	if err != nil {
		return nil, err
	}
	if !configured || chatModel == nil {
		return nil, fmt.Errorf("agent thread chat model is not configured")
	}

	resp, err := chatModel.Generate(ctx, messages, modelExecutorOptions(cfg)...)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("agent thread chat model returned empty response")
	}

	message := strings.TrimSpace(resp.Content)
	if message == "" {
		return nil, fmt.Errorf("agent thread chat model returned empty response")
	}

	return &RunExecutionResult{
		Message:  message,
		Metadata: modelExecutorMetadataWithUsage(cfg, resp),
	}, nil
}

type modelExecutorRunInput struct {
	Messages []*schema.Message `json:"messages"`
	Message  string            `json:"message"`
}

type modelExecutorConfig struct {
	ModelID          int64
	ModelName        string
	Temperature      *float32
	MaxTokens        *int
	TopP             *float32
	SystemPrompt     string
	AgentName        string
	AgentDescription string
	MaxIterations    int
}

func parseModelExecutorMessages(rawInput, systemPrompt string) ([]*schema.Message, error) {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return nil, fmt.Errorf("run input messages are required")
	}

	var input modelExecutorRunInput
	if err := json.Unmarshal([]byte(rawInput), &input); err != nil {
		return nil, fmt.Errorf("parse run input failed: %w", err)
	}

	messages := make([]*schema.Message, 0, len(input.Messages)+1)
	if prompt := strings.TrimSpace(systemPrompt); prompt != "" {
		messages = append(messages, schema.SystemMessage(prompt))
	}

	for _, item := range input.Messages {
		message, err := normalizeModelExecutorMessage(item)
		if err != nil {
			return nil, err
		}
		if message != nil {
			messages = append(messages, message)
		}
	}

	if len(messages) == 0 || (len(messages) == 1 && messages[0].Role == schema.System) {
		if message := strings.TrimSpace(input.Message); message != "" {
			messages = append(messages, schema.UserMessage(message))
		}
	}

	hasUserVisibleMessage := false
	for _, message := range messages {
		if message != nil &&
			message.Role != schema.System &&
			hasModelExecutorMessageContent(message) {
			hasUserVisibleMessage = true
			break
		}
	}
	if !hasUserVisibleMessage {
		return nil, fmt.Errorf("run input messages are required")
	}

	return messages, nil
}

func normalizeModelExecutorMessage(
	message *schema.Message,
) (*schema.Message, error) {
	if message == nil {
		return nil, nil
	}

	role := strings.ToLower(strings.TrimSpace(string(message.Role)))
	if role == "" {
		role = string(MessageRoleUser)
	}
	message.Role = schema.RoleType(role)
	message.Content = strings.TrimSpace(message.Content)
	message.Name = strings.TrimSpace(message.Name)
	message.ToolCallID = strings.TrimSpace(message.ToolCallID)
	message.ToolName = strings.TrimSpace(message.ToolName)

	switch role {
	case string(MessageRoleSystem):
	case string(MessageRoleUser):
	case string(MessageRoleAssistant):
	case string(MessageRoleTool):
	default:
		return nil, fmt.Errorf(
			"unsupported run input message role: %s",
			message.Role,
		)
	}
	if !hasModelExecutorMessageContent(message) {
		return nil, nil
	}
	return message, nil
}

func hasModelExecutorMessageContent(message *schema.Message) bool {
	if message == nil {
		return false
	}
	return strings.TrimSpace(message.Content) != "" ||
		strings.TrimSpace(message.ReasoningContent) != "" ||
		len(message.ToolCalls) > 0 ||
		len(message.MultiContent) > 0 ||
		len(message.UserInputMultiContent) > 0 ||
		len(message.AssistantGenMultiContent) > 0
}

func parseModelExecutorConfig(rawConfig string) (modelExecutorConfig, error) {
	cfg := modelExecutorConfig{}
	rawConfig = strings.TrimSpace(rawConfig)
	if rawConfig == "" {
		return cfg, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
		return cfg, fmt.Errorf("parse run config failed: %w", err)
	}

	cfg.ModelID = firstConfigInt64(payload, "model_id", "modelId", "model_type", "modelType")
	cfg.ModelName = firstConfigString(payload, "model_name", "modelName")
	cfg.Temperature = firstConfigFloat32(payload, "temperature")
	if maxTokens := firstConfigInt64(payload, "max_tokens", "maxTokens"); maxTokens > 0 {
		v := int(maxTokens)
		cfg.MaxTokens = &v
	}
	cfg.TopP = firstConfigFloat32(payload, "top_p", "topP")
	cfg.SystemPrompt = firstConfigString(payload, "system_prompt", "systemPrompt")
	cfg.AgentName = firstConfigString(payload, "agent_name", "agentName")
	cfg.AgentDescription = firstConfigString(payload, "agent_description", "agentDescription")
	if maxIterations := firstConfigInt64(payload, "max_iterations", "maxIterations"); maxIterations > 0 {
		cfg.MaxIterations = int(maxIterations)
	}

	return cfg, nil
}

func modelExecutorOptions(cfg modelExecutorConfig) []model.Option {
	opts := make([]model.Option, 0, 4)
	if cfg.ModelName != "" {
		opts = append(opts, model.WithModel(cfg.ModelName))
	}
	if cfg.Temperature != nil {
		opts = append(opts, model.WithTemperature(*cfg.Temperature))
	}
	if cfg.MaxTokens != nil {
		opts = append(opts, model.WithMaxTokens(*cfg.MaxTokens))
	}
	if cfg.TopP != nil {
		opts = append(opts, model.WithTopP(*cfg.TopP))
	}

	return opts
}

func modelExecutorMetadata(cfg modelExecutorConfig) string {
	payload := map[string]any{
		"source": "model_executor",
	}
	if cfg.ModelID > 0 {
		payload["model_id"] = cfg.ModelID
	}
	if cfg.ModelName != "" {
		payload["model_name"] = cfg.ModelName
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return `{"source":"model_executor"}`
	}

	return string(bytes)
}

func modelExecutorMetadataWithUsage(
	cfg modelExecutorConfig,
	message *schema.Message,
) string {
	usage := (*schema.TokenUsage)(nil)
	if message != nil && message.ResponseMeta != nil {
		usage = message.ResponseMeta.Usage
	}
	if usage == nil {
		return modelExecutorMetadata(cfg)
	}

	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	payload := map[string]any{
		"source": "model_executor",
		"usage": map[string]any{
			"source":        string(TokenUsageSourceLeadAgent),
			"model_name":    cfg.ModelName,
			"input_tokens":  usage.PromptTokens,
			"output_tokens": usage.CompletionTokens,
			"total_tokens":  totalTokens,
			"raw_usage": map[string]any{
				"prompt_tokens":     usage.PromptTokens,
				"completion_tokens": usage.CompletionTokens,
				"total_tokens":      totalTokens,
			},
		},
	}
	if cfg.ModelID > 0 {
		payload["model_id"] = cfg.ModelID
	}
	if cfg.ModelName != "" {
		payload["model_name"] = cfg.ModelName
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return modelExecutorMetadata(cfg)
	}
	return string(bytes)
}

func firstConfigString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}

		switch v := value.(type) {
		case string:
			if text := strings.TrimSpace(v); text != "" {
				return text
			}
		}
	}

	return ""
}

func firstConfigInt64(payload map[string]any, keys ...string) int64 {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if parsed, ok := configInt64(value); ok {
			return parsed
		}
	}

	return 0
}

func firstConfigFloat32(payload map[string]any, keys ...string) *float32 {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if parsed, ok := configFloat32(value); ok {
			return &parsed
		}
	}

	return nil
}

func configInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return int64(v), true
		}
	case int:
		if v > 0 {
			return int64(v), true
		}
	case int64:
		if v > 0 {
			return v, true
		}
	case json.Number:
		parsed, err := v.Int64()
		if err == nil && parsed > 0 {
			return parsed, true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}

	return 0, false
}

func configFloat32(value any) (float32, bool) {
	switch v := value.(type) {
	case float64:
		return float32(v), true
	case float32:
		return v, true
	case int:
		return float32(v), true
	case int64:
		return float32(v), true
	case json.Number:
		parsed, err := v.Float64()
		if err == nil {
			return float32(parsed), true
		}
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 32)
		if err == nil {
			return float32(parsed), true
		}
	}

	return 0, false
}
