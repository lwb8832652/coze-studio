// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	appinfra "github.com/coze-dev/coze-studio/backend/application/base/appinfra"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	appuser "github.com/coze-dev/coze-studio/backend/application/user"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSandboxRemoteProviderAllowedAuthorityPreservesCanonicalCustomPort(t *testing.T) {
	endpoint, err := url.Parse("https://Runner.Example.Test:8443/")
	require.NoError(t, err)

	authority, err := sandboxRemoteProviderAllowedAuthority(endpoint)

	require.NoError(t, err)
	require.Equal(t, "runner.example.test:8443", authority)
}

func TestPluginServiceComponentsUseApplicationCodeRunner(t *testing.T) {
	runner := &sandboxRunnerOnly{}
	services := &basicServices{
		infra:    &appinfra.AppDependencies{CodeRunner: runner},
		eventbus: &eventbusImpl{},
		userSVC:  &appuser.UserApplicationService{},
	}

	components := services.toPluginServiceComponents()

	require.Same(t, runner, components.CodeRunner)
}

func TestParseSandboxRemoteProviderAllowedPrivateCIDRs(t *testing.T) {
	t.Run("empty keeps public address behavior", func(t *testing.T) {
		cidrs, err := parseSandboxRemoteProviderAllowedPrivateCIDRs("")
		require.NoError(t, err)
		require.Empty(t, cidrs)
	})

	t.Run("canonical private prefixes are sorted and deduplicated", func(t *testing.T) {
		cidrs, err := parseSandboxRemoteProviderAllowedPrivateCIDRs(
			"fd12:3456::/48,10.20.0.0/16,192.168.10.0/24,172.20.0.0/16,10.20.0.0/16",
		)
		require.NoError(t, err)
		require.Equal(t, []string{
			"10.20.0.0/16",
			"172.20.0.0/16",
			"192.168.10.0/24",
			"fd12:3456::/48",
		}, cidrs)
	})

	for _, value := range []string{
		"not-a-cidr",
		"10.20.1.1/16",
		"10.0.0.0/7",
		"172.0.0.0/8",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"100.100.100.200/32",
		"0.0.0.0/0",
		"::/0",
		"fe80::/10",
		"FD00::/8",
		"10.0.0.0/8, 192.168.0.0/16",
		"10.0.0.0/8,",
	} {
		t.Run(value, func(t *testing.T) {
			cidrs, err := parseSandboxRemoteProviderAllowedPrivateCIDRs(value)
			require.Error(t, err)
			require.Nil(t, cidrs)
		})
	}

	t.Run("allows at most sixteen raw tokens", func(t *testing.T) {
		cidrs, err := parseSandboxRemoteProviderAllowedPrivateCIDRs(strings.Join(
			[]string{
				"10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24",
				"10.0.4.0/24", "10.0.5.0/24", "10.0.6.0/24", "10.0.7.0/24",
				"10.0.8.0/24", "10.0.9.0/24", "10.0.10.0/24", "10.0.11.0/24",
				"10.0.12.0/24", "10.0.13.0/24", "10.0.14.0/24", "10.0.15.0/24",
			}, ",",
		))
		require.NoError(t, err)
		require.Len(t, cidrs, 16)
	})

	t.Run("rejects seventeen raw tokens even when duplicated", func(t *testing.T) {
		cidrs, err := parseSandboxRemoteProviderAllowedPrivateCIDRs(strings.Join(
			[]string{
				"10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8",
				"10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8",
				"10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8",
				"10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8", "10.0.0.0/8",
				"10.0.0.0/8",
			}, ",",
		))
		require.Error(t, err)
		require.Nil(t, cidrs)
	})
}

