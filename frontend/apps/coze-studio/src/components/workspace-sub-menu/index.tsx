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
  IconCozBot,
  IconCozBotFill,
  IconCozCode,
  IconCozCodeFill,
  IconCozKnowledge,
  IconCozKnowledgeFill,
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

const MENU_ICONS = {
  [SPACE_SUB_MODULE.WORKBENCH]: {
    icon: <IconCozBot />,
    activeIcon: <IconCozBotFill />,
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
  <span
    className="flex h-[28px] w-[28px] shrink-0 items-center justify-center rounded-[6px] bg-gradient-to-br from-lime-300 to-green-500 text-white"
    aria-hidden="true"
  >
    <svg viewBox="0 0 24 24" className="h-[16px] w-[16px]" fill="none">
      <path
        d="M12 5.25 4.75 18.75h14.5L12 5.25Z"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinejoin="round"
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
    currentSpace?.name ||
    (userDisplayName ? `${userDisplayName} 的工作空间` : '');

  const menus = WORKSPACE_MENU_META.map(item => ({
    ...item,
    ...MENU_ICONS[item.path],
    title: () => item.label,
  }));

  const headerNode = (
    <div className="w-full">
      <div className="flex h-[60px] w-full items-center gap-[8px] border-0 border-b border-solid border-[rgba(77,101,148,0.2)] px-[12px]">
        <WorkspaceMark />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="min-w-0 flex-1 text-[14px] leading-[20px] font-[500] text-[#232938]"
        >
          {workspaceDisplayName}
        </Typography.Text>
        <span
          className="flex h-[24px] w-[24px] shrink-0 items-center justify-center rounded-[4px] border border-solid border-[rgba(77,101,148,0.2)] text-[#444c5c]"
          aria-hidden="true"
        >
          <IconCozArrowDown className="text-[14px]" />
        </span>
        <span
          className="flex h-[24px] w-[24px] shrink-0 items-center justify-center rounded-[4px] border border-solid border-[rgba(77,101,148,0.2)] text-[#444c5c]"
          aria-hidden="true"
        >
          <IconCozSideExpand className="text-[14px]" />
        </span>
      </div>
      <div className="px-[8px] pt-[16px]">
        <div className="flex h-[34px] items-center gap-[8px] rounded-[8px] border border-solid border-[rgba(77,101,148,0.2)] bg-[rgba(91,100,117,0.06)] px-[12px]">
          <span
            className="h-[20px] w-[20px] shrink-0 rounded-full bg-gradient-to-br from-green-300 to-lime-300"
            aria-hidden="true"
          />
          <span className="min-w-0 flex-1 truncate text-[13px] leading-[20px] font-[500] text-[#232938]">
            {ASSISTANT_LABEL}
          </span>
          <span
            className="flex h-[20px] shrink-0 items-center rounded-full px-[8px] text-[11px] leading-[16px] font-[500] text-[#1d2129]"
            style={{
              backgroundImage:
                'linear-gradient(65deg, rgb(246,255,120) 5%, rgb(195,255,134) 100%)',
            }}
          >
            {ASSISTANT_BADGE}
          </span>
        </div>
      </div>
    </div>
  );

  const footerNode = userInfo ? (
    <div className="border-0 border-t border-solid coz-stroke-primary px-[8px] py-[10px]">
      <div className="flex min-w-0 items-center gap-[8px]">
        <AccountDropdown />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="min-w-0 flex-1 text-[13px] leading-[20px] font-[500] coz-fg-primary"
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
