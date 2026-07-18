// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pluginAPI "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop"
	common "github.com/coze-dev/coze-studio/backend/api/model/plugin_develop/common"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/consts"
	"github.com/coze-dev/coze-studio/backend/crossdomain/plugin/model"
	pluginConf "github.com/coze-dev/coze-studio/backend/domain/plugin/conf"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/dto"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/entity"
	"github.com/coze-dev/coze-studio/backend/domain/plugin/repository"
	searchEntity "github.com/coze-dev/coze-studio/backend/domain/search/entity"
	search "github.com/coze-dev/coze-studio/backend/domain/search/service"
	userEntity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
	mockPlugin "github.com/coze-dev/coze-studio/backend/internal/mock/domain/plugin"
)

func TestRegisterPluginMetaAllowsOnlySupportedCreationCombinations(t *testing.T) {
	tests := []struct {
		name           string
		pluginType     common.PluginType
		creationMethod common.CreationMethod
	}{
		{
			name:           "code plugin cannot use coze creation",
			pluginType:     common.PluginType_FUNC,
			creationMethod: common.CreationMethod_COZE,
		},
		{
			name:           "http plugin cannot use ide creation",
			pluginType:     common.PluginType_PLUGIN,
			creationMethod: common.CreationMethod_IDE,
		},
		{
			name:           "other plugin types are rejected",
			pluginType:     common.PluginType_APP,
			creationMethod: common.CreationMethod_COZE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			service := &PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: codePluginOwnerSpaceAccess(),
				eventbus:    &codePluginEventBusStub{},
				codeRepo:    &codePluginLifecycleRepoStub{},
			}

			_, err := service.RegisterPluginMeta(
				codePluginTestContext(88),
				newRegisterPluginMetaRequest(tt.pluginType, tt.creationMethod),
			)
			require.Error(t, err)
		})
	}
}

func TestRegisterPluginMetaNormalPluginKeepsAuthorizationContract(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}
	req := newRegisterPluginMetaRequest(common.PluginType_PLUGIN, common.CreationMethod_COZE)
	req.AuthType = nil
	pluginURL := "https://plugins.example.test"
	req.URL = &pluginURL

	_, err := service.RegisterPluginMeta(codePluginTestContext(88), req)
	require.ErrorContains(t, err, "auth type")
}

func TestRegisterPluginMetaNormalPluginRequiresURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	domain.EXPECT().
		CreateDraftPlugin(gomock.Any(), gomock.Any()).
		Return(int64(1000), nil).
		AnyTimes()
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}
	req := newRegisterPluginMetaRequest(common.PluginType_PLUGIN, common.CreationMethod_COZE)
	req.URL = nil

	_, err := service.RegisterPluginMeta(codePluginTestContext(88), req)
	require.ErrorContains(t, err, "url")
}

func TestRegisterPluginMetaCodePluginRejectsHTTPAuthorization(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
	authType := common.AuthorizationType_Service
	req.AuthType = &authType

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    &codePluginLifecycleRepoStub{},
	}).RegisterPluginMeta(codePluginTestContext(88), req)
	require.ErrorContains(t, err, "auth type")
}

func TestRegisterPluginMetaCodePluginRejectsHTTPConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*pluginAPI.RegisterPluginMetaRequest)
	}{
		{name: "server url", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) { req.URL = stringPointer("https://private.example") }},
		{name: "common params", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) {
			req.CommonParams = map[common.ParameterLocation][]*common.CommonParamSchema{
				common.ParameterLocation_Header: {},
			}
		}},
		{name: "fixed export ip", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) {
			value := true
			req.FixedExportIP = &value
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
			req.AuthType = nil
			test.mutate(req)
			_, err := (&PluginApplicationService{
				eventbus:    &codePluginEventBusStub{},
				codeRepo:    &codePluginLifecycleRepoStub{},
				spaceAccess: codePluginOwnerSpaceAccess(),
			}).RegisterPluginMeta(codePluginTestContext(88), req)
			require.ErrorIs(t, err, ErrCodePluginInvalidRequest)
		})
	}
}

func TestRegisterPluginMetaRequiresRealEditableSpaceRole(t *testing.T) {
	tests := []struct {
		name    string
		members []*userEntity.SpaceMember
		wantErr bool
	}{
		{name: "non member", wantErr: true},
		{name: "read only member", members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Member)}}, wantErr: true},
		{name: "admin", members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Admin)}}},
		{name: "owner", members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Owner)}}},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			if !test.wantErr {
				domain.EXPECT().CreateDraftPlugin(gomock.Any(), gomock.Any()).Return(int64(5000+index), nil)
			}
			req := newRegisterPluginMetaRequest(common.PluginType_PLUGIN, common.CreationMethod_COZE)
			url := "https://plugin.example"
			req.URL = &url
			_, err := (&PluginApplicationService{
				DomainSVC:   domain,
				eventbus:    &codePluginEventBusStub{},
				spaceAccess: &codePluginSpaceAccessStub{members: test.members},
			}).RegisterPluginMeta(codePluginTestContext(88), req)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestRegisterPluginMetaCommittedCreateSurvivesResourceEventFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	domain.EXPECT().CreateDraftPlugin(gomock.Any(), gomock.Any()).Return(int64(5100), nil).Times(1)
	req := newRegisterPluginMetaRequest(common.PluginType_PLUGIN, common.CreationMethod_COZE)
	url := "https://plugin.example"
	req.URL = &url
	eventbus := &codePluginEventBusStub{err: errors.New("search unavailable")}
	resp, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
	}).RegisterPluginMeta(codePluginTestContext(88), req)
	require.NoError(t, err)
	require.Equal(t, int64(5100), resp.PluginID)
	require.Len(t, eventbus.events, 1)
}

