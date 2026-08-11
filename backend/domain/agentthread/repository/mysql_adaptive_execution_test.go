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
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestValidateAdaptiveVerifiedSuccessGate(t *testing.T) {
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(FinalizeRunSuccessRequest{}))

	valid := newValidAdaptiveVerifiedSuccessRequestForTest()
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(valid))
	atSequenceLimit := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
	atSequenceLimit.AdaptiveGate.Evidence.EventSequence = math.MaxUint64 - 3
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(atSequenceLimit))
	withoutTitleOrFallback := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
	withoutTitleOrFallback.TitleEvent = nil
	withoutTitleOrFallback.TerminalCheckpointOnTitleConflict = nil
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(withoutTitleOrFallback))

	t.Run("DurablePayloadStrictDecode", func(t *testing.T) {
		caller := valid.AdaptiveGate.VerificationEvent.Payload
		validNull := adaptiveVerifiedSuccessDurablePayloadForTest(
			t,
			caller,
			nil,
			strings.Repeat("d", 64),
		)
		_, err := decodeAdaptiveVerifiedSuccessPayload(validNull, true)
		require.NoError(t, err)

		validOutbox := adaptiveVerifiedSuccessDurablePayloadForTest(
			t,
			caller,
			strings.Repeat("e", 64),
			strings.Repeat("d", 64),
		)
		_, err = decodeAdaptiveVerifiedSuccessPayload(validOutbox, true)
		require.NoError(t, err)

		invalid := []struct {
			name    string
			payload string
		}{
			{
				name: "missing outbox fingerprint",
				payload: adaptiveVerifiedSuccessPayloadMutationForTest(t, validNull, func(fields map[string]any) {
					delete(fields, "outbox_fingerprint")
				}),
			},
			{
				name: "missing finalize request fingerprint",
				payload: adaptiveVerifiedSuccessPayloadMutationForTest(t, validNull, func(fields map[string]any) {
					delete(fields, "finalize_request_fingerprint")
				}),
			},
			{
				name: "invalid nullable outbox fingerprint",
				payload: adaptiveVerifiedSuccessPayloadMutationForTest(t, validNull, func(fields map[string]any) {
					fields["outbox_fingerprint"] = strings.Repeat("A", 64)
				}),
			},
			{
				name: "invalid finalize request fingerprint",
				payload: adaptiveVerifiedSuccessPayloadMutationForTest(t, validNull, func(fields map[string]any) {
					fields["finalize_request_fingerprint"] = strings.Repeat("d", 63)
				}),
			},
			{
				name: "required field has wrong type",
				payload: adaptiveVerifiedSuccessPayloadMutationForTest(t, validNull, func(fields map[string]any) {
					fields["decision_revision"] = "1"
				}),
			},
			{name: "multiple json values", payload: validNull + ` {}`},
			{name: "raw payload exceeds limit", payload: validNull + strings.Repeat(" ", adaptiveVerifiedSuccessMaxPayloadBytes)},
		}
		for _, test := range invalid {
			t.Run(test.name, func(t *testing.T) {
				_, decodeErr := decodeAdaptiveVerifiedSuccessPayload(test.payload, true)
				require.ErrorIs(t, decodeErr, ErrAdaptiveExecutionVerifiedSuccessInvalid)
			})
		}
	})

	t.Run("CallerPayloadBudgetIncludesServerFields", func(t *testing.T) {
		exact := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
		exact.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessCallerPayloadAtDurableSizeForTest(
			t,
			exact.AdaptiveGate.VerificationEvent.Payload,
			adaptiveVerifiedSuccessMaxPayloadBytes,
			false,
		)
		require.NoError(t, validateAdaptiveVerifiedSuccessGate(exact))

		tooLarge := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
		tooLarge.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessCallerPayloadAtDurableSizeForTest(
			t,
			tooLarge.AdaptiveGate.VerificationEvent.Payload,
			adaptiveVerifiedSuccessMaxPayloadBytes+1,
			false,
		)
		require.ErrorIs(
			t,
			validateAdaptiveVerifiedSuccessGate(tooLarge),
			ErrAdaptiveExecutionVerifiedSuccessInvalid,
		)

		withOutbox := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
		withOutbox.OutboxIntent = &NotificationOutboxIntent{
			AppendWithResult: func(context.Context, *gorm.DB, domainnotification.Event) (bool, error) {
				return true, nil
			},
		}
		withOutbox.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessCallerPayloadAtDurableSizeForTest(
			t,
			withOutbox.AdaptiveGate.VerificationEvent.Payload,
			adaptiveVerifiedSuccessMaxPayloadBytes,
			true,
		)
		require.NoError(t, validateAdaptiveVerifiedSuccessGate(withOutbox))

		withOutboxTooLarge := cloneAdaptiveVerifiedSuccessRequestForTest(withOutbox)
		withOutboxTooLarge.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessCallerPayloadAtDurableSizeForTest(
			t,
			valid.AdaptiveGate.VerificationEvent.Payload,
			adaptiveVerifiedSuccessMaxPayloadBytes+1,
			true,
		)
		require.ErrorIs(
			t,
			validateAdaptiveVerifiedSuccessGate(withOutboxTooLarge),
			ErrAdaptiveExecutionVerifiedSuccessInvalid,
		)
	})

	invalid := []struct {
		name   string
		mutate func(*FinalizeRunSuccessRequest)
	}{
		{name: "outer now required", mutate: func(req *FinalizeRunSuccessRequest) {
			req.Now = 0
		}},
		{name: "decision thread identity drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Decision.ThreadID++
		}},
		{name: "decision execution identity drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Decision.ExecutionRunID++
		}},
		{name: "evidence journal identity drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Evidence.JournalRunID++
		}},
		{name: "evidence attempt identity drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Evidence.AttemptID = "attempt-2"
		}},
		{name: "evidence generation drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Evidence.ExecutionGeneration++
		}},
		{name: "evidence plan scope drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Evidence.PlanScopeRunID++
		}},
		{name: "evidence source pair incomplete", mutate: func(req *FinalizeRunSuccessRequest) {
			sourceAttemptID := "source-attempt"
			req.AdaptiveGate.Evidence.SourceAttemptID = &sourceAttemptID
		}},
		{name: "source lineage drift", mutate: func(req *FinalizeRunSuccessRequest) {
			sourceAttemptID := "source-attempt"
			sourceCheckpointID := int64(7999)
			req.AdaptiveGate.Decision.SourceAttemptID = &sourceAttemptID
			req.AdaptiveGate.Decision.SourceCheckpointID = &sourceCheckpointID
		}},
		{name: "decision sequence after evidence", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.Decision.EventSequence = req.AdaptiveGate.Evidence.EventSequence + 1
		}},
		{name: "decision id blank", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.DecisionID = " "
		}},
		{name: "decision id too long", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.DecisionID = strings.Repeat("d", 192)
		}},
		{name: "decision id surrounding whitespace", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.DecisionID = " decision-1 "
		}},
		{name: "decision revision", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.DecisionRevision = 0
		}},
		{name: "verification key blank", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationIdempotencyKey = " "
		}},
		{name: "verification key too long", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationIdempotencyKey = strings.Repeat("v", 192)
		}},
		{name: "verification key surrounding whitespace", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationIdempotencyKey = " verification-1 "
		}},
		{name: "verification and completion key collide", mutate: func(req *FinalizeRunSuccessRequest) {
			req.JournalEvent.IdempotencyKey = req.AdaptiveGate.VerificationIdempotencyKey
		}},
		{name: "caller spoofs nullable outbox fingerprint", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["outbox_fingerprint"] = nil },
			)
		}},
		{name: "caller spoofs finalize request fingerprint", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["finalize_request_fingerprint"] = strings.Repeat("f", 64) },
			)
		}},
		{name: "caller payload missing required field", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { delete(fields, "attempt_id") },
			)
		}},
		{name: "caller raw payload exceeds limit", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload += strings.Repeat(" ", adaptiveVerifiedSuccessMaxPayloadBytes)
		}},
		{name: "verification event missing", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent = nil
		}},
		{name: "verification event id", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.ID = 0
		}},
		{name: "verification thread identity", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.ThreadID++
		}},
		{name: "verification run identity", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.RunID++
		}},
		{name: "verification event type", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.EventType = "adaptive.progress"
		}},
		{name: "verification created at", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.CreatedAt++
		}},
		{name: "verification status", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["status"] = "blocked" },
			)
		}},
		{name: "verification payload schema", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["schema"] = "workbench-adaptive-verification.v0" },
			)
		}},
		{name: "verification payload id", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["verification_id"] = "verification-2" },
			)
		}},
		{name: "verification payload generation", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["execution_generation"] = float64(4) },
			)
		}},
		{name: "verification payload decision id", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["decision_id"] = "decision-2" },
			)
		}},
		{name: "verification payload decision revision", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["decision_revision"] = float64(2) },
			)
		}},
		{name: "verification payload plan revision", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["expected_plan_revision"] = float64(4) },
			)
		}},
		{name: "verification payload plan fingerprint", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["expected_plan_fingerprint"] = strings.Repeat("C", 64) },
			)
		}},
		{name: "verification payload evidence head", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["evidence_head_event_id"] = float64(7001) },
			)
		}},
		{name: "verification payload created at", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["created_at"] = float64(999) },
			)
		}},
		{name: "verification payload authority drift", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["verified_checkpoint_id"] = float64(9999) },
			)
		}},
		{name: "verification does not sort before completion", mutate: func(req *FinalizeRunSuccessRequest) {
			req.AdaptiveGate.VerificationEvent.ID = req.CompletionEvent.ID
		}},
		{name: "title does not sort before verification", mutate: func(req *FinalizeRunSuccessRequest) {
			req.TitleEvent.ID = req.AdaptiveGate.VerificationEvent.ID
		}},
		{name: "completion identity", mutate: func(req *FinalizeRunSuccessRequest) {
			req.CompletionEvent.ThreadID++
		}},
		{name: "completion type", mutate: func(req *FinalizeRunSuccessRequest) {
			req.CompletionEvent.EventType = "run.failed"
		}},
		{name: "journal event missing", mutate: func(req *FinalizeRunSuccessRequest) {
			req.JournalEvent = nil
		}},
		{name: "journal terminal invalid", mutate: func(req *FinalizeRunSuccessRequest) {
			req.JournalEvent.Status = string(entity.RunAttemptStatusFailed)
		}},
		{name: "journal timestamp normalization overflows", mutate: func(req *FinalizeRunSuccessRequest) {
			req.Now = math.MaxInt64/1_000_000 + 1
			req.AdaptiveGate.VerificationEvent.CreatedAt = req.Now
			req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
				t,
				req.AdaptiveGate.VerificationEvent.Payload,
				func(fields map[string]any) { fields["created_at"] = req.Now },
			)
		}},
		{name: "terminal checkpoint missing", mutate: func(req *FinalizeRunSuccessRequest) {
			req.TerminalCheckpoint = nil
		}},
		{name: "terminal checkpoint parent", mutate: func(req *FinalizeRunSuccessRequest) {
			req.TerminalCheckpoint.ParentCheckpointID++
		}},
		{name: "terminal checkpoint runtime deleted", mutate: func(req *FinalizeRunSuccessRequest) {
			req.TerminalCheckpoint.RuntimeDeletedAt = req.Now
		}},
		{name: "terminal checkpoint runtime type", mutate: func(req *FinalizeRunSuccessRequest) {
			req.TerminalCheckpoint.RuntimeType = "legacy"
		}},
		{name: "terminal fallback runtime identity", mutate: func(req *FinalizeRunSuccessRequest) {
			req.TerminalCheckpointOnTitleConflict.RuntimeKey += ":drift"
		}},
		{name: "outbox callback with result required", mutate: func(req *FinalizeRunSuccessRequest) {
			req.OutboxIntent = &NotificationOutboxIntent{}
		}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			req := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
			test.mutate(&req)
			require.ErrorIs(
				t,
				validateAdaptiveVerifiedSuccessGate(req),
				ErrAdaptiveExecutionVerifiedSuccessInvalid,
			)
		})
	}

	for _, exhausted := range []uint64{math.MaxUint64 - 2, math.MaxUint64 - 1, math.MaxUint64} {
		t.Run(fmt.Sprintf("evidence sequence exhaustion %d", exhausted), func(t *testing.T) {
			req := cloneAdaptiveVerifiedSuccessRequestForTest(valid)
			req.AdaptiveGate.Evidence.EventSequence = exhausted
			require.ErrorIs(
				t,
				validateAdaptiveVerifiedSuccessGate(req),
				ErrAdaptiveExecutionVerifiedSuccessInvalid,
			)
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsPassedVerificationOutsideFinalizerWithoutWrites(t *testing.T) {
	repo, _ := canonicalMySQLMockRepository(t)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Event.EventType = "adaptive.verification"
	req.Event.Payload = newValidAdaptiveVerifiedSuccessRequestForTest().AdaptiveGate.VerificationEvent.Payload

	result, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionBoundaryInvalid)
}

func TestThreadRepositoryFinalizeRunSuccessNilAdaptiveGateDoesNotRequireAdaptiveTables(t *testing.T) {
	t.Run("WithoutJournalEventDoesNotRequireAdaptiveTables", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&threadPO{}, &runPO{}, &messagePO{}, &runEventPO{}))
		repo := NewThreadRepository(db)
		lease := seedNilAdaptiveGateFinalizeRunForTest(t, repo)
		req := nilAdaptiveGateFinalizeRequestForTest(lease, nil)

		result, finalizeErr := repo.FinalizeRunSuccess(context.Background(), req)

		require.NoError(t, finalizeErr)
		require.Nil(t, result.VerificationEvent)
		require.False(t, result.Replayed)
		require.Equal(t, entity.RunStatusSucceeded, result.Run.Status)
		var messageCount, eventCount int64
		require.NoError(t, db.Model(&messagePO{}).Count(&messageCount).Error)
		require.NoError(t, db.Model(&runEventPO{}).Count(&eventCount).Error)
		require.Equal(t, int64(1), messageCount)
		require.Equal(t, int64(1), eventCount)
	})

	t.Run("WithJournalEventPreservesLegacyAttemptProjection", func(t *testing.T) {
		db := newJournalRepositoryTestDB(t)
		require.NoError(t, db.AutoMigrate(&messagePO{}))
		repo := NewThreadRepository(db)
		lease := seedNilAdaptiveGateFinalizeRunForTest(t, repo)
		activeSlot := uint8(1)
		startedAt := int64(1_000)
		require.NoError(t, db.Create(&runAttemptPO{
			ID: 100, ThreadID: 10, JournalRunID: 1, ExecutionRunID: 1,
			AttemptID: "attempt-1", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
			ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
			EnrollmentVersion: entity.JournalSchemaVersion,
			ProjectionState:   string(entity.JournalProjectionStateHealthy),
			CreatedAt:         1_000, UpdatedAt: 1_000, StartedAt: &startedAt,
		}).Error)
		journal := &entity.JournalEvent{
			ID: 302, ThreadID: 10, RunID: 1, JournalRunID: 1, AttemptID: "attempt-1",
			IdempotencyKey: "completion-1", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Visibility: entity.JournalVisibilityUser,
			Payload: `{"type":"terminal","data":{"status":"completed"}}`, CreatedAt: 2_000,
		}
		req := nilAdaptiveGateFinalizeRequestForTest(lease, journal)

		result, finalizeErr := repo.FinalizeRunSuccess(context.Background(), req)

		require.NoError(t, finalizeErr)
		require.Nil(t, result.VerificationEvent)
		require.False(t, result.Replayed)
		var attempt runAttemptPO
		require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
		require.Equal(t, string(entity.RunAttemptStatusCompleted), attempt.Status)
		require.Nil(t, attempt.ActiveSlot)
		require.Equal(t, uint64(2), attempt.NextSequence)
		require.Equal(t, int64(302), requireInt64PointerForTest(t, attempt.TerminalEventID))
	})
}

func newValidAdaptiveVerifiedSuccessRequestForTest() FinalizeRunSuccessRequest {
	decision := AdaptiveExecutionBoundaryAuthority{
		ThreadID: 10, ExecutionRunID: 20, ExecutionGeneration: 3,
		JournalRunID: 30, AttemptID: "attempt-1",
		EventID: 7001, EventSequence: 1, IdempotencyKey: "decision-boundary-1",
		CheckpointID: 8001, PlanScopeRunID: 20, PlanRevision: 2,
		PlanItemFingerprint: strings.Repeat("a", 64),
	}
	evidence := decision
	evidence.EventID = 7002
	evidence.EventSequence = 2
	evidence.IdempotencyKey = "evidence-boundary-1"
	evidence.CheckpointID = 8002
	evidence.PlanRevision = 3
	evidence.PlanItemFingerprint = strings.Repeat("b", 64)
	payload := map[string]any{
		"schema":                    "workbench-adaptive-verification.v1",
		"verification_id":           "verification-1",
		"execution_run_id":          int64(20),
		"journal_run_id":            int64(30),
		"attempt_id":                "attempt-1",
		"execution_generation":      uint64(3),
		"decision_id":               "decision-1",
		"decision_revision":         int64(1),
		"expected_plan_revision":    int64(3),
		"expected_plan_fingerprint": strings.Repeat("c", 64),
		"verified_checkpoint_id":    int64(8002),
		"evidence_head_event_id":    int64(7002),
		"status":                    "passed",
		"created_at":                int64(1_000),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	primaryCheckpoint := &entity.Checkpoint{
		ID: 8003, ThreadID: 10, RunID: 20, ParentCheckpointID: 8002,
		CheckpointNS: "adaptive", RuntimeType: "eino_adk", RuntimeKey: "runtime-key",
		EnvelopeVersion: 2, ChannelValues: `{}`, ChannelVersions: `{}`,
		PendingSends: `[]`, Metadata: `{}`, CreatedAt: 1_000,
	}
	fallbackCheckpoint := *primaryCheckpoint
	fallbackCheckpoint.ChannelValues = `{"title":"preserved"}`
	return FinalizeRunSuccessRequest{
		RunID: 20, LeaseOwner: "worker-1", LeaseToken: "lease-1",
		ExecutionGeneration: 3, Now: 1_000,
		Message: &entity.Message{
			ID: 6001, ThreadID: 10, RunID: 20, Role: entity.MessageRoleAssistant,
			Content: "final answer", Metadata: `{}`, CreatedAt: 1_000,
		},
		TitleEvent: &entity.RunEvent{
			ID: 7003, ThreadID: 10, RunID: 20,
			EventType: "context.thread_title_updated", Payload: `{"thread_title":"generated"}`, CreatedAt: 1_000,
		},
		CompletionEvent: &entity.RunEvent{
			ID: 7005, ThreadID: 10, RunID: 20,
			EventType: "run.completed", Payload: `{"status":"succeeded"}`, CreatedAt: 1_000,
		},
		JournalEvent: &entity.JournalEvent{
			ID: 7005, ThreadID: 10, RunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
			IdempotencyKey: "completion-1", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCompleted), Visibility: entity.JournalVisibilityUser,
			Payload: `{"type":"terminal","data":{"status":"completed"}}`, CreatedAt: 1_000,
		},
		TerminalCheckpoint:                primaryCheckpoint,
		TerminalCheckpointOnTitleConflict: &fallbackCheckpoint,
		ExpectedThreadTitle:               "initial",
		ThreadTitle:                       "generated",
		AdaptiveGate: &AdaptiveVerifiedSuccessGate{
			Decision: decision, Evidence: evidence,
			DecisionID: "decision-1", DecisionRevision: 1,
			VerificationEvent: &entity.RunEvent{
				ID: 7004, ThreadID: 10, RunID: 20,
				EventType: "adaptive.verification", Payload: string(encoded), CreatedAt: 1_000,
			},
			VerificationIdempotencyKey: "verification-1",
		},
	}
}

func cloneAdaptiveVerifiedSuccessRequestForTest(req FinalizeRunSuccessRequest) FinalizeRunSuccessRequest {
	clone := req
	if req.Message != nil {
		message := *req.Message
		clone.Message = &message
	}
	if req.TitleEvent != nil {
		title := *req.TitleEvent
		clone.TitleEvent = &title
	}
	if req.CompletionEvent != nil {
		completion := *req.CompletionEvent
		clone.CompletionEvent = &completion
	}
	if req.JournalEvent != nil {
		journal := *req.JournalEvent
		clone.JournalEvent = &journal
	}
	if req.TerminalCheckpoint != nil {
		checkpoint := *req.TerminalCheckpoint
		clone.TerminalCheckpoint = &checkpoint
	}
	if req.TerminalCheckpointOnTitleConflict != nil {
		checkpoint := *req.TerminalCheckpointOnTitleConflict
		clone.TerminalCheckpointOnTitleConflict = &checkpoint
	}
	if req.AdaptiveGate != nil {
		gate := *req.AdaptiveGate
		gate.Decision = cloneAdaptiveExecutionAuthorityForTest(req.AdaptiveGate.Decision)
		gate.Evidence = cloneAdaptiveExecutionAuthorityForTest(req.AdaptiveGate.Evidence)
		if req.AdaptiveGate.VerificationEvent != nil {
			verification := *req.AdaptiveGate.VerificationEvent
			gate.VerificationEvent = &verification
		}
		clone.AdaptiveGate = &gate
	}
	if req.OutboxIntent != nil {
		intent := *req.OutboxIntent
		clone.OutboxIntent = &intent
	}
	return clone
}

func cloneAdaptiveExecutionAuthorityForTest(authority AdaptiveExecutionBoundaryAuthority) AdaptiveExecutionBoundaryAuthority {
	clone := authority
	if authority.SourceAttemptID != nil {
		value := *authority.SourceAttemptID
		clone.SourceAttemptID = &value
	}
	if authority.SourceCheckpointID != nil {
		value := *authority.SourceCheckpointID
		clone.SourceCheckpointID = &value
	}
	return clone
}

func adaptiveVerifiedSuccessPayloadMutationForTest(
	t *testing.T,
	payload string,
	mutate func(map[string]any),
) string {
	t.Helper()
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(payload), &fields))
	require.NotNil(t, fields)
	mutate(fields)
	encoded, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(encoded)
}

func adaptiveVerifiedSuccessDurablePayloadForTest(
	t *testing.T,
	caller string,
	outboxFingerprint any,
	finalizeRequestFingerprint any,
) string {
	t.Helper()
	return adaptiveVerifiedSuccessPayloadMutationForTest(t, caller, func(fields map[string]any) {
		fields["outbox_fingerprint"] = outboxFingerprint
		fields["finalize_request_fingerprint"] = finalizeRequestFingerprint
	})
}

func adaptiveVerifiedSuccessCallerPayloadAtDurableSizeForTest(
	t *testing.T,
	caller string,
	target int,
	withOutbox bool,
) string {
	t.Helper()
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(caller), &fields))
	fields["padding"] = ""
	serverFields := make(map[string]any, len(fields)+2)
	for key, value := range fields {
		serverFields[key] = value
	}
	serverFields["outbox_fingerprint"] = nil
	if withOutbox {
		serverFields["outbox_fingerprint"] = strings.Repeat("0", 64)
	}
	serverFields["finalize_request_fingerprint"] = strings.Repeat("0", 64)
	encoded, err := json.Marshal(serverFields)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), target)
	fields["padding"] = strings.Repeat("x", target-len(encoded))
	serverFields["padding"] = fields["padding"]
	encoded, err = json.Marshal(serverFields)
	require.NoError(t, err)
	require.Len(t, encoded, target)
	callerEncoded, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(callerEncoded)
}

func seedNilAdaptiveGateFinalizeRunForTest(t *testing.T, repo ThreadRepository) *entity.Run {
	t.Helper()
	require.NoError(t, repo.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 1, CreatorID: 2, Title: "initial",
		Status: entity.ThreadStatusRunning, Source: entity.ThreadSourceWeb,
		CreatedAt: 100, UpdatedAt: 100, LastMessageAt: 100,
	}))
	require.NoError(t, repo.CreateRun(context.Background(), newRepositoryTestRun(
		1,
		10,
		entity.RunStatusPending,
		100,
	)))
	claimed, err := repo.ClaimPendingRuns(context.Background(), ClaimPendingRunsRequest{
		WorkerID: "worker-a", Limit: 1, Now: 1_000, LeaseTTLMillis: 5_000,
	})
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	return claimed[0]
}

func nilAdaptiveGateFinalizeRequestForTest(
	lease *entity.Run,
	journal *entity.JournalEvent,
) FinalizeRunSuccessRequest {
	return FinalizeRunSuccessRequest{
		RunID: 1, LeaseOwner: lease.LeaseOwner, LeaseToken: lease.LeaseToken,
		ExecutionGeneration: lease.ExecutionGeneration, Now: 2_000,
		Message: &entity.Message{
			ID: 300, ThreadID: 10, RunID: 1, Role: entity.MessageRoleAssistant,
			Content: "final answer", Metadata: `{}`, CreatedAt: 2_000,
		},
		CompletionEvent: &entity.RunEvent{
			ID: 302, ThreadID: 10, RunID: 1,
			EventType: "run.completed", Payload: `{}`, CreatedAt: 2_000,
		},
		JournalEvent: journal,
	}
}

type adaptiveVerifiedSuccessFixtureForTest struct {
	DB       *gorm.DB
	Repo     *threadRepository
	Request  FinalizeRunSuccessRequest
	Decision *CommitAdaptiveExecutionBoundaryResult
	Evidence *CommitAdaptiveExecutionBoundaryResult
}

type adaptiveVerifiedSuccessOutboxPOForTest struct {
	EventID     string `gorm:"column:event_id;primaryKey"`
	Fingerprint string `gorm:"column:fingerprint"`
}

func (adaptiveVerifiedSuccessOutboxPOForTest) TableName() string {
	return "adaptive_verified_success_outbox_test"
}

type adaptiveVerifiedSuccessDBSnapshotForTest struct {
	Threads     []threadPO
	Runs        []runPO
	Attempts    []runAttemptPO
	Messages    []messagePO
	Events      []runEventPO
	Checkpoints []checkpointPO
	Plans       []agentRunPlanPO
	Items       []agentRunPlanItemPO
	Outbox      []adaptiveVerifiedSuccessOutboxPOForTest
}

func snapshotAdaptiveVerifiedSuccessDBForTest(
	t *testing.T,
	db *gorm.DB,
) adaptiveVerifiedSuccessDBSnapshotForTest {
	t.Helper()
	var snapshot adaptiveVerifiedSuccessDBSnapshotForTest
	require.NoError(t, db.Order("id ASC").Find(&snapshot.Threads).Error)
	require.NoError(t, db.Order("id ASC").Find(&snapshot.Runs).Error)
	require.NoError(t, db.Order("id ASC").Find(&snapshot.Attempts).Error)
	require.NoError(t, db.Order("id ASC").Find(&snapshot.Messages).Error)
	require.NoError(t, db.Order("id ASC").Find(&snapshot.Events).Error)
	require.NoError(t, db.Order("id ASC").Find(&snapshot.Checkpoints).Error)
	require.NoError(t, db.Order("run_id ASC").Find(&snapshot.Plans).Error)
	require.NoError(t, db.Order("run_id ASC, task_id ASC").Find(&snapshot.Items).Error)
	require.NoError(t, db.Order("event_id ASC").Find(&snapshot.Outbox).Error)
	return snapshot
}

type adaptiveVerifiedSuccessOptionalJSONOracleForTest struct {
	SQLNull bool            `json:"sql_null"`
	Value   json.RawMessage `json:"value"`
}

type adaptiveVerifiedSuccessRunOracleForTest struct {
	ID                  int64                                            `json:"id"`
	ThreadID            int64                                            `json:"thread_id"`
	ParentRunID         int64                                            `json:"parent_run_id"`
	SpaceID             int64                                            `json:"space_id"`
	CreatorID           int64                                            `json:"creator_id"`
	AssistantID         string                                           `json:"assistant_id"`
	RunKind             string                                           `json:"run_kind"`
	Status              string                                           `json:"status"`
	Command             adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"command"`
	Input               adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"input"`
	Config              adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"config"`
	Context             adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"context"`
	Metadata            adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"metadata"`
	StreamMode          adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"stream_mode"`
	MultitaskStrategy   string                                           `json:"multitask_strategy"`
	OnDisconnect        string                                           `json:"on_disconnect"`
	Durability          string                                           `json:"durability"`
	IdempotencyKey      *string                                          `json:"idempotency_key"`
	WorkerID            string                                           `json:"worker_id"`
	LeaseOwner          *string                                          `json:"lease_owner"`
	LeaseToken          *string                                          `json:"lease_token"`
	LeaseExpiresAt      *int64                                           `json:"lease_expires_at"`
	HeartbeatAt         *int64                                           `json:"heartbeat_at"`
	CancelRequestedAt   *int64                                           `json:"cancel_requested_at"`
	ExecutionGeneration uint64                                           `json:"execution_generation"`
	ErrorCode           string                                           `json:"error_code"`
	ErrorMessage        string                                           `json:"error_message"`
	StartedAt           int64                                            `json:"started_at"`
	EndedAt             int64                                            `json:"ended_at"`
	CreatedAt           int64                                            `json:"created_at"`
	UpdatedAt           int64                                            `json:"updated_at"`
}

