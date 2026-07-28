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
	"errors"
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
	codec := testCredentialCodec(t, bytes.NewReader(bytes.Repeat([]byte{1}, 24)))
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

func TestParseCredentialKeyRequiresBase64ThirtyTwoBytes(t *testing.T) {
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
}

func testCredentialCodec(t *testing.T, nonceSource *bytes.Reader) *CredentialCodec {
	t.Helper()
	key := bytes.Repeat([]byte{8}, 32)
	codec, err := NewCredentialCodec(key, nonceSource)
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}
	return codec
}
