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

import { useCallback, useMemo, useState } from 'react';

import { WorkspaceSubMenu as BaseWorkspaceSubMenu } from '@coze-foundation/space-ui-base';
import { useSpaceStore } from '@coze-foundation/space-store';
import {
  AccountDropdown,
  type AccountSettingsExtraTab,
} from '@coze-foundation/global-adapter';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozArrowDown,
  IconCozAsynchronousTask,
  IconCozAsynchronousTaskFill,
  IconCozCode,
  IconCozCodeFill,
  IconCozKnowledge,
  IconCozKnowledgeFill,
  IconCozMore,
  IconCozPlus,
  IconCozSetting,
  IconCozSettingFill,
  IconCozSideExpand,
} from '@coze-arch/coze-design/icons';
import { Typography } from '@coze-arch/coze-design';
import { useRouteConfig } from '@coze-arch/bot-hooks';

import {
  MCP_TOOL_SETTINGS_TAB_ID,
  MCPToolSettingsPanel,
} from '../../pages/tools/mcp-settings-panel';
import { WorkspaceTaskList } from './workspace-task-list';
import {
  ASSISTANT_BADGE,
  ASSISTANT_LABEL,
  SPACE_SUB_MODULE,
  WORKSPACE_MENU_META,
} from './menu';

import '../workspace-prototype.less';

const MENU_ICONS = {
  [SPACE_SUB_MODULE.WORKBENCH]: {
    icon: <IconCozPlus />,
    activeIcon: <IconCozPlus />,
  },
  [SPACE_SUB_MODULE.LIBRARY]: {
    icon: <IconCozKnowledge />,
    activeIcon: <IconCozKnowledgeFill />,
  },
  [SPACE_SUB_MODULE.SKILL]: {
    icon: <IconCozSetting />,
    activeIcon: <IconCozSettingFill />,
  },
  [SPACE_SUB_MODULE.DEVELOP]: {
    icon: <IconCozCode />,
    activeIcon: <IconCozCodeFill />,
  },
  [SPACE_SUB_MODULE.TASKS]: {
    icon: <IconCozAsynchronousTask />,
    activeIcon: <IconCozAsynchronousTaskFill />,
  },
};
const WORKSPACE_SUBMENU_STORAGE_KEY = 'workspace-submenu-width';
const WORKSPACE_SUBMENU_COLLAPSE_EVENT =
  'coze-workspace-submenu-collapse-change';

const readStoredSidebarCollapsed = () => {
  try {
    return (
      localStorage.getItem(`${WORKSPACE_SUBMENU_STORAGE_KEY}:collapsed`) ===
      'true'
    );
  } catch (error) {
    console.warn('Failed to read workspace sidebar collapsed state.', error);

    return false;
  }
};

const WorkspaceMark = () => (
  <span className="coze-prototype-workspace-mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" className="h-[16px] w-[16px]" fill="currentColor">
      <path d="M12 2 2 22h20L12 2zm0 6 6 12H6l6-12z" />
    </svg>
  </span>
);

export const WorkspaceSubMenu = () => {
  const { subMenuKey } = useRouteConfig();
  const currentSpace = useSpaceStore(state => state.space);
  const userInfo = useUserInfo();
  const [sidebarCollapsed, setSidebarCollapsed] = useState(
    readStoredSidebarCollapsed,
  );
  const userDisplayName = userInfo?.name || userInfo?.screen_name;
  const workspaceDisplayName = userDisplayName
    ? `${userDisplayName} 的工作空间`
    : currentSpace?.name || '';

  const menus = WORKSPACE_MENU_META.map(item => ({
    ...item,
    ...MENU_ICONS[item.path],
    suffix:
      item.path === SPACE_SUB_MODULE.TASKS ? (
        <IconCozMore className="text-[16px]" />
      ) : undefined,
    title: () => item.label,
  }));
  const mcpSettingsTabs = useMemo<AccountSettingsExtraTab[]>(
    () => [
      {
        id: MCP_TOOL_SETTINGS_TAB_ID,
        tabName: 'MCP 配置',
        content: () => <MCPToolSettingsPanel spaceId={currentSpace?.id} />,
      },
    ],
    [currentSpace?.id],
  );
  const toggleSidebarCollapsed = useCallback(() => {
    setSidebarCollapsed(current => {
      const nextCollapsed = !current;

      window.dispatchEvent(
        new CustomEvent(WORKSPACE_SUBMENU_COLLAPSE_EVENT, {
          detail: {
            storageKey: WORKSPACE_SUBMENU_STORAGE_KEY,
            collapsed: nextCollapsed,
          },
        }),
      );

      return nextCollapsed;
    });
  }, []);

  const headerNode = (
    <div className="w-full">
      <div className="coze-prototype-sidebar-header">
        <WorkspaceMark />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="coze-prototype-workspace-title"
        >
          {workspaceDisplayName}
        </Typography.Text>
        <span className="coze-prototype-header-icon-button" aria-hidden="true">
          <IconCozArrowDown className="text-[14px]" />
        </span>
        <button
          type="button"
          className="coze-prototype-header-icon-button coze-prototype-sidebar-collapse-button"
          aria-label={sidebarCollapsed ? '展开菜单' : '折叠菜单'}
          aria-expanded={!sidebarCollapsed}
          title={sidebarCollapsed ? '展开菜单' : '折叠菜单'}
          data-testid="workspace_sidebar_collapse_button"
          onClick={toggleSidebarCollapsed}
        >
          <IconCozSideExpand className="text-[14px]" />
        </button>
      </div>
      <div className="coze-prototype-sidebar-section">
        <div className="coze-prototype-assistant-card">
          <span className="coze-prototype-assistant-dot" aria-hidden="true" />
          <span className="min-w-0 flex-1 truncate text-[13px] leading-[20px] text-[#232938]">
            {ASSISTANT_LABEL}
          </span>
          <span className="coze-prototype-beta-pill">{ASSISTANT_BADGE}</span>
        </div>
      </div>
    </div>
  );

  const footerNode = userInfo ? (
    <div className="coze-prototype-sidebar-footer">
      <div className="flex min-w-0 items-center gap-[8px]">
        <AccountDropdown extraSettingsTabs={mcpSettingsTabs} />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="min-w-0 flex-1 text-[13px] leading-[20px] font-[500] text-[#232938]"
        >
          {userInfo.name || userInfo.screen_name}
        </Typography.Text>
      </div>
    </div>
  ) : null;

  return (
    <BaseWorkspaceSubMenu
      header={headerNode}
      menus={menus}
      currentSubMenu={subMenuKey}
      bottomPanel={<WorkspaceTaskList />}
      footer={footerNode}
      collapsed={sidebarCollapsed}
    />
  );
};

export default WorkspaceSubMenu;
