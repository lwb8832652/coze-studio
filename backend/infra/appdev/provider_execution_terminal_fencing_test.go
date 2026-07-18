// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"context"
	"errors"
	"testing"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestProviderExecutionTerminalOperationIsIdempotent(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	setProviderExecutionDBTime(t, repository, time.Date(2026, 7, 17, 5, 0, 0, 0, time.UTC))
	running, ownerHash := runningProviderExecutionForTerminalTest(t, repository, "terminal-operation")
	operationHash := testOperationHash(t, "terminal-operation-1")
	input := domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS: providerExecutionCAS(running, ownerHash, time.Time{}), OperationHash: operationHash,
		ObservedState: domainappdev.ProviderExecutionObservedFailed,
		SafeErrorCode: "provider_failed", SafeErrorMessage: "provider execution failed",
	}
	terminal, err := repository.AdvanceTerminal(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := repository.AdvanceTerminal(context.Background(), input)
	if err != nil || retried.Version != terminal.Version || retried.ObservedState != terminal.ObservedState {
		t.Fatalf("terminal response-loss retry = %#v, %v", retried, err)
	}
	wrongOperation := input
	wrongOperation.OperationHash = testOperationHash(t, "terminal-operation-2")
	if result, err := repository.AdvanceTerminal(context.Background(), wrongOperation); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("terminal different operation = %#v, %v", result, err)
	}
	wrongPostcondition := input
	wrongPostcondition.ObservedState = domainappdev.ProviderExecutionObservedSucceeded
	if result, err := repository.AdvanceTerminal(context.Background(), wrongPostcondition); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("terminal mismatched postcondition = %#v, %v", result, err)
	}
}

func TestProviderExecutionTerminalCanRecoverAndCleanupAfterRestart(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 17, 6, 0, 0, 0, time.UTC)}
	repository.clock = clock
	running, oldOwnerHash := runningProviderExecutionForTerminalTest(t, repository, "terminal-recovery")
	terminal, err := repository.AdvanceTerminal(context.Background(), domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS:      providerExecutionCAS(running, oldOwnerHash, time.Time{}),
		OperationHash: testOperationHash(t, "terminal-recovery-operation"), ObservedState: domainappdev.ProviderExecutionObservedTimedOut,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.set(clock.current().Add(2 * time.Minute))
	restarted := NewProviderExecutionRepository(db)
	restarted.clock = clock
	newOwnerHash := testOwnerHash("terminal-recovery-new-owner")
	recovered, err := restarted.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: terminal.SpaceID, ProjectID: terminal.ProjectID, Generation: terminal.Generation,
		ExpectedVersion: terminal.Version, OwnerHash: newOwnerHash, LeaseDuration: time.Minute,
	})
	if err != nil || recovered.OwnerEpoch != terminal.OwnerEpoch+1 {
		t.Fatalf("terminal restart claim = %#v, %v", recovered, err)
	}
	if _, err := restarted.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(terminal, oldOwnerHash, time.Time{}),
	}); !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("old owner terminal write = %v", err)
	}
	stopping, err := restarted.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(recovered, newOwnerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanupPending, err := restarted.BeginCleanup(context.Background(), domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(stopping, newOwnerHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := restarted.CompleteCleanup(context.Background(), domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS:    providerExecutionCAS(cleanupPending, newOwnerHash, time.Time{}),
		OperationID: "terminal-recovery-cleanup",
	})
	if err != nil || completed.ObservedState != domainappdev.ProviderExecutionObservedCleanupComplete {
		t.Fatalf("terminal cleanup completion = %#v, %v", completed, err)
	}
	if _, err := restarted.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: completed.SpaceID, ProjectID: completed.ProjectID, Generation: completed.Generation,
		ExpectedVersion: completed.Version, OwnerHash: testOwnerHash("cleanup-complete-owner"), LeaseDuration: time.Minute,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("cleanup_complete claim = %v", err)
	}
}

