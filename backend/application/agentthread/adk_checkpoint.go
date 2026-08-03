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
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const (
	adkCheckpointEnvelopeLegacyVersion  = 1
	adkCheckpointEnvelopeVersion        = 2
	adkJournalCheckpointEnvelopeVersion = 3
	adkJournalCheckpointSchemaVersion   = "coze.adk.checkpoint.v3"
	adkCheckpointRuntimeVersion         = "0.9.9"
	adkCheckpointMessageType            = "schema.Message"
	adkCheckpointNamespace              = "eino.adk"
	defaultADKCheckpointMaxBytes        = 32 << 20
	maxADKCheckpointKeyBytes            = 255
)

var ErrADKCheckpointRecoveryUnsafe = errors.New("checkpoint is not safe for journal recovery")

type ADKCheckpointPhase string

const (
	ADKCheckpointPhaseRuntime   ADKCheckpointPhase = "runtime"
	ADKCheckpointPhaseInterrupt ADKCheckpointPhase = "interrupt"
	ADKCheckpointPhaseTerminal  ADKCheckpointPhase = "terminal"
)

type ADKCheckpointEnvelope struct {
	EnvelopeVersion       int                            `json:"envelope_version"`
	SchemaVersion         string                         `json:"schema_version"`
	Runtime               string                         `json:"runtime"`
	RuntimeVersion        string                         `json:"runtime_version"`
	RuntimeKey            string                         `json:"runtime_key"`
	MessageType           string                         `json:"message_type"`
	CheckpointPhase       ADKCheckpointPhase             `json:"checkpoint_phase,omitempty"`
	Checkpoint            []byte                         `json:"checkpoint_bytes,omitempty"`
	RuntimeState          *ADKCheckpointRuntimeState     `json:"runtime_state,omitempty"`
	AttemptID             string                         `json:"attempt_id"`
	LastCommittedSequence uint64                         `json:"last_committed_sequence"`
	SideEffectLedger      []ADKSideEffectLedgerReference `json:"side_effect_ledger"`
	ParityState           *ADKParityState                `json:"parity_state,omitempty"`
	Interrupts            map[string]ADKInterruptItem    `json:"interrupts,omitempty"`
	RunRevision           int64                          `json:"run_revision"`
	CreatedAt             int64                          `json:"created_at"`
	Migration             map[string]string              `json:"migration,omitempty"`
}

type ADKCheckpointRuntimeState struct {
	Checkpoint []byte `json:"checkpoint_bytes"`
}

type ADKSideEffectLedgerReference struct {
	LedgerID                 int64  `json:"ledger_id"`
	IdempotencyKey           string `json:"idempotency_key"`
	ActionKind               string `json:"action_kind"`
	ReplayPolicy             string `json:"replay_policy"`
	Status                   string `json:"status"`
	Version                  uint64 `json:"version"`
	ResultEventID            int64  `json:"result_event_id,omitempty"`
	ResultSnapshotID         string `json:"result_snapshot_id,omitempty"`
	ResolutionAction         string `json:"resolution_action,omitempty"`
	ResolutionIdempotencyKey string `json:"resolution_idempotency_key,omitempty"`
}

