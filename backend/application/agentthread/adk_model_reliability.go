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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	adkModelRetryMaxRetriesLimit = 5
	adkModelRetryMaxBackoffMS    = 60000
	adkModelFailoverMaxRetries   = 5
)

func adkModelRetryConfigFromRun(run *RunSummary) (*adk.ModelRetryConfig, error) {
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return nil, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return nil, fmt.Errorf("decode adk model retry config: %w", err)
	}
	retryConfig := firstConfigMap(payload, "model_retry", "modelRetry")
	if retryConfig == nil {
		return nil, nil
	}

	maxRetries := firstConfigInt64(
		retryConfig,
		"max_retries",
		"maxRetries",
	)
	if maxRetries == 0 {
		return nil, nil
	}
	if maxRetries < 1 || maxRetries > adkModelRetryMaxRetriesLimit {
		return nil, fmt.Errorf(
			"model retry max_retries must be between 1 and %d",
			adkModelRetryMaxRetriesLimit,
		)
	}

	backoffMS, hasBackoff, err := firstConfigInt64AllowZero(
		retryConfig,
		"backoff_ms",
		"backoffMs",
	)
	if err != nil {
		return nil, fmt.Errorf("model retry backoff_ms is invalid: %w", err)
	}
	if hasBackoff && (backoffMS < 0 || backoffMS > adkModelRetryMaxBackoffMS) {
		return nil, fmt.Errorf(
			"model retry backoff_ms must be between 0 and %d",
			adkModelRetryMaxBackoffMS,
		)
	}

	retryEmptyOutput := firstConfigBool(
		retryConfig,
		"retry_empty_output",
		"retryEmptyOutput",
	)
	retryFinishReasons := newConfigStringSet(firstConfigStringSlice(
		retryConfig,
		"retry_finish_reasons",
		"retryFinishReasons",
	))

	config := &adk.ModelRetryConfig{
		MaxRetries: int(maxRetries),
		ShouldRetry: func(
			ctx context.Context,
			retryCtx *adk.RetryContext,
		) *adk.RetryDecision {
			return adkModelRetryDecision(
				ctx,
				retryCtx,
				retryEmptyOutput,
				retryFinishReasons,
			)
		},
	}
	if hasBackoff {
		config.BackoffFunc = func(context.Context, int) time.Duration {
			return time.Duration(backoffMS) * time.Millisecond
		}
	}

	return config, nil
}

func adkModelRetryDecision(
	_ context.Context,
	retryCtx *adk.RetryContext,
	retryEmptyOutput bool,
	retryFinishReasons map[string]struct{},
) *adk.RetryDecision {
	decision := &adk.RetryDecision{}
	if retryCtx == nil {
		return decision
	}
	if retryCtx.Err != nil {
		if shouldPassThroughADKModelRetryError(retryCtx.Err) {
			return decision
		}

		decision.Retry = true
		decision.RejectReason = "model_error"
		return decision
	}

	message := retryCtx.OutputMessage
	if retryEmptyOutput && isADKEmptyAssistantOutput(message) {
		decision.Retry = true
		decision.RejectReason = "empty_output"
		return decision
	}
	if message != nil && message.ResponseMeta != nil {
		finishReason := normalizeADKFinishReason(message.ResponseMeta.FinishReason)
		if _, ok := retryFinishReasons[finishReason]; ok &&
			!isADKSafetyFinishReason(finishReason) {
			decision.Retry = true
			decision.RejectReason = "finish_reason:" + finishReason
			return decision
		}
	}

	return decision
}

