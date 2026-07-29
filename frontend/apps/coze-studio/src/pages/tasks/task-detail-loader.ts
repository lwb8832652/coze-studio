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

import type {
  WorkbenchArtifact,
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchThread,
  WorkbenchTodo,
} from '../workbench/thread-client';

import {
  TaskThreadDetailStatus,
  type TaskThreadDetailEvent,
  type TaskThreadDetailModel,
} from './task-thread-detail-model';
import type { TaskDetailTokenUsage } from './task-detail-token-usage';
import {
  fetchTaskThreadSubagentRuns,
  getRunLifecycleByRunID,
  getSubagentLifecycleByChildRunID,
  getSubagentTimelineByChildRunID,
  type TaskDetailSubagentRun,
} from './task-detail-subagents';
import { mergeJournalTaskThreadEvents } from './task-detail-journal-events';
import {
  getTaskThread,
  listTaskThreadRuns,
  listTaskThreadArtifacts,
  listTaskThreadMessages,
  listTaskThreadRunEvents,
} from './service';
export {
  mapTaskThreadRunEventToDetailEvent,
  mergeJournalTaskThreadEvents,
} from './task-detail-journal-events';

export type {
  TaskDetailSubagentRun,
  TaskDetailSubagentStatus,
  TaskDetailSubagentTimelineItem,
} from './task-detail-subagents';
export type { TaskDetailTokenUsage } from './task-detail-token-usage';
export type { TaskTokenUsageViewMode } from './task-detail-token-usage';

type TaskThread = WorkbenchThread & { creator_id?: string };
type TaskThreadArtifact = WorkbenchArtifact;
type TaskThreadMessage = WorkbenchMessage;
type TaskThreadRun = WorkbenchRun & { config?: string };
type TaskThreadTodo = WorkbenchTodo;

const COMPLETED_TASK_PROGRESS = 100;

export interface TaskDetail {
  task?: TaskThreadDetailModel;
  events: TaskThreadDetailEvent[];
  artifacts?: TaskThreadArtifact[];
  latestTaskRunCreatedAt?: number;
  latestTaskRunID?: string;
  latestTaskRunStatus?: string;
  messages?: TaskThreadMessage[];
  suggestionModelName?: string;
  suggestionModelType?: string;
  threadId?: string;
  todos?: TaskThreadTodo[];
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageByRunID?: Record<string, TaskDetailTokenUsage>;
  subagentRuns?: TaskDetailSubagentRun[];
}

const getTaskThreadExecutionType = (thread: TaskThread) =>
  thread.source === 'ark' ? 'Ark' : 'Agent';

const mapTaskThreadStatus = (status: string) => {
  switch (status) {
    case 'running':
      return TaskThreadDetailStatus.Running;
    case 'completed':
    case 'succeeded':
      return TaskThreadDetailStatus.Succeeded;
    case 'failed':
      return TaskThreadDetailStatus.Failed;
    case 'canceling':
      return TaskThreadDetailStatus.Canceling;
    case 'canceled':
      return TaskThreadDetailStatus.Canceled;
    case 'queued':
      return TaskThreadDetailStatus.Queued;
    case 'created':
    case 'idle':
    default:
      return TaskThreadDetailStatus.Created;
  }
};

