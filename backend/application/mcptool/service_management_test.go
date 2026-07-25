// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/application/base/ctxutil"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type managementRoleReader struct {
	spaces []*userentity.Space
	members []*userentity.SpaceMember
	err    error
}

func (r *managementRoleReader) GetUserSpaceList(
	_ context.Context,
	_ int64,
) ([]*userentity.Space, error) {
	return r.spaces, r.err
}

func (r *managementRoleReader) GetSpaceMembers(
	_ context.Context,
	_ int64,
) ([]*userentity.SpaceMember, error) {
	if len(r.members) > 0 {
		return r.members, r.err
	}
	members := make([]*userentity.SpaceMember, 0, len(r.spaces))
	for _, space := range r.spaces {
		if space == nil {
			continue
		}
		members = append(members, &userentity.SpaceMember{
			UserID:   7,
			RoleType: space.RoleType,
		})
	}
	return members, r.err
}

func (r *managementRoleReader) ListSpaceMemberRoles(
	_ context.Context,
	_ int64,
) ([]SpaceMemberRole, error) {
	roles := make([]SpaceMemberRole, 0, len(r.spaces))
	for _, space := range r.spaces {
		if space == nil {
			continue
		}
		roles = append(roles, SpaceMemberRole{
			UserID:   7,
			RoleType: space.RoleType,
		})
	}
	return roles, r.err
}

type managementRuntimeExecutor struct {
	mu     sync.Mutex
	calls  []RuntimeToolCall
	result *RuntimeToolResult
	err    error
}

func (e *managementRuntimeExecutor) ExecuteMCPTool(
	_ context.Context,
	call RuntimeToolCall,
) (*RuntimeToolResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls = append(e.calls, call)
	return e.result, e.err
}

func (e *managementRuntimeExecutor) recordedCalls() []RuntimeToolCall {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]RuntimeToolCall(nil), e.calls...)
}

type managementDiscoverer struct {
	mu          sync.Mutex
	connections []MCPServerConnection
	result      *DiscoveredCapabilities
	err         error
}

func (d *managementDiscoverer) Discover(
	_ context.Context,
	connection MCPServerConnection,
) (*DiscoveredCapabilities, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.connections = append(d.connections, connection)
	return d.result, d.err
}

func (d *managementDiscoverer) recordedConnections() []MCPServerConnection {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]MCPServerConnection(nil), d.connections...)
}

func TestApplicationServiceAuthorizationFailsClosedAndUsesSpaceRoles(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))

	ownerReader := &managementRoleReader{spaces: []*userentity.Space{{ID: 10, RoleType: spaceRoleOwner}}}
	memberReader := &managementRoleReader{spaces: []*userentity.Space{{ID: 10, RoleType: 3}}}

	t.Run("missing authorization dependency", func(t *testing.T) {
		svc := NewApplicationService(&Components{Catalog: catalog})
		_, err := svc.ListServers(managementContext(7), &toolapi.ListMCPToolServersRequest{SpaceID: 10})
		require.ErrorIs(t, err, ErrMCPAuthorizationUnavailable)
	})

	t.Run("unauthenticated", func(t *testing.T) {
		svc := NewApplicationService(&Components{Catalog: catalog, UserSpaceRoleReader: ownerReader})
		_, err := svc.ListServers(context.Background(), &toolapi.ListMCPToolServersRequest{SpaceID: 10})
		require.ErrorIs(t, err, ErrMCPUnauthenticated)
	})

	t.Run("non member", func(t *testing.T) {
		svc := NewApplicationService(&Components{
			Catalog:             catalog,
			UserSpaceRoleReader: &managementRoleReader{},
		})
		_, err := svc.GetServer(managementContext(7), &toolapi.GetMCPToolServerRequest{ServerID: 41})
		require.ErrorIs(t, err, ErrMCPForbidden)
	})

	t.Run("member can use every public list and get", func(t *testing.T) {
		svc := NewApplicationService(&Components{Catalog: catalog, UserSpaceRoleReader: memberReader})
		ctx := managementContext(7)

		_, err := svc.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: 10})
		require.NoError(t, err)
		_, err = svc.ListSkillToolCandidates(ctx, 10)
		require.NoError(t, err)
		_, err = svc.ListRegistryEntries(ctx, &toolapi.ListMCPToolRegistryEntriesRequest{SpaceID: 10})
		require.NoError(t, err)
		_, err = svc.ListMCPToolRegistryEntries(ctx, 10)
		require.NoError(t, err)
		_, err = svc.GetServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
		require.NoError(t, err)
		_, err = svc.SafeExport(ctx, 41)
		require.NoError(t, err)
	})

	t.Run("member cannot manage", func(t *testing.T) {
		executor := &managementRuntimeExecutor{result: &RuntimeToolResult{Status: "success"}}
		discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{}}
		svc := NewApplicationService(&Components{
			Catalog:              catalog,
			IDGen:                &sequentialIDGen{next: 100},
			UserSpaceRoleReader:  memberReader,
			RuntimeExecutor:      executor,
			CapabilityDiscoverer: discoverer,
		})
		ctx := managementContext(7)

		_, err := svc.UpsertServer(ctx, validManagementUpsertRequest(0, 10))
		require.ErrorIs(t, err, ErrMCPForbidden)
		_, err = svc.UpsertServer(ctx, validManagementUpsertRequest(41, 10))
		require.ErrorIs(t, err, ErrMCPForbidden)
		_, err = svc.DeleteServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
		require.ErrorIs(t, err, ErrMCPForbidden)
		_, err = svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
			ServerID: 41, ToolName: "search", Arguments: `{}`,
		})
		require.ErrorIs(t, err, ErrMCPForbidden)
		_, err = svc.Discover(ctx, 41)
		require.ErrorIs(t, err, ErrMCPForbidden)
	})
}

