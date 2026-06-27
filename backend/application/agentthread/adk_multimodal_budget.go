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
	"errors"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	adkMultimodalBudgetCategory = "multimodal"
	adkFileBudgetCategory       = "file"
)

type ADKMultimodalProjection struct {
	Messages                  []*schema.Message
	KeptMultimodalParts       int
	OmittedMultimodalParts    int
	KeptFileParts             int
	OmittedFileParts          int
	EstimatedMultimodalTokens int
	EstimatedFileTokens       int
}

type ADKProtectedMultimodalBudgetError struct {
	Category        string
	EstimatedTokens int
	TokenLimit      int
	PartCount       int
}

func (e *ADKProtectedMultimodalBudgetError) Error() string {
	if e == nil {
		return "latest user attachment turn exceeds context budget"
	}
	return fmt.Sprintf(
		"latest user attachment turn exceeds %s history token budget %d: estimated %d tokens across %d part(s)",
		e.Category,
		e.TokenLimit,
		e.EstimatedTokens,
		e.PartCount,
	)
}

type adkMultimodalProjectionContextKey struct{}

type ADKMultimodalBudgetMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run       *RunSummary
	budget    ADKContextBudget
	eventSink RunEventSink
}

func NewADKMultimodalBudgetMiddleware(
	run *RunSummary,
	budget ADKContextBudget,
	eventSink RunEventSink,
) (*ADKMultimodalBudgetMiddleware, error) {
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if budget.MultimodalHistoryTokens <= 0 {
		return nil, fmt.Errorf(
			"multimodal history token budget must be positive",
		)
	}
	if budget.FileHistoryTokens <= 0 {
		return nil, fmt.Errorf("file history token budget must be positive")
	}
	return &ADKMultimodalBudgetMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		run:                          run,
		budget:                       budget,
		eventSink:                    eventSink,
	}, nil
}

func (m *ADKMultimodalBudgetMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil || m.run == nil {
		return nil, nil, fmt.Errorf(
			"eino adk multimodal budget middleware is invalid",
		)
	}
	if state == nil {
		return ctx, state, nil
	}
	projection, err := projectADKMultimodalHistory(
		state.Messages,
		m.budget,
	)
	if err != nil {
		m.emitBudgetExceeded(ctx, "model", err)
		return ctx, state, err
	}
	m.emitPruned(ctx, "model", projection)
	return context.WithValue(
		ctx,
		adkMultimodalProjectionContextKey{},
		projection,
	), state, nil
}

func (m *ADKMultimodalBudgetMiddleware) WrapModel(
	_ context.Context,
	base model.BaseChatModel,
	_ *adk.ModelContext,
) (model.BaseChatModel, error) {
	if base == nil {
		return nil, fmt.Errorf("eino adk multimodal budget model is required")
	}
	return &adkMultimodalProjectingModel{
		base:   base,
		budget: m.budget,
	}, nil
}

func (m *ADKMultimodalBudgetMiddleware) buildSummarizationModelInput(
	ctx context.Context,
	systemInstruction *schema.Message,
	userInstruction *schema.Message,
	originalMessages []*schema.Message,
) ([]*schema.Message, error) {
	contextStart := 0
	for contextStart < len(originalMessages) {
		message := originalMessages[contextStart]
		if message == nil || message.Role != schema.System {
			break
		}
		contextStart++
	}
	projection, err := projectADKMultimodalHistory(
		originalMessages[contextStart:],
		m.budget,
	)
	if err != nil {
		m.emitBudgetExceeded(ctx, "summarization", err)
		return nil, err
	}
	m.emitPruned(ctx, "summarization", projection)

	input := make(
		[]*schema.Message,
		0,
		len(projection.Messages)+2,
	)
	input = append(input, systemInstruction)
	input = append(input, projection.Messages...)
	input = append(input, userInstruction)
	return input, nil
}

