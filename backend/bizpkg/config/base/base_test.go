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

type fakeBasicConfigurationStore struct {
	value    *adminconfig.BasicConfiguration
	revision string
	getErr   error
	casErr   error
	casCalls int
}

func (f *fakeBasicConfigurationStore) GetVersioned(context.Context, string, string) (*adminconfig.BasicConfiguration, string, error) {
	if f.getErr != nil {
		return nil, "", f.getErr
	}
	if f.value == nil {
		return nil, "", kvstore.ErrKeyNotFound
	}
	return cloneFakeBasicConfiguration(f.value), f.revision, nil
}

func (f *fakeBasicConfigurationStore) CompareAndSwap(_ context.Context, _, _ string, expectedRevision string, value *adminconfig.BasicConfiguration) (string, error) {
	f.casCalls++
	if f.casErr != nil {
		return "", f.casErr
	}
	if expectedRevision != f.revision {
		return "", kvstore.ErrVersionConflict
	}
	f.value = cloneFakeBasicConfiguration(value)
	f.revision = "rev-next"
	return f.revision, nil
}

func TestSaveBaseConfigUsesCASPatchAndPreservesLegacyFields(t *testing.T) {
	current := &adminconfig.BasicConfiguration{
		AdminEmails:             "old@example.com",
		ServerHost:              "https://old.example.com",
		CodeRunnerType:          adminconfig.CodeRunnerType_Sandbox,
		DisableUserRegistration: false,
		SandboxConfig: &adminconfig.SandboxConfig{
			MemoryLimitMb: 256,
			AllowNet:      "api.example.com",
		},
	}
	store := &fakeBasicConfigurationStore{value: current, revision: "rev-1"}
	service := &BaseConfig{base: store}

	_, firstRevision, err := service.GetBaseConfigWithRevision(context.Background())
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	_, secondRevision, err := service.GetBaseConfigWithRevision(context.Background())
	if err != nil {
		t.Fatalf("second read: %v", err)
	}

	updatedEmail := "new@example.com"
	if _, err = service.SaveBaseConfig(context.Background(), BasicConfigurationPatch{AdminEmails: &updatedEmail}, firstRevision); err != nil {
		t.Fatalf("first save: %v", err)
	}
	updatedHost := "https://stale.example.com"
	if _, err = service.SaveBaseConfig(context.Background(), BasicConfigurationPatch{ServerHost: &updatedHost}, secondRevision); !errors.Is(err, kvstore.ErrVersionConflict) {
		t.Fatalf("stale save error = %v, want version conflict", err)
	}

	if got := store.value.AdminEmails; got != updatedEmail {
		t.Fatalf("admin emails = %q, want %q", got, updatedEmail)
	}
	if got := store.value.ServerHost; got != current.ServerHost {
		t.Fatalf("stale host overwrote current value: %q", got)
	}
	if got := store.value.CodeRunnerType; got != adminconfig.CodeRunnerType_Sandbox {
		t.Fatalf("legacy code runner type changed: %v", got)
	}
	if got := store.value.SandboxConfig; got == nil || got.MemoryLimitMb != 256 || got.AllowNet != "api.example.com" {
		t.Fatalf("legacy sandbox config changed: %#v", got)
	}
}

func TestSaveBaseConfigReadFailureDoesNotWrite(t *testing.T) {
	readErr := errors.New("kv read failed")
	store := &fakeBasicConfigurationStore{getErr: readErr}
	service := &BaseConfig{base: store}
	value := "new@example.com"

	_, err := service.SaveBaseConfig(context.Background(), BasicConfigurationPatch{AdminEmails: &value}, "rev-1")
	if !errors.Is(err, readErr) {
		t.Fatalf("SaveBaseConfig() error = %v, want %v", err, readErr)
	}
	if store.casCalls != 0 {
		t.Fatalf("CompareAndSwap() calls = %d, want 0", store.casCalls)
	}
}

func TestSaveBaseConfigInitializesFromLegacyConfigAndPreservesIt(t *testing.T) {
	t.Setenv("CODE_RUNNER_TYPE", "sandbox")
	t.Setenv("CODE_RUNNER_ALLOW_NET", "api.example.com")
	store := &fakeBasicConfigurationStore{revision: kvstore.MissingRevision}
	service := &BaseConfig{base: store}
	email := "admin@example.com"

	if _, err := service.SaveBaseConfig(context.Background(), BasicConfigurationPatch{AdminEmails: &email}, kvstore.MissingRevision); err != nil {
		t.Fatalf("SaveBaseConfig() error = %v", err)
	}
	if store.value.CodeRunnerType != adminconfig.CodeRunnerType_Sandbox {
		t.Fatalf("CodeRunnerType = %v, want sandbox", store.value.CodeRunnerType)
	}
	if store.value.SandboxConfig == nil || store.value.SandboxConfig.AllowNet != "api.example.com" {
		t.Fatalf("SandboxConfig = %#v, want legacy value", store.value.SandboxConfig)
	}
}

func TestGetLegacySandboxConfigReturnsDetachedCopy(t *testing.T) {
	store := &fakeBasicConfigurationStore{value: &adminconfig.BasicConfiguration{
		SandboxConfig: &adminconfig.SandboxConfig{MemoryLimitMb: 64},
	}, revision: "rev-1"}
	service := &BaseConfig{base: store}

	got, err := service.GetLegacySandboxConfig(context.Background())
	if err != nil {
		t.Fatalf("GetLegacySandboxConfig() error = %v", err)
	}
	got.MemoryLimitMb = 128
	if store.value.SandboxConfig.MemoryLimitMb != 64 {
		t.Fatalf("stored legacy config was mutated: %#v", store.value.SandboxConfig)
	}
}

func cloneFakeBasicConfiguration(value *adminconfig.BasicConfiguration) *adminconfig.BasicConfiguration {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.PluginConfiguration != nil {
		plugin := *value.PluginConfiguration
		cloned.PluginConfiguration = &plugin
	}
	if value.SandboxConfig != nil {
		sandbox := *value.SandboxConfig
		cloned.SandboxConfig = &sandbox
	}
	return &cloned
}
