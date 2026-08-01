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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

var ErrADKSideEffectReplayBlocked = errors.New(
	"eino adk side effect replay is blocked",
)

var adkSideEffectRegisteredPolicies = map[string]domainentity.SideEffectReplayPolicy{
	"read_file":            domainentity.SideEffectReplayPolicyReadOnly,
	"web_search":           domainentity.SideEffectReplayPolicyReadOnly,
	"web_fetch":            domainentity.SideEffectReplayPolicyReadOnly,
	"skill":                domainentity.SideEffectReplayPolicyReadOnly,
	"tool_search":          domainentity.SideEffectReplayPolicyReadOnly,
	"write_file":           domainentity.SideEffectReplayPolicyIdempotentWrite,
	"present_files":        domainentity.SideEffectReplayPolicyIdempotentWrite,
	"create_skill_package": domainentity.SideEffectReplayPolicyNonReplayable,
}

func ClassifyADKSideEffectTool(
	name string,
) (domainentity.SideEffectReplayPolicy, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if policy, ok := adkSideEffectRegisteredPolicies[name]; ok {
		return policy, true
	}
	return domainentity.SideEffectReplayPolicyNonReplayable, false
}

type ADKSideEffectRepository interface {
	GetActiveJournalAttempt(context.Context, int64) (*domainentity.RunAttempt, error)
	GetSideEffectLedger(context.Context, int64, string, string) (*domainentity.SideEffectLedger, error)
	PrepareSideEffect(
		context.Context,
		domainrepo.PrepareSideEffectRequest,
	) (*domainentity.SideEffectLedger, bool, error)
	TransitionSideEffect(
		context.Context,
		domainrepo.TransitionSideEffectRequest,
	) (*domainentity.SideEffectLedger, bool, error)
	CommitExecutionBoundary(
		context.Context,
		domainrepo.CommitExecutionBoundaryRequest,
	) (*domainrepo.CommitExecutionBoundaryResult, error)
}

type ADKSideEffectIDGenerator interface {
	GenID(context.Context) (int64, error)
}

type ADKSideEffectBoundaryCoordinatorOption func(*ADKSideEffectBoundaryCoordinator) error

func WithADKSideEffectClock(
	now func() int64,
) ADKSideEffectBoundaryCoordinatorOption {
	return func(coordinator *ADKSideEffectBoundaryCoordinator) error {
		if now == nil {
			return fmt.Errorf("eino adk side effect clock is required")
		}
		coordinator.now = now
		return nil
	}
}

type ADKSideEffectCheckpointInput struct {
	RuntimeKey         string
	RuntimeState       []byte
	ParityState        *ADKParityState
	ParentCheckpointID int64
	RunRevision        int64
	RuntimeVersion     string
}

type adkSideEffectInvocation struct {
	attempt        *domainentity.RunAttempt
	ledger         *domainentity.SideEffectLedger
	recoveryAction domainentity.SideEffectResolutionAction
}

type adkSideEffectRecoveryDirective struct {
	LedgerID                 int64  `json:"ledger_id"`
	SourceIdempotencyKey     string `json:"source_idempotency_key"`
	ActionKind               string `json:"action_kind"`
	RequestHash              string `json:"request_hash"`
	Action                   string `json:"action"`
	ResolutionIdempotencyKey string `json:"resolution_idempotency_key"`
}

type adkSideEffectRecoveryState struct {
	journalRunID    int64
	sourceAttemptID string
	directives      map[string]adkSideEffectRecoveryDirective
}

type adkPendingSideEffect struct {
	invocation              *adkSideEffectInvocation
	status                  domainentity.SideEffectLedgerStatus
	externalReferenceDigest string
	resultEvent             *domainentity.JournalEvent
	auditEvent              *domainentity.JournalEvent
	checkpointID            int64
}

type ADKSideEffectBoundaryCoordinator struct {
	run   *RunSummary
	repo  ADKSideEffectRepository
	idGen ADKSideEffectIDGenerator
	now   func() int64

	mu                 sync.Mutex
	pending            []*adkPendingSideEffect
	latest             *ADKSideEffectCheckpointInput
	latestCheckpointID int64
	recovery           *adkSideEffectRecoveryState
}

type adkSideEffectBoundaryCoordinatorContextKey struct{}

func withADKSideEffectBoundaryCoordinator(
	ctx context.Context,
	coordinator *ADKSideEffectBoundaryCoordinator,
) context.Context {
	if coordinator == nil {
		return ctx
	}
	return context.WithValue(ctx, adkSideEffectBoundaryCoordinatorContextKey{}, coordinator)
}

func adkSideEffectBoundaryCoordinatorFromContext(
	ctx context.Context,
) *ADKSideEffectBoundaryCoordinator {
	if ctx == nil {
		return nil
	}
	coordinator, _ := ctx.Value(adkSideEffectBoundaryCoordinatorContextKey{}).(*ADKSideEffectBoundaryCoordinator)
	return coordinator
}

