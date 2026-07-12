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
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestResumeRunProcessorCompletesClaimedResumeRunWithAssistantMessage(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{
				ID:                  200,
				ThreadID:            10,
				SpaceID:             30,
				CreatorID:           40,
				Status:              entity.RunStatusRunning,
				WorkerID:            "resume-worker-a",
				LeaseOwner:          "resume-worker-a",
				LeaseToken:          "lease-200",
				LeaseExpiresAt:      60_000,
				ExecutionGeneration: 4,
				Command:             `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
				Metadata:            `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
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
			Message:                  "resumed answer",
			Metadata:                 `{"source":"agent_harness","steps":2}`,
			ParityParentCheckpointID: 503,
			ParityState: &ADKParityState{
				SchemaVersion: adkParityStateSchemaVersion,
				Revision:      7,
				SpaceID:       30,
				ThreadID:      10,
				LastRunID:     200,
				Messages: []ADKParityMessage{
					{Role: "assistant", Content: "resumed answer", RunID: 200},
				},
				Workspace:    newADKParityWorkspace(30, 10),
				Todos:        []ADKParityTodo{},
				Uploads:      []ADKParityUpload{},
				Artifacts:    []ADKParityArtifact{},
				ViewedImages: map[string]ADKParityViewedImage{},
				ActiveSkills: []ADKParitySkill{},
				Interrupts:   []ADKParityInterrupt{{ID: "interrupt-1"}},
			},
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
	require.Equal(t, "resume-worker-a", domainSVC.completeRunReq.LeaseOwner)
	require.Equal(t, "lease-200", domainSVC.completeRunReq.LeaseToken)
	require.Equal(t, uint64(4), domainSVC.completeRunReq.ExecutionGeneration)
	require.Nil(t, domainSVC.failRunReq)
	require.Equal(t, []string{"run.resume.started", "run.resume.loaded"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[0].Payload, `"checkpoint_id":"503"`)
	require.Contains(t, eventSink.events[0].Payload, `"resume_from":"pending_sends"`)
	require.Contains(t, eventSink.events[1].Payload, `"checkpoint_step_count":1`)
	require.Contains(t, eventSink.events[1].Payload, `"pending_step_count":1`)
	require.Contains(t, eventSink.events[1].Payload, `"message_count":1`)
	require.Contains(t, domainSVC.finalizeRunSuccessReq.CompletionEventPayload, `"status":"succeeded"`)
	require.Contains(t, domainSVC.finalizeRunSuccessReq.CompletionEventPayload, `"checkpoint_ns":"harness.terminal"`)
	checkpoint := domainSVC.finalizeRunSuccessReq.TerminalCheckpoint
	require.NotNil(t, checkpoint)
	require.Equal(t, int64(503), checkpoint.ParentCheckpointID)
	require.Contains(t, checkpoint.Metadata, `"checkpoint_phase":"terminal"`)
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
	require.NoError(t, err)
	require.NotNil(t, envelope.ParityState)
	require.Empty(t, envelope.ParityState.Interrupts)
	require.Equal(t, "succeeded", envelope.ParityState.Completion.Status)
	require.Equal(t, int64(200), envelope.ParityState.Completion.RunID)
}

func TestResumeRunProcessorTreatsLateSuccessAfterCancellationAsCanceled(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{{
			ID:                  200,
			ThreadID:            10,
			Status:              entity.RunStatusRunning,
			WorkerID:            "resume-worker-a",
			LeaseOwner:          "resume-worker-a",
			LeaseToken:          "lease-200",
			ExecutionGeneration: 4,
			Command:             `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
		}},
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
		finalizeRunSuccessErr: domainrepo.ErrRunCanceled,
	}
	eventSink := &recordingRunEventSink{}
	processor := NewResumeRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
		Executor: ResumeRunExecutorFunc(func(
			context.Context,
			*RunSummary,
			*HarnessResumeInput,
		) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "resumed answer"}, nil
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.CanceledRuns)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.Nil(t, domainSVC.appendReq)
	require.Equal(t, []string{"run.resume.started", "run.resume.loaded"}, eventSink.eventTypes())
}

func TestResumeRunProcessorReportsProcessResultForCompletedRun(t *testing.T) {
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
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
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
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		Executor: ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "resumed answer"}, nil
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
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
			"memory":{"items":[{"id":"mem-1","scope":"thread","content":"remember this","score":0.8,"metadata":"{\"kind\":\"fact\"}"}]},
			"skills":{"items":[{"id":"101","name":"weekly-research","description":"Research weekly changes.","type":"deer_skill","version":"1.2.0","body":"Collect sources."}]}
		}`,
		ChannelVersions: `{"messages":1,"steps":1,"memory":1,"skills":1}`,
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
	require.Len(t, input.State.Skills.Items, 1)
	require.Equal(t, int64(101), input.State.Skills.Items[0].ID)
	require.Equal(t, "weekly-research", input.State.Skills.Items[0].Name)
	require.Equal(t, "Collect sources.", input.State.Skills.Items[0].Body)
	require.Len(t, input.PendingSteps, 1)
	require.Equal(t, "step-2", input.PendingSteps[0].ID)
	require.Equal(t, AgentStepTypeModel, input.PendingSteps[0].Type)
	require.Equal(t, "generate_answer", input.PendingSteps[0].Name)
	require.True(t, input.PendingSteps[0].Final)
}

func TestLoadADKResumeInputRestoresEnvelopeWithoutLegacyPendingSends(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
		RunRevision:     4,
		CreatedAt:       100,
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)

	input, err := loadADKResumeInput(
		&RunSummary{RunID: 200, ThreadID: 10},
		resumeRunPayload{CheckpointID: 503, CheckpointNS: "eino.adk"},
		&CheckpointSummary{
			CheckpointID:    503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "eino.adk",
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   string(raw),
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, RuntimeModeEinoADK, input.Runtime)
	require.Equal(t, "checkpoint-1", input.RuntimeKey)
	require.Equal(t, int64(199), input.SourceRunID)
	require.Equal(t, []byte{1, 2, 3}, input.ADKCheckpoint.Checkpoint)
}

func TestParseResumeRunPayloadAcceptsTargets(t *testing.T) {
	payload, err := parseResumeRunPayload(`{
		"resume":{
			"checkpoint_id":"503",
			"checkpoint_ns":"eino.adk",
			"resume_from":"interrupt",
			"targets":{
				"interrupt-1":{
					"schema":"coze.human_interaction_response.v1",
					"interaction_id":"hi_1",
					"kind":"clarification",
					"decision":"answered",
					"answer":"最近 7 天"
				}
			}
		}
	}`)

	require.NoError(t, err)
	require.Equal(t, int64(503), payload.CheckpointID)
	require.Equal(t, "eino.adk", payload.CheckpointNS)
	require.Equal(t, "interrupt", payload.ResumeFrom)
	require.Contains(t, payload.Targets, "interrupt-1")
	target, ok := payload.Targets["interrupt-1"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hi_1", target["interaction_id"])
	require.Equal(t, "最近 7 天", target["answer"])
}

func TestLoadADKResumeInputUsesCommandTargets(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {
				ID:          "interrupt-1",
				Address:     "lead/tool/ask_user_clarification",
				IsRootCause: true,
			},
		},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)

	input, err := loadADKResumeInput(
		&RunSummary{RunID: 200, ThreadID: 10},
		resumeRunPayload{
			CheckpointID: 503,
			CheckpointNS: "eino.adk",
			Targets: map[string]any{
				"interrupt-1": map[string]any{
					"schema":         humanInteractionResponseSchema,
					"interaction_id": "hi_1",
					"kind":           "clarification",
					"decision":       "answered",
					"answer":         "最近 7 天",
				},
			},
		},
		&CheckpointSummary{
			CheckpointID:    503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "eino.adk",
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   string(raw),
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
		},
	)

	require.NoError(t, err)
	require.Contains(t, input.ADKResumeTargets, "interrupt-1")
	target, ok := input.ADKResumeTargets["interrupt-1"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hi_1", target["interaction_id"])
	require.Equal(t, "最近 7 天", target["answer"])
}

func TestLoadADKResumeInputRejectsUnknownCommandTarget(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {ID: "interrupt-1", Address: "lead/tool/approval"},
		},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)

	input, err := loadADKResumeInput(
		&RunSummary{RunID: 200, ThreadID: 10},
		resumeRunPayload{
			CheckpointID: 503,
			CheckpointNS: "eino.adk",
			Targets: map[string]any{
				"missing": map[string]any{"decision": "answered"},
			},
		},
		&CheckpointSummary{
			CheckpointID:    503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "eino.adk",
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   string(raw),
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
		},
	)

	require.Error(t, err)
	require.Nil(t, input)
	require.Contains(t, err.Error(), "resume target missing is not present in checkpoint interrupts")
}

func TestLoadADKResumeInputKeepsNilTargetsWhenCommandTargetsMissing(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {ID: "interrupt-1", Address: "lead/tool/approval"},
		},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)

	input, err := loadADKResumeInput(
		&RunSummary{RunID: 200, ThreadID: 10},
		resumeRunPayload{CheckpointID: 503, CheckpointNS: "eino.adk"},
		&CheckpointSummary{
			CheckpointID:    503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "eino.adk",
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   string(raw),
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
		},
	)

	require.NoError(t, err)
	require.Contains(t, input.ADKResumeTargets, "interrupt-1")
	require.Nil(t, input.ADKResumeTargets["interrupt-1"])
}

