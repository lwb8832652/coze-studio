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

export type WorkspaceTaskStatusTone =
  | 'waiting'
  | 'running'
  | 'success'
  | 'danger'
  | 'neutral';

export interface WorkspaceTaskStatusMeta {
  tone: WorkspaceTaskStatusTone;
  color: string;
  ariaLabel: string;
}

type WorkspaceTaskStatusValue = workbenchTask.TaskStatus | string;

const waitingStatuses = new Set<WorkspaceTaskStatusValue>([
  workbenchTask.TaskStatus.Created,
  workbenchTask.TaskStatus.Queued,
  'created',
  'queued',
  'interrupted',
  'idle',
]);

const runningStatuses = new Set<WorkspaceTaskStatusValue>([
  workbenchTask.TaskStatus.Running,
  workbenchTask.TaskStatus.Canceling,
  'running',
  'canceling',
]);

const successStatuses = new Set<WorkspaceTaskStatusValue>([
  workbenchTask.TaskStatus.Succeeded,
  'succeeded',
  'completed',
]);

const dangerStatuses = new Set<WorkspaceTaskStatusValue>([
  workbenchTask.TaskStatus.Failed,
  'failed',
]);

export const getWorkspaceTaskStatusMeta = (
  status: WorkspaceTaskStatusValue,
): WorkspaceTaskStatusMeta => {
  if (waitingStatuses.has(status)) {
    return {
      tone: 'waiting',
      color: '#f5a623',
      ariaLabel: '等待状态',
    };
  }

  if (runningStatuses.has(status)) {
    return {
      tone: 'running',
      color: '#2a6df4',
      ariaLabel: '运行中状态',
    };
  }

  if (successStatuses.has(status)) {
    return {
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    };
  }

  if (dangerStatuses.has(status)) {
    return {
      tone: 'danger',
      color: '#f54a45',
      ariaLabel: '异常状态',
    };
  }

  return {
    tone: 'neutral',
    color: '#a7adb8',
    ariaLabel: '已取消状态',
  };
};
