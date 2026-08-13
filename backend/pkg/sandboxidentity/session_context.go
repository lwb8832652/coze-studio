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
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SessionContextHeader          = "X-Coze-Sandbox-Session-Context"
	SessionContextSignatureHeader = "X-Coze-Sandbox-Session-Context-Signature"
	SessionContextSchemaV2        = "coze.sandbox.session_context.v2"

	maxSessionContextBytes = 16 * 1024
)

type SessionRequest struct {
	ProviderID    int64
	Scope         Scope
	SpaceID       int64
	UserID        int64
	ThreadID      string
	RunID         string
	OperationID   string
	Profile       string
	RequestDigest []byte
}

func (SessionRequest) String() string {
	return "sandboxidentity.SessionRequest{identity:<redacted>}"
}
func (SessionRequest) GoString() string {
	return "sandboxidentity.SessionRequest{identity:<redacted>}"
}
func (SessionRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandboxidentity.SessionRequest{identity:<redacted>}")
}

type SessionNonceStore interface {
	Consume(context.Context, string, string, time.Time) (bool, error)
}

type SessionSigner interface {
	SignSession(SessionRequest, string, string) (SignedContext, error)
}

type SessionVerifier interface {
	VerifySession(context.Context, string, string, SessionRequest, string, string, time.Time, SessionNonceStore) (SessionRequest, error)
}

func (keyring Keyring) SignSession(request SessionRequest, method, requestPath string) (SignedContext, error) {
	if keyring.valid() != nil || !validSessionRequest(request) || !validSessionBinding(method, requestPath) {
		return SignedContext{}, ErrInvalidContext
	}
	nonce, err := keyring.Nonce()
	if err != nil || !validIdentifier(nonce) {
		return SignedContext{}, ErrInvalidContext
	}
	now := keyring.Now().UTC()
	envelope, err := newSessionEnvelope(request, keyring.ActiveKeyID, now.Unix(), now.Add(keyring.TTL).Unix(), nonce, method, requestPath)
	if err != nil {
		return SignedContext{}, ErrInvalidContext
	}
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maxSessionContextBytes {
		return SignedContext{}, ErrInvalidContext
	}
	mac := hmac.New(sha256.New, keyring.Keys[keyring.ActiveKeyID])
	_, _ = mac.Write(body)
	return SignedContext{
		Context:   base64.RawURLEncoding.EncodeToString(body),
		Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil)),
	}, nil
}

