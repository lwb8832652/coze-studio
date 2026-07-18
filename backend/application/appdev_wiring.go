// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	appinfra "github.com/coze-dev/coze-studio/backend/application/base/appinfra"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const (
	appDevPreviewGatewayEnv              = "APP_DEV_PREVIEW_GATEWAY_BASE_URL"
	appDevArtifactGatewayBaseURLEnv      = "APP_DEV_ARTIFACT_GATEWAY_BASE_URL"
	appDevArtifactTrustedProxyCIDRsEnv   = "APP_DEV_ARTIFACT_TRUSTED_PROXY_CIDRS"
	appDevProviderAuthenticationTokenEnv = "APP_DEV_PROVIDER_AUTH_TOKEN"
	appDevRuntimeEntrypoint              = "appdev/runtime"
	appDevRuntimeExecutionTimeout        = 30 * time.Minute
	appDevProviderLeaseDuration          = 5 * time.Minute
	appDevArtifactGrantTTL               = 60 * time.Second
	appDevRecoveryInterval               = 30 * time.Second
	appDevRecoveryErrorBackoff           = 5 * time.Second
	appDevRecoveryAttemptTimeout         = 20 * time.Second
	appDevRecoveryBatchSize              = 64
	appDevProviderAuthorizationHeader    = "Authorization"
	appDevProviderAuthorizationPrefix    = "Bearer "
	appDevProviderMinimumCredentialBytes = 16
)

var errAppDevProviderWiringUnavailable = errors.New("appdev provider control unavailable")
var appDevProviderInitializationMu sync.Mutex

func initAppDevProviderRuntimeForApplication(
	ctx context.Context,
	dependencies appDevProviderWiringDependencies,
) (*appDevProviderRuntime, error) {
	runtime, err := initAppDevProviderRuntime(ctx, dependencies)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	routingEnabled, routingErr := sandboxRuntimeRoutingEnabled(os.Getenv)
	if routingErr != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	if runtime == nil && routingEnabled {
		return nil, errAppDevProviderWiringUnavailable
	}
	return runtime, nil
}

func appDevProjectRuntimeLifecycleMode(getenv func(string) string) appdevapp.ProjectRuntimeLifecycleMode {
	if getenv != nil && getenv(sandboxAppEnv) == "debug" &&
		getenv(sandboxHostRuntimeEnabledEnv) == "true" {
		return appdevapp.ProjectRuntimeLifecycleModeLegacyDebug
	}
	return appdevapp.ProjectRuntimeLifecycleModeProviderRequired
}

type appDevProviderRepository interface {
	GetProviderDefault(context.Context, domainsandbox.Scope) (*domainsandbox.ProviderDefault, error)
	GetProvider(context.Context, int64) (*domainsandbox.Provider, error)
}

type appDevProviderWiringDependencies struct {
	Infra     *appinfra.AppDependencies
	Service   *appdevapp.Service
	Store     *infraappdev.PersistentStore
	Providers appDevProviderRepository
	Router    *appsandbox.ProviderRouter
}

type appDevProviderRuntime struct {
	facade                *appdevapp.ProviderAPIFacade
	gatewayHandler        *appdevapp.ProviderHTTPDependencies
	scheduler             *appDevRecoveryScheduler
	service               *appdevapp.Service
	archive               appdevapp.ProviderProjectArchiveLifecycle
	serviceOwner          appdevapp.ProviderRuntimePublication
	httpOwner             appdevapp.ProviderHTTPPublication
	quarantine            *appdevapp.ProviderQuarantineDispositionService
	quarantineOwner       appdevapp.ProviderQuarantineDispositionPublication
	shutdownOwner         applicationShutdownRegistration
	shutdownMu            sync.Mutex
	schedulerStopped      bool
	quarantineUnpublished bool
	httpUnpublished       bool
	serviceUnpublished    bool
}

func initAppDevProviderRuntime(ctx context.Context, dependencies appDevProviderWiringDependencies) (*appDevProviderRuntime, error) {
	appDevProviderInitializationMu.Lock()
	defer appDevProviderInitializationMu.Unlock()
	if appdevapp.CurrentProviderHTTPDependencies() != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	if dependencies.Service != nil {
		if _, existingErr := dependencies.Service.ProviderAPI(); existingErr == nil {
			return nil, errAppDevProviderWiringUnavailable
		}
	}
	runtime, err := newAppDevProviderRuntime(ctx, dependencies, os.Getenv)
	if err != nil || runtime == nil {
		return nil, err
	}
	runtime.service = dependencies.Service
	serviceOwner, err := dependencies.Service.PublishProviderRuntimeBindings(runtime.facade, runtime.archive)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	runtime.serviceOwner = serviceOwner
	httpOwner, err := appdevapp.PublishProviderHTTPDependencies(runtime.gatewayHandler)
	if err != nil {
		dependencies.Service.UnpublishProviderRuntimeBindings(serviceOwner)
		return nil, errAppDevProviderWiringUnavailable
	}
	runtime.httpOwner = httpOwner
	quarantineOwner, err := appdevapp.PublishProviderQuarantineDispositionControl(runtime.quarantine)
	if err != nil {
		appdevapp.UnpublishProviderHTTPDependencies(httpOwner)
		dependencies.Service.UnpublishProviderRuntimeBindings(serviceOwner)
		return nil, errAppDevProviderWiringUnavailable
	}
	runtime.quarantineOwner = quarantineOwner
	runtime.scheduler.Start(ctx)
	shutdownOwner, err := applicationShutdowns.Register(runtime)
	if err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appDevRecoveryAttemptTimeout)
		defer cancel()
		_ = runtime.Shutdown(rollbackCtx)
		return nil, errAppDevProviderWiringUnavailable
	}
	runtime.shutdownOwner = shutdownOwner
	return runtime, nil
}

