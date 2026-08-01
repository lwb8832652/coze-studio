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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	journalcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/journal_contract"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

type canonicalJournalProtocolMode string

const (
	canonicalJournalProtocolLegacy canonicalJournalProtocolMode = "legacy"
	canonicalJournalProtocolV11    canonicalJournalProtocolMode = "journal_v1_1"
)

type canonicalJournalErrorDefinition struct {
	status    int
	detail    string
	retryable bool
}

var canonicalJournalErrorDefinitions = map[string]canonicalJournalErrorDefinition{
	"JOURNAL_CURSOR_EXPIRED": {
		status: consts.StatusGone, detail: "Journal cursor expired", retryable: true,
	},
	"JOURNAL_EVENT_GAP": {
		status: consts.StatusConflict, detail: "Journal event gap detected", retryable: true,
	},
	"SNAPSHOT_UNAVAILABLE": {
		status: consts.StatusGone, detail: "Snapshot is unavailable", retryable: false,
	},
	"RESOURCE_NOT_FOUND": {
		status: consts.StatusNotFound, detail: "Resource not found", retryable: false,
	},
	"RECOVERY_CONFLICT": {
		status: consts.StatusConflict, detail: "Recovery is already in progress", retryable: true,
	},
	"RECOVERY_CONFIRM_REQUIRED": {
		status: consts.StatusConflict, detail: "Recovery confirmation is required", retryable: false,
	},
	"JOURNAL_RATE_LIMITED": {
		status: consts.StatusTooManyRequests, detail: "Journal request rate limited", retryable: true,
	},
	"SCHEMA_INCOMPATIBLE": {
		status: consts.StatusUnprocessableEntity, detail: "Journal schema is incompatible", retryable: false,
	},
	"NO_PERMISSION": {
		status: consts.StatusForbidden, detail: "Permission denied", retryable: false,
	},
}

func canonicalJournalError(code string) *canonicalError {
	definition, ok := canonicalJournalErrorDefinitions[code]
	if !ok {
		return nil
	}
	return newCanonicalError(
		definition.status,
		code,
		definition.detail,
		strings.ToLower(code),
		definition.retryable,
	)
}

func negotiateCanonicalJournalProtocolVersion(
	raw string,
) (canonicalJournalProtocolMode, string, *canonicalError) {
	if raw == "" {
		return canonicalJournalProtocolLegacy, "", nil
	}
	major, minor, ok := parseCanonicalJournalVersion(raw)
	if !ok || major != 1 {
		return canonicalJournalProtocolLegacy, "", canonicalJournalError("SCHEMA_INCOMPATIBLE")
	}
	if minor == 0 {
		return canonicalJournalProtocolLegacy, "1.0", nil
	}
	return canonicalJournalProtocolV11, "1.1", nil
}

func validateCanonicalJournalPayloadVersion(raw string) *canonicalError {
	if raw == "" {
		return nil
	}
	major, _, ok := parseCanonicalJournalVersion(raw)
	if !ok || major != 1 {
		return canonicalJournalError("SCHEMA_INCOMPATIBLE")
	}
	return nil
}

func validateCanonicalJournalActionPayload(
	payload *journalcontract.JournalActionEventPayload,
) *canonicalError {
	if payload == nil || payload.Data == nil {
		return canonicalJournalError("SCHEMA_INCOMPATIBLE")
	}

	expectedContentType := map[journalcontract.JournalActionPayloadType]journalcontract.JournalSnapshotContentType{
		journalcontract.JournalActionPayloadTypeDocument: journalcontract.JournalSnapshotContentTypeDocument,
		journalcontract.JournalActionPayloadTypeTerminal: journalcontract.JournalSnapshotContentTypeTerminal,
		journalcontract.JournalActionPayloadTypeCode:     journalcontract.JournalSnapshotContentTypeCode,
		journalcontract.JournalActionPayloadTypeSkill:    journalcontract.JournalSnapshotContentTypeSkill,
		journalcontract.JournalActionPayloadTypeBrowser:  journalcontract.JournalSnapshotContentTypeBrowser,
	}
	if payload.Type == journalcontract.JournalActionPayloadTypeGeneric {
		if payload.Data.ContentType != nil {
			return canonicalJournalError("SCHEMA_INCOMPATIBLE")
		}
		return nil
	}

	expected, ok := expectedContentType[payload.Type]
	if !ok || payload.Data.ContentType == nil || *payload.Data.ContentType != expected {
		return canonicalJournalError("SCHEMA_INCOMPATIBLE")
	}
	return nil
}

