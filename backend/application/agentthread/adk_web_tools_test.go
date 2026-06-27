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

package agentthread

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/stretchr/testify/require"
)

func TestADKWebToolCatalogDisabledByDefault(t *testing.T) {
	catalog := NewADKWebToolCatalog(ADKWebToolCatalogOptions{})

	definitions, err := catalog.LoadADKRuntimeTools(
		context.Background(),
		&RunSummary{RunID: 20},
	)

	require.NoError(t, err)
	require.Empty(t, definitions)
}

func TestADKWebFetchToolRequiresAllowedHostAndRejectsPrivateIP(t *testing.T) {
	catalog := NewADKWebToolCatalog(ADKWebToolCatalogOptions{})
	provider := NewADKRuntimeToolCatalogProvider(catalog)
	run := &RunSummary{
		RunID: 20,
		Config: `{
			"web_tools":{
				"enabled":true,
				"http":{
					"enabled":true,
					"allow_http":true,
					"allowed_hosts":["127.0.0.1"]
				}
			}
		}`,
	}
	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	fetchTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		adkWebFetchToolName,
	)

	_, err = fetchTool.InvokableRun(
		context.Background(),
		`{"url":"http://127.0.0.1/private"}`,
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "private ip")
	require.NotContains(t, err.Error(), "/private")
}

func TestADKWebFetchToolReturnsBoundedStructuredContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(strings.Repeat("a", 1100)))
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	run := &RunSummary{
		RunID: 20,
		Config: fmt.Sprintf(`{
			"web_tools":{
				"enabled":true,
				"http":{
					"enabled":true,
					"allow_http":true,
					"allow_private_ips":true,
					"allowed_hosts":[%q],
					"max_response_bytes":1024,
					"timeout_ms":1000
				}
			}
		}`, serverURL.Hostname()),
	}
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKWebToolCatalog(ADKWebToolCatalogOptions{}),
	)
	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	fetchTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		adkWebFetchToolName,
	)

	result, err := fetchTool.InvokableRun(
		context.Background(),
		fmt.Sprintf(`{"url":%q}`, server.URL+"/doc"),
	)

	require.NoError(t, err)
	var payload ADKWebFetchResponse
	require.NoError(t, json.Unmarshal([]byte(result), &payload))
	require.Equal(t, adkWebFetchSchema, payload.Schema)
	require.Equal(t, serverURL.Hostname(), payload.Host)
	require.Equal(t, http.StatusOK, payload.StatusCode)
	require.Equal(t, "text/plain; charset=utf-8", payload.ContentType)
	require.True(t, payload.Truncated)
	require.Len(t, payload.Body, 1024)
	require.NotContains(t, result, "/doc")
}

func TestADKWebFetchToolRejectsRedirectToDisallowedHost(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		http.Redirect(
			w,
			&http.Request{},
			"http://redirect.example.com/secret",
			http.StatusFound,
		)
	}))
	defer source.Close()
	sourceURL, err := url.Parse(source.URL)
	require.NoError(t, err)
	run := &RunSummary{
		RunID: 20,
		Config: fmt.Sprintf(`{
			"web_tools":{
				"enabled":true,
				"http":{
					"enabled":true,
					"allow_http":true,
					"allow_private_ips":true,
					"allowed_hosts":[%q]
				}
			}
		}`, sourceURL.Hostname()),
	}
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKWebToolCatalog(ADKWebToolCatalogOptions{}),
	)
	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	fetchTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		adkWebFetchToolName,
	)

	_, err = fetchTool.InvokableRun(
		context.Background(),
		fmt.Sprintf(`{"url":%q}`, source.URL+"/start"),
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "redirect target is not allowed")
	require.NotContains(t, err.Error(), "/secret")
}

func TestADKWebSearchToolUsesBoundedBackend(t *testing.T) {
	backend := ADKWebSearchBackendFunc(func(
		_ context.Context,
		request ADKWebSearchRequest,
	) (*ADKWebSearchResponse, error) {
		require.Equal(t, "coze studio", request.Query)
		require.Equal(t, 3, request.MaxResults)
		return &ADKWebSearchResponse{
			Schema: adkWebSearchSchema,
			Results: []ADKWebSearchResult{
				{
					Title:   "Coze Studio",
					URL:     "https://example.com/coze",
					Snippet: "Open agent platform",
					Source:  "test",
				},
			},
		}, nil
	})
	run := &RunSummary{
		RunID: 20,
		Config: `{
			"web_tools":{
				"enabled":true,
				"search":{
					"enabled":true,
					"max_results":3
				}
			}
		}`,
	}
	provider := NewADKRuntimeToolCatalogProvider(
		NewADKWebToolCatalog(ADKWebToolCatalogOptions{
			SearchBackend: backend,
		}),
	)
	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	searchTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		adkWebSearchToolName,
	)

	result, err := searchTool.InvokableRun(
		context.Background(),
		`{"query":"coze studio","max_results":99}`,
	)

	require.NoError(t, err)
	require.JSONEq(t, `{
		"schema":"coze.web_search.v1",
		"results":[{
			"title":"Coze Studio",
			"url":"https://example.com/coze",
			"snippet":"Open agent platform",
			"source":"test"
		}]
	}`, result)
}

