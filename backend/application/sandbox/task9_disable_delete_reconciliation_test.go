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
	"fmt"
	"sync"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type task9MutationOutcomeUOW struct {
	base      *testUnitOfWork
	commit    bool
	resultErr error
	before    func()
	after     func()
	cancel    context.CancelFunc
}

func (u *task9MutationOutcomeUOW) WithinProviderCreateTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	return u.WithinTransaction(ctx, callback)
}

func (u *task9MutationOutcomeUOW) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, domainsandbox.TransactionRepositories) error,
) error {
	u.base.calls++
	u.base.committed = false
	providersSnapshot := cloneTestProviderMap(u.base.providers.providers)
	defaultsSnapshot := cloneTestDefaultMap(u.base.defaults.defaults)
	auditsSnapshot := cloneTestAudits(u.base.audits.events)
	if u.before != nil {
		u.before()
	}
	callbackErr := callback(ctx, domainsandbox.TransactionRepositories{
		Providers: u.base.providers,
		Defaults:  u.base.defaults,
		Audits:    u.base.audits,
	})
	if callbackErr != nil || !u.commit {
		u.base.providers.providers = providersSnapshot
		u.base.defaults.defaults = defaultsSnapshot
		u.base.audits.events = auditsSnapshot
	} else {
		u.base.committed = true
	}
	if u.cancel != nil {
		u.cancel()
	}
	if u.after != nil {
		u.after()
	}
	if callbackErr != nil {
		return callbackErr
	}
	return u.resultErr
}

type task9ReconcileReadFailureRepository struct {
	*testProviderRepository
	mu        sync.Mutex
	readCount int
	failAfter int
}

func (r *task9ReconcileReadFailureRepository) GetProvider(
	ctx context.Context,
	providerID int64,
) (*domainsandbox.Provider, error) {
	r.mu.Lock()
	r.readCount++
	readCount := r.readCount
	r.mu.Unlock()
	if readCount > r.failAfter {
		return nil, errors.New("persistent state unavailable")
	}
	return r.testProviderRepository.GetProvider(ctx, providerID)
}

type task9DrainReconcileLifecycle struct {
	mu                   sync.Mutex
	sequence             int
	state                string
	beginTokens          []DrainFence
	restoreTokens        []DrainFence
	retainTokens         []DrainFence
	restoreContextErrors []error
	retainContextErrors  []error
	restoreApplied       int
}

func (l *task9DrainReconcileLifecycle) BeginDrain(
	ctx context.Context,
	_ string,
) (DrainHandle, error) {
	if err := ctx.Err(); err != nil {
		return DrainHandle{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sequence++
	current := DrainFence(fmt.Sprintf("task9-drain-%d", l.sequence))
	l.state = "g:" + current
	l.beginTokens = append(l.beginTokens, current)
	return DrainHandle{Current: current}, nil
}

func (l *task9DrainReconcileLifecycle) RestoreActive(
	ctx context.Context,
	_ string,
	current DrainFence,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.restoreTokens = append(l.restoreTokens, current)
	l.restoreContextErrors = append(l.restoreContextErrors, ctx.Err())
	if l.state == "g:"+current {
		l.state = "l:" + current
		l.restoreApplied++
	}
	return nil
}

func (l *task9DrainReconcileLifecycle) RetainDrain(
	ctx context.Context,
	_ string,
	current DrainFence,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.retainTokens = append(l.retainTokens, current)
	l.retainContextErrors = append(l.retainContextErrors, ctx.Err())
	return nil
}

func (l *task9DrainReconcileLifecycle) Activate(
	_ context.Context,
	_ string,
	current DrainFence,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state == "g:"+current {
		l.state = "l:" + current
	}
	return nil
}

func (l *task9DrainReconcileLifecycle) CompensateActivation(
	context.Context,
	string,
	DrainFence,
) (ActivationCompensationResult, error) {
	return ActivationCompensationStale, nil
}

func (l *task9DrainReconcileLifecycle) takeoverActive() DrainFence {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sequence++
	current := DrainFence(fmt.Sprintf("task9-drain-%d", l.sequence))
	l.state = "l:" + current
	return current
}

func (l *task9DrainReconcileLifecycle) snapshot() (string, []DrainFence, []DrainFence, []error, []error, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state,
		append([]DrainFence(nil), l.restoreTokens...),
		append([]DrainFence(nil), l.retainTokens...),
		append([]error(nil), l.restoreContextErrors...),
		append([]error(nil), l.retainContextErrors...),
		l.restoreApplied
}

func TestTask9DisableDeleteReconcileDisableAmbiguousCommitRetainsDrain(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1001, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base: h.uow, commit: true, resultErr: errors.New("commit acknowledgement lost"),
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	state, restores, retains, _, _, _ := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) ||
		h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusDisabled ||
		state != "g:"+lease.beginTokens[0] || len(restores) != 0 || len(retains) != 1 {
		t.Fatalf("disable ambiguous commit = err=%v provider=%#v state=%s restores=%v retains=%v",
			err, h.providers.providers[provider.ID], state, restores, retains)
	}
}

func TestTask9DisableDeleteReconcileDeleteAmbiguousCommitRetainsDrainAfterNotFound(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1002, 4)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base:      h.uow,
		commit:    true,
		resultErr: errors.New("commit acknowledgement lost"),
		before: func() {
			h.providers.providers[provider.ID].Status = domainsandbox.ProviderStatusDisabled
		},
	}

	_, err := h.service.Delete(context.Background(), testActor(), DeleteProviderRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version,
	})
	state, restores, retains, _, _, _ := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) || h.providers.providers[provider.ID].DeletedAt == nil ||
		state != "g:"+lease.beginTokens[0] || len(restores) != 0 || len(retains) != 1 {
		t.Fatalf("delete ambiguous commit = err=%v provider=%#v state=%s restores=%v retains=%v",
			err, h.providers.providers[provider.ID], state, restores, retains)
	}
}

