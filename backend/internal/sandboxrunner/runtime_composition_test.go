// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/cache"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	sandboxruntime "github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/runtime"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestNewRuntimeComposesSignedConfigurationAndRunnerProtocol(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newRuntimeStore(clock.now)
	driver := &lifecycleDriverFake{stopped: true}
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	runtime, err := NewRuntime(validRuntimeConfig(), RuntimeDependencies{Store: store, Driver: driver, Resources: fixedMemorySampler(4096), InitialSettings: settings, ConfigurationSigner: signer, Now: clock.now})
	require.NoError(t, err)
	require.NotNil(t, runtime.Handler())
	next := settings
	next.Version = 2
	next.MaxOutstanding = 16
	envelope, err := signer.Sign(next)
	require.NoError(t, err)
	require.NoError(t, runtime.configStore.ApplyConfiguration(context.Background(), envelope))
	require.Equal(t, uint64(2), runtime.scheduler.settings.Version)
	require.Equal(t, uint64(2), runtime.lifecycle.settings.Version)
}

func TestNewRuntimeAppliesSchedulerMemoryReserveToCoreGate(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	gate := &mutableCoreSettingsGate{enabled: true}
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	runtime, err := NewRuntime(config, RuntimeDependencies{
		Store: newRuntimeStore(clock.now), Driver: &lifecycleDriverFake{stopped: true},
		Resources: fixedMemorySampler(4096), InitialSettings: settings,
		ConfigurationSigner: signer, Now: clock.now, CoreRedisReadiness: alwaysReadyCache{},
		CoreLifecycle: &recordingCoreLifecycle{}, Session: http.NotFoundHandler(),
		CoreSettings: gate, CoreRecovery: &recordingCoreOperationRecovery{},
	})
	require.NoError(t, err)
	next := settings
	next.Version = 2
	next.HostMemoryReserveMB = 2048
	envelope, err := signer.Sign(next)
	require.NoError(t, err)
	require.NoError(t, runtime.configStore.ApplyConfiguration(context.Background(), envelope))
	require.Equal(t, 2048, gate.appliedMemoryReserveMB)
}

func TestNewRuntimeRejectsMissingExecutionDependencies(t *testing.T) {
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	_, err = NewRuntime(validRuntimeConfig(), RuntimeDependencies{InitialSettings: settings, ConfigurationSigner: signer})
	require.ErrorIs(t, err, ErrConfiguration)
}

func TestNewRuntimeRequiresCoreLifecycleOnlyWhenSessionBackendEnabled(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	dependencies := RuntimeDependencies{
		Store: newRuntimeStore(clock.now), Driver: &lifecycleDriverFake{stopped: true},
		Resources: fixedMemorySampler(4096), InitialSettings: settings,
		ConfigurationSigner: signer, Now: clock.now, CoreRedisReadiness: alwaysReadyCache{},
	}

	disabled := validRuntimeConfig()
	disabled.SessionBackendEnabled = false
	runtime, err := NewRuntime(disabled, dependencies)
	require.NoError(t, err)
	require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)

	enabled := validRuntimeConfig()
	enabled.SessionBackendEnabled = true
	runtime, err = NewRuntime(enabled, dependencies)
	require.ErrorIs(t, err, ErrConfiguration)

	core := &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
		Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 3,
	}}
	dependencies.CoreLifecycle = core
	_, err = NewRuntime(enabled, dependencies)
	require.ErrorIs(t, err, ErrConfiguration)
	dependencies.Session = http.NotFoundHandler()
	dependencies.CoreSettings = &mutableCoreSettingsGate{enabled: true}
	dependencies.CoreRecovery = &recordingCoreOperationRecovery{}
	runtime, err = NewRuntime(enabled, dependencies)
	require.NoError(t, err)
	require.NoError(t, runtime.recoverCoreOperations(context.Background()))
	require.NoError(t, runtime.CoreReady(context.Background()))
}

