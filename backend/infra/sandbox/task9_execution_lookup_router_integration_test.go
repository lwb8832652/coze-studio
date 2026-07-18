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

package sandbox_test

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type versionedLookupRouterDoer struct {
	mu       sync.Mutex
	status   infrasandbox.ExecutionLookupStatus
	requests []*http.Request
	bodies   []string
}

func (doer *versionedLookupRouterDoer) Do(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	doer.mu.Lock()
	doer.requests = append(doer.requests, request)
	doer.bodies = append(doer.bodies, string(body))
	status := doer.status
	doer.mu.Unlock()
	execution := "null"
	if status == infrasandbox.ExecutionLookupFound {
		execution = `{"schema":"coze.sandbox.execute.v1","execution_id":"lookup-router-real","status":"running","exit_code":null,"stdout":"","stderr":"","artifacts":[],"artifact_descriptors":[],"preview_route":"/preview/lookup"}`
	}
	response := testJSONResponse(request, http.StatusOK,
		`{"schema":"coze.sandbox.execution_lookup.v1","status":"`+string(status)+`","execution":`+execution+`}`)
	response.Header.Set(infrasandbox.ExecutionLookupProtocolVersionHeader, infrasandbox.ExecutionLookupProtocolVersionV1)
	return response, nil
}

func newVersionedLookupRouter(t *testing.T, status infrasandbox.ExecutionLookupStatus) (*appsandbox.ProviderRouter, *versionedLookupRouterDoer) {
	t.Helper()
	doer := &versionedLookupRouterDoer{status: status}
	remote, err := infrasandbox.NewRemoteProviderWithTestDoer(infrasandbox.RemoteProviderConfig{
		Endpoint: "https://lookup.example.test/", Credential: "synthetic-lookup-credential",
		AllowedHosts: []string{"lookup.example.test"}, Timeout: time.Second,
	}, doer)
	require.NoError(t, err)
	now := time.Now().UTC()
	provider := &domainsandbox.Provider{
		ID: 42, ProviderKey: "lookup-provider", Name: "lookup provider",
		Type: domainsandbox.ProviderTypeRemoteHTTP, Scopes: []domainsandbox.Scope{domainsandbox.ScopeAppDev},
		Policy: domainsandbox.RuntimePolicy{
			TimeoutSeconds: 30, MemoryLimitMB: 256, CPULimit: 1,
			MaxOutputBytes: 4096, MaxConcurrency: 2,
		},
		Status: domainsandbox.ProviderStatusEnabled,
		Health: domainsandbox.HealthSnapshot{
			Status:       domainsandbox.HealthStatusHealthy,
			Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAppDev}, CheckedAt: now,
		},
	}
	router, err := appsandbox.NewProviderRouter(
		authoritativeProviderLookup{provider: provider},
		authoritativeRuntimeFactory{runtime: remote},
		&authoritativeCapacityLimiter{},
		time.Minute,
	)
	require.NoError(t, err)
	return router, doer
}

func TestProviderRouterVersionedExecutionLookupUsesRemoteProtocol(t *testing.T) {
	for _, status := range []infrasandbox.ExecutionLookupStatus{
		infrasandbox.ExecutionLookupFound,
		infrasandbox.ExecutionLookupNotFound,
		infrasandbox.ExecutionLookupUnknown,
	} {
		t.Run("current_"+string(status), func(t *testing.T) {
			router, doer := newVersionedLookupRouter(t, status)
			digest := infrasandbox.ExecutionRequestDigest(sha256.Sum256([]byte("current canonical launch")))
			request, err := appsandbox.NewLookupProviderExecutionRequest(
				"lookup-provider", domainsandbox.ScopeAppDev, "current-operation", digest,
			)
			require.NoError(t, err)
			result, err := router.LookupProviderExecution(context.Background(), request)
			require.NoError(t, err)
			require.Equal(t, status, result.Status)
			require.Equal(t, status == infrasandbox.ExecutionLookupFound, result.Selection != nil)
			require.Len(t, doer.bodies, 1)
			require.Contains(t, doer.bodies[0], infrasandbox.ExecutionLookupSchemaV1)
			require.Contains(t, doer.bodies[0], digest.Hex())
			require.NotContains(t, doer.bodies[0], "space_id")
		})

		t.Run("legacy_"+string(status), func(t *testing.T) {
			router, doer := newVersionedLookupRouter(t, status)
			request, err := appsandbox.NewLegacyLookupProviderExecutionRequest(
				"lookup-provider", domainsandbox.ScopeAppDev,
				"1001", "project-a", 7, "legacy-operation",
			)
			require.NoError(t, err)
			result, err := router.LookupProviderExecution(context.Background(), request)
			require.NoError(t, err)
			require.Equal(t, status, result.Status)
			require.Equal(t, status == infrasandbox.ExecutionLookupFound, result.Selection != nil)
			require.Len(t, doer.bodies, 1)
			require.Contains(t, doer.bodies[0], infrasandbox.ExecutionLookupLegacySchemaV1)
			require.Contains(t, doer.bodies[0], `"space_id":"1001"`)
			require.Contains(t, doer.bodies[0], `"project_id":"project-a"`)
			require.NotContains(t, doer.bodies[0], "request_digest")
			require.NotContains(t, doer.bodies[0], "synthetic-lookup-credential")
		})
	}
}

func TestProviderRouterLegacyLookupRejectsMutatedTenantIdentityBeforeRemote(t *testing.T) {
	router, doer := newVersionedLookupRouter(t, infrasandbox.ExecutionLookupNotFound)
	request, err := appsandbox.NewLegacyLookupProviderExecutionRequest(
		"lookup-provider", domainsandbox.ScopeAppDev,
		"1001", "project-a", 7, "legacy-operation",
	)
	require.NoError(t, err)
	request.SpaceID = "1002"
	_, err = router.LookupProviderExecution(context.Background(), request)
	require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
	require.Empty(t, doer.bodies)
	require.NotContains(t, strings.Join(doer.bodies, ""), "1002")
}
