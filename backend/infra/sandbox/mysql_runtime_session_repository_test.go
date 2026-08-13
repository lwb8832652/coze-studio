// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm"
)

func TestMySQLRuntimeSessionGenerationSensitiveUpdatesLockSingletonFirst(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	ref := domainsandbox.SessionRef{
		SessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942",
		Key: domainsandbox.SessionKey{
			DeploymentID: "runner-dev-a", ProviderID: 1, SpaceID: 42, UserID: 43,
			ThreadID: "thread-a", Profile: domainsandbox.SessionProfileCore,
		},
		RuntimeGeneration: 1,
	}
	tests := map[string]func(*MySQLRepository) error{
		"bind": func(repository *MySQLRepository) error {
			_, err := repository.BindRuntimeSessionCAS(context.Background(), domainsandbox.BindRuntimeSessionInput{
				Ref: ref, ExpectedVersion: 1, UpstreamShellID: "shell-a",
				ExpiresAt: now.Add(time.Minute), Now: now,
			})
			return err
		},
		"recover": func(repository *MySQLRepository) error {
			_, err := repository.TransitionRuntimeSessionCAS(context.Background(), domainsandbox.TransitionRuntimeSessionInput{
				Ref: ref, ExpectedVersion: 1, Action: domainsandbox.SessionActionRecover,
				NextRuntimeGeneration: 2, UpstreamShellID: "shell-a",
				ExpiresAt: now.Add(time.Minute), Now: now,
			})
			return err
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			repository, mock := newProviderCreateTransactionRepository(t)
			mock.ExpectBegin()
			mock.ExpectQuery(regexp.QuoteMeta("SELECT `id`,`aio_runtime_generation`,`aio_runtime_deployment_id`,`aio_runtime_sentinel_id` FROM `sandbox_scheduler_settings` WHERE id = ? ORDER BY `sandbox_scheduler_settings`.`id` LIMIT ? FOR UPDATE")).
				WithArgs(1, 1).
				WillReturnRows(sqlmock.NewRows([]string{"id", "aio_runtime_generation", "aio_runtime_deployment_id", "aio_runtime_sentinel_id"}).
					AddRow(1, 3, "runner-dev-a", "newx-generation-0123456789abcdef0123456789abcdef"))
			mock.ExpectRollback()

			if err := run(repository); !errors.Is(err, domainsandbox.ErrVersionConflict) {
				t.Fatalf("generation-sensitive update error = %v, want ErrVersionConflict", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("generation-sensitive lock order = %v", err)
			}
		})
	}
}

func TestMySQLRuntimeSessionAcquireIsIdempotentAndTenantBound(t *testing.T) {
	repository, db := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	firstInput := validAcquireRuntimeSessionInput(now)

	first, err := repository.AcquireRuntimeSession(ctx, firstInput)
	if err != nil {
		t.Fatalf("AcquireRuntimeSession(first) error = %v", err)
	}
	secondInput := firstInput
	secondInput.CandidateSessionID = "ed28cd36-e62b-42ae-9f20-08f7aa6dc886"
	second, err := repository.AcquireRuntimeSession(ctx, secondInput)
	if err != nil {
		t.Fatalf("AcquireRuntimeSession(second) error = %v", err)
	}
	if !reflect.DeepEqual(second, first) || first.Ref.SessionID != firstInput.CandidateSessionID ||
		first.State != domainsandbox.SessionStateActive || first.Version != domainsandbox.InitialVersion {
		t.Fatalf("idempotent sessions = %#v / %#v", first, second)
	}

	differentThread := firstInput
	differentThread.Key.ThreadID = "thread-b"
	differentThread.CandidateSessionID = "ac47b7ba-30fe-41a8-9ec1-69a7dc873cbf"
	third, err := repository.AcquireRuntimeSession(ctx, differentThread)
	if err != nil || third.Ref.SessionID == first.Ref.SessionID {
		t.Fatalf("AcquireRuntimeSession(different thread) = %#v, %v", third, err)
	}

	wrongTenant := first.Ref
	wrongTenant.Key.UserID++
	if _, err := repository.GetRuntimeSession(ctx, wrongTenant); !errors.Is(err, domainsandbox.ErrSessionNotFound) {
		t.Fatalf("GetRuntimeSession(wrong tenant) error = %v, want ErrSessionNotFound", err)
	}
	var count int64
	if err := db.Model(&runtimeSessionPO{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("runtime session rows = %d, %v", count, err)
	}
}

func TestMySQLRuntimeSessionRejectsCandidateUUIDCollisionAcrossBusinessKeys(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	input := validAcquireRuntimeSessionInput(now)
	if _, err := repository.AcquireRuntimeSession(ctx, input); err != nil {
		t.Fatalf("AcquireRuntimeSession(first) error = %v", err)
	}
	input.Key.ThreadID = "thread-b"
	if _, err := repository.AcquireRuntimeSession(ctx, input); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("AcquireRuntimeSession(candidate collision) error = %v, want ErrVersionConflict", err)
	}
}

