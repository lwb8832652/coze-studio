// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type latestIgnoringRuntime struct {
	mu sync.Mutex

	executeResult infrasandbox.ExecuteResult
	healthStarted chan struct{}
	healthRelease chan struct{}
	healthOnce    sync.Once
	closeStarted  chan struct{}
	closeRelease  chan struct{}
	closeOnce     sync.Once
	closeCalls    int
	closeActive   int
}

func (r *latestIgnoringRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	if r.healthStarted != nil {
		r.healthOnce.Do(func() { close(r.healthStarted) })
	}
	if r.healthRelease != nil {
		<-r.healthRelease
	}
	return infrasandbox.HealthResult{
		ProtocolVersion: infrasandbox.HealthProtocolV1,
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAppDev},
	}, nil
}

func (r *latestIgnoringRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.executeResult, nil
}

func (*latestIgnoringRuntime) Cancel(context.Context, string) error { return nil }

func (r *latestIgnoringRuntime) CloseContext(ctx context.Context) error {
	r.mu.Lock()
	r.closeCalls++
	r.closeActive++
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.closeActive--
		r.mu.Unlock()
	}()
	if r.closeStarted != nil {
		r.closeOnce.Do(func() { close(r.closeStarted) })
	}
	if r.closeRelease != nil {
		select {
		case <-r.closeRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (r *latestIgnoringRuntime) closeCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeCalls
}

func (r *latestIgnoringRuntime) activeCloseCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeActive
}

type latestCancelRuntime struct {
	mu sync.Mutex

	executeResult infrasandbox.ExecuteResult
	statusResults []infrasandbox.ExecuteResult
	cancelErrors  []error
	cancelCalls   int
	cancelStarted chan struct{}
	cancelRelease chan struct{}
	cancelOnce    sync.Once
}

func (*latestCancelRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (r *latestCancelRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.executeResult, nil
}

func (r *latestCancelRuntime) Reconcile(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.executeResult, nil
}

func (r *latestCancelRuntime) Status(context.Context, string) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.statusResults) == 0 {
		return infrasandbox.ExecuteResult{}, errors.New("unexpected status")
	}
	result := r.statusResults[0]
	r.statusResults = r.statusResults[1:]
	return result, nil
}

func (*latestCancelRuntime) KeepAlive(context.Context, string) error { return nil }

func (r *latestCancelRuntime) Cancel(context.Context, string) error {
	r.mu.Lock()
	r.cancelCalls++
	call := r.cancelCalls
	var err error
	if call <= len(r.cancelErrors) {
		err = r.cancelErrors[call-1]
	}
	started, release := r.cancelStarted, r.cancelRelease
	r.mu.Unlock()
	if started != nil {
		r.cancelOnce.Do(func() { close(started) })
	}
	if release != nil && call == 1 {
		<-release
	}
	return err
}

func (*latestCancelRuntime) CloseContext(ctx context.Context) error { return ctx.Err() }

func (r *latestCancelRuntime) cancelCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelCalls
}

type latestReconcileRuntime struct {
	mu sync.Mutex

	executeCalls    int
	reconcileCalls  int
	reconcileResult infrasandbox.ExecuteResult
	reconcileErr    error
}

func (*latestReconcileRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (r *latestReconcileRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	r.executeCalls++
	r.mu.Unlock()
	return infrasandbox.ExecuteResult{}, errors.New("submission response lost")
}

func (r *latestReconcileRuntime) Reconcile(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	r.reconcileCalls++
	r.mu.Unlock()
	return r.reconcileResult, r.reconcileErr
}

func (*latestReconcileRuntime) Status(context.Context, string) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, errors.New("unexpected status")
}

func (*latestReconcileRuntime) KeepAlive(context.Context, string) error { return nil }
func (*latestReconcileRuntime) Cancel(context.Context, string) error    { return nil }
func (*latestReconcileRuntime) CloseContext(ctx context.Context) error  { return ctx.Err() }

func (r *latestReconcileRuntime) counts() (execute, reconcile int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.executeCalls, r.reconcileCalls
}

type latestAsyncWithoutReconcile struct {
	executeCalls int
	closeCalls   int
}

type latestReconciliationMarkerError struct {
	version string
	code    string
}

func (e latestReconciliationMarkerError) Error() string {
	return domainsandbox.ErrExecutionForbidden.Error()
}
func (e latestReconciliationMarkerError) Unwrap() error { return domainsandbox.ErrExecutionForbidden }
func (e latestReconciliationMarkerError) ReconciliationProtocolVersion() string {
	return e.version
}
func (e latestReconciliationMarkerError) ReconciliationCode() string { return e.code }

