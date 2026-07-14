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

package mcptool

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const (
	MCPAESAuthSecretEnv         = "MCP_AES_AUTH_SECRET"
	mcpAESAuthEnvelopeVersion   = "aes-gcm-v1"
	mcpLegacyAuthMaxCASAttempts = 3
)

type MySQLCatalog struct {
	db        *gorm.DB
	authCodec MCPAuthCodec
}

type MCPAuthCodec interface {
	EncodeMCPAuth(ctx context.Context, auth string) (string, error)
	DecodeMCPAuth(ctx context.Context, stored string) (string, error)
}

type AESMCPAuthCodec struct {
	secret string
}

type mcpAESAuthEnvelope struct {
	Payload mcpAESAuthEnvelopePayload `json:"_coze_mcp_auth"`
}

type mcpAESAuthEnvelopePayload struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type MySQLCatalogOption func(*MySQLCatalog)

type mcpToolServerPO struct {
	ServerID        int64          `gorm:"column:server_id;primaryKey"`
	SpaceID         int64          `gorm:"column:space_id;index:idx_mcp_tool_servers_space_updated,priority:1;index:idx_mcp_tool_servers_space_enabled,priority:1;index:idx_mcp_tool_servers_space_source_updated,priority:1;uniqueIndex:uk_mcp_tool_servers_space_name_deleted,priority:1"`
	CreatorID       int64          `gorm:"column:creator_id;not null;default:0;index:idx_mcp_tool_servers_creator_updated,priority:1"`
	SourceType      string         `gorm:"column:source_type;size:32;not null;default:custom;index:idx_mcp_tool_servers_space_source_updated,priority:2"`
	Name            string         `gorm:"column:name;size:128;uniqueIndex:uk_mcp_tool_servers_space_name_deleted,priority:2"`
	Description     string         `gorm:"column:description;size:512"`
	ServerType      string         `gorm:"column:server_type;size:64"`
	Enabled         bool           `gorm:"column:enabled;index:idx_mcp_tool_servers_space_enabled,priority:2"`
	Config          datatypes.JSON `gorm:"column:config;type:json"`
	Auth            datatypes.JSON `gorm:"column:auth;type:json"`
	Tools           datatypes.JSON `gorm:"column:tools;type:json"`
	Resources       datatypes.JSON `gorm:"column:resources;type:json"`
	Prompts         datatypes.JSON `gorm:"column:prompts;type:json"`
	HealthStatus    string         `gorm:"column:health_status;size:32"`
	HealthCheckedAt int64          `gorm:"column:health_checked_at"`
	HealthLatencyMs int64          `gorm:"column:health_latency_ms"`
	HealthError     string         `gorm:"column:health_error;size:512"`
	CreatedAt       int64          `gorm:"column:created_at"`
	UpdatedAt       int64          `gorm:"column:updated_at;index:idx_mcp_tool_servers_space_updated,priority:2;index:idx_mcp_tool_servers_creator_updated,priority:2;index:idx_mcp_tool_servers_space_source_updated,priority:3"`
	DeletedAt       int64          `gorm:"column:deleted_at;index:idx_mcp_tool_servers_space_enabled,priority:3;uniqueIndex:uk_mcp_tool_servers_space_name_deleted,priority:3"`
}

type mcpResourcePO struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mime_type"`
}

func (mcpToolServerPO) TableName() string {
	return "mcp_tool_servers"
}

func NewMySQLCatalog(db *gorm.DB, options ...MySQLCatalogOption) *MySQLCatalog {
	catalog := &MySQLCatalog{
		db: db,
	}
	for _, option := range options {
		if option != nil {
			option(catalog)
		}
	}
	return catalog
}

func WithMySQLCatalogAuthCodec(codec MCPAuthCodec) MySQLCatalogOption {
	return func(catalog *MySQLCatalog) {
		if catalog != nil && codec != nil {
			catalog.authCodec = codec
		}
	}
}