type adaptiveVerifiedSuccessAttemptOracleForTest struct {
	ID                     int64   `json:"id"`
	ThreadID               int64   `json:"thread_id"`
	JournalRunID           int64   `json:"journal_run_id"`
	ExecutionRunID         int64   `json:"execution_run_id"`
	AttemptID              string  `json:"attempt_id"`
	Ordinal                uint32  `json:"ordinal"`
	Status                 string  `json:"status"`
	ActiveSlot             *uint8  `json:"active_slot"`
	NextSequence           uint64  `json:"next_sequence"`
	LastCommittedSequence  uint64  `json:"last_committed_sequence"`
	SourceCheckpointID     *int64  `json:"source_checkpoint_id"`
	SourceAttemptID        *string `json:"source_attempt_id"`
	RecoveryIdempotencyKey *string `json:"recovery_idempotency_key"`
	EnrollmentVersion      string  `json:"enrollment_version"`
	SnapshotsEnabled       bool    `json:"snapshots_enabled"`
	ProjectionState        string  `json:"projection_state"`
	ProjectionDegradedAt   *int64  `json:"projection_degraded_at"`
	TraceID                *string `json:"trace_id"`
	TerminalEventID        *int64  `json:"terminal_event_id"`
	CreatedAt              int64   `json:"created_at"`
	UpdatedAt              int64   `json:"updated_at"`
	StartedAt              *int64  `json:"started_at"`
	EndedAt                *int64  `json:"ended_at"`
}

type adaptiveVerifiedSuccessEventOracleForTest struct {
	ID                 int64                                            `json:"id"`
	ThreadID           int64                                            `json:"thread_id"`
	RunID              int64                                            `json:"run_id"`
	JournalRunID       *int64                                           `json:"journal_run_id"`
	AttemptID          *string                                          `json:"attempt_id"`
	Sequence           *uint64                                          `json:"sequence"`
	IdempotencyKey     *string                                          `json:"idempotency_key"`
	ParentEventID      *int64                                           `json:"parent_event_id"`
	SchemaVersion      *string                                          `json:"schema_version"`
	Status             *string                                          `json:"status"`
	OccurredAtUnixNano *int64                                           `json:"occurred_at_unix_nano"`
	Visibility         *string                                          `json:"visibility"`
	PayloadVersion     *string                                          `json:"payload_version"`
	SnapshotID         *string                                          `json:"snapshot_id"`
	TraceID            *string                                          `json:"trace_id"`
	ActionID           *string                                          `json:"action_id"`
	Phase              *string                                          `json:"phase"`
	Operation          *string                                          `json:"operation"`
	Target             *string                                          `json:"target"`
	Milestone          *string                                          `json:"milestone"`
	EventType          string                                           `json:"event_type"`
	JournalEventType   *string                                          `json:"journal_event_type"`
	Payload            json.RawMessage                                  `json:"payload"`
	JournalPayload     adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"journal_payload"`
	CreatedAt          int64                                            `json:"created_at"`
}

type adaptiveVerifiedSuccessAuthorityOracleForTest struct {
	ThreadID            int64   `json:"thread_id"`
	ExecutionRunID      int64   `json:"execution_run_id"`
	ExecutionGeneration uint64  `json:"execution_generation"`
	JournalRunID        int64   `json:"journal_run_id"`
	AttemptID           string  `json:"attempt_id"`
	SourceAttemptID     *string `json:"source_attempt_id"`
	SourceCheckpointID  *int64  `json:"source_checkpoint_id"`
	EventID             int64   `json:"event_id"`
	EventSequence       uint64  `json:"event_sequence"`
	IdempotencyKey      string  `json:"idempotency_key"`
	CheckpointID        int64   `json:"checkpoint_id"`
	PlanScopeRunID      int64   `json:"plan_scope_run_id"`
	PlanRevision        int64   `json:"plan_revision"`
	PlanItemFingerprint string  `json:"plan_item_fingerprint"`
}

type adaptiveVerifiedSuccessMessageOracleForTest struct {
	ID        int64                                            `json:"id"`
	ThreadID  int64                                            `json:"thread_id"`
	RunID     int64                                            `json:"run_id"`
	Role      string                                           `json:"role"`
	Content   string                                           `json:"content"`
	Metadata  adaptiveVerifiedSuccessOptionalJSONOracleForTest `json:"metadata"`
	CreatedAt int64                                            `json:"created_at"`
}

type adaptiveVerifiedSuccessCheckpointOracleForTest struct {
	ID                 int64           `json:"id"`
	ThreadID           int64           `json:"thread_id"`
	RunID              int64           `json:"run_id"`
	ParentCheckpointID int64           `json:"parent_checkpoint_id"`
	CheckpointNS       string          `json:"checkpoint_ns"`
	RuntimeType        string          `json:"runtime_type"`
	RuntimeKey         string          `json:"runtime_key"`
	EnvelopeVersion    int32           `json:"envelope_version"`
	RuntimeDeletedAt   int64           `json:"runtime_deleted_at"`
	ChannelValues      json.RawMessage `json:"channel_values"`
	ChannelVersions    json.RawMessage `json:"channel_versions"`
	PendingSends       json.RawMessage `json:"pending_sends"`
	Metadata           json.RawMessage `json:"metadata"`
	CreatedAt          int64           `json:"created_at"`
}

type adaptiveVerifiedSuccessFinalizeOracleForTest struct {
	RunID                int64                                           `json:"run_id"`
	ExecutionGeneration  uint64                                          `json:"execution_generation"`
	Now                  int64                                           `json:"now"`
	TerminalRun          adaptiveVerifiedSuccessRunOracleForTest         `json:"terminal_run"`
	TerminalAttempt      adaptiveVerifiedSuccessAttemptOracleForTest     `json:"terminal_attempt"`
	Decision             adaptiveVerifiedSuccessAuthorityOracleForTest   `json:"decision"`
	Evidence             adaptiveVerifiedSuccessAuthorityOracleForTest   `json:"evidence"`
	DecisionID           string                                          `json:"decision_id"`
	DecisionRevision     int64                                           `json:"decision_revision"`
	Verification         adaptiveVerifiedSuccessEventOracleForTest       `json:"verification"`
	Completion           adaptiveVerifiedSuccessEventOracleForTest       `json:"completion"`
	Message              adaptiveVerifiedSuccessMessageOracleForTest     `json:"message"`
	TitleEvent           *adaptiveVerifiedSuccessEventOracleForTest      `json:"title_event"`
	ExpectedThreadTitle  string                                          `json:"expected_thread_title"`
	ThreadTitle          string                                          `json:"thread_title"`
	TitleUpdated         bool                                            `json:"title_updated"`
	CommittedThreadTitle string                                          `json:"committed_thread_title"`
	PrimaryCheckpoint    adaptiveVerifiedSuccessCheckpointOracleForTest  `json:"primary_checkpoint"`
	FallbackCheckpoint   *adaptiveVerifiedSuccessCheckpointOracleForTest `json:"fallback_checkpoint"`
	SelectedCheckpoint   adaptiveVerifiedSuccessCheckpointOracleForTest  `json:"selected_checkpoint"`
	OutboxFingerprint    *string                                         `json:"outbox_fingerprint"`
}

type adaptiveVerifiedSuccessOutboxOracleForTest struct {
	EventID          string                          `json:"event_id"`
	EventType        string                          `json:"event_type"`
	AggregateType    string                          `json:"aggregate_type"`
	AggregateID      string                          `json:"aggregate_id"`
	AggregateVersion int64                           `json:"aggregate_version"`
	OccurredAt       int64                           `json:"occurred_at"`
	ActorID          int64                           `json:"actor_id"`
	SpaceID          int64                           `json:"space_id"`
	RecipientPolicy  string                          `json:"recipient_policy"`
	PayloadSchema    int32                           `json:"payload_schema"`
	Payload          domainnotification.EventPayload `json:"payload"`
}

func adaptiveVerifiedSuccessCanonicalJSONOracleForTest(
	t *testing.T,
	raw []byte,
) json.RawMessage {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(string(raw))))
	decoder.UseNumber()
	var value any
	require.NoError(t, decoder.Decode(&value))
	var trailing any
	require.ErrorIs(t, decoder.Decode(&trailing), io.EOF)
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(
	t *testing.T,
	raw []byte,
) adaptiveVerifiedSuccessOptionalJSONOracleForTest {
	t.Helper()
	if len(raw) == 0 {
		return adaptiveVerifiedSuccessOptionalJSONOracleForTest{SQLNull: true}
	}
	return adaptiveVerifiedSuccessOptionalJSONOracleForTest{
		Value: adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, raw),
	}
}

func adaptiveVerifiedSuccessSHA256OracleForTest(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("%x", digest[:])
}

func adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(
	t *testing.T,
	event domainnotification.Event,
) string {
	t.Helper()
	canonical, err := domainnotification.CanonicalizeEvent(event)
	require.NoError(t, err)
	return adaptiveVerifiedSuccessSHA256OracleForTest(t, adaptiveVerifiedSuccessOutboxOracleForTest{
		EventID: canonical.EventID, EventType: string(canonical.EventType),
		AggregateType: canonical.AggregateType, AggregateID: canonical.AggregateID,
		AggregateVersion: canonical.AggregateVersion, OccurredAt: canonical.OccurredAt.UnixMilli(),
		ActorID: canonical.ActorID, SpaceID: canonical.SpaceID,
		RecipientPolicy: string(canonical.RecipientPolicy), PayloadSchema: canonical.PayloadSchema,
		Payload: canonical.Payload,
	})
}

func adaptiveVerifiedSuccessOutboxEventForTest(now int64) domainnotification.Event {
	return domainnotification.Event{
		EventID: "adaptive-success-outbox-1", EventType: domainnotification.EventTaskCompleted,
		AggregateType: "agent_run", AggregateID: "20", AggregateVersion: 3,
		OccurredAt: time.UnixMilli(now), ActorID: 20, SpaceID: 10,
		RecipientPolicy: domainnotification.RecipientActor,
		PayloadSchema:   domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName: "Adaptive run", ActorDisplayName: "Agent",
			TargetID: "20",
		},
	}
}

func buildAdaptiveVerifiedSuccessAuthorityOracleForTest(
	authority AdaptiveExecutionBoundaryAuthority,
) adaptiveVerifiedSuccessAuthorityOracleForTest {
	return adaptiveVerifiedSuccessAuthorityOracleForTest{
		ThreadID: authority.ThreadID, ExecutionRunID: authority.ExecutionRunID,
		ExecutionGeneration: authority.ExecutionGeneration, JournalRunID: authority.JournalRunID,
		AttemptID: authority.AttemptID, SourceAttemptID: authority.SourceAttemptID,
		SourceCheckpointID: authority.SourceCheckpointID, EventID: authority.EventID,
		EventSequence: authority.EventSequence, IdempotencyKey: authority.IdempotencyKey,
		CheckpointID: authority.CheckpointID, PlanScopeRunID: authority.PlanScopeRunID,
		PlanRevision: authority.PlanRevision, PlanItemFingerprint: authority.PlanItemFingerprint,
	}
}

func buildAdaptiveVerifiedSuccessRunOracleForTest(
	t *testing.T,
	run runPO,
) adaptiveVerifiedSuccessRunOracleForTest {
	t.Helper()
	return adaptiveVerifiedSuccessRunOracleForTest{
		ID: run.ID, ThreadID: run.ThreadID, ParentRunID: run.ParentRunID,
		SpaceID: run.SpaceID, CreatorID: run.CreatorID, AssistantID: run.AssistantID,
		RunKind: run.RunKind, Status: run.Status,
		Command:           buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, run.Command),
		Input:             buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, run.Input),
		Config:            buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, run.Config),
		Context:           buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, run.Context),
		Metadata:          buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, run.Metadata),
		StreamMode:        buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, run.StreamMode),
		MultitaskStrategy: run.MultitaskStrategy, OnDisconnect: run.OnDisconnect,
		Durability: run.Durability, IdempotencyKey: run.IdempotencyKey,
		WorkerID: run.WorkerID, LeaseOwner: run.LeaseOwner, LeaseToken: run.LeaseToken,
		LeaseExpiresAt: run.LeaseExpiresAt, HeartbeatAt: run.HeartbeatAt,
		CancelRequestedAt: run.CancelRequestedAt, ExecutionGeneration: run.ExecutionGeneration,
		ErrorCode: run.ErrorCode, ErrorMessage: run.ErrorMessage, StartedAt: run.StartedAt,
		EndedAt: run.EndedAt, CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
	}
}

func buildAdaptiveVerifiedSuccessAttemptOracleForTest(
	attempt runAttemptPO,
) adaptiveVerifiedSuccessAttemptOracleForTest {
	return adaptiveVerifiedSuccessAttemptOracleForTest{
		ID: attempt.ID, ThreadID: attempt.ThreadID, JournalRunID: attempt.JournalRunID,
		ExecutionRunID: attempt.ExecutionRunID, AttemptID: attempt.AttemptID,
		Ordinal: attempt.Ordinal, Status: attempt.Status, ActiveSlot: attempt.ActiveSlot,
		NextSequence: attempt.NextSequence, LastCommittedSequence: attempt.LastCommittedSequence,
		SourceCheckpointID: attempt.SourceCheckpointID, SourceAttemptID: attempt.SourceAttemptID,
		RecoveryIdempotencyKey: attempt.RecoveryIdempotencyKey,
		EnrollmentVersion:      attempt.EnrollmentVersion, SnapshotsEnabled: attempt.SnapshotsEnabled,
		ProjectionState: attempt.ProjectionState, ProjectionDegradedAt: attempt.ProjectionDegradedAt,
		TraceID: attempt.TraceID, TerminalEventID: attempt.TerminalEventID,
		CreatedAt: attempt.CreatedAt, UpdatedAt: attempt.UpdatedAt,
		StartedAt: attempt.StartedAt, EndedAt: attempt.EndedAt,
	}
}

func buildAdaptiveVerifiedSuccessCheckpointOracleForTest(
	t *testing.T,
	checkpoint *entity.Checkpoint,
) adaptiveVerifiedSuccessCheckpointOracleForTest {
	t.Helper()
	require.NotNil(t, checkpoint)
	return adaptiveVerifiedSuccessCheckpointOracleForTest{
		ID: checkpoint.ID, ThreadID: checkpoint.ThreadID, RunID: checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID, CheckpointNS: checkpoint.CheckpointNS,
		RuntimeType: checkpoint.RuntimeType, RuntimeKey: checkpoint.RuntimeKey,
		EnvelopeVersion: checkpoint.EnvelopeVersion, RuntimeDeletedAt: checkpoint.RuntimeDeletedAt,
		ChannelValues:   adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, []byte(checkpoint.ChannelValues)),
		ChannelVersions: adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, []byte(checkpoint.ChannelVersions)),
		PendingSends:    adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, []byte(checkpoint.PendingSends)),
		Metadata:        adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, []byte(checkpoint.Metadata)),
		CreatedAt:       checkpoint.CreatedAt,
	}
}

func adaptiveVerifiedSuccessBaseEventOracleForTest(
	t *testing.T,
	event *entity.RunEvent,
) adaptiveVerifiedSuccessEventOracleForTest {
	t.Helper()
	require.NotNil(t, event)
	return adaptiveVerifiedSuccessEventOracleForTest{
		ID: event.ID, ThreadID: event.ThreadID, RunID: event.RunID,
		EventType:      event.EventType,
		Payload:        adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, []byte(event.Payload)),
		JournalPayload: buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, nil),
		CreatedAt:      event.CreatedAt,
	}
}

func adaptiveVerifiedSuccessFinalizeFingerprintOracleForTest(
	t *testing.T,
	fixture adaptiveVerifiedSuccessFixtureForTest,
	preRun runPO,
	preAttempt runAttemptPO,
	outboxFingerprint *string,
	healthyCompletionProjection bool,
) string {
	t.Helper()
	req := fixture.Request
	terminalRun := preRun
	terminalRun.Status = string(entity.RunStatusSucceeded)
	terminalRun.ErrorCode = ""
	terminalRun.ErrorMessage = ""
	terminalRun.EndedAt = req.Now
	terminalRun.UpdatedAt = req.Now
	terminalRun.WorkerID = ""
	terminalRun.LeaseOwner = nil
	terminalRun.LeaseToken = nil
	terminalRun.LeaseExpiresAt = nil
	terminalRun.HeartbeatAt = nil
	terminalRun.CancelRequestedAt = nil

	verificationSequence := fixture.Evidence.Authority.EventSequence + 1
	completionSequence := verificationSequence + 1
	terminalAttempt := preAttempt
	terminalAttempt.Status = string(entity.RunAttemptStatusCompleted)
	terminalAttempt.ActiveSlot = nil
	terminalAttempt.NextSequence = completionSequence + 1
	terminalAttempt.LastCommittedSequence = verificationSequence
	terminalAttempt.TerminalEventID = adaptiveExecutionInt64Pointer(req.CompletionEvent.ID)
	terminalAttempt.EndedAt = adaptiveExecutionInt64Pointer(req.Now)
	terminalAttempt.UpdatedAt = req.Now

	verificationPayload := adaptiveVerifiedSuccessPayloadMutationForTest(
		t,
		req.AdaptiveGate.VerificationEvent.Payload,
		func(fields map[string]any) {
			fields["outbox_fingerprint"] = nil
			if outboxFingerprint != nil {
				fields["outbox_fingerprint"] = *outboxFingerprint
			}
			fields["finalize_request_fingerprint"] = ""
		},
	)
	verification := adaptiveVerifiedSuccessBaseEventOracleForTest(t, req.AdaptiveGate.VerificationEvent)
	verification.JournalRunID = adaptiveExecutionInt64Pointer(fixture.Evidence.Authority.JournalRunID)
	verification.AttemptID = adaptiveExecutionStringPointer(fixture.Evidence.Authority.AttemptID)
	verification.Sequence = adaptiveExecutionUint64Pointer(verificationSequence)
	verification.IdempotencyKey = adaptiveExecutionStringPointer(req.AdaptiveGate.VerificationIdempotencyKey)
	metadata := decodeAdaptiveExecutionMetadataForTest(t, []byte(fixture.Evidence.Checkpoint.Metadata))
	verification.SnapshotID = adaptiveExecutionStringPointer(metadata.CheckpointFingerprint)
	verification.Payload = adaptiveVerifiedSuccessCanonicalJSONOracleForTest(t, []byte(verificationPayload))

	completion := adaptiveVerifiedSuccessBaseEventOracleForTest(t, req.CompletionEvent)
	completion.JournalRunID = adaptiveExecutionInt64Pointer(fixture.Evidence.Authority.JournalRunID)
	completion.AttemptID = adaptiveExecutionStringPointer(fixture.Evidence.Authority.AttemptID)
	completion.Sequence = adaptiveExecutionUint64Pointer(completionSequence)
	completion.IdempotencyKey = adaptiveExecutionStringPointer(
		strings.TrimSpace(req.JournalEvent.IdempotencyKey),
	)
	if healthyCompletionProjection {
		completion.SchemaVersion = adaptiveExecutionStringPointer(entity.JournalSchemaVersion)
		completion.Status = adaptiveExecutionStringPointer(string(entity.RunAttemptStatusCompleted))
		completion.OccurredAtUnixNano = adaptiveExecutionInt64Pointer(req.JournalEvent.OccurredAtUnixNano)
		completion.Visibility = adaptiveExecutionStringPointer(string(entity.JournalVisibilityUser))
		completion.PayloadVersion = adaptiveExecutionStringPointer(entity.JournalPayloadVersion)
		completion.JournalEventType = adaptiveExecutionStringPointer("run.lifecycle")
		completion.JournalPayload = buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(
			t,
			[]byte(req.JournalEvent.Payload),
		)
	}

	message := req.Message
	title := adaptiveVerifiedSuccessBaseEventOracleForTest(t, req.TitleEvent)
	primary := buildAdaptiveVerifiedSuccessCheckpointOracleForTest(t, req.TerminalCheckpoint)
	fallback := buildAdaptiveVerifiedSuccessCheckpointOracleForTest(t, req.TerminalCheckpointOnTitleConflict)
	return adaptiveVerifiedSuccessSHA256OracleForTest(t, adaptiveVerifiedSuccessFinalizeOracleForTest{
		RunID: req.RunID, ExecutionGeneration: req.ExecutionGeneration, Now: req.Now,
		TerminalRun:     buildAdaptiveVerifiedSuccessRunOracleForTest(t, terminalRun),
		TerminalAttempt: buildAdaptiveVerifiedSuccessAttemptOracleForTest(terminalAttempt),
		Decision:        buildAdaptiveVerifiedSuccessAuthorityOracleForTest(req.AdaptiveGate.Decision),
		Evidence:        buildAdaptiveVerifiedSuccessAuthorityOracleForTest(req.AdaptiveGate.Evidence),
		DecisionID:      req.AdaptiveGate.DecisionID, DecisionRevision: req.AdaptiveGate.DecisionRevision,
		Verification: verification, Completion: completion,
		Message: adaptiveVerifiedSuccessMessageOracleForTest{
			ID: message.ID, ThreadID: message.ThreadID, RunID: message.RunID,
			Role: string(message.Role), Content: message.Content,
			Metadata:  buildAdaptiveVerifiedSuccessOptionalJSONOracleForTest(t, []byte(message.Metadata)),
			CreatedAt: message.CreatedAt,
		},
		TitleEvent: &title, ExpectedThreadTitle: strings.TrimSpace(req.ExpectedThreadTitle),
		ThreadTitle: strings.TrimSpace(req.ThreadTitle), TitleUpdated: true,
		CommittedThreadTitle: strings.TrimSpace(req.ThreadTitle),
		PrimaryCheckpoint:    primary, FallbackCheckpoint: &fallback,
		SelectedCheckpoint: primary, OutboxFingerprint: outboxFingerprint,
	})
}

func prepareAdaptiveVerifiedSuccessFixtureForTest(t *testing.T) adaptiveVerifiedSuccessFixtureForTest {
	return prepareAdaptiveVerifiedSuccessFixtureWithOptionsForTest(t, false)
}

func prepareAdaptiveVerifiedSuccessFixtureWithNewerDecisionForTest(
	t *testing.T,
) adaptiveVerifiedSuccessFixtureForTest {
	return prepareAdaptiveVerifiedSuccessFixtureWithOptionsForTest(t, true)
}

func prepareAdaptiveVerifiedSuccessFixtureWithOptionsForTest(
	t *testing.T,
	withNewerDecision bool,
) adaptiveVerifiedSuccessFixtureForTest {
	t.Helper()
	db := newAdaptiveExecutionRepositoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&messagePO{}, &adaptiveVerifiedSuccessOutboxPOForTest{}))
	seedAdaptiveExecutionInitialState(t, db)
	adaptiveRepo := NewAdaptiveExecutionRepository(db)

	decisionReq := adaptiveInitialBoundaryRequest(7001, 8001, 1_000)
	decisionReq.Event.EventType = "adaptive.decision"
	decisionReq.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-1","decision_revision":1}`
	decision, err := adaptiveRepo.CommitAdaptiveExecutionBoundary(context.Background(), decisionReq)
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, uint64(1), decision.Authority.EventSequence)
	evidenceExpectedRevision := int64(2)
	evidenceNextRevision := int64(3)
	evidenceExpectedItemVersion := int64(2)
	evidenceNextItemVersion := int64(3)
	evidenceEventSequence := uint64(2)
	if withNewerDecision {
		var currentItems []agentRunPlanItemPO
		require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&currentItems).Error)
		decision2Req := cloneAdaptiveExecutionBoundaryRequestForReplayTest(decisionReq)
		decision2Req.Now = 1_050
		decision2Req.IdempotencyKey = "decision-boundary-2"
		decision2Req.Event.ID = 7010
		decision2Req.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-2","decision_revision":2}`
		decision2Req.Event.CreatedAt = decision2Req.Now
		decision2Req.Checkpoint.ID = 8010
		decision2Req.Checkpoint.CreatedAt = decision2Req.Now
		decision2Req.PlanMutation = &AdaptivePlanMutation{
			PlanScopeRunID: 20, ExpectedRevision: 2, NextRevision: 3,
			Items: make([]AdaptivePlanItemMutation, 0, len(currentItems)),
		}
		for index := range currentItems {
			next := currentItems[index].toEntity()
			next.Subject += " after decision 2"
			next.Version++
			decision2Req.PlanMutation.Items = append(
				decision2Req.PlanMutation.Items,
				AdaptivePlanItemMutation{ExpectedVersion: currentItems[index].Version, NextItem: next},
			)
		}
		decision2, err := adaptiveRepo.CommitAdaptiveExecutionBoundary(context.Background(), decision2Req)
		require.NoError(t, err)
		require.Equal(t, uint64(2), decision2.Authority.EventSequence)
		evidenceExpectedRevision = 3
		evidenceNextRevision = 4
		evidenceExpectedItemVersion = 3
		evidenceNextItemVersion = 4
		evidenceEventSequence = 3
	}

	require.NoError(t, db.Model(&agentRunPlanPO{}).Where("run_id = ?", 20).
		Update("high_watermark", 4).Error)
	sentinel, err := agentRunPlanItemToPO(&entity.AgentRunPlanItem{
		ID: 62, RunID: 20, TaskID: 3, Subject: "completed sentinel",
		Description: "not referenced by evidence", Status: entity.AgentRunPlanItemStatusCompleted,
		ActiveForm: "completed", Owner: "agent", Blocks: `[]`, BlockedBy: `[]`,
		Metadata: `{}`, Active: true, Version: 1, CreatedAt: 1_050, UpdatedAt: 1_050,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(sentinel).Error)

	evidenceReq := cloneAdaptiveExecutionBoundaryRequestForReplayTest(decisionReq)
	evidenceReq.Now = 1_100
	evidenceReq.IdempotencyKey = "evidence-boundary-1"
	evidenceReq.Event.ID = 7002
	evidenceReq.Event.EventType = "adaptive.evidence"
	evidenceReq.Event.Payload = `{"schema":"workbench-adaptive-evidence.v1"}`
	evidenceReq.Event.CreatedAt = evidenceReq.Now
	evidenceReq.Checkpoint.ID = 8002
	evidenceReq.Checkpoint.ChannelValues = `{"verified":true}`
	evidenceReq.Checkpoint.ChannelVersions = `{"state":2}`
	evidenceReq.Checkpoint.Metadata = `{"runtime_field":"evidence"}`
	evidenceReq.Checkpoint.CreatedAt = evidenceReq.Now
	evidenceReq.PlanMutation = &AdaptivePlanMutation{
		PlanScopeRunID: 20, ExpectedRevision: evidenceExpectedRevision, NextRevision: evidenceNextRevision,
		Items: []AdaptivePlanItemMutation{{
			ExpectedVersion: evidenceExpectedItemVersion,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1, Subject: "verified major step",
				Description: "evidence update", Status: entity.AgentRunPlanItemStatusCompleted,
				ActiveForm: "completed", Owner: "agent", Blocks: `{"a":1,"z":2}`,
				BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
				Active: true, Version: evidenceNextItemVersion,
			},
		}},
	}
	evidence, err := adaptiveRepo.CommitAdaptiveExecutionBoundary(context.Background(), evidenceReq)
	require.NoError(t, err)
	require.NotNil(t, evidence)
	require.NotEqual(t, decisionReq.IdempotencyKey, evidenceReq.IdempotencyKey)
	require.Equal(t, evidenceEventSequence, evidence.Authority.EventSequence)

	var itemRows []agentRunPlanItemPO
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&itemRows).Error)
	items := make([]*entity.AgentRunPlanItem, 0, len(itemRows))
	for index := range itemRows {
		items = append(items, itemRows[index].toEntity())
	}
	currentFingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	require.NoError(t, err)

	req := newValidAdaptiveVerifiedSuccessRequestForTest()
	req.Now = 1_200
	req.ExpectedThreadTitle = "journal"
	req.ThreadTitle = "generated"
	req.Message.CreatedAt = req.Now
	req.TitleEvent.CreatedAt = req.Now
	req.CompletionEvent.CreatedAt = req.Now
	req.JournalEvent.CreatedAt = req.Now
	req.JournalEvent.OccurredAtUnixNano = req.Now * int64(time.Millisecond)
	req.TerminalCheckpoint = &entity.Checkpoint{
		ID: 8003, ThreadID: 10, RunID: 20, ParentCheckpointID: evidence.Checkpoint.ID,
		CheckpointNS: evidence.Checkpoint.CheckpointNS, RuntimeType: evidence.Checkpoint.RuntimeType,
		RuntimeKey: evidence.Checkpoint.RuntimeKey, EnvelopeVersion: evidence.Checkpoint.EnvelopeVersion,
		ChannelValues: `{"terminal":true}`, ChannelVersions: `{"state":3}`,
		PendingSends: `[]`, Metadata: `{"runtime_field":"terminal"}`, CreatedAt: req.Now,
	}
	fallback := *req.TerminalCheckpoint
	fallback.ChannelValues = `{"terminal":true,"title":"preserved"}`
	req.TerminalCheckpointOnTitleConflict = &fallback
	req.AdaptiveGate.Decision = decision.Authority
	req.AdaptiveGate.Evidence = evidence.Authority
	req.AdaptiveGate.VerificationEvent.CreatedAt = req.Now
	req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
		t,
		req.AdaptiveGate.VerificationEvent.Payload,
		func(fields map[string]any) {
			fields["execution_generation"] = decision.Authority.ExecutionGeneration
			fields["journal_run_id"] = evidence.Authority.JournalRunID
			fields["attempt_id"] = evidence.Authority.AttemptID
			fields["expected_plan_revision"] = evidence.Authority.PlanRevision
			fields["expected_plan_fingerprint"] = currentFingerprint
			fields["verified_checkpoint_id"] = evidence.Authority.CheckpointID
			fields["evidence_head_event_id"] = evidence.Authority.EventID
			fields["created_at"] = req.Now
		},
	)
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(req))
	return adaptiveVerifiedSuccessFixtureForTest{
		DB: db, Repo: &threadRepository{db: db}, Request: req, Decision: decision, Evidence: evidence,
	}
}

