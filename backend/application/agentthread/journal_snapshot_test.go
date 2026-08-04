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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestJournalSnapshotSubmissionUsesServerIdentityAndStandardActionEvent(t *testing.T) {
	service, _, _ := newJournalSnapshotApplicationTestService()
	snapshot, event, err := service.SubmitJournalContent(
		context.Background(),
		JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "action:read-prd:ready", TraceID: "trace-1",
			Status: entity.JournalContentStatusReady,
			Action: JournalContentAction{
				ActionID: "action-read-prd", MilestoneID: "milestone-plan",
				Operation: "read", Target: "Journal PRD",
				DisplayVerbRunning:   "正在读取 Journal PRD",
				DisplayVerbCompleted: "已读取 Journal PRD",
			},
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Review", Format: "markdown", Content: "Safe body",
			}},
		},
	)
	require.NoError(t, err)
	_, err = uuid.Parse(snapshot.SnapshotID)
	require.NoError(t, err)
	require.Equal(t, uint32(1), snapshot.Revision)
	require.Equal(t, int64(1_000)+(30*24*time.Hour).Milliseconds(), snapshot.ExpiresAt)
	require.Equal(t, "action.terminal", event.EventType)
	require.Equal(t, "action-read-prd", event.ActionID)
	require.Equal(t, "milestone-plan", event.Milestone)
	require.Equal(t, "read", event.Operation)
	require.Equal(t, "Journal PRD", event.Target)
	require.Equal(t, snapshot.SnapshotID, event.SnapshotID)
	require.JSONEq(t, `{
		"type":"document",
		"data":{
			"action_id":"action-read-prd",
			"milestone_id":"milestone-plan",
			"operation":"read",
			"target":"Journal PRD",
			"display_verb_running":"正在读取 Journal PRD",
			"display_verb_completed":"已读取 Journal PRD",
			"content_type":"document",
			"revision":1
		}
	}`, event.Payload)

	_, _, err = service.SubmitJournalContent(context.Background(), JournalContentSubmission{
		SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
		SnapshotID: "caller-controlled", IdempotencyKey: "caller-controlled",
		Status: entity.JournalContentStatusReady,
		Action: JournalContentAction{
			ActionID: "action-caller", Operation: "read", Target: "document",
			DisplayVerbRunning: "正在读取文档", DisplayVerbCompleted: "已读取文档",
		},
		Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "Review", Content: "Safe body",
		}},
	})
	require.Error(t, err)
}

func TestJournalSnapshotReadReturnsTypedContentAndValidatesCursorConsistently(t *testing.T) {
	service, _, _ := newJournalSnapshotApplicationTestService()
	snapshot, _, err := service.SubmitJournalContent(context.Background(), JournalContentSubmission{
		SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
		IdempotencyKey: "action:typed-read:ready",
		Status:         entity.JournalContentStatusReady,
		Action: JournalContentAction{
			ActionID: "action-typed-read", Operation: "read", Target: "Review",
			DisplayVerbRunning: "正在读取 Review", DisplayVerbCompleted: "已读取 Review",
		},
		Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "Review", Content: "Safe body",
		}},
	})
	require.NoError(t, err)

	view, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: snapshot.SnapshotID,
	})
	require.NoError(t, err)
	require.NotNil(t, view.Content.Document)
	require.Equal(t, "Safe body", view.Content.Document.Content)

	_, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: snapshot.SnapshotID, Cursor: "not-a-cursor",
	})
	require.Error(t, err)
}

func TestJournalSnapshotTypedProducersSanitizeFiveViews(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	tests := []struct {
		name        string
		snapshotID  string
		content     JournalTypedSnapshotContent
		contentType entity.JournalSnapshotContentType
	}{
		{
			name: "document", snapshotID: "snap-document",
			content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Review", Format: "markdown", Content: "# Result\nSafe body",
			}},
			contentType: entity.JournalSnapshotContentTypeDocument,
		},
		{
			name: "terminal", snapshotID: "snap-terminal",
			content: JournalTypedSnapshotContent{Terminal: &JournalTerminalContent{
				SessionID: "session-1", Command: "API_KEY=super-secret make test",
				WorkingDirectory: "/private/workspace/repo", Stdout: "\x1b[31mok\x1b[0m",
				Stderr: "safe\u0085\u202Etext", ExitCode: int32Pointer(0), StartedAt: 100, FinishedAt: 120,
			}},
			contentType: entity.JournalSnapshotContentTypeTerminal,
		},
		{
			name: "code", snapshotID: "snap-code",
			content: JournalTypedSnapshotContent{Code: &JournalCodeContent{
				Repository: "coze-studio", Revision: "0123456789abcdef",
				Path: "backend/main.go", Language: "go", Content: "package main\n",
				StartLine: 1, EndLine: 1,
			}},
			contentType: entity.JournalSnapshotContentTypeCode,
		},
		{
			name: "skill", snapshotID: "snap-skill",
			content: JournalTypedSnapshotContent{Skill: &JournalSkillContent{Skills: []JournalSkillSummary{
				{SkillID: "research-planner", Name: "Research planner", Description: "Plan public research", InvocationSummary: "internal details"},
			}}},
			contentType: entity.JournalSnapshotContentTypeSkill,
		},
		{
			name: "browser", snapshotID: "snap-browser",
			content: JournalTypedSnapshotContent{Browser: &JournalBrowserContent{
				CaptureID: "capture-1", Resource: "https://example.com/report?token=secret",
				Title: "Public report", MIMEType: "image/png",
				StaticSnapshot: append([]byte("\x89PNG\r\n\x1a\n"), []byte("safe-image")...),
				Analysis:       []string{"The report contains three sections."}, Index: 1, Total: 1,
				Redacted: true, RedactionEvidenceID: "evidence-1", RedactionPolicyVersion: "v1",
			}},
			contentType: entity.JournalSnapshotContentTypeBrowser,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, event, err := service.SubmitJournalContent(context.Background(), journalSnapshotTestSubmission(JournalContentSubmission{
				SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
				IdempotencyKey: "submit:" + test.snapshotID,
				TraceID:        "trace-1", Status: entity.JournalContentStatusReady,
				Content: test.content,
			}))
			require.NoError(t, err)
			require.Equal(t, test.contentType, snapshot.ContentType)
			require.Equal(t, snapshot.SnapshotID, event.SnapshotID)
			require.NotContains(t, snapshot.ContentJSON, "super-secret")
			require.NotContains(t, snapshot.ContentJSON, "\\u001b")
			require.NotContains(t, snapshot.ContentJSON, "/private/workspace")
			require.NotContains(t, snapshot.ContentJSON, "internal details")
			require.NotContains(t, snapshot.ContentJSON, "token=secret")
			require.NotContains(t, snapshot.ContentJSON, "\u0085")
			require.NotContains(t, snapshot.ContentJSON, "\u202e")
			require.Equal(t, snapshot, repo.snapshots[snapshot.SnapshotID])
		})
	}
}

func TestJournalSnapshotRejectsExecutableAndCredentialContent(t *testing.T) {
	service, _, _ := newJournalSnapshotApplicationTestService()
	tests := []struct {
		name    string
		content JournalTypedSnapshotContent
	}{
		{
			name: "document script",
			content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "unsafe", Content: "<script>alert(1)</script>",
			}},
		},
		{
			name: "javascript url",
			content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "unsafe", Content: "safe", OriginalURL: "javascript:alert(1)",
			}},
		},
		{
			name: "credential file",
			content: JournalTypedSnapshotContent{Code: &JournalCodeContent{
				Repository: "repo", Revision: "0123456789abcdef", Path: ".env",
				Language: "text", Content: "TOKEN=secret",
			}},
		},
		{
			name: "binary code",
			content: JournalTypedSnapshotContent{Code: &JournalCodeContent{
				Repository: "repo", Revision: "0123456789abcdef", Path: "data.bin",
				Language: "binary", Content: "a\x00b",
			}},
		},
		{
			name: "private key path",
			content: JournalTypedSnapshotContent{Code: &JournalCodeContent{
				Repository: "repo", Revision: "0123456789abcdef", Path: "certs/client.pem",
				Language: "text", Content: "certificate",
			}},
		},
		{
			name: "private key body",
			content: JournalTypedSnapshotContent{Code: &JournalCodeContent{
				Repository: "repo", Revision: "0123456789abcdef", Path: "config.txt",
				Language: "text", Content: "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
			}},
		},
		{
			name: "svg browser snapshot",
			content: JournalTypedSnapshotContent{Browser: &JournalBrowserContent{
				CaptureID: "capture", Resource: "https://example.com", Title: "unsafe",
				MIMEType: "image/svg+xml", StaticSnapshot: []byte(`<svg><script>alert(1)</script></svg>`),
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := service.SubmitJournalContent(context.Background(), journalSnapshotTestSubmission(JournalContentSubmission{
				SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
				IdempotencyKey: "unsafe:" + strings.ReplaceAll(test.name, " ", "-"),
				Status:         entity.JournalContentStatusReady, Content: test.content,
			}))
			require.ErrorIs(t, err, ErrJournalSnapshotUnsafeContent)
		})
	}
}

