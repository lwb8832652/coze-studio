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

package coze

import (
	"encoding/json"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestCanonicalProductProjectionUsesStringIDsAndRFC3339Times(t *testing.T) {
	collectionOrder := int32(2)
	artifact, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{
		ArtifactID:       9001,
		ThreadID:         8001,
		RunID:            7001,
		FileID:           6001,
		Title:            "public report",
		Source:           "tool_output",
		GenerationStatus: "ready",
		Capabilities:     []string{"open", "preview", "download", "copy"},
		IsPrimary:        true,
		CollectionID:     "collection-safe",
		CollectionOrder:  &collectionOrder,
		Metadata:         `{"visible":"yes"}`,
		CreatedAt:        1710000000123,
		UpdatedAt:        1710000001123,
	})
	require.NoError(t, err)
	require.Equal(t, "9001", artifact.ArtifactID)
	require.Equal(t, "8001", artifact.ThreadID)
	require.Equal(t, "7001", artifact.RunID)
	require.Equal(t, "6001", artifact.FileID)
	require.Equal(t, "tool_output", artifact.Source)
	require.Equal(t, "ready", artifact.GenerationStatus)
	require.Equal(t, []string{"open", "preview", "download", "copy"}, artifact.Capabilities)
	require.True(t, artifact.IsPrimary)
	require.Equal(t, "collection-safe", artifact.CollectionID)
	require.NotNil(t, artifact.CollectionOrder)
	require.Equal(t, int32(2), *artifact.CollectionOrder)
	for _, value := range []string{artifact.CreatedAt, artifact.UpdatedAt} {
		_, err := time.Parse(time.RFC3339Nano, value)
		require.NoError(t, err)
	}

	encoded := canonicalProjectionJSON(t, artifact)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal([]byte(encoded), &decoded))
	require.IsType(t, "", decoded["artifact_id"])
	require.IsType(t, map[string]any{}, decoded["metadata"])
}

func TestCanonicalProductProjectionRemovesRawUsageSecretsAndWorkerIdentity(t *testing.T) {
	sensitive := canonicalProductSensitiveMetadata()
	artifact, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{
		ArtifactID: 9001, ThreadID: 8001, RunID: 7001, FileID: 6001,
		Title: "public artifact", Metadata: sensitive, CreatedAt: 1710000000123, UpdatedAt: 1710000001123,
	})
	require.NoError(t, err)
	usage, err := projectCanonicalProductTokenUsage(&appagentthread.TokenUsageSummary{
		UsageID: 5001, ThreadID: 8001, RunID: 7001, Source: appagentthread.TokenUsageSourceLeadAgent,
		RawUsage: sensitive, Metadata: sensitive, CreatedAt: 1710000000123,
	})
	require.NoError(t, err)
	scan, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{
		JobID: 4001, ThreadID: 8001, RunID: 7001, ArtifactID: 9001, FileID: 6001,
		WorkerID: "worker-identity-secret", LastError: "provider body secret", Status: appagentthread.ArtifactScanJobStatusFailed,
		CreatedAt: 1710000000123, UpdatedAt: 1710000001123,
	})
	require.NoError(t, err)
	mem, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{
		MemoryID: 3001, ThreadID: 8001, RunID: 7001, Content: "approved memory", Metadata: sensitive, CreatedAt: 1710000000123, UpdatedAt: 1710000001123,
	})
	require.NoError(t, err)
	guardrail, err := projectCanonicalProductGuardrailAudit(&appagentthread.GuardrailAuditEventSummary{
		EventID: 2001, ThreadID: 8001, RunID: 7001, RuleIDs: `["rule-a", "authorization", "rule-b"]`, CreatedAt: 1710000000123,
	})
	require.NoError(t, err)
	mcp, err := projectCanonicalProductMCPRuntimeAudit(&appagentthread.MCPRuntimeAuditEventSummary{
		EventID: 1001, ThreadID: 8001, RunID: 7001, ServerID: 9001, RuntimeToolName: "approved_tool", CreatedAt: 1710000000123,
	})
	require.NoError(t, err)

	require.NotEmpty(t, scan.WorkerRef)
	require.NotEqual(t, "worker-identity-secret", scan.WorkerRef)
	require.Equal(t, "scan_failed", scan.ErrorCode)
	require.Equal(t, []string{"rule-a", "authorization", "rule-b"}, guardrail.RuleIDs)
	encoded := canonicalProjectionJSON(t, []any{artifact, usage, scan, mem, guardrail, mcp})
	for _, sensitive := range canonicalProductSensitiveSentinels() {
		require.NotContains(t, encoded, sensitive)
	}
	for _, field := range []string{"raw_usage", "worker_id", "lease_token", "signed_url"} {
		require.NotContains(t, encoded, field)
	}
}

