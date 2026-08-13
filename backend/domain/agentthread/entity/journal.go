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
	RunAttemptStatusPending     RunAttemptStatus = "pending"
	RunAttemptStatusRunning     RunAttemptStatus = "running"
	RunAttemptStatusCompleted   RunAttemptStatus = "completed"
	RunAttemptStatusFailed      RunAttemptStatus = "failed"
	RunAttemptStatusCancelled   RunAttemptStatus = "cancelled"
	RunAttemptStatusTimedOut    RunAttemptStatus = "timed_out"
	RunAttemptStatusInterrupted RunAttemptStatus = "interrupted"

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

type SideEffectReplayPolicy string

const (
	SideEffectReplayPolicyReadOnly        SideEffectReplayPolicy = "read_only"
	SideEffectReplayPolicyIdempotentWrite SideEffectReplayPolicy = "idempotent_write"
	SideEffectReplayPolicyNonReplayable   SideEffectReplayPolicy = "non_replayable"
)

func (p SideEffectReplayPolicy) Valid() bool {
	switch p {
	case SideEffectReplayPolicyReadOnly,
		SideEffectReplayPolicyIdempotentWrite,
		SideEffectReplayPolicyNonReplayable:
		return true
	default:
		return false
	}
}

func (p SideEffectReplayPolicy) AllowsAutomaticRecovery() bool {
	return p == SideEffectReplayPolicyReadOnly ||
		p == SideEffectReplayPolicyIdempotentWrite
}

type SideEffectLedgerStatus string

const (
	SideEffectLedgerStatusPrepared    SideEffectLedgerStatus = "prepared"
	SideEffectLedgerStatusExecuting   SideEffectLedgerStatus = "executing"
	SideEffectLedgerStatusSucceeded   SideEffectLedgerStatus = "succeeded"
	SideEffectLedgerStatusFailed      SideEffectLedgerStatus = "failed"
	SideEffectLedgerStatusUnknown     SideEffectLedgerStatus = "unknown"
	SideEffectLedgerStatusCompensated SideEffectLedgerStatus = "compensated"
)

func (s SideEffectLedgerStatus) Valid() bool {
	switch s {
	case SideEffectLedgerStatusPrepared,
		SideEffectLedgerStatusExecuting,
		SideEffectLedgerStatusSucceeded,
		SideEffectLedgerStatusFailed,
		SideEffectLedgerStatusUnknown,
		SideEffectLedgerStatusCompensated:
		return true
	default:
		return false
	}
}

func (s SideEffectLedgerStatus) IsTerminal() bool {
	switch s {
	case SideEffectLedgerStatusSucceeded,
		SideEffectLedgerStatusFailed,
		SideEffectLedgerStatusUnknown,
		SideEffectLedgerStatusCompensated:
		return true
	default:
		return false
	}
}

type SideEffectResolutionAction string

const (
	SideEffectResolutionActionMarkSucceeded SideEffectResolutionAction = "mark_succeeded"
	SideEffectResolutionActionSkip          SideEffectResolutionAction = "skip"
	SideEffectResolutionActionRetry         SideEffectResolutionAction = "retry"
)

func (a SideEffectResolutionAction) Valid() bool {
	switch a {
	case SideEffectResolutionActionMarkSucceeded,
		SideEffectResolutionActionSkip,
		SideEffectResolutionActionRetry:
		return true
	default:
		return false
	}
}

type SideEffectLedger struct {
	ID                       int64
	ThreadID                 int64
	JournalRunID             int64
	AttemptID                string
	IdempotencyKey           string
	ActionKind               string
	ReplayPolicy             SideEffectReplayPolicy
	Status                   SideEffectLedgerStatus
	RequestHash              string
	RequestSummary           string
	ExternalReferenceDigest  string
	ResultSnapshotID         string
	ResultEventID            *int64
	CheckpointID             *int64
	CompensationKind         string
	ResolutionAction         SideEffectResolutionAction
	ResolutionIdempotencyKey string
	Version                  uint64
	PreparedAt               int64
	ExecutingAt              *int64
	SucceededAt              *int64
	FailedAt                 *int64
	UnknownAt                *int64
	CompensatedAt            *int64
	ResolvedAt               *int64
	CreatedAt                int64
	UpdatedAt                int64
}

type JournalSnapshotContentType string

