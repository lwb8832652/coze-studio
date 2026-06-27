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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

type ArtifactFileReader interface {
	GetFileByID(ctx context.Context, id int64) (*entity.AgentFile, error)
}

type RegisterArtifactRequest struct {
	SpaceID      int64
	ThreadID     int64
	RunID        int64
	FileID       int64
	Title        string
	ArtifactType string
	Metadata     string
}

type ListArtifactsRequest struct {
	ThreadID    int64
	RunID       *int64
	DeletedOnly bool
	Page        int32
	PageSize    int32
}

type ListArtifactScanJobsRequest struct {
	ThreadID   int64
	RunID      *int64
	ArtifactID *int64
	Status     string
	Scanner    string
	Page       int32
	PageSize   int32
}

type GetArtifactRequest struct {
	ThreadID   int64
	ArtifactID int64
}

type DeleteArtifactRequest struct {
	ThreadID   int64
	ArtifactID int64
	DeletedAt  int64
}

type RestoreArtifactRequest struct {
	ThreadID   int64
	ArtifactID int64
	RestoredAt int64
}

type ListDeletedArtifactCleanupCandidatesRequest struct {
	CutoffDeletedAt int64
	Limit           int32
}

type MarkArtifactFileDeletedRequest struct {
	FileID    int64
	ObjectURI string
	DeletedAt int64
}

type UpdateArtifactScanResultRequest struct {
	ThreadID       int64
	ArtifactID     int64
	ScanStatus     string
	Scanner        string
	ScannerVersion string
	Reason         string
	ScannedAt      int64
}

type ClaimArtifactScanJobsRequest struct {
	Scanner        string
	WorkerID       string
	Limit          int32
	LeaseTTLMillis int64
}

type CompleteArtifactScanJobRequest struct {
	JobID          int64
	WorkerID       string
	ScanStatus     string
	ScannerVersion string
	Reason         string
	ScannedAt      int64
	EndedAt        int64
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
	EndedAt   int64
}