func TestRegisterPluginMetaCodePluginValidatesBasicMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*pluginAPI.RegisterPluginMetaRequest)
	}{
		{name: "name", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) { req.Name = "" }},
		{name: "description", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) { req.Desc = "" }},
		{name: "icon", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) { req.Icon.URI = "" }},
		{name: "space", mutate: func(req *pluginAPI.RegisterPluginMetaRequest) { req.SpaceID = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
			req.AuthType = nil
			tt.mutate(req)

			_, err := (&PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: codePluginOwnerSpaceAccess(),
				eventbus:    &codePluginEventBusStub{},
				codeRepo:    &codePluginLifecycleRepoStub{},
			}).RegisterPluginMeta(codePluginTestContext(88), req)
			require.Error(t, err)
		})
	}
}

func TestUpdateCodePluginMetaRejectsAuthorizationBypassFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*pluginAPI.UpdatePluginMetaRequest)
	}{
		{
			name: "service authorization",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				value := common.AuthorizationType_Service
				req.AuthType = &value
			},
		},
		{
			name: "oauth authorization",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				value := common.AuthorizationType_OAuth
				req.AuthType = &value
			},
		},
		{
			name: "service token with none authorization",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				authType := common.AuthorizationType_None
				token := "secret-token"
				req.AuthType = &authType
				req.ServiceToken = &token
			},
		},
		{
			name: "oauth payload without authorization type",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				payload := `{"client_id":"secret-client"}`
				req.OauthInfo = &payload
			},
		},
		{
			name: "http authorization location",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				location := common.AuthorizationServiceLocation_Header
				req.Location = &location
			},
		},
		{
			name: "server url",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				req.URL = stringPointer("https://private.example")
			},
		},
		{
			name: "fixed export ip",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				value := true
				req.FixedExportIP = &value
			},
		},
		{
			name: "authorization key",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				key := "Authorization"
				req.Key = &key
			},
		},
		{
			name: "authorization subtype",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				subType := int32(1)
				req.SubAuthType = &subType
			},
		},
		{
			name: "authorization payload",
			mutate: func(req *pluginAPI.UpdatePluginMetaRequest) {
				payload := `{"secret":"value"}`
				req.AuthPayload = &payload
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			plugin := newLifecyclePluginInfo(1150, common.PluginType_FUNC, 88)
			domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
			domain.EXPECT().UpdateDraftPlugin(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

			req := &pluginAPI.UpdatePluginMetaRequest{PluginID: plugin.ID}
			tt.mutate(req)
			_, err := (&PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: codePluginOwnerSpaceAccess(),
				eventbus:    &codePluginEventBusStub{},
			}).UpdatePluginMeta(codePluginTestContext(88), req)
			require.ErrorContains(t, err, "code plugin")
		})
	}
}

func TestUpdateCodePluginMetaForcesNoneAuthorization(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1151, common.PluginType_FUNC, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	var updated *dto.UpdateDraftPluginRequest
	domain.EXPECT().
		UpdateDraftPlugin(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *dto.UpdateDraftPluginRequest) error {
			updated = req
			return nil
		})
	name := "Updated code plugin"

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}).UpdatePluginMeta(codePluginTestContext(88), &pluginAPI.UpdatePluginMetaRequest{
		PluginID: plugin.ID,
		Name:     &name,
	})

	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.AuthInfo)
	require.NotNil(t, updated.AuthInfo.AuthzType)
	require.Equal(t, consts.AuthzTypeOfNone, *updated.AuthInfo.AuthzType)
}

func TestRegisterPluginMetaCodePluginInitializesRuntimeTemplate(t *testing.T) {
	tests := []struct {
		name        string
		runtime     *string
		wantRuntime entity.CodeRuntime
		wantEntry   string
		wantSource  string
	}{
		{name: "missing defaults to python", wantRuntime: entity.CodeRuntimePython, wantEntry: "main.py", wantSource: "main(args)"},
		{name: "numeric python", runtime: stringPointer("1"), wantRuntime: entity.CodeRuntimePython, wantEntry: "main.py", wantSource: "main(args)"},
		{name: "named python", runtime: stringPointer("python"), wantRuntime: entity.CodeRuntimePython, wantEntry: "main.py", wantSource: "main(args)"},
		{name: "numeric javascript", runtime: stringPointer("2"), wantRuntime: entity.CodeRuntimeJavaScript, wantEntry: "main.js", wantSource: "main({ params })"},
		{name: "named javascript", runtime: stringPointer("javascript"), wantRuntime: entity.CodeRuntimeJavaScript, wantEntry: "main.js", wantSource: "main({ params })"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			codeRepo := &codePluginLifecycleRepoStub{}
			eventbus := &codePluginEventBusStub{}
			var created *dto.CreateDraftPluginRequest

			domain.EXPECT().
				CreateDraftPlugin(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, req *dto.CreateDraftPluginRequest) (int64, error) {
					created = req
					return 1001, nil
				})

			req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
			req.AuthType = nil
			req.URL = nil
			req.IdeCodeRuntime = tt.runtime
			resp, err := (&PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: codePluginOwnerSpaceAccess(),
				eventbus:    eventbus,
				codeRepo:    codeRepo,
			}).RegisterPluginMeta(codePluginTestContext(88), req)

			require.NoError(t, err)
			require.Equal(t, int64(1001), resp.PluginID)
			require.NotNil(t, created)
			require.NotNil(t, created.AuthInfo)
			require.NotNil(t, created.AuthInfo.AuthzType)
			require.Equal(t, consts.AuthzTypeOfNone, *created.AuthInfo.AuthzType)
			require.Empty(t, created.ServerURL)
			require.Equal(t, 1, codeRepo.saveCalls)
			require.Equal(t, int64(0), codeRepo.expectedRevision)
			require.Equal(t, tt.wantRuntime, codeRepo.savedDraft.Runtime)
			require.Equal(t, tt.wantEntry, codeRepo.savedDraft.EntryFile)
			require.Len(t, codeRepo.savedDraft.Files, 1)
			require.Equal(t, tt.wantEntry, codeRepo.savedDraft.Files[0].Path)
			require.Contains(t, string(codeRepo.savedDraft.Files[0].Content), tt.wantSource)
			require.Len(t, eventbus.events, 1)
		})
	}
}

