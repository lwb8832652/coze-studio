// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
	mysqlimpl "github.com/coze-dev/coze-studio/backend/infra/orm/impl/mysql"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const defaultRunnerDispatchInterval = 100 * time.Millisecond
const defaultAIOObservationInterval = 5 * time.Second
const defaultAIOObservationTimeout = 5 * time.Second

// RuntimeStore is the single trusted encrypted execution store shared by HTTP
// status handlers, scheduling, recovery, and terminal result persistence.
type RuntimeStore interface {
	ExecutionStore
	ResultExecutionStore
	Store
	LifecycleManager
}

type RuntimeSessionStore interface {
	SessionOperationStore
	SessionLeaseManager
	sandboxidentity.SessionNonceStore
}

type RuntimeSessionRepository interface {
	domainsandbox.RuntimeSessionRepository
	domainsandbox.AIOGenerationRepository
	domainsandbox.SessionSettingsRepository
}

type RuntimeAIOUpstream interface {
	aio.LifecycleUpstream
	aio.SessionUpstreamClient
	aio.ShellLockIdentityProvider
}

type runtimeSessionStoreSource interface {
	RuntimeSessionStore() RuntimeSessionStore
}

type runtimeStoreBundle struct {
	RuntimeStore
	session RuntimeSessionStore
}

func (bundle runtimeStoreBundle) RuntimeSessionStore() RuntimeSessionStore { return bundle.session }

type RuntimeDependencies struct {
	Store               RuntimeStore
	Driver              sandboxruntime.Driver
	Resources           ResourceSampler
	InitialSettings     domainsandbox.SchedulerSettings
	ConfigurationSigner *infrasandbox.SchedulerConfigSigner
	CoreLifecycle       CoreLifecycle
	CoreRedisReadiness  cache.ReadinessChecker
	CoreSettings        CoreSettingsGate
	CoreRecovery        CoreOperationRecovery
	Session             http.Handler
	SQLPool             io.Closer
	Now                 func() time.Time
}

type CoreLifecycle interface {
	Observe(context.Context) (aio.LifecycleSnapshot, error)
	Snapshot() aio.LifecycleSnapshot
}

type CoreSettingsGate interface {
	CoreEnabled() bool
}

type CoreOperationRecovery interface {
	Recover(context.Context) ([]SessionOperationRecord, error)
}

type Runtime struct {
	server                *Server
	scheduler             *RunnerScheduler
	lifecycle             *Lifecycle
	configStore           *ConfigurationStore
	coreLifecycle         CoreLifecycle
	coreRedisReadiness    cache.ReadinessChecker
	coreSettings          CoreSettingsGate
	coreRecovery          CoreOperationRecovery
	coreRecoveryReady     atomic.Bool
	sessionBackendEnabled bool
	sqlPool               io.Closer
	closeOnce             sync.Once
	closeErr              error
	coreObservationActive atomic.Bool
	dispatchTick          time.Duration
	aioObservationTick    time.Duration
	aioObservationTimeout time.Duration
}

