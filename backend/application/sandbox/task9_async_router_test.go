// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type task9AsyncRuntime struct {
	mu              sync.Mutex
	executeResults  []task9ExecuteOutcome
	statusResults   []task9ExecuteOutcome
	executeKeys     []string
	executeRequests []infrasandbox.ExecuteRequest
	statusIDs       []string
	keepAliveIDs    []string
	cancelIDs       []string
	closeCalls      int
}

type task9ExecuteOutcome struct {
	result infrasandbox.ExecuteResult
	err    error
}

func (r *task9AsyncRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{
		ProtocolVersion: infrasandbox.HealthProtocolV1,
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAppDev},
	}, nil
}

func (r *task9AsyncRuntime) Execute(_ context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executeKeys = append(r.executeKeys, request.IdempotencyKey)
	r.executeRequests = append(r.executeRequests, request)
	if len(r.executeResults) == 0 {
		return infrasandbox.ExecuteResult{}, errors.New("unexpected execute")
	}
	outcome := r.executeResults[0]
	r.executeResults = r.executeResults[1:]
	return outcome.result, outcome.err
}

func (r *task9AsyncRuntime) Reconcile(ctx context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.Execute(ctx, request)
}

func (r *task9AsyncRuntime) Status(_ context.Context, executionID string) (infrasandbox.ExecuteResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statusIDs = append(r.statusIDs, executionID)
	if len(r.statusResults) == 0 {
		return infrasandbox.ExecuteResult{}, errors.New("unexpected status")
	}
	outcome := r.statusResults[0]
	r.statusResults = r.statusResults[1:]
	return outcome.result, outcome.err
}

func (r *task9AsyncRuntime) KeepAlive(_ context.Context, executionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.keepAliveIDs = append(r.keepAliveIDs, executionID)
	return nil
}

func (r *task9AsyncRuntime) Cancel(_ context.Context, executionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cancelIDs = append(r.cancelIDs, executionID)
	return nil
}

func (r *task9AsyncRuntime) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeCalls++
	return nil
}

func (r *task9AsyncRuntime) snapshot() (executeKeys, statusIDs, keepAliveIDs, cancelIDs []string, closeCalls int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.executeKeys...), append([]string(nil), r.statusIDs...),
		append([]string(nil), r.keepAliveIDs...), append([]string(nil), r.cancelIDs...), r.closeCalls
}

func task9AsyncRequest(idempotencyKey string) infrasandbox.ExecuteRequest {
	return infrasandbox.ExecuteRequest{
		Scope:          domainsandbox.ScopeAppDev,
		WorkloadKind:   infrasandbox.WorkloadAppDev,
		IdempotencyKey: idempotencyKey,
		Deadline:       time.Now().Add(time.Minute).UTC(),
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30, MemoryLimitMB: 256, CPULimit: 1,
			MaxOutputBytes: 4096, MaxConcurrency: 2,
		},
		Entrypoint: "workspace/server.js",
		Args:       []string{"--preview"},
		Env:        map[string]string{"MODE": "preview"},
	}
}

func task9TerminalResult(executionID string, status infrasandbox.ExecutionStatus) infrasandbox.ExecuteResult {
	result := infrasandbox.ExecuteResult{ExecutionID: executionID, Status: status}
	if status == infrasandbox.ExecutionStatusSucceeded {
		exitCode := 0
		result.ExitCode = &exitCode
	}
	return result
}

func task9RouterHarness(t *testing.T, runtime infrasandbox.RuntimeProvider, limiter *capacityLimiterFuncs) (*ProviderRouter, time.Time) {
	t.Helper()
	now := time.Unix(2_100_000_000, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, nil
		},
	}, limiter)
	return router, now
}

func task9ResolveAppDev(t *testing.T, router *ProviderRouter) *SelectedProvider {
	t.Helper()
	request, err := newResolveProviderRequest(
		testRouterProviderKey,
		domainsandbox.ScopeAppDev,
		strings.NewReader(strings.Repeat("\x01", 16)+strings.Repeat("\x02", 16)),
	)
	if err != nil {
		t.Fatalf("new resolve request: %v", err)
	}
	selected, err := router.Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return selected
}