func TestRegisterPluginMetaCodePluginRejectsUnsupportedRuntimeBeforeCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
	req.AuthType = nil
	req.IdeCodeRuntime = stringPointer("ruby")

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    &codePluginLifecycleRepoStub{},
	}).RegisterPluginMeta(codePluginTestContext(88), req)
	require.ErrorContains(t, err, "runtime")
}

func TestRegisterPluginMetaCodePluginCompensatesInitializationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	initErr := errors.New("initialize source failed")
	compensationErr := errors.New("compensation failed")
	codeRepo := &codePluginLifecycleRepoStub{saveErr: initErr}
	req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
	req.AuthType = nil

	domain.EXPECT().CreateDraftPlugin(gomock.Any(), gomock.Any()).Return(int64(1002), nil)
	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), int64(1002)).Return(compensationErr)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}).RegisterPluginMeta(codePluginTestContext(88), req)
	require.ErrorIs(t, err, initErr)
	require.NotErrorIs(t, err, compensationErr)
	require.Equal(t, 1, codeRepo.saveCalls)
}

func TestRegisterPluginMetaCodePluginFailsClosedWithoutRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	req := newRegisterPluginMetaRequest(common.PluginType_FUNC, common.CreationMethod_IDE)
	req.AuthType = nil

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}).RegisterPluginMeta(codePluginTestContext(88), req)
	require.ErrorContains(t, err, "repository")
}

func TestCodePluginOAuthSchemaIncludesRuntimeChoices(t *testing.T) {
	resp, err := (&PluginApplicationService{}).GetOAuthSchema(
		context.Background(),
		&pluginAPI.GetOAuthSchemaRequest{},
	)
	require.NoError(t, err)
	require.Equal(t, pluginConf.GetOAuthSchema(), resp.OauthSchema)

	var config map[string]struct {
		Default string `json:"default"`
		Options []struct {
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"options"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.IdeConf), &config))

	runtimes, exists := config["code_runtime_enum"]
	require.True(t, exists)
	require.Equal(t, "1", runtimes.Default)
	values := make(map[string]struct{}, len(runtimes.Options))
	for _, option := range runtimes.Options {
		require.NotEmpty(t, option.Label)
		values[option.Value] = struct{}{}
	}
	require.Contains(t, values, "1")
	require.Contains(t, values, "2")
	require.Contains(t, values, runtimes.Default)
}

func TestPluginInfoProjectsCreationMethodByPluginType(t *testing.T) {
	tests := []struct {
		name       string
		pluginType common.PluginType
		want       common.CreationMethod
	}{
		{name: "code plugin", pluginType: common.PluginType_FUNC, want: common.CreationMethod_IDE},
		{name: "http plugin", pluginType: common.PluginType_PLUGIN, want: common.CreationMethod_COZE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			plugin := newLifecyclePluginInfo(1101, tt.pluginType, 88)
			domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
			resp, err := (&PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: codePluginOwnerSpaceAccess(),
				oss:         &codePluginStorageStub{},
				toolRepo:    &codePluginToolRepoStub{},
				pluginRepo:  &codePluginPluginRepoStub{},
			}).GetPluginInfo(codePluginTestContext(88), &pluginAPI.GetPluginInfoRequest{PluginID: plugin.ID})

			require.NoError(t, err)
			require.Equal(t, tt.want, resp.CreationMethod)
		})
	}
}

func TestPublishPluginRejectsUndebuggedCodePlugin(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1201, common.PluginType_FUNC, 88)
	draft := newPublishableCodeDraft(plugin, 3)
	draft.LastDebuggedRevision = 2
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    &codePluginLifecycleRepoStub{draft: draft},
	}).PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorContains(t, err, "debug")
}

func TestPublishPluginCodePluginFailsClosedWithoutRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1202, common.PluginType_FUNC, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}).PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorContains(t, err, "repository")
}

func TestPublishPluginCodePluginCreatesSnapshotBeforeDomainPublish(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1203, common.PluginType_FUNC, 88)
	operations := []string{}
	codeRepo := &codePluginLifecycleRepoStub{
		draft:      newPublishableCodeDraft(plugin, 4),
		operations: &operations,
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	domain.EXPECT().
		PublishPlugin(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *model.PublishPluginRequest) error {
			operations = append(operations, "domain-publish")
			return nil
		})

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}).PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))

	require.NoError(t, err)
	require.Equal(t, []string{"create-version", "domain-publish", "ensure-version"}, operations)
	require.Equal(t, int64(88), codeRepo.createdBy)
	require.Equal(t, "v1.0.0", codeRepo.createdVersion)
}

