// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
)

type p1AmbiguousCodeRepo struct {
	repository.CodePluginRepository
	published     map[int64]bool
	compensateErr map[int64]error
	states        map[int64]repository.CodeVersionPreparationState
	prepared      []int64
	compensated   []int64
	ensured       []int64
}

func (r *p1AmbiguousCodeRepo) PrepareDebuggedVersion(
	_ context.Context,
	pluginID int64,
	version string,
	operatorID int64,
) (*repository.PreparedCodeVersion, error) {
	r.prepared = append(r.prepared, pluginID)
	state := r.states[pluginID]
	if state == "" {
		state = repository.CodeVersionPrepared
	}
	return &repository.PreparedCodeVersion{
		Version: &entity.CodeVersion{
			PluginID:       pluginID,
			Version:        version,
			SpaceID:        2001,
			SourceRevision: 1,
			CreatedBy:      operatorID,
		},
		Created: state == repository.CodeVersionPrepared,
		State:   state,
	}, nil
}

func (r *p1AmbiguousCodeRepo) CompensatePreparedVersion(
	_ context.Context,
	prepared *repository.PreparedCodeVersion,
) (bool, error) {
	pluginID := prepared.Version.PluginID
	r.compensated = append(r.compensated, pluginID)
	if err := r.compensateErr[pluginID]; err != nil {
		return false, err
	}
	return r.published[pluginID], nil
}

func (r *p1AmbiguousCodeRepo) EnsurePublishedVersion(
	_ context.Context,
	prepared *repository.PreparedCodeVersion,
) error {
	r.ensured = append(r.ensured, prepared.Version.PluginID)
	return nil
}

type p1AmbiguousPluginRepo struct {
	repository.PluginRepository
	drafts    []*entity.PluginInfo
	published []int64
}

func (r *p1AmbiguousPluginRepo) GetAPPAllDraftPlugins(
	context.Context,
	int64,
	...repository.PluginSelectedOptions,
) ([]*entity.PluginInfo, error) {
	return r.drafts, nil
}

func (r *p1AmbiguousPluginRepo) MGetOnlinePlugins(
	context.Context,
	[]int64,
	...repository.PluginSelectedOptions,
) ([]*entity.PluginInfo, error) {
	return nil, nil
}

func (r *p1AmbiguousPluginRepo) UpdateDraftPlugin(context.Context, *entity.PluginInfo) error {
	return nil
}

func (r *p1AmbiguousPluginRepo) PublishPlugins(
	_ context.Context,
	plugins []*entity.PluginInfo,
) error {
	for _, plugin := range plugins {
		r.published = append(r.published, plugin.ID)
	}
	return errors.New("ambiguous database commit")
}

type p1PublishToolRepo struct {
	repository.ToolRepository
}

func (p1PublishToolRepo) GetPluginAllDraftTools(
	context.Context,
	int64,
	...repository.ToolSelectedOptions,
) ([]*entity.ToolInfo, error) {
	status := common.APIDebugStatus_DebugPassed
	return []*entity.ToolInfo{{DebugStatus: &status}}, nil
}

type p1SuccessfulPluginRepo struct {
	p1AmbiguousPluginRepo
}

func (r *p1SuccessfulPluginRepo) PublishPlugins(
	_ context.Context,
	plugins []*entity.PluginInfo,
) error {
	for _, plugin := range plugins {
		r.published = append(r.published, plugin.ID)
	}
	return nil
}

func TestPublishAPPPluginsRecoversAllCommittedMixedOrdinaryAndFUNC(t *testing.T) {
	codeRepo := &p1AmbiguousCodeRepo{published: map[int64]bool{21: true}}
	pluginRepo := &p1AmbiguousPluginRepo{drafts: []*entity.PluginInfo{
		p1BatchPlugin(20, common.PluginType_PLUGIN),
		p1BatchPlugin(21, common.PluginType_FUNC),
	}}
	service := &pluginServiceImpl{
		pluginRepo: pluginRepo,
		toolRepo:   p1PublishToolRepo{},
		codeRepo:   codeRepo,
	}

	response, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.NoError(t, err)
	require.Len(t, response.AllDraftPlugins, 2)
	require.Equal(t, []int64{20, 21}, pluginRepo.published)
	require.Equal(t, []int64{21}, codeRepo.compensated)
	require.Equal(t, []int64{21}, codeRepo.ensured)
}

