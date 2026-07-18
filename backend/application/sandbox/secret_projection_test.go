// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestEndpointHintUsesCanonicalFixedMasks(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		fullHost  string
		forbidden []string
		want      string
	}{
		{
			name:      "private DNS with root path and port",
			raw:       "https://runner.private.example.com:8443/",
			fullHost:  "runner.private.example.com",
			forbidden: []string{"runner", "private", "example"},
			want:      "https://***.com:8443",
		},
		{
			name:      "two-label DNS",
			raw:       "https://x.io",
			fullHost:  "x.io",
			forbidden: []string{"x.io"},
			want:      "https://***.io",
		},
		{
			name:      "punycode suffix",
			raw:       "https://runner.xn--p1ai/",
			fullHost:  "runner.xn--p1ai",
			forbidden: []string{"runner"},
			want:      "https://***.xn--p1ai",
		},
		{
			name:      "private single label",
			raw:       "https://localhost/",
			fullHost:  "localhost",
			forbidden: []string{"localhost"},
			want:      "https://***",
		},
		{
			name:     "IPv4",
			raw:      "https://10.23.45.67:443/",
			fullHost: "10.23.45.67",
			want:     "https://***.***.***.***:443",
		},
		{
			name:      "IPv6",
			raw:       "https://[2001:db8:abcd:12::42]:9443",
			fullHost:  "2001:db8:abcd:12::42",
			forbidden: []string{"2001", "db8", "abcd", "42"},
			want:      "https://[****]:9443",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DeriveEndpointHint(test.raw)
			if err != nil {
				t.Fatalf("DeriveEndpointHint() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("DeriveEndpointHint() = %q, want %q", got, test.want)
			}
			if strings.Contains(got, test.fullHost) {
				t.Fatal("endpoint hint exposed the full host")
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(got, forbidden) {
					t.Fatal("endpoint hint exposed a private host component")
				}
			}
		})
	}
}

func TestEndpointHintRejectsNonCanonicalHTTPSURLsSafely(t *testing.T) {
	marker := "endpoint-marker-must-not-leak"
	tests := []string{
		"",
		" http://example.com/" + marker,
		"http://example.com/" + marker,
		"https:///missing-host/" + marker,
		"https://user:" + marker + "@example.com/",
		"https://example.com/private/" + marker,
		"https://example.com/?token=" + marker,
		"https://example.com?",
		"https://example.com/#" + marker,
		"https://example.com:",
		"https://example.com:0/",
		"https://example.com:0443/",
		"https://example.com:65536/",
		"https://example.com:not-a-port/",
		"https://bad host.example/",
		"https://exa_mple.com/",
		"https://private.example.*/",
		"https://010.023.045.067/",
		"https://999.1.1.1/",
		"https://1.2.3/",
		"https://1.2.3.4.5/",
		"https://1..2.3/",
		"https://runner.xn--/",
		"https://éxample.com/",
		"https://bad\x01host.example/",
		"https://[2001:db8::1",
		"https://[2001:db8::1]extra/",
		"https://[2001:db8::1]:/",
		"https://example.com/\n" + marker,
		strings.Repeat("x", MaxEndpointURLBytes+1),
	}
	for index, raw := range tests {
		if _, err := DeriveEndpointHint(raw); !errors.Is(err, ErrEndpointHintInvalid) {
			t.Fatalf("case %d error = %v, want fixed endpoint hint error", index, err)
		} else if len(err.Error()) > 96 || strings.Contains(err.Error(), marker) {
			t.Fatal("endpoint hint error leaked input or was unbounded")
		}
	}
}

func TestSecretProjectionHasExactExportedAndJSONFieldWhitelist(t *testing.T) {
	input := SecretProjectionInput{
		EndpointHint: "https://***.com:8443",
		EndpointEnvelope: &SecretEnvelopeInput{
			Active: true,
		},
		CredentialFingerprint: "d9e33adc9261a0bbbb82a25b171277c8",
		CredentialEnvelope: &SecretEnvelopeInput{
			NeedsRewrap: true,
		},
	}
	projection, err := ProjectSecrets(input)
	if err != nil {
		t.Fatalf("ProjectSecrets() error = %v", err)
	}
	if projection.EndpointHint != input.EndpointHint || !projection.CredentialConfigured {
		t.Fatal("projection omitted safe endpoint or configured state")
	}
	if projection.Active || !projection.NeedsRewrap {
		t.Fatal("projection did not aggregate active/rewrap state safely")
	}

	exportedNames := map[string]struct{}{
		"EndpointHint": {}, "CredentialConfigured": {}, "CredentialFingerprint": {},
		"Active": {}, "NeedsRewrap": {},
	}
	projectionType := reflect.TypeOf(SecretProjection{})
	if projectionType.NumField() != len(exportedNames) {
		t.Fatalf("SecretProjection exported field count = %d, want %d", projectionType.NumField(), len(exportedNames))
	}
	for index := 0; index < projectionType.NumField(); index++ {
		field := projectionType.Field(index)
		if _, allowed := exportedNames[field.Name]; !allowed || !field.IsExported() {
			t.Fatalf("SecretProjection exposed unexpected field %q", field.Name)
		}
	}

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	assertExactProjectionJSONKeys(t, encoded)
}

