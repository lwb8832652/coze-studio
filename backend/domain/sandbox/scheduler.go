// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CPUQuotaMilli stores a CPU quota as thousandths of one CPU. Keeping the
// persisted/domain representation integral avoids floating-point drift while
// its JSON representation remains the protocol number (for example 1.25).
type CPUQuotaMilli int

func (quota CPUQuotaMilli) MarshalJSON() ([]byte, error) {
	if quota < 0 {
		return nil, ErrInvalidInput
	}
	whole := int(quota) / 1000
	remainder := int(quota) % 1000
	if remainder == 0 {
		return []byte(strconv.Itoa(whole)), nil
	}
	decimal := strings.TrimRight(strconv.Itoa(remainder + 1000)[1:], "0")
	return []byte(strconv.Itoa(whole) + "." + decimal), nil
}

func (quota *CPUQuotaMilli) UnmarshalJSON(data []byte) error {
	value := string(data)
	if value == "" || strings.ContainsAny(value, "eE+-") {
		return ErrInvalidInput
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 3) {
		return ErrInvalidInput
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return ErrInvalidInput
	}
	maxQuotaMilli := int64(^uint(0) >> 1)
	if whole > maxQuotaMilli/1000 {
		return ErrInvalidInput
	}
	milli := whole * 1000
	if len(parts) == 2 {
		fraction := parts[1] + strings.Repeat("0", 3-len(parts[1]))
		parsed, err := strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return ErrInvalidInput
		}
		if parsed > maxQuotaMilli-milli {
			return ErrInvalidInput
		}
		milli += parsed
	}
	*quota = CPUQuotaMilli(milli)
	return nil
}

type SchedulerWorkload struct {
	Weight              int           `json:"weight"`
	CPULimit            CPUQuotaMilli `json:"cpu_limit"`
	MemoryLimitMB       int           `json:"memory_limit_mb"`
	PIDLimit            int           `json:"pid_limit"`
	QueueTimeoutSeconds int           `json:"queue_timeout_seconds"`
	IdleTTLSeconds      int           `json:"idle_ttl_seconds"`
}

type SchedulerSettings struct {
	TotalWeight             int                         `json:"total_weight"`
	MaxOutstanding          int                         `json:"max_outstanding"`
	GlobalQueueDepth        int                         `json:"global_queue_depth"`
	PerSpaceQueueDepth      int                         `json:"per_space_queue_depth"`
	PerUserQueueDepth       int                         `json:"per_user_queue_depth"`
	HostMemoryReserveMB     int                         `json:"host_memory_reserve_mb"`
	CancelGraceSeconds      int                         `json:"cancel_grace_seconds"`
	HealthFailureThreshold  int                         `json:"health_failure_threshold"`
	HealthRecoveryThreshold int                         `json:"health_recovery_threshold"`
	Workloads               map[Scope]SchedulerWorkload `json:"workloads"`

	Version   uint64 `json:"-"`
	UpdatedBy int64  `json:"-"`
}

type UpdateSchedulerSettingsInput struct {
	ExpectedVersion uint64
	Settings        SchedulerSettings
	UpdatedBy       int64
}

const (
	SchedulerAuditMetadataPreviousVersion = "previous_version"
	SchedulerAuditMetadataNewVersion      = "new_version"
	SchedulerAuditMetadataChangedFields   = "changed_fields"
)

var schedulerAuditChangedFields = map[string]struct{}{
	"total_weight": {}, "max_outstanding": {}, "global_queue_depth": {},
	"per_space_queue_depth": {}, "per_user_queue_depth": {}, "host_memory_reserve_mb": {},
	"cancel_grace_seconds": {}, "health_failure_threshold": {}, "health_recovery_threshold": {},
	"workloads": {},
}

var schedulerScopes = []Scope{ScopeAgent, ScopeAppDev, ScopeMCPStdio, ScopePlugin}