func prepareAdaptiveVerifiedSuccessFixtureWithDistinctPlanScopeForTest(
	t *testing.T,
) adaptiveVerifiedSuccessFixtureForTest {
	t.Helper()
	db := newAdaptiveExecutionRepositoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&messagePO{}, &adaptiveVerifiedSuccessOutboxPOForTest{}))
	seedAdaptiveExecutionInitialState(t, db)
	adaptiveRepo := NewAdaptiveExecutionRepository(db)

	sourceReq := adaptiveInitialBoundaryRequest(6990, 7990, 900)
	sourceReq.IdempotencyKey = "source-boundary-1"
	source, err := adaptiveRepo.CommitAdaptiveExecutionBoundary(context.Background(), sourceReq)
	require.NoError(t, err)
	require.NotNil(t, source)
	terminalizeAdaptiveAttemptForTest(t, db, 100)

	decisionReq := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: source.Checkpoint.ID,
		EventID: 7001, CheckpointID: 8001, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1_000,
	})
	decisionReq.IdempotencyKey = "decision-boundary-1"
	decisionReq.Event.EventType = "adaptive.decision"
	decisionReq.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-1","decision_revision":1}`
	decision, err := adaptiveRepo.CommitAdaptiveExecutionBoundary(context.Background(), decisionReq)
	require.NoError(t, err)
	require.NotNil(t, decision)
	require.Equal(t, uint64(1), decision.Authority.EventSequence)
	require.NotEqual(t, decision.Authority.ExecutionRunID, decision.Authority.PlanScopeRunID)
	evidence := decision

	var itemRows []agentRunPlanItemPO
	require.NoError(t, db.Where("run_id = ?", evidence.Authority.PlanScopeRunID).
		Order("task_id ASC").Find(&itemRows).Error)
	items := make([]*entity.AgentRunPlanItem, 0, len(itemRows))
	for index := range itemRows {
		items = append(items, itemRows[index].toEntity())
	}
	currentFingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	require.NoError(t, err)

	req := newValidAdaptiveVerifiedSuccessRequestForTest()
	req.RunID = decision.Authority.ExecutionRunID
	req.LeaseOwner = "worker-21"
	req.LeaseToken = "lease-21"
	req.ExecutionGeneration = decision.Authority.ExecutionGeneration
	req.Now = 1_200
	req.ExpectedThreadTitle = "journal"
	req.ThreadTitle = "generated"
	req.Message.RunID = req.RunID
	req.Message.CreatedAt = req.Now
	req.TitleEvent.RunID = req.RunID
	req.TitleEvent.CreatedAt = req.Now
	req.CompletionEvent.RunID = req.RunID
	req.CompletionEvent.CreatedAt = req.Now
	req.JournalEvent.RunID = req.RunID
	req.JournalEvent.JournalRunID = evidence.Authority.JournalRunID
	req.JournalEvent.AttemptID = evidence.Authority.AttemptID
	req.JournalEvent.CreatedAt = req.Now
	req.JournalEvent.OccurredAtUnixNano = req.Now * int64(time.Millisecond)
	req.TerminalCheckpoint = &entity.Checkpoint{
		ID: 8003, ThreadID: 10, RunID: req.RunID, ParentCheckpointID: evidence.Checkpoint.ID,
		CheckpointNS: evidence.Checkpoint.CheckpointNS, RuntimeType: evidence.Checkpoint.RuntimeType,
		RuntimeKey: evidence.Checkpoint.RuntimeKey, EnvelopeVersion: evidence.Checkpoint.EnvelopeVersion,
		ChannelValues: `{"terminal":true}`, ChannelVersions: `{"state":3}`,
		PendingSends: `[]`, Metadata: `{"runtime_field":"terminal"}`, CreatedAt: req.Now,
	}
	fallback := *req.TerminalCheckpoint
	fallback.ChannelValues = `{"terminal":true,"title":"preserved"}`
	req.TerminalCheckpointOnTitleConflict = &fallback
	req.AdaptiveGate.Decision = decision.Authority
	req.AdaptiveGate.Evidence = evidence.Authority
	req.AdaptiveGate.VerificationEvent.RunID = req.RunID
	req.AdaptiveGate.VerificationEvent.CreatedAt = req.Now
	req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
		t,
		req.AdaptiveGate.VerificationEvent.Payload,
		func(fields map[string]any) {
			fields["execution_run_id"] = decision.Authority.ExecutionRunID
			fields["execution_generation"] = decision.Authority.ExecutionGeneration
			fields["journal_run_id"] = evidence.Authority.JournalRunID
			fields["attempt_id"] = evidence.Authority.AttemptID
			fields["expected_plan_revision"] = evidence.Authority.PlanRevision
			fields["expected_plan_fingerprint"] = currentFingerprint
			fields["verified_checkpoint_id"] = evidence.Authority.CheckpointID
			fields["evidence_head_event_id"] = evidence.Authority.EventID
			fields["created_at"] = req.Now
		},
	)
	require.NoError(t, validateAdaptiveVerifiedSuccessGate(req))
	return adaptiveVerifiedSuccessFixtureForTest{
		DB: db, Repo: &threadRepository{db: db}, Request: req, Decision: decision, Evidence: evidence,
	}
}

func TestThreadRepositoryFinalizeRunSuccessCommitsAdaptiveVerificationBeforeCompletionAtomically(t *testing.T) {
	t.Run("HealthyCommit", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		callerPayload := fixture.Request.AdaptiveGate.VerificationEvent.Payload
		outboxEvent := adaptiveVerifiedSuccessOutboxEventForTest(fixture.Request.Now)
		outboxFingerprint := adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, outboxEvent)
		fixture.Request.OutboxIntent = &NotificationOutboxIntent{
			Event: outboxEvent,
			Append: func(context.Context, *gorm.DB, domainnotification.Event) error {
				return nil
			},
			AppendWithResult: func(_ context.Context, tx *gorm.DB, _ domainnotification.Event) (bool, error) {
				row := adaptiveVerifiedSuccessOutboxPOForTest{
					EventID: outboxEvent.EventID, Fingerprint: outboxFingerprint,
				}
				if err := tx.Create(&row).Error; err != nil {
					return false, err
				}
				return true, nil
			},
		}
		var preRun runPO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.RunID).First(&preRun).Error)
		var preAttempt runAttemptPO
		require.NoError(t, fixture.DB.Where("id = ?", 100).First(&preAttempt).Error)
		var prePlan agentRunPlanPO
		require.NoError(t, fixture.DB.Where("run_id = ?", fixture.Evidence.Authority.PlanScopeRunID).
			First(&prePlan).Error)
		var preItems []agentRunPlanItemPO
		require.NoError(t, fixture.DB.Where("run_id = ?", prePlan.RunID).
			Order("task_id ASC").Find(&preItems).Error)
		expectedFinalizeFingerprint := adaptiveVerifiedSuccessFinalizeFingerprintOracleForTest(
			t, fixture, preRun, preAttempt, &outboxFingerprint, true,
		)

		result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

		require.NoError(t, err)
		require.NotNil(t, result.VerificationEvent)
		require.Equal(t, callerPayload, fixture.Request.AdaptiveGate.VerificationEvent.Payload)
		require.False(t, result.Replayed)
		require.Equal(t, entity.RunStatusSucceeded, result.Run.Status)
		require.Equal(t, fixture.Request.Message, result.Message)
		require.True(t, result.TitleUpdated)
		require.Less(t, result.VerificationEvent.ID, result.CompletionEvent.ID)

		var verification, completion runEventPO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.AdaptiveGate.VerificationEvent.ID).
			First(&verification).Error)
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.CompletionEvent.ID).
			First(&completion).Error)
		require.Equal(t, fixture.Evidence.Authority.JournalRunID, requireInt64PointerForTest(t, verification.JournalRunID))
		require.Equal(t, fixture.Evidence.Authority.AttemptID, requireStringPointerForTest(t, verification.AttemptID))
		require.Equal(t, fixture.Evidence.Authority.EventSequence+1, requireUint64PointerForTest(t, verification.Sequence))
		require.Equal(t, fixture.Request.AdaptiveGate.VerificationIdempotencyKey, requireStringPointerForTest(t, verification.IdempotencyKey))
		require.Equal(t, fixture.Evidence.Authority.EventSequence+2, requireUint64PointerForTest(t, completion.Sequence))
		require.Equal(t, fixture.Request.JournalEvent.IdempotencyKey, requireStringPointerForTest(t, completion.IdempotencyKey))
		require.Less(t, requireUint64PointerForTest(t, verification.Sequence), requireUint64PointerForTest(t, completion.Sequence))
		require.LessOrEqual(t, len(verification.Payload), adaptiveVerifiedSuccessMaxPayloadBytes)
		durablePayload, decodeErr := decodeAdaptiveVerifiedSuccessPayload(string(verification.Payload), true)
		require.NoError(t, decodeErr)
		require.NotNil(t, durablePayload.OutboxFingerprint)
		require.Equal(t, outboxFingerprint, *durablePayload.OutboxFingerprint)
		require.Equal(t, expectedFinalizeFingerprint, durablePayload.FinalizeRequestFingerprint)
		require.Equal(t, string(verification.Payload), result.VerificationEvent.Payload)

		var attempt runAttemptPO
		require.NoError(t, fixture.DB.Where("id = ?", 100).First(&attempt).Error)
		require.Equal(t, fixture.Evidence.Authority.EventSequence+3, attempt.NextSequence)
		require.Equal(t, fixture.Evidence.Authority.EventSequence+1, attempt.LastCommittedSequence)
		require.Equal(t, string(entity.RunAttemptStatusCompleted), attempt.Status)
		require.Nil(t, attempt.ActiveSlot)
		require.Equal(t, result.CompletionEvent.ID, requireInt64PointerForTest(t, attempt.TerminalEventID))
		require.Equal(t, fixture.Request.Now, requireInt64PointerForTest(t, attempt.EndedAt))
		require.Equal(t, fixture.Request.Now, attempt.UpdatedAt)

		var message messagePO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.Message.ID).First(&message).Error)
		require.Equal(t, fixture.Request.Message.Content, message.Content)
		var terminal checkpointPO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.TerminalCheckpoint.ID).First(&terminal).Error)
		require.Equal(t, fixture.Evidence.Authority.CheckpointID, terminal.ParentCheckpointID)
		var plan agentRunPlanPO
		require.NoError(t, fixture.DB.Where("run_id = ?", fixture.Evidence.Authority.PlanScopeRunID).First(&plan).Error)
		require.Equal(t, prePlan, plan)
		var items []agentRunPlanItemPO
		require.NoError(t, fixture.DB.Where("run_id = ?", plan.RunID).Order("task_id ASC").Find(&items).Error)
		require.Equal(t, preItems, items)
		var outboxRows []adaptiveVerifiedSuccessOutboxPOForTest
		require.NoError(t, fixture.DB.Find(&outboxRows).Error)
		require.Equal(t, []adaptiveVerifiedSuccessOutboxPOForTest{{
			EventID: outboxEvent.EventID, Fingerprint: outboxFingerprint,
		}}, outboxRows)
	})

	t.Run("MessageMetadataSQLNullRemainsValid", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		fixture.Request.Message.Metadata = ""
		var preRun runPO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.RunID).First(&preRun).Error)
		var preAttempt runAttemptPO
		require.NoError(t, fixture.DB.Where("id = ?", 100).First(&preAttempt).Error)
		expectedFinalizeFingerprint := adaptiveVerifiedSuccessFinalizeFingerprintOracleForTest(
			t, fixture, preRun, preAttempt, nil, true,
		)

		result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

		require.NoError(t, err)
		require.NotNil(t, result)
		var metadataSQLNull bool
		require.NoError(t, fixture.DB.Raw(
			"SELECT metadata IS NULL FROM agent_thread_messages WHERE id = ?",
			fixture.Request.Message.ID,
		).Row().Scan(&metadataSQLNull))
		require.True(t, metadataSQLNull)
		var verification runEventPO
		require.NoError(t, fixture.DB.Where(
			"id = ?", fixture.Request.AdaptiveGate.VerificationEvent.ID,
		).First(&verification).Error)
		durablePayload, decodeErr := decodeAdaptiveVerifiedSuccessPayload(string(verification.Payload), true)
		require.NoError(t, decodeErr)
		require.Equal(t, expectedFinalizeFingerprint, durablePayload.FinalizeRequestFingerprint)
	})

	t.Run("AllCurrentPlanItemsFingerprintRejectsUnreferencedDrift", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		require.NoError(t, fixture.DB.Model(&agentRunPlanItemPO{}).Where("id = ?", 62).
			Update("subject", "drifted sentinel").Error)
		before := snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB)

		result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

		require.Nil(t, result)
		require.ErrorIs(t, err, ErrAdaptiveExecutionVerifiedSuccessConflict)
		require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB))
	})

	t.Run("ReservedHighWatermarkGapRemainsValid", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		var plan agentRunPlanPO
		require.NoError(t, fixture.DB.Where("run_id = ?", 20).First(&plan).Error)
		require.Equal(t, int64(4), plan.HighWatermark)

		result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

		require.NoError(t, err)
		require.NotNil(t, result.VerificationEvent)
	})
}

func adaptiveVerifiedSuccessRetryableOutboxIntentForDriftTest(
	t *testing.T,
	now int64,
) *NotificationOutboxIntent {
	t.Helper()
	event := adaptiveVerifiedSuccessOutboxEventForTest(now)
	fingerprint := adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, event)
	return &NotificationOutboxIntent{
		Event: event,
		Append: func(context.Context, *gorm.DB, domainnotification.Event) error {
			return fmt.Errorf("legacy append must not be used")
		},
		AppendWithResult: func(_ context.Context, tx *gorm.DB, _ domainnotification.Event) (bool, error) {
			var count int64
			if err := tx.Model(&adaptiveVerifiedSuccessOutboxPOForTest{}).
				Where("event_id = ?", event.EventID).Count(&count).Error; err != nil {
				return false, err
			}
			if count != 0 {
				var row adaptiveVerifiedSuccessOutboxPOForTest
				if err := tx.Where("event_id = ?", event.EventID).First(&row).Error; err != nil {
					return false, err
				}
				if row.Fingerprint != fingerprint {
					return false, fmt.Errorf("durable outbox fingerprint drift")
				}
				return false, nil
			}
			if err := tx.Create(&adaptiveVerifiedSuccessOutboxPOForTest{
				EventID: event.EventID, Fingerprint: fingerprint,
			}).Error; err != nil {
				return false, err
			}
			return true, nil
		},
	}
}

func driftAdaptiveVerifiedSuccessEvidenceRuntimeForTest(
	t *testing.T,
	fixture adaptiveVerifiedSuccessFixtureForTest,
) {
	t.Helper()
	var event runEventPO
	require.NoError(t, fixture.DB.Where("id = ?", fixture.Evidence.Authority.EventID).First(&event).Error)
	var checkpoint checkpointPO
	require.NoError(t, fixture.DB.Where("id = ?", fixture.Evidence.Authority.CheckpointID).
		First(&checkpoint).Error)
	checkpoint.RuntimeKey += ":durable-drift"
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &checkpoint)
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(checkpoint.Metadata)
	require.NoError(t, err)
	event.SnapshotID = adaptiveExecutionStringPointer(metadata.CheckpointFingerprint)
	eventFingerprint, err := adaptiveExecutionEventFingerprint(&event)
	require.NoError(t, err)
	checkpoint.Metadata = rewriteAdaptiveExecutionCheckpointMetadataForTest(
		t,
		checkpoint.Metadata,
		func(fields map[string]json.RawMessage) {
			encoded, marshalErr := json.Marshal(eventFingerprint)
			require.NoError(t, marshalErr)
			fields["event_fingerprint"] = encoded
		},
	)
	require.NoError(t, fixture.DB.Save(&event).Error)
	require.NoError(t, fixture.DB.Save(&checkpoint).Error)
}

func replaceAdaptiveExecutionCheckpointMetadataForTest(
	t *testing.T,
	raw []byte,
	metadata *adaptiveExecutionCheckpointMetadata,
) []byte {
	t.Helper()
	require.NotNil(t, metadata)
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &envelope))
	encodedMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)
	envelope["adaptive_execution"] = encodedMetadata
	encodedEnvelope, err := json.Marshal(envelope)
	require.NoError(t, err)
	return encodedEnvelope
}

func resignAdaptiveVerifiedSuccessAuthorityForTest(
	t *testing.T,
	fixture adaptiveVerifiedSuccessFixtureForTest,
	authority AdaptiveExecutionBoundaryAuthority,
	mutateEvent func(*runEventPO),
	mutateCheckpoint func(*checkpointPO),
	mutateMetadata func(*adaptiveExecutionCheckpointMetadata),
) {
	t.Helper()
	var event runEventPO
	require.NoError(t, fixture.DB.Where("id = ?", authority.EventID).First(&event).Error)
	var checkpoint checkpointPO
	require.NoError(t, fixture.DB.Where("id = ?", authority.CheckpointID).First(&checkpoint).Error)
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(checkpoint.Metadata)
	require.NoError(t, err)
	if mutateEvent != nil {
		mutateEvent(&event)
	}
	if mutateCheckpoint != nil {
		mutateCheckpoint(&checkpoint)
	}
	if mutateMetadata != nil {
		mutateMetadata(metadata)
	}
	checkpoint.Metadata = replaceAdaptiveExecutionCheckpointMetadataForTest(t, checkpoint.Metadata, metadata)
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &checkpoint)
	metadata, err = decodeAdaptiveExecutionCheckpointMetadata(checkpoint.Metadata)
	require.NoError(t, err)
	event.SnapshotID = adaptiveExecutionStringPointer(metadata.CheckpointFingerprint)
	eventFingerprint, err := adaptiveExecutionEventFingerprint(&event)
	require.NoError(t, err)
	metadata.EventFingerprint = eventFingerprint
	checkpoint.Metadata = replaceAdaptiveExecutionCheckpointMetadataForTest(t, checkpoint.Metadata, metadata)
	require.NoError(t, fixture.DB.Save(&event).Error)
	require.NoError(t, fixture.DB.Save(&checkpoint).Error)
}

func driftAdaptiveVerifiedSuccessEventAnchorForTest(
	t *testing.T,
	fixture adaptiveVerifiedSuccessFixtureForTest,
	authority AdaptiveExecutionBoundaryAuthority,
) {
	t.Helper()
	var event runEventPO
	require.NoError(t, fixture.DB.Where("id = ?", authority.EventID).First(&event).Error)
	var checkpoint checkpointPO
	require.NoError(t, fixture.DB.Where("id = ?", authority.CheckpointID).First(&checkpoint).Error)
	event.SnapshotID = adaptiveExecutionStringPointer(strings.Repeat("a", sha256.Size*2))
	eventFingerprint, err := adaptiveExecutionEventFingerprint(&event)
	require.NoError(t, err)
	checkpoint.Metadata = rewriteAdaptiveExecutionCheckpointMetadataForTest(
		t,
		checkpoint.Metadata,
		func(fields map[string]json.RawMessage) {
			encoded, marshalErr := json.Marshal(eventFingerprint)
			require.NoError(t, marshalErr)
			fields["event_fingerprint"] = encoded
		},
	)
	require.NoError(t, fixture.DB.Save(&event).Error)
	require.NoError(t, fixture.DB.Save(&checkpoint).Error)
}

func mutateAdaptiveVerifiedSuccessDurableVerificationPayloadForTest(
	t *testing.T,
	fixture adaptiveVerifiedSuccessFixtureForTest,
	mutate func(map[string]any),
) {
	t.Helper()
	var verification runEventPO
	require.NoError(t, fixture.DB.Where(
		"id = ?", fixture.Request.AdaptiveGate.VerificationEvent.ID,
	).First(&verification).Error)
	verification.Payload = []byte(adaptiveVerifiedSuccessPayloadMutationForTest(
		t,
		string(verification.Payload),
		mutate,
	))
	require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("id = ?", verification.ID).
		UpdateColumn("payload", verification.Payload).Error)
}

func requireAdaptiveVerifiedSuccessRowCountForTest(
	t *testing.T,
	db *gorm.DB,
	model any,
	query string,
	want int64,
	args ...any,
) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(model).Where(query, args...).Count(&count).Error)
	require.Equal(t, want, count)
}

func TestThreadRepositoryFinalizeRunSuccessRejectsAdaptiveAuthorityDriftWithoutWrites(t *testing.T) {
	type driftCase struct {
		name          string
		prepare       func(*testing.T) adaptiveVerifiedSuccessFixtureForTest
		commitFirst   bool
		expectedCause error
		errorContains string
		mutate        func(*testing.T, adaptiveVerifiedSuccessFixtureForTest)
	}
	defaultFixture := func(t *testing.T) adaptiveVerifiedSuccessFixtureForTest {
		return prepareAdaptiveVerifiedSuccessFixtureForTest(t)
	}
	cases := []driftCase{
		{
			name: "JournalRootMissing", prepare: defaultFixture,
			expectedCause: ErrRunLeaseLost,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(
					&runPO{}, fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				).Error)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 0,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1, fixture.Request.RunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &threadPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.ThreadID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runAttemptPO{}, "journal_run_id = ? AND attempt_id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				)
			},
		},
		{
			name: "ExecutionRunMissing", prepare: defaultFixture,
			expectedCause: ErrRunLeaseLost,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(&runPO{}, fixture.Request.RunID).Error)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 0, fixture.Request.RunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &threadPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.ThreadID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runAttemptPO{}, "journal_run_id = ? AND attempt_id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				)
			},
		},
		{
			name: "ThreadMissing", prepare: defaultFixture,
			errorContains: "thread",
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(
					&threadPO{}, fixture.Request.AdaptiveGate.Evidence.ThreadID,
				).Error)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1, fixture.Request.RunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &threadPO{}, "id = ?", 0,
					fixture.Request.AdaptiveGate.Evidence.ThreadID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runAttemptPO{}, "journal_run_id = ? AND attempt_id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				)
			},
		},
		{
			name: "AttemptMissing", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionAttemptConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Where(
					"journal_run_id = ? AND attempt_id = ?",
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				).Delete(&runAttemptPO{}).Error)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1, fixture.Request.RunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &threadPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.ThreadID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runAttemptPO{}, "journal_run_id = ? AND attempt_id = ?", 0,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				)
			},
		},
		{
			name:          "DistinctPlanScopeRunMissing",
			prepare:       prepareAdaptiveVerifiedSuccessFixtureWithDistinctPlanScopeForTest,
			expectedCause: ErrAdaptiveExecutionPlanScopeConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				scopeRunID := fixture.Request.AdaptiveGate.Evidence.PlanScopeRunID
				require.Equal(t, int64(20), scopeRunID)
				require.Equal(t, scopeRunID, fixture.Request.AdaptiveGate.Decision.PlanScopeRunID)
				require.NotEqual(t, fixture.Request.RunID, scopeRunID)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1, fixture.Request.RunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &threadPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.ThreadID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runAttemptPO{}, "journal_run_id = ? AND attempt_id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &agentRunPlanPO{}, "run_id = ?", 1, scopeRunID,
				)
				require.NoError(t, fixture.DB.Delete(&runPO{}, scopeRunID).Error)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 0, scopeRunID,
				)
			},
		},
		{
			name: "PlanMissing", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanScopeConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				scopeRunID := fixture.Request.AdaptiveGate.Evidence.PlanScopeRunID
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1, fixture.Request.RunID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &threadPO{}, "id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.ThreadID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runAttemptPO{}, "journal_run_id = ? AND attempt_id = ?", 1,
					fixture.Request.AdaptiveGate.Evidence.JournalRunID,
					fixture.Request.AdaptiveGate.Evidence.AttemptID,
				)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &runPO{}, "id = ?", 1, scopeRunID,
				)
				require.NoError(t, fixture.DB.Delete(&agentRunPlanPO{}, scopeRunID).Error)
				requireAdaptiveVerifiedSuccessRowCountForTest(
					t, fixture.DB, &agentRunPlanPO{}, "run_id = ?", 0, scopeRunID,
				)
			},
		},
		{
			name: "Plan", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanRevisionConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&agentRunPlanPO{}).Where("run_id = ?", 20).
					UpdateColumn("high_watermark", 2).Error)
			},
		},
		{
			name: "PlanItems", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanItemVersionConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&agentRunPlanItemPO{}).Where("id = ?", 62).
					UpdateColumn("subject", "unreferenced completed sentinel drift").Error)
			},
		},
		{
			name: "Decision", prepare: prepareAdaptiveVerifiedSuccessFixtureWithNewerDecisionForTest,
			mutate: func(*testing.T, adaptiveVerifiedSuccessFixtureForTest) {},
		},
		{
			name: "Verification", prepare: defaultFixture, commitFirst: true,
			expectedCause: ErrAdaptiveExecutionSequenceConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runEventPO{}).
					Where("id = ?", fixture.Request.CompletionEvent.ID).
					UpdateColumn("sequence", fixture.Evidence.Authority.EventSequence+3).Error)
			},
		},
		{
			name: "EvidenceHighWatermark", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionSequenceConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumn("last_committed_sequence", fixture.Evidence.Authority.EventSequence-1).Error)
			},
		},
		{
			name: "TerminalCheckpointReference", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			mutate:        driftAdaptiveVerifiedSuccessEvidenceRuntimeForTest,
		},
		{
			name: "AttemptTerminal", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionSequenceConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumns(map[string]any{
						"status": string(entity.RunAttemptStatusCompleted), "active_slot": nil,
					}).Error)
			},
		},
		{
			name: "AttemptSlot", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionSequenceConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumn("active_slot", nil).Error)
			},
		},
		{
			name: "AttemptExecutionDrift", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionAttemptConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumn("execution_run_id", 30).Error)
			},
		},
		{
			name: "AttemptNextSequence", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionSequenceConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumn("next_sequence", fixture.Evidence.Authority.EventSequence+2).Error)
			},
		},
		{
			name: "DecisionMissingEvent", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(
					&runEventPO{}, fixture.Decision.Authority.EventID,
				).Error)
			},
		},
		{
			name: "DecisionMissingCheckpoint", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(
					&checkpointPO{}, fixture.Decision.Authority.CheckpointID,
				).Error)
			},
		},
		{
			name: "DecisionCheckpointTuple", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			errorContains: "identity",
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t,
					fixture,
					fixture.Decision.Authority,
					nil,
					func(checkpoint *checkpointPO) { checkpoint.ThreadID++ },
					nil,
				)
			},
		},
		{
			name: "DecisionTuple", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runEventPO{}).
					Where("id = ?", fixture.Decision.Authority.EventID).
					UpdateColumn("idempotency_key", "decision-tuple-drift").Error)
			},
		},
		{
			name: "DecisionEventFingerprint", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runEventPO{}).
					Where("id = ?", fixture.Decision.Authority.EventID).
					UpdateColumn("payload", []byte(`{"schema":"workbench-adaptive-decision.v1","decision_id":"tampered","decision_revision":1}`)).Error)
			},
		},
		{
			name: "DecisionCheckpointFingerprint", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&checkpointPO{}).
					Where("id = ?", fixture.Decision.Authority.CheckpointID).
					UpdateColumn("channel_values", []byte(`{"tampered":true}`)).Error)
			},
		},
		{
			name: "DecisionPayload", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Decision.Authority,
					func(event *runEventPO) {
						event.Payload = []byte(`{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-drift","decision_revision":1}`)
					},
					nil,
					nil,
				)
			},
		},
		{
			name: "EvidenceMissingEvent", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(
					&runEventPO{}, fixture.Evidence.Authority.EventID,
				).Error)
			},
		},
		{
			name: "EvidenceMissingCheckpoint", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Delete(
					&checkpointPO{}, fixture.Evidence.Authority.CheckpointID,
				).Error)
			},
		},
		{
			name: "EvidenceSource", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) {
						sourceAttempt := "source-drift"
						sourceCheckpoint := int64(8999)
						metadata.SourceAttemptID = &sourceAttempt
						metadata.SourceCheckpointID = &sourceCheckpoint
					},
				)
			},
		},
		{
			name: "EvidenceGeneration", prepare: defaultFixture,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) { metadata.ExecutionGeneration++ },
				)
			},
		},
		{
			name: "EvidenceEventAnchor", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				driftAdaptiveVerifiedSuccessEventAnchorForTest(t, fixture, fixture.Evidence.Authority)
			},
		},
		{
			name: "EvidenceCheckpointFingerprint", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&checkpointPO{}).
					Where("id = ?", fixture.Evidence.Authority.CheckpointID).
					UpdateColumn("channel_versions", []byte(`{"tampered":true}`)).Error)
			},
		},
		{
			name: "EvidenceCheckpointParent", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			errorContains: "parent",
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t,
					fixture,
					fixture.Evidence.Authority,
					nil,
					func(checkpoint *checkpointPO) { checkpoint.ParentCheckpointID++ },
					nil,
				)
			},
		},
		{
			name: "EvidencePlanScope", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanScopeConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) { metadata.PlanScopeRunID = 30 },
				)
			},
		},
		{
			name: "EvidenceRevision", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanRevisionConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) { metadata.PlanRevision++ },
				)
			},
		},
		{
			name: "EvidenceItemReference", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanItemVersionConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) { metadata.ItemRefs[0].ID = 9999 },
				)
			},
		},
		{
			name: "EvidenceItemVersion", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanItemVersionConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) { metadata.ItemRefs[0].Version++ },
				)
			},
		},
		{
			name: "EvidenceFingerprint", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionPlanItemVersionConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				drift := strings.Repeat("d", sha256.Size*2)
				fixture.Request.AdaptiveGate.Evidence.PlanItemFingerprint = drift
				resignAdaptiveVerifiedSuccessAuthorityForTest(
					t, fixture, fixture.Evidence.Authority, nil, nil,
					func(metadata *adaptiveExecutionCheckpointMetadata) { metadata.ItemFingerprint = drift },
				)
			},
		},
		{
			name: "EvidenceNextSequenceHighWatermark", prepare: defaultFixture,
			expectedCause: ErrAdaptiveExecutionSequenceConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumn("next_sequence", fixture.Evidence.Authority.EventSequence+2).Error)
			},
		},
		{
			name: "VerificationServerFingerprint", prepare: defaultFixture, commitFirst: true,
			errorContains: "finalize_request_fingerprint",
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				mutateAdaptiveVerifiedSuccessDurableVerificationPayloadForTest(
					t,
					fixture,
					func(fields map[string]any) {
						fields["finalize_request_fingerprint"] = strings.Repeat("f", sha256.Size*2)
					},
				)
			},
		},
		{
			name: "VerificationServerOutboxFingerprint", prepare: defaultFixture, commitFirst: true,
			errorContains: "outbox_fingerprint",
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				mutateAdaptiveVerifiedSuccessDurableVerificationPayloadForTest(
					t,
					fixture,
					func(fields map[string]any) {
						fields["outbox_fingerprint"] = strings.Repeat("e", sha256.Size*2)
					},
				)
			},
		},
		{
			name: "VerificationTerminalAttemptPostImage", prepare: defaultFixture, commitFirst: true,
			expectedCause: ErrAdaptiveExecutionAttemptConflict,
			mutate: func(t *testing.T, fixture adaptiveVerifiedSuccessFixtureForTest) {
				require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
					UpdateColumn("ended_at", fixture.Request.Now+1).Error)
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := test.prepare(t)
			if test.commitFirst {
				fixture.Request.OutboxIntent = adaptiveVerifiedSuccessRetryableOutboxIntentForDriftTest(
					t, fixture.Request.Now,
				)
				committed, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)
				require.NoError(t, err)
				require.NotNil(t, committed)
				fixture.Repo = &threadRepository{db: fixture.DB}
			} else {
				callbackSentinel := fmt.Errorf("authority drift reached outbox callback")
				outboxEvent := adaptiveVerifiedSuccessOutboxEventForTest(fixture.Request.Now)
				outboxFingerprint := adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, outboxEvent)
				fixture.Request.OutboxIntent = &NotificationOutboxIntent{
					Event: outboxEvent,
					Append: func(context.Context, *gorm.DB, domainnotification.Event) error {
						return fmt.Errorf("legacy append must not be used")
					},
					AppendWithResult: func(_ context.Context, tx *gorm.DB, _ domainnotification.Event) (bool, error) {
						if err := tx.Create(&adaptiveVerifiedSuccessOutboxPOForTest{
							EventID: outboxEvent.EventID, Fingerprint: outboxFingerprint,
						}).Error; err != nil {
							return false, err
						}
						return false, callbackSentinel
					},
				}
			}
			test.mutate(t, fixture)
			before := snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB)

			result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

			require.Nil(t, result)
			require.ErrorIs(t, err, ErrAdaptiveExecutionVerifiedSuccessConflict)
			if test.expectedCause != nil {
				require.ErrorIs(t, err, test.expectedCause)
			}
			if test.errorContains != "" {
				require.ErrorContains(t, err, test.errorContains)
			}
			require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB))
		})
	}
}

