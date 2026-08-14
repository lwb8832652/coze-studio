// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"os"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type CapabilityPolicy interface {
	Evaluate() SandboxCapabilitiesDTO
}

type environmentCapabilityPolicy struct {
	getenv func(string) string
}

func NewEnvironmentCapabilityPolicy(getenv func(string) string) CapabilityPolicy {
	return environmentCapabilityPolicy{getenv: getenv}
}

func defaultCapabilityPolicy() CapabilityPolicy {
	return NewEnvironmentCapabilityPolicy(os.Getenv)
}

const (
	CapabilityReasonAvailable             = "AVAILABLE"
	CapabilityReasonControlPlaneDisabled  = "CONTROL_PLANE_DISABLED"
	CapabilityReasonLocalDebugUnavailable = "LOCAL_DEBUG_UNAVAILABLE"
	CapabilityReasonHostShellUnavailable  = "HOST_SHELL_UNAVAILABLE"

	sandboxControlPlaneEnabledEnvironment = "SANDBOX_CONTROL_PLANE_ENABLED"
)

func ProjectCapabilities(getenv func(string) string) *SandboxCapabilitiesDTO {
	capabilities := NewEnvironmentCapabilityPolicy(getenv).Evaluate()
	return &capabilities
}

func (p environmentCapabilityPolicy) Evaluate() SandboxCapabilitiesDTO {
	getenv := p.getenv
	controlPlaneAvailable := getenv != nil && getenv(sandboxControlPlaneEnabledEnvironment) == "true"
	controlPlane := CapabilityStateDTO{
		Available:  controlPlaneAvailable,
		ReasonCode: CapabilityReasonControlPlaneDisabled,
		Message:    "sandbox control plane is disabled",
	}
	if controlPlaneAvailable {
		controlPlane.ReasonCode = CapabilityReasonAvailable
		controlPlane.Message = "sandbox control plane is available"
	}

	localDebug := CapabilityStateDTO{
		ReasonCode: CapabilityReasonLocalDebugUnavailable,
		Message:    "local debug sandbox is unavailable",
	}
	if !controlPlaneAvailable {
		localDebug.ReasonCode = CapabilityReasonControlPlaneDisabled
		localDebug.Message = "sandbox control plane is disabled"
	} else if getenv(infrasandbox.AppEnvName) == "debug" &&
		getenv(infrasandbox.AppDevHostRuntimeEnabledEnvName) == "true" {
		localDebug.Available = true
		localDebug.ReasonCode = CapabilityReasonAvailable
		localDebug.Message = "local debug sandbox is available"
	}
	hostShell := CapabilityStateDTO{
		ReasonCode: CapabilityReasonHostShellUnavailable,
		Message:    "host shell sandbox is unavailable",
	}
	if !controlPlaneAvailable {
		hostShell.ReasonCode = CapabilityReasonControlPlaneDisabled
		hostShell.Message = "sandbox control plane is disabled"
	} else if infrasandbox.HostShellSessionAllowed(getenv) {
		hostShell.Available = true
		hostShell.ReasonCode = CapabilityReasonAvailable
		hostShell.Message = "host shell sandbox is available"
	}
	return SandboxCapabilitiesDTO{ControlPlane: controlPlane, LocalDebug: localDebug, HostShell: hostShell}
}

func (s *Service) requireLocalDebugCapability(providerType domainsandbox.ProviderType) error {
	if providerType != domainsandbox.ProviderTypeLocalDebug {
		return nil
	}
	if s == nil || s.capabilities == nil {
		return domainsandbox.ErrLocalDebugUnavailable
	}
	capabilities := s.capabilities.Evaluate()
	if !capabilities.ControlPlane.Available ||
		(!capabilities.LocalDebug.Available && !capabilities.HostShell.Available) {
		return domainsandbox.ErrLocalDebugUnavailable
	}
	return nil
}

func (s *Service) ListDefaults(ctx context.Context, actor Actor) (*ListProviderDefaultsResult, error) {
	if err := validateControlPlaneActor(actor, false); err != nil {
		return nil, err
	}
	if s == nil || s.management == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	allowedScopes, err := domainsandbox.NormalizeScopes(actor.AllowedScopes)
	if err != nil {
		return nil, ErrPermissionDenied
	}
	allowed := make(map[domainsandbox.Scope]struct{}, len(allowedScopes))
	for _, scope := range allowedScopes {
		allowed[scope] = struct{}{}
	}
	defaults, err := s.management.ListProviderDefaults(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	byScope := make(map[domainsandbox.Scope]*domainsandbox.ProviderDefault, len(defaults))
	for _, item := range defaults {
		if item == nil {
			return nil, domainsandbox.ErrUnavailable
		}
		if _, ok := allowed[item.Scope]; ok {
			copyValue := *item
			byScope[item.Scope] = &copyValue
		}
	}
	result := &ListProviderDefaultsResult{Items: make([]ProviderDefaultProjectionDTO, 0, len(allowedScopes))}
	for _, scope := range []domainsandbox.Scope{
		domainsandbox.ScopeAgent,
		domainsandbox.ScopeMCPStdio,
		domainsandbox.ScopeAppDev,
	} {
		if _, ok := allowed[scope]; !ok {
			continue
		}
		projection := ProviderDefaultProjectionDTO{Scope: scope}
		current := byScope[scope]
		if current != nil {
			provider, getErr := s.providers.GetProvider(ctx, current.ProviderID)
			if getErr != nil {
				return nil, stableControlPlaneError(getErr)
			}
			if authErr := authorizeControlPlaneScopes(actor, provider.Scopes); authErr != nil {
				return nil, authErr
			}
			providerID := current.ProviderID
			updatedAt := current.UpdatedAt.UTC()
			projection.Configured = true
			projection.ProviderID = &providerID
			projection.ProviderName = provider.Name
			projection.ProviderType = provider.Type
			projection.ProviderStatus = provider.Status
			projection.Version = current.Version
			projection.ProviderVersion = provider.Version
			projection.UpdatedAt = &updatedAt
		}
		result.Items = append(result.Items, projection)
	}
	return result, nil
}

func (s *Service) GetSummary(ctx context.Context, actor Actor) (*ProviderSummaryDTO, error) {
	if err := validateControlPlaneActor(actor, false); err != nil {
		return nil, err
	}
	if s == nil || s.management == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	allowedScopes, err := domainsandbox.NormalizeScopes(actor.AllowedScopes)
	if err != nil {
		return nil, ErrPermissionDenied
	}
	summary, err := s.management.SummarizeProviders(ctx, allowedScopes)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	if summary.Total < 0 || summary.Enabled < 0 || summary.Unhealthy < 0 ||
		summary.Enabled > summary.Total || summary.Unhealthy > summary.Total {
		return nil, domainsandbox.ErrUnavailable
	}
	return &ProviderSummaryDTO{
		Total: summary.Total, Enabled: summary.Enabled, Unhealthy: summary.Unhealthy,
	}, nil
}