func TestPublishPluginCodePluginRetriesMatchingSnapshotIdempotently(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1204, common.PluginType_FUNC, 88)
	draft := newPublishableCodeDraft(plugin, 5)
	codeRepo := &codePluginLifecycleRepoStub{draft: draft}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(2)
	gomock.InOrder(
		domain.EXPECT().PublishPlugin(gomock.Any(), gomock.Any()).Return(errors.New("domain publish failed")),
		domain.EXPECT().PublishPlugin(gomock.Any(), gomock.Any()).Return(nil),
	)
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}

	_, firstErr := service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorIs(t, firstErr, ErrCodePluginUnavailable)
	require.NotContains(t, firstErr.Error(), "domain publish failed")
	_, retryErr := service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.NoError(t, retryErr)
	require.Equal(t, 2, codeRepo.createVersionCalls)
	require.Equal(t, 0, codeRepo.getVersionCalls)
}

func TestPublishPluginCodePluginCompensationFailureIsRetryable(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1210, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		draft:         newPublishableCodeDraft(plugin, 7),
		compensateErr: errors.New("temporary compensation failure"),
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(2)
	gomock.InOrder(
		domain.EXPECT().PublishPlugin(gomock.Any(), gomock.Any()).Return(errors.New("publish failed")),
		domain.EXPECT().PublishPlugin(gomock.Any(), gomock.Any()).Return(nil),
	)
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}
	_, err := service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorIs(t, err, ErrCodePluginUnavailable)
	codeRepo.compensateErr = nil
	_, err = service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.NoError(t, err)
	require.Equal(t, 2, codeRepo.createVersionCalls)
	require.Equal(t, 1, codeRepo.ensureCalls)
}

func TestPublishPluginCodePluginAmbiguousFailureKeepsPublishedSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1211, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		draft:     newPublishableCodeDraft(plugin, 8),
		published: true,
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	resp, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}).PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, codeRepo.version)
	require.Equal(t, 1, codeRepo.ensureCalls)
}

func TestPublishPluginCodePluginRetriesAfterEnsureFailureWithoutRepublishing(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1212, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		draft:     newPublishableCodeDraft(plugin, 9),
		ensureErr: errors.New("temporary ensure failure"),
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(2)
	domain.EXPECT().
		PublishPlugin(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *model.PublishPluginRequest) error {
			codeRepo.domainPublishCalls++
			codeRepo.published = true
			return nil
		}).
		Times(1)
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}

	_, err := service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorIs(t, err, ErrCodePluginUnavailable)

	codeRepo.ensureErr = nil
	_, err = service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.NoError(t, err)
	require.Equal(t, 1, codeRepo.domainPublishCalls)
	require.Equal(t, 2, codeRepo.ensureCalls)
}

func TestPublishPluginCodePluginRetriesAfterCompensationLookupFailureWithoutRepublishing(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1213, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		draft:         newPublishableCodeDraft(plugin, 10),
		compensateErr: errors.New("temporary published-marker lookup failure"),
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil).Times(2)
	domain.EXPECT().
		PublishPlugin(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, *model.PublishPluginRequest) error {
			codeRepo.domainPublishCalls++
			codeRepo.published = true
			return errors.New("commit result unknown")
		}).
		Times(1)
	service := &PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}

	_, err := service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorIs(t, err, ErrCodePluginUnavailable)

	codeRepo.compensateErr = nil
	_, err = service.PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.NoError(t, err)
	require.Equal(t, 1, codeRepo.domainPublishCalls)
	require.Equal(t, 1, codeRepo.ensureCalls)
}

func TestPublishPluginCodePluginRejectsMismatchedExistingVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1205, common.PluginType_FUNC, 88)
	draft := newPublishableCodeDraft(plugin, 6)
	codeRepo := &codePluginLifecycleRepoStub{
		draft:     draft,
		published: true,
		version: &entity.CodeVersion{
			PluginID:        plugin.ID,
			Version:         "v1.0.0",
			SourceRevision:  draft.Revision - 1,
			SourceBundleRef: "different-bundle",
		},
	}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}).PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.ErrorIs(t, err, repository.ErrCodeVersionExists)
}

func TestPublishPluginNormalPluginDoesNotRequireCodeRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1206, common.PluginType_PLUGIN, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	domain.EXPECT().PublishPlugin(gomock.Any(), gomock.Any()).Return(nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}).PublishPlugin(codePluginTestContext(88), newPublishPluginRequest(plugin.ID))
	require.NoError(t, err)
}

func TestCodePluginDeleteKeepsCodeWhenMainDeleteFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1301, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{draft: newPublishableCodeDraft(plugin, 1)}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), plugin.ID).Return(errors.New("main delete failed"))

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
		codeRepo:    codeRepo,
	}).DelPlugin(codePluginTestContext(88), &pluginAPI.DelPluginRequest{PluginID: plugin.ID})
	require.ErrorContains(t, err, "main delete failed")
	require.Equal(t, 0, codeRepo.deleteCalls)
}

func TestCodePluginDeleteReliesOnDatabaseCascadeWithoutCodeRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1304, common.PluginType_FUNC, 88)
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)

	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), plugin.ID).Return(nil)
	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    &codePluginEventBusStub{},
	}).DelPlugin(codePluginTestContext(88), &pluginAPI.DelPluginRequest{PluginID: plugin.ID})
	require.NoError(t, err)
}

func TestCodePluginDeleteDoesNotRunPostCommitPseudoCleanup(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1302, common.PluginType_FUNC, 88)
	operations := []string{}
	codeRepo := &codePluginLifecycleRepoStub{
		draft:      newPublishableCodeDraft(plugin, 1),
		operations: &operations,
	}
	eventbus := &codePluginEventBusStub{operations: &operations}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	domain.EXPECT().
		DeleteDraftPlugin(gomock.Any(), plugin.ID).
		DoAndReturn(func(context.Context, int64) error {
			operations = append(operations, "domain-delete")
			return nil
		})

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).DelPlugin(codePluginTestContext(88), &pluginAPI.DelPluginRequest{PluginID: plugin.ID})
	require.NoError(t, err)
	require.Equal(t, []string{"domain-delete", "resource-event"}, operations)
	require.Zero(t, codeRepo.deleteCalls)
}

