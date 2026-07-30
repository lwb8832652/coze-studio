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

package entity

import "strings"

const (
	JournalSchemaVersion  = "1.1"
	JournalPayloadVersion = "1.0"
)

type RunAttemptStatus string

const (
	RunAttemptStatusPending   RunAttemptStatus = "pending"
	RunAttemptStatusRunning   RunAttemptStatus = "running"
	RunAttemptStatusCompleted RunAttemptStatus = "completed"
	RunAttemptStatusFailed    RunAttemptStatus = "failed"
	RunAttemptStatusCancelled RunAttemptStatus = "cancelled"
	RunAttemptStatusTimedOut  RunAttemptStatus = "timed_out"

	// Compatibility aliases keep existing internal callers source-compatible
	// while persisting only the frozen wire values above.
	RunAttemptStatusActive   = RunAttemptStatusRunning
	RunAttemptStatusCanceled = RunAttemptStatusCancelled
)

func (s RunAttemptStatus) IsActive() bool {
	return s == RunAttemptStatusPending || s == RunAttemptStatusRunning
}

func (s RunAttemptStatus) IsTerminal() bool {
	switch s {
	case RunAttemptStatusCompleted, RunAttemptStatusFailed,
		RunAttemptStatusCancelled, RunAttemptStatusTimedOut:
		return true
	default:
		return false
	}
}

// IsJournalTimeoutErrorCode recognizes only server-owned timeout categories.
func IsJournalTimeoutErrorCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "deadline_exceeded", "run_timeout", "task_timeout", "tool_timeout", "subagent_timeout", "timeout":
		return true
	default:
		return false
	}
}

type JournalProjectionState string

const (
	JournalProjectionStateHealthy  JournalProjectionState = "healthy"
	JournalProjectionStateDegraded JournalProjectionState = "degraded"
	JournalProjectionStateDisabled JournalProjectionState = "disabled"
)

type JournalVisibility string

const (
	JournalVisibilityUser     JournalVisibility = "user"
	JournalVisibilityInternal JournalVisibility = "internal"

	// Public was the pre-freeze internal name. It now aliases the user scope.
	JournalVisibilityPublic = JournalVisibilityUser
)

type RunAttempt struct {
	ID                     int64
	ThreadID               int64
	JournalRunID           int64
	ExecutionRunID         int64
	AttemptID              string
	Ordinal                uint32
	Status                 RunAttemptStatus
	ActiveSlot             *uint8
	NextSequence           uint64
	LastCommittedSequence  uint64
	SourceCheckpointID     *int64
	SourceAttemptID        *string
	RecoveryIdempotencyKey *string
	EnrollmentVersion      string
	SnapshotsEnabled       bool
	ProjectionState        JournalProjectionState
	ProjectionDegradedAt   *int64
	TraceID                *string
	TerminalEventID        *int64
	CreatedAt              int64
	UpdatedAt              int64
	StartedAt              *int64
	EndedAt                *int64
}

type JournalEvent struct {
	ID                 int64
	ThreadID           int64
	RunID              int64
	JournalRunID       int64
	AttemptID          string
	Sequence           uint64
	IdempotencyKey     string
	ParentEventID      int64
	SchemaVersion      string
	Status             string
	OccurredAtUnixNano int64
	Visibility         JournalVisibility
	PayloadVersion     string
	SnapshotID         string
	TraceID            string
	ActionID           string
	Phase              string
	Operation          string
	Target             string
	Milestone          string
	EventType          string
	Payload            string
	CreatedAt          int64
}
