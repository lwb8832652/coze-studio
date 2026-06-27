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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKModelReliabilityRetryConfigDisabledByDefault(t *testing.T) {
	config, err := adkModelRetryConfigFromRun(&RunSummary{})

	require.NoError(t, err)
	require.Nil(t, config)
}

func TestADKModelReliabilityRetryConfigRetriesConfiguredFailures(t *testing.T) {
	config, err := adkModelRetryConfigFromRun(&RunSummary{
		Config: `{
			"model_retry":{
				"max_retries":2,
				"backoff_ms":0,
				"retry_empty_output":true,
				"retry_finish_reasons":["length"]
			}
		}`,
	})
	require.NoError(t, err)
	require.NotNil(t, config)
	require.Equal(t, 2, config.MaxRetries)
	require.NotNil(t, config.ShouldRetry)
	require.NotNil(t, config.BackoffFunc)
	require.Zero(t, config.BackoffFunc(context.Background(), 1))

	errorDecision := config.ShouldRetry(context.Background(), &adk.RetryContext{
		RetryAttempt: 1,
		Err:          errors.New("temporary provider error"),
	})
	require.NotNil(t, errorDecision)
	require.True(t, errorDecision.Retry)

	emptyDecision := config.ShouldRetry(context.Background(), &adk.RetryContext{
		RetryAttempt:  1,
		OutputMessage: schema.AssistantMessage("", nil),
	})
	require.NotNil(t, emptyDecision)
	require.True(t, emptyDecision.Retry)
	require.Equal(t, "empty_output", emptyDecision.RejectReason)

	lengthDecision := config.ShouldRetry(context.Background(), &adk.RetryContext{
		RetryAttempt: 1,
		OutputMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: "partial",
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: "length",
			},
		},
	})
	require.NotNil(t, lengthDecision)
	require.True(t, lengthDecision.Retry)
	require.Equal(t, "finish_reason:length", lengthDecision.RejectReason)

	stopDecision := config.ShouldRetry(context.Background(), &adk.RetryContext{
		RetryAttempt: 1,
		OutputMessage: &schema.Message{
			Role:    schema.Assistant,
			Content: "done",
			ResponseMeta: &schema.ResponseMeta{
				FinishReason: "stop",
			},
		},
	})
	require.NotNil(t, stopDecision)
	require.False(t, stopDecision.Retry)
}

func TestADKModelReliabilityRetryConfigRejectsInvalidLimits(t *testing.T) {
	config, err := adkModelRetryConfigFromRun(&RunSummary{
		Config: `{"model_retry":{"max_retries":6}}`,
	})

	require.ErrorContains(t, err, "model retry max_retries must be between 1 and 5")
	require.Nil(t, config)

	config, err = adkModelRetryConfigFromRun(&RunSummary{
		Config: `{"model_retry":{"max_retries":1,"backoff_ms":60001}}`,
	})

	require.ErrorContains(t, err, "model retry backoff_ms must be between 0 and 60000")
	require.Nil(t, config)
}
