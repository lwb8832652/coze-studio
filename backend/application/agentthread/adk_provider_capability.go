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
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type ADKProviderCapability string

const (
	ADKProviderCapabilityThinking  ADKProviderCapability = "thinking"
	ADKProviderCapabilityReasoning ADKProviderCapability = "reasoning"
	ADKProviderCapabilityVision    ADKProviderCapability = "vision"
	ADKProviderCapabilityPDF       ADKProviderCapability = "pdf"
	ADKProviderCapabilityFile      ADKProviderCapability = "file"
	ADKProviderCapabilityAudio     ADKProviderCapability = "audio"
	ADKProviderCapabilityVideo     ADKProviderCapability = "video"

	adkProviderCapabilityDowngradedEventType = "model.capability_downgraded"
	adkProviderCapabilityDowngradeSchema     = "coze.provider_capability_downgrade.v1"
)

type ADKProviderCapabilityConfig struct {
	Capabilities ADKModelCapabilities
	Reasoning    ADKReasoningRequest
}

type ADKReasoningRequest struct {
	ReasoningEffort string
	ThinkingEnabled bool
}

type ADKProviderCapabilityError struct {
	Capability ADKProviderCapability
	PartType   string
	Count      int
}

func (e *ADKProviderCapabilityError) Error() string {
	if e == nil {
		return "provider capability unsupported"
	}
	return fmt.Sprintf(
		"provider capability unsupported: %s is required for %s",
		e.Capability,
		e.PartType,
	)
}

type ADKProviderCapabilityMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	config             ADKProviderCapabilityConfig
	run                *RunSummary
	eventSink          RunEventSink
	requestedReasoning ADKReasoningRequest
	effectiveReasoning ADKReasoningRequest
	downgradeOnce      sync.Once
}

func NewADKProviderCapabilityMiddleware(
	run *RunSummary,
	capabilities ADKModelCapabilities,
	runtimeConfigs ...DeerFlowRuntimeConfig,
) (*ADKProviderCapabilityMiddleware, error) {
	config, err := adkProviderCapabilityConfigFromRun(run, capabilities)
	if err != nil {
		return nil, err
	}
	requested := config.Reasoning
	effective := requested
	if len(runtimeConfigs) > 0 {
		requested = runtimeConfigs[0].ExecutionReasoningRequestOr(config.Reasoning)
		effective = effectiveADKReasoningRequest(
			requested,
			capabilities,
		)
	}
	config.Reasoning = effective

	return &ADKProviderCapabilityMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		config:                       config,
		run:                          run,
		requestedReasoning:           requested,
		effectiveReasoning:           effective,
	}, nil
}

func effectiveADKReasoningRequest(
	request ADKReasoningRequest,
	capabilities ADKModelCapabilities,
) ADKReasoningRequest {
	if !capabilities.Reasoning {
		request.ReasoningEffort = ""
	}
	if !capabilities.Thinking {
		request.ThinkingEnabled = false
	}
	return request
}

func (m *ADKProviderCapabilityMiddleware) BeforeAgent(
	ctx context.Context,
	runCtx *adk.ChatModelAgentContext,
) (context.Context, *adk.ChatModelAgentContext, error) {
	if m == nil {
		return ctx, runCtx, fmt.Errorf(
			"eino adk provider capability middleware is invalid",
		)
	}
	capabilities := m.downgradedCapabilities()
	if len(capabilities) == 0 {
		return ctx, runCtx, nil
	}
	m.downgradeOnce.Do(func() {
		m.emitDowngradeEvent(ctx, capabilities)
	})
	return ctx, runCtx, nil
}

func (m *ADKProviderCapabilityMiddleware) downgradedCapabilities() []string {
	if m == nil {
		return nil
	}
	capabilities := make([]string, 0, 2)
	if m.requestedReasoning.ReasoningEffort !=
		m.effectiveReasoning.ReasoningEffort {
		capabilities = append(capabilities, string(ADKProviderCapabilityReasoning))
	}
	if m.requestedReasoning.ThinkingEnabled !=
		m.effectiveReasoning.ThinkingEnabled {
		capabilities = append(capabilities, string(ADKProviderCapabilityThinking))
	}
	return capabilities
}

func (m *ADKProviderCapabilityMiddleware) emitDowngradeEvent(
	ctx context.Context,
	capabilities []string,
) {
	if m == nil || m.run == nil || len(capabilities) == 0 {
		return
	}
	emitRunEvent(ctx, m.eventSink, RunEvent{
		ThreadID:  m.run.ThreadID,
		RunID:     m.run.RunID,
		EventType: adkProviderCapabilityDowngradedEventType,
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"schema":                     adkProviderCapabilityDowngradeSchema,
			"capabilities":               capabilities,
			"requested_thinking_enabled": m.requestedReasoning.ThinkingEnabled,
			"effective_thinking_enabled": m.effectiveReasoning.ThinkingEnabled,
			"requested_reasoning_effort": adkReasoningEffortLabel(m.requestedReasoning.ReasoningEffort),
			"effective_reasoning_effort": adkReasoningEffortLabel(m.effectiveReasoning.ReasoningEffort),
		}),
	})
}

