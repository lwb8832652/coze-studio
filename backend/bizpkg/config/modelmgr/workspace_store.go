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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	config "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	workbenchmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/model"
)

const (
	modelInstanceTable     = "model_instance"
	modelEndpointTable     = "model_instance_endpoint"
	modelGrantTable        = "model_instance_grant"
	modelGrantSubjectSpace = "SPACE"
	managedModelStatusOff  = int32(0)
	managedModelStatusOn   = int32(1)
)

var (
	ErrWorkspaceModelNotFound          = errors.New("workspace model not found")
	ErrWorkspaceModelInvalid           = errors.New("invalid workspace model")
	ErrWorkspaceModelCredential        = errors.New("workspace model credential unavailable")
	ErrWorkspaceModelSchemaUnavailable = errors.New("workspace model schema unavailable")
	workspaceProviderKeyPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

type managedModelInstanceRow struct {
	ID               int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Type             int32      `gorm:"column:type"`
	Provider         string     `gorm:"column:provider"`
	DisplayInfo      string     `gorm:"column:display_info"`
	Connection       string     `gorm:"column:connection"`
	Capability       string     `gorm:"column:capability"`
	Parameters       string     `gorm:"column:parameters"`
	Extra            string     `gorm:"column:extra"`
	CreatedAt        int64      `gorm:"column:created_at"`
	UpdatedAt        int64      `gorm:"column:updated_at"`
	DeletedAt        *time.Time `gorm:"column:deleted_at"`
	ProviderKey      string     `gorm:"column:provider_key"`
	ModelIdentifier  string     `gorm:"column:model_identifier"`
	Description      string     `gorm:"column:description"`
	Status           int32      `gorm:"column:status"`
	SortOrder        int32      `gorm:"column:sort_order"`
	CreatorID        int64      `gorm:"column:creator_id"`
	Protocol         string     `gorm:"column:protocol"`
	RoutingStrategy  string     `gorm:"column:routing_strategy"`
	AccessMode       string     `gorm:"column:access_mode"`
	ScenarioJSON     string     `gorm:"column:scenario_json"`
	ReasoningMode    string     `gorm:"column:reasoning_mode"`
	FunctionCallMode string     `gorm:"column:function_call_mode"`
	MaxContextTokens int64      `gorm:"column:max_context_tokens"`
	MaxOutputTokens  int64      `gorm:"column:max_output_tokens"`
}

type managedModelEndpointRow struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ModelID           int64      `gorm:"column:model_id"`
	BaseURL           string     `gorm:"column:base_url"`
	APIKeyEnvelope    string     `gorm:"column:api_key_envelope"`
	APIKeyFingerprint string     `gorm:"column:api_key_fingerprint"`
	Weight            int32      `gorm:"column:weight"`
	Enabled           bool       `gorm:"column:enabled"`
	SortOrder         int32      `gorm:"column:sort_order"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
	DeletedAt         *time.Time `gorm:"column:deleted_at"`
}

type managedModelGrantRow struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ModelID     int64      `gorm:"column:model_id"`
	SubjectType string     `gorm:"column:subject_type"`
	SubjectID   int64      `gorm:"column:subject_id"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
}

type workspaceModelSettings struct {
	Capabilities   []string `json:"capabilities,omitempty"`
	UsageScenarios []string `json:"usage_scenarios,omitempty"`
}

type managedModelPayload struct {
	providerJSON   string
	displayJSON    string
	connectionJSON string
	capabilityJSON string
	parametersJSON string
	settingsJSON   string
}

