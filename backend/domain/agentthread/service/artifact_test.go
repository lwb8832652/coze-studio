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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestArtifactServiceRegistersActiveOutputFile(t *testing.T) {
	file := artifactServiceTestFile()
	files := &recordingArtifactFileReader{file: file}
	artifacts := &recordingArtifactRepository{}
	svc := NewArtifactService(&ArtifactComponents{
		FileReader:   files,
		ArtifactRepo: artifacts,
		IDGen:        &sequenceIDGen{next: 100},
	})

	artifact, created, err := svc.RegisterArtifact(
		context.Background(),
		&RegisterArtifactRequest{
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			ArtifactType: "report",
			Metadata:     `{"source":"present_files"}`,
		},
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(100), artifact.ID)
	require.Equal(t, int64(90), artifact.FileID)
	require.Equal(t, "report.txt", artifact.Title)
	require.Equal(t, "report", artifact.ArtifactType)
	require.Equal(t, entity.AgentArtifactPreviewModeText, artifact.PreviewMode)
	require.Equal(t, file.VirtualPath, artifact.VirtualPath)
	require.Equal(t, file.ObjectURI, artifact.ObjectURI)
	require.Equal(t, file.ContentType, artifact.ContentType)
	require.Equal(t, file.SizeBytes, artifact.SizeBytes)
	require.NotZero(t, artifact.CreatedAt)
	require.Equal(t, artifact.CreatedAt, artifact.UpdatedAt)
	require.Equal(t, artifact, artifacts.upserted)
}

func TestArtifactServiceRegistersPendingScanAndEnqueuesJob(t *testing.T) {
	file := artifactServiceTestFile()
	files := &recordingArtifactFileReader{file: file}
	artifacts := &recordingArtifactRepository{}
	svc := NewArtifactService(&ArtifactComponents{
		FileReader:   files,
		ArtifactRepo: artifacts,
		IDGen:        &sequenceIDGen{next: 100},
	})

	artifact, created, err := svc.RegisterArtifact(
		context.Background(),
		&RegisterArtifactRequest{
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			ArtifactType: "report",
			Metadata:     `{"source":"present_files","scan_status":"clean"}`,
		},
	)

	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, artifact)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(artifact.Metadata), &metadata))
	require.Equal(t, "present_files", metadata["source"])
	require.Equal(t, "pending", metadata["scan_status"])
	require.Equal(t, "default", metadata["scan_scanner"])
	require.NotZero(t, metadata["scan_requested_at"])

	require.NotNil(t, artifacts.scanJob)
	require.Equal(t, int64(10), artifacts.scanJob.ThreadID)
	require.Equal(t, int64(20), artifacts.scanJob.RunID)
	require.Equal(t, int64(30), artifacts.scanJob.SpaceID)
	require.Equal(t, int64(40), artifacts.scanJob.UserID)
	require.Equal(t, int64(100), artifacts.scanJob.ArtifactID)
	require.Equal(t, int64(90), artifacts.scanJob.FileID)
	require.Equal(t, "default", artifacts.scanJob.Scanner)
	require.Equal(t, "artifact_scan:100:default", artifacts.scanJob.IdempotencyKey)
	require.Equal(t, entity.ArtifactScanJobStatusPending, artifacts.scanJob.Status)
	require.Equal(t, artifacts.scanJob.CreatedAt, artifacts.scanJob.AvailableAt)
	require.NotContains(t, artifacts.scanJob.IdempotencyKey, "agent-runtime")
	require.NotContains(t, artifacts.scanJob.IdempotencyKey, "/mnt/user-data")
}

