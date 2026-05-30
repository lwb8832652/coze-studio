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

import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export enum SkillType {
  Script = 1,
  Workflow = 2,
}
export interface Skill {
  id: string,
  space_id: string,
  name: string,
  description: string,
  type: SkillType,
  version: string,
  enabled: boolean,
  input_schema: string,
  output_schema: string,
  executor: string,
  permissions: string,
  created_at: number,
  updated_at: number,
}
export interface UpsertSkillRequest {
  id?: string,
  space_id: string,
  name: string,
  description: string,
  type: SkillType,
  version: string,
  enabled: boolean,
  input_schema: string,
  output_schema: string,
  executor: string,
  permissions: string,
}
export interface UpdateSkillRequest {
  id: string,
  space_id: string,
  name: string,
  description: string,
  type: SkillType,
  version: string,
  enabled: boolean,
  input_schema: string,
  output_schema: string,
  executor: string,
  permissions: string,
}
export interface ImportSkillRequest {
  space_id: string,
  file_name: string,
  content: string,
}
export interface SkillResponse {
  data?: Skill,
  code: number,
  msg: string,
}
export interface ListSkillsRequest {
  space_id: string,
  type?: SkillType,
  enabled?: boolean,
}
export interface ListSkillsData {
  skills: Skill[]
}
export interface ListSkillsResponse {
  data?: ListSkillsData,
  code: number,
  msg: string,
}
export interface GetSkillRequest {
  skill_id: string
}
export interface TestRunSkillRequest {
  skill_id: string,
  input: string,
}
export interface TestRunSkillData {
  output?: string,
  task_id?: string,
}
export interface TestRunSkillResponse {
  data?: TestRunSkillData,
  code: number,
  msg: string,
}
export interface ExportSkillData {
  file_name: string,
  content: string,
}
export interface ExportSkillResponse {
  data?: ExportSkillData,
  code: number,
  msg: string,
}
export const CreateSkill = /*#__PURE__*/createAPI<UpsertSkillRequest, SkillResponse>({
  "url": "/api/workbench/skills",
  "method": "POST",
  "name": "CreateSkill",
  "reqType": "UpsertSkillRequest",
  "reqMapping": {
    "body": ["id", "space_id", "name", "description", "type", "version", "enabled", "input_schema", "output_schema", "executor", "permissions"]
  },
  "resType": "SkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const UpdateSkill = /*#__PURE__*/createAPI<UpdateSkillRequest, SkillResponse>({
  "url": "/api/workbench/skills/:id",
  "method": "PUT",
  "name": "UpdateSkill",
  "reqType": "UpdateSkillRequest",
  "reqMapping": {
    "path": ["id"],
    "body": ["space_id", "name", "description", "type", "version", "enabled", "input_schema", "output_schema", "executor", "permissions"]
  },
  "resType": "SkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const ImportSkill = /*#__PURE__*/createAPI<ImportSkillRequest, SkillResponse>({
  "url": "/api/workbench/skills/import",
  "method": "POST",
  "name": "ImportSkill",
  "reqType": "ImportSkillRequest",
  "reqMapping": {
    "body": ["space_id", "file_name", "content"]
  },
  "resType": "SkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const ListSkills = /*#__PURE__*/createAPI<ListSkillsRequest, ListSkillsResponse>({
  "url": "/api/workbench/skills",
  "method": "GET",
  "name": "ListSkills",
  "reqType": "ListSkillsRequest",
  "reqMapping": {
    "query": ["space_id", "type", "enabled"]
  },
  "resType": "ListSkillsResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const GetSkill = /*#__PURE__*/createAPI<GetSkillRequest, SkillResponse>({
  "url": "/api/workbench/skills/:skill_id",
  "method": "GET",
  "name": "GetSkill",
  "reqType": "GetSkillRequest",
  "reqMapping": {
    "path": ["skill_id"]
  },
  "resType": "SkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const ExportSkill = /*#__PURE__*/createAPI<GetSkillRequest, ExportSkillResponse>({
  "url": "/api/workbench/skills/:skill_id/export",
  "method": "GET",
  "name": "ExportSkill",
  "reqType": "GetSkillRequest",
  "reqMapping": {
    "path": ["skill_id"]
  },
  "resType": "ExportSkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const TestRunSkill = /*#__PURE__*/createAPI<TestRunSkillRequest, TestRunSkillResponse>({
  "url": "/api/workbench/skills/:skill_id/test_run",
  "method": "POST",
  "name": "TestRunSkill",
  "reqType": "TestRunSkillRequest",
  "reqMapping": {
    "path": ["skill_id"],
    "body": ["input"]
  },
  "resType": "TestRunSkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
