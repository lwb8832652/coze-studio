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

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func (r *threadRepository) createHumanResumeRunBundle(
	ctx context.Context,
	req CreateRunBundleRequest,
) (*CreateRunBundleResult, error) {
	normalized, err := normalizeHumanResumeRunBundleInput(req)
	if err != nil {
		return nil, err
	}

	var result *CreateRunBundleResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		thread, err := lockThreadForUpdate(tx, normalized.Run.ThreadID)
		if err != nil {
			return err
		}
		if normalized.Run.SpaceID != thread.SpaceID || normalized.Run.CreatorID != thread.CreatorID {
			return fmt.Errorf("run bundle ownership does not match thread")
		}
		if replayed, found, replayErr := findHumanResumeRolloverReplay(tx, normalized); replayErr != nil {
			return replayErr
		} else if found {
			result = replayed
			return nil
		}
		result, err = createHumanResumeRolloverLocked(tx, normalized)
		return err
	})
	if err != nil {
		// A concurrent winner may commit after this transaction begins. Reuse the
		// same full aggregate validator outside the failed transaction; a Run row
		// by itself is never accepted as a successful replay.
		if replayed, found, replayErr := findHumanResumeRolloverReplay(
			r.db.WithContext(ctx), normalized,
		); replayErr != nil {
			if errors.Is(replayErr, gorm.ErrRecordNotFound) {
				return nil, err
			}
			return nil, replayErr
		} else if found {
			return replayed, nil
		}
	}
	return result, err
}

func (r *threadRepository) GetHumanResumeRolloverReplay(
	ctx context.Context,
	req HumanResumeRolloverReplayRequest,
) (*HumanResumeRolloverReplayResult, error) {
	result, found, err := findHumanResumeRolloverReplayByAuthority(r.db.WithContext(ctx), req)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &HumanResumeRolloverReplayResult{
		Run: result.Run, Message: result.Message, Event: result.Event,
		Attempt: result.Attempt, SourceAttempt: result.SourceAttempt,
		TerminalEvent: result.TerminalEvent, Replayed: true,
	}, nil
}

func normalizeHumanResumeRunBundleInput(req CreateRunBundleRequest) (CreateRunBundleRequest, error) {
	if err := validateHumanResumeRunBundleInput(req); err != nil {
		return CreateRunBundleRequest{}, err
	}
	now := time.Now().UnixMilli()
	normalized := req
	run := *req.Run
	if run.CreatedAt <= 0 {
		run.CreatedAt = now
	}
	if run.UpdatedAt <= 0 {
		run.UpdatedAt = run.CreatedAt
	}
	normalized.Run = &run
	message := *req.Message
	if message.CreatedAt <= 0 {
		message.CreatedAt = run.CreatedAt
	}
	normalized.Message = &message
	base := *req.Event
	if base.CreatedAt <= 0 {
		base.CreatedAt = run.CreatedAt
	}
	normalized.Event = &base
	journal := *req.EventJournal
	journal.ID, journal.ThreadID, journal.RunID = base.ID, base.ThreadID, base.RunID
	if journal.CreatedAt <= 0 {
		journal.CreatedAt = base.CreatedAt
	}
	normalized.EventJournal = &journal
	attempt := *req.Attempt
	attempt.NextSequence = 1
	attempt.LastCommittedSequence = 0
	attempt.CreatedAt, attempt.UpdatedAt = run.CreatedAt, run.CreatedAt
	normalized.Attempt = &attempt
	rollover := *req.HumanResumeRollover
	terminalBase := *rollover.TerminalBase
	terminalJournal := *rollover.TerminalJournal
	rollover.TerminalBase, rollover.TerminalJournal = &terminalBase, &terminalJournal
	normalized.HumanResumeRollover = &rollover
	return normalized, nil
}

