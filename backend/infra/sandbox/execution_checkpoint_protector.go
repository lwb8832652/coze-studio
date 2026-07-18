// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	sandboxcontract "github.com/coze-dev/coze-studio/backend/pkg/sandboxcontract"
	"github.com/coze-dev/coze-studio/backend/pkg/secureaead"
)

const (
	executionCheckpointEnvelopeVersion       = "ecp1"
	MaxExecutionCheckpointEnvelopeBytes      = sandboxcontract.MaxExecutionCheckpointEnvelopeBytes
	maxExecutionCheckpointProtectorPlaintext = 4096
	maxExecutionCheckpointAADBytes           = 1024
)

var ErrExecutionCheckpointProtection = errors.New("execution checkpoint protection failed")

type ExecutionCheckpointProtector struct {
	keyRing     *SandboxKeyRing
	nonceSource io.Reader
	nonceMu     sync.Mutex
}

func NewExecutionCheckpointProtector(keyRing *SandboxKeyRing) (*ExecutionCheckpointProtector, error) {
	return newExecutionCheckpointProtectorWithNonceSource(keyRing, rand.Reader)
}

func newExecutionCheckpointProtectorWithNonceSource(keyRing *SandboxKeyRing, nonceSource io.Reader) (*ExecutionCheckpointProtector, error) {
	if keyRing == nil || nonceSource == nil || !validSandboxKeyID(keyRing.activeKeyID) || len(keyRing.keys[keyRing.activeKeyID]) != 32 {
		return nil, ErrExecutionCheckpointProtection
	}
	return &ExecutionCheckpointProtector{keyRing: keyRing, nonceSource: nonceSource}, nil
}

func (p *ExecutionCheckpointProtector) Seal(ctx context.Context, aad, plaintext []byte) (string, error) {
	if !p.valid() || ctx == nil || ctx.Err() != nil || len(aad) == 0 || len(aad) > maxExecutionCheckpointAADBytes ||
		len(plaintext) == 0 || len(plaintext) > maxExecutionCheckpointProtectorPlaintext {
		return "", ErrExecutionCheckpointProtection
	}
	p.nonceMu.Lock()
	nonce, ciphertext, err := secureaead.Seal(p.keyRing.keys[p.keyRing.activeKeyID], aad, plaintext, p.nonceSource)
	p.nonceMu.Unlock()
	if err != nil {
		return "", ErrExecutionCheckpointProtection
	}
	encoding := base64.RawURLEncoding
	envelope := strings.Join([]string{
		executionCheckpointEnvelopeVersion,
		p.keyRing.activeKeyID,
		encoding.EncodeToString(nonce),
		encoding.EncodeToString(ciphertext),
	}, ":")
	if len(envelope) > MaxExecutionCheckpointEnvelopeBytes {
		return "", ErrExecutionCheckpointProtection
	}
	return envelope, nil
}

func (p *ExecutionCheckpointProtector) Open(ctx context.Context, aad []byte, envelope string) ([]byte, error) {
	if !p.valid() || ctx == nil || ctx.Err() != nil || len(aad) == 0 || len(aad) > maxExecutionCheckpointAADBytes ||
		len(envelope) == 0 || len(envelope) > MaxExecutionCheckpointEnvelopeBytes {
		return nil, ErrExecutionCheckpointProtection
	}
	segments := strings.Split(envelope, ":")
	if len(segments) != 4 || segments[0] != executionCheckpointEnvelopeVersion || !validSandboxKeyID(segments[1]) {
		return nil, ErrExecutionCheckpointProtection
	}
	key, exists := p.keyRing.keys[segments[1]]
	if !exists || len(key) != 32 {
		return nil, ErrExecutionCheckpointProtection
	}
	nonce, ok := decodeExecutionCheckpointBase64(segments[2])
	if !ok || len(nonce) != 12 {
		return nil, ErrExecutionCheckpointProtection
	}
	ciphertext, ok := decodeExecutionCheckpointBase64(segments[3])
	if !ok || len(ciphertext) <= 16 || len(ciphertext) > maxExecutionCheckpointProtectorPlaintext+16 {
		return nil, ErrExecutionCheckpointProtection
	}
	plaintext, err := secureaead.Open(key, aad, nonce, ciphertext)
	if err != nil || len(plaintext) == 0 || len(plaintext) > maxExecutionCheckpointProtectorPlaintext {
		wipeBytes(plaintext)
		return nil, ErrExecutionCheckpointProtection
	}
	return plaintext, nil
}

func (p *ExecutionCheckpointProtector) valid() bool {
	return p != nil && p.keyRing != nil && p.nonceSource != nil && validSandboxKeyID(p.keyRing.activeKeyID) && len(p.keyRing.keys[p.keyRing.activeKeyID]) == 32
}

func decodeExecutionCheckpointBase64(value string) ([]byte, bool) {
	if value == "" {
		return nil, false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != value {
		wipeBytes(decoded)
		return nil, false
	}
	return decoded, true
}

func (*ExecutionCheckpointProtector) String() string {
	return "[REDACTED execution checkpoint protector]"
}
func (*ExecutionCheckpointProtector) GoString() string {
	return "[REDACTED execution checkpoint protector]"
}
func (*ExecutionCheckpointProtector) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "[REDACTED execution checkpoint protector]")
}
