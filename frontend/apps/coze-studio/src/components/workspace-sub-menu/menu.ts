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

export const ASSISTANT_LABEL = '专属助理';
export const ASSISTANT_BADGE = 'Beta';

export const SPACE_SUB_MODULE = {
  WORKBENCH: 'chats/new',
  LIBRARY: 'library',
  SKILL: 'skill',
  DEVELOP: 'develop',
  TASK_TRIGGER: 'task-trigger',
  TASKS: 'chats',
} as const;

export const WORKSPACE_MENU_META = [
  {
    label: '新建任务',
    path: SPACE_SUB_MODULE.WORKBENCH,
    dataTestId: 'navigation_workspace_new_task',
    variant: 'primary' as const,
  },
  {
    label: '资源配置',
    path: SPACE_SUB_MODULE.LIBRARY,
    dataTestId: 'navigation_workspace_library',
  },
  {
    label: '技能配置',
    path: SPACE_SUB_MODULE.SKILL,
    dataTestId: 'navigation_workspace_skill',
  },
  {
    label: '开发配置',
    path: SPACE_SUB_MODULE.DEVELOP,
    dataTestId: 'navigation_workspace_develop',
  },
  {
    label: '任务触发器',
    path: SPACE_SUB_MODULE.TASK_TRIGGER,
    dataTestId: 'navigation_workspace_task_trigger',
  },
  {
    label: '全部任务',
    path: SPACE_SUB_MODULE.TASKS,
    dataTestId: 'navigation_workspace_tasks',
  },
];