func TestSandboxWiringParsesRemoteProviderPrivateCIDRsOnlyWhenControlPlaneEnabled(t *testing.T) {
	t.Setenv(sandboxRemoteProviderAllowedPrivateCIDRsEnv, "not-a-cidr")
	t.Setenv(sandboxControlPlaneEnabledEnv, "false")
	require.NoError(t, initSandboxControlPlane(&appinfra.AppDependencies{}))

	t.Setenv(sandboxControlPlaneEnabledEnv, "true")
	require.Error(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
}

func TestSandboxWiringInjectsCanonicalRemoteProviderPrivateCIDRs(t *testing.T) {
	t.Setenv(sandboxControlPlaneEnabledEnv, "true")
	t.Setenv(sandboxRemoteProviderAllowedPrivateCIDRsEnv, "fd12:3456::/48,10.20.0.0/16,10.20.0.0/16")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	require.Equal(t, []string{"10.20.0.0/16", "fd12:3456::/48"}, capture.runtimeFactory.factory.allowedPrivateCIDRs)
}

func TestConfiguredSandboxProviderFactoryBuildAllowsConfiguredPrivateCustomPort(t *testing.T) {
	codec := sandboxWiringCredentialCodec(t)
	endpointSecret, err := codec.Encrypt(
		"remote-primary", infrasandbox.CredentialFieldEndpoint, []byte("https://10.20.30.40:8443/"),
	)
	require.NoError(t, err)
	credentialSecret, err := codec.Encrypt(
		"remote-primary", infrasandbox.CredentialFieldCredential, []byte("synthetic-bearer-credential"),
	)
	require.NoError(t, err)
	factory := &configuredSandboxProviderFactory{
		codec: codec, allowedPrivateCIDRs: []string{"10.20.0.0/16"},
	}
	provider := &domainsandbox.Provider{
		ProviderKey:      "remote-primary",
		Type:             domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret:   endpointSecret,
		CredentialSecret: credentialSecret,
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30,
		},
	}

	runtime, err := factory.build(context.Background(), provider)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NoError(t, runtime.CloseContext(context.Background()))
}

func sandboxWiringCredentialCodec(t *testing.T) *infrasandbox.CredentialCodec {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	keyRing, err := infrasandbox.ParseSandboxKeyRing(`{"primary":"`+key+`"}`, "primary")
	require.NoError(t, err)
	codec, err := infrasandbox.NewCredentialCodec(keyRing)
	require.NoError(t, err)
	return codec
}

func TestSandboxWiringFailsClosedWhenEnabledDependenciesAreMissing(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")

	err := initSandboxControlPlane(&appinfra.AppDependencies{})

	require.Error(t, err)
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
}

func TestSandboxWiringRejectsInvalidEnablementAndAllowsExplicitDisable(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "invalid")
	require.Error(t, initSandboxControlPlane(&appinfra.AppDependencies{}))
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)

	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")
	t.Setenv("APP_ENV", "production")
	require.NoError(t, initSandboxControlPlane(&appinfra.AppDependencies{}))
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
}

func TestSandboxWiringAllowsManagementBeforeRuntimeRouting(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("SANDBOX_RUNTIME_ROUTING_ENABLED", "false")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	err := initSandboxControlPlane(sandboxWiringReadyDependencies(nil))

	require.NoError(t, err)
	require.NotNil(t, SandboxSVC)
	require.NotNil(t, SandboxSessionSVC)
	require.NotNil(t, capture.sessionStore)
	require.Nil(t, capture.sessionRunner)
	require.NotNil(t, SandboxRuntimeRepository)
	require.Nil(t, SandboxRouter)
	_, ok := (sandboxMCPRuntimeBindingSource{}).LoadADKMCPRuntimeSandboxBinding()
	require.False(t, ok)
}

func TestSandboxWiringInjectsLiveHostShellStatusIntoSessionManagement(t *testing.T) {
	t.Setenv(sandboxControlPlaneEnabledEnv, "true")
	t.Setenv(infrasandbox.AppEnvName, "debug")
	t.Setenv(infrasandbox.HostShellSessionEnabledEnvName, "true")
	t.Setenv(infrasandbox.HostShellGatewayAddrEnvName, "127.0.0.1:8099")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	require.NotNil(t, capture.hostShellStatus)
	require.True(t, capture.hostShellStatus.HostShellAvailable(context.Background()))

	t.Setenv(infrasandbox.HostShellSessionEnabledEnvName, "false")
	require.False(t, capture.hostShellStatus.HostShellAvailable(context.Background()))
}

func TestSandboxWiringUsesSignedRunnerOnlyWhenSigningKeyringIsConfigured(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	require.Nil(t, capture.schedulerRunner)

	t.Setenv(infrasandbox.SandboxRunnerConfigSigningKeysJSONEnv, `{"keys":{"key-1":"0123456789abcdef0123456789abcdef"}}`)
	t.Setenv(infrasandbox.SandboxRunnerActiveConfigKeyIDEnv, "key-1")
	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	require.NotNil(t, capture.schedulerRunner)
}

