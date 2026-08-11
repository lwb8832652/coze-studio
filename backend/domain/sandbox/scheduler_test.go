// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchedulerSettingsDefault2C4GSnapshotIsCompleteAndDetached(t *testing.T) {
	settings := DefaultSchedulerSettings()
	if settings.TotalWeight != 2 || settings.MaxOutstanding != 32 || settings.GlobalQueueDepth != 32 ||
		settings.PerSpaceQueueDepth != 8 || settings.PerUserQueueDepth != 4 || settings.HostMemoryReserveMB != 1536 ||
		settings.CancelGraceSeconds != 5 || settings.HealthFailureThreshold != 3 || settings.HealthRecoveryThreshold != 2 {
		t.Fatalf("default scheduler scalar snapshot is not the 2C4G profile")
	}
	wantWorkloads := map[Scope]SchedulerWorkload{
		ScopeAgent:    {Weight: 2, CPULimit: CPUQuotaMilli(1250), MemoryLimitMB: 1536, PIDLimit: 128, QueueTimeoutSeconds: 600, IdleTTLSeconds: 300},
		ScopeAppDev:   {Weight: 2, CPULimit: CPUQuotaMilli(1250), MemoryLimitMB: 1536, PIDLimit: 192, QueueTimeoutSeconds: 1200, IdleTTLSeconds: 600},
		ScopeMCPStdio: {Weight: 1, CPULimit: CPUQuotaMilli(400), MemoryLimitMB: 384, PIDLimit: 64, QueueTimeoutSeconds: 300, IdleTTLSeconds: 180},
		ScopePlugin:   {Weight: 1, CPULimit: CPUQuotaMilli(400), MemoryLimitMB: 384, PIDLimit: 64, QueueTimeoutSeconds: 300, IdleTTLSeconds: 0},
	}
	if !reflect.DeepEqual(settings.Workloads, wantWorkloads) {
		t.Fatal("default scheduler workloads are not the complete 2C4G profile")
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal default scheduler settings: %v", err)
	}
	if !strings.Contains(string(encoded), `"cpu_limit":1.25`) || !strings.Contains(string(encoded), `"cpu_limit":0.4`) {
		t.Fatal("scheduler CPU quotas did not preserve exact JSON decimals")
	}
	settings.Workloads[ScopeAgent] = SchedulerWorkload{}
	if second := DefaultSchedulerSettings(); second.Workloads[ScopeAgent] != wantWorkloads[ScopeAgent] {
		t.Fatal("DefaultSchedulerSettings() returned shared workload state")
	}
}

func TestSchedulerSettingsRejectsIncompleteUnknownCaseVariantAndInvalidSnapshots(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SchedulerSettings)
	}{
		{name: "partial workloads", mutate: func(value *SchedulerSettings) { delete(value.Workloads, ScopePlugin) }},
		{name: "unknown workload", mutate: func(value *SchedulerSettings) {
			value.Workloads[Scope("shell")] = SchedulerWorkload{Weight: 1, CPULimit: CPUQuotaMilli(400), MemoryLimitMB: 384, PIDLimit: 64, QueueTimeoutSeconds: 300}
		}},
		{name: "case variant workload", mutate: func(value *SchedulerSettings) {
			delete(value.Workloads, ScopeAgent)
			value.Workloads[Scope("Agent")] = SchedulerWorkload{Weight: 2, CPULimit: CPUQuotaMilli(1250), MemoryLimitMB: 1536, PIDLimit: 128, QueueTimeoutSeconds: 600, IdleTTLSeconds: 300}
		}},
		{name: "total weight below minimum", mutate: func(value *SchedulerSettings) { value.TotalWeight = 0 }},
		{name: "queue depth above maximum", mutate: func(value *SchedulerSettings) { value.GlobalQueueDepth = 4097 }},
		{name: "nonpositive CPU", mutate: func(value *SchedulerSettings) {
			workload := value.Workloads[ScopeAgent]
			workload.CPULimit = 0
			value.Workloads[ScopeAgent] = workload
		}},
		{name: "workload weight above total", mutate: func(value *SchedulerSettings) {
			workload := value.Workloads[ScopeAgent]
			workload.Weight = value.TotalWeight + 1
			value.Workloads[ScopeAgent] = workload
		}},
		{name: "insufficient host reserve", mutate: func(value *SchedulerSettings) { value.HostMemoryReserveMB = 1535 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := DefaultSchedulerSettings()
			test.mutate(&candidate)
			if _, err := NormalizeSchedulerSettings(candidate); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("NormalizeSchedulerSettings() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestSchedulerSettingsNormalizationReturnsDetachedWorkloads(t *testing.T) {
	settings := DefaultSchedulerSettings()
	normalized, err := NormalizeSchedulerSettings(settings)
	if err != nil {
		t.Fatalf("NormalizeSchedulerSettings() error = %v", err)
	}
	settings.Workloads[ScopeAgent] = SchedulerWorkload{}
	if normalized.Workloads[ScopeAgent].Weight != 2 {
		t.Fatal("NormalizeSchedulerSettings() retained caller workload map")
	}
}

func TestDecodeSchedulerSettingsJSONRejectsDuplicateAndUnknownFields(t *testing.T) {
	encoded, err := json.Marshal(DefaultSchedulerSettings())
	if err != nil {
		t.Fatalf("marshal default scheduler settings: %v", err)
	}
	duplicate := strings.Replace(string(encoded), `"total_weight":2`, `"total_weight":2,"total_weight":2`, 1)
	unknown := strings.Replace(string(encoded), `{`, `{"unexpected":1,`, 1)
	for _, raw := range []string{duplicate, unknown} {
		if _, err := DecodeSchedulerSettingsJSON([]byte(raw)); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("DecodeSchedulerSettingsJSON() error = %v, want ErrInvalidInput", err)
		}
	}
}

func TestCPUQuotaMilliRejectsOverflowJSON(t *testing.T) {
	var quota CPUQuotaMilli
	if err := json.Unmarshal([]byte(`1844674407370955162`), &quota); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("overflow CPU quota error = %v, want ErrInvalidInput", err)
	}
}

func TestSchedulerSettingsAuditAcceptsOnlySafeMetadata(t *testing.T) {
	input := AppendSchedulerAuditEventInput{
		ActorUserID: 1,
		Action:      SchedulerAuditActionUpdate,
		Metadata: map[string]string{
			SchedulerAuditMetadataPreviousVersion: "1",
			SchedulerAuditMetadataNewVersion:      "2",
			SchedulerAuditMetadataChangedFields:   "max_outstanding",
		},
	}
	if _, err := NormalizeAppendSchedulerAuditEventInput(input); err != nil {
		t.Fatalf("NormalizeAppendSchedulerAuditEventInput() error = %v", err)
	}
	for _, forbidden := range []string{"settings", "updated_by", "user_id", "space_id", "project_id", "session_id", "credential", "token", "secret", "endpoint", "error"} {
		candidate := input
		candidate.Metadata = map[string]string{
			SchedulerAuditMetadataPreviousVersion: "1",
			SchedulerAuditMetadataNewVersion:      "2",
			SchedulerAuditMetadataChangedFields:   "max_outstanding",
			forbidden:                             "blocked",
		}
		if _, err := NormalizeAppendSchedulerAuditEventInput(candidate); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("scheduler audit accepted forbidden metadata")
		}
	}
}
