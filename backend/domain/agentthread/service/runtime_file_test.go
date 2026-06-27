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
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const runtimeFileTestDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func runtimeFileTestName() string {
	return runtimeFileTestDigest + ".txt"
}

func runtimeFileTestPath(runID string, phase string, fileName string) string {
	return "/mnt/user-data/workspace/.coze/tool-results/runs/" +
		runID + "/" + phase + "/" + fileName
}

func runtimeFileTestObjectURI(
	spaceID string,
	threadID string,
	runID string,
	phase string,
	fileName string,
) string {
	return "agent-runtime/" + spaceID + "/" + threadID +
		"/runs/" + runID + "/tool-results/" + phase + "/" + fileName
}

func TestRuntimeFileServiceRegistersOwnedWorkspaceOffload(t *testing.T) {
	repo := &recordingRuntimeFileRepo{
		run: &entity.Run{
			ID:        20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
	}
	svc := NewRuntimeFileService(&RuntimeFileComponents{
		RunReader: repo,
		FileRepo:  repo,
		IDGen:     &sequenceIDGen{next: 50},
	})

	file, created, err := svc.RegisterRuntimeFile(
		context.Background(),
		&RegisterRuntimeFileRequest{
			RunID:       20,
			FileName:    runtimeFileTestName(),
			FileKind:    entity.AgentFileKindWorkspace,
			VirtualPath: runtimeFileTestPath("20", "trunc", runtimeFileTestName()),
			ObjectURI: runtimeFileTestObjectURI(
				"30",
				"10",
				"20",
				"trunc",
				runtimeFileTestName(),
			),
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   128,
			Digest:      runtimeFileTestDigest,
			Metadata:    `{"purpose":"tool_result_offload","phase":"trunc"}`,
		},
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(50), file.ID)
	require.Equal(t, int64(10), file.ThreadID)
	require.Equal(t, int64(30), file.SpaceID)
	require.Equal(t, int64(40), file.UserID)
	require.Equal(t, entity.AgentFileStatusActive, file.Status)
	require.NotZero(t, file.CreatedAt)
	require.Equal(t, file.CreatedAt, file.UpdatedAt)
	require.Equal(t, file, repo.upserted)
}

func runtimeFileTestEntity() *entity.AgentFile {
	return &entity.AgentFile{
		ID:          99,
		SpaceID:     30,
		UserID:      40,
		ThreadID:    10,
		RunID:       20,
		FileName:    runtimeFileTestName(),
		FileKind:    entity.AgentFileKindWorkspace,
		VirtualPath: runtimeFileTestPath("20", "trunc", runtimeFileTestName()),
		ObjectURI: runtimeFileTestObjectURI(
			"30",
			"10",
			"20",
			"trunc",
			runtimeFileTestName(),
		),
		ContentType: "text/plain; charset=utf-8",
		SizeBytes:   128,
		Digest:      runtimeFileTestDigest,
		Status:      entity.AgentFileStatusActive,
		Metadata:    `{"purpose":"tool_result_offload"}`,
		CreatedAt:   100,
		UpdatedAt:   100,
	}
}

func TestRuntimeFileServiceResolvesOwnedWorkspaceOffload(t *testing.T) {
	repo := &recordingRuntimeFileRepo{runtimeFile: runtimeFileTestEntity()}
	svc := NewRuntimeFileService(&RuntimeFileComponents{
		FileRepo: repo,
	})

	file, err := svc.ResolveRuntimeFile(
		context.Background(),
		&ResolveRuntimeFileRequest{
			SpaceID:     30,
			ThreadID:    10,
			RunID:       20,
			VirtualPath: runtimeFileTestPath("20", "trunc", runtimeFileTestName()),
		},
	)

	require.NoError(t, err)
	require.Equal(t, repo.runtimeFile, file)
}

func TestRuntimeFileServiceRejectsInvalidRuntimeFileResolve(t *testing.T) {
	tests := []struct {
		name       string
		file       *entity.AgentFile
		mutateReq  func(*ResolveRuntimeFileRequest)
		mutateFile func(*entity.AgentFile)
	}{
		{
			name: "missing file",
			file: nil,
		},
		{
			name: "request scope mismatch",
			mutateReq: func(req *ResolveRuntimeFileRequest) {
				req.ThreadID = 11
			},
		},
		{
			name: "wrong kind",
			mutateFile: func(file *entity.AgentFile) {
				file.FileKind = entity.AgentFileKindUpload
			},
		},
		{
			name: "deleted status",
			mutateFile: func(file *entity.AgentFile) {
				file.Status = entity.AgentFileStatusDeleted
			},
		},
		{
			name: "file name mismatch",
			mutateFile: func(file *entity.AgentFile) {
				file.FileName = strings.Repeat("b", 64) + ".txt"
			},
		},
		{
			name: "object phase mismatch",
			mutateFile: func(file *entity.AgentFile) {
				file.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"clear",
					runtimeFileTestName(),
				)
			},
		},
		{
			name: "invalid persisted digest",
			mutateFile: func(file *entity.AgentFile) {
				file.Digest = strings.Repeat("A", 64)
			},
		},
		{
			name: "invalid persisted metadata",
			mutateFile: func(file *entity.AgentFile) {
				file.Metadata = "{"
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			file := testCase.file
			if file == nil && testCase.name != "missing file" {
				file = runtimeFileTestEntity()
			}
			if testCase.file == nil &&
				testCase.name == "missing file" {
				file = nil
			}
			if file != nil && testCase.mutateFile != nil {
				testCase.mutateFile(file)
			}
			repo := &recordingRuntimeFileRepo{runtimeFile: file}
			svc := NewRuntimeFileService(&RuntimeFileComponents{
				FileRepo: repo,
			})
			req := &ResolveRuntimeFileRequest{
				SpaceID:     30,
				ThreadID:    10,
				RunID:       20,
				VirtualPath: runtimeFileTestPath("20", "trunc", runtimeFileTestName()),
			}
			if testCase.mutateReq != nil {
				testCase.mutateReq(req)
			}

			resolved, err := svc.ResolveRuntimeFile(
				context.Background(),
				req,
			)

			require.Error(t, err)
			require.Nil(t, resolved)
		})
	}
}

