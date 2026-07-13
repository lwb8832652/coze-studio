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
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
)

const (
	adkCheckpointEnvelopeLegacyVersion = 1
	adkCheckpointEnvelopeVersion       = 2
	adkCheckpointRuntimeVersion        = "0.9.9"
	adkCheckpointMessageType           = "schema.Message"
	adkCheckpointNamespace             = "eino.adk"
	defaultADKCheckpointMaxBytes       = 32 << 20
	maxADKCheckpointKeyBytes           = 255
)

type ADKCheckpointPhase string

const (
	ADKCheckpointPhaseRuntime   ADKCheckpointPhase = "runtime"
	ADKCheckpointPhaseInterrupt ADKCheckpointPhase = "interrupt"
	ADKCheckpointPhaseTerminal  ADKCheckpointPhase = "terminal"
)

type ADKCheckpointEnvelope struct {
	EnvelopeVersion int                         `json:"envelope_version"`
	Runtime         string                      `json:"runtime"`
	RuntimeVersion  string                      `json:"runtime_version"`
	RuntimeKey      string                      `json:"runtime_key"`
	MessageType     string                      `json:"message_type"`
	CheckpointPhase ADKCheckpointPhase          `json:"checkpoint_phase,omitempty"`
	Checkpoint      []byte                      `json:"checkpoint_bytes"`
	ParityState     *ADKParityState             `json:"parity_state,omitempty"`
	Interrupts      map[string]ADKInterruptItem `json:"interrupts,omitempty"`
	RunRevision     int64                       `json:"run_revision"`
	CreatedAt       int64                       `json:"created_at"`
	Migration       map[string]string           `json:"migration,omitempty"`
}

func (e ADKCheckpointEnvelope) Marshal() ([]byte, error) {
	return marshalADKCheckpointEnvelope(e, defaultADKCheckpointMaxBytes)
}

func UnmarshalADKCheckpointEnvelope(raw []byte) (ADKCheckpointEnvelope, error) {
	return unmarshalADKCheckpointEnvelope(raw, defaultADKCheckpointMaxBytes)
}

func marshalADKCheckpointEnvelope(envelope ADKCheckpointEnvelope, maxCheckpointBytes int) ([]byte, error) {
	if err := validateADKCheckpointEnvelope(envelope, maxCheckpointBytes); err != nil {
		return nil, err
	}

	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint envelope: %w", err)
	}

	return raw, nil
}

func unmarshalADKCheckpointEnvelope(raw []byte, maxCheckpointBytes int) (ADKCheckpointEnvelope, error) {
	var envelope ADKCheckpointEnvelope
	if maxCheckpointBytes <= 0 {
		return envelope, fmt.Errorf("maximum checkpoint size must be positive")
	}
	maxEnvelopeBytes := maxCheckpointBytes + maxCheckpointBytes/2 + maxADKParityStateBytes + 64*1024
	if len(raw) > maxEnvelopeBytes {
		return envelope, fmt.Errorf("checkpoint envelope exceeds maximum encoded size of %d bytes", maxEnvelopeBytes)
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return envelope, fmt.Errorf("unmarshal checkpoint envelope: %w", err)
	}
	if err := validateADKCheckpointEnvelope(envelope, maxCheckpointBytes); err != nil {
		return envelope, err
	}

	return envelope, nil
}

