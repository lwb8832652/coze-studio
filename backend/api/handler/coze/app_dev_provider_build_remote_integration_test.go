/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package coze

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	applicationsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestAppDevProviderBuildRemoteFailureMessagesAreServerOwnedEndToEnd(t *testing.T) {
	const maliciousMessage = "https://provider.invalid/build?token=top-secret Bearer provider-api-key " +
		"s3://private-bucket/object/key minio://internal-bucket/secret-object\n\x00raw-provider-body"

	tests := []struct {
		name         string
		providerCode string
		wantCode     string
		wantMessage  string
	}{
		{
			name:         "allowlisted code",
			providerCode: infrasandbox.BuildSafeErrorCodeSourceInvalid,
			wantCode:     infrasandbox.BuildSafeErrorCodeSourceInvalid,
			wantMessage:  infrasandbox.BuildSafeErrorMessageSourceInvalid,
		},
		{
			name:         "unknown code",
			providerCode: "provider_internal_failure",
			wantCode:     infrasandbox.BuildSafeErrorCodeBuildFailed,
			wantMessage:  infrasandbox.BuildSafeErrorMessageBuildFailed,
		},
		{
			name:         "empty code",
			providerCode: "",
			wantCode:     infrasandbox.BuildSafeErrorCodeBuildFailed,
			wantMessage:  infrasandbox.BuildSafeErrorMessageBuildFailed,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operationID := fmt.Sprintf("remote-build-operation-%02d", index+1)
			fixture := newRemoteProviderBuildRequestFixture(t, operationID, test.providerCode, maliciousMessage, byte(index+1))

			begin := ut.PerformRequest(fixture.server.Engine, http.MethodPost, appDevProviderTestPath+"/build", nil,
				ut.Header{Key: appDevIdempotencyKeyHeader, Value: operationID})
			require.Equal(t, http.StatusAccepted, begin.Code)
			persistedBegin := fixture.assertLedgerOperation(t, operationID)
			beginPayload := assertPublicBuildInProgressResponse(t, begin.Result().Body(), persistedBegin, maliciousMessage)
			require.Equal(t, domainappdev.ProviderExecutionArtifactBuilding, persistedBegin.ArtifactStatus)
			require.Equal(t, 1, fixture.doer.buildBeginCalls())

			immediateRetry := ut.PerformRequest(fixture.server.Engine, http.MethodPost, appDevProviderTestPath+"/build", nil,
				ut.Header{Key: appDevIdempotencyKeyHeader, Value: operationID})
			require.Equal(t, http.StatusAccepted, immediateRetry.Code)
			persistedRetry := fixture.assertLedgerOperation(t, operationID)
			immediatePayload := assertPublicBuildInProgressResponse(t, immediateRetry.Result().Body(), persistedRetry, maliciousMessage)
			require.Equal(t, beginPayload["generation"], immediatePayload["generation"])
			require.Equal(t, beginPayload["state"], immediatePayload["state"])
			require.Equal(t, 1, fixture.doer.buildBeginCalls())
			require.Equal(t, persistedBegin.Generation, persistedRetry.Generation)
			require.Equal(t, domainappdev.ProviderExecutionArtifactBuilding, persistedRetry.ArtifactStatus)

			statusCallsBeforePoll := fixture.doer.buildStatusCalls()
			fixture.doer.allowTerminalFailure()

			poll := ut.PerformRequest(fixture.server.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil,
				ut.Header{Key: appDevIdempotencyKeyHeader, Value: operationID})
			require.Equal(t, http.StatusOK, poll.Code)
			persistedPoll := fixture.assertLedgerOperation(t, operationID)
			pollPayload := assertPublicBuildFailureResponse(t, poll.Result().Body(), persistedPoll, test.wantCode, test.wantMessage, maliciousMessage)
			require.Equal(t, beginPayload["generation"], pollPayload["generation"])
			require.Greater(t, fixture.doer.buildStatusCalls(), statusCallsBeforePoll)

			terminalRetry := ut.PerformRequest(fixture.server.Engine, http.MethodPost, appDevProviderTestPath+"/build", nil,
				ut.Header{Key: appDevIdempotencyKeyHeader, Value: operationID})
			require.Equal(t, http.StatusOK, terminalRetry.Code)
			persisted := fixture.assertLedgerOperation(t, operationID)
			terminalPayload := assertPublicBuildFailureResponse(t, terminalRetry.Result().Body(), persisted, test.wantCode, test.wantMessage, maliciousMessage)
			require.Equal(t, pollPayload["generation"], terminalPayload["generation"])
			require.Equal(t, pollPayload["state"], terminalPayload["state"])

			require.Equal(t, persistedBegin.Generation, persisted.Generation)
			require.Equal(t, domainappdev.ProviderExecutionArtifactFailed, persisted.ArtifactStatus)
			require.Equal(t, test.wantCode, persisted.ArtifactSafeErrorCode)
			require.Equal(t, test.wantMessage, persisted.ArtifactSafeErrorMessage)
			require.Equal(t, 1, fixture.doer.buildBeginCalls())
			fixture.doer.assertWireContract(t, operationID, persisted)
			fixture.assertCheckpointState(t, persisted, maliciousMessage)
			fixture.assertRawLedgerRowDoesNotLeak(t, persisted, maliciousMessage)
			require.Zero(t, fixture.publisher.callCount())
		})
	}
}