type latestBuildCloseRuntime struct {
	mu                sync.Mutex
	legacyCloseCalls  int
	contextCloseCalls int
	activeCloseCalls  int
	waitForContext    bool
	closeErr          error
	closeErrors       []error
	blockOnCloseCall  int
	closeStarted      chan struct{}
	closeRelease      chan struct{}
	closeOnce         sync.Once
	maxActiveCloses   int
	order             *[]string
}

func (*latestBuildCloseRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}
func (*latestBuildCloseRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}
func (*latestBuildCloseRuntime) Cancel(context.Context, string) error { return nil }
func (r *latestBuildCloseRuntime) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	r.contextCloseCalls++
	r.activeCloseCalls++
	call := r.contextCloseCalls
	if r.activeCloseCalls > r.maxActiveCloses {
		r.maxActiveCloses = r.activeCloseCalls
	}
	if r.order != nil {
		*r.order = append(*r.order, "close")
	}
	waitForContext, closeErr := r.waitForContext, r.closeErr
	if call <= len(r.closeErrors) {
		closeErr = r.closeErrors[call-1]
	}
	block := r.blockOnCloseCall == call
	started, release := r.closeStarted, r.closeRelease
	r.mu.Unlock()
	if block {
		if started != nil {
			r.closeOnce.Do(func() { close(started) })
		}
		select {
		case <-release:
		case <-ctx.Done():
			closeErr = ctx.Err()
		}
	}
	if waitForContext {
		<-ctx.Done()
		closeErr = ctx.Err()
	}
	r.mu.Lock()
	r.activeCloseCalls--
	r.mu.Unlock()
	return closeErr
}

func (r *latestBuildCloseRuntime) closeCounts() (legacy, contextAware int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.legacyCloseCalls, r.contextCloseCalls
}

func (r *latestBuildCloseRuntime) activeCloses() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeCloseCalls
}

func (r *latestBuildCloseRuntime) maximumActiveCloses() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.maxActiveCloses
}

func (*latestAsyncWithoutReconcile) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (r *latestAsyncWithoutReconcile) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	r.executeCalls++
	return infrasandbox.ExecuteResult{}, errors.New("must not execute")
}

func (*latestAsyncWithoutReconcile) Status(context.Context, string) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}

func (*latestAsyncWithoutReconcile) KeepAlive(context.Context, string) error { return nil }
func (*latestAsyncWithoutReconcile) Cancel(context.Context, string) error    { return nil }
func (r *latestAsyncWithoutReconcile) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.closeCalls++
	return nil
}

func TestProviderRouterReleaseDrainIsBoundedWhenRuntimeIgnoresCancellation(t *testing.T) {
	runtime := &latestIgnoringRuntime{healthStarted: make(chan struct{}), healthRelease: make(chan struct{})}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	healthDone := make(chan error, 1)
	go func() { _, err := selected.Health(context.Background()); healthDone <- err }()
	qualityWait(t, runtime.healthStarted, "blocking Health")
	releaseCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	releaseDone := make(chan error, 1)
	go func() { releaseDone <- selected.Release(releaseCtx) }()
	bounded := false
	var firstReleaseErr error
	select {
	case firstReleaseErr = <-releaseDone:
		bounded = true
	case <-time.After(100 * time.Millisecond):
	}
	if releaseCalls != 0 || runtime.closeCallCount() != 0 {
		t.Errorf("cleanup advanced before drain: release=%d close=%d", releaseCalls, runtime.closeCallCount())
	}
	close(runtime.healthRelease)
	if !bounded {
		firstReleaseErr = <-releaseDone
	}
	if err := <-healthDone; err != nil {
		t.Fatalf("Health completion: %v", err)
	}
	if !bounded || firstReleaseErr == nil {
		t.Fatalf("Release did not return bounded drain failure: bounded=%v err=%v", bounded, firstReleaseErr)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("drain retry cleanup: %v", err)
	}
	if releaseCalls != 1 || runtime.closeCallCount() != 1 {
		t.Fatalf("release/close calls = %d/%d", releaseCalls, runtime.closeCallCount())
	}
}

