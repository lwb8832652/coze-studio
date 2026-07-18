// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"errors"
	"testing"
)

func TestNormalizeProviderConfigurationRejectsLocalDebugPluginScope(t *testing.T) {
	_, _, _, _, err := normalizeProviderConfiguration(
		"local plugin",
		ProviderTypeLocalDebug,
		"",
		[]Scope{ScopeAgent, ScopePlugin},
		validPluginScopePolicy(),
	)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("normalizeProviderConfiguration() error = %v, want %v", err, ErrInvalidInput)
	}
}

func TestNormalizeProviderConfigurationAllowsRemotePluginAndLocalAgent(t *testing.T) {
	tests := []struct {
		name         string
		providerType ProviderType
		endpointHint string
		scopes       []Scope
	}{
		{
			name:         "remote plugin",
			providerType: ProviderTypeRemoteHTTP,
			endpointHint: "https://***.test",
			scopes:       []Scope{ScopePlugin},
		},
		{
			name:         "local agent",
			providerType: ProviderTypeLocalDebug,
			scopes:       []Scope{ScopeAgent},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, scopes, _, err := normalizeProviderConfiguration(
				test.name,
				test.providerType,
				test.endpointHint,
				test.scopes,
				validPluginScopePolicy(),
			)
			if err != nil {
				t.Fatalf("normalizeProviderConfiguration() error = %v", err)
			}
			if len(scopes) != 1 || scopes[0] != test.scopes[0] {
				t.Fatalf("normalized scopes = %#v, want %#v", scopes, test.scopes)
			}
		})
	}
}

func validPluginScopePolicy() RuntimePolicy {
	return RuntimePolicy{
		TimeoutSeconds:  60,
		MemoryLimitMB:   128,
		CPULimit:        1,
		MaxOutputBytes:  64 * 1024,
		MaxConcurrency: 1,
		NodeModulesMode: NodeModulesModeDisabled,
	}
}
