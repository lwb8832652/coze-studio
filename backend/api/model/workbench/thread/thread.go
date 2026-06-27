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

package thread

type TaskThread struct {
	ThreadID         int64  `json:"thread_id,string"`
	LegacyTaskID     int64  `json:"legacy_task_id,string"`
	SpaceID          int64  `json:"space_id,string"`
	CreatorID        int64  `json:"creator_id,string"`
	Title            string `json:"title"`
	Status           string `json:"status"`
	Source           string `json:"source"`
	Progress         int32  `json:"progress"`
	LastUserMessage  string `json:"last_user_message"`
	LastAgentMessage string `json:"last_agent_message"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

type TaskThreadMessage struct {
	MessageID int64  `json:"message_id,string"`
	ThreadID  int64  `json:"thread_id,string"`
	RunID     int64  `json:"run_id,string"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Metadata  string `json:"metadata"`
	CreatedAt int64  `json:"created_at"`
}

type TaskThreadRun struct {
	RunID             int64  `json:"run_id,string"`
	ThreadID          int64  `json:"thread_id,string"`
	ParentRunID       int64  `json:"parent_run_id,string"`
	SpaceID           int64  `json:"space_id,string"`
	CreatorID         int64  `json:"creator_id,string"`
	AssistantID       string `json:"assistant_id"`
	RunKind           string `json:"run_kind"`
	Status            string `json:"status"`
	Command           string `json:"command"`
	Input             string `json:"input"`
	Config            string `json:"config"`
	Context           string `json:"context"`
	Metadata          string `json:"metadata"`
	StreamMode        string `json:"stream_mode"`
	MultitaskStrategy string `json:"multitask_strategy"`
	OnDisconnect      string `json:"on_disconnect"`
	Durability        string `json:"durability"`
	IdempotencyKey    string `json:"idempotency_key"`
	WorkerID          string `json:"worker_id"`
	ErrorCode         string `json:"error_code"`
	ErrorMessage      string `json:"error_message"`
	StartedAt         int64  `json:"started_at"`
	EndedAt           int64  `json:"ended_at"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

type TaskThreadRunEvent struct {
	EventID   int64  `json:"event_id,string"`
	ThreadID  int64  `json:"thread_id,string"`
	RunID     int64  `json:"run_id,string"`
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	CreatedAt int64  `json:"created_at"`
}

type TaskThreadTokenUsage struct {
	UsageID      int64  `json:"usage_id,string"`
	ThreadID     int64  `json:"thread_id,string"`
	RunID        int64  `json:"run_id,string"`
	SpaceID      int64  `json:"space_id,string"`
	Source       string `json:"source"`
	StepID       string `json:"step_id"`
	StepIndex    int32  `json:"step_index"`
	StepName     string `json:"step_name"`
	ModelName    string `json:"model_name"`
	Provider     string `json:"provider"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	TotalTokens  int64  `json:"total_tokens"`
	CostMicros   int64  `json:"cost_micros"`
	Currency     string `json:"currency"`
	Estimated    bool   `json:"estimated"`
	RawUsage     string `json:"raw_usage"`
	Metadata     string `json:"metadata"`
	CreatedAt    int64  `json:"created_at"`
}

type TaskThreadTokenUsageAggregate struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	CostMicros       int64 `json:"cost_micros"`
	CallCount        int64 `json:"call_count"`
	LeadAgentTokens  int64 `json:"lead_agent_tokens"`
	SubagentTokens   int64 `json:"subagent_tokens"`
	MiddlewareTokens int64 `json:"middleware_tokens"`
	ToolTokens       int64 `json:"tool_tokens"`
}

type TaskThreadTokenUsageRunAggregate struct {
	RunID     int64                          `json:"run_id,string"`
	Aggregate *TaskThreadTokenUsageAggregate `json:"aggregate"`
}

type TaskThreadMemory struct {
	MemoryID             int64   `json:"memory_id,string"`
	ThreadID             int64   `json:"thread_id,string"`
	RunID                int64   `json:"run_id,string"`
	SpaceID              int64   `json:"space_id,string"`
	Scope                string  `json:"scope"`
	Content              string  `json:"content"`
	Metadata             string  `json:"metadata"`
	Score                float64 `json:"score"`
	Confidence           float64 `json:"confidence"`
	SourceType           string  `json:"source_type"`
	SourceID             string  `json:"source_id"`
	CorrectionOfMemoryID int64   `json:"correction_of_memory_id,string"`
	CorrectedAt          int64   `json:"corrected_at"`
	ExpiresAt            int64   `json:"expires_at"`
	CreatedAt            int64   `json:"created_at"`
	UpdatedAt            int64   `json:"updated_at"`
	DeletedAt            int64   `json:"deleted_at"`
}

