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
export interface CanonicalRouteRequest {
  thread_id?: string
}
export interface CanonicalRunRouteRequest {
  thread_id: string,
  run_id: string,
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
export interface CanonicalMessagePage {
  data: any,
  has_more: boolean,
  next_before_seq?: string,
  next_after_seq?: string,
}
export interface CanonicalRunEventPage {
  data: any,
  has_more: boolean,
  next_after_event_id?: string,
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
}
export interface GetCanonicalThreadRequest {
  thread_id: string,
  include?: string[],
}
export interface PatchCanonicalThreadRequest {
  thread_id: string,
  Prefer?: string,
  metadata?: any,
  ttl?: any,
}
export interface GetCanonicalThreadStateRequest {
  thread_id: string,
  checkpoint?: string,
  checkpoint_id?: string,
  subgraphs?: boolean,
}
export interface UpdateCanonicalThreadStateRequest {
  thread_id: string,
  values?: any,
  as_node?: string,
  checkpoint?: any,
  checkpoint_id?: string,
}
export interface GetCanonicalThreadHistoryRequest {
  thread_id: string,
  limit?: number,
  before?: string,
  checkpoint?: string,
  checkpoint_id?: string,
}
export interface PostCanonicalThreadHistoryRequest {
  thread_id: string,
  limit?: number,
  before?: string,
  checkpoint?: any,
  checkpoint_id?: string,
}
export interface ListCanonicalThreadMessagesRequest {
  thread_id: string,
  before_seq?: string,
  after_seq?: string,
  limit?: number,
}
export interface ListCanonicalRunsRequest {
  thread_id: string,
  status?: string,
  limit?: number,
  offset?: number,
  parent_run_id?: string,
  select?: string[],
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
}
export interface ReconnectCanonicalRunStreamRequest {
  thread_id: string,
  run_id: string,
  after_event_id?: string,
  cancel_on_disconnect?: string,
  "Last-Event-ID"?: string,
  stream_mode?: string[],
}
export interface JoinCanonicalRunRequest {
  thread_id: string,
  run_id: string,
  cancel_on_disconnect?: string,
}
export interface CancelCanonicalRunRequest {
  thread_id: string,
  run_id: string,
  action?: string,
  wait?: string,
}
export interface ResumeCanonicalRunRequest {
  thread_id: string,
  run_id: string,
  interrupt_id?: string,
  response?: any,
}
export interface ListCanonicalRunEventsRequest {
  thread_id: string,
  run_id: string,
  after_event_id?: string,
  event_types?: string[],
  limit?: number,
}
export interface ListCanonicalRunMessagesRequest {
  thread_id: string,
  run_id: string,
  before_seq?: string,
  after_seq?: string,
  limit?: number,
}
export const CreateCanonicalThread = /*#__PURE__*/createAPI<CreateCanonicalThreadRequest, CanonicalThread>({
  "url": "/api/workbench/threads",
  "method": "POST",
  "name": "CreateCanonicalThread",
  "reqType": "CreateCanonicalThreadRequest",
  "reqMapping": {
    "body": ["thread_id", "metadata", "if_exists", "ttl", "supersteps", "coze"]
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
    "body": ["metadata", "status", "ids", "limit", "offset", "sort_by", "sort_order", "values", "select", "extract"]
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
    "query": ["include"]
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
    "header": ["Prefer"],
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
    "path": ["thread_id"]
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
    "query": ["checkpoint", "checkpoint_id", "subgraphs"]
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
    "body": ["values", "as_node", "checkpoint", "checkpoint_id"]
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
    "query": ["limit", "before", "checkpoint", "checkpoint_id"]
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
    "body": ["limit", "before", "checkpoint", "checkpoint_id"]
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
    "query": ["before_seq", "after_seq", "limit"]
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
    "query": ["status", "limit", "offset", "parent_run_id", "select"]
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
    "body": ["assistant_id", "input", "command", "metadata", "config", "context", "stream_mode", "multitask_strategy", "on_disconnect", "durability", "stream_resumable", "stream_subgraphs", "if_not_exists", "webhook", "on_completion", "after_seconds", "feedback_keys", "interrupt_before", "interrupt_after", "checkpoint", "checkpoint_id", "langsmith_tracer"],
    "header": ["Idempotency-Key"]
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
    "body": ["assistant_id", "input", "command", "metadata", "config", "context", "stream_mode", "multitask_strategy", "on_disconnect", "durability", "stream_resumable", "stream_subgraphs", "if_not_exists", "webhook", "on_completion", "after_seconds", "feedback_keys", "interrupt_before", "interrupt_after", "checkpoint", "checkpoint_id", "langsmith_tracer"],
    "header": ["Idempotency-Key"]
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
    "body": ["assistant_id", "input", "command", "metadata", "config", "context", "stream_mode", "multitask_strategy", "on_disconnect", "durability", "stream_resumable", "stream_subgraphs", "if_not_exists", "webhook", "on_completion", "after_seconds", "feedback_keys", "interrupt_before", "interrupt_after", "checkpoint", "checkpoint_id", "langsmith_tracer", "raise_error"],
    "header": ["Idempotency-Key"]
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
    "path": ["thread_id", "run_id"]
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
    "query": ["after_event_id", "cancel_on_disconnect", "stream_mode"],
    "header": ["Last-Event-ID"]
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
    "query": ["cancel_on_disconnect"]
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
    "query": ["action", "wait"]
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
    "body": ["interrupt_id", "response"]
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
    "query": ["after_event_id", "event_types", "limit"]
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
    "query": ["before_seq", "after_seq", "limit"]
  },
  "resType": "CanonicalMessagePage",
  "schemaRoot": "api://schemas/idl_workbench_thread",
  "service": "workbenchThread"
});