func TestJournalSnapshotRedactsTerminalCredentialForms(t *testing.T) {
	t.Parallel()

	service, _, _ := newJournalSnapshotApplicationTestService()
	snapshot, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "terminal:credential-forms",
			Status:         entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Terminal: &JournalTerminalContent{
				Command: "MODE=mode-private-value curl -u alice:hunter2 https://dbuser:dbpass@example.com && mysql -uroot -pMySQLSecret",
			}},
		}),
	)
	require.NoError(t, err)
	for _, secret := range []string{"mode-private-value", "alice:hunter2", "dbuser:dbpass", "MySQLSecret"} {
		require.NotContains(t, snapshot.ContentJSON, secret)
	}
	require.Contains(t, snapshot.ContentJSON, "[redacted]")
}

func TestJournalSnapshotRejectsProducerSuppliedOriginalURL(t *testing.T) {
	t.Parallel()

	service, _, _ := newJournalSnapshotApplicationTestService()
	_, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "document:direct-original-url",
			Status:         entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Review", Content: "Safe body",
				OriginalURL: "https://storage.example.test/review.md?signature=private",
			}},
		}),
	)
	require.ErrorIs(t, err, ErrJournalSnapshotUnsafeContent)
}

func TestJournalSnapshotBrowserRequiresVerifiedRedactionEvidence(t *testing.T) {
	service, _, _ := newJournalSnapshotApplicationTestService()
	service.JournalBrowserRedactionVerifier = nil
	_, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "browser:unverified", Status: entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Browser: &JournalBrowserContent{
				CaptureID: "capture-unverified", MIMEType: "image/png",
				StaticSnapshot: append([]byte("\x89PNG\r\n\x1a\n"), []byte("safe-image")...),
				Redacted:       true, RedactionEvidenceID: "evidence-1", RedactionPolicyVersion: "v1",
			}},
		}),
	)
	require.ErrorIs(t, err, ErrJournalSnapshotUnsafeContent)
}

func TestJournalSnapshotBrowserEvidenceBindsAllPublicFields(t *testing.T) {
	t.Parallel()

	service, _, _ := newJournalSnapshotApplicationTestService()
	verifier := &journalBrowserRedactionVerifierStub{allowed: true}
	service.JournalBrowserRedactionVerifier = verifier
	for index, title := range []string{"Public report", "Updated report"} {
		_, _, err := service.SubmitJournalContent(
			context.Background(),
			journalSnapshotTestSubmission(JournalContentSubmission{
				SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
				IdempotencyKey: fmt.Sprintf("browser:evidence-fields:%d", index),
				Status:         entity.JournalContentStatusReady,
				Content: JournalTypedSnapshotContent{Browser: &JournalBrowserContent{
					CaptureID: "capture-fields", Resource: "https://example.com/report",
					Title: title, MIMEType: "image/png",
					StaticSnapshot: append([]byte("\x89PNG\r\n\x1a\n"), []byte("safe-image")...),
					Analysis:       []string{"Section one", "Section two"}, Index: 1, Total: 2,
					Redacted: true, RedactionEvidenceID: "evidence-fields",
					RedactionPolicyVersion: "v1",
				}},
			}),
		)
		require.NoError(t, err)
	}
	require.Len(t, verifier.evidences, 2)
	require.NotEmpty(t, verifier.evidences[0].PublicFieldsHash)
	require.NotEqual(t, verifier.evidences[0].PublicFieldsHash, verifier.evidences[1].PublicFieldsHash)
}

func TestJournalSnapshotSanitizesDocumentControlMetadata(t *testing.T) {
	service, _, _ := newJournalSnapshotApplicationTestService()
	snapshot, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "submit:document-metadata",
			Status:         entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Token: "api_key=private-value", Title: "Review", Format: "markdown",
				Content: "Safe body", ActiveBlock: "/private/workspace/secret",
				Revision: "rev-1", SyncStatus: "synced<script>",
			}},
		}),
	)
	require.NoError(t, err)
	require.NotContains(t, snapshot.ContentJSON, "private-value")
	require.NotContains(t, snapshot.ContentJSON, "/private/workspace")
	require.NotContains(t, snapshot.ContentJSON, "<script>")
	require.Contains(t, snapshot.ContentJSON, `"revision":"rev-1"`)
}

func TestJournalSnapshotReadReauthorizesAndAuditsEveryOutcome(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	registry := prometheus.NewRegistry()
	metrics, err := NewJournalPrometheusMetricsCollector(registry)
	require.NoError(t, err)
	service.JournalMetrics = metrics
	authorizer := &journalSnapshotAuthorizerStub{allowed: true}
	service.JournalSnapshotAuthorizer = authorizer
	service.JournalSnapshotNow = func() int64 { return 1_000 }
	repo.snapshots["snap-read"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-read", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 501,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON: `{"document":{"title":"Review","content":"safe"}}`,
		ACLDomain:   "space:10/thread:1", CreatedAt: 900,
	}

	view, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-read", TraceID: "trace-allowed",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusReady, view.Status)
	require.Equal(t, JournalSnapshotCacheControl, view.CacheControl)
	require.Equal(t, entity.JournalSnapshotPermissionAllowed, repo.audits[0].PermissionResult)

	authorizer.allowed = false
	view, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-read", TraceID: "trace-denied",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusNoPermission, view.Status)
	require.Equal(t, JournalErrorCodeNoPermission, view.ErrorCode)
	require.Nil(t, view.Content.Document)
	require.Equal(t, entity.JournalSnapshotPermissionDenied, repo.audits[1].PermissionResult)

	authorizer.allowed = true
	repo.snapshots["snap-read"].ExpiresAt = 999
	view, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-read", TraceID: "trace-expired",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusError, view.Status)
	require.Equal(t, JournalErrorCodeSnapshotUnavailable, view.ErrorCode)
	require.Nil(t, view.Content.Document)
	require.Equal(t, entity.JournalSnapshotPermissionExpired, repo.audits[2].PermissionResult)
	require.Equal(t, 3, authorizer.calls)

	for _, expected := range []JournalMetricLabels{
		{Version: entity.JournalSchemaVersion, RolloutCohort: "treatment", TaskType: "unknown", ClientVersion: "unknown", Result: "success", ErrorCode: "none"},
		{Version: entity.JournalSchemaVersion, RolloutCohort: "treatment", TaskType: "unknown", ClientVersion: "unknown", Result: "no_permission", ErrorCode: "no_permission"},
		{Version: entity.JournalSchemaVersion, RolloutCohort: "treatment", TaskType: "unknown", ClientVersion: "unknown", Result: "failed", ErrorCode: "snapshot_unavailable"},
	} {
		require.Equal(t, float64(1), testutil.ToFloat64(metrics.snapshotRequestsTotal.With(
			prometheus.Labels(expected.prometheusLabels()),
		)))
	}
}

func TestJournalSnapshotAuthorizationBackendFailureIsUnavailable(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	service.JournalSnapshotAuthorizer = &journalSnapshotAuthorizerStub{
		err: errors.New("authorization backend unavailable"),
	}
	repo.snapshots["snap-auth-unavailable"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-auth-unavailable", SpaceID: 10, ThreadID: 1, RunID: 10,
		JournalRunID: 10, AttemptID: "att-1", EventID: 508,
		ActionID: "action-auth", Revision: 1,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON: `{"document":{"title":"Review","content":"safe"}}`,
		ACLDomain:   "space:10/thread:1", CreatedAt: 900,
	}

	_, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-auth-unavailable", TraceID: "trace-auth-unavailable",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)
	require.Len(t, repo.audits, 1)
	require.Equal(t, entity.JournalSnapshotPermissionDenied, repo.audits[0].PermissionResult)
}

func TestJournalSnapshotRuntimeFileSourceIsReauthorizedForReadsAndActions(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	digest := strings.Repeat("d", 64)
	reader := &journalSnapshotRuntimeFileReaderStub{file: &entity.AgentFile{
		ID: 601, SpaceID: 10, ThreadID: 1, RunID: 10,
		FileKind: entity.AgentFileKindOutput, Status: entity.AgentFileStatusActive,
		Digest: digest,
	}}
	service.JournalSnapshotRuntimeFileReader = reader
	repo.snapshots["snap-runtime-file"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-runtime-file", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 509,
		ContentType: entity.JournalSnapshotContentTypeCode,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/plain", Encoding: "utf-8",
		ContentJSON: `{"code":{"repository":"repo","revision":"0123456","path":"main.go","content":"package main"}}`,
		ACLDomain:   "space:10/thread:1", SourceResourceType: "runtime_file",
		SourceResourceID: "601", SourceRevision: digest, CreatedAt: 900,
	}

	_, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-runtime-file", TraceID: "trace-runtime-read",
	})
	require.NoError(t, err)
	grant, err := service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: "snap-runtime-file", Action: entity.JournalSnapshotActionCopyCode,
			IdempotencyKey: "copy-runtime-1", TraceID: "trace-runtime-copy-1",
		},
	)
	require.NoError(t, err)
	require.True(t, grant.Allowed)

	reader.file.Digest = strings.Repeat("e", 64)
	grant, err = service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: "snap-runtime-file", Action: entity.JournalSnapshotActionCopyCode,
			IdempotencyKey: "copy-runtime-digest-changed", TraceID: "trace-runtime-digest-changed",
		},
	)
	require.NoError(t, err)
	require.False(t, grant.Allowed)

	reader.file.Digest = digest
	reader.file.Status = entity.AgentFileStatusDeleted
	grant, err = service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: "snap-runtime-file", Action: entity.JournalSnapshotActionCopyCode,
			IdempotencyKey: "copy-runtime-2", TraceID: "trace-runtime-copy-2",
		},
	)
	require.NoError(t, err)
	require.False(t, grant.Allowed)
	require.Equal(t, 4, reader.calls)
	require.Equal(t, entity.JournalSnapshotPermissionDenied,
		repo.audits[len(repo.audits)-1].PermissionResult)
}

