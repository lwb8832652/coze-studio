// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	appinfra "github.com/coze-dev/coze-studio/backend/application/base/appinfra"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const appDevWiringTestProviderKey = "018f0d2e-7b73-7e21-9a89-1a2b3c4d5e6f"

type appDevWiringProviderRepository struct {
	provider *domainsandbox.Provider
}

func (repository appDevWiringProviderRepository) GetProviderDefault(context.Context, domainsandbox.Scope) (*domainsandbox.ProviderDefault, error) {
	if repository.provider == nil {
		return nil, domainsandbox.ErrDefaultMissing
	}
	return &domainsandbox.ProviderDefault{Scope: domainsandbox.ScopeAppDev, ProviderID: repository.provider.ID, Version: 1}, nil
}

func (repository appDevWiringProviderRepository) GetProvider(context.Context, int64) (*domainsandbox.Provider, error) {
	if repository.provider == nil {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *repository.provider
	return &copy, nil
}

type appDevReadinessCache struct {
	cache.Cmdable
	ready func(context.Context) error
}

func (cache *appDevReadinessCache) CheckReadiness(ctx context.Context) error {
	if cache == nil || cache.Cmdable == nil {
		return errors.New("cache unavailable")
	}
	if cache.ready != nil {
		return cache.ready(ctx)
	}
	return nil
}

type appDevWiringStorage struct {
	ready func(context.Context) error
}

func (storageFake *appDevWiringStorage) CheckReadiness(ctx context.Context) error {
	if storageFake == nil {
		return errors.New("storage unavailable")
	}
	if storageFake.ready != nil {
		return storageFake.ready(ctx)
	}
	return nil
}

func (*appDevWiringStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error {
	return nil
}

func (*appDevWiringStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}

func (*appDevWiringStorage) GetObject(context.Context, string) ([]byte, error) {
	return nil, storage.ErrObjectNotFound
}

func (*appDevWiringStorage) DeleteObject(context.Context, string) error {
	return nil
}

func (*appDevWiringStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "", storage.ErrObjectNotFound
}

func (*appDevWiringStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, storage.ErrObjectNotFound
}

func (*appDevWiringStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, nil
}

func (*appDevWiringStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return &storage.ListObjectsPaginatedOutput{}, nil
}

func (*appDevWiringStorage) OpenObjectStream(context.Context, string) (io.ReadCloser, error) {
	return nil, storage.ErrObjectNotFound
}

func TestAppDevProviderWiringConfigurationMatrix(t *testing.T) {
	validEnv, validProvider := appDevWiringValidConfiguration(t)

	tests := []struct {
		name   string
		mutate func(map[string]string, *appDevProviderWiringDependencies, *domainsandbox.Provider)
		ready  bool
	}{
		{name: "disabled", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			env[sandboxControlPlaneEnabledEnv] = "false"
		}},
		{name: "runtime routing disabled", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			env[sandboxRuntimeRoutingEnabledEnv] = "false"
		}},
		{name: "production remote valid", ready: true},
		{name: "missing checkpoint key", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			delete(env, infrasandbox.SandboxCredentialKeysJSONEnv)
		}},
		{name: "missing redis", mutate: func(_ map[string]string, dependencies *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			dependencies.Infra.CacheCli = nil
		}},
		{name: "missing streaming storage", mutate: func(_ map[string]string, dependencies *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			dependencies.Infra.OSS = struct{ storage.Storage }{}
		}},
		{name: "missing preview gateway", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			delete(env, appDevPreviewGatewayEnv)
		}},
		{name: "missing artifact gateway", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			delete(env, appDevArtifactGatewayBaseURLEnv)
		}},
		{name: "http remote endpoint", mutate: func(_ map[string]string, dependencies *appDevProviderWiringDependencies, provider *domainsandbox.Provider) {
			provider.EndpointSecret = appDevWiringEncrypt(t, appDevWiringTestKeyRing(t), provider.ProviderKey, infrasandbox.CredentialFieldEndpoint, "http://runner.example.test")
		}},
		{name: "missing remote token", mutate: func(_ map[string]string, _ *appDevProviderWiringDependencies, provider *domainsandbox.Provider) {
			provider.CredentialSecret = ""
		}},
		{name: "bad proxy verifier", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, _ *domainsandbox.Provider) {
			env[appDevArtifactTrustedProxyCIDRsEnv] = "not-a-cidr"
		}},
		{name: "local provider forbidden in production", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, provider *domainsandbox.Provider) {
			env[sandboxHostRuntimeEnabledEnv] = "true"
			provider.Type = domainsandbox.ProviderTypeLocalDebug
			provider.EndpointSecret = ""
			provider.CredentialSecret = ""
		}},
		{name: "debug host valid", ready: true, mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, provider *domainsandbox.Provider) {
			env[sandboxAppEnv] = "debug"
			env[sandboxHostRuntimeEnabledEnv] = "true"
			env[appDevProviderAuthenticationTokenEnv] = "debug-provider-credential"
			env[appDevPreviewGatewayEnv] = "http://127.0.0.1:8080"
			env[appDevArtifactGatewayBaseURLEnv] = "http://127.0.0.1:8080"
			provider.Type = domainsandbox.ProviderTypeLocalDebug
			provider.EndpointSecret = ""
			provider.CredentialSecret = ""
		}},
		{name: "debug host invalid without both gates", mutate: func(env map[string]string, _ *appDevProviderWiringDependencies, provider *domainsandbox.Provider) {
			env[sandboxAppEnv] = "debug"
			env[sandboxHostRuntimeEnabledEnv] = "false"
			provider.Type = domainsandbox.ProviderTypeLocalDebug
			provider.EndpointSecret = ""
			provider.CredentialSecret = ""
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := cloneAppDevWiringEnv(validEnv)
			provider := *validProvider
			dependencies := appDevWiringDependencies(t, &provider)
			dependencies.Providers = appDevWiringProviderRepository{provider: &provider}
			if test.mutate != nil {
				test.mutate(env, &dependencies, &provider)
				dependencies.Providers = appDevWiringProviderRepository{provider: &provider}
			}
			runtime, err := newAppDevProviderRuntime(context.Background(), dependencies, appDevWiringGetenv(env))
			if test.ready {
				require.NoError(t, err)
				require.NotNil(t, runtime)
				require.NotNil(t, runtime.facade)
				require.NotNil(t, runtime.gatewayHandler)
				require.NotNil(t, runtime.scheduler)
				return
			}
			require.Nil(t, runtime)
			if env[sandboxControlPlaneEnabledEnv] == "false" ||
				env[sandboxRuntimeRoutingEnabledEnv] == "false" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, errAppDevProviderWiringUnavailable)
				require.NotContains(t, err.Error(), "debug-provider-credential")
				require.NotContains(t, err.Error(), "runner-credential")
				require.NotContains(t, err.Error(), "runner.example.test")
			}
		})
	}
}

