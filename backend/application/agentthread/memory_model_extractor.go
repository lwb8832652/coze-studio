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

	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
)

const (
	defaultModelMemoryExtractorMaxFacts = 16
	modelMemoryExtractorInstruction     = "Extract durable memory facts from the transcript. Return JSON only with shape {\"facts\":[{\"key\":\"stable_key\",\"scope\":\"long_term|thread|run\",\"content\":\"short fact\",\"metadata\":{},\"score\":0.0,\"confidence\":0.0}]}. Include only facts that are useful for future task execution, preferences, constraints, or stable user/project context. Do not include tool outputs, credentials, URLs, filenames, object keys, raw provider payloads, or transient chit-chat."
)

const (
	agentMemoryExtractorEnabledEnv     = "AGENT_MEMORY_EXTRACTOR_ENABLED"
	agentMemoryExtractorModelIDEnv     = "AGENT_MEMORY_EXTRACTOR_MODEL_ID"
	agentMemoryExtractorModelNameEnv   = "AGENT_MEMORY_EXTRACTOR_MODEL_NAME"
	agentMemoryExtractorTemperatureEnv = "AGENT_MEMORY_EXTRACTOR_TEMPERATURE"
	agentMemoryExtractorTopPEnv        = "AGENT_MEMORY_EXTRACTOR_TOP_P"
	agentMemoryExtractorMaxTokensEnv   = "AGENT_MEMORY_EXTRACTOR_MAX_TOKENS"
	agentMemoryExtractorMaxFactsEnv    = "AGENT_MEMORY_EXTRACTOR_MAX_FACTS"
)

type ModelMemoryExtractorOptions struct {
	ModelID        int64
	ModelName      string
	Temperature    *float32
	MaxTokens      *int
	TopP           *float32
	MaxFacts       int
	UsageCollector UsageCollector
}

type ModelMemoryExtractor struct {
	provider ChatModelProvider
	options  ModelMemoryExtractorOptions
}

func NewModelMemoryExtractor(
	provider ChatModelProvider,
	options ModelMemoryExtractorOptions,
) *ModelMemoryExtractor {
	if provider == nil {
		provider = DefaultChatModelProvider
	}
	if options.MaxFacts <= 0 {
		options.MaxFacts = defaultModelMemoryExtractorMaxFacts
	}

	return &ModelMemoryExtractor{
		provider: provider,
		options:  options,
	}
}

func NewModelMemoryExtractorFromEnv(
	collector UsageCollector,
) *ModelMemoryExtractor {
	if !envkey.GetBoolD(agentMemoryExtractorEnabledEnv, false) {
		return nil
	}
	options := ModelMemoryExtractorOptions{
		ModelID:        memoryExtractorEnvInt64(agentMemoryExtractorModelIDEnv, 0),
		ModelName:      strings.TrimSpace(envkey.GetStringD(agentMemoryExtractorModelNameEnv, "")),
		MaxFacts:       envkey.GetIntD(agentMemoryExtractorMaxFactsEnv, defaultModelMemoryExtractorMaxFacts),
		UsageCollector: collector,
	}
	if maxTokens := envkey.GetIntD(agentMemoryExtractorMaxTokensEnv, 0); maxTokens > 0 {
		options.MaxTokens = &maxTokens
	}
	if temperature, ok := memoryExtractorEnvFloat32(agentMemoryExtractorTemperatureEnv); ok {
		options.Temperature = &temperature
	}
	if topP, ok := memoryExtractorEnvFloat32(agentMemoryExtractorTopPEnv); ok {
		options.TopP = &topP
	}

	return NewModelMemoryExtractor(DefaultChatModelProvider, options)
}

