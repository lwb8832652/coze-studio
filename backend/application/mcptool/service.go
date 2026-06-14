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

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

var SVC = NewApplicationService(&Components{Catalog: NewInMemoryCatalog()})

type Components struct {
	Catalog Catalog
	IDGen   idgen.IDGenerator
}

type ApplicationService struct {
	components *Components
}

type Catalog interface {
	Upsert(ctx context.Context, server *toolapi.MCPToolServer) error
	Get(ctx context.Context, serverID int64) (*toolapi.MCPToolServer, error)
	List(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolServer, error)
}

func NewApplicationService(c *Components) *ApplicationService {
	if c == nil {
		c = &Components{}
	}
	if c.Catalog == nil {
		c.Catalog = NewInMemoryCatalog()
	}

	return &ApplicationService{components: c}
}

func InitService(c *Components) *ApplicationService {
	SVC = NewApplicationService(c)

	return SVC
}

func (s *ApplicationService) UpsertServer(ctx context.Context, req *toolapi.UpsertMCPToolServerRequest) (*toolapi.MCPToolServerResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if err := validateUpsertRequest(req); err != nil {
		return nil, err
	}

	serverID := req.ServerID
	var createdAt int64
	if serverID > 0 {
		existing, err := s.components.Catalog.Get(ctx, serverID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if existing != nil {
			createdAt = existing.CreatedAt
		}
	} else {
		if err := s.requireIDGen(); err != nil {
			return nil, err
		}
		id, err := s.components.IDGen.GenID(ctx)
		if err != nil {
			return nil, err
		}
		serverID = id
	}

	now := time.Now().UnixMilli()
	if createdAt == 0 {
		createdAt = now
	}
	server := &toolapi.MCPToolServer{
		ServerID:    serverID,
		SpaceID:     req.SpaceID,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		ServerType:  strings.TrimSpace(req.ServerType),
		Enabled:     req.Enabled,
		Config:      strings.TrimSpace(req.Config),
		Auth:        normalizeJSONText(req.Auth),
		Tools:       cloneToolDefinitions(req.Tools),
		CreatedAt:   createdAt,
		UpdatedAt:   now,
	}
	if err := s.components.Catalog.Upsert(ctx, server); err != nil {
		return nil, err
	}

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServer(server)}, nil
}

func (s *ApplicationService) ListServers(ctx context.Context, req *toolapi.ListMCPToolServersRequest) (*toolapi.ListMCPToolServersResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.SpaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}

	servers, err := s.components.Catalog.List(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}

	return &toolapi.ListMCPToolServersResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.ListMCPToolServersData{
			Servers: cloneServers(servers),
			Total:   int64(len(servers)),
		},
	}, nil
}

func (s *ApplicationService) GetServer(ctx context.Context, req *toolapi.GetMCPToolServerRequest) (*toolapi.MCPToolServerResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.ServerID <= 0 {
		return nil, InvalidArgumentErrorf("server_id is required")
	}

	server, err := s.components.Catalog.Get(ctx, req.ServerID)
	if err != nil {
		return nil, err
	}

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServer(server)}, nil
}

func (s *ApplicationService) TestCall(ctx context.Context, req *toolapi.TestMCPToolCallRequest) (*toolapi.TestMCPToolCallResponse, error) {
	startedAt := time.Now()
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.ServerID <= 0 {
		return nil, InvalidArgumentErrorf("server_id is required")
	}
	toolName := strings.TrimSpace(req.ToolName)
	if toolName == "" {
		return nil, InvalidArgumentErrorf("tool_name is required")
	}

	arguments, err := parseJSONMap("arguments", req.Arguments)
	if err != nil {
		return nil, err
	}
	server, err := s.components.Catalog.Get(ctx, req.ServerID)
	if err != nil {
		return nil, err
	}
	if !hasTool(server, toolName) {
		return nil, InvalidArgumentErrorf("tool %s is not configured on server %d", toolName, req.ServerID)
	}

	output, err := json.Marshal(map[string]any{
		"mode":        "mcp_test_call_stub",
		"server_id":   server.ServerID,
		"server_name": server.Name,
		"tool_name":   toolName,
		"arguments":   arguments,
	})
	if err != nil {
		return nil, err
	}

	return &toolapi.TestMCPToolCallResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.TestMCPToolCallData{
			Status:    "success",
			Output:    string(output),
			LatencyMs: time.Since(startedAt).Milliseconds(),
		},
	}, nil
}

func (s *ApplicationService) requireCatalog() error {
	if s == nil || s.components == nil {
		return fmt.Errorf("mcp tool service components are required")
	}
	if s.components.Catalog == nil {
		return fmt.Errorf("mcp tool catalog is required")
	}

	return nil
}

func (s *ApplicationService) requireIDGen() error {
	if s == nil || s.components == nil || s.components.IDGen == nil {
		return fmt.Errorf("id generator is required")
	}

	return nil
}

func validateUpsertRequest(req *toolapi.UpsertMCPToolServerRequest) error {
	if req == nil {
		return InvalidArgumentErrorf("mcp tool server is required")
	}
	if req.SpaceID <= 0 {
		return InvalidArgumentErrorf("space_id is required")
	}
	if strings.TrimSpace(req.Name) == "" {
		return InvalidArgumentErrorf("name is required")
	}
	if strings.TrimSpace(req.ServerType) == "" {
		return InvalidArgumentErrorf("server_type is required")
	}
	if _, err := parseJSONMap("config", req.Config); err != nil {
		return err
	}
	if _, err := parseJSONMap("auth", normalizeJSONText(req.Auth)); err != nil {
		return err
	}
	if len(req.Tools) == 0 {
		return InvalidArgumentErrorf("tools are required")
	}
	for _, tool := range req.Tools {
		if tool == nil {
			return InvalidArgumentErrorf("tool is required")
		}
		if strings.TrimSpace(tool.Name) == "" {
			return InvalidArgumentErrorf("tool name is required")
		}
		if _, err := parseJSONMap("input_schema", tool.InputSchema); err != nil {
			return err
		}
	}

	return nil
}

func parseJSONMap(name, raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, InvalidArgumentErrorf("%s json is required", name)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, InvalidArgumentErrorf("invalid %s json: %v", name, err)
	}
	if payload == nil {
		return nil, InvalidArgumentErrorf("invalid %s json: expected object", name)
	}

	return payload, nil
}

func normalizeJSONText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return `{}`
	}

	return raw
}

func hasTool(server *toolapi.MCPToolServer, toolName string) bool {
	if server == nil {
		return false
	}
	for _, item := range server.Tools {
		if item != nil && strings.TrimSpace(item.Name) == toolName {
			return true
		}
	}

	return false
}