func TestAppDevProductionWiringHasNoLegacyOrInMemoryFallback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "appdev_wiring.go"))
	require.NoError(t, err)
	text := string(source)
	for _, forbidden := range []string{
		"SandboxRuntimeManager",
		"NewConfiguredRuntimeManager",
		"NewLocalStore",
		"InMemory",
		"memoryArtifactGrant",
	} {
		require.NotContains(t, text, forbidden)
	}
	applicationSource, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "application.go"))
	require.NoError(t, err)
	require.NotContains(t, string(applicationSource), "NewConfiguredRuntimeManager")
	require.Contains(t, string(applicationSource), "NewPersistentStore")
}

func TestAppDevProviderRuntimeInitializationIsSingleOwner(t *testing.T) {
	validEnv, provider := appDevWiringValidConfiguration(t)
	for key, value := range validEnv {
		t.Setenv(key, value)
	}
	require.Nil(t, appdevapp.CurrentProviderHTTPDependencies())

	firstProvider := *provider
	secondProvider := *provider
	dependencies := []appDevProviderWiringDependencies{
		appDevWiringDependencies(t, &firstProvider),
		appDevWiringDependencies(t, &secondProvider),
	}
	dependencies[0].Providers = appDevWiringProviderRepository{provider: &firstProvider}
	dependencies[1].Providers = appDevWiringProviderRepository{provider: &secondProvider}

	start := make(chan struct{})
	results := make(chan struct {
		runtime *appDevProviderRuntime
		err     error
	}, len(dependencies))
	var wait sync.WaitGroup
	for index := range dependencies {
		wait.Add(1)
		go func(input appDevProviderWiringDependencies) {
			defer wait.Done()
			<-start
			runtime, err := initAppDevProviderRuntime(context.Background(), input)
			results <- struct {
				runtime *appDevProviderRuntime
				err     error
			}{runtime: runtime, err: err}
		}(dependencies[index])
	}
	close(start)
	wait.Wait()
	close(results)

	var owner *appDevProviderRuntime
	successes := 0
	var failures []error
	unexpectedRuntimeOnFailure := false
	for result := range results {
		if result.err == nil {
			successes++
			if owner == nil {
				owner = result.runtime
			}
			continue
		}
		failures = append(failures, result.err)
		unexpectedRuntimeOnFailure = unexpectedRuntimeOnFailure || result.runtime != nil
	}
	t.Cleanup(func() {
		if owner == nil {
			return
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultApplicationShutdownAttemptTimeout)
		defer cancel()
		_ = owner.Shutdown(shutdownCtx)
		applicationShutdowns.Unregister(owner.shutdownOwner)
	})
	require.NotNil(t, owner)
	require.Equal(t, 1, successes)
	require.Len(t, failures, 1)
	require.ErrorIs(t, failures[0], errAppDevProviderWiringUnavailable)
	require.False(t, unexpectedRuntimeOnFailure)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultApplicationShutdownAttemptTimeout)
	defer cancel()
	require.NoError(t, owner.Shutdown(shutdownCtx))
	require.True(t, applicationShutdowns.Unregister(owner.shutdownOwner))
	owner = nil
}