func (keyring Keyring) VerifySession(
	ctx context.Context,
	encodedContext string,
	encodedSignature string,
	expected SessionRequest,
	method string,
	requestPath string,
	now time.Time,
	nonceStore SessionNonceStore,
) (SessionRequest, error) {
	if ctx == nil || keyring.valid() != nil || !validSessionRequest(expected) ||
		!validSessionBinding(method, requestPath) || now.IsZero() || nonceStore == nil {
		return SessionRequest{}, ErrInvalidContext
	}
	body, err := base64.RawURLEncoding.DecodeString(encodedContext)
	if err != nil || len(body) == 0 || len(body) > maxSessionContextBytes {
		return SessionRequest{}, ErrInvalidContext
	}
	envelope, err := decodeCanonicalSessionEnvelope(body)
	if err != nil || envelope.validate(expected, method, requestPath) != nil {
		return SessionRequest{}, ErrInvalidContext
	}
	key, ok := keyring.Keys[envelope.KeyID]
	if !ok || len(key) < 16 {
		return SessionRequest{}, ErrInvalidContext
	}
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || len(signature) != sha256.Size {
		return SessionRequest{}, ErrInvalidContext
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	now = now.UTC()
	if !hmac.Equal(signature, mac.Sum(nil)) || envelope.IssuedAtUnix > now.Unix() ||
		envelope.ExpiresAtUnix <= now.Unix() {
		return SessionRequest{}, ErrInvalidContext
	}
	consumed, err := nonceStore.Consume(ctx, envelope.KeyID, envelope.Nonce, time.Unix(envelope.ExpiresAtUnix, 0).UTC())
	if err != nil || !consumed {
		return SessionRequest{}, ErrInvalidContext
	}
	return cloneSessionRequest(expected), nil
}

type sessionEnvelope struct {
	Schema        string `json:"schema"`
	KeyID         string `json:"key_id"`
	IssuedAtUnix  int64  `json:"issued_at_unix"`
	ExpiresAtUnix int64  `json:"expires_at_unix"`
	Nonce         string `json:"nonce"`
	ProviderID    int64  `json:"provider_id"`
	Scope         Scope  `json:"scope"`
	SpaceID       int64  `json:"space_id"`
	UserID        int64  `json:"user_id"`
	ThreadID      string `json:"thread_id"`
	RunID         string `json:"run_id"`
	OperationID   string `json:"operation_id"`
	Profile       string `json:"profile"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	RequestDigest string `json:"request_digest"`
}

func newSessionEnvelope(
	request SessionRequest,
	keyID string,
	issuedAtUnix int64,
	expiresAtUnix int64,
	nonce string,
	method string,
	requestPath string,
) (sessionEnvelope, error) {
	if !validSessionRequest(request) || !validIdentifier(keyID) || !validIdentifier(nonce) ||
		issuedAtUnix <= 0 || expiresAtUnix <= issuedAtUnix || !validSessionBinding(method, requestPath) {
		return sessionEnvelope{}, ErrInvalidContext
	}
	return sessionEnvelope{
		Schema: SessionContextSchemaV2, KeyID: keyID, IssuedAtUnix: issuedAtUnix,
		ExpiresAtUnix: expiresAtUnix, Nonce: nonce, ProviderID: request.ProviderID,
		Scope: request.Scope, SpaceID: request.SpaceID, UserID: request.UserID,
		ThreadID: request.ThreadID, RunID: request.RunID, OperationID: request.OperationID,
		Profile: request.Profile, Method: method, Path: requestPath,
		RequestDigest: base64.RawURLEncoding.EncodeToString(request.RequestDigest),
	}, nil
}

func (value sessionEnvelope) validate(expected SessionRequest, method, requestPath string) error {
	if value.Schema != SessionContextSchemaV2 || value.ProviderID != expected.ProviderID ||
		value.Scope != expected.Scope || value.SpaceID != expected.SpaceID || value.UserID != expected.UserID ||
		value.ThreadID != expected.ThreadID || value.RunID != expected.RunID ||
		value.OperationID != expected.OperationID || value.Profile != expected.Profile ||
		value.Method != method || value.Path != requestPath {
		return ErrInvalidContext
	}
	digest, err := base64.RawURLEncoding.DecodeString(value.RequestDigest)
	if err != nil || !hmac.Equal(digest, expected.RequestDigest) {
		return ErrInvalidContext
	}
	_, err = newSessionEnvelope(expected, value.KeyID, value.IssuedAtUnix, value.ExpiresAtUnix,
		value.Nonce, value.Method, value.Path)
	return err
}

func decodeCanonicalSessionEnvelope(body []byte) (sessionEnvelope, error) {
	if len(body) == 0 || len(body) > maxSessionContextBytes || !utf8.Valid(body) {
		return sessionEnvelope{}, ErrInvalidContext
	}
	allowed := map[string]struct{}{
		"schema": {}, "key_id": {}, "issued_at_unix": {}, "expires_at_unix": {}, "nonce": {},
		"provider_id": {}, "scope": {}, "space_id": {}, "user_id": {}, "thread_id": {},
		"run_id": {}, "operation_id": {}, "profile": {}, "method": {}, "path": {}, "request_digest": {},
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return sessionEnvelope{}, ErrInvalidContext
	}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok {
			return sessionEnvelope{}, ErrInvalidContext
		}
		if _, permitted := allowed[key]; !permitted {
			return sessionEnvelope{}, ErrInvalidContext
		}
		if _, duplicate := seen[key]; duplicate {
			return sessionEnvelope{}, ErrInvalidContext
		}
		seen[key] = struct{}{}
		var discard json.RawMessage
		if decoder.Decode(&discard) != nil {
			return sessionEnvelope{}, ErrInvalidContext
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != len(allowed) {
		return sessionEnvelope{}, ErrInvalidContext
	}
	if _, err := decoder.Token(); err != io.EOF {
		return sessionEnvelope{}, ErrInvalidContext
	}
	var value sessionEnvelope
	if json.Unmarshal(body, &value) != nil {
		return sessionEnvelope{}, ErrInvalidContext
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, body) {
		return sessionEnvelope{}, ErrInvalidContext
	}
	return value, nil
}

func validSessionRequest(request SessionRequest) bool {
	if request.ProviderID <= 0 || request.SpaceID <= 0 || request.UserID <= 0 ||
		!validIdentifier(request.ThreadID) || !validIdentifier(request.RunID) ||
		!validIdentifier(request.OperationID) || len(request.RequestDigest) != sha256.Size ||
		hmac.Equal(request.RequestDigest, make([]byte, sha256.Size)) {
		return false
	}
	switch request.Scope {
	case ScopeAgent, ScopeMCPStdio, ScopeAppDev, ScopePlugin:
	default:
		return false
	}
	return request.Profile == "core" || request.Profile == "interactive"
}

func validSessionBinding(method, requestPath string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return false
	}
	if requestPath == "" || len(requestPath) > 2048 || !utf8.ValidString(requestPath) ||
		!strings.HasPrefix(requestPath, "/") || path.Clean(requestPath) != requestPath ||
		strings.Contains(requestPath, "//") || strings.ContainsAny(requestPath, "\\?#\x00") ||
		strings.Contains(strings.ToLower(requestPath), "%2e") || strings.Contains(strings.ToLower(requestPath), "%2f") ||
		strings.Contains(strings.ToLower(requestPath), "%5c") {
		return false
	}
	for _, character := range requestPath {
		if character <= 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}

func cloneSessionRequest(request SessionRequest) SessionRequest {
	request.RequestDigest = append([]byte(nil), request.RequestDigest...)
	return request
}
