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
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

var SVC = NewApplicationService(&Components{Catalog: NewInMemoryCatalog()})

const mcpAuthMaskValue = "********"
const (
	mcpToolHealthStatusUnknown   = "unknown"
	mcpToolHealthStatusHealthy   = "healthy"
	mcpToolHealthStatusUnhealthy = "unhealthy"
)

type MCPToolHealthSnapshot struct {
	Status    string
	CheckedAt int64
	LatencyMs int64
	Error     string
}

type MCPRuntimeHealthReport struct {
	ServerID  int64
	Success   bool
	ErrorCode string
	LatencyMs int64
	CheckedAt int64
}

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
	Delete(ctx context.Context, serverID int64) error
	UpdateHealth(ctx context.Context, serverID int64, health MCPToolHealthSnapshot) error
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
	var existing *toolapi.MCPToolServer
	if serverID > 0 {
		got, err := s.components.Catalog.Get(ctx, serverID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if got != nil {
			if got.SpaceID != req.SpaceID {
				return nil, InvalidArgumentErrorf(
					"mcp tool server %d space mismatch",
					serverID,
				)
			}
			existing = got
			createdAt = got.CreatedAt
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
	auth, err := resolveMCPAuthForUpsert(req.Auth, existing)
	if err != nil {
		return nil, err
	}
	health := mcpToolHealthFromServer(existing)
	server := &toolapi.MCPToolServer{
		ServerID:        serverID,
		SpaceID:         req.SpaceID,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		ServerType:      strings.TrimSpace(req.ServerType),
		Enabled:         req.Enabled,
		Config:          strings.TrimSpace(req.Config),
		Auth:            auth,
		Tools:           cloneToolDefinitions(req.Tools),
		HealthStatus:    health.Status,
		HealthCheckedAt: health.CheckedAt,
		HealthLatencyMs: health.LatencyMs,
		HealthError:     health.Error,
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
	if err := s.components.Catalog.Upsert(ctx, server); err != nil {
		return nil, err
	}

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServerForResponse(server)}, nil
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
			Servers: cloneServersForResponse(servers),
			Total:   int64(len(servers)),
		},
	}, nil
}

func (s *ApplicationService) ListSkillToolCandidates(ctx context.Context, spaceID int64) ([]*skillapi.SkillToolCandidate, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if spaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}

	servers, err := s.components.Catalog.List(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	candidates := make([]*skillapi.SkillToolCandidate, 0)
	for _, server := range servers {
		if server == nil || !server.Enabled {
			continue
		}
		for _, item := range server.Tools {
			candidate := mcpToolToSkillToolCandidate(server, item)
			if candidate != nil {
				candidates = append(candidates, candidate)
			}
		}
	}

	return candidates, nil
}

func (s *ApplicationService) ListRegistryEntries(ctx context.Context, req *toolapi.ListMCPToolRegistryEntriesRequest) (*toolapi.ListMCPToolRegistryEntriesResponse, error) {
	if req == nil || req.SpaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}

	tools, err := s.ListMCPToolRegistryEntries(ctx, req.SpaceID)
	if err != nil {
		return nil, err
	}

	return &toolapi.ListMCPToolRegistryEntriesResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.ListMCPToolRegistryEntriesData{
			Tools: tools,
			Total: int64(len(tools)),
		},
	}, nil
}

func (s *ApplicationService) ListMCPToolRegistryEntries(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolRegistryEntry, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if spaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}

	servers, err := s.components.Catalog.List(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	return mcpToolRegistryEntriesFromServers(servers), nil
}

func (s *ApplicationService) ResolveADKMCPRuntimeServer(ctx context.Context, serverID int64) (*toolapi.MCPToolServer, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if serverID <= 0 {
		return nil, InvalidArgumentErrorf("server_id is required")
	}

	server, err := s.components.Catalog.Get(ctx, serverID)
	if err != nil {
		return nil, err
	}

	return cloneServer(server), nil
}

func (s *ApplicationService) RecordRuntimeHealth(
	ctx context.Context,
	report MCPRuntimeHealthReport,
) error {
	if err := s.requireCatalog(); err != nil {
		return err
	}
	if report.ServerID <= 0 {
		return InvalidArgumentErrorf("server_id is required")
	}
	checkedAt := report.CheckedAt
	if checkedAt <= 0 {
		checkedAt = time.Now().UnixMilli()
	}
	latencyMs := report.LatencyMs
	if latencyMs < 0 {
		latencyMs = 0
	}
	health := MCPToolHealthSnapshot{
		Status:    mcpToolHealthStatusHealthy,
		CheckedAt: checkedAt,
		LatencyMs: latencyMs,
		Error:     "",
	}
	if !report.Success {
		health.Status = mcpToolHealthStatusUnhealthy
		health.Error = normalizeMCPRuntimeHealthErrorCode(report.ErrorCode)
	}

	return s.components.Catalog.UpdateHealth(ctx, report.ServerID, health)
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

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServerForResponse(server)}, nil
}

