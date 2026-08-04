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

export const SYSTEM_MANAGEMENT_ENTRY = {
  label: '系统管理',
  path: '/system/overview',
} as const;

export const PERSONAL_CENTER_ENTRY = {
  label: '个人中心',
  path: '/profile',
} as const;

export const ACCOUNT_SETTINGS_ENTRY = {
  label: '账号设置',
  path: '/profile',
} as const;

export const SIGN_OUT_ENTRY = {
  label: '退出登录',
  action: 'logout',
} as const;

export const ACCOUNT_ACTION_ENTRIES = [
  ACCOUNT_SETTINGS_ENTRY,
  SIGN_OUT_ENTRY,
] as const;

export const SPACE_SUB_MODULE = {
  WORKBENCH: 'chats/new',
  LIBRARY: 'library',
  APP_DEV: 'app-dev',
  SKILL: 'skill',
  DEVELOP: 'develop',
  TOOLS: 'tools',
  WORKSPACE: 'workspace',
  TASK_CENTER: 'task-center',
  TASKS: 'chats',
} as const;

export interface WorkspaceMenuPolicySpace {
  allow_develop?: boolean | number;
  role_type?: number;
  space_role_type?: number;
  space_type?: number;
  type?: number;
}

const DEVELOPER_FEATURE_MENU_PATHS = new Set<string>([
  SPACE_SUB_MODULE.LIBRARY,
  SPACE_SUB_MODULE.APP_DEV,
  SPACE_SUB_MODULE.SKILL,
  SPACE_SUB_MODULE.DEVELOP,
]);

// AppDev remains routable and implemented while its primary entry is hidden by product policy.
const TEMPORARILY_HIDDEN_MENU_PATHS = new Set<string>([
  SPACE_SUB_MODULE.APP_DEV,
]);

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
    label: '网页应用开发',
    path: SPACE_SUB_MODULE.APP_DEV,
    dataTestId: 'navigation_workspace_app_dev',
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
    label: '工作空间',
    path: SPACE_SUB_MODULE.WORKSPACE,
    dataTestId: 'navigation_workspace_settings',
  },
  {
    label: '任务中心',
    path: SPACE_SUB_MODULE.TASK_CENTER,
    dataTestId: 'navigation_workspace_task_center',
  },
  {
    label: '全部任务',
    path: SPACE_SUB_MODULE.TASKS,
    dataTestId: 'navigation_workspace_tasks',
  },
];

const isDeveloperFeatureDisabled = (space?: WorkspaceMenuPolicySpace) => {
  if (!space) {
    return false;
  }

  const allowDevelop = space.allow_develop;
  if (allowDevelop !== false && allowDevelop !== 0) {
    return false;
  }

  return true;
};

export const getVisibleWorkspaceMenuMeta = (space?: WorkspaceMenuPolicySpace) =>
  WORKSPACE_MENU_META.filter(
    item =>
      !TEMPORARILY_HIDDEN_MENU_PATHS.has(item.path) &&
      (!isDeveloperFeatureDisabled(space) ||
        !DEVELOPER_FEATURE_MENU_PATHS.has(item.path)),
  );

export const shouldShowSystemManagementEntry = ({
  hasUser,
  isSystemAdmin,
}: {
  hasUser?: boolean;
  isSystemAdmin?: boolean;
}) => Boolean(hasUser && isSystemAdmin);
