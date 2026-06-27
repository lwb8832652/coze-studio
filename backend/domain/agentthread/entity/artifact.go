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

type AgentArtifactPreviewMode string

const (
	AgentArtifactPreviewModeText        AgentArtifactPreviewMode = "text"
	AgentArtifactPreviewModeImage       AgentArtifactPreviewMode = "image"
	AgentArtifactPreviewModePDF         AgentArtifactPreviewMode = "pdf"
	AgentArtifactPreviewModeDownload    AgentArtifactPreviewMode = "download"
	AgentArtifactPreviewModeUnsupported AgentArtifactPreviewMode = "unsupported"
)

type ArtifactScanJobStatus string

const (
	ArtifactScanJobStatusPending    ArtifactScanJobStatus = "pending"
	ArtifactScanJobStatusProcessing ArtifactScanJobStatus = "processing"
	ArtifactScanJobStatusSucceeded  ArtifactScanJobStatus = "succeeded"
	ArtifactScanJobStatusFailed     ArtifactScanJobStatus = "failed"
)

type AgentArtifact struct {
	ID           int64
	SpaceID      int64
	UserID       int64
	ThreadID     int64
	RunID        int64
	FileID       int64
	Title        string
	ArtifactType string
	VirtualPath  string
	ObjectURI    string
	ContentType  string
	SizeBytes    int64
	PreviewMode  AgentArtifactPreviewMode
	Metadata     string
	CreatedAt    int64
	UpdatedAt    int64
	DeletedAt    int64
}

type ArtifactScanJob struct {
	ID             int64
	ThreadID       int64
	RunID          int64
	SpaceID        int64
	UserID         int64
	ArtifactID     int64
	FileID         int64
	Scanner        string
	IdempotencyKey string
	Status         ArtifactScanJobStatus
	WorkerID       string
	AttemptCount   int32
	LastError      string
	AvailableAt    int64
	LeaseExpiresAt int64
	StartedAt      int64
	EndedAt        int64
	CreatedAt      int64
	UpdatedAt      int64
}
