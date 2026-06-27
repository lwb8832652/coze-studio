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
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

const (
	defaultADKContextWindowTokens     = 128000
	defaultADKSummarizationTokens     = 120000
	defaultADKSummarizationMessages   = 200
	defaultADKMemoryTokens            = 4000
	defaultADKSkillCatalogTokens      = 4000
	defaultADKSkillContentTokens      = 16000
	defaultADKToolDefinitionTokens    = 12000
	defaultADKMultimodalHistoryTokens = 16000
	defaultADKFileHistoryTokens       = 8000

	adkImageLowTokens     = 256
	adkImageDefaultTokens = 768
	adkImageHighTokens    = 1536
	adkAudioTokens        = 4096
	adkVideoTokens        = 8192
	adkFileTokens         = 2048
)

type ADKContextBudget struct {
	ContextWindowTokens     int
	SummarizationTokens     int
	SummarizationMessages   int
	MemoryTokens            int
	SkillCatalogTokens      int
	SkillContentTokens      int
	ToolDefinitionTokens    int
	MultimodalHistoryTokens int
	FileHistoryTokens       int
}

type adkContextBudgetConfig struct {
	ContextWindowTokens     *int `json:"context_window_tokens"`
	SummarizationTokens     *int `json:"summarization_tokens"`
	SummarizationMessages   *int `json:"summarization_messages"`
	MemoryTokens            *int `json:"memory_tokens"`
	SkillCatalogTokens      *int `json:"skill_catalog_tokens"`
	SkillContentTokens      *int `json:"skill_content_tokens"`
	ToolDefinitionTokens    *int `json:"tool_definition_tokens"`
	MultimodalHistoryTokens *int `json:"multimodal_history_tokens"`
	FileHistoryTokens       *int `json:"file_history_tokens"`
}

func adkContextBudgetFromRun(run *RunSummary) (ADKContextBudget, error) {
	budget := ADKContextBudget{
		ContextWindowTokens:     defaultADKContextWindowTokens,
		SummarizationTokens:     defaultADKSummarizationTokens,
		SummarizationMessages:   defaultADKSummarizationMessages,
		MemoryTokens:            defaultADKMemoryTokens,
		SkillCatalogTokens:      defaultADKSkillCatalogTokens,
		SkillContentTokens:      defaultADKSkillContentTokens,
		ToolDefinitionTokens:    defaultADKToolDefinitionTokens,
		MultimodalHistoryTokens: defaultADKMultimodalHistoryTokens,
		FileHistoryTokens:       defaultADKFileHistoryTokens,
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return budget, nil
	}

	var payload struct {
		ContextBudget adkContextBudgetConfig `json:"context_budget"`
	}
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return ADKContextBudget{}, fmt.Errorf("decode adk context budget: %w", err)
	}

	if payload.ContextBudget.ContextWindowTokens != nil {
		budget.ContextWindowTokens = *payload.ContextBudget.ContextWindowTokens
	}
	if payload.ContextBudget.SummarizationTokens != nil {
		budget.SummarizationTokens = *payload.ContextBudget.SummarizationTokens
	}
	if payload.ContextBudget.SummarizationMessages != nil {
		budget.SummarizationMessages = *payload.ContextBudget.SummarizationMessages
	}
	if payload.ContextBudget.MemoryTokens != nil {
		budget.MemoryTokens = *payload.ContextBudget.MemoryTokens
	}
	if payload.ContextBudget.SkillCatalogTokens != nil {
		budget.SkillCatalogTokens = *payload.ContextBudget.SkillCatalogTokens
	}
	if payload.ContextBudget.SkillContentTokens != nil {
		budget.SkillContentTokens = *payload.ContextBudget.SkillContentTokens
	}
	if payload.ContextBudget.ToolDefinitionTokens != nil {
		budget.ToolDefinitionTokens = *payload.ContextBudget.ToolDefinitionTokens
	}
	if payload.ContextBudget.MultimodalHistoryTokens != nil {
		budget.MultimodalHistoryTokens =
			*payload.ContextBudget.MultimodalHistoryTokens
	}
	if payload.ContextBudget.FileHistoryTokens != nil {
		budget.FileHistoryTokens = *payload.ContextBudget.FileHistoryTokens
	}
	if payload.ContextBudget.SkillCatalogTokens == nil {
		budget.SkillCatalogTokens = clampADKComponentBudget(
			budget.SkillCatalogTokens,
			budget.ContextWindowTokens,
		)
	}
	if payload.ContextBudget.SkillContentTokens == nil {
		budget.SkillContentTokens = clampADKComponentBudget(
			budget.SkillContentTokens,
			budget.ContextWindowTokens,
		)
	}
	if payload.ContextBudget.ToolDefinitionTokens == nil {
		budget.ToolDefinitionTokens = clampADKComponentBudget(
			budget.ToolDefinitionTokens,
			budget.ContextWindowTokens,
		)
	}
	if payload.ContextBudget.MultimodalHistoryTokens == nil {
		budget.MultimodalHistoryTokens = clampADKComponentBudget(
			budget.MultimodalHistoryTokens,
			budget.ContextWindowTokens,
		)
	}
	if payload.ContextBudget.FileHistoryTokens == nil {
		budget.FileHistoryTokens = clampADKComponentBudget(
			budget.FileHistoryTokens,
			budget.ContextWindowTokens,
		)
	}

	if err := validateADKContextBudget(budget); err != nil {
		return ADKContextBudget{}, err
	}
	return budget, nil
}

