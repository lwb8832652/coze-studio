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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const maxProviderConcurrencyPersistence uint32 = math.MaxInt32

var (
	errInvalidPersistedScopeJSON         = errors.New("sandbox persisted scope JSON is invalid")
	errInvalidPersistedFeatureJSON       = errors.New("sandbox persisted feature JSON is invalid")
	errInvalidPersistedPolicyJSON        = errors.New("sandbox persisted policy JSON is invalid")
	errInvalidPersistedSchedulerJSON     = errors.New("sandbox persisted scheduler JSON is invalid")
	errInvalidPersistedMetadataJSON      = errors.New("sandbox persisted audit metadata JSON is invalid")
	errInvalidPersistedProjection        = errors.New("sandbox persisted provider projection is invalid")
	errInvalidPersistedNumericProjection = errors.New("sandbox persisted numeric projection is invalid")
)

type runtimePolicyDocument struct {
	TimeoutSeconds          int                           `json:"timeout_seconds"`
	MemoryLimitMB           int                           `json:"memory_limit_mb"`
	CPULimit                float64                       `json:"cpu_limit"`
	MaxOutputBytes          int64                         `json:"max_output_bytes"`
	MaxConcurrency          int                           `json:"max_concurrency"`
	AllowNetwork            bool                          `json:"allow_network"`
	NetworkAllowlist        []string                      `json:"network_allowlist"`
	AllowedEnvNames         []string                      `json:"allowed_env_names,omitempty"`
	VirtualReadPrefixes     []string                      `json:"virtual_read_prefixes,omitempty"`
	VirtualWritePrefixes    []string                      `json:"virtual_write_prefixes,omitempty"`
	AllowedExecutables      []string                      `json:"allowed_executables,omitempty"`
	FFIEnabled              bool                          `json:"ffi_enabled,omitempty"`
	NodeModulesMode         domainsandbox.NodeModulesMode `json:"node_modules_mode,omitempty"`
	NodeModulesDirectoryRef string                        `json:"node_modules_directory_ref,omitempty"`
	AllowEnv                []string                      `json:"allow_env,omitempty"`
	AllowRead               []string                      `json:"allow_read,omitempty"`
	AllowWrite              []string                      `json:"allow_write,omitempty"`
	AllowRun                []string                      `json:"allow_run,omitempty"`
	AllowFFI                []string                      `json:"allow_ffi,omitempty"`
	NodeModulesDir          string                        `json:"node_modules_dir,omitempty"`
}

func marshalScopes(scopes []domainsandbox.Scope) (string, error) {
	values := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		values = append(values, string(scope))
	}
	return marshalCanonicalJSON(values)
}

func unmarshalProviderScopes(raw string) ([]domainsandbox.Scope, error) {
	scopes, err := unmarshalScopes(raw)
	if err != nil {
		return nil, err
	}
	normalized, err := domainsandbox.NormalizeScopes(scopes)
	if err != nil {
		return nil, errInvalidPersistedScopeJSON
	}
	return normalized, nil
}

func unmarshalHealthCapabilities(raw string) ([]domainsandbox.Scope, error) {
	capabilities, err := unmarshalScopes(raw)
	if err != nil {
		return nil, err
	}
	if len(capabilities) == 0 {
		return []domainsandbox.Scope{}, nil
	}
	normalized, err := domainsandbox.NormalizeScopes(capabilities)
	if err != nil {
		return nil, errInvalidPersistedScopeJSON
	}
	return normalized, nil
}

func marshalProviderFeatures(features []domainsandbox.ProviderFeature) (string, error) {
	values := make([]string, 0, len(features))
	for _, feature := range features {
		values = append(values, string(feature))
	}
	return marshalCanonicalJSON(values)
}

func unmarshalProviderFeatures(raw string) ([]domainsandbox.ProviderFeature, error) {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return nil, errInvalidPersistedFeatureJSON
	}
	features := make([]domainsandbox.ProviderFeature, 0, len(values))
	for _, value := range values {
		features = append(features, domainsandbox.ProviderFeature(value))
	}
	normalized, err := domainsandbox.NormalizeProviderFeatures(features)
	if err != nil {
		return nil, errInvalidPersistedFeatureJSON
	}
	return normalized, nil
}

