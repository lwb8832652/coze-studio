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

package coze_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	cozerouter "github.com/coze-dev/coze-studio/backend/api/router/coze"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	appworkspace "github.com/coze-dev/coze-studio/backend/application/workspace"
	"github.com/coze-dev/coze-studio/backend/pkg/errorx"
	"github.com/coze-dev/coze-studio/backend/types/errno"
)

func TestRegisterIncludesAppDevRoutes(t *testing.T) {
	h := server.Default()
	cozerouter.Register(h)

	listResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/app-dev/spaces/1001/projects",
		nil,
	)
	require.Equal(t, http.StatusUnauthorized, listResp.Code)

	filesResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/app-dev/spaces/1001/projects/appdev_test/files",
		nil,
	)
	require.Equal(t, http.StatusUnauthorized, filesResp.Code)
}

func TestAppDevRoutesRejectNonWorkspaceMembers(t *testing.T) {
	userID := int64(42)
	uidMock := mockey.Mock(ctxutil.GetUIDFromCtx).Return(&userID).Build()
	membershipMock := mockey.Mock(
		(*appworkspace.ApplicationService).CheckWorkspaceAppDevAccess,
	).Return(errorx.New(
		errno.ErrUserPermissionCode,
		errorx.KV("msg", "current user is not a workspace member"),
	)).Build()
	t.Cleanup(func() { uidMock.UnPatch() })
	t.Cleanup(func() { membershipMock.UnPatch() })

	h := server.Default()
	cozerouter.Register(h)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/app-dev/spaces/1001/projects",
		nil,
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"code":700000007`)
	require.Contains(t, body, "current user is not a workspace member")
}

func TestAppDevRoutesRejectWorkspacesWithDevelopmentDisabled(t *testing.T) {
	userID := int64(42)
	uidMock := mockey.Mock(ctxutil.GetUIDFromCtx).Return(&userID).Build()
	accessMock := mockey.Mock(
		(*appworkspace.ApplicationService).CheckWorkspaceAppDevAccess,
	).Return(errorx.New(
		errno.ErrUserPermissionCode,
		errorx.KV("msg", "workspace does not allow development"),
	)).Build()
	t.Cleanup(func() { uidMock.UnPatch() })
	t.Cleanup(func() { accessMock.UnPatch() })

	h := server.Default()
	cozerouter.Register(h)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/app-dev/spaces/1001/projects",
		nil,
	)
	body := string(resp.Result().Body())

	require.Equal(t, http.StatusOK, resp.Code)
	require.Contains(t, body, `"code":700000007`)
	require.Contains(t, body, "workspace does not allow development")
}

func TestAppDevManagementRoutesRequireOwnerOrAdmin(t *testing.T) {
	userID := int64(42)
	uidMock := mockey.Mock(ctxutil.GetUIDFromCtx).Return(&userID).Build()
	accessMock := mockey.Mock(
		(*appworkspace.ApplicationService).CheckWorkspaceAppDevAccess,
	).To(func(
		_ *appworkspace.ApplicationService,
		_ context.Context,
		req *appworkspace.CheckWorkspaceAppDevAccessRequest,
	) error {
		if !req.RequireManager {
			return nil
		}
		return errorx.New(
			errno.ErrUserPermissionCode,
			errorx.KV("msg", "workspace owner or admin role is required"),
		)
	}).Build()
	t.Cleanup(func() { uidMock.UnPatch() })
	t.Cleanup(func() { accessMock.UnPatch() })

	h := server.Default()
	cozerouter.Register(h)

	managementRoutes := []struct {
		method string
		path   string
	}{
		{method: http.MethodDelete, path: "/api/app-dev/spaces/1001/projects/appdev_test"},
		{method: http.MethodPost, path: "/api/app-dev/spaces/1001/projects/appdev_test/build"},
		{method: http.MethodPost, path: "/api/app-dev/spaces/1001/projects/appdev_test/runtime/start"},
		{method: http.MethodPost, path: "/api/app-dev/spaces/1001/projects/appdev_test/runtime/restart"},
		{method: http.MethodPost, path: "/api/app-dev/spaces/1001/projects/appdev_test/runtime/stop"},
	}

	for _, route := range managementRoutes {
		t.Run(route.path, func(t *testing.T) {
			resp := ut.PerformRequest(h.Engine, route.method, route.path, nil)
			body := string(resp.Result().Body())

			require.Equal(t, http.StatusOK, resp.Code)
			require.Contains(t, body, `"code":700000007`)
			require.Contains(t, body, "workspace owner or admin role is required")
		})
	}
}