func TestCanonicalProductProjectionMatchesIDLWireShapes(t *testing.T) {
	const createdAt = int64(1710000000123)
	const updatedAt = int64(1710000001123)

	upload, err := projectCanonicalProductUpload(&appagentthread.TaskThreadUploadedFileSummary{FileID: 1, CreatedAt: createdAt})
	require.NoError(t, err)
	requireCanonicalProductWireFields(t, upload, []string{"file_id", "file_name", "virtual_path", "content_type", "size_bytes", "created_at"})
	requireCanonicalProductRFC3339(t, upload.CreatedAt)

	artifact, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{ArtifactID: 1, ThreadID: 2, RunID: 3, FileID: 4, CreatedAt: createdAt, UpdatedAt: updatedAt})
	require.NoError(t, err)
	requireCanonicalProductWireFields(t, artifact, []string{"artifact_id", "thread_id", "run_id", "file_id", "title", "artifact_type", "virtual_path", "content_type", "size_bytes", "preview_mode", "metadata", "created_at", "updated_at"})
	requireCanonicalProductRFC3339(t, artifact.CreatedAt, artifact.UpdatedAt)

	scan, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: createdAt, UpdatedAt: updatedAt})
	require.NoError(t, err)
	scanFields := requireCanonicalProductWireFields(t, scan, []string{"job_id", "thread_id", "run_id", "artifact_id", "file_id", "scanner", "status", "worker_ref", "attempt_count", "error_code", "created_at", "updated_at"})
	require.NotContains(t, scanFields, "lease_expires_at")
	require.Equal(t, "none", scanFields["worker_ref"])
	require.Equal(t, "none", scanFields["error_code"])
	requireCanonicalProductRFC3339(t, scan.CreatedAt, scan.UpdatedAt)

	usage, err := projectCanonicalProductTokenUsage(&appagentthread.TokenUsageSummary{UsageID: 1, ThreadID: 2, RunID: 3, CreatedAt: createdAt})
	require.NoError(t, err)
	requireCanonicalProductWireFields(t, usage, []string{"usage_id", "thread_id", "run_id", "source", "step_id", "step_index", "step_name", "model_name", "provider", "input_tokens", "output_tokens", "total_tokens", "cost_micros", "currency", "estimated", "created_at"})
	requireCanonicalProductRFC3339(t, usage.CreatedAt)

	aggregate := projectCanonicalProductTokenUsageAggregate(nil)
	requireCanonicalProductWireFields(t, aggregate, []string{"input_tokens", "output_tokens", "total_tokens", "cost_micros", "call_count", "lead_agent_tokens", "subagent_tokens", "middleware_tokens", "tool_tokens"})
	runAggregate, err := projectCanonicalProductRunTokenUsageAggregate(&appagentthread.RunTokenUsageAggregateSummary{RunID: 3})
	require.NoError(t, err)
	requireCanonicalProductWireFields(t, runAggregate, []string{"run_id", "aggregate"})

	memory, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{MemoryID: 1, ThreadID: 2, SourceID: "snapshot:501:language", CreatedAt: createdAt, UpdatedAt: updatedAt})
	require.NoError(t, err)
	memoryFields := requireCanonicalProductWireFields(t, memory, []string{"memory_id", "thread_id", "scope", "content", "metadata", "score", "confidence", "source_type", "source_id", "created_at", "updated_at"})
	require.Equal(t, "snapshot:501:language", memoryFields["source_id"])
	requireCanonicalProductRFC3339(t, memory.CreatedAt, memory.UpdatedAt)

	memoryAudit, err := projectCanonicalProductMemoryAudit(&appagentthread.MemoryAuditEventSummary{EventID: 1, ThreadID: 2, ActorID: 7, SourceID: "snapshot:501:language", CreatedAt: createdAt})
	require.NoError(t, err)
	memoryAuditFields := requireCanonicalProductWireFields(t, memoryAudit, []string{"event_id", "thread_id", "actor_id", "event_type", "scope", "source_type", "source_id", "affected_count", "created_at"})
	require.Equal(t, "7", memoryAuditFields["actor_id"])
	require.Equal(t, "snapshot:501:language", memoryAuditFields["source_id"])
	requireCanonicalProductRFC3339(t, memoryAudit.CreatedAt)

	guardrail, err := projectCanonicalProductGuardrailAudit(&appagentthread.GuardrailAuditEventSummary{EventID: 1, ThreadID: 2, ActorID: 7, TargetID: "runtime_tool:search_docs", RuleIDs: `["authorization","https://storage.example.test/signed-url-secret","sk-secret-value"]`, CreatedAt: createdAt})
	require.NoError(t, err)
	guardrailFields := requireCanonicalProductWireFields(t, guardrail, []string{"event_id", "thread_id", "actor_id", "event_type", "target_type", "target_id", "operation", "source", "action", "fail_mode", "provider", "reason_code", "rule_ids", "created_at"})
	require.Equal(t, "7", guardrailFields["actor_id"])
	require.Equal(t, "runtime_tool:search_docs", guardrailFields["target_id"])
	require.Equal(t, []any{"authorization"}, guardrailFields["rule_ids"])
	requireCanonicalProductRFC3339(t, guardrail.CreatedAt)

	mcp, err := projectCanonicalProductMCPRuntimeAudit(&appagentthread.MCPRuntimeAuditEventSummary{EventID: 1, ThreadID: 2, CreatedAt: createdAt})
	require.NoError(t, err)
	requireCanonicalProductWireFields(t, mcp, []string{"event_id", "thread_id", "runtime_tool_name", "event_type", "error_code", "elapsed_millis", "output_bytes", "created_at"})
	requireCanonicalProductRFC3339(t, mcp.CreatedAt)
}

