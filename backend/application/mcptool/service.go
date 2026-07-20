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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

var defaultMCPServiceEnabled = false

var SVC = NewApplicationService(&Components{
	Enabled: &defaultMCPServiceEnabled,
	Catalog: NewInMemoryCatalog(),
})

var ErrMCPOfficialServerImmutable = errors.New("official MCP server only allows enabled changes")

const (
	mcpAuthConfiguredSentinel = `{"configured":true}`
	mcpSafeTestCallOutput     = `{"result":"completed"}`
)
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
	ServerID          int64
	ExpectedUpdatedAt int64
	Success           bool
	ErrorCode         string
	LatencyMs         int64
	CheckedAt         int64
}

type Components struct {
	// Enabled is always set by the application composition root. A nil value is
	// retained only for backwards-compatible in-process construction.
	Enabled                     *bool
	Catalog                     Catalog
	IDGen                       idgen.IDGenerator
	UserSpaceRoleReader         UserSpaceRoleReader
	RuntimeExecutor             RuntimeExecutor
	CapabilityDiscoverer        CapabilityDiscoverer
	AuditRepository             ManagementAuditRepository
	DefaultDeerFlowMCPConfigRaw []byte
}

type ApplicationService struct {
	components             *Components
	authorizer             *spaceAuthorizer
	enabled                bool
	managementRuntimeMu    sync.RWMutex
	runtimeExecutor        RuntimeExecutor
	capabilityDiscoverer   CapabilityDiscoverer
	managementRuntimeBound bool
}

type Catalog interface {
	Upsert(ctx context.Context, server *toolapi.MCPToolServer) error
	Create(ctx context.Context, server *toolapi.MCPToolServer) error
	UpdateServer(ctx context.Context, server *toolapi.MCPToolServer, expectedUpdatedAt int64) error
	DeleteServer(ctx context.Context, serverID, spaceID, expectedUpdatedAt int64) error
	ApplyServers(ctx context.Context, mutations []MCPToolServerMutation) error
	EnsureServers(ctx context.Context, servers []*toolapi.MCPToolServer) error
	Get(ctx context.Context, serverID int64) (*toolapi.MCPToolServer, error)
	List(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolServer, error)
	Delete(ctx context.Context, serverID int64) error
	UpdateCapabilities(ctx context.Context, serverID, spaceID, expectedUpdatedAt, updatedAt int64, capabilities MCPToolCapabilitySnapshot) error
	UpdateEnabled(ctx context.Context, serverID, spaceID, expectedUpdatedAt, updatedAt int64, enabled bool) error
	UpdateHealth(ctx context.Context, serverID, expectedUpdatedAt int64, health MCPToolHealthSnapshot) error
}

type managementCatalog interface {
	ListForManagement(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolServer, error)
}

func listMCPToolServersForManagement(
	ctx context.Context,
	catalog Catalog,
	spaceID int64,
) ([]*toolapi.MCPToolServer, error) {
	if management, ok := catalog.(managementCatalog); ok {
		return management.ListForManagement(ctx, spaceID)
	}

	return catalog.List(ctx, spaceID)
}

type trustedMCPServerFields struct {
	CreatorID  int64
	SourceType toolapi.MCPServerSourceType
	Tools      []*toolapi.MCPToolDefinition
	Resources  []*toolapi.MCPResource
	Prompts    []*toolapi.MCPPrompt
}

func NewApplicationService(c *Components) *ApplicationService {
	if c == nil {
		c = &Components{}
	}
	if c.Catalog == nil {
		c.Catalog = NewInMemoryCatalog()
	}
	if c.AuditRepository == nil {
		c.AuditRepository = NewInMemoryManagementAuditRepository()
	}
	enabled := true
	if c.Enabled != nil {
		enabled = *c.Enabled
	}

	return &ApplicationService{
		components:             c,
		authorizer:             newSpaceAuthorizer(c.UserSpaceRoleReader, nil),
		enabled:                enabled,
		runtimeExecutor:        c.RuntimeExecutor,
		capabilityDiscoverer:   c.CapabilityDiscoverer,
		managementRuntimeBound: c.RuntimeExecutor != nil || c.CapabilityDiscoverer != nil,
	}
}

