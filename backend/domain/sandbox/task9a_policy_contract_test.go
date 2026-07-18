// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTask9ARuntimePolicyNormalizesCompleteSafeMetadata(t *testing.T) {
	input := task9AValidRuntimePolicy()
	input.AllowedEnvNames = []string{" PATH ", "HOME", "PATH"}
	input.VirtualReadPrefixes = []string{"workspace/src/", "inputs", "workspace/src"}
	input.AllowedExecutables = []string{"python3", "node", "python3"}

	normalized, err := NormalizeRuntimePolicy(input)
	if err != nil {
		t.Fatalf("NormalizeRuntimePolicy() error = %v", err)
	}
	if !reflect.DeepEqual(normalized.AllowedEnvNames, []string{"HOME", "PATH"}) ||
		!reflect.DeepEqual(normalized.VirtualReadPrefixes, []string{"inputs", "workspace/src"}) ||
		!reflect.DeepEqual(normalized.AllowedExecutables, []string{"node", "python3"}) {
		t.Fatalf("normalized policy = %#v", normalized)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}
	text := string(encoded)
	for _, key := range []string{"allowed_env_names", "virtual_read_prefixes", "virtual_write_prefixes", "allowed_executables", "ffi_enabled", "node_modules_mode", "node_modules_directory_ref"} {
		if !strings.Contains(text, `"`+key+`"`) {
			t.Fatalf("policy JSON missing %s: %s", key, text)
		}
	}
	for _, prohibited := range []string{"AllowEnv", "AllowRead", "AllowWrite", "AllowRun", "AllowFFI", "NodeModulesDir"} {
		if strings.Contains(text, prohibited) {
			t.Fatalf("legacy field leaked through policy JSON: %s", text)
		}
	}
}

func TestTask9ARuntimePolicyRejectsValuesHostPathsAndBounds(t *testing.T) {
	tooMany := make([]string, MaxPolicyListEntries+1)
	for index := range tooMany {
		tooMany[index] = "ENV_" + strings.Repeat("A", index%4) + string(rune('A'+index%26))
	}
	tests := []struct {
		name   string
		mutate func(*RuntimePolicy)
	}{
		{"environment value", func(p *RuntimePolicy) { p.AllowedEnvNames = []string{"TOKEN=secret"} }},
		{"too many environment names", func(p *RuntimePolicy) { p.AllowedEnvNames = tooMany }},
		{"absolute read path", func(p *RuntimePolicy) { p.VirtualReadPrefixes = []string{"/etc"} }},
		{"parent read path", func(p *RuntimePolicy) { p.VirtualReadPrefixes = []string{"workspace/../etc"} }},
		{"host write path", func(p *RuntimePolicy) { p.VirtualWritePrefixes = []string{"C:\\private"} }},
		{"command string", func(p *RuntimePolicy) { p.AllowedExecutables = []string{"bash -c"} }},
		{"executable path", func(p *RuntimePolicy) { p.AllowedExecutables = []string{"/bin/bash"} }},
		{"unknown node mode", func(p *RuntimePolicy) { p.NodeModulesMode = NodeModulesMode("host") }},
		{"approved mode missing reference", func(p *RuntimePolicy) {
			p.NodeModulesMode = NodeModulesModeApprovedDirectory
			p.NodeModulesDirectoryRef = ""
		}},
		{"directory host path", func(p *RuntimePolicy) {
			p.NodeModulesMode = NodeModulesModeApprovedDirectory
			p.NodeModulesDirectoryRef = "/opt/node_modules"
		}},
		{"disabled mode with reference", func(p *RuntimePolicy) {
			p.NodeModulesMode = NodeModulesModeDisabled
			p.NodeModulesDirectoryRef = "modules-v1"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := task9AValidRuntimePolicy()
			test.mutate(&policy)
			if _, err := NormalizeRuntimePolicy(policy); err == nil {
				t.Fatalf("NormalizeRuntimePolicy(%#v) unexpectedly succeeded", policy)
			}
		})
	}
}

func task9AValidRuntimePolicy() RuntimePolicy {
	return RuntimePolicy{
		TimeoutSeconds: 60, MemoryLimitMB: 512, CPULimit: 1, MaxOutputBytes: 65536, MaxConcurrency: 8,
		AllowedEnvNames: []string{"PATH"}, VirtualReadPrefixes: []string{"workspace/src", "inputs"},
		VirtualWritePrefixes: []string{"outputs"}, AllowedExecutables: []string{"node", "python3"},
		FFIEnabled: true, NodeModulesMode: NodeModulesModeApprovedDirectory, NodeModulesDirectoryRef: "node-modules-v1",
	}
}