func parseCanonicalJournalVersion(raw string) (int, int, bool) {
	if raw == "" || raw != strings.TrimSpace(raw) {
		return 0, 0, false
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil || major < 0 || minor < 0 {
		return 0, 0, false
	}
	return major, minor, true
}

type canonicalJournalEventWire struct {
	EventID        string                                  `json:"event_id"`
	ThreadID       string                                  `json:"thread_id"`
	RunID          string                                  `json:"run_id"`
	EventType      string                                  `json:"event_type"`
	Payload        json.RawMessage                         `json:"payload"`
	CreatedAt      string                                  `json:"created_at"`
	SchemaVersion  *string                                 `json:"schema_version,omitempty"`
	AttemptID      *string                                 `json:"attempt_id,omitempty"`
	Sequence       *int64                                  `json:"sequence,omitempty"`
	IdempotencyKey *string                                 `json:"idempotency_key,omitempty"`
	ParentEventID  *string                                 `json:"parent_event_id,omitempty"`
	Status         *journalcontract.JournalExecutionStatus `json:"status,omitempty"`
	OccurredAt     *string                                 `json:"occurred_at,omitempty"`
	Visibility     *journalcontract.JournalVisibility      `json:"visibility,omitempty"`
	PayloadVersion *string                                 `json:"payload_version,omitempty"`
	SnapshotID     *string                                 `json:"snapshot_id,omitempty"`
	TraceID        *string                                 `json:"trace_id,omitempty"`
}

type canonicalJournalEventPageWire struct {
	Data              []*canonicalJournalEventWire `json:"data"`
	HasMore           bool                         `json:"has_more"`
	NextAfterEventID  *string                      `json:"next_after_event_id,omitempty"`
	AttemptID         *string                      `json:"attempt_id,omitempty"`
	LatestSequence    *int64                       `json:"latest_sequence,omitempty"`
	NextAfterSequence *int64                       `json:"next_after_sequence,omitempty"`
}

type canonicalJournalBootstrapWire struct {
	Attempts           []*journalcontract.JournalAttemptSummary     `json:"attempts"`
	DefaultAttemptID   string                                       `json:"default_attempt_id"`
	DefaultAttempt     *journalcontract.JournalAttemptSummary       `json:"default_attempt,omitempty"`
	ProjectionState    journalcontract.JournalProjectionState       `json:"projection_state"`
	LatestSequence     int64                                        `json:"latest_sequence"`
	Events             *canonicalJournalEventPageWire               `json:"events"`
	ContentTypes       []journalcontract.JournalSnapshotContentType `json:"content_types"`
	Enrollment         *journalcontract.JournalEnrollment           `json:"enrollment"`
	SubmitAt           string                                       `json:"submit_at"`
	ServerTime         string                                       `json:"server_time"`
	RecoveryCapability *journalcontract.JournalRecoveryCapability   `json:"recovery_capability"`
}

func parseCanonicalJournalEvent(raw []byte) (*journalcontract.JournalEvent, error) {
	var wire canonicalJournalEventWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return nil, err
	}

	return &journalcontract.JournalEvent{
		EventID:        wire.EventID,
		ThreadID:       wire.ThreadID,
		RunID:          wire.RunID,
		EventType:      wire.EventType,
		Payload:        string(wire.Payload),
		CreatedAt:      wire.CreatedAt,
		SchemaVersion:  wire.SchemaVersion,
		AttemptID:      wire.AttemptID,
		Sequence:       wire.Sequence,
		IdempotencyKey: wire.IdempotencyKey,
		ParentEventID:  wire.ParentEventID,
		Status:         wire.Status,
		OccurredAt:     wire.OccurredAt,
		Visibility:     wire.Visibility,
		PayloadVersion: wire.PayloadVersion,
		SnapshotID:     wire.SnapshotID,
		TraceID:        wire.TraceID,
	}, nil
}

func requireCanonicalJournalAgentThreadService(
	ctx context.Context,
	c *app.RequestContext,
) bool {
	if appagentthread.SVC != nil && appagentthread.SVC.ThreadSVC != nil {
		return true
	}
	public := newCanonicalError(
		consts.StatusServiceUnavailable,
		"dependency_unavailable",
		"Required service is unavailable",
		"agent_thread_service_unavailable",
		true,
	)
	writeCanonicalJournalError(ctx, c, public.status, *public)
	return false
}

func requireCanonicalJournalSpaceAccess(
	ctx context.Context,
	c *app.RequestContext,
) (context.Context, bool) {
	spaceID, public := canonicalSpaceID(ctx, c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return ctx, false
	}
	return context.WithValue(ctx, canonicalSpaceAccessContextKey{}, spaceID), true
}

func GetCanonicalRunJournal(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog(
		"journal.bootstrap.get",
		"/api/workbench/threads/:thread_id/runs/:run_id/journal",
	)
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	if !requireCanonicalJournalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalJournalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	requestLog.ResponseBodyKind = "event_page"
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
		return
	}

	_, protocolVersion, public := negotiateCanonicalJournalProtocolVersion(
		canonicalQueryString(c, "journal_protocol_version"),
	)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	if protocolVersion == "" {
		protocolVersion = "1.0"
	}
	limit, public := canonicalQueryLimit(c, "limit", 50)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	afterSequence, public := canonicalJournalSequenceCursor(c, "after_sequence")
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	_, afterSequenceSet := c.GetQuery("after_sequence")
	afterEventID, public := canonicalJournalEventCursor(c, "after_event_id")
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	attemptID := strings.TrimSpace(canonicalQueryString(c, "attempt_id"))
	if afterSequenceSet && attemptID == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *newCanonicalError(
			consts.StatusBadRequest,
			"invalid_request",
			"attempt_id is required with after_sequence",
			"journal_attempt_cursor_missing",
			false,
		))
		return
	}

	lease, ok := acquireCanonicalJournalAdmission(ctx, c, appagentthread.JournalAdmissionRequest{
		SpaceID: canonicalSpaceIDFromContext(ctx), ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID, RunID: runID,
		Kind: appagentthread.JournalAdmissionKindBootstrap,
	})
	if !ok {
		return
	}
	defer releaseCanonicalJournalAdmission(ctx, lease)

	bootstrap, err := appagentthread.SVC.GetJournalBootstrap(
		ctx,
		appagentthread.GetJournalBootstrapRequest{
			ViewerID: workbenchViewerIDFromCtx(ctx),
			SpaceID:  canonicalSpaceIDFromContext(ctx),
			ThreadID: threadID, RunID: runID, AttemptID: attemptID,
			AfterSequence: afterSequence, AfterSequenceSet: afterSequenceSet,
			AfterEventID: afterEventID,
			Limit:        int(limit),
		},
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	response, err := projectCanonicalJournalBootstrap(
		bootstrap,
		run.CreatedAt,
		protocolVersion,
		time.Now().UnixMilli(),
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.JSON(consts.StatusOK, response)
}

func canonicalJournalEventListRequested(c *app.RequestContext) bool {
	if c == nil {
		return false
	}
	if strings.TrimSpace(canonicalQueryString(c, "attempt_id")) != "" {
		return true
	}
	_, afterSequencePresent := c.GetQuery("after_sequence")
	return afterSequencePresent
}

func listCanonicalRunJournalEvents(
	ctx context.Context,
	c *app.RequestContext,
	requestLog *canonicalRequestLog,
	threadID int64,
	runID int64,
) {
	attemptID := strings.TrimSpace(canonicalQueryString(c, "attempt_id"))
	if attemptID == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *canonicalInvalidRequest(
			"attempt_id is required for Journal event pagination",
			"journal_attempt_cursor_missing",
		))
		return
	}
	if len(c.QueryArgs().PeekAll("event_types")) > 0 {
		writeCanonicalJournalError(ctx, c, consts.StatusUnprocessableEntity, *newCanonicalError(
			consts.StatusUnprocessableEntity,
			"invalid_request",
			"event_types is not supported for Journal event pagination",
			"journal_event_filter_unsupported",
			false,
		))
		return
	}
	afterSequence, public := canonicalJournalSequenceCursor(c, "after_sequence")
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	_, afterSequenceSet := c.GetQuery("after_sequence")
	afterEventID, public := canonicalJournalEventCursor(c, "after_event_id")
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	limit, public := canonicalQueryLimit(c, "limit", 20)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.AfterEventID = afterEventID
	requestLog.Limit = limit
	requestLog.StreamModes = string(canonicalJournalProtocolV11)
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
		return
	}

	lease, ok := acquireCanonicalJournalAdmission(ctx, c, appagentthread.JournalAdmissionRequest{
		SpaceID: canonicalSpaceIDFromContext(ctx), ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID, RunID: runID, Kind: appagentthread.JournalAdmissionKindBootstrap,
	})
	if !ok {
		return
	}
	defer releaseCanonicalJournalAdmission(ctx, lease)
	bootstrap, err := appagentthread.SVC.GetJournalBootstrap(
		ctx,
		appagentthread.GetJournalBootstrapRequest{
			ViewerID: workbenchViewerIDFromCtx(ctx), SpaceID: canonicalSpaceIDFromContext(ctx),
			ThreadID: threadID, RunID: runID, AttemptID: attemptID,
			AfterSequence: afterSequence, AfterSequenceSet: afterSequenceSet,
			AfterEventID: afterEventID, Limit: int(limit),
		},
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	page, err := projectCanonicalJournalEventPage(bootstrap)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.JSON(consts.StatusOK, page)
}