func InitService(c *Components) *ApplicationService {
	SVC = NewApplicationService(c)

	return SVC
}

// BindManagementRuntime is a startup-only wiring point for the shared MCP
// session adapter. Dependencies configured through Components cannot be
// replaced, and a successful bind can happen only once.
func (s *ApplicationService) BindManagementRuntime(
	executor RuntimeExecutor,
	discoverer CapabilityDiscoverer,
) error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if executor == nil || discoverer == nil {
		return ErrManagementRuntimeDependencies
	}
	if s == nil {
		return ErrManagementRuntimeDependencies
	}

	s.managementRuntimeMu.Lock()
	defer s.managementRuntimeMu.Unlock()
	if s.managementRuntimeBound || s.runtimeExecutor != nil || s.capabilityDiscoverer != nil {
		return ErrManagementRuntimeAlreadyBound
	}
	s.runtimeExecutor = executor
	s.capabilityDiscoverer = discoverer
	s.managementRuntimeBound = true

	return nil
}

func (s *ApplicationService) UpsertServer(ctx context.Context, req *toolapi.UpsertMCPToolServerRequest) (*toolapi.MCPToolServerResponse, error) {
	return s.upsertServer(ctx, req, nil)
}

func (s *ApplicationService) upsertServer(
	ctx context.Context,
	req *toolapi.UpsertMCPToolServerRequest,
	trusted *trustedMCPServerFields,
) (*toolapi.MCPToolServerResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if err := validateUpsertRequest(req); err != nil {
		return nil, err
	}
	if trusted == nil {
		if err := s.authorizeSpace(ctx, req.SpaceID, MCPAccessManage); err != nil {
			return nil, err
		}
	}

	serverID := req.ServerID
	var existing *toolapi.MCPToolServer
	if serverID > 0 {
		got, err := s.components.Catalog.Get(ctx, serverID)
		if err != nil {
			if trusted == nil && errors.Is(err, ErrNotFound) {
				return nil, ErrMCPForbidden
			}
			return nil, err
		}
		if got.SpaceID != req.SpaceID {
			if trusted == nil {
				return nil, ErrMCPForbidden
			}
			return nil, InvalidArgumentErrorf(
				"mcp tool server %d space mismatch",
				serverID,
			)
		}
		existing, err = s.canonicalizeServerCredentials(ctx, got)
		if err != nil {
			return nil, err
		}
	}

	if existing == nil {
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
	preserveAuth := existing != nil && isMCPAuthPreserveInput(req.Auth)
	auth, err := resolveMCPAuthForUpsert(req.Auth, existing)
	if err != nil {
		return nil, err
	}
	config, auth, err := canonicalizeMCPUpsertCredentials(req.ServerType, req.Config, auth, existing, preserveAuth)
	if err != nil {
		return nil, err
	}
	if isOfficialMCPServer(existing) && trusted == nil {
		if !officialMCPServerUpdateOnlyChangesEnabled(existing, req, config, auth) {
			return nil, ErrMCPOfficialServerImmutable
		}
		updated := cloneServer(existing)
		updated.Enabled = req.Enabled
		updated.UpdatedAt = nextMCPServerUpdatedAt(existing.UpdatedAt)
		if err := s.persistMCPServerCandidate(ctx, updated, existing); err != nil {
			return nil, err
		}

		return &toolapi.MCPToolServerResponse{
			Code: 0,
			Msg:  "success",
			Data: cloneServerForResponse(updated),
		}, nil
	}
	if isOfficialMCPServer(existing) && !isOfficialMCPSourceType(trusted.SourceType) {
		return nil, ErrMCPOfficialServerImmutable
	}

	creatorID := int64(0)
	sourceType := toolapi.MCPServerSourceTypeCustom
	createdAt := now
	tools := []*toolapi.MCPToolDefinition(nil)
	resources := []*toolapi.MCPResource(nil)
	prompts := []*toolapi.MCPPrompt(nil)
	if existing != nil {
		creatorID = existing.CreatorID
		sourceType = existing.SourceType
		createdAt = existing.CreatedAt
		tools = cloneToolDefinitions(existing.Tools)
		resources = cloneCatalogMCPResources(existing.Resources)
		prompts = cloneCatalogMCPPrompts(existing.Prompts)
		if trusted != nil {
			tools = cloneToolDefinitions(trusted.Tools)
			resources = cloneCatalogMCPResources(trusted.Resources)
			prompts = cloneCatalogMCPPrompts(trusted.Prompts)
		}
	} else if trusted != nil {
		creatorID = trusted.CreatorID
		sourceType = normalizeCatalogMCPServerSourceType(trusted.SourceType)
		tools = cloneToolDefinitions(trusted.Tools)
		resources = cloneCatalogMCPResources(trusted.Resources)
		prompts = cloneCatalogMCPPrompts(trusted.Prompts)
	} else {
		var ok bool
		creatorID, ok = authenticatedUserID(ctx)
		if !ok || creatorID <= 0 {
			return nil, ErrMCPUnauthenticated
		}
	}
	health := mcpToolHealthFromServer(existing)
	updatedAt := now
	if existing != nil {
		updatedAt = nextMCPServerUpdatedAt(existing.UpdatedAt)
	}
	server := &toolapi.MCPToolServer{
		ServerID:        serverID,
		SpaceID:         req.SpaceID,
		CreatorID:       creatorID,
		SourceType:      sourceType,
		Name:            strings.TrimSpace(req.Name),
		Description:     strings.TrimSpace(req.Description),
		ServerType:      strings.TrimSpace(req.ServerType),
		Enabled:         req.Enabled,
		Config:          config,
		Auth:            auth,
		Tools:           tools,
		Resources:       resources,
		Prompts:         prompts,
		HealthStatus:    health.Status,
		HealthCheckedAt: health.CheckedAt,
		HealthLatencyMs: health.LatencyMs,
		HealthError:     health.Error,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}
	if err := s.persistMCPServerCandidate(ctx, server, existing); err != nil {
		return nil, err
	}

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServerForResponse(server)}, nil
}

