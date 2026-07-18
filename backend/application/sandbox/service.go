// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/google/uuid"
)

const (
	maxControlPlaneRequestIDBytes  = 128
	preflightProviderKey           = "018f0d2e-7b73-7e21-9a89-1a2b3c4d5e6f"
	preflightSecret                = "pending-secret"
	preflightFingerprint           = "00000000000000000000000000000000"
	defaultProviderHealthFreshness = 5 * time.Minute
)

type Service struct {
	providers    domainsandbox.ProviderRepository
	unitOfWork   domainsandbox.ProviderCreateUnitOfWork
	management   domainsandbox.ProviderManagementRepository
	capabilities CapabilityPolicy
	codec        CredentialCodec
	leases       ProviderLifecycleGuard
	factory      HealthProviderFactory
	metrics      SandboxMetricsRecorder
	providerKey  func(Actor) (string, error)
	now          func() time.Time
}

func NewService(options ServiceOptions) (*Service, error) {
	if options.Providers == nil || options.UnitOfWork == nil || options.Codec == nil ||
		options.Leases == nil || options.Factory == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	providerKey := options.ProviderKey
	if providerKey == nil {
		providerKey = func(Actor) (string, error) { return uuid.NewString(), nil }
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	management := options.Management
	if management == nil {
		management, _ = options.Providers.(domainsandbox.ProviderManagementRepository)
	}
	capabilities := options.Capabilities
	if capabilities == nil {
		capabilities = defaultCapabilityPolicy()
	}
	return &Service{
		providers: options.Providers, unitOfWork: options.UnitOfWork, codec: options.Codec,
		management: management, capabilities: capabilities, leases: options.Leases, factory: options.Factory,
		metrics: options.Metrics, providerKey: providerKey, now: now,
	}, nil
}

func (s *Service) Create(ctx context.Context, actor Actor, request CreateProviderRequest) (*ProviderDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if err := s.requireLocalDebugCapability(request.Type); err != nil {
		return nil, err
	}
	normalized, err := normalizeCreateProviderRequest(actor, request)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	providerKey, err := s.providerKey(actor)
	if err != nil || domainsandbox.ValidateProviderKey(providerKey) != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}

	endpoint := append([]byte(nil), normalized.endpoint...)
	credential := append([]byte(nil), normalized.credential...)
	defer wipeApplicationBytes(endpoint)
	defer wipeApplicationBytes(credential)

	endpointSecret := ""
	credentialSecret := ""
	credentialFingerprint := ""
	if normalized.providerType == domainsandbox.ProviderTypeRemoteHTTP {
		endpointSecret, err = s.codec.Encrypt(providerKey, infrasandbox.CredentialFieldEndpoint, endpoint)
		if err != nil {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		credentialSecret, err = s.codec.Encrypt(providerKey, infrasandbox.CredentialFieldCredential, credential)
		if err != nil {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		credentialFingerprint, err = s.codec.FingerprintCredential(credential)
		if err != nil {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
	}

	input, err := domainsandbox.NormalizeCreateProviderInput(domainsandbox.CreateProviderInput{
		ProviderKey: providerKey, Name: normalized.name, Type: normalized.providerType,
		EndpointSecret: endpointSecret, EndpointHint: normalized.endpointHint,
		CredentialSecret: credentialSecret, CredentialFingerprint: credentialFingerprint,
		Scopes: normalized.scopes, Policy: normalized.policy, ActorUserID: actor.UserID,
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}

	var created *domainsandbox.Provider
	err = s.unitOfWork.WithinProviderCreateTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		provider, createErr := repositories.Providers.CreateProvider(txCtx, input)
		if createErr != nil {
			return stableControlPlaneError(createErr)
		}
		if auditErr := appendControlPlaneAudit(txCtx, repositories.Audits, actor, provider.ID,
			auditActionCreate, auditResultSuccess, map[string]string{
				domainsandbox.AuditMetadataKeyVersion: strconv.FormatUint(provider.Version, 10),
			}); auditErr != nil {
			return auditErr
		}
		created = cloneControlPlaneProvider(provider)
		return nil
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	return s.projectControlPlaneProvider(created)
}

func (s *Service) Get(ctx context.Context, actor Actor, providerID int64) (*ProviderDTO, error) {
	if err := validateControlPlaneActor(actor, false); err != nil {
		return nil, err
	}
	if providerID <= 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	provider, err := s.providers.GetProvider(ctx, providerID)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	if err := authorizeControlPlaneScopes(actor, provider.Scopes); err != nil {
		return nil, err
	}
	return s.projectControlPlaneProvider(provider)
}

func (s *Service) List(ctx context.Context, actor Actor, request ListProvidersRequest) (*ListProvidersResult, error) {
	if err := validateControlPlaneActor(actor, false); err != nil {
		return nil, err
	}
	authorizedScopes, err := domainsandbox.NormalizeScopes(actor.AllowedScopes)
	if err != nil {
		return nil, ErrPermissionDenied
	}
	if request.Scope != "" {
		normalizedScope, normalizeErr := domainsandbox.NormalizeScopes([]domainsandbox.Scope{request.Scope})
		if normalizeErr != nil {
			return nil, domainsandbox.ErrInvalidInput
		}
		request.Scope = normalizedScope[0]
		if err := authorizeControlPlaneScopes(actor, []domainsandbox.Scope{request.Scope}); err != nil {
			return nil, err
		}
	}
	normalized, err := domainsandbox.NormalizeProviderListRequest(domainsandbox.ProviderListRequest{
		Keyword: request.Keyword, Type: request.Type, Status: request.Status, Health: request.Health, Scope: request.Scope,
		AuthorizedScopes: authorizedScopes, Offset: request.Offset, Limit: request.Limit,
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	providers, total, err := s.providers.ListProviders(ctx, normalized)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	items := make([]ProviderDTO, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			return nil, domainsandbox.ErrUnavailable
		}
		if err := authorizeControlPlaneScopes(actor, provider.Scopes); err != nil {
			return nil, err
		}
		projected, projectErr := s.projectControlPlaneProvider(provider)
		if projectErr != nil {
			return nil, projectErr
		}
		items = append(items, *projected)
	}
	return &ListProvidersResult{Items: items, Total: total}, nil
}

func (s *Service) Update(ctx context.Context, actor Actor, request UpdateProviderRequest) (*ProviderDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	normalized, err := normalizeUpdateProviderRequest(request)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	if err := authorizeControlPlaneScopes(actor, normalized.scopes); err != nil {
		return nil, err
	}
	endpointValue := append([]byte(nil), normalized.endpoint.Value...)
	credentialValue := append([]byte(nil), normalized.credential.Value...)
	defer wipeApplicationBytes(endpointValue)
	defer wipeApplicationBytes(credentialValue)
	normalized.endpoint.Value = endpointValue
	normalized.credential.Value = credentialValue

	var updated *domainsandbox.Provider
	err = s.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		current, getErr := repositories.Providers.GetProviderForUpdate(txCtx, normalized.providerID)
		if getErr != nil {
			return stableControlPlaneError(getErr)
		}
		if authErr := authorizeControlPlaneScopes(actor, current.Scopes); authErr != nil {
			return authErr
		}
		if current.Version != normalized.expectedVersion {
			return domainsandbox.ErrVersionConflict
		}

		endpointSecret, endpointHint, endpointChanged, resolveErr := s.resolveEndpointMutation(current, normalized.endpoint)
		if resolveErr != nil {
			return resolveErr
		}
		credentialSecret, credentialFingerprint, credentialChanged, resolveErr := s.resolveCredentialMutation(current, normalized.credential)
		if resolveErr != nil {
			return resolveErr
		}

		mergedPolicy := domainsandbox.MergeRuntimePolicyPublicPatch(current.Policy, normalized.policy)
		input, normalizeErr := domainsandbox.NormalizeUpdateProviderInput(domainsandbox.UpdateProviderInput{
			ProviderID: current.ID, ExpectedVersion: normalized.expectedVersion,
			Name: normalized.name, Type: current.Type, EndpointSecret: endpointSecret, EndpointHint: endpointHint,
			CredentialSecret: credentialSecret, CredentialFingerprint: credentialFingerprint,
			Scopes: normalized.scopes, Policy: mergedPolicy, ActorUserID: actor.UserID,
		})
		if normalizeErr != nil {
			return stableControlPlaneError(normalizeErr)
		}
		input.ResetHealth = endpointChanged || credentialChanged ||
			!reflect.DeepEqual(current.Scopes, input.Scopes) ||
			!domainsandbox.PublicRuntimePoliciesEqual(current.Policy, input.Policy)
		changedFields := controlPlaneChangedFields(current, input, endpointChanged, credentialChanged)
		if len(changedFields) == 0 {
			updated = cloneControlPlaneProvider(current)
			return nil
		}
		if input.ResetHealth {
			isDefault, defaultErr := providerIsDefault(txCtx, repositories.Defaults, current.ID)
			if defaultErr != nil {
				return stableControlPlaneError(defaultErr)
			}
			if isDefault {
				return domainsandbox.ErrProviderInUse
			}
		}
		provider, updateErr := repositories.Providers.UpdateProvider(txCtx, input)
		if updateErr != nil {
			return stableControlPlaneError(updateErr)
		}
		metadata, metadataErr := changedFieldsAuditMetadata(changedFields, provider.Version)
		if metadataErr != nil {
			return metadataErr
		}
		if auditErr := appendControlPlaneAudit(txCtx, repositories.Audits, actor, provider.ID,
			auditActionUpdate, auditResultSuccess, metadata); auditErr != nil {
			return auditErr
		}
		updated = cloneControlPlaneProvider(provider)
		return nil
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	return s.projectControlPlaneProvider(updated)
}

func (s *Service) SetStatus(ctx context.Context, actor Actor, request SetProviderStatusRequest) (*ProviderDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if request.ProviderID <= 0 || request.ExpectedVersion == 0 ||
		(request.Status != domainsandbox.ProviderStatusEnabled && request.Status != domainsandbox.ProviderStatusDisabled) {
		return nil, domainsandbox.ErrInvalidInput
	}
	preflight, getErr := s.providers.GetProvider(ctx, request.ProviderID)
	if getErr != nil {
		return nil, stableControlPlaneError(getErr)
	}
	if authErr := authorizeControlPlaneScopes(actor, preflight.Scopes); authErr != nil {
		return nil, authErr
	}
	if preflight.Version != request.ExpectedVersion {
		return nil, domainsandbox.ErrVersionConflict
	}
	if request.Status == domainsandbox.ProviderStatusEnabled {
		if err := s.requireLocalDebugCapability(preflight.Type); err != nil {
			return nil, err
		}
	}
	drainHandle, drainErr := s.leases.BeginDrain(ctx, preflight.ProviderKey)
	if drainErr != nil || drainHandle.Current == "" {
		return nil, domainsandbox.ErrUnavailable
	}
	if request.Status == domainsandbox.ProviderStatusDisabled && drainHandle.ActiveCount > 0 {
		if recoveryErr := s.recoverProviderDrain(
			ctx,
			preflight.ProviderKey,
			drainHandle,
			preflight.Status == domainsandbox.ProviderStatusEnabled,
		); recoveryErr != nil {
			return nil, recoveryErr
		}
		return nil, domainsandbox.ErrProviderInUse
	}
	if request.Status == domainsandbox.ProviderStatusEnabled {
		if activateErr := s.leases.Activate(ctx, preflight.ProviderKey, drainHandle.Current); activateErr != nil {
			if errors.Is(activateErr, domainsandbox.ErrExecutionForbidden) {
				return nil, domainsandbox.ErrUnavailable
			}
			return nil, s.compensateProviderEnable(
				ctx,
				request.ProviderID,
				preflight.ProviderKey,
				drainHandle.Current,
				domainsandbox.ErrUnavailable,
			)
		}
	}

	var result *domainsandbox.Provider
	err := s.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		current, getErr := repositories.Providers.GetProviderForUpdate(txCtx, request.ProviderID)
		if getErr != nil {
			return stableControlPlaneError(getErr)
		}
		if authErr := authorizeControlPlaneScopes(actor, current.Scopes); authErr != nil {
			return authErr
		}
		if current.Version != request.ExpectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		if request.Status == domainsandbox.ProviderStatusEnabled {
			if eligibilityErr := s.validateProviderEnableEligibility(current); eligibilityErr != nil {
				return eligibilityErr
			}
		} else {
			isDefault, defaultErr := providerIsDefault(txCtx, repositories.Defaults, current.ID)
			if defaultErr != nil {
				return stableControlPlaneError(defaultErr)
			}
			if isDefault {
				return domainsandbox.ErrProviderInUse
			}
		}
		if current.Status == request.Status {
			result = cloneControlPlaneProvider(current)
			return nil
		}
		nextVersion, versionErr := domainsandbox.NextVersion(request.ExpectedVersion)
		if versionErr != nil {
			return versionErr
		}
		action := auditActionEnable
		if request.Status == domainsandbox.ProviderStatusDisabled {
			action = auditActionDisable
		}
		if auditErr := appendControlPlaneAudit(txCtx, repositories.Audits, actor, current.ID, action,
			auditResultSuccess, map[string]string{
				domainsandbox.AuditMetadataKeyPreviousStatus: string(current.Status),
				domainsandbox.AuditMetadataKeyNewStatus:      string(request.Status),
				domainsandbox.AuditMetadataKeyVersion:        strconv.FormatUint(nextVersion, 10),
			}); auditErr != nil {
			return auditErr
		}
		persistedVersion, updateErr := repositories.Providers.UpdateProviderStatus(txCtx, domainsandbox.UpdateProviderStatusInput{
			ProviderID: current.ID, ExpectedVersion: request.ExpectedVersion, Status: request.Status, ActorUserID: actor.UserID,
		})
		if updateErr != nil {
			return stableControlPlaneError(updateErr)
		}
		if persistedVersion != nextVersion {
			return domainsandbox.ErrVersionConflict
		}
		current.Status = request.Status
		current.Version = nextVersion
		current.UpdatedBy = actor.UserID
		result = cloneControlPlaneProvider(current)
		return nil
	})
	if err != nil {
		if request.Status == domainsandbox.ProviderStatusEnabled {
			return nil, s.compensateProviderEnable(
				ctx,
				request.ProviderID,
				preflight.ProviderKey,
				drainHandle.Current,
				err,
			)
		}
		return nil, s.reconcileProviderDrainAfterMutation(
			ctx,
			request.ProviderID,
			preflight.ProviderKey,
			drainHandle.Current,
			err,
		)
	}
	return s.projectControlPlaneProvider(result)
}

func (s *Service) SetDefault(ctx context.Context, actor Actor, request SetProviderDefaultRequest) (*ProviderDefaultDTO, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if request.ProviderID <= 0 || request.ProviderExpectedVersion == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	normalizedScope, scopeErr := domainsandbox.NormalizeScopes([]domainsandbox.Scope{request.Scope})
	if scopeErr != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	request.Scope = normalizedScope[0]
	if err := authorizeControlPlaneScopes(actor, []domainsandbox.Scope{request.Scope}); err != nil {
		return nil, err
	}
	var result *domainsandbox.ProviderDefault
	err := s.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		provider, getErr := repositories.Providers.GetProviderForUpdate(txCtx, request.ProviderID)
		if getErr != nil {
			return stableControlPlaneError(getErr)
		}
		if provider.Version != request.ProviderExpectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		if provider.Status != domainsandbox.ProviderStatusEnabled {
			return domainsandbox.ErrProviderDisabled
		}
		if !controlPlaneContainsScope(provider.Scopes, request.Scope) {
			return domainsandbox.ErrScopeUnsupported
		}
		if !domainsandbox.HealthUsableForScope(
			provider.Health,
			request.Scope,
			s.now().UTC(),
			defaultProviderHealthFreshness,
		) {
			return domainsandbox.ErrProviderUnhealthy
		}

		current, defaultErr := repositories.Defaults.GetProviderDefault(txCtx, request.Scope)
		if defaultErr != nil && !errors.Is(defaultErr, domainsandbox.ErrDefaultMissing) {
			return stableControlPlaneError(defaultErr)
		}
		if current != nil && current.ProviderID == provider.ID {
			if current.Version != request.ExpectedVersion {
				return domainsandbox.ErrVersionConflict
			}
			result = current
			return nil
		}
		if current == nil && request.ExpectedVersion != 0 {
			return domainsandbox.ErrDefaultMissing
		}
		defaultValue, setErr := repositories.Defaults.SetProviderDefault(txCtx, domainsandbox.SetProviderDefaultInput{
			Scope: request.Scope, ProviderID: provider.ID, ExpectedVersion: request.ExpectedVersion, ActorUserID: actor.UserID,
		})
		if setErr != nil {
			return stableControlPlaneError(setErr)
		}
		if auditErr := appendControlPlaneAudit(txCtx, repositories.Audits, actor, provider.ID,
			auditActionSetDefault, auditResultSuccess, map[string]string{
				domainsandbox.AuditMetadataKeyScope:   string(request.Scope),
				domainsandbox.AuditMetadataKeyVersion: strconv.FormatUint(defaultValue.Version, 10),
			}); auditErr != nil {
			return auditErr
		}
		copyValue := *defaultValue
		result = &copyValue
		return nil
	})
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	return &ProviderDefaultDTO{
		Scope: result.Scope, ProviderID: result.ProviderID, Version: result.Version, UpdatedAt: result.UpdatedAt,
	}, nil
}

func (s *Service) Delete(ctx context.Context, actor Actor, request DeleteProviderRequest) (*ProviderMutationResult, error) {
	if err := validateControlPlaneActor(actor, true); err != nil {
		return nil, err
	}
	if request.ProviderID <= 0 || request.ExpectedVersion == 0 {
		return nil, domainsandbox.ErrInvalidInput
	}
	preflight, err := s.providers.GetProvider(ctx, request.ProviderID)
	if err != nil {
		return nil, stableControlPlaneError(err)
	}
	if err := authorizeControlPlaneScopes(actor, preflight.Scopes); err != nil {
		return nil, err
	}
	if preflight.Version != request.ExpectedVersion {
		return nil, domainsandbox.ErrVersionConflict
	}
	drainHandle, err := s.leases.BeginDrain(ctx, preflight.ProviderKey)
	if err != nil || drainHandle.Current == "" {
		return nil, domainsandbox.ErrUnavailable
	}
	if drainHandle.ActiveCount > 0 {
		if recoveryErr := s.recoverProviderDrain(
			ctx,
			preflight.ProviderKey,
			drainHandle,
			preflight.Status == domainsandbox.ProviderStatusEnabled,
		); recoveryErr != nil {
			return nil, recoveryErr
		}
		return nil, domainsandbox.ErrProviderInUse
	}

	var nextVersion uint64
	err = s.unitOfWork.WithinTransaction(ctx, func(txCtx context.Context, repositories domainsandbox.TransactionRepositories) error {
		provider, getErr := repositories.Providers.GetProviderForUpdate(txCtx, request.ProviderID)
		if getErr != nil {
			return stableControlPlaneError(getErr)
		}
		if authErr := authorizeControlPlaneScopes(actor, provider.Scopes); authErr != nil {
			return authErr
		}
		if provider.Version != request.ExpectedVersion {
			return domainsandbox.ErrVersionConflict
		}
		if provider.Status != domainsandbox.ProviderStatusDisabled {
			return domainsandbox.ErrProviderInUse
		}
		isDefault, defaultErr := providerIsDefault(txCtx, repositories.Defaults, provider.ID)
		if defaultErr != nil {
			return stableControlPlaneError(defaultErr)
		}
		if isDefault {
			return domainsandbox.ErrProviderInUse
		}
		version, versionErr := domainsandbox.NextVersion(request.ExpectedVersion)
		if versionErr != nil {
			return versionErr
		}
		if auditErr := appendControlPlaneAudit(txCtx, repositories.Audits, actor, provider.ID,
			auditActionDelete, auditResultSuccess, map[string]string{
				domainsandbox.AuditMetadataKeyVersion: strconv.FormatUint(version, 10),
			}); auditErr != nil {
			return auditErr
		}
		persistedVersion, deleteErr := repositories.Providers.DeleteProvider(txCtx, domainsandbox.DeleteProviderInput{
			ProviderID: provider.ID, ExpectedVersion: request.ExpectedVersion, ActorUserID: actor.UserID,
		})
		if deleteErr != nil {
			return stableControlPlaneError(deleteErr)
		}
		if persistedVersion != version {
			return domainsandbox.ErrVersionConflict
		}
		nextVersion = version
		return nil
	})
	if err != nil {
		return nil, s.reconcileProviderDrainAfterMutation(
			ctx,
			request.ProviderID,
			preflight.ProviderKey,
			drainHandle.Current,
			err,
		)
	}
	return &ProviderMutationResult{ProviderID: request.ProviderID, Version: nextVersion}, nil
}

func (s *Service) recoverProviderDrain(
	ctx context.Context,
	providerKey string,
	handle DrainHandle,
	restoreActive bool,
) error {
	cleanupContext, cancel := newCapacityCleanupContext(ctx)
	defer cancel()
	var err error
	if restoreActive {
		err = s.leases.RestoreActive(cleanupContext, providerKey, handle.Current)
	} else {
		err = s.leases.RetainDrain(cleanupContext, providerKey, handle.Current)
	}
	if err != nil {
		return domainsandbox.ErrUnavailable
	}
	return nil
}

func (s *Service) reconcileProviderDrainAfterMutation(
	ctx context.Context,
	providerID int64,
	providerKey string,
	current DrainFence,
	primaryErr error,
) error {
	stateContext, cancelState := newCapacityCleanupContext(ctx)
	provider, stateErr := s.providers.GetProvider(stateContext, providerID)
	cancelState()

	stateConfirmed := false
	restoreActive := false
	switch {
	case stateErr == nil && provider != nil && provider.ProviderKey == providerKey:
		switch provider.Status {
		case domainsandbox.ProviderStatusEnabled:
			stateConfirmed = true
			restoreActive = true
		case domainsandbox.ProviderStatusDisabled:
			stateConfirmed = true
		default:
			stateErr = domainsandbox.ErrConfigurationInvalid
		}
	case errors.Is(stateErr, domainsandbox.ErrProviderNotFound):
		stateConfirmed = true
	case stateErr == nil:
		stateErr = domainsandbox.ErrConfigurationInvalid
	}

	lifecycleContext, cancelLifecycle := newCapacityCleanupContext(ctx)
	defer cancelLifecycle()
	var lifecycleErr error
	if restoreActive {
		lifecycleErr = s.leases.RestoreActive(lifecycleContext, providerKey, current)
	} else {
		lifecycleErr = s.leases.RetainDrain(lifecycleContext, providerKey, current)
	}
	if lifecycleErr != nil || !stateConfirmed {
		recoveryErr := errors.Join(
			stableControlPlaneError(primaryErr),
			stableControlPlaneError(stateErr),
			stableControlPlaneError(lifecycleErr),
		)
		logs.CtxErrorf(
			lifecycleContext,
			"[Sandbox] provider drain reconciliation failed provider_id=%d: %v",
			providerID,
			recoveryErr,
		)
		return domainsandbox.ErrUnavailable
	}
	return stableControlPlaneError(primaryErr)
}

func (s *Service) compensateProviderEnable(
	ctx context.Context,
	providerID int64,
	providerKey string,
	activation DrainFence,
	primaryErr error,
) error {
	cleanupContext, cancel := newCapacityCleanupContext(ctx)
	defer cancel()
	current, reconcileErr := s.providers.GetProvider(cleanupContext, providerID)
	if reconcileErr != nil || current == nil || current.ProviderKey != providerKey {
		if reconcileErr == nil {
			reconcileErr = domainsandbox.ErrConfigurationInvalid
		}
		safeRecoveryErr := errors.Join(stableControlPlaneError(primaryErr), stableControlPlaneError(reconcileErr))
		logs.CtxErrorf(cleanupContext, "[Sandbox] provider enable state reconciliation failed provider_id=%d: %v", providerID, safeRecoveryErr)
		return domainsandbox.ErrUnavailable
	}
	if current.Status == domainsandbox.ProviderStatusEnabled {
		return stableControlPlaneError(primaryErr)
	}
	compensationResult, compensationErr := s.leases.CompensateActivation(
		cleanupContext,
		providerKey,
		activation,
	)
	if compensationErr == nil &&
		(compensationResult == ActivationCompensationApplied ||
			compensationResult == ActivationCompensationAlreadyApplied) {
		return stableControlPlaneError(primaryErr)
	}
	if compensationErr == nil && compensationResult == ActivationCompensationStale {
		final, finalErr := s.providers.GetProvider(cleanupContext, providerID)
		if finalErr == nil && final != nil && final.ProviderKey == providerKey &&
			final.Status == domainsandbox.ProviderStatusEnabled {
			return stableControlPlaneError(primaryErr)
		}
		if finalErr == nil {
			finalErr = domainsandbox.ErrUnavailable
		}
		compensationErr = finalErr
	} else if compensationErr == nil {
		compensationErr = domainsandbox.ErrUnavailable
	}
	safeRecoveryErr := errors.Join(stableControlPlaneError(primaryErr), stableControlPlaneError(compensationErr))
	logs.CtxErrorf(cleanupContext, "[Sandbox] provider enable conditional recovery failed provider_id=%d: %v", providerID, safeRecoveryErr)
	return domainsandbox.ErrUnavailable
}

func (s *Service) validateProviderEnableEligibility(provider *domainsandbox.Provider) error {
	if provider == nil {
		return domainsandbox.ErrCredentialInvalid
	}
	if provider.Type == domainsandbox.ProviderTypeLocalDebug {
		return nil
	}
	if provider.Type != domainsandbox.ProviderTypeRemoteHTTP || provider.EndpointSecret == "" ||
		provider.EndpointHint == "" || provider.CredentialSecret == "" || provider.CredentialFingerprint == "" {
		return domainsandbox.ErrCredentialInvalid
	}
	endpointMetadata, err := s.codec.Inspect(provider.EndpointSecret)
	if err != nil || !endpointMetadata.Active || endpointMetadata.NeedsRewrap {
		return domainsandbox.ErrCredentialInvalid
	}
	credentialMetadata, err := s.codec.Inspect(provider.CredentialSecret)
	if err != nil || !credentialMetadata.Active || credentialMetadata.NeedsRewrap {
		return domainsandbox.ErrCredentialInvalid
	}
	projection, err := ProjectSecrets(SecretProjectionInput{
		EndpointEnvelope: &SecretEnvelopeInput{
			Active: endpointMetadata.Active, NeedsRewrap: endpointMetadata.NeedsRewrap,
		},
		EndpointHint: provider.EndpointHint,
		CredentialEnvelope: &SecretEnvelopeInput{
			Active: credentialMetadata.Active, NeedsRewrap: credentialMetadata.NeedsRewrap,
		},
		CredentialFingerprint: provider.CredentialFingerprint,
	})
	if err != nil || !projection.Active || projection.NeedsRewrap || !projection.CredentialConfigured {
		return domainsandbox.ErrCredentialInvalid
	}
	return nil
}

func providerIsDefault(
	ctx context.Context,
	repository domainsandbox.ProviderDefaultRepository,
	providerID int64,
) (bool, error) {
	defaults, err := repository.ListProviderDefaults(ctx)
	if err != nil {
		return false, err
	}
	for _, current := range defaults {
		if current != nil && current.ProviderID == providerID {
			return true, nil
		}
	}
	return false, nil
}

type normalizedCreateRequest struct {
	name         string
	providerType domainsandbox.ProviderType
	endpoint     []byte
	endpointHint string
	credential   []byte
	scopes       []domainsandbox.Scope
	policy       domainsandbox.RuntimePolicy
}

func normalizeCreateProviderRequest(actor Actor, request CreateProviderRequest) (normalizedCreateRequest, error) {
	name := strings.TrimSpace(request.Name)
	scopes, err := domainsandbox.NormalizeScopes(request.Scopes)
	if err != nil {
		return normalizedCreateRequest{}, err
	}
	if err = authorizeControlPlaneScopes(actor, scopes); err != nil {
		return normalizedCreateRequest{}, err
	}
	policy, err := domainsandbox.NormalizeRuntimePolicy(request.Policy)
	if err != nil {
		return normalizedCreateRequest{}, err
	}
	endpoint := []byte(nil)
	credential := []byte(nil)
	endpointHint := ""
	endpointSecret := ""
	credentialSecret := ""
	fingerprint := ""
	switch request.Type {
	case domainsandbox.ProviderTypeRemoteHTTP:
		endpoint, endpointHint, err = normalizeRemoteControlPlaneEndpoint(request.Endpoint)
		if err != nil || !validControlPlaneCredential(request.Credential) {
			return normalizedCreateRequest{}, domainsandbox.ErrInvalidInput
		}
		credential = append([]byte(nil), request.Credential...)
		endpointSecret = preflightSecret
		credentialSecret = preflightSecret
		fingerprint = preflightFingerprint
	case domainsandbox.ProviderTypeLocalDebug:
		if len(request.Endpoint) != 0 || len(request.Credential) != 0 {
			return normalizedCreateRequest{}, domainsandbox.ErrInvalidInput
		}
	default:
		return normalizedCreateRequest{}, domainsandbox.ErrInvalidInput
	}
	_, err = domainsandbox.NormalizeCreateProviderInput(domainsandbox.CreateProviderInput{
		ProviderKey: preflightProviderKey, Name: name, Type: request.Type,
		EndpointSecret: endpointSecret, EndpointHint: endpointHint,
		CredentialSecret: credentialSecret, CredentialFingerprint: fingerprint,
		Scopes: scopes, Policy: policy, ActorUserID: actor.UserID,
	})
	if err != nil {
		wipeApplicationBytes(endpoint)
		wipeApplicationBytes(credential)
		return normalizedCreateRequest{}, err
	}
	return normalizedCreateRequest{
		name: name, providerType: request.Type, endpoint: endpoint, endpointHint: endpointHint,
		credential: credential, scopes: scopes, policy: policy,
	}, nil
}

type normalizedUpdateRequest struct {
	providerID      int64
	expectedVersion uint64
	name            string
	scopes          []domainsandbox.Scope
	policy          domainsandbox.RuntimePolicy
	endpoint        SecretMutation
	credential      SecretMutation
}

func normalizeUpdateProviderRequest(request UpdateProviderRequest) (normalizedUpdateRequest, error) {
	if request.ProviderID <= 0 || request.ExpectedVersion == 0 {
		return normalizedUpdateRequest{}, domainsandbox.ErrInvalidInput
	}
	scopes, err := domainsandbox.NormalizeScopes(request.Scopes)
	if err != nil {
		return normalizedUpdateRequest{}, err
	}
	policy, err := domainsandbox.NormalizeRuntimePolicy(request.Policy)
	if err != nil {
		return normalizedUpdateRequest{}, err
	}
	endpoint, err := normalizeSecretMutation(request.Endpoint)
	if err != nil {
		return normalizedUpdateRequest{}, err
	}
	credential, err := normalizeSecretMutation(request.Credential)
	if err != nil {
		wipeApplicationBytes(endpoint.Value)
		return normalizedUpdateRequest{}, err
	}
	return normalizedUpdateRequest{
		providerID: request.ProviderID, expectedVersion: request.ExpectedVersion, name: strings.TrimSpace(request.Name),
		scopes: scopes, policy: policy, endpoint: endpoint, credential: credential,
	}, nil
}

func normalizeSecretMutation(input SecretMutation) (SecretMutation, error) {
	mode := input.Mode
	if mode == "" {
		mode = SecretMutationKeep
	}
	switch mode {
	case SecretMutationKeep, SecretMutationClear:
		if len(input.Value) != 0 {
			return SecretMutation{}, domainsandbox.ErrInvalidInput
		}
		return SecretMutation{Mode: mode}, nil
	case SecretMutationReplace:
		if len(input.Value) == 0 {
			return SecretMutation{}, domainsandbox.ErrInvalidInput
		}
		return SecretMutation{Mode: mode, Value: append([]byte(nil), input.Value...)}, nil
	default:
		return SecretMutation{}, domainsandbox.ErrInvalidInput
	}
}

func (s *Service) resolveEndpointMutation(current *domainsandbox.Provider, mutation SecretMutation) (string, string, bool, error) {
	switch mutation.Mode {
	case SecretMutationKeep:
		return current.EndpointSecret, current.EndpointHint, false, nil
	case SecretMutationClear:
		if current.Type != domainsandbox.ProviderTypeLocalDebug {
			return "", "", false, domainsandbox.ErrInvalidInput
		}
		return "", "", current.EndpointSecret != "" || current.EndpointHint != "", nil
	case SecretMutationReplace:
		if current.Type != domainsandbox.ProviderTypeRemoteHTTP {
			return "", "", false, domainsandbox.ErrInvalidInput
		}
		endpoint, hint, err := normalizeRemoteControlPlaneEndpoint(mutation.Value)
		if err != nil {
			return "", "", false, err
		}
		defer wipeApplicationBytes(endpoint)
		envelope, err := s.codec.Encrypt(current.ProviderKey, infrasandbox.CredentialFieldEndpoint, endpoint)
		if err != nil {
			return "", "", false, domainsandbox.ErrConfigurationInvalid
		}
		return envelope, hint, true, nil
	default:
		return "", "", false, domainsandbox.ErrInvalidInput
	}
}

func (s *Service) resolveCredentialMutation(current *domainsandbox.Provider, mutation SecretMutation) (string, string, bool, error) {
	switch mutation.Mode {
	case SecretMutationKeep:
		return current.CredentialSecret, current.CredentialFingerprint, false, nil
	case SecretMutationClear:
		if current.Status != domainsandbox.ProviderStatusDisabled {
			return "", "", false, domainsandbox.ErrProviderInUse
		}
		return "", "", current.CredentialSecret != "" || current.CredentialFingerprint != "", nil
	case SecretMutationReplace:
		if current.Type != domainsandbox.ProviderTypeRemoteHTTP || !validControlPlaneCredential(mutation.Value) {
			return "", "", false, domainsandbox.ErrInvalidInput
		}
		envelope, err := s.codec.Encrypt(current.ProviderKey, infrasandbox.CredentialFieldCredential, mutation.Value)
		if err != nil {
			return "", "", false, domainsandbox.ErrConfigurationInvalid
		}
		fingerprint, err := s.codec.FingerprintCredential(mutation.Value)
		if err != nil {
			return "", "", false, domainsandbox.ErrConfigurationInvalid
		}
		return envelope, fingerprint, true, nil
	default:
		return "", "", false, domainsandbox.ErrInvalidInput
	}
}

func normalizeRemoteControlPlaneEndpoint(input []byte) ([]byte, string, error) {
	if len(input) == 0 || len(input) > infrasandbox.MaxEndpointPlaintextBytes || !utf8.Valid(input) {
		return nil, "", domainsandbox.ErrInvalidInput
	}
	value := string(input)
	hint, err := DeriveEndpointHint(value)
	if err != nil {
		return nil, "", domainsandbox.ErrInvalidInput
	}
	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, "", domainsandbox.ErrInvalidInput
	}
	hostname := parsed.Hostname()
	if hostname == "" || strings.EqualFold(hostname, "localhost") {
		return nil, "", domainsandbox.ErrInvalidInput
	}
	if ip := net.ParseIP(hostname); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()) {
		return nil, "", domainsandbox.ErrInvalidInput
	}
	parsed.Scheme = "https"
	parsed.Host = strings.ToLower(parsed.Host)
	canonical := parsed.String()
	if parsed.Path == "/" {
		canonical = strings.TrimSuffix(canonical, "/")
	}
	return []byte(canonical), hint, nil
}

