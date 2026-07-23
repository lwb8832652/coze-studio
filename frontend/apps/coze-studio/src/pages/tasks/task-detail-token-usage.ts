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

type TaskThreadTokenUsageAggregate =
  workbenchTask.TaskThreadTokenUsageAggregate;
type TaskThreadTokenUsage = workbenchTask.TaskThreadTokenUsage;
type TaskThreadRunEvent = workbenchTask.TaskThreadRunEvent;

export type TaskTokenUsageViewMode = 'off' | 'summary' | 'per_turn' | 'debug';

export interface TaskDetailTokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  costMicros: number;
  currency: string;
  callCount: number;
  leadAgentTokens: number;
  subagentTokens: number;
  middlewareTokens: number;
  toolTokens: number;
  modelAttributions: string[];
}

const getSingleCurrency = (currencies?: string[]): string => {
  const normalized = Array.from(
    new Set(
      (currencies ?? [])
        .map(currency => currency.trim().toUpperCase())
        .filter(Boolean),
    ),
  );

  return normalized.length === 1 ? normalized[0] : '';
};

const getSingleTokenUsageCurrency = (rows?: TaskThreadTokenUsage[]): string =>
  getSingleCurrency(rows?.map(row => row.currency));

const emptyTaskDetailTokenUsage = (): TaskDetailTokenUsage => ({
  inputTokens: 0,
  outputTokens: 0,
  totalTokens: 0,
  costMicros: 0,
  currency: '',
  callCount: 0,
  leadAgentTokens: 0,
  subagentTokens: 0,
  middlewareTokens: 0,
  toolTokens: 0,
  modelAttributions: [],
});

const getSafeTokenUsageAttributionPart = (value?: string): string =>
  (value ?? '').trim().slice(0, 80);

const toSafeNumber = (value: unknown): number =>
  typeof value === 'number' && Number.isFinite(value) ? Math.max(0, value) : 0;

const toSafeString = (value: unknown, maxLength = 80): string =>
  typeof value === 'string'
    ? value.trim().slice(0, maxLength)
    : typeof value === 'number' && Number.isFinite(value)
      ? String(value).slice(0, maxLength)
      : '';

const parseTaskTokenUsageSnapshotPayload = (
  payload?: string,
): Record<string, unknown> | undefined => {
  const trimmed = payload?.trim();
  if (!trimmed) {
    return undefined;
  }

  try {
    const parsed: unknown = JSON.parse(trimmed);

    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : undefined;
  } catch {
    return undefined;
  }
};

const taskTokenUsageSourceBuckets: Record<
  string,
  keyof Pick<
    TaskDetailTokenUsage,
    'leadAgentTokens' | 'subagentTokens' | 'middlewareTokens' | 'toolTokens'
  >
> = {
  lead_agent: 'leadAgentTokens',
  subagent: 'subagentTokens',
  middleware: 'middlewareTokens',
  tool: 'toolTokens',
};

export interface TaskTokenUsageSnapshot {
  usageID: string;
  runID: string;
  source: string;
  stepID: string;
  stepName: string;
  modelName: string;
  provider: string;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  costMicros: number;
  currency: string;
  estimated: boolean;
  createdAt: number;
}

export const parseTaskTokenUsageSnapshotEvent = (
  event: TaskThreadRunEvent,
): TaskTokenUsageSnapshot | undefined => {
  if (event.event_type !== 'token_usage.snapshot') {
    return undefined;
  }

  const payload = parseTaskTokenUsageSnapshotPayload(event.payload);
  if (!payload) {
    return undefined;
  }

  const totalTokens = toSafeNumber(payload.total_tokens);
  if (totalTokens <= 0) {
    return undefined;
  }

  const runID = toSafeString(payload.run_id) || String(event.run_id ?? '');
  if (!runID) {
    return undefined;
  }

  return {
    usageID:
      toSafeString(payload.usage_id) ||
      `${runID}:${toSafeString(event.event_id)}`,
    runID,
    source: toSafeString(payload.source) || 'lead_agent',
    stepID: toSafeString(payload.step_id),
    stepName: toSafeString(payload.step_name),
    modelName: toSafeString(payload.model_name),
    provider: toSafeString(payload.provider),
    inputTokens: toSafeNumber(payload.input_tokens),
    outputTokens: toSafeNumber(payload.output_tokens),
    totalTokens,
    costMicros: toSafeNumber(payload.cost_micros),
    currency: toSafeString(payload.currency, 16).toUpperCase(),
    estimated: payload.estimated === true,
    createdAt: toSafeNumber(payload.created_at) || event.created_at,
  };
};

const getSnapshotModelAttribution = (snapshot: TaskTokenUsageSnapshot) => {
  const provider = getSafeTokenUsageAttributionPart(snapshot.provider);
  const modelName = getSafeTokenUsageAttributionPart(snapshot.modelName);

  if (provider && modelName) {
    return `${provider} / ${modelName}`;
  }

  return provider || modelName;
};

const mergeTokenUsageCurrency = (
  currentCurrency: string,
  currentTotalTokens: number,
  snapshot: TaskTokenUsageSnapshot,
) => {
  if (!snapshot.currency) {
    return currentCurrency;
  }
  if (currentTotalTokens <= 0) {
    return snapshot.currency;
  }

  return currentCurrency === snapshot.currency ? currentCurrency : '';
};

