// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSandboxKeyRingStrictParsing(t *testing.T) {
	activeKey := sandboxTestKey(0x22)
	oldKey := sandboxTestKey(0x11)
	standardAlphabetKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32))
	urlAlphabetKey := base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32))
	canonicalZeroKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	alternateZeroKey := canonicalZeroKey[:len(canonicalZeroKey)-2] + "B="
	validJSON := fmt.Sprintf(`{"old-key":%q,"active.v1":%q}`, oldKey, activeKey)
	ring, err := ParseSandboxKeyRing(validJSON, "active.v1")
	if err != nil {
		t.Fatalf("ParseSandboxKeyRing() error = %v", err)
	}
	if ring.ActiveKeyID() != "active.v1" || len(ring.keys) != 2 {
		t.Fatalf("unexpected parsed key ring metadata")
	}
	if _, err := ParseSandboxKeyRing(fmt.Sprintf(`{"active":%q}`, standardAlphabetKey), "active"); err != nil {
		t.Fatalf("canonical standard-base64 alphabet key was rejected: %v", err)
	}

	tests := []struct {
		name     string
		keysJSON string
		activeID string
	}{
		{name: "empty JSON", activeID: "active.v1"},
		{name: "array shape", keysJSON: `[]`, activeID: "active.v1"},
		{name: "null shape", keysJSON: `null`, activeID: "active.v1"},
		{name: "non-string value", keysJSON: `{"active.v1":7}`, activeID: "active.v1"},
		{name: "nested value", keysJSON: `{"active.v1":{"key":"value"}}`, activeID: "active.v1"},
		{name: "duplicate key", keysJSON: fmt.Sprintf(`{"active.v1":%q,"active.v1":%q}`, activeKey, activeKey), activeID: "active.v1"},
		{name: "colon key id", keysJSON: fmt.Sprintf(`{"bad:key":%q}`, activeKey), activeID: "bad:key"},
		{name: "invalid key id start", keysJSON: fmt.Sprintf(`{"_bad":%q}`, activeKey), activeID: "_bad"},
		{name: "long key id", keysJSON: fmt.Sprintf(`{%q:%q}`, strings.Repeat("a", 65), activeKey), activeID: strings.Repeat("a", 65)},
		{name: "malformed base64", keysJSON: `{"active.v1":"%%%"}`, activeID: "active.v1"},
		{name: "base64 whitespace", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, activeKey+" "), activeID: "active.v1"},
		{name: "base64 carriage return", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, activeKey+"\r"), activeID: "active.v1"},
		{name: "base64 newline", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, activeKey+"\n"), activeID: "active.v1"},
		{name: "base64 tab", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, activeKey+"\t"), activeID: "active.v1"},
		{name: "missing canonical padding", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, strings.TrimRight(activeKey, "=")), activeID: "active.v1"},
		{name: "excess padding", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, activeKey+"="), activeID: "active.v1"},
		{name: "URL alphabet confusion", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, urlAlphabetKey), activeID: "active.v1"},
		{name: "alternate textual encoding", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, alternateZeroKey), activeID: "active.v1"},
		{name: "short decoded key", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, base64.StdEncoding.EncodeToString(make([]byte, 31))), activeID: "active.v1"},
		{name: "long decoded key", keysJSON: fmt.Sprintf(`{"active.v1":%q}`, base64.StdEncoding.EncodeToString(make([]byte, 33))), activeID: "active.v1"},
		{name: "missing active id", keysJSON: validJSON},
		{name: "trimmed active mismatch", keysJSON: validJSON, activeID: " active.v1"},
		{name: "unknown active id", keysJSON: validJSON, activeID: "missing"},
		{name: "trailing JSON", keysJSON: validJSON + `{}`, activeID: "active.v1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseSandboxKeyRing(test.keysJSON, test.activeID)
			if !errors.Is(err, ErrSandboxKeyRingInvalid) {
				t.Fatalf("ParseSandboxKeyRing() error = %v, want fixed key-ring error", err)
			}
			assertSandboxCodecErrorSafe(t, err, activeKey, oldKey, test.keysJSON)
		})
	}
}

