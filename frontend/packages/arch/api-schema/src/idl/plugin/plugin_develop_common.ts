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

export enum OnlineStatus {
  OFFLINE = 0,
  ONLINE = 1,
}
export enum DebugExampleStatus {
  Default = 0,
  Enable = 1,
  Disable = 2,
}
export enum ParameterLocation {
  Path = 1,
  Query = 2,
  Body = 3,
  Header = 4,
}
/** plugin enumeration value */
export enum PluginParamTypeFormat {
  FileUrl = 0,
  ImageUrl = 1,
  DocUrl = 2,
  CodeUrl = 3,
  PptUrl = 4,
  TxtUrl = 5,
  ExcelUrl = 6,
  AudioUrl = 7,
  ZipUrl = 8,
  VideoUrl = 9,
}
export enum APIMethod {
  GET = 1,
  POST = 2,
  PUT = 3,
  DELETE = 4,
  PATCH = 5,
}
export enum APIDebugStatus {
  DebugWaiting = 0,
  DebugPassed = 1,
}
export enum ParameterType {
  String = 1,
  Integer = 2,
  Number = 3,
  Object = 4,
  Array = 5,
  Bool = 6,
}
/** Default imported parameter settings source */
export enum DefaultParamSource {
  /** default user input */
  Input = 0,
  /** reference variable */
  Variable = 1,
}
/** Subdivision types for File type parameters */
export enum AssistParameterType {
  DEFAULT = 1,
  IMAGE = 2,
  DOC = 3,
  CODE = 4,
  PPT = 5,
  TXT = 6,
  EXCEL = 7,
  AUDIO = 8,
  ZIP = 9,
  VIDEO = 10,
  /** voice */
  VOICE = 12,
}
export enum PluginToolAuthType {
  /** strong authorization */
  Required = 0,
  /** semi-anonymous authorization */
  Supported = 1,
  /** not authorized */
  Disable = 2,
}
export enum PluginCardStatus {
  Latest = 1,
  /** The main card version has been upgraded. */
  NeedUpdate = 2,
  /** Plugin tool exported parameters do not match */
  ParamMisMatch = 3,
}
export enum PluginType {
  PLUGIN = 1,
  APP = 2,
  FUNC = 3,
  WORKFLOW = 4,
  IMAGEFLOW = 5,
  LOCAL = 6,
}
export enum PluginStatus {
  SUBMITTED = 1,
  REVIEWING = 2,
  PREPARED = 3,
  PUBLISHED = 4,
  OFFLINE = 5,
  /** default value */
  Draft = 0,
  /** disable */
  BANNED = 6,
}
export enum ProductStatus {
  NeverListed = 0,
  Listed = 1,
  Unlisted = 2,
  Banned = 3,
}
export enum ProductUnlistType {
  ByAdmin = 1,
  ByUser = 2,
}
export enum CreationMethod {
  COZE = 0,
  IDE = 1,
}
export enum APIListOrderBy {
  CreateTime = 1,
}
export enum SpaceRoleType {
  /** default */
  Default = 0,
  /** owner */
  Owner = 1,
  /** administrator */
  Admin = 2,
  /** ordinary member */
  Member = 3,
}
export enum RunMode {
  DefaultToSync = 0,
  Sync = 1,
  Async = 2,
  Streaming = 3,
}
export enum AuthorizationType {
  None = 0,
  Service = 1,
  OAuth = 3,
  /** deprecated, the same as OAuth */
  Standard = 4,
}
export enum ServiceAuthSubType {
  ApiKey = 0,
  /** for opencoze */
  OAuthAuthorizationCode = 4,
}
export enum AuthorizationServiceLocation {
  Header = 1,
  Query = 2,
}
export enum PluginReferrerScene {
  SingleAgent = 0,
  WorkflowLlmNode = 1,
}
export enum WorkflowResponseMode {
  /** model summary */
  UseLLM = 0,
  /** Do not use model summaries */
  SkipLLM = 1,
}
export interface ResponseStyle {
  workflow_response_mode: WorkflowResponseMode
}
export interface CodeInfo {
  /** plugin manifest in json string */
  plugin_desc: string,
  /** plugin openapi3 document in yaml string */
  openapi_desc: string,
  client_id: string,
  client_secret: string,
  service_token: string,
}
export interface APIListOrder {
  order_by: APIListOrderBy,
  desc: boolean,
}
export interface UserLabel {
  label_id: string,
  label_name: string,
  icon_uri: string,
  icon_url: string,
  jump_link: string,
}
export interface PluginMetaInfo {
  /** plugin name */
  name: string,
  /** Plugin description */
  desc: string,
  /** Plugin service address prefix */
  url: string,
  /** plugin icon */
  icon: PluginIcon,
  /** Plugin authorization type, 0: no authorization, 1: service, 3: oauth */
  auth_type: AuthorizationType[],
  /** When the sub-authorization type is api/token, the token parameter position */
  location?: AuthorizationServiceLocation,
  /** When the sub-authorization type is api/token, the token parameter key */
  key?: string,
  /** When the sub-authorization type is api/token, the token parameter value */
  service_token?: string,
  /** When the sub-authorization type is oauth, the oauth information */
  oauth_info?: string,
  /** Plugin public parameters, key is the parameter position, value is the parameter list */
  common_params?: {
    [key: string | number]: commonParamSchema[]
  },
  /** Sub-authorization type, 0: api/token of service, 10: client credentials of oauth */
  sub_auth_type?: number,
  /** negligible */
  auth_payload?: string,
  /** negligible */
  fixed_export_ip: boolean,
}
export interface PluginIcon {
  uri: string,
  url: string,
}
export interface GetPlaygroundPluginListData {
  plugin_list: PluginInfoForPlayground[],
  total: number,
}
export interface PluginInfoForPlayground {
  id: string,
  /** name_for_human */
  name: string,
  /** description_for_human */
  desc_for_human: string,
  plugin_icon: string,
  plugin_type: PluginType,
  status: PluginStatus,
  auth: number,
  client_id: string,
  client_secret: string,
  plugin_apis: PluginApi[],
  /** plugin tag */
  tag: number,
  create_time: string,
  update_time: string,
  /** creator information */
  creator: Creator,
  /** Space ID */
  space_id: string,
  /** plugin statistics */
  statistic_data: PluginStatisticData,
  common_params?: {
    [key: string | number]: commonParamSchema[]
  },
  /** Product status of the plugin */
  plugin_product_status: ProductStatus,
  /** Plugin product removal type */
  plugin_product_unlist_type: ProductUnlistType,
  /** Material ID */
  material_id: string,
  /** Channel ID */
  channel_id: number,
  /** Plugin creation method */
  creation_method: CreationMethod,
  /** Is it an official plugin? */
  is_official: boolean,
  /** Project ID */
  project_id: string,
  /** Version number, millisecond timestamp */
  version_ts: string,
  /** version name */
  version_name: string,
}
export interface PluginApi {
  /** operationId */
  name: string,
  /** summary */
  desc: string,
  parameters: PluginParameter[],
  plugin_id: string,
  plugin_name: string,
  /** The serial number is the same as the playground */
  api_id: string,
  record_id: string,
  /** Card binding information, nil if not bound. */
  card_binding_info?: PresetCardBindingInfo,
  /** Debug API example */
  debug_example?: DebugExample,
  function_name?: string,
  /** operating mode */
  run_mode: RunMode,
}
export interface Creator {
  id: string,
  name: string,
  avatar_url: string,
  /** Did you create it yourself? */
  self: boolean,
  space_roly_type: SpaceRoleType,
  /** user name */
  user_unique_name: string,
  /** user tag */
  user_label: UserLabel,
}
export interface commonParamSchema {
  name: string,
  value: string,
}
export interface PluginParameter {
  name: string,
  desc: string,
  required: boolean,
  type: string,
  sub_parameters: PluginParameter[],
  /** If Type is an array, there is a subtype */
  sub_type: string,
  /** fromNodeId if the value of the imported parameter is a reference */
  from_node_id?: string,
  /** Which node's key is specifically referenced? */
  from_output?: string[],
  /** If the imported parameter is the user's hand input, put it here */
  value?: string,
  /** Format parameter */
  format?: PluginParamTypeFormat,
}
export interface PluginAPIInfo {
  plugin_id: string,
  api_id: string,
  name: string,
  desc: string,
  path: string,
  method: APIMethod,
  request_params: APIParameter[],
  response_params: APIParameter[],
  create_time: string,
  debug_status: APIDebugStatus,
  /** ignore */
  disabled: boolean,
  /** ignore */
  statistic_data: PluginStatisticData,
  /** if tool has been published, online_status is Online */
  online_status: OnlineStatus,
  /** ignore */
  api_extend: APIExtend,
  /** ignore */
  card_binding_info?: PresetCardBindingInfo,
  /** Debugging example */
  debug_example?: DebugExample,
  /** Debug sample state */
  debug_example_status: DebugExampleStatus,
  /** ignore */
  function_name: string,
}
export interface APIParameter {
  /** For the front end, no practical significance */
  id: string,
  /** parameter name */
  name: string,
  /** parameter desc */
  desc: string,
  /** parameter type */
  type: ParameterType,
  /** negligible */
  sub_type?: ParameterType,
  /** parameter location */
  location: ParameterLocation,
  /** Is it required? */
  is_required: boolean,
  /** sub-parameter */
  sub_parameters: APIParameter[],
  /** global default */
  global_default?: string,
  /** Is it enabled globally? */
  global_disable: boolean,
  /** Default value set in the smart body */
  local_default?: string,
  /** Is it enabled in the smart body? */
  local_disable: boolean,
  /** negligible */
  default_param_source?: DefaultParamSource,
  /** Reference variable key */
  variable_ref?: string,
  /** Multimodal auxiliary parameter types */
  assist_type?: AssistParameterType,
}
export interface PluginStatisticData {
  /** If it is empty, it will not be displayed. */
  bot_quote?: number
}
export interface APIExtend {
  /** Tool dimension authorization type */
  auth_mode: PluginToolAuthType
}
/** Plugin preset card binding information */
export interface PresetCardBindingInfo {
  card_id: string,
  card_version_num: string,
  status: PluginCardStatus,
  /** thumbnail */
  thumbnail: string,
}
export interface DebugExample {
  /** request example in json */
  req_example: string,
  /** response example in json */
  resp_example: string,
}
export interface UpdatePluginData {
  res: boolean,
  edit_version: number,
}
export interface GetUserAuthorityData {
  can_edit: boolean,
  can_read: boolean,
  can_delete: boolean,
  can_debug: boolean,
  can_publish: boolean,
  can_read_changelog: boolean,
}
/** authorization status */
export enum OAuthStatus {
  Authorized = 1,
  Unauthorized = 2,
}
export interface CheckAndLockPluginEditData {
  /** Is it occupied? */
  Occupied: boolean,
  /** If it is already occupied, return the user ID. */
  user: Creator,
  /** Was it successful? */
  Seized: boolean,
}
export interface PluginPublishInfo {
  /** publisher */
  publisher_id: string,
  /** Version, millisecond timestamp */
  version_ts: number,
  /** version name */
  version_name: string,
  /** version description */
  version_desc: string,
}
export enum DebugOperation {
  /** Debugging, the debugging state will be saved, and the return value will be checked. */
  Debug = 1,
  /** Parse only the return value structure */
  Parse = 2,
}
export interface RegisterPluginData {
  plugin_id: string,
  /** the same as the request 'openapi' */
  openapi: string,
}
export enum ScopeType {
  /** all */
  All = 0,
  /** self */
  Self = 1,
}
export enum OrderBy {
  CreateTime = 0,
  UpdateTime = 1,
  PublishTime = 2,
  Hot = 3,
}
export enum PluginTypeForFilter {
  /** Includes PLUGIN and APP. */
  CloudPlugin = 1,
  /** Include LOCAL */
  LocalPlugin = 2,
  /** Includes WORKFLOW and IMAGEFLOW */
  WorkflowPlugin = 3,
}
export enum PluginDataFormat {
  OpenAPI = 1,
  Curl = 2,
  Postman = 3,
  Swagger = 4,
}
export interface DuplicateAPIInfo {
  method: string,
  path: string,
  count: number,
}
export enum CodePluginRuntime {
  Python = 1,
  JavaScript = 2,
}
export enum CodePluginDebugStatus {
  Success = 1,
  RuntimeError = 2,
  Timeout = 3,
  Capacity = 4,
  OutputLimit = 5,
  Canceled = 6,
  Unavailable = 7,
}
export interface CodePluginFile {
  path: string,
  content: string,
  sha256?: string,
}
export interface CodePluginDraftData {
  plugin_id: string,
  space_id: string,
  runtime: CodePluginRuntime,
  entry_file: string,
  files: CodePluginFile[],
  revision: number,
  last_debugged_revision: number,
  debug_ready: boolean,
  input_schema_json: string,
  output_schema_json: string,
}
export interface CodePluginVersionData {
  plugin_id: string,
  space_id: string,
  version: string,
  runtime: CodePluginRuntime,
  entry_file: string,
  files: CodePluginFile[],
  source_revision: number,
  created_by: string,
  input_schema_json: string,
  output_schema_json: string,
}
export interface CodePluginDebugData {
  success: boolean,
  status: CodePluginDebugStatus,
  result: string,
  reason: string,
  duration_ms: number,
  output_bytes: number,
  revision: number,
}