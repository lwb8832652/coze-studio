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

import type { LoadedTaskDetailSource } from './task-detail-loader';
import { canCancelTask, canRetryTask } from './helpers';

type ChatTask = workbenchTask.ChatTask;

export type TaskRunActionLoading = '' | 'cancel' | 'retry';

export const TaskRunActionBar = ({
  error,
  latestRunID,
  loading,
  task,
  taskDetailSource,
  onCancelTaskRun,
  onRetryTaskRun,
}: {
  error?: string;
  latestRunID: string;
  loading: TaskRunActionLoading;
  task: ChatTask;
  taskDetailSource: LoadedTaskDetailSource;
  onCancelTaskRun: (runId: string) => void | Promise<void>;
  onRetryTaskRun: (runId: string) => void | Promise<void>;
}) => {
  const showCancel =
    taskDetailSource === 'thread' && latestRunID && canCancelTask(task.status);
  const showRetry =
    taskDetailSource === 'thread' && latestRunID && canRetryTask(task.status);

  if (!showCancel && !showRetry && !error) {
    return null;
  }

  return (
    <section className="coze-prototype-task-run-actions">
      {showCancel ? (
        <button
          type="button"
          className="coze-prototype-secondary-button"
          disabled={Boolean(loading)}
          onClick={() => void onCancelTaskRun(latestRunID)}
        >
          {loading === 'cancel' ? '取消中...' : '取消任务'}
        </button>
      ) : null}
      {showRetry ? (
        <button
          type="button"
          className="coze-prototype-primary-button"
          disabled={Boolean(loading)}
          onClick={() => void onRetryTaskRun(latestRunID)}
        >
          {loading === 'retry' ? '重试中...' : '重试任务'}
        </button>
      ) : null}
      {error ? (
        <span className="coze-prototype-error" role="alert">
          {error}
        </span>
      ) : null}
    </section>
  );
};