func validateADKCheckpointEnvelope(envelope ADKCheckpointEnvelope, maxCheckpointBytes int) error {
	if maxCheckpointBytes <= 0 {
		return fmt.Errorf("maximum checkpoint size must be positive")
	}
	if envelope.EnvelopeVersion != adkCheckpointEnvelopeLegacyVersion &&
		envelope.EnvelopeVersion != adkCheckpointEnvelopeVersion {
		return fmt.Errorf("unsupported checkpoint envelope version: %d", envelope.EnvelopeVersion)
	}
	if strings.TrimSpace(envelope.Runtime) != string(RuntimeModeEinoADK) {
		return fmt.Errorf("unsupported checkpoint runtime: %s", envelope.Runtime)
	}
	if strings.TrimSpace(envelope.RuntimeVersion) == "" {
		return fmt.Errorf("checkpoint runtime version is required")
	}
	if strings.TrimSpace(envelope.RuntimeKey) == "" {
		return fmt.Errorf("checkpoint runtime key is required")
	}
	if envelope.MessageType != adkCheckpointMessageType {
		return fmt.Errorf("unsupported checkpoint message type: %s", envelope.MessageType)
	}
	if len(envelope.Checkpoint) > maxCheckpointBytes {
		return fmt.Errorf(
			"checkpoint exceeds maximum size: got %d bytes, maximum is %d",
			len(envelope.Checkpoint),
			maxCheckpointBytes,
		)
	}
	if envelope.RunRevision < 0 {
		return fmt.Errorf("checkpoint run revision must not be negative")
	}
	if envelope.EnvelopeVersion == adkCheckpointEnvelopeLegacyVersion {
		if len(envelope.Checkpoint) == 0 {
			return fmt.Errorf("checkpoint bytes are required")
		}
		return nil
	}
	if envelope.ParityState == nil {
		return fmt.Errorf("checkpoint parity state is required")
	}
	if err := validateADKParityStateSnapshot(envelope.ParityState); err != nil {
		return err
	}
	switch envelope.CheckpointPhase {
	case ADKCheckpointPhaseRuntime, ADKCheckpointPhaseInterrupt:
		if len(envelope.Checkpoint) == 0 {
			return fmt.Errorf("checkpoint bytes are required for %s phase", envelope.CheckpointPhase)
		}
	case ADKCheckpointPhaseTerminal:
		if envelope.ParityState.Completion == nil {
			return fmt.Errorf("terminal checkpoint completion is required")
		}
	default:
		return fmt.Errorf("unsupported checkpoint phase: %s", envelope.CheckpointPhase)
	}

	return nil
}

type ADKCheckpointService interface {
	CreateCheckpoint(ctx context.Context, req *CreateCheckpointRequest) (*CreateCheckpointResponse, error)
	GetLatestRuntimeCheckpoint(
		ctx context.Context,
		req *GetLatestRuntimeCheckpointRequest,
	) (*GetLatestRuntimeCheckpointResponse, error)
	DeleteRuntimeCheckpoint(ctx context.Context, req *DeleteRuntimeCheckpointRequest) error
}

type ADKCheckpointInterruptRecorder interface {
	RecordInterrupts(
		ctx context.Context,
		checkpointID string,
		interrupts []ADKInterruptItem,
	) error
}

type ADKCheckpointThreadStateService interface {
	ListCheckpoints(
		ctx context.Context,
		req *ListCheckpointsRequest,
	) (*ListCheckpointsResponse, error)
}

type ADKParityCheckpointStore interface {
	ParityStateTracker(ctx context.Context) (*ADKParityStateTracker, int64, error)
	SetParityStateTracker(tracker *ADKParityStateTracker, parentCheckpointID int64) error
}

type ADKCheckpointStore struct {
	service            ADKCheckpointService
	run                *RunSummary
	runtimeVersion     string
	maxCheckpointBytes int
	runRevision        int64
	now                func() int64
	parityMu           sync.Mutex
	parityTracker      *ADKParityStateTracker
	parityParentID     int64
}

type ADKCheckpointStoreOption func(*ADKCheckpointStore) error

func WithADKCheckpointMaxBytes(maxBytes int) ADKCheckpointStoreOption {
	return func(store *ADKCheckpointStore) error {
		if maxBytes <= 0 {
			return fmt.Errorf("maximum checkpoint size must be positive")
		}
		store.maxCheckpointBytes = maxBytes

		return nil
	}
}

