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

type artifactPublishHTTPDoer func(*http.Request) (*http.Response, error)

func (doer artifactPublishHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	return doer(request)
}

func TestRemoteProviderPublishesArtifactOverAuthenticatedBoundedControlWire(t *testing.T) {
	const providerCredential = "provider-credential"
	const uploadToken = "artifact-upload-token"
	var called int
	provider, err := newRemoteProviderWithDoer(RemoteProviderConfig{
		Endpoint: "https://runner.example.com", Credential: providerCredential,
		AllowedHosts: []string{"runner.example.com"}, Timeout: time.Minute,
	}, artifactPublishHTTPDoer(func(request *http.Request) (*http.Response, error) {
		called++
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, "/v1/executions/execution-1/artifacts:publish", request.URL.Path)
		require.Equal(t, "Bearer "+providerCredential, request.Header.Get("Authorization"))
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		var wire map[string]any
		require.NoError(t, json.Unmarshal(body, &wire))
		require.Equal(t, ArtifactPublishSchemaV1, wire["schema"])
		require.Equal(t, uploadToken, wire["upload_token"])
		require.NotContains(t, string(body), "object_key")
		responseBody := `{"schema":"coze.sandbox.artifact_publish.v1","accepted":true}`
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(responseBody)), ContentLength: int64(len(responseBody)),
		}, nil
	}))
	require.NoError(t, err)

	result, err := provider.PublishArtifact(context.Background(), "execution-1", ArtifactPublishRequest{
		Descriptor: ArtifactDescriptor{Kind: ArtifactKindAppDevBuildArchive, Digest: "sha256:" + strings.Repeat("c", 64), Size: 4096},
		UploadURL:  "https://gateway.example/internal/grants/grant-1", Token: uploadToken,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	require.Equal(t, 1, called)
}

func TestRemoteProviderArtifactPublishPreservesContextAndHidesTransportSecrets(t *testing.T) {
	called := 0
	doer := artifactPublishHTTPDoer(func(*http.Request) (*http.Response, error) {
		called++
		return nil, errors.New("raw transport artifact-upload-token")
	})
	provider, err := newRemoteProviderWithDoer(RemoteProviderConfig{
		Endpoint: "https://runner.example.com", Credential: "provider-credential",
		AllowedHosts: []string{"runner.example.com"}, Timeout: time.Minute,
	}, doer)
	require.NoError(t, err)
	request := ArtifactPublishRequest{
		Descriptor: ArtifactDescriptor{Kind: ArtifactKindAppDevBuildArchive, Digest: "sha256:" + strings.Repeat("d", 64), Size: 1},
		UploadURL:  "https://gateway.example/internal/grants/grant-1", Token: "artifact-upload-token",
		ExpiresAt: time.Now().Add(time.Minute),
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.PublishArtifact(canceled, "execution-1", request)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, called)

	_, err = provider.PublishArtifact(context.Background(), "execution-1", request)
	require.ErrorIs(t, err, domainsandbox.ErrProviderUnhealthy)
	require.NotContains(t, err.Error(), "artifact-upload-token")

	provider.doer = artifactPublishHTTPDoer(func(*http.Request) (*http.Response, error) {
		body := strings.Repeat("x", MaxHealthResponseBodyBytes+1)
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)),
		}, nil
	})
	_, err = provider.PublishArtifact(context.Background(), "execution-1", request)
	require.ErrorIs(t, err, domainsandbox.ErrProviderUnhealthy)
}
