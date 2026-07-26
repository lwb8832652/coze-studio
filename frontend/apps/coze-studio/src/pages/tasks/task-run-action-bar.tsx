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

import type { TaskThreadDetailModel } from './task-thread-detail-model';
import { canRetryTask } from './helpers';

export type TaskRunActionLoading = '' | 'cancel' | 'retry';

export const TaskRunActionBar = ({
  disabled = false,
  error,
  latestRunID,
  loading,
  task,
  onRetryTaskRun,
}: {
  disabled?: boolean;
  error?: string;
  latestRunID: string;
  loading: TaskRunActionLoading;
  task: TaskThreadDetailModel;
  onRetryTaskRun: (runId: string) => void | Promise<void>;
}) => {
  const showRetry = latestRunID && canRetryTask(task.status);

  if (!showRetry && !error) {
    return null;
  }

  return (
    <section className="coze-prototype-task-run-actions">
      {showRetry ? (
        <button
          type="button"
          className="coze-prototype-primary-button"
          disabled={disabled || Boolean(loading)}
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
