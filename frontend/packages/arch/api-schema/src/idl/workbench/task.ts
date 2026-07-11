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
export enum TaskStatus {
  Created = 1,
  Queued = 2,
  Running = 3,
  Succeeded = 4,
  Failed = 5,
  Canceling = 6,
  Canceled = 7,
}
export interface TaskEvent {
  id: string;
  task_id: string;
  run_id?: string;
  event_type: string;
  payload?: string;
  created_at: number;
}
export interface ChatTask {
  id: string;
  space_id: string;
  creator_id: string;
  conversation_id?: string;
  message_id?: string;
  skill_id?: string;
  title: string;
  status: TaskStatus;
  progress: number;
  input?: string;
  result?: string;
  error?: string;
  created_at: number;
  updated_at: number;
}
export interface TaskThread {
  thread_id: string;
  legacy_task_id: string;
  space_id: string;
  creator_id: string;
  title: string;
  status: string;
  source: string;
  progress: number;
  last_user_message: string;
  last_agent_message: string;
  created_at: number;
  updated_at: number;
  values?: TaskThreadValues;
}
export interface TaskThreadTodo {
  id: string;
  title: string;
  status: string;
}
export interface TaskThreadValues {
  todos?: Array<TaskThreadTodo>;
}
export interface TaskThreadMessage {
  message_id: string;
  thread_id: string;
  run_id: string;
  role: string;
  content: string;
  metadata: string;
  created_at: number;
}
export interface TaskThreadRun {
  run_id: string;
  thread_id: string;
  parent_run_id: string;
  space_id: string;
  creator_id: string;
  assistant_id: string;
  run_kind: string;
  status: string;
  command: string;
  input: string;
  config: string;
  context: string;
  metadata: string;
  stream_mode: string;
  multitask_strategy: string;
  on_disconnect: string;
  durability: string;
  idempotency_key: string;
  worker_id: string;
  error_code: string;
  error_message: string;
  started_at: number;
  ended_at: number;
  created_at: number;
  updated_at: number;
}
export interface TaskThreadRunEvent {
  event_id: string;
  thread_id: string;
  run_id: string;
  event_type: string;
  payload: string;
  created_at: number;
}
export interface TaskThreadRunJournalToolCall {
  id: string;
  name: string;
  type: string;
  arguments: string;
}
export interface TaskThreadRunJournalMessage {
  id: string;
  thread_id: string;
  run_id: string;
  type: string;
  role: string;
  content: string;
  name: string;
  tool_call_id: string;
  tool_calls: TaskThreadRunJournalToolCall[];
  additional_kwargs: string;
  usage: string;
  created_at: number;
  source_event_id: string;
}
export interface TaskThreadTokenUsage {
  usage_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  source: string;
  step_id: string;
  step_index: number;
  step_name: string;
  model_name: string;
  provider: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_micros: number;
  currency: string;
  estimated: boolean;
  raw_usage: string;
  metadata: string;
  created_at: number;
}
export interface TaskThreadTokenUsageAggregate {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_micros: number;
  call_count: number;
  lead_agent_tokens: number;
  subagent_tokens: number;
  middleware_tokens: number;
  tool_tokens: number;
}
export interface TaskThreadTokenUsageRunAggregate {
  run_id: string;
  aggregate: TaskThreadTokenUsageAggregate;
}
export interface TaskThreadMemory {
  memory_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  scope: string;
  content: string;
  metadata: string;
  score: number;
  confidence: number;
  source_type: string;
  source_id: string;
  correction_of_memory_id: string;
  corrected_at: number;
  expires_at: number;
  created_at: number;
  updated_at: number;
  deleted_at: number;
}
export interface TaskThreadMemoryAuditEvent {
  event_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  memory_id: string;
  actor_id: string;
  event_type: string;
  scope: string;
  source_type: string;
  source_id: string;
  affected_count: number;
  created_at: number;
}
export interface TaskThreadGuardrailAuditEvent {
  event_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  actor_id: string;
  event_type: string;
  target_type: string;
  target_id: string;
  operation: string;
  source: string;
  action: string;
  fail_mode: string;
  provider: string;
  reason_code: string;
  rule_ids: string;
  created_at: number;
}
export interface TaskThreadMCPRuntimeAuditEvent {
  event_id: string;
  space_id: string;
  thread_id: string;
  run_id: string;
  server_id: string;
  runtime_tool_name: string;
  event_type: string;
  error_code: string;
  elapsed_millis: number;
  output_bytes: number;
  created_at: number;
}
export interface TaskThreadArtifact {
  artifact_id: string;
  thread_id: string;
  run_id: string;
  file_id: string;
  title: string;
  artifact_type: string;
  virtual_path: string;
  content_type: string;
  size_bytes: number;
  preview_mode: string;
  metadata: string;
  created_at: number;
  updated_at: number;
  deleted_at: number;
}
export interface CreateTaskRequest {
  space_id: string;
  title: string;
  conversation_id?: string;
  message_id?: string;
  skill_id?: string;
  input?: string;
}
export interface CreateTaskResponse {
  data?: ChatTask;
  code: number;
  msg: string;
}
export interface ListTasksRequest {
  space_id: string;
  status?: TaskStatus;
  page?: number;
  page_size?: number;
}
export interface ListTasksData {
  tasks: ChatTask[];
  total: number;
}
export interface ListTasksResponse {
  data?: ListTasksData;
  code: number;
  msg: string;
}
export interface GetTaskRequest {
  task_id: string;
}
export interface GetTaskResponse {
  data?: ChatTask;
  code: number;
  msg: string;
}
export interface ListTaskThreadsRequest {
  space_id: string;
  status?: string;
  page?: number;
  page_size?: number;
}
export interface ListTaskThreadsData {
  threads: TaskThread[];
  total: number;
}
export interface ListTaskThreadsResponse {
  data?: ListTaskThreadsData;
  code: number;
  msg: string;
}
export interface CreateTaskThreadRequest {
  space_id: string;
  message: string;
  title?: string;
  defer_start?: boolean;
  assistant_id?: string;
  command?: string;
  config?: string;
  context?: string;
  metadata?: string;
  stream_mode?: string;
  multitask_strategy?: string;
  on_disconnect?: string;
  durability?: string;
  idempotency_key?: string;
}
export interface CreateTaskThreadData {
  thread?: TaskThread;
  message?: TaskThreadMessage;
  run?: TaskThreadRun;
}
export interface CreateTaskThreadResponse {
  data?: CreateTaskThreadData;
  code: number;
  msg: string;
}
export interface GetTaskThreadRequest {
  thread_id: string;
}
export interface GetTaskThreadResponse {
  data?: TaskThread;
  code: number;
  msg: string;
}
export interface ListTaskThreadMessagesRequest {
  thread_id: string;
  page?: number;
  page_size?: number;
}
export interface TaskThreadSuggestionMessage {
  role: string;
  content: string;
}
export interface GenerateTaskThreadSuggestionsRequest {
  thread_id: string;
  messages: TaskThreadSuggestionMessage[];
  n?: number;
  model_name?: string;
  model_type?: string;
}
export interface AppendTaskThreadMessageRequest {
  thread_id: string;
  run_id?: string;
  role: string;
  content: string;
  metadata?: string;
}
export interface ListTaskThreadRunsRequest {
  thread_id: string;
  parent_run_id?: string;
  status?: string;
  page?: number;
  page_size?: number;
}
export interface ListTaskThreadRunEventsRequest {
  thread_id: string;
  run_id?: string;
  page?: number;
  page_size?: number;
}
export interface GetTaskThreadTokenUsageRequest {
  thread_id: string;
  run_id?: string;
  include_child_runs?: boolean;
  source?: string;
  page?: number;
  page_size?: number;
}
export interface ListTaskThreadMemoriesRequest {
  thread_id: string;
  run_id?: string;
  scope?: string;
  scopes?: string[];
  q?: string;
  include_expired?: boolean;
  include_deleted?: boolean;
  page?: number;
  page_size?: number;
}
export interface ExportTaskThreadMemoriesRequest {
  thread_id: string;
  run_id?: string;
  scope?: string;
  scopes?: string[];
  q?: string;
  include_expired?: boolean;
  include_deleted?: boolean;
  limit?: number;
}
export interface ListTaskThreadMemoryAuditEventsRequest {
  thread_id: string;
  memory_id?: string;
  page?: number;
  page_size?: number;
}
export interface ListTaskThreadGuardrailAuditEventsRequest {
  thread_id: string;
  run_id?: string;
  page?: number;
  page_size?: number;
}
export interface ListTaskThreadMCPRuntimeAuditEventsRequest {
  thread_id: string;
  run_id?: string;
  page?: number;
  page_size?: number;
}
export interface ExportTaskThreadGuardrailAuditEventsRequest {
  thread_id: string;
  run_id?: string;
  page?: number;
  page_size?: number;
}
export interface UpdateTaskThreadMemoryRequest {
  thread_id: string;
  memory_id: string;
  run_id?: string;
  scope: string;
  content: string;
  metadata?: string;
  score?: number;
  confidence?: number;
  source_type?: string;
  source_id?: string;
  correction_of_memory_id?: string;
  corrected_at?: number;
  expires_at?: number;
}
export interface ImportTaskThreadMemoryItem {
  run_id?: string;
  scope?: string;
  content: string;
  metadata?: string;
  score?: number;
  confidence?: number;
  source_type?: string;
  source_id?: string;
  correction_of_memory_id?: string;
  corrected_at?: number;
  expires_at?: number;
}
export interface ImportTaskThreadMemoriesRequest {
  thread_id: string;
  memories: ImportTaskThreadMemoryItem[];
}
export interface DeleteTaskThreadMemoryRequest {
  thread_id: string;
  memory_id: string;
}
export interface RestoreTaskThreadMemoryRequest {
  thread_id: string;
  memory_id: string;
}
export interface ClearTaskThreadMemoriesRequest {
  thread_id: string;
  run_id?: string;
  scopes?: string[];
}
export interface ListTaskThreadArtifactsRequest {
  thread_id: string;
  run_id?: string;
  deleted_only?: boolean;
  page?: number;
  page_size?: number;
  space_id?: string;
}
export interface GetTaskThreadArtifactSignedURLRequest {
  thread_id: string;
  artifact_id: string;
  mode?: string;
  ttl_seconds?: number;
  space_id?: string;
}
export interface CreateTaskThreadRunRequest {
  thread_id: string;
  assistant_id?: string;
  command?: string;
  input: string;
  config?: string;
  context?: string;
  metadata?: string;
  stream_mode?: string;
  multitask_strategy?: string;
  on_disconnect?: string;
  durability?: string;
  idempotency_key?: string;
  message_content?: string;
  message_metadata?: string;
}
export interface HumanInteractionResponse {
  schema: string;
  interaction_id: string;
  kind: 'clarification' | 'confirmation';
  decision: 'answered' | 'approved' | 'rejected';
  answer?: string;
  choice_id?: string;
  comment?: string;
  submitted_by?: string;
  submitted_at?: number;
  source?: string;
}
export interface ResumeTaskThreadRunRequest {
  thread_id: string;
  run_id: string;
  interrupt_id: string;
  response: HumanInteractionResponse;
  idempotency_key?: string;
}
export interface CancelTaskThreadRunRequest {
  thread_id: string;
  run_id: string;
}
export interface RetryTaskThreadSubagentRunRequest {
  thread_id: string;
  run_id: string;
  idempotency_key?: string;
}
export interface ListTaskThreadMessagesData {
  messages: TaskThreadMessage[];
  total: number;
}
export interface ListTaskThreadMessagesResponse {
  data?: ListTaskThreadMessagesData;
  code: number;
  msg: string;
}
export interface GenerateTaskThreadSuggestionsResponse {
  suggestions: string[];
}
export interface AppendTaskThreadMessageResponse {
  data?: TaskThreadMessage;
  code: number;
  msg: string;
}
export interface ListTaskThreadRunsData {
  runs: TaskThreadRun[];
  total: number;
}
export interface ListTaskThreadRunsResponse {
  data?: ListTaskThreadRunsData;
  code: number;
  msg: string;
}
export interface ListTaskThreadRunEventsData {
  events: TaskThreadRunEvent[];
  total: number;
  journal_messages?: TaskThreadRunJournalMessage[];
}
export interface ListTaskThreadRunEventsResponse {
  data?: ListTaskThreadRunEventsData;
  code: number;
  msg: string;
}
export interface GetTaskThreadTokenUsageData {
  usage: TaskThreadTokenUsage[];
  total: number;
  aggregate: TaskThreadTokenUsageAggregate;
  run_aggregates?: TaskThreadTokenUsageRunAggregate[];
}
export interface GetTaskThreadTokenUsageResponse {
  data?: GetTaskThreadTokenUsageData;
  code: number;
  msg: string;
}
export interface ListTaskThreadMemoriesData {
  memories: TaskThreadMemory[];
  total: number;
}
export interface ListTaskThreadMemoriesResponse {
  data?: ListTaskThreadMemoriesData;
  code: number;
  msg: string;
}
export interface ExportTaskThreadMemoriesData {
  schema: string;
  thread_id: string;
  exported_at: number;
  total: number;
  memories: TaskThreadMemory[];
}
export interface ExportTaskThreadMemoriesResponse {
  data?: ExportTaskThreadMemoriesData;
  code: number;
  msg: string;
}
export interface UpdateTaskThreadMemoryData {
  memory?: TaskThreadMemory;
  updated: boolean;
}
export interface UpdateTaskThreadMemoryResponse {
  data?: UpdateTaskThreadMemoryData;
  code: number;
  msg: string;
}
export interface ImportTaskThreadMemoriesData {
  imported: number;
  skipped: number;
  memories: TaskThreadMemory[];
}
export interface ImportTaskThreadMemoriesResponse {
  data?: ImportTaskThreadMemoriesData;
  code: number;
  msg: string;
}
export interface DeleteTaskThreadMemoryResponse {
  code: number;
  msg: string;
}
export interface ClearTaskThreadMemoriesData {
  deleted: number;
}
export interface ClearTaskThreadMemoriesResponse {
  data?: ClearTaskThreadMemoriesData;
  code: number;
  msg: string;
}
export interface RestoreTaskThreadMemoryData {
  memory?: TaskThreadMemory;
  restored: boolean;
}
export interface RestoreTaskThreadMemoryResponse {
  data?: RestoreTaskThreadMemoryData;
  code: number;
  msg: string;
}
export interface ListTaskThreadMemoryAuditEventsData {
  events: TaskThreadMemoryAuditEvent[];
  total: number;
}
export interface ListTaskThreadMemoryAuditEventsResponse {
  data?: ListTaskThreadMemoryAuditEventsData;
  code: number;
  msg: string;
}
export interface ListTaskThreadGuardrailAuditEventsData {
  events: TaskThreadGuardrailAuditEvent[];
  total: number;
}
export interface ListTaskThreadGuardrailAuditEventsResponse {
  data?: ListTaskThreadGuardrailAuditEventsData;
  code: number;
  msg: string;
}
export interface ListTaskThreadMCPRuntimeAuditEventsData {
  events: TaskThreadMCPRuntimeAuditEvent[];
  total: number;
}
export interface ListTaskThreadMCPRuntimeAuditEventsResponse {
  data?: ListTaskThreadMCPRuntimeAuditEventsData;
  code: number;
  msg: string;
}
export interface ExportTaskThreadGuardrailAuditEventsData {
  schema: string;
  thread_id: string;
  exported_at: number;
  page: number;
  page_size: number;
  total: number;
  events: TaskThreadGuardrailAuditEvent[];
}
export interface ExportTaskThreadGuardrailAuditEventsResponse {
  data?: ExportTaskThreadGuardrailAuditEventsData;
  code: number;
  msg: string;
}
export interface ListTaskThreadArtifactsData {
  artifacts: TaskThreadArtifact[];
  total: number;
}
export interface ListTaskThreadArtifactsResponse {
  data?: ListTaskThreadArtifactsData;
  code: number;
  msg: string;
}
export interface GetTaskThreadArtifactSignedURLData {
  artifact_id: string;
  url: string;
  expires_in_seconds: number;
  content_type: string;
  preview_mode: string;
}
export interface GetTaskThreadArtifactSignedURLResponse {
  data?: GetTaskThreadArtifactSignedURLData;
  code: number;
  msg: string;
}
export interface CreateTaskThreadRunResponse {
  data?: TaskThreadRun;
  message?: TaskThreadMessage;
  code: number;
  msg: string;
}
export interface ResumeTaskThreadRunResponse {
  data?: TaskThreadRun;
  code: number;
  msg: string;
}
export interface CancelTaskThreadRunResponse {
  data?: TaskThreadRun;
  code: number;
  msg: string;
}
export interface RetryTaskThreadSubagentRunResponse {
  data?: TaskThreadRun;
  code: number;
  msg: string;
}
export interface TaskEventsData {
  events: TaskEvent[];
}
export interface TaskEventsResponse {
  data?: TaskEventsData;
  code: number;
  msg: string;
}
export const CreateTask = /*#__PURE__*/ createAPI<
  CreateTaskRequest,
  CreateTaskResponse