func GetCanonicalRunSnapshot(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog(
		"journal.snapshot.get",
		"/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id",
	)
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	c.Response.Header.Set("Cache-Control", appagentthread.JournalSnapshotCacheControl)
	if !requireCanonicalJournalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalJournalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	requestLog.ResponseBodyKind = "snapshot"
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
		return
	}

	snapshotID := strings.TrimSpace(c.Param("snapshot_id"))
	if snapshotID == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *canonicalInvalidRequest(
			"snapshot_id is required",
			"invalid_snapshot_id",
		))
		return
	}
	cursor, public := canonicalJournalSnapshotCursor(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	limit, public := canonicalQueryLimit(c, "limit", 100)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}

	lease, ok := acquireCanonicalJournalAdmission(ctx, c, appagentthread.JournalAdmissionRequest{
		SpaceID: canonicalSpaceIDFromContext(ctx), ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID, RunID: runID,
		Kind: appagentthread.JournalAdmissionKindSnapshot,
	})
	if !ok {
		return
	}
	defer releaseCanonicalJournalAdmission(ctx, lease)

	envelope, err := appagentthread.SVC.GetJournalSnapshot(
		ctx,
		appagentthread.GetJournalSnapshotRequest{
			SpaceID: canonicalSpaceIDFromContext(ctx), ThreadID: threadID, RunID: runID,
			ViewerID: workbenchViewerIDFromCtx(ctx), SnapshotID: snapshotID,
			Cursor: cursor, Limit: int(limit), TraceID: canonicalTraceID(ctx),
		},
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	response, err := projectCanonicalJournalSnapshot(envelope)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, response)
}

func AuditCanonicalRunSnapshotAction(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog(
		"journal.snapshot.action.audit",
		"/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id/actions",
	)
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	c.Response.Header.Set("Cache-Control", appagentthread.JournalSnapshotCacheControl)
	if !requireCanonicalJournalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalJournalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	requestLog.ResponseBodyKind = "snapshot_action_grant"
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
		return
	}

	snapshotID := strings.TrimSpace(c.Param("snapshot_id"))
	if snapshotID == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *canonicalInvalidRequest(
			"snapshot_id is required",
			"invalid_snapshot_id",
		))
		return
	}
	clientIdempotencyKey, public := canonicalRunIdempotencyKey(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	if clientIdempotencyKey == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *canonicalInvalidRequest(
			"Idempotency-Key is required",
			"missing_idempotency_key",
		))
		return
	}
	idempotencyKey, public := canonicalPrincipalScopedIdempotencyKey(ctx, clientIdempotencyKey)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.IdempotencyKeyHash = canonicalLogHash(idempotencyKey)
	var body struct {
		Action     string `json:"action"`
		FragmentID string `json:"fragment_id"`
	}
	if public := decodeCanonicalJSON(c, &body); public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	action := domainentity.JournalSnapshotAction(strings.TrimSpace(body.Action))
	fragmentID := strings.TrimSpace(body.FragmentID)
	if !action.Valid() || !action.UserAction() ||
		(action == domainentity.JournalSnapshotActionDownloadFragment && fragmentID == "") ||
		(action != domainentity.JournalSnapshotActionDownloadFragment && fragmentID != "") {
		writeCanonicalJournalError(ctx, c, consts.StatusUnprocessableEntity, *newCanonicalError(
			consts.StatusUnprocessableEntity,
			"invalid_request",
			"Snapshot action is invalid",
			"invalid_snapshot_action",
			false,
		))
		return
	}

	lease, ok := acquireCanonicalJournalAdmission(ctx, c, appagentthread.JournalAdmissionRequest{
		SpaceID: canonicalSpaceIDFromContext(ctx), ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID, RunID: runID,
		Kind: appagentthread.JournalAdmissionKindAction,
	})
	if !ok {
		return
	}
	defer releaseCanonicalJournalAdmission(ctx, lease)

	grant, err := appagentthread.SVC.AuditJournalSnapshotAction(
		ctx,
		appagentthread.AuditJournalSnapshotActionRequest{
			SpaceID: canonicalSpaceIDFromContext(ctx), ThreadID: threadID, RunID: runID,
			ViewerID: workbenchViewerIDFromCtx(ctx), SnapshotID: snapshotID,
			Action: action, FragmentID: fragmentID, IdempotencyKey: idempotencyKey,
			TraceID: canonicalTraceID(ctx),
		},
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	response, err := projectCanonicalJournalActionGrant(grant)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	c.JSON(consts.StatusOK, response)
}

