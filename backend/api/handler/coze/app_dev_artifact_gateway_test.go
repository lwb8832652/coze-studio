// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type artifactGatewayHandlerServiceStub struct {
	upload        func(context.Context, applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error)
	openDownload  func(context.Context, applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error)
	uploadCalls   int
	downloadCalls int
	lastUpload    applicationappdev.UploadArtifactRequest
	lastDownload  applicationappdev.DownloadArtifactRequest
}

func (stub *artifactGatewayHandlerServiceStub) Upload(ctx context.Context, request applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error) {
	stub.uploadCalls++
	stub.lastUpload = request
	if stub.upload != nil {
		return stub.upload(ctx, request)
	}
	return &applicationappdev.ArtifactUploadResult{}, nil
}

func (stub *artifactGatewayHandlerServiceStub) OpenDownload(ctx context.Context, request applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error) {
	stub.downloadCalls++
	stub.lastDownload = request
	if stub.openDownload != nil {
		return stub.openDownload(ctx, request)
	}
	return nil, domainappdev.ErrArtifactGrantUnavailable
}

type artifactGatewayAuthenticatorStub struct {
	principal AppDevArtifactGatewayProviderPrincipal
	err       error
	calls     int
}

func (stub *artifactGatewayAuthenticatorStub) Authenticate(context.Context, *app.RequestContext) (AppDevArtifactGatewayProviderPrincipal, error) {
	stub.calls++
	return stub.principal, stub.err
}

type artifactGatewayTransportVerifierStub struct {
	err   error
	calls int
}

func (stub *artifactGatewayTransportVerifierStub) Verify(context.Context, *app.RequestContext) error {
	stub.calls++
	return stub.err
}

type artifactGatewayCloseTracker struct {
	io.Reader
	closed bool
}

func (tracker *artifactGatewayCloseTracker) Close() error {
	tracker.closed = true
	return nil
}

func TestArtifactGatewayHandlerDependenciesFailClosed(t *testing.T) {
	service := &artifactGatewayHandlerServiceStub{}
	authenticator := &artifactGatewayAuthenticatorStub{}
	transport := &artifactGatewayTransportVerifierStub{}

	for _, construct := range []func() (*AppDevArtifactGatewayHandler, error){
		func() (*AppDevArtifactGatewayHandler, error) {
			return NewAppDevArtifactGatewayHandler(nil, authenticator, transport)
		},
		func() (*AppDevArtifactGatewayHandler, error) {
			return NewAppDevArtifactGatewayHandler(service, nil, transport)
		},
		func() (*AppDevArtifactGatewayHandler, error) {
			return NewAppDevArtifactGatewayHandler(service, authenticator, nil)
		},
	} {
		handler, err := construct()
		require.Nil(t, handler)
		require.Error(t, err)
	}
}

func TestArtifactGatewayHandlerChecksTransportAndAuthenticationBeforeGateway(t *testing.T) {
	grantID, token := artifactGatewayHandlerCapability(t)
	service := &artifactGatewayHandlerServiceStub{}
	authenticator := &artifactGatewayAuthenticatorStub{principal: artifactGatewayHandlerPrincipal()}
	transport := &artifactGatewayTransportVerifierStub{err: errors.New("insecure forwarded request secret")}
	handler, err := NewAppDevArtifactGatewayHandler(service, authenticator, transport)
	require.NoError(t, err)
	h := artifactGatewayHandlerTestServer(handler)

	response := artifactGatewayHandlerRequest(h, http.MethodPut, grantID, token, []byte("payload"), nil)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.Equal(t, 0, authenticator.calls)
	require.Equal(t, 0, service.uploadCalls)
	require.NotContains(t, string(response.Result().Body()), token)
	require.NotContains(t, string(response.Result().Body()), "forwarded")

	transport.err = nil
	authenticator.err = errors.New("provider credential secret")
	response = artifactGatewayHandlerRequest(h, http.MethodPut, grantID, token, []byte("payload"), nil)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, 0, service.uploadCalls)
	require.NotContains(t, string(response.Result().Body()), "credential")

	authenticator.err = nil
	authenticator.principal = AppDevArtifactGatewayProviderPrincipal{}
	response = artifactGatewayHandlerRequest(h, http.MethodPut, grantID, token, []byte("payload"), nil)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, 0, service.uploadCalls)
}

