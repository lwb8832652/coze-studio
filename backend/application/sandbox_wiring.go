// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	sandboxControlPlaneEnabledEnv   = "SANDBOX_CONTROL_PLANE_ENABLED"
	sandboxRuntimeRoutingEnabledEnv = "SANDBOX_RUNTIME_ROUTING_ENABLED"
)

const (
	sandboxAppEnv                               = "APP_ENV"
	sandboxHostRuntimeEnabledEnv                = "APP_DEV_HOST_RUNTIME_ENABLED"
	sandboxRunnerDeploymentIDEnv                = "SANDBOX_RUNNER_DEPLOYMENT_ID"
	sandboxRemoteProviderAllowedPrivateCIDRsEnv = "SANDBOX_REMOTE_PROVIDER_ALLOWED_PRIVATE_CIDRS"
)

var sandboxRemoteProviderPrivateRanges = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("fc00::/7"),
}

const maxSandboxRemoteProviderAllowedPrivateCIDRs = 16

const (
	defaultHostShellMaxSessions = 128
	defaultHostShellCancelGrace = 3 * time.Second
)

var (
	SandboxSVC               *appsandbox.Service
	SandboxSchedulerSVC      *appsandbox.SchedulerService
	SandboxSessionSVC        *appsandbox.SessionSettingsService
	SandboxRouter            *appsandbox.ProviderRouter
	SandboxRuntimeRepository sandboxRepository
	sandboxProviderFactory   *configuredSandboxProviderFactory
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
	domainsandbox.SessionSettingsAuditRepository
}

type sandboxSharedLimiter interface {
	appsandbox.ProviderLifecycleGuard
	appsandbox.CapacityLimiter
}

type sandboxWiringConstructors struct {
	loadCodec           func(func(string) string) (*infrasandbox.CredentialCodec, error)
	loadIdentitySigner  func(func(string) string) (sandboxidentity.Signer, error)
	loadSchedulerSigner func(func(string) string) (*infrasandbox.SchedulerConfigSigner, bool, error)
	newRepository       func(*appinfra.AppDependencies) sandboxRepository
	newLimiter          func(*appinfra.AppDependencies) (sandboxSharedLimiter, error)
	newService          func(appsandbox.ServiceOptions) (*appsandbox.Service, error)
	newRouter           func(appsandbox.ProviderLookup, appsandbox.RuntimeProviderFactory, appsandbox.CapacityLimiter, time.Duration) (*appsandbox.ProviderRouter, error)
	newSchedulerService func(appsandbox.SchedulerServiceOptions) (*appsandbox.SchedulerService, error)
	newSessionService   func(appsandbox.SessionSettingsServiceOptions) (*appsandbox.SessionSettingsService, error)
}

var sandboxControlPlaneConstructors = sandboxWiringConstructors{
	loadCodec: func(getenv func(string) string) (*infrasandbox.CredentialCodec, error) {
		keyRing, err := infrasandbox.LoadSandboxKeyRing(getenv)
		if err != nil {
			return nil, err
		}
		return infrasandbox.NewCredentialCodec(keyRing)
	},
	loadIdentitySigner: func(getenv func(string) string) (sandboxidentity.Signer, error) {
		keyring, configured, err := sandboxidentity.LoadKeyringFromEnv(getenv, 5*time.Minute)
		if err != nil {
			return nil, err
		}
		if !configured {
			return nil, nil
		}
		return keyring, nil
	},
	loadSchedulerSigner: func(getenv func(string) string) (*infrasandbox.SchedulerConfigSigner, bool, error) {
		return infrasandbox.LoadSchedulerConfigSignerFromEnv(getenv, 5*time.Minute)
	},
	newRepository: func(infra *appinfra.AppDependencies) sandboxRepository {
		return infrasandbox.NewMySQLRepository(infra.DB)
	},
	newLimiter: func(infra *appinfra.AppDependencies) (sandboxSharedLimiter, error) {
		return infrasandbox.NewRedisCapacityLimiter(infra.CacheCli)
	},
	newService:          appsandbox.NewService,
	newRouter:           appsandbox.NewProviderRouter,
	newSchedulerService: appsandbox.NewSchedulerService,
	newSessionService:   appsandbox.NewSessionSettingsService,
}

var errSandboxControlPlaneInitialization = fmt.Errorf("sandbox control plane initialization failed")

