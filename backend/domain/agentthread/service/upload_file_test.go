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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const uploadFileTestDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestUploadFileServiceRegistersOwnedThreadUpload(t *testing.T) {
	repo := &recordingUploadFileRepo{
		thread: &entity.Thread{
			ID:        10,
			SpaceID:   30,
			CreatorID: 40,
		},
	}
	svc := NewUploadFileService(&UploadFileComponents{
		ThreadReader: repo,
		FileRepo:     repo,
		IDGen:        &sequenceIDGen{next: 50},
	})

	file, err := svc.RegisterUploadFile(
		context.Background(),
		&RegisterUploadFileRequest{
			SpaceID:          30,
			UserID:           40,
			ThreadID:         10,
			FileName:         "report.md",
			OriginalFileName: "report.md",
			ContentType:      "text/markdown; charset=utf-8",
			SizeBytes:        128,
			Digest:           uploadFileTestDigest,
			Metadata:         `{"source":"composer"}`,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(50), file.ID)
	require.Equal(t, int64(30), file.SpaceID)
	require.Equal(t, int64(40), file.UserID)
	require.Equal(t, int64(10), file.ThreadID)
	require.Equal(t, int64(0), file.RunID)
	require.Equal(t, entity.AgentFileKindUpload, file.FileKind)
	require.Equal(t, "report.md", file.FileName)
	require.Equal(t, "/mnt/user-data/uploads/report.md", file.VirtualPath)
	require.Equal(t, "agent-runtime/30/10/uploads/report.md", file.ObjectURI)
	require.Equal(t, entity.AgentFileStatusActive, file.Status)
	require.NotZero(t, file.CreatedAt)
	require.Equal(t, file.CreatedAt, file.UpdatedAt)
	require.Equal(t, file, repo.created)
}

func TestUploadFileServiceRejectsUnsafeOrUnownedUpload(t *testing.T) {
	tests := []struct {
		name   string
		thread *entity.Thread
		mutate func(*RegisterUploadFileRequest)
	}{
		{
			name: "thread missing",
		},
		{
			name: "space mismatch",
			thread: &entity.Thread{
				ID:        10,
				SpaceID:   31,
				CreatorID: 40,
			},
		},
		{
			name: "user mismatch",
			thread: &entity.Thread{
				ID:        10,
				SpaceID:   30,
				CreatorID: 41,
			},
		},
		{
			name: "path traversal",
			thread: &entity.Thread{
				ID:        10,
				SpaceID:   30,
				CreatorID: 40,
			},
			mutate: func(req *RegisterUploadFileRequest) {
				req.FileName = "../secret.txt"
			},
		},
		{
			name: "control char",
			thread: &entity.Thread{
				ID:        10,
				SpaceID:   30,
				CreatorID: 40,
			},
			mutate: func(req *RegisterUploadFileRequest) {
				req.FileName = "bad\nname.txt"
			},
		},
		{
			name: "uppercase digest",
			thread: &entity.Thread{
				ID:        10,
				SpaceID:   30,
				CreatorID: 40,
			},
			mutate: func(req *RegisterUploadFileRequest) {
				req.Digest = strings.Repeat("A", 64)
			},
		},
		{
			name: "duplicate active path",
			thread: &entity.Thread{
				ID:        10,
				SpaceID:   30,
				CreatorID: 40,
			},
			mutate: func(req *RegisterUploadFileRequest) {},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &recordingUploadFileRepo{thread: testCase.thread}
			if testCase.name == "duplicate active path" {
				repo.runtimeFile = &entity.AgentFile{
					ID:          99,
					SpaceID:     30,
					UserID:      40,
					ThreadID:    10,
					RunID:       0,
					FileName:    "report.md",
					FileKind:    entity.AgentFileKindUpload,
					VirtualPath: "/mnt/user-data/uploads/report.md",
					ObjectURI:   "agent-runtime/30/10/uploads/report.md",
					ContentType: "text/markdown; charset=utf-8",
					SizeBytes:   128,
					Digest:      uploadFileTestDigest,
					Status:      entity.AgentFileStatusActive,
					Metadata:    `{}`,
				}
			}
			svc := NewUploadFileService(&UploadFileComponents{
				ThreadReader: repo,
				FileRepo:     repo,
				IDGen:        &sequenceIDGen{next: 50},
			})
			req := &RegisterUploadFileRequest{
				SpaceID:     30,
				UserID:      40,
				ThreadID:    10,
				FileName:    "report.md",
				ContentType: "text/markdown; charset=utf-8",
				SizeBytes:   128,
				Digest:      uploadFileTestDigest,
				Metadata:    `{}`,
			}
			if testCase.mutate != nil {
				testCase.mutate(req)
			}

			file, err := svc.RegisterUploadFile(context.Background(), req)

			require.Error(t, err)
			require.Nil(t, file)
			require.Nil(t, repo.created)
		})
	}
}

