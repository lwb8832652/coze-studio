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