export const mergeTaskTokenUsageSnapshot = (
  current: TaskDetailTokenUsage | undefined,
  snapshot: TaskTokenUsageSnapshot,
): TaskDetailTokenUsage => {
  const merged = current
    ? {
        ...current,
        modelAttributions: [...current.modelAttributions],
      }
    : emptyTaskDetailTokenUsage();
  const bucket = taskTokenUsageSourceBuckets[snapshot.source];
  const attribution = getSnapshotModelAttribution(snapshot);
  const currentTotalTokens = merged.totalTokens;

  merged.inputTokens += snapshot.inputTokens;
  merged.outputTokens += snapshot.outputTokens;
  merged.totalTokens += snapshot.totalTokens;
  merged.costMicros += snapshot.costMicros;
  merged.callCount += 1;
  merged.currency = mergeTokenUsageCurrency(
    merged.currency,
    currentTotalTokens,
    snapshot,
  );
  if (bucket) {
    merged[bucket] += snapshot.totalTokens;
  }
  if (attribution && !merged.modelAttributions.includes(attribution)) {
    merged.modelAttributions.push(attribution);
  }

  return merged;
};

export const mergeTaskTokenUsageSnapshotByRunID = (
  current: Record<string, TaskDetailTokenUsage>,
  snapshot: TaskTokenUsageSnapshot,
): Record<string, TaskDetailTokenUsage> => ({
  ...current,
  [snapshot.runID]: mergeTaskTokenUsageSnapshot(
    current[snapshot.runID],
    snapshot,
  ),
});

export const mapTaskThreadTokenUsageRowToSnapshot = (
  row: TaskThreadTokenUsage,
): TaskTokenUsageSnapshot | undefined => {
  const runID = toSafeString(row.run_id);
  const totalTokens = toSafeNumber(row.total_tokens);
  if (!runID || totalTokens <= 0) {
    return undefined;
  }

  return {
    usageID: toSafeString(row.usage_id),
    runID,
    source: toSafeString(row.source) || 'lead_agent',
    stepID: toSafeString(row.step_id),
    stepName: toSafeString(row.step_name),
    modelName: toSafeString(row.model_name),
    provider: toSafeString(row.provider),
    inputTokens: toSafeNumber(row.input_tokens),
    outputTokens: toSafeNumber(row.output_tokens),
    totalTokens,
    costMicros: toSafeNumber(row.cost_micros),
    currency: toSafeString(row.currency, 16).toUpperCase(),
    estimated: row.estimated === true,
    createdAt: toSafeNumber(row.created_at),
  };
};

export const mapTaskTokenUsageSnapshotsByRunID = (
  snapshots: Iterable<TaskTokenUsageSnapshot>,
): Record<string, TaskDetailTokenUsage> => {
  let usageByRunID: Record<string, TaskDetailTokenUsage> = {};

  for (const snapshot of snapshots) {
    usageByRunID = mergeTaskTokenUsageSnapshotByRunID(usageByRunID, snapshot);
  }

  return usageByRunID;
};

const getModelAttribution = (row: TaskThreadTokenUsage): string => {
  const provider = getSafeTokenUsageAttributionPart(row.provider);
  const modelName = getSafeTokenUsageAttributionPart(row.model_name);

  if (provider && modelName) {
    return `${provider} / ${modelName}`;
  }

  return provider || modelName;
};

const getUniqueModelAttributions = (
  rows?: TaskThreadTokenUsage[],
): string[] => {
  const attributions: string[] = [];

  for (const row of rows ?? []) {
    const attribution = getModelAttribution(row);

    if (attribution && !attributions.includes(attribution)) {
      attributions.push(attribution);
    }
  }

  return attributions;
};

export const mapTaskThreadTokenUsageAggregate = (
  aggregate?: TaskThreadTokenUsageAggregate,
  rows?: TaskThreadTokenUsage[],
  totalRows?: number,
): TaskDetailTokenUsage | undefined => {
  if (!aggregate || aggregate.total_tokens <= 0) {
    return undefined;
  }
  const hasCompleteCurrencyRows =
    typeof totalRows !== 'number' || (rows?.length ?? 0) >= totalRows;

  return {
    inputTokens: aggregate.input_tokens,
    outputTokens: aggregate.output_tokens,
    totalTokens: aggregate.total_tokens,
    costMicros: aggregate.cost_micros,
    currency: hasCompleteCurrencyRows ? getSingleTokenUsageCurrency(rows) : '',
    callCount: aggregate.call_count,
    leadAgentTokens: aggregate.lead_agent_tokens,
    subagentTokens: aggregate.subagent_tokens,
    middlewareTokens: aggregate.middleware_tokens,
    toolTokens: aggregate.tool_tokens,
    modelAttributions: getUniqueModelAttributions(rows),
  };
};

export const formatTaskTokenCost = (
  costMicros: number,
  currency: string,
): string => {
  const normalizedCurrency = currency.trim().toUpperCase();

  if (costMicros <= 0 || !normalizedCurrency) {
    return '';
  }

  return `${normalizedCurrency} ${(costMicros / 1_000_000).toFixed(6)}`;
};
