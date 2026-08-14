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

package agentthread

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	legacyRequestedPolicyKey = "requested_policy"
	legacyModeKey            = "mode"
)

type LegacyAdaptiveAdmissionDecoder struct{}

func NewLegacyAdaptiveAdmissionDecoder() *LegacyAdaptiveAdmissionDecoder {
	return &LegacyAdaptiveAdmissionDecoder{}
}

func (*LegacyAdaptiveAdmissionDecoder) Decode(source *entity.Run) (entity.AdaptiveAdmissionSnapshot, error) {
	if source == nil || source.ID <= 0 || source.ExecutionGeneration == 0 {
		return entity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("legacy adaptive admission source identity is invalid")
	}
	config := []byte(source.Config)
	if !utf8.Valid(config) {
		return entity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("legacy adaptive admission config is not valid UTF-8")
	}
	requestedPolicy, mode, err := decodeLegacyAdaptiveControls(config)
	if err != nil {
		return entity.AdaptiveAdmissionSnapshot{}, err
	}
	if requestedPolicy != "" && mode != "" && legacyRequestedPolicyMode(requestedPolicy) != mode {
		return entity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("legacy adaptive admission controls conflict")
	}

	digest := sha256.Sum256(config)
	sourceRunID, sourceGeneration := source.ID, source.ExecutionGeneration
	snapshot := entity.AdaptiveAdmissionSnapshot{
		Schema:                    entity.AdaptiveAdmissionSchemaV1,
		FeatureGateEnabled:        false,
		Source:                    entity.AdaptiveAdmissionSourceLegacyDecoder,
		SourceRunID:               &sourceRunID,
		SourceExecutionGeneration: &sourceGeneration,
		SourceConfigDigest:        hex.EncodeToString(digest[:]),
		DecoderVersion:            entity.AdaptiveLegacyDecoderVersionV1,
		Capabilities: entity.AdaptiveCapabilities{
			PlanAllowed:             true,
			HumanInteractionAllowed: true,
		},
		Limits: entity.AdaptiveLimits{
			MaxToolCalls:             24,
			MaxReplans:               2,
			MaxVerificationRepairs:   2,
			MaxConsecutiveNoProgress: 3,
			MaxActiveDurationSeconds: 1200,
		},
	}
	if err := ValidateAdaptiveAdmissionSnapshot(snapshot); err != nil {
		return entity.AdaptiveAdmissionSnapshot{}, fmt.Errorf("validate decoded legacy adaptive admission: %w", err)
	}
	return snapshot, nil
}

func decodeLegacyAdaptiveControls(config []byte) (requestedPolicy string, mode string, err error) {
	decoder := json.NewDecoder(bytes.NewReader(config))
	decoder.UseNumber()
	opening, err := decoder.Token()
	if err != nil {
		return "", "", fmt.Errorf("decode legacy adaptive admission config: %w", err)
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return "", "", fmt.Errorf("legacy adaptive admission config must be a JSON object")
	}

	rootKeys := make([]string, 0, 8)
	for decoder.More() {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return "", "", fmt.Errorf("decode legacy adaptive admission root key: %w", tokenErr)
		}
		key, ok := token.(string)
		if !ok {
			return "", "", fmt.Errorf("legacy adaptive admission root key must be a string")
		}
		for _, seen := range rootKeys {
			if strings.EqualFold(seen, key) {
				return "", "", fmt.Errorf("legacy adaptive admission config has ambiguous root keys")
			}
		}
		rootKeys = append(rootKeys, key)

		var raw json.RawMessage
		if decodeErr := decoder.Decode(&raw); decodeErr != nil {
			return "", "", fmt.Errorf("decode legacy adaptive admission value: %w", decodeErr)
		}
		switch {
		case key == legacyRequestedPolicyKey:
			requestedPolicy, err = decodeLegacyAdaptiveString(raw, legacyRequestedPolicyKey, "auto", "pro", "ultra")
		case key == legacyModeKey:
			mode, err = decodeLegacyAdaptiveString(raw, legacyModeKey, "pro", "ultra")
		case strings.EqualFold(key, legacyRequestedPolicyKey) || strings.EqualFold(key, legacyModeKey):
			err = fmt.Errorf("legacy adaptive admission control key has ambiguous casing")
		}
		if err != nil {
			return "", "", err
		}
	}
	if _, closeErr := decoder.Token(); closeErr != nil {
		return "", "", fmt.Errorf("decode legacy adaptive admission object: %w", closeErr)
	}
	var trailing any
	if trailingErr := decoder.Decode(&trailing); trailingErr != io.EOF {
		if trailingErr == nil {
			return "", "", fmt.Errorf("legacy adaptive admission config has trailing JSON")
		}
		return "", "", fmt.Errorf("decode legacy adaptive admission trailing JSON: %w", trailingErr)
	}
	if requestedPolicy == "" && mode == "" {
		return "", "", fmt.Errorf("legacy adaptive admission control is required")
	}
	return requestedPolicy, mode, nil
}

func decodeLegacyAdaptiveString(raw json.RawMessage, key string, accepted ...string) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("legacy adaptive admission %s must be a string", key)
	}
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf("legacy adaptive admission %s is empty or changes when trimmed", key)
	}
	for _, candidate := range accepted {
		if value == candidate {
			return value, nil
		}
	}
	return "", fmt.Errorf("legacy adaptive admission %s is unsupported", key)
}

func legacyRequestedPolicyMode(requestedPolicy string) string {
	if requestedPolicy == "pro" {
		return "pro"
	}
	return "ultra"
}