func NewADKSideEffectBoundaryCoordinator(
	run *RunSummary,
	repo ADKSideEffectRepository,
	idGen ADKSideEffectIDGenerator,
	options ...ADKSideEffectBoundaryCoordinatorOption,
) (*ADKSideEffectBoundaryCoordinator, error) {
	if run == nil || run.RunID <= 0 || run.ThreadID <= 0 {
		return nil, fmt.Errorf("eino adk side effect run is required")
	}
	if repo == nil || idGen == nil {
		return nil, fmt.Errorf("eino adk side effect repository and id generator are required")
	}
	coordinator := &ADKSideEffectBoundaryCoordinator{
		run: run, repo: repo, idGen: idGen,
		now: func() int64 { return time.Now().UnixMilli() },
	}
	recovery, err := parseADKSideEffectRecoveryState(run)
	if err != nil {
		return nil, err
	}
	coordinator.recovery = recovery
	for _, option := range options {
		if option != nil {
			if err := option(coordinator); err != nil {
				return nil, err
			}
		}
	}
	return coordinator, nil
}

func parseADKSideEffectRecoveryState(
	run *RunSummary,
) (*adkSideEffectRecoveryState, error) {
	if run == nil {
		return nil, nil
	}
	if strings.TrimSpace(run.Metadata) == "" {
		if hasADKSideEffectRecoveryCommand(run.Command) {
			return nil, fmt.Errorf("eino adk journal recovery metadata is missing")
		}
		return nil, nil
	}
	var metadata struct {
		JournalRecovery *struct {
			Schema          string `json:"schema"`
			JournalRunID    int64  `json:"journal_run_id"`
			SourceAttemptID string `json:"source_attempt_id"`
			Action          string `json:"action"`
		} `json:"journal_recovery"`
	}
	metadataErr := json.Unmarshal([]byte(run.Metadata), &metadata)
	if metadataErr != nil || metadata.JournalRecovery == nil {
		if hasADKSideEffectRecoveryCommand(run.Command) {
			return nil, fmt.Errorf("eino adk journal recovery metadata is missing")
		}
		return nil, nil
	}
	marker := metadata.JournalRecovery
	marker.Schema = strings.TrimSpace(marker.Schema)
	marker.SourceAttemptID = strings.TrimSpace(marker.SourceAttemptID)
	marker.Action = strings.TrimSpace(marker.Action)
	if marker.Schema != "coze.journal.recovery.v1" || marker.JournalRunID <= 0 ||
		marker.SourceAttemptID == "" {
		return nil, fmt.Errorf("eino adk journal recovery metadata is invalid")
	}

	var command struct {
		Resume *struct {
			Journal *struct {
				JournalRunID    int64                            `json:"journal_run_id"`
				SourceAttemptID string                           `json:"source_attempt_id"`
				Action          string                           `json:"action"`
				Resolutions     []adkSideEffectRecoveryDirective `json:"ledger_resolutions"`
			} `json:"journal"`
		} `json:"resume"`
	}
	if err := json.Unmarshal([]byte(run.Command), &command); err != nil ||
		command.Resume == nil || command.Resume.Journal == nil {
		return nil, fmt.Errorf("eino adk journal recovery command is invalid")
	}
	journal := command.Resume.Journal
	journal.SourceAttemptID = strings.TrimSpace(journal.SourceAttemptID)
	journal.Action = strings.TrimSpace(journal.Action)
	if journal.JournalRunID != marker.JournalRunID ||
		journal.SourceAttemptID != marker.SourceAttemptID || journal.Action != marker.Action {
		return nil, fmt.Errorf("eino adk journal recovery command does not match metadata")
	}
	state := &adkSideEffectRecoveryState{
		journalRunID: marker.JournalRunID, sourceAttemptID: marker.SourceAttemptID,
		directives: make(map[string]adkSideEffectRecoveryDirective, len(journal.Resolutions)),
	}
	for _, directive := range journal.Resolutions {
		directive.SourceIdempotencyKey = strings.TrimSpace(directive.SourceIdempotencyKey)
		directive.ActionKind = strings.TrimSpace(directive.ActionKind)
		directive.RequestHash = strings.ToLower(strings.TrimSpace(directive.RequestHash))
		directive.Action = strings.TrimSpace(directive.Action)
		directive.ResolutionIdempotencyKey = strings.TrimSpace(directive.ResolutionIdempotencyKey)
		action := domainentity.SideEffectResolutionAction(directive.Action)
		if directive.LedgerID <= 0 || directive.SourceIdempotencyKey == "" ||
			directive.ActionKind == "" || !validADKSideEffectDigest(directive.RequestHash) ||
			!action.Valid() || directive.ResolutionIdempotencyKey == "" ||
			directive.Action != marker.Action {
			return nil, fmt.Errorf("eino adk journal recovery resolution is invalid")
		}
		if _, exists := state.directives[directive.SourceIdempotencyKey]; exists {
			return nil, fmt.Errorf("eino adk journal recovery resolution is duplicated")
		}
		state.directives[directive.SourceIdempotencyKey] = directive
	}
	if marker.Action != string(JournalRecoveryActionResume) && len(state.directives) == 0 {
		return nil, fmt.Errorf("eino adk journal recovery resolution is required")
	}
	return state, nil
}

