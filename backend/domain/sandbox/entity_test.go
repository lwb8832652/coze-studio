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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProviderEnumsAreStable(t *testing.T) {
	if ScopeAgent != "agent" || ScopeMCPStdio != "mcp_stdio" ||
		ScopeAppDev != "appdev" || ScopePlugin != "plugin" {
		t.Fatalf(
			"unexpected scope values: %q, %q, %q, %q",
			ScopeAgent,
			ScopeMCPStdio,
			ScopeAppDev,
			ScopePlugin,
		)
	}
	if ProviderTypeRemoteHTTP != "remote_http" || ProviderTypeLocalDebug != "local_debug" {
		t.Fatalf("unexpected provider type values: %q, %q", ProviderTypeRemoteHTTP, ProviderTypeLocalDebug)
	}
}

func TestProviderNormalizesPluginScope(t *testing.T) {
	scopes, err := NormalizeScopes([]Scope{ScopePlugin, ScopeAgent, ScopePlugin})
	if err != nil {
		t.Fatalf("NormalizeScopes() error = %v", err)
	}
	if want := []Scope{ScopeAgent, ScopePlugin}; !reflect.DeepEqual(scopes, want) {
		t.Fatalf("NormalizeScopes() = %#v, want %#v", scopes, want)
	}
}

func TestProviderNewRequiresCanonicalKeyAndOwnsLifecycle(t *testing.T) {
	input := validCreateProviderInput()
	provider, err := NewProvider(input)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}

	if provider.ProviderKey != input.ProviderKey {
		t.Fatalf("ProviderKey = %q, want %q", provider.ProviderKey, input.ProviderKey)
	}
	if provider.ID != 0 || provider.Version != 0 || !provider.CreatedAt.IsZero() || !provider.UpdatedAt.IsZero() || provider.DeletedAt != nil {
		t.Fatalf("repository-owned lifecycle was populated: %#v", provider)
	}
	if provider.Status != ProviderStatusDisabled || provider.Health.Status != HealthStatusUnknown {
		t.Fatalf("unsafe initial lifecycle: status=%q health=%q", provider.Status, provider.Health.Status)
	}

	inputType := reflect.TypeOf(CreateProviderInput{})
	for _, field := range []string{"ID", "Version", "CreatedAt", "UpdatedAt", "DeletedAt", "Status", "Health"} {
		if _, exists := inputType.FieldByName(field); exists {
			t.Errorf("CreateProviderInput must not expose repository-owned field %q", field)
		}
	}
}

func TestProviderNewRejectsNonCanonicalProviderKey(t *testing.T) {
	keys := []string{
		"",
		" provider",
		"provider ",
		"Provider",
		"-provider",
		"provider_",
		"provider.key",
		"provider/key",
		strings.Repeat("a", MaxProviderKeyLength+1),
	}

	for index, key := range keys {
		t.Run(fmt.Sprintf("case_%d", index), func(t *testing.T) {
			input := validCreateProviderInput()
			input.ProviderKey = key
			if _, err := NewProvider(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NewProvider(%q) error = %v, want ErrInvalidInput", key, err)
			}
		})
	}
}

func TestProviderKeyExportedValidationPreservesCanonicalRule(t *testing.T) {
	valid := []string{
		"a",
		"provider",
		"provider-1",
		"provider_1",
		strings.Repeat("a", MaxProviderKeyLength),
	}
	for _, providerKey := range valid {
		if err := ValidateProviderKey(providerKey); err != nil {
			t.Fatalf("ValidateProviderKey(%q) error = %v", providerKey, err)
		}
		normalized, err := NormalizeProviderKey(providerKey)
		if err != nil || normalized != providerKey {
			t.Fatalf("NormalizeProviderKey(%q) = %q, %v", providerKey, normalized, err)
		}
	}

	invalid := []string{
		"",
		" provider",
		"provider ",
		"Provider",
		"-provider",
		"provider_",
		"provider.key",
		"provider/key",
		strings.Repeat("a", MaxProviderKeyLength+1),
	}
	for _, providerKey := range invalid {
		if err := ValidateProviderKey(providerKey); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("ValidateProviderKey(%q) error = %v, want ErrInvalidInput", providerKey, err)
		}
		if normalized, err := NormalizeProviderKey(providerKey); normalized != "" || !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeProviderKey(%q) = %q, %v", providerKey, normalized, err)
		}
	}
}

func TestProviderNewRejectsInvalidConfigurationWithoutMutatingInput(t *testing.T) {

	tests := []struct {
		name   string
		mutate func(*CreateProviderInput)
	}{
		{name: "empty name", mutate: func(input *CreateProviderInput) { input.Name = " \t" }},
		{name: "name too long", mutate: func(input *CreateProviderInput) { input.Name = strings.Repeat("a", MaxProviderNameLength+1) }},
		{name: "unknown type", mutate: func(input *CreateProviderInput) { input.Type = ProviderType("container") }},
		{name: "empty scopes", mutate: func(input *CreateProviderInput) { input.Scopes = nil }},
		{name: "unknown scope", mutate: func(input *CreateProviderInput) { input.Scopes = []Scope{"workflow"} }},
		{name: "remote endpoint hint missing", mutate: func(input *CreateProviderInput) { input.EndpointHint = "" }},
		{name: "local endpoint hint present", mutate: func(input *CreateProviderInput) {
			input.Type = ProviderTypeLocalDebug
			input.EndpointHint = "local-only"
		}},
		{name: "endpoint hint too long", mutate: func(input *CreateProviderInput) {
			input.EndpointHint = strings.Repeat("a", MaxEndpointHintLength+1)
		}},
		{name: "endpoint hint contains control character", mutate: func(input *CreateProviderInput) {
			input.EndpointHint = "runner.example.test\nsecret"
		}},
		{name: "invalid runtime policy", mutate: func(input *CreateProviderInput) { input.Policy.TimeoutSeconds = 0 }},
		{name: "missing actor", mutate: func(input *CreateProviderInput) { input.ActorUserID = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validCreateProviderInput()
			input.Name = "  primary runner  "
			input.Scopes = []Scope{ScopeAppDev, ScopeAgent}
			tt.mutate(&input)
			before := cloneCreateProviderInput(input)

			_, err := NewProvider(input)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NewProvider() error = %v, want ErrInvalidInput", err)
			}
			if !reflect.DeepEqual(input, before) {
				t.Fatalf("NewProvider() mutated failed input: got %#v, want %#v", input, before)
			}
		})
	}
}

