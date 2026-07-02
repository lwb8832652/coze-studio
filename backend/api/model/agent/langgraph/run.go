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

type Run struct {
	RunID             string         `json:"run_id"`
	ThreadID          string         `json:"thread_id"`
	AssistantID       string         `json:"assistant_id"`
	Status            string         `json:"status"`
	CreatedAt         string         `json:"created_at"`
	UpdatedAt         string         `json:"updated_at"`
	Metadata          map[string]any `json:"metadata"`
	Input             any            `json:"input"`
	Command           map[string]any `json:"command"`
	Config            map[string]any `json:"config"`
	Context           map[string]any `json:"context"`
	StreamMode        []string       `json:"stream_mode"`
	MultitaskStrategy string         `json:"multitask_strategy"`
	OnDisconnect      string         `json:"on_disconnect"`
	Durability        string         `json:"durability"`
	Error             string         `json:"error,omitempty"`
}

type CreateRunRequest struct {
	ThreadID          int64          `path:"thread_id,required" json:"-"`
	AssistantID       string         `json:"assistant_id,omitempty"`
	Input             any            `json:"input,omitempty"`
	Command           map[string]any `json:"command,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	StreamMode        any            `json:"stream_mode,omitempty"`
	MultitaskStrategy string         `json:"multitask_strategy,omitempty"`
	OnDisconnect      string         `json:"on_disconnect,omitempty"`
	Durability        string         `json:"durability,omitempty"`
}

type CreateStreamRunRequest struct {
	ThreadID          int64          `path:"thread_id,required" json:"-"`
	AssistantID       string         `json:"assistant_id,omitempty"`
	Input             any            `json:"input,omitempty"`
	Command           map[string]any `json:"command,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	StreamMode        any            `json:"stream_mode,omitempty"`
	MultitaskStrategy string         `json:"multitask_strategy,omitempty"`
	OnDisconnect      string         `json:"on_disconnect,omitempty"`
	Durability        string         `json:"durability,omitempty"`
	IntervalMs        int64          `query:"interval_ms,omitempty"`
	TimeoutMs         int64          `query:"timeout_ms,omitempty"`
}

type StatelessCreateRunRequest struct {
	AssistantID       string         `json:"assistant_id,omitempty"`
	Input             any            `json:"input,omitempty"`
	Command           map[string]any `json:"command,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	StreamMode        any            `json:"stream_mode,omitempty"`
	MultitaskStrategy string         `json:"multitask_strategy,omitempty"`
	OnDisconnect      string         `json:"on_disconnect,omitempty"`
	Durability        string         `json:"durability,omitempty"`
}

type StatelessCreateStreamRunRequest struct {
	AssistantID       string         `json:"assistant_id,omitempty"`
	Input             any            `json:"input,omitempty"`
	Command           map[string]any `json:"command,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	Config            map[string]any `json:"config,omitempty"`
	Context           map[string]any `json:"context,omitempty"`
	StreamMode        any            `json:"stream_mode,omitempty"`
	MultitaskStrategy string         `json:"multitask_strategy,omitempty"`
	OnDisconnect      string         `json:"on_disconnect,omitempty"`
	Durability        string         `json:"durability,omitempty"`
	IntervalMs        int64          `query:"interval_ms,omitempty"`
	TimeoutMs         int64          `query:"timeout_ms,omitempty"`
}

type ListRunsRequest struct {
	ThreadID int64  `path:"thread_id,required"`
	Status   string `query:"status,omitempty"`
	Limit    int32  `query:"limit,omitempty"`
	Offset   int32  `query:"offset,omitempty"`
}

type GetRunRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	RunID    int64 `path:"run_id,required"`
}

type ListRunMessagesRequest struct {
	ThreadID  int64 `path:"thread_id,required"`
	RunID     int64 `path:"run_id,required"`
	Limit     int32 `query:"limit,omitempty"`
	BeforeSeq int64 `query:"before_seq,omitempty"`
	AfterSeq  int64 `query:"after_seq,omitempty"`
}

type StatelessListRunMessagesRequest struct {
	RunID     int64 `path:"run_id,required"`
	Limit     int32 `query:"limit,omitempty"`
	BeforeSeq int64 `query:"before_seq,omitempty"`
	AfterSeq  int64 `query:"after_seq,omitempty"`
}

type StatelessRunFeedbackRequest struct {
	RunID int64 `path:"run_id,required"`
}

type ListThreadMessagesRequest struct {
	ThreadID  int64 `path:"thread_id,required"`
	Limit     int32 `query:"limit,omitempty"`
	BeforeSeq int64 `query:"before_seq,omitempty"`
	AfterSeq  int64 `query:"after_seq,omitempty"`
}

type ListRunEventsRequest struct {
	ThreadID   int64  `path:"thread_id,required"`
	RunID      int64  `path:"run_id,required"`
	EventTypes string `query:"event_types,omitempty"`
	Limit      int32  `query:"limit,omitempty"`
}

type RunMessagesPage struct {
	Data    []map[string]any `json:"data"`
	HasMore bool             `json:"has_more"`
}

type CancelRunRequest struct {
	ThreadID int64 `path:"thread_id,required"`
	RunID    int64 `path:"run_id,required"`
}

type StreamRunRequest struct {
	ThreadID     int64    `path:"thread_id,required"`
	RunID        int64    `path:"run_id,required"`
	Action       string   `query:"action,omitempty"`
	Wait         int32    `query:"wait,omitempty"`
	StreamMode   string   `query:"stream_mode,omitempty"`
	StreamModes  []string `json:"-" query:"-"`
	AfterEventID int64    `query:"after_event_id,omitempty"`
	IntervalMs   int64    `query:"interval_ms,omitempty"`
	TimeoutMs    int64    `query:"timeout_ms,omitempty"`
}

type JoinRunRequest struct {
	ThreadID     int64 `path:"thread_id,required"`
	RunID        int64 `path:"run_id,required"`
	AfterEventID int64 `query:"after_event_id,omitempty"`
	IntervalMs   int64 `query:"interval_ms,omitempty"`
	TimeoutMs    int64 `query:"timeout_ms,omitempty"`
}

type StatelessRunRequest struct {
	RunID int64 `path:"run_id,required"`
}

type StatelessStreamRunRequest struct {
	RunID        int64    `path:"run_id,required"`
	StreamMode   string   `query:"stream_mode,omitempty"`
	StreamModes  []string `json:"-" query:"-"`
	AfterEventID int64    `query:"after_event_id,omitempty"`
	IntervalMs   int64    `query:"interval_ms,omitempty"`
	TimeoutMs    int64    `query:"timeout_ms,omitempty"`
}

type StatelessJoinRunRequest struct {
	RunID        int64 `path:"run_id,required"`
	AfterEventID int64 `query:"after_event_id,omitempty"`
	IntervalMs   int64 `query:"interval_ms,omitempty"`
	TimeoutMs    int64 `query:"timeout_ms,omitempty"`
}
