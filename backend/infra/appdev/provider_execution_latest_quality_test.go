// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"gorm.io/gorm/clause"
)

func TestProviderExecutionLatestProviderLeaseUsesDBClock(t *testing.T) {
	for _, nodeNow := range []time.Time{
		time.Date(2036, 7, 16, 0, 0, 0, 0, time.UTC),
		time.Date(2016, 7, 16, 0, 0, 0, 0, time.UTC),
	} {
		t.Run(nodeNow.Format("2006"), func(t *testing.T) {
			dbNow := time.Date(2026, 7, 17, 1, 0, 0, 123456000, time.UTC)
			repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
			setProviderExecutionDBTime(t, repository, dbNow)
			service, err := applicationappdev.NewProviderExecutionService(
				repository, &providerExecutionLostResponseCodec{},
				applicationappdev.WithProviderExecutionClock(func() time.Time { return nodeNow }),
				applicationappdev.WithProviderExecutionRandom(bytes.NewReader(bytes.Repeat([]byte{0x61}, 128))),
			)
			if err != nil {
				t.Fatal(err)
			}
			started, err := service.EnsureStart(context.Background(), applicationappdev.EnsureProviderExecutionStartRequest{
				SpaceID: "1001", ProjectID: "project-a", IdempotencyKey: "lease-" + nodeNow.Format("2006"),
				ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
			})
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := service.ClaimRecovery(context.Background(), applicationappdev.ClaimProviderExecutionRecoveryRequest{
				SpaceID: started.SpaceID, ProjectID: started.ProjectID, Generation: started.Generation, ExpectedVersion: started.Version,
			})
			if err != nil {
				t.Fatal(err)
			}
			submitting, err := service.StartProviderSubmission(
				context.Background(),
				providerExecutionServiceStartSubmissionRequest(
					claimed.Owner(), claimed.Metadata.Version, "provider-lease-launch",
				),
			)
			if err != nil {
				t.Fatal(err)
			}
			submitting, err = service.MarkProviderLaunchSubmitted(context.Background(), applicationappdev.MarkProviderExecutionLaunchSubmittedRequest{
				Owner: claimed.Owner(), ExpectedVersion: submitting.Version, OperationID: "provider-lease-launch",
				DispatchLeaseDuration: time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			running, err := service.SaveProviderSubmission(context.Background(), applicationappdev.SaveProviderExecutionSubmissionRequest{
				Owner: claimed.Owner(), ExpectedVersion: submitting.Version,
				LaunchOperationID:   "provider-lease-launch",
				ProviderExecutionID: "provider-lease-execution", ObservedState: domainappdev.ProviderExecutionObservedRunning,
				ProviderLeaseDuration: 2 * time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			if running.ProviderLeaseExpiresAt == nil || !running.ProviderLeaseExpiresAt.Equal(dbNow.Add(2*time.Minute)) || !running.UpdatedAt.Equal(dbNow) {
				t.Fatalf("provider lease/updated_at = %v/%v, DB now %v", running.ProviderLeaseExpiresAt, running.UpdatedAt, dbNow)
			}
		})
	}
}

func TestProviderExecutionLatestZeroRowUpdatesAreClassified(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	dbNow := time.Date(2026, 7, 17, 2, 0, 0, 0, time.UTC)
	setProviderExecutionDBTime(t, repository, dbNow)
	record, ownerHash := claimProviderExecutionForLatestTest(t, repository, "zero-row")
	cas := providerExecutionCAS(record, ownerHash, time.Time{})

	if result, err := repository.SaveSubmission(
		context.Background(),
		providerExecutionSaveSubmissionInput(
			t, cas, "wrong-state-launch", "wrong-state-execution", time.Minute, "",
		),
	); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("submission wrong state = %#v, %v", result, err)
	}
	if result, err := repository.AbortCheckpointWrite(context.Background(), domainappdev.AbortProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: testOperationHash(t, "missing-abort-operation"), CheckpointWriteRevision: 1,
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("missing abort = %#v, %v", result, err)
	}

	operationHash := testOperationHash(t, "zero-row-checkpoint")
	reservation, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	reservedCAS := cas
	reservedCAS.ExpectedVersion = reservation.ReservedVersion
	wrongOperation := testOperationHash(t, "wrong-checkpoint-operation")
	for name, input := range map[string]domainappdev.SaveProviderCheckpointInput{
		"operation": {OwnerCAS: reservedCAS, OperationHash: wrongOperation, CheckpointWriteRevision: reservation.Revision, CheckpointEnvelope: "ecp1:wrong-operation"},
		"revision":  {OwnerCAS: reservedCAS, OperationHash: operationHash, CheckpointWriteRevision: reservation.Revision + 1, CheckpointEnvelope: "ecp1:wrong-revision"},
	} {
		if result, err := repository.SaveCheckpoint(context.Background(), input); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
			t.Fatalf("checkpoint wrong %s = %#v, %v", name, result, err)
		}
	}
	aborted, err := repository.AbortCheckpointWrite(context.Background(), domainappdev.AbortProviderCheckpointWriteInput{
		OwnerCAS: reservedCAS, OperationHash: operationHash, CheckpointWriteRevision: reservation.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	repeatAbortCAS := providerExecutionCAS(aborted, ownerHash, time.Time{})
	if result, err := repository.AbortCheckpointWrite(context.Background(), domainappdev.AbortProviderCheckpointWriteInput{
		OwnerCAS: repeatAbortCAS, OperationHash: operationHash, CheckpointWriteRevision: reservation.Revision,
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("repeated abort = %#v, %v", result, err)
	}
}

func TestProviderExecutionLatestCleanupUsesDedicatedEntrypoint(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	setProviderExecutionDBTime(t, repository, time.Date(2026, 7, 17, 3, 0, 0, 0, time.UTC))
	record, ownerHash := claimProviderExecutionForLatestTest(t, repository, "cleanup-entry")
	cas := providerExecutionCAS(record, ownerHash, time.Time{})

	if result, err := repository.AdvanceTerminal(context.Background(), domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS: cas, OperationHash: testOperationHash(t, "invalid-cleanup-terminal-operation"), ObservedState: domainappdev.ProviderExecutionObservedCleanupPending,
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("generic cleanup transition = %#v, %v", result, err)
	}
	if result, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{OwnerCAS: cas}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("desired=run BeginCleanup = %#v, %v", result, err)
	}
	stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: cas})
	if err != nil {
		t.Fatal(err)
	}
	cleanupPending, err := repository.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(stopping, ownerHash, time.Time{}),
	})
	if err != nil || cleanupPending.ObservedState != domainappdev.ProviderExecutionObservedCleanupPending {
		t.Fatalf("dedicated BeginCleanup = %#v, %v", cleanupPending, err)
	}
	completed, err := repository.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS:    providerExecutionCAS(cleanupPending, ownerHash, time.Time{}),
		OperationID: "cleanup-entry-complete",
	})
	if err != nil || completed.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete {
		t.Fatalf("CompleteCleanup = %#v, %v", completed, err)
	}
	if result, err := repository.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(cleanupPending, ownerHash, time.Time{}), OperationID: "cleanup-entry-wrong",
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("cleanup different operation = %#v, %v", result, err)
	}
}

