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

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type ThreadStatus string

const (
	ThreadStatusIdle      ThreadStatus = "idle"
	ThreadStatusRunning   ThreadStatus = "running"
	ThreadStatusFailed    ThreadStatus = "failed"
	ThreadStatusCompleted ThreadStatus = "completed"
	ThreadStatusCanceled  ThreadStatus = "canceled"
)

type ThreadSource string

const (
	ThreadSourceWeb ThreadSource = "web"
	ThreadSourceIM  ThreadSource = "im"
	ThreadSourceAPI ThreadSource = "api"
)

type RunStatus string

const (
	RunStatusPending     RunStatus = "pending"
	RunStatusQueued      RunStatus = "queued"
	RunStatusRunning     RunStatus = "running"
	RunStatusInterrupted RunStatus = "interrupted"
	RunStatusSucceeded   RunStatus = "succeeded"
	RunStatusFailed      RunStatus = "failed"
	RunStatusCanceled    RunStatus = "canceled"
)

const (
	RunAwaitingInputInteractionRefSchema     = "coze.agentthread.awaiting_input.v1"
	RunAwaitingInputInteractionRefPayloadKey = "awaiting_input"
	maxRunAwaitingInputInteractionIDRunes    = 90
	maxRunAwaitingInputKindRunes             = 32
)

type RunAwaitingInputInteractionRef struct {
	Schema             string `json:"schema"`
	InteractionEventID string `json:"interaction_event_id"`
	InteractionID      string `json:"interaction_id,omitempty"`
	Kind               string `json:"kind,omitempty"`
}

func (r RunAwaitingInputInteractionRef) Normalized() RunAwaitingInputInteractionRef {
	return RunAwaitingInputInteractionRef{
		Schema:             strings.TrimSpace(r.Schema),
		InteractionEventID: strings.TrimSpace(r.InteractionEventID),
		InteractionID:      strings.TrimSpace(r.InteractionID),
		Kind:               strings.TrimSpace(r.Kind),
	}
}

func (r RunAwaitingInputInteractionRef) Validate() error {
	normalized := r.Normalized()
	if normalized.Schema != RunAwaitingInputInteractionRefSchema {
		return fmt.Errorf("awaiting-input interaction reference schema is invalid")
	}
	if !validRunAwaitingInputIdentifier(
		normalized.InteractionEventID,
		maxRunAwaitingInputInteractionIDRunes,
	) {
		return fmt.Errorf("awaiting-input interaction event id is invalid")
	}
	if normalized.InteractionID != "" &&
		!validRunAwaitingInputIdentifier(
			normalized.InteractionID,
			maxRunAwaitingInputInteractionIDRunes,
		) {
		return fmt.Errorf("awaiting-input interaction id is invalid")
	}
	if normalized.Kind != "" &&
		!validRunAwaitingInputIdentifier(normalized.Kind, maxRunAwaitingInputKindRunes) {
		return fmt.Errorf("awaiting-input interaction kind is invalid")
	}
	return nil
}

func RunAwaitingInputInteractionRefFromAny(
	value any,
) (RunAwaitingInputInteractionRef, error) {
	switch typed := value.(type) {
	case RunAwaitingInputInteractionRef:
		normalized := typed.Normalized()
		return normalized, normalized.Validate()
	case *RunAwaitingInputInteractionRef:
		if typed == nil {
			return RunAwaitingInputInteractionRef{}, fmt.Errorf("awaiting-input interaction reference is required")
		}
		normalized := typed.Normalized()
		return normalized, normalized.Validate()
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return RunAwaitingInputInteractionRef{}, err
		}
		var ref RunAwaitingInputInteractionRef
		if err := json.Unmarshal(raw, &ref); err != nil {
			return RunAwaitingInputInteractionRef{}, err
		}
		normalized := ref.Normalized()
		return normalized, normalized.Validate()
	}
}

func RunAwaitingInputInteractionRefFromEventPayload(
	payload string,
) (RunAwaitingInputInteractionRef, bool) {
	raw := strings.TrimSpace(payload)
	if raw == "" {
		return RunAwaitingInputInteractionRef{}, false
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(raw), &fields); err != nil || fields == nil {
		return RunAwaitingInputInteractionRef{}, false
	}
	value, exists := fields[RunAwaitingInputInteractionRefPayloadKey]
	if !exists {
		return RunAwaitingInputInteractionRef{}, false
	}
	ref, err := RunAwaitingInputInteractionRefFromAny(value)
	if err != nil {
		return RunAwaitingInputInteractionRef{}, false
	}
	return ref, true
}

func validRunAwaitingInputIdentifier(value string, maxRunes int) bool {
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) > maxRunes {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' ||
			r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' ||
			r == '.' ||
			r == '_' ||
			r == ':' ||
			r == '@' ||
			r == '-' {
			continue
		}
		return false
	}
	return true
}

type RunKind string

const (
	RunKindTask     RunKind = "task"
	RunKindSubagent RunKind = "subagent"
)

func DefaultRunKind(kind RunKind, parentRunID int64) RunKind {
	if kind != "" {
		return kind
	}
	if parentRunID > 0 {
		return RunKindSubagent
	}
	return RunKindTask
}

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleSystem    MessageRole = "system"
)

type MemoryScope string