func (m *ADKMultimodalBudgetMiddleware) emitPruned(
	ctx context.Context,
	phase string,
	projection *ADKMultimodalProjection,
) {
	if projection == nil ||
		(projection.OmittedMultimodalParts == 0 &&
			projection.OmittedFileParts == 0) {
		return
	}
	emitRunEvent(ctx, m.eventSink, RunEvent{
		ThreadID:  m.run.ThreadID,
		RunID:     m.run.RunID,
		EventType: "context.multimodal_pruned",
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"phase":                       phase,
			"kept_multimodal_parts":       projection.KeptMultimodalParts,
			"omitted_multimodal_parts":    projection.OmittedMultimodalParts,
			"kept_file_parts":             projection.KeptFileParts,
			"omitted_file_parts":          projection.OmittedFileParts,
			"estimated_multimodal_tokens": projection.EstimatedMultimodalTokens,
			"estimated_file_tokens":       projection.EstimatedFileTokens,
			"multimodal_token_limit":      m.budget.MultimodalHistoryTokens,
			"file_token_limit":            m.budget.FileHistoryTokens,
		}),
	})
}

func (m *ADKMultimodalBudgetMiddleware) emitBudgetExceeded(
	ctx context.Context,
	phase string,
	err error,
) {
	payload := map[string]any{
		"phase":                  phase,
		"multimodal_token_limit": m.budget.MultimodalHistoryTokens,
		"file_token_limit":       m.budget.FileHistoryTokens,
	}
	var budgetErr *ADKProtectedMultimodalBudgetError
	if errors.As(err, &budgetErr) {
		payload["category"] = budgetErr.Category
		payload["estimated_tokens"] = budgetErr.EstimatedTokens
		payload["token_limit"] = budgetErr.TokenLimit
		payload["part_count"] = budgetErr.PartCount
	} else if err != nil {
		payload["error"] = err.Error()
	}
	emitRunEvent(ctx, m.eventSink, RunEvent{
		ThreadID:  m.run.ThreadID,
		RunID:     m.run.RunID,
		EventType: "context.multimodal_budget_exceeded",
		Payload:   encodeRunEventPayload(ctx, payload),
	})
}

type adkMultimodalProjectingModel struct {
	base   model.BaseChatModel
	budget ADKContextBudget
}

func (m *adkMultimodalProjectingModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	projected, err := m.projectedInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return m.base.Generate(ctx, projected, options...)
}

func (m *adkMultimodalProjectingModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	projected, err := m.projectedInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return m.base.Stream(ctx, projected, options...)
}

func (m *adkMultimodalProjectingModel) projectedInput(
	ctx context.Context,
	input []*schema.Message,
) ([]*schema.Message, error) {
	if projection, ok := ctx.Value(
		adkMultimodalProjectionContextKey{},
	).(*ADKMultimodalProjection); ok && projection != nil {
		return projection.Messages, nil
	}
	projection, err := projectADKMultimodalHistory(input, m.budget)
	if err != nil {
		return nil, err
	}
	return projection.Messages, nil
}

