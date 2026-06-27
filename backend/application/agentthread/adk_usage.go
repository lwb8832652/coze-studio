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
	"strconv"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type adkUsageContextKey string

const (
	adkUsageAgentContextKey adkUsageContextKey = "agent"
	adkUsageCallContextKey  adkUsageContextKey = "model_call"
	adkUsageKindContextKey  adkUsageContextKey = "usage_kind"
)

type adkUsageCall struct {
	agentName     string
	callID        string
	retry         int
	usageKind     string
	modelName     string
	provider      string
	component     string
	componentName string
	finishReason  string
	modelConfig   adkUsageModelConfig
}

type adkUsageModelConfig struct {
	MaxTokens   int
	Temperature float32
	TopP        float32
	StopCount   int
}

type ADKUsageBridge struct {
	run       *RunSummary
	collector UsageCollector
	leadAgent string
	traceID   string

	mu                sync.Mutex
	callSequences     map[string]uint64
	seen              map[string]struct{}
	eventSuppressions map[string]int
	err               error
}

func NewADKUsageBridge(run *RunSummary, collector UsageCollector) *ADKUsageBridge {
	leadAgent := defaultADKAgentName
	if run != nil {
		if cfg, err := parseModelExecutorConfig(run.Config); err == nil && strings.TrimSpace(cfg.AgentName) != "" {
			leadAgent = strings.TrimSpace(cfg.AgentName)
		}
	}

	return &ADKUsageBridge{
		run:               run,
		collector:         collector,
		leadAgent:         leadAgent,
		traceID:           adkUsageTraceID(run),
		callSequences:     make(map[string]uint64),
		seen:              make(map[string]struct{}),
		eventSuppressions: make(map[string]int),
	}
}

func (b *ADKUsageBridge) Handler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(b.onStart).
		OnEndFn(b.onEnd).
		OnEndWithStreamOutputFn(b.onEndWithStreamOutput).
		Build()
}

func (b *ADKUsageBridge) onStart(
	ctx context.Context,
	info *callbacks.RunInfo,
	input callbacks.CallbackInput,
) context.Context {
	if info != nil && info.Component == adk.ComponentOfAgent {
		return context.WithValue(ctx, adkUsageAgentContextKey, strings.TrimSpace(info.Name))
	}
	if info == nil || info.Component != components.ComponentOfChatModel {
		return ctx
	}

	modelInput := model.ConvCallbackInput(input)
	if modelInput == nil {
		return ctx
	}

	call := b.newCall(ctx, info, modelInput.Config, modelInput.Extra)
	return context.WithValue(ctx, adkUsageCallContextKey, call)
}

func (b *ADKUsageBridge) onEnd(
	ctx context.Context,
	info *callbacks.RunInfo,
	output callbacks.CallbackOutput,
) context.Context {
	if info == nil || info.Component != components.ComponentOfChatModel {
		return ctx
	}
	modelOutput := model.ConvCallbackOutput(output)
	if modelOutput == nil {
		return ctx
	}

	b.recordModelOutput(ctx, info, modelOutput)
	return ctx
}

func (b *ADKUsageBridge) onEndWithStreamOutput(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *schema.StreamReader[callbacks.CallbackOutput],
) context.Context {
	if info == nil || info.Component != components.ComponentOfChatModel || output == nil {
		return ctx
	}
	defer output.Close()

	var last *model.CallbackOutput
	for {
		item, err := output.Recv()
		if err != nil {
			if err != io.EOF {
				b.setError(fmt.Errorf("consume eino usage callback stream: %w", err))
			}
			if last != nil {
				b.recordModelOutput(ctx, info, last)
			}
			return ctx
		}
		modelOutput := model.ConvCallbackOutput(item)
		if modelOutput == nil {
			continue
		}
		if modelOutput.TokenUsage != nil ||
			(modelOutput.Message != nil && modelOutput.Message.ResponseMeta != nil &&
				modelOutput.Message.ResponseMeta.Usage != nil) {
			last = modelOutput
		}
	}
}

