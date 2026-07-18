// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	mockPlugin "github.com/coze-dev/coze-studio/backend/internal/mock/domain/plugin"
)

func TestCodePluginMemberCanReadButCannotWrite(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := entity.NewPluginInfo(&model.PluginInfo{
		ID: 9201, PluginType: common.PluginType_FUNC, SpaceID: 2001, DeveloperID: 88,
	})
	repo := &codePluginRepoStub{
		draft:   newPublishableCodeDraft(plugin, 1),
		version: &entity.CodeVersion{PluginID: plugin.ID, SpaceID: plugin.SpaceID, Version: "v1.0.0"},
	}
	service := &PluginApplicationService{
		DomainSVC: domain,
		spaceAccess: &codePluginSpaceAccessStub{members: []*userEntity.SpaceMember{{
			UserID: 88, RoleType: int32(common.SpaceRoleType_Member),
		}}},
		codeRepo: repo,
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(6)
	ctx := codePluginTestContext(88)

	_, err := service.GetCodePluginDraft(ctx, &pluginAPI.GetCodePluginDraftRequest{PluginID: plugin.ID, SpaceID: plugin.SpaceID})
	require.NoError(t, err)
	_, err = service.GetCodePluginVersion(ctx, &pluginAPI.GetCodePluginVersionRequest{PluginID: plugin.ID, SpaceID: plugin.SpaceID, Version: "v1.0.0"})
	require.NoError(t, err)

	_, err = service.SaveCodePluginDraft(ctx, &pluginAPI.SaveCodePluginDraftRequest{PluginID: plugin.ID, SpaceID: plugin.SpaceID})
	require.ErrorIs(t, err, ErrCodePluginPermission)
	_, err = service.DebugCodePlugin(ctx, &pluginAPI.DebugCodePluginRequest{PluginID: plugin.ID, SpaceID: plugin.SpaceID})
	require.ErrorIs(t, err, ErrCodePluginPermission)
	_, err = service.PublishPlugin(ctx, newPublishPluginRequest(plugin.ID))
	require.ErrorIs(t, err, ErrCodePluginPermission)
	_, err = service.DelPlugin(ctx, &pluginAPI.DelPluginRequest{PluginID: plugin.ID})
	require.ErrorIs(t, err, ErrCodePluginPermission)
}

func TestCodePluginNonMemberCannotRead(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := entity.NewPluginInfo(&model.PluginInfo{
		ID: 9202, PluginType: common.PluginType_FUNC, SpaceID: 2001, DeveloperID: 88,
	})
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: &codePluginSpaceAccessStub{},
		codeRepo:    &codePluginRepoStub{},
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := service.GetCodePluginDraft(codePluginTestContext(88), &pluginAPI.GetCodePluginDraftRequest{
		PluginID: plugin.ID,
		SpaceID:  plugin.SpaceID,
	})
	require.ErrorIs(t, err, ErrCodePluginPermission)
}

func TestCodePluginAdminHasWriteCapability(t *testing.T) {
	service := &PluginApplicationService{
		spaceAccess: &codePluginSpaceAccessStub{members: []*userEntity.SpaceMember{{
			UserID: 88, RoleType: int32(common.SpaceRoleType_Admin),
		}}},
	}
	require.NoError(t, service.requirePluginEditor(codePluginTestContext(88), 2001, 88))
}
