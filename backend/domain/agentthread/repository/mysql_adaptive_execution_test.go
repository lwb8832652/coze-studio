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
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestValidateAdaptiveExecutionBoundaryRequest(t *testing.T) {
	newValidRequest := func() CommitAdaptiveExecutionBoundaryRequest {
		return CommitAdaptiveExecutionBoundaryRequest{
			ThreadID:       10,
			ExecutionRunID: 20,
			JournalRunID:   30,
			AttemptID:      "attempt-1",
			Generation:     3,
			LeaseOwner:     "worker-1",
			LeaseToken:     "lease-1",
			Now:            1000,
			IdempotencyKey: "boundary-1",
			Event: &entity.RunEvent{
				ID: 40, ThreadID: 10, RunID: 20,
				EventType: "run.boundary", Payload: `{}`, CreatedAt: 1000,
			},
			Checkpoint: &entity.Checkpoint{
				ID: 50, ThreadID: 10, RunID: 20,
				CheckpointNS: "adaptive", RuntimeType: "eino_adk", CreatedAt: 1000,
			},
		}
	}

	require.NoError(t, validateAdaptiveExecutionBoundaryRequest(newValidRequest()))

	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "thread id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.ThreadID = 0 }},
		{name: "execution run id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.ExecutionRunID = 0 }},
		{name: "journal run id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.JournalRunID = 0 }},
		{name: "attempt id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.AttemptID = "" }},
		{name: "generation", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Generation = 0 }},
		{name: "lease owner", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.LeaseOwner = "" }},
		{name: "lease token", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.LeaseToken = "" }},
		{name: "now", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Now = 0 }},
		{name: "idempotency key", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.IdempotencyKey = "" }},
		{name: "event", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event = nil }},
		{name: "checkpoint", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint = nil }},
		{name: "event thread drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.ThreadID++ }},
		{name: "event run drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.RunID++ }},
		{name: "checkpoint thread drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ThreadID++ }},
		{name: "checkpoint run drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RunID++ }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newValidRequest()
			tt.mutate(&req)
			require.ErrorIs(t, validateAdaptiveExecutionBoundaryRequest(req), ErrAdaptiveExecutionBoundaryInvalid)
		})
	}
}

func TestLockAdaptiveExecutionRun(t *testing.T) {
	const now = int64(1000)

	tests := []struct {
		name          string
		mutateRun     func(*runPO)
		mutateRequest func(*CommitAdaptiveExecutionBoundaryRequest)
		expectedErr   error
	}{
		{name: "success"},
		{
			name: "wrong thread id",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.ThreadID++
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "non-running",
			mutateRun: func(run *runPO) {
				run.Status = string(entity.RunStatusPending)
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "stale generation",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.Generation--
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "wrong owner",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.LeaseOwner = "worker-2"
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "wrong token",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.LeaseToken = "lease-2"
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "expiry equals now",
			mutateRun: func(run *runPO) {
				expiresAt := now
				run.LeaseExpiresAt = &expiresAt
			},
			expectedErr: ErrRunLeaseLost,
		},
		{
			name: "cancel requested",
			mutateRun: func(run *runPO) {
				cancelRequestedAt := now - 1
				run.CancelRequestedAt = &cancelRequestedAt
			},
			expectedErr: ErrRunCanceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			seedJournalThread(t, db, 10)
			leaseOwner := "worker-1"
			leaseToken := "lease-1"
			leaseExpiresAt := now + 1
			run := runPO{
				ID: 20, ThreadID: 10, Status: string(entity.RunStatusRunning),
				LeaseOwner: &leaseOwner, LeaseToken: &leaseToken,
				LeaseExpiresAt: &leaseExpiresAt, ExecutionGeneration: 3,
			}
			if tt.mutateRun != nil {
				tt.mutateRun(&run)
			}
			require.NoError(t, db.Create(&run).Error)

			var before runPO
			require.NoError(t, db.Where("id = ?", run.ID).First(&before).Error)
			req := newAdaptiveExecutionBoundaryRequestForFenceTest()
			if tt.mutateRequest != nil {
				tt.mutateRequest(&req)
			}

			var locked *runPO
			err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				locked, err = lockAdaptiveExecutionRun(tx, req)
				return err
			})
			if tt.expectedErr == nil {
				require.NoError(t, err)
				require.Equal(t, before, *locked)
			} else {
				require.ErrorIs(t, err, tt.expectedErr)
			}

			var after runPO
			require.NoError(t, db.Where("id = ?", run.ID).First(&after).Error)
			require.Equal(t, before, after)
		})
	}
}

func TestLockAdaptiveExecutionAttempt(t *testing.T) {
	tests := []struct {
		name          string
		mutateAttempt func(*runAttemptPO)
		mutateRequest func(*CommitAdaptiveExecutionBoundaryRequest)
		expectedErr   error
	}{
		{name: "success"},
		{
			name: "terminal status",
			mutateAttempt: func(attempt *runAttemptPO) {
				attempt.Status = string(entity.RunAttemptStatusCompleted)
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "null active slot",
			mutateAttempt: func(attempt *runAttemptPO) {
				attempt.ActiveSlot = nil
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "execution run drift",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.ExecutionRunID++
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
		{
			name: "cross-thread",
			mutateRequest: func(req *CommitAdaptiveExecutionBoundaryRequest) {
				req.ThreadID++
			},
			expectedErr: ErrAdaptiveExecutionAttemptConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newJournalRepositoryTestDB(t)
			activeSlot := uint8(1)
			attempt := runAttemptPO{
				ID: 100, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20,
				AttemptID: "attempt-1", Ordinal: 1,
				Status: string(entity.RunAttemptStatusRunning), ActiveSlot: &activeSlot,
				NextSequence: 7, LastCommittedSequence: 6,
				EnrollmentVersion: entity.JournalSchemaVersion,
				ProjectionState:   string(entity.JournalProjectionStateHealthy),
				CreatedAt:         1000, UpdatedAt: 1000,
			}
			if tt.mutateAttempt != nil {
				tt.mutateAttempt(&attempt)
			}
			require.NoError(t, db.Create(&attempt).Error)

			var before runAttemptPO
			require.NoError(t, db.Where("id = ?", attempt.ID).First(&before).Error)
			req := newAdaptiveExecutionBoundaryRequestForFenceTest()
			if tt.mutateRequest != nil {
				tt.mutateRequest(&req)
			}

			var locked *runAttemptPO
			err := db.Transaction(func(tx *gorm.DB) error {
				var err error
				locked, err = lockAdaptiveExecutionAttempt(tx, req)
				return err
			})
			if tt.expectedErr == nil {
				require.NoError(t, err)
				require.Equal(t, before, *locked)
			} else {
				require.ErrorIs(t, err, tt.expectedErr)
			}

			var after runAttemptPO
			require.NoError(t, db.Where("id = ?", attempt.ID).First(&after).Error)
			require.Equal(t, before, after)
		})
	}
}

func newAdaptiveExecutionBoundaryRequestForFenceTest() CommitAdaptiveExecutionBoundaryRequest {
	return CommitAdaptiveExecutionBoundaryRequest{
		ThreadID:       10,
		ExecutionRunID: 20,
		JournalRunID:   30,
		AttemptID:      "attempt-1",
		Generation:     3,
		LeaseOwner:     "worker-1",
		LeaseToken:     "lease-1",
		Now:            1000,
		IdempotencyKey: "boundary-1",
		Event:          &entity.RunEvent{ID: 40, ThreadID: 10, RunID: 20},
		Checkpoint:     &entity.Checkpoint{ID: 50, ThreadID: 10, RunID: 20},
	}
}