func RecoverCanonicalRunJournal(ctx context.Context, c *app.RequestContext) {
	requestLog := beginCanonicalRequestLog(
		"journal.recovery.create",
		"/api/workbench/threads/:thread_id/runs/:run_id/recover",
	)
	defer completeCanonicalRequestLog(ctx, c, requestLog)
	c.Response.Header.Set("Cache-Control", "private, no-store")
	if !requireCanonicalJournalAgentThreadService(ctx, c) {
		return
	}
	ctx, ok := requireCanonicalJournalSpaceAccess(ctx, c)
	if !ok {
		return
	}
	threadID, runID, public := canonicalRunPathIDs(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.ThreadID, requestLog.RunID = threadID, runID
	requestLog.ResponseBodyKind = "recovery_attempt"
	ctx = canonicalThreadAccessContext(ctx, threadID, runID)
	run, err := getCanonicalAuthorizedRun(ctx, threadID, runID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	if run == nil {
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
		return
	}

	clientIdempotencyKey, public := canonicalRunIdempotencyKey(c)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	if clientIdempotencyKey == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusBadRequest, *canonicalInvalidRequest(
			"Idempotency-Key is required",
			"missing_idempotency_key",
		))
		return
	}
	idempotencyKey, public := canonicalPrincipalScopedIdempotencyKey(ctx, clientIdempotencyKey)
	if public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	requestLog.IdempotencyKeyHash = canonicalLogHash(idempotencyKey)
	var body struct {
		SourceAttemptID string `json:"source_attempt_id"`
		Action          string `json:"action"`
		Confirmed       bool   `json:"confirmed"`
	}
	if public := decodeCanonicalJSON(c, &body); public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	if strings.TrimSpace(body.Action) == "" {
		writeCanonicalJournalError(ctx, c, consts.StatusUnprocessableEntity, *newCanonicalError(
			consts.StatusUnprocessableEntity,
			"invalid_request",
			"Recovery action is invalid",
			"invalid_recovery_action",
			false,
		))
		return
	}

	lease, ok := acquireCanonicalJournalAdmission(ctx, c, appagentthread.JournalAdmissionRequest{
		SpaceID: canonicalSpaceIDFromContext(ctx), ViewerID: workbenchViewerIDFromCtx(ctx),
		ThreadID: threadID, RunID: runID,
		Kind: appagentthread.JournalAdmissionKindAction,
	})
	if !ok {
		return
	}
	defer releaseCanonicalJournalAdmission(ctx, lease)

	// Task 7 installs the checkpoint/ledger-backed recovery service. Until then,
	// fail closed after full authentication and request validation.
	writeCanonicalJournalError(ctx, c, consts.StatusServiceUnavailable, *newCanonicalError(
		consts.StatusServiceUnavailable,
		"dependency_unavailable",
		"Required service is unavailable",
		"journal_recovery_unavailable",
		true,
	))
}

func GetCanonicalJournalSettings(ctx context.Context, c *app.RequestContext) {
	viewerID := workbenchViewerIDFromCtx(ctx)
	if viewerID <= 0 {
		writeCanonicalJournalError(ctx, c, consts.StatusUnauthorized, *newCanonicalError(
			consts.StatusUnauthorized,
			"unauthenticated",
			"Authentication required",
			"unauthenticated",
			false,
		))
		return
	}
	settings, err := appagentthread.SVC.GetJournalUserSettings(ctx, viewerID)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.JSON(consts.StatusOK, projectCanonicalJournalUserSettings(settings))
}

func PatchCanonicalJournalSettings(ctx context.Context, c *app.RequestContext) {
	viewerID := workbenchViewerIDFromCtx(ctx)
	if viewerID <= 0 {
		writeCanonicalJournalError(ctx, c, consts.StatusUnauthorized, *newCanonicalError(
			consts.StatusUnauthorized,
			"unauthenticated",
			"Authentication required",
			"unauthenticated",
			false,
		))
		return
	}
	var body struct {
		SplitRatio float64 `json:"split_ratio"`
		Revision   string  `json:"revision"`
	}
	if public := decodeCanonicalJSON(c, &body); public != nil {
		writeCanonicalJournalError(ctx, c, public.status, *public)
		return
	}
	settings, err := appagentthread.SVC.PatchJournalUserSettings(
		ctx,
		appagentthread.PatchJournalUserSettingsRequest{
			ViewerID: viewerID, SplitRatio: body.SplitRatio, Revision: body.Revision,
		},
	)
	if err != nil {
		writeCanonicalJournalApplicationError(ctx, c, err)
		return
	}
	c.Response.Header.Set("Cache-Control", "private, no-store")
	c.JSON(consts.StatusOK, projectCanonicalJournalUserSettings(settings))
}

func canonicalJournalSequenceCursor(
	c *app.RequestContext,
	name string,
) (uint64, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, canonicalInvalidRequest(
			name+" must be a non-negative decimal sequence",
			"invalid_journal_sequence",
		)
	}
	return value, nil
}

func canonicalJournalEventCursor(
	c *app.RequestContext,
	name string,
) (int64, *canonicalError) {
	raw, exists := c.GetQuery(name)
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	value, ok := parseCanonicalRunEventCursor(raw)
	if !ok {
		return 0, canonicalInvalidRequest(
			name+" must be a non-negative decimal ID",
			"invalid_journal_event_cursor",
		)
	}
	return value, nil
}

func canonicalJournalSnapshotCursor(c *app.RequestContext) (string, *canonicalError) {
	raw, exists := c.GetQuery("cursor")
	if !exists || strings.TrimSpace(raw) == "" {
		return "", nil
	}
	value := strings.TrimSpace(raw)
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < -1 {
		return "", canonicalInvalidRequest(
			"cursor must be a valid snapshot fragment index",
			"invalid_snapshot_cursor",
		)
	}
	return value, nil
}

func acquireCanonicalJournalAdmission(
	ctx context.Context,
	c *app.RequestContext,
	req appagentthread.JournalAdmissionRequest,
) (appagentthread.JournalAdmissionLease, bool) {
	lease, err := appagentthread.SVC.AcquireJournalAdmission(ctx, req)
	if err == nil {
		return lease, true
	}
	var rateLimited *appagentthread.JournalRateLimitError
	if errors.As(err, &rateLimited) {
		retryAfter := int(math.Ceil(rateLimited.RetryAfter.Seconds()))
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Response.Header.Set("Retry-After", strconv.Itoa(retryAfter))
		writeCanonicalJournalCode(ctx, c, "JOURNAL_RATE_LIMITED")
		return nil, false
	}
	writeCanonicalJournalError(ctx, c, consts.StatusServiceUnavailable, *newCanonicalError(
		consts.StatusServiceUnavailable,
		"dependency_unavailable",
		"Required service is unavailable",
		"journal_admission_unavailable",
		true,
	))
	return nil, false
}

