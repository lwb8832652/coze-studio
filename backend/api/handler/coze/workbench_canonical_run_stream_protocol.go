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
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	"github.com/coze-dev/coze-studio/backend/pkg/sonic"
)

// The canonical Workbench stream keeps reviewed SDK-compatible event names
// and payload shapes without exposing a separate compatibility route surface.
const (
	canonicalRunStreamEventMetadata      = "metadata"
	canonicalRunStreamEventValues        = "values"
	canonicalRunStreamEventUpdates       = "updates"
	canonicalRunStreamEventMessages      = "messages"
	canonicalRunStreamEventEvents        = "events"
	canonicalRunStreamEventDebug         = "debug"
	canonicalRunStreamEventCustom        = "custom"
	canonicalRunStreamEventEnd           = "end"
	canonicalRunStreamEventError         = "error"
	canonicalJournalStreamEventHeartbeat = "heartbeat"
	canonicalJournalStreamEventControl   = "control"
	canonicalRunStreamPageSize           = int32(200)
)

type canonicalRunStreamProtocolWriter interface {
	runEventStreamWriter
}

func setCanonicalRunStreamHeaders(c *app.RequestContext) {
	c.SetContentType("text/event-stream; charset=utf-8")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")
}

func writeCanonicalRunStreamMetadata(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	run *appagentthread.RunSummary,
) bool {
	if run == nil {
		return true
	}

	payload, err := sonic.Marshal(map[string]any{
		"run_id":      strconv.FormatInt(run.RunID, 10),
		"thread_id":   strconv.FormatInt(run.ThreadID, 10),
		"status":      canonicalRunStreamStatus(run.Status),
		"attempt":     1,
		"server_time": canonicalRunStreamTime(time.Now().UnixMilli()),
	})
	if err != nil {
		writeCanonicalRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent("", canonicalRunStreamEventMetadata, payload); err != nil {
		logs.CtxWarnf(ctx, "event_name=workbench.run.stream.write_failed client_contract=%s stage=metadata error_class=%T", canonicalContractVersion, err)
		return false
	}

	return true
}

func writeCanonicalJournalStreamMetadata(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	run *appagentthread.RunSummary,
	bootstrap *appagentthread.JournalBootstrapResult,
	protocolVersion string,
) bool {
	if run == nil || bootstrap == nil || bootstrap.SelectedAttempt == nil {
		return false
	}
	attempt := bootstrap.SelectedAttempt
	return writeCanonicalJournalStreamPayload(ctx, writer, "", canonicalRunStreamEventMetadata, map[string]any{
		"run_id":                   strconv.FormatInt(run.RunID, 10),
		"thread_id":                strconv.FormatInt(run.ThreadID, 10),
		"status":                   string(attempt.Status),
		"attempt":                  attempt.Ordinal,
		"attempt_id":               attempt.AttemptID,
		"latest_sequence":          bootstrap.LatestSequence,
		"submit_at":                canonicalRunStreamTime(run.CreatedAt),
		"server_time":              canonicalRunStreamTime(time.Now().UnixMilli()),
		"journal_enabled":          bootstrap.JournalEnabled,
		"snapshots_enabled":        bootstrap.SnapshotsEnabled,
		"journal_protocol_version": protocolVersion,
	})
}

func writeCanonicalJournalStreamEvent(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	event *domainentity.JournalEvent,
) bool {
	projected, err := projectCanonicalJournalEventWire(event)
	if err != nil {
		writeCanonicalRunStreamError(ctx, writer, err)
		return false
	}
	return writeCanonicalJournalStreamPayload(
		ctx,
		writer,
		projected.EventID,
		canonicalRunStreamEventEvents,
		map[string]any{"kind": "event", "event": projected},
	)
}

func writeCanonicalJournalStreamHeartbeat(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	attempt *domainentity.RunAttempt,
	latestSequence uint64,
) bool {
	if attempt == nil {
		return false
	}
	return writeCanonicalJournalStreamPayload(
		ctx,
		writer,
		"",
		canonicalJournalStreamEventHeartbeat,
		map[string]any{
			"kind": "heartbeat",
			"heartbeat": map[string]any{
				"server_time": canonicalRunStreamTime(time.Now().UnixMilli()),
				"attempt_id":  attempt.AttemptID, "latest_sequence": latestSequence,
			},
		},
	)
}

func writeCanonicalJournalStreamControl(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	controlType string,
	attempt *domainentity.RunAttempt,
	latestSequence uint64,
	errorCode string,
	retryable bool,
	protocolVersion string,
) bool {
	control := map[string]any{
		"type": controlType, "schema_version": domainentity.JournalSchemaVersion,
		"journal_protocol_version": protocolVersion,
		"server_time":              canonicalRunStreamTime(time.Now().UnixMilli()),
		"latest_sequence":          latestSequence, "retryable": retryable,
	}
	if attempt != nil {
		control["attempt_id"] = attempt.AttemptID
	}
	if errorCode != "" {
		control["error_code"] = errorCode
	}
	return writeCanonicalJournalStreamPayload(
		ctx,
		writer,
		"",
		canonicalJournalStreamEventControl,
		map[string]any{"kind": "control", "control": control},
	)
}

func writeCanonicalJournalStreamEnd(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	attempt *domainentity.RunAttempt,
	latestSequence uint64,
) bool {
	if attempt == nil {
		return false
	}
	return writeCanonicalJournalStreamPayload(ctx, writer, "", canonicalRunStreamEventEnd, map[string]any{
		"attempt_id": attempt.AttemptID, "status": string(attempt.Status),
		"latest_sequence": latestSequence, "reason": "terminal_attempt",
	})
}

func writeCanonicalJournalStreamPayload(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	id string,
	eventType string,
	payload any,
) bool {
	encoded, err := sonic.Marshal(payload)
	if err != nil {
		writeCanonicalRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent(id, eventType, encoded); err != nil {
		logs.CtxWarnf(
			ctx,
			"event_name=workbench.journal.stream.write_failed client_contract=%s stage=%s error_class=%T",
			canonicalContractVersion,
			canonicalLogEnum(eventType, canonicalRunStreamEventMetadata, canonicalRunStreamEventEvents,
				canonicalJournalStreamEventHeartbeat, canonicalJournalStreamEventControl,
				canonicalRunStreamEventEnd),
			err,
		)
		return false
	}
	return true
}

func writeCanonicalRunStreamProtocolEvent(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	event *appagentthread.RunEventSummary,
	streamModes map[string]struct{},
) bool {
	if event == nil {
		return true
	}

	eventType, eventPayload, ok := canonicalRunStreamEventPayload(event, streamModes)
	if !ok {
		return true
	}

	payload, err := sonic.Marshal(eventPayload)
	if err != nil {
		writeCanonicalRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent(strconv.FormatInt(event.EventID, 10), eventType, payload); err != nil {
		logs.CtxWarnf(
			ctx,
			"event_name=workbench.run.stream.write_failed client_contract=%s stage=event thread_id=%d run_id=%d event_id=%d error_class=%T",
			canonicalContractVersion,
			event.ThreadID,
			event.RunID,
			event.EventID,
			err,
		)
		return false
	}

	return true
}

func writeCanonicalRunStreamEnd(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	run *appagentthread.RunSummary,
) bool {
	if run == nil {
		return true
	}

	payload, err := sonic.Marshal(map[string]any{
		"run_id":    strconv.FormatInt(run.RunID, 10),
		"thread_id": strconv.FormatInt(run.ThreadID, 10),
		"status":    canonicalRunStreamStatus(run.Status),
		"reason":    "terminal_run",
	})
	if err != nil {
		writeCanonicalRunStreamError(ctx, writer, err)
		return false
	}
	if err := writer.WriteEvent("", canonicalRunStreamEventEnd, payload); err != nil {
		logs.CtxWarnf(
			ctx,
			"event_name=workbench.run.stream.write_failed client_contract=%s stage=end thread_id=%d run_id=%d error_class=%T",
			canonicalContractVersion,
			run.ThreadID,
			run.RunID,
			err,
		)
		return false
	}

	return true
}

func writeCanonicalRunStreamError(
	ctx context.Context,
	writer canonicalRunStreamProtocolWriter,
	err error,
) {
	if err == nil {
		return
	}
	publicError := appagentthread.ProjectPublicRuntimeError("runtime_failed", err.Error())
	payload, marshalErr := sonic.Marshal(publicError)
	if marshalErr != nil {
		payload = []byte(`{"code":"runtime_failed","message":"Agent run failed"}`)
	}
	if writeErr := writer.WriteEvent("", canonicalRunStreamEventError, payload); writeErr != nil {
		logs.CtxWarnf(ctx, "event_name=workbench.run.stream.write_failed client_contract=%s stage=error error_class=%T", canonicalContractVersion, writeErr)
	}
}

func canonicalRunStreamEventPayload(
	event *appagentthread.RunEventSummary,
	streamModes map[string]struct{},
) (string, any, bool) {
	projected := appagentthread.ProjectPublicRunEvent(event)
	if projected == nil {
		return "", nil, false
	}
	if len(streamModes) == 0 {
		return canonicalRunStreamEventEvents, canonicalPublicRunGenericEventPayload(projected), true
	}
	if _, ok := streamModes[canonicalRunStreamEventEvents]; ok {
		return canonicalRunStreamEventEvents, canonicalPublicRunGenericEventPayload(projected), true
	}

	mode := canonicalPublicRunStreamEventMode(projected)
	if _, ok := streamModes[mode]; !ok {
		return "", nil, false
	}

	switch mode {
	case canonicalRunStreamEventUpdates:
		return mode, canonicalPublicRunUpdateEventPayload(projected), true
	case canonicalRunStreamEventMessages:
		return mode, canonicalPublicRunMessageEventPayload(projected, false), true
	case "messages-tuple":
		return mode, canonicalPublicRunMessageEventPayload(projected, true), true
	case canonicalRunStreamEventValues:
		return mode, canonicalPublicRunValuesEventPayload(projected), true
	default:
		return mode, canonicalPublicRunModeEventPayload(projected), true
	}
}

func canonicalPublicRunGenericEventPayload(event *appagentthread.PublicRunEvent) map[string]any {
	if event == nil {
		return map[string]any{}
	}
	return map[string]any{
		"event_id":   strconv.FormatInt(event.EventID, 10),
		"thread_id":  strconv.FormatInt(event.ThreadID, 10),
		"run_id":     strconv.FormatInt(event.RunID, 10),
		"event_type": event.EventType,
		"payload":    canonicalRunEventPayload(event.Payload),
		"created_at": canonicalRunStreamTime(event.CreatedAt),
	}
}

func canonicalPublicRunUpdateEventPayload(event *appagentthread.PublicRunEvent) map[string]any {
	payload := canonicalRunEventPayloadMap(event.Payload)
	node := canonicalStringValue(payload["node"])
	if node == "" {
		node = "agent"
	}

	update := any(payload)
	if value, ok := payload["delta"]; ok {
		update = value
	} else if value, ok := payload["update"]; ok {
		update = value
	}

	return map[string]any{
		node:       update,
		"metadata": canonicalPublicRunStreamEventMetadata(event, node),
	}
}

func canonicalPublicRunMessageEventPayload(event *appagentthread.PublicRunEvent, tuple bool) any {
	payload := canonicalRunEventPayloadMap(event.Payload)
	node := canonicalStringValue(payload["node"])
	if node == "" {
		node = "agent"
	}

	chunk := any(payload)
	if value, ok := payload["chunk"]; ok {
		chunk = value
	} else if value, ok := payload["message"]; ok {
		chunk = value
	}
	metadata := canonicalPublicRunStreamEventMetadata(event, node)
	if tuple {
		return []any{chunk, metadata}
	}

	return map[string]any{
		"chunk":    chunk,
		"metadata": metadata,
	}
}

func canonicalPublicRunValuesEventPayload(event *appagentthread.PublicRunEvent) any {
	payload := canonicalRunEventPayloadMap(event.Payload)
	if value, ok := payload["values"]; ok {
		return value
	}

	return payload
}

func canonicalPublicRunModeEventPayload(event *appagentthread.PublicRunEvent) map[string]any {
	return map[string]any{
		"payload":  canonicalRunEventPayload(event.Payload),
		"metadata": canonicalPublicRunStreamEventMetadata(event, ""),
	}
}

func canonicalPublicRunStreamEventMetadata(
	event *appagentthread.PublicRunEvent,
	node string,
) map[string]any {
	metadata := map[string]any{
		"event_id":   strconv.FormatInt(event.EventID, 10),
		"thread_id":  strconv.FormatInt(event.ThreadID, 10),
		"run_id":     strconv.FormatInt(event.RunID, 10),
		"event_type": event.EventType,
		"created_at": canonicalRunStreamTime(event.CreatedAt),
	}
	if node != "" {
		metadata["node"] = node
	}

	return metadata
}

func canonicalPublicRunStreamEventMode(event *appagentthread.PublicRunEvent) string {
	eventType := strings.ToLower(strings.TrimSpace(event.EventType))
	switch {
	case eventType == canonicalRunStreamEventValues || strings.HasPrefix(eventType, "state.") || strings.HasPrefix(eventType, "checkpoint."):
		return canonicalRunStreamEventValues
	case eventType == canonicalRunStreamEventUpdates || strings.HasPrefix(eventType, "node.") || strings.HasPrefix(eventType, "step."):
		return canonicalRunStreamEventUpdates
	case eventType == canonicalRunStreamEventMessages || eventType == "messages-tuple" || strings.HasPrefix(eventType, "message.") || strings.HasPrefix(eventType, "llm."):
		if eventType == "messages-tuple" {
			return "messages-tuple"
		}
		return canonicalRunStreamEventMessages
	case eventType == canonicalRunStreamEventDebug || strings.HasPrefix(eventType, "debug."):
		return canonicalRunStreamEventDebug
	case eventType == canonicalRunStreamEventCustom || strings.HasPrefix(eventType, "custom."):
		return canonicalRunStreamEventCustom
	default:
		return canonicalRunStreamEventEvents
	}
}

func canonicalRunEventPayload(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}

	var payload any
	if err := sonic.UnmarshalString(raw, &payload); err != nil {
		return raw
	}

	return payload
}

func canonicalRunEventPayloadMap(raw string) map[string]any {
	payload := canonicalRunEventPayload(raw)
	if mapped, ok := payload.(map[string]any); ok {
		return mapped
	}

	return map[string]any{"value": payload}
}

func parseCanonicalRunEventCursor(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 0 {
		return 0, false
	}

	return parsed, true
}

func canonicalRunStreamStatus(status appagentthread.RunStatus) string {
	switch status {
	case appagentthread.RunStatusSucceeded:
		return "success"
	case appagentthread.RunStatusFailed:
		return "error"
	case appagentthread.RunStatusCanceled:
		return "interrupted"
	default:
		return string(status)
	}
}

func canonicalRequestedRunStreamModes(raw string, values []string) map[string]struct{} {
	modes := compactCanonicalRunStreamModes(values)
	if len(modes) == 0 {
		modes = canonicalRunStreamModeQueryValues(raw)
	}
	if len(modes) == 0 {
		return nil
	}

	result := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		result[mode] = struct{}{}
	}

	return result
}

func canonicalRunStreamModeQueryValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := sonic.UnmarshalString(raw, &values); err == nil {
			return compactCanonicalRunStreamModes(values)
		}
	}

	return compactCanonicalRunStreamModes(strings.Split(raw, ","))
}

func compactCanonicalRunStreamModes(values []string) []string {
	modes := make([]string, 0, len(values))
	for _, value := range values {
		mode := strings.TrimSpace(value)
		if mode != "" {
			modes = append(modes, mode)
		}
	}

	return modes
}

func canonicalStringValue(value any) string {
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed)
	}
	return ""
}

func canonicalRunStreamTime(ms int64) string {
	if ms <= 0 {
		return ""
	}

	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}
