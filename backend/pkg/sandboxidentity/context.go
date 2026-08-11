// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// Package sandboxidentity signs server-owned execution tenant context without
// depending on a remote provider's credentials or transport.
package sandboxidentity

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidContext = errors.New("sandbox execution identity is invalid")

type Scope string

const (
	ScopeAgent    Scope = "agent"
	ScopeMCPStdio Scope = "mcp_stdio"
	ScopeAppDev   Scope = "appdev"
	ScopePlugin   Scope = "plugin"
)

type Request struct {
	Scope         Scope
	SpaceID       int64
	UserID        int64
	ProjectID     string
	SessionID     string
	ExecutionID   string
	RequestDigest []byte
}

func (Request) String() string   { return "sandboxidentity.Request{identity:<redacted>}" }
func (Request) GoString() string { return "sandboxidentity.Request{identity:<redacted>}" }

type SignedContext struct {
	Context   string
	Signature string
}

func (SignedContext) String() string   { return "sandboxidentity.SignedContext{value:<redacted>}" }
func (SignedContext) GoString() string { return "sandboxidentity.SignedContext{value:<redacted>}" }

// Signer keeps RemoteProviderConfig independent from key-management details.
type Signer interface {
	Sign(Request) (SignedContext, error)
}

type Keyring struct {
	ActiveKeyID string
	Keys        map[string][]byte
	TTL         time.Duration
	Now         func() time.Time
	Nonce       func() (string, error)
}

func (Keyring) String() string   { return "sandboxidentity.Keyring{keys:<redacted>}" }
func (Keyring) GoString() string { return "sandboxidentity.Keyring{keys:<redacted>}" }

func NewKeyring(activeKeyID string, keys map[string][]byte, ttl time.Duration) (Keyring, error) {
	if !validIdentifier(activeKeyID) || ttl <= 0 || len(keys) == 0 {
		return Keyring{}, ErrInvalidContext
	}
	cloned := make(map[string][]byte, len(keys))
	for keyID, key := range keys {
		if !validIdentifier(keyID) || len(key) < 16 {
			return Keyring{}, ErrInvalidContext
		}
		cloned[keyID] = append([]byte(nil), key...)
	}
	if _, ok := cloned[activeKeyID]; !ok {
		return Keyring{}, ErrInvalidContext
	}
	return Keyring{ActiveKeyID: activeKeyID, Keys: cloned, TTL: ttl,
		Now: func() time.Time { return time.Now().UTC() }, Nonce: randomNonce}, nil
}

func (keyring Keyring) Sign(request Request) (SignedContext, error) {
	if keyring.valid() != nil {
		return SignedContext{}, ErrInvalidContext
	}
	nonce, err := keyring.Nonce()
	if err != nil || !validIdentifier(nonce) {
		return SignedContext{}, ErrInvalidContext
	}
	now := keyring.Now().UTC()
	envelope, err := newEnvelope(request, keyring.ActiveKeyID, now.Unix(), now.Add(keyring.TTL).Unix(), nonce)
	if err != nil {
		return SignedContext{}, ErrInvalidContext
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return SignedContext{}, ErrInvalidContext
	}
	mac := hmac.New(sha256.New, keyring.Keys[keyring.ActiveKeyID])
	_, _ = mac.Write(body)
	return SignedContext{Context: base64.RawURLEncoding.EncodeToString(body), Signature: base64.RawURLEncoding.EncodeToString(mac.Sum(nil))}, nil
}

func (keyring Keyring) Verify(encodedContext, encodedSignature string, scope Scope, expectedDigest []byte, now time.Time) (Request, error) {
	if keyring.valid() != nil || len(expectedDigest) != sha256.Size || now.IsZero() {
		return Request{}, ErrInvalidContext
	}
	body, err := base64.RawURLEncoding.DecodeString(encodedContext)
	if err != nil || len(body) == 0 {
		return Request{}, ErrInvalidContext
	}
	envelope, err := decodeCanonicalEnvelope(body)
	if err != nil || envelope.validate(scope, expectedDigest) != nil {
		return Request{}, ErrInvalidContext
	}
	key, ok := keyring.Keys[envelope.KeyID]
	if !ok {
		return Request{}, ErrInvalidContext
	}
	signature, err := base64.RawURLEncoding.DecodeString(encodedSignature)
	if err != nil || len(signature) != sha256.Size {
		return Request{}, ErrInvalidContext
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) || envelope.IssuedAtUnix > now.UTC().Unix() || envelope.ExpiresAtUnix < now.UTC().Unix() {
		return Request{}, ErrInvalidContext
	}
	return Request{Scope: scope, SpaceID: envelope.SpaceID, UserID: envelope.UserID, ProjectID: envelope.ProjectID,
		SessionID: envelope.SessionID, ExecutionID: envelope.ExecutionID, RequestDigest: append([]byte(nil), expectedDigest...)}, nil
}

