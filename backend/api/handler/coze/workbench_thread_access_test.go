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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func mustMarshalJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func authenticatedAgentThreadTestServer() *server.Hertz {
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(2))
	h.Use(func(ctx context.Context, c *app.RequestContext) {
		if len(c.GetHeader(canonicalSpaceIDHeader)) == 0 {
			c.Request.Header.Set(canonicalSpaceIDHeader, "1001")
		}
		c.Next(ctx)
	})
	return h
}

func installAgentThreadTestService(t *testing.T) {
	t.Helper()
	prevThreadSVC := appagentthread.SVC.ThreadSVC
	prevThreadAuthorizer := appagentthread.SVC.ThreadAuthorizer
	prevWorkspaceAuthorizer := appagentthread.SVC.WorkspaceAuthorizer
	prevRuntimeFileSVC := appagentthread.SVC.RuntimeFileSVC
	prevPlanSVC := appagentthread.SVC.PlanSVC
	prevArtifactSVC := appagentthread.SVC.ArtifactSVC
	prevArtifactObjectStorage := appagentthread.SVC.ArtifactObjectStorage
	prevArtifactAuthorizer := appagentthread.SVC.ArtifactAuthorizer
	prevMemoryAuthorizer := appagentthread.SVC.MemoryAuthorizer
	prevGuardrailAuditRepository := appagentthread.SVC.GuardrailAuditRepository
	prevGuardrailAuditAuthorizer := appagentthread.SVC.GuardrailAuditAuthorizer
	prevMCPRuntimeAuditRepository := appagentthread.SVC.MCPRuntimeAuditRepository
	prevMCPRuntimeAuditAuthorizer := appagentthread.SVC.MCPRuntimeAuditAuthorizer
	prevArtifactScannerStatus := appagentthread.SVC.ArtifactScannerStatus
	prevArtifactReviewClock := appagentthread.SVC.ArtifactReviewClock
	prevJournalSnapshotRepository := appagentthread.SVC.JournalSnapshotRepository
	prevJournalQueryRepository := appagentthread.SVC.JournalQueryRepository
	prevJournalSnapshotAttemptReader := appagentthread.SVC.JournalSnapshotAttemptReader
	prevJournalSnapshotObjectStorage := appagentthread.SVC.JournalSnapshotObjectStorage
	prevJournalSnapshotAuthorizer := appagentthread.SVC.JournalSnapshotAuthorizer
	prevJournalSnapshotRuntimeFileReader := appagentthread.SVC.JournalSnapshotRuntimeFileReader
	prevJournalSnapshotArtifactReader := appagentthread.SVC.JournalSnapshotArtifactReader
	prevJournalSnapshotArtifactCapabilityIssuer := appagentthread.SVC.JournalSnapshotArtifactCapabilityIssuer
	prevJournalBrowserRedactionVerifier := appagentthread.SVC.JournalBrowserRedactionVerifier
	prevJournalSnapshotIDGenerator := appagentthread.SVC.JournalSnapshotIDGenerator
	prevJournalSnapshotNow := appagentthread.SVC.JournalSnapshotNow
	prevJournalUserSettingsStore := appagentthread.SVC.JournalUserSettingsStore
	prevJournalSettingsNow := appagentthread.SVC.JournalSettingsNow
	prevJournalAdmissionLimiter := appagentthread.SVC.JournalAdmissionLimiter
	prevJournalAdmissionRequired := appagentthread.SVC.JournalAdmissionRequired
	t.Cleanup(func() {
		appagentthread.SVC.ThreadSVC = prevThreadSVC
		appagentthread.SVC.ThreadAuthorizer = prevThreadAuthorizer
		appagentthread.SVC.WorkspaceAuthorizer = prevWorkspaceAuthorizer
		appagentthread.SVC.RuntimeFileSVC = prevRuntimeFileSVC
		appagentthread.SVC.PlanSVC = prevPlanSVC
		appagentthread.SVC.ArtifactSVC = prevArtifactSVC
		appagentthread.SVC.ArtifactObjectStorage = prevArtifactObjectStorage
		appagentthread.SVC.ArtifactAuthorizer = prevArtifactAuthorizer
		appagentthread.SVC.MemoryAuthorizer = prevMemoryAuthorizer
		appagentthread.SVC.GuardrailAuditRepository = prevGuardrailAuditRepository
		appagentthread.SVC.GuardrailAuditAuthorizer = prevGuardrailAuditAuthorizer
		appagentthread.SVC.MCPRuntimeAuditRepository = prevMCPRuntimeAuditRepository
		appagentthread.SVC.MCPRuntimeAuditAuthorizer = prevMCPRuntimeAuditAuthorizer
		appagentthread.SVC.ArtifactScannerStatus = prevArtifactScannerStatus
		appagentthread.SVC.ArtifactReviewClock = prevArtifactReviewClock
		appagentthread.SVC.JournalSnapshotRepository = prevJournalSnapshotRepository
		appagentthread.SVC.JournalQueryRepository = prevJournalQueryRepository
		appagentthread.SVC.JournalSnapshotAttemptReader = prevJournalSnapshotAttemptReader
		appagentthread.SVC.JournalSnapshotObjectStorage = prevJournalSnapshotObjectStorage
		appagentthread.SVC.JournalSnapshotAuthorizer = prevJournalSnapshotAuthorizer
		appagentthread.SVC.JournalSnapshotRuntimeFileReader = prevJournalSnapshotRuntimeFileReader
		appagentthread.SVC.JournalSnapshotArtifactReader = prevJournalSnapshotArtifactReader
		appagentthread.SVC.JournalSnapshotArtifactCapabilityIssuer = prevJournalSnapshotArtifactCapabilityIssuer
		appagentthread.SVC.JournalBrowserRedactionVerifier = prevJournalBrowserRedactionVerifier
		appagentthread.SVC.JournalSnapshotIDGenerator = prevJournalSnapshotIDGenerator
		appagentthread.SVC.JournalSnapshotNow = prevJournalSnapshotNow
		appagentthread.SVC.JournalUserSettingsStore = prevJournalUserSettingsStore
		appagentthread.SVC.JournalSettingsNow = prevJournalSettingsNow
		appagentthread.SVC.JournalAdmissionLimiter = prevJournalAdmissionLimiter
		appagentthread.SVC.JournalAdmissionRequired = prevJournalAdmissionRequired
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadHandlerTableForTest(db))
	appagentthread.InitService(&appagentthread.ServiceComponents{DB: db, IDGen: &sequentialIDGen{next: 1}})
	appagentthread.SVC.WorkspaceAuthorizer = allowWorkbenchWorkspaceAuthorizer{}
	appagentthread.SVC.ArtifactAuthorizer = nil
	appagentthread.SVC.MemoryAuthorizer = nil
	_, err = appagentthread.SVC.CreateThread(context.Background(), &appagentthread.CreateThreadRequest{
		SpaceID:  1,
		UserID:   2,
		Title:    "任务列表",
		Source:   appagentthread.ThreadSourceWeb,
		Metadata: `{"message":"hello"}`,
	})
	require.NoError(t, err)
}

type allowWorkbenchWorkspaceAuthorizer struct{}

func (allowWorkbenchWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	context.Context,
	appagentthread.WorkspaceAccessRequest,
) error {
	return nil
}

type recordingWorkbenchWorkspaceAuthorizer struct {
	req appagentthread.WorkspaceAccessRequest
	err error
}

func (a *recordingWorkbenchWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	_ context.Context,
	req appagentthread.WorkspaceAccessRequest,
) error {
	a.req = req
	return a.err
}

type countingWorkbenchWorkspaceAuthorizer struct {
	calls int
}

func (a *countingWorkbenchWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	context.Context,
	appagentthread.WorkspaceAccessRequest,
) error {
	a.calls++
	return nil
}

func workbenchSessionMiddlewareForTest(userID int64) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		ctx = ctxcache.Init(ctx)
		ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userentity.Session{
			UserID: userID,
		})
		c.Next(ctx)
	}
}

