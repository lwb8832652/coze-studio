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

export const getWorkspaceTaskStatusMeta = (
  status: workbenchTask.TaskStatus,
): WorkspaceTaskStatusMeta => {
  if (
    status === workbenchTask.TaskStatus.Created ||
    status === workbenchTask.TaskStatus.Queued
  ) {
    return {
      tone: 'waiting',
      color: '#f5a623',
      ariaLabel: '等待状态',
    };
  }

  if (
    status === workbenchTask.TaskStatus.Running ||
    status === workbenchTask.TaskStatus.Canceling
  ) {
    return {
      tone: 'running',
      color: '#2a6df4',
      ariaLabel: '运行中状态',
    };
  }

  if (status === workbenchTask.TaskStatus.Succeeded) {
    return {
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    };
  }

  if (status === workbenchTask.TaskStatus.Failed) {
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
