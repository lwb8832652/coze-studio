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
	"testing"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type runtimePersistenceStore struct {
	domainappdev.Store
	projectDir string
	updateErr  error
}

func (s *runtimePersistenceStore) GetProject(context.Context, string, string) (*domainappdev.Project, error) {
	return &domainappdev.Project{ID: "project-1", SpaceID: "1001"}, nil
}

func (s *runtimePersistenceStore) ProjectFilesDir(string, string) (string, error) {
	return s.projectDir, nil
}

func (s *runtimePersistenceStore) UpdateProjectRuntime(
	context.Context,
	string,
	string,
	domainappdev.RuntimeStatus,
	string,
) (*domainappdev.Project, error) {
	return nil, s.updateErr
}

type runtimePersistenceManager struct {
	RuntimeManager
	stopCalls int
}

func (m *runtimePersistenceManager) Start(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return runningRuntimeInfo(), nil
}

func (m *runtimePersistenceManager) Status(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return runningRuntimeInfo(), nil
}

func (m *runtimePersistenceManager) KeepAlive(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return runningRuntimeInfo(), nil
}

func (m *runtimePersistenceManager) Restart(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return runningRuntimeInfo(), nil
}

func (m *runtimePersistenceManager) Stop(context.Context, *RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	m.stopCalls++
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusStopped}, nil
}

func runningRuntimeInfo() *domainappdev.RuntimeInfo {
	return &domainappdev.RuntimeInfo{
		Status:     domainappdev.RuntimeStatusRunning,
		PreviewURL: "http://127.0.0.1:5173",
	}
}

func TestRuntimeAPIsDoNotSucceedWithUnpersistedState(t *testing.T) {
	testCases := []struct {
		name          string
		call          func(*Service, context.Context, *RuntimeRequest) error
		expectedStops int
	}{
		{
			name: "start rolls back the new runtime",
			call: func(service *Service, ctx context.Context, req *RuntimeRequest) error {
				_, err := service.StartRuntime(ctx, req)
				return err
			},
			expectedStops: 1,
		},
		{
			name: "restart rolls back the replacement runtime",
			call: func(service *Service, ctx context.Context, req *RuntimeRequest) error {
				_, err := service.RestartRuntime(ctx, req)
				return err
			},
			expectedStops: 1,
		},
		{
			name: "stop reports the persistence failure",
			call: func(service *Service, ctx context.Context, req *RuntimeRequest) error {
				_, err := service.StopRuntime(ctx, req)
				return err
			},
			expectedStops: 1,
		},
		{
			name: "status reports the persistence failure",
			call: func(service *Service, ctx context.Context, req *RuntimeRequest) error {
				_, err := service.GetRuntimeStatus(ctx, req)
				return err
			},
		},
		{
			name: "keepalive reports the persistence failure",
			call: func(service *Service, ctx context.Context, req *RuntimeRequest) error {
				_, err := service.KeepAliveRuntime(ctx, req)
				return err
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			store := &runtimePersistenceStore{
				projectDir: t.TempDir(),
				updateErr:  errors.New("forced runtime persistence failure"),
			}
			manager := &runtimePersistenceManager{}
			service := NewService(store, manager)
			req := &RuntimeRequest{SpaceID: "1001", CurrentUserID: 42, ProjectID: "project-1"}

			err := testCase.call(service, context.Background(), req)

			require.ErrorContains(t, err, "persist appdev runtime state")
			require.Equal(t, testCase.expectedStops, manager.stopCalls)
		})
	}
}