func TestRuntimeModeFromCheckpointIgnoresUnrelatedLegacyMetadataRuntime(t *testing.T) {
	mode, err := runtimeModeFromCheckpoint(&CheckpointSummary{
		RuntimeType: "legacy",
		Metadata:    `{"runtime":"go","source":"checkpoint"}`,
	})

	require.NoError(t, err)
	require.Equal(t, RuntimeModeLegacy, mode)
}

func TestResumeRunProcessorMarksADKInterruptWithoutFailingRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusRunning,
			WorkerID: "resume-worker-a",
			Command:  `{"resume":{"checkpoint_id":"503"}}`,
		}},
		checkpoint: &entity.Checkpoint{
			ID:              503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "eino.adk",
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues: mustADKCheckpointEnvelopeJSON(t, ADKCheckpointEnvelope{
				EnvelopeVersion: 1,
				Runtime:         string(RuntimeModeEinoADK),
				RuntimeVersion:  "0.9.9",
				RuntimeKey:      "checkpoint-1",
				MessageType:     "schema.Message",
				Checkpoint:      []byte{1},
			}),
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
		},
		interruptedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusInterrupted,
			WorkerID: "resume-worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
		Executor: ResumeRunExecutorFunc(func(
			context.Context,
			*RunSummary,
			*HarnessResumeInput,
		) (*RunExecutionResult, error) {
			return nil, &RunInterruptedError{
				CheckpointKey: "checkpoint-1",
				Interrupts: []ADKInterruptItem{{
					ID:          "approval",
					Address:     "agent:lead;tool:approval",
					IsRootCause: true,
				}},
			}
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.NotNil(t, domainSVC.interruptRunReq)
	require.Nil(t, domainSVC.failRunReq)
	require.Nil(t, domainSVC.appendReq)
	require.Equal(t, []string{
		"run.resume.started",
		"run.resume.loaded",
	}, eventSink.eventTypes())
	require.Contains(t, domainSVC.interruptRunReq.EventPayload, `"status":"interrupted"`)
	require.Contains(t, domainSVC.interruptRunReq.EventPayload, `"checkpoint_key":"checkpoint-1"`)
	require.False(t, domainSVC.interruptRunReq.EventAlreadyPersisted)
}

func TestResumeRunProcessorDoesNotFailCanceledADKRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusRunning,
			WorkerID: "resume-worker-a",
			Command:  `{"resume":{"checkpoint_id":"503"}}`,
		}},
		checkpoint: &entity.Checkpoint{
			ID:              503,
			ThreadID:        10,
			RunID:           199,
			CheckpointNS:    "eino.adk",
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues: mustADKCheckpointEnvelopeJSON(t, ADKCheckpointEnvelope{
				EnvelopeVersion: 1,
				Runtime:         string(RuntimeModeEinoADK),
				RuntimeVersion:  "0.9.9",
				RuntimeKey:      "checkpoint-1",
				MessageType:     "schema.Message",
				Checkpoint:      []byte{1},
			}),
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{"runtime":"eino_adk"}`,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		EventSink: eventSink,
		Executor: ResumeRunExecutorFunc(func(
			context.Context,
			*RunSummary,
			*HarnessResumeInput,
		) (*RunExecutionResult, error) {
			return nil, &RunCanceledError{EventPersisted: true}
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.CanceledRuns)
	require.Nil(t, domainSVC.failRunReq)
	require.Nil(t, domainSVC.appendReq)
	require.Equal(t, []string{
		"run.resume.started",
		"run.resume.loaded",
	}, eventSink.eventTypes())
}

func mustADKCheckpointEnvelopeJSON(t *testing.T, envelope ADKCheckpointEnvelope) string {
	t.Helper()

	raw, err := envelope.Marshal()
	require.NoError(t, err)

	return string(raw)
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
	require.Equal(t, []string{"run.resume.started"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"error_code":"checkpoint_resume_payload_invalid"`)
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "resume run is missing")
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
	require.Equal(t, []string{"run.resume.started", "run.resume.loaded"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"error_code":"checkpoint_resume_executor_error"`)
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "resume executor failed")
}

func TestResumeRunProcessorReportsProcessResultForFailedRun(t *testing.T) {
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
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false}],"memory":{"items":[]}}`,
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
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		Executor:  &recordingResumeRunExecutor{err: errors.New("resume executor failed")},
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		FailedRuns:    1,
	}, result)
}