func TestJournalSnapshotArtifactSourceIsReauthorizedOnEveryRead(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	digest := strings.Repeat("e", 64)
	reader := &journalSnapshotArtifactReaderStub{artifact: &entity.AgentArtifact{
		ID: 701, SpaceID: 10, ThreadID: 1, RunID: 10,
		Metadata: `{"content_hash":"` + digest + `"}`,
	}}
	service.JournalSnapshotArtifactReader = reader
	service.ArtifactAuthorizer = &journalSnapshotArtifactAuthorizerStub{allowed: true}
	repo.snapshots["snap-artifact"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-artifact", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 512,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON: `{"document":{"title":"Review","content":"safe"}}`,
		ACLDomain:   "space:10/thread:1", SourceResourceType: "artifact",
		SourceResourceID: "701", SourceRevision: digest, CreatedAt: 900,
	}

	view, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-artifact", TraceID: "trace-artifact-1",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusReady, view.Status)

	reader.artifact.Metadata = `{"content_hash":"` + strings.Repeat("f", 64) + `"}`
	view, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-artifact", TraceID: "trace-artifact-revision-changed",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusNoPermission, view.Status)

	reader.artifact.Metadata = `{"content_hash":"` + digest + `"}`
	reader.artifact.DeletedAt = 1_000
	view, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-artifact", TraceID: "trace-artifact-2",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusNoPermission, view.Status)
	require.Equal(t, 3, reader.calls)
}

func TestJournalSnapshotOriginalUsesImmutableArtifactCapability(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	digest := strings.Repeat("a", 64)
	artifact := &entity.AgentArtifact{
		ID: 701, SpaceID: 10, ThreadID: 1, RunID: 10,
		ObjectURI: "agent-runtime/10/1/runs/10/outputs/review.md",
		Metadata:  `{"content_hash":"` + digest + `","scan_status":"clean"}`,
	}
	reader := &journalSnapshotArtifactReaderStub{artifact: artifact}
	issuer := &journalSnapshotArtifactCapabilityIssuerStub{
		response: &CreateArtifactSignedURLResponse{URL: "https://objects.example/review.md"},
	}
	service.JournalSnapshotArtifactReader = reader
	service.JournalSnapshotArtifactCapabilityIssuer = issuer
	service.ArtifactAuthorizer = &journalSnapshotArtifactAuthorizerStub{allowed: true}
	repo.snapshots["snap-artifact-original"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-artifact-original", SpaceID: 10, ThreadID: 1, RunID: 10,
		JournalRunID: 10, AttemptID: "att-1", EventID: 509,
		ActionID: "action-artifact", Revision: 1,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON:        `{"document":{"title":"Review","content":"safe"}}`,
		SourceResourceType: "artifact", SourceResourceID: "701", SourceRevision: digest,
		OriginalObjectKey: artifact.ObjectURI,
		ACLDomain:         "space:10/thread:1", CreatedAt: 900,
	}

	grant, err := service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: "snap-artifact-original", Action: entity.JournalSnapshotActionOpenOriginal,
			IdempotencyKey: "open-artifact-original",
		},
	)
	require.NoError(t, err)
	require.Equal(t, "https://objects.example/review.md", grant.DownloadURL)
	require.Equal(t, 1, issuer.calls)

	reader.artifact.Metadata = `{"content_hash":"` + strings.Repeat("b", 64) + `","scan_status":"clean"}`
	view, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-artifact-original", TraceID: "artifact-revision-changed",
	})
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusNoPermission, view.Status)
	require.Equal(t, 1, issuer.calls)
}

func TestJournalSnapshotRuntimeFileOriginalCannotBeDirectlySigned(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	digest := strings.Repeat("c", 64)
	service.JournalSnapshotRuntimeFileReader = &journalSnapshotRuntimeFileReaderStub{
		file: &entity.AgentFile{
			ID: 702, SpaceID: 10, ThreadID: 1, RunID: 10,
			Status: entity.AgentFileStatusActive, Digest: digest,
			ObjectURI: "agent-runtime/10/1/runs/10/outputs/review.md",
		},
	}
	repo.snapshots["snap-runtime-original"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-runtime-original", SpaceID: 10, ThreadID: 1, RunID: 10,
		JournalRunID: 10, AttemptID: "att-1", EventID: 510,
		ActionID: "action-runtime", Revision: 1,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON:        `{"document":{"title":"Review","content":"safe"}}`,
		SourceResourceType: "runtime_file", SourceResourceID: "702", SourceRevision: digest,
		OriginalObjectKey: "agent-runtime/10/1/runs/10/outputs/review.md",
		ACLDomain:         "space:10/thread:1", CreatedAt: 900,
	}

	_, err := service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: "snap-runtime-original", Action: entity.JournalSnapshotActionOpenOriginal,
			IdempotencyKey: "open-runtime-original",
		},
	)
	require.ErrorIs(t, err, ErrJournalSnapshotActionUnavailable)
	require.Len(t, repo.audits, 1)
	require.Equal(t, entity.JournalSnapshotPermissionDenied, repo.audits[0].PermissionResult)
}

func TestJournalSnapshotActionAuditFailureBlocksContentAccess(t *testing.T) {
	service, repo, objects := newJournalSnapshotApplicationTestService()
	service.JournalSnapshotAuthorizer = &journalSnapshotAuthorizerStub{allowed: true}
	repo.snapshots["snap-action"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-action", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 502,
		ContentType: entity.JournalSnapshotContentTypeTerminal,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/plain", Encoding: "utf-8", ObjectKey: "journal-snapshots/staging/10/object",
		ACLDomain: "space:10/thread:1", CreatedAt: 900,
	}
	objects.objects["journal-snapshots/staging/10/object"] = []byte(
		`{"terminal":{"command":"go test","stdout":"ok"}}`,
	)
	repo.auditErr = errors.New("audit unavailable")
	_, err := service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-action", Action: entity.JournalSnapshotActionCopyOutput,
		IdempotencyKey: "copy-1", TraceID: "trace-action",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotAuditUnavailable)
	require.Zero(t, objects.getCalls)

	repo.auditErr = nil
	grant, err := service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-action", Action: entity.JournalSnapshotActionCopyOutput,
		IdempotencyKey: "copy-1", TraceID: "trace-action",
	})
	require.NoError(t, err)
	require.True(t, grant.Allowed)
	require.Equal(t, "ok", grant.CopyText)
	require.Equal(t, 1, objects.getCalls)
}

func TestJournalSnapshotActionIdempotencyBindsFragmentTarget(t *testing.T) {
	service, repo, objects := newJournalSnapshotApplicationTestService()
	snapshot, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10,
			IdempotencyKey: "submit:fragment-targets",
			Status:         entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Fragments", Content: strings.Repeat("first paragraph\n\nsecond paragraph\n\n", 2_000),
			}},
		}),
	)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(repo.snapshots[snapshot.SnapshotID].Fragments), 2)
	firstFragmentID := repo.snapshots[snapshot.SnapshotID].Fragments[0].FragmentID
	secondFragmentID := repo.snapshots[snapshot.SnapshotID].Fragments[1].FragmentID

	first, err := service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: snapshot.SnapshotID, Action: entity.JournalSnapshotActionDownloadFragment,
			FragmentID: firstFragmentID, IdempotencyKey: "download-once",
		},
	)
	require.NoError(t, err)
	require.True(t, first.Allowed)
	require.NotEmpty(t, first.DownloadContent)
	require.Equal(t, "text/markdown", first.DownloadMIMEType)
	require.Empty(t, first.DownloadURL)
	require.Equal(t, 1, objects.getCalls, "fragment download must not reload the full manifest")

	_, err = service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID: snapshot.SnapshotID, Action: entity.JournalSnapshotActionDownloadFragment,
			FragmentID: secondFragmentID, IdempotencyKey: "download-once",
		},
	)
	require.ErrorIs(t, err, ErrJournalSnapshotIdempotencyConflict)
	require.Len(t, repo.audits, 1)
}

