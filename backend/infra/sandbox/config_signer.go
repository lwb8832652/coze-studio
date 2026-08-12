// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const SchedulerConfigurationSchemaV1 = "coze.sandbox.scheduler_config.v1"

const (
	SandboxRunnerConfigSigningKeysJSONEnv = "SANDBOX_RUNNER_CONFIG_SIGNING_KEYS_JSON"
	SandboxRunnerActiveConfigKeyIDEnv     = "SANDBOX_RUNNER_ACTIVE_CONFIG_KEY_ID"
)

var ErrSchedulerConfiguration = errors.New("sandbox scheduler configuration is invalid")

// SchedulerConfiguration is the complete, signed non-secret scheduler
// snapshot accepted by a Runner. Secrets, credentials and endpoints are never
// part of this envelope.
type SchedulerConfiguration struct {
	Schema    string                          `json:"schema"`
	KeyID     string                          `json:"key_id"`
	IssuedAt  string                          `json:"issued_at"`
	ExpiresAt string                          `json:"expires_at"`
	Version   uint64                          `json:"version"`
	Settings  domainsandbox.SchedulerSettings `json:"settings"`
	Signature string                          `json:"signature"`
}

func (SchedulerConfiguration) String() string { return "SchedulerConfiguration{signature:<redacted>}" }
func (SchedulerConfiguration) GoString() string {
	return "SchedulerConfiguration{signature:<redacted>}"
}

type SchedulerConfigSigner struct {
	activeKeyID string
	keys        map[string][]byte
	ttl         time.Duration
	now         func() time.Time
}

func NewSchedulerConfigSigner(activeKeyID string, keys map[string][]byte, ttl time.Duration) (*SchedulerConfigSigner, error) {
	if !validSchedulerConfigKeyID(activeKeyID) || ttl <= 0 || ttl > time.Hour || len(keys) == 0 {
		return nil, ErrSchedulerConfiguration
	}
	copyKeys := make(map[string][]byte, len(keys))
	for id, value := range keys {
		if !validSchedulerConfigKeyID(id) || len(value) < 16 {
			return nil, ErrSchedulerConfiguration
		}
		copyKeys[id] = append([]byte(nil), value...)
	}
	if len(copyKeys[activeKeyID]) == 0 {
		return nil, ErrSchedulerConfiguration
	}
	return &SchedulerConfigSigner{activeKeyID: activeKeyID, keys: copyKeys, ttl: ttl, now: func() time.Time { return time.Now().UTC() }}, nil
}