func TestProviderExecutionCheckpointReservationUsesDBTimeFences(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 17, 7, 0, 0, 0, time.UTC)}
	repository.clock = clock
	record, ownerHash := claimProviderExecutionForLatestTest(t, repository, "reservation-fence")
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	operationHash := testOperationHash(t, "reservation-fence-operation")
	first, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: 10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstSaveCAS := cas
	firstSaveCAS.ExpectedVersion = first.ReservedVersion
	clock.set(clock.current().Add(11 * time.Second))
	if result, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: firstSaveCAS, OperationHash: operationHash, CheckpointWriteRevision: first.Revision,
		CheckpointEnvelope: "ecp1:expired-reservation",
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("expired reservation save = %#v, %v", result, err)
	}
	renewed, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: cas, OperationHash: operationHash, ReservationDuration: 10 * time.Second,
	})
	if err != nil || renewed.Revision != first.Revision+1 || renewed.ReservedVersion != first.ReservedVersion+1 {
		t.Fatalf("same-operation reservation renewal = %#v, %v", renewed, err)
	}
	if result, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: firstSaveCAS, OperationHash: operationHash, CheckpointWriteRevision: first.Revision,
		CheckpointEnvelope: "ecp1:stale-reservation-after-renewal",
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("stale reservation after renewal = %#v, %v", result, err)
	}
	renewedCAS := cas
	renewedCAS.ExpectedVersion = renewed.ReservedVersion
	saved, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: renewedCAS, OperationHash: operationHash, CheckpointWriteRevision: renewed.Revision,
		CheckpointEnvelope: "ecp1:renewed-reservation",
	})
	if err != nil || saved.CheckpointEnvelope != "ecp1:renewed-reservation" {
		t.Fatalf("renewed reservation save = %#v, %v", saved, err)
	}

	second, secondOwnerHash := claimProviderExecutionForLatestTest(t, repository, "owner-fence")
	secondCAS := providerExecutionCAS(second, secondOwnerHash, time.Time{})
	secondOperation := testOperationHash(t, "owner-fence-operation")
	secondReservation, err := repository.ReserveCheckpointWrite(context.Background(), domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: secondCAS, OperationHash: secondOperation, ReservationDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.set(clock.current().Add(61 * time.Second))
	secondCAS.ExpectedVersion = secondReservation.ReservedVersion
	if result, err := repository.SaveCheckpoint(context.Background(), domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: secondCAS, OperationHash: secondOperation, CheckpointWriteRevision: secondReservation.Revision,
		CheckpointEnvelope: "ecp1:expired-owner",
	}); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("expired owner checkpoint save = %#v, %v", result, err)
	}
}

func TestProviderExecutionReleaseOwnerUsesBoundOperationCapability(t *testing.T) {
	repository, db := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	setProviderExecutionDBTime(t, repository, time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC))
	record, ownerHash := claimProviderExecutionForLatestTest(t, repository, "release-capability")
	cas := providerExecutionCAS(record, ownerHash, time.Time{})
	operationHash, err := domainappdev.HashProviderExecutionReleaseOperationID("release-operation", ownerHash)
	if err != nil {
		t.Fatal(err)
	}
	input := domainappdev.ReleaseProviderExecutionOwnerInput{OwnerCAS: cas, OperationHash: operationHash}
	released, err := repository.ReleaseOwner(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := repository.ReleaseOwner(context.Background(), input)
	if err != nil || retried.Version != released.Version {
		t.Fatalf("release response-loss retry = %#v, %v", retried, err)
	}
	wrongOperation, _ := domainappdev.HashProviderExecutionReleaseOperationID("release-operation-2", ownerHash)
	wrong := input
	wrong.OperationHash = wrongOperation
	if result, err := repository.ReleaseOwner(context.Background(), wrong); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("release different operation = %#v, %v", result, err)
	}
	forgedOwner := testOwnerHash("release-forged-owner")
	forgedOperation, _ := domainappdev.HashProviderExecutionReleaseOperationID("release-operation", forgedOwner)
	forged := input
	forged.OwnerCAS.OwnerHash = forgedOwner
	forged.OperationHash = forgedOperation
	if result, err := repository.ReleaseOwner(context.Background(), forged); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("release forged owner = %#v, %v", result, err)
	}

	newOwnerHash := testOwnerHash("release-new-owner")
	claimed, err := repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: released.SpaceID, ProjectID: released.ProjectID, Generation: released.Generation,
		ExpectedVersion: released.Version, OwnerHash: newOwnerHash, LeaseDuration: time.Minute,
	})
	if err != nil || claimed.OwnerEpoch != released.OwnerEpoch+1 {
		t.Fatalf("claim after release = %#v, %v", claimed, err)
	}
	var stored providerExecutionRecord
	if err := db.Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", claimed.Generation).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored.ReleaseOwnerOperationHash) != 0 {
		t.Fatal("new claim retained old release capability")
	}
	if result, err := repository.ReleaseOwner(context.Background(), input); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("old release capability after new epoch = %#v, %v", result, err)
	}
}