func TestArtifactGatewayHandlerUsesAuthenticatedAudienceAndDedicatedCapabilityHeader(t *testing.T) {
	grantID, token := artifactGatewayHandlerCapability(t)
	principal := artifactGatewayHandlerPrincipal()
	service := &artifactGatewayHandlerServiceStub{}
	authenticator := &artifactGatewayAuthenticatorStub{principal: principal}
	handler, err := NewAppDevArtifactGatewayHandler(service, authenticator, &artifactGatewayTransportVerifierStub{})
	require.NoError(t, err)
	h := artifactGatewayHandlerTestServer(handler)

	response := artifactGatewayHandlerRequest(h, http.MethodPut, grantID, token, []byte("payload"), []ut.Header{
		{Key: "X-Space-ID", Value: "999"},
		{Key: "X-Project-ID", Value: "spoofed-project"},
		{Key: "X-Provider-Key", Value: "spoofed-provider"},
	})
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Equal(t, principal.Audience, service.lastUpload.Audience)
	require.Equal(t, domainappdev.ArtifactGrantDirectionUpload, service.lastUpload.Direction)
	require.Equal(t, grantID, service.lastUpload.Capability.GrantID().Encoded())
	require.Equal(t, token, service.lastUpload.Capability.BearerToken())

	response = artifactGatewayHandlerRequest(h, http.MethodPut, grantID+"?grant_token="+token, token, []byte("payload"), nil)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, 1, service.uploadCalls)
	require.NotContains(t, string(response.Result().Body()), token)

	response = artifactGatewayHandlerRequest(h, http.MethodPut, "../invalid", token, []byte("payload"), nil)
	require.NotEqual(t, http.StatusNoContent, response.Code)
	require.Equal(t, 1, service.uploadCalls)

	response = artifactGatewayHandlerRequest(h, http.MethodPut, grantID, strings.Repeat("x", MaxAppDevArtifactGrantHeaderBytes+1), []byte("payload"), nil)
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Equal(t, 1, service.uploadCalls)
}

func TestArtifactGatewayHandlerUploadClosesBodyAndMapsSafeErrors(t *testing.T) {
	grantID, token := artifactGatewayHandlerCapability(t)
	service := &artifactGatewayHandlerServiceStub{}
	handler, err := NewAppDevArtifactGatewayHandler(service, &artifactGatewayAuthenticatorStub{principal: artifactGatewayHandlerPrincipal()}, &artifactGatewayTransportVerifierStub{})
	require.NoError(t, err)
	h := artifactGatewayHandlerTestServer(handler)

	service.upload = func(_ context.Context, request applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error) {
		body, readErr := io.ReadAll(request.Body)
		require.NoError(t, readErr)
		require.Equal(t, []byte("payload"), body)
		require.Equal(t, int64(len(body)), request.ContentLength)
		return &applicationappdev.ArtifactUploadResult{Size: int64(len(body))}, nil
	}
	response := artifactGatewayHandlerRequest(h, http.MethodPut, grantID, token, []byte("payload"), nil)
	require.Equal(t, http.StatusNoContent, response.Code)
	require.Empty(t, response.Result().Body())

	for _, test := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"invalid length or digest", domainappdev.ErrArtifactGrantInvalid, http.StatusBadRequest, "ARTIFACT_INVALID"},
		{"replay", domainappdev.ErrArtifactGrantConsumed, http.StatusForbidden, "ARTIFACT_GRANT_DENIED"},
		{"expired", domainappdev.ErrArtifactGrantExpired, http.StatusForbidden, "ARTIFACT_GRANT_DENIED"},
		{"revoked", domainappdev.ErrArtifactGrantRevoked, http.StatusForbidden, "ARTIFACT_GRANT_DENIED"},
		{"audience", domainappdev.ErrArtifactGrantAudienceMismatch, http.StatusForbidden, "ARTIFACT_GRANT_DENIED"},
		{"storage", fmt.Errorf("private-object-key: %w", domainappdev.ErrArtifactGrantStorage), http.StatusServiceUnavailable, "ARTIFACT_GATEWAY_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service.upload = func(context.Context, applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error) {
				return nil, test.err
			}
			response := artifactGatewayHandlerRequest(h, http.MethodPut, grantID, token, []byte("payload"), nil)
			body := string(response.Result().Body())
			var headers strings.Builder
			response.Result().Header.VisitAll(func(key, value []byte) {
				headers.Write(key)
				headers.Write(value)
			})
			require.Equal(t, test.status, response.Code)
			require.Contains(t, body, `"code":"`+test.code+`"`)
			for _, forbidden := range []string{token, grantID, "private-object-key", "http://", "https://"} {
				require.NotContains(t, body, forbidden)
				require.NotContains(t, headers.String(), forbidden)
			}
		})
	}
}