func (s *ApplicationService) persistMCPServerCandidate(
	ctx context.Context,
	candidate *toolapi.MCPToolServer,
	existing *toolapi.MCPToolServer,
) error {
	if candidate == nil {
		return InvalidArgumentErrorf("mcp tool server is required")
	}
	connectionChanged := existing == nil ||
		existing.ServerType != candidate.ServerType ||
		existing.Config != candidate.Config ||
		existing.Auth != candidate.Auth
	requiresDiscovery := candidate.Enabled &&
		(existing == nil || !existing.Enabled || connectionChanged)
	fieldMask := MCPToolServerMutationConnectionFields
	if requiresDiscovery {
		startedAt := time.Now()
		_, discoverer := s.managementRuntimeDependencies()
		capabilities, err := discoverCapabilities(ctx, discoverer, MCPServerConnection{
			ServerType: candidate.ServerType, Config: candidate.Config, Auth: candidate.Auth,
		})
		if err != nil {
			return err
		}
		candidate.Tools = cloneToolDefinitions(capabilities.Tools)
		candidate.Resources = cloneCatalogMCPResources(capabilities.Resources)
		candidate.Prompts = cloneCatalogMCPPrompts(capabilities.Prompts)
		candidate.HealthStatus = mcpToolHealthStatusHealthy
		candidate.HealthCheckedAt = time.Now().UnixMilli()
		candidate.HealthLatencyMs = max(time.Since(startedAt).Milliseconds(), 0)
		candidate.HealthError = ""
		fieldMask |= MCPToolServerMutationCapabilityFields | MCPToolServerMutationHealthFields
	} else if connectionChanged {
		candidate.Tools = nil
		candidate.Resources = nil
		candidate.Prompts = nil
		candidate.HealthStatus = mcpToolHealthStatusUnknown
		candidate.HealthCheckedAt = 0
		candidate.HealthLatencyMs = 0
		candidate.HealthError = ""
		fieldMask |= MCPToolServerMutationCapabilityFields | MCPToolServerMutationHealthFields
	}
	expectedUpdatedAt := int64(0)
	if existing != nil {
		expectedUpdatedAt = existing.UpdatedAt
	}
	return s.components.Catalog.ApplyServers(ctx, []MCPToolServerMutation{{
		Server: candidate, ExpectedUpdatedAt: expectedUpdatedAt, FieldMask: fieldMask,
	}})
}

