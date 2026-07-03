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

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestApplicationUploadTaskThreadFilesStoresObjectAndReturnsBoundedMetadata(t *testing.T) {
	uploads := &recordingUploadFileService{
		listed: []*entity.AgentFile{
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
				Digest:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Status:      entity.AgentFileStatusActive,
				Metadata:    `{}`,
				CreatedAt:   100,
				UpdatedAt:   100,
			},
		},
		registered: &entity.AgentFile{
			ID:          91,
			SpaceID:     30,
			UserID:      40,
			ThreadID:    10,
			RunID:       0,
			FileName:    "report-1.md",
			FileKind:    entity.AgentFileKindUpload,
			VirtualPath: "/mnt/user-data/uploads/report-1.md",
			ObjectURI:   "agent-runtime/30/10/uploads/report-1.md",
			ContentType: "text/markdown; charset=utf-8",
			SizeBytes:   9,
			Digest:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			Status:      entity.AgentFileStatusActive,
			Metadata:    `{"source":"workbench_composer"}`,
			CreatedAt:   200,
			UpdatedAt:   200,
		},
	}
	storage := &recordingArtifactObjectReader{}
	app := &ApplicationService{
		UploadFileSVC:         uploads,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.UploadTaskThreadFiles(
		context.Background(),
		&UploadTaskThreadFilesRequest{
			SpaceID:  30,
			UserID:   40,
			ThreadID: 10,
			Files: []TaskThreadUploadFileInput{
				{
					FileName:    "report.md",
					Content:     []byte("# report"),
					ContentType: "text/markdown; charset=utf-8",
				},
			},
		},
	)

	require.NoError(t, err)
	require.Len(t, resp.Files, 1)
	require.Empty(t, resp.SkippedFiles)
	require.Equal(t, "report-1.md", uploads.registerReq.FileName)
	require.Equal(t, "agent-runtime/30/10/uploads/report-1.md", storage.key)
	require.Equal(t, []byte("# report"), storage.objects[storage.key])
	require.Equal(t, int64(91), resp.Files[0].FileID)
	require.Equal(t, "report-1.md", resp.Files[0].FileName)
	require.Equal(t, "/mnt/user-data/uploads/report-1.md", resp.Files[0].VirtualPath)
	require.Equal(t, int64(9), resp.Files[0].SizeBytes)
}

func TestApplicationListAndDeleteTaskThreadUploadFiles(t *testing.T) {
	expected := &entity.AgentFile{
		ID:          91,
		SpaceID:     30,
		UserID:      40,
		ThreadID:    10,
		RunID:       0,
		FileName:    "report.md",
		FileKind:    entity.AgentFileKindUpload,
		VirtualPath: "/mnt/user-data/uploads/report.md",
		ObjectURI:   "agent-runtime/30/10/uploads/report.md",
		ContentType: "text/markdown; charset=utf-8",
		SizeBytes:   9,
		Digest:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Status:      entity.AgentFileStatusActive,
		Metadata:    `{}`,
		CreatedAt:   200,
		UpdatedAt:   200,
	}
	uploads := &recordingUploadFileService{
		listed:  []*entity.AgentFile{expected},
		deleted: expected,
	}
	app := &ApplicationService{UploadFileSVC: uploads}

	listResp, err := app.ListTaskThreadUploadFiles(
		context.Background(),
		&ListTaskThreadUploadFilesRequest{
			SpaceID:  30,
			UserID:   40,
			ThreadID: 10,
		},
	)
	require.NoError(t, err)
	require.Len(t, listResp.Files, 1)
	require.Equal(t, "report.md", listResp.Files[0].FileName)
	require.Equal(t, "/mnt/user-data/uploads/report.md", listResp.Files[0].VirtualPath)

	deleteResp, err := app.DeleteTaskThreadUploadFile(
		context.Background(),
		&DeleteTaskThreadUploadFileRequest{
			SpaceID:  30,
			UserID:   40,
			ThreadID: 10,
			FileName: "report.md",
		},
	)
	require.NoError(t, err)
	require.True(t, deleteResp.Deleted)
	require.Equal(t, "report.md", deleteResp.File.FileName)
	require.Equal(t, "report.md", uploads.deleteReq.FileName)
}

type recordingUploadFileService struct {
	listed      []*entity.AgentFile
	registered  *entity.AgentFile
	deleted     *entity.AgentFile
	registerReq *domainservice.RegisterUploadFileRequest
	listReq     *domainservice.ListUploadFilesRequest
	deleteReq   *domainservice.DeleteUploadFileRequest
}

func (s *recordingUploadFileService) RegisterUploadFile(
	_ context.Context,
	req *domainservice.RegisterUploadFileRequest,
) (*entity.AgentFile, error) {
	s.registerReq = req
	if s.registered == nil {
		return nil, nil
	}
	cloned := *s.registered
	return &cloned, nil
}

func (s *recordingUploadFileService) ListUploadFiles(
	_ context.Context,
	req *domainservice.ListUploadFilesRequest,
) ([]*entity.AgentFile, error) {
	s.listReq = req
	return s.listed, nil
}

func (s *recordingUploadFileService) DeleteUploadFile(
	_ context.Context,
	req *domainservice.DeleteUploadFileRequest,
) (*entity.AgentFile, bool, error) {
	s.deleteReq = req
	if s.deleted == nil {
		return nil, false, nil
	}
	cloned := *s.deleted
	return &cloned, true, nil
}