type TaskThreadMemoryAuditEvent struct {
	EventID       int64  `json:"event_id,string"`
	ThreadID      int64  `json:"thread_id,string"`
	RunID         int64  `json:"run_id,string"`
	SpaceID       int64  `json:"space_id,string"`
	MemoryID      int64  `json:"memory_id,string"`
	ActorID       int64  `json:"actor_id,string"`
	EventType     string `json:"event_type"`
	Scope         string `json:"scope"`
	SourceType    string `json:"source_type"`
	SourceID      string `json:"source_id"`
	AffectedCount int64  `json:"affected_count"`
	CreatedAt     int64  `json:"created_at"`
}

type TaskThreadGuardrailAuditEvent struct {
	EventID    int64  `json:"event_id,string"`
	ThreadID   int64  `json:"thread_id,string"`
	RunID      int64  `json:"run_id,string"`
	SpaceID    int64  `json:"space_id,string"`
	ActorID    int64  `json:"actor_id,string"`
	EventType  string `json:"event_type"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Operation  string `json:"operation"`
	Source     string `json:"source"`
	Action     string `json:"action"`
	FailMode   string `json:"fail_mode"`
	Provider   string `json:"provider"`
	ReasonCode string `json:"reason_code"`
	RuleIDs    string `json:"rule_ids"`
	CreatedAt  int64  `json:"created_at"`
}

type TaskThreadArtifact struct {
	ArtifactID   int64  `json:"artifact_id,string"`
	ThreadID     int64  `json:"thread_id,string"`
	RunID        int64  `json:"run_id,string"`
	FileID       int64  `json:"file_id,string"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type"`
	VirtualPath  string `json:"virtual_path"`
	ContentType  string `json:"content_type"`
	SizeBytes    int64  `json:"size_bytes"`
	PreviewMode  string `json:"preview_mode"`
	Metadata     string `json:"metadata"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	DeletedAt    int64  `json:"deleted_at"`
}

type TaskThreadArtifactScanJob struct {
	JobID          int64  `json:"job_id,string"`
	ThreadID       int64  `json:"thread_id,string"`
	RunID          int64  `json:"run_id,string"`
	SpaceID        int64  `json:"space_id,string"`
	UserID         int64  `json:"user_id,string"`
	ArtifactID     int64  `json:"artifact_id,string"`
	FileID         int64  `json:"file_id,string"`
	Scanner        string `json:"scanner"`
	Status         string `json:"status"`
	WorkerID       string `json:"worker_id"`
	AttemptCount   int32  `json:"attempt_count"`
	LastError      string `json:"last_error"`
	AvailableAt    int64  `json:"available_at"`
	LeaseExpiresAt int64  `json:"lease_expires_at"`
	StartedAt      int64  `json:"started_at"`
	EndedAt        int64  `json:"ended_at"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type ListTaskThreadsRequest struct {
	SpaceID  int64  `query:"space_id,required"`
	Status   string `query:"status"`
	Page     int32  `query:"page"`
	PageSize int32  `query:"page_size"`
}

type GetTaskThreadRequest struct {
	ThreadID int64 `path:"thread_id,required"`
}

type ListTaskThreadMessagesRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	Page     int32 `query:"page"`
	PageSize int32 `query:"page_size"`
}

type AppendTaskThreadMessageRequest struct {
	ThreadID int64  `path:"thread_id,required" json:"-"`
	RunID    int64  `json:"run_id,string,omitempty"`
	Role     string `json:"role,required"`
	Content  string `json:"content,required"`
	Metadata string `json:"metadata,omitempty"`
}

type ListTaskThreadRunsRequest struct {
	ThreadID    int64  `path:"thread_id,required"`
	ParentRunID int64  `query:"parent_run_id"`
	Status      string `query:"status"`
	Page        int32  `query:"page"`
	PageSize    int32  `query:"page_size"`
}

type ListTaskThreadRunEventsRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	RunID    int64 `query:"run_id"`
	Page     int32 `query:"page"`
	PageSize int32 `query:"page_size"`
}