func releaseCanonicalJournalAdmission(
	ctx context.Context,
	lease appagentthread.JournalAdmissionLease,
) {
	if lease != nil {
		_ = lease.Release(ctx)
	}
}

func writeCanonicalJournalCode(
	ctx context.Context,
	c *app.RequestContext,
	code string,
) {
	public := canonicalJournalError(code)
	if public == nil {
		writeCanonicalJournalApplicationError(ctx, c, fmt.Errorf("unknown journal error"))
		return
	}
	writeCanonicalJournalError(ctx, c, public.status, *public)
}

func writeCanonicalJournalApplicationError(
	ctx context.Context,
	c *app.RequestContext,
	err error,
) {
	switch {
	case errors.Is(err, domainrepo.ErrJournalCursorExpired):
		writeCanonicalJournalCode(ctx, c, "JOURNAL_CURSOR_EXPIRED")
	case errors.Is(err, domainrepo.ErrJournalEventGap):
		writeCanonicalJournalCode(ctx, c, "JOURNAL_EVENT_GAP")
	case errors.Is(err, domainrepo.ErrJournalSnapshotNotFound):
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
	case errors.Is(err, appagentthread.ErrJournalSnapshotNoPermission):
		writeCanonicalJournalCode(ctx, c, "NO_PERMISSION")
	case errors.Is(err, appagentthread.ErrJournalSnapshotUnsafeContent),
		errors.Is(err, appagentthread.ErrJournalSnapshotUnavailable):
		writeCanonicalJournalCode(ctx, c, "SNAPSHOT_UNAVAILABLE")
	case errors.Is(err, appagentthread.ErrJournalSnapshotAuditUnavailable):
		public := newCanonicalError(
			consts.StatusServiceUnavailable,
			"dependency_unavailable",
			"Required service is unavailable",
			"journal_snapshot_audit_unavailable",
			true,
		)
		writeCanonicalJournalError(ctx, c, public.status, *public)
	case errors.Is(err, appagentthread.ErrJournalSnapshotActionUnavailable):
		public := newCanonicalError(
			consts.StatusUnprocessableEntity,
			"invalid_request",
			"Snapshot action is unavailable",
			"journal_snapshot_action_unavailable",
			false,
		)
		writeCanonicalJournalError(ctx, c, public.status, *public)
	case errors.Is(err, appagentthread.ErrJournalSnapshotIdempotencyConflict):
		public := newCanonicalError(
			consts.StatusConflict,
			"state_conflict",
			"Snapshot action conflicts with an existing request",
			"journal_snapshot_action_conflict",
			false,
		)
		writeCanonicalJournalError(ctx, c, public.status, *public)
	case errors.Is(err, domainrepo.ErrJournalNotEnrolled),
		errors.Is(err, appagentthread.ErrThreadAccessDenied):
		writeCanonicalJournalCode(ctx, c, "RESOURCE_NOT_FOUND")
	case errors.Is(err, appagentthread.ErrJournalSettingsInvalid):
		public := newCanonicalError(
			consts.StatusUnprocessableEntity,
			"invalid_request",
			"Journal settings are invalid",
			"journal_settings_invalid",
			false,
		)
		writeCanonicalJournalError(ctx, c, public.status, *public)
	case errors.Is(err, appagentthread.ErrJournalSettingsConflict):
		public := newCanonicalError(
			consts.StatusConflict,
			"state_conflict",
			"Journal settings revision is stale",
			"journal_settings_conflict",
			true,
		)
		writeCanonicalJournalError(ctx, c, public.status, *public)
	case errors.Is(err, appagentthread.ErrJournalSettingsUnavailable),
		errors.Is(err, appagentthread.ErrJournalAdmissionUnavailable):
		public := newCanonicalError(
			consts.StatusServiceUnavailable,
			"dependency_unavailable",
			"Required service is unavailable",
			"journal_dependency_unavailable",
			true,
		)
		writeCanonicalJournalError(ctx, c, public.status, *public)
	default:
		mapped := mapCanonicalApplicationError(err)
		writeCanonicalJournalError(ctx, c, mapped.status, mapped)
	}
}

func projectCanonicalJournalBootstrap(
	bootstrap *appagentthread.JournalBootstrapResult,
	submitAt int64,
	protocolVersion string,
	serverTime int64,
) (*canonicalJournalBootstrapWire, error) {
	if bootstrap == nil || bootstrap.SelectedAttempt == nil {
		return nil, fmt.Errorf("journal bootstrap is empty")
	}
	attempts := make([]*journalcontract.JournalAttemptSummary, 0, len(bootstrap.Attempts))
	var selectedSummary *journalcontract.JournalAttemptSummary
	for _, attempt := range bootstrap.Attempts {
		if attempt == nil {
			continue
		}
		latest := attempt.NextSequence - 1
		if attempt.AttemptID == bootstrap.SelectedAttempt.AttemptID {
			latest = bootstrap.LatestSequence
		}
		summary := projectCanonicalJournalAttempt(attempt, latest)
		attempts = append(attempts, summary)
		if attempt.AttemptID == bootstrap.SelectedAttempt.AttemptID {
			selectedSummary = summary
		}
	}
	if selectedSummary == nil {
		return nil, fmt.Errorf("journal bootstrap selected attempt is missing")
	}
	page, err := projectCanonicalJournalEventPage(bootstrap)
	if err != nil {
		return nil, err
	}
	contentTypes := make([]journalcontract.JournalSnapshotContentType, 0, len(bootstrap.ContentTypes))
	for _, contentType := range bootstrap.ContentTypes {
		contentTypes = append(contentTypes, journalcontract.JournalSnapshotContentType(contentType))
	}
	recovery := canonicalJournalRecoveryUnavailable()
	return &canonicalJournalBootstrapWire{
		Attempts: attempts, DefaultAttemptID: selectedSummary.AttemptID,
		DefaultAttempt: selectedSummary,
		ProjectionState: journalcontract.JournalProjectionState(
			bootstrap.SelectedAttempt.ProjectionState,
		),
		LatestSequence: int64(bootstrap.LatestSequence), Events: page,
		ContentTypes: contentTypes,
		Enrollment: &journalcontract.JournalEnrollment{
			Enrolled: true, SchemaVersion: domainentity.JournalSchemaVersion,
			PayloadVersion:         domainentity.JournalPayloadVersion,
			JournalProtocolVersion: protocolVersion,
			JournalEnabled:         bootstrap.JournalEnabled,
			SnapshotsEnabled:       bootstrap.SnapshotsEnabled,
		},
		SubmitAt:           canonicalRunStreamTime(submitAt),
		ServerTime:         canonicalRunStreamTime(serverTime),
		RecoveryCapability: recovery,
	}, nil
}

