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

import { vi } from 'vitest';
import { renderHook } from '@testing-library/react';

const mockUseBaseAccountSettings = vi.hoisted(() =>
  vi.fn(() => ({
    node: <div data-testid="account-settings-node" />,
    open: vi.fn(),
  })),
);

vi.mock('@coze-arch/i18n', () => ({
  I18n: {
    t: (key: string) =>
      ({
        menu_profile_account: '账号',
        settings_api_authorization: 'API 授权',
      })[key] || key,
  },
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror package component names. */
vi.mock('@coze-foundation/account-ui-base', () => ({
  UserInfoPanel: () => <div data-testid="user-info-panel" />,
  useAccountSettings: mockUseBaseAccountSettings,
}));

vi.mock('@coze-studio/open-auth', () => ({
  PatBody: () => <div data-testid="pat-body" />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after package mocks. */

import { useAccountSettings } from '../index';

describe('useAccountSettings', () => {
  it('registers extra settings inside the settings modal tabs', () => {
    renderHook(() =>
      useAccountSettings([
        {
          id: 'mcp-tools',
          tabName: 'MCP 配置',
          content: () => <div data-testid="mcp-settings" />,
        },
      ]),
    );

    const { tabs } = mockUseBaseAccountSettings.mock.calls[0][0];

    expect(tabs.map((item: { id: string }) => item.id)).toEqual([
      'account',
      'api-auth',
      'mcp-tools',
    ]);
    expect(tabs.at(-1)).toMatchObject({
      id: 'mcp-tools',
      tabName: 'MCP 配置',
    });
  });
});
