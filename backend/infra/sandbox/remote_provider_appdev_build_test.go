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

package sandbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestRemoteProviderAppDevBuildBeginAndStatusWire(t *testing.T) {
	const credential = "provider-build-credential"
	const operationID = "build-operation-001"
	digest := "sha256:" + strings.Repeat("a", 64)
	requests := 0
	provider, err := newRemoteProviderWithDoer(RemoteProviderConfig{
		Endpoint: "https://runner.example.com", Credential: credential,
		AllowedHosts: []string{"runner.example.com"}, Timeout: time.Minute,
	}, artifactPublishHTTPDoer(func(request *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, "Bearer "+credential, request.Header.Get("Authorization"))
		var responseBody string
		switch requests {
		case 1:
			require.Equal(t, http.MethodPost, request.Method)
			require.Equal(t, "/v1/executions/execution-1/builds", request.URL.Path)
			body, readErr := io.ReadAll(request.Body)
			require.NoError(t, readErr)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(body, &wire))
			require.Equal(t, AppDevBuildSchemaV1, wire["schema"])
			require.Equal(t, operationID, wire["operation_id"])
			require.NotContains(t, string(body), "object_key")
			require.NotContains(t, string(body), "uri")
			responseBody = `{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build-operation-001","status":"accepted"}`
		case 2:
			require.Equal(t, http.MethodGet, request.Method)
			require.Equal(t, "/v1/executions/execution-1/builds/build-operation-001", request.URL.Path)
			responseBody = `{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build-operation-001","status":"descriptor_ready","descriptor":{"kind":"appdev_build_archive","digest":"` + digest + `","size":4096}}`
		default:
			t.Fatalf("unexpected request %d", requests)
		}
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(responseBody)), ContentLength: int64(len(responseBody)),
		}, nil
	}))
	require.NoError(t, err)

	begin, err := provider.BeginBuild(context.Background(), "execution-1", operationID)
	require.NoError(t, err)
	require.Equal(t, BuildStatusAccepted, begin.Status)
	status, err := provider.BuildStatus(context.Background(), "execution-1", operationID)
	require.NoError(t, err)
	require.Equal(t, BuildStatusDescriptorReady, status.Status)
	require.Equal(t, ArtifactDescriptor{Kind: ArtifactKindAppDevBuildArchive, Digest: digest, Size: 4096}, status.Descriptor)
	require.Equal(t, 2, requests)
}

func TestRemoteProviderAppDevBuildFailsClosedAndRedactsErrors(t *testing.T) {
	called := 0
	provider, err := newRemoteProviderWithDoer(RemoteProviderConfig{
		Endpoint: "https://runner.example.com", Credential: "provider-build-credential",
		AllowedHosts: []string{"runner.example.com"}, Timeout: time.Minute,
	}, artifactPublishHTTPDoer(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("raw provider-build-credential object://secret")
	}))
	require.NoError(t, err)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.BeginBuild(canceled, "execution-1", "build-operation-001")
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, called)

	_, err = provider.BeginBuild(context.Background(), "execution-1", "build-operation-001")
	require.ErrorIs(t, err, domainsandbox.ErrProviderUnhealthy)
	require.NotContains(t, err.Error(), "provider-build-credential")
	require.NotContains(t, err.Error(), "object://secret")

	tests := []string{
		`{"schema":"coze.sandbox.appdev_build.v1","operation_id":"wrong-operation","status":"accepted"}`,
		`{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build-operation-001","status":"descriptor_ready","descriptor":{"kind":"appdev_build_archive","digest":"sha256:bad","size":1}}`,
		`{"schema":"coze.sandbox.appdev_build.v1","operation_id":"build-operation-001","status":"accepted","object_key":"internal/secret"}`,
	}
	for _, body := range tests {
		provider.doer = artifactPublishHTTPDoer(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)),
			}, nil
		})
		_, err = provider.BuildStatus(context.Background(), "execution-1", "build-operation-001")
		require.ErrorIs(t, err, domainsandbox.ErrProviderUnhealthy)
		require.NotContains(t, err.Error(), "internal/secret")
	}

	provider.doer = artifactPublishHTTPDoer(func(*http.Request) (*http.Response, error) {
		body := strings.Repeat("x", MaxHealthResponseBodyBytes+1)
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)),
		}, nil
	})
	_, err = provider.BuildStatus(context.Background(), "execution-1", "build-operation-001")
	require.ErrorIs(t, err, domainsandbox.ErrProviderUnhealthy)

	_, err = provider.BuildStatus(context.Background(), "execution-1", "../traversal")
	require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
}

var _ AppDevBuildExecutor = (*RemoteProvider)(nil)