func NewAESMCPAuthCodec(secret string) (*AESMCPAuthCodec, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("%s is required", MCPAESAuthSecretEnv)
	}
	switch len([]byte(secret)) {
	case 16, 24, 32:
		return &AESMCPAuthCodec{secret: secret}, nil
	default:
		return nil, fmt.Errorf("%s must be 16, 24, or 32 bytes", MCPAESAuthSecretEnv)
	}
}

func (c *AESMCPAuthCodec) EncodeMCPAuth(
	ctx context.Context,
	auth string,
) (string, error) {
	aead, err := c.newAEAD()
	if err != nil {
		return "", errors.New("mcp auth AES encryption failed")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", errors.New("mcp auth AES encryption failed")
	}
	ciphertext := aead.Seal(
		nil,
		nonce,
		[]byte(auth),
		[]byte(mcpAESAuthEnvelopeVersion),
	)
	encoded, err := json.Marshal(&mcpAESAuthEnvelope{
		Payload: mcpAESAuthEnvelopePayload{
			Version:    mcpAESAuthEnvelopeVersion,
			Nonce:      base64.RawURLEncoding.EncodeToString(nonce),
			Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
		},
	})
	if err != nil {
		return "", errors.New("mcp auth AES encryption failed")
	}

	return string(encoded), nil
}

func (c *AESMCPAuthCodec) DecodeMCPAuth(
	ctx context.Context,
	stored string,
) (string, error) {
	var outerEnvelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stored), &outerEnvelope); err != nil ||
		len(outerEnvelope) != 1 {
		return "", errors.New("mcp auth AES envelope is invalid")
	}
	payload, exists := outerEnvelope["_coze_mcp_auth"]
	if !exists {
		return "", errors.New("mcp auth AES envelope is invalid")
	}
	var envelope mcpAESAuthEnvelopePayload
	if err := json.Unmarshal(payload, &envelope); err != nil ||
		envelope.Version != mcpAESAuthEnvelopeVersion ||
		strings.TrimSpace(envelope.Nonce) == "" ||
		strings.TrimSpace(envelope.Ciphertext) == "" {
		return "", errors.New("mcp auth AES envelope is invalid")
	}
	aead, err := c.newAEAD()
	if err != nil {
		return "", errors.New("mcp auth AES decryption failed")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != aead.NonceSize() {
		return "", errors.New("mcp auth AES envelope is invalid")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < aead.Overhead() {
		return "", errors.New("mcp auth AES envelope is invalid")
	}
	decoded, err := aead.Open(
		nil,
		nonce,
		ciphertext,
		[]byte(mcpAESAuthEnvelopeVersion),
	)
	if err != nil || !json.Valid(decoded) {
		return "", errors.New("mcp auth AES decryption failed")
	}

	return string(decoded), nil
}

func (c *AESMCPAuthCodec) newAEAD() (cipher.AEAD, error) {
	if c == nil || c.secret == "" {
		return nil, errors.New("mcp auth AES codec is not configured")
	}
	block, err := aes.NewCipher([]byte(c.secret))
	if err != nil {
		return nil, errors.New("mcp auth AES codec is invalid")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("mcp auth AES codec is invalid")
	}

	return aead, nil
}

func (c *MySQLCatalog) Upsert(ctx context.Context, server *toolapi.MCPToolServer) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	po, err := c.serverToPO(ctx, server)
	if err != nil {
		return err
	}

	return c.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "server_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"space_id",
				"creator_id",
				"source_type",
				"name",
				"description",
				"server_type",
				"enabled",
				"config",
				"auth",
				"tools",
				"resources",
				"prompts",
				"health_status",
				"health_checked_at",
				"health_latency_ms",
				"health_error",
				"created_at",
				"updated_at",
				"deleted_at",
			}),
		}).
		Create(po).Error
}

