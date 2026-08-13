// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

const sessionPhaseOneTotalWeight = 2

type SessionRuntimeSettings struct {
	CoreEnabled           bool `json:"core_enabled"`
	InteractiveEnabled    bool `json:"interactive_enabled"`
	HostShellEnabled      bool `json:"host_shell_enabled"`
	CoreWeight            int  `json:"core_weight"`
	HeavyWeight           int  `json:"heavy_weight"`
	PerUserActiveLimit    int  `json:"per_user_active_limit"`
	IdleSessionLimit      int  `json:"idle_session_limit"`
	IdleShellLimit        int  `json:"idle_shell_limit"`
	SessionIdleTTLSeconds int  `json:"session_idle_ttl_seconds"`
	ShellIdleTTLSeconds   int  `json:"shell_idle_ttl_seconds"`
	CommandTimeoutSeconds int  `json:"command_timeout_seconds"`
	CancelGraceSeconds    int  `json:"cancel_grace_seconds"`
	WorkspaceQuotaMB      int  `json:"workspace_quota_mb"`

	Version   uint64 `json:"-"`
	UpdatedBy int64  `json:"-"`
}

type UpdateSessionSettingsInput struct {
	ExpectedVersion uint64
	Settings        SessionRuntimeSettings
	UpdatedBy       int64
}

func DefaultSessionRuntimeSettings() SessionRuntimeSettings {
	return SessionRuntimeSettings{
		CoreEnabled: false, InteractiveEnabled: false, HostShellEnabled: false,
		CoreWeight: 1, HeavyWeight: 2, PerUserActiveLimit: 1,
		IdleSessionLimit: 20, IdleShellLimit: 4,
		SessionIdleTTLSeconds: 1200, ShellIdleTTLSeconds: 300,
		CommandTimeoutSeconds: 600, CancelGraceSeconds: 5,
		WorkspaceQuotaMB: 2048,
	}
}

func NormalizeSessionRuntimeSettings(settings SessionRuntimeSettings) (SessionRuntimeSettings, error) {
	if settings.InteractiveEnabled ||
		settings.CoreWeight < 1 || settings.CoreWeight > sessionPhaseOneTotalWeight ||
		settings.HeavyWeight < 1 || settings.HeavyWeight > sessionPhaseOneTotalWeight ||
		settings.PerUserActiveLimit < 1 || settings.PerUserActiveLimit > 4096 ||
		settings.IdleSessionLimit < 1 || settings.IdleSessionLimit > 4096 ||
		settings.IdleShellLimit < 0 || settings.IdleShellLimit > settings.IdleSessionLimit ||
		settings.SessionIdleTTLSeconds < 1 || settings.SessionIdleTTLSeconds > 86400 ||
		settings.ShellIdleTTLSeconds < 1 || settings.ShellIdleTTLSeconds > settings.SessionIdleTTLSeconds ||
		settings.CommandTimeoutSeconds < 1 || settings.CommandTimeoutSeconds > 86400 ||
		settings.CancelGraceSeconds < 1 || settings.CancelGraceSeconds > settings.CommandTimeoutSeconds ||
		settings.WorkspaceQuotaMB < 1 || settings.WorkspaceQuotaMB > 1048576 {
		return SessionRuntimeSettings{}, ErrInvalidInput
	}
	return settings, nil
}

func NormalizeUpdateSessionSettingsInput(input UpdateSessionSettingsInput, _ SessionRuntimeSettings) (UpdateSessionSettingsInput, error) {
	if input.ExpectedVersion < InitialVersion || input.UpdatedBy <= 0 {
		return UpdateSessionSettingsInput{}, ErrInvalidInput
	}
	normalized, err := NormalizeSessionRuntimeSettings(input.Settings)
	if err != nil {
		return UpdateSessionSettingsInput{}, ErrInvalidInput
	}
	input.Settings = normalized
	return input, nil
}

func DecodeSessionRuntimeSettingsJSON(data []byte) (SessionRuntimeSettings, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	root, err := decodeUniqueJSONObject(decoder)
	if err != nil || requireJSONEOF(decoder) != nil || len(root) != 13 {
		return SessionRuntimeSettings{}, ErrInvalidInput
	}
	var settings SessionRuntimeSettings
	for _, field := range []struct {
		name string
		dst  any
	}{
		{"core_enabled", &settings.CoreEnabled},
		{"interactive_enabled", &settings.InteractiveEnabled},
		{"host_shell_enabled", &settings.HostShellEnabled},
		{"core_weight", &settings.CoreWeight},
		{"heavy_weight", &settings.HeavyWeight},
		{"per_user_active_limit", &settings.PerUserActiveLimit},
		{"idle_session_limit", &settings.IdleSessionLimit},
		{"idle_shell_limit", &settings.IdleShellLimit},
		{"session_idle_ttl_seconds", &settings.SessionIdleTTLSeconds},
		{"shell_idle_ttl_seconds", &settings.ShellIdleTTLSeconds},
		{"command_timeout_seconds", &settings.CommandTimeoutSeconds},
		{"cancel_grace_seconds", &settings.CancelGraceSeconds},
		{"workspace_quota_mb", &settings.WorkspaceQuotaMB},
	} {
		raw, ok := root[field.name]
		if !ok || json.Unmarshal(raw, field.dst) != nil {
			return SessionRuntimeSettings{}, ErrInvalidInput
		}
	}
	return NormalizeSessionRuntimeSettings(settings)
}

