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
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const (
	humanInteractionResolvedEventType = "human.interaction.resolved"
)

var (
	ErrHumanInteractionResumeInvalid  = errors.New("human interaction resume request is invalid")
	ErrHumanInteractionResumeConflict = errors.New("human interaction run is not resumable")
)

type humanInteractionResumeSemanticError struct {
	kind  error
	cause error
}

func (e *humanInteractionResumeSemanticError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *humanInteractionResumeSemanticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *humanInteractionResumeSemanticError) Is(target error) bool {
	return e != nil && (errors.Is(e.kind, target) || errors.Is(e.cause, target))
}

func newHumanInteractionResumeSemanticError(kind error, cause error) error {
	return &humanInteractionResumeSemanticError{kind: kind, cause: cause}
}

func (s *ApplicationService) ResumeHumanInteraction(
	ctx context.Context,
	req *ResumeHumanInteractionRequest,
) (*ResumeHumanInteractionResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("resume human interaction request is required")
	}
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.SourceRunID <= 0 {
		return nil, fmt.Errorf("source run id is required")
	}
	interruptID := strings.TrimSpace(req.InterruptID)
	if interruptID == "" {
		return nil, fmt.Errorf("interrupt id is required")
	}
	response := req.Response
	response.Schema = strings.TrimSpace(response.Schema)
	response.InteractionID = strings.TrimSpace(response.InteractionID)
	response.Answer = strings.TrimSpace(response.Answer)
	response.ChoiceID = strings.TrimSpace(response.ChoiceID)
	response.Comment = strings.TrimSpace(response.Comment)
	response.SubmittedBy = strings.TrimSpace(response.SubmittedBy)
	response.Source = strings.TrimSpace(response.Source)
	response.SubmittedAt = 0
	if err := validateHumanInteractionResponse(response); err != nil {
		return nil, newHumanInteractionResumeSemanticError(ErrHumanInteractionResumeInvalid, err)
	}

	sourceRun, err := s.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: req.SourceRunID})
	if err != nil {
		return nil, err
	}
	if sourceRun == nil {
		return nil, fmt.Errorf("source run is missing")
	}
	if sourceRun.ThreadID != req.ThreadID {
		return nil, fmt.Errorf("source run does not belong to thread")
	}
	if sourceRun.SpaceID <= 0 || sourceRun.CreatorID <= 0 || sourceRun.ParentRunID != 0 ||
		domainentity.DefaultRunKind(sourceRun.RunKind, sourceRun.ParentRunID) != domainentity.RunKindTask {
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeConflict,
			errors.New("source run must be a resumable top-level task"),
		)
	}
	idempotencyKey, err := humanInteractionResumeIdempotencyKey(req, response)
	if err != nil {
		return nil, err
	}

	journalSource := RunEvent{
		ThreadID: req.ThreadID, RunID: req.SourceRunID,
		EventType: humanInteractionResolvedEventType,
		Payload: encodeRunEventPayload(ctx, humanInteractionResolvedPayload(
			req.ThreadID, req.SourceRunID, 0, interruptID, response,
		)),
	}
	journalProjection, err := ProjectRunEventToJournal(journalSource)
	if err != nil || journalProjection == nil {
		if err == nil {
			err = errors.New("human interaction resolved journal projection is required")
		}
		return nil, err
	}
	legacyReplay := req.IdempotencyOperation == "" && req.IdempotencyFingerprint == ""
	if !legacyReplay {
		replayService, ok := s.ThreadSVC.(domainservice.HumanResumeRolloverReplayService)
		if !ok {
			return nil, errors.New("human resume rollover replay service is unavailable")
		}
		replay, replayErr := replayService.GetHumanResumeRolloverReplay(ctx, domainrepo.HumanResumeRolloverReplayRequest{
			SpaceID: sourceRun.SpaceID, ThreadID: req.ThreadID, SourceRunID: req.SourceRunID,
			IdempotencyKey: idempotencyKey, IdempotencyOperation: req.IdempotencyOperation,
			IdempotencyFingerprint: req.IdempotencyFingerprint,
			ResolvedJournalKey:     journalProjection.IdempotencyKey, InterruptID: interruptID,
			Response:                humanResumeRolloverStableResponse(response),
			PersistMessageReference: req.PersistMessageReference,
		})
		if replayErr == nil && replay != nil {
			if replay.Run == nil || replay.Message == nil || replay.Event == nil ||
				replay.Attempt == nil || replay.SourceAttempt == nil || replay.TerminalEvent == nil ||
				!replay.Replayed || replay.Run.ThreadID != req.ThreadID ||
				replay.Run.SpaceID != sourceRun.SpaceID || replay.Run.ID == req.SourceRunID {
				return nil, errors.New("human resume rollover replay result is incomplete")
			}
			return &ResumeHumanInteractionResponse{Run: DomainRunToSummary(replay.Run)}, nil
		}
		if replayErr != nil && !errors.Is(replayErr, domainrepo.ErrJournalNotEnrolled) {
			if errors.Is(replayErr, domainrepo.ErrHumanResumeRolloverConflict) ||
				errors.Is(replayErr, domainrepo.ErrActiveRunExists) {
				return nil, newHumanInteractionResumeSemanticError(
					ErrHumanInteractionResumeConflict, replayErr,
				)
			}
			return nil, replayErr
		}
		legacyReplay = errors.Is(replayErr, domainrepo.ErrJournalNotEnrolled)
	}
	if legacyReplay {
		existing, lookupErr := s.ThreadSVC.GetRunByIdempotencyKey(ctx, sourceRun.SpaceID, idempotencyKey)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if existing != nil {
			if validateErr := domainentity.ValidateRunIdempotencyValues(
				existing.Metadata, req.IdempotencyOperation, req.IdempotencyFingerprint,
			); validateErr != nil {
				return nil, validateErr
			}
			return &ResumeHumanInteractionResponse{Run: DomainRunToSummary(existing)}, nil
		}
	}
	if sourceRun.Status != domainentity.RunStatusInterrupted {
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeConflict,
			errors.New("source run must be interrupted"),
		)
	}

	var sourceAttempt *domainentity.RunAttempt
	if s == nil || s.JournalRecoveryRepository == nil {
		return nil, errors.New("journal attempt reader is unavailable")
	}
	sourceAttempt, err = s.JournalRecoveryRepository.GetActiveJournalAttempt(ctx, req.SourceRunID)
	switch {
	case err == nil && sourceAttempt == nil:
		return nil, errors.New("journal attempt reader returned no result")
	case err == nil && (sourceAttempt.ThreadID != req.ThreadID ||
		sourceAttempt.ExecutionRunID != req.SourceRunID ||
		sourceAttempt.JournalRunID <= 0 || strings.TrimSpace(sourceAttempt.AttemptID) == "" ||
		sourceAttempt.Status != domainentity.RunAttemptStatusRunning ||
		sourceAttempt.ActiveSlot == nil || *sourceAttempt.ActiveSlot != 1):
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeConflict,
			errors.New("active journal attempt does not match source run"),
		)
	case errors.Is(err, domainrepo.ErrJournalNotEnrolled):
		sourceAttempt = nil
	case errors.Is(err, domainrepo.ErrJournalAttemptTerminal):
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeConflict,
			errors.New("journal human interaction source attempt is terminal"),
		)
	case err != nil:
		return nil, err
	}

	checkpoint, envelope, err := s.latestActiveADKCheckpoint(ctx, req.ThreadID, req.SourceRunID)
	if err != nil {
		return nil, err
	}
	interrupt, ok := envelope.Interrupts[interruptID]
	if !ok {
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeInvalid,
			errors.New("interrupt id is not resumable"),
		)
	}
	if strings.TrimSpace(interrupt.ID) != "" && strings.TrimSpace(interrupt.ID) != interruptID {
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeInvalid,
			errors.New("interrupt id is not resumable"),
		)
	}
	prompt, ok := humanInteractionPromptFromInfo(interrupt.Info)
	if !ok || prompt == nil {
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeInvalid,
			errors.New("interrupt id is not a human interaction"),
		)
	}
	if prompt.InteractionID != response.InteractionID || prompt.Kind != response.Kind {
		return nil, newHumanInteractionResumeSemanticError(
			ErrHumanInteractionResumeInvalid,
			errors.New("human interaction response does not match interrupt"),
		)
	}
	response.SubmittedAt = time.Now().UnixMilli()

	resumeCommand, err := humanInteractionResumeCommand(checkpoint, interruptID, response)
	if err != nil {
		return nil, err
	}
	resolved := humanInteractionResolvedPayload(req.ThreadID, req.SourceRunID, 0, interruptID, response)
	metadata, err := humanInteractionResumeMetadata(checkpoint, req.SourceRunID, resolved)
	if err != nil {
		return nil, err
	}
	resumeConfig, err := stripSubmittedExecutionControls(sourceRun.Config)
	if err != nil {
		return nil, err
	}
	journalSource.Payload = encodeRunEventPayload(ctx, resolved)
	journalProjection, err = ProjectRunEventToJournal(journalSource)
	if err != nil || journalProjection == nil {
		if err == nil {
			err = errors.New("human interaction resolved journal projection is required")
		}
		return nil, err
	}
	journalProjectionErr := error(nil)
	var journalEnrollment *domainservice.JournalEnrollmentOptions
	if sourceAttempt != nil {
		journalEnrollment = &domainservice.JournalEnrollmentOptions{HumanResume: &domainservice.JournalHumanResumeEnrollmentOptions{
			JournalRunID: sourceAttempt.JournalRunID, SourceRunID: req.SourceRunID,
			SourceAttemptID: sourceAttempt.AttemptID, SourceCheckpointID: checkpoint.CheckpointID,
			IdempotencyKey: idempotencyKey,
		}}
	}

	bundle, err := s.ThreadSVC.CreateRunBundle(ctx, &domainservice.CreateRunBundleRequest{
		Run: domainservice.CreateRunRequest{
			ThreadID: req.ThreadID, AssistantID: sourceRun.AssistantID,
			Status: domainentity.RunStatusQueued, Command: resumeCommand,
			Input: `{"messages":[]}`, Config: resumeConfig, Context: sourceRun.Context,
			Metadata: metadata, StreamMode: sourceRun.StreamMode,
			MultitaskStrategy: "reject", OnDisconnect: sourceRun.OnDisconnect,
			Durability: sourceRun.Durability, IdempotencyKey: idempotencyKey,
			IdempotencyOperation:   req.IdempotencyOperation,
			IdempotencyFingerprint: req.IdempotencyFingerprint,
		},
		Message: &domainservice.CreateMessageSpec{
			Role:     domainentity.MessageRoleUser,
			Content:  humanInteractionUserMessageContent(response),
			Metadata: humanInteractionMessageMetadata(req.SourceRunID, interruptID, response),
		},
		Event: &domainservice.CreateRunEventSpec{
			EventType:               humanInteractionResolvedEventType,
			JournalSourceRunID:      req.SourceRunID,
			Journal:                 journalProjectionToDomainRequest(journalSource, journalProjection),
			JournalProjectionFailed: journalProjectionErr != nil,
			PayloadBuilder: func(runID int64) string {
				resolved["resume_run_id"] = runID
				return encodeRunEventPayload(ctx, resolved)
			},
		},
		EnrollJournal:           sourceAttempt != nil,
		JournalEnrollment:       journalEnrollment,
		PersistMessageReference: req.PersistMessageReference,
	})
	if err != nil {
		if errors.Is(err, domainrepo.ErrHumanResumeRolloverConflict) ||
			errors.Is(err, domainrepo.ErrActiveRunExists) {
			return nil, newHumanInteractionResumeSemanticError(ErrHumanInteractionResumeConflict, err)
		}
		return nil, err
	}
	if bundle == nil || bundle.Run == nil || bundle.Message == nil || bundle.Event == nil ||
		(sourceAttempt != nil && bundle.Attempt == nil) {
		return nil, fmt.Errorf("agent thread service returned incomplete human resume bundle")
	}
	s.cancelMultitaskInterruptedADKRuns(bundle.InterruptedRuns)

	return &ResumeHumanInteractionResponse{Run: DomainRunToSummary(bundle.Run)}, nil
}

