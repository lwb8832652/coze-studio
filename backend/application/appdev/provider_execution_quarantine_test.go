// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type providerQuarantineRepositoryFake struct {
	input  domainappdev.DisposeProviderExecutionQuarantineInput
	result *domainappdev.ProviderExecutionQuarantineDisposition
	err    error
	calls  int
}

func (repository *providerQuarantineRepositoryFake) DisposeQuarantine(
	_ context.Context,
	input domainappdev.DisposeProviderExecutionQuarantineInput,
) (*domainappdev.ProviderExecutionQuarantineDisposition, error) {
	repository.calls++
	repository.input = input
	return repository.result, repository.err
}

func TestProviderQuarantineDispositionServiceValidatesCapabilityAndReturnsSafeProjection(t *testing.T) {
	now := time.Date(2026, 7, 17, 18, 0, 0, 0, time.UTC)
	repository := &providerQuarantineRepositoryFake{
		result: &domainappdev.ProviderExecutionQuarantineDisposition{
			Execution: &domainappdev.ProviderExecution{
				ID: "execution-private", SpaceID: "1001", ProjectID: "project-a", Generation: 7,
				DesiredState:  domainappdev.ProviderExecutionDesiredStop,
				ObservedState: domainappdev.ProviderExecutionObservedPending,
				ProviderKey:   "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
				LaunchState: domainappdev.ProviderExecutionLaunchAborted, Version: 12,
			},
			Audit: &domainappdev.ProviderExecutionQuarantineAudit{
				ActorID:         42,
				Acknowledgement: domainappdev.ProviderExecutionQuarantineProviderAbsent,
				CreatedAt:       now,
			},
		},
	}
	service, err := NewProviderQuarantineDispositionService(
		repository,
		bytes.NewReader(bytes.Repeat([]byte{0x42}, domainappdev.ProviderExecutionOwnerTokenBytes)),
	)
	require.NoError(t, err)

	result, err := service.Dispose(context.Background(), ProviderQuarantineDispositionActor{
		UserID: 42, SystemAdmin: true,
	}, ProviderQuarantineDispositionRequest{
		SpaceID: "1001", ProjectID: "project-a", Generation: 7, ExpectedVersion: 11,
		ExpectedState:   domainappdev.ProviderExecutionLaunchQuarantined,
		OperationID:     "operator-disposition-1",
		Acknowledgement: domainappdev.ProviderExecutionQuarantineProviderAbsent,
		Reason:          domainappdev.ProviderExecutionQuarantineReasonProviderAbsent,
		EvidenceHash:    "125c9e7f0d7d31dbaac8f21843892d98385c5d00ed408bc0108c4af1bb6d1e92",
	})
	require.NoError(t, err)
	require.Equal(t, 1, repository.calls)
	require.False(t, repository.input.OwnerHash.IsZero())
	require.Equal(t, int64(42), repository.input.ActorID)
	require.Equal(t, uint64(11), repository.input.ExpectedVersion)
	require.Equal(t, "operator-disposition-1", repository.input.OperationID)
	require.Equal(t, uint64(7), result.Generation)
	require.Equal(t, uint64(12), result.Version)
	require.Equal(t, domainappdev.ProviderExecutionLaunchAborted, result.State)
	require.True(t, result.CleanupRequired)

	wire, err := json.Marshal(result)
	require.NoError(t, err)
	text := string(wire) + fmt.Sprintf(" %v %+v %#v", result, result, result)
	for _, forbidden := range []string{
		"execution-private", "125c9e7f", "operator-disposition-1", "provider-a",
		"token", "checkpoint", "credential",
	} {
		require.NotContains(t, text, forbidden)
	}
}

func TestProviderQuarantineDispositionServiceFailsClosedBeforeRepository(t *testing.T) {
	repository := &providerQuarantineRepositoryFake{}
	service, err := NewProviderQuarantineDispositionService(
		repository,
		bytes.NewReader(bytes.Repeat([]byte{0x11}, domainappdev.ProviderExecutionOwnerTokenBytes*8)),
	)
	require.NoError(t, err)
	valid := ProviderQuarantineDispositionRequest{
		SpaceID: "1001", ProjectID: "project-a", Generation: 7, ExpectedVersion: 11,
		ExpectedState:   domainappdev.ProviderExecutionLaunchQuarantined,
		OperationID:     "operator-disposition-1",
		Acknowledgement: domainappdev.ProviderExecutionQuarantineProviderAbsent,
		Reason:          domainappdev.ProviderExecutionQuarantineReasonProviderAbsent,
		EvidenceHash:    "125c9e7f0d7d31dbaac8f21843892d98385c5d00ed408bc0108c4af1bb6d1e92",
	}
	tests := []struct {
		name    string
		actor   ProviderQuarantineDispositionActor
		mutate  func(*ProviderQuarantineDispositionRequest)
		context context.Context
	}{
		{name: "not admin", actor: ProviderQuarantineDispositionActor{UserID: 42}, context: context.Background()},
		{name: "missing actor", actor: ProviderQuarantineDispositionActor{SystemAdmin: true}, context: context.Background()},
		{name: "wrong state", actor: ProviderQuarantineDispositionActor{UserID: 42, SystemAdmin: true}, context: context.Background(), mutate: func(request *ProviderQuarantineDispositionRequest) {
			request.ExpectedState = domainappdev.ProviderExecutionLaunchSubmitted
		}},
		{name: "unbounded reason", actor: ProviderQuarantineDispositionActor{UserID: 42, SystemAdmin: true}, context: context.Background(), mutate: func(request *ProviderQuarantineDispositionRequest) {
			request.Reason = "Bearer secret"
		}},
		{name: "invalid evidence", actor: ProviderQuarantineDispositionActor{UserID: 42, SystemAdmin: true}, context: context.Background(), mutate: func(request *ProviderQuarantineDispositionRequest) {
			request.EvidenceHash = "not-a-digest"
		}},
		{name: "canceled", actor: ProviderQuarantineDispositionActor{UserID: 42, SystemAdmin: true}, context: canceledProviderQuarantineContext()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := valid
			if test.mutate != nil {
				test.mutate(&request)
			}
			_, disposeErr := service.Dispose(test.context, test.actor, request)
			require.Error(t, disposeErr)
		})
	}
	require.Zero(t, repository.calls)

	repository.err = errors.New("database raw secret https://provider.invalid/token=x")
	_, err = service.Dispose(context.Background(), ProviderQuarantineDispositionActor{
		UserID: 42, SystemAdmin: true,
	}, valid)
	require.ErrorIs(t, err, domainappdev.ErrProviderExecutionUnavailable)
	require.NotContains(t, err.Error(), "provider.invalid")
}

func canceledProviderQuarantineContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}
