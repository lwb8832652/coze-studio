// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"strconv"
)

const (
	coreRuntimeDisabled = "disabled"
	coreRuntimeReady    = "ready"
	coreRuntimeUnknown  = "unknown"
)

// CoreRuntimeStatusSnapshot contains only the stable state exposed to the
// operations projection. The upstream endpoint and reserved sentinel remain
// private to the lifecycle supervisor.
type CoreRuntimeStatusSnapshot struct {
	State      string
	Generation uint64
}

func (snapshot CoreRuntimeStatusSnapshot) String() string {
	return "sandboxrunner.CoreRuntimeStatusSnapshot{state:" + snapshot.State + ",generation:" + strconv.FormatUint(snapshot.Generation, 10) + "}"
}

func (snapshot CoreRuntimeStatusSnapshot) GoString() string { return snapshot.String() }

type CoreRuntimeStatusSource interface {
	CoreRuntimeStatus(context.Context) (CoreRuntimeStatusSnapshot, error)
}

// runtimeStatusSource joins existing aggregate snapshots for the authenticated
// operations endpoint. It intentionally does not retain execution records or
// runtime driver references, so it cannot accidentally expose identifiers.
type runtimeStatusSource struct {
	scheduler             *RunnerScheduler
	lifecycle             *Lifecycle
	configuration         ConfigurationSource
	sessionBackendEnabled bool
	core                  CoreRuntimeStatusSource
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
	core := source.coreStatus(ctx)
	return RuntimeStatusProjection{
		Schema:                      runtimeStatusSchemaV1,
		AppliedConfigurationVersion: configuration.Version,
		CoreState:                   core.State,
		AIORuntimeGeneration:        core.Generation,
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

func (source runtimeStatusSource) coreStatus(ctx context.Context) CoreRuntimeStatusSnapshot {
	if !source.sessionBackendEnabled {
		return CoreRuntimeStatusSnapshot{State: coreRuntimeDisabled}
	}
	if source.core == nil {
		return CoreRuntimeStatusSnapshot{State: coreRuntimeUnknown}
	}
	snapshot, err := source.core.CoreRuntimeStatus(ctx)
	if err != nil {
		return CoreRuntimeStatusSnapshot{State: coreRuntimeUnknown}
	}
	if snapshot.State == coreRuntimeReady && snapshot.Generation > 0 {
		return snapshot
	}
	return CoreRuntimeStatusSnapshot{State: coreRuntimeUnknown}
}

var _ RuntimeStatusSource = runtimeStatusSource{}
