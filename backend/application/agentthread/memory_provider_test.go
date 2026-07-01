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

func TestThreadMemoryProviderRecallsApplicationMemories(t *testing.T) {
	domainSVC := &recordingThreadService{
		recalledMemories: []*entity.Memory{
			{
				ID:                   301,
				ThreadID:             10,
				RunID:                20,
				Scope:                entity.MemoryScopeThread,
				Content:              "用户偏好中文回答",
				Metadata:             `{"source":"profile"}`,
				Score:                0.91,
				Confidence:           0.86,
				SourceType:           "manual",
				SourceID:             "profile-card",
				CorrectionOfMemoryID: 299,
				CorrectedAt:          1234,
			},
		},
		memoryTotal: 1,
	}
	provider := NewThreadMemoryProvider(&ApplicationService{ThreadSVC: domainSVC}, 6)

	memories, err := provider.Recall(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.recallMemoriesReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.recallMemoriesReq.RunID)
	require.Equal(t, int32(6), domainSVC.recallMemoriesReq.Limit)
	require.Len(t, memories, 1)
	require.Equal(t, "301", memories[0].ID)
	require.Equal(t, "thread", memories[0].Scope)
	require.Equal(t, "用户偏好中文回答", memories[0].Content)
	require.Equal(t, `{"source":"profile"}`, memories[0].Metadata)
	require.Equal(t, 0.91, memories[0].Score)
	require.Equal(t, 0.86, memories[0].Confidence)
	require.Equal(t, "manual", memories[0].SourceType)
	require.Equal(t, "profile-card", memories[0].SourceID)
	require.Equal(t, int64(299), memories[0].CorrectionOfMemoryID)
	require.Equal(t, int64(1234), memories[0].CorrectedAt)
}

func TestThreadMemoryProviderAppliesContextAwareRetrievalConfig(t *testing.T) {
	domainSVC := &recordingThreadService{
		recalledMemories: []*entity.Memory{
			{
				ID:         1,
				ThreadID:   10,
				Scope:      entity.MemoryScopeThread,
				Content:    "unrelated high score",
				Score:      0.99,
				Confidence: 0,
			},
			{
				ID:         2,
				ThreadID:   10,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "apac deployment region",
				Score:      0.7,
				Confidence: 0.8,
			},
			{
				ID:         3,
				ThreadID:   10,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "apac budget is 10",
				Score:      0.95,
				Confidence: 0.9,
			},
			{
				ID:                   4,
				ThreadID:             10,
				Scope:                entity.MemoryScopeLongTerm,
				Content:              "apac budget is 20",
				Score:                0.75,
				Confidence:           0.95,
				CorrectionOfMemoryID: 3,
				CorrectedAt:          2000,
			},
			{
				ID:         5,
				ThreadID:   10,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "apac budget low confidence",
				Score:      1,
				Confidence: 0.2,
			},
		},
		memoryTotal: 5,
	}
	provider := NewThreadMemoryProvider(&ApplicationService{ThreadSVC: domainSVC}, 3)

	memories, err := provider.Recall(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
		Input: `{
			"messages":[
				{"role":"user","content":"review emea rollout"},
				{"role":"assistant","content":"emea rollout summarized"},
				{"role":"user","content":"review apac budget"}
			]
		}`,
		Config: `{
			"memory_retrieval":{
				"limit":2,
				"candidate_limit":5,
				"scopes":["long_term","thread"],
				"min_confidence":0.6
			}
		}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.recallMemoriesReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.recallMemoriesReq.RunID)
	require.Equal(t, "review apac budget", domainSVC.recallMemoriesReq.Query)
	require.Equal(t, int32(5), domainSVC.recallMemoriesReq.Limit)
	require.Equal(t, []entity.MemoryScope{
		entity.MemoryScopeLongTerm,
		entity.MemoryScopeThread,
	}, domainSVC.recallMemoriesReq.Scopes)
	require.Len(t, memories, 2)
	require.Equal(t, []string{"4", "2"}, []string{memories[0].ID, memories[1].ID})
	require.NotContains(t, []string{memories[0].ID, memories[1].ID}, "3")
	require.NotContains(t, []string{memories[0].ID, memories[1].ID}, "5")
}