func NewRuntime(config Config, dependencies RuntimeDependencies) (*Runtime, error) {
	succeeded := false
	defer func() {
		if !succeeded && dependencies.SQLPool != nil {
			_ = dependencies.SQLPool.Close()
		}
	}()
	if dependencies.Store == nil || dependencies.Driver == nil || dependencies.Resources == nil || dependencies.ConfigurationSigner == nil {
		return nil, ErrConfiguration
	}
	if config.SessionBackendEnabled && (dependencies.Session == nil || dependencies.CoreLifecycle == nil || dependencies.CoreRedisReadiness == nil) {
		return nil, ErrConfiguration
	}
	settings, err := domainsandbox.NormalizeSchedulerSettings(dependencies.InitialSettings)
	if err != nil || settings.Version == 0 {
		return nil, ErrConfiguration
	}
	if dependencies.Now == nil {
		dependencies.Now = func() time.Time { return time.Now().UTC() }
	}
	lifecycle, err := NewLifecycle(LifecycleConfig{Driver: dependencies.Driver, Settings: settings, DeploymentID: config.DeploymentID, Now: dependencies.Now})
	if err != nil {
		return nil, ErrConfiguration
	}
	var scheduler *RunnerScheduler
	finisher := resultFinisherFunc(func(ctx context.Context, result infrasandbox.ExecuteResult) error {
		if scheduler == nil {
			return ErrUnavailable
		}
		return scheduler.FinishResult(ctx, result)
	})
	dispatcher, err := NewWorkloadDispatcher(WorkloadDispatcherConfig{Executor: lifecycle, ExecutionImage: config.ExecutionImage, CredentialGeneration: config.ActiveQueueKeyID, Finisher: finisher, Now: dependencies.Now})
	if err != nil {
		return nil, ErrConfiguration
	}
	scheduler, err = NewRunnerScheduler(RunnerSchedulerConfig{Store: dependencies.Store, Dispatcher: dispatcher, Settings: settings, Resources: dependencies.Resources, Now: dependencies.Now})
	if err != nil {
		return nil, ErrConfiguration
	}
	storeSettings, _ := dependencies.Store.(SchedulerSettingsApplier)
	configurationApplier := runtimeSettingsApplier{scheduler: scheduler, lifecycle: lifecycle, store: storeSettings}
	configStore, err := NewConfigurationStoreWithApplier(dependencies.ConfigurationSigner, settings, configurationApplier)
	if err != nil {
		return nil, ErrConfiguration
	}
	lifecycleManager := runtimeLifecycleManager{store: dependencies.Store, scheduler: scheduler, lifecycle: lifecycle}
	runtime := &Runtime{
		scheduler: scheduler, lifecycle: lifecycle, configStore: configStore,
		coreLifecycle: dependencies.CoreLifecycle, coreRedisReadiness: dependencies.CoreRedisReadiness, coreSettings: dependencies.CoreSettings,
		coreRecovery: dependencies.CoreRecovery, sessionBackendEnabled: config.SessionBackendEnabled,
		sqlPool: dependencies.SQLPool, dispatchTick: defaultRunnerDispatchInterval,
		aioObservationTick: defaultAIOObservationInterval, aioObservationTimeout: defaultAIOObservationTimeout,
	}
	runtime.coreRecoveryReady.Store(!config.SessionBackendEnabled)
	coreReadiness := runtimeCoreReadiness{sessionBackendEnabled: config.SessionBackendEnabled, lifecycle: dependencies.CoreLifecycle, redisReadiness: dependencies.CoreRedisReadiness, settings: dependencies.CoreSettings, recoveryReady: &runtime.coreRecoveryReady}
	server, err := NewServer(config, Dependencies{Scheduler: scheduler, Store: dependencies.Store, Lifecycle: lifecycleManager, Configuration: configStore, ConfigurationApplier: configStore, RuntimeStatus: runtimeStatusSource{scheduler: scheduler, lifecycle: lifecycle, configuration: configStore, sessionBackendEnabled: config.SessionBackendEnabled, coreSettings: dependencies.CoreSettings, core: runtimeCoreStatusSource{lifecycle: dependencies.CoreLifecycle, redisReadiness: dependencies.CoreRedisReadiness}, recoveryReady: &runtime.coreRecoveryReady}, Readiness: runtimeReadinessProbe{driver: dependencies.Driver}, Session: dependencies.Session, CoreReadiness: coreReadiness})
	if err != nil {
		return nil, ErrConfiguration
	}
	succeeded = true
	runtime.server = server
	return runtime, nil
}

// NewProcessRuntime wires the only production implementation: Redis for
// encrypted queue state and a dedicated rootless Docker-compatible socket for
// execution containers. It intentionally has no host-execution fallback.
func NewProcessRuntime(ctx context.Context, config Config) (*Runtime, error) {
	return newProcessRuntime(ctx, config, defaultProcessRuntimeFactories())
}

type processRuntimeFactories struct {
	newStore                func(context.Context, Config) (RuntimeStore, cache.ReadinessChecker, error)
	newDriver               func(Config) (sandboxruntime.Driver, error)
	newGenerationRepository func() (domainsandbox.AIOGenerationRepository, io.Closer, error)
	newAIOUpstream          func(Config) (aio.LifecycleUpstream, error)
	newCoreLifecycle        func(aio.LifecycleConfig) (CoreLifecycle, error)
}