func adkModelFailoverConfigFromRun(
	run *RunSummary,
	provider ChatModelProvider,
	primaryModelID int64,
) (*adk.ModelFailoverConfig[*schema.Message], error) {
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return nil, nil
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return nil, fmt.Errorf("decode adk model failover config: %w", err)
	}
	failoverConfig := firstConfigMap(payload, "model_failover", "modelFailover")
	if failoverConfig == nil {
		return nil, nil
	}

	candidateModelIDs, err := firstConfigInt64Slice(
		failoverConfig,
		"candidate_model_ids",
		"candidateModelIds",
		"candidateModelIDs",
		"fallback_model_ids",
		"fallbackModelIds",
		"fallbackModelIDs",
	)
	if err != nil {
		return nil, fmt.Errorf("model failover candidate_model_ids is invalid: %w", err)
	}
	candidateModelIDs = normalizeADKModelFailoverCandidates(
		candidateModelIDs,
		primaryModelID,
	)
	if len(candidateModelIDs) == 0 {
		return nil, nil
	}

	maxRetries := firstConfigInt64(
		failoverConfig,
		"max_retries",
		"maxRetries",
	)
	if maxRetries == 0 {
		maxRetries = int64(len(candidateModelIDs))
	}
	if maxRetries < 1 || maxRetries > adkModelFailoverMaxRetries {
		return nil, fmt.Errorf(
			"model failover max_retries must be between 1 and %d",
			adkModelFailoverMaxRetries,
		)
	}
	if maxRetries > int64(len(candidateModelIDs)) {
		maxRetries = int64(len(candidateModelIDs))
	}

	if provider == nil {
		provider = DefaultChatModelProvider
	}
	failoverEmptyOutput := firstConfigBool(
		failoverConfig,
		"failover_empty_output",
		"failoverEmptyOutput",
	)
	failoverFinishReasons := newConfigStringSet(firstConfigStringSlice(
		failoverConfig,
		"failover_finish_reasons",
		"failoverFinishReasons",
	))

	config := &adk.ModelFailoverConfig[*schema.Message]{
		MaxRetries: uint(maxRetries),
		ShouldFailover: func(
			ctx context.Context,
			outputMessage *schema.Message,
			outputErr error,
		) bool {
			return adkModelFailoverDecision(
				ctx,
				outputMessage,
				outputErr,
				failoverEmptyOutput,
				failoverFinishReasons,
			)
		},
		GetFailoverModel: func(
			ctx context.Context,
			failoverCtx *adk.FailoverContext[*schema.Message],
		) (model.BaseChatModel, []*schema.Message, error) {
			if failoverCtx == nil || failoverCtx.FailoverAttempt == 0 {
				return nil, nil, fmt.Errorf("model failover candidate is unavailable")
			}
			index := int(failoverCtx.FailoverAttempt) - 1
			if index < 0 || index >= len(candidateModelIDs) {
				return nil, nil, fmt.Errorf("model failover candidate is unavailable")
			}
			chatModel, configured, err := provider(ctx, candidateModelIDs[index])
			if err != nil {
				return nil, nil, fmt.Errorf("resolve agent thread failover model: %w", err)
			}
			if !configured || chatModel == nil {
				return nil, nil, fmt.Errorf("agent thread failover model is not configured")
			}

			return chatModel, nil, nil
		},
	}

	return config, nil
}

func adkModelFailoverDecision(
	ctx context.Context,
	outputMessage *schema.Message,
	outputErr error,
	failoverEmptyOutput bool,
	failoverFinishReasons map[string]struct{},
) bool {
	if err := ctx.Err(); err != nil {
		return false
	}
	if outputErr != nil {
		var exhausted *adk.RetryExhaustedError
		if errors.As(outputErr, &exhausted) &&
			exhausted.LastErr != nil &&
			shouldPassThroughADKModelRetryError(exhausted.LastErr) {
			return false
		}
		if shouldPassThroughADKModelRetryError(outputErr) {
			return false
		}
		return true
	}

	if failoverEmptyOutput && isADKEmptyAssistantOutput(outputMessage) {
		return true
	}
	if outputMessage != nil && outputMessage.ResponseMeta != nil {
		finishReason := normalizeADKFinishReason(outputMessage.ResponseMeta.FinishReason)
		if _, ok := failoverFinishReasons[finishReason]; ok &&
			!isADKSafetyFinishReason(finishReason) {
			return true
		}
	}

	return false
}

func normalizeADKModelFailoverCandidates(
	modelIDs []int64,
	primaryModelID int64,
) []int64 {
	normalized := make([]int64, 0, len(modelIDs))
	seen := map[int64]struct{}{}
	for _, modelID := range modelIDs {
		if modelID <= 0 || modelID == primaryModelID {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		seen[modelID] = struct{}{}
		normalized = append(normalized, modelID)
	}
	return normalized
}

func shouldPassThroughADKModelRetryError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if isADKCancellationError(err) {
		return true
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return true
	}
	var signal *adk.InterruptSignal
	return errors.As(err, &signal)
}

