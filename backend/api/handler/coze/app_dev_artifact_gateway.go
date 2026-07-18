// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"sync"

	"github.com/cloudwego/hertz/pkg/app"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

const (
	AppDevArtifactGrantHeader         = "X-Coze-Artifact-Grant-Token"
	MaxAppDevArtifactGrantHeaderBytes = 128
)

type AppDevArtifactGatewayProviderPrincipal struct {
	Audience domainappdev.ArtifactGrantAudience
}

type AppDevArtifactGatewayProviderAuthenticator interface {
	Authenticate(context.Context, *app.RequestContext) (AppDevArtifactGatewayProviderPrincipal, error)
}

type AppDevArtifactGatewaySecureTransportVerifier interface {
	Verify(context.Context, *app.RequestContext) error
}

type AppDevArtifactGatewayService interface {
	Upload(context.Context, applicationappdev.UploadArtifactRequest) (*applicationappdev.ArtifactUploadResult, error)
	OpenDownload(context.Context, applicationappdev.DownloadArtifactRequest) (*applicationappdev.ArtifactGatewayDownloadResponse, error)
}

type AppDevArtifactGatewayHandler struct {
	gateway       AppDevArtifactGatewayService
	authenticator AppDevArtifactGatewayProviderAuthenticator
	transport     AppDevArtifactGatewaySecureTransportVerifier
	unavailable   bool
	dynamic       bool
}

func NewUnavailableAppDevArtifactGatewayHandler() *AppDevArtifactGatewayHandler {
	return &AppDevArtifactGatewayHandler{unavailable: true}
}

func NewDynamicAppDevArtifactGatewayHandler() *AppDevArtifactGatewayHandler {
	return &AppDevArtifactGatewayHandler{dynamic: true}
}

func CurrentAppDevArtifactGatewayHandler() *AppDevArtifactGatewayHandler {
	dependencies := applicationappdev.CurrentProviderHTTPDependencies()
	if dependencies == nil {
		return nil
	}
	handler, err := NewAppDevArtifactGatewayHandler(
		dependencies.ArtifactGateway(),
		appDevProviderAuthenticatorAdapter{delegate: dependencies.ArtifactAuthenticator()},
		appDevSecureTransportAdapter{delegate: dependencies.SecureTransportVerifier()},
	)
	if err != nil {
		return nil
	}
	return handler
}

type appDevProviderAuthenticatorAdapter struct {
	delegate applicationappdev.ProviderArtifactAuthenticator
}

func (adapter appDevProviderAuthenticatorAdapter) Authenticate(
	ctx context.Context,
	request *app.RequestContext,
) (AppDevArtifactGatewayProviderPrincipal, error) {
	if adapter.delegate == nil || request == nil {
		return AppDevArtifactGatewayProviderPrincipal{}, domainappdev.ErrArtifactGrantUnavailable
	}
	audience, err := adapter.delegate.AuthenticateArtifactProvider(
		ctx,
		request.Param("grant_id"),
		string(request.Request.Header.Peek("Authorization")),
	)
	if err != nil {
		return AppDevArtifactGatewayProviderPrincipal{}, domainappdev.ErrArtifactGrantUnavailable
	}
	return AppDevArtifactGatewayProviderPrincipal{Audience: audience}, nil
}

type appDevSecureTransportAdapter struct {
	delegate applicationappdev.ProviderSecureTransportVerifier
}

func (adapter appDevSecureTransportAdapter) Verify(ctx context.Context, request *app.RequestContext) error {
	if adapter.delegate == nil || request == nil {
		return domainappdev.ErrArtifactGrantUnavailable
	}
	remoteAddress := ""
	if remote := request.RemoteAddr(); remote != nil {
		remoteAddress = remote.String()
	}
	return adapter.delegate.VerifyArtifactProviderTransport(ctx, applicationappdev.ProviderSecureTransportInput{
		DirectHTTPS:    string(request.Request.URI().Scheme()) == "https",
		RemoteAddress:  remoteAddress,
		ForwardedProto: string(request.Request.Header.Peek("X-Forwarded-Proto")),
	})
}

