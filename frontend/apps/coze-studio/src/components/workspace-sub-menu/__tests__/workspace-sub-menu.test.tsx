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

import { getWorkspaceTaskStatusMeta } from '../workspace-task-status';
import { ASSISTANT_BADGE, ASSISTANT_LABEL, WORKSPACE_MENU_META } from '../menu';

describe('Coze Studio WorkspaceSubMenu', () => {
  it('defines the Figma workspace navigation structure', () => {
    const labels = WORKSPACE_MENU_META.map(item => item.label);

    expect(ASSISTANT_LABEL).toBe('专属助理');
    expect(ASSISTANT_BADGE).toBe('Beta');
    expect(labels).toEqual([
      '新建任务',
      '资源配置',
      '技能配置',
      '开发配置',
      '任务触发器',
      '全部任务',
    ]);
    expect(WORKSPACE_MENU_META[0]).toMatchObject({
      label: '新建任务',
      path: 'chats/new',
      variant: 'primary',
    });
    expect(WORKSPACE_MENU_META.at(-1)).toMatchObject({
      label: '全部任务',
      path: 'chats',
    });
  });

  it('uses distinct sidebar status indicators and keeps green for completed tasks only', () => {
    const completedMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Succeeded,
    );
    const runningMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Running,
    );
    const failedMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Failed,
    );

    expect(completedMeta).toMatchObject({
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    });
    expect(runningMeta).toMatchObject({
      tone: 'running',
      color: '#2a6df4',
      ariaLabel: '运行中状态',
    });
    expect(failedMeta).toMatchObject({
      tone: 'danger',
      color: '#f54a45',
      ariaLabel: '异常状态',
    });
    expect(runningMeta.color).not.toBe(completedMeta.color);
  });
});
