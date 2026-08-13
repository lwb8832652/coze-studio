// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

const (
	defaultSessionRequestTimeout = 30 * time.Second
	maxSessionResponseBytes      = 64 << 20
	maxSessionEventsBytes        = 16 * 1024
	sessionOperationWriteGrace   = 5 * time.Second
)

type SessionCoreReadiness interface{ CoreReady(context.Context) error }

type SessionVerifyInput struct {
	ContextHeader   string
	SignatureHeader string
	Target          sandboxidentity.SessionContextTarget
	Method          string
	Path            string
	RequestDigest   []byte
}

type SessionIdentityVerifier interface {
	VerifySession(context.Context, SessionVerifyInput) (sandboxidentity.SessionRequest, error)
}

type sessionContextIdentityVerifier struct {
	keyring sandboxidentity.Keyring
	nonces  sandboxidentity.SessionNonceStore
	now     func() time.Time
}

// NewSessionContextIdentityVerifier adapts the authenticated v2 envelope
// verifier to the HTTP boundary. Claims are returned only after canonical
// HMAC, method/path/digest, expiry and one-time nonce validation succeeds.
func NewSessionContextIdentityVerifier(keyring sandboxidentity.Keyring, nonces sandboxidentity.SessionNonceStore, now func() time.Time) (SessionIdentityVerifier, error) {
	if keyring.ActiveKeyID == "" || nonces == nil || now == nil {
		return nil, ErrConfiguration
	}
	return sessionContextIdentityVerifier{keyring: keyring, nonces: nonces, now: now}, nil
}

func (verifier sessionContextIdentityVerifier) VerifySession(ctx context.Context, input SessionVerifyInput) (sandboxidentity.SessionRequest, error) {
	if ctx == nil || input.ContextHeader == "" || input.SignatureHeader == "" || len(input.RequestDigest) != sha256.Size {
		return sandboxidentity.SessionRequest{}, sandboxidentity.ErrInvalidContext
	}
	return verifier.keyring.VerifySessionContext(ctx, input.ContextHeader, input.SignatureHeader,
		input.Target, input.Method, input.Path, input.RequestDigest, verifier.now().UTC(), verifier.nonces)
}

type SessionIdentityResolver interface {
	ResolveSessionIdentity(context.Context, string, SessionStableIdentity) (SessionStableIdentity, error)
}

type SessionAcquirer interface {
	Acquire(context.Context, SessionAcquireCommand) (SessionProjection, error)
}
type SessionGetter interface {
	Get(context.Context, SessionRouteCommand) (SessionProjection, error)
}
type SessionReleaser interface {
	Release(context.Context, SessionRouteCommand) error
}
type SessionDestroyer interface {
	Destroy(context.Context, SessionRouteCommand) error
}
type SessionRecoverer interface {
	Recover(context.Context, SessionRouteCommand) (SessionProjection, error)
}
type SessionOperationSubmitter interface {
	SubmitOperation(context.Context, SessionOperationCommand) (SessionOperationProjection, error)
}
type SessionOperationGetter interface {
	GetOperation(context.Context, SessionOperationRouteCommand) (SessionOperationProjection, error)
}
type SessionOperationEventSource interface {
	OperationEvents(context.Context, SessionOperationRouteCommand) ([]SessionOperationEvent, error)
}
type SessionOperationCanceler interface {
	CancelOperation(context.Context, SessionOperationRouteCommand) (SessionOperationProjection, error)
}
type SessionConfigurationGetter interface {
	SessionConfiguration(context.Context, sandboxidentity.SessionRequest) (SessionConfigurationProjection, error)
}
type SessionConfigurationApplier interface {
	ApplySessionConfiguration(context.Context, SessionConfigurationCommand) (SessionConfigurationProjection, error)
}

type SessionHTTPConfig struct {
	DeploymentID            string
	AuthToken               string
	RequestTimeout          time.Duration
	Now                     func() time.Time
	AllowedEnvironmentNames []string
}

type SessionHTTPDependencies struct {
	Readiness SessionCoreReadiness
	Verifier  SessionIdentityVerifier
	Service   any
}

type SessionHTTPHandler struct {
	config          SessionHTTPConfig
	readiness       SessionCoreReadiness
	verifier        SessionIdentityVerifier
	service         any
	allowedEnvNames []string
}