func TestProviderRouterAsyncAcceptedRunningRetainsLeaseUntilTerminal(t *testing.T) {
	runtime := &task9AsyncRuntime{
		executeResults: []task9ExecuteOutcome{{result: infrasandbox.ExecuteResult{
			ExecutionID: "exec-real", Status: infrasandbox.ExecutionStatusAccepted,
		}}},
		statusResults: []task9ExecuteOutcome{
			{result: infrasandbox.ExecuteResult{ExecutionID: "exec-real", Status: infrasandbox.ExecutionStatusRunning}},
			{result: task9TerminalResult("exec-real", infrasandbox.ExecutionStatusSucceeded)},
		},
	}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{
		release: func(context.Context, string, string, string) error {
			releaseCalls++
			return nil
		},
	})
	selected := task9ResolveAppDev(t, router)

	result, err := selected.Execute(context.Background(), task9AsyncRequest("idem-accepted"))
	if err != nil || result.Status != infrasandbox.ExecutionStatusAccepted {
		t.Fatalf("execute accepted = %#v, %v", result, err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) || releaseCalls != 0 {
		t.Fatalf("accepted release = %v, calls=%d", err, releaseCalls)
	}
	if err := selected.KeepAlive(context.Background()); err != nil {
		t.Fatalf("keep alive: %v", err)
	}
	result, err = selected.Status(context.Background())
	if err != nil || result.Status != infrasandbox.ExecutionStatusRunning {
		t.Fatalf("running status = %#v, %v", result, err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) || releaseCalls != 0 {
		t.Fatalf("running release = %v, calls=%d", err, releaseCalls)
	}
	result, err = selected.Status(context.Background())
	if err != nil || result.Status != infrasandbox.ExecutionStatusSucceeded {
		t.Fatalf("terminal status = %#v, %v", result, err)
	}
	if err := selected.Release(context.Background()); err != nil || releaseCalls != 1 {
		t.Fatalf("terminal release = %v, calls=%d", err, releaseCalls)
	}
	_, statusIDs, keepAliveIDs, _, closeCalls := runtime.snapshot()
	if !reflect.DeepEqual(statusIDs, []string{"exec-real", "exec-real"}) ||
		!reflect.DeepEqual(keepAliveIDs, []string{"exec-real"}) || closeCalls != 1 {
		t.Fatalf("async ids/status = %#v/%#v close=%d", statusIDs, keepAliveIDs, closeCalls)
	}
}

func TestProviderRouterAsyncCancelUsesRecordedIDAndWaitsForTerminal(t *testing.T) {
	runtime := &task9AsyncRuntime{
		executeResults: []task9ExecuteOutcome{{result: infrasandbox.ExecuteResult{
			ExecutionID: "exec-recorded", Status: infrasandbox.ExecutionStatusRunning,
		}}},
		statusResults: []task9ExecuteOutcome{{result: task9TerminalResult("exec-recorded", infrasandbox.ExecutionStatusCanceled)}},
	}
	releaseCalls := 0
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{
		release: func(context.Context, string, string, string) error { releaseCalls++; return nil },
	})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-cancel")); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if err := selected.Cancel(context.Background(), "caller-controlled-id"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) || releaseCalls != 0 {
		t.Fatalf("cancel success released before terminal: %v calls=%d", err, releaseCalls)
	}
	if _, err := selected.Status(context.Background()); err != nil {
		t.Fatalf("terminal status after cancel: %v", err)
	}
	if err := selected.Release(context.Background()); err != nil || releaseCalls != 1 {
		t.Fatalf("release canceled terminal: %v calls=%d", err, releaseCalls)
	}
	_, statusIDs, _, cancelIDs, _ := runtime.snapshot()
	if !reflect.DeepEqual(cancelIDs, []string{"exec-recorded"}) || !reflect.DeepEqual(statusIDs, []string{"exec-recorded"}) {
		t.Fatalf("router trusted caller id: cancel=%#v status=%#v", cancelIDs, statusIDs)
	}
}