func defaultProcessRuntimeFactories() processRuntimeFactories {
	return processRuntimeFactories{
		newStore: func(ctx context.Context, config Config) (RuntimeStore, cache.ReadinessChecker, error) {
			client, err := redisimpl.New(ctx)
			if err != nil {
				return nil, nil, ErrUnavailable
			}
			readiness, ok := client.(cache.ReadinessChecker)
			if !ok {
				return nil, nil, ErrUnavailable
			}
			store, err := NewRedisStore(client, RedisStoreConfig{DeploymentID: config.DeploymentID, ActiveKeyID: config.ActiveQueueKeyID, Keys: config.QueueKeys.Keys,
				MaxQueueDepth: domainsandboxDefaultSettings().GlobalQueueDepth, PerSpaceQueueDepth: domainsandboxDefaultSettings().PerSpaceQueueDepth, PerUserQueueDepth: domainsandboxDefaultSettings().PerUserQueueDepth, RecordTTL: 24 * time.Hour})
			if err != nil {
				return nil, nil, err
			}
			sessionStore, err := NewRedisSessionStore(client, RedisSessionStoreConfig{
				DeploymentID: config.DeploymentID, ActiveKeyID: config.ActiveQueueKeyID, Keys: config.QueueKeys.Keys,
				RecordTTL: 24 * time.Hour, LeaseTTL: 30 * time.Second, MaxQueueDepth: maxSessionOperationQueueDepth,
			})
			if err != nil {
				return nil, nil, err
			}
			return runtimeStoreBundle{RuntimeStore: store, session: sessionStore}, readiness, nil
		},
		newDriver: func(config Config) (sandboxruntime.Driver, error) {
			return sandboxruntime.NewDockerDriver(sandboxruntime.DockerDriverConfig{Endpoint: config.RootlessEndpoint})
		},
		newGenerationRepository: func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			db, err := mysqlimpl.New()
			if err != nil {
				return nil, nil, ErrUnavailable
			}
			return generationRepositoryFromDB(db)
		},
		newAIOUpstream: func(config Config) (aio.LifecycleUpstream, error) {
			return aio.NewUpstreamClient(aio.UpstreamClientConfig{BaseURL: config.AIOUpstreamURL, BearerJWT: config.AIOBearerToken, HTTPClient: NewAIOHTTPClient()})
		},
		newCoreLifecycle: func(config aio.LifecycleConfig) (CoreLifecycle, error) {
			return aio.NewLifecycleSupervisor(config)
		},
	}
}

func generationRepositoryFromDB(db *gorm.DB) (domainsandbox.AIOGenerationRepository, io.Closer, error) {
	if db == nil {
		return nil, nil, ErrUnavailable
	}
	sqlPool, err := db.DB()
	if err != nil || sqlPool == nil {
		if sqlPool != nil {
			_ = sqlPool.Close()
		} else if pool, ok := db.ConnPool.(io.Closer); ok && pool != nil {
			_ = pool.Close()
		}
		return nil, nil, ErrUnavailable
	}
	return infrasandbox.NewMySQLRepository(db), sqlPool, nil
}

