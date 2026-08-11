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
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/cloudwego/hertz/pkg/app"
	hertzconsts "github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/stretchr/testify/require"

	journalcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/journal_contract"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	projectconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestCanonicalDecodeRejectsUnknownField(t *testing.T) {
	type request struct {
		AssistantID string `json:"assistant_id"`
	}

	var c app.RequestContext
	c.Request.SetBody([]byte(`{"assistant_id":"agent","checkpoint_during":"secret-payload"}`))
	var req request
	public := decodeCanonicalJSON(&c, &req)

	require.NotNil(t, public)
	require.Equal(t, hertzconsts.StatusUnprocessableEntity, public.status)
	require.Equal(t, "unsupported_sdk_field", public.Code)
	require.Equal(t, "Unsupported field: checkpoint_during", public.Detail)
	require.NotContains(t, public.Detail, "secret-payload")
}

func TestCanonicalDecodeAcceptsSingleObject(t *testing.T) {
	type request struct {
		AssistantID string `json:"assistant_id"`
	}

	var c app.RequestContext
	c.Request.SetBody([]byte(`{"assistant_id":"agent"}`))
	var req request
	require.Nil(t, decodeCanonicalJSON(&c, &req))
	require.Equal(t, "agent", req.AssistantID)
}

func TestCanonicalDecodeRejectsTrailingJSON(t *testing.T) {
	type request struct {
		AssistantID string `json:"assistant_id"`
	}

	for _, body := range []string{
		`{"assistant_id":"agent"}{"assistant_id":"second"}`,
		`{"assistant_id":"agent"} trailing`,
		``,
		`[]`,
		`null`,
	} {
		body := body
		t.Run(body, func(t *testing.T) {
			var c app.RequestContext
			c.Request.SetBody([]byte(body))
			var req request
			public := decodeCanonicalJSON(&c, &req)

			require.NotNil(t, public)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_json", public.Code)
		})
	}
}

func TestCanonicalParseIDRejectsZeroNegativeAndNonDecimal(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "+1", " 1", "1.0", "0x10", "abc"} {
		value := value
		t.Run("path_"+value, func(t *testing.T) {
			c := app.NewContext(1)
			c.Params = param.Params{{Key: "thread_id", Value: value}}

			id, public := canonicalPathID(c, "thread_id")
			require.Zero(t, id)
			require.NotNil(t, public)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_path_parameter", public.Code)
		})
	}

	c := app.NewContext(1)
	c.Params = param.Params{{Key: "thread_id", Value: "42"}}
	id, public := canonicalPathID(c, "thread_id")
	require.Nil(t, public)
	require.Equal(t, int64(42), id)

	var query app.RequestContext
	require.Nil(t, canonicalQueryValue(t, &query, "run_id", nil))
	query.Request.SetRequestURI("/?run_id=84")
	require.Equal(t, int64(84), *canonicalQueryValue(t, &query, "run_id", nil))
	query.Request.SetRequestURI("/?run_id=0")
	canonicalQueryValue(t, &query, "run_id", func(public *canonicalError) {
		require.Equal(t, hertzconsts.StatusBadRequest, public.status)
		require.Equal(t, "invalid_query_parameter", public.Code)
	})
}

