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
				ID:       301,
				ThreadID: 10,
				RunID:    20,
				Scope:    entity.MemoryScopeThread,
				Content:  "用户偏好中文回答",
				Metadata: `{"source":"profile"}`,
				Score:    0.91,
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
}