func adkReasoningEffortLabel(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "none"
	}
	return value
}

func (m *ADKProviderCapabilityMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if m == nil {
		return ctx, state, fmt.Errorf(
			"eino adk provider capability middleware is invalid",
		)
	}
	if err := validateADKProviderCapabilityRequest(m.config); err != nil {
		return ctx, state, err
	}
	if state == nil {
		return ctx, state, nil
	}
	if err := validateADKProviderCapabilityMessages(
		m.config.Capabilities,
		state.Messages,
	); err != nil {
		return ctx, state, err
	}

	return ctx, state, nil
}

func adkProviderCapabilityConfigFromRun(
	run *RunSummary,
	capabilities ADKModelCapabilities,
) (ADKProviderCapabilityConfig, error) {
	config := ADKProviderCapabilityConfig{Capabilities: capabilities}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return config, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return config, fmt.Errorf("parse provider capability config: %w", err)
	}

	for _, key := range []string{
		"provider_capabilities",
		"providerCapabilities",
		"model_capabilities",
		"modelCapabilities",
	} {
		capabilityMap, ok := configObject(payload[key])
		if !ok {
			continue
		}
		applyADKProviderCapabilityOverrides(
			&config.Capabilities,
			capabilityMap,
		)
	}

	config.Reasoning.ReasoningEffort = firstConfigString(
		payload,
		"reasoning_effort",
		"reasoningEffort",
	)
	if config.Reasoning.ReasoningEffort != "" {
		config.Reasoning.ReasoningEffort = strings.ToLower(
			config.Reasoning.ReasoningEffort,
		)
		if !validADKReasoningEffort(config.Reasoning.ReasoningEffort) {
			return config, fmt.Errorf(
				"unsupported reasoning_effort: %s",
				config.Reasoning.ReasoningEffort,
			)
		}
	}
	config.Reasoning.ThinkingEnabled = firstADKConfigBool(
		payload,
		"thinking_enabled",
		"thinkingEnabled",
	)

	return config, nil
}

func applyADKProviderCapabilityOverrides(
	capabilities *ADKModelCapabilities,
	payload map[string]any,
) {
	if capabilities == nil {
		return
	}
	applyADKCapabilityBool(
		&capabilities.Thinking,
		payload,
		"thinking",
		"thinking_enabled",
		"thinkingEnabled",
	)
	applyADKCapabilityBool(
		&capabilities.Reasoning,
		payload,
		"reasoning",
		"reasoning_effort",
		"reasoningEffort",
	)
	applyADKCapabilityBool(
		&capabilities.Vision,
		payload,
		"vision",
		"image",
		"image_input",
		"imageInput",
	)
	applyADKCapabilityBool(
		&capabilities.PDF,
		payload,
		"pdf",
		"pdf_input",
		"pdfInput",
	)
	applyADKCapabilityBool(
		&capabilities.File,
		payload,
		"file",
		"file_input",
		"fileInput",
	)
	applyADKCapabilityBool(
		&capabilities.Audio,
		payload,
		"audio",
		"audio_input",
		"audioInput",
	)
	applyADKCapabilityBool(
		&capabilities.Video,
		payload,
		"video",
		"video_input",
		"videoInput",
	)
}

func applyADKCapabilityBool(
	target *bool,
	payload map[string]any,
	keys ...string,
) {
	if target == nil {
		return
	}
	for _, key := range keys {
		parsed, ok := configBool(payload[key])
		if !ok {
			continue
		}
		*target = parsed
		return
	}
}

func validateADKProviderCapabilityRequest(
	config ADKProviderCapabilityConfig,
) error {
	if config.Reasoning.ReasoningEffort != "" &&
		!config.Capabilities.Reasoning {
		return &ADKProviderCapabilityError{
			Capability: ADKProviderCapabilityReasoning,
			PartType:   "reasoning_effort",
			Count:      1,
		}
	}
	if config.Reasoning.ThinkingEnabled && !config.Capabilities.Thinking {
		return &ADKProviderCapabilityError{
			Capability: ADKProviderCapabilityThinking,
			PartType:   "thinking_enabled",
			Count:      1,
		}
	}
	return nil
}