func TestSecretProjectionRejectsInconsistentOrNonCanonicalMetadata(t *testing.T) {
	validFingerprint := "d9e33adc9261a0bbbb82a25b171277c8"
	tests := []SecretProjectionInput{
		{EndpointHint: "https://full.private.example", EndpointEnvelope: &SecretEnvelopeInput{Active: true}},
		{EndpointHint: "https://a**.e******.com", EndpointEnvelope: &SecretEnvelopeInput{Active: true}},
		{EndpointHint: "https://***.com/path", EndpointEnvelope: &SecretEnvelopeInput{Active: true}},
		{EndpointHint: "https://***.com:", EndpointEnvelope: &SecretEnvelopeInput{Active: true}},
		{EndpointHint: "https://***.example.com", EndpointEnvelope: &SecretEnvelopeInput{Active: true}},
		{EndpointHint: "https://***.com"},
		{EndpointEnvelope: &SecretEnvelopeInput{Active: true}},
		{EndpointHint: "https://***.com", EndpointEnvelope: &SecretEnvelopeInput{}},
		{EndpointHint: "https://***.com", EndpointEnvelope: &SecretEnvelopeInput{Active: true, NeedsRewrap: true}},
		{CredentialFingerprint: validFingerprint},
		{CredentialEnvelope: &SecretEnvelopeInput{Active: true}},
		{CredentialFingerprint: strings.Repeat("A", 32), CredentialEnvelope: &SecretEnvelopeInput{Active: true}},
		{CredentialFingerprint: validFingerprint, CredentialEnvelope: &SecretEnvelopeInput{}},
		{CredentialFingerprint: validFingerprint, CredentialEnvelope: &SecretEnvelopeInput{Active: true, NeedsRewrap: true}},
	}
	for index, input := range tests {
		if _, err := ProjectSecrets(input); !errors.Is(err, ErrSecretProjectionInvalid) {
			t.Fatalf("case %d error = %v, want fixed projection error", index, err)
		} else if len(err.Error()) > 96 {
			t.Fatal("projection error is unbounded")
		}
	}
}

func TestSecretProjectionRealEncryptionLeakageChain(t *testing.T) {
	const (
		keyID               = "synthetic-rotation-key"
		providerKey         = "provider_1"
		pathToken           = "synthetic-path-secret-7f4c"
		queryToken          = "synthetic-query-secret-9a2e"
		credentialPlaintext = "synthetic-credential-secret-5d8b"
	)
	keyMaterial := bytes.Repeat([]byte{0x7a}, 32)
	encodedKey := base64.StdEncoding.EncodeToString(keyMaterial)
	keysJSON, err := json.Marshal(map[string]string{keyID: encodedKey})
	if err != nil {
		t.Fatalf("key fixture marshal error = %v", err)
	}
	keyRing, err := infrasandbox.ParseSandboxKeyRing(string(keysJSON), keyID)
	if err != nil {
		t.Fatalf("ParseSandboxKeyRing() error = %v", err)
	}
	codec, err := infrasandbox.NewCredentialCodec(keyRing)
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}

	endpointWithRejectedComponents := "https://runner.private.example.com:9443/" + pathToken + "?token=" + queryToken
	if _, err := DeriveEndpointHint(endpointWithRejectedComponents); !errors.Is(err, ErrEndpointHintInvalid) {
		t.Fatalf("secret path/query endpoint error = %v, want fixed rejection", err)
	}
	validEndpointForHint := "https://runner.private.example.com:9443/"
	endpointHint, err := DeriveEndpointHint(validEndpointForHint)
	if err != nil {
		t.Fatalf("DeriveEndpointHint(valid root) error = %v", err)
	}
	endpointEnvelope, err := codec.Encrypt(
		providerKey,
		infrasandbox.CredentialFieldEndpoint,
		[]byte(endpointWithRejectedComponents),
	)
	if err != nil {
		t.Fatalf("endpoint Encrypt() error = %v", err)
	}
	credentialEnvelope, err := codec.Encrypt(
		providerKey,
		infrasandbox.CredentialFieldCredential,
		[]byte(credentialPlaintext),
	)
	if err != nil {
		t.Fatalf("credential Encrypt() error = %v", err)
	}
	endpointMetadata, err := codec.Inspect(endpointEnvelope)
	if err != nil {
		t.Fatalf("endpoint Inspect() error = %v", err)
	}
	credentialMetadata, err := codec.Inspect(credentialEnvelope)
	if err != nil {
		t.Fatalf("credential Inspect() error = %v", err)
	}
	fingerprint, err := codec.FingerprintCredential([]byte(credentialPlaintext))
	if err != nil {
		t.Fatalf("FingerprintCredential() error = %v", err)
	}

	projection, err := ProjectSecrets(SecretProjectionInput{
		EndpointHint: endpointHint,
		EndpointEnvelope: &SecretEnvelopeInput{
			Active: endpointMetadata.Active, NeedsRewrap: endpointMetadata.NeedsRewrap,
		},
		CredentialFingerprint: fingerprint,
		CredentialEnvelope: &SecretEnvelopeInput{
			Active: credentialMetadata.Active, NeedsRewrap: credentialMetadata.NeedsRewrap,
		},
	})
	if err != nil {
		t.Fatalf("ProjectSecrets() error = %v", err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("projection marshal error = %v", err)
	}
	assertExactProjectionJSONKeys(t, encoded)

	for _, forbidden := range []string{
		endpointWithRejectedComponents,
		validEndpointForHint,
		"runner",
		"private",
		"example",
		pathToken,
		queryToken,
		credentialPlaintext,
		endpointEnvelope,
		credentialEnvelope,
		envelopeNonceSegment(t, endpointEnvelope),
		envelopeNonceSegment(t, credentialEnvelope),
		keyID,
		encodedKey,
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("projection JSON leaked secret-chain material")
		}
	}
	if projection.EndpointHint != "https://***.com:9443" || !projection.Active || projection.NeedsRewrap {
		t.Fatal("projection safe metadata mismatch")
	}
}

