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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdaptiveDecisionModelOperationPrepareElectsOneOwnerAndFailsClosedForLoser(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveDecisionModelOperationRepository(db)
	req := newAdaptiveDecisionModelPrepareRequestForTest()

	winner, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), req)
	require.NoError(t, err)
	require.True(t, winner.Owned)
	require.Equal(t, AdaptiveDecisionModelOperationStatusCalling, winner.Operation.Status)

	loserReq := req
	loserReq.ClaimEventID++
	loserReq.ResultEventID++
	loserReq.ClaimToken = "claim-token-loser"
	loser, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), loserReq)
	require.NoError(t, err)
	require.False(t, loser.Owned)
	require.Equal(t, AdaptiveDecisionModelOperationStatusCalling, loser.Operation.Status)
	require.Equal(t, winner.Operation, loser.Operation)

	var events []runEventPO
	require.NoError(t, db.Where(
		"journal_run_id = ? AND attempt_id = ? AND event_type = ?",
		req.JournalRunID,
		req.AttemptID,
		adaptiveDecisionModelClaimEventType,
	).Find(&events).Error)
	require.Len(t, events, 1)
	require.Nil(t, events[0].Sequence)
	require.Equal(t, "internal", *events[0].Visibility)
	require.NotContains(t, string(events[0].Payload), req.OperationKey)
	require.NotContains(t, string(events[0].Payload), req.ClaimToken)
}

func TestAdaptiveDecisionModelOperationCompletesAndReplaysExactResult(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveDecisionModelOperationRepository(db)
	prepare := newAdaptiveDecisionModelPrepareRequestForTest()
	claimed, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
	require.NoError(t, err)
	require.True(t, claimed.Owned)

	payload := []byte(`{"decision":"direct","score":1}`)
	complete := newAdaptiveDecisionModelCompleteRequestForTest(
		prepare,
		AdaptiveDecisionModelOperationStatusCompleted,
		payload,
		"",
	)
	completed, err := repo.CompleteAdaptiveDecisionModelOperation(context.Background(), complete)
	require.NoError(t, err)
	require.False(t, completed.Replayed)
	require.Equal(t, AdaptiveDecisionModelOperationStatusCompleted, completed.Operation.Status)
	require.JSONEq(t, string(payload), string(completed.Operation.ResultPayload))
	digest := sha256.Sum256(payload)
	require.Equal(t, hex.EncodeToString(digest[:]), completed.Operation.ResultDigest)
	require.Empty(t, completed.Operation.ErrorCode)

	complete.Now++
	replayed, err := repo.CompleteAdaptiveDecisionModelOperation(context.Background(), complete)
	require.NoError(t, err)
	require.True(t, replayed.Replayed)
	require.Equal(t, completed.Operation, replayed.Operation)

	read, err := repo.ReadAdaptiveDecisionModelOperation(
		context.Background(),
		newAdaptiveDecisionModelReadRequestForTest(prepare),
	)
	require.NoError(t, err)
	require.Equal(t, completed.Operation, read)

	loser := prepare
	loser.ClaimEventID += 10
	loser.ResultEventID += 10
	loser.ClaimToken = "another-claim-token"
	alreadyCompleted, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), loser)
	require.NoError(t, err)
	require.False(t, alreadyCompleted.Owned)
	require.Equal(t, completed.Operation, alreadyCompleted.Operation)
	requireAdaptiveDecisionModelEventShapeForTest(t, db, prepare, 2)
}

func TestAdaptiveDecisionModelOperationReadClaimOnlyFailsClosed(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveDecisionModelOperationRepository(db)
	prepare := newAdaptiveDecisionModelPrepareRequestForTest()
	_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
	require.NoError(t, err)

	read, err := repo.ReadAdaptiveDecisionModelOperation(
		context.Background(),
		newAdaptiveDecisionModelReadRequestForTest(prepare),
	)
	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionModelOperationStatusCalling, read.Status)
	require.Empty(t, read.ResultPayload)
	require.Empty(t, read.ResultDigest)
	require.Empty(t, read.ErrorCode)
}

