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
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestRegisterIncludesAdminAuthStatusRoute(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	resp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/admin/auth/status",
		nil,
	)

	require.NotEqual(t, http.StatusNotFound, resp.Code)
}

func TestRegisterIncludesAdminManagementRoutes(t *testing.T) {
	h := server.Default()
	Register(h)
	RegisterCustomRoutes(h)

	workspacesResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/workspaces/list",
		nil,
	)
	require.NotEqual(t, http.StatusNotFound, workspacesResp.Code)

	usersResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/users/list",
		nil,
	)
	require.NotEqual(t, http.StatusNotFound, usersResp.Code)

	membersResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/workspaces/members",
		nil,
	)
	require.NotEqual(t, http.StatusNotFound, membersResp.Code)

	userSpacesResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/users/spaces",
		nil,
	)
	require.NotEqual(t, http.StatusNotFound, userSpacesResp.Code)
}
