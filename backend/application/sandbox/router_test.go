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

	"github.com/alicebob/miniredis/v2"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	testRouterProviderKey = "provider-router-primary"
	testRouterLeaseToken  = "AQEBAQEBAQEBAQEBAQEBAQ"
	testRouterLeaseFence  = "AgICAgICAgICAgICAgICAg"
)

func TestProviderRouterRoutesPluginWithoutAppDevLookupRequirement(t *testing.T) {
	now := time.Unix(2_000_000_010, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopePlugin}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopePlugin}
	router := newRouterForTest(
		t,
		now,
		provider,
		&runtimeProviderFactoryFuncs{},
		&capacityLimiterFuncs{},
	)
	request, err := NewResolveProviderRequest(
		testRouterProviderKey,
		domainsandbox.ScopePlugin,
	)
	if err != nil {
		t.Fatalf("NewResolveProviderRequest() error = %v", err)
	}

	selected, err := router.Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if selected.Scope != domainsandbox.ScopePlugin {
		t.Fatalf("selected scope = %q, want %q", selected.Scope, domainsandbox.ScopePlugin)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func TestProviderRouterPluginScopeFailsClosedWhenProviderDoesNotSupportIt(t *testing.T) {
	now := time.Unix(2_000_000_020, 0).UTC()
	provider := healthyRouterProvider(now)
	acquireCalls := 0
	router := newRouterForTest(
		t,
		now,
		provider,
		&runtimeProviderFactoryFuncs{},
		&capacityLimiterFuncs{
			acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
				acquireCalls++
				return now.Add(time.Minute).UnixMilli(), nil
			},
		},
	)
	request, err := NewResolveProviderRequest(
		testRouterProviderKey,
		domainsandbox.ScopePlugin,
	)
	if err != nil {
		t.Fatalf("NewResolveProviderRequest() error = %v", err)
	}

	if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrScopeUnsupported) {
		t.Fatalf("Resolve() error = %v, want %v", err, domainsandbox.ErrScopeUnsupported)
	}
	if acquireCalls != 0 {
		t.Fatalf("unsupported plugin scope acquired capacity %d times", acquireCalls)
	}
}

func TestProviderRouterFeatureAwareAdmissionLimitUsesPersistedSchedulerSettings(t *testing.T) {
	now := time.Unix(2_000_000_030, 0).UTC()
	for name, test := range map[string]struct {
		features      []domainsandbox.ProviderFeature
		settings      domainsandbox.SchedulerSettings
		settingsErr   error
		setRepository bool
		wantLimit     int
		wantErr       error
	}{
		"legacy provider retains policy concurrency": {
			settings:  domainsandbox.DefaultSchedulerSettings(),
			wantLimit: 2,
		},
		"queue provider uses max outstanding": {
			features:      []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1},
			settings:      schedulerSettingsWithMaxOutstanding(19),
			setRepository: true,
			wantLimit:     19,
		},
		"queue provider fails closed without repository": {
			features: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1},
			wantErr:  domainsandbox.ErrConfigurationInvalid,
		},
		"queue provider fails closed when settings read fails": {
			features:      []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1},
			settingsErr:   errors.New("scheduler repository unavailable"),
			setRepository: true,
			wantErr:       domainsandbox.ErrUnavailable,
		},
		"queue provider fails closed for invalid settings": {
			features:      []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1},
			settings:      schedulerSettingsWithMaxOutstanding(0),
			setRepository: true,
			wantErr:       domainsandbox.ErrConfigurationInvalid,
		},
	} {
		t.Run(name, func(t *testing.T) {
			provider := healthyRouterProvider(now)
			provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
			provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
			provider.Health.Features = append([]domainsandbox.ProviderFeature(nil), test.features...)
			limits := make([]int, 0, 2)
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{
				acquire: func(_ context.Context, _ string, _ string, _ string, limit int, _ time.Duration) (int64, error) {
					limits = append(limits, limit)
					return now.Add(time.Minute).UnixMilli(), nil
				},
			})
			repository := &schedulerSettingsRepositoryFake{settings: test.settings, err: test.settingsErr}
			if test.setRepository {
				router.SetSchedulerSettingsRepository(repository)
			}

			request, err := NewResolveProviderRequest(testRouterProviderKey, domainsandbox.ScopeAppDev)
			if err != nil {
				t.Fatalf("NewResolveProviderRequest() error = %v", err)
			}
			selected, err := router.Resolve(context.Background(), request)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) || selected != nil || len(limits) != 0 {
					t.Fatalf("Resolve() error/selection/acquires = %v/%t/%d", err, selected != nil, len(limits))
				}
				return
			}
			if err != nil || selected == nil || !reflect.DeepEqual(limits, []int{test.wantLimit}) {
				t.Fatalf("Resolve() error/selection/limits = %v/%t/%v", err, selected != nil, limits)
			}
			if err := selected.Release(context.Background()); err != nil {
				t.Fatalf("Release() error = %v", err)
			}

			digest, digestErr := infrasandbox.ExecutionRequestDigestFromBytes([]byte(strings.Repeat("a", 32)))
			if digestErr != nil {
				t.Fatalf("ExecutionRequestDigestFromBytes() error = %v", digestErr)
			}
			lookupRequest, requestErr := NewLookupProviderExecutionRequest(
				testRouterProviderKey, domainsandbox.ScopeAppDev, testExecutionLookupNotFoundOperation, digest,
			)
			if requestErr != nil {
				t.Fatalf("NewLookupProviderExecutionRequest() error = %v", requestErr)
			}
			lookup, lookupErr := router.LookupProviderExecution(context.Background(), lookupRequest)
			if lookupErr != nil || lookup.Status != infrasandbox.ExecutionLookupNotFound ||
				!reflect.DeepEqual(limits, []int{test.wantLimit, test.wantLimit}) {
				t.Fatalf("LookupProviderExecution() error/status/limits = %v/%q/%v", lookupErr, lookup.Status, limits)
			}
		})
	}
}

func TestProviderRouterQueueCheckpointKeepsRecordedAdmissionLimitOnResume(t *testing.T) {
	now := time.Unix(2_000_000_040, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Features = []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureQueueStatusV1,
		domainsandbox.ProviderFeatureSignedExecutionContext,
	}
	runtime := &task9AsyncRuntime{executeResults: []task9ExecuteOutcome{{
		result: infrasandbox.ExecuteResult{ExecutionID: "exec-admission-checkpoint", Status: infrasandbox.ExecutionStatusAccepted},
	}}}
	repository := &schedulerSettingsRepositoryFake{settings: schedulerSettingsWithMaxOutstanding(23)}
	acquireCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, nil
		},
	}, &capacityLimiterFuncs{
		acquire: func(_ context.Context, _ string, _ string, _ string, limit int, _ time.Duration) (int64, error) {
			acquireCalls++
			if limit != 23 {
				t.Fatalf("Acquire() limit = %d, want 23", limit)
			}
			return now.Add(time.Minute).UnixMilli(), nil
		},
		renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
			return now.Add(2 * time.Minute).UnixMilli(), nil
		},
	})
	router.SetSchedulerSettingsRepository(repository)
	selected := task9ResolveAppDev(t, router)
	if _, err := selected.Execute(context.Background(), task9AsyncRequest("idem-admission-checkpoint")); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	checkpoint, err := selected.Checkpoint()
	if err != nil || checkpoint.admissionLimit != 23 || !checkpoint.queueStatusFeature ||
		!checkpoint.signedExecutionContextFeature {
		t.Fatalf("Checkpoint() error/limit/features = %v/%d/%t/%t", err, checkpoint.admissionLimit, checkpoint.queueStatusFeature, checkpoint.signedExecutionContextFeature)
	}
	provider.Health.Features = nil
	repository.settings = schedulerSettingsWithMaxOutstanding(1)
	resumed, err := router.Resume(context.Background(), checkpoint, checkpoint.executionID)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if !resumed.HasFeature(domainsandbox.ProviderFeatureSignedExecutionContext) {
		t.Fatal("Resume() lost signed execution context feature")
	}
	if acquireCalls != 1 || repository.calls != 1 {
		t.Fatalf("Resume() reinterpreted admission configuration: acquires=%d settings_reads=%d", acquireCalls, repository.calls)
	}
}

