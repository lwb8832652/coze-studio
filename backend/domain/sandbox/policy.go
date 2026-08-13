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
	"math"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxProviderKeyLength      = 64
	MaxProviderNameLength     = 128
	MaxEndpointHintLength     = 255
	MaxHealthReasonCodeLength = 64
	MaxHealthMessageLength    = 255

	MinTimeoutSeconds = 1
	MaxTimeoutSeconds = 3600
	MinMemoryLimitMB  = 64
	MaxMemoryLimitMB  = 32768
	MinCPULimit       = 0.1
	MaxCPULimit       = 64.0

	MinOutputLimitBytes int64 = 1024
	MaxOutputLimitBytes int64 = 16 * 1024 * 1024

	MinProviderConcurrency = 1
	// MaxProviderConcurrency bounds lease and accounting growth while allowing
	// large shared runner pools. Deployments needing more must shard providers.
	MaxProviderConcurrency      = 1024
	MaxNetworkAllowlistEntries  = 128
	MaxPolicyListEntries        = 64
	MaxEnvironmentNameLength    = 128
	MaxVirtualPrefixLength      = 256
	MaxExecutableNameLength     = 128
	MaxDirectoryReferenceLength = 128

	InitialVersion uint64 = 1

	maxNetworkHostLength = 253
	maxDNSLabelLength    = 63

	MaxAuditMetadataValueLength = 256
	MaxAuditHealthCodeLength    = 64
	MaxAuditActionLength        = 64
	MaxAuditResultLength        = 64
	MaxAuditRequestIDLength     = 128
)

const (
	AuditMetadataKeyScope          = "scope"
	AuditMetadataKeyChangedFields  = "changed_fields"
	AuditMetadataKeyPreviousStatus = "previous_status"
	AuditMetadataKeyNewStatus      = "new_status"
	AuditMetadataKeyHealthCode     = "health_code"
	AuditMetadataKeyVersion        = "version"
)

var allowedAuditChangedFields = map[string]struct{}{
	"name":          {},
	"provider_type": {},
	"endpoint":      {},
	"credential":    {},
	"scopes":        {},
	"policy":        {},
	"status":        {},
	"health":        {},
	"default":       {},
}

func NormalizeCreateProviderInput(input CreateProviderInput) (CreateProviderInput, error) {
	if ValidateProviderKey(input.ProviderKey) != nil || input.ActorUserID <= 0 ||
		ValidateLegacySourceHash(input.LegacySourceHash) != nil {
		return CreateProviderInput{}, ErrInvalidInput
	}
	name, endpointHint, scopes, policy, err := normalizeProviderConfiguration(
		input.Name,
		input.Type,
		input.EndpointHint,
		input.Scopes,
		input.Policy,
	)
	if err != nil {
		return CreateProviderInput{}, err
	}
	if err := ValidateProviderSecretPersistence(
		input.Type,
		input.EndpointSecret,
		endpointHint,
		input.CredentialSecret,
		input.CredentialFingerprint,
	); err != nil {
		return CreateProviderInput{}, err
	}
	normalized := input
	normalized.Name = name
	normalized.EndpointHint = endpointHint
	normalized.Scopes = scopes
	normalized.Policy = policy
	return normalized, nil
}

func NewProvider(input CreateProviderInput) (*Provider, error) {
	normalized, err := NormalizeCreateProviderInput(input)
	if err != nil {
		return nil, err
	}

	return &Provider{
		ProviderKey:           normalized.ProviderKey,
		Name:                  normalized.Name,
		Type:                  normalized.Type,
		EndpointSecret:        normalized.EndpointSecret,
		EndpointHint:          normalized.EndpointHint,
		CredentialSecret:      normalized.CredentialSecret,
		CredentialFingerprint: normalized.CredentialFingerprint,
		Scopes:                normalized.Scopes,
		Policy:                normalized.Policy,
		LegacySourceHash:      normalized.LegacySourceHash,
		Status:                ProviderStatusDisabled,
		Health: HealthSnapshot{
			Status: HealthStatusUnknown,
		},
		CreatedBy: normalized.ActorUserID,
		UpdatedBy: normalized.ActorUserID,
	}, nil
}

