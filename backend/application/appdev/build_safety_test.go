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
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type buildSafetyStore struct {
	domainappdev.Store
	failUpdateAt int32
	updateCalls  atomic.Int32
}

func (s *buildSafetyStore) ProjectSourceURL(context.Context, string, string) (string, error) {
	return "https://example.test/source.zip", nil
}

func (s *buildSafetyStore) ProjectFilesDir(string, string) (string, error) {
	return "/tmp/appdev-build-safety", nil
}

func (s *buildSafetyStore) UpdateProjectBuild(
	context.Context,
	string,
	string,
	string,
	string,
	string,
	string,
	string,
) (*domainappdev.Project, error) {
	call := s.updateCalls.Add(1)
	if s.failUpdateAt > 0 && call == s.failUpdateAt {
		return nil, errors.New("forced build persistence failure")
	}
	return &domainappdev.Project{}, nil
}

type blockingBuildRuntime struct {
	RuntimeManager
	calls    atomic.Int32
	started  chan int32
	release  chan struct{}
	buildErr error
}

func (r *blockingBuildRuntime) Build(context.Context, *IsolatedBuildRequest) (*IsolatedBuildResult, error) {
	call := r.calls.Add(1)
	if r.started != nil {
		r.started <- call
	}
	if r.release != nil {
		<-r.release
	}
	if r.buildErr != nil {
		return nil, r.buildErr
	}
	now := time.Now().UTC()
	return &IsolatedBuildResult{
		Status:            "success",
		ArtifactObjectKey: "artifact.zip",
		StartedAt:         now,
		FinishedAt:        now,
	}, nil
}

func TestBuildProjectSerializesRemoteBuildsForTheSameProject(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")

	store := &buildSafetyStore{}
	runtime := &blockingBuildRuntime{
		started: make(chan int32, 2),
		release: make(chan struct{}),
	}
	service := NewService(store, runtime)
	req := &BuildProjectRequest{SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1", PublishType: "page"}
	results := make(chan error, 2)

	go func() {
		_, err := service.BuildProject(context.Background(), req)
		results <- err
	}()
	require.Equal(t, int32(1), <-runtime.started)

	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		_, err := service.BuildProject(context.Background(), req)
		results <- err
	}()
	<-secondStarted
	select {
	case call := <-runtime.started:
		t.Fatalf("second build started before the first completed: call %d", call)
	case <-time.After(50 * time.Millisecond):
	}

	runtime.release <- struct{}{}
	select {
	case call := <-runtime.started:
		require.Equal(t, int32(2), call)
	case <-time.After(time.Second):
		t.Fatal("second build did not start after the first completed")
	}
	runtime.release <- struct{}{}
	require.NoError(t, <-results)
	require.NoError(t, <-results)
}

func TestProjectBuildLocksDoNotBlockUnrelatedProjects(t *testing.T) {
	registry := newProjectBuildLockRegistry()
	releaseFirst := registry.lock("1001", "project-1")
	defer releaseFirst()

	acquired := make(chan struct{})
	go func() {
		releaseSecond := registry.lock("1001", "project-2")
		close(acquired)
		releaseSecond()
	}()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("an unrelated project was blocked by the active build")
	}
}

func TestBuildProjectDoesNotRunWhenBuildStartCannotPersist(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	store := &buildSafetyStore{failUpdateAt: 1}
	runtime := &blockingBuildRuntime{}
	service := NewService(store, runtime)

	_, err := service.BuildProject(context.Background(), &BuildProjectRequest{
		SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1", PublishType: "page",
	})

	require.ErrorContains(t, err, "persist appdev build start")
	require.Zero(t, runtime.calls.Load())
}

func TestBuildProjectReturnsFinalPersistenceFailure(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	store := &buildSafetyStore{failUpdateAt: 2}
	runtime := &blockingBuildRuntime{}
	service := NewService(store, runtime)

	_, err := service.BuildProject(context.Background(), &BuildProjectRequest{
		SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1", PublishType: "page",
	})

	require.ErrorContains(t, err, "persist appdev build success")
	require.Equal(t, int32(1), runtime.calls.Load())
}

func TestBuildProjectReportsRunnerAndFailurePersistenceErrors(t *testing.T) {
	t.Setenv("APP_ENV", "release")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "false")
	store := &buildSafetyStore{failUpdateAt: 2}
	runtime := &blockingBuildRuntime{buildErr: errors.New("runner failed")}
	service := NewService(store, runtime)

	_, err := service.BuildProject(context.Background(), &BuildProjectRequest{
		SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1", PublishType: "page",
	})

	require.ErrorContains(t, err, "runner failed")
	require.ErrorContains(t, err, "persist appdev build failure")
}
