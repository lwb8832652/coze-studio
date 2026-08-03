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

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestADKCheckpointEnvelopeRoundTrip(t *testing.T) {
	input := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
		Interrupts: map[string]ADKInterruptItem{
			"approval": {
				ID:          "approval",
				Address:     "agent:lead;tool:approval",
				IsRootCause: true,
			},
		},
		RunRevision: 4,
		CreatedAt:   100,
		Migration:   map[string]string{"source": "native"},
	}

	raw, err := input.Marshal()
	require.NoError(t, err)

	output, err := UnmarshalADKCheckpointEnvelope(raw)
	require.NoError(t, err)
	require.Equal(t, input, output)
}

func TestADKParityCheckpointEnvelopeV2RoundTrip(t *testing.T) {
	state := newTestADKParityStateTracker(t).Snapshot()
	input := ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		Checkpoint:      []byte{1, 2, 3},
		ParityState:     &state,
		RunRevision:     4,
		CreatedAt:       100,
	}

	raw, err := input.Marshal()
	require.NoError(t, err)

	output, err := UnmarshalADKCheckpointEnvelope(raw)
	require.NoError(t, err)
	require.Equal(t, input, output)
}

func TestADKJournalCheckpointEnvelopeV3RoundTrip(t *testing.T) {
	state := newTestADKParityStateTracker(t).Snapshot()
	input := ADKCheckpointEnvelope{
		EnvelopeVersion:       3,
		SchemaVersion:         adkJournalCheckpointSchemaVersion,
		Runtime:               string(RuntimeModeEinoADK),
		RuntimeVersion:        "0.9.9",
		RuntimeKey:            "checkpoint-1",
		MessageType:           "schema.Message",
		CheckpointPhase:       ADKCheckpointPhaseRuntime,
		RuntimeState:          &ADKCheckpointRuntimeState{Checkpoint: []byte{1, 2, 3}},
		AttemptID:             "att_100",
		LastCommittedSequence: 7,
		SideEffectLedger: []ADKSideEffectLedgerReference{{
			LedgerID: 500, IdempotencyKey: "tool-1", ActionKind: "write_file",
			ReplayPolicy: "idempotent_write", Status: "succeeded", Version: 3,
		}},
		ParityState: &state,
		RunRevision: 4,
		CreatedAt:   100,
	}

	raw, err := input.Marshal()
	require.NoError(t, err)
	output, err := UnmarshalADKCheckpointEnvelope(raw)
	require.NoError(t, err)
	require.Equal(t, input, output)
	recovery, err := DecodeADKRecoveryCheckpoint(output)
	require.NoError(t, err)
	require.Equal(t, "att_100", recovery.AttemptID)
	require.Equal(t, uint64(7), recovery.LastCommittedSequence)
	require.Equal(t, []byte{1, 2, 3}, recovery.RuntimeCheckpoint)
	require.Len(t, recovery.SideEffectLedger, 1)
}

func TestADKRecoveryCheckpointFailsClosedForV2AndIncompleteV3(t *testing.T) {
	state := newTestADKParityStateTracker(t).Snapshot()
	v2 := ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		Checkpoint:      []byte{1},
		ParityState:     &state,
	}
	_, err := DecodeADKRecoveryCheckpoint(v2)
	require.ErrorIs(t, err, ErrADKCheckpointRecoveryUnsafe)

	incomplete := ADKCheckpointEnvelope{
		EnvelopeVersion: 3,
		SchemaVersion:   adkJournalCheckpointSchemaVersion,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		RuntimeState:    &ADKCheckpointRuntimeState{Checkpoint: []byte{1}},
		ParityState:     &state,
	}
	_, err = incomplete.Marshal()
	require.ErrorContains(t, err, "attempt id")
}

func TestADKParityCheckpointEnvelopeAllowsTerminalSnapshotWithoutRuntimeBytes(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)
	require.NoError(t, tracker.SetCompletion(ADKParityCompletion{
		Status: "succeeded", Reason: "completed", CompletedAt: 100,
	}))
	state := tracker.Snapshot()
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseTerminal,
		ParityState:     &state,
	}

	raw, err := envelope.Marshal()
	require.NoError(t, err)
	decoded, err := UnmarshalADKCheckpointEnvelope(raw)
	require.NoError(t, err)
	require.Empty(t, decoded.Checkpoint)
	require.Equal(t, "succeeded", decoded.ParityState.Completion.Status)
}