func TestSandboxHealthMonitorIsNotCreatedWhenProductionControlPlaneIsDisabled(
	t *testing.T,
) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")
	t.Setenv("SANDBOX_RUNTIME_ROUTING_ENABLED", "false")
	clearSandboxControlPlane()
	t.Cleanup(clearSandboxControlPlane)

	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	monitor, err := newSandboxHealthMonitor(
		SandboxSVC,
		infrasandbox.NewMySQLHealthMonitorRepository(&gorm.DB{}, nil),
	)
	require.NoError(t, err)
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
	require.Nil(t, monitor)
}

func TestSandboxHealthMonitorRunsWithControlPlaneBeforeRuntimeRouting(
	t *testing.T,
) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("SANDBOX_RUNTIME_ROUTING_ENABLED", "false")
	clearSandboxControlPlane()
	t.Cleanup(clearSandboxControlPlane)
	installSandboxWiringTestConstructors(t, &sandboxWiringCapture{})

	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	monitor, err := newSandboxHealthMonitor(
		SandboxSVC,
		infrasandbox.NewMySQLHealthMonitorRepository(&gorm.DB{}, nil),
	)
	require.NoError(t, err)
	require.NotNil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
	require.NotNil(t, monitor)
}

func TestSandboxWiringRejectsInvalidRuntimeRoutingFlag(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("SANDBOX_RUNTIME_ROUTING_ENABLED", "invalid")

	err := initSandboxControlPlane(sandboxWiringReadyDependencies(nil))

	require.Error(t, err)
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
	require.Nil(t, SandboxRuntimeRepository)
}

func TestSandboxWiringLocalDebugRequiresBothGatesAndDelegate(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")

	tests := []struct {
		name        string
		appEnv      string
		hostEnabled string
		runner      coderunner.Runner
		wantLocal   bool
		wantErr     bool
	}{
		{"neither gate", "", "", &sandboxRunnerDelegate{}, false, false},
		{"debug gate only", "debug", "false", &sandboxRunnerDelegate{}, false, false},
		{"host gate only", "production", "true", &sandboxRunnerDelegate{}, false, false},
		{"debug both gates missing runner", "debug", "true", nil, false, true},
		{"debug both gates wrong type", "debug", "true", &sandboxRunnerOnly{}, false, true},
		{"debug app env whitespace", " debug", "true", &sandboxRunnerDelegate{}, false, false},
		{"debug host flag whitespace", "debug", " true", &sandboxRunnerDelegate{}, false, false},
		{"debug both gates", "debug", "true", &sandboxRunnerDelegate{}, true, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("APP_ENV", test.appEnv)
			t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", test.hostEnabled)
			capture := &sandboxWiringCapture{}
			installSandboxWiringTestConstructors(t, capture)

			err := initSandboxControlPlane(sandboxWiringReadyDependencies(test.runner))

			if test.wantErr {
				require.Error(t, err)
				require.Nil(t, SandboxSVC)
				require.Nil(t, SandboxRouter)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, SandboxSVC)
			require.NotNil(t, SandboxRouter)
			if test.wantLocal {
				require.Same(t, test.runner, capture.localDelegate)
			} else {
				require.Nil(t, capture.localDelegate)
			}
		})
	}
}

func TestSandboxWiringProductionNeverFallsBackToLocalDebug(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	err := initSandboxControlPlane(sandboxWiringReadyDependencies(&sandboxRunnerDelegate{}))

	require.NoError(t, err)
	require.True(t, capture.assembled)
	require.Nil(t, capture.localDelegate)
	require.NotNil(t, SandboxSVC)
	require.NotNil(t, SandboxRouter)
}

