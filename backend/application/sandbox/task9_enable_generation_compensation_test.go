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

package sandbox

import (
	"context"
	"errors"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type task9FailFirstEnableTransaction struct {
	delegate *testUnitOfWork
	calls    int
}

func (u *task9FailFirstEnableTransaction) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	u.calls++
	if u.calls == 1 {
		return errors.New("enable A database transaction failed")
	}
	return u.delegate.WithinTransaction(ctx, callback)
}

func (u *task9FailFirstEnableTransaction) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	return u.WithinTransaction(ctx, callback)
}

func TestTask9EnableCompensationDoesNotDrainNewerCommittedActivation(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(910, 2)
	expectedVersion := provider.Version
	h.providers.providers[provider.ID] = provider
	h.service.unitOfWork = &task9FailFirstEnableTransaction{delegate: h.uow}

	var (
		activationA DrainFence
	)
	aAtCompensation := make(chan DrainFence, 1)
	resumeA := make(chan struct{})
	h.leases.beforeCompensate = func(leases *testLeaseActivityChecker, current DrainFence) {
		aAtCompensation <- current
		<-resumeA
	}

	errACh := make(chan error, 1)
	go func() {
		_, errA := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: expectedVersion, Status: domainsandbox.ProviderStatusEnabled,
		})
		errACh <- errA
	}()
	activationA = <-aAtCompensation
	if h.leases.activeGeneration != activationA || h.leases.draining {
		t.Fatalf("A did not pause after observing disabled DB state: %#v", h.leases)
	}
	resultB, errB := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: expectedVersion, Status: domainsandbox.ProviderStatusEnabled,
	})
	close(resumeA)
	errA := <-errACh

	if !errors.Is(errA, domainsandbox.ErrUnavailable) || errB != nil || resultB == nil ||
		resultB.Status != domainsandbox.ProviderStatusEnabled || resultB.Version != expectedVersion+1 {
		t.Fatalf("interleaved enable result A=%v B=%#v/%v", errA, resultB, errB)
	}
	stored := h.providers.providers[provider.ID]
	if stored.Status != domainsandbox.ProviderStatusEnabled || h.leases.draining ||
		h.leases.activeGeneration == "" || h.leases.activeGeneration == activationA ||
		h.leases.compensateCalls != 1 {
		t.Fatalf("A compensation overwrote B: provider=%#v leases=%#v A=%q", stored, h.leases, activationA)
	}
}

func TestTask9EnableCompensationDoesNotReplaceNewerDrainGeneration(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(911, 2)
	h.providers.providers[provider.ID] = provider
	h.service.unitOfWork = &task9FailFirstEnableTransaction{delegate: h.uow}

	var handleB DrainHandle
	h.leases.beforeCompensate = func(leases *testLeaseActivityChecker, _ DrainFence) {
		var err error
		handleB, err = leases.BeginDrain(context.Background(), provider.ProviderKey)
		if err != nil {
			t.Fatalf("BeginDrain(B): %v", err)
		}
	}
	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || !h.leases.draining ||
		h.leases.fence != handleB.Current || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled {
		t.Fatalf("A compensation replaced B drain: err=%v handleB=%#v leases=%#v provider=%#v",
			err, handleB, h.leases, h.providers.providers[provider.ID])
	}
}