func TestProviderRouterLegacyCheckpointPreservesLegacyAdmissionOnResume(t *testing.T) {
	now := time.Unix(2_000_000_050, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
	provider.Health.Features = []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1}
	runtime := &task9AsyncRuntime{}
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, nil
		},
	}, &capacityLimiterFuncs{
		renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
			return now.Add(2 * time.Minute).UnixMilli(), nil
		},
	})
	checkpoint := ExecutionCheckpoint{
		providerKey: testRouterProviderKey, scope: domainsandbox.ScopeAppDev,
		leaseToken: testRouterLeaseToken, leaseFence: testRouterLeaseFence,
		leaseExpiryMilli: now.Add(time.Minute).UnixMilli(), executionID: "exec-legacy-checkpoint",
	}
	resumed, err := router.Resume(context.Background(), checkpoint, checkpoint.executionID)
	if err != nil {
		t.Fatalf("Resume() error = %v", err)
	}
	if resumed.admissionLimit != provider.Policy.MaxConcurrency || resumed.HasFeature(domainsandbox.ProviderFeatureQueueStatusV1) {
		t.Fatalf("legacy resume admission/feature = %d/%t", resumed.admissionLimit, resumed.HasFeature(domainsandbox.ProviderFeatureQueueStatusV1))
	}
	if resumedCheckpoint, checkpointErr := resumed.Checkpoint(); checkpointErr != nil || resumedCheckpoint.admissionLimit != provider.Policy.MaxConcurrency || resumedCheckpoint.queueStatusFeature {
		t.Fatalf("Checkpoint() error/limit/feature = %v/%d/%t", checkpointErr, resumedCheckpoint.admissionLimit, resumedCheckpoint.queueStatusFeature)
	}
}

func TestSelectedProviderExecutesIdentityOnlyForSignedContextProviders(t *testing.T) {
	now := time.Unix(2_000_000_060, 0).UTC()
	identity := infrasandbox.ExecutionIdentity{SpaceID: 11, UserID: 22, ExecutionID: "run_33"}
	for _, test := range []struct {
		name         string
		features     []domainsandbox.ProviderFeature
		wantIdentity bool
	}{
		{name: "legacy provider", wantIdentity: false},
		{name: "signed runner", features: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureSignedExecutionContext}, wantIdentity: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := healthyRouterProvider(now)
			provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAgent}
			provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAgent}
			provider.Health.Features = test.features
			runtime := &task9AsyncRuntime{executeResults: []task9ExecuteOutcome{{
				result: infrasandbox.ExecuteResult{ExecutionID: "exec-identity-feature", Status: infrasandbox.ExecutionStatusAccepted},
			}}}
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
				build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
					return runtime, nil
				},
			}, &capacityLimiterFuncs{})
			request, err := NewResolveProviderRequest(provider.ProviderKey, domainsandbox.ScopeAgent)
			if err != nil {
				t.Fatalf("NewResolveProviderRequest() error = %v", err)
			}
			selected, err := router.Resolve(context.Background(), request)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			execute := task9AsyncRequest("identity-feature")
			execute.Scope = domainsandbox.ScopeAgent
			execute.WorkloadKind = infrasandbox.WorkloadAgent
			execute.Identity = identity
			if _, err := selected.Execute(context.Background(), execute); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if len(runtime.executeRequests) != 1 {
				t.Fatalf("execute request count = %d, want 1", len(runtime.executeRequests))
			}
			if got := !runtime.executeRequests[0].Identity.IsZero(); got != test.wantIdentity {
				t.Fatalf("identity retained = %t, want %t", got, test.wantIdentity)
			}
		})
	}
}

func schedulerSettingsWithMaxOutstanding(maxOutstanding int) domainsandbox.SchedulerSettings {
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.MaxOutstanding = maxOutstanding
	return settings
}

type schedulerSettingsRepositoryFake struct {
	settings domainsandbox.SchedulerSettings
	err      error
	calls    int
}

type sessionSettingsRepositoryFake struct {
	settings domainsandbox.SessionRuntimeSettings
	err      error
	calls    int
}

func (f *sessionSettingsRepositoryFake) GetSessionSettings(context.Context) (domainsandbox.SessionRuntimeSettings, error) {
	f.calls++
	if f.err != nil {
		return domainsandbox.SessionRuntimeSettings{}, f.err
	}
	return f.settings, nil
}

func (*sessionSettingsRepositoryFake) UpdateSessionSettingsCAS(context.Context, domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, error) {
	return domainsandbox.SessionRuntimeSettings{}, domainsandbox.ErrInvalidInput
}

func enabledSessionSettingsRepository() *sessionSettingsRepositoryFake {
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.CoreEnabled = true
	settings.Version = domainsandbox.InitialVersion
	return &sessionSettingsRepositoryFake{settings: settings}
}

func (f *schedulerSettingsRepositoryFake) GetSchedulerSettings(context.Context) (domainsandbox.SchedulerSettings, error) {
	f.calls++
	if f.err != nil {
		return domainsandbox.SchedulerSettings{}, f.err
	}
	return f.settings, nil
}

func (*schedulerSettingsRepositoryFake) UpdateSchedulerSettingsCAS(context.Context, domainsandbox.UpdateSchedulerSettingsInput) (domainsandbox.SchedulerSettings, error) {
	return domainsandbox.SchedulerSettings{}, domainsandbox.ErrInvalidInput
}

type cleanupTraceKey struct{}

type failingRequestEntropy struct{}

func (failingRequestEntropy) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

type routerStaticScriptCmd struct {
	result interface{}
	err    error
}

func (c *routerStaticScriptCmd) Err() error {
	return c.err
}

func (c *routerStaticScriptCmd) Result() (interface{}, error) {
	return c.result, c.err
}

type routerScriptClientFunc func(context.Context, string, []string, ...interface{}) cache.ScriptCmd

func (f routerScriptClientFunc) RunScript(
	ctx context.Context,
	script string,
	keys []string,
	args ...interface{},
) cache.ScriptCmd {
	return f(ctx, script, keys, args...)
}

type providerLookupFunc func(context.Context, string) (*domainsandbox.Provider, error)

func (f providerLookupFunc) GetProviderByKey(ctx context.Context, providerKey string) (*domainsandbox.Provider, error) {
	return f(ctx, providerKey)
}

type runtimeProviderFactoryFuncs struct {
	validate        func(context.Context, ProviderDescriptor) error
	build           func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error)
	validateSession func(context.Context, ProviderDescriptor) error
	buildSession    func(context.Context, domainsandbox.Provider, ProviderDescriptor) (infrasandbox.SessionRuntimeProvider, error)
}

func (f *runtimeProviderFactoryFuncs) ValidateConfig(
	ctx context.Context,
	descriptor ProviderDescriptor,
) error {
	if f.validate == nil {
		return nil
	}
	return f.validate(ctx, descriptor)
}

func (f *runtimeProviderFactoryFuncs) Build(
	ctx context.Context,
	provider domainsandbox.Provider,
) (infrasandbox.RuntimeProvider, error) {
	if f.build == nil {
		return &runtimeProviderStub{}, nil
	}
	return f.build(ctx, provider)
}

func (f *runtimeProviderFactoryFuncs) ValidateSessionConfig(
	ctx context.Context,
	descriptor ProviderDescriptor,
) error {
	if f.validateSession == nil {
		return nil
	}
	return f.validateSession(ctx, descriptor)
}

func (f *runtimeProviderFactoryFuncs) BuildSession(
	ctx context.Context,
	provider domainsandbox.Provider,
	descriptor ProviderDescriptor,
) (infrasandbox.SessionRuntimeProvider, error) {
	if f.buildSession == nil {
		return &sessionRuntimeProviderStub{}, nil
	}
	return f.buildSession(ctx, provider, descriptor)
}

type sessionRuntimeProviderStub struct {
	acquireCalls        int
	getCalls            int
	releaseSessionCalls int
	destroyCalls        int
	recoverCalls        int
	closeCalls          int
	closeErr            error
}

func (p *sessionRuntimeProviderStub) Acquire(context.Context, infrasandbox.AcquireSessionRequest) (infrasandbox.SandboxSession, error) {
	p.acquireCalls++
	return nil, nil
}

func (p *sessionRuntimeProviderStub) Get(context.Context, domainsandbox.SessionRef) (infrasandbox.SandboxSession, error) {
	p.getCalls++
	return nil, nil
}

func (p *sessionRuntimeProviderStub) Release(context.Context, domainsandbox.SessionRef) error {
	p.releaseSessionCalls++
	return nil
}

func (p *sessionRuntimeProviderStub) Destroy(context.Context, domainsandbox.SessionRef) error {
	p.destroyCalls++
	return nil
}

func (p *sessionRuntimeProviderStub) Recover(context.Context, domainsandbox.SessionRef) (infrasandbox.SandboxSession, error) {
	p.recoverCalls++
	return nil, nil
}

func (p *sessionRuntimeProviderStub) CloseContext(context.Context) error {
	p.closeCalls++
	return p.closeErr
}

type runtimeProviderStub struct {
	healthCalls  int
	executeCalls int
	cancelCalls  int
	closeCalls   int
	closeErr     error
}