func (s *ApplicationService) ListServers(ctx context.Context, req *toolapi.ListMCPToolServersRequest) (*toolapi.ListMCPToolServersResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.SpaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}
	if err := s.authorizeSpace(ctx, req.SpaceID, MCPAccessRead); err != nil {
		return nil, err
	}
	if err := s.ensureDefaultDeerFlowMCPServers(ctx, req.SpaceID); err != nil {
		return nil, err
	}

	servers, err := listMCPToolServersForManagement(ctx, s.components.Catalog, req.SpaceID)
	if err != nil {
		return nil, err
	}
	canManage := s.authorizeSpace(ctx, req.SpaceID, MCPAccessManage) == nil
	for index, server := range servers {
		canonical, _ := s.canonicalizeServerCredentials(ctx, server)
		servers[index] = canonical
	}

	return &toolapi.ListMCPToolServersResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.ListMCPToolServersData{
			Servers:   cloneServersForResponse(servers),
			Total:     int64(len(servers)),
			CanManage: canManage,
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
	if err := s.authorizeSpace(ctx, spaceID, MCPAccessRead); err != nil {
		return nil, err
	}
	if err := s.ensureDefaultDeerFlowMCPServers(ctx, spaceID); err != nil {
		return nil, err
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
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.SpaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}

	if err := s.authorizeSpace(ctx, req.SpaceID, MCPAccessRead); err != nil {
		return nil, err
	}

	tools, err := s.listMCPToolRegistryEntriesForUser(ctx, req.SpaceID)
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
	if err := s.authorizeSpace(ctx, spaceID, MCPAccessRead); err != nil {
		return nil, err
	}

	return s.listMCPToolRegistryEntriesForUser(ctx, spaceID)
}

func (s *ApplicationService) listMCPToolRegistryEntriesForUser(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolRegistryEntry, error) {
	if err := s.ensureDefaultDeerFlowMCPServers(ctx, spaceID); err != nil {
		return nil, err
	}

	return s.listMCPToolRegistryEntries(ctx, spaceID)
}

// ListMCPToolRegistryEntriesForRuntime accepts only the SpaceID from an
// already validated durable run. It intentionally performs no user session
// authorization; default initialization is an idempotent catalog operation.
func (s *ApplicationService) ListMCPToolRegistryEntriesForRuntime(
	ctx context.Context,
	spaceID int64,
) ([]*toolapi.MCPToolRegistryEntry, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if spaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}
	if err := s.ensureDefaultDeerFlowMCPServersForRuntime(ctx, spaceID); err != nil {
		return nil, err
	}

	return s.listMCPToolRegistryEntries(ctx, spaceID)
}

