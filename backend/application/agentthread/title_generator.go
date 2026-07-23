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
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultRunTitleMaxWords    = 6
	defaultRunTitleMaxChars    = 60
	defaultRunTitlePromptChars = 500
	defaultRunTitleMaxTokens   = 96
)

type ModelRunTitleGenerator struct {
	provider ChatModelProvider
}

type runTitleGenerationConfig struct {
	ModelID   int64
	ModelName string
	MaxWords  int
	MaxChars  int
	MaxTokens int
}

func NewModelRunTitleGenerator(provider ChatModelProvider) *ModelRunTitleGenerator {
	if provider == nil {
		provider = DefaultChatModelProvider
	}

	return &ModelRunTitleGenerator{provider: provider}
}

func (g *ModelRunTitleGenerator) GenerateTitle(
	ctx context.Context,
	input RunTitleGenerationInput,
) (string, error) {
	if g == nil {
		return "", fmt.Errorf("model run title generator is required")
	}
	if input.Run == nil {
		return "", fmt.Errorf("run is required")
	}

	cfg, err := runTitleGenerationConfigFromRun(input.Run)
	if err != nil {
		return "", err
	}
	provider := g.provider
	if provider == nil {
		provider = DefaultChatModelProvider
	}
	chatModel, configured, err := provider(ctx, cfg.ModelID)
	if err != nil {
		return "", err
	}
	if !configured || chatModel == nil {
		return "", fmt.Errorf("agent thread title model is not configured")
	}

	resp, err := chatModel.Generate(
		ctx,
		[]*schema.Message{schema.UserMessage(buildRunTitlePrompt(input, cfg))},
		runTitleModelOptions(cfg)...,
	)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("agent thread title model returned empty response")
	}
	return normalizeGeneratedThreadTitleWithLimit(resp.Content, cfg.MaxChars), nil
}

func runTitleGenerationConfigFromRun(
	run *RunSummary,
) (runTitleGenerationConfig, error) {
	cfg := runTitleGenerationConfig{
		MaxWords:  defaultRunTitleMaxWords,
		MaxChars:  defaultRunTitleMaxChars,
		MaxTokens: defaultRunTitleMaxTokens,
	}
	if run == nil {
		return cfg, nil
	}
	modelCfg, err := parseModelExecutorConfig(run.Config)
	if err != nil {
		return cfg, err
	}
	cfg.ModelID = modelCfg.ModelID
	cfg.ModelName = modelCfg.ModelName

	rawConfig := strings.TrimSpace(run.Config)
	if rawConfig == "" {
		return cfg, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &payload); err != nil {
		return cfg, fmt.Errorf("parse run title config failed: %w", err)
	}
	titlePayload := firstConfigMap(
		payload,
		"title",
		"title_generation",
		"titleGeneration",
	)
	if titlePayload == nil {
		return cfg, nil
	}
	if modelID := firstConfigInt64(titlePayload, "model_id", "modelId"); modelID > 0 {
		cfg.ModelID = modelID
	}
	if modelName := firstConfigString(titlePayload, "model_name", "modelName"); modelName != "" {
		cfg.ModelName = modelName
	}
	if maxWords := firstConfigInt64(titlePayload, "max_words", "maxWords"); maxWords > 0 && maxWords <= 20 {
		cfg.MaxWords = int(maxWords)
	}
	if maxChars := firstConfigInt64(titlePayload, "max_chars", "maxChars"); maxChars >= 10 && maxChars <= 200 {
		cfg.MaxChars = int(maxChars)
	}
	if maxTokens := firstConfigInt64(titlePayload, "max_tokens", "maxTokens"); maxTokens > 0 && maxTokens <= 256 {
		cfg.MaxTokens = int(maxTokens)
	}

	return cfg, nil
}

func buildRunTitlePrompt(
	input RunTitleGenerationInput,
	cfg runTitleGenerationConfig,
) string {
	userMessage := truncateRunTitlePromptText(
		cleanRunTitlePromptText(input.UserMessage),
		defaultRunTitlePromptChars,
	)
	assistantMessage := truncateRunTitlePromptText(
		cleanRunTitlePromptText(
			generatedThreadTitleThinkTagRE.ReplaceAllString(
				strings.TrimSpace(input.AssistantMessage),
				"",
			),
		),
		defaultRunTitlePromptChars,
	)

	return fmt.Sprintf(
		"Generate a concise title (max %d words) for this conversation.\nDo not include tool names, skill names, or @mentions.\nUser: %s\nAssistant: %s\n\nReturn ONLY the title, no quotes, no explanation.",
		cfg.MaxWords,
		userMessage,
		assistantMessage,
	)
}

func cleanRunTitlePromptText(text string) string {
	source := strings.TrimSpace(text)
	cleaned := cleanTaskTitleSource(source)
	if cleaned == "" {
		return ""
	}

	runes := []rune(source)
	if len(runes) == 0 {
		return cleaned
	}
	trailing := runes[len(runes)-1]
	if strings.ContainsRune("，。,.；;:：", trailing) &&
		!strings.HasSuffix(cleaned, string(trailing)) {
		return cleaned + string(trailing)
	}
	return cleaned
}

func truncateRunTitlePromptText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

func runTitleModelOptions(cfg runTitleGenerationConfig) []model.Option {
	options := make([]model.Option, 0, 2)
	if cfg.ModelName != "" {
		options = append(options, model.WithModel(cfg.ModelName))
	}
	if cfg.MaxTokens > 0 {
		options = append(options, model.WithMaxTokens(cfg.MaxTokens))
	}
	return options
}

func normalizeGeneratedThreadTitleWithLimit(title string, limit int) string {
	if limit <= 0 {
		limit = defaultRunTitleMaxChars
	}
	title = strings.TrimSpace(title)
	title = generatedThreadTitleThinkTagRE.ReplaceAllString(title, "")
	title = cleanTaskTitleSource(title)
	for {
		next := strings.TrimSpace(title)
		next = strings.Trim(next, "\"'“”‘’")
		next = strings.TrimSpace(next)
		next = strings.Trim(next, "，。,.；;:：")
		next = strings.TrimSpace(next)
		next = strings.Trim(next, "\"'“”‘’")
		next = strings.TrimSpace(next)
		if next == title {
			break
		}
		title = next
	}
	if title == "" {
		return ""
	}
	runes := []rune(title)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return title
}