func TestADKCheckpointEnvelopeRejectsInvalidPayloads(t *testing.T) {
	valid := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
	}

	tests := []struct {
		name      string
		mutate    func(*ADKCheckpointEnvelope)
		errString string
	}{
		{
			name: "unknown envelope version",
			mutate: func(envelope *ADKCheckpointEnvelope) {
				envelope.EnvelopeVersion = 4
			},
			errString: "unsupported checkpoint envelope version",
		},
		{
			name: "missing runtime version",
			mutate: func(envelope *ADKCheckpointEnvelope) {
				envelope.RuntimeVersion = ""
			},
			errString: "runtime version is required",
		},
		{
			name: "wrong message type",
			mutate: func(envelope *ADKCheckpointEnvelope) {
				envelope.MessageType = "schema.AgenticMessage"
			},
			errString: "unsupported checkpoint message type",
		},
		{
			name: "empty checkpoint",
			mutate: func(envelope *ADKCheckpointEnvelope) {
				envelope.Checkpoint = nil
			},
			errString: "checkpoint bytes are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			tt.mutate(&input)

			_, err := input.Marshal()

			require.ErrorContains(t, err, tt.errString)
		})
	}

	_, err := marshalADKCheckpointEnvelope(valid, 0)
	require.ErrorContains(t, err, "maximum checkpoint size")

	oversized := valid
	oversized.Checkpoint = []byte{1, 2}
	_, err = marshalADKCheckpointEnvelope(oversized, 1)
	require.ErrorContains(t, err, "checkpoint exceeds maximum size")
}

