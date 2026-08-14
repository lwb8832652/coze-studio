// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

var sessionRuntimeReasonCodePattern = regexp.MustCompile(`^[A-Z0-9_]{1,64}$`)

type SessionSettingsService struct {
	store  domainsandbox.SessionSettingsAuditRepository
	runner NativeSessionRunner
}

func NewSessionSettingsService(options SessionSettingsServiceOptions) (*SessionSettingsService, error) {
	if options.Store == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &SessionSettingsService{store: options.Store, runner: options.Runner}, nil
}

func (s *SessionSettingsService) Get(ctx context.Context, actor Actor) (*SessionSettingsDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	settings, err := s.store.GetSessionSettings(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	return &SessionSettingsDTO{Version: settings.Version, Settings: sessionSettingsProjection(settings)}, nil
}

func (s *SessionSettingsService) Update(ctx context.Context, actor Actor, request UpdateSessionSettingsRequest) (*SessionSettingsUpdateResult, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if request.Settings.InteractiveEnabled {
		return nil, ErrInteractiveUnsupported
	}
	if request.ExpectedVersion == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	settings, err := domainsandbox.NormalizeSessionRuntimeSettings(request.Settings)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	previous, err := s.store.GetSessionSettings(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	attemptedNextVersion, err := domainsandbox.NextVersion(request.ExpectedVersion)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	changedFields := domainsandbox.SessionSettingsChangedFields(previous, settings)
	if previous.Version != request.ExpectedVersion {
		s.recordFailedUpdate(ctx, actor, request.ExpectedVersion, attemptedNextVersion, changedFields)
		return nil, domainsandbox.ErrVersionConflict
	}
	if len(changedFields) == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	updated, err := s.store.UpdateSessionSettingsCASWithAudit(ctx, domainsandbox.UpdateSessionSettingsInput{
		ExpectedVersion: request.ExpectedVersion,
		Settings:        settings,
		UpdatedBy:       actor.UserID,
	}, sessionSettingsAuditInput(actor, domainsandbox.SchedulerAuditActionUpdate, previous.Version, attemptedNextVersion, changedFields))
	if err != nil {
		failedPreviousVersion, failedNextVersion := previous.Version, previous.Version
		if errors.Is(err, domainsandbox.ErrVersionConflict) {
			failedPreviousVersion, failedNextVersion = request.ExpectedVersion, attemptedNextVersion
		}
		s.recordFailedUpdate(ctx, actor, failedPreviousVersion, failedNextVersion, changedFields)
		return nil, stableControlPlaneError(err)
	}
	result := &SessionSettingsUpdateResult{
		Version: updated.Version, Settings: sessionSettingsProjection(updated),
	}
	if s.runner == nil {
		s.recordFailedUpdate(ctx, actor, previous.Version, updated.Version, changedFields)
		result.ReasonCode = SessionReasonRunnerUnavailable
		return result, nil
	}
	appliedVersion, applyErr := s.runner.ApplySessionSettings(ctx, updated)
	result.AppliedVersion = appliedVersion
	if applyErr != nil || appliedVersion != updated.Version {
		s.recordFailedUpdate(ctx, actor, previous.Version, updated.Version, changedFields)
		result.ReasonCode = SessionReasonRunnerUnavailable
		return result, nil
	}
	result.Applied = true
	return result, nil
}

func (s *SessionSettingsService) RuntimeStatus(ctx context.Context, actor Actor) (*SessionRuntimeStatusDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	settings, err := s.store.GetSessionSettings(ctx)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	result := &SessionRuntimeStatusDTO{DesiredConfigVersion: settings.Version}
	if s.runner == nil {
		result.ReasonCode = SessionReasonRunnerUnavailable
		return result, nil
	}
	status, err := s.runner.SessionRuntimeStatus(ctx)
	if err != nil {
		result.ReasonCode = SessionReasonRunnerUnavailable
		return result, nil
	}
	if !validNativeSessionRuntimeStatus(status) {
		return nil, domainsandbox.ErrUnavailable
	}
	result = projectNativeSessionRuntimeStatus(status)
	result.DesiredConfigVersion = settings.Version
	if !status.Available {
		result.ReasonCode = stableSessionRuntimeReason(status.ReasonCode)
		if result.ReasonCode == "" {
			result.ReasonCode = SessionReasonRunnerUnavailable
		}
	}
	return result, nil
}

func validNativeSessionRuntimeStatus(status NativeSessionRuntimeStatus) bool {
	if status.QueueDepth < 0 || status.Running < 0 || status.UsedWeight < 0 || status.TotalWeight < 0 ||
		status.ActiveSessions < 0 || status.IdleSessions < 0 || status.ActiveShells < 0 || status.IdleShells < 0 ||
		status.UsedWeight > status.TotalWeight || status.Running > status.ActiveSessions || status.ActiveShells > status.ActiveSessions {
		return false
	}
	if status.TransportEncrypted && !status.TransportKnown {
		return false
	}
	if status.Available && status.AppliedConfigVersion == 0 {
		return false
	}
	switch status.GenerationState {
	case "disabled", "unknown":
		if status.RuntimeGeneration != 0 {
			return false
		}
	case "ready", "recovering":
		if status.RuntimeGeneration == 0 {
			return false
		}
	default:
		return false
	}
	if status.Available && status.ReasonCode != "" {
		return false
	}
	return status.ReasonCode == "" || stableSessionRuntimeReason(status.ReasonCode) != ""
}

func projectNativeSessionRuntimeStatus(status NativeSessionRuntimeStatus) *SessionRuntimeStatusDTO {
	return &SessionRuntimeStatusDTO{
		Available: status.Available, AppliedConfigVersion: status.AppliedConfigVersion,
		RuntimeGeneration: status.RuntimeGeneration, CoreEnabled: status.CoreEnabled,
		InteractiveEnabled: status.InteractiveEnabled, HostShellEnabled: status.HostShellEnabled,
		HostShellAvailable: status.HostShellAvailable, RawAIOReady: status.RawAIOReady,
		GenerationState: status.GenerationState, QueueDepth: status.QueueDepth, Running: status.Running,
		UsedWeight: status.UsedWeight, TotalWeight: status.TotalWeight,
		ActiveSessions: status.ActiveSessions, IdleSessions: status.IdleSessions,
		ActiveShells: status.ActiveShells, IdleShells: status.IdleShells,
		TransportKnown: status.TransportKnown, TransportEncrypted: status.TransportEncrypted,
	}
}

func stableSessionRuntimeReason(reason string) string {
	if !sessionRuntimeReasonCodePattern.MatchString(reason) {
		return ""
	}
	return reason
}

func (s *SessionSettingsService) recordFailedUpdate(ctx context.Context, actor Actor, previousVersion, newVersion uint64, fields []string) {
	if previousVersion == 0 || newVersion == 0 || len(fields) == 0 {
		return
	}
	_, _ = s.store.AppendSessionSettingsAuditEvent(ctx, sessionSettingsAuditInput(actor, domainsandbox.SchedulerAuditActionUpdateFailed, previousVersion, newVersion, fields))
}

func sessionSettingsAuditInput(actor Actor, action domainsandbox.SchedulerAuditAction, previousVersion, newVersion uint64, fields []string) domainsandbox.AppendSchedulerAuditEventInput {
	return domainsandbox.AppendSchedulerAuditEventInput{
		ActorUserID: actor.UserID, RequestID: actor.RequestID, Action: action,
		Metadata: map[string]string{
			domainsandbox.SchedulerAuditMetadataPreviousVersion: strconv.FormatUint(previousVersion, 10),
			domainsandbox.SchedulerAuditMetadataNewVersion:      strconv.FormatUint(newVersion, 10),
			domainsandbox.SchedulerAuditMetadataChangedFields:   strings.Join(fields, ","),
			domainsandbox.SessionSettingsAuditMetadataDomain:    domainsandbox.SessionSettingsAuditDomain,
		},
	}
}

func sessionSettingsProjection(settings domainsandbox.SessionRuntimeSettings) domainsandbox.SessionRuntimeSettings {
	cloned, err := domainsandbox.NormalizeSessionRuntimeSettings(settings)
	if err != nil {
		return domainsandbox.SessionRuntimeSettings{}
	}
	cloned.Version, cloned.UpdatedBy = 0, 0
	return cloned
}