func projectADKMultimodalHistory(
	messages []*schema.Message,
	budget ADKContextBudget,
) (*ADKMultimodalProjection, error) {
	if budget.MultimodalHistoryTokens <= 0 {
		return nil, fmt.Errorf(
			"multimodal history token budget must be positive",
		)
	}
	if budget.FileHistoryTokens <= 0 {
		return nil, fmt.Errorf("file history token budget must be positive")
	}

	protectedIndex := latestADKUserMessageIndex(messages)
	protected := adkMessageAttachmentBudget(nil)
	if protectedIndex >= 0 {
		protected = adkMessageAttachmentBudget(messages[protectedIndex])
	}
	if protected.multimodalTokens > budget.MultimodalHistoryTokens {
		return nil, &ADKProtectedMultimodalBudgetError{
			Category:        adkMultimodalBudgetCategory,
			EstimatedTokens: protected.multimodalTokens,
			TokenLimit:      budget.MultimodalHistoryTokens,
			PartCount:       protected.multimodalParts,
		}
	}
	if protected.fileTokens > budget.FileHistoryTokens {
		return nil, &ADKProtectedMultimodalBudgetError{
			Category:        adkFileBudgetCategory,
			EstimatedTokens: protected.fileTokens,
			TokenLimit:      budget.FileHistoryTokens,
			PartCount:       protected.fileParts,
		}
	}

	projected := cloneADKMessagesForProjection(messages)
	result := &ADKMultimodalProjection{
		Messages:                  projected,
		KeptMultimodalParts:       protected.multimodalParts,
		KeptFileParts:             protected.fileParts,
		EstimatedMultimodalTokens: protected.multimodalTokens,
		EstimatedFileTokens:       protected.fileTokens,
	}
	usedMultimodal := protected.multimodalTokens
	usedFiles := protected.fileTokens
	for messageIndex := len(projected) - 1; messageIndex >= 0; messageIndex-- {
		if messageIndex == protectedIndex || projected[messageIndex] == nil {
			continue
		}
		projectADKMessageParts(
			projected[messageIndex],
			budget,
			&usedMultimodal,
			&usedFiles,
			result,
		)
	}
	result.EstimatedMultimodalTokens = usedMultimodal
	result.EstimatedFileTokens = usedFiles
	return result, nil
}

type adkAttachmentBudget struct {
	multimodalTokens int
	multimodalParts  int
	fileTokens       int
	fileParts        int
}

func adkMessageAttachmentBudget(
	message *schema.Message,
) adkAttachmentBudget {
	budget := adkAttachmentBudget{}
	if message == nil {
		return budget
	}
	for _, part := range message.MultiContent {
		category, tokens, ok := adkLegacyPartBudget(part)
		budget.add(category, tokens, ok)
	}
	for _, part := range message.UserInputMultiContent {
		category, tokens, ok := adkInputPartBudget(part)
		budget.add(category, tokens, ok)
	}
	for _, part := range message.AssistantGenMultiContent {
		category, tokens, ok := adkOutputPartBudget(part)
		budget.add(category, tokens, ok)
	}
	return budget
}

func (b *adkAttachmentBudget) add(category string, tokens int, ok bool) {
	if b == nil || !ok {
		return
	}
	if category == adkFileBudgetCategory {
		b.fileTokens += tokens
		b.fileParts++
		return
	}
	b.multimodalTokens += tokens
	b.multimodalParts++
}

func cloneADKMessagesForProjection(
	messages []*schema.Message,
) []*schema.Message {
	cloned := make([]*schema.Message, len(messages))
	for index, message := range messages {
		if message == nil {
			continue
		}
		copyMessage := *message
		copyMessage.Extra = cloneADKAnyMap(message.Extra)
		copyMessage.ToolCalls = cloneADKToolCalls(message.ToolCalls)
		copyMessage.MultiContent = cloneADKLegacyParts(message.MultiContent)
		copyMessage.UserInputMultiContent = cloneADKInputParts(
			message.UserInputMultiContent,
		)
		copyMessage.AssistantGenMultiContent = cloneADKOutputParts(
			message.AssistantGenMultiContent,
		)
		cloned[index] = &copyMessage
	}
	return cloned
}

func cloneADKToolCalls(calls []schema.ToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	cloned := make([]schema.ToolCall, len(calls))
	for index, call := range calls {
		cloned[index] = call
		cloned[index].Extra = cloneADKAnyMap(call.Extra)
		if call.Index != nil {
			value := *call.Index
			cloned[index].Index = &value
		}
	}
	return cloned
}