type ArtifactService interface {
	RegisterArtifact(
		ctx context.Context,
		req *RegisterArtifactRequest,
	) (*entity.AgentArtifact, bool, error)
	GetArtifact(
		ctx context.Context,
		req *GetArtifactRequest,
	) (*entity.AgentArtifact, error)
	DeleteArtifact(
		ctx context.Context,
		req *DeleteArtifactRequest,
	) (*entity.AgentArtifact, bool, error)
	RestoreArtifact(
		ctx context.Context,
		req *RestoreArtifactRequest,
	) (*entity.AgentArtifact, bool, error)
	ListDeletedArtifactCleanupCandidates(
		ctx context.Context,
		req *ListDeletedArtifactCleanupCandidatesRequest,
	) ([]*entity.AgentArtifact, error)
	MarkArtifactFileDeleted(
		ctx context.Context,
		req *MarkArtifactFileDeletedRequest,
	) (bool, error)
	UpdateArtifactScanResult(
		ctx context.Context,
		req *UpdateArtifactScanResultRequest,
	) (*entity.AgentArtifact, bool, error)
	ClaimArtifactScanJobs(
		ctx context.Context,
		req *ClaimArtifactScanJobsRequest,
	) ([]*entity.ArtifactScanJob, error)
	CompleteArtifactScanJob(
		ctx context.Context,
		req *CompleteArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	RetryArtifactScanJob(
		ctx context.Context,
		req *RetryArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	RequeueFailedArtifactScanJob(
		ctx context.Context,
		req *RequeueFailedArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	FailArtifactScanJob(
		ctx context.Context,
		req *FailArtifactScanJobRequest,
	) (*entity.ArtifactScanJob, bool, error)
	ListArtifactScanJobs(
		ctx context.Context,
		req *ListArtifactScanJobsRequest,
	) ([]*entity.ArtifactScanJob, int64, error)
	ListArtifacts(
		ctx context.Context,
		req *ListArtifactsRequest,
	) ([]*entity.AgentArtifact, int64, error)
}

type ArtifactComponents struct {
	FileReader   ArtifactFileReader
	ArtifactRepo repository.ArtifactRepository
	IDGen        idgen.IDGenerator
}

type artifactService struct {
	fileReader   ArtifactFileReader
	artifactRepo repository.ArtifactRepository
	idGen        idgen.IDGenerator
}

const defaultArtifactScanScanner = "default"
const defaultArtifactScanLeaseTTLMillis = int64(300000)

func NewArtifactService(c *ArtifactComponents) ArtifactService {
	if c == nil {
		return &artifactService{}
	}
	return &artifactService{
		fileReader:   c.FileReader,
		artifactRepo: c.ArtifactRepo,
		idGen:        c.IDGen,
	}
}

func DetermineArtifactPreviewMode(contentType string) entity.AgentArtifactPreviewMode {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		mediaType = strings.TrimSpace(contentType)
		if index := strings.Index(mediaType, ";"); index >= 0 {
			mediaType = mediaType[:index]
		}
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	switch mediaType {
	case "text/html", "application/xhtml+xml", "image/svg+xml":
		return entity.AgentArtifactPreviewModeDownload
	case "text/plain", "text/markdown", "text/csv", "text/tab-separated-values",
		"application/json":
		return entity.AgentArtifactPreviewModeText
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp",
		"image/tiff":
		return entity.AgentArtifactPreviewModeImage
	case "application/pdf":
		return entity.AgentArtifactPreviewModePDF
	default:
		return entity.AgentArtifactPreviewModeDownload
	}
}

func (s *artifactService) RegisterArtifact(
	ctx context.Context,
	req *RegisterArtifactRequest,
) (*entity.AgentArtifact, bool, error) {
	if s == nil || s.fileReader == nil || s.artifactRepo == nil || s.idGen == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"register artifact request is required",
		)
	}
	if req.SpaceID <= 0 || req.ThreadID <= 0 || req.RunID <= 0 ||
		req.FileID <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"artifact scope is required",
		)
	}
	artifactType := strings.TrimSpace(req.ArtifactType)
	if artifactType == "" {
		return nil, false, InvalidArgumentErrorf(
			"artifact type is required",
		)
	}
	metadata := strings.TrimSpace(req.Metadata)
	if metadata == "" {
		metadata = "{}"
	}
	if !json.Valid([]byte(metadata)) {
		return nil, false, InvalidArgumentErrorf(
			"artifact metadata must be valid json",
		)
	}

	file, err := s.fileReader.GetFileByID(ctx, req.FileID)
	if err != nil {
		return nil, false, err
	}
	if file == nil ||
		file.ID != req.FileID ||
		file.SpaceID != req.SpaceID ||
		file.ThreadID != req.ThreadID ||
		file.RunID != req.RunID ||
		file.Status != entity.AgentFileStatusActive ||
		(file.FileKind != entity.AgentFileKindWorkspace &&
			file.FileKind != entity.AgentFileKindOutput) ||
		file.SizeBytes <= 0 ||
		strings.TrimSpace(file.VirtualPath) == "" ||
		strings.TrimSpace(file.ObjectURI) == "" {
		return nil, false, InvalidArgumentErrorf(
			"artifact file is invalid",
		)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSpace(file.OriginalFileName)
	}
	if title == "" {
		title = strings.TrimSpace(file.FileName)
	}
	if title == "" {
		title = "artifact"
	}

	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, false, err
	}
	now := time.Now().UnixMilli()
	metadata, err = mergeArtifactPendingScanMetadata(
		metadata,
		defaultArtifactScanScanner,
		now,
	)
	if err != nil {
		return nil, false, err
	}
	artifact, created, err := s.artifactRepo.UpsertArtifact(ctx, &entity.AgentArtifact{
		ID:           id,
		SpaceID:      file.SpaceID,
		UserID:       file.UserID,
		ThreadID:     file.ThreadID,
		RunID:        file.RunID,
		FileID:       file.ID,
		Title:        title,
		ArtifactType: artifactType,
		VirtualPath:  strings.TrimSpace(file.VirtualPath),
		ObjectURI:    strings.TrimSpace(file.ObjectURI),
		ContentType:  strings.TrimSpace(file.ContentType),
		SizeBytes:    file.SizeBytes,
		PreviewMode:  DetermineArtifactPreviewMode(file.ContentType),
		Metadata:     metadata,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return nil, false, err
	}

	if _, _, err := s.enqueueArtifactScanJob(
		ctx,
		artifact,
		defaultArtifactScanScanner,
		now,
	); err != nil {
		return nil, false, err
	}
	return artifact, created, nil
}

