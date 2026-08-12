// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
)

const defaultRunnerDispatchInterval = 100 * time.Millisecond

// RuntimeStore is the single trusted encrypted execution store shared by HTTP
// status handlers, scheduling, recovery, and terminal result persistence.
type RuntimeStore interface {
	ExecutionStore
	ResultExecutionStore
	Store
	LifecycleManager
}

type RuntimeDependencies struct {
	Store               RuntimeStore
	Driver              sandboxruntime.Driver
	Resources           ResourceSampler
	InitialSettings     domainsandbox.SchedulerSettings
	ConfigurationSigner *infrasandbox.SchedulerConfigSigner
	Now                 func() time.Time
}

type Runtime struct {
	server       *Server
	scheduler    *RunnerScheduler
	lifecycle    *Lifecycle
	configStore  *ConfigurationStore
	dispatchTick time.Duration
}

func NewRuntime(config Config, dependencies RuntimeDependencies) (*Runtime, error) {
	if dependencies.Store == nil || dependencies.Driver == nil || dependencies.Resources == nil || dependencies.ConfigurationSigner == nil {
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
	server, err := NewServer(config, Dependencies{Scheduler: scheduler, Store: dependencies.Store, Lifecycle: lifecycleManager, Configuration: configStore, ConfigurationApplier: configStore, RuntimeStatus: runtimeStatusSource{scheduler: scheduler, lifecycle: lifecycle, configuration: configStore}, Readiness: runtimeReadinessProbe{driver: dependencies.Driver}})
	if err != nil {
		return nil, ErrConfiguration
	}
	return &Runtime{server: server, scheduler: scheduler, lifecycle: lifecycle, configStore: configStore, dispatchTick: defaultRunnerDispatchInterval}, nil
}

// NewProcessRuntime wires the only production implementation: Redis for
// encrypted queue state and a dedicated rootless Docker-compatible socket for
// execution containers. It intentionally has no host-execution fallback.
func NewProcessRuntime(ctx context.Context, config Config) (*Runtime, error) {
	if ctx == nil {
		return nil, ErrConfiguration
	}
	client, err := redisimpl.New(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	store, err := NewRedisStore(client, RedisStoreConfig{DeploymentID: config.DeploymentID, ActiveKeyID: config.ActiveQueueKeyID, Keys: config.QueueKeys.Keys,
		MaxQueueDepth: domainsandboxDefaultSettings().GlobalQueueDepth, PerSpaceQueueDepth: domainsandboxDefaultSettings().PerSpaceQueueDepth, PerUserQueueDepth: domainsandboxDefaultSettings().PerUserQueueDepth, RecordTTL: 24 * time.Hour})
	if err != nil {
		return nil, ErrConfiguration
	}
	driver, err := sandboxruntime.NewDockerDriver(sandboxruntime.DockerDriverConfig{Endpoint: config.RootlessEndpoint})
	if err != nil {
		return nil, ErrConfiguration
	}
	settings := domainsandboxDefaultSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner(config.SchedulerConfigKeys.ActiveKeyID, stringKeyringBytes(config.SchedulerConfigKeys.Keys), 5*time.Minute)
	if err != nil {
		return nil, ErrConfiguration
	}
	return NewRuntime(config, RuntimeDependencies{Store: store, Driver: driver, Resources: NewHostMemorySampler(), InitialSettings: settings, ConfigurationSigner: signer})
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
	if err := runtime.lifecycle.Recover(ctx); err != nil {
		return err
	}
	if err := runtime.scheduler.Recover(ctx); err != nil {
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
		}
	}
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
