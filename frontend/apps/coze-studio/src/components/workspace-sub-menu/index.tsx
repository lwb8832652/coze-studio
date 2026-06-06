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

import { WorkspaceSubMenu as BaseWorkspaceSubMenu } from '@coze-foundation/space-ui-base';
import { useSpaceStore } from '@coze-foundation/space-store';
import { AccountDropdown } from '@coze-foundation/global-adapter';
import { useRouteConfig } from '@coze-arch/bot-hooks';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import { Typography } from '@coze-arch/coze-design';
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
  IconCozTrigger,
} from '@coze-arch/coze-design/icons';

import {
  ASSISTANT_BADGE,
  ASSISTANT_LABEL,
  SPACE_SUB_MODULE,
  WORKSPACE_MENU_META,
} from './menu';
import { WorkspaceTaskList } from './workspace-task-list';

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
  [SPACE_SUB_MODULE.TASK_TRIGGER]: {
    icon: <IconCozTrigger />,
    activeIcon: <IconCozTrigger />,
  },
  [SPACE_SUB_MODULE.TASKS]: {
    icon: <IconCozAsynchronousTask />,
    activeIcon: <IconCozAsynchronousTaskFill />,
  },
};

const WorkspaceMark = () => (
  <span className="coze-prototype-workspace-mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" className="h-[16px] w-[16px]" fill="currentColor">
      <path
        d="M12 2 2 22h20L12 2zm0 6 6 12H6l6-12z"
      />
    </svg>
  </span>
);

export const WorkspaceSubMenu = () => {
  const { subMenuKey } = useRouteConfig();
  const currentSpace = useSpaceStore(state => state.space);
  const userInfo = useUserInfo();
  const userDisplayName = userInfo?.name || userInfo?.screen_name;
  const workspaceDisplayName =
    userDisplayName
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
        <span className="coze-prototype-header-icon-button" aria-hidden="true">
          <IconCozSideExpand className="text-[14px]" />
        </span>
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
        <AccountDropdown />
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
    />
  );
};

export default WorkspaceSubMenu;