func DefaultSchedulerSettings() SchedulerSettings {
	return SchedulerSettings{
		TotalWeight:             2,
		MaxOutstanding:          32,
		GlobalQueueDepth:        32,
		PerSpaceQueueDepth:      8,
		PerUserQueueDepth:       4,
		HostMemoryReserveMB:     1536,
		CancelGraceSeconds:      5,
		HealthFailureThreshold:  3,
		HealthRecoveryThreshold: 2,
		Workloads: map[Scope]SchedulerWorkload{
			ScopeAgent:    {Weight: 2, CPULimit: 1250, MemoryLimitMB: 1536, PIDLimit: 128, QueueTimeoutSeconds: 600, IdleTTLSeconds: 300},
			ScopeAppDev:   {Weight: 2, CPULimit: 1250, MemoryLimitMB: 1536, PIDLimit: 192, QueueTimeoutSeconds: 1200, IdleTTLSeconds: 600},
			ScopeMCPStdio: {Weight: 1, CPULimit: 400, MemoryLimitMB: 384, PIDLimit: 64, QueueTimeoutSeconds: 300, IdleTTLSeconds: 180},
			ScopePlugin:   {Weight: 1, CPULimit: 400, MemoryLimitMB: 384, PIDLimit: 64, QueueTimeoutSeconds: 300, IdleTTLSeconds: 0},
		},
	}
}

func NormalizeSchedulerSettings(settings SchedulerSettings) (SchedulerSettings, error) {
	if settings.TotalWeight < 1 || settings.TotalWeight > 64 ||
		settings.MaxOutstanding < 1 || settings.MaxOutstanding > 4096 ||
		settings.GlobalQueueDepth < 1 || settings.GlobalQueueDepth > 4096 ||
		settings.PerSpaceQueueDepth < 1 || settings.PerSpaceQueueDepth > settings.GlobalQueueDepth ||
		settings.PerUserQueueDepth < 1 || settings.PerUserQueueDepth > settings.PerSpaceQueueDepth ||
		settings.HostMemoryReserveMB < 1536 || settings.HostMemoryReserveMB > 32768 ||
		settings.CancelGraceSeconds < 1 || settings.CancelGraceSeconds > 300 ||
		settings.HealthFailureThreshold < 1 || settings.HealthFailureThreshold > 100 ||
		settings.HealthRecoveryThreshold < 1 || settings.HealthRecoveryThreshold > 100 ||
		len(settings.Workloads) != len(schedulerScopes) {
		return SchedulerSettings{}, ErrInvalidInput
	}

	workloads := make(map[Scope]SchedulerWorkload, len(schedulerScopes))
	for _, scope := range schedulerScopes {
		workload, ok := settings.Workloads[scope]
		if !ok || workload.Weight < 1 || workload.Weight > settings.TotalWeight ||
			workload.CPULimit < 1 || workload.CPULimit > 64000 ||
			workload.MemoryLimitMB < 1 || workload.MemoryLimitMB > 131072 ||
			workload.PIDLimit < 1 || workload.PIDLimit > 65535 ||
			workload.QueueTimeoutSeconds < 1 || workload.QueueTimeoutSeconds > 86400 ||
			workload.IdleTTLSeconds < 0 || workload.IdleTTLSeconds > 86400 {
			return SchedulerSettings{}, ErrInvalidInput
		}
		workloads[scope] = workload
	}
	for scope := range settings.Workloads {
		if !isSchedulerScope(scope) {
			return SchedulerSettings{}, ErrInvalidInput
		}
	}
	normalized := settings
	normalized.Workloads = workloads
	return normalized, nil
}

func NormalizeUpdateSchedulerSettingsInput(input UpdateSchedulerSettingsInput) (UpdateSchedulerSettingsInput, error) {
	if input.ExpectedVersion < InitialVersion || input.UpdatedBy <= 0 {
		return UpdateSchedulerSettingsInput{}, ErrInvalidInput
	}
	settings, err := NormalizeSchedulerSettings(input.Settings)
	if err != nil {
		return UpdateSchedulerSettingsInput{}, err
	}
	input.Settings = settings
	return input, nil
}