func (c *MySQLCatalog) Create(ctx context.Context, server *toolapi.MCPToolServer) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	po, err := c.serverToPO(ctx, server)
	if err != nil {
		return err
	}
	if err := c.db.WithContext(ctx).Create(po).Error; err != nil {
		if isMCPToolDuplicateKeyError(err) {
			return ErrMCPConflict
		}
		return err
	}
	return nil
}

func (c *MySQLCatalog) UpdateServer(
	ctx context.Context,
	server *toolapi.MCPToolServer,
	expectedUpdatedAt int64,
) error {
	return c.updateServerFields(
		ctx,
		server,
		expectedUpdatedAt,
		MCPToolServerMutationConnectionFields,
	)
}

func (c *MySQLCatalog) updateServerFields(
	ctx context.Context,
	server *toolapi.MCPToolServer,
	expectedUpdatedAt int64,
	fieldMask MCPToolServerMutationFieldMask,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	if server == nil || server.UpdatedAt <= expectedUpdatedAt {
		return ErrMCPConflict
	}
	po, err := c.serverToPO(ctx, server)
	if err != nil {
		return err
	}
	fieldMask, err = normalizeMCPToolServerMutationFieldMask(fieldMask)
	if err != nil {
		return err
	}
	updates := map[string]any{"updated_at": po.UpdatedAt}
	if fieldMask&MCPToolServerMutationConnectionFields != 0 {
		updates["name"] = po.Name
		updates["description"] = po.Description
		updates["server_type"] = po.ServerType
		updates["enabled"] = po.Enabled
		updates["config"] = po.Config
		updates["auth"] = po.Auth
	}
	if fieldMask&MCPToolServerMutationCapabilityFields != 0 {
		updates["tools"] = po.Tools
		updates["resources"] = po.Resources
		updates["prompts"] = po.Prompts
	}
	if fieldMask&MCPToolServerMutationHealthFields != 0 {
		updates["health_status"] = po.HealthStatus
		updates["health_checked_at"] = po.HealthCheckedAt
		updates["health_latency_ms"] = po.HealthLatencyMs
		updates["health_error"] = po.HealthError
	}
	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where(
			"server_id = ? AND space_id = ? AND updated_at = ? AND deleted_at = 0",
			server.ServerID,
			server.SpaceID,
			expectedUpdatedAt,
		).
		UpdateColumns(updates)
	if db.Error != nil {
		if isMCPToolDuplicateKeyError(db.Error) {
			return ErrMCPConflict
		}
		return db.Error
	}
	if db.RowsAffected != 1 {
		return ErrMCPConflict
	}
	return nil
}

func (c *MySQLCatalog) DeleteServer(
	ctx context.Context,
	serverID int64,
	spaceID int64,
	expectedUpdatedAt int64,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where(
			"server_id = ? AND space_id = ? AND updated_at = ? AND deleted_at = 0",
			serverID,
			spaceID,
			expectedUpdatedAt,
		).
		UpdateColumn("deleted_at", serverID)
	if db.Error != nil {
		if isMCPToolDuplicateKeyError(db.Error) {
			return ErrMCPConflict
		}
		return db.Error
	}
	if db.RowsAffected != 1 {
		return ErrMCPConflict
	}
	return nil
}