type StreamTaskThreadRunEventsRequest struct {
	ThreadID     int64 `path:"thread_id,required"`
	RunID        int64 `query:"run_id"`
	AfterEventID int64 `query:"after_event_id"`
	IntervalMs   int64 `query:"interval_ms"`
	TimeoutMs    int64 `query:"timeout_ms"`
}

type GetTaskThreadTokenUsageRequest struct {
	ThreadID         int64  `path:"thread_id,required"`
	RunID            int64  `query:"run_id"`
	IncludeChildRuns bool   `query:"include_child_runs"`
	Source           string `query:"source"`
	Page             int32  `query:"page"`
	PageSize         int32  `query:"page_size"`
}

type ListTaskThreadMemoriesRequest struct {
	ThreadID       int64    `path:"thread_id,required"`
	RunID          int64    `query:"run_id"`
	Scope          string   `query:"scope"`
	Scopes         []string `query:"scopes"`
	Query          string   `query:"q"`
	IncludeExpired bool     `query:"include_expired"`
	IncludeDeleted bool     `query:"include_deleted"`
	Page           int32    `query:"page"`
	PageSize       int32    `query:"page_size"`
}

type ExportTaskThreadMemoriesRequest struct {
	ThreadID       int64    `path:"thread_id,required"`
	RunID          int64    `query:"run_id"`
	Scope          string   `query:"scope"`
	Scopes         []string `query:"scopes"`
	Query          string   `query:"q"`
	IncludeExpired bool     `query:"include_expired"`
	IncludeDeleted bool     `query:"include_deleted"`
	Limit          int32    `query:"limit"`
}

type ListTaskThreadMemoryAuditEventsRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	MemoryID int64 `query:"memory_id"`
	Page     int32 `query:"page"`
	PageSize int32 `query:"page_size"`
}

type ListTaskThreadGuardrailAuditEventsRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	RunID    int64 `query:"run_id"`
	Page     int32 `query:"page"`
	PageSize int32 `query:"page_size"`
}

type ExportTaskThreadGuardrailAuditEventsRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	RunID    int64 `query:"run_id"`
	Page     int32 `query:"page"`
	PageSize int32 `query:"page_size"`
}

type ListTaskThreadArtifactsRequest struct {
	ThreadID    int64 `path:"thread_id,required"`
	RunID       int64 `query:"run_id"`
	DeletedOnly bool  `query:"deleted_only"`
	Page        int32 `query:"page"`
	PageSize    int32 `query:"page_size"`
}

type ListTaskThreadArtifactScanJobsRequest struct {
	ThreadID   int64  `path:"thread_id,required"`
	RunID      int64  `query:"run_id"`
	ArtifactID int64  `query:"artifact_id"`
	Status     string `query:"status"`
	Scanner    string `query:"scanner"`
	Page       int32  `query:"page"`
	PageSize   int32  `query:"page_size"`
}

type RetryTaskThreadArtifactScanJobRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	JobID    int64 `path:"job_id,required"`
}