func newAppDevProviderRuntime(
	ctx context.Context,
	dependencies appDevProviderWiringDependencies,
	getenv func(string) string,
) (*appDevProviderRuntime, error) {
	if getenv == nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	routingEnabled, routingErr := sandboxRuntimeRoutingEnabled(getenv)
	if routingErr != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	if !routingEnabled {
		return nil, nil
	}
	if ctx == nil || dependencies.Infra == nil || dependencies.Infra.DB == nil ||
		appDevInterfaceNil(dependencies.Infra.CacheCli) || appDevInterfaceNil(dependencies.Infra.OSS) ||
		dependencies.Service == nil || dependencies.Store == nil ||
		dependencies.Providers == nil || dependencies.Router == nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	if err := checkAppDevProviderDependenciesReady(
		ctx,
		dependencies.Infra.CacheCli,
		dependencies.Infra.OSS,
	); err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}

	keyRing, err := infrasandbox.LoadSandboxKeyRing(getenv)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	credentialCodec, err := infrasandbox.NewCredentialCodec(keyRing)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	defaultProvider, err := dependencies.Providers.GetProviderDefault(ctx, domainsandbox.ScopeAppDev)
	if err != nil || defaultProvider == nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	provider, err := dependencies.Providers.GetProvider(ctx, defaultProvider.ProviderID)
	if err != nil || provider == nil || provider.Status != domainsandbox.ProviderStatusEnabled ||
		domainsandbox.ValidateProviderKey(provider.ProviderKey) != nil ||
		domainsandbox.ValidateRuntimePolicy(provider.Policy) != nil ||
		!appDevProviderSupportsScope(provider, domainsandbox.ScopeAppDev) {
		return nil, errAppDevProviderWiringUnavailable
	}
	providerCredential, debug, err := appDevProviderCredential(provider, credentialCodec, getenv)
	if err != nil {
		return nil, fmt.Errorf("provider credential gate: %w", errAppDevProviderWiringUnavailable)
	}

	previewProjector, err := newAppDevConfiguredPreviewProjector(getenv(appDevPreviewGatewayEnv), debug)
	if err != nil {
		return nil, fmt.Errorf("preview projector gate: %w", errAppDevProviderWiringUnavailable)
	}
	artifactURLPolicy := appdevapp.NewArtifactCapabilityURLPolicy(debug)
	endpoints, err := newAppDevArtifactEndpointBuilder(getenv(appDevArtifactGatewayBaseURLEnv), artifactURLPolicy)
	if err != nil {
		return nil, fmt.Errorf("artifact endpoint gate: %w", errAppDevProviderWiringUnavailable)
	}
	trustedProxies, err := parseAppDevTrustedProxyCIDRs(getenv(appDevArtifactTrustedProxyCIDRsEnv))
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}

	scriptClient, ok := dependencies.Infra.CacheCli.(cache.ScriptCmdable)
	if !ok || scriptClient == nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	grantRepository, err := infraappdev.NewRedisArtifactGrantRepository(scriptClient)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	grants, err := appdevapp.NewArtifactGrantLifecycleService(grantRepository)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	artifactStore, err := infraappdev.NewArtifactStore(dependencies.Infra.OSS)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	gateway, err := appdevapp.NewArtifactGateway(grants, artifactStore)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	authenticator, err := newAppDevProviderAuthenticator(provider.ProviderKey, providerCredential, grantRepository)
	if err != nil {
		return nil, fmt.Errorf("provider authentication gate: %w", errAppDevProviderWiringUnavailable)
	}
	httpDependencies, err := appdevapp.NewProviderHTTPDependencies(
		gateway, authenticator,
		&appDevSecureTransportVerifier{trustedProxies: trustedProxies, debugLoopback: debug},
		previewProjector,
	)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}

	protector, err := infrasandbox.NewExecutionCheckpointProtector(keyRing)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	checkpointCodec, err := appsandbox.NewExecutionCheckpointCodec(protector)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	executionRepository := infraappdev.NewProviderExecutionRepository(dependencies.Infra.DB)
	executions, err := appdevapp.NewProviderExecutionService(executionRepository, checkpointCodec)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	quarantine, err := appdevapp.NewProviderQuarantineDispositionService(executionRepository, cryptorand.Reader)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(cryptorand.Reader)
	if err != nil || ownerToken.IsZero() {
		return nil, errAppDevProviderWiringUnavailable
	}
	ownerGenerator := appDevFixedOwnerGenerator{token: ownerToken}
	router := appdevapp.SandboxProviderRuntimeRouter{Router: dependencies.Router}
	runtimeOrchestrator, err := appdevapp.NewProviderRuntimeOrchestrator(
		executions,
		executions,
		router,
		dependencies.Store,
		grants,
		endpoints,
		ownerGenerator,
		appdevapp.ProviderRuntimeOrchestratorConfig{
			ProviderKey: provider.ProviderKey, ProviderScope: domainsandbox.ScopeAppDev,
			Policy: provider.Policy, Entrypoint: appDevRuntimeEntrypoint,
			ExecutionTimeout: appDevRuntimeExecutionTimeout, GrantTTL: appDevArtifactGrantTTL,
			ProviderLeaseDuration: appDevProviderLeaseDuration, ArtifactURLPolicy: artifactURLPolicy,
		},
	)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	publisher, err := appdevapp.NewBuildArtifactPublisherService(
		grants, endpoints, artifactStore, appDevArtifactGrantTTL,
		appdevapp.WithBuildArtifactURLPolicy(artifactURLPolicy),
	)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	buildOrchestrator, err := appdevapp.NewProviderBuildOrchestrator(
		executions,
		router,
		publisher,
		ownerGenerator,
		appdevapp.ProviderBuildOrchestratorConfig{ProviderKey: provider.ProviderKey, ProviderScope: domainsandbox.ScopeAppDev},
	)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	releases, err := appdevapp.NewProviderReadyReleaseReader(executions, dependencies.Store, artifactStore)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	facade := appdevapp.NewProviderAPIFacade(runtimeOrchestrator, buildOrchestrator, releases, dependencies.Store)
	archiveLifecycle, err := appdevapp.NewProviderProjectArchiveController(executions, runtimeOrchestrator)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	scheduler, err := newAppDevRecoveryScheduler(
		executionRepository,
		runtimeOrchestrator,
		buildOrchestrator,
		runtimeOrchestrator,
		dependencies.Store,
		appDevRecoverySchedulerConfig{
			Interval: appDevRecoveryInterval, ErrorBackoff: appDevRecoveryErrorBackoff,
			AttemptTimeout: appDevRecoveryAttemptTimeout, BatchSize: appDevRecoveryBatchSize,
		},
	)
	if err != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	return &appDevProviderRuntime{
		facade: facade, gatewayHandler: httpDependencies, scheduler: scheduler, archive: archiveLifecycle,
		quarantine: quarantine,
	}, nil
}

