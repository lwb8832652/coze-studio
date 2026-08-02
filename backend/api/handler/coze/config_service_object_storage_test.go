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
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/middleware"
	appobjectstorage "github.com/coze-dev/coze-studio/backend/application/objectstorage"
	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

func TestObjectStorageHandlersRedactCredentialsAndMapErrors(t *testing.T) {
	stub := &objectStorageServiceStub{}
	h := newObjectStorageTestServer(stub)
	body := `{"name":"minio","provider_type":6,"config":{"bucket":"coze","endpoint":"minio:9000","use_ssl":false},"credential":{"access_key_id":"ak-secret","secret_access_key":"sk-secret"}}`

	resp := performObjectStorageHandlerRequest(h, http.MethodPost, "/api/admin/config/object-storage/create", body)

	require.Equal(t, http.StatusOK, resp.Code, string(resp.Result().Body()))
	responseBody := string(resp.Result().Body())
	require.NotContains(t, responseBody, "ak-secret")
	require.NotContains(t, responseBody, "sk-secret")
	require.Equal(t, domain.ProviderMinIO, stub.createRequest.ProviderType)
	require.Equal(t, "ak-secret", stub.createRequest.Credential.AccessKeyID)
}

func TestObjectStorageHandlersRejectIncompleteCredentials(t *testing.T) {
	h := newObjectStorageTestServer(&objectStorageServiceStub{createErr: domain.ErrConfigInvalid})
	body := `{"name":"minio","provider_type":6,"config":{"bucket":"coze","endpoint":"minio:9000","use_ssl":false},"credential":{"access_key_id":"ak-only"}}`

	resp := performObjectStorageHandlerRequest(h, http.MethodPost, "/api/admin/config/object-storage/create", body)

	require.Equal(t, http.StatusBadRequest, resp.Code, string(resp.Result().Body()))
	require.Contains(t, string(resp.Result().Body()), "OBJECT_STORAGE_CONFIG_INVALID")
}

func newObjectStorageTestServer(service objectStorageAdminService) *server.Hertz {
	h := server.New(server.WithStreamBody(true))
	h.Use(middleware.RequestBodyCompatibilityMW(maxObjectStorageBodyBytes))
	handler := newObjectStorageAdminHandler(service)
	h.GET("/api/admin/config/object-storage/list", handler.list)
	h.POST("/api/admin/config/object-storage/create", handler.create)
	h.POST("/api/admin/config/object-storage/update", handler.update)
	h.POST("/api/admin/config/object-storage/test", handler.test)
	h.POST("/api/admin/config/object-storage/activate", handler.activate)
	h.POST("/api/admin/config/object-storage/delete", handler.delete)
	return h
}

func performObjectStorageHandlerRequest(h *server.Hertz, method, path, body string) *ut.ResponseRecorder {
	var requestBody *ut.Body
	headers := make([]ut.Header, 0, 1)
	if body != "" {
		requestBody = &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
		headers = append(headers, ut.Header{Key: "content-type", Value: "application/json"})
	}
	return ut.PerformRequest(h.Engine, method, path, requestBody, headers...)
}

type objectStorageServiceStub struct {
	createRequest appobjectstorage.CreateRequest
	createErr     error
}

func (s *objectStorageServiceStub) List(context.Context) (*appobjectstorage.ListResult, error) {
	return &appobjectstorage.ListResult{RuntimeSource: domain.RuntimeSourceDatabase}, nil
}

func (s *objectStorageServiceStub) Create(_ context.Context, request appobjectstorage.CreateRequest) (*appobjectstorage.ConfigView, error) {
	s.createRequest = request
	if s.createErr != nil {
		return nil, s.createErr
	}
	return objectStorageTestConfigView(request.Name, request.ProviderType, request.PublicConfig), nil
}

func (s *objectStorageServiceStub) Update(context.Context, appobjectstorage.UpdateRequest) (*appobjectstorage.ConfigView, error) {
	return objectStorageTestConfigView("updated", domain.ProviderMinIO, domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000"}), nil
}

func (s *objectStorageServiceStub) Test(context.Context, appobjectstorage.TestRequest) (*appobjectstorage.TestResult, error) {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	return &appobjectstorage.TestResult{Success: true, Health: domain.Health{
		Status: domain.HealthHealthy, Code: "OK", Message: "OK", CheckedAt: &now,
	}}, nil
}

func (s *objectStorageServiceStub) Activate(context.Context, appobjectstorage.ActivateRequest) (*appobjectstorage.ConfigView, error) {
	return objectStorageTestConfigView("activated", domain.ProviderMinIO, domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000"}), nil
}

func (s *objectStorageServiceStub) Delete(context.Context, appobjectstorage.DeleteRequest) error {
	return nil
}

func objectStorageTestConfigView(name string, provider domain.ProviderType, config domain.PublicConfig) *appobjectstorage.ConfigView {
	now := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	return &appobjectstorage.ConfigView{
		ID:                   7,
		Name:                 strings.TrimSpace(name),
		ProviderType:         provider,
		PublicConfig:         config,
		CredentialConfigured: true,
		Health:               domain.Health{Status: domain.HealthUnknown},
		Version:              1,
		RuntimeRevision:      1,
		CreatedAt:            now.Format(time.RFC3339),
		UpdatedAt:            now.Format(time.RFC3339),
	}
}