func TestRuntimeClosesOwnedSQLPoolOnceIncludingConstructionFailure(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)

	failedCloser := &recordingCloser{}
	_, err = NewRuntime(validRuntimeConfig(), RuntimeDependencies{
		InitialSettings: settings, ConfigurationSigner: signer, SQLPool: failedCloser,
	})
	require.ErrorIs(t, err, ErrConfiguration)
	require.Equal(t, 1, failedCloser.Count())

	closer := &recordingCloser{}
	runtime, err := NewRuntime(validRuntimeConfig(), RuntimeDependencies{
		Store: newRuntimeStore(clock.now), Driver: &lifecycleDriverFake{stopped: true},
		Resources: fixedMemorySampler(4096), InitialSettings: settings,
		ConfigurationSigner: signer, Now: clock.now, SQLPool: closer,
	})
	require.NoError(t, err)
	require.NoError(t, runtime.Close())
	require.NoError(t, runtime.Close())
	require.Equal(t, 1, closer.Count())
}

func TestCoreReadyFailsClosedWhenRedisBecomesUnavailable(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	redisReadiness := &mutableCacheReadiness{}
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	runtime, err := NewRuntime(config, RuntimeDependencies{
		Store: newRuntimeStore(clock.now), Driver: &lifecycleDriverFake{stopped: true}, Resources: fixedMemorySampler(4096),
		InitialSettings: settings, ConfigurationSigner: signer, Now: clock.now, CoreRedisReadiness: redisReadiness,
		CoreLifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 1}},
		Session:       http.NotFoundHandler(), CoreSettings: &mutableCoreSettingsGate{enabled: true}, CoreRecovery: &recordingCoreOperationRecovery{},
	})
	require.NoError(t, err)
	require.NoError(t, runtime.recoverCoreOperations(context.Background()))
	require.NoError(t, runtime.CoreReady(context.Background()))
	redisReadiness.err = errors.New("redis down")
	require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
}

func TestCoreReadyRequiresDatabaseCoreSettingToBeEnabled(t *testing.T) {
	gate := &mutableCoreSettingsGate{}
	readiness := runtimeCoreReadiness{
		sessionBackendEnabled: true,
		lifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
			Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 1,
		}},
		redisReadiness: alwaysReadyCache{},
		settings:       gate,
	}
	require.ErrorIs(t, readiness.CoreReady(context.Background()), ErrUnavailable)
	gate.enabled = true
	require.NoError(t, readiness.CoreReady(context.Background()))
}

func TestProcessRuntimeFactoriesInitializeSessionDependenciesOnlyWhenEnabled(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	repository := &generationRepositoryFake{}
	upstream := &lifecycleUpstreamFake{}
	closer := &recordingCloser{}
	var repositoryCalls, upstreamCalls int
	factories := processRuntimeFactories{
		newStore: func(context.Context, Config) (RuntimeStore, cache.ReadinessChecker, error) {
			return newRuntimeStore(clock.now), alwaysReadyCache{}, nil
		},
		newDriver: func(Config) (sandboxruntime.Driver, error) { return &lifecycleDriverFake{stopped: true}, nil },
		newGenerationRepository: func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			repositoryCalls++
			return repository, closer, nil
		},
		newAIOUpstream: func(Config) (aio.LifecycleUpstream, error) {
			upstreamCalls++
			return upstream, nil
		},
	}

	disabled := validRuntimeConfig()
	disabled.SessionBackendEnabled = false
	runtime, err := newProcessRuntime(context.Background(), disabled, factories)
	require.NoError(t, err)
	require.Equal(t, 0, repositoryCalls)
	require.Equal(t, 0, upstreamCalls)
	require.NoError(t, runtime.Close())
	require.Equal(t, 0, closer.Count())

	enabled := validRuntimeConfig()
	enabled.SessionBackendEnabled = true
	runtime, err = newProcessRuntime(context.Background(), enabled, factories)
	require.NoError(t, err)
	require.Equal(t, 1, repositoryCalls)
	require.Equal(t, 1, upstreamCalls)
	require.NoError(t, runtime.Close())
	require.Equal(t, 1, closer.Count())
}

func TestNewProcessRuntimeWiresOneSessionHandlerFromSharedCoreDependencies(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	runtime, err := newProcessRuntime(context.Background(), config, processRuntimeFactories{
		newStore: func(context.Context, Config) (RuntimeStore, cache.ReadinessChecker, error) {
			return newRuntimeStore(clock.now), alwaysReadyCache{}, nil
		},
		newDriver: func(Config) (sandboxruntime.Driver, error) { return &lifecycleDriverFake{stopped: true}, nil },
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })

	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", nil)
	response := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)

	status, err := runtime.configStore.Configuration(context.Background())
	require.NoError(t, err)
	require.NotZero(t, status.Version)
	require.Equal(t, coreRuntimeUnknown, runtime.server.runtimeStatus.(runtimeStatusSource).coreStatus(context.Background()).State)
}