func TestArtifactServiceForcesActiveContentToDownload(t *testing.T) {
	file := artifactServiceTestFile()
	file.FileName = "page.html"
	file.ContentType = "text/html; charset=utf-8"
	files := &recordingArtifactFileReader{file: file}
	artifacts := &recordingArtifactRepository{}
	svc := NewArtifactService(&ArtifactComponents{
		FileReader:   files,
		ArtifactRepo: artifacts,
		IDGen:        &sequenceIDGen{next: 100},
	})

	artifact, _, err := svc.RegisterArtifact(
		context.Background(),
		&RegisterArtifactRequest{
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			ArtifactType: "html",
			Metadata:     `{}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, entity.AgentArtifactPreviewModeDownload, artifact.PreviewMode)
}

func TestArtifactServiceGetsScopedArtifact(t *testing.T) {
	expected := &entity.AgentArtifact{
		ID:           100,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "Report",
		ArtifactType: "report",
		ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
		ContentType:  "text/plain; charset=utf-8",
		SizeBytes:    128,
		PreviewMode:  entity.AgentArtifactPreviewModeText,
	}
	artifacts := &recordingArtifactRepository{got: expected}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	got, err := svc.GetArtifact(context.Background(), &GetArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
	})

	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, int64(10), artifacts.threadID)
	require.Equal(t, int64(100), artifacts.artifactID)
	require.Equal(t, int64(100), got.ID)
	require.Equal(t, "Report", got.Title)
}

func TestArtifactServiceDeletesScopedArtifact(t *testing.T) {
	expected := &entity.AgentArtifact{
		ID:           100,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "Report",
		ArtifactType: "report",
		ContentType:  "text/plain",
		SizeBytes:    128,
		PreviewMode:  entity.AgentArtifactPreviewModeText,
		DeletedAt:    1300,
	}
	artifacts := &recordingArtifactRepository{deleted: expected}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	deleted, ok, err := svc.DeleteArtifact(context.Background(), &DeleteArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
		DeletedAt:  1300,
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, deleted)
	require.Equal(t, int64(10), artifacts.deleteThreadID)
	require.Equal(t, int64(100), artifacts.deleteArtifactID)
	require.Equal(t, int64(1300), artifacts.deletedAt)
	require.Equal(t, int64(100), deleted.ID)
	require.Equal(t, int64(1300), deleted.DeletedAt)
}

func TestArtifactServiceRestoresScopedArtifact(t *testing.T) {
	expected := &entity.AgentArtifact{
		ID:           100,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "Report",
		ArtifactType: "report",
		ContentType:  "text/plain",
		SizeBytes:    128,
		PreviewMode:  entity.AgentArtifactPreviewModeText,
		DeletedAt:    0,
		UpdatedAt:    1500,
	}
	artifacts := &recordingArtifactRepository{restored: expected}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	restored, ok, err := svc.RestoreArtifact(context.Background(), &RestoreArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
		RestoredAt: 1500,
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, restored)
	require.Equal(t, int64(10), artifacts.restoreThreadID)
	require.Equal(t, int64(100), artifacts.restoreArtifactID)
	require.Equal(t, int64(1500), artifacts.restoredAt)
	require.Equal(t, int64(100), restored.ID)
	require.Equal(t, int64(0), restored.DeletedAt)
}

func TestArtifactServiceListsAndMarksCleanupCandidates(t *testing.T) {
	expected := []*entity.AgentArtifact{
		{
			ID:        100,
			ThreadID:  10,
			RunID:     20,
			FileID:    90,
			ObjectURI: "agent-runtime/30/10/runs/20/tool-results/trunc/file.txt",
			DeletedAt: 1200,
		},
	}
	artifacts := &recordingArtifactRepository{
		cleanupCandidates: expected,
		markFileDeletedOK: true,
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	got, err := svc.ListDeletedArtifactCleanupCandidates(
		context.Background(),
		&ListDeletedArtifactCleanupCandidatesRequest{
			CutoffDeletedAt: 1500,
			Limit:           1000,
		},
	)

	require.NoError(t, err)
	require.Equal(t, expected, got)
	require.Equal(t, int64(1500), artifacts.cleanupReq.CutoffDeletedAt)
	require.Equal(t, int64(500), artifacts.cleanupReq.Limit)

	marked, err := svc.MarkArtifactFileDeleted(
		context.Background(),
		&MarkArtifactFileDeletedRequest{
			FileID:    90,
			ObjectURI: "agent-runtime/30/10/runs/20/tool-results/trunc/file.txt",
			DeletedAt: 2000,
		},
	)

	require.NoError(t, err)
	require.True(t, marked)
	require.Equal(t, int64(90), artifacts.markFileDeletedReq.FileID)
	require.Equal(
		t,
		"agent-runtime/30/10/runs/20/tool-results/trunc/file.txt",
		artifacts.markFileDeletedReq.ObjectURI,
	)
	require.Equal(t, int64(2000), artifacts.markFileDeletedReq.DeletedAt)
}

func TestArtifactServiceUpdatesScanResultPreservingMetadata(t *testing.T) {
	expected := &entity.AgentArtifact{
		ID:           100,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "Report",
		ArtifactType: "report",
		ContentType:  "text/plain",
		SizeBytes:    128,
		Metadata:     `{"source":"present_files","scan_scanner":"old","scan_reason":"old"}`,
	}
	artifacts := &recordingArtifactRepository{got: expected}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	updated, ok, err := svc.UpdateArtifactScanResult(
		context.Background(),
		&UpdateArtifactScanResultRequest{
			ThreadID:       10,
			ArtifactID:     100,
			ScanStatus:     "clean",
			Scanner:        " clamav ",
			ScannerVersion: " 1.2.3 ",
			Reason:         " no risky content ",
			ScannedAt:      2000,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, updated)
	require.Equal(t, int64(10), artifacts.threadID)
	require.Equal(t, int64(100), artifacts.artifactID)
	require.Equal(t, int64(2000), artifacts.updateScannedAt)
	require.JSONEq(
		t,
		`{"source":"present_files","scan_status":"clean","scan_scanner":"clamav","scan_scanner_version":"1.2.3","scan_reason":"no risky content","scan_scanned_at":2000}`,
		artifacts.updateMetadata,
	)
	require.Equal(t, artifacts.updateMetadata, updated.Metadata)
}

func TestArtifactServiceRejectsInvalidScanStatusBeforeRepositoryUpdate(t *testing.T) {
	artifacts := &recordingArtifactRepository{
		got: &entity.AgentArtifact{
			ID:       100,
			ThreadID: 10,
			Metadata: `{}`,
		},
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	updated, ok, err := svc.UpdateArtifactScanResult(
		context.Background(),
		&UpdateArtifactScanResultRequest{
			ThreadID:   10,
			ArtifactID: 100,
			ScanStatus: "unknown",
			ScannedAt:  2000,
		},
	)

	require.Error(t, err)
	require.False(t, ok)
	require.Nil(t, updated)
	require.Empty(t, artifacts.updateMetadata)
}

func TestArtifactServiceClaimScanJobsNormalizesRequest(t *testing.T) {
	artifacts := &recordingArtifactRepository{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:             3001,
				ThreadID:       10,
				RunID:          20,
				ArtifactID:     100,
				Scanner:        "default",
				Status:         entity.ArtifactScanJobStatusProcessing,
				WorkerID:       "worker-a",
				LeaseExpiresAt: 500,
			},
		},
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	claimed, err := svc.ClaimArtifactScanJobs(
		context.Background(),
		&ClaimArtifactScanJobsRequest{
			Scanner:  " default ",
			WorkerID: " worker-a ",
		},
	)

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, "default", artifacts.claimScanJobsReq.Scanner)
	require.Equal(t, "worker-a", artifacts.claimScanJobsReq.WorkerID)
	require.Equal(t, int32(10), artifacts.claimScanJobsReq.Limit)
	require.NotZero(t, artifacts.claimScanJobsReq.Now)
	require.Equal(t, int64(300000), artifacts.claimScanJobsReq.LeaseExpiresAt-artifacts.claimScanJobsReq.Now)
}

func TestArtifactServiceClaimScanJobsRequiresWorkerID(t *testing.T) {
	artifacts := &recordingArtifactRepository{}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	claimed, err := svc.ClaimArtifactScanJobs(
		context.Background(),
		&ClaimArtifactScanJobsRequest{
			WorkerID: " ",
		},
	)

	require.Error(t, err)
	require.Nil(t, claimed)
	require.Empty(t, artifacts.claimScanJobsReq.WorkerID)
}

func TestArtifactServiceCompleteScanJobUpdatesArtifactBeforeTerminal(t *testing.T) {
	artifacts := &recordingArtifactRepository{
		scanJob: &entity.ArtifactScanJob{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			UserID:         40,
			ArtifactID:     100,
			FileID:         90,
			Scanner:        "default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			WorkerID:       "worker-a",
			LeaseExpiresAt: 5000,
		},
		got: &entity.AgentArtifact{
			ID:       100,
			ThreadID: 10,
			Metadata: `{"scan_status":"pending"}`,
		},
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	job, ok, err := svc.CompleteArtifactScanJob(
		context.Background(),
		&CompleteArtifactScanJobRequest{
			JobID:          3001,
			WorkerID:       " worker-a ",
			ScanStatus:     "clean",
			ScannerVersion: "1.2.3",
			Reason:         "safe",
			ScannedAt:      3000,
			EndedAt:        3000,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, job)
	require.Equal(t, int64(3001), artifacts.completeScanJobReq.JobID)
	require.Equal(t, "worker-a", artifacts.completeScanJobReq.WorkerID)
	require.NotZero(t, artifacts.completeScanJobReq.Now)
	require.JSONEq(
		t,
		`{"scan_status":"clean","scan_scanner":"default","scan_scanner_version":"1.2.3","scan_reason":"safe","scan_scanned_at":3000}`,
		artifacts.updateMetadata,
	)
	require.Equal(t, int64(3000), artifacts.updateScannedAt)
}

func TestArtifactServiceFailScanJobDoesNotMutateArtifactMetadata(t *testing.T) {
	artifacts := &recordingArtifactRepository{
		scanJob: &entity.ArtifactScanJob{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			ArtifactID:     100,
			Scanner:        "default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			WorkerID:       "worker-a",
			LeaseExpiresAt: 5000,
		},
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	job, ok, err := svc.FailArtifactScanJob(
		context.Background(),
		&FailArtifactScanJobRequest{
			JobID:     3001,
			WorkerID:  "worker-a",
			ErrorText: "scanner unavailable",
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, job)
	require.Equal(t, int64(3001), artifacts.failScanJobReq.JobID)
	require.Equal(t, "worker-a", artifacts.failScanJobReq.WorkerID)
	require.Equal(t, "scanner unavailable", artifacts.failScanJobReq.ErrorText)
	require.Empty(t, artifacts.updateMetadata)
}

func TestArtifactServiceRetryScanJobDoesNotMutateArtifactMetadata(t *testing.T) {
	artifacts := &recordingArtifactRepository{
		scanJob: &entity.ArtifactScanJob{
			ID:             3001,
			ThreadID:       10,
			RunID:          20,
			ArtifactID:     100,
			Scanner:        "default",
			Status:         entity.ArtifactScanJobStatusProcessing,
			WorkerID:       "worker-a",
			LeaseExpiresAt: 5000,
		},
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	job, ok, err := svc.RetryArtifactScanJob(
		context.Background(),
		&RetryArtifactScanJobRequest{
			JobID:       3001,
			WorkerID:    "worker-a",
			ErrorText:   "scanner unavailable",
			AvailableAt: 7000,
			Now:         3000,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, job)
	require.Equal(t, int64(3001), artifacts.retryScanJobReq.JobID)
	require.Equal(t, "worker-a", artifacts.retryScanJobReq.WorkerID)
	require.Equal(t, "scanner unavailable", artifacts.retryScanJobReq.ErrorText)
	require.Equal(t, int64(7000), artifacts.retryScanJobReq.AvailableAt)
	require.Equal(t, int64(3000), artifacts.retryScanJobReq.Now)
	require.Empty(t, artifacts.updateMetadata)
}

func TestArtifactServiceRequeueFailedScanJobDoesNotMutateArtifactMetadata(t *testing.T) {
	artifacts := &recordingArtifactRepository{
		scanJob: &entity.ArtifactScanJob{
			ID:         3001,
			ThreadID:   10,
			RunID:      20,
			ArtifactID: 100,
			Scanner:    "default",
			Status:     entity.ArtifactScanJobStatusPending,
		},
	}
	svc := NewArtifactService(&ArtifactComponents{ArtifactRepo: artifacts})

	job, ok, err := svc.RequeueFailedArtifactScanJob(
		context.Background(),
		&RequeueFailedArtifactScanJobRequest{
			JobID:       3001,
			ThreadID:    10,
			ErrorText:   "manual retry requested",
			AvailableAt: 7000,
			Now:         3000,
		},
	)

	require.NoError(t, err)
	require.True(t, ok)
	require.NotNil(t, job)
	require.Equal(t, int64(3001), artifacts.requeueFailedScanJobReq.JobID)
	require.Equal(t, int64(10), artifacts.requeueFailedScanJobReq.ThreadID)
	require.Equal(
		t,
		"manual retry requested",
		artifacts.requeueFailedScanJobReq.ErrorText,
	)
	require.Equal(t, int64(7000), artifacts.requeueFailedScanJobReq.AvailableAt)
	require.Equal(t, int64(3000), artifacts.requeueFailedScanJobReq.Now)
	require.Empty(t, artifacts.updateMetadata)
}

func TestArtifactServiceListScanJobsMapsFilters(t *testing.T) {
	runID := int64(20)
	artifactID := int64(100)
	artifacts := &recordingArtifactRepository{
		listScanJobs: []*entity.ArtifactScanJob{
			{
				ID:         3001,
				ThreadID:   10,
				RunID:      runID,
				ArtifactID: artifactID,
				Scanner:    "clamav",
				Status:     entity.ArtifactScanJobStatusFailed,
				LastError:  "scanner unavailable",
			},
		},
		listScanJobsTotal: 1,
	}
	svc := NewArtifactService(&ArtifactComponents{
		ArtifactRepo: artifacts,
	})

	jobs, total, err := svc.ListArtifactScanJobs(
		context.Background(),
		&ListArtifactScanJobsRequest{
			ThreadID:   10,
			RunID:      &runID,
			ArtifactID: &artifactID,
			Status:     " failed ",
			Scanner:    " clamav ",
			Page:       0,
			PageSize:   500,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, jobs, 1)
	require.Equal(t, int64(10), artifacts.listScanJobsReq.ThreadID)
	require.Equal(t, runID, *artifacts.listScanJobsReq.RunID)
	require.Equal(t, artifactID, *artifacts.listScanJobsReq.ArtifactID)
	require.Equal(
		t,
		entity.ArtifactScanJobStatusFailed,
		*artifacts.listScanJobsReq.Status,
	)
	require.Equal(t, "clamav", artifacts.listScanJobsReq.Scanner)
	require.Equal(t, int64(1), artifacts.listScanJobsReq.Page)
	require.Equal(t, int64(100), artifacts.listScanJobsReq.PageSize)
}

func TestArtifactServiceListScanJobsRejectsUnknownStatus(t *testing.T) {
	artifacts := &recordingArtifactRepository{}
	svc := NewArtifactService(&ArtifactComponents{
		ArtifactRepo: artifacts,
	})

	jobs, total, err := svc.ListArtifactScanJobs(
		context.Background(),
		&ListArtifactScanJobsRequest{
			ThreadID: 10,
			Status:   "quarantined",
		},
	)

	require.Error(t, err)
	require.Nil(t, jobs)
	require.Zero(t, total)
	require.Nil(t, artifacts.listScanJobsReq.Status)
}

func TestArtifactPreviewModeFromContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        entity.AgentArtifactPreviewMode
	}{
		{name: "plain text", contentType: "text/plain", want: entity.AgentArtifactPreviewModeText},
		{name: "json", contentType: "application/json", want: entity.AgentArtifactPreviewModeText},
		{name: "png", contentType: "image/png", want: entity.AgentArtifactPreviewModeImage},
		{name: "pdf", contentType: "application/pdf", want: entity.AgentArtifactPreviewModePDF},
		{name: "svg download", contentType: "image/svg+xml", want: entity.AgentArtifactPreviewModeDownload},
		{name: "unknown download", contentType: "application/octet-stream", want: entity.AgentArtifactPreviewModeDownload},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(
				t,
				testCase.want,
				DetermineArtifactPreviewMode(testCase.contentType),
			)
		})
	}
}

func TestArtifactServiceRejectsInvalidFileScope(t *testing.T) {
	tests := []struct {
		name       string
		mutateReq  func(*RegisterArtifactRequest)
		mutateFile func(*entity.AgentFile)
	}{
		{
			name: "thread mismatch",
			mutateReq: func(req *RegisterArtifactRequest) {
				req.ThreadID = 11
			},
		},
		{
			name: "deleted file",
			mutateFile: func(file *entity.AgentFile) {
				file.Status = entity.AgentFileStatusDeleted
			},
		},
		{
			name: "upload file",
			mutateFile: func(file *entity.AgentFile) {
				file.FileKind = entity.AgentFileKindUpload
			},
		},
		{
			name: "missing metadata",
			mutateReq: func(req *RegisterArtifactRequest) {
				req.Metadata = "{"
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			file := artifactServiceTestFile()
			if testCase.mutateFile != nil {
				testCase.mutateFile(file)
			}
			files := &recordingArtifactFileReader{file: file}
			artifacts := &recordingArtifactRepository{}
			svc := NewArtifactService(&ArtifactComponents{
				FileReader:   files,
				ArtifactRepo: artifacts,
				IDGen:        &sequenceIDGen{next: 100},
			})
			req := &RegisterArtifactRequest{
				SpaceID:      30,
				ThreadID:     10,
				RunID:        20,
				FileID:       90,
				ArtifactType: "report",
				Metadata:     `{}`,
			}
			if testCase.mutateReq != nil {
				testCase.mutateReq(req)
			}

			artifact, created, err := svc.RegisterArtifact(
				context.Background(),
				req,
			)

			require.Error(t, err)
			require.Nil(t, artifact)
			require.False(t, created)
			require.Nil(t, artifacts.upserted)
		})
	}
}

func artifactServiceTestFile() *entity.AgentFile {
	return &entity.AgentFile{
		ID:               90,
		SpaceID:          30,
		UserID:           40,
		ThreadID:         10,
		RunID:            20,
		FileName:         "report.txt",
		OriginalFileName: "report.txt",
		FileKind:         entity.AgentFileKindOutput,
		VirtualPath:      "/mnt/user-data/outputs/report.txt",
		ObjectURI:        "agent-runtime/30/10/runs/20/outputs/report.txt",
		ContentType:      "text/plain; charset=utf-8",
		SizeBytes:        128,
		Digest:           runtimeFileTestDigest,
		Status:           entity.AgentFileStatusActive,
		Metadata:         `{}`,
		CreatedAt:        1000,
		UpdatedAt:        1000,
	}
}

type recordingArtifactFileReader struct {
	file *entity.AgentFile
}

func (r *recordingArtifactFileReader) GetFileByID(
	context.Context,
	int64,
) (*entity.AgentFile, error) {
	if r.file == nil {
		return nil, nil
	}
	cloned := *r.file
	return &cloned, nil
}

type recordingArtifactRepository struct {
	upserted                *entity.AgentArtifact
	got                     *entity.AgentArtifact
	deleted                 *entity.AgentArtifact
	restored                *entity.AgentArtifact
	scanJob                 *entity.ArtifactScanJob
	claimedScanJobs         []*entity.ArtifactScanJob
	artifactScanBacklog     []*entity.ArtifactScanBacklogAggregate
	listScanJobs            []*entity.ArtifactScanJob
	listScanJobsTotal       int64
	cleanupCandidates       []*entity.AgentArtifact
	markFileDeletedOK       bool
	threadID                int64
	artifactID              int64
	deleteThreadID          int64
	deleteArtifactID        int64
	deletedAt               int64
	restoreThreadID         int64
	restoreArtifactID       int64
	restoredAt              int64
	updateMetadata          string
	updateScannedAt         int64
	claimScanJobsReq        repository.ClaimArtifactScanJobsRequest
	aggregateScanBacklogReq repository.AggregateArtifactScanBacklogRequest
	completeScanJobReq      repository.CompleteArtifactScanJobRequest
	retryScanJobReq         repository.RetryArtifactScanJobRequest
	requeueFailedScanJobReq repository.RequeueFailedArtifactScanJobRequest
	failScanJobReq          repository.FailArtifactScanJobRequest
	listScanJobsReq         repository.ListArtifactScanJobsRequest
	cleanupReq              repository.ListDeletedArtifactCleanupCandidatesRequest
	markFileDeletedReq      repository.MarkArtifactFileDeletedRequest
}

func (r *recordingArtifactRepository) UpsertArtifact(
	_ context.Context,
	artifact *entity.AgentArtifact,
) (*entity.AgentArtifact, bool, error) {
	cloned := *artifact
	r.upserted = &cloned
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) CreateOrGetArtifactScanJob(
	_ context.Context,
	job *entity.ArtifactScanJob,
) (*entity.ArtifactScanJob, bool, error) {
	cloned := *job
	r.scanJob = &cloned
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) ClaimArtifactScanJobs(
	_ context.Context,
	req repository.ClaimArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, error) {
	r.claimScanJobsReq = req
	return r.claimedScanJobs, nil
}

func (r *recordingArtifactRepository) AggregateArtifactScanBacklog(
	_ context.Context,
	req repository.AggregateArtifactScanBacklogRequest,
) ([]*entity.ArtifactScanBacklogAggregate, error) {
	r.aggregateScanBacklogReq = req
	return r.artifactScanBacklog, nil
}

func (r *recordingArtifactRepository) GetArtifactScanJob(
	_ context.Context,
	jobID int64,
) (*entity.ArtifactScanJob, error) {
	if r.scanJob == nil || r.scanJob.ID != jobID {
		return nil, nil
	}
	cloned := *r.scanJob
	return &cloned, nil
}

func (r *recordingArtifactRepository) CompleteArtifactScanJob(
	_ context.Context,
	req repository.CompleteArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	r.completeScanJobReq = req
	if r.scanJob == nil {
		return nil, false, nil
	}
	cloned := *r.scanJob
	cloned.Status = entity.ArtifactScanJobStatusSucceeded
	cloned.WorkerID = req.WorkerID
	cloned.EndedAt = req.Now
	cloned.UpdatedAt = req.Now
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) FailArtifactScanJob(
	_ context.Context,
	req repository.FailArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	r.failScanJobReq = req
	if r.scanJob == nil {
		return nil, false, nil
	}
	cloned := *r.scanJob
	cloned.Status = entity.ArtifactScanJobStatusFailed
	cloned.WorkerID = req.WorkerID
	cloned.LastError = req.ErrorText
	cloned.EndedAt = req.Now
	cloned.UpdatedAt = req.Now
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) RetryArtifactScanJob(
	_ context.Context,
	req repository.RetryArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	r.retryScanJobReq = req
	if r.scanJob == nil {
		return nil, false, nil
	}
	cloned := *r.scanJob
	cloned.Status = entity.ArtifactScanJobStatusPending
	cloned.WorkerID = ""
	cloned.LastError = req.ErrorText
	cloned.AvailableAt = req.AvailableAt
	cloned.LeaseExpiresAt = 0
	cloned.UpdatedAt = req.Now
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) RequeueFailedArtifactScanJob(
	_ context.Context,
	req repository.RequeueFailedArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	r.requeueFailedScanJobReq = req
	if r.scanJob == nil {
		return nil, false, nil
	}
	cloned := *r.scanJob
	cloned.Status = entity.ArtifactScanJobStatusPending
	cloned.WorkerID = ""
	cloned.LastError = req.ErrorText
	cloned.AvailableAt = req.AvailableAt
	cloned.LeaseExpiresAt = 0
	cloned.StartedAt = 0
	cloned.EndedAt = 0
	cloned.UpdatedAt = req.Now
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) ListArtifactScanJobs(
	_ context.Context,
	req repository.ListArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, int64, error) {
	r.listScanJobsReq = req
	return r.listScanJobs, r.listScanJobsTotal, nil
}

func (r *recordingArtifactRepository) ListArtifacts(
	context.Context,
	repository.ListArtifactsRequest,
) ([]*entity.AgentArtifact, int64, error) {
	return nil, 0, nil
}

func (r *recordingArtifactRepository) ListDeletedArtifactCleanupCandidates(
	_ context.Context,
	req repository.ListDeletedArtifactCleanupCandidatesRequest,
) ([]*entity.AgentArtifact, error) {
	r.cleanupReq = req
	return r.cleanupCandidates, nil
}

func (r *recordingArtifactRepository) MarkArtifactFileDeleted(
	_ context.Context,
	req repository.MarkArtifactFileDeletedRequest,
) (bool, error) {
	r.markFileDeletedReq = req
	return r.markFileDeletedOK, nil
}

func (r *recordingArtifactRepository) GetArtifact(
	_ context.Context,
	threadID int64,
	artifactID int64,
) (*entity.AgentArtifact, error) {
	r.threadID = threadID
	r.artifactID = artifactID
	if r.got == nil {
		return nil, nil
	}
	cloned := *r.got
	return &cloned, nil
}

func (r *recordingArtifactRepository) DeleteArtifact(
	_ context.Context,
	threadID int64,
	artifactID int64,
	deletedAt int64,
) (*entity.AgentArtifact, bool, error) {
	r.deleteThreadID = threadID
	r.deleteArtifactID = artifactID
	r.deletedAt = deletedAt
	if r.deleted == nil {
		return nil, false, nil
	}
	cloned := *r.deleted
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) RestoreArtifact(
	_ context.Context,
	threadID int64,
	artifactID int64,
	restoredAt int64,
) (*entity.AgentArtifact, bool, error) {
	r.restoreThreadID = threadID
	r.restoreArtifactID = artifactID
	r.restoredAt = restoredAt
	if r.restored == nil {
		return nil, false, nil
	}
	cloned := *r.restored
	return &cloned, true, nil
}

func (r *recordingArtifactRepository) UpdateArtifactScanMetadata(
	_ context.Context,
	threadID int64,
	artifactID int64,
	metadata string,
	updatedAt int64,
) (*entity.AgentArtifact, bool, error) {
	r.threadID = threadID
	r.artifactID = artifactID
	r.updateMetadata = metadata
	r.updateScannedAt = updatedAt
	if r.got == nil {
		return nil, false, nil
	}
	cloned := *r.got
	cloned.Metadata = metadata
	cloned.UpdatedAt = updatedAt
	return &cloned, true, nil
}
