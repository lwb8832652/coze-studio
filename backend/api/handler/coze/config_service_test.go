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
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	baseconfig "github.com/coze-dev/coze-studio/backend/bizpkg/config/base"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

type basicConfigurationBackendStub struct {
	configuration *adminconfig.BasicConfiguration
	revision      string
	err           error
	patch         baseconfig.BasicConfigurationPatch
	expected      string
}

func (s *basicConfigurationBackendStub) GetBaseConfigWithRevision(context.Context) (*adminconfig.BasicConfiguration, string, error) {
	return s.configuration, s.revision, s.err
}

func (s *basicConfigurationBackendStub) SaveBaseConfig(_ context.Context, patch baseconfig.BasicConfigurationPatch, expected string) (string, error) {
	s.patch = patch
	s.expected = expected
	return s.revision, s.err
}

func TestBasicConfigurationHandlersExposeRevisionAndIgnoreLegacyWrites(t *testing.T) {
	stub := &basicConfigurationBackendStub{
		configuration: &adminconfig.BasicConfiguration{
			ServerHost:     "https://old.example.test",
			CodeRunnerType: adminconfig.CodeRunnerType_Sandbox,
			SandboxConfig:  &adminconfig.SandboxConfig{AllowNet: "private.example.test"},
		},
		revision: "rev-7",
	}
	h := newBasicConfigurationTestServer(stub)

	getResponse := ut.PerformRequest(h.Engine, http.MethodGet, "/api/admin/config/basic/get", nil)
	require.Equal(t, http.StatusOK, getResponse.Code, string(getResponse.Result().Body()))
	var getBody map[string]any
	require.NoError(t, json.Unmarshal(getResponse.Result().Body(), &getBody))
	require.Equal(t, "rev-7", getBody["revision"])

	requestBody := `{"expected_revision":"rev-7","configuration":{"server_host":"https://new.example.test","code_runner_type":0,"sandbox_config":{"allow_net":"attacker.example.test"}}}`
	saveResponse := ut.PerformRequest(h.Engine, http.MethodPost, "/api/admin/config/basic/save", &ut.Body{Body: stringsReader(requestBody), Len: len(requestBody)}, ut.Header{Key: "content-type", Value: "application/json"})
	require.Equal(t, http.StatusOK, saveResponse.Code, string(saveResponse.Result().Body()))
	require.Equal(t, "rev-7", stub.expected)
	require.NotNil(t, stub.patch.ServerHost)
	require.Equal(t, "https://new.example.test", *stub.patch.ServerHost)
}

func TestBasicConfigurationHandlerReturnsStableConflict(t *testing.T) {
	stub := &basicConfigurationBackendStub{revision: "rev-8", err: kvstore.ErrVersionConflict}
	h := newBasicConfigurationTestServer(stub)
	requestBody := `{"expected_revision":"rev-7","configuration":{"admin_emails":"admin@example.test"}}`

	response := ut.PerformRequest(h.Engine, http.MethodPost, "/api/admin/config/basic/save", &ut.Body{Body: stringsReader(requestBody), Len: len(requestBody)}, ut.Header{Key: "content-type", Value: "application/json"})
	require.Equal(t, http.StatusConflict, response.Code, string(response.Result().Body()))
	require.Contains(t, string(response.Result().Body()), `"error_code":"BASE_CONFIG_VERSION_CONFLICT"`)
}

func TestBasicConfigurationHandlerDoesNotWriteAfterReadFailure(t *testing.T) {
	status, code, _ := basicConfigurationErrorContract(errors.New("storage unavailable"))
	require.Equal(t, http.StatusInternalServerError, status)
	require.Equal(t, "BASE_CONFIG_INTERNAL", code)
}

func newBasicConfigurationTestServer(backend basicConfigurationBackend) *server.Hertz {
	h := server.New()
	h.GET("/api/admin/config/basic/get", func(ctx context.Context, c *app.RequestContext) {
		getBasicConfiguration(ctx, c, backend)
	})
	h.POST("/api/admin/config/basic/save", func(ctx context.Context, c *app.RequestContext) {
		saveBasicConfiguration(ctx, c, backend)
	})
	return h
}

func stringsReader(value string) *strings.Reader { return strings.NewReader(value) }