func (s *artifactService) GetArtifact(
	ctx context.Context,
	req *GetArtifactRequest,
) (*entity.AgentArtifact, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, InvalidArgumentErrorf(
			"get artifact request is required",
		)
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, InvalidArgumentErrorf(
			"artifact scope is required",
		)
	}
	return s.artifactRepo.GetArtifact(ctx, req.ThreadID, req.ArtifactID)
}

func (s *artifactService) DeleteArtifact(
	ctx context.Context,
	req *DeleteArtifactRequest,
) (*entity.AgentArtifact, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"delete artifact request is required",
		)
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"artifact scope is required",
		)
	}
	deletedAt := req.DeletedAt
	if deletedAt <= 0 {
		deletedAt = time.Now().UnixMilli()
	}

	return s.artifactRepo.DeleteArtifact(
		ctx,
		req.ThreadID,
		req.ArtifactID,
		deletedAt,
	)
}

func (s *artifactService) RestoreArtifact(
	ctx context.Context,
	req *RestoreArtifactRequest,
) (*entity.AgentArtifact, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"restore artifact request is required",
		)
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"artifact scope is required",
		)
	}
	restoredAt := req.RestoredAt
	if restoredAt <= 0 {
		restoredAt = time.Now().UnixMilli()
	}

	return s.artifactRepo.RestoreArtifact(
		ctx,
		req.ThreadID,
		req.ArtifactID,
		restoredAt,
	)
}

func (s *artifactService) ListDeletedArtifactCleanupCandidates(
	ctx context.Context,
	req *ListDeletedArtifactCleanupCandidatesRequest,
) ([]*entity.AgentArtifact, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, InvalidArgumentErrorf(
			"list deleted artifact cleanup candidates request is required",
		)
	}
	if req.CutoffDeletedAt <= 0 {
		return nil, InvalidArgumentErrorf(
			"cleanup cutoff deleted_at is required",
		)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	return s.artifactRepo.ListDeletedArtifactCleanupCandidates(
		ctx,
		repository.ListDeletedArtifactCleanupCandidatesRequest{
			CutoffDeletedAt: req.CutoffDeletedAt,
			Limit:           int64(limit),
		},
	)
}

func (s *artifactService) MarkArtifactFileDeleted(
	ctx context.Context,
	req *MarkArtifactFileDeletedRequest,
) (bool, error) {
	if s == nil || s.artifactRepo == nil {
		return false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return false, InvalidArgumentErrorf(
			"mark artifact file deleted request is required",
		)
	}
	if req.FileID <= 0 || strings.TrimSpace(req.ObjectURI) == "" {
		return false, InvalidArgumentErrorf(
			"artifact file scope is required",
		)
	}
	deletedAt := req.DeletedAt
	if deletedAt <= 0 {
		deletedAt = time.Now().UnixMilli()
	}

	return s.artifactRepo.MarkArtifactFileDeleted(
		ctx,
		repository.MarkArtifactFileDeletedRequest{
			FileID:    req.FileID,
			ObjectURI: strings.TrimSpace(req.ObjectURI),
			DeletedAt: deletedAt,
		},
	)
}

