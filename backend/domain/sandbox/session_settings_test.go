// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
)

const defaultSessionRuntimeSettingsJSON = `{"core_enabled":false,"interactive_enabled":false,"host_shell_enabled":false,"core_weight":1,"heavy_weight":2,"per_user_active_limit":1,"idle_session_limit":20,"idle_shell_limit":4,"session_idle_ttl_seconds":1200,"shell_idle_ttl_seconds":300,"command_timeout_seconds":600,"cancel_grace_seconds":5,"workspace_quota_mb":2048}`

func TestSessionSettingsDefaultSnapshotMatchesPlan(t *testing.T) {
	settings := DefaultSessionRuntimeSettings()
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal default session settings: %v", err)
	}
	if string(encoded) != defaultSessionRuntimeSettingsJSON {
		t.Fatalf("default session settings JSON = %s", encoded)
	}
	decoded, err := DecodeSessionRuntimeSettingsJSON(encoded)
	if err != nil {
		t.Fatalf("DecodeSessionRuntimeSettingsJSON(default) error = %v", err)
	}
	if decoded != settings {
		t.Fatalf("decoded default = %#v, want %#v", decoded, settings)
	}
}

func TestSessionSettingsAuditAcceptsOnlySafeProjection(t *testing.T) {
	input := AppendSchedulerAuditEventInput{
		ActorUserID: 7,
		Action:      SchedulerAuditActionUpdate,
		Metadata: map[string]string{
			SchedulerAuditMetadataPreviousVersion: "1",
			SchedulerAuditMetadataNewVersion:      "2",
			SchedulerAuditMetadataChangedFields:   "host_shell_enabled,core_enabled",
			SessionSettingsAuditMetadataDomain:    SessionSettingsAuditDomain,
		},
	}
	normalized, err := NormalizeAppendSessionSettingsAuditEventInput(input)
	if err != nil {
		t.Fatalf("NormalizeAppendSessionSettingsAuditEventInput() error = %v", err)
	}
	if got := normalized.Metadata[SchedulerAuditMetadataChangedFields]; got != "core_enabled,host_shell_enabled" {
		t.Fatalf("changed fields = %q", got)
	}
	invalid := input
	invalid.Metadata = map[string]string{
		SchedulerAuditMetadataPreviousVersion: strconv.FormatUint(1, 10),
		SchedulerAuditMetadataNewVersion:      strconv.FormatUint(2, 10),
		SchedulerAuditMetadataChangedFields:   "settings",
		SessionSettingsAuditMetadataDomain:    SessionSettingsAuditDomain,
	}
	if _, err := NormalizeAppendSessionSettingsAuditEventInput(invalid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unsafe session audit error = %v, want ErrInvalidInput", err)
	}
	if _, err := NormalizeAppendSchedulerAuditEventInput(input); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("legacy scheduler audit accepted session metadata: %v", err)
	}
}

func TestSessionSettingsDecodeRejectsPartialUnknownDuplicateAndWrongBoolean(t *testing.T) {
	for name, raw := range map[string]string{
		"partial":       strings.Replace(defaultSessionRuntimeSettingsJSON, `,"workspace_quota_mb":2048`, "", 1),
		"unknown":       strings.Replace(defaultSessionRuntimeSettingsJSON, "{", `{"unexpected":1,`, 1),
		"duplicate":     strings.Replace(defaultSessionRuntimeSettingsJSON, `"core_weight":1`, `"core_weight":1,"core_weight":1`, 1),
		"wrong boolean": strings.Replace(defaultSessionRuntimeSettingsJSON, `"host_shell_enabled":false`, `"host_shell_enabled":"false"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeSessionRuntimeSettingsJSON([]byte(raw)); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("DecodeSessionRuntimeSettingsJSON() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSessionSettingsValidationRejectsUnsafeSnapshots(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SessionRuntimeSettings)
	}{
		{name: "interactive phase one", mutate: func(value *SessionRuntimeSettings) { value.InteractiveEnabled = true }},
		{name: "core weight above phase one budget", mutate: func(value *SessionRuntimeSettings) { value.CoreWeight = 3 }},
		{name: "heavy weight above phase one budget", mutate: func(value *SessionRuntimeSettings) { value.HeavyWeight = 3 }},
		{name: "negative session ttl", mutate: func(value *SessionRuntimeSettings) { value.SessionIdleTTLSeconds = -1 }},
		{name: "shell ttl above session ttl", mutate: func(value *SessionRuntimeSettings) { value.ShellIdleTTLSeconds = value.SessionIdleTTLSeconds + 1 }},
		{name: "idle shell above idle session", mutate: func(value *SessionRuntimeSettings) { value.IdleShellLimit = value.IdleSessionLimit + 1 }},
		{name: "missing workspace quota", mutate: func(value *SessionRuntimeSettings) { value.WorkspaceQuotaMB = 0 }},
		{name: "workspace quota above limit", mutate: func(value *SessionRuntimeSettings) { value.WorkspaceQuotaMB = 1048577 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := DefaultSessionRuntimeSettings()
			test.mutate(&candidate)
			if _, err := NormalizeSessionRuntimeSettings(candidate); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeSessionRuntimeSettings() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSessionSettingsUpdateAcceptsValidatedRuntimeChange(t *testing.T) {
	current := DefaultSessionRuntimeSettings()
	current.Version = InitialVersion
	next := current
	next.CommandTimeoutSeconds--
	got, err := NormalizeUpdateSessionSettingsInput(UpdateSessionSettingsInput{
		ExpectedVersion: current.Version,
		Settings:        next,
		UpdatedBy:       7,
	}, current)
	if err != nil || got.Settings != next {
		t.Fatalf("NormalizeUpdateSessionSettingsInput() = %#v, %v", got, err)
	}
}
