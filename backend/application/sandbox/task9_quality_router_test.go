// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type qualityStatusCall struct {
	started chan struct{}
	release chan struct{}
	result  infrasandbox.ExecuteResult
	err     error
}

type qualityLifecycleRuntime struct {
	mu sync.Mutex

	executeResult infrasandbox.ExecuteResult
	executeErr    error
	statusCalls   []*qualityStatusCall
	statusIndex   int

	keepAliveStarted chan struct{}
	keepAliveRelease chan struct{}
	keepAliveErr     error
	cancelStarted    chan struct{}
	cancelRelease    chan struct{}
	cancelErr        error

	closeStarted chan struct{}
	closeOnce    sync.Once
	closeErrors  []error
	closeCalls   int
}

func (r *qualityLifecycleRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{
		ProtocolVersion: infrasandbox.HealthProtocolV1,
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAppDev},
	}, nil
}

func (r *qualityLifecycleRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.executeResult, r.executeErr
}

func (r *qualityLifecycleRuntime) Reconcile(ctx context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.Execute(ctx, request)
}

func (r *qualityLifecycleRuntime) Status(ctx context.Context, _ string) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	if r.statusIndex >= len(r.statusCalls) {
		r.mu.Unlock()
		return infrasandbox.ExecuteResult{}, errors.New("unexpected status")
	}
	call := r.statusCalls[r.statusIndex]
	r.statusIndex++
	r.mu.Unlock()
	if call.started != nil {
		close(call.started)
	}
	if call.release != nil {
		select {
		case <-call.release:
		case <-ctx.Done():
			return infrasandbox.ExecuteResult{}, ctx.Err()
		}
	}
	return call.result, call.err
}

func (r *qualityLifecycleRuntime) KeepAlive(ctx context.Context, _ string) error {
	if r.keepAliveStarted != nil {
		close(r.keepAliveStarted)
	}
	if r.keepAliveRelease != nil {
		select {
		case <-r.keepAliveRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.keepAliveErr
}

func (r *qualityLifecycleRuntime) Cancel(ctx context.Context, _ string) error {
	if r.cancelStarted != nil {
		close(r.cancelStarted)
	}
	if r.cancelRelease != nil {
		select {
		case <-r.cancelRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return r.cancelErr
}

func (r *qualityLifecycleRuntime) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	index := r.closeCalls
	r.closeCalls++
	var err error
	if index < len(r.closeErrors) {
		err = r.closeErrors[index]
	}
	r.mu.Unlock()
	if r.closeStarted != nil {
		r.closeOnce.Do(func() { close(r.closeStarted) })
	}
	return err
}

func (r *qualityLifecycleRuntime) closeCallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeCalls
}

type qualitySyncRuntime struct {
	result     infrasandbox.ExecuteResult
	closeCalls int
}

func (*qualitySyncRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (r *qualitySyncRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.result, nil
}

func (*qualitySyncRuntime) Cancel(context.Context, string) error { return nil }

func (r *qualitySyncRuntime) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.closeCalls++
	return nil
}

type qualitySyncContractRuntime struct {
	mu sync.Mutex

	result       infrasandbox.ExecuteResult
	executeCalls int
	closeCalls   int
	closeErrors  []error
	closeEntered chan int
	closeRelease chan struct{}
}

func (*qualitySyncContractRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (r *qualitySyncContractRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	r.executeCalls++
	r.mu.Unlock()
	return r.result, nil
}

func (*qualitySyncContractRuntime) Cancel(context.Context, string) error { return nil }

func (r *qualitySyncContractRuntime) CloseContext(ctx context.Context) error {
	r.mu.Lock()
	index := r.closeCalls
	r.closeCalls++
	var err error
	if index < len(r.closeErrors) {
		err = r.closeErrors[index]
	}
	entered, release := r.closeEntered, r.closeRelease
	r.mu.Unlock()
	if entered != nil {
		entered <- index + 1
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (r *qualitySyncContractRuntime) counts() (execute, close int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.executeCalls, r.closeCalls
}

type qualityDeadlineRuntime struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
}

func (*qualityDeadlineRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{}, nil
}

func (r *qualityDeadlineRuntime) Execute(ctx context.Context, _ infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.started)
		<-ctx.Done()
		return infrasandbox.ExecuteResult{}, ctx.Err()
	}
	return infrasandbox.ExecuteResult{ExecutionID: "exec-deadline-recovered", Status: infrasandbox.ExecutionStatusAccepted}, nil
}

func (r *qualityDeadlineRuntime) Reconcile(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{ExecutionID: "exec-deadline-recovered", Status: infrasandbox.ExecutionStatusAccepted}, nil
}

func (*qualityDeadlineRuntime) Status(context.Context, string) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, errors.New("unexpected status")
}

func (*qualityDeadlineRuntime) KeepAlive(context.Context, string) error { return nil }
func (*qualityDeadlineRuntime) Cancel(context.Context, string) error    { return nil }
func (*qualityDeadlineRuntime) CloseContext(ctx context.Context) error  { return ctx.Err() }

type qualityBlockingHealthRuntime struct {
	qualityLifecycleRuntime
	started chan struct{}
	release chan struct{}
}

func (r *qualityBlockingHealthRuntime) Health(ctx context.Context) (infrasandbox.HealthResult, error) {
	close(r.started)
	select {
	case <-r.release:
		return infrasandbox.HealthResult{
			ProtocolVersion: infrasandbox.HealthProtocolV1,
			Status:          domainsandbox.HealthStatusHealthy,
			Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAppDev},
		}, nil
	case <-ctx.Done():
		return infrasandbox.HealthResult{}, ctx.Err()
	}
}

