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
	"reflect"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestTask9FinalEnableActivateFailureDoesNotEnterTransaction(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(901, 2)
	h.providers.providers[provider.ID] = provider
	h.leases.activateErr = errors.New("redis unavailable")

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if h.uow.calls != 0 || len(h.audits.events) != 0 || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled {
		t.Fatalf("activate failure mutated DB: tx=%d audits=%d provider=%#v", h.uow.calls, len(h.audits.events), h.providers.providers[provider.ID])
	}
}

func TestTask9FinalEnableAuditFailureActivatesThenRestoresDrain(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(902, 2)
	h.providers.providers[provider.ID] = provider
	h.audits.appendErr = errors.New("audit unavailable")

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || h.leases.activateCalls != 1 || h.leases.beginCalls != 1 ||
		h.leases.compensateCalls != 1 ||
		!h.leases.draining || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled || len(h.audits.events) != 0 {
		t.Fatalf("audit failure saga = err=%v leases=%#v provider=%#v audits=%d", err, h.leases, h.providers.providers[provider.ID], len(h.audits.events))
	}
}

func TestTask9FinalEnableVersionConflictAfterActivationRestoresDrain(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(905, 2)
	h.providers.providers[provider.ID] = provider
	h.leases.beforeActivate = func(*testLeaseActivityChecker) {
		h.providers.providers[provider.ID].Version++
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrVersionConflict) || h.leases.activateCalls != 1 || h.leases.beginCalls != 1 ||
		h.leases.compensateCalls != 1 ||
		!h.leases.draining || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled || len(h.audits.events) != 0 {
		t.Fatalf("version conflict saga = err=%v leases=%#v provider=%#v audits=%d", err, h.leases, h.providers.providers[provider.ID], len(h.audits.events))
	}
}

func TestTask9FinalConcurrentEnableConflictDoesNotDrainCommittedProvider(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(908, 2)
	expectedVersion := provider.Version
	h.providers.providers[provider.ID] = provider
	h.leases.beforeActivate = func(*testLeaseActivityChecker) {
		stored := h.providers.providers[provider.ID]
		stored.Status = domainsandbox.ProviderStatusEnabled
		stored.Version++
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: expectedVersion, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrVersionConflict) || h.leases.beginCalls != 1 || h.leases.activateCalls != 1 ||
		h.leases.draining || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusEnabled {
		t.Fatalf("concurrent enable reconciliation = err=%v leases=%#v provider=%#v", err, h.leases, h.providers.providers[provider.ID])
	}
}

type task9FinalCommittedWithErrorUOW struct {
	*testUnitOfWork
}

func (u *task9FinalCommittedWithErrorUOW) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	if err := u.testUnitOfWork.WithinTransaction(ctx, callback); err != nil {
		return err
	}
	return errors.New("commit acknowledgement lost")
}

func (u *task9FinalCommittedWithErrorUOW) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	return u.WithinTransaction(ctx, callback)
}

func TestTask9FinalAmbiguousCommitKeepsActivatedLeaseWhenDatabaseIsEnabled(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(909, 2)
	h.providers.providers[provider.ID] = provider
	h.service.unitOfWork = &task9FinalCommittedWithErrorUOW{testUnitOfWork: h.uow}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || h.leases.beginCalls != 1 || h.leases.activateCalls != 1 ||
		h.leases.draining || h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusEnabled || len(h.audits.events) != 1 {
		t.Fatalf("ambiguous commit reconciliation = err=%v leases=%#v provider=%#v audits=%d",
			err, h.leases, h.providers.providers[provider.ID], len(h.audits.events))
	}
}

type task9FinalCommitFailUOW struct {
	base      *testUnitOfWork
	commitErr error
	cancel    context.CancelFunc
}

func (u *task9FinalCommitFailUOW) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	return u.WithinTransaction(ctx, callback)
}

func (u *task9FinalCommitFailUOW) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	providersSnapshot := cloneTestProviderMap(u.base.providers.providers)
	defaultsSnapshot := cloneTestDefaultMap(u.base.defaults.defaults)
	auditsSnapshot := cloneTestAudits(u.base.audits.events)
	err := callback(ctx, domainsandbox.TransactionRepositories{
		Providers: u.base.providers,
		Defaults:  u.base.defaults,
		Audits:    u.base.audits,
	})
	if u.cancel != nil {
		u.cancel()
	}
	if err == nil {
		err = u.commitErr
	}
	if err != nil {
		u.base.providers.providers = providersSnapshot
		u.base.defaults.defaults = defaultsSnapshot
		u.base.audits.events = auditsSnapshot
	}
	return err
}

type task9FinalContextLease struct {
	*testLeaseActivityChecker
	compensationContextErrors []error
}

func (l *task9FinalContextLease) CompensateActivation(
	ctx context.Context,
	providerKey string,
	current DrainFence,
) (ActivationCompensationResult, error) {
	l.compensationContextErrors = append(l.compensationContextErrors, ctx.Err())
	return l.testLeaseActivityChecker.CompensateActivation(ctx, providerKey, current)
}