func (c *ModelConfig) UpsertWorkspaceModel(
	ctx context.Context,
	spaceID int64,
	creatorID int64,
	modelID *int64,
	draft *workbenchmodel.WorkspaceModelDraft,
) (*workbenchmodel.WorkspaceModel, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("%w: model database is not initialized", ErrWorkspaceModelInvalid)
	}
	if !c.workspaceModelSchemaReady() {
		return nil, ErrWorkspaceModelSchemaUnavailable
	}
	if spaceID <= 0 || creatorID <= 0 {
		return nil, fmt.Errorf("%w: workspace and creator are required", ErrWorkspaceModelInvalid)
	}
	if err := validateWorkspaceModelDraft(draft); err != nil {
		return nil, err
	}
	payload, err := c.buildManagedModelPayload(draft)
	if err != nil {
		return nil, err
	}

	var persistedID int64
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		nowMillis := now.UnixMilli()
		status := managedModelStatusOff
		if draft.GetEnabled() {
			status = managedModelStatusOn
		}
		if modelID == nil || *modelID <= 0 {
			row := &managedModelInstanceRow{
				Type:             0,
				Provider:         payload.providerJSON,
				DisplayInfo:      payload.displayJSON,
				Connection:       payload.connectionJSON,
				Capability:       payload.capabilityJSON,
				Parameters:       payload.parametersJSON,
				Extra:            "{}",
				CreatedAt:        nowMillis,
				UpdatedAt:        nowMillis,
				ProviderKey:      normalizeProviderKey(draft.ProviderKey),
				ModelIdentifier:  strings.TrimSpace(draft.ModelIdentifier),
				Description:      strings.TrimSpace(draft.GetDescription()),
				Status:           status,
				CreatorID:        creatorID,
				Protocol:         strings.TrimSpace(draft.Protocol),
				RoutingStrategy:  "priority",
				AccessMode:       "workspace",
				ScenarioJSON:     payload.settingsJSON,
				FunctionCallMode: strings.TrimSpace(draft.GetFunctionCallMode()),
				MaxContextTokens: draft.GetMaxContextTokens(),
				MaxOutputTokens:  draft.GetMaxOutputTokens(),
			}
			if err := tx.Table(modelInstanceTable).Create(row).Error; err != nil {
				return fmt.Errorf("create workspace model: %w", err)
			}
			persistedID = row.ID
			grant := &managedModelGrantRow{
				ModelID:     row.ID,
				SubjectType: modelGrantSubjectSpace,
				SubjectID:   spaceID,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Table(modelGrantTable).Create(grant).Error; err != nil {
				return fmt.Errorf("grant workspace model: %w", err)
			}
		} else {
			persistedID = *modelID
			if err := requireWorkspaceModelGrant(tx, spaceID, persistedID); err != nil {
				return err
			}
			updates := map[string]any{
				"provider":           payload.providerJSON,
				"display_info":       payload.displayJSON,
				"connection":         payload.connectionJSON,
				"capability":         payload.capabilityJSON,
				"parameters":         payload.parametersJSON,
				"updated_at":         nowMillis,
				"provider_key":       normalizeProviderKey(draft.ProviderKey),
				"model_identifier":   strings.TrimSpace(draft.ModelIdentifier),
				"description":        strings.TrimSpace(draft.GetDescription()),
				"status":             status,
				"protocol":           strings.TrimSpace(draft.Protocol),
				"scenario_json":      payload.settingsJSON,
				"function_call_mode": strings.TrimSpace(draft.GetFunctionCallMode()),
				"max_context_tokens": draft.GetMaxContextTokens(),
				"max_output_tokens":  draft.GetMaxOutputTokens(),
			}
			result := tx.Table(modelInstanceTable).
				Where("id = ? AND deleted_at IS NULL", persistedID).
				Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("update workspace model: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrWorkspaceModelNotFound
			}
		}

		if err := c.replaceManagedEndpoints(tx, persistedID, draft.Endpoints, now); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return c.GetWorkspaceModel(ctx, spaceID, persistedID, true)
}

func (c *ModelConfig) ListWorkspaceModels(
	ctx context.Context,
	spaceID int64,
	canManage bool,
	keyword string,
) (*workbenchmodel.ListWorkspaceModelsData, error) {
	if c == nil || c.db == nil || spaceID <= 0 {
		return nil, fmt.Errorf("%w: workspace is required", ErrWorkspaceModelInvalid)
	}
	if !c.workspaceModelSchemaReady() {
		return c.listLegacySystemModels(ctx, keyword)
	}

	workspaceQuery := c.db.WithContext(ctx).Table(modelInstanceTable+" AS mi").
		Select("mi.*").
		Joins("JOIN "+modelGrantTable+" AS mg ON mg.model_id = mi.id AND mg.deleted_at IS NULL").
		Where("mi.deleted_at IS NULL AND mi.access_mode = ? AND mg.subject_type = ? AND mg.subject_id = ?", systemModelAccessWorkspace, modelGrantSubjectSpace, spaceID)
	systemQuery := c.db.WithContext(ctx).Table(modelInstanceTable+" AS mi").
		Select("mi.*").
		Where("mi.deleted_at IS NULL AND mi.access_mode <> ?", systemModelAccessWorkspace)

	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword != "" {
		like := "%" + keyword + "%"
		filter := "(LOWER(mi.provider_key) LIKE ? OR LOWER(mi.model_identifier) LIKE ? OR LOWER(mi.display_info) LIKE ?)"
		workspaceQuery = workspaceQuery.Where(filter, like, like, like)
		systemQuery = systemQuery.Where(filter, like, like, like)
	}

	var workspaceRows, systemRows []managedModelInstanceRow
	if err := workspaceQuery.Order("mi.sort_order ASC, mi.id DESC").Find(&workspaceRows).Error; err != nil {
		return nil, fmt.Errorf("list workspace models: %w", err)
	}
	if err := systemQuery.Order("mi.sort_order ASC, mi.id DESC").Find(&systemRows).Error; err != nil {
		return nil, fmt.Errorf("list system models: %w", err)
	}

	workspaceModels, err := c.projectManagedModels(ctx, workspaceRows, workbenchmodel.WorkspaceModelScope_Space, canManage)
	if err != nil {
		return nil, err
	}
	systemModels, err := c.projectManagedModels(ctx, systemRows, workbenchmodel.WorkspaceModelScope_System, false)
	if err != nil {
		return nil, err
	}
	return &workbenchmodel.ListWorkspaceModelsData{
		SystemModels:    systemModels,
		WorkspaceModels: workspaceModels,
		Providers:       workspaceModelProviderOptions(),
		CanManage:       canManage,
	}, nil
}

