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
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	adkModelRetryMaxRetriesLimit = 5
	adkModelRetryMaxBackoffMS    = 60000
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
		"blocked",
		"prohibited_content",
		"recitation":
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
