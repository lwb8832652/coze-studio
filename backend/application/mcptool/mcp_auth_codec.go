// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/pkg/secureaead"
)

const (
	MCPAESAuthSecretEnv       = "MCP_AES_AUTH_SECRET"
	mcpAESAuthEnvelopeVersion = "aes-gcm-v1"

	maxMCPAuthPlaintextBytes            = 256 * 1024
	maxMCPAuthEnvelopeBytes             = 512 * 1024
	mcpAESAuthMaxPlaintextBytes         = maxMCPAuthPlaintextBytes
	mcpAESAuthMaxEnvelopeBytes          = maxMCPAuthEnvelopeBytes
	mcpAESAuthGCMNonceBytes             = 12
	mcpAESAuthGCMTagBytes               = 16
	mcpAESAuthMaxCiphertextBytes        = mcpAESAuthMaxPlaintextBytes + mcpAESAuthGCMTagBytes
	mcpAESAuthMaxEncodedCiphertextBytes = (mcpAESAuthMaxCiphertextBytes*8 + 5) / 6
)

var (
	errMCPAuthEnvelopeInvalid  = errors.New("mcp auth AES envelope is invalid")
	errMCPAuthEncryptionFailed = errors.New("mcp auth AES encryption failed")
	errMCPAuthDecryptionFailed = errors.New("mcp auth AES decryption failed")
)

type MCPAuthCodec interface {
	EncodeMCPAuth(ctx context.Context, auth string) (string, error)
	DecodeMCPAuth(ctx context.Context, stored string) (string, error)
}

type AESMCPAuthCodec struct {
	secret      string
	nonceSource io.Reader
}

type mcpAESAuthEnvelope struct {
	Payload mcpAESAuthEnvelopePayload `json:"_coze_mcp_auth"`
}

type mcpAESAuthEnvelopePayload struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func NewAESMCPAuthCodec(secret string) (*AESMCPAuthCodec, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("%s is required", MCPAESAuthSecretEnv)
	}
	switch len([]byte(secret)) {
	case 16, 24, 32:
		return &AESMCPAuthCodec{secret: secret, nonceSource: rand.Reader}, nil
	default:
		return nil, fmt.Errorf("%s must be 16, 24, or 32 bytes", MCPAESAuthSecretEnv)
	}
}

func (c *AESMCPAuthCodec) EncodeMCPAuth(
	_ context.Context,
	auth string,
) (string, error) {
	if c == nil || c.secret == "" || len(auth) > mcpAESAuthMaxPlaintextBytes || !validUniqueJSONObjectString(auth) {
		return "", errMCPAuthEncryptionFailed
	}
	plaintext := []byte(auth)
	nonce, ciphertext, err := secureaead.Seal(
		[]byte(c.secret),
		[]byte(mcpAESAuthEnvelopeVersion),
		plaintext,
		c.effectiveNonceSource(),
	)
	if err != nil {
		return "", errMCPAuthEncryptionFailed
	}
	encoded, err := json.Marshal(&mcpAESAuthEnvelope{
		Payload: mcpAESAuthEnvelopePayload{
			Version:    mcpAESAuthEnvelopeVersion,
			Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
			Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		},
	})
	if err != nil || len(encoded) > mcpAESAuthMaxEnvelopeBytes {
		return "", errMCPAuthEncryptionFailed
	}
	return string(encoded), nil
}

func (c *AESMCPAuthCodec) DecodeMCPAuth(
	_ context.Context,
	stored string,
) (string, error) {
	if c == nil || c.secret == "" {
		return "", errMCPAuthDecryptionFailed
	}
	if len(stored) == 0 || len(stored) > mcpAESAuthMaxEnvelopeBytes || !utf8.ValidString(stored) {
		return "", errMCPAuthEnvelopeInvalid
	}
	envelope, err := parseStrictMCPAuthEnvelope(stored)
	if err != nil || envelope.Version != mcpAESAuthEnvelopeVersion {
		return "", errMCPAuthEnvelopeInvalid
	}
	nonce, ok := decodeCanonicalMCPRawURLBase64(
		envelope.Nonce,
		mcpAESAuthGCMNonceBytes,
		mcpAESAuthGCMNonceBytes,
	)
	if !ok {
		return "", errMCPAuthEnvelopeInvalid
	}
	ciphertext, ok := decodeCanonicalMCPRawURLBase64(
		envelope.Ciphertext,
		mcpAESAuthGCMTagBytes,
		mcpAESAuthMaxCiphertextBytes,
	)
	if !ok {
		return "", errMCPAuthEnvelopeInvalid
	}
	decoded, err := secureaead.Open(
		[]byte(c.secret),
		[]byte(mcpAESAuthEnvelopeVersion),
		nonce,
		ciphertext,
	)
	if err != nil || len(decoded) > mcpAESAuthMaxPlaintextBytes || !validUniqueJSONObjectBytes(decoded) {
		return "", errMCPAuthDecryptionFailed
	}
	return string(decoded), nil
}

func (c *AESMCPAuthCodec) effectiveNonceSource() io.Reader {
	if c != nil && c.nonceSource != nil {
		return c.nonceSource
	}
	return rand.Reader
}

