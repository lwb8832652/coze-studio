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

package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/model/playground"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	"github.com/coze-dev/coze-studio/backend/domain/user/entity"
	userservice "github.com/coze-dev/coze-studio/backend/domain/user/service"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestSpaceDo2BotSpaceV2PreservesRoleAndMemberCount(t *testing.T) {
	t.Parallel()

	botSpace := spaceDo2BotSpaceV2(&entity.Space{
		ID:             101,
		Name:           "团队空间",
		Description:    "协作",
		IconURL:        "https://cdn.example.test/space.png",
		SpaceType:      entity.SpaceTypeTeam,
		OwnerID:        9,
		AllowDevelop:   true,
		ReceivePublish: false,
		RoleType:       2,
		MemberCount:    7,
	})

	require.Equal(t, int64(101), botSpace.ID)
	require.Equal(t, "团队空间", botSpace.Name)
	require.Equal(t, int32(2), botSpace.RoleType)
	require.Equal(t, playground.SpaceRoleType_Admin, botSpace.SpaceRoleType)
	require.Equal(t, int64(9), botSpace.GetOwnerUserID())
	require.Equal(t, int64(7), botSpace.GetTotalMemberNum())
	require.True(t, botSpace.AllowDevelop)
	require.False(t, botSpace.ReceivePublish)
}

func TestSaveSpaceV2CreatesTeamSpaceForCurrentUser(t *testing.T) {
	t.Parallel()

	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &entity.Session{
		UserID: 9,
	})
	domain := &saveSpaceDomain{}
	app := &UserApplicationService{
		DomainSVC: domain,
	}

	resp, err := app.SaveSpaceV2(ctx, &SaveSpaceV2Request{
		Name:        "  畅享 AI  ",
		Description: "  团队协作空间  ",
		IconURI:     "space/team.png",
		SpaceType:   playground.SpaceType_Team,
	})

	require.NoError(t, err)
	require.Equal(t, int64(0), resp.Code)
	require.Equal(t, "201", resp.Data.ID)
	require.False(t, resp.Data.CheckNotPass)
	require.Equal(t, int64(9), domain.request.UserID)
	require.Equal(t, "畅享 AI", domain.request.Name)
	require.Equal(t, "团队协作空间", domain.request.Description)
	require.Equal(t, "space/team.png", domain.request.IconURI)
	require.Equal(t, entity.SpaceTypeTeam, domain.request.SpaceType)
	require.Equal(t, int64(9), ctxutil.MustGetUIDFromCtx(ctx))
}

type saveSpaceDomain struct {
	userservice.User
	request *userservice.CreateSpaceRequest
}

func (d *saveSpaceDomain) CreateSpace(_ context.Context, request *userservice.CreateSpaceRequest) (*entity.Space, error) {
	d.request = request
	return &entity.Space{
		ID:          201,
		Name:        request.Name,
		Description: request.Description,
		IconURL:     "https://cdn.example.test/" + request.IconURI,
		SpaceType:   request.SpaceType,
		OwnerID:     request.UserID,
		RoleType:    1,
		MemberCount: 1,
	}, nil
}