func isADKEmptyAssistantOutput(message *schema.Message) bool {
	if message == nil {
		return true
	}
	return !hasModelExecutorMessageContent(message)
}

func normalizeADKFinishReason(reason string) string {
	reason = strings.TrimSpace(strings.ToLower(reason))
	reason = strings.ReplaceAll(reason, "-", "_")
	reason = strings.ReplaceAll(reason, " ", "_")
	return reason
}

func isADKSafetyFinishReason(reason string) bool {
	switch normalizeADKFinishReason(reason) {
	case "content_filter",
		"safety",
		"blocklist",
		"blocked",
		"prohibited_content",
		"spii",
		"recitation",
		"image_safety",
		"image_prohibited_content",
		"image_recitation",
		"refusal":
		return true
	default:
		return false
	}
}

func firstConfigMap(payload map[string]any, keys ...string) map[string]any {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		if parsed, ok := value.(map[string]any); ok {
			return parsed
		}
	}

	return nil
}

func firstConfigBool(payload map[string]any, keys ...string) bool {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case bool:
			return v
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(v))
			if err == nil {
				return parsed
			}
		}
	}

	return false
}

func firstConfigStringSlice(payload map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case []string:
			return normalizeConfigStringSlice(v)
		case []any:
			items := make([]string, 0, len(v))
			for _, item := range v {
				if text, ok := item.(string); ok {
					items = append(items, text)
				}
			}
			return normalizeConfigStringSlice(items)
		case string:
			return normalizeConfigStringSlice(strings.Split(v, ","))
		}
	}

	return nil
}

func firstConfigInt64Slice(payload map[string]any, keys ...string) ([]int64, error) {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		return configInt64Slice(value)
	}

	return nil, nil
}

func configInt64Slice(value any) ([]int64, error) {
	switch typed := value.(type) {
	case []int64:
		return typed, nil
	case []int:
		items := make([]int64, 0, len(typed))
		for _, item := range typed {
			items = append(items, int64(item))
		}
		return items, nil
	case []any:
		items := make([]int64, 0, len(typed))
		for _, item := range typed {
			parsed, err := configInt64AllowZero(item)
			if err != nil {
				return nil, err
			}
			items = append(items, parsed)
		}
		return items, nil
	case string:
		parts := strings.Split(typed, ",")
		items := make([]int64, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			parsed, err := configInt64AllowZero(part)
			if err != nil {
				return nil, err
			}
			items = append(items, parsed)
		}
		return items, nil
	default:
		parsed, err := configInt64AllowZero(value)
		if err != nil {
			return nil, err
		}
		return []int64{parsed}, nil
	}
}

func normalizeConfigStringSlice(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			normalized = append(normalized, item)
		}
	}

	return normalized
}

func newConfigStringSet(items []string) map[string]struct{} {
	values := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = normalizeADKFinishReason(item)
		if item != "" {
			values[item] = struct{}{}
		}
	}

	return values
}

func firstConfigInt64AllowZero(
	payload map[string]any,
	keys ...string,
) (int64, bool, error) {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		parsed, err := configInt64AllowZero(value)
		if err != nil {
			return 0, true, err
		}
		return parsed, true, nil
	}

	return 0, false, nil
}

func configInt64AllowZero(value any) (int64, error) {
	switch v := value.(type) {
	case float64:
		if v < 0 {
			return 0, fmt.Errorf("must be non-negative")
		}
		return int64(v), nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("must be non-negative")
		}
		return int64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("must be non-negative")
		}
		return v, nil
	case json.Number:
		parsed, err := v.Int64()
		if err != nil {
			return 0, err
		}
		if parsed < 0 {
			return 0, fmt.Errorf("must be non-negative")
		}
		return parsed, nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, err
		}
		if parsed < 0 {
			return 0, fmt.Errorf("must be non-negative")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unsupported numeric value %T", value)
	}
}