func TestUploadFileServiceListsAndDeletesOwnedUploads(t *testing.T) {
	expected := []*entity.AgentFile{
		{
			ID:          90,
			SpaceID:     30,
			UserID:      40,
			ThreadID:    10,
			RunID:       0,
			FileName:    "report.md",
			FileKind:    entity.AgentFileKindUpload,
			VirtualPath: "/mnt/user-data/uploads/report.md",
			ObjectURI:   "agent-runtime/30/10/uploads/report.md",
			ContentType: "text/markdown; charset=utf-8",
			SizeBytes:   128,
			Digest:      uploadFileTestDigest,
			Status:      entity.AgentFileStatusActive,
			Metadata:    `{}`,
			CreatedAt:   100,
			UpdatedAt:   100,
		},
	}
	repo := &recordingUploadFileRepo{
		thread: &entity.Thread{
			ID:        10,
			SpaceID:   30,
			CreatorID: 40,
		},
		uploads: expected,
	}
	svc := NewUploadFileService(&UploadFileComponents{
		ThreadReader: repo,
		FileRepo:     repo,
		IDGen:        &sequenceIDGen{next: 50},
	})

	files, err := svc.ListUploadFiles(context.Background(), &ListUploadFilesRequest{
		SpaceID:  30,
		UserID:   40,
		ThreadID: 10,
	})
	require.NoError(t, err)
	require.Equal(t, expected, files)
	require.Equal(t, repository.ListThreadUploadFilesRequest{
		SpaceID:  30,
		UserID:   40,
		ThreadID: 10,
	}, repo.listReq)

	deleted, ok, err := svc.DeleteUploadFile(context.Background(), &DeleteUploadFileRequest{
		SpaceID:   30,
		UserID:    40,
		ThreadID:  10,
		FileName:  "report.md",
		DeletedAt: 200,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, expected[0], deleted)
	require.Equal(t, repository.MarkThreadUploadFileDeletedRequest{
		SpaceID:   30,
		UserID:    40,
		ThreadID:  10,
		FileName:  "report.md",
		DeletedAt: 200,
	}, repo.deleteReq)
}

type recordingUploadFileRepo struct {
	thread      *entity.Thread
	runtimeFile *entity.AgentFile
	created     *entity.AgentFile
	uploads     []*entity.AgentFile
	listReq     repository.ListThreadUploadFilesRequest
	deleteReq   repository.MarkThreadUploadFileDeletedRequest
}

func (r *recordingUploadFileRepo) GetThread(
	_ context.Context,
	_ int64,
) (*entity.Thread, error) {
	if r.thread == nil {
		return nil, nil
	}
	cloned := *r.thread
	return &cloned, nil
}

func (r *recordingUploadFileRepo) CreateRuntimeFile(
	_ context.Context,
	file *entity.AgentFile,
) error {
	cloned := *file
	r.created = &cloned
	return nil
}

func (r *recordingUploadFileRepo) GetRuntimeFile(
	_ context.Context,
	_ int64,
	_ string,
) (*entity.AgentFile, error) {
	if r.runtimeFile == nil {
		return nil, nil
	}
	cloned := *r.runtimeFile
	return &cloned, nil
}

func (r *recordingUploadFileRepo) ListThreadUploadFiles(
	_ context.Context,
	req repository.ListThreadUploadFilesRequest,
) ([]*entity.AgentFile, error) {
	r.listReq = req
	return r.uploads, nil
}

func (r *recordingUploadFileRepo) MarkThreadUploadFileDeleted(
	_ context.Context,
	req repository.MarkThreadUploadFileDeletedRequest,
) (*entity.AgentFile, bool, error) {
	r.deleteReq = req
	if len(r.uploads) == 0 {
		return nil, false, nil
	}
	return r.uploads[0], true, nil
}
