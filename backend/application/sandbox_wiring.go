// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	appinfra "github.com/coze-dev/coze-studio/backend/application/base/appinfra"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	codecontrolplane "github.com/coze-dev/coze-studio/backend/infra/coderunner/impl/controlplane"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	sandboxControlPlaneEnabledEnv   = "SANDBOX_CONTROL_PLANE_ENABLED"
	sandboxRuntimeRoutingEnabledEnv = "SANDBOX_RUNTIME_ROUTING_ENABLED"
)

const (
	sandboxAppEnv                = "APP_ENV"
	sandboxHostRuntimeEnabledEnv = "APP_DEV_HOST_RUNTIME_ENABLED"
)

var (
	SandboxSVC               *appsandbox.Service
	SandboxSchedulerSVC      *appsandbox.SchedulerService
	SandboxRouter            *appsandbox.ProviderRouter
	SandboxRuntimeRepository sandboxRepository
)

var sandboxMCPRuntimeRegistry struct {
	sync.RWMutex
	generation uint64
	repository sandboxRepository
	router     *appsandbox.ProviderRouter
}

type sandboxMCPRuntimeBindingToken struct {
	generation uint64
}

type sandboxMCPRuntimeBindingSource struct{}

type sandboxCodeRunnerBindingSource struct{}

type sandboxMCPRuntimeRouterAdapter struct {
	router *appsandbox.ProviderRouter
}

type sandboxMCPRuntimeSelectionAdapter struct {
	selection *appsandbox.SelectedProvider
}

type sandboxRepository interface {
	domainsandbox.ProviderRepository
	domainsandbox.ProviderDefaultRepository
	domainsandbox.ProviderManagementRepository
	domainsandbox.ProviderCreateUnitOfWork
	appsandbox.ProviderLookup
	domainsandbox.SchedulerSettingsAuditRepository
}

type sandboxSharedLimiter interface {
	appsandbox.ProviderLifecycleGuard
	appsandbox.CapacityLimiter
}

type sandboxWiringConstructors struct {
	loadCodec     func(func(string) string) (*infrasandbox.CredentialCodec, error)
	newRepository func(*appinfra.AppDependencies) sandboxRepository
	newLimiter    func(*appinfra.AppDependencies) (sandboxSharedLimiter, error)
	newService    func(appsandbox.ServiceOptions) (*appsandbox.Service, error)
	newRouter     func(appsandbox.ProviderLookup, appsandbox.RuntimeProviderFactory, appsandbox.CapacityLimiter, time.Duration) (*appsandbox.ProviderRouter, error)
}

var sandboxControlPlaneConstructors = sandboxWiringConstructors{
	loadCodec: func(getenv func(string) string) (*infrasandbox.CredentialCodec, error) {
		keyRing, err := infrasandbox.LoadSandboxKeyRing(getenv)
		if err != nil {
			return nil, err
		}
		return infrasandbox.NewCredentialCodec(keyRing)
	},
	newRepository: func(infra *appinfra.AppDependencies) sandboxRepository {
		return infrasandbox.NewMySQLRepository(infra.DB)
	},
	newLimiter: func(infra *appinfra.AppDependencies) (sandboxSharedLimiter, error) {
		return infrasandbox.NewRedisCapacityLimiter(infra.CacheCli)
	},
	newService: appsandbox.NewService,
	newRouter:  appsandbox.NewProviderRouter,
}

var errSandboxControlPlaneInitialization = fmt.Errorf("sandbox control plane initialization failed")

func clearSandboxControlPlane() {
	clearSandboxMCPRuntimeBinding()
	SandboxSVC = nil
	SandboxSchedulerSVC = nil
	SandboxRouter = nil
	SandboxRuntimeRepository = nil
}

func initSandboxControlPlaneForApplication(infra *appinfra.AppDependencies) error {
	if err := initSandboxControlPlane(infra); err != nil {
		clearSandboxControlPlane()
		return errSandboxControlPlaneInitialization
	}
	return nil
}