func TestDefaultADKToolProviderExposesWebSearchWithInjectedBackend(t *testing.T) {
	backend := ADKWebSearchBackendFunc(func(
		_ context.Context,
		request ADKWebSearchRequest,
	) (*ADKWebSearchResponse, error) {
		require.Equal(t, "deerflow parity", request.Query)
		require.Equal(t, 2, request.MaxResults)
		return &ADKWebSearchResponse{
			Results: []ADKWebSearchResult{
				{
					Title:   "Parity",
					URL:     "https://example.com/parity",
					Snippet: "Search result",
					Source:  "test",
				},
			},
		}, nil
	})
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(
		nil,
		WithDefaultADKToolProviderWebSearchBackend(backend),
	)
	toolSetProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)
	run := &RunSummary{
		RunID: 20,
		Config: `{
			"web_tools":{
				"enabled":true,
				"search":{
					"enabled":true,
					"max_results":2
				}
			}
		}`,
	}

	set, err := toolSetProvider.ResolveToolSet(context.Background(), run)

	require.NoError(t, err)
	searchTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		adkWebSearchToolName,
	)
	result, err := searchTool.InvokableRun(
		context.Background(),
		`{"query":"deerflow parity","max_results":5}`,
	)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"schema":"coze.web_search.v1",
		"results":[{
			"title":"Parity",
			"url":"https://example.com/parity",
			"snippet":"Search result",
			"source":"test"
		}]
	}`, result)
}

func TestADKHTTPWebSearchBackendPostsBoundedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		var request ADKWebSearchRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		require.Equal(t, "coze web", request.Query)
		require.Equal(t, 2, request.MaxResults)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"schema":"ignored",
			"results":[{
				"title":"Result",
				"url":"https://example.com/1",
				"snippet":"Summary"
			}]
		}`))
	}))
	defer server.Close()

	backend, err := NewADKHTTPWebSearchBackend(
		ADKHTTPWebSearchBackendOptions{
			Endpoint:        server.URL + "/search",
			APIKey:          "secret-token",
			AllowHTTP:       true,
			AllowPrivateIPs: true,
			Source:          "http-provider",
		},
	)
	require.NoError(t, err)

	response, err := backend.SearchADKWeb(
		context.Background(),
		ADKWebSearchRequest{Query: "coze web", MaxResults: 2},
	)

	require.NoError(t, err)
	require.Equal(t, []ADKWebSearchResult{
		{
			Title:   "Result",
			URL:     "https://example.com/1",
			Snippet: "Summary",
			Source:  "http-provider",
		},
	}, response.Results)
}

func TestADKHTTPWebSearchBackendSanitizesErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		http.Error(w, "secret response body", http.StatusBadGateway)
	}))
	defer server.Close()

	backend, err := NewADKHTTPWebSearchBackend(
		ADKHTTPWebSearchBackendOptions{
			Endpoint:        server.URL + "/secret-provider-path",
			APIKey:          "sk-secret",
			AllowHTTP:       true,
			AllowPrivateIPs: true,
		},
	)
	require.NoError(t, err)

	_, err = backend.SearchADKWeb(
		context.Background(),
		ADKWebSearchRequest{Query: "private query", MaxResults: 1},
	)

	require.Error(t, err)
	require.NotContains(t, err.Error(), "private query")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "secret response body")
	require.NotContains(t, err.Error(), "secret-provider-path")
}

func TestADKHTTPWebSearchBackendRejectsRedirectToDisallowedHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		http.Redirect(
			w,
			r,
			"http://redirect.example.com/secret",
			http.StatusFound,
		)
	}))
	defer server.Close()

	backend, err := NewADKHTTPWebSearchBackend(
		ADKHTTPWebSearchBackendOptions{
			Endpoint:        server.URL + "/search",
			AllowHTTP:       true,
			AllowPrivateIPs: true,
		},
	)
	require.NoError(t, err)

	_, err = backend.SearchADKWeb(
		context.Background(),
		ADKWebSearchRequest{Query: "redirect query", MaxResults: 1},
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "redirect target is not allowed")
	require.NotContains(t, err.Error(), "redirect query")
	require.NotContains(t, err.Error(), "redirect.example.com")
	require.NotContains(t, err.Error(), "/secret")
}