func (e *ModelMemoryExtractor) ExtractMemories(
	ctx context.Context,
	req MemoryExtractionRequest,
) ([]MemoryExtractionFact, error) {
	if e == nil {
		return nil, fmt.Errorf("model memory extractor is required")
	}
	provider := e.provider
	if provider == nil {
		provider = DefaultChatModelProvider
	}
	chatModel, configured, err := provider(ctx, e.options.ModelID)
	if err != nil {
		return nil, fmt.Errorf("resolve memory extraction model: %w", err)
	}
	if !configured || chatModel == nil {
		return nil, fmt.Errorf("memory extraction model is not configured")
	}

	resp, err := chatModel.Generate(ctx, e.messages(req), e.modelOptions()...)
	if err != nil {
		return nil, fmt.Errorf("run memory extraction model: %w", err)
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil, fmt.Errorf("memory extraction model returned empty response")
	}
	if err := e.recordUsage(ctx, req, resp); err != nil {
		return nil, err
	}

	facts, err := parseModelMemoryExtractionFacts(resp.Content, e.options.MaxFacts)
	if err != nil {
		return nil, err
	}
	return facts, nil
}

func (e *ModelMemoryExtractor) messages(req MemoryExtractionRequest) []*schema.Message {
	user := schema.UserMessage(fmt.Sprintf(
		"Transcript snapshot metadata:\nkind=%s\nsnapshot_id=%d\nmessage_count=%d\ndigest=%s\nmetadata=%s\n\nTranscript messages JSON:\n%s",
		req.Kind,
		req.SnapshotID,
		req.MessageCount,
		req.Digest,
		strings.TrimSpace(req.Metadata),
		strings.TrimSpace(req.Messages),
	))
	user.Extra = map[string]any{
		"memory_model_id": e.options.ModelID,
		"snapshot_id":     req.SnapshotID,
		"thread_id":       req.ThreadID,
		"run_id":          req.RunID,
	}

	return []*schema.Message{
		schema.SystemMessage(modelMemoryExtractorInstruction),
		user,
	}
}

func (e *ModelMemoryExtractor) modelOptions() []model.Option {
	options := make([]model.Option, 0, 4)
	if strings.TrimSpace(e.options.ModelName) != "" {
		options = append(options, model.WithModel(strings.TrimSpace(e.options.ModelName)))
	}
	if e.options.Temperature != nil {
		options = append(options, model.WithTemperature(*e.options.Temperature))
	}
	if e.options.MaxTokens != nil {
		options = append(options, model.WithMaxTokens(*e.options.MaxTokens))
	}
	if e.options.TopP != nil {
		options = append(options, model.WithTopP(*e.options.TopP))
	}
	return options
}

func (e *ModelMemoryExtractor) recordUsage(
	ctx context.Context,
	req MemoryExtractionRequest,
	message *schema.Message,
) error {
	if e == nil || e.options.UsageCollector == nil || message == nil ||
		message.ResponseMeta == nil || message.ResponseMeta.Usage == nil {
		return nil
	}
	usage := message.ResponseMeta.Usage
	raw, err := json.Marshal(map[string]any{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
		"cached_tokens":     usage.PromptTokenDetails.CachedTokens,
		"reasoning_tokens":  usage.CompletionTokensDetails.ReasoningTokens,
	})
	if err != nil {
		return fmt.Errorf("marshal memory extraction usage: %w", err)
	}
	stepID := "memory_extract:" + strconv.FormatInt(req.SnapshotID, 10)
	modelName := strings.TrimSpace(e.options.ModelName)
	metadata, err := json.Marshal(map[string]any{
		"finish_reason":   strings.TrimSpace(message.ResponseMeta.FinishReason),
		"idempotency_key": memoryExtractionUsageIdempotencyKey(req, modelName),
		"kind":            req.Kind,
		"model_id":        e.options.ModelID,
		"model_name":      modelName,
		"run_id":          req.RunID,
		"schema":          "coze.memory_extraction_usage.v1",
		"snapshot_id":     req.SnapshotID,
		"source":          "model_memory_extractor",
		"thread_id":       req.ThreadID,
		"usage_kind":      "memory_extraction",
	})
	if err != nil {
		return fmt.Errorf("marshal memory extraction usage metadata: %w", err)
	}

	err = e.options.UsageCollector.Record(ctx, &RunSummary{
		RunID:    req.RunID,
		ThreadID: req.ThreadID,
		SpaceID:  req.SpaceID,
	}, AgentTokenUsage{
		Source:       TokenUsageSourceMiddleware,
		StepID:       stepID,
		StepName:     "memory_extractor",
		ModelName:    modelName,
		InputTokens:  int64(usage.PromptTokens),
		OutputTokens: int64(usage.CompletionTokens),
		TotalTokens:  int64(usage.TotalTokens),
		RawUsage:     string(raw),
		Metadata:     string(metadata),
	})
	if err != nil {
		return fmt.Errorf("record memory extraction usage: %w", err)
	}
	return nil
}