type ReviewTaskThreadArtifactScanRequest struct {
	ThreadID   int64  `path:"thread_id,required"`
	ArtifactID int64  `path:"artifact_id,required"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
}

type GetTaskThreadArtifactContentRequest struct {
	ThreadID   int64  `path:"thread_id,required"`
	ArtifactID int64  `path:"artifact_id,required"`
	Mode       string `query:"mode"`
}

type GetTaskThreadArtifactSignedURLRequest struct {
	ThreadID   int64  `path:"thread_id,required"`
	ArtifactID int64  `path:"artifact_id,required"`
	Mode       string `query:"mode"`
	TTLSeconds int64  `query:"ttl_seconds"`
}

type DeleteTaskThreadArtifactRequest struct {
	ThreadID   int64 `path:"thread_id,required"`
	ArtifactID int64 `path:"artifact_id,required"`
}

type RestoreTaskThreadArtifactRequest struct {
	ThreadID   int64 `path:"thread_id,required"`
	ArtifactID int64 `path:"artifact_id,required"`
}

type UpdateTaskThreadMemoryRequest struct {
	ThreadID             int64   `path:"thread_id,required" json:"-"`
	MemoryID             int64   `path:"memory_id,required" json:"-"`
	RunID                int64   `json:"run_id,string,omitempty"`
	Scope                string  `json:"scope,required"`
	Content              string  `json:"content,required"`
	Metadata             string  `json:"metadata,omitempty"`
	Score                float64 `json:"score,omitempty"`
	Confidence           float64 `json:"confidence,omitempty"`
	SourceType           string  `json:"source_type,omitempty"`
	SourceID             string  `json:"source_id,omitempty"`
	CorrectionOfMemoryID int64   `json:"correction_of_memory_id,string,omitempty"`
	CorrectedAt          int64   `json:"corrected_at,omitempty"`
	ExpiresAt            int64   `json:"expires_at,omitempty"`
}

type ImportTaskThreadMemoryItem struct {
	RunID                int64   `json:"run_id,string,omitempty"`
	Scope                string  `json:"scope,omitempty"`
	Content              string  `json:"content,required"`
	Metadata             string  `json:"metadata,omitempty"`
	Score                float64 `json:"score,omitempty"`
	Confidence           float64 `json:"confidence,omitempty"`
	SourceType           string  `json:"source_type,omitempty"`
	SourceID             string  `json:"source_id,omitempty"`
	CorrectionOfMemoryID int64   `json:"correction_of_memory_id,string,omitempty"`
	CorrectedAt          int64   `json:"corrected_at,omitempty"`
	ExpiresAt            int64   `json:"expires_at,omitempty"`
}

type ImportTaskThreadMemoriesRequest struct {
	ThreadID int64                         `path:"thread_id,required" json:"-"`
	Memories []*ImportTaskThreadMemoryItem `json:"memories,required"`
}

type DeleteTaskThreadMemoryRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	MemoryID int64 `path:"memory_id,required"`
}

type RestoreTaskThreadMemoryRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	MemoryID int64 `path:"memory_id,required"`
}

type ClearTaskThreadMemoriesRequest struct {
	ThreadID int64    `path:"thread_id,required" json:"-"`
	RunID    int64    `json:"run_id,string,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
}

type CreateTaskThreadRunRequest struct {
	ThreadID          int64  `path:"thread_id,required" json:"-"`
	AssistantID       string `json:"assistant_id,omitempty"`
	Command           string `json:"command,omitempty"`
	Input             string `json:"input,required"`
	Config            string `json:"config,omitempty"`
	Context           string `json:"context,omitempty"`
	Metadata          string `json:"metadata,omitempty"`
	StreamMode        string `json:"stream_mode,omitempty"`
	MultitaskStrategy string `json:"multitask_strategy,omitempty"`
	OnDisconnect      string `json:"on_disconnect,omitempty"`
	Durability        string `json:"durability,omitempty"`
	IdempotencyKey    string `json:"idempotency_key,omitempty"`
}

type HumanInteractionResponse struct {
	Schema        string `json:"schema"`
	InteractionID string `json:"interaction_id"`
	Kind          string `json:"kind"`
	Decision      string `json:"decision"`
	Answer        string `json:"answer,omitempty"`
	ChoiceID      string `json:"choice_id,omitempty"`
	Comment       string `json:"comment,omitempty"`
	SubmittedBy   string `json:"submitted_by,omitempty"`
	SubmittedAt   int64  `json:"submitted_at,omitempty"`
	Source        string `json:"source,omitempty"`
}

type ResumeTaskThreadRunRequest struct {
	ThreadID       int64                    `path:"thread_id,required" json:"-"`
	RunID          int64                    `path:"run_id,required" json:"-"`
	InterruptID    string                   `json:"interrupt_id"`
	Response       HumanInteractionResponse `json:"response"`
	IdempotencyKey string                   `json:"idempotency_key,omitempty"`
}