func TestCanonicalProductProjectionRejectsMissingOrInvalidRequiredTimes(t *testing.T) {
	const validTime = int64(1710000000123)
	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "upload zero", call: func() error {
			_, err := projectCanonicalProductUpload(&appagentthread.TaskThreadUploadedFileSummary{FileID: 1})
			return err
		}},
		{name: "artifact updated zero", call: func() error {
			_, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{ArtifactID: 1, ThreadID: 2, RunID: 3, FileID: 4, CreatedAt: validTime})
			return err
		}},
		{name: "scan created negative", call: func() error {
			_, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: -1, UpdatedAt: validTime})
			return err
		}},
		{name: "token usage zero", call: func() error {
			_, err := projectCanonicalProductTokenUsage(&appagentthread.TokenUsageSummary{UsageID: 1, ThreadID: 2, RunID: 3})
			return err
		}},
		{name: "memory updated zero", call: func() error {
			_, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{MemoryID: 1, ThreadID: 2, CreatedAt: validTime})
			return err
		}},
		{name: "memory audit zero", call: func() error {
			_, err := projectCanonicalProductMemoryAudit(&appagentthread.MemoryAuditEventSummary{EventID: 1, ThreadID: 2})
			return err
		}},
		{name: "guardrail audit zero", call: func() error {
			_, err := projectCanonicalProductGuardrailAudit(&appagentthread.GuardrailAuditEventSummary{EventID: 1, ThreadID: 2})
			return err
		}},
		{name: "mcp audit zero", call: func() error {
			_, err := projectCanonicalProductMCPRuntimeAudit(&appagentthread.MCPRuntimeAuditEventSummary{EventID: 1, ThreadID: 2})
			return err
		}},
		{name: "overflowing rfc3339", call: func() error {
			_, err := projectCanonicalProductUpload(&appagentthread.TaskThreadUploadedFileSummary{FileID: 1, CreatedAt: math.MaxInt64})
			return err
		}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.call())
		})
	}
}

