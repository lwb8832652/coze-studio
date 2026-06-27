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
	"errors"
	"sort"
	"strings"
	"sync"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

var ErrNotFound = errors.New("mcp tool server not found")

type InMemoryCatalog struct {
	mu      sync.RWMutex
	servers map[int64]*toolapi.MCPToolServer
}

func NewInMemoryCatalog() *InMemoryCatalog {
	return &InMemoryCatalog{
		servers: make(map[int64]*toolapi.MCPToolServer),
	}
}

func (c *InMemoryCatalog) Upsert(ctx context.Context, server *toolapi.MCPToolServer) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}
	if server == nil {
		return errors.New("mcp tool server is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.servers == nil {
		c.servers = make(map[int64]*toolapi.MCPToolServer)
	}
	c.servers[server.ServerID] = cloneServer(server)

	return nil
}

func (c *InMemoryCatalog) Get(ctx context.Context, serverID int64) (*toolapi.MCPToolServer, error) {
	if c == nil {
		return nil, errors.New("mcp tool catalog is required")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	server := c.servers[serverID]
	if server == nil {
		return nil, ErrNotFound
	}

	return cloneServer(server), nil
}

func (c *InMemoryCatalog) List(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolServer, error) {
	if c == nil {
		return nil, errors.New("mcp tool catalog is required")
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	servers := make([]*toolapi.MCPToolServer, 0)
	for _, server := range c.servers {
		if server.SpaceID == spaceID {
			servers = append(servers, cloneServer(server))
		}
	}
	sort.Slice(servers, func(i, j int) bool {
		if servers[i].UpdatedAt == servers[j].UpdatedAt {
			return servers[i].ServerID > servers[j].ServerID
		}

		return servers[i].UpdatedAt > servers[j].UpdatedAt
	})

	return servers, nil
}

func (c *InMemoryCatalog) Delete(ctx context.Context, serverID int64) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.servers[serverID] == nil {
		return ErrNotFound
	}
	delete(c.servers, serverID)

	return nil
}

func (c *InMemoryCatalog) UpdateHealth(ctx context.Context, serverID int64, health MCPToolHealthSnapshot) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	server := c.servers[serverID]
	if server == nil {
		return ErrNotFound
	}
	updated := cloneServer(server)
	updated.HealthStatus = normalizeMCPToolHealthStatus(health.Status)
	updated.HealthCheckedAt = health.CheckedAt
	updated.HealthLatencyMs = health.LatencyMs
	updated.HealthError = boundedMCPToolHealthError(health.Error)
	c.servers[serverID] = updated

	return nil
}

func cloneServers(servers []*toolapi.MCPToolServer) []*toolapi.MCPToolServer {
	result := make([]*toolapi.MCPToolServer, 0, len(servers))
	for _, server := range servers {
		result = append(result, cloneServer(server))
	}

	return result
}

func cloneServer(server *toolapi.MCPToolServer) *toolapi.MCPToolServer {
	if server == nil {
		return nil
	}

	return &toolapi.MCPToolServer{
		ServerID:        server.ServerID,
		SpaceID:         server.SpaceID,
		Name:            server.Name,
		Description:     server.Description,
		ServerType:      server.ServerType,
		Enabled:         server.Enabled,
		Config:          server.Config,
		Auth:            server.Auth,
		Tools:           cloneToolDefinitions(server.Tools),
		HealthStatus:    normalizeMCPToolHealthStatus(server.HealthStatus),
		HealthCheckedAt: server.HealthCheckedAt,
		HealthLatencyMs: server.HealthLatencyMs,
		HealthError:     boundedMCPToolHealthError(server.HealthError),
		CreatedAt:       server.CreatedAt,
		UpdatedAt:       server.UpdatedAt,
	}
}

func cloneToolDefinitions(tools []*toolapi.MCPToolDefinition) []*toolapi.MCPToolDefinition {
	result := make([]*toolapi.MCPToolDefinition, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		result = append(result, &toolapi.MCPToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	return result
}

func normalizeMCPToolHealthStatus(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return mcpToolHealthStatusUnknown
	}

	return status
}

func boundedMCPToolHealthError(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= 256 {
		return value
	}

	return string([]rune(value)[:256])
}