func humanResumeRolloverStableResponse(
	response HumanInteractionResponse,
) domainrepo.HumanResumeRolloverResponse {
	return domainrepo.HumanResumeRolloverResponse{
		Schema: response.Schema, InteractionID: response.InteractionID,
		Kind: string(response.Kind), Decision: string(response.Decision),
		Answer: response.Answer, ChoiceID: response.ChoiceID, Comment: response.Comment,
		SubmittedBy: response.SubmittedBy, Source: response.Source,
	}
}

func (s *ApplicationService) latestActiveADKCheckpoint(
	ctx context.Context,
	threadID int64,
	runID int64,
) (*CheckpointSummary, ADKCheckpointEnvelope, error) {
	checkpoints, _, err := s.ThreadSVC.ListCheckpoints(ctx, &domainservice.ListCheckpointsRequest{
		ThreadID: threadID,
		RunID:    runID,
		Limit:    50,
	})
	if err != nil {
		return nil, ADKCheckpointEnvelope{}, err
	}

	var latest *domainentity.Checkpoint
	for _, checkpoint := range checkpoints {
		if checkpoint == nil || checkpoint.RuntimeDeletedAt > 0 {
			continue
		}
		summary := DomainCheckpointToSummary(checkpoint)
		mode, err := runtimeModeFromCheckpoint(summary)
		if err != nil || mode != RuntimeModeEinoADK {
			continue
		}
		if latest == nil ||
			checkpoint.CreatedAt > latest.CreatedAt ||
			(checkpoint.CreatedAt == latest.CreatedAt && checkpoint.ID > latest.ID) {
			latest = checkpoint
		}
	}
	if latest == nil {
		return nil, ADKCheckpointEnvelope{}, fmt.Errorf("active eino adk checkpoint is required")
	}

	summary := DomainCheckpointToSummary(latest)
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(summary.ChannelValues))
	if err != nil {
		return nil, ADKCheckpointEnvelope{}, fmt.Errorf("decode eino checkpoint: %w", err)
	}
	if summary.RuntimeKey != "" && summary.RuntimeKey != envelope.RuntimeKey {
		return nil, ADKCheckpointEnvelope{}, fmt.Errorf("checkpoint runtime key does not match envelope")
	}
	if summary.EnvelopeVersion != 0 && int(summary.EnvelopeVersion) != envelope.EnvelopeVersion {
		return nil, ADKCheckpointEnvelope{}, fmt.Errorf("checkpoint envelope version does not match indexed version")
	}

	return summary, envelope, nil
}