type adaptiveVerifiedSuccessCommittedReplayFixtureForTest struct {
	Fixture             adaptiveVerifiedSuccessFixtureForTest
	First               *FinalizeRunSuccessResult
	OutboxEvent         domainnotification.Event
	OutboxFingerprint   string
	OutboxCallbackCalls *int
}

func adaptiveVerifiedSuccessTrackedOutboxIntentForReplayTest(
	t *testing.T,
	now int64,
	calls *int,
) *NotificationOutboxIntent {
	t.Helper()
	require.NotNil(t, calls)
	event := adaptiveVerifiedSuccessOutboxEventForTest(now)
	fingerprint := adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, event)
	return &NotificationOutboxIntent{
		Event: event,
		Append: func(context.Context, *gorm.DB, domainnotification.Event) error {
			return fmt.Errorf("legacy append must not be used")
		},
		AppendWithResult: func(_ context.Context, tx *gorm.DB, _ domainnotification.Event) (bool, error) {
			*calls++
			var count int64
			if err := tx.Model(&adaptiveVerifiedSuccessOutboxPOForTest{}).
				Where("event_id = ?", event.EventID).Count(&count).Error; err != nil {
				return false, err
			}
			if count == 0 {
				if err := tx.Create(&adaptiveVerifiedSuccessOutboxPOForTest{
					EventID: event.EventID, Fingerprint: fingerprint,
				}).Error; err != nil {
					return false, err
				}
				return true, nil
			}
			var row adaptiveVerifiedSuccessOutboxPOForTest
			if err := tx.Where("event_id = ?", event.EventID).First(&row).Error; err != nil {
				return false, err
			}
			if row.Fingerprint != fingerprint {
				return false, fmt.Errorf(
					"%w: durable outbox fingerprint drift",
					domainnotification.ErrIdempotencyConflict,
				)
			}
			return false, nil
		},
	}
}

func prepareAdaptiveVerifiedSuccessCommittedReplayFixtureForTest(
	t *testing.T,
	withOutbox bool,
	titleConflict bool,
) adaptiveVerifiedSuccessCommittedReplayFixtureForTest {
	t.Helper()
	fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
	if titleConflict {
		require.NoError(t, fixture.DB.Model(&threadPO{}).Where("id = ?", 10).
			UpdateColumn("title", "concurrent title").Error)
	}
	calls := new(int)
	var outboxEvent domainnotification.Event
	var outboxFingerprint string
	if withOutbox {
		fixture.Request.OutboxIntent = adaptiveVerifiedSuccessTrackedOutboxIntentForReplayTest(
			t,
			fixture.Request.Now,
			calls,
		)
		outboxEvent = fixture.Request.OutboxIntent.Event
		outboxFingerprint = adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, outboxEvent)
	}
	first, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.False(t, first.Replayed)
	require.NotNil(t, first.VerificationEvent)
	require.Equal(t, !titleConflict, first.TitleUpdated)
	if titleConflict {
		require.Nil(t, first.TitleEvent)
	} else {
		require.Equal(t, fixture.Request.TitleEvent, first.TitleEvent)
	}
	if withOutbox {
		require.Equal(t, 1, *calls)
	} else {
		require.Zero(t, *calls)
	}
	var run runPO
	require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.RunID).First(&run).Error)
	require.Equal(t, string(entity.RunStatusSucceeded), run.Status)
	require.Nil(t, run.LeaseOwner)
	require.Nil(t, run.LeaseToken)
	require.Nil(t, run.LeaseExpiresAt)
	var attempt runAttemptPO
	require.NoError(t, fixture.DB.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, string(entity.RunAttemptStatusCompleted), attempt.Status)
	require.Nil(t, attempt.ActiveSlot)
	require.Equal(t, fixture.Request.Now, requireInt64PointerForTest(t, attempt.EndedAt))
	require.Equal(t, fixture.Request.Now, attempt.UpdatedAt)
	return adaptiveVerifiedSuccessCommittedReplayFixtureForTest{
		Fixture: fixture, First: first,
		OutboxEvent: outboxEvent, OutboxFingerprint: outboxFingerprint,
		OutboxCallbackCalls: calls,
	}
}

func TestThreadRepositoryFinalizeRunSuccessAdaptiveVerificationSurvivesDegradedProjection(t *testing.T) {
	t.Run("DurablyDegradedWritesAuthoritativeBaseTuples", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		degradedAt := int64(1_150)
		require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
			UpdateColumns(map[string]any{
				"projection_state":       string(entity.JournalProjectionStateDegraded),
				"projection_degraded_at": degradedAt,
			}).Error)
		var preRun runPO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.RunID).First(&preRun).Error)
		var preAttempt runAttemptPO
		require.NoError(t, fixture.DB.Where("id = ?", 100).First(&preAttempt).Error)
		expectedFinalizeFingerprint := adaptiveVerifiedSuccessFinalizeFingerprintOracleForTest(
			t,
			fixture,
			preRun,
			preAttempt,
			nil,
			false,
		)

		result, err := (&threadRepository{db: fixture.DB}).FinalizeRunSuccess(
			context.Background(),
			fixture.Request,
		)

		require.NoError(t, err)
		require.NotNil(t, result)
		require.False(t, result.Replayed)
		require.NotNil(t, result.VerificationEvent)
		var verification, completion runEventPO
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.AdaptiveGate.VerificationEvent.ID).
			First(&verification).Error)
		require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.CompletionEvent.ID).
			First(&completion).Error)
		require.Equal(t, fixture.Evidence.Authority.JournalRunID, requireInt64PointerForTest(t, verification.JournalRunID))
		require.Equal(t, fixture.Evidence.Authority.AttemptID, requireStringPointerForTest(t, verification.AttemptID))
		require.Equal(t, fixture.Evidence.Authority.EventSequence+1, requireUint64PointerForTest(t, verification.Sequence))
		require.Equal(t, fixture.Request.AdaptiveGate.VerificationIdempotencyKey, requireStringPointerForTest(t, verification.IdempotencyKey))
		durableVerification, decodeErr := decodeAdaptiveVerifiedSuccessPayload(string(verification.Payload), true)
		require.NoError(t, decodeErr)
		require.Nil(t, durableVerification.OutboxFingerprint)
		require.Equal(t, expectedFinalizeFingerprint, durableVerification.FinalizeRequestFingerprint)
		require.Equal(t, fixture.Evidence.Authority.JournalRunID, requireInt64PointerForTest(t, completion.JournalRunID))
		require.Equal(t, fixture.Evidence.Authority.AttemptID, requireStringPointerForTest(t, completion.AttemptID))
		require.Equal(t, fixture.Evidence.Authority.EventSequence+2, requireUint64PointerForTest(t, completion.Sequence))
		require.Equal(t, fixture.Request.JournalEvent.IdempotencyKey, requireStringPointerForTest(t, completion.IdempotencyKey))
		require.Nil(t, completion.JournalEventType)
		require.Empty(t, completion.JournalPayload)
		var attempt runAttemptPO
		require.NoError(t, fixture.DB.Where("id = ?", 100).First(&attempt).Error)
		require.Equal(t, string(entity.JournalProjectionStateDegraded), attempt.ProjectionState)
		require.Equal(t, degradedAt, requireInt64PointerForTest(t, attempt.ProjectionDegradedAt))
		require.Equal(t, string(entity.RunAttemptStatusCompleted), attempt.Status)
		require.Nil(t, attempt.ActiveSlot)
		require.Equal(t, fixture.Evidence.Authority.EventSequence+3, attempt.NextSequence)
		require.Equal(t, fixture.Evidence.Authority.EventSequence+1, attempt.LastCommittedSequence)
		require.Equal(t, completion.ID, requireInt64PointerForTest(t, attempt.TerminalEventID))
		require.Equal(t, fixture.Request.Now, requireInt64PointerForTest(t, attempt.EndedAt))
		require.Equal(t, fixture.Request.Now, attempt.UpdatedAt)
	})

	t.Run("ProjectionModesCanonicalizeCompletionIdempotencyKeyEqually", func(t *testing.T) {
		for _, projectionState := range []entity.JournalProjectionState{
			entity.JournalProjectionStateHealthy,
			entity.JournalProjectionStateDegraded,
		} {
			projectionState := projectionState
			t.Run(string(projectionState), func(t *testing.T) {
				fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
				fixture.Request.JournalEvent.IdempotencyKey = "  completion-1  "
				if projectionState == entity.JournalProjectionStateDegraded {
					require.NoError(t, fixture.DB.Model(&runAttemptPO{}).Where("id = ?", 100).
						UpdateColumns(map[string]any{
							"projection_state":       string(projectionState),
							"projection_degraded_at": int64(1_150),
						}).Error)
				}

				result, err := (&threadRepository{db: fixture.DB}).FinalizeRunSuccess(
					context.Background(),
					fixture.Request,
				)

				require.NoError(t, err)
				require.NotNil(t, result)
				var completion runEventPO
				require.NoError(t, fixture.DB.Where("id = ?", fixture.Request.CompletionEvent.ID).
					First(&completion).Error)
				require.Equal(t, "completion-1", requireStringPointerForTest(t, completion.IdempotencyKey))
			})
		}
	})

	t.Run("HealthyRejectsInvalidJournalProjectionWithoutWrites", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		fixture.Request.JournalEvent.Visibility = entity.JournalVisibility("invalid")
		before := snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB)

		result, err := (&threadRepository{db: fixture.DB}).FinalizeRunSuccess(
			context.Background(),
			fixture.Request,
		)

		require.Nil(t, result)
		require.ErrorIs(t, err, ErrAdaptiveExecutionVerifiedSuccessInvalid)
		require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB))
	})
}

func TestThreadRepositoryFinalizeRunSuccessAdaptiveGateReplaysExactCommittedResultWithoutWrites(t *testing.T) {
	t.Run("ExactCommittedRetry", func(t *testing.T) {
		committed := prepareAdaptiveVerifiedSuccessCommittedReplayFixtureForTest(t, true, false)
		before := snapshotAdaptiveVerifiedSuccessDBForTest(t, committed.Fixture.DB)

		replayed, err := (&threadRepository{db: committed.Fixture.DB}).FinalizeRunSuccess(
			context.Background(),
			cloneAdaptiveVerifiedSuccessRequestForTest(committed.Fixture.Request),
		)

		require.NoError(t, err)
		expected := *committed.First
		expected.Replayed = true
		require.Equal(t, &expected, replayed)
		require.Equal(t, 2, *committed.OutboxCallbackCalls)
		require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, committed.Fixture.DB))
	})

	t.Run("ExactCommittedRetryWhenConcurrentTitleAlreadyEqualsRequested", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		requestedTitle := strings.TrimSpace(fixture.Request.ThreadTitle)
		require.NotEmpty(t, requestedTitle)
		require.NoError(t, fixture.DB.Model(&threadPO{}).Where("id = ?", fixture.Request.Message.ThreadID).
			UpdateColumn("title", requestedTitle).Error)
		calls := new(int)
		fixture.Request.OutboxIntent = adaptiveVerifiedSuccessTrackedOutboxIntentForReplayTest(
			t,
			fixture.Request.Now,
			calls,
		)
		first, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)
		require.NoError(t, err)
		require.NotNil(t, first)
		require.False(t, first.TitleUpdated)
		require.Nil(t, first.TitleEvent)
		require.Equal(t, fixture.Request.TerminalCheckpointOnTitleConflict, first.TerminalCheckpoint)
		require.Equal(t, 1, *calls)
		before := snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB)

		replayed, err := (&threadRepository{db: fixture.DB}).FinalizeRunSuccess(
			context.Background(),
			cloneAdaptiveVerifiedSuccessRequestForTest(fixture.Request),
		)

		require.NoError(t, err)
		expected := *first
		expected.Replayed = true
		require.Equal(t, &expected, replayed)
		require.Equal(t, 2, *calls)
		require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB))
	})

	type replayConflictMutation func(
		*testing.T,
		*adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
		*FinalizeRunSuccessRequest,
	) func(*testing.T)
	type replayConflictVariant struct {
		name   string
		mutate replayConflictMutation
	}
	type replayConflictCase struct {
		name          string
		withOutbox    bool
		titleConflict bool
		expectedCause error
		errorContains string
		mutate        replayConflictMutation
		variants      []replayConflictVariant
	}
	requireOutboxCalls := func(
		committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
		want int,
	) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			require.Equal(t, want, *committed.OutboxCallbackCalls)
		}
	}
	cases := []replayConflictCase{
		{
			name: "InitialOutboxRetryWithoutOutbox", withOutbox: true,
			errorContains: "outbox",
			mutate: func(
				_ *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				retry.OutboxIntent = nil
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name:          "InitialWithoutOutboxRetryWithOutbox",
			errorContains: "outbox",
			mutate: func(
				t *testing.T,
				_ *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				calls := new(int)
				retry.OutboxIntent = adaptiveVerifiedSuccessTrackedOutboxIntentForReplayTest(
					t,
					retry.Now,
					calls,
				)
				return func(t *testing.T) { require.Zero(t, *calls) }
			},
		},
		{
			name: "InitialOutboxRetryMissingDurableRow", withOutbox: true,
			errorContains: "outbox",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Where(
					"event_id = ?", committed.OutboxEvent.EventID,
				).Delete(&adaptiveVerifiedSuccessOutboxPOForTest{}).Error)
				return requireOutboxCalls(committed, 2)
			},
		},
		{
			name: "MessageIDOrBody", withOutbox: true,
			errorContains: "finalize_request_fingerprint",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				replacement := *retry.Message
				replacement.ID = 6_101
				replacement.Content = "replacement final answer"
				replacementPO, err := messageToPO(&replacement)
				require.NoError(t, err)
				require.NoError(t, committed.Fixture.DB.Create(replacementPO).Error)
				retry.Message = &replacement
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "SelectedCheckpointIDOrBody", withOutbox: true,
			errorContains: "finalize_request_fingerprint",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				replacement := *retry.TerminalCheckpoint
				replacement.ID = 8_103
				replacement.ChannelValues = `{"terminal":"replacement"}`
				fallback := *retry.TerminalCheckpointOnTitleConflict
				fallback.ID = replacement.ID
				fallback.ChannelValues = `{"terminal":"replacement","title":"preserved"}`
				replacementPO, err := checkpointToPO(&replacement)
				require.NoError(t, err)
				require.NoError(t, committed.Fixture.DB.Create(replacementPO).Error)
				retry.TerminalCheckpoint = &replacement
				retry.TerminalCheckpointOnTitleConflict = &fallback
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "UnselectedCheckpointIDOrBody", withOutbox: true,
			errorContains: "finalize_request_fingerprint",
			mutate: func(
				_ *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				fallback := *retry.TerminalCheckpointOnTitleConflict
				fallback.ChannelValues = `{"terminal":true,"title":"unselected replacement"}`
				retry.TerminalCheckpointOnTitleConflict = &fallback
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "ExpectedOrUpdatedTitle", withOutbox: true,
			errorContains: "finalize_request_fingerprint",
			mutate: func(
				_ *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				retry.ExpectedThreadTitle = "replacement expected title"
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "UnpersistedTitleEventIDOrBody", withOutbox: true, titleConflict: true,
			errorContains: "finalize_request_fingerprint",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				title := *retry.TitleEvent
				title.ID = 6_999
				title.Payload = `{"thread_title":"replacement"}`
				retry.TitleEvent = &title
				var count int64
				require.NoError(t, committed.Fixture.DB.Model(&runEventPO{}).
					Where("id IN ?", []int64{committed.Fixture.Request.TitleEvent.ID, title.ID}).
					Count(&count).Error)
				require.Zero(t, count)
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "OutboxImmutableRequestIdentity", withOutbox: true,
			errorContains: "outbox",
			mutate: func(
				_ *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				retry *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				intent := *retry.OutboxIntent
				intent.Event.AggregateVersion++
				called := false
				intent.AppendWithResult = func(
					context.Context,
					*gorm.DB,
					domainnotification.Event,
				) (bool, error) {
					called = true
					return false, nil
				}
				retry.OutboxIntent = &intent
				return func(t *testing.T) {
					require.False(t, called)
					require.Equal(t, 1, *committed.OutboxCallbackCalls)
				}
			},
		},
		{
			name: "RunPostImage", withOutbox: true,
			errorContains: "finalize_request_fingerprint",
			variants: []replayConflictVariant{
				{
					name: "Metadata",
					mutate: func(
						t *testing.T,
						committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
						_ *FinalizeRunSuccessRequest,
					) func(*testing.T) {
						require.NoError(t, committed.Fixture.DB.Model(&runPO{}).
							Where("id = ?", committed.Fixture.Request.RunID).
							UpdateColumn("metadata", []byte(`{"durable":"run-postimage-drift"}`)).Error)
						return requireOutboxCalls(committed, 1)
					},
				},
				{
					name: "Context",
					mutate: func(
						t *testing.T,
						committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
						_ *FinalizeRunSuccessRequest,
					) func(*testing.T) {
						require.NoError(t, committed.Fixture.DB.Model(&runPO{}).
							Where("id = ?", committed.Fixture.Request.RunID).
							UpdateColumn("context", []byte(`{"durable":"run-context-drift"}`)).Error)
						return requireOutboxCalls(committed, 1)
					},
				},
			},
		},
		{
			name: "AttemptPostImage", withOutbox: true,
			errorContains: "finalize_request_fingerprint",
			variants: []replayConflictVariant{
				{
					name: "TraceID",
					mutate: func(
						t *testing.T,
						committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
						_ *FinalizeRunSuccessRequest,
					) func(*testing.T) {
						require.NoError(t, committed.Fixture.DB.Model(&runAttemptPO{}).
							Where("id = ?", 100).
							UpdateColumn("trace_id", "attempt-postimage-drift").Error)
						return requireOutboxCalls(committed, 1)
					},
				},
				{
					name: "EnrollmentVersion",
					mutate: func(
						t *testing.T,
						committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
						_ *FinalizeRunSuccessRequest,
					) func(*testing.T) {
						require.NoError(t, committed.Fixture.DB.Model(&runAttemptPO{}).
							Where("id = ?", 100).
							UpdateColumn("enrollment_version", "attempt-enrollment-drift").Error)
						return requireOutboxCalls(committed, 1)
					},
				},
			},
		},
		{
			name: "DurableCompletion", withOutbox: true,
			errorContains: "completion",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Model(&runEventPO{}).
					Where("id = ?", committed.Fixture.Request.CompletionEvent.ID).
					UpdateColumn("payload", []byte(`{"status":"durable-drift"}`)).Error)
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "DurableMessage", withOutbox: true,
			errorContains: "message",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Model(&messagePO{}).
					Where("id = ?", committed.Fixture.Request.Message.ID).
					UpdateColumn("content", "durable message drift").Error)
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "DurableTitle", withOutbox: true,
			errorContains: "title",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Model(&runEventPO{}).
					Where("id = ?", committed.Fixture.Request.TitleEvent.ID).
					UpdateColumn("payload", []byte(`{"thread_title":"durable drift"}`)).Error)
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "DurableThreadTitle", withOutbox: true,
			errorContains: "title",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.True(t, committed.First.TitleUpdated)
				require.Equal(t, committed.Fixture.Request.TerminalCheckpoint, committed.First.TerminalCheckpoint)
				require.NoError(t, committed.Fixture.DB.Model(&threadPO{}).
					Where("id = ?", committed.Fixture.Request.Message.ThreadID).
					UpdateColumn("title", "durable thread title drift").Error)
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "DurableSelectedCheckpoint", withOutbox: true,
			expectedCause: ErrAdaptiveExecutionCheckpointConflict,
			errorContains: "checkpoint",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Model(&checkpointPO{}).
					Where("id = ?", committed.Fixture.Request.TerminalCheckpoint.ID).
					UpdateColumn("channel_values", []byte(`{"terminal":"durable-drift"}`)).Error)
				return requireOutboxCalls(committed, 1)
			},
		},
		{
			name: "DurableOutbox", withOutbox: true,
			expectedCause: domainnotification.ErrIdempotencyConflict,
			errorContains: "outbox",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Model(&adaptiveVerifiedSuccessOutboxPOForTest{}).
					Where("event_id = ?", committed.OutboxEvent.EventID).
					UpdateColumn("fingerprint", strings.Repeat("d", sha256.Size*2)).Error)
				return requireOutboxCalls(committed, 2)
			},
		},
		{
			name: "DurablePlanRevision", withOutbox: true,
			expectedCause: ErrAdaptiveExecutionPlanRevisionConflict,
			errorContains: "plan",
			mutate: func(
				t *testing.T,
				committed *adaptiveVerifiedSuccessCommittedReplayFixtureForTest,
				_ *FinalizeRunSuccessRequest,
			) func(*testing.T) {
				require.NoError(t, committed.Fixture.DB.Model(&agentRunPlanPO{}).
					Where("run_id = ?", committed.Fixture.Request.AdaptiveGate.Evidence.PlanScopeRunID).
					UpdateColumn("revision", committed.Fixture.Request.AdaptiveGate.Evidence.PlanRevision+1).Error)
				return requireOutboxCalls(committed, 1)
			},
		},
	}
	runConflict := func(
		t *testing.T,
		test replayConflictCase,
		mutate replayConflictMutation,
	) {
		t.Helper()
		require.NotNil(t, mutate)
		committed := prepareAdaptiveVerifiedSuccessCommittedReplayFixtureForTest(
			t,
			test.withOutbox,
			test.titleConflict,
		)
		retry := cloneAdaptiveVerifiedSuccessRequestForTest(committed.Fixture.Request)
		verify := mutate(t, &committed, &retry)
		before := snapshotAdaptiveVerifiedSuccessDBForTest(t, committed.Fixture.DB)

		result, err := (&threadRepository{db: committed.Fixture.DB}).FinalizeRunSuccess(
			context.Background(),
			retry,
		)

		require.Nil(t, result)
		require.ErrorIs(t, err, ErrAdaptiveExecutionVerifiedSuccessConflict)
		if test.expectedCause != nil {
			require.ErrorIs(t, err, test.expectedCause)
		}
		if test.errorContains != "" {
			require.ErrorContains(t, err, test.errorContains)
		}
		if verify != nil {
			verify(t)
		}
		require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, committed.Fixture.DB))
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			if len(test.variants) == 0 {
				runConflict(t, test, test.mutate)
				return
			}
			for _, variant := range test.variants {
				variant := variant
				t.Run(variant.name, func(t *testing.T) {
					runConflict(t, test, variant.mutate)
				})
			}
		})
	}
}

func p0dMySQLMockRepository(t *testing.T) (*threadRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: sqlDB, SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			mock.MatchExpectationsInOrder(false)
			mock.ExpectClose()
			_ = sqlDB.Close()
			return
		}
		mock.ExpectClose()
		require.NoError(t, sqlDB.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	return &threadRepository{db: db}, mock
}

