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
	"fmt"
	"strings"
	"testing"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	domainsystemadmin "github.com/coze-dev/coze-studio/backend/domain/systemadmin"
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

func TestSaveBaseConfigCanonicalizesAdminEmailsBeforeCAS(t *testing.T) {
	store := &fakeBasicConfigurationStore{
		value: &adminconfig.BasicConfiguration{
			AdminEmails: "existing-admin@example.test",
		},
		revision: "rev-1",
	}
	service := &BaseConfig{base: store}
	value := " ADMIN@example.test ,admin@example.test,other@example.test "

	revision, err := service.SaveBaseConfig(
		context.Background(),
		BasicConfigurationPatch{AdminEmails: &value},
		"rev-1",
	)
	if err != nil {
		t.Fatalf("SaveBaseConfig() error = %v", err)
	}
	if revision != "rev-next" || store.revision != "rev-next" {
		t.Fatalf("saved revision = %q, store revision = %q", revision, store.revision)
	}
	if got := store.value.AdminEmails; got != "admin@example.test,other@example.test" {
		t.Fatalf("canonical AdminEmails = %q", got)
	}
}

func TestSaveBaseConfigRejectsInvalidAdminEmailsWithoutChangingAuthentication(t *testing.T) {
	tooMany := make([]string, domainnotification.MaxExplicitRecipients+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("admin-%d@example.test", index)
	}
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "empty item", value: "admin@example.test,,other@example.test"},
		{name: "trailing comma", value: "admin@example.test,"},
		{name: "invalid address", value: "admin@example.test,bad address <"},
		{name: "over limit", value: strings.Join(tooMany, ",")},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeBasicConfigurationStore{
				value: &adminconfig.BasicConfiguration{
					AdminEmails: "existing-admin@example.test",
				},
				revision: "rev-1",
			}
			service := &BaseConfig{base: store}

			_, err := service.SaveBaseConfig(
				context.Background(),
				BasicConfigurationPatch{AdminEmails: &test.value},
				"rev-1",
			)
			if !errors.Is(err, domainsystemadmin.ErrInvalidEmailProjection) {
				t.Fatalf("SaveBaseConfig() error = %v", err)
			}
			if store.casCalls != 0 ||
				store.revision != "rev-1" ||
				store.value.AdminEmails != "existing-admin@example.test" {
				t.Fatalf(
					"rejected write changed storage: calls=%d revision=%q config=%#v",
					store.casCalls,
					store.revision,
					store.value,
				)
			}
			if !domainsystemadmin.ContainsEmail(
				"existing-admin@example.test",
				store.value.AdminEmails,
				domainnotification.MaxExplicitRecipients,
			) {
				t.Fatal("original persisted administrator no longer authenticates")
			}
		})
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

func TestSaveBaseConfigInitializesBootstrapAdminWhenPersistingSiteBrand(t *testing.T) {
	t.Setenv("COZE_SYSTEM_ADMIN_EMAILS", "admin@example.com")
	store := &fakeBasicConfigurationStore{revision: kvstore.MissingRevision}
	service := &BaseConfig{base: store}
	siteName := "NewX AI"

	if _, err := service.SaveBaseConfig(
		context.Background(),
		BasicConfigurationPatch{SiteName: &siteName},
		kvstore.MissingRevision,
	); err != nil {
		t.Fatalf("SaveBaseConfig() error = %v", err)
	}
	if got := store.value.AdminEmails; got != "admin@example.com" {
		t.Fatalf("AdminEmails = %q, want bootstrap administrator", got)
	}
}

func TestSaveBaseConfigPatchesSiteBrandAndPreservesUnchangedFields(t *testing.T) {
	oldName := "Old Site"
	oldDescription := "Existing description"
	oldLogoURI := "site-brand/logo/old.png"
	oldFaviconURI := "site-brand/favicon/old.png"
	store := &fakeBasicConfigurationStore{
		value: &adminconfig.BasicConfiguration{
			AdminEmails:     "admin@example.com",
			SiteName:        &oldName,
			SiteDescription: &oldDescription,
			SiteLogoURI:     &oldLogoURI,
			FaviconURI:      &oldFaviconURI,
		},
		revision: "rev-1",
	}
	service := &BaseConfig{base: store}
	newName := "NewX AI"
	removedLogoURI := ""

	if _, err := service.SaveBaseConfig(
		context.Background(),
		BasicConfigurationPatch{
			SiteName:    &newName,
			SiteLogoURI: &removedLogoURI,
		},
		"rev-1",
	); err != nil {
		t.Fatalf("SaveBaseConfig() error = %v", err)
	}
	if got := store.value.GetSiteName(); got != newName {
		t.Fatalf("SiteName = %q, want %q", got, newName)
	}
	if got := store.value.GetSiteDescription(); got != oldDescription {
		t.Fatalf("SiteDescription = %q, want preserved value %q", got, oldDescription)
	}
	if got := store.value.GetSiteLogoURI(); got != "" {
		t.Fatalf("SiteLogoURI = %q, want explicit removal", got)
	}
	if got := store.value.GetFaviconURI(); got != oldFaviconURI {
		t.Fatalf("FaviconURI = %q, want preserved value %q", got, oldFaviconURI)
	}
	if got := store.value.AdminEmails; got != "admin@example.com" {
		t.Fatalf("AdminEmails = %q, want preserved administrator", got)
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