func TestProviderRouterCloseAttemptIsBoundedSingleflightAndFailClosed(t *testing.T) {
	for _, contractViolation := range []bool{false, true} {
		name := "normal"
		if contractViolation {
			name = "contract violation"
		}
		t.Run(name, func(t *testing.T) {
			runtime := &latestIgnoringRuntime{
				executeResult: infrasandbox.ExecuteResult{ExecutionID: "exec-close-stuck", Status: infrasandbox.ExecutionStatusAccepted},
				closeStarted:  make(chan struct{}),
				closeRelease:  make(chan struct{}),
			}
			releaseCalls := 0
			router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
				releaseCalls++
				return nil
			}})
			selected := task9ResolveAppDev(t, router)
			if contractViolation {
				if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-close-stuck")); !errors.Is(err, domainsandbox.ErrUnavailable) {
					t.Fatalf("contract violation Execute: %v", err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			firstDone := make(chan error, 1)
			go func() { firstDone <- selected.Release(ctx) }()
			qualityWait(t, runtime.closeStarted, "stuck Close")
			if releaseCalls != 0 {
				t.Errorf("capacity released while Close was stuck: %d", releaseCalls)
			}
			secondCtx, secondCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			secondErr := selected.Release(secondCtx)
			secondCancel()
			if secondErr == nil {
				t.Error("retry observed stuck Close as success")
			}
			if runtime.closeCallCount() != 1 {
				t.Errorf("concurrent context Close attempts = %d", runtime.closeCallCount())
			}
			select {
			case firstErr := <-firstDone:
				if firstErr == nil {
					t.Fatal("deadline-bounded Close unexpectedly succeeded")
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Release did not honor Close context deadline")
			}
			if runtime.activeCloseCount() != 0 {
				t.Fatalf("Close call remained active after deadline: %d", runtime.activeCloseCount())
			}
			close(runtime.closeRelease)
			if err := selected.Release(context.Background()); err != nil {
				t.Fatalf("Close completion retry: %v", err)
			}
			if releaseCalls != 1 || runtime.closeCallCount() != 2 || runtime.activeCloseCount() != 0 {
				t.Fatalf("release/close calls = %d/%d", releaseCalls, runtime.closeCallCount())
			}
		})
	}
}

func TestProviderRouterSyncInvalidSuccessAlwaysRequiresContractCleanup(t *testing.T) {
	invalid := []infrasandbox.ExecuteResult{
		{ExecutionID: "exec-invalid-terminal", Status: infrasandbox.ExecutionStatusSucceeded},
		{ExecutionID: "", Status: infrasandbox.ExecutionStatusAccepted},
	}
	for index, result := range invalid {
		runtime := &qualitySyncContractRuntime{result: result}
		releaseCalls := 0
		router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
			releaseCalls++
			return nil
		}})
		selected := task9ResolveAppDev(t, router)
		request := task9AsyncRequest("idem-invalid-sync")
		if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
			t.Fatalf("case %d invalid success error = %v", index, err)
		}
		if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
			t.Fatalf("case %d invalid success was retryable: %v", index, err)
		}
		if err := selected.Release(context.Background()); err != nil {
			t.Fatalf("case %d contract cleanup: %v", index, err)
		}
		_, closeCalls := runtime.counts()
		if closeCalls != 1 || releaseCalls != 1 {
			t.Fatalf("case %d close/release = %d/%d", index, closeCalls, releaseCalls)
		}
	}
}

func TestProviderRouterCancelFailureDoesNotBecomeAcceptedAndCanRetry(t *testing.T) {
	runtime := &latestCancelRuntime{
		executeResult: infrasandbox.ExecuteResult{ExecutionID: "exec-cancel-retry", Status: infrasandbox.ExecutionStatusAccepted},
		statusResults: []infrasandbox.ExecuteResult{{ExecutionID: "exec-cancel-retry", Status: infrasandbox.ExecutionStatusRunning}},
		cancelErrors:  []error{errors.New("cancel transport failed"), nil},
	}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-cancel-retry")); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if err := selected.Cancel(context.Background(), "ignored"); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("first Cancel: %v", err)
	}
	status, err := selected.Status(context.Background())
	if err != nil || status.Status != infrasandbox.ExecutionStatusRunning {
		t.Fatalf("running Status after failed Cancel = %#v, %v", status, err)
	}
	if qualitySelectedState(selected) == selectedProviderCanceling {
		t.Fatal("failed Cancel was treated as accepted")
	}
	if err := selected.Cancel(context.Background(), "ignored"); err != nil {
		t.Fatalf("Cancel retry: %v", err)
	}
	if runtime.cancelCallCount() != 2 || qualitySelectedState(selected) != selectedProviderCanceling {
		t.Fatalf("cancel calls/state = %d/%v", runtime.cancelCallCount(), qualitySelectedState(selected))
	}
}