func TestSandboxHealthFactoryAdvertisesHostSessionsWithoutOpeningLegacyOneShot(t *testing.T) {
	t.Setenv(infrasandbox.AppEnvName, "debug")
	t.Setenv(infrasandbox.AppDevHostRuntimeEnabledEnvName, "false")
	t.Setenv(infrasandbox.HostShellSessionEnabledEnvName, "true")
	t.Setenv(infrasandbox.HostShellGatewayAddrEnvName, "127.0.0.1:8099")
	configured := &configuredSandboxProviderFactory{codec: &infrasandbox.CredentialCodec{}}
	healthFactory := sandboxHealthProviderFactory{factory: configured}
	provider := &domainsandbox.Provider{
		Type:   domainsandbox.ProviderTypeLocalDebug,
		Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent},
	}

	health, err := healthFactory.Build(context.Background(), provider)
	require.NoError(t, err)
	result, err := health.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, domainsandbox.HealthStatusHealthy, result.Status)
	require.Equal(t, []domainsandbox.Scope{domainsandbox.ScopeAgent}, result.Capabilities)
	require.Equal(t, []domainsandbox.ProviderFeature{
		domainsandbox.ProviderFeatureSandboxSessionV1,
		domainsandbox.ProviderFeatureSignedSessionContextV2,
	}, result.Features)
	require.NoError(t, health.CloseContext(context.Background()))

	_, err = configured.build(context.Background(), provider)
	require.ErrorIs(t, err, domainsandbox.ErrUnavailable)
}

func TestSandboxSessionFactoryBuildsHostOnlyBehindExactIndependentGate(t *testing.T) {
	environment := map[string]string{
		infrasandbox.AppEnvName:                      "debug",
		infrasandbox.AppDevHostRuntimeEnabledEnvName: "true",
	}
	factory := sandboxRuntimeProviderFactory{factory: &configuredSandboxProviderFactory{
		getenv:        func(key string) string { return environment[key] },
		hostRootDir:   t.TempDir(),
		hostSkillsDir: filepath.Join(t.TempDir(), "skills-created-on-first-use"),
	}}
	provider := domainsandbox.Provider{
		ID: 41, ProviderKey: "local-host", Type: domainsandbox.ProviderTypeLocalDebug,
		Scopes: []domainsandbox.Scope{domainsandbox.ScopeAgent},
		Policy: domainsandbox.RuntimePolicy{AllowedEnvNames: []string{"LANG"}},
	}
	descriptor := appsandbox.ProviderDescriptor{
		ProviderKey: provider.ProviderKey, ProviderType: provider.Type,
		Scope: domainsandbox.ScopeAgent, Policy: provider.Policy,
	}

	_, err := factory.BuildSession(context.Background(), provider, descriptor)
	require.ErrorIs(t, err, domainsandbox.ErrExecutionForbidden)

	environment[infrasandbox.AppDevHostRuntimeEnabledEnvName] = "false"
	environment[infrasandbox.HostShellSessionEnabledEnvName] = "true"
	environment[infrasandbox.HostShellGatewayAddrEnvName] = "[::1]:8099"
	runtime, err := factory.BuildSession(context.Background(), provider, descriptor)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	session, err := runtime.Acquire(context.Background(), infrasandbox.AcquireSessionRequest{Key: domainsandbox.SessionKey{
		DeploymentID: "local-debug", ProviderID: provider.ID, SpaceID: 11, UserID: 22,
		ThreadID: "thread-host", Profile: domainsandbox.SessionProfileCore,
	}})
	require.NoError(t, err)

	closer, ok := runtime.(interface{ CloseContext(context.Context) error })
	require.True(t, ok)
	require.NoError(t, closer.CloseContext(context.Background()))
	_, err = runtime.Get(context.Background(), session.Ref())
	require.NoError(t, err, "selection cleanup must not destroy the persistent Host Session")

	environment[infrasandbox.HostShellSessionEnabledEnvName] = "false"
	_, err = runtime.Get(context.Background(), session.Ref())
	require.ErrorIs(t, err, domainsandbox.ErrUnavailable)
}

func TestSandboxWiringProductionDoesNotRequireHostRunnerOrLegacyRunnerEnvironment(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	err := initSandboxControlPlane(sandboxWiringReadyDependencies(nil))

	require.NoError(t, err)
	require.True(t, capture.assembled)
	require.Nil(t, capture.localDelegate)
}

func TestSandboxWiringAssemblesRealGraphWithSharedLimiterGuardAndFactory(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	capture := &sandboxWiringCapture{}
	installSandboxWiringTestConstructors(t, capture)

	err := initSandboxControlPlane(sandboxWiringReadyDependencies(&sandboxRunnerOnly{}))

	require.NoError(t, err)
	require.NotNil(t, SandboxSVC)
	require.NotNil(t, SandboxRouter)
	require.True(t, capture.assembled)
	require.Same(t, capture.limiter, capture.serviceLimiter)
	require.Same(t, capture.limiter, capture.routerLimiter)
	require.Same(t, capture.healthFactory.factory, capture.runtimeFactory.factory)
	require.Nil(t, capture.healthFactory.factory.localDelegate)
}

