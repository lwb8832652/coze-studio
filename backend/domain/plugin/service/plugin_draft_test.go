// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/consts"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
)

type capturingPluginRepository struct {
	repository.PluginRepository
	plugin *entity.PluginInfo
}

func (r *capturingPluginRepository) CreateDraftPlugin(_ context.Context, plugin *entity.PluginInfo) (int64, error) {
	r.plugin = plugin
	return 1001, nil
}

func TestCreateDraftCodePluginAllowsEmptyServerURL(t *testing.T) {
	repo := &capturingPluginRepository{}
	service := &pluginServiceImpl{pluginRepo: repo}
	authType := consts.AuthzTypeOfNone

	pluginID, err := service.CreateDraftPlugin(context.Background(), &dto.CreateDraftPluginRequest{
		PluginType:  common.PluginType_FUNC,
		IconURI:     "default_icon/plugin_default_icon.png",
		SpaceID:     2001,
		DeveloperID: 3001,
		Name:        "code_plugin",
		Desc:        "code plugin",
		AuthInfo: &dto.PluginAuthInfo{
			AuthzType: &authType,
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(1001), pluginID)
	require.NotNil(t, repo.plugin)
	require.Empty(t, repo.plugin.OpenapiDoc.Servers)
	require.Equal(t, "", repo.plugin.GetServerURL())
}

func TestCreateDraftHTTPPluginStillRequiresServerURL(t *testing.T) {
	repo := &capturingPluginRepository{}
	service := &pluginServiceImpl{pluginRepo: repo}
	authType := consts.AuthzTypeOfNone

	_, err := service.CreateDraftPlugin(context.Background(), &dto.CreateDraftPluginRequest{
		PluginType:  common.PluginType_PLUGIN,
		IconURI:     "default_icon/plugin_default_icon.png",
		SpaceID:     2001,
		DeveloperID: 3001,
		Name:        "http_plugin",
		Desc:        "http plugin",
		AuthInfo: &dto.PluginAuthInfo{
			AuthzType: &authType,
		},
	})

	require.Error(t, err)
	require.Nil(t, repo.plugin)
}