func TestProviderRouterConcurrentCancelSharesAttemptResult(t *testing.T) {
	runtime := &latestCancelRuntime{
		executeResult: infrasandbox.ExecuteResult{ExecutionID: "exec-cancel-concurrent", Status: infrasandbox.ExecutionStatusAccepted},
		cancelErrors:  []error{errors.New("shared cancel failure")},
		cancelStarted: make(chan struct{}),
		cancelRelease: make(chan struct{}),
	}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-cancel-concurrent")); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	first := make(chan error, 1)
	go func() { first <- selected.Cancel(context.Background(), "ignored") }()
	qualityWait(t, runtime.cancelStarted, "first Cancel")
	second := make(chan error, 1)
	go func() { second <- selected.Cancel(context.Background(), "ignored") }()
	secondReturnedEarly := false
	select {
	case <-second:
		secondReturnedEarly = true
	case <-time.After(25 * time.Millisecond):
	}
	close(runtime.cancelRelease)
	firstErr := <-first
	var secondErr error
	if secondReturnedEarly {
		secondErr = nil
	} else {
		secondErr = <-second
	}
	if secondReturnedEarly || !errors.Is(firstErr, domainsandbox.ErrUnavailable) || !errors.Is(secondErr, domainsandbox.ErrUnavailable) {
		t.Fatalf("concurrent Cancel results early/first/second = %v/%v/%v", secondReturnedEarly, firstErr, secondErr)
	}
	if runtime.cancelCallCount() != 1 {
		t.Fatalf("provider Cancel calls = %d", runtime.cancelCallCount())
	}
}

func TestProviderRouterUncertainRetryUsesReconciliation(t *testing.T) {
	runtime := &latestReconcileRuntime{reconcileResult: infrasandbox.ExecuteResult{
		ExecutionID: "exec-reconciled", Status: infrasandbox.ExecutionStatusAccepted,
	}}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-reconcile")
	request.Deadline = time.Now().Add(-time.Minute)
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("ambiguous Execute: %v", err)
	}
	result, err := selected.Execute(context.Background(), request)
	if err != nil || result.ExecutionID != "exec-reconciled" {
		t.Fatalf("Reconcile result = %#v, %v", result, err)
	}
	if executeCalls, reconcileCalls := runtime.counts(); executeCalls != 1 || reconcileCalls != 1 {
		t.Fatalf("execute/reconcile calls = %d/%d", executeCalls, reconcileCalls)
	}
}

func TestProviderRouterForgedAuthoritativeMarkerCannotCleanup(t *testing.T) {
	runtime := &latestReconcileRuntime{reconcileErr: latestReconciliationMarkerError{
		version: "1", code: "execution_not_recorded",
	}}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-reconcile-missing")
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("ambiguous Execute: %v", err)
	}
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("forged reconciliation result: %v", err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) || releaseCalls != 0 {
		t.Fatalf("forged marker enabled cleanup = %v, calls=%d", err, releaseCalls)
	}
}

func TestProviderRouterRejectsAsyncProviderWithoutReconciliationAfterLease(t *testing.T) {
	now := time.Unix(2_100_000_500, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	runtime := &latestAsyncWithoutReconcile{}
	acquireCalls, releaseCalls := 0, 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, nil
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(time.Minute).UnixMilli(), nil
		},
		release: func(context.Context, string, string, string) error {
			releaseCalls++
			return nil
		},
	})
	request, err := NewResolveProviderRequest(testRouterProviderKey, domainsandbox.ScopeAppDev)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("Resolve async provider without reconciliation = %v", err)
	}
	if acquireCalls != 1 || releaseCalls != 1 || runtime.executeCalls != 0 || runtime.closeCalls != 1 {
		t.Fatalf(
			"acquire/release/execute/close = %d/%d/%d/%d",
			acquireCalls, releaseCalls, runtime.executeCalls, runtime.closeCalls,
		)
	}
}

func TestProviderRouterReconcileOnlyAuthoritativeNotRecordedCanLeaveUncertain(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "plain definite error", err: domainsandbox.ErrInvalidInput},
		{name: "wrong protocol", err: latestReconciliationMarkerError{version: "2", code: "execution_not_recorded"}},
		{name: "wrong code", err: latestReconciliationMarkerError{version: "1", code: "other"}},
		{name: "forged exact public methods", err: latestReconciliationMarkerError{version: "1", code: "execution_not_recorded"}},
		{name: "matching string", err: errors.New("execution_not_recorded")},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &latestReconcileRuntime{reconcileErr: test.err}
			router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
			selected := task9ResolveAppDev(t, router)
			request := task9AsyncRequest("idem-authoritative-reconcile")
			if _, err := selected.Execute(context.Background(), request); err == nil {
				t.Fatal("ambiguous initial Execute succeeded")
			}
			if _, err := selected.Execute(context.Background(), request); err == nil {
				t.Fatal("reconciliation error was hidden")
			}
			different := request
			different.Args = []string{"--different"}
			_, differentErr := selected.Execute(context.Background(), different)
			if !errors.Is(differentErr, domainsandbox.ErrExecutionForbidden) {
				t.Fatalf("non-authoritative reconcile escaped uncertain: %v", differentErr)
			}
		})
	}
}

