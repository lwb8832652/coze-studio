// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type providerExecutionRestartLookup struct{ provider *domainsandbox.Provider }

func (l providerExecutionRestartLookup) GetProviderByKey(ctx context.Context, key string) (*domainsandbox.Provider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if l.provider == nil || key != l.provider.ProviderKey {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *l.provider
	return &copy, nil
}

type providerExecutionRestartFactory struct{ runtime infrasandbox.RuntimeProvider }

func (providerExecutionRestartFactory) ValidateConfig(context.Context, applicationsandbox.ProviderDescriptor) error {
	return nil
}
func (f providerExecutionRestartFactory) Build(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return f.runtime, nil
}

type providerExecutionRestartLimiter struct{}

func (providerExecutionRestartLimiter) Acquire(context.Context, string, string, string, int, time.Duration) (int64, error) {
	return time.Now().Add(10 * time.Minute).UnixMilli(), nil
}
func (providerExecutionRestartLimiter) Renew(context.Context, string, string, string, string, time.Duration) (int64, error) {
	return time.Now().Add(10 * time.Minute).UnixMilli(), nil
}
func (providerExecutionRestartLimiter) Release(context.Context, string, string, string) error {
	return nil
}

type providerExecutionRestartRuntime struct{}

func (providerExecutionRestartRuntime) Health(context.Context) (infrasandbox.HealthResult, error) {
	return infrasandbox.HealthResult{
		ProtocolVersion: infrasandbox.HealthProtocolV1, Status: domainsandbox.HealthStatusHealthy,
		Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAppDev},
	}, nil
}
func (providerExecutionRestartRuntime) Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{ExecutionID: "restart-provider-execution", Status: infrasandbox.ExecutionStatusAccepted}, nil
}
func (r providerExecutionRestartRuntime) Reconcile(ctx context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error) {
	return r.Execute(ctx, request)
}
func (providerExecutionRestartRuntime) LookupExecution(context.Context, infrasandbox.ExecutionLookupRequest) (infrasandbox.ExecutionLookupResult, error) {
	return infrasandbox.ExecutionLookupResult{
		Status: infrasandbox.ExecutionLookupFound,
		Execution: infrasandbox.ExecuteResult{
			ExecutionID: "restart-provider-execution",
			Status:      infrasandbox.ExecutionStatusRunning,
		},
	}, nil
}
func (providerExecutionRestartRuntime) Status(context.Context, string) (infrasandbox.ExecuteResult, error) {
	return infrasandbox.ExecuteResult{ExecutionID: "restart-provider-execution", Status: infrasandbox.ExecutionStatusRunning}, nil
}
func (providerExecutionRestartRuntime) KeepAlive(context.Context, string) error { return nil }
func (providerExecutionRestartRuntime) Cancel(context.Context, string) error    { return nil }
func (providerExecutionRestartRuntime) CloseContext(ctx context.Context) error  { return ctx.Err() }