func WithADKCheckpointClock(now func() int64) ADKCheckpointStoreOption {
	return func(store *ADKCheckpointStore) error {
		if now == nil {
			return fmt.Errorf("checkpoint clock is required")
		}
		store.now = now

		return nil
	}
}

func WithADKCheckpointRunRevision(revision int64) ADKCheckpointStoreOption {
	return func(store *ADKCheckpointStore) error {
		if revision < 0 {
			return fmt.Errorf("checkpoint run revision must not be negative")
		}
		store.runRevision = revision

		return nil
	}
}

func WithADKCheckpointRuntimeVersion(version string) ADKCheckpointStoreOption {
	return func(store *ADKCheckpointStore) error {
		version = strings.TrimSpace(version)
		if version == "" {
			return fmt.Errorf("checkpoint runtime version is required")
		}
		store.runtimeVersion = version

		return nil
	}
}

func NewADKCheckpointStore(
	service ADKCheckpointService,
	run *RunSummary,
	options ...ADKCheckpointStoreOption,
) (*ADKCheckpointStore, error) {
	if service == nil {
		return nil, fmt.Errorf("agent checkpoint service is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.ThreadID <= 0 {
		return nil, fmt.Errorf("run thread id is required")
	}
	if run.RunID <= 0 {
		return nil, fmt.Errorf("run id is required")
	}

	store := &ADKCheckpointStore{
		service:            service,
		run:                run,
		runtimeVersion:     adkCheckpointRuntimeVersion,
		maxCheckpointBytes: defaultADKCheckpointMaxBytes,
		now: func() int64 {
			return time.Now().UnixMilli()
		},
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(store); err != nil {
			return nil, err
		}
	}

	return store, nil
}

func (s *ADKCheckpointStore) Set(ctx context.Context, checkpointID string, checkpoint []byte) error {
	checkpointID, err := s.normalizeKey(checkpointID)
	if err != nil {
		return err
	}
	if len(checkpoint) == 0 {
		return fmt.Errorf("checkpoint bytes are required")
	}
	if len(checkpoint) > s.maxCheckpointBytes {
		return fmt.Errorf(
			"checkpoint exceeds maximum size: got %d bytes, maximum is %d",
			len(checkpoint),
			s.maxCheckpointBytes,
		)
	}

	tracker, parityParentID, err := s.ParityStateTracker(ctx)
	if err != nil {
		return err
	}
	parentCheckpointID := parityParentID
	latest, err := s.service.GetLatestRuntimeCheckpoint(ctx, &GetLatestRuntimeCheckpointRequest{
		ThreadID:    s.run.ThreadID,
		RunID:       s.run.RunID,
		RuntimeType: string(RuntimeModeEinoADK),
		RuntimeKey:  checkpointID,
	})
	if err != nil {
		return fmt.Errorf("get parent runtime checkpoint: %w", err)
	}
	if latest != nil && latest.Checkpoint != nil {
		parentCheckpointID = latest.Checkpoint.CheckpointID
	}
	parityState := tracker.Snapshot()

	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkCheckpointEnvelopeVersion,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  s.runtimeVersion,
		RuntimeKey:      checkpointID,
		MessageType:     adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		Checkpoint:      checkpoint,
		ParityState:     &parityState,
		RunRevision:     s.runRevision,
		CreatedAt:       s.now(),
	}
	raw, err := marshalADKCheckpointEnvelope(envelope, s.maxCheckpointBytes)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"runtime":          string(RuntimeModeEinoADK),
		"runtime_version":  s.runtimeVersion,
		"runtime_key":      checkpointID,
		"envelope_version": adkCheckpointEnvelopeVersion,
		"message_type":     adkCheckpointMessageType,
		"checkpoint_phase": ADKCheckpointPhaseRuntime,
	})
	if err != nil {
		return fmt.Errorf("marshal checkpoint metadata: %w", err)
	}

	resp, err := s.service.CreateCheckpoint(ctx, &CreateCheckpointRequest{
		ThreadID:           s.run.ThreadID,
		RunID:              s.run.RunID,
		ParentCheckpointID: parentCheckpointID,
		CheckpointNS:       adkCheckpointNamespace,
		RuntimeType:        string(RuntimeModeEinoADK),
		RuntimeKey:         checkpointID,
		EnvelopeVersion:    adkCheckpointEnvelopeVersion,
		ChannelValues:      string(raw),
		ChannelVersions:    `{}`,
		PendingSends:       `[]`,
		Metadata:           string(metadata),
	})
	if err != nil {
		return fmt.Errorf("persist eino checkpoint: %w", err)
	}
	if resp == nil || resp.Checkpoint == nil {
		return fmt.Errorf("persist eino checkpoint returned empty response")
	}

	return nil
}