func assertPublicBuildInProgressResponse(
	t *testing.T,
	body []byte,
	persisted *domainappdev.ProviderExecution,
	malicious string,
) map[string]any {
	t.Helper()
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.ArtifactUpdatedAt)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assertRemoteBuildExactJSONObject(t, payload, map[string]any{
		"generation":        float64(persisted.Generation),
		"state":             string(appdev.ProviderBuildStateBuilding),
		"release_available": false,
		"size":              float64(0),
		"updated_at":        persisted.ArtifactUpdatedAt.UTC().Format(time.RFC3339Nano),
		"stale":             false,
	})
	assertRemoteBuildResponseDoesNotLeak(t, body, malicious)
	return payload
}

func assertPublicBuildFailureResponse(
	t *testing.T,
	body []byte,
	persisted *domainappdev.ProviderExecution,
	wantCode, wantMessage, malicious string,
) map[string]any {
	t.Helper()
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.ArtifactUpdatedAt)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	assertRemoteBuildExactJSONObject(t, payload, map[string]any{
		"generation":        float64(persisted.Generation),
		"state":             string(appdev.ProviderBuildStateFailed),
		"release_available": false,
		"size":              float64(0),
		"updated_at":        persisted.ArtifactUpdatedAt.UTC().Format(time.RFC3339Nano),
		"stale":             false,
		"safe_error_code":   wantCode,
		"safe_message":      wantMessage,
	})
	assertRemoteBuildResponseDoesNotLeak(t, body, malicious)
	return payload
}

func assertRemoteBuildResponseDoesNotLeak(t *testing.T, body []byte, malicious string) {
	t.Helper()
	public := string(body)
	for _, forbidden := range []string{
		malicious, "provider.invalid", "token=top-secret", "Bearer", "provider-api-key",
		"s3://", "minio://", "object/key", "secret-object", "raw-provider-body",
	} {
		require.NotContains(t, public, forbidden)
	}
}

type remoteProviderBuildRequestFixture struct {
	server                    *server.Hertz
	ledger                    *appdev.ProviderExecutionService
	repository                *infraappdev.ProviderExecutionRepository
	rawDB                     *sql.DB
	codec                     *applicationsandbox.ExecutionCheckpointCodec
	checkpointProtector       *remoteProviderBuildRecordingProtector
	initialCheckpointEnvelope string
	initialOwnerHash          domainappdev.ProviderExecutionOwnerHash
	initialOwnerEpoch         uint64
	doer                      *remoteProviderBuildHTTPDoer
	publisher                 *remoteProviderBuildRecordingPublisher
}

func (fixture *remoteProviderBuildRequestFixture) assertLedgerOperation(t *testing.T, operationID string) *domainappdev.ProviderExecution {
	t.Helper()
	persisted, err := fixture.repository.LoadCurrent(context.Background(), domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-safe",
	})
	require.NoError(t, err)
	require.Equal(t, operationID, persisted.BuildOperationID)
	require.Equal(t, "provider-execution-real", persisted.ProviderExecutionID)
	require.NotEmpty(t, persisted.CheckpointEnvelope)
	require.Equal(t, fixture.initialCheckpointEnvelope, persisted.CheckpointEnvelope)
	require.Equal(t, fixture.initialOwnerHash, persisted.OwnerIdentityHash)
	require.Equal(t, fixture.initialOwnerEpoch, persisted.OwnerEpoch)
	expectedHash, err := domainappdev.HashProviderExecutionBuildOperationID(operationID, domainappdev.ProviderExecutionOwnerCAS{
		SpaceID: persisted.SpaceID, ProjectID: persisted.ProjectID, Generation: persisted.Generation,
		ExpectedVersion: persisted.Version, OwnerHash: persisted.OwnerIdentityHash, OwnerEpoch: persisted.OwnerEpoch,
		ProviderKey: persisted.ProviderKey, ProviderScope: persisted.ProviderScope,
	}, persisted.ProviderExecutionID)
	require.NoError(t, err)
	require.Equal(t, expectedHash, persisted.BuildOperationHash)
	return persisted
}

