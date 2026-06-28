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

import { formatTaskTokenCount } from './task-token-usage-indicator';
import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';

type TaskThreadMessage = workbenchTask.TaskThreadMessage;

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

export const TaskMessageTokenUsage = ({
  tokenUsage,
  viewMode,
}: {
  tokenUsage?: TaskDetailTokenUsage;
  viewMode: TaskTokenUsageViewMode;
}) => {
  if (viewMode !== 'per_turn' || !tokenUsage || tokenUsage.totalTokens <= 0) {
    return null;
  }

  return (
    <div className="coze-prototype-message-token-usage">
      <span className="coze-prototype-message-token-label">Tokens</span>
      <span>输入: {formatTaskTokenCount(tokenUsage.inputTokens)}</span>
      <span>输出: {formatTaskTokenCount(tokenUsage.outputTokens)}</span>
      <span className="coze-prototype-message-token-total">
        总计: {formatTaskTokenCount(tokenUsage.totalTokens)}
      </span>
    </div>
  );
};
