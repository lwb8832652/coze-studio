/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sandbox

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MinPageLimit                 = 1
	DefaultPageLimit             = 20
	MaxPageLimit                 = 100
	MaxPageOffset                = 100000
	MaxProviderListKeywordLength = 128

	ProviderListStableOrder        = "created_at_desc_id_desc"
	ProviderAuditListStableOrder   = "created_at_desc_id_desc"
	ProviderDefaultListStableOrder = "scope_asc"
)

// ProviderListRequest advances Offset from newest to oldest under
// ProviderListStableOrder. Any cursor projection must preserve that direction.
type ProviderListRequest struct {
	Keyword          string
	Type             ProviderType
	Status           ProviderStatus
	Health           HealthStatus
	Scope            Scope
	AuthorizedScopes []Scope
	Offset           int
	Limit            int
}

// ProviderAuditListRequest advances Offset from newest to oldest under
// ProviderAuditListStableOrder. Any cursor projection must preserve that
// direction.
type ProviderAuditListRequest struct {
	ProviderID int64
	Action     string
	Result     string
	Offset     int
	Limit      int
}

func NormalizeProviderListRequest(request ProviderListRequest) (ProviderListRequest, error) {
	offset, limit, err := normalizePage(request.Offset, request.Limit)
	if err != nil {
		return ProviderListRequest{}, err
	}
	if request.Type != "" && !validProviderType(request.Type) {
		return ProviderListRequest{}, ErrInvalidInput
	}
	if request.Status != "" && !validProviderStatus(request.Status) {
		return ProviderListRequest{}, ErrInvalidInput
	}
	if request.Health != "" && !validHealthStatus(request.Health) {
		return ProviderListRequest{}, ErrInvalidInput
	}
	if request.Scope != "" && !validScope(request.Scope) {
		return ProviderListRequest{}, ErrInvalidInput
	}
	var authorizedScopes []Scope
	if len(request.AuthorizedScopes) != 0 {
		authorizedScopes, err = NormalizeScopes(request.AuthorizedScopes)
		if err != nil {
			return ProviderListRequest{}, err
		}
	}
	keyword := strings.TrimSpace(request.Keyword)
	if !utf8.ValidString(keyword) || len(keyword) > MaxProviderListKeywordLength || containsControl(keyword) {
		return ProviderListRequest{}, ErrInvalidInput
	}
	normalized := request
	normalized.Keyword = keyword
	normalized.AuthorizedScopes = authorizedScopes
	normalized.Offset = offset
	normalized.Limit = limit
	return normalized, nil
}

func NormalizeProviderAuditListRequest(request ProviderAuditListRequest) (ProviderAuditListRequest, error) {
	offset, limit, err := normalizePage(request.Offset, request.Limit)
	if err != nil || request.ProviderID < 0 {
		return ProviderAuditListRequest{}, ErrInvalidInput
	}
	action, err := normalizeOptionalRepositoryFilter(request.Action, MaxAuditActionLength)
	if err != nil {
		return ProviderAuditListRequest{}, err
	}
	result, err := normalizeOptionalRepositoryFilter(request.Result, MaxAuditResultLength)
	if err != nil {
		return ProviderAuditListRequest{}, err
	}
	normalized := request
	normalized.Action = action
	normalized.Result = result
	normalized.Offset = offset
	normalized.Limit = limit
	return normalized, nil
}

func normalizePage(offset, limit int) (int, int, error) {
	if offset < 0 || offset > MaxPageOffset || limit < 0 || limit > MaxPageLimit {
		return 0, 0, ErrInvalidInput
	}
	if limit == 0 {
		limit = DefaultPageLimit
	}
	if limit < MinPageLimit {
		return 0, 0, ErrInvalidInput
	}
	return offset, limit, nil
}

func normalizeOptionalRepositoryFilter(value string, maxLength int) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || len(value) > maxLength || containsControl(value) {
		return "", ErrInvalidInput
	}
	return value, nil
}