func TestAdaptiveDecisionModelOperationFailedResultStoresOnlyClosedCode(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveDecisionModelOperationRepository(db)
	prepare := newAdaptiveDecisionModelPrepareRequestForTest()
	_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
	require.NoError(t, err)

	failed, err := repo.CompleteAdaptiveDecisionModelOperation(
		context.Background(),
		newAdaptiveDecisionModelCompleteRequestForTest(
			prepare,
			AdaptiveDecisionModelOperationStatusFailed,
			nil,
			"model_output_invalid",
		),
	)
	require.NoError(t, err)
	require.Equal(t, AdaptiveDecisionModelOperationStatusFailed, failed.Operation.Status)
	require.Equal(t, "model_output_invalid", failed.Operation.ErrorCode)
	require.Empty(t, failed.Operation.ResultPayload)
	require.Empty(t, failed.Operation.ResultDigest)

	var result runEventPO
	require.NoError(t, db.Where("id = ?", prepare.ResultEventID).First(&result).Error)
	require.NotContains(t, string(result.Payload), prepare.OperationKey)
	require.NotContains(t, string(result.Payload), prepare.ClaimToken)
	require.NotContains(t, string(result.Payload), "provider response")
}

func TestAdaptiveDecisionModelOperationRejectsDriftPartialAndTamper(t *testing.T) {
	t.Run("request fingerprint drift", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveDecisionModelOperationRepository(db)
		prepare := newAdaptiveDecisionModelPrepareRequestForTest()
		_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
		require.NoError(t, err)
		before := snapshotAdaptiveExecutionDBForTest(t, db)

		drifted := prepare
		drifted.RequestFingerprint = strings.Repeat("f", sha256.Size*2)
		_, err = repo.PrepareAdaptiveDecisionModelOperation(context.Background(), drifted)
		require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationConflict)
		require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
	})

	t.Run("result without claim is partial", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveDecisionModelOperationRepository(db)
		prepare := newAdaptiveDecisionModelPrepareRequestForTest()
		_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
		require.NoError(t, err)
		_, err = repo.CompleteAdaptiveDecisionModelOperation(
			context.Background(),
			newAdaptiveDecisionModelCompleteRequestForTest(
				prepare,
				AdaptiveDecisionModelOperationStatusCompleted,
				[]byte(`{"decision":"direct"}`),
				"",
			),
		)
		require.NoError(t, err)
		require.NoError(t, db.Where("id = ?", prepare.ClaimEventID).Delete(&runEventPO{}).Error)

		_, err = repo.ReadAdaptiveDecisionModelOperation(
			context.Background(),
			newAdaptiveDecisionModelReadRequestForTest(prepare),
		)
		require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationConflict)
	})

	t.Run("result payload tamper", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveDecisionModelOperationRepository(db)
		prepare := newAdaptiveDecisionModelPrepareRequestForTest()
		_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
		require.NoError(t, err)
		_, err = repo.CompleteAdaptiveDecisionModelOperation(
			context.Background(),
			newAdaptiveDecisionModelCompleteRequestForTest(
				prepare,
				AdaptiveDecisionModelOperationStatusCompleted,
				[]byte(`{"decision":"direct"}`),
				"",
			),
		)
		require.NoError(t, err)
		require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", prepare.ResultEventID).
			Update("payload", []byte(`{"tampered":true}`)).Error)

		_, err = repo.ReadAdaptiveDecisionModelOperation(
			context.Background(),
			newAdaptiveDecisionModelReadRequestForTest(prepare),
		)
		require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationConflict)
	})
}