func TestThreadRepositoryFinalizeRunSuccessAdaptiveGateLocksJournalRootBeforeExecutionRunAndAttempt(t *testing.T) {
	for _, test := range []struct {
		name           string
		shared         bool
		callerMismatch bool
	}{
		{name: "DistinctJournalAndExecutionRuns"},
		{name: "SharedJournalAndExecutionRun", shared: true},
		{name: "CallerThreadSelectorCannotPrelockUnrelatedThread", callerMismatch: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := p0dMySQLMockRepository(t)
			req := newValidAdaptiveVerifiedSuccessRequestForTest()
			if test.shared {
				req.AdaptiveGate.Decision.JournalRunID = req.RunID
				req.AdaptiveGate.Evidence.JournalRunID = req.RunID
				req.JournalEvent.JournalRunID = req.RunID
				req.AdaptiveGate.VerificationEvent.Payload = adaptiveVerifiedSuccessPayloadMutationForTest(
					t, req.AdaptiveGate.VerificationEvent.Payload,
					func(fields map[string]any) { fields["journal_run_id"] = req.RunID },
				)
			}
			if test.callerMismatch {
				req.Message.ThreadID = 11
				req.TitleEvent.ThreadID = 11
				req.CompletionEvent.ThreadID = 11
				req.JournalEvent.ThreadID = 11
				req.TerminalCheckpoint.ThreadID = 11
				req.TerminalCheckpointOnTitleConflict.ThreadID = 11
				req.AdaptiveGate.Decision.ThreadID = 11
				req.AdaptiveGate.Evidence.ThreadID = 11
				req.AdaptiveGate.VerificationEvent.ThreadID = 11
			}
			sentinel := fmt.Errorf("adaptive verification tuple lock sentinel")
			mock.ExpectBegin()
			mock.ExpectQuery("^SELECT .*FROM .*agent_runs.*WHERE id = \\?.*LIMIT \\?$").
				WithArgs(req.AdaptiveGate.Evidence.JournalRunID, 1).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "thread_id", "parent_run_id", "space_id", "creator_id", "run_kind", "status", "execution_generation",
					"lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at",
				}).AddRow(
					req.AdaptiveGate.Evidence.JournalRunID, 10, 0, 10, 20, string(entity.RunKindTask),
					string(entity.RunStatusRunning), uint64(3), "worker-1", "lease-1", int64(2_000), nil,
				))
			mock.ExpectQuery("SELECT .*FROM .*agent_threads.*FOR UPDATE").
				WithArgs(int64(10), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "space_id", "creator_id", "title"}).
					AddRow(10, 10, 20, "initial"))
			mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
				WithArgs(req.AdaptiveGate.Evidence.JournalRunID, 1).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "thread_id", "parent_run_id", "space_id", "creator_id", "run_kind", "status", "execution_generation",
					"lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at",
				}).AddRow(
					req.AdaptiveGate.Evidence.JournalRunID, 10, 0, 10, 20, string(entity.RunKindTask),
					string(entity.RunStatusRunning), uint64(3), "worker-1", "lease-1", int64(2_000), nil,
				))
			if test.callerMismatch {
				mock.ExpectRollback()
			} else {
				if !test.shared {
					mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
						WithArgs(req.RunID, 1).
						WillReturnRows(sqlmock.NewRows([]string{
							"id", "thread_id", "parent_run_id", "space_id", "creator_id", "status", "execution_generation", "lease_owner", "lease_token",
							"lease_expires_at", "cancel_requested_at",
						}).AddRow(req.RunID, 10, 0, 10, 20, string(entity.RunStatusRunning), uint64(3), "worker-1", "lease-1", int64(2_000), nil))
				}
				mock.ExpectQuery("SELECT .*FROM .*run_attempts.*FOR UPDATE").
					WithArgs(req.AdaptiveGate.Evidence.JournalRunID, "attempt-1", 1).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "thread_id", "journal_run_id", "execution_run_id", "attempt_id", "status",
						"active_slot", "next_sequence", "last_committed_sequence", "projection_state",
					}).AddRow(100, 10, req.AdaptiveGate.Evidence.JournalRunID, 20, "attempt-1",
						string(entity.RunAttemptStatusRunning), 1, 3, 2, string(entity.JournalProjectionStateHealthy)))
				mock.ExpectQuery(
					"SELECT .*FROM .*run_events.*journal_run_id.*attempt_id.*idempotency_key.*FOR UPDATE",
				).
					WithArgs(
						req.AdaptiveGate.Evidence.JournalRunID,
						req.AdaptiveGate.Evidence.AttemptID,
						req.AdaptiveGate.VerificationIdempotencyKey,
						1,
					).
					WillReturnError(sentinel)
				mock.ExpectRollback()
			}

			result, err := repo.FinalizeRunSuccess(context.Background(), req)

			require.Nil(t, result)
			if test.callerMismatch {
				if !errors.Is(err, ErrAdaptiveExecutionVerifiedSuccessConflict) ||
					!strings.Contains(err.Error(), "identity drift") {
					t.Fatalf("P0D_LOCK_ORDER_RED: caller Thread selector was not rejected by durable root identity: %v", err)
				}
			} else if !errors.Is(err, sentinel) {
				t.Fatalf("P0D_LOCK_ORDER_RED: gate-on lock prefix is not Thread-first: %v", err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestThreadRepositoryFinalizeRunSuccessGateOffLocksThreadBeforeExecutionRun(t *testing.T) {
	for _, test := range []struct {
		name           string
		callerMismatch bool
	}{
		{name: "AuthoritativeThreadPrecedesExecutionRun"},
		{name: "CallerThreadSelectorCannotPrelockUnrelatedThread", callerMismatch: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := p0dMySQLMockRepository(t)
			req := newValidAdaptiveVerifiedSuccessRequestForTest()
			req.AdaptiveGate = nil
			if test.callerMismatch {
				req.Message.ThreadID = 11
				req.TitleEvent.ThreadID = 11
				req.CompletionEvent.ThreadID = 11
				req.JournalEvent.ThreadID = 11
				req.TerminalCheckpoint.ThreadID = 11
				req.TerminalCheckpointOnTitleConflict.ThreadID = 11
			}
			sentinel := fmt.Errorf("gate-off execution fence sentinel")
			mock.ExpectBegin()
			mock.ExpectQuery("^SELECT .*FROM .*agent_runs.*WHERE id = \\?.*LIMIT \\?$").
				WithArgs(req.RunID, 1).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "thread_id", "space_id", "creator_id", "status", "execution_generation",
					"lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at",
				}).AddRow(
					req.RunID, 10, 10, 20, string(entity.RunStatusRunning), uint64(3),
					"worker-1", "lease-1", int64(2_000), nil,
				))
			mock.ExpectQuery("SELECT .*FROM .*agent_threads.*FOR UPDATE").
				WithArgs(int64(10), 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "space_id", "creator_id", "title"}).
					AddRow(10, 10, 20, "initial"))
			update := mock.ExpectExec("UPDATE .*agent_runs").WithArgs(
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
				req.RunID, string(entity.RunStatusRunning), req.LeaseOwner, req.LeaseToken,
				req.ExecutionGeneration, req.Now,
			)
			if test.callerMismatch {
				update.WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery("SELECT .*FROM .*agent_runs.*WHERE id = \\?.*LIMIT \\?").
					WithArgs(req.RunID, 1).
					WillReturnRows(sqlmock.NewRows([]string{
						"id", "thread_id", "space_id", "creator_id", "status", "execution_generation",
					}).AddRow(req.RunID, 10, 10, 20, string(entity.RunStatusSucceeded), uint64(3)))
			} else {
				update.WillReturnError(sentinel)
			}
			mock.ExpectRollback()

			result, err := repo.FinalizeRunSuccess(context.Background(), req)

			require.Nil(t, result)
			if test.callerMismatch {
				if err == nil || !strings.Contains(err.Error(), "thread") {
					t.Fatalf("P0D_LOCK_ORDER_RED: gate-off caller Thread selector was not rejected after durable Thread lock: %v", err)
				}
			} else if !errors.Is(err, sentinel) {
				t.Fatalf("P0D_LOCK_ORDER_RED: gate-off lock prefix is not Thread-first: %v", err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestThreadRepositoryCreateRunBundleRecoveryLocksExecutionRunBeforeSourceAttempt(t *testing.T) {
	repo, mock := p0dMySQLMockRepository(t)
	recoveryKey := "recovery-21"
	sourceCheckpointID := int64(8001)
	sourceAttemptID := "attempt-1"
	run := newRepositoryTestRun(21, 10, entity.RunStatusQueued, 1_200)
	run.SpaceID = 10
	run.CreatorID = 20
	run.RunKind = entity.RunKindTask
	run.IdempotencyKey = recoveryKey
	req := CreateRunBundleRequest{
		Run: run,
		Attempt: &entity.RunAttempt{
			ID: 101, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 21,
			AttemptID: "attempt-2", Status: entity.RunAttemptStatusPending,
			SourceCheckpointID: &sourceCheckpointID, SourceAttemptID: &sourceAttemptID,
			RecoveryIdempotencyKey: &recoveryKey,
			ProjectionState:        entity.JournalProjectionStateHealthy,
		},
		RecoverySourceLease: &ReconcileExpiredRunLeaseRequest{
			RunID: 20, LeaseOwner: "worker-1", LeaseToken: "lease-1",
			ExecutionGeneration: 3, ToStatus: entity.RunStatusFailed, Now: 2_100,
			ErrorCode: "run_recovered", ErrorMessage: "recovered",
			Event: &entity.RunEvent{
				ID: 7100, ThreadID: 10, RunID: 20, EventType: "run.failed",
				Payload: `{"status":"failed","error_code":"run_recovered"}`, CreatedAt: 2_100,
			},
			JournalEvent: &entity.JournalEvent{
				ID: 7100, ThreadID: 10, RunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
				IdempotencyKey: "recovery-source-failed", SchemaVersion: entity.JournalSchemaVersion,
				Status: string(entity.RunAttemptStatusFailed), Visibility: entity.JournalVisibilityUser,
				PayloadVersion: entity.JournalPayloadVersion, EventType: "run.lifecycle",
				Payload:            `{"type":"terminal","data":{"status":"failed"}}`,
				OccurredAtUnixNano: 2_100 * int64(time.Millisecond), CreatedAt: 2_100,
			},
		},
	}
	sentinel := fmt.Errorf("recovery source checkpoint lock sentinel")
	mock.MatchExpectationsInOrder(false)
	type observedRecoveryQuery struct {
		table   string
		firstID any
		locked  bool
	}
	observed := make([]observedRecoveryQuery, 0, 8)
	const callbackName = "p0d:observe_recovery_source_lock_order"
	require.NoError(t, repo.db.Callback().Query().After("gorm:query").Register(
		callbackName,
		func(tx *gorm.DB) {
			if tx == nil || tx.Statement == nil {
				return
			}
			var firstID any
			if len(tx.Statement.Vars) > 0 {
				firstID = tx.Statement.Vars[0]
			}
			observed = append(observed, observedRecoveryQuery{
				table: tx.Statement.Table, firstID: firstID,
				locked: strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), "FOR UPDATE"),
			})
		},
	))
	t.Cleanup(func() {
		require.NoError(t, repo.db.Callback().Query().Remove(callbackName))
	})
	mock.ExpectBegin()
	for index := 0; index < 3; index++ {
		mock.ExpectQuery("SELECT .*FROM .*agent_runs.*space_id.*idempotency_key").
			WithArgs(int64(10), recoveryKey, 1).WillReturnError(gorm.ErrRecordNotFound)
	}
	mock.ExpectQuery("SELECT .*FROM .*agent_threads.*FOR UPDATE").
		WithArgs(int64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "space_id", "creator_id"}).AddRow(10, 10, 20))
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
		WithArgs(int64(30), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "parent_run_id", "space_id", "creator_id", "run_kind", "status",
		}).AddRow(30, 10, 0, 10, 20, string(entity.RunKindTask), string(entity.RunStatusSucceeded)))
	mock.ExpectQuery("^SELECT .*FROM .*agent_run_attempts.*journal_run_id.*attempt_id.*LIMIT \\?$").
		WithArgs(int64(30), "attempt-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "journal_run_id", "execution_run_id", "attempt_id", "status",
			"active_slot", "next_sequence", "last_committed_sequence",
		}).AddRow(100, 10, 30, 20, "attempt-1", string(entity.RunAttemptStatusRunning), 1, 2, 1))
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
		WithArgs(int64(20), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "space_id", "creator_id", "status", "execution_generation",
			"lease_owner", "lease_token", "lease_expires_at",
		}).AddRow(20, 10, 10, 20, string(entity.RunStatusRunning), uint64(3), "worker-1", "lease-1", int64(2_000)))
	mock.ExpectQuery("SELECT .*FROM .*agent_run_attempts.*FOR UPDATE").
		WithArgs(int64(30), "attempt-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "journal_run_id", "execution_run_id", "attempt_id", "status",
			"active_slot", "next_sequence", "last_committed_sequence",
		}).AddRow(100, 10, 30, 20, "attempt-1", string(entity.RunAttemptStatusRunning), 1, 2, 1))
	mock.ExpectQuery("SELECT .*FROM .*agent_checkpoints.*FOR UPDATE").
		WithArgs(sourceCheckpointID, 1).WillReturnError(sentinel)
	mock.ExpectRollback()

	result, err := repo.CreateRunBundle(context.Background(), req)

	require.Nil(t, result)
	plainAttemptIndex, sourceRunIndex, lockedAttemptIndex := -1, -1, -1
	for index, query := range observed {
		switch {
		case query.table == "agent_run_attempts" && !query.locked:
			plainAttemptIndex = index
		case query.table == "agent_runs" && query.locked && fmt.Sprint(query.firstID) == "20":
			sourceRunIndex = index
		case query.table == "agent_run_attempts" && query.locked:
			lockedAttemptIndex = index
		}
	}
	if !errors.Is(err, sentinel) || plainAttemptIndex < 0 || sourceRunIndex < 0 ||
		lockedAttemptIndex < 0 || plainAttemptIndex >= sourceRunIndex || sourceRunIndex >= lockedAttemptIndex {
		t.Fatalf(
			"P0D_LOCK_ORDER_RED: recovery source order plain-attempt=%d execution-run=%d locked-attempt=%d observed=%v err=%v",
			plainAttemptIndex, sourceRunIndex, lockedAttemptIndex, observed, err,
		)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestThreadRepositoryFinalizeRunSuccessRollsBackAdaptiveVerificationOnLateOutboxFailure(t *testing.T) {
	fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
	sentinel := fmt.Errorf("late adaptive outbox sentinel")
	callbackObserved := false
	outboxEvent := adaptiveVerifiedSuccessOutboxEventForTest(fixture.Request.Now)
	outboxFingerprint := adaptiveVerifiedSuccessOutboxFingerprintOracleForTest(t, outboxEvent)
	fixture.Request.OutboxIntent = &NotificationOutboxIntent{
		Event: outboxEvent,
		Append: func(context.Context, *gorm.DB, domainnotification.Event) error {
			return fmt.Errorf("legacy append must not be used")
		},
		AppendWithResult: func(_ context.Context, tx *gorm.DB, _ domainnotification.Event) (bool, error) {
			var run runPO
			if err := tx.Where("id = ?", fixture.Request.RunID).First(&run).Error; err != nil {
				return false, fmt.Errorf("callback cannot see terminal run: %w", err)
			}
			if entity.RunStatus(run.Status) != entity.RunStatusSucceeded {
				return false, fmt.Errorf("callback saw run status %s", run.Status)
			}
			var attempt runAttemptPO
			if err := tx.Where("id = ?", 100).First(&attempt).Error; err != nil {
				return false, fmt.Errorf("callback cannot see terminal attempt: %w", err)
			}
			if attempt.Status != string(entity.RunAttemptStatusCompleted) || attempt.ActiveSlot != nil ||
				attempt.NextSequence != fixture.Evidence.Authority.EventSequence+3 ||
				attempt.LastCommittedSequence != fixture.Evidence.Authority.EventSequence+1 ||
				attempt.TerminalEventID == nil || *attempt.TerminalEventID != fixture.Request.CompletionEvent.ID ||
				attempt.EndedAt == nil || *attempt.EndedAt != fixture.Request.Now || attempt.UpdatedAt != fixture.Request.Now {
				return false, fmt.Errorf("callback saw incomplete terminal attempt")
			}
			var message messagePO
			if err := tx.Where("id = ?", fixture.Request.Message.ID).First(&message).Error; err != nil ||
				message.Content != fixture.Request.Message.Content {
				return false, fmt.Errorf("callback cannot see final message: %w", err)
			}
			var verification, completion runEventPO
			if err := tx.Where("id = ?", fixture.Request.AdaptiveGate.VerificationEvent.ID).
				First(&verification).Error; err != nil {
				return false, fmt.Errorf("callback cannot see verification: %w", err)
			}
			if err := tx.Where("id = ?", fixture.Request.CompletionEvent.ID).
				First(&completion).Error; err != nil {
				return false, fmt.Errorf("callback cannot see completion: %w", err)
			}
			if verification.Sequence == nil || completion.Sequence == nil ||
				*verification.Sequence >= *completion.Sequence {
				return false, fmt.Errorf("callback saw invalid event order")
			}
			var checkpoint checkpointPO
			if err := tx.Where("id = ?", fixture.Request.TerminalCheckpoint.ID).
				First(&checkpoint).Error; err != nil ||
				checkpoint.ParentCheckpointID != fixture.Evidence.Authority.CheckpointID {
				return false, fmt.Errorf("callback cannot see selected checkpoint: %w", err)
			}
			var thread threadPO
			if err := tx.Where("id = ?", int64(10)).First(&thread).Error; err != nil ||
				thread.Title != fixture.Request.ThreadTitle {
				return false, fmt.Errorf("callback cannot see committed title: %w", err)
			}
			outboxRow := adaptiveVerifiedSuccessOutboxPOForTest{
				EventID: outboxEvent.EventID, Fingerprint: outboxFingerprint,
			}
			if err := tx.Create(&outboxRow).Error; err != nil {
				return false, err
			}
			callbackObserved = true
			return false, sentinel
		},
	}
	for _, absent := range []struct {
		model any
		id    int64
	}{
		{model: &messagePO{}, id: fixture.Request.Message.ID},
		{model: &runEventPO{}, id: fixture.Request.TitleEvent.ID},
		{model: &runEventPO{}, id: fixture.Request.AdaptiveGate.VerificationEvent.ID},
		{model: &runEventPO{}, id: fixture.Request.CompletionEvent.ID},
		{model: &checkpointPO{}, id: fixture.Request.TerminalCheckpoint.ID},
	} {
		var count int64
		require.NoError(t, fixture.DB.Model(absent.model).Where("id = ?", absent.id).Count(&count).Error)
		require.Zero(t, count)
	}
	before := snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB)

	result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

	require.Nil(t, result)
	require.ErrorIs(t, err, sentinel)
	require.True(t, callbackObserved)
	require.Equal(t, before, snapshotAdaptiveVerifiedSuccessDBForTest(t, fixture.DB))
}

func TestThreadRepositoryFinalizeRunSuccessAdaptiveGateAndCancellationHaveSingleTerminalOutcome(t *testing.T) {
	t.Run("CancelFirst", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)
		cancelEvent := &entity.RunEvent{
			ID: 7100, ThreadID: 10, RunID: 20, EventType: "run.canceled", Payload: `{}`, CreatedAt: 1_150,
		}
		cancelJournal := &entity.JournalEvent{
			ID: 7100, ThreadID: 10, RunID: 20, JournalRunID: 30, AttemptID: "attempt-1",
			IdempotencyKey: "cancel-1", EventType: "run.lifecycle",
			Status: string(entity.RunAttemptStatusCancelled), Visibility: entity.JournalVisibilityUser,
			Payload:            `{"type":"terminal","data":{"status":"cancelled"}}`,
			OccurredAtUnixNano: 1_150 * int64(time.Millisecond), CreatedAt: 1_150,
		}
		_, err := fixture.Repo.RequestRunCancellation(context.Background(), RequestRunCancellationRequest{
			RunID: 20, Now: 1_150, Event: cancelEvent, JournalEvent: cancelJournal,
		})
		require.NoError(t, err)

		result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)

		require.Nil(t, result)
		require.ErrorIs(t, err, ErrRunCanceled)
		var verificationCount, completionCount, canceledCount int64
		require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("event_type = ?", "adaptive.verification").Count(&verificationCount).Error)
		require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("event_type = ?", "run.completed").Count(&completionCount).Error)
		require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("event_type = ?", "run.canceled").Count(&canceledCount).Error)
		require.Zero(t, verificationCount)
		require.Zero(t, completionCount)
		require.Equal(t, int64(1), canceledCount)
		var terminalEvents []runEventPO
		require.NoError(t, fixture.DB.Where("event_type IN ?", []string{
			"run.completed", "run.succeeded", "run.failed", "run.canceled", "run.cancelled", "run.timed_out",
		}).Order("id ASC").Find(&terminalEvents).Error)
		require.Len(t, terminalEvents, 1)
		require.Equal(t, "run.canceled", terminalEvents[0].EventType)
		require.Equal(t, int64(7100), terminalEvents[0].ID)
	})

	t.Run("SuccessFirst", func(t *testing.T) {
		fixture := prepareAdaptiveVerifiedSuccessFixtureForTest(t)

		result, err := fixture.Repo.FinalizeRunSuccess(context.Background(), fixture.Request)
		require.NoError(t, err)
		require.NotNil(t, result.VerificationEvent)
		_, cancelErr := fixture.Repo.RequestRunCancellation(context.Background(), RequestRunCancellationRequest{
			RunID: 20, Now: 1_300,
			Event: &entity.RunEvent{
				ID: 7100, ThreadID: 10, RunID: 20, EventType: "run.canceled", Payload: `{}`, CreatedAt: 1_300,
			},
		})
		require.Error(t, cancelErr)
		var verificationCount, completionCount, canceledCount int64
		require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("event_type = ?", "adaptive.verification").Count(&verificationCount).Error)
		require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("event_type = ?", "run.completed").Count(&completionCount).Error)
		require.NoError(t, fixture.DB.Model(&runEventPO{}).Where("event_type = ?", "run.canceled").Count(&canceledCount).Error)
		require.Equal(t, int64(1), verificationCount)
		require.Equal(t, int64(1), completionCount)
		require.Zero(t, canceledCount)
	})
}

func TestAdaptiveExecutionPlanItemFingerprintUsesCanonicalJSON(t *testing.T) {
	items := []*entity.AgentRunPlanItem{
		{
			ID: 60, RunID: 20, TaskID: 1, Subject: "step one", Description: "first",
			Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
			Blocks: `{"z":2,"a":1}`, BlockedBy: `[{"b":2,"a":1}]`,
			Metadata: `{"nested":{"z":2,"a":1}}`, Active: true, Version: 2,
			CreatedAt: 900, UpdatedAt: 1000,
		},
		{
			ID: 61, RunID: 20, TaskID: 2, Subject: "step two", Description: "second",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
			Blocks: `[]`, BlockedBy: `[1,2]`, Metadata: `{}`, Active: true, Version: 1,
			CreatedAt: 1000, UpdatedAt: 1000,
		},
	}

	fingerprint, err := adaptiveExecutionPlanItemFingerprint(items)
	require.NoError(t, err)

	reordered := cloneAdaptiveItemsForTest(items)
	reordered[0].Blocks = `{ "a": 1, "z": 2 }`
	reordered[0].BlockedBy = `[{"a":1,"b":2}]`
	reordered[0].Metadata = `{"nested":{"a":1,"z":2}}`
	reorderedFingerprint, err := adaptiveExecutionPlanItemFingerprint(reordered)
	require.NoError(t, err)
	require.Equal(t, fingerprint, reorderedFingerprint)

	changedValue := cloneAdaptiveItemsForTest(items)
	changedValue[0].Metadata = `{"nested":{"a":1,"z":3}}`
	changedFingerprint, err := adaptiveExecutionPlanItemFingerprint(changedValue)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, changedFingerprint)

	changedArrayOrder := cloneAdaptiveItemsForTest(items)
	changedArrayOrder[1].BlockedBy = `[2,1]`
	changedFingerprint, err = adaptiveExecutionPlanItemFingerprint(changedArrayOrder)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, changedFingerprint)

	changedContent := cloneAdaptiveItemsForTest(items)
	changedContent[1].Subject = "different"
	changedFingerprint, err = adaptiveExecutionPlanItemFingerprint(changedContent)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, changedFingerprint)

	for _, malformed := range []string{`{"a":`, `{} trailing`} {
		invalid := cloneAdaptiveItemsForTest(items)
		invalid[0].Blocks = malformed
		_, err := adaptiveExecutionPlanItemFingerprint(invalid)
		require.Error(t, err)
	}
}

func TestAdaptiveExecutionCheckpointFingerprintBindsPhysicalRowCanonically(t *testing.T) {
	base, err := checkpointToPO(&entity.Checkpoint{
		ID: 8001, ThreadID: 10, RunID: 20, ParentCheckpointID: 7999,
		CheckpointNS: "adaptive", RuntimeType: "eino_adk", RuntimeKey: "runtime-key",
		EnvelopeVersion: 2, RuntimeDeletedAt: 3,
		ChannelValues:   `{"z":2,"a":1}`,
		ChannelVersions: `{"messages":1,"state":2}`,
		PendingSends:    `[{"z":2,"a":1},2]`,
		Metadata:        `{"user":{"z":2,"a":1}}`,
		CreatedAt:       1000,
	})
	require.NoError(t, err)
	authority := adaptiveExecutionCheckpointMetadata{
		SchemaVersion: adaptiveExecutionCheckpointSchemaVersion,
		EventID:       7001, EventSequence: 1, JournalRunID: 30, AttemptID: "attempt-1",
		EventIdempotencyKey: "boundary-1",
		EventFingerprint:    strings.Repeat("a", 64), CheckpointFingerprint: strings.Repeat("b", 64),
		PlanScopeRunID: 20, PlanRevision: 2, ItemFingerprint: strings.Repeat("c", 64),
		ItemRefs: []adaptiveExecutionCheckpointItemRef{
			{ID: 60, TaskID: 1, Version: 2}, {ID: 61, TaskID: 2, Version: 1},
		},
		ExecutionRunID: 20, ExecutionGeneration: 3,
	}
	fingerprint, err := adaptiveExecutionCheckpointFingerprint(base, &authority)
	require.NoError(t, err)
	require.True(t, validAdaptiveExecutionFingerprint(fingerprint))

	equivalent := *base
	equivalent.ChannelValues = []byte(` { "a": 1, "z": 2 } `)
	equivalent.ChannelVersions = []byte(`{"state":2,"messages":1}`)
	equivalent.PendingSends = []byte(`[ { "a": 1, "z": 2 }, 2 ]`)
	equivalent.Metadata = []byte(`{
		"adaptive_execution":{"checkpoint_fingerprint":"self-is-excluded","event_id":999},
		"user":{"a":1,"z":2}
	}`)
	equivalentFingerprint, err := adaptiveExecutionCheckpointFingerprint(&equivalent, &authority)
	require.NoError(t, err)
	require.Equal(t, fingerprint, equivalentFingerprint)

	digestOnlyChange := authority
	digestOnlyChange.EventFingerprint = strings.Repeat("d", 64)
	digestOnlyChange.CheckpointFingerprint = strings.Repeat("e", 64)
	digestOnlyFingerprint, err := adaptiveExecutionCheckpointFingerprint(base, &digestOnlyChange)
	require.NoError(t, err)
	require.Equal(t, fingerprint, digestOnlyFingerprint)

	authorityChange := authority
	authorityChange.ItemRefs = []adaptiveExecutionCheckpointItemRef{{ID: 60, TaskID: 1, Version: 2}}
	authorityChange.ItemFingerprint = strings.Repeat("f", 64)
	authorityFingerprint, err := adaptiveExecutionCheckpointFingerprint(base, &authorityChange)
	require.NoError(t, err)
	require.NotEqual(t, fingerprint, authorityFingerprint)

	mutations := []struct {
		name   string
		mutate func(*checkpointPO)
	}{
		{name: "id", mutate: func(checkpoint *checkpointPO) { checkpoint.ID++ }},
		{name: "thread id", mutate: func(checkpoint *checkpointPO) { checkpoint.ThreadID++ }},
		{name: "run id", mutate: func(checkpoint *checkpointPO) { checkpoint.RunID++ }},
		{name: "parent id", mutate: func(checkpoint *checkpointPO) { checkpoint.ParentCheckpointID++ }},
		{name: "namespace", mutate: func(checkpoint *checkpointPO) { checkpoint.CheckpointNS += ".changed" }},
		{name: "runtime type", mutate: func(checkpoint *checkpointPO) { checkpoint.RuntimeType += ".changed" }},
		{name: "runtime key", mutate: func(checkpoint *checkpointPO) { checkpoint.RuntimeKey += ".changed" }},
		{name: "envelope version", mutate: func(checkpoint *checkpointPO) { checkpoint.EnvelopeVersion++ }},
		{name: "runtime deleted at", mutate: func(checkpoint *checkpointPO) { checkpoint.RuntimeDeletedAt++ }},
		{name: "created at", mutate: func(checkpoint *checkpointPO) { checkpoint.CreatedAt++ }},
		{name: "channel values", mutate: func(checkpoint *checkpointPO) { checkpoint.ChannelValues = []byte(`{"a":2,"z":2}`) }},
		{name: "channel versions", mutate: func(checkpoint *checkpointPO) { checkpoint.ChannelVersions = []byte(`{"messages":2,"state":2}`) }},
		{name: "pending send array order", mutate: func(checkpoint *checkpointPO) { checkpoint.PendingSends = []byte(`[2,{"a":1,"z":2}]`) }},
		{name: "user metadata", mutate: func(checkpoint *checkpointPO) { checkpoint.Metadata = []byte(`{"user":{"a":2,"z":2}}`) }},
	}
	for _, tt := range mutations {
		t.Run("binds "+tt.name, func(t *testing.T) {
			changed := *base
			tt.mutate(&changed)
			changedFingerprint, fingerprintErr := adaptiveExecutionCheckpointFingerprint(&changed, &authority)
			require.NoError(t, fingerprintErr)
			require.NotEqual(t, fingerprint, changedFingerprint)
		})
	}

	for _, tt := range []struct {
		name   string
		mutate func(*checkpointPO)
	}{
		{name: "channel values", mutate: func(checkpoint *checkpointPO) { checkpoint.ChannelValues = []byte(`{"a":1} trailing`) }},
		{name: "channel versions", mutate: func(checkpoint *checkpointPO) { checkpoint.ChannelVersions = []byte(`{"a":`) }},
		{name: "pending sends", mutate: func(checkpoint *checkpointPO) { checkpoint.PendingSends = []byte(`[1] trailing`) }},
		{name: "metadata", mutate: func(checkpoint *checkpointPO) { checkpoint.Metadata = []byte(`[]`) }},
	} {
		t.Run("rejects malformed "+tt.name, func(t *testing.T) {
			changed := *base
			tt.mutate(&changed)
			_, fingerprintErr := adaptiveExecutionCheckpointFingerprint(&changed, &authority)
			require.Error(t, fingerprintErr)
		})
	}
}