func NormalizeUpdateProviderInput(input UpdateProviderInput) (UpdateProviderInput, error) {
	if err := validateMutationIdentity(input.ProviderID, input.ExpectedVersion, input.ActorUserID); err != nil {
		return UpdateProviderInput{}, err
	}
	name, endpointHint, scopes, policy, err := normalizeProviderConfiguration(
		input.Name,
		input.Type,
		input.EndpointHint,
		input.Scopes,
		input.Policy,
	)
	if err != nil {
		return UpdateProviderInput{}, err
	}
	if err := ValidateProviderSecretPersistence(
		input.Type,
		input.EndpointSecret,
		endpointHint,
		input.CredentialSecret,
		input.CredentialFingerprint,
	); err != nil {
		return UpdateProviderInput{}, err
	}

	normalized := input
	normalized.Name = name
	normalized.EndpointHint = endpointHint
	normalized.Scopes = scopes
	normalized.Policy = policy
	return normalized, nil
}

// ValidateProviderSecretPersistence is the canonical domain/repository
// boundary for persisted secret projections. Ciphertext is opaque here; its
// envelope metadata is checked by the application before enabling execution.
func ValidateProviderSecretPersistence(
	providerType ProviderType,
	endpointSecret string,
	endpointHint string,
	credentialSecret string,
	credentialFingerprint string,
) error {
	switch providerType {
	case ProviderTypeRemoteHTTP:
		if endpointSecret == "" || !validMaskedEndpointHint(endpointHint) {
			return ErrInvalidInput
		}
		if (credentialSecret == "") != (credentialFingerprint == "") {
			return ErrInvalidInput
		}
		if credentialFingerprint != "" && !validCredentialFingerprint(credentialFingerprint) {
			return ErrInvalidInput
		}
	case ProviderTypeLocalDebug:
		if endpointSecret != "" || endpointHint != "" || credentialSecret != "" || credentialFingerprint != "" {
			return ErrInvalidInput
		}
	default:
		return ErrInvalidInput
	}
	return nil
}

func validCredentialFingerprint(value string) bool {
	if len(value) != 32 {
		return false
	}
	for index := range value {
		if !(value[index] >= '0' && value[index] <= '9') &&
			!(value[index] >= 'a' && value[index] <= 'f') {
			return false
		}
	}
	return true
}

func validMaskedEndpointHint(value string) bool {
	const prefix = "https://"
	if len(value) <= len(prefix) || len(value) > MaxEndpointHintLength || !strings.HasPrefix(value, prefix) {
		return false
	}
	authority := strings.TrimPrefix(value, prefix)
	if authority == "" || strings.ContainsAny(authority, "/?#@") {
		return false
	}
	for index := range authority {
		if authority[index] <= 0x20 || authority[index] >= 0x7f {
			return false
		}
	}

	host := authority
	port := ""
	if strings.HasPrefix(authority, "[****]") {
		suffix := strings.TrimPrefix(authority, "[****]")
		if suffix != "" {
			if !strings.HasPrefix(suffix, ":") {
				return false
			}
			port = strings.TrimPrefix(suffix, ":")
		}
		host = "[****]"
	} else {
		if strings.ContainsAny(authority, "[]") || strings.Count(authority, ":") > 1 {
			return false
		}
		if separator := strings.LastIndex(authority, ":"); separator >= 0 {
			host = authority[:separator]
			port = authority[separator+1:]
		}
	}
	if !validMaskedEndpointPort(port) {
		return false
	}
	switch host {
	case "***", "***.***.***.***", "[****]":
		return true
	}
	if !strings.HasPrefix(host, "***.") {
		return false
	}
	suffix := strings.TrimPrefix(host, "***.")
	return !strings.Contains(suffix, ".") && validMaskedEndpointLabel(suffix)
}

func validMaskedEndpointPort(port string) bool {
	if port == "" {
		return true
	}
	if len(port) > 1 && port[0] == '0' {
		return false
	}
	for index := range port {
		if port[index] < '0' || port[index] > '9' {
			return false
		}
	}
	value, err := strconv.Atoi(port)
	return err == nil && value >= 1 && value <= 65535
}

func validMaskedEndpointLabel(label string) bool {
	if len(label) == 0 || len(label) > maxDNSLabelLength ||
		!isLowerASCIIAlphaNumeric(label[0]) || !isLowerASCIIAlphaNumeric(label[len(label)-1]) {
		return false
	}
	for index := 1; index < len(label)-1; index++ {
		if !isLowerASCIIAlphaNumeric(label[index]) && label[index] != '-' {
			return false
		}
	}
	return true
}