func checkAppDevProviderDependenciesReady(
	parent context.Context,
	cacheClient cache.Cmdable,
	objectStorage storage.Storage,
) error {
	if parent == nil || appDevInterfaceNil(cacheClient) || appDevInterfaceNil(objectStorage) {
		return errAppDevProviderWiringUnavailable
	}
	cacheReadiness, cacheOK := cacheClient.(cache.ReadinessChecker)
	storageReadiness, storageOK := objectStorage.(storage.ReadinessChecker)
	if !cacheOK || !storageOK ||
		appDevInterfaceNil(cacheReadiness) || appDevInterfaceNil(storageReadiness) {
		return errAppDevProviderWiringUnavailable
	}
	readinessCtx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := cacheReadiness.CheckReadiness(readinessCtx); err != nil {
		return errAppDevProviderWiringUnavailable
	}
	if err := storageReadiness.CheckReadiness(readinessCtx); err != nil {
		return errAppDevProviderWiringUnavailable
	}
	return nil
}

func appDevInterfaceNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

type appDevFixedOwnerGenerator struct {
	token domainappdev.ProviderExecutionOwnerToken
}

func (generator appDevFixedOwnerGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	if generator.token.IsZero() {
		return domainappdev.ProviderExecutionOwnerToken{}, errAppDevProviderWiringUnavailable
	}
	return generator.token, nil
}

func appDevProviderSupportsScope(provider *domainsandbox.Provider, scope domainsandbox.Scope) bool {
	for _, candidate := range provider.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func appDevProviderCredential(
	provider *domainsandbox.Provider,
	codec *infrasandbox.CredentialCodec,
	getenv func(string) string,
) ([]byte, bool, error) {
	if provider == nil || codec == nil || getenv == nil {
		return nil, false, errAppDevProviderWiringUnavailable
	}
	debug := getenv(sandboxAppEnv) == "debug" && getenv(sandboxHostRuntimeEnabledEnv) == "true"
	switch provider.Type {
	case domainsandbox.ProviderTypeRemoteHTTP:
		if debug {
			return nil, false, errAppDevProviderWiringUnavailable
		}
		endpoint, err := codec.Decrypt(provider.ProviderKey, infrasandbox.CredentialFieldEndpoint, provider.EndpointSecret)
		if err != nil {
			return nil, false, errAppDevProviderWiringUnavailable
		}
		defer wipeSandboxWiringBytes(endpoint)
		if validateSandboxHTTPSURL(string(endpoint), "provider endpoint") != nil {
			return nil, false, errAppDevProviderWiringUnavailable
		}
		credential, err := codec.Decrypt(provider.ProviderKey, infrasandbox.CredentialFieldCredential, provider.CredentialSecret)
		if err != nil || len(credential) < appDevProviderMinimumCredentialBytes {
			wipeSandboxWiringBytes(credential)
			return nil, false, errAppDevProviderWiringUnavailable
		}
		return credential, false, nil
	case domainsandbox.ProviderTypeLocalDebug:
		if !debug {
			return nil, false, errAppDevProviderWiringUnavailable
		}
		credential := []byte(getenv(appDevProviderAuthenticationTokenEnv))
		if len(credential) < appDevProviderMinimumCredentialBytes {
			wipeSandboxWiringBytes(credential)
			return nil, false, errAppDevProviderWiringUnavailable
		}
		return credential, true, nil
	default:
		return nil, false, errAppDevProviderWiringUnavailable
	}
}

type appDevConfiguredPreviewProjector struct {
	base  url.URL
	debug bool
}

func newAppDevConfiguredPreviewProjector(raw string, debug bool) (*appDevConfiguredPreviewProjector, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return nil, errAppDevProviderWiringUnavailable
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errAppDevProviderWiringUnavailable
	}
	if debug {
		if parsed.Scheme != "http" || !appDevLoopbackHost(parsed.Hostname()) {
			return nil, errAppDevProviderWiringUnavailable
		}
	} else if parsed.Scheme != "https" {
		return nil, errAppDevProviderWiringUnavailable
	}
	return &appDevConfiguredPreviewProjector{base: *parsed, debug: debug}, nil
}

