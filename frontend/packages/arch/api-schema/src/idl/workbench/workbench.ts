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

import * as task from './task';
export { task };
import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export enum ChatMode {
  Auto = 1,
  Ask = 2,
  Agent = 3,
}
export enum RouteTarget {
  ChatDirect = 1,
  AgentEngine = 2,
  SkillEngine = 3,
  TaskEngine = 4,
}
export interface WorkbenchChatRequest {
  space_id: string,
  conversation_id?: string,
  message: string,
  mode: ChatMode,
  selected_skill_id?: string,
  task_id?: string,
  enable_skills?: string[],
  enable_mcp?: string[],
  enable_kbs?: string[],
  enable_databases?: string[],
  model_type?: string,
  model_name?: string,
  runtime_settings?: string,
}
export interface WorkbenchChatData {
  route_target: RouteTarget,
  answer?: string,
  task?: task.ChatTask,
  conversation_id?: string,
  reason?: string,
  result_type?: string,
  execution_type?: string,
}
export interface WorkbenchChatResponse {
  data?: WorkbenchChatData,
  code: number,
  msg: string,
}
export const WorkbenchChat = /*#__PURE__*/createAPI<WorkbenchChatRequest, WorkbenchChatResponse>({
  "url": "/api/workbench/chat",
  "method": "POST",
  "name": "WorkbenchChat",
  "reqType": "WorkbenchChatRequest",
  "reqMapping": {
    "body": [
      "space_id",
      "conversation_id",
      "message",
      "mode",
      "selected_skill_id",
      "task_id",
      "enable_skills",
      "enable_mcp",
      "enable_kbs",
      "enable_databases",
      "model_type",
      "model_name",
      "runtime_settings"
    ]
  },
  "resType": "WorkbenchChatResponse",
  "schemaRoot": "api://schemas/idl_workbench_workbench",
  "service": "workbench"
});