func qualityWait(t *testing.T, channel <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-channel:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func qualitySelectedState(selected *SelectedProvider) selectedProviderState {
	selected.mu.Lock()
	defer selected.mu.Unlock()
	return selected.state
}

func qualityLifecycleMutexAvailable(selected *SelectedProvider) <-chan struct{} {
	acquired := make(chan struct{})
	go func() {
		selected.mu.Lock()
		close(acquired)
		selected.mu.Unlock()
	}()
	return acquired
}

func TestProviderRouterBlockingIODoesNotHoldLifecycleMutex(t *testing.T) {
	t.Run("health", func(t *testing.T) {
		runtime := &qualityBlockingHealthRuntime{started: make(chan struct{}), release: make(chan struct{})}
		router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
		selected := task9ResolveAppDev(t, router)
		done := make(chan error, 1)
		go func() { _, err := selected.Health(context.Background()); done <- err }()
		qualityWait(t, runtime.started, "Health start")
		acquired := qualityLifecycleMutexAvailable(selected)
		availableDuringIO := false
		select {
		case <-acquired:
			availableDuringIO = true
		case <-time.After(30 * time.Millisecond):
		}
		close(runtime.release)
		if err := <-done; err != nil {
			t.Fatalf("Health: %v", err)
		}
		if !availableDuringIO {
			qualityWait(t, acquired, "lifecycle mutex after Health")
			t.Fatal("Health held the lifecycle mutex across provider I/O")
		}
	})

	t.Run("renew", func(t *testing.T) {
		started := make(chan struct{})
		unblock := make(chan struct{})
		var now time.Time
		var router *ProviderRouter
		router, now = task9RouterHarness(t, &qualityLifecycleRuntime{}, &capacityLimiterFuncs{
			renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
				close(started)
				<-unblock
				return now.Add(time.Minute).UnixMilli(), nil
			},
		})
		selected := task9ResolveAppDev(t, router)
		done := make(chan error, 1)
		go func() { done <- selected.Renew(context.Background()) }()
		qualityWait(t, started, "Renew start")
		acquired := qualityLifecycleMutexAvailable(selected)
		availableDuringIO := false
		select {
		case <-acquired:
			availableDuringIO = true
		case <-time.After(30 * time.Millisecond):
		}
		close(unblock)
		if err := <-done; err != nil {
			t.Fatalf("Renew: %v", err)
		}
		if !availableDuringIO {
			qualityWait(t, acquired, "lifecycle mutex after Renew")
			t.Fatal("Renew held the lifecycle mutex across limiter I/O")
		}
	})
}