func (s *artifactService) UpdateArtifactScanResult(
	ctx context.Context,
	req *UpdateArtifactScanResultRequest,
) (*entity.AgentArtifact, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"update artifact scan result request is required",
		)
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, false, InvalidArgumentErrorf(
			"artifact scope is required",
		)
	}
	scanStatus, err := normalizeArtifactScanResultStatus(req.ScanStatus)
	if err != nil {
		return nil, false, err
	}
	scannedAt := req.ScannedAt
	if scannedAt <= 0 {
		scannedAt = time.Now().UnixMilli()
	}

	artifact, err := s.artifactRepo.GetArtifact(
		ctx,
		req.ThreadID,
		req.ArtifactID,
	)
	if err != nil {
		return nil, false, err
	}
	if artifact == nil {
		return nil, false, nil
	}

	metadata, err := mergeArtifactScanMetadata(
		artifact.Metadata,
		scanStatus,
		boundedArtifactScanField(req.Scanner, 128),
		boundedArtifactScanField(req.ScannerVersion, 64),
		boundedArtifactScanField(req.Reason, 512),
		scannedAt,
	)
	if err != nil {
		return nil, false, err
	}

	return s.artifactRepo.UpdateArtifactScanMetadata(
		ctx,
		req.ThreadID,
		req.ArtifactID,
		metadata,
		scannedAt,
	)
}

func (s *artifactService) ClaimArtifactScanJobs(
	ctx context.Context,
	req *ClaimArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, InvalidArgumentErrorf(
			"claim artifact scan jobs request is required",
		)
	}
	workerID := boundedArtifactScanField(req.WorkerID, 128)
	if workerID == "" {
		return nil, InvalidArgumentErrorf("worker id is required")
	}
	scanner := boundedArtifactScanField(req.Scanner, 128)
	if scanner == "" {
		scanner = defaultArtifactScanScanner
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	leaseTTL := req.LeaseTTLMillis
	if leaseTTL <= 0 {
		leaseTTL = defaultArtifactScanLeaseTTLMillis
	}
	now := time.Now().UnixMilli()
	return s.artifactRepo.ClaimArtifactScanJobs(
		ctx,
		repository.ClaimArtifactScanJobsRequest{
			Scanner:        scanner,
			WorkerID:       workerID,
			Limit:          limit,
			Now:            now,
			LeaseExpiresAt: now + leaseTTL,
		},
	)
}

func (s *artifactService) CompleteArtifactScanJob(
	ctx context.Context,
	req *CompleteArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"complete artifact scan job request is required",
		)
	}
	if req.JobID <= 0 {
		return nil, false, InvalidArgumentErrorf("artifact scan job id is required")
	}
	workerID := boundedArtifactScanField(req.WorkerID, 128)
	if workerID == "" {
		return nil, false, InvalidArgumentErrorf("worker id is required")
	}
	scanStatus, err := normalizeArtifactScanResultStatus(req.ScanStatus)
	if err != nil {
		return nil, false, err
	}
	now := req.EndedAt
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	job, err := s.artifactRepo.GetArtifactScanJob(ctx, req.JobID)
	if err != nil {
		return nil, false, err
	}
	if !artifactScanJobCanFinish(job, workerID, now) {
		return nil, false, nil
	}
	scannedAt := req.ScannedAt
	if scannedAt <= 0 {
		scannedAt = now
	}
	if _, updated, err := s.UpdateArtifactScanResult(
		ctx,
		&UpdateArtifactScanResultRequest{
			ThreadID:       job.ThreadID,
			ArtifactID:     job.ArtifactID,
			ScanStatus:     scanStatus,
			Scanner:        job.Scanner,
			ScannerVersion: req.ScannerVersion,
			Reason:         req.Reason,
			ScannedAt:      scannedAt,
		},
	); err != nil {
		return nil, false, err
	} else if !updated {
		return nil, false, nil
	}
	return s.artifactRepo.CompleteArtifactScanJob(
		ctx,
		repository.CompleteArtifactScanJobRequest{
			JobID:    req.JobID,
			WorkerID: workerID,
			Now:      now,
		},
	)
}