func TestSandboxWiringInjectsRunnerDeploymentAndV2SignerOnlyIntoSessionFactory(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv(sandboxRunnerDeploymentIDEnv, "runner-dev-a")
	signer := sandboxWiringSessionKeyring(t)
	capture := &sandboxWiringCapture{identitySigner: signer}
	installSandboxWiringTestConstructors(t, capture)

	require.NoError(t, initSandboxControlPlane(sandboxWiringReadyDependencies(nil)))
	require.Equal(t, "runner-dev-a", capture.runtimeFactory.factory.deploymentID)
	require.NotNil(t, capture.runtimeFactory.factory.sessionSigner)
	require.NotNil(t, SandboxSessionSVC)
	require.NotNil(t, capture.sessionRunner)

	descriptor := appsandbox.ProviderDescriptor{
		ProviderKey: "remote-primary", ProviderType: domainsandbox.ProviderTypeRemoteHTTP,
		Scope: domainsandbox.ScopeAgent,
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30, MemoryLimitMB: 128, CPULimit: 1,
			MaxOutputBytes: 4096, MaxConcurrency: 1,
		},
	}
	// Missing Session capabilities fail closed only on the optional factory;
	// the same base factory continues to validate one-shot providers.
	require.NoError(t, capture.runtimeFactory.ValidateConfig(context.Background(), descriptor))
	require.ErrorIs(t, capture.runtimeFactory.ValidateSessionConfig(context.Background(), descriptor), domainsandbox.ErrScopeUnsupported)
}

func TestSandboxSessionFactoryRequiresValidDeploymentWithoutBreakingOneShot(t *testing.T) {
	policy := domainsandbox.RuntimePolicy{
		TimeoutSeconds: 30, MemoryLimitMB: 128, CPULimit: 1,
		MaxOutputBytes: 4096, MaxConcurrency: 1,
	}
	for _, deploymentID := range []string{"", "runner/dev"} {
		factory := sandboxRuntimeProviderFactory{factory: &configuredSandboxProviderFactory{
			identitySigner: sandboxWiringSessionKeyring(t),
			sessionSigner:  sandboxWiringSessionKeyring(t),
			deploymentID:   deploymentID,
		}}
		descriptor := appsandbox.ProviderDescriptor{
			ProviderKey: "remote-primary", ProviderType: domainsandbox.ProviderTypeRemoteHTTP,
			Scope: domainsandbox.ScopeAgent, Policy: policy,
		}
		require.NoError(t, factory.ValidateConfig(context.Background(), descriptor))
		require.ErrorIs(t, factory.ValidateSessionConfig(context.Background(), descriptor), domainsandbox.ErrConfigurationInvalid)
	}
}

func TestSandboxSessionFactoryUsesValidatedDescriptorScopeForMultiScopeProvider(t *testing.T) {
	factory := sandboxRuntimeProviderFactory{factory: &configuredSandboxProviderFactory{
		sessionSigner: sandboxWiringSessionKeyring(t),
		deploymentID:  "runner-dev-a",
	}}
	provider := domainsandbox.Provider{
		ID: 41, ProviderKey: "remote-primary", Type: domainsandbox.ProviderTypeRemoteHTTP,
		Scopes: []domainsandbox.Scope{domainsandbox.ScopeAppDev, domainsandbox.ScopeAgent},
	}
	descriptor := appsandbox.ProviderDescriptor{
		ProviderKey: provider.ProviderKey, ProviderType: provider.Type,
		Scope: domainsandbox.ScopeAgent,
	}

	// It reaches the shared base builder (and fails for this deliberately empty
	// codec) instead of rejecting a valid multi-scope provider or guessing the
	// first configured scope.
	_, err := factory.BuildSession(context.Background(), provider, descriptor)
	require.ErrorIs(t, err, domainsandbox.ErrConfigurationInvalid)

	descriptor.Scope = domainsandbox.ScopePlugin
	_, err = factory.BuildSession(context.Background(), provider, descriptor)
	require.ErrorIs(t, err, domainsandbox.ErrConfigurationInvalid)
}

