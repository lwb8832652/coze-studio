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

package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

type ArtifactRepository interface {
	UpsertArtifact(
		ctx context.Context,
		artifact *entity.AgentArtifact,
	) (*entity.AgentArtifact, bool, error)
	GetArtifact(
		ctx context.Context,
		threadID int64,
		artifactID int64,
	) (*entity.AgentArtifact, error)
	DeleteArtifact(
		ctx context.Context,
		threadID int64,
		artifactID int64,
		deletedAt int64,
	) (*entity.AgentArtifact, bool, error)
	RestoreArtifact(
		ctx context.Context,
		threadID int64,
		artifactID int64,
		restoredAt int64,
	) (*entity.AgentArtifact, bool, error)
	ListDeletedArtifactCleanupCandidates(
		ctx context.Context,
		req ListDeletedArtifactCleanupCandidatesRequest,
	) ([]*entity.AgentArtifact, error)
	MarkArtifactFileDeleted(
		ctx context.Context,
		req MarkArtifactFileDeletedRequest,
	) (bool, error)
	UpdateArtifactScanMetadata(
		ctx context.Context,
		threadID int64,
		artifactID int64,
		metadata string,
		updatedAt int64,
	) (*entity.AgentArtifact, bool, error)
	CreateOrGetArtifactScanJob(
		ctx context.Context,
		job *entity.ArtifactScanJob,
	) (*entity.ArtifactScanJob, bool, error)
	ClaimArtifactScanJobs(
		ctx context.Context,
		req ClaimArtifactScanJobsRequest,
	) ([]*entity.ArtifactScanJob, error)
	GetArtifactScanJob(
		ctx context.Context,
		jobID int64,
	) (*entity.ArtifactScanJob, error)
	CompleteArtifactScanJob(
		ctx context.Context,
		req CompleteArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	RetryArtifactScanJob(
		ctx context.Context,
		req RetryArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	RequeueFailedArtifactScanJob(
		ctx context.Context,
		req RequeueFailedArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	FailArtifactScanJob(
		ctx context.Context,
		req FailArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	ListArtifactScanJobs(
		ctx context.Context,
		req ListArtifactScanJobsRequest,
	) ([]*entity.ArtifactScanJob, int64, error)
	ListArtifacts(
		ctx context.Context,
		req ListArtifactsRequest,
	) ([]*entity.AgentArtifact, int64, error)
}

type ListArtifactsRequest struct {
	ThreadID    int64
	RunID       *int64
	DeletedOnly bool
	Page        int64
	PageSize    int64
}

type ListDeletedArtifactCleanupCandidatesRequest struct {
	CutoffDeletedAt int64
	Limit           int64
}

type MarkArtifactFileDeletedRequest struct {
	FileID    int64
	ObjectURI string
	DeletedAt int64
}

type ListArtifactScanJobsRequest struct {
	ThreadID   int64
	RunID      *int64
	ArtifactID *int64
	Status     *entity.ArtifactScanJobStatus
	Scanner    string
	Page       int64
	PageSize   int64
}

type ClaimArtifactScanJobsRequest struct {
	Scanner        string
	WorkerID       string
	Limit          int32
	Now            int64
	LeaseExpiresAt int64
}

type CompleteArtifactScanJobRequest struct {
	JobID    int64
	WorkerID string
	Now      int64
}

type RetryArtifactScanJobRequest struct {
	JobID       int64
	WorkerID    string
	ErrorText   string
	AvailableAt int64
	Now         int64
}

type RequeueFailedArtifactScanJobRequest struct {
	JobID       int64
	ThreadID    int64
	ErrorText   string
	AvailableAt int64
	Now         int64
}

type FailArtifactScanJobRequest struct {
	JobID     int64
	WorkerID  string
	ErrorText string
	Now       int64
}

func NewArtifactRepository(db *gorm.DB) ArtifactRepository {
	return &threadRepository{db: db}
}