func TestProviderRouterResumeRenewsOriginalLeaseBeforeBuild(t *testing.T) {
	now := time.Unix(2_100_000_100, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	runtime := &task9AsyncRuntime{
		executeResults: []task9ExecuteOutcome{{result: infrasandbox.ExecuteResult{
			ExecutionID: "exec-resumed", Status: infrasandbox.ExecutionStatusAccepted,
		}}},
		statusResults: []task9ExecuteOutcome{{
			result: task9TerminalResult("exec-resumed", infrasandbox.ExecutionStatusSucceeded),
		}},
	}
	var events []string
	acquireCalls := 0
	var renewedToken, renewedFence string
	limiter := &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
		renew: func(_ context.Context, _ string, token, fence, _ string, _ time.Duration) (int64, error) {
			events = append(events, "renew")
			renewedToken, renewedFence = token, fence
			return now.Add(4 * time.Second).UnixMilli(), nil
		},
	}
	buildCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			buildCalls++
			if buildCalls > 1 {
				events = append(events, "build")
			}
			return runtime, nil
		},
	}, limiter)
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-resume")); err != nil {
		t.Fatalf("execute before checkpoint: %v", err)
	}
	checkpoint, err := selected.Checkpoint()
	if err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	encoded, err := json.Marshal(checkpoint)
	if err == nil {
		t.Fatalf("checkpoint JSON unexpectedly succeeded: %s", encoded)
	}
	for _, secret := range []string{checkpoint.leaseToken, checkpoint.leaseFence, checkpoint.executionID} {
		if secret != "" && (strings.Contains(string(encoded), secret) || strings.Contains(err.Error(), secret)) {
			t.Fatalf("checkpoint JSON error leaked internal value %q: bytes=%q err=%v", secret, encoded, err)
		}
	}
	typeOfCheckpoint := reflect.TypeOf(checkpoint)
	for index := 0; index < typeOfCheckpoint.NumField(); index++ {
		if typeOfCheckpoint.Field(index).Tag.Get("json") != "" {
			t.Fatalf("checkpoint field %s has JSON tag", typeOfCheckpoint.Field(index).Name)
		}
	}
	if _, err := router.Resume(context.Background(), checkpoint, "exec-forged"); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("resume mismatched execution id: %v", err)
	}
	if acquireCalls != 1 || len(events) != 0 || renewedToken != "" || renewedFence != "" || buildCalls != 1 {
		t.Fatalf("mismatched resume reached lease/provider: acquire=%d events=%#v renew=%q/%q build=%d", acquireCalls, events, renewedToken, renewedFence, buildCalls)
	}
	_, statusIDs, keepAliveIDs, cancelIDs, _ := runtime.snapshot()
	if len(statusIDs) != 0 || len(keepAliveIDs) != 0 || len(cancelIDs) != 0 {
		t.Fatalf("mismatched resume reached provider control: status=%#v keepalive=%#v cancel=%#v", statusIDs, keepAliveIDs, cancelIDs)
	}
	resumed, err := router.Resume(context.Background(), checkpoint, "exec-resumed")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if acquireCalls != 1 || !reflect.DeepEqual(events, []string{"renew", "build"}) {
		t.Fatalf("resume acquire/events = %d/%#v", acquireCalls, events)
	}
	if renewedToken != checkpoint.leaseToken || renewedFence != checkpoint.leaseFence {
		t.Fatalf("resume changed lease identity token=%q fence=%q", renewedToken, renewedFence)
	}
	if _, err := resumed.Status(context.Background()); err != nil {
		t.Fatalf("resumed status: %v", err)
	}
}

func TestProviderRouterCheckpointRequiresRecordedExecutionID(t *testing.T) {
	runtime := &task9AsyncRuntime{}
	renewCalls := 0
	var now time.Time
	var router *ProviderRouter
	router, now = task9RouterHarness(t, runtime, &capacityLimiterFuncs{
		renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
			renewCalls++
			return now.Add(time.Minute).UnixMilli(), nil
		},
	})
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Checkpoint(); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("never-executed selection produced checkpoint: %v", err)
	}
	forged := ExecutionCheckpoint{
		providerKey: testRouterProviderKey, scope: domainsandbox.ScopeAppDev,
		leaseToken: testRouterLeaseToken, leaseFence: testRouterLeaseFence,
		leaseExpiryMilli: now.Add(time.Minute).UnixMilli(),
	}
	if _, err := router.Resume(context.Background(), forged, "exec-forged"); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("checkpoint without bound execution id resumed: %v", err)
	}
	if renewCalls != 0 {
		t.Fatalf("invalid checkpoint renewed lease %d times", renewCalls)
	}
	_, statusIDs, keepAliveIDs, cancelIDs, _ := runtime.snapshot()
	if len(statusIDs) != 0 || len(keepAliveIDs) != 0 || len(cancelIDs) != 0 {
		t.Fatalf("invalid checkpoint reached provider control: status=%#v keepalive=%#v cancel=%#v", statusIDs, keepAliveIDs, cancelIDs)
	}
}

func TestProviderRouterAsyncAmbiguousExecuteRetriesOnlySameRequest(t *testing.T) {
	runtime := &task9AsyncRuntime{
		executeResults: []task9ExecuteOutcome{
			{err: errors.New("transport response lost token must-not-leak")},
			{result: infrasandbox.ExecuteResult{ExecutionID: "exec-recovered", Status: infrasandbox.ExecutionStatusAccepted}},
		},
		statusResults: []task9ExecuteOutcome{{result: task9TerminalResult("exec-recovered", infrasandbox.ExecutionStatusSucceeded)}},
	}
	router, _ := task9RouterHarness(t, runtime, &capacityLimiterFuncs{})
	selected := task9ResolveAppDev(t, router)
	request := task9AsyncRequest("idem-retry")
	if _, err := selected.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) || strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("ambiguous execute error = %v", err)
	}
	changed := task9AsyncRequest("idem-retry")
	changed.Deadline = request.Deadline
	changed.Args = []string{"--different"}
	if _, err := selected.Execute(context.Background(), changed); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("changed request reused selection: %v", err)
	}
	result, err := selected.Execute(context.Background(), request)
	if err != nil || result.ExecutionID != "exec-recovered" {
		t.Fatalf("same request retry = %#v, %v", result, err)
	}
	if _, err := selected.Status(context.Background()); err != nil {
		t.Fatalf("status recovered execution: %v", err)
	}
	executeKeys, _, _, _, _ := runtime.snapshot()
	if !reflect.DeepEqual(executeKeys, []string{"idem-retry", "idem-retry"}) {
		t.Fatalf("ambiguous retry keys = %#v", executeKeys)
	}
}