func validateHumanResumeRunBundleInput(req CreateRunBundleRequest) error {
	rollover := req.HumanResumeRollover
	if req.Run == nil || req.Message == nil || req.Event == nil || req.EventJournal == nil ||
		req.Attempt == nil || rollover == nil || rollover.TerminalBase == nil || rollover.TerminalJournal == nil {
		return fmt.Errorf("human resume run bundle aggregate is required")
	}
	if req.RecoverySourceLease != nil || req.EventJournalProjectionFailed {
		return fmt.Errorf("human resume rollover requires strict journal projection")
	}
	if req.Run.Status != entity.RunStatusQueued || !isTopLevelTaskRun(req.Run) ||
		strings.TrimSpace(req.Run.MultitaskStrategy) != "reject" {
		return fmt.Errorf("human resume target must be a queued root task with reject admission")
	}
	if req.Message.ThreadID != req.Run.ThreadID || req.Message.RunID != req.Run.ID ||
		req.Message.Role != entity.MessageRoleUser || req.Event.ThreadID != req.Run.ThreadID ||
		req.Event.RunID != req.Run.ID || req.Event.EventType != "human.interaction.resolved" {
		return fmt.Errorf("human resume message and resolved event must belong to target run")
	}
	if req.EventJournalSourceRunID != rollover.SourceRunID ||
		req.EventJournal.ID != req.Event.ID || req.EventJournal.ThreadID != req.Event.ThreadID ||
		req.EventJournal.RunID != req.Event.RunID || req.EventJournal.EventType != "confirmation.resolved" ||
		req.EventJournal.Status != "completed" || req.EventJournal.Visibility != entity.JournalVisibilityUser {
		return fmt.Errorf("human resume resolved projection is invalid")
	}
	if req.Attempt.ID <= 0 || strings.TrimSpace(req.Attempt.AttemptID) == "" ||
		req.Attempt.ThreadID != req.Run.ThreadID || req.Attempt.ExecutionRunID != req.Run.ID ||
		req.Attempt.Status != entity.RunAttemptStatusPending || req.Attempt.JournalRunID <= 0 ||
		req.Attempt.SourceCheckpointID == nil || req.Attempt.SourceAttemptID == nil ||
		req.Attempt.RecoveryIdempotencyKey == nil ||
		strings.TrimSpace(req.Run.IdempotencyKey) == "" ||
		strings.TrimSpace(*req.Attempt.RecoveryIdempotencyKey) != strings.TrimSpace(req.Run.IdempotencyKey) {
		return fmt.Errorf("human resume target attempt lineage is invalid")
	}
	if req.EventJournal.JournalRunID != 0 || strings.TrimSpace(req.EventJournal.AttemptID) != "" ||
		strings.TrimSpace(req.EventJournal.SchemaVersion) != entity.JournalSchemaVersion ||
		strings.TrimSpace(req.EventJournal.PayloadVersion) != entity.JournalPayloadVersion ||
		strings.TrimSpace(req.EventJournal.IdempotencyKey) == "" {
		return fmt.Errorf("human resume resolved projection authority is invalid")
	}
	terminal := rollover.TerminalBase
	terminalJournal := rollover.TerminalJournal
	if rollover.SourceRunID <= 0 || terminal.ID != terminalJournal.ID ||
		terminal.ThreadID != req.Run.ThreadID || terminal.RunID != rollover.SourceRunID ||
		terminal.EventType != entity.JournalAttemptInterruptedRunEventType ||
		terminalJournal.ThreadID != terminal.ThreadID || terminalJournal.RunID != terminal.RunID ||
		terminalJournal.ParentEventID != req.Event.ID ||
		terminalJournal.JournalRunID != 0 || strings.TrimSpace(terminalJournal.AttemptID) != "" ||
		terminalJournal.IdempotencyKey != fmt.Sprintf("journal:run:%d:terminal:interrupted", rollover.SourceRunID) {
		return fmt.Errorf("human resume terminal identity is invalid")
	}
	var terminalPayload struct {
		Schema      string `json:"schema"`
		Status      string `json:"status"`
		ResumeRunID int64  `json:"resume_run_id"`
	}
	if err := json.Unmarshal([]byte(terminal.Payload), &terminalPayload); err != nil ||
		terminalPayload.Schema != "coze.journal_attempt_interrupted.v1" ||
		terminalPayload.Status != string(entity.RunAttemptStatusInterrupted) ||
		terminalPayload.ResumeRunID != req.Run.ID {
		return fmt.Errorf("human resume terminal payload is invalid")
	}
	expectedTerminalPayload := fmt.Sprintf(
		`{"schema":"coze.journal_attempt_interrupted.v1","status":"interrupted","resume_run_id":%d}`,
		req.Run.ID,
	)
	if equal, err := adaptiveExecutionJSONEqual(terminal.Payload, expectedTerminalPayload); err != nil || !equal {
		return fmt.Errorf("human resume terminal payload is invalid")
	}
	if err := validateTerminalJournalEvent(terminalJournal, entity.RunAttemptStatusInterrupted); err != nil {
		return err
	}
	if equal, err := adaptiveExecutionJSONEqual(
		terminalJournal.Payload,
		`{"type":"terminal","data":{"status":"interrupted"}}`,
	); err != nil || !equal {
		return fmt.Errorf("human resume terminal journal payload is invalid")
	}
	if _, err := humanResumeAuthorityFromBundle(req); err != nil {
		return err
	}
	return nil
}

func findHumanResumeRolloverReplay(
	db *gorm.DB,
	req CreateRunBundleRequest,
) (*CreateRunBundleResult, bool, error) {
	authority, err := humanResumeAuthorityFromBundle(req)
	if err != nil {
		return nil, false, err
	}
	aggregate, found, err := findHumanResumeRolloverReplayByAuthority(db, authority)
	if err != nil || !found {
		return nil, found, err
	}
	target := aggregate.Run
	conflict := func(reason string) (*CreateRunBundleResult, bool, error) {
		return nil, false, fmt.Errorf("%w: %s", ErrRunIdempotencyConflict, reason)
	}
	if target.ParentRunID != req.Run.ParentRunID || target.CreatorID != req.Run.CreatorID ||
		target.AssistantID != req.Run.AssistantID ||
		entity.DefaultRunKind(target.RunKind, target.ParentRunID) !=
			entity.DefaultRunKind(req.Run.RunKind, req.Run.ParentRunID) {
		return conflict("target run identity drift")
	}
	for _, comparison := range []struct {
		name   string
		stored string
		wanted string
	}{
		{name: "input", stored: target.Input, wanted: req.Run.Input},
		{name: "config", stored: target.Config, wanted: req.Run.Config},
		{name: "context", stored: target.Context, wanted: req.Run.Context},
		{name: "stream mode", stored: target.StreamMode, wanted: req.Run.StreamMode},
	} {
		equal, compareErr := adaptiveExecutionJSONEqual(comparison.stored, comparison.wanted)
		if compareErr != nil || !equal {
			return conflict("target run " + comparison.name + " drift")
		}
	}
	if target.MultitaskStrategy != req.Run.MultitaskStrategy || target.OnDisconnect != req.Run.OnDisconnect ||
		target.Durability != req.Run.Durability {
		return conflict("target run options drift")
	}
	if aggregate.Attempt.JournalRunID != req.Attempt.JournalRunID ||
		!equalInt64Pointers(aggregate.Attempt.SourceCheckpointID, req.Attempt.SourceCheckpointID) ||
		!equalStringPointers(aggregate.Attempt.SourceAttemptID, req.Attempt.SourceAttemptID) {
		return conflict("target attempt lineage drift")
	}
	if req.Attempt.EnrollmentVersion != "" && aggregate.Attempt.EnrollmentVersion != req.Attempt.EnrollmentVersion {
		return conflict("target attempt enrollment drift")
	}
	return &CreateRunBundleResult{
		Run: aggregate.Run, Message: aggregate.Message, Event: aggregate.Event,
		Attempt: aggregate.Attempt, Created: false,
	}, true, nil
}