// ProviderRepository owns persisted numeric IDs, UTC timestamps, soft-deletion
// state, and version assignment. CreateProvider returns InitialVersion. Every
// CAS mutation returns ExpectedVersion+1 and never rewrites ProviderKey.
//
// Get methods return ErrProviderNotFound when no live row exists. CAS methods
// return ErrProviderNotFound when the target does not exist and
// ErrVersionConflict when it exists with a different version.
// DeleteProvider returns ErrProviderInUse when, after existence and version
// checks succeed, any default still references the provider.
//
// Inputs and returned entities are detached values: implementations must clone
// scopes, health capabilities, and policy allowlists at repository boundaries.
// Every mutator must invoke its mandatory domain boundary before persistence:
// NormalizeCreateProviderInput, NormalizeUpdateProviderInput,
// NormalizeUpdateProviderStatusInput, NormalizeUpdateProviderHealthInput, or
// ValidateDeleteProviderInput. CreateProvider maps a provider_key uniqueness
// conflict to ErrProviderAlreadyExists.
// ListProviders always uses ProviderListStableOrder after request normalization.
type ProviderRepository interface {
	CreateProvider(ctx context.Context, input CreateProviderInput) (*Provider, error)
	GetProvider(ctx context.Context, providerID int64) (*Provider, error)
	// GetProviderForUpdate must run on the UnitOfWork transaction and hold a
	// row lock until that transaction commits or rolls back.
	GetProviderForUpdate(ctx context.Context, providerID int64) (*Provider, error)
	GetProviderByKey(ctx context.Context, providerKey string) (*Provider, error)
	ListProviders(ctx context.Context, request ProviderListRequest) ([]*Provider, int64, error)
	UpdateProvider(ctx context.Context, input UpdateProviderInput) (*Provider, error)
	UpdateProviderStatus(ctx context.Context, input UpdateProviderStatusInput) (nextVersion uint64, err error)
	UpdateProviderHealth(ctx context.Context, input UpdateProviderHealthInput) (nextVersion uint64, err error)
	DeleteProvider(ctx context.Context, input DeleteProviderInput) (nextVersion uint64, err error)
}

// ProviderDefaultRepository owns UTC timestamps and versions for defaults.
// SetProviderDefault must invoke NormalizeSetProviderDefaultInput with the
// current row inside the transaction before persistence. ExpectedVersion=0
// creates a missing scope once and returns InitialVersion. Existing rows require
// their current positive version and return ExpectedVersion+1. A missing row
// with a positive expected version returns ErrDefaultMissing; an existing row
// with a stale or zero version returns ErrVersionConflict. Defaults cannot be
// deleted or recreated, so versions never reset.
//
// Returned defaults are detached values. ListProviderDefaults always uses
// ProviderDefaultListStableOrder.
type ProviderDefaultRepository interface {
	GetProviderDefault(ctx context.Context, scope Scope) (*ProviderDefault, error)
	ListProviderDefaults(ctx context.Context) ([]*ProviderDefault, error)
	SetProviderDefault(ctx context.Context, input SetProviderDefaultInput) (*ProviderDefault, error)
}

// SchedulerSettingsRepository owns the singleton scheduler snapshot. Returned
// settings are detached values. UpdateSchedulerSettingsCAS initializes no
// additional rows and changes the version exactly once on success.
type SchedulerSettingsRepository interface {
	GetSchedulerSettings(ctx context.Context) (SchedulerSettings, error)
	UpdateSchedulerSettingsCAS(ctx context.Context, input UpdateSchedulerSettingsInput) (SchedulerSettings, error)
}

// SessionSettingsRepository owns the independent Session runtime snapshot.
// It never reads or changes the legacy Scheduler settings version.
type SessionSettingsRepository interface {
	GetSessionSettings(ctx context.Context) (SessionRuntimeSettings, error)
	UpdateSessionSettingsCAS(ctx context.Context, input UpdateSessionSettingsInput) (SessionRuntimeSettings, error)
}

type SessionSettingsAuditRepository interface {
	SessionSettingsRepository
	AppendSessionSettingsAuditEvent(ctx context.Context, input AppendSchedulerAuditEventInput) (*SchedulerAuditEvent, error)
	UpdateSessionSettingsCASWithAudit(ctx context.Context, input UpdateSessionSettingsInput, audit AppendSchedulerAuditEventInput) (SessionRuntimeSettings, error)
}

// RuntimeSessionRepository owns the stable Session business key and its
// optimistic lifecycle. Returned values are detached and never contain a
// physical workspace path.
type RuntimeSessionRepository interface {
	AcquireRuntimeSession(ctx context.Context, input AcquireRuntimeSessionInput) (RuntimeSession, error)
	GetRuntimeSession(ctx context.Context, ref SessionRef) (RuntimeSession, error)
	// GetRuntimeSessionByKey resolves an existing persisted generation while
	// still binding the opaque session ID to its complete canonical tenant key.
	GetRuntimeSessionByKey(ctx context.Context, sessionID string, key SessionKey) (RuntimeSession, error)
	BindRuntimeSessionCAS(ctx context.Context, input BindRuntimeSessionInput) (RuntimeSession, error)
	TransitionRuntimeSessionCAS(ctx context.Context, input TransitionRuntimeSessionInput) (RuntimeSession, error)
	ListRecoverableRuntimeSessions(ctx context.Context, input ListRecoverableRuntimeSessionsInput) ([]RuntimeSession, error)
}

// AIOGenerationRepository is the single MySQL linearization point for a raw
// AIO sentinel replacement and generation change.
type AIOGenerationRepository interface {
	GetAIOGeneration(ctx context.Context, deploymentID string) (AIOGenerationState, error)
	CompareAndReplaceAIOSentinel(ctx context.Context, input CompareAndReplaceAIOSentinelInput) (AIOGenerationState, bool, error)
}