func (c *ModelConfig) GetWorkspaceModel(
	ctx context.Context,
	spaceID int64,
	modelID int64,
	canManage bool,
) (*workbenchmodel.WorkspaceModel, error) {
	if c == nil || c.db == nil || spaceID <= 0 || modelID <= 0 {
		return nil, ErrWorkspaceModelNotFound
	}
	if !c.workspaceModelSchemaReady() {
		return nil, ErrWorkspaceModelNotFound
	}
	var row managedModelInstanceRow
	err := c.db.WithContext(ctx).Table(modelInstanceTable+" AS mi").
		Select("mi.*").
		Joins("JOIN "+modelGrantTable+" AS mg ON mg.model_id = mi.id AND mg.deleted_at IS NULL").
		Where("mi.id = ? AND mi.deleted_at IS NULL AND mg.subject_type = ? AND mg.subject_id = ?", modelID, modelGrantSubjectSpace, spaceID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrWorkspaceModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get workspace model: %w", err)
	}
	models, err := c.projectManagedModels(ctx, []managedModelInstanceRow{row}, workbenchmodel.WorkspaceModelScope_Space, canManage)
	if err != nil {
		return nil, err
	}
	if len(models) != 1 {
		return nil, ErrWorkspaceModelNotFound
	}
	return models[0], nil
}

func (c *ModelConfig) SetWorkspaceModelStatus(ctx context.Context, spaceID, modelID int64, enabled bool) error {
	if c == nil || c.db == nil {
		return ErrWorkspaceModelNotFound
	}
	if !c.workspaceModelSchemaReady() {
		return ErrWorkspaceModelSchemaUnavailable
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireWorkspaceModelGrant(tx, spaceID, modelID); err != nil {
			return err
		}
		status := managedModelStatusOff
		if enabled {
			status = managedModelStatusOn
		}
		result := tx.Table(modelInstanceTable).
			Where("id = ? AND deleted_at IS NULL", modelID).
			Updates(map[string]any{"status": status, "updated_at": time.Now().UnixMilli()})
		if result.Error != nil {
			return fmt.Errorf("update workspace model status: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrWorkspaceModelNotFound
		}
		return nil
	})
}

func (c *ModelConfig) DeleteWorkspaceModel(ctx context.Context, spaceID, modelID int64) error {
	if c == nil || c.db == nil {
		return ErrWorkspaceModelNotFound
	}
	if !c.workspaceModelSchemaReady() {
		return ErrWorkspaceModelSchemaUnavailable
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := requireWorkspaceModelGrant(tx, spaceID, modelID); err != nil {
			return err
		}
		now := time.Now()
		nowMillis := now.UnixMilli()
		if err := tx.Table(modelEndpointTable).
			Where("model_id = ? AND deleted_at IS NULL", modelID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("delete workspace model endpoints: %w", err)
		}
		if err := tx.Table(modelGrantTable).
			Where("model_id = ? AND subject_type = ? AND subject_id = ? AND deleted_at IS NULL", modelID, modelGrantSubjectSpace, spaceID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("delete workspace model grant: %w", err)
		}
		result := tx.Table(modelInstanceTable).
			Where("id = ? AND deleted_at IS NULL", modelID).
			Updates(map[string]any{"deleted_at": now, "updated_at": nowMillis})
		if result.Error != nil {
			return fmt.Errorf("delete workspace model: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrWorkspaceModelNotFound
		}
		return nil
	})
}

func (c *ModelConfig) AvailableModelIDs(ctx context.Context, spaceID int64) (map[int64]struct{}, error) {
	if c == nil || c.db == nil {
		return nil, fmt.Errorf("%w: model database is not initialized", ErrWorkspaceModelInvalid)
	}
	if !c.workspaceModelSchemaReady() {
		var ids []int64
		if err := c.db.WithContext(ctx).Table(modelInstanceTable).
			Where("deleted_at IS NULL").
			Pluck("id", &ids).Error; err != nil {
			return nil, fmt.Errorf("list legacy model ids: %w", err)
		}
		available := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			available[id] = struct{}{}
		}
		return available, nil
	}
	query := c.db.WithContext(ctx).Table(modelInstanceTable+" AS mi").
		Distinct("mi.id").
		Where("mi.deleted_at IS NULL AND mi.status = ?", managedModelStatusOn).
		Where("NOT EXISTS (SELECT 1 FROM "+modelGrantTable+" AS any_grant WHERE any_grant.model_id = mi.id AND any_grant.subject_type = ? AND any_grant.deleted_at IS NULL)", modelGrantSubjectSpace)
	if spaceID > 0 {
		query = c.db.WithContext(ctx).Table(modelInstanceTable+" AS mi").
			Distinct("mi.id").
			Where("mi.deleted_at IS NULL AND mi.status = ?", managedModelStatusOn).
			Where(
				"NOT EXISTS (SELECT 1 FROM "+modelGrantTable+" AS any_grant WHERE any_grant.model_id = mi.id AND any_grant.subject_type = ? AND any_grant.deleted_at IS NULL) OR "+
					"EXISTS (SELECT 1 FROM "+modelGrantTable+" AS workspace_grant WHERE workspace_grant.model_id = mi.id AND workspace_grant.subject_type = ? AND workspace_grant.subject_id = ? AND workspace_grant.deleted_at IS NULL)",
				modelGrantSubjectSpace, modelGrantSubjectSpace, spaceID,
			)
	}
	var ids []int64
	if err := query.Pluck("mi.id", &ids).Error; err != nil {
		return nil, fmt.Errorf("list available models: %w", err)
	}
	available := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		available[id] = struct{}{}
	}
	return available, nil
}