func findHumanResumeRolloverReplayByAuthority(
	db *gorm.DB,
	req HumanResumeRolloverReplayRequest,
) (*HumanResumeRolloverReplayResult, bool, error) {
	if err := validateHumanResumeStableAuthority(req); err != nil {
		return nil, false, err
	}
	conflict := func(reason string) (*HumanResumeRolloverReplayResult, bool, error) {
		return nil, false, fmt.Errorf("%w: %s", ErrRunIdempotencyConflict, reason)
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	var target runPO
	if err := db.Where("space_id = ? AND idempotency_key = ?", req.SpaceID, key).First(&target).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	if target.ThreadID != req.ThreadID || target.SpaceID != req.SpaceID || target.ParentRunID != 0 ||
		entity.DefaultRunKind(entity.RunKind(target.RunKind), target.ParentRunID) != entity.RunKindTask {
		return conflict("target run identity drift")
	}
	if err := entity.ValidateRunIdempotencyValues(
		jsonToString(target.Metadata), req.IdempotencyOperation, req.IdempotencyFingerprint,
	); err != nil {
		return nil, false, err
	}

	var targetAttempt runAttemptPO
	if err := db.Where("execution_run_id = ?", target.ID).First(&targetAttempt).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		var sourceCount int64
		if countErr := db.Model(&runAttemptPO{}).Where("execution_run_id = ?", req.SourceRunID).
			Count(&sourceCount).Error; countErr != nil {
			return nil, false, countErr
		}
		if sourceCount == 0 {
			return nil, false, ErrJournalNotEnrolled
		}
		return conflict("target attempt is missing")
	}
	if targetAttempt.ID <= 0 || strings.TrimSpace(targetAttempt.AttemptID) == "" ||
		targetAttempt.ThreadID != target.ThreadID || targetAttempt.ExecutionRunID != target.ID ||
		targetAttempt.JournalRunID <= 0 || targetAttempt.SourceCheckpointID == nil ||
		targetAttempt.SourceAttemptID == nil || targetAttempt.RecoveryIdempotencyKey == nil ||
		strings.TrimSpace(*targetAttempt.RecoveryIdempotencyKey) != key {
		return conflict("target attempt lineage drift")
	}

	var messages []messagePO
	if err := db.Where("thread_id = ? AND run_id = ? AND role = ?", target.ThreadID, target.ID,
		string(entity.MessageRoleUser)).Order("id ASC").Find(&messages).Error; err != nil {
		return nil, false, err
	}
	if len(messages) != 1 {
		return conflict("target user message is missing or duplicated")
	}
	message := &messages[0]
	var resolvedRows []runEventPO
	if err := db.Where("thread_id = ? AND run_id = ? AND event_type = ?", target.ThreadID, target.ID,
		"human.interaction.resolved").Order("id ASC").Find(&resolvedRows).Error; err != nil {
		return nil, false, err
	}
	if len(resolvedRows) != 1 {
		return conflict("resolved event is missing or duplicated")
	}
	resolved := &resolvedRows[0]

	command, metadata, messageMetadata, resolvedPayload, response, err := decodeHumanResumeAggregateFacts(
		jsonToString(target.Command), jsonToString(target.Metadata), jsonToString(message.Metadata),
		jsonToString(resolved.Payload),
	)
	if err != nil {
		return conflict("persisted human resume facts are invalid")
	}
	if len(command.Resume.Targets) != 1 || command.Resume.ResumeFrom != "interrupt" ||
		metadata.CheckpointResume.ResumeFrom != "interrupt" ||
		!metadata.CheckpointResume.ProtectedFromWorkerClaim || command.Resume.CheckpointID <= 0 ||
		command.Resume.CheckpointID != metadata.CheckpointResume.CheckpointID ||
		command.Resume.CheckpointID != *targetAttempt.SourceCheckpointID ||
		strings.TrimSpace(command.Resume.CheckpointNS) == "" ||
		command.Resume.CheckpointNS != metadata.CheckpointResume.CheckpointNS ||
		metadata.CheckpointResume.SourceRunID != req.SourceRunID ||
		!humanResumeResponseMatches(response, req.Response) {
		return conflict("persisted human resume facts drift")
	}
	persistedInterrupt := ""
	for interrupt := range command.Resume.Targets {
		persistedInterrupt = strings.TrimSpace(interrupt)
	}
	if persistedInterrupt != strings.TrimSpace(req.InterruptID) ||
		message.Content != humanResumeMessageContent(req.Response) ||
		!humanResumeResolvedMatches(metadata.HumanInteraction, req, 0, req.ThreadID, response.SubmittedAt) ||
		!humanResumeResolvedMatches(messageMetadata.HumanInteraction, req, 0, 0, response.SubmittedAt) ||
		!humanResumeResolvedMatches(resolvedPayload, req, target.ID, req.ThreadID, response.SubmittedAt) {
		return conflict("persisted human resume correlation drift")
	}
	if (metadata.Message != nil) != req.PersistMessageReference ||
		(metadata.Message != nil && metadata.Message.MessageID != message.ID) {
		return conflict("target message reference drift")
	}

	var sourceAttempt runAttemptPO
	if err := db.Where("journal_run_id = ? AND attempt_id = ?", targetAttempt.JournalRunID,
		*targetAttempt.SourceAttemptID).First(&sourceAttempt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return conflict("source attempt is missing")
		}
		return nil, false, err
	}
	if sourceAttempt.ThreadID != req.ThreadID || sourceAttempt.ExecutionRunID != req.SourceRunID ||
		entity.RunAttemptStatus(sourceAttempt.Status) != entity.RunAttemptStatusInterrupted ||
		sourceAttempt.ActiveSlot != nil || sourceAttempt.EndedAt == nil || sourceAttempt.TerminalEventID == nil ||
		targetAttempt.Ordinal != sourceAttempt.Ordinal+1 ||
		targetAttempt.EnrollmentVersion != sourceAttempt.EnrollmentVersion ||
		targetAttempt.SnapshotsEnabled != sourceAttempt.SnapshotsEnabled ||
		!equalStringPointers(targetAttempt.TraceID, sourceAttempt.TraceID) {
		return conflict("source or target attempt inheritance drift")
	}

	var root, sourceRun runPO
	if err := db.Where("id = ?", targetAttempt.JournalRunID).First(&root).Error; err != nil {
		return conflict("journal root is missing")
	}
	if err := db.Where("id = ?", req.SourceRunID).First(&sourceRun).Error; err != nil {
		return conflict("source run is missing")
	}
	if root.ThreadID != req.ThreadID || root.SpaceID != req.SpaceID || root.CreatorID != target.CreatorID ||
		!isTopLevelTaskRun(root.toEntity()) || sourceRun.ThreadID != req.ThreadID ||
		sourceRun.SpaceID != req.SpaceID || sourceRun.CreatorID != target.CreatorID ||
		!isTopLevelTaskRun(sourceRun.toEntity()) {
		return conflict("source or root run identity drift")
	}

	if resolved.JournalRunID == nil || *resolved.JournalRunID != sourceAttempt.JournalRunID ||
		resolved.AttemptID == nil || *resolved.AttemptID != sourceAttempt.AttemptID ||
		resolved.Sequence == nil || *resolved.Sequence == 0 || resolved.IdempotencyKey == nil ||
		*resolved.IdempotencyKey != strings.TrimSpace(req.ResolvedJournalKey) ||
		resolved.JournalEventType == nil || *resolved.JournalEventType != "confirmation.resolved" ||
		stringFromPtr(resolved.Status) != "completed" ||
		stringFromPtr(resolved.Visibility) != string(entity.JournalVisibilityUser) ||
		stringFromPtr(resolved.SchemaVersion) != entity.JournalSchemaVersion ||
		stringFromPtr(resolved.PayloadVersion) != entity.JournalPayloadVersion {
		return conflict("resolved journal projection drift")
	}
	expectedResolvedJournal, err := humanResumeResolvedJournalPayload(req)
	if err != nil {
		return nil, false, err
	}
	if equal, compareErr := adaptiveExecutionJSONEqual(
		jsonToString(resolved.JournalPayload), expectedResolvedJournal,
	); compareErr != nil || !equal {
		return conflict("resolved journal payload drift")
	}

	var terminal runEventPO
	if err := db.Where("id = ?", *sourceAttempt.TerminalEventID).First(&terminal).Error; err != nil {
		return conflict("terminal event is missing")
	}
	if terminal.ThreadID != req.ThreadID || terminal.RunID != req.SourceRunID ||
		terminal.EventType != entity.JournalAttemptInterruptedRunEventType ||
		terminal.JournalRunID == nil || *terminal.JournalRunID != sourceAttempt.JournalRunID ||
		terminal.AttemptID == nil || *terminal.AttemptID != sourceAttempt.AttemptID ||
		terminal.Sequence == nil || *terminal.Sequence != *resolved.Sequence+1 ||
		terminal.IdempotencyKey == nil || *terminal.IdempotencyKey !=
		fmt.Sprintf("journal:run:%d:terminal:interrupted", req.SourceRunID) ||
		terminal.ParentEventID == nil || *terminal.ParentEventID != resolved.ID ||
		terminal.JournalEventType == nil || *terminal.JournalEventType != "run.lifecycle" ||
		stringFromPtr(terminal.Status) != string(entity.RunAttemptStatusInterrupted) ||
		stringFromPtr(terminal.Visibility) != string(entity.JournalVisibilityUser) ||
		stringFromPtr(terminal.SchemaVersion) != entity.JournalSchemaVersion ||
		stringFromPtr(terminal.PayloadVersion) != entity.JournalPayloadVersion ||
		sourceAttempt.LastCommittedSequence != *terminal.Sequence ||
		sourceAttempt.NextSequence != *terminal.Sequence+1 {
		return conflict("terminal event drift")
	}
	expectedTerminalPayload := fmt.Sprintf(
		`{"schema":"coze.journal_attempt_interrupted.v1","status":"interrupted","resume_run_id":%d}`,
		target.ID,
	)
	if equal, compareErr := adaptiveExecutionJSONEqual(
		jsonToString(terminal.Payload), expectedTerminalPayload,
	); compareErr != nil || !equal {
		return conflict("terminal base payload drift")
	}
	if equal, compareErr := adaptiveExecutionJSONEqual(
		jsonToString(terminal.JournalPayload), `{"type":"terminal","data":{"status":"interrupted"}}`,
	); compareErr != nil || !equal {
		return conflict("terminal journal payload drift")
	}

	return &HumanResumeRolloverReplayResult{
		Run: target.toEntity(), Message: message.toEntity(), Event: resolved.toEntity(),
		Attempt: targetAttempt.toEntity(), SourceAttempt: sourceAttempt.toEntity(),
		TerminalEvent: terminal.toEntity(), Replayed: true,
	}, true, nil
}

