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
	SpaceID           int64  `json:"space_id,string"`
	CreatorID         int64  `json:"creator_id,string"`
	AssistantID       string `json:"assistant_id"`
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
	ThreadID int64  `path:"thread_id,required"`
	Status   string `query:"status"`
	Page     int32  `query:"page"`
	PageSize int32  `query:"page_size"`
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
	ThreadID int64  `path:"thread_id,required"`
	RunID    int64  `query:"run_id"`
	Source   string `query:"source"`
	Page     int32  `query:"page"`
	PageSize int32  `query:"page_size"`
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
	Usage     []*TaskThreadTokenUsage        `json:"usage"`
	Total     int64                          `json:"total"`
	Aggregate *TaskThreadTokenUsageAggregate `json:"aggregate"`
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

type CreateTaskThreadRunResponse struct {
	Data *TaskThreadRun `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}
