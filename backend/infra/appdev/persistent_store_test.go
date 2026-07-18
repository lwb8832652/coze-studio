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
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestPersistentStoreRestoresLatestSourceAcrossInstances(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:appdev-persistent?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&appDevProjectRecord{},
		&appDevSnapshotRecord{},
		&appDevRuntimeRecord{},
	))

	ctx := context.Background()
	objects := newMemoryObjectStorage()
	storeA := NewPersistentStoreForTest(db, objects, t.TempDir())
	project := &domainappdev.Project{
		ID:        "project-1",
		SpaceID:   "1001",
		Name:      "Persistent app",
		Status:    domainappdev.ProjectStatusReady,
		CreatorID: "42",
	}
	_, err = storeA.CreateProject(ctx, project, map[string]string{
		"src/App.tsx": "export default function App() { return 'v1'; }",
	})
	require.NoError(t, err)

	_, err = storeA.SaveFileContent(
		ctx,
		project.SpaceID,
		project.ID,
		"src/App.tsx",
		"export default function App() { return 'v2'; }",
	)
	require.NoError(t, err)
	snapshot, err := storeA.CreateProjectSnapshot(ctx, project.SpaceID, project.ID, "v2")
	require.NoError(t, err)

	_, err = storeA.SaveFileContent(
		ctx,
		project.SpaceID,
		project.ID,
		"src/App.tsx",
		"export default function App() { return 'v3'; }",
	)
	require.NoError(t, err)

	storeB := NewPersistentStoreForTest(db, objects, t.TempDir())
	latest, err := storeB.GetFileContent(ctx, project.SpaceID, project.ID, "src/App.tsx")
	require.NoError(t, err)
	require.Contains(t, latest.Content, "'v3'")

	require.NoError(t, storeB.RestoreProjectSnapshot(
		ctx,
		project.SpaceID,
		project.ID,
		snapshot.ID,
	))

	storeC := NewPersistentStoreForTest(db, objects, t.TempDir())
	restored, err := storeC.GetFileContent(ctx, project.SpaceID, project.ID, "src/App.tsx")
	require.NoError(t, err)
	require.Contains(t, restored.Content, "'v2'")

	var record appDevProjectRecord
	require.NoError(t, db.Where("id = ? AND space_id = ?", project.ID, 1001).Take(&record).Error)
	require.Equal(t, int64(4), record.SourceVersion)
	require.NotEmpty(t, record.SourceObjectKey)

	artifact := []byte("release archive")
	artifactKey, err := storeC.SaveBuildArtifact(ctx, project.SpaceID, project.ID, artifact)
	require.NoError(t, err)
	_, err = storeC.UpdateProjectBuild(
		ctx,
		project.SpaceID,
		project.ID,
		"success",
		"page",
		artifactKey,
		"success",
		time.Now().UTC().Format(time.RFC3339),
	)
	require.NoError(t, err)
	persistedArtifact, err := storeC.GetBuildArtifact(ctx, project.SpaceID, project.ID)
	require.NoError(t, err)
	require.Equal(t, artifact, persistedArtifact)
}

func TestPersistentStoreLoadsTenantScopedProviderRuntimeSourceArtifactByStreaming(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:appdev-provider-runtime-source?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&appDevProjectRecord{}, &appDevSnapshotRecord{}, &appDevRuntimeRecord{}))

	ctx := context.Background()
	objects := newMemoryObjectStorage()
	store := NewPersistentStoreForTest(db, objects, t.TempDir())
	project := &domainappdev.Project{
		ID: "source-project", SpaceID: "2001", Name: "Source", Status: domainappdev.ProjectStatusReady, CreatorID: "42",
	}
	_, err = store.CreateProject(ctx, project, map[string]string{"src/main.ts": "export const value = 1"})
	require.NoError(t, err)

	artifact, err := store.LoadProviderRuntimeSourceArtifact(ctx, project.SpaceID, project.ID)
	require.NoError(t, err)
	require.Greater(t, artifact.Size(), int64(0))
	require.True(t, strings.HasPrefix(artifact.Digest(), "sha256:"))
	require.Equal(t, artifact.Size(), objects.streamedBytes)
	require.NotEmpty(t, artifact.ObjectKey())
	formatted := fmt.Sprintf("%v|%+v|%#v", artifact, artifact, artifact)
	require.NotContains(t, formatted, artifact.ObjectKey())
	_, err = json.Marshal(artifact)
	require.Error(t, err)

	_, err = store.LoadProviderRuntimeSourceArtifact(ctx, "2002", project.ID)
	require.ErrorIs(t, err, domainappdev.ErrNotFound)
	_, err = store.LoadProviderRuntimeSourceArtifact(ctx, project.SpaceID, "other-project")
	require.ErrorIs(t, err, domainappdev.ErrNotFound)

	objects.mu.Lock()
	for key, content := range objects.objects {
		digest := sha256.Sum256(content)
		require.Equal(t, fmt.Sprintf("sha256:%x", digest[:]), artifact.Digest(), key)
	}
	objects.mu.Unlock()
}

type memoryObjectStorage struct {
	mu            sync.Mutex
	objects       map[string][]byte
	streamedBytes int64
}

func newMemoryObjectStorage() *memoryObjectStorage {
	return &memoryObjectStorage{objects: map[string][]byte{}}
}

func (s *memoryObjectStorage) PutObject(_ context.Context, key string, content []byte, _ ...storage.PutOptFn) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = append([]byte(nil), content...)
	return nil
}

func (s *memoryObjectStorage) PutObjectWithReader(_ context.Context, key string, _ io.Reader, _ ...storage.PutOptFn) error {
	return fmt.Errorf("unexpected reader upload for %s", key)
}

func (s *memoryObjectStorage) GetObject(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.objects[key]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return append([]byte(nil), content...), nil
}

func (s *memoryObjectStorage) OpenObjectStream(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.objects[key]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	s.streamedBytes = int64(len(content))
	return io.NopCloser(strings.NewReader(string(content))), nil
}

func (s *memoryObjectStorage) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *memoryObjectStorage) GetObjectUrl(_ context.Context, key string, _ ...storage.GetOptFn) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.objects[key]; !ok {
		return "", storage.ErrObjectNotFound
	}
	return "https://objects.example/" + key, nil
}

func (s *memoryObjectStorage) HeadObject(_ context.Context, key string, _ ...storage.GetOptFn) (*storage.FileInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.objects[key]
	if !ok {
		return nil, storage.ErrObjectNotFound
	}
	return &storage.FileInfo{Key: key, Size: int64(len(content))}, nil
}

func (s *memoryObjectStorage) ListAllObjects(_ context.Context, prefix string, _ ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files := make([]*storage.FileInfo, 0)
	for key, content := range s.objects {
		if strings.HasPrefix(key, prefix) {
			files = append(files, &storage.FileInfo{Key: key, Size: int64(len(content))})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Key < files[j].Key })
	return files, nil
}

func (s *memoryObjectStorage) ListObjectsPaginated(ctx context.Context, input *storage.ListObjectsPaginatedInput, opts ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	files, err := s.ListAllObjects(ctx, input.Prefix, opts...)
	if err != nil {
		return nil, err
	}
	return &storage.ListObjectsPaginatedOutput{
		Files:       files,
		IsTruncated: false,
		Cursor:      time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