const (
	MemoryScopeThread   MemoryScope = "thread"
	MemoryScopeRun      MemoryScope = "run"
	MemoryScopeLongTerm MemoryScope = "long_term"
)

type TranscriptKind string

const (
	TranscriptKindSummaryInput TranscriptKind = "summary_input"
	TranscriptKindTerminal     TranscriptKind = "terminal"
)

type MemoryFlushJobStatus string

const (
	MemoryFlushJobStatusPending    MemoryFlushJobStatus = "pending"
	MemoryFlushJobStatusProcessing MemoryFlushJobStatus = "processing"
	MemoryFlushJobStatusSucceeded  MemoryFlushJobStatus = "succeeded"
	MemoryFlushJobStatusFailed     MemoryFlushJobStatus = "failed"
)

type TokenUsageSource string

const (
	TokenUsageSourceLeadAgent  TokenUsageSource = "lead_agent"
	TokenUsageSourceSubagent   TokenUsageSource = "subagent"
	TokenUsageSourceMiddleware TokenUsageSource = "middleware"
	TokenUsageSourceTool       TokenUsageSource = "tool"
)

type Thread struct {
	ID            int64
	SpaceID       int64
	CreatorID     int64
	AgentID       int64
	Title         string
	Status        ThreadStatus
	Source        ThreadSource
	Metadata      string
	CreatedAt     int64
	UpdatedAt     int64
	LastMessageAt int64
}

type Run struct {
	ID                  int64
	ThreadID            int64
	ParentRunID         int64
	SpaceID             int64
	CreatorID           int64
	AssistantID         string
	RunKind             RunKind
	Status              RunStatus
	Command             string
	Input               string
	Config              string
	Context             string
	Metadata            string
	StreamMode          string
	MultitaskStrategy   string
	OnDisconnect        string
	Durability          string
	IdempotencyKey      string
	WorkerID            string
	LeaseOwner          string
	LeaseToken          string
	LeaseExpiresAt      int64
	HeartbeatAt         int64
	CancelRequestedAt   int64
	ExecutionGeneration uint64
	ErrorCode           string
	ErrorMessage        string
	StartedAt           int64
	EndedAt             int64
	CreatedAt           int64
	UpdatedAt           int64
}

type RunBacklogAggregate struct {
	Status RunStatus
	Config string
	Count  int64
}

type Message struct {
	ID        int64
	ThreadID  int64
	RunID     int64
	Role      MessageRole
	Content   string
	Metadata  string
	CreatedAt int64
}

type RunEvent struct {
	ID        int64
	ThreadID  int64
	RunID     int64
	EventType string
	Payload   string
	CreatedAt int64
}

type Checkpoint struct {
	ID                 int64
	ThreadID           int64
	RunID              int64
	ParentCheckpointID int64
	CheckpointNS       string
	RuntimeType        string
	RuntimeKey         string
	EnvelopeVersion    int32
	RuntimeDeletedAt   int64
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
	CreatedAt          int64
}

type Memory struct {
	ID                   int64
	ThreadID             int64
	RunID                int64
	SpaceID              int64
	Scope                MemoryScope
	Content              string
	Metadata             string
	Score                float64
	Confidence           float64
	SourceType           string
	SourceID             string
	CorrectionOfMemoryID int64
	CorrectedAt          int64
	ExpiresAt            int64
	CreatedAt            int64
	UpdatedAt            int64
	DeletedAt            int64
}

type MemoryAuditEvent struct {
	ID            int64
	ThreadID      int64
	RunID         int64
	SpaceID       int64
	MemoryID      int64
	ActorID       int64
	EventType     string
	Scope         MemoryScope
	SourceType    string
	SourceID      string
	AffectedCount int64
	CreatedAt     int64
}

type TranscriptSnapshot struct {
	ID             int64
	ThreadID       int64
	RunID          int64
	SpaceID        int64
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
	Metadata       string
	CreatedAt      int64
}

type MemoryFlushJob struct {
	ID                   int64
	ThreadID             int64
	RunID                int64
	SpaceID              int64
	UserID               int64
	AssistantID          string
	TranscriptSnapshotID int64
	IdempotencyKey       string
	Status               MemoryFlushJobStatus
	AttemptCount         int32
	WorkerID             string
	LastError            string
	AvailableAt          int64
	LeaseExpiresAt       int64
	StartedAt            int64
	EndedAt              int64
	CreatedAt            int64
	UpdatedAt            int64
}

type MemoryFlushBacklogAggregate struct {
	Status MemoryFlushJobStatus
	Count  int64
}

type TokenUsage struct {
	ID           int64
	ThreadID     int64
	RunID        int64
	SpaceID      int64
	Source       TokenUsageSource
	StepID       string
	StepIndex    int32
	StepName     string
	ModelName    string
	Provider     string
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	CostMicros   int64
	Currency     string
	Estimated    bool
	RawUsage     string
	Metadata     string
	CreatedAt    int64
}

type TokenUsageAggregate struct {
	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
	CostMicros       int64
	CallCount        int64
	LeadAgentTokens  int64
	SubagentTokens   int64
	MiddlewareTokens int64
	ToolTokens       int64
}

type RunTokenUsageAggregate struct {
	RunID     int64
	Aggregate *TokenUsageAggregate
}
