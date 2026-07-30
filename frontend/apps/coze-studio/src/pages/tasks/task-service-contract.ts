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

import type { LegacyPageResponse } from '../workbench/thread-client/legacy-page-response';
import type {
  AppendWorkbenchMessageRequest,
  CancelWorkbenchRunRequest,
  CreateWorkbenchRunRequest,
  CreateWorkbenchThreadRequest,
  GenerateWorkbenchSuggestionsRequest,
  GetWorkbenchThreadRequest,
  ListWorkbenchRunsRequest,
  ResumeWorkbenchRunRequest,
  RetryWorkbenchSubagentRunRequest,
  SearchWorkbenchThreadsRequest,
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchRunEvent,
  WorkbenchThread,
  WorkbenchThreadCreation,
} from '../workbench/thread-client';

export interface PageScopedRequest {
  space_id: string;
}

export type PageSpace<Request extends { space_id: string }> = Omit<
  Request,
  'space_id'
> &
  PageScopedRequest;

export type ListTaskThreadsRequest = PageSpace<SearchWorkbenchThreadsRequest>;
export type CreateTaskThreadRequest = PageSpace<CreateWorkbenchThreadRequest>;
export type GetTaskThreadRequest = PageSpace<GetWorkbenchThreadRequest>;
export interface ListTaskThreadMessagesRequest extends PageScopedRequest {
  thread_id: string;
  page?: number;
  page_size?: number;
  signal?: AbortSignal;
}
export type GenerateTaskThreadSuggestionsRequest =
  PageSpace<GenerateWorkbenchSuggestionsRequest>;
export type AppendTaskThreadMessageRequest =
  PageSpace<AppendWorkbenchMessageRequest>;
export type ListTaskThreadRunsRequest = PageSpace<ListWorkbenchRunsRequest>;
export type CreateTaskThreadRunRequest = PageSpace<CreateWorkbenchRunRequest>;
export type ResumeTaskThreadRunRequest = PageSpace<ResumeWorkbenchRunRequest>;
export type CancelTaskThreadRunRequest = PageSpace<CancelWorkbenchRunRequest>;
export type RetryTaskThreadSubagentRunRequest =
  PageSpace<RetryWorkbenchSubagentRunRequest>;
export interface ListTaskThreadRunEventsRequest extends PageScopedRequest {
  thread_id: string;
  run_id?: string;
  event_types?: string[];
  page?: number;
  page_size?: number;
  signal?: AbortSignal;
}

export type ListTaskThreadsResponse = LegacyPageResponse<{
  threads: WorkbenchThread[];
  total: number;
}>;
export type CreateTaskThreadResponse =
  LegacyPageResponse<WorkbenchThreadCreation>;
export type GetTaskThreadResponse = LegacyPageResponse<WorkbenchThread>;
export type ListTaskThreadMessagesResponse = LegacyPageResponse<{
  messages: WorkbenchMessage[];
  total: number;
}>;
export interface GenerateTaskThreadSuggestionsResponse {
  suggestions: string[];
}
export type AppendTaskThreadMessageResponse =
  LegacyPageResponse<WorkbenchMessage>;
export type ListTaskThreadRunsResponse = LegacyPageResponse<{
  runs: WorkbenchRun[];
  total: number;
}>;
export interface CreateTaskThreadRunResponse
  extends LegacyPageResponse<WorkbenchRun> {
  message?: WorkbenchMessage;
}
export type ResumeTaskThreadRunResponse = LegacyPageResponse<WorkbenchRun>;
export type CancelTaskThreadRunResponse = Omit<
  LegacyPageResponse<never>,
  'data'
>;
export type RetryTaskThreadSubagentRunResponse =
  LegacyPageResponse<WorkbenchRun>;
export type ListTaskThreadRunEventsResponse = LegacyPageResponse<{
  events: WorkbenchRunEvent[];
  total: number;
}>;