func (c *ModelConfig) TestWorkspaceModelConnection(
	ctx context.Context,
	spaceID int64,
	modelID *int64,
	draft *workbenchmodel.WorkspaceModelDraft,
) (*workbenchmodel.TestWorkspaceModelData, error) {
	if err := validateWorkspaceModelDraft(draft); err != nil {
		return nil, err
	}
	endpoint := firstEnabledEndpoint(draft.Endpoints)
	if endpoint == nil {
		return nil, fmt.Errorf("%w: an enabled endpoint is required", ErrWorkspaceModelInvalid)
	}
	apiKey := strings.TrimSpace(endpoint.GetAPIKey())
	if apiKey == "" {
		if modelID == nil || *modelID <= 0 || endpoint.ID == nil || endpoint.GetID() <= 0 {
			return nil, fmt.Errorf("%w: API key is required for connectivity testing", ErrWorkspaceModelInvalid)
		}
		if c.credentialCodec == nil {
			return nil, ErrWorkspaceModelCredential
		}
		if err := requireWorkspaceModelGrant(c.db.WithContext(ctx), spaceID, *modelID); err != nil {
			return nil, err
		}
		var stored managedModelEndpointRow
		err := c.db.WithContext(ctx).Table(modelEndpointTable).
			Where("id = ? AND model_id = ? AND deleted_at IS NULL", endpoint.GetID(), *modelID).
			First(&stored).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkspaceModelNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("load workspace model credential: %w", err)
		}
		apiKey, err = c.credentialCodec.Decrypt(*modelID, stored.ID, stored.APIKeyEnvelope)
		if err != nil {
			return nil, fmt.Errorf("decrypt workspace model credential: %w", err)
		}
	}

	targetURL := workspaceModelProbeURL(endpoint.BaseURL, draft.Protocol)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: endpoint URL is invalid", ErrWorkspaceModelInvalid)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	request.Header.Set("x-api-key", apiKey)
	request.Header.Set("User-Agent", "Coze-Studio-Model-Connectivity/1.0")

	client := &http.Client{
		Timeout: 12 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 8 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	started := time.Now()
	response, err := client.Do(request)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		code := "network_error"
		message := "无法连接模型服务，请检查地址和网络后重试"
		return &workbenchmodel.TestWorkspaceModelData{
			Success: false, DurationMs: duration, ErrorCode: &code, Message: &message,
		}, nil
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		code := "provider_rejected"
		message := "模型服务拒绝了连接，请检查 API Key、模型标识和服务地址"
		if response.StatusCode >= http.StatusInternalServerError {
			code = "provider_unavailable"
			message = "模型服务暂时不可用，请稍后重试"
		}
		return &workbenchmodel.TestWorkspaceModelData{
			Success: false, DurationMs: duration, ErrorCode: &code, Message: &message,
		}, nil
	}
	message := "连接成功"
	return &workbenchmodel.TestWorkspaceModelData{Success: true, DurationMs: duration, Message: &message}, nil
}

func (c *ModelConfig) hydrateManagedConnection(
	ctx context.Context,
	modelID int64,
	connection *config.Connection,
) (*config.Connection, error) {
	if c == nil || c.db == nil || modelID <= 0 {
		return connection, nil
	}
	if !c.workspaceModelSchemaReady() {
		return connection, nil
	}
	var endpoint managedModelEndpointRow
	err := c.db.WithContext(ctx).Table(modelEndpointTable).
		Where("model_id = ? AND enabled = ? AND deleted_at IS NULL", modelID, true).
		Order("sort_order ASC, id ASC").
		First(&endpoint).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return connection, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load managed model endpoint: %w", err)
	}
	if c.credentialCodec == nil {
		return nil, ErrWorkspaceModelCredential
	}
	if strings.TrimSpace(endpoint.APIKeyEnvelope) == "" {
		return nil, fmt.Errorf("%w: endpoint credential is missing", ErrWorkspaceModelCredential)
	}
	apiKey, err := c.credentialCodec.Decrypt(modelID, endpoint.ID, endpoint.APIKeyEnvelope)
	if err != nil {
		return nil, fmt.Errorf("decrypt managed model credential: %w", err)
	}
	if connection == nil {
		connection = &config.Connection{}
	}
	if connection.BaseConnInfo == nil {
		connection.BaseConnInfo = &config.BaseConnectionInfo{}
	}
	connection.BaseConnInfo.BaseURL = endpoint.BaseURL
	connection.BaseConnInfo.APIKey = apiKey
	return connection, nil
}