func TestADKWebSearchBackendFromEnvBuildsHTTPBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		require.Equal(t, "env-secret", r.Header.Get("X-Search-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"results":[{
				"title":"Env Result",
				"url":"https://example.com/env",
				"snippet":"Configured through env"
			}]
		}`))
	}))
	defer server.Close()
	resetADKWebSearchEnv(t)
	t.Setenv(agentThreadWebSearchEnabledEnv, "true")
	t.Setenv(agentThreadWebSearchEndpointEnv, server.URL+"/search")
	t.Setenv(agentThreadWebSearchAPIKeyEnv, "env-secret")
	t.Setenv(agentThreadWebSearchHeaderEnv, "X-Search-Key")
	t.Setenv(agentThreadWebSearchAllowHTTPEnv, "true")
	t.Setenv(agentThreadWebSearchAllowPrivateIPsEnv, "true")
	t.Setenv(agentThreadWebSearchMaxResponseBytesEnv, "2048")

	backend, enabled, err := ADKWebSearchBackendFromEnv()
	require.NoError(t, err)
	require.True(t, enabled)

	response, err := backend.SearchADKWeb(
		context.Background(),
		ADKWebSearchRequest{Query: "env query", MaxResults: 1},
	)

	require.NoError(t, err)
	require.Equal(t, "Env Result", response.Results[0].Title)
	require.Equal(t, "coze_env_http", response.Results[0].Source)
}

func TestADKWebSearchBackendFromEnvDisabledByDefault(t *testing.T) {
	resetADKWebSearchEnv(t)

	backend, enabled, err := ADKWebSearchBackendFromEnv()

	require.NoError(t, err)
	require.False(t, enabled)
	require.Nil(t, backend)
}

func TestADKWebSearchBackendFromEnvReportsInvalidConfigSafely(t *testing.T) {
	resetADKWebSearchEnv(t)
	t.Setenv(agentThreadWebSearchEnabledEnv, "true")
	t.Setenv(agentThreadWebSearchEndpointEnv, "https://user:pass@example.com/search")
	t.Setenv(agentThreadWebSearchAPIKeyEnv, "sk-secret-token")

	backend, enabled, err := ADKWebSearchBackendFromEnv()

	require.Error(t, err)
	require.True(t, enabled)
	require.Nil(t, backend)
	require.Contains(t, err.Error(), "web search endpoint must not include userinfo")
	require.NotContains(t, err.Error(), "user:pass")
	require.NotContains(t, err.Error(), "sk-secret-token")
}

func resetADKWebSearchEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		agentThreadWebSearchEnabledEnv,
		agentThreadWebSearchEndpointEnv,
		agentThreadWebSearchAPIKeyEnv,
		agentThreadWebSearchHeaderEnv,
		agentThreadWebSearchTimeoutMsEnv,
		agentThreadWebSearchMaxResponseBytesEnv,
		agentThreadWebSearchAllowHTTPEnv,
		agentThreadWebSearchAllowPrivateIPsEnv,
	} {
		t.Setenv(key, "")
	}
}

func TestDefaultADKToolProviderExposesWebFetchWhenConfigured(t *testing.T) {
	provider := NewDefaultADKToolProvider()
	toolSetProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)
	run := &RunSummary{
		RunID: 20,
		Config: `{
			"webTools":{
				"enabled":true,
				"http":{
					"enabled":true,
					"allowed_hosts":["docs.example.com"]
				}
			}
		}`,
	}

	set, err := toolSetProvider.ResolveToolSet(context.Background(), run)

	require.NoError(t, err)
	names := adkToolNames(t, context.Background(), set.StaticTools)
	require.Contains(t, names, adkWebFetchToolName)
	require.Contains(t, names, adkClarificationToolName)
	require.Contains(t, names, adkConfirmationToolName)
}

func requireADKInvokableTool(
	t *testing.T,
	ctx context.Context,
	tools []tool.BaseTool,
	name string,
) tool.InvokableTool {
	t.Helper()
	for _, candidate := range tools {
		info, err := candidate.Info(ctx)
		require.NoError(t, err)
		if info.Name != name {
			continue
		}
		invokable, ok := candidate.(tool.InvokableTool)
		require.True(t, ok)
		return invokable
	}
	require.FailNowf(t, "tool not found", "missing tool %s", name)
	return nil
}

func adkToolNames(
	t *testing.T,
	ctx context.Context,
	tools []tool.BaseTool,
) []string {
	t.Helper()
	names := make([]string, 0, len(tools))
	for _, candidate := range tools {
		info, err := candidate.Info(ctx)
		require.NoError(t, err)
		names = append(names, info.Name)
	}
	return names
}
