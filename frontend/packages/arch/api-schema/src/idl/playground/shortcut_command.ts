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

import * as bot_common from './../app/bot_common';
export { bot_common };
import * as base from './../base';
export { base };
export interface CreateShortcutCommandRequest {
  object_id: string,
  shortcuts: ShortcutCommand,
}
export interface CreateShortcutCommandResponse {
  shortcuts: ShortcutCommand
}
export interface ShortcutStruct {
  /** Shortcut ID list, bound on the entity */
  shortcut_sort?: string[],
  /** Quick command content list */
  shortcut_list?: ShortcutCommand[],
}
export interface ShortcutCommand {
  /** Binding Entity ID */
  object_id: string,
  /** command name */
  command_name: string,
  /** Quick Instruction */
  shortcut_command: string,
  /** describe */
  description: string,
  /** Send type */
  send_type: SendType,
  /** Use tool type */
  tool_type: ToolType,
  work_flow_id: string,
  plugin_id: string,
  plugin_api_name: string,
  /** Template query */
  template_query: string,
  /** Panel parameters */
  components_list: Components[],
  /** Form schema */
  card_schema: string,
  /** Instruction ID */
  command_id: string,
  /** Tool information, including name + variable list +... */
  tool_info: ToolInfo,
  /** command icon */
  shortcut_icon: ShortcutFileInfo,
  /** Multi instruction, which node executes the instruction */
  agent_id?: string,
  plugin_api_id: string,
  plugin_from?: bot_common.PluginFrom,
}
export interface ShortcutFileInfo {
  url: string,
  uri: string,
}
export interface Components {
  /** Panel parameters */
  name: string,
  description: string,
  input_type: InputType,
  /** When requesting the tool, the key of the parameter */
  parameter: string,
  options: string[],
  default_value: DefaultValue,
  /** Whether to hide or not to show */
  hide: boolean,
  /** What types are supported input_type MixUpload */
  upload_options: InputType[],
}
export interface DefaultValue {
  value: string,
  type: InputType,
}
export interface ToolInfo {
  tool_name: string,
  /** Variable lists, plugins & workFLow */
  tool_params_list: ToolParams[],
}
export interface ToolParams {
  /** parameter list */
  name: string,
  required: boolean,
  desc: string,
  type: string,
  /** default value */
  default_value: string,
  /** Is it a panel parameter? */
  refer_component: boolean,
}
export enum SendType {
  /** Send query directly */
  SendTypeQuery = 0,
  /** use panel */
  SendTypePanel = 1,
}
export enum ToolType {
  /** Using WorkFlow */
  ToolTypeWorkFlow = 1,
  /** use plug-ins */
  ToolTypePlugin = 2,
}
export enum InputType {
  TextInput = 0,
  Select = 1,
  UploadImage = 2,
  UploadDoc = 3,
  UploadTable = 4,
  UploadAudio = 5,
  MixUpload = 6,
  VIDEO = 7,
  ARCHIVE = 8,
  CODE = 9,
  TXT = 10,
  PPT = 11,
}
export interface CreateUpdateShortcutCommandRequest {
  object_id: string,
  space_id: string,
  shortcuts: ShortcutCommand,
}
export interface CreateUpdateShortcutCommandResponse {
  shortcuts: ShortcutCommand,
  code: number,
  msg: string,
}