func (c *ModelConfig) buildManagedModelPayload(draft *workbenchmodel.WorkspaceModelDraft) (*managedModelPayload, error) {
	modelClass := developer_api.ModelClass(draft.ModelClass)
	provider, ok := GetModelProvider(modelClass)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported provider", ErrWorkspaceModelInvalid)
	}
	if c.ModelMetaConf == nil {
		return nil, fmt.Errorf("%w: model metadata is unavailable", ErrWorkspaceModelInvalid)
	}
	meta, err := c.ModelMetaConf.GetModelMeta(modelClass, strings.TrimSpace(draft.ModelIdentifier))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorkspaceModelInvalid, err)
	}
	if meta.DisplayInfo == nil {
		meta.DisplayInfo = &config.DisplayInfo{}
	}
	meta.DisplayInfo.Name = strings.TrimSpace(draft.DisplayName)
	meta.DisplayInfo.MaxTokens = draft.GetMaxContextTokens()
	meta.DisplayInfo.OutputTokens = draft.GetMaxOutputTokens()
	if meta.Connection == nil {
		meta.Connection = &config.Connection{}
	}
	if meta.Connection.BaseConnInfo == nil {
		meta.Connection.BaseConnInfo = &config.BaseConnectionInfo{}
	}
	meta.Connection.BaseConnInfo.Model = strings.TrimSpace(draft.ModelIdentifier)
	meta.Connection.BaseConnInfo.BaseURL = strings.TrimSpace(draft.Endpoints[0].BaseURL)
	meta.Connection.BaseConnInfo.APIKey = ""

	settings := workspaceModelSettings{
		Capabilities:   normalizeStringList(draft.GetCapabilities()),
		UsageScenarios: normalizeStringList(draft.GetUsageScenarios()),
	}
	providerJSON, err := marshalManagedModelJSON(provider)
	if err != nil {
		return nil, err
	}
	displayJSON, err := marshalManagedModelJSON(meta.DisplayInfo)
	if err != nil {
		return nil, err
	}
	connectionJSON, err := marshalManagedModelJSON(meta.Connection)
	if err != nil {
		return nil, err
	}
	capabilityJSON, err := marshalManagedModelJSON(meta.Capability)
	if err != nil {
		return nil, err
	}
	parametersJSON, err := marshalManagedModelJSON(meta.Parameters)
	if err != nil {
		return nil, err
	}
	settingsJSON, err := marshalManagedModelJSON(settings)
	if err != nil {
		return nil, err
	}
	return &managedModelPayload{
		providerJSON: providerJSON, displayJSON: displayJSON, connectionJSON: connectionJSON,
		capabilityJSON: capabilityJSON, parametersJSON: parametersJSON, settingsJSON: settingsJSON,
	}, nil
}