func TestApplicationServiceManagementDerivesAndPreservesOwnedFields(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		IDGen:                &sequentialIDGen{next: 100},
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{result: &DiscoveredCapabilities{}},
	})
	ctx := managementContext(77)
	request := validManagementUpsertRequest(0, 10)
	request.Tools = []*toolapi.MCPToolDefinition{{Name: "browser-injected"}}

	created, err := svc.UpsertServer(ctx, request)

	require.NoError(t, err)
	require.Equal(t, int64(77), created.Data.CreatorID)
	require.Equal(t, toolapi.MCPServerSourceTypeCustom, created.Data.SourceType)
	require.Empty(t, created.Data.Tools)
	require.Empty(t, created.Data.Resources)
	require.Empty(t, created.Data.Prompts)

	stored, err := catalog.Get(ctx, 100)
	require.NoError(t, err)
	stored.CreatorID = 51
	stored.Tools = []*toolapi.MCPToolDefinition{{Name: "trusted-tool", InputSchema: `{"type":"object"}`}}
	stored.Resources = []*toolapi.MCPResource{{URI: "resource://trusted", Name: "trusted-resource"}}
	stored.Prompts = []*toolapi.MCPPrompt{{Name: "trusted-prompt", Arguments: []*toolapi.MCPPromptArgument{}}}
	require.NoError(t, catalog.Upsert(ctx, stored))

	update := validManagementUpsertRequest(100, 10)
	update.Name = "updated-name"
	update.Config = `{"url":"https://updated.example.com"}`
	update.Tools = []*toolapi.MCPToolDefinition{{Name: "tampered-tool"}}
	_, err = svc.UpsertServer(ctx, update)
	require.NoError(t, err)

	updated, err := catalog.Get(ctx, 100)
	require.NoError(t, err)
	require.Equal(t, int64(51), updated.CreatorID)
	require.Equal(t, toolapi.MCPServerSourceTypeCustom, updated.SourceType)
	require.Equal(t, "updated-name", updated.Name)
	require.JSONEq(t, `{"url":"https://updated.example.com"}`, updated.Config)
	require.Empty(t, updated.Tools)
	require.Empty(t, updated.Resources)
	require.Empty(t, updated.Prompts)
}

func TestApplicationServiceOfficialServerOnlyAllowsEnabledChanges(t *testing.T) {
	catalog := NewInMemoryCatalog()
	official := managementServer(41, 10)
	official.SourceType = toolapi.MCPServerSourceType("official")
	official.CreatorID = 1
	official.Auth = `{"token":"official-secret"}`
	require.NoError(t, catalog.Upsert(context.Background(), official))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})
	ctx := managementContext(7)

	changedConfig := validManagementUpsertRequest(41, 10)
	changedConfig.Name = official.Name
	changedConfig.Description = official.Description
	changedConfig.ServerType = official.ServerType
	changedConfig.Config = `{"url":"https://attacker.example.com"}`
	_, err := svc.UpsertServer(ctx, changedConfig)
	require.ErrorIs(t, err, ErrMCPOfficialServerImmutable)

	unchanged, err := catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.Equal(t, official.Config, unchanged.Config)
	require.True(t, unchanged.Enabled)

	toggle := validManagementUpsertRequest(41, 10)
	toggle.Name = official.Name
	toggle.Description = official.Description
	toggle.ServerType = official.ServerType
	toggle.Config = official.Config
	toggle.Auth = mcpAuthConfiguredSentinel
	toggle.Enabled = false
	toggle.Tools = []*toolapi.MCPToolDefinition{{Name: "browser-injected"}}
	_, err = svc.UpsertServer(ctx, toggle)
	require.NoError(t, err)

	updated, err := catalog.Get(ctx, 41)
	require.NoError(t, err)
	expected := cloneServer(official)
	expected.Enabled = false
	expected.UpdatedAt = updated.UpdatedAt
	require.Equal(t, expected, updated)

	_, err = svc.DeleteServer(ctx, &toolapi.GetMCPToolServerRequest{ServerID: 41})
	require.ErrorIs(t, err, ErrMCPOfficialServerImmutable)
}

