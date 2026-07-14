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
	"fmt"
	"sort"
	"strings"
	"sync"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

var (
	ErrNotFound    = errors.New("mcp tool server not found")
	ErrMCPConflict = errors.New("mcp tool server version conflict")
)

type MCPToolCapabilitySnapshot struct {
	Tools     []*toolapi.MCPToolDefinition
	Resources []*toolapi.MCPResource
	Prompts   []*toolapi.MCPPrompt
}

type MCPToolServerMutation struct {
	Server            *toolapi.MCPToolServer
	ExpectedUpdatedAt int64
	FieldMask         MCPToolServerMutationFieldMask
}

type MCPToolServerMutationFieldMask uint8

const (
	MCPToolServerMutationConnectionFields MCPToolServerMutationFieldMask = 1 << iota
	MCPToolServerMutationCapabilityFields
	MCPToolServerMutationHealthFields
)

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

func (c *InMemoryCatalog) Create(ctx context.Context, server *toolapi.MCPToolServer) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}
	if err := validateMCPToolServerForWrite(server); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.servers == nil {
		c.servers = make(map[int64]*toolapi.MCPToolServer)
	}
	return createInMemoryMCPToolServer(c.servers, server)
}

func (c *InMemoryCatalog) UpdateServer(
	ctx context.Context,
	server *toolapi.MCPToolServer,
	expectedUpdatedAt int64,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}
	if err := validateMCPToolServerForWrite(server); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return updateInMemoryMCPToolServer(
		c.servers,
		server,
		expectedUpdatedAt,
		MCPToolServerMutationConnectionFields,
	)
}

func (c *InMemoryCatalog) DeleteServer(
	ctx context.Context,
	serverID int64,
	spaceID int64,
	expectedUpdatedAt int64,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	server := c.servers[serverID]
	if server == nil || server.SpaceID != spaceID || server.UpdatedAt != expectedUpdatedAt {
		return ErrMCPConflict
	}
	delete(c.servers, serverID)
	return nil
}