func NormalizeAppendSchedulerAuditEventInput(input AppendSchedulerAuditEventInput) (AppendSchedulerAuditEventInput, error) {
	if input.ActorUserID <= 0 || (input.Action != SchedulerAuditActionUpdate && input.Action != SchedulerAuditActionUpdateFailed) ||
		strings.TrimSpace(input.RequestID) != input.RequestID || len(input.RequestID) > MaxAuditRequestIDLength || containsControl(input.RequestID) ||
		len(input.Metadata) != 3 {
		return AppendSchedulerAuditEventInput{}, ErrInvalidInput
	}
	previous, okPrevious := normalizeSchedulerAuditVersion(input.Metadata[SchedulerAuditMetadataPreviousVersion])
	next, okNext := normalizeSchedulerAuditVersion(input.Metadata[SchedulerAuditMetadataNewVersion])
	changed, okChanged := normalizeSchedulerAuditChangedFields(input.Metadata[SchedulerAuditMetadataChangedFields])
	if !okPrevious || !okNext || !okChanged {
		return AppendSchedulerAuditEventInput{}, ErrInvalidInput
	}
	for key := range input.Metadata {
		if key != SchedulerAuditMetadataPreviousVersion && key != SchedulerAuditMetadataNewVersion && key != SchedulerAuditMetadataChangedFields {
			return AppendSchedulerAuditEventInput{}, ErrInvalidInput
		}
	}
	return AppendSchedulerAuditEventInput{
		ActorUserID: input.ActorUserID, RequestID: input.RequestID, Action: input.Action,
		Metadata: map[string]string{
			SchedulerAuditMetadataPreviousVersion: previous,
			SchedulerAuditMetadataNewVersion:      next,
			SchedulerAuditMetadataChangedFields:   changed,
		},
	}, nil
}

func normalizeSchedulerAuditVersion(value string) (string, bool) {
	if strings.TrimSpace(value) != value || value == "" {
		return "", false
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return "", false
	}
	return strconv.FormatUint(parsed, 10), true
}