func (keyring Keyring) valid() error {
	if !validIdentifier(keyring.ActiveKeyID) || keyring.TTL <= 0 || keyring.Now == nil || keyring.Nonce == nil || len(keyring.Keys[keyring.ActiveKeyID]) < 16 {
		return ErrInvalidContext
	}
	return nil
}

type envelope struct {
	KeyID         string `json:"key_id"`
	IssuedAtUnix  int64  `json:"issued_at_unix"`
	ExpiresAtUnix int64  `json:"expires_at_unix"`
	Nonce         string `json:"nonce"`
	SpaceID       int64  `json:"space_id"`
	UserID        int64  `json:"user_id"`
	ProjectID     string `json:"project_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	ExecutionID   string `json:"execution_id"`
	RequestDigest string `json:"request_digest"`
}

func newEnvelope(request Request, keyID string, issuedAtUnix, expiresAtUnix int64, nonce string) (envelope, error) {
	if len(request.RequestDigest) != sha256.Size || !validIdentifier(keyID) || !validIdentifier(nonce) || issuedAtUnix <= 0 || expiresAtUnix <= issuedAtUnix || request.SpaceID <= 0 || request.UserID <= 0 || !validIdentifier(request.ExecutionID) {
		return envelope{}, ErrInvalidContext
	}
	switch request.Scope {
	case ScopeAgent, ScopePlugin:
		if request.ProjectID != "" || request.SessionID != "" {
			return envelope{}, ErrInvalidContext
		}
	case ScopeAppDev:
		if !validIdentifier(request.ProjectID) || request.SessionID != "" {
			return envelope{}, ErrInvalidContext
		}
	case ScopeMCPStdio:
		if request.ProjectID != "" || !validIdentifier(request.SessionID) {
			return envelope{}, ErrInvalidContext
		}
	default:
		return envelope{}, ErrInvalidContext
	}
	return envelope{KeyID: keyID, IssuedAtUnix: issuedAtUnix, ExpiresAtUnix: expiresAtUnix, Nonce: nonce,
		SpaceID: request.SpaceID, UserID: request.UserID, ProjectID: request.ProjectID, SessionID: request.SessionID,
		ExecutionID: request.ExecutionID, RequestDigest: base64.RawURLEncoding.EncodeToString(request.RequestDigest)}, nil
}

func (value envelope) validate(scope Scope, expectedDigest []byte) error {
	digest, err := base64.RawURLEncoding.DecodeString(value.RequestDigest)
	if err != nil || !hmac.Equal(digest, expectedDigest) {
		return ErrInvalidContext
	}
	_, err = newEnvelope(Request{Scope: scope, SpaceID: value.SpaceID, UserID: value.UserID, ProjectID: value.ProjectID,
		SessionID: value.SessionID, ExecutionID: value.ExecutionID, RequestDigest: digest}, value.KeyID, value.IssuedAtUnix, value.ExpiresAtUnix, value.Nonce)
	return err
}

func decodeCanonicalEnvelope(body []byte) (envelope, error) {
	if !utf8.Valid(body) {
		return envelope{}, ErrInvalidContext
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	opening, err := decoder.Token()
	if err != nil {
		return envelope{}, ErrInvalidContext
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return envelope{}, ErrInvalidContext
	}
	allowed := map[string]struct{}{"key_id": {}, "issued_at_unix": {}, "expires_at_unix": {}, "nonce": {}, "space_id": {}, "user_id": {}, "project_id": {}, "session_id": {}, "execution_id": {}, "request_digest": {}}
	seen := make(map[string]struct{}, len(allowed))
	for decoder.More() {
		token, tokenErr := decoder.Token()
		key, ok := token.(string)
		if tokenErr != nil || !ok {
			return envelope{}, ErrInvalidContext
		}
		if _, allowedKey := allowed[key]; !allowedKey {
			return envelope{}, ErrInvalidContext
		}
		if _, duplicate := seen[key]; duplicate {
			return envelope{}, ErrInvalidContext
		}
		seen[key] = struct{}{}
		var discard json.RawMessage
		if decoder.Decode(&discard) != nil {
			return envelope{}, ErrInvalidContext
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return envelope{}, ErrInvalidContext
	}
	if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return envelope{}, ErrInvalidContext
	}
	if _, err := decoder.Token(); err != io.EOF {
		return envelope{}, ErrInvalidContext
	}
	var value envelope
	if json.Unmarshal(body, &value) != nil {
		return envelope{}, ErrInvalidContext
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, body) {
		return envelope{}, ErrInvalidContext
	}
	return value, nil
}

func randomNonce() (string, error) {
	value := make([]byte, 24)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func validIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') && !(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' && character != '+' {
			return false
		}
	}
	return true
}