func (b *ADKUsageBridge) newCall(
	ctx context.Context,
	info *callbacks.RunInfo,
	config *model.Config,
	extra map[string]any,
) adkUsageCall {
	agentName, _ := ctx.Value(adkUsageAgentContextKey).(string)
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		agentName = b.leadAgent
	}
	b.mu.Lock()
	b.callSequences[agentName]++
	sequence := b.callSequences[agentName]
	b.mu.Unlock()

	call := adkUsageCall{
		agentName: agentName,
		callID:    usageExtraString(extra, "model_call_id"),
		retry:     usageExtraInt(extra, "retry_attempt"),
		usageKind: usageExtraString(extra, "usage_kind"),
	}
	if call.callID == "" {
		call.callID = "callback-" + strconv.FormatUint(sequence, 10)
	}
	if call.usageKind == "" {
		call.usageKind = adkUsageKindFromContext(ctx)
	}
	if call.usageKind == "" && info != nil {
		call.usageKind = strings.TrimSpace(info.Name)
	}
	if call.usageKind == "" {
		call.usageKind = "chat"
	}
	if config != nil {
		call.modelName = strings.TrimSpace(config.Model)
		call.modelConfig = adkUsageModelConfigFromConfig(config)
	}
	if info != nil {
		call.provider = strings.TrimSpace(info.Type)
		call.component = strings.TrimSpace(fmt.Sprint(info.Component))
		call.componentName = strings.TrimSpace(info.Name)
	}

	return call
}

func (b *ADKUsageBridge) recordModelOutput(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *model.CallbackOutput,
) {
	if output == nil {
		return
	}
	call, _ := ctx.Value(adkUsageCallContextKey).(adkUsageCall)
	if call.agentName == "" {
		call = b.newCall(ctx, info, output.Config, output.Extra)
	}
	if output.Config != nil && strings.TrimSpace(output.Config.Model) != "" {
		call.modelName = strings.TrimSpace(output.Config.Model)
	}
	if output.Config != nil {
		call.modelConfig = adkUsageModelConfigFromConfig(output.Config)
	}
	if output.Message != nil && output.Message.ResponseMeta != nil {
		call.finishReason = strings.TrimSpace(output.Message.ResponseMeta.FinishReason)
	}
	if value := usageExtraString(output.Extra, "model_call_id"); value != "" {
		call.callID = value
	}
	if value := usageExtraString(output.Extra, "usage_kind"); value != "" {
		call.usageKind = value
	}
	if output.Extra != nil {
		call.retry = usageExtraInt(output.Extra, "retry_attempt")
	}

	usage := output.TokenUsage
	if usage == nil && output.Message != nil && output.Message.ResponseMeta != nil &&
		output.Message.ResponseMeta.Usage != nil {
		usage = modelUsageFromSchema(output.Message.ResponseMeta.Usage)
	}
	if usage == nil {
		return
	}

	raw := adkUsagePayload{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		CachedTokens:     usage.PromptTokenDetails.CachedTokens,
		ReasoningTokens:  usage.CompletionTokensDetails.ReasoningTokens,
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		b.setError(fmt.Errorf("marshal eino callback token usage: %w", err))
		return
	}

	source := b.sourceFor(call)
	idempotencyKey := adkUsageIdempotencyKey(b.runID(), call)
	spanID := adkUsageSpanID(b.traceID, call)
	parentSpanID := adkUsageParentSpanID(b.traceID, call.agentName)
	metadata, err := json.Marshal(map[string]any{
		"agent_name":      call.agentName,
		"component":       call.component,
		"component_name":  call.componentName,
		"idempotency_key": idempotencyKey,
		"model_metadata":  adkUsageModelMetadata(call),
		"model_name":      call.modelName,
		"model_call_id":   call.callID,
		"parent_span_id":  parentSpanID,
		"provider":        call.provider,
		"retry_attempt":   call.retry,
		"span_id":         spanID,
		"source":          "eino_callback",
		"trace_id":        b.traceID,
		"usage_kind":      call.usageKind,
	})
	if err != nil {
		b.setError(fmt.Errorf("marshal eino callback usage metadata: %w", err))
		return
	}

	record := AgentTokenUsage{
		Source:       source,
		StepID:       call.callID,
		StepName:     call.agentName,
		ModelName:    call.modelName,
		Provider:     call.provider,
		InputTokens:  int64(usage.PromptTokens),
		OutputTokens: int64(usage.CompletionTokens),
		TotalTokens:  int64(usage.TotalTokens),
		RawUsage:     string(rawJSON),
		Metadata:     string(metadata),
	}
	if b.record(ctx, idempotencyKey, record) {
		b.mu.Lock()
		b.eventSuppressions[adkUsageFingerprint(record)]++
		b.mu.Unlock()
	}
}