func (s *ApplicationService) DeleteServer(ctx context.Context, req *toolapi.GetMCPToolServerRequest) (*toolapi.MCPToolServerResponse, error) {
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
	if err := s.components.Catalog.Delete(ctx, req.ServerID); err != nil {
		return nil, err
	}

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServerForResponse(server)}, nil
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
	latencyMs := time.Since(startedAt).Milliseconds()
	if err := s.components.Catalog.UpdateHealth(ctx, req.ServerID, MCPToolHealthSnapshot{
		Status:    mcpToolHealthStatusHealthy,
		CheckedAt: time.Now().UnixMilli(),
		LatencyMs: latencyMs,
		Error:     "",
	}); err != nil {
		return nil, err
	}

	return &toolapi.TestMCPToolCallResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.TestMCPToolCallData{
			Status:    "success",
			Output:    string(output),
			LatencyMs: latencyMs,
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

func mcpToolHealthFromServer(server *toolapi.MCPToolServer) MCPToolHealthSnapshot {
	if server == nil || strings.TrimSpace(server.HealthStatus) == "" {
		return MCPToolHealthSnapshot{Status: mcpToolHealthStatusUnknown}
	}

	return MCPToolHealthSnapshot{
		Status:    strings.TrimSpace(server.HealthStatus),
		CheckedAt: server.HealthCheckedAt,
		LatencyMs: server.HealthLatencyMs,
		Error:     strings.TrimSpace(server.HealthError),
	}
}

func normalizeMCPRuntimeHealthErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 64 {
		return "runtime_failed"
	}
	for _, r := range code {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		if r == '_' || r == '-' || r == '.' {
			continue
		}

		return "runtime_failed"
	}

	return code
}

func resolveMCPAuthForUpsert(raw string, existing *toolapi.MCPToolServer) (string, error) {
	raw = strings.TrimSpace(raw)
	if existing != nil && raw == "" {
		return normalizeJSONText(existing.Auth), nil
	}

	auth := normalizeJSONText(raw)
	if existing == nil || !strings.Contains(auth, mcpAuthMaskValue) {
		return auth, nil
	}

	nextPayload, err := parseJSONMap("auth", auth)
	if err != nil {
		return "", err
	}
	existingPayload, err := parseJSONMap("stored auth", normalizeJSONText(existing.Auth))
	if err != nil {
		return "", err
	}

	merged := mergeMaskedMCPAuth(nextPayload, existingPayload)
	encoded, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func mergeMaskedMCPAuth(next map[string]any, existing map[string]any) map[string]any {
	merged := make(map[string]any, len(next))
	for key, value := range next {
		if isMCPAuthSensitiveKey(key) && isMCPAuthMask(value) {
			if previous, ok := existing[key]; ok {
				merged[key] = previous
			} else {
				merged[key] = ""
			}
			continue
		}

		nextMap, nextIsMap := value.(map[string]any)
		existingMap, existingIsMap := existing[key].(map[string]any)
		if nextIsMap && existingIsMap {
			merged[key] = mergeMaskedMCPAuth(nextMap, existingMap)
			continue
		}
		if nextSlice, ok := value.([]any); ok {
			var existingSlice []any
			if rawExistingSlice, ok := existing[key].([]any); ok {
				existingSlice = rawExistingSlice
			}
			merged[key] = mergeMaskedMCPAuthSlice(nextSlice, existingSlice)
			continue
		}

		merged[key] = value
	}

	return merged
}

func mergeMaskedMCPAuthSlice(next []any, existing []any) []any {
	merged := make([]any, 0, len(next))
	for index, value := range next {
		var previous any
		if index < len(existing) {
			previous = existing[index]
		}
		nextMap, nextIsMap := value.(map[string]any)
		existingMap, existingIsMap := previous.(map[string]any)
		if nextIsMap && existingIsMap {
			merged = append(merged, mergeMaskedMCPAuth(nextMap, existingMap))
			continue
		}
		merged = append(merged, value)
	}

	return merged
}

func cloneServersForResponse(servers []*toolapi.MCPToolServer) []*toolapi.MCPToolServer {
	result := make([]*toolapi.MCPToolServer, 0, len(servers))
	for _, server := range servers {
		result = append(result, cloneServerForResponse(server))
	}

	return result
}

func cloneServerForResponse(server *toolapi.MCPToolServer) *toolapi.MCPToolServer {
	cloned := cloneServer(server)
	if cloned == nil {
		return nil
	}
	cloned.Auth = maskMCPAuth(cloned.Auth)

	return cloned
}

func maskMCPAuth(raw string) string {
	payload, err := parseJSONMap("auth", normalizeJSONText(raw))
	if err != nil {
		return "{}"
	}
	encoded, err := json.Marshal(maskMCPAuthValue(payload))
	if err != nil {
		return "{}"
	}

	return string(encoded)
}

func maskMCPAuthValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		masked := make(map[string]any, len(typed))
		for key, item := range typed {
			if isMCPAuthSensitiveKey(key) {
				masked[key] = mcpAuthMaskValue
				continue
			}
			masked[key] = maskMCPAuthValue(item)
		}
		return masked
	case []any:
		masked := make([]any, 0, len(typed))
		for _, item := range typed {
			masked = append(masked, maskMCPAuthValue(item))
		}
		return masked
	default:
		return value
	}
}

func isMCPAuthSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "api_key", "apikey", "access_key", "secret_key", "token", "access_token",
		"refresh_token", "id_token", "client_secret", "secret", "password",
		"private_key", "credential", "credentials":
		return true
	default:
		return false
	}
}

func isMCPAuthMask(value any) bool {
	text, ok := value.(string)

	return ok && text == mcpAuthMaskValue
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

func mcpToolToSkillToolCandidate(server *toolapi.MCPToolServer, item *toolapi.MCPToolDefinition) *skillapi.SkillToolCandidate {
	if server == nil || item == nil {
		return nil
	}
	toolName := strings.TrimSpace(item.Name)
	description := strings.TrimSpace(item.Description)
	if toolName == "" || description == "" {
		return nil
	}

	return &skillapi.SkillToolCandidate{
		Name:        mcpToolGrantName(server.ServerID, toolName),
		DisplayName: fmt.Sprintf("%s / %s", strings.TrimSpace(server.Name), toolName),
		Description: description,
		Category:    "mcp",
		Visibility:  "static",
		Source:      "mcp",
		SourceID:    fmt.Sprintf("%d", server.ServerID),
		SourceName:  strings.TrimSpace(server.Name),
	}
}

func mcpToolRegistryEntriesFromServers(servers []*toolapi.MCPToolServer) []*toolapi.MCPToolRegistryEntry {
	entries := make([]*toolapi.MCPToolRegistryEntry, 0)
	for _, server := range servers {
		if server == nil || !server.Enabled {
			continue
		}
		for _, item := range server.Tools {
			entry := mcpToolToRegistryEntry(server, item)
			if entry != nil {
				entries = append(entries, entry)
			}
		}
	}

	return entries
}

func mcpToolToRegistryEntry(server *toolapi.MCPToolServer, item *toolapi.MCPToolDefinition) *toolapi.MCPToolRegistryEntry {
	if server == nil || item == nil {
		return nil
	}
	toolName := strings.TrimSpace(item.Name)
	description := strings.TrimSpace(item.Description)
	if toolName == "" || description == "" {
		return nil
	}

	return &toolapi.MCPToolRegistryEntry{
		Name:            mcpToolGrantName(server.ServerID, toolName),
		Source:          "mcp",
		Category:        "mcp",
		Visibility:      "static",
		ServerID:        server.ServerID,
		ServerName:      strings.TrimSpace(server.Name),
		ToolName:        toolName,
		Description:     description,
		Enabled:         server.Enabled,
		HealthStatus:    normalizeMCPToolHealthStatus(server.HealthStatus),
		HealthCheckedAt: server.HealthCheckedAt,
		HealthLatencyMs: server.HealthLatencyMs,
		HealthError:     boundedMCPToolHealthError(server.HealthError),
	}
}

func mcpToolGrantName(serverID int64, toolName string) string {
	prefix := fmt.Sprintf("mcp_%d_", serverID)
	slug := sanitizeMCPToolNameSegment(toolName)
	if slug == "" {
		slug = "tool"
	}

	name := prefix + slug
	if len(name) <= 64 {
		return name
	}

	hash := sha1.Sum([]byte(toolName))
	suffix := "_" + hex.EncodeToString(hash[:])[:8]
	maxSlugLength := 64 - len(prefix) - len(suffix)
	if maxSlugLength <= 0 {
		return prefix[:min(len(prefix), 55)] + suffix
	}
	if len(slug) > maxSlugLength {
		slug = slug[:maxSlugLength]
	}

	return prefix + slug + suffix
}

func sanitizeMCPToolNameSegment(value string) string {
	var builder strings.Builder
	lastUnderscore := false
	for _, char := range strings.TrimSpace(value) {
		valid := false
		switch {
		case char >= 'a' && char <= 'z':
			valid = true
		case char >= 'A' && char <= 'Z':
			valid = true
		case char >= '0' && char <= '9':
			valid = true
		case char == '_':
			valid = true
		}
		if valid {
			builder.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}

	return strings.Trim(builder.String(), "_")
}