func (fixture *remoteProviderBuildRequestFixture) assertCheckpointState(
	t *testing.T,
	persisted *domainappdev.ProviderExecution,
	malicious string,
) {
	t.Helper()
	checkpoint, err := fixture.codec.Open(context.Background(), applicationsandbox.ExecutionCheckpointBinding{
		SpaceID: persisted.SpaceID, ProjectID: persisted.ProjectID, Generation: persisted.Generation,
		ProviderKey: persisted.ProviderKey, Scope: persisted.ProviderScope,
	}, persisted.CheckpointEnvelope)
	require.NoError(t, err)
	for _, formatted := range []string{
		fmt.Sprintf("%v", checkpoint), fmt.Sprintf("%+v", checkpoint), fmt.Sprintf("%#v", checkpoint), checkpoint.GoString(),
	} {
		require.Equal(t, "ExecutionCheckpoint{secrets:<redacted>}", formatted)
		assertRemoteBuildTextDoesNotLeak(t, formatted, malicious)
	}
	encoded, marshalErr := json.Marshal(checkpoint)
	require.Error(t, marshalErr)
	assertRemoteBuildTextDoesNotLeak(t, string(encoded), malicious)
	plaintext := fixture.checkpointProtector.latestOpenedPlaintext()
	require.NotEmpty(t, plaintext)
	assertRemoteBuildTextDoesNotLeak(t, string(plaintext), malicious)
	assertRemoteBuildTextDoesNotLeak(t, persisted.CheckpointEnvelope, malicious)
}

func (fixture *remoteProviderBuildRequestFixture) assertRawLedgerRowDoesNotLeak(
	t *testing.T,
	persisted *domainappdev.ProviderExecution,
	malicious string,
) {
	t.Helper()
	rows, err := fixture.rawDB.QueryContext(context.Background(),
		"SELECT * FROM appdev_provider_executions WHERE space_id = ? AND project_id = ? AND generation = ?",
		int64(1001), persisted.ProjectID, persisted.Generation)
	require.NoError(t, err)
	defer rows.Close()
	columns, err := rows.Columns()
	require.NoError(t, err)
	require.True(t, rows.Next())
	values := make([]sql.RawBytes, len(columns))
	destinations := make([]any, len(columns))
	for index := range values {
		destinations[index] = &values[index]
	}
	require.NoError(t, rows.Scan(destinations...))
	for index, value := range values {
		assertRemoteBuildTextDoesNotLeakf(t, string(value), malicious, "raw SQLite column %s leaked provider data", columns[index])
	}
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
}

func newRemoteProviderBuildRequestFixture(
	t *testing.T,
	buildOperationID string,
	providerCode string,
	providerMessage string,
	entropyByte byte,
) *remoteProviderBuildRequestFixture {
	t.Helper()
	ctx := context.Background()
	db, sqlDB := openRemoteProviderBuildSQLite(t)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, migrateRemoteProviderBuildSQLite(db))
	require.NoError(t, db.Exec("INSERT INTO appdev_projects (id, space_id) VALUES (?, ?)", "project-safe", int64(1001)).Error)

	codec, checkpointProtector := newRemoteProviderBuildCheckpointCodec(t)
	repository := infraappdev.NewProviderExecutionRepository(db)
	ledger, err := appdev.NewProviderExecutionService(
		repository,
		codec,
		appdev.WithProviderExecutionOwnerLease(5*time.Minute),
		appdev.WithProviderExecutionCheckpointReservation(time.Minute),
		appdev.WithProviderExecutionRandom(bytes.NewReader(bytes.Repeat([]byte{entropyByte + 0x30}, 4096))),
	)
	require.NoError(t, err)

	doer := &remoteProviderBuildHTTPDoer{
		t: t, buildOperationID: buildOperationID, providerCode: providerCode, providerMessage: providerMessage,
	}
	remote, err := infrasandbox.NewRemoteProvider(infrasandbox.RemoteProviderConfig{
		Endpoint: "https://runner.example.test", Credential: "remote-provider-credential",
		AllowedHosts: []string{"runner.example.test"}, Timeout: time.Minute,
	})
	require.NoError(t, err)
	setRemoteProviderHTTPDoerForRequestTest(t, remote, doer)

	providerKey := "remote-appdev-default"
	policy := domainsandbox.RuntimePolicy{
		TimeoutSeconds: 30, MemoryLimitMB: 256, CPULimit: 1,
		MaxOutputBytes: 1024 * 1024, MaxConcurrency: 2,
	}
	provider := &domainsandbox.Provider{
		ID: 9100, ProviderKey: providerKey, Name: "Remote AppDev", Type: domainsandbox.ProviderTypeRemoteHTTP,
		EndpointSecret: "encrypted-endpoint", CredentialSecret: "encrypted-credential",
		Scopes: []domainsandbox.Scope{domainsandbox.ScopeAppDev}, Policy: policy,
		Status: domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{
			Status: domainsandbox.HealthStatusHealthy, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAppDev}, CheckedAt: time.Now().UTC(),
		},
	}
	router, err := applicationsandbox.NewProviderRouter(
		&remoteProviderBuildLookup{provider: provider},
		&remoteProviderBuildFactory{runtime: remote},
		remoteProviderBuildCapacityLimiter{},
		2*time.Minute,
	)
	require.NoError(t, err)

	ownerToken, err := domainappdev.NewProviderExecutionOwnerToken(bytes.NewReader(bytes.Repeat([]byte{entropyByte + 0x60}, 64)))
	require.NoError(t, err)
	seedRemoteProviderBuildRunningExecution(t, ctx, ledger, router, ownerToken, providerKey, policy)
	initialRecord, err := repository.LoadCurrent(ctx, domainappdev.LoadCurrentProviderExecutionInput{
		SpaceID: "1001", ProjectID: "project-safe",
	})
	require.NoError(t, err)
	require.NotEmpty(t, initialRecord.CheckpointEnvelope)

	publisher := &remoteProviderBuildRecordingPublisher{}
	orchestrator, err := appdev.NewProviderBuildOrchestrator(
		ledger,
		appdev.SandboxProviderRuntimeRouter{Router: router},
		publisher,
		remoteProviderBuildOwnerGenerator{token: ownerToken},
		appdev.ProviderBuildOrchestratorConfig{ProviderKey: providerKey, ProviderScope: domainsandbox.ScopeAppDev},
	)
	require.NoError(t, err)
	facade := appdev.NewProviderAPIFacade(nil, orchestrator, nil, nil)
	control := &remoteProviderBuildHTTPControl{facade: facade}
	h := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)
	return &remoteProviderBuildRequestFixture{
		server: h, ledger: ledger, repository: repository, rawDB: sqlDB, codec: codec, checkpointProtector: checkpointProtector,
		initialCheckpointEnvelope: initialRecord.CheckpointEnvelope,
		initialOwnerHash:          initialRecord.OwnerIdentityHash, initialOwnerEpoch: initialRecord.OwnerEpoch,
		doer: doer, publisher: publisher,
	}
}

