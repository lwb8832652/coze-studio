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
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appadmin "github.com/coze-dev/coze-studio/backend/application/admin"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
)

func TestAdminManagementHandlersReturnWorkspacesAndUsers(t *testing.T) {
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(9))
	h.POST("/api/admin/workspaces/list", ListAdminWorkspaces)
	h.POST("/api/admin/workspaces/members", ListAdminWorkspaceMembers)
	h.POST("/api/admin/users/list", ListAdminUsers)
	h.POST("/api/admin/users/spaces", ListAdminUserSpaces)
	installAdminManagementTestService(t)

	workspacesResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/workspaces/list",
		&ut.Body{
			Body: bytes.NewBufferString(`{"keyword":"畅享","page":1,"size":20}`),
			Len:  len(`{"keyword":"畅享","page":1,"size":20}`),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	workspacesBody := string(workspacesResp.Result().Body())

	require.Equal(t, http.StatusOK, workspacesResp.Code)
	require.Contains(t, workspacesBody, `"id":"101"`)
	require.Contains(t, workspacesBody, `"name":"畅享 AI"`)
	require.Contains(t, workspacesBody, `"owner_name":"Owner"`)
	require.Contains(t, workspacesBody, `"total_member_num":2`)

	usersResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/users/list",
		&ut.Body{
			Body: bytes.NewBufferString(`{"keyword":"owner","page":1,"size":20}`),
			Len:  len(`{"keyword":"owner","page":1,"size":20}`),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	usersBody := string(usersResp.Result().Body())

	require.Equal(t, http.StatusOK, usersResp.Code)
	require.Contains(t, usersBody, `"user_id":"9"`)
	require.Contains(t, usersBody, `"email":"owner@example.test"`)

	membersResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/workspaces/members",
		&ut.Body{
			Body: bytes.NewBufferString(`{"space_id":"101"}`),
			Len:  len(`{"space_id":"101"}`),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	membersBody := string(membersResp.Result().Body())

	require.Equal(t, http.StatusOK, membersResp.Code)
	require.Contains(t, membersBody, `"user_id":"9"`)
	require.Contains(t, membersBody, `"email":"owner@example.test"`)
	require.Contains(t, membersBody, `"role_type":2`)

	userSpacesResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/admin/users/spaces",
		&ut.Body{
			Body: bytes.NewBufferString(`{"user_id":"9"}`),
			Len:  len(`{"user_id":"9"}`),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	userSpacesBody := string(userSpacesResp.Result().Body())

	require.Equal(t, http.StatusOK, userSpacesResp.Code)
	require.Contains(t, userSpacesBody, `"id":"101"`)
	require.Contains(t, userSpacesBody, `"name":"畅享 AI"`)
	require.Contains(t, userSpacesBody, `"role_type":2`)
}

func installAdminManagementTestService(t *testing.T) {
	t.Helper()

	previousDomain := appadmin.ManagementApplicationSVC.UserDomainSVC
	t.Cleanup(func() {
		appadmin.ManagementApplicationSVC.UserDomainSVC = previousDomain
	})

	appadmin.ManagementApplicationSVC.UserDomainSVC = &adminManagementUserDomain{
		users: []*userentity.User{
			{
				UserID:     9,
				Name:       "Owner",
				UniqueName: "owner",
				Email:      "owner@example.test",
				CreatedAt:  1717000000,
			},
		},
		usersByID: map[int64]*userentity.User{
			9: {
				UserID: 9,
				Name:   "Owner",
				Email:  "owner@example.test",
			},
		},
		spaces: []*userentity.Space{
			{
				ID:          101,
				Name:        "畅享 AI",
				Description: "团队协作空间",
				OwnerID:     9,
				MemberCount: 2,
				CreatedAt:   1717000000,
			},
		},
		userSpaces: []*userentity.Space{
			{
				ID:          101,
				Name:        "畅享 AI",
				Description: "团队协作空间",
				OwnerID:     9,
				RoleType:    2,
				MemberCount: 2,
				CreatedAt:   1717000000,
			},
		},
		members: []*userentity.SpaceMember{
			{
				UserID:     9,
				Name:       "Owner",
				UniqueName: "owner",
				Email:      "owner@example.test",
				RoleType:   2,
				JoinedAt:   1717000000,
			},
		},
		total: 1,
	}
}

type adminManagementUserDomain struct {
	users      []*userentity.User
	usersByID  map[int64]*userentity.User
	spaces     []*userentity.Space
	userSpaces []*userentity.Space
	members    []*userentity.SpaceMember
	total      int64
}

func (d *adminManagementUserDomain) ListAllUsers(context.Context, string, int, int) ([]*userentity.User, int64, error) {
	return d.users, d.total, nil
}

func (d *adminManagementUserDomain) ListAllSpaces(context.Context, string, int, int) ([]*userentity.Space, int64, error) {
	return d.spaces, d.total, nil
}

func (d *adminManagementUserDomain) MGetUserProfiles(_ context.Context, userIDs []int64) ([]*userentity.User, error) {
	users := make([]*userentity.User, 0, len(userIDs))
	for _, userID := range userIDs {
		if user := d.usersByID[userID]; user != nil {
			users = append(users, user)
		}
	}
	return users, nil
}

func (d *adminManagementUserDomain) GetSpaceMembers(context.Context, int64) ([]*userentity.SpaceMember, error) {
	return d.members, nil
}

func (d *adminManagementUserDomain) GetUserSpaceList(context.Context, int64) ([]*userentity.Space, error) {
	return d.userSpaces, nil
}