func hasADKSideEffectRecoveryCommand(command string) bool {
	var payload struct {
		Resume *struct {
			Journal json.RawMessage `json:"journal"`
		} `json:"resume"`
	}
	if json.Unmarshal([]byte(command), &payload) != nil || payload.Resume == nil {
		return false
	}
	return len(payload.Resume.Journal) > 0 && string(payload.Resume.Journal) != "null"
}

func validADKSideEffectDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (c *ADKSideEffectBoundaryCoordinator) begin(
	ctx context.Context,
	toolName string,
	callID string,
	arguments string,
	policy domainentity.SideEffectReplayPolicy,
	registered bool,
) (*adkSideEffectInvocation, bool, error) {
	attempt, err := c.repo.GetActiveJournalAttempt(ctx, c.run.RunID)
	if errors.Is(err, domainrepo.ErrJournalNotEnrolled) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if attempt == nil || attempt.ThreadID != c.run.ThreadID ||
		attempt.ExecutionRunID != c.run.RunID || !attempt.Status.IsActive() {
		return nil, true, fmt.Errorf("eino adk side effect attempt does not belong to run")
	}
	toolName = strings.TrimSpace(toolName)
	callID = strings.TrimSpace(callID)
	if toolName == "" || callID == "" || !policy.Valid() {
		return nil, true, fmt.Errorf("eino adk side effect call identity is required")
	}
	recoveryAction, err := c.recoveryAction(ctx, attempt, toolName, callID, arguments)
	if err != nil {
		return nil, true, err
	}
	ledgerID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, true, err
	}
	auditID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, true, err
	}
	now := c.now()
	key := adkSideEffectIdempotencyKey(toolName, callID)
	classification := "unregistered"
	if registered {
		classification = "registered"
	}
	requestSummary, err := json.Marshal(map[string]any{
		"argument_bytes": len(arguments),
		"classification": classification,
	})
	if err != nil {
		return nil, true, err
	}
	ledger := &domainentity.SideEffectLedger{
		ID: ledgerID, ThreadID: attempt.ThreadID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
		IdempotencyKey: key, ActionKind: toolName, ReplayPolicy: policy,
		Status:         domainentity.SideEffectLedgerStatusPrepared,
		RequestHash:    adkSideEffectRequestHash(toolName, arguments),
		RequestSummary: string(requestSummary), Version: 1,
		PreparedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	prepared, _, err := c.repo.PrepareSideEffect(ctx, domainrepo.PrepareSideEffectRequest{
		Ledger: ledger,
		AuditEvent: c.sideEffectAuditEvent(
			auditID, attempt, ledger, "prepared", key+":prepared",
		),
	})
	if err != nil {
		return nil, true, err
	}
	if prepared == nil {
		return nil, true, fmt.Errorf("eino adk side effect prepare returned empty ledger")
	}
	if prepared.Status != domainentity.SideEffectLedgerStatusPrepared {
		return nil, true, fmt.Errorf(
			"%w: tool %s call %s is already %s",
			ErrADKSideEffectReplayBlocked, toolName, callID, prepared.Status,
		)
	}
	executingAuditID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, true, err
	}
	executing, _, err := c.repo.TransitionSideEffect(
		ctx,
		domainrepo.TransitionSideEffectRequest{
			JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
			LedgerID: prepared.ID, ExpectedVersion: prepared.Version,
			FromStatus: domainentity.SideEffectLedgerStatusPrepared,
			ToStatus:   domainentity.SideEffectLedgerStatusExecuting,
			OccurredAt: c.now(), AuditEvent: c.sideEffectAuditEvent(
				executingAuditID, attempt, prepared, "executing", key+":executing",
			),
		},
	)
	if err != nil {
		return nil, true, err
	}
	if executing == nil || executing.Status != domainentity.SideEffectLedgerStatusExecuting {
		return nil, true, fmt.Errorf("eino adk side effect did not enter executing state")
	}
	return &adkSideEffectInvocation{
		attempt: attempt, ledger: executing, recoveryAction: recoveryAction,
	}, true, nil
}

