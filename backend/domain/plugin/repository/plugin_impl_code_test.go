// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"testing"

	"github.com/stretchr/testify/require"

	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

func TestCodePluginRepositoryPublishAllowsNoHTTPTools(t *testing.T) {
	require.False(t, pluginPublishRequiresActivatedTools(common.PluginType_FUNC))
	require.True(t, pluginPublishRequiresActivatedTools(common.PluginType_PLUGIN))
}

func TestCodePluginPublishNeverCarriesHTTPTools(t *testing.T) {
	historical := []*entity.ToolInfo{{}, {}}
	require.Empty(t, toolsForPluginPublish(common.PluginType_FUNC, historical))
	require.Equal(t, historical, toolsForPluginPublish(common.PluginType_PLUGIN, historical))
}