func (c *MySQLCatalog) ApplyServers(
	ctx context.Context,
	mutations []MCPToolServerMutation,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	for _, mutation := range mutations {
		if err := validateMCPToolServerForWrite(mutation.Server); err != nil {
			return err
		}
		if _, err := normalizeMCPToolServerMutationFieldMask(mutation.FieldMask); err != nil {
			return err
		}
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCatalog := &MySQLCatalog{db: tx, authCodec: c.authCodec}
		for _, mutation := range mutations {
			var err error
			if mutation.ExpectedUpdatedAt > 0 {
				err = txCatalog.updateServerFields(
					ctx,
					mutation.Server,
					mutation.ExpectedUpdatedAt,
					mutation.FieldMask,
				)
			} else {
				err = txCatalog.Create(ctx, mutation.Server)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func isMCPToolDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate entry") || strings.Contains(message, "unique constraint failed")
}

func (c *MySQLCatalog) EnsureServers(
	ctx context.Context,
	servers []*toolapi.MCPToolServer,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	pos := make([]*mcpToolServerPO, 0, len(servers))
	for _, server := range servers {
		po, err := c.serverToPO(ctx, server)
		if err != nil {
			return err
		}
		if po.SpaceID <= 0 || strings.TrimSpace(po.Name) == "" {
			return errors.New("valid mcp tool server is required")
		}
		pos = append(pos, po)
	}
	if len(pos) == 0 {
		return nil
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&pos).Error; err != nil {
			return err
		}
		for _, po := range pos {
			var count int64
			if err := tx.Model(&mcpToolServerPO{}).
				Where(
					"space_id = ? AND name = ? AND deleted_at = 0",
					po.SpaceID,
					po.Name,
				).
				Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return ErrMCPConflict
			}
		}

		return nil
	})
}

func (c *MySQLCatalog) Get(ctx context.Context, serverID int64) (*toolapi.MCPToolServer, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("mcp tool catalog db is required")
	}

	var po mcpToolServerPO
	err := c.db.WithContext(ctx).
		Where("server_id = ? AND deleted_at = 0", serverID).
		First(&po).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return c.poToAPI(ctx, &po)
}

func (c *MySQLCatalog) List(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolServer, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("mcp tool catalog db is required")
	}

	pos := make([]*mcpToolServerPO, 0)
	if err := c.db.WithContext(ctx).
		Where("space_id = ? AND deleted_at = 0", spaceID).
		Order("updated_at DESC, server_id DESC").
		Find(&pos).Error; err != nil {
		return nil, err
	}

	servers := make([]*toolapi.MCPToolServer, 0, len(pos))
	for _, po := range pos {
		server, err := c.poToAPI(ctx, po)
		if err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}

	return servers, nil
}

func (c *MySQLCatalog) Delete(ctx context.Context, serverID int64) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}

	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where("server_id = ? AND deleted_at = 0", serverID).
		Update("deleted_at", time.Now().UnixMilli())
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (c *MySQLCatalog) UpdateCapabilities(
	ctx context.Context,
	serverID int64,
	spaceID int64,
	expectedUpdatedAt int64,
	updatedAt int64,
	capabilities MCPToolCapabilitySnapshot,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}
	tools, err := json.Marshal(cloneToolDefinitions(capabilities.Tools))
	if err != nil {
		return fmt.Errorf("marshal mcp tools: %w", err)
	}
	resources, err := json.Marshal(mcpResourcesToPO(capabilities.Resources))
	if err != nil {
		return fmt.Errorf("marshal mcp resources: %w", err)
	}
	prompts, err := json.Marshal(cloneCatalogMCPPrompts(capabilities.Prompts))
	if err != nil {
		return fmt.Errorf("marshal mcp prompts: %w", err)
	}

	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where(
			"server_id = ? AND space_id = ? AND updated_at = ? AND deleted_at = 0",
			serverID,
			spaceID,
			expectedUpdatedAt,
		).
		Updates(map[string]any{
			"tools":      datatypes.JSON(tools),
			"resources":  datatypes.JSON(resources),
			"prompts":    datatypes.JSON(prompts),
			"updated_at": updatedAt,
		})
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected != 1 {
		return ErrMCPConflict
	}

	return nil
}

func (c *MySQLCatalog) UpdateEnabled(
	ctx context.Context,
	serverID int64,
	spaceID int64,
	expectedUpdatedAt int64,
	updatedAt int64,
	enabled bool,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}

	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where(
			"server_id = ? AND space_id = ? AND updated_at = ? AND deleted_at = 0",
			serverID,
			spaceID,
			expectedUpdatedAt,
		).
		Updates(map[string]any{
			"enabled":    enabled,
			"updated_at": updatedAt,
		})
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected != 1 {
		return ErrMCPConflict
	}

	return nil
}

