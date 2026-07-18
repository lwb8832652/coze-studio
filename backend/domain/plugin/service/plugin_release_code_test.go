// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/consts"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
)

func TestCodePluginPublishDoesNotRequireHTTPTools(t *testing.T) {
	require.False(t, pluginPublishRequiresTools(common.PluginType_FUNC))
	require.True(t, pluginPublishRequiresTools(common.PluginType_PLUGIN))
}

func TestSanitizeCodePluginHTTPConfigurationClearsHistoricalState(t *testing.T) {
	serverURL := "https://legacy.example"
	plugin := entity.NewPluginInfo(&model.PluginInfo{
		ID:         42,
		PluginType: common.PluginType_FUNC,
		ServerURL:  &serverURL,
		Manifest: &model.PluginManifest{
			Auth: &model.AuthV2{
				Type:    consts.AuthzTypeOfService,
				SubType: consts.AuthzSubTypeOfServiceAPIToken,
				Payload: "TOP_SECRET",
			},
			CommonParams: map[consts.HTTPParamLocation][]*common.CommonParamSchema{
				consts.ParamInHeader: {{Name: "Authorization", Value: "TOP_SECRET"}},
			},
		},
		OpenapiDoc: &model.Openapi3T{
			Servers: openapi3.Servers{{URL: serverURL}},
			Paths: openapi3.Paths{
				"/legacy": &openapi3.PathItem{},
			},
		},
	})

	sanitizeCodePluginHTTPConfiguration(plugin)

	require.Empty(t, plugin.GetServerURL())
	require.Empty(t, plugin.OpenapiDoc.Servers)
	require.Empty(t, plugin.OpenapiDoc.Paths)
	require.Empty(t, plugin.Manifest.CommonParams)
	require.NotNil(t, plugin.Manifest.Auth)
	require.Equal(t, consts.AuthzTypeOfNone, plugin.Manifest.Auth.Type)
	require.Empty(t, plugin.Manifest.Auth.Payload)
}
