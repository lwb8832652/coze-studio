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

import { type ReactNode, useState } from 'react';

import { GlobalLayoutAccountDropdown } from '@coze-foundation/layout';
import { useLogout } from '@coze-foundation/account-ui-adapter';
import { I18n } from '@coze-arch/i18n';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozExit,
  IconCozPlugin,
  IconCozSetting,
} from '@coze-arch/coze-design/icons';
import { Dropdown } from '@coze-arch/coze-design';

import { UserInfoMenu } from './user-info-menu';
import {
  type AccountSettingsExtraTab,
  useAccountSettings,
} from './account-settings';

interface AccountDropdownProps {
  extraSettingsTabs?: AccountSettingsExtraTab[];
  extraMenuItems?: AccountDropdownExtraMenuItem[];
}

export interface AccountDropdownExtraMenuItem {
  key: string;
  prefixIcon?: ReactNode;
  title: string;
  onClick: () => void;
  dataTestId?: string;
}

export const AccountDropdown = ({
  extraSettingsTabs = [],
  extraMenuItems = [],
}: AccountDropdownProps) => {
  const [visible, setVisible] = useState(false);
  const userInfo = useUserInfo();
  const { node: logoutModal, open: openLogoutModal } = useLogout();

  const { node: accountSettingsNode, open: openAccountSettings } =
    useAccountSettings(extraSettingsTabs);

  if (!userInfo) {
    return null;
  }

  const extraSettingsMenus = extraSettingsTabs.map((item, index) => {
    if (item === 'divider') {
      return <Dropdown.Divider key={`extra-settings-divider-${index}`} />;
    }

    return {
      prefixIcon: <IconCozPlugin />,
      title: item.tabName,
      onClick: () => {
        openAccountSettings(item.id);
      },
      dataTestId: `layout_avatar_${item.id}`,
    };
  });
  const accountExtraMenus = extraMenuItems.map(item => ({
    prefixIcon: item.prefixIcon,
    title: item.title,
    onClick: item.onClick,
    dataTestId: item.dataTestId,
  }));

  return (
    <GlobalLayoutAccountDropdown
      menus={[
        <UserInfoMenu />,
        <Dropdown.Divider />,
        {
          prefixIcon: <IconCozExit />,
          title: I18n.t('settings_api_authorization'),
          onClick: () => {
            openAccountSettings('api-auth');
          },
          dataTestId: 'layout_avatar_api-auth',
        },
        ...extraSettingsMenus,
        {
          prefixIcon: <IconCozSetting />,
          title: I18n.t('navi_bar_account_settings'),
          onClick: () => {
            openAccountSettings('account');
          },
          dataTestId: 'layout_avatar_profile-settings',
        },
        ...(accountExtraMenus.length
          ? [
              <Dropdown.Divider key="extra-menu-divider" />,
              ...accountExtraMenus,
            ]
          : []),
        <Dropdown.Divider />,
        {
          prefixIcon: <IconCozExit />,
          title: I18n.t('basic_log_out'),
          onClick: () => {
            openLogoutModal();
          },
          dataTestId: 'layout_avatar_logout-button',
        },
      ]}
      visible={visible}
      onVisibleChange={setVisible}
    >
      {logoutModal}
      {accountSettingsNode}
    </GlobalLayoutAccountDropdown>
  );
};