func TestAdaptiveDecisionModelOperationRejectsInvalidResultWithoutWrites(t *testing.T) {
	tests := []struct {
		name      string
		status    AdaptiveDecisionModelOperationStatus
		payload   []byte
		errorCode string
	}{
		{name: "non canonical payload", status: AdaptiveDecisionModelOperationStatusCompleted, payload: []byte(`{ "b":2,"a":1}`)},
		{name: "oversized payload", status: AdaptiveDecisionModelOperationStatusCompleted, payload: []byte(`"` + strings.Repeat("a", 64*1024) + `"`)},
		{name: "completed with error code", status: AdaptiveDecisionModelOperationStatusCompleted, payload: []byte(`{"ok":true}`), errorCode: "model_failed"},
		{name: "failed with payload", status: AdaptiveDecisionModelOperationStatusFailed, payload: []byte(`{"raw":"provider response"}`), errorCode: "model_failed"},
		{name: "failed with free text code", status: AdaptiveDecisionModelOperationStatusFailed, errorCode: "provider failed: secret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			repo := NewAdaptiveDecisionModelOperationRepository(db)
			prepare := newAdaptiveDecisionModelPrepareRequestForTest()
			_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
			require.NoError(t, err)
			before := snapshotAdaptiveExecutionDBForTest(t, db)

			_, err = repo.CompleteAdaptiveDecisionModelOperation(
				context.Background(),
				newAdaptiveDecisionModelCompleteRequestForTest(
					prepare,
					test.status,
					test.payload,
					test.errorCode,
				),
			)
			require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationInvalid)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveDecisionModelOperationFencesPrepareAndCompleteWithoutWrites(t *testing.T) {
	t.Run("prepare lease and generation", func(t *testing.T) {
		for _, mutate := range []func(*PrepareAdaptiveDecisionModelOperationRequest){
			func(req *PrepareAdaptiveDecisionModelOperationRequest) { req.LeaseToken = "lost-lease" },
			func(req *PrepareAdaptiveDecisionModelOperationRequest) { req.Generation++ },
			func(req *PrepareAdaptiveDecisionModelOperationRequest) { req.Now = 2000 },
		} {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			repo := NewAdaptiveDecisionModelOperationRepository(db)
			req := newAdaptiveDecisionModelPrepareRequestForTest()
			mutate(&req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)

			_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), req)
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrRunLeaseLost) || errors.Is(err, ErrAdaptiveExecutionAttemptConflict))
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		}
	})

	t.Run("non fresh attempt", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		source := "source-attempt"
		require.NoError(t, db.Model(&runAttemptPO{}).
			Where("journal_run_id = ? AND attempt_id = ?", int64(30), "attempt-1").
			Update("source_attempt_id", source).Error)
		before := snapshotAdaptiveExecutionDBForTest(t, db)

		_, err := NewAdaptiveDecisionModelOperationRepository(db).PrepareAdaptiveDecisionModelOperation(
			context.Background(),
			newAdaptiveDecisionModelPrepareRequestForTest(),
		)
		require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationConflict)
		require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
	})

	t.Run("complete lease", func(t *testing.T) {
		db := newAdaptiveExecutionRepositoryTestDB(t)
		seedAdaptiveExecutionInitialState(t, db)
		repo := NewAdaptiveDecisionModelOperationRepository(db)
		prepare := newAdaptiveDecisionModelPrepareRequestForTest()
		_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
		require.NoError(t, err)
		before := snapshotAdaptiveExecutionDBForTest(t, db)
		complete := newAdaptiveDecisionModelCompleteRequestForTest(
			prepare,
			AdaptiveDecisionModelOperationStatusCompleted,
			[]byte(`{"decision":"direct"}`),
			"",
		)
		complete.LeaseToken = "lost-lease"

		_, err = repo.CompleteAdaptiveDecisionModelOperation(context.Background(), complete)
		require.ErrorIs(t, err, ErrRunLeaseLost)
		require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
	})
}

