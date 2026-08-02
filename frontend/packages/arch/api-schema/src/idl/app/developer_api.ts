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

import * as shortcut_command from './../playground/shortcut_command';
export { shortcut_command };
import * as bot_common from './bot_common';
export { bot_common };
import * as base from './../base';
export { base };
export interface DraftBotCreateRequest {
  space_id: string,
  name: string,
  description: string,
  icon_uri: string,
  visibility: VisibilityType,
  monetization_conf?: MonetizationConf,
  /** Create source navi: navbar space: space */
  create_from?: string,
  business_type?: bot_common.BusinessType,
}
export interface MonetizationConf {
  is_enable?: boolean
}
export enum VisibilityType {
  /** invisible */
  Invisible = 0,
  /** visible */
  Visible = 1,
}
export interface DraftBotCreateData {
  bot_id: string,
  /** True: The machine audit verification failed */
  check_not_pass: boolean,
  /** The machine audit verification failed the copy. */
  check_not_pass_msg?: string,
}
export interface DraftBotCreateResponse {
  code: number,
  msg: string,
  data: DraftBotCreateData,
}
export interface DeleteDraftBotRequest {
  space_id: string,
  bot_id: string,
}
export interface DeleteDraftBotData {}
export interface DeleteDraftBotResponse {
  code: number,
  msg: string,
  data: DeleteDraftBotData,
}
export interface DuplicateDraftBotRequest {
  space_id: string,
  bot_id: string,
}
export interface UserLabel {
  label_id: string,
  label_name: string,
  icon_uri: string,
  icon_url: string,
  jump_link: string,
}
export interface Creator {
  id: string,
  /** nickname */
  name: string,
  avatar_url: string,
  /** Did you create it yourself? */
  self: boolean,
  /** user name */
  user_unique_name: string,
  /** user tag */
  user_label: UserLabel,
}
export interface DuplicateDraftBotData {
  bot_id: string,
  name: string,
  user_info: Creator,
}
export interface DuplicateDraftBotResponse {
  code: number,
  msg: string,
  data: DuplicateDraftBotData,
}
export interface UpdateDraftBotDisplayInfoResponse {
  code: number,
  msg: string,
}
export interface DraftBotDisplayInfoData {
  tab_display_info?: TabDisplayItems
}
export interface UpdateDraftBotDisplayInfoRequest {
  bot_id: string,
  display_info?: DraftBotDisplayInfoData,
  space_id?: string,
}
/** draft bot display info */
export enum TabStatus {
  Default = 0,
  Open = 1,
  Close = 2,
  Hide = 3,
}
export interface TabDisplayItems {
  plugin_tab_status?: TabStatus,
  workflow_tab_status?: TabStatus,
  knowledge_tab_status?: TabStatus,
  database_tab_status?: TabStatus,
  variable_tab_status?: TabStatus,
  opening_dialog_tab_status?: TabStatus,
  scheduled_task_tab_status?: TabStatus,
  suggestion_tab_status?: TabStatus,
  tts_tab_status?: TabStatus,
  filebox_tab_status?: TabStatus,
  long_term_memory_tab_status?: TabStatus,
  answer_action_tab_status?: TabStatus,
  imageflow_tab_status?: TabStatus,
  background_image_tab_status?: TabStatus,
  shortcut_tab_status?: TabStatus,
  knowledge_table_tab_status?: TabStatus,
  knowledge_text_tab_status?: TabStatus,
  knowledge_photo_tab_status?: TabStatus,
  hook_info_tab_status?: TabStatus,
  default_user_input_tab_status?: TabStatus,
}
export interface GetDraftBotDisplayInfoResponse {
  code: number,
  msg: string,
  data: DraftBotDisplayInfoData,
}
export interface GetDraftBotDisplayInfoRequest {
  bot_id: string
}
export interface PublishDraftBotResponse {
  code: number,
  msg: string,
  data: PublishDraftBotData,
}
export interface PublishDraftBotData {
  /** Key represents connector_name enumeration Feishu = "feishu" -- obsolete */
  connector_bind_result: {
    [key: string | number]: ConnectorBindResult[]
  },
  /** The key represents connector_id, and the value is the published result */
  publish_result: {
    [key: string | number]: ConnectorBindResult
  },
  /** True: The machine audit verification failed */
  check_not_pass: boolean,
  /** Added bot marketing results */
  submit_bot_market_result?: SubmitBotMarketResult,
  /** In human moderation */
  hit_manual_check?: boolean,
  /** starlingKey list of reasons why the machine audit failed */
  not_pass_reason?: string[],
  /** Publish bot billing results */
  publish_monetization_result?: boolean,
}
export interface ConnectorBindResult {
  connector: Connector,
  /** The status code returned downstream of the publish call is not consumed by the front end. */
  code: number,
  /** Additional copy of the release status, the front end is parsed in markdown format */
  msg: string,
  /** post result status */
  publish_result_status?: PublishResultStatus,
}
export interface Connector {
  /** connector_name enumeration Feishu = "feishu" */
  name: string,
  app_id: string,
  app_secret: string,
  share_link: string,
  bind_info?: {
    [key: string | number]: string
  },
}
export enum PublishResultStatus {
  /** success */
  Success = 1,
  /** fail */
  Failed = 2,
  /** in approval */
  InReview = 3,
}
export interface SubmitBotMarketResult {
  /** Shelf status, 0-success */
  result_code?: number,
  /** msg */
  msg?: string,
}
export enum AgentType {
  Start_Agent = 0,
  LLM_Agent = 1,
  Task_Agent = 2,
  Global_Agent = 3,
  Bot_Agent = 4,
}
export interface AgentInfo {
  id?: string,
  agent_type?: AgentType,
  name?: string,
  position?: AgentPosition,
  icon_uri?: string,
  intents?: Intent[],
  work_info?: AgentWorkInfo,
  reference_id?: string,
  first_version?: string,
  current_version?: string,
  /** 1: Available update 2: Removed */
  reference_info_status?: ReferenceInfoStatus,
  description?: string,
  update_type?: ReferenceUpdateType,
}
export enum ReferenceInfoStatus {
  /** 1: Updates are available */
  HasUpdates = 1,
  /** 2: Deleted */
  IsDelete = 2,
}
export enum ReferenceUpdateType {
  ManualUpdate = 1,
  AutoUpdate = 2,
}
export interface AgentPosition {
  x: number,
  y: number,
}
export interface Intent {
  intent_id?: string,
  prompt?: string,
  next_agent_id?: string,
}
/** Information about each module in the agent workspace */
export interface AgentWorkInfo {
  /** The agent prompts the front-end information, the server does not need to perceive */
  prompt?: string,
  /** model configuration */
  other_info?: string,
  /** Plugin information */
  tools?: string,
  /** Dataset information */
  dataset?: string,
  /** Workflow information */
  workflow?: string,
  /** system_info_all with bot */
  system_info_all?: string,
  /** backtrack configuration */
  jump_config?: JumpConfig,
  /** Referral Configuration */
  suggest_reply?: string,
  /** Hook configuration */
  hook_info?: string,
}
export interface JumpConfig {
  backtrack: BacktrackMode,
  recognition: RecognitionMode,
  independent_conf?: IndependentModeConfig,
}
export enum BacktrackMode {
  Current = 1,
  Previous = 2,
  Start = 3,
  MostSuitable = 4,
}
export enum RecognitionMode {
  FunctionCall = 1,
  Independent = 2,
}
export enum IndependentTiming {
  /** Determine user input (front) */
  Pre = 1,
  /** Determine node output (postfix) */
  Post = 2,
  /** Front mode and rear mode support simultaneous selection */
  PreAndPost = 3,
}
export enum IndependentRecognitionModelType {
  /** Small model */
  SLM = 0,
  /** Large model */
  LLM = 1,
}
export interface IndependentModeConfig {
  /** Judge timing */
  judge_timing: IndependentTiming,
  history_round: number,
  model_type: IndependentRecognitionModelType,
  model_id?: string,
  prompt?: string,
}
export interface BotTagInfo {
  bot_id: number,
  /** time_capsule */
  key: string,
  /** TimeCapsuleInfo json */
  value: string,
  version: number,
}
export interface PublishDraftBotRequest {
  space_id: string,
  bot_id: string,
  work_info: WorkInfo,
  /** Key represents connector_name enumeration Feishu = "feishu" -- obsolete */
  connector_list: {
    [key: string | number]: Connector[]
  },
  /** The key represents connector_id, and the value is the published parameter */
  connectors: {
    [key: string | number]: {
      [key: string | number]: string
    }
  },
  /** Default 0 */
  botMode?: BotMode,
  agents?: AgentInfo[],
  canvas_data?: string,
  bot_tag_info?: BotTagInfo[],
  /** Configuration published to the market */
  submit_bot_market_config?: SubmitBotMarketConfig,
  publish_id?: string,
  /** Specify the release of a CommitVersion */
  commit_version?: string,
  /** Release type, online release/pre-release */
  publish_type?: PublishType,
  /** Pre-release other information */
  pre_publish_ext?: string,
  /** Replace the history_info in the original workinfo */
  history_info?: string,
}
export enum PublishType {
  OnlinePublish = 0,
  PrePublish = 1,
}
export interface SubmitBotMarketConfig {
  /** Whether to publish to the market */
  need_submit?: boolean,
  /** Is it open source? */
  open_source?: boolean,
  /** classification */
  category_id?: string,
}
export enum BotMode {
  SingleMode = 0,
  MultiMode = 1,
  WorkflowMode = 2,
}
/** Information for each module in the workspace */
export interface WorkInfo {
  message_info?: string,
  prompt?: string,
  variable?: string,
  other_info?: string,
  history_info?: string,
  tools?: string,
  system_info_all?: string,
  dataset?: string,
  onboarding?: string,
  profile_memory?: string,
  table_info?: string,
  workflow?: string,
  task?: string,
  suggest_reply?: string,
  tts?: string,
  background_image_info_list?: string,
  /** Quick Instruction */
  shortcuts?: shortcut_command.ShortcutStruct,
  /** Hook configuration */
  hook_info?: string,
  /** User query collection configuration */
  user_query_collect_conf?: UserQueryCollectConf,
  /** Workflow pattern orchestration data */
  layout_info?: LayoutInfo,
}
export interface UserQueryCollectConf {
  /** Whether to turn on the collection switch */
  is_collected: boolean,
  /** Privacy Policy Link */
  private_policy: string,
}
export interface LayoutInfo {
  /** workflowId */
  workflow_id: string,
  /** PluginId */
  plugin_id: string,
}
export enum HistoryType {
  /** abandoned */
  SUBMIT = 1,
  /** publish */
  FLAG = 2,
  /** submit */
  COMMIT = 4,
  /** Submit and publish */
  COMMITANDFLAG = 5,
}
export interface ListDraftBotHistoryRequest {
  space_id: string,
  bot_id: string,
  page_index: number,
  page_size: number,
  history_type: HistoryType,
  connector_id?: string,
}
export interface ListDraftBotHistoryResponse {
  code: number,
  msg: string,
  data: ListDraftBotHistoryData,
}
export interface ListDraftBotHistoryData {
  history_infos: HistoryInfo[],
  total: number,
}
/** If historical information is preserved */
export interface HistoryInfo {
  version: string,
  history_type: HistoryType,
  /** Additional information added to the historical record */
  info: string,
  create_time: string,
  connector_infos: ConnectorInfo[],
  creator: Creator,
  publish_id?: string,
  /** Instructions to fill in when submitting */
  commit_remark?: string,
}
export interface ConnectorInfo {
  id: string,
  name: string,
  icon: string,
  connector_status: ConnectorDynamicStatus,
  share_link?: string,
}
export enum ConnectorDynamicStatus {
  Normal = 0,
  Offline = 1,
  TokenDisconnect = 2,
}
export enum IconType {
  Bot = 1,
  User = 2,
  Plugin = 3,
  Dataset = 4,
  Space = 5,
  Workflow = 6,
  Imageflow = 7,
  Society = 8,
  Connector = 9,
  ChatFlow = 10,
  Voice = 11,
  Enterprise = 12,
}
export interface GetIconRequest {
  icon_type: IconType
}
export interface Icon {
  url: string,
  uri: string,
}
export interface GetIconResponseData {
  icon_list: Icon[]
}
export interface GetIconResponse {
  code: number,
  msg: string,
  data: GetIconResponseData,
}
export interface GetUploadAuthTokenResponse {
  code: number,
  msg: string,
  data: GetUploadAuthTokenData,
}
export interface GetUploadAuthTokenData {
  service_id: string,
  upload_path_prefix: string,
  auth: UploadAuthTokenInfo,
  upload_host: string,
  schema: string,
}
export interface UploadAuthTokenInfo {
  access_key_id: string,
  secret_access_key: string,
  session_token: string,
  expired_time: string,
  current_time: string,
}
export interface GetUploadAuthTokenRequest {
  scene: string,
  data_type: string,
}
export interface UploadFileRequest {
  /** Document related description */
  file_head: CommonFileInfo,
  /** file data */
  data: string,
}
/** Upload file, file header */
export interface CommonFileInfo {
  /** File type, suffix */
  file_type: string,
  /** business type */
  biz_type: FileBizType,
}
export enum FileBizType {
  BIZ_UNKNOWN = 0,
  BIZ_BOT_ICON = 1,
  BIZ_BOT_DATASET = 2,
  BIZ_DATASET_ICON = 3,
  BIZ_PLUGIN_ICON = 4,
  BIZ_BOT_SPACE = 5,
  BIZ_BOT_WORKFLOW = 6,
  BIZ_SOCIETY_ICON = 7,
  BIZ_CONNECTOR_ICON = 8,
  BIZ_LIBRARY_VOICE_ICON = 9,
  BIZ_ENTERPRISE_ICON = 10,
}
export interface UploadFileResponse {
  code: number,
  msg: string,
  /** data */
  data: UploadFileData,
}
export interface GetTypeListRequest {
  model?: boolean,
  voice?: boolean,
  raw_model?: boolean,
  space_id?: string,
  /** The model ID used by the current bot to handle issues that cannot be displayed by the bot model synchronized by cici/doubao */
  cur_model_id?: string,
  /** Compatible with MultiAgent, with multiple cur_model_id */
  cur_model_ids?: string[],
  /** model scenario */
  model_scene?: ModelScene,
}
export enum ModelScene {
  Douyin = 1,
}
export enum ModelClass {
  GPT = 1,
  SEED = 2,
  Claude = 3,
  /** name: MiniMax */
  MiniMax = 4,
  Plugin = 5,
  StableDiffusion = 6,
  ByteArtist = 7,
  Maas = 9,
  /** Abandoned: Qianfan (Baidu Cloud) */
  QianFan = 10,
  /** name：Google Gemini */
  Gemini = 11,
  /** name: Moonshot */
  Moonshot = 12,
  /** Name: Zhipu */
  GLM = 13,
  /** Name: Volcano Ark */
  MaaSAutoSync = 14,
  /** Name: Tongyi Qianwen */
  QWen = 15,
  /** name: Cohere */
  Cohere = 16,
  /** Name: Baichuan Intelligent */
  Baichuan = 17,
  /** Name: ERNIE Bot */
  Ernie = 18,
  /** Name: Magic Square */
  DeekSeek = 19,
  /** name: Llama */
  Llama = 20,
  StepFun = 23,
  Other = 999,
}
export interface ModelQuota {
  /** Maximum total number of tokens */
  token_limit: number,
  /** Final reply maximum number of tokens */
  token_resp: number,
  /** Prompt system maximum number of tokens */
  token_system: number,
  /** Prompt user to enter maximum number of tokens */
  token_user_in: number,
  /** Prompt tool to enter maximum number of tokens */
  token_tools_in: number,
  /** Prompt tool output maximum number of tokens */
  token_tools_out: number,
  /** Prompt data maximum number of tokens */
  token_data: number,
  /** Prompt history maximum number of tokens */
  token_history: number,
  /** Prompt history maximum number of tokens */
  token_cut_switch: boolean,
  /** input cost */
  price_in: number,
  /** output cost */
  price_out: number,
  /** Systemprompt input restrictions, if not passed, no input restrictions */
  system_prompt_limit?: number,
}
export enum ModelTagClass {
  ModelType = 1,
  ModelUserRight = 2,
  ModelFeature = 3,
  ModelFunction = 4,
  /** Do not do this issue */
  Custom = 20,
  Others = 100,
}
export enum ModelParamType {
  Float = 1,
  Int = 2,
  Boolean = 3,
  String = 4,
}
export interface ModelParamDefaultValue {
  default_val: string,
  creative?: string,
  balance?: string,
  precise?: string,
}
export interface ModelParamClass {
  /** 1="Generation diversity", 2="Input and output length", 3="Output format" */
  class_id: number,
  label: string,
}
export interface Option {
  /** The value displayed by the option */
  label: string,
  /** Filled in value */
  value: string,
}
export interface ModelParameter {
  /** Configuration fields, such as max_tokens */
  name: string,
  /** Configure field display name */
  label: string,
  /** Configuration field detail description */
  desc: string,
  /** type */
  type: ModelParamType,
  /** Numerical type parameters, the minimum value allowed to be set */
  min: string,
  /** Numerical type parameter, the maximum value allowed to be set */
  max: string,
  /** Precision of float type parameters */
  precision: number,
  /** Parameter default {"default": xx, "creative": xx} */
  default_val: ModelParamDefaultValue,
  /** Enumeration values such as response_format support text, markdown, json */
  options: Option[],
  /** Parameter classification, "Generation diversity", "Input and output length", "Output format" */
  param_class: ModelParamClass,
}
export interface ModelDescGroup {
  group_name: string,
  desc: string[],
}
export interface ModelTag {
  tag_name: string,
  tag_class: ModelTagClass,
  tag_icon: string,
  tag_descriptions: string,
}
export interface ModelSeriesInfo {
  series_name: string,
  icon_url: string,
  model_vendor: string,
  model_tips?: string,
}
export enum ModelTagValue {
  Flagship = 1,
  HighSpeed = 2,
  ToolInvocation = 3,
  RolePlaying = 4,
  LongText = 5,
  ImageUnderstanding = 6,
  Reasoning = 7,
  VideoUnderstanding = 8,
  CostPerformance = 9,
  CodeSpecialization = 10,
  AudioUnderstanding = 11,
}
export interface ModelStatusDetails {
  /** Is it a new model? */
  is_new_model: boolean,
  /** Is it a high-level model? */
  is_advanced_model: boolean,
  /** Is it a free model? */
  is_free_model: boolean,
  /** Will it be removed from the shelves soon? */
  is_upcoming_deprecated: boolean,
  /** removal date */
  deprecated_date: string,
  /** Remove the replacement model from the shelves. */
  replace_model_name: string,
  /** Recently updated information */
  update_info: string,
  /** Model Features */
  model_feature: ModelTagValue,
}
export interface ModelAbility {
  /** Do you want to show cot? */
  cot_display?: boolean,
  /** Supports function calls */
  function_call?: boolean,
  /** Does it support picture understanding? */
  image_understanding?: boolean,
  /** Does it support video understanding? */
  video_understanding?: boolean,
  /** Does it support audio understanding? */
  audio_understanding?: boolean,
  /** Does it support multimodality? */
  support_multi_modal?: boolean,
  /** Whether to support continuation */
  prefill_resp?: boolean,
}
export interface Model {
  name: string,
  model_type: number,
  model_class: ModelClass,
  /** Model icon url */
  model_icon: string,
  model_input_price: number,
  model_output_price: number,
  model_quota: ModelQuota,
  /** Model real name, front-end calculation token */
  model_name: string,
  model_class_name: string,
  is_offline: boolean,
  model_params: ModelParameter[],
  model_desc?: ModelDescGroup[],
  /** model function configuration */
  func_config?: {
    [key: string | number]: bot_common.ModelFuncConfigStatus
  },
  /** Ark model node name */
  endpoint_name?: string,
  /** model label */
  model_tag_list?: ModelTag[],
  /** User prompt must have and cannot be empty */
  is_up_required?: boolean,
  /** Model brief description */
  model_brief_desc: string,
  /** Model series */
  model_series: ModelSeriesInfo,
  /** model state */
  model_status_details: ModelStatusDetails,
  /** model capability */
  model_ability: ModelAbility,
}
export interface VoiceType {
  id: number,
  model_name: string,
  name: string,
  language: string,
  style_id: string,
  style_name: string,
}
export interface GetTypeListData {
  model_list: Model[],
  voice_list: VoiceType[],
  raw_model_list: Model[],
}
export interface GetTypeListResponse {
  code: number,
  msg: string,
  data: GetTypeListData,
}
export interface UploadFileData {
  /** File URL */
  upload_url: string,
  /** File URI, submit using this */
  upload_uri: string,
}
export interface UpdateUserProfileCheckRequest {
  user_unique_name?: string
}
export interface UpdateUserProfileCheckResponse {
  code: number,
  msg: string,
}
export enum CommitStatus {
  Undefined = 0,
  /** It is the latest, the same as the main draft */
  Uptodate = 1,
  /** Behind the main draft */
  Behind = 2,
  /** No personal draft */
  NoDraftReplica = 3,
}
export interface Committer {
  id?: string,
  name?: string,
  commit_time?: string,
}
/** Check if the draft can be submitted and returned. */
export interface CheckDraftBotCommitResponse {
  code?: number,
  msg?: string,
  data?: CheckDraftBotCommitData,
}
export interface CheckDraftBotCommitData {
  status?: CommitStatus,
  /** master draft version */
  base_commit_version?: string,
  /** Master Draft Submission Information */
  base_committer?: Committer,
  /** Personal draft version */
  commit_version?: string,
}
/** Check if the draft can be submitted to the request */
export interface CheckDraftBotCommitRequest {
  space_id: string,
  bot_id: string,
  commit_version?: string,
}
export interface GetOnboardingRequest {
  bot_id: string,
  bot_prompt: string,
}
export interface GetOnboardingResponseData {
  onboarding_content: OnboardingContent
}
export interface GetOnboardingResponse {
  code: number,
  msg: string,
  data: GetOnboardingResponseData,
}
export interface OnboardingContent {
  /** opening statement */
  prologue?: string,
  /** suggestion question */
  suggested_questions?: string[],
}
export enum ConfigStatus {
  /** Configured */
  Configured = 1,
  /** Not configured */
  NotConfigured = 2,
  /** Token changes */
  Disconnected = 3,
  /** Configuring, authorizing */
  Configuring = 4,
  /** Need to reconfigure */
  NeedReconfiguring = 5,
}
export enum BindType {
  /** No binding required */
  NoBindRequired = 1,
  /** Auth binding */
  AuthBind = 2,
  /** Kv binding = */
  KvBind = 3,
  /** Kv and Auth authorization */
  KvAuthBind = 4,
  /** API channel binding */
  ApiBind = 5,
  WebSDKBind = 6,
  StoreBind = 7,
  /** One button each for authorization and configuration */
  AuthAndConfig = 8,
}
export enum AllowPublishStatus {
  Allowed = 0,
  Forbid = 1,
}
export interface AuthLoginInfo {
  app_id: string,
  response_type: string,
  authorize_url: string,
  scope: string,
  client_id: string,
  duration: string,
  aid: string,
  client_key: string,
}
export enum BotConnectorStatus {
  /** Normal */
  Normal = 0,
  /** Under review. */
  InReview = 1,
  /** offline */
  Offline = 2,
}
export enum UserAuthStatus {
  /** Authorized */
  Authorized = 1,
  /** unauthorized */
  UnAuthorized = 2,
  /** Authorizing */
  Authorizing = 3,
}
export interface PublishConnectorListRequest {
  space_id: string,
  bot_id: string,
  commit_version?: string,
}
export interface PublishConnectorInfo {
  /** Publishing Platform connector_id */
  id: string,
  /** publishing platform name */
  name: string,
  /** publishing platform icon */
  icon: string,
  /** Publish Platform Description */
  desc: string,
  /** share link */
  share_link: string,
  /** Configuration Status 1: Bound 2: Unbound */
  config_status: ConfigStatus,
  /** Last Post */
  last_publish_time: number,
  /** Binding type 1: No binding required 2: Auth 3: kv value */
  bind_type: BindType,
  /** Binding information key field name value is value */
  bind_info: {
    [key: string | number]: string
  },
  /** Bind id information for unbinding and use */
  bind_id?: string,
  /** user authorization login information */
  auth_login_info?: AuthLoginInfo,
  /** Is it the last release? */
  is_last_published?: boolean,
  /** bot channel status */
  connector_status?: BotConnectorStatus,
  /** Privacy Policy */
  privacy_policy?: string,
  /** User Agreement */
  user_agreement?: string,
  /** Is the channel allowed to publish? */
  allow_punish?: AllowPublishStatus,
  /** Reason for not allowing posting */
  not_allow_reason?: string,
  /** Configuration status toast */
  config_status_toast?: string,
  /** Brand ID */
  brand_id?: number,
  /** Support commercialization */
  support_monetization?: boolean,
  /** 1: Authorized, 2: Unauthorized. Currently this field is only available bind_type == 8 */
  auth_status?: UserAuthStatus,
  /** URL of the complete info button */
  to_complete_info_url?: string,
}
export interface SubmitBotMarketOption {
  /** Is it possible to publicly orchestrate? */
  can_open_source?: boolean
}
export interface ConnectorBrandInfo {
  id: number,
  name: string,
  icon: string,
}
export interface PublishTips {
  /** cost-bearing reminder */
  cost_tips?: string
}
export interface PublishConnectorListResponse {
  code: number,
  msg: string,
  publish_connector_list: PublishConnectorInfo[],
  submit_bot_market_option?: SubmitBotMarketOption,
  /** The configuration of the last submitted market */
  last_submit_config?: SubmitBotMarketConfig,
  /** Channel brand information */
  connector_brand_info_map: {
    [key: string | number]: ConnectorBrandInfo
  },
  /** post alert */
  publish_tips?: PublishTips,
}
export interface PluginOauthAuthorizationCodeReq {
  code: string,
  state: string,
  plugin_id: string,
}
export interface PluginOauthAuthorizationCodeResp {
  code: number,
  msg: string,
}
export interface PluginOauthInfo {
  connector_id: string,
  plugin_id: string,
  plugin_name: string,
  plugin_icon: string,
  username: string,
  plugin_url: string,
  connector_name: string,
}
export interface PluginOauthInfoReq {
  confirm_code: string
}
export interface PluginOauthInfoResp {
  code: number,
  msg: string,
  data: PluginOauthInfo,
}
export interface PluginOauthConfirmReq {
  confirm_code: string
}
export interface PluginOauthConfirmResp {
  code: number,
  msg: string,
}
