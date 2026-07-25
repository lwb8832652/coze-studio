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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	baseconfig "github.com/coze-dev/coze-studio/backend/bizpkg/config/base"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

type basicConfigurationBackendStub struct {
	configuration *adminconfig.BasicConfiguration
	revision      string
	err           error
	patch         baseconfig.BasicConfigurationPatch
	expected      string
	saveCalls     int
}

func (s *basicConfigurationBackendStub) GetBaseConfigWithRevision(context.Context) (*adminconfig.BasicConfiguration, string, error) {
	return s.configuration, s.revision, s.err
}

func (s *basicConfigurationBackendStub) SaveBaseConfig(_ context.Context, patch baseconfig.BasicConfigurationPatch, expected string) (string, error) {
	s.saveCalls++
	s.patch = patch
	s.expected = expected
	if s.err == nil && patch.AdminEmails != nil {
		if s.configuration == nil {
			s.configuration = &adminconfig.BasicConfiguration{}
		}
		s.configuration.AdminEmails = *patch.AdminEmails
	}
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

func TestBasicConfigurationHandlerCanonicalizesAdminEmailsBeforeSave(t *testing.T) {
	stub := &basicConfigurationBackendStub{revision: "rev-8"}
	h := newBasicConfigurationTestServer(stub)
	requestBody := `{"expected_revision":"rev-7","configuration":{"admin_emails":" ADMIN@example.test ,admin@example.test,other@example.test "}}`

	response := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/config/basic/save",
		&ut.Body{Body: stringsReader(requestBody), Len: len(requestBody)},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, response.Code, string(response.Result().Body()))
	require.Equal(t, 1, stub.saveCalls)
	require.NotNil(t, stub.patch.AdminEmails)
	require.Equal(
		t,
		"admin@example.test,other@example.test",
		*stub.patch.AdminEmails,
	)
}

func TestBasicConfigurationHandlerRejectsInvalidAdminEmailsBeforeStorage(t *testing.T) {
	tooMany := make([]string, domainnotification.MaxExplicitRecipients+1)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("admin-%d@example.test", index)
	}
	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "empty item", value: "admin@example.test,,other@example.test"},
		{name: "trailing comma", value: "admin@example.test,"},
		{name: "invalid address", value: "admin@example.test,bad address <"},
		{name: "over limit", value: strings.Join(tooMany, ",")},
	} {
		t.Run(test.name, func(t *testing.T) {
			stub := &basicConfigurationBackendStub{
				configuration: &adminconfig.BasicConfiguration{
					AdminEmails: "existing-admin@example.test",
				},
				revision: "rev-7",
			}
			h := newBasicConfigurationTestServer(stub)
			body, err := json.Marshal(map[string]any{
				"expected_revision": "rev-7",
				"configuration": map[string]any{
					"admin_emails": test.value,
				},
			})
			require.NoError(t, err)

			response := ut.PerformRequest(
				h.Engine,
				http.MethodPost,
				"/api/admin/config/basic/save",
				&ut.Body{
					Body: stringsReader(string(body)),
					Len: len(body),
				},
				ut.Header{Key: "content-type", Value: "application/json"},
			)
			require.Equal(
				t,
				http.StatusBadRequest,
				response.Code,
				string(response.Result().Body()),
			)
			require.Contains(
				t,
				string(response.Result().Body()),
				`"error_code":"ADMIN_EMAILS_INVALID"`,
			)
			require.Equal(t, 0, stub.saveCalls)
			require.Equal(t, "rev-7", stub.revision)
			require.Equal(
				t,
				"existing-admin@example.test",
				stub.configuration.AdminEmails,
			)
		})
	}
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
