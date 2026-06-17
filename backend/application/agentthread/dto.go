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

type TokenUsageSource string

const (
	TokenUsageSourceLeadAgent  TokenUsageSource = "lead_agent"
	TokenUsageSourceSubagent   TokenUsageSource = "subagent"
	TokenUsageSourceMiddleware TokenUsageSource = "middleware"
	TokenUsageSourceTool       TokenUsageSource = "tool"
)

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
)

type ThreadSummary struct {
	ThreadID         int64
	LegacyTaskID     int64
	SpaceID          int64
	CreatorID        int64
	Title            string
	Status           ThreadStatus
	Source           ThreadSource
	Progress         int32
	Metadata         string
	LastUserMessage  string
	LastAgentMessage string
	CreatedAt        int64
	UpdatedAt        int64
}

type MessageSummary struct {
	MessageID int64
	ThreadID  int64
	RunID     int64
	Role      MessageRole
	Content   string
	Metadata  string
	CreatedAt int64
}

type RunSummary struct {
	RunID             int64
	ThreadID          int64
	SpaceID           int64
	CreatorID         int64
	AssistantID       string
	Status            RunStatus
	Command           string
	Input             string
	Config            string
	Context           string
	Metadata          string
	StreamMode        string
	MultitaskStrategy string
	OnDisconnect      string
	Durability        string
	IdempotencyKey    string
	WorkerID          string
	ErrorCode         string
	ErrorMessage      string
	StartedAt         int64
	EndedAt           int64
	CreatedAt         int64
	UpdatedAt         int64
}

type RunEventSummary struct {
	EventID   int64
	ThreadID  int64
	RunID     int64
	EventType string
	Payload   string
	CreatedAt int64
}

type CheckpointSummary struct {
	CheckpointID       int64
	ThreadID           int64
	RunID              int64
	ParentCheckpointID int64
	CheckpointNS       string
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
	CreatedAt          int64
}

type MemorySummary struct {
	MemoryID  int64
	ThreadID  int64
	RunID     int64
	SpaceID   int64
	Scope     MemoryScope
	Content   string
	Metadata  string
	Score     float64
	ExpiresAt int64
	CreatedAt int64
	UpdatedAt int64
}

type TokenUsageSummary struct {
	UsageID      int64
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

type TokenUsageAggregateSummary struct {
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

type CreateThreadRequest struct {
	SpaceID      int64
	UserID       int64
	AgentID      int64
	Title        string
	Source       ThreadSource
	LegacyTaskID int64
	Metadata     string
}

type CreateThreadResponse struct {
	Thread *ThreadSummary
}

type GetThreadRequest struct {
	ThreadID int64
}

type GetThreadResponse struct {
	Thread *ThreadSummary
}

type ListThreadsRequest struct {
	SpaceID  int64
	UserID   int64
	Status   *ThreadStatus
	Page     int32
	PageSize int32
}

type ListThreadsResponse struct {
	Threads []*ThreadSummary
	Total   int64
}

type AppendMessageRequest struct {
	ThreadID int64
	RunID    int64
	Role     MessageRole
	Content  string
	Metadata string
}

type AppendMessageResponse struct {
	Message *MessageSummary
}

type ListMessagesRequest struct {
	ThreadID int64
	Page     int32
	PageSize int32
}

type ListMessagesResponse struct {
	Messages []*MessageSummary
	Total    int64
}

type CreateRunRequest struct {
	ThreadID          int64
	AssistantID       string
	Status            RunStatus
	Command           string
	Input             string
	Config            string
	Context           string
	Metadata          string
	StreamMode        string
	MultitaskStrategy string
	OnDisconnect      string
	Durability        string
	IdempotencyKey    string
}

type CreateRunResponse struct {
	Run *RunSummary
}

type GetRunRequest struct {
	RunID int64
}

type GetRunResponse struct {
	Run *RunSummary
}

type ListRunsRequest struct {
	ThreadID int64
	Status   *RunStatus
	Page     int32
	PageSize int32
}

type ListRunsResponse struct {
	Runs  []*RunSummary
	Total int64
}

type AppendRunEventRequest struct {
	ThreadID  int64
	RunID     int64
	EventType string
	Payload   string
}

type AppendRunEventResponse struct {
	Event *RunEventSummary
}

type ListRunEventsRequest struct {
	ThreadID int64
	RunID    int64
	Page     int32
	PageSize int32
}

type ListRunEventsResponse struct {
	Events []*RunEventSummary
	Total  int64
}

type CreateCheckpointRequest struct {
	ThreadID           int64
	RunID              int64
	ParentCheckpointID int64
	CheckpointNS       string
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
}

type CreateCheckpointResponse struct {
	Checkpoint *CheckpointSummary
}

type ListCheckpointsRequest struct {
	ThreadID int64
	RunID    int64
	Limit    int32
}

type ListCheckpointsResponse struct {
	Checkpoints []*CheckpointSummary
	Total       int64
}

type GetCheckpointRequest struct {
	CheckpointID int64
}

type GetCheckpointResponse struct {
	Checkpoint *CheckpointSummary
}

type GetLatestCheckpointRequest struct {
	ThreadID int64
}

type GetLatestCheckpointResponse struct {
	Checkpoint *CheckpointSummary
}

type RememberMemoryRequest struct {
	ThreadID  int64
	RunID     int64
	Scope     MemoryScope
	Content   string
	Metadata  string
	Score     float64
	ExpiresAt int64
}

type RememberMemoryResponse struct {
	Memory *MemorySummary
}

type RecallMemoriesRequest struct {
	ThreadID int64
	RunID    int64
	Scopes   []MemoryScope
	Limit    int32
}

type RecallMemoriesResponse struct {
	Memories []*MemorySummary
	Total    int64
}

type RecordTokenUsageRequest struct {
	RunID        int64
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
}

type RecordTokenUsageResponse struct {
	Usage *TokenUsageSummary
}

type GetTokenUsageRequest struct {
	ThreadID int64
	RunID    int64
	Source   TokenUsageSource
	Page     int32
	PageSize int32
}

type GetTokenUsageResponse struct {
	Usage     []*TokenUsageSummary
	Total     int64
	Aggregate *TokenUsageAggregateSummary
}

type ClaimPendingRunsRequest struct {
	WorkerID string
	Limit    int32
}

type ClaimPendingRunsResponse struct {
	Runs []*RunSummary
}

type UpdateRunStatusRequest struct {
	RunID        int64
	From         RunStatus
	To           RunStatus
	WorkerID     string
	ErrorCode    string
	ErrorMessage string
}

type UpdateRunStatusResponse struct {
	Run *RunSummary
}
