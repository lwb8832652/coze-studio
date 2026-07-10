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

package appdev_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	infraappdev "github.com/coze-dev/coze-studio/backend/infra/appdev"
)

type snapshotRuntime struct {
	calls []string
}

func (r *snapshotRuntime) Start(_ context.Context, _ *applicationappdev.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	r.calls = append(r.calls, "start")
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusRunning}, nil
}

func (r *snapshotRuntime) Status(_ context.Context, _ *applicationappdev.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	r.calls = append(r.calls, "status")
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusRunning}, nil
}

func (r *snapshotRuntime) KeepAlive(_ context.Context, _ *applicationappdev.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusRunning}, nil
}

func (r *snapshotRuntime) Restart(_ context.Context, _ *applicationappdev.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	r.calls = append(r.calls, "restart")
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusRunning}, nil
}

func (r *snapshotRuntime) Stop(_ context.Context, _ *applicationappdev.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	r.calls = append(r.calls, "stop")
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusStopped}, nil
}

func (r *snapshotRuntime) Logs(_ context.Context, _ *applicationappdev.RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error) {
	return nil, nil
}

func TestRestoreSnapshotRestartsActiveRuntimeAfterMaterialization(t *testing.T) {
	ctx := context.Background()
	store := infraappdev.NewLocalStoreForTest(t.TempDir())
	runtime := &snapshotRuntime{}
	svc := applicationappdev.NewService(store, runtime)

	created, err := svc.CreateProject(ctx, &applicationappdev.CreateProjectRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		Name:          "snapshot runtime",
		Prompt:        "initial",
	})
	require.NoError(t, err)

	snapshot, err := svc.CreateSnapshot(ctx, &applicationappdev.CreateSnapshotRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Label:         "initial",
	})
	require.NoError(t, err)

	_, err = svc.SaveFileContent(ctx, &applicationappdev.SaveFileContentRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Path:          "src/App.tsx",
		Content:       "changed",
	})
	require.NoError(t, err)

	_, err = svc.RestoreSnapshot(ctx, &applicationappdev.RestoreSnapshotRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		SnapshotID:    snapshot.Data.ID,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"status", "stop", "start"}, runtime.calls)

	content, err := svc.GetFileContent(ctx, &applicationappdev.FileContentRequest{
		SpaceID:       "1001",
		CurrentUserID: 88,
		ProjectID:     created.Data.ID,
		Path:          "src/App.tsx",
	})
	require.NoError(t, err)
	require.NotEqual(t, "changed", content.Data.Content)
}