const (
	JournalSnapshotContentTypeDocument JournalSnapshotContentType = "document"
	JournalSnapshotContentTypeTerminal JournalSnapshotContentType = "terminal"
	JournalSnapshotContentTypeCode     JournalSnapshotContentType = "code"
	JournalSnapshotContentTypeSkill    JournalSnapshotContentType = "skill"
	JournalSnapshotContentTypeBrowser  JournalSnapshotContentType = "browser"
)

func (t JournalSnapshotContentType) Valid() bool {
	switch t {
	case JournalSnapshotContentTypeDocument, JournalSnapshotContentTypeTerminal,
		JournalSnapshotContentTypeCode, JournalSnapshotContentTypeSkill,
		JournalSnapshotContentTypeBrowser:
		return true
	default:
		return false
	}
}

type JournalContentStatus string

const (
	JournalContentStatusEmpty        JournalContentStatus = "empty"
	JournalContentStatusLoading      JournalContentStatus = "loading"
	JournalContentStatusStreaming    JournalContentStatus = "streaming"
	JournalContentStatusReady        JournalContentStatus = "ready"
	JournalContentStatusError        JournalContentStatus = "error"
	JournalContentStatusNoPermission JournalContentStatus = "no_permission"
)

func (s JournalContentStatus) Valid() bool {
	switch s {
	case JournalContentStatusEmpty, JournalContentStatusLoading,
		JournalContentStatusStreaming, JournalContentStatusReady,
		JournalContentStatusError, JournalContentStatusNoPermission:
		return true
	default:
		return false
	}
}

type JournalSnapshotCompression string

const (
	JournalSnapshotCompressionIdentity JournalSnapshotCompression = "identity"
)

func (c JournalSnapshotCompression) Valid() bool {
	return c == JournalSnapshotCompressionIdentity
}

type JournalSnapshotCleanupState string

const (
	JournalSnapshotCleanupStateActive   JournalSnapshotCleanupState = "active"
	JournalSnapshotCleanupStatePending  JournalSnapshotCleanupState = "pending"
	JournalSnapshotCleanupStateDeleting JournalSnapshotCleanupState = "deleting"
	JournalSnapshotCleanupStateFailed   JournalSnapshotCleanupState = "failed"
)

func (s JournalSnapshotCleanupState) Valid() bool {
	switch s {
	case JournalSnapshotCleanupStateActive, JournalSnapshotCleanupStatePending,
		JournalSnapshotCleanupStateDeleting, JournalSnapshotCleanupStateFailed:
		return true
	default:
		return false
	}
}

type JournalContentSnapshot struct {
	SnapshotID         string
	SpaceID            int64
	ThreadID           int64
	RunID              int64
	JournalRunID       int64
	AttemptID          string
	EventID            int64
	ActionID           string
	Revision           uint32
	ContentType        JournalSnapshotContentType
	Status             JournalContentStatus
	IsFragmented       bool
	FragmentCount      uint32
	Visibility         JournalVisibility
	ErrorCode          string
	MIMEType           string
	Encoding           string
	Compression        JournalSnapshotCompression
	ContentJSON        string
	ObjectKey          string
	SummaryJSON        string
	SummaryHash        string
	ContentLength      int64
	ContentHash        string
	ACLDomain          string
	SourceResourceType string
	SourceResourceID   string
	SourceRevision     string
	OriginalObjectKey  string
	ExpiresAt          int64
	CleanupState       JournalSnapshotCleanupState
	DeletedAt          int64
	CreatedAt          int64
	Fragments          []*JournalSnapshotFragment
}

const JournalSnapshotRetentionMillis int64 = 30 * 24 * 60 * 60 * 1000

type JournalSnapshotReservation struct {
	SnapshotID       string
	ReservationToken string
	SpaceID          int64
	ThreadID         int64
	RunID            int64
	JournalRunID     int64
	AttemptID        string
	ActionID         string
	Revision         uint32
	EventID          int64
	IdempotencyKey   string
	ContentHash      string
	ACLDomain        string
	StagingPrefix    string
	ExpiresAt        int64
	CreatedAt        int64
}

type JournalSnapshotFragment struct {
	FragmentID    string
	SnapshotID    string
	FragmentIndex int32
	Kind          JournalSnapshotFragmentKind
	MetadataJSON  string
	MIMEType      string
	InlineContent string
	ObjectKey     string
	ByteStart     int64
	ByteEnd       int64
	SizeBytes     int64
	ContentHash   string
	CreatedAt     int64
}

