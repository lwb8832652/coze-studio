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
	"fmt"
	"strings"
	"unicode"

	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

var ErrJournalTelemetryInvalid = errors.New("journal telemetry event is invalid")

type JournalRecoveryTelemetryEvent struct {
	EventName     string `json:"event_name"`
	RunID         int64  `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	TraceID       string `json:"trace_id,omitempty"`
	Action        string `json:"action"`
	Result        string `json:"result"`
	ErrorCode     string `json:"error_code"`
	Version       string `json:"version,omitempty"`
	RolloutCohort string `json:"rollout_cohort,omitempty"`
	TaskType      string `json:"task_type,omitempty"`
}

type JournalTelemetrySink interface {
	EmitJournalRecovery(context.Context, JournalRecoveryTelemetryEvent) error
}

type JournalTelemetry struct {
	sink JournalTelemetrySink
}

func NewJournalTelemetry(sink JournalTelemetrySink) *JournalTelemetry {
	return &JournalTelemetry{sink: sink}
}

func NewJournalLogTelemetry() *JournalTelemetry {
	return NewJournalTelemetry(journalRecoveryLogTelemetrySink{})
}

func (t *JournalTelemetry) RecordRecoveryResult(
	ctx context.Context,
	event JournalRecoveryTelemetryEvent,
) error {
	event.EventName = strings.TrimSpace(event.EventName)
	event.AttemptID = strings.TrimSpace(event.AttemptID)
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.Action = strings.TrimSpace(event.Action)
	event.Result = strings.TrimSpace(event.Result)
	event.ErrorCode = strings.TrimSpace(event.ErrorCode)
	event.Version = strings.TrimSpace(event.Version)
	event.RolloutCohort = strings.TrimSpace(event.RolloutCohort)
	event.TaskType = strings.TrimSpace(event.TaskType)
	if event.ErrorCode == "" {
		event.ErrorCode = "none"
	}
	if err := validateJournalRecoveryTelemetry(event); err != nil {
		return err
	}
	if t == nil || t.sink == nil {
		return nil
	}
	return t.sink.EmitJournalRecovery(ctx, event)
}

func validateJournalRecoveryTelemetry(event JournalRecoveryTelemetryEvent) error {
	if event.EventName != "journal_recovery_result" || event.RunID <= 0 ||
		!JournalRecoveryAction(event.Action).Valid() ||
		!journalTelemetryToken(event.AttemptID, 191, true) ||
		!journalTelemetryToken(event.TraceID, 128, false) ||
		!journalTelemetryToken(event.Result, 48, true) ||
		!journalTelemetryToken(event.ErrorCode, 64, true) ||
		!journalTelemetryToken(event.Version, 24, false) ||
		!journalTelemetryToken(event.RolloutCohort, 24, false) ||
		!journalTelemetryToken(event.TaskType, 24, false) {
		return ErrJournalTelemetryInvalid
	}
	return nil
}

func journalTelemetryToken(value string, maxLength int, required bool) bool {
	if value == "" {
		return !required
	}
	if len(value) > maxLength {
		return false
	}
	for _, r := range value {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			r != '_' && r != '-' && r != '.' && r != ':' {
			return false
		}
	}
	return true
}

type journalRecoveryLogTelemetrySink struct{}

func (journalRecoveryLogTelemetrySink) EmitJournalRecovery(
	ctx context.Context,
	event JournalRecoveryTelemetryEvent,
) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal Journal recovery telemetry: %w", err)
	}
	logs.CtxInfof(ctx, "[journal-telemetry] %s", payload)
	return nil
}
