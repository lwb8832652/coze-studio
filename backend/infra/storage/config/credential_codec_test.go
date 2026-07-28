/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

func TestCredentialCodecRoundTripAndDoesNotStorePlaintext(t *testing.T) {
	codec := testCredentialCodec(t, bytes.NewReader(bytes.Repeat([]byte{9}, 24)))
	plain := domain.CredentialInput{AccessKeyID: "ak-live-marker", SecretAccessKey: "sk-live-marker"}
	envelope, err := codec.Encrypt(42, domain.ProviderMinIO, 1, plain)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if strings.Contains(envelope, "ak-live-marker") || strings.Contains(envelope, "sk-live-marker") {
		t.Fatal("envelope leaked plaintext credential")
	}
	decoded, err := codec.Decrypt(42, domain.ProviderMinIO, 1, envelope)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decoded != plain {
		t.Fatalf("decoded credential = %+v", decoded)
	}
}

func TestCredentialCodecUsesFreshNonce(t *testing.T) {
	codec := testCredentialCodec(t, fixedNonceReader([]byte{1}, []byte{2}))
	plain := domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}
	first, err := codec.Encrypt(7, domain.ProviderAWSS3, 1, plain)
	if err != nil {
		t.Fatalf("first Encrypt() error = %v", err)
	}
	second, err := codec.Encrypt(7, domain.ProviderAWSS3, 1, plain)
	if err != nil {
		t.Fatalf("second Encrypt() error = %v", err)
	}
	if first == second {
		t.Fatal("two envelopes used the same nonce")
	}
}

func TestLoadCredentialCodecRejectsNilGetenvAndLoadsEnvKey(t *testing.T) {
	if _, err := LoadCredentialCodec(nil); !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("LoadCredentialCodec(nil) error = %v", err)
	}

	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{4}, 32))
	codec, err := LoadCredentialCodec(func(name string) string {
		if name != ObjectStorageCredentialKeyEnv {
			t.Fatalf("getenv name = %q", name)
		}
		return key
	})
	if err != nil {
		t.Fatalf("LoadCredentialCodec(valid) error = %v", err)
	}
	if codec == nil {
		t.Fatal("LoadCredentialCodec(valid) returned nil codec")
	}
}

func TestNewCredentialCodecCopiesKey(t *testing.T) {
	key := bytes.Repeat([]byte{8}, 32)
	codec, err := NewCredentialCodec(key, fixedNonceReader([]byte{1}, []byte{2}))
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}
	for i := range key {
		key[i] = 0
	}

	plain := domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}
	envelope, err := codec.Encrypt(1, domain.ProviderMinIO, 1, plain)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	decoded, err := codec.Decrypt(1, domain.ProviderMinIO, 1, envelope)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decoded != plain {
		t.Fatalf("decoded credential = %+v", decoded)
	}
}

func TestNewCredentialCodecRejectsTypedNilNonceSource(t *testing.T) {
	var nonceSource *typedNilNonceSource
	if _, err := NewCredentialCodec(bytes.Repeat([]byte{8}, 32), nonceSource); !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("NewCredentialCodec(typed nil nonce) error = %v", err)
	}
}

func TestCredentialCodecBindsAAD(t *testing.T) {
	codec := testCredentialCodec(t, bytes.NewReader(bytes.Repeat([]byte{5}, 24)))
	envelope, err := codec.Encrypt(1, domain.ProviderQiniu, 1, domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"})
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	for _, test := range []struct {
		id       uint64
		provider domain.ProviderType
		version  uint64
	}{
		{id: 2, provider: domain.ProviderQiniu, version: 1},
		{id: 1, provider: domain.ProviderMinIO, version: 1},
		{id: 1, provider: domain.ProviderQiniu, version: 2},
	} {
		if _, err = codec.Decrypt(test.id, test.provider, test.version, envelope); !errors.Is(err, domain.ErrCredentialUnavailable) {
			t.Fatalf("Decrypt(%+v) error = %v", test, err)
		}
	}
}

func TestCredentialCodecParseCredentialKeyRequiresBase64ThirtyTwoBytes(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	decoded, err := ParseCredentialKey(key)
	if err != nil {
		t.Fatalf("ParseCredentialKey(valid) error = %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded length = %d", len(decoded))
	}
	if _, err = ParseCredentialKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31))); !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("short key error = %v", err)
	}
	if _, err = ParseCredentialKey(base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{255}, 32))); !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("url-safe key error = %v", err)
	}
	if _, err = ParseCredentialKey(key + "\n"); !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("key with newline error = %v", err)
	}
}

