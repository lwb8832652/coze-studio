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

import {
  SECTION_CONTENT,
  SYSTEM_SECTIONS,
  type SystemSectionKey,
} from './content';

export const getActiveSystemSection = (section?: string): SystemSectionKey => {
  if (SYSTEM_SECTIONS.some(item => item.key === section)) {
    return section as SystemSectionKey;
  }
  return 'overview';
};

export const getSystemSectionContent = (section?: string) =>
  SECTION_CONTENT[getActiveSystemSection(section)];

export const getRoleLabel = (roleType?: number) => {
  if (roleType === 1) {
    return '所有者';
  }
  if (roleType === 2) {
    return '管理员';
  }
  if (roleType === 3) {
    return '成员';
  }
  return '未知角色';
};

export const getI18nText = (text?: { zh_cn?: string; en_us?: string }) =>
  text?.zh_cn || text?.en_us || '-';

export const isDefaultPersonalWorkspaceName = (name?: string) =>
  name?.trim().toLowerCase() === 'personal space';

export const isDefaultPersonalWorkspaceDescription = (description?: string) =>
  description?.trim().toLowerCase() === 'this is your personal space';

export const isPersonalWorkspace = (workspace?: {
  name?: string;
  space_type?: number;
}) =>
  workspace?.space_type === 1 ||
  isDefaultPersonalWorkspaceName(workspace?.name);

export const getWorkspaceTypeLabel = (workspace?: {
  name?: string;
  space_type?: number;
}) => (isPersonalWorkspace(workspace) ? '个人空间' : '团队空间');

export const getWorkspaceDisplayName = (workspace?: {
  name?: string;
  space_type?: number;
}) => {
  if (isPersonalWorkspace(workspace)) {
    return '个人空间';
  }

  return workspace?.name || '-';
};

export const getWorkspaceDisplayDescription = (workspace?: {
  description?: string;
  name?: string;
  space_type?: number;
}) => {
  if (
    isPersonalWorkspace(workspace) ||
    isDefaultPersonalWorkspaceDescription(workspace?.description)
  ) {
    return '默认个人空间';
  }

  return workspace?.description || '暂无描述';
};

export const formatAdminTime = (timestamp?: number) => {
  if (!timestamp) {
    return '-';
  }

  const normalizedTimestamp =
    timestamp < 1000000000000 ? timestamp * 1000 : timestamp;

  return new Date(normalizedTimestamp).toLocaleString('zh-CN', {
    hour12: false,
  });
};
