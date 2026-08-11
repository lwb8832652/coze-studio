// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

var ErrPermissionDenied = errors.New("sandbox control-plane permission denied")

type Actor struct {
	UserID        int64
	RequestID     string
	SystemAdmin   bool
	AllowedScopes []domainsandbox.Scope
}

type SecretMutationMode string

const (
	SecretMutationKeep    SecretMutationMode = "keep"
	SecretMutationReplace SecretMutationMode = "replace"
	SecretMutationClear   SecretMutationMode = "clear"
)

type SecretMutation struct {
	Mode  SecretMutationMode
	Value []byte
}

type CreateProviderRequest struct {
	Name       string
	Type       domainsandbox.ProviderType
	Endpoint   []byte
	Credential []byte
	Scopes     []domainsandbox.Scope
	Policy     domainsandbox.RuntimePolicy
}

type UpdateProviderRequest struct {
	ProviderID      int64
	ExpectedVersion uint64
	Name            string
	Scopes          []domainsandbox.Scope
	Policy          domainsandbox.RuntimePolicy
	Endpoint        SecretMutation
	Credential      SecretMutation
}

type ListProvidersRequest struct {
	Keyword string
	Type    domainsandbox.ProviderType
	Status  domainsandbox.ProviderStatus
	Health  domainsandbox.HealthStatus
	Scope   domainsandbox.Scope
	Offset  int
	Limit   int
}

type SetProviderStatusRequest struct {
	ProviderID      int64
	ExpectedVersion uint64
	Status          domainsandbox.ProviderStatus
}

type SetProviderDefaultRequest struct {
	ProviderID              int64
	ProviderExpectedVersion uint64
	Scope                   domainsandbox.Scope
	ExpectedVersion         uint64
}

type DeleteProviderRequest struct {
	ProviderID      int64
	ExpectedVersion uint64
}

type HealthCheckRequest struct {
	ProviderID      int64
	ExpectedVersion uint64
}

type ListAuditEventsRequest struct {
	ProviderID int64
	Action     string
	Result     string
	Offset     int
	Limit      int
}

type HealthProjection struct {
	Status        domainsandbox.HealthStatus `json:"status"`
	Capabilities  []domainsandbox.Scope      `json:"capabilities"`
	ReasonCode    string                     `json:"reason_code"`
	Message       string                     `json:"message"`
	LatencyBucket string                     `json:"latency_bucket"`
	CheckedAt     time.Time                  `json:"checked_at"`
}

type ProviderDTO struct {
	ID                    int64                        `json:"id"`
	Name                  string                       `json:"name"`
	Type                  domainsandbox.ProviderType   `json:"type"`
	EndpointHint          string                       `json:"endpoint_hint"`
	CredentialConfigured  bool                         `json:"credential_configured"`
	CredentialFingerprint string                       `json:"credential_fingerprint"`
	Active                bool                         `json:"active"`
	NeedsRewrap           bool                         `json:"needs_rewrap"`
	Scopes                []domainsandbox.Scope        `json:"scopes"`
	Policy                domainsandbox.RuntimePolicy  `json:"policy"`
	Status                domainsandbox.ProviderStatus `json:"status"`
	Health                HealthProjection             `json:"health"`
	Version               uint64                       `json:"version"`
	CreatedAt             time.Time                    `json:"created_at"`
	UpdatedAt             time.Time                    `json:"updated_at"`
}

type ListProvidersResult struct {
	Items []ProviderDTO `json:"items"`
	Total int64         `json:"total"`
}

type ProviderMutationResult struct {
	ProviderID int64  `json:"provider_id"`
	Version    uint64 `json:"version"`
}

type ProviderDefaultDTO struct {
	Scope      domainsandbox.Scope `json:"scope"`
	ProviderID int64               `json:"provider_id"`
	Version    uint64              `json:"version"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

type ProviderDefaultProjectionDTO struct {
	Scope           domainsandbox.Scope          `json:"scope"`
	Configured      bool                         `json:"configured"`
	ProviderID      *int64                       `json:"provider_id"`
	ProviderName    string                       `json:"provider_name"`
	ProviderType    domainsandbox.ProviderType   `json:"provider_type"`
	ProviderStatus  domainsandbox.ProviderStatus `json:"provider_status"`
	Version         uint64                       `json:"version"`
	ProviderVersion uint64                       `json:"provider_version"`
	UpdatedAt       *time.Time                   `json:"updated_at"`
}

type ListProviderDefaultsResult struct {
	Items []ProviderDefaultProjectionDTO `json:"items"`
}

type ProviderSummaryDTO struct {
	Total     int64 `json:"total"`
	Enabled   int64 `json:"enabled"`
	Unhealthy int64 `json:"unhealthy"`
}

type CapabilityStateDTO struct {
	Available  bool   `json:"available"`
	ReasonCode string `json:"reason_code"`
	Message    string `json:"message"`
}

type SandboxCapabilitiesDTO struct {
	ControlPlane CapabilityStateDTO `json:"control_plane"`
	LocalDebug   CapabilityStateDTO `json:"local_debug"`
}

type AuditEventDTO struct {
	ID          int64             `json:"id"`
	ProviderID  int64             `json:"provider_id"`
	ActorUserID int64             `json:"actor_user_id"`
	Action      string            `json:"action"`
	Result      string            `json:"result"`
	RequestID   string            `json:"request_id"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
}

