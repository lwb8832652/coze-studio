// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	appinfra "github.com/coze-dev/coze-studio/backend/application/base/appinfra"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

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
	require.NotNil(t, SandboxRuntimeRepository)
	require.Nil(t, SandboxRouter)
	_, ok := (sandboxMCPRuntimeBindingSource{}).LoadADKMCPRuntimeSandboxBinding()
	require.False(t, ok)
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
	assembled      bool
	localDelegate  infrasandbox.LocalExecutionDelegate
	limiter        *sandboxSharedLimiterStub
	serviceLimiter appsandbox.ProviderLifecycleGuard
	routerLimiter  appsandbox.CapacityLimiter
	healthFactory  sandboxHealthProviderFactory
	runtimeFactory sandboxRuntimeProviderFactory
	serviceErr     error
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