func TestCodePluginDeleteCommittedDeleteSurvivesResourceEventFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	plugin := newLifecyclePluginInfo(1303, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		draft: newPublishableCodeDraft(plugin, 1),
	}
	eventbus := &codePluginEventBusStub{err: errors.New("search unavailable")}
	domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), plugin.ID).Return(nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).DelPlugin(codePluginTestContext(88), &pluginAPI.DelPluginRequest{PluginID: plugin.ID})
	require.NoError(t, err)
	require.Equal(t, 0, codeRepo.deleteCalls)
	require.Len(t, eventbus.events, 1)
}

func TestGetUserAuthorityUsesSessionOwnershipAndSpaceRole(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		developer int64
		members   []*userEntity.SpaceMember
		wantErr   bool
		canRead   bool
		canEdit   bool
	}{
		{name: "no session", ctx: context.Background(), developer: 88, wantErr: true},
		{name: "non member", ctx: codePluginTestContext(88), developer: 88},
		{name: "member owner is read only", ctx: codePluginTestContext(88), developer: 88, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Member)}}, canRead: true},
		{name: "admin non developer can edit", ctx: codePluginTestContext(88), developer: 99, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Admin)}}, canRead: true, canEdit: true},
		{name: "admin developer can edit", ctx: codePluginTestContext(88), developer: 88, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Admin)}}, canRead: true, canEdit: true},
		{name: "developer demoted to member is read only", ctx: codePluginTestContext(88), developer: 88, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Member)}}, canRead: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			if !test.wantErr {
				domain.EXPECT().GetDraftPlugin(gomock.Any(), int64(1601)).
					Return(newLifecyclePluginInfo(1601, common.PluginType_FUNC, test.developer), nil)
			}
			resp, err := (&PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: &codePluginSpaceAccessStub{members: test.members},
			}).GetUserAuthority(test.ctx, &pluginAPI.GetUserAuthorityRequest{PluginID: 1601})
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.canRead, resp.Data.CanRead)
			require.Equal(t, test.canRead, resp.Data.CanReadChangelog)
			require.Equal(t, test.canEdit, resp.Data.CanEdit)
			require.Equal(t, test.canEdit, resp.Data.CanDelete)
			require.Equal(t, test.canEdit, resp.Data.CanDebug)
			require.Equal(t, test.canEdit, resp.Data.CanPublish)
		})
	}
}

func TestValidateDraftPluginAccessUsesSpaceRoleInsteadOfDeveloperOwnership(t *testing.T) {
	tests := []struct {
		name      string
		developer int64
		members   []*userEntity.SpaceMember
		wantErr   bool
	}{
		{name: "owner manages another developer plugin", developer: 99, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Owner)}}},
		{name: "admin manages another developer plugin", developer: 99, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Admin)}}},
		{name: "developer demoted to member cannot write", developer: 88, members: []*userEntity.SpaceMember{{UserID: 88, RoleType: int32(common.SpaceRoleType_Member)}}, wantErr: true},
		{name: "developer removed from space cannot write", developer: 88, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			domain := mockPlugin.NewMockPluginService(ctrl)
			plugin := newLifecyclePluginInfo(1602, common.PluginType_FUNC, test.developer)
			domain.EXPECT().GetDraftPlugin(gomock.Any(), plugin.ID).Return(plugin, nil)
			service := &PluginApplicationService{
				DomainSVC:   domain,
				spaceAccess: &codePluginSpaceAccessStub{members: test.members},
			}

			_, err := service.validateDraftPluginAccess(codePluginTestContext(88), plugin.ID)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCopyCodePluginCopiesDraftAndResetsLifecycleState(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	sourcePlugin := newLifecyclePluginInfo(1401, common.PluginType_FUNC, 88)
	targetPlugin := newLifecyclePluginInfo(2401, common.PluginType_FUNC, 88)
	sourceDraft := newPublishableCodeDraft(sourcePlugin, 7)
	sourceDraft.Runtime = entity.CodeRuntimeJavaScript
	sourceDraft.EntryFile = defaultJSEntry
	sourceDraft.Files = []*entity.CodeFile{
		{Path: defaultJSEntry, Content: []byte(defaultJSCode)},
	}
	sourceDraft.InputSchemaJSON = `{"type":"object","required":["query"],"properties":{"query":{"type":"string"}}}`
	sourceDraft.OutputSchemaJSON = `{"type":"object","required":["answer"],"properties":{"answer":{"type":"string"}}}`
	codeRepo := &codePluginLifecycleRepoStub{draft: sourceDraft}
	eventbus := &codePluginEventBusStub{}
	domain.EXPECT().
		CopyPlugin(gomock.Any(), gomock.Any()).
		Return(&dto.CopyPluginResponse{
			Plugin: targetPlugin,
			Tools:  map[int64]*entity.ToolInfo{},
		}, nil)

	resp, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).CopyPlugin(context.Background(), &dto.CopyPluginRequest{
		UserID:    88,
		PluginID:  sourcePlugin.ID,
		CopyScene: consts.CopySceneOfDuplicate,
	})

	require.NoError(t, err)
	require.Same(t, targetPlugin, resp.Plugin)
	require.Equal(t, 1, codeRepo.copyCalls)
	require.Equal(t, sourcePlugin.ID, codeRepo.copySourcePluginID)
	require.Equal(t, targetPlugin.ID, codeRepo.copyTargetPluginID)
	require.Equal(t, targetPlugin.SpaceID, codeRepo.copyTargetSpaceID)
	require.NotNil(t, codeRepo.copiedDraft)
	require.Equal(t, entity.CodeRuntimeJavaScript, codeRepo.copiedDraft.Runtime)
	require.Equal(t, defaultJSEntry, codeRepo.copiedDraft.EntryFile)
	require.JSONEq(t, sourceDraft.InputSchemaJSON, codeRepo.copiedDraft.InputSchemaJSON)
	require.JSONEq(t, sourceDraft.OutputSchemaJSON, codeRepo.copiedDraft.OutputSchemaJSON)
	require.Equal(t, int64(1), codeRepo.copiedDraft.Revision)
	require.Zero(t, codeRepo.copiedDraft.LastDebuggedRevision)
	require.False(t, targetPlugin.Published())
	require.Len(t, eventbus.events, 1)
}