type RetryTaskThreadSubagentRunRequest struct {
	ThreadID       int64  `path:"thread_id,required" json:"-"`
	RunID          int64  `path:"run_id,required" json:"-"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type ListTaskThreadsData struct {
	Threads []*TaskThread `json:"threads"`
	Total   int64         `json:"total"`
}

type ListTaskThreadMessagesData struct {
	Messages []*TaskThreadMessage `json:"messages"`
	Total    int64                `json:"total"`
}

type ListTaskThreadRunsData struct {
	Runs  []*TaskThreadRun `json:"runs"`
	Total int64            `json:"total"`
}

type ListTaskThreadRunEventsData struct {
	Events []*TaskThreadRunEvent `json:"events"`
	Total  int64                 `json:"total"`
}

type GetTaskThreadTokenUsageData struct {
	Usage         []*TaskThreadTokenUsage             `json:"usage"`
	Total         int64                               `json:"total"`
	Aggregate     *TaskThreadTokenUsageAggregate      `json:"aggregate"`
	RunAggregates []*TaskThreadTokenUsageRunAggregate `json:"run_aggregates,omitempty"`
}

type ListTaskThreadMemoriesData struct {
	Memories []*TaskThreadMemory `json:"memories"`
	Total    int64               `json:"total"`
}

type ExportTaskThreadMemoriesData struct {
	Schema     string              `json:"schema"`
	ThreadID   int64               `json:"thread_id,string"`
	ExportedAt int64               `json:"exported_at"`
	Total      int64               `json:"total"`
	Memories   []*TaskThreadMemory `json:"memories"`
}

type UpdateTaskThreadMemoryData struct {
	Memory  *TaskThreadMemory `json:"memory,omitempty"`
	Updated bool              `json:"updated"`
}

type ImportTaskThreadMemoriesData struct {
	Imported int64               `json:"imported"`
	Skipped  int64               `json:"skipped"`
	Memories []*TaskThreadMemory `json:"memories"`
}

type RestoreTaskThreadMemoryData struct {
	Memory   *TaskThreadMemory `json:"memory,omitempty"`
	Restored bool              `json:"restored"`
}

type ListTaskThreadMemoryAuditEventsData struct {
	Events []*TaskThreadMemoryAuditEvent `json:"events"`
	Total  int64                         `json:"total"`
}

type ListTaskThreadGuardrailAuditEventsData struct {
	Events []*TaskThreadGuardrailAuditEvent `json:"events"`
	Total  int64                            `json:"total"`
}

type ExportTaskThreadGuardrailAuditEventsData struct {
	Schema     string                           `json:"schema"`
	ThreadID   int64                            `json:"thread_id,string"`
	ExportedAt int64                            `json:"exported_at"`
	Page       int32                            `json:"page"`
	PageSize   int32                            `json:"page_size"`
	Total      int64                            `json:"total"`
	Events     []*TaskThreadGuardrailAuditEvent `json:"events"`
}

type ClearTaskThreadMemoriesData struct {
	Deleted int64 `json:"deleted"`
}

type ListTaskThreadArtifactsData struct {
	Artifacts []*TaskThreadArtifact `json:"artifacts"`
	Total     int64                 `json:"total"`
}

type ListTaskThreadArtifactScanJobsData struct {
	Jobs  []*TaskThreadArtifactScanJob `json:"jobs"`
	Total int64                        `json:"total"`
}

type RetryTaskThreadArtifactScanJobData struct {
	Job     *TaskThreadArtifactScanJob `json:"job,omitempty"`
	Retried bool                       `json:"retried"`
}

type ReviewTaskThreadArtifactScanData struct {
	ArtifactID int64  `json:"artifact_id,string"`
	Decision   string `json:"decision"`
	ScanStatus string `json:"scan_status"`
	Reviewed   bool   `json:"reviewed"`
}

type RestoreTaskThreadArtifactData struct {
	ArtifactID int64 `json:"artifact_id,string"`
	Restored   bool  `json:"restored"`
}

type GetTaskThreadArtifactSignedURLData struct {
	ArtifactID       int64  `json:"artifact_id,string"`
	URL              string `json:"url"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
	ContentType      string `json:"content_type"`
	PreviewMode      string `json:"preview_mode"`
}

type ListTaskThreadsResponse struct {
	Data *ListTaskThreadsData `json:"data,omitempty"`
	Code int64                `json:"code"`
	Msg  string               `json:"msg"`
}

type GetTaskThreadResponse struct {
	Data *TaskThread `json:"data,omitempty"`
	Code int64       `json:"code"`
	Msg  string      `json:"msg"`
}

type ListTaskThreadMessagesResponse struct {
	Data *ListTaskThreadMessagesData `json:"data,omitempty"`
	Code int64                       `json:"code"`
	Msg  string                      `json:"msg"`
}

type AppendTaskThreadMessageResponse struct {
	Data *TaskThreadMessage `json:"data,omitempty"`
	Code int64              `json:"code"`
	Msg  string             `json:"msg"`
}

type ListTaskThreadRunsResponse struct {
	Data *ListTaskThreadRunsData `json:"data,omitempty"`
	Code int64                   `json:"code"`
	Msg  string                  `json:"msg"`
}

