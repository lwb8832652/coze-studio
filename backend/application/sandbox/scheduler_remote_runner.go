// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"sync"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

// SchedulerConfigurationRemote is the limited remote Runner capability used
// by the control plane. It cannot receive partial settings or credentials.
type SchedulerConfigurationRemote interface {
	ApplySchedulerConfiguration(context.Context, infrasandbox.SchedulerConfiguration) error
	RuntimeStatus(context.Context) (infrasandbox.SchedulerRuntimeStatus, error)
}

// SignedSchedulerRunner signs a complete persisted scheduler snapshot before
// every push. Each remote validates the signature independently and therefore
// never trusts an unsigned control-plane request.
type SignedSchedulerRunner struct {
	signer  *infrasandbox.SchedulerConfigSigner
	remotes []SchedulerConfigurationRemote

	mu      sync.RWMutex
	lastSet domainsandbox.SchedulerSettings
}

func NewSignedSchedulerRunner(signer *infrasandbox.SchedulerConfigSigner, remotes []SchedulerConfigurationRemote) (*SignedSchedulerRunner, error) {
	if signer == nil || len(remotes) == 0 {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	cloned := make([]SchedulerConfigurationRemote, 0, len(remotes))
	for _, remote := range remotes {
		if remote == nil {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		cloned = append(cloned, remote)
	}
	return &SignedSchedulerRunner{signer: signer, remotes: cloned}, nil
}

func (runner *SignedSchedulerRunner) ApplySchedulerSettings(ctx context.Context, settings domainsandbox.SchedulerSettings) error {
	if runner == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	normalized, err := domainsandbox.NormalizeSchedulerSettings(settings)
	if err != nil || normalized.Version == 0 {
		return domainsandbox.ErrInvalidInput
	}
	configuration, err := runner.signer.Sign(normalized)
	if err != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	for _, remote := range runner.remotes {
		if err := remote.ApplySchedulerConfiguration(ctx, configuration); err != nil {
			return domainsandbox.ErrUnavailable
		}
	}
	runner.mu.Lock()
	runner.lastSet = normalized
	runner.mu.Unlock()
	return nil
}

func (runner *SignedSchedulerRunner) RuntimeStatus(ctx context.Context) (NativeRunnerStatus, error) {
	if runner == nil || ctx == nil || len(runner.remotes) == 0 {
		return NativeRunnerStatus{}, domainsandbox.ErrUnavailable
	}
	status, err := runner.remotes[0].RuntimeStatus(ctx)
	if err != nil {
		return NativeRunnerStatus{}, err
	}
	return nativeRunnerStatusFromRemote(status), nil
}

func nativeRunnerStatusFromRemote(status infrasandbox.SchedulerRuntimeStatus) NativeRunnerStatus {
	return NativeRunnerStatus{Healthy: true, AppliedConfigVersion: status.AppliedConfigurationVersion, QueueDepth: status.Queued, QueueByScope: cloneQueueByScope(status.QueuedByScope), ActiveSlots: status.Running, SlotCapacity: status.TotalWeight, QueueHighWatermark: status.Queued, QuarantinedCount: status.QuarantinedContainers, IdleContainers: status.IdleContainers, ActiveContainers: status.ActiveContainers, MemoryReserveState: string(status.MemoryReserveState)}
}

func cloneQueueByScope(values map[domainsandbox.Scope]int) map[domainsandbox.Scope]int {
	if len(values) == 0 {
		return nil
	}
	result := make(map[domainsandbox.Scope]int, len(values))
	for scope, count := range values {
		result[scope] = count
	}
	return result
}

var _ NativeSchedulerRunner = (*SignedSchedulerRunner)(nil)
