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

import { describe, expect, it } from 'vitest';

import {
  getActiveSystemSection,
  formatAdminTime,
  getI18nText,
  getRoleLabel,
  getSystemSectionContent,
  getWorkspaceDisplayDescription,
  getWorkspaceDisplayName,
} from '../view-model';

describe('system view model helpers', () => {
  it('maps workspace member roles to readable labels', () => {
    expect(getRoleLabel(1)).toBe('所有者');
    expect(getRoleLabel(2)).toBe('管理员');
    expect(getRoleLabel(3)).toBe('成员');
    expect(getRoleLabel(0)).toBe('未知角色');
    expect(getRoleLabel()).toBe('未知角色');
  });

  it('prefers Chinese i18n text and falls back to English', () => {
    expect(getI18nText({ zh_cn: 'OpenAI 模型', en_us: 'OpenAI Model' })).toBe(
      'OpenAI 模型',
    );
    expect(getI18nText({ en_us: 'OpenAI Model' })).toBe('OpenAI Model');
    expect(getI18nText({})).toBe('-');
    expect(getI18nText()).toBe('-');
  });

  it('normalizes active system section keys', () => {
    expect(getActiveSystemSection()).toBe('overview');
    expect(getActiveSystemSection('unknown')).toBe('overview');
    expect(getActiveSystemSection('users')).toBe('users');
    expect(getActiveSystemSection('workspaces')).toBe('workspaces');
    expect(getActiveSystemSection('models')).toBe('models');
    expect(getActiveSystemSection('settings')).toBe('settings');
  });

  it('returns content for the normalized system section', () => {
    expect(getSystemSectionContent('models').heading).toBe('模型配置');
    expect(getSystemSectionContent('settings').heading).toBe('系统配置');
    expect(getSystemSectionContent('unknown').heading).toBe('系统管理概览');
  });

  it('localizes default personal workspace display values', () => {
    expect(
      getWorkspaceDisplayName({
        name: 'Personal Space',
      }),
    ).toBe('个人空间');
    expect(
      getWorkspaceDisplayDescription({
        name: 'Personal Space',
        description: 'This is your personal space',
      }),
    ).toBe('默认个人空间');
    expect(
      getWorkspaceDisplayName({
        name: '畅享 AI',
      }),
    ).toBe('畅享 AI');
  });

  it('formats both second and millisecond timestamps', () => {
    expect(formatAdminTime(1710000000)).not.toContain('584');
    expect(formatAdminTime(1710000000000)).not.toContain('561');
    expect(formatAdminTime()).toBe('-');
  });
});