func TestSandboxKeyRingGetenvInjection(t *testing.T) {
	values := map[string]string{
		SandboxCredentialKeysJSONEnv:    fmt.Sprintf(`{"active":%q}`, sandboxTestKey(0x33)),
		SandboxCredentialActiveKeyIDEnv: "active",
	}
	var requested []string
	ring, err := LoadSandboxKeyRing(func(name string) string {
		requested = append(requested, name)
		return values[name]
	})
	if err != nil {
		t.Fatalf("LoadSandboxKeyRing() error = %v", err)
	}
	if ring.ActiveKeyID() != "active" {
		t.Fatalf("active key ID mismatch")
	}
	if len(requested) != 2 || requested[0] != SandboxCredentialKeysJSONEnv || requested[1] != SandboxCredentialActiveKeyIDEnv {
		t.Fatalf("unexpected environment lookup contract: %v", requested)
	}
	if _, err := LoadSandboxKeyRing(nil); !errors.Is(err, ErrSandboxKeyRingInvalid) {
		t.Fatalf("nil getenv error = %v", err)
	}
}

func TestCredentialCodecProductionConstructorOwnsCryptoRandomReader(t *testing.T) {
	keysJSON := fmt.Sprintf(`{"active":%q}`, sandboxTestKey(0x22))
	ring, err := ParseSandboxKeyRing(keysJSON, "active")
	if err != nil {
		t.Fatalf("ParseSandboxKeyRing() error = %v", err)
	}
	codec, err := NewCredentialCodec(ring)
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}
	if codec.nonceSource != rand.Reader {
		t.Fatal("production credential codec does not own crypto/rand.Reader")
	}
}

func TestCredentialCodecSerializesConcurrentNonceReads(t *testing.T) {
	const workers = 24
	keysJSON := fmt.Sprintf(`{"active":%q}`, sandboxTestKey(0x22))
	ring, err := ParseSandboxKeyRing(keysJSON, "active")
	if err != nil {
		t.Fatalf("ParseSandboxKeyRing() error = %v", err)
	}
	reader := &coordinatedUnsafeNonceReader{
		entered: make(chan struct{}, workers),
		release: make(chan struct{}, workers),
	}
	codec, err := newCredentialCodecWithNonceSource(ring, reader)
	if err != nil {
		t.Fatalf("newCredentialCodecWithNonceSource() error = %v", err)
	}

	start := make(chan struct{})
	results := make(chan credentialConcurrentResult, workers)
	for index := 0; index < workers; index++ {
		go func() {
			<-start
			envelope, err := codec.Encrypt(
				"provider_1",
				CredentialFieldCredential,
				[]byte("same-synthetic-concurrent-credential"),
			)
			results <- credentialConcurrentResult{envelope: envelope, err: err}
		}()
	}
	close(start)
	for index := 0; index < workers; index++ {
		<-reader.entered
		reader.release <- struct{}{}
	}

	nonces := make(map[string]struct{}, workers)
	envelopes := make(map[string]struct{}, workers)
	for index := 0; index < workers; index++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent Encrypt() error = %v", result.err)
		}
		segments := strings.SplitN(result.envelope, ":", 5)
		if len(segments) != 4 {
			t.Fatal("concurrent Encrypt() returned invalid envelope")
		}
		nonces[segments[2]] = struct{}{}
		envelopes[result.envelope] = struct{}{}
	}
	if len(nonces) != workers || len(envelopes) != workers {
		t.Fatalf("unique nonces/envelopes = %d/%d, want %d/%d", len(nonces), len(envelopes), workers, workers)
	}
}

