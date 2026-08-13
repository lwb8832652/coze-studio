// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSessionHTTPRouteMatrixRejectsUnfrozenRenewAndPublish(t *testing.T) {
	handler := newSessionTestHandler(t, &recordingSessionHTTPDependencies{})
	for _, request := range []struct{ method, path string }{
		{http.MethodPost, "/v1/sessions:acquire"},
		{http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000"},
		{http.MethodPost, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000:release"},
		{http.MethodPost, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000:destroy"},
		{http.MethodPost, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000:recover"},
		{http.MethodPost, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations"},
		{http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01"},
		{http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01/events"},
		{http.MethodPost, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01:cancel"},
		{http.MethodGet, "/v1/session-configuration"},
		{http.MethodPut, "/v1/session-configuration"},
	} {
		if _, ok := parseSessionRoute(request.method, request.path); !ok {
			t.Errorf("frozen route rejected: %s %s", request.method, request.path)
		}
	}
	for _, path := range []string{
		"/v1/sessions/550e8400-e29b-41d4-a716-446655440000:renew",
		"/v1/sessions/550e8400-e29b-41d4-a716-446655440000:publish",
		"/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01/",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if response.Code != http.StatusNotFound {
			t.Errorf("unfrozen route %q status = %d", path, response.Code)
		}
	}
}

func TestSessionHTTPFailsClosedWhenCoreIsNotReadyAfterAuthentication(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{readyErr: errors.New("redis secret detail")}
	handler := newSessionTestHandler(t, dependencies)
	request := sessionAcquireRequest(t, validAcquireSessionBody())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || dependencies.verifyCalls != 1 || dependencies.acquireCalls != 0 {
		t.Fatalf("status/verify/acquire = %d/%d/%d", response.Code, dependencies.verifyCalls, dependencies.acquireCalls)
	}
	assertSessionErrorRedacted(t, response.Body.String())
}

func TestSessionHTTPVerifierFailureDoesNotResolveSessionExistence(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{verifyErr: errors.New("signature secret detail")}
	handler := newSessionTestHandler(t, dependencies)
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000", nil)
	setSessionTestHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || dependencies.resolveCalls != 0 || dependencies.getCalls != 0 {
		t.Fatalf("status/resolve/get = %d/%d/%d", response.Code, dependencies.resolveCalls, dependencies.getCalls)
	}
	assertSessionErrorRedacted(t, response.Body.String())
}

func TestSessionHTTPAcquireStrictBodyBindsRunnerDeploymentAndVerifiedClaims(t *testing.T) {
	for name, body := range map[string]string{
		"unknown":    strings.TrimSuffix(validAcquireSessionBody(), "}") + `,"physical_workspace":"/mnt/user-data/other"}`,
		"duplicate":  strings.Replace(validAcquireSessionBody(), `"provider_id":41`, `"provider_id":41,"provider_id":41`, 1),
		"deployment": strings.Replace(validAcquireSessionBody(), `"deployment_id":"runner-a"`, `"deployment_id":"runner-b"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			dependencies := &recordingSessionHTTPDependencies{}
			handler := newSessionTestHandler(t, dependencies)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, sessionAcquireRequest(t, body))
			if response.Code != http.StatusBadRequest || dependencies.acquireCalls != 0 {
				t.Fatalf("status/acquire = %d/%d", response.Code, dependencies.acquireCalls)
			}
			assertSessionErrorRedacted(t, response.Body.String())
		})
	}

	dependencies := &recordingSessionHTTPDependencies{claimMutator: func(claims *sandboxidentity.SessionRequest) { claims.UserID++ }}
	handler := newSessionTestHandler(t, dependencies)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, sessionAcquireRequest(t, validAcquireSessionBody()))
	if response.Code != http.StatusUnauthorized || dependencies.acquireCalls != 0 {
		t.Fatalf("mismatched claims status/acquire = %d/%d", response.Code, dependencies.acquireCalls)
	}
}

func TestSessionHTTPGetUsesEmptyBodyDigestAndRejectsMismatchedStoredIdentity(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{}
	handler := newSessionTestHandler(t, dependencies)
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000", nil)
	setSessionTestHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	emptyDigest := sha256.Sum256(nil)
	if response.Code != http.StatusOK || !hmac.Equal(dependencies.lastVerify.RequestDigest, emptyDigest[:]) || dependencies.getCalls != 1 {
		t.Fatalf("status/digest/get = %d/%x/%d", response.Code, dependencies.lastVerify.RequestDigest, dependencies.getCalls)
	}
	wantResolved := SessionStableIdentity{DeploymentID: "runner-a", ProviderID: 41, SpaceID: 42, UserID: 43, ThreadID: "thread_01", Profile: domainsandbox.SessionProfileCore}
	if dependencies.lastResolveExpected != wantResolved {
		t.Fatalf("resolver expected identity = %#v", dependencies.lastResolveExpected)
	}

	dependencies = &recordingSessionHTTPDependencies{identityMutator: func(identity *SessionStableIdentity) { identity.UserID++ }}
	handler = newSessionTestHandler(t, dependencies)
	request = httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000", nil)
	setSessionTestHeaders(request)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || dependencies.getCalls != 0 {
		t.Fatalf("mismatched row status/get = %d/%d", response.Code, dependencies.getCalls)
	}
}

func TestSessionHTTPMutationRequiresExactJSONContentTypeBeforeVerification(t *testing.T) {
	for _, contentType := range []string{"", "text/plain", "application/json; charset=utf-8"} {
		dependencies := &recordingSessionHTTPDependencies{}
		handler := newSessionTestHandler(t, dependencies)
		request := httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", strings.NewReader(validAcquireSessionBody()))
		request.Header.Set("Content-Type", contentType)
		setSessionTestHeaders(request)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || dependencies.verifyCalls != 0 {
			t.Fatalf("content type %q status/verify = %d/%d", contentType, response.Code, dependencies.verifyCalls)
		}
	}
}

func TestSessionHTTPEventsExposeOnlyBoundedAcceptedThenTerminal(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{events: []SessionOperationEvent{
		{Schema: sessionEventSchemaV1, EventID: "event_01", OperationID: "operation_01", State: SessionOperationAccepted},
		{Schema: sessionEventSchemaV1, EventID: "event_02", OperationID: "operation_01", State: SessionOperationSucceeded},
	}}
	handler := newSessionTestHandler(t, dependencies)
	request := httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01/events", nil)
	request.Header.Set("Last-Event-ID", "event_01")
	setSessionTestHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/x-ndjson" || dependencies.lastAfterEventID != "event_01" {
		t.Fatalf("status/content-type/resume = %d/%q/%q", response.Code, response.Header().Get("Content-Type"), dependencies.lastAfterEventID)
	}
	lines := strings.Split(strings.TrimSpace(response.Body.String()), "\n")
	if len(lines) != 2 || strings.Contains(response.Body.String(), "running") {
		t.Fatalf("events = %q", response.Body.String())
	}
	for _, line := range lines {
		var event SessionOperationEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("event JSON = %q: %v", line, err)
		}
	}

	dependencies.events = []SessionOperationEvent{{Schema: sessionEventSchemaV1, EventID: "event_02", OperationID: "operation_01", State: SessionOperationSucceeded}}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01/events", nil)
	request.Header.Set("Last-Event-ID", "event_01")
	setSessionTestHeaders(request)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Count(strings.TrimSpace(response.Body.String()), "\n") != 0 {
		t.Fatalf("terminal-only resume status/body = %d/%q", response.Code, response.Body.String())
	}

	dependencies.events = []SessionOperationEvent{{Schema: sessionEventSchemaV1, EventID: "event_02", OperationID: "operation_01", State: SessionOperationRunning}}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01/events", nil)
	setSessionTestHeaders(request)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "running") {
		t.Fatalf("running event status/body = %d/%s", response.Code, response.Body.String())
	}

	dependencies.events = []SessionOperationEvent{
		{Schema: sessionEventSchemaV1, EventID: "event_02", OperationID: "operation_01", State: SessionOperationSucceeded},
		{Schema: sessionEventSchemaV1, EventID: "event_01", OperationID: "operation_01", State: SessionOperationAccepted},
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations/operation_01/events", nil)
	setSessionTestHeaders(request)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("out-of-order events status/body = %d/%s", response.Code, response.Body.String())
	}
}

func TestSessionHTTPResponseProjectionRejectsMismatchedIdentifiersAndConfiguration(t *testing.T) {
	handler := &SessionHTTPHandler{}
	operation := httptest.NewRecorder()
	handler.writeOperationProjection(operation, http.StatusOK, "session_expected", "operation_expected", SessionOperationProjection{
		Schema: sessionOperationSchemaV1, SessionID: "session_other", OperationID: "operation_expected",
		Kind: SessionOperationRead, State: SessionOperationSucceeded,
	}, nil)
	if operation.Code != http.StatusServiceUnavailable || strings.Contains(operation.Body.String(), "session_other") {
		t.Fatalf("operation projection status/body = %d/%s", operation.Code, operation.Body.String())
	}

	configuration := httptest.NewRecorder()
	handler.writeConfigurationProjection(configuration, SessionConfigurationProjection{
		Schema: "wrong.schema", Version: 1, Settings: domainsandbox.DefaultSessionRuntimeSettings(),
	}, nil)
	if configuration.Code != http.StatusServiceUnavailable || strings.Contains(configuration.Body.String(), "wrong.schema") {
		t.Fatalf("configuration projection status/body = %d/%s", configuration.Code, configuration.Body.String())
	}
}

func TestSessionHTTPOperationProjectionValidatesInlineResultDigestAndBound(t *testing.T) {
	handler := &SessionHTTPHandler{}
	result := json.RawMessage(`{"data":"b2s="}`)
	digest := sha256.Sum256(result)
	validDigest := base64.RawURLEncoding.EncodeToString(digest[:])

	valid := httptest.NewRecorder()
	handler.writeOperationProjection(valid, http.StatusOK, "session_expected", "operation_expected", SessionOperationProjection{
		Schema: sessionOperationSchemaV1, SessionID: "session_expected", OperationID: "operation_expected",
		Kind: SessionOperationRead, State: SessionOperationSucceeded, ResultDigest: validDigest, Result: result,
	}, nil)
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), `"result":{"data":"b2s="}`) {
		t.Fatalf("valid inline result status/body = %d/%s", valid.Code, valid.Body.String())
	}

	tooLarge := json.RawMessage(`"` + strings.Repeat("a", maxSessionInlineResultBytes) + `"`)
	tooLargeDigest := sha256.Sum256(tooLarge)
	for name, projection := range map[string]SessionOperationProjection{
		"digest mismatch": {
			Schema: sessionOperationSchemaV1, SessionID: "session_expected", OperationID: "operation_expected",
			Kind: SessionOperationRead, State: SessionOperationSucceeded,
			ResultDigest: base64.RawURLEncoding.EncodeToString(make([]byte, sha256.Size)), Result: result,
		},
		"result on failure": {
			Schema: sessionOperationSchemaV1, SessionID: "session_expected", OperationID: "operation_expected",
			Kind: SessionOperationRead, State: SessionOperationFailed, ResultDigest: validDigest, Result: result,
		},
		"result above cap": {
			Schema: sessionOperationSchemaV1, SessionID: "session_expected", OperationID: "operation_expected",
			Kind: SessionOperationRead, State: SessionOperationSucceeded,
			ResultDigest: base64.RawURLEncoding.EncodeToString(tooLargeDigest[:]), Result: tooLarge,
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.writeOperationProjection(response, http.StatusOK, "session_expected", "operation_expected", projection, nil)
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "b2s") {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSessionHTTPAllowedEnvironmentNamesRejectDuplicatesAndCanonicalize(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{}
	base := SessionHTTPConfig{DeploymentID: "runner-a", AuthToken: "runner-auth-token-0123456789", Now: time.Now}
	for name, names := range map[string][]string{
		"duplicate": {"LANG", "LANG"},
		"invalid":   {"NOT-AN-ENV"},
	} {
		t.Run(name, func(t *testing.T) {
			config := base
			config.AllowedEnvironmentNames = names
			if _, err := NewSessionHTTPHandler(config, SessionHTTPDependencies{Readiness: dependencies, Verifier: dependencies, Service: dependencies}); !errors.Is(err, ErrConfiguration) {
				t.Fatalf("NewSessionHTTPHandler(%v) = %v", names, err)
			}
		})
	}
	base.AllowedEnvironmentNames = []string{"TZ", "LANG"}
	handler, err := NewSessionHTTPHandler(base, SessionHTTPDependencies{Readiness: dependencies, Verifier: dependencies, Service: dependencies})
	if err != nil || strings.Join(handler.allowedEnvNames, ",") != "LANG,TZ" {
		t.Fatalf("NewSessionHTTPHandler() allowed env = %v, %v", handler.allowedEnvNames, err)
	}
}

func TestSessionHTTPSubmitUsesBoundedOperationAndWriteDeadlines(t *testing.T) {
	now := time.Now().UTC()
	operationDeadline := now.Add(45 * time.Minute).Truncate(time.Millisecond)
	dependencies := &recordingSessionHTTPDependencies{}
	handler, err := NewSessionHTTPHandler(SessionHTTPConfig{
		DeploymentID: "runner-a", AuthToken: "runner-auth-token-0123456789", Now: func() time.Time { return now },
		AllowedEnvironmentNames: []string{"LANG"},
	}, SessionHTTPDependencies{Readiness: dependencies, Verifier: dependencies, Service: dependencies})
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"schema":"coze.sandbox.session_operation.v1","operation_id":"request_01","kind":"read","deadline":%q,"payload":{"path":"/mnt/user-data/workspace/result.txt","max_bytes":1}}`, operationDeadline.Format(time.RFC3339Nano))
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions/550e8400-e29b-41d4-a716-446655440000/operations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	setSessionTestHeaders(request)
	response := &deadlineRecordingResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || dependencies.submitCalls != 1 {
		t.Fatalf("status/submit = %d/%d; body=%s", response.Code, dependencies.submitCalls, response.Body.String())
	}
	if !dependencies.lastSubmitDeadline.Equal(operationDeadline) {
		t.Fatalf("submit context deadline = %v, want %v", dependencies.lastSubmitDeadline, operationDeadline)
	}
	verifyRemaining := dependencies.lastVerifyDeadline.Sub(now)
	if verifyRemaining < defaultSessionRequestTimeout-time.Second || verifyRemaining > defaultSessionRequestTimeout+time.Second {
		t.Fatalf("submit verification timeout = %v", verifyRemaining)
	}
	wantWriteDeadline := operationDeadline.Add(sessionOperationWriteGrace)
	if !response.writeDeadline.Equal(wantWriteDeadline) {
		t.Fatalf("write deadline = %v, want %v", response.writeDeadline, wantWriteDeadline)
	}
}

func TestSessionHTTPNonSubmitRouteKeepsConfiguredRequestTimeout(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{}
	handler := newSessionTestHandler(t, dependencies)
	request := sessionAcquireRequest(t, validAcquireSessionBody())
	response := &deadlineRecordingResponseWriter{ResponseRecorder: httptest.NewRecorder()}
	started := time.Now()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || dependencies.lastAcquireDeadline.IsZero() {
		t.Fatalf("status/acquire deadline = %d/%v", response.Code, dependencies.lastAcquireDeadline)
	}
	remaining := dependencies.lastAcquireDeadline.Sub(started)
	if remaining < defaultSessionRequestTimeout-time.Second || remaining > defaultSessionRequestTimeout+time.Second {
		t.Fatalf("non-submit timeout = %v", remaining)
	}
	if !response.writeDeadline.IsZero() {
		t.Fatalf("non-submit write deadline = %v", response.writeDeadline)
	}
}

func TestSessionHTTPConfigurationAuthenticatesAndChecksCoreWithoutResolvingSession(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{}
	handler := newSessionTestHandler(t, dependencies)
	request := httptest.NewRequest(http.MethodGet, "/v1/session-configuration", nil)
	setSessionTestHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || dependencies.verifyCalls != 1 || dependencies.readyCalls != 1 || dependencies.resolveCalls != 0 || dependencies.configurationCalls != 1 {
		t.Fatalf("status/verify/ready/resolve/config = %d/%d/%d/%d/%d", response.Code, dependencies.verifyCalls, dependencies.readyCalls, dependencies.resolveCalls, dependencies.configurationCalls)
	}
}

func TestSessionHTTPConfigurationRejectsClaimsForAnotherDeployment(t *testing.T) {
	dependencies := &recordingSessionHTTPDependencies{claimMutator: func(claims *sandboxidentity.SessionRequest) {
		claims.DeploymentID = "runner-b"
	}}
	handler := newSessionTestHandler(t, dependencies)
	request := httptest.NewRequest(http.MethodGet, "/v1/session-configuration", nil)
	setSessionTestHeaders(request)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || dependencies.configurationCalls != 0 {
		t.Fatalf("status/config = %d/%d", response.Code, dependencies.configurationCalls)
	}
}

func TestSessionContextIdentityVerifierUsesAuthenticatedV2EnvelopeAndNonce(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring, err := sandboxidentity.NewKeyring("key_01", map[string][]byte{"key_01": []byte("0123456789abcdef0123456789abcdef")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keyring.Now = func() time.Time { return now }
	keyring.Nonce = func() (string, error) { return "nonce_01", nil }
	digest := sha256.Sum256(nil)
	claims := sandboxidentity.SessionRequest{DeploymentID: "runner-a", ProviderID: 41, Scope: sandboxidentity.ScopeAgent, SpaceID: 42, UserID: 43, ThreadID: "thread_01", RunID: "run_01", OperationID: "request_01", Profile: "core", RequestDigest: digest[:]}
	signed, err := keyring.SignSession(claims, http.MethodGet, "/v1/session-configuration")
	if err != nil {
		t.Fatal(err)
	}
	nonces := &memorySessionNonceStore{seen: map[string]struct{}{}}
	verifier, err := NewSessionContextIdentityVerifier(keyring, nonces, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	input := SessionVerifyInput{ContextHeader: signed.Context, SignatureHeader: signed.Signature,
		Target: sandboxidentity.SessionContextTarget{DeploymentID: "runner-a", Audience: sandboxidentity.SessionContextAudienceRunner},
		Method: http.MethodGet, Path: "/v1/session-configuration", RequestDigest: digest[:]}
	verified, err := verifier.VerifySession(context.Background(), input)
	if err != nil || verified.UserID != claims.UserID {
		t.Fatalf("VerifySession() = %#v, %v", verified, err)
	}
	if _, err := verifier.VerifySession(context.Background(), input); err == nil {
		t.Fatal("VerifySession() accepted replayed nonce")
	}
}

type recordingSessionHTTPDependencies struct {
	readyErr            error
	verifyErr           error
	claimMutator        func(*sandboxidentity.SessionRequest)
	identityMutator     func(*SessionStableIdentity)
	verifyCalls         int
	readyCalls          int
	resolveCalls        int
	acquireCalls        int
	submitCalls         int
	getCalls            int
	configurationCalls  int
	lastVerify          SessionVerifyInput
	lastVerifyDeadline  time.Time
	lastResolveExpected SessionStableIdentity
	lastAcquireDeadline time.Time
	lastSubmitDeadline  time.Time
	events              []SessionOperationEvent
	lastAfterEventID    string
}

func (d *recordingSessionHTTPDependencies) CoreReady(context.Context) error {
	d.readyCalls++
	return d.readyErr
}

func (d *recordingSessionHTTPDependencies) VerifySession(ctx context.Context, input SessionVerifyInput) (sandboxidentity.SessionRequest, error) {
	d.verifyCalls++
	d.lastVerify = input
	d.lastVerifyDeadline, _ = ctx.Deadline()
	if d.verifyErr != nil {
		return sandboxidentity.SessionRequest{}, d.verifyErr
	}
	claims := sandboxidentity.SessionRequest{
		DeploymentID: input.Target.DeploymentID, ProviderID: 41, Scope: sandboxidentity.ScopeAgent, SpaceID: 42, UserID: 43,
		ThreadID: "thread_01", RunID: "run_01", OperationID: "request_01", Profile: "core",
		RequestDigest: append([]byte(nil), input.RequestDigest...),
	}
	if route, ok := parseSessionRoute(input.Method, input.Path); ok && route.operationID != "" {
		claims.OperationID = route.operationID
	}
	if d.claimMutator != nil {
		d.claimMutator(&claims)
	}
	return claims, nil
}

func (d *recordingSessionHTTPDependencies) ResolveSessionIdentity(_ context.Context, _ string, expected SessionStableIdentity) (SessionStableIdentity, error) {
	d.resolveCalls++
	d.lastResolveExpected = expected
	identity := SessionStableIdentity{DeploymentID: "runner-a", ProviderID: 41, SpaceID: 42, UserID: 43, ThreadID: "thread_01", Profile: domainsandbox.SessionProfileCore}
	if d.identityMutator != nil {
		d.identityMutator(&identity)
	}
	return identity, nil
}

func (d *recordingSessionHTTPDependencies) Acquire(ctx context.Context, _ SessionAcquireCommand) (SessionProjection, error) {
	d.acquireCalls++
	d.lastAcquireDeadline, _ = ctx.Deadline()
	return SessionProjection{Schema: sessionProjectionSchemaV1, SessionID: "550e8400-e29b-41d4-a716-446655440000", State: domainsandbox.SessionStateActive, RuntimeGeneration: 1, Profile: domainsandbox.SessionProfileCore}, nil
}

func (d *recordingSessionHTTPDependencies) SubmitOperation(ctx context.Context, command SessionOperationCommand) (SessionOperationProjection, error) {
	d.submitCalls++
	d.lastSubmitDeadline, _ = ctx.Deadline()
	return SessionOperationProjection{Schema: sessionOperationSchemaV1, SessionID: command.SessionID, OperationID: command.OperationID, Kind: command.Kind, State: SessionOperationQueued}, nil
}

func (d *recordingSessionHTTPDependencies) Get(context.Context, SessionRouteCommand) (SessionProjection, error) {
	d.getCalls++
	return SessionProjection{Schema: sessionProjectionSchemaV1, SessionID: "550e8400-e29b-41d4-a716-446655440000", State: domainsandbox.SessionStateActive, RuntimeGeneration: 1, Profile: domainsandbox.SessionProfileCore}, nil
}

func (d *recordingSessionHTTPDependencies) OperationEvents(_ context.Context, command SessionOperationRouteCommand) ([]SessionOperationEvent, error) {
	d.lastAfterEventID = command.AfterEventID
	return append([]SessionOperationEvent(nil), d.events...), nil
}

func (d *recordingSessionHTTPDependencies) SessionConfiguration(context.Context, sandboxidentity.SessionRequest) (SessionConfigurationProjection, error) {
	d.configurationCalls++
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	return SessionConfigurationProjection{Schema: sessionConfigurationSchemaV1, Version: 1, Settings: settings}, nil
}

func newSessionTestHandler(t *testing.T, dependencies *recordingSessionHTTPDependencies) http.Handler {
	t.Helper()
	handler, err := NewSessionHTTPHandler(SessionHTTPConfig{
		DeploymentID: "runner-a", AuthToken: "runner-auth-token-0123456789",
		Now: func() time.Time { return time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC) }, AllowedEnvironmentNames: []string{"LANG"},
	}, SessionHTTPDependencies{Readiness: dependencies, Verifier: dependencies, Service: dependencies})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func sessionAcquireRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/sessions:acquire", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	setSessionTestHeaders(request)
	return request
}

func setSessionTestHeaders(request *http.Request) {
	request.Header.Set("Authorization", "Bearer runner-auth-token-0123456789")
	request.Header.Set(sandboxidentity.SessionContextHeader, "signed-context-secret")
	request.Header.Set(sandboxidentity.SessionContextSignatureHeader, "signed-signature-secret")
}

func validAcquireSessionBody() string {
	return `{"schema":"coze.sandbox.session_acquire.v1","deployment_id":"runner-a","provider_id":41,"scope":"agent","space_id":42,"user_id":43,"thread_id":"thread_01","run_id":"run_01","operation_id":"request_01","profile":"core"}`
}

func assertSessionErrorRedacted(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{"secret", "signed-context", "signed-signature", "/mnt/user-data", "redis"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(secret)) {
			t.Fatalf("error leaked %q: %s", secret, body)
		}
	}
}

func digestSessionBody(body string) []byte {
	digest := sha256.Sum256([]byte(body))
	return digest[:]
}

type memorySessionNonceStore struct{ seen map[string]struct{} }

func (store *memorySessionNonceStore) Consume(_ context.Context, keyID, nonce string, _ time.Time) (bool, error) {
	key := keyID + ":" + nonce
	if _, ok := store.seen[key]; ok {
		return false, nil
	}
	store.seen[key] = struct{}{}
	return true, nil
}

type deadlineRecordingResponseWriter struct {
	*httptest.ResponseRecorder
	writeDeadline time.Time
}

func (writer *deadlineRecordingResponseWriter) SetWriteDeadline(deadline time.Time) error {
	writer.writeDeadline = deadline
	return nil
}