func (s *ApplicationService) listMCPToolRegistryEntries(ctx context.Context, spaceID int64) ([]*toolapi.MCPToolRegistryEntry, error) {

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
		if errors.Is(err, ErrNotFound) {
			return nil, ErrMCPForbidden
		}
		return nil, err
	}
	server, err = s.canonicalizeServerCredentials(ctx, server)
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

	return s.components.Catalog.UpdateHealth(
		ctx,
		report.ServerID,
		report.ExpectedUpdatedAt,
		health,
	)
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
		if errors.Is(err, ErrNotFound) {
			return nil, ErrMCPForbidden
		}
		return nil, err
	}
	if err := s.authorizeSpace(ctx, server.SpaceID, MCPAccessRead); err != nil {
		return nil, err
	}
	server, _ = s.canonicalizeServerCredentials(ctx, server)

	return &toolapi.MCPToolServerResponse{Code: 0, Msg: "success", Data: cloneServerForResponse(server)}, nil
}

func (s *ApplicationService) SafeExport(
	ctx context.Context,
	serverID int64,
) (*toolapi.ExportMCPToolServerData, error) {
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
	if err := s.authorizeSpace(ctx, server.SpaceID, MCPAccessRead); err != nil {
		return nil, err
	}
	server, _ = s.canonicalizeServerCredentials(ctx, server)

	return &toolapi.ExportMCPToolServerData{
		Name:        server.Name,
		Description: server.Description,
		ServerType:  server.ServerType,
		Config:      safeMCPConfigForExport(server.ServerType, server.Config),
		Tools:       cloneToolDefinitions(server.Tools),
		Resources:   projectMCPResourcesForResponse(server.Resources),
		Prompts:     cloneCatalogMCPPrompts(server.Prompts),
	}, nil
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
		if errors.Is(err, ErrNotFound) {
			return nil, ErrMCPForbidden
		}
		return nil, err
	}
	if err := s.authorizeSpace(ctx, server.SpaceID, MCPAccessManage); err != nil {
		return nil, err
	}
	if isOfficialMCPServer(server) {
		return nil, ErrMCPOfficialServerImmutable
	}
	if err := s.components.Catalog.DeleteServer(ctx, req.ServerID, server.SpaceID, server.UpdatedAt); err != nil {
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

	server, err := s.components.Catalog.Get(ctx, req.ServerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrMCPForbidden
		}
		return nil, err
	}
	server, err = s.canonicalizeServerCredentials(ctx, server)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeSpace(ctx, server.SpaceID, MCPAccessManage); err != nil {
		return nil, err
	}
	if !server.Enabled {
		return nil, InvalidArgumentErrorf("mcp tool server %d is disabled", server.ServerID)
	}
	if !hasTool(server, toolName) {
		return nil, InvalidArgumentErrorf("tool %s is not configured on server %d", toolName, req.ServerID)
	}
	arguments := strings.TrimSpace(req.Arguments)
	if _, err := parseJSONMap("arguments", arguments); err != nil {
		return nil, err
	}
	actorID, ok := authenticatedUserID(ctx)
	if !ok || actorID <= 0 {
		return nil, ErrMCPUnauthenticated
	}
	auditEvent := &ManagementAuditEvent{
		SpaceID:   server.SpaceID,
		ServerID:  server.ServerID,
		ActorID:   actorID,
		ToolName:  toolName,
		Status:    ManagementAuditStatusPending,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.components.AuditRepository.CreatePending(ctx, auditEvent); err != nil {
		return nil, ErrManagementAuditUnavailable
	}
	executor, _ := s.managementRuntimeDependencies()
	result, runtimeErr := executeRuntimeToolCall(ctx, executor, RuntimeToolCall{
		SpaceID:   server.SpaceID,
		ServerID:  server.ServerID,
		ToolName:  toolName,
		Arguments: arguments,
	})
	checkedAt := time.Now().UnixMilli()
	if runtimeErr != nil {
		latencyMs := time.Since(startedAt).Milliseconds()
		errorCode := ManagementAuditErrorRuntimeFailed
		if errors.Is(runtimeErr, ErrRuntimeUnavailable) {
			errorCode = ManagementAuditErrorRuntimeUnavailable
		} else if errors.Is(runtimeErr, ErrRuntimeInvalidResult) || errors.Is(runtimeErr, ErrRuntimeOutputTooLarge) {
			errorCode = ManagementAuditErrorInvalidResult
		}
		_ = s.completeManagementAudit(ctx, auditEvent, ManagementAuditCompletion{
			Status: ManagementAuditStatusFailed, LatencyMs: latencyMs,
			ErrorCode: errorCode, CompletedAt: checkedAt,
		})
		if errors.Is(runtimeErr, ErrRuntimeUnavailable) {
			return nil, ErrRuntimeUnavailable
		}
		if errors.Is(runtimeErr, ErrRuntimeInvalidResult) || errors.Is(runtimeErr, ErrRuntimeOutputTooLarge) {
			return nil, ErrRuntimeCallFailed
		}
		_ = s.components.Catalog.UpdateHealth(ctx, req.ServerID, server.UpdatedAt, MCPToolHealthSnapshot{
			Status:    mcpToolHealthStatusUnhealthy,
			CheckedAt: checkedAt,
			LatencyMs: time.Since(startedAt).Milliseconds(),
			Error:     "runtime_failed",
		})
		return nil, ErrRuntimeCallFailed
	}
	latencyMs := result.LatencyMs
	if latencyMs < 0 {
		latencyMs = 0
	}
	if !strings.EqualFold(strings.TrimSpace(result.Status), "success") {
		_ = s.completeManagementAudit(ctx, auditEvent, ManagementAuditCompletion{
			Status: ManagementAuditStatusFailed, LatencyMs: latencyMs,
			ErrorCode: ManagementAuditErrorRuntimeFailed, CompletedAt: checkedAt,
		})
		_ = s.components.Catalog.UpdateHealth(ctx, req.ServerID, server.UpdatedAt, MCPToolHealthSnapshot{
			Status:    mcpToolHealthStatusUnhealthy,
			CheckedAt: checkedAt,
			LatencyMs: latencyMs,
			Error:     "runtime_failed",
		})
		return nil, ErrRuntimeCallFailed
	}
	_ = s.completeManagementAudit(ctx, auditEvent, ManagementAuditCompletion{
		Status: ManagementAuditStatusSuccess, LatencyMs: latencyMs, CompletedAt: checkedAt,
	})
	_ = s.components.Catalog.UpdateHealth(ctx, req.ServerID, server.UpdatedAt, MCPToolHealthSnapshot{
		Status:    mcpToolHealthStatusHealthy,
		CheckedAt: checkedAt,
		LatencyMs: latencyMs,
		Error:     "",
	})

	return &toolapi.TestMCPToolCallResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.TestMCPToolCallData{
			Status:    result.Status,
			Output:    mcpSafeTestCallOutput,
			LatencyMs: latencyMs,
		},
	}, nil
}

