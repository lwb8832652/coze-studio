// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestRuntimeExecutesAdvertisedScopesWithBoundInternalAndBusinessIDs(t *testing.T) {
	clock := newSchedulerClock(time.Now().UTC())
	store := newRuntimeStore(clock.now)
	driver := &lifecycleDriverFake{stopped: true}
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer := newRuntimeConfigurationSigner(t)
	runtime, err := NewRuntime(validRuntimeConfig(), RuntimeDependencies{
		Store: store, Driver: driver, Resources: fixedMemorySampler(4096),
		InitialSettings: settings, ConfigurationSigner: signer, Now: clock.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		scope      domainsandbox.Scope
		entrypoint string
	}{
		{domainsandbox.ScopeAgent, "agent/code/run"},
		{domainsandbox.ScopePlugin, infrasandbox.PluginCodeRunnerEntrypoint},
	} {
		t.Run(string(test.scope), func(t *testing.T) {
			command := schedulerCommand(clock.now(), "runtime-e2e-"+string(test.scope), 101, 201, test.scope)
			command.Entrypoint = test.entrypoint
			command.Policy.TimeoutSeconds, command.Policy.MaxOutputBytes = 30, 4096
			command.RawBody = []byte(`{"canonical":true}`)
			command.Identity.ExecutionID = "signed-business-" + string(test.scope)
			accepted, err := runtime.scheduler.Accept(context.Background(), command)
			if err != nil {
				t.Fatal(err)
			}
			dispatched, err := runtime.scheduler.DispatchNext(context.Background())
			if err != nil || !dispatched {
				t.Fatalf("DispatchNext() = %t, %v", dispatched, err)
			}
			result := waitRuntimeExecution(t, store, accepted.ExecutionID)
			if result.Status != infrasandbox.ExecutionStatusSucceeded || result.ExecutionID != accepted.ExecutionID || result.ExitCode == nil || *result.ExitCode != 0 {
				t.Fatalf("result = %#v", result)
			}
		})
	}
	if driver.runCalls != 2 || driver.stageCalls != 2 {
		t.Fatalf("runtime calls stage/run = %d/%d", driver.stageCalls, driver.runCalls)
	}
}

func newRuntimeConfigurationSigner(t *testing.T) *infrasandbox.SchedulerConfigSigner {
	t.Helper()
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func waitRuntimeExecution(t *testing.T, store *runtimeStore, executionID string) infrasandbox.ExecuteResult {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		result, err := store.Status(context.Background(), executionID)
		if err == nil && (result.Status == infrasandbox.ExecutionStatusSucceeded || result.Status == infrasandbox.ExecutionStatusFailed || result.Status == infrasandbox.ExecutionStatusTimedOut || result.Status == infrasandbox.ExecutionStatusCanceled) {
			return result
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("runner did not persist a terminal execution result")
	return infrasandbox.ExecuteResult{}
}
