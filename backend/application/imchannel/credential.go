// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"fmt"

	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	IMChannelCredentialKeysJSONEnv    = "IM_CHANNEL_CREDENTIAL_KEYS_JSON"
	IMChannelCredentialActiveKeyIDEnv = "IM_CHANNEL_CREDENTIAL_ACTIVE_KEY_ID"
)

type CredentialCodec interface {
	Encrypt(configID int64, plaintext string) (string, string, error)
	Decrypt(configID int64, envelope string) (string, error)
}

type sandboxCredentialCodec struct {
	codec *infrasandbox.CredentialCodec
}

func LoadCredentialCodec(getenv func(string) string) (CredentialCodec, error) {
	if getenv == nil {
		return nil, nil
	}
	keysJSON := getenv(IMChannelCredentialKeysJSONEnv)
	activeKeyID := getenv(IMChannelCredentialActiveKeyIDEnv)
	if keysJSON == "" && activeKeyID == "" {
		keysJSON = getenv(infrasandbox.SandboxCredentialKeysJSONEnv)
		activeKeyID = getenv(infrasandbox.SandboxCredentialActiveKeyIDEnv)
	}
	if keysJSON == "" && activeKeyID == "" {
		return nil, nil
	}
	if keysJSON == "" || activeKeyID == "" {
		return nil, fmt.Errorf("IM channel credential key ring is incomplete")
	}
	keyRing, err := infrasandbox.ParseSandboxKeyRing(keysJSON, activeKeyID)
	if err != nil {
		return nil, fmt.Errorf("parse IM channel credential key ring: %w", err)
	}
	codec, err := infrasandbox.NewCredentialCodec(keyRing)
	if err != nil {
		return nil, fmt.Errorf("create IM channel credential codec: %w", err)
	}
	return &sandboxCredentialCodec{codec: codec}, nil
}

func (c *sandboxCredentialCodec) Encrypt(configID int64, plaintext string) (string, string, error) {
	if c == nil || c.codec == nil || configID <= 0 || plaintext == "" {
		return "", "", domain.ErrCredentialCodecMissing
	}
	raw := []byte(plaintext)
	envelope, err := c.codec.Encrypt(
		credentialProviderKey(configID),
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

func (c *sandboxCredentialCodec) Decrypt(configID int64, envelope string) (string, error) {
	if c == nil || c.codec == nil || configID <= 0 || envelope == "" {
		return "", domain.ErrCredentialCodecMissing
	}
	raw, err := c.codec.Decrypt(
		credentialProviderKey(configID),
		infrasandbox.CredentialFieldCredential,
		envelope,
	)
	if err != nil {
		return "", err
	}
	secret := string(raw)
	for index := range raw {
		raw[index] = 0
	}
	return secret, nil
}

func credentialProviderKey(configID int64) string {
	return fmt.Sprintf("feishu-im-%d", configID)
}
