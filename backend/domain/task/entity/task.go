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

type Status string

const (
	StatusCreated   Status = "created"
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceling Status = "canceling"
	StatusCanceled  Status = "canceled"
)

type Task struct {
	ID             int64
	SpaceID        int64
	CreatorID      int64
	ConversationID int64
	MessageID      int64
	SkillID        int64
	Title          string
	Status         Status
	Progress       int32
	Input          string
	Result         string
	Error          string
	CreatedAt      int64
	UpdatedAt      int64
}

type Attempt struct {
	ID        int64
	TaskID    int64
	AttemptNo int32
	Status    Status
	StartedAt int64
	EndedAt   int64
	Runtime   string
	Error     string
}

type Event struct {
	ID        int64
	TaskID    int64
	EventType string
	Payload   string
	CreatedAt int64
}