func TestProviderNewNormalizesAndClonesOwnedSlices(t *testing.T) {
	input := validCreateProviderInput()
	input.Name = "  primary runner  "
	input.Scopes = []Scope{ScopeAppDev, ScopeAgent, ScopeAppDev}
	input.Policy.AllowNetwork = true
	input.Policy.NetworkAllowlist = []string{" Z.example.com ", "a.example.com", "z.example.com."}

	provider, err := NewProvider(input)
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	if provider.Name != "primary runner" {
		t.Fatalf("Name = %q, want normalized name", provider.Name)
	}
	if want := []Scope{ScopeAgent, ScopeAppDev}; !reflect.DeepEqual(provider.Scopes, want) {
		t.Fatalf("Scopes = %#v, want %#v", provider.Scopes, want)
	}
	if want := []string{"a.example.com", "z.example.com"}; !reflect.DeepEqual(provider.Policy.NetworkAllowlist, want) {
		t.Fatalf("NetworkAllowlist = %#v, want %#v", provider.Policy.NetworkAllowlist, want)
	}

	input.Scopes[0] = ScopeMCPStdio
	input.Policy.NetworkAllowlist[0] = "mutated.example.com"
	if !reflect.DeepEqual(provider.Scopes, []Scope{ScopeAgent, ScopeAppDev}) {
		t.Fatalf("provider scopes alias caller input: %#v", provider.Scopes)
	}
	if !reflect.DeepEqual(provider.Policy.NetworkAllowlist, []string{"a.example.com", "z.example.com"}) {
		t.Fatalf("provider allowlist aliases caller input: %#v", provider.Policy.NetworkAllowlist)
	}
}

func TestProviderUpdateOmitsImmutableKeyAndRequiresCASIdentity(t *testing.T) {
	inputType := reflect.TypeOf(UpdateProviderInput{})
	for _, field := range []string{"ProviderKey", "Version", "CreatedAt", "UpdatedAt", "DeletedAt", "Status", "Health"} {
		if _, exists := inputType.FieldByName(field); exists {
			t.Errorf("UpdateProviderInput must not expose immutable or lifecycle field %q", field)
		}
	}
	for _, field := range []string{"ProviderID", "ExpectedVersion"} {
		if _, exists := inputType.FieldByName(field); !exists {
			t.Errorf("UpdateProviderInput is missing CAS field %q", field)
		}
	}

	input := validUpdateProviderInput()
	input.Name = "  renamed runner  "
	input.Scopes = []Scope{ScopeAppDev, ScopeAgent, ScopeAppDev}
	normalized, err := NormalizeUpdateProviderInput(input)
	if err != nil {
		t.Fatalf("NormalizeUpdateProviderInput() error = %v", err)
	}
	if normalized.Name != "renamed runner" || !reflect.DeepEqual(normalized.Scopes, []Scope{ScopeAgent, ScopeAppDev}) {
		t.Fatalf("unexpected normalized update: %#v", normalized)
	}

	for _, mutate := range []func(*UpdateProviderInput){
		func(input *UpdateProviderInput) { input.ProviderID = 0 },
		func(input *UpdateProviderInput) { input.ExpectedVersion = 0 },
	} {
		invalid := validUpdateProviderInput()
		invalid.Name = "  renamed runner  "
		invalid.Scopes = []Scope{ScopeAppDev, ScopeAgent}
		mutate(&invalid)
		before := cloneUpdateProviderInput(invalid)
		if _, err := NormalizeUpdateProviderInput(invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeUpdateProviderInput() error = %v, want ErrInvalidInput", err)
		}
		if !reflect.DeepEqual(invalid, before) {
			t.Fatalf("NormalizeUpdateProviderInput() mutated failed input")
		}
	}
}

func TestProviderLifecycleCommandsRequireIdentityAndExpectedVersion(t *testing.T) {
	statusInput := UpdateProviderStatusInput{
		ProviderID:      11,
		ExpectedVersion: 7,
		Status:          ProviderStatusEnabled,
		ActorUserID:     42,
	}
	if _, err := NormalizeUpdateProviderStatusInput(statusInput); err != nil {
		t.Fatalf("NormalizeUpdateProviderStatusInput() error = %v", err)
	}
	for _, mutate := range []func(*UpdateProviderStatusInput){
		func(input *UpdateProviderStatusInput) { input.ProviderID = 0 },
		func(input *UpdateProviderStatusInput) { input.ExpectedVersion = 0 },
		func(input *UpdateProviderStatusInput) { input.Status = ProviderStatus("ready") },
		func(input *UpdateProviderStatusInput) { input.ActorUserID = 0 },
	} {
		invalid := statusInput
		mutate(&invalid)
		if _, err := NormalizeUpdateProviderStatusInput(invalid); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("NormalizeUpdateProviderStatusInput() error = %v, want ErrInvalidInput", err)
		}
	}

	deleteInput := DeleteProviderInput{ProviderID: 11, ExpectedVersion: 7, ActorUserID: 42}
	if err := ValidateDeleteProviderInput(deleteInput); err != nil {
		t.Fatalf("ValidateDeleteProviderInput() error = %v", err)
	}
	deleteInput.ExpectedVersion = 0
	if err := ValidateDeleteProviderInput(deleteInput); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ValidateDeleteProviderInput() error = %v, want ErrInvalidInput", err)
	}
}

