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
	"fmt"
	"sort"
	"strings"

	toolapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
)

const defaultDeerFlowExtensionsConfigJSON = `{
  "mcpServers": {
    "github": {
      "enabled": true,
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {"GITHUB_TOKEN": ""},
      "url": null,
      "headers": {},
      "oauth": null,
      "description": "GitHub MCP server for repository operations"
    },
    "postgres": {
      "enabled": true,
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-postgres", "postgresql://localhost/mydb"],
      "env": {},
      "url": null,
      "headers": {},
      "oauth": null,
      "description": "PostgreSQL database access"
    }
  }
}`

// DefaultDeerFlowMCPConfigRaw returns the DeerFlow default MCP server config
// that Coze imports into its durable MCP catalog.
func DefaultDeerFlowMCPConfigRaw() []byte {
	config, err := parseDeerFlowExtensionsConfig(
		[]byte(defaultDeerFlowExtensionsConfigJSON),
	)
	if err != nil {
		return []byte(defaultDeerFlowExtensionsConfigJSON)
	}
	config.MCPServers["weather"] = deerFlowMCPServerConfig{
		Enabled:     boolPtr(true),
		Type:        "stdio",
		Command:     "node",
		Args:        []string{"-e", defaultWeatherMCPStdioScript()},
		Env:         map[string]string{},
		Description: "Weather MCP server for local weather queries",
	}
	config.MCPServers["openmeteo"] = deerFlowMCPServerConfig{
		Enabled: boolPtr(true),
		Type:    "stdio",
		Command: "npx",
		Args: []string{
			"-y",
			"-p",
			"open-meteo-mcp-server",
			"open-meteo-mcp-server",
		},
		Env:         map[string]string{},
		Description: "Open-Meteo MCP server for weather forecast queries",
	}

	return mustMarshalDeerFlowExtensionsConfig(config)
}

type deerFlowExtensionsConfig struct {
	MCPServers map[string]deerFlowMCPServerConfig `json:"mcpServers"`
}

