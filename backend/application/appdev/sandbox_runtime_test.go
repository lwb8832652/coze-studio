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

package appdev

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type appDevSandboxRuntimeStore struct {
	domainappdev.Store
	archive   []byte
	sourceURL string
}

func (s *appDevSandboxRuntimeStore) GetProject(context.Context, string, string) (*domainappdev.Project, error) {
	return &domainappdev.Project{ID: "project-1", SpaceID: "1001"}, nil
}

func (s *appDevSandboxRuntimeStore) ProjectFilesDir(string, string) (string, error) {
	return "/Users/operator/private/appdev/project-1", nil
}

func (s *appDevSandboxRuntimeStore) ProjectSourceURL(context.Context, string, string) (string, error) {
	return s.sourceURL, nil
}

func (s *appDevSandboxRuntimeStore) ExportProjectArchive(context.Context, string, string) ([]byte, string, error) {
	return append([]byte(nil), s.archive...), "project-1.zip", nil
}

func (s *appDevSandboxRuntimeStore) UpdateProjectRuntime(
	context.Context,
	string,
	string,
	domainappdev.RuntimeStatus,
	string,
) (*domainappdev.Project, error) {
	return &domainappdev.Project{}, nil
}

type appDevSandboxRuntimeCapture struct {
	RuntimeManager
	request *RuntimeManagerRequest
}

func (*appDevSandboxRuntimeCapture) RequiresSourceSnapshot() bool { return true }

func (m *appDevSandboxRuntimeCapture) Start(
	_ context.Context,
	req *RuntimeManagerRequest,
) (*domainappdev.RuntimeInfo, error) {
	copyRequest := *req
	if req.Snapshot != nil {
		copySnapshot := *req.Snapshot
		copyRequest.Snapshot = &copySnapshot
	}
	m.request = &copyRequest
	return &domainappdev.RuntimeInfo{
		Status:     domainappdev.RuntimeStatusRunning,
		PreviewURL: "https://preview.example.com/apps/runtime-1/",
	}, nil
}

func TestAppDevStartBuildsBoundedSandboxSnapshotWithoutHostPath(t *testing.T) {
	archive := []byte("safe deterministic project archive")
	store := &appDevSandboxRuntimeStore{
		archive:   archive,
		sourceURL: "https://objects.example.com/project-1.zip?signature=opaque",
	}
	runtime := &appDevSandboxRuntimeCapture{}
	service := NewService(store, runtime)

	_, err := service.StartRuntime(context.Background(), &RuntimeRequest{
		SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1",
	})
	require.NoError(t, err)
	require.NotNil(t, runtime.request)
	require.Empty(t, runtime.request.ProjectDir)
	require.Empty(t, runtime.request.SourceURL)
	require.NotNil(t, runtime.request.Snapshot)
	require.Equal(t, "source.zip", runtime.request.Snapshot.Path)
	require.Equal(t, int64(len(archive)), runtime.request.Snapshot.Size)
	digest := sha256.Sum256(archive)
	require.Equal(t, "sha256:"+hex.EncodeToString(digest[:]), runtime.request.Snapshot.Digest)
	require.Equal(t, store.sourceURL, runtime.request.Snapshot.DownloadURL)
	require.NotContains(t, runtime.request.Snapshot.ID, "/")
}

func TestAppDevStartRejectsUnsafeSandboxSnapshotURLBeforeRuntime(t *testing.T) {
	store := &appDevSandboxRuntimeStore{
		archive:   []byte("archive"),
		sourceURL: "http://127.0.0.1/private.zip",
	}
	runtime := &appDevSandboxRuntimeCapture{}
	service := NewService(store, runtime)

	_, err := service.StartRuntime(context.Background(), &RuntimeRequest{
		SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1",
	})
	require.Error(t, err)
	require.Nil(t, runtime.request)
}