func (s *ADKCheckpointStore) Get(ctx context.Context, checkpointID string) ([]byte, bool, error) {
	checkpointID, err := s.normalizeKey(checkpointID)
	if err != nil {
		return nil, false, err
	}

	resp, err := s.service.GetLatestRuntimeCheckpoint(ctx, &GetLatestRuntimeCheckpointRequest{
		ThreadID:    s.run.ThreadID,
		RunID:       s.run.RunID,
		RuntimeType: string(RuntimeModeEinoADK),
		RuntimeKey:  checkpointID,
	})
	if err != nil {
		return nil, false, fmt.Errorf("load eino checkpoint: %w", err)
	}
	if resp == nil || resp.Checkpoint == nil {
		return nil, false, nil
	}
	checkpoint := resp.Checkpoint
	if checkpoint.ThreadID != s.run.ThreadID || checkpoint.RunID != s.run.RunID {
		return nil, false, fmt.Errorf("runtime checkpoint does not belong to the active run")
	}
	if checkpoint.RuntimeType != string(RuntimeModeEinoADK) || checkpoint.RuntimeKey != checkpointID {
		return nil, false, fmt.Errorf("runtime checkpoint identity does not match requested key")
	}

	envelope, err := unmarshalADKCheckpointEnvelope(
		[]byte(checkpoint.ChannelValues),
		s.maxCheckpointBytes,
	)
	if err != nil {
		return nil, false, fmt.Errorf("decode eino checkpoint: %w", err)
	}
	if envelope.RuntimeKey != checkpointID {
		return nil, false, fmt.Errorf("checkpoint envelope runtime key does not match requested key")
	}
	if envelope.RuntimeVersion != s.runtimeVersion {
		return nil, false, fmt.Errorf(
			"checkpoint runtime version %s is incompatible with %s",
			envelope.RuntimeVersion,
			s.runtimeVersion,
		)
	}
	if checkpoint.EnvelopeVersion != 0 &&
		int(checkpoint.EnvelopeVersion) != envelope.EnvelopeVersion {
		return nil, false, fmt.Errorf("checkpoint envelope version does not match indexed version")
	}
	if envelope.EnvelopeVersion == adkCheckpointEnvelopeVersion &&
		envelope.CheckpointPhase == ADKCheckpointPhaseTerminal {
		return nil, false, fmt.Errorf("terminal checkpoint is not resumable")
	}

	return envelope.Checkpoint, true, nil
}

func (s *ADKCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	checkpointID, err := s.normalizeKey(checkpointID)
	if err != nil {
		return err
	}

	if err := s.service.DeleteRuntimeCheckpoint(ctx, &DeleteRuntimeCheckpointRequest{
		ThreadID:    s.run.ThreadID,
		RunID:       s.run.RunID,
		RuntimeType: string(RuntimeModeEinoADK),
		RuntimeKey:  checkpointID,
		DeletedAt:   s.now(),
	}); err != nil {
		return fmt.Errorf("delete eino checkpoint: %w", err)
	}

	return nil
}