func (s *ApplicationService) completeManagementAudit(
	ctx context.Context,
	event *ManagementAuditEvent,
	completion ManagementAuditCompletion,
) error {
	if event == nil || event.EventID <= 0 || s.components.AuditRepository == nil {
		return ErrManagementAuditUnavailable
	}
	if err := s.components.AuditRepository.Complete(ctx, event.EventID, completion); err != nil {
		return ErrManagementAuditUnavailable
	}
	return nil
}

func (s *ApplicationService) ListAuditEvents(
	ctx context.Context,
	serverID int64,
	rawCursor string,
	requestedLimit int,
) (*toolapi.ListMCPRuntimeAuditEventsResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if serverID <= 0 {
		return nil, InvalidArgumentErrorf("server_id is required")
	}
	server, err := s.components.Catalog.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrMCPForbidden
		}
		return nil, err
	}
	if err := s.authorizeSpace(ctx, server.SpaceID, MCPAccessRead); err != nil {
		return nil, err
	}
	cursor, err := decodeManagementAuditCursor(rawCursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeManagementAuditPageSize(requestedLimit)
	events, err := s.components.AuditRepository.List(ctx, server.SpaceID, serverID, cursor, limit+1)
	if err != nil {
		return nil, ErrManagementAuditUnavailable
	}
	nextCursor := ""
	if len(events) > limit {
		events = events[:limit]
		last := events[len(events)-1]
		nextCursor = encodeManagementAuditCursor(ManagementAuditCursor{
			CreatedAt: last.CreatedAt,
			EventID:   last.EventID,
		})
	}
	responseEvents := make([]*toolapi.MCPRuntimeAuditEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		responseEvents = append(responseEvents, &toolapi.MCPRuntimeAuditEvent{
			EventID:      fmt.Sprintf("%d", event.EventID),
			ActorID:      event.ActorID,
			ToolName:     event.ToolName,
			Status:       event.Status,
			LatencyMs:    event.LatencyMs,
			ErrorCode:    event.ErrorCode,
			ErrorSummary: event.ErrorSummary,
			CreatedAt:    event.CreatedAt,
			CompletedAt:  event.CompletedAt,
		})
	}
	return &toolapi.ListMCPRuntimeAuditEventsResponse{
		Code: 0,
		Msg:  "success",
		Data: &toolapi.ListMCPRuntimeAuditEventsData{
			Events: responseEvents, NextCursor: nextCursor,
		},
	}, nil
}

