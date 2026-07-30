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

import type { WorkbenchRunEvent } from '../workbench/thread-client';
import { listTaskThreadRunEvents } from './service';

export type TaskDetailSubagentStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'canceled';

export interface TaskDetailSubagentTimelineItem {
  id: string;
  eventType: string;
  title: string;
  status: TaskDetailSubagentStatus;
  elapsedMs: number;
  terminalClassification: string;
  errorMessage: string;
  createdAt: number;
}

export interface TaskThreadSubagentLifecycle {
  childRunId: string;
  errorCode: string;
  errorMessage: string;
  elapsedMs: number;
  terminalClassification: string;
  createdAt: number;
}

const SUBAGENT_HISTORY_EVENT_TYPES = [
  'run.completed',
  'run.failed',
  'run.canceled',
  'run.cancelled',
  'run.interrupted',
  'subagent.run.started',
  'subagent.run.completed',
  'subagent.run.failed',
  'subagent.run.canceled',
];

export const parseJSONObject = (value?: string): Record<string, unknown> => {
  if (!value?.trim()) {
    return {};
  }

  try {
    const parsed: unknown = JSON.parse(value);

    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch (error) {
    if (error instanceof SyntaxError) {
      return {};
    }

    throw error;
  }

  return {};
};

export const getPayloadID = (
  payload: Record<string, unknown>,
  key: string,
): string => {
  const value = payload[key];

  if (typeof value === 'string') {
    return value.trim();
  }
  if (typeof value === 'number' && Number.isFinite(value)) {
    return String(value);
  }

  return '';
};

export const getPayloadString = (
  payload: Record<string, unknown>,
  key: string,
): string => {
  const value = payload[key];

  return typeof value === 'string' ? value.trim() : '';
};

export const getPayloadNumber = (
  payload: Record<string, unknown>,
  key: string,
): number => {
  const value = payload[key];

  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
};

const getSubagentRunID = (payload: Record<string, unknown>) =>
  getPayloadID(payload, 'child_run_id') ||
  getPayloadID(payload, 'subagent_run_id');

export const getSubagentLifecycleByChildRunID = (
  events: WorkbenchRunEvent[],
): Map<string, TaskThreadSubagentLifecycle> => {
  const lifecycleByRunID = new Map<string, TaskThreadSubagentLifecycle>();

  for (const event of events) {
    if (!event.event_type?.startsWith('subagent.run.')) {
      continue;
    }
    const payload = parseJSONObject(event.payload);
    const childRunId = getSubagentRunID(payload);
    if (!childRunId) {
      continue;
    }

    const current = lifecycleByRunID.get(childRunId);
    if (current && current.createdAt > event.created_at) {
      continue;
    }
    lifecycleByRunID.set(childRunId, {
      childRunId,
      errorCode: getPayloadString(payload, 'error_code'),
      errorMessage: getPayloadString(payload, 'error_message'),
      elapsedMs: getPayloadNumber(payload, 'elapsed_ms'),
      terminalClassification: getPayloadString(
        payload,
        'terminal_classification',
      ),
      createdAt: event.created_at,
    });
  }

  return lifecycleByRunID;
};

export const getRunLifecycleByRunID = (
  events: WorkbenchRunEvent[],
): Map<string, TaskThreadSubagentLifecycle> => {
  const lifecycleByRunID = new Map<string, TaskThreadSubagentLifecycle>();

  for (const event of events) {
    if (!event.event_type?.startsWith('run.')) {
      continue;
    }
    const runId = String(event.run_id ?? '').trim();
    if (!runId) {
      continue;
    }
    const current = lifecycleByRunID.get(runId);
    if (current && current.createdAt > event.created_at) {
      continue;
    }
    const payload = parseJSONObject(event.payload);
    lifecycleByRunID.set(runId, {
      childRunId: runId,
      errorCode: getPayloadString(payload, 'error_code'),
      errorMessage: getPayloadString(payload, 'error_message'),
      elapsedMs: getPayloadNumber(payload, 'elapsed_ms'),
      terminalClassification:
        getPayloadString(payload, 'terminal_classification') ||
        getPayloadString(payload, 'reason'),
      createdAt: event.created_at,
    });
  }

  return lifecycleByRunID;
};

const getSubagentTimelineTitle = (eventType: string) => {
  switch (eventType) {
    case 'subagent.run.started':
      return '子智能体已启动';
    case 'subagent.run.completed':
      return '子智能体已完成';
    case 'subagent.run.failed':
      return '子智能体失败';
    case 'subagent.run.canceled':
      return '子智能体已取消';
    default:
      return eventType;
  }
};

const normalizeSubagentStatus = (
  status: string,
): TaskDetailSubagentStatus | undefined => {
  switch (status) {
    case 'running':
      return 'running';
    case 'completed':
    case 'success':
    case 'succeeded':
      return 'completed';
    case 'error':
    case 'failed':
      return 'failed';
    case 'canceling':
    case 'canceled':
      return 'canceled';
    case 'queued':
    case 'created':
    case 'pending':
    case 'interrupted':
      return 'pending';
    default:
      return undefined;
  }
};

const getSubagentTimelineStatus = (
  eventType: string,
  payload: Record<string, unknown>,
): TaskDetailSubagentStatus => {
  const payloadStatus = normalizeSubagentStatus(
    getPayloadString(payload, 'status'),
  );
  if (payloadStatus) {
    return payloadStatus;
  }

  switch (eventType) {
    case 'subagent.run.completed':
      return 'completed';
    case 'subagent.run.failed':
      return 'failed';
    case 'subagent.run.canceled':
      return 'canceled';
    case 'subagent.run.started':
      return 'running';
    default:
      return 'pending';
  }
};

export const getSubagentTimelineByChildRunID = (
  events: WorkbenchRunEvent[],
): Map<string, TaskDetailSubagentTimelineItem[]> => {
  const timelineByRunID = new Map<string, TaskDetailSubagentTimelineItem[]>();

  for (const event of events) {
    if (!event.event_type?.startsWith('subagent.run.')) {
      continue;
    }
    const payload = parseJSONObject(event.payload);
    const childRunId = getSubagentRunID(payload);
    if (!childRunId) {
      continue;
    }

    const timeline = timelineByRunID.get(childRunId) ?? [];
    timeline.push({
      id: event.event_id,
      eventType: event.event_type,
      title: getSubagentTimelineTitle(event.event_type),
      status: getSubagentTimelineStatus(event.event_type, payload),
      elapsedMs: getPayloadNumber(payload, 'elapsed_ms'),
      terminalClassification: getPayloadString(
        payload,
        'terminal_classification',
      ),
      errorMessage: getPayloadString(payload, 'error_message'),
      createdAt: event.created_at,
    });
    timelineByRunID.set(childRunId, timeline);
  }

  for (const timeline of timelineByRunID.values()) {
    timeline.sort((left, right) => {
      if (left.createdAt !== right.createdAt) {
        return left.createdAt - right.createdAt;
      }

      return left.id.localeCompare(right.id);
    });
  }

  return timelineByRunID;
};

const mergeLifecycleMaps = (
  target: Map<string, TaskThreadSubagentLifecycle>,
  incoming: Map<string, TaskThreadSubagentLifecycle>,
) => {
  for (const [runId, lifecycle] of incoming) {
    const current = target.get(runId);
    if (!current || lifecycle.createdAt >= current.createdAt) {
      target.set(runId, lifecycle);
    }
  }
};

const mergeTimelineMaps = (
  target: Map<string, TaskDetailSubagentTimelineItem[]>,
  incoming: Map<string, TaskDetailSubagentTimelineItem[]>,
) => {
  for (const [runId, incomingTimeline] of incoming) {
    const timelineByID = new Map(
      (target.get(runId) ?? []).map(item => [item.id, item]),
    );
    for (const item of incomingTimeline) {
      timelineByID.set(item.id, item);
    }
    target.set(
      runId,
      Array.from(timelineByID.values()).sort((left, right) => {
        if (left.createdAt !== right.createdAt) {
          return left.createdAt - right.createdAt;
        }

        return left.id.localeCompare(right.id);
      }),
    );
  }
};

export const loadTaskThreadSubagentEventHistory = async ({
  eventSourceRunId,
  lifecycleByChildRunID,
  parentRunIds,
  retryRunIds,
  runLifecycleByRunID,
  spaceId,
  threadId,
  timelineByChildRunID,
}: {
  eventSourceRunId?: string;
  lifecycleByChildRunID: Map<string, TaskThreadSubagentLifecycle>;
  parentRunIds: string[];
  retryRunIds: string[];
  runLifecycleByRunID: Map<string, TaskThreadSubagentLifecycle>;
  spaceId: string;
  threadId: string;
  timelineByChildRunID: Map<string, TaskDetailSubagentTimelineItem[]>;
}) => {
  const mergedLifecycleByChildRunID = new Map(lifecycleByChildRunID);
  const mergedRunLifecycleByRunID = new Map(runLifecycleByRunID);
  const mergedTimelineByChildRunID = new Map(timelineByChildRunID);
  const historyRunIDs = new Set([...parentRunIds, ...retryRunIds]);
  if (eventSourceRunId) {
    historyRunIDs.delete(eventSourceRunId);
  }

  const results = await Promise.allSettled(
    Array.from(historyRunIDs).map(runId =>
      listTaskThreadRunEvents({
        thread_id: threadId,
        space_id: spaceId,
        run_id: runId,
        event_types: SUBAGENT_HISTORY_EVENT_TYPES,
        page: 1,
        page_size: 100,
      }),
    ),
  );
  for (const result of results) {
    if (result.status !== 'fulfilled') {
      continue;
    }
    const events = result.value.data?.events ?? [];
    mergeLifecycleMaps(
      mergedLifecycleByChildRunID,
      getSubagentLifecycleByChildRunID(events),
    );
    mergeLifecycleMaps(
      mergedRunLifecycleByRunID,
      getRunLifecycleByRunID(events),
    );
    mergeTimelineMaps(
      mergedTimelineByChildRunID,
      getSubagentTimelineByChildRunID(events),
    );
  }

  return {
    lifecycleByChildRunID: mergedLifecycleByChildRunID,
    runLifecycleByRunID: mergedRunLifecycleByRunID,
    timelineByChildRunID: mergedTimelineByChildRunID,
  };
};