func sandboxWiringSessionKeyring(t *testing.T) sandboxidentity.Keyring {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, 32))
	keyring, configured, err := sandboxidentity.LoadKeyringFromEnv(func(name string) string {
		switch name {
		case sandboxidentity.ExecutionContextSigningKeysJSONEnv:
			return `{"keys":{"primary":"` + key + `"}}`
		case sandboxidentity.ExecutionContextActiveKeyIDEnv:
			return "primary"
		default:
			return ""
		}
	}, time.Minute)
	require.NoError(t, err)
	require.True(t, configured)
	return keyring
}

func TestSandboxWiringPropagatesStartupAssemblyError(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "production")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	wantErr := errors.New("assembler failed")
	capture := &sandboxWiringCapture{serviceErr: wantErr}
	installSandboxWiringTestConstructors(t, capture)

	err := initSandboxControlPlane(sandboxWiringReadyDependencies(&sandboxRunnerOnly{}))

	require.ErrorIs(t, err, wantErr)
	require.Nil(t, SandboxSVC)
	require.Nil(t, SandboxRouter)
}

func TestSandboxMCPRuntimeBindingPublishReplaceUnpublish(t *testing.T) {
	clearSandboxControlPlane()
	t.Cleanup(clearSandboxControlPlane)

	firstRepository := infrasandbox.NewMySQLRepository(&gorm.DB{})
	firstRouter := &appsandbox.ProviderRouter{}
	first, err := publishSandboxMCPRuntimeBinding(firstRepository, firstRouter)
	require.NoError(t, err)

	binding, ok := (sandboxMCPRuntimeBindingSource{}).LoadADKMCPRuntimeSandboxBinding()
	require.True(t, ok)
	require.Same(t, firstRepository, binding.ProviderDefaults)
	require.Same(t, firstRepository, binding.Providers)

	secondRepository := infrasandbox.NewMySQLRepository(&gorm.DB{})
	secondRouter := &appsandbox.ProviderRouter{}
	second, err := publishSandboxMCPRuntimeBinding(secondRepository, secondRouter)
	require.NoError(t, err)
	binding, ok = (sandboxMCPRuntimeBindingSource{}).LoadADKMCPRuntimeSandboxBinding()
	require.True(t, ok)
	require.Same(t, secondRepository, binding.ProviderDefaults)
	require.Same(t, secondRepository, binding.Providers)

	require.False(t, unpublishSandboxMCPRuntimeBinding(first))
	require.True(t, unpublishSandboxMCPRuntimeBinding(second))
	_, ok = (sandboxMCPRuntimeBindingSource{}).LoadADKMCPRuntimeSandboxBinding()
	require.False(t, ok)
}

type sandboxWiringCapture struct {
	assembled       bool
	localDelegate   infrasandbox.LocalExecutionDelegate
	limiter         *sandboxSharedLimiterStub
	serviceLimiter  appsandbox.ProviderLifecycleGuard
	routerLimiter   appsandbox.CapacityLimiter
	healthFactory   sandboxHealthProviderFactory
	runtimeFactory  sandboxRuntimeProviderFactory
	schedulerRunner appsandbox.NativeSchedulerRunner
	sessionRunner   appsandbox.NativeSessionRunner
	sessionStore    domainsandbox.SessionSettingsAuditRepository
	hostShellStatus appsandbox.HostShellRuntimeStatusSource
	serviceErr      error
	identitySigner  sandboxidentity.Signer
}

