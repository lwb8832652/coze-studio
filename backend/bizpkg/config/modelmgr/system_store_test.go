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

package modelmgr

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
)

func TestSystemModelManagementPersistsEncryptedEndpointsAndAccessGrants(t *testing.T) {
	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	draft := newSystemModelTestInput("system-secret")

	modelID, err := cfg.UpsertSystemModel(ctx, 9, nil, draft)
	require.NoError(t, err)
	require.Positive(t, modelID)

	models, total, err := cfg.ListSystemModels(ctx, &config.GetModelListReq{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, models, 1)
	require.Equal(t, "DeepSeek V4 Pro", models[0].Name)
	require.Equal(t, config.ModelAccessMode_ALL, models[0].AccessMode)

	detail, err := cfg.GetSystemModelDetail(ctx, modelID)
	require.NoError(t, err)
	require.Len(t, detail.Endpoints, 1)
	require.True(t, detail.Endpoints[0].HasAPIKey)
	require.NotContains(t, detail.Endpoints[0].BaseURL, "system-secret")

	var stored systemModelEndpointRow
	require.NoError(t, cfg.db.Table(modelEndpointTable).Where("model_id = ?", modelID).First(&stored).Error)
	require.NotEqual(t, "system-secret", stored.APIKeyEnvelope)

	require.NoError(t, cfg.SaveSystemModelGrants(ctx, modelID, config.ModelAccessMode_RESTRICTED, []*config.ModelGrantSubject{
		{SubjectType: config.ModelGrantSubjectType_WORKSPACE, SubjectID: 101},
		{SubjectType: config.ModelGrantSubjectType_USER, SubjectID: 9},
	}))
	mode, grants, err := cfg.GetSystemModelGrants(ctx, modelID)
	require.NoError(t, err)
	require.Equal(t, config.ModelAccessMode_RESTRICTED, mode)
	require.Len(t, grants, 2)

	require.NoError(t, cfg.SetSystemModelStatus(ctx, modelID, false))
	disabled := false
	models, total, err = cfg.ListSystemModels(ctx, &config.GetModelListReq{Enabled: &disabled})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.False(t, models[0].Enabled)

	preview, err := cfg.DeleteSystemModel(ctx, modelID, true)
	require.NoError(t, err)
	require.Empty(t, preview)
	_, err = cfg.GetSystemModelDetail(ctx, modelID)
	require.NoError(t, err)
	require.NoError(t, func() error {
		_, deleteErr := cfg.DeleteSystemModel(ctx, modelID, false)
		return deleteErr
	}())
	_, err = cfg.GetSystemModelDetail(ctx, modelID)
	require.ErrorIs(t, err, ErrSystemModelNotFound)
}

func TestSystemModelManagementEnablesDatabaseModelList(t *testing.T) {
	cfg := newWorkspaceModelTestConfig(t)
	staleReaderCtx := ctxcache.Init(context.Background())

	useOldModels, err := cfg.UseOldModelConf(staleReaderCtx)
	require.NoError(t, err)
	require.True(t, useOldModels)

	_, err = cfg.UpsertSystemModel(context.Background(), 9, nil, newSystemModelTestInput("system-secret"))
	require.NoError(t, err)

	useOldModels, err = cfg.UseOldModelConf(staleReaderCtx)
	require.NoError(t, err)
	require.False(t, useOldModels)
}

func TestSystemModelUpdatePreservesWriteOnlyCredential(t *testing.T) {
	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	draft := newSystemModelTestInput("system-secret")
	modelID, err := cfg.UpsertSystemModel(ctx, 9, nil, draft)
	require.NoError(t, err)
	detail, err := cfg.GetSystemModelDetail(ctx, modelID)
	require.NoError(t, err)

	draft.Name = "DeepSeek V4 Pro Updated"
	draft.Endpoints[0].ID = ptr.Of(detail.Endpoints[0].ID)
	draft.Endpoints[0].APIKey = nil
	_, err = cfg.UpsertSystemModel(ctx, 9, &modelID, draft)
	require.NoError(t, err)

	var stored systemModelEndpointRow
	require.NoError(t, cfg.db.Table(modelEndpointTable).Where("model_id = ?", modelID).First(&stored).Error)
	plain, err := cfg.credentialCodec.Decrypt(modelID, stored.ID, stored.APIKeyEnvelope)
	require.NoError(t, err)
	require.Equal(t, "system-secret", plain)
}

func TestSystemModelManagementProjectsAndMigratesLegacyRows(t *testing.T) {
	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	providerJSON, err := marshalManagedModelJSON(&config.ModelProvider{
		ModelClass: developer_api.ModelClass_DeekSeek,
	})
	require.NoError(t, err)
	displayJSON, err := marshalManagedModelJSON(&config.DisplayInfo{Name: "Legacy DeepSeek"})
	require.NoError(t, err)
	connectionJSON, err := marshalManagedModelJSON(&config.Connection{
		BaseConnInfo: &config.BaseConnectionInfo{
			Model: "deepseek-v4-pro", APIKey: "legacy-system-secret", ThinkingType: config.ThinkingType_Enable,
		},
	})
	require.NoError(t, err)
	capabilityJSON, err := marshalManagedModelJSON(&developer_api.ModelAbility{
		CotDisplay: ptr.Of(true), FunctionCall: ptr.Of(true), ImageUnderstanding: ptr.Of(true),
	})
	require.NoError(t, err)
	require.NoError(t, cfg.db.Exec(
		`INSERT INTO model_instance (id, provider, display_info, connection, capability, parameters, extra, access_mode, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '[]', '{}', 'all', 1, 2)`,
		100002, providerJSON, displayJSON, connectionJSON, capabilityJSON,
	).Error)

	models, total, err := cfg.ListSystemModels(ctx, &config.GetModelListReq{})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.ElementsMatch(t, []string{"image", "reasoning", "text"}, models[0].CapabilityTypes)

	detail, err := cfg.GetSystemModelDetail(ctx, 100002)
	require.NoError(t, err)
	require.Equal(t, int64(128000), detail.MaxContextTokens)
	require.Equal(t, int64(4096), detail.MaxOutputTokens)
	require.Equal(t, "enabled", detail.ReasoningMode)
	require.Equal(t, "native", detail.FunctionCallMode)
	require.ElementsMatch(t, []string{"agent", "appdev", "chat", "workflow"}, detail.UsageScenarios)
	require.Len(t, detail.Endpoints, 1)
	require.Equal(t, "https://api.deepseek.com/v1", detail.Endpoints[0].BaseURL)
	require.True(t, detail.Endpoints[0].HasAPIKey)

	legacyUpdate := newSystemModelTestInput("")
	legacyUpdate.Name = detail.Summary.Name
	legacyUpdate.ModelIdentifier = detail.Summary.ModelIdentifier
	legacyUpdate.CapabilityTypes = detail.Summary.CapabilityTypes
	legacyUpdate.ReasoningMode = detail.ReasoningMode
	legacyUpdate.MaxContextTokens = detail.MaxContextTokens
	legacyUpdate.MaxOutputTokens = detail.MaxOutputTokens
	legacyUpdate.FunctionCallMode = detail.FunctionCallMode
	legacyUpdate.UsageScenarios = detail.UsageScenarios
	legacyUpdate.Endpoints[0].BaseURL = detail.Endpoints[0].BaseURL
	legacyUpdate.Endpoints[0].APIKey = nil
	modelID := int64(100002)
	_, err = cfg.UpsertSystemModel(ctx, 9, &modelID, legacyUpdate)
	require.NoError(t, err)

	var stored systemModelEndpointRow
	require.NoError(t, cfg.db.Table(modelEndpointTable).Where("model_id = ?", modelID).First(&stored).Error)
	plain, err := cfg.credentialCodec.Decrypt(modelID, stored.ID, stored.APIKeyEnvelope)
	require.NoError(t, err)
	require.Equal(t, "legacy-system-secret", plain)
}

func newSystemModelTestInput(secret string) *config.ModelManagementInput {
	return &config.ModelManagementInput{
		ProviderKey: "deepseek", Name: "DeepSeek V4 Pro", ModelIdentifier: "deepseek-v4-pro",
		Description: ptr.Of("system reasoning model"), CapabilityTypes: []string{"text", "reasoning"},
		ReasoningMode: "enabled", MaxContextTokens: 128000, MaxOutputTokens: 8192,
		FunctionCallMode: "native", Enabled: true, UsageScenarios: []string{"chat", "agent"},
		Protocol: "openai-compatible", RoutingStrategy: config.ModelRoutingStrategy_ROUND_ROBIN,
		AccessMode: config.ModelAccessMode_ALL,
		Endpoints: []*config.ModelEndpointInput{{
			BaseURL: "https://api.example.com/v1", APIKey: ptr.Of(secret), Weight: 1, Enabled: true, SortOrder: 0,
		}},
		EnableBase64URL: ptr.Of(false),
	}
}
