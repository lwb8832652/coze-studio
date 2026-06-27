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
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestGuardrailAuditObjectStorageArchiveWriterWritesJSONArchive(
	t *testing.T,
) {
	objectStorage := &recordingGuardrailAuditArchiveStorage{}
	writer := NewGuardrailAuditObjectStorageArchiveWriter(
		GuardrailAuditObjectStorageArchiveWriterOptions{
			Storage: objectStorage,
			Prefix:  "guardrail/audit/archive",
		},
	)
	payload := GuardrailAuditArchivePayload{
		Schema:          GuardrailAuditArchiveSchema,
		ExportedAt:      2000,
		CutoffCreatedAt: 1500,
		Total:           1,
		Events: []*GuardrailAuditEventSummary{
			{
				EventID:    9101,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    30,
				ActorID:    40,
				EventType:  "guardrail.decision.deny",
				TargetType: "tool_call",
				TargetID:   "runtime_tool:search_docs",
				Operation:  "invoke",
				Source:     "adk_runtime_tool",
				Action:     "deny",
				FailMode:   "fail_closed",
				Provider:   "scanner",
				ReasonCode: "high_risk",
				RuleIDs:    "rule:high_risk",
				CreatedAt:  1000,
			},
		},
	}

	result, err := writer.WriteGuardrailAuditArchive(
		context.Background(),
		payload,
	)

	require.NoError(t, err)
	require.NotEmpty(t, result.ArchiveID)
	require.LessOrEqual(t, len(result.ArchiveID), 64)
	require.NotContains(t, result.ArchiveID, "/")
	require.NotContains(t, result.ArchiveID, "s3://")
	require.Equal(t, 1, objectStorage.calls)
	require.Equal(t, "guardrail/audit/archive/1500/"+result.ArchiveID+".json", objectStorage.key)
	require.NotContains(t, objectStorage.key, "s3://")
	require.NotNil(t, objectStorage.option.ContentType)
	require.Equal(
		t,
		"application/json; charset=utf-8",
		*objectStorage.option.ContentType,
	)
	require.Equal(t, int64(len(objectStorage.content)), objectStorage.option.ObjectSize)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(objectStorage.content, &decoded))
	require.Equal(t, GuardrailAuditArchiveSchema, decoded["schema"])
	require.Equal(t, float64(2000), decoded["exported_at"])
	require.Equal(t, float64(1500), decoded["cutoff_created_at"])
	require.Equal(t, float64(1), decoded["total"])
	events, ok := decoded["events"].([]any)
	require.True(t, ok)
	require.Len(t, events, 1)
	event, ok := events[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(9101), event["event_id"])
	require.Equal(t, "runtime_tool:search_docs", event["target_id"])
	require.Equal(t, "high_risk", event["reason_code"])

	serialized := string(objectStorage.content)
	require.Contains(t, serialized, `"event_id"`)
	require.Contains(t, serialized, `"cutoff_created_at"`)
	require.NotContains(t, serialized, "secret prompt")
	require.NotContains(t, serialized, "tool_args")
	require.NotContains(t, serialized, "s3://")
	require.NotContains(t, serialized, "checkpoint")
	require.NotContains(t, serialized, "provider_raw")
}

func TestGuardrailAuditObjectStorageArchiveWriterSanitizesStorageErrors(
	t *testing.T,
) {
	objectStorage := &recordingGuardrailAuditArchiveStorage{
		err: errors.New("put s3://bucket/raw/archive with sk-secret prompt"),
	}
	writer := NewGuardrailAuditObjectStorageArchiveWriter(
		GuardrailAuditObjectStorageArchiveWriterOptions{
			Storage: objectStorage,
		},
	)

	result, err := writer.WriteGuardrailAuditArchive(
		context.Background(),
		validGuardrailAuditArchivePayloadForStorageTest(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "guardrail audit archive write failed")
	require.NotContains(t, err.Error(), "s3://")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "prompt")
}

func TestGuardrailAuditObjectStorageArchiveWriterRejectsInvalidPayload(
	t *testing.T,
) {
	objectStorage := &recordingGuardrailAuditArchiveStorage{}
	writer := NewGuardrailAuditObjectStorageArchiveWriter(
		GuardrailAuditObjectStorageArchiveWriterOptions{
			Storage: objectStorage,
		},
	)

	invalid := validGuardrailAuditArchivePayloadForStorageTest()
	invalid.Schema = ""
	result, err := writer.WriteGuardrailAuditArchive(
		context.Background(),
		invalid,
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Equal(t, 0, objectStorage.calls)
	require.Contains(t, err.Error(), "guardrail audit archive write failed")
}

func TestGuardrailAuditObjectStorageArchiveWriterRequiresStorage(t *testing.T) {
	writer := NewGuardrailAuditObjectStorageArchiveWriter(
		GuardrailAuditObjectStorageArchiveWriterOptions{},
	)

	result, err := writer.WriteGuardrailAuditArchive(
		context.Background(),
		validGuardrailAuditArchivePayloadForStorageTest(),
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "guardrail audit archive write failed")
}

func validGuardrailAuditArchivePayloadForStorageTest() GuardrailAuditArchivePayload {
	return GuardrailAuditArchivePayload{
		Schema:          GuardrailAuditArchiveSchema,
		ExportedAt:      2000,
		CutoffCreatedAt: 1500,
		Total:           1,
		Events: []*GuardrailAuditEventSummary{
			{
				EventID:    9101,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    30,
				ActorID:    40,
				EventType:  "guardrail.decision.deny",
				TargetType: "tool_call",
				TargetID:   "runtime_tool:search_docs",
				Operation:  "invoke",
				Source:     "adk_runtime_tool",
				Action:     "deny",
				FailMode:   "fail_closed",
				Provider:   "scanner",
				ReasonCode: "high_risk",
				RuleIDs:    "rule:high_risk",
				CreatedAt:  1000,
			},
		},
	}
}

type recordingGuardrailAuditArchiveStorage struct {
	key     string
	content []byte
	option  storage.PutOption
	err     error
	calls   int
}

func (s *recordingGuardrailAuditArchiveStorage) PutObject(
	_ context.Context,
	key string,
	content []byte,
	opts ...storage.PutOptFn,
) error {
	s.calls++
	s.key = key
	s.content = append([]byte(nil), content...)
	for _, opt := range opts {
		if opt != nil {
			opt(&s.option)
		}
	}
	return s.err
}