func (s *artifactService) FailArtifactScanJob(
	ctx context.Context,
	req *FailArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"fail artifact scan job request is required",
		)
	}
	if req.JobID <= 0 {
		return nil, false, InvalidArgumentErrorf("artifact scan job id is required")
	}
	workerID := boundedArtifactScanField(req.WorkerID, 128)
	if workerID == "" {
		return nil, false, InvalidArgumentErrorf("worker id is required")
	}
	now := req.EndedAt
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	return s.artifactRepo.FailArtifactScanJob(
		ctx,
		repository.FailArtifactScanJobRequest{
			JobID:     req.JobID,
			WorkerID:  workerID,
			ErrorText: boundedArtifactScanField(req.ErrorText, 512),
			Now:       now,
		},
	)
}

func (s *artifactService) RetryArtifactScanJob(
	ctx context.Context,
	req *RetryArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"retry artifact scan job request is required",
		)
	}
	if req.JobID <= 0 {
		return nil, false, InvalidArgumentErrorf("artifact scan job id is required")
	}
	workerID := boundedArtifactScanField(req.WorkerID, 128)
	if workerID == "" {
		return nil, false, InvalidArgumentErrorf("worker id is required")
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	availableAt := req.AvailableAt
	if availableAt <= now {
		availableAt = now
	}
	return s.artifactRepo.RetryArtifactScanJob(
		ctx,
		repository.RetryArtifactScanJobRequest{
			JobID:       req.JobID,
			WorkerID:    workerID,
			ErrorText:   boundedArtifactScanField(req.ErrorText, 512),
			AvailableAt: availableAt,
			Now:         now,
		},
	)
}

func (s *artifactService) RequeueFailedArtifactScanJob(
	ctx context.Context,
	req *RequeueFailedArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, false, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf(
			"requeue failed artifact scan job request is required",
		)
	}
	if req.JobID <= 0 {
		return nil, false, InvalidArgumentErrorf("artifact scan job id is required")
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("artifact thread id is required")
	}
	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	availableAt := req.AvailableAt
	if availableAt <= now {
		availableAt = now
	}
	return s.artifactRepo.RequeueFailedArtifactScanJob(
		ctx,
		repository.RequeueFailedArtifactScanJobRequest{
			JobID:       req.JobID,
			ThreadID:    req.ThreadID,
			ErrorText:   boundedArtifactScanField(req.ErrorText, 512),
			AvailableAt: availableAt,
			Now:         now,
		},
	)
}

func (s *artifactService) ListArtifactScanJobs(
	ctx context.Context,
	req *ListArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, int64, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, 0, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf(
			"list artifact scan jobs request is required",
		)
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf(
			"artifact thread id is required",
		)
	}

	var status *entity.ArtifactScanJobStatus
	normalizedStatus := strings.ToLower(strings.TrimSpace(req.Status))
	if normalizedStatus != "" {
		mapped, err := normalizeArtifactScanJobStatus(normalizedStatus)
		if err != nil {
			return nil, 0, err
		}
		status = &mapped
	}

	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return s.artifactRepo.ListArtifactScanJobs(
		ctx,
		repository.ListArtifactScanJobsRequest{
			ThreadID:   req.ThreadID,
			RunID:      req.RunID,
			ArtifactID: req.ArtifactID,
			Status:     status,
			Scanner:    boundedArtifactScanField(req.Scanner, 128),
			Page:       int64(page),
			PageSize:   int64(pageSize),
		},
	)
}

func artifactScanJobCanFinish(
	job *entity.ArtifactScanJob,
	workerID string,
	now int64,
) bool {
	return job != nil &&
		job.Status == entity.ArtifactScanJobStatusProcessing &&
		job.WorkerID == workerID &&
		job.LeaseExpiresAt > now
}

func (s *artifactService) ListArtifacts(
	ctx context.Context,
	req *ListArtifactsRequest,
) ([]*entity.AgentArtifact, int64, error) {
	if s == nil || s.artifactRepo == nil {
		return nil, 0, InvalidArgumentErrorf(
			"artifact service is not configured",
		)
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf(
			"list artifacts request is required",
		)
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf(
			"artifact thread id is required",
		)
	}
	return s.artifactRepo.ListArtifacts(ctx, repository.ListArtifactsRequest{
		ThreadID:    req.ThreadID,
		RunID:       req.RunID,
		DeletedOnly: req.DeletedOnly,
		Page:        int64(req.Page),
		PageSize:    int64(req.PageSize),
	})
}