func (projector *appDevConfiguredPreviewProjector) ProjectAppDevPreviewURL(
	_ context.Context,
	spaceID string,
	projectID string,
	relativeRoute string,
) (string, error) {
	if projector == nil || !domainappdev.ValidProviderExecutionSpaceID(spaceID) ||
		!domainappdev.ValidProviderExecutionProjectID(projectID) ||
		!validAppDevWiringPreviewRoute(relativeRoute) {
		return "", errAppDevProviderWiringUnavailable
	}
	projected := projector.base
	projected.Path = strings.TrimSuffix(projected.Path, "/") + relativeRoute
	projected.RawPath, projected.RawQuery, projected.Fragment = "", "", ""
	return projected.String(), nil
}

func validAppDevWiringPreviewRoute(value string) bool {
	return value != "" && len(value) <= 512 && strings.HasPrefix(value, "/") &&
		!strings.HasPrefix(value, "//") && !strings.ContainsAny(value, "?#\\\x00\r\n\t") &&
		!strings.Contains(value, "..")
}

type appDevArtifactEndpointBuilder struct {
	base      url.URL
	urlPolicy appdevapp.ArtifactCapabilityURLPolicy
}

func newAppDevArtifactEndpointBuilder(
	raw string,
	policy appdevapp.ArtifactCapabilityURLPolicy,
) (*appDevArtifactEndpointBuilder, error) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return nil, errAppDevProviderWiringUnavailable
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errAppDevProviderWiringUnavailable
	}
	if policy == nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	probe := *parsed
	probe.Path = strings.TrimSuffix(probe.Path, "/") + "/internal/provider/app-dev/artifacts/probe"
	probe.RawPath = ""
	if policy.Validate(probe.String()) != nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	return &appDevArtifactEndpointBuilder{base: *parsed, urlPolicy: policy}, nil
}

func (builder *appDevArtifactEndpointBuilder) ArtifactDownloadEndpoint(_ context.Context, id domainappdev.ArtifactGrantID) (string, error) {
	return builder.endpoint(id)
}

func (builder *appDevArtifactEndpointBuilder) ArtifactUploadEndpoint(_ context.Context, id domainappdev.ArtifactGrantID) (string, error) {
	return builder.endpoint(id)
}

func (builder *appDevArtifactEndpointBuilder) endpoint(id domainappdev.ArtifactGrantID) (string, error) {
	if builder == nil || id.IsZero() {
		return "", errAppDevProviderWiringUnavailable
	}
	projected := builder.base
	projected.Path = strings.TrimSuffix(projected.Path, "/") + "/internal/provider/app-dev/artifacts/" + id.Encoded()
	projected.RawPath = ""
	result := projected.String()
	if builder.urlPolicy.Validate(result) != nil {
		return "", errAppDevProviderWiringUnavailable
	}
	return result, nil
}

func appDevLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type appDevGrantAudienceResolver interface {
	ResolveArtifactGrantAudience(context.Context, domainappdev.ArtifactGrantID) (domainappdev.ArtifactGrantAudience, error)
}

type appDevProviderAuthenticator struct {
	providerKey    string
	credentialHash [sha256.Size]byte
	resolver       appDevGrantAudienceResolver
}

func newAppDevProviderAuthenticator(providerKey string, credential []byte, resolver appDevGrantAudienceResolver) (*appDevProviderAuthenticator, error) {
	defer wipeSandboxWiringBytes(credential)
	if domainsandbox.ValidateProviderKey(providerKey) != nil || len(credential) < appDevProviderMinimumCredentialBytes || resolver == nil {
		return nil, errAppDevProviderWiringUnavailable
	}
	return &appDevProviderAuthenticator{providerKey: providerKey, credentialHash: sha256.Sum256(credential), resolver: resolver}, nil
}

func (authenticator *appDevProviderAuthenticator) AuthenticateArtifactProvider(
	ctx context.Context,
	rawGrantID string,
	authorization string,
) (domainappdev.ArtifactGrantAudience, error) {
	if authenticator == nil || ctx == nil || ctx.Err() != nil {
		return domainappdev.ArtifactGrantAudience{}, errAppDevProviderWiringUnavailable
	}
	if !strings.HasPrefix(authorization, appDevProviderAuthorizationPrefix) {
		return domainappdev.ArtifactGrantAudience{}, errAppDevProviderWiringUnavailable
	}
	value := []byte(strings.TrimPrefix(authorization, appDevProviderAuthorizationPrefix))
	if len(value) < appDevProviderMinimumCredentialBytes {
		return domainappdev.ArtifactGrantAudience{}, errAppDevProviderWiringUnavailable
	}
	hash := sha256.Sum256(value)
	if subtle.ConstantTimeCompare(hash[:], authenticator.credentialHash[:]) != 1 {
		return domainappdev.ArtifactGrantAudience{}, errAppDevProviderWiringUnavailable
	}
	grantID, err := domainappdev.ParseArtifactGrantID(rawGrantID)
	if err != nil {
		return domainappdev.ArtifactGrantAudience{}, errAppDevProviderWiringUnavailable
	}
	audience, err := authenticator.resolver.ResolveArtifactGrantAudience(ctx, grantID)
	if err != nil || audience.ProviderKey != authenticator.providerKey ||
		domainappdev.ValidateArtifactGrantAudience(audience) != nil {
		return domainappdev.ArtifactGrantAudience{}, errAppDevProviderWiringUnavailable
	}
	return audience, nil
}

