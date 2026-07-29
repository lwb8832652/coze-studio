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

import type { workbenchTask } from '@coze-studio/api-schema';

import {
  mapTaskThreadTokenUsageAggregate,
  type TaskDetailTokenUsage,
} from './task-detail-token-usage';
import { getTaskThreadTokenUsage, listTaskThreadRuns } from './service';

type TaskThreadRun = workbenchTask.TaskThreadRun;
type TaskThreadRunEvent = workbenchTask.TaskThreadRunEvent;

export type TaskDetailSubagentStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'canceled';

export interface TaskDetailSubagentRun {
  runId: string;
  parentRunId: string;
  name: string;
  assistantId: string;
  status: TaskDetailSubagentStatus;
  statusText: string;
  errorCode: string;
  errorMessage: string;
  elapsedMs: number;
  terminalClassification: string;
  modelAttribution: string;
  retryAttempts?: TaskDetailSubagentRetryAttempt[];
  tokenUsage?: TaskDetailTokenUsage;
  timeline?: TaskDetailSubagentTimelineItem[];
  startedAt: number;
  endedAt: number;
  updatedAt: number;
}

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

export interface TaskDetailSubagentRetryAttempt {
  retryRunId: string;
  status: TaskDetailSubagentStatus;
  statusText: string;
  errorCode: string;
  errorMessage: string;
  requestedAt: number;
  updatedAt: number;
}

interface TaskThreadSubagentLifecycle {
  childRunId: string;
  errorCode: string;
  errorMessage: string;
  elapsedMs: number;
  terminalClassification: string;
  createdAt: number;
}