func TestCopyCodePluginReturnsSuccessWhenPostCommitEventFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	sourcePlugin := newLifecyclePluginInfo(1410, common.PluginType_FUNC, 88)
	targetPlugin := newLifecyclePluginInfo(2410, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{draft: newPublishableCodeDraft(sourcePlugin, 3)}
	eventbus := &codePluginEventBusStub{err: errors.New("search unavailable")}
	domain.EXPECT().
		CopyPlugin(gomock.Any(), gomock.Any()).
		Return(&dto.CopyPluginResponse{Plugin: targetPlugin, Tools: map[int64]*entity.ToolInfo{}}, nil)

	resp, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).CopyPlugin(context.Background(), &dto.CopyPluginRequest{
		UserID:    88,
		PluginID:  sourcePlugin.ID,
		CopyScene: consts.CopySceneOfDuplicate,
	})

	require.NoError(t, err)
	require.Equal(t, targetPlugin.ID, resp.Plugin.ID)
	require.Equal(t, 1, codeRepo.copyCalls)
	require.Len(t, eventbus.events, 1)
}

func TestCopyCodePluginMissingSourceCompensatesTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	targetPlugin := newLifecyclePluginInfo(2402, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{}
	eventbus := &codePluginEventBusStub{}
	domain.EXPECT().
		CopyPlugin(gomock.Any(), gomock.Any()).
		Return(&dto.CopyPluginResponse{Plugin: targetPlugin}, nil)
	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), targetPlugin.ID).Return(nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).CopyPlugin(context.Background(), &dto.CopyPluginRequest{
		UserID:    88,
		PluginID:  1402,
		CopyScene: consts.CopySceneOfDuplicate,
	})

	require.ErrorContains(t, err, "copy code plugin source failed")
	require.Equal(t, 1, codeRepo.copyCalls)
	require.Empty(t, eventbus.events)
}

func TestCopyCodePluginFailureCompensatesAndRedactsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	targetPlugin := newLifecyclePluginInfo(2403, common.PluginType_FUNC, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		copyErr: errors.New("mysql password=do-not-expose"),
	}
	eventbus := &codePluginEventBusStub{}
	domain.EXPECT().
		CopyPlugin(gomock.Any(), gomock.Any()).
		Return(&dto.CopyPluginResponse{Plugin: targetPlugin}, nil)
	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), targetPlugin.ID).Return(nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).CopyPlugin(context.Background(), &dto.CopyPluginRequest{
		UserID:    88,
		PluginID:  1403,
		CopyScene: consts.CopySceneOfDuplicate,
	})

	require.ErrorContains(t, err, "copy code plugin source failed")
	require.NotContains(t, err.Error(), "password")
	require.NotContains(t, err.Error(), "do-not-expose")
	require.Equal(t, 1, codeRepo.copyCalls)
	require.Empty(t, eventbus.events)
}

func TestCopyPublishedCodePluginTargetCompensatesWithoutCopyingDraft(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	targetPlugin := newLifecyclePluginInfo(2404, common.PluginType_FUNC, 88)
	version := "v0.0.1"
	targetPlugin.Version = &version
	codeRepo := &codePluginLifecycleRepoStub{
		draft: newPublishableCodeDraft(newLifecyclePluginInfo(1404, common.PluginType_FUNC, 88), 1),
	}
	eventbus := &codePluginEventBusStub{}
	domain.EXPECT().
		CopyPlugin(gomock.Any(), gomock.Any()).
		Return(&dto.CopyPluginResponse{Plugin: targetPlugin}, nil)
	domain.EXPECT().DeleteDraftPlugin(gomock.Any(), targetPlugin.ID).Return(nil)

	_, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).CopyPlugin(context.Background(), &dto.CopyPluginRequest{
		UserID:    88,
		PluginID:  1404,
		CopyScene: consts.CopySceneOfToLibrary,
	})

	require.ErrorContains(t, err, "published state")
	require.Equal(t, 0, codeRepo.copyCalls)
	require.Empty(t, eventbus.events)
}

func TestCopyNormalPluginDoesNotCreateCodeDraft(t *testing.T) {
	ctrl := gomock.NewController(t)
	domain := mockPlugin.NewMockPluginService(ctrl)
	targetPlugin := newLifecyclePluginInfo(2405, common.PluginType_PLUGIN, 88)
	codeRepo := &codePluginLifecycleRepoStub{
		copyErr: errors.New("must not be called"),
	}
	eventbus := &codePluginEventBusStub{}
	domain.EXPECT().
		CopyPlugin(gomock.Any(), gomock.Any()).
		Return(&dto.CopyPluginResponse{
			Plugin: targetPlugin,
			Tools:  map[int64]*entity.ToolInfo{},
		}, nil)

	resp, err := (&PluginApplicationService{
		DomainSVC:   domain,
		spaceAccess: codePluginOwnerSpaceAccess(),
		eventbus:    eventbus,
		codeRepo:    codeRepo,
	}).CopyPlugin(context.Background(), &dto.CopyPluginRequest{
		UserID:    88,
		PluginID:  1405,
		CopyScene: consts.CopySceneOfDuplicate,
	})

	require.NoError(t, err)
	require.Same(t, targetPlugin, resp.Plugin)
	require.Equal(t, 0, codeRepo.copyCalls)
	require.Len(t, eventbus.events, 1)
}