func (c *ADKSideEffectBoundaryCoordinator) recoveryAction(
	ctx context.Context,
	attempt *domainentity.RunAttempt,
	toolName string,
	callID string,
	arguments string,
) (domainentity.SideEffectResolutionAction, error) {
	if c.recovery == nil {
		return "", nil
	}
	if attempt == nil || attempt.JournalRunID != c.recovery.journalRunID ||
		attempt.SourceAttemptID == nil ||
		strings.TrimSpace(*attempt.SourceAttemptID) != c.recovery.sourceAttemptID {
		return "", fmt.Errorf(
			"%w: recovery attempt does not match its source",
			ErrADKSideEffectReplayBlocked,
		)
	}
	key := adkSideEffectIdempotencyKey(toolName, callID)
	requestHash := adkSideEffectRequestHash(toolName, arguments)
	directive, found := c.recovery.directives[key]
	if !found {
		for _, candidate := range c.recovery.directives {
			if candidate.ActionKind == toolName && candidate.RequestHash == requestHash {
				return "", fmt.Errorf(
					"%w: recovery tool call identity changed",
					ErrADKSideEffectReplayBlocked,
				)
			}
		}
		return "", nil
	}
	if directive.ActionKind != toolName || directive.RequestHash != requestHash {
		return "", fmt.Errorf(
			"%w: recovery tool request changed",
			ErrADKSideEffectReplayBlocked,
		)
	}
	source, err := c.repo.GetSideEffectLedger(
		ctx, c.recovery.journalRunID, c.recovery.sourceAttemptID,
		directive.SourceIdempotencyKey,
	)
	if err != nil {
		return "", fmt.Errorf("%w: source side effect ledger is unavailable", ErrADKSideEffectReplayBlocked)
	}
	action := domainentity.SideEffectResolutionAction(directive.Action)
	if source == nil || source.ID != directive.LedgerID ||
		source.JournalRunID != c.recovery.journalRunID ||
		source.AttemptID != c.recovery.sourceAttemptID ||
		source.IdempotencyKey != directive.SourceIdempotencyKey ||
		source.ActionKind != directive.ActionKind || source.RequestHash != directive.RequestHash ||
		source.Status != domainentity.SideEffectLedgerStatusUnknown ||
		source.ResolutionAction != action ||
		source.ResolutionIdempotencyKey != directive.ResolutionIdempotencyKey {
		return "", fmt.Errorf(
			"%w: source side effect resolution is not verified",
			ErrADKSideEffectReplayBlocked,
		)
	}
	return action, nil
}

func (c *ADKSideEffectBoundaryCoordinator) complete(
	ctx context.Context,
	invocation *adkSideEffectInvocation,
	resultDigest string,
	callErr error,
) error {
	if invocation == nil || invocation.ledger == nil || invocation.attempt == nil {
		return fmt.Errorf("eino adk side effect invocation is required")
	}
	if callErr != nil {
		if errors.Is(callErr, context.Canceled) || errors.Is(callErr, context.DeadlineExceeded) {
			return c.markUnknown(ctx, invocation)
		}
		if latest, ok := c.latestCheckpoint(); ok {
			pending, err := c.newPending(
				ctx, invocation, domainentity.SideEffectLedgerStatusFailed, resultDigest,
			)
			if err != nil {
				return err
			}
			c.enqueue(pending)
			_, _, err = c.CommitCheckpoint(ctx, latest)
			return err
		}
		return c.markUnknown(ctx, invocation)
	}
	pending, err := c.newPending(
		ctx, invocation, domainentity.SideEffectLedgerStatusSucceeded, resultDigest,
	)
	if err != nil {
		return err
	}
	c.enqueue(pending)
	return nil
}

func (c *ADKSideEffectBoundaryCoordinator) markUnknown(
	ctx context.Context,
	invocation *adkSideEffectInvocation,
) error {
	auditID, err := c.idGen.GenID(ctx)
	if err != nil {
		return err
	}
	ledger := invocation.ledger
	updated, _, err := c.repo.TransitionSideEffect(
		ctx,
		domainrepo.TransitionSideEffectRequest{
			JournalRunID: ledger.JournalRunID, AttemptID: ledger.AttemptID,
			LedgerID: ledger.ID, ExpectedVersion: ledger.Version,
			FromStatus: domainentity.SideEffectLedgerStatusExecuting,
			ToStatus:   domainentity.SideEffectLedgerStatusUnknown,
			OccurredAt: c.now(), AuditEvent: c.sideEffectAuditEvent(
				auditID, invocation.attempt, ledger, "unknown",
				ledger.IdempotencyKey+":unknown",
			),
		},
	)
	if err == nil && updated != nil {
		invocation.ledger = updated
	}
	return err
}