func TestJournalSnapshotReadRejectsTamperedContentAndCrossTenantObjects(t *testing.T) {
	service, repo, objects := newJournalSnapshotApplicationTestService()
	service.JournalSnapshotAuthorizer = &journalSnapshotAuthorizerStub{allowed: true}
	repo.snapshots["snap-tampered"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-tampered", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 504,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON: `{"document":{"title":"Unsafe","content":"<script>alert(1)</script>"}}`,
		ACLDomain:   "space:10/thread:1", CreatedAt: 900,
	}
	_, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-tampered", TraceID: "trace-tampered",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnsafeContent)

	repo.snapshots["snap-cross-object"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-cross-object", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 505,
		ContentType: entity.JournalSnapshotContentTypeTerminal,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/plain", Encoding: "utf-8",
		ObjectKey: "journal-snapshots/staging/11/foreign",
		ACLDomain: "space:10/thread:1", CreatedAt: 900,
	}
	objects.objects["journal-snapshots/staging/11/foreign"] = []byte(
		`{"terminal":{"command":"go test","stdout":"foreign"}}`,
	)
	_, err = service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-cross-object", Action: entity.JournalSnapshotActionCopyOutput,
		IdempotencyKey: "copy-cross-object", TraceID: "trace-cross-object",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)
	require.Zero(t, objects.getCalls)

	repo.snapshots["snap-cross-fragment"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-cross-fragment", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 510,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, IsFragmented: true,
		FragmentCount: 1, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ACLDomain: "space:10/thread:1", CreatedAt: 900,
		Fragments: []*entity.JournalSnapshotFragment{{
			FragmentID: "foreign-fragment", SnapshotID: "snap-cross-fragment",
			FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock,
			ObjectKey:   "journal-snapshots/staging/11/foreign",
			ContentHash: strings.Repeat("a", 64), SizeBytes: 32,
		}},
	}
	_, err = service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-cross-fragment", Action: entity.JournalSnapshotActionDownloadFragment,
		FragmentID: "foreign-fragment", IdempotencyKey: "download-cross-fragment",
		TraceID: "trace-cross-fragment",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotNoPermission)

	repo.snapshots["snap-tampered-fragment"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-tampered-fragment", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 511,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, IsFragmented: true,
		FragmentCount: 1, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ACLDomain: "space:10/thread:1", CreatedAt: 900,
		Fragments: []*entity.JournalSnapshotFragment{{
			FragmentID: "tampered-fragment", SnapshotID: "snap-tampered-fragment",
			FragmentIndex: 0, Kind: entity.JournalSnapshotFragmentKindDocumentBlock,
			InlineContent: `{"document":{"title":"Review","content":"safe"}}`,
			ContentHash:   strings.Repeat("b", 64), SizeBytes: 48,
		}},
	}
	_, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-tampered-fragment", TraceID: "trace-tampered-fragment",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)

	digest := strings.Repeat("f", 64)
	repo.snapshots["snap-bad-signed-url"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-bad-signed-url", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 506,
		ContentType: entity.JournalSnapshotContentTypeDocument,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/markdown", Encoding: "utf-8",
		ContentJSON:        `{"document":{"title":"Review","content":"safe"}}`,
		SourceResourceType: "artifact", SourceResourceID: "601", SourceRevision: digest,
		OriginalObjectKey: "agent-runtime/10/1/runs/10/outputs/review.md",
		ACLDomain:         "space:10/thread:1", CreatedAt: 900,
	}
	service.JournalSnapshotArtifactReader = &journalSnapshotArtifactReaderStub{
		artifact: &entity.AgentArtifact{
			ID: 601, SpaceID: 10, ThreadID: 1, RunID: 10,
			ObjectURI: "agent-runtime/10/1/runs/10/outputs/review.md",
			Metadata:  `{"content_hash":"` + digest + `"}`,
		},
	}
	service.ArtifactAuthorizer = &journalSnapshotArtifactAuthorizerStub{allowed: true}
	service.JournalSnapshotArtifactCapabilityIssuer = &journalSnapshotArtifactCapabilityIssuerStub{
		response: &CreateArtifactSignedURLResponse{URL: "javascript:alert(1)"},
	}
	_, err = service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-bad-signed-url", Action: entity.JournalSnapshotActionOpenOriginal,
		IdempotencyKey: "open-bad-url", TraceID: "trace-bad-url",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnsafeContent)
}

func TestJournalSnapshotObjectStagingIsTenantScopedAndReapableAfterRollback(t *testing.T) {
	service, repo, objects := newJournalSnapshotApplicationTestService()
	repo.createErr = errors.New("database rollback")
	large := strings.Repeat("section content\n", 8_000)
	_, _, err := service.SubmitJournalContent(context.Background(), journalSnapshotTestSubmission(JournalContentSubmission{
		SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
		IdempotencyKey: "submit:orphan",
		Status:         entity.JournalContentStatusReady,
		Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "Large", Content: large,
		}},
	}))
	require.Error(t, err)
	require.NotEmpty(t, objects.putKeys)
	orphanSnapshotID := journalSnapshotServerID(
		JournalContentSubmission{SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1"},
		"action-"+journalSnapshotHash([]byte("submit:orphan"))[:16],
		1,
	)
	require.Contains(t, repo.reservations, orphanSnapshotID)
	for _, key := range objects.putKeys {
		require.Contains(t, key, "/10/")
		require.NotContains(t, key, "section content")
	}

	service.JournalSnapshotNow = func() int64 { return 1_000_000 }
	deleted, err := service.ReapOrphanedJournalSnapshotObjects(
		context.Background(), 10, 2_000,
	)
	require.NoError(t, err)
	require.Equal(t, len(objects.putKeys), deleted)
	require.Len(t, objects.objects, 0)
	require.NotContains(t, repo.reservations, orphanSnapshotID)

	repo.createErr = nil
	objects.putKeys = nil
	_, _, err = service.SubmitJournalContent(context.Background(), journalSnapshotTestSubmission(JournalContentSubmission{
		SpaceID: 11, ThreadID: 2, RunID: 20, AttemptID: "att-2",
		IdempotencyKey: "submit:tenant-11",
		Status:         entity.JournalContentStatusReady,
		Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "Large", Content: large,
		}},
	}))
	require.NoError(t, err)
	require.NotEmpty(t, objects.putKeys)
	for _, key := range objects.putKeys {
		require.Contains(t, key, "/11/")
		require.NotContains(t, key, "/10/")
	}
}

func TestJournalSnapshotOrphanReaperTraversesBoundedPages(t *testing.T) {
	service, _, objects := newJournalSnapshotApplicationTestService()
	const objectCount = 450
	for index := 0; index < objectCount; index++ {
		key := fmt.Sprintf("journal-snapshots/staging/10/20260730/orphan-%04d", index)
		objects.objects[key] = []byte("orphan")
	}
	service.JournalSnapshotNow = func() int64 { return 1_000_000 }

	deleted, err := service.ReapOrphanedJournalSnapshotObjects(
		context.Background(),
		10,
		2_000,
	)

	require.NoError(t, err)
	require.Equal(t, objectCount, deleted)
	require.Empty(t, objects.objects)
	require.Equal(t, 3, objects.listCalls)
}

func TestJournalSnapshotAdmissionFailureDoesNotWriteObjects(t *testing.T) {
	service, repo, objects := newJournalSnapshotApplicationTestService()
	repo.reserveErr = domainrepo.ErrJournalProjectionInactive
	_, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "submit:admission-denied",
			Status:         entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Large", Content: strings.Repeat("section content\n", 8_000),
			}},
		}),
	)
	require.ErrorIs(t, err, domainrepo.ErrJournalProjectionInactive)
	require.Empty(t, objects.putKeys)
}

func TestJournalSnapshotMissingReadAndRevokedIdempotentActionRemainAudited(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	authorizer := &journalSnapshotAuthorizerStub{allowed: true}
	service.JournalSnapshotAuthorizer = authorizer

	_, err := service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "missing", TraceID: "trace-missing",
	})
	require.ErrorIs(t, err, domainrepo.ErrJournalSnapshotNotFound)
	require.Len(t, repo.audits, 1)
	require.Equal(t, entity.JournalSnapshotPermissionDenied, repo.audits[0].PermissionResult)
	require.Empty(t, repo.audits[0].ContentType)
	_, err = service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "missing", Action: entity.JournalSnapshotActionOpenOriginal,
		IdempotencyKey: "missing-action", TraceID: "trace-missing-action",
	})
	require.ErrorIs(t, err, domainrepo.ErrJournalSnapshotNotFound)
	require.Len(t, repo.audits, 2)
	require.Equal(t, entity.JournalSnapshotPermissionDenied, repo.audits[1].PermissionResult)

	repo.snapshots["snap-reauth"] = &entity.JournalContentSnapshot{
		SnapshotID: "snap-reauth", SpaceID: 10, ThreadID: 1, RunID: 10,
		AttemptID: "att-1", EventID: 503,
		ContentType: entity.JournalSnapshotContentTypeTerminal,
		Status:      entity.JournalContentStatusReady, Visibility: entity.JournalVisibilityUser,
		MIMEType: "text/plain", Encoding: "utf-8",
		ContentJSON: `{"terminal":{"command":"go test","stdout":"ok"}}`,
		ACLDomain:   "space:10/thread:1", CreatedAt: 900,
	}
	first, err := service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-reauth", Action: entity.JournalSnapshotActionCopyOutput,
		IdempotencyKey: "same-action", TraceID: "trace-action",
	})
	require.NoError(t, err)
	require.True(t, first.Allowed)

	authorizer.allowed = false
	second, err := service.AuditJournalSnapshotAction(context.Background(), AuditJournalSnapshotActionRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: "snap-reauth", Action: entity.JournalSnapshotActionCopyOutput,
		IdempotencyKey: "same-action", TraceID: "trace-action",
	})
	require.NoError(t, err)
	require.False(t, second.Allowed)
	require.Len(t, repo.audits, 4)
	require.Equal(t, entity.JournalSnapshotPermissionDenied, repo.audits[3].PermissionResult)
	require.Contains(t, repo.audits[3].IdempotencyKey, ":reauth:denied")
}

