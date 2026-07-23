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

import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';

type TaskThreadMessage = workbenchTask.TaskThreadMessage;

export interface TaskUsageDetailItem {
  canLocate: boolean;
  createdAt?: number;
  runID: string;
  usage: TaskDetailTokenUsage;
}

const TOKEN_USAGE_VIEW_STORAGE_KEY = 'coze.task-detail.token-usage-view-mode';
const DEFAULT_TOKEN_USAGE_VIEW_MODE: TaskTokenUsageViewMode = 'per_turn';

const isTaskTokenUsageViewMode = (
  value: string | null,
): value is TaskTokenUsageViewMode =>
  value === 'off' ||
  value === 'summary' ||
  value === 'per_turn' ||
  value === 'debug';

export const loadTaskTokenUsageViewMode = (): TaskTokenUsageViewMode => {
  if (typeof window === 'undefined') {
    return DEFAULT_TOKEN_USAGE_VIEW_MODE;
  }

  try {
    const stored = window.localStorage.getItem(TOKEN_USAGE_VIEW_STORAGE_KEY);

    return isTaskTokenUsageViewMode(stored)
      ? stored
      : DEFAULT_TOKEN_USAGE_VIEW_MODE;
  } catch (err) {
    void err;

    return DEFAULT_TOKEN_USAGE_VIEW_MODE;
  }
};

export const saveTaskTokenUsageViewMode = (mode: TaskTokenUsageViewMode) => {
  if (typeof window === 'undefined') {
    return;
  }

  try {
    window.localStorage.setItem(TOKEN_USAGE_VIEW_STORAGE_KEY, mode);
  } catch (err) {
    void err;
  }
};

export const getLatestAssistantRunID = (messages: TaskThreadMessage[]) => {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    const message = messages[index];

    if (message.role === 'assistant' && message.run_id?.trim()) {
      return message.run_id.trim();
    }
  }

  return '';
};

const normalizeTaskUsageRunID = (runID?: string) => {
  const value = String(runID ?? '').trim();

  return value && value !== '0' ? value : '';
};

export const getCurrentAssistantRunID = (
  messages: TaskThreadMessage[],
  latestTaskRunID?: string,
) => {
  const latestRunID = normalizeTaskUsageRunID(latestTaskRunID);
  let latestUserIndex = -1;

  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === 'user') {
      latestUserIndex = index;
      break;
    }
  }
  if (latestUserIndex < 0) {
    return '';
  }

  if (latestRunID) {
    for (let index = messages.length - 1; index > latestUserIndex; index -= 1) {
      const message = messages[index];
      if (
        message.role === 'assistant' &&
        normalizeTaskUsageRunID(message.run_id) === latestRunID
      ) {
        return latestRunID;
      }
    }

    return normalizeTaskUsageRunID(messages[latestUserIndex].run_id) ===
      latestRunID
      ? latestRunID
      : '';
  }

  for (let index = messages.length - 1; index > latestUserIndex; index -= 1) {
    const message = messages[index];
    if (message.role === 'assistant') {
      return normalizeTaskUsageRunID(message.run_id);
    }
  }

  return '';
};

export const buildTaskUsageDetailItems = (
  messages: TaskThreadMessage[],
  tokenUsageByRunID?: Record<string, TaskDetailTokenUsage>,
): TaskUsageDetailItem[] => {
  const detailItems: TaskUsageDetailItem[] = [];
  const includedRunIDs = new Set<string>();

  for (const message of messages) {
    if (message.role !== 'assistant') {
      continue;
    }
    const runID = normalizeTaskUsageRunID(message.run_id);
    const usage = runID ? tokenUsageByRunID?.[runID] : undefined;
    if (
      !runID ||
      !usage ||
      usage.totalTokens <= 0 ||
      includedRunIDs.has(runID)
    ) {
      continue;
    }

    includedRunIDs.add(runID);
    detailItems.push({
      canLocate: true,
      createdAt:
        Number.isFinite(message.created_at) && message.created_at > 0
          ? message.created_at
          : undefined,
      runID,
      usage,
    });
  }

  for (const [runIDValue, usage] of Object.entries(tokenUsageByRunID ?? {})) {
    const runID = normalizeTaskUsageRunID(runIDValue);
    if (!runID || usage.totalTokens <= 0 || includedRunIDs.has(runID)) {
      continue;
    }

    includedRunIDs.add(runID);
    detailItems.push({ canLocate: false, runID, usage });
  }

  return detailItems;
};

export const TaskMessageTokenUsage = ({
  tokenUsage,
  viewMode,
}: {
  tokenUsage?: TaskDetailTokenUsage;
  viewMode: TaskTokenUsageViewMode;
}) => {
  void tokenUsage;
  void viewMode;

  return null;
};