func NewAppDevArtifactGatewayHandler(
	gateway AppDevArtifactGatewayService,
	authenticator AppDevArtifactGatewayProviderAuthenticator,
	transport AppDevArtifactGatewaySecureTransportVerifier,
) (*AppDevArtifactGatewayHandler, error) {
	if appDevArtifactGatewayNilDependency(gateway) ||
		appDevArtifactGatewayNilDependency(authenticator) ||
		appDevArtifactGatewayNilDependency(transport) {
		return nil, domainappdev.ErrArtifactGrantInvalid
	}
	return &AppDevArtifactGatewayHandler{
		gateway: gateway, authenticator: authenticator, transport: transport,
	}, nil
}

func (handler *AppDevArtifactGatewayHandler) Upload(ctx context.Context, requestContext *app.RequestContext) {
	if handler != nil && handler.dynamic {
		current := CurrentAppDevArtifactGatewayHandler()
		if current == nil {
			appDevArtifactGatewayUnavailable(requestContext)
			return
		}
		current.Upload(ctx, requestContext)
		return
	}
	if handler == nil || handler.unavailable {
		appDevArtifactGatewayUnavailable(requestContext)
		return
	}
	capability, audience, ok := handler.authorize(ctx, requestContext, domainappdev.ArtifactGrantDirectionUpload)
	if !ok {
		return
	}
	body := appDevArtifactGatewayRequestBody(requestContext)
	if body == nil {
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantInvalid)
		return
	}
	defer body.Close()
	_, err := handler.gateway.Upload(ctx, applicationappdev.UploadArtifactRequest{
		Capability: capability, Audience: audience,
		Direction: domainappdev.ArtifactGrantDirectionUpload,
		Body:      body, ContentLength: int64(requestContext.Request.Header.ContentLength()),
	})
	if err != nil {
		appDevArtifactGatewayError(requestContext, err)
		return
	}
	appDevArtifactGatewaySafeHeaders(requestContext)
	requestContext.SetStatusCode(http.StatusNoContent)
}

func (handler *AppDevArtifactGatewayHandler) Download(ctx context.Context, requestContext *app.RequestContext) {
	if handler != nil && handler.dynamic {
		current := CurrentAppDevArtifactGatewayHandler()
		if current == nil {
			appDevArtifactGatewayUnavailable(requestContext)
			return
		}
		current.Download(ctx, requestContext)
		return
	}
	if handler == nil || handler.unavailable {
		appDevArtifactGatewayUnavailable(requestContext)
		return
	}
	capability, audience, ok := handler.authorize(ctx, requestContext, domainappdev.ArtifactGrantDirectionDownload)
	if !ok {
		return
	}
	download, err := handler.gateway.OpenDownload(ctx, applicationappdev.DownloadArtifactRequest{
		Capability: capability, Audience: audience, Direction: domainappdev.ArtifactGrantDirectionDownload,
	})
	if err != nil {
		appDevArtifactGatewayError(requestContext, err)
		return
	}
	if download == nil || download.Body == nil || download.Size < 0 || download.Size > domainappdev.MaxArtifactGrantObjectBytes {
		if download != nil && download.Body != nil {
			_ = download.Body.Close()
		}
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantUnavailable)
		return
	}
	appDevArtifactGatewaySafeHeaders(requestContext)
	requestContext.Response.Header.SetContentType("application/octet-stream")
	requestContext.Response.Header.Set("Content-Length", strconv.FormatInt(download.Size, 10))
	requestContext.SetStatusCode(http.StatusOK)
	requestContext.Response.SetBodyStream(download.Body, int(download.Size))
}

func appDevArtifactGatewayUnavailable(requestContext *app.RequestContext) {
	if requestContext == nil {
		return
	}
	requestContext.Response.Header.Set("Retry-After", "5")
	appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantUnavailable)
}