func cloneADKLegacyParts(
	parts []schema.ChatMessagePart,
) []schema.ChatMessagePart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]schema.ChatMessagePart, len(parts))
	for index, part := range parts {
		cloned[index] = part
		if part.ImageURL != nil {
			value := *part.ImageURL
			value.Extra = cloneADKAnyMap(part.ImageURL.Extra)
			cloned[index].ImageURL = &value
		}
		if part.AudioURL != nil {
			value := *part.AudioURL
			value.Extra = cloneADKAnyMap(part.AudioURL.Extra)
			cloned[index].AudioURL = &value
		}
		if part.VideoURL != nil {
			value := *part.VideoURL
			value.Extra = cloneADKAnyMap(part.VideoURL.Extra)
			cloned[index].VideoURL = &value
		}
		if part.FileURL != nil {
			value := *part.FileURL
			value.Extra = cloneADKAnyMap(part.FileURL.Extra)
			cloned[index].FileURL = &value
		}
	}
	return cloned
}

func cloneADKInputParts(
	parts []schema.MessageInputPart,
) []schema.MessageInputPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]schema.MessageInputPart, len(parts))
	for index, part := range parts {
		cloned[index] = part
		cloned[index].Extra = cloneADKAnyMap(part.Extra)
		if part.Image != nil {
			value := *part.Image
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.Image.MessagePartCommon,
			)
			cloned[index].Image = &value
		}
		if part.Audio != nil {
			value := *part.Audio
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.Audio.MessagePartCommon,
			)
			cloned[index].Audio = &value
		}
		if part.Video != nil {
			value := *part.Video
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.Video.MessagePartCommon,
			)
			cloned[index].Video = &value
		}
		if part.File != nil {
			value := *part.File
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.File.MessagePartCommon,
			)
			cloned[index].File = &value
		}
	}
	return cloned
}

func cloneADKOutputParts(
	parts []schema.MessageOutputPart,
) []schema.MessageOutputPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]schema.MessageOutputPart, len(parts))
	for index, part := range parts {
		cloned[index] = part
		cloned[index].Extra = cloneADKAnyMap(part.Extra)
		if part.Image != nil {
			value := *part.Image
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.Image.MessagePartCommon,
			)
			cloned[index].Image = &value
		}
		if part.Audio != nil {
			value := *part.Audio
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.Audio.MessagePartCommon,
			)
			cloned[index].Audio = &value
		}
		if part.Video != nil {
			value := *part.Video
			value.MessagePartCommon = cloneADKMessagePartCommon(
				part.Video.MessagePartCommon,
			)
			cloned[index].Video = &value
		}
		if part.Reasoning != nil {
			value := *part.Reasoning
			cloned[index].Reasoning = &value
		}
		if part.StreamingMeta != nil {
			value := *part.StreamingMeta
			cloned[index].StreamingMeta = &value
		}
	}
	return cloned
}

func cloneADKMessagePartCommon(
	common schema.MessagePartCommon,
) schema.MessagePartCommon {
	cloned := common
	cloned.URL = cloneADKStringPointer(common.URL)
	cloned.Base64Data = cloneADKStringPointer(common.Base64Data)
	cloned.Extra = cloneADKAnyMap(common.Extra)
	return cloned
}

func cloneADKStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneADKAnyMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = cloneADKJSONCompatibleValue(item)
	}
	return cloned
}

func cloneADKJSONCompatibleValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneADKAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneADKJSONCompatibleValue(item)
		}
		return cloned
	default:
		return value
	}
}

func latestADKUserMessageIndex(messages []*schema.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] != nil && messages[index].Role == schema.User {
			return index
		}
	}
	return -1
}