func TestSandboxEnvelopeRoundTripMetadataAADAndLimits(t *testing.T) {
	codec := sandboxTestCodec(t, "active", bytes.Repeat([]byte{0x44}, 12*5))
	endpoint := []byte("https://private.synthetic.example/path?token=hidden")
	envelope, err := codec.Encrypt("provider_1", CredentialFieldEndpoint, endpoint)
	if err != nil {
		t.Fatalf("Encrypt(endpoint) error = %v", err)
	}
	if strings.Contains(envelope, string(endpoint)) {
		t.Fatal("endpoint plaintext appears in envelope")
	}
	metadata, err := codec.Inspect(envelope)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if metadata.KeyID != "active" || !metadata.Active || metadata.NeedsRewrap {
		t.Fatalf("unexpected active envelope metadata: %#v", metadata)
	}
	decoded, err := codec.Decrypt("provider_1", CredentialFieldEndpoint, envelope)
	if err != nil || !bytes.Equal(decoded, endpoint) {
		t.Fatalf("Decrypt(endpoint) failed")
	}
	if _, err := codec.Decrypt("provider_2", CredentialFieldEndpoint, envelope); !errors.Is(err, ErrCredentialDecryptFailed) {
		t.Fatalf("wrong-provider AAD error = %v", err)
	}
	if _, err := codec.Decrypt("provider_1", CredentialFieldCredential, envelope); !errors.Is(err, ErrCredentialDecryptFailed) {
		t.Fatalf("wrong-field AAD error = %v", err)
	}
	if _, err := codec.Encrypt("Provider", CredentialFieldEndpoint, endpoint); !errors.Is(err, ErrCredentialInputInvalid) {
		t.Fatalf("noncanonical provider key error = %v", err)
	}
	if _, err := codec.Encrypt("provider_1", CredentialField("unknown"), endpoint); !errors.Is(err, ErrCredentialInputInvalid) {
		t.Fatalf("unknown field error = %v", err)
	}
	if _, err := codec.Encrypt("provider_1", CredentialFieldEndpoint, nil); !errors.Is(err, ErrCredentialInputInvalid) {
		t.Fatalf("empty endpoint error = %v", err)
	}
	if _, err := codec.Encrypt("provider_1", CredentialFieldEndpoint, bytes.Repeat([]byte{'e'}, MaxEndpointPlaintextBytes+1)); !errors.Is(err, ErrCredentialInputInvalid) {
		t.Fatalf("oversized endpoint error = %v", err)
	}
	if _, err := codec.Encrypt("provider_1", CredentialFieldCredential, bytes.Repeat([]byte{'c'}, MaxCredentialPlaintextBytes+1)); !errors.Is(err, ErrCredentialInputInvalid) {
		t.Fatalf("oversized credential error = %v", err)
	}
}

func TestSandboxEnvelopeRejectsMalformedUnknownAndTamperedValues(t *testing.T) {
	codec := sandboxTestCodec(t, "active", bytes.Repeat([]byte{0x55}, 24))
	secret := []byte("sandbox-synthetic-secret-marker")
	envelope, err := codec.Encrypt("provider-1", CredentialFieldCredential, secret)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	parts := strings.Split(envelope, ":")
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatal("test envelope ciphertext decode failed")
	}
	ciphertext[len(ciphertext)-1] ^= 1
	tampered := strings.Join([]string{parts[0], parts[1], parts[2], base64.RawURLEncoding.EncodeToString(ciphertext)}, ":")
	alternateCiphertext := alternateRawURLBase64(parts[3])
	originalDecoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		t.Fatal("canonical ciphertext fixture decode failed")
	}
	alternateDecoded, err := base64.RawURLEncoding.DecodeString(alternateCiphertext)
	if err != nil || !bytes.Equal(alternateDecoded, originalDecoded) || alternateCiphertext == parts[3] {
		t.Fatal("alternate raw URL base64 fixture is not equivalent")
	}
	tests := []struct {
		name  string
		value string
	}{
		{name: "empty"},
		{name: "too few segments", value: "v1:active:nonce"},
		{name: "too many segments", value: envelope + ":extra"},
		{name: "unknown version", value: strings.Replace(envelope, "v1:", "v2:", 1)},
		{name: "unknown key", value: strings.Replace(envelope, ":active:", ":unknown:", 1)},
		{name: "invalid key id", value: strings.Replace(envelope, ":active:", ":bad$key:", 1)},
		{name: "padded nonce", value: strings.Join([]string{parts[0], parts[1], parts[2] + "=", parts[3]}, ":")},
		{name: "nonce carriage return", value: strings.Join([]string{parts[0], parts[1], parts[2] + "\r", parts[3]}, ":")},
		{name: "nonce newline", value: strings.Join([]string{parts[0], parts[1], parts[2] + "\n", parts[3]}, ":")},
		{name: "nonce space", value: strings.Join([]string{parts[0], parts[1], parts[2] + " ", parts[3]}, ":")},
		{name: "nonce tab", value: strings.Join([]string{parts[0], parts[1], parts[2] + "\t", parts[3]}, ":")},
		{name: "standard alphabet confusion", value: strings.Join([]string{parts[0], parts[1], base64.RawStdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 12)), parts[3]}, ":")},
		{name: "short nonce", value: strings.Join([]string{parts[0], parts[1], base64.RawURLEncoding.EncodeToString(make([]byte, 11)), parts[3]}, ":")},
		{name: "long nonce", value: strings.Join([]string{parts[0], parts[1], base64.RawURLEncoding.EncodeToString(make([]byte, 13)), parts[3]}, ":")},
		{name: "malformed ciphertext", value: strings.Join([]string{parts[0], parts[1], parts[2], "%%%"}, ":")},
		{name: "padded ciphertext", value: strings.Join([]string{parts[0], parts[1], parts[2], parts[3] + "="}, ":")},
		{name: "ciphertext carriage return", value: strings.Join([]string{parts[0], parts[1], parts[2], parts[3] + "\r"}, ":")},
		{name: "ciphertext newline", value: strings.Join([]string{parts[0], parts[1], parts[2], parts[3] + "\n"}, ":")},
		{name: "ciphertext space", value: strings.Join([]string{parts[0], parts[1], parts[2], parts[3] + " "}, ":")},
		{name: "ciphertext tab", value: strings.Join([]string{parts[0], parts[1], parts[2], parts[3] + "\t"}, ":")},
		{name: "alternate ciphertext text", value: strings.Join([]string{parts[0], parts[1], parts[2], alternateCiphertext}, ":")},
		{name: "empty plaintext ciphertext", value: strings.Join([]string{parts[0], parts[1], parts[2], base64.RawURLEncoding.EncodeToString(make([]byte, 16))}, ":")},
		{name: "oversized", value: strings.Repeat("x", MaxSandboxCredentialEnvelopeBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codec.Inspect(test.value)
			if !errors.Is(err, ErrSandboxEnvelopeInvalid) {
				t.Fatalf("Inspect() error = %v, want fixed envelope error", err)
			}
			assertSandboxCodecErrorSafe(t, err, string(secret), envelope, test.value)
		})
	}
	if _, err := codec.Decrypt("provider-1", CredentialFieldCredential, tampered); !errors.Is(err, ErrCredentialDecryptFailed) {
		t.Fatalf("tamper error = %v", err)
	} else {
		assertSandboxCodecErrorSafe(t, err, string(secret), tampered)
	}
}