func TestNewProcessRuntimeSharesHostMemoryAdmissionWithCore(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	sessionStore, _ := newSessionRedisStoreFixture(t)
	sessionSettings := domainsandbox.DefaultSessionRuntimeSettings()
	sessionSettings.Version = 1
	repository := &processRuntimeSessionRepository{
		dispatcherRepository: &dispatcherRepository{events: &dispatcherEventLog{}},
		settings:             sessionSettings,
	}
	closer := &recordingCloser{}
	upstream := &processRuntimeAIOUpstreamFake{lifecycleUpstreamFake: &lifecycleUpstreamFake{}}
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	runtime, err := newProcessRuntime(context.Background(), config, processRuntimeFactories{
		newStore: func(context.Context, Config) (RuntimeStore, cache.ReadinessChecker, error) {
			return runtimeStoreBundle{RuntimeStore: newRuntimeStore(clock.now), session: sessionStore}, alwaysReadyCache{}, nil
		},
		newDriver: func(Config) (sandboxruntime.Driver, error) { return &lifecycleDriverFake{stopped: true}, nil },
		newGenerationRepository: func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			return repository, closer, nil
		},
		newAIOUpstream: func(Config) (aio.LifecycleUpstream, error) { return upstream, nil },
		newCoreLifecycle: func(aio.LifecycleConfig) (CoreLifecycle, error) {
			return &recordingCoreLifecycle{}, nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, runtime.Close()) })
	core, ok := runtime.coreSettings.(*CoreSessionScheduler)
	require.True(t, ok)
	require.Same(t, runtime.scheduler.watermark.Sampler, core.watermark.Sampler)
	require.Equal(t, domainsandbox.DefaultSchedulerSettings().HostMemoryReserveMB, core.watermark.ReserveMB)
}

func TestProcessRuntimeKeepsOneShotAvailableWhenCoreDependenciesFail(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	baseFactories := processRuntimeFactories{
		newStore: func(context.Context, Config) (RuntimeStore, cache.ReadinessChecker, error) {
			return newRuntimeStore(clock.now), alwaysReadyCache{}, nil
		},
		newDriver:      func(Config) (sandboxruntime.Driver, error) { return &lifecycleDriverFake{stopped: true}, nil },
		newAIOUpstream: func(Config) (aio.LifecycleUpstream, error) { return &lifecycleUpstreamFake{}, nil },
	}
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true

	t.Run("MySQL initialization", func(t *testing.T) {
		factories := baseFactories
		factories.newGenerationRepository = func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			return nil, nil, errors.New("secret MySQL failure")
		}
		runtime, err := newProcessRuntime(context.Background(), config, factories)
		require.NoError(t, err)
		require.NotNil(t, runtime.Handler())
		require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
		require.NoError(t, runtime.Close())
	})

	t.Run("MySQL partial initialization", func(t *testing.T) {
		factories := baseFactories
		closer := &recordingCloser{}
		factories.newGenerationRepository = func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			return &generationRepositoryFake{}, closer, errors.New("secret MySQL failure after pool creation")
		}
		runtime, err := newProcessRuntime(context.Background(), config, factories)
		require.NoError(t, err)
		require.NotNil(t, runtime.Handler())
		require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
		require.Equal(t, 1, closer.Count())
		require.NoError(t, runtime.Close())
		require.Equal(t, 1, closer.Count())
	})

	t.Run("invalid MySQL repository", func(t *testing.T) {
		factories := baseFactories
		closer := &recordingCloser{}
		factories.newGenerationRepository = func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			return nil, closer, nil
		}
		runtime, err := newProcessRuntime(context.Background(), config, factories)
		require.NoError(t, err)
		require.NotNil(t, runtime.Handler())
		require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
		require.Equal(t, 1, closer.Count())
		require.NoError(t, runtime.Close())
		require.Equal(t, 1, closer.Count())
	})

	t.Run("upstream initialization", func(t *testing.T) {
		factories := baseFactories
		closer := &recordingCloser{}
		factories.newGenerationRepository = func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			return &generationRepositoryFake{}, closer, nil
		}
		factories.newAIOUpstream = func(Config) (aio.LifecycleUpstream, error) {
			return nil, errors.New("secret upstream failure")
		}
		runtime, err := newProcessRuntime(context.Background(), config, factories)
		require.NoError(t, err)
		require.NotNil(t, runtime.Handler())
		require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
		require.Equal(t, 1, closer.Count())
		require.NoError(t, runtime.Close())
		require.Equal(t, 1, closer.Count())
	})
}