func TestAdaptiveExecutionBoundaryCommitsInitialMutationAtomically(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)

	result, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, uint64(1), result.LastCommittedSequence)
	require.Equal(t, []int64{1, 2}, adaptiveTaskIDsForTest(result.Items))

	expectedItems := []*entity.AgentRunPlanItem{
		{
			ID: 60, RunID: 20, TaskID: 1, Subject: "updated major step", Description: "updated description",
			Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
			Blocks: `{"z":2,"a":1}`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
			Active: true, Version: 2, CreatedAt: 900, UpdatedAt: 1000,
		},
		{
			ID: 61, RunID: 20, TaskID: 2, Subject: "new major step", Description: "new description",
			Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
			Blocks: `[]`, BlockedBy: `[1]`, Metadata: `{"substeps_ref":"lazy:2"}`,
			Active: true, Version: 1, CreatedAt: 1000, UpdatedAt: 1000,
		},
	}
	require.Equal(t, expectedItems, result.Items)
	require.Equal(t, &entity.RunEvent{
		ID: 7001, ThreadID: 10, RunID: 20, EventType: "run.boundary",
		Payload: `{"step":1}`, CreatedAt: 1000,
	}, result.Event)
	require.Equal(t, &entity.AgentRunPlan{
		RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
		HighWatermark: 2, Revision: 2, CreatedAt: 800, UpdatedAt: 1000,
	}, result.Plan)

	var event runEventPO
	require.NoError(t, db.Where("id = ?", 7001).First(&event).Error)
	require.Equal(t, int64(30), requireInt64PointerForTest(t, event.JournalRunID))
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, event.AttemptID))
	require.Equal(t, uint64(1), requireUint64PointerForTest(t, event.Sequence))
	require.Equal(t, "boundary-1", requireStringPointerForTest(t, event.IdempotencyKey))

	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&checkpoint).Error)
	require.Equal(t, result.Checkpoint, checkpoint.toEntity())
	metadata := decodeAdaptiveExecutionMetadataForTest(t, checkpoint.Metadata)
	require.Equal(t, "workbench-adaptive-boundary.v2", metadata.SchemaVersion)
	require.Equal(t, int64(7001), metadata.EventID)
	require.Equal(t, uint64(1), metadata.EventSequence)
	require.Equal(t, int64(30), metadata.JournalRunID)
	require.Equal(t, "attempt-1", metadata.AttemptID)
	require.Equal(t, int64(20), metadata.PlanScopeRunID)
	require.Equal(t, int64(2), metadata.PlanRevision)
	require.Equal(t, "3855b0cfab3915e2fe2a588e3155382777d4e8b87922bde23fdd278f2bcdb7db", metadata.ItemFingerprint)
	require.Equal(t, int64(20), metadata.ExecutionRunID)
	require.Equal(t, uint64(3), metadata.ExecutionGeneration)

	var plan agentRunPlanPO
	require.NoError(t, db.Where("run_id = ?", 20).First(&plan).Error)
	require.Equal(t, result.Plan, plan.toEntity())
	var itemPOs []agentRunPlanItemPO
	require.NoError(t, db.Where("run_id = ?", 20).Order("task_id ASC").Find(&itemPOs).Error)
	require.Len(t, itemPOs, 2)
	storedItems := []*entity.AgentRunPlanItem{itemPOs[0].toEntity(), itemPOs[1].toEntity()}
	require.Equal(t, expectedItems, storedItems)

	var attempt runAttemptPO
	require.NoError(t, db.Where("journal_run_id = ? AND attempt_id = ?", 30, "attempt-1").First(&attempt).Error)
	require.Equal(t, uint64(2), attempt.NextSequence)
	require.Equal(t, uint64(1), attempt.LastCommittedSequence)
	require.Equal(t, int64(1000), attempt.UpdatedAt)
}

func TestAdaptiveExecutionBoundaryReplaysLostResponseWithoutWrites(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)

	first, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	require.False(t, first.Replayed)

	cancelRequestedAt := int64(1050)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", req.ExecutionRunID).Updates(map[string]any{
		"status":              string(entity.RunStatusSucceeded),
		"lease_owner":         "replacement-worker",
		"lease_token":         "replacement-lease",
		"lease_expires_at":    int64(999),
		"cancel_requested_at": cancelRequestedAt,
	}).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("journal_run_id = ? AND attempt_id = ?", req.JournalRunID, req.AttemptID).Updates(map[string]any{
		"status":                  string(entity.RunAttemptStatusCompleted),
		"active_slot":             nil,
		"next_sequence":           uint64(10),
		"last_committed_sequence": uint64(9),
	}).Error)
	beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)

	expected := *first
	expected.Replayed = true
	for _, candidate := range []AdaptiveExecutionRepository{repo, NewAdaptiveExecutionRepository(db)} {
		replayed, replayErr := candidate.CommitAdaptiveExecutionBoundary(context.Background(), req)
		require.NoError(t, replayErr)
		require.Equal(t, &expected, replayed)
		require.Equal(t, uint64(1), replayed.LastCommittedSequence)
		require.Equal(t, uint64(1), replayed.Authority.EventSequence)
		require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
	}
}

func TestAdaptiveExecutionBoundaryRejectsReplayPayloadAndMutationDrift(t *testing.T) {
	conflicts := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "event id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.ID++ }},
		{name: "event type", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.EventType = "adaptive.progress" }},
		{name: "event payload", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.Payload = `{"step":2}` }},
		{name: "boundary time", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Now++
			req.Event.CreatedAt++
			req.Checkpoint.CreatedAt++
		}},
		{name: "checkpoint id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ID++ }},
		{name: "checkpoint namespace", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.CheckpointNS += ".changed" }},
		{name: "checkpoint runtime key", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeKey += ":changed" }},
		{name: "checkpoint envelope", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.EnvelopeVersion++ }},
		{name: "checkpoint parent", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ParentCheckpointID = 7999 }},
		{name: "channel values", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelValues = `{"changed":true}` }},
		{name: "channel versions", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelVersions = `{"messages":2}` }},
		{name: "pending sends", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.PendingSends = `[1]` }},
		{name: "user metadata", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Checkpoint.Metadata = `{"runtime_field":"changed"}`
		}},
		{name: "plan next revision", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.PlanMutation.ExpectedRevision = 2
			req.PlanMutation.NextRevision = 3
		}},
		{name: "item id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.ID++ }},
		{name: "item task id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.TaskID++ }},
		{name: "item subject", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.PlanMutation.Items[1].NextItem.Subject += " changed"
		}},
		{name: "item version", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.PlanMutation.Items[1].ExpectedVersion = 2
			req.PlanMutation.Items[1].NextItem.Version = 3
		}},
		{name: "execution generation", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Generation++ }},
	}
	for _, test := range conflicts {
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			repo := NewAdaptiveExecutionRepository(db)
			_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.NoError(t, err)
			beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)
			retry := cloneAdaptiveExecutionBoundaryRequestForReplayTest(req)
			test.mutate(&retry)
			_, err = repo.CommitAdaptiveExecutionBoundary(context.Background(), retry)
			require.ErrorIs(t, err, ErrAdaptiveExecutionReplayConflict)
			require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}

	persistedDrift := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB)
	}{
		{name: "persisted plan revision advance", mutate: func(t *testing.T, db *gorm.DB) {
			t.Helper()
			require.NoError(t, db.Model(&agentRunPlanPO{}).Where("run_id = ?", 20).Updates(map[string]any{
				"revision": int64(3), "updated_at": int64(1100),
			}).Error)
		}},
		{name: "persisted referenced item post-image and version advance", mutate: func(t *testing.T, db *gorm.DB) {
			t.Helper()
			require.NoError(t, db.Model(&agentRunPlanItemPO{}).Where("id = ?", 60).Updates(map[string]any{
				"subject": "advanced after boundary", "version": int64(3), "updated_at": int64(1100),
			}).Error)
		}},
	}
	for _, test := range persistedDrift {
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			repo := NewAdaptiveExecutionRepository(db)
			_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.NoError(t, err)
			test.mutate(t, db)
			beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)

			_, err = repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionReplayConflict)
			require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}

	equivalent := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "channel values formatting", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelValues = `{ }` }},
		{name: "channel versions formatting", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelVersions = `{ }` }},
		{name: "pending sends formatting", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.PendingSends = `[ ]` }},
		{name: "user metadata formatting", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Checkpoint.Metadata = `{ "runtime_field" : "preserved" }`
		}},
		{name: "event payload formatting", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.Payload = `{ "step" : 1 }` }},
		{name: "item JSON formatting", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.PlanMutation.Items[1].NextItem.Blocks = `{ "a" : 1, "z" : 2 }`
		}},
		{name: "transient lease credentials", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.LeaseOwner = "replacement-worker"
			req.LeaseToken = "replacement-lease"
		}},
	}
	for _, test := range equivalent {
		t.Run(test.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			repo := NewAdaptiveExecutionRepository(db)
			first, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.NoError(t, err)
			beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)
			retry := cloneAdaptiveExecutionBoundaryRequestForReplayTest(req)
			test.mutate(&retry)
			replayed, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), retry)
			require.NoError(t, err)
			expected := *first
			expected.Replayed = true
			require.Equal(t, &expected, replayed)
			require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsClonedCheckpointReplay(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	repo := NewAdaptiveExecutionRepository(db)
	_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)

	var original checkpointPO
	require.NoError(t, db.Where("id = ?", req.Checkpoint.ID).First(&original).Error)
	cloned := original
	cloned.ID = 8998
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &cloned)
	require.NoError(t, db.Create(&cloned).Error)
	retry := cloneAdaptiveExecutionBoundaryRequestForReplayTest(req)
	retry.Checkpoint.ID = cloned.ID
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), retry)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionReplayConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsInPlaceCheckpointReplayDrift(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	repo := NewAdaptiveExecutionRepository(db)
	_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)

	changedChannelValues := []byte(`{"messages":[2]}`)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", req.Checkpoint.ID).
		Update("channel_values", changedChannelValues).Error)
	var changedCheckpoint checkpointPO
	require.NoError(t, db.Where("id = ?", req.Checkpoint.ID).First(&changedCheckpoint).Error)
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &changedCheckpoint)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", req.Checkpoint.ID).
		Update("metadata", changedCheckpoint.Metadata).Error)
	retry := cloneAdaptiveExecutionBoundaryRequestForReplayTest(req)
	retry.Checkpoint.ChannelValues = string(changedChannelValues)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), retry)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionReplayConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryDuplicateDecisionConflicts(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Event.EventType = "adaptive.decision"
	req.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision":"multi_step"}`
	repo := NewAdaptiveExecutionRepository(db)
	_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)
	retry := cloneAdaptiveExecutionBoundaryRequestForReplayTest(req)
	retry.Event.Payload = `{"schema":"workbench-adaptive-decision.v1","decision":"direct"}`
	_, err = repo.CommitAdaptiveExecutionBoundary(context.Background(), retry)
	require.ErrorIs(t, err, ErrAdaptiveExecutionReplayConflict)
	require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryDuplicateVerificationReplaysAfterRepositoryReload(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Event.EventType = "adaptive.verification"
	req.Event.Payload = `{"schema":"workbench-adaptive-verification.v1","status":"blocked"}`
	first, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)
	replayed, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	expected := *first
	expected.Replayed = true
	require.Equal(t, &expected, replayed)
	require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryReplaySurvivesDegradedProjection(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	first, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	require.NoError(t, db.Model(&runAttemptPO{}).Where("journal_run_id = ? AND attempt_id = ?", req.JournalRunID, req.AttemptID).
		Update("projection_state", string(entity.JournalProjectionStateDegraded)).Error)
	var storedEvent runEventPO
	require.NoError(t, db.Where("id = ?", req.Event.ID).First(&storedEvent).Error)
	require.Nil(t, storedEvent.JournalEventType)
	require.Empty(t, storedEvent.JournalPayload)
	beforeReplay := snapshotAdaptiveExecutionDBForTest(t, db)
	replayed, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	expected := *first
	expected.Replayed = true
	require.Equal(t, &expected, replayed)
	require.Equal(t, beforeReplay, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryChecksReplayTupleAfterRunAndAttemptLocks(t *testing.T) {
	repo, mock := canonicalMySQLMockRepository(t)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	sentinel := fmt.Errorf("event tuple sentinel")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
		WithArgs(req.JournalRunID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "parent_run_id", "run_kind",
		}).AddRow(req.JournalRunID, req.ThreadID, 0, string(entity.RunKindTask)))
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{
		"id", "thread_id", "status", "execution_generation", "lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at",
	}).AddRow(20, 10, string(entity.RunStatusRunning), 3, "worker-1", "lease-1", 2000, nil))
	mock.ExpectQuery("SELECT .*FROM .*run_attempts.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{
		"id", "thread_id", "journal_run_id", "execution_run_id", "attempt_id", "status", "active_slot", "next_sequence", "last_committed_sequence",
	}).AddRow(100, 10, 30, 20, "attempt-1", string(entity.RunAttemptStatusRunning), 1, 1, 0))
	mock.ExpectQuery("SELECT .*FROM .*run_events.*FOR UPDATE").WillReturnError(sentinel)
	mock.ExpectRollback()
	_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.ErrorIs(t, err, sentinel)
}

func TestAdaptiveExecutionBoundaryLocksJournalRootBeforeExecutionRun(t *testing.T) {
	repo, mock := canonicalMySQLMockRepository(t)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	sentinel := fmt.Errorf("execution run lock sentinel")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
		WithArgs(req.JournalRunID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "parent_run_id", "run_kind",
		}).AddRow(req.JournalRunID, req.ThreadID, 0, string(entity.RunKindTask)))
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
		WithArgs(req.ExecutionRunID, 1).
		WillReturnError(sentinel)
	mock.ExpectRollback()

	result, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.Nil(t, result)
	require.ErrorIs(t, err, sentinel)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdaptiveExecutionBoundaryLocksSourceRunBeforeSourceAttempt(t *testing.T) {
	repo, mock := canonicalMySQLMockRepository(t)
	req := adaptiveInitialBoundaryRequest(7002, 8002, 1100)
	req.ExecutionRunID = 21
	req.AttemptID = "attempt-2"
	req.Generation = 4
	req.LeaseOwner = "worker-21"
	req.LeaseToken = "lease-21"
	req.Event.RunID = 21
	req.Checkpoint.RunID = 21
	req.Checkpoint.ParentCheckpointID = 8001
	sentinel := fmt.Errorf("source attempt lock sentinel")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").
		WithArgs(req.JournalRunID, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "thread_id", "parent_run_id", "run_kind",
		}).AddRow(req.JournalRunID, req.ThreadID, 0, string(entity.RunKindTask)))
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{
		"id", "thread_id", "status", "execution_generation", "lease_owner", "lease_token", "lease_expires_at", "cancel_requested_at", "space_id", "creator_id",
	}).AddRow(21, 10, string(entity.RunStatusRunning), 4, "worker-21", "lease-21", 3000, nil, 10, 20))
	mock.ExpectQuery("SELECT .*FROM .*run_attempts.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{
		"id", "thread_id", "journal_run_id", "execution_run_id", "attempt_id", "status", "active_slot", "next_sequence", "last_committed_sequence", "source_attempt_id", "source_checkpoint_id",
	}).AddRow(101, 10, 30, 21, "attempt-2", string(entity.RunAttemptStatusRunning), 1, 1, 0, "attempt-1", 8001))
	mock.ExpectQuery("SELECT .*FROM .*run_events.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT .*FROM .*run_attempts.*LIMIT \\?$").WillReturnRows(sqlmock.NewRows([]string{
		"id", "thread_id", "journal_run_id", "execution_run_id", "attempt_id", "status", "next_sequence", "last_committed_sequence",
	}).AddRow(100, 10, 30, 20, "attempt-1", string(entity.RunAttemptStatusCompleted), 2, 1))
	mock.ExpectQuery("SELECT .*FROM .*agent_runs.*FOR UPDATE").WillReturnRows(sqlmock.NewRows([]string{
		"id", "thread_id", "execution_generation",
	}).AddRow(20, 10, 3))
	mock.ExpectQuery("SELECT .*FROM .*run_attempts.*FOR UPDATE").WillReturnError(sentinel)
	mock.ExpectRollback()
	_, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.ErrorIs(t, err, sentinel)
}

func cloneAdaptiveExecutionBoundaryRequestForReplayTest(req CommitAdaptiveExecutionBoundaryRequest) CommitAdaptiveExecutionBoundaryRequest {
	clone := req
	event := *req.Event
	clone.Event = &event
	checkpoint := *req.Checkpoint
	clone.Checkpoint = &checkpoint
	mutation := *req.PlanMutation
	mutation.Items = append([]AdaptivePlanItemMutation(nil), req.PlanMutation.Items...)
	for index := range mutation.Items {
		item := *req.PlanMutation.Items[index].NextItem
		mutation.Items[index].NextItem = &item
	}
	clone.PlanMutation = &mutation
	return clone
}

func TestAdaptiveExecutionCheckpointMetadataV2RoundTrip(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Checkpoint.Metadata = `{"runtime_field":{"nested":true},"runtime_number":1.25}`
	req.PlanMutation.Items[0], req.PlanMutation.Items[1] = req.PlanMutation.Items[1], req.PlanMutation.Items[0]

	result, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.NoError(t, err)
	require.False(t, result.Replayed)
	require.JSONEq(t, `{"nested":true}`, extractJSONFieldForTest(t, result.Checkpoint.Metadata, "runtime_field"))
	require.JSONEq(t, `1.25`, extractJSONFieldForTest(t, result.Checkpoint.Metadata, "runtime_number"))
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata([]byte(result.Checkpoint.Metadata))
	require.NoError(t, err)
	require.Equal(t, "workbench-adaptive-boundary.v2", metadata.SchemaVersion)
	require.Equal(t, int64(7001), metadata.EventID)
	require.Equal(t, uint64(1), metadata.EventSequence)
	require.Equal(t, int64(30), metadata.JournalRunID)
	require.Equal(t, "attempt-1", metadata.AttemptID)
	require.Nil(t, metadata.SourceAttemptID)
	require.Nil(t, metadata.SourceCheckpointID)
	require.Equal(t, "boundary-1", metadata.EventIdempotencyKey)
	require.True(t, validAdaptiveExecutionFingerprint(metadata.EventFingerprint))
	require.True(t, validAdaptiveExecutionFingerprint(metadata.CheckpointFingerprint))
	var storedEvent runEventPO
	require.NoError(t, db.Where("id = ?", 7001).First(&storedEvent).Error)
	storedEventFingerprint, err := adaptiveExecutionEventFingerprint(&storedEvent)
	require.NoError(t, err)
	require.Equal(t, metadata.EventFingerprint, storedEventFingerprint)
	require.Equal(t, metadata.CheckpointFingerprint, requireStringPointerForTest(t, storedEvent.SnapshotID))
	require.Equal(t, int64(20), metadata.PlanScopeRunID)
	require.Equal(t, int64(2), metadata.PlanRevision)
	require.Equal(t, []adaptiveExecutionCheckpointItemRef{
		{ID: 60, TaskID: 1, Version: 2},
		{ID: 61, TaskID: 2, Version: 1},
	}, metadata.ItemRefs)
	require.Equal(t, int64(20), metadata.ExecutionRunID)
	require.Equal(t, uint64(3), metadata.ExecutionGeneration)
	require.Equal(t, AdaptiveExecutionBoundaryAuthority{
		ThreadID: 10, ExecutionRunID: 20, ExecutionGeneration: 3,
		JournalRunID: 30, AttemptID: "attempt-1",
		EventID: 7001, EventSequence: 1, IdempotencyKey: "boundary-1",
		CheckpointID: 8001, PlanScopeRunID: 20, PlanRevision: 2,
		PlanItemFingerprint: metadata.ItemFingerprint,
	}, result.Authority)

	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(result.Checkpoint.Metadata), &envelope))
	var serverFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["adaptive_execution"], &serverFields))
	for _, field := range []string{"source_attempt_id", "source_checkpoint_id"} {
		value, exists := serverFields[field]
		require.True(t, exists, field)
		require.Equal(t, "null", string(value), field)
	}

	canonicalEvent := func(payload string) *runEventPO {
		event, eventErr := runEventToPO(&entity.RunEvent{
			ID: 7001, ThreadID: 10, RunID: 20, EventType: "run.boundary",
			Payload: payload, CreatedAt: 1000,
		})
		require.NoError(t, eventErr)
		event.JournalRunID = adaptiveExecutionInt64Pointer(30)
		event.AttemptID = adaptiveExecutionStringPointer("attempt-1")
		event.Sequence = adaptiveExecutionUint64Pointer(1)
		event.IdempotencyKey = adaptiveExecutionStringPointer("boundary-1")
		return event
	}
	canonicalBase := canonicalEvent(`{"b":2,"a":1}`)
	eventFingerprint, err := adaptiveExecutionEventFingerprint(canonicalBase)
	require.NoError(t, err)
	equivalentFingerprint, err := adaptiveExecutionEventFingerprint(canonicalEvent(` { "a": 1, "b": 2 } `))
	require.NoError(t, err)
	require.Equal(t, eventFingerprint, equivalentFingerprint)
	changedEvent := canonicalEvent(`{"a":1,"b":3}`)
	changedFingerprint, err := adaptiveExecutionEventFingerprint(changedEvent)
	require.NoError(t, err)
	require.NotEqual(t, eventFingerprint, changedFingerprint)
	malformedEvent := *canonicalBase
	malformedEvent.Payload = []byte(`{"a":1} trailing`)
	_, err = adaptiveExecutionEventFingerprint(&malformedEvent)
	require.Error(t, err)
	for _, tt := range []struct {
		name   string
		mutate func(*runEventPO)
	}{
		{name: "event id", mutate: func(event *runEventPO) { event.ID++ }},
		{name: "thread id", mutate: func(event *runEventPO) { event.ThreadID++ }},
		{name: "run id", mutate: func(event *runEventPO) { event.RunID++ }},
		{name: "event type", mutate: func(event *runEventPO) { event.EventType += ".changed" }},
		{name: "payload", mutate: func(event *runEventPO) { event.Payload = []byte(`{"a":1,"b":3}`) }},
		{name: "created at", mutate: func(event *runEventPO) { event.CreatedAt++ }},
		{name: "journal run id", mutate: func(event *runEventPO) { event.JournalRunID = adaptiveExecutionInt64Pointer(31) }},
		{name: "attempt id", mutate: func(event *runEventPO) { event.AttemptID = adaptiveExecutionStringPointer("attempt-2") }},
		{name: "sequence", mutate: func(event *runEventPO) { event.Sequence = adaptiveExecutionUint64Pointer(2) }},
		{name: "idempotency key", mutate: func(event *runEventPO) { event.IdempotencyKey = adaptiveExecutionStringPointer("boundary-2") }},
		{name: "parent event id", mutate: func(event *runEventPO) { event.ParentEventID = adaptiveExecutionInt64Pointer(6999) }},
		{name: "schema version", mutate: func(event *runEventPO) { event.SchemaVersion = adaptiveExecutionStringPointer("journal.v1") }},
		{name: "status", mutate: func(event *runEventPO) { event.Status = adaptiveExecutionStringPointer("running") }},
		{name: "occurred at", mutate: func(event *runEventPO) { event.OccurredAtUnixNano = adaptiveExecutionInt64Pointer(1000000) }},
		{name: "visibility", mutate: func(event *runEventPO) { event.Visibility = adaptiveExecutionStringPointer("user") }},
		{name: "payload version", mutate: func(event *runEventPO) { event.PayloadVersion = adaptiveExecutionStringPointer("v1") }},
		{name: "snapshot id", mutate: func(event *runEventPO) { event.SnapshotID = adaptiveExecutionStringPointer("snapshot-1") }},
		{name: "trace id", mutate: func(event *runEventPO) { event.TraceID = adaptiveExecutionStringPointer("trace-1") }},
		{name: "action id", mutate: func(event *runEventPO) { event.ActionID = adaptiveExecutionStringPointer("action-1") }},
		{name: "phase", mutate: func(event *runEventPO) { event.Phase = adaptiveExecutionStringPointer("started") }},
		{name: "operation", mutate: func(event *runEventPO) { event.Operation = adaptiveExecutionStringPointer("write") }},
		{name: "target", mutate: func(event *runEventPO) { event.Target = adaptiveExecutionStringPointer("artifact-1") }},
		{name: "milestone", mutate: func(event *runEventPO) { event.Milestone = adaptiveExecutionStringPointer("step-1") }},
		{name: "journal event type", mutate: func(event *runEventPO) { event.JournalEventType = adaptiveExecutionStringPointer("action.started") }},
		{name: "journal payload", mutate: func(event *runEventPO) { event.JournalPayload = []byte(`{"a":1,"b":2}`) }},
	} {
		t.Run("event fingerprint binds "+tt.name, func(t *testing.T) {
			changed := *canonicalBase
			tt.mutate(&changed)
			fingerprint, fingerprintErr := adaptiveExecutionEventFingerprint(&changed)
			require.NoError(t, fingerprintErr)
			require.NotEqual(t, eventFingerprint, fingerprint)
		})
	}
	journalPayload := *canonicalBase
	journalPayload.JournalPayload = []byte(`{"b":2,"a":1}`)
	journalPayloadFingerprint, err := adaptiveExecutionEventFingerprint(&journalPayload)
	require.NoError(t, err)
	equivalentJournalPayload := journalPayload
	equivalentJournalPayload.JournalPayload = []byte(` { "a": 1, "b": 2 } `)
	equivalentJournalPayloadFingerprint, err := adaptiveExecutionEventFingerprint(&equivalentJournalPayload)
	require.NoError(t, err)
	require.Equal(t, journalPayloadFingerprint, equivalentJournalPayloadFingerprint)
	changedJournalPayload := journalPayload
	changedJournalPayload.JournalPayload = []byte(`{"a":1,"b":3}`)
	changedJournalPayloadFingerprint, err := adaptiveExecutionEventFingerprint(&changedJournalPayload)
	require.NoError(t, err)
	require.NotEqual(t, journalPayloadFingerprint, changedJournalPayloadFingerprint)
	jsonNullJournalPayload := *canonicalBase
	jsonNullJournalPayload.JournalPayload = []byte(`null`)
	jsonNullJournalPayloadFingerprint, err := adaptiveExecutionEventFingerprint(&jsonNullJournalPayload)
	require.NoError(t, err)
	require.NotEqual(t, eventFingerprint, jsonNullJournalPayloadFingerprint)
	malformedJournalPayload := *canonicalBase
	malformedJournalPayload.JournalPayload = []byte(`{"a":1} trailing`)
	_, err = adaptiveExecutionEventFingerprint(&malformedJournalPayload)
	require.Error(t, err)

	sourceAttemptID := "attempt-source"
	sourceCheckpointID := int64(7999)
	serverFields["source_attempt_id"], err = json.Marshal(sourceAttemptID)
	require.NoError(t, err)
	serverFields["source_checkpoint_id"], err = json.Marshal(sourceCheckpointID)
	require.NoError(t, err)
	envelope["adaptive_execution"], err = json.Marshal(serverFields)
	require.NoError(t, err)
	recoveryEncoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	recoveryMetadata, err := decodeAdaptiveExecutionCheckpointMetadata(recoveryEncoded)
	require.NoError(t, err)
	require.Equal(t, sourceAttemptID, requireStringPointerForTest(t, recoveryMetadata.SourceAttemptID))
	require.Equal(t, sourceCheckpointID, requireInt64PointerForTest(t, recoveryMetadata.SourceCheckpointID))

	terminalizeAdaptiveAttemptForTest(t, db, 100)
	bRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: 8001,
		EventID: 7002, CheckpointID: 8002, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1100,
	})
	bResult, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), bRequest)
	require.NoError(t, err)
	bMetadata, err := decodeAdaptiveExecutionCheckpointMetadata([]byte(bResult.Checkpoint.Metadata))
	require.NoError(t, err)
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, bMetadata.SourceAttemptID))
	require.Equal(t, int64(8001), requireInt64PointerForTest(t, bMetadata.SourceCheckpointID))
	require.Equal(t, int64(8001), bResult.Checkpoint.ParentCheckpointID)
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, bResult.Authority.SourceAttemptID))
	require.Equal(t, int64(8001), requireInt64PointerForTest(t, bResult.Authority.SourceCheckpointID))

	terminalizeAdaptiveAttemptForTest(t, db, 101)
	cRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 22, AttemptRowID: 102, AttemptID: "attempt-3", Generation: 5,
		SourceAttemptID: "attempt-2", SourceCheckpointID: 8002,
		EventID: 7003, CheckpointID: 8003, ExpectedRevision: 3, ExpectedItemVersion: 3, Now: 1200,
	})
	cResult, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), cRequest)
	require.NoError(t, err)
	cMetadata, err := decodeAdaptiveExecutionCheckpointMetadata([]byte(cResult.Checkpoint.Metadata))
	require.NoError(t, err)
	require.Equal(t, "attempt-2", requireStringPointerForTest(t, cMetadata.SourceAttemptID))
	require.Equal(t, int64(8002), requireInt64PointerForTest(t, cMetadata.SourceCheckpointID))
	require.Equal(t, int64(8002), cResult.Checkpoint.ParentCheckpointID)
	require.Equal(t, "attempt-2", requireStringPointerForTest(t, cResult.Authority.SourceAttemptID))
	require.Equal(t, int64(8002), requireInt64PointerForTest(t, cResult.Authority.SourceCheckpointID))

	validEncoded := []byte(result.Checkpoint.Metadata)
	mutateMetadata := func(t *testing.T, mutate func(map[string]json.RawMessage)) []byte {
		t.Helper()
		var copiedEnvelope map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(validEncoded, &copiedEnvelope))
		var copiedMetadata map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(copiedEnvelope["adaptive_execution"], &copiedMetadata))
		mutate(copiedMetadata)
		encodedMetadata, marshalErr := json.Marshal(copiedMetadata)
		require.NoError(t, marshalErr)
		copiedEnvelope["adaptive_execution"] = encodedMetadata
		encodedEnvelope, marshalErr := json.Marshal(copiedEnvelope)
		require.NoError(t, marshalErr)
		return encodedEnvelope
	}
	raw := func(value any) json.RawMessage {
		encoded, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		return encoded
	}
	invalidCases := []struct {
		name   string
		mutate func(map[string]json.RawMessage)
	}{
		{name: "v1 schema", mutate: func(fields map[string]json.RawMessage) {
			fields["schema_version"] = raw("workbench-adaptive-boundary.v1")
		}},
		{name: "unknown field", mutate: func(fields map[string]json.RawMessage) { fields["unknown"] = raw(true) }},
		{name: "blank attempt", mutate: func(fields map[string]json.RawMessage) { fields["attempt_id"] = raw(" ") }},
		{name: "long attempt", mutate: func(fields map[string]json.RawMessage) { fields["attempt_id"] = raw(strings.Repeat("a", 65)) }},
		{name: "multibyte long attempt", mutate: func(fields map[string]json.RawMessage) { fields["attempt_id"] = raw(strings.Repeat("界", 22)) }},
		{name: "partial source attempt", mutate: func(fields map[string]json.RawMessage) { fields["source_attempt_id"] = raw("source") }},
		{name: "partial source checkpoint", mutate: func(fields map[string]json.RawMessage) { fields["source_checkpoint_id"] = raw(int64(1)) }},
		{name: "blank source attempt", mutate: func(fields map[string]json.RawMessage) {
			fields["source_attempt_id"] = raw(" ")
			fields["source_checkpoint_id"] = raw(int64(1))
		}},
		{name: "long source attempt", mutate: func(fields map[string]json.RawMessage) {
			fields["source_attempt_id"] = raw(strings.Repeat("a", 65))
			fields["source_checkpoint_id"] = raw(int64(1))
		}},
		{name: "nonpositive source checkpoint", mutate: func(fields map[string]json.RawMessage) {
			fields["source_attempt_id"] = raw("source")
			fields["source_checkpoint_id"] = raw(int64(0))
		}},
		{name: "blank event key", mutate: func(fields map[string]json.RawMessage) { fields["event_idempotency_key"] = raw(" ") }},
		{name: "long event key", mutate: func(fields map[string]json.RawMessage) {
			fields["event_idempotency_key"] = raw(strings.Repeat("k", 192))
		}},
		{name: "multibyte long event key", mutate: func(fields map[string]json.RawMessage) {
			fields["event_idempotency_key"] = raw(strings.Repeat("界", 64))
		}},
		{name: "short event digest", mutate: func(fields map[string]json.RawMessage) { fields["event_fingerprint"] = raw(strings.Repeat("a", 63)) }},
		{name: "uppercase event digest", mutate: func(fields map[string]json.RawMessage) { fields["event_fingerprint"] = raw(strings.Repeat("A", 64)) }},
		{name: "short checkpoint digest", mutate: func(fields map[string]json.RawMessage) {
			fields["checkpoint_fingerprint"] = raw(strings.Repeat("a", 63))
		}},
		{name: "uppercase checkpoint digest", mutate: func(fields map[string]json.RawMessage) {
			fields["checkpoint_fingerprint"] = raw(strings.Repeat("A", 64))
		}},
		{name: "uppercase item digest", mutate: func(fields map[string]json.RawMessage) { fields["item_fingerprint"] = raw(strings.Repeat("A", 64)) }},
		{name: "empty refs", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{})
		}},
		{name: "too many refs", mutate: func(fields map[string]json.RawMessage) {
			refs := make([]adaptiveExecutionCheckpointItemRef, 33)
			for i := range refs {
				refs[i] = adaptiveExecutionCheckpointItemRef{ID: int64(i + 1), TaskID: int64(i + 1), Version: 1}
			}
			fields["item_refs"] = raw(refs)
		}},
		{name: "unsorted refs", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{{ID: 2, TaskID: 2, Version: 1}, {ID: 1, TaskID: 1, Version: 1}})
		}},
		{name: "zero ref id", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{{ID: 0, TaskID: 1, Version: 1}})
		}},
		{name: "zero ref task", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{{ID: 1, TaskID: 0, Version: 1}})
		}},
		{name: "zero ref version", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{{ID: 1, TaskID: 1, Version: 0}})
		}},
		{name: "duplicate ref id", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{{ID: 1, TaskID: 1, Version: 1}, {ID: 1, TaskID: 2, Version: 1}})
		}},
		{name: "duplicate ref task", mutate: func(fields map[string]json.RawMessage) {
			fields["item_refs"] = raw([]adaptiveExecutionCheckpointItemRef{{ID: 1, TaskID: 1, Version: 1}, {ID: 2, TaskID: 1, Version: 1}})
		}},
	}
	for _, tt := range invalidCases {
		t.Run(tt.name, func(t *testing.T) {
			_, decodeErr := decodeAdaptiveExecutionCheckpointMetadata(mutateMetadata(t, tt.mutate))
			require.Error(t, decodeErr)
		})
	}
	for _, field := range []string{
		"schema_version", "event_id", "event_sequence", "journal_run_id", "attempt_id",
		"source_attempt_id", "source_checkpoint_id", "event_idempotency_key", "event_fingerprint",
		"checkpoint_fingerprint",
		"plan_scope_run_id", "plan_revision", "item_fingerprint", "item_refs",
		"execution_run_id", "execution_generation",
	} {
		field := field
		t.Run("missing "+field, func(t *testing.T) {
			_, decodeErr := decodeAdaptiveExecutionCheckpointMetadata(mutateMetadata(t, func(fields map[string]json.RawMessage) {
				delete(fields, field)
			}))
			require.Error(t, decodeErr)
		})
	}
}