func TestProviderExecutionLatestClaimRejectsNonRecoverableWithoutMutation(t *testing.T) {
	for _, terminal := range []domainappdev.ProviderExecutionObservedState{domainappdev.ProviderExecutionObservedCleanupComplete} {
		t.Run(string(terminal), func(t *testing.T) {
			repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
			clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 17, 4, 0, 0, 0, time.UTC)}
			repository.clock = clock
			record, ownerHash := claimProviderExecutionForLatestTest(t, repository, "claim-"+string(terminal))
			cas := providerExecutionCAS(record, ownerHash, time.Time{})
			if terminal == domainappdev.ProviderExecutionObservedFailed {
				submitting, err := repository.StartSubmission(
					context.Background(),
					providerExecutionStartSubmissionInput(t, cas, "latest-quality-launch-"+string(terminal)),
				)
				if err != nil {
					t.Fatal(err)
				}
				submitting = markProviderExecutionSubmittedForTest(
					t, repository, submitting, ownerHash, "latest-quality-launch-"+string(terminal),
				)
				running, err := repository.SaveSubmission(
					context.Background(),
					providerExecutionSaveSubmissionInput(
						t, providerExecutionCAS(submitting, ownerHash, time.Time{}),
						"latest-quality-launch-"+string(terminal), "terminal-execution", time.Minute, "",
					),
				)
				if err != nil {
					t.Fatal(err)
				}
				record, err = repository.AdvanceTerminal(context.Background(), domainappdev.AdvanceProviderExecutionTerminalInput{
					OwnerCAS:      providerExecutionCAS(running, ownerHash, time.Time{}),
					OperationHash: testOperationHash(t, "latest-terminal-operation"), ObservedState: terminal,
				})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				stopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: cas})
				if err != nil {
					t.Fatal(err)
				}
				record, err = repository.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
					OwnerCAS:    providerExecutionCAS(stopping, ownerHash, time.Time{}),
					OperationID: "claim-cleanup-complete",
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			clock.set(clock.current().Add(2 * time.Minute))
			var before providerExecutionRecord
			if err := db.Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", record.Generation).Take(&before).Error; err != nil {
				t.Fatal(err)
			}
			if result, err := repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
				SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
				ExpectedVersion: record.Version, OwnerHash: testOwnerHash("nonrecoverable-claim"), LeaseDuration: time.Minute,
			}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
				t.Fatalf("claim nonrecoverable = %#v, %v", result, err)
			}
			var after providerExecutionRecord
			if err := db.Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", record.Generation).Take(&after).Error; err != nil {
				t.Fatal(err)
			}
			beforeOwner, beforeOwnerValid := providerExecutionOwnerHashFromBytes(before.OwnerIdentityHash)
			afterOwner, afterOwnerValid := providerExecutionOwnerHashFromBytes(after.OwnerIdentityHash)
			if after.Version != before.Version || after.OwnerEpoch != before.OwnerEpoch || !sameNullableTime(after.OwnerExpiresAt, before.OwnerExpiresAt) ||
				!beforeOwnerValid || !afterOwnerValid || !beforeOwner.Equal(afterOwner) {
				t.Fatalf("nonrecoverable claim mutated row: before=%#v after=%#v", before, after)
			}
		})
	}
}