func TestSecretReplacementSemantics(t *testing.T) {
	omitted, err := ResolveSecretReplacement(SecretFieldCredential, SecretReplacementInput{})
	if err != nil || omitted.Mode() != SecretReplacementOmitted {
		t.Fatalf("omitted replacement mismatch: %#v, %v", omitted, err)
	}
	if _, ok := omitted.Value(); ok {
		t.Fatal("omitted credential was auto-filled")
	}

	replaced, err := ResolveSecretReplacement(SecretFieldEndpoint, SecretReplacementInput{Supplied: true, Value: "https://endpoint.example/"})
	if err != nil || replaced.Mode() != SecretReplacementReplace {
		t.Fatalf("replace semantics mismatch: %#v, %v", replaced, err)
	}
	if value, ok := replaced.Value(); !ok || value != "https://endpoint.example/" {
		t.Fatal("replacement value unavailable to write path")
	}

	removed, err := ResolveSecretReplacement(SecretFieldCredential, SecretReplacementInput{Supplied: true, Remove: true})
	if err != nil || removed.Mode() != SecretReplacementRemove {
		t.Fatalf("credential removal mismatch: %#v, %v", removed, err)
	}
	if _, ok := removed.Value(); ok {
		t.Fatal("removal exposed a replacement value")
	}

	invalid := []struct {
		field SecretField
		input SecretReplacementInput
	}{
		{field: SecretField("unknown")},
		{field: SecretFieldEndpoint, input: SecretReplacementInput{Value: "unsupplied"}},
		{field: SecretFieldEndpoint, input: SecretReplacementInput{Supplied: true}},
		{field: SecretFieldEndpoint, input: SecretReplacementInput{Supplied: true, Remove: true}},
		{field: SecretFieldCredential, input: SecretReplacementInput{Supplied: true, Remove: true, Value: "contradiction"}},
	}
	for index, test := range invalid {
		if _, err := ResolveSecretReplacement(test.field, test.input); !errors.Is(err, ErrSecretReplacementInvalid) {
			t.Fatalf("case %d error = %v, want fixed replacement error", index, err)
		}
	}
}

func assertExactProjectionJSONKeys(t *testing.T, encoded []byte) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("projection JSON decode error = %v", err)
	}
	allowed := map[string]struct{}{
		"endpoint_hint": {}, "credential_configured": {}, "credential_fingerprint": {},
		"active": {}, "needs_rewrap": {},
	}
	if len(object) != len(allowed) {
		t.Fatalf("projection JSON key count = %d, want %d", len(object), len(allowed))
	}
	for key := range object {
		if _, exists := allowed[key]; !exists {
			t.Fatalf("projection JSON exposed unexpected key %q", key)
		}
	}
}

func envelopeNonceSegment(t *testing.T, envelope string) string {
	t.Helper()
	segments := strings.SplitN(envelope, ":", 5)
	if len(segments) != 4 {
		t.Fatal("synthetic envelope shape is invalid")
	}
	return segments[2]
}
