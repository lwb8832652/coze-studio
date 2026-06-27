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

type AgentRunPlanItemStatus string

const (
	AgentRunPlanItemStatusPending    AgentRunPlanItemStatus = "pending"
	AgentRunPlanItemStatusInProgress AgentRunPlanItemStatus = "in_progress"
	AgentRunPlanItemStatusCompleted  AgentRunPlanItemStatus = "completed"
	AgentRunPlanItemStatusDeleted    AgentRunPlanItemStatus = "deleted"
)

type AgentRunPlan struct {
	RunID         int64
	ThreadID      int64
	SpaceID       int64
	UserID        int64
	HighWatermark int64
	Revision      int64
	CreatedAt     int64
	UpdatedAt     int64
}

type AgentRunPlanItem struct {
	ID          int64
	RunID       int64
	TaskID      int64
	Subject     string
	Description string
	Status      AgentRunPlanItemStatus
	ActiveForm  string
	Owner       string
	Blocks      string
	BlockedBy   string
	Metadata    string
	Active      bool
	Version     int64
	CreatedAt   int64
	UpdatedAt   int64
}