func TestJournalSnapshotFrozenStatusesAndFragmentFlagStaySeparate(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	statuses := []entity.JournalContentStatus{
		entity.JournalContentStatusEmpty,
		entity.JournalContentStatusLoading,
		entity.JournalContentStatusError,
		entity.JournalContentStatusNoPermission,
	}
	for index, status := range statuses {
		snapshotID := "status-" + string(status)
		snapshot, _, err := service.SubmitJournalContent(context.Background(), journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10,
			IdempotencyKey: "submit:" + snapshotID,
			Status:         status, ContentType: entity.JournalSnapshotContentTypeDocument,
			ErrorCode: []string{"", "", "CONTENT_ERROR", JournalErrorCodeNoPermission}[index],
		}))
		require.NoError(t, err)
		require.Equal(t, status, snapshot.Status)
		require.False(t, snapshot.IsFragmented)
	}

	large := strings.Repeat("line content\n", 8_000)
	fragmented, _, err := service.SubmitJournalContent(context.Background(), journalSnapshotTestSubmission(JournalContentSubmission{
		SpaceID: 10, ThreadID: 1, RunID: 10,
		IdempotencyKey: "submit:fragmented",
		Status:         entity.JournalContentStatusReady,
		Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "Large", Content: large,
		}},
	}))
	require.NoError(t, err)
	require.Equal(t, entity.JournalContentStatusReady, fragmented.Status)
	require.True(t, fragmented.IsFragmented)
	require.NotEmpty(t, repo.snapshots[fragmented.SnapshotID].Fragments)
	require.NotEqual(t, entity.JournalContentStatus("fragmented"), fragmented.Status)
}

func TestJournalSnapshotFragmentedViewsExposeSemanticContent(t *testing.T) {
	largeText := strings.Repeat("semantic line\n", 8_000)
	largeSkills := make([]JournalSkillSummary, 0, 256)
	for index := 0; index < 256; index++ {
		largeSkills = append(largeSkills, JournalSkillSummary{
			SkillID:     fmt.Sprintf("skill-%03d", index),
			Name:        fmt.Sprintf("Skill %03d", index),
			Description: strings.Repeat("safe description ", 32),
		})
	}
	largePNG := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x42}, 96*1024)...)

	tests := []struct {
		name        string
		contentType entity.JournalSnapshotContentType
		content     JournalTypedSnapshotContent
		kind        JournalSnapshotFragmentKind
		assert      func(*testing.T, JournalSnapshotFragmentView)
	}{
		{
			name: "document", contentType: entity.JournalSnapshotContentTypeDocument,
			content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Large document", Content: largeText,
			}},
			kind: JournalSnapshotFragmentKindDocumentBlock,
			assert: func(t *testing.T, fragment JournalSnapshotFragmentView) {
				require.NotEmpty(t, fragment.BlockID)
				require.Contains(t, fragment.Content, "semantic line")
			},
		},
		{
			name: "terminal", contentType: entity.JournalSnapshotContentTypeTerminal,
			content: JournalTypedSnapshotContent{Terminal: &JournalTerminalContent{
				Command: "run-checks", Stdout: largeText,
			}},
			kind: JournalSnapshotFragmentKindTerminalStdout,
			assert: func(t *testing.T, fragment JournalSnapshotFragmentView) {
				require.Equal(t, "stdout", fragment.Stream)
				require.Contains(t, fragment.Content, "semantic line")
			},
		},
		{
			name: "code", contentType: entity.JournalSnapshotContentTypeCode,
			content: JournalTypedSnapshotContent{Code: &JournalCodeContent{
				Repository: "coze-studio", Revision: "abcdef1234567",
				Path: "backend/service.go", Language: "go", Content: largeText,
				StartLine: 1,
			}},
			kind: JournalSnapshotFragmentKindCodeLines,
			assert: func(t *testing.T, fragment JournalSnapshotFragmentView) {
				require.Equal(t, int32(1), fragment.StartLine)
				require.GreaterOrEqual(t, fragment.EndLine, fragment.StartLine)
			},
		},
		{
			name: "skill", contentType: entity.JournalSnapshotContentTypeSkill,
			content: JournalTypedSnapshotContent{Skill: &JournalSkillContent{Skills: largeSkills}},
			kind:    JournalSnapshotFragmentKindSkillItems,
			assert: func(t *testing.T, fragment JournalSnapshotFragmentView) {
				require.NotEmpty(t, fragment.Skills)
				require.Empty(t, fragment.Content)
			},
		},
		{
			name: "browser", contentType: entity.JournalSnapshotContentTypeBrowser,
			content: JournalTypedSnapshotContent{Browser: &JournalBrowserContent{
				CaptureID: "capture-large", MIMEType: "image/png", StaticSnapshot: largePNG,
				Redacted: true, RedactionEvidenceID: "evidence-large",
				RedactionPolicyVersion: "policy-v1",
			}},
			kind: JournalSnapshotFragmentKindBrowserSnapshot,
			assert: func(t *testing.T, fragment JournalSnapshotFragmentView) {
				require.Equal(t, "image/png", fragment.MIMEType)
				require.NotEmpty(t, fragment.BinaryContent)
				require.Empty(t, fragment.Content)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, _, objects := newJournalSnapshotApplicationTestService()
			snapshot, _, err := service.SubmitJournalContent(
				context.Background(),
				journalSnapshotTestSubmission(JournalContentSubmission{
					SpaceID: 10, ThreadID: 1, RunID: 10,
					IdempotencyKey: "semantic:" + tt.name,
					Status:         entity.JournalContentStatusReady, ContentType: tt.contentType,
					Content: tt.content,
				}),
			)
			require.NoError(t, err)
			require.True(t, snapshot.IsFragmented)
			require.NotEmpty(t, snapshot.ObjectKey, "the immutable canonical snapshot remains addressable")

			envelope, err := service.GetJournalSnapshot(
				context.Background(),
				GetJournalSnapshotRequest{
					SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
					SnapshotID: snapshot.SnapshotID, Limit: 1,
				},
			)
			require.NoError(t, err)
			require.Len(t, envelope.Fragments, 1)
			fragment := envelope.Fragments[0]
			require.Equal(t, tt.kind, fragment.Kind)
			require.NotContains(t, fragment.Content, "{\"document\"")
			require.NotContains(t, fragment.Content, "{\"terminal\"")
			require.NotContains(t, fragment.Content, "{\"code\"")
			tt.assert(t, fragment)
			require.Equal(t, 1, objects.getCalls, "a fragment page must not load the full manifest")
		})
	}
}

func TestJournalSnapshotFragmentPageRejectsMissingSemanticFragment(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	snapshot, _, err := service.SubmitJournalContent(
		context.Background(),
		journalSnapshotTestSubmission(JournalContentSubmission{
			SpaceID: 10, ThreadID: 1, RunID: 10, AttemptID: "att-1",
			IdempotencyKey: "submit:fragment-gap",
			Status:         entity.JournalContentStatusReady,
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "Large", Content: strings.Repeat("semantic line\n", 8_000),
			}},
		}),
	)
	require.NoError(t, err)
	require.Greater(t, len(repo.snapshots[snapshot.SnapshotID].Fragments), 1)
	repo.snapshots[snapshot.SnapshotID].Fragments =
		repo.snapshots[snapshot.SnapshotID].Fragments[1:]

	_, err = service.GetJournalSnapshot(context.Background(), GetJournalSnapshotRequest{
		SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
		SnapshotID: snapshot.SnapshotID, TraceID: "trace-fragment-gap",
	})
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)

	_, err = service.AuditJournalSnapshotAction(
		context.Background(),
		AuditJournalSnapshotActionRequest{
			SpaceID: 10, ThreadID: 1, RunID: 10, ViewerID: 20,
			SnapshotID:     snapshot.SnapshotID,
			Action:         entity.JournalSnapshotActionDownloadFragment,
			FragmentID:     repo.snapshots[snapshot.SnapshotID].Fragments[0].FragmentID,
			IdempotencyKey: "download-fragment-gap",
		},
	)
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)
}