type humanResumeCommandEnvelope struct {
	Resume struct {
		CheckpointID int64                                  `json:"checkpoint_id"`
		CheckpointNS string                                 `json:"checkpoint_ns"`
		ResumeFrom   string                                 `json:"resume_from"`
		Targets      map[string]humanResumeResponseEnvelope `json:"targets"`
	} `json:"resume"`
}

type humanResumeResponseEnvelope struct {
	Schema        string `json:"schema"`
	InteractionID string `json:"interaction_id"`
	Kind          string `json:"kind"`
	Decision      string `json:"decision"`
	Answer        string `json:"answer,omitempty"`
	ChoiceID      string `json:"choice_id,omitempty"`
	Comment       string `json:"comment,omitempty"`
	SubmittedBy   string `json:"submitted_by,omitempty"`
	SubmittedAt   int64  `json:"submitted_at,omitempty"`
	Source        string `json:"source,omitempty"`
}

type humanResumeResolvedEnvelope struct {
	Schema        string `json:"schema"`
	ThreadID      int64  `json:"thread_id,omitempty"`
	SourceRunID   int64  `json:"source_run_id"`
	ResumeRunID   int64  `json:"resume_run_id,omitempty"`
	InterruptID   string `json:"interrupt_id"`
	InteractionID string `json:"interaction_id"`
	Kind          string `json:"kind"`
	Decision      string `json:"decision"`
	SubmittedAt   int64  `json:"submitted_at"`
	ChoiceID      string `json:"choice_id,omitempty"`
	SubmittedBy   string `json:"submitted_by,omitempty"`
	Source        string `json:"source,omitempty"`
}

