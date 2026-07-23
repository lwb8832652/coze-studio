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
export enum WorkspaceModelScope {
  System = 1,
  Space = 2,
}
export interface WorkspaceModelProviderOption {
  key: string,
  name: string,
  model_class: number,
  icon_url?: string,
}
export interface WorkspaceModelEndpointInput {
  id?: string,
  base_url: string,
  api_key?: string,
  weight?: number,
  enabled?: boolean,
}
export interface WorkspaceModelEndpointView {
  id: string,
  base_url: string,
  credential_configured: boolean,
  weight: number,
  enabled: boolean,
}
export interface WorkspaceModel {
  id: string,
  scope: WorkspaceModelScope,
  provider_key: string,
  model_class: number,
  display_name: string,
  model_identifier: string,
  enabled: boolean,
  credential_configured: boolean,
  can_manage: boolean,
  description?: string,
  protocol?: string,
  capabilities?: string[],
  usage_scenarios?: string[],
  max_context_tokens?: number,
  max_output_tokens?: number,
  function_call_mode?: string,
  endpoints?: WorkspaceModelEndpointView[],
  creator_id?: string,
  updated_at?: number,
}
export interface WorkspaceModelDraft {
  provider_key: string,
  model_class: number,
  display_name: string,
  model_identifier: string,
  description?: string,
  protocol: string,
  endpoints: WorkspaceModelEndpointInput[],
  capabilities?: string[],
  usage_scenarios?: string[],
  max_context_tokens?: number,
  max_output_tokens?: number,
  function_call_mode?: string,
  enabled?: boolean,
}
export interface ListWorkspaceModelsRequest {
  space_id: string,
  scope?: WorkspaceModelScope,
  keyword?: string,
}
export interface ListWorkspaceModelsData {
  system_models: WorkspaceModel[],
  workspace_models: WorkspaceModel[],
  providers: WorkspaceModelProviderOption[],
  can_manage: boolean,
}
export interface ListWorkspaceModelsResponse {
  data?: ListWorkspaceModelsData,
  code: number,
  msg: string,
}
export interface GetWorkspaceModelRequest {
  model_id: string,
  space_id: string,
}
export interface WorkspaceModelResponse {
  data?: WorkspaceModel,
  code: number,
  msg: string,
}
export interface UpsertWorkspaceModelRequest {
  space_id: string,
  model_id?: string,
  model: WorkspaceModelDraft,
}
export interface TestWorkspaceModelRequest {
  space_id: string,
  model_id?: string,
  model: WorkspaceModelDraft,
}
export interface TestWorkspaceModelData {
  success: boolean,
  duration_ms: number,
  error_code?: string,
  message?: string,
}
export interface TestWorkspaceModelResponse {
  data?: TestWorkspaceModelData,
  code: number,
  msg: string,
}
export interface SetWorkspaceModelStatusRequest {
  model_id: string,
  space_id: string,
  enabled: boolean,
}
export interface DeleteWorkspaceModelRequest {
  model_id: string,
  space_id: string,
}
export interface WorkspaceModelMutationResponse {
  code: number,
  msg: string,
}
export const ListWorkspaceModels = /*#__PURE__*/createAPI<ListWorkspaceModelsRequest, ListWorkspaceModelsResponse>({
  "url": "/api/workbench/models",
  "method": "GET",
  "name": "ListWorkspaceModels",
  "reqType": "ListWorkspaceModelsRequest",
  "reqMapping": {
    "query": ["space_id", "scope", "keyword"]
  },
  "resType": "ListWorkspaceModelsResponse",
  "schemaRoot": "api://schemas/idl_workbench_model",
  "service": "workbenchModel"
});
export const GetWorkspaceModel = /*#__PURE__*/createAPI<GetWorkspaceModelRequest, WorkspaceModelResponse>({
  "url": "/api/workbench/models/:model_id",
  "method": "GET",
  "name": "GetWorkspaceModel",
  "reqType": "GetWorkspaceModelRequest",
  "reqMapping": {
    "path": ["model_id"],
    "query": ["space_id"]
  },
  "resType": "WorkspaceModelResponse",
  "schemaRoot": "api://schemas/idl_workbench_model",
  "service": "workbenchModel"
});
export const UpsertWorkspaceModel = /*#__PURE__*/createAPI<UpsertWorkspaceModelRequest, WorkspaceModelResponse>({
  "url": "/api/workbench/models",
  "method": "POST",
  "name": "UpsertWorkspaceModel",
  "reqType": "UpsertWorkspaceModelRequest",
  "reqMapping": {
    "body": ["space_id", "model_id", "model"]
  },
  "resType": "WorkspaceModelResponse",
  "schemaRoot": "api://schemas/idl_workbench_model",
  "service": "workbenchModel"
});
export const TestWorkspaceModel = /*#__PURE__*/createAPI<TestWorkspaceModelRequest, TestWorkspaceModelResponse>({
  "url": "/api/workbench/models/test",
  "method": "POST",
  "name": "TestWorkspaceModel",
  "reqType": "TestWorkspaceModelRequest",
  "reqMapping": {
    "body": ["space_id", "model_id", "model"]
  },
  "resType": "TestWorkspaceModelResponse",
  "schemaRoot": "api://schemas/idl_workbench_model",
  "service": "workbenchModel"
});
export const SetWorkspaceModelStatus = /*#__PURE__*/createAPI<SetWorkspaceModelStatusRequest, WorkspaceModelMutationResponse>({
  "url": "/api/workbench/models/:model_id/status",
  "method": "POST",
  "name": "SetWorkspaceModelStatus",
  "reqType": "SetWorkspaceModelStatusRequest",
  "reqMapping": {
    "path": ["model_id"],
    "body": ["space_id", "enabled"]
  },
  "resType": "WorkspaceModelMutationResponse",
  "schemaRoot": "api://schemas/idl_workbench_model",
  "service": "workbenchModel"
});
export const DeleteWorkspaceModel = /*#__PURE__*/createAPI<DeleteWorkspaceModelRequest, WorkspaceModelMutationResponse>({
  "url": "/api/workbench/models/:model_id",
  "method": "DELETE",
  "name": "DeleteWorkspaceModel",
  "reqType": "DeleteWorkspaceModelRequest",
  "reqMapping": {
    "path": ["model_id"],
    "body": ["space_id"]
  },
  "resType": "WorkspaceModelMutationResponse",
  "schemaRoot": "api://schemas/idl_workbench_model",
  "service": "workbenchModel"
});