func (c *ModelConfig) replaceManagedEndpoints(
	tx *gorm.DB,
	modelID int64,
	inputs []*workbenchmodel.WorkspaceModelEndpointInput,
	now time.Time,
) error {
	var existing []managedModelEndpointRow
	if err := tx.Table(modelEndpointTable).
		Where("model_id = ? AND deleted_at IS NULL", modelID).
		Find(&existing).Error; err != nil {
		return fmt.Errorf("load workspace model endpoints: %w", err)
	}
	existingByID := make(map[int64]managedModelEndpointRow, len(existing))
	for _, endpoint := range existing {
		existingByID[endpoint.ID] = endpoint
	}
	retained := make(map[int64]struct{}, len(inputs))

	for index, input := range inputs {
		if input == nil {
			return fmt.Errorf("%w: endpoint is required", ErrWorkspaceModelInvalid)
		}
		baseURL := strings.TrimSpace(input.BaseURL)
		weight := input.GetWeight()
		if weight <= 0 {
			weight = 1
		}
		if input.ID == nil || input.GetID() <= 0 {
			apiKey := strings.TrimSpace(input.GetAPIKey())
			if apiKey == "" {
				return fmt.Errorf("%w: API key is required for a new endpoint", ErrWorkspaceModelInvalid)
			}
			if c.credentialCodec == nil {
				return ErrWorkspaceModelCredential
			}
			row := &managedModelEndpointRow{
				ModelID: modelID, BaseURL: baseURL, Weight: weight, Enabled: input.GetEnabled(),
				SortOrder: int32(index), CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Table(modelEndpointTable).Create(row).Error; err != nil {
				return fmt.Errorf("create workspace model endpoint: %w", err)
			}
			envelope, fingerprint, err := c.credentialCodec.Encrypt(modelID, row.ID, apiKey)
			if err != nil {
				return fmt.Errorf("encrypt workspace model credential: %w", err)
			}
			if err := tx.Table(modelEndpointTable).Where("id = ? AND model_id = ?", row.ID, modelID).
				Updates(map[string]any{"api_key_envelope": envelope, "api_key_fingerprint": fingerprint}).Error; err != nil {
				return fmt.Errorf("persist workspace model credential: %w", err)
			}
			retained[row.ID] = struct{}{}
			continue
		}

		endpointID := input.GetID()
		current, ok := existingByID[endpointID]
		if !ok {
			return fmt.Errorf("%w: endpoint does not belong to model", ErrWorkspaceModelInvalid)
		}
		updates := map[string]any{
			"base_url": baseURL, "weight": weight, "enabled": input.GetEnabled(),
			"sort_order": int32(index), "updated_at": now,
		}
		apiKey := strings.TrimSpace(input.GetAPIKey())
		if apiKey != "" {
			if c.credentialCodec == nil {
				return ErrWorkspaceModelCredential
			}
			envelope, fingerprint, err := c.credentialCodec.Encrypt(modelID, endpointID, apiKey)
			if err != nil {
				return fmt.Errorf("encrypt workspace model credential: %w", err)
			}
			updates["api_key_envelope"] = envelope
			updates["api_key_fingerprint"] = fingerprint
		} else if strings.TrimSpace(current.APIKeyEnvelope) == "" {
			return fmt.Errorf("%w: endpoint credential is missing", ErrWorkspaceModelInvalid)
		}
		if err := tx.Table(modelEndpointTable).
			Where("id = ? AND model_id = ? AND deleted_at IS NULL", endpointID, modelID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update workspace model endpoint: %w", err)
		}
		retained[endpointID] = struct{}{}
	}

	for _, endpoint := range existing {
		if _, ok := retained[endpoint.ID]; ok {
			continue
		}
		if err := tx.Table(modelEndpointTable).Where("id = ? AND model_id = ? AND deleted_at IS NULL", endpoint.ID, modelID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("delete workspace model endpoint: %w", err)
		}
	}
	return nil
}

func (c *ModelConfig) projectManagedModels(
	ctx context.Context,
	rows []managedModelInstanceRow,
	scope workbenchmodel.WorkspaceModelScope,
	canManage bool,
) ([]*workbenchmodel.WorkspaceModel, error) {
	if len(rows) == 0 {
		return []*workbenchmodel.WorkspaceModel{}, nil
	}
	modelIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		modelIDs = append(modelIDs, row.ID)
	}
	var endpoints []managedModelEndpointRow
	if err := c.db.WithContext(ctx).Table(modelEndpointTable).
		Where("model_id IN ? AND deleted_at IS NULL", modelIDs).
		Order("sort_order ASC, id ASC").Find(&endpoints).Error; err != nil {
		return nil, fmt.Errorf("list workspace model endpoints: %w", err)
	}
	endpointsByModel := make(map[int64][]managedModelEndpointRow)
	for _, endpoint := range endpoints {
		endpointsByModel[endpoint.ModelID] = append(endpointsByModel[endpoint.ModelID], endpoint)
	}

	models := make([]*workbenchmodel.WorkspaceModel, 0, len(rows))
	for _, row := range rows {
		model, err := projectManagedModel(row, endpointsByModel[row.ID], scope, canManage)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return models, nil
}

func projectManagedModel(
	row managedModelInstanceRow,
	endpoints []managedModelEndpointRow,
	scope workbenchmodel.WorkspaceModelScope,
	canManage bool,
) (*workbenchmodel.WorkspaceModel, error) {
	provider := &config.ModelProvider{}
	if err := json.Unmarshal([]byte(row.Provider), provider); err != nil {
		return nil, fmt.Errorf("decode model provider %d: %w", row.ID, err)
	}
	display := &config.DisplayInfo{}
	if err := json.Unmarshal([]byte(row.DisplayInfo), display); err != nil {
		return nil, fmt.Errorf("decode model display info %d: %w", row.ID, err)
	}
	settings := workspaceModelSettings{}
	if strings.TrimSpace(row.ScenarioJSON) != "" {
		if err := json.Unmarshal([]byte(row.ScenarioJSON), &settings); err != nil {
			return nil, fmt.Errorf("decode model settings %d: %w", row.ID, err)
		}
	}
	endpointViews := make([]*workbenchmodel.WorkspaceModelEndpointView, 0, len(endpoints))
	credentialConfigured := false
	for _, endpoint := range endpoints {
		configured := strings.TrimSpace(endpoint.APIKeyEnvelope) != ""
		credentialConfigured = credentialConfigured || configured
		endpointViews = append(endpointViews, &workbenchmodel.WorkspaceModelEndpointView{
			ID: endpoint.ID, BaseURL: endpoint.BaseURL, CredentialConfigured: configured,
			Weight: endpoint.Weight, Enabled: endpoint.Enabled,
		})
	}
	if scope == workbenchmodel.WorkspaceModelScope_System && !credentialConfigured {
		connection := &config.Connection{}
		if err := json.Unmarshal([]byte(row.Connection), connection); err == nil && connection.BaseConnInfo != nil {
			credentialConfigured = strings.TrimSpace(connection.BaseConnInfo.APIKey) != ""
		}
	}
	description := strings.TrimSpace(row.Description)
	protocol := strings.TrimSpace(row.Protocol)
	functionCallMode := strings.TrimSpace(row.FunctionCallMode)
	creatorID := row.CreatorID
	updatedAt := row.UpdatedAt
	maxContextTokens := row.MaxContextTokens
	maxOutputTokens := row.MaxOutputTokens
	return &workbenchmodel.WorkspaceModel{
		ID:                   row.ID,
		Scope:                scope,
		ProviderKey:          row.ProviderKey,
		ModelClass:           int32(provider.ModelClass),
		DisplayName:          display.Name,
		ModelIdentifier:      row.ModelIdentifier,
		Enabled:              row.Status == managedModelStatusOn,
		CredentialConfigured: credentialConfigured,
		CanManage:            scope == workbenchmodel.WorkspaceModelScope_Space && canManage,
		Description:          optionalString(description),
		Protocol:             optionalString(protocol),
		Capabilities:         settings.Capabilities,
		UsageScenarios:       settings.UsageScenarios,
		MaxContextTokens:     optionalPositiveInt64(maxContextTokens),
		MaxOutputTokens:      optionalPositiveInt64(maxOutputTokens),
		FunctionCallMode:     optionalString(functionCallMode),
		Endpoints:            endpointViews,
		CreatorID:            optionalPositiveInt64(creatorID),
		UpdatedAt:            optionalPositiveInt64(updatedAt),
	}, nil
}

func requireWorkspaceModelGrant(tx *gorm.DB, spaceID, modelID int64) error {
	if spaceID <= 0 || modelID <= 0 {
		return ErrWorkspaceModelNotFound
	}
	var count int64
	err := tx.Table(modelGrantTable).
		Where("model_id = ? AND subject_type = ? AND subject_id = ? AND deleted_at IS NULL", modelID, modelGrantSubjectSpace, spaceID).
		Count(&count).Error
	if err != nil {
		return fmt.Errorf("check workspace model grant: %w", err)
	}
	if count != 1 {
		return ErrWorkspaceModelNotFound
	}
	return nil
}

func validateWorkspaceModelDraft(draft *workbenchmodel.WorkspaceModelDraft) error {
	if draft == nil {
		return fmt.Errorf("%w: model is required", ErrWorkspaceModelInvalid)
	}
	providerKey := normalizeProviderKey(draft.ProviderKey)
	if !workspaceProviderKeyPattern.MatchString(providerKey) {
		return fmt.Errorf("%w: provider is invalid", ErrWorkspaceModelInvalid)
	}
	if len(strings.TrimSpace(draft.DisplayName)) == 0 || len(strings.TrimSpace(draft.DisplayName)) > 128 {
		return fmt.Errorf("%w: display name is invalid", ErrWorkspaceModelInvalid)
	}
	if len(strings.TrimSpace(draft.ModelIdentifier)) == 0 || len(strings.TrimSpace(draft.ModelIdentifier)) > 256 {
		return fmt.Errorf("%w: model identifier is invalid", ErrWorkspaceModelInvalid)
	}
	if len(strings.TrimSpace(draft.GetDescription())) > 1000 {
		return fmt.Errorf("%w: description is too long", ErrWorkspaceModelInvalid)
	}
	if len(strings.TrimSpace(draft.Protocol)) == 0 || len(strings.TrimSpace(draft.Protocol)) > 64 {
		return fmt.Errorf("%w: protocol is invalid", ErrWorkspaceModelInvalid)
	}
	if len(draft.Endpoints) == 0 || len(draft.Endpoints) > 8 {
		return fmt.Errorf("%w: one to eight endpoints are required", ErrWorkspaceModelInvalid)
	}
	for _, endpoint := range draft.Endpoints {
		if endpoint == nil {
			return fmt.Errorf("%w: endpoint is required", ErrWorkspaceModelInvalid)
		}
		baseURL := strings.TrimSpace(endpoint.BaseURL)
		if len(baseURL) == 0 || len(baseURL) > 2048 {
			return fmt.Errorf("%w: endpoint URL is invalid", ErrWorkspaceModelInvalid)
		}
		parsed, err := url.Parse(baseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
			return fmt.Errorf("%w: endpoint URL is invalid", ErrWorkspaceModelInvalid)
		}
		if len(endpoint.GetAPIKey()) > 8192 {
			return fmt.Errorf("%w: API key is too long", ErrWorkspaceModelInvalid)
		}
	}
	if draft.GetMaxContextTokens() < 0 || draft.GetMaxOutputTokens() < 0 {
		return fmt.Errorf("%w: token limits cannot be negative", ErrWorkspaceModelInvalid)
	}
	if len(draft.GetCapabilities()) > 32 || len(draft.GetUsageScenarios()) > 32 {
		return fmt.Errorf("%w: too many model tags", ErrWorkspaceModelInvalid)
	}
	return nil
}

func workspaceModelProviderOptions() []*workbenchmodel.WorkspaceModelProviderOption {
	providers := getModelProviderList()
	options := make([]*workbenchmodel.WorkspaceModelProviderOption, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		name := provider.ModelClass.String()
		if provider.Name != nil {
			if strings.TrimSpace(provider.Name.ZhCn) != "" {
				name = provider.Name.ZhCn
			} else if strings.TrimSpace(provider.Name.EnUs) != "" {
				name = provider.Name.EnUs
			}
		}
		options = append(options, &workbenchmodel.WorkspaceModelProviderOption{
			Key: providerKeyForClass(provider.ModelClass), Name: name, ModelClass: int32(provider.ModelClass),
		})
	}
	sort.SliceStable(options, func(i, j int) bool { return options[i].Name < options[j].Name })
	return options
}

func providerKeyForClass(class developer_api.ModelClass) string {
	switch class {
	case developer_api.ModelClass_SEED:
		return "doubao"
	case developer_api.ModelClass_Claude:
		return "claude"
	case developer_api.ModelClass_DeekSeek:
		return "deepseek"
	case developer_api.ModelClass_Gemini:
		return "gemini"
	case developer_api.ModelClass_Llama:
		return "ollama"
	case developer_api.ModelClass_GPT:
		return "openai"
	case developer_api.ModelClass_QWen:
		return "qwen"
	default:
		return strings.ToLower(class.String())
	}
}

func normalizeProviderKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func marshalManagedModelJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode workspace model metadata: %w", err)
	}
	return string(data), nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalPositiveInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

