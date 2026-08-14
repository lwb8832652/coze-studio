// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSessionServiceAppliesOnlyExactPersistedDesiredSnapshotWithoutWritingDesired(t *testing.T) {
	service, _ := newSessionServiceFixture(t)
	desired := domainsandbox.DefaultSessionRuntimeSettings()
	desired.CoreEnabled = true
	desired.Version = 2
	store := &recordingDesiredSessionSettings{settings: desired}
	runtime := &recordingAppliedSessionSettings{settings: versionedDefaultSessionSettings(1)}
	service.settings, service.applier = store, runtime

	projection, err := service.ApplySessionConfiguration(context.Background(), SessionConfigurationCommand{
		Version: 2, Settings: desired, Claims: configurationTestClaims("runner-a"),
	})
	if err != nil || projection.Version != 2 || runtime.applyCalls != 1 || store.updateCalls != 0 {
		t.Fatalf("ApplySessionConfiguration() = %#v, %v; applies=%d updates=%d", projection, err, runtime.applyCalls, store.updateCalls)
	}

	mismatch := desired
	mismatch.WorkspaceQuotaMB++
	if _, err := service.ApplySessionConfiguration(context.Background(), SessionConfigurationCommand{
		Version: 2, Settings: mismatch, Claims: configurationTestClaims("runner-a"),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) || runtime.applyCalls != 1 || store.updateCalls != 0 {
		t.Fatalf("mismatched desired error/applies/updates = %v/%d/%d", err, runtime.applyCalls, store.updateCalls)
	}
	stale := desired
	stale.Version = 1
	if _, err := service.ApplySessionConfiguration(context.Background(), SessionConfigurationCommand{
		Version: 1, Settings: stale, Claims: configurationTestClaims("runner-a"),
	}); !errors.Is(err, domainsandbox.ErrVersionConflict) || runtime.applyCalls != 1 || store.updateCalls != 0 {
		t.Fatalf("stale desired error/applies/updates = %v/%d/%d", err, runtime.applyCalls, store.updateCalls)
	}
}

func TestSessionServiceConfigurationProjectsAppliedMemoryVersionNotNewerDesired(t *testing.T) {
	service, _ := newSessionServiceFixture(t)
	desired := versionedDefaultSessionSettings(3)
	applied := versionedDefaultSessionSettings(2)
	service.settings = &recordingDesiredSessionSettings{settings: desired}
	service.applier = &recordingAppliedSessionSettings{settings: applied}
	projection, err := service.SessionConfiguration(context.Background(), configurationTestClaims("runner-a"))
	if err != nil || projection.Version != 2 || projection.Settings != applied {
		t.Fatalf("SessionConfiguration() = %#v, %v", projection, err)
	}
}

func TestSessionServiceApplyConfigurationNeverDowngradesOrReappliesCurrentVersion(t *testing.T) {
	t.Run("newer applied version rejects desired downgrade", func(t *testing.T) {
		service, _ := newSessionServiceFixture(t)
		desired := versionedDefaultSessionSettings(2)
		runtime := &recordingAppliedSessionSettings{settings: versionedDefaultSessionSettings(3)}
		service.settings = &recordingDesiredSessionSettings{settings: desired}
		service.applier = runtime

		_, err := service.ApplySessionConfiguration(context.Background(), SessionConfigurationCommand{
			Version: 2, Settings: desired, Claims: configurationTestClaims("runner-a"),
		})
		if !errors.Is(err, domainsandbox.ErrVersionConflict) || runtime.applyCalls != 0 || runtime.settings.Version != 3 {
			t.Fatalf("ApplySessionConfiguration() error/applies/version = %v/%d/%d", err, runtime.applyCalls, runtime.settings.Version)
		}
	})

	t.Run("same applied version and payload is idempotent", func(t *testing.T) {
		service, _ := newSessionServiceFixture(t)
		desired := versionedDefaultSessionSettings(2)
		runtime := &recordingAppliedSessionSettings{settings: desired}
		service.settings = &recordingDesiredSessionSettings{settings: desired}
		service.applier = runtime

		projection, err := service.ApplySessionConfiguration(context.Background(), SessionConfigurationCommand{
			Version: 2, Settings: desired, Claims: configurationTestClaims("runner-a"),
		})
		if err != nil || projection.Version != 2 || projection.Settings != desired || runtime.applyCalls != 0 {
			t.Fatalf("ApplySessionConfiguration() = %#v, %v; applies=%d", projection, err, runtime.applyCalls)
		}
	})

	t.Run("same applied version with different payload is rejected", func(t *testing.T) {
		service, _ := newSessionServiceFixture(t)
		desired := versionedDefaultSessionSettings(2)
		applied := desired
		applied.WorkspaceQuotaMB++
		runtime := &recordingAppliedSessionSettings{settings: applied}
		service.settings = &recordingDesiredSessionSettings{settings: desired}
		service.applier = runtime

		_, err := service.ApplySessionConfiguration(context.Background(), SessionConfigurationCommand{
			Version: 2, Settings: desired, Claims: configurationTestClaims("runner-a"),
		})
		if !errors.Is(err, domainsandbox.ErrVersionConflict) || runtime.applyCalls != 0 || runtime.settings != applied {
			t.Fatalf("ApplySessionConfiguration() error/applies/settings = %v/%d/%#v", err, runtime.applyCalls, runtime.settings)
		}
	})
}

func configurationTestClaims(deploymentID string) sandboxidentity.SessionRequest {
	digest := sha256.Sum256([]byte("session configuration body"))
	return sandboxidentity.SessionRequest{DeploymentID: deploymentID, RequestDigest: digest[:]}
}

func versionedDefaultSessionSettings(version uint64) domainsandbox.SessionRuntimeSettings {
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version = version
	return settings
}

type recordingDesiredSessionSettings struct {
	settings    domainsandbox.SessionRuntimeSettings
	err         error
	updateCalls int
}

func (store *recordingDesiredSessionSettings) GetSessionSettings(context.Context) (domainsandbox.SessionRuntimeSettings, error) {
	return store.settings, store.err
}

func (store *recordingDesiredSessionSettings) UpdateSessionSettingsCAS(context.Context, domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, error) {
	store.updateCalls++
	return domainsandbox.SessionRuntimeSettings{}, errors.New("runner must not write desired settings")
}

type recordingAppliedSessionSettings struct {
	settings   domainsandbox.SessionRuntimeSettings
	applyCalls int
	err        error
}

func (runtime *recordingAppliedSessionSettings) ApplySessionSettings(settings domainsandbox.SessionRuntimeSettings) error {
	runtime.applyCalls++
	if runtime.err != nil {
		return runtime.err
	}
	runtime.settings = settings
	return nil
}

func (runtime *recordingAppliedSessionSettings) AppliedSessionSettings() domainsandbox.SessionRuntimeSettings {
	return runtime.settings
}