func TestProviderRouterConcurrentStatusTerminalIsMonotonicAndReleaseIsSingleShot(t *testing.T) {
	running := &qualityStatusCall{
		started: make(chan struct{}), release: make(chan struct{}),
		result: infrasandbox.ExecuteResult{ExecutionID: "exec-race", Status: infrasandbox.ExecutionStatusRunning},
	}
	terminal := &qualityStatusCall{
		started: make(chan struct{}), release: make(chan struct{}),
		result: task9TerminalResult("exec-race", infrasandbox.ExecutionStatusSucceeded),
	}
	runtime := &qualityLifecycleRuntime{
		executeResult: infrasandbox.ExecuteResult{ExecutionID: "exec-race", Status: infrasandbox.ExecutionStatusAccepted},
		statusCalls:   []*qualityStatusCall{running, terminal},
		closeStarted:  make(chan struct{}),
	}
	releaseCalled := make(chan struct{})
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		if releaseCalls == 1 {
			close(releaseCalled)
		}
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-status-race")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	runningDone := make(chan error, 1)
	go func() { _, err := selected.Status(context.Background()); runningDone <- err }()
	qualityWait(t, running.started, "running status start")
	terminalDone := make(chan error, 1)
	go func() { _, err := selected.Status(context.Background()); terminalDone <- err }()
	qualityWait(t, terminal.started, "terminal status start")
	close(terminal.release)
	if err := <-terminalDone; err != nil {
		t.Fatalf("terminal status: %v", err)
	}
	if state := qualitySelectedState(selected); state != selectedProviderCompleted {
		t.Fatalf("state after terminal status = %v", state)
	}

	releaseDone := make(chan error, 1)
	go func() { releaseDone <- selected.Release(context.Background()) }()
	qualityWait(t, releaseCalled, "capacity release")
	close(running.release)
	if err := <-runningDone; !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("late running Status = %v", err)
	}
	if err := <-releaseDone; err != nil {
		t.Fatalf("release: %v", err)
	}
	if state := qualitySelectedState(selected); state != selectedProviderReleased {
		t.Fatalf("late running status regressed released state to %v", state)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if releaseCalls != 1 || runtime.closeCallCount() != 1 {
		t.Fatalf("release/close calls = %d/%d", releaseCalls, runtime.closeCallCount())
	}
}

func TestProviderRouterLateControlErrorsCannotRegressRelease(t *testing.T) {
	for _, operation := range []string{"keepalive", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			started := make(chan struct{})
			unblock := make(chan struct{})
			terminal := &qualityStatusCall{result: task9TerminalResult("exec-control-race", infrasandbox.ExecutionStatusSucceeded)}
			runtime := &qualityLifecycleRuntime{
				executeResult: infrasandbox.ExecuteResult{ExecutionID: "exec-control-race", Status: infrasandbox.ExecutionStatusAccepted},
				statusCalls:   []*qualityStatusCall{terminal},
				closeStarted:  make(chan struct{}),
			}
			if operation == "keepalive" {
				runtime.keepAliveStarted, runtime.keepAliveRelease = started, unblock
				runtime.keepAliveErr = errors.New("late keepalive transport failure")
			} else {
				runtime.cancelStarted, runtime.cancelRelease = started, unblock
				runtime.cancelErr = errors.New("late cancel transport failure")
			}
			releaseCalled := make(chan struct{})
			releaseCalls := 0
			router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
				releaseCalls++
				if releaseCalls == 1 {
					close(releaseCalled)
				}
				return nil
			}})
			selected := task9ResolveAppDev(t, router)
			if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-control-race")); err != nil {
				t.Fatalf("execute: %v", err)
			}
			operationDone := make(chan error, 1)
			go func() {
				if operation == "keepalive" {
					operationDone <- selected.KeepAlive(context.Background())
					return
				}
				operationDone <- selected.Cancel(context.Background(), "ignored")
			}()
			qualityWait(t, started, operation+" start")
			if _, err := selected.Status(context.Background()); err != nil {
				t.Fatalf("terminal status: %v", err)
			}
			releaseDone := make(chan error, 1)
			go func() { releaseDone <- selected.Release(context.Background()) }()
			qualityWait(t, releaseCalled, "capacity release")
			close(unblock)
			if err := <-operationDone; !errors.Is(err, domainsandbox.ErrUnavailable) {
				t.Fatalf("late %s error = %v", operation, err)
			}
			if err := <-releaseDone; err != nil {
				t.Fatalf("release: %v", err)
			}
			if state := qualitySelectedState(selected); state != selectedProviderReleased {
				t.Fatalf("late %s regressed state to %v", operation, state)
			}
			if releaseCalls != 1 || runtime.closeCallCount() != 1 {
				t.Fatalf("release/close calls = %d/%d", releaseCalls, runtime.closeCallCount())
			}
		})
	}
}