func (b *ADKUsageBridge) RecordEvent(ctx context.Context, usage AgentTokenUsage) error {
	if b == nil || b.collector == nil {
		return nil
	}

	fingerprint := adkUsageFingerprint(usage)
	b.mu.Lock()
	if b.eventSuppressions[fingerprint] > 0 {
		b.eventSuppressions[fingerprint]--
		b.mu.Unlock()
		return nil
	}
	b.mu.Unlock()

	call := adkUsageCall{
		agentName: usage.StepName,
		callID:    usage.StepID,
		modelName: usage.ModelName,
		provider:  usage.Provider,
		usageKind: "event",
	}
	if call.callID == "" {
		call.callID = fingerprint
	}
	key := adkUsageIdempotencyKey(b.runID(), call)
	spanID := adkUsageSpanID(b.traceID, call)
	parentSpanID := adkUsageParentSpanID(b.traceID, call.agentName)
	metadata, err := json.Marshal(map[string]any{
		"agent_name":      call.agentName,
		"idempotency_key": key,
		"parent_span_id":  parentSpanID,
		"span_id":         spanID,
		"source":          "eino_event",
		"trace_id":        b.traceID,
		"usage_kind":      "event",
	})
	if err != nil {
		return err
	}
	usage.Metadata = string(metadata)
	b.record(ctx, key, usage)

	return b.Err()
}

func (b *ADKUsageBridge) TraceID() string {
	if b == nil {
		return ""
	}

	return b.traceID
}

func (b *ADKUsageBridge) Err() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *ADKUsageBridge) record(ctx context.Context, key string, usage AgentTokenUsage) bool {
	if b == nil || b.collector == nil {
		return false
	}

	b.mu.Lock()
	if _, exists := b.seen[key]; exists {
		b.mu.Unlock()
		return false
	}
	b.seen[key] = struct{}{}
	b.mu.Unlock()

	if err := b.collector.Record(ctx, b.run, usage); err != nil {
		b.setError(err)
	}

	return true
}

func (b *ADKUsageBridge) sourceFor(call adkUsageCall) TokenUsageSource {
	if strings.Contains(strings.ToLower(call.usageKind), "summar") ||
		strings.Contains(strings.ToLower(call.usageKind), "middleware") {
		return TokenUsageSourceMiddleware
	}
	if call.agentName != "" && call.agentName != b.leadAgent {
		return TokenUsageSourceSubagent
	}
	return TokenUsageSourceLeadAgent
}

func (b *ADKUsageBridge) runID() int64 {
	if b == nil || b.run == nil {
		return 0
	}
	return b.run.RunID
}

func (b *ADKUsageBridge) setError(err error) {
	if b == nil || err == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err == nil {
		b.err = err
	}
}

func adkUsageIdempotencyKey(runID int64, call adkUsageCall) string {
	value := strings.Join([]string{
		strconv.FormatInt(runID, 10),
		call.agentName,
		call.callID,
		strconv.Itoa(call.retry),
		call.usageKind,
		call.modelName,
		call.provider,
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func adkUsageTraceID(run *RunSummary) string {
	parts := []string{"coze_eino_adk_trace"}
	if run != nil {
		parts = append(
			parts,
			strconv.FormatInt(run.SpaceID, 10),
			strconv.FormatInt(run.ThreadID, 10),
			strconv.FormatInt(run.RunID, 10),
		)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))

	return hex.EncodeToString(sum[:16])
}

func adkUsageSpanID(traceID string, call adkUsageCall) string {
	value := strings.Join([]string{
		traceID,
		call.agentName,
		call.callID,
		strconv.Itoa(call.retry),
		call.usageKind,
		call.modelName,
		call.provider,
		call.finishReason,
	}, "\x00")
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:8])
}