func NewSessionHTTPHandler(config SessionHTTPConfig, dependencies SessionHTTPDependencies) (*SessionHTTPHandler, error) {
	if !validSessionProtocolIdentifier(config.DeploymentID) || len(config.AuthToken) < 16 || config.Now == nil ||
		dependencies.Readiness == nil || dependencies.Verifier == nil || dependencies.Service == nil {
		return nil, ErrConfiguration
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = defaultSessionRequestTimeout
	}
	if config.RequestTimeout <= 0 || config.RequestTimeout > time.Minute {
		return nil, ErrConfiguration
	}
	allowedEnvNames, err := canonicalAllowedEnvironmentNames(config.AllowedEnvironmentNames)
	if err != nil {
		return nil, ErrConfiguration
	}
	return &SessionHTTPHandler{config: config, readiness: dependencies.Readiness, verifier: dependencies.Verifier, service: dependencies.Service,
		allowedEnvNames: allowedEnvNames}, nil
}

type sessionRouteKind uint8

const (
	sessionRouteAcquire sessionRouteKind = iota + 1
	sessionRouteGet
	sessionRouteRelease
	sessionRouteDestroy
	sessionRouteRecover
	sessionRouteSubmitOperation
	sessionRouteGetOperation
	sessionRouteEvents
	sessionRouteCancelOperation
	sessionRouteGetConfiguration
	sessionRoutePutConfiguration
)

type sessionRoute struct {
	kind        sessionRouteKind
	sessionID   string
	operationID string
}

func parseSessionRoute(method, requestPath string) (sessionRoute, bool) {
	if method == http.MethodPost && requestPath == "/v1/sessions:acquire" {
		return sessionRoute{kind: sessionRouteAcquire}, true
	}
	if method == http.MethodGet && requestPath == "/v1/session-configuration" {
		return sessionRoute{kind: sessionRouteGetConfiguration}, true
	}
	if method == http.MethodPut && requestPath == "/v1/session-configuration" {
		return sessionRoute{kind: sessionRoutePutConfiguration}, true
	}
	const prefix = "/v1/sessions/"
	if !strings.HasPrefix(requestPath, prefix) {
		return sessionRoute{}, false
	}
	remainder := strings.TrimPrefix(requestPath, prefix)
	if remainder == "" || strings.HasSuffix(remainder, "/") || strings.Contains(remainder, "//") {
		return sessionRoute{}, false
	}
	if !strings.Contains(remainder, "/") {
		for suffix, kind := range map[string]sessionRouteKind{":release": sessionRouteRelease, ":destroy": sessionRouteDestroy, ":recover": sessionRouteRecover} {
			if strings.HasSuffix(remainder, suffix) && method == http.MethodPost {
				id := strings.TrimSuffix(remainder, suffix)
				if validSessionProtocolIdentifier(id) {
					return sessionRoute{kind: kind, sessionID: id}, true
				}
			}
		}
		if method == http.MethodGet && validSessionProtocolIdentifier(remainder) {
			return sessionRoute{kind: sessionRouteGet, sessionID: remainder}, true
		}
		return sessionRoute{}, false
	}
	parts := strings.Split(remainder, "/")
	if len(parts) == 2 && parts[1] == "operations" && method == http.MethodPost && validSessionProtocolIdentifier(parts[0]) {
		return sessionRoute{kind: sessionRouteSubmitOperation, sessionID: parts[0]}, true
	}
	if len(parts) == 4 && parts[1] == "operations" && parts[3] == "events" && method == http.MethodGet &&
		validSessionProtocolIdentifier(parts[0]) && validSessionProtocolIdentifier(parts[2]) {
		return sessionRoute{kind: sessionRouteEvents, sessionID: parts[0], operationID: parts[2]}, true
	}
	if len(parts) != 3 || parts[1] != "operations" || !validSessionProtocolIdentifier(parts[0]) {
		return sessionRoute{}, false
	}
	operationID := parts[2]
	if method == http.MethodPost && strings.HasSuffix(operationID, ":cancel") {
		operationID = strings.TrimSuffix(operationID, ":cancel")
		if validSessionProtocolIdentifier(operationID) {
			return sessionRoute{kind: sessionRouteCancelOperation, sessionID: parts[0], operationID: operationID}, true
		}
	}
	if method == http.MethodGet && validSessionProtocolIdentifier(operationID) {
		return sessionRoute{kind: sessionRouteGetOperation, sessionID: parts[0], operationID: operationID}, true
	}
	return sessionRoute{}, false
}