func TestGenerationRepositoryFromDBClosesPoolWhenExtractionFails(t *testing.T) {
	pool := &invalidGORMConnPool{}
	db := &gorm.DB{Config: &gorm.Config{ConnPool: pool}}

	repository, ownedPool, err := generationRepositoryFromDB(db)

	require.ErrorIs(t, err, ErrUnavailable)
	require.Nil(t, repository)
	require.Nil(t, ownedPool)
	require.Equal(t, 1, pool.Count())
}

func TestCoreReadyReadsCachedLifecycleWithoutBlockingOnObservation(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	core := &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 4}}
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	runtime, err := NewRuntime(config, RuntimeDependencies{
		Store: newRuntimeStore(clock.now), Driver: &lifecycleDriverFake{stopped: true}, Resources: fixedMemorySampler(4096),
		InitialSettings: settings, ConfigurationSigner: signer, Now: clock.now, CoreRedisReadiness: alwaysReadyCache{}, CoreLifecycle: core,
		Session: http.NotFoundHandler(), CoreSettings: &mutableCoreSettingsGate{enabled: true}, CoreRecovery: &recordingCoreOperationRecovery{},
	})
	require.NoError(t, err)
	require.NoError(t, runtime.recoverCoreOperations(context.Background()))
	require.NoError(t, runtime.CoreReady(context.Background()))
	require.Equal(t, 0, core.observeCalls)
	core.snapshot = aio.LifecycleSnapshot{Enabled: true, State: aio.LifecycleStateUnknown, Generation: 4}
	require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
	require.Equal(t, 0, core.observeCalls)
}

func TestRuntimeCoreStatusReadsCachedLifecycleWithoutStartingObservation(t *testing.T) {
	core := &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
		Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 4,
	}}
	source := runtimeCoreStatusSource{lifecycle: core, redisReadiness: alwaysReadyCache{}}

	status, err := source.CoreRuntimeStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, CoreRuntimeStatusSnapshot{State: coreRuntimeReady, Generation: 4}, status)
	require.Equal(t, 0, core.observeCalls)
}

func TestNewProcessRuntimeDoesNotSynchronouslyObserveAIO(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	closer := &recordingCloser{}
	block := make(chan struct{})
	core := &recordingCoreLifecycle{observeBlock: block}
	factories := processRuntimeFactories{
		newStore: func(context.Context, Config) (RuntimeStore, cache.ReadinessChecker, error) {
			return newRuntimeStore(clock.now), alwaysReadyCache{}, nil
		},
		newDriver: func(Config) (sandboxruntime.Driver, error) { return &lifecycleDriverFake{stopped: true}, nil },
		newGenerationRepository: func() (domainsandbox.AIOGenerationRepository, io.Closer, error) {
			return &generationRepositoryFake{}, closer, nil
		},
		newAIOUpstream:   func(Config) (aio.LifecycleUpstream, error) { return &lifecycleUpstreamFake{}, nil },
		newCoreLifecycle: func(aio.LifecycleConfig) (CoreLifecycle, error) { return core, nil },
	}
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true

	started := time.Now()
	runtime, err := newProcessRuntime(context.Background(), config, factories)
	require.NoError(t, err)
	require.Less(t, time.Since(started), 100*time.Millisecond)
	require.Equal(t, 0, core.observeCalls)
	require.NoError(t, runtime.Close())
}

func TestCoreObservationIsBoundedAndNeverOverlaps(t *testing.T) {
	block := make(chan struct{})
	core := &recordingCoreLifecycle{observeBlock: block}
	runtime := &Runtime{coreLifecycle: core, aioObservationTimeout: 20 * time.Millisecond}

	runtime.scheduleCoreObservation(context.Background())
	require.Eventually(t, func() bool { return core.ObserveCalls() == 1 }, time.Second, time.Millisecond)
	runtime.scheduleCoreObservation(context.Background())
	time.Sleep(5 * time.Millisecond)
	require.Equal(t, 1, core.ObserveCalls())
	require.Eventually(t, func() bool { return !runtime.coreObservationActive.Load() }, time.Second, time.Millisecond)
}