func TestAdaptiveExecutionBoundaryRecoversCanonicalPlanScopeAcrossMultipleHops(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)

	a, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000))
	require.NoError(t, err)
	require.Equal(t, int64(20), decodeAdaptiveExecutionMetadataForTest(t, []byte(a.Checkpoint.Metadata)).PlanScopeRunID)
	terminalizeAdaptiveAttemptForTest(t, db, 100)

	bRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: 8001,
		EventID: 7002, CheckpointID: 8002, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1100,
	})
	b, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), bRequest)
	require.NoError(t, err)
	bMetadata := decodeAdaptiveExecutionMetadataForTest(t, []byte(b.Checkpoint.Metadata))
	require.Equal(t, int64(20), bMetadata.PlanScopeRunID)
	require.Equal(t, int64(21), bMetadata.ExecutionRunID)
	require.Equal(t, uint64(4), bMetadata.ExecutionGeneration)
	require.Equal(t, int64(3), bMetadata.PlanRevision)
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, bMetadata.SourceAttemptID))
	require.Equal(t, int64(8001), requireInt64PointerForTest(t, bMetadata.SourceCheckpointID))
	require.Equal(t, int64(8001), b.Checkpoint.ParentCheckpointID)
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, b.Authority.SourceAttemptID))
	require.Equal(t, int64(8001), requireInt64PointerForTest(t, b.Authority.SourceCheckpointID))
	terminalizeAdaptiveAttemptForTest(t, db, 101)

	cRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 22, AttemptRowID: 102, AttemptID: "attempt-3", Generation: 5,
		SourceAttemptID: "attempt-2", SourceCheckpointID: 8002,
		EventID: 7003, CheckpointID: 8003, ExpectedRevision: 3, ExpectedItemVersion: 3, Now: 1200,
	})
	c, err := repo.CommitAdaptiveExecutionBoundary(context.Background(), cRequest)
	require.NoError(t, err)
	cMetadata := decodeAdaptiveExecutionMetadataForTest(t, []byte(c.Checkpoint.Metadata))
	require.Equal(t, int64(20), cMetadata.PlanScopeRunID)
	require.Equal(t, int64(22), cMetadata.ExecutionRunID)
	require.Equal(t, uint64(5), cMetadata.ExecutionGeneration)
	require.Equal(t, int64(4), cMetadata.PlanRevision)
	require.Equal(t, "attempt-2", requireStringPointerForTest(t, cMetadata.SourceAttemptID))
	require.Equal(t, int64(8002), requireInt64PointerForTest(t, cMetadata.SourceCheckpointID))
	require.Equal(t, int64(8002), c.Checkpoint.ParentCheckpointID)
	require.Equal(t, "attempt-2", requireStringPointerForTest(t, c.Authority.SourceAttemptID))
	require.Equal(t, int64(8002), requireInt64PointerForTest(t, c.Authority.SourceCheckpointID))
	require.Equal(t, int64(20), c.Plan.RunID)
}