func TestApplicationServiceRuntimeTestCallUsesRealServerAndUpdatesHealth(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	require.NoError(t, catalog.Upsert(context.Background(), server))
	executor := &managementRuntimeExecutor{
		result: &RuntimeToolResult{Status: "success", Output: `{"items":["coze"]}`, LatencyMs: 23},
	}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
	})
	ctx := managementContext(7)

	response, err := svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{"query":"coze"}`,
	})

	require.NoError(t, err)
	require.Equal(t, "success", response.Data.Status)
	require.JSONEq(t, `{"result":"completed"}`, response.Data.Output)
	require.Equal(t, int64(23), response.Data.LatencyMs)
	require.Equal(t, []RuntimeToolCall{{
		SpaceID: 10, ServerID: 41, ToolName: "search", Arguments: `{"query":"coze"}`,
	}}, executor.recordedCalls())
	require.NotContains(t, response.Data.Output, "mcp_test_call_stub")

	healthy, err := catalog.Get(ctx, 41)
	require.NoError(t, err)
	require.Equal(t, mcpToolHealthStatusHealthy, healthy.HealthStatus)
	require.Equal(t, int64(23), healthy.HealthLatencyMs)
	require.Empty(t, healthy.HealthError)
	require.Positive(t, healthy.HealthCheckedAt)

	t.Run("validates before executing", func(t *testing.T) {
		healthy.Enabled = false
		require.NoError(t, catalog.Upsert(ctx, healthy))
		before := len(executor.recordedCalls())

		_, err := svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
			ServerID: 41, ToolName: "search", Arguments: `{}`,
		})
		require.Error(t, err)
		_, err = svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
			ServerID: 41, ToolName: "missing", Arguments: `{}`,
		})
		require.Error(t, err)
		_, err = svc.TestCall(ctx, &toolapi.TestMCPToolCallRequest{
			ServerID: 41, ToolName: "search", Arguments: `[]`,
		})
		require.Error(t, err)
		require.Len(t, executor.recordedCalls(), before)
	})
}

func TestApplicationServiceRuntimeTestCallSanitizesFailures(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	executor := &managementRuntimeExecutor{err: errors.New("dial failed with token super-secret")}
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor:     executor,
	})

	_, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.ErrorIs(t, err, ErrRuntimeCallFailed)
	require.NotContains(t, err.Error(), "super-secret")
	stored, getErr := catalog.Get(context.Background(), 41)
	require.NoError(t, getErr)
	require.Equal(t, mcpToolHealthStatusUnhealthy, stored.HealthStatus)
	require.Equal(t, "runtime_failed", stored.HealthError)
	require.NotContains(t, stored.HealthError, "super-secret")
}

func TestApplicationServiceDiscoverUsesRawConnectionAndAtomicallyPersistsCapabilities(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.CreatorID = 22
	server.Config = `{"url":"https://mcp.example.com"}`
	server.Auth = `{"token":"raw-secret"}`
	require.NoError(t, catalog.Upsert(context.Background(), server))
	discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{
		Tools:     []*toolapi.MCPToolDefinition{{Name: "forecast", InputSchema: `{"type":"object"}`}},
		Resources: []*toolapi.MCPResource{{URI: "resource://guide", Name: "guide"}},
		Prompts:   []*toolapi.MCPPrompt{{Name: "summarize", Arguments: []*toolapi.MCPPromptArgument{}}},
	}}
	svc := NewApplicationService(&Components{
		Catalog:              catalog,
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: discoverer,
	})

	result, err := svc.Discover(managementContext(7), 41)

	require.NoError(t, err)
	require.Equal(t, discoverer.result.Tools, result.Tools)
	require.Len(t, result.Resources, 1)
	require.Empty(t, result.Resources[0].URI)
	require.NotEmpty(t, result.Resources[0].ResourceID)
	require.Equal(t, discoverer.result.Resources[0].Name, result.Resources[0].Name)
	require.Equal(t, discoverer.result.Prompts, result.Prompts)
	require.Equal(t, []MCPServerConnection{{
		ServerType: server.ServerType,
		Config:     server.Config,
		Auth:       server.Auth,
	}}, discoverer.recordedConnections())

	stored, err := catalog.Get(context.Background(), 41)
	require.NoError(t, err)
	require.Equal(t, int64(22), stored.CreatorID)
	require.Equal(t, server.SourceType, stored.SourceType)
	require.Equal(t, server.Config, stored.Config)
	require.Equal(t, server.Auth, stored.Auth)
	require.Equal(t, discoverer.result.Tools, stored.Tools)
	require.Equal(t, discoverer.result.Resources, stored.Resources)
	require.Equal(t, discoverer.result.Prompts, stored.Prompts)

	missing := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})
	_, err = missing.Discover(managementContext(7), 41)
	require.ErrorIs(t, err, ErrCapabilityDiscoveryUnavailable)
}

func TestApplicationServiceExportRequiresReadAndNeverContainsAuth(t *testing.T) {
	catalog := NewInMemoryCatalog()
	server := managementServer(41, 10)
	server.Auth = `{"token":"must-not-export"}`
	server.Resources = []*toolapi.MCPResource{{URI: "resource://guide"}}
	server.Prompts = []*toolapi.MCPPrompt{{Name: "summarize", Arguments: []*toolapi.MCPPromptArgument{}}}
	require.NoError(t, catalog.Upsert(context.Background(), server))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: &managementRoleReader{spaces: []*userentity.Space{{ID: 10, RoleType: 3}}},
	})

	exported, err := svc.SafeExport(managementContext(7), 41)

	require.NoError(t, err)
	require.Equal(t, server.Name, exported.Name)
	require.Equal(t, server.Tools, exported.Tools)
	require.Len(t, exported.Resources, 1)
	require.Empty(t, exported.Resources[0].URI)
	require.NotEmpty(t, exported.Resources[0].ResourceID)
	require.Equal(t, server.Prompts, exported.Prompts)
	encoded, err := json.Marshal(exported)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "must-not-export")
	require.NotContains(t, string(encoded), `"auth"`)
}

func TestApplicationServiceManagementRuntimeBindingCannotReplaceDependencies(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	svc := NewApplicationService(&Components{
		Catalog:             catalog,
		UserSpaceRoleReader: ownerRoleReader(10),
	})
	firstExecutor := &managementRuntimeExecutor{
		result: &RuntimeToolResult{Status: "success", Output: `{"source":"first"}`},
	}
	firstDiscoverer := &managementDiscoverer{result: &DiscoveredCapabilities{
		Tools: []*toolapi.MCPToolDefinition{{Name: "first", InputSchema: `{}`}},
	}}

	require.NoError(t, svc.BindManagementRuntime(firstExecutor, firstDiscoverer))
	err := svc.BindManagementRuntime(
		&managementRuntimeExecutor{result: &RuntimeToolResult{Status: "success", Output: `{"source":"second"}`}},
		&managementDiscoverer{result: &DiscoveredCapabilities{Tools: []*toolapi.MCPToolDefinition{{Name: "second", InputSchema: `{}`}}}},
	)
	require.ErrorIs(t, err, ErrManagementRuntimeAlreadyBound)

	response, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"result":"completed"}`, response.Data.Output)
	_, err = svc.Discover(managementContext(7), 41)
	require.NoError(t, err)
	require.Len(t, firstDiscoverer.recordedConnections(), 1)
}