type appDevSecureTransportVerifier struct {
	trustedProxies []*net.IPNet
	debugLoopback  bool
}

func (verifier *appDevSecureTransportVerifier) VerifyArtifactProviderTransport(
	ctx context.Context,
	input appdevapp.ProviderSecureTransportInput,
) error {
	if verifier == nil || ctx == nil || ctx.Err() != nil {
		return errAppDevProviderWiringUnavailable
	}
	if input.DirectHTTPS {
		return nil
	}
	if verifier.debugLoopback && appDevLoopbackHost(remoteHost(input.RemoteAddress)) {
		return nil
	}
	if input.ForwardedProto != "https" || input.RemoteAddress == "" {
		return errAppDevProviderWiringUnavailable
	}
	ip := net.ParseIP(remoteHost(input.RemoteAddress))
	for _, network := range verifier.trustedProxies {
		if network.Contains(ip) {
			return nil
		}
	}
	return errAppDevProviderWiringUnavailable
}

func remoteHost(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}

func parseAppDevTrustedProxyCIDRs(raw string) ([]*net.IPNet, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	result := make([]*net.IPNet, 0, len(parts))
	for _, part := range parts {
		if part == "" || part != strings.TrimSpace(part) {
			return nil, errAppDevProviderWiringUnavailable
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			return nil, errAppDevProviderWiringUnavailable
		}
		result = append(result, network)
	}
	return result, nil
}

type infraAppDevRecoveryProject = infraappdev.ProviderExecutionRecoveryProject
type infraAppDevRecoveryCursor = infraappdev.ProviderExecutionRecoveryCursor

type appDevRecoveryProjectSource interface {
	ListProviderExecutionRecoveryProjects(
		context.Context,
		*infraAppDevRecoveryCursor,
		int,
	) ([]infraAppDevRecoveryProject, *infraAppDevRecoveryCursor, error)
}

type appDevRuntimeRecoverer interface {
	RecoverProjectResumeOnly(
		context.Context,
		appdevapp.ProviderRuntimeRecoverProjectInput,
	) (*appdevapp.ProviderRuntimeProjection, error)
}

type appDevBuildRecoverer interface {
	RecoverBuild(context.Context, appdevapp.ProviderBuildRecoverInput) (*appdevapp.ProviderBuildProjection, error)
}

type appDevRecoveryOwnerReleaser interface {
	ReleaseRecoveryOwner(context.Context, appdevapp.ProviderRuntimeRecoverProjectInput) error
}

type appDevSnapshotCleanup interface {
	CleanupDeferredProviderSnapshotRestoreObjects(context.Context, int) (int, error)
}

type appDevSnapshotCleanupPager interface {
	CleanupDeferredProviderSnapshotRestoreObjectsPage(
		context.Context,
		*infraappdev.ProviderSnapshotCleanupCursor,
		int,
	) (int, *infraappdev.ProviderSnapshotCleanupCursor, error)
}

type appDevRecoverySchedulerConfig struct {
	Interval             time.Duration
	ErrorBackoff         time.Duration
	AttemptTimeout       time.Duration
	ProjectTimeout       time.Duration
	ClaimedLeaseDuration time.Duration
	BatchSize            int
	Now                  func() time.Time
}

type appDevRecoverySchedulerState uint8

const (
	appDevRecoverySchedulerStopped appDevRecoverySchedulerState = iota
	appDevRecoverySchedulerRunning
	appDevRecoverySchedulerStopping
)

type appDevRecoveryScheduler struct {
	source   appDevRecoveryProjectSource
	runtime  appDevRuntimeRecoverer
	build    appDevBuildRecoverer
	releaser appDevRecoveryOwnerReleaser
	cleanup  appDevSnapshotCleanup
	config   appDevRecoverySchedulerConfig

	shutdownMu sync.Mutex
	mu         sync.Mutex
	state      appDevRecoverySchedulerState
	cancel     context.CancelFunc
	done       chan struct{}
	claimed    map[string]infraAppDevRecoveryProject
	claimUntil map[string]time.Time

	runtimeCursor *infraAppDevRecoveryCursor
	buildCursor   *infraAppDevRecoveryCursor
	cleanupCursor *infraappdev.ProviderSnapshotCleanupCursor
}