func TestAdaptiveExecutionRecoveryReadUsesExactSourceCheckpoint(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	repo := NewAdaptiveExecutionRepository(db)
	sourceBoundary, err := repo.CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.NoError(t, err)
	terminalizeAdaptiveAttemptForTest(t, db, 100)
	target := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: 8001,
		EventID: 7002, CheckpointID: 8002, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1100,
	})
	var source checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&source).Error)
	decoy := source
	decoy.ID = 8998
	decoy.CreatedAt = 2000
	require.NoError(t, db.Create(&decoy).Error)
	terminalizeAdaptiveAttemptForTest(t, db, 101)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := repo.ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.NoError(t, err)
	expected := *sourceBoundary
	expected.Replayed = true
	require.Equal(t, &expected, result)
	require.Equal(t, int64(7001), result.Event.ID)
	require.Equal(t, uint64(1), result.LastCommittedSequence)
	require.Equal(t, int64(8001), result.Checkpoint.ID)
	require.Equal(t, int64(20), result.Plan.RunID)
	require.Equal(t, []int64{1, 2}, adaptiveTaskIDsForTest(result.Items))
	require.Equal(t, "attempt-1", result.Authority.AttemptID)
	require.Equal(t, int64(7001), result.Authority.EventID)
	require.Equal(t, uint64(1), result.Authority.EventSequence)
	require.Equal(t, int64(8001), result.Authority.CheckpointID)
	require.True(t, result.Replayed)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionRecoveryReadRecoversCanonicalPlanScopeAcrossMultipleHops(t *testing.T) {
	db, target := prepareAdaptiveSecondHopRecoveryTestForTest(t)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.NoError(t, err)
	require.Equal(t, int64(8002), result.Checkpoint.ID)
	require.Equal(t, "attempt-2", result.Authority.AttemptID)
	require.Equal(t, "attempt-1", requireStringPointerForTest(t, result.Authority.SourceAttemptID))
	require.Equal(t, int64(8001), requireInt64PointerForTest(t, result.Authority.SourceCheckpointID))
	require.Equal(t, int64(20), result.Authority.PlanScopeRunID)
	require.Equal(t, int64(20), result.Plan.RunID)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionRecoveryReadRejectsAuthorityDriftWithoutWrites(t *testing.T) {
	db, target := prepareAdaptiveRecoveryTestForTest(t)
	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	tests := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB)
	}{
		{name: "target thread identity", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("thread_id", 11).Error)
		}},
		{name: "target journal identity", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("journal_run_id", 31).Error)
		}},
		{name: "partial target source", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", nil).Error)
		}},
		{name: "self target source", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", "attempt-2").Error)
		}},
		{name: "source checkpoint missing", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", 8998).Error)
		}},
		{name: "source checkpoint cross thread", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("thread_id", 11).Error)
		}},
		{name: "source checkpoint deleted", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("runtime_deleted_at", 1).Error)
		}},
		{name: "source checkpoint parent drift", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("parent_checkpoint_id", 7999).Error)
		}},
		{name: "only source attempt pointer changes", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", "missing-attempt").Error)
		}},
		{name: "only source checkpoint pointer changes", mutate: func(t *testing.T, db *gorm.DB) {
			var checkpoint checkpointPO
			require.NoError(t, db.Where("id = ?", 8001).First(&checkpoint).Error)
			checkpoint.ID = 8998
			checkpoint.RunID = 21
			require.NoError(t, db.Create(&checkpoint).Error)
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", 8998).Error)
		}},
		{name: "source attempt pointer drift", mutate: func(t *testing.T, db *gorm.DB) {
			seedAmbientAdaptiveSourceForTest(t, db)
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Updates(map[string]any{
				"source_attempt_id": "ambient-attempt", "source_checkpoint_id": 8999,
			}).Error)
		}},
		{name: "source attempt ordinal does not precede target", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Update("ordinal", 3).Error)
		}},
		{name: "source last committed sequence", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Update("last_committed_sequence", 0).Error)
		}},
		{name: "source run generation", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runPO{}).Where("id = ?", 20).Update("execution_generation", 4).Error)
		}},
		{name: "event tuple", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 7001).Update("sequence", 2).Error)
		}},
		{name: "event fingerprint", mutate: func(t *testing.T, db *gorm.DB) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["event_fingerprint"] = json.RawMessage(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
			})
		}},
		{name: "plan scope", mutate: func(t *testing.T, db *gorm.DB) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["plan_scope_run_id"] = json.RawMessage(`21`)
			})
		}},
		{name: "plan revision", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&agentRunPlanPO{}).Where("run_id = ?", 20).Update("revision", 3).Error)
		}},
		{name: "item ref missing", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Where("id = ?", 60).Delete(&agentRunPlanItemPO{}).Error)
		}},
		{name: "item ref task", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&agentRunPlanItemPO{}).Where("id = ?", 60).Update("task_id", 9).Error)
		}},
		{name: "item ref version", mutate: func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&agentRunPlanItemPO{}).Where("id = ?", 60).Update("version", 3).Error)
		}},
		{name: "item fingerprint", mutate: func(t *testing.T, db *gorm.DB) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["item_fingerprint"] = json.RawMessage(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
			})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, target := prepareAdaptiveRecoveryTestForTest(t)
			tt.mutate(t, db)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
				context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
					ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
				},
			)
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionRecoveryReadRejectsClonedSourceCheckpoint(t *testing.T) {
	db, target := prepareAdaptiveRecoveryTestForTest(t)
	var original checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&original).Error)
	cloned := original
	cloned.ID = 8998
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &cloned)
	require.NoError(t, db.Create(&cloned).Error)
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND attempt_id = ?", 30, target.AttemptID).
		Update("source_checkpoint_id", cloned.ID).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionRecoveryReadRejectsInPlaceSourceCheckpointDrift(t *testing.T) {
	db, target := prepareAdaptiveRecoveryTestForTest(t)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).
		Update("channel_values", []byte(`{"messages":[2]}`)).Error)
	var changedCheckpoint checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&changedCheckpoint).Error)
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &changedCheckpoint)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).
		Update("metadata", changedCheckpoint.Metadata).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionRecoveryReadRejectsCoherentCheckpointAuthorityDrift(t *testing.T) {
	db, target := prepareAdaptiveRecoveryTestForTest(t)
	shrinkAdaptiveExecutionCheckpointAuthorityForTest(t, db, 8001)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionRecoveryReadMatchesCompatibilityReaders(t *testing.T) {
	db, target := prepareAdaptiveRecoveryTestForTest(t)
	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.NoError(t, err)
	compatibilityRepo := &threadRepository{db: db}
	events, total, err := compatibilityRepo.ListRunEvents(context.Background(), ListRunEventsRequest{
		ThreadID: 10, RunID: 20, Page: 1, PageSize: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []*entity.RunEvent{result.Event}, events)
	checkpoint, err := compatibilityRepo.GetCheckpoint(context.Background(), 8001)
	require.NoError(t, err)
	require.Equal(t, checkpoint, result.Checkpoint)
	plan, err := compatibilityRepo.GetPlan(context.Background(), 20)
	require.NoError(t, err)
	require.Equal(t, plan, result.Plan)
	items, err := compatibilityRepo.ListPlanItems(context.Background(), 20, true)
	require.NoError(t, err)
	require.Equal(t, items, result.Items)
}

func TestAdaptiveExecutionRecoveryReadSelectsSafeTransactionOptions(t *testing.T) {
	db, target := prepareAdaptiveRecoveryTestForTest(t)
	require.Nil(t, adaptiveExecutionRecoveryTransactionOptions(db))
	mysqlRepo, _ := canonicalMySQLMockRepository(t)
	options := adaptiveExecutionRecoveryTransactionOptions(mysqlRepo.db)
	require.NotNil(t, options)
	require.Equal(t, sql.LevelRepeatableRead, options.Isolation)
	require.True(t, options.ReadOnly)

	result, err := NewAdaptiveExecutionRepository(db).ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, result)

	failingRepo, failingMock := canonicalMySQLMockRepository(t)
	sentinel := fmt.Errorf("recovery read sentinel")
	failingMock.ExpectBegin()
	failingMock.ExpectQuery("SELECT .*FROM .*run_attempts").WillReturnError(sentinel)
	failingMock.ExpectRollback()
	result, err = failingRepo.ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
	require.ErrorIs(t, err, sentinel)

	beginRepo, beginMock := canonicalMySQLMockRepository(t)
	beginSentinel := fmt.Errorf("recovery begin sentinel")
	beginMock.ExpectBegin().WillReturnError(beginSentinel)
	result, err = beginRepo.ReadAdaptiveExecutionRecoverySource(
		context.Background(), ReadAdaptiveExecutionRecoverySourceRequest{
			ThreadID: 10, JournalRunID: 30, TargetAttemptID: target.AttemptID,
		},
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
	require.ErrorIs(t, err, beginSentinel)
}

func TestAdaptiveExecutionBoundaryRejectsNonCanonicalRecoveryLineage(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "source attempt only", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", nil).Error)
		}},
		{name: "source checkpoint only", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", nil).Error)
		}},
		{name: "source attempt does not own checkpoint", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			seedAmbientAdaptiveSourceForTest(t, db)
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", "ambient-attempt").Error)
		}},
		{name: "source attempt ordinal does not precede target", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Update("ordinal", 3).Error)
		}},
		{name: "ambient same thread source", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			seedAmbientAdaptiveSourceForTest(t, db)
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Updates(map[string]any{
				"source_attempt_id": "ambient-attempt", "source_checkpoint_id": 8999,
			}).Error)
		}},
		{name: "target parent drift", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Checkpoint.ParentCheckpointID++
		}},
		{name: "self source attempt", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_attempt_id", "attempt-2").Error)
		}},
		{name: "self source checkpoint", mutate: func(t *testing.T, db *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 101).Update("source_checkpoint_id", req.Checkpoint.ID).Error)
			req.Checkpoint.ParentCheckpointID = req.Checkpoint.ID
		}},
		{name: "coherent source self reference", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Updates(map[string]any{
				"source_attempt_id": "attempt-1", "source_checkpoint_id": int64(8001),
			}).Error)
			var checkpoint checkpointPO
			require.NoError(t, db.Where("id = ?", 8001).First(&checkpoint).Error)
			checkpoint.ParentCheckpointID = checkpoint.ID
			checkpoint.Metadata = rewriteAdaptiveExecutionCheckpointMetadataForTest(
				t,
				checkpoint.Metadata,
				func(metadata map[string]json.RawMessage) {
					metadata["source_attempt_id"] = json.RawMessage(`"attempt-1"`)
					metadata["source_checkpoint_id"] = json.RawMessage(`8001`)
				},
			)
			refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &checkpoint)
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpoint.ID).Updates(map[string]any{
				"parent_checkpoint_id": checkpoint.ParentCheckpointID,
				"metadata":             checkpoint.Metadata,
			}).Error)
		}},
		{name: "source refs and item fingerprint drift", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["item_refs"] = json.RawMessage(`[{"id":61,"task_id":2,"version":1}]`)
				metadata["item_fingerprint"] = json.RawMessage(`"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`)
			})
		}},
		{name: "source checkpoint deleted", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("runtime_deleted_at", 1).Error)
		}},
		{name: "metadata missing namespace", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("metadata", []byte(`{"runtime_field":"preserved"}`)).Error)
		}},
		{name: "metadata unknown schema", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["schema_version"] = json.RawMessage(`"unknown"`)
			})
		}},
		{name: "metadata event missing", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["event_id"] = json.RawMessage(`7999`)
			})
		}},
		{name: "metadata sequence beyond committed", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["event_sequence"] = json.RawMessage(`2`)
			})
		}},
		{name: "metadata request plan scope drift", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			req.PlanMutation.PlanScopeRunID = 21
			req.PlanMutation.Items[0].NextItem.RunID = 21
		}},
		{name: "metadata plan revision drift", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) {
				metadata["plan_revision"] = json.RawMessage(`1`)
			})
		}},
		{name: "source checkpoint cross thread", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("thread_id", 11).Error)
		}},
		{name: "source attempt cross thread", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Update("thread_id", 11).Error)
		}},
	}

	for _, field := range []string{
		"schema_version", "event_id", "event_sequence", "journal_run_id", "attempt_id",
		"source_attempt_id", "source_checkpoint_id", "event_idempotency_key", "event_fingerprint",
		"checkpoint_fingerprint",
		"plan_scope_run_id", "plan_revision", "item_fingerprint", "item_refs",
		"execution_run_id", "execution_generation",
	} {
		field := field
		tests = append(tests, struct {
			name   string
			mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
		}{name: "metadata missing " + field, mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceMetadataForTest(t, db, func(metadata map[string]json.RawMessage) { delete(metadata, field) })
		}})
	}
	for _, field := range []string{"thread_id", "run_id", "journal_run_id", "attempt_id", "sequence"} {
		field := field
		tests = append(tests, struct {
			name   string
			mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
		}{name: "source event " + field + " drift", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			mutateAdaptiveSourceEventFieldForTest(t, db, field)
		}})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, req := prepareAdaptiveRecoveryTestForTest(t)
			tt.mutate(t, db, &req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsCoherentCheckpointAuthorityDrift(t *testing.T) {
	db, req := prepareAdaptiveRecoveryTestForTest(t)
	shrinkAdaptiveExecutionCheckpointAuthorityForTest(t, db, 8001)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), req,
	)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRecoveryLineagePreservesCheckpointConflictCause(t *testing.T) {
	db, req := prepareAdaptiveRecoveryTestForTest(t)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).
		Update("channel_values", []byte(`{"messages":[2]}`)).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	result, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsStaleSourceGeneration(t *testing.T) {
	db, req := prepareAdaptiveRecoveryTestForTest(t)
	require.NoError(t, db.Model(&runPO{}).Where("id = ?", 20).Update("execution_generation", 4).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsInvalidTargetAttemptCursor(t *testing.T) {
	activeZero := uint8(0)
	activeTwo := uint8(2)
	tests := []struct {
		name        string
		updates     map[string]any
		expectedErr error
		directMax   bool
	}{
		{name: "nil active slot", updates: map[string]any{"active_slot": nil}, expectedErr: ErrAdaptiveExecutionAttemptConflict},
		{name: "zero active slot", updates: map[string]any{"active_slot": activeZero}, expectedErr: ErrAdaptiveExecutionAttemptConflict},
		{name: "two active slot", updates: map[string]any{"active_slot": activeTwo}, expectedErr: ErrAdaptiveExecutionAttemptConflict},
		{name: "zero next sequence", updates: map[string]any{"next_sequence": uint64(0)}, expectedErr: ErrAdaptiveExecutionSequenceConflict},
		{name: "committed equals next", updates: map[string]any{"last_committed_sequence": uint64(1)}, expectedErr: ErrAdaptiveExecutionSequenceConflict},
		{name: "committed beyond next", updates: map[string]any{"last_committed_sequence": uint64(2)}, expectedErr: ErrAdaptiveExecutionSequenceConflict},
		{name: "next sequence max", expectedErr: ErrAdaptiveExecutionSequenceConflict, directMax: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			if !tt.directMax {
				require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", 100).Updates(tt.updates).Error)
			}
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			if tt.directMax {
				var locked runAttemptPO
				require.NoError(t, db.Where("id = ?", 100).First(&locked).Error)
				locked.NextSequence = math.MaxUint64
				require.ErrorIs(t, validateAdaptiveExecutionTargetAttemptCursor(&locked), tt.expectedErr)
				require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
				return
			}
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
				context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
			)
			require.ErrorIs(t, err, tt.expectedErr)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsInitialCheckpointParent(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	req.Checkpoint.ParentCheckpointID = 7999
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
	require.ErrorIs(t, err, ErrAdaptiveExecutionLineageConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsMalformedMutationJSONWithoutWrites(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "event payload", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.Payload = `{"broken":` }},
		{name: "checkpoint channel values", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelValues = `{"broken":` }},
		{name: "checkpoint channel versions", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ChannelVersions = `{"broken":` }},
		{name: "checkpoint pending sends", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.PendingSends = `[{` }},
		{name: "checkpoint metadata", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.Metadata = `{"broken":` }},
		{name: "plan item blocks", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.Blocks = `[` }},
		{name: "plan item blocked by", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.BlockedBy = `[` }},
		{name: "plan item metadata", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.Metadata = `{` }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			tt.mutate(&req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionBoundaryInvalid)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRollsBackCheckpointConflict(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 8001, ThreadID: 10, RunID: 20, CheckpointNS: "existing",
		RuntimeType: "eino_adk", RuntimeKey: "existing", EnvelopeVersion: 1,
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`),
		Metadata: []byte(`{}`), CreatedAt: 900,
	}).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.ErrorIs(t, err, ErrAdaptiveExecutionCheckpointConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
	var count int64
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 7001).Count(&count).Error)
	require.Zero(t, count)
}

func TestAdaptiveExecutionBoundaryRejectsStalePlanRevision(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	require.NoError(t, db.Model(&agentRunPlanPO{}).Where("run_id = ?", 20).Update("revision", 2).Error)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.ErrorIs(t, err, ErrAdaptiveExecutionPlanRevisionConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func TestAdaptiveExecutionBoundaryRejectsStalePlanItemVersion(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB, *CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "second update stale", mutate: func(t *testing.T, db *gorm.DB, _ *CommitAdaptiveExecutionBoundaryRequest) {
			require.NoError(t, db.Model(&agentRunPlanItemPO{}).Where("id = ?", 60).Update("version", 2).Error)
		}},
		{name: "create tuple exists", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			item := newAdaptivePlanItem(61, 20, 1, 1, req.Now)
			req.PlanMutation.Items = []AdaptivePlanItemMutation{{ExpectedVersion: 0, NextItem: item}}
		}},
		{name: "create id exists", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			item := newAdaptivePlanItem(60, 20, 2, 1, req.Now)
			req.PlanMutation.Items = []AdaptivePlanItemMutation{{ExpectedVersion: 0, NextItem: item}}
		}},
		{name: "task beyond high watermark", mutate: func(_ *testing.T, _ *gorm.DB, req *CommitAdaptiveExecutionBoundaryRequest) {
			item := newAdaptivePlanItem(61, 20, 3, 1, req.Now)
			req.PlanMutation.Items = []AdaptivePlanItemMutation{{ExpectedVersion: 0, NextItem: item}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newAdaptiveExecutionRepositoryTestDB(t)
			seedAdaptiveExecutionInitialState(t, db)
			req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
			tt.mutate(t, db, &req)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionPlanItemVersionConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRejectsPlanScopeIdentityDrift(t *testing.T) {
	tests := []struct {
		name   string
		model  any
		where  string
		column string
		value  any
	}{
		{name: "scope run thread", model: &runPO{}, where: "id = ?", column: "thread_id", value: int64(11)},
		{name: "scope run space", model: &runPO{}, where: "id = ?", column: "space_id", value: int64(11)},
		{name: "scope run creator", model: &runPO{}, where: "id = ?", column: "creator_id", value: int64(21)},
		{name: "plan thread", model: &agentRunPlanPO{}, where: "run_id = ?", column: "thread_id", value: int64(11)},
		{name: "plan space", model: &agentRunPlanPO{}, where: "run_id = ?", column: "space_id", value: int64(11)},
		{name: "plan user", model: &agentRunPlanPO{}, where: "run_id = ?", column: "user_id", value: int64(21)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, req := prepareAdaptiveSecondHopRecoveryTestForTest(t)
			require.NoError(t, db.Model(tt.model).Where(tt.where, 20).Update(tt.column, tt.value).Error)
			before := snapshotAdaptiveExecutionDBForTest(t, db)
			_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), req)
			require.ErrorIs(t, err, ErrAdaptiveExecutionPlanScopeConflict)
			require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
		})
	}
}

func TestAdaptiveExecutionBoundaryRollsBackAttemptSequenceCASConflict(t *testing.T) {
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	req := adaptiveInitialBoundaryRequest(7001, 8001, 1000)
	before := snapshotAdaptiveExecutionDBForTest(t, db)

	err := db.Transaction(func(tx *gorm.DB) error {
		state := buildAdaptiveExecutionLockedStateForTest(t, tx, req)
		require.NoError(t, tx.Model(&runAttemptPO{}).Where("id = ?", state.attempt.ID).
			Update("next_sequence", state.attempt.NextSequence+1).Error)
		return commitAdaptiveExecutionMutationLocked(tx, req, state)
	})
	require.ErrorIs(t, err, ErrAdaptiveExecutionSequenceConflict)
	require.Equal(t, before, snapshotAdaptiveExecutionDBForTest(t, db))
}

func prepareAdaptiveSecondHopRecoveryTestForTest(t *testing.T) (*gorm.DB, CommitAdaptiveExecutionBoundaryRequest) {
	t.Helper()
	db, bRequest := prepareAdaptiveRecoveryTestForTest(t)
	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(context.Background(), bRequest)
	require.NoError(t, err)
	terminalizeAdaptiveAttemptForTest(t, db, 101)
	cRequest := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 22, AttemptRowID: 102, AttemptID: "attempt-3", Generation: 5,
		SourceAttemptID: "attempt-2", SourceCheckpointID: 8002,
		EventID: 7003, CheckpointID: 8003, ExpectedRevision: 3, ExpectedItemVersion: 3, Now: 1200,
	})
	return db, cRequest
}

func buildAdaptiveExecutionLockedStateForTest(
	t *testing.T,
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) *adaptiveExecutionLockedState {
	t.Helper()
	run, err := lockAdaptiveExecutionRun(tx, req)
	require.NoError(t, err)
	attempt, err := lockAdaptiveExecutionAttempt(tx, req)
	require.NoError(t, err)
	_, err = lockAdaptiveExecutionLineage(tx, req, attempt)
	require.NoError(t, err)
	scopeRun, err := lockAdaptiveExecutionScopeRun(tx, req.PlanMutation.PlanScopeRunID)
	require.NoError(t, err)
	plan, err := lockAdaptiveExecutionPlan(tx, req.PlanMutation.PlanScopeRunID)
	require.NoError(t, err)
	require.NoError(t, validateAdaptiveExecutionPlanScope(run, scopeRun, plan, req.PlanMutation))
	items, err := lockAdaptiveExecutionPlanItems(tx, plan, req.PlanMutation, req.Now)
	require.NoError(t, err)
	nextItems := make([]*entity.AgentRunPlanItem, 0, len(items))
	for _, item := range items {
		nextItems = append(nextItems, item.next)
	}
	fingerprint, err := adaptiveExecutionPlanItemFingerprint(nextItems)
	require.NoError(t, err)
	itemRefs, err := adaptiveExecutionItemRefs(nextItems)
	require.NoError(t, err)
	event, err := runEventToPO(req.Event)
	require.NoError(t, err)
	event.JournalRunID = adaptiveExecutionInt64Pointer(req.JournalRunID)
	event.AttemptID = adaptiveExecutionStringPointer(req.AttemptID)
	event.Sequence = adaptiveExecutionUint64Pointer(attempt.NextSequence)
	event.IdempotencyKey = adaptiveExecutionStringPointer(req.IdempotencyKey)
	checkpointEntity := *req.Checkpoint
	checkpointBase, err := checkpointToPO(&checkpointEntity)
	require.NoError(t, err)
	metadata := adaptiveExecutionCheckpointMetadata{
		SchemaVersion: adaptiveExecutionCheckpointSchemaVersion,
		EventID:       event.ID, EventSequence: attempt.NextSequence,
		JournalRunID: req.JournalRunID, AttemptID: req.AttemptID,
		SourceAttemptID: attempt.SourceAttemptID, SourceCheckpointID: attempt.SourceCheckpointID,
		EventIdempotencyKey: req.IdempotencyKey,
		PlanScopeRunID:      req.PlanMutation.PlanScopeRunID, PlanRevision: req.PlanMutation.NextRevision,
		ItemFingerprint: fingerprint, ItemRefs: itemRefs,
		ExecutionRunID: run.ID, ExecutionGeneration: run.ExecutionGeneration,
	}
	checkpointFingerprint, err := adaptiveExecutionCheckpointFingerprint(checkpointBase, &metadata)
	require.NoError(t, err)
	event.SnapshotID = adaptiveExecutionStringPointer(checkpointFingerprint)
	eventFingerprint, err := adaptiveExecutionEventFingerprint(event)
	require.NoError(t, err)
	metadata.EventFingerprint = eventFingerprint
	metadata.CheckpointFingerprint = checkpointFingerprint
	checkpointEntity.Metadata, err = mergeAdaptiveExecutionCheckpointMetadata(req.Checkpoint.Metadata, metadata)
	require.NoError(t, err)
	checkpoint, err := checkpointToPO(&checkpointEntity)
	require.NoError(t, err)
	return &adaptiveExecutionLockedState{
		run: run, attempt: attempt, plan: plan, event: event, checkpoint: checkpoint,
		items: items, sequence: attempt.NextSequence,
	}
}

type adaptiveRecoveryTargetForTest struct {
	RunID               int64
	AttemptRowID        int64
	AttemptID           string
	Generation          uint64
	SourceAttemptID     string
	SourceCheckpointID  int64
	EventID             int64
	CheckpointID        int64
	ExpectedRevision    int64
	ExpectedItemVersion int64
	Now                 int64
}

func prepareAdaptiveRecoveryTestForTest(t *testing.T) (*gorm.DB, CommitAdaptiveExecutionBoundaryRequest) {
	t.Helper()
	db := newAdaptiveExecutionRepositoryTestDB(t)
	seedAdaptiveExecutionInitialState(t, db)
	_, err := NewAdaptiveExecutionRepository(db).CommitAdaptiveExecutionBoundary(
		context.Background(), adaptiveInitialBoundaryRequest(7001, 8001, 1000),
	)
	require.NoError(t, err)
	terminalizeAdaptiveAttemptForTest(t, db, 100)
	req := seedAdaptiveRecoveryTargetForTest(t, db, adaptiveRecoveryTargetForTest{
		RunID: 21, AttemptRowID: 101, AttemptID: "attempt-2", Generation: 4,
		SourceAttemptID: "attempt-1", SourceCheckpointID: 8001,
		EventID: 7002, CheckpointID: 8002, ExpectedRevision: 2, ExpectedItemVersion: 2, Now: 1100,
	})
	return db, req
}

func seedAdaptiveRecoveryTargetForTest(
	t *testing.T,
	db *gorm.DB,
	target adaptiveRecoveryTargetForTest,
) CommitAdaptiveExecutionBoundaryRequest {
	t.Helper()
	leaseOwner := fmt.Sprintf("worker-%d", target.RunID)
	leaseToken := fmt.Sprintf("lease-%d", target.RunID)
	leaseExpiresAt := target.Now + 1000
	require.NoError(t, db.Create(&runPO{
		ID: target.RunID, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		Status: string(entity.RunStatusRunning), ExecutionGeneration: target.Generation,
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		CreatedAt: target.Now - 100, UpdatedAt: target.Now - 100,
	}).Error)
	activeSlot := uint8(1)
	sourceAttemptID := target.SourceAttemptID
	sourceCheckpointID := target.SourceCheckpointID
	require.NoError(t, db.Create(&runAttemptPO{
		ID: target.AttemptRowID, ThreadID: 10, JournalRunID: 30, ExecutionRunID: target.RunID,
		AttemptID: target.AttemptID, Ordinal: uint32(target.AttemptRowID - 99),
		Status: string(entity.RunAttemptStatusRunning), ActiveSlot: &activeSlot,
		NextSequence: 1, LastCommittedSequence: 0,
		SourceAttemptID: &sourceAttemptID, SourceCheckpointID: &sourceCheckpointID,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         target.Now - 100, UpdatedAt: target.Now - 100,
	}).Error)

	req := newValidAdaptiveExecutionMutationRequest()
	req.ExecutionRunID = target.RunID
	req.AttemptID = target.AttemptID
	req.Generation = target.Generation
	req.LeaseOwner = leaseOwner
	req.LeaseToken = leaseToken
	req.Now = target.Now
	req.IdempotencyKey = fmt.Sprintf("boundary-%s", target.AttemptID)
	req.Event = &entity.RunEvent{
		ID: target.EventID, ThreadID: 10, RunID: target.RunID,
		EventType: "run.boundary", Payload: `{"recovery":true}`, CreatedAt: target.Now,
	}
	req.Checkpoint = &entity.Checkpoint{
		ID: target.CheckpointID, ThreadID: 10, RunID: target.RunID,
		ParentCheckpointID: target.SourceCheckpointID,
		CheckpointNS:       "adaptive", RuntimeType: "eino_adk",
		RuntimeKey: fmt.Sprintf("thread:10:run:%d", target.RunID), EnvelopeVersion: 1,
		ChannelValues: `{}`, ChannelVersions: `{}`, PendingSends: `[]`,
		Metadata: `{"runtime_field":"preserved"}`, CreatedAt: target.Now,
	}
	req.PlanMutation = &AdaptivePlanMutation{
		PlanScopeRunID: 20, ExpectedRevision: target.ExpectedRevision, NextRevision: target.ExpectedRevision + 1,
		Items: []AdaptivePlanItemMutation{{
			ExpectedVersion: target.ExpectedItemVersion,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject:     fmt.Sprintf("recovered step %d", target.ExpectedItemVersion+1),
				Description: "recovery update", Status: entity.AgentRunPlanItemStatusInProgress,
				ActiveForm: "executing", Owner: "agent", Blocks: `[]`, BlockedBy: `[]`,
				Metadata: `{"substeps_ref":"lazy:1"}`, Active: true,
				Version: target.ExpectedItemVersion + 1,
			},
		}},
	}
	return req
}

func terminalizeAdaptiveAttemptForTest(t *testing.T, db *gorm.DB, id int64) {
	t.Helper()
	require.NoError(t, db.Model(&runAttemptPO{}).Where("id = ?", id).Updates(map[string]any{
		"status": string(entity.RunAttemptStatusCompleted), "active_slot": nil,
	}).Error)
}

func seedAmbientAdaptiveSourceForTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Create(&runPO{
		ID: 29, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		Status: string(entity.RunStatusSucceeded), ExecutionGeneration: 1,
		CreatedAt: 900, UpdatedAt: 900,
	}).Error)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 109, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 29,
		AttemptID: "ambient-attempt", Ordinal: 9, Status: string(entity.RunAttemptStatusCompleted),
		NextSequence: 2, LastCommittedSequence: 1,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         900, UpdatedAt: 900,
	}).Error)
	require.NoError(t, db.Create(&checkpointPO{
		ID: 8999, ThreadID: 10, RunID: 29, CheckpointNS: "adaptive",
		RuntimeType: "eino_adk", RuntimeKey: "ambient", EnvelopeVersion: 1,
		ChannelValues: []byte(`{}`), ChannelVersions: []byte(`{}`), PendingSends: []byte(`[]`),
		Metadata: []byte(`{"ambient":true}`), CreatedAt: 900,
	}).Error)
}

func mutateAdaptiveSourceMetadataForTest(
	t *testing.T,
	db *gorm.DB,
	mutate func(map[string]json.RawMessage),
) {
	t.Helper()
	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", 8001).First(&checkpoint).Error)
	encodedEnvelope := rewriteAdaptiveExecutionCheckpointMetadataForTest(t, checkpoint.Metadata, mutate)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", 8001).Update("metadata", encodedEnvelope).Error)
}

func rewriteAdaptiveExecutionCheckpointMetadataForTest(
	t *testing.T,
	raw []byte,
	mutate func(map[string]json.RawMessage),
) []byte {
	t.Helper()
	var envelope map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &envelope))
	var metadata map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope["adaptive_execution"], &metadata))
	mutate(metadata)
	encodedMetadata, err := json.Marshal(metadata)
	require.NoError(t, err)
	envelope["adaptive_execution"] = encodedMetadata
	encodedEnvelope, err := json.Marshal(envelope)
	require.NoError(t, err)
	return encodedEnvelope
}

func refreshAdaptiveExecutionCheckpointFingerprintForTest(t *testing.T, checkpoint *checkpointPO) {
	t.Helper()
	require.NotNil(t, checkpoint)
	metadata, err := decodeAdaptiveExecutionCheckpointMetadata(checkpoint.Metadata)
	require.NoError(t, err)
	fingerprint, err := adaptiveExecutionCheckpointFingerprint(checkpoint, metadata)
	require.NoError(t, err)
	checkpoint.Metadata = rewriteAdaptiveExecutionCheckpointMetadataForTest(
		t,
		checkpoint.Metadata,
		func(metadata map[string]json.RawMessage) {
			encoded, marshalErr := json.Marshal(fingerprint)
			require.NoError(t, marshalErr)
			metadata["checkpoint_fingerprint"] = encoded
		},
	)
}

func shrinkAdaptiveExecutionCheckpointAuthorityForTest(t *testing.T, db *gorm.DB, checkpointID int64) {
	t.Helper()
	var item agentRunPlanItemPO
	require.NoError(t, db.Where("id = ?", 60).First(&item).Error)
	itemFingerprint, err := adaptiveExecutionPlanItemFingerprint([]*entity.AgentRunPlanItem{item.toEntity()})
	require.NoError(t, err)
	var checkpoint checkpointPO
	require.NoError(t, db.Where("id = ?", checkpointID).First(&checkpoint).Error)
	checkpoint.Metadata = rewriteAdaptiveExecutionCheckpointMetadataForTest(
		t,
		checkpoint.Metadata,
		func(metadata map[string]json.RawMessage) {
			refs, marshalErr := json.Marshal([]adaptiveExecutionCheckpointItemRef{{
				ID: item.ID, TaskID: item.TaskID, Version: item.Version,
			}})
			require.NoError(t, marshalErr)
			fingerprint, marshalErr := json.Marshal(itemFingerprint)
			require.NoError(t, marshalErr)
			metadata["item_refs"] = refs
			metadata["item_fingerprint"] = fingerprint
		},
	)
	refreshAdaptiveExecutionCheckpointFingerprintForTest(t, &checkpoint)
	require.NoError(t, db.Model(&checkpointPO{}).Where("id = ?", checkpointID).
		Update("metadata", checkpoint.Metadata).Error)
}

func mutateAdaptiveSourceEventFieldForTest(t *testing.T, db *gorm.DB, field string) {
	t.Helper()
	updates := map[string]any{
		"thread_id": int64(11), "run_id": int64(29), "journal_run_id": int64(31),
		"attempt_id": "wrong-attempt", "sequence": uint64(2),
	}
	require.NoError(t, db.Model(&runEventPO{}).Where("id = ?", 7001).Update(field, updates[field]).Error)
}

type adaptiveExecutionDBSnapshotForTest struct {
	Runs        []runPO
	Attempts    []runAttemptPO
	Events      []runEventPO
	Checkpoints []checkpointPO
	Plans       []agentRunPlanPO
	Items       []agentRunPlanItemPO
}

func snapshotAdaptiveExecutionDBForTest(t *testing.T, db *gorm.DB) adaptiveExecutionDBSnapshotForTest {
	t.Helper()
	var result adaptiveExecutionDBSnapshotForTest
	require.NoError(t, db.Order("id ASC").Find(&result.Runs).Error)
	require.NoError(t, db.Order("id ASC").Find(&result.Attempts).Error)
	require.NoError(t, db.Order("id ASC").Find(&result.Events).Error)
	require.NoError(t, db.Order("id ASC").Find(&result.Checkpoints).Error)
	require.NoError(t, db.Order("run_id ASC").Find(&result.Plans).Error)
	require.NoError(t, db.Order("run_id ASC, task_id ASC").Find(&result.Items).Error)
	return result
}

type adaptiveExecutionMetadataForTest struct {
	SchemaVersion         string                               `json:"schema_version"`
	EventID               int64                                `json:"event_id"`
	EventSequence         uint64                               `json:"event_sequence"`
	JournalRunID          int64                                `json:"journal_run_id"`
	AttemptID             string                               `json:"attempt_id"`
	SourceAttemptID       *string                              `json:"source_attempt_id"`
	SourceCheckpointID    *int64                               `json:"source_checkpoint_id"`
	EventIdempotencyKey   string                               `json:"event_idempotency_key"`
	EventFingerprint      string                               `json:"event_fingerprint"`
	CheckpointFingerprint string                               `json:"checkpoint_fingerprint"`
	PlanScopeRunID        int64                                `json:"plan_scope_run_id"`
	PlanRevision          int64                                `json:"plan_revision"`
	ItemFingerprint       string                               `json:"item_fingerprint"`
	ItemRefs              []adaptiveExecutionCheckpointItemRef `json:"item_refs"`
	ExecutionRunID        int64                                `json:"execution_run_id"`
	ExecutionGeneration   uint64                               `json:"execution_generation"`
}

func newAdaptiveExecutionRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := newJournalRepositoryTestDB(t)
	require.NoError(t, db.AutoMigrate(&checkpointPO{}, &agentRunPlanPO{}, &agentRunPlanItemPO{}))
	return db
}

func seedAdaptiveExecutionInitialState(t *testing.T, db *gorm.DB) {
	t.Helper()
	seedJournalThread(t, db, 10)
	require.NoError(t, db.Create(&runPO{
		ID: 30, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		RunKind: string(entity.RunKindTask), Status: string(entity.RunStatusSucceeded),
		CreatedAt: 600, UpdatedAt: 600,
	}).Error)
	leaseOwner := "worker-1"
	leaseToken := "lease-1"
	leaseExpiresAt := int64(2000)
	require.NoError(t, db.Create(&runPO{
		ID: 20, ThreadID: 10, SpaceID: 10, CreatorID: 20,
		Status: string(entity.RunStatusRunning), ExecutionGeneration: 3,
		LeaseOwner: &leaseOwner, LeaseToken: &leaseToken, LeaseExpiresAt: &leaseExpiresAt,
		CreatedAt: 700, UpdatedAt: 700,
	}).Error)
	activeSlot := uint8(1)
	require.NoError(t, db.Create(&runAttemptPO{
		ID: 100, ThreadID: 10, JournalRunID: 30, ExecutionRunID: 20,
		AttemptID: "attempt-1", Ordinal: 1, Status: string(entity.RunAttemptStatusRunning),
		ActiveSlot: &activeSlot, NextSequence: 1, LastCommittedSequence: 0,
		EnrollmentVersion: entity.JournalSchemaVersion,
		ProjectionState:   string(entity.JournalProjectionStateHealthy),
		CreatedAt:         700, UpdatedAt: 700,
	}).Error)
	require.NoError(t, db.Create(&agentRunPlanPO{
		RunID: 20, ThreadID: 10, SpaceID: 10, UserID: 20,
		HighWatermark: 2, Revision: 1, CreatedAt: 800, UpdatedAt: 800,
	}).Error)
	item, err := agentRunPlanItemToPO(&entity.AgentRunPlanItem{
		ID: 60, RunID: 20, TaskID: 1, Subject: "major step", Description: "description",
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
		Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
		Active: true, Version: 1, CreatedAt: 900, UpdatedAt: 900,
	})
	require.NoError(t, err)
	require.NoError(t, db.Create(item).Error)
}

func adaptiveInitialBoundaryRequest(eventID, checkpointID, now int64) CommitAdaptiveExecutionBoundaryRequest {
	req := newValidAdaptiveExecutionMutationRequest()
	req.Event.ID = eventID
	req.Event.CreatedAt = now
	req.Checkpoint.ID = checkpointID
	req.Checkpoint.CreatedAt = now
	req.Now = now
	req.PlanMutation.Items = []AdaptivePlanItemMutation{
		{
			ExpectedVersion: 0,
			NextItem: &entity.AgentRunPlanItem{
				ID: 61, RunID: 20, TaskID: 2, Subject: "new major step", Description: "new description",
				Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "planning", Owner: "agent",
				Blocks: `[]`, BlockedBy: `[1]`, Metadata: `{"substeps_ref":"lazy:2"}`,
				Active: true, Version: 1,
			},
		},
		{
			ExpectedVersion: 1,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1, Subject: "updated major step", Description: "updated description",
				Status: entity.AgentRunPlanItemStatusInProgress, ActiveForm: "executing", Owner: "agent",
				Blocks: `{"z":2,"a":1}`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
				Active: true, Version: 2, CreatedAt: 123, UpdatedAt: 456,
			},
		},
	}
	return req
}

func cloneAdaptiveItemsForTest(items []*entity.AgentRunPlanItem) []*entity.AgentRunPlanItem {
	result := make([]*entity.AgentRunPlanItem, 0, len(items))
	for _, item := range items {
		clone := *item
		result = append(result, &clone)
	}
	return result
}

func adaptiveTaskIDsForTest(items []*entity.AgentRunPlanItem) []int64 {
	result := make([]int64, 0, len(items))
	for _, item := range items {
		result = append(result, item.TaskID)
	}
	return result
}

func decodeAdaptiveExecutionMetadataForTest(t *testing.T, raw []byte) adaptiveExecutionMetadataForTest {
	t.Helper()
	var envelope struct {
		AdaptiveExecution adaptiveExecutionMetadataForTest `json:"adaptive_execution"`
	}
	require.NoError(t, json.Unmarshal(raw, &envelope))
	return envelope.AdaptiveExecution
}

func extractJSONFieldForTest(t *testing.T, raw, field string) string {
	t.Helper()
	var object map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &object))
	value, exists := object[field]
	require.True(t, exists)
	return string(value)
}

func requireInt64PointerForTest(t *testing.T, value *int64) int64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireUint64PointerForTest(t *testing.T, value *uint64) uint64 {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func requireStringPointerForTest(t *testing.T, value *string) string {
	t.Helper()
	require.NotNil(t, value)
	return *value
}

func TestValidateAdaptiveExecutionMutationRequest(t *testing.T) {
	require.NoError(t, validateAdaptiveExecutionMutationRequest(newValidAdaptiveExecutionMutationRequest()))

	tests := []struct {
		name   string
		mutate func(*CommitAdaptiveExecutionBoundaryRequest)
	}{
		{name: "nil mutation", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation = nil }},
		{name: "plan scope", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.PlanScopeRunID = 0 }},
		{name: "expected revision", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.ExpectedRevision = 0 }},
		{name: "revision continuity", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.NextRevision++ }},
		{name: "zero items", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items = nil }},
		{name: "too many items", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			item := req.PlanMutation.Items[0]
			req.PlanMutation.Items = make([]AdaptivePlanItemMutation, 33)
			for index := range req.PlanMutation.Items {
				next := *item.NextItem
				next.ID += int64(index)
				next.TaskID += int64(index)
				req.PlanMutation.Items[index] = AdaptivePlanItemMutation{ExpectedVersion: item.ExpectedVersion, NextItem: &next}
			}
		}},
		{name: "duplicate item id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			duplicate := *req.PlanMutation.Items[0].NextItem
			duplicate.TaskID++
			req.PlanMutation.Items = append(req.PlanMutation.Items, AdaptivePlanItemMutation{ExpectedVersion: 1, NextItem: &duplicate})
		}},
		{name: "duplicate task id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			duplicate := *req.PlanMutation.Items[0].NextItem
			duplicate.ID++
			req.PlanMutation.Items = append(req.PlanMutation.Items, AdaptivePlanItemMutation{ExpectedVersion: 1, NextItem: &duplicate})
		}},
		{name: "negative expected version", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].ExpectedVersion = -1 }},
		{name: "nil next item", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem = nil }},
		{name: "item id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.ID = 0 }},
		{name: "item run id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.RunID = 0 }},
		{name: "item task id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.TaskID = 0 }},
		{name: "item scope drift", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.RunID++ }},
		{name: "item version continuity", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.PlanMutation.Items[0].NextItem.Version++ }},
		{name: "event id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.ID = 0 }},
		{name: "event type", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.EventType = "" }},
		{name: "event timestamp", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Event.CreatedAt++ }},
		{name: "checkpoint id", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.ID = 0 }},
		{name: "checkpoint namespace", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.CheckpointNS = "" }},
		{name: "checkpoint timestamp", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.CreatedAt++ }},
		{name: "runtime type", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeType = "legacy" }},
		{name: "runtime key", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeKey = "" }},
		{name: "envelope version", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.EnvelopeVersion = 0 }},
		{name: "runtime deleted", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.RuntimeDeletedAt = 1 }},
		{name: "metadata non object", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Checkpoint.Metadata = `[]` }},
		{name: "reserved metadata namespace", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) {
			req.Checkpoint.Metadata = `{"adaptive_execution":{}}`
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newValidAdaptiveExecutionMutationRequest()
			tt.mutate(&req)
			require.ErrorIs(t, validateAdaptiveExecutionMutationRequest(req), ErrAdaptiveExecutionBoundaryInvalid)
		})
	}
}

func newValidAdaptiveExecutionMutationRequest() CommitAdaptiveExecutionBoundaryRequest {
	req := newAdaptiveExecutionBoundaryRequestForFenceTest()
	req.Event.EventType = "run.boundary"
	req.Event.Payload = `{"step":1}`
	req.Event.CreatedAt = req.Now
	req.Checkpoint.CheckpointNS = "adaptive"
	req.Checkpoint.RuntimeType = "eino_adk"
	req.Checkpoint.RuntimeKey = "thread:10:run:20"
	req.Checkpoint.EnvelopeVersion = 1
	req.Checkpoint.ChannelValues = `{}`
	req.Checkpoint.ChannelVersions = `{}`
	req.Checkpoint.PendingSends = `[]`
	req.Checkpoint.Metadata = `{"runtime_field":"preserved"}`
	req.Checkpoint.CreatedAt = req.Now
	req.PlanMutation = &AdaptivePlanMutation{
		PlanScopeRunID:   20,
		ExpectedRevision: 1,
		NextRevision:     2,
		Items: []AdaptivePlanItemMutation{{
			ExpectedVersion: 1,
			NextItem: &entity.AgentRunPlanItem{
				ID: 60, RunID: 20, TaskID: 1,
				Subject: "major step", Description: "execute the step",
				Status:     entity.AgentRunPlanItemStatusInProgress,
				ActiveForm: "executing", Owner: "agent",
				Blocks: `[]`, BlockedBy: `[]`, Metadata: `{"substeps_ref":"lazy:1"}`,
				Active: true, Version: 2, CreatedAt: req.Now - 1, UpdatedAt: req.Now,
			},
		}},
	}
	return req
}

func newAdaptivePlanItem(id, runID, taskID, version, now int64) *entity.AgentRunPlanItem {
	return &entity.AgentRunPlanItem{
		ID: id, RunID: runID, TaskID: taskID,
		Subject: fmt.Sprintf("step-%d", taskID), Description: fmt.Sprintf("description-%d", taskID),
		Status: entity.AgentRunPlanItemStatusPending, ActiveForm: "working", Owner: "agent",
		Blocks: `[]`, BlockedBy: `[]`, Metadata: `{}`, Active: true,
		Version: version, CreatedAt: now, UpdatedAt: now,
	}
}

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
		{name: "attempt id too long", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.AttemptID = strings.Repeat("a", 65) }},
		{name: "attempt id multibyte too long", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.AttemptID = strings.Repeat("界", 22) }},
		{name: "generation", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Generation = 0 }},
		{name: "lease owner", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.LeaseOwner = "" }},
		{name: "lease token", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.LeaseToken = "" }},
		{name: "now", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.Now = 0 }},
		{name: "idempotency key", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.IdempotencyKey = "" }},
		{name: "idempotency key too long", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.IdempotencyKey = strings.Repeat("k", 192) }},
		{name: "idempotency key multibyte too long", mutate: func(req *CommitAdaptiveExecutionBoundaryRequest) { req.IdempotencyKey = strings.Repeat("界", 64) }},
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

func TestValidateAdaptiveExecutionRecoverySourceRequest(t *testing.T) {
	valid := ReadAdaptiveExecutionRecoverySourceRequest{
		ThreadID: 10, JournalRunID: 30, TargetAttemptID: "attempt-2",
	}
	require.NoError(t, validateAdaptiveExecutionRecoverySourceRequest(valid))

	tests := []struct {
		name   string
		mutate func(*ReadAdaptiveExecutionRecoverySourceRequest)
	}{
		{name: "thread id", mutate: func(req *ReadAdaptiveExecutionRecoverySourceRequest) { req.ThreadID = 0 }},
		{name: "journal run id", mutate: func(req *ReadAdaptiveExecutionRecoverySourceRequest) { req.JournalRunID = 0 }},
		{name: "blank target attempt", mutate: func(req *ReadAdaptiveExecutionRecoverySourceRequest) { req.TargetAttemptID = " \t" }},
		{name: "target attempt too long", mutate: func(req *ReadAdaptiveExecutionRecoverySourceRequest) { req.TargetAttemptID = strings.Repeat("a", 65) }},
		{name: "target attempt multibyte too long", mutate: func(req *ReadAdaptiveExecutionRecoverySourceRequest) { req.TargetAttemptID = strings.Repeat("界", 22) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := valid
			tt.mutate(&req)
			require.ErrorIs(t, validateAdaptiveExecutionRecoverySourceRequest(req), ErrAdaptiveExecutionBoundaryInvalid)
		})
	}

	result, err := NewAdaptiveExecutionRepository(nil).ReadAdaptiveExecutionRecoverySource(context.Background(), valid)
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAdaptiveExecutionRecoveryConflict)
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
