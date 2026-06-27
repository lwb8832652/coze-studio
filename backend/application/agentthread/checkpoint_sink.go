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
	"fmt"
)

type AgentCheckpoint struct {
	ParentCheckpointID int64
	CheckpointNS       string
	RuntimeType        string
	RuntimeKey         string
	EnvelopeVersion    int32
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
}

type CheckpointSink interface {
	SaveCheckpoint(ctx context.Context, run *RunSummary, checkpoint AgentCheckpoint) (*CheckpointSummary, error)
}

type ThreadCheckpointSink struct {
	app *ApplicationService
}

func NewThreadCheckpointSink(app *ApplicationService) *ThreadCheckpointSink {
	return &ThreadCheckpointSink{app: app}
}

func (s *ThreadCheckpointSink) SaveCheckpoint(ctx context.Context, run *RunSummary, checkpoint AgentCheckpoint) (*CheckpointSummary, error) {
	if s == nil || s.app == nil {
		return nil, fmt.Errorf("agent thread checkpoint sink application service is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.RunID <= 0 {
		return nil, fmt.Errorf("run id is required")
	}

	resp, err := s.app.CreateCheckpoint(ctx, &CreateCheckpointRequest{
		ThreadID:           run.ThreadID,
		RunID:              run.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       checkpoint.CheckpointNS,
		RuntimeType:        checkpoint.RuntimeType,
		RuntimeKey:         checkpoint.RuntimeKey,
		EnvelopeVersion:    checkpoint.EnvelopeVersion,
		ChannelValues:      checkpoint.ChannelValues,
		ChannelVersions:    checkpoint.ChannelVersions,
		PendingSends:       checkpoint.PendingSends,
		Metadata:           checkpoint.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("agent thread checkpoint sink returned empty response")
	}

	return resp.Checkpoint, nil
}
