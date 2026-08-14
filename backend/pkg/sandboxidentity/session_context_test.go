// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxidentity

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSessionContextV2UsesIndependentHeadersSchemaAndCompleteIdentity(t *testing.T) {
	if SessionContextHeader == "" || SessionContextSignatureHeader == "" ||
		SessionContextHeader == "X-Coze-Sandbox-Execution-Context" ||
		SessionContextSignatureHeader == "X-Coze-Sandbox-Execution-Context-Signature" {
		t.Fatal("session context v2 must use independent headers")
	}
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession() error = %v", err)
	}
	body, err := base64.RawURLEncoding.DecodeString(signed.Context)
	if err != nil {
		t.Fatalf("decode context: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("decode v2 envelope: %v", err)
	}
	want := []string{
		"schema", "issuer", "audience", "key_id", "issued_at_unix", "expires_at_unix", "nonce", "provider_id", "scope",
		"space_id", "user_id", "thread_id", "run_id", "operation_id", "profile", "method", "path", "request_digest",
	}
	if len(fields) != len(want) {
		t.Fatalf("v2 field count = %d, want %d: %s", len(fields), len(want), body)
	}
	for _, field := range want {
		if _, ok := fields[field]; !ok {
			t.Fatalf("v2 envelope missing %q: %s", field, body)
		}
	}
	if got := string(fields["schema"]); got != `"coze.sandbox.session_context.v2"` {
		t.Fatalf("schema = %s", got)
	}
}

