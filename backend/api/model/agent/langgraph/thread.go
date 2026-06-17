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

package langgraph

type Thread struct {
	ThreadID  string         `json:"thread_id"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	Metadata  map[string]any `json:"metadata"`
	Status    string         `json:"status"`
	Values    *ThreadValues  `json:"values"`
}

type ThreadValues struct {
	Messages []map[string]any `json:"messages"`
}

type CreateThreadRequest struct {
	ThreadID string         `json:"thread_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	IfExists string         `json:"if_exists,omitempty"`
}

type GetThreadRequest struct {
	ThreadID int64 `path:"thread_id,required"`
}

type SearchThreadsRequest struct {
	Metadata map[string]any `json:"metadata,omitempty"`
	Status   string         `json:"status,omitempty"`
	Limit    int32          `json:"limit,omitempty"`
	Offset   int32          `json:"offset,omitempty"`
}

type GetThreadStateRequest struct {
	ThreadID int64 `path:"thread_id,required"`
}

type GetThreadHistoryRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	Limit    int32 `query:"limit,omitempty"`
	Offset   int32 `query:"offset,omitempty"`
}

type GetCheckpointResumeRequest struct {
	ThreadID     int64 `path:"thread_id,required"`
	CheckpointID int64 `path:"checkpoint_id,required"`
}

type ThreadState struct {
	Values    map[string]any `json:"values"`
	Next      []string       `json:"next"`
	Config    map[string]any `json:"config"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

type CheckpointResumeReadiness struct {
	ThreadID     string         `json:"thread_id"`
	RunID        string         `json:"run_id"`
	CheckpointID string         `json:"checkpoint_id"`
	CheckpointNS string         `json:"checkpoint_ns"`
	Resumable    bool           `json:"resumable"`
	ResumeFrom   string         `json:"resume_from,omitempty"`
	Reason       string         `json:"reason"`
	Status       string         `json:"status,omitempty"`
	ErrorType    string         `json:"error_type,omitempty"`
	PendingSends []string       `json:"pending_sends"`
	Config       map[string]any `json:"config"`
	Metadata     map[string]any `json:"metadata"`
}