func marshalSchedulerSettings(settings domainsandbox.SchedulerSettings) (string, error) {
	normalized, err := domainsandbox.NormalizeSchedulerSettings(settings)
	if err != nil {
		return "", err
	}
	return marshalCanonicalJSON(normalized)
}

func unmarshalSchedulerSettings(raw string) (domainsandbox.SchedulerSettings, error) {
	settings, err := domainsandbox.DecodeSchedulerSettingsJSON([]byte(raw))
	if err != nil {
		return domainsandbox.SchedulerSettings{}, errInvalidPersistedSchedulerJSON
	}
	return settings, nil
}

func unmarshalScopes(raw string) ([]domainsandbox.Scope, error) {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil || values == nil {
		return nil, errInvalidPersistedScopeJSON
	}
	scopes := make([]domainsandbox.Scope, 0, len(values))
	for _, value := range values {
		scopes = append(scopes, domainsandbox.Scope(value))
	}
	return scopes, nil
}

func marshalRuntimePolicy(policy domainsandbox.RuntimePolicy) (string, error) {
	allowlist := make([]string, len(policy.NetworkAllowlist))
	copy(allowlist, policy.NetworkAllowlist)
	return marshalCanonicalJSON(runtimePolicyDocument{
		TimeoutSeconds:          policy.TimeoutSeconds,
		MemoryLimitMB:           policy.MemoryLimitMB,
		CPULimit:                policy.CPULimit,
		MaxOutputBytes:          policy.MaxOutputBytes,
		MaxConcurrency:          policy.MaxConcurrency,
		AllowNetwork:            policy.AllowNetwork,
		NetworkAllowlist:        allowlist,
		AllowedEnvNames:         append([]string(nil), policy.AllowedEnvNames...),
		VirtualReadPrefixes:     append([]string(nil), policy.VirtualReadPrefixes...),
		VirtualWritePrefixes:    append([]string(nil), policy.VirtualWritePrefixes...),
		AllowedExecutables:      append([]string(nil), policy.AllowedExecutables...),
		FFIEnabled:              policy.FFIEnabled,
		NodeModulesMode:         policy.NodeModulesMode,
		NodeModulesDirectoryRef: policy.NodeModulesDirectoryRef,
		AllowEnv:                append([]string(nil), policy.AllowEnv...),
		AllowRead:               append([]string(nil), policy.AllowRead...),
		AllowWrite:              append([]string(nil), policy.AllowWrite...),
		AllowRun:                append([]string(nil), policy.AllowRun...),
		AllowFFI:                append([]string(nil), policy.AllowFFI...),
		NodeModulesDir:          policy.NodeModulesDir,
	})
}

func unmarshalRuntimePolicy(raw string) (domainsandbox.RuntimePolicy, error) {
	var document runtimePolicyDocument
	if err := json.Unmarshal([]byte(raw), &document); err != nil {
		return domainsandbox.RuntimePolicy{}, errInvalidPersistedPolicyJSON
	}
	policy, err := domainsandbox.NormalizeRuntimePolicy(domainsandbox.RuntimePolicy{
		TimeoutSeconds:          document.TimeoutSeconds,
		MemoryLimitMB:           document.MemoryLimitMB,
		CPULimit:                document.CPULimit,
		MaxOutputBytes:          document.MaxOutputBytes,
		MaxConcurrency:          document.MaxConcurrency,
		AllowNetwork:            document.AllowNetwork,
		NetworkAllowlist:        append([]string(nil), document.NetworkAllowlist...),
		AllowedEnvNames:         append([]string(nil), document.AllowedEnvNames...),
		VirtualReadPrefixes:     append([]string(nil), document.VirtualReadPrefixes...),
		VirtualWritePrefixes:    append([]string(nil), document.VirtualWritePrefixes...),
		AllowedExecutables:      append([]string(nil), document.AllowedExecutables...),
		FFIEnabled:              document.FFIEnabled,
		NodeModulesMode:         document.NodeModulesMode,
		NodeModulesDirectoryRef: document.NodeModulesDirectoryRef,
		AllowEnv:                append([]string(nil), document.AllowEnv...),
		AllowRead:               append([]string(nil), document.AllowRead...),
		AllowWrite:              append([]string(nil), document.AllowWrite...),
		AllowRun:                append([]string(nil), document.AllowRun...),
		AllowFFI:                append([]string(nil), document.AllowFFI...),
		NodeModulesDir:          document.NodeModulesDir,
	})
	if err != nil {
		return domainsandbox.RuntimePolicy{}, errInvalidPersistedPolicyJSON
	}
	policy, err = domainsandbox.NormalizeLegacyRuntimePolicy(policy)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, errInvalidPersistedPolicyJSON
	}
	return policy, nil
}