func initSandboxControlPlane(infra *appinfra.AppDependencies) error {
	clearSandboxControlPlane()
	switch os.Getenv(sandboxControlPlaneEnabledEnv) {
	case "", "false":
		return nil
	case "true":
	default:
		return fmt.Errorf("%s must be true or false", sandboxControlPlaneEnabledEnv)
	}
	runtimeRoutingEnabled, err := sandboxRuntimeRoutingEnabled(os.Getenv)
	if err != nil {
		return err
	}
	if infra == nil || infra.DB == nil || infra.CacheCli == nil {
		return fmt.Errorf("sandbox control plane dependencies are incomplete")
	}
	var localDelegate infrasandbox.LocalExecutionDelegate
	if sandboxLocalDebugEnabled(os.Getenv) {
		if infra.LocalCodeRunner == nil {
			return fmt.Errorf("sandbox local debug runner is unavailable")
		}
		localDelegate = infra.LocalCodeRunner
	}

	constructors := sandboxControlPlaneConstructors
	if constructors.loadCodec == nil || constructors.newRepository == nil || constructors.newLimiter == nil ||
		constructors.newService == nil || (runtimeRoutingEnabled && constructors.newRouter == nil) {
		return fmt.Errorf("sandbox control plane constructors are incomplete")
	}
	codec, err := constructors.loadCodec(os.Getenv)
	if err != nil {
		return fmt.Errorf("load sandbox credential codec: %w", err)
	}
	repository := constructors.newRepository(infra)
	limiter, err := constructors.newLimiter(infra)
	if err != nil {
		return fmt.Errorf("create sandbox capacity limiter: %w", err)
	}
	metrics := appsandbox.NewSandboxPrometheusMetricsCollectorFromEnv()
	providerFactory := &configuredSandboxProviderFactory{
		codec: codec, localDelegate: localDelegate, metrics: metrics,
	}
	service, err := constructors.newService(appsandbox.ServiceOptions{
		Providers:  repository,
		UnitOfWork: repository,
		Management: repository,
		Codec:      codec,
		Leases:     limiter,
		Factory:    sandboxHealthProviderFactory{factory: providerFactory},
		Metrics:    metrics,
	})
	if err != nil {
		return fmt.Errorf("create sandbox control plane service: %w", err)
	}
	SandboxSVC = service
	schedulerService, err := appsandbox.NewSchedulerService(appsandbox.SchedulerServiceOptions{Store: repository})
	if err != nil {
		return fmt.Errorf("create sandbox scheduler service: %w", err)
	}
	SandboxSchedulerSVC = schedulerService
	SandboxRuntimeRepository = repository
	if !runtimeRoutingEnabled {
		return nil
	}
	router, err := constructors.newRouter(
		repository,
		sandboxRuntimeProviderFactory{factory: providerFactory},
		limiter,
		5*time.Minute,
	)
	if err != nil {
		return fmt.Errorf("create sandbox provider router: %w", err)
	}
	router.SetSchedulerSettingsRepository(repository)
	router.SetMetricsRecorder(metrics)
	if auditRepository, ok := any(repository).(domainsandbox.ProviderAuditRepository); ok {
		router.SetRuntimeAuditRecorder(appsandbox.NewProviderRuntimeAuditRecorder(
			auditRepository,
			func(ctx context.Context) int64 {
				if uid := ctxutil.GetUIDFromCtx(ctx); uid != nil {
					return *uid
				}
				return 0
			},
		))
	}
	controlPlaneCodeRunner, err := codecontrolplane.NewRunner(
		codecontrolplane.Options{
			BindingSource: sandboxCodeRunnerBindingSource{},
		},
	)
	if err != nil {
		return fmt.Errorf("create sandbox code runner: %w", err)
	}
	if _, err := publishSandboxMCPRuntimeBinding(repository, router); err != nil {
		return fmt.Errorf("publish sandbox mcp runtime binding: %w", err)
	}
	SandboxRouter = router
	infra.CodeRunner = controlPlaneCodeRunner
	return nil
}

func sandboxRuntimeRoutingEnabled(getenv func(string) string) (bool, error) {
	if getenv == nil || getenv(sandboxControlPlaneEnabledEnv) != "true" {
		return false, nil
	}
	switch getenv(sandboxRuntimeRoutingEnabledEnv) {
	case "", "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", sandboxRuntimeRoutingEnabledEnv)
	}
}

func publishSandboxMCPRuntimeBinding(
	repository sandboxRepository,
	router *appsandbox.ProviderRouter,
) (sandboxMCPRuntimeBindingToken, error) {
	if repository == nil || router == nil {
		return sandboxMCPRuntimeBindingToken{}, domainsandbox.ErrConfigurationInvalid
	}
	sandboxMCPRuntimeRegistry.Lock()
	defer sandboxMCPRuntimeRegistry.Unlock()
	sandboxMCPRuntimeRegistry.generation++
	sandboxMCPRuntimeRegistry.repository = repository
	sandboxMCPRuntimeRegistry.router = router
	return sandboxMCPRuntimeBindingToken{
		generation: sandboxMCPRuntimeRegistry.generation,
	}, nil
}