func TestProviderExecutionLatestMySQLProviderLeaseSQLShape(t *testing.T) {
	updates := providerExecutionSubmissionUpdates(providerExecutionMySQLDBClock{}, domainappdev.SaveProviderSubmissionInput{
		ProviderExecutionID: "provider-sql-shape", ObservedState: domainappdev.ProviderExecutionObservedRunning,
		ProviderLeaseDuration: 90 * time.Second,
	})
	lease, ok := updates["provider_lease_expires_at"].(clause.Expr)
	if !ok || !strings.Contains(lease.SQL, "TIMESTAMPADD(MICROSECOND") || !strings.Contains(lease.SQL, "UTC_TIMESTAMP(6)") {
		t.Fatalf("provider lease SQL = %#v", updates["provider_lease_expires_at"])
	}
	updated, ok := updates["updated_at"].(clause.Expr)
	if !ok || updated.SQL != "UTC_TIMESTAMP(6)" {
		t.Fatalf("updated_at SQL = %#v", updates["updated_at"])
	}
}

func claimProviderExecutionForLatestTest(t *testing.T, repository *ProviderExecutionRepository, suffix string) (*domainappdev.ProviderExecution, domainappdev.ProviderExecutionOwnerHash) {
	t.Helper()
	record, err := repository.EnsureStart(context.Background(), providerExecutionStartInput("latest-"+suffix, "latest-"+suffix, time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	ownerHash := testOwnerHash("latest-owner-" + suffix)
	record, err = repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: record.SpaceID, ProjectID: record.ProjectID, Generation: record.Generation,
		ExpectedVersion: record.Version, OwnerHash: ownerHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	return record, ownerHash
}

func sameNullableTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