func parseStrictMCPAuthEnvelope(stored string) (mcpAESAuthEnvelopePayload, error) {
	decoder := json.NewDecoder(strings.NewReader(stored))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
	}
	seenPayload := false
	var payload mcpAESAuthEnvelopePayload
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok || key != "_coze_mcp_auth" || seenPayload {
			return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
		}
		seenPayload = true
		payload, err = parseStrictMCPAuthEnvelopePayload(decoder)
		if err != nil {
			return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || !seenPayload || !jsonDecoderAtEOF(decoder) {
		return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
	}
	return payload, nil
}

func parseStrictMCPAuthEnvelopePayload(decoder *json.Decoder) (mcpAESAuthEnvelopePayload, error) {
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
	}
	seen := make(map[string]struct{}, 3)
	var payload mcpAESAuthEnvelopePayload
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
		}
		if _, duplicate := seen[key]; duplicate {
			return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
		}
		seen[key] = struct{}{}
		valueToken, err := decoder.Token()
		value, ok := valueToken.(string)
		if err != nil || !ok {
			return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
		}
		switch key {
		case "version":
			payload.Version = value
		case "nonce":
			payload.Nonce = value
		case "ciphertext":
			payload.Ciphertext = value
		default:
			return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || len(seen) != 3 {
		return mcpAESAuthEnvelopePayload{}, errMCPAuthEnvelopeInvalid
	}
	return payload, nil
}

func decodeCanonicalMCPRawURLBase64(encoded string, minDecoded, maxDecoded int) ([]byte, bool) {
	if encoded == "" || minDecoded < 0 || maxDecoded < minDecoded {
		return nil, false
	}
	encoding := base64.RawURLEncoding
	if len(encoded) > encoding.EncodedLen(maxDecoded) {
		return nil, false
	}
	decodedLength := encoding.DecodedLen(len(encoded))
	if decodedLength < minDecoded || decodedLength > maxDecoded {
		return nil, false
	}
	for index := range encoded {
		character := encoded[index]
		if character >= 'A' && character <= 'Z' ||
			character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			continue
		}
		return nil, false
	}
	decoded, err := encoding.Strict().DecodeString(encoded)
	if err != nil ||
		len(decoded) < minDecoded ||
		len(decoded) > maxDecoded ||
		encoding.EncodeToString(decoded) != encoded {
		return nil, false
	}
	return decoded, true
}

func validUniqueJSONObjectString(raw string) bool {
	if !utf8.ValidString(raw) {
		return false
	}
	return validUniqueJSONObjectDecoder(json.NewDecoder(strings.NewReader(raw)))
}

func validUniqueJSONObjectBytes(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	return validUniqueJSONObjectDecoder(json.NewDecoder(bytes.NewReader(raw)))
}

func validUniqueJSONObjectDecoder(decoder *json.Decoder) bool {
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') || !consumeUniqueJSONObject(decoder) {
		return false
	}
	return jsonDecoderAtEOF(decoder)
}

func consumeUniqueJSONObject(decoder *json.Decoder) bool {
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return false
		}
		if _, duplicate := seen[key]; duplicate {
			return false
		}
		seen[key] = struct{}{}
		if !consumeUniqueJSONValue(decoder) {
			return false
		}
	}
	closing, err := decoder.Token()
	return err == nil && closing == json.Delim('}')
}

func consumeUniqueJSONArray(decoder *json.Decoder) bool {
	for decoder.More() {
		if !consumeUniqueJSONValue(decoder) {
			return false
		}
	}
	closing, err := decoder.Token()
	return err == nil && closing == json.Delim(']')
}

func consumeUniqueJSONValue(decoder *json.Decoder) bool {
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		return consumeUniqueJSONObject(decoder)
	case '[':
		return consumeUniqueJSONArray(decoder)
	default:
		return false
	}
}

func jsonDecoderAtEOF(decoder *json.Decoder) bool {
	_, err := decoder.Token()
	return errors.Is(err, io.EOF)
}

var errMCPAuthObjectInvalid = errors.New("mcp auth object is invalid")

type mcpAuthObjectParseResult struct {
	fieldCount               int
	hasReservedEnvelopeField bool
}

// parseBoundedMCPAuthObjectString is the package-wide catalog boundary parser.
// It bounds input before allocation, reuses the recursive unique-key validator,
// and derives top-level classification metadata from that same validated object.
func parseBoundedMCPAuthObjectString(raw string, maxBytes int) (mcpAuthObjectParseResult, error) {
	if maxBytes < 0 || len(raw) > maxBytes || !utf8.ValidString(raw) || !validUniqueJSONObjectString(raw) {
		return mcpAuthObjectParseResult{}, errMCPAuthObjectInvalid
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || fields == nil {
		return mcpAuthObjectParseResult{}, errMCPAuthObjectInvalid
	}
	_, hasReservedEnvelopeField := fields["_coze_mcp_auth"]
	return mcpAuthObjectParseResult{
		fieldCount:               len(fields),
		hasReservedEnvelopeField: hasReservedEnvelopeField,
	}, nil
}