type queueStatusRuntimeProviderStub struct {
	runtimeProviderStub
	queueStatusCalls int
	queueStatus      infrasandbox.QueueStatus
	queueStatusErr   error
}

func (p *queueStatusRuntimeProviderStub) QueueStatus(context.Context, string) (infrasandbox.QueueStatus, error) {
	p.queueStatusCalls++
	return p.queueStatus, p.queueStatusErr
}

func TestSelectedProviderQueueStatusRequiresDeclaredFeatureAndRuntimeSupport(t *testing.T) {
	runtime := &queueStatusRuntimeProviderStub{queueStatus: infrasandbox.QueueStatus{
		Schema: infrasandbox.QueueStatusSchemaV1, Waiting: true, ApproximatePosition: 2,
	}}
	selected := newSelectedProvider(
		ProviderDescriptor{ProviderKey: testRouterProviderKey, Scope: domainsandbox.ScopeAppDev},
		runtime,
		nil,
		selectedProviderActive,
		"exec-queue-status",
	)
	if _, err := selected.QueueStatus(context.Background()); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("QueueStatus() without feature error = %v", err)
	}
	if runtime.queueStatusCalls != 0 {
		t.Fatalf("QueueStatus() called runtime without declared feature: %d", runtime.queueStatusCalls)
	}

	selected.features = []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1}
	status, err := selected.QueueStatus(context.Background())
	if err != nil || status != runtime.queueStatus || runtime.queueStatusCalls != 1 {
		t.Fatalf("QueueStatus() status/error/calls = %#v/%v/%d", status, err, runtime.queueStatusCalls)
	}

	unsupported := newSelectedProvider(
		ProviderDescriptor{
			ProviderKey: testRouterProviderKey,
			Scope:       domainsandbox.ScopeAppDev,
			features:    []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureQueueStatusV1},
		},
		&runtimeProviderStub{},
		nil,
		selectedProviderActive,
		"exec-queue-status-unsupported",
	)
	if _, err := unsupported.QueueStatus(context.Background()); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("QueueStatus() unsupported runtime error = %v", err)
	}
}

type blockingRuntimeProvider struct {
	started      chan struct{}
	finish       chan struct{}
	startOnce    sync.Once
	mu           sync.Mutex
	executeCalls int
	cancelIDs    []string
	closeCalls   int
}

func newBlockingRuntimeProvider() *blockingRuntimeProvider {
	return &blockingRuntimeProvider{started: make(chan struct{}), finish: make(chan struct{})}
}

func (p *blockingRuntimeProvider) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{Status: domainsandbox.HealthStatusHealthy}, nil
}

func (p *blockingRuntimeProvider) Execute(
	context.Context,
	infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	p.mu.Lock()
	p.executeCalls++
	p.mu.Unlock()
	p.startOnce.Do(func() { close(p.started) })
	<-p.finish
	exitCode := 0
	return infrasandbox.ExecuteResult{
		ExecutionID: "execution-running",
		Status:      infrasandbox.ExecutionStatusSucceeded,
		ExitCode:    &exitCode,
	}, nil
}

func (p *blockingRuntimeProvider) Cancel(_ context.Context, executionID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelIDs = append(p.cancelIDs, executionID)
	return nil
}

func (p *blockingRuntimeProvider) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeCalls++
	return nil
}

func (p *blockingRuntimeProvider) counts() (int, []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.executeCalls, append([]string(nil), p.cancelIDs...)
}

func (p *runtimeProviderStub) Health(context.Context) (infrasandbox.HealthResult, error) {
	p.healthCalls++
	return infrasandbox.HealthResult{
		ProtocolVersion: "v1",
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAgent},
	}, nil
}

func (p *runtimeProviderStub) Execute(
	context.Context,
	infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	p.executeCalls++
	exitCode := 0
	return infrasandbox.ExecuteResult{ExecutionID: "execution-1", Status: infrasandbox.ExecutionStatusSucceeded, ExitCode: &exitCode}, nil
}

func (p *runtimeProviderStub) Cancel(context.Context, string) error {
	p.cancelCalls++
	return nil
}

func (p *runtimeProviderStub) CloseContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.closeCalls++
	return p.closeErr
}

type capacityLimiterFuncs struct {
	acquire func(context.Context, string, string, string, int, time.Duration) (int64, error)
	renew   func(context.Context, string, string, string, string, time.Duration) (int64, error)
	release func(context.Context, string, string, string) error
}

func (f *capacityLimiterFuncs) Acquire(
	ctx context.Context,
	providerKey string,
	token string,
	fence string,
	capacity int,
	duration time.Duration,
) (int64, error) {
	if f.acquire == nil {
		return 2_000_999_999_000, nil
	}
	return f.acquire(ctx, providerKey, token, fence, capacity, duration)
}

func (f *capacityLimiterFuncs) Renew(
	ctx context.Context,
	providerKey string,
	token string,
	fence string,
	renewalID string,
	duration time.Duration,
) (int64, error) {
	if f.renew == nil {
		return 2_001_000_000_000, nil
	}
	return f.renew(ctx, providerKey, token, fence, renewalID, duration)
}

func (f *capacityLimiterFuncs) Release(ctx context.Context, providerKey, token, fence string) error {
	if f.release == nil {
		return nil
	}
	return f.release(ctx, providerKey, token, fence)
}

func healthyRouterProvider(now time.Time) *domainsandbox.Provider {
	return &domainsandbox.Provider{
		ID:               100,
		ProviderKey:      testRouterProviderKey,
		Name:             "Primary sandbox",
		Type:             domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret:   "encrypted-endpoint-secret",
		CredentialSecret: "encrypted-credential-secret",
		Scopes:           []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30,
			MemoryLimitMB:  128,
			CPULimit:       1,
			MaxOutputBytes: 4096,
			MaxConcurrency: 2,
		},
		Status: domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{
			Status:       domainsandbox.HealthStatusHealthy,
			Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
			CheckedAt:    now,
		},
	}
}

func testRouterRequest() ResolveProviderRequest {
	entropy := strings.NewReader(strings.Repeat("\x01", 16) + strings.Repeat("\x02", 16))
	request, err := newResolveProviderRequest(testRouterProviderKey, domainsandbox.ScopeAgent, entropy)
	if err != nil {
		panic(err)
	}
	return request
}

func newRouterForTest(
	t *testing.T,
	now time.Time,
	provider *domainsandbox.Provider,
	factory *runtimeProviderFactoryFuncs,
	limiter *capacityLimiterFuncs,
) *ProviderRouter {
	t.Helper()
	lookup := providerLookupFunc(func(ctx context.Context, providerKey string) (*domainsandbox.Provider, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if provider == nil || providerKey != provider.ProviderKey {
			return nil, domainsandbox.ErrProviderNotFound
		}
		copy := *provider
		return &copy, nil
	})
	router, err := newProviderRouter(lookup, factory, limiter, 2*time.Second, func() time.Time {
		return now
	})
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	router.SetSessionSettingsRepository(enabledSessionSettingsRepository())
	return router
}

func TestProviderRouterResolveSessionRequiresBothFeaturesAndNeverAcquiresCapacity(t *testing.T) {
	now := time.Unix(2_000_001_000, 0).UTC()
	tests := []struct {
		name     string
		features []domainsandbox.ProviderFeature
		want     error
	}{
		{name: "none", want: domainsandbox.ErrScopeUnsupported},
		{name: "manager only", features: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureSandboxSessionV1}, want: domainsandbox.ErrScopeUnsupported},
		{name: "signing only", features: []domainsandbox.ProviderFeature{domainsandbox.ProviderFeatureSignedSessionContextV2}, want: domainsandbox.ErrScopeUnsupported},
		{name: "both", features: []domainsandbox.ProviderFeature{
			domainsandbox.ProviderFeatureSandboxSessionV1,
			domainsandbox.ProviderFeatureSignedSessionContextV2,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := healthyRouterProvider(now)
			provider.Health.Features = tt.features
			capacityCalls := 0
			sessionRuntime := &sessionRuntimeProviderStub{}
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
				buildSession: func(_ context.Context, got domainsandbox.Provider, descriptor ProviderDescriptor) (infrasandbox.SessionRuntimeProvider, error) {
					if got.ID != provider.ID || got.ProviderKey != provider.ProviderKey {
						t.Fatalf("BuildSession() provider identity = %d/%q", got.ID, got.ProviderKey)
					}
					if descriptor.Scope != domainsandbox.ScopeAgent {
						t.Fatalf("BuildSession() descriptor scope = %q", descriptor.Scope)
					}
					return sessionRuntime, nil
				},
			}, &capacityLimiterFuncs{acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
				capacityCalls++
				return now.Add(time.Minute).UnixMilli(), nil
			}})

			selection, err := router.ResolveSession(context.Background(), testRouterRequest())
			if !errors.Is(err, tt.want) {
				t.Fatalf("ResolveSession() error = %v, want %v", err, tt.want)
			}
			if capacityCalls != 0 {
				t.Fatalf("ResolveSession() acquired one-shot capacity %d times", capacityCalls)
			}
			if tt.want != nil {
				if selection != nil {
					t.Fatal("ineligible provider returned a session selection")
				}
				return
			}
			if selection == nil || selection.ProviderKey != provider.ProviderKey {
				t.Fatalf("ResolveSession() selection = %#v", selection)
			}
			if _, err := selection.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{}); err != nil {
				t.Fatalf("selection.Acquire() error = %v", err)
			}
			if sessionRuntime.acquireCalls != 1 {
				t.Fatalf("session manager Acquire() calls = %d, want 1", sessionRuntime.acquireCalls)
			}
			if err := selection.CloseContext(context.Background()); err != nil {
				t.Fatalf("selection.CloseContext() error = %v", err)
			}
			if sessionRuntime.closeCalls != 1 || sessionRuntime.releaseSessionCalls != 0 {
				t.Fatalf("selection cleanup close/session release calls = %d/%d", sessionRuntime.closeCalls, sessionRuntime.releaseSessionCalls)
			}
			if _, err := selection.Get(context.Background(), domainsandbox.SessionRef{}); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
				t.Fatalf("selection.Get() after CloseContext error = %v", err)
			}
		})
	}
}

