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

export type AppDevProjectStatus = 'creating' | 'ready' | 'archived' | 'error';

export type AppDevRuntimeStatus =
  | 'stopped'
  | 'starting'
  | 'running'
  | 'recovering'
  | 'stopping'
  | 'cleanup_pending'
  | 'error';

export type AppDevBuildState = 'idle' | 'building' | 'ready' | 'failed';

export const APP_DEV_BUILD_SAFE_ERROR_CODES = [
  'build_failed',
  'source_invalid',
  'dependency_failed',
  'provider_unavailable',
  'provider_capability_missing',
  'provider_contract_violation',
  'build_timed_out',
  'build_canceled',
  'artifact_invalid',
] as const;

export type AppDevBuildSafeErrorCode =
  (typeof APP_DEV_BUILD_SAFE_ERROR_CODES)[number];

export type AppDevChatRole = 'user' | 'assistant' | 'system';

export type AppDevMessageType =
  | 'user'
  | 'assistant'
  | 'thinking'
  | 'tool_call'
  | 'tool_call_update'
  | 'error'
  | 'section';

export interface AppDevProject {
  id: string;
  name: string;
  description?: string;
  prompt?: string;
  status: AppDevProjectStatus;
  runtimeStatus?: AppDevRuntimeStatus;
  previewUrl?: string;
  lastBuildStatus?: 'building' | 'success' | 'error';
  lastBuildType?: string;
  lastBuildMessage?: string;
  lastBuildAt?: string;
  sourceUpdatedAt?: string;
  creatorName?: string;
  updatedAt?: string;
  createdAt?: string;
}

export interface AppDevFileNode {
  id: string;
  path: string;
  name: string;
  type: 'file' | 'directory';
  size?: number;
  children?: AppDevFileNode[];
  updatedAt?: string;
}

export interface AppDevFileContent {
  path: string;
  content: string;
  version: string;
  language: string;
}

export interface AppDevRuntimeInfo {
  generation: number;
  status: AppDevRuntimeStatus;
  canStart: boolean;
  recovering: boolean;
  stopping: boolean;
  previewUrl?: string;
  message?: string;
  lastKeepAliveAt?: string;
}

export interface AppDevBuildInfo {
  generation: number;
  state: AppDevBuildState;
  releaseAvailable: boolean;
  size: number;
  updatedAt?: string;
  stale: boolean;
  safeErrorCode?: AppDevBuildSafeErrorCode;
  safeMessage?: string;
}

export interface AppDevRuntimeLog {
  id: string;
  level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  timestamp: string;
}

export interface AppDevChatAttachment {
  id: string;
  name: string;
  path: string;
  mimeType?: string;
  size?: number;
  type: 'image' | 'file' | 'prototype_image';
}

export interface AppDevChatMessage {
  id: string;
  type: AppDevMessageType;
  role?: AppDevChatRole;
  content: string;
  title?: string;
  attachments?: AppDevChatAttachment[];
  isStreaming?: boolean;
  createdAt?: string;
}

export interface AppDevChatSession {
  sessionId: string;
  requestId: string;
  running: boolean;
}

export interface AppDevChatStatus {
  running: boolean;
  sessionId?: string;
  requestId?: string;
}

export interface AppDevSSEEvent {
  event: string;
  data: Record<string, unknown>;
}

export interface AppDevModel {
  id: string;
  name: string;
  provider?: string;
  protocol?: string;
  supportsMultiModal?: boolean;
  supportsImageUnderstanding?: boolean;
  enableBase64URL?: boolean;
}

export interface AppDevDataSource {
  id: string;
  name: string;
  type: string;
  description?: string;
}

export interface AppDevPageResult<T> {
  items: T[];
  total: number;
}

export interface AppDevApiResponse<T> {
  code?: number;
  message?: string;
  msg?: string;
  data?: T;
}

export interface AppDevCreateProjectParams {
  spaceId: string;
  name: string;
  prompt: string;
}

export interface AppDevProjectIdentity {
  spaceId: string;
  projectId: string;
}

export interface AppDevSnapshot {
  id: string;
  label: string;
  createdAt: string;
}