type JournalSnapshotFragmentKind string

const (
	JournalSnapshotFragmentKindDocumentBlock    JournalSnapshotFragmentKind = "document_block"
	JournalSnapshotFragmentKindDocumentChapters JournalSnapshotFragmentKind = "document_chapters"
	JournalSnapshotFragmentKindTerminalStdout   JournalSnapshotFragmentKind = "terminal_stdout"
	JournalSnapshotFragmentKindTerminalStderr   JournalSnapshotFragmentKind = "terminal_stderr"
	JournalSnapshotFragmentKindCodeLines        JournalSnapshotFragmentKind = "code_lines"
	JournalSnapshotFragmentKindCodeHighlights   JournalSnapshotFragmentKind = "code_highlights"
	JournalSnapshotFragmentKindSkillItems       JournalSnapshotFragmentKind = "skill_items"
	JournalSnapshotFragmentKindBrowserThumbnail JournalSnapshotFragmentKind = "browser_thumbnail"
	JournalSnapshotFragmentKindBrowserSnapshot  JournalSnapshotFragmentKind = "browser_snapshot"
	JournalSnapshotFragmentKindBrowserAnalysis  JournalSnapshotFragmentKind = "browser_analysis"
)

func (k JournalSnapshotFragmentKind) Valid() bool {
	switch k {
	case JournalSnapshotFragmentKindDocumentBlock,
		JournalSnapshotFragmentKindDocumentChapters,
		JournalSnapshotFragmentKindTerminalStdout,
		JournalSnapshotFragmentKindTerminalStderr,
		JournalSnapshotFragmentKindCodeLines,
		JournalSnapshotFragmentKindCodeHighlights,
		JournalSnapshotFragmentKindSkillItems,
		JournalSnapshotFragmentKindBrowserThumbnail,
		JournalSnapshotFragmentKindBrowserSnapshot,
		JournalSnapshotFragmentKindBrowserAnalysis:
		return true
	default:
		return false
	}
}

type JournalSnapshotAction string

const (
	JournalSnapshotActionReadMetadata     JournalSnapshotAction = "read_metadata"
	JournalSnapshotActionReadContent      JournalSnapshotAction = "read_content"
	JournalSnapshotActionCopyCommand      JournalSnapshotAction = "copy_command"
	JournalSnapshotActionCopyOutput       JournalSnapshotAction = "copy_output"
	JournalSnapshotActionCopyCode         JournalSnapshotAction = "copy_code"
	JournalSnapshotActionOpenOriginal     JournalSnapshotAction = "open_original"
	JournalSnapshotActionDownloadFragment JournalSnapshotAction = "download_fragment"
)

func (a JournalSnapshotAction) Valid() bool {
	switch a {
	case JournalSnapshotActionReadMetadata, JournalSnapshotActionReadContent,
		JournalSnapshotActionCopyCommand, JournalSnapshotActionCopyOutput,
		JournalSnapshotActionCopyCode, JournalSnapshotActionOpenOriginal,
		JournalSnapshotActionDownloadFragment:
		return true
	default:
		return false
	}
}

func (a JournalSnapshotAction) UserAction() bool {
	switch a {
	case JournalSnapshotActionCopyCommand, JournalSnapshotActionCopyOutput,
		JournalSnapshotActionCopyCode, JournalSnapshotActionOpenOriginal,
		JournalSnapshotActionDownloadFragment:
		return true
	default:
		return false
	}
}

type JournalSnapshotPermissionResult string

const (
	JournalSnapshotPermissionAllowed JournalSnapshotPermissionResult = "allowed"
	JournalSnapshotPermissionDenied  JournalSnapshotPermissionResult = "denied"
	JournalSnapshotPermissionExpired JournalSnapshotPermissionResult = "expired"
)

type JournalSnapshotAccessAudit struct {
	ID               int64
	SpaceID          int64
	ThreadID         int64
	RunID            int64
	AttemptID        string
	SnapshotID       string
	ContentType      JournalSnapshotContentType
	Action           JournalSnapshotAction
	ActorID          int64
	PermissionResult JournalSnapshotPermissionResult
	IdempotencyKey   string
	TargetHash       string
	TraceID          string
	CreatedAt        int64
}