type humanResumeMetadataEnvelope struct {
	CheckpointResume struct {
		ProtectedFromWorkerClaim bool   `json:"protected_from_worker_claim"`
		CheckpointID             int64  `json:"checkpoint_id"`
		CheckpointNS             string `json:"checkpoint_ns"`
		ResumeFrom               string `json:"resume_from"`
		SourceRunID              int64  `json:"source_run_id"`
	} `json:"checkpoint_resume"`
	HumanInteraction humanResumeResolvedEnvelope     `json:"human_interaction"`
	Idempotency      humanResumePersistedIdempotency `json:"_idempotency"`
	Message          *struct {
		MessageID int64 `json:"message_id"`
	} `json:"_message,omitempty"`
}

type humanResumeMessageMetadataEnvelope struct {
	HumanInteraction humanResumeResolvedEnvelope `json:"human_interaction"`
}

func humanResumeAuthorityFromBundle(req CreateRunBundleRequest) (HumanResumeRolloverReplayRequest, error) {
	if req.Run == nil || req.Message == nil || req.Event == nil || req.EventJournal == nil ||
		req.Attempt == nil || req.HumanResumeRollover == nil || req.Attempt.SourceCheckpointID == nil {
		return HumanResumeRolloverReplayRequest{}, fmt.Errorf("human resume aggregate consistency is invalid")
	}
	command, metadata, messageMetadata, resolved, response, err := decodeHumanResumeAggregateFacts(
		req.Run.Command, req.Run.Metadata, req.Message.Metadata, req.Event.Payload,
	)
	if err != nil {
		return HumanResumeRolloverReplayRequest{}, err
	}
	fail := func(reason string) (HumanResumeRolloverReplayRequest, error) {
		return HumanResumeRolloverReplayRequest{}, fmt.Errorf("human resume aggregate consistency is invalid: %s", reason)
	}
	if len(command.Resume.Targets) != 1 || command.Resume.ResumeFrom != "interrupt" ||
		metadata.CheckpointResume.ResumeFrom != "interrupt" ||
		!metadata.CheckpointResume.ProtectedFromWorkerClaim ||
		command.Resume.CheckpointID <= 0 || command.Resume.CheckpointID != metadata.CheckpointResume.CheckpointID ||
		command.Resume.CheckpointID != *req.Attempt.SourceCheckpointID ||
		strings.TrimSpace(command.Resume.CheckpointNS) == "" ||
		command.Resume.CheckpointNS != metadata.CheckpointResume.CheckpointNS ||
		metadata.CheckpointResume.SourceRunID != req.HumanResumeRollover.SourceRunID ||
		resolved.ResumeRunID != req.Run.ID {
		return fail("command, metadata, or lineage differs")
	}
	interruptID := ""
	for key := range command.Resume.Targets {
		interruptID = strings.TrimSpace(key)
	}
	authority := HumanResumeRolloverReplayRequest{
		SpaceID: req.Run.SpaceID, ThreadID: req.Run.ThreadID,
		SourceRunID:        req.HumanResumeRollover.SourceRunID,
		IdempotencyKey:     strings.TrimSpace(req.Run.IdempotencyKey),
		ResolvedJournalKey: strings.TrimSpace(req.EventJournal.IdempotencyKey),
		InterruptID:        interruptID, Response: humanResumeStableResponse(response),
		PersistMessageReference: metadata.Message != nil,
	}
	if contract, err := humanResumeIdempotencyContract(req.Run.Metadata); err != nil {
		return HumanResumeRolloverReplayRequest{}, err
	} else {
		authority.IdempotencyOperation = contract.Operation
		authority.IdempotencyFingerprint = contract.Fingerprint
	}
	if err := validateHumanResumeStableAuthority(authority); err != nil {
		return HumanResumeRolloverReplayRequest{}, err
	}
	expectedJournalPayload, err := humanResumeResolvedJournalPayload(authority)
	if err != nil {
		return HumanResumeRolloverReplayRequest{}, err
	}
	if equal, compareErr := adaptiveExecutionJSONEqual(req.EventJournal.Payload, expectedJournalPayload); compareErr != nil || !equal {
		return fail("resolved journal payload differs")
	}
	if metadata.Message != nil && metadata.Message.MessageID != req.Message.ID {
		return fail("message reference differs")
	}
	if req.Message.Content != humanResumeMessageContent(authority.Response) {
		return fail("message content differs")
	}
	if !humanResumeResolvedMatches(metadata.HumanInteraction, authority, 0, req.Run.ThreadID, response.SubmittedAt) {
		return fail("run metadata resolved facts differ")
	}
	if !humanResumeResolvedMatches(messageMetadata.HumanInteraction, authority, 0, 0, response.SubmittedAt) {
		return fail("message metadata resolved facts differ")
	}
	if !humanResumeResolvedMatches(resolved, authority, req.Run.ID, req.Run.ThreadID, response.SubmittedAt) {
		return fail("resolved event facts differ")
	}
	return authority, nil
}

