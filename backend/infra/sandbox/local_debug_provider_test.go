// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const appDevHostRuntimeEnabledEnvName = "APP_DEV_HOST_RUNTIME_ENABLED"

func TestLocalDebugProviderRequiresBothExactGatesAtConstruction(t *testing.T) {
	t.Parallel()

	for _, environment := range []map[string]string{
		{},
		{AppEnvName: "debug"},
		{appDevHostRuntimeEnabledEnvName: "true"},
		{AppEnvName: "DEBUG", appDevHostRuntimeEnabledEnvName: "true"},
		{AppEnvName: "debug", appDevHostRuntimeEnabledEnvName: "TRUE"},
		{AppEnvName: "debug", appDevHostRuntimeEnabledEnvName: "1"},
		{AppEnvName: " debug", appDevHostRuntimeEnabledEnvName: "true"},
		{AppEnvName: "debug ", appDevHostRuntimeEnabledEnvName: "true"},
		{AppEnvName: "debug", appDevHostRuntimeEnabledEnvName: " true"},
		{AppEnvName: "debug", appDevHostRuntimeEnabledEnvName: "true "},
		{"SANDBOX_LOCAL_DEBUG_ENABLED": "true"},
		{AppEnvName: "debug", "SANDBOX_LOCAL_DEBUG_ENABLED": "true"},
	} {
		if _, err := newLocalDebugProvider(&fakeLocalDelegate{}, mapGetenv(environment)); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
			t.Fatalf("environment %#v must be forbidden, got %v", environment, err)
		}
	}
	if _, err := newLocalDebugProvider(&fakeLocalDelegate{}, mapGetenv(validLocalEnvironment())); err != nil {
		t.Fatalf("valid debug environment: %v", err)
	}
}

func TestHostShellSessionGateRequiresThreeExactIndependentValues(t *testing.T) {
	t.Parallel()

	for _, environment := range []map[string]string{
		{},
		{AppEnvName: "debug"},
		{AppEnvName: "debug", HostShellSessionEnabledEnvName: "true"},
		{AppEnvName: "debug", HostShellGatewayAddrEnvName: "127.0.0.1:8099"},
		{AppEnvName: "DEBUG", HostShellSessionEnabledEnvName: "true", HostShellGatewayAddrEnvName: "127.0.0.1:8099"},
		{AppEnvName: "debug", HostShellSessionEnabledEnvName: "TRUE", HostShellGatewayAddrEnvName: "127.0.0.1:8099"},
		{AppEnvName: "debug", HostShellSessionEnabledEnvName: "true", HostShellGatewayAddrEnvName: "localhost:8099"},
		{AppEnvName: "debug", HostShellSessionEnabledEnvName: "true", HostShellGatewayAddrEnvName: "0.0.0.0:8099"},
		{AppEnvName: "debug", HostShellSessionEnabledEnvName: "true", HostShellGatewayAddrEnvName: "127.0.0.1:8100"},
		{AppEnvName: "debug", HostShellSessionEnabledEnvName: "true", HostShellGatewayAddrEnvName: " 127.0.0.1:8099"},
		{AppEnvName: "debug", AppDevHostRuntimeEnabledEnvName: "true"},
	} {
		if HostShellSessionAllowed(mapGetenv(environment)) {
			t.Fatalf("environment %#v must not enable Host Shell", environment)
		}
	}

	for _, gateway := range []string{"127.0.0.1:8099", "[::1]:8099"} {
		environment := map[string]string{
			AppEnvName:                      "debug",
			HostShellSessionEnabledEnvName:  "true",
			HostShellGatewayAddrEnvName:     gateway,
			AppDevHostRuntimeEnabledEnvName: "false",
		}
		if !HostShellSessionAllowed(mapGetenv(environment)) {
			t.Fatalf("exact Host Shell environment with gateway %q was rejected", gateway)
		}
	}
}