func memoryExtractionUsageIdempotencyKey(
	req MemoryExtractionRequest,
	modelName string,
) string {
	parts := []string{
		"memory_extraction",
		strconv.FormatInt(req.RunID, 10),
		strconv.FormatInt(req.SnapshotID, 10),
		strings.TrimSpace(req.Digest),
		strings.TrimSpace(modelName),
	}
	return strings.Join(parts, ":")
}

type modelMemoryExtractionPayload struct {
	Facts []modelMemoryExtractionFact `json:"facts"`
}

type modelMemoryExtractionFact struct {
	Key                  string          `json:"key"`
	Scope                string          `json:"scope"`
	Content              string          `json:"content"`
	Metadata             json.RawMessage `json:"metadata"`
	Score                float64         `json:"score"`
	Confidence           float64         `json:"confidence"`
	SourceType           string          `json:"source_type"`
	SourceID             string          `json:"source_id"`
	CorrectionOfMemoryID int64           `json:"correction_of_memory_id"`
	CorrectedAt          int64           `json:"corrected_at"`
	ExpiresAt            int64           `json:"expires_at"`
}

func parseModelMemoryExtractionFacts(raw string, maxFacts int) ([]MemoryExtractionFact, error) {
	var payload modelMemoryExtractionPayload
	if err := json.Unmarshal([]byte(modelMemoryJSONPayload(raw)), &payload); err != nil {
		return nil, fmt.Errorf("decode memory extraction model output: %w", err)
	}
	if maxFacts <= 0 {
		maxFacts = defaultModelMemoryExtractorMaxFacts
	}
	facts := make([]MemoryExtractionFact, 0, len(payload.Facts))
	for _, item := range payload.Facts {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		facts = append(facts, MemoryExtractionFact{
			Key:                  strings.TrimSpace(item.Key),
			Scope:                memoryFlushFactScope(MemoryScope(strings.TrimSpace(item.Scope))),
			Content:              content,
			Metadata:             modelMemoryFactMetadata(item.Metadata),
			Score:                clampModelMemoryFloat(item.Score),
			Confidence:           clampModelMemoryFloat(item.Confidence),
			SourceType:           strings.TrimSpace(item.SourceType),
			SourceID:             strings.TrimSpace(item.SourceID),
			CorrectionOfMemoryID: item.CorrectionOfMemoryID,
			CorrectedAt:          item.CorrectedAt,
			ExpiresAt:            item.ExpiresAt,
		})
		if len(facts) >= maxFacts {
			break
		}
	}
	return facts, nil
}

func modelMemoryJSONPayload(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSpace(raw)
		raw = strings.TrimSuffix(raw, "```")
	}
	return strings.TrimSpace(raw)
}

func modelMemoryFactMetadata(raw json.RawMessage) string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return "{}"
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		text = strings.TrimSpace(text)
		if text != "" && json.Valid([]byte(text)) {
			return text
		}
		return "{}"
	}
	if json.Valid(raw) {
		return string(raw)
	}
	return "{}"
}

func clampModelMemoryFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func memoryExtractorEnvInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(envkey.GetStringD(key, ""))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func memoryExtractorEnvFloat32(key string) (float32, bool) {
	value := strings.TrimSpace(envkey.GetStringD(key, ""))
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(value, 32)
	if err != nil {
		return 0, false
	}
	return float32(parsed), true
}
