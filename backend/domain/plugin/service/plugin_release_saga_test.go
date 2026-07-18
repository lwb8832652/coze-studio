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

type batchCodeRepoStub struct {
	states      map[int64]repository.CodeVersionPreparationState
	prepared    []int64
	compensated []int64
	ensured     []int64
}

func (s *batchCodeRepoStub) GetDraft(context.Context, int64) (*entity.CodeDraft, bool, error) {
	return nil, false, nil
}
func (s *batchCodeRepoStub) SaveDraftCAS(context.Context, *entity.CodeDraft, int64) (*entity.CodeDraft, error) {
	return nil, errors.New("not implemented")
}
func (s *batchCodeRepoStub) MarkDebuggedCAS(context.Context, int64, int64) error {
	return errors.New("not implemented")
}
func (s *batchCodeRepoStub) PublishDebuggedVersion(context.Context, int64, string, int64) error {
	return errors.New("not implemented")
}
func (s *batchCodeRepoStub) GetVersion(context.Context, int64, string) (*entity.CodeVersion, bool, error) {
	return nil, false, nil
}
func (s *batchCodeRepoStub) CopyDraft(context.Context, int64, int64, int64) error {
	return errors.New("not implemented")
}
func (s *batchCodeRepoStub) DeletePluginData(context.Context, int64) error {
	return errors.New("not implemented")
}
func (s *batchCodeRepoStub) PrepareDebuggedVersion(_ context.Context, pluginID int64, version string, _ int64) (*repository.PreparedCodeVersion, error) {
	s.prepared = append(s.prepared, pluginID)
	state := s.states[pluginID]
	if state == "" {
		state = repository.CodeVersionPrepared
	}
	return &repository.PreparedCodeVersion{
		Version: &entity.CodeVersion{PluginID: pluginID, Version: version},
		Created: state == repository.CodeVersionPrepared,
		State:   state,
	}, nil
}
func (s *batchCodeRepoStub) CompensatePreparedVersion(_ context.Context, prepared *repository.PreparedCodeVersion) (bool, error) {
	s.compensated = append(s.compensated, prepared.Version.PluginID)
	return prepared.State == repository.CodeVersionAlreadyPublished, nil
}
func (s *batchCodeRepoStub) EnsurePublishedVersion(_ context.Context, prepared *repository.PreparedCodeVersion) error {
	s.ensured = append(s.ensured, prepared.Version.PluginID)
	return nil
}

func TestPrepareAPPCodePluginVersionsPairsEveryFUNCAndLeavesOrdinaryBatchBehavior(t *testing.T) {
	codeRepo := &batchCodeRepoStub{states: map[int64]repository.CodeVersionPreparationState{
		2: repository.CodeVersionAlreadyPublished,
	}}
	service := &pluginServiceImpl{codeRepo: codeRepo}
	plugins := []*entity.PluginInfo{
		entity.NewPluginInfo(&model.PluginInfo{ID: 1, PluginType: common.PluginType_PLUGIN}),
		entity.NewPluginInfo(&model.PluginInfo{ID: 2, PluginType: common.PluginType_FUNC, DeveloperID: 20}),
		entity.NewPluginInfo(&model.PluginInfo{ID: 3, PluginType: common.PluginType_FUNC, DeveloperID: 30}),
	}

	publishable, prepared, err := service.prepareAPPCodePluginVersions(context.Background(), "1.2.3", plugins)
	require.NoError(t, err)
	require.Equal(t, []int64{2, 3}, codeRepo.prepared)
	require.Len(t, prepared, 2)
	require.Equal(t, []int64{1, 3}, []int64{publishable[0].ID, publishable[1].ID})
	require.NoError(t, service.ensureAPPCodePluginVersions(context.Background(), prepared))
	require.Equal(t, []int64{2, 3}, codeRepo.ensured)
}