type ADKRecoveryCheckpoint struct {
	AttemptID             string
	LastCommittedSequence uint64
	RuntimeCheckpoint     []byte
	SideEffectLedger      []ADKSideEffectLedgerReference
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
		envelope.EnvelopeVersion != adkCheckpointEnvelopeVersion &&
		envelope.EnvelopeVersion != adkJournalCheckpointEnvelopeVersion {
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
	checkpointBytes := envelope.Checkpoint
	if envelope.EnvelopeVersion == adkJournalCheckpointEnvelopeVersion {
		if strings.TrimSpace(envelope.SchemaVersion) != adkJournalCheckpointSchemaVersion {
			return fmt.Errorf("unsupported journal checkpoint schema version: %s", envelope.SchemaVersion)
		}
		if strings.TrimSpace(envelope.AttemptID) == "" {
			return fmt.Errorf("journal checkpoint attempt id is required")
		}
		if envelope.RuntimeState == nil {
			return fmt.Errorf("journal checkpoint runtime state is required")
		}
		if len(envelope.Checkpoint) != 0 {
			return fmt.Errorf("journal checkpoint cannot contain legacy checkpoint bytes")
		}
		if envelope.SideEffectLedger == nil {
			return fmt.Errorf("journal checkpoint side effect ledger is required")
		}
		if err := validateADKSideEffectLedgerReferences(envelope.SideEffectLedger); err != nil {
			return err
		}
		checkpointBytes = envelope.RuntimeState.Checkpoint
	}
	if len(checkpointBytes) > maxCheckpointBytes {
		return fmt.Errorf(
			"checkpoint exceeds maximum size: got %d bytes, maximum is %d",
			len(checkpointBytes),
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
		if len(checkpointBytes) == 0 {
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

func validateADKSideEffectLedgerReferences(refs []ADKSideEffectLedgerReference) error {
	seenIDs := make(map[int64]struct{}, len(refs))
	seenKeys := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key := strings.TrimSpace(ref.IdempotencyKey)
		if ref.LedgerID <= 0 || key == "" || strings.TrimSpace(ref.ActionKind) == "" ||
			ref.Version == 0 || ref.ResultEventID < 0 {
			return fmt.Errorf("journal checkpoint side effect ledger reference is incomplete")
		}
		switch strings.TrimSpace(ref.ReplayPolicy) {
		case "read_only", "idempotent_write", "non_replayable":
		default:
			return fmt.Errorf("journal checkpoint side effect replay policy is invalid")
		}
		switch strings.TrimSpace(ref.Status) {
		case "prepared", "executing", "succeeded", "failed", "unknown", "compensated":
		default:
			return fmt.Errorf("journal checkpoint side effect status is invalid")
		}
		resolutionAction := strings.TrimSpace(ref.ResolutionAction)
		resolutionKey := strings.TrimSpace(ref.ResolutionIdempotencyKey)
		if (resolutionAction == "") != (resolutionKey == "") {
			return fmt.Errorf("journal checkpoint side effect resolution is incomplete")
		}
		if resolutionAction != "" {
			switch resolutionAction {
			case "mark_succeeded", "skip", "retry":
			default:
				return fmt.Errorf("journal checkpoint side effect resolution is invalid")
			}
		}
		if _, exists := seenIDs[ref.LedgerID]; exists {
			return fmt.Errorf("journal checkpoint side effect ledger id is duplicated")
		}
		if _, exists := seenKeys[key]; exists {
			return fmt.Errorf("journal checkpoint side effect idempotency key is duplicated")
		}
		seenIDs[ref.LedgerID] = struct{}{}
		seenKeys[key] = struct{}{}
	}
	return nil
}

func DecodeADKRecoveryCheckpoint(
	envelope ADKCheckpointEnvelope,
) (*ADKRecoveryCheckpoint, error) {
	if envelope.EnvelopeVersion != adkJournalCheckpointEnvelopeVersion {
		return nil, ErrADKCheckpointRecoveryUnsafe
	}
	if err := validateADKCheckpointEnvelope(envelope, defaultADKCheckpointMaxBytes); err != nil {
		return nil, err
	}
	if envelope.CheckpointPhase == ADKCheckpointPhaseTerminal {
		return nil, ErrADKCheckpointRecoveryUnsafe
	}
	return &ADKRecoveryCheckpoint{
		AttemptID:             envelope.AttemptID,
		LastCommittedSequence: envelope.LastCommittedSequence,
		RuntimeCheckpoint:     append([]byte(nil), envelope.RuntimeState.Checkpoint...),
		SideEffectLedger:      append([]ADKSideEffectLedgerReference(nil), envelope.SideEffectLedger...),
	}, nil
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
	journalStateReader ADKJournalCheckpointStateReader
	sideEffectRepo     ADKSideEffectRepository
	sideEffectIDGen    ADKSideEffectIDGenerator
	sideEffectBoundary *ADKSideEffectBoundaryCoordinator
}

type ADKJournalCheckpointStateReader interface {
	GetActiveJournalAttempt(context.Context, int64) (*domainentity.RunAttempt, error)
	ListSideEffectLedgers(context.Context, int64, string) ([]*domainentity.SideEffectLedger, error)
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

func WithADKJournalCheckpointStateReader(
	reader ADKJournalCheckpointStateReader,
) ADKCheckpointStoreOption {
	return func(store *ADKCheckpointStore) error {
		if reader == nil {
			return fmt.Errorf("journal checkpoint state reader is required")
		}
		store.journalStateReader = reader
		return nil
	}
}

func WithADKSideEffectBoundary(
	repo ADKSideEffectRepository,
	idGen ADKSideEffectIDGenerator,
) ADKCheckpointStoreOption {
	return func(store *ADKCheckpointStore) error {
		if repo == nil || idGen == nil {
			return fmt.Errorf("journal side effect repository and id generator are required")
		}
		store.sideEffectRepo = repo
		store.sideEffectIDGen = idGen
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
	if (store.sideEffectRepo == nil) != (store.sideEffectIDGen == nil) {
		return nil, fmt.Errorf("journal side effect boundary dependencies are incomplete")
	}
	if store.sideEffectRepo != nil {
		coordinator, err := NewADKSideEffectBoundaryCoordinator(
			run, store.sideEffectRepo, store.sideEffectIDGen,
			WithADKSideEffectClock(store.now),
		)
		if err != nil {
			return nil, err
		}
		store.sideEffectBoundary = coordinator
	}

	return store, nil
}

func (s *ADKCheckpointStore) SideEffectBoundaryCoordinator() *ADKSideEffectBoundaryCoordinator {
	if s == nil {
		return nil
	}
	return s.sideEffectBoundary
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
	boundaryInput := ADKSideEffectCheckpointInput{
		RuntimeKey: checkpointID, RuntimeState: append([]byte(nil), checkpoint...),
		ParityState: &parityState, ParentCheckpointID: parentCheckpointID,
		RunRevision: s.runRevision, RuntimeVersion: s.runtimeVersion,
	}
	if s.sideEffectBoundary != nil {
		_, committed, err := s.sideEffectBoundary.CommitCheckpoint(ctx, boundaryInput)
		if err != nil {
			return fmt.Errorf("commit eino side effect checkpoint boundary: %w", err)
		}
		if committed {
			return nil
		}
	}

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
	if s.journalStateReader != nil {
		attempt, ledgers, enrolled, err := s.loadJournalCheckpointState(ctx)
		if err != nil {
			return err
		}
		if enrolled {
			envelope.EnvelopeVersion = adkJournalCheckpointEnvelopeVersion
			envelope.SchemaVersion = adkJournalCheckpointSchemaVersion
			envelope.Checkpoint = nil
			envelope.RuntimeState = &ADKCheckpointRuntimeState{Checkpoint: checkpoint}
			envelope.AttemptID = attempt.AttemptID
			envelope.LastCommittedSequence = attempt.LastCommittedSequence
			envelope.SideEffectLedger = adkSideEffectLedgerReferences(ledgers)
		}
	}
	raw, err := marshalADKCheckpointEnvelope(envelope, s.maxCheckpointBytes)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(map[string]any{
		"runtime":          string(RuntimeModeEinoADK),
		"runtime_version":  s.runtimeVersion,
		"runtime_key":      checkpointID,
		"envelope_version": envelope.EnvelopeVersion,
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
		EnvelopeVersion:    int32(envelope.EnvelopeVersion),
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
	if s.sideEffectBoundary != nil {
		s.sideEffectBoundary.RememberCheckpoint(
			boundaryInput,
			resp.Checkpoint.CheckpointID,
		)
	}

	return nil
}

func (s *ADKCheckpointStore) loadJournalCheckpointState(
	ctx context.Context,
) (*domainentity.RunAttempt, []*domainentity.SideEffectLedger, bool, error) {
	attempt, err := s.journalStateReader.GetActiveJournalAttempt(ctx, s.run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("load active journal checkpoint attempt: %w", err)
	}
	if attempt == nil || attempt.ThreadID != s.run.ThreadID ||
		attempt.ExecutionRunID != s.run.RunID || strings.TrimSpace(attempt.AttemptID) == "" {
		return nil, nil, false, fmt.Errorf("active journal checkpoint attempt does not belong to run")
	}
	ledgers, err := s.journalStateReader.ListSideEffectLedgers(
		ctx, attempt.JournalRunID, attempt.AttemptID,
	)
	if err != nil {
		return nil, nil, false, fmt.Errorf("load journal checkpoint side effect ledger: %w", err)
	}
	for _, ledger := range ledgers {
		if ledger == nil {
			return nil, nil, false, fmt.Errorf("journal checkpoint side effect ledger is incomplete")
		}
	}
	return attempt, ledgers, true, nil
}

func adkSideEffectLedgerReferences(
	ledgers []*domainentity.SideEffectLedger,
) []ADKSideEffectLedgerReference {
	refs := make([]ADKSideEffectLedgerReference, 0, len(ledgers))
	for _, ledger := range ledgers {
		if ledger == nil {
			continue
		}
		ref := ADKSideEffectLedgerReference{
			LedgerID: ledger.ID, IdempotencyKey: ledger.IdempotencyKey,
			ActionKind: ledger.ActionKind, ReplayPolicy: string(ledger.ReplayPolicy),
			Status: string(ledger.Status), Version: ledger.Version,
			ResultSnapshotID:         ledger.ResultSnapshotID,
			ResolutionAction:         string(ledger.ResolutionAction),
			ResolutionIdempotencyKey: ledger.ResolutionIdempotencyKey,
		}
		if ledger.ResultEventID != nil {
			ref.ResultEventID = *ledger.ResultEventID
		}
		refs = append(refs, ref)
	}
	return refs
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
	if envelope.EnvelopeVersion != adkCheckpointEnvelopeLegacyVersion &&
		envelope.CheckpointPhase == ADKCheckpointPhaseTerminal {
		return nil, false, fmt.Errorf("terminal checkpoint is not resumable")
	}
	runtimeCheckpoint := envelope.Checkpoint
	if envelope.EnvelopeVersion == adkJournalCheckpointEnvelopeVersion {
		runtimeCheckpoint = envelope.RuntimeState.Checkpoint
	}
	return runtimeCheckpoint, true, nil
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
	if envelope.EnvelopeVersion == adkCheckpointEnvelopeLegacyVersion {
		envelope.EnvelopeVersion = adkCheckpointEnvelopeVersion
	}
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
		EnvelopeVersion:    int32(envelope.EnvelopeVersion),
		ChannelValues:      string(raw),
		ChannelVersions:    `{}`,
		PendingSends:       `[]`,
		Metadata: adkCheckpointMetadataJSONVersion(
			s.runtimeVersion, checkpointID, ADKCheckpointPhaseInterrupt, envelope.EnvelopeVersion,
		),
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
	return adkCheckpointMetadataJSONVersion(
		runtimeVersion, runtimeKey, phase, adkCheckpointEnvelopeVersion,
	)
}

func adkCheckpointMetadataJSONVersion(
	runtimeVersion string,
	runtimeKey string,
	phase ADKCheckpointPhase,
	envelopeVersion int,
) string {
	raw, err := json.Marshal(map[string]any{
		"runtime":          string(RuntimeModeEinoADK),
		"runtime_version":  runtimeVersion,
		"runtime_key":      runtimeKey,
		"envelope_version": envelopeVersion,
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
