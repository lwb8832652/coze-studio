// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	MaxLegacyPolicyEntries    = 128
	MaxLegacyPolicyItemBytes  = 1024
	MaxLegacyPolicyTotalBytes = 16 * 1024
	MaxNodeModulesDirBytes    = 4096
)

// NormalizeLegacyRuntimePolicy validates the internal-only restrictions
// migrated from BasicConfiguration.sandbox_config. These fields are persisted
// for local-debug compatibility but deliberately omitted from public JSON.
func NormalizeLegacyRuntimePolicy(policy RuntimePolicy) (RuntimePolicy, error) {
	var err error
	policy.AllowEnv, err = normalizeLegacyPolicyList(policy.AllowEnv, true)
	if err != nil {
		return RuntimePolicy{}, err
	}
	policy.AllowRead, err = normalizeLegacyPolicyList(policy.AllowRead, false)
	if err != nil {
		return RuntimePolicy{}, err
	}
	policy.AllowWrite, err = normalizeLegacyPolicyList(policy.AllowWrite, false)
	if err != nil {
		return RuntimePolicy{}, err
	}
	policy.AllowRun, err = normalizeLegacyPolicyList(policy.AllowRun, false)
	if err != nil {
		return RuntimePolicy{}, err
	}
	policy.AllowFFI, err = normalizeLegacyPolicyList(policy.AllowFFI, false)
	if err != nil {
		return RuntimePolicy{}, err
	}
	policy.NodeModulesDir = strings.TrimSpace(policy.NodeModulesDir)
	if len(policy.NodeModulesDir) > MaxNodeModulesDirBytes ||
		!validLegacyPolicyText(policy.NodeModulesDir, MaxNodeModulesDirBytes) {
		return RuntimePolicy{}, ErrInvalidInput
	}
	return policy, nil
}

func ValidateLegacySourceHash(value string) error {
	if value == "" {
		return nil
	}
	if len(value) != 64 {
		return ErrInvalidInput
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			if value[index] < 'a' || value[index] > 'f' {
				return ErrInvalidInput
			}
		}
	}
	return nil
}

func normalizeLegacyPolicyList(values []string, environmentNames bool) ([]string, error) {
	wasNil := values == nil
	if len(values) > MaxLegacyPolicyEntries {
		return nil, ErrInvalidInput
	}
	unique := make(map[string]struct{}, len(values))
	totalBytes := 0
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !validLegacyPolicyText(value, MaxLegacyPolicyItemBytes) ||
			environmentNames && !validCanonicalEnvironmentName(value) {
			return nil, ErrInvalidInput
		}
		totalBytes += len(value)
		if totalBytes > MaxLegacyPolicyTotalBytes {
			return nil, ErrInvalidInput
		}
		unique[value] = struct{}{}
	}
	if len(unique) > MaxLegacyPolicyEntries {
		return nil, ErrInvalidInput
	}
	normalized := make([]string, 0, len(unique))
	for value := range unique {
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 && wasNil {
		return nil, nil
	}
	sort.Strings(normalized)
	return normalized, nil
}

func validCanonicalEnvironmentName(value string) bool {
	if len(value) == 0 || len(value) > 128 || value[0] != '_' && (value[0] < 'A' || value[0] > 'Z') {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character != '_' && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validLegacyPolicyText(value string, maximum int) bool {
	if len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == 0 || character == 0x7f || character < 0x20 {
			return false
		}
	}
	return true
}