func validateADKProviderCapabilityMessages(
	capabilities ADKModelCapabilities,
	messages []*schema.Message,
) error {
	counts := countADKProviderCapabilityParts(messages)
	checks := []struct {
		capability ADKProviderCapability
		partType   string
		supported  bool
		count      int
	}{
		{
			capability: ADKProviderCapabilityVision,
			partType:   string(schema.ChatMessagePartTypeImageURL),
			supported:  capabilities.Vision,
			count:      counts.Vision,
		},
		{
			capability: ADKProviderCapabilityPDF,
			partType:   string(schema.ChatMessagePartTypeFileURL),
			supported:  capabilities.PDF,
			count:      counts.PDF,
		},
		{
			capability: ADKProviderCapabilityFile,
			partType:   string(schema.ChatMessagePartTypeFileURL),
			supported:  capabilities.File,
			count:      counts.File,
		},
		{
			capability: ADKProviderCapabilityAudio,
			partType:   string(schema.ChatMessagePartTypeAudioURL),
			supported:  capabilities.Audio,
			count:      counts.Audio,
		},
		{
			capability: ADKProviderCapabilityVideo,
			partType:   string(schema.ChatMessagePartTypeVideoURL),
			supported:  capabilities.Video,
			count:      counts.Video,
		},
	}
	for _, check := range checks {
		if check.count == 0 || check.supported {
			continue
		}
		return &ADKProviderCapabilityError{
			Capability: check.capability,
			PartType:   check.partType,
			Count:      check.count,
		}
	}
	return nil
}

type adkProviderCapabilityPartCounts struct {
	Vision int
	PDF    int
	File   int
	Audio  int
	Video  int
}

func countADKProviderCapabilityParts(
	messages []*schema.Message,
) adkProviderCapabilityPartCounts {
	counts := adkProviderCapabilityPartCounts{}
	for _, message := range messages {
		if message == nil {
			continue
		}
		for _, part := range message.MultiContent {
			counts.addLegacyPart(part)
		}
		for _, part := range message.UserInputMultiContent {
			counts.addInputPart(part)
		}
		for _, part := range message.AssistantGenMultiContent {
			counts.addOutputPart(part)
		}
	}
	return counts
}

func (c *adkProviderCapabilityPartCounts) addLegacyPart(
	part schema.ChatMessagePart,
) {
	if c == nil {
		return
	}
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.ImageURL != nil {
			c.Vision++
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.AudioURL != nil {
			c.Audio++
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.VideoURL != nil {
			c.Video++
		}
	case schema.ChatMessagePartTypeFileURL:
		if part.FileURL == nil {
			return
		}
		if adkCapabilityFileIsPDF(
			part.FileURL.MIMEType,
			part.FileURL.Name,
			part.FileURL.URL,
		) {
			c.PDF++
			return
		}
		c.File++
	}
}

func (c *adkProviderCapabilityPartCounts) addInputPart(
	part schema.MessageInputPart,
) {
	if c == nil {
		return
	}
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.Image != nil {
			c.Vision++
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio != nil {
			c.Audio++
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video != nil {
			c.Video++
		}
	case schema.ChatMessagePartTypeFileURL:
		if part.File == nil {
			return
		}
		if adkCapabilityFileIsPDF(
			part.File.MIMEType,
			part.File.Name,
			stringValue(part.File.URL),
		) {
			c.PDF++
			return
		}
		c.File++
	}
}

func (c *adkProviderCapabilityPartCounts) addOutputPart(
	part schema.MessageOutputPart,
) {
	if c == nil {
		return
	}
	switch part.Type {
	case schema.ChatMessagePartTypeImageURL:
		if part.Image != nil {
			c.Vision++
		}
	case schema.ChatMessagePartTypeAudioURL:
		if part.Audio != nil {
			c.Audio++
		}
	case schema.ChatMessagePartTypeVideoURL:
		if part.Video != nil {
			c.Video++
		}
	}
}

func adkCapabilityFileIsPDF(mimeType, name, rawURL string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if semicolon := strings.Index(mimeType, ";"); semicolon >= 0 {
		mimeType = strings.TrimSpace(mimeType[:semicolon])
	}
	if mimeType == "application/pdf" {
		return true
	}
	return hasPDFSuffix(name) || hasPDFSuffix(rawURL)
}

func hasPDFSuffix(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return false
	}
	if question := strings.Index(value, "?"); question >= 0 {
		value = value[:question]
	}
	if hash := strings.Index(value, "#"); hash >= 0 {
		value = value[:hash]
	}
	return strings.HasSuffix(value, ".pdf")
}

func validADKReasoningEffort(value string) bool {
	switch value {
	case "minimal", "low", "medium", "high":
		return true
	default:
		return false
	}
}

func firstADKConfigBool(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		parsed, ok := configBool(payload[key])
		if ok {
			return parsed
		}
	}
	return false
}

func configBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed := strings.ToLower(strings.TrimSpace(typed))
		switch parsed {
		case "true", "1", "yes", "y", "on":
			return true, true
		case "false", "0", "no", "n", "off":
			return false, true
		}
	}
	return false, false
}

func configObject(value any) (map[string]any, bool) {
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	return nil, false
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