func seedRemoteProviderBuildRunningExecution(
	t *testing.T,
	ctx context.Context,
	ledger *appdev.ProviderExecutionService,
	router *applicationsandbox.ProviderRouter,
	ownerToken domainappdev.ProviderExecutionOwnerToken,
	providerKey string,
	policy domainsandbox.RuntimePolicy,
) {
	t.Helper()
	metadata, err := ledger.EnsureStart(ctx, appdev.EnsureProviderExecutionStartRequest{
		SpaceID: "1001", ProjectID: "project-safe", IdempotencyKey: "runtime-start-operation-001",
		ProviderKey: providerKey, ProviderScope: domainsandbox.ScopeAppDev, RequireNoActive: true,
	})
	require.NoError(t, err)
	claim, err := ledger.ClaimRecovery(ctx, appdev.ClaimProviderExecutionRecoveryRequest{
		SpaceID: metadata.SpaceID, ProjectID: metadata.ProjectID, Generation: metadata.Generation,
		ExpectedVersion: metadata.Version, ProposedOwner: ownerToken,
	})
	require.NoError(t, err)
	executeRequest := infrasandbox.ExecuteRequest{
		Scope: domainsandbox.ScopeAppDev, WorkloadKind: infrasandbox.WorkloadAppDev,
		IdempotencyKey: "runtime-provider-execute-001", Deadline: time.Now().Add(time.Minute).UTC(),
		Policy: policy, Entrypoint: "bin/appdev", Args: []string{"serve"}, Env: map[string]string{"MODE": "test"},
	}
	requestDigest, err := infrasandbox.DigestExecuteRequest(executeRequest)
	require.NoError(t, err)
	metadata, err = ledger.StartProviderSubmission(ctx, appdev.StartProviderExecutionSubmissionRequest{
		Owner: claim.Owner(), ExpectedVersion: claim.Metadata.Version,
		OperationID: "runtime-start-operation-001", ProviderOperationID: executeRequest.IdempotencyKey,
		RequestDigest: domainappdev.ProviderExecutionLaunchRequestDigest(requestDigest),
	})
	require.NoError(t, err)
	metadata, err = ledger.MarkProviderLaunchSubmitted(ctx, appdev.MarkProviderExecutionLaunchSubmittedRequest{
		Owner: claim.Owner(), ExpectedVersion: metadata.Version, OperationID: "runtime-start-operation-001",
		DispatchLeaseDuration: time.Minute,
	})
	require.NoError(t, err)

	resolveRequest, err := applicationsandbox.NewResolveProviderRequest(providerKey, domainsandbox.ScopeAppDev)
	require.NoError(t, err)
	selection, err := router.Resolve(ctx, resolveRequest)
	require.NoError(t, err)
	execution, err := selection.Execute(ctx, executeRequest)
	require.NoError(t, err)
	require.Equal(t, infrasandbox.ExecutionStatusRunning, execution.Status)
	metadata, err = ledger.SaveProviderSubmission(ctx, appdev.SaveProviderExecutionSubmissionRequest{
		Owner: claim.Owner(), ExpectedVersion: metadata.Version, LaunchOperationID: "runtime-start-operation-001",
		ProviderExecutionID: execution.ExecutionID,
		ObservedState:       domainappdev.ProviderExecutionObservedRunning, ProviderLeaseDuration: 2 * time.Minute,
	})
	require.NoError(t, err)
	checkpoint, err := selection.Checkpoint()
	require.NoError(t, err)
	_, err = ledger.SaveCheckpoint(ctx, appdev.SaveProviderExecutionCheckpointRequest{
		Owner: claim.Owner(), ExpectedVersion: metadata.Version, OperationID: "checkpoint-operation-001",
		LaunchOperationID: "runtime-start-operation-001",
		ProviderKey:       providerKey, ProviderScope: domainsandbox.ScopeAppDev, Checkpoint: checkpoint,
	})
	require.NoError(t, err)
}