func TestProviderDefaultSetEnforcesAbsentCreateExistingCASAndNoReset(t *testing.T) {
	createInput := SetProviderDefaultInput{
		Scope:           ScopeAgent,
		ProviderID:      11,
		ExpectedVersion: 0,
		ActorUserID:     42,
	}
	normalized, nextVersion, err := NormalizeSetProviderDefaultInput(createInput, nil)
	if err != nil {
		t.Fatalf("NormalizeSetProviderDefaultInput(absent create) error = %v", err)
	}
	if normalized != createInput || nextVersion != InitialVersion {
		t.Fatalf("absent create = %#v, version %d", normalized, nextVersion)
	}

	existing := &ProviderDefault{Scope: ScopeAgent, ProviderID: 11, Version: 7}
	updateInput := createInput
	updateInput.ProviderID = 12
	updateInput.ExpectedVersion = existing.Version
	normalized, nextVersion, err = NormalizeSetProviderDefaultInput(updateInput, existing)
	if err != nil {
		t.Fatalf("NormalizeSetProviderDefaultInput(existing update) error = %v", err)
	}
	if normalized != updateInput || nextVersion != 8 {
		t.Fatalf("existing update = %#v, version %d", normalized, nextVersion)
	}

	missingWithVersion := createInput
	missingWithVersion.ExpectedVersion = 7
	if _, _, err := NormalizeSetProviderDefaultInput(missingWithVersion, nil); !errors.Is(err, ErrDefaultMissing) {
		t.Fatalf("missing existing default error = %v, want ErrDefaultMissing", err)
	}

	reset := updateInput
	reset.ExpectedVersion = 0
	if _, _, err := NormalizeSetProviderDefaultInput(reset, existing); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("existing default reset error = %v, want ErrVersionConflict", err)
	}

	stale := updateInput
	stale.ExpectedVersion = 6
	if _, _, err := NormalizeSetProviderDefaultInput(stale, existing); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale default update error = %v, want ErrVersionConflict", err)
	}

	advanced := *existing
	advanced.Version = 8
	oldVersion := updateInput
	oldVersion.ExpectedVersion = 7
	if _, _, err := NormalizeSetProviderDefaultInput(oldVersion, &advanced); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("ABA default update error = %v, want ErrVersionConflict", err)
	}

	repositoryType := reflect.TypeOf((*ProviderDefaultRepository)(nil)).Elem()
	if _, exists := repositoryType.MethodByName("DeleteProviderDefault"); exists {
		t.Fatal("ProviderDefaultRepository must not expose deletion/recreation")
	}
}

func TestProviderErrorCodesAreStableSafeAndUnspoofable(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{ErrInvalidInput, "SANDBOX_INVALID_INPUT"},
		{ErrProviderNotFound, "SANDBOX_PROVIDER_NOT_FOUND"},
		{ErrProviderDisabled, "SANDBOX_PROVIDER_DISABLED"},
		{ErrDefaultMissing, "SANDBOX_DEFAULT_MISSING"},
		{ErrProviderUnhealthy, "SANDBOX_PROVIDER_UNHEALTHY"},
		{ErrCredentialInvalid, "SANDBOX_CREDENTIAL_INVALID"},
		{ErrCapacityExhausted, "SANDBOX_CAPACITY_EXHAUSTED"},
		{ErrExecutionForbidden, "SANDBOX_EXECUTION_FORBIDDEN"},
		{ErrVersionConflict, "SANDBOX_VERSION_CONFLICT"},
		{ErrProviderAlreadyExists, "SANDBOX_PROVIDER_ALREADY_EXISTS"},
		{ErrProviderInUse, "SANDBOX_PROVIDER_IN_USE"},
	}

	for _, tt := range tests {
		wrapped := fmt.Errorf("wrapped: %w", tt.err)
		if !errors.Is(wrapped, tt.err) {
			t.Errorf("errors.Is(%v, %v) = false", wrapped, tt.err)
		}
		if got := ErrorCodeOf(wrapped); got != tt.code {
			t.Errorf("ErrorCodeOf(%v) = %q, want %q", tt.err, got, tt.code)
		}
		message := strings.ToLower(tt.err.Error())
		for _, forbidden := range []string{"http://", "https://", "/tmp/", "select ", "curl ", "token="} {
			if strings.Contains(message, forbidden) {
				t.Errorf("error %q contains forbidden internal detail %q", message, forbidden)
			}
		}
	}

	foreign := foreignCodedError{code: ErrCodeProviderInUse}
	if got := ErrorCodeOf(foreign); got != "" {
		t.Fatalf("foreign coded error spoofed sandbox code: %q", got)
	}
}

func TestProviderDeleteContractRetainsCASSurfaceForProviderInUse(t *testing.T) {
	var repository ProviderRepository = (*transactionProviderRepository)(nil)
	var contract interface {
		DeleteProvider(context.Context, DeleteProviderInput) (uint64, error)
	} = repository
	if contract == nil {
		t.Fatal("ProviderRepository does not expose DeleteProvider CAS contract")
	}
}