type ListAuditEventsResult struct {
	Items []AuditEventDTO `json:"items"`
	Total int64           `json:"total"`
}

type CredentialCodec interface {
	Encrypt(providerKey string, field infrasandbox.CredentialField, plaintext []byte) (string, error)
	FingerprintCredential(plaintext []byte) (string, error)
	Inspect(envelope string) (infrasandbox.CredentialEnvelopeMetadata, error)
}

type LeaseActivityChecker interface {
	HasActiveLeases(ctx context.Context, providerKey string) (bool, error)
}

// ProviderLifecycleGuard closes the lease-check/mutation race without leaking
// Redis representation into the control plane.
type ProviderLifecycleGuard interface {
	BeginDrain(ctx context.Context, providerKey string) (DrainHandle, error)
	// RestoreActive publishes current as the new active lifecycle epoch. It
	// never restores an older generation observed before BeginDrain.
	RestoreActive(ctx context.Context, providerKey string, current DrainFence) error
	// RetainDrain verifies that current still owns the drain gate and leaves it
	// persistent. A superseded token is a stale no-op.
	RetainDrain(ctx context.Context, providerKey string, current DrainFence) error
	Activate(ctx context.Context, providerKey string, current DrainFence) error
	CompensateActivation(
		ctx context.Context,
		providerKey string,
		current DrainFence,
	) (ActivationCompensationResult, error)
}

type HealthProvider interface {
	Health(ctx context.Context) (infrasandbox.HealthResult, error)
	CloseContext(ctx context.Context) error
}

type HealthProviderFactory interface {
	Build(ctx context.Context, provider *domainsandbox.Provider) (HealthProvider, error)
}

type ServiceOptions struct {
	Providers    domainsandbox.ProviderRepository
	UnitOfWork   domainsandbox.ProviderCreateUnitOfWork
	Management   domainsandbox.ProviderManagementRepository
	Capabilities CapabilityPolicy
	Codec        CredentialCodec
	Leases       ProviderLifecycleGuard
	Factory      HealthProviderFactory
	Metrics      SandboxMetricsRecorder
	ProviderKey  func(Actor) (string, error)
	Now          func() time.Time
}

const SchedulerReasonProviderUnavailable = "PROVIDER_UNAVAILABLE"

type SchedulerSettingsDTO struct {
	Version  uint64                          `json:"version"`
	Settings domainsandbox.SchedulerSettings `json:"settings"`
}

type UpdateSchedulerSettingsRequest struct {
	ExpectedVersion uint64
	Settings        domainsandbox.SchedulerSettings
}

type SchedulerSettingsUpdateResult struct {
	Version    uint64                          `json:"version"`
	Settings   domainsandbox.SchedulerSettings `json:"settings"`
	Applied    bool                            `json:"applied"`
	ReasonCode string                          `json:"reason_code,omitempty"`
}

type NativeRunnerStatus struct {
	Healthy              bool
	AppliedConfigVersion uint64
	QueueDepth           int
	ActiveSlots          int
	SlotCapacity         int
	QueueHighWatermark   int
	DrainingCount        int
	QuarantinedCount     int
	ReasonCode           string
}

type SchedulerRuntimeStatusDTO struct {
	Available            bool   `json:"available"`
	DesiredConfigVersion uint64 `json:"desired_config_version"`
	AppliedConfigVersion uint64 `json:"applied_config_version"`
	QueueDepth           int    `json:"queue_depth"`
	ActiveSlots          int    `json:"active_slots"`
	SlotCapacity         int    `json:"slot_capacity"`
	QueueHighWatermark   int    `json:"queue_high_watermark"`
	DrainingCount        int    `json:"draining_count"`
	QuarantinedCount     int    `json:"quarantined_count"`
	ReasonCode           string `json:"reason_code,omitempty"`
}

// NativeSchedulerRunner is deliberately narrow: Task 3 only persists desired
// settings and asks the native Runner to apply/project safe aggregates.
type NativeSchedulerRunner interface {
	ApplySchedulerSettings(context.Context, domainsandbox.SchedulerSettings) error
	RuntimeStatus(context.Context) (NativeRunnerStatus, error)
}

type SchedulerServiceOptions struct {
	Store  domainsandbox.SchedulerSettingsAuditRepository
	Runner NativeSchedulerRunner
}