func clampADKComponentBudget(value, contextWindow int) int {
	if contextWindow > 1 && value >= contextWindow {
		return contextWindow - 1
	}
	return value
}

func validateADKContextBudget(budget ADKContextBudget) error {
	if budget.ContextWindowTokens <= 0 {
		return fmt.Errorf("context window tokens must be positive")
	}
	if budget.SummarizationTokens <= 0 ||
		budget.SummarizationTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"summarization tokens must be positive and below context window tokens",
		)
	}
	if budget.SummarizationMessages <= 0 {
		return fmt.Errorf("summarization messages must be positive")
	}
	if budget.MemoryTokens <= 0 ||
		budget.MemoryTokens >= budget.SummarizationTokens {
		return fmt.Errorf(
			"memory tokens must be positive and below summarization tokens",
		)
	}
	if budget.SkillCatalogTokens <= 0 ||
		budget.SkillCatalogTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"skill catalog tokens must be positive and below context window tokens",
		)
	}
	if budget.SkillContentTokens <= 0 ||
		budget.SkillContentTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"skill content tokens must be positive and below context window tokens",
		)
	}
	if budget.ToolDefinitionTokens <= 0 ||
		budget.ToolDefinitionTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"tool definition tokens must be positive and below context window tokens",
		)
	}
	if budget.MultimodalHistoryTokens <= 0 ||
		budget.MultimodalHistoryTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"multimodal history tokens must be positive and below context window tokens",
		)
	}
	if budget.FileHistoryTokens <= 0 ||
		budget.FileHistoryTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"file history tokens must be positive and below context window tokens",
		)
	}
	return nil
}

func validateADKMemoryBudget(budget ADKContextBudget) error {
	if budget.ContextWindowTokens <= 0 {
		return fmt.Errorf("context window tokens must be positive")
	}
	if budget.SummarizationTokens <= 0 ||
		budget.SummarizationTokens >= budget.ContextWindowTokens {
		return fmt.Errorf(
			"summarization tokens must be positive and below context window tokens",
		)
	}
	if budget.SummarizationMessages <= 0 {
		return fmt.Errorf("summarization messages must be positive")
	}
	if budget.MemoryTokens <= 0 ||
		budget.MemoryTokens >= budget.SummarizationTokens {
		return fmt.Errorf(
			"memory tokens must be positive and below summarization tokens",
		)
	}
	return nil
}

