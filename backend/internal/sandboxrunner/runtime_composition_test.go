// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestNewRuntimeComposesSignedConfigurationAndRunnerProtocol(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newRuntimeStore(clock.now)
	driver := &lifecycleDriverFake{stopped: true}
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	runtime, err := NewRuntime(validRuntimeConfig(), RuntimeDependencies{Store: store, Driver: driver, Resources: fixedMemorySampler(4096), InitialSettings: settings, ConfigurationSigner: signer, Now: clock.now})
	require.NoError(t, err)
	require.NotNil(t, runtime.Handler())
	next := settings
	next.Version = 2
	next.MaxOutstanding = 16
	envelope, err := signer.Sign(next)
	require.NoError(t, err)
	require.NoError(t, runtime.configStore.ApplyConfiguration(context.Background(), envelope))
	require.Equal(t, uint64(2), runtime.scheduler.settings.Version)
	require.Equal(t, uint64(2), runtime.lifecycle.settings.Version)
}

func TestNewRuntimeRejectsMissingExecutionDependencies(t *testing.T) {
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	_, err = NewRuntime(validRuntimeConfig(), RuntimeDependencies{InitialSettings: settings, ConfigurationSigner: signer})
	require.ErrorIs(t, err, ErrConfiguration)
}

func TestRuntimeLifecycleManagerCancelsRunningContainerBeforeReleasingSchedulerCapacity(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newRuntimeStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	require.NoError(t, err)
	accepted, err := scheduler.Accept(context.Background(), schedulerCommand(clock.now(), "runtime-cancel", 101, 201, domainsandbox.ScopePlugin))
	require.NoError(t, err)
	dispatched, err := scheduler.DispatchNext(context.Background())
	require.NoError(t, err)
	require.True(t, dispatched)
	lifecycle, driver := newLifecycleFixture(t, clock.now())
	request := lifecycleRequest(domainsandbox.ScopePlugin, accepted.ExecutionID, 101, 201, "project", "session")
	request.Identity.ExecutionID = accepted.ExecutionID
	_, err = lifecycle.Acquire(context.Background(), request)
	require.NoError(t, err)
	manager := runtimeLifecycleManager{store: store, scheduler: scheduler, lifecycle: lifecycle}
	require.NoError(t, manager.Cancel(context.Background(), accepted.ExecutionID))
	require.Equal(t, 1, driver.terminateCalls)
	stored, err := store.Get(context.Background(), accepted.ExecutionID)
	require.NoError(t, err)
	require.Equal(t, infrasandbox.ExecutionStatusCanceled, stored.State)
	require.Zero(t, scheduler.usedWeight)
	require.Empty(t, scheduler.running)
}

func validRuntimeConfig() Config {
	keyring, _ := sandboxidentity.NewKeyring("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	return Config{DeploymentID: "runner-dev-1", AuthToken: "runner-auth-token-0123456789", ContextVerifyKeys: keyring, ActiveQueueKeyID: "key-1", ExecutionImage: "registry.example/coze-sandbox-runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
}

type runtimeStore struct{ *schedulerStore }

func newRuntimeStore(now func() time.Time) *runtimeStore {
	return &runtimeStore{schedulerStore: newSchedulerStore(now)}
}

func (store *runtimeStore) Status(ctx context.Context, executionID string) (infrasandbox.ExecuteResult, error) {
	record, err := store.Get(ctx, executionID)
	return record.Result, err
}
func (store *runtimeStore) Lookup(context.Context, infrasandbox.ExecutionLookupRequest) (infrasandbox.ExecutionLookupResult, error) {
	return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupNotFound}, nil
}
func (store *runtimeStore) QueueStatus(context.Context, string) (infrasandbox.QueueStatus, error) {
	return infrasandbox.QueueStatus{}, nil
}
func (store *runtimeStore) KeepAlive(context.Context, string) error { return nil }
func (store *runtimeStore) Cancel(context.Context, string) error    { return nil }

var _ sandboxruntime.Driver = (*lifecycleDriverFake)(nil)