func projectADKMessageParts(
	message *schema.Message,
	budget ADKContextBudget,
	usedMultimodal *int,
	usedFiles *int,
	result *ADKMultimodalProjection,
) {
	for index, part := range message.MultiContent {
		category, tokens, ok := adkLegacyPartBudget(part)
		if !ok {
			continue
		}
		if retainADKAttachmentPart(
			category,
			tokens,
			budget,
			usedMultimodal,
			usedFiles,
			result,
		) {
			continue
		}
		message.MultiContent[index] = schema.ChatMessagePart{
			Type: schema.ChatMessagePartTypeText,
			Text: adkOmittedAttachmentMarker(part.Type, category),
		}
	}
	for index, part := range message.UserInputMultiContent {
		category, tokens, ok := adkInputPartBudget(part)
		if !ok {
			continue
		}
		if retainADKAttachmentPart(
			category,
			tokens,
			budget,
			usedMultimodal,
			usedFiles,
			result,
		) {
			continue
		}
		message.UserInputMultiContent[index] = schema.MessageInputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: adkOmittedAttachmentMarker(part.Type, category),
		}
	}
	for index, part := range message.AssistantGenMultiContent {
		category, tokens, ok := adkOutputPartBudget(part)
		if !ok {
			continue
		}
		if retainADKAttachmentPart(
			category,
			tokens,
			budget,
			usedMultimodal,
			usedFiles,
			result,
		) {
			continue
		}
		message.AssistantGenMultiContent[index] = schema.MessageOutputPart{
			Type: schema.ChatMessagePartTypeText,
			Text: adkOmittedAttachmentMarker(part.Type, category),
		}
	}
}

func retainADKAttachmentPart(
	category string,
	tokens int,
	budget ADKContextBudget,
	usedMultimodal *int,
	usedFiles *int,
	result *ADKMultimodalProjection,
) bool {
	if category == adkFileBudgetCategory {
		if *usedFiles+tokens > budget.FileHistoryTokens {
			result.OmittedFileParts++
			return false
		}
		*usedFiles += tokens
		result.KeptFileParts++
		return true
	}
	if *usedMultimodal+tokens > budget.MultimodalHistoryTokens {
		result.OmittedMultimodalParts++
		return false
	}
	*usedMultimodal += tokens
	result.KeptMultimodalParts++
	return true
}

func adkInputPartBudget(
	part schema.MessageInputPart,
) (string, int, bool) {
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.Image == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory,
			adkImageTokens(part.Image.Detail),
			true
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkAudioTokens, true
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkVideoTokens, true
	case schema.ChatMessagePartTypeFileURL:
		if part.File == nil {
			return "", 0, false
		}
		return adkFileBudgetCategory, adkFileTokens, true
	default:
		return "", 0, false
	}
}

func adkOutputPartBudget(
	part schema.MessageOutputPart,
) (string, int, bool) {
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.Image == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkImageDefaultTokens, true
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkAudioTokens, true
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkVideoTokens, true
	default:
		return "", 0, false
	}
}

func adkLegacyPartBudget(
	part schema.ChatMessagePart,
) (string, int, bool) {
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.ImageURL == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory,
			adkImageTokens(part.ImageURL.Detail),
			true
	case schema.ChatMessagePartTypeAudioURL:
		if part.AudioURL == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkAudioTokens, true
	case schema.ChatMessagePartTypeVideoURL:
		if part.VideoURL == nil {
			return "", 0, false
		}
		return adkMultimodalBudgetCategory, adkVideoTokens, true
	case schema.ChatMessagePartTypeFileURL:
		if part.FileURL == nil {
			return "", 0, false
		}
		return adkFileBudgetCategory, adkFileTokens, true
	default:
		return "", 0, false
	}
}

func adkOmittedAttachmentMarker(
	partType schema.ChatMessagePartType,
	category string,
) string {
	kind := string(partType)
	switch partType {
	case schema.ChatMessagePartTypeImageURL:
		kind = "image"
	case schema.ChatMessagePartTypeAudioURL:
		kind = "audio"
	case schema.ChatMessagePartTypeVideoURL:
		kind = "video"
	case schema.ChatMessagePartTypeFileURL:
		kind = "file"
	}
	return fmt.Sprintf(
		"[historical %s omitted: %s history budget exceeded]",
		kind,
		category,
	)
}