func estimateADKTextTokens(value string) int {
	if value == "" {
		return 0
	}

	asciiRunes := 0
	nonASCIIRunes := 0
	for _, current := range value {
		if current <= 0x7f {
			asciiRunes++
		} else {
			nonASCIIRunes++
		}
	}
	asciiTokens := (asciiRunes + 3) / 4
	if !utf8.ValidString(value) {
		asciiTokens += (len(value) + 3) / 4
	}
	return asciiTokens + nonASCIIRunes
}

func estimateADKMessagesTokens(messages []*schema.Message, tools []*schema.ToolInfo) int {
	estimate := estimateADKMessageTokenBreakdown(messages)
	total := estimate.TextTokens +
		estimate.MultimodalTokens +
		estimate.FileTokens
	for _, info := range tools {
		if info == nil {
			continue
		}
		total += 8
		total += estimateADKTextTokens(info.Name)
		total += estimateADKTextTokens(info.Desc)
		total += estimateADKJSONTokens(info.Extra)
		total += estimateADKJSONTokens(info.ParamsOneOf)
	}
	return total
}

type ADKMessageTokenEstimate struct {
	TextTokens       int
	MultimodalTokens int
	FileTokens       int
}

func estimateADKMessageTokenBreakdown(
	messages []*schema.Message,
) ADKMessageTokenEstimate {
	estimate := ADKMessageTokenEstimate{}
	for _, message := range messages {
		if message == nil {
			continue
		}
		estimate.TextTokens += 4
		estimate.TextTokens += estimateADKTextTokens(string(message.Role))
		estimate.TextTokens += estimateADKTextTokens(message.Content)
		estimate.TextTokens += estimateADKTextTokens(message.Name)
		estimate.TextTokens += estimateADKTextTokens(message.ToolCallID)
		estimate.TextTokens += estimateADKTextTokens(message.ToolName)
		estimate.TextTokens += estimateADKTextTokens(message.ReasoningContent)
		estimate.TextTokens += estimateADKJSONTokens(message.Extra)
		for _, call := range message.ToolCalls {
			estimate.TextTokens += 4
			estimate.TextTokens += estimateADKTextTokens(call.ID)
			estimate.TextTokens += estimateADKTextTokens(call.Type)
			estimate.TextTokens += estimateADKTextTokens(call.Function.Name)
			estimate.TextTokens += estimateADKTextTokens(
				call.Function.Arguments,
			)
			estimate.TextTokens += estimateADKJSONTokens(call.Extra)
		}
		for _, part := range message.MultiContent {
			addADKLegacyPartEstimate(&estimate, part)
		}
		for _, part := range message.UserInputMultiContent {
			addADKInputPartEstimate(&estimate, part)
		}
		for _, part := range message.AssistantGenMultiContent {
			addADKOutputPartEstimate(&estimate, part)
		}
	}
	return estimate
}

func addADKInputPartEstimate(
	estimate *ADKMessageTokenEstimate,
	part schema.MessageInputPart,
) {
	if estimate == nil {
		return
	}
	estimate.TextTokens += estimateADKTextTokens(string(part.Type))
	estimate.TextTokens += estimateADKTextTokens(part.Text)
	estimate.TextTokens += estimateADKJSONTokens(part.Extra)
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.Image != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.Image.MessagePartCommon,
			)
			estimate.MultimodalTokens += adkImageTokens(part.Image.Detail)
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.Audio.MessagePartCommon,
			)
			estimate.MultimodalTokens += adkAudioTokens
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.Video.MessagePartCommon,
			)
			estimate.MultimodalTokens += adkVideoTokens
		}
	case schema.ChatMessagePartTypeFileURL:
		if part.File != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.File.MessagePartCommon,
			)
			estimate.TextTokens += estimateADKTextTokens(part.File.Name)
			estimate.FileTokens += adkFileTokens
		}
	case schema.ChatMessagePartTypeToolSearchResult:
		estimate.TextTokens += estimateADKJSONTokens(part.ToolSearchResult)
	}
}