func TestCanonicalProductProjectionOptionalTimesFailClosed(t *testing.T) {
	const validTime = int64(1710000000123)

	artifact, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{
		ArtifactID: 1, ThreadID: 2, RunID: 3, FileID: 4, CreatedAt: validTime, UpdatedAt: validTime,
	})
	require.NoError(t, err)
	require.Empty(t, artifact.DeletedAt)
	artifact, err = projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{
		ArtifactID: 1, ThreadID: 2, RunID: 3, FileID: 4, CreatedAt: validTime, UpdatedAt: validTime, DeletedAt: validTime,
	})
	require.NoError(t, err)
	requireCanonicalProductRFC3339(t, artifact.DeletedAt)

	scan, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{
		JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: validTime, UpdatedAt: validTime,
	})
	require.NoError(t, err)
	require.Empty(t, scan.AvailableAt)
	require.Empty(t, scan.StartedAt)
	require.Empty(t, scan.EndedAt)
	scan, err = projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{
		JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: validTime, UpdatedAt: validTime,
		AvailableAt: validTime, StartedAt: validTime, EndedAt: validTime,
	})
	require.NoError(t, err)
	requireCanonicalProductRFC3339(t, scan.AvailableAt, scan.StartedAt, scan.EndedAt)

	memory, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{
		MemoryID: 1, ThreadID: 2, CreatedAt: validTime, UpdatedAt: validTime,
	})
	require.NoError(t, err)
	require.Empty(t, memory.CorrectedAt)
	require.Empty(t, memory.ExpiresAt)
	require.Empty(t, memory.DeletedAt)
	memory, err = projectCanonicalProductMemory(&appagentthread.MemorySummary{
		MemoryID: 1, ThreadID: 2, CreatedAt: validTime, UpdatedAt: validTime,
		CorrectedAt: validTime, ExpiresAt: validTime, DeletedAt: validTime,
	})
	require.NoError(t, err)
	requireCanonicalProductRFC3339(t, memory.CorrectedAt, memory.ExpiresAt, memory.DeletedAt)

	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "artifact deleted overflowing", call: func() error {
			_, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{ArtifactID: 1, ThreadID: 2, RunID: 3, FileID: 4, CreatedAt: validTime, UpdatedAt: validTime, DeletedAt: math.MaxInt64})
			return err
		}},
		{name: "scan available negative", call: func() error {
			_, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: validTime, UpdatedAt: validTime, AvailableAt: -1})
			return err
		}},
		{name: "scan started overflowing", call: func() error {
			_, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: validTime, UpdatedAt: validTime, StartedAt: math.MaxInt64})
			return err
		}},
		{name: "scan ended negative", call: func() error {
			_, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4, FileID: 5, CreatedAt: validTime, UpdatedAt: validTime, EndedAt: -1})
			return err
		}},
		{name: "memory corrected overflowing", call: func() error {
			_, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{MemoryID: 1, ThreadID: 2, CreatedAt: validTime, UpdatedAt: validTime, CorrectedAt: math.MaxInt64})
			return err
		}},
		{name: "memory expires negative", call: func() error {
			_, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{MemoryID: 1, ThreadID: 2, CreatedAt: validTime, UpdatedAt: validTime, ExpiresAt: -1})
			return err
		}},
		{name: "memory deleted overflowing", call: func() error {
			_, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{MemoryID: 1, ThreadID: 2, CreatedAt: validTime, UpdatedAt: validTime, DeletedAt: math.MaxInt64})
			return err
		}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.call())
		})
	}
}

