// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	sessionRuntimeStatusSchemaV1              = "coze.sandbox.session_runtime_status.v1"
	sessionRuntimeReasonAIOUnavailable        = "AIO_UNAVAILABLE"
	sessionRuntimeReasonRecoveryPending       = "SESSION_RECOVERY_PENDING"
	sessionRuntimeReasonProjectionUnavailable = "SESSION_RUNTIME_STATUS_UNAVAILABLE"
	sessionRuntimeReasonBackendDisabled       = "SESSION_BACKEND_DISABLED"
)

// SessionRuntimeStatusProjection is an aggregate-only private Runner wire
// contract. Transport encryption is intentionally absent: only the caller's
// validated endpoint policy can state whether this particular hop used TLS.
type SessionRuntimeStatusProjection struct {
	Schema               string `json:"schema"`
	Available            bool   `json:"available"`
	AppliedConfigVersion uint64 `json:"applied_config_version"`
	RuntimeGeneration    uint64 `json:"runtime_generation"`
	CoreEnabled          bool   `json:"core_enabled"`
	InteractiveEnabled   bool   `json:"interactive_enabled"`
	HostShellEnabled     bool   `json:"host_shell_enabled"`
	HostShellAvailable   bool   `json:"host_shell_available"`
	RawAIOReady          bool   `json:"raw_aio_ready"`
	GenerationState      string `json:"generation_state"`
	QueueDepth           int    `json:"queue_depth"`
	Running              int    `json:"running"`
	UsedWeight           int    `json:"used_weight"`
	TotalWeight          int    `json:"total_weight"`
	ActiveSessions       int    `json:"active_sessions"`
	IdleSessions         int    `json:"idle_sessions"`
	ActiveShells         int    `json:"active_shells"`
	IdleShells           int    `json:"idle_shells"`
	ReasonCode           string `json:"reason_code,omitempty"`
}

type RuntimeSessionAggregateRepository interface {
	RuntimeSessionAggregate(context.Context, string, uint64) (infrasandbox.RuntimeSessionAggregate, error)
}

type sessionRuntimeStatusSource struct {
	deploymentID string
	lifecycle    CoreLifecycle
	scheduler    *CoreSessionScheduler
	repository   RuntimeSessionAggregateRepository
}

func (source sessionRuntimeStatusSource) SessionRuntimeStatus(ctx context.Context, claims sandboxidentity.SessionRequest) (SessionRuntimeStatusProjection, error) {
	if ctx == nil || source.scheduler == nil || !validSessionConfigurationClaims(claims, claims.RequestDigest) ||
		claims.DeploymentID != source.deploymentID {
		return SessionRuntimeStatusProjection{}, ErrUnavailable
	}
	settings := source.scheduler.AppliedSessionSettings()
	if _, err := domainsandbox.NormalizeSessionRuntimeSettings(settings); err != nil || settings.Version == 0 {
		return SessionRuntimeStatusProjection{}, ErrUnavailable
	}
	status := SessionRuntimeStatusProjection{
		Schema: sessionRuntimeStatusSchemaV1, AppliedConfigVersion: settings.Version,
		CoreEnabled: settings.CoreEnabled, InteractiveEnabled: settings.InteractiveEnabled,
		HostShellEnabled: settings.HostShellEnabled, HostShellAvailable: false,
		GenerationState: string(aio.LifecycleStateUnknown), TotalWeight: coreSessionTotalWeight,
	}
	operationSource, ok := source.scheduler.store.(SessionOperationAggregateSource)
	if !ok {
		status.ReasonCode = sessionRuntimeReasonProjectionUnavailable
		return status, nil
	}
	operations, err := operationSource.SessionOperationAggregate(ctx)
	if err != nil {
		status.ReasonCode = sessionRuntimeReasonProjectionUnavailable
		return status, nil
	}
	status.QueueDepth, status.Running, status.UsedWeight = operations.QueueDepth, operations.Running, operations.UsedWeight

	lifecycle := aio.LifecycleSnapshot{State: aio.LifecycleStateUnknown}
	if source.lifecycle != nil {
		lifecycle = source.lifecycle.Snapshot()
	}
	switch {
	case !lifecycle.Enabled && lifecycle.State == aio.LifecycleStateDisabled:
		status.GenerationState = string(aio.LifecycleStateDisabled)
		status.ReasonCode = sessionRuntimeReasonBackendDisabled
		return status, nil
	case lifecycle.State == aio.LifecycleStateReady && lifecycle.Enabled && lifecycle.Ready && lifecycle.Generation > 0:
		status.RawAIOReady = true
		status.GenerationState = string(aio.LifecycleStateReady)
		status.RuntimeGeneration = lifecycle.Generation
	case lifecycle.State == aio.LifecycleStateRecovering && lifecycle.Enabled && !lifecycle.Ready && lifecycle.Generation > 0:
		status.GenerationState = string(aio.LifecycleStateRecovering)
		status.RuntimeGeneration = lifecycle.Generation
		status.ReasonCode = sessionRuntimeReasonAIOUnavailable
	case lifecycle.State == aio.LifecycleStateUnknown && lifecycle.Enabled && !lifecycle.Ready:
		status.GenerationState = string(aio.LifecycleStateUnknown)
		status.ReasonCode = sessionRuntimeReasonAIOUnavailable
	default:
		status.ReasonCode = sessionRuntimeReasonProjectionUnavailable
		return status, nil
	}

	// A preserved generation is safe for database aggregation even while raw
	// AIO health is unknown; the public generation remains zero in that state.
	aggregateGeneration := lifecycle.Generation
	if aggregateGeneration == 0 || source.repository == nil {
		if status.ReasonCode == "" {
			status.ReasonCode = sessionRuntimeReasonProjectionUnavailable
		}
		return status, nil
	}
	aggregate, err := source.repository.RuntimeSessionAggregate(ctx, source.deploymentID, aggregateGeneration)
	if err != nil || !validRuntimeSessionAggregate(aggregate) {
		status.Available = false
		status.ActiveSessions, status.IdleSessions, status.ActiveShells, status.IdleShells = 0, 0, 0, 0
		status.ReasonCode = sessionRuntimeReasonProjectionUnavailable
		return status, nil
	}
	status.ActiveSessions = aggregate.ReadySessions + aggregate.OperationFencedSessions
	status.IdleSessions = aggregate.IdleSessions
	if operations.Running > aggregate.BoundShells || operations.Running > status.ActiveSessions {
		status.ActiveShells, status.IdleShells = 0, 0
		status.ReasonCode = sessionRuntimeReasonProjectionUnavailable
		return status, nil
	}
	// Redis running operations identify Shells currently in use. Every other
	// database-bound Shell is idle; this avoids guessing from opaque Shell IDs.
	status.ActiveShells = operations.Running
	status.IdleShells = aggregate.BoundShells - operations.Running
	if aggregate.RecoveringSessions > 0 {
		if status.ReasonCode == "" {
			status.ReasonCode = sessionRuntimeReasonRecoveryPending
		}
		return status, nil
	}
	if status.RawAIOReady {
		status.Available = true
		status.ReasonCode = ""
	}
	return status, nil
}

func validRuntimeSessionAggregate(aggregate infrasandbox.RuntimeSessionAggregate) bool {
	return aggregate.ReadySessions >= 0 && aggregate.OperationFencedSessions >= 0 && aggregate.RecoveringSessions >= 0 &&
		aggregate.IdleSessions >= 0 && aggregate.BoundShells >= 0 &&
		aggregate.BoundShells <= aggregate.ReadySessions+aggregate.OperationFencedSessions
}