func newProcessRuntime(ctx context.Context, config Config, factories processRuntimeFactories) (*Runtime, error) {
	if ctx == nil {
		return nil, ErrConfiguration
	}
	if factories.newStore == nil || factories.newDriver == nil {
		return nil, ErrConfiguration
	}
	store, redisReadiness, err := factories.newStore(ctx, config)
	if err != nil {
		return nil, err
	}
	driver, err := factories.newDriver(config)
	if err != nil {
		return nil, ErrConfiguration
	}
	settings := domainsandboxDefaultSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner(config.SchedulerConfigKeys.ActiveKeyID, stringKeyringBytes(config.SchedulerConfigKeys.Keys), 5*time.Minute)
	if err != nil {
		return nil, ErrConfiguration
	}
	dependencies := RuntimeDependencies{Store: store, Driver: driver, Resources: NewHostMemorySampler(), InitialSettings: settings, ConfigurationSigner: signer, CoreRedisReadiness: redisReadiness}
	lifecycleFactory := factories.newCoreLifecycle
	if lifecycleFactory == nil {
		lifecycleFactory = func(config aio.LifecycleConfig) (CoreLifecycle, error) {
			return aio.NewLifecycleSupervisor(config)
		}
	}
	var sessionStore RuntimeSessionStore
	if source, ok := store.(runtimeSessionStoreSource); ok {
		sessionStore = source.RuntimeSessionStore()
	}
	if config.SessionBackendEnabled && factories.newGenerationRepository != nil && factories.newAIOUpstream != nil {
		repository, ownedPool, repositoryErr := factories.newGenerationRepository()
		sessionRepository, repositoryOK := repository.(RuntimeSessionRepository)
		if repositoryErr == nil && repository != nil && ownedPool != nil {
			upstream, upstreamErr := factories.newAIOUpstream(config)
			sessionUpstream, upstreamOK := upstream.(RuntimeAIOUpstream)
			if upstreamErr == nil && sessionStore != nil && repositoryOK && sessionRepository != nil && upstreamOK && sessionUpstream != nil {
				initialSessionSettings, settingsErr := sessionRepository.GetSessionSettings(ctx)
				supervisor, lifecycleErr := lifecycleFactory(aio.LifecycleConfig{
					Enabled: true, DeploymentID: config.DeploymentID, Repository: repository, Upstream: upstream,
				})
				if settingsErr == nil && lifecycleErr == nil {
					coreScheduler, schedulerErr := NewCoreSessionScheduler(CoreSessionSchedulerConfig{Store: sessionStore, Settings: initialSessionSettings})
					adapterFactory, adapterErr := NewAIOSessionAdapterFactory(sessionUpstream, time.Now, nil)
					ids, idsErr := NewRandomSessionIDSource(nil)
					verifier, verifierErr := NewSessionContextIdentityVerifier(config.ContextVerifyKeys, sessionStore, time.Now)
					if schedulerErr != nil || adapterErr != nil || idsErr != nil || verifierErr != nil {
						if ownedPool != nil {
							_ = ownedPool.Close()
						}
						return nil, ErrConfiguration
					}
					dispatcher, dispatcherErr := NewSessionDispatcher(SessionDispatcherConfig{
						Repository: sessionRepository, Leases: sessionStore, Generation: sessionRepository, AdapterFactory: adapterFactory,
						IDs: ids, Clock: time.Now, SessionTTL: time.Duration(initialSessionSettings.SessionIdleTTLSeconds) * time.Second,
					})
					service, serviceErr := NewSessionService(SessionServiceConfig{
						DeploymentID: config.DeploymentID, Manager: dispatcher, Repository: sessionRepository, Scheduler: coreScheduler,
						Settings: sessionRepository, SettingsApplier: coreScheduler, Clock: time.Now,
					})
					coreReadiness := runtimeCoreReadiness{sessionBackendEnabled: true, lifecycle: supervisor, redisReadiness: redisReadiness, settings: coreScheduler}
					sessionHandler, handlerErr := NewSessionHTTPHandler(SessionHTTPConfig{
						DeploymentID: config.DeploymentID, AuthToken: config.AuthToken, Now: time.Now,
					}, SessionHTTPDependencies{Readiness: coreReadiness, Verifier: verifier, Service: service})
					if dispatcherErr != nil || serviceErr != nil || handlerErr != nil {
						if ownedPool != nil {
							_ = ownedPool.Close()
						}
						return nil, ErrConfiguration
					}
					dependencies.CoreLifecycle = supervisor
					dependencies.CoreSettings = coreScheduler
					dependencies.CoreRecovery = coreScheduler
					dependencies.Session = sessionHandler
					dependencies.SQLPool = ownedPool
					ownedPool = nil // NewRuntime owns the pool on both success and failure.
				}
			}
		}
		if ownedPool != nil {
			_ = ownedPool.Close()
		}
	}
	if config.SessionBackendEnabled && dependencies.Session == nil {
		// Preserve the desired enabled state for operations visibility. Frozen
		// Session routes remain present but fail closed until every Core dependency
		// can be composed; one-shot routes and readiness remain independent.
		dependencies.Session = unavailableSessionHandler{}
		dependencies.CoreLifecycle = unavailableCoreLifecycle{}
		// The persisted setting could not be read, so this is an unknown dependency
		// state rather than a confirmed database-level disable. A nil settings gate
		// keeps Core admission closed while runtime status reports unknown.
		dependencies.CoreSettings = nil
	}
	return NewRuntime(config, dependencies)
}

func (runtime *Runtime) Handler() http.Handler {
	if runtime == nil || runtime.server == nil {
		return http.NotFoundHandler()
	}
	return runtime.server.Handler()
}

