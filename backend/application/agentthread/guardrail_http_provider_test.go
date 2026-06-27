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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPGuardrailProviderPostsMetadataOnlyRequestAndSanitizesDecision(t *testing.T) {
	var gotHeader string
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		_, _ = w.Write([]byte(`{
			"action": "confirm",
			"provider": "external scanner /mnt/raw",
			"reason_code": "network review",
			"message": "review sk-secret from /mnt/raw/object",
			"rule_ids": [" url review ", "raw/prompt"],
			"metadata": {
				"target_type": "network",
				"safe_count": "2",
				"raw_prompt": "secret prompt",
				"object_uri": "s3://bucket/raw"
			}
		}`))
	}))
	defer server.Close()
	provider, err := NewHTTPGuardrailProvider(HTTPGuardrailProviderOptions{
		Endpoint: server.URL,
		Token:    "guardrail-token",
		Timeout:  time.Second,
	})
	require.NoError(t, err)

	decision, err := provider.EvaluateGuardrail(
		context.Background(),
		GuardrailRequest{
			SpaceID:    30,
			ThreadID:   10,
			RunID:      20,
			UserID:     40,
			TargetType: GuardrailTargetNetwork,
			TargetID:   "web_fetch",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   GuardrailFailClosed,
			Metadata: map[string]string{
				"domain":           "example.com",
				"raw_prompt":       "secret prompt sk-secret",
				"checkpoint_bytes": strings.Repeat("a", 128),
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, "Bearer guardrail-token", gotHeader)
	require.Equal(t, "coze.guardrail_scan_request.v1", got["schema"])
	require.Equal(t, float64(30), got["space_id"])
	require.Equal(t, float64(10), got["thread_id"])
	require.Equal(t, float64(20), got["run_id"])
	require.Equal(t, float64(40), got["user_id"])
	require.Equal(t, "network", got["target_type"])
	require.Equal(t, "web_fetch", got["target_id"])
	require.Equal(t, "invoke", got["operation"])
	require.Equal(t, "adk_runtime_tool", got["source"])
	require.Equal(t, "fail_closed", got["fail_mode"])
	require.Equal(t, map[string]any{"domain": "example.com"}, got["metadata"])
	require.NotContains(t, got, "prompt")
	require.NotContains(t, got, "tool_arguments")
	require.NotContains(t, got, "checkpoint_bytes")
	require.NotContains(t, got, "object_uri")

	require.Equal(t, GuardrailActionConfirm, decision.Action)
	require.Equal(t, "external_scanner_mnt_raw", decision.Provider)
	require.Equal(t, "network_review", decision.ReasonCode)
	require.Equal(t, "guardrail decision requires review", decision.Message)
	require.Equal(t, []string{"url_review", "raw_prompt"}, decision.RuleIDs)
	require.Equal(t, map[string]string{
		"safe_count":  "2",
		"target_type": "network",
	}, decision.Metadata)
}

func TestHTTPGuardrailProviderRejectsInvalidResponseWithoutLeakingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"action":"allow","message":"secret sk-secret"}`))
	}))
	defer server.Close()
	provider, err := NewHTTPGuardrailProvider(HTTPGuardrailProviderOptions{
		Endpoint: server.URL,
		Timeout:  time.Second,
	})
	require.NoError(t, err)

	decision, err := provider.EvaluateGuardrail(
		context.Background(),
		GuardrailRequest{
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:search_docs",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   GuardrailFailClosed,
		},
	)

	require.Error(t, err)
	require.Equal(t, GuardrailDecision{}, decision)
	require.Contains(t, err.Error(), "guardrail scanner response action is invalid")
	require.NotContains(t, err.Error(), "sk-secret")
}

func TestGuardrailProviderFromEnvBuildsHTTPProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer env-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"action":"deny","provider":"scanner","reason_code":"blocked"}`))
	}))
	defer server.Close()
	t.Setenv(agentGuardrailProviderTypeEnv, "http")
	t.Setenv(agentGuardrailHTTPURLEnv, server.URL)
	t.Setenv(agentGuardrailHTTPTokenEnv, "env-token")
	t.Setenv(agentGuardrailHTTPTimeoutMsEnv, "1000")

	provider, status := NewGuardrailProviderFromEnvWithStatus()

	require.True(t, status.Enabled)
	require.True(t, status.Configured)
	require.Equal(t, "http", status.Type)
	require.Empty(t, status.Error)
	require.NotNil(t, provider)
	decision, err := provider.EvaluateGuardrail(
		context.Background(),
		GuardrailRequest{
			TargetType: GuardrailTargetToolCall,
			TargetID:   "runtime_tool:search_docs",
			Operation:  "invoke",
			Source:     "adk_runtime_tool",
			FailMode:   GuardrailFailClosed,
		},
	)
	require.NoError(t, err)
	require.Equal(t, GuardrailActionDeny, decision.Action)
	require.Equal(t, "blocked", decision.ReasonCode)
}

func TestGuardrailProviderFromEnvReportsInvalidHTTPConfigSafely(t *testing.T) {
	t.Setenv(agentGuardrailProviderTypeEnv, "http")
	t.Setenv(agentGuardrailHTTPURLEnv, "https://user:pass@example.com/scan")
	t.Setenv(agentGuardrailHTTPTokenEnv, "sk-secret-token")

	provider, status := NewGuardrailProviderFromEnvWithStatus()

	require.Nil(t, provider)
	require.True(t, status.Enabled)
	require.False(t, status.Configured)
	require.Equal(t, "http", status.Type)
	require.Contains(t, status.Error, "guardrail scanner endpoint must not include userinfo")
	require.NotContains(t, status.Error, "sk-secret-token")
	require.NotContains(t, status.Error, "user:pass")
}