func TestPrepareAPPCodePluginVersionsCompensatesEarlierFUNCOnFailure(t *testing.T) {
	codeRepo := &batchCodeRepoStubWithFailure{
		batchCodeRepoStub: batchCodeRepoStub{states: map[int64]repository.CodeVersionPreparationState{}},
		failPluginID:      3,
	}
	service := &pluginServiceImpl{codeRepo: codeRepo}
	plugins := []*entity.PluginInfo{
		entity.NewPluginInfo(&model.PluginInfo{ID: 2, PluginType: common.PluginType_FUNC, DeveloperID: 20}),
		entity.NewPluginInfo(&model.PluginInfo{ID: 3, PluginType: common.PluginType_FUNC, DeveloperID: 30}),
	}

	_, _, err := service.prepareAPPCodePluginVersions(context.Background(), "1.2.3", plugins)
	require.Error(t, err)
	require.Equal(t, []int64{2}, codeRepo.compensated)
}

type batchCodeRepoStubWithFailure struct {
	batchCodeRepoStub
	failPluginID int64
}

func (s *batchCodeRepoStubWithFailure) PrepareDebuggedVersion(ctx context.Context, pluginID int64, version string, operatorID int64) (*repository.PreparedCodeVersion, error) {
	if pluginID == s.failPluginID {
		return nil, errors.New("database unavailable")
	}
	return s.batchCodeRepoStub.PrepareDebuggedVersion(ctx, pluginID, version, operatorID)
}

type batchPluginRepoStub struct {
	repository.PluginRepository
	drafts    []*entity.PluginInfo
	published []*entity.PluginInfo
}

func (s *batchPluginRepoStub) GetAPPAllDraftPlugins(context.Context, int64, ...repository.PluginSelectedOptions) ([]*entity.PluginInfo, error) {
	return s.drafts, nil
}

func (s *batchPluginRepoStub) MGetOnlinePlugins(context.Context, []int64, ...repository.PluginSelectedOptions) ([]*entity.PluginInfo, error) {
	return nil, nil
}

func (s *batchPluginRepoStub) UpdateDraftPlugin(context.Context, *entity.PluginInfo) error {
	return nil
}

func (s *batchPluginRepoStub) PublishPlugins(_ context.Context, plugins []*entity.PluginInfo) error {
	s.published = append(s.published, plugins...)
	return nil
}

func TestPublishAPPPluginsUsesCodeSagaAndSkipsAlreadyPublishedFUNCMainVersion(t *testing.T) {
	codeRepo := &batchCodeRepoStub{states: map[int64]repository.CodeVersionPreparationState{
		21: repository.CodeVersionAlreadyPublished,
		22: repository.CodeVersionPrepared,
	}}
	pluginRepo := &batchPluginRepoStub{drafts: []*entity.PluginInfo{
		batchCodePlugin(21),
		batchCodePlugin(22),
	}}
	service := &pluginServiceImpl{pluginRepo: pluginRepo, codeRepo: codeRepo}

	response, err := service.PublishAPPPlugins(context.Background(), &model.PublishAPPPluginsRequest{
		APPID: 9, Version: "1.2.3",
	})

	require.NoError(t, err)
	require.Len(t, response.AllDraftPlugins, 2)
	require.Equal(t, []int64{21, 22}, codeRepo.prepared)
	require.Equal(t, []int64{22}, []int64{pluginRepo.published[0].ID})
	require.Equal(t, []int64{21, 22}, codeRepo.ensured)
}

func batchCodePlugin(id int64) *entity.PluginInfo {
	manifest := model.NewDefaultPluginManifest()
	return entity.NewPluginInfo(&model.PluginInfo{
		ID:          id,
		APPID:       func() *int64 { value := int64(9); return &value }(),
		SpaceID:     2001,
		DeveloperID: 88,
		PluginType:  common.PluginType_FUNC,
		Manifest:    manifest,
		OpenapiDoc:  model.NewDefaultOpenapiDoc(),
	})
}