func TestReadJournalSnapshotObjectStopsAtConfiguredLimit(t *testing.T) {
	t.Parallel()

	reader := &journalSnapshotCountingReadCloser{
		Reader: bytes.NewReader(bytes.Repeat([]byte("x"), journalSnapshotFragmentBytes+1024)),
	}
	storage := &journalSnapshotObjectStorageStub{
		objects:    map[string][]byte{},
		openReader: reader,
	}

	_, err := readJournalSnapshotObject(
		context.Background(),
		storage,
		"journal-staging/10/fragment",
		journalSnapshotFragmentBytes,
	)
	require.ErrorIs(t, err, ErrJournalSnapshotUnavailable)
	require.Equal(t, journalSnapshotFragmentBytes+1, reader.bytesRead)
	require.True(t, reader.closed)
}

func TestChunkJournalSnapshotItemsPreservesOrderAndBounds(t *testing.T) {
	t.Parallel()

	items := make([]JournalSkillSummary, 5_000)
	for index := range items {
		items[index] = JournalSkillSummary{
			SkillID:     fmt.Sprintf("skill-%04d", index),
			Name:        fmt.Sprintf("Skill %04d", index),
			Description: strings.Repeat("safe description ", 4),
		}
	}

	chunks, err := chunkJournalSnapshotItems(items)
	require.NoError(t, err)
	require.Greater(t, len(chunks), 1)

	decoded := make([]JournalSkillSummary, 0, len(items))
	expectedStart := int32(0)
	for _, chunk := range chunks {
		require.LessOrEqual(t, len(chunk.Content), journalSnapshotFragmentBytes)
		require.Equal(t, expectedStart, chunk.ItemStart)
		require.Greater(t, chunk.ItemEnd, chunk.ItemStart)
		var page []JournalSkillSummary
		require.NoError(t, json.Unmarshal(chunk.Content, &page))
		require.Len(t, page, int(chunk.ItemEnd-chunk.ItemStart))
		decoded = append(decoded, page...)
		expectedStart = chunk.ItemEnd
	}
	require.Equal(t, int32(len(items)), expectedStart)
	require.Equal(t, items, decoded)
}

func TestJournalSnapshotFiveViewsSupportFrozenContentStatuses(t *testing.T) {
	service, _, _ := newJournalSnapshotApplicationTestService()
	contentTypes := []entity.JournalSnapshotContentType{
		entity.JournalSnapshotContentTypeDocument,
		entity.JournalSnapshotContentTypeTerminal,
		entity.JournalSnapshotContentTypeCode,
		entity.JournalSnapshotContentTypeSkill,
		entity.JournalSnapshotContentTypeBrowser,
	}
	statuses := []entity.JournalContentStatus{
		entity.JournalContentStatusEmpty,
		entity.JournalContentStatusLoading,
		entity.JournalContentStatusReady,
		entity.JournalContentStatusStreaming,
		entity.JournalContentStatusError,
		entity.JournalContentStatusNoPermission,
	}
	for _, contentType := range contentTypes {
		for _, status := range statuses {
			name := string(contentType) + "-" + string(status)
			req := JournalContentSubmission{
				SpaceID: 10, ThreadID: 1, RunID: 10,
				IdempotencyKey: "matrix:" + name,
				Status:         status, ContentType: contentType,
			}
			if status == entity.JournalContentStatusReady ||
				status == entity.JournalContentStatusStreaming {
				req.Content = journalSnapshotContentFixture(contentType)
			}
			snapshot, _, err := service.SubmitJournalContent(
				context.Background(), journalSnapshotTestSubmission(req),
			)
			require.NoError(t, err, name)
			require.Equal(t, contentType, snapshot.ContentType, name)
			require.Equal(t, status, snapshot.Status, name)
		}
	}
}

func TestJournalContentProducerDerivesAttemptAndScopeFromServerState(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	traceID := "trace-authoritative"
	repo.activeAttempt = &entity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-authoritative", Status: entity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
		TraceID: &traceID,
	}
	service.JournalSnapshotAttemptReader = repo

	snapshot, event, err := service.ProduceJournalContent(
		context.Background(),
		JournalRuntimeContentSubmission{
			Run:         &RunSummary{RunID: 10, ThreadID: 1, SpaceID: 10},
			Status:      entity.JournalContentStatusReady,
			ContentType: entity.JournalSnapshotContentTypeDocument,
			Source: JournalSnapshotSource{
				ResourceType: "runtime_file", ResourceID: "701",
				Revision: strings.Repeat("a", 64),
			},
			Action: JournalContentAction{
				ActionID: "tool-call-1", Operation: "write", Target: "评审文档",
				DisplayVerbRunning:   "正在编辑评审文档",
				DisplayVerbCompleted: "已编辑评审文档",
			},
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "评审文档", Content: "# 结论\n",
			}},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.NotNil(t, event)
	require.Equal(t, "att-authoritative", snapshot.AttemptID)
	require.Equal(t, "space:10/thread:1", snapshot.ACLDomain)
	require.Equal(t, traceID, event.TraceID)
	require.Equal(t, "tool-call-1", snapshot.ActionID)
	require.Contains(t, event.IdempotencyKey, "journal-content:")
}

func TestJournalContentProducerAllowsAuthorizedChildRunOnRootAttempt(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &entity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-root", Status: entity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
	}
	service.JournalSnapshotAttemptReader = repo

	snapshot, event, err := service.ProduceJournalContent(
		context.Background(),
		JournalRuntimeContentSubmission{
			Run: &RunSummary{
				RunID: 11, ParentRunID: 10, PlanScopeRunID: 10,
				ThreadID: 1, SpaceID: 10, RunKind: RunKindSubagent,
			},
			Status:      entity.JournalContentStatusReady,
			ContentType: entity.JournalSnapshotContentTypeDocument,
			Source: JournalSnapshotSource{
				ResourceType: "runtime_file", ResourceID: "702",
				Revision: strings.Repeat("b", 64),
			},
			Action: JournalContentAction{
				ActionID: "child-tool-call", Operation: "write", Target: "子任务报告",
				DisplayVerbRunning:   "正在编辑子任务报告",
				DisplayVerbCompleted: "已编辑子任务报告",
			},
			Content: JournalTypedSnapshotContent{Document: &JournalDocumentContent{
				Title: "子任务报告", Content: "# 结果\n",
			}},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	require.NotNil(t, event)
	require.Equal(t, int64(11), snapshot.RunID)
	require.Equal(t, int64(10), snapshot.JournalRunID)
	require.Equal(t, "att-root", snapshot.AttemptID)
}

func TestJournalContentProducerSkipsAttemptsWithoutSnapshotEnrollment(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &entity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-disabled", Status: entity.RunAttemptStatusRunning,
		SnapshotsEnabled: false, ProjectionState: entity.JournalProjectionStateHealthy,
	}
	service.JournalSnapshotAttemptReader = repo

	snapshot, event, err := service.ProduceJournalContent(
		context.Background(),
		JournalRuntimeContentSubmission{
			Run:    &RunSummary{RunID: 10, ThreadID: 1, SpaceID: 10},
			Status: entity.JournalContentStatusReady,
			Action: JournalContentAction{
				ActionID: "tool-call-disabled", Operation: "use_skill", Target: "技能",
				DisplayVerbRunning: "正在使用技能", DisplayVerbCompleted: "已使用技能",
			},
			Content: JournalTypedSnapshotContent{Skill: &JournalSkillContent{
				Skills: []JournalSkillSummary{{SkillID: "1", Name: "review"}},
			}},
		},
	)
	require.NoError(t, err)
	require.Nil(t, snapshot)
	require.Nil(t, event)
	require.Empty(t, repo.snapshots)
}

func TestSkillsLoadedProducerPublishesCatalogEventWithoutJournalAction(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &entity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-skills", Status: entity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
	}
	service.JournalSnapshotAttemptReader = repo
	events := &recordingRunEventSink{}
	run := &RunSummary{RunID: 10, ThreadID: 1, SpaceID: 10}

	emitSkillsLoadedRunEvent(
		context.Background(),
		events,
		run,
		AgentSkillContext{Items: []AgentSkill{{
			ID: 7, Name: "review", Description: "Review an implementation.",
			Body: "internal instructions must never enter Journal",
		}}},
	)

	require.Equal(t, []string{"skills.loaded"}, events.eventTypes())
	require.Empty(t, repo.snapshots)
}

