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
	"gorm.io/gorm"

	appworkspace "github.com/coze-dev/coze-studio/backend/application/workspace"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
)

func TestWorkspaceHandlersReturnDetailAndMembers(t *testing.T) {
	h := server.Default()
	h.Use(workbenchSessionMiddlewareForTest(9))
	h.GET("/api/workspace/space/detail", GetWorkspaceDetail)
	h.POST("/api/workspace/space/members", ListWorkspaceMembers)
	h.POST("/api/workspace/space/members/add", AddWorkspaceMembers)
	h.POST("/api/workspace/space/member/role", UpdateWorkspaceMemberRole)
	h.POST("/api/workspace/space/member/remove", RemoveWorkspaceMember)
	h.POST("/api/workspace/space/transfer", TransferWorkspace)
	h.POST("/api/workspace/space/delete", DeleteWorkspace)
	installWorkspaceTestService(t)

	detailResp := ut.PerformRequest(
		h.Engine,
		http.MethodGet,
		"/api/workspace/space/detail?space_id=101",
		nil,
	)
	detailBody := string(detailResp.Result().Body())

	require.Equal(t, http.StatusOK, detailResp.Code)
	require.Contains(t, detailBody, `"id":"101"`)
	require.Contains(t, detailBody, `"name":"畅享 AI"`)
	require.Contains(t, detailBody, `"current_user_role":1`)
	require.Contains(t, detailBody, `"total_member_num":2`)

	membersResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workspace/space/members",
		&ut.Body{
			Body: bytes.NewBufferString(`{"space_id":"101"}`),
			Len:  len(`{"space_id":"101"}`),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	membersBody := string(membersResp.Result().Body())

	require.Equal(t, http.StatusOK, membersResp.Code)
	require.Contains(t, membersBody, `"user_id":"9"`)
	require.Contains(t, membersBody, `"name":"Owner"`)
	require.Contains(t, membersBody, `"role_type":1`)

	addResp := ut.PerformRequest(
		h.Engine,
		http.MethodPost,
		"/api/workspace/space/members/add",
		&ut.Body{
			Body: bytes.NewBufferString(`{"space_id":"101","members":[{"user_id":"11","role_type":3}]}`),
			Len:  len(`{"space_id":"101","members":[{"user_id":"11","role_type":3}]}`),
		},
		ut.Header{Key: "content-type", Value: "application/json"},
	)
	require.Equal(t, http.StatusOK, addResp.Code)
	require.Contains(t, string(addResp.Result().Body()), `"code":0`)
}

func installWorkspaceTestService(t *testing.T) {
	t.Helper()

	previous := appworkspace.SVC.UserDomainSVC
	t.Cleanup(func() {
		appworkspace.SVC.UserDomainSVC = previous
	})

	appworkspace.SVC.UserDomainSVC = &workspaceHandlerUserDomain{
		spaces: []*userentity.Space{
			{
				ID:          101,
				Name:        "畅享 AI",
				Description: "团队协作空间",
				OwnerID:     9,
			},
		},
		members: []*userentity.SpaceMember{
			{
				UserID:   9,
				Name:     "Owner",
				Email:    "owner@example.test",
				RoleType: 1,
				JoinedAt: 1717000000,
			},
			{
				UserID:   10,
				Name:     "Member",
				Email:    "member@example.test",
				RoleType: 3,
				JoinedAt: 1717000100,
			},
		},
	}
}

type workspaceHandlerUserDomain struct {
	spaces       []*userentity.Space
	members      []*userentity.SpaceMember
	addedMembers []*userservice.AddSpaceMemberRequest
}

func (d *workspaceHandlerUserDomain) GetUserSpaceList(_ context.Context, _ int64) ([]*userentity.Space, error) {
	return d.spaces, nil
}

func (d *workspaceHandlerUserDomain) GetSpaceMembers(_ context.Context, _ int64) ([]*userentity.SpaceMember, error) {
	return d.members, nil
}

func (d *workspaceHandlerUserDomain) ListAllUsers(_ context.Context, _ string, _ int, _ int) ([]*userentity.User, int64, error) {
	return nil, 0, nil
}

func (d *workspaceHandlerUserDomain) UpdateSpace(_ context.Context, _ *userservice.UpdateSpaceRequest) error {
	return nil
}

func (d *workspaceHandlerUserDomain) AddSpaceMembers(_ context.Context, members []*userservice.AddSpaceMemberRequest) error {
	d.addedMembers = append(d.addedMembers, members...)
	return nil
}

func (d *workspaceHandlerUserDomain) AddSpaceMembersWithNotification(_ context.Context, _ int64, members []*userservice.AddSpaceMemberRequest, _ func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return d.AddSpaceMembers(context.Background(), members)
}

func (d *workspaceHandlerUserDomain) UpdateSpaceMemberRole(context.Context, int64, int64, int32) error {
	return nil
}

func (d *workspaceHandlerUserDomain) UpdateSpaceMemberRoleWithNotification(context.Context, int64, int64, int64, int32, func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return nil
}

func (d *workspaceHandlerUserDomain) RemoveSpaceMember(context.Context, int64, int64) error {
	return nil
}

func (d *workspaceHandlerUserDomain) RemoveSpaceMemberWithNotification(context.Context, int64, int64, int64, func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return nil
}

func (d *workspaceHandlerUserDomain) TransferSpace(context.Context, int64, int64) error {
	return nil
}

func (d *workspaceHandlerUserDomain) TransferSpaceWithNotification(context.Context, int64, int64, int64, func(context.Context, *gorm.DB, domainnotification.Event) error) error {
	return nil
}

func (d *workspaceHandlerUserDomain) DeleteSpace(context.Context, int64) error {
	return nil
}
