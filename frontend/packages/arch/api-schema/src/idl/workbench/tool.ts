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

import { createAPI } from './../../api/config';

export interface MCPToolDefinition {
  name: string;
  description: string;
  input_schema: string;
}

export interface MCPToolServer {
  server_id: string;
  space_id: string;
  name: string;
  description: string;
  server_type: string;
  enabled: boolean;
  config: string;
  auth: string;
  tools: MCPToolDefinition[];
  health_status: string;
  health_checked_at: number;
  health_latency_ms: number;
  health_error: string;
  created_at: number;
  updated_at: number;
}

export interface UpsertMCPToolServerRequest {
  server_id?: string;
  space_id: string;
  name: string;
  description?: string;
  server_type: string;
  enabled: boolean;
  config: string;
  auth?: string;
  tools: MCPToolDefinition[];
}

export interface ListMCPToolServersRequest {
  space_id: string;
}

export interface ListMCPToolRegistryEntriesRequest {
  space_id: string;
}

export interface GetMCPToolServerRequest {
  server_id: string;
}

export interface TestMCPToolCallRequest {
  server_id: string;
  tool_name: string;
  arguments: string;
}

export interface ListMCPToolServersData {
  servers: MCPToolServer[];
  total: number;
}

export interface MCPToolRegistryEntry {
  name: string;
  source: string;
  category: string;
  visibility: string;
  server_id: string;
  server_name: string;
  tool_name: string;
  description: string;
  input_schema: string;
  enabled: boolean;
  health_status: string;
  health_checked_at: number;
  health_latency_ms: number;
  health_error: string;
}

export interface ListMCPToolRegistryEntriesData {
  tools: MCPToolRegistryEntry[];
  total: number;
}

export interface TestMCPToolCallData {
  status: string;
  output: string;
  latency_ms: number;
}

export interface MCPToolServerResponse {
  data?: MCPToolServer;
  code: number;
  msg: string;
}

export interface ListMCPToolServersResponse {
  data?: ListMCPToolServersData;
  code: number;
  msg: string;
}

export interface ListMCPToolRegistryEntriesResponse {
  data?: ListMCPToolRegistryEntriesData;
  code: number;
  msg: string;
}

export interface TestMCPToolCallResponse {
  data?: TestMCPToolCallData;
  code: number;
  msg: string;
}

export const ListMCPToolServers = /*#__PURE__*/createAPI<
  ListMCPToolServersRequest,
  ListMCPToolServersResponse
>({
  url: '/api/workbench/mcp_tools',
  method: 'GET',
  name: 'ListMCPToolServers',
  reqType: 'ListMCPToolServersRequest',
  reqMapping: {
    query: ['space_id'],
  },
  resType: 'ListMCPToolServersResponse',
  schemaRoot: 'api://schemas/idl_workbench_tool',
  service: 'workbenchTool',
});

export const UpsertMCPToolServer = /*#__PURE__*/createAPI<
  UpsertMCPToolServerRequest,
  MCPToolServerResponse
>({
  url: '/api/workbench/mcp_tools',
  method: 'POST',
  name: 'UpsertMCPToolServer',
  reqType: 'UpsertMCPToolServerRequest',
  reqMapping: {
    body: [
      'server_id',
      'space_id',
      'name',
      'description',
      'server_type',
      'enabled',
      'config',
      'auth',
      'tools',
    ],
  },
  resType: 'MCPToolServerResponse',
  schemaRoot: 'api://schemas/idl_workbench_tool',
  service: 'workbenchTool',
});

export const ListMCPToolRegistryEntries = /*#__PURE__*/createAPI<
  ListMCPToolRegistryEntriesRequest,
  ListMCPToolRegistryEntriesResponse
>({
  url: '/api/workbench/mcp_tools/registry_entries',
  method: 'GET',
  name: 'ListMCPToolRegistryEntries',
  reqType: 'ListMCPToolRegistryEntriesRequest',
  reqMapping: {
    query: ['space_id'],
  },
  resType: 'ListMCPToolRegistryEntriesResponse',
  schemaRoot: 'api://schemas/idl_workbench_tool',
  service: 'workbenchTool',
});

export const GetMCPToolServer = /*#__PURE__*/createAPI<
  GetMCPToolServerRequest,
  MCPToolServerResponse
>({
  url: '/api/workbench/mcp_tools/:server_id',
  method: 'GET',
  name: 'GetMCPToolServer',
  reqType: 'GetMCPToolServerRequest',
  reqMapping: {
    path: ['server_id'],
  },
  resType: 'MCPToolServerResponse',
  schemaRoot: 'api://schemas/idl_workbench_tool',
  service: 'workbenchTool',
});

export const DeleteMCPToolServer = /*#__PURE__*/createAPI<
  GetMCPToolServerRequest,
  MCPToolServerResponse
>({
  url: '/api/workbench/mcp_tools/:server_id',
  method: 'DELETE',
  name: 'DeleteMCPToolServer',
  reqType: 'GetMCPToolServerRequest',
  reqMapping: {
    path: ['server_id'],
  },
  resType: 'MCPToolServerResponse',
  schemaRoot: 'api://schemas/idl_workbench_tool',
  service: 'workbenchTool',
});

export const TestMCPToolCall = /*#__PURE__*/createAPI<
  TestMCPToolCallRequest,
  TestMCPToolCallResponse
>({
  url: '/api/workbench/mcp_tools/:server_id/test_call',
  method: 'POST',
  name: 'TestMCPToolCall',
  reqType: 'TestMCPToolCallRequest',
  reqMapping: {
    path: ['server_id'],
    body: ['tool_name', 'arguments'],
  },
  resType: 'TestMCPToolCallResponse',
  schemaRoot: 'api://schemas/idl_workbench_tool',
  service: 'workbenchTool',
});
