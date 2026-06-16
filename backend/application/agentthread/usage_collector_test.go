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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestThreadUsageCollectorRecordsApplicationUsage(t *testing.T) {
	domainSVC := &recordingThreadService{
		recordedTokenUsage: &entity.TokenUsage{
			ID:          400,
			ThreadID:    10,
			RunID:       20,
			Source:      entity.TokenUsageSourceLeadAgent,
			TotalTokens: 20,
		},
	}
	collector := NewThreadUsageCollector(&ApplicationService{ThreadSVC: domainSVC})

	err := collector.Record(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
	}, AgentTokenUsage{
		Source:       TokenUsageSourceLeadAgent,
		StepID:       "model-1",
		StepIndex:    0,
		StepName:     "generate_answer",
		ModelName:    "gpt-test",
		Provider:     "openai-compatible",
		InputTokens:  12,
		OutputTokens: 8,
		RawUsage:     `{"prompt_tokens":12,"completion_tokens":8}`,
		Metadata:     `{"source":"harness"}`,
	})

	require.NoError(t, err)
	require.NotNil(t, domainSVC.recordTokenUsageReq)
	require.Equal(t, int64(20), domainSVC.recordTokenUsageReq.RunID)
	require.Equal(t, entity.TokenUsageSourceLeadAgent, domainSVC.recordTokenUsageReq.Source)
	require.Equal(t, "model-1", domainSVC.recordTokenUsageReq.StepID)
	require.Equal(t, int64(12), domainSVC.recordTokenUsageReq.InputTokens)
	require.Equal(t, int64(8), domainSVC.recordTokenUsageReq.OutputTokens)
	require.Equal(t, `{"prompt_tokens":12,"completion_tokens":8}`, domainSVC.recordTokenUsageReq.RawUsage)
}
