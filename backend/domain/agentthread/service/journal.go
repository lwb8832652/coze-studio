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

package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func newRunTerminalJournalEvent(
	base *entity.RunEvent,
	runStatus entity.RunStatus,
	errorCode string,
) (*entity.JournalEvent, error) {
	if base == nil || base.ID <= 0 || base.RunID <= 0 || base.ThreadID <= 0 {
		return nil, fmt.Errorf("terminal base run event is required")
	}
	attemptStatus, ok := terminalJournalAttemptStatus(runStatus, errorCode)
	if !ok {
		return nil, nil
	}
	payload, err := json.Marshal(map[string]any{
		"type": "terminal",
		"data": map[string]any{"status": string(attemptStatus)},
	})
	if err != nil {
		return nil, err
	}
	return &entity.JournalEvent{
		ID: base.ID, ThreadID: base.ThreadID, RunID: base.RunID,
		IdempotencyKey: fmt.Sprintf("journal:run:%d:terminal:%s", base.RunID, attemptStatus),
		SchemaVersion:  entity.JournalSchemaVersion, Status: string(attemptStatus),
		OccurredAtUnixNano: base.CreatedAt * int64(1_000_000),
		Visibility:         entity.JournalVisibilityUser,
		PayloadVersion:     entity.JournalPayloadVersion,
		EventType:          "run.lifecycle",
		Payload:            string(payload),
		CreatedAt:          base.CreatedAt,
	}, nil
}

func terminalJournalAttemptStatus(
	runStatus entity.RunStatus,
	errorCode string,
) (entity.RunAttemptStatus, bool) {
	switch runStatus {
	case entity.RunStatusSucceeded:
		return entity.RunAttemptStatusCompleted, true
	case entity.RunStatusCanceled:
		return entity.RunAttemptStatusCancelled, true
	case entity.RunStatusFailed:
		if entity.IsJournalTimeoutErrorCode(errorCode) {
			return entity.RunAttemptStatusTimedOut, true
		}
		return entity.RunAttemptStatusFailed, true
	default:
		return "", false
	}
}

func (s *threadService) persistRunEventWithOptionalJournal(
	ctx context.Context,
	event *entity.RunEvent,
	projection *AppendJournalEventRequest,
	projectionFailed bool,
) error {
	projectionRepo, ok := s.repo.(repository.RunEventProjectionRepository)
	if !ok || (projection == nil && !projectionFailed) {
		return s.repo.CreateRunEvent(ctx, event)
	}
	var journal *entity.JournalEvent
	if projection != nil {
		journal = journalEventFromServiceRequest(projection, event.ID, event.ThreadID)
		journal.RunID = event.RunID
		journal.CreatedAt = event.CreatedAt
	}
	_, err := projectionRepo.CreateRunEventWithJournalProjection(
		ctx,
		repository.CreateRunEventWithJournalProjectionRequest{
			Event: event, Journal: journal, ProjectionFailed: projectionFailed,
		},
	)
	return err
}