func TestProviderRouterResolveSessionReadsPersistedCoreGateBeforeFactory(t *testing.T) {
	now := time.Unix(2_000_001_050, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Health.Features = []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureSandboxSessionV1,
		domainsandbox.ProviderFeatureSignedSessionContextV2,
	}
	tests := []struct {
		name       string
		repository *sessionSettingsRepositoryFake
		inject     bool
		want       error
	}{
		{name: "repository missing", want: domainsandbox.ErrConfigurationInvalid},
		{name: "repository unavailable", inject: true, repository: &sessionSettingsRepositoryFake{err: errors.New("database unavailable")}, want: domainsandbox.ErrUnavailable},
		{name: "invalid persisted snapshot", inject: true, repository: &sessionSettingsRepositoryFake{settings: domainsandbox.SessionRuntimeSettings{Version: 1}}, want: domainsandbox.ErrConfigurationInvalid},
		{name: "core disabled", inject: true, repository: func() *sessionSettingsRepositoryFake {
			settings := domainsandbox.DefaultSessionRuntimeSettings()
			settings.Version = 1
			return &sessionSettingsRepositoryFake{settings: settings}
		}(), want: domainsandbox.ErrExecutionForbidden},
		{name: "core enabled", inject: true, repository: enabledSessionSettingsRepository()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validateCalls, buildCalls := 0, 0
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
				validateSession: func(context.Context, ProviderDescriptor) error {
					validateCalls++
					return nil
				},
				buildSession: func(context.Context, domainsandbox.Provider, ProviderDescriptor) (infrasandbox.SessionRuntimeProvider, error) {
					buildCalls++
					return &sessionRuntimeProviderStub{}, nil
				},
			}, &capacityLimiterFuncs{})
			router.SetSessionSettingsRepository(nil)
			if test.inject {
				router.SetSessionSettingsRepository(test.repository)
			}

			selection, err := router.ResolveSession(context.Background(), testRouterRequest())
			if !errors.Is(err, test.want) {
				t.Fatalf("ResolveSession() error = %v, want %v", err, test.want)
			}
			if test.want != nil {
				if selection != nil || validateCalls != 0 || buildCalls != 0 {
					t.Fatalf("closed gate reached factory: selection=%t validate=%d build=%d", selection != nil, validateCalls, buildCalls)
				}
				if test.inject && test.repository.calls != 1 {
					t.Fatalf("GetSessionSettings() calls = %d, want 1", test.repository.calls)
				}
				return
			}
			if selection == nil || validateCalls != 1 || buildCalls != 1 || test.repository.calls != 1 {
				t.Fatalf("enabled gate selection/validate/build/reads = %t/%d/%d/%d", selection != nil, validateCalls, buildCalls, test.repository.calls)
			}
			if closeErr := selection.CloseContext(context.Background()); closeErr != nil {
				t.Fatalf("CloseContext() error = %v", closeErr)
			}
		})
	}
}

func TestProviderRouterResolveSessionFailsClosedForHealthFactoryAndRequestState(t *testing.T) {
	now := time.Unix(2_000_001_100, 0).UTC()
	tests := []struct {
		name   string
		mutate func(*domainsandbox.Provider)
		want   error
	}{
		{name: "disabled", mutate: func(p *domainsandbox.Provider) { p.Status = domainsandbox.ProviderStatusDisabled }, want: domainsandbox.ErrProviderDisabled},
		{name: "unhealthy", mutate: func(p *domainsandbox.Provider) { p.Health.Status = domainsandbox.HealthStatusUnhealthy }, want: domainsandbox.ErrProviderUnhealthy},
		{name: "stale", mutate: func(p *domainsandbox.Provider) {
			p.Health.CheckedAt = now.Add(-DefaultProviderHealthMaxAge - time.Second)
		}, want: domainsandbox.ErrProviderUnhealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := healthyRouterProvider(now)
			provider.Health.Features = []domainsandbox.ProviderFeature{
				domainsandbox.ProviderFeatureSandboxSessionV1,
				domainsandbox.ProviderFeatureSignedSessionContextV2,
			}
			tt.mutate(provider)
			buildCalls := 0
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
				buildSession: func(context.Context, domainsandbox.Provider, ProviderDescriptor) (infrasandbox.SessionRuntimeProvider, error) {
					buildCalls++
					return &sessionRuntimeProviderStub{}, nil
				},
			}, &capacityLimiterFuncs{})
			if selection, err := router.ResolveSession(context.Background(), testRouterRequest()); !errors.Is(err, tt.want) || selection != nil {
				t.Fatalf("ResolveSession() = %#v, %v; want nil, %v", selection, err, tt.want)
			}
			if buildCalls != 0 {
				t.Fatalf("ineligible provider reached Session factory %d times", buildCalls)
			}
		})
	}

	provider := healthyRouterProvider(now)
	provider.Health.Features = []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureSandboxSessionV1,
		domainsandbox.ProviderFeatureSignedSessionContextV2,
	}
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{})
	request := testRouterRequest()
	selection, err := router.ResolveSession(context.Background(), request)
	if err != nil {
		t.Fatalf("first ResolveSession() error = %v", err)
	}
	defer func() { _ = selection.CloseContext(context.Background()) }()
	if _, err := router.ResolveSession(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("reused Session resolve request error = %v", err)
	}
}

func TestProviderRouterResolveSessionUsesValidatedRequestedScopeForMultiScopeProvider(t *testing.T) {
	now := time.Unix(2_000_001_200, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev, domainsandbox.ScopeAgent}
	provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeAppDev, domainsandbox.ScopeAgent}
	provider.Health.Features = []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureSandboxSessionV1,
		domainsandbox.ProviderFeatureSignedSessionContextV2,
	}
	var builtScope domainsandbox.Scope
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		buildSession: func(_ context.Context, _ domainsandbox.Provider, descriptor ProviderDescriptor) (infrasandbox.SessionRuntimeProvider, error) {
			builtScope = descriptor.Scope
			return &sessionRuntimeProviderStub{}, nil
		},
	}, &capacityLimiterFuncs{})

	selection, err := router.ResolveSession(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("ResolveSession() error = %v", err)
	}
	defer func() { _ = selection.CloseContext(context.Background()) }()
	if builtScope != domainsandbox.ScopeAgent || selection.Scope != domainsandbox.ScopeAgent {
		t.Fatalf("BuildSession()/selection scope = %q/%q, want requested agent", builtScope, selection.Scope)
	}
}

func TestProviderRouterResolveSessionDoesNotFallbackToOneShotFactory(t *testing.T) {
	now := time.Unix(2_000_001_300, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Health.Features = []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureSandboxSessionV1,
		domainsandbox.ProviderFeatureSignedSessionContextV2,
	}
	oneShotBuildCalls := 0
	factory := runtimeProviderFactoryWithoutSessions{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			oneShotBuildCalls++
			return &runtimeProviderStub{}, nil
		},
	}
	lookup := providerLookupFunc(func(context.Context, string) (*domainsandbox.Provider, error) {
		copy := *provider
		return &copy, nil
	})
	router, err := newProviderRouter(lookup, factory, &capacityLimiterFuncs{}, 2*time.Second, func() time.Time { return now })
	if err != nil {
		t.Fatalf("newProviderRouter() error = %v", err)
	}
	if selection, err := router.ResolveSession(context.Background(), testRouterRequest()); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) || selection != nil {
		t.Fatalf("ResolveSession() = %#v, %v", selection, err)
	}
	if oneShotBuildCalls != 0 {
		t.Fatalf("ResolveSession() fell back to one-shot Build %d times", oneShotBuildCalls)
	}
}