func TestAdaptiveDecisionModelOperationRejectsResultReplayDrift(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveDecisionModelOperationRepository(db)
	prepare := newAdaptiveDecisionModelPrepareRequestForTest()
	_, err := repo.PrepareAdaptiveDecisionModelOperation(context.Background(), prepare)
	require.NoError(t, err)
	complete := newAdaptiveDecisionModelCompleteRequestForTest(
		prepare,
		AdaptiveDecisionModelOperationStatusCompleted,
		[]byte(`{"decision":"direct"}`),
		"",
	)
	_, err = repo.CompleteAdaptiveDecisionModelOperation(context.Background(), complete)
	require.NoError(t, err)
	before := snapshotAdaptiveExecutionDBForTest(t, db)
	complete.ResultPayload = []byte(`{"decision":"plan"}`)

	_, err = repo.CompleteAdaptiveDecisionModelOperation(context.Background(), complete)
	require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveDecisionModelOperationReadMissing(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	prepare := newAdaptiveDecisionModelPrepareRequestForTest()

	_, err := NewAdaptiveDecisionModelOperationRepository(db).ReadAdaptiveDecisionModelOperation(
		context.Background(),
		newAdaptiveDecisionModelReadRequestForTest(prepare),
	)
	require.ErrorIs(t, err, ErrAdaptiveDecisionModelOperationNotFound)
}

func newAdaptiveDecisionModelPrepareRequestForTest() PrepareAdaptiveDecisionModelOperationRequest {
	return PrepareAdaptiveDecisionModelOperationRequest{
		ThreadID:           10,
		ExecutionRunID:     20,
		JournalRunID:       30,
		AttemptID:          "attempt-1",
		Generation:         3,
		OperationKey:       "adaptive-decision-model:operation-1",
		RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		LeaseOwner:         "worker-1",
		LeaseToken:         "lease-1",
		ClaimEventID:       7101,
		ResultEventID:      7102,
		ClaimToken:         "claim-token-winner",
		Now:                1000,
	}
}

func newAdaptiveDecisionModelReadRequestForTest(
	prepare PrepareAdaptiveDecisionModelOperationRequest,
) ReadAdaptiveDecisionModelOperationRequest {
	return ReadAdaptiveDecisionModelOperationRequest{
		ThreadID: prepare.ThreadID, ExecutionRunID: prepare.ExecutionRunID,
		JournalRunID: prepare.JournalRunID, AttemptID: prepare.AttemptID,
		Generation: prepare.Generation, OperationKey: prepare.OperationKey,
		RequestFingerprint: prepare.RequestFingerprint,
	}
}

func newAdaptiveDecisionModelCompleteRequestForTest(
	prepare PrepareAdaptiveDecisionModelOperationRequest,
	status AdaptiveDecisionModelOperationStatus,
	payload []byte,
	errorCode string,
) CompleteAdaptiveDecisionModelOperationRequest {
	return CompleteAdaptiveDecisionModelOperationRequest{
		ThreadID: prepare.ThreadID, ExecutionRunID: prepare.ExecutionRunID,
		JournalRunID: prepare.JournalRunID, AttemptID: prepare.AttemptID,
		Generation: prepare.Generation, OperationKey: prepare.OperationKey,
		RequestFingerprint: prepare.RequestFingerprint,
		LeaseOwner:         prepare.LeaseOwner, LeaseToken: prepare.LeaseToken,
		ClaimToken: prepare.ClaimToken, Now: prepare.Now,
		Status: status, ResultPayload: append([]byte(nil), payload...), ErrorCode: errorCode,
	}
}

func requireAdaptiveDecisionModelEventShapeForTest(
	t *testing.T,
	db *gorm.DB,
	prepare PrepareAdaptiveDecisionModelOperationRequest,
	want int,
) {
	t.Helper()
	var events []runEventPO
	require.NoError(t, db.Where(
		"journal_run_id = ? AND attempt_id = ? AND event_type IN ?",
		prepare.JournalRunID,
		prepare.AttemptID,
		[]string{adaptiveDecisionModelClaimEventType, adaptiveDecisionModelResultEventType},
	).Order("id ASC").Find(&events).Error)
	require.Len(t, events, want)
	for _, event := range events {
		require.Nil(t, event.Sequence)
		require.NotNil(t, event.Visibility)
		require.Equal(t, "internal", *event.Visibility)
		require.NotContains(t, string(event.Payload), prepare.OperationKey)
		require.NotContains(t, string(event.Payload), prepare.ClaimToken)
	}
}