func (handler *AppDevArtifactGatewayHandler) authorize(
	ctx context.Context,
	requestContext *app.RequestContext,
	direction domainappdev.ArtifactGrantDirection,
) (*applicationappdev.ArtifactGrantCapability, domainappdev.ArtifactGrantAudience, bool) {
	if handler == nil || requestContext == nil || ctx == nil {
		if requestContext != nil {
			appDevArtifactGatewayTransportError(requestContext)
		}
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	if err := handler.transport.Verify(ctx, requestContext); err != nil {
		appDevArtifactGatewayTransportError(requestContext)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	principal, err := handler.authenticator.Authenticate(ctx, requestContext)
	if err != nil || domainappdev.ValidateArtifactGrantAudience(principal.Audience) != nil {
		appDevArtifactGatewayAuthenticationError(requestContext)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	if len(requestContext.Request.URI().QueryString()) != 0 {
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantInvalid)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	grantID, err := domainappdev.ParseArtifactGrantID(requestContext.Param("grant_id"))
	if err != nil {
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantInvalid)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	bearer := requestContext.Request.Header.Peek(AppDevArtifactGrantHeader)
	if len(bearer) == 0 || len(bearer) > MaxAppDevArtifactGrantHeaderBytes {
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantInvalid)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	token, err := domainappdev.ParseArtifactGrantToken(string(bearer))
	if err != nil {
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantInvalid)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	capability, err := applicationappdev.NewArtifactGrantGatewayCapability(grantID, token, principal.Audience, direction)
	if err != nil {
		appDevArtifactGatewayError(requestContext, domainappdev.ErrArtifactGrantInvalid)
		return nil, domainappdev.ArtifactGrantAudience{}, false
	}
	return capability, principal.Audience, true
}

type appDevArtifactGatewayBody struct {
	io.Reader
	close func() error
	once  sync.Once
	err   error
}

func (body *appDevArtifactGatewayBody) Close() error {
	body.once.Do(func() {
		if body.close != nil {
			body.err = body.close()
		}
	})
	return body.err
}

func appDevArtifactGatewayRequestBody(requestContext *app.RequestContext) io.ReadCloser {
	if requestContext.Request.IsBodyStream() {
		stream := requestContext.Request.BodyStream()
		if stream == nil {
			return nil
		}
		return &appDevArtifactGatewayBody{Reader: stream, close: requestContext.Request.CloseBodyStream}
	}
	return io.NopCloser(bytes.NewReader(requestContext.Request.Body()))
}

type appDevArtifactGatewayEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func appDevArtifactGatewayTransportError(requestContext *app.RequestContext) {
	appDevArtifactGatewayWriteError(requestContext, http.StatusForbidden, "PROVIDER_TRANSPORT_REQUIRED", "secure provider transport required")
}

func appDevArtifactGatewayAuthenticationError(requestContext *app.RequestContext) {
	appDevArtifactGatewayWriteError(requestContext, http.StatusUnauthorized, "PROVIDER_AUTHENTICATION_REQUIRED", "provider authentication required")
}

func appDevArtifactGatewayError(requestContext *app.RequestContext, err error) {
	status, code, message := http.StatusServiceUnavailable, "ARTIFACT_GATEWAY_UNAVAILABLE", "artifact gateway unavailable"
	switch {
	case errors.Is(err, domainappdev.ErrArtifactGrantInvalid):
		status, code, message = http.StatusBadRequest, "ARTIFACT_INVALID", "artifact request invalid"
	case errors.Is(err, domainappdev.ErrArtifactGrantDenied),
		errors.Is(err, domainappdev.ErrArtifactGrantConsumed),
		errors.Is(err, domainappdev.ErrArtifactGrantExpired),
		errors.Is(err, domainappdev.ErrArtifactGrantRevoked),
		errors.Is(err, domainappdev.ErrArtifactGrantAudienceMismatch),
		errors.Is(err, domainappdev.ErrArtifactGrantDirectionMismatch):
		status, code, message = http.StatusForbidden, "ARTIFACT_GRANT_DENIED", "artifact grant denied"
	}
	appDevArtifactGatewayWriteError(requestContext, status, code, message)
}

func appDevArtifactGatewayWriteError(requestContext *app.RequestContext, status int, code, message string) {
	appDevArtifactGatewaySafeHeaders(requestContext)
	requestContext.AbortWithStatusJSON(status, appDevArtifactGatewayEnvelope{Code: code, Message: message})
}

func appDevArtifactGatewaySafeHeaders(requestContext *app.RequestContext) {
	requestContext.Response.Header.Set("Cache-Control", "no-store")
	requestContext.Response.Header.Set("X-Content-Type-Options", "nosniff")
}

func appDevArtifactGatewayNilDependency(dependency any) bool {
	if dependency == nil {
		return true
	}
	value := reflect.ValueOf(dependency)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