func installSandboxWiringTestConstructors(t *testing.T, capture *sandboxWiringCapture) {
	t.Helper()
	if capture == nil {
		capture = &sandboxWiringCapture{}
	}
	previous := sandboxControlPlaneConstructors
	limiter := &sandboxSharedLimiterStub{}
	capture.limiter = limiter
	sandboxControlPlaneConstructors = sandboxWiringConstructors{
		loadCodec: func(func(string) string) (*infrasandbox.CredentialCodec, error) {
			return &infrasandbox.CredentialCodec{}, nil
		},
		loadIdentitySigner: func(func(string) string) (sandboxidentity.Signer, error) {
			return capture.identitySigner, nil
		},
		loadSchedulerSigner: func(getenv func(string) string) (*infrasandbox.SchedulerConfigSigner, bool, error) {
			return infrasandbox.LoadSchedulerConfigSignerFromEnv(getenv, time.Minute)
		},
		newRepository: func(*appinfra.AppDependencies) sandboxRepository {
			return infrasandbox.NewMySQLRepository(&gorm.DB{})
		},
		newLimiter: func(*appinfra.AppDependencies) (sandboxSharedLimiter, error) {
			return limiter, nil
		},
		newService: func(options appsandbox.ServiceOptions) (*appsandbox.Service, error) {
			capture.assembled = true
			capture.serviceLimiter = options.Leases
			capture.healthFactory = options.Factory.(sandboxHealthProviderFactory)
			if capture.serviceErr != nil {
				return nil, capture.serviceErr
			}
			return &appsandbox.Service{}, nil
		},
		newRouter: func(
			_ appsandbox.ProviderLookup,
			factory appsandbox.RuntimeProviderFactory,
			limiter appsandbox.CapacityLimiter,
			_ time.Duration,
		) (*appsandbox.ProviderRouter, error) {
			capture.routerLimiter = limiter
			capture.runtimeFactory = factory.(sandboxRuntimeProviderFactory)
			capture.localDelegate = capture.runtimeFactory.factory.localDelegate
			return &appsandbox.ProviderRouter{}, nil
		},
		newSchedulerService: func(options appsandbox.SchedulerServiceOptions) (*appsandbox.SchedulerService, error) {
			capture.schedulerRunner = options.Runner
			return appsandbox.NewSchedulerService(options)
		},
		newSessionService: func(options appsandbox.SessionSettingsServiceOptions) (*appsandbox.SessionSettingsService, error) {
			capture.sessionStore = options.Store
			capture.sessionRunner = options.Runner
			capture.hostShellStatus = options.HostShellStatus
			return appsandbox.NewSessionSettingsService(options)
		},
	}
	t.Cleanup(func() { sandboxControlPlaneConstructors = previous })
}

func sandboxWiringReadyDependencies(runner coderunner.Runner) *appinfra.AppDependencies {
	var localRunner infrasandbox.LocalExecutionDelegate
	if candidate, ok := any(runner).(infrasandbox.LocalExecutionDelegate); ok {
		localRunner = candidate
	}
	return &appinfra.AppDependencies{
		DB:              &gorm.DB{},
		CacheCli:        &sandboxCacheStub{},
		CodeRunner:      runner,
		LocalCodeRunner: localRunner,
	}
}

type sandboxCacheStub struct {
	cache.Cmdable
}

type sandboxRunnerOnly struct{}

func (*sandboxRunnerOnly) Run(context.Context, *coderunner.RunRequest) (*coderunner.RunResponse, error) {
	return &coderunner.RunResponse{}, nil
}

type sandboxRunnerDelegate struct {
	sandboxRunnerOnly
}

func (*sandboxRunnerDelegate) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{ProtocolVersion: "v1", Status: domainsandbox.HealthStatusHealthy}, nil
}

func (*sandboxRunnerDelegate) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{}, nil
}

func (*sandboxRunnerDelegate) Cancel(context.Context, string) error {
	return nil
}

type sandboxSharedLimiterStub struct{}

func (*sandboxSharedLimiterStub) Acquire(context.Context, string, string, string, int, time.Duration) (int64, error) {
	return 0, nil
}

func (*sandboxSharedLimiterStub) Renew(context.Context, string, string, string, string, time.Duration) (int64, error) {
	return 0, nil
}

func (*sandboxSharedLimiterStub) Release(context.Context, string, string, string) error {
	return nil
}

func (*sandboxSharedLimiterStub) BeginDrain(context.Context, string) (appsandbox.DrainHandle, error) {
	return appsandbox.DrainHandle{}, nil
}

func (*sandboxSharedLimiterStub) AbortDrain(context.Context, string, appsandbox.DrainHandle) error {
	return nil
}

func (*sandboxSharedLimiterStub) Activate(context.Context, string, appsandbox.DrainFence) error {
	return nil
}

func (*sandboxSharedLimiterStub) RestoreActive(context.Context, string, appsandbox.DrainFence) error {
	return nil
}

func (*sandboxSharedLimiterStub) RetainDrain(context.Context, string, appsandbox.DrainFence) error {
	return nil
}

func (*sandboxSharedLimiterStub) CompensateActivation(
	context.Context,
	string,
	appsandbox.DrainFence,
) (appsandbox.ActivationCompensationResult, error) {
	var result appsandbox.ActivationCompensationResult
	return result, nil
}