func TestMySQLRuntimeSessionBindAndLifecycleCAS(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	bound, err := repository.BindRuntimeSessionCAS(ctx, domainsandbox.BindRuntimeSessionInput{
		Ref: created.Ref, ExpectedVersion: created.Version, UpstreamShellID: "shell-opaque-a",
		ExpiresAt: now.Add(20 * time.Minute), Now: now.Add(time.Second),
	})
	if err != nil || bound.UpstreamShellID != "shell-opaque-a" || bound.Version != created.Version+1 {
		t.Fatalf("BindRuntimeSessionCAS() = %#v, %v", bound, err)
	}
	if _, err := repository.BindRuntimeSessionCAS(ctx, domainsandbox.BindRuntimeSessionInput{
		Ref: created.Ref, ExpectedVersion: created.Version, UpstreamShellID: "shell-loser",
		ExpiresAt: now.Add(20 * time.Minute), Now: now.Add(2 * time.Second),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale BindRuntimeSessionCAS() error = %v", err)
	}

	released, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: bound.Ref, ExpectedVersion: bound.Version, Action: domainsandbox.SessionActionRelease,
		Now: now.Add(3 * time.Second),
	})
	if err != nil || released.State != domainsandbox.SessionStateReleased || released.UpstreamShellID != "" || released.Version != bound.Version+1 {
		t.Fatalf("release = %#v, %v", released, err)
	}
	if _, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: released.Ref, ExpectedVersion: bound.Version, Action: domainsandbox.SessionActionDestroy,
		Now: now.Add(4 * time.Second),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale transition error = %v", err)
	}
	reacquiredInput := validAcquireRuntimeSessionInput(now.Add(5 * time.Second))
	reacquiredInput.CandidateSessionID = "ed28cd36-e62b-42ae-9f20-08f7aa6dc886"
	reacquired, err := repository.AcquireRuntimeSession(ctx, reacquiredInput)
	if err != nil || reacquired.Ref.SessionID != released.Ref.SessionID ||
		reacquired.State != domainsandbox.SessionStateActive || reacquired.Version != released.Version+1 {
		t.Fatalf("reacquire released session = %#v, %v", reacquired, err)
	}
}

func TestMySQLRuntimeSessionAcquireAndRecoverFailClosedOnStaleGeneration(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	firstGeneration := activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	secondGeneration, replaced, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID: "runner-dev-a", ExpectedSentinelID: firstGeneration.SentinelID,
		CandidateSentinelID: "newx-generation-fedcba9876543210fedcba9876543210",
	})
	if err != nil || !replaced || secondGeneration.Generation != 2 {
		t.Fatalf("advance generation = %#v, %t, %v", secondGeneration, replaced, err)
	}
	staleAcquire := validAcquireRuntimeSessionInput(now.Add(time.Minute))
	if _, err := repository.AcquireRuntimeSession(ctx, staleAcquire); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale same-key acquire error = %v, want ErrVersionConflict", err)
	}
	recovering, err := repository.GetRuntimeSession(ctx, created.Ref)
	if err != nil {
		t.Fatalf("GetRuntimeSession(recovering) error = %v", err)
	}
	if _, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: recovering.Ref, ExpectedVersion: recovering.Version, Action: domainsandbox.SessionActionRecover,
		NextRuntimeGeneration: secondGeneration.Generation + 1, UpstreamShellID: "shell-future",
		ExpiresAt: now.Add(20 * time.Minute), Now: now.Add(2 * time.Minute),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("future generation recover error = %v, want ErrVersionConflict", err)
	}
	recovered, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: recovering.Ref, ExpectedVersion: recovering.Version, Action: domainsandbox.SessionActionRecover,
		NextRuntimeGeneration: secondGeneration.Generation, UpstreamShellID: "shell-current",
		ExpiresAt: now.Add(20 * time.Minute), Now: now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("current generation recover error = %v", err)
	}
	if _, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: recovered.Ref, ExpectedVersion: recovered.Version, Action: domainsandbox.SessionActionRecover,
		NextRuntimeGeneration: secondGeneration.Generation, UpstreamShellID: "shell-different",
		ExpiresAt: now.Add(20 * time.Minute), Now: now.Add(3 * time.Minute),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("mismatched idempotent recover error = %v, want ErrVersionConflict", err)
	}
}