const (
	SessionSettingsAuditMetadataDomain = "settings_domain"
	SessionSettingsAuditDomain         = "session"
)

var sessionSettingsAuditChangedFields = map[string]struct{}{
	"core_enabled": {}, "interactive_enabled": {}, "host_shell_enabled": {},
	"core_weight": {}, "heavy_weight": {}, "per_user_active_limit": {},
	"idle_session_limit": {}, "idle_shell_limit": {}, "session_idle_ttl_seconds": {},
	"shell_idle_ttl_seconds": {}, "command_timeout_seconds": {}, "cancel_grace_seconds": {},
	"workspace_quota_mb": {},
}

func NormalizeAppendSessionSettingsAuditEventInput(input AppendSchedulerAuditEventInput) (AppendSchedulerAuditEventInput, error) {
	if input.ActorUserID <= 0 ||
		(input.Action != SchedulerAuditActionUpdate && input.Action != SchedulerAuditActionUpdateFailed) ||
		strings.TrimSpace(input.RequestID) != input.RequestID || len(input.RequestID) > MaxAuditRequestIDLength || containsControl(input.RequestID) ||
		len(input.Metadata) != 4 || input.Metadata[SessionSettingsAuditMetadataDomain] != SessionSettingsAuditDomain {
		return AppendSchedulerAuditEventInput{}, ErrInvalidInput
	}
	previous, okPrevious := normalizeSchedulerAuditVersion(input.Metadata[SchedulerAuditMetadataPreviousVersion])
	next, okNext := normalizeSchedulerAuditVersion(input.Metadata[SchedulerAuditMetadataNewVersion])
	changed, okChanged := normalizeSessionSettingsAuditChangedFields(input.Metadata[SchedulerAuditMetadataChangedFields])
	if !okPrevious || !okNext || !okChanged {
		return AppendSchedulerAuditEventInput{}, ErrInvalidInput
	}
	for key := range input.Metadata {
		if key != SchedulerAuditMetadataPreviousVersion && key != SchedulerAuditMetadataNewVersion && key != SchedulerAuditMetadataChangedFields && key != SessionSettingsAuditMetadataDomain {
			return AppendSchedulerAuditEventInput{}, ErrInvalidInput
		}
	}
	return AppendSchedulerAuditEventInput{
		ActorUserID: input.ActorUserID, RequestID: input.RequestID, Action: input.Action,
		Metadata: map[string]string{
			SchedulerAuditMetadataPreviousVersion: previous,
			SchedulerAuditMetadataNewVersion:      next,
			SchedulerAuditMetadataChangedFields:   changed,
			SessionSettingsAuditMetadataDomain:    SessionSettingsAuditDomain,
		},
	}, nil
}

func normalizeSessionSettingsAuditChangedFields(value string) (string, bool) {
	if value == "" || len(value) > MaxAuditMetadataValueLength || containsControl(value) {
		return "", false
	}
	seen := make(map[string]struct{})
	for _, field := range strings.Split(value, ",") {
		if _, ok := sessionSettingsAuditChangedFields[field]; !ok {
			return "", false
		}
		seen[field] = struct{}{}
	}
	ordered := make([]string, 0, len(seen))
	for field := range seen {
		ordered = append(ordered, field)
	}
	sort.Strings(ordered)
	return strings.Join(ordered, ","), true
}

func SessionSettingsChangedFields(previous, next SessionRuntimeSettings) []string {
	fields := make([]string, 0, len(sessionSettingsAuditChangedFields))
	if previous.CoreEnabled != next.CoreEnabled {
		fields = append(fields, "core_enabled")
	}
	if previous.InteractiveEnabled != next.InteractiveEnabled {
		fields = append(fields, "interactive_enabled")
	}
	if previous.HostShellEnabled != next.HostShellEnabled {
		fields = append(fields, "host_shell_enabled")
	}
	if previous.CoreWeight != next.CoreWeight {
		fields = append(fields, "core_weight")
	}
	if previous.HeavyWeight != next.HeavyWeight {
		fields = append(fields, "heavy_weight")
	}
	if previous.PerUserActiveLimit != next.PerUserActiveLimit {
		fields = append(fields, "per_user_active_limit")
	}
	if previous.IdleSessionLimit != next.IdleSessionLimit {
		fields = append(fields, "idle_session_limit")
	}
	if previous.IdleShellLimit != next.IdleShellLimit {
		fields = append(fields, "idle_shell_limit")
	}
	if previous.SessionIdleTTLSeconds != next.SessionIdleTTLSeconds {
		fields = append(fields, "session_idle_ttl_seconds")
	}
	if previous.ShellIdleTTLSeconds != next.ShellIdleTTLSeconds {
		fields = append(fields, "shell_idle_ttl_seconds")
	}
	if previous.CommandTimeoutSeconds != next.CommandTimeoutSeconds {
		fields = append(fields, "command_timeout_seconds")
	}
	if previous.CancelGraceSeconds != next.CancelGraceSeconds {
		fields = append(fields, "cancel_grace_seconds")
	}
	if previous.WorkspaceQuotaMB != next.WorkspaceQuotaMB {
		fields = append(fields, "workspace_quota_mb")
	}
	sort.Strings(fields)
	return fields
}