func appDevWiringDependencies(t *testing.T, provider *domainsandbox.Provider) appDevProviderWiringDependencies {
	t.Helper()
	objects := &appDevWiringStorage{}
	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)
	redisClient := redisimpl.NewWithAddrAndPassword(server.Addr(), "")
	database, err := gorm.Open(sqlite.Open("file:appdev-wiring?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	store := infraappdev.NewPersistentStoreForTest(database, objects, t.TempDir())
	return appDevProviderWiringDependencies{
		Infra:     &appinfra.AppDependencies{DB: database, CacheCli: redisClient, OSS: objects},
		Service:   appdevapp.NewService(store),
		Store:     store,
		Providers: appDevWiringProviderRepository{provider: provider},
		Router:    &appsandbox.ProviderRouter{},
	}
}

func TestAppDevProviderWiringReadinessFailsClosed(t *testing.T) {
	validEnv, provider := appDevWiringValidConfiguration(t)
	tests := []struct {
		name   string
		mutate func(*appDevProviderWiringDependencies)
	}{
		{
			name: "redis unavailable",
			mutate: func(dependencies *appDevProviderWiringDependencies) {
				dependencies.Infra.CacheCli = &appDevReadinessCache{
					Cmdable: dependencies.Infra.CacheCli,
					ready: func(context.Context) error {
						return errors.New("redis://secret@cache.internal")
					},
				}
			},
		},
		{
			name: "storage unavailable",
			mutate: func(dependencies *appDevProviderWiringDependencies) {
				dependencies.Infra.OSS = &appDevWiringStorage{
					ready: func(context.Context) error {
						return errors.New("s3://secret-bucket/internal/object")
					},
				}
			},
		},
		{
			name: "typed nil cache",
			mutate: func(dependencies *appDevProviderWiringDependencies) {
				var typedNil *appDevReadinessCache
				dependencies.Infra.CacheCli = typedNil
			},
		},
		{
			name: "typed nil storage",
			mutate: func(dependencies *appDevProviderWiringDependencies) {
				var typedNil *appDevWiringStorage
				dependencies.Infra.OSS = typedNil
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCopy := *provider
			dependencies := appDevWiringDependencies(t, &providerCopy)
			dependencies.Providers = appDevWiringProviderRepository{provider: &providerCopy}
			test.mutate(&dependencies)
			runtime, err := newAppDevProviderRuntime(
				context.Background(),
				dependencies,
				appDevWiringGetenv(validEnv),
			)
			require.Nil(t, runtime)
			require.ErrorIs(t, err, errAppDevProviderWiringUnavailable)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "cache.internal")
			require.NotContains(t, err.Error(), "internal/object")
		})
	}
}

