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

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestResumeRunProcessorCompletesClaimedResumeRunWithAssistantMessage(t *testing.T) {
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
			ID:              503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "harness.terminal",
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false,"message_present":true}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-1"}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		appended: &entity.Message{
			ID:       301,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "resumed answer",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "resume-worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	executor := &recordingResumeRunExecutor{
		result: &RunExecutionResult{
			Message:  "resumed answer",
			Metadata: `{"source":"agent_harness","steps":2}`,
		},
	}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
		Executor:  executor,
	})

	err := processor.ProcessQueuedResumeRuns(context.Background())

	require.NoError(t, err)
	require.Equal(t, "resume-worker-a", domainSVC.claimQueuedResumeRunsReq.WorkerID)
	require.Equal(t, int32(1), domainSVC.claimQueuedResumeRunsReq.Limit)
	require.Equal(t, int64(503), domainSVC.getCheckpointReq.CheckpointID)
	require.Equal(t, int64(200), executor.run.RunID)
	require.Equal(t, int64(503), executor.input.CheckpointID)
	require.Len(t, executor.input.PendingSteps, 1)
	require.Equal(t, int64(200), domainSVC.appendReq.RunID)
	require.Equal(t, entity.MessageRoleAssistant, domainSVC.appendReq.Role)
	require.Equal(t, "resumed answer", domainSVC.appendReq.Content)
	require.Equal(t, `{"source":"agent_harness","steps":2}`, domainSVC.appendReq.Metadata)
	require.Equal(t, int64(200), domainSVC.completeRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.completeRunReq.From)
	require.Equal(t, "resume-worker-a", domainSVC.completeRunReq.WorkerID)
	require.Nil(t, domainSVC.failRunReq)
	require.Equal(t, []string{"run.resume.started", "run.resume.loaded", "run.completed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"checkpoint_id":"503"`)
	require.Contains(t, eventSink.events[0].Payload, `"resume_from":"pending_sends"`)
	require.Contains(t, eventSink.events[1].Payload, `"checkpoint_step_count":1`)
	require.Contains(t, eventSink.events[1].Payload, `"pending_step_count":1`)
	require.Contains(t, eventSink.events[1].Payload, `"message_count":1`)
	require.Contains(t, eventSink.events[2].Payload, `"status":"succeeded"`)
	require.Contains(t, eventSink.events[2].Payload, `"checkpoint_ns":"harness.terminal"`)
}

func TestLoadHarnessResumeInputConvertsCheckpointState(t *testing.T) {
	run := &RunSummary{ThreadID: 10, RunID: 200}
	resume := resumeRunPayload{
		CheckpointID: 503,
		CheckpointNS: "harness.terminal",
		ResumeFrom:   "pending_sends",
	}
	checkpoint := &CheckpointSummary{
		CheckpointID: 503,
		ThreadID:     10,
		RunID:        199,
		CheckpointNS: "harness.terminal",
		ChannelValues: `{
			"messages":[
				{"role":"user","content":"hello"},
				{"role":"assistant","content":"partial","step_id":"step-1"}
			],
			"steps":[
				{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false,"message_present":true}
			],
			"memory":{"items":[{"id":"mem-1","scope":"thread","content":"remember this","score":0.8,"metadata":"{\"kind\":\"fact\"}"}]}
		}`,
		ChannelVersions: `{"messages":1,"steps":1,"memory":1}`,
		PendingSends:    `[{"node":"generate_answer","step_id":"step-2","step_type":"model","step_name":"generate_answer","final":true}]`,
		Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
	}

	input, err := loadHarnessResumeInput(run, resume, checkpoint)

	require.NoError(t, err)
	require.Equal(t, int64(10), input.ThreadID)
	require.Equal(t, int64(200), input.RunID)
	require.Equal(t, int64(199), input.SourceRunID)
	require.Equal(t, int64(503), input.CheckpointID)
	require.Equal(t, "harness.terminal", input.CheckpointNS)
	require.Equal(t, "pending_sends", input.ResumeFrom)
	require.Len(t, input.Messages, 2)
	require.Equal(t, float64(1), input.ChannelVersions["steps"])
	require.Equal(t, "failed", input.Metadata["status"])
	require.Len(t, input.State.Steps, 1)
	require.Equal(t, "step-1", input.State.Steps[0].StepID)
	require.Equal(t, AgentStepTypeModel, input.State.Steps[0].StepType)
	require.Equal(t, "draft", input.State.Steps[0].StepName)
	require.Equal(t, "partial", input.State.Steps[0].Message)
	require.Equal(t, 1, input.State.StepIndex)
	require.Len(t, input.State.Results, 1)
	require.Equal(t, "partial", input.State.Results[0].Message)
	require.Len(t, input.State.Memory.Items, 1)
	require.Equal(t, "mem-1", input.State.Memory.Items[0].ID)
	require.Equal(t, "remember this", input.State.Memory.Items[0].Content)
	require.Len(t, input.PendingSteps, 1)
	require.Equal(t, "step-2", input.PendingSteps[0].ID)
	require.Equal(t, AgentStepTypeModel, input.PendingSteps[0].Type)
	require.Equal(t, "generate_answer", input.PendingSteps[0].Name)
	require.True(t, input.PendingSteps[0].Final)
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

func TestResumeRunProcessorMarksRunFailedWhenResumeExecutorErrors(t *testing.T) {
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
			ID:              503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "harness.terminal",
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false,"message_present":true}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "resume-worker-a",
			ErrorCode:    "checkpoint_resume_executor_error",
			ErrorMessage: "resume executor failed",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
		Executor:  &recordingResumeRunExecutor{err: errors.New("resume executor failed")},
	})

	err := processor.ProcessQueuedResumeRuns(context.Background())

	require.NoError(t, err)
	require.Nil(t, domainSVC.appendReq)
	require.Nil(t, domainSVC.completeRunReq)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, int64(200), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "resume-worker-a", domainSVC.failRunReq.WorkerID)
	require.Equal(t, "checkpoint_resume_executor_error", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "resume executor failed", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, []string{"run.resume.started", "run.resume.loaded", "run.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[2].Payload, `"error_code":"checkpoint_resume_executor_error"`)
}

type recordingResumeRunExecutor struct {
	run    *RunSummary
	input  *HarnessResumeInput
	result *RunExecutionResult
	err    error
}

func (e *recordingResumeRunExecutor) Resume(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
	e.run = run
	e.input = input

	return e.result, e.err
}