func TestADKCheckpointStorePersistsLoadsAndDeletesByRuntimeKey(t *testing.T) {
	service := &recordingADKCheckpointService{}
	store, err := NewADKCheckpointStore(
		service,
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 7, CreatorID: 9},
		WithADKCheckpointMaxBytes(1024),
		WithADKCheckpointClock(func() int64 { return 1234 }),
		WithADKCheckpointRunRevision(7),
	)
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), "checkpoint-1", []byte{1, 2, 3}))
	require.NotNil(t, service.created)
	require.Equal(t, int64(10), service.created.ThreadID)
	require.Equal(t, int64(20), service.created.RunID)
	require.Equal(t, "eino_adk", service.created.RuntimeType)
	require.Equal(t, "checkpoint-1", service.created.RuntimeKey)
	require.Equal(t, int32(2), service.created.EnvelopeVersion)
	require.Equal(t, "eino.adk", service.created.CheckpointNS)
	require.Equal(t, `{}`, service.created.ChannelVersions)
	require.Equal(t, `[]`, service.created.PendingSends)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"runtime_version":"0.9.9",
		"runtime_key":"checkpoint-1",
		"envelope_version":2,
		"message_type":"schema.Message",
		"checkpoint_phase":"runtime"
	}`, service.created.Metadata)

	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, envelope.Checkpoint)
	require.Equal(t, ADKCheckpointPhaseRuntime, envelope.CheckpointPhase)
	require.NotNil(t, envelope.ParityState)
	require.Equal(t, int64(10), envelope.ParityState.ThreadID)
	require.Equal(t, int64(7), envelope.RunRevision)
	require.Equal(t, int64(1234), envelope.CreatedAt)

	require.NoError(t, store.RecordInterrupts(context.Background(), "checkpoint-1", []ADKInterruptItem{{
		ID:          "interrupt-1",
		Address:     "agent:lead;tool:approval",
		IsRootCause: true,
	}}))
	envelope, err = UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, ADKCheckpointPhaseInterrupt, envelope.CheckpointPhase)
	require.Equal(t, ADKInterruptItem{
		ID:          "interrupt-1",
		Address:     "agent:lead;tool:approval",
		IsRootCause: true,
	}, envelope.Interrupts["interrupt-1"])
	require.Equal(t, []ADKParityInterrupt{{
		ID: "interrupt-1", Address: "agent:lead;tool:approval", IsRootCause: true,
	}}, envelope.ParityState.Interrupts)
	require.Len(t, service.createdHistory, 2)
	require.Equal(t, service.createdHistory[0].createdID, service.createdHistory[1].request.ParentCheckpointID)

	value, exists, err := store.Get(context.Background(), "checkpoint-1")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, []byte{1, 2, 3}, value)
	require.Equal(t, "checkpoint-1", service.latestReq.RuntimeKey)

	require.NoError(t, store.Delete(context.Background(), "checkpoint-1"))
	require.Equal(t, "checkpoint-1", service.deleted.RuntimeKey)
	require.Equal(t, int64(1234), service.deleted.DeletedAt)

	_, exists, err = store.Get(context.Background(), "checkpoint-1")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestADKCheckpointStorePersistsJournalV3WithLedgerState(t *testing.T) {
	service := &recordingADKCheckpointService{}
	reader := &recordingADKJournalCheckpointStateReader{
		attempt: &domainentity.RunAttempt{
			ID: 100, ThreadID: 10, JournalRunID: 20, ExecutionRunID: 20,
			AttemptID: "att_100", LastCommittedSequence: 7,
		},
		ledgers: []*domainentity.SideEffectLedger{{
			ID: 500, IdempotencyKey: "tool-1", ActionKind: "write_file",
			ReplayPolicy: domainentity.SideEffectReplayPolicyIdempotentWrite,
			Status:       domainentity.SideEffectLedgerStatusSucceeded, Version: 3,
		}},
	}
	store, err := NewADKCheckpointStore(
		service,
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 7, CreatorID: 9},
		WithADKJournalCheckpointStateReader(reader),
	)
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), "checkpoint-1", []byte{1, 2, 3}))
	require.Equal(t, int32(3), service.created.EnvelopeVersion)
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, adkJournalCheckpointSchemaVersion, envelope.SchemaVersion)
	require.Equal(t, "att_100", envelope.AttemptID)
	require.Equal(t, uint64(7), envelope.LastCommittedSequence)
	require.Equal(t, []byte{1, 2, 3}, envelope.RuntimeState.Checkpoint)
	require.Len(t, envelope.SideEffectLedger, 1)
	require.Equal(t, int64(500), envelope.SideEffectLedger[0].LedgerID)
	require.Empty(t, envelope.Checkpoint)

	require.NoError(t, store.RecordInterrupts(context.Background(), "checkpoint-1", []ADKInterruptItem{{
		ID: "approval", Address: "agent:lead;tool:approval", IsRootCause: true,
	}}))
	require.Equal(t, int32(3), service.created.EnvelopeVersion)
	envelope, err = UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, "att_100", envelope.AttemptID)
	require.Equal(t, ADKCheckpointPhaseInterrupt, envelope.CheckpointPhase)
}

func TestADKCheckpointStoreRejectsRuntimeVersionMismatch(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.8.0",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)

	service := &recordingADKCheckpointService{
		latest: &CheckpointSummary{
			ThreadID:        10,
			RunID:           20,
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			ChannelValues:   string(raw),
		},
	}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 20, ThreadID: 10, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	_, _, err = store.Get(context.Background(), "checkpoint-1")

	require.ErrorContains(t, err, "checkpoint runtime version 0.8.0 is incompatible with 0.9.9")
}

func TestADKCheckpointStoreLoadsLegacyV1RuntimeBytes(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1, 2, 3},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)
	service := &recordingADKCheckpointService{latest: &CheckpointSummary{
		CheckpointID: 1, ThreadID: 10, RunID: 20,
		RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "checkpoint-1",
		EnvelopeVersion: 1, ChannelValues: string(raw),
	}}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 20, ThreadID: 10, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	value, exists, err := store.Get(context.Background(), "checkpoint-1")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, []byte{1, 2, 3}, value)
}

func TestADKCheckpointStoreRejectsIndexedEnvelopeVersionMismatch(t *testing.T) {
	state := newTestADKParityStateTracker(t).Snapshot()
	raw, err := (ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK), RuntimeVersion: "0.9.9",
		RuntimeKey: "checkpoint-1", MessageType: "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		Checkpoint:      []byte{1}, ParityState: &state,
	}).Marshal()
	require.NoError(t, err)
	service := &recordingADKCheckpointService{latest: &CheckpointSummary{
		CheckpointID: 1, ThreadID: 42, RunID: 1,
		RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "checkpoint-1",
		EnvelopeVersion: 1, ChannelValues: string(raw),
	}}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 1, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	_, _, err = store.Get(context.Background(), "checkpoint-1")
	require.ErrorContains(t, err, "indexed version")
}

func TestADKCheckpointStoreNormalizesRuntimeKey(t *testing.T) {
	service := &recordingADKCheckpointService{}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 20, ThreadID: 10, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), " checkpoint-1 ", []byte{1}))

	require.Equal(t, "checkpoint-1", service.created.RuntimeKey)
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, "checkpoint-1", envelope.RuntimeKey)
}

func TestADKCheckpointStoreSeedsParityStateFromLatestThreadCheckpoint(t *testing.T) {
	seedTracker := newTestADKParityStateTracker(t)
	require.NoError(t, seedTracker.ReplaceTodos([]ADKParityTodo{{
		ID: "1", Title: "Persisted", Status: "completed",
	}}))
	seed := seedTracker.Snapshot()
	require.NoError(t, seedTracker.SetCompletion(ADKParityCompletion{
		Status: "succeeded", Reason: "completed",
	}))
	seed = seedTracker.Snapshot()
	raw, err := (ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "coze-run-1",
		MessageType:     "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseTerminal,
		ParityState:     &seed,
	}).Marshal()
	require.NoError(t, err)

	service := &recordingADKCheckpointService{threadLatest: &CheckpointSummary{
		CheckpointID:    40,
		ThreadID:        42,
		RunID:           1,
		RuntimeType:     string(RuntimeModeEinoADK),
		RuntimeKey:      "coze-run-1",
		EnvelopeVersion: 2,
		ChannelValues:   string(raw),
	}}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 2, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	tracker, parentID, err := store.ParityStateTracker(context.Background())
	require.NoError(t, err)
	require.NotNil(t, service.listReq)
	require.Equal(t, string(RuntimeModeEinoADK), service.listReq.RuntimeType)
	require.Equal(t, int64(40), parentID)
	require.Equal(t, int64(2), tracker.Snapshot().LastRunID)
	require.Equal(t, []ADKParityTodo{{
		ID: "1", Title: "Persisted", Status: "completed",
	}}, tracker.Snapshot().Todos)
}

func TestADKCheckpointStoreRejectsCrossThreadParitySeed(t *testing.T) {
	seed := newTestADKParityStateTracker(t).Snapshot()
	seed.Completion = &ADKParityCompletion{
		RunID: 1, Status: "succeeded", Reason: "completed",
	}
	seed.ThreadID = 99
	seed.Workspace.ThreadID = 99
	seed.Workspace.Identity = "space:7/thread:99"
	raw, err := (ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK), RuntimeVersion: "0.9.9",
		RuntimeKey: "coze-run-1", MessageType: "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseTerminal, ParityState: &seed,
	}).Marshal()
	require.NoError(t, err)
	service := &recordingADKCheckpointService{threadLatest: &CheckpointSummary{
		CheckpointID: 40, ThreadID: 42, RunID: 1,
		RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "coze-run-1",
		EnvelopeVersion: 2, ChannelValues: string(raw),
	}}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 2, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	_, _, err = store.ParityStateTracker(context.Background())
	require.ErrorContains(t, err, "active thread")
}

func TestADKCheckpointStoreRejectsTerminalEnvelopeAsResumeBytes(t *testing.T) {
	state := newTestADKParityStateTracker(t).Snapshot()
	state.Completion = &ADKParityCompletion{
		RunID: 1, Status: "succeeded", Reason: "completed",
	}
	raw, err := (ADKCheckpointEnvelope{
		EnvelopeVersion: 2,
		Runtime:         string(RuntimeModeEinoADK), RuntimeVersion: "0.9.9",
		RuntimeKey: "checkpoint-1", MessageType: "schema.Message",
		CheckpointPhase: ADKCheckpointPhaseTerminal, ParityState: &state,
	}).Marshal()
	require.NoError(t, err)
	service := &recordingADKCheckpointService{latest: &CheckpointSummary{
		CheckpointID: 40, ThreadID: 42, RunID: 1,
		RuntimeType: string(RuntimeModeEinoADK), RuntimeKey: "checkpoint-1",
		EnvelopeVersion: 2, ChannelValues: string(raw),
	}}
	store, err := NewADKCheckpointStore(service, &RunSummary{
		RunID: 1, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	})
	require.NoError(t, err)

	_, _, err = store.Get(context.Background(), "checkpoint-1")
	require.ErrorContains(t, err, "terminal checkpoint is not resumable")
}

type recordedADKCheckpointCreate struct {
	createdID int64
	request   *CreateCheckpointRequest
}

type recordingADKJournalCheckpointStateReader struct {
	attempt *domainentity.RunAttempt
	ledgers []*domainentity.SideEffectLedger
}

func (r *recordingADKJournalCheckpointStateReader) GetActiveJournalAttempt(
	_ context.Context,
	_ int64,
) (*domainentity.RunAttempt, error) {
	return r.attempt, nil
}

func (r *recordingADKJournalCheckpointStateReader) ListSideEffectLedgers(
	_ context.Context,
	_ int64,
	_ string,
) ([]*domainentity.SideEffectLedger, error) {
	return r.ledgers, nil
}

type recordingADKCheckpointService struct {
	created        *CreateCheckpointRequest
	createdHistory []recordedADKCheckpointCreate
	listReq        *ListCheckpointsRequest
	latestReq      *GetLatestRuntimeCheckpointRequest
	latest         *CheckpointSummary
	threadLatest   *CheckpointSummary
	deleted        *DeleteRuntimeCheckpointRequest
}

func (s *recordingADKCheckpointService) CreateCheckpoint(
	ctx context.Context,
	req *CreateCheckpointRequest,
) (*CreateCheckpointResponse, error) {
	s.created = req
	checkpointID := int64(100 + len(s.createdHistory))
	s.latest = &CheckpointSummary{
		CheckpointID:    checkpointID,
		ThreadID:        req.ThreadID,
		RunID:           req.RunID,
		CheckpointNS:    req.CheckpointNS,
		ChannelValues:   req.ChannelValues,
		RuntimeType:     req.RuntimeType,
		RuntimeKey:      req.RuntimeKey,
		EnvelopeVersion: req.EnvelopeVersion,
		CreatedAt:       1234,
	}
	s.threadLatest = s.latest
	s.createdHistory = append(s.createdHistory, recordedADKCheckpointCreate{
		createdID: checkpointID,
		request:   req,
	})

	return &CreateCheckpointResponse{Checkpoint: s.latest}, nil
}

func (s *recordingADKCheckpointService) ListCheckpoints(
	ctx context.Context,
	req *ListCheckpointsRequest,
) (*ListCheckpointsResponse, error) {
	s.listReq = req
	checkpoints := []*CheckpointSummary{}
	if s.threadLatest != nil {
		checkpoints = append(checkpoints, s.threadLatest)
	}
	return &ListCheckpointsResponse{
		Checkpoints: checkpoints,
		Total:       int64(len(checkpoints)),
	}, nil
}

func (s *recordingADKCheckpointService) GetLatestRuntimeCheckpoint(
	ctx context.Context,
	req *GetLatestRuntimeCheckpointRequest,
) (*GetLatestRuntimeCheckpointResponse, error) {
	s.latestReq = req

	return &GetLatestRuntimeCheckpointResponse{Checkpoint: s.latest}, nil
}

func (s *recordingADKCheckpointService) DeleteRuntimeCheckpoint(
	ctx context.Context,
	req *DeleteRuntimeCheckpointRequest,
) error {
	s.deleted = req
	s.latest = nil

	return nil
}