func (s *ApplicationService) Discover(
	ctx context.Context,
	serverID int64,
) (*toolapi.DiscoverMCPToolServerData, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if serverID <= 0 {
		return nil, InvalidArgumentErrorf("server_id is required")
	}
	server, err := s.components.Catalog.Get(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrMCPForbidden
		}
		return nil, err
	}
	server, err = s.canonicalizeServerCredentials(ctx, server)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeSpace(ctx, server.SpaceID, MCPAccessManage); err != nil {
		return nil, err
	}
	_, discoverer := s.managementRuntimeDependencies()
	capabilities, err := discoverCapabilities(ctx, discoverer, MCPServerConnection{
		ServerType: server.ServerType,
		Config:     server.Config,
		Auth:       server.Auth,
	})
	if err != nil {
		return nil, err
	}

	updated := cloneServer(server)
	updated.Tools = cloneToolDefinitions(capabilities.Tools)
	updated.Resources = cloneCatalogMCPResources(capabilities.Resources)
	updated.Prompts = cloneCatalogMCPPrompts(capabilities.Prompts)
	updated.HealthStatus = mcpToolHealthStatusHealthy
	updated.HealthCheckedAt = time.Now().UnixMilli()
	updated.HealthLatencyMs = 0
	updated.HealthError = ""
	updated.UpdatedAt = nextMCPServerUpdatedAt(server.UpdatedAt)
	if err := s.components.Catalog.ApplyServers(ctx, []MCPToolServerMutation{{
		Server: updated, ExpectedUpdatedAt: server.UpdatedAt,
		FieldMask: MCPToolServerMutationCapabilityFields | MCPToolServerMutationHealthFields,
	}}); err != nil {
		return nil, err
	}

	return &toolapi.DiscoverMCPToolServerData{
		Tools:     cloneToolDefinitions(capabilities.Tools),
		Resources: projectMCPResourcesForResponse(capabilities.Resources),
		Prompts:   cloneCatalogMCPPrompts(capabilities.Prompts),
	}, nil
}

func nextMCPServerUpdatedAt(expected int64) int64 {
	now := time.Now().UnixMilli()
	if now <= expected {
		return expected + 1
	}

	return now
}

func (s *ApplicationService) authorizeSpace(ctx context.Context, spaceID int64, access MCPAccess) error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if s == nil || s.authorizer == nil {
		return ErrMCPAuthorizationUnavailable
	}

	return s.authorizer.Authorize(ctx, spaceID, access)
}