func (s *ADKCheckpointStore) RecordInterrupts(
	ctx context.Context,
	checkpointID string,
	interrupts []ADKInterruptItem,
) error {
	checkpointID, err := s.normalizeKey(checkpointID)
	if err != nil {
		return err
	}
	if len(interrupts) == 0 {
		return nil
	}
	tracker, _, err := s.ParityStateTracker(ctx)
	if err != nil {
		return err
	}
	parityInterrupts := make([]ADKParityInterrupt, 0, len(interrupts))
	for _, interrupt := range interrupts {
		parityInterrupts = append(parityInterrupts, ADKParityInterrupt{
			ID:          interrupt.ID,
			Address:     interrupt.Address,
			IsRootCause: interrupt.IsRootCause,
			ParentID:    interrupt.ParentID,
		})
	}
	if err := tracker.ReplaceInterrupts(parityInterrupts); err != nil {
		return fmt.Errorf("record eino parity interrupt targets: %w", err)
	}
	if err := tracker.SetCompletion(ADKParityCompletion{
		RunID:       s.run.RunID,
		Status:      "interrupted",
		Reason:      "human_interaction",
		CompletedAt: s.now(),
	}); err != nil {
		return fmt.Errorf("record eino parity interrupt completion: %w", err)
	}

	resp, err := s.service.GetLatestRuntimeCheckpoint(ctx, &GetLatestRuntimeCheckpointRequest{
		ThreadID:    s.run.ThreadID,
		RunID:       s.run.RunID,
		RuntimeType: string(RuntimeModeEinoADK),
		RuntimeKey:  checkpointID,
	})
	if err != nil {
		return fmt.Errorf("load eino checkpoint for interrupt targets: %w", err)
	}
	if resp == nil || resp.Checkpoint == nil {
		return fmt.Errorf("eino checkpoint is missing for interrupt targets")
	}
	latest := resp.Checkpoint
	envelope, err := unmarshalADKCheckpointEnvelope(
		[]byte(latest.ChannelValues),
		s.maxCheckpointBytes,
	)
	if err != nil {
		return fmt.Errorf("decode eino checkpoint for interrupt targets: %w", err)
	}
	envelope.Interrupts = make(map[string]ADKInterruptItem, len(interrupts))
	for _, interrupt := range interrupts {
		key := strings.TrimSpace(interrupt.ID)
		if key == "" {
			key = strings.TrimSpace(interrupt.Address)
		}
		if key != "" {
			envelope.Interrupts[key] = interrupt
		}
	}
	if len(envelope.Interrupts) == 0 {
		return fmt.Errorf("eino interrupt targets are missing stable identities")
	}
	envelope.EnvelopeVersion = adkCheckpointEnvelopeVersion
	envelope.CheckpointPhase = ADKCheckpointPhaseInterrupt
	parityState := tracker.Snapshot()
	envelope.ParityState = &parityState

	raw, err := marshalADKCheckpointEnvelope(envelope, s.maxCheckpointBytes)
	if err != nil {
		return err
	}
	created, err := s.service.CreateCheckpoint(ctx, &CreateCheckpointRequest{
		ThreadID:           s.run.ThreadID,
		RunID:              s.run.RunID,
		ParentCheckpointID: latest.CheckpointID,
		CheckpointNS:       adkCheckpointNamespace,
		RuntimeType:        string(RuntimeModeEinoADK),
		RuntimeKey:         checkpointID,
		EnvelopeVersion:    adkCheckpointEnvelopeVersion,
		ChannelValues:      string(raw),
		ChannelVersions:    `{}`,
		PendingSends:       `[]`,
		Metadata:           adkCheckpointMetadataJSON(s.runtimeVersion, checkpointID, ADKCheckpointPhaseInterrupt),
	})
	if err != nil {
		return fmt.Errorf("persist eino interrupt targets: %w", err)
	}
	if created == nil || created.Checkpoint == nil {
		return fmt.Errorf("persist eino interrupt targets returned empty response")
	}

	return nil
}