func TestTask9FinalEnableCommitFailureUsesUncancelledBoundedCompensation(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(903, 2)
	h.providers.providers[provider.ID] = provider
	ctx, cancel := context.WithCancel(context.Background())
	lease := &task9FinalContextLease{testLeaseActivityChecker: h.leases}
	h.service.leases = lease
	h.service.unitOfWork = &task9FinalCommitFailUOW{
		base: h.uow, commitErr: errors.New("commit failed"), cancel: cancel,
	}

	_, err := h.service.SetStatus(ctx, testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || len(lease.compensationContextErrors) != 1 ||
		lease.compensationContextErrors[0] != nil ||
		h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled || len(h.audits.events) != 0 || !lease.draining {
		t.Fatalf("commit failure cleanup = err=%v compensationCtx=%v provider=%#v audits=%d lease=%#v",
			err, lease.compensationContextErrors, h.providers.providers[provider.ID], len(h.audits.events), lease.testLeaseActivityChecker)
	}
}

type task9FinalCompensationFailureLease struct {
	*testLeaseActivityChecker
}

func (l *task9FinalCompensationFailureLease) CompensateActivation(
	context.Context,
	string,
	DrainFence,
) (ActivationCompensationResult, error) {
	return ActivationCompensationStale, errors.New("redis recovery unavailable")
}

func TestTask9FinalEnableCompensationFailureLeavesDatabaseDisabled(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(906, 2)
	h.providers.providers[provider.ID] = provider
	h.audits.appendErr = errors.New("audit unavailable")
	lease := &task9FinalCompensationFailureLease{testLeaseActivityChecker: h.leases}
	h.service.leases = lease

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if !errors.Is(err, domainsandbox.ErrUnavailable) || h.leases.beginCalls != 1 ||
		h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled || len(h.audits.events) != 0 {
		t.Fatalf("compensation failure = err=%v begin=%d provider=%#v audits=%d",
			err, h.leases.beginCalls, h.providers.providers[provider.ID], len(h.audits.events))
	}
}

type task9FinalOrderLease struct {
	*testLeaseActivityChecker
	events *[]string
}

func (l *task9FinalOrderLease) BeginDrain(ctx context.Context, providerKey string) (DrainHandle, error) {
	*l.events = append(*l.events, "begin")
	return l.testLeaseActivityChecker.BeginDrain(ctx, providerKey)
}

func (l *task9FinalOrderLease) Activate(ctx context.Context, providerKey string, current DrainFence) error {
	*l.events = append(*l.events, "activate")
	return l.testLeaseActivityChecker.Activate(ctx, providerKey, current)
}

type task9FinalOrderUOW struct {
	*testUnitOfWork
	events *[]string
}

func (u *task9FinalOrderUOW) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	*u.events = append(*u.events, "transaction")
	err := u.testUnitOfWork.WithinTransaction(ctx, callback)
	if err == nil {
		*u.events = append(*u.events, "commit")
	}
	return err
}

func (u *task9FinalOrderUOW) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	return u.WithinTransaction(ctx, callback)
}

func TestTask9FinalEnableSuccessActivatesBeforeTransactionAndAuditsVersion(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(904, 2)
	h.providers.providers[provider.ID] = provider
	events := []string{}
	h.service.leases = &task9FinalOrderLease{testLeaseActivityChecker: h.leases, events: &events}
	h.service.unitOfWork = &task9FinalOrderUOW{testUnitOfWork: h.uow, events: &events}

	result, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
	})
	if err != nil || !reflect.DeepEqual(events, []string{"begin", "activate", "transaction", "commit"}) ||
		result.Version != 3 || len(h.audits.events) != 1 {
		t.Fatalf("success saga = result=%#v err=%v events=%v audits=%#v", result, err, events, h.audits.events)
	}
	audit := h.audits.events[0]
	if audit.Action != auditActionEnable ||
		audit.Metadata[domainsandbox.AuditMetadataKeyPreviousStatus] != "disabled" ||
		audit.Metadata[domainsandbox.AuditMetadataKeyNewStatus] != "enabled" ||
		audit.Metadata[domainsandbox.AuditMetadataKeyVersion] != "3" {
		t.Fatalf("enable audit = %#v", audit)
	}
}

func TestTask9FinalRepeatedEnableIsIdempotent(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(907, 4)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider

	for range 2 {
		result, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
			ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusEnabled,
		})
		if err != nil || result.Version != provider.Version || result.Status != domainsandbox.ProviderStatusEnabled {
			t.Fatalf("repeated enable = %#v, %v", result, err)
		}
	}
	if len(h.audits.events) != 0 || h.leases.activateCalls != 2 || h.leases.draining {
		t.Fatalf("repeated enable side effects: audits=%d leases=%#v", len(h.audits.events), h.leases)
	}
}
