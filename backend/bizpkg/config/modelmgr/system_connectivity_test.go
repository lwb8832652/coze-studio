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

package modelmgr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
)

func TestSystemModelEndpointProbeExecutesRequestedOpenAIModel(t *testing.T) {
	type observedRequest struct {
		Method        string
		Path          string
		Authorization string
		Body          map[string]any
	}
	observed := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet && request.URL.Path == "/v1/models" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		observed <- observedRequest{
			Method: request.Method, Path: request.URL.Path,
			Authorization: request.Header.Get("Authorization"), Body: body,
		}
		http.Error(writer, `{"error":{"message":"Model not exist: requested-model"}}`, http.StatusNotFound)
	}))
	defer server.Close()

	result, err := (&ModelConfig{}).TestSystemModelEndpoint(context.Background(), &config.TestModelEndpointReq{
		ProviderKey: "qwen", ModelIdentifier: "requested-model", Protocol: "openai-compatible",
		Endpoint: &config.ModelEndpointInput{BaseURL: server.URL + "/v1", APIKey: ptr.Of("test-secret")},
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "model_not_found", requireSystemModelTestString(t, result.ErrorCode))
	require.NotContains(t, requireSystemModelTestString(t, result.ErrorMessage), "requested-model")

	request := <-observed
	require.Equal(t, http.MethodPost, request.Method)
	require.Equal(t, "/v1/chat/completions", request.Path)
	require.Equal(t, "Bearer test-secret", request.Authorization)
	require.Equal(t, "requested-model", request.Body["model"])
	require.Equal(t, float64(1), request.Body["max_tokens"])
	require.Equal(t, false, request.Body["stream"])
}

func TestSystemModelEndpointProbeMapsProviderErrorsWithoutLeakingBody(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{status: http.StatusUnauthorized, code: "invalid_credential"},
		{status: http.StatusForbidden, code: "permission_denied"},
		{status: http.StatusNotFound, code: "model_not_found"},
		{status: http.StatusTooManyRequests, code: "rate_limited"},
		{status: http.StatusBadGateway, code: "provider_unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				http.Error(writer, "upstream leaked-secret-value", tt.status)
			}))
			defer server.Close()

			result, err := (&ModelConfig{}).TestSystemModelEndpoint(context.Background(), &config.TestModelEndpointReq{
				ProviderKey: "openai", ModelIdentifier: "gpt-test", Protocol: "openai-compatible",
				Endpoint: &config.ModelEndpointInput{BaseURL: server.URL, APIKey: ptr.Of("test-secret")},
			})
			require.NoError(t, err)
			require.False(t, result.Success)
			require.Equal(t, tt.code, requireSystemModelTestString(t, result.ErrorCode))
			require.False(t, strings.Contains(requireSystemModelTestString(t, result.ErrorMessage), "leaked-secret-value"))
		})
	}
}