func addADKOutputPartEstimate(
	estimate *ADKMessageTokenEstimate,
	part schema.MessageOutputPart,
) {
	if estimate == nil {
		return
	}
	estimate.TextTokens += estimateADKTextTokens(string(part.Type))
	estimate.TextTokens += estimateADKTextTokens(part.Text)
	estimate.TextTokens += estimateADKJSONTokens(part.Extra)
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.Image != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.Image.MessagePartCommon,
			)
			estimate.MultimodalTokens += adkImageDefaultTokens
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.Audio.MessagePartCommon,
			)
			estimate.MultimodalTokens += adkAudioTokens
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video != nil {
			estimate.TextTokens += estimateADKMediaMetadataTokens(
				part.Video.MessagePartCommon,
			)
			estimate.MultimodalTokens += adkVideoTokens
		}
	case schema.ChatMessagePartTypeReasoning:
		if part.Reasoning != nil {
			estimate.TextTokens += estimateADKTextTokens(part.Reasoning.Text)
			estimate.TextTokens += estimateADKTextTokens(
				part.Reasoning.Signature,
			)
		}
	}
}

func addADKLegacyPartEstimate(
	estimate *ADKMessageTokenEstimate,
	part schema.ChatMessagePart,
) {
	if estimate == nil {
		return
	}
	estimate.TextTokens += estimateADKTextTokens(string(part.Type))
	estimate.TextTokens += estimateADKTextTokens(part.Text)
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.ImageURL != nil {
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.ImageURL.URL,
			)
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.ImageURL.URI,
			)
			estimate.TextTokens += estimateADKTextTokens(
				part.ImageURL.MIMEType,
			)
			estimate.TextTokens += estimateADKJSONTokens(part.ImageURL.Extra)
			estimate.MultimodalTokens += adkImageTokens(
				part.ImageURL.Detail,
			)
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.AudioURL != nil {
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.AudioURL.URL,
			)
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.AudioURL.URI,
			)
			estimate.TextTokens += estimateADKTextTokens(
				part.AudioURL.MIMEType,
			)
			estimate.TextTokens += estimateADKJSONTokens(part.AudioURL.Extra)
			estimate.MultimodalTokens += adkAudioTokens
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.VideoURL != nil {
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.VideoURL.URL,
			)
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.VideoURL.URI,
			)
			estimate.TextTokens += estimateADKTextTokens(
				part.VideoURL.MIMEType,
			)
			estimate.TextTokens += estimateADKJSONTokens(part.VideoURL.Extra)
			estimate.MultimodalTokens += adkVideoTokens
		}
	case schema.ChatMessagePartTypeFileURL:
		if part.FileURL != nil {
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.FileURL.URL,
			)
			estimate.TextTokens += estimateADKMediaReferenceTokens(
				part.FileURL.URI,
			)
			estimate.TextTokens += estimateADKTextTokens(
				part.FileURL.MIMEType,
			)
			estimate.TextTokens += estimateADKTextTokens(part.FileURL.Name)
			estimate.TextTokens += estimateADKJSONTokens(part.FileURL.Extra)
			estimate.FileTokens += adkFileTokens
		}
	}
}

func estimateADKMediaMetadataTokens(common schema.MessagePartCommon) int {
	total := estimateADKTextTokens(common.MIMEType)
	if common.URL != nil {
		total += estimateADKMediaReferenceTokens(*common.URL)
	}
	total += estimateADKJSONTokens(common.Extra)
	return total
}

func estimateADKMediaReferenceTokens(value string) int {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		if separator := strings.IndexByte(value, ','); separator >= 0 {
			value = value[:separator]
		}
	}
	return estimateADKTextTokens(value)
}

func adkImageTokens(detail schema.ImageURLDetail) int {
	switch detail {
	case schema.ImageURLDetailLow:
		return adkImageLowTokens
	case schema.ImageURLDetailHigh:
		return adkImageHighTokens
	default:
		return adkImageDefaultTokens
	}
}

func estimateADKJSONTokens(value any) int {
	if value == nil {
		return 0
	}
	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" || string(raw) == "[]" {
		return 0
	}
	return estimateADKTextTokens(string(raw))
}