func validControlPlaneCredential(input []byte) bool {
	if len(input) == 0 || len(input) > infrasandbox.MaxCredentialPlaintextBytes {
		return false
	}
	for _, value := range input {
		if value < 0x21 || value > 0x7e {
			return false
		}
	}
	return true
}

func validateControlPlaneActor(actor Actor, write bool) error {
	if actor.UserID <= 0 || !actor.SystemAdmin {
		return ErrPermissionDenied
	}
	if _, err := domainsandbox.NormalizeScopes(actor.AllowedScopes); err != nil {
		return ErrPermissionDenied
	}
	if write && !validControlPlaneRequestID(actor.RequestID) {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func authorizeControlPlaneScopes(actor Actor, scopes []domainsandbox.Scope) error {
	allowedScopes, err := domainsandbox.NormalizeScopes(actor.AllowedScopes)
	if err != nil {
		return ErrPermissionDenied
	}
	requestedScopes, err := domainsandbox.NormalizeScopes(scopes)
	if err != nil {
		return ErrPermissionDenied
	}
	allowed := make(map[domainsandbox.Scope]struct{}, len(allowedScopes))
	for _, scope := range allowedScopes {
		allowed[scope] = struct{}{}
	}
	for _, scope := range requestedScopes {
		if _, exists := allowed[scope]; !exists {
			return ErrPermissionDenied
		}
	}
	return nil
}

func validControlPlaneRequestID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxControlPlaneRequestIDBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func controlPlaneChangedFields(current *domainsandbox.Provider, input domainsandbox.UpdateProviderInput, endpointChanged, credentialChanged bool) []string {
	fields := make([]string, 0, 6)
	if current.Name != input.Name {
		fields = append(fields, auditChangedFieldName)
	}
	if !reflect.DeepEqual(current.Scopes, input.Scopes) {
		fields = append(fields, auditChangedFieldScope)
	}
	if !domainsandbox.PublicRuntimePoliciesEqual(current.Policy, input.Policy) {
		fields = append(fields, auditChangedFieldPolicy)
	}
	if endpointChanged {
		fields = append(fields, auditChangedFieldEndpoint)
	}
	if credentialChanged {
		fields = append(fields, auditChangedFieldCredential)
	}
	return fields
}

func (s *Service) projectControlPlaneProvider(provider *domainsandbox.Provider) (*ProviderDTO, error) {
	if provider == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	secrets, err := s.projectControlPlaneSecrets(provider)
	if err != nil {
		return nil, err
	}
	latencyBucket := healthLatencyBucketUnknown
	if provider.Health.Status != domainsandbox.HealthStatusUnknown {
		latencyBucket = boundedHealthLatencyBucket(provider.Health.LatencyMillis)
	}
	return &ProviderDTO{
		ID: provider.ID, Name: provider.Name, Type: provider.Type,
		EndpointHint:          secrets.EndpointHint,
		CredentialConfigured:  secrets.CredentialConfigured,
		CredentialFingerprint: secrets.CredentialFingerprint,
		Active:                secrets.Active,
		NeedsRewrap:           secrets.NeedsRewrap,
		Scopes:                append([]domainsandbox.Scope(nil), provider.Scopes...), Policy: cloneControlPlanePolicy(provider.Policy),
		Status: provider.Status, Health: HealthProjection{
			Status: provider.Health.Status, Capabilities: append([]domainsandbox.Scope(nil), provider.Health.Capabilities...),
			ReasonCode:    sanitizeControlPlaneText(provider.Health.ReasonCode, 64),
			Message:       sanitizeControlPlaneText(provider.Health.Message, domainsandbox.MaxHealthMessageLength),
			LatencyBucket: latencyBucket, CheckedAt: provider.Health.CheckedAt,
		},
		Version: provider.Version, CreatedAt: provider.CreatedAt, UpdatedAt: provider.UpdatedAt,
	}, nil
}

func (s *Service) projectControlPlaneSecrets(provider *domainsandbox.Provider) (SecretProjection, error) {
	if s == nil || s.codec == nil || provider == nil {
		return SecretProjection{}, domainsandbox.ErrUnavailable
	}
	input := SecretProjectionInput{
		EndpointHint:          provider.EndpointHint,
		CredentialFingerprint: provider.CredentialFingerprint,
	}
	if provider.EndpointSecret != "" {
		metadata, err := s.codec.Inspect(provider.EndpointSecret)
		if err != nil {
			return SecretProjection{}, domainsandbox.ErrUnavailable
		}
		input.EndpointEnvelope = &SecretEnvelopeInput{
			Active: metadata.Active, NeedsRewrap: metadata.NeedsRewrap,
		}
	}
	if provider.CredentialSecret != "" {
		metadata, err := s.codec.Inspect(provider.CredentialSecret)
		if err != nil {
			return SecretProjection{}, domainsandbox.ErrUnavailable
		}
		input.CredentialEnvelope = &SecretEnvelopeInput{
			Active: metadata.Active, NeedsRewrap: metadata.NeedsRewrap,
		}
	}
	projection, err := ProjectSecrets(input)
	if err != nil {
		return SecretProjection{}, domainsandbox.ErrUnavailable
	}
	return projection, nil
}

func sanitizeControlPlaneText(value string, maxBytes int) string {
	if value == "" {
		return ""
	}
	if len(value) > maxBytes || !utf8.ValidString(value) {
		return ""
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return ""
		}
	}
	return value
}

func cloneControlPlaneProvider(input *domainsandbox.Provider) *domainsandbox.Provider {
	if input == nil {
		return nil
	}
	clone := *input
	clone.Scopes = append([]domainsandbox.Scope(nil), input.Scopes...)
	clone.Policy = cloneControlPlanePolicy(input.Policy)
	clone.Health.Capabilities = append([]domainsandbox.Scope(nil), input.Health.Capabilities...)
	if input.DeletedAt != nil {
		deletedAt := *input.DeletedAt
		clone.DeletedAt = &deletedAt
	}
	return &clone
}

func cloneControlPlanePolicy(input domainsandbox.RuntimePolicy) domainsandbox.RuntimePolicy {
	clone := input
	clone.NetworkAllowlist = append([]string(nil), input.NetworkAllowlist...)
	clone.AllowedEnvNames = append([]string(nil), input.AllowedEnvNames...)
	clone.VirtualReadPrefixes = append([]string(nil), input.VirtualReadPrefixes...)
	clone.VirtualWritePrefixes = append([]string(nil), input.VirtualWritePrefixes...)
	clone.AllowedExecutables = append([]string(nil), input.AllowedExecutables...)
	clone.AllowEnv = append([]string(nil), input.AllowEnv...)
	clone.AllowRead = append([]string(nil), input.AllowRead...)
	clone.AllowWrite = append([]string(nil), input.AllowWrite...)
	clone.AllowRun = append([]string(nil), input.AllowRun...)
	clone.AllowFFI = append([]string(nil), input.AllowFFI...)
	return clone
}

func controlPlaneContainsScope(scopes []domainsandbox.Scope, target domainsandbox.Scope) bool {
	for _, scope := range scopes {
		if scope == target {
			return true
		}
	}
	return false
}

func stableControlPlaneError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrPermissionDenied) {
		return ErrPermissionDenied
	}
	for _, safe := range []error{
		domainsandbox.ErrInvalidInput,
		domainsandbox.ErrProviderNotFound,
		domainsandbox.ErrProviderDisabled,
		domainsandbox.ErrDefaultMissing,
		domainsandbox.ErrProviderUnhealthy,
		domainsandbox.ErrCredentialInvalid,
		domainsandbox.ErrCapacityExhausted,
		domainsandbox.ErrExecutionForbidden,
		domainsandbox.ErrVersionConflict,
		domainsandbox.ErrProviderAlreadyExists,
		domainsandbox.ErrProviderInUse,
		domainsandbox.ErrScopeUnsupported,
		domainsandbox.ErrConfigurationInvalid,
		domainsandbox.ErrLocalDebugUnavailable,
		domainsandbox.ErrUnavailable,
	} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return domainsandbox.ErrUnavailable
}

func wipeApplicationBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