func TestCanonicalSpaceIDRequiresAuthorizedHeader(t *testing.T) {
	previous := appagentthread.SVC.WorkspaceAuthorizer
	t.Cleanup(func() { appagentthread.SVC.WorkspaceAuthorizer = previous })

	t.Run("missing header", func(t *testing.T) {
		var c app.RequestContext
		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusBadRequest, public.status)
		require.Equal(t, "invalid_space_id", public.Code)
	})

	for _, value := range []string{"0", "-1", "+1", " 1001", "workspace"} {
		value := value
		t.Run("invalid header "+value, func(t *testing.T) {
			var c app.RequestContext
			c.Request.Header.Set("X-Coze-Space-ID", value)
			spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
			require.Zero(t, spaceID)
			require.Equal(t, hertzconsts.StatusBadRequest, public.status)
			require.Equal(t, "invalid_space_id", public.Code)
		})
	}

	t.Run("unauthenticated", func(t *testing.T) {
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")
		spaceID, public := canonicalSpaceID(context.Background(), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusUnauthorized, public.status)
		require.Equal(t, "unauthenticated", public.Code)
	})

	t.Run("authorized server side", func(t *testing.T) {
		authorizer := &canonicalRecordingWorkspaceAuthorizer{}
		appagentthread.SVC.WorkspaceAuthorizer = authorizer
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Nil(t, public)
		require.Equal(t, int64(1001), spaceID)
		require.Equal(t, 1, authorizer.calls)
		require.Equal(t, appagentthread.WorkspaceAccessRequest{
			ViewerID: 42,
			SpaceID:  1001,
		}, authorizer.req)
	})

	t.Run("denied space is masked", func(t *testing.T) {
		appagentthread.SVC.WorkspaceAuthorizer = &canonicalRecordingWorkspaceAuthorizer{
			err: appagentthread.ErrThreadAccessDenied,
		}
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusNotFound, public.status)
		require.Equal(t, "workspace_not_found", public.Code)
	})

	t.Run("authorization dependency fails closed", func(t *testing.T) {
		appagentthread.SVC.WorkspaceAuthorizer = &canonicalRecordingWorkspaceAuthorizer{
			err: appagentthread.ErrThreadAuthorizationUnavailable,
		}
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusServiceUnavailable, public.status)
		require.Equal(t, "dependency_unavailable", public.Code)
		require.True(t, public.Retryable)
	})

	t.Run("missing authorizer fails closed", func(t *testing.T) {
		appagentthread.SVC.WorkspaceAuthorizer = nil
		var c app.RequestContext
		c.Request.Header.Set("X-Coze-Space-ID", "1001")

		spaceID, public := canonicalSpaceID(canonicalViewerContext(42), &c)
		require.Zero(t, spaceID)
		require.Equal(t, hertzconsts.StatusServiceUnavailable, public.status)
		require.Equal(t, "dependency_unavailable", public.Code)
		require.True(t, public.Retryable)
	})
}

func TestCanonicalErrorNeverLeaksCauseOrPayload(t *testing.T) {
	ctx := context.WithValue(context.Background(), projectconsts.CtxLogIDKey, "trace-test")
	var c app.RequestContext
	writeCanonicalError(ctx, &c, hertzconsts.StatusUnprocessableEntity, canonicalError{
		Detail: "Unsupported field: checkpoint_during",
		Code:   "unsupported_sdk_field",
	})

	require.Equal(t, hertzconsts.StatusUnprocessableEntity, c.Response.StatusCode())
	require.Equal(t,
		`{"detail":"Unsupported field: checkpoint_during","code":"unsupported_sdk_field","retryable":false,"trace_id":"trace-test"}`,
		string(c.Response.Body()),
	)

	mapped := mapCanonicalApplicationError(errors.New(
		"database failed with sk-secret and raw provider payload",
	))
	var internal app.RequestContext
	writeCanonicalError(ctx, &internal, mapped.status, mapped)
	body := string(internal.Response.Body())
	require.Equal(t, hertzconsts.StatusInternalServerError, internal.Response.StatusCode())
	require.NotContains(t, body, "sk-secret")
	require.NotContains(t, body, "provider payload")
	require.NotContains(t, body, "database failed")
}

func TestCanonicalJournalContractVersionsAndLegacyEvents(t *testing.T) {
	require.Equal(t, "1.1", journalcontract.JOURNALSCHEMAVERSION)
	require.Equal(t, "1.0", journalcontract.JOURNALPAYLOADVERSION)
	require.Equal(t, "1.1", journalcontract.JOURNALPROTOCOLVERSION)
	require.Equal(t, 0.40, journalcontract.JOURNALSPLITRATIOMIN)
	require.Equal(t, 0.70, journalcontract.JOURNALSPLITRATIOMAX)

	legacyJSON := []byte(`{
		"event_id":"101",
		"thread_id":"202",
		"run_id":"303",
		"event_type":"values",
		"payload":{"answer":"kept for legacy clients"},
		"created_at":"2026-07-30T14:32:10.123456789Z",
		"future_optional_field":"ignored"
	}`)
	event, err := parseCanonicalJournalEvent(legacyJSON)
	require.NoError(t, err)
	require.Equal(t, "101", event.EventID)
	require.Equal(t, "202", event.ThreadID)
	require.Equal(t, "303", event.RunID)
	require.Equal(t, "values", event.EventType)
	require.JSONEq(t, `{"answer":"kept for legacy clients"}`, event.Payload)
	require.Equal(t, "2026-07-30T14:32:10.123456789Z", event.CreatedAt)
	require.Nil(t, event.SchemaVersion)
	require.Nil(t, event.PayloadVersion)

	for _, field := range []string{"EventID", "ThreadID", "RunID"} {
		contractField, ok := reflect.TypeOf(*event).FieldByName(field)
		require.True(t, ok, field)
		require.Equal(t, reflect.String, contractField.Type.Kind(), field)
	}

	bootstrapRequestType := reflect.TypeOf(journalcontract.GetCanonicalRunJournalRequest{})
	afterEventIDField, ok := bootstrapRequestType.FieldByName("AfterEventID")
	require.True(t, ok, "bootstrap request must preserve the legacy event cursor")
	require.Equal(t, reflect.Int64, afterEventIDField.Type.Elem().Kind())
	require.Contains(t, afterEventIDField.Tag.Get("json"), ",string")
}