const parseJSONObject = (value?: string): Record<string, unknown> => {
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

const getSubagentName = (run: TaskThreadRun) => {
  const metadata = parseJSONObject(run.metadata);
  const { subagent } = metadata;

  if (subagent && typeof subagent === 'object' && !Array.isArray(subagent)) {
    const { name } = subagent as Record<string, unknown>;

    if (typeof name === 'string' && name.trim()) {
      return name.trim();
    }
  }

  return run.assistant_id || run.run_id;
};

const getPayloadID = (
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

const getPayloadString = (
  payload: Record<string, unknown>,
  key: string,
): string => {
  const value = payload[key];

  return typeof value === 'string' ? value.trim() : '';
};

const getPayloadNumber = (
  payload: Record<string, unknown>,
  key: string,
): number => {
  const value = payload[key];

  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
};

export const getSubagentLifecycleByChildRunID = (
  events: TaskThreadRunEvent[],
): Map<string, TaskThreadSubagentLifecycle> => {
  const lifecycleByRunID = new Map<string, TaskThreadSubagentLifecycle>();

  for (const event of events) {
    if (!event.event_type?.startsWith('subagent.run.')) {
      continue;
    }
    const payload = parseJSONObject(event.payload);
    const childRunId = getPayloadID(payload, 'child_run_id');

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
    case 'succeeded':
      return 'completed';
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
  events: TaskThreadRunEvent[],
): Map<string, TaskDetailSubagentTimelineItem[]> => {
  const timelineByRunID = new Map<string, TaskDetailSubagentTimelineItem[]>();

  for (const event of events) {
    if (!event.event_type?.startsWith('subagent.run.')) {
      continue;
    }

    const payload = parseJSONObject(event.payload);
    const childRunId = getPayloadID(payload, 'child_run_id');

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

const mapSubagentRunStatus = (
  run: TaskThreadRun,
): Pick<TaskDetailSubagentRun, 'status' | 'statusText'> => {
  switch (run.status) {
    case 'running':
      return { status: 'running', statusText: '运行中' };
    case 'completed':
    case 'succeeded':
      return { status: 'completed', statusText: '已完成' };
    case 'failed':
      return {
        status: 'failed',
        statusText: run.error_code === 'subagent_timeout' ? '超时' : '失败',
      };
    case 'canceling':
    case 'canceled':
      return { status: 'canceled', statusText: '已取消' };
    case 'queued':
      return { status: 'pending', statusText: '排队中' };
    case 'interrupted':
      return { status: 'pending', statusText: '等待确认' };
    case 'created':
    case 'pending':
    default:
      return { status: 'pending', statusText: '待执行' };
  }
};

const getSubagentRetrySourceRunID = (run: TaskThreadRun) => {
  const metadata = parseJSONObject(run.metadata);
  if (getPayloadString(metadata, 'source') !== 'subagent_retry') {
    return '';
  }

  return getPayloadID(metadata, 'source_run_id');
};

const groupRetryAttemptsBySourceRunID = (runs: TaskThreadRun[]) => {
  const attemptsByRunID = new Map<string, TaskDetailSubagentRetryAttempt[]>();

  for (const run of runs) {
    const sourceRunId = getSubagentRetrySourceRunID(run);
    if (!sourceRunId) {
      continue;
    }

    const status = mapSubagentRunStatus(run);
    const metadata = parseJSONObject(run.metadata);
    const attempts = attemptsByRunID.get(sourceRunId) ?? [];
    attempts.push({
      retryRunId: run.run_id,
      status: status.status,
      statusText: status.statusText,
      errorCode: run.error_code,
      errorMessage: run.error_message,
      requestedAt: getPayloadNumber(metadata, 'requested_at'),
      updatedAt: run.updated_at,
    });
    attemptsByRunID.set(sourceRunId, attempts);
  }

  for (const attempts of attemptsByRunID.values()) {
    attempts.sort((left, right) => {
      const leftTime = left.requestedAt || left.updatedAt;
      const rightTime = right.requestedAt || right.updatedAt;

      if (leftTime !== rightTime) {
        return rightTime - leftTime;
      }

      return right.retryRunId.localeCompare(left.retryRunId);
    });
  }

  return attemptsByRunID;
};

const mapTaskThreadSubagentRun = (
  run: TaskThreadRun,
  options: {
    lifecycle?: TaskThreadSubagentLifecycle;
    modelAttribution?: string;
    timeline?: TaskDetailSubagentTimelineItem[];
  } = {},
): TaskDetailSubagentRun => {
  const status = mapSubagentRunStatus(run);
  const timeline = options.timeline ?? [];

  return {
    runId: run.run_id,
    parentRunId: run.parent_run_id,
    name: getSubagentName(run),
    assistantId: run.assistant_id,
    status: status.status,
    statusText: status.statusText,
    errorCode: run.error_code || options.lifecycle?.errorCode || '',
    errorMessage: run.error_message || options.lifecycle?.errorMessage || '',
    elapsedMs: options.lifecycle?.elapsedMs ?? 0,
    terminalClassification: options.lifecycle?.terminalClassification ?? '',
    modelAttribution: options.modelAttribution ?? '',
    timeline: timeline.length ? timeline : undefined,
    startedAt: run.started_at,
    endedAt: run.ended_at,
    updatedAt: run.updated_at,
  };
};

const getModelAttribution = (
  usage: workbenchTask.TaskThreadTokenUsage,
): string => {
  const provider = usage.provider.trim();
  const model = usage.model_name.trim();

  if (provider && model) {
    return `${provider} / ${model}`;
  }
  if (model) {
    return model;
  }

  return provider;
};

const formatModelAttributionSummary = (attributions: string[] = []) => {
  if (!attributions.length) {
    return '';
  }

  const [first, ...rest] = attributions;

  return rest.length ? `${first} +${rest.length}` : first;
};

const getSingleCurrency = (currencies: string[] = []) => {
  if (currencies.length !== 1) {
    return '';
  }

  return currencies[0];
};

export const fetchTaskThreadSubagentRuns = async ({
  lifecycleByChildRunID,
  spaceId,
  threadId,
  timelineByChildRunID,
}: {
  threadId: string;
  lifecycleByChildRunID: Map<string, TaskThreadSubagentLifecycle>;
  timelineByChildRunID: Map<string, TaskDetailSubagentTimelineItem[]>;
  spaceId?: string;
}): Promise<TaskDetailSubagentRun[]> => {
  const topLevelRunsResponse = await listTaskThreadRuns({
    thread_id: threadId,
    space_id: spaceId,
    page: 1,
    page_size: 20,
  });
  const topLevelRuns = (topLevelRunsResponse.data?.runs ?? []).filter(
    run => run.run_kind !== 'subagent',
  );
  const retryAttemptsByRunID = groupRetryAttemptsBySourceRunID(topLevelRuns);
  const parentRuns = topLevelRuns.filter(
    run => !getSubagentRetrySourceRunID(run),
  );

  if (!parentRuns.length) {
    return [];
  }

  const childRunResponses = await Promise.all(
    parentRuns.map(run =>
      listTaskThreadRuns({
        thread_id: threadId,
        space_id: spaceId,
        parent_run_id: run.run_id,
        page: 1,
        page_size: 20,
      }),
    ),
  );

  const childRuns = childRunResponses
    .flatMap(response => response.data?.runs ?? [])
    .filter(run => run.run_kind === 'subagent');

  if (!childRuns.length) {
    return [];
  }

  const parentUsageResponses = await Promise.all(
    parentRuns.map(run =>
      getTaskThreadTokenUsage({
        thread_id: threadId,
        space_id: spaceId,
        run_id: run.run_id,
        include_child_runs: true,
        page: 1,
        page_size: 100,
      }),
    ),
  );
  const tokenUsageByRunID = new Map<string, TaskDetailTokenUsage>();
  const modelAttributionsByRunID = new Map<string, string[]>();
  const currenciesByRunID = new Map<string, string[]>();
  for (const response of parentUsageResponses) {
    for (const runAggregate of response.data?.run_aggregates ?? []) {
      const tokenUsage = mapTaskThreadTokenUsageAggregate(
        runAggregate.aggregate,
      );

      if (tokenUsage) {
        tokenUsageByRunID.set(runAggregate.run_id, tokenUsage);
      }
    }
    for (const usage of response.data?.usage ?? []) {
      const attribution = getModelAttribution(usage);

      if (!attribution) {
        continue;
      }

      const existing = modelAttributionsByRunID.get(usage.run_id) ?? [];
      if (!existing.includes(attribution)) {
        modelAttributionsByRunID.set(usage.run_id, [...existing, attribution]);
      }

      const currency = usage.currency.trim().toUpperCase();
      if (!currency) {
        continue;
      }

      const currencies = currenciesByRunID.get(usage.run_id) ?? [];
      if (!currencies.includes(currency)) {
        currenciesByRunID.set(usage.run_id, [...currencies, currency]);
      }
    }
  }

  return childRuns.map(run => {
    const tokenUsage = tokenUsageByRunID.get(run.run_id);
    const currency = getSingleCurrency(currenciesByRunID.get(run.run_id));

    return {
      ...mapTaskThreadSubagentRun(run, {
        lifecycle: lifecycleByChildRunID.get(run.run_id),
        modelAttribution: formatModelAttributionSummary(
          modelAttributionsByRunID.get(run.run_id),
        ),
        timeline: timelineByChildRunID.get(run.run_id),
      }),
      retryAttempts: retryAttemptsByRunID.get(run.run_id),
      tokenUsage: tokenUsage ? { ...tokenUsage, currency } : undefined,
    } satisfies TaskDetailSubagentRun;
  });
};