func decodeHumanResumeAggregateFacts(
	commandRaw, metadataRaw, messageMetadataRaw, resolvedRaw string,
) (humanResumeCommandEnvelope, humanResumeMetadataEnvelope, humanResumeMessageMetadataEnvelope, humanResumeResolvedEnvelope, humanResumeResponseEnvelope, error) {
	var command humanResumeCommandEnvelope
	var metadata humanResumeMetadataEnvelope
	var messageMetadata humanResumeMessageMetadataEnvelope
	var resolved humanResumeResolvedEnvelope
	for _, item := range []struct {
		raw  string
		into any
	}{
		{commandRaw, &command}, {metadataRaw, &metadata},
		{messageMetadataRaw, &messageMetadata}, {resolvedRaw, &resolved},
	} {
		if err := decodeHumanResumeClosedJSON(item.raw, item.into); err != nil {
			return command, metadata, messageMetadata, resolved, humanResumeResponseEnvelope{},
				fmt.Errorf("human resume aggregate consistency is invalid: %w", err)
		}
	}
	if len(command.Resume.Targets) != 1 {
		return command, metadata, messageMetadata, resolved, humanResumeResponseEnvelope{},
			fmt.Errorf("human resume aggregate consistency is invalid")
	}
	for _, response := range command.Resume.Targets {
		return command, metadata, messageMetadata, resolved, response, nil
	}
	return command, metadata, messageMetadata, resolved, humanResumeResponseEnvelope{},
		fmt.Errorf("human resume aggregate consistency is invalid")
}

func decodeHumanResumeClosedJSON(raw string, into any) error {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

type humanResumePersistedIdempotency struct {
	Operation   string `json:"operation"`
	Fingerprint string `json:"fingerprint"`
}

func humanResumeIdempotencyContract(raw string) (humanResumePersistedIdempotency, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return humanResumePersistedIdempotency{}, fmt.Errorf("human resume idempotency metadata is invalid")
	}
	var contract humanResumePersistedIdempotency
	if err := json.Unmarshal(root["_idempotency"], &contract); err != nil {
		return humanResumePersistedIdempotency{}, fmt.Errorf("human resume idempotency metadata is invalid")
	}
	return contract, nil
}

func humanResumeStableResponse(response humanResumeResponseEnvelope) HumanResumeRolloverResponse {
	return HumanResumeRolloverResponse{
		Schema: strings.TrimSpace(response.Schema), InteractionID: strings.TrimSpace(response.InteractionID),
		Kind: strings.TrimSpace(response.Kind), Decision: strings.TrimSpace(response.Decision),
		Answer: strings.TrimSpace(response.Answer), ChoiceID: strings.TrimSpace(response.ChoiceID),
		Comment: strings.TrimSpace(response.Comment), SubmittedBy: strings.TrimSpace(response.SubmittedBy),
		Source: strings.TrimSpace(response.Source),
	}
}