func runningProviderExecutionForTerminalTest(t *testing.T, repository *ProviderExecutionRepository, suffix string) (*domainappdev.ProviderExecution, domainappdev.ProviderExecutionOwnerHash) {
	t.Helper()
	record, ownerHash := claimProviderExecutionForLatestTest(t, repository, suffix)
	submittingCAS := providerExecutionCAS(record, ownerHash, time.Time{})
	submitting, err := repository.StartSubmission(
		context.Background(),
		providerExecutionStartSubmissionInput(t, submittingCAS, "terminal-fixture-launch-"+suffix),
	)
	if err != nil {
		t.Fatal(err)
	}
	submitting = markProviderExecutionSubmittedForTest(
		t, repository, submitting, ownerHash, "terminal-fixture-launch-"+suffix,
	)
	running, err := repository.SaveSubmission(context.Background(), providerExecutionSaveSubmissionInput(
		t, providerExecutionCAS(submitting, ownerHash, time.Time{}),
		"terminal-fixture-launch-"+suffix, "provider-"+suffix, time.Minute, "",
	))
	if err != nil {
		t.Fatal(err)
	}
	running = completeProviderExecutionLaunchForTest(t, repository, running, ownerHash, "terminal-fixture-launch-"+suffix)
	return running, ownerHash
}

func TestProviderExecutionCompleteCleanupBindsOwnerEpochAndExecution(t *testing.T) {
	repository, _ := newProviderExecutionSQLiteRepository(t, 1001, "project-a")
	clock := &providerExecutionControlledDBClock{now: time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)}
	repository.clock = clock
	first, firstOwner := claimProviderExecutionForLatestTest(t, repository, "cleanup-capability-first")
	firstStopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(first, firstOwner, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	firstInput := domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(firstStopping, firstOwner, time.Time{}), OperationID: "predictable-cleanup-operation",
	}
	firstComplete, err := repository.CompleteCleanup(context.Background(), firstInput)
	if err != nil {
		t.Fatal(err)
	}
	firstRetry, err := repository.CompleteCleanup(context.Background(), firstInput)
	if err != nil || firstRetry.Version != firstComplete.Version {
		t.Fatalf("same-owner cleanup retry = %#v, %v", firstRetry, err)
	}
	forgedOwner := firstInput
	forgedOwner.OwnerCAS.OwnerHash = testOwnerHash("forged-cleanup-owner")
	if result, err := repository.CompleteCleanup(context.Background(), forgedOwner); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("forged owner cleanup retry = %#v, %v", result, err)
	}

	second, secondOwner := claimProviderExecutionForLatestTest(t, repository, "cleanup-capability-second")
	secondStopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(second, secondOwner, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	crossExecution := firstInput
	crossExecution.OwnerCAS = providerExecutionCAS(secondStopping, firstOwner, time.Time{})
	if result, err := repository.CompleteCleanup(context.Background(), crossExecution); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("cross-execution cleanup replay = %#v, %v", result, err)
	}

	third, oldOwner := claimProviderExecutionForLatestTest(t, repository, "cleanup-capability-replacement")
	thirdStopping, err := repository.SetDesiredStop(context.Background(), domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(third, oldOwner, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	clock.set(clock.current().Add(2 * time.Minute))
	newOwner := testOwnerHash("cleanup-capability-new-owner")
	replaced, err := repository.Claim(context.Background(), domainappdev.ClaimProviderExecutionInput{
		SpaceID: thirdStopping.SpaceID, ProjectID: thirdStopping.ProjectID, Generation: thirdStopping.Generation,
		ExpectedVersion: thirdStopping.Version, OwnerHash: newOwner, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	replacementInput := domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(replaced, newOwner, time.Time{}), OperationID: "predictable-cleanup-operation",
	}
	if _, err := repository.CompleteCleanup(context.Background(), replacementInput); err != nil {
		t.Fatal(err)
	}
	staleOwnerInput := replacementInput
	staleOwnerInput.OwnerCAS = providerExecutionCAS(thirdStopping, oldOwner, time.Time{})
	if staleOwnerInput.OwnerCAS.OwnerEpoch >= replacementInput.OwnerCAS.OwnerEpoch {
		t.Fatalf("stale owner epoch = %d, replacement epoch = %d", staleOwnerInput.OwnerCAS.OwnerEpoch, replacementInput.OwnerCAS.OwnerEpoch)
	}
	if result, err := repository.CompleteCleanup(context.Background(), staleOwnerInput); result != nil || !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("replaced owner cleanup retry = %#v, %v", result, err)
	}
}
