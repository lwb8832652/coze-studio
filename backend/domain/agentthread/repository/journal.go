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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type JournalRepository interface {
	CreateJournalAttempt(ctx context.Context, attempt *entity.RunAttempt) (*entity.RunAttempt, error)
	GetActiveJournalAttempt(ctx context.Context, runID int64) (*entity.RunAttempt, error)
	ListJournalAttempts(ctx context.Context, runID int64) ([]*entity.RunAttempt, error)
	AppendJournalEvent(ctx context.Context, event *entity.JournalEvent) (*entity.JournalEvent, error)
	FinalizeJournalAttempt(
		ctx context.Context,
		req FinalizeJournalAttemptRequest,
	) (*entity.JournalEvent, bool, error)
	GetJournalEvent(ctx context.Context, eventID int64) (*entity.JournalEvent, error)
	ListJournalEvents(ctx context.Context, req ListJournalEventsRequest) (*ListJournalEventsResult, error)
}

// RunEventProjectionRepository atomically preserves the existing RunEvent view
// and, when the run is enrolled, enriches the same row with a public Journal
// projection.
type RunEventProjectionRepository interface {
	CreateRunEventWithJournalProjection(
		ctx context.Context,
		req CreateRunEventWithJournalProjectionRequest,
	) (*entity.JournalEvent, error)
}

type CreateRunEventWithJournalProjectionRequest struct {
	Event            *entity.RunEvent
	Journal          *entity.JournalEvent
	ProjectionFailed bool
}

type Repository interface {
	ThreadRepository
	JournalRepository
}

type FinalizeJournalAttemptRequest struct {
	RunID   int64
	Status  entity.RunAttemptStatus
	Event   *entity.JournalEvent
	EndedAt int64
}

type ListJournalEventsRequest struct {
	RunID         int64
	AttemptID     string
	AfterSequence uint64
	AfterEventID  int64
	Limit         int
}

type ListJournalEventsResult struct {
	Events  []*entity.JournalEvent
	HasMore bool
	Legacy  bool
}