func TestCredentialCodecDecryptRejectsUnavailableInputsSafely(t *testing.T) {
	codec := testCredentialCodec(t, fixedNonceReader([]byte{1}, []byte{2}, []byte{3}))
	envelope, err := codec.Encrypt(1, domain.ProviderQiniu, 1, domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"})
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	var parsed credentialEnvelope
	if err = json.Unmarshal([]byte(envelope), &parsed); err != nil {
		t.Fatalf("Unmarshal(envelope) error = %v", err)
	}
	tampered := parsed
	ciphertext, err := base64.StdEncoding.DecodeString(tampered.Ciphertext)
	if err != nil {
		t.Fatalf("DecodeString(ciphertext) error = %v", err)
	}
	ciphertext[0] ^= 1
	tampered.Ciphertext = base64.StdEncoding.EncodeToString(ciphertext)
	tamperedEnvelope, err := json.Marshal(tampered)
	if err != nil {
		t.Fatalf("Marshal(tampered) error = %v", err)
	}

	wrongKeyCodec := testCredentialCodecWithKey(t, bytes.Repeat([]byte{7}, 32), fixedNonceReader([]byte{4}))
	for _, test := range []struct {
		name    string
		codec   *CredentialCodec
		payload string
	}{
		{name: "malformed json", codec: codec, payload: "{malicious-envelope-marker"},
		{name: "unknown version", codec: codec, payload: `{"version":"v2","nonce":"","ciphertext":""}`},
		{name: "invalid nonce base64", codec: codec, payload: `{"version":"v1","nonce":"!!!!","ciphertext":""}`},
		{name: "invalid ciphertext base64", codec: codec, payload: `{"version":"v1","nonce":"AQEBAQEBAQEBAQEB","ciphertext":"!!!!"}`},
		{name: "tampered ciphertext", codec: codec, payload: string(tamperedEnvelope)},
		{name: "wrong key", codec: wrongKeyCodec, payload: envelope},
	} {
		if _, err = test.codec.Decrypt(1, domain.ProviderQiniu, 1, test.payload); !errors.Is(err, domain.ErrCredentialUnavailable) {
			t.Fatalf("%s Decrypt() error = %v", test.name, err)
		}
	}
}

func TestCredentialCodecEncryptNormalizesCredentials(t *testing.T) {
	codec := testCredentialCodec(t, fixedNonceReader([]byte{1}))
	envelope, err := codec.Encrypt(1, domain.ProviderMinIO, 1, domain.CredentialInput{
		AccessKeyID:     " ak ",
		SecretAccessKey: " sk ",
	})
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	decoded, err := codec.Decrypt(1, domain.ProviderMinIO, 1, envelope)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decoded != (domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}) {
		t.Fatalf("decoded credential = %+v", decoded)
	}
}

func TestCredentialCodecErrorsDoNotLeakMarkers(t *testing.T) {
	codec := testCredentialCodec(t, fixedNonceReader([]byte{1}))
	if _, err := codec.Encrypt(1, domain.ProviderMinIO, 1, domain.CredentialInput{
		AccessKeyID:     "ak-error-marker",
		SecretAccessKey: "",
	}); err == nil || errorContainsAny(err, "ak-error-marker", "sk-error-marker") {
		t.Fatalf("Encrypt() leaked marker in error: %v", err)
	}
	if _, err := codec.Decrypt(1, domain.ProviderMinIO, 1, "{malicious-envelope-marker"); err == nil || errorContainsAny(err, "malicious-envelope-marker") {
		t.Fatalf("Decrypt() leaked marker in error: %v", err)
	}
}

func testCredentialCodec(t *testing.T, nonceSource *bytes.Reader) *CredentialCodec {
	t.Helper()
	return testCredentialCodecWithKey(t, bytes.Repeat([]byte{8}, 32), nonceSource)
}

func testCredentialCodecWithKey(t *testing.T, key []byte, nonceSource io.Reader) *CredentialCodec {
	t.Helper()
	codec, err := NewCredentialCodec(key, nonceSource)
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}
	return codec
}

func fixedNonceReader(nonces ...[]byte) *bytes.Reader {
	var stream []byte
	for _, nonce := range nonces {
		stream = append(stream, bytes.Repeat(nonce, 12)...)
	}
	return bytes.NewReader(stream)
}

func errorContainsAny(err error, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(err.Error(), marker) {
			return true
		}
	}
	return false
}

type typedNilNonceSource struct{}

func (*typedNilNonceSource) Read([]byte) (int, error) {
	return 0, io.EOF
}