type remoteProviderBuildHTTPDoer struct {
	t                *testing.T
	buildOperationID string
	providerCode     string
	providerMessage  string
	mu               sync.Mutex
	beginOperations  []string
	statusOperations []string
	terminalFailure  bool
	exchanges        []remoteProviderBuildHTTPExchange
}

type remoteProviderBuildHTTPExchange struct {
	Method       string
	Scheme       string
	Host         string
	Path         string
	RawQuery     string
	Header       http.Header
	RequestBody  []byte
	ResponseBody []byte
}

func (doer *remoteProviderBuildHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	switch {
	case request.Method == http.MethodPost && request.URL.Path == "/v1/executions":
		return remoteProviderBuildJSONResponse(request, map[string]any{
			"schema": "coze.sandbox.execute.v1", "execution_id": "provider-execution-real",
			"status": "running", "exit_code": nil, "stdout": "", "stderr": "", "artifacts": []any{},
		})
	case request.Method == http.MethodPost && request.URL.Path == "/v1/executions/provider-execution-real/builds":
		requestBody, err := io.ReadAll(request.Body)
		require.NoError(doer.t, err)
		var body map[string]any
		require.NoError(doer.t, json.Unmarshal(requestBody, &body))
		operationID, ok := body["operation_id"].(string)
		require.True(doer.t, ok)
		doer.mu.Lock()
		doer.beginOperations = append(doer.beginOperations, operationID)
		doer.mu.Unlock()
		return doer.buildResponse(request, requestBody, map[string]any{
			"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID,
			"status": "accepted",
		})
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/executions/provider-execution-real/builds/"):
		operationID := strings.TrimPrefix(request.URL.Path, "/v1/executions/provider-execution-real/builds/")
		doer.mu.Lock()
		doer.statusOperations = append(doer.statusOperations, operationID)
		terminalFailure := doer.terminalFailure
		doer.mu.Unlock()
		if !terminalFailure {
			return doer.buildResponse(request, nil, map[string]any{
				"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID,
				"status": "running",
			})
		}
		return doer.buildResponse(request, nil, map[string]any{
			"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID,
			"status": "failed", "safe_error_code": doer.providerCode, "safe_error_message": doer.providerMessage,
		})
	default:
		doer.t.Fatalf("unexpected remote provider request %s %s", request.Method, request.URL.Path)
		return nil, fmt.Errorf("unexpected remote provider request")
	}
}

func (doer *remoteProviderBuildHTTPDoer) buildResponse(request *http.Request, requestBody []byte, payload any) (*http.Response, error) {
	responseBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	exchange := remoteProviderBuildHTTPExchange{
		Method: request.Method, Scheme: request.URL.Scheme, Host: request.URL.Host, Path: request.URL.Path,
		RawQuery: request.URL.RawQuery, Header: request.Header.Clone(),
		RequestBody: append([]byte(nil), requestBody...), ResponseBody: append([]byte(nil), responseBody...),
	}
	doer.mu.Lock()
	doer.exchanges = append(doer.exchanges, exchange)
	doer.mu.Unlock()
	return remoteProviderBuildRawJSONResponse(request, responseBody), nil
}

func (doer *remoteProviderBuildHTTPDoer) buildBeginCalls() int {
	doer.mu.Lock()
	defer doer.mu.Unlock()
	return len(doer.beginOperations)
}

func (doer *remoteProviderBuildHTTPDoer) buildStatusCalls() int {
	doer.mu.Lock()
	defer doer.mu.Unlock()
	return len(doer.statusOperations)
}

func (doer *remoteProviderBuildHTTPDoer) allowTerminalFailure() {
	doer.mu.Lock()
	doer.terminalFailure = true
	doer.mu.Unlock()
}

