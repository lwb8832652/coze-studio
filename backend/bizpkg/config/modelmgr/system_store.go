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
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
)

var (
	ErrSystemModelInvalid           = errors.New("invalid system model configuration")
	ErrSystemModelNotFound          = errors.New("system model not found")
	ErrSystemModelSchemaUnavailable = errors.New("system model schema is unavailable")
	ErrSystemModelCredential        = errors.New("system model credential service is unavailable")
)

const (
	systemModelAccessAll        = "all"
	systemModelAccessRestricted = "restricted"
	systemModelAccessWorkspace  = "workspace"
	systemModelGrantUser        = "user"
)

type systemModelRow struct {
	ID               int64      `gorm:"column:id;primaryKey"`
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
	SortOrder        int64      `gorm:"column:sort_order"`
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

type systemModelEndpointRow struct {
	ID                int64      `gorm:"column:id;primaryKey"`
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

type systemModelGrantRow struct {
	ID          int64      `gorm:"column:id;primaryKey"`
	ModelID     int64      `gorm:"column:model_id"`
	SubjectType string     `gorm:"column:subject_type"`
	SubjectID   int64      `gorm:"column:subject_id"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
}

type systemModelPayload struct {
	providerJSON   string
	displayJSON    string
	connectionJSON string
	capabilityJSON string
	parametersJSON string
	extraJSON      string
	settingsJSON   string
}

type SystemModelEndpointTestResult struct {
	Success      bool
	LatencyMS    int64
	ErrorCode    *string
	ErrorMessage *string
}

func (c *ModelConfig) ListSystemModelProviders(_ context.Context) ([]*config.ModelProviderOption, error) {
	providers := getModelProviderList()
	result := make([]*config.ModelProviderOption, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		key := providerKeyForClass(provider.ModelClass)
		protocol := "openai-compatible"
		defaultURL := systemModelDefaultBaseURL(key)
		if key == "ollama" {
			protocol = "ollama"
		}
		result = append(result, &config.ModelProviderOption{
			ProviderKey:           key,
			Name:                  provider.Name,
			ModelClass:            provider.ModelClass,
			Protocol:              protocol,
			SupportsCustomBaseURL: true,
			SupportsFunctionCall:  key != "ollama",
			SupportsMultimodal:    key != "ollama" && key != "deepseek",
			DefaultBaseURL:        systemOptionalString(defaultURL),
		})
	}
	return result, nil
}

func (c *ModelConfig) ListSystemModels(ctx context.Context, req *config.GetModelListReq) ([]*config.ModelManagementItem, int64, error) {
	if c == nil || c.db == nil {
		return nil, 0, ErrSystemModelSchemaUnavailable
	}
	if !c.workspaceModelSchemaReady() {
		return nil, 0, ErrSystemModelSchemaUnavailable
	}
	query := c.db.WithContext(ctx).Table(modelInstanceTable).
		Where("deleted_at IS NULL AND access_mode <> ?", systemModelAccessWorkspace)
	if req != nil {
		if keyword := strings.TrimSpace(req.GetKeyword()); keyword != "" {
			like := "%" + strings.ToLower(keyword) + "%"
			query = query.Where(
				"LOWER(provider_key) LIKE ? OR LOWER(model_identifier) LIKE ? OR LOWER(description) LIKE ? OR LOWER(display_info) LIKE ?",
				like, like, like, like,
			)
		}
		if providerKey := normalizeProviderKey(req.GetProviderKey()); providerKey != "" {
			query = query.Where("provider_key = ?", providerKey)
		}
		if req.IsSetEnabled() {
			status := managedModelStatusOff
			if req.GetEnabled() {
				status = managedModelStatusOn
			}
			query = query.Where("status = ?", status)
		}
		if accessMode := strings.TrimSpace(req.GetAccessMode()); accessMode != "" {
			query = query.Where("access_mode = ?", accessMode)
		}
	}

	var rows []systemModelRow
	if err := query.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list system models: %w", err)
	}
	models := make([]*config.ModelManagementItem, 0, len(rows))
	capabilityFilter := ""
	if req != nil {
		capabilityFilter = strings.TrimSpace(req.GetCapabilityType())
	}
	for _, row := range rows {
		item, err := projectSystemModelSummary(row)
		if err != nil {
			return nil, 0, err
		}
		if capabilityFilter != "" && !containsString(item.CapabilityTypes, capabilityFilter) {
			continue
		}
		models = append(models, item)
	}

	total := int64(len(models))
	page, pageSize := int32(1), int32(20)
	if req != nil {
		if req.GetPage() > 0 {
			page = req.GetPage()
		}
		if req.GetPageSize() > 0 {
			pageSize = req.GetPageSize()
		}
	}
	if pageSize > 100 {
		pageSize = 100
	}
	start := int((page - 1) * pageSize)
	if start >= len(models) {
		return []*config.ModelManagementItem{}, total, nil
	}
	end := start + int(pageSize)
	if end > len(models) {
		end = len(models)
	}
	return models[start:end], total, nil
}

func (c *ModelConfig) GetSystemModelDetail(ctx context.Context, modelID int64) (*config.ModelDetail, error) {
	row, err := c.getSystemModelRow(ctx, modelID)
	if err != nil {
		return nil, err
	}
	summary, err := projectSystemModelSummary(*row)
	if err != nil {
		return nil, err
	}
	var settings workspaceModelSettings
	_ = json.Unmarshal([]byte(row.ScenarioJSON), &settings)
	var extra struct {
		EnableBase64URL bool `json:"enable_base64_url"`
	}
	_ = json.Unmarshal([]byte(row.Extra), &extra)

	endpoints, err := c.listSystemModelEndpoints(ctx, *row)
	if err != nil {
		return nil, err
	}
	usageScenarios := normalizeStringList(settings.UsageScenarios)
	if len(usageScenarios) == 0 {
		usageScenarios = []string{"chat", "agent", "workflow", "appdev"}
	}
	maxContextTokens := row.MaxContextTokens
	if maxContextTokens <= 0 {
		maxContextTokens = 128000
	}
	maxOutputTokens := row.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = 4096
	}
	return &config.ModelDetail{
		Summary:          summary,
		ReasoningMode:    systemModelReasoningMode(*row),
		MaxContextTokens: maxContextTokens,
		MaxOutputTokens:  maxOutputTokens,
		FunctionCallMode: systemModelFunctionCallMode(*row),
		UsageScenarios:   usageScenarios,
		Protocol:         fallbackString(row.Protocol, "openai-compatible"),
		RoutingStrategy:  systemRoutingStrategyFromString(row.RoutingStrategy),
		Endpoints:        endpoints,
		EnableBase64URL:  extra.EnableBase64URL,
	}, nil
}

func (c *ModelConfig) UpsertSystemModel(
	ctx context.Context,
	creatorID int64,
	modelID *int64,
	input *config.ModelManagementInput,
) (int64, error) {
	if c == nil || c.db == nil || !c.workspaceModelSchemaReady() {
		return 0, ErrSystemModelSchemaUnavailable
	}
	modelClass, err := c.validateSystemModelInput(input, modelID == nil)
	if err != nil {
		return 0, err
	}
	payload, err := c.buildSystemModelPayload(modelClass, input)
	if err != nil {
		return 0, err
	}
	accessMode, err := systemAccessModeToString(input.AccessMode)
	if err != nil {
		return 0, err
	}
	routingStrategy, err := systemRoutingStrategyToString(input.RoutingStrategy)
	if err != nil {
		return 0, err
	}

	var persistedID int64
	err = c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		nowMillis := now.UnixMilli()
		status := managedModelStatusOff
		if input.Enabled {
			status = managedModelStatusOn
		}
		legacyAPIKey := ""
		if modelID == nil || *modelID <= 0 {
			var maxSort int64
			if err := tx.Table(modelInstanceTable).
				Select("COALESCE(MAX(sort_order), 0)").
				Where("deleted_at IS NULL AND access_mode <> ?", systemModelAccessWorkspace).
				Scan(&maxSort).Error; err != nil {
				return fmt.Errorf("load system model sort order: %w", err)
			}
			row := &systemModelRow{
				Type:             int32(config.ModelType_LLM),
				Provider:         payload.providerJSON,
				DisplayInfo:      payload.displayJSON,
				Connection:       payload.connectionJSON,
				Capability:       payload.capabilityJSON,
				Parameters:       payload.parametersJSON,
				Extra:            payload.extraJSON,
				CreatedAt:        nowMillis,
				UpdatedAt:        nowMillis,
				ProviderKey:      normalizeProviderKey(input.ProviderKey),
				ModelIdentifier:  strings.TrimSpace(input.ModelIdentifier),
				Description:      strings.TrimSpace(input.GetDescription()),
				Status:           status,
				SortOrder:        maxSort + 1,
				CreatorID:        creatorID,
				Protocol:         strings.TrimSpace(input.Protocol),
				RoutingStrategy:  routingStrategy,
				AccessMode:       accessMode,
				ScenarioJSON:     payload.settingsJSON,
				ReasoningMode:    strings.TrimSpace(input.ReasoningMode),
				FunctionCallMode: strings.TrimSpace(input.FunctionCallMode),
				MaxContextTokens: input.MaxContextTokens,
				MaxOutputTokens:  input.MaxOutputTokens,
			}
			if err := tx.Table(modelInstanceTable).Create(row).Error; err != nil {
				return fmt.Errorf("create system model: %w", err)
			}
			persistedID = row.ID
		} else {
			persistedID = *modelID
			var current systemModelRow
			err := tx.Table(modelInstanceTable).
				Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", persistedID, systemModelAccessWorkspace).
				First(&current).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSystemModelNotFound
			}
			if err != nil {
				return fmt.Errorf("load system model: %w", err)
			}
			legacyAPIKey = legacySystemModelAPIKey(current.Connection)
			updates := map[string]any{
				"provider":           payload.providerJSON,
				"display_info":       payload.displayJSON,
				"connection":         payload.connectionJSON,
				"capability":         payload.capabilityJSON,
				"parameters":         payload.parametersJSON,
				"extra":              payload.extraJSON,
				"updated_at":         nowMillis,
				"provider_key":       normalizeProviderKey(input.ProviderKey),
				"model_identifier":   strings.TrimSpace(input.ModelIdentifier),
				"description":        strings.TrimSpace(input.GetDescription()),
				"status":             status,
				"protocol":           strings.TrimSpace(input.Protocol),
				"routing_strategy":   routingStrategy,
				"access_mode":        accessMode,
				"scenario_json":      payload.settingsJSON,
				"reasoning_mode":     strings.TrimSpace(input.ReasoningMode),
				"function_call_mode": strings.TrimSpace(input.FunctionCallMode),
				"max_context_tokens": input.MaxContextTokens,
				"max_output_tokens":  input.MaxOutputTokens,
			}
			result := tx.Table(modelInstanceTable).
				Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", persistedID, systemModelAccessWorkspace).
				Updates(updates)
			if result.Error != nil {
				return fmt.Errorf("update system model: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrSystemModelNotFound
			}
		}

		if err := c.replaceSystemModelEndpoints(tx, persistedID, normalizeProviderKey(input.ProviderKey), input.Endpoints, legacyAPIKey, now); err != nil {
			return err
		}
		if input.AccessMode == config.ModelAccessMode_ALL {
			if err := softDeleteSystemModelGrants(tx, persistedID, now); err != nil {
				return err
			}
		}
		if err := ensureDatabaseModelListEnabled(ctx, kvstore.New[struct{}](tx)); err != nil {
			return fmt.Errorf("enable database model list: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	ctxcache.Store(ctx, doNotUseOldModelFlagContextKey, false)
	return persistedID, nil
}

func (c *ModelConfig) DeleteSystemModel(ctx context.Context, modelID int64, preview bool) ([]*config.ModelDependencySummary, error) {
	if _, err := c.getSystemModelRow(ctx, modelID); err != nil {
		return nil, err
	}
	dependencies := []*config.ModelDependencySummary{}
	if preview {
		return dependencies, nil
	}
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Table(modelEndpointTable).
			Where("model_id = ? AND deleted_at IS NULL", modelID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("delete system model endpoints: %w", err)
		}
		if err := softDeleteSystemModelGrants(tx, modelID, now); err != nil {
			return err
		}
		result := tx.Table(modelInstanceTable).
			Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", modelID, systemModelAccessWorkspace).
			Updates(map[string]any{"deleted_at": now, "updated_at": now.UnixMilli()})
		if result.Error != nil {
			return fmt.Errorf("delete system model: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrSystemModelNotFound
		}
		return nil
	})
	return dependencies, err
}

func (c *ModelConfig) SetSystemModelStatus(ctx context.Context, modelID int64, enabled bool) error {
	status := managedModelStatusOff
	if enabled {
		status = managedModelStatusOn
	}
	result := c.db.WithContext(ctx).Table(modelInstanceTable).
		Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", modelID, systemModelAccessWorkspace).
		Updates(map[string]any{"status": status, "updated_at": time.Now().UnixMilli()})
	if result.Error != nil {
		return fmt.Errorf("update system model status: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrSystemModelNotFound
	}
	return nil
}

func (c *ModelConfig) SortSystemModels(ctx context.Context, items []*config.ModelSortItem) error {
	if len(items) == 0 {
		return fmt.Errorf("%w: sort items are required", ErrSystemModelInvalid)
	}
	seen := make(map[int64]struct{}, len(items))
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if item == nil || item.ID <= 0 {
				return fmt.Errorf("%w: invalid sort item", ErrSystemModelInvalid)
			}
			if _, ok := seen[item.ID]; ok {
				return fmt.Errorf("%w: duplicate model in sort request", ErrSystemModelInvalid)
			}
			seen[item.ID] = struct{}{}
			result := tx.Table(modelInstanceTable).
				Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", item.ID, systemModelAccessWorkspace).
				Updates(map[string]any{"sort_order": item.SortOrder, "updated_at": time.Now().UnixMilli()})
			if result.Error != nil {
				return fmt.Errorf("sort system model: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrSystemModelNotFound
			}
		}
		return nil
	})
}

func (c *ModelConfig) GetSystemModelGrants(ctx context.Context, modelID int64) (config.ModelAccessMode, []*config.ModelGrantSubject, error) {
	row, err := c.getSystemModelRow(ctx, modelID)
	if err != nil {
		return config.ModelAccessMode_ALL, nil, err
	}
	var grants []systemModelGrantRow
	if err := c.db.WithContext(ctx).Table(modelGrantTable).
		Where("model_id = ? AND deleted_at IS NULL", modelID).
		Order("subject_type ASC, subject_id ASC").Find(&grants).Error; err != nil {
		return config.ModelAccessMode_ALL, nil, fmt.Errorf("list system model grants: %w", err)
	}
	result := make([]*config.ModelGrantSubject, 0, len(grants))
	for _, grant := range grants {
		subjectType := config.ModelGrantSubjectType_USER
		if grant.SubjectType == modelGrantSubjectSpace {
			subjectType = config.ModelGrantSubjectType_WORKSPACE
		}
		result = append(result, &config.ModelGrantSubject{
			SubjectType: subjectType,
			SubjectID:   grant.SubjectID,
		})
	}
	return systemAccessModeFromString(row.AccessMode), result, nil
}

func (c *ModelConfig) SaveSystemModelGrants(
	ctx context.Context,
	modelID int64,
	accessMode config.ModelAccessMode,
	grants []*config.ModelGrantSubject,
) error {
	accessModeValue, err := systemAccessModeToString(accessMode)
	if err != nil {
		return err
	}
	if _, err := c.getSystemModelRow(ctx, modelID); err != nil {
		return err
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := softDeleteSystemModelGrants(tx, modelID, now); err != nil {
			return err
		}
		if accessMode == config.ModelAccessMode_RESTRICTED {
			rows := make([]systemModelGrantRow, 0, len(grants))
			seen := make(map[string]struct{}, len(grants))
			for _, grant := range grants {
				if grant == nil || grant.SubjectID <= 0 {
					return fmt.Errorf("%w: invalid authorization target", ErrSystemModelInvalid)
				}
				subjectType := systemModelGrantUser
				if grant.SubjectType == config.ModelGrantSubjectType_WORKSPACE {
					subjectType = modelGrantSubjectSpace
				} else if grant.SubjectType != config.ModelGrantSubjectType_USER {
					return fmt.Errorf("%w: unsupported authorization target", ErrSystemModelInvalid)
				}
				key := fmt.Sprintf("%s:%d", subjectType, grant.SubjectID)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				rows = append(rows, systemModelGrantRow{
					ModelID: modelID, SubjectType: subjectType, SubjectID: grant.SubjectID,
					CreatedAt: now, UpdatedAt: now,
				})
			}
			if len(rows) > 0 {
				if err := tx.Table(modelGrantTable).Create(&rows).Error; err != nil {
					return fmt.Errorf("save system model grants: %w", err)
				}
			}
		}
		result := tx.Table(modelInstanceTable).
			Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", modelID, systemModelAccessWorkspace).
			Updates(map[string]any{"access_mode": accessModeValue, "updated_at": now.UnixMilli()})
		if result.Error != nil {
			return fmt.Errorf("update system model access mode: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrSystemModelNotFound
		}
		return nil
	})
}

func (c *ModelConfig) TestSystemModelEndpoint(ctx context.Context, req *config.TestModelEndpointReq) (*SystemModelEndpointTestResult, error) {
	if req == nil || req.Endpoint == nil {
		return nil, fmt.Errorf("%w: endpoint is required", ErrSystemModelInvalid)
	}
	if err := validateSystemEndpointURL(req.Endpoint.BaseURL); err != nil {
		return nil, err
	}
	apiKey, err := c.resolveSystemEndpointCredential(ctx, req.GetModelID(), req.Endpoint)
	if err != nil {
		return nil, err
	}
	if apiKey == "" && normalizeProviderKey(req.ProviderKey) != "ollama" {
		return nil, fmt.Errorf("%w: API key is required", ErrSystemModelInvalid)
	}

	targetURL := workspaceModelProbeURL(req.Endpoint.BaseURL, req.Protocol)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: endpoint URL is invalid", ErrSystemModelInvalid)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Coze-Studio-System-Model-Connectivity/1.0")
	if apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
		request.Header.Set("x-api-key", apiKey)
	}
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
	latency := time.Since(started).Milliseconds()
	if err != nil {
		code, message := "network_error", "无法连接模型服务，请检查地址和网络后重试"
		return &SystemModelEndpointTestResult{LatencyMS: latency, ErrorCode: &code, ErrorMessage: &message}, nil
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		code, message := "provider_rejected", "模型服务拒绝了连接，请检查 API Key、模型标识和服务地址"
		if response.StatusCode >= http.StatusInternalServerError {
			code, message = "provider_unavailable", "模型服务暂时不可用，请稍后重试"
		}
		return &SystemModelEndpointTestResult{LatencyMS: latency, ErrorCode: &code, ErrorMessage: &message}, nil
	}
	return &SystemModelEndpointTestResult{Success: true, LatencyMS: latency}, nil
}

func (c *ModelConfig) validateSystemModelInput(input *config.ModelManagementInput, creating bool) (developer_api.ModelClass, error) {
	if input == nil {
		return 0, fmt.Errorf("%w: model configuration is required", ErrSystemModelInvalid)
	}
	providerKey := normalizeProviderKey(input.ProviderKey)
	if !workspaceProviderKeyPattern.MatchString(providerKey) {
		return 0, fmt.Errorf("%w: provider is invalid", ErrSystemModelInvalid)
	}
	modelClass, ok := systemModelClassForProviderKey(providerKey)
	if !ok {
		return 0, fmt.Errorf("%w: provider is unsupported", ErrSystemModelInvalid)
	}
	if name := strings.TrimSpace(input.Name); name == "" || len(name) > 128 {
		return 0, fmt.Errorf("%w: model name is invalid", ErrSystemModelInvalid)
	}
	if identifier := strings.TrimSpace(input.ModelIdentifier); identifier == "" || len(identifier) > 256 {
		return 0, fmt.Errorf("%w: model identifier is invalid", ErrSystemModelInvalid)
	}
	if len(strings.TrimSpace(input.GetDescription())) > 1024 {
		return 0, fmt.Errorf("%w: model description is too long", ErrSystemModelInvalid)
	}
	if len(input.CapabilityTypes) == 0 {
		return 0, fmt.Errorf("%w: at least one capability is required", ErrSystemModelInvalid)
	}
	if input.MaxContextTokens <= 0 || input.MaxOutputTokens <= 0 || input.MaxOutputTokens > input.MaxContextTokens {
		return 0, fmt.Errorf("%w: token limits are invalid", ErrSystemModelInvalid)
	}
	if _, err := systemAccessModeToString(input.AccessMode); err != nil {
		return 0, err
	}
	if _, err := systemRoutingStrategyToString(input.RoutingStrategy); err != nil {
		return 0, err
	}
	if len(input.Endpoints) == 0 || len(input.Endpoints) > 20 {
		return 0, fmt.Errorf("%w: one to twenty endpoints are required", ErrSystemModelInvalid)
	}
	hasEnabled := false
	for _, endpoint := range input.Endpoints {
		if endpoint == nil {
			return 0, fmt.Errorf("%w: endpoint is required", ErrSystemModelInvalid)
		}
		if err := validateSystemEndpointURL(endpoint.BaseURL); err != nil {
			return 0, err
		}
		if endpoint.Weight <= 0 || endpoint.Weight > 10000 {
			return 0, fmt.Errorf("%w: endpoint weight is invalid", ErrSystemModelInvalid)
		}
		if endpoint.Enabled {
			hasEnabled = true
		}
		if creating && strings.TrimSpace(endpoint.GetAPIKey()) == "" && providerKey != "ollama" {
			return 0, fmt.Errorf("%w: API key is required for a new endpoint", ErrSystemModelInvalid)
		}
	}
	if !hasEnabled {
		return 0, fmt.Errorf("%w: at least one endpoint must be enabled", ErrSystemModelInvalid)
	}
	return modelClass, nil
}

func (c *ModelConfig) buildSystemModelPayload(modelClass developer_api.ModelClass, input *config.ModelManagementInput) (*systemModelPayload, error) {
	provider, ok := GetModelProvider(modelClass)
	if !ok {
		return nil, fmt.Errorf("%w: provider is unsupported", ErrSystemModelInvalid)
	}
	meta, err := c.ModelMetaConf.GetModelMeta(modelClass, strings.TrimSpace(input.ModelIdentifier))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSystemModelInvalid, err)
	}
	if meta.DisplayInfo == nil {
		meta.DisplayInfo = &config.DisplayInfo{}
	}
	meta.DisplayInfo.Name = strings.TrimSpace(input.Name)
	meta.DisplayInfo.Description = &config.I18nText{
		ZhCn: strings.TrimSpace(input.GetDescription()),
		EnUs: strings.TrimSpace(input.GetDescription()),
	}
	meta.DisplayInfo.MaxTokens = input.MaxContextTokens
	meta.DisplayInfo.OutputTokens = input.MaxOutputTokens
	if meta.Connection == nil {
		meta.Connection = &config.Connection{}
	}
	if meta.Connection.BaseConnInfo == nil {
		meta.Connection.BaseConnInfo = &config.BaseConnectionInfo{}
	}
	meta.Connection.BaseConnInfo.Model = strings.TrimSpace(input.ModelIdentifier)
	meta.Connection.BaseConnInfo.BaseURL = strings.TrimSpace(input.Endpoints[0].BaseURL)
	meta.Connection.BaseConnInfo.APIKey = ""
	meta.Connection.BaseConnInfo.ThinkingType = systemThinkingType(input.ReasoningMode)
	settings := workspaceModelSettings{
		Capabilities:   normalizeStringList(input.CapabilityTypes),
		UsageScenarios: normalizeStringList(input.UsageScenarios),
	}
	extra := struct {
		EnableBase64URL bool `json:"enable_base64_url"`
	}{EnableBase64URL: input.GetEnableBase64URL()}
	values := []any{provider, meta.DisplayInfo, meta.Connection, meta.Capability, meta.Parameters, extra, settings}
	encoded := make([]string, len(values))
	for index, value := range values {
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return nil, fmt.Errorf("encode system model metadata: %w", marshalErr)
		}
		encoded[index] = string(data)
	}
	return &systemModelPayload{
		providerJSON: encoded[0], displayJSON: encoded[1], connectionJSON: encoded[2],
		capabilityJSON: encoded[3], parametersJSON: encoded[4], extraJSON: encoded[5], settingsJSON: encoded[6],
	}, nil
}

func (c *ModelConfig) replaceSystemModelEndpoints(
	tx *gorm.DB,
	modelID int64,
	providerKey string,
	inputs []*config.ModelEndpointInput,
	legacyAPIKey string,
	now time.Time,
) error {
	var existing []systemModelEndpointRow
	if err := tx.Table(modelEndpointTable).Where("model_id = ? AND deleted_at IS NULL", modelID).Find(&existing).Error; err != nil {
		return fmt.Errorf("load system model endpoints: %w", err)
	}
	existingByID := make(map[int64]systemModelEndpointRow, len(existing))
	for _, endpoint := range existing {
		existingByID[endpoint.ID] = endpoint
	}
	retained := make(map[int64]struct{}, len(inputs))
	for index, input := range inputs {
		weight := input.Weight
		if weight <= 0 {
			weight = 1
		}
		if input.GetID() <= 0 {
			apiKey := strings.TrimSpace(input.GetAPIKey())
			if apiKey == "" && index == 0 {
				apiKey = legacyAPIKey
			}
			if apiKey == "" && providerKey != "ollama" {
				return fmt.Errorf("%w: API key is required for a new endpoint", ErrSystemModelInvalid)
			}
			row := &systemModelEndpointRow{
				ModelID: modelID, BaseURL: strings.TrimSpace(input.BaseURL), Weight: weight,
				Enabled: input.Enabled, SortOrder: int32(index), CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Table(modelEndpointTable).Create(row).Error; err != nil {
				return fmt.Errorf("create system model endpoint: %w", err)
			}
			if apiKey != "" {
				if c.credentialCodec == nil {
					return ErrSystemModelCredential
				}
				envelope, fingerprint, err := c.credentialCodec.Encrypt(modelID, row.ID, apiKey)
				if err != nil {
					return fmt.Errorf("encrypt system model credential: %w", err)
				}
				if err := tx.Table(modelEndpointTable).Where("id = ? AND model_id = ?", row.ID, modelID).
					Updates(map[string]any{"api_key_envelope": envelope, "api_key_fingerprint": fingerprint}).Error; err != nil {
					return fmt.Errorf("persist system model credential: %w", err)
				}
			}
			retained[row.ID] = struct{}{}
			continue
		}

		current, ok := existingByID[input.GetID()]
		if !ok {
			return fmt.Errorf("%w: endpoint does not belong to model", ErrSystemModelInvalid)
		}
		updates := map[string]any{
			"base_url": strings.TrimSpace(input.BaseURL), "weight": weight, "enabled": input.Enabled,
			"sort_order": int32(index), "updated_at": now,
		}
		apiKey := strings.TrimSpace(input.GetAPIKey())
		if apiKey != "" {
			if c.credentialCodec == nil {
				return ErrSystemModelCredential
			}
			envelope, fingerprint, err := c.credentialCodec.Encrypt(modelID, current.ID, apiKey)
			if err != nil {
				return fmt.Errorf("encrypt system model credential: %w", err)
			}
			updates["api_key_envelope"] = envelope
			updates["api_key_fingerprint"] = fingerprint
		} else if input.GetClearAPIKey() {
			updates["api_key_envelope"] = ""
			updates["api_key_fingerprint"] = ""
		}
		if err := tx.Table(modelEndpointTable).
			Where("id = ? AND model_id = ? AND deleted_at IS NULL", current.ID, modelID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update system model endpoint: %w", err)
		}
		retained[current.ID] = struct{}{}
	}
	for _, endpoint := range existing {
		if _, ok := retained[endpoint.ID]; ok {
			continue
		}
		if err := tx.Table(modelEndpointTable).Where("id = ? AND model_id = ? AND deleted_at IS NULL", endpoint.ID, modelID).
			Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
			return fmt.Errorf("delete system model endpoint: %w", err)
		}
	}
	return nil
}

func (c *ModelConfig) listSystemModelEndpoints(ctx context.Context, row systemModelRow) ([]*config.ModelEndpointView, error) {
	var rows []systemModelEndpointRow
	if err := c.db.WithContext(ctx).Table(modelEndpointTable).
		Where("model_id = ? AND deleted_at IS NULL", row.ID).
		Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list system model endpoints: %w", err)
	}
	result := make([]*config.ModelEndpointView, 0, len(rows))
	for _, endpoint := range rows {
		result = append(result, &config.ModelEndpointView{
			ID: endpoint.ID, BaseURL: endpoint.BaseURL, HasAPIKey: endpoint.APIKeyEnvelope != "",
			Weight: endpoint.Weight, Enabled: endpoint.Enabled, SortOrder: endpoint.SortOrder,
		})
	}
	if len(result) > 0 {
		return result, nil
	}
	var connection config.Connection
	if err := json.Unmarshal([]byte(row.Connection), &connection); err != nil || connection.BaseConnInfo == nil {
		return result, nil
	}
	baseURL := strings.TrimSpace(connection.BaseConnInfo.BaseURL)
	if baseURL == "" {
		providerKey := normalizeProviderKey(row.ProviderKey)
		if providerKey == "" {
			var provider config.ModelProvider
			_ = json.Unmarshal([]byte(row.Provider), &provider)
			providerKey = providerKeyForClass(provider.ModelClass)
		}
		baseURL = systemModelDefaultBaseURL(providerKey)
	}
	if baseURL == "" {
		return result, nil
	}
	return []*config.ModelEndpointView{{
		ID: 0, BaseURL: baseURL,
		HasAPIKey: strings.TrimSpace(connection.BaseConnInfo.APIKey) != "",
		Weight:    1, Enabled: true, SortOrder: 0,
	}}, nil
}

func (c *ModelConfig) resolveSystemEndpointCredential(ctx context.Context, modelID int64, endpoint *config.ModelEndpointInput) (string, error) {
	if value := strings.TrimSpace(endpoint.GetAPIKey()); value != "" {
		return value, nil
	}
	if modelID <= 0 {
		return "", nil
	}
	if endpoint.GetID() > 0 {
		var stored systemModelEndpointRow
		err := c.db.WithContext(ctx).Table(modelEndpointTable).
			Where("id = ? AND model_id = ? AND deleted_at IS NULL", endpoint.GetID(), modelID).
			First(&stored).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", ErrSystemModelNotFound
		}
		if err != nil {
			return "", fmt.Errorf("load system model credential: %w", err)
		}
		if stored.APIKeyEnvelope == "" {
			return "", nil
		}
		if c.credentialCodec == nil {
			return "", ErrSystemModelCredential
		}
		return c.credentialCodec.Decrypt(modelID, stored.ID, stored.APIKeyEnvelope)
	}
	row, err := c.getSystemModelRow(ctx, modelID)
	if err != nil {
		return "", err
	}
	return legacySystemModelAPIKey(row.Connection), nil
}

func (c *ModelConfig) getSystemModelRow(ctx context.Context, modelID int64) (*systemModelRow, error) {
	if c == nil || c.db == nil || modelID <= 0 {
		return nil, ErrSystemModelNotFound
	}
	if !c.workspaceModelSchemaReady() {
		return nil, ErrSystemModelSchemaUnavailable
	}
	var row systemModelRow
	err := c.db.WithContext(ctx).Table(modelInstanceTable).
		Where("id = ? AND deleted_at IS NULL AND access_mode <> ?", modelID, systemModelAccessWorkspace).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSystemModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get system model: %w", err)
	}
	return &row, nil
}

func projectSystemModelSummary(row systemModelRow) (*config.ModelManagementItem, error) {
	var provider config.ModelProvider
	_ = json.Unmarshal([]byte(row.Provider), &provider)
	var display config.DisplayInfo
	_ = json.Unmarshal([]byte(row.DisplayInfo), &display)
	var connection config.Connection
	_ = json.Unmarshal([]byte(row.Connection), &connection)
	var settings workspaceModelSettings
	_ = json.Unmarshal([]byte(row.ScenarioJSON), &settings)
	providerKey := normalizeProviderKey(row.ProviderKey)
	if providerKey == "" {
		providerKey = providerKeyForClass(provider.ModelClass)
	}
	name := strings.TrimSpace(display.Name)
	if name == "" {
		name = strings.TrimSpace(row.ModelIdentifier)
	}
	identifier := strings.TrimSpace(row.ModelIdentifier)
	if identifier == "" && connection.BaseConnInfo != nil {
		identifier = strings.TrimSpace(connection.BaseConnInfo.Model)
	}
	description := strings.TrimSpace(row.Description)
	if description == "" && display.Description != nil {
		description = strings.TrimSpace(display.Description.ZhCn)
	}
	updatedAt := row.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = row.CreatedAt
	}
	sortOrder := row.SortOrder
	if sortOrder <= 0 {
		sortOrder = row.ID
	}
	capabilityTypes := normalizeStringList(settings.Capabilities)
	if len(capabilityTypes) == 0 {
		capabilityTypes = legacySystemModelCapabilities(row)
	}
	return &config.ModelManagementItem{
		ID: row.ID, ProviderKey: providerKey, ModelClass: provider.ModelClass,
		Name: name, ModelIdentifier: identifier, Description: systemOptionalString(description),
		CapabilityTypes: capabilityTypes, Enabled: row.Status != managedModelStatusOff,
		AccessMode: systemAccessModeFromString(row.AccessMode), CreatorID: row.CreatorID,
		UpdatedAtMs: updatedAt, SortOrder: sortOrder,
	}, nil
}

func legacySystemModelCapabilities(row systemModelRow) []string {
	capabilities := []string{"text"}
	ability := legacySystemModelAbility(row)
	if ability.GetImageUnderstanding() {
		capabilities = append(capabilities, "image")
	}
	if ability.GetAudioUnderstanding() {
		capabilities = append(capabilities, "audio")
	}
	if ability.GetVideoUnderstanding() {
		capabilities = append(capabilities, "video")
	}
	if ability.GetCotDisplay() {
		capabilities = append(capabilities, "reasoning")
	}
	return normalizeStringList(capabilities)
}

func legacySystemModelAbility(row systemModelRow) developer_api.ModelAbility {
	var ability developer_api.ModelAbility
	_ = json.Unmarshal([]byte(row.Capability), &ability)
	return ability
}

func systemModelReasoningMode(row systemModelRow) string {
	configured := strings.ToLower(strings.TrimSpace(row.ReasoningMode))
	if configured != "" && configured != "default" {
		return configured
	}
	var connection config.Connection
	if json.Unmarshal([]byte(row.Connection), &connection) == nil && connection.BaseConnInfo != nil {
		switch connection.BaseConnInfo.ThinkingType {
		case config.ThinkingType_Enable:
			return "enabled"
		case config.ThinkingType_Disable:
			return "disabled"
		case config.ThinkingType_Auto:
			return "auto"
		}
	}
	return "default"
}

func systemModelFunctionCallMode(row systemModelRow) string {
	configured := strings.ToLower(strings.TrimSpace(row.FunctionCallMode))
	if configured != "" && configured != "auto" {
		return configured
	}
	ability := legacySystemModelAbility(row)
	if ability.GetFunctionCall() {
		return "native"
	}
	return "auto"
}

func systemModelDefaultBaseURL(providerKey string) string {
	switch normalizeProviderKey(providerKey) {
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "ollama":
		return "http://localhost:11434/v1"
	default:
		return ""
	}
}

func systemModelClassForProviderKey(providerKey string) (developer_api.ModelClass, bool) {
	for _, provider := range getModelProviderList() {
		if provider != nil && providerKeyForClass(provider.ModelClass) == providerKey {
			return provider.ModelClass, true
		}
	}
	return 0, false
}

func systemAccessModeToString(value config.ModelAccessMode) (string, error) {
	switch value {
	case config.ModelAccessMode_ALL:
		return systemModelAccessAll, nil
	case config.ModelAccessMode_RESTRICTED:
		return systemModelAccessRestricted, nil
	default:
		return "", fmt.Errorf("%w: access mode is invalid", ErrSystemModelInvalid)
	}
}

func systemAccessModeFromString(value string) config.ModelAccessMode {
	if value == systemModelAccessRestricted {
		return config.ModelAccessMode_RESTRICTED
	}
	return config.ModelAccessMode_ALL
}

func systemRoutingStrategyToString(value config.ModelRoutingStrategy) (string, error) {
	switch value {
	case config.ModelRoutingStrategy_ROUND_ROBIN:
		return "round_robin", nil
	case config.ModelRoutingStrategy_WEIGHTED_ROUND_ROBIN:
		return "weighted_round_robin", nil
	default:
		return "", fmt.Errorf("%w: routing strategy is invalid", ErrSystemModelInvalid)
	}
}

func systemRoutingStrategyFromString(value string) config.ModelRoutingStrategy {
	if value == "weighted_round_robin" {
		return config.ModelRoutingStrategy_WEIGHTED_ROUND_ROBIN
	}
	return config.ModelRoutingStrategy_ROUND_ROBIN
}

func systemThinkingType(value string) config.ThinkingType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "enabled", "enable":
		return config.ThinkingType_Enable
	case "disabled", "disable":
		return config.ThinkingType_Disable
	case "auto":
		return config.ThinkingType_Auto
	default:
		return config.ThinkingType_Default
	}
}

func validateSystemEndpointURL(value string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: endpoint URL must be an HTTP or HTTPS URL", ErrSystemModelInvalid)
	}
	return nil
}

func softDeleteSystemModelGrants(tx *gorm.DB, modelID int64, now time.Time) error {
	if err := tx.Table(modelGrantTable).Where("model_id = ? AND deleted_at IS NULL", modelID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
		return fmt.Errorf("delete system model grants: %w", err)
	}
	return nil
}

func legacySystemModelAPIKey(connectionJSON string) string {
	var connection config.Connection
	if err := json.Unmarshal([]byte(connectionJSON), &connection); err != nil || connection.BaseConnInfo == nil {
		return ""
	}
	return strings.TrimSpace(connection.BaseConnInfo.APIKey)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func systemOptionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
