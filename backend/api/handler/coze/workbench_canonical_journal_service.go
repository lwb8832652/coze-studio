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
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	journalcontract "github.com/coze-dev/coze-studio/backend/api/model/workbench/journal_contract"
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

func writeCanonicalJournalNotImplemented(ctx context.Context, c *app.RequestContext) {
	public := newCanonicalError(
		consts.StatusNotImplemented,
		"journal_not_implemented",
		"Journal endpoint is not implemented",
		"journal_not_implemented",
		false,
	)
	writeCanonicalJournalError(ctx, c, public.status, *public)
}

func GetCanonicalRunJournal(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}

func GetCanonicalRunSnapshot(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}

func AuditCanonicalRunSnapshotAction(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}

func RecoverCanonicalRunJournal(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}

func GetCanonicalJournalSettings(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}

func PatchCanonicalJournalSettings(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}

func CopyCanonicalThreadArtifactLink(ctx context.Context, c *app.RequestContext) {
	writeCanonicalJournalNotImplemented(ctx, c)
}
