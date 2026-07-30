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

import * as thread_product from './thread_product';
export { thread_product };
import * as journal from './journal';
export { journal };
import * as base from './../base';
export { base };
import { createAPI } from './../../api/config';
export interface CanonicalRouteRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
}
export interface CanonicalRunRouteRequest {
  thread_id: string,
  run_id: string,
  "X-Coze-Space-ID": string,
}
export interface CanonicalThread {
  thread_id: string,
  created_at: string,
  updated_at: string,
  metadata: any,
  status: string,
  values: any,
  interrupts: any,
  coze: any,
}
export interface CanonicalRun {
  run_id: string,
  thread_id: string,
  assistant_id: string,
  status: string,
  created_at: string,
  updated_at: string,
  metadata: any,
  multitask_strategy: string,
  coze: any,
}
export interface CanonicalCheckpoint {
  thread_id: string,
  checkpoint_ns: string,
  checkpoint_id: string,
  checkpoint_map: any,
}
export interface CanonicalThreadState {
  values: any,
  next: string[],
  checkpoint: CanonicalCheckpoint,
  metadata: any,
  created_at: string,
  parent_checkpoint?: CanonicalCheckpoint,
  tasks: any,
  interrupts: any,
}
export interface CanonicalThreadUpdateStateResult {
  checkpoint: CanonicalCheckpoint,
  configurable: CanonicalCheckpoint,
}
export interface CanonicalMessage {
  message_id: string,
  thread_id: string,
  run_id: string,
  role: string,
  content: string,
  metadata: any,
  created_at: string,
  seq?: string,
}
export interface CanonicalMessagePage {
  data: CanonicalMessage[],
  has_more: boolean,
  next_before_seq?: string,
  next_after_seq?: string,
}
export interface CanonicalRunEventPage {
  data: journal.JournalEvent[],
  has_more: boolean,
  next_after_event_id?: string,
  attempt_id?: string,
  latest_sequence?: number,
  next_after_sequence?: number,
}
export interface CanonicalThreadListResponse {
  body: CanonicalThread[]
}
export interface CanonicalThreadStateListResponse {
  body: CanonicalThreadState[]
}
export interface CanonicalRunListResponse {
  body: CanonicalRun[]
}
export interface CanonicalValuesResponse {
  body: any
}
export interface CanonicalStreamResponse {
  body: string
}
export interface CanonicalEmptyResponse {}
export interface CreateCanonicalThreadRequest {
  thread_id?: string,
  metadata?: any,
  if_exists?: string,
  ttl?: any,
  supersteps?: any,
  coze?: any,
  "X-Coze-Space-ID": string,
}
export interface SearchCanonicalThreadsRequest {
  metadata?: any,
  status?: string,
  ids?: string[],
  limit?: number,
  offset?: number,
  sort_by?: string,
  sort_order?: string,
  values?: boolean,
  select?: string[],
  extract?: boolean,
  "X-Coze-Space-ID": string,
}
export interface GetCanonicalThreadRequest {
  thread_id: string,
  include?: string[],
  "X-Coze-Space-ID": string,
}
export interface PatchCanonicalThreadRequest {
  thread_id: string,
  Prefer?: string,
  metadata?: any,
  ttl?: any,
  "X-Coze-Space-ID": string,
}
export interface GetCanonicalThreadStateRequest {
  thread_id: string,
  checkpoint?: string,
  checkpoint_id?: string,
  subgraphs?: boolean,
  "X-Coze-Space-ID": string,
}
export interface UpdateCanonicalThreadStateRequest {
  thread_id: string,
  values?: any,
  as_node?: string,
  checkpoint?: any,
  checkpoint_id?: string,
  "X-Coze-Space-ID": string,
}
export interface GetCanonicalThreadHistoryRequest {
  thread_id: string,
  limit?: number,
  before?: string,
  checkpoint?: string,
  checkpoint_id?: string,
  "X-Coze-Space-ID": string,
}
export interface PostCanonicalThreadHistoryRequest {
  thread_id: string,
  limit?: number,
  before?: string,
  checkpoint?: any,
  checkpoint_id?: string,
  "X-Coze-Space-ID": string,
}
export interface ListCanonicalThreadMessagesRequest {
  thread_id: string,
  before_seq?: string,
  after_seq?: string,
  limit?: number,
  "X-Coze-Space-ID": string,
}
export interface ListCanonicalRunsRequest {
  thread_id: string,
  status?: string,
  limit?: number,
  offset?: number,
  parent_run_id?: string,
  select?: string[],
  "X-Coze-Space-ID": string,
}
export interface CreateCanonicalRunRequest {
  thread_id: string,
  assistant_id: string,
  input?: any,
  command?: any,
  metadata?: any,
  config?: any,
  context?: any,
  stream_mode?: any,
  multitask_strategy?: string,
  on_disconnect?: string,
  durability?: string,
  stream_resumable?: boolean,
  stream_subgraphs?: boolean,
  if_not_exists?: string,
  webhook?: any,
  on_completion?: any,
  after_seconds?: any,
  feedback_keys?: any,
  interrupt_before?: any,
  interrupt_after?: any,
  checkpoint?: any,
  checkpoint_id?: any,
  langsmith_tracer?: any,
  "Idempotency-Key"?: string,
  "X-Coze-Space-ID": string,
  coze?: any,
}
export interface WaitCanonicalRunRequest {
  thread_id: string,
  assistant_id: string,
  input?: any,
  command?: any,
  metadata?: any,
  config?: any,
  context?: any,
  stream_mode?: any,
  multitask_strategy?: string,
  on_disconnect?: string,
  durability?: string,
  stream_resumable?: boolean,
  stream_subgraphs?: boolean,
  if_not_exists?: string,
  webhook?: any,
  on_completion?: any,
  after_seconds?: any,
  feedback_keys?: any,
  interrupt_before?: any,
  interrupt_after?: any,
  checkpoint?: any,
  checkpoint_id?: any,
  langsmith_tracer?: any,
  raise_error?: boolean,
  "Idempotency-Key"?: string,
  "X-Coze-Space-ID": string,
  coze?: any,
}
export interface ReconnectCanonicalRunStreamRequest {
  thread_id: string,
  run_id: string,
  after_event_id?: string,
  cancel_on_disconnect?: string,
  "Last-Event-ID"?: string,
  stream_mode?: string[],
  "X-Coze-Space-ID": string,
  journal_protocol_version?: string,
}
export interface JoinCanonicalRunRequest {
  thread_id: string,
  run_id: string,
  cancel_on_disconnect?: string,
  "X-Coze-Space-ID": string,
}
export interface CancelCanonicalRunRequest {
  thread_id: string,
  run_id: string,
  action?: string,
  wait?: string,
  "X-Coze-Space-ID": string,
}
export interface ResumeCanonicalRunRequest {
  thread_id: string,
  run_id: string,
  interrupt_id?: string,
  response?: any,
  "X-Coze-Space-ID": string,
}
export interface ListCanonicalRunEventsRequest {
  thread_id: string,
  run_id: string,
  after_event_id?: string,
  event_types?: string[],
  limit?: number,
  "X-Coze-Space-ID": string,
  attempt_id?: string,
  after_sequence?: number,
}
export interface ListCanonicalRunMessagesRequest {
  thread_id: string,
  run_id: string,
  before_seq?: string,
  after_seq?: string,
  limit?: number,
  "X-Coze-Space-ID": string,
}
export const CreateCanonicalThread = /*#__PURE__*/createAPI<CreateCanonicalThreadRequest, CanonicalThread>({
  "url": "/api/workbench/threads",
  "method": "POST",
  "name": "CreateCanonicalThread",
  "reqType": "CreateCanonicalThreadRequest",
  "reqMapping": {
    "body": ["thread_id", "metadata", "if_exists", "ttl", "supersteps", "coze"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThread",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const SearchCanonicalThreads = /*#__PURE__*/createAPI<SearchCanonicalThreadsRequest, CanonicalThreadListResponse['body']>({
  "url": "/api/workbench/threads/search",
  "method": "POST",
  "name": "SearchCanonicalThreads",
  "reqType": "SearchCanonicalThreadsRequest",
  "reqMapping": {
    "body": ["metadata", "status", "ids", "limit", "offset", "sort_by", "sort_order", "values", "select", "extract"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThreadListResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalThread = /*#__PURE__*/createAPI<GetCanonicalThreadRequest, CanonicalThread>({
  "url": "/api/workbench/threads/:thread_id",
  "method": "GET",
  "name": "GetCanonicalThread",
  "reqType": "GetCanonicalThreadRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "query": ["include"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThread",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const PatchCanonicalThread = /*#__PURE__*/createAPI<PatchCanonicalThreadRequest, CanonicalThread>({
  "url": "/api/workbench/threads/:thread_id",
  "method": "PATCH",
  "name": "PatchCanonicalThread",
  "reqType": "PatchCanonicalThreadRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["Prefer", "X-Coze-Space-ID"],
    "body": ["metadata", "ttl"]
  },
  "resType": "CanonicalThread",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const DeleteCanonicalThread = /*#__PURE__*/createAPI<CanonicalRouteRequest, CanonicalEmptyResponse>({
  "url": "/api/workbench/threads/:thread_id",
  "method": "DELETE",
  "name": "DeleteCanonicalThread",
  "reqType": "CanonicalRouteRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalEmptyResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalThreadState = /*#__PURE__*/createAPI<GetCanonicalThreadStateRequest, CanonicalThreadState>({
  "url": "/api/workbench/threads/:thread_id/state",
  "method": "GET",
  "name": "GetCanonicalThreadState",
  "reqType": "GetCanonicalThreadStateRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "query": ["checkpoint", "checkpoint_id", "subgraphs"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThreadState",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const UpdateCanonicalThreadState = /*#__PURE__*/createAPI<UpdateCanonicalThreadStateRequest, CanonicalThreadUpdateStateResult>({
  "url": "/api/workbench/threads/:thread_id/state",
  "method": "POST",
  "name": "UpdateCanonicalThreadState",
  "reqType": "UpdateCanonicalThreadStateRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "body": ["values", "as_node", "checkpoint", "checkpoint_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThreadUpdateStateResult",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalThreadHistory = /*#__PURE__*/createAPI<GetCanonicalThreadHistoryRequest, CanonicalThreadStateListResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/history",
  "method": "GET",
  "name": "GetCanonicalThreadHistory",
  "reqType": "GetCanonicalThreadHistoryRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "query": ["limit", "before", "checkpoint", "checkpoint_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThreadStateListResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const PostCanonicalThreadHistory = /*#__PURE__*/createAPI<PostCanonicalThreadHistoryRequest, CanonicalThreadStateListResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/history",
  "method": "POST",
  "name": "PostCanonicalThreadHistory",
  "reqType": "PostCanonicalThreadHistoryRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "body": ["limit", "before", "checkpoint", "checkpoint_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalThreadStateListResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadMessages = /*#__PURE__*/createAPI<ListCanonicalThreadMessagesRequest, CanonicalMessagePage>({
  "url": "/api/workbench/threads/:thread_id/messages",
  "method": "GET",
  "name": "ListCanonicalThreadMessages",
  "reqType": "ListCanonicalThreadMessagesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "query": ["before_seq", "after_seq", "limit"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalMessagePage",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalRuns = /*#__PURE__*/createAPI<ListCanonicalRunsRequest, CanonicalRunListResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/runs",
  "method": "GET",
  "name": "ListCanonicalRuns",
  "reqType": "ListCanonicalRunsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "query": ["status", "limit", "offset", "parent_run_id", "select"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalRunListResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const CreateCanonicalRun = /*#__PURE__*/createAPI<CreateCanonicalRunRequest, CanonicalRun>({
  "url": "/api/workbench/threads/:thread_id/runs",
  "method": "POST",
  "name": "CreateCanonicalRun",
  "reqType": "CreateCanonicalRunRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "body": ["assistant_id", "input", "command", "metadata", "config", "context", "stream_mode", "multitask_strategy", "on_disconnect", "durability", "stream_resumable", "stream_subgraphs", "if_not_exists", "webhook", "on_completion", "after_seconds", "feedback_keys", "interrupt_before", "interrupt_after", "checkpoint", "checkpoint_id", "langsmith_tracer", "coze"],
    "header": ["Idempotency-Key", "X-Coze-Space-ID"]
  },
  "resType": "CanonicalRun",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const StreamCanonicalRun = /*#__PURE__*/createAPI<CreateCanonicalRunRequest, CanonicalStreamResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/runs/stream",
  "method": "POST",
  "name": "StreamCanonicalRun",
  "reqType": "CreateCanonicalRunRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "body": ["assistant_id", "input", "command", "metadata", "config", "context", "stream_mode", "multitask_strategy", "on_disconnect", "durability", "stream_resumable", "stream_subgraphs", "if_not_exists", "webhook", "on_completion", "after_seconds", "feedback_keys", "interrupt_before", "interrupt_after", "checkpoint", "checkpoint_id", "langsmith_tracer", "coze"],
    "header": ["Idempotency-Key", "X-Coze-Space-ID"]
  },
  "resType": "CanonicalStreamResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const WaitCanonicalRun = /*#__PURE__*/createAPI<WaitCanonicalRunRequest, CanonicalValuesResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/runs/wait",
  "method": "POST",
  "name": "WaitCanonicalRun",
  "reqType": "WaitCanonicalRunRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "body": ["assistant_id", "input", "command", "metadata", "config", "context", "stream_mode", "multitask_strategy", "on_disconnect", "durability", "stream_resumable", "stream_subgraphs", "if_not_exists", "webhook", "on_completion", "after_seconds", "feedback_keys", "interrupt_before", "interrupt_after", "checkpoint", "checkpoint_id", "langsmith_tracer", "raise_error", "coze"],
    "header": ["Idempotency-Key", "X-Coze-Space-ID"]
  },
  "resType": "CanonicalValuesResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalRun = /*#__PURE__*/createAPI<CanonicalRunRouteRequest, CanonicalRun>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id",
  "method": "GET",
  "name": "GetCanonicalRun",
  "reqType": "CanonicalRunRouteRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalRun",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ReconnectCanonicalRunStream = /*#__PURE__*/createAPI<ReconnectCanonicalRunStreamRequest, CanonicalStreamResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/stream",
  "method": "GET",
  "name": "ReconnectCanonicalRunStream",
  "reqType": "ReconnectCanonicalRunStreamRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "query": ["after_event_id", "cancel_on_disconnect", "stream_mode", "journal_protocol_version"],
    "header": ["Last-Event-ID", "X-Coze-Space-ID"]
  },
  "resType": "CanonicalStreamResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const JoinCanonicalRun = /*#__PURE__*/createAPI<JoinCanonicalRunRequest, CanonicalValuesResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/join",
  "method": "GET",
  "name": "JoinCanonicalRun",
  "reqType": "JoinCanonicalRunRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "query": ["cancel_on_disconnect"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalValuesResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const CancelCanonicalRun = /*#__PURE__*/createAPI<CancelCanonicalRunRequest, CanonicalEmptyResponse>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/cancel",
  "method": "POST",
  "name": "CancelCanonicalRun",
  "reqType": "CancelCanonicalRunRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "query": ["action", "wait"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalEmptyResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ResumeCanonicalRun = /*#__PURE__*/createAPI<ResumeCanonicalRunRequest, CanonicalRun>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/resume",
  "method": "POST",
  "name": "ResumeCanonicalRun",
  "reqType": "ResumeCanonicalRunRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "body": ["interrupt_id", "response"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalRun",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalRunEvents = /*#__PURE__*/createAPI<ListCanonicalRunEventsRequest, CanonicalRunEventPage>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/events",
  "method": "GET",
  "name": "ListCanonicalRunEvents",
  "reqType": "ListCanonicalRunEventsRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "query": ["after_event_id", "event_types", "limit", "attempt_id", "after_sequence"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalRunEventPage",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalRunMessages = /*#__PURE__*/createAPI<ListCanonicalRunMessagesRequest, CanonicalMessagePage>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/messages",
  "method": "GET",
  "name": "ListCanonicalRunMessages",
  "reqType": "ListCanonicalRunMessagesRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "query": ["before_seq", "after_seq", "limit"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "CanonicalMessagePage",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const AppendCanonicalThreadMessage = /*#__PURE__*/createAPI<thread_product.AppendCanonicalThreadMessageRequest, CanonicalMessage>({
  "url": "/api/workbench/threads/:thread_id/messages",
  "method": "POST",
  "name": "AppendCanonicalThreadMessage",
  "reqType": "thread_product.AppendCanonicalThreadMessageRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "body": ["run_id", "role", "content", "metadata", "append_mode"]
  },
  "resType": "CanonicalMessage",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GenerateCanonicalThreadSuggestions = /*#__PURE__*/createAPI<thread_product.GenerateCanonicalThreadSuggestionsRequest, thread_product.CanonicalSuggestionResponse>({
  "url": "/api/workbench/threads/:thread_id/suggestions",
  "method": "POST",
  "name": "GenerateCanonicalThreadSuggestions",
  "reqType": "thread_product.GenerateCanonicalThreadSuggestionsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "body": ["n", "model_name", "model_type"]
  },
  "resType": "thread_product.CanonicalSuggestionResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadUploads = /*#__PURE__*/createAPI<thread_product.CanonicalProductThreadRequest, thread_product.CanonicalUploadListResponse>({
  "url": "/api/workbench/threads/:thread_id/uploads",
  "method": "GET",
  "name": "ListCanonicalThreadUploads",
  "reqType": "thread_product.CanonicalProductThreadRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalUploadListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const UploadCanonicalThreadFiles = /*#__PURE__*/createAPI<thread_product.UploadCanonicalThreadFilesRequest, thread_product.CanonicalUploadResponse>({
  "url": "/api/workbench/threads/:thread_id/uploads",
  "method": "POST",
  "name": "UploadCanonicalThreadFiles",
  "reqType": "thread_product.UploadCanonicalThreadFilesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalUploadResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const DeleteCanonicalThreadUpload = /*#__PURE__*/createAPI<thread_product.DeleteCanonicalThreadUploadRequest, thread_product.CanonicalProductEmptyResponse>({
  "url": "/api/workbench/threads/:thread_id/uploads/:file_id",
  "method": "DELETE",
  "name": "DeleteCanonicalThreadUpload",
  "reqType": "thread_product.DeleteCanonicalThreadUploadRequest",
  "reqMapping": {
    "path": ["thread_id", "file_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalProductEmptyResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadArtifacts = /*#__PURE__*/createAPI<thread_product.ListCanonicalThreadArtifactsRequest, thread_product.CanonicalArtifactListResponse>({
  "url": "/api/workbench/threads/:thread_id/artifacts",
  "method": "GET",
  "name": "ListCanonicalThreadArtifacts",
  "reqType": "thread_product.ListCanonicalThreadArtifactsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "deleted_only", "limit", "offset", "collection_id"]
  },
  "resType": "thread_product.CanonicalArtifactListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalThreadArtifactContent = /*#__PURE__*/createAPI<thread_product.GetCanonicalThreadArtifactContentRequest, thread_product.CanonicalArtifactContentResponse['body']>({
  "url": "/api/workbench/threads/:thread_id/artifacts/:artifact_id/content",
  "method": "GET",
  "name": "GetCanonicalThreadArtifactContent",
  "reqType": "thread_product.GetCanonicalThreadArtifactContentRequest",
  "reqMapping": {
    "path": ["thread_id", "artifact_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["mode"]
  },
  "resType": "thread_product.CanonicalArtifactContentResponse['body']",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalThreadArtifactSignedURL = /*#__PURE__*/createAPI<thread_product.GetCanonicalThreadArtifactSignedURLRequest, thread_product.CanonicalArtifactSignedURLResponse>({
  "url": "/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url",
  "method": "GET",
  "name": "GetCanonicalThreadArtifactSignedURL",
  "reqType": "thread_product.GetCanonicalThreadArtifactSignedURLRequest",
  "reqMapping": {
    "path": ["thread_id", "artifact_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["mode", "ttl_seconds"]
  },
  "resType": "thread_product.CanonicalArtifactSignedURLResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const DeleteCanonicalThreadArtifact = /*#__PURE__*/createAPI<thread_product.CanonicalArtifactRouteRequest, thread_product.CanonicalProductEmptyResponse>({
  "url": "/api/workbench/threads/:thread_id/artifacts/:artifact_id",
  "method": "DELETE",
  "name": "DeleteCanonicalThreadArtifact",
  "reqType": "thread_product.CanonicalArtifactRouteRequest",
  "reqMapping": {
    "path": ["thread_id", "artifact_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalProductEmptyResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const RestoreCanonicalThreadArtifact = /*#__PURE__*/createAPI<thread_product.CanonicalArtifactRouteRequest, thread_product.CanonicalArtifactRestoreResponse>({
  "url": "/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore",
  "method": "POST",
  "name": "RestoreCanonicalThreadArtifact",
  "reqType": "thread_product.CanonicalArtifactRouteRequest",
  "reqMapping": {
    "path": ["thread_id", "artifact_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalArtifactRestoreResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ReviewCanonicalThreadArtifactScan = /*#__PURE__*/createAPI<thread_product.ReviewCanonicalThreadArtifactScanRequest, thread_product.CanonicalArtifactScanReviewResponse>({
  "url": "/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review",
  "method": "POST",
  "name": "ReviewCanonicalThreadArtifactScan",
  "reqType": "thread_product.ReviewCanonicalThreadArtifactScanRequest",
  "reqMapping": {
    "path": ["thread_id", "artifact_id"],
    "header": ["X-Coze-Space-ID"],
    "body": ["decision", "reason"]
  },
  "resType": "thread_product.CanonicalArtifactScanReviewResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadArtifactScanJobs = /*#__PURE__*/createAPI<thread_product.ListCanonicalThreadArtifactScanJobsRequest, thread_product.CanonicalArtifactScanJobListResponse>({
  "url": "/api/workbench/threads/:thread_id/artifact_scan_jobs",
  "method": "GET",
  "name": "ListCanonicalThreadArtifactScanJobs",
  "reqType": "thread_product.ListCanonicalThreadArtifactScanJobsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "artifact_id", "status", "scanner", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalArtifactScanJobListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const RetryCanonicalThreadArtifactScanJob = /*#__PURE__*/createAPI<thread_product.RetryCanonicalThreadArtifactScanJobRequest, thread_product.CanonicalArtifactScanJobRetryResponse>({
  "url": "/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry",
  "method": "POST",
  "name": "RetryCanonicalThreadArtifactScanJob",
  "reqType": "thread_product.RetryCanonicalThreadArtifactScanJobRequest",
  "reqMapping": {
    "path": ["thread_id", "job_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalArtifactScanJobRetryResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalThreadTokenUsage = /*#__PURE__*/createAPI<thread_product.GetCanonicalThreadTokenUsageRequest, thread_product.CanonicalTokenUsageResponse>({
  "url": "/api/workbench/threads/:thread_id/token_usage",
  "method": "GET",
  "name": "GetCanonicalThreadTokenUsage",
  "reqType": "thread_product.GetCanonicalThreadTokenUsageRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "include_child_runs", "source", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalTokenUsageResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadMemories = /*#__PURE__*/createAPI<thread_product.ListCanonicalThreadMemoriesRequest, thread_product.CanonicalMemoryListResponse>({
  "url": "/api/workbench/threads/:thread_id/memories",
  "method": "GET",
  "name": "ListCanonicalThreadMemories",
  "reqType": "thread_product.ListCanonicalThreadMemoriesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "scope", "scopes", "q", "include_expired", "include_deleted", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalMemoryListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const UpdateCanonicalThreadMemory = /*#__PURE__*/createAPI<thread_product.UpdateCanonicalThreadMemoryRequest, thread_product.CanonicalMemoryUpdateResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/:memory_id",
  "method": "PUT",
  "name": "UpdateCanonicalThreadMemory",
  "reqType": "thread_product.UpdateCanonicalThreadMemoryRequest",
  "reqMapping": {
    "path": ["thread_id", "memory_id"],
    "header": ["X-Coze-Space-ID"],
    "body": ["run_id", "scope", "content", "metadata", "score", "confidence", "source_type", "source_id", "correction_of_memory_id", "corrected_at", "expires_at"]
  },
  "resType": "thread_product.CanonicalMemoryUpdateResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const DeleteCanonicalThreadMemory = /*#__PURE__*/createAPI<thread_product.CanonicalMemoryRouteRequest, thread_product.CanonicalProductEmptyResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/:memory_id",
  "method": "DELETE",
  "name": "DeleteCanonicalThreadMemory",
  "reqType": "thread_product.CanonicalMemoryRouteRequest",
  "reqMapping": {
    "path": ["thread_id", "memory_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalProductEmptyResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const RestoreCanonicalThreadMemory = /*#__PURE__*/createAPI<thread_product.CanonicalMemoryRouteRequest, thread_product.CanonicalMemoryRestoreResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/:memory_id/restore",
  "method": "POST",
  "name": "RestoreCanonicalThreadMemory",
  "reqType": "thread_product.CanonicalMemoryRouteRequest",
  "reqMapping": {
    "path": ["thread_id", "memory_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CanonicalMemoryRestoreResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ClearCanonicalThreadMemories = /*#__PURE__*/createAPI<thread_product.ClearCanonicalThreadMemoriesRequest, thread_product.CanonicalMemoryClearResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/clear",
  "method": "POST",
  "name": "ClearCanonicalThreadMemories",
  "reqType": "thread_product.ClearCanonicalThreadMemoriesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "body": ["run_id", "scopes"]
  },
  "resType": "thread_product.CanonicalMemoryClearResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ExportCanonicalThreadMemories = /*#__PURE__*/createAPI<thread_product.ExportCanonicalThreadMemoriesRequest, thread_product.CanonicalMemoryExportResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/export",
  "method": "GET",
  "name": "ExportCanonicalThreadMemories",
  "reqType": "thread_product.ExportCanonicalThreadMemoriesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "scope", "scopes", "q", "include_expired", "include_deleted", "limit"]
  },
  "resType": "thread_product.CanonicalMemoryExportResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ImportCanonicalThreadMemories = /*#__PURE__*/createAPI<thread_product.ImportCanonicalThreadMemoriesRequest, thread_product.CanonicalMemoryImportResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/import",
  "method": "POST",
  "name": "ImportCanonicalThreadMemories",
  "reqType": "thread_product.ImportCanonicalThreadMemoriesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "body": ["memories"]
  },
  "resType": "thread_product.CanonicalMemoryImportResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadMemoryAuditEvents = /*#__PURE__*/createAPI<thread_product.ListCanonicalThreadMemoryAuditEventsRequest, thread_product.CanonicalMemoryAuditEventListResponse>({
  "url": "/api/workbench/threads/:thread_id/memories/audit_events",
  "method": "GET",
  "name": "ListCanonicalThreadMemoryAuditEvents",
  "reqType": "thread_product.ListCanonicalThreadMemoryAuditEventsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["memory_id", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalMemoryAuditEventListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadGuardrailAuditEvents = /*#__PURE__*/createAPI<thread_product.ListCanonicalThreadGuardrailAuditEventsRequest, thread_product.CanonicalGuardrailAuditEventListResponse>({
  "url": "/api/workbench/threads/:thread_id/guardrail_audit_events",
  "method": "GET",
  "name": "ListCanonicalThreadGuardrailAuditEvents",
  "reqType": "thread_product.ListCanonicalThreadGuardrailAuditEventsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalGuardrailAuditEventListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ExportCanonicalThreadGuardrailAuditEvents = /*#__PURE__*/createAPI<thread_product.ExportCanonicalThreadGuardrailAuditEventsRequest, thread_product.CanonicalGuardrailAuditExportResponse>({
  "url": "/api/workbench/threads/:thread_id/guardrail_audit_events/export",
  "method": "GET",
  "name": "ExportCanonicalThreadGuardrailAuditEvents",
  "reqType": "thread_product.ExportCanonicalThreadGuardrailAuditEventsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalGuardrailAuditExportResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const ListCanonicalThreadMCPRuntimeAuditEvents = /*#__PURE__*/createAPI<thread_product.ListCanonicalThreadMCPRuntimeAuditEventsRequest, thread_product.CanonicalMCPRuntimeAuditEventListResponse>({
  "url": "/api/workbench/threads/:thread_id/mcp_runtime_audit_events",
  "method": "GET",
  "name": "ListCanonicalThreadMCPRuntimeAuditEvents",
  "reqType": "thread_product.ListCanonicalThreadMCPRuntimeAuditEventsRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["run_id", "limit", "offset"]
  },
  "resType": "thread_product.CanonicalMCPRuntimeAuditEventListResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const RetryCanonicalSubagentRun = /*#__PURE__*/createAPI<thread_product.RetryCanonicalSubagentRunRequest, CanonicalRun>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/retry",
  "method": "POST",
  "name": "RetryCanonicalSubagentRun",
  "reqType": "thread_product.RetryCanonicalSubagentRunRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "header": ["X-Coze-Space-ID", "Idempotency-Key"]
  },
  "resType": "CanonicalRun",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalRunJournal = /*#__PURE__*/createAPI<journal.GetCanonicalRunJournalRequest, journal.JournalBootstrap>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/journal",
  "method": "GET",
  "name": "GetCanonicalRunJournal",
  "reqType": "journal.GetCanonicalRunJournalRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["attempt_id", "after_sequence", "limit", "journal_protocol_version", "after_event_id"]
  },
  "resType": "journal.JournalBootstrap",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalRunSnapshot = /*#__PURE__*/createAPI<journal.GetCanonicalRunSnapshotRequest, journal.JournalSnapshotEnvelope>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id",
  "method": "GET",
  "name": "GetCanonicalRunSnapshot",
  "reqType": "journal.GetCanonicalRunSnapshotRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id", "snapshot_id"],
    "header": ["X-Coze-Space-ID"],
    "query": ["cursor", "limit"]
  },
  "resType": "journal.JournalSnapshotEnvelope",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const AuditCanonicalRunSnapshotAction = /*#__PURE__*/createAPI<journal.AuditCanonicalRunSnapshotActionRequest, journal.JournalSnapshotActionAuditResponse>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id/actions",
  "method": "POST",
  "name": "AuditCanonicalRunSnapshotAction",
  "reqType": "journal.AuditCanonicalRunSnapshotActionRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id", "snapshot_id"],
    "header": ["X-Coze-Space-ID", "Idempotency-Key"],
    "body": ["action"]
  },
  "resType": "journal.JournalSnapshotActionAuditResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const RecoverCanonicalRunJournal = /*#__PURE__*/createAPI<journal.RecoverCanonicalRunJournalRequest, journal.RecoverCanonicalRunJournalResponse>({
  "url": "/api/workbench/threads/:thread_id/runs/:run_id/recover",
  "method": "POST",
  "name": "RecoverCanonicalRunJournal",
  "reqType": "journal.RecoverCanonicalRunJournalRequest",
  "reqMapping": {
    "path": ["thread_id", "run_id"],
    "header": ["X-Coze-Space-ID", "Idempotency-Key"],
    "body": ["source_attempt_id", "action", "confirmed"]
  },
  "resType": "journal.RecoverCanonicalRunJournalResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const GetCanonicalJournalSettings = /*#__PURE__*/createAPI<journal.GetCanonicalJournalSettingsRequest, journal.JournalUserSettings>({
  "url": "/api/workbench/journal/settings",
  "method": "GET",
  "name": "GetCanonicalJournalSettings",
  "reqType": "journal.GetCanonicalJournalSettingsRequest",
  "reqMapping": {},
  "resType": "journal.JournalUserSettings",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const PatchCanonicalJournalSettings = /*#__PURE__*/createAPI<journal.PatchCanonicalJournalSettingsRequest, journal.JournalUserSettings>({
  "url": "/api/workbench/journal/settings",
  "method": "PATCH",
  "name": "PatchCanonicalJournalSettings",
  "reqType": "journal.PatchCanonicalJournalSettingsRequest",
  "reqMapping": {
    "body": ["split_ratio", "revision"]
  },
  "resType": "journal.JournalUserSettings",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
export const CopyCanonicalThreadArtifactLink = /*#__PURE__*/createAPI<thread_product.CopyCanonicalThreadArtifactLinkRequest, thread_product.CopyCanonicalThreadArtifactLinkResponse>({
  "url": "/api/workbench/threads/:thread_id/artifacts/:artifact_id/copy_link",
  "method": "POST",
  "name": "CopyCanonicalThreadArtifactLink",
  "reqType": "thread_product.CopyCanonicalThreadArtifactLinkRequest",
  "reqMapping": {
    "path": ["thread_id", "artifact_id"],
    "header": ["X-Coze-Space-ID"]
  },
  "resType": "thread_product.CopyCanonicalThreadArtifactLinkResponse",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});