func (c *MySQLCatalog) UpdateHealth(
	ctx context.Context,
	serverID int64,
	expectedUpdatedAt int64,
	health MCPToolHealthSnapshot,
) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}

	query := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where("server_id = ? AND deleted_at = 0", serverID)
	if expectedUpdatedAt > 0 {
		query = query.Where("updated_at = ?", expectedUpdatedAt)
	}
	db := query.
		Where("health_checked_at < ?", health.CheckedAt).
		UpdateColumns(map[string]any{
			"health_status":     normalizeMCPToolHealthStatus(health.Status),
			"health_checked_at": health.CheckedAt,
			"health_latency_ms": health.LatencyMs,
			"health_error":      boundedMCPToolHealthError(health.Error),
		})
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 1 {
		return nil
	}

	var current mcpToolServerPO
	err := c.db.WithContext(ctx).
		Where("server_id = ? AND deleted_at = 0", serverID).
		Take(&current).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if expectedUpdatedAt > 0 && current.UpdatedAt != expectedUpdatedAt {
		return ErrMCPConflict
	}
	if current.HealthCheckedAt >= health.CheckedAt {
		return nil
	}

	return ErrMCPConflict
}

func (c *MySQLCatalog) serverToPO(
	ctx context.Context,
	server *toolapi.MCPToolServer,
) (*mcpToolServerPO, error) {
	if server == nil {
		return nil, errors.New("mcp tool server is required")
	}
	if server.ServerID <= 0 {
		return nil, errors.New("mcp tool server_id is required")
	}

	tools, err := json.Marshal(cloneToolDefinitions(server.Tools))
	if err != nil {
		return nil, fmt.Errorf("marshal mcp tools: %w", err)
	}
	resources, err := json.Marshal(mcpResourcesToPO(server.Resources))
	if err != nil {
		return nil, fmt.Errorf("marshal mcp resources: %w", err)
	}
	prompts, err := json.Marshal(cloneCatalogMCPPrompts(server.Prompts))
	if err != nil {
		return nil, fmt.Errorf("marshal mcp prompts: %w", err)
	}
	normalizedConfig := normalizeCatalogJSONText(server.Config, "{}")
	if err := validatePersistableMCPConfig(server.ServerType, normalizedConfig); err != nil {
		return nil, err
	}
	encodedAuth, err := c.encodeAuth(ctx, normalizeCatalogJSONText(server.Auth, "{}"))
	if err != nil {
		return nil, err
	}

	return &mcpToolServerPO{
		ServerID:        server.ServerID,
		SpaceID:         server.SpaceID,
		CreatorID:       server.CreatorID,
		SourceType:      string(normalizeCatalogMCPServerSourceType(server.SourceType)),
		Name:            strings.TrimSpace(server.Name),
		Description:     strings.TrimSpace(server.Description),
		ServerType:      strings.TrimSpace(server.ServerType),
		Enabled:         server.Enabled,
		Config:          datatypes.JSON(normalizedConfig),
		Auth:            datatypes.JSON(encodedAuth),
		Tools:           datatypes.JSON(tools),
		Resources:       datatypes.JSON(resources),
		Prompts:         datatypes.JSON(prompts),
		HealthStatus:    normalizeMCPToolHealthStatus(server.HealthStatus),
		HealthCheckedAt: server.HealthCheckedAt,
		HealthLatencyMs: server.HealthLatencyMs,
		HealthError:     boundedMCPToolHealthError(server.HealthError),
		CreatedAt:       server.CreatedAt,
		UpdatedAt:       server.UpdatedAt,
		DeletedAt:       0,
	}, nil
}