func TestCanonicalJournalContractTypedPayloadsAndSnapshots(t *testing.T) {
	action := journalcontract.JournalActionEventPayload{
		Type: journalcontract.JournalActionPayloadTypeTerminal,
		Data: &journalcontract.JournalActionEventData{
			ActionID:             "action-1",
			MilestoneID:          canonicalTestStringPointer("milestone-1"),
			Operation:            "read",
			Target:               "requirements.md",
			DisplayVerbRunning:   "正在读取 requirements.md",
			DisplayVerbCompleted: "已读取 requirements.md",
			ContentType:          canonicalTestStringPointer(journalcontract.JournalSnapshotContentTypeTerminal),
		},
	}
	encoded, err := json.Marshal(action)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"type":"terminal",
		"data":{
			"action_id":"action-1",
			"milestone_id":"milestone-1",
			"operation":"read",
			"target":"requirements.md",
			"display_verb_running":"正在读取 requirements.md",
			"display_verb_completed":"已读取 requirements.md",
			"content_type":"terminal"
		}
	}`, string(encoded))
	require.Nil(t, validateCanonicalJournalActionPayload(&action))

	mismatchedAction := action
	mismatchedAction.Data = &journalcontract.JournalActionEventData{
		ActionID:             "action-2",
		Operation:            "read",
		Target:               "requirements.md",
		DisplayVerbRunning:   "正在读取 requirements.md",
		DisplayVerbCompleted: "已读取 requirements.md",
		ContentType:          canonicalTestStringPointer(journalcontract.JournalSnapshotContentTypeDocument),
	}
	public := validateCanonicalJournalActionPayload(&mismatchedAction)
	require.NotNil(t, public)
	require.Equal(t, "SCHEMA_INCOMPATIBLE", public.Code)

	require.Nil(t, validateCanonicalJournalActionPayload(&journalcontract.JournalActionEventPayload{
		Type: journalcontract.JournalActionPayloadTypeGeneric,
		Data: &journalcontract.JournalActionEventData{
			ActionID:             "action-3",
			Operation:            "wait",
			Target:               "run",
			DisplayVerbRunning:   "正在等待任务",
			DisplayVerbCompleted: "已等待任务",
		},
	}))

	require.Equal(t, "milestone", journalcontract.JournalMilestonePayloadTypeMilestone)
	require.Equal(t, []journalcontract.JournalActionPayloadType{
		"generic", "document", "terminal", "code", "skill", "browser",
	}, []journalcontract.JournalActionPayloadType{
		journalcontract.JournalActionPayloadTypeGeneric,
		journalcontract.JournalActionPayloadTypeDocument,
		journalcontract.JournalActionPayloadTypeTerminal,
		journalcontract.JournalActionPayloadTypeCode,
		journalcontract.JournalActionPayloadTypeSkill,
		journalcontract.JournalActionPayloadTypeBrowser,
	})
	require.Equal(t, "artifact", journalcontract.JournalArtifactPayloadTypeArtifact)
	require.Equal(t, "verification", journalcontract.JournalVerificationPayloadTypeVerification)
	require.Equal(t, "confirmation", journalcontract.JournalConfirmationPayloadTypeConfirmation)

	typedPayloads := []any{
		journalcontract.JournalMilestoneEventPayload{},
		journalcontract.JournalActionEventPayload{},
		journalcontract.JournalArtifactEventPayload{},
		journalcontract.JournalVerificationEventPayload{},
		journalcontract.JournalConfirmationEventPayload{},
	}
	for _, payload := range typedPayloads {
		payloadType := reflect.TypeOf(payload)
		typeField, ok := payloadType.FieldByName("Type")
		require.True(t, ok, payloadType.Name())
		require.NotEqual(t, "JournalPayloadType", typeField.Type.Name(), payloadType.Name())
		dataField, ok := payloadType.FieldByName("Data")
		require.True(t, ok, payloadType.Name())
		require.NotEqual(t, reflect.Interface, dataField.Type.Kind(), payloadType.Name())
		require.NotEqual(t, reflect.Map, dataField.Type.Kind(), payloadType.Name())
	}

	stableFields := []struct {
		value  any
		fields []string
	}{
		{journalcontract.JournalMilestoneEventData{}, []string{
			"MilestoneID", "Title",
		}},
		{journalcontract.JournalActionEventData{}, []string{
			"ActionID", "MilestoneID", "Operation", "Target",
			"DisplayVerbRunning", "DisplayVerbCompleted", "ContentType",
		}},
		{journalcontract.JournalArtifactEventData{}, []string{
			"ArtifactID", "CollectionID",
		}},
		{journalcontract.JournalVerificationEventData{}, []string{
			"VerificationID", "Title", "ResultSummary",
		}},
		{journalcontract.JournalConfirmationEventData{}, []string{
			"ConfirmationID", "ConfirmationType", "Prompt", "AllowedActionKeys",
		}},
	}
	for _, contract := range stableFields {
		contractType := reflect.TypeOf(contract.value)
		for _, field := range contract.fields {
			_, ok := contractType.FieldByName(field)
			require.True(t, ok, "%s.%s", contractType.Name(), field)
		}
	}

	snapshotType := reflect.TypeOf(journalcontract.JournalSnapshotEnvelope{})
	for _, field := range []string{
		"ContentType", "SnapshotID", "EventID", "AttemptID", "IsFragmented",
		"Status", "CreatedAt", "Visibility", "ErrorCode", "Fragments",
		"HasMore", "NextCursor", "Content",
	} {
		_, ok := snapshotType.FieldByName(field)
		require.True(t, ok, field)
	}
	snapshotContentType := reflect.TypeOf(journalcontract.JournalSnapshotContent{})
	for _, field := range []string{"Document", "Terminal", "Code", "Skill", "Browser"} {
		_, ok := snapshotContentType.FieldByName(field)
		require.True(t, ok, field)
	}
	fragmentType := reflect.TypeOf(journalcontract.JournalSnapshotFragment{})
	for _, field := range []string{
		"FragmentID", "FragmentIndex", "Content", "ByteStart", "ByteEnd", "SizeBytes",
		"ContentHash", "Kind", "BlockID", "Stream", "StartLine", "EndLine", "ItemStart",
		"ItemEnd", "BinaryContentBase64", "MimeType", "Chapters", "Highlights", "Skills", "Analysis",
	} {
		_, ok := fragmentType.FieldByName(field)
		require.True(t, ok, "JournalSnapshotFragment.%s", field)
	}

	snapshotStructs := []struct {
		value  any
		fields []string
	}{
		{journalcontract.JournalDocumentChapter{}, []string{
			"ChapterID", "Title", "Level",
		}},
		{journalcontract.JournalDocumentSnapshotContent{}, []string{
			"Title", "Format", "Content", "SourceArtifactID", "Token", "Chapters",
			"ActiveBlock", "Revision", "SyncStatus",
		}},
		{journalcontract.JournalTerminalSnapshotContent{}, []string{
			"Command", "Output", "ExitCode", "WorkingDirectory", "SessionID", "StartedAt",
			"FinishedAt", "Stdout", "Stderr", "DurationMs",
		}},
		{journalcontract.JournalCodeHighlight{}, []string{
			"StartLine", "EndLine", "Kind",
		}},
		{journalcontract.JournalCodeSnapshotContent{}, []string{
			"FilePath", "Language", "Content", "Diff", "StartLine", "EndLine",
			"Repository", "Revision", "Highlights",
		}},
		{journalcontract.JournalBrowserSnapshotContent{}, []string{
			"URL", "Title", "ScreenshotArtifactID", "CaptureID", "ThumbnailBase64",
			"StaticSnapshotBase64", "MimeType", "Analysis", "Index", "Total", "Redacted",
			"RedactionEvidenceID", "RedactionPolicyVersion",
		}},
		{journalcontract.AuditCanonicalRunSnapshotActionRequest{}, []string{
			"ThreadID", "RunID", "SnapshotID", "SpaceID", "Action", "IdempotencyKey", "FragmentID",
		}},
		{journalcontract.JournalSnapshotActionAuditResponse{}, []string{
			"SnapshotID", "Action", "Allowed", "AuditedAt", "CopyText", "DownloadURL",
			"DownloadContentBase64", "DownloadMimeType",
		}},
	}
	for _, contract := range snapshotStructs {
		contractType := reflect.TypeOf(contract.value)
		for _, field := range contract.fields {
			_, ok := contractType.FieldByName(field)
			require.True(t, ok, "%s.%s", contractType.Name(), field)
		}
	}
	_, exposesObjectKey := reflect.TypeOf(journalcontract.JournalDocumentSnapshotContent{}).
		FieldByName("OriginalObjectKey")
	require.False(t, exposesObjectKey)
	_, exposesOriginalURL := reflect.TypeOf(journalcontract.JournalDocumentSnapshotContent{}).
		FieldByName("OriginalURL")
	require.False(t, exposesOriginalURL)
	_, exposesBrowserContent := reflect.TypeOf(journalcontract.JournalBrowserSnapshotContent{}).
		FieldByName("Content")
	require.False(t, exposesBrowserContent)

	contentField, ok := snapshotType.FieldByName("Content")
	require.True(t, ok)
	require.Contains(t, contentField.Tag.Get("thrift"), "optional")
	require.Contains(t, contentField.Tag.Get("json"), "omitempty")

	validContent := &journalcontract.JournalSnapshotContent{
		Terminal: &journalcontract.JournalTerminalSnapshotContent{Command: "pwd"},
	}
	require.Equal(t, 1, validContent.CountSetFieldsJournalSnapshotContent())
	require.NoError(t, validContent.Write(thrift.NewTBinaryProtocolTransport(
		thrift.NewTMemoryBufferLen(128),
	)))

	invalidContent := &journalcontract.JournalSnapshotContent{
		Document: &journalcontract.JournalDocumentSnapshotContent{Title: "requirements"},
		Terminal: &journalcontract.JournalTerminalSnapshotContent{Command: "pwd"},
	}
	require.Equal(t, 2, invalidContent.CountSetFieldsJournalSnapshotContent())
	require.Error(t, invalidContent.Write(thrift.NewTBinaryProtocolTransport(
		thrift.NewTMemoryBufferLen(128),
	)))

	require.Equal(t, "copy_command", journalcontract.JournalSnapshotActionCopyCommand)
	require.Equal(t, "copy_output", journalcontract.JournalSnapshotActionCopyOutput)
	require.Equal(t, "copy_code", journalcontract.JournalSnapshotActionCopyCode)
	require.Equal(t, "open_original", journalcontract.JournalSnapshotActionOpenOriginal)
	require.Equal(t, "download_fragment", journalcontract.JournalSnapshotActionDownloadFragment)
	require.Equal(t, []journalcontract.JournalSnapshotFragmentKind{
		"document_block", "document_chapters", "terminal_stdout", "terminal_stderr",
		"code_lines", "code_highlights", "skill_items", "browser_thumbnail",
		"browser_snapshot", "browser_analysis",
	}, []journalcontract.JournalSnapshotFragmentKind{
		journalcontract.JournalSnapshotFragmentKindDocumentBlock,
		journalcontract.JournalSnapshotFragmentKindDocumentChapters,
		journalcontract.JournalSnapshotFragmentKindTerminalStdout,
		journalcontract.JournalSnapshotFragmentKindTerminalStderr,
		journalcontract.JournalSnapshotFragmentKindCodeLines,
		journalcontract.JournalSnapshotFragmentKindCodeHighlights,
		journalcontract.JournalSnapshotFragmentKindSkillItems,
		journalcontract.JournalSnapshotFragmentKindBrowserThumbnail,
		journalcontract.JournalSnapshotFragmentKindBrowserSnapshot,
		journalcontract.JournalSnapshotFragmentKindBrowserAnalysis,
	})
}

func TestCanonicalJournalContractControlFramesAndErrorCodes(t *testing.T) {
	controlTypes := []journalcontract.JournalControlFrameType{
		journalcontract.JournalControlFrameTypeJournalDisabled,
		journalcontract.JournalControlFrameTypeJournalDegraded,
		journalcontract.JournalControlFrameTypeCapabilityUnavailable,
		journalcontract.JournalControlFrameTypeProtocolIncompatible,
	}
	require.Equal(t, []journalcontract.JournalControlFrameType{
		"journal_disabled", "journal_degraded", "capability_unavailable", "protocol_incompatible",
	}, controlTypes)

	streamFrameTypes := []any{
		journalcontract.JournalEventStreamFrame{},
		journalcontract.JournalHeartbeatStreamFrame{},
		journalcontract.JournalControlStreamFrame{},
	}
	for _, frame := range streamFrameTypes {
		frameType := reflect.TypeOf(frame)
		_, hasKind := frameType.FieldByName("Kind")
		require.True(t, hasKind, frameType.Name())
	}

	errorCodes := []journalcontract.JournalErrorCode{
		journalcontract.JournalErrorCodeJournalCursorExpired,
		journalcontract.JournalErrorCodeJournalEventGap,
		journalcontract.JournalErrorCodeSnapshotUnavailable,
		journalcontract.JournalErrorCodeResourceNotFound,
		journalcontract.JournalErrorCodeRecoveryConflict,
		journalcontract.JournalErrorCodeRecoveryConfirmRequired,
		journalcontract.JournalErrorCodeJournalRateLimited,
		journalcontract.JournalErrorCodeSchemaIncompatible,
		journalcontract.JournalErrorCodeNoPermission,
	}
	require.Equal(t, []journalcontract.JournalErrorCode{
		"JOURNAL_CURSOR_EXPIRED",
		"JOURNAL_EVENT_GAP",
		"SNAPSHOT_UNAVAILABLE",
		"RESOURCE_NOT_FOUND",
		"RECOVERY_CONFLICT",
		"RECOVERY_CONFIRM_REQUIRED",
		"JOURNAL_RATE_LIMITED",
		"SCHEMA_INCOMPATIBLE",
		"NO_PERMISSION",
	}, errorCodes)
}

func canonicalTestStringPointer(value string) *string {
	return &value
}

func TestCanonicalJournalContractProtocolNegotiation(t *testing.T) {
	mode, selected, public := negotiateCanonicalJournalProtocolVersion("")
	require.Nil(t, public)
	require.Equal(t, canonicalJournalProtocolLegacy, mode)
	require.Empty(t, selected)

	mode, selected, public = negotiateCanonicalJournalProtocolVersion("1.0")
	require.Nil(t, public)
	require.Equal(t, canonicalJournalProtocolLegacy, mode)
	require.Equal(t, "1.0", selected)

	mode, selected, public = negotiateCanonicalJournalProtocolVersion("1.1")
	require.Nil(t, public)
	require.Equal(t, canonicalJournalProtocolV11, mode)
	require.Equal(t, "1.1", selected)

	mode, selected, public = negotiateCanonicalJournalProtocolVersion("1.7")
	require.Nil(t, public)
	require.Equal(t, canonicalJournalProtocolV11, mode)
	require.Equal(t, "1.1", selected)

	for _, incompatible := range []string{"2.0", "0.9", "invalid"} {
		_, _, public = negotiateCanonicalJournalProtocolVersion(incompatible)
		require.NotNil(t, public, incompatible)
		require.Equal(t, "SCHEMA_INCOMPATIBLE", public.Code, incompatible)
		require.False(t, public.Retryable, incompatible)
	}

	require.Nil(t, validateCanonicalJournalPayloadVersion(""))
	require.Nil(t, validateCanonicalJournalPayloadVersion("1.0"))
	require.Nil(t, validateCanonicalJournalPayloadVersion("1.9"))
	require.Equal(t, "SCHEMA_INCOMPATIBLE", validateCanonicalJournalPayloadVersion("2.0").Code)
}

func TestCanonicalErrorCompatibility(t *testing.T) {
	tests := []struct {
		code      string
		retryable bool
	}{
		{"JOURNAL_CURSOR_EXPIRED", true},
		{"JOURNAL_EVENT_GAP", true},
		{"SNAPSHOT_UNAVAILABLE", false},
		{"RESOURCE_NOT_FOUND", false},
		{"RECOVERY_CONFLICT", true},
		{"RECOVERY_CONFIRM_REQUIRED", false},
		{"JOURNAL_RATE_LIMITED", true},
		{"SCHEMA_INCOMPATIBLE", false},
		{"NO_PERMISSION", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.code, func(t *testing.T) {
			public := canonicalJournalError(tt.code)
			require.NotNil(t, public)
			require.Equal(t, tt.code, public.Code)
			require.Equal(t, tt.retryable, public.Retryable)

			ctx := context.WithValue(context.Background(), projectconsts.CtxLogIDKey, "trace-journal")
			var c app.RequestContext
			writeCanonicalJournalError(ctx, &c, public.status, *public)

			var body map[string]any
			require.NoError(t, json.Unmarshal(c.Response.Body(), &body))
			require.Equal(t, tt.code, body["error_code"])
			require.Equal(t, tt.code, body["code"])
			require.Equal(t, "trace-journal", body["trace_id"])
			require.Equal(t, tt.retryable, body["retryable"])
			require.NotContains(t, body, "internal_reason")
		})
	}

	require.Nil(t, canonicalJournalError("UNKNOWN"))
}

func TestCanonicalErrorMapsApplicationFailures(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		status    int
		code      string
		retryable bool
	}{
		{"invalid argument", domainservice.ErrInvalidArgument, hertzconsts.StatusBadRequest, "invalid_request", false},
		{"access denied", appagentthread.ErrThreadAccessDenied, hertzconsts.StatusNotFound, "resource_not_found", false},
		{"active run", appagentthread.ErrActiveRunExists, hertzconsts.StatusConflict, "run_conflict", false},
		{"idempotency conflict", appagentthread.ErrRunIdempotencyConflict, hertzconsts.StatusConflict, "idempotency_conflict", false},
		{"invalid resume", appagentthread.ErrHumanInteractionResumeInvalid, hertzconsts.StatusUnprocessableEntity, "invalid_resume", false},
		{"resume conflict", appagentthread.ErrHumanInteractionResumeConflict, hertzconsts.StatusConflict, "run_not_resumable", false},
		{"journal budget", errCanonicalJournalBudgetExceeded, hertzconsts.StatusUnprocessableEntity, "journal_too_large", false},
		{"deadline", context.DeadlineExceeded, hertzconsts.StatusGatewayTimeout, "run_wait_timeout", true},
		{"canceled", context.Canceled, hertzconsts.StatusRequestTimeout, "request_canceled", true},
		{"unsupported value", appagentthread.ErrUnsupportedMultitaskStrategy, hertzconsts.StatusUnprocessableEntity, "unsupported_value", false},
		{"runtime config", appagentthread.ErrInvalidRuntimeConfig, hertzconsts.StatusUnprocessableEntity, "invalid_runtime_config", false},
		{"dependency", appagentthread.ErrThreadAuthorizationUnavailable, hertzconsts.StatusServiceUnavailable, "dependency_unavailable", true},
		{"unknown", errors.New("sensitive internal cause"), hertzconsts.StatusInternalServerError, "internal_error", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			public := mapCanonicalApplicationError(tt.err)
			require.Equal(t, tt.status, public.status)
			require.Equal(t, tt.code, public.Code)
			require.Equal(t, tt.retryable, public.Retryable)
			require.NotContains(t, public.Detail, tt.err.Error())
		})
	}
}

func TestCanonicalErrorMapsUnsupportedExecutionControlPath(t *testing.T) {
	applicationService := &appagentthread.ApplicationService{
		ThreadSVC: domainservice.NewService(nil),
	}
	_, err := applicationService.CreateTaskThread(context.Background(), &appagentthread.CreateTaskThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Message: "continue",
		Config:  `{"configurable":{"mode":"provider-secret"}}`,
	})
	require.ErrorIs(t, err, appagentthread.ErrUnsupportedExecutionControl)
	require.Equal(t, "config.configurable.mode", mustUnsupportedExecutionControlPath(t, err))

	public := mapCanonicalApplicationError(err)
	require.Equal(t, hertzconsts.StatusUnprocessableEntity, public.status)
	require.Equal(t, "unsupported_execution_control", public.Code)
	require.Equal(t, "Unsupported execution control: config.configurable.mode", public.Detail)
	require.Equal(t, "unsupported_execution_control", public.errorClass)
	require.False(t, public.Retryable)
	require.NotContains(t, public.Detail, "provider-secret")
}

func mustUnsupportedExecutionControlPath(t *testing.T, err error) string {
	t.Helper()
	path, ok := appagentthread.UnsupportedExecutionControlPath(err)
	require.True(t, ok)
	return path
}

func TestCanonicalErrorLogFieldsNeverExposeRawCause(t *testing.T) {
	const sensitiveCause = "database failed with sk-secret and raw provider payload"

	errorCode, errorClass := canonicalErrorLogFields(errors.New(sensitiveCause))

	require.Equal(t, "internal_error", errorCode)
	require.Equal(t, "internal_error", errorClass)
	require.NotContains(t, errorCode, sensitiveCause)
	require.NotContains(t, errorClass, sensitiveCause)
}

func TestCanonicalTraceIDUsesRequestLogID(t *testing.T) {
	ctx := context.WithValue(context.Background(), projectconsts.CtxLogIDKey, "trace-test")
	require.Equal(t, "trace-test", canonicalTraceID(ctx))
	require.Empty(t, canonicalTraceID(context.Background()))
	require.Empty(t, canonicalTraceID(context.WithValue(
		context.Background(),
		projectconsts.CtxLogIDKey,
		int64(42),
	)))
}

func TestCanonicalLogHashIsStableAndNeverEchoesSource(t *testing.T) {
	const source = "canonical-idempotency-secret"
	first := canonicalLogHash(source)
	second := canonicalLogHash(source)

	require.Equal(t, first, second)
	require.Len(t, first, 16)
	require.NotContains(t, first, source)
	require.Equal(t, "none", canonicalLogHash(""))
}

func TestCanonicalLogEnumsFailClosed(t *testing.T) {
	require.Equal(t, "values", canonicalResponseBodyKind("values", hertzconsts.StatusOK))
	require.Equal(t, "error", canonicalResponseBodyKind("values", hertzconsts.StatusBadRequest))
	require.Equal(t, "none", canonicalResponseBodyKind("request-body", hertzconsts.StatusOK))
	require.Equal(t, "run_retry", canonicalSubmissionKind("run_retry"))
	require.Equal(t, "not_applicable", canonicalSubmissionKind("retry-payload"))
	require.Equal(t, "not_applicable", canonicalRaiseErrorMode(nil))
	value := true
	require.Equal(t, "true", canonicalRaiseErrorMode(&value))
}

func TestCanonicalHeadersAreAPIBaseRelative(t *testing.T) {
	require.Equal(t, "/threads/101/runs/202", canonicalRunPath(101, 202))
	require.Equal(t, "/threads/101/runs/202/stream", canonicalRunStreamPath(101, 202))
	require.Equal(t, "/threads/101/runs/202/join", canonicalRunJoinPath(101, 202))

	for _, location := range []string{
		canonicalRunPath(101, 202),
		canonicalRunStreamPath(101, 202),
		canonicalRunJoinPath(101, 202),
	} {
		require.True(t, strings.HasPrefix(location, "/threads/"))
		require.NotContains(t, location, "/api/workbench")
		require.NotContains(t, location, "://")
		require.NotContains(t, location, "?")
	}
}

func TestCanonicalPaginationHeadersAreStable(t *testing.T) {
	var c app.RequestContext
	setCanonicalPaginationHeaders(&c, 53, 20, 10)
	require.Equal(t, "53", string(c.Response.Header.Peek("X-Pagination-Total")))
	require.Equal(t, "30", string(c.Response.Header.Peek("X-Pagination-Next")))

	setCanonicalPaginationHeaders(&c, 25, 20, 10)
	require.Equal(t, "25", string(c.Response.Header.Peek("X-Pagination-Total")))
	require.Empty(t, c.Response.Header.Peek("X-Pagination-Next"))

	setCanonicalPaginationHeaders(&c, int64(^uint64(0)>>1), int(^uint(0)>>1), int(^uint(0)>>1))
	require.Empty(t, c.Response.Header.Peek("X-Pagination-Next"))
}

type canonicalRecordingWorkspaceAuthorizer struct {
	calls int
	req   appagentthread.WorkspaceAccessRequest
	err   error
}

func (a *canonicalRecordingWorkspaceAuthorizer) AuthorizeWorkspaceAccess(
	_ context.Context,
	req appagentthread.WorkspaceAccessRequest,
) error {
	a.calls++
	a.req = req
	return a.err
}

func canonicalViewerContext(userID int64) context.Context {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, projectconsts.SessionDataKeyInCtx, &userentity.Session{UserID: userID})
	return ctx
}

func canonicalQueryValue(
	t *testing.T,
	c *app.RequestContext,
	name string,
	assertError func(*canonicalError),
) *int64 {
	t.Helper()
	value, public := canonicalQueryInt64(c, name)
	if assertError == nil {
		require.Nil(t, public)
		return value
	}
	require.NotNil(t, public)
	assertError(public)
	return nil
}