func TestRuntimeFileServiceRejectsInvalidWorkspaceOffload(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RegisterRuntimeFileRequest)
	}{
		{
			name: "wrong kind",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.FileKind = entity.AgentFileKindUpload
			},
		},
		{
			name: "outside prefix",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.VirtualPath = "/mnt/user-data/workspace/report.txt"
				req.FileName = path.Base(req.VirtualPath)
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"trunc",
					"report.txt",
				)
			},
		},
		{
			name: "path run mismatch",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.VirtualPath = runtimeFileTestPath("21", "trunc", runtimeFileTestName())
				req.FileName = path.Base(req.VirtualPath)
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"21",
					"trunc",
					req.FileName,
				)
			},
		},
		{
			name: "unsupported phase",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.VirtualPath = runtimeFileTestPath("20", "output", runtimeFileTestName())
				req.FileName = path.Base(req.VirtualPath)
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"output",
					req.FileName,
				)
			},
		},
		{
			name: "short file name",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.VirtualPath = runtimeFileTestPath("20", "trunc", "short.txt")
				req.FileName = path.Base(req.VirtualPath)
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"trunc",
					req.FileName,
				)
			},
		},
		{
			name: "uppercase path digest",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.VirtualPath = runtimeFileTestPath(
					"20",
					"trunc",
					strings.Repeat("A", 64)+".txt",
				)
				req.FileName = path.Base(req.VirtualPath)
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"trunc",
					req.FileName,
				)
			},
		},
		{
			name: "empty size",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.SizeBytes = 0
			},
		},
		{
			name: "uppercase digest",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.Digest = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
			},
		},
		{
			name: "object url",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = "https://storage.example.test/private"
			},
		},
		{
			name: "object space mismatch",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = runtimeFileTestObjectURI(
					"31",
					"10",
					"20",
					"trunc",
					runtimeFileTestName(),
				)
			},
		},
		{
			name: "object thread mismatch",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"11",
					"20",
					"trunc",
					runtimeFileTestName(),
				)
			},
		},
		{
			name: "object run mismatch",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"21",
					"trunc",
					runtimeFileTestName(),
				)
			},
		},
		{
			name: "object phase mismatch",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"clear",
					runtimeFileTestName(),
				)
			},
		},
		{
			name: "object digest mismatch",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"trunc",
					strings.Repeat("b", 64)+".txt",
				)
			},
		},
		{
			name: "object backslash",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.ObjectURI = "agent-runtime/30/10/runs/20/tool-results/trunc\\" +
					runtimeFileTestName()
			},
		},
		{
			name: "invalid metadata",
			mutate: func(req *RegisterRuntimeFileRequest) {
				req.Metadata = "{"
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &recordingRuntimeFileRepo{
				run: &entity.Run{
					ID:        20,
					ThreadID:  10,
					SpaceID:   30,
					CreatorID: 40,
				},
			}
			svc := NewRuntimeFileService(&RuntimeFileComponents{
				RunReader: repo,
				FileRepo:  repo,
				IDGen:     &sequenceIDGen{next: 50},
			})
			req := &RegisterRuntimeFileRequest{
				RunID:       20,
				FileName:    runtimeFileTestName(),
				FileKind:    entity.AgentFileKindWorkspace,
				VirtualPath: runtimeFileTestPath("20", "trunc", runtimeFileTestName()),
				ObjectURI: runtimeFileTestObjectURI(
					"30",
					"10",
					"20",
					"trunc",
					runtimeFileTestName(),
				),
				ContentType: "text/plain; charset=utf-8",
				SizeBytes:   128,
				Digest:      runtimeFileTestDigest,
				Metadata:    `{"purpose":"tool_result_offload"}`,
			}
			testCase.mutate(req)

			file, created, err := svc.RegisterRuntimeFile(
				context.Background(),
				req,
			)

			require.Error(t, err)
			require.Nil(t, file)
			require.False(t, created)
			require.Nil(t, repo.upserted)
		})
	}
}

type recordingRuntimeFileRepo struct {
	run         *entity.Run
	runtimeFile *entity.AgentFile
	upserted    *entity.AgentFile
}

func (r *recordingRuntimeFileRepo) GetRun(
	context.Context,
	int64,
) (*entity.Run, error) {
	return r.run, nil
}

func (r *recordingRuntimeFileRepo) UpsertRuntimeFile(
	_ context.Context,
	file *entity.AgentFile,
) (*entity.AgentFile, bool, error) {
	cloned := *file
	r.upserted = &cloned
	return &cloned, true, nil
}

func (r *recordingRuntimeFileRepo) GetRuntimeFile(
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

func (r *recordingRuntimeFileRepo) GetFileByID(
	_ context.Context,
	_ int64,
) (*entity.AgentFile, error) {
	if r.runtimeFile == nil {
		return nil, nil
	}
	cloned := *r.runtimeFile
	return &cloned, nil
}
