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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestGuardrailAuditArchiveExporterWritesMetadataOnlyPayload(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{
		listBeforeEvents: []*domainentity.GuardrailAuditEvent{
			{
				ID:         9101,
				SpaceID:    30,
				ThreadID:   10,
				RunID:      20,
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
		listBeforeTotal: 3,
	}
	writer := &recordingGuardrailAuditArchiveWriter{
		result: GuardrailAuditArchiveWriteResult{ArchiveID: "archive_20260627_01"},
	}
	exporter := NewGuardrailAuditArchiveExporter(
		GuardrailAuditArchiveExporterOptions{
			Repository: repo,
			Writer:     writer,
			BatchSize:  7,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := exporter.ArchiveExpiredGuardrailAuditEvents(
		context.Background(),
		1500,
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailAuditArchiveExportResult{
		CutoffCreatedAt: 1500,
		Archived:        1,
		ArchivedEventIDs: []int64{
			9101,
		},
		Total:     3,
		ArchiveID: "archive_20260627_01",
	}, result)
	require.Equal(t, 1, repo.listBeforeCalls)
	require.Equal(t, int64(1500), repo.listBeforeReq.CutoffCreatedAt)
	require.Equal(t, int32(7), repo.listBeforeReq.Limit)
	require.Equal(t, 1, writer.calls)
	require.Equal(t, GuardrailAuditArchiveSchema, writer.payload.Schema)
	require.Equal(t, int64(2000), writer.payload.ExportedAt)
	require.Equal(t, int64(1500), writer.payload.CutoffCreatedAt)
	require.Equal(t, int64(3), writer.payload.Total)
	require.Len(t, writer.payload.Events, 1)
	require.Equal(t, int64(9101), writer.payload.Events[0].EventID)
	require.Equal(t, "runtime_tool:search_docs", writer.payload.Events[0].TargetID)

	serialized := strings.Join([]string{
		writer.payload.Schema,
		writer.payload.Events[0].EventType,
		writer.payload.Events[0].TargetType,
		writer.payload.Events[0].TargetID,
		writer.payload.Events[0].Operation,
		writer.payload.Events[0].Source,
		writer.payload.Events[0].Action,
		writer.payload.Events[0].FailMode,
		writer.payload.Events[0].Provider,
		writer.payload.Events[0].ReasonCode,
		writer.payload.Events[0].RuleIDs,
	}, " ")
	require.NotContains(t, serialized, "secret prompt")
	require.NotContains(t, serialized, "tool_args")
	require.NotContains(t, serialized, "s3://")
	require.NotContains(t, serialized, "checkpoint")
	require.NotContains(t, serialized, "provider_raw")
}

func TestGuardrailAuditArchiveExporterSkipsWriterWhenNoExpiredEvents(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{}
	writer := &recordingGuardrailAuditArchiveWriter{}
	exporter := NewGuardrailAuditArchiveExporter(
		GuardrailAuditArchiveExporterOptions{
			Repository: repo,
			Writer:     writer,
			BatchSize:  7,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := exporter.ArchiveExpiredGuardrailAuditEvents(
		context.Background(),
		1500,
	)

	require.NoError(t, err)
	require.Equal(t, GuardrailAuditArchiveExportResult{
		CutoffCreatedAt: 1500,
	}, result)
	require.Equal(t, 1, repo.listBeforeCalls)
	require.Equal(t, 0, writer.calls)
}

func TestGuardrailAuditArchiveExporterSanitizesWriterErrors(t *testing.T) {
	repo := &recordingGuardrailAuditRepository{
		listBeforeEvents: []*domainentity.GuardrailAuditEvent{
			{
				ID:        9101,
				ThreadID:  10,
				RunID:     20,
				SpaceID:   30,
				ActorID:   40,
				EventType: "guardrail.decision.deny",
				CreatedAt: 1000,
			},
		},
		listBeforeTotal: 1,
	}
	writer := &recordingGuardrailAuditArchiveWriter{
		err: errors.New("write s3://bucket/raw/archive with sk-secret prompt"),
	}
	exporter := NewGuardrailAuditArchiveExporter(
		GuardrailAuditArchiveExporterOptions{
			Repository: repo,
			Writer:     writer,
			NowMillis:  func() int64 { return 2000 },
		},
	)

	result, err := exporter.ArchiveExpiredGuardrailAuditEvents(
		context.Background(),
		1500,
	)

	require.Error(t, err)
	require.Empty(t, result)
	require.Contains(t, err.Error(), "guardrail audit archive export failed")
	require.NotContains(t, err.Error(), "s3://")
	require.NotContains(t, err.Error(), "sk-secret")
	require.NotContains(t, err.Error(), "prompt")
}

type recordingGuardrailAuditArchiveWriter struct {
	payload GuardrailAuditArchivePayload
	result  GuardrailAuditArchiveWriteResult
	err     error
	calls   int
}

func (w *recordingGuardrailAuditArchiveWriter) WriteGuardrailAuditArchive(
	ctx context.Context,
	payload GuardrailAuditArchivePayload,
) (GuardrailAuditArchiveWriteResult, error) {
	w.calls++
	w.payload = payload
	if w.err != nil {
		return GuardrailAuditArchiveWriteResult{}, w.err
	}

	return w.result, nil
}