func normalizeSchedulerAuditChangedFields(value string) (string, bool) {
	if value == "" || len(value) > MaxAuditMetadataValueLength || containsControl(value) {
		return "", false
	}
	seen := make(map[string]struct{})
	for _, field := range strings.Split(value, ",") {
		if _, ok := schedulerAuditChangedFields[field]; !ok {
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

func SchedulerSettingsChangedFields(previous, next SchedulerSettings) []string {
	fields := make([]string, 0, len(schedulerAuditChangedFields))
	if previous.TotalWeight != next.TotalWeight {
		fields = append(fields, "total_weight")
	}
	if previous.MaxOutstanding != next.MaxOutstanding {
		fields = append(fields, "max_outstanding")
	}
	if previous.GlobalQueueDepth != next.GlobalQueueDepth {
		fields = append(fields, "global_queue_depth")
	}
	if previous.PerSpaceQueueDepth != next.PerSpaceQueueDepth {
		fields = append(fields, "per_space_queue_depth")
	}
	if previous.PerUserQueueDepth != next.PerUserQueueDepth {
		fields = append(fields, "per_user_queue_depth")
	}
	if previous.HostMemoryReserveMB != next.HostMemoryReserveMB {
		fields = append(fields, "host_memory_reserve_mb")
	}
	if previous.CancelGraceSeconds != next.CancelGraceSeconds {
		fields = append(fields, "cancel_grace_seconds")
	}
	if previous.HealthFailureThreshold != next.HealthFailureThreshold {
		fields = append(fields, "health_failure_threshold")
	}
	if previous.HealthRecoveryThreshold != next.HealthRecoveryThreshold {
		fields = append(fields, "health_recovery_threshold")
	}
	if !schedulerWorkloadsEqual(previous.Workloads, next.Workloads) {
		fields = append(fields, "workloads")
	}
	return fields
}

func schedulerWorkloadsEqual(left, right map[Scope]SchedulerWorkload) bool {
	if len(left) != len(right) {
		return false
	}
	for scope, workload := range left {
		if other, ok := right[scope]; !ok || other != workload {
			return false
		}
	}
	return true
}

func NewSchedulerAuditEvent(input AppendSchedulerAuditEventInput) (*SchedulerAuditEvent, error) {
	normalized, err := NormalizeAppendSchedulerAuditEventInput(input)
	if err != nil {
		return nil, err
	}
	return &SchedulerAuditEvent{ActorUserID: normalized.ActorUserID, RequestID: normalized.RequestID, Action: normalized.Action, Metadata: normalized.Metadata, CreatedAt: time.Time{}}, nil
}

func isSchedulerScope(scope Scope) bool {
	for _, candidate := range schedulerScopes {
		if scope == candidate {
			return true
		}
	}
	return false
}

// DecodeSchedulerSettingsJSON accepts only the complete scheduler settings
// schema. It rejects unknown and duplicate fields before domain validation.
func DecodeSchedulerSettingsJSON(data []byte) (SchedulerSettings, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	root, err := decodeUniqueJSONObject(decoder)
	if err != nil {
		return SchedulerSettings{}, ErrInvalidInput
	}
	if err := requireJSONEOF(decoder); err != nil {
		return SchedulerSettings{}, ErrInvalidInput
	}
	const fieldCount = 10
	if len(root) != fieldCount {
		return SchedulerSettings{}, ErrInvalidInput
	}
	var settings SchedulerSettings
	for _, field := range []struct {
		name string
		dst  any
	}{
		{"total_weight", &settings.TotalWeight},
		{"max_outstanding", &settings.MaxOutstanding},
		{"global_queue_depth", &settings.GlobalQueueDepth},
		{"per_space_queue_depth", &settings.PerSpaceQueueDepth},
		{"per_user_queue_depth", &settings.PerUserQueueDepth},
		{"host_memory_reserve_mb", &settings.HostMemoryReserveMB},
		{"cancel_grace_seconds", &settings.CancelGraceSeconds},
		{"health_failure_threshold", &settings.HealthFailureThreshold},
		{"health_recovery_threshold", &settings.HealthRecoveryThreshold},
	} {
		raw, ok := root[field.name]
		if !ok || json.Unmarshal(raw, field.dst) != nil {
			return SchedulerSettings{}, ErrInvalidInput
		}
	}
	workloadRaw, ok := root["workloads"]
	if !ok {
		return SchedulerSettings{}, ErrInvalidInput
	}
	workloadDecoder := json.NewDecoder(bytes.NewReader(workloadRaw))
	workloadObjects, err := decodeUniqueJSONObject(workloadDecoder)
	if err != nil || len(workloadObjects) != len(schedulerScopes) {
		return SchedulerSettings{}, ErrInvalidInput
	}
	settings.Workloads = make(map[Scope]SchedulerWorkload, len(schedulerScopes))
	for _, scope := range schedulerScopes {
		raw, ok := workloadObjects[string(scope)]
		if !ok {
			return SchedulerSettings{}, ErrInvalidInput
		}
		objectDecoder := json.NewDecoder(bytes.NewReader(raw))
		fields, err := decodeUniqueJSONObject(objectDecoder)
		if err != nil || len(fields) != 6 {
			return SchedulerSettings{}, ErrInvalidInput
		}
		for _, name := range []string{"weight", "cpu_limit", "memory_limit_mb", "pid_limit", "queue_timeout_seconds", "idle_ttl_seconds"} {
			if _, ok := fields[name]; !ok {
				return SchedulerSettings{}, ErrInvalidInput
			}
		}
		var workload SchedulerWorkload
		if json.Unmarshal(raw, &workload) != nil {
			return SchedulerSettings{}, ErrInvalidInput
		}
		settings.Workloads[scope] = workload
	}
	return NormalizeSchedulerSettings(settings)
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalidInput
	}
	return nil
}

func decodeUniqueJSONObject(decoder *json.Decoder) (map[string]json.RawMessage, error) {
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrInvalidInput
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return nil, ErrInvalidInput
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrInvalidInput
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, ErrInvalidInput
		}
		fields[name] = raw
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, ErrInvalidInput
	}
	return fields, nil
}