func TestSelectedSessionProviderCloseContextIsRetryableAndNeverReleasesSession(t *testing.T) {
	runtime := &sessionRuntimeProviderStub{closeErr: errors.New("temporary close failure")}
	selection := newSelectedSessionProviderForTest(runtime)
	if err := selection.CloseContext(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("first CloseContext() error = %v", err)
	}
	if runtime.releaseSessionCalls != 0 {
		t.Fatalf("selection cleanup released Runtime Session %d times", runtime.releaseSessionCalls)
	}
	runtime.closeErr = nil
	if err := selection.CloseContext(context.Background()); err != nil {
		t.Fatalf("retry CloseContext() error = %v", err)
	}
	if runtime.closeCalls != 2 || runtime.releaseSessionCalls != 0 {
		t.Fatalf("retry cleanup close/session release calls = %d/%d", runtime.closeCalls, runtime.releaseSessionCalls)
	}
	if err := selection.Destroy(context.Background(), domainsandbox.SessionRef{}); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Destroy() after selection cleanup error = %v", err)
	}
}

func newSelectedSessionProviderForTest(runtime infrasandbox.SessionRuntimeProvider) *SelectedSessionProvider {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &SelectedSessionProvider{
		runtime: runtime, state: selectedSessionProviderReady,
		lifecycleCtx: lifecycleCtx, lifecycleCancel: lifecycleCancel,
	}
}

type runtimeProviderFactoryWithoutSessions struct {
	build func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error)
}

func (runtimeProviderFactoryWithoutSessions) ValidateConfig(context.Context, ProviderDescriptor) error {
	return nil
}

func (f runtimeProviderFactoryWithoutSessions) Build(ctx context.Context, provider domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return f.build(ctx, provider)
}

func TestProviderRouterSelectsExactProviderAndReturnsSanitizedMetadata(t *testing.T) {
	now := time.Unix(2_000_001_000, 0).UTC()
	provider := healthyRouterProvider(now)
	var lookedUpKey, acquiredKey, releasedKey, releasedToken string
	runtime := &runtimeProviderStub{}
	lookup := providerLookupFunc(func(_ context.Context, key string) (*domainsandbox.Provider, error) {
		lookedUpKey = key
		copy := *provider
		return &copy, nil
	})
	factory := &runtimeProviderFactoryFuncs{
		build: func(_ context.Context, got domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			if got.EndpointSecret != provider.EndpointSecret || got.CredentialSecret != provider.CredentialSecret {
				t.Fatal("adapter factory did not receive encrypted provider material")
			}
			return runtime, nil
		},
	}
	limiter := &capacityLimiterFuncs{
		acquire: func(_ context.Context, key, token, fence string, capacity int, duration time.Duration) (int64, error) {
			acquiredKey = key
			if token != testRouterLeaseToken || fence != testRouterLeaseFence {
				t.Fatalf("router did not reuse request lease identity token=%q fence=%q", token, fence)
			}
			if capacity != provider.Policy.MaxConcurrency || duration != 2*time.Second {
				t.Fatalf("unexpected lease config capacity=%d duration=%s", capacity, duration)
			}
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
		release: func(_ context.Context, key, token, fence string) error {
			if fence != testRouterLeaseFence {
				t.Fatalf("release lost lease fence %q", fence)
			}
			releasedKey, releasedToken = key, token
			return nil
		},
	}
	router, err := newProviderRouter(lookup, factory, limiter, 2*time.Second, func() time.Time { return now })
	if err != nil {
		t.Fatalf("create router: %v", err)
	}

	selected, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if lookedUpKey != testRouterProviderKey || acquiredKey != testRouterProviderKey {
		t.Fatalf("router did not use exact provider key: lookup=%q acquire=%q", lookedUpKey, acquiredKey)
	}
	if selected.ProviderKey != testRouterProviderKey || selected.Scope != domainsandbox.ScopeAgent {
		t.Fatalf("unexpected selection %#v", selected)
	}
	selectionType := reflect.TypeOf(*selected)
	if _, exposed := selectionType.FieldByName("Runtime"); exposed {
		t.Fatal("selected provider exposes raw runtime")
	}
	if field, exists := selectionType.FieldByName("runtime"); !exists || field.PkgPath == "" {
		t.Fatal("selected provider runtime must remain unexported")
	}
	if _, exposed := selectionType.FieldByName("Token"); exposed {
		t.Fatal("selected provider exposes lease token")
	}
	encoded, err := json.Marshal(selected)
	if err != nil {
		t.Fatalf("marshal selected provider metadata: %v", err)
	}
	for _, prohibited := range []string{provider.EndpointSecret, provider.CredentialSecret, testRouterLeaseToken} {
		if strings.Contains(string(encoded), prohibited) {
			t.Fatalf("selected provider leaked secret %q: %s", prohibited, encoded)
		}
	}
	if health, err := selected.Health(context.Background()); err != nil || health.Status != domainsandbox.HealthStatusHealthy {
		t.Fatalf("controlled health delegation failed: health=%#v err=%v", health, err)
	}
	if result, err := selected.Execute(context.Background(), infrasandbox.ExecuteRequest{}); err != nil || result.ExecutionID != "execution-1" {
		t.Fatalf("controlled execute delegation failed: result=%#v err=%v", result, err)
	}
	if err := selected.Cancel(context.Background(), "execution-1"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("completed execution cancel must fail closed: %v", err)
	}
	if runtime.healthCalls != 1 || runtime.executeCalls != 1 || runtime.cancelCalls != 0 {
		t.Fatalf("unexpected delegation counts: %#v", runtime)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("release selection: %v", err)
	}
	if err := selected.Release(context.Background()); err != nil || runtime.closeCalls != 1 {
		t.Fatalf("release must close runtime exactly once: err=%v close=%d", err, runtime.closeCalls)
	}
	if releasedKey != testRouterProviderKey || releasedToken != testRouterLeaseToken {
		t.Fatalf("release lost lease ownership: key=%q token=%q", releasedKey, releasedToken)
	}
}

type ambiguousReleaseCapacityLimiter struct {
	mu           sync.Mutex
	committed    bool
	releaseCalls int
	renewCalls   int
}

func (*ambiguousReleaseCapacityLimiter) Acquire(
	context.Context,
	string,
	string,
	string,
	int,
	time.Duration,
) (int64, error) {
	return 0, errors.New("unexpected acquire")
}

func (l *ambiguousReleaseCapacityLimiter) Renew(
	context.Context,
	string,
	string,
	string,
	string,
	time.Duration,
) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.renewCalls++
	return time.Now().Add(time.Minute).UnixMilli(), nil
}

func (l *ambiguousReleaseCapacityLimiter) Release(
	_ context.Context,
	_ string,
	token string,
	fence string,
) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseCalls++
	if l.releaseCalls == 1 {
		l.committed = true
		return errors.New("redis release response lost after commit token=" + token + " fence=" + fence)
	}
	return nil
}

func (l *ambiguousReleaseCapacityLimiter) snapshot() (committed bool, releaseCalls, renewCalls int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.committed, l.releaseCalls, l.renewCalls
}