func TestMySQLRuntimeSessionDestroyedIdentityCannotBeReacquired(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	destroyed, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: created.Ref, ExpectedVersion: created.Version, Action: domainsandbox.SessionActionDestroy,
		Now: now.Add(time.Minute),
	})
	if err != nil || destroyed.State != domainsandbox.SessionStateDestroyed {
		t.Fatalf("destroy = %#v, %v", destroyed, err)
	}
	reacquire := validAcquireRuntimeSessionInput(now.Add(2 * time.Minute))
	reacquire.CandidateSessionID = "ed28cd36-e62b-42ae-9f20-08f7aa6dc886"
	if _, err := repository.AcquireRuntimeSession(ctx, reacquire); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("destroyed reacquire error = %v, want ErrVersionConflict", err)
	}
}

func TestMySQLRuntimeSessionLifecycleIdempotenceDoesNotBumpVersion(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	released, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: created.Ref, ExpectedVersion: created.Version, Action: domainsandbox.SessionActionRelease,
		Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("release error = %v", err)
	}
	idempotentRelease, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: released.Ref, ExpectedVersion: released.Version, Action: domainsandbox.SessionActionRelease,
		Now: now.Add(2 * time.Minute),
	})
	if err != nil || idempotentRelease.Version != released.Version || idempotentRelease.UpdatedAt != released.UpdatedAt {
		t.Fatalf("idempotent release = %#v, %v; want version/time unchanged", idempotentRelease, err)
	}
	destroyed, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: released.Ref, ExpectedVersion: released.Version, Action: domainsandbox.SessionActionDestroy,
		Now: now.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("destroy error = %v", err)
	}
	idempotentDestroy, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: destroyed.Ref, ExpectedVersion: destroyed.Version, Action: domainsandbox.SessionActionDestroy,
		Now: now.Add(4 * time.Minute),
	})
	if err != nil || idempotentDestroy.Version != destroyed.Version || idempotentDestroy.UpdatedAt != destroyed.UpdatedAt {
		t.Fatalf("idempotent destroy = %#v, %v; want version/time unchanged", idempotentDestroy, err)
	}
}

func TestMySQLRuntimeSessionReleaseClearsRecoveryReason(t *testing.T) {
	repository, db := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	if err := db.Model(&runtimeSessionPO{}).Where("session_id = ?", created.Ref.SessionID).UpdateColumn("recovery_reason", "stale_reason").Error; err != nil {
		t.Fatalf("seed stale recovery reason: %v", err)
	}
	released, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: created.Ref, ExpectedVersion: created.Version, Action: domainsandbox.SessionActionRelease,
		Now: now.Add(time.Minute),
	})
	if err != nil || released.RecoveryReason != "" {
		t.Fatalf("release with stale reason = %#v, %v; want cleared reason", released, err)
	}
}

func TestMySQLRuntimeSessionRecoveringReasonIsIdempotentOnlyWhenEqual(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	recovering, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: created.Ref, ExpectedVersion: created.Version, Action: domainsandbox.SessionActionMarkRecovering,
		RecoveryReason: "lease_lost", Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("mark recovering error = %v", err)
	}
	if _, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: recovering.Ref, ExpectedVersion: recovering.Version, Action: domainsandbox.SessionActionMarkRecovering,
		RecoveryReason: "different_reason", Now: now.Add(2 * time.Minute),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("different idempotency reason error = %v, want ErrVersionConflict", err)
	}
}

func TestMySQLRuntimeSessionRejectsCorruptPersistedState(t *testing.T) {
	repository, db := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	if err := db.Model(&runtimeSessionPO{}).Where("session_id = ?", created.Ref.SessionID).UpdateColumns(map[string]any{
		"state": "released", "upstream_shell_id": "shell-corrupt",
	}).Error; err != nil {
		t.Fatalf("corrupt runtime session fixture: %v", err)
	}
	if _, err := repository.GetRuntimeSession(ctx, created.Ref); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("GetRuntimeSession(corrupt) error = %v, want ErrConfigurationInvalid", err)
	}
}

