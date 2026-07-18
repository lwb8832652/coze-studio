// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package router

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	handlercoze "github.com/coze-dev/coze-studio/backend/api/handler/coze"
	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type registeredArtifactGatewayService struct{}

func (registeredArtifactGatewayService) Upload(context.Context, applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error) {
	return &applicationappdev.ArtifactUploadResult{}, nil
}

func (registeredArtifactGatewayService) OpenDownload(context.Context, applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error) {
	return &applicationappdev.ArtifactGatewayDownloadResponse{Size: 2, Body: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
}

type registeredArtifactGatewayAuthenticator struct{}

func (registeredArtifactGatewayAuthenticator) Authenticate(context.Context, *app.RequestContext) (handlercoze.AppDevArtifactGatewayProviderPrincipal, error) {
	return handlercoze.AppDevArtifactGatewayProviderPrincipal{Audience: domainappdev.ArtifactGrantAudience{
		SpaceID: "1001", ProjectID: "project-a", ProviderKey: "provider-a",
		ProviderScope: domainsandbox.ScopeAppDev, Operation: "runtime.start",
	}}, nil
}

type registeredArtifactGatewayTransport struct{}

func (registeredArtifactGatewayTransport) Verify(context.Context, *app.RequestContext) error {
	return nil
}

type registeredProviderAuthenticator struct{}

func (registeredProviderAuthenticator) AuthenticateArtifactProvider(context.Context, string, string) (domainappdev.ArtifactGrantAudience, error) {
	return domainappdev.ArtifactGrantAudience{
		SpaceID: "1001", ProjectID: "project-a", ProviderKey: "provider-a",
		ProviderScope: domainsandbox.ScopeAppDev, Operation: "runtime.start",
	}, nil
}

type registeredProviderTransport struct{}

func (registeredProviderTransport) VerifyArtifactProviderTransport(context.Context, applicationappdev.ProviderSecureTransportInput) error {
	return nil
}

type registeredProviderProjector struct{}

func (registeredProviderProjector) ProjectAppDevPreviewURL(context.Context, string, string, string) (string, error) {
	return "https://preview.example.test/opaque", nil
}

type registeredArtifactGatewayBodyService struct{ body string }

func (service registeredArtifactGatewayBodyService) Upload(context.Context, applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error) {
	return &applicationappdev.ArtifactUploadResult{}, nil
}

func (service registeredArtifactGatewayBodyService) OpenDownload(context.Context, applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error) {
	return &applicationappdev.ArtifactGatewayDownloadResponse{
		Size: int64(len(service.body)), Body: io.NopCloser(bytes.NewReader([]byte(service.body))),
	}, nil
}

func publishRegisteredArtifactGateway(t *testing.T, body string) applicationappdev.ProviderHTTPPublication {
	t.Helper()
	dependencies, err := applicationappdev.NewProviderHTTPDependencies(
		registeredArtifactGatewayBodyService{body: body},
		registeredProviderAuthenticator{},
		registeredProviderTransport{},
		registeredProviderProjector{},
	)
	require.NoError(t, err)
	publication, err := applicationappdev.PublishProviderHTTPDependencies(dependencies)
	require.NoError(t, err)
	return publication
}

func TestGeneratedRegisterArtifactGatewayDynamicallyFollowsOwnedPublication(t *testing.T) {
	router := server.Default(server.WithStreamBody(true))
	GeneratedRegister(router)
	response := ut.PerformRequest(router.Engine, http.MethodGet, "/internal/provider/app-dev/artifacts/missing", nil)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.NotContains(t, response.Body.String(), "missing")

	grantID, err := domainappdev.NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{0x41}, domainappdev.ArtifactGrantIDBytes)))
	require.NoError(t, err)
	token, err := domainappdev.NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{0x42}, domainappdev.ArtifactGrantTokenBytes)))
	require.NoError(t, err)
	request := func() *ut.ResponseRecorder {
		return ut.PerformRequest(
			router.Engine,
			http.MethodGet,
			"/internal/provider/app-dev/artifacts/"+grantID.Encoded(),
			nil,
			ut.Header{Key: handlercoze.AppDevArtifactGrantHeader, Value: token.Bearer()},
		)
	}

	ownerA := publishRegisteredArtifactGateway(t, "owner-a")
	response = request()
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "owner-a", string(response.Result().Body()))

	ownerB := publishRegisteredArtifactGateway(t, "owner-b")
	response = request()
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "owner-b", string(response.Result().Body()))

	require.False(t, applicationappdev.UnpublishProviderHTTPDependencies(ownerA))
	response = request()
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "owner-b", string(response.Result().Body()))

	require.True(t, applicationappdev.UnpublishProviderHTTPDependencies(ownerB))
	response = request()
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
}