type recordingWorkbenchArtifactAuthorizer struct {
	req appagentthread.ArtifactAccessRequest
	err error
}

func (a *recordingWorkbenchArtifactAuthorizer) AuthorizeArtifactAccess(
	_ context.Context,
	req appagentthread.ArtifactAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchMemoryAuthorizer struct {
	req appagentthread.MemoryAccessRequest
	err error
}

func (a *recordingWorkbenchMemoryAuthorizer) AuthorizeMemoryAccess(
	_ context.Context,
	req appagentthread.MemoryAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchGuardrailAuditAuthorizer struct {
	req appagentthread.GuardrailAuditAccessRequest
	err error
}

func (a *recordingWorkbenchGuardrailAuditAuthorizer) AuthorizeGuardrailAuditAccess(
	_ context.Context,
	req appagentthread.GuardrailAuditAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchMCPRuntimeAuditAuthorizer struct {
	req appagentthread.MCPRuntimeAuditAccessRequest
	err error
}

func (a *recordingWorkbenchMCPRuntimeAuditAuthorizer) AuthorizeMCPRuntimeAuditAccess(
	_ context.Context,
	req appagentthread.MCPRuntimeAuditAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingWorkbenchArtifactStorage struct {
	objects                map[string][]byte
	signedURL              string
	getCalls               int
	openCalls              int
	signKey                string
	signExpire             int64
	signContentDisposition string
	signContentType        string
}

func (s *recordingWorkbenchArtifactStorage) GetObject(
	_ context.Context,
	objectKey string,
) ([]byte, error) {
	s.getCalls++
	content := s.objects[objectKey]
	return append([]byte(nil), content...), nil
}

func (s *recordingWorkbenchArtifactStorage) OpenObjectStream(
	_ context.Context,
	objectKey string,
) (io.ReadCloser, error) {
	s.openCalls++
	return io.NopCloser(bytes.NewReader(s.objects[objectKey])), nil
}

func (s *recordingWorkbenchArtifactStorage) GetObjectUrl(
	_ context.Context,
	objectKey string,
	opts ...storage.GetOptFn,
) (string, error) {
	s.signKey = objectKey
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}
	s.signExpire = option.Expire
	s.signContentDisposition = option.ResponseContentDisposition
	s.signContentType = option.ResponseContentType
	return s.signedURL, nil
}

func createInterruptedHumanInteractionRun(t *testing.T) int64 {
	t.Helper()
	runID, _ := createInterruptedHumanInteractionRunWithCheckpoint(t)

	return runID
}

func fencedRunStatusRequestForTest(
	t *testing.T,
	req *appagentthread.UpdateRunStatusRequest,
) *appagentthread.UpdateRunStatusRequest {
	t.Helper()
	require.NotNil(t, req)
	persisted, err := appagentthread.SVC.GetRun(context.Background(), &appagentthread.GetRunRequest{RunID: req.RunID})
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.NotNil(t, persisted.Run)
	req.LeaseOwner = persisted.Run.LeaseOwner
	req.LeaseToken = persisted.Run.LeaseToken
	req.ExecutionGeneration = persisted.Run.ExecutionGeneration
	return req
}

func createInterruptedHumanInteractionRunWithCheckpoint(t *testing.T) (int64, int64) {
	t.Helper()
	runResp, err := appagentthread.SVC.CreateRun(context.Background(), &appagentthread.CreateRunRequest{
		ThreadID:    1,
		AssistantID: "assistant-a",
		Input:       `{"messages":[{"role":"user","content":"请分析周报"}]}`,
		Config:      `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.ClaimPendingRuns(context.Background(), &appagentthread.ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    1,
	})
	require.NoError(t, err)
	_, err = appagentthread.SVC.InterruptRun(context.Background(), fencedRunStatusRequestForTest(t, &appagentthread.UpdateRunStatusRequest{
		RunID:    runResp.Run.RunID,
		From:     appagentthread.RunStatusRunning,
		WorkerID: "worker-a",
	}))
	require.NoError(t, err)

	envelope := appagentthread.ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         "eino_adk",
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
		Interrupts: map[string]appagentthread.ADKInterruptItem{
			"interrupt-1": {
				ID:          "interrupt-1",
				Address:     "lead/tool/ask_user_clarification",
				IsRootCause: true,
				Info: appagentthread.HumanInteractionPrompt{
					Schema:        "coze.human_interaction.v1",
					InteractionID: "hi_1",
					Kind:          appagentthread.HumanInteractionKindClarification,
					Question:      "请选择时间范围",
					Required:      true,
					AllowFreeText: true,
				},
			},
		},
	}
	raw, err := envelope.Marshal()
	require.NoError(t, err)
	checkpointResp, err := appagentthread.SVC.CreateCheckpoint(context.Background(), &appagentthread.CreateCheckpointRequest{
		ThreadID:        1,
		RunID:           runResp.Run.RunID,
		CheckpointNS:    "eino.adk",
		RuntimeType:     "eino_adk",
		RuntimeKey:      "checkpoint-1",
		EnvelopeVersion: 1,
		ChannelValues:   string(raw),
		ChannelVersions: `{}`,
		PendingSends:    `[]`,
		Metadata:        `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)

	return runResp.Run.RunID, checkpointResp.Checkpoint.CheckpointID
}

func migrateAgentThreadHandlerTableForTest(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE agent_threads (
			id integer PRIMARY KEY,
			space_id integer,
			creator_id integer,
			agent_id integer,
			title text,
			status text,
			source text,
			metadata json,
			created_at integer,
			updated_at integer,
			last_message_at integer
		);
		CREATE TABLE agent_thread_messages (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			role text,
			content text,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_runs (
			id integer PRIMARY KEY,
			thread_id integer,
			parent_run_id integer DEFAULT 0,
			space_id integer,
			creator_id integer,
			assistant_id text,
			run_kind text DEFAULT 'task',
			status text,
			command json,
			input json,
			config json,
			context json,
			metadata json,
			stream_mode json,
			multitask_strategy text,
			on_disconnect text,
			durability text,
			idempotency_key text,
			worker_id text,
			lease_owner text,
			lease_token text,
			lease_expires_at integer,
			heartbeat_at integer,
			cancel_requested_at integer,
			execution_generation integer NOT NULL DEFAULT 0,
			error_code text,
			error_message text,
			started_at integer,
			ended_at integer,
			created_at integer,
			updated_at integer
		);
		CREATE TABLE agent_run_events (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			journal_run_id integer,
			attempt_id text,
			sequence integer,
			idempotency_key text,
			parent_event_id integer,
			schema_version text,
			status text,
			occurred_at_unix_nano integer,
			visibility text,
			payload_version text,
			snapshot_id text,
			trace_id text,
			action_id text,
			phase text,
			operation text,
			target text,
			milestone text,
			event_type text,
			journal_event_type text,
			payload json,
			journal_payload json,
			created_at integer
		);
		CREATE TABLE agent_run_attempts (
			id integer PRIMARY KEY,
			thread_id integer NOT NULL,
			journal_run_id integer NOT NULL,
			execution_run_id integer NOT NULL,
			attempt_id text NOT NULL,
			ordinal integer NOT NULL,
			status text NOT NULL,
			active_slot integer,
			next_sequence integer NOT NULL DEFAULT 1,
			last_committed_sequence integer NOT NULL DEFAULT 0,
			source_checkpoint_id integer,
			source_attempt_id text,
			recovery_idempotency_key text,
			enrollment_version text NOT NULL,
			snapshots_enabled integer NOT NULL DEFAULT 0,
			projection_state text NOT NULL DEFAULT 'healthy',
			projection_degraded_at integer,
			trace_id text,
			terminal_event_id integer,
			created_at integer NOT NULL,
			updated_at integer NOT NULL,
			started_at integer,
			ended_at integer
		);
		CREATE TABLE agent_journal_snapshots (
			snapshot_id text PRIMARY KEY,
			space_id integer NOT NULL,
			thread_id integer NOT NULL,
			run_id integer NOT NULL,
			journal_run_id integer NOT NULL,
			attempt_id text NOT NULL,
			event_id integer NOT NULL,
			action_id text NOT NULL,
			revision integer NOT NULL,
			content_type text NOT NULL,
			status text NOT NULL,
			is_fragmented integer NOT NULL,
			fragment_count integer NOT NULL,
			visibility text NOT NULL,
			error_code text,
			mime_type text NOT NULL,
			encoding text NOT NULL,
			compression text NOT NULL,
			content_json blob,
			object_key text,
			summary_json blob,
			summary_hash text,
			content_length integer NOT NULL,
			content_hash text NOT NULL,
			acl_domain text NOT NULL,
			source_resource_type text,
			source_resource_id text,
			source_revision text,
			original_object_key text,
			expires_at integer NOT NULL,
			cleanup_state text NOT NULL,
			deleted_at integer,
			created_at integer NOT NULL
		);
		CREATE TABLE agent_journal_snapshot_fragments (
			fragment_id text PRIMARY KEY,
			snapshot_id text NOT NULL,
			fragment_index integer NOT NULL,
			kind text NOT NULL,
			metadata_json blob,
			mime_type text,
			inline_content blob,
			object_key text,
			byte_start integer NOT NULL,
			byte_end integer NOT NULL,
			size_bytes integer NOT NULL,
			content_hash text NOT NULL,
			created_at integer NOT NULL
		);
		CREATE TABLE agent_journal_snapshot_access_audits (
			id integer PRIMARY KEY AUTOINCREMENT,
			space_id integer NOT NULL,
			thread_id integer NOT NULL,
			run_id integer NOT NULL,
			attempt_id text,
			snapshot_id text NOT NULL,
			content_type text,
			action text NOT NULL,
			actor_id integer NOT NULL,
			permission_result text NOT NULL,
			idempotency_key text NOT NULL,
			target_hash text NOT NULL,
			trace_id text,
			created_at integer NOT NULL
		);
		CREATE TABLE agent_checkpoints (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			parent_checkpoint_id integer,
			checkpoint_ns text,
			runtime_type text DEFAULT 'legacy',
			runtime_key text DEFAULT '',
			envelope_version integer DEFAULT 0,
			runtime_deleted_at integer DEFAULT 0,
			channel_values json,
			channel_versions json,
			pending_sends json,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_thread_memories (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			scope text,
			content text,
			metadata json,
			score real,
			confidence real,
			source_type text,
			source_id text,
			correction_of_memory_id integer,
			corrected_at integer,
			expires_at integer,
			created_at integer,
			updated_at integer,
			deleted_at integer DEFAULT 0
		);
		CREATE TABLE agent_memory_audit_events (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			memory_id integer,
			actor_id integer,
			event_type text,
			scope text,
			source_type text,
			source_id text,
			affected_count integer,
			created_at integer
		);
		CREATE TABLE agent_guardrail_audit_events (
			id integer PRIMARY KEY,
			space_id integer,
			thread_id integer,
			run_id integer,
			actor_id integer,
			event_type text,
			target_type text,
			target_id text,
			operation text,
			source text,
			action text,
			fail_mode text,
			provider text,
			reason_code text,
			rule_ids text,
			created_at integer
		);
		CREATE TABLE agent_mcp_runtime_audit_events (
			id integer PRIMARY KEY,
			space_id integer,
			thread_id integer,
			run_id integer,
			server_id integer,
			runtime_tool_name text,
			event_type text,
			error_code text,
			elapsed_ms integer,
			output_bytes integer,
			created_at integer
		);
		CREATE TABLE agent_token_usage (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			source text,
			step_id text,
			step_index integer,
			step_name text,
			model_name text,
			provider text,
			input_tokens integer,
			output_tokens integer,
			total_tokens integer,
			cost_micros integer,
			currency text,
			estimated boolean,
			raw_usage json,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_files (
			id integer PRIMARY KEY,
			space_id integer,
			user_id integer,
			thread_id integer,
			run_id integer,
			file_name text,
			original_file_name text DEFAULT '',
				file_kind text,
				virtual_path text,
				virtual_path_hash text DEFAULT '',
				object_uri text,
				content_type text DEFAULT '',
			size_bytes integer DEFAULT 0,
			digest text DEFAULT '',
			status text DEFAULT 'active',
			metadata json,
			created_at integer,
			updated_at integer,
				UNIQUE (run_id, virtual_path_hash)
			);
		CREATE TABLE agent_artifacts (
			id integer PRIMARY KEY,
			space_id integer,
			user_id integer,
			thread_id integer,
			run_id integer,
			journal_run_id integer,
			file_id integer UNIQUE,
			title text DEFAULT '',
			artifact_type text,
			virtual_path text,
			object_uri text,
			content_type text DEFAULT '',
			size_bytes integer DEFAULT 0,
			preview_mode text DEFAULT 'download',
			source text DEFAULT 'agent_generated',
			generation_status text DEFAULT 'processing',
			primary_slot integer,
			collection_id text,
			collection_order integer,
			detected_content_type text,
			scanned_size_bytes integer,
			content_hash text,
			metadata json,
			created_at integer,
			updated_at integer,
			deleted_at integer DEFAULT 0,
			UNIQUE (journal_run_id, primary_slot),
			UNIQUE (journal_run_id, collection_id, collection_order)
		);
		CREATE TABLE agent_artifact_scan_jobs (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			user_id integer,
			artifact_id integer,
			file_id integer,
			scanner text,
			idempotency_key text,
			status text,
			worker_id text DEFAULT '',
			attempt_count integer DEFAULT 0,
			last_error text DEFAULT '',
			available_at integer DEFAULT 0,
			lease_expires_at integer DEFAULT 0,
			started_at integer DEFAULT 0,
			ended_at integer DEFAULT 0,
			created_at integer,
			updated_at integer,
			UNIQUE (artifact_id, idempotency_key)
		);
		CREATE TABLE agent_transcript_snapshots (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			kind text,
			digest text,
			idempotency_key text,
			message_count integer,
			messages json,
			metadata json,
			created_at integer
		);
		CREATE TABLE agent_memory_flush_jobs (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			user_id integer,
			assistant_id text,
			transcript_snapshot_id integer,
			idempotency_key text,
			status text,
			attempt_count integer,
			worker_id text,
			last_error text,
			available_at integer,
			lease_expires_at integer,
			started_at integer,
			ended_at integer,
			created_at integer,
			updated_at integer
		);
		CREATE TABLE agent_run_plans (
			run_id integer PRIMARY KEY,
			thread_id integer,
			space_id integer,
			user_id integer,
			high_watermark integer,
			revision integer,
			created_at integer,
			updated_at integer
		);
		CREATE TABLE agent_run_plan_items (
			id integer PRIMARY KEY,
			run_id integer,
			task_id integer,
			subject text,
			description text,
			status text,
			active_form text,
			owner text,
			blocks json,
			blocked_by json,
			metadata json,
			active boolean,
			version integer,
			created_at integer,
			updated_at integer
		)
	`).Error
}

type sequentialIDGen struct {
	mu   sync.Mutex
	next int64
}

type recordingTaskThreadRunEventStreamWriter struct {
	buffer bytes.Buffer
	ids    []string
}

type failingTaskThreadRunEventStreamWriter struct{}

func (failingTaskThreadRunEventStreamWriter) WriteEvent(string, string, []byte) error {
	return errors.New("stream disconnected")
}

func (failingTaskThreadRunEventStreamWriter) WriteKeepAlive() error {
	return errors.New("stream disconnected")
}

type heartbeatFailingTaskThreadRunEventStreamWriter struct{}

func (heartbeatFailingTaskThreadRunEventStreamWriter) WriteEvent(string, string, []byte) error {
	return nil
}

func (heartbeatFailingTaskThreadRunEventStreamWriter) WriteKeepAlive() error {
	return errors.New("stream disconnected")
}

func publicRunEventPayloadForTest(eventType, payload string) string {
	projected := appagentthread.ProjectPublicRunEvent(&appagentthread.RunEventSummary{
		EventType: eventType,
		Payload:   payload,
	})
	if projected == nil {
		return `{}`
	}
	return projected.Payload
}

func (w *recordingTaskThreadRunEventStreamWriter) WriteEvent(id, eventType string, data []byte) error {
	if id != "" {
		w.ids = append(w.ids, id)
		w.buffer.WriteString("id: ")
		w.buffer.WriteString(id)
		w.buffer.WriteByte('\n')
	}
	if eventType != "" {
		w.buffer.WriteString("event: ")
		w.buffer.WriteString(eventType)
		w.buffer.WriteByte('\n')
	}
	if len(data) > 0 {
		w.buffer.WriteString("data: ")
		w.buffer.Write(data)
		w.buffer.WriteByte('\n')
	}
	w.buffer.WriteByte('\n')

	return nil
}

func (w *recordingTaskThreadRunEventStreamWriter) WriteKeepAlive() error {
	return nil
}

func (w *recordingTaskThreadRunEventStreamWriter) String() string {
	return w.buffer.String()
}

func (g *sequentialIDGen) GenID(ctx context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.next <= 0 {
		g.next = 2
		return 1, nil
	}

	id := g.next
	g.next++
	return id, nil
}

func (g *sequentialIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}

	return ids, nil
}

func TestWorkbenchRunTerminalStatuses(t *testing.T) {
	require.True(t, isWorkbenchRunTerminal(appagentthread.RunStatusSucceeded))
	require.True(t, isWorkbenchRunTerminal(appagentthread.RunStatusFailed))
	require.True(t, isWorkbenchRunTerminal(appagentthread.RunStatusInterrupted))
	require.True(t, isWorkbenchRunTerminal(appagentthread.RunStatusCanceled))
	require.False(t, isWorkbenchRunTerminal(appagentthread.RunStatusPending))
	require.False(t, isWorkbenchRunTerminal(appagentthread.RunStatusRunning))
}
