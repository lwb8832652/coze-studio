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

package modelmgr

import (
	"errors"
	"fmt"

	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	ModelCredentialKeysJSONEnv    = "MODEL_CREDENTIAL_KEYS_JSON"
	ModelCredentialActiveKeyIDEnv = "MODEL_CREDENTIAL_ACTIVE_KEY_ID"
)

var ErrModelCredentialCodecMissing = errors.New("model credential encryption is not configured")

type ModelCredentialCodec interface {
	Encrypt(modelID, endpointID int64, plaintext string) (envelope, fingerprint string, err error)
	Decrypt(modelID, endpointID int64, envelope string) (string, error)
	Rewrap(modelID, endpointID int64, envelope string) (
		rewrapped, fingerprint string,
		changed bool,
		err error,
	)
}

type modelCredentialCodec struct {
	codec *infrasandbox.CredentialCodec
}

func LoadModelCredentialCodec(getenv func(string) string) (ModelCredentialCodec, error) {
	if getenv == nil {
		return nil, nil
	}
	keysJSON := getenv(ModelCredentialKeysJSONEnv)
	activeKeyID := getenv(ModelCredentialActiveKeyIDEnv)
	if keysJSON == "" && activeKeyID == "" {
		return nil, nil
	}
	if keysJSON == "" || activeKeyID == "" {
		return nil, fmt.Errorf("model credential key ring is incomplete")
	}
	keyRing, err := infrasandbox.ParseSandboxKeyRing(keysJSON, activeKeyID)
	if err != nil {
		return nil, fmt.Errorf("parse model credential key ring: %w", err)
	}
	codec, err := infrasandbox.NewCredentialCodec(keyRing)
	if err != nil {
		return nil, fmt.Errorf("create model credential codec: %w", err)
	}
	return &modelCredentialCodec{codec: codec}, nil
}

func (c *modelCredentialCodec) Encrypt(
	modelID, endpointID int64,
	plaintext string,
) (string, string, error) {
	if c == nil || c.codec == nil || modelID <= 0 || endpointID <= 0 || plaintext == "" {
		return "", "", ErrModelCredentialCodecMissing
	}
	raw := []byte(plaintext)
	defer wipeModelCredential(raw)
	envelope, err := c.codec.Encrypt(
		modelCredentialProviderKey(modelID, endpointID),
		infrasandbox.CredentialFieldCredential,
		raw,
	)
	if err != nil {
		return "", "", err
	}
	fingerprint, err := c.codec.FingerprintCredential(raw)
	if err != nil {
		return "", "", err
	}
	return envelope, fingerprint, nil
}

func (c *modelCredentialCodec) Decrypt(
	modelID, endpointID int64,
	envelope string,
) (string, error) {
	if c == nil || c.codec == nil || modelID <= 0 || endpointID <= 0 || envelope == "" {
		return "", ErrModelCredentialCodecMissing
	}
	raw, err := c.codec.Decrypt(
		modelCredentialProviderKey(modelID, endpointID),
		infrasandbox.CredentialFieldCredential,
		envelope,
	)
	if err != nil {
		return "", err
	}
	secret := string(raw)
	wipeModelCredential(raw)
	return secret, nil
}

func (c *modelCredentialCodec) Rewrap(
	modelID, endpointID int64,
	envelope string,
) (string, string, bool, error) {
	if c == nil || c.codec == nil || modelID <= 0 || endpointID <= 0 || envelope == "" {
		return "", "", false, ErrModelCredentialCodecMissing
	}
	result, err := c.codec.Rewrap(
		modelCredentialProviderKey(modelID, endpointID),
		infrasandbox.CredentialFieldCredential,
		envelope,
	)
	if err != nil {
		return "", "", false, err
	}
	return result.Envelope, result.Fingerprint, result.Rewrapped, nil
}

func modelCredentialProviderKey(modelID, endpointID int64) string {
	return fmt.Sprintf("model-%d-endpoint-%d", modelID, endpointID)
}

func wipeModelCredential(raw []byte) {
	for index := range raw {
		raw[index] = 0
	}
}