func TestProviderHealthSnapshotEnforcesConsistencyAndClonesCapabilities(t *testing.T) {
	checkedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	snapshot := HealthSnapshot{
		Status:        HealthStatusHealthy,
		Capabilities:  []Scope{ScopeMCPStdio, ScopeAgent, ScopeMCPStdio},
		ReasonCode:    "HEALTH_OK",
		LatencyMillis: 12,
		CheckedAt:     checkedAt,
	}
	normalized, err := NormalizeHealthSnapshot(snapshot, []Scope{ScopeAgent, ScopeMCPStdio})
	if err != nil {
		t.Fatalf("NormalizeHealthSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(normalized.Capabilities, []Scope{ScopeAgent, ScopeMCPStdio}) {
		t.Fatalf("Capabilities = %#v", normalized.Capabilities)
	}
	if normalized.CheckedAt.Location() != time.UTC || !normalized.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckedAt = %v, want same instant in UTC", normalized.CheckedAt)
	}
	snapshot.Capabilities[0] = ScopeAppDev
	if !reflect.DeepEqual(normalized.Capabilities, []Scope{ScopeAgent, ScopeMCPStdio}) {
		t.Fatalf("normalized capabilities alias input: %#v", normalized.Capabilities)
	}

	tests := []struct {
		name     string
		snapshot HealthSnapshot
		scopes   []Scope
	}{
		{name: "healthy without checked time", snapshot: HealthSnapshot{Status: HealthStatusHealthy, LatencyMillis: 0}, scopes: []Scope{ScopeAgent}},
		{name: "degraded without checked time", snapshot: HealthSnapshot{Status: HealthStatusDegraded, ReasonCode: "HEALTH_DEGRADED", LatencyMillis: 0}, scopes: []Scope{ScopeAgent}},
		{name: "unhealthy without checked time", snapshot: HealthSnapshot{Status: HealthStatusUnhealthy, ReasonCode: "HEALTH_UNHEALTHY", LatencyMillis: 0}, scopes: []Scope{ScopeAgent}},
		{name: "healthy with negative latency", snapshot: HealthSnapshot{Status: HealthStatusHealthy, CheckedAt: checkedAt, LatencyMillis: -1}, scopes: []Scope{ScopeAgent}},
		{name: "degraded with negative latency", snapshot: HealthSnapshot{Status: HealthStatusDegraded, ReasonCode: "HEALTH_DEGRADED", CheckedAt: checkedAt, LatencyMillis: -1}, scopes: []Scope{ScopeAgent}},
		{name: "unhealthy with negative latency", snapshot: HealthSnapshot{Status: HealthStatusUnhealthy, ReasonCode: "HEALTH_UNHEALTHY", CheckedAt: checkedAt, LatencyMillis: -1}, scopes: []Scope{ScopeAgent}},
		{name: "unknown with negative latency", snapshot: HealthSnapshot{Status: HealthStatusUnknown, LatencyMillis: -1}, scopes: []Scope{ScopeAgent}},
		{name: "unknown with nonzero latency", snapshot: HealthSnapshot{Status: HealthStatusUnknown, LatencyMillis: 1}, scopes: []Scope{ScopeAgent}},
		{name: "unknown with checked time", snapshot: HealthSnapshot{Status: HealthStatusUnknown, CheckedAt: checkedAt}, scopes: []Scope{ScopeAgent}},
		{name: "unknown with capabilities", snapshot: HealthSnapshot{Status: HealthStatusUnknown, Capabilities: []Scope{ScopeAgent}}, scopes: []Scope{ScopeAgent}},
		{name: "capability outside provider scopes", snapshot: HealthSnapshot{Status: HealthStatusHealthy, CheckedAt: checkedAt, Capabilities: []Scope{ScopeAppDev}}, scopes: []Scope{ScopeAgent}},
		{name: "unknown health enum", snapshot: HealthSnapshot{Status: HealthStatus("passing")}, scopes: []Scope{ScopeAgent}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NormalizeHealthSnapshot(tt.snapshot, tt.scopes); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeHealthSnapshot() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestProviderHealthSnapshotAcceptsCheckedResultsAndNeverCheckedUnknown(t *testing.T) {
	checkedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	for _, status := range []HealthStatus{
		HealthStatusHealthy,
		HealthStatusDegraded,
		HealthStatusUnhealthy,
	} {
		reasonCode := "HEALTH_OK"
		if status == HealthStatusDegraded {
			reasonCode = "HEALTH_DEGRADED"
		}
		if status == HealthStatusUnhealthy {
			reasonCode = "HEALTH_UNHEALTHY"
		}
		snapshot := HealthSnapshot{
			Status:        status,
			Capabilities:  []Scope{ScopeAgent},
			ReasonCode:    reasonCode,
			LatencyMillis: 0,
			CheckedAt:     checkedAt,
		}
		if _, err := NormalizeHealthSnapshot(snapshot, []Scope{ScopeAgent}); err != nil {
			t.Fatalf("NormalizeHealthSnapshot(%q) error = %v", status, err)
		}
	}

	unknown := HealthSnapshot{Status: HealthStatusUnknown}
	normalized, err := NormalizeHealthSnapshot(unknown, []Scope{ScopeAgent})
	if err != nil {
		t.Fatalf("NormalizeHealthSnapshot(unknown) error = %v", err)
	}
	if !normalized.CheckedAt.IsZero() || normalized.LatencyMillis != 0 || len(normalized.Capabilities) != 0 {
		t.Fatalf("normalized unknown health is not never-checked: %#v", normalized)
	}
}

func TestProviderHealthDiagnosticsAreBoundedAndSanitized(t *testing.T) {
	checkedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	snapshot := HealthSnapshot{
		Status:        HealthStatusHealthy,
		Capabilities:  []Scope{ScopeAgent},
		ReasonCode:    "HEALTH_OK",
		Message:       "  runner healthy  ",
		LatencyMillis: 1,
		CheckedAt:     checkedAt,
	}
	normalized, err := NormalizeHealthSnapshot(snapshot, []Scope{ScopeAgent})
	if err != nil {
		t.Fatalf("NormalizeHealthSnapshot() error = %v", err)
	}
	if normalized.ReasonCode != "HEALTH_OK" || normalized.Message != "runner healthy" {
		t.Fatalf("normalized diagnostics = reason %q message %q", normalized.ReasonCode, normalized.Message)
	}
	if snapshot.Message != "  runner healthy  " {
		t.Fatal("NormalizeHealthSnapshot() mutated input diagnostics")
	}

	edge := snapshot
	edge.ReasonCode = strings.Repeat("A", MaxHealthReasonCodeLength)
	edge.Message = strings.Repeat("m", MaxHealthMessageLength)
	if _, err := NormalizeHealthSnapshot(edge, []Scope{ScopeAgent}); err != nil {
		t.Fatalf("NormalizeHealthSnapshot(max diagnostics) error = %v", err)
	}

	healthyWithoutReason := snapshot
	healthyWithoutReason.ReasonCode = ""
	if _, err := NormalizeHealthSnapshot(healthyWithoutReason, []Scope{ScopeAgent}); err != nil {
		t.Fatalf("healthy empty reason error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*HealthSnapshot)
	}{
		{name: "reason too long", mutate: func(health *HealthSnapshot) {
			health.ReasonCode = strings.Repeat("A", MaxHealthReasonCodeLength+1)
		}},
		{name: "reason not stable ASCII", mutate: func(health *HealthSnapshot) { health.ReasonCode = "health-ok" }},
		{name: "reason contains surrounding space", mutate: func(health *HealthSnapshot) { health.ReasonCode = " HEALTH_OK" }},
		{name: "degraded missing reason", mutate: func(health *HealthSnapshot) {
			health.Status = HealthStatusDegraded
			health.ReasonCode = ""
		}},
		{name: "unhealthy missing reason", mutate: func(health *HealthSnapshot) {
			health.Status = HealthStatusUnhealthy
			health.ReasonCode = ""
		}},
		{name: "unknown has reason", mutate: func(health *HealthSnapshot) {
			health.Status = HealthStatusUnknown
			health.ReasonCode = "NEVER_CHECKED"
			health.CheckedAt = time.Time{}
			health.LatencyMillis = 0
			health.Capabilities = nil
		}},
		{name: "message too long", mutate: func(health *HealthSnapshot) {
			health.Message = strings.Repeat("m", MaxHealthMessageLength+1)
		}},
		{name: "message invalid UTF-8", mutate: func(health *HealthSnapshot) {
			health.Message = string([]byte{0xff})
		}},
		{name: "message C0 control", mutate: func(health *HealthSnapshot) { health.Message = "bad\x1fmessage" }},
		{name: "message DEL control", mutate: func(health *HealthSnapshot) { health.Message = "bad\x7fmessage" }},
		{name: "message C1 control", mutate: func(health *HealthSnapshot) { health.Message = "bad\u0085message" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invalid := snapshot
			invalid.Capabilities = append([]Scope(nil), snapshot.Capabilities...)
			tt.mutate(&invalid)
			if _, err := NormalizeHealthSnapshot(invalid, []Scope{ScopeAgent}); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeHealthSnapshot() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestProviderHealthUpdateInputNormalizesAndClonesCapabilities(t *testing.T) {
	checkedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	configuredScopes := []Scope{ScopeMCPStdio, ScopeAgent}
	input := UpdateProviderHealthInput{
		ProviderID:      11,
		ExpectedVersion: 7,
		ActorUserID:     42,
		Health: HealthSnapshot{
			Status:        HealthStatusHealthy,
			Capabilities:  []Scope{ScopeMCPStdio, ScopeAgent, ScopeMCPStdio},
			ReasonCode:    "HEALTH_OK",
			LatencyMillis: 12,
			CheckedAt:     checkedAt,
		},
	}

	normalized, err := NormalizeUpdateProviderHealthInput(input, configuredScopes)
	if err != nil {
		t.Fatalf("NormalizeUpdateProviderHealthInput() error = %v", err)
	}
	if !reflect.DeepEqual(normalized.Health.Capabilities, []Scope{ScopeAgent, ScopeMCPStdio}) {
		t.Fatalf("Capabilities = %#v", normalized.Health.Capabilities)
	}
	if normalized.Health.CheckedAt.Location() != time.UTC || !normalized.Health.CheckedAt.Equal(checkedAt) {
		t.Fatalf("CheckedAt = %v, want same instant in UTC", normalized.Health.CheckedAt)
	}

	input.Health.Capabilities[0] = ScopeAppDev
	configuredScopes[0] = ScopeAppDev
	if !reflect.DeepEqual(normalized.Health.Capabilities, []Scope{ScopeAgent, ScopeMCPStdio}) {
		t.Fatalf("normalized capabilities alias caller slices: %#v", normalized.Health.Capabilities)
	}
}

func TestProviderHealthUpdateInputRejectsInvalidIdentityAndCapabilities(t *testing.T) {
	checkedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	valid := UpdateProviderHealthInput{
		ProviderID:      11,
		ExpectedVersion: 7,
		ActorUserID:     42,
		Health: HealthSnapshot{
			Status:        HealthStatusHealthy,
			Capabilities:  []Scope{ScopeAgent},
			LatencyMillis: 1,
			CheckedAt:     checkedAt,
		},
	}
	tests := []struct {
		name   string
		mutate func(*UpdateProviderHealthInput)
		scopes []Scope
	}{
		{name: "missing provider ID", mutate: func(input *UpdateProviderHealthInput) { input.ProviderID = 0 }, scopes: []Scope{ScopeAgent}},
		{name: "missing expected version", mutate: func(input *UpdateProviderHealthInput) { input.ExpectedVersion = 0 }, scopes: []Scope{ScopeAgent}},
		{name: "missing actor ID", mutate: func(input *UpdateProviderHealthInput) { input.ActorUserID = 0 }, scopes: []Scope{ScopeAgent}},
		{name: "capability outside configured scopes", mutate: func(input *UpdateProviderHealthInput) {
			input.Health.Capabilities = []Scope{ScopeAppDev}
		}, scopes: []Scope{ScopeAgent}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			input.Health.Capabilities = append([]Scope(nil), valid.Health.Capabilities...)
			tt.mutate(&input)
			before := input
			before.Health.Capabilities = append([]Scope(nil), input.Health.Capabilities...)

			if _, err := NormalizeUpdateProviderHealthInput(input, tt.scopes); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeUpdateProviderHealthInput() error = %v, want ErrInvalidInput", err)
			}
			if !reflect.DeepEqual(input, before) {
				t.Fatal("NormalizeUpdateProviderHealthInput() mutated failed input")
			}
		})
	}
}

func TestProviderHealthUsabilityFailsClosed(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	base := HealthSnapshot{
		Status:        HealthStatusHealthy,
		Capabilities:  []Scope{ScopeAgent},
		LatencyMillis: 10,
		CheckedAt:     now.Add(-5 * time.Minute),
	}
	if !HealthUsableForScope(base, ScopeAgent, now, 10*time.Minute) {
		t.Fatal("fresh healthy capability should be usable")
	}
	atBoundary := base
	atBoundary.CheckedAt = now.Add(-10 * time.Minute)
	if !HealthUsableForScope(atBoundary, ScopeAgent, now, 10*time.Minute) {
		t.Fatal("health at freshness boundary should be usable")
	}

	tests := []struct {
		name   string
		health HealthSnapshot
		scope  Scope
		now    time.Time
		maxAge time.Duration
	}{
		{name: "degraded", health: func() HealthSnapshot { h := base; h.Status = HealthStatusDegraded; return h }(), scope: ScopeAgent, now: now, maxAge: 10 * time.Minute},
		{name: "stale", health: func() HealthSnapshot { h := base; h.CheckedAt = now.Add(-11 * time.Minute); return h }(), scope: ScopeAgent, now: now, maxAge: 10 * time.Minute},
		{name: "future", health: func() HealthSnapshot { h := base; h.CheckedAt = now.Add(time.Second); return h }(), scope: ScopeAgent, now: now, maxAge: 10 * time.Minute},
		{name: "zero checked time", health: func() HealthSnapshot { h := base; h.CheckedAt = time.Time{}; return h }(), scope: ScopeAgent, now: now, maxAge: 10 * time.Minute},
		{name: "negative latency", health: func() HealthSnapshot { h := base; h.LatencyMillis = -1; return h }(), scope: ScopeAgent, now: now, maxAge: 10 * time.Minute},
		{name: "missing capability", health: base, scope: ScopeMCPStdio, now: now, maxAge: 10 * time.Minute},
		{name: "unknown scope", health: base, scope: Scope("workflow"), now: now, maxAge: 10 * time.Minute},
		{name: "zero current time", health: base, scope: ScopeAgent, now: time.Time{}, maxAge: 10 * time.Minute},
		{name: "non-positive max age", health: base, scope: ScopeAgent, now: now, maxAge: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HealthUsableForScope(tt.health, tt.scope, tt.now, tt.maxAge) {
				t.Fatal("HealthUsableForScope() = true, want fail closed")
			}
		})
	}
}

func TestProviderAuditMetadataIsBoundedAllowlistedAndIsolated(t *testing.T) {
	input := map[string]string{
		AuditMetadataKeyScope:          "agent",
		AuditMetadataKeyChangedFields:  "policy, name,policy",
		AuditMetadataKeyPreviousStatus: "disabled",
		AuditMetadataKeyNewStatus:      "enabled",
		AuditMetadataKeyHealthCode:     "HEALTH_OK",
		AuditMetadataKeyVersion:        "007",
	}
	normalized, err := NormalizeAuditMetadata(input)
	if err != nil {
		t.Fatalf("NormalizeAuditMetadata() error = %v", err)
	}
	want := map[string]string{
		AuditMetadataKeyScope:          "agent",
		AuditMetadataKeyChangedFields:  "name,policy",
		AuditMetadataKeyPreviousStatus: "disabled",
		AuditMetadataKeyNewStatus:      "enabled",
		AuditMetadataKeyHealthCode:     "HEALTH_OK",
		AuditMetadataKeyVersion:        "7",
	}
	if !reflect.DeepEqual(normalized, want) {
		t.Fatalf("NormalizeAuditMetadata() = %#v, want %#v", normalized, want)
	}
	input[AuditMetadataKeyScope] = "appdev"
	if normalized[AuditMetadataKeyScope] != "agent" {
		t.Fatal("normalized metadata aliases caller map")
	}
	again, err := NormalizeAuditMetadata(normalized)
	if err != nil || !reflect.DeepEqual(again, normalized) {
		t.Fatalf("metadata normalization is not idempotent: %#v, %v", again, err)
	}

	invalid := []map[string]string{
		{"credential": "secret"},
		{AuditMetadataKeyScope: "workflow"},
		{AuditMetadataKeyChangedFields: "credential_value"},
		{AuditMetadataKeyPreviousStatus: "ready"},
		{AuditMetadataKeyNewStatus: "ready"},
		{AuditMetadataKeyHealthCode: "https://internal.example"},
		{AuditMetadataKeyHealthCode: strings.Repeat("A", MaxAuditHealthCodeLength+1)},
		{AuditMetadataKeyVersion: "not-a-version"},
	}
	for index, metadata := range invalid {
		t.Run(fmt.Sprintf("invalid_%d", index), func(t *testing.T) {
			before := cloneMetadata(metadata)
			if _, err := NormalizeAuditMetadata(metadata); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeAuditMetadata() error = %v, want ErrInvalidInput", err)
			}
			if !reflect.DeepEqual(metadata, before) {
				t.Fatal("NormalizeAuditMetadata() mutated failed input")
			}
		})
	}

	metadata := map[string]string{AuditMetadataKeyScope: "agent", AuditMetadataKeyVersion: "8"}
	event, err := NewProviderAuditEvent(AppendProviderAuditEventInput{
		ProviderID:  11,
		ActorUserID: 42,
		Action:      "provider.update",
		Result:      "success",
		RequestID:   "request-1",
		Metadata:    metadata,
	})
	if err != nil {
		t.Fatalf("NewProviderAuditEvent() error = %v", err)
	}
	metadata[AuditMetadataKeyScope] = "appdev"
	if event.Metadata[AuditMetadataKeyScope] != "agent" {
		t.Fatal("audit event metadata aliases caller map")
	}
	if event.ID != 0 || !event.CreatedAt.IsZero() {
		t.Fatalf("repository-owned audit lifecycle populated: %#v", event)
	}
}

func TestProviderAuditBoundaryBoundsInputAndRemainsAppendOnly(t *testing.T) {
	valid := AppendProviderAuditEventInput{
		ProviderID:  11,
		ActorUserID: 42,
		Action:      strings.Repeat("A", MaxAuditActionLength),
		Result:      strings.Repeat("R", MaxAuditResultLength),
		RequestID:   strings.Repeat("Q", MaxAuditRequestIDLength),
		Metadata:    map[string]string{AuditMetadataKeyVersion: "7"},
	}
	normalized, err := NormalizeAppendProviderAuditEventInput(valid)
	if err != nil {
		t.Fatalf("NormalizeAppendProviderAuditEventInput() error = %v", err)
	}
	if normalized.Action != valid.Action || normalized.Result != valid.Result || normalized.RequestID != valid.RequestID {
		t.Fatalf("normalized audit input = %#v", normalized)
	}

	tests := []struct {
		name   string
		mutate func(*AppendProviderAuditEventInput)
	}{
		{name: "action too long", mutate: func(input *AppendProviderAuditEventInput) {
			input.Action = strings.Repeat("A", MaxAuditActionLength+1)
		}},
		{name: "result too long", mutate: func(input *AppendProviderAuditEventInput) {
			input.Result = strings.Repeat("R", MaxAuditResultLength+1)
		}},
		{name: "request ID too long", mutate: func(input *AppendProviderAuditEventInput) {
			input.RequestID = strings.Repeat("Q", MaxAuditRequestIDLength+1)
		}},
		{name: "action control character", mutate: func(input *AppendProviderAuditEventInput) {
			input.Action = "provider\nupdate"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := valid
			input.Metadata = cloneMetadata(valid.Metadata)
			tt.mutate(&input)
			if _, err := NormalizeAppendProviderAuditEventInput(input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeAppendProviderAuditEventInput() error = %v, want ErrInvalidInput", err)
			}
		})
	}

	repositoryType := reflect.TypeOf((*ProviderAuditRepository)(nil)).Elem()
	if repositoryType.NumMethod() != 2 {
		t.Fatalf("audit repository must remain append-only, methods = %d", repositoryType.NumMethod())
	}
	for _, method := range []string{"AppendProviderAuditEvent", "ListProviderAuditEvents"} {
		if _, exists := repositoryType.MethodByName(method); !exists {
			t.Fatalf("audit repository missing %s", method)
		}
	}
}

func TestProviderRepositoryBoundaryNormalizersRejectInvalidMutations(t *testing.T) {
	create := validCreateProviderInput()
	create.ProviderKey = "INVALID"
	if _, err := NormalizeCreateProviderInput(create); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeCreateProviderInput() error = %v, want ErrInvalidInput", err)
	}

	update := validUpdateProviderInput()
	update.Name = ""
	if _, err := NormalizeUpdateProviderInput(update); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeUpdateProviderInput() error = %v, want ErrInvalidInput", err)
	}

	status := UpdateProviderStatusInput{
		ProviderID: 11, ExpectedVersion: 7, Status: ProviderStatus("ready"), ActorUserID: 42,
	}
	if _, err := NormalizeUpdateProviderStatusInput(status); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeUpdateProviderStatusInput() error = %v, want ErrInvalidInput", err)
	}

	health := UpdateProviderHealthInput{
		ProviderID: 11, ExpectedVersion: 7, ActorUserID: 42,
		Health: HealthSnapshot{
			Status: HealthStatusHealthy, CheckedAt: time.Now().UTC(), Capabilities: []Scope{ScopeAppDev},
		},
	}
	if _, err := NormalizeUpdateProviderHealthInput(health, []Scope{ScopeAgent}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeUpdateProviderHealthInput() error = %v, want ErrInvalidInput", err)
	}

	if err := ValidateDeleteProviderInput(DeleteProviderInput{ProviderID: 11, ActorUserID: 42}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ValidateDeleteProviderInput() error = %v, want ErrInvalidInput", err)
	}

	defaultInput := SetProviderDefaultInput{Scope: ScopeAgent, ProviderID: 11, ActorUserID: 0}
	if _, _, err := NormalizeSetProviderDefaultInput(defaultInput, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeSetProviderDefaultInput() error = %v, want ErrInvalidInput", err)
	}

	audit := AppendProviderAuditEventInput{
		ProviderID: 11, ActorUserID: 42, Action: "provider.update", Result: "success", RequestID: "request-1",
		Metadata: map[string]string{"secret": "value"},
	}
	if _, err := NormalizeAppendProviderAuditEventInput(audit); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("NormalizeAppendProviderAuditEventInput() error = %v, want ErrInvalidInput", err)
	}
}