func TestPublishAPPPluginsDoesNotUseAlreadyPublishedFUNCAsCurrentTransactionEvidence(t *testing.T) {
	codeRepo := &p1AmbiguousCodeRepo{
		published: map[int64]bool{21: true},
		states: map[int64]repository.CodeVersionPreparationState{
			21: repository.CodeVersionAlreadyPublished,
		},
	}
	pluginRepo := &p1AmbiguousPluginRepo{drafts: []*entity.PluginInfo{
		p1BatchPlugin(20, common.PluginType_PLUGIN),
		p1BatchPlugin(21, common.PluginType_FUNC),
	}}
	service := &pluginServiceImpl{
		pluginRepo: pluginRepo,
		toolRepo:   p1PublishToolRepo{},
		codeRepo:   codeRepo,
	}

	_, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.ErrorIs(t, err, ErrAPPCodePluginPublishUnavailable)
	require.Equal(t, []int64{20}, pluginRepo.published)
	require.Empty(t, codeRepo.compensated)
	require.Empty(t, codeRepo.ensured)
}

func TestPublishAPPPluginsOrdinarySuccessRemainsUnchanged(t *testing.T) {
	pluginRepo := &p1SuccessfulPluginRepo{
		p1AmbiguousPluginRepo: p1AmbiguousPluginRepo{
			drafts: []*entity.PluginInfo{
				p1BatchPlugin(20, common.PluginType_PLUGIN),
			},
		},
	}
	service := &pluginServiceImpl{
		pluginRepo: pluginRepo,
		toolRepo:   p1PublishToolRepo{},
		codeRepo:   &p1AmbiguousCodeRepo{},
	}

	response, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.NoError(t, err)
	require.Len(t, response.AllDraftPlugins, 1)
	require.Equal(t, []int64{20}, pluginRepo.published)
}

func TestPublishAPPPluginsCompensatesAllUncommittedAndReturnsUnavailable(t *testing.T) {
	codeRepo := &p1AmbiguousCodeRepo{published: map[int64]bool{}}
	service := p1AmbiguousPublishService(codeRepo)

	_, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.ErrorIs(t, err, ErrAPPCodePluginPublishUnavailable)
	require.ElementsMatch(t, []int64{21, 22}, codeRepo.compensated)
	require.Empty(t, codeRepo.ensured)
}

func TestPublishAPPPluginsMixedCommitFailsClosedWithoutDeletingPublishedSnapshot(t *testing.T) {
	codeRepo := &p1AmbiguousCodeRepo{published: map[int64]bool{21: true, 22: false}}
	service := p1AmbiguousPublishService(codeRepo)

	_, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.ErrorIs(t, err, ErrAPPCodePluginPublishUnavailable)
	require.ElementsMatch(t, []int64{21, 22}, codeRepo.compensated)
	require.Equal(t, []int64{21}, codeRepo.ensured)
}

func TestPublishAPPPluginsUnknownCommitFailsClosedAfterCheckingEveryFUNC(t *testing.T) {
	codeRepo := &p1AmbiguousCodeRepo{
		published:     map[int64]bool{21: true},
		compensateErr: map[int64]error{22: errors.New("database unavailable")},
	}
	service := p1AmbiguousPublishService(codeRepo)

	_, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.ErrorIs(t, err, ErrAPPCodePluginPublishUnavailable)
	require.ElementsMatch(t, []int64{21, 22}, codeRepo.compensated)
	require.Equal(t, []int64{21}, codeRepo.ensured)
}

func p1AmbiguousPublishService(codeRepo *p1AmbiguousCodeRepo) *pluginServiceImpl {
	return &pluginServiceImpl{
		pluginRepo: &p1AmbiguousPluginRepo{drafts: []*entity.PluginInfo{
			p1BatchPlugin(21, common.PluginType_FUNC),
			p1BatchPlugin(22, common.PluginType_FUNC),
		}},
		codeRepo: codeRepo,
	}
}

func p1BatchPlugin(id int64, pluginType common.PluginType) *entity.PluginInfo {
	manifest := model.NewDefaultPluginManifest()
	return entity.NewPluginInfo(&model.PluginInfo{
		ID:          id,
		APPID:       func() *int64 { value := int64(9); return &value }(),
		SpaceID:     2001,
		DeveloperID: 88,
		PluginType:  pluginType,
		Manifest:    manifest,
		OpenapiDoc:  model.NewDefaultOpenapiDoc(),
	})
}
