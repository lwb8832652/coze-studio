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
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	config "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	workbenchmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/model"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

type workspaceModelTestCodec struct{}

func (workspaceModelTestCodec) Encrypt(modelID, endpointID int64, plaintext string) (string, string, error) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(plaintext))
	return fmt.Sprintf("sealed:%d:%d:%s", modelID, endpointID, payload), "fingerprint", nil
}

func (workspaceModelTestCodec) Decrypt(modelID, endpointID int64, envelope string) (string, error) {
	prefix := fmt.Sprintf("sealed:%d:%d:", modelID, endpointID)
	if !strings.HasPrefix(envelope, prefix) {
		return "", fmt.Errorf("invalid test envelope")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(envelope, prefix))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func (workspaceModelTestCodec) Rewrap(modelID, endpointID int64, envelope string) (string, string, bool, error) {
	plaintext, err := workspaceModelTestCodec{}.Decrypt(modelID, endpointID, envelope)
	if err != nil {
		return "", "", false, err
	}
	rewrapped, fingerprint, err := workspaceModelTestCodec{}.Encrypt(modelID, endpointID, plaintext)
	return rewrapped, fingerprint, true, err
}

func TestWorkspaceModelStoreEncryptsScopesAndSanitizes(t *testing.T) {
	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	secret := "sk-workspace-secret"
	draft := newWorkspaceModelTestDraft(secret)

	created, err := cfg.UpsertWorkspaceModel(ctx, 101, 7, nil, draft)
	require.NoError(t, err)
	require.Equal(t, workbenchmodel.WorkspaceModelScope_Space, created.Scope)
	require.True(t, created.CredentialConfigured)
	require.True(t, created.CanManage)
	require.Len(t, created.Endpoints, 1)
	require.True(t, created.Endpoints[0].CredentialConfigured)

	var endpoint managedModelEndpointRow
	require.NoError(t, cfg.db.Table(modelEndpointTable).First(&endpoint, created.Endpoints[0].ID).Error)
	require.NotEqual(t, secret, endpoint.APIKeyEnvelope)
	require.NotContains(t, endpoint.APIKeyEnvelope, `"api_key"`)

	var grantCount int64
	require.NoError(t, cfg.db.Table(modelGrantTable).
		Where("model_id = ? AND subject_type = ? AND subject_id = ? AND deleted_at IS NULL", created.ID, modelGrantSubjectSpace, 101).
		Count(&grantCount).Error)
	require.EqualValues(t, 1, grantCount)

	listed, err := cfg.ListWorkspaceModels(ctx, 101, true, "deepseek")
	require.NoError(t, err)
	require.Empty(t, listed.SystemModels)
	require.Len(t, listed.WorkspaceModels, 1)
	require.Equal(t, created.ID, listed.WorkspaceModels[0].ID)
	require.True(t, listed.WorkspaceModels[0].CredentialConfigured)
	require.True(t, listed.CanManage)

	previousEnvelope := endpoint.APIKeyEnvelope
	endpointID := created.Endpoints[0].ID
	draft.Endpoints[0].ID = &endpointID
	draft.Endpoints[0].APIKey = nil
	draft.Endpoints[0].BaseURL = "https://updated.example.com/v1"
	modelID := created.ID
	updated, err := cfg.UpsertWorkspaceModel(ctx, 101, 7, &modelID, draft)
	require.NoError(t, err)
	require.Equal(t, "https://updated.example.com/v1", updated.Endpoints[0].BaseURL)
	require.NoError(t, cfg.db.Table(modelEndpointTable).First(&endpoint, endpointID).Error)
	require.Equal(t, previousEnvelope, endpoint.APIKeyEnvelope)

	err = cfg.SetWorkspaceModelStatus(ctx, 202, modelID, false)
	require.ErrorIs(t, err, ErrWorkspaceModelNotFound)
	err = cfg.DeleteWorkspaceModel(ctx, 202, modelID)
	require.ErrorIs(t, err, ErrWorkspaceModelNotFound)

	available, err := cfg.AvailableModelIDs(ctx, 101)
	require.NoError(t, err)
	require.Contains(t, available, modelID)
	foreignAvailable, err := cfg.AvailableModelIDs(ctx, 202)
	require.NoError(t, err)
	require.NotContains(t, foreignAvailable, modelID)
	require.NoError(t, cfg.SetWorkspaceModelStatus(ctx, 101, modelID, false))
	available, err = cfg.AvailableModelIDs(ctx, 101)
	require.NoError(t, err)
	require.NotContains(t, available, modelID)
}

func TestWorkspaceModelStoreReadsSQLDatetimeRows(t *testing.T) {
	ctx := context.Background()
	cfg := newWorkspaceModelTestConfig(t)
	created, err := cfg.UpsertWorkspaceModel(ctx, 101, 7, nil, newWorkspaceModelTestDraft("sk-datetime"))
	require.NoError(t, err)
	require.Len(t, created.Endpoints, 1)

	databaseTime := time.Date(2026, time.July, 22, 10, 30, 0, 0, time.UTC)
	require.NoError(t, cfg.db.Table(modelEndpointTable).
		Where("id = ?", created.Endpoints[0].ID).
		Updates(map[string]any{"created_at": databaseTime, "updated_at": databaseTime}).Error)

	listed, err := cfg.ListWorkspaceModels(ctx, 101, true, "")
	require.NoError(t, err)
	require.Len(t, listed.WorkspaceModels, 1)

	var grantTimes struct {
		CreatedAt time.Time `gorm:"column:created_at"`
		UpdatedAt time.Time `gorm:"column:updated_at"`
	}
	require.NoError(t, cfg.db.Table(modelGrantTable).
		Where("model_id = ?", created.ID).
		Take(&grantTimes).Error)
	require.False(t, grantTimes.CreatedAt.IsZero())
	require.False(t, grantTimes.UpdatedAt.IsZero())
}

func TestPreMigrationSchemaKeepsLegacyModelsAvailable(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open("file:legacy_models?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE model_instance (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type INTEGER NOT NULL DEFAULT 0,
		provider TEXT NOT NULL,
		display_info TEXT NOT NULL,
		connection TEXT NOT NULL,
		capability TEXT NOT NULL,
		parameters TEXT NOT NULL,
		extra TEXT,
		created_at INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0,
		deleted_at DATETIME
	)`).Error)
	providerJSON, err := marshalManagedModelJSON(&config.ModelProvider{
		ModelClass: developer_api.ModelClass_DeekSeek,
	})
	require.NoError(t, err)
	displayJSON, err := marshalManagedModelJSON(&config.DisplayInfo{Name: "Legacy DeepSeek"})
	require.NoError(t, err)
	connectionJSON, err := marshalManagedModelJSON(&config.Connection{
		BaseConnInfo: &config.BaseConnectionInfo{
			Model: "deepseek-chat", BaseURL: "https://api.deepseek.com/v1", APIKey: "legacy-key",
		},
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		`INSERT INTO model_instance (id, provider, display_info, connection, capability, parameters, extra, created_at, updated_at)
		 VALUES (?, ?, ?, ?, '{}', '[]', '{}', 1, 2)`,
		100001, providerJSON, displayJSON, connectionJSON,
	).Error)
	cfg := &ModelConfig{db: db}

	connection := &config.Connection{BaseConnInfo: &config.BaseConnectionInfo{APIKey: "legacy-key"}}
	hydrated, err := cfg.hydrateManagedConnection(ctx, 100001, connection)
	require.NoError(t, err)
	require.Equal(t, "legacy-key", hydrated.BaseConnInfo.APIKey)

	available, err := cfg.AvailableModelIDs(ctx, 101)
	require.NoError(t, err)
	require.Contains(t, available, int64(100001))

	models, err := cfg.ListWorkspaceModels(ctx, 101, true, "")
	require.NoError(t, err)
	require.Len(t, models.SystemModels, 1)
	require.Empty(t, models.WorkspaceModels)
	require.False(t, models.CanManage)
	require.Equal(t, "Legacy DeepSeek", models.SystemModels[0].DisplayName)
}

func newWorkspaceModelTestConfig(t *testing.T) *ModelConfig {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	statements := []string{
		`CREATE TABLE kv_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			namespace TEXT NOT NULL,
			key_data TEXT NOT NULL,
			value_data BLOB NOT NULL,
			UNIQUE(namespace, key_data)
		)`,
		`CREATE TABLE model_instance (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type INTEGER NOT NULL DEFAULT 0,
			provider TEXT NOT NULL,
			display_info TEXT NOT NULL,
			connection TEXT NOT NULL,
			capability TEXT NOT NULL,
			parameters TEXT NOT NULL,
			extra TEXT,
			created_at INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0,
			deleted_at DATETIME,
			provider_key TEXT NOT NULL DEFAULT '',
			model_identifier TEXT NOT NULL DEFAULT '',
			description TEXT,
			status INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0,
			creator_id INTEGER NOT NULL DEFAULT 0,
			protocol TEXT NOT NULL DEFAULT '',
			routing_strategy TEXT NOT NULL DEFAULT 'priority',
			access_mode TEXT NOT NULL DEFAULT 'workspace',
			scenario_json TEXT,
			reasoning_mode TEXT NOT NULL DEFAULT '',
			function_call_mode TEXT NOT NULL DEFAULT '',
			max_context_tokens INTEGER NOT NULL DEFAULT 0,
			max_output_tokens INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE model_instance_endpoint (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id INTEGER NOT NULL,
			base_url TEXT NOT NULL,
			api_key_envelope TEXT NOT NULL,
			api_key_fingerprint TEXT NOT NULL DEFAULT '',
			weight INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME
		)`,
		`CREATE TABLE model_instance_grant (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id INTEGER NOT NULL,
			subject_type TEXT NOT NULL,
			subject_id INTEGER NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME
		)`,
	}
	for _, statement := range statements {
		require.NoError(t, db.Exec(statement).Error)
	}

	return &ModelConfig{
		db:              db,
		kv:              kvstore.New[struct{}](db),
		credentialCodec: workspaceModelTestCodec{},
		ModelMetaConf: &ModelMetaConf{Provider2Models: map[string]map[string]ModelMeta{
			developer_api.ModelClass_DeekSeek.String(): {
				"default": {
					DisplayInfo: &config.DisplayInfo{Name: "DeepSeek"},
					Connection:  &config.Connection{BaseConnInfo: &config.BaseConnectionInfo{}},
					Capability:  &developer_api.ModelAbility{},
				},
			},
		}},
	}
}

func newWorkspaceModelTestDraft(secret string) *workbenchmodel.WorkspaceModelDraft {
	return &workbenchmodel.WorkspaceModelDraft{
		ProviderKey:     "deepseek",
		ModelClass:      int32(developer_api.ModelClass_DeekSeek),
		DisplayName:     "DeepSeek V4 Pro",
		ModelIdentifier: "deepseek-v4-pro",
		Description:     stringPtr("workspace reasoning model"),
		Protocol:        "openai-compatible",
		Endpoints: []*workbenchmodel.WorkspaceModelEndpointInput{
			{
				BaseURL: "https://api.example.com/v1",
				APIKey:  &secret,
				Weight:  1,
				Enabled: true,
			},
		},
		Capabilities:     []string{"chat", "function_call"},
		UsageScenarios:   []string{"chat"},
		MaxContextTokens: int64Ptr(128000),
		MaxOutputTokens:  int64Ptr(8192),
		FunctionCallMode: stringPtr("native"),
		Enabled:          true,
	}
}

func stringPtr(value string) *string { return &value }

func int64Ptr(value int64) *int64 { return &value }