func marshalAuditMetadata(metadata map[string]string) (string, error) {
	if metadata == nil {
		metadata = map[string]string{}
	}
	return marshalCanonicalJSON(metadata)
}

func unmarshalAuditMetadata(raw string) (map[string]string, error) {
	metadata := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil || metadata == nil {
		return nil, errInvalidPersistedMetadataJSON
	}
	normalized, err := domainsandbox.NormalizeAuditMetadata(metadata)
	if err != nil {
		return nil, errInvalidPersistedMetadataJSON
	}
	return normalized, nil
}

func marshalCanonicalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode sandbox JSON: %w", err)
	}
	return string(encoded), nil
}

func newProviderPO(input domainsandbox.CreateProviderInput, now time.Time) (*providerPO, error) {
	if err := domainsandbox.ValidateLegacySourceHash(input.LegacySourceHash); err != nil {
		return nil, err
	}
	actorUserID, err := positiveDomainInt64ToUint64(input.ActorUserID)
	if err != nil {
		return nil, err
	}
	maxConcurrency, err := providerMaxConcurrencyToPO(input.Policy.MaxConcurrency)
	if err != nil {
		return nil, err
	}
	scopesJSON, err := marshalScopes(input.Scopes)
	if err != nil {
		return nil, err
	}
	policyJSON, err := marshalRuntimePolicy(input.Policy)
	if err != nil {
		return nil, err
	}
	capabilitiesJSON, err := marshalScopes([]domainsandbox.Scope{})
	if err != nil {
		return nil, err
	}
	featuresJSON, err := marshalProviderFeatures([]domainsandbox.ProviderFeature{})
	if err != nil {
		return nil, err
	}
	return &providerPO{
		ProviderKey:                input.ProviderKey,
		Name:                       input.Name,
		ProviderType:               string(input.Type),
		EndpointSecret:             nullableString(input.EndpointSecret),
		EndpointHint:               input.EndpointHint,
		CredentialSecret:           nullableString(input.CredentialSecret),
		CredentialFingerprint:      input.CredentialFingerprint,
		LegacySourceHash:           nullableString(input.LegacySourceHash),
		ScopesJSON:                 scopesJSON,
		PolicyJSON:                 policyJSON,
		MaxConcurrency:             maxConcurrency,
		Status:                     string(domainsandbox.ProviderStatusDisabled),
		HealthStatus:               string(domainsandbox.HealthStatusUnknown),
		LastHealthCapabilitiesJSON: capabilitiesJSON,
		LastHealthFeaturesJSON:     featuresJSON,
		Version:                    domainsandbox.InitialVersion,
		CreatedBy:                  actorUserID,
		UpdatedBy:                  actorUserID,
		CreatedAt:                  now,
		UpdatedAt:                  now,
	}, nil
}

