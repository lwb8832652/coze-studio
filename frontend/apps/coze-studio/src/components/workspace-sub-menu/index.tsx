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
import { useRouteConfig } from '@coze-arch/bot-hooks';
import { Avatar, Space, Typography } from '@coze-arch/coze-design';
import {
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

export const WorkspaceSubMenu = () => {
  const { subMenuKey } = useRouteConfig();
  const currentSpace = useSpaceStore(state => state.space);

  const menus = WORKSPACE_MENU_META.map(item => ({
    ...item,
    ...MENU_ICONS[item.path],
    title: () => item.label,
  }));

  const headerNode = (
    <div className="w-full">
      <Space
        className="h-[48px] px-[8px] w-full hover:coz-mg-secondary-hovered rounded-[8px]"
        spacing={8}
      >
        <Avatar
          className="w-[24px] h-[24px] rounded-[6px] shrink-0"
          src={currentSpace?.icon_url}
        />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="flex-1 coz-fg-primary text-[14px] font-[500]"
        >
          {currentSpace?.name || ''}
        </Typography.Text>
      </Space>
      <div className="mt-[8px] flex items-center justify-between rounded-[8px] border border-solid coz-stroke-primary px-[10px] py-[8px] coz-bg-plus">
        <span className="text-[13px] leading-[20px] font-[500] coz-fg-primary">
          {ASSISTANT_LABEL}
        </span>
        <span className="rounded-[6px] bg-[#e8fff4] px-[6px] text-[12px] leading-[18px] font-[600] text-[#0a8f5a]">
          {ASSISTANT_BADGE}
        </span>
      </div>
    </div>
  );

  return (
    <BaseWorkspaceSubMenu
      header={headerNode}
      menus={menus}
      currentSubMenu={subMenuKey}
      bottomPanel={<WorkspaceTaskList />}
    />
  );
};

export default WorkspaceSubMenu;