>({
  url: '/api/workbench/tasks',
  method: 'POST',
  name: 'CreateTask',
  reqType: 'CreateTaskRequest',
  reqMapping: {
    body: [
      'space_id',
      'title',
      'conversation_id',
      'message_id',
      'skill_id',
      'input',
    ],
  },
  resType: 'CreateTaskResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTasks = /*#__PURE__*/ createAPI<
  ListTasksRequest,
  ListTasksResponse
>({
  url: '/api/workbench/tasks',
  method: 'GET',
  name: 'ListTasks',
  reqType: 'ListTasksRequest',
  reqMapping: {
    query: ['space_id', 'status', 'page', 'page_size'],
  },
  resType: 'ListTasksResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const GetTask = /*#__PURE__*/ createAPI<GetTaskRequest, GetTaskResponse>(
  {
    url: '/api/workbench/tasks/:task_id',
    method: 'GET',
    name: 'GetTask',
    reqType: 'GetTaskRequest',
    reqMapping: {
      path: ['task_id'],
    },
    resType: 'GetTaskResponse',
    schemaRoot: 'api://schemas/idl_workbench_task',
    service: 'workbenchTask',
  },
);
export const ListTaskThreads = /*#__PURE__*/ createAPI<
  ListTaskThreadsRequest,
  ListTaskThreadsResponse
>({
  url: '/api/workbench/task_threads',
  method: 'GET',
  name: 'ListTaskThreads',
  reqType: 'ListTaskThreadsRequest',
  reqMapping: {
    query: ['space_id', 'status', 'page', 'page_size'],
  },
  resType: 'ListTaskThreadsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const CreateTaskThread = /*#__PURE__*/ createAPI<
  CreateTaskThreadRequest,
  CreateTaskThreadResponse
>({
  url: '/api/workbench/task_threads',
  method: 'POST',
  name: 'CreateTaskThread',
  reqType: 'CreateTaskThreadRequest',
  reqMapping: {
    body: [
      'space_id',
      'message',
      'title',
      'defer_start',
      'assistant_id',
      'command',
      'config',
      'context',
      'metadata',
      'stream_mode',
      'multitask_strategy',
      'on_disconnect',
      'durability',
      'idempotency_key',
    ],
  },
  resType: 'CreateTaskThreadResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const GetTaskThread = /*#__PURE__*/ createAPI<
  GetTaskThreadRequest,
  GetTaskThreadResponse
>({
  url: '/api/workbench/task_threads/:thread_id',
  method: 'GET',
  name: 'GetTaskThread',
  reqType: 'GetTaskThreadRequest',
  reqMapping: {
    path: ['thread_id'],
  },
  resType: 'GetTaskThreadResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadMessages = /*#__PURE__*/ createAPI<
  ListTaskThreadMessagesRequest,
  ListTaskThreadMessagesResponse
>({
  url: '/api/workbench/task_threads/:thread_id/messages',
  method: 'GET',
  name: 'ListTaskThreadMessages',
  reqType: 'ListTaskThreadMessagesRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['page', 'page_size'],
  },
  resType: 'ListTaskThreadMessagesResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const GenerateTaskThreadSuggestions = /*#__PURE__*/ createAPI<
  GenerateTaskThreadSuggestionsRequest,
  GenerateTaskThreadSuggestionsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/suggestions',
  method: 'POST',
  name: 'GenerateTaskThreadSuggestions',
  reqType: 'GenerateTaskThreadSuggestionsRequest',
  reqMapping: {
    path: ['thread_id'],
    body: ['messages', 'n', 'model_name'],
  },
  resType: 'GenerateTaskThreadSuggestionsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const AppendTaskThreadMessage = /*#__PURE__*/ createAPI<
  AppendTaskThreadMessageRequest,
  AppendTaskThreadMessageResponse
>({
  url: '/api/workbench/task_threads/:thread_id/messages',
  method: 'POST',
  name: 'AppendTaskThreadMessage',
  reqType: 'AppendTaskThreadMessageRequest',
  reqMapping: {
    path: ['thread_id'],
    body: ['run_id', 'role', 'content', 'metadata'],
  },
  resType: 'AppendTaskThreadMessageResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadRuns = /*#__PURE__*/ createAPI<
  ListTaskThreadRunsRequest,
  ListTaskThreadRunsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/runs',
  method: 'GET',
  name: 'ListTaskThreadRuns',
  reqType: 'ListTaskThreadRunsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['parent_run_id', 'status', 'page', 'page_size'],
  },
  resType: 'ListTaskThreadRunsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadRunEvents = /*#__PURE__*/ createAPI<
  ListTaskThreadRunEventsRequest,
  ListTaskThreadRunEventsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/run_events',
  method: 'GET',
  name: 'ListTaskThreadRunEvents',
  reqType: 'ListTaskThreadRunEventsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['run_id', 'page', 'page_size'],
  },
  resType: 'ListTaskThreadRunEventsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const GetTaskThreadTokenUsage = /*#__PURE__*/ createAPI<
  GetTaskThreadTokenUsageRequest,
  GetTaskThreadTokenUsageResponse
>({
  url: '/api/workbench/task_threads/:thread_id/token_usage',
  method: 'GET',
  name: 'GetTaskThreadTokenUsage',
  reqType: 'GetTaskThreadTokenUsageRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['run_id', 'include_child_runs', 'source', 'page', 'page_size'],
  },
  resType: 'GetTaskThreadTokenUsageResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadMemories = /*#__PURE__*/ createAPI<
  ListTaskThreadMemoriesRequest,
  ListTaskThreadMemoriesResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories',
  method: 'GET',
  name: 'ListTaskThreadMemories',
  reqType: 'ListTaskThreadMemoriesRequest',
  reqMapping: {
    path: ['thread_id'],
    query: [
      'run_id',
      'scope',
      'scopes',
      'q',
      'include_expired',
      'include_deleted',
      'page',
      'page_size',
    ],
  },
  resType: 'ListTaskThreadMemoriesResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ExportTaskThreadMemories = /*#__PURE__*/ createAPI<
  ExportTaskThreadMemoriesRequest,
  ExportTaskThreadMemoriesResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/export',
  method: 'GET',
  name: 'ExportTaskThreadMemories',
  reqType: 'ExportTaskThreadMemoriesRequest',
  reqMapping: {
    path: ['thread_id'],
    query: [
      'run_id',
      'scope',
      'scopes',
      'q',
      'include_expired',
      'include_deleted',
      'limit',
    ],
  },
  resType: 'ExportTaskThreadMemoriesResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ImportTaskThreadMemories = /*#__PURE__*/ createAPI<
  ImportTaskThreadMemoriesRequest,
  ImportTaskThreadMemoriesResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/import',
  method: 'POST',
  name: 'ImportTaskThreadMemories',
  reqType: 'ImportTaskThreadMemoriesRequest',
  reqMapping: {
    path: ['thread_id'],
    body: ['memories'],
  },
  resType: 'ImportTaskThreadMemoriesResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const UpdateTaskThreadMemory = /*#__PURE__*/ createAPI<
  UpdateTaskThreadMemoryRequest,
  UpdateTaskThreadMemoryResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/:memory_id',
  method: 'PUT',
  name: 'UpdateTaskThreadMemory',
  reqType: 'UpdateTaskThreadMemoryRequest',
  reqMapping: {
    path: ['thread_id', 'memory_id'],
    body: [
      'run_id',
      'scope',
      'content',
      'metadata',
      'score',
      'confidence',
      'source_type',
      'source_id',
      'correction_of_memory_id',
      'corrected_at',
      'expires_at',
    ],
  },
  resType: 'UpdateTaskThreadMemoryResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const DeleteTaskThreadMemory = /*#__PURE__*/ createAPI<
  DeleteTaskThreadMemoryRequest,
  DeleteTaskThreadMemoryResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/:memory_id',
  method: 'DELETE',
  name: 'DeleteTaskThreadMemory',
  reqType: 'DeleteTaskThreadMemoryRequest',
  reqMapping: {
    path: ['thread_id', 'memory_id'],
  },
  resType: 'DeleteTaskThreadMemoryResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ClearTaskThreadMemories = /*#__PURE__*/ createAPI<
  ClearTaskThreadMemoriesRequest,
  ClearTaskThreadMemoriesResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/clear',
  method: 'POST',
  name: 'ClearTaskThreadMemories',
  reqType: 'ClearTaskThreadMemoriesRequest',
  reqMapping: {
    path: ['thread_id'],
    body: ['run_id', 'scopes'],
  },
  resType: 'ClearTaskThreadMemoriesResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const RestoreTaskThreadMemory = /*#__PURE__*/ createAPI<
  RestoreTaskThreadMemoryRequest,
  RestoreTaskThreadMemoryResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/:memory_id/restore',
  method: 'POST',
  name: 'RestoreTaskThreadMemory',
  reqType: 'RestoreTaskThreadMemoryRequest',
  reqMapping: {
    path: ['thread_id', 'memory_id'],
  },
  resType: 'RestoreTaskThreadMemoryResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadMemoryAuditEvents = /*#__PURE__*/ createAPI<
  ListTaskThreadMemoryAuditEventsRequest,
  ListTaskThreadMemoryAuditEventsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/memories/audit_events',
  method: 'GET',
  name: 'ListTaskThreadMemoryAuditEvents',
  reqType: 'ListTaskThreadMemoryAuditEventsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['memory_id', 'page', 'page_size'],
  },
  resType: 'ListTaskThreadMemoryAuditEventsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadGuardrailAuditEvents = /*#__PURE__*/ createAPI<
  ListTaskThreadGuardrailAuditEventsRequest,
  ListTaskThreadGuardrailAuditEventsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/guardrail_audit_events',
  method: 'GET',
  name: 'ListTaskThreadGuardrailAuditEvents',
  reqType: 'ListTaskThreadGuardrailAuditEventsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['run_id', 'page', 'page_size'],
  },
  resType: 'ListTaskThreadGuardrailAuditEventsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadMCPRuntimeAuditEvents = /*#__PURE__*/ createAPI<
  ListTaskThreadMCPRuntimeAuditEventsRequest,
  ListTaskThreadMCPRuntimeAuditEventsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/mcp_runtime_audit_events',
  method: 'GET',
  name: 'ListTaskThreadMCPRuntimeAuditEvents',
  reqType: 'ListTaskThreadMCPRuntimeAuditEventsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['run_id', 'page', 'page_size'],
  },
  resType: 'ListTaskThreadMCPRuntimeAuditEventsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ExportTaskThreadGuardrailAuditEvents = /*#__PURE__*/ createAPI<
  ExportTaskThreadGuardrailAuditEventsRequest,
  ExportTaskThreadGuardrailAuditEventsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/guardrail_audit_events/export',
  method: 'GET',
  name: 'ExportTaskThreadGuardrailAuditEvents',
  reqType: 'ExportTaskThreadGuardrailAuditEventsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['run_id', 'page', 'page_size'],
  },
  resType: 'ExportTaskThreadGuardrailAuditEventsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskThreadArtifacts = /*#__PURE__*/ createAPI<
  ListTaskThreadArtifactsRequest,
  ListTaskThreadArtifactsResponse
>({
  url: '/api/workbench/task_threads/:thread_id/artifacts',
  method: 'GET',
  name: 'ListTaskThreadArtifacts',
  reqType: 'ListTaskThreadArtifactsRequest',
  reqMapping: {
    path: ['thread_id'],
    query: ['run_id', 'deleted_only', 'page', 'page_size', 'space_id'],
  },
  resType: 'ListTaskThreadArtifactsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const GetTaskThreadArtifactSignedURL = /*#__PURE__*/ createAPI<
  GetTaskThreadArtifactSignedURLRequest,
  GetTaskThreadArtifactSignedURLResponse
>({
  url: '/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url',
  method: 'GET',
  name: 'GetTaskThreadArtifactSignedURL',
  reqType: 'GetTaskThreadArtifactSignedURLRequest',
  reqMapping: {
    path: ['thread_id', 'artifact_id'],
    query: ['mode', 'ttl_seconds', 'space_id'],
  },
  resType: 'GetTaskThreadArtifactSignedURLResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const CreateTaskThreadRun = /*#__PURE__*/ createAPI<
  CreateTaskThreadRunRequest,
  CreateTaskThreadRunResponse
>({
  url: '/api/workbench/task_threads/:thread_id/runs',
  method: 'POST',
  name: 'CreateTaskThreadRun',
  reqType: 'CreateTaskThreadRunRequest',
  reqMapping: {
    path: ['thread_id'],
    body: [
      'assistant_id',
      'command',
      'input',
      'config',
      'context',
      'metadata',
      'stream_mode',
      'multitask_strategy',
      'on_disconnect',
      'durability',
      'idempotency_key',
      'message_content',
      'message_metadata',
    ],
  },
  resType: 'CreateTaskThreadRunResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ResumeTaskThreadRun = /*#__PURE__*/ createAPI<
  ResumeTaskThreadRunRequest,
  ResumeTaskThreadRunResponse
>({
  url: '/api/workbench/task_threads/:thread_id/runs/:run_id/resume',
  method: 'POST',
  name: 'ResumeTaskThreadRun',
  reqType: 'ResumeTaskThreadRunRequest',
  reqMapping: {
    path: ['thread_id', 'run_id'],
    body: ['interrupt_id', 'response', 'idempotency_key'],
  },
  resType: 'ResumeTaskThreadRunResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const CancelTaskThreadRun = /*#__PURE__*/ createAPI<
  CancelTaskThreadRunRequest,
  CancelTaskThreadRunResponse
>({
  url: '/api/workbench/task_threads/:thread_id/runs/:run_id/cancel',
  method: 'POST',
  name: 'CancelTaskThreadRun',
  reqType: 'CancelTaskThreadRunRequest',
  reqMapping: {
    path: ['thread_id', 'run_id'],
  },
  resType: 'CancelTaskThreadRunResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const RetryTaskThreadSubagentRun = /*#__PURE__*/ createAPI<
  RetryTaskThreadSubagentRunRequest,
  RetryTaskThreadSubagentRunResponse
>({
  url: '/api/workbench/task_threads/:thread_id/runs/:run_id/retry',
  method: 'POST',
  name: 'RetryTaskThreadSubagentRun',
  reqType: 'RetryTaskThreadSubagentRunRequest',
  reqMapping: {
    path: ['thread_id', 'run_id'],
    body: ['idempotency_key'],
  },
  resType: 'RetryTaskThreadSubagentRunResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const CancelTask = /*#__PURE__*/ createAPI<
  GetTaskRequest,
  GetTaskResponse
>({
  url: '/api/workbench/tasks/:task_id/cancel',
  method: 'POST',
  name: 'CancelTask',
  reqType: 'GetTaskRequest',
  reqMapping: {
    path: ['task_id'],
  },
  resType: 'GetTaskResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const RetryTask = /*#__PURE__*/ createAPI<
  GetTaskRequest,
  GetTaskResponse
>({
  url: '/api/workbench/tasks/:task_id/retry',
  method: 'POST',
  name: 'RetryTask',
  reqType: 'GetTaskRequest',
  reqMapping: {
    path: ['task_id'],
  },
  resType: 'GetTaskResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
export const ListTaskEvents = /*#__PURE__*/ createAPI<
  GetTaskRequest,
  TaskEventsResponse
>({
  url: '/api/workbench/tasks/:task_id/events',
  method: 'GET',
  name: 'ListTaskEvents',
  reqType: 'GetTaskRequest',
  reqMapping: {
    path: ['task_id'],
  },
  resType: 'TaskEventsResponse',
  schemaRoot: 'api://schemas/idl_workbench_task',
  service: 'workbenchTask',
});