func (po *providerPO) toDomain() (*domainsandbox.Provider, error) {
	if po == nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	id, err := positivePersistedUint64ToInt64(po.ID)
	if err != nil {
		return nil, err
	}
	createdBy, err := positivePersistedUint64ToInt64(po.CreatedBy)
	if err != nil {
		return nil, err
	}
	updatedBy, err := positivePersistedUint64ToInt64(po.UpdatedBy)
	if err != nil {
		return nil, err
	}
	scopes, err := unmarshalProviderScopes(po.ScopesJSON)
	if err != nil {
		return nil, err
	}
	policy, err := unmarshalRuntimePolicy(po.PolicyJSON)
	if err != nil {
		return nil, err
	}
	maxConcurrency, err := providerMaxConcurrencyFromPO(po.MaxConcurrency)
	if err != nil {
		return nil, err
	}
	if policy.MaxConcurrency != maxConcurrency {
		return nil, errInvalidPersistedProjection
	}
	capabilities, err := unmarshalHealthCapabilities(po.LastHealthCapabilitiesJSON)
	if err != nil {
		return nil, err
	}
	features, err := unmarshalProviderFeatures(po.LastHealthFeaturesJSON)
	if err != nil {
		return nil, err
	}
	health := domainsandbox.HealthSnapshot{
		Status:        domainsandbox.HealthStatus(po.HealthStatus),
		Capabilities:  capabilities,
		Features:      features,
		ReasonCode:    po.LastHealthCode,
		Message:       po.LastHealthMessage,
		LatencyMillis: int64(po.LastHealthLatencyMS),
	}
	if po.LastHealthAt != nil {
		health.CheckedAt = po.LastHealthAt.UTC()
	}
	health, err = domainsandbox.NormalizeHealthSnapshot(health, scopes)
	if err != nil {
		return nil, errInvalidPersistedProjection
	}
	providerType := domainsandbox.ProviderType(po.ProviderType)
	endpointSecret := stringValue(po.EndpointSecret)
	credentialSecret := stringValue(po.CredentialSecret)
	if err := domainsandbox.ValidateProviderSecretPersistence(
		providerType,
		endpointSecret,
		po.EndpointHint,
		credentialSecret,
		po.CredentialFingerprint,
	); err != nil {
		return nil, err
	}
	return &domainsandbox.Provider{
		ID:                    id,
		ProviderKey:           po.ProviderKey,
		Name:                  po.Name,
		Type:                  providerType,
		EndpointSecret:        endpointSecret,
		EndpointHint:          po.EndpointHint,
		CredentialSecret:      credentialSecret,
		CredentialFingerprint: po.CredentialFingerprint,
		Scopes:                append([]domainsandbox.Scope(nil), scopes...),
		Policy:                cloneRuntimePolicy(policy),
		Status:                domainsandbox.ProviderStatus(po.Status),
		Health:                cloneHealthSnapshot(health),
		LegacySourceHash:      stringValue(po.LegacySourceHash),
		Version:               po.Version,
		CreatedBy:             createdBy,
		UpdatedBy:             updatedBy,
		CreatedAt:             po.CreatedAt.UTC(),
		UpdatedAt:             po.UpdatedAt.UTC(),
		DeletedAt:             cloneTime(po.DeletedAt),
	}, nil
}

func (po *providerDefaultPO) toDomain() (*domainsandbox.ProviderDefault, error) {
	if po == nil {
		return nil, nil
	}
	providerID, err := positivePersistedUint64ToInt64(po.ProviderID)
	if err != nil {
		return nil, err
	}
	updatedBy, err := positivePersistedUint64ToInt64(po.UpdatedBy)
	if err != nil {
		return nil, err
	}
	return &domainsandbox.ProviderDefault{
		Scope:      domainsandbox.Scope(po.Scope),
		ProviderID: providerID,
		Version:    po.Version,
		UpdatedBy:  updatedBy,
		CreatedAt:  po.CreatedAt.UTC(),
		UpdatedAt:  po.UpdatedAt.UTC(),
	}, nil
}

func (po *schedulerSettingsPO) toDomain() (domainsandbox.SchedulerSettings, error) {
	if po == nil || po.ID != 1 || po.Version < domainsandbox.InitialVersion || po.UpdatedBy > math.MaxInt64 {
		return domainsandbox.SchedulerSettings{}, errInvalidPersistedProjection
	}
	settings, err := unmarshalSchedulerSettings(po.SettingsJSON)
	if err != nil {
		return domainsandbox.SchedulerSettings{}, err
	}
	settings.Version = po.Version
	settings.UpdatedBy = int64(po.UpdatedBy)
	return settings, nil
}