func newRegisterPluginMetaRequest(pluginType common.PluginType, creationMethod common.CreationMethod) *pluginAPI.RegisterPluginMetaRequest {
	authType := common.AuthorizationType_None
	return &pluginAPI.RegisterPluginMetaRequest{
		Name:           "Lifecycle plugin",
		Desc:           "Lifecycle plugin description",
		Icon:           &common.PluginIcon{URI: "plugin-icon"},
		AuthType:       &authType,
		SpaceID:        2001,
		CreationMethod: &creationMethod,
		PluginType:     &pluginType,
	}
}

func newPublishPluginRequest(pluginID int64) *pluginAPI.PublishPluginRequest {
	return &pluginAPI.PublishPluginRequest{
		PluginID:    pluginID,
		VersionName: "v1.0.0",
		VersionDesc: "initial release",
	}
}

func newLifecyclePluginInfo(pluginID int64, pluginType common.PluginType, developerID int64) *entity.PluginInfo {
	iconURI := "plugin-icon"
	serverURL := ""
	manifest := model.NewDefaultPluginManifest()
	manifest.NameForHuman = "Lifecycle plugin"
	manifest.NameForModel = "lifecycle_plugin"
	manifest.DescriptionForHuman = "Lifecycle plugin description"
	manifest.DescriptionForModel = "Lifecycle plugin description"
	manifest.LogoURL = iconURI
	return entity.NewPluginInfo(&model.PluginInfo{
		ID:          pluginID,
		PluginType:  pluginType,
		SpaceID:     2001,
		DeveloperID: developerID,
		IconURI:     &iconURI,
		ServerURL:   &serverURL,
		Manifest:    manifest,
		OpenapiDoc:  model.NewDefaultOpenapiDoc(),
	})
}

func newPublishableCodeDraft(plugin *entity.PluginInfo, revision int64) *entity.CodeDraft {
	return &entity.CodeDraft{
		PluginID:             plugin.ID,
		SpaceID:              plugin.SpaceID,
		Runtime:              entity.CodeRuntimePython,
		EntryFile:            defaultPythonEntry,
		SourceBundleRef:      "bundle-ref",
		Revision:             revision,
		LastDebuggedRevision: revision,
		Files: []*entity.CodeFile{
			{Path: defaultPythonEntry, Content: []byte(defaultPythonCode)},
		},
	}
}

func stringPointer(value string) *string {
	return &value
}

type codePluginEventBusStub struct {
	search.ResourceEventBus
	events     []*searchEntity.ResourceDomainEvent
	err        error
	operations *[]string
}

func (s *codePluginEventBusStub) PublishResources(_ context.Context, event *searchEntity.ResourceDomainEvent) error {
	s.events = append(s.events, event)
	if s.operations != nil {
		*s.operations = append(*s.operations, "resource-event")
	}
	return s.err
}

type codePluginStorageStub struct {
	storage.Storage
}

func (s *codePluginStorageStub) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) {
	return "https://assets.example.test/plugin-icon", nil
}

type codePluginToolRepoStub struct {
	repository.ToolRepository
}

func (s *codePluginToolRepoStub) GetPluginAllDraftTools(context.Context, int64, ...repository.ToolSelectedOptions) ([]*entity.ToolInfo, error) {
	return []*entity.ToolInfo{}, nil
}

type codePluginPluginRepoStub struct {
	repository.PluginRepository
}

func (s *codePluginPluginRepoStub) GetOnlinePlugin(context.Context, int64, ...repository.PluginSelectedOptions) (*entity.PluginInfo, bool, error) {
	return nil, false, nil
}

type codePluginLifecycleRepoStub struct {
	draft              *entity.CodeDraft
	version            *entity.CodeVersion
	saveErr            error
	deleteErr          error
	copyErr            error
	saveCalls          int
	createVersionCalls int
	getVersionCalls    int
	deleteCalls        int
	copyCalls          int
	expectedRevision   int64
	savedDraft         *entity.CodeDraft
	copiedDraft        *entity.CodeDraft
	createdVersion     string
	createdBy          int64
	copySourcePluginID int64
	copyTargetPluginID int64
	copyTargetSpaceID  int64
	operations         *[]string
	compensateErr      error
	ensureErr          error
	published          bool
	compensateCalls    int
	ensureCalls        int
	domainPublishCalls int
}

func (r *codePluginLifecycleRepoStub) GetDraft(context.Context, int64) (*entity.CodeDraft, bool, error) {
	return r.draft, r.draft != nil, nil
}

func (r *codePluginLifecycleRepoStub) SaveDraftCAS(_ context.Context, draft *entity.CodeDraft, expectedRevision int64) (*entity.CodeDraft, error) {
	r.saveCalls++
	r.expectedRevision = expectedRevision
	r.savedDraft = draft
	if r.saveErr != nil {
		return nil, r.saveErr
	}
	prepared, err := entity.PrepareCodeDraft(draft)
	if err != nil {
		return nil, err
	}
	prepared.Revision = expectedRevision + 1
	r.draft = prepared
	return prepared, nil
}

func (r *codePluginLifecycleRepoStub) MarkDebuggedCAS(context.Context, int64, int64) error {
	return nil
}

