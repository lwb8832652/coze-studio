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
  mapTaskThreadTokenUsageAggregate,
  type TaskDetailTokenUsage,
} from './task-detail-token-usage';
import {
  fetchTaskThreadSubagentRuns,
  getSubagentLifecycleByChildRunID,
  getSubagentTimelineByChildRunID,
  type TaskDetailSubagentRun,
} from './task-detail-subagents';
import {
  getTask,
  getTaskThread,
  getTaskThreadTokenUsage,
  listTaskThreadRuns,
  listTaskThreadArtifacts,
  listTaskEvents,
  listTaskThreadMessages,
  listTaskThreadRunEvents,
} from './service';

export type {
  TaskDetailSubagentRun,
  TaskDetailSubagentStatus,
  TaskDetailSubagentTimelineItem,
} from './task-detail-subagents';
export type { TaskDetailTokenUsage } from './task-detail-token-usage';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;
type TaskThread = workbenchTask.TaskThread;
type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;
type TaskThreadRun = workbenchTask.TaskThreadRun;
type TaskThreadRunEvent = workbenchTask.TaskThreadRunEvent;

export type LoadedTaskDetailSource = 'task' | 'thread';
export type TaskDetailSource = LoadedTaskDetailSource | 'auto';

export interface TaskDetail {
  source: LoadedTaskDetailSource;
  task?: ChatTask;
  events: TaskEvent[];
  artifacts?: TaskThreadArtifact[];
  latestTaskRunID?: string;
  latestTaskRunStatus?: string;
  threadId?: string;
  tokenUsage?: TaskDetailTokenUsage;
  subagentRuns?: TaskDetailSubagentRun[];
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

const mapTaskThreadRunStatus = (status?: string) => {
  switch (
    String(status ?? '')
      .trim()
      .toLowerCase()
  ) {
    case 'pending':
    case 'queued':
      return workbenchTask.TaskStatus.Queued;
    case 'running':
      return workbenchTask.TaskStatus.Running;
    case 'succeeded':
      return workbenchTask.TaskStatus.Succeeded;
    case 'failed':
      return workbenchTask.TaskStatus.Failed;
    case 'canceled':
      return workbenchTask.TaskStatus.Canceled;
    default:
      return undefined;
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
  latestRun?: TaskThreadRun,
): ChatTask => {
  const executionType = getTaskThreadExecutionType(thread);
  const userMessage =
    getLatestThreadMessageContent(messages, 'user') ||
    thread.last_user_message ||
    thread.title;
  const assistantMessage =
    getLatestThreadMessageContent(messages, 'assistant') ||
    thread.last_agent_message;
  const latestRunStatus = mapTaskThreadRunStatus(latestRun?.status);
  const status = latestRunStatus ?? mapTaskThreadStatus(thread.status);
  const progress =
    status === workbenchTask.TaskStatus.Succeeded
      ? 100
      : Math.max(thread.progress, 0);

  return {
    id: thread.thread_id,
    space_id: thread.space_id,
    creator_id: thread.creator_id,
    title: thread.title,
    status,
    progress,
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
  run_id: event.run_id,
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
    source: 'task',
    task: taskResponse.data,
    events: eventsResponse.data?.events ?? [],
  };
};

const getTaskThreadForDetail = async (
  id: string,
  { suppressNotFoundError = false } = {},
) => {
  try {
    const threadResponse = await getTaskThread({ thread_id: id });

    return threadResponse.data;
  } catch (err) {
    if (suppressNotFoundError) {
      return undefined;
    }

    throw err;
  }
};

const fetchTaskThreadDetail = async (
  id: string,
  { suppressNotFoundError = false } = {},
): Promise<TaskDetail | undefined> => {
  const thread = await getTaskThreadForDetail(id, { suppressNotFoundError });

  if (!thread) {
    return undefined;
  }

  const threadID = thread.thread_id;
  const [
    messagesResponse,
    topLevelRunsResponse,
    runEventsResponse,
    tokenUsageResponse,
    artifactsResponse,
  ] = await Promise.all([
    listTaskThreadMessages({
      thread_id: threadID,
      page: 1,
      page_size: 50,
    }),
    listTaskThreadRuns({
      thread_id: threadID,
      parent_run_id: '0',
      page: 1,
      page_size: 1,
    }),
    listTaskThreadRunEvents({
      thread_id: threadID,
      page: 1,
      page_size: 100,
    }),
    getTaskThreadTokenUsage({
      thread_id: threadID,
      page: 1,
      page_size: 50,
    }),
    listTaskThreadArtifacts({
      thread_id: threadID,
      page: 1,
      page_size: 50,
    }),
  ]);
  const rawRunEvents = runEventsResponse.data?.events ?? [];
  const latestTopLevelRun: TaskThreadRun | undefined =
    topLevelRunsResponse.data?.runs?.[0];
  const subagentRuns = await fetchTaskThreadSubagentRuns(
    threadID,
    getSubagentLifecycleByChildRunID(rawRunEvents),
    getSubagentTimelineByChildRunID(rawRunEvents),
  );

  return {
    source: 'thread',
    threadId: thread.thread_id,
    task: mapTaskThreadToTask(
      thread,
      messagesResponse.data?.messages ?? [],
      latestTopLevelRun,
    ),
    artifacts: artifactsResponse.data?.artifacts ?? [],
    events: rawRunEvents.map(mapTaskThreadRunEventToTaskEvent),
    latestTaskRunID: latestTopLevelRun?.run_id ?? '',
    latestTaskRunStatus: latestTopLevelRun?.status ?? '',
    tokenUsage: mapTaskThreadTokenUsageAggregate(
      tokenUsageResponse.data?.aggregate,
    ),
    subagentRuns,
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

  const threadDetail = await fetchTaskThreadDetail(id, {
    suppressNotFoundError: source === 'auto',
  });

  if (threadDetail) {
    return threadDetail;
  }

  if (source === 'auto') {
    return fetchLegacyTaskDetail(id);
  }

  return {
    source: 'thread',
    threadId: id,
    task: undefined,
    events: [],
  };
};
