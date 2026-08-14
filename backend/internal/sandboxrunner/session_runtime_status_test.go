// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"testing"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestRedisSessionOperationAggregateReadsOnlyBoundedEncryptedRuntimeMetadata(t *testing.T) {
	store, redisServer := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	queued := sessionOperationInput(store.now(), "status-queued", 41)
	running := sessionOperationInput(store.now(), "status-running", 42)
	for _, input := range []SessionOperationInput{running, queued} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	if _, started, err := store.ClaimQueued(ctx, running.SessionID, running.OperationID, coreSessionTotalWeight, 1); err != nil || !started {
		t.Fatalf("ClaimQueued() started=%t error=%v", started, err)
	}

	aggregate, err := store.SessionOperationAggregate(ctx)
	if err != nil || aggregate != (SessionOperationAggregate{QueueDepth: 1, Running: 1, UsedWeight: 1}) {
		t.Fatalf("SessionOperationAggregate() = %#v, %v", aggregate, err)
	}
	if _, err := redisServer.RPush(store.activeOperationsKey(), "missing-record"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SessionOperationAggregate(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("aggregate with missing encrypted record error = %v", err)
	}
}

func TestSessionRuntimeStatusSourceProjectsRealReadyAggregates(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "status-running", 42)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, coreSessionTotalWeight, 1); err != nil || !started {
		t.Fatalf("ClaimQueued() started=%t error=%v", started, err)
	}
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version, settings.CoreEnabled, settings.HostShellEnabled = 4, true, true
	scheduler, err := NewCoreSessionScheduler(CoreSessionSchedulerConfig{
		Store: store, Settings: settings, Resources: fixedMemorySampler(4096), HostMemoryReserveMB: 1536,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := &sessionAggregateRepositoryFake{aggregate: infrasandbox.RuntimeSessionAggregate{
		ReadySessions: 2, OperationFencedSessions: 1, IdleSessions: 3, BoundShells: 3,
	}}
	source := sessionRuntimeStatusSource{
		deploymentID: "runner-dev-a", lifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
			Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 7,
		}}, scheduler: scheduler, repository: repository,
	}
	status, err := source.SessionRuntimeStatus(ctx, deploymentOnlySessionClaims())
	if err != nil {
		t.Fatalf("SessionRuntimeStatus() error = %v", err)
	}
	want := SessionRuntimeStatusProjection{
		Schema: sessionRuntimeStatusSchemaV1, Available: true, AppliedConfigVersion: 4,
		RuntimeGeneration: 7, CoreEnabled: true, HostShellEnabled: true,
		CoreMemoryReserveState: memoryReserveAvailable,
		RawAIOReady:            true, GenerationState: "ready", Running: 1, UsedWeight: 1,
		TotalWeight: coreSessionTotalWeight, ActiveSessions: 3, IdleSessions: 3,
		ActiveShells: 1, IdleShells: 2,
	}
	if status != want || repository.generation != 7 {
		t.Fatalf("SessionRuntimeStatus() = %#v, want %#v; repository generation=%d", status, want, repository.generation)
	}
}

