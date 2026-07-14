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
  DeerSkill = 3,
  PublicSkill = 4,
  CustomSkill = 5,
}
export enum SkillResourceOperation {
  Upsert = 1,
  Delete = 2,
  Move = 3,
  CreateDirectory = 4,
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
  icon_uri?: string,
  usage_scenarios?: string,
  development_thread_id?: string,
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
  icon_uri?: string,
  usage_scenarios?: string,
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
  icon_uri?: string,
  usage_scenarios?: string,
  expected_version_id: string,
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
export interface SkillVersion {
  id: string,
  skill_id: string,
  version: string,
  skill_md: string,
  input_schema: string,
  output_schema: string,
  executor: string,
  permissions: string,
  created_at: number,
}
export interface ListSkillVersionsRequest {
  skill_id: string
}
export interface ListSkillVersionsData {
  versions: SkillVersion[]
}
export interface ListSkillVersionsResponse {
  data?: ListSkillVersionsData,
  code: number,
  msg: string,
}
export interface SkillVersionResponse {
  data?: SkillVersion,
  code: number,
  msg: string,
}
export interface SkillResource {
  id: string,
  skill_id: string,
  version_id: string,
  path: string,
  content_base64: string,
  size: number,
  sha256: string,
  created_at: number,
}
export interface ListSkillVersionResourcesRequest {
  skill_id: string,
  version_id: string,
}
export interface ListSkillVersionResourcesData {
  resources: SkillResource[]
}
export interface ListSkillVersionResourcesResponse {
  data?: ListSkillVersionResourcesData,
  code: number,
  msg: string,
}
export interface UpdateSkillVersionResourceRequest {
  skill_id: string,
  version_id: string,
  path: string,
  content_base64: string,
  operation?: SkillResourceOperation,
  target_path?: string,
}
export interface UpdateSkillVersionContentRequest {
  skill_id: string,
  version_id: string,
  skill_md: string,
}
export interface ExportSkillVersionRequest {
  skill_id: string,
  version_id: string,
}
export interface ExportSkillVersionData {
  file_name: string,
  content_base64: string,
  content_type: string,
}
export interface ExportSkillVersionResponse {
  data?: ExportSkillVersionData,
  code: number,
  msg: string,
}
export interface RollbackSkillVersionRequest {
  skill_id: string,
  version_id: string,
  expected_version_id: string,
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
export interface ListSkillToolCandidatesRequest {
  space_id: string
}
export interface SkillToolCandidate {
  name: string,
  display_name: string,
  description: string,
  category: string,
  visibility: string,
  source?: string,
  source_id?: string,
  source_name?: string,
}
export interface ListSkillToolCandidatesData {
  tools: SkillToolCandidate[]
}
export interface ListSkillToolCandidatesResponse {
  data?: ListSkillToolCandidatesData,
  code: number,
  msg: string,
}
export interface InstallSkillFromArtifactRequest {
  space_id: string,
  thread_id: string,
  artifact_id: string,
}
export interface InstallSkillFromArtifactData {
  success: boolean,
  skill_name: string,
  message: string,
  skill?: Skill,
}
export interface InstallSkillFromArtifactResponse {
  data?: InstallSkillFromArtifactData,
  code: number,
  msg: string,
}
export const CreateSkill = /*#__PURE__*/createAPI<UpsertSkillRequest, SkillResponse>({
  "url": "/api/workbench/skills",
  "method": "POST",
  "name": "CreateSkill",
  "reqType": "UpsertSkillRequest",
  "reqMapping": {
    "body": ["id", "space_id", "name", "description", "type", "version", "enabled", "input_schema", "output_schema", "executor", "permissions", "icon_uri", "usage_scenarios"]
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
    "body": ["space_id", "name", "description", "type", "version", "enabled", "input_schema", "output_schema", "executor", "permissions", "icon_uri", "usage_scenarios", "expected_version_id"]
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
export const InstallSkillFromArtifact = /*#__PURE__*/createAPI<InstallSkillFromArtifactRequest, InstallSkillFromArtifactResponse>({
  "url": "/api/workbench/skills/install",
  "method": "POST",
  "name": "InstallSkillFromArtifact",
  "reqType": "InstallSkillFromArtifactRequest",
  "reqMapping": {
    "body": ["space_id", "thread_id", "artifact_id"]
  },
  "resType": "InstallSkillFromArtifactResponse",
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
export const ListSkillToolCandidates = /*#__PURE__*/createAPI<ListSkillToolCandidatesRequest, ListSkillToolCandidatesResponse>({
  "url": "/api/workbench/skills/tool_candidates",
  "method": "GET",
  "name": "ListSkillToolCandidates",
  "reqType": "ListSkillToolCandidatesRequest",
  "reqMapping": {
    "query": ["space_id"]
  },
  "resType": "ListSkillToolCandidatesResponse",
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
export const DeleteSkill = /*#__PURE__*/createAPI<GetSkillRequest, SkillResponse>({
  "url": "/api/workbench/skills/:skill_id",
  "method": "DELETE",
  "name": "DeleteSkill",
  "reqType": "GetSkillRequest",
  "reqMapping": {
    "path": ["skill_id"]
  },
  "resType": "SkillResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const ListSkillVersions = /*#__PURE__*/createAPI<ListSkillVersionsRequest, ListSkillVersionsResponse>({
  "url": "/api/workbench/skills/:skill_id/versions",
  "method": "GET",
  "name": "ListSkillVersions",
  "reqType": "ListSkillVersionsRequest",
  "reqMapping": {
    "path": ["skill_id"]
  },
  "resType": "ListSkillVersionsResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const ListSkillVersionResources = /*#__PURE__*/createAPI<ListSkillVersionResourcesRequest, ListSkillVersionResourcesResponse>({
  "url": "/api/workbench/skills/:skill_id/versions/:version_id/resources",
  "method": "GET",
  "name": "ListSkillVersionResources",
  "reqType": "ListSkillVersionResourcesRequest",
  "reqMapping": {
    "path": ["skill_id", "version_id"],
    "body": ["expected_version_id"]
  },
  "resType": "ListSkillVersionResourcesResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const UpdateSkillVersionResource = /*#__PURE__*/createAPI<UpdateSkillVersionResourceRequest, SkillVersionResponse>({
  "url": "/api/workbench/skills/:skill_id/versions/:version_id/resources",
  "method": "PUT",
  "name": "UpdateSkillVersionResource",
  "reqType": "UpdateSkillVersionResourceRequest",
  "reqMapping": {
    "path": ["skill_id", "version_id"],
    "body": ["path", "content_base64", "operation", "target_path"]
  },
  "resType": "SkillVersionResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const UpdateSkillVersionContent = /*#__PURE__*/createAPI<UpdateSkillVersionContentRequest, SkillVersionResponse>({
  "url": "/api/workbench/skills/:skill_id/versions/:version_id/content",
  "method": "PUT",
  "name": "UpdateSkillVersionContent",
  "reqType": "UpdateSkillVersionContentRequest",
  "reqMapping": {
    "path": ["skill_id", "version_id"],
    "body": ["skill_md"]
  },
  "resType": "SkillVersionResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const ExportSkillVersion = /*#__PURE__*/createAPI<ExportSkillVersionRequest, ExportSkillVersionResponse>({
  "url": "/api/workbench/skills/:skill_id/versions/:version_id/export",
  "method": "GET",
  "name": "ExportSkillVersion",
  "reqType": "ExportSkillVersionRequest",
  "reqMapping": {
    "path": ["skill_id", "version_id"]
  },
  "resType": "ExportSkillVersionResponse",
  "schemaRoot": "api://schemas/idl_workbench_skill",
  "service": "workbenchSkill"
});
export const RollbackSkillVersion = /*#__PURE__*/createAPI<RollbackSkillVersionRequest, SkillResponse>({
  "url": "/api/workbench/skills/:skill_id/versions/:version_id/rollback",
  "method": "POST",
  "name": "RollbackSkillVersion",
  "reqType": "RollbackSkillVersionRequest",
  "reqMapping": {
    "path": ["skill_id", "version_id"]
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