func TestTask9DisableDeleteReconcileRollbackRestoresActive(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1003, 3)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base: h.uow, commit: false, resultErr: errors.New("transaction rolled back"),
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	state, restores, retains, _, _, applied := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) ||
		h.providers.providers[provider.ID].Status != domainsandbox.ProviderStatusEnabled ||
		state != "l:"+lease.beginTokens[0] || len(restores) != 1 || len(retains) != 0 || applied != 1 {
		t.Fatalf("disable rollback = err=%v provider=%#v state=%s restores=%v retains=%v applied=%d",
			err, h.providers.providers[provider.ID], state, restores, retains, applied)
	}
}

func TestTask9DisableDeleteReconcileReadFailureRetainsDrainAndFailsClosed(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1004, 5)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	h.service.providers = &task9ReconcileReadFailureRepository{
		testProviderRepository: h.providers, failAfter: 1,
	}
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base: h.uow, commit: false, resultErr: errors.New("transaction result unknown"),
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	state, restores, retains, _, _, _ := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) || state != "g:"+lease.beginTokens[0] ||
		len(restores) != 0 || len(retains) != 1 {
		t.Fatalf("state read failure = err=%v state=%s restores=%v retains=%v", err, state, restores, retains)
	}
}

func TestTask9DisableDeleteReconcileUnknownStatusRetainsDrainAndFailsClosed(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1007, 6)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base:      h.uow,
		commit:    true,
		resultErr: domainsandbox.ErrVersionConflict,
		after: func() {
			h.providers.providers[provider.ID].Status = domainsandbox.ProviderStatus("unknown")
		},
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	state, restores, retains, _, _, _ := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) || errors.Is(err, domainsandbox.ErrVersionConflict) ||
		state != "g:"+lease.beginTokens[0] || len(restores) != 0 || len(retains) != 1 {
		t.Fatalf("unknown status = err=%v state=%s restores=%v retains=%v", err, state, restores, retains)
	}
}

func TestTask9DisableDeleteReconcileNewerEpochMakesRestoreStaleNoOp(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1005, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	var newer DrainFence
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base:      h.uow,
		commit:    false,
		resultErr: errors.New("transaction rolled back"),
		after: func() {
			newer = lease.takeoverActive()
		},
	}

	_, err := h.service.SetStatus(context.Background(), testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	state, restores, _, _, _, applied := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) || state != "l:"+newer ||
		len(restores) != 1 || restores[0] != lease.beginTokens[0] || applied != 0 {
		t.Fatalf("newer epoch = err=%v state=%s newer=%s restores=%v applied=%d", err, state, newer, restores, applied)
	}
}

func TestTask9DisableDeleteReconcileRequestCancellationDoesNotSkipRestore(t *testing.T) {
	h := newControlPlaneHarness(t)
	provider := testProvider(1006, 2)
	provider.Status = domainsandbox.ProviderStatusEnabled
	h.providers.providers[provider.ID] = provider
	ctx, cancel := context.WithCancel(context.Background())
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	h.service.unitOfWork = &task9MutationOutcomeUOW{
		base: h.uow, commit: false, resultErr: errors.New("commit failed"), cancel: cancel,
	}

	_, err := h.service.SetStatus(ctx, testActor(), SetProviderStatusRequest{
		ProviderID: provider.ID, ExpectedVersion: provider.Version, Status: domainsandbox.ProviderStatusDisabled,
	})
	state, restores, _, restoreContexts, _, _ := lease.snapshot()
	if !errors.Is(err, domainsandbox.ErrUnavailable) || ctx.Err() != context.Canceled ||
		state != "l:"+lease.beginTokens[0] || len(restores) != 1 ||
		len(restoreContexts) != 1 || restoreContexts[0] != nil {
		t.Fatalf("cancelled restore = err=%v request=%v state=%s restores=%v contexts=%v",
			err, ctx.Err(), state, restores, restoreContexts)
	}
}

func TestTask9DisableDeleteReconcileRepeatedRestoreIsIdempotent(t *testing.T) {
	h := newControlPlaneHarness(t)
	lease := &task9DrainReconcileLifecycle{}
	h.service.leases = lease
	handle, err := lease.BeginDrain(context.Background(), testProviderKey)
	if err != nil {
		t.Fatalf("BeginDrain() error = %v", err)
	}
	if err := h.service.recoverProviderDrain(context.Background(), testProviderKey, handle, true); err != nil {
		t.Fatalf("first recoverProviderDrain() error = %v", err)
	}
	if err := h.service.recoverProviderDrain(context.Background(), testProviderKey, handle, true); err != nil {
		t.Fatalf("second recoverProviderDrain() error = %v", err)
	}
	state, restores, _, _, _, applied := lease.snapshot()
	if state != "l:"+handle.Current || len(restores) != 2 || applied != 1 {
		t.Fatalf("repeated restore = state=%s restores=%v applied=%d", state, restores, applied)
	}
}