type deerFlowMCPServerConfig struct {
	Enabled     *bool             `json:"enabled"`
	Type        string            `json:"type"`
	Transport   string            `json:"transport"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	OAuth       map[string]any    `json:"oauth"`
	Description string            `json:"description"`
}

type deerFlowMCPToolField struct {
	Name string
	Type string
}

func (s *ApplicationService) ImportDeerFlowExtensionsConfig(
	ctx context.Context,
	spaceID int64,
	raw []byte,
) (*toolapi.ListMCPToolServersResponse, error) {
	if err := s.importDeerFlowExtensionsConfig(ctx, spaceID, raw, nil); err != nil {
		return nil, err
	}

	return s.ListServers(ctx, &toolapi.ListMCPToolServersRequest{SpaceID: spaceID})
}

func (s *ApplicationService) ensureDefaultDeerFlowMCPServers(
	ctx context.Context,
	spaceID int64,
) error {
	if s == nil || s.components == nil ||
		len(s.components.DefaultDeerFlowMCPConfigRaw) == 0 {
		return nil
	}
	config, err := parseDeerFlowExtensionsConfig(
		s.components.DefaultDeerFlowMCPConfigRaw,
	)
	if err != nil {
		return err
	}
	if len(config.MCPServers) == 0 {
		return nil
	}
	existing, err := s.components.Catalog.List(ctx, spaceID)
	if err != nil {
		return err
	}
	existingNames := make(map[string]struct{}, len(existing))
	for _, server := range existing {
		if server != nil {
			existingNames[strings.TrimSpace(server.Name)] = struct{}{}
		}
	}
	missing := make(map[string]struct{})
	for name := range config.MCPServers {
		if _, ok := existingNames[name]; !ok {
			missing[name] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return nil
	}

	return s.importDeerFlowExtensionsConfig(
		ctx,
		spaceID,
		s.components.DefaultDeerFlowMCPConfigRaw,
		missing,
	)
}

func (s *ApplicationService) importDeerFlowExtensionsConfig(
	ctx context.Context,
	spaceID int64,
	raw []byte,
	onlyNames map[string]struct{},
) error {
	if err := s.requireCatalog(); err != nil {
		return err
	}
	if err := s.requireIDGen(); err != nil {
		return err
	}
	if spaceID <= 0 {
		return InvalidArgumentErrorf("space_id is required")
	}
	config, err := parseDeerFlowExtensionsConfig(raw)
	if err != nil {
		return err
	}
	existing, err := s.components.Catalog.List(ctx, spaceID)
	if err != nil {
		return err
	}
	existingByName := make(map[string]*toolapi.MCPToolServer, len(existing))
	for _, server := range existing {
		if server != nil {
			existingByName[strings.TrimSpace(server.Name)] = server
		}
	}

	names := make([]string, 0, len(config.MCPServers))
	for name := range config.MCPServers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if len(onlyNames) > 0 {
			if _, ok := onlyNames[name]; !ok {
				continue
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		serverConfig := config.MCPServers[name]
		req, err := deerFlowMCPServerUpsertRequest(
			spaceID,
			name,
			serverConfig,
		)
		if err != nil {
			return err
		}
		if existing := existingByName[name]; existing != nil {
			req.ServerID = existing.ServerID
		}
		if _, err := s.UpsertServer(ctx, req); err != nil {
			return err
		}
	}

	return nil
}

func parseDeerFlowExtensionsConfig(
	raw []byte,
) (deerFlowExtensionsConfig, error) {
	if len(raw) == 0 {
		return deerFlowExtensionsConfig{},
			InvalidArgumentErrorf("deerflow extensions config is required")
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return deerFlowExtensionsConfig{},
			InvalidArgumentErrorf("invalid deerflow extensions config json: %v", err)
	}
	var servers map[string]deerFlowMCPServerConfig
	if rawServers, ok := payload["mcpServers"]; ok {
		if err := json.Unmarshal(rawServers, &servers); err != nil {
			return deerFlowExtensionsConfig{},
				InvalidArgumentErrorf("invalid deerflow mcpServers json: %v", err)
		}
	} else if rawServers, ok := payload["mcp_servers"]; ok {
		if err := json.Unmarshal(rawServers, &servers); err != nil {
			return deerFlowExtensionsConfig{},
				InvalidArgumentErrorf("invalid deerflow mcp_servers json: %v", err)
		}
	}
	if servers == nil {
		servers = map[string]deerFlowMCPServerConfig{}
	}

	return deerFlowExtensionsConfig{MCPServers: servers}, nil
}

func deerFlowMCPServerUpsertRequest(
	spaceID int64,
	name string,
	serverConfig deerFlowMCPServerConfig,
) (*toolapi.UpsertMCPToolServerRequest, error) {
	serverType := normalizeDeerFlowMCPServerType(serverConfig)
	configJSON, authJSON, err := deerFlowMCPServerConfigAndAuthJSON(
		serverType,
		serverConfig,
	)
	if err != nil {
		return nil, err
	}
	tools := deerFlowMCPToolDefinitions(name)
	if len(tools) == 0 {
		return nil, InvalidArgumentErrorf(
			"deerflow mcp server %s has no migrated tool definitions",
			name,
		)
	}
	enabled := true
	if serverConfig.Enabled != nil {
		enabled = *serverConfig.Enabled
	}

	return &toolapi.UpsertMCPToolServerRequest{
		SpaceID:     spaceID,
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(serverConfig.Description),
		ServerType:  serverType,
		Enabled:     enabled,
		Config:      configJSON,
		Auth:        authJSON,
		Tools:       tools,
	}, nil
}

func normalizeDeerFlowMCPServerType(config deerFlowMCPServerConfig) string {
	serverType := strings.TrimSpace(config.Type)
	if serverType == "" {
		serverType = strings.TrimSpace(config.Transport)
	}
	if serverType == "" {
		return "stdio"
	}
	if strings.EqualFold(serverType, "http") {
		return "streamable_http"
	}

	return strings.ToLower(serverType)
}

func deerFlowMCPServerConfigAndAuthJSON(
	serverType string,
	config deerFlowMCPServerConfig,
) (string, string, error) {
	switch serverType {
	case "stdio":
		return deerFlowStdioMCPServerConfigAndAuthJSON(config)
	case "sse", "streamable_http":
		return deerFlowRemoteMCPServerConfigAndAuthJSON(serverType, config)
	default:
		return "", "", InvalidArgumentErrorf(
			"unsupported deerflow mcp server type: %s",
			serverType,
		)
	}
}

func deerFlowStdioMCPServerConfigAndAuthJSON(
	config deerFlowMCPServerConfig,
) (string, string, error) {
	command := strings.TrimSpace(config.Command)
	if command == "" {
		return "", "", InvalidArgumentErrorf("deerflow stdio command is required")
	}
	if !allowedDeerFlowStdioCommand(command) {
		return "", "", InvalidArgumentErrorf(
			"deerflow stdio command %s is not allowed",
			command,
		)
	}

	env, authEnv, authPayload := splitDeerFlowMCPEnv(config.Env)
	payload := map[string]any{
		"command": command,
		"args":    append([]string(nil), config.Args...),
		"env":     env,
	}
	if len(authEnv) > 0 {
		payload["auth_env"] = authEnv
	}

	return mustMarshalJSONObject(payload), mustMarshalJSONObject(authPayload), nil
}

func deerFlowRemoteMCPServerConfigAndAuthJSON(
	serverType string,
	config deerFlowMCPServerConfig,
) (string, string, error) {
	url := strings.TrimSpace(config.URL)
	if url == "" {
		return "", "", InvalidArgumentErrorf(
			"deerflow %s url is required",
			serverType,
		)
	}
	payload := map[string]any{"url": url}
	headers, authHeaders, authPayload := splitDeerFlowMCPHeaders(config.Headers)
	if len(headers) > 0 {
		payload["headers"] = headers
	}
	if len(authHeaders) > 0 {
		payload["auth_headers"] = authHeaders
	}
	if len(config.OAuth) > 0 {
		authPayload["oauth"] = config.OAuth
	}

	return mustMarshalJSONObject(payload), mustMarshalJSONObject(authPayload), nil
}

func allowedDeerFlowStdioCommand(command string) bool {
	switch strings.TrimSpace(command) {
	case "npx", "node", "uvx":
		return true
	default:
		return false
	}
}

func splitDeerFlowMCPEnv(
	env map[string]string,
) (map[string]string, map[string]string, map[string]any) {
	configEnv := make(map[string]string)
	authEnv := make(map[string]string)
	authValues := make(map[string]any)
	for _, key := range sortedStringMapKeys(env) {
		value := env[key]
		if isMCPAuthSensitiveKey(key) && strings.TrimSpace(value) != "" {
			authValues[key] = value
			authEnv[key] = "env." + key
			continue
		}
		configEnv[key] = value
	}
	authPayload := map[string]any{}
	if len(authValues) > 0 {
		authPayload["env"] = authValues
	}

	return configEnv, authEnv, authPayload
}

func splitDeerFlowMCPHeaders(
	headers map[string]string,
) (map[string]string, map[string]string, map[string]any) {
	configHeaders := make(map[string]string)
	authHeaders := make(map[string]string)
	authValues := make(map[string]any)
	for _, key := range sortedStringMapKeys(headers) {
		value := headers[key]
		if isMCPAuthSensitiveKey(key) && strings.TrimSpace(value) != "" {
			authValues[key] = value
			authHeaders[key] = "headers." + key
			continue
		}
		configHeaders[key] = value
	}
	authPayload := map[string]any{}
	if len(authValues) > 0 {
		authPayload["headers"] = authValues
	}

	return configHeaders, authHeaders, authPayload
}

func deerFlowMCPToolDefinitions(name string) []*toolapi.MCPToolDefinition {
	switch strings.TrimSpace(name) {
	case "github":
		return deerFlowGitHubMCPToolDefinitions()
	case "openmeteo":
		return deerFlowOpenMeteoMCPToolDefinitions()
	case "postgres":
		return deerFlowPostgresMCPToolDefinitions()
	case "weather":
		return deerFlowWeatherMCPToolDefinitions()
	default:
		return nil
	}
}

func deerFlowOpenMeteoMCPToolDefinitions() []*toolapi.MCPToolDefinition {
	return []*toolapi.MCPToolDefinition{
		{
			Name:        "geocoding",
			Description: "Search locations and return coordinates using Open-Meteo geocoding",
			InputSchema: deerFlowMCPObjectSchema(
				[]string{"name"},
				deerFlowMCPToolField{Name: "name", Type: "string"},
				deerFlowMCPToolField{Name: "count", Type: "number"},
				deerFlowMCPToolField{Name: "language", Type: "string"},
				deerFlowMCPToolField{Name: "countryCode", Type: "string"},
			),
		},
		{
			Name:        "weather_forecast",
			Description: "Get weather forecast data for coordinates using Open-Meteo API",
			InputSchema: deerFlowMCPObjectSchema(
				[]string{"latitude", "longitude"},
				deerFlowMCPToolField{Name: "latitude", Type: "number"},
				deerFlowMCPToolField{Name: "longitude", Type: "number"},
				deerFlowMCPToolField{Name: "current", Type: "array"},
				deerFlowMCPToolField{Name: "current_weather", Type: "boolean"},
				deerFlowMCPToolField{Name: "hourly", Type: "array"},
				deerFlowMCPToolField{Name: "daily", Type: "array"},
				deerFlowMCPToolField{Name: "forecast_days", Type: "number"},
				deerFlowMCPToolField{Name: "past_days", Type: "number"},
				deerFlowMCPToolField{Name: "timezone", Type: "string"},
				deerFlowMCPToolField{Name: "temperature_unit", Type: "string"},
				deerFlowMCPToolField{Name: "wind_speed_unit", Type: "string"},
				deerFlowMCPToolField{Name: "precipitation_unit", Type: "string"},
				deerFlowMCPToolField{Name: "models", Type: "string"},
			),
		},
	}
}

func deerFlowWeatherMCPToolDefinitions() []*toolapi.MCPToolDefinition {
	return []*toolapi.MCPToolDefinition{
		{
			Name:        "get_weather",
			Description: "Get current weather for a city",
			InputSchema: deerFlowMCPObjectSchema(
				[]string{"city"},
				deerFlowMCPToolField{Name: "city", Type: "string"},
				deerFlowMCPToolField{Name: "unit", Type: "string"},
			),
		},
	}
}

func deerFlowPostgresMCPToolDefinitions() []*toolapi.MCPToolDefinition {
	return []*toolapi.MCPToolDefinition{
		{
			Name:        "query",
			Description: "Run a read-only SQL query",
			InputSchema: deerFlowMCPObjectSchema(
				[]string{"sql"},
				deerFlowMCPToolField{Name: "sql", Type: "string"},
			),
		},
	}
}

func deerFlowGitHubMCPToolDefinitions() []*toolapi.MCPToolDefinition {
	return []*toolapi.MCPToolDefinition{
		deerFlowGitHubTool(
			"create_or_update_file",
			"Create or update a single file in a GitHub repository",
			[]string{"owner", "repo", "path", "content", "message", "branch"},
			fields("owner:string", "repo:string", "path:string", "content:string",
				"message:string", "branch:string", "sha:string"),
		),
		deerFlowGitHubTool(
			"search_repositories",
			"Search for GitHub repositories",
			[]string{"query"},
			fields("query:string", "page:number", "perPage:number"),
		),
		deerFlowGitHubTool(
			"create_repository",
			"Create a new GitHub repository in your account",
			[]string{"name"},
			fields("name:string", "description:string", "private:boolean",
				"autoInit:boolean"),
		),
		deerFlowGitHubTool(
			"get_file_contents",
			"Get the contents of a file or directory from a GitHub repository",
			[]string{"owner", "repo", "path"},
			fields("owner:string", "repo:string", "path:string", "branch:string"),
		),
		deerFlowGitHubTool(
			"push_files",
			"Push multiple files to a GitHub repository in a single commit",
			[]string{"owner", "repo", "branch", "files", "message"},
			fields("owner:string", "repo:string", "branch:string", "files:array",
				"message:string"),
		),
		deerFlowGitHubTool(
			"create_issue",
			"Create a new issue in a GitHub repository",
			[]string{"owner", "repo", "title"},
			fields("owner:string", "repo:string", "title:string", "body:string",
				"assignees:array", "labels:array", "milestone:number"),
		),
		deerFlowGitHubTool(
			"create_pull_request",
			"Create a new pull request in a GitHub repository",
			[]string{"owner", "repo", "title", "head", "base"},
			fields("owner:string", "repo:string", "title:string", "body:string",
				"head:string", "base:string", "draft:boolean",
				"maintainer_can_modify:boolean"),
		),
		deerFlowGitHubTool(
			"fork_repository",
			"Fork a GitHub repository to your account or specified organization",
			[]string{"owner", "repo"},
			fields("owner:string", "repo:string", "organization:string"),
		),
		deerFlowGitHubTool(
			"create_branch",
			"Create a new branch in a GitHub repository",
			[]string{"owner", "repo", "branch"},
			fields("owner:string", "repo:string", "branch:string",
				"from_branch:string"),
		),
		deerFlowGitHubTool(
			"list_commits",
			"Get list of commits of a branch in a GitHub repository",
			[]string{"owner", "repo"},
			fields("owner:string", "repo:string", "page:number",
				"perPage:number", "sha:string"),
		),
		deerFlowGitHubTool(
			"list_issues",
			"List issues in a GitHub repository with filtering options",
			[]string{"owner", "repo"},
			fields("owner:string", "repo:string", "state:string", "labels:array",
				"sort:string", "direction:string", "since:string", "page:number",
				"per_page:number"),
		),
		deerFlowGitHubTool(
			"update_issue",
			"Update an existing issue in a GitHub repository",
			[]string{"owner", "repo", "issue_number"},
			fields("owner:string", "repo:string", "issue_number:number",
				"title:string", "body:string", "state:string", "labels:array",
				"assignees:array", "milestone:number"),
		),
		deerFlowGitHubTool(
			"add_issue_comment",
			"Add a comment to an existing issue",
			[]string{"owner", "repo", "issue_number", "body"},
			fields("owner:string", "repo:string", "issue_number:number",
				"body:string"),
		),
		deerFlowGitHubTool(
			"search_code",
			"Search for code across GitHub repositories",
			[]string{"q"},
			fields("q:string", "sort:string", "order:string", "per_page:number",
				"page:number"),
		),
		deerFlowGitHubTool(
			"search_issues",
			"Search for issues and pull requests across GitHub repositories",
			[]string{"q"},
			fields("q:string", "sort:string", "order:string", "per_page:number",
				"page:number"),
		),
		deerFlowGitHubTool(
			"search_users",
			"Search for users on GitHub",
			[]string{"q"},
			fields("q:string", "sort:string", "order:string", "per_page:number",
				"page:number"),
		),
		deerFlowGitHubTool(
			"get_issue",
			"Get details of a specific issue in a GitHub repository.",
			[]string{"owner", "repo", "issue_number"},
			fields("owner:string", "repo:string", "issue_number:number"),
		),
		deerFlowGitHubTool(
			"get_pull_request",
			"Get details of a specific pull request",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number"),
		),
		deerFlowGitHubTool(
			"list_pull_requests",
			"List and filter repository pull requests",
			[]string{"owner", "repo"},
			fields("owner:string", "repo:string", "state:string", "head:string",
				"base:string", "sort:string", "direction:string",
				"per_page:number", "page:number"),
		),
		deerFlowGitHubTool(
			"create_pull_request_review",
			"Create a review on a pull request",
			[]string{"owner", "repo", "pull_number", "body", "event"},
			fields("owner:string", "repo:string", "pull_number:number",
				"body:string", "event:string", "commit_id:string",
				"comments:array"),
		),
		deerFlowGitHubTool(
			"merge_pull_request",
			"Merge a pull request",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number",
				"commit_title:string", "commit_message:string",
				"merge_method:string"),
		),
		deerFlowGitHubTool(
			"get_pull_request_files",
			"Get the list of files changed in a pull request",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number"),
		),
		deerFlowGitHubTool(
			"get_pull_request_status",
			"Get the combined status of all status checks for a pull request",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number"),
		),
		deerFlowGitHubTool(
			"update_pull_request_branch",
			"Update a pull request branch with the latest changes from the base branch",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number",
				"expected_head_sha:string"),
		),
		deerFlowGitHubTool(
			"get_pull_request_comments",
			"Get the review comments on a pull request",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number"),
		),
		deerFlowGitHubTool(
			"get_pull_request_reviews",
			"Get the reviews on a pull request",
			[]string{"owner", "repo", "pull_number"},
			fields("owner:string", "repo:string", "pull_number:number"),
		),
	}
}

func deerFlowGitHubTool(
	name string,
	description string,
	required []string,
	fields []deerFlowMCPToolField,
) *toolapi.MCPToolDefinition {
	return &toolapi.MCPToolDefinition{
		Name:        name,
		Description: description,
		InputSchema: deerFlowMCPObjectSchema(required, fields...),
	}
}

func fields(values ...string) []deerFlowMCPToolField {
	result := make([]deerFlowMCPToolField, 0, len(values))
	for _, value := range values {
		name, typ, _ := strings.Cut(value, ":")
		result = append(result, deerFlowMCPToolField{
			Name: strings.TrimSpace(name),
			Type: strings.TrimSpace(typ),
		})
	}

	return result
}

func deerFlowMCPObjectSchema(
	required []string,
	fields ...deerFlowMCPToolField,
) string {
	properties := make(map[string]any, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			continue
		}
		properties[field.Name] = deerFlowMCPFieldSchema(field.Type)
	}
	payload := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		payload["required"] = append([]string(nil), required...)
	}

	return mustMarshalJSONObject(payload)
}

func deerFlowMCPFieldSchema(typ string) map[string]any {
	switch strings.TrimSpace(typ) {
	case "boolean":
		return map[string]any{"type": "boolean"}
	case "number":
		return map[string]any{"type": "number"}
	case "array":
		return map[string]any{"type": "array"}
	default:
		return map[string]any{"type": "string"}
	}
}

func sortedStringMapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func mustMarshalJSONObject(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(fmt.Sprintf("marshal json object: %v", err))
	}

	return string(encoded)
}

func mustMarshalDeerFlowExtensionsConfig(config deerFlowExtensionsConfig) []byte {
	encoded, err := json.Marshal(config)
	if err != nil {
		panic(fmt.Sprintf("marshal deerflow extensions config: %v", err))
	}

	return encoded
}

func boolPtr(value bool) *bool {
	return &value
}

func defaultWeatherMCPStdioScript() string {
	return `const r=require("readline").createInterface({input:process.stdin});const tool={name:"get_weather",description:"Get current weather for a city",inputSchema:{type:"object",properties:{city:{type:"string",description:"City name, such as Wuhan or Shanghai"},unit:{type:"string",description:"celsius or fahrenheit"}},required:["city"]}};function w(o){process.stdout.write(JSON.stringify(o)+"\n")}function weather(a){const city=String((a&&a.city)||"").trim()||"武汉";const unit=String((a&&a.unit)||"celsius").toLowerCase();const data={"武汉":{condition:"多云",temperature_c:29,humidity_percent:72,wind:"东北风 2级",advisory:"适合出行，注意补水"},"上海":{condition:"小雨",temperature_c:27,humidity_percent:80,wind:"东南风 3级",advisory:"建议带伞"},"北京":{condition:"晴",temperature_c:31,humidity_percent:38,wind:"西南风 2级",advisory:"紫外线较强"}}[city]||{condition:"晴到多云",temperature_c:26,humidity_percent:60,wind:"微风",advisory:"天气平稳"};const t=unit==="fahrenheit"?Math.round(data.temperature_c*9/5+32):data.temperature_c;return {city,condition:data.condition,temperature:t,unit:unit==="fahrenheit"?"fahrenheit":"celsius",humidity_percent:data.humidity_percent,wind:data.wind,advisory:data.advisory,source:"coze-local-weather-mcp"}}r.on("line",l=>{let m;try{m=JSON.parse(l)}catch(e){return}if(!m||m.id===undefined){return}if(m.method==="initialize"){w({jsonrpc:"2.0",id:m.id,result:{protocolVersion:"2025-03-26",capabilities:{tools:{}},serverInfo:{name:"coze-weather-mcp",version:"1.0.0"}}});return}if(m.method==="tools/list"){w({jsonrpc:"2.0",id:m.id,result:{tools:[tool]}});return}if(m.method==="tools/call"){const args=(m.params&&m.params.arguments)||{};const out=weather(args);w({jsonrpc:"2.0",id:m.id,result:{content:[{type:"text",text:JSON.stringify(out)}],structuredContent:out}});return}w({jsonrpc:"2.0",id:m.id,error:{code:-32601,message:"method not found"}})})`
}