func (c *InMemoryCatalog) ApplyServers(
	ctx context.Context,
	mutations []MCPToolServerMutation,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}
	for _, mutation := range mutations {
		if err := validateMCPToolServerForWrite(mutation.Server); err != nil {
			return err
		}
		if _, err := normalizeMCPToolServerMutationFieldMask(mutation.FieldMask); err != nil {
			return err
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	next := make(map[int64]*toolapi.MCPToolServer, len(c.servers)+len(mutations))
	for serverID, server := range c.servers {
		next[serverID] = cloneServer(server)
	}
	for _, mutation := range mutations {
		var err error
		if mutation.ExpectedUpdatedAt > 0 {
			fieldMask, maskErr := normalizeMCPToolServerMutationFieldMask(mutation.FieldMask)
			if maskErr != nil {
				return maskErr
			}
			err = updateInMemoryMCPToolServer(
				next,
				mutation.Server,
				mutation.ExpectedUpdatedAt,
				fieldMask,
			)
		} else {
			err = createInMemoryMCPToolServer(next, mutation.Server)
		}
		if err != nil {
			return err
		}
	}
	c.servers = next
	return nil
}

func validateMCPToolServerForWrite(server *toolapi.MCPToolServer) error {
	if server == nil || server.ServerID <= 0 || server.SpaceID <= 0 || strings.TrimSpace(server.Name) == "" {
		return errors.New("valid mcp tool server is required")
	}
	return nil
}

func createInMemoryMCPToolServer(
	servers map[int64]*toolapi.MCPToolServer,
	server *toolapi.MCPToolServer,
) error {
	if servers[server.ServerID] != nil {
		return ErrMCPConflict
	}
	nameKey := mcpCatalogLiveNameKey(server.SpaceID, server.Name)
	for _, stored := range servers {
		if stored != nil && mcpCatalogLiveNameKey(stored.SpaceID, stored.Name) == nameKey {
			return ErrMCPConflict
		}
	}
	servers[server.ServerID] = cloneServer(server)
	return nil
}

func updateInMemoryMCPToolServer(
	servers map[int64]*toolapi.MCPToolServer,
	server *toolapi.MCPToolServer,
	expectedUpdatedAt int64,
	fieldMask MCPToolServerMutationFieldMask,
) error {
	stored := servers[server.ServerID]
	if stored == nil || stored.SpaceID != server.SpaceID || stored.UpdatedAt != expectedUpdatedAt || server.UpdatedAt <= expectedUpdatedAt {
		return ErrMCPConflict
	}
	if fieldMask&MCPToolServerMutationConnectionFields != 0 {
		nameKey := mcpCatalogLiveNameKey(server.SpaceID, server.Name)
		for serverID, candidate := range servers {
			if serverID != server.ServerID && candidate != nil && mcpCatalogLiveNameKey(candidate.SpaceID, candidate.Name) == nameKey {
				return ErrMCPConflict
			}
		}
	}
	updated := cloneServer(stored)
	if fieldMask&MCPToolServerMutationConnectionFields != 0 {
		updated.Name = server.Name
		updated.Description = server.Description
		updated.ServerType = server.ServerType
		updated.Enabled = server.Enabled
		updated.Config = server.Config
		updated.Auth = server.Auth
	}
	if fieldMask&MCPToolServerMutationCapabilityFields != 0 {
		updated.Tools = cloneToolDefinitions(server.Tools)
		updated.Resources = cloneCatalogMCPResources(server.Resources)
		updated.Prompts = cloneCatalogMCPPrompts(server.Prompts)
	}
	if fieldMask&MCPToolServerMutationHealthFields != 0 {
		updated.HealthStatus = normalizeMCPToolHealthStatus(server.HealthStatus)
		updated.HealthCheckedAt = server.HealthCheckedAt
		updated.HealthLatencyMs = server.HealthLatencyMs
		updated.HealthError = boundedMCPToolHealthError(server.HealthError)
	}
	updated.UpdatedAt = server.UpdatedAt
	servers[server.ServerID] = updated
	return nil
}

func normalizeMCPToolServerMutationFieldMask(
	fieldMask MCPToolServerMutationFieldMask,
) (MCPToolServerMutationFieldMask, error) {
	if fieldMask == 0 {
		return MCPToolServerMutationConnectionFields, nil
	}
	allowed := MCPToolServerMutationConnectionFields |
		MCPToolServerMutationCapabilityFields |
		MCPToolServerMutationHealthFields
	if fieldMask&^allowed != 0 {
		return 0, errors.New("invalid mcp tool server mutation field mask")
	}
	return fieldMask, nil
}

func (c *InMemoryCatalog) EnsureServers(
	ctx context.Context,
	servers []*toolapi.MCPToolServer,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}
	for _, server := range servers {
		if server == nil || server.ServerID <= 0 || server.SpaceID <= 0 || strings.TrimSpace(server.Name) == "" {
			return errors.New("valid mcp tool server is required")
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.servers == nil {
		c.servers = make(map[int64]*toolapi.MCPToolServer)
	}
	liveNames := make(map[string]struct{}, len(c.servers)+len(servers))
	for _, stored := range c.servers {
		if stored != nil {
			liveNames[mcpCatalogLiveNameKey(stored.SpaceID, stored.Name)] = struct{}{}
		}
	}
	toInsert := make([]*toolapi.MCPToolServer, 0, len(servers))
	for _, server := range servers {
		nameKey := mcpCatalogLiveNameKey(server.SpaceID, server.Name)
		if _, exists := liveNames[nameKey]; exists {
			continue
		}
		if existing := c.servers[server.ServerID]; existing != nil {
			return ErrMCPConflict
		}
		liveNames[nameKey] = struct{}{}
		toInsert = append(toInsert, cloneServer(server))
	}
	for _, server := range toInsert {
		c.servers[server.ServerID] = server
	}

	return nil
}

func mcpCatalogLiveNameKey(spaceID int64, name string) string {
	return fmt.Sprintf("%d\x00%s", spaceID, strings.TrimSpace(name))
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

func (c *InMemoryCatalog) UpdateCapabilities(
	ctx context.Context,
	serverID int64,
	spaceID int64,
	expectedUpdatedAt int64,
	updatedAt int64,
	capabilities MCPToolCapabilitySnapshot,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	server := c.servers[serverID]
	if server == nil || server.SpaceID != spaceID || server.UpdatedAt != expectedUpdatedAt {
		return ErrMCPConflict
	}
	updated := cloneServer(server)
	updated.Tools = cloneToolDefinitions(capabilities.Tools)
	updated.Resources = cloneCatalogMCPResources(capabilities.Resources)
	updated.Prompts = cloneCatalogMCPPrompts(capabilities.Prompts)
	updated.UpdatedAt = updatedAt
	c.servers[serverID] = updated

	return nil
}

func (c *InMemoryCatalog) UpdateEnabled(
	ctx context.Context,
	serverID int64,
	spaceID int64,
	expectedUpdatedAt int64,
	updatedAt int64,
	enabled bool,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	server := c.servers[serverID]
	if server == nil || server.SpaceID != spaceID || server.UpdatedAt != expectedUpdatedAt {
		return ErrMCPConflict
	}
	updated := cloneServer(server)
	updated.Enabled = enabled
	updated.UpdatedAt = updatedAt
	c.servers[serverID] = updated

	return nil
}

func (c *InMemoryCatalog) UpdateHealth(
	ctx context.Context,
	serverID int64,
	expectedUpdatedAt int64,
	health MCPToolHealthSnapshot,
) error {
	if c == nil {
		return errors.New("mcp tool catalog is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	server := c.servers[serverID]
	if server == nil {
		return ErrNotFound
	}
	if expectedUpdatedAt > 0 && server.UpdatedAt != expectedUpdatedAt {
		return ErrMCPConflict
	}
	if health.CheckedAt <= server.HealthCheckedAt {
		return nil
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
		CreatorID:       server.CreatorID,
		SourceType:      normalizeCatalogMCPServerSourceType(server.SourceType),
		Name:            server.Name,
		Description:     server.Description,
		ServerType:      server.ServerType,
		Enabled:         server.Enabled,
		Config:          server.Config,
		Auth:            server.Auth,
		Tools:           cloneToolDefinitions(server.Tools),
		Resources:       cloneCatalogMCPResources(server.Resources),
		Prompts:         cloneCatalogMCPPrompts(server.Prompts),
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

func cloneCatalogMCPResources(resources []*toolapi.MCPResource) []*toolapi.MCPResource {
	result := make([]*toolapi.MCPResource, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		result = append(result, &toolapi.MCPResource{
			URI:         resource.URI,
			ResourceID:  resource.ResourceID,
			Name:        resource.Name,
			Description: resource.Description,
			MIMEType:    resource.MIMEType,
		})
	}

	return result
}

func cloneCatalogMCPPrompts(prompts []*toolapi.MCPPrompt) []*toolapi.MCPPrompt {
	result := make([]*toolapi.MCPPrompt, 0, len(prompts))
	for _, prompt := range prompts {
		if prompt == nil {
			continue
		}
		arguments := make([]*toolapi.MCPPromptArgument, 0, len(prompt.Arguments))
		for _, argument := range prompt.Arguments {
			if argument == nil {
				continue
			}
			arguments = append(arguments, &toolapi.MCPPromptArgument{
				Name:        argument.Name,
				Description: argument.Description,
				Required:    argument.Required,
			})
		}
		result = append(result, &toolapi.MCPPrompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Arguments:   arguments,
		})
	}

	return result
}

func normalizeCatalogMCPServerSourceType(sourceType toolapi.MCPServerSourceType) toolapi.MCPServerSourceType {
	normalized := toolapi.MCPServerSourceType(strings.ToLower(strings.TrimSpace(string(sourceType))))
	if normalized.Valid() {
		return normalized
	}

	return toolapi.MCPServerSourceTypeCustom
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
