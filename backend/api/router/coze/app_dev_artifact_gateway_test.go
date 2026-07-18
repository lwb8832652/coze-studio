// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

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

type artifactGatewayRouteService struct{}

func (artifactGatewayRouteService) Upload(context.Context, applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error) {
	return &applicationappdev.ArtifactUploadResult{}, nil
}

func (artifactGatewayRouteService) OpenDownload(context.Context, applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error) {
	return &applicationappdev.ArtifactGatewayDownloadResponse{Size: 2, Body: io.NopCloser(bytes.NewReader([]byte("ok")))}, nil
}

type artifactGatewayRouteAuthenticator struct{}

func (artifactGatewayRouteAuthenticator) Authenticate(context.Context, *app.RequestContext) (handlercoze.AppDevArtifactGatewayProviderPrincipal, error) {
	return handlercoze.AppDevArtifactGatewayProviderPrincipal{Audience: domainappdev.ArtifactGrantAudience{
		SpaceID: "42", ProjectID: "project-1", ProviderKey: "runner-1",
		ProviderScope: domainsandbox.ScopeAppDev, Operation: "artifact.transfer",
	}}, nil
}

type artifactGatewayRouteTransport struct{}

func (artifactGatewayRouteTransport) Verify(context.Context, *app.RequestContext) error { return nil }

func TestArtifactGatewayRouteRegistrationRequiresExplicitSafeConfiguration(t *testing.T) {
	handler, err := handlercoze.NewAppDevArtifactGatewayHandler(artifactGatewayRouteService{}, artifactGatewayRouteAuthenticator{}, artifactGatewayRouteTransport{})
	require.NoError(t, err)
	h := server.Default()

	require.Error(t, RegisterAppDevArtifactGatewayRoutes(nil, handler))
	require.Error(t, RegisterAppDevArtifactGatewayRoutes(&AppDevArtifactGatewayRouteConfig{Root: h.Group("")}, nil))
	require.Error(t, RegisterAppDevArtifactGatewayRoutes(&AppDevArtifactGatewayRouteConfig{}, handler))
}

func TestArtifactGatewayRoutesAreProviderOnlyWithExactMethodsAndPath(t *testing.T) {
	handler, err := handlercoze.NewAppDevArtifactGatewayHandler(artifactGatewayRouteService{}, artifactGatewayRouteAuthenticator{}, artifactGatewayRouteTransport{})
	require.NoError(t, err)
	h := server.Default(server.WithRedirectTrailingSlash(false), server.WithStreamBody(true))
	require.NoError(t, RegisterAppDevArtifactGatewayRoutes(&AppDevArtifactGatewayRouteConfig{Root: h.Group("")}, handler))

	grantID, token := artifactGatewayRouteCapability(t)
	path := AppDevArtifactGatewayProviderPathPrefix + "/" + grantID
	upload := ut.PerformRequest(h.Engine, http.MethodPut, path, &ut.Body{Body: bytes.NewReader([]byte("ok")), Len: 2}, ut.Header{Key: handlercoze.AppDevArtifactGrantHeader, Value: token})
	require.Equal(t, http.StatusNoContent, upload.Code)
	download := ut.PerformRequest(h.Engine, http.MethodGet, path, nil, ut.Header{Key: handlercoze.AppDevArtifactGrantHeader, Value: token})
	require.Equal(t, http.StatusOK, download.Code)
	require.Equal(t, "ok", string(download.Result().Body()))

	for _, request := range []struct{ method, path string }{
		{http.MethodPost, path},
		{http.MethodDelete, path},
		{http.MethodGet, "/api/app-dev/artifacts/" + grantID},
		{http.MethodGet, AppDevArtifactGatewayProviderPathPrefix + "/"},
	} {
		response := ut.PerformRequest(h.Engine, request.method, request.path, nil, ut.Header{Key: handlercoze.AppDevArtifactGrantHeader, Value: token})
		require.NotEqualf(t, http.StatusOK, response.Code, "%s %s", request.method, request.path)
		require.NotEqualf(t, http.StatusNoContent, response.Code, "%s %s", request.method, request.path)
	}
}

func artifactGatewayRouteCapability(t *testing.T) (string, string) {
	t.Helper()
	grantID, err := domainappdev.NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{0x41}, domainappdev.ArtifactGrantIDBytes)))
	require.NoError(t, err)
	token, err := domainappdev.NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{0x72}, domainappdev.ArtifactGrantTokenBytes)))
	require.NoError(t, err)
	return grantID.Encoded(), token.Bearer()
}
