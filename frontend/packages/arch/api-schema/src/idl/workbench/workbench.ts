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

import * as thread from './thread';
export { thread };
import * as task from './task';
export { task };
import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export interface GetWorkbenchRuntimeDoctorRequest {
  space_id: string
}
export interface RuntimeDoctorCheck {
  name: string,
  category: string,
  status: string,
  message?: string,
}
export interface RuntimeDoctorRuntimeData {
  default_mode: string,
  eino_adk_enabled: boolean,
}
export interface RuntimeDoctorWebToolStatus {
  status: string,
  configured: boolean,
  message?: string,
}
export interface RuntimeDoctorWebToolsData {
  web_fetch: RuntimeDoctorWebToolStatus,
  web_search: RuntimeDoctorWebToolStatus,
}
export interface RuntimeDoctorMCPToolsData {
  status: string,
  total_servers: number,
  enabled_servers: number,
  healthy_servers: number,
  unhealthy_servers: number,
  unknown_servers: number,
}
export interface RuntimeDoctorModelCapabilities {
  native_tool_search: boolean,
  thinking: boolean,
  reasoning: boolean,
  vision: boolean,
  pdf: boolean,
  file: boolean,
  audio: boolean,
  video: boolean,
}
export interface RuntimeDoctorModelData {
  status: string,
  configured: boolean,
  live_probe: string,
  capabilities?: RuntimeDoctorModelCapabilities,
  message?: string,
}
export interface RuntimeDoctorSandboxScopeData {
  scope: string,
  configured: boolean,
  available: boolean,
  selected: boolean,
  health_status: string,
  reason_code: string,
  provider_type?: string,
  provider_ref?: string,
  checked_at?: string,
}
export interface RuntimeDoctorSandboxData {
  status: string,
  runner_type: string,
  network: string,
  process: string,
  ffi: string,
  node_modules: string,
  message?: string,
  scopes?: RuntimeDoctorSandboxScopeData[],
}
export interface WorkbenchRuntimeDoctorData {
  status: string,
  runtime: RuntimeDoctorRuntimeData,
  web_tools: RuntimeDoctorWebToolsData,
  mcp_tools: RuntimeDoctorMCPToolsData,
  checks: RuntimeDoctorCheck[],
  model: RuntimeDoctorModelData,
  sandbox: RuntimeDoctorSandboxData,
}
export interface WorkbenchRuntimeDoctorResponse {
  data?: WorkbenchRuntimeDoctorData,
  code: number,
  msg: string,
}
export const GetWorkbenchRuntimeDoctor = /*#__PURE__*/createAPI<GetWorkbenchRuntimeDoctorRequest, WorkbenchRuntimeDoctorResponse>({
  "url": "/api/workbench/runtime_doctor",
  "method": "GET",
  "name": "GetWorkbenchRuntimeDoctor",
  "reqType": "GetWorkbenchRuntimeDoctorRequest",
  "reqMapping": {
    "query": ["space_id"]
  },
  "resType": "WorkbenchRuntimeDoctorResponse",
  "schemaRoot": "api://schemas/idl_workbench_workbench",
  "service": "workbench"
});