func TestSessionContextV2BindsFixedIssuerAndProviderAudience(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "POST", "/v1/sessions:acquire")
	if err != nil {
		t.Fatalf("SignSession() error = %v", err)
	}
	body, err := base64.RawURLEncoding.DecodeString(signed.Context)
	if err != nil {
		t.Fatalf("decode context: %v", err)
	}
	var envelope struct {
		Issuer   string `json:"issuer"`
		Audience struct {
			Kind         string `json:"kind"`
			DeploymentID string `json:"deployment_id"`
			ProviderID   int64  `json:"provider_id"`
		} `json:"audience"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode v2 envelope: %v", err)
	}
	if envelope.Issuer != "coze-sandbox-control-plane" || envelope.Audience.Kind != "provider" ||
		envelope.Audience.DeploymentID != "runner-a" || envelope.Audience.ProviderID != request.ProviderID {
		t.Fatalf("issuer/audience = %#v", envelope)
	}
}

func TestSessionContextV2VerifiesCanonicalRequestAndConsumesNonceOnce(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession() error = %v", err)
	}
	store := newFakeSessionNonceStore()
	got, err := keyring.VerifySession(context.Background(), signed.Context, signed.Signature, request,
		"POST", "/v1/sessions/session_01/operations", now, store)
	if err != nil {
		t.Fatalf("VerifySession() error = %v", err)
	}
	if !sameSessionRequest(got, request) {
		t.Fatalf("VerifySession() = %#v, want %#v", got, request)
	}
	if _, err := keyring.VerifySession(context.Background(), signed.Context, signed.Signature, request,
		"POST", "/v1/sessions/session_01/operations", now, store); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySession(replay) error = %v, want ErrInvalidContext", err)
	}
}

func TestSessionContextV2RejectsV1TamperBindingAndTemporalFailures(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession() error = %v", err)
	}
	verify := func(signed SignedContext, expected SessionRequest, method, requestPath string, at time.Time) error {
		_, err := keyring.VerifySession(context.Background(), signed.Context, signed.Signature, expected,
			method, requestPath, at, newFakeSessionNonceStore())
		return err
	}

	v1Digest := sha256.Sum256([]byte("legacy request"))
	v1, err := keyring.Sign(Request{Scope: ScopeAgent, SpaceID: 42, UserID: 43, ExecutionID: "run_01", RequestDigest: v1Digest[:]})
	if err != nil {
		t.Fatalf("Sign(v1) error = %v", err)
	}
	if err := verify(v1, request, "POST", "/v1/sessions/session_01/operations", now); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySession(v1) error = %v, want ErrInvalidContext", err)
	}

	wrongProvider := request
	wrongProvider.ProviderID++
	wrongProfile := request
	wrongProfile.Profile = "interactive"
	wrongDigest := request
	otherDigest := sha256.Sum256([]byte("different body"))
	wrongDigest.RequestDigest = otherDigest[:]
	for name, expected := range map[string]struct {
		request SessionRequest
		method  string
		path    string
		at      time.Time
	}{
		"provider":  {wrongProvider, "POST", "/v1/sessions/session_01/operations", now},
		"profile":   {wrongProfile, "POST", "/v1/sessions/session_01/operations", now},
		"method":    {request, "PUT", "/v1/sessions/session_01/operations", now},
		"path":      {request, "POST", "/v1/sessions/session_02/operations", now},
		"digest":    {wrongDigest, "POST", "/v1/sessions/session_01/operations", now},
		"at expiry": {request, "POST", "/v1/sessions/session_01/operations", now.Add(time.Minute)},
		"expired":   {request, "POST", "/v1/sessions/session_01/operations", now.Add(2 * time.Minute)},
	} {
		if err := verify(signed, expected.request, expected.method, expected.path, expected.at); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySession(%s) error = %v, want ErrInvalidContext", name, err)
		}
	}

	futureKeyring := newTestKeyring(t, now.Add(time.Second))
	future, err := futureKeyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession(future) error = %v", err)
	}
	if err := verify(future, request, "POST", "/v1/sessions/session_01/operations", now); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySession(future) error = %v, want ErrInvalidContext", err)
	}

	unknownKeyring, err := NewKeyring("other", map[string][]byte{"other": []byte("other-test-signing-key")}, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring(other) error = %v", err)
	}
	unknownKeyring.Now = func() time.Time { return now }
	unknownKeyring.Nonce = func() (string, error) { return "nonce_other", nil }
	unknown, err := unknownKeyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession(other) error = %v", err)
	}
	if err := verify(unknown, request, "POST", "/v1/sessions/session_01/operations", now); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySession(unknown key) error = %v, want ErrInvalidContext", err)
	}
}

func TestSessionContextV2RejectsUnknownAndDuplicateJSONFields(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession() error = %v", err)
	}
	body, err := base64.RawURLEncoding.DecodeString(signed.Context)
	if err != nil {
		t.Fatalf("decode context: %v", err)
	}
	unknown := append(append([]byte(nil), body[:len(body)-1]...), []byte(`,"unknown":"value"}`)...)
	duplicate := bytes.Replace(body, []byte(`"provider_id":41`), []byte(`"provider_id":41,"provider_id":41`), 1)
	for name, malformed := range map[string][]byte{"unknown": unknown, "duplicate": duplicate} {
		forged := signSessionBody(malformed, []byte("test-signing-key-material"))
		if _, err := keyring.VerifySession(context.Background(), forged.Context, forged.Signature, request,
			"POST", "/v1/sessions/session_01/operations", now, newFakeSessionNonceStore()); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySession(%s JSON) error = %v, want ErrInvalidContext", name, err)
		}
	}
}

func TestSessionContextV2RejectsUnknownAndDuplicateAudienceFields(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "GET", "/v1/sessions/session_01")
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.RawURLEncoding.DecodeString(signed.Context)
	if err != nil {
		t.Fatal(err)
	}
	malformed := map[string][]byte{
		"unknown":   bytes.Replace(body, []byte(`"deployment_id":"runner-a"`), []byte(`"deployment_id":"runner-a","unknown":"value"`), 1),
		"duplicate": bytes.Replace(body, []byte(`"deployment_id":"runner-a"`), []byte(`"deployment_id":"runner-a","deployment_id":"runner-a"`), 1),
	}
	target := SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider}
	for name, candidate := range malformed {
		forged := signSessionBody(candidate, []byte("test-signing-key-material"))
		if _, err := keyring.VerifySessionContext(context.Background(), forged.Context, forged.Signature,
			target, "GET", "/v1/sessions/session_01", request.RequestDigest, now, newFakeSessionNonceStore()); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySessionContext(%s audience field) error = %v, want ErrInvalidContext", name, err)
		}
	}
}

func TestSessionContextV2RejectsMalformedAudienceEvenWithValidSignature(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "GET", "/v1/sessions/session_01")
	if err != nil {
		t.Fatal(err)
	}
	body, err := base64.RawURLEncoding.DecodeString(signed.Context)
	if err != nil {
		t.Fatal(err)
	}
	malformed := map[string][]byte{
		"unknown kind":       bytes.Replace(body, []byte(`"kind":"provider"`), []byte(`"kind":"unknown"`), 1),
		"missing provider":   bytes.Replace(body, []byte(`,"provider_id":41`), nil, 1),
		"invalid deployment": bytes.Replace(body, []byte(`"deployment_id":"runner-a"`), []byte(`"deployment_id":"../runner-a"`), 1),
	}
	target := SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider}
	for name, candidate := range malformed {
		forged := signSessionBody(candidate, []byte("test-signing-key-material"))
		if _, err := keyring.VerifySessionContext(context.Background(), forged.Context, forged.Signature,
			target, "GET", "/v1/sessions/session_01", request.RequestDigest, now, newFakeSessionNonceStore()); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySessionContext(%s) error = %v, want ErrInvalidContext", name, err)
		}
	}
}

func TestSessionContextV2RejectsNonCanonicalMethodPathAndInvalidNonceStore(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	request := newValidSessionRequest()
	for _, input := range []struct{ method, path string }{
		{"post", "/v1/sessions/session_01/operations"},
		{"POST", "v1/sessions/session_01/operations"},
		{"POST", "/v1/sessions/../operations"},
		{"POST", "/v1/sessions//operations"},
		{"POST", "/v1/sessions/session_01/operations?debug=1"},
		{"POST", "/v1/sessions/%2e%2e/operations"},
	} {
		if _, err := keyring.SignSession(request, input.method, input.path); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("SignSession(%q, %q) error = %v, want ErrInvalidContext", input.method, input.path, err)
		}
	}

	signed, err := keyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatalf("SignSession() error = %v", err)
	}
	if _, err := keyring.VerifySession(context.Background(), signed.Context, signed.Signature, request,
		"POST", "/v1/sessions/session_01/operations", now, nil); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySession(nil store) error = %v, want ErrInvalidContext", err)
	}
	failedStore := newFakeSessionNonceStore()
	failedStore.err = errors.New("redis unavailable")
	if _, err := keyring.VerifySession(context.Background(), signed.Context, signed.Signature, request,
		"POST", "/v1/sessions/session_01/operations", now, failedStore); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySession(failed store) error = %v, want ErrInvalidContext", err)
	}
	zeroDigest := request
	zeroDigest.RequestDigest = make([]byte, sha256.Size)
	if _, err := keyring.SignSession(zeroDigest, "POST", "/v1/sessions/session_01/operations"); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("SignSession(zero digest) error = %v, want ErrInvalidContext", err)
	}
}

func TestSessionContextV2AuthenticatedDecodeReturnsSignedClaimsAndConsumesNonce(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	keyring.Nonce = func() (string, error) { return "nonce_authenticated_decode", nil }
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "GET", "/v1/sessions/session_01")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeSessionNonceStore()
	verified, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
		SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider},
		"GET", "/v1/sessions/session_01", request.RequestDigest, now, store)
	if err != nil || !sameSessionRequest(verified, request) {
		t.Fatalf("VerifySessionContext() = %#v, %v", verified, err)
	}
	if _, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
		SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider},
		"GET", "/v1/sessions/session_01", request.RequestDigest, now, store); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("VerifySessionContext(replay) error = %v", err)
	}
}

func TestSessionContextV2AuthenticatedDecodeRejectsBindingBeforeNonceConsumption(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	keyring.Nonce = func() (string, error) { return "nonce_binding_reject", nil }
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "POST", "/v1/sessions/session_01/operations")
	if err != nil {
		t.Fatal(err)
	}
	store := newFakeSessionNonceStore()
	otherDigest := sha256.Sum256([]byte("different body"))
	for _, test := range []struct {
		method string
		path   string
		digest []byte
	}{
		{method: "GET", path: "/v1/sessions/session_01/operations", digest: request.RequestDigest},
		{method: "POST", path: "/v1/sessions/other/operations", digest: request.RequestDigest},
		{method: "POST", path: "/v1/sessions/session_01/operations", digest: otherDigest[:]},
	} {
		if _, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
			SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider},
			test.method, test.path, test.digest, now, store); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySessionContext(%s %s) error = %v", test.method, test.path, err)
		}
	}
	verified, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
		SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider},
		"POST", "/v1/sessions/session_01/operations", request.RequestDigest, now, store)
	if err != nil || !sameSessionRequest(verified, request) {
		t.Fatalf("valid request after rejected bindings = %#v, %v", verified, err)
	}
}

func TestSessionContextV2AuthenticatedDecodeRejectsWrongExpectedAudienceBeforeNonceConsumption(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	keyring.Nonce = func() (string, error) { return "nonce_audience_reject", nil }
	request := newValidSessionRequest()
	signed, err := keyring.SignSession(request, "GET", "/v1/sessions/session_01")
	if err != nil {
		t.Fatal(err)
	}
	runnerBStore := newFakeSessionNonceStore()
	for _, target := range []SessionContextTarget{
		{DeploymentID: "runner-b", Audience: SessionContextAudienceProvider},
		{DeploymentID: "runner-a", Audience: SessionContextAudienceRunner},
	} {
		if _, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
			target, "GET", "/v1/sessions/session_01", request.RequestDigest, now, runnerBStore); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySessionContext(%#v) error = %v, want ErrInvalidContext", target, err)
		}
	}
	runnerAStore := newFakeSessionNonceStore()
	verified, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
		SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider},
		"GET", "/v1/sessions/session_01", request.RequestDigest, now, runnerAStore)
	if err != nil || !sameSessionRequest(verified, request) {
		t.Fatalf("valid audience after rejected targets = %#v, %v", verified, err)
	}
}

func TestSessionContextV2RunnerAudienceIsLimitedToDeploymentControlRoutes(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	digest := sha256.Sum256([]byte("runner configuration"))
	request := SessionRequest{DeploymentID: "runner-a", RequestDigest: digest[:]}
	for _, requestPath := range []string{"/v1/session-configuration", "/v1/session-runtime-status"} {
		signed, err := keyring.SignSession(request, "GET", requestPath)
		if err != nil {
			t.Fatalf("SignSession(%s) error = %v", requestPath, err)
		}
		if _, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
			SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceProvider},
			"GET", requestPath, request.RequestDigest, now, newFakeSessionNonceStore()); !errors.Is(err, ErrInvalidContext) {
			t.Fatalf("VerifySessionContext(%s provider target) error = %v, want ErrInvalidContext", requestPath, err)
		}
		if _, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
			SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceRunner},
			"GET", requestPath, request.RequestDigest, now, newFakeSessionNonceStore()); err != nil {
			t.Fatalf("VerifySessionContext(%s runner target) error = %v", requestPath, err)
		}
	}
}

func TestSessionContextV2RunnerConfigurationUsesDeploymentOnlyClaims(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	digest := sha256.Sum256([]byte(`{"schema":"coze.sandbox.session_configuration.v1","version":2}`))
	request := SessionRequest{DeploymentID: "runner-a", RequestDigest: digest[:]}
	signed, err := keyring.SignSession(request, "PUT", "/v1/session-configuration")
	if err != nil {
		t.Fatalf("SignSession(deployment configuration) error = %v", err)
	}
	verified, err := keyring.VerifySessionContext(context.Background(), signed.Context, signed.Signature,
		SessionContextTarget{DeploymentID: "runner-a", Audience: SessionContextAudienceRunner},
		"PUT", "/v1/session-configuration", digest[:], now, newFakeSessionNonceStore())
	if err != nil || !sameSessionRequest(verified, request) {
		t.Fatalf("VerifySessionContext(deployment configuration) = %#v, %v", verified, err)
	}
	if _, err := keyring.SignSession(request, "GET", "/v1/sessions/session_01"); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("deployment-only claims signed a business route: %v", err)
	}
}

func TestSessionRequestPublicFormattingRedactsIdentityAndDigest(t *testing.T) {
	request := newValidSessionRequest()
	for _, rendered := range []string{fmt.Sprint(request), fmt.Sprintf("%#v", request), fmt.Sprintf("%+v", request)} {
		for _, secret := range []string{"thread_01", "run_01", "operation_01", base64.RawURLEncoding.EncodeToString(request.RequestDigest)} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("SessionRequest formatting leaked %q in %q", secret, rendered)
			}
		}
	}
}

func newValidSessionRequest() SessionRequest {
	digest := sha256.Sum256([]byte("canonical session operation body"))
	return SessionRequest{
		DeploymentID: "runner-a", ProviderID: 41, Scope: ScopeAgent, SpaceID: 42, UserID: 43, ThreadID: "thread_01",
		RunID: "run_01", OperationID: "operation_01", Profile: "core", RequestDigest: digest[:],
	}
}

func sameSessionRequest(left, right SessionRequest) bool {
	return left.DeploymentID == right.DeploymentID && left.ProviderID == right.ProviderID && left.Scope == right.Scope && left.SpaceID == right.SpaceID &&
		left.UserID == right.UserID && left.ThreadID == right.ThreadID && left.RunID == right.RunID &&
		left.OperationID == right.OperationID && left.Profile == right.Profile &&
		hmac.Equal(left.RequestDigest, right.RequestDigest)
}

type fakeSessionNonceStore struct {
	seen map[string]struct{}
	err  error
}

func newFakeSessionNonceStore() *fakeSessionNonceStore {
	return &fakeSessionNonceStore{seen: make(map[string]struct{})}
}

func (store *fakeSessionNonceStore) Consume(_ context.Context, keyID, nonce string, _ time.Time) (bool, error) {
	if store.err != nil {
		return false, store.err
	}
	key := keyID + ":" + nonce
	if _, exists := store.seen[key]; exists {
		return false, nil
	}
	store.seen[key] = struct{}{}
	return true, nil
}

func signSessionBody(body, key []byte) SignedContext {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	return SignedContext{
		Context:   base64.RawURLEncoding.EncodeToString(body),
		Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
	}
}