func NormalizeUpdateProviderStatusInput(input UpdateProviderStatusInput) (UpdateProviderStatusInput, error) {
	if err := validateMutationIdentity(input.ProviderID, input.ExpectedVersion, input.ActorUserID); err != nil {
		return UpdateProviderStatusInput{}, err
	}
	if !validProviderStatus(input.Status) {
		return UpdateProviderStatusInput{}, ErrInvalidInput
	}
	return input, nil
}

func NormalizeUpdateProviderHealthInput(
	input UpdateProviderHealthInput,
	configuredProviderScopes []Scope,
) (UpdateProviderHealthInput, error) {
	if err := validateMutationIdentity(input.ProviderID, input.ExpectedVersion, input.ActorUserID); err != nil {
		return UpdateProviderHealthInput{}, err
	}
	health, err := NormalizeHealthSnapshot(input.Health, configuredProviderScopes)
	if err != nil {
		return UpdateProviderHealthInput{}, err
	}
	normalized := input
	normalized.Health = health
	return normalized, nil
}

func ValidateDeleteProviderInput(input DeleteProviderInput) error {
	return validateMutationIdentity(input.ProviderID, input.ExpectedVersion, input.ActorUserID)
}

func NormalizeSetProviderDefaultInput(
	input SetProviderDefaultInput,
	current *ProviderDefault,
) (SetProviderDefaultInput, uint64, error) {
	if !validScope(input.Scope) || input.ProviderID <= 0 || input.ActorUserID <= 0 {
		return SetProviderDefaultInput{}, 0, ErrInvalidInput
	}
	if current == nil {
		if input.ExpectedVersion != 0 {
			return SetProviderDefaultInput{}, 0, ErrDefaultMissing
		}
		return input, InitialVersion, nil
	}
	if !validScope(current.Scope) || current.Scope != input.Scope || current.ProviderID <= 0 || current.Version == 0 {
		return SetProviderDefaultInput{}, 0, ErrInvalidInput
	}
	if input.ExpectedVersion == 0 || input.ExpectedVersion != current.Version {
		return SetProviderDefaultInput{}, 0, ErrVersionConflict
	}
	nextVersion, err := NextVersion(input.ExpectedVersion)
	if err != nil {
		return SetProviderDefaultInput{}, 0, err
	}
	return input, nextVersion, nil
}

func NextVersion(expectedVersion uint64) (uint64, error) {
	if expectedVersion == math.MaxUint64 {
		return 0, ErrInvalidInput
	}
	return expectedVersion + 1, nil
}

func normalizeProviderConfiguration(
	name string,
	providerType ProviderType,
	endpointHint string,
	scopes []Scope,
	policy RuntimePolicy,
) (string, string, []Scope, RuntimePolicy, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > MaxProviderNameLength || containsControl(name) {
		return "", "", nil, RuntimePolicy{}, ErrInvalidInput
	}
	if !validProviderType(providerType) {
		return "", "", nil, RuntimePolicy{}, ErrInvalidInput
	}

	endpointHint = strings.TrimSpace(endpointHint)
	if utf8.RuneCountInString(endpointHint) > MaxEndpointHintLength || containsControl(endpointHint) {
		return "", "", nil, RuntimePolicy{}, ErrInvalidInput
	}
	switch providerType {
	case ProviderTypeRemoteHTTP:
		if endpointHint == "" {
			return "", "", nil, RuntimePolicy{}, ErrInvalidInput
		}
	case ProviderTypeLocalDebug:
		if endpointHint != "" {
			return "", "", nil, RuntimePolicy{}, ErrInvalidInput
		}
	}

	normalizedScopes, err := NormalizeScopes(scopes)
	if err != nil {
		return "", "", nil, RuntimePolicy{}, err
	}
	if !providerTypeSupportsScopes(providerType, normalizedScopes) {
		return "", "", nil, RuntimePolicy{}, ErrInvalidInput
	}
	normalizedPolicy, err := NormalizeRuntimePolicy(policy)
	if err != nil {
		return "", "", nil, RuntimePolicy{}, err
	}
	normalizedPolicy, err = NormalizeLegacyRuntimePolicy(normalizedPolicy)
	if err != nil {
		return "", "", nil, RuntimePolicy{}, err
	}
	return name, endpointHint, normalizedScopes, normalizedPolicy, nil
}

