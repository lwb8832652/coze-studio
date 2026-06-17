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

func TestResumeRunProcessorFailsClaimedRunUntilReplayIsImplemented(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				WorkerID: "resume-worker-a",
				Command:  `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
				Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
			},
		},
		checkpoint: &entity.Checkpoint{
			ID:            503,
			ThreadID:      10,
			RunID:         199,
			CheckpointNS:  "harness.terminal",
			ChannelValues: `{"messages":[{"role":"assistant","content":"partial"}]}`,
			PendingSends:  `[{"node":"generate_answer","step_id":"step-1"}]`,
			Metadata:      `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "resume-worker-a",
			ErrorCode:    "checkpoint_replay_not_implemented",
			ErrorMessage: "checkpoint replay is not implemented yet",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err := processor.ProcessQueuedResumeRuns(context.Background())

	require.NoError(t, err)
	require.Equal(t, "resume-worker-a", domainSVC.claimQueuedResumeRunsReq.WorkerID)
	require.Equal(t, int32(1), domainSVC.claimQueuedResumeRunsReq.Limit)
	require.Equal(t, int64(503), domainSVC.getCheckpointReq.CheckpointID)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, int64(200), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "resume-worker-a", domainSVC.failRunReq.WorkerID)
	require.Equal(t, "checkpoint_replay_not_implemented", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "checkpoint replay is not implemented yet", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, []string{"run.resume.started", "run.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"checkpoint_id":"503"`)
	require.Contains(t, eventSink.events[0].Payload, `"resume_from":"pending_sends"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_code":"checkpoint_replay_not_implemented"`)
	require.Contains(t, eventSink.events[1].Payload, `"checkpoint_ns":"harness.terminal"`)
}

func TestResumeRunProcessorFailsRunWhenCheckpointIDIsMissing(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				WorkerID: "resume-worker-a",
				Command:  `{"resume":{"resume_from":"pending_sends"}}`,
				Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
			},
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "resume-worker-a",
			ErrorCode:    "checkpoint_resume_payload_invalid",
			ErrorMessage: "resume run is missing command.resume.checkpoint_id",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err := processor.ProcessQueuedResumeRuns(context.Background())

	require.NoError(t, err)
	require.Nil(t, domainSVC.getCheckpointReq)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, "checkpoint_resume_payload_invalid", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "resume run is missing command.resume.checkpoint_id", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, []string{"run.resume.started", "run.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"error_code":"checkpoint_resume_payload_invalid"`)
}
