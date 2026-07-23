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

import { useNavigate } from 'react-router-dom';
import { useEffect, useMemo, useState } from 'react';

import { useSpaceStore } from '@coze-foundation/space-store';
import { type AccountSettingsExtraTab } from '@coze-foundation/global-adapter/account-settings';
import { AccountDropdown } from '@coze-foundation/global-adapter/account-dropdown';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import { IconCozSetting } from '@coze-arch/coze-design/icons';

import {
  WORKSPACE_MODEL_SETTINGS_TAB_ID,
  WorkspaceModelSettingsPanel,
} from '../pages/tools/workspace-model-settings-panel';
import {
  MCP_TOOL_SETTINGS_TAB_ID,
  MCPToolSettingsPanel,
} from '../pages/tools/mcp-settings-panel';
import {
  FEISHU_IM_SETTINGS_TAB_ID,
  FeishuIMSettingsPanel,
} from '../pages/tools/feishu-im-settings-panel';
import { getSystemAdminStatus } from '../pages/system/service';
import {
  SYSTEM_MANAGEMENT_ENTRY,
  shouldShowSystemManagementEntry,
} from './workspace-sub-menu/menu';

const systemAdminStatusRequests = new Map<string, Promise<boolean>>();

const getSharedSystemAdminStatus = (userKey: string) => {
  const pendingRequest = systemAdminStatusRequests.get(userKey);
  if (pendingRequest) {
    return pendingRequest;
  }

  const request = getSystemAdminStatus().then(status => status.is_admin);
  systemAdminStatusRequests.set(userKey, request);

  const clearPendingRequest = () => {
    if (systemAdminStatusRequests.get(userKey) === request) {
      systemAdminStatusRequests.delete(userKey);
    }
  };
  void request.then(clearPendingRequest, clearPendingRequest);

  return request;
};

export const WorkspaceAccountDropdown = () => {
  const navigate = useNavigate();
  const currentSpace = useSpaceStore(state => state.space);
  const userInfo = useUserInfo();
  const userId = userInfo?.user_id_str;
  const [systemAdminState, setSystemAdminState] = useState({
    userId,
    isAdmin: false,
  });
  const hasUser = Boolean(userInfo);
  const isSystemAdmin =
    systemAdminState.userId === userId && systemAdminState.isAdmin;

  const accountSettingsTabs = useMemo<AccountSettingsExtraTab[]>(
    () => [
      {
        id: WORKSPACE_MODEL_SETTINGS_TAB_ID,
        tabName: '模型管理',
        content: () => (
          <WorkspaceModelSettingsPanel spaceId={currentSpace?.id} />
        ),
      },
      {
        id: MCP_TOOL_SETTINGS_TAB_ID,
        tabName: 'MCP 配置',
        content: () => <MCPToolSettingsPanel spaceId={currentSpace?.id} />,
      },
      {
        id: FEISHU_IM_SETTINGS_TAB_ID,
        tabName: 'IM 机器人',
        content: () => <FeishuIMSettingsPanel spaceId={currentSpace?.id} />,
      },
    ],
    [currentSpace?.id],
  );
  const accountExtraMenuItems = useMemo(() => {
    const items = [
      {
        key: 'billing-center',
        prefixIcon: <IconCozSetting />,
        title: '积分与订阅',
        onClick: () => navigate('/billing/subscriptions'),
        dataTestId: 'layout_avatar_billing-center',
      },
    ];
    if (
      shouldShowSystemManagementEntry({
        hasUser,
        isSystemAdmin,
      })
    ) {
      items.push({
        key: 'system-management',
        prefixIcon: <IconCozSetting />,
        title: SYSTEM_MANAGEMENT_ENTRY.label,
        onClick: () => {
          navigate(SYSTEM_MANAGEMENT_ENTRY.path);
        },
        dataTestId: 'layout_avatar_system-management',
      });
    }
    return items;
  }, [hasUser, isSystemAdmin, navigate]);

  useEffect(() => {
    let canceled = false;

    setSystemAdminState({
      userId,
      isAdmin: false,
    });

    if (!userId) {
      return () => {
        canceled = true;
      };
    }

    void getSharedSystemAdminStatus(userId)
      .then(isAdmin => {
        if (canceled) {
          return;
        }

        setSystemAdminState(current =>
          current.userId === userId
            ? {
                userId,
                isAdmin,
              }
            : current,
        );
      })
      .catch(() => {
        if (canceled) {
          return;
        }

        setSystemAdminState(current =>
          current.userId === userId
            ? {
                userId,
                isAdmin: false,
              }
            : current,
        );
      });

    return () => {
      canceled = true;
    };
  }, [userId]);

  return (
    <AccountDropdown
      extraSettingsTabs={accountSettingsTabs}
      extraMenuItems={accountExtraMenuItems}
    />
  );
};