func NormalizeScopes(scopes []Scope) ([]Scope, error) {
	if len(scopes) == 0 {
		return nil, ErrInvalidInput
	}

	seen := make(map[Scope]struct{}, len(scopes))
	for _, raw := range scopes {
		scope := Scope(strings.TrimSpace(string(raw)))
		if !validScope(scope) {
			return nil, ErrInvalidInput
		}
		seen[scope] = struct{}{}
	}

	normalized := make([]Scope, 0, len(seen))
	for _, scope := range []Scope{ScopeAgent, ScopeMCPStdio, ScopeAppDev, ScopePlugin} {
		if _, ok := seen[scope]; ok {
			normalized = append(normalized, scope)
		}
	}
	return normalized, nil
}

func ValidateRuntimePolicy(policy RuntimePolicy) error {
	_, err := NormalizeRuntimePolicy(policy)
	return err
}

// MergeRuntimePolicyPublicPatch applies the Admin API policy projection while
// preserving internal compatibility metadata that is intentionally omitted
// from that API. The returned slices are detached from the current entity.
func MergeRuntimePolicyPublicPatch(current, publicPatch RuntimePolicy) RuntimePolicy {
	merged := publicPatch
	merged.AllowEnv = cloneCompatibilityPolicySlice(current.AllowEnv)
	merged.AllowRead = cloneCompatibilityPolicySlice(current.AllowRead)
	merged.AllowWrite = cloneCompatibilityPolicySlice(current.AllowWrite)
	merged.AllowRun = cloneCompatibilityPolicySlice(current.AllowRun)
	merged.AllowFFI = cloneCompatibilityPolicySlice(current.AllowFFI)
	merged.NodeModulesDir = current.NodeModulesDir
	return merged
}

