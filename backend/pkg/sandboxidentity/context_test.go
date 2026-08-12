// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxidentity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestRequestContextRetainsOnlyValidatedServerOwnedIdentity(t *testing.T) {
	request := Request{Scope: ScopeMCPStdio, SpaceID: 11, UserID: 22, SessionID: "session_33", ExecutionID: "exec_44", RequestDigest: []byte("must-not-be-propagated")}
	ctx := WithRequest(context.Background(), request)
	got, ok := RequestFromContext(ctx)
	if !ok || got.Scope != request.Scope || got.SpaceID != request.SpaceID || got.UserID != request.UserID || got.SessionID != request.SessionID || got.ExecutionID != request.ExecutionID || got.RequestDigest != nil {
		t.Fatalf("RequestFromContext() = %#v, %v", got, ok)
	}
	if _, ok := RequestFromContext(WithRequest(context.Background(), Request{Scope: ScopeMCPStdio, SpaceID: 11, UserID: 22, ExecutionID: "exec_44"})); ok {
		t.Fatal("RequestFromContext() accepted invalid MCP identity")
	}
}

func TestLoadKeyringFromEnvKeepsLegacyProvidersConfigurableAndRejectsPartialSecrets(t *testing.T) {
	getenv := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}
	if _, configured, err := LoadKeyringFromEnv(getenv(nil), time.Minute); err != nil || configured {
		t.Fatalf("LoadKeyringFromEnv() legacy result = configured:%t err:%v", configured, err)
	}
	if _, _, err := LoadKeyringFromEnv(getenv(map[string]string{ExecutionContextSigningKeysJSONEnv: `{"keys":{"runner.v1":"0123456789abcdef"}}`}), time.Minute); err == nil {
		t.Fatal("LoadKeyringFromEnv() accepted partial configuration")
	}
	keyring, configured, err := LoadKeyringFromEnv(getenv(map[string]string{
		ExecutionContextSigningKeysJSONEnv: `{"keys":{"runner.v1":"0123456789abcdef"}}`,
		ExecutionContextActiveKeyIDEnv:     "runner.v1",
	}), time.Minute)
	if err != nil || !configured || keyring.ActiveKeyID != "runner.v1" {
		t.Fatalf("LoadKeyringFromEnv() = %#v, %t, %v", keyring, configured, err)
	}
}

