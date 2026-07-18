// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type providerExecutionReservationRepository struct {
	domainappdev.ProviderExecutionRepository
	reserveErr   error
	reservation  *domainappdev.ProviderExecutionCheckpointReservation
	reserveCalls int
	abortCalls   int
	abortErr     error
	saveCalls    int
	saveErr      error
	saved        domainappdev.SaveProviderCheckpointInput
	listCalls    int
}

func (r *providerExecutionReservationRepository) ReserveCheckpointWrite(_ context.Context, _ domainappdev.ReserveProviderCheckpointWriteInput) (*domainappdev.ProviderExecutionCheckpointReservation, error) {
	r.reserveCalls++
	return r.reservation, r.reserveErr
}

func (r *providerExecutionReservationRepository) SaveCheckpoint(_ context.Context, input domainappdev.SaveProviderCheckpointInput) (*domainappdev.ProviderExecution, error) {
	r.saveCalls++
	r.saved = input
	if r.saveErr != nil {
		return nil, r.saveErr
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	return &domainappdev.ProviderExecution{
		ID: "apx_test", SpaceID: input.OwnerCAS.SpaceID, ProjectID: input.OwnerCAS.ProjectID,
		Generation: input.OwnerCAS.Generation, DesiredState: domainappdev.ProviderExecutionDesiredRun,
		ObservedState: domainappdev.ProviderExecutionObservedRunning, ProviderKey: input.OwnerCAS.ProviderKey,
		ProviderScope: input.OwnerCAS.ProviderScope, Version: input.OwnerCAS.ExpectedVersion + 1,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (r *providerExecutionReservationRepository) AbortCheckpointWrite(_ context.Context, _ domainappdev.AbortProviderCheckpointWriteInput) (*domainappdev.ProviderExecution, error) {
	r.abortCalls++
	return nil, r.abortErr
}

func (r *providerExecutionReservationRepository) ListRecoverable(context.Context, domainappdev.ListRecoverableProviderExecutionsInput) ([]*domainappdev.RecoverableProviderExecution, error) {
	r.listCalls++
	return nil, nil
}

type providerExecutionCountingCodec struct {
	sealCalls int
	sealErr   error
}

func (c *providerExecutionCountingCodec) Seal(context.Context, applicationsandbox.ExecutionCheckpointBinding, applicationsandbox.ExecutionCheckpoint) (string, error) {
	c.sealCalls++
	if c.sealErr != nil {
		return "", c.sealErr
	}
	return "ecp1:reserved-envelope", nil
}

func (*providerExecutionCountingCodec) Open(context.Context, applicationsandbox.ExecutionCheckpointBinding, string) (applicationsandbox.ExecutionCheckpoint, error) {
	return applicationsandbox.ExecutionCheckpoint{}, nil
}

func TestProviderExecutionServiceReservesBeforeSeal(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	owner := providerExecutionTestOwner(t)
	for _, reserveErr := range []error{
		domainappdev.ErrProviderExecutionOwnerConflict,
		domainappdev.ErrProviderExecutionVersionConflict,
		domainappdev.ErrProviderExecutionGenerationConflict,
	} {
		repository := &providerExecutionReservationRepository{reserveErr: reserveErr}
		codec := &providerExecutionCountingCodec{}
		service, err := NewProviderExecutionService(repository, codec, WithProviderExecutionClock(func() time.Time { return now }))
		if err != nil {
			t.Fatal(err)
		}
		_, err = service.SaveCheckpoint(context.Background(), providerExecutionCheckpointRequest(owner, 4))
		if !errors.Is(err, reserveErr) || codec.sealCalls != 0 || repository.saveCalls != 0 {
			t.Fatalf("reservation error/seal/save = %v/%d/%d", err, codec.sealCalls, repository.saveCalls)
		}
	}

	repository := &providerExecutionReservationRepository{reservation: &domainappdev.ProviderExecutionCheckpointReservation{Revision: 7, ReservedVersion: 5}}
	codec := &providerExecutionCountingCodec{}
	service, err := NewProviderExecutionService(repository, codec, WithProviderExecutionClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := service.SaveCheckpoint(context.Background(), providerExecutionCheckpointRequest(owner, 4))
	if err != nil || metadata.Version != 6 || codec.sealCalls != 1 || repository.saveCalls != 1 {
		t.Fatalf("reserved save = %#v, %v seals=%d saves=%d", metadata, err, codec.sealCalls, repository.saveCalls)
	}
	if repository.saved.OwnerCAS.ExpectedVersion != 5 || repository.saved.CheckpointWriteRevision != 7 ||
		repository.saved.OperationHash.IsZero() || repository.saved.CheckpointEnvelope != "ecp1:reserved-envelope" {
		t.Fatalf("saved reservation = %#v", repository.saved)
	}
}

func TestProviderExecutionServiceSealFailureAbortsReservation(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	repository := &providerExecutionReservationRepository{reservation: &domainappdev.ProviderExecutionCheckpointReservation{Revision: 9, ReservedVersion: 5}}
	codec := &providerExecutionCountingCodec{sealErr: errors.New("seal failed")}
	service, err := NewProviderExecutionService(repository, codec, WithProviderExecutionClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveCheckpoint(context.Background(), providerExecutionCheckpointRequest(providerExecutionTestOwner(t), 4))
	if !errors.Is(err, ErrProviderExecutionServiceInvalid) || repository.reserveCalls != 1 || codec.sealCalls != 1 || repository.abortCalls != 1 || repository.saveCalls != 0 {
		t.Fatalf("seal failure = %v reserve/seal/abort/save=%d/%d/%d/%d", err, repository.reserveCalls, codec.sealCalls, repository.abortCalls, repository.saveCalls)
	}
}

func TestProviderExecutionServiceSaveFailureDoesNotAbortReservation(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	repository := &providerExecutionReservationRepository{
		reservation: &domainappdev.ProviderExecutionCheckpointReservation{Revision: 4, ReservedVersion: 5},
		saveErr:     domainappdev.ErrProviderExecutionUnavailable,
	}
	codec := &providerExecutionCountingCodec{}
	service, err := NewProviderExecutionService(repository, codec, WithProviderExecutionClock(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveCheckpoint(context.Background(), providerExecutionCheckpointRequest(providerExecutionTestOwner(t), 4))
	if !errors.Is(err, domainappdev.ErrProviderExecutionUnavailable) || repository.saveCalls != 1 || repository.abortCalls != 0 {
		t.Fatalf("save failure = %v save/abort=%d/%d", err, repository.saveCalls, repository.abortCalls)
	}
}

func TestProviderExecutionServiceRequiresRecoveryProjectScope(t *testing.T) {
	repository := &providerExecutionReservationRepository{}
	service, err := NewProviderExecutionService(repository, &providerExecutionCountingCodec{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ListRecoverable(context.Background(), ListRecoverableProviderExecutionsRequest{SpaceID: "1001", Limit: 10})
	if !errors.Is(err, ErrProviderExecutionServiceInvalid) || repository.listCalls != 0 {
		t.Fatalf("empty project = %v, repository calls=%d", err, repository.listCalls)
	}
}

func providerExecutionTestOwner(t *testing.T) ProviderExecutionOwner {
	t.Helper()
	token, err := domainappdev.NewProviderExecutionOwnerToken(bytes.NewReader(bytes.Repeat([]byte{0x41}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return ProviderExecutionOwner{
		spaceID: "1001", projectID: "project-a", generation: 1,
		providerKey: "provider-a", providerScope: domainsandbox.ScopeAppDev,
		token: token, epoch: 1,
	}
}

func providerExecutionCheckpointRequest(owner ProviderExecutionOwner, version uint64) SaveProviderExecutionCheckpointRequest {
	return SaveProviderExecutionCheckpointRequest{
		Owner: owner, ExpectedVersion: version, OperationID: "checkpoint-operation-1",
		ProviderKey: owner.providerKey, ProviderScope: owner.providerScope,
	}
}