func TestProviderRouterReleaseAmbiguityFailsClosedUntilCleanupCompletes(t *testing.T) {
	limiter := &ambiguousReleaseCapacityLimiter{}
	lease, err := newCapacityLease(
		limiter,
		testRouterProviderKey,
		testRouterLeaseToken,
		testRouterLeaseFence,
		time.Minute,
		time.Now().Add(time.Minute).UnixMilli(),
	)
	if err != nil {
		t.Fatalf("create lease: %v", err)
	}
	runtime := &runtimeProviderStub{}
	selected := &SelectedProvider{
		ProviderDescriptor: ProviderDescriptor{
			ProviderKey: testRouterProviderKey,
			Scope:       domainsandbox.ScopeAgent,
		},
		runtime: runtime,
		lease:   lease,
	}

	releaseErr := selected.Release(context.Background())
	if releaseErr != domainsandbox.ErrUnavailable {
		t.Fatalf("ambiguous release returned unstable error: %v", releaseErr)
	}
	if strings.Contains(releaseErr.Error(), testRouterLeaseToken) || strings.Contains(releaseErr.Error(), testRouterLeaseFence) {
		t.Fatalf("ambiguous release leaked lease identity: %v", releaseErr)
	}
	if _, err := selected.Execute(context.Background(), infrasandbox.ExecuteRequest{}); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("execute after ambiguous release must fail closed: %v", err)
	}
	if err := selected.Renew(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("renew after ambiguous release must fail closed: %v", err)
	}
	if err := selected.Cancel(context.Background(), "execution-after-ambiguous-release"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("cancel after ambiguous release must fail closed: %v", err)
	}
	committed, releaseCalls, renewCalls := limiter.snapshot()
	if !committed || releaseCalls != 1 || renewCalls != 0 {
		t.Fatalf("unexpected limiter calls after ambiguous release: committed=%v release=%d renew=%d", committed, releaseCalls, renewCalls)
	}
	if runtime.executeCalls != 0 || runtime.cancelCalls != 0 {
		t.Fatalf("runtime called after ambiguous release: %#v", runtime)
	}
	if runtime.closeCalls != 1 {
		t.Fatalf("ambiguous cleanup must close runtime once, got %d", runtime.closeCalls)
	}

	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("idempotent release cleanup retry: %v", err)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("released selection must remain idempotent: %v", err)
	}
	_, releaseCalls, renewCalls = limiter.snapshot()
	if releaseCalls != 2 || renewCalls != 0 {
		t.Fatalf("unexpected limiter calls after cleanup: release=%d renew=%d", releaseCalls, renewCalls)
	}
}

func TestProviderRouterRequestOwnsCanonicalTokenAndReusesItOnRetry(t *testing.T) {
	now := time.Unix(2_000_001_050, 0).UTC()
	request, err := NewResolveProviderRequest(testRouterProviderKey, domainsandbox.ScopeAgent)
	if err != nil {
		t.Fatalf("create internal resolve request: %v", err)
	}
	if !validOpaqueLeaseToken(request.leaseToken) || !validOpaqueLeaseToken(request.leaseFence) {
		t.Fatalf("request generated non-canonical identity token=%q fence=%q", request.leaseToken, request.leaseFence)
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if strings.Contains(string(requestJSON), request.leaseToken) {
		t.Fatalf("internal lease token is externally serializable: %s", requestJSON)
	}
	if strings.Contains(string(requestJSON), request.leaseFence) {
		t.Fatalf("internal lease fence is externally serializable: %s", requestJSON)
	}
	field, exists := reflect.TypeOf(request).FieldByName("leaseToken")
	if !exists || field.PkgPath == "" {
		t.Fatal("resolve request lease token must be unexported")
	}
	fenceField, exists := reflect.TypeOf(request).FieldByName("leaseFence")
	if !exists || fenceField.PkgPath == "" {
		t.Fatal("resolve request lease fence must be unexported")
	}

	provider := healthyRouterProvider(now)
	var acquiredIdentities []string
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{
		acquire: func(_ context.Context, _ string, token, fence string, _ int, _ time.Duration) (int64, error) {
			acquiredIdentities = append(acquiredIdentities, token+":"+fence)
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
	})
	if _, err := router.Resolve(context.Background(), request); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("consumed request resolved twice: %v", err)
	}
	wantIdentity := request.leaseToken + ":" + request.leaseFence
	if len(acquiredIdentities) != 1 || acquiredIdentities[0] != wantIdentity {
		t.Fatalf("consumed request acquired more than once: %#v", acquiredIdentities)
	}

	if _, err := newResolveProviderRequest(testRouterProviderKey, domainsandbox.ScopeAgent, failingRequestEntropy{}); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("token entropy failure must fail closed, got %v", err)
	}
}

func TestProviderRouterRequestAllowsOnlyOneConcurrentSelection(t *testing.T) {
	now := time.Unix(2_000_001_075, 0).UTC()
	provider := healthyRouterProvider(now)
	buildStarted := make(chan struct{})
	finishBuild := make(chan struct{})
	var buildOnce sync.Once
	acquireCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			buildOnce.Do(func() { close(buildStarted) })
			<-finishBuild
			return &runtimeProviderStub{}, nil
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
	})
	request := testRouterRequest()
	firstResult := make(chan error, 1)
	go func() {
		_, err := router.Resolve(context.Background(), request)
		firstResult <- err
	}()
	<-buildStarted
	if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("concurrent request reuse was not rejected: %v", err)
	}
	close(finishBuild)
	if err := <-firstResult; err != nil {
		t.Fatalf("first concurrent resolve failed: %v", err)
	}
	if acquireCalls != 1 {
		t.Fatalf("concurrent request acquired %d slots", acquireCalls)
	}
	if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("successfully consumed request resolved again: %v", err)
	}
}

func TestProviderRouterAmbiguousAcquireRetryReusesRequestIdentity(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	server.SetTime(time.Unix(2_000_001_090, 0).UTC())
	client := redisimpl.NewWithAddrAndPassword(server.Addr(), "")
	calls := 0
	lossyClient := routerScriptClientFunc(func(
		ctx context.Context,
		script string,
		keys []string,
		args ...interface{},
	) cache.ScriptCmd {
		result, err := client.RunScript(ctx, script, keys, args...).Result()
		calls++
		if calls == 1 && err == nil {
			return &routerStaticScriptCmd{err: errors.New("acquire response lost after commit")}
		}
		return &routerStaticScriptCmd{result: result, err: err}
	})
	limiter, err := infrasandbox.NewRedisCapacityLimiter(lossyClient)
	if err != nil {
		t.Fatalf("create limiter: %v", err)
	}
	now := time.Unix(2_000_001_090, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Policy.MaxConcurrency = 1
	router, err := newProviderRouter(
		providerLookupFunc(func(context.Context, string) (*domainsandbox.Provider, error) {
			copy := *provider
			return &copy, nil
		}),
		&runtimeProviderFactoryFuncs{},
		limiter,
		2*time.Second,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	request := testRouterRequest()
	if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("lost acquire response must fail closed: %v", err)
	}
	selected, err := router.Resolve(context.Background(), request)
	if err != nil {
		t.Fatalf("retry same logical request: %v", err)
	}
	card, err := client.RunScript(
		context.Background(),
		`return redis.call('ZCARD', KEYS[1])`,
		[]string{"sandbox:capacity:" + testRouterProviderKey},
	).Result()
	if err != nil || card != int64(1) {
		t.Fatalf("ambiguous request retry duplicated lease: card=%#v err=%v", card, err)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("release recovered selection: %v", err)
	}
}

func TestProviderRouterRejectsIneligibleExactProviderBeforeCapacity(t *testing.T) {
	now := time.Unix(2_000_001_100, 0).UTC()
	tests := []struct {
		name       string
		mutate     func(*domainsandbox.Provider)
		requestKey string
		want       error
		wantCode   string
	}{
		{
			name: "disabled",
			mutate: func(provider *domainsandbox.Provider) {
				provider.Status = domainsandbox.ProviderStatusDisabled
			},
			want: domainsandbox.ErrProviderDisabled,
		},
		{
			name: "deleted",
			mutate: func(provider *domainsandbox.Provider) {
				deletedAt := now
				provider.DeletedAt = &deletedAt
			},
			want: domainsandbox.ErrProviderNotFound,
		},
		{
			name: "unhealthy",
			mutate: func(provider *domainsandbox.Provider) {
				provider.Health.Status = domainsandbox.HealthStatusUnhealthy
			},
			want: domainsandbox.ErrProviderUnhealthy,
		},
		{
			name: "stale health",
			mutate: func(provider *domainsandbox.Provider) {
				provider.Health.CheckedAt = now.Add(-DefaultProviderHealthMaxAge - time.Second)
			},
			want: domainsandbox.ErrProviderUnhealthy,
		},
		{
			name: "configured scope unsupported",
			mutate: func(provider *domainsandbox.Provider) {
				provider.Scopes = []domainsandbox.Scope{domainsandbox.ScopeAppDev}
			},
			want:     domainsandbox.ErrScopeUnsupported,
			wantCode: domainsandbox.ErrCodeScopeUnsupported,
		},
		{
			name: "health capability unsupported",
			mutate: func(provider *domainsandbox.Provider) {
				provider.Health.Capabilities = []domainsandbox.Scope{domainsandbox.ScopeMCPStdio}
			},
			want:     domainsandbox.ErrScopeUnsupported,
			wantCode: domainsandbox.ErrCodeScopeUnsupported,
		},
		{
			name: "invalid persisted policy",
			mutate: func(provider *domainsandbox.Provider) {
				provider.Policy.MaxConcurrency = 0
			},
			want:     domainsandbox.ErrConfigurationInvalid,
			wantCode: domainsandbox.ErrCodeConfigurationInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := healthyRouterProvider(now)
			test.mutate(provider)
			acquireCalls := 0
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{
				acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
					acquireCalls++
					return now.Add(2 * time.Second).UnixMilli(), nil
				},
			})
			_, err := router.Resolve(context.Background(), testRouterRequest())
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v want %v", err, test.want)
			}
			if acquireCalls != 0 {
				t.Fatalf("ineligible provider acquired capacity %d times", acquireCalls)
			}
			if test.wantCode != "" && domainsandbox.ErrorCodeOf(err) != test.wantCode {
				t.Fatalf("got code %q want %q", domainsandbox.ErrorCodeOf(err), test.wantCode)
			}
		})
	}
}