func (c *ADKSideEffectBoundaryCoordinator) newPending(
	ctx context.Context,
	invocation *adkSideEffectInvocation,
	status domainentity.SideEffectLedgerStatus,
	resultDigest string,
) (*adkPendingSideEffect, error) {
	resultEventID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	auditEventID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	checkpointID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	ledger := invocation.ledger
	wireStatus := "completed"
	if status == domainentity.SideEffectLedgerStatusFailed {
		wireStatus = "failed"
	}
	payload, err := json.Marshal(map[string]any{
		"type": "terminal",
		"data": map[string]any{
			"action_id": ledger.IdempotencyKey,
			"operation": ledger.ActionKind,
			"status":    wireStatus,
			"target":    ledger.ActionKind,
		},
	})
	if err != nil {
		return nil, err
	}
	now := c.now()
	traceID := ""
	if invocation.attempt.TraceID != nil {
		traceID = strings.TrimSpace(*invocation.attempt.TraceID)
	}
	resultEvent := &domainentity.JournalEvent{
		ID: resultEventID, ThreadID: invocation.attempt.ThreadID,
		RunID:          invocation.attempt.ExecutionRunID,
		JournalRunID:   invocation.attempt.JournalRunID,
		AttemptID:      invocation.attempt.AttemptID,
		IdempotencyKey: ledger.IdempotencyKey + ":terminal",
		SchemaVersion:  domainentity.JournalSchemaVersion,
		Status:         wireStatus, OccurredAtUnixNano: now * int64(time.Millisecond),
		Visibility:     domainentity.JournalVisibilityUser,
		PayloadVersion: domainentity.JournalPayloadVersion,
		TraceID:        traceID, ActionID: ledger.IdempotencyKey,
		Phase: "terminal", Operation: ledger.ActionKind, Target: ledger.ActionKind,
		EventType: "action.terminal", Payload: string(payload), CreatedAt: now,
	}
	return &adkPendingSideEffect{
		invocation: invocation, status: status,
		externalReferenceDigest: strings.ToLower(strings.TrimSpace(resultDigest)),
		resultEvent:             resultEvent,
		auditEvent: c.sideEffectAuditEvent(
			auditEventID, invocation.attempt, ledger, string(status),
			ledger.IdempotencyKey+":"+string(status),
		),
		checkpointID: checkpointID,
	}, nil
}

func (c *ADKSideEffectBoundaryCoordinator) enqueue(pending *adkPendingSideEffect) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.pending {
		if existing != nil && pending != nil &&
			existing.invocation.ledger.ID == pending.invocation.ledger.ID {
			return
		}
	}
	c.pending = append(c.pending, pending)
}

func (c *ADKSideEffectBoundaryCoordinator) CommitCheckpoint(
	ctx context.Context,
	input ADKSideEffectCheckpointInput,
) (*domainentity.Checkpoint, bool, error) {
	if strings.TrimSpace(input.RuntimeKey) == "" || len(input.RuntimeState) == 0 ||
		input.ParityState == nil || input.ParentCheckpointID < 0 || input.RunRevision < 0 {
		return nil, false, fmt.Errorf("eino adk side effect checkpoint input is incomplete")
	}
	if err := validateADKParityStateSnapshot(input.ParityState); err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(input.RuntimeVersion) == "" {
		input.RuntimeVersion = adkCheckpointRuntimeVersion
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) == 0 {
		return nil, false, nil
	}
	parentID := input.ParentCheckpointID
	var last *domainentity.Checkpoint
	for len(c.pending) > 0 {
		pending := c.pending[0]
		if pending == nil || pending.invocation == nil || pending.invocation.ledger == nil {
			return nil, false, fmt.Errorf("eino adk pending side effect is incomplete")
		}
		result, err := c.repo.CommitExecutionBoundary(
			ctx,
			domainrepo.CommitExecutionBoundaryRequest{
				JournalRunID:            pending.invocation.ledger.JournalRunID,
				AttemptID:               pending.invocation.ledger.AttemptID,
				LedgerID:                pending.invocation.ledger.ID,
				ExpectedVersion:         pending.invocation.ledger.Version,
				Status:                  pending.status,
				ExternalReferenceDigest: pending.externalReferenceDigest,
				ResultEvent:             pending.resultEvent, AuditEvent: pending.auditEvent,
				CheckpointFactory: func(
					lastCommittedSequence uint64,
					ledgers []*domainentity.SideEffectLedger,
				) (*domainentity.Checkpoint, error) {
					return c.executionBoundaryCheckpoint(
						input, parentID, pending.checkpointID,
						pending.invocation.attempt, lastCommittedSequence, ledgers,
					)
				},
			},
		)
		if err != nil {
			return nil, false, err
		}
		if result == nil || result.Checkpoint == nil || result.Ledger == nil {
			return nil, false, fmt.Errorf("eino adk side effect boundary returned empty result")
		}
		pending.invocation.ledger = result.Ledger
		last = result.Checkpoint
		parentID = last.ID
		c.pending = c.pending[1:]
	}
	latest := input
	latest.ParentCheckpointID = parentID
	latest.RuntimeState = append([]byte(nil), input.RuntimeState...)
	parity := cloneADKParityState(*input.ParityState)
	latest.ParityState = &parity
	c.latest = &latest
	c.latestCheckpointID = parentID
	return last, true, nil
}