func (s *ApplicationService) managementRuntimeDependencies() (RuntimeExecutor, CapabilityDiscoverer) {
	if s == nil {
		return nil, nil
	}
	s.managementRuntimeMu.RLock()
	defer s.managementRuntimeMu.RUnlock()

	return s.runtimeExecutor, s.capabilityDiscoverer
}

func (s *ApplicationService) requireCatalog() error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if s == nil || s.components == nil {
		return fmt.Errorf("mcp tool service components are required")
	}
	if s.components.Catalog == nil {
		return fmt.Errorf("mcp tool catalog is required")
	}

	return nil
}

func (s *ApplicationService) requireEnabled() error {
	if s == nil || !s.enabled {
		return ErrMCPDisabled
	}

	return nil
}

func (s *ApplicationService) requireIDGen() error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
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
	if _, err := parseJSONMap("config", normalizeJSONText(req.Config)); err != nil {
		return err
	}
	if _, err := parseJSONMap("auth", normalizeJSONText(req.Auth)); err != nil {
		return err
	}
	return nil
}

func isOfficialMCPServer(server *toolapi.MCPToolServer) bool {
	return server != nil && isOfficialMCPSourceType(server.SourceType)
}

func isOfficialMCPSourceType(sourceType toolapi.MCPServerSourceType) bool {
	return strings.EqualFold(strings.TrimSpace(string(sourceType)), "official")
}

func officialMCPServerUpdateOnlyChangesEnabled(
	existing *toolapi.MCPToolServer,
	req *toolapi.UpsertMCPToolServerRequest,
	resolvedConfig string,
	resolvedAuth string,
) bool {
	if existing == nil || req == nil {
		return false
	}

	return existing.SpaceID == req.SpaceID &&
		existing.Name == strings.TrimSpace(req.Name) &&
		existing.Description == strings.TrimSpace(req.Description) &&
		existing.ServerType == strings.TrimSpace(req.ServerType) &&
		existing.Config == resolvedConfig &&
		existing.Auth == resolvedAuth
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
	if existing != nil && (raw == "" || raw == mcpAuthConfiguredSentinel) {
		return normalizeJSONText(existing.Auth), nil
	}

	return normalizeJSONText(raw), nil
}

func isMCPAuthPreserveInput(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw == "" || raw == mcpAuthConfiguredSentinel
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
	cloned.Config = maskMCPConfigForResponse(cloned.ServerType, cloned.Config)
	cloned.Auth = opaqueMCPAuth(cloned.Auth)
	cloned.Resources = projectMCPResourcesForResponse(cloned.Resources)

	return cloned
}

func projectMCPResourcesForResponse(resources []*toolapi.MCPResource) []*toolapi.MCPResource {
	result := make([]*toolapi.MCPResource, 0, len(resources))
	for _, resource := range resources {
		if resource == nil || strings.TrimSpace(resource.URI) == "" {
			continue
		}
		digest := sha256.Sum256([]byte(strings.TrimSpace(resource.URI)))
		result = append(result, &toolapi.MCPResource{
			ResourceID:  "mcp_resource_" + hex.EncodeToString(digest[:12]),
			Name:        boundedMCPResourceMetadata(resource.Name, maxMCPNameRunes),
			Description: boundedMCPResourceMetadata(resource.Description, maxMCPDescriptionRunes),
			MIMEType:    boundedMCPResourceMetadata(resource.MIMEType, maxMCPMIMETypeRunes),
		})
	}
	return result
}

func boundedMCPResourceMetadata(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || runeLen(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func opaqueMCPAuth(raw string) string {
	payload, err := parseJSONMap("auth", normalizeJSONText(raw))
	if err != nil || len(payload) == 0 {
		return "{}"
	}

	return mcpAuthConfiguredSentinel
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
		InputSchema:     strings.TrimSpace(item.InputSchema),
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
