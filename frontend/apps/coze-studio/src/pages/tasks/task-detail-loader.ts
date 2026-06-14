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

import { workbenchTask } from '@coze-studio/api-schema';

import {
  getTask,
  getTaskThread,
  listTaskEvents,
  listTaskThreadRunEvents,
  listTaskThreadMessages,
} from './service';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;
type TaskThread = workbenchTask.TaskThread;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;
type TaskThreadRunEvent = workbenchTask.TaskThreadRunEvent;

export type TaskDetailSource = 'task' | 'thread';

interface TaskDetail {
  task?: ChatTask;
  events: TaskEvent[];
}

const getTaskThreadExecutionType = (thread: TaskThread) =>
  thread.source === 'ark' ? 'Ark' : 'Agent';

const mapTaskThreadStatus = (status: string) => {
  switch (status) {
    case 'running':
      return workbenchTask.TaskStatus.Running;
    case 'completed':
    case 'succeeded':
      return workbenchTask.TaskStatus.Succeeded;
    case 'failed':
      return workbenchTask.TaskStatus.Failed;
    case 'canceling':
      return workbenchTask.TaskStatus.Canceling;
    case 'canceled':
      return workbenchTask.TaskStatus.Canceled;
    case 'queued':
      return workbenchTask.TaskStatus.Queued;
    case 'created':
    case 'idle':
    default:
      return workbenchTask.TaskStatus.Created;
  }
};

const getLatestThreadMessageContent = (
  messages: TaskThreadMessage[],
  role: string,
) => {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];

    if (message?.role !== role) {
      continue;
    }

    const content = message.content.trim();

    if (content) {
      return content;
    }
  }

  return '';
};

const mapTaskThreadToTask = (
  thread: TaskThread,
  messages: TaskThreadMessage[] = [],
): ChatTask => {
  const executionType = getTaskThreadExecutionType(thread);
  const userMessage =
    getLatestThreadMessageContent(messages, 'user') ||
    thread.last_user_message ||
    thread.title;
  const assistantMessage =
    getLatestThreadMessageContent(messages, 'assistant') ||
    thread.last_agent_message;

  return {
    id: thread.legacy_task_id || thread.thread_id,
    space_id: thread.space_id,
    creator_id: thread.creator_id,
    title: thread.title,
    status: mapTaskThreadStatus(thread.status),
    progress: thread.progress,
    input: JSON.stringify({
      message: userMessage,
      execution_type: executionType,
    }),
    result: JSON.stringify({
      message: assistantMessage,
      result_type: 'answer',
      execution_type: executionType,
    }),
    created_at: thread.created_at,
    updated_at: thread.updated_at,
  };
};

export const mapTaskThreadRunEventToTaskEvent = (
  event: TaskThreadRunEvent,
): TaskEvent => ({
  id: event.event_id,
  task_id: event.thread_id,
  event_type: event.event_type,
  payload: event.payload,
  created_at: event.created_at,
});

const fetchLegacyTaskDetail = async (taskId: string): Promise<TaskDetail> => {
  const [taskResponse, eventsResponse] = await Promise.all([
    getTask({ task_id: taskId }),
    listTaskEvents({ task_id: taskId }),
  ]);

  return {
    task: taskResponse.data,
    events: eventsResponse.data?.events ?? [],
  };
};

export const fetchTaskDetail = async ({
  id,
  source,
}: {
  id: string;
  source: TaskDetailSource;
}): Promise<TaskDetail> => {
  if (source === 'task') {
    return fetchLegacyTaskDetail(id);
  }

  const threadResponse = await getTaskThread({ thread_id: id });
  const thread = threadResponse.data;

  if (!thread) {
    return {
      task: undefined,
      events: [],
    };
  }

  if (thread.legacy_task_id) {
    return fetchLegacyTaskDetail(thread.legacy_task_id);
  }

  const [messagesResponse, runEventsResponse] = await Promise.all([
    listTaskThreadMessages({
      thread_id: id,
      page: 1,
      page_size: 50,
    }),
    listTaskThreadRunEvents({
      thread_id: id,
      page: 1,
      page_size: 100,
    }),
  ]);

  return {
    task: mapTaskThreadToTask(thread, messagesResponse.data?.messages ?? []),
    events: (runEventsResponse.data?.events ?? []).map(
      mapTaskThreadRunEventToTaskEvent,
    ),
  };
};
