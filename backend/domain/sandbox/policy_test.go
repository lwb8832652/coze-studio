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
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimePolicyAcceptsInclusiveResourceBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimePolicy)
	}{
		{name: "minimum timeout", mutate: func(policy *RuntimePolicy) { policy.TimeoutSeconds = MinTimeoutSeconds }},
		{name: "maximum timeout", mutate: func(policy *RuntimePolicy) { policy.TimeoutSeconds = MaxTimeoutSeconds }},
		{name: "minimum memory", mutate: func(policy *RuntimePolicy) { policy.MemoryLimitMB = MinMemoryLimitMB }},
		{name: "maximum memory", mutate: func(policy *RuntimePolicy) { policy.MemoryLimitMB = MaxMemoryLimitMB }},
		{name: "minimum CPU", mutate: func(policy *RuntimePolicy) { policy.CPULimit = MinCPULimit }},
		{name: "maximum CPU", mutate: func(policy *RuntimePolicy) { policy.CPULimit = MaxCPULimit }},
		{name: "minimum output", mutate: func(policy *RuntimePolicy) { policy.MaxOutputBytes = MinOutputLimitBytes }},
		{name: "maximum output", mutate: func(policy *RuntimePolicy) { policy.MaxOutputBytes = MaxOutputLimitBytes }},
		{name: "minimum concurrency", mutate: func(policy *RuntimePolicy) { policy.MaxConcurrency = MinProviderConcurrency }},
		{name: "maximum concurrency", mutate: func(policy *RuntimePolicy) { policy.MaxConcurrency = MaxProviderConcurrency }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validRuntimePolicy()
			tt.mutate(&policy)
			if _, err := NormalizeRuntimePolicy(policy); err != nil {
				t.Fatalf("NormalizeRuntimePolicy() error = %v", err)
			}
		})
	}
}