func TestSandboxEnvelopeRejectsMaxSizeAllColonAdversary(t *testing.T) {
	codec := sandboxTestCodec(t, "active", nil)
	_, err := codec.Inspect(strings.Repeat(":", MaxSandboxCredentialEnvelopeBytes))
	if !errors.Is(err, ErrSandboxEnvelopeInvalid) {
		t.Fatalf("all-colon envelope error = %v, want fixed envelope error", err)
	}
}

func TestCredentialFingerprintAndSingleValueRewrap(t *testing.T) {
	keysJSON := fmt.Sprintf(`{"old":%q,"active":%q}`, sandboxTestKey(0x11), sandboxTestKey(0x22))
	oldRing, err := ParseSandboxKeyRing(keysJSON, "old")
	if err != nil {
		t.Fatalf("old key ring error = %v", err)
	}
	oldCodec, err := newCredentialCodecWithNonceSource(oldRing, bytes.NewReader(bytes.Repeat([]byte{0x61}, 24)))
	if err != nil {
		t.Fatalf("old codec error = %v", err)
	}
	credential := []byte("credential-synthetic-value")
	oldEnvelope, err := oldCodec.Encrypt("provider_1", CredentialFieldCredential, credential)
	if err != nil {
		t.Fatalf("old Encrypt() error = %v", err)
	}
	oldFingerprint, err := oldCodec.FingerprintCredential(credential)
	if err != nil {
		t.Fatalf("old FingerprintCredential() error = %v", err)
	}

	activeRing, err := ParseSandboxKeyRing(keysJSON, "active")
	if err != nil {
		t.Fatalf("active key ring error = %v", err)
	}
	activeCodec, err := newCredentialCodecWithNonceSource(activeRing, bytes.NewReader(bytes.Repeat([]byte{0x62}, 24)))
	if err != nil {
		t.Fatalf("active codec error = %v", err)
	}
	fingerprint, err := activeCodec.FingerprintCredential(credential)
	if err != nil {
		t.Fatalf("FingerprintCredential() error = %v", err)
	}
	if fingerprint != "d9e33adc9261a0bbbb82a25b171277c8" {
		t.Fatalf("fingerprint fixture mismatch")
	}
	if oldFingerprint == fingerprint {
		t.Fatal("fingerprint did not change across active-key epochs")
	}
	staleMetadata, err := activeCodec.Inspect(oldEnvelope)
	if err != nil {
		t.Fatalf("Inspect(old envelope) error = %v", err)
	}
	if staleMetadata.Active || !staleMetadata.NeedsRewrap || staleMetadata.KeyID != "old" {
		t.Fatal("old envelope did not transition to needs-rewrap under the active epoch")
	}
	rawDigest := sha256.Sum256(credential)
	if fingerprint == fmt.Sprintf("%x", rawDigest[:16]) {
		t.Fatal("credential fingerprint is a raw SHA-256 prefix")
	}
	if _, err := activeCodec.FingerprintCredential(nil); !errors.Is(err, ErrCredentialInputInvalid) {
		t.Fatalf("empty fingerprint input error = %v", err)
	}

	rewrapped, err := activeCodec.Rewrap("provider_1", CredentialFieldCredential, oldEnvelope)
	if err != nil {
		t.Fatalf("Rewrap() error = %v", err)
	}
	if !rewrapped.Rewrapped || rewrapped.Envelope == oldEnvelope || rewrapped.Fingerprint != fingerprint || rewrapped.Metadata.KeyID != "active" {
		t.Fatalf("unexpected rewrap result metadata")
	}
	if !rewrapped.Metadata.Active || rewrapped.Metadata.NeedsRewrap {
		t.Fatal("rewrapped envelope did not transition to active metadata")
	}
	if strings.Contains(fmt.Sprintf("%#v", rewrapped), string(credential)) {
		t.Fatal("rewrap result leaked plaintext")
	}
	decoded, err := activeCodec.Decrypt("provider_1", CredentialFieldCredential, rewrapped.Envelope)
	if err != nil || !bytes.Equal(decoded, credential) {
		t.Fatal("rewrapped credential did not decrypt")
	}

	idempotentCodec, err := newCredentialCodecWithNonceSource(activeRing, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("idempotent codec error = %v", err)
	}
	idempotent, err := idempotentCodec.Rewrap("provider_1", CredentialFieldCredential, rewrapped.Envelope)
	if err != nil {
		t.Fatalf("active-key Rewrap() error = %v", err)
	}
	if idempotent.Rewrapped || idempotent.Envelope != rewrapped.Envelope || idempotent.Fingerprint != fingerprint {
		t.Fatal("active-key rewrap was not ciphertext-idempotent")
	}

	oldEndpoint, err := oldCodec.Encrypt("provider_1", CredentialFieldEndpoint, []byte("https://endpoint.example/path"))
	if err != nil {
		t.Fatalf("old endpoint Encrypt() error = %v", err)
	}
	endpointResult, err := activeCodec.Rewrap("provider_1", CredentialFieldEndpoint, oldEndpoint)
	if err != nil {
		t.Fatalf("endpoint Rewrap() error = %v", err)
	}
	if endpointResult.Fingerprint != "" || !endpointResult.Rewrapped {
		t.Fatal("endpoint rewrap produced credential fingerprint or failed to rotate")
	}
}