func humanInteractionResumeIdempotencyKey(
	req *ResumeHumanInteractionRequest,
	response HumanInteractionResponse,
) (string, error) {
	if req == nil {
		return "", fmt.Errorf("resume human interaction request is required")
	}
	key := strings.TrimSpace(req.IdempotencyKey)
	if key != "" {
		if len(key) > 128 {
			return "", fmt.Errorf("idempotency key is invalid")
		}
		return key, nil
	}

	raw, err := json.Marshal(map[string]any{
		"thread_id":       req.ThreadID,
		"source_run_id":   req.SourceRunID,
		"interrupt_id":    strings.TrimSpace(req.InterruptID),
		"response":        response,
		"response_schema": humanInteractionResponseSchema,
	})
	if err != nil {
		return "", fmt.Errorf("marshal idempotency payload: %w", err)
	}
	sum := sha256.Sum256(raw)

	return "human-resume:" + hex.EncodeToString(sum[:16]), nil
}

func humanInteractionResumeCommand(
	checkpoint *CheckpointSummary,
	interruptID string,
	response HumanInteractionResponse,
) (string, error) {
	if checkpoint == nil {
		return "", fmt.Errorf("checkpoint is required")
	}
	payload := map[string]any{
		"resume": map[string]any{
			"checkpoint_id": checkpoint.CheckpointID,
			"checkpoint_ns": checkpoint.CheckpointNS,
			"resume_from":   "interrupt",
			"targets": map[string]any{
				interruptID: response,
			},
		},
	}

	return marshalHumanInteractionJSON(payload, "resume command")
}