func TestRuntimePolicyRejectsOutOfRangeResources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RuntimePolicy)
	}{
		{name: "timeout below minimum", mutate: func(policy *RuntimePolicy) { policy.TimeoutSeconds = MinTimeoutSeconds - 1 }},
		{name: "timeout above maximum", mutate: func(policy *RuntimePolicy) { policy.TimeoutSeconds = MaxTimeoutSeconds + 1 }},
		{name: "memory below minimum", mutate: func(policy *RuntimePolicy) { policy.MemoryLimitMB = MinMemoryLimitMB - 1 }},
		{name: "memory above maximum", mutate: func(policy *RuntimePolicy) { policy.MemoryLimitMB = MaxMemoryLimitMB + 1 }},
		{name: "CPU below minimum", mutate: func(policy *RuntimePolicy) { policy.CPULimit = MinCPULimit - 0.01 }},
		{name: "CPU above maximum", mutate: func(policy *RuntimePolicy) { policy.CPULimit = MaxCPULimit + 0.01 }},
		{name: "CPU NaN", mutate: func(policy *RuntimePolicy) { policy.CPULimit = math.NaN() }},
		{name: "CPU infinity", mutate: func(policy *RuntimePolicy) { policy.CPULimit = math.Inf(1) }},
		{name: "output below minimum", mutate: func(policy *RuntimePolicy) { policy.MaxOutputBytes = MinOutputLimitBytes - 1 }},
		{name: "output above maximum", mutate: func(policy *RuntimePolicy) { policy.MaxOutputBytes = MaxOutputLimitBytes + 1 }},
		{name: "concurrency below minimum", mutate: func(policy *RuntimePolicy) { policy.MaxConcurrency = MinProviderConcurrency - 1 }},
		{name: "concurrency above maximum", mutate: func(policy *RuntimePolicy) { policy.MaxConcurrency = MaxProviderConcurrency + 1 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := validRuntimePolicy()
			tt.mutate(&policy)
			if _, err := NormalizeRuntimePolicy(policy); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeRuntimePolicy() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestRuntimePolicyDeniesNetworkByDefault(t *testing.T) {
	policy := validRuntimePolicy()
	normalized, err := NormalizeRuntimePolicy(policy)
	if err != nil {
		t.Fatalf("NormalizeRuntimePolicy() error = %v", err)
	}
	if normalized.AllowNetwork || len(normalized.NetworkAllowlist) != 0 {
		t.Fatalf("zero-value network policy must deny access: %#v", normalized)
	}
}

func TestRuntimePolicyNetworkNormalizationIsDeterministicAndIdempotent(t *testing.T) {
	first := validRuntimePolicy()
	first.AllowNetwork = true
	first.NetworkAllowlist = []string{
		" Z.example.com ",
		"api.example.com.",
		" *.Services.Example.COM. ",
		"api.example.com",
	}
	second := first
	second.NetworkAllowlist = []string{
		"*.services.example.com",
		"API.EXAMPLE.COM",
		"z.example.com.",
	}

	normalizedFirst, err := NormalizeRuntimePolicy(first)
	if err != nil {
		t.Fatalf("NormalizeRuntimePolicy(first) error = %v", err)
	}
	normalizedSecond, err := NormalizeRuntimePolicy(second)
	if err != nil {
		t.Fatalf("NormalizeRuntimePolicy(second) error = %v", err)
	}
	want := []string{"*.services.example.com", "api.example.com", "z.example.com"}
	if !reflect.DeepEqual(normalizedFirst.NetworkAllowlist, want) {
		t.Fatalf("first allowlist = %#v, want %#v", normalizedFirst.NetworkAllowlist, want)
	}
	if !reflect.DeepEqual(normalizedSecond.NetworkAllowlist, want) {
		t.Fatalf("permuted allowlist = %#v, want %#v", normalizedSecond.NetworkAllowlist, want)
	}
	again, err := NormalizeRuntimePolicy(normalizedFirst)
	if err != nil || !reflect.DeepEqual(again, normalizedFirst) {
		t.Fatalf("normalization is not idempotent: %#v, %v", again, err)
	}
}

func TestRuntimePolicyBoundsNetworkAllowlistEntries(t *testing.T) {
	entries := make([]string, MaxNetworkAllowlistEntries+1)
	for index := range entries {
		entries[index] = fmt.Sprintf("host-%03d.example.com", index)
	}

	atLimit := validRuntimePolicy()
	atLimit.AllowNetwork = true
	atLimit.NetworkAllowlist = append([]string(nil), entries[:MaxNetworkAllowlistEntries]...)
	if _, err := NormalizeRuntimePolicy(atLimit); err != nil {
		t.Fatalf("NormalizeRuntimePolicy(at limit) error = %v", err)
	}

	overLimit := validRuntimePolicy()
	overLimit.AllowNetwork = true
	overLimit.NetworkAllowlist = entries
	if _, err := NormalizeRuntimePolicy(overLimit); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeRuntimePolicy(over limit) error = %v, want ErrInvalidInput", err)
	}
}

func TestRuntimePolicyRejectsMalformedOrInconsistentNetworkAllowlist(t *testing.T) {
	malformed := []string{
		"",
		"https://api.example.com",
		"api.example.com:443",
		"api.example.com/path",
		"api example.com",
		"example..com",
		"-api.example.com",
		"api-.example.com",
		"*.*.example.com",
		strings.Repeat("a", 64) + ".example.com",
	}

	for index, entry := range malformed {
		t.Run(fmtCase(index), func(t *testing.T) {
			policy := validRuntimePolicy()
			policy.AllowNetwork = true
			policy.NetworkAllowlist = []string{entry}
			if _, err := NormalizeRuntimePolicy(policy); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeRuntimePolicy() error = %v, want ErrInvalidInput", err)
			}
		})
	}

	t.Run("network enabled without allowlist", func(t *testing.T) {
		policy := validRuntimePolicy()
		policy.AllowNetwork = true
		if _, err := NormalizeRuntimePolicy(policy); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeRuntimePolicy() error = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("allowlist present while network denied", func(t *testing.T) {
		policy := validRuntimePolicy()
		policy.NetworkAllowlist = []string{"api.example.com"}
		if _, err := NormalizeRuntimePolicy(policy); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeRuntimePolicy() error = %v, want ErrInvalidInput", err)
		}
	})
}

func TestRuntimePolicyFailedNormalizationDoesNotMutateInput(t *testing.T) {
	policy := validRuntimePolicy()
	policy.AllowNetwork = true
	policy.NetworkAllowlist = []string{" Z.example.com ", "https://bad.example.com"}
	before := policy
	before.NetworkAllowlist = append([]string(nil), policy.NetworkAllowlist...)

	if _, err := NormalizeRuntimePolicy(policy); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeRuntimePolicy() error = %v, want ErrInvalidInput", err)
	}
	if !reflect.DeepEqual(policy, before) {
		t.Fatalf("NormalizeRuntimePolicy() mutated failed input: got %#v, want %#v", policy, before)
	}
}

func TestRuntimePolicyRepositoryPaginationAndOrderingContracts(t *testing.T) {
	providerRequest, err := NormalizeProviderListRequest(ProviderListRequest{})
	if err != nil {
		t.Fatalf("NormalizeProviderListRequest(default) error = %v", err)
	}
	if providerRequest.Offset != 0 || providerRequest.Limit != DefaultPageLimit {
		t.Fatalf("default provider page = %#v", providerRequest)
	}
	for _, limit := range []int{MinPageLimit, MaxPageLimit} {
		request, err := NormalizeProviderListRequest(ProviderListRequest{Offset: MaxPageOffset, Limit: limit})
		if err != nil || request.Limit != limit {
			t.Fatalf("provider limit %d = %#v, %v", limit, request, err)
		}
	}
	maxKeyword := strings.Repeat("k", MaxProviderListKeywordLength)
	if request, err := NormalizeProviderListRequest(ProviderListRequest{Keyword: maxKeyword}); err != nil || request.Keyword != maxKeyword {
		t.Fatalf("provider max keyword = %#v, %v", request, err)
	}
	for _, request := range []ProviderListRequest{
		{Offset: -1},
		{Offset: MaxPageOffset + 1},
		{Limit: -1},
		{Limit: MaxPageLimit + 1},
		{Keyword: strings.Repeat("k", MaxProviderListKeywordLength+1)},
		{Type: ProviderType("container")},
		{Status: ProviderStatus("ready")},
		{Scope: Scope("workflow")},
	} {
		if _, err := NormalizeProviderListRequest(request); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeProviderListRequest(%#v) error = %v, want ErrInvalidInput", request, err)
		}
	}

	auditRequest, err := NormalizeProviderAuditListRequest(ProviderAuditListRequest{})
	if err != nil {
		t.Fatalf("NormalizeProviderAuditListRequest(default) error = %v", err)
	}
	if auditRequest.Limit != DefaultPageLimit {
		t.Fatalf("default audit page = %#v", auditRequest)
	}
	maxAuditFilter := ProviderAuditListRequest{
		Action: strings.Repeat("A", MaxAuditActionLength),
		Result: strings.Repeat("R", MaxAuditResultLength),
		Offset: MaxPageOffset,
		Limit:  MaxPageLimit,
	}
	if _, err := NormalizeProviderAuditListRequest(maxAuditFilter); err != nil {
		t.Fatalf("NormalizeProviderAuditListRequest(max bounds) error = %v", err)
	}
	for _, request := range []ProviderAuditListRequest{
		{ProviderID: -1},
		{Offset: MaxPageOffset + 1},
		{Limit: MaxPageLimit + 1},
		{Action: strings.Repeat("A", MaxAuditActionLength+1)},
		{Result: strings.Repeat("R", MaxAuditResultLength+1)},
		{Action: "provider\nupdate"},
	} {
		if _, err := NormalizeProviderAuditListRequest(request); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeProviderAuditListRequest(%#v) error = %v, want ErrInvalidInput", request, err)
		}
	}

	if ProviderListStableOrder != "created_at_desc_id_desc" ||
		ProviderAuditListStableOrder != "created_at_desc_id_desc" ||
		ProviderDefaultListStableOrder != "scope_asc" {
		t.Fatalf("unstable repository ordering constants: %q %q %q",
			ProviderListStableOrder, ProviderAuditListStableOrder, ProviderDefaultListStableOrder)
	}
}

func validRuntimePolicy() RuntimePolicy {
	return RuntimePolicy{
		TimeoutSeconds: 60,
		MemoryLimitMB:  512,
		CPULimit:       1,
		MaxOutputBytes: 64 * 1024,
		MaxConcurrency: 8,
	}
}

func fmtCase(index int) string {
	const digits = "0123456789"
	if index < len(digits) {
		return "case_" + string(digits[index])
	}
	return "case_many"
}