func TestProviderRouterDoesNotFallbackFromRequestedProvider(t *testing.T) {
	now := time.Unix(2_000_001_200, 0).UTC()
	requested := healthyRouterProvider(now)
	requested.Status = domainsandbox.ProviderStatusDisabled
	lookupCalls := 0
	lookup := providerLookupFunc(func(_ context.Context, key string) (*domainsandbox.Provider, error) {
		lookupCalls++
		if key != testRouterProviderKey {
			t.Fatalf("router attempted fallback key %q", key)
		}
		copy := *requested
		return &copy, nil
	})
	router, err := newProviderRouter(
		lookup,
		&runtimeProviderFactoryFuncs{},
		&capacityLimiterFuncs{},
		2*time.Second,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}

	_, err = router.Resolve(context.Background(), testRouterRequest())
	if !errors.Is(err, domainsandbox.ErrProviderDisabled) || lookupCalls != 1 {
		t.Fatalf("expected exact disabled provider without fallback, err=%v calls=%d", err, lookupCalls)
	}
}

func TestProviderRouterMapsLookupFailureAndCapacityFull(t *testing.T) {
	now := time.Unix(2_000_001_300, 0).UTC()
	router, err := newProviderRouter(
		providerLookupFunc(func(context.Context, string) (*domainsandbox.Provider, error) {
			return nil, errors.New("database unavailable")
		}),
		&runtimeProviderFactoryFuncs{},
		&capacityLimiterFuncs{},
		2*time.Second,
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}
	_, err = router.Resolve(context.Background(), testRouterRequest())
	if !errors.Is(err, domainsandbox.ErrUnavailable) || domainsandbox.ErrorCodeOf(err) != domainsandbox.ErrCodeUnavailable {
		t.Fatalf("lookup failure must be stable unavailable, got %v", err)
	}

	provider := healthyRouterProvider(now)
	buildCalls := 0
	router = newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			buildCalls++
			return &runtimeProviderStub{}, nil
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			return 0, domainsandbox.ErrCapacityExhausted
		},
	})
	_, err = router.Resolve(context.Background(), testRouterRequest())
	if !errors.Is(err, domainsandbox.ErrCapacityExhausted) || buildCalls != 0 {
		t.Fatalf("full provider built runtime before capacity check, err=%v buildCalls=%d", err, buildCalls)
	}
}

func TestProviderRouterValidatesAdapterPolicyBeforeCapacity(t *testing.T) {
	now := time.Unix(2_000_001_400, 0).UTC()
	provider := healthyRouterProvider(now)
	provider.Type = domainsandbox.ProviderTypeLocalDebug
	acquireCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		validate: func(_ context.Context, descriptor ProviderDescriptor) error {
			if descriptor.ProviderType != domainsandbox.ProviderTypeLocalDebug {
				t.Fatalf("unexpected descriptor %#v", descriptor)
			}
			return domainsandbox.ErrExecutionForbidden
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
	})

	_, err := router.Resolve(context.Background(), testRouterRequest())
	if !errors.Is(err, domainsandbox.ErrExecutionForbidden) || acquireCalls != 0 {
		t.Fatalf("adapter policy must deny before capacity, err=%v calls=%d", err, acquireCalls)
	}
}

func TestProviderRouterBuildsRuntimeAfterAcquire(t *testing.T) {
	now := time.Unix(2_000_001_500, 0).UTC()
	provider := healthyRouterProvider(now)
	acquireCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return nil, domainsandbox.ErrCredentialInvalid
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
	})

	_, err := router.Resolve(context.Background(), testRouterRequest())
	if !errors.Is(err, domainsandbox.ErrCredentialInvalid) || acquireCalls != 1 {
		t.Fatalf("runtime Build did not follow lease acquisition, err=%v calls=%d", err, acquireCalls)
	}
}

func TestProviderRouterPropagatesBuildCancellationAfterAcquire(t *testing.T) {
	now := time.Unix(2_000_001_600, 0).UTC()
	provider := healthyRouterProvider(now)
	ctx, cancel := context.WithCancel(context.Background())
	acquireCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(ctx context.Context, _ domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			cancel()
			return nil, ctx.Err()
		},
	}, &capacityLimiterFuncs{
		acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
			acquireCalls++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
	})

	_, err := router.Resolve(ctx, testRouterRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if acquireCalls != 1 {
		t.Fatalf("canceled Build acquisition calls = %d", acquireCalls)
	}
}

func TestProviderRouterLeaseReleaseOwnershipAndRenewal(t *testing.T) {
	now := time.Unix(2_000_001_700, 0).UTC()
	provider := healthyRouterProvider(now)
	tokens := []string{testRouterLeaseToken, "AgICAgICAgICAgICAgICAg"}
	fences := []string{testRouterLeaseFence, "AwMDAwMDAwMDAwMDAwMDAw"}
	acquireIndex := 0
	var released, renewed []string
	cleanupRetainedTrace := false
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{
		acquire: func(_ context.Context, _ string, token, fence string, _ int, _ time.Duration) (int64, error) {
			if token != tokens[acquireIndex] || fence != fences[acquireIndex] {
				t.Fatalf("acquire identity mismatch: token=%q fence=%q", token, fence)
			}
			acquireIndex++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
		renew: func(_ context.Context, providerKey, token, fence, renewalID string, duration time.Duration) (int64, error) {
			if providerKey != testRouterProviderKey || duration != 2*time.Second {
				t.Fatalf("unexpected renew key=%q duration=%s", providerKey, duration)
			}
			if !validOpaqueLeaseToken(renewalID) {
				t.Fatalf("renewal id is not canonical: %q", renewalID)
			}
			renewed = append(renewed, token+":"+fence)
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
		release: func(ctx context.Context, providerKey, token, fence string) error {
			if providerKey != testRouterProviderKey {
				t.Fatalf("unexpected release key %q", providerKey)
			}
			if ctx.Err() != nil {
				t.Fatalf("lease cleanup received canceled context: %v", ctx.Err())
			}
			cleanupRetainedTrace = cleanupRetainedTrace || ctx.Value(cleanupTraceKey{}) == "trace-value"
			released = append(released, token+":"+fence)
			return nil
		},
	})

	first, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("resolve first: %v", err)
	}
	secondRequest := testRouterRequest()
	secondRequest.leaseToken = tokens[1]
	secondRequest.leaseFence = fences[1]
	second, err := router.Resolve(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	releaseContext := context.WithValue(context.Background(), cleanupTraceKey{}, "trace-value")
	if err := first.Release(releaseContext); err != nil {
		t.Fatalf("release first: %v", err)
	}
	if err := first.Release(context.Background()); err != nil {
		t.Fatalf("release first twice: %v", err)
	}
	if err := second.Renew(context.Background()); err != nil {
		t.Fatalf("renew second: %v", err)
	}
	if err := second.Release(context.Background()); err != nil {
		t.Fatalf("release second: %v", err)
	}
	if len(released) != 2 || released[0] != tokens[0]+":"+fences[0] || released[1] != tokens[1]+":"+fences[1] {
		t.Fatalf("lease ownership mismatch: released %#v", released)
	}
	if len(renewed) != 1 || renewed[0] != tokens[1]+":"+fences[1] {
		t.Fatalf("renewed wrong token: %#v", renewed)
	}
	if !cleanupRetainedTrace {
		t.Fatal("lease cleanup did not preserve tracing values")
	}
	if err := second.Renew(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("released selection renewed lease: %v", err)
	}
}

func TestProviderRouterSelectedProviderEnforcesConcurrentLifecycle(t *testing.T) {
	now := time.Unix(2_000_001_725, 0).UTC()
	provider := healthyRouterProvider(now)
	runtime := newBlockingRuntimeProvider()
	releaseCalls := 0
	renewCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, nil
		},
	}, &capacityLimiterFuncs{
		renew: func(context.Context, string, string, string, string, time.Duration) (int64, error) {
			renewCalls++
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
		release: func(context.Context, string, string, string) error {
			releaseCalls++
			return nil
		},
	})
	selected, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	executeResult := make(chan error, 1)
	go func() {
		_, err := selected.Execute(context.Background(), infrasandbox.ExecuteRequest{})
		executeResult <- err
	}()
	<-runtime.started
	if _, err := selected.Execute(context.Background(), infrasandbox.ExecuteRequest{}); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("concurrent execute was not rejected: %v", err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("running selection released capacity: %v", err)
	}
	if releaseCalls != 0 {
		t.Fatalf("running release reached Redis %d times", releaseCalls)
	}
	if runtime.closeCalls != 0 {
		t.Fatalf("running selection closed runtime %d times", runtime.closeCalls)
	}
	if err := selected.Cancel(context.Background(), "execution-running"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("cancel before provider records execution identity: %v", err)
	}
	if err := selected.Cancel(context.Background(), "different-execution"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("cancel accepted caller-controlled execution identity: %v", err)
	}
	close(runtime.finish)
	if err := <-executeResult; err != nil {
		t.Fatalf("running execute failed: %v", err)
	}
	executeCalls, cancelIDs := runtime.counts()
	if executeCalls != 1 || len(cancelIDs) != 0 {
		t.Fatalf("runtime lifecycle calls execute=%d cancel=%#v", executeCalls, cancelIDs)
	}
	if _, err := selected.Execute(context.Background(), infrasandbox.ExecuteRequest{}); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("completed selection executed twice: %v", err)
	}
	if err := selected.Renew(context.Background()); err != nil || renewCalls != 1 {
		t.Fatalf("completed selection could not renew: err=%v calls=%d", err, renewCalls)
	}
	if err := selected.Release(context.Background()); err != nil || releaseCalls != 1 {
		t.Fatalf("completed selection release failed: err=%v calls=%d", err, releaseCalls)
	}
	if runtime.closeCalls != 1 {
		t.Fatalf("completed release close calls = %d", runtime.closeCalls)
	}
	if err := selected.Renew(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("released selection renewed: %v", err)
	}
	if _, err := selected.Execute(context.Background(), infrasandbox.ExecuteRequest{}); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("released selection executed: %v", err)
	}
	if err := selected.Cancel(context.Background(), "execution-running"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("released selection canceled runtime: %v", err)
	}
}

