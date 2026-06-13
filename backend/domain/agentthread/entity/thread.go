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
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
)

type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
	MessageRoleSystem    MessageRole = "system"
)

type Thread struct {
	ID            int64
	SpaceID       int64
	CreatorID     int64
	AgentID       int64
	Title         string
	Status        ThreadStatus
	Source        ThreadSource
	LegacyTaskID  int64
	Metadata      string
	CreatedAt     int64
	UpdatedAt     int64
	LastMessageAt int64
}

type Run struct {
	ID        int64
	ThreadID  int64
	Status    RunStatus
	Input     string
	Output    string
	Error     string
	CreatedAt int64
	UpdatedAt int64
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