func (r *codePluginLifecycleRepoStub) PublishDebuggedVersion(_ context.Context, pluginID int64, version string, operatorID int64) error {
	r.createVersionCalls++
	r.createdVersion = version
	r.createdBy = operatorID
	if r.operations != nil {
		*r.operations = append(*r.operations, "create-version")
	}
	if r.draft == nil {
		return repository.ErrCodeDraftNotFound
	}
	if r.draft.Revision <= 0 ||
		r.draft.SourceBundleRef == "" ||
		r.draft.LastDebuggedRevision != r.draft.Revision {
		return repository.ErrCodeDraftNotDebugged
	}
	if r.version != nil {
		if r.version.SourceRevision == r.draft.Revision &&
			r.version.SourceBundleRef == r.draft.SourceBundleRef {
			return nil
		}
		if r.published {
			return repository.ErrCodeVersionExists
		}
		r.version = nil
	}
	r.version = &entity.CodeVersion{
		PluginID:        pluginID,
		SpaceID:         r.draft.SpaceID,
		Version:         version,
		Runtime:         r.draft.Runtime,
		EntryFile:       r.draft.EntryFile,
		SourceBundleRef: r.draft.SourceBundleRef,
		SourceRevision:  r.draft.Revision,
		CreatedBy:       operatorID,
		Files:           entity.CloneCodeFiles(r.draft.Files),
	}
	return nil
}

func (r *codePluginLifecycleRepoStub) PrepareDebuggedVersion(
	ctx context.Context,
	pluginID int64,
	version string,
	operatorID int64,
) (*repository.PreparedCodeVersion, error) {
	previous := r.version
	if err := r.PublishDebuggedVersion(ctx, pluginID, version, operatorID); err != nil {
		return nil, err
	}
	copyVersion := *r.version
	copyVersion.Files = entity.CloneCodeFiles(r.version.Files)
	return &repository.PreparedCodeVersion{
		Version: &copyVersion,
		Created: previous == nil || previous.SourceBundleRef != r.version.SourceBundleRef,
		State: func() repository.CodeVersionPreparationState {
			if r.published {
				return repository.CodeVersionAlreadyPublished
			}
			if previous != nil {
				return repository.CodeVersionRecovered
			}
			return repository.CodeVersionPrepared
		}(),
	}, nil
}

func (r *codePluginLifecycleRepoStub) CompensatePreparedVersion(
	_ context.Context,
	prepared *repository.PreparedCodeVersion,
) (bool, error) {
	r.compensateCalls++
	if r.compensateErr != nil {
		return false, r.compensateErr
	}
	if r.published {
		return true, nil
	}
	if prepared != nil && prepared.Created && r.version != nil &&
		r.version.SourceRevision == prepared.Version.SourceRevision &&
		r.version.SourceBundleRef == prepared.Version.SourceBundleRef {
		r.version = nil
	}
	return false, nil
}

func (r *codePluginLifecycleRepoStub) EnsurePublishedVersion(
	_ context.Context,
	prepared *repository.PreparedCodeVersion,
) error {
	r.ensureCalls++
	if r.operations != nil {
		*r.operations = append(*r.operations, "ensure-version")
	}
	if r.ensureErr != nil {
		return r.ensureErr
	}
	r.published = true
	if r.version == nil && prepared != nil && prepared.Version != nil {
		copyVersion := *prepared.Version
		copyVersion.Files = entity.CloneCodeFiles(prepared.Version.Files)
		r.version = &copyVersion
	}
	return nil
}

func (r *codePluginLifecycleRepoStub) GetVersion(context.Context, int64, string) (*entity.CodeVersion, bool, error) {
	r.getVersionCalls++
	return r.version, r.version != nil, nil
}

func (r *codePluginLifecycleRepoStub) CopyDraft(_ context.Context, sourcePluginID, targetPluginID, targetSpaceID int64) error {
	r.copyCalls++
	r.copySourcePluginID = sourcePluginID
	r.copyTargetPluginID = targetPluginID
	r.copyTargetSpaceID = targetSpaceID
	if r.copyErr != nil {
		return r.copyErr
	}
	if r.draft == nil || r.draft.PluginID != sourcePluginID {
		return repository.ErrCodeDraftNotFound
	}
	prepared, err := entity.PrepareCodeDraft(&entity.CodeDraft{
		PluginID:         targetPluginID,
		SpaceID:          targetSpaceID,
		Runtime:          r.draft.Runtime,
		EntryFile:        r.draft.EntryFile,
		InputSchemaJSON:  r.draft.InputSchemaJSON,
		OutputSchemaJSON: r.draft.OutputSchemaJSON,
		Files:            entity.CloneCodeFiles(r.draft.Files),
	})
	if err != nil {
		return err
	}
	prepared.Revision = 1
	prepared.LastDebuggedRevision = 0
	r.copiedDraft = prepared
	return nil
}

func (r *codePluginLifecycleRepoStub) DeletePluginData(context.Context, int64) error {
	r.deleteCalls++
	if r.operations != nil {
		*r.operations = append(*r.operations, "code-delete")
	}
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.draft = nil
	r.version = nil
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

type codePluginSpaceAccessStub struct {
	members []*userEntity.SpaceMember
	err     error
}

func (s *codePluginSpaceAccessStub) GetSpaceMembers(context.Context, int64) ([]*userEntity.SpaceMember, error) {
	return s.members, s.err
}

func codePluginOwnerSpaceAccess() pluginSpaceMemberReader {
	return &codePluginSpaceAccessStub{
		members: []*userEntity.SpaceMember{{
			UserID:   88,
			RoleType: int32(common.SpaceRoleType_Owner),
		}},
	}
}
