// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	mockPlugin "github.com/coze-dev/coze-studio/backend/internal/mock/domain/plugin"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestCodePluginSaveAndDebug(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	codeRepo := &codePluginRepoStub{}
	runner := &codeRunnerStub{response: &coderunner.RunResponse{Result: map[string]any{"answer": "ok"}}}
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		codeRepo:    codeRepo,
		codeRunner:  runner,
	}
	ctx := codePluginTestContext(88)
	plugin := entity.NewPluginInfo(&model.PluginInfo{
		ID:          1001,
		PluginType:  common.PluginType_FUNC,
		SpaceID:     2001,
		DeveloperID: 88,
	})
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(2)
	inputSchema := `{"properties":{"question":{"type":"string"}},"required":["question"],"type":"object"}`
	outputSchema := `{"properties":{"answer":{"type":"string"}},"required":["answer"],"type":"object"}`

	saved, err := service.SaveCodePluginDraft(ctx, &pluginAPI.SaveCodePluginDraftRequest{
		PluginID:         plugin.ID,
		SpaceID:          plugin.SpaceID,
		Revision:         0,
		Runtime:          common.CodePluginRuntime_Python,
		EntryFile:        "main.py",
		InputSchemaJSON:  &inputSchema,
		OutputSchemaJSON: &outputSchema,
		Files: []*common.CodePluginFile{
			{Path: "main.py", Content: "async def main(args):\n    return {'answer': 'ok'}"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), saved.Data.Revision)
	require.Equal(t, inputSchema, saved.Data.InputSchemaJSON)
	require.Equal(t, outputSchema, saved.Data.OutputSchemaJSON)

	debugged, err := service.DebugCodePlugin(ctx, &pluginAPI.DebugCodePluginRequest{
		PluginID:        plugin.ID,
		SpaceID:         plugin.SpaceID,
		Revision:        saved.Data.Revision,
		ArgumentsInJSON: `{"question":"hello"}`,
	})
	require.NoError(t, err)
	require.True(t, debugged.Data.Success)
	require.Equal(t, common.CodePluginDebugStatus_Success, debugged.Data.Status)
	require.Equal(t, coderunner.PurposePlugin, runner.request.Purpose)
	require.Equal(t, int64(1), codeRepo.draft.LastDebuggedRevision)
}

func TestCodePluginDebugRejectsInvalidInputBeforeRunner(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	draft := codePluginDraftWithSchemas(t, 1003, 2003,
		`{"type":"object","required":["question"],"properties":{"question":{"type":"string"}}}`,
		entity.DefaultCodeSchemaJSON,
	)
	codeRepo := &codePluginRepoStub{draft: draft}
	runner := &codeRunnerStub{response: &coderunner.RunResponse{Result: map[string]any{"answer": "ok"}}}
	service := &PluginApplicationService{DomainSVC: domain, spaceAccess: codePluginOwnerSpaceAccess(), codeRepo: codeRepo, codeRunner: runner}
	ctx := codePluginTestContext(88)
	plugin := codePluginInfo(1003, 2003, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := service.DebugCodePlugin(ctx, &pluginAPI.DebugCodePluginRequest{
		PluginID:        plugin.ID,
		SpaceID:         plugin.SpaceID,
		Revision:        draft.Revision,
		ArgumentsInJSON: `{"question":123,"secret":"TOP_SECRET_INPUT"}`,
	})
	require.ErrorContains(t, err, "input validation failed")
	require.ErrorContains(t, err, "/question")
	require.NotContains(t, err.Error(), "TOP_SECRET_INPUT")
	require.Nil(t, runner.request)
	require.Zero(t, codeRepo.markDebuggedCalls)
}

func TestCodePluginDebugRejectsInvalidOutputWithoutMarkingDebugged(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	draft := codePluginDraftWithSchemas(t, 1004, 2004,
		`{"type":"object","required":["question"],"properties":{"question":{"type":"string"}}}`,
		`{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`,
	)
	codeRepo := &codePluginRepoStub{draft: draft}
	runner := &codeRunnerStub{response: &coderunner.RunResponse{
		Result: map[string]any{"answer": 42, "secret": "TOP_SECRET_OUTPUT"},
	}}
	service := &PluginApplicationService{DomainSVC: domain, spaceAccess: codePluginOwnerSpaceAccess(), codeRepo: codeRepo, codeRunner: runner}
	ctx := codePluginTestContext(88)
	plugin := codePluginInfo(1004, 2004, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := service.DebugCodePlugin(ctx, &pluginAPI.DebugCodePluginRequest{
		PluginID:        plugin.ID,
		SpaceID:         plugin.SpaceID,
		Revision:        draft.Revision,
		ArgumentsInJSON: `{"question":"hello"}`,
	})
	require.ErrorContains(t, err, "output validation failed")
	require.ErrorContains(t, err, "/answer")
	require.NotContains(t, err.Error(), "TOP_SECRET_OUTPUT")
	require.NotNil(t, runner.request)
	require.Zero(t, codeRepo.markDebuggedCalls)
	require.Zero(t, codeRepo.draft.LastDebuggedRevision)
}

func TestCodePluginSavePreservesSchemasWhenOptionalFieldsAreAbsent(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	existing := codePluginDraftWithSchemas(t, 1005, 2005,
		`{"type":"object","properties":{"query":{"type":"string"}}}`,
		`{"type":"object","properties":{"result":{"type":"string"}}}`,
	)
	existing.Revision = 3
	codeRepo := &codePluginRepoStub{draft: existing}
	service := &PluginApplicationService{DomainSVC: domain, spaceAccess: codePluginOwnerSpaceAccess(), codeRepo: codeRepo}
	ctx := codePluginTestContext(88)
	plugin := codePluginInfo(1005, 2005, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	saved, err := service.SaveCodePluginDraft(ctx, &pluginAPI.SaveCodePluginDraftRequest{
		PluginID:  plugin.ID,
		SpaceID:   plugin.SpaceID,
		Revision:  existing.Revision,
		Runtime:   common.CodePluginRuntime_Python,
		EntryFile: "main.py",
		Files: []*common.CodePluginFile{
			{Path: "main.py", Content: "async def main(args):\n    return args.params"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, existing.InputSchemaJSON, saved.Data.InputSchemaJSON)
	require.Equal(t, existing.OutputSchemaJSON, saved.Data.OutputSchemaJSON)
	require.Equal(t, existing.InputSchemaJSON, codeRepo.draft.InputSchemaJSON)
	require.Equal(t, existing.OutputSchemaJSON, codeRepo.draft.OutputSchemaJSON)
}

func TestCodePluginDataMapsSchemas(t *testing.T) {
	t.Parallel()
	draft := codePluginDraftWithSchemas(t, 1006, 2006,
		`{"type":"object","properties":{"input":{"type":"string"}}}`,
		`{"type":"object","properties":{"output":{"type":"string"}}}`,
	)
	draftData := codeDraftData(draft)
	require.Equal(t, draft.InputSchemaJSON, draftData.InputSchemaJSON)
	require.Equal(t, draft.OutputSchemaJSON, draftData.OutputSchemaJSON)

	version := &entity.CodeVersion{
		PluginID:         draft.PluginID,
		SpaceID:          draft.SpaceID,
		Version:          "1.0.0",
		Runtime:          draft.Runtime,
		EntryFile:        draft.EntryFile,
		InputSchemaJSON:  draft.InputSchemaJSON,
		OutputSchemaJSON: draft.OutputSchemaJSON,
		SourceRevision:   draft.Revision,
		CreatedBy:        88,
		Files:            draft.Files,
	}
	versionData := codeVersionData(version)
	require.Equal(t, version.InputSchemaJSON, versionData.InputSchemaJSON)
	require.Equal(t, version.OutputSchemaJSON, versionData.OutputSchemaJSON)
}

func TestCodePluginDebugRejectsRemoteReferenceAndExcessiveSchemaDepth(t *testing.T) {
	t.Parallel()
	deepSchema := `{"type":"object"}`
	for range 20 {
		deepSchema = `{"type":"object","properties":{"nested":` + deepSchema + `}}`
	}
	testCases := []struct {
		name   string
		schema string
	}{
		{name: "remote reference", schema: `{"$ref":"https://TOP_SECRET.example/schema.json"}`},
		{name: "excessive depth", schema: deepSchema},
	}
	for index, testCase := range testCases {
		index, testCase := index, testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			pluginID := int64(1010 + index)
			spaceID := int64(2010 + index)
			draft := codePluginDraftWithSchemas(t, pluginID, spaceID, testCase.schema, entity.DefaultCodeSchemaJSON)
			codeRepo := &codePluginRepoStub{draft: draft}
			runner := &codeRunnerStub{response: &coderunner.RunResponse{Result: map[string]any{}}}
			service := &PluginApplicationService{DomainSVC: domain, spaceAccess: codePluginOwnerSpaceAccess(), codeRepo: codeRepo, codeRunner: runner}
			ctx := codePluginTestContext(88)
			plugin := codePluginInfo(pluginID, spaceID, 88)
			domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

			_, err := service.DebugCodePlugin(ctx, &pluginAPI.DebugCodePluginRequest{
				PluginID:        plugin.ID,
				SpaceID:         plugin.SpaceID,
				Revision:        draft.Revision,
				ArgumentsInJSON: `{}`,
			})
			require.ErrorContains(t, err, "input schema")
			require.NotContains(t, err.Error(), "TOP_SECRET")
			require.Nil(t, runner.request)
			require.Zero(t, codeRepo.markDebuggedCalls)
		})
	}
}

func TestSaveCodePluginDraftRejectsSchemaBeforeRepositoryWrite(t *testing.T) {
	tests := []struct {
		name   string
		schema string
	}{
		{name: "remote reference", schema: `{"type":"object","properties":{"value":{"$ref":"https://example.com/schema.json"}}}`},
		{name: "excessive depth", schema: `{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"object","properties":{"c":{"type":"object","properties":{"d":{"type":"object","properties":{"e":{"type":"object","properties":{"f":{"type":"object","properties":{"g":{"type":"object","properties":{"h":{"type":"object","properties":{"i":{"type":"object","properties":{"j":{"type":"object","properties":{"k":{"type":"object","properties":{"l":{"type":"object","properties":{"m":{"type":"object","properties":{"n":{"type":"object","properties":{"o":{"type":"object","properties":{"p":{"type":"object","properties":{"q":{"type":"string"}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			codeRepo := &codePluginRepoStub{}
			plugin := codePluginInfo(1701, 2701, 88)
			domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
			service := &PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: codePluginOwnerSpaceAccess(),
				codeRepo:    codeRepo,
			}
			defaultSchema := entity.DefaultCodeSchemaJSON
			_, err := service.SaveCodePluginDraft(codePluginTestContext(88), &pluginAPI.SaveCodePluginDraftRequest{
				PluginID:         plugin.ID,
				SpaceID:          plugin.SpaceID,
				Runtime:          common.CodePluginRuntime_Python,
				EntryFile:        "main.py",
				Files:            []*common.CodePluginFile{{Path: "main.py", Content: defaultPythonCode}},
				InputSchemaJSON:  &test.schema,
				OutputSchemaJSON: &defaultSchema,
			})
			require.ErrorIs(t, err, ErrCodePluginValidation)
			require.Nil(t, codeRepo.draft)
		})
	}
}

func TestCodePluginRejectsSpaceMismatchAndMultipleFiles(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	service := &PluginApplicationService{DomainSVC: domain, spaceAccess: codePluginOwnerSpaceAccess(), codeRepo: &codePluginRepoStub{}}
	ctx := codePluginTestContext(88)
	plugin := entity.NewPluginInfo(&model.PluginInfo{
		ID:          1002,
		PluginType:  common.PluginType_FUNC,
		SpaceID:     2002,
		DeveloperID: 88,
	})
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(2)

	_, err := service.GetCodePluginDraft(ctx, &pluginAPI.GetCodePluginDraftRequest{
		PluginID: plugin.ID,
		SpaceID:  plugin.SpaceID + 1,
	})
	require.ErrorContains(t, err, "requested space")

	_, err = service.SaveCodePluginDraft(ctx, &pluginAPI.SaveCodePluginDraftRequest{
		PluginID:  plugin.ID,
		SpaceID:   plugin.SpaceID,
		Runtime:   common.CodePluginRuntime_Python,
		EntryFile: "main.py",
		Files: []*common.CodePluginFile{
			{Path: "main.py", Content: "first"},
			{Path: "helper.py", Content: "second"},
		},
	})
	require.ErrorContains(t, err, "exactly one")
}

func TestCodeDebugFailureDoesNotExposeProviderError(t *testing.T) {
	t.Parallel()
	resp := codeDebugFailure(7, coderunner.ErrCodeRunnerUnavailable, 0)
	require.False(t, resp.Data.Success)
	require.Equal(t, common.CodePluginDebugStatus_Unavailable, resp.Data.Status)
	require.Equal(t, "code sandbox is unavailable", resp.Data.Reason)
}

func TestCodePluginDebugUnavailableReturnsTypedServiceError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	draft := codePluginDraftWithSchemas(t, 1099, 2099, entity.DefaultCodeSchemaJSON, entity.DefaultCodeSchemaJSON)
	codeRepo := &codePluginRepoStub{draft: draft}
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		codeRepo:    codeRepo,
	}
	plugin := codePluginInfo(1099, 2099, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := service.DebugCodePlugin(codePluginTestContext(88), &pluginAPI.DebugCodePluginRequest{
		PluginID:        plugin.ID,
		SpaceID:         plugin.SpaceID,
		Revision:        draft.Revision,
		ArgumentsInJSON: `{}`,
	})
	require.ErrorIs(t, err, ErrCodePluginUnavailable)
}

func codePluginTestContext(userID int64) context.Context {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userEntity.Session{UserID: userID})
	return ctx
}

func codePluginInfo(pluginID, spaceID, developerID int64) *entity.PluginInfo {
	return entity.NewPluginInfo(&model.PluginInfo{
		ID:          pluginID,
		PluginType:  common.PluginType_FUNC,
		SpaceID:     spaceID,
		DeveloperID: developerID,
	})
}

func codePluginDraftWithSchemas(t *testing.T, pluginID, spaceID int64, inputSchema, outputSchema string) *entity.CodeDraft {
	t.Helper()
	draft, err := entity.PrepareCodeDraft(&entity.CodeDraft{
		PluginID:         pluginID,
		SpaceID:          spaceID,
		Runtime:          entity.CodeRuntimePython,
		EntryFile:        "main.py",
		InputSchemaJSON:  inputSchema,
		OutputSchemaJSON: outputSchema,
		Files: []*entity.CodeFile{{
			Path:    "main.py",
			Content: []byte("async def main(args):\n    return args.params"),
		}},
	})
	require.NoError(t, err)
	draft.Revision = 1
	return draft
}

type codeRunnerStub struct {
	request  *coderunner.RunRequest
	response *coderunner.RunResponse
	err      error
}

func (r *codeRunnerStub) Run(_ context.Context, request *coderunner.RunRequest) (*coderunner.RunResponse, error) {
	r.request = request
	return r.response, r.err
}

type codePluginRepoStub struct {
	draft             *entity.CodeDraft
	version           *entity.CodeVersion
	markDebuggedCalls int
}

func (r *codePluginRepoStub) GetDraft(context.Context, int64) (*entity.CodeDraft, bool, error) {
	return r.draft, r.draft != nil, nil
}

func (r *codePluginRepoStub) SaveDraftCAS(_ context.Context, draft *entity.CodeDraft, expectedRevision int64) (*entity.CodeDraft, error) {
	if r.draft != nil && r.draft.Revision != expectedRevision {
		return nil, repository.ErrCodeDraftConflict
	}
	prepared, err := entity.PrepareCodeDraft(draft)
	if err != nil {
		return nil, err
	}
	prepared.Revision = expectedRevision + 1
	if r.draft != nil {
		prepared.LastDebuggedRevision = r.draft.LastDebuggedRevision
	}
	r.draft = prepared
	return prepared, nil
}

func (r *codePluginRepoStub) MarkDebuggedCAS(_ context.Context, _ int64, revision int64) error {
	r.markDebuggedCalls++
	if r.draft == nil || r.draft.Revision != revision {
		return repository.ErrCodeDraftConflict
	}
	r.draft.LastDebuggedRevision = revision
	return nil
}

func (r *codePluginRepoStub) PublishDebuggedVersion(context.Context, int64, string, int64) error {
	return nil
}

func (r *codePluginRepoStub) GetVersion(context.Context, int64, string) (*entity.CodeVersion, bool, error) {
	return r.version, r.version != nil, nil
}

func (r *codePluginRepoStub) CopyDraft(context.Context, int64, int64, int64) error {
	return nil
}

func (r *codePluginRepoStub) DeletePluginData(context.Context, int64) error {
	return nil
}