func (c *ADKSideEffectBoundaryCoordinator) RememberCheckpoint(
	input ADKSideEffectCheckpointInput,
	checkpointID int64,
) {
	if checkpointID <= 0 || input.ParityState == nil || len(input.RuntimeState) == 0 {
		return
	}
	copy := input
	copy.ParentCheckpointID = checkpointID
	copy.RuntimeState = append([]byte(nil), input.RuntimeState...)
	parity := cloneADKParityState(*input.ParityState)
	copy.ParityState = &parity
	c.mu.Lock()
	c.latest = &copy
	c.latestCheckpointID = checkpointID
	c.mu.Unlock()
}

func (c *ADKSideEffectBoundaryCoordinator) latestCheckpoint() (
	ADKSideEffectCheckpointInput,
	bool,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.latest == nil || c.latestCheckpointID <= 0 {
		return ADKSideEffectCheckpointInput{}, false
	}
	copy := *c.latest
	copy.ParentCheckpointID = c.latestCheckpointID
	copy.RuntimeState = append([]byte(nil), c.latest.RuntimeState...)
	parity := cloneADKParityState(*c.latest.ParityState)
	copy.ParityState = &parity
	return copy, true
}

func (c *ADKSideEffectBoundaryCoordinator) executionBoundaryCheckpoint(
	input ADKSideEffectCheckpointInput,
	parentCheckpointID int64,
	checkpointID int64,
	attempt *domainentity.RunAttempt,
	lastCommittedSequence uint64,
	ledgers []*domainentity.SideEffectLedger,
) (*domainentity.Checkpoint, error) {
	if attempt == nil || len(ledgers) == 0 {
		return nil, fmt.Errorf("eino adk side effect checkpoint ledger state is required")
	}
	parity := cloneADKParityState(*input.ParityState)
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkJournalCheckpointEnvelopeVersion,
		SchemaVersion:   adkJournalCheckpointSchemaVersion,
		Runtime:         string(RuntimeModeEinoADK), RuntimeVersion: input.RuntimeVersion,
		RuntimeKey: strings.TrimSpace(input.RuntimeKey), MessageType: adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		RuntimeState: &ADKCheckpointRuntimeState{
			Checkpoint: append([]byte(nil), input.RuntimeState...),
		},
		AttemptID:             attempt.AttemptID,
		LastCommittedSequence: lastCommittedSequence,
		SideEffectLedger:      adkSideEffectLedgerReferences(ledgers),
		ParityState:           &parity, RunRevision: input.RunRevision, CreatedAt: c.now(),
	}
	raw, err := envelope.Marshal()
	if err != nil {
		return nil, err
	}
	return &domainentity.Checkpoint{
		ID: checkpointID, ThreadID: attempt.ThreadID, RunID: attempt.ExecutionRunID,
		ParentCheckpointID: parentCheckpointID,
		CheckpointNS:       adkCheckpointNamespace, RuntimeType: string(RuntimeModeEinoADK),
		RuntimeKey: envelope.RuntimeKey, EnvelopeVersion: int32(envelope.EnvelopeVersion),
		ChannelValues: string(raw), ChannelVersions: `{}`, PendingSends: `[]`,
		Metadata: adkCheckpointMetadataJSONVersion(
			envelope.RuntimeVersion, envelope.RuntimeKey,
			ADKCheckpointPhaseRuntime, adkJournalCheckpointEnvelopeVersion,
		),
		CreatedAt: envelope.CreatedAt,
	}, nil
}

func (c *ADKSideEffectBoundaryCoordinator) sideEffectAuditEvent(
	id int64,
	attempt *domainentity.RunAttempt,
	ledger *domainentity.SideEffectLedger,
	status string,
	idempotencyKey string,
) *domainentity.JournalEvent {
	now := c.now()
	payload, _ := json.Marshal(map[string]any{
		"type": "side_effect",
		"data": map[string]any{
			"action_kind":   ledger.ActionKind,
			"replay_policy": ledger.ReplayPolicy,
			"status":        status,
		},
	})
	return &domainentity.JournalEvent{
		ID: id, ThreadID: attempt.ThreadID, RunID: attempt.ExecutionRunID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
		IdempotencyKey: idempotencyKey,
		SchemaVersion:  domainentity.JournalSchemaVersion,
		Status:         status, OccurredAtUnixNano: now * int64(time.Millisecond),
		Visibility:     domainentity.JournalVisibilityInternal,
		PayloadVersion: domainentity.JournalPayloadVersion,
		EventType:      "side_effect.audit", Payload: string(payload), CreatedAt: now,
	}
}

type ADKSideEffectMiddlewareOption func(*ADKSideEffectMiddleware)

func WithADKSideEffectNonReplayableTools(names []string) ADKSideEffectMiddlewareOption {
	return func(middleware *ADKSideEffectMiddleware) {
		for _, name := range names {
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" {
				middleware.overrides[name] = domainentity.SideEffectReplayPolicyNonReplayable
			}
		}
	}
}

type ADKSideEffectMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	coordinator *ADKSideEffectBoundaryCoordinator
	overrides   map[string]domainentity.SideEffectReplayPolicy
}

func NewADKSideEffectMiddleware(
	coordinator *ADKSideEffectBoundaryCoordinator,
	options ...ADKSideEffectMiddlewareOption,
) *ADKSideEffectMiddleware {
	middleware := &ADKSideEffectMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		coordinator:                  coordinator,
		overrides:                    make(map[string]domainentity.SideEffectReplayPolicy),
	}
	for _, option := range options {
		if option != nil {
			option(middleware)
		}
	}
	return middleware
}

func (m *ADKSideEffectMiddleware) classify(
	name string,
) (domainentity.SideEffectReplayPolicy, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if policy, ok := m.overrides[normalized]; ok {
		return policy, true
	}
	return ClassifyADKSideEffectTool(normalized)
}

func (m *ADKSideEffectMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	if endpoint == nil || tCtx == nil {
		return nil, fmt.Errorf("eino adk invokable side effect endpoint is required")
	}
	return func(ctx context.Context, arguments string, opts ...tool.Option) (string, error) {
		invocation, enrolled, err := m.begin(ctx, tCtx, arguments)
		if err != nil {
			return "", err
		}
		if result, bypass := adkSideEffectRecoveryResult(invocation); enrolled && bypass {
			completeErr := m.coordinator.complete(
				ctx, invocation, digestADKSideEffectBytes([]byte(result)), nil,
			)
			return result, completeErr
		}
		result, callErr := endpoint(ctx, arguments, opts...)
		if !enrolled {
			return result, callErr
		}
		completeErr := m.coordinator.complete(
			ctx, invocation, digestADKSideEffectBytes([]byte(result)), callErr,
		)
		return result, joinADKSideEffectErrors(callErr, completeErr)
	}, nil
}

func (m *ADKSideEffectMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	if endpoint == nil || tCtx == nil {
		return nil, fmt.Errorf("eino adk streamable side effect endpoint is required")
	}
	return func(ctx context.Context, arguments string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		invocation, enrolled, err := m.begin(ctx, tCtx, arguments)
		if err != nil {
			return nil, err
		}
		if result, bypass := adkSideEffectRecoveryResult(invocation); enrolled && bypass {
			if err := m.coordinator.complete(
				ctx, invocation, digestADKSideEffectBytes([]byte(result)), nil,
			); err != nil {
				return nil, err
			}
			return schema.StreamReaderFromArray([]string{result}), nil
		}
		stream, callErr := endpoint(ctx, arguments, opts...)
		if !enrolled || callErr != nil {
			if enrolled {
				callErr = joinADKSideEffectErrors(
					callErr, m.coordinator.complete(ctx, invocation, "", callErr),
				)
			}
			return stream, callErr
		}
		return monitorADKSideEffectStream(
			ctx, stream, invocation, m.coordinator,
			func(hasher hash.Hash, chunk string) { _, _ = hasher.Write([]byte(chunk)) },
		), nil
	}, nil
}

func (m *ADKSideEffectMiddleware) WrapEnhancedInvokableToolCall(
	_ context.Context,
	endpoint adk.EnhancedInvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.EnhancedInvokableToolCallEndpoint, error) {
	if endpoint == nil || tCtx == nil {
		return nil, fmt.Errorf("eino adk enhanced side effect endpoint is required")
	}
	return func(ctx context.Context, argument *schema.ToolArgument, opts ...tool.Option) (*schema.ToolResult, error) {
		arguments := ""
		if argument != nil {
			arguments = argument.Text
		}
		invocation, enrolled, err := m.begin(ctx, tCtx, arguments)
		if err != nil {
			return nil, err
		}
		if result, bypass := adkSideEffectRecoveryResult(invocation); enrolled && bypass {
			toolResult := adkSideEffectRecoveryToolResult(result)
			raw, _ := json.Marshal(toolResult)
			completeErr := m.coordinator.complete(
				ctx, invocation, digestADKSideEffectBytes(raw), nil,
			)
			return toolResult, completeErr
		}
		result, callErr := endpoint(ctx, argument, opts...)
		if !enrolled {
			return result, callErr
		}
		raw, _ := json.Marshal(result)
		completeErr := m.coordinator.complete(
			ctx, invocation, digestADKSideEffectBytes(raw), callErr,
		)
		return result, joinADKSideEffectErrors(callErr, completeErr)
	}, nil
}