func TestResumeRunProcessorReportsProcessResultWhenCompleteRunFails(t *testing.T) {
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
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		appended: &entity.Message{
			ID:       301,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "resumed answer",
		},
		completeRunErr: errors.New("complete run failed"),
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:  "resume-worker-a",
		BatchSize: 1,
		Executor: ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "resumed answer"}, nil
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.ErrorContains(t, err, "complete run failed")
	require.NotNil(t, domainSVC.appendReq)
	require.NotNil(t, domainSVC.completeRunReq)
	require.Nil(t, domainSVC.failRunReq)
	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		ErroredRuns:   1,
	}, result)
}

func TestResumeRunProcessorIsolatesBatchFailureAndReleasesBackToProtectedQueue(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{ID: 200, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "resume-worker-a", LeaseOwner: "resume-worker-a", LeaseToken: "lease-200", ExecutionGeneration: 1, Command: `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`, Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`},
			{ID: 201, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "resume-worker-a", LeaseOwner: "resume-worker-a", LeaseToken: "lease-201", ExecutionGeneration: 1, Command: `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`, Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`},
			{ID: 202, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "resume-worker-a", LeaseOwner: "resume-worker-a", LeaseToken: "lease-202", ExecutionGeneration: 1, Command: `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`, Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`},
		},
		checkpoint: &entity.Checkpoint{
			ID: 503, ThreadID: 10, RunID: 199, CheckpointNS: "harness.terminal",
			ChannelValues: `{"messages":[],"steps":[],"memory":{"items":[]}}`, ChannelVersions: `{}`,
			PendingSends: `[{"node":"generate_answer","step_id":"step-1"}]`, Metadata: `{}`,
		},
		appended:          &entity.Message{ID: 300, ThreadID: 10, Role: entity.MessageRoleAssistant, Content: "resumed"},
		completedRun:      &entity.Run{ID: 999, ThreadID: 10, Status: entity.RunStatusSucceeded},
		completeRunErrors: map[int64]error{200: errors.New("resume completion infrastructure failure")},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	executed := make([]int64, 0, 3)
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID: "resume-worker-a", BatchSize: 3,
		Executor: ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
			executed = append(executed, run.RunID)
			return &RunExecutionResult{Message: "resumed"}, nil
		}),
	})

	result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())

	require.ErrorContains(t, err, "resume completion infrastructure failure")
	require.Equal(t, []int64{200, 201, 202}, executed)
	require.Equal(t, ResumeRunProcessResult{
		ClaimedRuns:   3,
		ProcessedRuns: 3,
		SucceededRuns: 2,
		ErroredRuns:   1,
	}, result)
	require.Len(t, domainSVC.completeRunReqs, 3)
	require.Len(t, domainSVC.releaseRunLeaseReqs, 1)
	require.Equal(t, int64(200), domainSVC.releaseRunLeaseReqs[0].RunID)
	require.Equal(t, entity.RunStatusQueued, domainSVC.releaseRunLeaseReqs[0].ToStatus)
	require.Equal(t, "lease-200", domainSVC.releaseRunLeaseReqs[0].LeaseToken)
}

func TestResumeRunProcessorRenewsLeaseWhileExecutionIsActive(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(1_000))
	renewCalls := make(chan *domainservice.RenewRunLeaseRequest, 1)
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "resume-worker-a", LeaseOwner: "resume-worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 4,
			Command:             `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
			Metadata:            `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
		}},
		checkpoint: &entity.Checkpoint{
			ID: 503, ThreadID: 10, RunID: 199, CheckpointNS: "harness.terminal",
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false,"message_present":true}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		renewRunLeaseCalls: renewCalls,
		appended:           &entity.Message{ID: 301, ThreadID: 10, RunID: 200, Role: entity.MessageRoleAssistant, Content: "resumed"},
		completedRun:       &entity.Run{ID: 200, ThreadID: 10, Status: entity.RunStatusSucceeded},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	started := make(chan struct{})
	release := make(chan struct{})
	processor := NewResumeRunProcessor(app, ResumeRunProcessorOptions{
		WorkerID:          "resume-worker-a",
		BatchSize:         1,
		LeaseTTL:          6 * time.Second,
		HeartbeatInterval: 2 * time.Second,
		LeaseClock:        clock,
		Executor: ResumeRunExecutorFunc(func(ctx context.Context, run *RunSummary, input *HarnessResumeInput) (*RunExecutionResult, error) {
			close(started)
			<-release
			return &RunExecutionResult{Message: "resumed"}, nil
		}),
	})
	done := make(chan error, 1)
	go func() {
		done <- processor.ProcessQueuedResumeRuns(context.Background())
	}()

	<-started
	clock.Tick(time.UnixMilli(3_000))
	renew := <-renewCalls
	require.Equal(t, int64(200), renew.RunID)
	require.Equal(t, "resume-worker-a", renew.LeaseOwner)
	require.Equal(t, "lease-200", renew.LeaseToken)
	require.Equal(t, uint64(4), renew.ExecutionGeneration)
	require.Equal(t, int64(3_000), renew.Now)
	require.Equal(t, int64(6_000), renew.LeaseTTLMillis)
	close(release)
	require.NoError(t, <-done)
	require.Equal(t, int64(1_000), domainSVC.claimQueuedResumeRunsReq.Now)
	require.Equal(t, int64(6_000), domainSVC.claimQueuedResumeRunsReq.LeaseTTLMillis)
}

func TestResumeRunProcessorTreatsLeaseLossFromDurableCancellationAsCanceled(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(1_000))
	renewCalls := make(chan *domainservice.RenewRunLeaseRequest, 1)
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "resume-worker-a", LeaseOwner: "resume-worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 4,
			Command:             `{"resume":{"checkpoint_id":"503","checkpoint_ns":"harness.terminal","resume_from":"pending_sends"}}`,
		}},
		checkpoint: &entity.Checkpoint{
			ID: 503, ThreadID: 10, RunID: 199, CheckpointNS: "harness.terminal",
			ChannelValues:   `{"messages":[{"role":"assistant","content":"partial","step_id":"step-1"}],"steps":[{"step_id":"step-1","step_type":"model","step_name":"draft","step_index":0,"final":false,"message_present":true}],"memory":{"items":[]}}`,
			ChannelVersions: `{"messages":1,"steps":1,"memory":0}`,
			PendingSends:    `[{"node":"generate_answer","step_id":"step-2","final":true}]`,
			Metadata:        `{"source":"agent_harness","checkpoint_phase":"terminal","status":"failed"}`,
		},
		gotRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusCanceled,
			CancelRequestedAt: 3_000, ExecutionGeneration: 5,
		},
		renewRunLeaseCalls: renewCalls,
		renewRunLeaseErr:   domainrepo.ErrRunLeaseLost,
	}
	started := make(chan struct{})
	processor := NewResumeRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, ResumeRunProcessorOptions{
		WorkerID:          "resume-worker-a",
		BatchSize:         1,
		LeaseTTL:          6 * time.Second,
		HeartbeatInterval: 2 * time.Second,
		LeaseClock:        clock,
		Executor: ResumeRunExecutorFunc(func(
			ctx context.Context,
			_ *RunSummary,
			_ *HarnessResumeInput,
		) (*RunExecutionResult, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}),
	})
	done := make(chan struct {
		result ResumeRunProcessResult
		err    error
	}, 1)
	go func() {
		result, err := processor.ProcessQueuedResumeRunsWithResult(context.Background())
		done <- struct {
			result ResumeRunProcessResult
			err    error
		}{result: result, err: err}
	}()

	<-started
	clock.Tick(time.UnixMilli(3_000))
	<-renewCalls
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, 1, got.result.CanceledRuns)
	require.Empty(t, domainSVC.releaseRunLeaseReqs)
	require.Nil(t, domainSVC.failRunReq)
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
