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

type MCPToolServer struct {
	ServerID    int64                `json:"server_id,string"`
	SpaceID     int64                `json:"space_id,string"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	ServerType  string               `json:"server_type"`
	Enabled     bool                 `json:"enabled"`
	Config      string               `json:"config"`
	Auth        string               `json:"auth"`
	Tools       []*MCPToolDefinition `json:"tools"`
	CreatedAt   int64                `json:"created_at"`
	UpdatedAt   int64                `json:"updated_at"`
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

type GetMCPToolServerRequest struct {
	ServerID int64 `path:"server_id,required"`
}

type TestMCPToolCallRequest struct {
	ServerID  int64  `path:"server_id,required" json:"-"`
	ToolName  string `json:"tool_name,required"`
	Arguments string `json:"arguments,required"`
}

type ListMCPToolServersData struct {
	Servers []*MCPToolServer `json:"servers"`
	Total   int64            `json:"total"`
}

type TestMCPToolCallData struct {
	Status    string `json:"status"`
	Output    string `json:"output"`
	LatencyMs int64  `json:"latency_ms"`
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

type TestMCPToolCallResponse struct {
	Data *TestMCPToolCallData `json:"data,omitempty"`
	Code int64                `json:"code"`
	Msg  string               `json:"msg"`
}
