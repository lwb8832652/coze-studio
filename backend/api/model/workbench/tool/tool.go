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

package tool

type MCPToolDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"`
}

type MCPServerSourceType string

const (
	MCPServerSourceTypeCustom   MCPServerSourceType = "custom"
	MCPServerSourceTypeOfficial MCPServerSourceType = "official"
)

func (s MCPServerSourceType) Valid() bool {
	return s == MCPServerSourceTypeCustom || s == MCPServerSourceTypeOfficial
}

type MCPResource struct {
	// URI is an internal runtime locator. It must never be serialized through a
	// Workbench response or export.
	URI         string `json:"-"`
	ResourceID  string `json:"resource_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MIMEType    string `json:"mime_type"`
}

type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type MCPPrompt struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Arguments   []*MCPPromptArgument `json:"arguments"`
}

type MCPToolServer struct {
	ServerID        int64                `json:"server_id,string"`
	SpaceID         int64                `json:"space_id,string"`
	CreatorID       int64                `json:"creator_id,string"`
	SourceType      MCPServerSourceType  `json:"source_type"`
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	ServerType      string               `json:"server_type"`
	Enabled         bool                 `json:"enabled"`
	Config          string               `json:"config"`
	Auth            string               `json:"auth"`
	Tools           []*MCPToolDefinition `json:"tools"`
	Resources       []*MCPResource       `json:"resources"`
	Prompts         []*MCPPrompt         `json:"prompts"`
	HealthStatus    string               `json:"health_status"`
	HealthCheckedAt int64                `json:"health_checked_at"`
	HealthLatencyMs int64                `json:"health_latency_ms"`
	HealthError     string               `json:"health_error"`
	CreatedAt       int64                `json:"created_at"`
	UpdatedAt       int64                `json:"updated_at"`
}

type UpsertMCPToolServerRequest struct {
	ServerID    int64                `path:"server_id" json:"server_id,string,omitempty"`
	SpaceID     int64                `json:"space_id,string,required" query:"space_id"`
	Name        string               `json:"name,required"`
	Description string               `json:"description,omitempty"`
	ServerType  string               `json:"server_type,required"`
	Enabled     bool                 `json:"enabled"`
	Config      string               `json:"config,required"`
	Auth        string               `json:"auth,omitempty"`
	Tools       []*MCPToolDefinition `json:"tools,required"`
}

type ListMCPToolServersRequest struct {
	SpaceID int64 `query:"space_id,required"`
}

type ListMCPToolRegistryEntriesRequest struct {
	SpaceID int64 `query:"space_id,required"`
}

type GetMCPToolServerRequest struct {
	ServerID int64 `path:"server_id,required"`
}

type TestMCPToolCallRequest struct {
	ServerID  int64  `path:"server_id,required" json:"-"`
	ToolName  string `json:"tool_name,required"`
	Arguments string `json:"arguments,required"`
}

type DiscoverMCPToolServerRequest struct {
	ServerID int64 `path:"server_id,required" json:"-"`
}

type ExportMCPToolServerRequest struct {
	ServerID int64 `path:"server_id,required" json:"-"`
}

type ListMCPRuntimeAuditEventsRequest struct {
	ServerID int64  `path:"server_id,required" json:"-"`
	Limit    int32  `query:"limit"`
	Cursor   string `query:"cursor"`
}

type ListMCPToolServersData struct {
	Servers   []*MCPToolServer `json:"servers"`
	Total     int64            `json:"total"`
	CanManage bool             `json:"can_manage"`
}

type MCPToolRegistryEntry struct {
	Name            string `json:"name"`
	Source          string `json:"source"`
	Category        string `json:"category"`
	Visibility      string `json:"visibility"`
	ServerID        int64  `json:"server_id,string"`
	ServerName      string `json:"server_name"`
	ToolName        string `json:"tool_name"`
	Description     string `json:"description"`
	InputSchema     string `json:"input_schema"`
	Enabled         bool   `json:"enabled"`
	HealthStatus    string `json:"health_status"`
	HealthCheckedAt int64  `json:"health_checked_at"`
	HealthLatencyMs int64  `json:"health_latency_ms"`
	HealthError     string `json:"health_error"`
}

type ListMCPToolRegistryEntriesData struct {
	Tools []*MCPToolRegistryEntry `json:"tools"`
	Total int64                   `json:"total"`
}

type TestMCPToolCallData struct {
	Status    string `json:"status"`
	Output    string `json:"output"`
	LatencyMs int64  `json:"latency_ms"`
}

type DiscoverMCPToolServerData struct {
	Tools     []*MCPToolDefinition `json:"tools"`
	Resources []*MCPResource       `json:"resources"`
	Prompts   []*MCPPrompt         `json:"prompts"`
}

// ExportMCPToolServerData intentionally omits authentication material. It is a
// portable control-plane representation, not a dump of the persisted record.
type ExportMCPToolServerData struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	ServerType  string               `json:"server_type"`
	Config      string               `json:"config"`
	Tools       []*MCPToolDefinition `json:"tools"`
	Resources   []*MCPResource       `json:"resources"`
	Prompts     []*MCPPrompt         `json:"prompts"`
}

type MCPRuntimeAuditEvent struct {
	EventID      string `json:"event_id"`
	ActorID      int64  `json:"actor_id,string"`
	ToolName     string `json:"tool_name"`
	Status       string `json:"status"`
	LatencyMs    int64  `json:"latency_ms"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorSummary string `json:"error_summary,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	CompletedAt  int64  `json:"completed_at,omitempty"`
}

type ListMCPRuntimeAuditEventsData struct {
	Events     []*MCPRuntimeAuditEvent `json:"events"`
	NextCursor string                  `json:"next_cursor"`
}

type MCPToolServerResponse struct {
	Data *MCPToolServer `json:"data,omitempty"`
	Code int64          `json:"code"`
	Msg  string         `json:"msg"`
}

type ListMCPToolServersResponse struct {
	Data *ListMCPToolServersData `json:"data,omitempty"`
	Code int64                   `json:"code"`
	Msg  string                  `json:"msg"`
}

type ListMCPToolRegistryEntriesResponse struct {
	Data *ListMCPToolRegistryEntriesData `json:"data,omitempty"`
	Code int64                           `json:"code"`
	Msg  string                          `json:"msg"`
}

type TestMCPToolCallResponse struct {
	Data *TestMCPToolCallData `json:"data,omitempty"`
	Code int64                `json:"code"`
	Msg  string               `json:"msg"`
}

type DiscoverMCPToolServerResponse struct {
	Data *DiscoverMCPToolServerData `json:"data,omitempty"`
	Code int64                      `json:"code"`
	Msg  string                     `json:"msg"`
}

type ExportMCPToolServerResponse struct {
	Data *ExportMCPToolServerData `json:"data,omitempty"`
	Code int64                    `json:"code"`
	Msg  string                   `json:"msg"`
}

type ListMCPRuntimeAuditEventsResponse struct {
	Data *ListMCPRuntimeAuditEventsData `json:"data,omitempty"`
	Code int64                          `json:"code"`
	Msg  string                         `json:"msg"`
}
