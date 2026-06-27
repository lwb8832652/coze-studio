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
				envelope.EnvelopeVersion = 2
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
		&RunSummary{RunID: 20, ThreadID: 10},
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
	require.Equal(t, int32(1), service.created.EnvelopeVersion)
	require.Equal(t, "eino.adk", service.created.CheckpointNS)
	require.Equal(t, `{}`, service.created.ChannelVersions)
	require.Equal(t, `[]`, service.created.PendingSends)
	require.JSONEq(t, `{
		"runtime":"eino_adk",
		"runtime_version":"0.9.9",
		"runtime_key":"checkpoint-1",
		"envelope_version":1,
		"message_type":"schema.Message"
	}`, service.created.Metadata)

	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, envelope.Checkpoint)
	require.Equal(t, int64(7), envelope.RunRevision)
	require.Equal(t, int64(1234), envelope.CreatedAt)

	require.NoError(t, store.RecordInterrupts(context.Background(), "checkpoint-1", []ADKInterruptItem{{
		ID:          "interrupt-1",
		Address:     "agent:lead;tool:approval",
		IsRootCause: true,
	}}))
	envelope, err = UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, ADKInterruptItem{
		ID:          "interrupt-1",
		Address:     "agent:lead;tool:approval",
		IsRootCause: true,
	}, envelope.Interrupts["interrupt-1"])

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
	store, err := NewADKCheckpointStore(service, &RunSummary{RunID: 20, ThreadID: 10})
	require.NoError(t, err)

	_, _, err = store.Get(context.Background(), "checkpoint-1")

	require.ErrorContains(t, err, "checkpoint runtime version 0.8.0 is incompatible with 0.9.9")
}

func TestADKCheckpointStoreNormalizesRuntimeKey(t *testing.T) {
	service := &recordingADKCheckpointService{}
	store, err := NewADKCheckpointStore(service, &RunSummary{RunID: 20, ThreadID: 10})
	require.NoError(t, err)

	require.NoError(t, store.Set(context.Background(), " checkpoint-1 ", []byte{1}))

	require.Equal(t, "checkpoint-1", service.created.RuntimeKey)
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(service.created.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, "checkpoint-1", envelope.RuntimeKey)
}

type recordingADKCheckpointService struct {
	created   *CreateCheckpointRequest
	latestReq *GetLatestRuntimeCheckpointRequest
	latest    *CheckpointSummary
	deleted   *DeleteRuntimeCheckpointRequest
}

func (s *recordingADKCheckpointService) CreateCheckpoint(
	ctx context.Context,
	req *CreateCheckpointRequest,
) (*CreateCheckpointResponse, error) {
	s.created = req
	s.latest = &CheckpointSummary{
		CheckpointID:    100,
		ThreadID:        req.ThreadID,
		RunID:           req.RunID,
		CheckpointNS:    req.CheckpointNS,
		ChannelValues:   req.ChannelValues,
		RuntimeType:     req.RuntimeType,
		RuntimeKey:      req.RuntimeKey,
		EnvelopeVersion: req.EnvelopeVersion,
		CreatedAt:       1234,
	}

	return &CreateCheckpointResponse{Checkpoint: s.latest}, nil
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
