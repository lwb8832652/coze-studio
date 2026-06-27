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

type MySQLCatalog struct {
	db        *gorm.DB
	authCodec MCPAuthCodec
}

type MCPAuthCodec interface {
	EncodeMCPAuth(ctx context.Context, auth string) (string, error)
	DecodeMCPAuth(ctx context.Context, stored string) (string, error)
}

type MySQLCatalogOption func(*MySQLCatalog)

type passthroughMCPAuthCodec struct{}

type mcpToolServerPO struct {
	ServerID        int64          `gorm:"column:server_id;primaryKey"`
	SpaceID         int64          `gorm:"column:space_id;index:idx_mcp_tool_servers_space_updated"`
	Name            string         `gorm:"column:name;size:128"`
	Description     string         `gorm:"column:description;size:512"`
	ServerType      string         `gorm:"column:server_type;size:64"`
	Enabled         bool           `gorm:"column:enabled;index:idx_mcp_tool_servers_space_enabled"`
	Config          datatypes.JSON `gorm:"column:config;type:json"`
	Auth            datatypes.JSON `gorm:"column:auth;type:json"`
	Tools           datatypes.JSON `gorm:"column:tools;type:json"`
	HealthStatus    string         `gorm:"column:health_status;size:32"`
	HealthCheckedAt int64          `gorm:"column:health_checked_at"`
	HealthLatencyMs int64          `gorm:"column:health_latency_ms"`
	HealthError     string         `gorm:"column:health_error;size:512"`
	CreatedAt       int64          `gorm:"column:created_at"`
	UpdatedAt       int64          `gorm:"column:updated_at;index:idx_mcp_tool_servers_space_updated"`
	DeletedAt       int64          `gorm:"column:deleted_at;index:idx_mcp_tool_servers_space_enabled"`
}

func (mcpToolServerPO) TableName() string {
	return "mcp_tool_servers"
}

func NewMySQLCatalog(db *gorm.DB, options ...MySQLCatalogOption) *MySQLCatalog {
	catalog := &MySQLCatalog{
		db:        db,
		authCodec: passthroughMCPAuthCodec{},
	}
	for _, option := range options {
		if option != nil {
			option(catalog)
		}
	}
	if catalog.authCodec == nil {
		catalog.authCodec = passthroughMCPAuthCodec{}
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

func (passthroughMCPAuthCodec) EncodeMCPAuth(
	ctx context.Context,
	auth string,
) (string, error) {
	return auth, nil
}

func (passthroughMCPAuthCodec) DecodeMCPAuth(
	ctx context.Context,
	stored string,
) (string, error) {
	return stored, nil
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
				"name",
				"description",
				"server_type",
				"enabled",
				"config",
				"auth",
				"tools",
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

func (c *MySQLCatalog) UpdateHealth(ctx context.Context, serverID int64, health MCPToolHealthSnapshot) error {
	if c == nil || c.db == nil {
		return errors.New("mcp tool catalog db is required")
	}

	db := c.db.WithContext(ctx).
		Model(&mcpToolServerPO{}).
		Where("server_id = ? AND deleted_at = 0", serverID).
		Updates(map[string]any{
			"health_status":     normalizeMCPToolHealthStatus(health.Status),
			"health_checked_at": health.CheckedAt,
			"health_latency_ms": health.LatencyMs,
			"health_error":      boundedMCPToolHealthError(health.Error),
		})
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return ErrNotFound
	}

	return nil
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
	encodedAuth, err := c.encodeAuth(ctx, normalizeCatalogJSONText(server.Auth, "{}"))
	if err != nil {
		return nil, err
	}

	return &mcpToolServerPO{
		ServerID:        server.ServerID,
		SpaceID:         server.SpaceID,
		Name:            strings.TrimSpace(server.Name),
		Description:     strings.TrimSpace(server.Description),
		ServerType:      strings.TrimSpace(server.ServerType),
		Enabled:         server.Enabled,
		Config:          datatypes.JSON(normalizeCatalogJSONText(server.Config, "{}")),
		Auth:            datatypes.JSON(encodedAuth),
		Tools:           datatypes.JSON(tools),
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

	tools := make([]*toolapi.MCPToolDefinition, 0)
	if len(po.Tools) > 0 {
		if err := json.Unmarshal(po.Tools, &tools); err != nil {
			return nil, fmt.Errorf("unmarshal mcp tools: %w", err)
		}
	}
	auth, err := c.decodeAuth(ctx, normalizeCatalogJSONText(string(po.Auth), "{}"))
	if err != nil {
		return nil, err
	}

	return &toolapi.MCPToolServer{
		ServerID:        po.ServerID,
		SpaceID:         po.SpaceID,
		Name:            po.Name,
		Description:     po.Description,
		ServerType:      po.ServerType,
		Enabled:         po.Enabled,
		Config:          string(po.Config),
		Auth:            auth,
		Tools:           cloneToolDefinitions(tools),
		HealthStatus:    normalizeMCPToolHealthStatus(po.HealthStatus),
		HealthCheckedAt: po.HealthCheckedAt,
		HealthLatencyMs: po.HealthLatencyMs,
		HealthError:     boundedMCPToolHealthError(po.HealthError),
		CreatedAt:       po.CreatedAt,
		UpdatedAt:       po.UpdatedAt,
	}, nil
}

func (c *MySQLCatalog) encodeAuth(ctx context.Context, auth string) (string, error) {
	codec := c.authCodecOrDefault()
	encoded, err := codec.EncodeMCPAuth(ctx, auth)
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
	codec := c.authCodecOrDefault()
	decoded, err := codec.DecodeMCPAuth(ctx, stored)
	if err != nil {
		return "", errors.New("mcp tool auth decode failed")
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" || !json.Valid([]byte(decoded)) {
		return "", errors.New("mcp tool auth decode failed")
	}

	return decoded, nil
}

func (c *MySQLCatalog) authCodecOrDefault() MCPAuthCodec {
	if c == nil || c.authCodec == nil {
		return passthroughMCPAuthCodec{}
	}

	return c.authCodec
}

func normalizeCatalogJSONText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}
