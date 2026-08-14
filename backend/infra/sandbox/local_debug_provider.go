// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"os"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	AppEnvName                      = "APP_ENV"
	AppDevHostRuntimeEnabledEnvName = "APP_DEV_HOST_RUNTIME_ENABLED"
	HostShellSessionEnabledEnvName  = "SANDBOX_HOST_SHELL_SESSION_ENABLED"
	HostShellGatewayAddrEnvName     = "SANDBOX_HOST_SHELL_GATEWAY_ADDR"
	requiredLocalDebugAppEnv        = "debug"
	requiredLocalDebugFlagValue     = "true"
	requiredHostShellIPv4Gateway    = "127.0.0.1:8099"
	requiredHostShellIPv6Gateway    = "[::1]:8099"
)

type LocalDebugProvider struct {
	delegate LocalExecutionDelegate
	getenv   func(string) string
}

// LocalDebugHealthProvider keeps the legacy one-shot and Host Shell gates
// independent while projecting the single provider health snapshot required
// by the control plane. It never executes workloads.
type LocalDebugHealthProvider struct {
	delegate LocalExecutionDelegate
	scopes   []domainsandbox.Scope
	getenv   func(string) string
}

func newLocalDebugHealthProvider(
	delegate LocalExecutionDelegate,
	scopes []domainsandbox.Scope,
	getenv func(string) string,
) (*LocalDebugHealthProvider, error) {
	if getenv == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalizedScopes, err := domainsandbox.NormalizeScopes(scopes)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	legacyEnabled := localDebugAllowed(getenv)
	hostEnabled := HostShellSessionAllowed(getenv)
	if !legacyEnabled && !hostEnabled {
		return nil, domainsandbox.ErrExecutionForbidden
	}
	if legacyEnabled && !hostEnabled && delegate == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &LocalDebugHealthProvider{
		delegate: delegate,
		scopes:   append([]domainsandbox.Scope(nil), normalizedScopes...),
		getenv:   getenv,
	}, nil
}

func NewLocalDebugHealthProvider(
	delegate LocalExecutionDelegate,
	scopes []domainsandbox.Scope,
) (*LocalDebugHealthProvider, error) {
	return newLocalDebugHealthProvider(delegate, scopes, os.Getenv)
}

func (p *LocalDebugHealthProvider) Health(ctx context.Context) (HealthResult, error) {
	if p == nil || p.getenv == nil || (!localDebugAllowed(p.getenv) && !HostShellSessionAllowed(p.getenv)) {
		return HealthResult{}, domainsandbox.ErrExecutionForbidden
	}
	if ctx == nil {
		return HealthResult{}, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return HealthResult{}, err
	}

	result := HealthResult{
		ProtocolVersion: HealthProtocolV1,
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    append([]domainsandbox.Scope(nil), p.scopes...),
	}
	if localDebugAllowed(p.getenv) && p.delegate != nil {
		delegateResult, err := p.delegate.Health(ctx)
		if err != nil {
			return HealthResult{}, mapProviderError(err)
		}
		result, err = normalizeHealthResult(delegateResult)
		if err != nil {
			return HealthResult{}, domainsandbox.ErrProviderUnhealthy
		}
	}
	if HostShellSessionAllowed(p.getenv) {
		result.Capabilities = append([]domainsandbox.Scope(nil), p.scopes...)
		result.Features = appendHostShellSessionFeatures(result.Features)
	}
	normalized, err := normalizeHealthResult(result)
	if err != nil {
		return HealthResult{}, domainsandbox.ErrProviderUnhealthy
	}
	return normalized, nil
}

func (p *LocalDebugHealthProvider) CloseContext(ctx context.Context) error {
	if ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	return ctx.Err()
}

func appendHostShellSessionFeatures(features []domainsandbox.ProviderFeature) []domainsandbox.ProviderFeature {
	result := append([]domainsandbox.ProviderFeature(nil), features...)
	for _, required := range []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureSandboxSessionV1,
		domainsandbox.ProviderFeatureSignedSessionContextV2,
	} {
		found := false
		for _, current := range result {
			if current == required {
				found = true
				break
			}
		}
		if !found {
			result = append(result, required)
		}
	}
	return result
}

func NewLocalDebugProvider(delegate LocalExecutionDelegate) (*LocalDebugProvider, error) {
	return newLocalDebugProvider(delegate, os.Getenv)
}

func newLocalDebugProvider(delegate LocalExecutionDelegate, getenv func(string) string) (*LocalDebugProvider, error) {
	if !localDebugAllowed(getenv) {
		return nil, domainsandbox.ErrExecutionForbidden
	}
	if delegate == nil || getenv == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &LocalDebugProvider{delegate: delegate, getenv: getenv}, nil
}

func (p *LocalDebugProvider) Health(ctx context.Context) (HealthResult, error) {
	if p == nil || !localDebugAllowed(p.getenv) {
		return HealthResult{}, domainsandbox.ErrExecutionForbidden
	}
	if ctx == nil {
		return HealthResult{}, domainsandbox.ErrInvalidInput
	}
	result, err := p.delegate.Health(ctx)
	if err != nil {
		return HealthResult{}, mapProviderError(err)
	}
	normalized, err := normalizeHealthResult(result)
	if err != nil {
		return HealthResult{}, domainsandbox.ErrProviderUnhealthy
	}
	return normalized, nil
}

func (p *LocalDebugProvider) Execute(ctx context.Context, input ExecuteRequest) (ExecuteResult, error) {
	if p == nil || !localDebugAllowed(p.getenv) {
		return ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	if ctx == nil {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	normalized, err := normalizeExecuteRequest(input, timeNow())
	if err != nil {
		return ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	executionContext, cancel := context.WithDeadline(ctx, normalized.Deadline)
	defer cancel()
	result, err := p.delegate.Execute(executionContext, cloneExecuteRequest(normalized))
	if err != nil {
		return ExecuteResult{}, mapProviderError(err)
	}
	result, err = normalizeExecuteResult(result, normalized.Policy.MaxOutputBytes)
	if err != nil {
		return ExecuteResult{}, domainsandbox.ErrProviderUnhealthy
	}
	return result, nil
}

func (p *LocalDebugProvider) Cancel(ctx context.Context, executionID string) error {
	if p == nil || !localDebugAllowed(p.getenv) {
		return domainsandbox.ErrExecutionForbidden
	}
	if ctx == nil || !validExecutionID(executionID) {
		return domainsandbox.ErrInvalidInput
	}
	return mapProviderError(p.delegate.Cancel(ctx, executionID))
}

func (p *LocalDebugProvider) CloseContext(ctx context.Context) error {
	if ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func localDebugAllowed(getenv func(string) string) bool {
	return getenv != nil && getenv(AppEnvName) == requiredLocalDebugAppEnv &&
		getenv(AppDevHostRuntimeEnabledEnvName) == requiredLocalDebugFlagValue
}

// HostShellSessionAllowed is the single exact gate used by execution,
// provider selection, health and runtime projection. It deliberately ignores
// APP_DEV_HOST_RUNTIME_ENABLED so enabling AppDev cannot expose Host Shell.
func HostShellSessionAllowed(getenv func(string) string) bool {
	if getenv == nil || getenv(AppEnvName) != requiredLocalDebugAppEnv ||
		getenv(HostShellSessionEnabledEnvName) != requiredLocalDebugFlagValue {
		return false
	}
	switch getenv(HostShellGatewayAddrEnvName) {
	case requiredHostShellIPv4Gateway, requiredHostShellIPv6Gateway:
		return true
	default:
		return false
	}
}