func (m *ADKSideEffectMiddleware) WrapEnhancedStreamableToolCall(
	_ context.Context,
	endpoint adk.EnhancedStreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.EnhancedStreamableToolCallEndpoint, error) {
	if endpoint == nil || tCtx == nil {
		return nil, fmt.Errorf("eino adk enhanced stream side effect endpoint is required")
	}
	return func(ctx context.Context, argument *schema.ToolArgument, opts ...tool.Option) (*schema.StreamReader[*schema.ToolResult], error) {
		arguments := ""
		if argument != nil {
			arguments = argument.Text
		}
		invocation, enrolled, err := m.begin(ctx, tCtx, arguments)
		if err != nil {
			return nil, err
		}
		if result, bypass := adkSideEffectRecoveryResult(invocation); enrolled && bypass {
			toolResult := adkSideEffectRecoveryToolResult(result)
			raw, _ := json.Marshal(toolResult)
			if err := m.coordinator.complete(
				ctx, invocation, digestADKSideEffectBytes(raw), nil,
			); err != nil {
				return nil, err
			}
			return schema.StreamReaderFromArray([]*schema.ToolResult{toolResult}), nil
		}
		stream, callErr := endpoint(ctx, argument, opts...)
		if !enrolled || callErr != nil {
			if enrolled {
				callErr = joinADKSideEffectErrors(
					callErr, m.coordinator.complete(ctx, invocation, "", callErr),
				)
			}
			return stream, callErr
		}
		return monitorADKSideEffectStream(
			ctx, stream, invocation, m.coordinator,
			func(hasher hash.Hash, chunk *schema.ToolResult) {
				raw, _ := json.Marshal(chunk)
				_, _ = hasher.Write(raw)
			},
		), nil
	}, nil
}

func (m *ADKSideEffectMiddleware) begin(
	ctx context.Context,
	tCtx *adk.ToolContext,
	arguments string,
) (*adkSideEffectInvocation, bool, error) {
	if m == nil || m.coordinator == nil {
		return nil, true, fmt.Errorf("eino adk side effect coordinator is required")
	}
	policy, registered := m.classify(tCtx.Name)
	return m.coordinator.begin(
		ctx, tCtx.Name, tCtx.CallID, arguments, policy, registered,
	)
}

func adkSideEffectRecoveryResult(
	invocation *adkSideEffectInvocation,
) (string, bool) {
	if invocation == nil {
		return "", false
	}
	switch invocation.recoveryAction {
	case domainentity.SideEffectResolutionActionSkip:
		return `{"status":"skipped","external_call_executed":false}`, true
	case domainentity.SideEffectResolutionActionMarkSucceeded:
		return `{"status":"confirmed_succeeded","external_call_executed":false}`, true
	default:
		return "", false
	}
}

func adkSideEffectRecoveryToolResult(result string) *schema.ToolResult {
	return &schema.ToolResult{Parts: []schema.ToolOutputPart{{
		Type: schema.ToolPartTypeText,
		Text: result,
	}}}
}

func monitorADKSideEffectStream[T any](
	ctx context.Context,
	source *schema.StreamReader[T],
	invocation *adkSideEffectInvocation,
	coordinator *ADKSideEffectBoundaryCoordinator,
	writeDigest func(hash.Hash, T),
) *schema.StreamReader[T] {
	reader, writer := schema.Pipe[T](1)
	go func() {
		defer writer.Close()
		if source == nil {
			var zero T
			err := fmt.Errorf("eino adk tool returned an empty stream")
			completeErr := coordinator.complete(ctx, invocation, "", err)
			writer.Send(zero, joinADKSideEffectErrors(err, completeErr))
			return
		}
		defer source.Close()
		hasher := sha256.New()
		for {
			chunk, err := source.Recv()
			if errors.Is(err, io.EOF) {
				completeErr := coordinator.complete(
					ctx, invocation, hex.EncodeToString(hasher.Sum(nil)), nil,
				)
				if completeErr != nil {
					var zero T
					writer.Send(zero, completeErr)
				}
				return
			}
			if err != nil {
				completeErr := coordinator.complete(ctx, invocation, "", err)
				var zero T
				writer.Send(zero, joinADKSideEffectErrors(err, completeErr))
				return
			}
			writeDigest(hasher, chunk)
			writer.Send(chunk, nil)
		}
	}()
	return reader
}

func adkSideEffectIdempotencyKey(toolName string, callID string) string {
	raw := sha256.Sum256([]byte(strings.TrimSpace(toolName) + "\x00" + strings.TrimSpace(callID)))
	return "tool:" + hex.EncodeToString(raw[:])
}

func adkSideEffectRequestHash(toolName string, arguments string) string {
	raw := sha256.Sum256([]byte(strings.TrimSpace(toolName) + "\x00" + arguments))
	return hex.EncodeToString(raw[:])
}

func digestADKSideEffectBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func joinADKSideEffectErrors(callErr error, boundaryErr error) error {
	if callErr == nil {
		return boundaryErr
	}
	if boundaryErr == nil {
		return callErr
	}
	return errors.Join(callErr, boundaryErr)
}
