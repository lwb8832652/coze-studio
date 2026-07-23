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

	arkmodel "github.com/cloudwego/eino-ext/components/model/ark"
	deepseekmodel "github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	arkruntime "github.com/volcengine/volcengine-go-sdk/service/arkruntime/model"
)

const (
	defaultRunTitleMaxWords    = 6
	defaultRunTitleMaxChars    = 60
	defaultRunTitlePromptChars = 500
	defaultRunTitleMaxTokens   = 256
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
		runTitleModelOptions(chatModel, cfg)...,
	)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("agent thread title model returned empty response")
	}
	return normalizeGeneratedThreadTitleWithLimits(
		resp.Content,
		cfg.MaxWords,
		cfg.MaxChars,
	), nil
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
	var activatedResources map[string]struct{}
	if input.Run != nil {
		activatedResources = taskTitleActivatedResources(input.Run.Config)
	}
	userMessage := truncateRunTitlePromptText(
		cleanRunTitlePromptText(input.UserMessage, activatedResources),
		defaultRunTitlePromptChars,
	)
	assistantMessage := truncateRunTitlePromptText(
		cleanRunTitlePromptText(
			generatedThreadTitleThinkTagRE.ReplaceAllString(
				strings.TrimSpace(input.AssistantMessage),
				"",
			),
			activatedResources,
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

func cleanRunTitlePromptText(
	text string,
	activatedResources map[string]struct{},
) string {
	source := strings.TrimSpace(text)
	cleaned, _ := cleanTaskTitleSourceDetails(source, activatedResources)
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

func runTitleModelOptions(
	chatModel model.BaseChatModel,
	cfg runTitleGenerationConfig,
) []model.Option {
	options := make([]model.Option, 0, 3)
	if cfg.ModelName != "" {
		options = append(options, model.WithModel(cfg.ModelName))
	}
	if cfg.MaxTokens > 0 {
		options = append(options, model.WithMaxTokens(cfg.MaxTokens))
	}
	switch chatModel.(type) {
	case *arkmodel.ChatModel:
		options = append(options, arkmodel.WithThinking(&arkruntime.Thinking{
			Type: arkruntime.ThinkingTypeDisabled,
		}))
	case *deepseekmodel.ChatModel:
		options = append(options, deepseekmodel.WithExtraFields(
			map[string]interface{}{
				"thinking": map[string]interface{}{
					"type": "disabled",
				},
			},
		))
	}
	return options
}

func normalizeGeneratedThreadTitleWithLimit(title string, limit int) string {
	return normalizeGeneratedThreadTitleWithLimits(title, 0, limit)
}

func normalizeGeneratedThreadTitleWithLimits(
	title string,
	maxWords int,
	maxChars int,
) string {
	if maxChars <= 0 {
		maxChars = defaultRunTitleMaxChars
	}
	title = strings.TrimSpace(title)
	title = generatedThreadTitleThinkTagRE.ReplaceAllString(title, "")
	title = stripIndependentASCIIAtMentions(title)
	title = cleanTaskTitleSource(title)
	for {
		next := strings.TrimSpace(title)
		next = strings.Trim(next, "\"'“”‘’")
		next = strings.TrimSpace(next)
		next = strings.TrimRight(next, "，。,.；;:：")
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
	if maxWords > 0 {
		words := strings.Fields(title)
		if len(words) > maxWords {
			title = strings.Join(words[:maxWords], " ")
		}
	}
	runes := []rune(title)
	if len(runes) > maxChars {
		return string(runes[:maxChars])
	}
	return title
}

func stripIndependentASCIIAtMentions(text string) string {
	runes := []rune(text)
	cleaned := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); {
		if runes[i] == '@' &&
			(i == 0 || !isASCIIAtMentionWordRune(runes[i-1])) {
			end := i + 1
			for end < len(runes) && isASCIIResourceMarkerRune(runes[end]) {
				end++
			}
			if end > i+1 {
				i = end
				continue
			}
		}
		cleaned = append(cleaned, runes[i])
		i++
	}
	return strings.Join(strings.Fields(string(cleaned)), " ")
}

func isASCIIAtMentionWordRune(r rune) bool {
	return r >= 'a' && r <= 'z' ||
		r >= 'A' && r <= 'Z' ||
		r >= '0' && r <= '9' ||
		r == '_' || r == '.' || r == '+' || r == '-'
}
