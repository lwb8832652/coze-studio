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
	requiredLocalDebugAppEnv        = "debug"
	requiredLocalDebugFlagValue     = "true"
)

type LocalDebugProvider struct {
	delegate LocalExecutionDelegate
	getenv   func(string) string
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
