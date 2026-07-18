// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/secureaead"
)

const (
	SandboxCredentialKeysJSONEnv    = "SANDBOX_CREDENTIAL_KEYS_JSON"
	SandboxCredentialActiveKeyIDEnv = "SANDBOX_CREDENTIAL_ACTIVE_KEY_ID"

	MaxEndpointPlaintextBytes         = 4096
	MaxCredentialPlaintextBytes       = 65536
	MaxSandboxCredentialEnvelopeBytes = 87488

	maxSandboxKeyRingJSONBytes = 64 * 1024
	maxSandboxKeyRingKeys      = 128
	sandboxEnvelopeVersion     = "v1"
	sandboxGCMNonceBytes       = 12
	sandboxGCMTagBytes         = 16

	credentialFingerprintKeyDomain = "sandbox-credential-fingerprint-key-v1"
	credentialFingerprintDomain    = "sandbox-credential-fingerprint-v1"
)

var (
	ErrSandboxKeyRingInvalid   = errors.New("sandbox credential key ring is invalid")
	ErrCredentialCodecInvalid  = errors.New("sandbox credential codec is invalid")
	ErrCredentialInputInvalid  = errors.New("sandbox credential input is invalid")
	ErrSandboxEnvelopeInvalid  = errors.New("sandbox credential envelope is invalid")
	ErrCredentialEncryptFailed = errors.New("sandbox credential encryption failed")
	ErrCredentialDecryptFailed = errors.New("sandbox credential decryption failed")
	ErrCredentialRewrapFailed  = errors.New("sandbox credential rewrap failed")
)

type CredentialField string

const (
	CredentialFieldEndpoint   CredentialField = "endpoint"
	CredentialFieldCredential CredentialField = "credential"
)

type SandboxKeyRing struct {
	keys        map[string][]byte
	activeKeyID string
}

type CredentialCodec struct {
	keyRing     *SandboxKeyRing
	nonceSource io.Reader
	nonceMu     sync.Mutex
}

type CredentialEnvelopeMetadata struct {
	KeyID       string
	Active      bool
	NeedsRewrap bool
}

type CredentialRewrapResult struct {
	Envelope    string
	Fingerprint string
	Rewrapped   bool
	Metadata    CredentialEnvelopeMetadata
}

type parsedCredentialEnvelope struct {
	keyID      string
	nonce      []byte
	ciphertext []byte
}

func ParseSandboxKeyRing(keysJSON, activeKeyID string) (*SandboxKeyRing, error) {
	if len(keysJSON) == 0 || len(keysJSON) > maxSandboxKeyRingJSONBytes || !validSandboxKeyID(activeKeyID) {
		return nil, ErrSandboxKeyRingInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(keysJSON))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrSandboxKeyRingInvalid
	}
	keys := make(map[string][]byte)
	for decoder.More() {
		keyToken, err := decoder.Token()
		keyID, ok := keyToken.(string)
		if err != nil || !ok || !validSandboxKeyID(keyID) || len(keys) >= maxSandboxKeyRingKeys {
			wipeKeyMap(keys)
			return nil, ErrSandboxKeyRingInvalid
		}
		if _, exists := keys[keyID]; exists {
			wipeKeyMap(keys)
			return nil, ErrSandboxKeyRingInvalid
		}
		valueToken, err := decoder.Token()
		encodedKey, ok := valueToken.(string)
		if err != nil || !ok {
			wipeKeyMap(keys)
			return nil, ErrSandboxKeyRingInvalid
		}
		decodedKey, canonical := decodeCanonicalStandardBase64(encodedKey)
		if !canonical || len(decodedKey) != 32 {
			wipeBytes(decodedKey)
			wipeKeyMap(keys)
			return nil, ErrSandboxKeyRingInvalid
		}
		keys[keyID] = decodedKey
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		wipeKeyMap(keys)
		return nil, ErrSandboxKeyRingInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		wipeKeyMap(keys)
		return nil, ErrSandboxKeyRingInvalid
	}
	if _, exists := keys[activeKeyID]; !exists {
		wipeKeyMap(keys)
		return nil, ErrSandboxKeyRingInvalid
	}
	return &SandboxKeyRing{keys: keys, activeKeyID: activeKeyID}, nil
}