func TestArtifactGatewayHandlerDownloadSetsSafeHeadersAndClosesValidatedStream(t *testing.T) {
	grantID, token := artifactGatewayHandlerCapability(t)
	tracker := &artifactGatewayCloseTracker{Reader: bytes.NewReader([]byte("download"))}
	service := &artifactGatewayHandlerServiceStub{openDownload: func(_ context.Context, request applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error) {
		require.Equal(t, domainappdev.ArtifactGrantDirectionDownload, request.Direction)
		return &applicationappdev.ArtifactGatewayDownloadResponse{Size: 8, Body: tracker}, nil
	}}
	handler, err := NewAppDevArtifactGatewayHandler(service, &artifactGatewayAuthenticatorStub{principal: artifactGatewayHandlerPrincipal()}, &artifactGatewayTransportVerifierStub{})
	require.NoError(t, err)
	h := artifactGatewayHandlerTestServer(handler)

	response := artifactGatewayHandlerRequest(h, http.MethodGet, grantID, token, nil, nil)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "download", string(response.Result().Body()))
	require.Equal(t, "8", string(response.Result().Header.Peek("Content-Length")))
	require.Equal(t, "application/octet-stream", string(response.Result().Header.ContentType()))
	require.Equal(t, "no-store", string(response.Result().Header.Peek("Cache-Control")))
	require.Equal(t, "nosniff", string(response.Result().Header.Peek("X-Content-Type-Options")))
	require.True(t, tracker.closed)
}

func TestArtifactGatewayHandlerDownloadFailureWritesNoArtifactBytes(t *testing.T) {
	grantID, token := artifactGatewayHandlerCapability(t)
	service := &artifactGatewayHandlerServiceStub{openDownload: func(context.Context, applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error) {
		return nil, errors.New("partial-secret-body https://storage.invalid/private/object")
	}}
	handler, err := NewAppDevArtifactGatewayHandler(service, &artifactGatewayAuthenticatorStub{principal: artifactGatewayHandlerPrincipal()}, &artifactGatewayTransportVerifierStub{})
	require.NoError(t, err)
	h := artifactGatewayHandlerTestServer(handler)

	response := artifactGatewayHandlerRequest(h, http.MethodGet, grantID, token, nil, nil)
	body := string(response.Result().Body())
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Contains(t, body, `"code":"ARTIFACT_GATEWAY_UNAVAILABLE"`)
	require.NotContains(t, body, "partial-secret-body")
	require.NotContains(t, body, "storage.invalid")
	require.NotContains(t, body, token)
}

func artifactGatewayHandlerTestServer(handler *AppDevArtifactGatewayHandler) *server.Hertz {
	h := server.Default(server.WithRedirectTrailingSlash(false), server.WithStreamBody(true))
	h.PUT("/internal/provider/app-dev/artifacts/:grant_id", handler.Upload)
	h.GET("/internal/provider/app-dev/artifacts/:grant_id", handler.Download)
	return h
}

func artifactGatewayHandlerRequest(h *server.Hertz, method, grantID, token string, body []byte, extra []ut.Header) *ut.ResponseRecorder {
	var requestBody *ut.Body
	if body != nil {
		requestBody = &ut.Body{Body: bytes.NewReader(body), Len: len(body)}
	}
	headers := append([]ut.Header{{Key: AppDevArtifactGrantHeader, Value: token}}, extra...)
	return ut.PerformRequest(h.Engine, method, "/internal/provider/app-dev/artifacts/"+grantID, requestBody, headers...)
}

func artifactGatewayHandlerCapability(t *testing.T) (string, string) {
	t.Helper()
	grantID, err := domainappdev.NewRandomArtifactGrantID(bytes.NewReader(bytes.Repeat([]byte{0x31}, domainappdev.ArtifactGrantIDBytes)))
	require.NoError(t, err)
	token, err := domainappdev.NewRandomArtifactGrantToken(bytes.NewReader(bytes.Repeat([]byte{0x62}, domainappdev.ArtifactGrantTokenBytes)))
	require.NoError(t, err)
	return grantID.Encoded(), token.Bearer()
}

func artifactGatewayHandlerPrincipal() AppDevArtifactGatewayProviderPrincipal {
	return AppDevArtifactGatewayProviderPrincipal{Audience: domainappdev.ArtifactGrantAudience{
		SpaceID: "42", ProjectID: "project-1", ProviderKey: "runner-1",
		ProviderScope: domainsandbox.ScopeAppDev, Operation: "snapshot.upload",
	}}
}
