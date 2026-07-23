/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

package mcptool

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const (
	officialMCPCatalogGitHub   = "github"
	officialMCPCatalogPostgres = "postgres"
)

type officialMCPCatalogDefinition struct {
	CatalogID          string
	Name               string
	PersistedName      string
	Description        string
	IconURL            string
	Publisher          string
	Source             string
	ServerType         string
	CredentialFields   []*toolapi.MCPOfficialCredentialField
	Tools              []*toolapi.MCPToolDefinition
	Availability       toolapi.MCPOfficialAvailability
	AvailabilityReason string
}

func baseOfficialMCPCatalogDefinitions() []*officialMCPCatalogDefinition {
	return []*officialMCPCatalogDefinition{
		{
			CatalogID: officialMCPCatalogGitHub, Name: "GitHub", PersistedName: "github",
			Description: "读取仓库、Issue 与 Pull Request，支持研发协作场景。",
			ServerType:  "stdio",
			CredentialFields: []*toolapi.MCPOfficialCredentialField{{
				Key: "github_token", Label: "GitHub Token", Required: true, Secret: true,
				Description: "建议使用最小权限 Fine-grained personal access token。",
				Placeholder: "github_pat_...",
			}},
			Tools: deerFlowMCPToolDefinitions("github"),
		},
		{
			CatalogID: officialMCPCatalogPostgres, Name: "PostgreSQL", PersistedName: "postgres",
			Description: "连接 PostgreSQL 数据库并提供只读查询等数据能力。",
			ServerType:  "stdio",
			CredentialFields: []*toolapi.MCPOfficialCredentialField{{
				Key: "database_url", Label: "数据库连接地址", Required: true, Secret: true,
				Description: "请使用权限受限的专用数据库账号。连接地址将加密保存。",
				Placeholder: "postgresql://user:password@host:5432/database",
			}},
			Tools: deerFlowMCPToolDefinitions("postgres"),
		},
	}
}

func (s *ApplicationService) ListOfficialCatalog(
	ctx context.Context,
	req *toolapi.ListMCPOfficialCatalogRequest,
) (*toolapi.ListMCPOfficialCatalogResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.SpaceID <= 0 {
		return nil, InvalidArgumentErrorf("space_id is required")
	}
	if err := s.authorizeSpace(ctx, req.SpaceID, MCPAccessRead); err != nil {
		return nil, err
	}
	servers, err := listMCPToolServersForManagement(ctx, s.components.Catalog, req.SpaceID)
	if err != nil {
		return nil, err
	}
	serversByName := make(map[string]*toolapi.MCPToolServer, len(servers))
	for _, server := range servers {
		if server != nil {
			serversByName[normalizeOfficialMCPName(server.Name)] = server
		}
	}
	definitions := officialMCPCatalogDefinitions()
	entries := make([]*toolapi.MCPOfficialCatalogEntry, 0, len(definitions))
	for _, definition := range definitions {
		entry := &toolapi.MCPOfficialCatalogEntry{
			CatalogID: definition.CatalogID, Name: definition.Name,
			Description: definition.Description, ServerType: definition.ServerType,
			IconURL: definition.IconURL, Publisher: definition.Publisher, Source: definition.Source,
			Tools:              cloneToolDefinitions(definition.Tools),
			CredentialFields:   cloneOfficialMCPCredentialFields(definition.CredentialFields),
			Availability:       definition.Availability,
			AvailabilityReason: definition.AvailabilityReason,
			InstallStatus:      toolapi.MCPOfficialInstallStatusAvailable,
		}
		if server := serversByName[normalizeOfficialMCPName(definition.PersistedName)]; server != nil {
			entry.InstallStatus = toolapi.MCPOfficialInstallStatusNeedsMigration
			if isOfficialMCPServer(server) {
				entry.InstallStatus = toolapi.MCPOfficialInstallStatusInstalled
			}
			entry.Installation = officialMCPInstallation(server)
		}
		entries = append(entries, entry)
	}
	return &toolapi.ListMCPOfficialCatalogResponse{
		Code: 0, Msg: "success",
		Data: &toolapi.ListMCPOfficialCatalogData{
			Entries: entries, Total: int64(len(entries)),
			CanManage: s.authorizeSpace(ctx, req.SpaceID, MCPAccessManage) == nil,
		},
	}, nil
}