const mapTaskThreadRunStatus = (run?: TaskThreadRun) => {
  switch (
    String(run?.status ?? '')
      .trim()
      .toLowerCase()
  ) {
    case 'pending':
    case 'queued':
      return TaskThreadDetailStatus.Queued;
    case 'running':
      return TaskThreadDetailStatus.Running;
    case 'success':
    case 'succeeded':
      return TaskThreadDetailStatus.Succeeded;
    case 'error':
    case 'failed':
      return TaskThreadDetailStatus.Failed;
    case 'canceled':
      return TaskThreadDetailStatus.Canceled;
    case 'interrupted':
      return ['canceled', 'cancelled'].includes(
        String(run?.terminal_reason ?? '')
          .trim()
          .toLowerCase(),
      )
        ? TaskThreadDetailStatus.Canceled
        : undefined;
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

const mapTaskThreadToDetailModel = (
  thread: TaskThread,
  messages: TaskThreadMessage[] = [],
  latestRun?: TaskThreadRun,
): TaskThreadDetailModel => {
  const executionType = getTaskThreadExecutionType(thread);
  const userMessage =
    getLatestThreadMessageContent(messages, 'user') ||
    thread.last_user_message ||
    thread.title;
  const assistantMessage =
    getLatestThreadMessageContent(messages, 'assistant') ||
    thread.last_agent_message;
  const latestRunStatus = mapTaskThreadRunStatus(latestRun);
  const status = latestRunStatus ?? mapTaskThreadStatus(thread.status);
  const progress =
    status === TaskThreadDetailStatus.Succeeded
      ? COMPLETED_TASK_PROGRESS
      : Math.max(thread.progress, 0);

  return {
    id: thread.thread_id,
    space_id: thread.space_id,
    ...(thread.creator_id ? { creator_id: thread.creator_id } : {}),
    ...(typeof thread.can_edit === 'boolean'
      ? { can_edit: thread.can_edit }
      : {}),
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

const parseJSONObject = (
  value?: string,
): Record<string, unknown> | undefined => {
  const trimmed = value?.trim();

  if (!trimmed) {
    return undefined;
  }

  try {
    const parsed: unknown = JSON.parse(trimmed);

    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch (error) {
    void error;
    return undefined;
  }

  return undefined;
};

const getLatestRunSuggestionModel = (run?: TaskThreadRun) => {
  const config = parseJSONObject(run?.config);
  const modelType = config?.model_type;
  const modelName = config?.model_name;

  return {
    suggestionModelName:
      typeof modelName === 'string' ? modelName.trim() : undefined,
    suggestionModelType:
      typeof modelType === 'string' || typeof modelType === 'number'
        ? String(modelType).trim()
        : undefined,
  };
};

// eslint-disable-next-line complexity -- Keeps one coherent detail snapshot across parallel page reads.
const fetchTaskThreadDetail = async (
  id: string,
  { spaceId }: { spaceId: string },
): Promise<TaskDetail | undefined> => {
  const threadResponse = await getTaskThread({
    thread_id: id,
    space_id: spaceId,
  });
  const thread = threadResponse.data;

  if (!thread) {
    return undefined;
  }

  const threadID = thread.thread_id;
  const [messagesResponse, topLevelRunsResponse, artifactsResponse] =
    await Promise.all([
    listTaskThreadMessages({
      thread_id: threadID,
      space_id: spaceId,
      page: 1,
      page_size: 50,
    }),
    listTaskThreadRuns({
      thread_id: threadID,
      space_id: spaceId,
      parent_run_id: '0',
      page: 1,
      page_size: 1,
    }),
    listTaskThreadArtifacts({
      thread_id: threadID,
      space_id: spaceId,
      page: 1,
      page_size: 50,
    }),
  ]);
  const latestTopLevelRun: TaskThreadRun | undefined =
    topLevelRunsResponse.data?.runs?.[0];
  const runEventsResponse = latestTopLevelRun
    ? await listTaskThreadRunEvents({
        thread_id: threadID,
        run_id: latestTopLevelRun.run_id,
        space_id: spaceId,
        page: 1,
        page_size: 100,
      })
    : undefined;
  const rawRunEvents = runEventsResponse?.data?.events ?? [];
  const suggestionModel = getLatestRunSuggestionModel(latestTopLevelRun);
  const subagentRuns = await fetchTaskThreadSubagentRuns({
    eventSourceRunId: latestTopLevelRun?.run_id,
    threadId: threadID,
    lifecycleByChildRunID: getSubagentLifecycleByChildRunID(rawRunEvents),
    runLifecycleByRunID: getRunLifecycleByRunID(rawRunEvents),
    timelineByChildRunID: getSubagentTimelineByChildRunID(rawRunEvents),
    spaceId,
  });

  return {
    threadId: thread.thread_id,
    messages: messagesResponse.data?.messages ?? [],
    todos: thread.values?.todos ?? [],
    task: mapTaskThreadToDetailModel(
      thread,
      messagesResponse.data?.messages ?? [],
      latestTopLevelRun,
    ),
    artifacts: artifactsResponse.data?.artifacts ?? [],
    events: mergeJournalTaskThreadEvents({ runEvents: rawRunEvents }),
    latestTaskRunCreatedAt: latestTopLevelRun?.created_at,
    latestTaskRunID: latestTopLevelRun?.run_id ?? '',
    latestTaskRunStatus: latestTopLevelRun?.status ?? '',
    suggestionModelName: suggestionModel.suggestionModelName,
    suggestionModelType: suggestionModel.suggestionModelType,
    subagentRuns,
  };
};

export const fetchTaskDetail = async ({
  id,
  spaceId,
}: {
  id: string;
  spaceId: string;
}): Promise<TaskDetail> => {
  const threadDetail = await fetchTaskThreadDetail(id, { spaceId });

  if (threadDetail) {
    return threadDetail;
  }

  return {
    threadId: id,
    task: undefined,
    events: [],
  };
};
