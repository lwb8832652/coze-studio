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
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestLegacyAdaptiveAdmissionDecoderMapsKnownValuesToConservativeGateOff(t *testing.T) {
	for _, test := range []struct {
		name   string
		config string
	}{
		{name: "requested policy auto", config: `{"runtime":"eino_adk", "requested_policy":"auto"}`},
		{name: "requested policy pro", config: `{"requested_policy":"pro"}`},
		{name: "requested policy ultra", config: `{"requested_policy":"ultra"}`},
		{name: "mode pro", config: `{"mode":"pro"}`},
		{name: "mode ultra", config: `{"mode":"ultra"}`},
		{name: "matching pro", config: `{"requested_policy":"pro","mode":"pro"}`},
		{name: "auto resolves to ultra", config: `{"requested_policy":"auto","mode":"ultra"}`},
		{name: "matching ultra", config: `{"requested_policy":"ultra","mode":"ultra"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := &entity.Run{ID: 41, ExecutionGeneration: 7, Config: test.config}
			originalConfig := source.Config
			digest := sha256.Sum256([]byte(test.config))

			snapshot, err := NewLegacyAdaptiveAdmissionDecoder().Decode(source)

			require.NoError(t, err)
			require.Equal(t, entity.AdaptiveAdmissionSnapshot{
				Schema:                    entity.AdaptiveAdmissionSchemaV1,
				FeatureGateEnabled:        false,
				Source:                    entity.AdaptiveAdmissionSourceLegacyDecoder,
				SourceRunID:               legacyDecoderInt64Pointer(41),
				SourceExecutionGeneration: legacyDecoderUint64Pointer(7),
				SourceConfigDigest:        hex.EncodeToString(digest[:]),
				DecoderVersion:            entity.AdaptiveLegacyDecoderVersionV1,
				Capabilities: entity.AdaptiveCapabilities{
					PlanAllowed:             true,
					ReadOnlyToolsAllowed:    false,
					SandboxWritesAllowed:    false,
					HumanInteractionAllowed: true,
					SubagentsAllowed:        false,
				},
				Limits: entity.AdaptiveLimits{
					MaxToolCalls:             24,
					MaxReplans:               2,
					MaxVerificationRepairs:   2,
					MaxConsecutiveNoProgress: 3,
					MaxActiveDurationSeconds: 1200,
				},
			}, snapshot)
			require.NoError(t, ValidateAdaptiveAdmissionSnapshot(snapshot))
			require.Equal(t, originalConfig, source.Config)
		})
	}
}

func TestLegacyAdaptiveAdmissionDecoderRejectsAmbiguousOrInvalidLegacyConfig(t *testing.T) {
	invalidUTF8 := string([]byte{'{', '"', 'm', 'o', 'd', 'e', '"', ':', '"', 0xff, '"', '}'})
	tests := []struct {
		name   string
		source *entity.Run
	}{
		{name: "nil source"},
		{name: "zero source run", source: &entity.Run{ExecutionGeneration: 7, Config: `{"mode":"pro"}`}},
		{name: "zero generation", source: &entity.Run{ID: 41, Config: `{"mode":"pro"}`}},
		{name: "empty config", source: legacyDecoderSource("")},
		{name: "whitespace config", source: legacyDecoderSource(" \n\t")},
		{name: "invalid utf8", source: legacyDecoderSource(invalidUTF8)},
		{name: "malformed json", source: legacyDecoderSource(`{"mode":`)},
		{name: "scalar", source: legacyDecoderSource(`"pro"`)},
		{name: "array", source: legacyDecoderSource(`["pro"]`)},
		{name: "null object", source: legacyDecoderSource(`null`)},
		{name: "empty object", source: legacyDecoderSource(`{}`)},
		{name: "trailing value", source: legacyDecoderSource(`{"mode":"pro"} {}`)},
		{name: "duplicate root key", source: legacyDecoderSource(`{"mode":"pro","mode":"pro"}`)},
		{name: "casefold duplicate root key", source: legacyDecoderSource(`{"mode":"pro","MODE":"pro"}`)},
		{name: "casefold legacy key", source: legacyDecoderSource(`{"Requested_Policy":"pro"}`)},
		{name: "requested policy null", source: legacyDecoderSource(`{"requested_policy":null}`)},
		{name: "requested policy bool", source: legacyDecoderSource(`{"requested_policy":true}`)},
		{name: "requested policy number", source: legacyDecoderSource(`{"requested_policy":1}`)},
		{name: "requested policy object", source: legacyDecoderSource(`{"requested_policy":{}}`)},
		{name: "requested policy array", source: legacyDecoderSource(`{"requested_policy":[]}`)},
		{name: "requested policy empty", source: legacyDecoderSource(`{"requested_policy":""}`)},
		{name: "requested policy whitespace", source: legacyDecoderSource(`{"requested_policy":" "}`)},
		{name: "requested policy trim change", source: legacyDecoderSource(`{"requested_policy":" pro"}`)},
		{name: "requested policy unknown", source: legacyDecoderSource(`{"requested_policy":"standard"}`)},
		{name: "requested policy case change", source: legacyDecoderSource(`{"requested_policy":"PRO"}`)},
		{name: "mode null", source: legacyDecoderSource(`{"mode":null}`)},
		{name: "mode empty", source: legacyDecoderSource(`{"mode":""}`)},
		{name: "mode trim change", source: legacyDecoderSource(`{"mode":"pro "}`)},
		{name: "mode auto unsupported", source: legacyDecoderSource(`{"mode":"auto"}`)},
		{name: "mode unknown", source: legacyDecoderSource(`{"mode":"standard"}`)},
		{name: "pro conflicts with ultra", source: legacyDecoderSource(`{"requested_policy":"pro","mode":"ultra"}`)},
		{name: "ultra conflicts with pro", source: legacyDecoderSource(`{"requested_policy":"ultra","mode":"pro"}`)},
		{name: "auto conflicts with pro", source: legacyDecoderSource(`{"requested_policy":"auto","mode":"pro"}`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewLegacyAdaptiveAdmissionDecoder().Decode(test.source)
			require.Error(t, err)
		})
	}
}

func legacyDecoderSource(config string) *entity.Run {
	return &entity.Run{ID: 41, ExecutionGeneration: 7, Config: config}
}

func legacyDecoderInt64Pointer(value int64) *int64 { return &value }

func legacyDecoderUint64Pointer(value uint64) *uint64 { return &value }