func LoadSandboxKeyRing(getenv func(string) string) (*SandboxKeyRing, error) {
	if getenv == nil {
		return nil, ErrSandboxKeyRingInvalid
	}
	keysJSON := getenv(SandboxCredentialKeysJSONEnv)
	activeKeyID := getenv(SandboxCredentialActiveKeyIDEnv)
	return ParseSandboxKeyRing(keysJSON, activeKeyID)
}

func (r *SandboxKeyRing) ActiveKeyID() string {
	if r == nil {
		return ""
	}
	return r.activeKeyID
}

// NewCredentialCodec fixes crypto/rand.Reader and serialized nonce reads as a
// constructor invariant. AES-GCM security requires a unique nonce for every
// encryption performed with the same key.
func NewCredentialCodec(keyRing *SandboxKeyRing) (*CredentialCodec, error) {
	return newCredentialCodecWithNonceSource(keyRing, rand.Reader)
}

func newCredentialCodecWithNonceSource(keyRing *SandboxKeyRing, nonceSource io.Reader) (*CredentialCodec, error) {
	if keyRing == nil || nonceSource == nil || !validSandboxKeyID(keyRing.activeKeyID) {
		return nil, ErrCredentialCodecInvalid
	}
	keys := make(map[string][]byte, len(keyRing.keys))
	for keyID, key := range keyRing.keys {
		if !validSandboxKeyID(keyID) || len(key) != 32 {
			wipeKeyMap(keys)
			return nil, ErrCredentialCodecInvalid
		}
		keys[keyID] = append([]byte(nil), key...)
	}
	if _, exists := keys[keyRing.activeKeyID]; !exists {
		wipeKeyMap(keys)
		return nil, ErrCredentialCodecInvalid
	}
	return &CredentialCodec{
		keyRing:     &SandboxKeyRing{keys: keys, activeKeyID: keyRing.activeKeyID},
		nonceSource: nonceSource,
	}, nil
}

func (c *CredentialCodec) Encrypt(providerKey string, field CredentialField, plaintext []byte) (string, error) {
	if !c.valid() || domainsandbox.ValidateProviderKey(providerKey) != nil || !validCredentialPlaintext(field, plaintext) {
		return "", ErrCredentialInputInvalid
	}
	key := c.keyRing.keys[c.keyRing.activeKeyID]
	nonce, ciphertext, err := c.seal(key, credentialAAD(providerKey, field), plaintext)
	if err != nil {
		return "", ErrCredentialEncryptFailed
	}
	envelope := encodeCredentialEnvelope(c.keyRing.activeKeyID, nonce, ciphertext)
	if len(envelope) > MaxSandboxCredentialEnvelopeBytes {
		return "", ErrCredentialEncryptFailed
	}
	return envelope, nil
}

func (c *CredentialCodec) Decrypt(providerKey string, field CredentialField, envelope string) ([]byte, error) {
	if !c.valid() || domainsandbox.ValidateProviderKey(providerKey) != nil || !validCredentialField(field) {
		return nil, ErrCredentialInputInvalid
	}
	parsed, _, err := c.parseEnvelope(envelope)
	if err != nil {
		return nil, err
	}
	return c.decryptParsed(providerKey, field, parsed)
}

func (c *CredentialCodec) Inspect(envelope string) (CredentialEnvelopeMetadata, error) {
	if !c.valid() {
		return CredentialEnvelopeMetadata{}, ErrCredentialCodecInvalid
	}
	_, metadata, err := c.parseEnvelope(envelope)
	return metadata, err
}