func managementContext(userID int64) context.Context {
	ctx := ctxcache.Init(context.Background())
	ctxcache.Store(ctx, consts.SessionDataKeyInCtx, &userentity.Session{UserID: userID})
	requireUserID, ok := authenticatedUserID(ctx)
	if !ok || requireUserID != userID || ctxutil.GetUIDFromCtx(ctx) == nil {
		panic("management test context is not authenticated")
	}
	return ctx
}

func ownerRoleReader(spaceID int64) *managementRoleReader {
	return &managementRoleReader{spaces: []*userentity.Space{{ID: spaceID, RoleType: spaceRoleOwner}}}
}

func managementServer(serverID, spaceID int64) *toolapi.MCPToolServer {
	return &toolapi.MCPToolServer{
		ServerID:    serverID,
		SpaceID:     spaceID,
		CreatorID:   7,
		SourceType:  toolapi.MCPServerSourceTypeCustom,
		Name:        "management-mcp",
		Description: "management test server",
		ServerType:  "streamable_http",
		Enabled:     true,
		Config:      `{"url":"https://mcp.example.com"}`,
		Auth:        `{}`,
		Tools: []*toolapi.MCPToolDefinition{{
			Name: "search", Description: "Search", InputSchema: `{"type":"object"}`,
		}},
		HealthStatus: mcpToolHealthStatusUnknown,
		CreatedAt:    100,
		UpdatedAt:    100,
	}
}

func validManagementUpsertRequest(serverID, spaceID int64) *toolapi.UpsertMCPToolServerRequest {
	return &toolapi.UpsertMCPToolServerRequest{
		ServerID:    serverID,
		SpaceID:     spaceID,
		Name:        "management-mcp",
		Description: "management test server",
		ServerType:  "streamable_http",
		Enabled:     true,
		Config:      `{"url":"https://mcp.example.com"}`,
		Auth:        `{}`,
	}
}
