// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"strconv"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type SchedulerService struct {
	store  domainsandbox.SchedulerSettingsAuditRepository
	runner NativeSchedulerRunner
}

func NewSchedulerService(options SchedulerServiceOptions) (*SchedulerService, error) {
	if options.Store == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &SchedulerService{store: options.Store, runner: options.Runner}, nil
}

func (s *SchedulerService) Get(ctx context.Context, actor Actor) (*SchedulerSettingsDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	settings, err := s.store.GetSchedulerSettings(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	return &SchedulerSettingsDTO{Version: settings.Version, Settings: schedulerSettingsProjection(settings)}, nil
}

func (s *SchedulerService) Update(ctx context.Context, actor Actor, request UpdateSchedulerSettingsRequest) (*SchedulerSettingsUpdateResult, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if request.ExpectedVersion == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	settings, err := domainsandbox.NormalizeSchedulerSettings(request.Settings)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	previous, err := s.store.GetSchedulerSettings(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	attemptedNextVersion, err := domainsandbox.NextVersion(request.ExpectedVersion)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	if previous.Version != request.ExpectedVersion {
		// The pre-read is no longer current, so never derive a successful CAS
		// audit from it. The failed audit describes only the client's attempt.
		s.recordFailedUpdate(ctx, actor, request.ExpectedVersion, attemptedNextVersion, domainsandbox.SchedulerSettingsChangedFields(previous, settings))
		return nil, domainsandbox.ErrVersionConflict
	}
	changedFields := domainsandbox.SchedulerSettingsChangedFields(previous, settings)
	if len(changedFields) == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	successAudit := schedulerAuditInput(actor, domainsandbox.SchedulerAuditActionUpdate, previous.Version, attemptedNextVersion, changedFields)
	updated, err := s.store.UpdateSchedulerSettingsCASWithAudit(ctx, domainsandbox.UpdateSchedulerSettingsInput{
		ExpectedVersion: request.ExpectedVersion, Settings: settings, UpdatedBy: actor.UserID,
	}, successAudit)
	if err != nil {
		failedPreviousVersion, failedNextVersion := previous.Version, previous.Version
		if errors.Is(err, domainsandbox.ErrVersionConflict) {
			// A version conflict means the CAS never changed the snapshot. Record
			// the transition requested by the client, not the stale pre-read state.
			failedPreviousVersion, failedNextVersion = request.ExpectedVersion, attemptedNextVersion
		}
		s.recordFailedUpdate(ctx, actor, failedPreviousVersion, failedNextVersion, changedFields)
		return nil, stableControlPlaneError(err)
	}
	result := &SchedulerSettingsUpdateResult{Version: updated.Version, Settings: schedulerSettingsProjection(updated)}
	if s.runner == nil || s.runner.ApplySchedulerSettings(ctx, updated) != nil {
		s.recordFailedUpdate(ctx, actor, previous.Version, updated.Version, changedFields)
		result.ReasonCode = SchedulerReasonProviderUnavailable
		return result, nil
	}
	result.Applied = true
	return result, nil
}

func (s *SchedulerService) RuntimeStatus(ctx context.Context, actor Actor) (*SchedulerRuntimeStatusDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	settings, err := s.store.GetSchedulerSettings(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	result := &SchedulerRuntimeStatusDTO{DesiredConfigVersion: settings.Version}
	if s.runner == nil {
		result.ReasonCode = SchedulerReasonProviderUnavailable
		return result, nil
	}
	status, err := s.runner.RuntimeStatus(ctx)
	if err != nil || !status.Healthy {
		result.ReasonCode = SchedulerReasonProviderUnavailable
		return result, nil
	}
	if status.QueueDepth < 0 || status.ActiveSlots < 0 || status.SlotCapacity < 0 || status.QueueHighWatermark < 0 || status.DrainingCount < 0 || status.QuarantinedCount < 0 || status.IdleContainers < 0 || status.ActiveContainers < 0 || !validRuntimeStatusAggregate(status) {
		return nil, domainsandbox.ErrUnavailable
	}
	result.Available = true
	result.AppliedConfigVersion = status.AppliedConfigVersion
	result.QueueDepth, result.ActiveSlots, result.SlotCapacity = status.QueueDepth, status.ActiveSlots, status.SlotCapacity
	result.QueueHighWatermark, result.DrainingCount, result.QuarantinedCount = status.QueueHighWatermark, status.DrainingCount, status.QuarantinedCount
	result.QueueByScope, result.IdleContainers, result.ActiveContainers, result.MemoryReserveState = cloneQueueByScope(status.QueueByScope), status.IdleContainers, status.ActiveContainers, status.MemoryReserveState
	return result, nil
}

func validRuntimeStatusAggregate(status NativeRunnerStatus) bool {
	if status.MemoryReserveState != "available" && status.MemoryReserveState != "below_watermark" && status.MemoryReserveState != "unknown" {
		return false
	}
	queued := 0
	for scope, count := range status.QueueByScope {
		if !validSchedulerScope(scope) || count < 0 {
			return false
		}
		queued += count
	}
	return queued == status.QueueDepth
}

func validSchedulerScope(scope domainsandbox.Scope) bool {
	return scope == domainsandbox.ScopeAgent || scope == domainsandbox.ScopeMCPStdio || scope == domainsandbox.ScopeAppDev || scope == domainsandbox.ScopePlugin
}

func (s *SchedulerService) recordFailedUpdate(ctx context.Context, actor Actor, previousVersion, newVersion uint64, fields []string) {
	if previousVersion == 0 || newVersion == 0 || len(fields) == 0 {
		return
	}
	_, _ = s.store.AppendSchedulerAuditEvent(ctx, schedulerAuditInput(actor, domainsandbox.SchedulerAuditActionUpdateFailed, previousVersion, newVersion, fields))
}

func schedulerAuditInput(actor Actor, action domainsandbox.SchedulerAuditAction, previousVersion, newVersion uint64, fields []string) domainsandbox.AppendSchedulerAuditEventInput {
	return domainsandbox.AppendSchedulerAuditEventInput{ActorUserID: actor.UserID, RequestID: actor.RequestID, Action: action, Metadata: map[string]string{
		domainsandbox.SchedulerAuditMetadataPreviousVersion: strconv.FormatUint(previousVersion, 10),
		domainsandbox.SchedulerAuditMetadataNewVersion:      strconv.FormatUint(newVersion, 10),
		domainsandbox.SchedulerAuditMetadataChangedFields:   strings.Join(fields, ","),
	}}
}

func schedulerSettingsProjection(settings domainsandbox.SchedulerSettings) domainsandbox.SchedulerSettings {
	cloned, err := domainsandbox.NormalizeSchedulerSettings(settings)
	if err != nil {
		return domainsandbox.SchedulerSettings{}
	}
	cloned.Version, cloned.UpdatedBy = 0, 0
	return cloned
}
