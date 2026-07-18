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
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	appmcptool "github.com/coze-dev/coze-studio/backend/application/mcptool"
	appworkbench "github.com/coze-dev/coze-studio/backend/application/workbench"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestWorkbenchRuntimeDoctorReturnsSafeSummary(t *testing.T) {
	t.Setenv("AGENT_THREAD_RUNTIME_DEFAULT", "eino_adk")
	t.Setenv("AGENT_THREAD_EINO_ADK_ENABLED", "true")
	t.Setenv("AGENT_THREAD_WEB_SEARCH_ENABLED", "true")
	t.Setenv("AGENT_THREAD_WEB_SEARCH_ENDPOINT", "https://search.example.test/private/path")
	t.Setenv("AGENT_THREAD_WEB_SEARCH_API_KEY", "secret-search-key")
	t.Setenv("WORKBENCH_RUNTIME_DOCTOR_LIVE_MODEL_PROBE", "true")
	t.Setenv(consts.CodeRunnerType, "sandbox")
	t.Setenv(consts.CodeRunnerAllowNet, "search.example.test,private.internal")
	t.Setenv(consts.CodeRunnerNodeModulesDir, "/private/node_modules")
	installMCPToolTestService(t)
	appworkbench.InitService(&appworkbench.ServiceComponents{
		MCPToolSVC: appmcptool.SVC,
		ChatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, _ []*schema.Message) (*schema.Message, error) {
					return schema.AssistantMessage("hidden runtime probe response", nil), nil
				},
			}, true, nil
		},
	})
	identityContext := ctxcache.Init(context.Background())
	ctxcache.Store(identityContext, consts.SessionDataKeyInCtx, &userentity.Session{UserID: 7})

	_, err := appmcptool.SVC.UpsertServer(identityContext, &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     1,
		Name:        "secure-tools",
		Description: "Secure tool server",
		ServerType:  "streamable_http",
		Enabled:     true,
		Config:      `{"url":"https://mcp.example.test/private"}`,
		Auth:        `{"type":"bearer","token":"secret-token"}`,
		Tools: []*toolapi.MCPToolDefinition{
			{
				Name:        "search",
				Description: "Search",
				InputSchema: `{"type":"object"}`,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, appmcptool.SVC.RecordRuntimeHealth(
		identityContext,
		appmcptool.MCPRuntimeHealthReport{
			ServerID: 100,
			Success:  true,
		},
	))

	h := server.Default()
	h.GET("/api/workbench/runtime_doctor", func(_ context.Context, c *app.RequestContext) {
		requestContext := ctxcache.Init(context.Background())
		ctxcache.Store(requestContext, consts.SessionDataKeyInCtx, &userentity.Session{UserID: 7})
		GetWorkbenchRuntimeDoctor(requestContext, c)
	})

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workbench/runtime_doctor?space_id=1",
		nil,
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"code":0`)
	require.Contains(t, body, `"default_mode":"eino_adk"`)
	require.Contains(t, body, `"eino_adk_enabled":true`)
	require.Contains(t, body, `"web_search"`)
	require.Contains(t, body, `"configured":true`)
	require.Contains(t, body, `"model"`)
	require.Contains(t, body, `"live_probe":"ready"`)
	require.Contains(t, body, `"sandbox"`)
	require.Contains(t, body, `"runner_type":"disabled"`)
	require.Contains(t, body, `"reason_code":"control_plane_disabled"`)
	require.Contains(t, body, `"model.default"`)
	require.Contains(t, body, `"model.capabilities"`)
	require.Contains(t, body, `"model.live_connectivity"`)
	require.Contains(t, body, `"sandbox.provider.agent"`)
	require.Contains(t, body, `"skills.runtime_catalog"`)
	require.Contains(t, body, `"mcp_tools"`)
	require.Contains(t, body, `"total_servers":1`)
	require.Contains(t, body, `"healthy_servers":1`)
	require.NotContains(t, body, "secret-search-key")
	require.NotContains(t, body, "secret-token")
	require.NotContains(t, body, "mcp.example.test")
	require.NotContains(t, body, "search.example.test/private")
	require.NotContains(t, body, "hidden runtime probe response")
	require.NotContains(t, body, "private.internal")
	require.NotContains(t, body, "/private/node_modules")

	var decoded struct {
		Data struct {
			Status string `json:"status"`
			Model  struct {
				Configured bool   `json:"configured"`
				LiveProbe  string `json:"live_probe"`
			} `json:"model"`
			Sandbox struct {
				Status     string `json:"status"`
				RunnerType string `json:"runner_type"`
			} `json:"sandbox"`
			Checks []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(resp.Result().Body(), &decoded))
	require.Equal(t, "warning", decoded.Data.Status)
	require.True(t, decoded.Data.Model.Configured)
	require.Equal(t, "ready", decoded.Data.Model.LiveProbe)
	require.Equal(t, "disabled", decoded.Data.Sandbox.Status)
	require.Equal(t, "disabled", decoded.Data.Sandbox.RunnerType)
	require.NotEmpty(t, decoded.Data.Checks)
}