func TestProviderRouterCanceledReconcileLeavesOriginalSubmissionUncertain(t *testing.T) {
	runtime := &latestReconcileRuntime{}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-canceled-reconcile")
	if _, err := selected.Execute(context.Background(), request); err == nil {
		t.Fatal("ambiguous initial Execute succeeded")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := selected.Execute(canceled, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled reconciliation = %v", err)
	}
	different := request
	different.Args = []string{"--different"}
	if _, err := selected.Execute(context.Background(), different); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("pre-canceled reconciliation escaped uncertain: %v", err)
	}
	if executeCalls, reconcileCalls := runtime.counts(); executeCalls != 1 || reconcileCalls != 0 {
		t.Fatalf("execute/reconcile calls = %d/%d", executeCalls, reconcileCalls)
	}
}

func TestProviderRouterConstructsLeaseBeforeRuntimeBuild(t *testing.T) {
	now := time.Now().UTC()
	runtime := &latestBuildCloseRuntime{}
	buildCalls, releaseCalls := 0, 0
	router := newRouterForTest(t, now, healthyRouterProvider(now), &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			buildCalls++
			return runtime, nil
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			return 0, nil
		},
		release: func(context.Context, string, string, string) error {
			releaseCalls++
			return nil
		},
	})
	if _, err := router.Resolve(context.Background(), testRouterRequest()); err == nil {
		t.Fatal("invalid acquired lease unexpectedly resolved")
	}
	if buildCalls != 0 || releaseCalls != 1 {
		t.Fatalf("lease/build compensation calls = build:%d release:%d", buildCalls, releaseCalls)
	}
	legacy, contextAware := runtime.closeCounts()
	if legacy != 0 || contextAware != 0 {
		t.Fatalf("runtime was created/closed despite lease construction failure: %d/%d", legacy, contextAware)
	}
}

func TestProviderRouterBuildFailureCompensationIsBoundedCloseFirst(t *testing.T) {
	for _, closeErr := range []error{nil, errors.New("close failed")} {
		name := "close succeeds"
		if closeErr != nil {
			name = "close fails"
		}
		t.Run(name, func(t *testing.T) {
			now := time.Now().UTC()
			order := []string{}
			runtime := &latestBuildCloseRuntime{closeErr: closeErr, order: &order}
			releaseCalls := 0
			router := newRouterForTest(t, now, healthyRouterProvider(now), &runtimeProviderFactoryFuncs{
				build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
					order = append(order, "build")
					return runtime, errors.New("build failed after allocation")
				},
			}, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
				order = append(order, "release")
				releaseCalls++
				return nil
			}})
			if _, err := router.Resolve(context.Background(), testRouterRequest()); err == nil {
				t.Fatal("post-allocation Build failure unexpectedly resolved")
			}
			legacy, contextAware := runtime.closeCounts()
			if legacy != 0 || contextAware != 1 {
				t.Fatalf("close contract calls = legacy:%d context:%d", legacy, contextAware)
			}
			if closeErr == nil {
				if strings.Join(order, ",") != "build,close,release" || releaseCalls != 1 {
					t.Fatalf("successful compensation order/calls = %v/%d", order, releaseCalls)
				}
			} else if strings.Join(order, ",") != "build,close" || releaseCalls != 0 {
				t.Fatalf("failed Close released capacity: %v/%d", order, releaseCalls)
			}
		})
	}
}

func TestProviderRouterBuildFailureCloseTimeoutRetainsCapacity(t *testing.T) {
	now := time.Now().UTC()
	runtime := &latestBuildCloseRuntime{waitForContext: true}
	releaseCalls := 0
	router := newRouterForTest(t, now, healthyRouterProvider(now), &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, errors.New("build failed after allocation")
		},
	}, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := router.Resolve(ctx, testRouterRequest()); err == nil {
		t.Fatal("Build/Close timeout unexpectedly resolved")
	}
	if elapsed := time.Since(started); elapsed < 100*time.Millisecond || elapsed > selectedProviderCleanupTimeout+2*time.Second {
		t.Fatalf("Build compensation did not use its detached bounded deadline: %s", elapsed)
	}
	_, contextAware := runtime.closeCounts()
	if contextAware != 1 || runtime.activeCloses() != 0 || releaseCalls != 0 {
		t.Fatalf("timeout close/active/release = %d/%d/%d", contextAware, runtime.activeCloses(), releaseCalls)
	}
}