func humanInteractionResumeMetadata(
	checkpoint *CheckpointSummary,
	sourceRunID int64,
	resolved map[string]any,
) (string, error) {
	if checkpoint == nil {
		return "", fmt.Errorf("checkpoint is required")
	}
	payload := map[string]any{
		"checkpoint_resume": map[string]any{
			"protected_from_worker_claim": true,
			"checkpoint_id":               checkpoint.CheckpointID,
			"checkpoint_ns":               checkpoint.CheckpointNS,
			"resume_from":                 "interrupt",
			"source_run_id":               sourceRunID,
		},
		"human_interaction": resolved,
	}

	return marshalHumanInteractionJSON(payload, "resume metadata")
}

func humanInteractionMessageMetadata(
	sourceRunID int64,
	interruptID string,
	response HumanInteractionResponse,
) string {
	payload := map[string]any{
		"human_interaction": humanInteractionResolvedPayload(0, sourceRunID, 0, interruptID, response),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return `{}`
	}

	return string(raw)
}

func humanInteractionResolvedPayload(
	threadID int64,
	sourceRunID int64,
	resumeRunID int64,
	interruptID string,
	response HumanInteractionResponse,
) map[string]any {
	payload := map[string]any{
		"schema":         "coze.human_interaction_resolved.v1",
		"source_run_id":  sourceRunID,
		"interrupt_id":   strings.TrimSpace(interruptID),
		"interaction_id": strings.TrimSpace(response.InteractionID),
		"kind":           string(response.Kind),
		"decision":       string(response.Decision),
		"submitted_at":   response.SubmittedAt,
	}
	if threadID > 0 {
		payload["thread_id"] = threadID
	}
	if resumeRunID > 0 {
		payload["resume_run_id"] = resumeRunID
	}
	if strings.TrimSpace(response.ChoiceID) != "" {
		payload["choice_id"] = strings.TrimSpace(response.ChoiceID)
	}
	if strings.TrimSpace(response.SubmittedBy) != "" {
		payload["submitted_by"] = strings.TrimSpace(response.SubmittedBy)
	}
	if strings.TrimSpace(response.Source) != "" {
		payload["source"] = strings.TrimSpace(response.Source)
	}

	return payload
}

func humanInteractionUserMessageContent(response HumanInteractionResponse) string {
	switch response.Kind {
	case HumanInteractionKindClarification:
		if strings.TrimSpace(response.Answer) != "" {
			return strings.TrimSpace(response.Answer)
		}
		return strings.TrimSpace(response.ChoiceID)
	case HumanInteractionKindConfirmation:
		if response.Decision == HumanInteractionDecisionApproved {
			return "已确认执行"
		}
		comment := strings.TrimSpace(response.Comment)
		if comment == "" {
			return "已拒绝执行"
		}
		return "已拒绝执行：" + comment
	default:
		return ""
	}
}

func marshalHumanInteractionJSON(value any, name string) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", name, err)
	}

	return string(raw), nil
}