type ListTaskThreadRunEventsResponse struct {
	Data *ListTaskThreadRunEventsData `json:"data,omitempty"`
	Code int64                        `json:"code"`
	Msg  string                       `json:"msg"`
}

type GetTaskThreadTokenUsageResponse struct {
	Data *GetTaskThreadTokenUsageData `json:"data,omitempty"`
	Code int64                        `json:"code"`
	Msg  string                       `json:"msg"`
}

type ListTaskThreadMemoriesResponse struct {
	Data *ListTaskThreadMemoriesData `json:"data,omitempty"`
	Code int64                       `json:"code"`
	Msg  string                      `json:"msg"`
}

type ExportTaskThreadMemoriesResponse struct {
	Data *ExportTaskThreadMemoriesData `json:"data,omitempty"`
	Code int64                         `json:"code"`
	Msg  string                        `json:"msg"`
}

type UpdateTaskThreadMemoryResponse struct {
	Data *UpdateTaskThreadMemoryData `json:"data,omitempty"`
	Code int64                       `json:"code"`
	Msg  string                      `json:"msg"`
}

type ImportTaskThreadMemoriesResponse struct {
	Data *ImportTaskThreadMemoriesData `json:"data,omitempty"`
	Code int64                         `json:"code"`
	Msg  string                        `json:"msg"`
}

type RestoreTaskThreadMemoryResponse struct {
	Data *RestoreTaskThreadMemoryData `json:"data,omitempty"`
	Code int64                        `json:"code"`
	Msg  string                       `json:"msg"`
}

type ListTaskThreadMemoryAuditEventsResponse struct {
	Data *ListTaskThreadMemoryAuditEventsData `json:"data,omitempty"`
	Code int64                                `json:"code"`
	Msg  string                               `json:"msg"`
}

type ListTaskThreadGuardrailAuditEventsResponse struct {
	Data *ListTaskThreadGuardrailAuditEventsData `json:"data,omitempty"`
	Code int64                                   `json:"code"`
	Msg  string                                  `json:"msg"`
}

type ExportTaskThreadGuardrailAuditEventsResponse struct {
	Data *ExportTaskThreadGuardrailAuditEventsData `json:"data,omitempty"`
	Code int64                                     `json:"code"`
	Msg  string                                    `json:"msg"`
}

type DeleteTaskThreadMemoryResponse struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

type ClearTaskThreadMemoriesResponse struct {
	Data *ClearTaskThreadMemoriesData `json:"data,omitempty"`
	Code int64                        `json:"code"`
	Msg  string                       `json:"msg"`
}

type ListTaskThreadArtifactsResponse struct {
	Data *ListTaskThreadArtifactsData `json:"data,omitempty"`
	Code int64                        `json:"code"`
	Msg  string                       `json:"msg"`
}

type ListTaskThreadArtifactScanJobsResponse struct {
	Data *ListTaskThreadArtifactScanJobsData `json:"data,omitempty"`
	Code int64                               `json:"code"`
	Msg  string                              `json:"msg"`
}

type RetryTaskThreadArtifactScanJobResponse struct {
	Data *RetryTaskThreadArtifactScanJobData `json:"data,omitempty"`
	Code int64                               `json:"code"`
	Msg  string                              `json:"msg"`
}

type ReviewTaskThreadArtifactScanResponse struct {
	Data *ReviewTaskThreadArtifactScanData `json:"data,omitempty"`
	Code int64                             `json:"code"`
	Msg  string                            `json:"msg"`
}

type DeleteTaskThreadArtifactResponse struct {
	Code int64  `json:"code"`
	Msg  string `json:"msg"`
}

type RestoreTaskThreadArtifactResponse struct {
	Data *RestoreTaskThreadArtifactData `json:"data,omitempty"`
	Code int64                          `json:"code"`
	Msg  string                         `json:"msg"`
}

type GetTaskThreadArtifactSignedURLResponse struct {
	Data *GetTaskThreadArtifactSignedURLData `json:"data,omitempty"`
	Code int64                               `json:"code"`
	Msg  string                              `json:"msg"`
}

type CreateTaskThreadRunResponse struct {
	Data *TaskThreadRun `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}

type ResumeTaskThreadRunResponse struct {
	Data *TaskThreadRun `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}

type RetryTaskThreadSubagentRunResponse struct {
	Data *TaskThreadRun `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}