func TestProviderRouterResumeBuildFailureCompensation(t *testing.T) {
	tests := []struct {
		name           string
		runtime        *latestBuildCloseRuntime
		buildRuntime   bool
		releaseErr     error
		wantOrder      string
		wantCloseCalls int
		wantRelease    int
	}{
		{name: "nil runtime", wantOrder: "renew,build,release", wantRelease: 1},
		{name: "runtime close succeeds", runtime: &latestBuildCloseRuntime{}, buildRuntime: true, wantOrder: "renew,build,close,release", wantCloseCalls: 1, wantRelease: 1},
		{name: "runtime close fails", runtime: &latestBuildCloseRuntime{closeErr: errors.New("close secret must-not-leak")}, buildRuntime: true, wantOrder: "renew,build,close", wantCloseCalls: 1},
		{name: "capacity release fails", runtime: &latestBuildCloseRuntime{}, buildRuntime: true, releaseErr: errors.New("release token must-not-leak"), wantOrder: "renew,build,close,release", wantCloseCalls: 1, wantRelease: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			order := []string{}
			provider := healthyRouterProvider(now)
			provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
			provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
			if test.runtime != nil {
				test.runtime.order = &order
			}
			releaseCalls := 0
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
				build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
					order = append(order, "build")
					if test.buildRuntime {
						return test.runtime, errors.New("build credential must-not-leak")
					}
					return nil, errors.New("build credential must-not-leak")
				},
			}, &capacityLimiterFuncs{
				renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
					order = append(order, "renew")
					return now.Add(time.Minute).UnixMilli(), nil
				},
				release: func(context.Context, string, string, string) error {
					order = append(order, "release")
					releaseCalls++
					return test.releaseErr
				},
			})
			checkpoint := ExecutionCheckpoint{
				providerKey: testRouterProviderKey, scope: domainsandbox.ScopeAppDev,
				leaseToken: testRouterLeaseToken, leaseFence: testRouterLeaseFence,
				leaseExpiryMilli: now.Add(time.Minute).UnixMilli(), executionID: "exec-resume-build-failure",
			}
			_, err := router.Resume(context.Background(), checkpoint, checkpoint.executionID)
			if err == nil || strings.Contains(err.Error(), "must-not-leak") ||
				strings.Contains(err.Error(), testRouterLeaseToken) || strings.Contains(err.Error(), testRouterLeaseFence) {
				t.Fatalf("Resume error leaked or succeeded: %v", err)
			}
			closeCalls := 0
			if test.runtime != nil {
				_, closeCalls = test.runtime.closeCounts()
			}
			if strings.Join(order, ",") != test.wantOrder || closeCalls != test.wantCloseCalls || releaseCalls != test.wantRelease {
				t.Fatalf("order/close/release = %v/%d/%d", order, closeCalls, releaseCalls)
			}
		})
	}
}

func TestProviderRouterResumeBuildCloseTimeoutRetainsLease(t *testing.T) {
	now := time.Now().UTC()
	order := []string{}
	runtime := &latestBuildCloseRuntime{waitForContext: true, order: &order}
	releaseCalls := 0
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			order = append(order, "build")
			return runtime, errors.New("build failed")
		},
	}, &capacityLimiterFuncs{
		renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
			order = append(order, "renew")
			return now.Add(time.Minute).UnixMilli(), nil
		},
		release: func(context.Context, string, string, string) error {
			releaseCalls++
			return nil
		},
	})
	checkpoint := ExecutionCheckpoint{
		providerKey: testRouterProviderKey, scope: domainsandbox.ScopeAppDev,
		leaseToken: testRouterLeaseToken, leaseFence: testRouterLeaseFence,
		leaseExpiryMilli: now.Add(time.Minute).UnixMilli(), executionID: "exec-resume-close-timeout",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := router.Resume(ctx, checkpoint, checkpoint.executionID); err == nil {
		t.Fatal("Resume Close timeout unexpectedly succeeded")
	}
	_, closeCalls := runtime.closeCounts()
	if strings.Join(order, ",") != "renew,build,close" || closeCalls != 1 || runtime.activeCloses() != 0 || releaseCalls != 0 {
		t.Fatalf("timeout order/close/active/release = %v/%d/%d/%d", order, closeCalls, runtime.activeCloses(), releaseCalls)
	}
}

