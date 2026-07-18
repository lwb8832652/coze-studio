// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/plugin/service"
)

type p1CodePluginDomain struct {
	domainservice.PluginService
	plugin    *entity.PluginInfo
	updateErr error
}

func (s *p1CodePluginDomain) GetDraftPlugin(context.Context, int64) (*entity.PluginInfo, error) {
	return s.plugin, nil
}

func (s *p1CodePluginDomain) UpdateDraftPlugin(context.Context, *dto.UpdateDraftPluginRequest) error {
	return s.updateErr
}

type p1SaveCodeRepo struct {
	repository.CodePluginRepository
	saveErr error
}

func (r *p1SaveCodeRepo) SaveDraftCAS(
	context.Context,
	*entity.CodeDraft,
	int64,
) (*entity.CodeDraft, error) {
	return nil, r.saveErr
}

func TestSaveCodePluginDraftClassifiesInvalidRuntimeAsBadRequest(t *testing.T) {
	plugin := codePluginInfo(1099, 2099, 88)
	service := &PluginApplicationService{
		DomainSVC:   &p1CodePluginDomain{plugin: plugin},
		spaceAccess: codePluginOwnerSpaceAccess(),
		codeRepo:    &p1SaveCodeRepo{},
	}

	_, err := service.SaveCodePluginDraft(codePluginTestContext(88), p1SaveDraftRequest(plugin, 999))

	require.ErrorIs(t, err, ErrCodePluginInvalidRequest)
	require.NotErrorIs(t, err, ErrCodePluginUnavailable)
}

func TestSaveCodePluginDraftClassifiesDatabaseFailureAsUnavailable(t *testing.T) {
	plugin := codePluginInfo(1099, 2099, 88)
	service := &PluginApplicationService{
		DomainSVC:   &p1CodePluginDomain{plugin: plugin},
		spaceAccess: codePluginOwnerSpaceAccess(),
		codeRepo:    &p1SaveCodeRepo{saveErr: errors.New("database unavailable")},
	}

	_, err := service.SaveCodePluginDraft(
		codePluginTestContext(88),
		p1SaveDraftRequest(plugin, int64(common.CodePluginRuntime_Python)),
	)

	require.ErrorIs(t, err, ErrCodePluginUnavailable)
	require.NotErrorIs(t, err, ErrCodePluginInvalidRequest)
}

func TestRegisterPluginMetaWithoutSessionIsForbidden(t *testing.T) {
	service := &PluginApplicationService{}

	_, err := service.RegisterPluginMeta(context.Background(), &pluginAPI.RegisterPluginMetaRequest{})

	require.ErrorIs(t, err, ErrCodePluginPermission)
}

func TestUpdateCodePluginMetaPersistenceFailureIsUnavailable(t *testing.T) {
	plugin := codePluginInfo(1099, 2099, 88)
	service := &PluginApplicationService{
		DomainSVC: &p1CodePluginDomain{
			plugin:    plugin,
			updateErr: errors.New("database unavailable"),
		},
		spaceAccess: codePluginOwnerSpaceAccess(),
	}

	_, err := service.UpdatePluginMeta(codePluginTestContext(88), &pluginAPI.UpdatePluginMetaRequest{
		PluginID: plugin.ID,
	})

	require.ErrorIs(t, err, ErrCodePluginUnavailable)
}

func p1SaveDraftRequest(
	plugin *entity.PluginInfo,
	runtime int64,
) *pluginAPI.SaveCodePluginDraftRequest {
	inputSchema := entity.DefaultCodeSchemaJSON
	outputSchema := entity.DefaultCodeSchemaJSON
	return &pluginAPI.SaveCodePluginDraftRequest{
		PluginID:         plugin.ID,
		SpaceID:          plugin.SpaceID,
		Runtime:          common.CodePluginRuntime(runtime),
		EntryFile:        "main.py",
		Files:            []*common.CodePluginFile{{Path: "main.py", Content: defaultPythonCode}},
		InputSchemaJSON:  &inputSchema,
		OutputSchemaJSON: &outputSchema,
	}
}