func (c *MySQLCatalog) poToAPI(
	ctx context.Context,
	po *mcpToolServerPO,
) (*toolapi.MCPToolServer, error) {
	if po == nil {
		return nil, ErrNotFound
	}
	currentPO, auth, err := c.resolveAuthForRead(ctx, po)
	if err != nil {
		return nil, err
	}
	po = currentPO

	tools := make([]*toolapi.MCPToolDefinition, 0)
	if len(po.Tools) > 0 {
		if err := json.Unmarshal(po.Tools, &tools); err != nil {
			return nil, fmt.Errorf("unmarshal mcp tools: %w", err)
		}
	}
	resourcePOs := make([]mcpResourcePO, 0)
	if len(po.Resources) > 0 {
		if err := json.Unmarshal(po.Resources, &resourcePOs); err != nil {
			return nil, fmt.Errorf("unmarshal mcp resources: %w", err)
		}
	}
	resources := mcpResourcesFromPO(resourcePOs)
	prompts := make([]*toolapi.MCPPrompt, 0)
	if len(po.Prompts) > 0 {
		if err := json.Unmarshal(po.Prompts, &prompts); err != nil {
			return nil, fmt.Errorf("unmarshal mcp prompts: %w", err)
		}
	}
	return &toolapi.MCPToolServer{
		ServerID:        po.ServerID,
		SpaceID:         po.SpaceID,
		CreatorID:       po.CreatorID,
		SourceType:      normalizeCatalogMCPServerSourceType(toolapi.MCPServerSourceType(po.SourceType)),
		Name:            po.Name,
		Description:     po.Description,
		ServerType:      po.ServerType,
		Enabled:         po.Enabled,
		Config:          string(po.Config),
		Auth:            auth,
		Tools:           cloneToolDefinitions(tools),
		Resources:       cloneCatalogMCPResources(resources),
		Prompts:         cloneCatalogMCPPrompts(prompts),
		HealthStatus:    normalizeMCPToolHealthStatus(po.HealthStatus),
		HealthCheckedAt: po.HealthCheckedAt,
		HealthLatencyMs: po.HealthLatencyMs,
		HealthError:     boundedMCPToolHealthError(po.HealthError),
		CreatedAt:       po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}, nil
}

func mcpResourcesToPO(resources []*toolapi.MCPResource) []mcpResourcePO {
	result := make([]mcpResourcePO, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		result = append(result, mcpResourcePO{
			URI: strings.TrimSpace(resource.URI), Name: resource.Name,
			Description: resource.Description, MIMEType: resource.MIMEType,
		})
	}
	return result
}

func mcpResourcesFromPO(resources []mcpResourcePO) []*toolapi.MCPResource {
	result := make([]*toolapi.MCPResource, 0, len(resources))
	for _, resource := range resources {
		result = append(result, &toolapi.MCPResource{
			URI: resource.URI, Name: resource.Name,
			Description: resource.Description, MIMEType: resource.MIMEType,
		})
	}
	return result
}

func (c *MySQLCatalog) encodeAuth(ctx context.Context, auth string) (string, error) {
	empty, err := validateCatalogMCPAuthObject(auth)
	if err != nil {
		return "", err
	}
	if empty {
		return "{}", nil
	}
	if c == nil || c.authCodec == nil {
		return "", errors.New("mcp tool auth codec is required")
	}
	encoded, err := c.authCodec.EncodeMCPAuth(ctx, auth)
	if err != nil {
		return "", errors.New("mcp tool auth encode failed")
	}
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || !json.Valid([]byte(encoded)) {
		return "", errors.New("mcp tool auth encode failed")
	}

	return encoded, nil
}

func (c *MySQLCatalog) decodeAuth(ctx context.Context, stored string) (string, error) {
	if isEmptyCatalogMCPAuth(stored) {
		return "{}", nil
	}
	if c == nil || c.authCodec == nil {
		return "", errors.New("mcp tool auth codec is required")
	}
	decoded, err := c.authCodec.DecodeMCPAuth(ctx, stored)
	if err != nil {
		return "", errors.New("mcp tool auth decode failed")
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" || !json.Valid([]byte(decoded)) {
		return "", errors.New("mcp tool auth decode failed")
	}

	return decoded, nil
}