func TestProviderUnitOfWorkSuppliesAtomicRepositoriesAndCASContracts(t *testing.T) {
	state := &transactionCallState{}
	providers := &transactionProviderRepository{state: state, version: 7}
	defaults := &transactionDefaultRepository{state: state}
	audits := &transactionAuditRepository{state: state}
	uow := &transactionUnitOfWork{
		repositories: TransactionRepositories{
			Providers: providers,
			Defaults:  defaults,
			Audits:    audits,
		},
	}
	var _ UnitOfWork = uow

	err := uow.WithinTransaction(context.Background(), func(ctx context.Context, repositories TransactionRepositories) error {
		nextVersion, err := repositories.Providers.UpdateProviderStatus(ctx, UpdateProviderStatusInput{
			ProviderID:      11,
			ExpectedVersion: 7,
			Status:          ProviderStatusEnabled,
			ActorUserID:     42,
		})
		if err != nil {
			return err
		}
		if nextVersion != 8 {
			t.Fatalf("provider next version = %d, want 8", nextVersion)
		}

		providerDefault, err := repositories.Defaults.SetProviderDefault(ctx, SetProviderDefaultInput{
			Scope:           ScopeAgent,
			ProviderID:      11,
			ExpectedVersion: 0,
			ActorUserID:     42,
		})
		if err != nil {
			return err
		}
		if providerDefault.Version != InitialVersion {
			t.Fatalf("default initial version = %d, want %d", providerDefault.Version, InitialVersion)
		}

		_, err = repositories.Audits.AppendProviderAuditEvent(ctx, AppendProviderAuditEventInput{
			ProviderID:  11,
			ActorUserID: 42,
			Action:      "provider.enable",
			Result:      "success",
			RequestID:   "request-1",
			Metadata: map[string]string{
				AuditMetadataKeyNewStatus: "enabled",
				AuditMetadataKeyVersion:   "8",
			},
		})
		return err
	})
	if err != nil {
		t.Fatalf("WithinTransaction() error = %v", err)
	}
	if uow.callbacks != 1 || state.providerMutations != 1 || state.defaultMutations != 1 || state.auditAppends != 1 {
		t.Fatalf("transaction did not use all repositories atomically: uow=%d state=%#v", uow.callbacks, state)
	}

	if _, err := providers.UpdateProviderStatus(context.Background(), UpdateProviderStatusInput{
		ProviderID: 11, ExpectedVersion: 6, Status: ProviderStatusEnabled, ActorUserID: 42,
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale provider update error = %v, want ErrVersionConflict", err)
	}
	if _, err := providers.UpdateProviderStatus(context.Background(), UpdateProviderStatusInput{
		ProviderID: 99, ExpectedVersion: 8, Status: ProviderStatusEnabled, ActorUserID: 42,
	}); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("missing provider update error = %v, want ErrProviderNotFound", err)
	}
}