func TestProviderRouterClosesRuntimeOnEveryPostBuildFailure(t *testing.T) {
	now := time.Unix(2_000_001_900, 0).UTC()
	provider := healthyRouterProvider(now)

	for _, test := range []struct {
		name        string
		buildErr    error
		acquireErr  error
		expiresAt   int64
		wantClose   int
		wantRelease int
	}{
		{
			name: "build returns runtime and error", buildErr: domainsandbox.ErrCredentialInvalid,
			expiresAt: now.Add(time.Minute).UnixMilli(), wantClose: 1, wantRelease: 1,
		},
		{name: "capacity acquire fails", acquireErr: errors.New("redis token must-not-leak")},
		{name: "lease construction fails after acquire", expiresAt: 0, wantRelease: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := &runtimeProviderStub{}
			releaseCalls := 0
			expiresAt := test.expiresAt
			if expiresAt == 0 && test.wantRelease == 0 {
				expiresAt = now.Add(time.Minute).UnixMilli()
			}
			router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
				build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
					return runtime, test.buildErr
				},
			}, &capacityLimiterFuncs{
				acquire: func(context.Context, string, string, string, int, time.Duration) (int64, error) {
					return expiresAt, test.acquireErr
				},
				release: func(context.Context, string, string, string) error {
					releaseCalls++
					return nil
				},
			})

			_, err := router.Resolve(context.Background(), testRouterRequest())
			if err == nil {
				t.Fatal("post-build failure unexpectedly succeeded")
			}
			if runtime.closeCalls != test.wantClose || releaseCalls != test.wantRelease {
				t.Fatalf(
					"cleanup close/release = %d/%d, want %d/%d",
					runtime.closeCalls, releaseCalls, test.wantClose, test.wantRelease,
				)
			}
			if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "must-not-leak") {
				t.Fatalf("cleanup error leaked internals: %v", err)
			}
		})
	}
}

func TestProviderRouterReleaseClosesBeforeCapacityAndSanitizesCloseError(t *testing.T) {
	now := time.Unix(2_000_001_925, 0).UTC()
	provider := healthyRouterProvider(now)
	runtime := &runtimeProviderStub{closeErr: errors.New("close raw endpoint token fence must-not-leak")}
	releaseCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{
		build: func(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
			return runtime, nil
		},
	}, &capacityLimiterFuncs{release: func(context.Context, string, string, string) error {
		releaseCalls++
		if runtime.closeCalls == 0 {
			t.Fatal("capacity released before runtime Close")
		}
		return nil
	}})
	selected, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	err = selected.Release(context.Background())
	if !errors.Is(err, domainsandbox.ErrUnavailable) || strings.Contains(err.Error(), "endpoint") || strings.Contains(err.Error(), "token") {
		t.Fatalf("close error was not sanitized: %v", err)
	}
	if releaseCalls != 0 || runtime.closeCalls != 1 {
		t.Fatalf("release/close calls = %d/%d", releaseCalls, runtime.closeCalls)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) || releaseCalls != 0 || runtime.closeCalls != 2 {
		t.Fatalf("repeated release did not retry Close: err=%v release=%d close=%d", err, releaseCalls, runtime.closeCalls)
	}
}

func TestProviderRouterRenewRetryReusesOperationIdentity(t *testing.T) {
	now := time.Unix(2_000_001_740, 0).UTC()
	provider := healthyRouterProvider(now)
	var renewalIDs []string
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{
		renew: func(_ context.Context, _, _, _, renewalID string, _ time.Duration) (int64, error) {
			renewalIDs = append(renewalIDs, renewalID)
			if len(renewalIDs) == 1 {
				return 0, errors.New("renew response lost")
			}
			return now.Add(2 * time.Second).UnixMilli(), nil
		},
	})
	selected, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := selected.Renew(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("lost renew response must fail closed: %v", err)
	}
	if err := selected.Renew(context.Background()); err != nil {
		t.Fatalf("retry renew: %v", err)
	}
	if len(renewalIDs) != 2 || renewalIDs[0] != renewalIDs[1] {
		t.Fatalf("renew retry changed operation identity: %#v", renewalIDs)
	}
	if err := selected.Renew(context.Background()); err != nil {
		t.Fatalf("new renew operation: %v", err)
	}
	if len(renewalIDs) != 3 || renewalIDs[2] == renewalIDs[1] {
		t.Fatalf("new renew operation reused completed identity: %#v", renewalIDs)
	}
}

func TestProviderRouterReleaseFailsClosedWhenCapacityReleaseFails(t *testing.T) {
	now := time.Unix(2_000_001_750, 0).UTC()
	provider := healthyRouterProvider(now)
	releaseCalls := 0
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{
		release: func(ctx context.Context, _, _, _ string) error {
			releaseCalls++
			if ctx.Err() != nil {
				t.Fatalf("cleanup context was canceled: %v", ctx.Err())
			}
			if releaseCalls == 1 {
				return errors.New("redis unavailable")
			}
			return nil
		},
	})
	selected, err := router.Resolve(context.Background(), testRouterRequest())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := selected.Release(context.Background()); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Redis release failure must be stable unavailable, got %v", err)
	}
	if releaseCalls != 1 {
		t.Fatalf("canceled release did not attempt Redis cleanup: %d", releaseCalls)
	}
	if err := selected.Release(context.Background()); err != nil {
		t.Fatalf("release retry after Redis recovery: %v", err)
	}
	if releaseCalls != 2 {
		t.Fatalf("release failure marked selection permanently released: %d", releaseCalls)
	}
}

func TestProviderRouterRejectsInvalidRequestAndConfiguration(t *testing.T) {
	now := time.Unix(2_000_001_800, 0).UTC()
	provider := healthyRouterProvider(now)
	router := newRouterForTest(t, now, provider, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{})

	invalidRequests := []ResolveProviderRequest{
		{ProviderKey: "", Scope: domainsandbox.ScopeAgent},
		{ProviderKey: testRouterProviderKey, Scope: "unknown"},
		{ProviderKey: testRouterProviderKey, Scope: domainsandbox.ScopeAgent},
	}
	for _, request := range invalidRequests {
		if _, err := router.Resolve(context.Background(), request); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("invalid request %#v returned %v", request, err)
		}
	}
	if _, err := newProviderRouter(nil, &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{}, time.Second, time.Now); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("nil lookup must reject configuration: %v", err)
	}
	if _, err := newProviderRouter(providerLookupFunc(func(context.Context, string) (*domainsandbox.Provider, error) {
		return provider, nil
	}), &runtimeProviderFactoryFuncs{}, &capacityLimiterFuncs{}, 0, time.Now); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("zero lease duration must reject configuration: %v", err)
	}
}