func validateHumanResumeStableAuthority(req HumanResumeRolloverReplayRequest) error {
	response := req.Response
	if req.SpaceID <= 0 || req.ThreadID <= 0 || req.SourceRunID <= 0 ||
		strings.TrimSpace(req.IdempotencyKey) == "" || strings.TrimSpace(req.IdempotencyOperation) == "" ||
		strings.TrimSpace(req.IdempotencyFingerprint) == "" || strings.TrimSpace(req.ResolvedJournalKey) == "" ||
		strings.TrimSpace(req.InterruptID) == "" || strings.TrimSpace(response.Schema) != "coze.human_interaction_response.v1" ||
		strings.TrimSpace(response.InteractionID) == "" {
		return fmt.Errorf("human resume replay authority is invalid")
	}
	switch strings.TrimSpace(response.Kind) {
	case "clarification":
		if strings.TrimSpace(response.Decision) != "answered" ||
			(strings.TrimSpace(response.Answer) == "" && strings.TrimSpace(response.ChoiceID) == "") {
			return fmt.Errorf("human resume replay authority is invalid")
		}
	case "confirmation":
		if decision := strings.TrimSpace(response.Decision); decision != "approved" && decision != "rejected" {
			return fmt.Errorf("human resume replay authority is invalid")
		}
	default:
		return fmt.Errorf("human resume replay authority is invalid")
	}
	if strings.TrimSpace(req.ResolvedJournalKey) != humanResumeResolvedJournalKey(req.SourceRunID, req.InterruptID) {
		return fmt.Errorf("human resume replay authority is invalid")
	}
	return nil
}

func humanResumeResponseMatches(persisted humanResumeResponseEnvelope, wanted HumanResumeRolloverResponse) bool {
	return persisted.SubmittedAt > 0 && humanResumeStableResponse(persisted) == wanted
}

func humanResumeResolvedMatches(
	persisted humanResumeResolvedEnvelope,
	authority HumanResumeRolloverReplayRequest,
	resumeRunID, threadID, submittedAt int64,
) bool {
	response := authority.Response
	return persisted.Schema == "coze.human_interaction_resolved.v1" &&
		persisted.ThreadID == threadID && persisted.SourceRunID == authority.SourceRunID &&
		persisted.ResumeRunID == resumeRunID && persisted.InterruptID == authority.InterruptID &&
		persisted.InteractionID == response.InteractionID && persisted.Kind == response.Kind &&
		persisted.Decision == response.Decision && persisted.SubmittedAt == submittedAt &&
		persisted.ChoiceID == response.ChoiceID && persisted.SubmittedBy == response.SubmittedBy &&
		persisted.Source == response.Source
}

func humanResumeMessageContent(response HumanResumeRolloverResponse) string {
	if response.Kind == "clarification" {
		if answer := strings.TrimSpace(response.Answer); answer != "" {
			return answer
		}
		return strings.TrimSpace(response.ChoiceID)
	}
	if response.Decision == "approved" {
		return "已确认执行"
	}
	if comment := strings.TrimSpace(response.Comment); comment != "" {
		return "已拒绝执行：" + comment
	}
	return "已拒绝执行"
}

func humanResumeResolvedJournalPayload(req HumanResumeRolloverReplayRequest) (string, error) {
	raw, err := json.Marshal(map[string]any{
		"type": "confirmation",
		"data": map[string]any{
			"confirmation_id": req.InterruptID, "confirmation_type": req.Response.Kind,
			"allowed_action_keys": []string{},
		},
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func humanResumeResolvedJournalKey(sourceRunID int64, interruptID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"runtime-v1", fmt.Sprintf("%d", sourceRunID), "confirmation", strings.TrimSpace(interruptID), "resolved",
	}, "\x00")))
	return "journal:runtime-v1:" + hex.EncodeToString(sum[:])
}

