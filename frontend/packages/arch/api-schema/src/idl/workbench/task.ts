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
  id: string,
  task_id: string,
  event_type: string,
  payload?: string,
  created_at: number,
}
export interface ChatTask {
  id: string,
  space_id: string,
  creator_id: string,
  conversation_id?: string,
  message_id?: string,
  skill_id?: string,
  title: string,
  status: TaskStatus,
  progress: number,
  input?: string,
  result?: string,
  error?: string,
  created_at: number,
  updated_at: number,
}
export interface TaskThread {
  thread_id: string,
  legacy_task_id: string,
  space_id: string,
  creator_id: string,
  title: string,
  status: string,
  source: string,
  progress: number,
  last_user_message: string,
  last_agent_message: string,
  created_at: number,
  updated_at: number,
}
export interface TaskThreadMessage {
  message_id: string,
  thread_id: string,
  run_id: string,
  role: string,
  content: string,
  metadata: string,
  created_at: number,
}
export interface CreateTaskRequest {
  space_id: string,
  title: string,
  conversation_id?: string,
  message_id?: string,
  skill_id?: string,
  input?: string,
}
export interface CreateTaskResponse {
  data?: ChatTask,
  code: number,
  msg: string,
}
export interface ListTasksRequest {
  space_id: string,
  status?: TaskStatus,
  page?: number,
  page_size?: number,
}
export interface ListTasksData {
  tasks: ChatTask[],
  total: number,
}
export interface ListTasksResponse {
  data?: ListTasksData,
  code: number,
  msg: string,
}
export interface GetTaskRequest {
  task_id: string
}
export interface GetTaskResponse {
  data?: ChatTask,
  code: number,
  msg: string,
}
export interface ListTaskThreadsRequest {
  space_id: string,
  status?: string,
  page?: number,
  page_size?: number,
}
export interface ListTaskThreadsData {
  threads: TaskThread[],
  total: number,
}
export interface ListTaskThreadsResponse {
  data?: ListTaskThreadsData,
  code: number,
  msg: string,
}
export interface GetTaskThreadRequest {
  thread_id: string
}
export interface GetTaskThreadResponse {
  data?: TaskThread,
  code: number,
  msg: string,
}
export interface ListTaskThreadMessagesRequest {
  thread_id: string,
  page?: number,
  page_size?: number,
}
export interface AppendTaskThreadMessageRequest {
  thread_id: string,
  run_id?: string,
  role: string,
  content: string,
  metadata?: string,
}
export interface ListTaskThreadMessagesData {
  messages: TaskThreadMessage[],
  total: number,
}
export interface ListTaskThreadMessagesResponse {
  data?: ListTaskThreadMessagesData,
  code: number,
  msg: string,
}
export interface AppendTaskThreadMessageResponse {
  data?: TaskThreadMessage,
  code: number,
  msg: string,
}
export interface TaskEventsData {
  events: TaskEvent[]
}
export interface TaskEventsResponse {
  data?: TaskEventsData,
  code: number,
  msg: string,
}
export const CreateTask = /*#__PURE__*/createAPI<CreateTaskRequest, CreateTaskResponse>({
  "url": "/api/workbench/tasks",
  "method": "POST",
  "name": "CreateTask",
  "reqType": "CreateTaskRequest",
  "reqMapping": {
    "body": ["space_id", "title", "conversation_id", "message_id", "skill_id", "input"]
  },
  "resType": "CreateTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListTasks = /*#__PURE__*/createAPI<ListTasksRequest, ListTasksResponse>({
  "url": "/api/workbench/tasks",
  "method": "GET",
  "name": "ListTasks",
  "reqType": "ListTasksRequest",
  "reqMapping": {
    "query": ["space_id", "status", "page", "page_size"]
  },
  "resType": "ListTasksResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const GetTask = /*#__PURE__*/createAPI<GetTaskRequest, GetTaskResponse>({
  "url": "/api/workbench/tasks/:task_id",
  "method": "GET",
  "name": "GetTask",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "GetTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListTaskThreads = /*#__PURE__*/createAPI<ListTaskThreadsRequest, ListTaskThreadsResponse>({
  "url": "/api/workbench/task_threads",
  "method": "GET",
  "name": "ListTaskThreads",
  "reqType": "ListTaskThreadsRequest",
  "reqMapping": {
    "query": ["space_id", "status", "page", "page_size"]
  },
  "resType": "ListTaskThreadsResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const GetTaskThread = /*#__PURE__*/createAPI<GetTaskThreadRequest, GetTaskThreadResponse>({
  "url": "/api/workbench/task_threads/:thread_id",
  "method": "GET",
  "name": "GetTaskThread",
  "reqType": "GetTaskThreadRequest",
  "reqMapping": {
    "path": ["thread_id"]
  },
  "resType": "GetTaskThreadResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListTaskThreadMessages = /*#__PURE__*/createAPI<ListTaskThreadMessagesRequest, ListTaskThreadMessagesResponse>({
  "url": "/api/workbench/task_threads/:thread_id/messages",
  "method": "GET",
  "name": "ListTaskThreadMessages",
  "reqType": "ListTaskThreadMessagesRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "query": ["page", "page_size"]
  },
  "resType": "ListTaskThreadMessagesResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const AppendTaskThreadMessage = /*#__PURE__*/createAPI<AppendTaskThreadMessageRequest, AppendTaskThreadMessageResponse>({
  "url": "/api/workbench/task_threads/:thread_id/messages",
  "method": "POST",
  "name": "AppendTaskThreadMessage",
  "reqType": "AppendTaskThreadMessageRequest",
  "reqMapping": {
    "path": ["thread_id"],
    "body": ["run_id", "role", "content", "metadata"]
  },
  "resType": "AppendTaskThreadMessageResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const CancelTask = /*#__PURE__*/createAPI<GetTaskRequest, GetTaskResponse>({
  "url": "/api/workbench/tasks/:task_id/cancel",
  "method": "POST",
  "name": "CancelTask",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "GetTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const RetryTask = /*#__PURE__*/createAPI<GetTaskRequest, GetTaskResponse>({
  "url": "/api/workbench/tasks/:task_id/retry",
  "method": "POST",
  "name": "RetryTask",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "GetTaskResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
export const ListTaskEvents = /*#__PURE__*/createAPI<GetTaskRequest, TaskEventsResponse>({
  "url": "/api/workbench/tasks/:task_id/events",
  "method": "GET",
  "name": "ListTaskEvents",
  "reqType": "GetTaskRequest",
  "reqMapping": {
    "path": ["task_id"]
  },
  "resType": "TaskEventsResponse",
  "schemaRoot": "api://schemas/idl_workbench_task",
  "service": "workbenchTask"
});