func adkUsageParentSpanID(traceID string, agentName string) string {
	value := strings.Join([]string{
		traceID,
		"agent",
		strings.TrimSpace(agentName),
	}, "\x00")
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:8])
}

func adkUsageModelConfigFromConfig(config *model.Config) adkUsageModelConfig {
	if config == nil {
		return adkUsageModelConfig{}
	}

	return adkUsageModelConfig{
		MaxTokens:   config.MaxTokens,
		Temperature: config.Temperature,
		TopP:        config.TopP,
		StopCount:   len(config.Stop),
	}
}

func adkUsageModelMetadata(call adkUsageCall) map[string]any {
	metadata := make(map[string]any)
	if call.modelName != "" {
		metadata["model_name"] = call.modelName
	}
	if call.provider != "" {
		metadata["provider"] = call.provider
	}
	if call.component != "" {
		metadata["component"] = call.component
	}
	if call.componentName != "" {
		metadata["component_name"] = call.componentName
	}
	if call.finishReason != "" {
		metadata["finish_reason"] = call.finishReason
	}
	if call.modelConfig.MaxTokens > 0 {
		metadata["max_tokens"] = call.modelConfig.MaxTokens
	}
	if call.modelConfig.Temperature != 0 {
		metadata["temperature"] = call.modelConfig.Temperature
	}
	if call.modelConfig.TopP != 0 {
		metadata["top_p"] = call.modelConfig.TopP
	}
	if call.modelConfig.StopCount > 0 {
		metadata["stop_count"] = call.modelConfig.StopCount
	}

	return metadata
}

func adkUsageFingerprint(usage AgentTokenUsage) string {
	value := strings.Join([]string{
		usage.StepName,
		strconv.FormatInt(usage.InputTokens, 10),
		strconv.FormatInt(usage.OutputTokens, 10),
		strconv.FormatInt(usage.TotalTokens, 10),
		usage.RawUsage,
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func usageExtraString(extra map[string]any, key string) string {
	if extra == nil {
		return ""
	}
	value, exists := extra[key]
	if !exists || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func usageExtraInt(extra map[string]any, key string) int {
	raw := usageExtraString(extra, key)
	value, _ := strconv.Atoi(raw)
	return value
}

func withADKUsageKind(ctx context.Context, usageKind string) context.Context {
	usageKind = strings.TrimSpace(usageKind)
	if usageKind == "" {
		return ctx
	}
	return context.WithValue(ctx, adkUsageKindContextKey, usageKind)
}

func adkUsageKindFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(adkUsageKindContextKey).(string)
	return strings.TrimSpace(value)
}

type adkUsageKindChatModel struct {
	base      model.BaseChatModel
	usageKind string
}

func newADKUsageKindChatModel(
	base model.BaseChatModel,
	usageKind string,
) model.BaseChatModel {
	if base == nil || strings.TrimSpace(usageKind) == "" {
		return base
	}
	return &adkUsageKindChatModel{
		base:      base,
		usageKind: strings.TrimSpace(usageKind),
	}
}

func (m *adkUsageKindChatModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	return m.base.Generate(
		withADKUsageKind(ctx, m.usageKind),
		input,
		options...,
	)
}

func (m *adkUsageKindChatModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return m.base.Stream(
		withADKUsageKind(ctx, m.usageKind),
		input,
		options...,
	)
}

func modelUsageFromSchema(usage *schema.TokenUsage) *model.TokenUsage {
	if usage == nil {
		return nil
	}
	return &model.TokenUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		PromptTokenDetails: model.PromptTokenDetails{
			CachedTokens: usage.PromptTokenDetails.CachedTokens,
		},
		CompletionTokensDetails: model.CompletionTokensDetails{
			ReasoningTokens: usage.CompletionTokensDetails.ReasoningTokens,
		},
	}
}
