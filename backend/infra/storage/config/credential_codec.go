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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/pkg/secureaead"
)

const (
	ObjectStorageCredentialKeyEnv = "OBJECT_STORAGE_CREDENTIAL_KEY"
	credentialEnvelopeVersion     = "v1"
)

type CredentialCodec struct {
	key        []byte
	nonce      io.Reader
	nonceMutex sync.Mutex
}

type credentialEnvelope struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type credentialPayload struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

func LoadCredentialCodec(getenv func(string) string) (*CredentialCodec, error) {
	if getenv == nil {
		return nil, domain.ErrCredentialUnavailable
	}
	key, err := ParseCredentialKey(getenv(ObjectStorageCredentialKeyEnv))
	if err != nil {
		return nil, err
	}
	return NewCredentialCodec(key, nil)
}

func ParseCredentialKey(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, domain.ErrCredentialUnavailable
	}
	if len(decoded) != 32 || base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, domain.ErrCredentialUnavailable
	}
	return decoded, nil
}

func NewCredentialCodec(key []byte, nonceSource io.Reader) (*CredentialCodec, error) {
	if len(key) != 32 {
		return nil, domain.ErrCredentialUnavailable
	}
	copiedKey := append([]byte(nil), key...)
	if nonceSource == nil {
		nonceSource = rand.Reader
	}
	if nilReader(nonceSource) {
		return nil, domain.ErrCredentialUnavailable
	}
	return &CredentialCodec{
		key:   copiedKey,
		nonce: nonceSource,
	}, nil
}

func (c *CredentialCodec) Encrypt(id uint64, provider domain.ProviderType, version uint64, input domain.CredentialInput) (string, error) {
	if c == nil {
		return "", domain.ErrCredentialUnavailable
	}
	normalized := domain.NormalizeCredentialInput(input)
	if err := domain.ValidateCredentialInput(normalized); err != nil {
		return "", err
	}
	if !domain.HasCredentialPair(normalized) {
		return "", errors.Join(domain.ErrConfigInvalid, errors.New("credential pair is required"))
	}

	plaintext, err := json.Marshal(credentialPayload{
		AccessKeyID:     normalized.AccessKeyID,
		SecretAccessKey: normalized.SecretAccessKey,
	})
	if err != nil {
		return "", domain.ErrCredentialUnavailable
	}

	c.nonceMutex.Lock()
	defer c.nonceMutex.Unlock()
	nonce, ciphertext, err := secureaead.Seal(c.key, credentialAAD(id, provider, version), plaintext, c.nonce)
	if err != nil {
		return "", domain.ErrCredentialUnavailable
	}

	envelope, err := json.Marshal(credentialEnvelope{
		Version:    credentialEnvelopeVersion,
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Ciphertext: base64.StdEncoding.EncodeToString(ciphertext),
	})
	if err != nil {
		return "", domain.ErrCredentialUnavailable
	}
	return string(envelope), nil
}

func (c *CredentialCodec) Decrypt(id uint64, provider domain.ProviderType, version uint64, envelope string) (domain.CredentialInput, error) {
	if c == nil {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	parsed, err := decodeCredentialEnvelope(envelope)
	if err != nil {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	if parsed.Version != credentialEnvelopeVersion {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	nonce, err := base64.StdEncoding.Strict().DecodeString(parsed.Nonce)
	if err != nil || base64.StdEncoding.EncodeToString(nonce) != parsed.Nonce {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	ciphertext, err := base64.StdEncoding.Strict().DecodeString(parsed.Ciphertext)
	if err != nil || base64.StdEncoding.EncodeToString(ciphertext) != parsed.Ciphertext {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}

	plaintext, err := secureaead.Open(c.key, credentialAAD(id, provider, version), nonce, ciphertext)
	if err != nil {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	var payload credentialPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	output := domain.NormalizeCredentialInput(domain.CredentialInput{
		AccessKeyID:     payload.AccessKeyID,
		SecretAccessKey: payload.SecretAccessKey,
	})
	if err := domain.ValidateCredentialInput(output); err != nil || !domain.HasCredentialPair(output) {
		return domain.CredentialInput{}, domain.ErrCredentialUnavailable
	}
	return output, nil
}

func decodeCredentialEnvelope(envelope string) (credentialEnvelope, error) {
	decoder := json.NewDecoder(strings.NewReader(envelope))
	start, err := decoder.Token()
	if err != nil {
		return credentialEnvelope{}, err
	}
	delimiter, ok := start.(json.Delim)
	if !ok || delimiter != '{' {
		return credentialEnvelope{}, domain.ErrCredentialUnavailable
	}

	seen := map[string]struct{}{}
	var parsed credentialEnvelope
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return credentialEnvelope{}, err
		}
		field, ok := token.(string)
		if !ok {
			return credentialEnvelope{}, domain.ErrCredentialUnavailable
		}
		if _, exists := seen[field]; exists {
			return credentialEnvelope{}, domain.ErrCredentialUnavailable
		}
		seen[field] = struct{}{}

		var value string
		if err := decoder.Decode(&value); err != nil {
			return credentialEnvelope{}, err
		}
		switch field {
		case "version":
			parsed.Version = value
		case "nonce":
			parsed.Nonce = value
		case "ciphertext":
			parsed.Ciphertext = value
		default:
			return credentialEnvelope{}, domain.ErrCredentialUnavailable
		}
	}

	end, err := decoder.Token()
	if err != nil {
		return credentialEnvelope{}, err
	}
	delimiter, ok = end.(json.Delim)
	if !ok || delimiter != '}' {
		return credentialEnvelope{}, domain.ErrCredentialUnavailable
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return credentialEnvelope{}, domain.ErrCredentialUnavailable
	}
	return parsed, nil
}

func credentialAAD(id uint64, provider domain.ProviderType, version uint64) []byte {
	var builder strings.Builder
	builder.WriteString("object-storage-credential/")
	builder.WriteString(credentialEnvelopeVersion)
	builder.WriteByte(':')
	builder.WriteString(strconv.FormatUint(id, 10))
	builder.WriteByte(':')
	builder.WriteString(string(provider))
	builder.WriteByte(':')
	builder.WriteString(strconv.FormatUint(version, 10))
	return []byte(builder.String())
}

func nilReader(reader io.Reader) bool {
	value := reflect.ValueOf(reader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