func (doer *remoteProviderBuildHTTPDoer) assertWireContract(
	t *testing.T,
	operationID string,
	persisted *domainappdev.ProviderExecution,
) {
	t.Helper()
	doer.mu.Lock()
	beginOperations := append([]string(nil), doer.beginOperations...)
	statusOperations := append([]string(nil), doer.statusOperations...)
	exchanges := make([]remoteProviderBuildHTTPExchange, len(doer.exchanges))
	copy(exchanges, doer.exchanges)
	doer.mu.Unlock()
	require.Equal(t, []string{operationID}, beginOperations)
	require.NotEmpty(t, statusOperations)
	for _, captured := range statusOperations {
		require.Equal(t, operationID, captured)
	}
	require.NotEmpty(t, exchanges)
	hashHex := hex.EncodeToString(persisted.BuildOperationHash[:])
	hashBase64 := base64.StdEncoding.EncodeToString(persisted.BuildOperationHash[:])
	failedResponses := 0
	for index, exchange := range exchanges {
		require.Equal(t, "https", exchange.Scheme)
		require.Equal(t, "runner.example.test", exchange.Host)
		require.Empty(t, exchange.RawQuery)
		require.Equal(t, "Bearer remote-provider-credential", exchange.Header.Get("Authorization"))
		require.Equal(t, persisted.ProviderExecutionID, remoteProviderBuildExecutionIDFromPath(t, exchange.Path))
		var response map[string]any
		require.NoError(t, json.Unmarshal(exchange.ResponseBody, &response))
		if index == 0 {
			require.Equal(t, http.MethodPost, exchange.Method)
			require.Equal(t, "/v1/executions/"+persisted.ProviderExecutionID+"/builds", exchange.Path)
			var requestWire map[string]any
			require.NoError(t, json.Unmarshal(exchange.RequestBody, &requestWire))
			assertRemoteBuildExactJSONObject(t, requestWire, map[string]any{
				"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID,
			})
			assertRemoteBuildExactJSONObject(t, response, map[string]any{
				"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID, "status": "accepted",
			})
		} else {
			require.Equal(t, http.MethodGet, exchange.Method)
			require.Equal(t, "/v1/executions/"+persisted.ProviderExecutionID+"/builds/"+operationID, exchange.Path)
			require.Empty(t, exchange.RequestBody)
			if index == len(exchanges)-1 {
				assertRemoteBuildExactJSONObject(t, response, map[string]any{
					"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID, "status": "failed",
					"safe_error_code": doer.providerCode, "safe_error_message": doer.providerMessage,
				})
				failedResponses++
			} else {
				assertRemoteBuildExactJSONObject(t, response, map[string]any{
					"schema": "coze.sandbox.appdev_build.v1", "operation_id": operationID, "status": "running",
				})
			}
		}
		require.NotContains(t, response, "generation")
		require.NotContains(t, response, "provider_execution_id")
		require.NotContains(t, response, "build_operation_hash")
		require.NotContains(t, response, "checkpoint")
		requestText := exchange.Method + "\n" + exchange.Path + "\n" + exchange.RawQuery + "\n" + string(exchange.RequestBody)
		for header, values := range exchange.Header {
			requestText += "\n" + header + ":" + strings.Join(values, ",")
		}
		for _, forbidden := range []string{
			"generation", "provider_execution_id", "build_operation_hash", "checkpoint", "checkpoint_envelope",
			hashHex, hashBase64, persisted.CheckpointEnvelope,
		} {
			require.NotContains(t, requestText, forbidden)
		}
	}
	require.Equal(t, 1, failedResponses)
}

func assertRemoteBuildExactJSONObject(t *testing.T, actual, expected map[string]any) {
	t.Helper()
	actualKeys := make([]string, 0, len(actual))
	for key := range actual {
		actualKeys = append(actualKeys, key)
	}
	expectedKeys := make([]string, 0, len(expected))
	for key := range expected {
		expectedKeys = append(expectedKeys, key)
	}
	sort.Strings(actualKeys)
	sort.Strings(expectedKeys)
	require.Equal(t, expectedKeys, actualKeys)
	for _, key := range expectedKeys {
		expectedNested, nested := expected[key].(map[string]any)
		if nested {
			actualNested, ok := actual[key].(map[string]any)
			require.Truef(t, ok, "JSON field %q must be an object", key)
			assertRemoteBuildExactJSONObject(t, actualNested, expectedNested)
			continue
		}
		require.Equalf(t, expected[key], actual[key], "unexpected JSON field %q", key)
	}
}

func remoteProviderBuildExecutionIDFromPath(t *testing.T, path string) string {
	t.Helper()
	const prefix = "/v1/executions/"
	require.True(t, strings.HasPrefix(path, prefix))
	remainder := strings.TrimPrefix(path, prefix)
	parts := strings.Split(remainder, "/")
	require.GreaterOrEqual(t, len(parts), 2)
	require.Equal(t, "builds", parts[1])
	return parts[0]
}

func remoteProviderBuildJSONResponse(request *http.Request, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return remoteProviderBuildRawJSONResponse(request, body), nil
}

func remoteProviderBuildRawJSONResponse(request *http.Request, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}

func setRemoteProviderHTTPDoerForRequestTest(t *testing.T, provider *infrasandbox.RemoteProvider, doer *remoteProviderBuildHTTPDoer) {
	t.Helper()
	field := reflect.ValueOf(provider).Elem().FieldByName("doer")
	require.True(t, field.IsValid())
	value := reflect.ValueOf(doer)
	require.True(t, value.Type().AssignableTo(field.Type()) || value.Type().Implements(field.Type()))
	writable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	writable.Set(value)
}