func projectCanonicalJournalEventPage(
	bootstrap *appagentthread.JournalBootstrapResult,
) (*canonicalJournalEventPageWire, error) {
	if bootstrap == nil || bootstrap.SelectedAttempt == nil {
		return nil, fmt.Errorf("journal event page is empty")
	}
	events := make([]*canonicalJournalEventWire, 0, len(bootstrap.Events))
	for _, event := range bootstrap.Events {
		projected, err := projectCanonicalJournalEventWire(event)
		if err != nil {
			return nil, err
		}
		events = append(events, projected)
	}
	page := &canonicalJournalEventPageWire{
		Data: events, HasMore: bootstrap.HasMore,
		AttemptID:      stringPointer(bootstrap.SelectedAttempt.AttemptID),
		LatestSequence: int64Pointer(int64(bootstrap.LatestSequence)),
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		page.NextAfterEventID = stringPointer(last.EventID)
		if last.Sequence != nil {
			page.NextAfterSequence = int64Pointer(*last.Sequence)
		}
	}
	return page, nil
}

func projectCanonicalJournalEventWire(
	event *domainentity.JournalEvent,
) (*canonicalJournalEventWire, error) {
	projected, err := projectCanonicalJournalEvent(event)
	if err != nil {
		return nil, err
	}
	payload := json.RawMessage(projected.Payload)
	if !json.Valid(payload) {
		return nil, fmt.Errorf("journal event payload is invalid")
	}
	return &canonicalJournalEventWire{
		EventID: projected.EventID, ThreadID: projected.ThreadID, RunID: projected.RunID,
		EventType: projected.EventType, Payload: payload, CreatedAt: projected.CreatedAt,
		SchemaVersion: projected.SchemaVersion, AttemptID: projected.AttemptID,
		Sequence: projected.Sequence, IdempotencyKey: projected.IdempotencyKey,
		ParentEventID: projected.ParentEventID, Status: projected.Status,
		OccurredAt: projected.OccurredAt, Visibility: projected.Visibility,
		PayloadVersion: projected.PayloadVersion, SnapshotID: projected.SnapshotID,
		TraceID: projected.TraceID,
	}, nil
}

func projectCanonicalJournalAttempt(
	attempt *domainentity.RunAttempt,
	latest uint64,
) *journalcontract.JournalAttemptSummary {
	result := &journalcontract.JournalAttemptSummary{
		AttemptID:          attempt.AttemptID,
		RunID:              strconv.FormatInt(attempt.JournalRunID, 10),
		Status:             journalcontract.JournalExecutionStatus(attempt.Status),
		ProjectionState:    journalcontract.JournalProjectionState(attempt.ProjectionState),
		LatestSequence:     int64(latest),
		CreatedAt:          canonicalRunStreamTime(attempt.CreatedAt),
		RecoveryCapability: canonicalJournalRecoveryUnavailable(),
	}
	if attempt.StartedAt != nil {
		result.StartedAt = stringPointer(canonicalRunStreamTime(*attempt.StartedAt))
	}
	if attempt.EndedAt != nil {
		result.EndedAt = stringPointer(canonicalRunStreamTime(*attempt.EndedAt))
	}
	return result
}

func canonicalJournalRecoveryUnavailable() *journalcontract.JournalRecoveryCapability {
	return &journalcontract.JournalRecoveryCapability{
		Allowed: false, RequiresConfirmation: false, AllowedActions: []string{},
	}
}

func projectCanonicalJournalEvent(
	event *domainentity.JournalEvent,
) (*journalcontract.JournalEvent, error) {
	if event == nil || event.ID <= 0 || event.ThreadID <= 0 || event.RunID <= 0 ||
		event.JournalRunID <= 0 ||
		event.AttemptID == "" || event.Sequence == 0 ||
		event.Visibility != domainentity.JournalVisibilityUser {
		return nil, fmt.Errorf("journal event projection is invalid")
	}
	sequence := int64(event.Sequence)
	result := &journalcontract.JournalEvent{
		EventID:   strconv.FormatInt(event.ID, 10),
		ThreadID:  strconv.FormatInt(event.ThreadID, 10),
		RunID:     strconv.FormatInt(event.JournalRunID, 10),
		EventType: event.EventType, Payload: event.Payload,
		CreatedAt:     canonicalRunStreamTime(event.CreatedAt),
		SchemaVersion: stringPointer(event.SchemaVersion),
		AttemptID:     stringPointer(event.AttemptID), Sequence: &sequence,
		Visibility:     journalVisibilityPointer(journalcontract.JournalVisibilityUser),
		PayloadVersion: stringPointer(event.PayloadVersion),
	}
	if event.ParentEventID > 0 {
		result.ParentEventID = stringPointer(strconv.FormatInt(event.ParentEventID, 10))
	}
	if event.Status != "" {
		status := journalcontract.JournalExecutionStatus(event.Status)
		result.Status = &status
	}
	if event.OccurredAtUnixNano > 0 {
		occurredAt := time.Unix(0, event.OccurredAtUnixNano).UTC().Format(time.RFC3339Nano)
		result.OccurredAt = &occurredAt
	}
	if event.SnapshotID != "" {
		result.SnapshotID = stringPointer(event.SnapshotID)
	}
	if event.TraceID != "" {
		result.TraceID = stringPointer(event.TraceID)
	}
	if event.IdempotencyKey != "" {
		result.IdempotencyKey = stringPointer(event.IdempotencyKey)
	}
	return result, nil
}