func (c *ModelConfig) workspaceModelSchemaReady() bool {
	if c == nil || c.db == nil {
		return false
	}
	migrator := c.db.Migrator()
	return migrator.HasTable(modelEndpointTable) &&
		migrator.HasTable(modelGrantTable) &&
		migrator.HasColumn(modelInstanceTable, "provider_key")
}

func (c *ModelConfig) listLegacySystemModels(
	ctx context.Context,
	keyword string,
) (*workbenchmodel.ListWorkspaceModelsData, error) {
	query := c.db.WithContext(ctx).Table(modelInstanceTable).
		Select("id, type, provider, display_info, connection, capability, parameters, extra, created_at, updated_at, deleted_at").
		Where("deleted_at IS NULL")
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where(
			"LOWER(provider) LIKE ? OR LOWER(display_info) LIKE ? OR LOWER(connection) LIKE ?",
			like, like, like,
		)
	}
	var rows []managedModelInstanceRow
	if err := query.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list legacy system models: %w", err)
	}
	models := make([]*workbenchmodel.WorkspaceModel, 0, len(rows))
	for _, row := range rows {
		model, err := projectLegacySystemModel(row)
		if err != nil {
			return nil, err
		}
		models = append(models, model)
	}
	return &workbenchmodel.ListWorkspaceModelsData{
		SystemModels:    models,
		WorkspaceModels: []*workbenchmodel.WorkspaceModel{},
		Providers:       workspaceModelProviderOptions(),
		CanManage:       false,
	}, nil
}

