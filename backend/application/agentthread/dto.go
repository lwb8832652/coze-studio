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

type ThreadSummary struct {
	ThreadID         int64
	LegacyTaskID     int64
	SpaceID          int64
	CreatorID        int64
	Title            string
	Status           ThreadStatus
	Source           ThreadSource
	Progress         int32
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
