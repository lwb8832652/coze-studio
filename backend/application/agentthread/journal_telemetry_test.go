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
	"testing"

	"github.com/stretchr/testify/require"
)

type journalTelemetrySinkStub struct {
	events []JournalRecoveryTelemetryEvent
	err    error
}

func (s *journalTelemetrySinkStub) EmitJournalRecovery(
	_ context.Context,
	event JournalRecoveryTelemetryEvent,
) error {
	s.events = append(s.events, event)
	return s.err
}

func TestJournalTelemetryEmitsOnlyTypedRecoveryResult(t *testing.T) {
	sink := &journalTelemetrySinkStub{}
	telemetry := NewJournalTelemetry(sink)
	err := telemetry.RecordRecoveryResult(context.Background(), JournalRecoveryTelemetryEvent{
		EventName: "journal_recovery_result",
		RunID:     10, AttemptID: "att-1", TraceID: "trace-1",
		Action: "resume", Result: "success", ErrorCode: "none",
		Version: "1.1", RolloutCohort: "treatment", TaskType: "complex",
	})
	require.NoError(t, err)
	require.Len(t, sink.events, 1)

	encoded, err := json.Marshal(sink.events[0])
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	for _, forbidden := range []string{
		"body", "content", "command", "code", "cookie", "token", "user_input",
	} {
		require.NotContains(t, fields, forbidden)
	}
}

func TestJournalTelemetryRejectsUnknownEventAndUnboundedIdentifiers(t *testing.T) {
	sink := &journalTelemetrySinkStub{}
	telemetry := NewJournalTelemetry(sink)
	for _, event := range []JournalRecoveryTelemetryEvent{
		{EventName: "journal_first_visible", RunID: 10, AttemptID: "att-1", Action: "resume", Result: "success"},
		{EventName: "journal_recovery_result", RunID: 10, AttemptID: string(make([]byte, 300)), Action: "resume", Result: "success"},
		{EventName: "journal_recovery_result", RunID: 10, AttemptID: "att-1", Action: "unknown", Result: "success"},
	} {
		err := telemetry.RecordRecoveryResult(context.Background(), event)
		require.Error(t, err)
	}
	require.Empty(t, sink.events)
}