func projectCanonicalJournalUserSettings(
	settings *appagentthread.JournalUserSettings,
) *journalcontract.JournalUserSettings {
	result := &journalcontract.JournalUserSettings{
		SplitRatio: settings.SplitRatio, Revision: settings.Revision,
	}
	if settings.UpdatedAt > 0 {
		result.UpdatedAt = stringPointer(canonicalRunStreamTime(settings.UpdatedAt))
	}
	return result
}

func projectCanonicalJournalSnapshot(
	envelope *appagentthread.JournalSnapshotEnvelope,
) (*journalcontract.JournalSnapshotEnvelope, error) {
	if envelope == nil || envelope.SnapshotID == "" || envelope.EventID <= 0 ||
		envelope.AttemptID == "" || envelope.CreatedAt <= 0 ||
		!envelope.ContentType.Valid() || !envelope.Status.Valid() ||
		envelope.Visibility != domainentity.JournalVisibilityUser {
		return nil, fmt.Errorf("journal snapshot envelope is invalid")
	}
	content, err := projectCanonicalJournalSnapshotContent(
		envelope.ContentType,
		envelope.Content,
	)
	if err != nil {
		return nil, err
	}
	fragments := make([]*journalcontract.JournalSnapshotFragment, 0, len(envelope.Fragments))
	for _, fragment := range envelope.Fragments {
		projected, err := projectCanonicalJournalSnapshotFragment(fragment)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, projected)
	}
	result := &journalcontract.JournalSnapshotEnvelope{
		ContentType: journalcontract.JournalSnapshotContentType(envelope.ContentType),
		SnapshotID:  envelope.SnapshotID, EventID: strconv.FormatInt(envelope.EventID, 10),
		AttemptID: envelope.AttemptID, IsFragmented: envelope.IsFragmented,
		Status:     journalcontract.JournalContentStatus(envelope.Status),
		CreatedAt:  canonicalRunStreamTime(envelope.CreatedAt),
		Visibility: journalcontract.JournalVisibility(envelope.Visibility),
		Fragments:  fragments, HasMore: envelope.HasMore, Content: content,
	}
	result.ErrorCode = stringPointer(envelope.ErrorCode)
	result.NextCursor = stringPointer(envelope.NextCursor)
	return result, nil
}

func projectCanonicalJournalSnapshotFragment(
	fragment appagentthread.JournalSnapshotFragmentView,
) (*journalcontract.JournalSnapshotFragment, error) {
	if fragment.FragmentID == "" || fragment.FragmentIndex < 0 {
		return nil, fmt.Errorf("journal snapshot fragment is invalid")
	}
	result := &journalcontract.JournalSnapshotFragment{
		FragmentID: fragment.FragmentID, FragmentIndex: fragment.FragmentIndex,
	}
	result.Content = stringPointer(fragment.Content)
	result.ContentHash = stringPointer(fragment.ContentHash)
	result.BlockID = stringPointer(fragment.BlockID)
	result.Stream = stringPointer(fragment.Stream)
	result.MimeType = stringPointer(fragment.MIMEType)
	if fragment.Kind != "" {
		kind := journalcontract.JournalSnapshotFragmentKind(fragment.Kind)
		result.Kind = &kind
	}
	if len(fragment.BinaryContent) > 0 {
		encoded := base64.StdEncoding.EncodeToString(fragment.BinaryContent)
		result.BinaryContentBase64 = &encoded
	}
	if fragment.ByteStart != 0 || fragment.ByteEnd != 0 || fragment.SizeBytes != 0 {
		result.ByteStart = int64Pointer(fragment.ByteStart)
		result.ByteEnd = int64Pointer(fragment.ByteEnd)
		result.SizeBytes = int64Pointer(fragment.SizeBytes)
	}
	if fragment.StartLine > 0 || fragment.EndLine > 0 {
		result.StartLine = int32Pointer(fragment.StartLine)
		result.EndLine = int32Pointer(fragment.EndLine)
	}
	if fragment.ItemStart > 0 || fragment.ItemEnd > 0 {
		result.ItemStart = int32Pointer(fragment.ItemStart)
		result.ItemEnd = int32Pointer(fragment.ItemEnd)
	}
	result.Chapters = projectCanonicalJournalDocumentChapters(fragment.Chapters)
	result.Highlights = projectCanonicalJournalCodeHighlights(fragment.Highlights)
	result.Skills = projectCanonicalJournalSkills(fragment.Skills)
	if len(fragment.Analysis) > 0 {
		result.Analysis = append([]string(nil), fragment.Analysis...)
	}
	return result, nil
}