func normalizeArtifactScanResultStatus(status string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "clean", "pending", "failed", "blocked", "infected", "quarantined":
		return normalized, nil
	default:
		return "", InvalidArgumentErrorf("artifact scan status is invalid")
	}
}

func normalizeArtifactScanJobStatus(status string) (entity.ArtifactScanJobStatus, error) {
	normalized := entity.ArtifactScanJobStatus(
		strings.ToLower(strings.TrimSpace(status)),
	)
	switch normalized {
	case entity.ArtifactScanJobStatusPending,
		entity.ArtifactScanJobStatusProcessing,
		entity.ArtifactScanJobStatusSucceeded,
		entity.ArtifactScanJobStatusFailed:
		return normalized, nil
	default:
		return "", InvalidArgumentErrorf("artifact scan job status is invalid")
	}
}

func mergeArtifactScanMetadata(
	existing string,
	scanStatus string,
	scanner string,
	scannerVersion string,
	reason string,
	scannedAt int64,
) (string, error) {
	metadata := make(map[string]any)
	trimmed := strings.TrimSpace(existing)
	if trimmed != "" {
		_ = json.Unmarshal([]byte(trimmed), &metadata)
	}
	metadata["scan_status"] = scanStatus
	metadata["scan_scanned_at"] = scannedAt
	setOrDeleteArtifactScanMetadata(metadata, "scan_scanner", scanner)
	setOrDeleteArtifactScanMetadata(metadata, "scan_scanner_version", scannerVersion)
	setOrDeleteArtifactScanMetadata(metadata, "scan_reason", reason)

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func setOrDeleteArtifactScanMetadata(
	metadata map[string]any,
	key string,
	value string,
) {
	if value == "" {
		delete(metadata, key)
		return
	}
	metadata[key] = value
}

func boundedArtifactScanField(value string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}

func mergeArtifactPendingScanMetadata(
	existing string,
	scanner string,
	requestedAt int64,
) (string, error) {
	metadata := make(map[string]any)
	trimmed := strings.TrimSpace(existing)
	if trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &metadata); err != nil {
			return "", err
		}
	}
	metadata["scan_status"] = "pending"
	metadata["scan_scanner"] = boundedArtifactScanField(scanner, 128)
	metadata["scan_requested_at"] = requestedAt
	delete(metadata, "scan_scanner_version")
	delete(metadata, "scan_reason")
	delete(metadata, "scan_scanned_at")
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *artifactService) enqueueArtifactScanJob(
	ctx context.Context,
	artifact *entity.AgentArtifact,
	scanner string,
	availableAt int64,
) (*entity.ArtifactScanJob, bool, error) {
	if artifact == nil || artifact.ID <= 0 {
		return nil, false, InvalidArgumentErrorf("artifact is required")
	}
	jobID, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, false, err
	}
	if availableAt <= 0 {
		availableAt = time.Now().UnixMilli()
	}
	scanner = boundedArtifactScanField(scanner, 128)
	if scanner == "" {
		scanner = defaultArtifactScanScanner
	}
	return s.artifactRepo.CreateOrGetArtifactScanJob(ctx, &entity.ArtifactScanJob{
		ID:             jobID,
		ThreadID:       artifact.ThreadID,
		RunID:          artifact.RunID,
		SpaceID:        artifact.SpaceID,
		UserID:         artifact.UserID,
		ArtifactID:     artifact.ID,
		FileID:         artifact.FileID,
		Scanner:        scanner,
		IdempotencyKey: fmt.Sprintf("artifact_scan:%d:%s", artifact.ID, scanner),
		Status:         entity.ArtifactScanJobStatusPending,
		AvailableAt:    availableAt,
		CreatedAt:      availableAt,
		UpdatedAt:      availableAt,
	})
}