func TestProviderRouterDefiniteExecuteFailureCanRelease(t *testing.T) {
	runtime := &task9AsyncRuntime{executeResults: []task9ExecuteOutcome{{err: domainsandbox.ErrInvalidInput}}}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-definite")); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("definite execute error = %v", err)
	}
	if err := selected.Release(context.Background()); err != nil || releaseCalls != 1 {
		t.Fatalf("release after definite failure = %v, calls=%d", err, releaseCalls)
	}
}

func TestProviderRouterExecuteDeadlineCanRecoverWithFreshContext(t *testing.T) {
	runtime := &qualityDeadlineRuntime{started: make(chan struct{})}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-deadline-recovery")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := selected.Execute(ctx, request); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline execute error = %v", err)
	}
	result, err := selected.Execute(context.Background(), request)
	if err != nil || result.ExecutionID != "exec-deadline-recovered" {
		t.Fatalf("fresh-context retry = %#v, %v", result, err)
	}
}

func TestProviderRouterSyncProviderNonterminalQuarantinesSelection(t *testing.T) {
	runtime := &qualitySyncContractRuntime{result: infrasandbox.ExecuteResult{
		ExecutionID: "exec-detached-forbidden", Status: infrasandbox.ExecutionStatusAccepted,
	}}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-sync-contract")
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("sync nonterminal result error = %v", err)
	}
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("same request retried after contract violation: %v", err)
	}
	different := request
	different.Args = []string{"--different"}
	if _, err := selected.Execute(context.Background(), different); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("different request executed after contract violation: %v", err)
	}
	if _, err := selected.Health(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Health allowed after contract violation: %v", err)
	}
	if err := selected.Renew(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Renew allowed after contract violation: %v", err)
	}
	if err := selected.Cancel(context.Background(), "exec-detached-forbidden"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Cancel allowed after contract violation: %v", err)
	}
	if executeCalls, _ := runtime.counts(); executeCalls != 1 {
		t.Fatalf("provider Execute calls = %d", executeCalls)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("ordered cleanup: %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("capacity release calls = %d", releaseCalls)
	}
}

func TestProviderRouterSyncContractCleanupClosesBeforeCapacityAndIsIdempotent(t *testing.T) {
	closeEntered := make(chan int, 1)
	closeRelease := make(chan struct{})
	runtime := &qualitySyncContractRuntime{
		result:       infrasandbox.ExecuteResult{ExecutionID: "exec-close-first", Status: infrasandbox.ExecutionStatusRunning},
		closeEntered: closeEntered,
		closeRelease: closeRelease,
	}
	releaseCalled := make(chan struct{}, 1)
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		releaseCalled <- struct{}{}
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-close-first")); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("sync nonterminal result error = %v", err)
	}
	firstRelease := make(chan error, 1)
	go func() { firstRelease <- selected.Release(context.Background()) }()
	select {
	case <-closeEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime Close did not start")
	}
	capacityReleasedBeforeClose := false
	select {
	case <-releaseCalled:
		capacityReleasedBeforeClose = true
	default:
	}
	secondRelease := make(chan error, 1)
	go func() { secondRelease <- selected.Release(context.Background()) }()
	select {
	case err := <-secondRelease:
		t.Fatalf("concurrent contract cleanup returned before shared Close completed: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(closeRelease)
	if err := <-firstRelease; err != nil {
		t.Fatalf("contract cleanup: %v", err)
	}
	if err := <-secondRelease; err != nil {
		t.Fatalf("shared concurrent contract cleanup: %v", err)
	}
	if capacityReleasedBeforeClose {
		t.Fatal("capacity released before runtime Close completed")
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("repeated Release: %v", err)
	}
	_, closeCalls := runtime.counts()
	if releaseCalls != 1 || closeCalls != 1 || qualitySelectedState(selected) != selectedProviderReleased {
		t.Fatalf("release/close/state = %d/%d/%v", releaseCalls, closeCalls, qualitySelectedState(selected))
	}
}

func TestProviderRouterSyncContractCleanupRetriesCloseBeforeCapacity(t *testing.T) {
	runtime := &qualitySyncContractRuntime{
		result:      infrasandbox.ExecuteResult{ExecutionID: "exec-close-retry", Status: infrasandbox.ExecutionStatusAccepted},
		closeErrors: []error{errors.New("close failed"), nil},
	}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-close-retry")
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("sync nonterminal result error = %v", err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("first Close failure = %v", err)
	}
	if releaseCalls != 0 {
		t.Fatalf("capacity released after failed Close: %d", releaseCalls)
	}
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Execute allowed during cleanup pending: %v", err)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("Close retry cleanup: %v", err)
	}
	_, closeCalls := runtime.counts()
	if releaseCalls != 1 || closeCalls != 2 || qualitySelectedState(selected) != selectedProviderReleased {
		t.Fatalf("release/close/state = %d/%d/%v", releaseCalls, closeCalls, qualitySelectedState(selected))
	}
}

