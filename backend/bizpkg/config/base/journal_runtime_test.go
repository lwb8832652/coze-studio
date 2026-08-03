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

package base

import (
	"context"
	"errors"
	"testing"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

type journalReadinessStub struct {
	err error
}

func (s journalReadinessStub) CheckReadiness(context.Context) error {
	return s.err
}

func TestJournalRuntimeConfigurationDefaultsOffAndUsesOperationalLimits(t *testing.T) {
	got := getBasicConfigurationFromOldConfig().JournalRuntimeConfiguration
	if got == nil {
		t.Fatal("journal runtime configuration is nil")
	}
	if got.JournalProjection || got.JournalUI || got.JournalSnapshots || got.CheckpointRecovery {
		t.Fatalf("journal gates must default off: %#v", got)
	}
	if got.SseTenantConnectionCap <= 0 || got.SseClusterConnectionCap < got.SseTenantConnectionCap {
		t.Fatalf("invalid default SSE caps: %#v", got)
	}
	if got.SseSendQueueHighWatermark <= 0 || got.SseSendQueueMax < got.SseSendQueueHighWatermark {
		t.Fatalf("invalid default queue limits: %#v", got)
	}
	if got.ShortRequestQPS <= 0 || got.ShortRequestBurst < got.ShortRequestQPS {
		t.Fatalf("invalid default short request limits: %#v", got)
	}
	if got.LeaseTTLSeconds <= 0 || got.SnapshotFragmentThresholdBytes <= 0 {
		t.Fatalf("invalid default lease or fragment limits: %#v", got)
	}
}

func TestSaveBaseConfigValidatesJournalRuntimeConfiguration(t *testing.T) {
	valid := defaultJournalRuntimeConfiguration()
	tests := []struct {
		name   string
		mutate func(*adminconfig.JournalRuntimeConfiguration)
	}{
		{
			name: "rollout below zero",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.JournalProjectionRolloutBasisPoints = -1
			},
		},
		{
			name: "rollout above ten thousand",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.JournalUIRolloutBasisPoints = 10001
			},
		},
		{
			name: "tenant cap exceeds cluster cap",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.SseTenantConnectionCap = value.SseClusterConnectionCap + 1
			},
		},
		{
			name: "queue watermark exceeds maximum",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.SseSendQueueHighWatermark = value.SseSendQueueMax + 1
			},
		},
		{
			name: "burst below qps",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.ShortRequestBurst = value.ShortRequestQPS - 1
			},
		},
		{
			name: "lease ttl below hard minimum",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.LeaseTTLSeconds = 1
			},
		},
		{
			name: "fragment threshold above hard maximum",
			mutate: func(value *adminconfig.JournalRuntimeConfiguration) {
				value.SnapshotFragmentThresholdBytes = 1 << 30
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *valid
			test.mutate(&candidate)
			store := &fakeBasicConfigurationStore{
				value:    getBasicConfigurationFromOldConfig(),
				revision: "rev-1",
			}
			service := &BaseConfig{base: store}
			_, err := service.SaveBaseConfig(
				context.Background(),
				BasicConfigurationPatch{JournalRuntimeConfiguration: &candidate},
				"rev-1",
			)
			if err == nil {
				t.Fatal("SaveBaseConfig() error = nil, want validation error")
			}
			if store.casCalls != 0 {
				t.Fatalf("CompareAndSwap() calls = %d, want 0", store.casCalls)
			}
		})
	}
}

func TestSaveBaseConfigRequiresReadyRedisBeforeProductionJournalEnable(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	enabled := defaultJournalRuntimeConfiguration()
	enabled.JournalProjection = true
	enabled.JournalProjectionRolloutBasisPoints = 10000

	for _, test := range []struct {
		name      string
		readiness JournalDependencyReadiness
	}{
		{name: "dependency missing"},
		{name: "dependency unhealthy", readiness: journalReadinessStub{err: errors.New("redis unavailable")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeBasicConfigurationStore{
				value:    getBasicConfigurationFromOldConfig(),
				revision: "rev-1",
			}
			service := &BaseConfig{base: store, journalReadiness: test.readiness}
			_, err := service.SaveBaseConfig(
				context.Background(),
				BasicConfigurationPatch{JournalRuntimeConfiguration: enabled},
				"rev-1",
			)
			if err == nil {
				t.Fatal("SaveBaseConfig() error = nil, want fail-closed dependency error")
			}
			if store.casCalls != 0 {
				t.Fatalf("CompareAndSwap() calls = %d, want 0", store.casCalls)
			}
		})
	}
}

func TestSaveBaseConfigPersistsJournalConfigurationWithMatchingRevision(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	current := getBasicConfigurationFromOldConfig()
	store := &fakeBasicConfigurationStore{value: current, revision: "rev-1"}
	service := &BaseConfig{base: store, journalReadiness: journalReadinessStub{}}
	enabled := defaultJournalRuntimeConfiguration()
	enabled.JournalProjection = true
	enabled.JournalProjectionRolloutBasisPoints = 2500
	enabled.ConfigRevision = "rev-1"

	revision, err := service.SaveBaseConfig(
		context.Background(),
		BasicConfigurationPatch{JournalRuntimeConfiguration: enabled},
		"rev-1",
	)
	if err != nil {
		t.Fatalf("SaveBaseConfig() error = %v", err)
	}
	if revision != "rev-next" {
		t.Fatalf("revision = %q, want rev-next", revision)
	}
	if store.value.JournalRuntimeConfiguration == nil ||
		!store.value.JournalRuntimeConfiguration.JournalProjection ||
		store.value.JournalRuntimeConfiguration.JournalProjectionRolloutBasisPoints != 2500 {
		t.Fatalf("stored journal configuration = %#v", store.value.JournalRuntimeConfiguration)
	}
	if store.value.JournalRuntimeConfiguration.ConfigRevision != "" {
		t.Fatalf("stored nested revision = %q, want empty", store.value.JournalRuntimeConfiguration.ConfigRevision)
	}

	stale := *enabled
	stale.ConfigRevision = "rev-stale"
	_, err = service.SaveBaseConfig(
		context.Background(),
		BasicConfigurationPatch{JournalRuntimeConfiguration: &stale},
		"rev-next",
	)
	if !errors.Is(err, kvstore.ErrVersionConflict) {
		t.Fatalf("stale nested revision error = %v, want version conflict", err)
	}
}

func TestGetBaseConfigReturnsDetachedJournalConfigurationAndCurrentRevision(t *testing.T) {
	current := getBasicConfigurationFromOldConfig()
	store := &fakeBasicConfigurationStore{value: current, revision: "rev-7"}
	service := &BaseConfig{base: store}

	got, revision, err := service.GetBaseConfigWithRevision(context.Background())
	if err != nil {
		t.Fatalf("GetBaseConfigWithRevision() error = %v", err)
	}
	if revision != "rev-7" || got.JournalRuntimeConfiguration.ConfigRevision != "rev-7" {
		t.Fatalf("revisions = %q / %q, want rev-7", revision, got.JournalRuntimeConfiguration.ConfigRevision)
	}
	got.JournalRuntimeConfiguration.JournalProjection = true
	if store.value.JournalRuntimeConfiguration.JournalProjection {
		t.Fatal("caller mutated stored journal configuration")
	}
}
