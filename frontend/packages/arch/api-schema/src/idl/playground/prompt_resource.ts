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
export interface GetOfficialPromptResourceListRequest {
  keyword?: string
}
export interface PromptResource {
  id?: string,
  space_id?: string,
  name?: string,
  description?: string,
  prompt_text?: string,
}
export interface GetOfficialPromptResourceListResponse {
  data: PromptResource[],
  code: number,
  msg: string,
}
export interface GetPromptResourceInfoRequest {
  prompt_resource_id: string
}
export interface GetPromptResourceInfoResponse {
  data?: PromptResource,
  code: number,
  msg: string,
}
export interface UpsertPromptResourceRequest {
  prompt: PromptResource
}
export interface UpsertPromptResourceResponse {
  data?: ShowPromptResource,
  code: number,
  msg: string,
}
export interface ShowPromptResource {
  id: string
}
export interface DeletePromptResourceRequest {
  prompt_resource_id: string
}
export interface DeletePromptResourceResponse {
  code: number,
  msg: string,
}
/** Parameter priority from top to bottom */
export interface SyncPromptResourceToEsRequest {
  SyncAll?: boolean,
  PromptResourceIDList?: number[],
  SpaceIDList?: number[],
}
export interface SyncPromptResourceToEsResponse {}
export interface MGetDisplayResourceInfoRequest {
  /** The maximum number of one page can be transferred, and the implementer can limit the maximum to 100. */
  ResIDs: number[],
  /** The current user, the implementation is used to determine the authority */
  CurrentUserID: number,
}
export interface MGetDisplayResourceInfoResponse {
  ResourceList: DisplayResourceInfo[]
}
export enum ActionKey {
  /** copy */
  Copy = 1,
  /** delete */
  Delete = 2,
  /** enable/disable */
  EnableSwitch = 3,
  /** edit */
  Edit = 4,
  /** Cross-space copy */
  CrossSpaceCopy = 10,
}
export interface ResourceAction {
  /** An operation corresponds to a unique key, and the key is constrained by the resource side */
  key: ActionKey,
  /** ture = can operate this Action, false = grey out */
  enable: boolean,
}
/** For display, the implementer provides display information */
export interface DisplayResourceInfo {
  /** Resource ID */
  ResID?: number,
  /** resource description */
  Desc?: string,
  /** Resource Icon, full url */
  Icon?: string,
  /** Resource status, each type of resource defines itself */
  BizResStatus?: number,
  /** Whether to enable multi-person editing */
  CollaborationEnable?: boolean,
  /** Business carry extended information to res_type distinguish, each res_type defined schema and meaning is not the same, need to judge before use res_type */
  BizExtend?: {
    [key: string | number]: string
  },
  /** Different types of different operation buttons are agreed upon by the resource implementer and the front end. Return is displayed, if you want to hide a button, do not return; */
  Actions?: ResourceAction[],
  /** Whether to ban entering the details page */
  DetailDisable?: boolean,
  /** resource name */
  Name?: string,
  /** Resource release status, 1 - unpublished, 2 - published */
  PublishStatus?: ResourcePublishStatus,
  /** Last edited, unix timestamp */
  EditTime?: number,
}
export enum ResourcePublishStatus {
  /** unpublished */
  UnPublished = 1,
  /** Published */
  Published = 2,
}