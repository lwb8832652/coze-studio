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

import { type ReactElement, type ReactNode } from 'react';

import { vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

const mockOpenAccountSettings = vi.hoisted(() => vi.fn());
const mockOpenLogoutModal = vi.hoisted(() => vi.fn());

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror package component names. */
vi.mock('@coze-foundation/layout', () => ({
  GlobalLayoutAccountDropdown: ({
    children,
    menus,
  }: {
    children?: ReactNode;
    menus: Array<
      | ReactElement
      | {
          dataTestId?: string;
          onClick?: () => void;
          title: string;
        }
    >;
  }) => (
    <div>
      <div data-testid="account-dropdown-menu">
        {menus.map((item, index) =>
          'title' in item ? (
            <button
              key={`${item.title}-${index}`}
              type="button"
              data-testid={item.dataTestId}
              onClick={item.onClick}
            >
              {item.title}
            </button>
          ) : (
            <span key={index} data-testid="account-dropdown-divider" />
          ),
        )}
      </div>
      {children}
    </div>
  ),
}));

vi.mock('@coze-foundation/account-ui-adapter', () => ({
  useLogout: () => ({
    node: <div data-testid="logout-modal" />,
    open: mockOpenLogoutModal,
  }),
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({ name: 'codex-smoke', screen_name: 'codex-smoke' }),
}));

vi.mock('@coze-arch/i18n', () => ({
  I18n: {
    t: (key: string) =>
      ({
        basic_log_out: '退出登录',
        navi_bar_account_settings: '账号设置',
        settings_api_authorization: 'API 授权',
      })[key] || key,
  },
}));

vi.mock('@coze-arch/coze-design/icons', () => {
  const icon = () => <span data-testid="menu-icon" />;

  return {
    IconCozExit: icon,
    IconCozPlugin: icon,
    IconCozSetting: icon,
  };
});

vi.mock('@coze-arch/coze-design', () => ({
  Dropdown: {
    Divider: () => <span data-testid="coze-dropdown-divider" />,
  },
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after package mocks. */

vi.mock('../account-settings', () => ({
  useAccountSettings: () => ({
    node: <div data-testid="account-settings-node" />,
    open: mockOpenAccountSettings,
  }),
}));

import { AccountDropdown } from '../index';

describe('AccountDropdown', () => {
  beforeEach(() => {
    mockOpenAccountSettings.mockReset();
    mockOpenLogoutModal.mockReset();
  });

  it('renders extra settings tabs as account dropdown entries', () => {
    render(
      <AccountDropdown
        extraSettingsTabs={[
          {
            id: 'mcp-tools',
            tabName: 'MCP 配置',
            content: () => <div data-testid="mcp-settings-panel" />,
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole('button', { name: 'MCP 配置' }));

    expect(screen.getByText('MCP 配置')).toBeTruthy();
    expect(mockOpenAccountSettings).toHaveBeenCalledWith('mcp-tools');
  });
});