func (handler *SessionHTTPHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if handler == nil || request == nil {
		writePublicError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	route, ok := parseSessionRouteWithEvents(request.Method, request.URL.Path)
	if !ok {
		writePublicError(writer, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if request.URL.RawQuery != "" {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if !authenticateBearer(request, handler.config.AuthToken) {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	body, err := readSessionRequestBody(writer, request, route)
	if err != nil {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	parsedAcquire, operation, configuration, err := handler.parseRouteBody(route, body)
	if err != nil {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	digest := sessionRequestDigest(body)
	contextHeader := request.Header.Get(sandboxidentity.SessionContextHeader)
	signatureHeader := request.Header.Get(sandboxidentity.SessionContextSignatureHeader)
	if contextHeader == "" || signatureHeader == "" {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	authCtx, cancelAuth := context.WithTimeout(request.Context(), handler.config.RequestTimeout)
	defer cancelAuth()
	targetAudience := sandboxidentity.SessionContextAudienceProvider
	if route.kind == sessionRouteGetConfiguration || route.kind == sessionRoutePutConfiguration {
		targetAudience = sandboxidentity.SessionContextAudienceRunner
	}
	claims, err := handler.verifier.VerifySession(authCtx, SessionVerifyInput{
		ContextHeader: contextHeader, SignatureHeader: signatureHeader,
		Target: sandboxidentity.SessionContextTarget{DeploymentID: handler.config.DeploymentID, Audience: targetAudience},
		Method: request.Method, Path: request.URL.Path, RequestDigest: digest,
	})
	if err != nil || claims.DeploymentID != handler.config.DeploymentID || !validSessionClaims(claims, digest) {
		writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	ctx := authCtx
	cancelOperation := func() {}
	if route.kind == sessionRouteSubmitOperation {
		if err := http.NewResponseController(writer).SetWriteDeadline(operation.Deadline.Add(sessionOperationWriteGrace)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			writePublicError(writer, http.StatusServiceUnavailable, "SANDBOX_UNAVAILABLE")
			return
		}
		ctx, cancelOperation = context.WithDeadline(request.Context(), operation.Deadline)
	}
	defer cancelOperation()
	if route.kind == sessionRouteAcquire {
		if !claimsMatchAcquire(claims, parsedAcquire) {
			writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
			return
		}
	} else if route.sessionID != "" {
		if route.operationID != "" && claims.OperationID != route.operationID {
			writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
			return
		}
		resolver, ok := handler.service.(SessionIdentityResolver)
		if !ok {
			writePublicError(writer, http.StatusServiceUnavailable, "SANDBOX_UNAVAILABLE")
			return
		}
		expectedIdentity := SessionStableIdentity{DeploymentID: handler.config.DeploymentID, ProviderID: claims.ProviderID, SpaceID: claims.SpaceID,
			UserID: claims.UserID, ThreadID: claims.ThreadID, Profile: domainsandbox.SessionProfile(claims.Profile)}
		identity, resolveErr := resolver.ResolveSessionIdentity(ctx, route.sessionID, expectedIdentity)
		if resolveErr != nil || identity.DeploymentID != handler.config.DeploymentID || !stableIdentityMatchesClaims(identity, claims) {
			handler.writeServiceError(writer, resolveErr)
			return
		}
	}
	if err := handler.readiness.CoreReady(ctx); err != nil {
		writePublicError(writer, http.StatusServiceUnavailable, "SANDBOX_UNAVAILABLE")
		return
	}
	handler.dispatch(writer, ctx, route, claims, parsedAcquire, operation, configuration, request.Header.Get("Last-Event-ID"))
}

func parseSessionRouteWithEvents(method, requestPath string) (sessionRoute, bool) {
	return parseSessionRoute(method, requestPath)
}

func readSessionRequestBody(writer http.ResponseWriter, request *http.Request, route sessionRoute) ([]byte, error) {
	mutation := request.Method == http.MethodPost || request.Method == http.MethodPut
	if mutation {
		mediaType, parameters, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" || len(parameters) != 0 {
			return nil, errSessionProtocol
		}
	}
	body := http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	defer body.Close()
	raw, err := io.ReadAll(body)
	if err != nil || len(raw) > maxRequestBytes {
		return nil, errSessionProtocol
	}
	if !mutation && len(raw) != 0 {
		return nil, errSessionProtocol
	}
	if mutation && len(raw) == 0 {
		return nil, errSessionProtocol
	}
	return raw, nil
}

func (handler *SessionHTTPHandler) parseRouteBody(route sessionRoute, body []byte) (parsedSessionAcquire, SessionOperationCommand, SessionConfigurationCommand, error) {
	var acquire parsedSessionAcquire
	var operation SessionOperationCommand
	var configuration SessionConfigurationCommand
	var err error
	switch route.kind {
	case sessionRouteAcquire:
		acquire, err = parseSessionAcquire(body, handler.config.DeploymentID)
	case sessionRouteSubmitOperation:
		operation, err = parseSessionOperation(body, handler.config.Now(), handler.allowedEnvNames...)
	case sessionRoutePutConfiguration:
		configuration, err = parseSessionConfiguration(body)
	case sessionRouteRelease, sessionRouteDestroy, sessionRouteRecover, sessionRouteCancelOperation:
		if !strictEmptyJSONObject(body) {
			err = errSessionProtocol
		}
	case sessionRouteGet, sessionRouteGetOperation, sessionRouteEvents, sessionRouteGetConfiguration:
		if len(body) != 0 {
			err = errSessionProtocol
		}
	default:
		err = errSessionProtocol
	}
	return acquire, operation, configuration, err
}

func (handler *SessionHTTPHandler) dispatch(writer http.ResponseWriter, ctx context.Context, route sessionRoute, claims sandboxidentity.SessionRequest, acquire parsedSessionAcquire, operation SessionOperationCommand, configuration SessionConfigurationCommand, afterEventID string) {
	command := SessionRouteCommand{SessionID: route.sessionID, Claims: claims}
	switch route.kind {
	case sessionRouteAcquire:
		service, ok := handler.service.(SessionAcquirer)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		projection, err := service.Acquire(ctx, SessionAcquireCommand{Identity: acquire.Identity, Claims: claims})
		handler.writeProjection(writer, http.StatusOK, projection, err)
	case sessionRouteGet:
		service, ok := handler.service.(SessionGetter)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		projection, err := service.Get(ctx, command)
		handler.writeProjection(writer, http.StatusOK, projection, err)
	case sessionRouteRelease:
		service, ok := handler.service.(SessionReleaser)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		if err := service.Release(ctx, command); err != nil {
			handler.writeServiceError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	case sessionRouteDestroy:
		service, ok := handler.service.(SessionDestroyer)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		if err := service.Destroy(ctx, command); err != nil {
			handler.writeServiceError(writer, err)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	case sessionRouteRecover:
		service, ok := handler.service.(SessionRecoverer)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		projection, err := service.Recover(ctx, command)
		handler.writeProjection(writer, http.StatusOK, projection, err)
	case sessionRouteSubmitOperation:
		service, ok := handler.service.(SessionOperationSubmitter)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		operation.SessionID, operation.Claims = route.sessionID, claims
		if claims.OperationID != operation.OperationID {
			writePublicError(writer, http.StatusUnauthorized, "UNAUTHORIZED")
			return
		}
		projection, err := service.SubmitOperation(ctx, operation)
		handler.writeOperationProjection(writer, http.StatusAccepted, route.sessionID, operation.OperationID, projection, err)
	case sessionRouteGetOperation:
		handler.dispatchGetOperation(writer, ctx, route, claims)
	case sessionRouteEvents:
		handler.dispatchEvents(writer, ctx, route, claims, afterEventID)
	case sessionRouteCancelOperation:
		service, ok := handler.service.(SessionOperationCanceler)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		projection, err := service.CancelOperation(ctx, SessionOperationRouteCommand{SessionID: route.sessionID, OperationID: route.operationID, Claims: claims})
		handler.writeOperationProjection(writer, http.StatusOK, route.sessionID, route.operationID, projection, err)
	case sessionRouteGetConfiguration:
		service, ok := handler.service.(SessionConfigurationGetter)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		projection, err := service.SessionConfiguration(ctx, claims)
		handler.writeConfigurationProjection(writer, projection, err)
	case sessionRoutePutConfiguration:
		service, ok := handler.service.(SessionConfigurationApplier)
		if !ok {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		configuration.Claims = claims
		projection, err := service.ApplySessionConfiguration(ctx, configuration)
		handler.writeConfigurationProjection(writer, projection, err)
	}
}

func (handler *SessionHTTPHandler) dispatchGetOperation(writer http.ResponseWriter, ctx context.Context, route sessionRoute, claims sandboxidentity.SessionRequest) {
	service, ok := handler.service.(SessionOperationGetter)
	if !ok {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	projection, err := service.GetOperation(ctx, SessionOperationRouteCommand{SessionID: route.sessionID, OperationID: route.operationID, Claims: claims})
	handler.writeOperationProjection(writer, http.StatusOK, route.sessionID, route.operationID, projection, err)
}

func (handler *SessionHTTPHandler) dispatchEvents(writer http.ResponseWriter, ctx context.Context, route sessionRoute, claims sandboxidentity.SessionRequest, afterEventID string) {
	if afterEventID != "" && !validSessionProtocolIdentifier(afterEventID) {
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	service, ok := handler.service.(SessionOperationEventSource)
	if !ok {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	events, err := service.OperationEvents(ctx, SessionOperationRouteCommand{SessionID: route.sessionID, OperationID: route.operationID, Claims: claims, AfterEventID: afterEventID})
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	if len(events) > 2 {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	for index, event := range events {
		if event.Schema != sessionEventSchemaV1 || event.OperationID != route.operationID || !validSessionProtocolIdentifier(event.EventID) ||
			(event.State != SessionOperationAccepted && !terminalSessionOperationState(event.State)) || event.State == SessionOperationRunning || len(event.ReasonCode) > 64 {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		if index == 0 && event.State != SessionOperationAccepted && !(afterEventID != "" && terminalSessionOperationState(event.State)) ||
			index == 1 && !terminalSessionOperationState(event.State) {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
		if err := encoder.Encode(event); err != nil || output.Len() > maxSessionEventsBytes {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
	}
	writer.Header().Set("Content-Type", "application/x-ndjson")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(output.Bytes())
}

func (handler *SessionHTTPHandler) writeProjection(writer http.ResponseWriter, status int, projection SessionProjection, err error) {
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	if projection.Schema != sessionProjectionSchemaV1 || !validSessionProtocolIdentifier(projection.SessionID) || projection.RuntimeGeneration == 0 ||
		projection.Profile != domainsandbox.SessionProfileCore || (projection.State != domainsandbox.SessionStateActive && projection.State != domainsandbox.SessionStateRecovering && projection.State != domainsandbox.SessionStateReleased && projection.State != domainsandbox.SessionStateDestroyed) {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	handler.writeBoundedJSON(writer, status, projection)
}

func (handler *SessionHTTPHandler) writeOperationProjection(writer http.ResponseWriter, status int, expectedSessionID, expectedOperationID string, projection SessionOperationProjection, err error) {
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	if projection.Schema != sessionOperationSchemaV1 || projection.SessionID != expectedSessionID || projection.OperationID != expectedOperationID ||
		!validSessionProtocolIdentifier(projection.SessionID) || !validSessionProtocolIdentifier(projection.OperationID) ||
		!validSessionOperationKind(projection.Kind) || !validSessionOperationState(projection.State) || len(projection.ReasonCode) > 64 ||
		(projection.ResultDigest != "" && !validSHA256Digest(projection.ResultDigest)) {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	if len(projection.Result) != 0 {
		if projection.State != SessionOperationSucceeded || len(projection.Result) > maxSessionInlineResultBytes ||
			!json.Valid(projection.Result) || !sessionResultDigestMatches(projection.Result, projection.ResultDigest) {
			handler.writeServiceError(writer, ErrUnavailable)
			return
		}
	}
	handler.writeBoundedJSON(writer, status, projection)
}

func (handler *SessionHTTPHandler) writeConfigurationProjection(writer http.ResponseWriter, projection SessionConfigurationProjection, err error) {
	if err != nil {
		handler.writeServiceError(writer, err)
		return
	}
	settings, settingsErr := domainsandbox.NormalizeSessionRuntimeSettings(projection.Settings)
	if projection.Schema != sessionConfigurationSchemaV1 || projection.Version == 0 || settingsErr != nil || settings != projection.Settings {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	handler.writeBoundedJSON(writer, http.StatusOK, projection)
}

func validSHA256Digest(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func sessionResultDigestMatches(result json.RawMessage, encodedDigest string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(encodedDigest)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	digest := sha256.Sum256(result)
	return hmac.Equal(decoded, digest[:])
}

func (handler *SessionHTTPHandler) writeBoundedJSON(writer http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil || len(body)+1 > maxSessionResponseBytes {
		handler.writeServiceError(writer, ErrUnavailable)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(append(body, '\n'))
}

func (handler *SessionHTTPHandler) writeServiceError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainsandbox.ErrSessionNotFound):
		writePublicError(writer, http.StatusNotFound, domainsandbox.ErrCodeSessionNotFound)
	case errors.Is(err, domainsandbox.ErrVersionConflict):
		writePublicError(writer, http.StatusConflict, domainsandbox.ErrCodeVersionConflict)
	case errors.Is(err, domainsandbox.ErrInvalidInput), errors.Is(err, errSessionProtocol), errors.Is(err, ErrProtocol):
		writePublicError(writer, http.StatusBadRequest, "INVALID_REQUEST")
	case errors.Is(err, domainsandbox.ErrCapacityExhausted):
		writePublicError(writer, http.StatusTooManyRequests, domainsandbox.ErrCodeCapacityExhausted)
	default:
		writePublicError(writer, http.StatusServiceUnavailable, domainsandbox.ErrCodeUnavailable)
	}
}