func TestMySQLRuntimeSessionRejectsOversizedPersistedRecoveryReason(t *testing.T) {
	repository, db := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}
	if err := db.Model(&runtimeSessionPO{}).Where("session_id = ?", created.Ref.SessionID).UpdateColumns(map[string]any{
		"state": "recovering", "recovery_reason": strings.Repeat("a", domainsandbox.MaxSessionRecoveryReasonBytes+1),
	}).Error; err != nil {
		t.Fatalf("corrupt runtime session fixture: %v", err)
	}
	if _, err := repository.GetRuntimeSession(ctx, created.Ref); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("GetRuntimeSession(oversized reason) error = %v, want ErrConfigurationInvalid", err)
	}
}

func TestMySQLAIOGenerationRejectsCorruptSingletonTuple(t *testing.T) {
	repository, db := newSQLiteRuntimeSessionRepository(t)
	if err := db.Model(&schedulerSettingsPO{}).Where("id = ?", 1).UpdateColumns(map[string]any{
		"aio_runtime_generation": 1,
	}).Error; err != nil {
		t.Fatalf("corrupt generation fixture: %v", err)
	}
	if _, err := repository.GetAIOGeneration(context.Background(), "runner-dev-a"); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("GetAIOGeneration(corrupt) error = %v, want ErrConfigurationInvalid", err)
	}
	if _, _, err := repository.CompareAndReplaceAIOSentinel(context.Background(), domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID: "runner-dev-a", CandidateSentinelID: "newx-generation-0123456789abcdef0123456789abcdef",
	}); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("CompareAndReplaceAIOSentinel(corrupt) error = %v, want ErrConfigurationInvalid", err)
	}
}

func TestMySQLAIOGenerationCASOwnsSingletonAndMarksOldSessionsRecovering(t *testing.T) {
	repository, _ := newSQLiteRuntimeSessionRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	deploymentID := "runner-dev-a"
	firstCandidate := "newx-generation-0123456789abcdef0123456789abcdef"
	state, replaced, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID: deploymentID, CandidateSentinelID: firstCandidate,
	})
	if err != nil || !replaced || state.Generation != 1 || state.SentinelID != firstCandidate {
		t.Fatalf("initial CompareAndReplaceAIOSentinel() = %#v, %t, %v", state, replaced, err)
	}
	created, err := repository.AcquireRuntimeSession(ctx, validAcquireRuntimeSessionInput(now))
	if err != nil {
		t.Fatalf("AcquireRuntimeSession() error = %v", err)
	}

	loserState, replaced, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID:        created.Ref.Key.DeploymentID,
		CandidateSentinelID: "newx-generation-11111111111111111111111111111111",
	})
	if err != nil || replaced || loserState != state {
		t.Fatalf("loser CompareAndReplaceAIOSentinel() = %#v, %t, %v", loserState, replaced, err)
	}
	if _, _, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID: "runner-other", ExpectedSentinelID: firstCandidate,
		CandidateSentinelID: "newx-generation-22222222222222222222222222222222",
	}); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("owner mismatch error = %v, want ErrConfigurationInvalid", err)
	}

	nextCandidate := "newx-generation-fedcba9876543210fedcba9876543210"
	next, replaced, err := repository.CompareAndReplaceAIOSentinel(ctx, domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID: created.Ref.Key.DeploymentID, ExpectedSentinelID: firstCandidate,
		CandidateSentinelID: nextCandidate,
	})
	if err != nil || !replaced || next.Generation != 2 || next.SentinelID != nextCandidate {
		t.Fatalf("replacement CompareAndReplaceAIOSentinel() = %#v, %t, %v", next, replaced, err)
	}
	got, err := repository.GetRuntimeSession(ctx, created.Ref)
	if err != nil || got.State != domainsandbox.SessionStateRecovering || got.RecoveryReason != "aio_runtime_restarted" || got.Version != created.Version+1 {
		t.Fatalf("generation recovery session = %#v, %v", got, err)
	}
	recoverable, err := repository.ListRecoverableRuntimeSessions(ctx, domainsandbox.ListRecoverableRuntimeSessionsInput{
		DeploymentID: created.Ref.Key.DeploymentID, BeforeGeneration: next.Generation, Limit: 10,
	})
	if err != nil || len(recoverable) != 1 || recoverable[0].Ref.SessionID != created.Ref.SessionID {
		t.Fatalf("ListRecoverableRuntimeSessions() = %#v, %v", recoverable, err)
	}
	recovered, err := repository.TransitionRuntimeSessionCAS(ctx, domainsandbox.TransitionRuntimeSessionInput{
		Ref: got.Ref, ExpectedVersion: got.Version, Action: domainsandbox.SessionActionRecover,
		NextRuntimeGeneration: next.Generation, UpstreamShellID: "shell-recovered",
		ExpiresAt: now.Add(20 * time.Minute), Now: now.Add(time.Minute),
	})
	if err != nil || recovered.Ref.RuntimeGeneration != next.Generation || recovered.State != domainsandbox.SessionStateActive || recovered.UpstreamShellID != "shell-recovered" {
		t.Fatalf("recover to current generation = %#v, %v", recovered, err)
	}
	stale := validAcquireRuntimeSessionInput(now)
	stale.Key.ThreadID = "thread-stale"
	stale.CandidateSessionID = "ac47b7ba-30fe-41a8-9ec1-69a7dc873cbf"
	if _, err := repository.AcquireRuntimeSession(ctx, stale); !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale generation acquire error = %v, want ErrVersionConflict", err)
	}
}