// FingerprintCredential returns an active-key epoch-scoped display/change
// detector. It is intentionally not stable across key rotation epochs.
func (c *CredentialCodec) FingerprintCredential(plaintext []byte) (string, error) {
	if !c.valid() || len(plaintext) == 0 || len(plaintext) > MaxCredentialPlaintextBytes {
		return "", ErrCredentialInputInvalid
	}
	activeKey := c.keyRing.keys[c.keyRing.activeKeyID]
	derive := hmac.New(sha256.New, activeKey)
	_, _ = derive.Write([]byte(credentialFingerprintKeyDomain))
	fingerprintKey := derive.Sum(nil)
	defer wipeBytes(fingerprintKey)
	mac := hmac.New(sha256.New, fingerprintKey)
	_, _ = mac.Write([]byte(credentialFingerprintDomain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(plaintext)
	digest := mac.Sum(nil)
	return hex.EncodeToString(digest[:16]), nil
}

func (c *CredentialCodec) Rewrap(providerKey string, field CredentialField, envelope string) (CredentialRewrapResult, error) {
	if !c.valid() || domainsandbox.ValidateProviderKey(providerKey) != nil || !validCredentialField(field) {
		return CredentialRewrapResult{}, ErrCredentialInputInvalid
	}
	parsed, metadata, err := c.parseEnvelope(envelope)
	if err != nil {
		return CredentialRewrapResult{}, err
	}
	plaintext, err := c.decryptParsed(providerKey, field, parsed)
	if err != nil {
		return CredentialRewrapResult{}, err
	}
	defer wipeBytes(plaintext)
	fingerprint := ""
	if field == CredentialFieldCredential {
		fingerprint, err = c.FingerprintCredential(plaintext)
		if err != nil {
			return CredentialRewrapResult{}, ErrCredentialRewrapFailed
		}
	}
	if metadata.Active {
		return CredentialRewrapResult{
			Envelope: envelope, Fingerprint: fingerprint, Metadata: metadata,
		}, nil
	}
	activeKey := c.keyRing.keys[c.keyRing.activeKeyID]
	nonce, ciphertext, err := c.seal(activeKey, credentialAAD(providerKey, field), plaintext)
	if err != nil {
		return CredentialRewrapResult{}, ErrCredentialRewrapFailed
	}
	rewrappedEnvelope := encodeCredentialEnvelope(c.keyRing.activeKeyID, nonce, ciphertext)
	if len(rewrappedEnvelope) > MaxSandboxCredentialEnvelopeBytes {
		return CredentialRewrapResult{}, ErrCredentialRewrapFailed
	}
	return CredentialRewrapResult{
		Envelope:    rewrappedEnvelope,
		Fingerprint: fingerprint,
		Rewrapped:   true,
		Metadata: CredentialEnvelopeMetadata{
			KeyID: c.keyRing.activeKeyID, Active: true,
		},
	}, nil
}

func (c *CredentialCodec) parseEnvelope(envelope string) (parsedCredentialEnvelope, CredentialEnvelopeMetadata, error) {
	if len(envelope) == 0 || len(envelope) > MaxSandboxCredentialEnvelopeBytes {
		return parsedCredentialEnvelope{}, CredentialEnvelopeMetadata{}, ErrSandboxEnvelopeInvalid
	}
	segments := strings.SplitN(envelope, ":", 5)
	if len(segments) != 4 || segments[0] != sandboxEnvelopeVersion || !validSandboxKeyID(segments[1]) {
		return parsedCredentialEnvelope{}, CredentialEnvelopeMetadata{}, ErrSandboxEnvelopeInvalid
	}
	if _, exists := c.keyRing.keys[segments[1]]; !exists {
		return parsedCredentialEnvelope{}, CredentialEnvelopeMetadata{}, ErrSandboxEnvelopeInvalid
	}
	nonce, canonical := decodeCanonicalRawURLBase64(segments[2])
	if !canonical || len(nonce) != sandboxGCMNonceBytes {
		return parsedCredentialEnvelope{}, CredentialEnvelopeMetadata{}, ErrSandboxEnvelopeInvalid
	}
	ciphertext, canonical := decodeCanonicalRawURLBase64(segments[3])
	if !canonical || len(ciphertext) <= sandboxGCMTagBytes || len(ciphertext) > MaxCredentialPlaintextBytes+sandboxGCMTagBytes {
		return parsedCredentialEnvelope{}, CredentialEnvelopeMetadata{}, ErrSandboxEnvelopeInvalid
	}
	needsRewrap := segments[1] != c.keyRing.activeKeyID
	return parsedCredentialEnvelope{
			keyID: segments[1], nonce: nonce, ciphertext: ciphertext,
		}, CredentialEnvelopeMetadata{
			KeyID: segments[1], Active: !needsRewrap, NeedsRewrap: needsRewrap,
		}, nil
}

func (c *CredentialCodec) decryptParsed(providerKey string, field CredentialField, envelope parsedCredentialEnvelope) ([]byte, error) {
	plaintext, err := secureaead.Open(
		c.keyRing.keys[envelope.keyID],
		credentialAAD(providerKey, field),
		envelope.nonce,
		envelope.ciphertext,
	)
	if err != nil || !validCredentialPlaintext(field, plaintext) {
		wipeBytes(plaintext)
		return nil, ErrCredentialDecryptFailed
	}
	return plaintext, nil
}

func (c *CredentialCodec) seal(key, aad, plaintext []byte) ([]byte, []byte, error) {
	c.nonceMu.Lock()
	defer c.nonceMu.Unlock()
	return secureaead.Seal(key, aad, plaintext, c.nonceSource)
}

func (c *CredentialCodec) valid() bool {
	if c == nil || c.keyRing == nil || c.nonceSource == nil || !validSandboxKeyID(c.keyRing.activeKeyID) {
		return false
	}
	key, exists := c.keyRing.keys[c.keyRing.activeKeyID]
	return exists && len(key) == 32
}

func validCredentialPlaintext(field CredentialField, plaintext []byte) bool {
	if len(plaintext) == 0 {
		return false
	}
	switch field {
	case CredentialFieldEndpoint:
		return len(plaintext) <= MaxEndpointPlaintextBytes
	case CredentialFieldCredential:
		return len(plaintext) <= MaxCredentialPlaintextBytes
	default:
		return false
	}
}

func validCredentialField(field CredentialField) bool {
	return field == CredentialFieldEndpoint || field == CredentialFieldCredential
}

func credentialAAD(providerKey string, field CredentialField) []byte {
	return []byte("sandbox-provider:" + providerKey + ":" + string(field))
}

func encodeCredentialEnvelope(keyID string, nonce, ciphertext []byte) string {
	encoding := base64.RawURLEncoding
	return strings.Join([]string{
		sandboxEnvelopeVersion,
		keyID,
		encoding.EncodeToString(nonce),
		encoding.EncodeToString(ciphertext),
	}, ":")
}

func decodeCanonicalStandardBase64(encoded string) ([]byte, bool) {
	if encoded == "" || !validBase64Alphabet(encoded, false) {
		return nil, false
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != encoded {
		wipeBytes(decoded)
		return nil, false
	}
	return decoded, true
}

func decodeCanonicalRawURLBase64(encoded string) ([]byte, bool) {
	if encoded == "" || !validBase64Alphabet(encoded, true) {
		return nil, false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		wipeBytes(decoded)
		return nil, false
	}
	return decoded, true
}

func validBase64Alphabet(encoded string, rawURL bool) bool {
	for index := range encoded {
		character := encoded[index]
		if isASCIIAlphaNumeric(character) {
			continue
		}
		if rawURL && (character == '-' || character == '_') {
			continue
		}
		if !rawURL && (character == '+' || character == '/' || character == '=') {
			continue
		}
		return false
	}
	return true
}

func validSandboxKeyID(keyID string) bool {
	if len(keyID) == 0 || len(keyID) > 64 || !isASCIIAlphaNumeric(keyID[0]) {
		return false
	}
	for index := 1; index < len(keyID); index++ {
		character := keyID[index]
		if !isASCIIAlphaNumeric(character) && character != '.' && character != '_' && character != '-' {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(character byte) bool {
	return character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}

func wipeKeyMap(keys map[string][]byte) {
	for _, key := range keys {
		wipeBytes(key)
	}
}

func wipeBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
