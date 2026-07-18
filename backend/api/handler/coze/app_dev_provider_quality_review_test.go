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

package coze

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestAppDevProviderFacadeDomainNotFoundUsesSafe404(t *testing.T) {
	runtime := providerHTTPNotFoundRuntime{}

	for _, test := range []struct {
		name         string
		project      bool
		snapshotOnly bool
	}{
		{name: "missing project", snapshotOnly: true},
		{name: "missing snapshot", project: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newProviderHTTPPersistentStore(t, test.project, test.snapshotOnly)
			facade := appdev.NewProviderAPIFacade(runtime, nil, nil, store)
			control := &providerHTTPFacadeControl{appDevProviderControlFake: &appDevProviderControlFake{}, facade: facade}
			handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)
			response := ut.PerformRequest(handler.Engine, http.MethodPost, appDevProviderTestPath+"/snapshots/missing-snapshot/restore", nil,
				ut.Header{Key: appDevIdempotencyKeyHeader, Value: "missing-snapshot-operation"})
			require.Equal(t, http.StatusNotFound, response.Code)
			require.Contains(t, string(response.Result().Body()), `"code":"not_found"`)
			for _, internal := range []string{"sql", "record not found", "object_key", "provider"} {
				require.NotContains(t, strings.ToLower(string(response.Result().Body())), internal)
			}
		})
	}

	t.Run("missing release", func(t *testing.T) {
		store := newProviderHTTPPersistentStore(t, false, false)
		facade := appdev.NewProviderAPIFacade(nil, nil, providerHTTPPersistentRelease{projects: store}, nil)
		control := &providerHTTPFacadeControl{appDevProviderControlFake: &appDevProviderControlFake{}, facade: facade}
		handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)
		response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/release", nil)
		require.Equal(t, http.StatusNotFound, response.Code)
		require.Contains(t, string(response.Result().Body()), `"code":"not_found"`)
		require.NotContains(t, strings.ToLower(string(response.Result().Body())), "object")
	})
}

func TestAppDevProviderBuildProjectionForwardsAuthoritativeStale(t *testing.T) {
	control := &appDevProviderControlFake{buildResult: &appdev.ProviderBuildAPIProjection{
		Generation: 17, State: appdev.ProviderBuildStateReady, ReleaseAvailable: true, Size: 42, Stale: true,
	}}
	handler := newAppDevProviderHTTPHandlerForTest(t, control, &appDevPreviewProjectorFake{result: "https://preview.example.test/app"}, true)
	response := ut.PerformRequest(handler.Engine, http.MethodGet, appDevProviderTestPath+"/build", nil,
		ut.Header{Key: appDevIdempotencyKeyHeader, Value: "build-stale-operation"})
	require.Equal(t, http.StatusOK, response.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Result().Body(), &payload))
	require.Equal(t, true, payload["stale"])
}

type providerHTTPFacadeControl struct {
	*appDevProviderControlFake
	facade *appdev.ProviderAPIFacade
}

func (control *providerHTTPFacadeControl) RestoreSnapshot(ctx context.Context, request appdev.ProviderSnapshotRestoreRequest) (*appdev.ProviderRuntimeAPIProjection, error) {
	return control.facade.RestoreSnapshot(ctx, request)
}

func (control *providerHTTPFacadeControl) OpenRelease(ctx context.Context, request appdev.ProviderReleaseRequest) (*appDevHTTPRelease, error) {
	release, err := control.facade.OpenRelease(ctx, request)
	if err != nil || release == nil {
		return nil, err
	}
	return &appDevHTTPRelease{Body: release, Size: release.Size, Stale: release.Stale, UpdatedAt: release.UpdatedAt}, nil
}

type providerHTTPNotFoundRuntime struct{}

func (providerHTTPNotFoundRuntime) Start(context.Context, appdev.ProviderRuntimeStartInput) (*appdev.ProviderRuntimeProjection, error) {
	return nil, errors.New("unexpected start")
}

func (providerHTTPNotFoundRuntime) Status(context.Context, appdev.ProviderRuntimeStatusInput) (*appdev.ProviderRuntimeProjection, error) {
	return &appdev.ProviderRuntimeProjection{State: appdev.ProviderRuntimeStateStopped, CanStart: true}, nil
}

func (providerHTTPNotFoundRuntime) Stop(context.Context, appdev.ProviderRuntimeStopInput) (*appdev.ProviderRuntimeProjection, error) {
	return nil, errors.New("unexpected stop")
}

func (providerHTTPNotFoundRuntime) MatchCurrentStartOperation(context.Context, appdev.ProviderRuntimeStartInput) (*appdev.ProviderRuntimeProjection, bool, error) {
	return nil, false, nil
}

type providerHTTPPersistentRelease struct{ projects *infraappdev.PersistentStore }

func (release providerHTTPPersistentRelease) OpenRelease(ctx context.Context, request appdev.ProviderReleaseRequest) (*appdev.ProviderRelease, error) {
	if _, err := release.projects.GetProject(ctx, request.SpaceID, request.ProjectID); err != nil {
		return nil, err
	}
	return nil, domainappdev.ErrNotFound
}

func newProviderHTTPPersistentStore(t *testing.T, withProject, withSnapshot bool) *infraappdev.PersistentStore {
	t.Helper()
	dsn := "file:handler-provider-notfound-" + strings.ReplaceAll(t.Name(), "/", "-") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE appdev_projects (id text PRIMARY KEY, space_id integer NOT NULL)").Error)
	require.NoError(t, db.Exec("CREATE TABLE appdev_project_snapshots (id text PRIMARY KEY, space_id integer NOT NULL, project_id text NOT NULL)").Error)
	if withProject {
		require.NoError(t, db.Exec("INSERT INTO appdev_projects (id, space_id) VALUES (?, ?)", "project-safe", 1001).Error)
	}
	if withSnapshot {
		require.NoError(t, db.Exec("INSERT INTO appdev_project_snapshots (id, space_id, project_id) VALUES (?, ?, ?)", "missing-snapshot", 1001, "project-safe").Error)
	}
	return infraappdev.NewPersistentStoreForTest(db, providerHTTPNoopStorage{}, t.TempDir())
}

type providerHTTPNoopStorage struct{}

func (providerHTTPNoopStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error {
	return nil
}
func (providerHTTPNoopStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error {
	return nil
}
func (providerHTTPNoopStorage) GetObject(context.Context, string) ([]byte, error) {
	return nil, storage.ErrObjectNotFound
}
func (providerHTTPNoopStorage) DeleteObject(context.Context, string) error { return nil }
func (providerHTTPNoopStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "", storage.ErrObjectNotFound
}
func (providerHTTPNoopStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) {
	return nil, storage.ErrObjectNotFound
}
func (providerHTTPNoopStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) {
	return nil, nil
}
func (providerHTTPNoopStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return &storage.ListObjectsPaginatedOutput{}, nil
}