func TestRuntimeCoreRecoveryFailsClosedWithoutStoppingOneShot(t *testing.T) {
	recovery := &recordingCoreOperationRecovery{err: errors.New("redis recovery unavailable")}
	runtime := &Runtime{sessionBackendEnabled: true, coreRecovery: recovery}

	if err := runtime.recoverCoreOperations(context.Background()); err != nil {
		t.Fatalf("recoverCoreOperations() = %v; Core failure must not stop one-shot startup", err)
	}
	require.Equal(t, 1, recovery.calls)
	require.False(t, runtime.coreRecoveryReady.Load())

	recovery.err = nil
	if err := runtime.recoverCoreOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
	require.Equal(t, 2, recovery.calls)
	require.True(t, runtime.coreRecoveryReady.Load())
}

func TestRuntimeCoreRecoveryIsNoopWhileSessionBackendDisabled(t *testing.T) {
	recovery := &recordingCoreOperationRecovery{}
	runtime := &Runtime{coreRecovery: recovery}
	require.NoError(t, runtime.recoverCoreOperations(context.Background()))
	require.Zero(t, recovery.calls)
	require.True(t, runtime.coreRecoveryReady.Load())
}

func TestRuntimeCoreReadyRemainsClosedUntilStartupRecoverySucceeds(t *testing.T) {
	runtime := &Runtime{
		sessionBackendEnabled: true,
		coreLifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
			Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 3,
		}},
		coreRedisReadiness: alwaysReadyCache{},
		coreSettings:       &mutableCoreSettingsGate{enabled: true},
		coreRecovery:       &recordingCoreOperationRecovery{},
	}
	require.ErrorIs(t, runtime.CoreReady(context.Background()), ErrUnavailable)
	require.NoError(t, runtime.recoverCoreOperations(context.Background()))
	require.NoError(t, runtime.CoreReady(context.Background()))
}

func TestRuntimeSessionRoutesFeaturesAndStatusShareStartupRecoveryGate(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 1
	signer, err := infrasandbox.NewSchedulerConfigSigner("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	require.NoError(t, err)
	config := validRuntimeConfig()
	config.SessionBackendEnabled = true
	handler := &recordingSessionHandler{status: http.StatusNoContent}
	runtime, err := NewRuntime(config, RuntimeDependencies{
		Store: newRuntimeStore(clock.now), Driver: &lifecycleDriverFake{stopped: true}, Resources: fixedMemorySampler(4096),
		InitialSettings: settings, ConfigurationSigner: signer, Now: clock.now, CoreRedisReadiness: alwaysReadyCache{},
		CoreLifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 4}},
		Session:       handler, CoreSettings: &mutableCoreSettingsGate{enabled: true}, CoreRecovery: &recordingCoreOperationRecovery{},
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+config.AuthToken)
	response := httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Zero(t, handler.calls)
	assertRuntimeHealthSessionFeatures(t, runtime.Handler(), false)
	require.Equal(t, coreRuntimeUnknown, runtime.server.runtimeStatus.(runtimeStatusSource).coreStatus(context.Background()).State)

	require.NoError(t, runtime.recoverCoreOperations(context.Background()))
	response = httptest.NewRecorder()
	runtime.Handler().ServeHTTP(response, request.Clone(context.Background()))
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, 1, handler.calls)
	assertRuntimeHealthSessionFeatures(t, runtime.Handler(), true)
	require.Equal(t, coreRuntimeReady, runtime.server.runtimeStatus.(runtimeStatusSource).coreStatus(context.Background()).State)
}