func (runtime *Runtime) Run(ctx context.Context, config Config) error {
	if runtime == nil || runtime.server == nil || runtime.scheduler == nil || runtime.lifecycle == nil || ctx == nil {
		return ErrConfiguration
	}
	defer runtime.Close()
	if err := runtime.lifecycle.Recover(ctx); err != nil {
		return err
	}
	if err := runtime.scheduler.Recover(ctx); err != nil {
		return err
	}
	if err := runtime.recoverCoreOperations(ctx); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", config.ListenAddr)
	if err != nil {
		return ErrUnavailable
	}
	defer listener.Close()
	httpServer := &http.Server{Handler: runtime.server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	errorsCh := make(chan error, 1)
	go func() {
		if config.TLSCertFile != "" {
			httpServer.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			errorsCh <- httpServer.ServeTLS(listener, config.TLSCertFile, config.TLSKeyFile)
			return
		}
		errorsCh <- httpServer.Serve(listener)
	}()
	ticker := time.NewTicker(runtime.dispatchTick)
	defer ticker.Stop()
	var aioTicker *time.Ticker
	var aioTick <-chan time.Time
	if runtime.sessionBackendEnabled && runtime.coreLifecycle != nil {
		aioTicker = time.NewTicker(runtime.aioObservationTick)
		aioTick = aioTicker.C
		defer aioTicker.Stop()
		runtime.scheduleCoreObservation(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
			return nil
		case err := <-errorsCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return ErrUnavailable
		case <-ticker.C:
			_, _ = runtime.scheduler.DispatchNext(ctx)
		case <-aioTick:
			runtime.scheduleCoreObservation(ctx)
		}
	}
}

func (runtime *Runtime) scheduleCoreObservation(ctx context.Context) {
	if runtime == nil || runtime.coreLifecycle == nil || ctx == nil || !runtime.coreObservationActive.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer runtime.coreObservationActive.Store(false)
		runtime.observeCore(ctx)
	}()
}

func (runtime *Runtime) CoreReady(ctx context.Context) error {
	if runtime == nil {
		return ErrUnavailable
	}
	return runtimeCoreReadiness{sessionBackendEnabled: runtime.sessionBackendEnabled, lifecycle: runtime.coreLifecycle, redisReadiness: runtime.coreRedisReadiness, settings: runtime.coreSettings, recoveryReady: &runtime.coreRecoveryReady}.CoreReady(ctx)
}

func (runtime *Runtime) recoverCoreOperations(ctx context.Context) error {
	if runtime == nil || ctx == nil {
		return ErrProtocol
	}
	if !runtime.sessionBackendEnabled {
		runtime.coreRecoveryReady.Store(true)
		return nil
	}
	runtime.coreRecoveryReady.Store(false)
	if runtime.coreRecovery == nil {
		return nil
	}
	if _, err := runtime.coreRecovery.Recover(ctx); err != nil {
		// Core uncertainty must not prevent the established one-shot listener
		// from starting. Readiness remains closed until a future process start
		// completes recovery successfully.
		return nil
	}
	runtime.coreRecoveryReady.Store(true)
	return nil
}

func (runtime *Runtime) observeCore(ctx context.Context) {
	if runtime == nil || runtime.coreLifecycle == nil || ctx == nil {
		return
	}
	timeout := runtime.aioObservationTimeout
	if timeout <= 0 {
		timeout = defaultAIOObservationTimeout
	}
	observeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, _ = runtime.coreLifecycle.Observe(observeCtx)
}

func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	runtime.closeOnce.Do(func() {
		if runtime.sqlPool != nil {
			runtime.closeErr = runtime.sqlPool.Close()
		}
	})
	return runtime.closeErr
}

type runtimeCoreStatusSource struct {
	lifecycle      CoreLifecycle
	redisReadiness cache.ReadinessChecker
}

type unavailableSessionHandler struct{}

func (unavailableSessionHandler) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	writePublicError(writer, http.StatusServiceUnavailable, "SANDBOX_UNAVAILABLE")
}

type unavailableCoreLifecycle struct{}

func (unavailableCoreLifecycle) Observe(context.Context) (aio.LifecycleSnapshot, error) {
	return aio.LifecycleSnapshot{}, ErrUnavailable
}
func (unavailableCoreLifecycle) Snapshot() aio.LifecycleSnapshot {
	return aio.LifecycleSnapshot{Enabled: true, State: aio.LifecycleStateUnknown}
}

type runtimeCoreReadiness struct {
	sessionBackendEnabled bool
	lifecycle             CoreLifecycle
	redisReadiness        cache.ReadinessChecker
	settings              CoreSettingsGate
	recoveryReady         *atomic.Bool
}