func newAppDevRecoveryScheduler(
	source appDevRecoveryProjectSource,
	runtime appDevRuntimeRecoverer,
	build appDevBuildRecoverer,
	releaser appDevRecoveryOwnerReleaser,
	cleanup appDevSnapshotCleanup,
	config appDevRecoverySchedulerConfig,
) (*appDevRecoveryScheduler, error) {
	if source == nil || runtime == nil || build == nil || releaser == nil || cleanup == nil ||
		config.Interval <= 0 || config.ErrorBackoff <= 0 || config.AttemptTimeout <= 0 || config.BatchSize <= 0 {
		return nil, errAppDevProviderWiringUnavailable
	}
	if config.ProjectTimeout <= 0 {
		config.ProjectTimeout = config.AttemptTimeout / 3
		if config.ProjectTimeout <= 0 {
			return nil, errAppDevProviderWiringUnavailable
		}
	}
	if config.ProjectTimeout >= config.AttemptTimeout {
		return nil, errAppDevProviderWiringUnavailable
	}
	if config.ClaimedLeaseDuration <= 0 {
		config.ClaimedLeaseDuration = appDevProviderLeaseDuration
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &appDevRecoveryScheduler{
		source: source, runtime: runtime, build: build, releaser: releaser, cleanup: cleanup,
		config: config, claimed: make(map[string]infraAppDevRecoveryProject),
		claimUntil: make(map[string]time.Time),
	}, nil
}

func (scheduler *appDevRecoveryScheduler) Start(parent context.Context) {
	if scheduler == nil {
		return
	}
	scheduler.mu.Lock()
	if scheduler.state != appDevRecoverySchedulerStopped || scheduler.done != nil {
		scheduler.mu.Unlock()
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	scheduler.state = appDevRecoverySchedulerRunning
	scheduler.cancel = cancel
	scheduler.done = make(chan struct{})
	done := scheduler.done
	scheduler.mu.Unlock()
	go scheduler.run(ctx, done)
}

func (scheduler *appDevRecoveryScheduler) run(ctx context.Context, done chan struct{}) {
	defer func() {
		close(done)
		scheduler.mu.Lock()
		if scheduler.done == done && scheduler.state == appDevRecoverySchedulerRunning {
			scheduler.state = appDevRecoverySchedulerStopped
			scheduler.cancel = nil
			scheduler.done = nil
		}
		scheduler.mu.Unlock()
	}()
	delay := time.Duration(0)
	for {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return
		}
		if scheduler.reconcile(ctx) {
			delay = scheduler.config.Interval
		} else {
			delay = scheduler.config.ErrorBackoff
		}
	}
}

func (scheduler *appDevRecoveryScheduler) reconcile(ctx context.Context) bool {
	cleanupCtx, cleanupCancel := context.WithTimeout(ctx, scheduler.config.AttemptTimeout)
	ok := scheduler.reconcileCleanup(cleanupCtx)
	cleanupCancel()

	runtimeCtx, runtimeCancel := context.WithTimeout(ctx, scheduler.config.AttemptTimeout)
	if !scheduler.reconcileRuntimePage(runtimeCtx) {
		ok = false
	}
	runtimeCancel()

	buildCtx, buildCancel := context.WithTimeout(ctx, scheduler.config.AttemptTimeout)
	if !scheduler.reconcileBuildPage(buildCtx) {
		ok = false
	}
	buildCancel()
	return ok
}

func (scheduler *appDevRecoveryScheduler) reconcileCleanup(ctx context.Context) bool {
	cleanupCtx, cancel := context.WithTimeout(ctx, scheduler.config.ProjectTimeout)
	defer cancel()
	if pager, ok := scheduler.cleanup.(appDevSnapshotCleanupPager); ok {
		scheduler.mu.Lock()
		cursor := cloneAppDevSnapshotCleanupCursor(scheduler.cleanupCursor)
		scheduler.mu.Unlock()
		_, next, err := pager.CleanupDeferredProviderSnapshotRestoreObjectsPage(
			cleanupCtx,
			cursor,
			scheduler.config.BatchSize,
		)
		if err != nil {
			return false
		}
		scheduler.mu.Lock()
		scheduler.cleanupCursor = cloneAppDevSnapshotCleanupCursor(next)
		scheduler.mu.Unlock()
		return true
	}
	_, err := scheduler.cleanup.CleanupDeferredProviderSnapshotRestoreObjects(
		cleanupCtx,
		scheduler.config.BatchSize,
	)
	return err == nil
}

func (scheduler *appDevRecoveryScheduler) reconcileRuntimePage(ctx context.Context) bool {
	scheduler.mu.Lock()
	cursor := cloneAppDevRecoveryCursor(scheduler.runtimeCursor)
	scheduler.mu.Unlock()
	projects, next, err := scheduler.source.ListProviderExecutionRecoveryProjects(
		ctx,
		cursor,
		scheduler.config.BatchSize,
	)
	if err != nil {
		return false
	}
	ok := true
	processed := 0
	for _, project := range projects {
		if ctx.Err() != nil {
			ok = false
			break
		}
		projectCtx, cancel := context.WithTimeout(ctx, scheduler.config.ProjectTimeout)
		projection, recoverErr := scheduler.runtime.RecoverProjectResumeOnly(
			projectCtx,
			appdevapp.ProviderRuntimeRecoverProjectInput{
				SpaceID: project.SpaceID, ProjectID: project.ProjectID,
			},
		)
		cancel()
		processed++
		scheduler.setRuntimeCursor(project)
		scheduler.commitRuntimeRecovery(project, projection, recoverErr)
		if recoverErr != nil && !appDevRecoveryConflict(recoverErr) {
			ok = false
		}
	}
	if processed == len(projects) {
		scheduler.mu.Lock()
		if next == nil {
			scheduler.runtimeCursor = nil
		} else {
			scheduler.runtimeCursor = cloneAppDevRecoveryCursor(next)
		}
		scheduler.mu.Unlock()
	}
	return ok
}

func (scheduler *appDevRecoveryScheduler) reconcileBuildPage(ctx context.Context) bool {
	projects, next := scheduler.claimedBuildPage()
	if len(projects) == 0 {
		scheduler.mu.Lock()
		scheduler.buildCursor = nil
		scheduler.mu.Unlock()
		return true
	}
	ok := true
	processed := 0
	for _, project := range projects {
		if ctx.Err() != nil {
			ok = false
			break
		}
		processed++
		scheduler.mu.Lock()
		scheduler.buildCursor = &infraAppDevRecoveryCursor{
			SpaceID: project.SpaceID, ProjectID: project.ProjectID,
		}
		scheduler.mu.Unlock()
		if !scheduler.hasCurrentRecoveryClaim(project) {
			continue
		}
		projectCtx, cancel := context.WithTimeout(ctx, scheduler.config.ProjectTimeout)
		_, recoverErr := scheduler.build.RecoverBuild(projectCtx, appdevapp.ProviderBuildRecoverInput{
			SpaceID: project.SpaceID, ProjectID: project.ProjectID,
		})
		cancel()
		if recoverErr != nil && !appDevRecoveryConflict(recoverErr) &&
			!errors.Is(recoverErr, domainappdev.ErrProviderExecutionNotFound) {
			ok = false
		}
	}
	if processed == len(projects) {
		scheduler.mu.Lock()
		if next == nil {
			scheduler.buildCursor = nil
		} else {
			scheduler.buildCursor = cloneAppDevRecoveryCursor(next)
		}
		scheduler.mu.Unlock()
	}
	return ok
}

func (scheduler *appDevRecoveryScheduler) claimedBuildPage() ([]infraAppDevRecoveryProject, *infraAppDevRecoveryCursor) {
	now := scheduler.config.Now().UTC()
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	for key, expiresAt := range scheduler.claimUntil {
		if !expiresAt.After(now) {
			delete(scheduler.claimed, key)
			delete(scheduler.claimUntil, key)
		}
	}
	projects := make([]infraAppDevRecoveryProject, 0, len(scheduler.claimed))
	for _, project := range scheduler.claimed {
		projects = append(projects, project)
	}
	sort.Slice(projects, func(left, right int) bool {
		if projects[left].SpaceID != projects[right].SpaceID {
			return projects[left].SpaceID < projects[right].SpaceID
		}
		return projects[left].ProjectID < projects[right].ProjectID
	})
	start := 0
	if scheduler.buildCursor != nil {
		for start < len(projects) &&
			(projects[start].SpaceID < scheduler.buildCursor.SpaceID ||
				(projects[start].SpaceID == scheduler.buildCursor.SpaceID &&
					projects[start].ProjectID <= scheduler.buildCursor.ProjectID)) {
			start++
		}
	}
	if start >= len(projects) {
		return nil, nil
	}
	end := start + scheduler.config.BatchSize
	if end > len(projects) {
		end = len(projects)
	}
	page := append([]infraAppDevRecoveryProject(nil), projects[start:end]...)
	last := page[len(page)-1]
	return page, &infraAppDevRecoveryCursor{SpaceID: last.SpaceID, ProjectID: last.ProjectID}
}

func (scheduler *appDevRecoveryScheduler) hasCurrentRecoveryClaim(
	project infraAppDevRecoveryProject,
) bool {
	key := project.SpaceID + "\x00" + project.ProjectID
	now := scheduler.config.Now().UTC()
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	claimed, ok := scheduler.claimed[key]
	if !ok ||
		claimed.SpaceID != project.SpaceID ||
		claimed.ProjectID != project.ProjectID ||
		!scheduler.claimUntil[key].After(now) {
		delete(scheduler.claimed, key)
		delete(scheduler.claimUntil, key)
		return false
	}
	return true
}

func (scheduler *appDevRecoveryScheduler) setRuntimeCursor(project infraAppDevRecoveryProject) {
	scheduler.mu.Lock()
	scheduler.runtimeCursor = &infraAppDevRecoveryCursor{
		SpaceID: project.SpaceID, ProjectID: project.ProjectID,
	}
	scheduler.mu.Unlock()
}

func (scheduler *appDevRecoveryScheduler) commitRuntimeRecovery(
	project infraAppDevRecoveryProject,
	projection *appdevapp.ProviderRuntimeProjection,
	err error,
) {
	key := project.SpaceID + "\x00" + project.ProjectID
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if appdevapp.IsProviderRuntimeRecoveryOwnerRetained(err) ||
		(projection != nil &&
			projection.RecoveryOwnership() == appdevapp.ProviderRuntimeRecoveryOwnershipRetained) {
		scheduler.claimed[key] = project
		scheduler.claimUntil[key] = scheduler.config.Now().UTC().Add(scheduler.config.ClaimedLeaseDuration)
		return
	}
	if projection != nil &&
		projection.RecoveryOwnership() == appdevapp.ProviderRuntimeRecoveryOwnershipReleased {
		delete(scheduler.claimed, key)
		delete(scheduler.claimUntil, key)
		return
	}
	if err != nil || projection == nil {
		return
	}
	if projection.CanStart ||
		projection.State == appdevapp.ProviderRuntimeStateStopped ||
		projection.State == appdevapp.ProviderRuntimeStateCleanupComplete {
		delete(scheduler.claimed, key)
		delete(scheduler.claimUntil, key)
		return
	}
	scheduler.claimed[key] = project
	scheduler.claimUntil[key] = scheduler.config.Now().UTC().Add(scheduler.config.ClaimedLeaseDuration)
}

func appDevRecoveryConflict(err error) bool {
	return errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) ||
		errors.Is(err, domainappdev.ErrProviderExecutionConflict) ||
		errors.Is(err, domainappdev.ErrProviderExecutionVersionConflict) ||
		errors.Is(err, appdevapp.ErrProviderRuntimeConflict)
}