func TestMySQLRuntimeSessionAcquireConcurrentCallersShareWinner(t *testing.T) {
	repository, db := newSQLiteRuntimeSessionRepository(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQLite sql.DB: %v", err)
	}
	// Shared-cache SQLite returns SQLITE_LOCKED instead of exercising MySQL's
	// duplicate-key loser path. One connection still verifies concurrent API
	// callers are serialized to the same persisted winner; the real MySQL gate
	// covers independent database connections.
	sqlDB.SetMaxOpenConns(1)
	ctx := context.Background()
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	activateTestAIOGeneration(t, repository, "runner-dev-a")
	inputs := []domainsandbox.AcquireRuntimeSessionInput{
		validAcquireRuntimeSessionInput(now),
		validAcquireRuntimeSessionInput(now),
	}
	inputs[1].CandidateSessionID = "ed28cd36-e62b-42ae-9f20-08f7aa6dc886"
	type result struct {
		session domainsandbox.RuntimeSession
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, len(inputs))
	for _, input := range inputs {
		go func(candidate domainsandbox.AcquireRuntimeSessionInput) {
			<-start
			session, err := repository.AcquireRuntimeSession(ctx, candidate)
			results <- result{session: session, err: err}
		}(input)
	}
	close(start)
	first := <-results
	second := <-results
	if first.err != nil || second.err != nil || first.session.Ref.SessionID != second.session.Ref.SessionID {
		t.Fatalf("concurrent acquire = %#v / %#v", first, second)
	}
	var count int64
	if err := db.Model(&runtimeSessionPO{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("concurrent runtime session rows = %d, %v", count, err)
	}
}

func newSQLiteRuntimeSessionRepository(t *testing.T) (*MySQLRepository, *gorm.DB) {
	t.Helper()
	repository, db := newSQLiteRepository(t)
	if err := db.AutoMigrate(&schedulerSettingsPO{}, &runtimeSessionPO{}); err != nil {
		t.Fatalf("migrate runtime session test schema: %v", err)
	}
	if err := db.Create(&schedulerSettingsPO{
		ID: 1, SettingsJSON: mustSchedulerSettingsJSON(t), Version: domainsandbox.InitialVersion,
		CreatedAt: persistenceNow(), UpdatedAt: persistenceNow(),
	}).Error; err != nil {
		t.Fatalf("seed scheduler singleton: %v", err)
	}
	return repository, db
}

func validAcquireRuntimeSessionInput(now time.Time) domainsandbox.AcquireRuntimeSessionInput {
	return domainsandbox.AcquireRuntimeSessionInput{
		Key: domainsandbox.SessionKey{
			DeploymentID: "runner-dev-a", ProviderID: 1, SpaceID: 42, UserID: 43,
			ThreadID: "thread-a", Profile: domainsandbox.SessionProfileCore,
		},
		CandidateSessionID: "91df5ac2-cf99-461f-b1a4-44fdd067b942",
		RuntimeGeneration:  1, ExpiresAt: now.Add(20 * time.Minute), Now: now,
	}
}

func activateTestAIOGeneration(t *testing.T, repository *MySQLRepository, deploymentID string) domainsandbox.AIOGenerationState {
	t.Helper()
	state, replaced, err := repository.CompareAndReplaceAIOSentinel(context.Background(), domainsandbox.CompareAndReplaceAIOSentinelInput{
		DeploymentID:        deploymentID,
		CandidateSentinelID: "newx-generation-0123456789abcdef0123456789abcdef",
	})
	if err != nil || !replaced || state.Generation != 1 {
		t.Fatalf("activate AIO generation = %#v, %t, %v", state, replaced, err)
	}
	return state
}