func TestExecutionIdentitySignsOnlyCanonicalSafeFields(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	keyring, err := NewKeyring("current", map[string][]byte{"current": []byte("test-signing-key-material")}, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	keyring.Now = func() time.Time { return now }
	keyring.Nonce = func() (string, error) { return "nonce_123", nil }
	digest := sha256.Sum256([]byte("canonical execute body"))
	signed, err := keyring.Sign(Request{Scope: ScopeAppDev, SpaceID: 11, UserID: 22, ProjectID: "project_33", ExecutionID: "exec_44", RequestDigest: digest[:]})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(signed.Context)
	if err != nil {
		t.Fatalf("decode context = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode canonical context = %v", err)
	}
	want := map[string]bool{"key_id": true, "issued_at_unix": true, "expires_at_unix": true, "nonce": true, "space_id": true, "user_id": true, "project_id": true, "execution_id": true, "request_digest": true}
	if len(fields) != len(want) {
		t.Fatalf("identity field count = %d, want %d", len(fields), len(want))
	}
	for field := range fields {
		if !want[field] {
			t.Fatalf("unexpected identity field %q", field)
		}
	}
	if _, err := keyring.Verify(signed.Context, signed.Signature, ScopeAppDev, digest[:], now); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestExecutionIdentityRejectsTamperExpiryWrongKeyDigestAndScope(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	keyring, err := NewKeyring("current", map[string][]byte{"current": []byte("test-signing-key-material")}, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	keyring.Now = func() time.Time { return now }
	keyring.Nonce = func() (string, error) { return "nonce_123", nil }
	digest := sha256.Sum256([]byte("canonical execute body"))
	signed, err := keyring.Sign(Request{Scope: ScopeAgent, SpaceID: 11, UserID: 22, ExecutionID: "exec_44", RequestDigest: digest[:]})
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if _, err := keyring.Verify(signed.Context+"x", signed.Signature, ScopeAgent, digest[:], now); err == nil {
		t.Fatal("Verify() accepted tampered context")
	}
	if _, err := keyring.Verify(signed.Context, signed.Signature, ScopeAgent, digest[:], now.Add(2*time.Minute)); err == nil {
		t.Fatal("Verify() accepted expired context")
	}
	wrongKey, err := NewKeyring("current", map[string][]byte{"current": []byte("different-key-material")}, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring(wrong key) error = %v", err)
	}
	if _, err := wrongKey.Verify(signed.Context, signed.Signature, ScopeAgent, digest[:], now); err == nil {
		t.Fatal("Verify() accepted wrong key")
	}
	wrongDigest := sha256.Sum256([]byte("different canonical body"))
	if _, err := keyring.Verify(signed.Context, signed.Signature, ScopeAgent, wrongDigest[:], now); err == nil {
		t.Fatal("Verify() accepted wrong digest")
	}
	if _, err := keyring.Sign(Request{Scope: ScopeMCPStdio, SpaceID: 11, UserID: 22, ExecutionID: "exec_44", RequestDigest: digest[:]}); err == nil {
		t.Fatal("Sign() accepted missing MCP session identity")
	}
}

func TestExecutionIdentityScopeMatrixAppliesToSignAndVerify(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	keyring := newTestKeyring(t, now)
	digest := sha256.Sum256([]byte("canonical execute body"))

	tests := []struct {
		name        string
		scope       Scope
		projectID   string
		sessionID   string
		wantAllowed bool
	}{
		{name: "agent none", scope: ScopeAgent, wantAllowed: true},
		{name: "agent project", scope: ScopeAgent, projectID: "project_33"},
		{name: "agent session", scope: ScopeAgent, sessionID: "session_33"},
		{name: "agent project session", scope: ScopeAgent, projectID: "project_33", sessionID: "session_33"},
		{name: "plugin none", scope: ScopePlugin, wantAllowed: true},
		{name: "plugin project", scope: ScopePlugin, projectID: "project_33"},
		{name: "plugin session", scope: ScopePlugin, sessionID: "session_33"},
		{name: "plugin project session", scope: ScopePlugin, projectID: "project_33", sessionID: "session_33"},
		{name: "appdev none", scope: ScopeAppDev},
		{name: "appdev project", scope: ScopeAppDev, projectID: "project_33", wantAllowed: true},
		{name: "appdev session", scope: ScopeAppDev, sessionID: "session_33"},
		{name: "appdev project session", scope: ScopeAppDev, projectID: "project_33", sessionID: "session_33"},
		{name: "mcp none", scope: ScopeMCPStdio},
		{name: "mcp project", scope: ScopeMCPStdio, projectID: "project_33"},
		{name: "mcp session", scope: ScopeMCPStdio, sessionID: "session_33", wantAllowed: true},
		{name: "mcp project session", scope: ScopeMCPStdio, projectID: "project_33", sessionID: "session_33"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := Request{Scope: test.scope, SpaceID: 11, UserID: 22, ProjectID: test.projectID, SessionID: test.sessionID, ExecutionID: "exec_44", RequestDigest: digest[:]}
			signed, signErr := keyring.Sign(request)
			if test.wantAllowed {
				if signErr != nil {
					t.Fatalf("Sign() error = %v", signErr)
				}
				if _, verifyErr := keyring.Verify(signed.Context, signed.Signature, test.scope, digest[:], now); verifyErr != nil {
					t.Fatalf("Verify() error = %v", verifyErr)
				}
				return
			}
			if signErr == nil {
				t.Fatal("Sign() accepted invalid scope identity")
			}
			forged := signedEnvelopeForTest(t, test.scope, test.projectID, test.sessionID, digest[:], now)
			if _, verifyErr := keyring.Verify(forged.Context, forged.Signature, test.scope, digest[:], now); verifyErr == nil {
				t.Fatal("Verify() accepted invalid scope identity")
			}
		})
	}
}

func TestExecutionIdentityKeyringVerifiesRotatedNonActiveKey(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	keys := map[string][]byte{
		"old": []byte("old-test-signing-key-material"),
		"new": []byte("new-test-signing-key-material"),
	}
	oldKeyring, err := NewKeyring("old", keys, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring(old) error = %v", err)
	}
	oldKeyring.Now = func() time.Time { return now }
	oldKeyring.Nonce = func() (string, error) { return "nonce_123", nil }
	digest := sha256.Sum256([]byte("canonical execute body"))
	signed, err := oldKeyring.Sign(Request{Scope: ScopeAgent, SpaceID: 11, UserID: 22, ExecutionID: "exec_44", RequestDigest: digest[:]})
	if err != nil {
		t.Fatalf("Sign(old) error = %v", err)
	}
	rotated, err := NewKeyring("new", keys, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring(new) error = %v", err)
	}
	if _, err := rotated.Verify(signed.Context, signed.Signature, ScopeAgent, digest[:], now); err != nil {
		t.Fatalf("Verify(rotated old key) error = %v", err)
	}
}

func newTestKeyring(t *testing.T, now time.Time) Keyring {
	t.Helper()
	keyring, err := NewKeyring("current", map[string][]byte{"current": []byte("test-signing-key-material")}, time.Minute)
	if err != nil {
		t.Fatalf("NewKeyring() error = %v", err)
	}
	keyring.Now = func() time.Time { return now }
	keyring.Nonce = func() (string, error) { return "nonce_123", nil }
	return keyring
}

func signedEnvelopeForTest(t *testing.T, scope Scope, projectID, sessionID string, digest []byte, now time.Time) SignedContext {
	t.Helper()
	body, err := json.Marshal(envelope{
		KeyID: "current", IssuedAtUnix: now.Unix(), ExpiresAtUnix: now.Add(time.Minute).Unix(), Nonce: "nonce_123",
		SpaceID: 11, UserID: 22, ProjectID: projectID, SessionID: sessionID, ExecutionID: "exec_44",
		RequestDigest: base64.RawURLEncoding.EncodeToString(digest),
	})
	if err != nil {
		t.Fatalf("marshal forged envelope: %v", err)
	}
	mac := hmac.New(sha256.New, []byte("test-signing-key-material"))
	_, _ = mac.Write(body)
	return SignedContext{Context: base64.RawURLEncoding.EncodeToString(body), Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}
}
