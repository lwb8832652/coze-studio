// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import "context"

// runtimeStatusSource joins existing aggregate snapshots for the authenticated
// operations endpoint. It intentionally does not retain execution records or
// runtime driver references, so it cannot accidentally expose identifiers.
type runtimeStatusSource struct {
	scheduler     *RunnerScheduler
	lifecycle     *Lifecycle
	configuration ConfigurationSource
}

func (source runtimeStatusSource) RuntimeStatus(ctx context.Context) (RuntimeStatusProjection, error) {
	if source.scheduler == nil || source.lifecycle == nil || source.configuration == nil {
		return RuntimeStatusProjection{}, ErrUnavailable
	}
	configuration, err := source.configuration.Configuration(ctx)
	if err != nil {
		return RuntimeStatusProjection{}, ErrUnavailable
	}
	scheduler := source.scheduler.Snapshot()
	lifecycle := source.lifecycle.Snapshot()
	return RuntimeStatusProjection{
		Schema:                      runtimeStatusSchemaV1,
		AppliedConfigurationVersion: configuration.Version,
		Queued:                      scheduler.Queued,
		QueuedByScope:               scheduler.QueuedByScope,
		Running:                     scheduler.Running,
		UsedWeight:                  scheduler.UsedWeight,
		TotalWeight:                 scheduler.TotalWeight,
		IdleContainers:              lifecycle.Idle,
		ActiveContainers:            lifecycle.Active,
		QuarantinedContainers:       lifecycle.Quarantined,
		MemoryReserveState:          source.scheduler.MemoryReserveState(ctx),
	}, nil
}

var _ RuntimeStatusSource = runtimeStatusSource{}