func TestProviderExecutionRestartClaimsAndOpensRealRouterCheckpoint(t *testing.T) {
	now := time.Date(2026, 7, 16, 19, 0, 0, 123456000, time.UTC)
	checkpoint := providerExecutionRouterCheckpoint(t)
	key := bytes.Repeat([]byte{0x71}, 32)
	codec1 := providerExecutionRealCheckpointCodec(t, key)
	databasePath := filepath.Join(t.TempDir(), "restart-provider-execution.db")
	db1, sqlDB1 := openProviderExecutionSQLiteTestDB(t, databasePath)
	if err := migrateProviderExecutionSQLiteTestSchema(db1); err != nil {
		t.Fatal(err)
	}
	seedProviderExecutionProject(t, db1, 1001, "project-a")
	repository1 := NewProviderExecutionRepository(db1)
	repository1.clock = &providerExecutionControlledDBClock{now: now}
	service1, err := applicationappdev.NewProviderExecutionService(
		repository1, codec1,
		applicationappdev.WithProviderExecutionClock(func() time.Time { return now }),
		applicationappdev.WithProviderExecutionRandom(bytes.NewReader(bytes.Repeat([]byte{0x31}, 128))),
		applicationappdev.WithProviderExecutionOwnerLease(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	started, err := service1.EnsureStart(context.Background(), applicationappdev.EnsureProviderExecutionStartRequest{
		SpaceID: "1001", ProjectID: "project-a", IdempotencyKey: "restart-idempotency",
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service1.ClaimRecovery(context.Background(), applicationappdev.ClaimProviderExecutionRecoveryRequest{
		SpaceID: started.SpaceID, ProjectID: started.ProjectID, Generation: started.Generation, ExpectedVersion: started.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	owner1 := claimed.Owner()
	submission, err := service1.StartProviderSubmission(
		context.Background(),
		providerExecutionServiceStartSubmissionRequest(
			owner1, claimed.Metadata.Version, "restart-launch",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	submission, err = service1.MarkProviderLaunchSubmitted(context.Background(), applicationappdev.MarkProviderExecutionLaunchSubmittedRequest{
		Owner: owner1, ExpectedVersion: submission.Version, OperationID: "restart-launch",
		DispatchLeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := service1.SaveProviderSubmission(context.Background(), applicationappdev.SaveProviderExecutionSubmissionRequest{
		Owner: owner1, ExpectedVersion: submission.Version, LaunchOperationID: "restart-launch",
		ProviderExecutionID: "restart-provider-execution",
		ObservedState:       domainappdev.ProviderExecutionObservedRunning, ProviderLeaseDuration: 10 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service1.SaveCheckpoint(context.Background(), applicationappdev.SaveProviderExecutionCheckpointRequest{
		Owner: owner1, ExpectedVersion: submitted.Version, OperationID: "restart-checkpoint-operation", ProviderKey: "provider-a",
		ProviderScope: domainsandbox.ScopeAppDev, LaunchOperationID: "restart-launch", Checkpoint: checkpoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB1.Close(); err != nil {
		t.Fatal(err)
	}

	restartNow := now.Add(2 * time.Minute)
	db2, sqlDB2 := openProviderExecutionSQLiteTestDB(t, databasePath)
	t.Cleanup(func() { _ = sqlDB2.Close() })
	codec2 := providerExecutionRealCheckpointCodec(t, key)
	repository2 := NewProviderExecutionRepository(db2)
	repository2.clock = &providerExecutionControlledDBClock{now: restartNow}
	service2, err := applicationappdev.NewProviderExecutionService(
		repository2, codec2,
		applicationappdev.WithProviderExecutionClock(func() time.Time { return restartNow }),
		applicationappdev.WithProviderExecutionRandom(bytes.NewReader(bytes.Repeat([]byte{0x42}, 128))),
		applicationappdev.WithProviderExecutionOwnerLease(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	reclaimed, err := service2.ClaimRecovery(context.Background(), applicationappdev.ClaimProviderExecutionRecoveryRequest{
		SpaceID: saved.SpaceID, ProjectID: saved.ProjectID, Generation: saved.Generation, ExpectedVersion: saved.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service2.SetDesiredStop(context.Background(), applicationappdev.SetProviderExecutionDesiredStopRequest{
		Owner: owner1, ExpectedVersion: saved.Version,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("old owner write after restart = %v", err)
	}
	recovery, err := service2.LoadRecovery(context.Background(), applicationappdev.LoadProviderExecutionRecoveryRequest{
		Owner: reclaimed.Owner(), ExpectedVersion: reclaimed.Metadata.Version,
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
	})
	if err != nil || !recovery.HasCheckpoint() || recovery.ProviderExecutionID() != "restart-provider-execution" {
		t.Fatalf("restart recovery = %#v, %v", recovery, err)
	}

	var stored providerExecutionRecord
	if err := db2.Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", saved.Generation).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	binding := applicationsandbox.ExecutionCheckpointBinding{
		SpaceID: "1001", ProjectID: "project-a", Generation: saved.Generation,
		ProviderKey: "provider-a", Scope: domainsandbox.ScopeAppDev,
	}
	wrongKeyCodec := providerExecutionRealCheckpointCodec(t, bytes.Repeat([]byte{0x72}, 32))
	if _, err := wrongKeyCodec.Open(context.Background(), binding, stored.CheckpointEnvelope); !errors.Is(err, applicationsandbox.ErrExecutionCheckpointCodec) {
		t.Fatalf("wrong key error = %v", err)
	}
	binding.ProjectID = "project-b"
	if _, err := codec2.Open(context.Background(), binding, stored.CheckpointEnvelope); !errors.Is(err, applicationsandbox.ErrExecutionCheckpointCodec) {
		t.Fatalf("wrong AAD error = %v", err)
	}
}

func providerExecutionRouterCheckpoint(t *testing.T) applicationsandbox.ExecutionCheckpoint {
	t.Helper()
	now := time.Now().UTC()
	provider := &domainsandbox.Provider{
		ID: 1, ProviderKey: "provider-a", Name: "restart provider", Type: domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret: "encrypted-endpoint", CredentialSecret: "encrypted-credential",
		Scopes: []domainsandbox.Scope{domainsandbox.ScopeAppDev},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30, MemoryLimitMB: 256, CPULimit: 1, MaxOutputBytes: 4096, MaxConcurrency: 2,
		},
		Status: domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{
			Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAppDev}, CheckedAt: now,
		},
	}
	runtime := providerExecutionRestartRuntime{}
	router, err := applicationsandbox.NewProviderRouter(
		providerExecutionRestartLookup{provider: provider}, providerExecutionRestartFactory{runtime: runtime},
		providerExecutionRestartLimiter{}, 2*time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := applicationsandbox.NewResolveProviderRequest("provider-a", domainsandbox.ScopeAppDev)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := router.Resolve(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = selected.Execute(context.Background(), infrasandbox.ExecuteRequest{
		Scope: domainsandbox.ScopeAppDev, WorkloadKind: infrasandbox.WorkloadAppDev,
		IdempotencyKey: "restart-router-idempotency", Deadline: now.Add(time.Minute),
		Policy: provider.Policy, Entrypoint: "workspace/server.js",
	})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := selected.Checkpoint()
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint
}

func providerExecutionRealCheckpointCodec(t *testing.T, key []byte) *applicationsandbox.ExecutionCheckpointCodec {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"primary": base64.StdEncoding.EncodeToString(key)})
	if err != nil {
		t.Fatal(err)
	}
	keyRing, err := infrasandbox.ParseSandboxKeyRing(string(payload), "primary")
	if err != nil {
		t.Fatal(err)
	}
	protector, err := infrasandbox.NewExecutionCheckpointProtector(keyRing)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := applicationsandbox.NewExecutionCheckpointCodec(protector)
	if err != nil {
		t.Fatal(err)
	}
	return codec
}
