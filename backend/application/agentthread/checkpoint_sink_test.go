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

func TestThreadCheckpointSinkPersistsApplicationCheckpoint(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdCheckpoint: &entity.Checkpoint{
			ID:              501,
			ThreadID:        10,
			RunID:           20,
			CheckpointNS:    "harness.step",
			ChannelValues:   `{"messages":[]}`,
			ChannelVersions: `{"messages":1}`,
			PendingSends:    `[]`,
			Metadata:        `{"source":"agent_harness"}`,
		},
	}
	sink := NewThreadCheckpointSink(&ApplicationService{ThreadSVC: domainSVC})

	checkpoint, err := sink.SaveCheckpoint(context.Background(), &RunSummary{
		ThreadID: 10,
		RunID:    20,
	}, AgentCheckpoint{
		ParentCheckpointID: 500,
		CheckpointNS:       "harness.step",
		ChannelValues:      `{"messages":[]}`,
		ChannelVersions:    `{"messages":1}`,
		PendingSends:       `[]`,
		Metadata:           `{"source":"agent_harness"}`,
	})

	require.NoError(t, err)
	require.NotNil(t, checkpoint)
	require.Equal(t, int64(501), checkpoint.CheckpointID)
	require.NotNil(t, domainSVC.createCheckpointReq)
	require.Equal(t, int64(10), domainSVC.createCheckpointReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.createCheckpointReq.RunID)
	require.Equal(t, int64(500), domainSVC.createCheckpointReq.ParentCheckpointID)
	require.Equal(t, "harness.step", domainSVC.createCheckpointReq.CheckpointNS)
	require.Equal(t, `{"messages":[]}`, domainSVC.createCheckpointReq.ChannelValues)
	require.Equal(t, `{"messages":1}`, domainSVC.createCheckpointReq.ChannelVersions)
	require.Equal(t, `[]`, domainSVC.createCheckpointReq.PendingSends)
	require.Equal(t, `{"source":"agent_harness"}`, domainSVC.createCheckpointReq.Metadata)
}
