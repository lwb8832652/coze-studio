// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"strconv"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	healthCodeOK             = "HEALTHY"
	healthCodeDegraded       = "DEGRADED"
	healthCodeUnhealthy      = "UNHEALTHY"
	healthCodeProbeFailed    = domainsandbox.ErrCodeProviderUnhealthy
	healthMessageDegraded    = "health probe degraded"
	healthMessageUnhealthy   = "health probe reported unhealthy"
	healthMessageProbeFailed = "health probe failed"
	maxHealthLatencyMillis   = int64(60_000)

	healthLatencyBucketUnknown  = "unknown"
	healthLatencyBucketFast     = "fast"
	healthLatencyBucketNormal   = "normal"
	healthLatencyBucketSlow     = "slow"
	healthLatencyBucketVerySlow = "very_slow"
)

func (s *Service) HealthCheck(ctx context.Context, actor Actor, request HealthCheckRequest) (*ProviderDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if request.ProviderID <= 0 || request.ExpectedVersion == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	provider, err := s.providers.GetProvider(ctx, request.ProviderID)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	if err = authorizeControlPlaneScopes(actor, provider.Scopes); err != nil {
		return nil, err
	}
	if provider.Version != request.ExpectedVersion {
		return nil, domainsandbox.ErrVersionConflict
	}
	if err = s.requireLocalDebugCapability(provider.Type); err != nil {
		return nil, err
	}

	startedAt := s.now().UTC()
	result, probeErr := func() (result infrasandbox.HealthResult, probeErr error) {
		adapter, buildErr := s.factory.Build(ctx, cloneControlPlaneProvider(provider))
		if adapter != nil {
			defer func() {
				if closeErr := adapter.CloseContext(ctx); closeErr != nil && probeErr == nil {
					probeErr = domainsandbox.ErrProviderUnhealthy
				}
			}()
		}
		if buildErr != nil {
			return infrasandbox.HealthResult{}, buildErr
		}
		if adapter == nil {
			return infrasandbox.HealthResult{}, domainsandbox.ErrProviderUnhealthy
		}
		return adapter.Health(ctx)
	}()
	checkedAt := s.now().UTC()
	latencyMillis := checkedAt.Sub(startedAt).Milliseconds()
	if latencyMillis < 0 {
		latencyMillis = 0
	}
	if latencyMillis > maxHealthLatencyMillis {
		latencyMillis = maxHealthLatencyMillis
	}

	snapshot, probeSucceeded := boundedHealthSnapshot(result, probeErr, provider.Scopes, checkedAt, latencyMillis)
	if s.metrics != nil {
		resultCode := sandboxMetricsResultCode(probeErr)
		if probeErr == nil && !probeSucceeded {
			resultCode = string(snapshot.ReasonCode)
		}
		s.metrics.RecordProviderHealth(ctx, ProviderHealthMetricsObservation{
			ProviderType: provider.Type,
			Status:       snapshot.Status,
			Outcome:      sandboxMetricsOutcome(probeErr),
			ResultCode:   resultCode,
			Elapsed:      checkedAt.Sub(startedAt),
		})
	}
	var persisted *domainsandbox.Provider
	err = s.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		current, getErr := repositories.Providers.GetProvider(txCtx, request.ProviderID)
		if getErr != nil {
			return stableControlPlaneError(getErr)
		}
		if current.Version != request.ExpectedVersion || current.ProviderKey != provider.ProviderKey {
			return domainsandbox.ErrVersionConflict
		}
		nextVersion, updateErr := repositories.Providers.UpdateProviderHealth(txCtx, domainsandbox.UpdateProviderHealthInput{
			ProviderID: current.ID, ExpectedVersion: request.ExpectedVersion, Health: snapshot, ActorUserID: actor.UserID,
		})
		if updateErr != nil {
			return stableControlPlaneError(updateErr)
		}
		auditResult := auditResultFailure
		if probeSucceeded {
			auditResult = auditResultSuccess
		}
		if auditErr := appendControlPlaneAudit(txCtx, repositories.Audits, actor, current.ID,
			auditActionHealthCheck, auditResult, map[string]string{
				domainsandbox.AuditMetadataKeyHealthCode: snapshot.ReasonCode,
				domainsandbox.AuditMetadataKeyVersion:    strconv.FormatUint(nextVersion, 10),
			}); auditErr != nil {
			return auditErr
		}
		current.Health = snapshot
		current.Version = nextVersion
		current.UpdatedBy = actor.UserID
		persisted = cloneControlPlaneProvider(current)
		return nil
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	return s.projectControlPlaneProvider(persisted)
}

func boundedHealthLatencyBucket(latencyMillis int64) string {
	switch {
	case latencyMillis < 0 || latencyMillis > maxHealthLatencyMillis:
		return healthLatencyBucketUnknown
	case latencyMillis < 100:
		return healthLatencyBucketFast
	case latencyMillis < 500:
		return healthLatencyBucketNormal
	case latencyMillis < 2_000:
		return healthLatencyBucketSlow
	default:
		return healthLatencyBucketVerySlow
	}
}

func boundedHealthSnapshot(result infrasandbox.HealthResult, probeErr error, providerScopes []domainsandbox.Scope,
	checkedAt time.Time, latencyMillis int64) (domainsandbox.HealthSnapshot, bool) {
	if probeErr != nil || result.ProtocolVersion != infrasandbox.HealthProtocolV1 {
		return failedHealthSnapshot(providerScopes, checkedAt, latencyMillis), false
	}
	snapshot := domainsandbox.HealthSnapshot{
		Status: result.Status, Capabilities: append([]domainsandbox.Scope(nil), result.Capabilities...),
		LatencyMillis: latencyMillis, CheckedAt: checkedAt,
	}
	succeeded := true
	switch result.Status {
	case domainsandbox.HealthStatusHealthy:
		snapshot.ReasonCode = healthCodeOK
	case domainsandbox.HealthStatusDegraded:
		snapshot.ReasonCode = healthCodeDegraded
		snapshot.Message = healthMessageDegraded
	case domainsandbox.HealthStatusUnhealthy:
		snapshot.ReasonCode = healthCodeUnhealthy
		snapshot.Message = healthMessageUnhealthy
		succeeded = false
	default:
		return failedHealthSnapshot(providerScopes, checkedAt, latencyMillis), false
	}
	normalized, err := domainsandbox.NormalizeHealthSnapshot(snapshot, providerScopes)
	if err != nil {
		return failedHealthSnapshot(providerScopes, checkedAt, latencyMillis), false
	}
	return normalized, succeeded
}

func failedHealthSnapshot(providerScopes []domainsandbox.Scope, checkedAt time.Time, latencyMillis int64) domainsandbox.HealthSnapshot {
	snapshot := domainsandbox.HealthSnapshot{
		Status: domainsandbox.HealthStatusUnhealthy, Capabilities: []domainsandbox.Scope{},
		ReasonCode: healthCodeProbeFailed, Message: healthMessageProbeFailed,
		LatencyMillis: latencyMillis, CheckedAt: checkedAt,
	}
	normalized, err := domainsandbox.NormalizeHealthSnapshot(snapshot, providerScopes)
	if err != nil {
		return snapshot
	}
	return normalized
}