func unpublishSandboxMCPRuntimeBinding(token sandboxMCPRuntimeBindingToken) bool {
	if token.generation == 0 {
		return false
	}
	sandboxMCPRuntimeRegistry.Lock()
	defer sandboxMCPRuntimeRegistry.Unlock()
	if sandboxMCPRuntimeRegistry.generation != token.generation ||
		sandboxMCPRuntimeRegistry.repository == nil ||
		sandboxMCPRuntimeRegistry.router == nil {
		return false
	}
	sandboxMCPRuntimeRegistry.generation++
	sandboxMCPRuntimeRegistry.repository = nil
	sandboxMCPRuntimeRegistry.router = nil
	return true
}

func clearSandboxMCPRuntimeBinding() {
	sandboxMCPRuntimeRegistry.Lock()
	defer sandboxMCPRuntimeRegistry.Unlock()
	sandboxMCPRuntimeRegistry.generation++
	sandboxMCPRuntimeRegistry.repository = nil
	sandboxMCPRuntimeRegistry.router = nil
}

func (sandboxMCPRuntimeBindingSource) LoadADKMCPRuntimeSandboxBinding() (
	agentthread.ADKMCPRuntimeSandboxBinding,
	bool,
) {
	sandboxMCPRuntimeRegistry.RLock()
	defer sandboxMCPRuntimeRegistry.RUnlock()
	if sandboxMCPRuntimeRegistry.repository == nil || sandboxMCPRuntimeRegistry.router == nil {
		return agentthread.ADKMCPRuntimeSandboxBinding{}, false
	}
	return agentthread.ADKMCPRuntimeSandboxBinding{
		ProviderDefaults: sandboxMCPRuntimeRegistry.repository,
		Providers:        sandboxMCPRuntimeRegistry.repository,
		Router: sandboxMCPRuntimeRouterAdapter{
			router: sandboxMCPRuntimeRegistry.router,
		},
	}, true
}

func (sandboxCodeRunnerBindingSource) LoadCodeRunnerSandboxBinding() (
	codecontrolplane.Binding,
	bool,
) {
	sandboxMCPRuntimeRegistry.RLock()
	defer sandboxMCPRuntimeRegistry.RUnlock()
	if sandboxMCPRuntimeRegistry.repository == nil ||
		sandboxMCPRuntimeRegistry.router == nil {
		return codecontrolplane.Binding{}, false
	}
	return codecontrolplane.Binding{
		ProviderDefaults: sandboxMCPRuntimeRegistry.repository,
		Providers:        sandboxMCPRuntimeRegistry.repository,
		Router: sandboxMCPRuntimeRouterAdapter{
			router: sandboxMCPRuntimeRegistry.router,
		},
	}, true
}

func (a sandboxMCPRuntimeRouterAdapter) ResolveADKMCPRuntimeSandbox(
	ctx context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (agentthread.ADKMCPRuntimeSandboxSelection, error) {
	if a.router == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	request, err := appsandbox.NewResolveProviderRequest(providerKey, scope)
	if err != nil {
		return nil, err
	}
	selection, err := a.router.Resolve(ctx, request)
	if err != nil {
		return nil, err
	}
	return sandboxMCPRuntimeSelectionAdapter{selection: selection}, nil
}

func (a sandboxMCPRuntimeRouterAdapter) ResolveCodeRunnerSandbox(
	ctx context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (codecontrolplane.Selection, error) {
	if a.router == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	request, err := appsandbox.NewResolveProviderRequest(providerKey, scope)
	if err != nil {
		return nil, err
	}
	selection, err := a.router.Resolve(ctx, request)
	if err != nil {
		return nil, err
	}
	return sandboxMCPRuntimeSelectionAdapter{selection: selection}, nil
}

func (a sandboxMCPRuntimeSelectionAdapter) ADKMCPRuntimeSandboxPolicy() domainsandbox.RuntimePolicy {
	if a.selection == nil {
		return domainsandbox.RuntimePolicy{}
	}
	return a.selection.Policy
}

func (a sandboxMCPRuntimeSelectionAdapter) CodeRunnerSandboxPolicy() domainsandbox.RuntimePolicy {
	if a.selection == nil {
		return domainsandbox.RuntimePolicy{}
	}
	return a.selection.Policy
}

func (a sandboxMCPRuntimeSelectionAdapter) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	if a.selection == nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	return a.selection.Execute(ctx, request)
}

func (a sandboxMCPRuntimeSelectionAdapter) Status(
	ctx context.Context,
) (infrasandbox.ExecuteResult, error) {
	if a.selection == nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	return a.selection.Status(ctx)
}

func (a sandboxMCPRuntimeSelectionAdapter) Cancel(
	ctx context.Context,
	executionID string,
) error {
	if a.selection == nil {
		return domainsandbox.ErrUnavailable
	}
	return a.selection.Cancel(ctx, executionID)
}

func (a sandboxMCPRuntimeSelectionAdapter) Release(ctx context.Context) error {
	if a.selection == nil {
		return domainsandbox.ErrUnavailable
	}
	return a.selection.Release(ctx)
}

func sandboxLocalDebugEnabled(getenv func(string) string) bool {
	return getenv != nil && getenv(sandboxAppEnv) == "debug" &&
		getenv(sandboxHostRuntimeEnabledEnv) == "true"
}

func validateSandboxHTTPSURL(raw, name string) error {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("sandbox %s must be an HTTPS URL", name)
	}
	return nil
}