func (po *providerAuditEventPO) toDomain() (*domainsandbox.ProviderAuditEvent, error) {
	if po == nil {
		return nil, fmt.Errorf("sandbox audit event is nil")
	}
	metadata, err := unmarshalAuditMetadata(po.MetadataJSON)
	if err != nil {
		return nil, err
	}
	id, err := positivePersistedUint64ToInt64(po.EventID)
	if err != nil {
		return nil, err
	}
	providerID, err := nullablePersistedUint64ToInt64(po.ProviderID)
	if err != nil {
		return nil, err
	}
	actorUserID, err := positivePersistedUint64ToInt64(po.ActorUserID)
	if err != nil {
		return nil, err
	}
	return &domainsandbox.ProviderAuditEvent{
		ID:          id,
		ProviderID:  providerID,
		ActorUserID: actorUserID,
		Action:      po.Action,
		Result:      po.Result,
		RequestID:   po.RequestID,
		Metadata:    cloneMetadata(metadata),
		CreatedAt:   po.CreatedAt.UTC(),
	}, nil
}

func cloneRuntimePolicy(policy domainsandbox.RuntimePolicy) domainsandbox.RuntimePolicy {
	cloned := policy
	cloned.NetworkAllowlist = append([]string(nil), policy.NetworkAllowlist...)
	cloned.AllowedEnvNames = append([]string(nil), policy.AllowedEnvNames...)
	cloned.VirtualReadPrefixes = append([]string(nil), policy.VirtualReadPrefixes...)
	cloned.VirtualWritePrefixes = append([]string(nil), policy.VirtualWritePrefixes...)
	cloned.AllowedExecutables = append([]string(nil), policy.AllowedExecutables...)
	cloned.AllowEnv = append([]string(nil), policy.AllowEnv...)
	cloned.AllowRead = append([]string(nil), policy.AllowRead...)
	cloned.AllowWrite = append([]string(nil), policy.AllowWrite...)
	cloned.AllowRun = append([]string(nil), policy.AllowRun...)
	cloned.AllowFFI = append([]string(nil), policy.AllowFFI...)
	return cloned
}

func cloneHealthSnapshot(snapshot domainsandbox.HealthSnapshot) domainsandbox.HealthSnapshot {
	cloned := snapshot
	cloned.Capabilities = append([]domainsandbox.Scope(nil), snapshot.Capabilities...)
	cloned.Features = append([]domainsandbox.ProviderFeature(nil), snapshot.Features...)
	return cloned
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func positiveDomainInt64ToUint64(value int64) (uint64, error) {
	if value <= 0 {
		return 0, errInvalidPersistedNumericProjection
	}
	return uint64(value), nil
}

func providerMaxConcurrencyToPO(value int) (uint32, error) {
	if value <= 0 || uint64(value) > uint64(maxProviderConcurrencyPersistence) {
		return 0, domainsandbox.ErrInvalidInput
	}
	return uint32(value), nil
}

func providerMaxConcurrencyFromPO(value uint32) (int, error) {
	if value == 0 || value > maxProviderConcurrencyPersistence {
		return 0, errInvalidPersistedNumericProjection
	}
	return int(value), nil
}

func nonNegativeDomainInt64ToUint32(value int64) (uint32, error) {
	if value < 0 || uint64(value) > math.MaxUint32 {
		return 0, errInvalidPersistedNumericProjection
	}
	return uint32(value), nil
}

func positivePersistedUint64ToInt64(value uint64) (int64, error) {
	if value == 0 || value > math.MaxInt64 {
		return 0, errInvalidPersistedNumericProjection
	}
	return int64(value), nil
}

func nullablePersistedUint64ToInt64(value *uint64) (int64, error) {
	if value == nil {
		return 0, nil
	}
	return positivePersistedUint64ToInt64(*value)
}