type remoteProviderBuildLookup struct{ provider *domainsandbox.Provider }

func (lookup *remoteProviderBuildLookup) GetProviderByKey(ctx context.Context, key string) (*domainsandbox.Provider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if lookup.provider == nil || key != lookup.provider.ProviderKey {
		return nil, domainsandbox.ErrProviderNotFound
	}
	copy := *lookup.provider
	return &copy, nil
}

type remoteProviderBuildFactory struct{ runtime *infrasandbox.RemoteProvider }

func (*remoteProviderBuildFactory) ValidateConfig(context.Context, applicationsandbox.ProviderDescriptor) error {
	return nil
}

func (factory *remoteProviderBuildFactory) Build(context.Context, domainsandbox.Provider) (infrasandbox.RuntimeProvider, error) {
	return factory.runtime, nil
}

type remoteProviderBuildCapacityLimiter struct{}

func (remoteProviderBuildCapacityLimiter) Acquire(context.Context, string, string, string, int, time.Duration) (int64, error) {
	return time.Now().Add(5 * time.Minute).UnixMilli(), nil
}

func (remoteProviderBuildCapacityLimiter) Renew(context.Context, string, string, string, string, time.Duration) (int64, error) {
	return time.Now().Add(5 * time.Minute).UnixMilli(), nil
}

func (remoteProviderBuildCapacityLimiter) Release(context.Context, string, string, string) error {
	return nil
}

type remoteProviderBuildOwnerGenerator struct {
	token domainappdev.ProviderExecutionOwnerToken
}

func (generator remoteProviderBuildOwnerGenerator) GenerateProviderRuntimeOwnerCapability() (domainappdev.ProviderExecutionOwnerToken, error) {
	return generator.token, nil
}

type remoteProviderBuildRecordingPublisher struct {
	calls atomic.Int64
}

func (publisher *remoteProviderBuildRecordingPublisher) PublishWithPublisher(
	context.Context,
	infrasandbox.ArtifactPublisher,
	appdev.PublishBuildArtifactCommand,
) (*appdev.BuildArtifactReceipt, error) {
	publisher.calls.Add(1)
	return nil, fmt.Errorf("artifact publisher must not be called for failed provider builds")
}

func (publisher *remoteProviderBuildRecordingPublisher) callCount() int64 {
	return publisher.calls.Load()
}

type remoteProviderBuildHTTPControl struct {
	facade *appdev.ProviderAPIFacade
}

