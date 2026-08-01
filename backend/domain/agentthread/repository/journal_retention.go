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
	"errors"
)

var ErrJournalRetentionClaimLost = errors.New("journal retention cleanup claim is no longer owned")

type JournalRetentionClaimRequest struct {
	CutoffAt   int64
	Now        int64
	LeaseUntil int64
	BatchSize  int
}

type JournalSnapshotCleanupClaim struct {
	SpaceID    int64
	SnapshotID string
	ClaimToken string
	ObjectKeys []string
}

type JournalStagingCleanupClaim struct {
	SpaceID    int64
	SnapshotID string
	ClaimToken string
	Prefix     string
}

type JournalRetentionDeleteRequest struct {
	CutoffAt  int64
	BatchSize int
}

type JournalRetentionBacklogRequest struct {
	SnapshotCutoffAt  int64
	ExecutionCutoffAt int64
	StagingCutoffAt   int64
}

type JournalExecutionCleanupResult struct {
	EventsDeleted  int64
	LedgersDeleted int64
}

type JournalRetentionRepository interface {
	ClaimJournalSnapshotCleanup(context.Context, JournalRetentionClaimRequest) ([]JournalSnapshotCleanupClaim, error)
	CompleteJournalSnapshotCleanup(context.Context, JournalSnapshotCleanupClaim) error
	ReleaseJournalSnapshotCleanup(context.Context, JournalSnapshotCleanupClaim, string) error
	ClaimJournalStagingCleanup(context.Context, JournalRetentionClaimRequest) ([]JournalStagingCleanupClaim, error)
	CompleteJournalStagingCleanup(context.Context, JournalStagingCleanupClaim) error
	ReleaseJournalStagingCleanup(context.Context, JournalStagingCleanupClaim, string) error
	DeleteExpiredJournalCheckpoints(context.Context, JournalRetentionDeleteRequest) (int64, error)
	DeleteExpiredJournalEventsAndLedgers(context.Context, JournalRetentionDeleteRequest) (JournalExecutionCleanupResult, error)
	DeleteUnreferencedJournalAttempts(context.Context, JournalRetentionDeleteRequest) (int64, error)
	CountJournalRetentionBacklog(context.Context, JournalRetentionBacklogRequest) (int64, error)
}