func (s *ADKCheckpointStore) ParityStateTracker(
	ctx context.Context,
) (*ADKParityStateTracker, int64, error) {
	if s == nil || s.run == nil {
		return nil, 0, fmt.Errorf("eino adk checkpoint store is required")
	}
	s.parityMu.Lock()
	defer s.parityMu.Unlock()
	if s.parityTracker != nil {
		return s.parityTracker, s.parityParentID, nil
	}

	var seed *ADKParityState
	parentCheckpointID := int64(0)
	if source, ok := s.service.(ADKCheckpointThreadStateService); ok {
		resp, err := source.ListCheckpoints(ctx, &ListCheckpointsRequest{
			ThreadID:    s.run.ThreadID,
			RuntimeType: string(RuntimeModeEinoADK),
			Limit:       1,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("load latest eino parity checkpoint: %w", err)
		}
		if resp != nil && len(resp.Checkpoints) > 0 && resp.Checkpoints[0] != nil {
			checkpoint := resp.Checkpoints[0]
			if checkpoint.ThreadID != s.run.ThreadID {
				return nil, 0, fmt.Errorf("latest parity checkpoint does not belong to the active thread")
			}
			parentCheckpointID = checkpoint.CheckpointID
			if strings.TrimSpace(checkpoint.RuntimeType) == string(RuntimeModeEinoADK) {
				envelope, err := unmarshalADKCheckpointEnvelope(
					[]byte(checkpoint.ChannelValues),
					s.maxCheckpointBytes,
				)
				if err != nil {
					return nil, 0, fmt.Errorf("decode latest eino parity checkpoint: %w", err)
				}
				if checkpoint.EnvelopeVersion != 0 &&
					int(checkpoint.EnvelopeVersion) != envelope.EnvelopeVersion {
					return nil, 0, fmt.Errorf("latest parity checkpoint envelope version does not match indexed version")
				}
				if envelope.ParityState != nil {
					copy := cloneADKParityState(*envelope.ParityState)
					seed = &copy
				}
			}
		}
	}

	tracker, err := NewADKParityStateTracker(s.run, seed)
	if err != nil {
		return nil, 0, err
	}
	s.parityTracker = tracker
	s.parityParentID = parentCheckpointID
	return tracker, parentCheckpointID, nil
}

func (s *ADKCheckpointStore) SetParityStateTracker(
	tracker *ADKParityStateTracker,
	parentCheckpointID int64,
) error {
	if s == nil || s.run == nil || tracker == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	if parentCheckpointID < 0 {
		return fmt.Errorf("eino adk parity parent checkpoint id is invalid")
	}
	snapshot := tracker.Snapshot()
	if snapshot.ThreadID != s.run.ThreadID || snapshot.SpaceID != s.run.SpaceID {
		return fmt.Errorf("eino adk parity state does not belong to the active thread")
	}
	if err := validateADKParityStateSnapshot(&snapshot); err != nil {
		return err
	}
	s.parityMu.Lock()
	s.parityTracker = tracker
	s.parityParentID = parentCheckpointID
	s.parityMu.Unlock()
	return nil
}

func adkCheckpointMetadataJSON(
	runtimeVersion string,
	runtimeKey string,
	phase ADKCheckpointPhase,
) string {
	raw, err := json.Marshal(map[string]any{
		"runtime":          string(RuntimeModeEinoADK),
		"runtime_version":  runtimeVersion,
		"runtime_key":      runtimeKey,
		"envelope_version": adkCheckpointEnvelopeVersion,
		"message_type":     adkCheckpointMessageType,
		"checkpoint_phase": phase,
	})
	if err != nil {
		return `{}`
	}
	return string(raw)
}

func validateADKTerminalCheckpointRequest(
	req *CreateCheckpointRequest,
	threadID int64,
	runID int64,
) error {
	if req == nil {
		return nil
	}
	if threadID <= 0 || runID <= 0 || req.ThreadID != threadID || req.RunID != runID ||
		req.ParentCheckpointID < 0 {
		return fmt.Errorf("terminal eino adk checkpoint ownership is invalid")
	}
	runtimeKey := adkCheckpointKeyForRun(runID)
	if strings.TrimSpace(req.CheckpointNS) != adkCheckpointNamespace ||
		strings.TrimSpace(req.RuntimeType) != string(RuntimeModeEinoADK) ||
		strings.TrimSpace(req.RuntimeKey) != runtimeKey ||
		int(req.EnvelopeVersion) != adkCheckpointEnvelopeVersion {
		return fmt.Errorf("terminal eino adk checkpoint identity is invalid")
	}
	var versions map[string]any
	if err := json.Unmarshal([]byte(req.ChannelVersions), &versions); err != nil || len(versions) != 0 {
		return fmt.Errorf("terminal eino adk checkpoint channel versions are invalid")
	}
	var pending []any
	if err := json.Unmarshal([]byte(req.PendingSends), &pending); err != nil || len(pending) != 0 {
		return fmt.Errorf("terminal eino adk checkpoint pending sends are invalid")
	}
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(req.ChannelValues))
	if err != nil {
		return fmt.Errorf("decode terminal eino adk checkpoint: %w", err)
	}
	if envelope.EnvelopeVersion != adkCheckpointEnvelopeVersion ||
		envelope.CheckpointPhase != ADKCheckpointPhaseTerminal ||
		envelope.RuntimeKey != runtimeKey || len(envelope.Checkpoint) != 0 ||
		envelope.ParityState == nil || envelope.ParityState.ThreadID != threadID ||
		envelope.ParityState.LastRunID != runID ||
		envelope.ParityState.Completion == nil ||
		envelope.ParityState.Completion.RunID != runID ||
		envelope.ParityState.Completion.Status != "succeeded" {
		return fmt.Errorf("terminal eino adk checkpoint state is invalid")
	}
	return nil
}