func TestLocalDebugHealthProviderAdvertisesHostSessionWithoutLegacyOneShotGate(t *testing.T) {
	t.Parallel()

	environment := map[string]string{
		AppEnvName:                      "debug",
		HostShellSessionEnabledEnvName:  "true",
		HostShellGatewayAddrEnvName:     "127.0.0.1:8099",
		AppDevHostRuntimeEnabledEnvName: "false",
	}
	provider, err := newLocalDebugHealthProvider(
		nil,
		[]domainsandbox.Scope{domainsandbox.ScopeAgent},
		mapGetenv(environment),
	)
	if err != nil {
		t.Fatalf("newLocalDebugHealthProvider() error = %v", err)
	}
	result, err := provider.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if result.Status != domainsandbox.HealthStatusHealthy ||
		!reflect.DeepEqual(result.Capabilities, []domainsandbox.Scope{domainsandbox.ScopeAgent}) ||
		!reflect.DeepEqual(result.Features, []domainsandbox.ProviderFeature{
			domainsandbox.ProviderFeatureSandboxSessionV1,
			domainsandbox.ProviderFeatureSignedSessionContextV2,
		}) {
		t.Fatalf("Host health = %#v", result)
	}
	if err := provider.CloseContext(context.Background()); err != nil {
		t.Fatalf("CloseContext() error = %v", err)
	}

	environment[HostShellSessionEnabledEnvName] = "false"
	if _, err := provider.Health(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("Health() after hot gate close error = %v", err)
	}
}

func TestLocalDebugHealthProviderDoesNotFakeLegacyHealthWithoutDelegate(t *testing.T) {
	t.Parallel()
	if _, err := newLocalDebugHealthProvider(
		nil,
		[]domainsandbox.Scope{domainsandbox.ScopeAgent},
		mapGetenv(validLocalEnvironment()),
	); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("legacy-only health without delegate error = %v", err)
	}
}

func TestLocalDebugProviderCloseIsIdempotentNoop(t *testing.T) {
	provider, err := newLocalDebugProvider(&fakeLocalDelegate{}, mapGetenv(validLocalEnvironment()))
	if err != nil {
		t.Fatalf("new local provider: %v", err)
	}
	if err := provider.CloseContext(context.Background()); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := provider.CloseContext(context.Background()); err != nil {
		t.Fatalf("close twice: %v", err)
	}
}

func TestLocalDebugProviderRechecksGatesBeforeEveryOperation(t *testing.T) {
	t.Parallel()

	environment := validLocalEnvironment()
	delegate := &fakeLocalDelegate{}
	provider, err := newLocalDebugProvider(delegate, mapGetenv(environment))
	if err != nil {
		t.Fatalf("new local provider: %v", err)
	}
	environment[appDevHostRuntimeEnabledEnvName] = "false"
	if _, err := provider.Health(context.Background()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("health gate error = %v", err)
	}
	if _, err := provider.Execute(context.Background(), validExecuteRequest()); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("execute gate error = %v", err)
	}
	if err := provider.Cancel(context.Background(), "exec_123"); !errors.Is(err, domainsandbox.ErrExecutionForbidden) {
		t.Fatalf("cancel gate error = %v", err)
	}
	if delegate.healthCalls != 0 || delegate.executeCalls != 0 || delegate.cancelCalls != 0 {
		t.Fatalf("forbidden operations reached delegate: %#v", delegate)
	}
}