func createHumanResumeRolloverLocked(
	tx *gorm.DB,
	req CreateRunBundleRequest,
) (*CreateRunBundleResult, error) {
	lock := func(query *gorm.DB) *gorm.DB {
		if tx.Dialector.Name() != "sqlite" {
			return query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		return query
	}
	var root runPO
	if err := lock(tx.Where("id = ?", req.Attempt.JournalRunID)).First(&root).Error; err != nil {
		return nil, err
	}
	var sourceRun runPO
	if root.ID == req.HumanResumeRollover.SourceRunID {
		sourceRun = root
	} else if err := lock(tx.Where("id = ?", req.HumanResumeRollover.SourceRunID)).First(&sourceRun).Error; err != nil {
		return nil, err
	}
	var sourceAttempt runAttemptPO
	if err := lock(tx.Where("journal_run_id = ? AND attempt_id = ?", root.ID,
		*req.Attempt.SourceAttemptID)).First(&sourceAttempt).Error; err != nil {
		return nil, err
	}
	var latestAttempt runAttemptPO
	if err := lock(tx.Where("journal_run_id = ?", root.ID).Order("ordinal DESC")).First(&latestAttempt).Error; err != nil {
		return nil, err
	}
	var checkpoint checkpointPO
	if err := lock(tx.Where("id = ?", *req.Attempt.SourceCheckpointID)).First(&checkpoint).Error; err != nil {
		return nil, err
	}
	if root.ThreadID != req.Run.ThreadID || root.SpaceID != req.Run.SpaceID || root.CreatorID != req.Run.CreatorID ||
		!isTopLevelTaskRun(root.toEntity()) ||
		sourceRun.ID != req.HumanResumeRollover.SourceRunID || sourceRun.ThreadID != req.Run.ThreadID ||
		sourceRun.SpaceID != req.Run.SpaceID || sourceRun.CreatorID != req.Run.CreatorID ||
		!isTopLevelTaskRun(sourceRun.toEntity()) ||
		entity.RunStatus(sourceRun.Status) != entity.RunStatusInterrupted {
		return nil, ErrHumanResumeRolloverConflict
	}
	if latestAttempt.ID != sourceAttempt.ID || sourceAttempt.ThreadID != req.Run.ThreadID ||
		sourceAttempt.ExecutionRunID != sourceRun.ID || sourceAttempt.AttemptID != *req.Attempt.SourceAttemptID ||
		entity.RunAttemptStatus(sourceAttempt.Status) != entity.RunAttemptStatusRunning ||
		sourceAttempt.ActiveSlot == nil || *sourceAttempt.ActiveSlot != 1 || sourceAttempt.StartedAt == nil ||
		sourceAttempt.EndedAt != nil || sourceAttempt.TerminalEventID != nil ||
		entity.JournalProjectionState(sourceAttempt.ProjectionState) != entity.JournalProjectionStateHealthy {
		return nil, ErrHumanResumeRolloverConflict
	}
	if checkpoint.ThreadID != req.Run.ThreadID || checkpoint.RunID != sourceRun.ID ||
		checkpoint.RuntimeDeletedAt != 0 || checkpoint.CheckpointNS != "eino.adk" ||
		checkpoint.RuntimeType != "eino_adk" || strings.TrimSpace(checkpoint.RuntimeKey) == "" ||
		checkpoint.EnvelopeVersion == 0 {
		return nil, fmt.Errorf("human resume source checkpoint is invalid")
	}
	activeRuns, err := lockActiveTopLevelRuns(tx, req.Run, false)
	if err != nil {
		return nil, err
	}
	if len(activeRuns) > 0 {
		return nil, ErrHumanResumeRolloverConflict
	}

	targetAttempt := *req.Attempt
	targetAttempt.Ordinal = sourceAttempt.Ordinal + 1
	targetAttempt.EnrollmentVersion = sourceAttempt.EnrollmentVersion
	targetAttempt.SnapshotsEnabled = sourceAttempt.SnapshotsEnabled
	targetAttempt.TraceID = cloneStringPointer(sourceAttempt.TraceID)
	targetAttempt.ProjectionState = entity.JournalProjectionStateHealthy
	activeSlot := uint8(1)
	targetAttempt.ActiveSlot = &activeSlot
	req.Attempt = &targetAttempt

	runPO, err := runToPO(req.Run)
	if err != nil {
		return nil, err
	}
	if err := tx.Create(runPO).Error; err != nil {
		return nil, err
	}
	messagePO, err := messageToPO(req.Message)
	if err != nil {
		return nil, err
	}
	if err := tx.Create(messagePO).Error; err != nil {
		return nil, err
	}
	resolved := *req.EventJournal
	resolved.JournalRunID, resolved.AttemptID = sourceAttempt.JournalRunID, sourceAttempt.AttemptID
	normalizedResolved, err := normalizeJournalEvent(&resolved)
	if err != nil {
		return nil, err
	}
	var existingResolved int64
	if err := tx.Model(&runEventPO{}).Where(
		"journal_run_id = ? AND attempt_id = ? AND idempotency_key = ?",
		sourceAttempt.JournalRunID, sourceAttempt.AttemptID, normalizedResolved.IdempotencyKey,
	).Count(&existingResolved).Error; err != nil {
		return nil, err
	}
	if existingResolved != 0 {
		return nil, fmt.Errorf("%w: resolved event already exists", ErrRunIdempotencyConflict)
	}
	_, err = appendJournalEventLockedWithBase(tx, &sourceAttempt, normalizedResolved, req.Event)
	if err != nil {
		return nil, err
	}
	terminal := *req.HumanResumeRollover.TerminalJournal
	terminal.JournalRunID, terminal.AttemptID = sourceAttempt.JournalRunID, sourceAttempt.AttemptID
	normalizedTerminal, err := normalizeJournalEvent(&terminal)
	if err != nil {
		return nil, err
	}
	resolvedParentID, err := resolvePublicJournalParent(tx, &sourceAttempt, normalizedTerminal.ParentEventID)
	if err != nil {
		return nil, err
	}
	if resolvedParentID != req.Event.ID {
		return nil, ErrJournalParentMismatch
	}
	normalizedTerminal.ParentEventID = resolvedParentID
	sequence := sourceAttempt.NextSequence
	terminalPO, err := journalEventToPOWithBase(normalizedTerminal, sequence, req.HumanResumeRollover.TerminalBase)
	if err != nil {
		return nil, err
	}
	endedAt := req.HumanResumeRollover.TerminalBase.CreatedAt
	if err := tx.Create(terminalPO).Error; err != nil {
		return nil, err
	}
	cas := tx.Model(&runAttemptPO{}).Where(
		"id = ? AND status = ? AND active_slot = ? AND next_sequence = ?",
		sourceAttempt.ID, entity.RunAttemptStatusRunning, 1, sequence,
	).Updates(map[string]any{
		"status": string(entity.RunAttemptStatusInterrupted), "active_slot": nil,
		"next_sequence": sequence + 1, "last_committed_sequence": sequence,
		"terminal_event_id": terminalPO.ID, "ended_at": endedAt, "updated_at": endedAt,
	})
	if cas.Error != nil {
		return nil, cas.Error
	}
	if cas.RowsAffected != 1 {
		return nil, ErrHumanResumeRolloverConflict
	}
	if err := tx.Create(runAttemptToPO(&targetAttempt)).Error; err != nil {
		return nil, err
	}
	return &CreateRunBundleResult{
		Run: req.Run, Message: req.Message, Event: req.Event, Attempt: &targetAttempt, Created: true,
		InterruptedRuns:   []*entity.Run{sourceRun.toEntity()},
		InterruptedEvents: []*entity.RunEvent{req.HumanResumeRollover.TerminalBase},
	}, nil
}