func sandboxTestKey(fill byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{fill}, 32))
}

func alternateRawURLBase64(canonical string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	remainder := len(canonical) % 4
	if len(canonical) == 0 || remainder != 2 && remainder != 3 {
		return canonical
	}
	last := strings.IndexByte(alphabet, canonical[len(canonical)-1])
	if last < 0 {
		return canonical
	}
	alternate := []byte(canonical)
	alternate[len(alternate)-1] = alphabet[last^1]
	return string(alternate)
}

func sandboxTestCodec(t *testing.T, activeID string, nonceBytes []byte) *CredentialCodec {
	t.Helper()
	keysJSON := fmt.Sprintf(`{"old":%q,"active":%q}`, sandboxTestKey(0x11), sandboxTestKey(0x22))
	ring, err := ParseSandboxKeyRing(keysJSON, activeID)
	if err != nil {
		t.Fatalf("ParseSandboxKeyRing() error = %v", err)
	}
	codec, err := newCredentialCodecWithNonceSource(ring, bytes.NewReader(nonceBytes))
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}
	return codec
}

type credentialConcurrentResult struct {
	envelope string
	err      error
}

var errConcurrentNonceRead = errors.New("concurrent nonce reader use")

type coordinatedUnsafeNonceReader struct {
	entered  chan struct{}
	release  chan struct{}
	reading  bool
	sequence byte
}

func (r *coordinatedUnsafeNonceReader) Read(buffer []byte) (int, error) {
	concurrent := r.reading
	r.reading = true
	r.entered <- struct{}{}
	<-r.release
	if concurrent {
		r.reading = false
		return 0, errConcurrentNonceRead
	}
	r.sequence++
	for index := range buffer {
		buffer[index] = r.sequence
	}
	r.reading = false
	return len(buffer), nil
}

func assertSandboxCodecErrorSafe(t *testing.T, err error, markers ...string) {
	t.Helper()
	if err == nil || len(err.Error()) > 128 {
		t.Fatal("sandbox codec error is nil or unbounded")
	}
	for _, marker := range markers {
		if marker != "" && strings.Contains(err.Error(), marker) {
			t.Fatal("sandbox codec error leaked caller data")
		}
	}
}
