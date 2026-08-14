// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

// RuntimeSessionAggregate is the only database projection exposed to Runner
// status. It deliberately contains no tenant key, Session ID, upstream Shell
// ID, path, or credential-bearing value.
type RuntimeSessionAggregate struct {
	ReadySessions           int
	OperationFencedSessions int
	RecoveringSessions      int
	IdleSessions            int
	BoundShells             int
}

// SessionRuntimeStatus is the safe control-plane view returned by a remote
// Session Runner. TransportEncrypted is not accepted from Runner wire data;
// RemoteSessionProvider fills it from its already-validated endpoint policy.
type SessionRuntimeStatus struct {
	Available            bool
	AppliedConfigVersion uint64
	RuntimeGeneration    uint64
	CoreEnabled          bool
	InteractiveEnabled   bool
	HostShellEnabled     bool
	HostShellAvailable   bool
	RawAIOReady          bool
	GenerationState      string
	QueueDepth           int
	Running              int
	UsedWeight           int
	TotalWeight          int
	ActiveSessions       int
	IdleSessions         int
	ActiveShells         int
	IdleShells           int
	TransportEncrypted   bool
	ReasonCode           string
}

// RuntimeSessionAggregate reports the current deployment generation only.
// Operation fences are separated from recovery-pending rows so the Runner can
// distinguish a currently executing Shell from a Session that is unavailable
// for execution. BoundShells is intentionally not classified as active/idle
// until it is reconciled with the Redis running-operation aggregate.
func (r *MySQLRepository) RuntimeSessionAggregate(ctx context.Context, deploymentID string, runtimeGeneration uint64) (RuntimeSessionAggregate, error) {
	normalizedDeploymentID, err := domainsandbox.NormalizeAIOGenerationDeploymentID(deploymentID)
	if err != nil || runtimeGeneration == 0 {
		return RuntimeSessionAggregate{}, domainsandbox.ErrInvalidInput
	}
	release, err := r.acquireOperation()
	if err != nil {
		return RuntimeSessionAggregate{}, err
	}
	defer release()
	db, err := r.dbFor(ctx)
	if err != nil {
		return RuntimeSessionAggregate{}, err
	}
	var aggregate RuntimeSessionAggregate
	err = db.Raw(`SELECT
		COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS ready_sessions,
		COALESCE(SUM(CASE WHEN state = ? AND recovery_reason = ? THEN 1 ELSE 0 END), 0) AS operation_fenced_sessions,
		COALESCE(SUM(CASE WHEN state = ? AND recovery_reason <> ? THEN 1 ELSE 0 END), 0) AS recovering_sessions,
		COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS idle_sessions,
		COALESCE(SUM(CASE WHEN upstream_shell_id IS NOT NULL AND (state = ? OR (state = ? AND recovery_reason = ?)) THEN 1 ELSE 0 END), 0) AS bound_shells
		FROM sandbox_runtime_sessions
		WHERE deployment_id = ? AND runtime_generation = ? AND state <> ?`,
		string(domainsandbox.SessionStateActive),
		string(domainsandbox.SessionStateRecovering), domainsandbox.SessionOperationFenceReason,
		string(domainsandbox.SessionStateRecovering), domainsandbox.SessionOperationFenceReason,
		string(domainsandbox.SessionStateReleased),
		string(domainsandbox.SessionStateActive), string(domainsandbox.SessionStateRecovering), domainsandbox.SessionOperationFenceReason,
		normalizedDeploymentID, runtimeGeneration,
		string(domainsandbox.SessionStateDestroyed),
	).Scan(&aggregate).Error
	if err != nil {
		return RuntimeSessionAggregate{}, mapRuntimeSessionSchemaError(err)
	}
	if aggregate.ReadySessions < 0 || aggregate.OperationFencedSessions < 0 || aggregate.RecoveringSessions < 0 ||
		aggregate.IdleSessions < 0 || aggregate.BoundShells < 0 ||
		aggregate.BoundShells > aggregate.ReadySessions+aggregate.OperationFencedSessions {
		return RuntimeSessionAggregate{}, domainsandbox.ErrConfigurationInvalid
	}
	return aggregate, nil
}