func (c *MySQLCatalog) decodeAuthForRead(
	ctx context.Context,
	stored string,
) (auth string, migratedAuth string, err error) {
	if isEmptyCatalogMCPAuth(stored) {
		return "{}", "", nil
	}
	if isLegacyCatalogMCPAuth(stored) {
		codec, ok := c.authCodec.(*AESMCPAuthCodec)
		if !ok || codec == nil {
			return "", "", errors.New("mcp tool legacy auth migration requires AES-GCM codec")
		}
		encoded, err := codec.EncodeMCPAuth(ctx, stored)
		if err != nil {
			return "", "", errors.New("mcp tool legacy auth re-encryption failed")
		}

		return stored, encoded, nil
	}
	decoded, err := c.decodeAuth(ctx, stored)
	if err != nil {
		return "", "", err
	}

	return decoded, "", nil
}

func (c *MySQLCatalog) resolveAuthForRead(
	ctx context.Context,
	po *mcpToolServerPO,
) (*mcpToolServerPO, string, error) {
	current := po
	for attempt := 0; ; attempt++ {
		stored := normalizeCatalogJSONText(string(current.Auth), "{}")
		auth, migratedAuth, err := c.decodeAuthForRead(ctx, stored)
		if err != nil {
			return nil, "", err
		}
		if migratedAuth == "" {
			return current, auth, nil
		}
		if attempt >= mcpLegacyAuthMaxCASAttempts {
			return nil, "", errors.New("mcp tool legacy auth migration conflict")
		}
		updated, err := c.persistMigratedAuth(ctx, current.ServerID, stored, migratedAuth)
		if err != nil {
			return nil, "", err
		}
		if updated {
			return current, auth, nil
		}
		current, err = c.reloadMCPToolServerPO(ctx, current.ServerID)
		if err != nil {
			return nil, "", err
		}
	}
}

func (c *MySQLCatalog) persistMigratedAuth(
	ctx context.Context,
	serverID int64,
	original string,
	encoded string,
) (bool, error) {
	if c == nil || c.db == nil {
		return false, errors.New("mcp tool legacy auth migration failed")
	}
	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where(
			"server_id = ? AND deleted_at = 0 AND auth = ?",
			serverID,
			datatypes.JSON(original),
		).
		Update("auth", datatypes.JSON(encoded))
	if db.Error != nil {
		return false, errors.New("mcp tool legacy auth migration failed")
	}

	return db.RowsAffected == 1, nil
}

func (c *MySQLCatalog) reloadMCPToolServerPO(
	ctx context.Context,
	serverID int64,
) (*mcpToolServerPO, error) {
	if c == nil || c.db == nil {
		return nil, errors.New("mcp tool legacy auth migration conflict")
	}
	var po mcpToolServerPO
	if err := c.db.WithContext(ctx).
		Where("server_id = ? AND deleted_at = 0", serverID).
		First(&po).Error; err != nil {
		return nil, errors.New("mcp tool legacy auth migration conflict")
	}

	return &po, nil
}

func normalizeCatalogJSONText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}

func isEmptyCatalogMCPAuth(value string) bool {
	empty, err := validateCatalogMCPAuthObject(value)

	return err == nil && empty
}

func validateCatalogMCPAuthObject(value string) (bool, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &payload); err != nil || payload == nil {
		return false, errors.New("mcp tool auth must be a JSON object")
	}

	return len(payload) == 0, nil
}

func isLegacyCatalogMCPAuth(value string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &payload); err != nil ||
		payload == nil || len(payload) == 0 {
		return false
	}
	if _, exists := payload["_coze_mcp_auth"]; exists {
		return false
	}

	return true
}