func TestProviderUnitOfWorkPropagatesCallbackErrorExactlyOnce(t *testing.T) {
	uow := &transactionUnitOfWork{}
	callbackError := errors.New("callback failed")
	err := uow.WithinTransaction(context.Background(), func(context.Context, TransactionRepositories) error {
		return callbackError
	})
	if !errors.Is(err, callbackError) {
		t.Fatalf("WithinTransaction() error = %v, want callback error", err)
	}
	if uow.callbacks != 1 {
		t.Fatalf("WithinTransaction() callback count = %d, want 1", uow.callbacks)
	}
}

func validCreateProviderInput() CreateProviderInput {
	return CreateProviderInput{
		ProviderKey:    "provider_primary-01",
		Name:           "primary runner",
		Type:           ProviderTypeRemoteHTTP,
		EndpointSecret: "encrypted:endpoint",
		EndpointHint:   "https://***.test",
		Scopes:         []Scope{ScopeAgent, ScopeMCPStdio},
		Policy:         validRuntimePolicy(),
		ActorUserID:    42,
	}
}

func validUpdateProviderInput() UpdateProviderInput {
	return UpdateProviderInput{
		ProviderID:      11,
		ExpectedVersion: 7,
		Name:            "renamed runner",
		Type:            ProviderTypeRemoteHTTP,
		EndpointSecret:  "encrypted:endpoint",
		EndpointHint:    "https://***.test",
		Scopes:          []Scope{ScopeAgent, ScopeMCPStdio},
		Policy:          validRuntimePolicy(),
		ActorUserID:     42,
	}
}