// PublicRuntimePoliciesEqual compares only fields represented by the Admin
// API. Nil and empty lists are equivalent after JSON round trips.
func PublicRuntimePoliciesEqual(left, right RuntimePolicy) bool {
	left, leftErr := NormalizeRuntimePolicy(left)
	right, rightErr := NormalizeRuntimePolicy(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return left.TimeoutSeconds == right.TimeoutSeconds &&
		left.MemoryLimitMB == right.MemoryLimitMB &&
		left.CPULimit == right.CPULimit &&
		left.MaxOutputBytes == right.MaxOutputBytes &&
		left.MaxConcurrency == right.MaxConcurrency &&
		left.AllowNetwork == right.AllowNetwork &&
		equalPolicyLists(left.NetworkAllowlist, right.NetworkAllowlist) &&
		equalPolicyLists(left.AllowedEnvNames, right.AllowedEnvNames) &&
		equalPolicyLists(left.VirtualReadPrefixes, right.VirtualReadPrefixes) &&
		equalPolicyLists(left.VirtualWritePrefixes, right.VirtualWritePrefixes) &&
		equalPolicyLists(left.AllowedExecutables, right.AllowedExecutables) &&
		left.FFIEnabled == right.FFIEnabled &&
		left.NodeModulesMode == right.NodeModulesMode &&
		left.NodeModulesDirectoryRef == right.NodeModulesDirectoryRef
}

func cloneCompatibilityPolicySlice(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func equalPolicyLists(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func NormalizeRuntimePolicy(policy RuntimePolicy) (RuntimePolicy, error) {
	if policy.TimeoutSeconds < MinTimeoutSeconds || policy.TimeoutSeconds > MaxTimeoutSeconds {
		return RuntimePolicy{}, ErrInvalidInput
	}
	if policy.MemoryLimitMB < MinMemoryLimitMB || policy.MemoryLimitMB > MaxMemoryLimitMB {
		return RuntimePolicy{}, ErrInvalidInput
	}
	if math.IsNaN(policy.CPULimit) || math.IsInf(policy.CPULimit, 0) ||
		policy.CPULimit < MinCPULimit || policy.CPULimit > MaxCPULimit {
		return RuntimePolicy{}, ErrInvalidInput
	}
	if policy.MaxOutputBytes < MinOutputLimitBytes || policy.MaxOutputBytes > MaxOutputLimitBytes {
		return RuntimePolicy{}, ErrInvalidInput
	}
	if policy.MaxConcurrency < MinProviderConcurrency || policy.MaxConcurrency > MaxProviderConcurrency {
		return RuntimePolicy{}, ErrInvalidInput
	}

	normalized := policy
	var err error
	normalized.AllowedEnvNames, err = normalizePolicyList(policy.AllowedEnvNames, normalizeEnvironmentName)
	if err != nil {
		return RuntimePolicy{}, err
	}
	normalized.VirtualReadPrefixes, err = normalizePolicyList(policy.VirtualReadPrefixes, normalizeVirtualPrefix)
	if err != nil {
		return RuntimePolicy{}, err
	}
	normalized.VirtualWritePrefixes, err = normalizePolicyList(policy.VirtualWritePrefixes, normalizeVirtualPrefix)
	if err != nil {
		return RuntimePolicy{}, err
	}
	normalized.AllowedExecutables, err = normalizePolicyList(policy.AllowedExecutables, normalizeExecutableName)
	if err != nil {
		return RuntimePolicy{}, err
	}
	normalized.NodeModulesMode, normalized.NodeModulesDirectoryRef, err = normalizeNodeModulesPolicy(
		policy.NodeModulesMode,
		policy.NodeModulesDirectoryRef,
	)
	if err != nil {
		return RuntimePolicy{}, err
	}
	if !policy.AllowNetwork {
		if len(policy.NetworkAllowlist) != 0 {
			return RuntimePolicy{}, ErrInvalidInput
		}
		normalized.NetworkAllowlist = nil
		return normalized, nil
	}

	allowlist, err := normalizeNetworkAllowlist(policy.NetworkAllowlist)
	if err != nil {
		return RuntimePolicy{}, err
	}
	normalized.NetworkAllowlist = allowlist
	return normalized, nil
}

type policyMetadataNormalizer func(string) (string, bool)

func normalizePolicyList(values []string, normalize policyMetadataNormalizer) ([]string, error) {
	if len(values) > MaxPolicyListEntries {
		return nil, ErrInvalidInput
	}
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value, ok := normalize(raw)
		if !ok {
			return nil, ErrInvalidInput
		}
		seen[value] = struct{}{}
	}
	normalized := make([]string, 0, len(seen))
	for value := range seen {
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeEnvironmentName(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if len(value) == 0 || len(value) > MaxEnvironmentNameLength ||
		!utf8.ValidString(value) || containsControl(value) {
		return "", false
	}
	for index := range value {
		character := value[index]
		if index == 0 {
			if (character < 'A' || character > 'Z') && character != '_' {
				return "", false
			}
			continue
		}
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return "", false
		}
	}
	return value, true
}

func normalizeVirtualPrefix(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if len(value) == 0 || len(value) > MaxVirtualPrefixLength || !utf8.ValidString(value) ||
		containsControl(value) || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\:") {
		return "", false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	root := strings.SplitN(cleaned, "/", 2)[0]
	switch root {
	case "workspace", "inputs", "outputs", "artifacts":
	default:
		return "", false
	}
	for index := range cleaned {
		character := cleaned[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '/' && character != '_' &&
			character != '-' && character != '.' {
			return "", false
		}
	}
	return cleaned, true
}

func normalizeExecutableName(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if len(value) == 0 || len(value) > MaxExecutableNameLength || !utf8.ValidString(value) || containsControl(value) {
		return "", false
	}
	for index := range value {
		character := value[index]
		if index == 0 && !isASCIIAlphaNumeric(character) {
			return "", false
		}
		if !isASCIIAlphaNumeric(character) && character != '_' && character != '-' && character != '.' && character != '+' {
			return "", false
		}
	}
	return value, true
}

func normalizeNodeModulesPolicy(mode NodeModulesMode, rawReference string) (NodeModulesMode, string, error) {
	if mode == "" {
		mode = NodeModulesModeDisabled
	}
	reference := strings.TrimSpace(rawReference)
	switch mode {
	case NodeModulesModeDisabled:
		if reference != "" {
			return "", "", ErrInvalidInput
		}
		return mode, "", nil
	case NodeModulesModeApprovedDirectory:
		if !validDirectoryReference(reference) {
			return "", "", ErrInvalidInput
		}
		return mode, reference, nil
	default:
		return "", "", ErrInvalidInput
	}
}

func validDirectoryReference(value string) bool {
	if len(value) == 0 || len(value) > MaxDirectoryReferenceLength || !utf8.ValidString(value) || containsControl(value) {
		return false
	}
	for index := range value {
		character := value[index]
		if index == 0 && !isASCIIAlphaNumeric(character) {
			return false
		}
		if !isASCIIAlphaNumeric(character) && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func NormalizeHealthSnapshot(snapshot HealthSnapshot, providerScopes []Scope) (HealthSnapshot, error) {
	if !validHealthStatus(snapshot.Status) {
		return HealthSnapshot{}, ErrInvalidInput
	}
	reasonCode, err := normalizeHealthReasonCode(snapshot.Status, snapshot.ReasonCode)
	if err != nil {
		return HealthSnapshot{}, err
	}
	message, err := normalizeHealthMessage(snapshot.Message)
	if err != nil {
		return HealthSnapshot{}, err
	}
	normalizedProviderScopes, err := NormalizeScopes(providerScopes)
	if err != nil {
		return HealthSnapshot{}, err
	}
	providerScopeSet := make(map[Scope]struct{}, len(normalizedProviderScopes))
	for _, scope := range normalizedProviderScopes {
		providerScopeSet[scope] = struct{}{}
	}

	normalized := snapshot
	normalized.Capabilities = nil
	normalized.Features = nil
	normalized.ReasonCode = reasonCode
	normalized.Message = message
	if len(snapshot.Capabilities) > 0 {
		normalizedCapabilities, err := NormalizeScopes(snapshot.Capabilities)
		if err != nil {
			return HealthSnapshot{}, err
		}
		for _, capability := range normalizedCapabilities {
			if _, ok := providerScopeSet[capability]; !ok {
				return HealthSnapshot{}, ErrInvalidInput
			}
		}
		normalized.Capabilities = normalizedCapabilities
	}
	features, err := NormalizeProviderFeatures(snapshot.Features)
	if err != nil {
		return HealthSnapshot{}, err
	}
	normalized.Features = features

	switch snapshot.Status {
	case HealthStatusHealthy, HealthStatusDegraded, HealthStatusUnhealthy:
		if snapshot.CheckedAt.IsZero() || snapshot.LatencyMillis < 0 {
			return HealthSnapshot{}, ErrInvalidInput
		}
	case HealthStatusUnknown:
		if !snapshot.CheckedAt.IsZero() || snapshot.LatencyMillis != 0 || len(snapshot.Capabilities) != 0 || len(snapshot.Features) != 0 {
			return HealthSnapshot{}, ErrInvalidInput
		}
	}
	if !snapshot.CheckedAt.IsZero() {
		normalized.CheckedAt = snapshot.CheckedAt.UTC()
	}
	return normalized, nil
}

// NormalizeProviderFeatures keeps optional, versioned protocol extensions
// separate from Provider capabilities (which remain scoped workload support).
func NormalizeProviderFeatures(features []ProviderFeature) ([]ProviderFeature, error) {
	if len(features) == 0 {
		return nil, nil
	}
	normalized := make([]ProviderFeature, 0, len(features))
	seen := make(map[ProviderFeature]struct{}, len(features))
	for _, feature := range features {
		if feature != ProviderFeatureQueueStatusV1 && feature != ProviderFeatureSignedExecutionContext &&
			feature != ProviderFeatureSandboxSessionV1 && feature != ProviderFeatureSignedSessionContextV2 {
			return nil, ErrInvalidInput
		}
		if _, duplicate := seen[feature]; duplicate {
			return nil, ErrInvalidInput
		}
		seen[feature] = struct{}{}
		normalized = append(normalized, feature)
	}
	return normalized, nil
}

func normalizeHealthReasonCode(status HealthStatus, reasonCode string) (string, error) {
	if reasonCode == "" {
		if status == HealthStatusDegraded || status == HealthStatusUnhealthy {
			return "", ErrInvalidInput
		}
		return "", nil
	}
	if status == HealthStatusUnknown || len(reasonCode) > MaxHealthReasonCodeLength {
		return "", ErrInvalidInput
	}
	for index := 0; index < len(reasonCode); index++ {
		character := reasonCode[index]
		if (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '_' {
			return "", ErrInvalidInput
		}
	}
	return reasonCode, nil
}

func normalizeHealthMessage(message string) (string, error) {
	if !utf8.ValidString(message) {
		return "", ErrInvalidInput
	}
	message = strings.TrimSpace(message)
	if len(message) > MaxHealthMessageLength {
		return "", ErrInvalidInput
	}
	for _, character := range message {
		if character <= 0x1f || (character >= 0x7f && character <= 0x9f) {
			return "", ErrInvalidInput
		}
	}
	return message, nil
}

func HealthUsableForScope(
	snapshot HealthSnapshot,
	scope Scope,
	now time.Time,
	maxAge time.Duration,
) bool {
	if snapshot.Status != HealthStatusHealthy || !validScope(scope) || now.IsZero() || maxAge <= 0 {
		return false
	}
	if snapshot.CheckedAt.IsZero() || snapshot.LatencyMillis < 0 || snapshot.CheckedAt.After(now) {
		return false
	}
	if now.Sub(snapshot.CheckedAt) > maxAge {
		return false
	}
	for _, capability := range snapshot.Capabilities {
		if capability == scope {
			return true
		}
	}
	return false
}

func NormalizeAuditMetadata(metadata map[string]string) (map[string]string, error) {
	normalized := make(map[string]string, len(metadata))
	for key, rawValue := range metadata {
		value := strings.TrimSpace(rawValue)
		if value == "" || len(value) > MaxAuditMetadataValueLength || containsControl(value) {
			return nil, ErrInvalidInput
		}

		switch key {
		case AuditMetadataKeyScope:
			scope := Scope(value)
			if !validScope(scope) {
				return nil, ErrInvalidInput
			}
			normalized[key] = string(scope)
		case AuditMetadataKeyChangedFields:
			changedFields, err := normalizeAuditChangedFields(value)
			if err != nil {
				return nil, err
			}
			normalized[key] = changedFields
		case AuditMetadataKeyPreviousStatus, AuditMetadataKeyNewStatus:
			status := ProviderStatus(value)
			if !validProviderStatus(status) {
				return nil, ErrInvalidInput
			}
			normalized[key] = string(status)
		case AuditMetadataKeyHealthCode:
			if !validAuditHealthCode(value) {
				return nil, ErrInvalidInput
			}
			normalized[key] = value
		case AuditMetadataKeyVersion:
			version, err := strconv.ParseUint(value, 10, 64)
			if err != nil || version == 0 {
				return nil, ErrInvalidInput
			}
			normalized[key] = strconv.FormatUint(version, 10)
		default:
			return nil, ErrInvalidInput
		}
	}
	return normalized, nil
}

func NormalizeAppendProviderAuditEventInput(
	input AppendProviderAuditEventInput,
) (AppendProviderAuditEventInput, error) {
	if input.ProviderID <= 0 || input.ActorUserID <= 0 {
		return AppendProviderAuditEventInput{}, ErrInvalidInput
	}
	action, ok := normalizeBoundedAuditField(input.Action, MaxAuditActionLength)
	if !ok {
		return AppendProviderAuditEventInput{}, ErrInvalidInput
	}
	result, ok := normalizeBoundedAuditField(input.Result, MaxAuditResultLength)
	if !ok {
		return AppendProviderAuditEventInput{}, ErrInvalidInput
	}
	requestID, ok := normalizeBoundedAuditField(input.RequestID, MaxAuditRequestIDLength)
	if !ok {
		return AppendProviderAuditEventInput{}, ErrInvalidInput
	}
	metadata, err := NormalizeAuditMetadata(input.Metadata)
	if err != nil {
		return AppendProviderAuditEventInput{}, err
	}
	normalized := input
	normalized.Action = action
	normalized.Result = result
	normalized.RequestID = requestID
	normalized.Metadata = metadata
	return normalized, nil
}

func NewProviderAuditEvent(input AppendProviderAuditEventInput) (*ProviderAuditEvent, error) {
	normalized, err := NormalizeAppendProviderAuditEventInput(input)
	if err != nil {
		return nil, err
	}
	return &ProviderAuditEvent{
		ProviderID:  normalized.ProviderID,
		ActorUserID: normalized.ActorUserID,
		Action:      normalized.Action,
		Result:      normalized.Result,
		RequestID:   normalized.RequestID,
		Metadata:    normalized.Metadata,
	}, nil
}

func normalizeNetworkAllowlist(entries []string) ([]string, error) {
	if len(entries) == 0 || len(entries) > MaxNetworkAllowlistEntries {
		return nil, ErrInvalidInput
	}

	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		host, ok := normalizeNetworkHostPattern(entry)
		if !ok {
			return nil, ErrInvalidInput
		}
		seen[host] = struct{}{}
	}
	normalized := make([]string, 0, len(seen))
	for host := range seen {
		normalized = append(normalized, host)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeNetworkHostPattern(raw string) (string, bool) {
	host := strings.ToLower(strings.TrimSpace(raw))
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return "", false
	}

	prefix := ""
	if strings.HasPrefix(host, "*.") {
		prefix = "*."
		host = strings.TrimPrefix(host, "*.")
	}
	if host == "" || len(host) > maxNetworkHostLength || strings.Contains(host, "*") {
		return "", false
	}

	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > maxDNSLabelLength || label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for index := 0; index < len(label); index++ {
			character := label[index]
			if !isLowerASCIIAlphaNumeric(character) && character != '-' {
				return "", false
			}
		}
	}
	return prefix + host, true
}

func normalizeAuditChangedFields(value string) (string, error) {
	seen := make(map[string]struct{})
	for _, rawField := range strings.Split(value, ",") {
		field := strings.TrimSpace(rawField)
		if _, ok := allowedAuditChangedFields[field]; !ok {
			return "", ErrInvalidInput
		}
		seen[field] = struct{}{}
	}
	fields := make([]string, 0, len(seen))
	for field := range seen {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return strings.Join(fields, ","), nil
}

func validAuditHealthCode(value string) bool {
	if len(value) == 0 || len(value) > MaxAuditHealthCodeLength {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !isASCIIAlphaNumeric(character) && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func normalizeBoundedAuditField(value string, maxLength int) (string, bool) {
	value = strings.TrimSpace(value)
	return value, value != "" && utf8.ValidString(value) && len(value) <= maxLength && !containsControl(value)
}

func validateMutationIdentity(providerID int64, expectedVersion uint64, actorUserID int64) error {
	if providerID <= 0 || expectedVersion == 0 || actorUserID <= 0 {
		return ErrInvalidInput
	}
	if _, err := NextVersion(expectedVersion); err != nil {
		return err
	}
	return nil
}

// NormalizeProviderKey validates the canonical persisted provider-key form.
// It deliberately performs no trimming or case conversion.
func NormalizeProviderKey(providerKey string) (string, error) {
	if !validProviderKey(providerKey) {
		return "", ErrInvalidInput
	}
	return providerKey, nil
}

// ValidateProviderKey reports whether providerKey already has canonical form.
func ValidateProviderKey(providerKey string) error {
	_, err := NormalizeProviderKey(providerKey)
	return err
}

func validProviderKey(providerKey string) bool {
	if len(providerKey) == 0 || len(providerKey) > MaxProviderKeyLength {
		return false
	}
	if !isLowerASCIIAlphaNumeric(providerKey[0]) || !isLowerASCIIAlphaNumeric(providerKey[len(providerKey)-1]) {
		return false
	}
	for index := 1; index < len(providerKey)-1; index++ {
		character := providerKey[index]
		if !isLowerASCIIAlphaNumeric(character) && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func validProviderType(providerType ProviderType) bool {
	return providerType == ProviderTypeRemoteHTTP || providerType == ProviderTypeLocalDebug
}

func providerTypeSupportsScopes(providerType ProviderType, scopes []Scope) bool {
	if providerType != ProviderTypeLocalDebug {
		return true
	}
	for _, scope := range scopes {
		if scope == ScopePlugin {
			return false
		}
	}
	return true
}

func validProviderStatus(status ProviderStatus) bool {
	return status == ProviderStatusDisabled || status == ProviderStatusEnabled
}

func validHealthStatus(status HealthStatus) bool {
	switch status {
	case HealthStatusUnknown, HealthStatusHealthy, HealthStatusDegraded, HealthStatusUnhealthy:
		return true
	default:
		return false
	}
}

func validScope(scope Scope) bool {
	return scope == ScopeAgent || scope == ScopeMCPStdio || scope == ScopeAppDev ||
		scope == ScopePlugin
}

func isLowerASCIIAlphaNumeric(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
}

func isASCIIAlphaNumeric(character byte) bool {
	return isLowerASCIIAlphaNumeric(character) || (character >= 'A' && character <= 'Z')
}

func containsControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