func TestProviderRouterSyncContractCleanupRetriesCapacityWithoutRepeatingClose(t *testing.T) {
	runtime := &qualitySyncContractRuntime{result: infrasandbox.ExecuteResult{
		ExecutionID: "exec-capacity-retry", Status: infrasandbox.ExecutionStatusRunning,
	}}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		if releaseCalls == 1 {
			return errors.New("capacity release failed")
		}
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-capacity-retry")
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("sync nonterminal result error = %v", err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("first capacity failure = %v", err)
	}
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Execute allowed between cleanup stages: %v", err)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("capacity retry cleanup: %v", err)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("repeated released cleanup: %v", err)
	}
	_, closeCalls := runtime.counts()
	if releaseCalls != 2 || closeCalls != 1 || qualitySelectedState(selected) != selectedProviderReleased {
		t.Fatalf("release/close/state = %d/%d/%v", releaseCalls, closeCalls, qualitySelectedState(selected))
	}
}

func TestProviderRouterResumeRetryReusesCheckpointRenewalIdentity(t *testing.T) {
	runtime := &task9AsyncRuntime{executeResults: []task9ExecuteOutcome{{result: infrasandbox.ExecuteResult{
		ExecutionID: "exec-resume-quality", Status: infrasandbox.ExecutionStatusAccepted,
	}}}}
	var now time.Time
	var renewalIDs []string
	applied := map[string]int{}
	acquireCalls := 0
	limiter := &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(time.Minute).UnixMilli(), nil
		},
		renew: func(_ context.Context, _, _, _, renewalID string, _ time.Duration) (int64, error) {
			renewalIDs = append(renewalIDs, renewalID)
			if applied[renewalID] == 0 {
				applied[renewalID]++
				return 0, errors.New("renew committed but response lost")
			}
			return now.Add(2 * time.Minute).UnixMilli(), nil
		},
	}
	var router *ProviderRouter
	router, now = task9RouterHarness(t, runtime, limiter)
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-resume-quality")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	checkpoint, err := selected.Checkpoint()
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	if _, err := router.Resume(context.Background(), checkpoint, "exec-resume-quality"); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("lost resume response error = %v", err)
	}
	if _, err := router.Resume(context.Background(), checkpoint, "exec-resume-quality"); err != nil {
		t.Fatalf("resume retry: %v", err)
	}
	if acquireCalls != 1 || len(renewalIDs) != 2 || renewalIDs[0] != renewalIDs[1] || len(applied) != 1 {
		t.Fatalf("acquire/renew identities/applied = %d/%#v/%#v", acquireCalls, renewalIDs, applied)
	}
}

func TestProviderRouterReleaseNilDoesNotMutateOrClose(t *testing.T) {
	runtime := &qualityLifecycleRuntime{}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	if err := selected.Release(nil); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Release(nil) = %v", err)
	}
	if state := qualitySelectedState(selected); state != selectedProviderReady {
		t.Fatalf("Release(nil) mutated state to %v", state)
	}
	if releaseCalls != 0 || runtime.closeCallCount() != 0 {
		t.Fatalf("Release(nil) release/close calls = %d/%d", releaseCalls, runtime.closeCallCount())
	}
}