func projectCanonicalJournalSnapshotContent(
	contentType domainentity.JournalSnapshotContentType,
	content appagentthread.JournalTypedSnapshotContent,
) (*journalcontract.JournalSnapshotContent, error) {
	branchCount := 0
	if content.Document != nil {
		branchCount++
	}
	if content.Terminal != nil {
		branchCount++
	}
	if content.Code != nil {
		branchCount++
	}
	if content.Skill != nil {
		branchCount++
	}
	if content.Browser != nil {
		branchCount++
	}
	if branchCount == 0 {
		return nil, nil
	}
	if branchCount != 1 {
		return nil, fmt.Errorf("journal snapshot content union is invalid")
	}
	result := &journalcontract.JournalSnapshotContent{}
	switch contentType {
	case domainentity.JournalSnapshotContentTypeDocument:
		if content.Document == nil {
			return nil, fmt.Errorf("journal document snapshot content is missing")
		}
		document := content.Document
		result.Document = &journalcontract.JournalDocumentSnapshotContent{
			Title: document.Title, Format: stringPointer(document.Format),
			Content: stringPointer(document.Content), Token: stringPointer(document.Token),
			Chapters:    projectCanonicalJournalDocumentChapters(document.Chapters),
			ActiveBlock: stringPointer(document.ActiveBlock), Revision: stringPointer(document.Revision),
			SyncStatus: stringPointer(document.SyncStatus),
		}
	case domainentity.JournalSnapshotContentTypeTerminal:
		if content.Terminal == nil {
			return nil, fmt.Errorf("journal terminal snapshot content is missing")
		}
		terminal := content.Terminal
		result.Terminal = &journalcontract.JournalTerminalSnapshotContent{
			Command: terminal.Command, ExitCode: terminal.ExitCode,
			WorkingDirectory: stringPointer(terminal.WorkingDirectory),
			SessionID:        stringPointer(terminal.SessionID),
			Stdout:           stringPointer(terminal.Stdout), Stderr: stringPointer(terminal.Stderr),
		}
		if terminal.StartedAt > 0 {
			result.Terminal.StartedAt = stringPointer(canonicalRunStreamTime(terminal.StartedAt))
		}
		if terminal.FinishedAt > 0 {
			result.Terminal.FinishedAt = stringPointer(canonicalRunStreamTime(terminal.FinishedAt))
		}
		if terminal.DurationMS > 0 {
			result.Terminal.DurationMs = int64Pointer(terminal.DurationMS)
		}
	case domainentity.JournalSnapshotContentTypeCode:
		if content.Code == nil {
			return nil, fmt.Errorf("journal code snapshot content is missing")
		}
		code := content.Code
		result.Code = &journalcontract.JournalCodeSnapshotContent{
			FilePath: code.Path, Language: stringPointer(code.Language),
			Content: stringPointer(code.Content), Repository: code.Repository,
			Revision: code.Revision, Highlights: projectCanonicalJournalCodeHighlights(code.Highlights),
		}
		if code.StartLine > 0 {
			result.Code.StartLine = int32Pointer(code.StartLine)
		}
		if code.EndLine > 0 {
			result.Code.EndLine = int32Pointer(code.EndLine)
		}
	case domainentity.JournalSnapshotContentTypeSkill:
		if content.Skill == nil {
			return nil, fmt.Errorf("journal skill snapshot content is missing")
		}
		result.Skill = &journalcontract.JournalSkillSnapshotContent{
			Skills: projectCanonicalJournalSkills(content.Skill.Skills),
		}
	case domainentity.JournalSnapshotContentTypeBrowser:
		if content.Browser == nil {
			return nil, fmt.Errorf("journal browser snapshot content is missing")
		}
		browser := content.Browser
		result.Browser = &journalcontract.JournalBrowserSnapshotContent{
			URL: stringPointer(browser.Resource), Title: stringPointer(browser.Title),
			CaptureID:            browser.CaptureID,
			StaticSnapshotBase64: base64.StdEncoding.EncodeToString(browser.StaticSnapshot),
			MimeType:             browser.MIMEType, Analysis: append([]string(nil), browser.Analysis...),
			Redacted: browser.Redacted, RedactionEvidenceID: browser.RedactionEvidenceID,
			RedactionPolicyVersion: browser.RedactionPolicyVersion,
		}
		if len(browser.Thumbnail) > 0 {
			thumbnail := base64.StdEncoding.EncodeToString(browser.Thumbnail)
			result.Browser.ThumbnailBase64 = &thumbnail
		}
		if browser.Index > 0 {
			result.Browser.Index = int32Pointer(browser.Index)
		}
		if browser.Total > 0 {
			result.Browser.Total = int32Pointer(browser.Total)
		}
	default:
		return nil, fmt.Errorf("journal snapshot content type is invalid")
	}
	return result, nil
}

func projectCanonicalJournalDocumentChapters(
	chapters []appagentthread.JournalDocumentChapter,
) []*journalcontract.JournalDocumentChapter {
	if len(chapters) == 0 {
		return nil
	}
	result := make([]*journalcontract.JournalDocumentChapter, 0, len(chapters))
	for _, chapter := range chapters {
		result = append(result, &journalcontract.JournalDocumentChapter{
			ChapterID: chapter.ChapterID, Title: chapter.Title, Level: chapter.Level,
		})
	}
	return result
}

func projectCanonicalJournalCodeHighlights(
	highlights []appagentthread.JournalCodeHighlight,
) []*journalcontract.JournalCodeHighlight {
	if len(highlights) == 0 {
		return nil
	}
	result := make([]*journalcontract.JournalCodeHighlight, 0, len(highlights))
	for _, highlight := range highlights {
		result = append(result, &journalcontract.JournalCodeHighlight{
			StartLine: highlight.StartLine, EndLine: highlight.EndLine,
			Kind: stringPointer(highlight.Kind),
		})
	}
	return result
}

func projectCanonicalJournalSkills(
	skills []appagentthread.JournalSkillSummary,
) []*journalcontract.JournalSkill {
	if len(skills) == 0 {
		return nil
	}
	result := make([]*journalcontract.JournalSkill, 0, len(skills))
	for _, skill := range skills {
		result = append(result, &journalcontract.JournalSkill{
			SkillID: skill.SkillID, Name: skill.Name,
			Description: stringPointer(skill.Description),
		})
	}
	return result
}

func projectCanonicalJournalActionGrant(
	grant *appagentthread.JournalSnapshotActionGrant,
) (*journalcontract.JournalSnapshotActionAuditResponse, error) {
	if grant == nil || grant.SnapshotID == "" || !grant.Action.UserAction() || grant.AuditedAt <= 0 {
		return nil, fmt.Errorf("journal snapshot action grant is invalid")
	}
	result := &journalcontract.JournalSnapshotActionAuditResponse{
		SnapshotID: grant.SnapshotID,
		Action:     journalcontract.JournalSnapshotAction(grant.Action),
		Allowed:    grant.Allowed, AuditedAt: canonicalRunStreamTime(grant.AuditedAt),
		CopyText: stringPointer(grant.CopyText), DownloadURL: stringPointer(grant.DownloadURL),
		DownloadMimeType: stringPointer(grant.DownloadMIMEType),
	}
	if len(grant.DownloadContent) > 0 {
		content := base64.StdEncoding.EncodeToString(grant.DownloadContent)
		result.DownloadContentBase64 = &content
	}
	return result, nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func int64Pointer(value int64) *int64 { return &value }

func int32Pointer(value int32) *int32 { return &value }

func journalVisibilityPointer(
	value journalcontract.JournalVisibility,
) *journalcontract.JournalVisibility {
	return &value
}