// LoadSchedulerConfigSignerFromEnv loads the NewX-side signing keyring. Both
// variables may be absent while no native Runner is configured; a partial or
// malformed keyring is rejected rather than silently disabling verification.
func LoadSchedulerConfigSignerFromEnv(getenv func(string) string, ttl time.Duration) (*SchedulerConfigSigner, bool, error) {
	if getenv == nil || ttl <= 0 {
		return nil, false, ErrSchedulerConfiguration
	}
	keysJSON := getenv(SandboxRunnerConfigSigningKeysJSONEnv)
	activeKeyID := getenv(SandboxRunnerActiveConfigKeyIDEnv)
	if keysJSON == "" && activeKeyID == "" {
		return nil, false, nil
	}
	if keysJSON == "" || activeKeyID == "" || len(keysJSON) > 64*1024 {
		return nil, false, ErrSchedulerConfiguration
	}
	if err := rejectDuplicateSchedulerConfigurationJSONKeys([]byte(keysJSON)); err != nil {
		return nil, false, ErrSchedulerConfiguration
	}
	var value struct {
		Keys map[string]string `json:"keys"`
	}
	decoder := json.NewDecoder(strings.NewReader(keysJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, false, ErrSchedulerConfiguration
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || len(value.Keys) == 0 || len(value.Keys) > 128 {
		return nil, false, ErrSchedulerConfiguration
	}
	keys := make(map[string][]byte, len(value.Keys))
	for keyID, key := range value.Keys {
		if !validSchedulerConfigKeyID(keyID) || len(key) < 16 {
			return nil, false, ErrSchedulerConfiguration
		}
		keys[keyID] = []byte(key)
	}
	signer, err := NewSchedulerConfigSigner(activeKeyID, keys, ttl)
	if err != nil {
		return nil, false, ErrSchedulerConfiguration
	}
	return signer, true, nil
}

func (signer *SchedulerConfigSigner) Sign(settings domainsandbox.SchedulerSettings) (SchedulerConfiguration, error) {
	if signer == nil || signer.now == nil {
		return SchedulerConfiguration{}, ErrSchedulerConfiguration
	}
	normalized, err := domainsandbox.NormalizeSchedulerSettings(settings)
	if err != nil || normalized.Version == 0 {
		return SchedulerConfiguration{}, ErrSchedulerConfiguration
	}
	now := signer.now().UTC()
	version := normalized.Version
	normalized.Version = 0
	normalized.UpdatedBy = 0
	value := SchedulerConfiguration{
		Schema: SchedulerConfigurationSchemaV1, KeyID: signer.activeKeyID, Version: version,
		IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(signer.ttl).Format(time.RFC3339Nano), Settings: normalized,
	}
	signature, err := signer.signature(value)
	if err != nil {
		return SchedulerConfiguration{}, err
	}
	value.Signature = signature
	return value, nil
}

func (signer *SchedulerConfigSigner) Verify(value SchedulerConfiguration, currentVersion uint64) (domainsandbox.SchedulerSettings, error) {
	if signer == nil || signer.now == nil || !validSchedulerConfiguration(value) || currentVersion > value.Version {
		return domainsandbox.SchedulerSettings{}, ErrSchedulerConfiguration
	}
	issuedAt, issueErr := time.Parse(time.RFC3339Nano, value.IssuedAt)
	expiresAt, expiryErr := time.Parse(time.RFC3339Nano, value.ExpiresAt)
	now := signer.now().UTC()
	if issueErr != nil || expiryErr != nil || !expiresAt.After(issuedAt) || now.Before(issuedAt.Add(-time.Minute)) || !now.Before(expiresAt) {
		return domainsandbox.SchedulerSettings{}, ErrSchedulerConfiguration
	}
	expected, err := signer.signature(value)
	if err != nil || !hmac.Equal([]byte(expected), []byte(value.Signature)) {
		return domainsandbox.SchedulerSettings{}, ErrSchedulerConfiguration
	}
	settings := value.Settings
	settings.Version = value.Version
	return domainsandbox.NormalizeSchedulerSettings(settings)
}

func (signer *SchedulerConfigSigner) signature(value SchedulerConfiguration) (string, error) {
	if signer == nil || !validSchedulerConfigKeyID(value.KeyID) || len(signer.keys[value.KeyID]) == 0 {
		return "", ErrSchedulerConfiguration
	}
	payload, err := schedulerConfigurationPayload(value)
	if err != nil {
		return "", ErrSchedulerConfiguration
	}
	mac := hmac.New(sha256.New, signer.keys[value.KeyID])
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func schedulerConfigurationPayload(value SchedulerConfiguration) ([]byte, error) {
	normalized, err := domainsandbox.NormalizeSchedulerSettings(value.Settings)
	if err != nil || value.Schema != SchedulerConfigurationSchemaV1 || !validSchedulerConfigKeyID(value.KeyID) || value.IssuedAt == "" || value.ExpiresAt == "" {
		return nil, ErrSchedulerConfiguration
	}
	payload := struct {
		Schema    string                          `json:"schema"`
		KeyID     string                          `json:"key_id"`
		IssuedAt  string                          `json:"issued_at"`
		ExpiresAt string                          `json:"expires_at"`
		Version   uint64                          `json:"version"`
		Settings  domainsandbox.SchedulerSettings `json:"settings"`
	}{Schema: value.Schema, KeyID: value.KeyID, IssuedAt: value.IssuedAt, ExpiresAt: value.ExpiresAt, Version: value.Version, Settings: normalized}
	return json.Marshal(payload)
}

func DecodeSchedulerConfiguration(data []byte) (SchedulerConfiguration, error) {
	if len(data) == 0 || len(data) > 64*1024 {
		return SchedulerConfiguration{}, ErrSchedulerConfiguration
	}
	if err := rejectDuplicateSchedulerConfigurationJSONKeys(data); err != nil {
		return SchedulerConfiguration{}, ErrSchedulerConfiguration
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value SchedulerConfiguration
	if err := decoder.Decode(&value); err != nil {
		return SchedulerConfiguration{}, ErrSchedulerConfiguration
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF || !validSchedulerConfiguration(value) {
		return SchedulerConfiguration{}, ErrSchedulerConfiguration
	}
	return value, nil
}

func rejectDuplicateSchedulerConfigurationJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var scan func() error
	scan = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				key, keyErr := decoder.Token()
				if keyErr != nil {
					return keyErr
				}
				name, ok := key.(string)
				if !ok {
					return ErrSchedulerConfiguration
				}
				if _, exists := seen[name]; exists {
					return ErrSchedulerConfiguration
				}
				seen[name] = struct{}{}
				if err := scan(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := scan(); err != nil {
					return err
				}
			}
		}
		_, err = decoder.Token()
		return err
	}
	if err := scan(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return ErrSchedulerConfiguration
	}
	return nil
}

func validSchedulerConfiguration(value SchedulerConfiguration) bool {
	if value.Schema != SchedulerConfigurationSchemaV1 || !validSchedulerConfigKeyID(value.KeyID) || value.IssuedAt == "" || value.ExpiresAt == "" || value.Signature == "" || len(value.Signature) > 256 || value.Version == 0 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(value.Signature)
	return err == nil
}

func validSchedulerConfigKeyID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}
