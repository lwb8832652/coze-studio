// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcptool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestUpsertEnabledCandidateDiscoversBeforePersist(t *testing.T) {
	catalog := NewInMemoryCatalog()
	original := managementServer(41, 10)
	require.NoError(t, catalog.Upsert(context.Background(), original))
	discoverer := &managementDiscoverer{err: errors.New("candidate unavailable")}
	svc := NewApplicationService(&Components{
		Catalog: catalog, UserSpaceRoleReader: ownerRoleReader(10),
		CapabilityDiscoverer: discoverer,
	})
	req := validManagementUpsertRequest(41, 10)
	req.Config = `{"url":"https://candidate.example.com"}`

	_, err := svc.UpsertServer(managementContext(7), req)

	require.Error(t, err)
	stored, getErr := catalog.Get(context.Background(), 41)
	require.NoError(t, getErr)
	require.Equal(t, original.Config, stored.Config)
	require.True(t, stored.Enabled)
	require.Equal(t, "search", stored.Tools[0].Name)
	connections := discoverer.recordedConnections()
	require.Len(t, connections, 1)
	require.Equal(t, req.Config, connections[0].Config)
}

func TestUpsertEnabledCreateFailureLeavesNoRecord(t *testing.T) {
	catalog := NewInMemoryCatalog()
	svc := NewApplicationService(&Components{
		Catalog: catalog, IDGen: newAtomicIDGen(900),
		UserSpaceRoleReader:  ownerRoleReader(10),
		CapabilityDiscoverer: &managementDiscoverer{err: errors.New("offline")},
	})
	req := validManagementUpsertRequest(0, 10)

	_, err := svc.UpsertServer(managementContext(7), req)

	require.Error(t, err)
	servers, listErr := catalog.List(context.Background(), 10)
	require.NoError(t, listErr)
	require.Empty(t, servers)
}

func TestUpsertEnabledCandidateAtomicallyReplacesCapabilities(t *testing.T) {
	catalog := NewInMemoryCatalog()
	require.NoError(t, catalog.Upsert(context.Background(), managementServer(41, 10)))
	discoverer := &managementDiscoverer{result: &DiscoveredCapabilities{
		Tools:     []*toolapi.MCPToolDefinition{{Name: "new_tool", Description: "new", InputSchema: `{}`}},
		Resources: []*toolapi.MCPResource{{URI: "resource://guide", Name: "guide"}},
	}}
	svc := NewApplicationService(&Components{
		Catalog: catalog, UserSpaceRoleReader: ownerRoleReader(10),
		CapabilityDiscoverer: discoverer,
	})
	req := validManagementUpsertRequest(41, 10)
	req.Config = `{"url":"https://candidate.example.com"}`

	response, err := svc.UpsertServer(managementContext(7), req)

	require.NoError(t, err)
	require.Equal(t, "new_tool", response.Data.Tools[0].Name)
	require.NotEmpty(t, response.Data.Resources[0].ResourceID)
	require.Empty(t, response.Data.Resources[0].URI)
	stored, getErr := catalog.Get(context.Background(), 41)
	require.NoError(t, getErr)
	require.Equal(t, req.Config, stored.Config)
	require.Equal(t, "new_tool", stored.Tools[0].Name)
	require.Equal(t, mcpToolHealthStatusHealthy, stored.HealthStatus)
}

func TestMCPResourceURIProjectionAndValidation(t *testing.T) {
	for _, raw := range []string{
		"https://user:pass@example.com/resource",
		"https://example.com/resource?token=secret",
		"file:///private/key",
		"object://bucket/key",
		"internal://metadata",
	} {
		require.False(t, validMCPDiscoveryURI(raw), raw)
	}
	require.True(t, validMCPDiscoveryURI("resource://guide/public?lang=zh"))
	server := managementServer(41, 10)
	server.Resources = []*toolapi.MCPResource{{
		URI: "resource://private/token", Name: "guide", Description: "safe", MIMEType: "text/plain",
	}}
	projected := cloneServerForResponse(server)
	encoded, err := json.Marshal(projected)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "resource://private/token")
	require.NotContains(t, string(encoded), `"uri"`)
	require.Contains(t, string(encoded), `"resource_id":"mcp_resource_`)
}

func TestDefaultOfficialStdioSeedsPassCommandPolicy(t *testing.T) {
	config, err := parseDeerFlowExtensionsConfig(DefaultDeerFlowMCPConfigRaw())
	require.NoError(t, err)
	require.NotContains(t, config.MCPServers, "weather")
	require.NotContains(t, config.MCPServers, "openmeteo")
	root := t.TempDir()
	for name, server := range config.MCPServers {
		if normalizeDeerFlowMCPServerType(server) != "stdio" || server.Enabled == nil || !*server.Enabled {
			continue
		}
		command := filepath.Join(root, filepath.Base(server.Command))
		require.NoError(t, os.WriteFile(command, []byte("safe"), 0o700))
		packages := []string{}
		if filepath.Base(server.Command) == "npx" {
			index := 0
			if len(server.Args) > 0 && (server.Args[0] == "-y" || server.Args[0] == "--yes") {
				index++
			}
			require.Less(t, index, len(server.Args), name)
			packages = append(packages, server.Args[index])
		}
		policy, policyErr := mcpruntime.NewStdioCommandPolicy(mcpruntime.StdioCommandPolicyOptions{
			AllowedCommands: []string{command}, NpxPackages: packages,
		})
		require.NoError(t, policyErr, name)
		_, resolveErr := policy.Resolve(command, server.Args, root)
		require.NoError(t, resolveErr, name+" "+strings.Join(server.Args, " "))
	}
}

type healthWriteFailCatalog struct {
	Catalog
}

func (c *healthWriteFailCatalog) UpdateHealth(
	context.Context, int64, int64, MCPToolHealthSnapshot,
) error {
	return errors.New("health store unavailable")
}

func TestTestCallSuccessSurvivesHealthWriteFailure(t *testing.T) {
	base := NewInMemoryCatalog()
	require.NoError(t, base.Upsert(context.Background(), managementServer(41, 10)))
	svc := NewApplicationService(&Components{
		Catalog:             &healthWriteFailCatalog{Catalog: base},
		UserSpaceRoleReader: ownerRoleReader(10),
		RuntimeExecutor: &managementAuditExecutor{execute: func(RuntimeToolCall) (*RuntimeToolResult, error) {
			return &RuntimeToolResult{Status: "success", LatencyMs: 3}, nil
		}},
		AuditRepository: &managementAuditRepositorySpy{},
	})

	response, err := svc.TestCall(managementContext(7), &toolapi.TestMCPToolCallRequest{
		ServerID: 41, ToolName: "search", Arguments: `{}`,
	})

	require.NoError(t, err)
	require.Equal(t, "success", response.Data.Status)
}
