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

import * as prompt_resource from './prompt_resource';
export { prompt_resource };
import * as shortcut_command from './shortcut_command';
export { shortcut_command };
import * as bot_common from './../app/bot_common';
export { bot_common };
import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export interface UpdateDraftBotInfoAgwResponse {
  data: UpdateDraftBotInfoAgwData,
  code: number,
  msg: string,
}
export interface UpdateDraftBotInfoAgwData {
  /** Is there any change? */
  has_change?: boolean,
  /** True: The machine audit verification failed */
  check_not_pass: boolean,
  /** Which branch is it currently on? */
  branch?: Branch,
  same_with_online?: boolean,
  /** The machine audit verification failed the copy. */
  check_not_pass_msg?: string,
}
/** branch */
export enum Branch {
  Undefined = 0,
  /** draft */
  PersonalDraft = 1,
  /** Space draft */
  Base = 2,
  /** Online version, used in diff scenarios */
  Publish = 3,
}
export interface UpdateDraftBotInfoAgwRequest {
  bot_info?: bot_common.BotInfoForUpdate,
  base_commit_version?: string,
}
export interface GetDraftBotInfoAgwRequest {
  /** Draft bot_id */
  bot_id: string,
  /** Check the history, the id of the historical version, corresponding to the id of the bot_draft_history */
  version?: string,
  /** Query specifies commit_version version, pre-release use, seems to be the same thing as version, but the acquisition logic is different */
  commit_version?: string,
}
export interface GetDraftBotInfoAgwResponse {
  data: GetDraftBotInfoAgwData,
  code: number,
  msg: string,
}
export interface GetDraftBotInfoAgwData {
  /** core bot data */
  bot_info: bot_common.BotInfo,
  /** bot option information */
  bot_option_data?: BotOptionData,
  /** Are there any unpublished changes? */
  has_unpublished_change?: boolean,
  /** The product status after the bot is put on the shelves */
  bot_market_status?: BotMarketStatus,
  /** Is the bot in multiplayer cooperative mode? */
  in_collaboration?: boolean,
  /** Is the content committed consistent with the online content? */
  same_with_online?: boolean,
  /** For frontend, permission related, can the current user edit this bot */
  editable?: boolean,
  /** For frontend, permission related, can the current user delete this bot */
  deletable?: boolean,
  /** Is the publisher of the latest release version */
  publisher?: UserInfo,
  /** Has it been published? */
  has_publish: boolean,
  /** Space ID */
  space_id: string,
  /** Published business line details */
  connectors: BotConnectorInfo[],
  /** What branch did you get the content of? */
  branch?: Branch,
  /** If branch=PersonalDraft, the version number of checkout/rebase; if branch = base, the committed version */
  commit_version?: string,
  /** For the front end, the most recent author */
  committer_name?: string,
  /** For frontend, commit time */
  commit_time?: string,
  /** For frontend, release time */
  publish_time?: string,
  /** Multi-person collaboration related operation permissions */
  collaborator_status?: BotCollaboratorStatus,
  /** Details of the most recent review */
  latest_audit_info?: AuditInfo,
  /** Douyin's doppelganger bot will have appId. */
  app_id?: string,
}
export interface BotOptionData {
  /** model details */
  model_detail_map?: {
    [key: string | number]: ModelDetail
  },
  /** plugin details */
  plugin_detail_map?: {
    [key: string | number]: PluginDetal
  },
  /** Plugin API Details */
  plugin_api_detail_map?: {
    [key: string | number]: PluginAPIDetal
  },
  /** Workflow Details */
  workflow_detail_map?: {
    [key: string | number]: WorkflowDetail
  },
  /** Knowledge Details */
  knowledge_detail_map?: {
    [key: string | number]: KnowledgeDetail
  },
  /** Quick command list */
  shortcut_command_list?: shortcut_command.ShortcutCommand[],
}
export interface ModelDetail {
  /** Model display name (to the user) */
  name?: string,
  /** Model name (for internal) */
  model_name?: string,
  /** Model ID */
  model_id?: string,
  /** Model Category */
  model_family?: number,
  /** IconURL */
  model_icon_url?: string,
}
export interface PluginDetal {
  id?: string,
  name?: string,
  description?: string,
  icon_url?: string,
  plugin_type?: string,
  plugin_status?: string,
  is_official?: boolean,
  /**  */
  plugin_from?: bot_common.PluginFrom,
}
export interface PluginAPIDetal {
  id?: string,
  name?: string,
  description?: string,
  parameters?: PluginParameter[],
  plugin_id?: string,
}
export interface PluginParameter {
  name?: string,
  description?: string,
  is_required?: boolean,
  type?: string,
  sub_parameters?: PluginParameter[],
  /** If Type is an array, there is a subtype */
  sub_type?: string,
  assist_type?: number,
}
export interface WorkflowDetail {
  id?: string,
  name?: string,
  description?: string,
  icon_url?: string,
  status?: number,
  /** Type 1: Official Template */
  type?: number,
  /** Plugin ID corresponding to workfklow */
  plugin_id?: string,
  is_official?: boolean,
  api_detail?: PluginAPIDetal,
}
export interface KnowledgeDetail {
  id?: string,
  name?: string,
  icon_url?: string,
  format_type: DataSetType,
}
export enum DataSetType {
  /** Text */
  Text = 0,
  /** table */
  Table = 1,
  /** image */
  Image = 2,
}
export enum BotMarketStatus {
  /** offline */
  Offline = 0,
  /** put on the shelves */
  Online = 1,
}
export interface UserInfo {
  /** user id */
  user_id: string,
  /** user name */
  name: string,
  /** user icon */
  icon_url: string,
}
export interface BotConnectorInfo {
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
export interface BotCollaboratorStatus {
  /** Can the current user submit? */
  commitable: boolean,
  /** Is the current user operable? */
  operateable: boolean,
  /** Can the current user manage collaborators? */
  manageable: boolean,
}
export interface AuditInfo {
  audit_status?: AuditStatus,
  publish_id?: string,
  commit_version?: string,
}
export interface AuditResult {
  AuditStatus: AuditStatus,
  AuditMessage: string,
}
export enum AuditStatus {
  /** Under review. */
  Auditing = 0,
  /** approved */
  Success = 1,
  /** audit failed */
  Failed = 2,
}
/** Onboarding json structure */
export interface OnboardingContent {
  /** Introductory remarks (C-end usage scenarios, only 1; background scenarios, possibly multiple) */
  prologue?: string,
  /** suggestion question */
  suggested_questions?: string[],
  suggested_questions_show_mode?: bot_common.SuggestedQuestionsShowMode,
}
export enum ScopeType {
  /** All under the enterprise (effective under the enterprise) */
  All = 0,
  /** I joined (both companies and individuals are valid, no default self is passed on) */
  Self = 1,
}
export interface GetSpaceListV2Request {
  /** Search term */
  search_word?: string,
  /** Enterprise ID */
  enterprise_id?: string,
  /** organization id */
  organization_id?: string,
  /** range type */
  scope_type?: ScopeType,
  /** paging information */
  page?: number,
  /** Paging size -- if page and size are not passed on, it is considered not paging */
  size?: number,
}
export enum SpaceType {
  /** individual */
  Personal = 1,
  /** group */
  Team = 2,
}
export enum SpaceMode {
  Normal = 0,
  DevMode = 1,
}
export enum SpaceTag {
  /** Professional Edition */
  Professional = 1,
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
/** Application management list */
export enum SpaceApplyStatus {
  /** all */
  All = 0,
  /** Joined */
  Joined = 1,
  /** Confirming */
  Confirming = 2,
  /** Rejected */
  Rejected = 3,
}
export interface AppIDInfo {
  id: string,
  name: string,
  icon: string,
}
export interface ConnectorInfo {
  id: string,
  name: string,
  icon: string,
}
export interface BotSpaceV2 {
  /** Space id, newly created as 0 */
  id: string,
  /** publishing platform */
  app_ids: AppIDInfo[],
  /** space name */
  name: string,
  /** spatial description */
  description: string,
  /** icon url */
  icon_url: string,
  /** space type */
  space_type: SpaceType,
  /** publishing platform */
  connectors: ConnectorInfo[],
  /** Whether to hide New, Copy Delete buttons */
  hide_operation: boolean,
  /** Role in team 1-owner 2-admin 3-member */
  role_type: number,
  /** Spatial Mode */
  space_mode?: SpaceMode,
  /** Whether to display the end-side plug-in creation entry */
  display_local_plugin: boolean,
  /** Role type, enumeration */
  space_role_type: SpaceRoleType,
  /** spatial label */
  space_tag?: SpaceTag,
  /** Enterprise ID */
  enterprise_id?: string,
  /** organization id */
  organization_id?: string,
  /** Space owner uid */
  owner_user_id?: string,
  /** Space owner nickname */
  owner_name?: string,
  /** Space owner username */
  owner_user_name?: string,
  /** Space owner image */
  owner_icon_url?: string,
  /** The current visiting user joins the space status */
  space_apply_status?: SpaceApplyStatus,
  /** The total number of space members, only the organization space can be queried. */
  total_member_num?: number,
  /** Whether AppDev is enabled for the workspace. */
  allow_develop: boolean,
  /** Whether the workspace accepts published resources. */
  receive_publish: boolean,
}
export interface SpaceInfo {
  /** User joins space list */
  bot_space_list: BotSpaceV2[],
  /** Is there any personal space available? */
  has_personal_space: boolean,
  /** Number of team spaces created by individuals */
  team_space_num: number,
  /** The maximum number of spaces an individual can create */
  max_team_space_num: number,
  /** list of recently used spaces */
  recently_used_space_list: BotSpaceV2[],
  /** Effective when paging */
  total?: number,
  /** Effective when paging */
  has_more?: boolean,
}
export interface GetSpaceListV2Response {
  data: SpaceInfo,
  code: number,
  msg: string,
}
export interface GetImagexShortUrlResponse {
  data: GetImagexShortUrlData,
  code: number,
  msg: string,
}
export interface GetImagexShortUrlData {
  /** Audit status, key uri, value url and, audit status */
  url_info: {
    [key: string | number]: UrlInfo
  }
}
export interface UrlInfo {
  url: string,
  review_status: boolean,
}
export enum GetImageScene {
  Onboarding = 0,
  BackgroundImage = 1,
}
export interface GetImagexShortUrlRequest {
  uris: string[],
  scene: GetImageScene,
}
export interface UserBasicInfo {
  user_id: string,
  /** nickname */
  user_name: string,
  /** avatar */
  user_avatar: string,
  /** user name */
  user_unique_name?: string,
  /** user tag */
  user_label?: bot_common.UserLabel,
  /** user creation time */
  create_time?: number,
}
export interface MGetUserBasicInfoRequest {
  user_ids: string[],
  need_user_status?: boolean,
  /** Whether enterprise authentication information is required, the default is true when the front end is called through AGW */
  need_enterprise_identity?: boolean,
  /** Do you need a volcano username? */
  need_volcano_user_name?: boolean,
}
export interface MGetUserBasicInfoResponse {
  id_user_info_map?: {
    [key: string | number]: UserBasicInfo
  },
  code: number,
  msg: string,
}
export interface GetBotPopupInfoRequest {
  bot_popup_types: BotPopupType[],
  bot_id: string,
}
export interface GetBotPopupInfoResponse {
  data: BotPopupInfoData,
  code: number,
  msg: string,
}
export interface BotPopupInfoData {
  bot_popup_count_info: {
    [key: string | number]: number
  }
}
export enum BotPopupType {
  AutoGenBeforePublish = 1,
}
export interface UpdateBotPopupInfoResponse {
  code: number,
  msg: string,
}
export interface UpdateBotPopupInfoRequest {
  bot_popup_type: BotPopupType,
  bot_id: string,
}
export interface ReportUserBehaviorRequest {
  resource_id: string,
  resource_type: SpaceResourceType,
  behavior_type: BehaviorType,
  /** This requirement must be passed on */
  space_id?: string,
}
export interface ReportUserBehaviorResponse {
  code: number,
  msg: string,
}
export enum SpaceResourceType {
  DraftBot = 1,
  Project = 2,
  Space = 3,
  DouyinAvatarBot = 4,
}
export enum BehaviorType {
  Visit = 1,
  Edit = 2,
}
export interface FileInfo {
  url: string,
  uri: string,
}
export enum GetFileUrlsScene {
  shorcutIcon = 1,
}
export interface GetFileUrlsRequest {
  scene: GetFileUrlsScene
}
export interface GetFileUrlsResponse {
  file_list: FileInfo[],
  code: number,
  msg: string,
}
export enum NoticeRankType {
  All = 0,
  Unread = 1,
}
export enum NoticeSenderType {
  Bot = 1,
}
export enum ReadStatus {
  Unread = 1,
  Read = 2,
}
export enum NoticeSeverity {
  Info = 1,
  Success = 2,
  Warning = 3,
  Error = 4,
}
export enum NoticeCategory {
  Task = 1,
  ScheduledTask = 2,
  AppDev = 3,
  MCP = 4,
  Resource = 5,
  Workspace = 6,
  IM = 7,
  Billing = 8,
  System = 9,
}
export enum NoticeRoute {
  None = 0,
  TaskThread = 1,
  ScheduledTaskCenter = 2,
  AppDev = 3,
  Skill = 4,
  Workspace = 5,
  Billing = 6,
  SystemAnnouncements = 7,
}
export enum AdminAnnouncementRouteType {
  None = 0,
  WorkspaceHome = 1,
  SystemAnnouncements = 2,
}
export interface AdminAnnouncementRoute {
  type: AdminAnnouncementRouteType,
  space_id?: string,
}
export interface AdminAnnouncementAudience {
  type: string,
  target_ids?: string[],
}
export interface AdminAnnouncement {
  id: string,
  title: string,
  body: string,
  severity: string,
  route: AdminAnnouncementRoute,
  audience: AdminAnnouncementAudience,
  status: string,
  projection_status: string,
  scheduled_at?: string,
  publish_requested_at?: string,
  snapshot_at?: string,
  published_at?: string,
  cancelled_at?: string,
  created_by: string,
  updated_by: string,
  recipient_count: number,
  projected_count: number,
  last_error_code?: string,
  version: number,
  created_at: string,
  updated_at: string,
}
export interface AdminAnnouncementAuditEvent {
  id: string,
  announcement_id: string,
  actor_id: string,
  action: string,
  from_status?: string,
  to_status?: string,
  projection_status: string,
  result: string,
  error_code?: string,
  recipient_count: number,
  projected_count: number,
  created_at: string,
}
export interface AdminAnnouncementData {
  announcement: AdminAnnouncement,
  replayed?: boolean,
}
export interface AdminAnnouncementListData {
  announcements: AdminAnnouncement[],
  total: number,
}
export interface AdminAnnouncementPublicationData {
  announcement: AdminAnnouncement,
  deferred: boolean,
  error_code?: string,
  replayed?: boolean,
}
export interface AdminAnnouncementReplayData {
  processed: number,
  completed: number,
  failed: number,
  deferred: number,
  error_codes?: {
    [key: string | number]: number
  },
}
export interface AdminAnnouncementAuditListData {
  audit_events: AdminAnnouncementAuditEvent[],
  total: number,
}
export interface AdminAnnouncementResponse {
  code: number,
  msg: string,
  error_code?: string,
  data?: AdminAnnouncementData,
}
export interface AdminAnnouncementListResponse {
  code: number,
  msg: string,
  error_code?: string,
  data?: AdminAnnouncementListData,
}
export interface AdminAnnouncementPublicationResponse {
  code: number,
  msg: string,
  error_code?: string,
  data?: AdminAnnouncementPublicationData,
}
export interface AdminAnnouncementReplayResponse {
  code: number,
  msg: string,
  error_code?: string,
  data?: AdminAnnouncementReplayData,
}
export interface AdminAnnouncementAuditListResponse {
  code: number,
  msg: string,
  error_code?: string,
  data?: AdminAnnouncementAuditListData,
}
export interface CreateAdminAnnouncementRequest {
  title: string,
  body: string,
  severity: string,
  route: AdminAnnouncementRoute,
  audience: AdminAnnouncementAudience,
  idempotency_key: string,
}
export interface ListAdminAnnouncementsRequest {
  status?: string,
  offset?: number,
  limit?: number,
}
export interface GetAdminAnnouncementRequest {
  id: string
}
export interface UpdateAdminAnnouncementRequest {
  id: string,
  expected_version: number,
  title: string,
  body: string,
  severity: string,
  route: AdminAnnouncementRoute,
  audience: AdminAnnouncementAudience,
}
export interface ScheduleAdminAnnouncementRequest {
  id: string,
  expected_version: number,
  scheduled_at: string,
}
export interface PublishAdminAnnouncementRequest {
  id: string,
  expected_version: number,
  idempotency_key: string,
}
export interface CancelAdminAnnouncementRequest {
  id: string,
  expected_version: number,
}
export interface ReplayAdminAnnouncementsRequest {
  announcement_id?: string
}
export interface ListAdminAnnouncementAuditEventsRequest {
  id: string,
  offset?: number,
  limit?: number,
}
export enum NoticeReadMode {
  NoticeIDs = 1,
  Snapshot = 2,
}
export interface NoticeSender {
  sender_type?: NoticeSenderType,
  sender_id?: string,
  sender_name?: string,
  sender_icon_url?: string,
}
export interface Notice {
  id?: string,
  content?: string,
  /** Deprecated compatibility field; clients construct paths from route. */
  jump_link?: string,
  read_status?: ReadStatus,
  sender?: NoticeSender,
  create_time?: string,
  severity?: NoticeSeverity,
  category?: NoticeCategory,
  route?: NoticeRoute,
  route_space_id?: string,
  route_target_id?: string,
}
export interface NoticeMarkReadRequest {
  /** Required only for NoticeIDs mode. */
  notice_ids?: string[],
  read_mode: NoticeReadMode,
  /** Required only for Snapshot mode. */
  snapshot_cutoff?: string,
}
export interface NoticeMarkReadResponse {
  error_code?: string,
  code: number,
  msg: string,
}
export interface GetNoticeListData {
  notice_list?: Notice[],
  next_cursor?: string,
  has_more?: boolean,
  snapshot_cutoff: string,
}
export interface GetNoticeListRequest {
  cursor: string,
  count?: number,
  notice_rank_type?: NoticeRankType,
}
export interface GetNoticeListResponse {
  data?: GetNoticeListData,
  error_code?: string,
  code: number,
  msg: string,
}
export interface GetNoticeUnreadCountRequest {}
export interface GetNoticeUnreadCountData {
  unread_count?: number
}
export interface GetNoticeUnreadCountResponse {
  data?: GetNoticeUnreadCountData,
  error_code?: string,
  code: number,
  msg: string,
}
export const CreateAdminAnnouncement = /*#__PURE__*/createAPI<CreateAdminAnnouncementRequest, AdminAnnouncementResponse>({
  "url": "/api/admin/announcements",
  "method": "POST",
  "name": "CreateAdminAnnouncement",
  "reqType": "CreateAdminAnnouncementRequest",
  "reqMapping": {
    "body": ["title", "body", "severity", "route", "audience", "idempotency_key"]
  },
  "resType": "AdminAnnouncementResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const ListAdminAnnouncements = /*#__PURE__*/createAPI<ListAdminAnnouncementsRequest, AdminAnnouncementListResponse>({
  "url": "/api/admin/announcements",
  "method": "GET",
  "name": "ListAdminAnnouncements",
  "reqType": "ListAdminAnnouncementsRequest",
  "reqMapping": {
    "query": ["status", "offset", "limit"]
  },
  "resType": "AdminAnnouncementListResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const ReplayAdminAnnouncements = /*#__PURE__*/createAPI<ReplayAdminAnnouncementsRequest, AdminAnnouncementReplayResponse>({
  "url": "/api/admin/announcements/replay",
  "method": "POST",
  "name": "ReplayAdminAnnouncements",
  "reqType": "ReplayAdminAnnouncementsRequest",
  "reqMapping": {
    "body": ["announcement_id"]
  },
  "resType": "AdminAnnouncementReplayResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetAdminAnnouncement = /*#__PURE__*/createAPI<GetAdminAnnouncementRequest, AdminAnnouncementResponse>({
  "url": "/api/admin/announcements/:id",
  "method": "GET",
  "name": "GetAdminAnnouncement",
  "reqType": "GetAdminAnnouncementRequest",
  "reqMapping": {
    "path": ["id"]
  },
  "resType": "AdminAnnouncementResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const UpdateAdminAnnouncement = /*#__PURE__*/createAPI<UpdateAdminAnnouncementRequest, AdminAnnouncementResponse>({
  "url": "/api/admin/announcements/:id",
  "method": "PUT",
  "name": "UpdateAdminAnnouncement",
  "reqType": "UpdateAdminAnnouncementRequest",
  "reqMapping": {
    "path": ["id"],
    "body": ["expected_version", "title", "body", "severity", "route", "audience"]
  },
  "resType": "AdminAnnouncementResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const ScheduleAdminAnnouncement = /*#__PURE__*/createAPI<ScheduleAdminAnnouncementRequest, AdminAnnouncementResponse>({
  "url": "/api/admin/announcements/:id/schedule",
  "method": "POST",
  "name": "ScheduleAdminAnnouncement",
  "reqType": "ScheduleAdminAnnouncementRequest",
  "reqMapping": {
    "path": ["id"],
    "body": ["expected_version", "scheduled_at"]
  },
  "resType": "AdminAnnouncementResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const PublishAdminAnnouncement = /*#__PURE__*/createAPI<PublishAdminAnnouncementRequest, AdminAnnouncementPublicationResponse>({
  "url": "/api/admin/announcements/:id/publish",
  "method": "POST",
  "name": "PublishAdminAnnouncement",
  "reqType": "PublishAdminAnnouncementRequest",
  "reqMapping": {
    "path": ["id"],
    "body": ["expected_version", "idempotency_key"]
  },
  "resType": "AdminAnnouncementPublicationResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const CancelAdminAnnouncement = /*#__PURE__*/createAPI<CancelAdminAnnouncementRequest, AdminAnnouncementResponse>({
  "url": "/api/admin/announcements/:id/cancel",
  "method": "POST",
  "name": "CancelAdminAnnouncement",
  "reqType": "CancelAdminAnnouncementRequest",
  "reqMapping": {
    "path": ["id"],
    "body": ["expected_version"]
  },
  "resType": "AdminAnnouncementResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const ListAdminAnnouncementAuditEvents = /*#__PURE__*/createAPI<ListAdminAnnouncementAuditEventsRequest, AdminAnnouncementAuditListResponse>({
  "url": "/api/admin/announcements/:id/audit-events",
  "method": "GET",
  "name": "ListAdminAnnouncementAuditEvents",
  "reqType": "ListAdminAnnouncementAuditEventsRequest",
  "reqMapping": {
    "path": ["id"],
    "query": ["offset", "limit"]
  },
  "resType": "AdminAnnouncementAuditListResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const UpdateDraftBotInfoAgw = /*#__PURE__*/createAPI<UpdateDraftBotInfoAgwRequest, UpdateDraftBotInfoAgwResponse>({
  "url": "/api/playground_api/draftbot/update_draft_bot_info",
  "method": "POST",
  "name": "UpdateDraftBotInfoAgw",
  "reqType": "UpdateDraftBotInfoAgwRequest",
  "reqMapping": {
    "body": ["bot_info", "base_commit_version"]
  },
  "resType": "UpdateDraftBotInfoAgwResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetDraftBotInfoAgw = /*#__PURE__*/createAPI<GetDraftBotInfoAgwRequest, GetDraftBotInfoAgwResponse>({
  "url": "/api/playground_api/draftbot/get_draft_bot_info",
  "method": "POST",
  "name": "GetDraftBotInfoAgw",
  "reqType": "GetDraftBotInfoAgwRequest",
  "reqMapping": {
    "body": ["bot_id", "version", "commit_version"]
  },
  "resType": "GetDraftBotInfoAgwResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetImagexShortUrl = /*#__PURE__*/createAPI<GetImagexShortUrlRequest, GetImagexShortUrlResponse>({
  "url": "/api/playground_api/get_imagex_url",
  "method": "POST",
  "name": "GetImagexShortUrl",
  "reqType": "GetImagexShortUrlRequest",
  "reqMapping": {
    "body": ["uris", "scene"]
  },
  "resType": "GetImagexShortUrlResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
/** public popup_info */
export const GetBotPopupInfo = /*#__PURE__*/createAPI<GetBotPopupInfoRequest, GetBotPopupInfoResponse>({
  "url": "/api/playground_api/operate/get_bot_popup_info",
  "method": "POST",
  "name": "GetBotPopupInfo",
  "reqType": "GetBotPopupInfoRequest",
  "reqMapping": {
    "body": ["bot_popup_types", "bot_id"]
  },
  "resType": "GetBotPopupInfoResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const UpdateBotPopupInfo = /*#__PURE__*/createAPI<UpdateBotPopupInfoRequest, UpdateBotPopupInfoResponse>({
  "url": "/api/playground_api/operate/update_bot_popup_info",
  "method": "POST",
  "name": "UpdateBotPopupInfo",
  "reqType": "UpdateBotPopupInfoRequest",
  "reqMapping": {
    "body": ["bot_popup_type", "bot_id"]
  },
  "resType": "UpdateBotPopupInfoResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const ReportUserBehavior = /*#__PURE__*/createAPI<ReportUserBehaviorRequest, ReportUserBehaviorResponse>({
  "url": "/api/playground_api/report_user_behavior",
  "method": "POST",
  "name": "ReportUserBehavior",
  "reqType": "ReportUserBehaviorRequest",
  "reqMapping": {
    "body": ["resource_id", "resource_type", "behavior_type", "space_id"]
  },
  "resType": "ReportUserBehaviorResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
/** Create shortcut instructions */
export const CreateUpdateShortcutCommand = /*#__PURE__*/createAPI<shortcut_command.CreateUpdateShortcutCommandRequest, shortcut_command.CreateUpdateShortcutCommandResponse>({
  "url": "/api/playground_api/create_update_shortcut_command",
  "method": "POST",
  "name": "CreateUpdateShortcutCommand",
  "reqType": "shortcut_command.CreateUpdateShortcutCommandRequest",
  "reqMapping": {
    "body": ["object_id", "space_id", "shortcuts"]
  },
  "resType": "shortcut_command.CreateUpdateShortcutCommandResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetFileUrls = /*#__PURE__*/createAPI<GetFileUrlsRequest, GetFileUrlsResponse>({
  "url": "/api/playground_api/get_file_list",
  "method": "POST",
  "name": "GetFileUrls",
  "reqType": "GetFileUrlsRequest",
  "reqMapping": {
    "body": ["scene"]
  },
  "resType": "GetFileUrlsResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const NoticeMarkRead = /*#__PURE__*/createAPI<NoticeMarkReadRequest, NoticeMarkReadResponse>({
  "url": "/api/playground_api/notice/mark_read",
  "method": "POST",
  "name": "NoticeMarkRead",
  "reqType": "NoticeMarkReadRequest",
  "reqMapping": {
    "body": ["notice_ids", "read_mode", "snapshot_cutoff"]
  },
  "resType": "NoticeMarkReadResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetNoticeList = /*#__PURE__*/createAPI<GetNoticeListRequest, GetNoticeListResponse>({
  "url": "/api/playground_api/notice/get_list",
  "method": "POST",
  "name": "GetNoticeList",
  "reqType": "GetNoticeListRequest",
  "reqMapping": {
    "body": ["cursor", "count", "notice_rank_type"]
  },
  "resType": "GetNoticeListResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetNoticeUnreadCount = /*#__PURE__*/createAPI<GetNoticeUnreadCountRequest, GetNoticeUnreadCountResponse>({
  "url": "/api/playground_api/notice/get_unread_count",
  "method": "POST",
  "name": "GetNoticeUnreadCount",
  "reqType": "GetNoticeUnreadCountRequest",
  "reqMapping": {},
  "resType": "GetNoticeUnreadCountResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
/** prompt resource */
export const GetOfficialPromptResourceList = /*#__PURE__*/createAPI<prompt_resource.GetOfficialPromptResourceListRequest, prompt_resource.GetOfficialPromptResourceListResponse>({
  "url": "/api/playground_api/get_official_prompt_list",
  "method": "POST",
  "name": "GetOfficialPromptResourceList",
  "reqType": "prompt_resource.GetOfficialPromptResourceListRequest",
  "reqMapping": {
    "body": ["keyword"]
  },
  "resType": "prompt_resource.GetOfficialPromptResourceListResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetPromptResourceInfo = /*#__PURE__*/createAPI<prompt_resource.GetPromptResourceInfoRequest, prompt_resource.GetPromptResourceInfoResponse>({
  "url": "/api/playground_api/get_prompt_resource_info",
  "method": "GET",
  "name": "GetPromptResourceInfo",
  "reqType": "prompt_resource.GetPromptResourceInfoRequest",
  "reqMapping": {
    "body": ["prompt_resource_id"]
  },
  "resType": "prompt_resource.GetPromptResourceInfoResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const UpsertPromptResource = /*#__PURE__*/createAPI<prompt_resource.UpsertPromptResourceRequest, prompt_resource.UpsertPromptResourceResponse>({
  "url": "/api/playground_api/upsert_prompt_resource",
  "method": "POST",
  "name": "UpsertPromptResource",
  "reqType": "prompt_resource.UpsertPromptResourceRequest",
  "reqMapping": {
    "body": ["prompt"]
  },
  "resType": "prompt_resource.UpsertPromptResourceResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const DeletePromptResource = /*#__PURE__*/createAPI<prompt_resource.DeletePromptResourceRequest, prompt_resource.DeletePromptResourceResponse>({
  "url": "/api/playground_api/delete_prompt_resource",
  "method": "POST",
  "name": "DeletePromptResource",
  "reqType": "prompt_resource.DeletePromptResourceRequest",
  "reqMapping": {
    "body": ["prompt_resource_id"]
  },
  "resType": "prompt_resource.DeletePromptResourceResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const GetSpaceListV2 = /*#__PURE__*/createAPI<GetSpaceListV2Request, GetSpaceListV2Response>({
  "url": "/api/playground_api/space/list",
  "method": "POST",
  "name": "GetSpaceListV2",
  "reqType": "GetSpaceListV2Request",
  "reqMapping": {
    "body": ["search_word", "enterprise_id", "organization_id", "scope_type", "page", "size"]
  },
  "resType": "GetSpaceListV2Response",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});
export const MGetUserBasicInfo = /*#__PURE__*/createAPI<MGetUserBasicInfoRequest, MGetUserBasicInfoResponse>({
  "url": "/api/playground_api/mget_user_info",
  "method": "POST",
  "name": "MGetUserBasicInfo",
  "reqType": "MGetUserBasicInfoRequest",
  "reqMapping": {
    "body": ["user_ids", "need_user_status", "need_enterprise_identity", "need_volcano_user_name"]
  },
  "resType": "MGetUserBasicInfoResponse",
  "schemaRoot": "api://schemas/idl_playground_playground",
  "service": "playground"
});