func TestADKSkillBackendPublishesOnlySelectedSkillSnapshot(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &entity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-skills", Status: entity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
	}
	service.JournalSnapshotAttemptReader = repo
	events := &recordingRunEventSink{}
	run := &RunSummary{RunID: 10, ThreadID: 1, SpaceID: 10, CreatorID: 9}
	tracker, err := NewADKParityStateTracker(run, nil)
	require.NoError(t, err)
	require.NoError(t, tracker.ReplaceTodos([]ADKParityTodo{{
		ID: "plan-1", Title: "核验实现", Status: "in_progress",
	}}))
	ctx := withADKParityStateTracker(context.Background(), tracker)
	backend, err := newADKSkillBackend(
		[]AgentSkill{
			{ID: 7, Name: "review", Description: "Review an implementation.", Body: "review body"},
			{ID: 8, Name: "document", Description: "Prepare a document.", Body: "document body"},
		},
		ADKContextBudget{SkillCatalogTokens: 200, SkillContentTokens: 200},
		WithADKSkillBackendJournal(run, events, service),
	)
	require.NoError(t, err)

	_, err = backend.Get(ctx, "review")

	require.NoError(t, err)
	require.Equal(t, []string{"skill.started"}, events.eventTypes())
	require.Len(t, repo.snapshots, 1)
	for _, snapshot := range repo.snapshots {
		require.Equal(t, entity.JournalSnapshotContentTypeSkill, snapshot.ContentType)
		require.NotContains(t, snapshot.ContentJSON, "internal instructions")
		var content JournalTypedSnapshotContent
		require.NoError(t, json.Unmarshal([]byte(snapshot.ContentJSON), &content))
		require.Equal(t, []JournalSkillSummary{{
			SkillID: "7", Name: "review", Description: "Review an implementation.",
		}}, content.Skill.Skills)
		event := repo.events[snapshot.EventID]
		require.NotNil(t, event)
		require.Equal(t, "review", event.Target)
		require.Equal(t,
			journalStableProjectionID(10, "milestone", "plan-1"),
			event.Milestone,
		)
	}
}

func TestOutputSnapshotKeepsTheBoundPlanMilestone(t *testing.T) {
	service, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &entity.RunAttempt{
		ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
		AttemptID: "att-output", Status: entity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: entity.JournalProjectionStateHealthy,
	}
	service.JournalSnapshotAttemptReader = repo
	run := &RunSummary{RunID: 10, ThreadID: 1, SpaceID: 10, CreatorID: 9}
	tracker, err := NewADKParityStateTracker(run, nil)
	require.NoError(t, err)
	require.NoError(t, tracker.ReplaceTodos([]ADKParityTodo{{
		ID: "plan-output", Title: "生成验收文档", Status: "in_progress",
	}}))
	ctx := withADKParityStateTracker(context.Background(), tracker)

	service.publishJournalDocumentSnapshot(
		ctx,
		run,
		"call-write-output",
		&OutputFileSummary{
			FileID: 7, FileName: "report.md",
			VirtualPath: "/mnt/user-data/outputs/report.md",
			ContentType: "text/markdown",
			Digest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		"",
		[]byte("# report"),
	)

	require.Len(t, repo.events, 1)
	started, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: 1, RunID: 10, EventType: "message.completed",
		Payload: `{"role":"assistant","plan_task_id":"plan-output","tool_calls":[{"id":"call-write-output","function":{"name":"write_file"}}]}`,
	})
	require.NoError(t, err)
	require.NotNil(t, started)
	for _, event := range repo.events {
		require.Equal(t, started.ActionID, event.ActionID)
		require.Equal(t, started.Operation, event.Operation)
		require.Equal(t, started.Target, event.Target)
		require.Equal(t,
			started.Milestone,
			event.Milestone,
		)
	}
	for _, snapshot := range repo.snapshots {
		var content JournalTypedSnapshotContent
		require.NoError(t, json.Unmarshal([]byte(snapshot.ContentJSON), &content))
		require.NotNil(t, content.Document)
		require.Equal(t, "report.md", content.Document.Title)
	}
}

func journalSnapshotTestSubmission(req JournalContentSubmission) JournalContentSubmission {
	if req.Action.ActionID != "" {
		return req
	}
	target := string(req.ContentType)
	if target == "" {
		target = "snapshot"
	}
	req.Action = JournalContentAction{
		ActionID:             "action-" + journalSnapshotHash([]byte(req.IdempotencyKey))[:16],
		Operation:            "test",
		Target:               target,
		DisplayVerbRunning:   "正在处理 " + target,
		DisplayVerbCompleted: "已处理 " + target,
	}
	return req
}

func newJournalSnapshotApplicationTestService() (
	*ApplicationService,
	*journalSnapshotRepositoryStub,
	*journalSnapshotObjectStorageStub,
) {
	repo := &journalSnapshotRepositoryStub{
		snapshots: map[string]*entity.JournalContentSnapshot{},
		events:    map[int64]*entity.JournalEvent{}, auditByKey: map[string]*entity.JournalSnapshotAccessAudit{},
		reservations: map[string]*entity.JournalSnapshotReservation{},
	}
	objects := &journalSnapshotObjectStorageStub{objects: map[string][]byte{}}
	return &ApplicationService{
		JournalSnapshotRepository:       repo,
		JournalSnapshotObjectStorage:    objects,
		JournalSnapshotAuthorizer:       &journalSnapshotAuthorizerStub{allowed: true},
		JournalBrowserRedactionVerifier: &journalBrowserRedactionVerifierStub{allowed: true},
		JournalSnapshotIDGenerator:      &journalSnapshotIDGeneratorStub{next: 500},
		JournalSnapshotNow:              func() int64 { return 1_000 },
	}, repo, objects
}

type journalSnapshotRepositoryStub struct {
	snapshots     map[string]*entity.JournalContentSnapshot
	events        map[int64]*entity.JournalEvent
	audits        []*entity.JournalSnapshotAccessAudit
	auditByKey    map[string]*entity.JournalSnapshotAccessAudit
	reservations  map[string]*entity.JournalSnapshotReservation
	createErr     error
	reserveErr    error
	auditErr      error
	activeAttempt *entity.RunAttempt
	attemptErr    error
}

func (r *journalSnapshotRepositoryStub) GetActiveJournalAttempt(
	_ context.Context,
	_ int64,
) (*entity.RunAttempt, error) {
	if r.attemptErr != nil || r.activeAttempt == nil {
		return nil, r.attemptErr
	}
	clone := *r.activeAttempt
	return &clone, nil
}

func (r *journalSnapshotRepositoryStub) ReserveJournalSnapshot(
	_ context.Context,
	req domainrepo.ReserveJournalSnapshotRequest,
) (*domainrepo.ReserveJournalSnapshotResult, error) {
	if r.reserveErr != nil {
		return nil, r.reserveErr
	}
	if existing := r.snapshots[req.Reservation.SnapshotID]; existing != nil {
		return &domainrepo.ReserveJournalSnapshotResult{
			Snapshot: existing, Event: r.events[existing.EventID], Replayed: true,
		}, nil
	}
	if existing := r.reservations[req.Reservation.SnapshotID]; existing != nil &&
		existing.ExpiresAt > req.Now {
		clone := *existing
		return &domainrepo.ReserveJournalSnapshotResult{Reservation: &clone}, nil
	}
	clone := *req.Reservation
	clone.JournalRunID = clone.RunID
	if r.activeAttempt != nil && r.activeAttempt.JournalRunID > 0 {
		clone.JournalRunID = r.activeAttempt.JournalRunID
	}
	r.reservations[clone.SnapshotID] = &clone
	return &domainrepo.ReserveJournalSnapshotResult{Reservation: &clone}, nil
}

func (r *journalSnapshotRepositoryStub) DeleteExpiredJournalSnapshotReservations(
	_ context.Context,
	spaceID int64,
	now int64,
	limit int,
) (int64, error) {
	var deleted int64
	for snapshotID, reservation := range r.reservations {
		if deleted >= int64(limit) {
			break
		}
		if reservation.SpaceID == spaceID && reservation.ExpiresAt <= now {
			delete(r.reservations, snapshotID)
			deleted++
		}
	}
	return deleted, nil
}

func (r *journalSnapshotRepositoryStub) CreateJournalSnapshot(
	_ context.Context,
	req domainrepo.CreateJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, *entity.JournalEvent, bool, error) {
	if r.createErr != nil {
		return nil, nil, false, r.createErr
	}
	if existing := r.snapshots[req.Snapshot.SnapshotID]; existing != nil {
		return existing, r.events[existing.EventID], true, nil
	}
	snapshot := *req.Snapshot
	if reservation := r.reservations[snapshot.SnapshotID]; reservation != nil {
		snapshot.JournalRunID = reservation.JournalRunID
	}
	snapshot.Fragments = req.Fragments
	event := *req.Event
	event.JournalRunID = snapshot.JournalRunID
	r.snapshots[snapshot.SnapshotID] = &snapshot
	r.events[event.ID] = &event
	delete(r.reservations, snapshot.SnapshotID)
	return &snapshot, &event, false, nil
}

func (r *journalSnapshotRepositoryStub) GetJournalSnapshot(
	_ context.Context,
	req domainrepo.GetJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, error) {
	snapshot := r.snapshots[req.SnapshotID]
	if snapshot == nil || snapshot.SpaceID != req.SpaceID ||
		snapshot.ThreadID != req.ThreadID || snapshot.RunID != req.RunID {
		return nil, domainrepo.ErrJournalSnapshotNotFound
	}
	clone := *snapshot
	return &clone, nil
}

