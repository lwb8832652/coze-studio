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

type CreateTaskThreadRunResponse struct {
	Data *TaskThreadRun `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}