func cloneAppDevRecoveryCursor(cursor *infraAppDevRecoveryCursor) *infraAppDevRecoveryCursor {
	if cursor == nil {
		return nil
	}
	copy := *cursor
	return &copy
}

func cloneAppDevSnapshotCleanupCursor(
	cursor *infraappdev.ProviderSnapshotCleanupCursor,
) *infraappdev.ProviderSnapshotCleanupCursor {
	if cursor == nil {
		return nil
	}
	copy := *cursor
	return &copy
}

func (scheduler *appDevRecoveryScheduler) Shutdown(ctx context.Context) error {
	if scheduler == nil || ctx == nil {
		return errAppDevProviderWiringUnavailable
	}
	scheduler.shutdownMu.Lock()
	defer scheduler.shutdownMu.Unlock()
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, scheduler.config.AttemptTimeout)
	defer shutdownCancel()
	scheduler.mu.Lock()
	if scheduler.state == appDevRecoverySchedulerRunning {
		scheduler.state = appDevRecoverySchedulerStopping
	}
	cancel, done := scheduler.cancel, scheduler.done
	scheduler.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
	for {
		scheduler.mu.Lock()
		now := scheduler.config.Now().UTC()
		for key, expiresAt := range scheduler.claimUntil {
			if !expiresAt.IsZero() && !expiresAt.After(now) {
				delete(scheduler.claimed, key)
				delete(scheduler.claimUntil, key)
			}
		}
		projects := make([]infraAppDevRecoveryProject, 0, len(scheduler.claimed))
		for _, project := range scheduler.claimed {
			projects = append(projects, project)
		}
		scheduler.mu.Unlock()
		if len(projects) == 0 {
			scheduler.finishShutdown(done)
			return nil
		}
		sort.Slice(projects, func(left, right int) bool {
			if projects[left].SpaceID != projects[right].SpaceID {
				return projects[left].SpaceID < projects[right].SpaceID
			}
			return projects[left].ProjectID < projects[right].ProjectID
		})
		var releaseErrors []error
		for _, project := range projects {
			key := project.SpaceID + "\x00" + project.ProjectID
			err := scheduler.releaser.ReleaseRecoveryOwner(shutdownCtx, appdevapp.ProviderRuntimeRecoverProjectInput{
				SpaceID: project.SpaceID, ProjectID: project.ProjectID,
			})
			if err == nil {
				scheduler.mu.Lock()
				delete(scheduler.claimed, key)
				delete(scheduler.claimUntil, key)
				scheduler.mu.Unlock()
				continue
			}
			releaseErrors = append(releaseErrors, fmt.Errorf(
				"release recovery owner for scoped project: %w",
				errAppDevProviderWiringUnavailable,
			))
		}
		if len(releaseErrors) == 0 {
			scheduler.finishShutdown(done)
			return nil
		}
		timer := time.NewTimer(scheduler.config.ErrorBackoff)
		select {
		case <-shutdownCtx.Done():
			timer.Stop()
			return errors.Join(append([]error{shutdownCtx.Err()}, releaseErrors...)...)
		case <-timer.C:
		}
	}
}