func cloneCreateProviderInput(input CreateProviderInput) CreateProviderInput {
	cloned := input
	cloned.Scopes = append([]Scope(nil), input.Scopes...)
	cloned.Policy.NetworkAllowlist = append([]string(nil), input.Policy.NetworkAllowlist...)
	return cloned
}

func cloneUpdateProviderInput(input UpdateProviderInput) UpdateProviderInput {
	cloned := input
	cloned.Scopes = append([]Scope(nil), input.Scopes...)
	cloned.Policy.NetworkAllowlist = append([]string(nil), input.Policy.NetworkAllowlist...)
	return cloned
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

type foreignCodedError struct {
	code string
}

func (e foreignCodedError) Error() string {
	return "foreign error"
}

func (e foreignCodedError) Code() string {
	return e.code
}

type transactionCallState struct {
	providerMutations int
	defaultMutations  int
	auditAppends      int
}

type transactionProviderRepository struct {
	state   *transactionCallState
	version uint64
}

func (r *transactionProviderRepository) CreateProvider(_ context.Context, input CreateProviderInput) (*Provider, error) {
	provider, err := NewProvider(input)
	if err != nil {
		return nil, err
	}
	provider.ID = 11
	provider.Version = InitialVersion
	provider.CreatedAt = time.Now().UTC()
	provider.UpdatedAt = provider.CreatedAt
	return provider, nil
}

func (r *transactionProviderRepository) GetProvider(_ context.Context, providerID int64) (*Provider, error) {
	if providerID != 11 {
		return nil, ErrProviderNotFound
	}
	return &Provider{ID: providerID, Version: r.version}, nil
}

func (r *transactionProviderRepository) GetProviderForUpdate(ctx context.Context, providerID int64) (*Provider, error) {
	return r.GetProvider(ctx, providerID)
}

func (r *transactionProviderRepository) GetProviderByKey(_ context.Context, providerKey string) (*Provider, error) {
	if providerKey != "provider_primary-01" {
		return nil, ErrProviderNotFound
	}
	return &Provider{ID: 11, ProviderKey: providerKey, Version: r.version}, nil
}

func (r *transactionProviderRepository) ListProviders(_ context.Context, _ ProviderListRequest) ([]*Provider, int64, error) {
	return nil, 0, nil
}

func (r *transactionProviderRepository) UpdateProvider(_ context.Context, input UpdateProviderInput) (*Provider, error) {
	next, err := r.mutate(input.ProviderID, input.ExpectedVersion)
	if err != nil {
		return nil, err
	}
	return &Provider{ID: input.ProviderID, Version: next, Name: input.Name}, nil
}

func (r *transactionProviderRepository) UpdateProviderStatus(_ context.Context, input UpdateProviderStatusInput) (uint64, error) {
	return r.mutate(input.ProviderID, input.ExpectedVersion)
}

func (r *transactionProviderRepository) UpdateProviderHealth(_ context.Context, input UpdateProviderHealthInput) (uint64, error) {
	return r.mutate(input.ProviderID, input.ExpectedVersion)
}

func (r *transactionProviderRepository) DeleteProvider(_ context.Context, input DeleteProviderInput) (uint64, error) {
	return r.mutate(input.ProviderID, input.ExpectedVersion)
}

func (r *transactionProviderRepository) mutate(providerID int64, expectedVersion uint64) (uint64, error) {
	if providerID != 11 {
		return 0, ErrProviderNotFound
	}
	if expectedVersion != r.version {
		return 0, ErrVersionConflict
	}
	next, err := NextVersion(expectedVersion)
	if err != nil {
		return 0, err
	}
	r.version = next
	r.state.providerMutations++
	return next, nil
}

type transactionDefaultRepository struct {
	state   *transactionCallState
	current *ProviderDefault
}

func (r *transactionDefaultRepository) GetProviderDefault(_ context.Context, _ Scope) (*ProviderDefault, error) {
	return nil, ErrDefaultMissing
}

func (r *transactionDefaultRepository) ListProviderDefaults(_ context.Context) ([]*ProviderDefault, error) {
	return nil, nil
}

func (r *transactionDefaultRepository) SetProviderDefault(_ context.Context, input SetProviderDefaultInput) (*ProviderDefault, error) {
	normalized, next, err := NormalizeSetProviderDefaultInput(input, r.current)
	if err != nil {
		return nil, err
	}
	r.state.defaultMutations++
	r.current = &ProviderDefault{
		Scope:      normalized.Scope,
		ProviderID: normalized.ProviderID,
		Version:    next,
		UpdatedBy:  normalized.ActorUserID,
	}
	result := *r.current
	return &result, nil
}

type transactionAuditRepository struct {
	state *transactionCallState
}

func (r *transactionAuditRepository) AppendProviderAuditEvent(_ context.Context, input AppendProviderAuditEventInput) (*ProviderAuditEvent, error) {
	event, err := NewProviderAuditEvent(input)
	if err != nil {
		return nil, err
	}
	r.state.auditAppends++
	event.ID = 1
	event.CreatedAt = time.Now().UTC()
	return event, nil
}

func (r *transactionAuditRepository) ListProviderAuditEvents(_ context.Context, _ ProviderAuditListRequest) ([]*ProviderAuditEvent, int64, error) {
	return nil, 0, nil
}

type transactionUnitOfWork struct {
	repositories TransactionRepositories
	callbacks    int
}

func (u *transactionUnitOfWork) WithinTransaction(
	ctx context.Context,
	callback func(context.Context, TransactionRepositories) error,
) error {
	u.callbacks++
	return callback(ctx, u.repositories)
}
