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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

func TestCanonicalProductProjectionUsesStringIDsAndRFC3339Times(t *testing.T) {
	artifact, err := projectCanonicalProductArtifact(&appagentthread.ArtifactSummary{
		ArtifactID: 9001,
		ThreadID:   8001,
		RunID:      7001,
		FileID:     6001,
		Title:      "public report",
		Metadata:   `{"visible":"yes"}`,
		CreatedAt:  1710000000123,
		UpdatedAt:  1710000001123,
	})
	require.NoError(t, err)
	require.Equal(t, "9001", artifact.ArtifactID)
	require.Equal(t, "8001", artifact.ThreadID)
	require.Equal(t, "7001", artifact.RunID)
	require.Equal(t, "6001", artifact.FileID)
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
		Title: "public artifact", Metadata: sensitive, CreatedAt: 1710000000123,
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
		CreatedAt: 1710000000123,
	})
	require.NoError(t, err)
	mem, err := projectCanonicalProductMemory(&appagentthread.MemorySummary{
		MemoryID: 3001, ThreadID: 8001, RunID: 7001, Content: "approved memory", Metadata: sensitive, CreatedAt: 1710000000123,
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
	require.Equal(t, []string{"rule-a", "rule-b"}, guardrail.RuleIDs)
	encoded := canonicalProjectionJSON(t, []any{artifact, usage, scan, mem, guardrail, mcp})
	for _, sensitive := range canonicalProductSensitiveSentinels() {
		require.NotContains(t, encoded, sensitive)
	}
	for _, field := range []string{"raw_usage", "worker_id", "lease_token", "signed_url"} {
		require.NotContains(t, encoded, field)
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