func TestProviderRouterPendingBuildCleanupBlocksRebuildUntilRecovered(t *testing.T) {
	now := time.Now().UTC()
	order := []string{}
	failedRuntime := &latestBuildCloseRuntime{
		closeErrors: []error{errors.New("close one"), errors.New("close two"), nil}, order: &order,
	}
	healthyRuntime := &latestBuildCloseRuntime{}
	buildCalls, acquireCalls, releaseCalls := 0, 0, 0
	var cancelFirst context.CancelFunc
	router := newRouterForTest(t, now, healthyRouterProvider(now), &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			buildCalls++
			order = append(order, "build")
			if buildCalls == 1 {
				cancelFirst()
				return failedRuntime, errors.New("build secret must-not-leak")
			}
			return healthyRuntime, nil
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			order = append(order, "acquire")
			return now.Add(time.Minute).UnixMilli(), nil
		},
		release: func(context.Context, string, string, string) error {
			releaseCalls++
			order = append(order, "release")
			return nil
		},
	})
	firstCtx, cancel := context.WithCancel(context.Background())
	cancelFirst = cancel
	if _, err := router.Resolve(firstCtx, testRouterRequest()); err == nil || strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("first failed Build result = %v", err)
	}
	if _, closeCalls := failedRuntime.closeCounts(); closeCalls != 1 {
		t.Fatalf("canceled parent prevented compensation Close: %d", closeCalls)
	}
	if _, err := router.Resolve(context.Background(), testRouterRequest()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("failed pending cleanup Resolve = %v", err)
	}
	if buildCalls != 1 || acquireCalls != 1 || releaseCalls != 0 {
		t.Fatalf("failed cleanup rebuilt/acquired/released = %d/%d/%d", buildCalls, acquireCalls, releaseCalls)
	}
	selected, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil || selected == nil {
		t.Fatalf("recovered cleanup Resolve = %v", err)
	}
	if _, closeCalls := failedRuntime.closeCounts(); closeCalls != 3 || buildCalls != 2 || acquireCalls != 2 || releaseCalls != 1 {
		t.Fatalf("recovery close/build/acquire/release = %d/%d/%d/%d", closeCalls, buildCalls, acquireCalls, releaseCalls)
	}
}

func latestPendingCleanupLease(t *testing.T, limiter CapacityLimiter, providerKey string) *capacityLease {
	t.Helper()
	lease, err := newCapacityLease(
		limiter,
		providerKey,
		deriveOpaqueLeaseToken("pending-cleanup-test-token", providerKey),
		deriveOpaqueLeaseToken("pending-cleanup-test-fence", providerKey),
		time.Minute,
		time.Now().Add(time.Minute).UnixMilli(),
	)
	if err != nil {
		t.Fatalf("new pending cleanup lease: %v", err)
	}
	return lease
}