func TestLocalDebugProviderCopiesRequestResultAndDelegatesCancellation(t *testing.T) {
	t.Parallel()

	delegateResult := ExecuteResult{
		ExecutionID: "exec_123",
		Status:      ExecutionStatusSucceeded,
		ExitCode:    intPointer(0),
		Stdout:      "ok",
		Artifacts: []ArtifactSummary{{
			ID: "artifact_1", Name: "result.json", MediaType: "application/json", Size: 10,
		}},
	}
	delegate := &fakeLocalDelegate{
		healthResult: HealthResult{
			ProtocolVersion: HealthProtocolV1,
			Status:          domainsandbox.HealthStatusHealthy,
			Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAgent},
		},
		executeResult: delegateResult,
		mutateRequest: true,
	}
	provider, err := newLocalDebugProvider(delegate, mapGetenv(validLocalEnvironment()))
	if err != nil {
		t.Fatalf("new local provider: %v", err)
	}
	request := validExecuteRequest()
	result, err := provider.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if request.Args[0] != "--safe" || request.Env["MODE"] != "test" || request.Files[0].Path != "inputs/request.json" || request.Policy.NetworkAllowlist[0] != "api.example.test" {
		t.Fatalf("delegate mutated caller request: %#v", request)
	}
	delegate.executeResult.Artifacts[0].Name = "mutated"
	if result.Artifacts[0].Name != "result.json" {
		t.Fatalf("result was not defensively copied: %#v", result)
	}
	health, err := provider.Health(context.Background())
	if err != nil || len(health.Capabilities) != 1 {
		t.Fatalf("health = %#v, %v", health, err)
	}
	if err := provider.Cancel(context.Background(), "exec_123"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if delegate.cancelID != "exec_123" || delegate.cancelCalls != 1 {
		t.Fatalf("cancel delegate state = %#v", delegate)
	}
}

func TestLocalDebugProviderMapsTimeoutAndRejectsUnboundedOutput(t *testing.T) {
	t.Parallel()

	timeoutDelegate := &fakeLocalDelegate{executeFunc: func(ctx context.Context, _ ExecuteRequest) (ExecuteResult, error) {
		<-ctx.Done()
		return ExecuteResult{}, errors.New("delegate timeout at /host/secret")
	}}
	provider, err := newLocalDebugProvider(timeoutDelegate, mapGetenv(validLocalEnvironment()))
	if err != nil {
		t.Fatalf("new timeout provider: %v", err)
	}
	request := validExecuteRequest()
	request.Deadline = time.Now().Add(20 * time.Millisecond)
	_, err = provider.Execute(context.Background(), request)
	if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) || strings.Contains(err.Error(), "/host/secret") {
		t.Fatalf("timeout error was not safely mapped: %v", err)
	}

	oversizedDelegate := &fakeLocalDelegate{executeResult: ExecuteResult{
		ExecutionID: "exec_123",
		Status:      ExecutionStatusSucceeded,
		ExitCode:    intPointer(0),
		Stdout:      strings.Repeat("x", MaxOutputStreamBytes+1),
	}}
	provider, err = newLocalDebugProvider(oversizedDelegate, mapGetenv(validLocalEnvironment()))
	if err != nil {
		t.Fatalf("new output provider: %v", err)
	}
	_, err = provider.Execute(context.Background(), validExecuteRequest())
	if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
		t.Fatalf("oversized output error = %v", err)
	}
}

type fakeLocalDelegate struct {
	healthResult  HealthResult
	healthErr     error
	executeResult ExecuteResult
	executeErr    error
	executeFunc   func(context.Context, ExecuteRequest) (ExecuteResult, error)
	cancelErr     error
	mutateRequest bool
	healthCalls   int
	executeCalls  int
	cancelCalls   int
	cancelID      string
}

func (d *fakeLocalDelegate) Health(context.Context) (HealthResult, error) {
	d.healthCalls++
	return d.healthResult, d.healthErr
}

func (d *fakeLocalDelegate) Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error) {
	d.executeCalls++
	if d.mutateRequest {
		request.Args[0] = "mutated"
		request.Env["MODE"] = "mutated"
		request.Files[0].Path = "mutated"
		request.Policy.NetworkAllowlist[0] = "mutated"
	}
	if d.executeFunc != nil {
		return d.executeFunc(ctx, request)
	}
	return d.executeResult, d.executeErr
}

func (d *fakeLocalDelegate) Cancel(_ context.Context, executionID string) error {
	d.cancelCalls++
	d.cancelID = executionID
	return d.cancelErr
}

func validLocalEnvironment() map[string]string {
	return map[string]string{AppEnvName: "debug", appDevHostRuntimeEnabledEnvName: "true"}
}

func mapGetenv(environment map[string]string) func(string) string {
	return func(key string) string { return environment[key] }
}

func intPointer(value int) *int {
	return &value
}