func (s *ApplicationService) InstallOfficialCatalogEntry(
	ctx context.Context,
	req *toolapi.InstallMCPOfficialCatalogRequest,
) (*toolapi.MCPToolServerResponse, error) {
	if err := s.requireCatalog(); err != nil {
		return nil, err
	}
	if req == nil || req.SpaceID <= 0 || strings.TrimSpace(req.CatalogID) == "" {
		return nil, InvalidArgumentErrorf("catalog_id and space_id are required")
	}
	if err := s.authorizeSpace(ctx, req.SpaceID, MCPAccessManage); err != nil {
		return nil, err
	}
	definition := findOfficialMCPCatalogDefinition(req.CatalogID)
	if definition == nil {
		return nil, InvalidArgumentErrorf("official MCP catalog entry is unavailable")
	}
	config, err := buildOfficialMCPConfig(definition, req.Credentials)
	if err != nil {
		return nil, err
	}
	servers, err := listMCPToolServersForManagement(ctx, s.components.Catalog, req.SpaceID)
	if err != nil {
		return nil, err
	}
	serverID := int64(0)
	for _, server := range servers {
		if server != nil && normalizeOfficialMCPName(server.Name) == normalizeOfficialMCPName(definition.PersistedName) {
			serverID = server.ServerID
			break
		}
	}
	return s.upsertServer(ctx, &toolapi.UpsertMCPToolServerRequest{
		ServerID: serverID, SpaceID: req.SpaceID, Name: definition.PersistedName,
		Description: definition.Description, ServerType: definition.ServerType,
		Enabled: false, Config: config, Auth: `{}`, Tools: cloneToolDefinitions(definition.Tools),
	}, &trustedMCPServerFields{
		CreatorID: 0, SourceType: toolapi.MCPServerSourceTypeOfficial,
		Tools: cloneToolDefinitions(definition.Tools),
	})
}

func buildOfficialMCPConfig(
	definition *officialMCPCatalogDefinition,
	credentials map[string]string,
) (string, error) {
	if config, handled, err := buildNuwaxOfficialMCPConfig(definition, credentials); handled {
		return config, err
	}

	for _, field := range definition.CredentialFields {
		if field.Required && strings.TrimSpace(credentials[field.Key]) == "" {
			return "", InvalidArgumentErrorf("required official MCP credential is missing")
		}
	}
	var config map[string]any
	switch definition.CatalogID {
	case officialMCPCatalogGitHub:
		config = map[string]any{
			"command": "npx",
			"args":    []string{"-y", "@modelcontextprotocol/server-github"},
			"env":     map[string]string{"GITHUB_TOKEN": strings.TrimSpace(credentials["github_token"])},
		}
	case officialMCPCatalogPostgres:
		databaseURL := strings.TrimSpace(credentials["database_url"])
		parsed, err := url.Parse(databaseURL)
		if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.ContainsAny(databaseURL, "\r\n") {
			return "", InvalidArgumentErrorf("database connection address must be a valid PostgreSQL URL")
		}
		config = map[string]any{
			"command": "npx",
			"args":    []string{"-y", "@modelcontextprotocol/server-postgres", databaseURL},
		}
	default:
		return "", InvalidArgumentErrorf("official MCP catalog entry is unavailable")
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func findOfficialMCPCatalogDefinition(catalogID string) *officialMCPCatalogDefinition {
	for _, definition := range officialMCPCatalogDefinitions() {
		if definition.CatalogID == strings.ToLower(strings.TrimSpace(catalogID)) {
			return definition
		}
	}
	return nil
}

func officialMCPInstallation(server *toolapi.MCPToolServer) *toolapi.MCPOfficialInstallation {
	return &toolapi.MCPOfficialInstallation{
		ServerID: server.ServerID, Enabled: server.Enabled, HealthStatus: server.HealthStatus,
		HealthCheckedAt: server.HealthCheckedAt, HealthLatencyMs: server.HealthLatencyMs,
		UpdatedAt: server.UpdatedAt,
	}
}

func cloneOfficialMCPCredentialFields(fields []*toolapi.MCPOfficialCredentialField) []*toolapi.MCPOfficialCredentialField {
	result := make([]*toolapi.MCPOfficialCredentialField, 0, len(fields))
	for _, field := range fields {
		if field == nil {
			continue
		}
		cloned := *field
		result = append(result, &cloned)
	}
	return result
}

func normalizeOfficialMCPName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
