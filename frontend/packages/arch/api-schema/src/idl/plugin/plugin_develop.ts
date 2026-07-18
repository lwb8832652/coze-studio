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

import * as plugin_develop_common from './plugin_develop_common';
export { plugin_develop_common };
import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export const GetOAuthSchema = /*#__PURE__*/createAPI<GetOAuthSchemaRequest, GetOAuthSchemaResponse>({
  "url": "/api/plugin/get_oauth_schema",
  "method": "POST",
  "name": "GetOAuthSchema",
  "reqType": "GetOAuthSchemaRequest",
  "reqMapping": {},
  "resType": "GetOAuthSchemaResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetOAuthSchemaAPI = /*#__PURE__*/createAPI<GetOAuthSchemaRequest, GetOAuthSchemaResponse>({
  "url": "/api/plugin_api/get_oauth_schema",
  "method": "POST",
  "name": "GetOAuthSchemaAPI",
  "reqType": "GetOAuthSchemaRequest",
  "reqMapping": {},
  "resType": "GetOAuthSchemaResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Get a list of published workflows, plugins, or details of multiple plugins */
export const GetPlaygroundPluginList = /*#__PURE__*/createAPI<GetPlaygroundPluginListRequest, GetPlaygroundPluginListResponse>({
  "url": "/api/plugin_api/get_playground_plugin_list",
  "method": "POST",
  "name": "GetPlaygroundPluginList",
  "reqType": "GetPlaygroundPluginListRequest",
  "reqMapping": {
    "body": ["page", "size", "name", "space_id", "plugin_ids", "plugin_types", "channel_id", "self_created", "order_by", "is_get_offline"],
    "header": ["Referer"]
  },
  "resType": "GetPlaygroundPluginListResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Creating plugins with code */
export const RegisterPlugin = /*#__PURE__*/createAPI<RegisterPluginRequest, RegisterPluginResponse>({
  "url": "/api/plugin_api/register",
  "method": "POST",
  "name": "RegisterPlugin",
  "reqType": "RegisterPluginRequest",
  "reqMapping": {
    "body": ["ai_plugin", "openapi", "client_id", "client_secret", "service_token", "plugin_type", "space_id", "import_from_file", "project_id"]
  },
  "resType": "RegisterPluginResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Create plugins through UI */
export const RegisterPluginMeta = /*#__PURE__*/createAPI<RegisterPluginMetaRequest, RegisterPluginMetaResponse>({
  "url": "/api/plugin_api/register_plugin_meta",
  "method": "POST",
  "name": "RegisterPluginMeta",
  "reqType": "RegisterPluginMetaRequest",
  "reqMapping": {
    "body": ["name", "desc", "url", "icon", "auth_type", "location", "key", "service_token", "oauth_info", "space_id", "common_params", "creation_method", "ide_code_runtime", "plugin_type", "project_id", "sub_auth_type", "auth_payload", "fixed_export_ip"]
  },
  "resType": "RegisterPluginMetaResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Get a list of plug-in tools, or multiple tool details */
export const GetPluginAPIs = /*#__PURE__*/createAPI<GetPluginAPIsRequest, GetPluginAPIsResponse>({
  "url": "/api/plugin_api/get_plugin_apis",
  "method": "POST",
  "name": "GetPluginAPIs",
  "reqType": "GetPluginAPIsRequest",
  "reqMapping": {
    "body": ["plugin_id", "api_ids", "page", "size", "order", "preview_version_ts"]
  },
  "resType": "GetPluginAPIsResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Get plugin details */
export const GetPluginInfo = /*#__PURE__*/createAPI<GetPluginInfoRequest, GetPluginInfoResponse>({
  "url": "/api/plugin_api/get_plugin_info",
  "method": "POST",
  "name": "GetPluginInfo",
  "reqType": "GetPluginInfoRequest",
  "reqMapping": {
    "body": ["plugin_id", "preview_version_tsx"]
  },
  "resType": "GetPluginInfoResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Updated list of tools compared to the most recent release */
export const GetUpdatedAPIs = /*#__PURE__*/createAPI<GetUpdatedAPIsRequest, GetUpdatedAPIsResponse>({
  "url": "/api/plugin_api/get_updated_apis",
  "method": "POST",
  "name": "GetUpdatedAPIs",
  "reqType": "GetUpdatedAPIsRequest",
  "reqMapping": {
    "body": ["plugin_id"]
  },
  "resType": "GetUpdatedAPIsResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetOAuthStatus = /*#__PURE__*/createAPI<GetOAuthStatusRequest, GetOAuthStatusResponse>({
  "url": "/api/plugin_api/get_oauth_status",
  "method": "POST",
  "name": "GetOAuthStatus",
  "reqType": "GetOAuthStatusRequest",
  "reqMapping": {
    "body": ["plugin_id"]
  },
  "resType": "GetOAuthStatusResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const CheckAndLockPluginEdit = /*#__PURE__*/createAPI<CheckAndLockPluginEditRequest, CheckAndLockPluginEditResponse>({
  "url": "/api/plugin_api/check_and_lock_plugin_edit",
  "method": "POST",
  "name": "CheckAndLockPluginEdit",
  "reqType": "CheckAndLockPluginEditRequest",
  "reqMapping": {
    "body": ["plugin_id"]
  },
  "resType": "CheckAndLockPluginEditResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const UnlockPluginEdit = /*#__PURE__*/createAPI<UnlockPluginEditRequest, UnlockPluginEditResponse>({
  "url": "/api/plugin_api/unlock_plugin_edit",
  "method": "POST",
  "name": "UnlockPluginEdit",
  "reqType": "UnlockPluginEditRequest",
  "reqMapping": {
    "body": ["plugin_id"]
  },
  "resType": "UnlockPluginEditResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Update plugins via code */
export const UpdatePlugin = /*#__PURE__*/createAPI<UpdatePluginRequest, UpdatePluginResponse>({
  "url": "/api/plugin_api/update",
  "method": "POST",
  "name": "UpdatePlugin",
  "reqType": "UpdatePluginRequest",
  "reqMapping": {
    "body": ["plugin_id", "ai_plugin", "openapi", "client_id", "client_secret", "service_token", "source_code", "edit_version"]
  },
  "resType": "UpdatePluginResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** removal tool */
export const DeleteAPI = /*#__PURE__*/createAPI<DeleteAPIRequest, DeleteAPIResponse>({
  "url": "/api/plugin_api/delete_api",
  "method": "POST",
  "name": "DeleteAPI",
  "reqType": "DeleteAPIRequest",
  "reqMapping": {
    "body": ["plugin_id", "api_id", "edit_version"]
  },
  "resType": "DeleteAPIResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Remove plugin */
export const DelPlugin = /*#__PURE__*/createAPI<DelPluginRequest, DelPluginResponse>({
  "url": "/api/plugin_api/del_plugin",
  "method": "POST",
  "name": "DelPlugin",
  "reqType": "DelPluginRequest",
  "reqMapping": {
    "body": ["plugin_id"]
  },
  "resType": "DelPluginResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** publishing plugin */
export const PublishPlugin = /*#__PURE__*/createAPI<PublishPluginRequest, PublishPluginResponse>({
  "url": "/api/plugin_api/publish_plugin",
  "method": "POST",
  "name": "PublishPlugin",
  "reqType": "PublishPluginRequest",
  "reqMapping": {
    "body": ["plugin_id", "privacy_status", "privacy_info", "version_name", "version_desc"]
  },
  "resType": "PublishPluginResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Update plugins via UI */
export const UpdatePluginMeta = /*#__PURE__*/createAPI<UpdatePluginMetaRequest, UpdatePluginMetaResponse>({
  "url": "/api/plugin_api/update_plugin_meta",
  "method": "POST",
  "name": "UpdatePluginMeta",
  "reqType": "UpdatePluginMetaRequest",
  "reqMapping": {
    "body": ["plugin_id", "name", "desc", "url", "icon", "auth_type", "location", "key", "service_token", "oauth_info", "common_params", "creation_method", "edit_version", "plugin_type", "sub_auth_type", "auth_payload", "fixed_export_ip"]
  },
  "resType": "UpdatePluginMetaResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetBotDefaultParams = /*#__PURE__*/createAPI<GetBotDefaultParamsRequest, GetBotDefaultParamsResponse>({
  "url": "/api/plugin_api/get_bot_default_params",
  "method": "POST",
  "name": "GetBotDefaultParams",
  "reqType": "GetBotDefaultParamsRequest",
  "reqMapping": {
    "body": ["space_id", "bot_id", "dev_id", "plugin_id", "api_name", "plugin_referrer_id", "plugin_referrer_scene", "plugin_is_debug", "workflow_id", "plugin_publish_version_ts"]
  },
  "resType": "GetBotDefaultParamsResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const UpdateBotDefaultParams = /*#__PURE__*/createAPI<UpdateBotDefaultParamsRequest, UpdateBotDefaultParamsResponse>({
  "url": "/api/plugin_api/update_bot_default_params",
  "method": "POST",
  "name": "UpdateBotDefaultParams",
  "reqType": "UpdateBotDefaultParamsRequest",
  "reqMapping": {
    "body": ["space_id", "bot_id", "dev_id", "plugin_id", "api_name", "request_params", "response_params", "plugin_referrer_id", "plugin_referrer_scene", "response_style", "workflow_id"]
  },
  "resType": "UpdateBotDefaultParamsResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** creation tool */
export const CreateAPI = /*#__PURE__*/createAPI<CreateAPIRequest, CreateAPIResponse>({
  "url": "/api/plugin_api/create_api",
  "method": "POST",
  "name": "CreateAPI",
  "reqType": "CreateAPIRequest",
  "reqMapping": {
    "body": ["plugin_id", "name", "desc", "path", "method", "api_extend", "request_params", "response_params", "disabled", "edit_version", "function_name"]
  },
  "resType": "CreateAPIResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** update tool */
export const UpdateAPI = /*#__PURE__*/createAPI<UpdateAPIRequest, UpdateAPIResponse>({
  "url": "/api/plugin_api/update_api",
  "method": "POST",
  "name": "UpdateAPI",
  "reqType": "UpdateAPIRequest",
  "reqMapping": {
    "body": ["plugin_id", "api_id", "name", "desc", "path", "method", "request_params", "response_params", "disabled", "api_extend", "edit_version", "save_example", "debug_example", "function_name"]
  },
  "resType": "UpdateAPIResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetUserAuthority = /*#__PURE__*/createAPI<GetUserAuthorityRequest, GetUserAuthorityResponse>({
  "url": "/api/plugin_api/get_user_authority",
  "method": "POST",
  "name": "GetUserAuthority",
  "reqType": "GetUserAuthorityRequest",
  "reqMapping": {
    "body": ["plugin_id", "creation_method", "project_id"]
  },
  "resType": "GetUserAuthorityResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const DebugAPI = /*#__PURE__*/createAPI<DebugAPIRequest, DebugAPIResponse>({
  "url": "/api/plugin_api/debug_api",
  "method": "POST",
  "name": "DebugAPI",
  "reqType": "DebugAPIRequest",
  "reqMapping": {
    "body": ["plugin_id", "api_id", "parameters", "operation", "edit_version"]
  },
  "resType": "DebugAPIResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetPluginNextVersion = /*#__PURE__*/createAPI<GetPluginNextVersionRequest, GetPluginNextVersionResponse>({
  "url": "/api/plugin_api/get_plugin_next_version",
  "method": "POST",
  "name": "GetPluginNextVersion",
  "reqType": "GetPluginNextVersionRequest",
  "reqMapping": {
    "body": ["plugin_id", "space_id"]
  },
  "resType": "GetPluginNextVersionResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetDevPluginList = /*#__PURE__*/createAPI<GetDevPluginListRequest, GetDevPluginListResponse>({
  "url": "/api/plugin_api/get_dev_plugin_list",
  "method": "POST",
  "name": "GetDevPluginList",
  "reqType": "GetDevPluginListRequest",
  "reqMapping": {
    "body": ["status", "page", "size", "dev_id", "space_id", "scope_type", "order_by", "publish_status", "name", "plugin_type_for_filter", "project_id", "plugin_ids"]
  },
  "resType": "GetDevPluginListResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Protocol conversion, such as converting curl and mail carrier collection protocols to openapi3 protocols */
export const Convert2OpenAPI = /*#__PURE__*/createAPI<Convert2OpenAPIRequest, Convert2OpenAPIResponse>({
  "url": "/api/plugin_api/convert_to_openapi",
  "method": "POST",
  "name": "Convert2OpenAPI",
  "reqType": "Convert2OpenAPIRequest",
  "reqMapping": {
    "body": ["plugin_name", "plugin_url", "data", "merge_same_paths", "space_id", "plugin_description"]
  },
  "resType": "Convert2OpenAPIResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
/** Batch creation tool, currently used with the Convert2 OpenAPI interface */
export const BatchCreateAPI = /*#__PURE__*/createAPI<BatchCreateAPIRequest, BatchCreateAPIResponse>({
  "url": "/api/plugin_api/batch_create_api",
  "method": "POST",
  "name": "BatchCreateAPI",
  "reqType": "BatchCreateAPIRequest",
  "reqMapping": {
    "body": ["plugin_id", "ai_plugin", "openapi", "space_id", "dev_id", "replace_same_paths", "paths_to_replace", "edit_version"]
  },
  "resType": "BatchCreateAPIResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const RevokeAuthToken = /*#__PURE__*/createAPI<RevokeAuthTokenRequest, RevokeAuthTokenResponse>({
  "url": "/api/plugin_api/revoke_auth_token",
  "method": "POST",
  "name": "RevokeAuthToken",
  "reqType": "RevokeAuthTokenRequest",
  "reqMapping": {
    "body": ["plugin_id", "bot_id", "context_type"]
  },
  "resType": "RevokeAuthTokenResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetQueriedOAuthPluginList = /*#__PURE__*/createAPI<GetQueriedOAuthPluginListRequest, GetQueriedOAuthPluginListResponse>({
  "url": "/api/plugin_api/get_queried_oauth_plugins",
  "method": "POST",
  "name": "GetQueriedOAuthPluginList",
  "reqType": "GetQueriedOAuthPluginListRequest",
  "reqMapping": {
    "body": ["bot_id"]
  },
  "resType": "GetQueriedOAuthPluginListResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetCodePluginDraft = /*#__PURE__*/createAPI<GetCodePluginDraftRequest, GetCodePluginDraftResponse>({
  "url": "/api/plugin_api/get_code_plugin_draft",
  "method": "POST",
  "name": "GetCodePluginDraft",
  "reqType": "GetCodePluginDraftRequest",
  "reqMapping": {
    "body": ["plugin_id", "space_id"]
  },
  "resType": "GetCodePluginDraftResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const SaveCodePluginDraft = /*#__PURE__*/createAPI<SaveCodePluginDraftRequest, SaveCodePluginDraftResponse>({
  "url": "/api/plugin_api/save_code_plugin_draft",
  "method": "POST",
  "name": "SaveCodePluginDraft",
  "reqType": "SaveCodePluginDraftRequest",
  "reqMapping": {
    "body": ["plugin_id", "space_id", "revision", "runtime", "entry_file", "files", "input_schema_json", "output_schema_json"]
  },
  "resType": "SaveCodePluginDraftResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const DebugCodePlugin = /*#__PURE__*/createAPI<DebugCodePluginRequest, DebugCodePluginResponse>({
  "url": "/api/plugin_api/debug_code_plugin",
  "method": "POST",
  "name": "DebugCodePlugin",
  "reqType": "DebugCodePluginRequest",
  "reqMapping": {
    "body": ["plugin_id", "space_id", "revision", "arguments_in_json"]
  },
  "resType": "DebugCodePluginResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export const GetCodePluginVersion = /*#__PURE__*/createAPI<GetCodePluginVersionRequest, GetCodePluginVersionResponse>({
  "url": "/api/plugin_api/get_code_plugin_version",
  "method": "POST",
  "name": "GetCodePluginVersion",
  "reqType": "GetCodePluginVersionRequest",
  "reqMapping": {
    "body": ["plugin_id", "space_id", "version"]
  },
  "resType": "GetCodePluginVersionResponse",
  "schemaRoot": "api://schemas/idl_plugin_plugin_develop",
  "service": "pluginDevelop"
});
export interface GetPlaygroundPluginListRequest {
  /** page number */
  page?: number,
  /** page size */
  size?: number,
  /** ignore */
  name?: string,
  /** Space ID */
  space_id?: string,
  /** If present, query according to plug-in id, no paging logic */
  plugin_ids: string[],
  /** When the length is 1 and it is a workflow, return the list of published workflows, and return the list of published plugins by default */
  plugin_types: number[],
  /** ignore */
  channel_id?: number,
  /** ignore */
  self_created?: boolean,
  /** sort */
  order_by?: number,
  /** ignore */
  is_get_offline?: boolean,
  /** ignore */
  Referer: string,
}
export interface GetPlaygroundPluginListResponse {
  code: number,
  msg: string,
  data: plugin_develop_common.GetPlaygroundPluginListData,
}
export interface GetPluginAPIsRequest {
  /** Plugin ID */
  plugin_id: string,
  /** If present, query according to tool id, no paging logic */
  api_ids: string[],
  /** page number */
  page: number,
  /** page size */
  size: number,
  /** ignore */
  order: plugin_develop_common.APIListOrder,
  /** ignore */
  preview_version_ts?: string,
}
export interface GetPluginAPIsResponse {
  code: number,
  msg: string,
  api_info: plugin_develop_common.PluginAPIInfo[],
  total: number,
  edit_version: number,
}
export interface GetUpdatedAPIsRequest {
  /** Plugin ID */
  plugin_id: string
}
export interface GetUpdatedAPIsResponse {
  code: number,
  msg: string,
  /** Newly created tool name */
  created_api_names: string[],
  /** Deleted tool name */
  deleted_api_names: string[],
  /** updated tool name */
  updated_api_names: string[],
}
export interface GetPluginInfoRequest {
  /** Currently only plugins are supported OpenAPI plugin information */
  plugin_id: string,
  /** ignore */
  preview_version_tsx?: string,
}
export interface GetPluginInfoResponse {
  code: number,
  msg: string,
  meta_info: plugin_develop_common.PluginMetaInfo,
  code_info: plugin_develop_common.CodeInfo,
  /** 0 No updates 1 Yes updates Not released */
  status: boolean,
  /** Has it been published? */
  published: boolean,
  /** creator information */
  creator: plugin_develop_common.Creator,
  /** ignore */
  statistic_data: plugin_develop_common.PluginStatisticData,
  /** ignore */
  plugin_product_status: plugin_develop_common.ProductStatus,
  /** ignore */
  privacy_status: boolean,
  /** ignore */
  privacy_info: string,
  /** ignore */
  creation_method: plugin_develop_common.CreationMethod,
  /** ignore */
  ide_code_runtime: string,
  /** ignore */
  edit_version: number,
  /** ignore */
  plugin_type: plugin_develop_common.PluginType,
}
export interface UpdatePluginRequest {
  plugin_id: string,
  /** plugin manifest in json string */
  ai_plugin: string,
  /** plugin openapi3 document in yaml string */
  openapi: string,
  /** ignore */
  client_id?: string,
  /** ignore */
  client_secret?: string,
  /** ignore */
  service_token?: string,
  /** ignore */
  source_code?: string,
  /** ignore */
  edit_version?: number,
}
export interface UpdatePluginResponse {
  code: number,
  msg: string,
  data: plugin_develop_common.UpdatePluginData,
}
export interface RegisterPluginMetaRequest {
  /** plugin name */
  name: string,
  /** Plugin description */
  desc: string,
  /** Plugin service address prefix */
  url?: string,
  /** plugin icon */
  icon: plugin_develop_common.PluginIcon,
  /** plug-in authorization type */
  auth_type?: plugin_develop_common.AuthorizationType,
  /** When the sub-authorization type is api/token, the token parameter position */
  location?: plugin_develop_common.AuthorizationServiceLocation,
  /** When the sub-authorization type is api/token, the token parameter key */
  key?: string,
  /** When the sub-authorization type is api/token, the token parameter value */
  service_token?: string,
  /** The authorization type is oauth Yes, oauth information, see GetOAuthSchema return value */
  oauth_info?: string,
  /** Space ID */
  space_id: string,
  /** Plugin public parameters, key is the parameter position, value is the parameter list */
  common_params?: {
    [key: string | number]: plugin_develop_common.commonParamSchema[]
  },
  /** ignore */
  creation_method?: plugin_develop_common.CreationMethod,
  /** ignore */
  ide_code_runtime?: string,
  /** ignore */
  plugin_type?: plugin_develop_common.PluginType,
  /** App ID */
  project_id?: string,
  /** Level 2 authorization type, 0: api/token of service, 10: client credentials of oauth */
  sub_auth_type?: number,
  /** ignore */
  auth_payload?: string,
  /** ignore */
  fixed_export_ip?: boolean,
}
export interface RegisterPluginMetaResponse {
  code: number,
  msg: string,
  plugin_id: string,
}
export interface UpdatePluginMetaRequest {
  plugin_id: string,
  name?: string,
  desc?: string,
  /** plugin service url */
  url?: string,
  icon?: plugin_develop_common.PluginIcon,
  auth_type?: plugin_develop_common.AuthorizationType,
  /** When the sub-authorization type is api/token, the token parameter position */
  location?: plugin_develop_common.AuthorizationServiceLocation,
  /** When the sub-authorization type is api/token, the token parameter key */
  key?: string,
  /** When the sub-authorization type is api/token, the token parameter value */
  service_token?: string,
  /** When the sub-authorization type is oauth, for oauth information, see GetOAuthSchema return value */
  oauth_info?: string,
  /** JSON serialization */
  common_params?: {
    [key: string | number]: plugin_develop_common.commonParamSchema[]
  },
  /** ignore */
  creation_method?: plugin_develop_common.CreationMethod,
  /** ignore */
  edit_version?: number,
  plugin_type?: plugin_develop_common.PluginType,
  /** Level 2 authorization type */
  sub_auth_type?: number,
  /** ignore */
  auth_payload?: string,
  /** ignore */
  fixed_export_ip?: boolean,
}
export interface UpdatePluginMetaResponse {
  code: number,
  msg: string,
  edit_version: number,
}
export interface PublishPluginRequest {
  plugin_id: string,
  /** Privacy Statement Status */
  privacy_status: boolean,
  /** Privacy Statement Content */
  privacy_info: string,
  version_name: string,
  version_desc: string,
}
export interface PublishPluginResponse {
  code: number,
  msg: string,
  version_ts: string,
}
/** Bot reference plugin */
export interface GetBotDefaultParamsRequest {
  space_id: string,
  bot_id: string,
  dev_id: string,
  plugin_id: string,
  api_name: string,
  plugin_referrer_id: string,
  plugin_referrer_scene: plugin_develop_common.PluginReferrerScene,
  plugin_is_debug: boolean,
  workflow_id: string,
  plugin_publish_version_ts?: string,
}
export interface GetBotDefaultParamsResponse {
  code: number,
  msg: string,
  request_params: plugin_develop_common.APIParameter[],
  response_params: plugin_develop_common.APIParameter[],
  response_style: plugin_develop_common.ResponseStyle,
}
export interface UpdateBotDefaultParamsRequest {
  space_id: string,
  bot_id: string,
  dev_id: string,
  plugin_id: string,
  api_name: string,
  request_params: plugin_develop_common.APIParameter[],
  response_params: plugin_develop_common.APIParameter[],
  plugin_referrer_id: string,
  plugin_referrer_scene: plugin_develop_common.PluginReferrerScene,
  response_style: plugin_develop_common.ResponseStyle,
  workflow_id: string,
}
export interface UpdateBotDefaultParamsResponse {
  code: number,
  msg: string,
}
export interface DeleteBotDefaultParamsRequest {
  bot_id: string,
  dev_id: string,
  plugin_id: string,
  api_name: string,
  /**
   * Bot removal tool when: DeleteBot = false, APIName to set
   * Delete bot: DeleteBot = true, APIName is empty
  */
  delete_bot: boolean,
  space_id: string,
  plugin_referrer_id: string,
  plugin_referrer_scene: plugin_develop_common.PluginReferrerScene,
  workflow_id: string,
  api_id: string,
}
export interface DeleteBotDefaultParamsResponse {}
export interface UpdateAPIRequest {
  plugin_id: string,
  api_id: string,
  name?: string,
  desc?: string,
  /** http subURL of tool */
  path?: string,
  /** http method of tool */
  method?: plugin_develop_common.APIMethod,
  /** request parameters of tool */
  request_params?: plugin_develop_common.APIParameter[],
  /** response parameters of tool */
  response_params?: plugin_develop_common.APIParameter[],
  /** whether disable tool */
  disabled?: boolean,
  /** ignore */
  api_extend?: plugin_develop_common.APIExtend,
  /** ignore */
  edit_version?: number,
  /** whether save example */
  save_example?: boolean,
  debug_example?: plugin_develop_common.DebugExample,
  /** ignore */
  function_name?: string,
}
export interface UpdateAPIResponse {
  code: number,
  msg: string,
  edit_version: number,
}
export interface DelPluginRequest {
  plugin_id: string
}
export interface DelPluginResponse {
  code: number,
  msg: string,
}
export interface CreateAPIRequest {
  plugin_id: string,
  /** tool name */
  name: string,
  /** tool description */
  desc: string,
  /** http subURL of tool */
  path?: string,
  /** http method of tool */
  method?: plugin_develop_common.APIMethod,
  /** ignore */
  api_extend?: plugin_develop_common.APIExtend,
  /** ignore */
  request_params?: plugin_develop_common.APIParameter[],
  /** ignore */
  response_params?: plugin_develop_common.APIParameter[],
  /** ignore */
  disabled?: boolean,
  /** ignore */
  edit_version?: number,
  /** ignore */
  function_name?: string,
}
export interface CreateAPIResponse {
  code: number,
  msg: string,
  api_id: string,
  edit_version: number,
}
export interface DeleteAPIRequest {
  plugin_id: string,
  api_id: string,
  /** ignore */
  edit_version?: number,
}
export interface DeleteAPIResponse {
  code: number,
  msg: string,
  edit_version: number,
}
export interface GetOAuthSchemaRequest {}
export interface GetOAuthSchemaResponse {
  code: number,
  msg: string,
  oauth_schema: string,
  ide_conf: string,
}
export interface GetUserAuthorityRequest {
  plugin_id: string,
  creation_method: plugin_develop_common.CreationMethod,
  project_id: string,
}
export interface GetUserAuthorityResponse {
  code: number,
  msg: string,
  data: plugin_develop_common.GetUserAuthorityData,
}
/** Get authorization status--plugin debug area */
export interface GetOAuthStatusRequest {
  plugin_id: string
}
export interface GetOAuthStatusResponse {
  /** Is it an authorized plugin? */
  is_oauth: boolean,
  /** user authorization status */
  status: plugin_develop_common.OAuthStatus,
  /** Unauthorized, return the authorized url. */
  content: string,
  code: number,
  msg: string,
}
export interface CheckAndLockPluginEditRequest {
  plugin_id: string
}
export interface CheckAndLockPluginEditResponse {
  code: number,
  msg: string,
  data: plugin_develop_common.CheckAndLockPluginEditData,
}
export interface GetPluginPublishHistoryRequest {
  plugin_id: string,
  space_id: string,
  /** Turn the page, what page? */
  page?: number,
  /** Flip pages, a few entries per page */
  size?: number,
}
export interface GetPluginPublishHistoryResponse {
  code: number,
  msg: string,
  /** reverse time */
  plugin_publish_info_list: plugin_develop_common.PluginPublishInfo[],
  /** How many in total, greater than page x size description and next page */
  total: number,
}
export interface DebugAPIRequest {
  plugin_id: string,
  api_id: string,
  /** request parameters in json string */
  parameters: string,
  /** ignore */
  operation: plugin_develop_common.DebugOperation,
  /** ignore */
  edit_version?: number,
}
export interface DebugAPIResponse {
  code: number,
  msg: string,
  /** response parameters */
  response_params: plugin_develop_common.APIParameter[],
  /** invoke success or not */
  success: boolean,
  /** trimmed response in json string */
  resp: string,
  /** invoke failed reason */
  reason: string,
  /** raw response in json string */
  raw_resp: string,
  /** raw request in json string */
  raw_req: string,
}
export interface UnlockPluginEditRequest {
  plugin_id: string
}
export interface UnlockPluginEditResponse {
  code: number,
  msg: string,
  released: boolean,
}
export interface GetPluginNextVersionRequest {
  plugin_id: string,
  space_id: string,
}
export interface GetPluginNextVersionResponse {
  code: number,
  msg: string,
  next_version_name: string,
}
export interface RegisterPluginRequest {
  /** plugin manifest in json string */
  ai_plugin: string,
  /** plugin openapi3 document in yaml string */
  openapi: string,
  /** ignore */
  client_id?: string,
  /** ignore */
  client_secret?: string,
  /** ignore */
  service_token?: string,
  /** ignore */
  plugin_type?: plugin_develop_common.PluginType,
  space_id: string,
  /** ignore */
  import_from_file: boolean,
  project_id?: string,
}
export interface RegisterPluginResponse {
  code: number,
  msg: string,
  data: plugin_develop_common.RegisterPluginData,
}
export interface GetDevPluginListRequest {
  status?: plugin_develop_common.PluginStatus[],
  page?: number,
  size?: number,
  dev_id: string,
  space_id: string,
  scope_type?: plugin_develop_common.ScopeType,
  order_by?: plugin_develop_common.OrderBy,
  /** Release status filter: true: published, false: not published */
  publish_status?: boolean,
  /** Plugin name or tool name */
  name?: string,
  /** Plugin Type Filter, End/Cloud */
  plugin_type_for_filter?: plugin_develop_common.PluginTypeForFilter,
  project_id: string,
  /** plugin id list */
  plugin_ids: string[],
}
export interface GetDevPluginListResponse {
  code: number,
  msg: string,
  plugin_list: plugin_develop_common.PluginInfoForPlayground[],
  total: string,
}
export interface Convert2OpenAPIRequest {
  plugin_name?: string,
  plugin_url?: string,
  /** import content, e.g. curl, postman, swagger */
  data: string,
  /** ignore */
  merge_same_paths?: boolean,
  space_id: string,
  /** ignore */
  plugin_description?: string,
}
export interface Convert2OpenAPIResponse {
  code: number,
  msg: string,
  /** openapi3 document in yaml string */
  openapi?: string,
  /** plugin manifest in json string */
  ai_plugin?: string,
  /** protocol type */
  plugin_data_format?: plugin_develop_common.PluginDataFormat,
  /** ignore */
  duplicate_api_infos: plugin_develop_common.DuplicateAPIInfo[],
}
export interface BatchCreateAPIRequest {
  plugin_id: string,
  /** plugin manifest in json string */
  ai_plugin: string,
  /** plugin openapi3 document in yaml string */
  openapi: string,
  space_id: string,
  /** ignore */
  dev_id: string,
  /** whether to replace the same tool, method:subURL is unique */
  replace_same_paths: boolean,
  /** ignore */
  paths_to_replace?: plugin_develop_common.PluginAPIInfo[],
  /** ignore */
  edit_version?: number,
}
export interface BatchCreateAPIResponse {
  code: number,
  msg: string,
  /**
   * PathsToReplace represents the tools to override,
   * If BaseResp. StatusCode = DuplicateAPIPath, then PathsToReplace is not empty
  */
  paths_duplicated?: plugin_develop_common.PluginAPIInfo[],
  paths_created?: plugin_develop_common.PluginAPIInfo[],
  edit_version: number,
}
export interface RevokeAuthTokenRequest {
  plugin_id: string,
  /** If not passed using uid assignment bot_id = connector_uid */
  bot_id?: string,
  context_type?: number,
}
export interface RevokeAuthTokenResponse {}
export interface OAuthPluginInfo {
  plugin_id: string,
  /** user authorization status */
  status: plugin_develop_common.OAuthStatus,
  /** Plugin name */
  name: string,
  /** plugin avatar */
  plugin_icon: string,
}
export interface GetQueriedOAuthPluginListRequest {
  bot_id: string
}
export interface GetQueriedOAuthPluginListResponse {
  oauth_plugin_list: OAuthPluginInfo[],
  code: number,
  msg: string,
}
export interface GetCodePluginDraftRequest {
  plugin_id: string,
  space_id: string,
}
export interface GetCodePluginDraftResponse {
  code: number,
  msg: string,
  data?: plugin_develop_common.CodePluginDraftData,
}
export interface SaveCodePluginDraftRequest {
  plugin_id: string,
  space_id: string,
  revision: number,
  runtime: plugin_develop_common.CodePluginRuntime,
  entry_file: string,
  files: plugin_develop_common.CodePluginFile[],
  input_schema_json?: string,
  output_schema_json?: string,
}
export interface SaveCodePluginDraftResponse {
  code: number,
  msg: string,
  data?: plugin_develop_common.CodePluginDraftData,
}
export interface DebugCodePluginRequest {
  plugin_id: string,
  space_id: string,
  revision: number,
  arguments_in_json: string,
}
export interface DebugCodePluginResponse {
  code: number,
  msg: string,
  data?: plugin_develop_common.CodePluginDebugData,
}
export interface GetCodePluginVersionRequest {
  plugin_id: string,
  space_id: string,
  version: string,
}
export interface GetCodePluginVersionResponse {
  code: number,
  msg: string,
  data?: plugin_develop_common.CodePluginVersionData,
}