func TestCanonicalProductProjectionRejectsInvalidRequiredIDs(t *testing.T) {
	for _, test := range []struct {
		name string
		call func() error
	}{
		{name: "artifact thread", call: func() error {
			_, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{ArtifactID: 1, RunID: 2, FileID: 3})
			return err
		}},
		{name: "scan file", call: func() error {
			_, err := projectCanonicalProductArtifactScanJob(&appagentthread.ArtifactScanJobSummary{JobID: 1, ThreadID: 2, RunID: 3, ArtifactID: 4})
			return err
		}},
		{name: "token usage run", call: func() error {
			_, err := projectCanonicalProductTokenUsage(&appagentthread.TokenUsageSummary{UsageID: 1, ThreadID: 2})
			return err
		}},
		{name: "memory thread", call: func() error {
			_, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{MemoryID: 1})
			return err
		}},
		{name: "memory audit thread", call: func() error {
			_, err := projectCanonicalProductMemoryAudit(&appagentthread.MemoryAuditEventSummary{EventID: 1})
			return err
		}},
		{name: "guardrail audit thread", call: func() error {
			_, err := projectCanonicalProductGuardrailAudit(&appagentthread.GuardrailAuditEventSummary{EventID: 1})
			return err
		}},
		{name: "mcp audit thread", call: func() error {
			_, err := projectCanonicalProductMCPRuntimeAudit(&appagentthread.MCPRuntimeAuditEventSummary{EventID: 1})
			return err
		}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.call())
		})
	}
}

func requireCanonicalProductWireFields(t *testing.T, value any, want []string) map[string]any {
	t.Helper()
	encoded := canonicalProjectionJSON(t, value)
	var fields map[string]any
	require.NoError(t, json.Unmarshal([]byte(encoded), &fields))
	got := make([]string, 0, len(fields))
	for key := range fields {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	require.Equal(t, want, got)
	return fields
}

func requireCanonicalProductRFC3339(t *testing.T, values ...string) {
	t.Helper()
	for _, value := range values {
		_, err := time.Parse(time.RFC3339Nano, value)
		require.NoError(t, err)
	}
}

func TestCanonicalProductProjectionSupportsUploadsAggregatesAndMemoryAudits(t *testing.T) {
	upload, err := projectCanonicalProductUpload(&appagentthread.TaskThreadUploadedFileSummary{
		FileID: 6001, FileName: "report.pdf", ContentType: "application/pdf", SizeBytes: 42, CreatedAt: 1710000000123,
	})
	require.NoError(t, err)
	require.Equal(t, "6001", upload.FileID)

	aggregate := projectCanonicalProductTokenUsageAggregate(&appagentthread.TokenUsageAggregateSummary{TotalTokens: 42})
	require.Equal(t, int64(42), aggregate.TotalTokens)
	runAggregate, err := projectCanonicalProductRunTokenUsageAggregate(&appagentthread.RunTokenUsageAggregateSummary{
		RunID: 7001, Aggregate: &appagentthread.TokenUsageAggregateSummary{TotalTokens: 42},
	})
	require.NoError(t, err)
	require.Equal(t, "7001", runAggregate.RunID)

	memoryAudit, err := projectCanonicalProductMemoryAudit(&appagentthread.MemoryAuditEventSummary{
		EventID: 2001, ThreadID: 8001, RunID: 7001, MemoryID: 3001, CreatedAt: 1710000000123,
	})
	require.NoError(t, err)
	require.Equal(t, "2001", memoryAudit.EventID)
	require.Equal(t, "3001", memoryAudit.MemoryID)

	encoded := canonicalProjectionJSON(t, []any{upload, aggregate, runAggregate, memoryAudit})
	for _, sensitive := range canonicalProductSensitiveSentinels() {
		require.NotContains(t, encoded, sensitive)
	}
}

func canonicalProductSensitiveMetadata() string {
	return `{"authorization":"authorization-secret","api_key":"api-key-secret","tool_arguments":"tool-arguments-secret","provider_body":"provider-body-secret","worker_id":"worker-identity-secret","lease_token":"lease-token-secret","raw_usage":"raw-usage-secret","signed_url":"https://storage.example.invalid/artifact?signature=signed-url-secret"}`
}

func canonicalProductSensitiveSentinels() []string {
	return []string{
		"authorization-secret", "api-key-secret", "tool-arguments-secret", "provider-body-secret",
		"worker-identity-secret", "lease-token-secret", "raw-usage-secret", "signed-url-secret",
	}
}
