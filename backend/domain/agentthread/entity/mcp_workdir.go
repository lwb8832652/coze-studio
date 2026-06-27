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

type MCPRuntimeWorkdirLeaseStatus string

const (
	MCPRuntimeWorkdirLeaseStatusActive   MCPRuntimeWorkdirLeaseStatus = "active"
	MCPRuntimeWorkdirLeaseStatusReleased MCPRuntimeWorkdirLeaseStatus = "released"
	MCPRuntimeWorkdirLeaseStatusFailed   MCPRuntimeWorkdirLeaseStatus = "failed"
)

type MCPRuntimeWorkdirLease struct {
	ID              int64
	SpaceID         int64
	ThreadID        int64
	RunID           int64
	ServerID        int64
	RuntimeToolName string
	Workdir         string
	Status          MCPRuntimeWorkdirLeaseStatus
	WorkerID        string
	LeaseExpiresAt  int64
	ReleasedAt      int64
	LastError       string
	CreatedAt       int64
	UpdatedAt       int64
}