func (r *journalSnapshotRepositoryStub) ListJournalSnapshotFragments(
	_ context.Context,
	req domainrepo.ListJournalSnapshotFragmentsRequest,
) (*domainrepo.ListJournalSnapshotFragmentsResult, error) {
	snapshot := r.snapshots[req.SnapshotID]
	if snapshot == nil || snapshot.SpaceID != req.SpaceID {
		return nil, domainrepo.ErrJournalSnapshotNotFound
	}
	result := &domainrepo.ListJournalSnapshotFragmentsResult{NextIndex: req.AfterIndex}
	for _, fragment := range snapshot.Fragments {
		if fragment.FragmentIndex <= req.AfterIndex {
			continue
		}
		if len(result.Fragments) >= req.Limit {
			result.HasMore = true
			break
		}
		clone := *fragment
		result.Fragments = append(result.Fragments, &clone)
		result.NextIndex = clone.FragmentIndex
	}
	return result, nil
}

func (r *journalSnapshotRepositoryStub) RecordJournalSnapshotAccess(
	_ context.Context,
	audit *entity.JournalSnapshotAccessAudit,
) (*entity.JournalSnapshotAccessAudit, bool, error) {
	if r.auditErr != nil {
		return nil, false, r.auditErr
	}
	key := strings.Join([]string{
		audit.SnapshotID, string(audit.Action), strconv.FormatInt(audit.ActorID, 10),
		audit.IdempotencyKey,
	}, ":")
	if existing := r.auditByKey[key]; existing != nil {
		return existing, true, nil
	}
	clone := *audit
	clone.ID = int64(len(r.audits) + 1)
	r.audits = append(r.audits, &clone)
	r.auditByKey[key] = &clone
	return &clone, false, nil
}

type journalSnapshotRuntimeFileReaderStub struct {
	file  *entity.AgentFile
	err   error
	calls int
}

type journalSnapshotArtifactReaderStub struct {
	artifact *entity.AgentArtifact
	err      error
	calls    int
}

func (r *journalSnapshotArtifactReaderStub) GetArtifact(
	_ context.Context,
	_, _ int64,
) (*entity.AgentArtifact, error) {
	r.calls++
	if r.err != nil || r.artifact == nil {
		return nil, r.err
	}
	clone := *r.artifact
	return &clone, nil
}

type journalSnapshotArtifactAuthorizerStub struct {
	allowed bool
}

type journalSnapshotArtifactCapabilityIssuerStub struct {
	response *CreateArtifactSignedURLResponse
	err      error
	calls    int
}

func (i *journalSnapshotArtifactCapabilityIssuerStub) CreateArtifactSignedURL(
	_ context.Context,
	_ *CreateArtifactSignedURLRequest,
) (*CreateArtifactSignedURLResponse, error) {
	i.calls++
	return i.response, i.err
}

func (a *journalSnapshotArtifactAuthorizerStub) AuthorizeArtifactAccess(
	_ context.Context,
	_ ArtifactAccessRequest,
) error {
	if a.allowed {
		return nil
	}
	return ErrArtifactAccessDenied
}

func (r *journalSnapshotRuntimeFileReaderStub) GetFileByID(
	_ context.Context,
	_ int64,
) (*entity.AgentFile, error) {
	r.calls++
	if r.err != nil || r.file == nil {
		return nil, r.err
	}
	clone := *r.file
	return &clone, nil
}

func (r *journalSnapshotRepositoryStub) IsJournalSnapshotObjectProtected(
	_ context.Context,
	spaceID int64,
	objectKey string,
	now int64,
) (bool, error) {
	for _, snapshot := range r.snapshots {
		if snapshot.SpaceID != spaceID {
			continue
		}
		if snapshot.ObjectKey == objectKey || snapshot.OriginalObjectKey == objectKey {
			return true, nil
		}
		for _, fragment := range snapshot.Fragments {
			if fragment.ObjectKey == objectKey {
				return true, nil
			}
		}
	}
	for _, reservation := range r.reservations {
		if reservation.SpaceID == spaceID && reservation.ExpiresAt > now &&
			strings.HasPrefix(objectKey, reservation.StagingPrefix) {
			return true, nil
		}
	}
	return false, nil
}

type journalSnapshotAuthorizerStub struct {
	allowed bool
	err     error
	calls   int
}

type journalBrowserRedactionVerifierStub struct {
	allowed   bool
	calls     int
	evidences []JournalBrowserRedactionEvidence
}

func (v *journalBrowserRedactionVerifierStub) VerifyJournalBrowserRedaction(
	_ context.Context,
	evidence JournalBrowserRedactionEvidence,
) error {
	v.calls++
	v.evidences = append(v.evidences, evidence)
	if v.allowed {
		return nil
	}
	return errors.New("redaction evidence rejected")
}

func (a *journalSnapshotAuthorizerStub) AuthorizeJournalSnapshotAccess(
	_ context.Context,
	_ JournalSnapshotAccessRequest,
) error {
	a.calls++
	if a.err != nil {
		return a.err
	}
	if a.allowed {
		return nil
	}
	return ErrJournalSnapshotNoPermission
}

type journalSnapshotIDGeneratorStub struct {
	next int64
}

func (g *journalSnapshotIDGeneratorStub) GenID(context.Context) (int64, error) {
	g.next++
	return g.next, nil
}

func (g *journalSnapshotIDGeneratorStub) GenMultiIDs(
	ctx context.Context,
	count int,
) ([]int64, error) {
	ids := make([]int64, count)
	for i := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

type journalSnapshotObjectStorageStub struct {
	objects    map[string][]byte
	putKeys    []string
	getCalls   int
	openReader io.ReadCloser
	listCalls  int
}

func (s *journalSnapshotObjectStorageStub) PutJournalSnapshotObject(
	_ context.Context,
	key string,
	content []byte,
) error {
	s.objects[key] = append([]byte(nil), content...)
	s.putKeys = append(s.putKeys, key)
	return nil
}

func (s *journalSnapshotObjectStorageStub) OpenJournalSnapshotObject(
	_ context.Context,
	key string,
) (io.ReadCloser, error) {
	s.getCalls++
	if s.openReader != nil {
		return s.openReader, nil
	}
	content, ok := s.objects[key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

type journalSnapshotCountingReadCloser struct {
	*bytes.Reader
	bytesRead int
	closed    bool
}

func (r *journalSnapshotCountingReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytesRead += n
	return n, err
}

func (r *journalSnapshotCountingReadCloser) Close() error {
	r.closed = true
	return nil
}

func (s *journalSnapshotObjectStorageStub) DeleteJournalSnapshotObject(
	_ context.Context,
	key string,
) error {
	delete(s.objects, key)
	return nil
}

func (s *journalSnapshotObjectStorageStub) ListJournalSnapshotObjects(
	_ context.Context,
	prefix string,
	cursor string,
	limit int,
) ([]JournalSnapshotObjectInfo, string, error) {
	s.listCalls++
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) && key > cursor {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}
	objects := make([]JournalSnapshotObjectInfo, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, JournalSnapshotObjectInfo{Key: key, LastModified: 1_000})
	}
	if hasMore {
		return objects, keys[len(keys)-1], nil
	}
	return objects, "", nil
}

func int32Pointer(value int32) *int32 {
	return &value
}

func journalSnapshotContentFixture(
	contentType entity.JournalSnapshotContentType,
) JournalTypedSnapshotContent {
	switch contentType {
	case entity.JournalSnapshotContentTypeDocument:
		return JournalTypedSnapshotContent{Document: &JournalDocumentContent{
			Title: "Review", Format: "markdown", Content: "Safe body",
		}}
	case entity.JournalSnapshotContentTypeTerminal:
		return JournalTypedSnapshotContent{Terminal: &JournalTerminalContent{
			Command: "go test", Stdout: "ok",
		}}
	case entity.JournalSnapshotContentTypeCode:
		return JournalTypedSnapshotContent{Code: &JournalCodeContent{
			Repository: "repo", Revision: "0123456", Path: "main.go",
			Language: "go", Content: "package main",
		}}
	case entity.JournalSnapshotContentTypeSkill:
		return JournalTypedSnapshotContent{Skill: &JournalSkillContent{
			Skills: []JournalSkillSummary{{SkillID: "planner", Name: "Planner"}},
		}}
	case entity.JournalSnapshotContentTypeBrowser:
		return JournalTypedSnapshotContent{Browser: &JournalBrowserContent{
			CaptureID: "capture", MIMEType: "image/png", Redacted: true,
			RedactionEvidenceID: "evidence-fixture", RedactionPolicyVersion: "v1",
			StaticSnapshot: append([]byte("\x89PNG\r\n\x1a\n"), []byte("image")...),
		}}
	default:
		return JournalTypedSnapshotContent{}
	}
}