func assertRuntimeHealthSessionFeatures(t *testing.T, handler http.Handler, wantSession bool) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Features []domainsandbox.ProviderFeature `json:"features"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, wantSession, slices.Contains(body.Features, domainsandbox.ProviderFeatureSandboxSessionV1))
}

func TestRuntimeLifecycleManagerCancelsRunningContainerBeforeReleasingSchedulerCapacity(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newRuntimeStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	require.NoError(t, err)
	accepted, err := scheduler.Accept(context.Background(), schedulerCommand(clock.now(), "runtime-cancel", 101, 201, domainsandbox.ScopePlugin))
	require.NoError(t, err)
	dispatched, err := scheduler.DispatchNext(context.Background())
	require.NoError(t, err)
	require.True(t, dispatched)
	lifecycle, driver := newLifecycleFixture(t, clock.now())
	request := lifecycleRequest(domainsandbox.ScopePlugin, accepted.ExecutionID, 101, 201, "project", "session")
	request.Identity.ExecutionID = accepted.ExecutionID
	_, err = lifecycle.Acquire(context.Background(), request)
	require.NoError(t, err)
	manager := runtimeLifecycleManager{store: store, scheduler: scheduler, lifecycle: lifecycle}
	require.NoError(t, manager.Cancel(context.Background(), accepted.ExecutionID))
	require.Equal(t, 1, driver.terminateCalls)
	stored, err := store.Get(context.Background(), accepted.ExecutionID)
	require.NoError(t, err)
	require.Equal(t, infrasandbox.ExecutionStatusCanceled, stored.State)
	require.Zero(t, scheduler.usedWeight)
	require.Empty(t, scheduler.running)
}

func validRuntimeConfig() Config {
	keyring, _ := sandboxidentity.NewKeyring("key-1", map[string][]byte{"key-1": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	return Config{
		DeploymentID: "runner-dev-1", AuthToken: "runner-auth-token-0123456789", ContextVerifyKeys: keyring,
		SchedulerConfigKeys: keyringConfig{ActiveKeyID: "key-1", Keys: map[string]string{"key-1": "0123456789abcdef0123456789abcdef"}},
		ActiveQueueKeyID:    "key-1", ExecutionImage: "registry.example/coze-sandbox-runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
}

type runtimeStore struct{ *schedulerStore }

type recordingCoreOperationRecovery struct {
	err   error
	calls int
}

func (recovery *recordingCoreOperationRecovery) Recover(context.Context) ([]SessionOperationRecord, error) {
	recovery.calls++
	return nil, recovery.err
}

func newRuntimeStore(now func() time.Time) *runtimeStore {
	return &runtimeStore{schedulerStore: newSchedulerStore(now)}
}

func (store *runtimeStore) Status(ctx context.Context, executionID string) (infrasandbox.ExecuteResult, error) {
	record, err := store.Get(ctx, executionID)
	return record.Result, err
}
func (store *runtimeStore) Lookup(context.Context, infrasandbox.ExecutionLookupRequest) (infrasandbox.ExecutionLookupResult, error) {
	return infrasandbox.ExecutionLookupResult{Status: infrasandbox.ExecutionLookupNotFound}, nil
}
func (store *runtimeStore) QueueStatus(context.Context, string) (infrasandbox.QueueStatus, error) {
	return infrasandbox.QueueStatus{}, nil
}
func (store *runtimeStore) KeepAlive(context.Context, string) error { return nil }
func (store *runtimeStore) Cancel(context.Context, string) error    { return nil }

var _ sandboxruntime.Driver = (*lifecycleDriverFake)(nil)

type recordingCoreLifecycle struct {
	mu           sync.Mutex
	snapshot     aio.LifecycleSnapshot
	err          error
	observeBlock <-chan struct{}
	observeCalls int
}

func (lifecycle *recordingCoreLifecycle) Observe(ctx context.Context) (aio.LifecycleSnapshot, error) {
	lifecycle.mu.Lock()
	lifecycle.observeCalls++
	lifecycle.mu.Unlock()
	if lifecycle.observeBlock != nil {
		select {
		case <-lifecycle.observeBlock:
		case <-ctx.Done():
			return lifecycle.snapshot, ctx.Err()
		}
	}
	return lifecycle.snapshot, lifecycle.err
}

func (lifecycle *recordingCoreLifecycle) Snapshot() aio.LifecycleSnapshot {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	return lifecycle.snapshot
}

func (lifecycle *recordingCoreLifecycle) ObserveCalls() int {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	return lifecycle.observeCalls
}

type recordingCloser struct {
	mu    sync.Mutex
	count int
}

type invalidGORMConnPool struct{ recordingCloser }

func (*invalidGORMConnPool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("not used")
}
func (*invalidGORMConnPool) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, errors.New("not used")
}
func (*invalidGORMConnPool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("not used")
}
func (*invalidGORMConnPool) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return nil
}

func (closer *recordingCloser) Close() error {
	closer.mu.Lock()
	defer closer.mu.Unlock()
	closer.count++
	return nil
}

func (closer *recordingCloser) Count() int {
	closer.mu.Lock()
	defer closer.mu.Unlock()
	return closer.count
}

type generationRepositoryFake struct{}

func (*generationRepositoryFake) GetAIOGeneration(context.Context, string) (domainsandbox.AIOGenerationState, error) {
	return domainsandbox.AIOGenerationState{
		DeploymentID: "runner-dev-1", Generation: 1,
		SentinelID: "newx-generation-0123456789abcdef0123456789abcdef",
	}, nil
}

func (*generationRepositoryFake) CompareAndReplaceAIOSentinel(context.Context, domainsandbox.CompareAndReplaceAIOSentinelInput) (domainsandbox.AIOGenerationState, bool, error) {
	return domainsandbox.AIOGenerationState{}, false, errors.New("not used")
}

type processRuntimeSessionRepository struct {
	*dispatcherRepository
	generationRepositoryFake
	settings domainsandbox.SessionRuntimeSettings
}

func (repository *processRuntimeSessionRepository) GetSessionSettings(context.Context) (domainsandbox.SessionRuntimeSettings, error) {
	return repository.settings, nil
}
func (*processRuntimeSessionRepository) UpdateSessionSettingsCAS(context.Context, domainsandbox.UpdateSessionSettingsInput) (domainsandbox.SessionRuntimeSettings, error) {
	return domainsandbox.SessionRuntimeSettings{}, errors.New("not used")
}

type lifecycleUpstreamFake struct{}

func (*lifecycleUpstreamFake) Health(context.Context) error { return nil }
func (*lifecycleUpstreamFake) ListSessions(context.Context) (*sandboxapi.ResponseActiveShellSessionsResult, error) {
	success := true
	return &sandboxapi.ResponseActiveShellSessionsResult{
		Success: &success,
		Data: &sandboxapi.ActiveShellSessionsResult{Sessions: map[string]*sandboxapi.ShellSessionInfo{
			"newx-generation-0123456789abcdef0123456789abcdef": {WorkingDir: "/mnt/user-data"},
		}},
	}, nil
}
func (*lifecycleUpstreamFake) Create(context.Context, *sandboxapi.ShellCreateSessionRequest) (*sandboxapi.ResponseShellCreateSessionResponse, error) {
	return nil, errors.New("not used")
}
func (*lifecycleUpstreamFake) View(context.Context, *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error) {
	return nil, errors.New("not used")
}
func (*lifecycleUpstreamFake) Cleanup(context.Context, string) error { return nil }

type processRuntimeAIOUpstreamFake struct{ *lifecycleUpstreamFake }

func (*processRuntimeAIOUpstreamFake) Exec(context.Context, *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Wait(context.Context, *sandboxapi.ShellWaitRequest) (*sandboxapi.ResponseShellWaitResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Kill(context.Context, *sandboxapi.ShellKillProcessRequest) (*sandboxapi.ResponseShellKillResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Read(context.Context, *sandboxapi.FileReadRequest) (*sandboxapi.ResponseFileReadResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Write(context.Context, *sandboxapi.FileWriteRequest) (*sandboxapi.ResponseFileWriteResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) List(context.Context, *sandboxapi.FileListRequest) (*sandboxapi.ResponseFileListResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Glob(context.Context, *sandboxapi.FileGlobRequest) (*sandboxapi.ResponseFileGlobResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Grep(context.Context, *sandboxapi.FileGrepRequest) (*sandboxapi.ResponseFileGrepResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Replace(context.Context, *sandboxapi.FileReplaceRequest) (*sandboxapi.ResponseFileReplaceResult, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) Download(context.Context, string) (io.Reader, error) {
	return nil, errors.New("not used")
}
func (*processRuntimeAIOUpstreamFake) ShellLockIdentity() string { return "test-aio-upstream" }

type alwaysReadyCache struct{}

func (alwaysReadyCache) CheckReadiness(context.Context) error { return nil }

type mutableCacheReadiness struct{ err error }

func (readiness *mutableCacheReadiness) CheckReadiness(context.Context) error { return readiness.err }

type mutableCoreSettingsGate struct {
	enabled                bool
	appliedMemoryReserveMB int
}

func (gate *mutableCoreSettingsGate) CoreEnabled() bool { return gate.enabled }
func (gate *mutableCoreSettingsGate) ApplySchedulerSettings(_ context.Context, settings domainsandbox.SchedulerSettings) error {
	gate.appliedMemoryReserveMB = settings.HostMemoryReserveMB
	return nil
}