func clearSandboxControlPlane() {
	if sandboxProviderFactory != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		sandboxProviderFactory.shutdownHostShellSessions(shutdownCtx)
		shutdownCancel()
		sandboxProviderFactory = nil
	}
	clearSandboxMCPRuntimeBinding()
	SandboxSVC = nil
	SandboxSchedulerSVC = nil
	SandboxSessionSVC = nil
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
	allowedPrivateCIDRs, err := parseSandboxRemoteProviderAllowedPrivateCIDRs(
		os.Getenv(sandboxRemoteProviderAllowedPrivateCIDRsEnv),
	)
	if err != nil {
		return fmt.Errorf("%s is invalid", sandboxRemoteProviderAllowedPrivateCIDRsEnv)
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
	if constructors.loadCodec == nil || constructors.loadIdentitySigner == nil || constructors.loadSchedulerSigner == nil || constructors.newRepository == nil || constructors.newLimiter == nil ||
		constructors.newService == nil || constructors.newSchedulerService == nil || constructors.newSessionService == nil || (runtimeRoutingEnabled && constructors.newRouter == nil) {
		return fmt.Errorf("sandbox control plane constructors are incomplete")
	}
	codec, err := constructors.loadCodec(os.Getenv)
	if err != nil {
		return fmt.Errorf("load sandbox credential codec: %w", err)
	}
	identitySigner, err := constructors.loadIdentitySigner(os.Getenv)
	if err != nil {
		return fmt.Errorf("load sandbox execution identity signer: %w", err)
	}
	schedulerSigner, schedulerSigningConfigured, err := constructors.loadSchedulerSigner(os.Getenv)
	if err != nil {
		return fmt.Errorf("load sandbox scheduler signing keyring: %w", err)
	}
	repository := constructors.newRepository(infra)
	limiter, err := constructors.newLimiter(infra)
	if err != nil {
		return fmt.Errorf("create sandbox capacity limiter: %w", err)
	}
	metrics := appsandbox.NewSandboxPrometheusMetricsCollectorFromEnv()
	providerFactory := &configuredSandboxProviderFactory{
		codec: codec, identitySigner: identitySigner, sessionSigner: sessionSignerFromIdentitySigner(identitySigner),
		deploymentID:        strings.TrimSpace(os.Getenv(sandboxRunnerDeploymentIDEnv)),
		allowedPrivateCIDRs: append([]string(nil), allowedPrivateCIDRs...),
		localDelegate:       localDelegate, getenv: os.Getenv, metrics: metrics,
		hostRootDir:   filepath.Join(os.TempDir(), "coze-sandbox-host-shell"),
		hostSkillsDir: filepath.Join(os.TempDir(), "coze-sandbox-host-shell-skills"),
	}
	sandboxProviderFactory = providerFactory
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
	var schedulerRunner appsandbox.NativeSchedulerRunner
	if schedulerSigningConfigured {
		schedulerRunner, err = appsandbox.NewDefaultProviderSchedulerRunner(appsandbox.DefaultProviderSchedulerRunnerOptions{
			Signer: schedulerSigner, Defaults: repository, Providers: repository,
			Factory: sandboxRuntimeProviderFactory{factory: providerFactory}, Now: time.Now,
		})
		if err != nil {
			return fmt.Errorf("create sandbox scheduler runner: %w", err)
		}
	}
	schedulerService, err := constructors.newSchedulerService(appsandbox.SchedulerServiceOptions{Store: repository, Runner: schedulerRunner})
	if err != nil {
		return fmt.Errorf("create sandbox scheduler service: %w", err)
	}
	SandboxSchedulerSVC = schedulerService
	var sessionRunner appsandbox.NativeSessionRunner
	if providerFactory.sessionSigner != nil {
		deploymentID, deploymentErr := domainsandbox.NormalizeAIOGenerationDeploymentID(providerFactory.deploymentID)
		if deploymentErr == nil && deploymentID == providerFactory.deploymentID {
			sessionRunner, err = appsandbox.NewDefaultProviderSessionRunner(appsandbox.DefaultProviderSessionRunnerOptions{
				Defaults: repository, Providers: repository,
				Factory: sandboxRuntimeProviderFactory{factory: providerFactory},
			})
			if err != nil {
				return fmt.Errorf("create sandbox session runner: %w", err)
			}
		}
	}
	sessionService, err := constructors.newSessionService(appsandbox.SessionSettingsServiceOptions{
		Store: repository, Runner: sessionRunner,
		HostShellStatus: hostShellRuntimeStatusSource{
			getenv: providerFactory.environment(),
		},
	})
	if err != nil {
		return fmt.Errorf("create sandbox session settings service: %w", err)
	}
	SandboxSessionSVC = sessionService
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
	router.SetSessionSettingsRepository(repository)
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

func parseSandboxRemoteProviderAllowedPrivateCIDRs(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	values := strings.Split(raw, ",")
	if len(values) > maxSandboxRemoteProviderAllowedPrivateCIDRs {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.IsValid() || prefix.Addr().Zone() != "" || prefix.Addr().Is4In6() ||
			prefix != prefix.Masked() || prefix.String() != value || !sandboxRemoteProviderPrivatePrefixAllowed(prefix) {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func sandboxRemoteProviderPrivatePrefixAllowed(prefix netip.Prefix) bool {
	for _, privateRange := range sandboxRemoteProviderPrivateRanges {
		if prefix.Addr().BitLen() == privateRange.Addr().BitLen() &&
			prefix.Bits() >= privateRange.Bits() && privateRange.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}

func sandboxRemoteProviderAllowedAuthority(endpoint *url.URL) (string, error) {
	return safehttp.NormalizeAuthority(endpoint)
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
	codec               *infrasandbox.CredentialCodec
	identitySigner      sandboxidentity.Signer
	sessionSigner       sandboxidentity.SessionSigner
	deploymentID        string
	allowedPrivateCIDRs []string
	localDelegate       infrasandbox.LocalExecutionDelegate
	getenv              func(string) string
	metrics             appsandbox.SandboxMetricsRecorder
	hostRootDir         string
	hostSkillsDir       string
	hostMu              sync.Mutex
	hostManagers        map[int64]*infrasandbox.HostShellSessionManager
}

type hostShellRuntimeStatusSource struct {
	getenv func(string) string
}

func (s hostShellRuntimeStatusSource) HostShellAvailable(ctx context.Context) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return infrasandbox.HostShellSessionAllowed(s.getenv)
}

func (f *configuredSandboxProviderFactory) environment() func(string) string {
	if f != nil && f.getenv != nil {
		return f.getenv
	}
	return os.Getenv
}

func (f *configuredSandboxProviderFactory) hostSessionManager(
	provider domainsandbox.Provider,
) (*infrasandbox.HostShellSessionManager, error) {
	if f == nil || provider.ID <= 0 || !infrasandbox.HostShellSessionAllowed(f.environment()) ||
		!filepath.IsAbs(f.hostRootDir) || !filepath.IsAbs(f.hostSkillsDir) {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	f.hostMu.Lock()
	defer f.hostMu.Unlock()
	if current := f.hostManagers[provider.ID]; current != nil {
		if err := current.UpdateAllowedEnvironmentNames(provider.Policy.AllowedEnvNames); err != nil {
			return nil, err
		}
		return current, nil
	}
	manager, err := infrasandbox.NewHostShellSessionManager(infrasandbox.HostShellSessionManagerOptions{
		RootDir: f.hostRootDir, SkillsDir: f.hostSkillsDir, ExpectedProviderID: provider.ID,
		AllowedEnvironmentNames: append([]string(nil), provider.Policy.AllowedEnvNames...),
		Gate: func() bool {
			return infrasandbox.HostShellSessionAllowed(f.environment())
		},
		CancelGrace: defaultHostShellCancelGrace,
		MaxSessions: defaultHostShellMaxSessions,
	})
	if err != nil {
		return nil, err
	}
	if f.hostManagers == nil {
		f.hostManagers = make(map[int64]*infrasandbox.HostShellSessionManager)
	}
	f.hostManagers[provider.ID] = manager
	return manager, nil
}

func (f *configuredSandboxProviderFactory) shutdownHostShellSessions(ctx context.Context) {
	if f == nil {
		return
	}
	f.hostMu.Lock()
	managers := make([]*infrasandbox.HostShellSessionManager, 0, len(f.hostManagers))
	for _, manager := range f.hostManagers {
		managers = append(managers, manager)
	}
	f.hostManagers = nil
	f.hostMu.Unlock()
	for _, manager := range managers {
		_ = manager.Shutdown(ctx)
	}
}

type hostShellSessionRuntime struct {
	*infrasandbox.HostShellSessionManager
}

func (r *hostShellSessionRuntime) CloseContext(ctx context.Context) error {
	if r == nil || r.HostShellSessionManager == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	return ctx.Err()
}

func sessionSignerFromIdentitySigner(signer sandboxidentity.Signer) sandboxidentity.SessionSigner {
	if signer == nil {
		return nil
	}
	sessionSigner, _ := signer.(sandboxidentity.SessionSigner)
	return sessionSigner
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
		endpointAuthority, err := sandboxRemoteProviderAllowedAuthority(parsedEndpoint)
		if err != nil {
			return nil, domainsandbox.ErrConfigurationInvalid
		}
		allowedHosts := append([]string(nil), provider.Policy.NetworkAllowlist...)
		allowedHosts = append(allowedHosts, endpointAuthority)
		return infrasandbox.NewRemoteProvider(infrasandbox.RemoteProviderConfig{
			Endpoint:            string(endpoint),
			Credential:          string(credential),
			AllowedHosts:        allowedHosts,
			AllowedPrivateCIDRs: append([]string(nil), f.allowedPrivateCIDRs...),
			Timeout:             time.Duration(provider.Policy.TimeoutSeconds) * time.Second,
			IdentitySigner:      f.identitySigner,
		})
	default:
		return nil, domainsandbox.ErrConfigurationInvalid
	}
}

type sandboxHealthProviderFactory struct {
	factory *configuredSandboxProviderFactory
}

func (f sandboxHealthProviderFactory) Build(ctx context.Context, provider *domainsandbox.Provider) (appsandbox.HealthProvider, error) {
	if f.factory == nil || provider == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	if provider.Type == domainsandbox.ProviderTypeLocalDebug {
		return infrasandbox.NewLocalDebugHealthProvider(f.factory.localDelegate, provider.Scopes)
	}
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
		if descriptor.HasFeature(domainsandbox.ProviderFeatureSignedExecutionContext) && (f.factory == nil || f.factory.identitySigner == nil) {
			return domainsandbox.ErrConfigurationInvalid
		}
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

func (f sandboxRuntimeProviderFactory) ValidateSessionConfig(_ context.Context, descriptor appsandbox.ProviderDescriptor) error {
	if err := domainsandbox.ValidateRuntimePolicy(descriptor.Policy); err != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	if _, err := domainsandbox.NormalizeScopes([]domainsandbox.Scope{descriptor.Scope}); err != nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	if f.factory == nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	switch descriptor.ProviderType {
	case domainsandbox.ProviderTypeRemoteHTTP:
		if f.factory.sessionSigner == nil || f.factory.deploymentID == "" {
			return domainsandbox.ErrConfigurationInvalid
		}
		if normalized, err := domainsandbox.NormalizeAIOGenerationDeploymentID(f.factory.deploymentID); err != nil || normalized != f.factory.deploymentID {
			return domainsandbox.ErrConfigurationInvalid
		}
	case domainsandbox.ProviderTypeLocalDebug:
		if !infrasandbox.HostShellSessionAllowed(f.factory.environment()) ||
			!filepath.IsAbs(f.factory.hostRootDir) || !filepath.IsAbs(f.factory.hostSkillsDir) {
			return domainsandbox.ErrExecutionForbidden
		}
	default:
		return domainsandbox.ErrConfigurationInvalid
	}
	if !descriptor.HasFeature(domainsandbox.ProviderFeatureSandboxSessionV1) ||
		!descriptor.HasFeature(domainsandbox.ProviderFeatureSignedSessionContextV2) {
		return domainsandbox.ErrScopeUnsupported
	}
	return nil
}

func (f sandboxRuntimeProviderFactory) BuildSession(
	ctx context.Context,
	provider domainsandbox.Provider,
	descriptor appsandbox.ProviderDescriptor,
) (infrasandbox.SessionRuntimeProvider, error) {
	if f.factory == nil || provider.ID <= 0 ||
		descriptor.ProviderKey != provider.ProviderKey || descriptor.ProviderType != provider.Type ||
		!scopeIncludedForSandboxWiring(provider.Scopes, descriptor.Scope) {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	if provider.Type == domainsandbox.ProviderTypeLocalDebug {
		if !infrasandbox.HostShellSessionAllowed(f.factory.environment()) {
			return nil, domainsandbox.ErrExecutionForbidden
		}
		manager, err := f.factory.hostSessionManager(provider)
		if err != nil {
			return nil, err
		}
		return &hostShellSessionRuntime{HostShellSessionManager: manager}, nil
	}
	if provider.Type != domainsandbox.ProviderTypeRemoteHTTP || f.factory.sessionSigner == nil || f.factory.deploymentID == "" {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	runtime, err := f.factory.build(ctx, &provider)
	if err != nil {
		return nil, err
	}
	remote, ok := runtime.(*infrasandbox.RemoteProvider)
	if !ok {
		if runtime != nil {
			_ = runtime.CloseContext(ctx)
		}
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	session, err := infrasandbox.NewRemoteSessionProvider(remote, infrasandbox.RemoteSessionProviderConfig{
		DeploymentID:            f.factory.deploymentID,
		ProviderID:              provider.ID,
		Scope:                   sandboxidentity.Scope(descriptor.Scope),
		SessionSigner:           f.factory.sessionSigner,
		AllowedEnvironmentNames: append([]string(nil), provider.Policy.AllowedEnvNames...),
	})
	if err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_ = remote.CloseContext(cleanupCtx)
		cleanupCancel()
		return nil, err
	}
	return session, nil
}

func scopeIncludedForSandboxWiring(scopes []domainsandbox.Scope, requested domainsandbox.Scope) bool {
	for _, scope := range scopes {
		if scope == requested {
			return true
		}
	}
	return false
}

func wipeSandboxWiringBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
