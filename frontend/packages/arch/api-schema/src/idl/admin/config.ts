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

import * as developer_api from './../app/developer_api';
export { developer_api };
import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export interface GetModelListReq {
  keyword?: string,
  provider_key?: string,
  capability_type?: string,
  enabled?: boolean,
  access_mode?: string,
  page?: number,
  page_size?: number,
}
export interface GetModelListResp {
  provider_model_list: ProviderModelList[],
  models?: ModelManagementItem[],
  total?: number,
  code: number,
  msg: string,
}
export interface ProviderModelList {
  provider: ModelProvider,
  model_list: Model[],
}
export interface I18nText {
  zh_cn: string,
  en_us: string,
}
export interface ModelProvider {
  name: I18nText,
  icon_uri: string,
  icon_url: string,
  description: I18nText,
  model_class: developer_api.ModelClass,
}
export interface DisplayInfo {
  name: string,
  description: I18nText,
  output_tokens: number,
  max_tokens: number,
}
export enum ModelType {
  LLM = 0,
  TextEmbedding = 1,
  Rerank = 2,
}
export interface Model {
  id: number,
  provider: ModelProvider,
  display_info: DisplayInfo,
  capability: developer_api.ModelAbility,
  connection: Connection,
  type: ModelType,
  parameters: developer_api.ModelParameter[],
  status: ModelStatus,
  enable_base64_url: boolean,
  delete_at_ms: number,
}
export enum ThinkingType {
  Default = 0,
  Enable = 1,
  Disable = 2,
  Auto = 3,
}
export enum ModelStatus {
  /** Default state when not configured, equivalent to StatusInUse */
  StatusDefault = 0,
  /** In the application, it can be used to create new */
  StatusInUse = 1,
  /** It is offline, unusable, and cannot be created. */
  StatusDeleted = 2,
}
export enum ModelAccessMode {
  ALL = 1,
  RESTRICTED = 2,
}
export enum ModelGrantSubjectType {
  WORKSPACE = 1,
  USER = 2,
}
export enum ModelRoutingStrategy {
  ROUND_ROBIN = 1,
  WEIGHTED_ROUND_ROBIN = 2,
}
export interface ModelProviderOption {
  provider_key: string,
  name: I18nText,
  model_class: developer_api.ModelClass,
  protocol: string,
  supports_custom_base_url: boolean,
  supports_function_call: boolean,
  supports_multimodal: boolean,
  default_base_url?: string,
}
export interface ListModelProvidersReq {}
export interface ListModelProvidersResp {
  providers: ModelProviderOption[],
  code: number,
  msg: string,
}
export interface ModelEndpointInput {
  id?: string,
  base_url: string,
  api_key?: string,
  clear_api_key?: boolean,
  weight: number,
  enabled: boolean,
  sort_order: number,
}
export interface ModelEndpointView {
  id: string,
  base_url: string,
  has_api_key: boolean,
  weight: number,
  enabled: boolean,
  sort_order: number,
}
export interface ModelManagementInput {
  provider_key: string,
  name: string,
  model_identifier: string,
  description?: string,
  capability_types: string[],
  reasoning_mode: string,
  max_context_tokens: number,
  max_output_tokens: number,
  function_call_mode: string,
  enabled: boolean,
  usage_scenarios: string[],
  protocol: string,
  routing_strategy: ModelRoutingStrategy,
  access_mode: ModelAccessMode,
  endpoints: ModelEndpointInput[],
  enable_base64_url?: boolean,
}
export interface ModelManagementItem {
  id: string,
  provider_key: string,
  model_class: developer_api.ModelClass,
  name: string,
  model_identifier: string,
  description?: string,
  capability_types: string[],
  enabled: boolean,
  access_mode: ModelAccessMode,
  creator_id: string,
  creator_name?: string,
  updated_at_ms: number,
  sort_order: number,
}
export interface ModelDetail {
  summary: ModelManagementItem,
  reasoning_mode: string,
  max_context_tokens: number,
  max_output_tokens: number,
  function_call_mode: string,
  usage_scenarios: string[],
  protocol: string,
  routing_strategy: ModelRoutingStrategy,
  endpoints: ModelEndpointView[],
  enable_base64_url: boolean,
}
export interface GetModelDetailReq {
  id: string
}
export interface GetModelDetailResp {
  model: ModelDetail,
  code: number,
  msg: string,
}
export interface ModelGrantSubject {
  subject_type: ModelGrantSubjectType,
  subject_id: string,
  name?: string,
  description?: string,
}
export interface GetModelGrantsReq {
  model_id: string
}
export interface GetModelGrantsResp {
  access_mode: ModelAccessMode,
  grants: ModelGrantSubject[],
  code: number,
  msg: string,
}
export interface SaveModelGrantsReq {
  model_id: string,
  access_mode: ModelAccessMode,
  grants: ModelGrantSubject[],
}
export interface SaveModelGrantsResp {
  code: number,
  msg: string,
}
export interface TestModelEndpointReq {
  model_id?: string,
  endpoint: ModelEndpointInput,
  provider_key: string,
  model_identifier: string,
  protocol: string,
}
export interface TestModelEndpointResp {
  success: boolean,
  latency_ms: number,
  error_code?: string,
  error_message?: string,
  code: number,
  msg: string,
}
export interface UpdateModelStatusReq {
  id: string,
  enabled: boolean,
}
export interface UpdateModelStatusResp {
  code: number,
  msg: string,
}
export interface ModelSortItem {
  id: string,
  sort_order: number,
}
export interface UpdateModelSortReq {
  items: ModelSortItem[]
}
export interface UpdateModelSortResp {
  code: number,
  msg: string,
}
export interface ModelDependencySample {
  id: string,
  name: string,
}
export interface ModelDependencySummary {
  dependency_type: string,
  count: number,
  samples: ModelDependencySample[],
}
export interface Connection {
  base_conn_info: BaseConnectionInfo,
  ark?: ArkConnInfo,
  openai?: OpenAIConnInfo,
  deepseek?: DeepseekConnInfo,
  gemini?: GeminiConnInfo,
  qwen?: QwenConnInfo,
  ollama?: OllamaConnInfo,
  claude?: ClaudeConnInfo,
}
export interface BaseConnectionInfo {
  base_url: string,
  api_key: string,
  model: string,
  thinking_type: ThinkingType,
}
export interface EmbeddingInfo {
  dims: number
}
export interface ArkConnInfo {
  region: string,
  api_type: string,
}
export interface OpenAIConnInfo {
  by_azure: boolean,
  api_version: string,
}
export interface GeminiConnInfo {
  /** "1" for BackendGeminiAPI / "2" for BackendVertexAI */
  backend: number,
  project: string,
  location: string,
}
export interface DeepseekConnInfo {}
export interface QwenConnInfo {}
export interface OllamaConnInfo {}
export interface ClaudeConnInfo {}
export interface CreateModelReq {
  model_class: developer_api.ModelClass,
  model_name: string,
  connection: Connection,
  enable_base64_url: boolean,
  management?: ModelManagementInput,
}
export interface CreateModelResp {
  id: string,
  code: number,
  msg: string,
}
export interface DeleteModelReq {
  id: string,
  preview?: boolean,
}
export interface DeleteModelResp {
  dependencies?: ModelDependencySummary[],
  code: number,
  msg: string,
}
export interface UpdateModelReq {
  model?: Model,
  id?: string,
  management?: ModelManagementInput,
}
export interface UpdateModelResp {
  code: number,
  msg: string,
}
export interface SaveBasicConfigurationReq {
  configuration: BasicConfiguration
}
export interface SaveBasicConfigurationResp {
  code: number,
  msg: string,
}
export interface GetBasicConfigurationReq {}
export interface GetBasicConfigurationResp {
  configuration: BasicConfiguration,
  code: number,
  msg: string,
}
export enum CodeRunnerType {
  Local = 0,
  Sandbox = 1,
}
export interface SandboxConfig {
  allow_env: string,
  allow_read: string,
  allow_write: string,
  allow_run: string,
  allow_net: string,
  allow_ffi: string,
  node_modules_dir: string,
  timeout_seconds: number,
  memory_limit_mb: number,
}
export interface BasicConfiguration {
  admin_emails: string,
  disable_user_registration: boolean,
  allow_registration_email: string,
  plugin_configuration: PluginConfiguration,
  code_runner_type: CodeRunnerType,
  sandbox_config?: SandboxConfig,
  server_host: string,
  site_name?: string,
  site_description?: string,
  site_logo_uri?: string,
  favicon_uri?: string,
}
export interface PluginConfiguration {
  coze_saas_plugin_enabled: boolean,
  coze_api_token: string,
  coze_saas_api_base_url: string,
}
export interface UpdateKnowledgeConfigReq {
  knowledge_config: KnowledgeConfig
}
export interface UpdateKnowledgeConfigResp {
  code: number,
  msg: string,
}
export interface GetKnowledgeConfigReq {}
export interface GetKnowledgeConfigResp {
  knowledge_config: KnowledgeConfig,
  code: number,
  msg: string,
}
export interface KnowledgeConfig {
  embedding_config: EmbeddingConfig,
  rerank_config: RerankConfig,
  ocr_config: OCRConfig,
  parser_config: ParserConfig,
  builtin_model_id: number,
}
export interface EmbeddingConfig {
  type: EmbeddingType,
  max_batch_size: number,
  connection: EmbeddingConnection,
}
export enum EmbeddingType {
  Ark = 0,
  OpenAI = 1,
  Ollama = 2,
  Gemini = 3,
  HTTP = 4,
}
export interface EmbeddingConnection {
  base_conn_info: BaseConnectionInfo,
  embedding_info: EmbeddingInfo,
  ark?: ArkConnInfo,
  openai?: OpenAIConnInfo,
  ollama?: OllamaConnInfo,
  gemini?: GeminiConnInfo,
  http?: HttpConnection,
}
export interface HttpConnection {
  address: string
}
export enum RerankType {
  VikingDB = 0,
  RRF = 1,
}
export interface RerankConfig {
  type: RerankType,
  vikingdb_config: VikingDBConfig,
}
export interface VikingDBConfig {
  ak: string,
  sk: string,
  host: string,
  region: string,
  model: string,
}
export enum OCRType {
  Volcengine = 0,
  Paddleocr = 1,
}
export interface OCRConfig {
  type: OCRType,
  volcengine_ak: string,
  volcengine_sk: string,
  paddleocr_api_url: string,
}
export enum ParserType {
  builtin = 0,
  Paddleocr = 1,
}
export interface ParserConfig {
  type: ParserType,
  paddleocr_structure_api_url: string,
}
export enum ObjectStorageProviderType {
  QINIU = 1,
  ALIYUN_OSS = 2,
  TENCENT_COS = 3,
  HUAWEI_OBS = 4,
  AWS_S3 = 5,
  MINIO = 6,
  TOS = 7,
}
export enum ObjectStorageHealthStatus {
  UNKNOWN = 1,
  HEALTHY = 2,
  UNHEALTHY = 3,
}
export enum ObjectStorageRuntimeSource {
  DATABASE = 1,
  ENV_RESCUE = 2,
}
export interface ObjectStoragePublicConfig {
  bucket?: string,
  region?: string,
  endpoint?: string,
  endpoint_override?: string,
  force_path_style?: boolean,
  use_ssl?: boolean,
  download_domain?: string,
  use_https?: boolean,
}
export interface ObjectStorageCredentialInput {
  access_key_id?: string,
  secret_access_key?: string,
}
export interface ObjectStorageHealthView {
  status: ObjectStorageHealthStatus,
  code?: string,
  message?: string,
  latency_ms?: number,
  checked_at?: string,
}
export interface ObjectStorageConfigView {
  id: string,
  name: string,
  provider_type: ObjectStorageProviderType,
  config: ObjectStoragePublicConfig,
  credential_configured: boolean,
  health: ObjectStorageHealthView,
  desired_active: boolean,
  runtime_active: boolean,
  restart_required: boolean,
  version: string,
  runtime_revision: string,
  created_at: string,
  updated_at: string,
}
export interface ListObjectStorageConfigsReq {}
export interface ListObjectStorageConfigsResp {
  configs: ObjectStorageConfigView[],
  runtime_source: ObjectStorageRuntimeSource,
  restart_required: boolean,
  code: number,
  msg: string,
}
export interface CreateObjectStorageConfigReq {
  name: string,
  provider_type: ObjectStorageProviderType,
  config: ObjectStoragePublicConfig,
  credential: ObjectStorageCredentialInput,
}
export interface CreateObjectStorageConfigResp {
  config: ObjectStorageConfigView,
  code: number,
  msg: string,
}
export interface UpdateObjectStorageConfigReq {
  id: string,
  expected_version: string,
  name: string,
  config: ObjectStoragePublicConfig,
  credential?: ObjectStorageCredentialInput,
}
export interface UpdateObjectStorageConfigResp {
  config: ObjectStorageConfigView,
  code: number,
  msg: string,
}
export interface TestObjectStorageConfigReq {
  id?: string,
  expected_version?: string,
  provider_type: ObjectStorageProviderType,
  config: ObjectStoragePublicConfig,
  credential?: ObjectStorageCredentialInput,
}
export interface TestObjectStorageConfigResp {
  success: boolean,
  health: ObjectStorageHealthView,
  code: number,
  msg: string,
}
export interface ActivateObjectStorageConfigReq {
  id: string,
  expected_version: string,
  migration_confirmed: boolean,
}
export interface ActivateObjectStorageConfigResp {
  config: ObjectStorageConfigView,
  code: number,
  msg: string,
}
export interface DeleteObjectStorageConfigReq {
  id: string,
  expected_version: string,
}
export interface DeleteObjectStorageConfigResp {
  code: number,
  msg: string,
}
export const GetBasicConfiguration = /*#__PURE__*/createAPI<GetBasicConfigurationReq, GetBasicConfigurationResp>({
  "url": "/api/admin/config/basic/get",
  "method": "GET",
  "name": "GetBasicConfiguration",
  "reqType": "GetBasicConfigurationReq",
  "reqMapping": {},
  "resType": "GetBasicConfigurationResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const SaveBasicConfiguration = /*#__PURE__*/createAPI<SaveBasicConfigurationReq, SaveBasicConfigurationResp>({
  "url": "/api/admin/config/basic/save",
  "method": "POST",
  "name": "SaveBasicConfiguration",
  "reqType": "SaveBasicConfigurationReq",
  "reqMapping": {
    "body": ["configuration"]
  },
  "resType": "SaveBasicConfigurationResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const GetKnowledgeConfig = /*#__PURE__*/createAPI<GetKnowledgeConfigReq, GetKnowledgeConfigResp>({
  "url": "/api/admin/config/knowledge/get",
  "method": "GET",
  "name": "GetKnowledgeConfig",
  "reqType": "GetKnowledgeConfigReq",
  "reqMapping": {},
  "resType": "GetKnowledgeConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const UpdateKnowledgeConfig = /*#__PURE__*/createAPI<UpdateKnowledgeConfigReq, UpdateKnowledgeConfigResp>({
  "url": "/api/admin/config/knowledge/save",
  "method": "POST",
  "name": "UpdateKnowledgeConfig",
  "reqType": "UpdateKnowledgeConfigReq",
  "reqMapping": {
    "body": ["knowledge_config"]
  },
  "resType": "UpdateKnowledgeConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const GetModelList = /*#__PURE__*/createAPI<GetModelListReq, GetModelListResp>({
  "url": "/api/admin/config/model/list",
  "method": "GET",
  "name": "GetModelList",
  "reqType": "GetModelListReq",
  "reqMapping": {
    "query": ["keyword", "provider_key", "capability_type", "enabled", "access_mode", "page", "page_size"]
  },
  "resType": "GetModelListResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const ListModelProviders = /*#__PURE__*/createAPI<ListModelProvidersReq, ListModelProvidersResp>({
  "url": "/api/admin/config/model/providers",
  "method": "GET",
  "name": "ListModelProviders",
  "reqType": "ListModelProvidersReq",
  "reqMapping": {},
  "resType": "ListModelProvidersResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const ListModels = /*#__PURE__*/createAPI<GetModelListReq, GetModelListResp>({
  "url": "/api/admin/config/model/manage/list",
  "method": "GET",
  "name": "ListModels",
  "reqType": "GetModelListReq",
  "reqMapping": {
    "query": ["keyword", "provider_key", "capability_type", "enabled", "access_mode", "page", "page_size"]
  },
  "resType": "GetModelListResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const GetModelDetail = /*#__PURE__*/createAPI<GetModelDetailReq, GetModelDetailResp>({
  "url": "/api/admin/config/model/detail",
  "method": "GET",
  "name": "GetModelDetail",
  "reqType": "GetModelDetailReq",
  "reqMapping": {
    "query": ["id"]
  },
  "resType": "GetModelDetailResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const CreateModel = /*#__PURE__*/createAPI<CreateModelReq, CreateModelResp>({
  "url": "/api/admin/config/model/create",
  "method": "POST",
  "name": "CreateModel",
  "reqType": "CreateModelReq",
  "reqMapping": {
    "body": ["model_class", "model_name", "connection", "enable_base64_url", "management"]
  },
  "resType": "CreateModelResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const UpdateModel = /*#__PURE__*/createAPI<UpdateModelReq, UpdateModelResp>({
  "url": "/api/admin/config/model/update",
  "method": "POST",
  "name": "UpdateModel",
  "reqType": "UpdateModelReq",
  "reqMapping": {
    "body": ["model", "id", "management"]
  },
  "resType": "UpdateModelResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const TestModelEndpoint = /*#__PURE__*/createAPI<TestModelEndpointReq, TestModelEndpointResp>({
  "url": "/api/admin/config/model/test",
  "method": "POST",
  "name": "TestModelEndpoint",
  "reqType": "TestModelEndpointReq",
  "reqMapping": {
    "body": ["model_id", "endpoint", "provider_key", "model_identifier", "protocol"]
  },
  "resType": "TestModelEndpointResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const UpdateModelStatus = /*#__PURE__*/createAPI<UpdateModelStatusReq, UpdateModelStatusResp>({
  "url": "/api/admin/config/model/status",
  "method": "POST",
  "name": "UpdateModelStatus",
  "reqType": "UpdateModelStatusReq",
  "reqMapping": {
    "body": ["id", "enabled"]
  },
  "resType": "UpdateModelStatusResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const UpdateModelSort = /*#__PURE__*/createAPI<UpdateModelSortReq, UpdateModelSortResp>({
  "url": "/api/admin/config/model/sort",
  "method": "POST",
  "name": "UpdateModelSort",
  "reqType": "UpdateModelSortReq",
  "reqMapping": {
    "body": ["items"]
  },
  "resType": "UpdateModelSortResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const GetModelGrants = /*#__PURE__*/createAPI<GetModelGrantsReq, GetModelGrantsResp>({
  "url": "/api/admin/config/model/grants",
  "method": "GET",
  "name": "GetModelGrants",
  "reqType": "GetModelGrantsReq",
  "reqMapping": {
    "query": ["model_id"]
  },
  "resType": "GetModelGrantsResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const SaveModelGrants = /*#__PURE__*/createAPI<SaveModelGrantsReq, SaveModelGrantsResp>({
  "url": "/api/admin/config/model/grants",
  "method": "POST",
  "name": "SaveModelGrants",
  "reqType": "SaveModelGrantsReq",
  "reqMapping": {
    "body": ["model_id", "access_mode", "grants"]
  },
  "resType": "SaveModelGrantsResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const DeleteModel = /*#__PURE__*/createAPI<DeleteModelReq, DeleteModelResp>({
  "url": "/api/admin/config/model/delete",
  "method": "POST",
  "name": "DeleteModel",
  "reqType": "DeleteModelReq",
  "reqMapping": {
    "body": ["id", "preview"]
  },
  "resType": "DeleteModelResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const ListObjectStorageConfigs = /*#__PURE__*/createAPI<ListObjectStorageConfigsReq, ListObjectStorageConfigsResp>({
  "url": "/api/admin/config/object-storage/list",
  "method": "GET",
  "name": "ListObjectStorageConfigs",
  "reqType": "ListObjectStorageConfigsReq",
  "reqMapping": {},
  "resType": "ListObjectStorageConfigsResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const CreateObjectStorageConfig = /*#__PURE__*/createAPI<CreateObjectStorageConfigReq, CreateObjectStorageConfigResp>({
  "url": "/api/admin/config/object-storage/create",
  "method": "POST",
  "name": "CreateObjectStorageConfig",
  "reqType": "CreateObjectStorageConfigReq",
  "reqMapping": {
    "body": ["name", "provider_type", "config", "credential"]
  },
  "resType": "CreateObjectStorageConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const UpdateObjectStorageConfig = /*#__PURE__*/createAPI<UpdateObjectStorageConfigReq, UpdateObjectStorageConfigResp>({
  "url": "/api/admin/config/object-storage/update",
  "method": "POST",
  "name": "UpdateObjectStorageConfig",
  "reqType": "UpdateObjectStorageConfigReq",
  "reqMapping": {
    "body": ["id", "expected_version", "name", "config", "credential"]
  },
  "resType": "UpdateObjectStorageConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const TestObjectStorageConfig = /*#__PURE__*/createAPI<TestObjectStorageConfigReq, TestObjectStorageConfigResp>({
  "url": "/api/admin/config/object-storage/test",
  "method": "POST",
  "name": "TestObjectStorageConfig",
  "reqType": "TestObjectStorageConfigReq",
  "reqMapping": {
    "body": ["id", "expected_version", "provider_type", "config", "credential"]
  },
  "resType": "TestObjectStorageConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const ActivateObjectStorageConfig = /*#__PURE__*/createAPI<ActivateObjectStorageConfigReq, ActivateObjectStorageConfigResp>({
  "url": "/api/admin/config/object-storage/activate",
  "method": "POST",
  "name": "ActivateObjectStorageConfig",
  "reqType": "ActivateObjectStorageConfigReq",
  "reqMapping": {
    "body": ["id", "expected_version", "migration_confirmed"]
  },
  "resType": "ActivateObjectStorageConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
export const DeleteObjectStorageConfig = /*#__PURE__*/createAPI<DeleteObjectStorageConfigReq, DeleteObjectStorageConfigResp>({
  "url": "/api/admin/config/object-storage/delete",
  "method": "POST",
  "name": "DeleteObjectStorageConfig",
  "reqType": "DeleteObjectStorageConfigReq",
  "reqMapping": {
    "body": ["id", "expected_version"]
  },
  "resType": "DeleteObjectStorageConfigResp",
  "schemaRoot": "api://schemas/idl_admin_config",
  "service": "adminConfig"
});