type SchedulerAuditRepository interface {
	AppendSchedulerAuditEvent(ctx context.Context, input AppendSchedulerAuditEventInput) (*SchedulerAuditEvent, error)
}

// SchedulerSettingsAuditRepository preserves the desired scheduler snapshot
// and its successful audit row in one transaction.
type SchedulerSettingsAuditRepository interface {
	SchedulerSettingsRepository
	SchedulerAuditRepository
	UpdateSchedulerSettingsCASWithAudit(ctx context.Context, input UpdateSchedulerSettingsInput, audit AppendSchedulerAuditEventInput) (SchedulerSettings, error)
}

type ProviderSummary struct {
	Total     int64
	Enabled   int64
	Unhealthy int64
}

// ProviderManagementRepository exposes only safe read projections needed by
// system management. Implementations exclude soft-deleted rows and enforce
// authorized scope boundaries in the database query.
type ProviderManagementRepository interface {
	ListProviderDefaults(ctx context.Context) ([]*ProviderDefault, error)
	SummarizeProviders(ctx context.Context, authorizedScopes []Scope) (ProviderSummary, error)
}

// ProviderAuditRepository owns audit IDs and UTC CreatedAt timestamps. Metadata
// and the complete append command must pass
// NormalizeAppendProviderAuditEventInput and must be cloned on write and read.
// Audit events are append-only. ListProviderAuditEvents always moves from
// newest to oldest using ProviderAuditListStableOrder.
type ProviderAuditRepository interface {
	AppendProviderAuditEvent(ctx context.Context, input AppendProviderAuditEventInput) (*ProviderAuditEvent, error)
	ListProviderAuditEvents(ctx context.Context, request ProviderAuditListRequest) ([]*ProviderAuditEvent, int64, error)
}

// TransactionRepositories contains repositories bound to one infrastructure
// transaction. Implementations must not allow these bindings to escape the
// callback.
type TransactionRepositories struct {
	Providers ProviderRepository
	Defaults  ProviderDefaultRepository
	Audits    ProviderAuditRepository
}

// UnitOfWork invokes one callback with Provider, Default, and Audit
// repositories bound to the same transaction. It commits only when the
// callback returns nil and rolls back when the callback returns an error. It
// must not retry or invoke the callback more than once.
type UnitOfWork interface {
	WithinTransaction(
		ctx context.Context,
		callback func(context.Context, TransactionRepositories) error,
	) error
}

// ProviderCreateUnitOfWork serializes transactions that can change whether
// the provider table is empty. Implementations must use one database-scoped,
// cross-process lock shared by normal provider creation and legacy import,
// and release it only after commit succeeds or rollback completes.
type ProviderCreateUnitOfWork interface {
	UnitOfWork
	WithinProviderCreateTransaction(
		ctx context.Context,
		callback func(context.Context, TransactionRepositories) error,
	) error
}

const MaxHealthMonitorBatchSize = 100

type HealthMonitorClaimRequest struct {
	WorkerID string
	Now      time.Time
	Lease    time.Duration
	Limit    int
}

type HealthMonitorClaim struct {
	Provider       *Provider
	LeaseOwner     string
	LeaseToken     string
	ClaimedAt      time.Time
	LeaseExpiresAt time.Time
}

type CompleteHealthMonitorCheckInput struct {
	Claim            HealthMonitorClaim
	Health           HealthSnapshot
	CheckedAt        time.Time
	NextCheckAt      time.Time
	FailureThreshold int
}

type HealthMonitorCheckCompletion struct {
	Applied      bool
	Episode      ProviderHealthEpisode
	Notification HealthIncidentNotification
}

type HealthNotificationProjectionRequest struct {
	WorkerID string
	Now      time.Time
	Lease    time.Duration
	Limit    int
}

type HealthNotificationProjectionResult struct {
	Claimed   int
	Projected int
}

// HealthMonitorRepository owns cross-process claim leases and atomically
// persists provider health, episode transitions, and durable notification
// projections. Projection appends outbox rows and marks projections complete in
// a later transaction. A stale provider version may release its claim but must
// still return ErrVersionConflict explicitly.
type HealthMonitorRepository interface {
	ReconcileDisabledHealthEpisodes(
		ctx context.Context,
		now time.Time,
		limit int,
	) (int64, error)
	ClaimHealthChecks(
		ctx context.Context,
		request HealthMonitorClaimRequest,
	) ([]HealthMonitorClaim, error)
	CompleteHealthCheck(
		ctx context.Context,
		input CompleteHealthMonitorCheckInput,
	) (HealthMonitorCheckCompletion, error)
	ProjectPendingHealthNotifications(
		ctx context.Context,
		request HealthNotificationProjectionRequest,
	) (HealthNotificationProjectionResult, error)
}