func (source runtimeCoreReadiness) CoreReady(ctx context.Context) error {
	if ctx == nil || !source.sessionBackendEnabled || source.lifecycle == nil || source.redisReadiness == nil || source.settings == nil || !source.settings.CoreEnabled() ||
		source.recoveryReady != nil && !source.recoveryReady.Load() || source.redisReadiness.CheckReadiness(ctx) != nil {
		return ErrUnavailable
	}
	snapshot := source.lifecycle.Snapshot()
	if !snapshot.Enabled || !snapshot.Ready || snapshot.State != aio.LifecycleStateReady || snapshot.Generation == 0 {
		return ErrUnavailable
	}
	return nil
}

func (source runtimeCoreStatusSource) CoreRuntimeStatus(ctx context.Context) (CoreRuntimeStatusSnapshot, error) {
	if source.lifecycle == nil {
		return CoreRuntimeStatusSnapshot{}, ErrUnavailable
	}
	if source.redisReadiness == nil || source.redisReadiness.CheckReadiness(ctx) != nil {
		return CoreRuntimeStatusSnapshot{}, ErrUnavailable
	}
	snapshot := source.lifecycle.Snapshot()
	if !snapshot.Enabled || !snapshot.Ready || snapshot.State != aio.LifecycleStateReady || snapshot.Generation == 0 {
		return CoreRuntimeStatusSnapshot{State: coreRuntimeUnknown}, nil
	}
	return CoreRuntimeStatusSnapshot{State: coreRuntimeReady, Generation: snapshot.Generation}, nil
}

type resultFinisherFunc func(context.Context, infrasandbox.ExecuteResult) error

func (function resultFinisherFunc) FinishResult(ctx context.Context, result infrasandbox.ExecuteResult) error {
	return function(ctx, result)
}

type runtimeSettingsApplier struct {
	scheduler *RunnerScheduler
	lifecycle *Lifecycle
	store     SchedulerSettingsApplier
}

// runtimeLifecycleManager keeps the public cancellation contract aligned with
// the scheduler and the actual execution container. A Redis-only transition is
// insufficient for running work because it would leave the process alive.
type runtimeLifecycleManager struct {
	store     LifecycleManager
	scheduler *RunnerScheduler
	lifecycle *Lifecycle
}

type runtimeReadinessProbe struct{ driver sandboxruntime.Driver }

func (probe runtimeReadinessProbe) Ready(ctx context.Context) error {
	if probe.driver == nil || ctx == nil {
		return ErrUnavailable
	}
	_, err := probe.driver.List(ctx)
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

func (manager runtimeLifecycleManager) KeepAlive(ctx context.Context, executionID string) error {
	if manager.store == nil {
		return ErrUnavailable
	}
	return manager.store.KeepAlive(ctx, executionID)
}

func (manager runtimeLifecycleManager) Cancel(ctx context.Context, executionID string) error {
	if manager.scheduler == nil || manager.lifecycle == nil {
		return ErrUnavailable
	}
	if err := manager.scheduler.Cancel(ctx, executionID); err == nil {
		return nil
	} else if !errors.Is(err, ErrExecutionConflict) {
		return err
	}
	if err := manager.lifecycle.Cancel(ctx, executionID); err != nil {
		return err
	}
	return manager.scheduler.Finish(ctx, executionID, infrasandbox.ExecutionStatusCanceled)
}

func (applier runtimeSettingsApplier) ApplySchedulerSettings(ctx context.Context, settings domainsandbox.SchedulerSettings) error {
	if applier.scheduler == nil || applier.lifecycle == nil {
		return ErrConfiguration
	}
	if err := applier.scheduler.ApplySchedulerSettings(ctx, settings); err != nil {
		return err
	}
	if err := applier.lifecycle.ApplySchedulerSettings(ctx, settings); err != nil {
		return err
	}
	if applier.store != nil {
		return applier.store.ApplySchedulerSettings(ctx, settings)
	}
	return nil
}

func stringKeyringBytes(values map[string]string) map[string][]byte {
	result := make(map[string][]byte, len(values))
	for id, value := range values {
		result[id] = []byte(value)
	}
	return result
}

func domainsandboxDefaultSettings() domainsandbox.SchedulerSettings {
	return domainsandbox.DefaultSchedulerSettings()
}
