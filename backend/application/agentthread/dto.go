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

type ArtifactPreviewMode string

const (
	ArtifactPreviewModeText        ArtifactPreviewMode = "text"
	ArtifactPreviewModeImage       ArtifactPreviewMode = "image"
	ArtifactPreviewModePDF         ArtifactPreviewMode = "pdf"
	ArtifactPreviewModeDownload    ArtifactPreviewMode = "download"
	ArtifactPreviewModeUnsupported ArtifactPreviewMode = "unsupported"
)

type ArtifactScanJobStatus string

const (
	ArtifactScanJobStatusPending    ArtifactScanJobStatus = "pending"
	ArtifactScanJobStatusProcessing ArtifactScanJobStatus = "processing"
	ArtifactScanJobStatusSucceeded  ArtifactScanJobStatus = "succeeded"
	ArtifactScanJobStatusFailed     ArtifactScanJobStatus = "failed"
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

type RunKind string

const (
	RunKindTask     RunKind = "task"
	RunKindSubagent RunKind = "subagent"
)

type ThreadSummary struct {
	ThreadID         int64
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
	RunID               int64
	PlanScopeRunID      int64
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

type MemorySummary struct {
	MemoryID             int64
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

type MemoryAuditEventSummary struct {
	EventID       int64
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

type GuardrailAuditEventSummary struct {
	EventID    int64
	ThreadID   int64
	RunID      int64
	SpaceID    int64
	ActorID    int64
	EventType  string
	TargetType string
	TargetID   string
	Operation  string
	Source     string
	Action     string
	FailMode   string
	Provider   string
	ReasonCode string
	RuleIDs    string
	CreatedAt  int64
}

type MCPRuntimeAuditEventSummary struct {
	EventID         int64
	SpaceID         int64
	ThreadID        int64
	RunID           int64
	ServerID        int64
	RuntimeToolName string
	EventType       string
	ErrorCode       string
	ElapsedMillis   int64
	OutputBytes     int64
	CreatedAt       int64
}

const GuardrailAuditExportSchema = "coze.task_thread_guardrail_audit.export.v1"

type ExportGuardrailAuditEventsRequest struct {
	ThreadID int64
	RunID    int64
	ViewerID int64
	Page     int32
	PageSize int32
}

type ExportGuardrailAuditEventsResponse struct {
	Schema     string
	ThreadID   int64
	ExportedAt int64
	Page       int32
	PageSize   int32
	Total      int64
	Events     []*GuardrailAuditEventSummary
}

const MemoryExportSchema = "coze.task_thread_memories.export.v1"

type ExportMemoriesRequest struct {
	ThreadID       int64
	ViewerID       int64
	RunID          int64
	Scopes         []MemoryScope
	Query          string
	IncludeExpired bool
	IncludeDeleted bool
	Limit          int32
}

type ExportMemoriesResponse struct {
	Schema     string
	ThreadID   int64
	ExportedAt int64
	Total      int64
	Memories   []*MemorySummary
}

type ImportMemoryItem struct {
	RunID                int64
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
}

type ImportMemoriesRequest struct {
	ThreadID int64
	ActorID  int64
	ViewerID int64
	Memories []ImportMemoryItem
}

type ImportMemoriesResponse struct {
	Imported int64
	Skipped  int64
	Memories []*MemorySummary
}

type TranscriptSnapshotSummary struct {
	SnapshotID     int64
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

type MemoryFlushJobSummary struct {
	JobID                int64
	ThreadID             int64
	RunID                int64
	SpaceID              int64
	UserID               int64
	AssistantID          string
	TranscriptSnapshotID int64
	IdempotencyKey       string
	Status               string
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

type ArtifactSummary struct {
	ArtifactID   int64
	SpaceID      int64
	ThreadID     int64
	RunID        int64
	FileID       int64
	Title        string
	ArtifactType string
	VirtualPath  string
	ContentType  string
	SizeBytes    int64
	PreviewMode  ArtifactPreviewMode
	Metadata     string
	CreatedAt    int64
	UpdatedAt    int64
	DeletedAt    int64
}

type ArtifactScanJobSummary struct {
	JobID          int64
	ThreadID       int64
	RunID          int64
	SpaceID        int64
	UserID         int64
	ArtifactID     int64
	FileID         int64
	Scanner        string
	Status         ArtifactScanJobStatus
	WorkerID       string
	AttemptCount   int32
	LastError      string
	AvailableAt    int64
	LeaseExpiresAt int64
	StartedAt      int64
	EndedAt        int64
	CreatedAt      int64
	UpdatedAt      int64
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

type RunTokenUsageAggregateSummary struct {
	RunID     int64
	Aggregate *TokenUsageAggregateSummary
}

type CreateThreadRequest struct {
	SpaceID  int64
	UserID   int64
	AgentID  int64
	Title    string
	Source   ThreadSource
	Metadata string
}

type CreateThreadResponse struct {
	Thread *ThreadSummary
}

type CreateTaskThreadRequest struct {
	SpaceID int64
	UserID  int64
	Message string
	Title   string
	// Zero values keep the legacy Workbench thread source and metadata.
	ThreadMetadata    string
	ThreadSource      ThreadSource
	DeferStart        bool
	AssistantID       string
	Command           string
	Config            string
	Context           string
	Metadata          string
	StreamMode        string
	MultitaskStrategy string
	OnDisconnect      string
	Durability        string
	IdempotencyKey    string
	// Canonical-only replay fields remain empty for all legacy callers.
	IdempotencyOperation   string
	IdempotencyFingerprint string
}

type CreateTaskThreadResponse struct {
	Thread  *ThreadSummary
	Message *MessageSummary
	Run     *RunSummary
}

type GetThreadRequest struct {
	ThreadID int64
}

type GetThreadResponse struct {
	Thread *ThreadSummary
}

type UpdateThreadTitleRequest struct {
	ThreadID int64
	Title    string
}

type UpdateThreadTitleResponse struct {
	Thread  *ThreadSummary
	Updated bool
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

type UpdateThreadMetadataRequest struct {
	ThreadID int64
	Metadata string
}

type UpdateThreadMetadataResponse struct {
	Thread  *ThreadSummary
	Updated bool
}

type DeleteThreadRequest struct {
	ThreadID int64
}

type DeleteThreadResponse struct {
	Deleted bool
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

type ListRecentPublicMessagesRequest struct {
	ThreadID int64
	Limit    int32
}

type ListRecentPublicMessagesResponse struct {
	Messages []*PublicMessage
}

type CreateRunRequest struct {
	ThreadID                 int64
	ParentRunID              int64
	TopLevelRetrySourceRunID int64
	AssistantID              string
	RunKind                  RunKind
	Status                   RunStatus
	Command                  string
	Input                    string
	Config                   string
	Context                  string
	Metadata                 string
	StreamMode               string
	MultitaskStrategy        string
	OnDisconnect             string
	Durability               string
	IdempotencyKey           string
	IdempotencyOperation     string
	IdempotencyFingerprint   string
	MessageContent           string
	MessageMetadata          string
	PersistMessageReference  bool
}

type CreateRunResponse struct {
	Run     *RunSummary
	Message *MessageSummary
}

type ResumeHumanInteractionRequest struct {
	ThreadID                int64
	SourceRunID             int64
	InterruptID             string
	Response                HumanInteractionResponse
	IdempotencyKey          string
	IdempotencyOperation    string
	IdempotencyFingerprint  string
	PersistMessageReference bool
}

type ResumeHumanInteractionResponse struct {
	Run *RunSummary
}

type RetrySubagentRunRequest struct {
	ThreadID       int64
	SourceRunID    int64
	IdempotencyKey string
}

type RetrySubagentRunResponse struct {
	Run *RunSummary
}

type GetRunRequest struct {
	RunID int64
}

type GetRunResponse struct {
	Run *RunSummary
}

// GetRunByIdempotencyKeyRequest resolves an optional existing Run only within
// the already-authorized Thread. Idempotency keys are scoped by the Thread's
// server-owned SpaceID rather than any caller-supplied ownership field.
type GetRunByIdempotencyKeyRequest struct {
	ThreadID               int64
	IdempotencyKey         string
	IdempotencyOperation   string
	IdempotencyFingerprint string
}

type GetRunByIdempotencyKeyResponse struct {
	Run *RunSummary
}

type ListRunsRequest struct {
	ThreadID         int64
	ParentRunID      *int64
	IncludeChildRuns bool
	Status           *RunStatus
	Page             int32
	PageSize         int32
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
	ThreadID     int64
	RunID        int64
	AfterEventID int64
	Page         int32
	PageSize     int32
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
	RuntimeType        string
	RuntimeKey         string
	EnvelopeVersion    int32
	ChannelValues      string
	ChannelVersions    string
	PendingSends       string
	Metadata           string
}

type CreateCheckpointResponse struct {
	Checkpoint *CheckpointSummary
}

type ListCheckpointsRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	Limit       int32
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

type GetLatestRuntimeCheckpointRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	RuntimeKey  string
}

type GetLatestRuntimeCheckpointResponse struct {
	Checkpoint *CheckpointSummary
}

type DeleteRuntimeCheckpointRequest struct {
	ThreadID    int64
	RunID       int64
	RuntimeType string
	RuntimeKey  string
	DeletedAt   int64
}

type RememberMemoryRequest struct {
	ThreadID             int64
	RunID                int64
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
}

type RememberMemoryResponse struct {
	Memory *MemorySummary
}

type RecallMemoriesRequest struct {
	ThreadID int64
	RunID    int64
	Scopes   []MemoryScope
	Query    string
	Limit    int32
}

type RecallMemoriesResponse struct {
	Memories []*MemorySummary
	Total    int64
}

type ListMemoriesRequest struct {
	ThreadID       int64
	ViewerID       int64
	RunID          int64
	Scopes         []MemoryScope
	Query          string
	IncludeExpired bool
	IncludeDeleted bool
	Page           int32
	PageSize       int32
}

type ListMemoriesResponse struct {
	Memories []*MemorySummary
	Total    int64
}

type UpdateMemoryRequest struct {
	ThreadID             int64
	MemoryID             int64
	ActorID              int64
	ViewerID             int64
	RunID                int64
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
}

type UpdateMemoryResponse struct {
	Memory  *MemorySummary
	Updated bool
}

type DeleteMemoryRequest struct {
	ThreadID int64
	MemoryID int64
	ActorID  int64
	ViewerID int64
}

type DeleteMemoryResponse struct {
	Deleted bool
}

type ClearMemoriesRequest struct {
	ThreadID int64
	RunID    int64
	Scopes   []MemoryScope
	ActorID  int64
	ViewerID int64
}

type ClearMemoriesResponse struct {
	Deleted int64
}

type RestoreMemoryRequest struct {
	ThreadID int64
	MemoryID int64
	ActorID  int64
	ViewerID int64
}

type RestoreMemoryResponse struct {
	Memory   *MemorySummary
	Restored bool
}

type ListMemoryAuditEventsRequest struct {
	ThreadID int64
	MemoryID int64
	ViewerID int64
	Page     int32
	PageSize int32
}

type ListMemoryAuditEventsResponse struct {
	Events []*MemoryAuditEventSummary
	Total  int64
}

type ListGuardrailAuditEventsRequest struct {
	ThreadID int64
	RunID    int64
	ViewerID int64
	Page     int32
	PageSize int32
}

type ListGuardrailAuditEventsResponse struct {
	Events []*GuardrailAuditEventSummary
	Total  int64
}

type ListMCPRuntimeAuditEventsRequest struct {
	ThreadID int64
	RunID    int64
	ViewerID int64
	Page     int32
	PageSize int32
}

type ListMCPRuntimeAuditEventsResponse struct {
	Events []*MCPRuntimeAuditEventSummary
	Total  int64
}

type PersistTranscriptSnapshotRequest struct {
	ThreadID       int64
	RunID          int64
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
	Metadata       string
}

type PersistTranscriptSnapshotResponse struct {
	Snapshot *TranscriptSnapshotSummary
	Created  bool
}

type EnqueueMemoryFlushJobRequest struct {
	ThreadID             int64
	RunID                int64
	TranscriptSnapshotID int64
	IdempotencyKey       string
	AvailableAt          int64
}

type EnqueueMemoryFlushJobResponse struct {
	Job     *MemoryFlushJobSummary
	Created bool
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
	ThreadID         int64
	RunID            int64
	IncludeChildRuns bool
	Source           TokenUsageSource
	Page             int32
	PageSize         int32
}

type GetTokenUsageResponse struct {
	Usage         []*TokenUsageSummary
	Total         int64
	Aggregate     *TokenUsageAggregateSummary
	RunAggregates []*RunTokenUsageAggregateSummary
}

type ListArtifactsRequest struct {
	ThreadID    int64
	RunID       *int64
	DeletedOnly bool
	SpaceID     int64
	ViewerID    int64
	Page        int32
	PageSize    int32
}

type ListArtifactsResponse struct {
	Artifacts []*ArtifactSummary
	Total     int64
}

type OutputFileSummary struct {
	FileID      int64
	FileName    string
	VirtualPath string
	ContentType string
	SizeBytes   int64
	Digest      string
}

type WriteOutputFileRequest struct {
	Run         *RunSummary
	FilePath    string
	Content     string
	ContentType string
}

type WriteOutputFileResponse struct {
	File    *OutputFileSummary
	Created bool
	Notice  string
}

type SkillPackageResource struct {
	Path    string
	Content string
}

type CreateSkillPackageRequest struct {
	Run        *RunSummary
	SkillName  string
	SkillMD    string
	OutputPath string
	Resources  []SkillPackageResource
}

type CreateSkillPackageResponse struct {
	File    *OutputFileSummary
	Created bool
	Notice  string
}

type PresentOutputFilesRequest struct {
	Run       *RunSummary
	FilePaths []string
}

type PresentOutputFilesResponse struct {
	Artifacts []*ArtifactSummary
	Notice    string
}

type ListArtifactScanJobsRequest struct {
	ThreadID   int64
	RunID      *int64
	ArtifactID *int64
	Status     string
	Scanner    string
	SpaceID    int64
	ViewerID   int64
	Page       int32
	PageSize   int32
}

type ListArtifactScanJobsResponse struct {
	Jobs  []*ArtifactScanJobSummary
	Total int64
}

type RetryArtifactScanJobRequest struct {
	ThreadID int64
	JobID    int64
	SpaceID  int64
	ViewerID int64
}

type RetryArtifactScanJobResponse struct {
	Job     *ArtifactScanJobSummary
	Retried bool
}

type ArtifactContentMode string

const (
	ArtifactContentModePreview  ArtifactContentMode = "preview"
	ArtifactContentModeDownload ArtifactContentMode = "download"
)

type ReadArtifactContentRequest struct {
	ThreadID   int64
	ArtifactID int64
	Mode       ArtifactContentMode
	SpaceID    int64
	ViewerID   int64
}

type CreateArtifactSignedURLRequest struct {
	ThreadID   int64
	ArtifactID int64
	Mode       ArtifactContentMode
	SpaceID    int64
	ViewerID   int64
	TTLSeconds int64
}

type CreateArtifactSignedURLResponse struct {
	Artifact         *ArtifactSummary
	URL              string
	ExpiresInSeconds int64
	ContentType      string
	PreviewMode      ArtifactPreviewMode
}

type DeleteArtifactRequest struct {
	ThreadID   int64
	ArtifactID int64
	SpaceID    int64
	ViewerID   int64
	DeletedAt  int64
}

type DeleteArtifactResponse struct {
	Deleted bool
}

type RestoreArtifactRequest struct {
	ThreadID   int64
	ArtifactID int64
	SpaceID    int64
	ViewerID   int64
	RestoredAt int64
}

type RestoreArtifactResponse struct {
	Artifact *ArtifactSummary
	Restored bool
}

type ProcessDeletedArtifactCleanupRequest struct {
	RetentionMillis int64
	NowMillis       int64
	Limit           int32
}

type ProcessDeletedArtifactCleanupResponse struct {
	Candidates int32
	Deleted    int32
	NotFound   int32
	Failed     int32
	Skipped    int32
}

type RecordArtifactScanResultRequest struct {
	ThreadID       int64
	ArtifactID     int64
	ScanStatus     string
	Scanner        string
	ScannerVersion string
	Reason         string
	ScannedAt      int64
}

type RecordArtifactScanResultResponse struct {
	Artifact *ArtifactSummary
	Updated  bool
}

type ReviewArtifactScanRequest struct {
	ThreadID   int64
	ArtifactID int64
	SpaceID    int64
	ViewerID   int64
	Decision   string
	Reason     string
}

type ReviewArtifactScanResponse struct {
	ArtifactID int64
	Decision   string
	ScanStatus string
	Reviewed   bool
}

type ProcessArtifactScanJobsRequest struct {
	Scanner            string
	WorkerID           string
	Limit              int32
	LeaseTTLMillis     int64
	MaxAttempts        int32
	RetryBackoffMillis int64
}

type ProcessArtifactScanJobsResponse struct {
	Claimed    int32
	Succeeded  int32
	Retried    int32
	Failed     int32
	Skipped    int32
	JobMetrics []*ArtifactScanJobMetricsSummary
}

type ArtifactScanJobMetricsSummary struct {
	Scanner           string
	ContentFamily     string
	Result            string
	ErrorCode         string
	CreatedAt         int64
	StartedAt         int64
	EndedAt           int64
	ObserveLatency    bool
	ObserveQueueDelay bool
}

type ProcessMemoryFlushJobsRequest struct {
	WorkerID           string
	Limit              int32
	LeaseTTLMillis     int64
	MaxAttempts        int32
	RetryBackoffMillis int64
}

type ProcessMemoryFlushJobsResponse struct {
	Claimed     int32
	Succeeded   int32
	Retried     int32
	Failed      int32
	Skipped     int32
	JobMetrics  []*MemoryFlushJobMetricsSummary
	FactMetrics []*MemoryFactMetricsSummary
}

type MemoryFlushJobMetricsSummary struct {
	Result         string
	ErrorCode      string
	StartedAt      int64
	EndedAt        int64
	ObserveLatency bool
}

type MemoryFactMetricsSummary struct {
	Operation string
	Result    string
	Count     int64
}

type ReadArtifactContentResponse struct {
	Artifact    *ArtifactSummary
	Content     []byte
	ContentType string
	FileName    string
	Attachment  bool
}

type ClaimPendingRunsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseTTLMillis int64
}

type ClaimPendingRunsResponse struct {
	Runs []*RunSummary
}

type ClaimQueuedResumeRunsRequest struct {
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseTTLMillis int64
}

type ClaimQueuedResumeRunsResponse struct {
	Runs []*RunSummary
}

type RenewRunLeaseRequest struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	Now                 int64
	LeaseTTLMillis      int64
}

type RenewRunLeaseResponse struct {
	Run *RunSummary
}

type ReleaseRunLeaseRequest struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	ToStatus            RunStatus
	Now                 int64
}

type ReleaseRunLeaseResponse struct {
	Run *RunSummary
}

type ListExpiredRunLeasesRequest struct {
	Now   int64
	Limit int32
}

type ListExpiredRunLeasesResponse struct {
	Runs []*RunSummary
}

type ReconcileExpiredRunLeaseRequest struct {
	RunID               int64
	LeaseOwner          string
	LeaseToken          string
	ExecutionGeneration uint64
	ToStatus            RunStatus
	Now                 int64
	ErrorCode           string
	ErrorMessage        string
	EventPayload        string
}

type ReconcileExpiredRunLeaseResponse struct {
	Run *RunSummary
}

type FinalizeRunSuccessRequest struct {
	RunID                             int64
	ThreadID                          int64
	LeaseOwner                        string
	LeaseToken                        string
	ExecutionGeneration               uint64
	Now                               int64
	Message                           string
	MessageMetadata                   string
	TitleEventPayload                 string
	CompletionEventPayload            string
	ExpectedThreadTitle               string
	ThreadTitle                       string
	TerminalCheckpoint                *CreateCheckpointRequest
	TerminalCheckpointOnTitleConflict *CreateCheckpointRequest
}

type FinalizeRunSuccessResponse struct {
	Run          *RunSummary
	Message      *MessageSummary
	Checkpoint   *CheckpointSummary
	TitleUpdated bool
}

type UpdateRunStatusRequest struct {
	RunID                 int64
	From                  RunStatus
	To                    RunStatus
	WorkerID              string
	LeaseOwner            string
	LeaseToken            string
	ExecutionGeneration   uint64
	Now                   int64
	ErrorCode             string
	ErrorMessage          string
	EventPayload          string
	EventAlreadyPersisted bool
}

type UpdateRunStatusResponse struct {
	Run *RunSummary
}

type CancelRunOnDisconnectRequest struct {
	RunID int64
}

type CancelRunOnDisconnectResponse struct {
	Run      *RunSummary
	Canceled bool
}