func TestAppDevProviderWiringReadinessHonorsCallerDeadline(t *testing.T) {
	validEnv, provider := appDevWiringValidConfiguration(t)
	dependencies := appDevWiringDependencies(t, provider)
	dependencies.Providers = appDevWiringProviderRepository{provider: provider}
	dependencies.Infra.OSS = &appDevWiringStorage{ready: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	runtime, err := newAppDevProviderRuntime(ctx, dependencies, appDevWiringGetenv(validEnv))
	require.Nil(t, runtime)
	require.ErrorIs(t, err, errAppDevProviderWiringUnavailable)
}

func appDevWiringValidConfiguration(t *testing.T) (map[string]string, *domainsandbox.Provider) {
	t.Helper()
	keys := `{"primary":"` + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, 32)) + `"}`
	keyRing := appDevWiringTestKeyRing(t)
	provider := &domainsandbox.Provider{
		ID: 7, ProviderKey: appDevWiringTestProviderKey, Type: domainsandbox.ProviderTypeRemoteHTTP,
		Status: domainsandbox.ProviderStatusEnabled, Scopes: []domainsandbox.Scope{domainsandbox.ScopeAppDev},
		Policy: appDevWiringRuntimePolicy(),
	}
	provider.EndpointSecret = appDevWiringEncrypt(t, keyRing, provider.ProviderKey, infrasandbox.CredentialFieldEndpoint, "https://runner.example.test")
	provider.CredentialSecret = appDevWiringEncrypt(t, keyRing, provider.ProviderKey, infrasandbox.CredentialFieldCredential, "runner-credential")
	return map[string]string{
		sandboxControlPlaneEnabledEnv:                "true",
		sandboxRuntimeRoutingEnabledEnv:              "true",
		sandboxAppEnv:                                "production",
		sandboxHostRuntimeEnabledEnv:                 "false",
		appDevPreviewGatewayEnv:                      "https://preview.example.test",
		appDevArtifactGatewayBaseURLEnv:              "https://gateway.example.test",
		infrasandbox.SandboxCredentialKeysJSONEnv:    keys,
		infrasandbox.SandboxCredentialActiveKeyIDEnv: "primary",
	}, provider
}

func appDevWiringTestKeyRing(t *testing.T) *infrasandbox.SandboxKeyRing {
	t.Helper()
	keyRing, err := infrasandbox.ParseSandboxKeyRing(
		`{"primary":"`+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, 32))+`"}`,
		"primary",
	)
	require.NoError(t, err)
	return keyRing
}

func appDevWiringEncrypt(t *testing.T, keyRing *infrasandbox.SandboxKeyRing, providerKey string, field infrasandbox.CredentialField, value string) string {
	t.Helper()
	codec, err := infrasandbox.NewCredentialCodec(keyRing)
	require.NoError(t, err)
	encrypted, err := codec.Encrypt(providerKey, field, []byte(value))
	require.NoError(t, err)
	return encrypted
}

func appDevWiringRuntimePolicy() domainsandbox.RuntimePolicy {
	return domainsandbox.RuntimePolicy{
		TimeoutSeconds: 60, MemoryLimitMB: 512, CPULimit: 1, MaxOutputBytes: 1 << 20,
		MaxConcurrency: 1, NodeModulesMode: domainsandbox.NodeModulesModeDisabled,
	}
}

func appDevWiringGetenv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func cloneAppDevWiringEnv(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

var _ cache.ScriptCmdable = (*appDevReadinessCache)(nil)
var _ storage.Storage = (*appDevWiringStorage)(nil)
var _ storage.StreamingStorage = (*appDevWiringStorage)(nil)
var _ = errors.Is
var _ = strings.Contains