func sameADKTerminalCheckpointIdentity(left, right *CreateCheckpointRequest) bool {
	if left == nil || right == nil {
		return false
	}
	return left.ThreadID == right.ThreadID && left.RunID == right.RunID &&
		left.ParentCheckpointID == right.ParentCheckpointID &&
		strings.TrimSpace(left.CheckpointNS) == strings.TrimSpace(right.CheckpointNS) &&
		strings.TrimSpace(left.RuntimeType) == strings.TrimSpace(right.RuntimeType) &&
		strings.TrimSpace(left.RuntimeKey) == strings.TrimSpace(right.RuntimeKey) &&
		left.EnvelopeVersion == right.EnvelopeVersion &&
		strings.TrimSpace(left.ChannelVersions) == strings.TrimSpace(right.ChannelVersions) &&
		strings.TrimSpace(left.PendingSends) == strings.TrimSpace(right.PendingSends) &&
		strings.TrimSpace(left.Metadata) == strings.TrimSpace(right.Metadata)
}

func (s *ADKCheckpointStore) normalizeKey(checkpointID string) (string, error) {
	checkpointID = strings.TrimSpace(checkpointID)
	if checkpointID == "" {
		return "", fmt.Errorf("checkpoint runtime key is required")
	}
	if len(checkpointID) > maxADKCheckpointKeyBytes {
		return "", fmt.Errorf("checkpoint runtime key exceeds %d bytes", maxADKCheckpointKeyBytes)
	}

	return checkpointID, nil
}

var _ adk.CheckPointStore = (*ADKCheckpointStore)(nil)
var _ adk.CheckPointDeleter = (*ADKCheckpointStore)(nil)
var _ ADKCheckpointInterruptRecorder = (*ADKCheckpointStore)(nil)
var _ ADKParityCheckpointStore = (*ADKCheckpointStore)(nil)