func (control *remoteProviderBuildHTTPControl) StartRuntime(ctx context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	return control.facade.StartRuntime(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) RuntimeStatus(ctx context.Context, request appdev.ProviderRuntimeStatusRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	return control.facade.RuntimeStatus(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) StopRuntime(ctx context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	return control.facade.StopRuntime(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) RestartRuntime(ctx context.Context, request appdev.ProviderRuntimeMutationRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	return control.facade.RestartRuntime(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) BeginBuild(ctx context.Context, request appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error) {
	return control.facade.BeginBuild(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) PollBuild(ctx context.Context, request appdev.ProviderBuildOperationRequest) (*appdev.ProviderBuildAPIProjection, error) {
	return control.facade.PollBuild(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) RestoreSnapshot(ctx context.Context, request appdev.ProviderSnapshotRestoreRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	return control.facade.RestoreSnapshot(ctx, request)
}

func (control *remoteProviderBuildHTTPControl) OpenRelease(ctx context.Context, request appdev.ProviderReleaseRequest) (*appDevHTTPRelease, error) {
	release, err := control.facade.OpenRelease(ctx, request)
	if err != nil || release == nil {
		return nil, err
	}
	return &appDevHTTPRelease{Body: release, Size: release.Size, Stale: release.Stale, UpdatedAt: release.UpdatedAt}, nil
}

func openRemoteProviderBuildSQLite(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	dsn := "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "provider-build-request.db")) +
		"?_foreign_keys=on&_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	return db, sqlDB
}

func migrateRemoteProviderBuildSQLite(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE appdev_projects (id TEXT NOT NULL PRIMARY KEY, space_id INTEGER NOT NULL)`,
		`CREATE TABLE appdev_provider_executions (
            id TEXT NOT NULL PRIMARY KEY,
            space_id INTEGER NOT NULL,
            project_id TEXT NOT NULL,
            generation INTEGER NOT NULL,
            idempotency_key BLOB NOT NULL,
            desired_state TEXT NOT NULL,
            observed_state TEXT NOT NULL,
            provider_key TEXT NOT NULL,
            provider_scope TEXT NOT NULL,
            provider_execution_id BLOB NOT NULL,
            submission_started_at DATETIME NULL,
            launch_state TEXT NOT NULL DEFAULT 'none',
            launch_operation_hash BLOB NULL,
            launch_provider_operation_id BLOB NOT NULL DEFAULT '',
            launch_request_digest BLOB NULL,
            launch_expires_at DATETIME NULL,
            checkpoint_envelope TEXT NOT NULL,
            checkpoint_write_revision INTEGER NOT NULL DEFAULT 0,
            checkpoint_write_pending INTEGER NOT NULL DEFAULT 0,
            checkpoint_write_operation_hash BLOB NULL,
            checkpoint_write_expires_at DATETIME NULL,
            checkpoint_last_operation_hash BLOB NULL,
            cleanup_operation_hash BLOB NULL,
            terminal_operation_hash BLOB NULL,
            release_owner_operation_hash BLOB NULL,
            provider_lease_expires_at DATETIME NULL,
            owner_identity_hash BLOB NULL,
            owner_epoch INTEGER NOT NULL DEFAULT 0,
            owner_expires_at DATETIME NULL,
            preview_route TEXT NOT NULL DEFAULT '',
            artifact_object_key TEXT NOT NULL DEFAULT '',
            build_operation_id BLOB NOT NULL DEFAULT '',
            build_operation_hash BLOB NULL,
            artifact_status TEXT NOT NULL DEFAULT 'none',
            artifact_kind TEXT NOT NULL DEFAULT '',
            artifact_digest TEXT NOT NULL DEFAULT '',
            artifact_size INTEGER NOT NULL DEFAULT 0,
            artifact_version INTEGER NOT NULL DEFAULT 0,
            build_started_at DATETIME NULL,
            artifact_updated_at DATETIME NULL,
            artifact_safe_error_code TEXT NOT NULL DEFAULT '',
            artifact_safe_error_message TEXT NOT NULL DEFAULT '',
            safe_error_code TEXT NOT NULL DEFAULT '',
            safe_error_message TEXT NOT NULL DEFAULT '',
            version INTEGER NOT NULL DEFAULT 1,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        )`,
		`CREATE UNIQUE INDEX uk_appdev_provider_exec_generation ON appdev_provider_executions (space_id, project_id, generation)`,
		`CREATE UNIQUE INDEX uk_appdev_provider_exec_idempotency ON appdev_provider_executions (space_id, project_id, idempotency_key)`,
		`CREATE INDEX idx_appdev_provider_exec_recoverable ON appdev_provider_executions (space_id, project_id, observed_state, desired_state, owner_expires_at, updated_at, id)`,
		`CREATE INDEX idx_appdev_provider_exec_provider_id ON appdev_provider_executions (space_id, project_id, provider_key, provider_execution_id)`,
		`CREATE INDEX idx_appdev_provider_exec_launch ON appdev_provider_executions (space_id, project_id, launch_state, launch_expires_at)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

type remoteProviderBuildRecordingProtector struct {
	delegate applicationsandbox.ExecutionCheckpointProtector
	mu       sync.Mutex
	opened   [][]byte
}

func (protector *remoteProviderBuildRecordingProtector) Seal(ctx context.Context, aad, plaintext []byte) (string, error) {
	return protector.delegate.Seal(ctx, aad, plaintext)
}

func (protector *remoteProviderBuildRecordingProtector) Open(ctx context.Context, aad []byte, envelope string) ([]byte, error) {
	plaintext, err := protector.delegate.Open(ctx, aad, envelope)
	if err == nil {
		protector.mu.Lock()
		protector.opened = append(protector.opened, append([]byte(nil), plaintext...))
		protector.mu.Unlock()
	}
	return plaintext, err
}

func (protector *remoteProviderBuildRecordingProtector) latestOpenedPlaintext() []byte {
	protector.mu.Lock()
	defer protector.mu.Unlock()
	if len(protector.opened) == 0 {
		return nil
	}
	return append([]byte(nil), protector.opened[len(protector.opened)-1]...)
}

func newRemoteProviderBuildCheckpointCodec(t *testing.T) (*applicationsandbox.ExecutionCheckpointCodec, *remoteProviderBuildRecordingProtector) {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	keys, err := json.Marshal(map[string]string{"primary": key})
	require.NoError(t, err)
	ring, err := infrasandbox.ParseSandboxKeyRing(string(keys), "primary")
	require.NoError(t, err)
	protector, err := infrasandbox.NewExecutionCheckpointProtector(ring)
	require.NoError(t, err)
	recordingProtector := &remoteProviderBuildRecordingProtector{delegate: protector}
	codec, err := applicationsandbox.NewExecutionCheckpointCodec(recordingProtector)
	require.NoError(t, err)
	return codec, recordingProtector
}

func assertRemoteBuildTextDoesNotLeak(t *testing.T, text, malicious string) {
	t.Helper()
	assertRemoteBuildTextDoesNotLeakf(t, text, malicious, "provider data leaked")
}

func assertRemoteBuildTextDoesNotLeakf(t *testing.T, text, malicious, message string, args ...any) {
	t.Helper()
	for _, forbidden := range []string{
		malicious, "provider.invalid", "token=top-secret", "Bearer", "provider-api-key",
		"s3://", "minio://", "object/key", "secret-object", "raw-provider-body",
	} {
		require.NotContainsf(t, text, forbidden, message, args...)
	}
}