func (scheduler *appDevRecoveryScheduler) finishShutdown(done chan struct{}) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.done == done {
		scheduler.cancel = nil
		scheduler.done = nil
	}
	scheduler.state = appDevRecoverySchedulerStopped
}

func (runtime *appDevProviderRuntime) Shutdown(ctx context.Context) error {
	if runtime == nil || ctx == nil {
		return errAppDevProviderWiringUnavailable
	}
	runtime.shutdownMu.Lock()
	defer runtime.shutdownMu.Unlock()
	var shutdownErrors []error
	if !runtime.schedulerStopped && runtime.scheduler != nil {
		if err := runtime.scheduler.Shutdown(ctx); err != nil {
			return err
		}
		runtime.schedulerStopped = true
	} else if runtime.scheduler == nil {
		runtime.schedulerStopped = true
	}
	if !runtime.quarantineUnpublished {
		if runtime.quarantine == nil ||
			appdevapp.UnpublishProviderQuarantineDispositionControl(runtime.quarantineOwner) ||
			!runtime.quarantineOwner.IsCurrent() {
			runtime.quarantineUnpublished = true
		} else {
			shutdownErrors = append(shutdownErrors, errAppDevProviderWiringUnavailable)
		}
	}
	if !runtime.httpUnpublished {
		if runtime.gatewayHandler == nil ||
			appdevapp.UnpublishProviderHTTPDependencies(runtime.httpOwner) ||
			!runtime.httpOwner.IsCurrent() {
			runtime.httpUnpublished = true
		} else {
			shutdownErrors = append(shutdownErrors, errAppDevProviderWiringUnavailable)
		}
	}
	if !runtime.serviceUnpublished {
		if runtime.service == nil ||
			runtime.service.UnpublishProviderRuntimeBindings(runtime.serviceOwner) ||
			!runtime.service.IsProviderRuntimePublicationCurrent(runtime.serviceOwner) {
			runtime.serviceUnpublished = true
		} else {
			shutdownErrors = append(shutdownErrors, errAppDevProviderWiringUnavailable)
		}
	}
	if len(shutdownErrors) > 0 {
		return errors.Join(shutdownErrors...)
	}
	return nil
}

func (runtime *appDevProviderRuntime) String() string {
	return fmt.Sprintf("appDevProviderRuntime{ready:%t}", runtime != nil && runtime.facade != nil)
}