type configuredSandboxProviderFactory struct {
	codec         *infrasandbox.CredentialCodec
	localDelegate infrasandbox.LocalExecutionDelegate
	metrics       appsandbox.SandboxMetricsRecorder
}

func (f *configuredSandboxProviderFactory) build(
	_ context.Context,
	provider *domainsandbox.Provider,
) (infrasandbox.RuntimeProvider, error) {
	if f == nil || f.codec == nil || provider == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	switch provider.Type {
	case domainsandbox.ProviderTypeLocalDebug:
		if f.localDelegate == nil {
			return nil, domainsandbox.ErrUnavailable
		}
		return infrasandbox.NewLocalDebugProvider(f.localDelegate)
	case domainsandbox.ProviderTypeRemoteHTTP:
		endpoint, err := f.codec.Decrypt(provider.ProviderKey, infrasandbox.CredentialFieldEndpoint, provider.EndpointSecret)
		if err != nil {
			if f.metrics != nil {
				f.metrics.RecordCredentialDecryptFailure(
					context.Background(),
					appsandbox.CredentialDecryptFailureMetricsObservation{
						ProviderType: provider.Type,
						Field:        string(infrasandbox.CredentialFieldEndpoint),
						ResultCode:   domainsandbox.ErrCodeConfigurationInvalid,
					},
				)
			}
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		defer wipeSandboxWiringBytes(endpoint)
		credential, err := f.codec.Decrypt(provider.ProviderKey, infrasandbox.CredentialFieldCredential, provider.CredentialSecret)
		if err != nil {
			if f.metrics != nil {
				f.metrics.RecordCredentialDecryptFailure(
					context.Background(),
					appsandbox.CredentialDecryptFailureMetricsObservation{
						ProviderType: provider.Type,
						Field:        string(infrasandbox.CredentialFieldCredential),
						ResultCode:   domainsandbox.ErrCodeConfigurationInvalid,
					},
				)
			}
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		defer wipeSandboxWiringBytes(credential)
		parsedEndpoint, err := url.Parse(string(endpoint))
		if err != nil || parsedEndpoint == nil || parsedEndpoint.Hostname() == "" {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		allowedHosts := append([]string(nil), provider.Policy.NetworkAllowlist...)
		allowedHosts = append(allowedHosts, strings.ToLower(parsedEndpoint.Hostname()))
		return infrasandbox.NewRemoteProvider(infrasandbox.RemoteProviderConfig{
			Endpoint:     string(endpoint),
			Credential:   string(credential),
			AllowedHosts: allowedHosts,
			Timeout:      time.Duration(provider.Policy.TimeoutSeconds) * time.Second,
		})
	default:
		return nil, domainsandbox.ErrConfigurationInvalid
	}
}

type sandboxHealthProviderFactory struct {
	factory *configuredSandboxProviderFactory
}

func (f sandboxHealthProviderFactory) Build(ctx context.Context, provider *domainsandbox.Provider) (appsandbox.HealthProvider, error) {
	return f.factory.build(ctx, provider)
}

type sandboxRuntimeProviderFactory struct {
	factory *configuredSandboxProviderFactory
}

func (f sandboxRuntimeProviderFactory) ValidateConfig(_ context.Context, descriptor appsandbox.ProviderDescriptor) error {
	if err := domainsandbox.ValidateRuntimePolicy(descriptor.Policy); err != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	if _, err := domainsandbox.NormalizeScopes([]domainsandbox.Scope{descriptor.Scope}); err != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	switch descriptor.ProviderType {
	case domainsandbox.ProviderTypeRemoteHTTP:
		return nil
	case domainsandbox.ProviderTypeLocalDebug:
		if f.factory == nil || f.factory.localDelegate == nil {
			return domainsandbox.ErrUnavailable
		}
		_, err := infrasandbox.NewLocalDebugProvider(f.factory.localDelegate)
		return err
	default:
		return domainsandbox.ErrConfigurationInvalid
	}
}

func (f sandboxRuntimeProviderFactory) Build(ctx context.Context, provider domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return f.factory.build(ctx, &provider)
}

func wipeSandboxWiringBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