func TestProviderRouterStalePendingCleanupGroupCannotBecomeOwner(t *testing.T) {
	key := pendingBuildCleanupKey{providerKey: "stale-cleanup-provider", scope: domainsandbox.ScopeAgent}
	var callsMu sync.Mutex
	releaseCalls, buildCalls := 0, 0
	runtime := &latestBuildCloseRuntime{}
	limiter := &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		callsMu.Lock()
		releaseCalls++
		callsMu.Unlock()
		return nil
	}}
	router := &ProviderRouter{factory: &runtimeProviderFactoryFuncs{build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
		callsMu.Lock()
		buildCalls++
		callsMu.Unlock()
		return &latestBuildCloseRuntime{}, nil
	}}}
	router.registerPendingBuildCleanup(key, runtime, latestPendingCleanupLease(t, limiter, key.providerKey))
	stale, ok := router.lookupPendingBuildCleanup(key)
	if !ok {
		t.Fatal("pending cleanup group was not registered")
	}

	held := make(chan struct{})
	tryTakeover := make(chan struct{})
	type takeoverResult struct {
		reload bool
		err    error
	}
	result := make(chan takeoverResult, 1)
	go func() {
		close(held)
		<-tryTakeover
		reload, err := router.retryPendingBuildCleanupSnapshot(context.Background(), key, stale)
		result <- takeoverResult{reload: reload, err: err}
	}()
	<-held
	if err := router.retryPendingBuildCleanup(context.Background(), key); err != nil {
		t.Fatalf("owner cleanup: %v", err)
	}
	close(tryTakeover)
	select {
	case got := <-result:
		if got.err != nil || got.reload {
			t.Fatalf("stale takeover result = reload:%v err:%v", got.reload, got.err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("stale group takeover did not return promptly")
	}
	callsMu.Lock()
	defer callsMu.Unlock()
	_, closeCalls := runtime.closeCounts()
	if closeCalls != 1 || releaseCalls != 1 || buildCalls != 0 {
		t.Fatalf("stale takeover repeated close/release/build = %d/%d/%d", closeCalls, releaseCalls, buildCalls)
	}
}

func TestProviderRouterDetachedEmptyPendingCleanupDoesNotSpin(t *testing.T) {
	router := &ProviderRouter{}
	key := pendingBuildCleanupKey{providerKey: "detached-empty-provider", scope: domainsandbox.ScopeAgent}
	group := &pendingBuildCleanupGroup{generation: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- router.performPendingBuildCleanup(ctx, key, group)
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("empty detached cleanup = %v", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("empty detached cleanup spun after context timeout")
	}
}

func TestProviderRouterPendingBuildCleanupHighConcurrencySingleflight(t *testing.T) {
	key := pendingBuildCleanupKey{providerKey: "concurrent-cleanup-provider", scope: domainsandbox.ScopeAgent}
	runtime := &latestBuildCloseRuntime{
		blockOnCloseCall: 1,
		closeStarted:     make(chan struct{}),
		closeRelease:     make(chan struct{}),
	}
	var releaseMu sync.Mutex
	releaseCalls := 0
	limiter := &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseMu.Lock()
		releaseCalls++
		releaseMu.Unlock()
		return nil
	}}
	router := &ProviderRouter{}
	router.registerPendingBuildCleanup(key, runtime, latestPendingCleanupLease(t, limiter, key.providerKey))

	const goroutines = 64
	results := make(chan error, goroutines)
	for range goroutines {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			results <- router.retryPendingBuildCleanup(ctx, key)
		}()
	}
	select {
	case <-runtime.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("concurrent cleanup owner did not start")
	}
	close(runtime.closeRelease)
	for range goroutines {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("concurrent cleanup = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent cleanup waiter did not finish")
		}
	}
	_, closeCalls := runtime.closeCounts()
	releaseMu.Lock()
	defer releaseMu.Unlock()
	if closeCalls != 1 || runtime.maximumActiveCloses() != 1 || releaseCalls != 1 {
		t.Fatalf("concurrent close/active/release = %d/%d/%d", closeCalls, runtime.maximumActiveCloses(), releaseCalls)
	}
}

func TestProviderRouterPendingBuildCleanupRetryIsSingleflight(t *testing.T) {
	now := time.Now().UTC()
	failedRuntime := &latestBuildCloseRuntime{
		closeErrors:      []error{errors.New("first close failed"), nil},
		blockOnCloseCall: 2, closeStarted: make(chan struct{}), closeRelease: make(chan struct{}),
	}
	healthyRuntime := &latestBuildCloseRuntime{}
	buildCalls, releaseCalls := 0, 0
	var callsMu sync.Mutex
	router := newRouterForTest(t, now, healthyRouterProvider(now), &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			callsMu.Lock()
			buildCalls++
			buildCall := buildCalls
			callsMu.Unlock()
			if buildCall == 1 {
				return failedRuntime, errors.New("build failed")
			}
			return healthyRuntime, nil
		},
	}, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		callsMu.Lock()
		releaseCalls++
		callsMu.Unlock()
		return nil
	}})
	if _, err := router.Resolve(context.Background(), testRouterRequest()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("initial cleanup failure = %v", err)
	}
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := router.Resolve(context.Background(), testRouterRequest())
			results <- err
		}()
	}
	select {
	case <-failedRuntime.closeStarted:
	case <-time.After(time.Second):
		t.Fatal("pending cleanup retry did not start")
	}
	if _, closeCalls := failedRuntime.closeCounts(); closeCalls != 2 || failedRuntime.maximumActiveCloses() != 1 {
		t.Fatalf("concurrent cleanup Close calls/active = %d/%d", closeCalls, failedRuntime.maximumActiveCloses())
	}
	close(failedRuntime.closeRelease)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Resolve after shared cleanup = %v", err)
		}
	}
	if _, closeCalls := failedRuntime.closeCounts(); closeCalls != 2 || releaseCalls != 1 {
		t.Fatalf("shared cleanup close/release = %d/%d", closeCalls, releaseCalls)
	}
}

func TestExecutionCheckpointCommentAndFormattingRemainOpaque(t *testing.T) {
	checkpoint := ExecutionCheckpoint{executionID: "must-not-format"}
	if strings.Contains(checkpoint.String(), checkpoint.executionID) || strings.Contains(checkpoint.GoString(), checkpoint.executionID) {
		t.Fatal("checkpoint formatting exposed execution ID")
	}
}