func projectLegacySystemModel(row managedModelInstanceRow) (*workbenchmodel.WorkspaceModel, error) {
	provider := &config.ModelProvider{}
	if err := json.Unmarshal([]byte(row.Provider), provider); err != nil {
		return nil, fmt.Errorf("decode legacy model provider %d: %w", row.ID, err)
	}
	display := &config.DisplayInfo{}
	if err := json.Unmarshal([]byte(row.DisplayInfo), display); err != nil {
		return nil, fmt.Errorf("decode legacy model display info %d: %w", row.ID, err)
	}
	connection := &config.Connection{}
	if err := json.Unmarshal([]byte(row.Connection), connection); err != nil {
		return nil, fmt.Errorf("decode legacy model connection %d: %w", row.ID, err)
	}
	modelIdentifier := ""
	credentialConfigured := false
	if connection.BaseConnInfo != nil {
		modelIdentifier = connection.BaseConnInfo.Model
		credentialConfigured = strings.TrimSpace(connection.BaseConnInfo.APIKey) != ""
	}
	return &workbenchmodel.WorkspaceModel{
		ID:                   row.ID,
		Scope:                workbenchmodel.WorkspaceModelScope_System,
		ProviderKey:          providerKeyForClass(provider.ModelClass),
		ModelClass:           int32(provider.ModelClass),
		DisplayName:          display.Name,
		ModelIdentifier:      modelIdentifier,
		Enabled:              true,
		CredentialConfigured: credentialConfigured,
		CanManage:            false,
		MaxContextTokens:     optionalPositiveInt64(display.MaxTokens),
		MaxOutputTokens:      optionalPositiveInt64(display.OutputTokens),
		UpdatedAt:            optionalPositiveInt64(row.UpdatedAt),
	}, nil
}

func firstEnabledEndpoint(endpoints []*workbenchmodel.WorkspaceModelEndpointInput) *workbenchmodel.WorkspaceModelEndpointInput {
	for _, endpoint := range endpoints {
		if endpoint != nil && endpoint.GetEnabled() {
			return endpoint
		}
	}
	return nil
}

func workspaceModelProbeURL(baseURL, protocol string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.Contains(strings.ToLower(protocol), "openai") && !strings.HasSuffix(strings.ToLower(baseURL), "/models") {
		return baseURL + "/models"
	}
	return baseURL
}