func TestSystemModelEndpointProbeUsesStoredWriteOnlyCredential(t *testing.T) {
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization <- request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	draft := newSystemModelTestInput("stored-secret")
	draft.Endpoints[0].BaseURL = server.URL + "/v1"
	modelID, err := cfg.UpsertSystemModel(ctx, 9, nil, draft)
	require.NoError(t, err)
	detail, err := cfg.GetSystemModelDetail(ctx, modelID)
	require.NoError(t, err)

	result, err := cfg.TestSystemModelEndpoint(ctx, &config.TestModelEndpointReq{
		ModelID: &modelID, ProviderKey: "deepseek", ModelIdentifier: draft.ModelIdentifier,
		Protocol: draft.Protocol,
		Endpoint: &config.ModelEndpointInput{
			ID: ptr.Of(detail.Endpoints[0].ID), BaseURL: detail.Endpoints[0].BaseURL,
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "Bearer stored-secret", <-authorization)
}

func TestSystemModelEndpointProbeUsesStoredAzureRuntimeOptions(t *testing.T) {
	type observedRequest struct {
		Path          string
		RawQuery      string
		APIKey        string
		Authorization string
	}
	observed := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observed <- observedRequest{
			Path: request.URL.Path, RawQuery: request.URL.RawQuery,
			APIKey: request.Header.Get("api-key"), Authorization: request.Header.Get("Authorization"),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	cfg.ModelMetaConf.Provider2Models[developer_api.ModelClass_GPT.String()] = map[string]ModelMeta{
		"default": {
			DisplayInfo: &config.DisplayInfo{Name: "Azure OpenAI"},
			Connection:  &config.Connection{BaseConnInfo: &config.BaseConnectionInfo{}},
			Capability:  &developer_api.ModelAbility{},
		},
	}
	draft := newSystemModelTestInput("azure-secret")
	draft.ProviderKey = "openai"
	draft.Name = "Azure OpenAI"
	draft.ModelIdentifier = "gpt-4o:test"
	draft.Endpoints[0].BaseURL = server.URL
	draft.ProviderOptions = &config.ModelProviderOptions{
		OpenaiByAzure: ptr.Of(true), OpenaiAPIVersion: ptr.Of("2025-04-01-preview"),
	}
	modelID, err := cfg.UpsertSystemModel(ctx, 9, nil, draft)
	require.NoError(t, err)
	detail, err := cfg.GetSystemModelDetail(ctx, modelID)
	require.NoError(t, err)

	result, err := cfg.TestSystemModelEndpoint(ctx, &config.TestModelEndpointReq{
		ModelID: &modelID, ProviderKey: "openai", ModelIdentifier: draft.ModelIdentifier,
		Protocol: draft.Protocol,
		Endpoint: &config.ModelEndpointInput{
			ID: ptr.Of(detail.Endpoints[0].ID), BaseURL: detail.Endpoints[0].BaseURL,
		},
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	request := <-observed
	require.Equal(t, "/openai/deployments/gpt-4otest/chat/completions", request.Path)
	require.Equal(t, "api-version=2025-04-01-preview", request.RawQuery)
	require.Equal(t, "azure-secret", request.APIKey)
	require.Empty(t, request.Authorization)

	overrideResult, err := cfg.TestSystemModelEndpoint(ctx, &config.TestModelEndpointReq{
		ModelID: &modelID, ProviderKey: "openai", ModelIdentifier: draft.ModelIdentifier,
		Protocol: draft.Protocol,
		ProviderOptions: &config.ModelProviderOptions{
			OpenaiByAzure: ptr.Of(false), OpenaiAPIVersion: ptr.Of("2025-06-01"),
		},
		Endpoint: &config.ModelEndpointInput{
			ID: ptr.Of(detail.Endpoints[0].ID), BaseURL: detail.Endpoints[0].BaseURL,
		},
	})
	require.NoError(t, err)
	require.True(t, overrideResult.Success)

	overrideRequest := <-observed
	require.Equal(t, "/chat/completions", overrideRequest.Path)
	require.Empty(t, overrideRequest.RawQuery)
	require.Empty(t, overrideRequest.APIKey)
	require.Equal(t, "Bearer azure-secret", overrideRequest.Authorization)
}

func TestSystemModelEndpointProbePrefersPendingAzureRuntimeOptions(t *testing.T) {
	type observedRequest struct {
		Path          string
		RawQuery      string
		APIKey        string
		Authorization string
	}
	observed := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observed <- observedRequest{
			Path: request.URL.Path, RawQuery: request.URL.RawQuery,
			APIKey: request.Header.Get("api-key"), Authorization: request.Header.Get("Authorization"),
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer server.Close()

	result, err := (&ModelConfig{}).TestSystemModelEndpoint(context.Background(), &config.TestModelEndpointReq{
		ProviderKey: "openai", ModelIdentifier: "gpt-4o:pending", Protocol: "openai-compatible",
		ProviderOptions: &config.ModelProviderOptions{
			OpenaiByAzure: ptr.Of(true), OpenaiAPIVersion: ptr.Of("2025-06-01"),
		},
		Endpoint: &config.ModelEndpointInput{BaseURL: server.URL, APIKey: ptr.Of("pending-secret")},
	})
	require.NoError(t, err)
	require.True(t, result.Success)

	request := <-observed
	require.Equal(t, "/openai/deployments/gpt-4opending/chat/completions", request.Path)
	require.Equal(t, "api-version=2025-06-01", request.RawQuery)
	require.Equal(t, "pending-secret", request.APIKey)
	require.Empty(t, request.Authorization)
}

func TestSystemModelEndpointProbeMapsNetworkFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	result, err := (&ModelConfig{}).TestSystemModelEndpoint(context.Background(), &config.TestModelEndpointReq{
		ProviderKey: "qwen", ModelIdentifier: "qwen-test", Protocol: "openai-compatible",
		Endpoint: &config.ModelEndpointInput{BaseURL: baseURL, APIKey: ptr.Of("test-secret")},
	})
	require.NoError(t, err)
	require.False(t, result.Success)
	require.Equal(t, "network_error", requireSystemModelTestString(t, result.ErrorCode))
}

func requireSystemModelTestString(t *testing.T, value *string) string {
	t.Helper()
	require.NotNil(t, value)
	return *value
}