func TestSessionRuntimeStatusSourceReturnsSafeUnavailableForAIOAndRecoveryUncertainty(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version = 2
	scheduler, err := NewCoreSessionScheduler(CoreSessionSchedulerConfig{
		Store: store, Settings: settings, Resources: fixedMemorySampler(4096), HostMemoryReserveMB: 1536,
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := &sessionAggregateRepositoryFake{aggregate: infrasandbox.RuntimeSessionAggregate{
		ReadySessions: 1, OperationFencedSessions: 1, RecoveringSessions: 1, IdleSessions: 2, BoundShells: 2,
	}}
	source := sessionRuntimeStatusSource{
		deploymentID: "runner-dev-a", lifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
			Enabled: true, State: aio.LifecycleStateUnknown, Generation: 9,
		}}, scheduler: scheduler, repository: repository,
	}
	status, err := source.SessionRuntimeStatus(context.Background(), deploymentOnlySessionClaims())
	if err != nil || status.Available || status.RawAIOReady || status.RuntimeGeneration != 0 || status.GenerationState != "unknown" ||
		status.ReasonCode != sessionRuntimeReasonAIOUnavailable || status.ActiveSessions != 2 || status.IdleShells != 2 {
		t.Fatalf("AIO unavailable SessionRuntimeStatus() = %#v, %v", status, err)
	}

	source.lifecycle = &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 9}}
	status, err = source.SessionRuntimeStatus(context.Background(), deploymentOnlySessionClaims())
	if err != nil || status.Available || !status.RawAIOReady || status.RuntimeGeneration != 9 ||
		status.ReasonCode != sessionRuntimeReasonRecoveryPending || status.ActiveSessions != 2 {
		t.Fatalf("recovery-pending SessionRuntimeStatus() = %#v, %v", status, err)
	}

	repository.aggregate = infrasandbox.RuntimeSessionAggregate{ReadySessions: 1, BoundShells: 0}
	input := sessionOperationInput(store.now(), "inconsistent-running", 42)
	if _, _, err := store.Accept(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(context.Background(), input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(context.Background(), input.SessionID, input.OperationID, coreSessionTotalWeight, 1); err != nil || !started {
		t.Fatalf("ClaimQueued() started=%t error=%v", started, err)
	}
	status, err = source.SessionRuntimeStatus(context.Background(), deploymentOnlySessionClaims())
	if err != nil || status.Available || status.ReasonCode != sessionRuntimeReasonProjectionUnavailable || status.ActiveShells != 0 || status.IdleShells != 0 {
		t.Fatalf("inconsistent SessionRuntimeStatus() = %#v, %v", status, err)
	}
}

func TestSessionRuntimeStatusSourceProjectsCoreMemoryReserveAdmission(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.Version, settings.CoreEnabled = 1, true
	available := 1536
	var sampleErr error
	scheduler, err := NewCoreSessionScheduler(CoreSessionSchedulerConfig{
		Store: store, Settings: settings,
		Resources:           resourceSamplerFunc(func(context.Context) (int, error) { return available, sampleErr }),
		HostMemoryReserveMB: 1536,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := sessionRuntimeStatusSource{
		deploymentID: "runner-dev-a",
		lifecycle: &recordingCoreLifecycle{snapshot: aio.LifecycleSnapshot{
			Enabled: true, Ready: true, State: aio.LifecycleStateReady, Generation: 1,
		}},
		scheduler: scheduler, repository: &sessionAggregateRepositoryFake{},
	}
	for _, test := range []struct {
		name       string
		available  int
		sampleErr  error
		wantState  string
		wantReady  bool
		wantReason string
	}{
		{name: "available", available: 1536, wantState: memoryReserveAvailable, wantReady: true},
		{name: "below watermark", available: 1535, wantState: memoryReserveBelowWatermark, wantReason: "CORE_MEMORY_BELOW_WATERMARK"},
		{name: "unknown", available: 1536, sampleErr: errors.New("memory sample failed"), wantState: memoryReserveUnknown, wantReason: "CORE_MEMORY_RESERVE_UNKNOWN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			available, sampleErr = test.available, test.sampleErr
			status, err := source.SessionRuntimeStatus(context.Background(), deploymentOnlySessionClaims())
			if err != nil || status.Available != test.wantReady || status.ReasonCode != test.wantReason {
				t.Fatalf("SessionRuntimeStatus() = %#v, %v", status, err)
			}
			encoded, err := json.Marshal(status)
			if err != nil {
				t.Fatal(err)
			}
			var projection map[string]any
			if err := json.Unmarshal(encoded, &projection); err != nil {
				t.Fatal(err)
			}
			if got := projection["core_memory_reserve_state"]; got != test.wantState {
				t.Fatalf("core_memory_reserve_state = %#v, want %q", got, test.wantState)
			}
		})
	}
}

type sessionAggregateRepositoryFake struct {
	aggregate  infrasandbox.RuntimeSessionAggregate
	err        error
	generation uint64
}

func (repository *sessionAggregateRepositoryFake) RuntimeSessionAggregate(_ context.Context, _ string, generation uint64) (infrasandbox.RuntimeSessionAggregate, error) {
	repository.generation = generation
	return repository.aggregate, repository.err
}

func deploymentOnlySessionClaims() sandboxidentity.SessionRequest {
	digest := sha256.Sum256(nil)
	return sandboxidentity.SessionRequest{DeploymentID: "runner-dev-a", RequestDigest: digest[:]}
}