func TestProviderRouterReleaseRetriesCloseWithoutRepeatingCapacityRelease(t *testing.T) {
	runtime := &qualityLifecycleRuntime{closeErrors: []error{errors.New("close failed once"), nil}}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		return nil
	}})
	selected := task9ResolveAppDev(t, router)
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("first release = %v", err)
	}
	if state := qualitySelectedState(selected); state != selectedProviderCleanupPending {
		t.Fatalf("state after close failure = %v", state)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("close retry release = %v", err)
	}
	if releaseCalls != 1 || runtime.closeCallCount() != 2 {
		t.Fatalf("release/close calls = %d/%d", releaseCalls, runtime.closeCallCount())
	}
}

func TestSandboxOpaqueLifecycleValuesAreRedactedForFormatting(t *testing.T) {
	runtime := &qualityLifecycleRuntime{executeResult: infrasandbox.ExecuteResult{
		ExecutionID: "exec-format-secret", Status: infrasandbox.ExecutionStatusAccepted,
	}}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-format")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	checkpoint, err := selected.Checkpoint()
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	values := []any{checkpoint, selected.lease}
	secrets := []string{checkpoint.leaseToken, checkpoint.leaseFence, checkpoint.executionID}
	for _, value := range values {
		formatted := fmt.Sprintf("%v %+v %#v", value, value, value)
		goStringer, ok := value.(fmt.GoStringer)
		if !ok {
			t.Fatalf("%T does not implement fmt.GoStringer", value)
		}
		formatted += " " + goStringer.GoString()
		for _, secret := range secrets {
			if strings.Contains(formatted, secret) {
				t.Fatalf("%T formatting leaked %q: %s", value, secret, formatted)
			}
		}
	}
	var logOutput bytes.Buffer
	logger := logs.DefaultLogger()
	logger.SetOutput(&logOutput)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	logs.CtxInfof(context.Background(), "checkpoint=%+v lease=%#v", checkpoint, selected.lease)
	for _, secret := range secrets {
		if strings.Contains(logOutput.String(), secret) {
			t.Fatalf("repository logger leaked %q: %s", secret, logOutput.String())
		}
	}
	encoded, err := json.Marshal(checkpoint)
	if err == nil || len(encoded) != 0 {
		t.Fatalf("checkpoint JSON = %q, %v", encoded, err)
	}
}

func TestProviderRouterValidatesEveryExecuteAndStatusResult(t *testing.T) {
	exitOne := 1
	invalidResults := map[string]infrasandbox.ExecuteResult{
		"output limit": {
			ExecutionID: "exec-invalid", Status: infrasandbox.ExecutionStatusRunning,
			Stdout: strings.Repeat("x", 4097),
		},
		"artifact": {
			ExecutionID: "exec-invalid", Status: infrasandbox.ExecutionStatusSucceeded, ExitCode: func() *int { value := 0; return &value }(),
			Artifacts: []infrasandbox.ArtifactSummary{{ID: "artifact-1", Name: "../secret", MediaType: "text/plain", Size: 1}},
		},
		"exit status": {
			ExecutionID: "exec-invalid", Status: infrasandbox.ExecutionStatusSucceeded, ExitCode: &exitOne,
		},
		"status": {
			ExecutionID: "exec-invalid", Status: infrasandbox.ExecutionStatus("mystery"),
		},
		"invalid utf8": {
			ExecutionID: "exec-invalid", Status: infrasandbox.ExecutionStatusRunning, Stdout: string([]byte{0xff}),
		},
	}
	for name, invalid := range invalidResults {
		t.Run("execute "+name, func(t *testing.T) {
			runtime := &task9AsyncRuntime{executeResults: []task9ExecuteOutcome{{result: invalid}}}
			router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
			selected := task9ResolveAppDev(t, router)
			if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-invalid-execute")); !errors.Is(err, domainsandbox.ErrUnavailable) {
				t.Fatalf("invalid Execute result error = %v", err)
			}
		})
		t.Run("status "+name, func(t *testing.T) {
			invalid.ExecutionID = "exec-status-invalid"
			runtime := &task9AsyncRuntime{
				executeResults: []task9ExecuteOutcome{{result: infrasandbox.ExecuteResult{ExecutionID: "exec-status-invalid", Status: infrasandbox.ExecutionStatusAccepted}}},
				statusResults:  []task9ExecuteOutcome{{result: invalid}},
			}
			router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
			selected := task9ResolveAppDev(t, router)
			if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-invalid-status")); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if _, err := selected.Status(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
				t.Fatalf("invalid Status result error = %v", err)
			}
		})
	}
}
