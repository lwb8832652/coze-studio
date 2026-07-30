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
  WorkbenchRun,
  WorkbenchTokenUsage,
} from '../workbench/thread-client';
import {
  mapTaskThreadTokenUsageAggregate,
  type TaskDetailTokenUsage,
} from './task-detail-token-usage';
import {
  getPayloadID,
  getPayloadNumber,
  getPayloadString,
  loadTaskThreadSubagentEventHistory,
  parseJSONObject,
  type TaskDetailSubagentStatus,
  type TaskDetailSubagentTimelineItem,
  type TaskThreadSubagentLifecycle,
} from './task-detail-subagent-events';
import { getTaskThreadTokenUsage, listTaskThreadRuns } from './service';

export {
  getRunLifecycleByRunID,
  getSubagentLifecycleByChildRunID,
  getSubagentTimelineByChildRunID,
} from './task-detail-subagent-events';
export type {
  TaskDetailSubagentStatus,
  TaskDetailSubagentTimelineItem,
} from './task-detail-subagent-events';

type TaskThreadRun = WorkbenchRun;

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

export interface TaskDetailSubagentRetryAttempt {
  retryRunId: string;
  status: TaskDetailSubagentStatus;
  statusText: string;
  errorCode: string;
  errorMessage: string;
  requestedAt: number;
  updatedAt: number;
}

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

const mapSubagentRunStatus = (
  run: TaskThreadRun,
  lifecycle?: TaskThreadSubagentLifecycle,
): Pick<TaskDetailSubagentRun, 'status' | 'statusText'> => {
  switch (run.status) {
    case 'running':
      return { status: 'running', statusText: '运行中' };
    case 'completed':
    case 'success':
    case 'succeeded':
      return { status: 'completed', statusText: '已完成' };
    case 'error':
    case 'failed':
      return {
        status: 'failed',
        statusText:
          lifecycle?.errorCode === 'subagent_timeout' ? '超时' : '失败',
      };
    case 'canceling':
    case 'canceled':
      return { status: 'canceled', statusText: '已取消' };
    case 'queued':
    case 'pending':
      return { status: 'pending', statusText: '排队中' };
    case 'interrupted':
      return ['canceled', 'cancelled'].includes(
        String(run.terminal_reason ?? '')
          .trim()
          .toLowerCase(),
      )
        ? { status: 'canceled', statusText: '已取消' }
        : { status: 'pending', statusText: '等待确认' };
    case 'created':
    default:
      return { status: 'pending', statusText: '待执行' };
  }
};

export const getSubagentRetrySourceRunID = (run: TaskThreadRun) => {
  const metadata = parseJSONObject(run.metadata);
  if (getPayloadString(metadata, 'source') !== 'subagent_retry') {
    return '';
  }

  return getPayloadID(metadata, 'source_run_id');
};

const groupRetryAttemptsBySourceRunID = (
  runs: TaskThreadRun[],
  lifecycleByRunID: Map<string, TaskThreadSubagentLifecycle>,
) => {
  const attemptsByRunID = new Map<string, TaskDetailSubagentRetryAttempt[]>();

  for (const run of runs) {
    const sourceRunId = getSubagentRetrySourceRunID(run);
    if (!sourceRunId) {
      continue;
    }

    const lifecycle = lifecycleByRunID.get(run.run_id);
    const status = mapSubagentRunStatus(run, lifecycle);
    const metadata = parseJSONObject(run.metadata);
    const attempts = attemptsByRunID.get(sourceRunId) ?? [];
    attempts.push({
      retryRunId: run.run_id,
      status: status.status,
      statusText: status.statusText,
      errorCode: lifecycle?.errorCode || '',
      errorMessage: lifecycle?.errorMessage || '',
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
  const status = mapSubagentRunStatus(run, options.lifecycle);
  const timeline = options.timeline ?? [];

  return {
    runId: run.run_id,
    parentRunId: run.parent_run_id ?? '',
    name: getSubagentName(run),
    assistantId: run.assistant_id,
    status: status.status,
    statusText: status.statusText,
    errorCode: options.lifecycle?.errorCode || '',
    errorMessage: options.lifecycle?.errorMessage || '',
    elapsedMs: options.lifecycle?.elapsedMs ?? 0,
    terminalClassification: options.lifecycle?.terminalClassification ?? '',
    modelAttribution: options.modelAttribution ?? '',
    timeline: timeline.length ? timeline : undefined,
    startedAt: run.started_at ?? 0,
    endedAt: run.ended_at ?? 0,
    updatedAt: run.updated_at,
  };
};

const getModelAttribution = (usage: WorkbenchTokenUsage): string => {
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
  eventSourceRunId,
  lifecycleByChildRunID,
  runLifecycleByRunID,
  spaceId,
  threadId,
  timelineByChildRunID,
}: {
  eventSourceRunId?: string;
  threadId: string;
  lifecycleByChildRunID: Map<string, TaskThreadSubagentLifecycle>;
  runLifecycleByRunID: Map<string, TaskThreadSubagentLifecycle>;
  timelineByChildRunID: Map<string, TaskDetailSubagentTimelineItem[]>;
  spaceId: string;
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
  const retryRuns = topLevelRuns.filter(run =>
    Boolean(getSubagentRetrySourceRunID(run)),
  );
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

  const parentRunIDsWithChildren = new Set(
    childRuns.map(run => run.parent_run_id).filter(Boolean),
  );
  const eventHistory = await loadTaskThreadSubagentEventHistory({
    eventSourceRunId,
    lifecycleByChildRunID,
    parentRunIds: parentRuns
      .filter(run => parentRunIDsWithChildren.has(run.run_id))
      .map(run => run.run_id),
    retryRunIds: retryRuns.map(run => run.run_id),
    runLifecycleByRunID,
    spaceId,
    threadId,
    timelineByChildRunID,
  });
  const retryAttemptsByRunID = groupRetryAttemptsBySourceRunID(
    topLevelRuns,
    eventHistory.runLifecycleByRunID,
  );

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
        lifecycle: eventHistory.lifecycleByChildRunID.get(run.run_id),
        modelAttribution: formatModelAttributionSummary(
          modelAttributionsByRunID.get(run.run_id),
        ),
        timeline: eventHistory.timelineByChildRunID.get(run.run_id),
      }),
      retryAttempts: retryAttemptsByRunID.get(run.run_id),
      tokenUsage: tokenUsage ? { ...tokenUsage, currency } : undefined,
    } satisfies TaskDetailSubagentRun;
  });
};
