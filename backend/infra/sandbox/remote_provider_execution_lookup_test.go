// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func sortedJSONKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestRemoteProviderLookupExecutionUsesStrictOperationDigestWire(t *testing.T) {
	digest := ExecutionRequestDigest(sha256.Sum256([]byte("canonical launch request")))
	var captured map[string]any
	provider, err := newRemoteProviderWithDoer(validRemoteProviderConfig(), providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/v1/executions:lookup", request.URL.Path)
		require.Equal(t, "Bearer synthetic-test-token", request.Header.Get("Authorization"))
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.NoError(t, json.Unmarshal(body, &captured))
		require.Equal(t, []string{"operation_id", "request_digest", "schema", "scope", "workload_kind"}, sortedJSONKeys(captured))
		require.Equal(t, ExecutionLookupSchemaV1, captured["schema"])
		require.Equal(t, "provider-operation-stable", captured["operation_id"])
		require.Equal(t, digest.Hex(), captured["request_digest"])
		require.NotContains(t, string(body), "grant")
		require.NotContains(t, string(body), "token")
		response := `{"schema":"coze.sandbox.execution_lookup.v1","status":"found","execution":{"schema":"coze.sandbox.execute.v1","execution_id":"provider-execution-real","status":"running","exit_code":null,"stdout":"","stderr":"","artifacts":[],"artifact_descriptors":[],"preview_route":"/preview/opaque"}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				ExecutionLookupProtocolVersionHeader: []string{ExecutionLookupProtocolVersionV1},
				"Content-Type":                       []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(response)),
		}, nil
	}))
	require.NoError(t, err)

	result, err := provider.LookupExecution(context.Background(), ExecutionLookupRequest{
		Scope: domainsandbox.ScopeAppDev, WorkloadKind: WorkloadAppDev,
		OperationID: "provider-operation-stable", RequestDigest: digest,
	})
	require.NoError(t, err)
	require.Equal(t, ExecutionLookupFound, result.Status)
	require.Equal(t, "provider-execution-real", result.Execution.ExecutionID)
	require.Equal(t, ExecutionStatusRunning, result.Execution.Status)
}

func TestRemoteProviderLookupExecutionOnlyAcceptsVersionedAuthoritativeNotFound(t *testing.T) {
	digest := ExecutionRequestDigest(sha256.Sum256([]byte("canonical launch request")))
	tests := []struct {
		name       string
		header     string
		body       string
		wantStatus ExecutionLookupStatus
		wantErr    error
	}{
		{
			name: "authoritative not found", header: ExecutionLookupProtocolVersionV1,
			body:       `{"schema":"coze.sandbox.execution_lookup.v1","status":"not_found","execution":null}`,
			wantStatus: ExecutionLookupNotFound,
		},
		{
			name:       "missing version is unknown",
			body:       `{"schema":"coze.sandbox.execution_lookup.v1","status":"not_found","execution":null}`,
			wantStatus: ExecutionLookupUnknown, wantErr: domainsandbox.ErrUnavailable,
		},
		{
			name: "unknown status remains unknown", header: ExecutionLookupProtocolVersionV1,
			body:       `{"schema":"coze.sandbox.execution_lookup.v1","status":"unknown","execution":null}`,
			wantStatus: ExecutionLookupUnknown,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := newRemoteProviderWithDoer(validRemoteProviderConfig(), providerDoerFunc(func(*http.Request) (*http.Response, error) {
				header := make(http.Header)
				if test.header != "" {
					header.Set(ExecutionLookupProtocolVersionHeader, test.header)
				}
				header.Set("Content-Type", "application/json")
				return &http.Response{
					StatusCode: http.StatusOK, Header: header,
					Body: io.NopCloser(strings.NewReader(test.body)),
				}, nil
			}))
			require.NoError(t, err)
			result, lookupErr := provider.LookupExecution(context.Background(), ExecutionLookupRequest{
				Scope: domainsandbox.ScopeAppDev, WorkloadKind: WorkloadAppDev,
				OperationID: "provider-operation-stable", RequestDigest: digest,
			})
			require.Equal(t, test.wantStatus, result.Status)
			if test.wantErr == nil {
				require.NoError(t, lookupErr)
			} else {
				require.ErrorIs(t, lookupErr, test.wantErr)
			}
		})
	}
}

func TestRemoteProviderLookupExecutionLegacyIdentityUsesVersionedTenantScopedWire(t *testing.T) {
	var captured map[string]any
	provider, err := newRemoteProviderWithDoer(validRemoteProviderConfig(), providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/v1/executions:lookup", request.URL.Path)
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.NoError(t, json.Unmarshal(body, &captured))
		require.Equal(t, []string{
			"generation", "operation_id", "project_id", "schema", "scope", "space_id", "workload_kind",
		}, sortedJSONKeys(captured))
		require.Equal(t, ExecutionLookupLegacySchemaV1, captured["schema"])
		require.Equal(t, "1001", captured["space_id"])
		require.Equal(t, "project-a", captured["project_id"])
		require.Equal(t, float64(7), captured["generation"])
		require.NotContains(t, string(body), "request_digest")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				ExecutionLookupProtocolVersionHeader: []string{ExecutionLookupProtocolVersionV1},
				"Content-Type":                       []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(
				`{"schema":"coze.sandbox.execution_lookup.v1","status":"not_found","execution":null}`,
			)),
		}, nil
	}))
	require.NoError(t, err)

	result, err := provider.LookupExecution(context.Background(), ExecutionLookupRequest{
		Legacy: true, SpaceID: "1001", ProjectID: "project-a", Generation: 7,
		Scope: domainsandbox.ScopeAppDev, WorkloadKind: WorkloadAppDev,
		OperationID: "appdev_start_legacy_operation",
	})
	require.NoError(t, err)
	require.Equal(t, ExecutionLookupNotFound, result.Status)
}
