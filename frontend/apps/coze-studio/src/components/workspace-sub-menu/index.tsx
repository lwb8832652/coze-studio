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

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive orchestrator. */
import { useLocation, useNavigate } from 'react-router-dom';
import { useCallback, useMemo, useState } from 'react';

import { WorkspaceSubMenu as BaseWorkspaceSubMenu } from '@coze-foundation/space-ui-base';
import { useSpaceStore } from '@coze-foundation/space-store';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozArrowDown,
  IconCozAsynchronousTask,
  IconCozAsynchronousTaskFill,
  IconCozBot,
  IconCozBotFill,
  IconCozClock,
  IconCozClockFill,
  IconCozCode,
  IconCozCodeFill,
  IconCozKnowledge,
  IconCozKnowledgeFill,
  IconCozMore,
  IconCozPlus,
  IconCozSideExpand,
  IconCozSkill,
  IconCozWorkspace,
  IconCozWorkspaceFill,
} from '@coze-arch/coze-design/icons';
import { Input, Modal, Toast, Typography } from '@coze-arch/coze-design';
import { useRouteConfig } from '@coze-arch/bot-hooks';
import { SpaceType, type BotSpace } from '@coze-arch/bot-api/developer_api';

import { WorkspaceMark } from '../workspace-mark';
import { WorkspaceAccountDropdown } from '../workspace-account-dropdown';
import { WorkspaceTaskList } from './workspace-task-list';
import {
  ASSISTANT_BADGE,
  ASSISTANT_LABEL,
  SPACE_SUB_MODULE,
  getVisibleWorkspaceMenuMeta,
  type WorkspaceMenuPolicySpace,
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
  [SPACE_SUB_MODULE.APP_DEV]: {
    icon: <IconCozCode />,
    activeIcon: <IconCozCodeFill />,
  },
  [SPACE_SUB_MODULE.SKILL]: {
    icon: <IconCozSkill />,
    activeIcon: <IconCozSkill />,
  },
  [SPACE_SUB_MODULE.DEVELOP]: {
    icon: <IconCozBot />,
    activeIcon: <IconCozBotFill />,
  },
  [SPACE_SUB_MODULE.WORKSPACE]: {
    icon: <IconCozWorkspace />,
    activeIcon: <IconCozWorkspaceFill />,
  },
  [SPACE_SUB_MODULE.TASK_CENTER]: {
    icon: <IconCozClock />,
    activeIcon: <IconCozClockFill />,
  },
  [SPACE_SUB_MODULE.TASKS]: {
    icon: <IconCozAsynchronousTask />,
    activeIcon: <IconCozAsynchronousTaskFill />,
  },
};

const SPACE_SEARCH_THRESHOLD = 10;
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

const getSpaceDisplayName = (space?: BotSpace, fallbackName?: string) => {
  if (space?.space_type === SpaceType.Personal) {
    return '个人空间';
  }

  return space?.name || fallbackName || '工作空间';
};

const getSpaceTypeLabel = (space?: BotSpace) =>
  space?.space_type === SpaceType.Personal ? '个人空间' : '团队空间';

const shouldShowSpaceTypeLabel = (space?: BotSpace, fallbackName?: string) =>
  getSpaceDisplayName(space, fallbackName) !== getSpaceTypeLabel(space);

const buildSpaceSwitchPath = ({
  currentSpaceId,
  nextSpaceId,
  pathname,
  search,
}: {
  currentSpaceId?: string;
  nextSpaceId: string;
  pathname: string;
  search: string;
}) => {
  const defaultPath = `/space/${nextSpaceId}/${SPACE_SUB_MODULE.WORKBENCH}`;

  if (!currentSpaceId) {
    return defaultPath;
  }

  const currentSpacePrefix = `/space/${currentSpaceId}`;
  if (
    pathname === currentSpacePrefix ||
    pathname.startsWith(`${currentSpacePrefix}/`)
  ) {
    return `${pathname.replace(
      currentSpacePrefix,
      `/space/${nextSpaceId}`,
    )}${search}`;
  }

  return defaultPath;
};

interface WorkspaceSwitcherProps {
  currentSpace?: BotSpace;
  fallbackName?: string;
  sidebarCollapsed?: boolean;
  onToggleSidebarCollapsed?: () => void;
}

const WorkspaceSwitcher = ({
  currentSpace,
  fallbackName,
  sidebarCollapsed,
  onToggleSidebarCollapsed,
}: WorkspaceSwitcherProps) => {
  const navigate = useNavigate();
  const location = useLocation();
  const spaceList = useSpaceStore(state => state.spaceList);
  const setSpace = useSpaceStore(state => state.setSpace);
  const createSpace = useSpaceStore(state => state.createSpace);
  const fetchSpaces = useSpaceStore(state => state.fetchSpaces);
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [teamName, setTeamName] = useState('');
  const [teamDescription, setTeamDescription] = useState('');
  const [spaceSearchKeyword, setSpaceSearchKeyword] = useState('');
  const [creating, setCreating] = useState(false);
  const [formError, setFormError] = useState('');
  const currentSpaceId = currentSpace?.id;
  const currentSpaceName = getSpaceDisplayName(currentSpace, fallbackName);
  const allSpaces = useMemo(() => {
    const dedupedSpaces = new Map<string, BotSpace>();

    [...spaceList, currentSpace].forEach(space => {
      if (space?.id) {
        dedupedSpaces.set(space.id, space);
      }
    });

    return Array.from(dedupedSpaces.values());
  }, [currentSpace, spaceList]);
  const otherSpaces = allSpaces.filter(space => space.id !== currentSpaceId);
  const showSpaceSearch = otherSpaces.length > SPACE_SEARCH_THRESHOLD;
  const normalizedSpaceKeyword = spaceSearchKeyword.trim().toLowerCase();
  const visibleOtherSpaces = normalizedSpaceKeyword
    ? otherSpaces.filter(space =>
        getSpaceDisplayName(space)
          .toLowerCase()
          .includes(normalizedSpaceKeyword),
      )
    : otherSpaces;

  const switchSpace = (space: BotSpace) => {
    if (!space.id) {
      return;
    }

    setSpace(space.id);
    setDropdownOpen(false);
    setSpaceSearchKeyword('');
    navigate(
      buildSpaceSwitchPath({
        currentSpaceId,
        nextSpaceId: space.id,
        pathname: location.pathname,
        search: location.search,
      }),
    );
  };

  const openCreateModal = () => {
    setDropdownOpen(false);
    setSpaceSearchKeyword('');
    setFormError('');
    setCreateModalOpen(true);
  };

  const closeCreateModal = () => {
    if (creating) {
      return;
    }

    setCreateModalOpen(false);
    setTeamName('');
    setTeamDescription('');
    setFormError('');
  };

  const handleCreateTeamSpace = async () => {
    const nextName = teamName.trim();
    if (!nextName) {
      setFormError('请输入团队空间名称');
      return;
    }

    setCreating(true);
    setFormError('');

    try {
      const createdSpace = await createSpace({
        name: nextName,
        description: teamDescription.trim(),
        icon_uri: '',
        space_type: SpaceType.Team,
      });

      if (createdSpace?.check_not_pass) {
        setFormError('团队空间名称未通过校验，请调整后重试');
        return;
      }

      await fetchSpaces(true);

      if (createdSpace?.id) {
        setSpace(createdSpace.id);
        navigate(`/space/${createdSpace.id}/${SPACE_SUB_MODULE.WORKBENCH}`);
      }

      Toast.success({
        content: '团队空间创建成功',
        showClose: false,
      });
      closeCreateModal();
    } catch {
      setFormError('创建团队空间失败，请稍后重试');
      Toast.error({
        content: '创建团队空间失败，请稍后重试',
        showClose: false,
      });
    } finally {
      setCreating(false);
    }
  };

  const renderSpaceRow = (space: BotSpace) => {
    const isCurrent = space.id === currentSpaceId;

    return (
      <button
        key={space.id || space.name}
        type="button"
        className="coze-prototype-space-dropdown-row"
        data-current={isCurrent}
        onClick={() => switchSpace(space)}
      >
        <span className="coze-prototype-space-check" aria-hidden="true">
          {isCurrent ? '✓' : ''}
        </span>
        <span className="coze-prototype-space-row-main">
          <span className="coze-prototype-space-row-name">
            {getSpaceDisplayName(space)}
          </span>
          {shouldShowSpaceTypeLabel(space) ? (
            <span className="coze-prototype-space-row-type">
              {getSpaceTypeLabel(space)}
            </span>
          ) : null}
        </span>
      </button>
    );
  };

  return (
    <>
      <div className="coze-prototype-workspace-switcher">
        <div className="coze-prototype-sidebar-header coze-prototype-sidebar-header-button">
          <button
            type="button"
            className="coze-prototype-workspace-trigger"
            aria-haspopup="menu"
            aria-expanded={dropdownOpen}
            onClick={() => {
              const nextOpen = !dropdownOpen;
              if (!nextOpen) {
                setSpaceSearchKeyword('');
              }
              setDropdownOpen(nextOpen);
            }}
          >
            <WorkspaceMark />
            <Typography.Text
              ellipsis={{ showTooltip: true, rows: 1 }}
              className="coze-prototype-workspace-title"
            >
              {currentSpaceName}
            </Typography.Text>
            <span
              className="coze-prototype-header-icon-button"
              aria-hidden="true"
            >
              <IconCozArrowDown
                className="text-[14px]"
                data-open={dropdownOpen}
              />
            </span>
          </button>
          <button
            type="button"
            className="coze-prototype-header-icon-button coze-prototype-sidebar-collapse-button"
            aria-label={sidebarCollapsed ? '展开菜单' : '折叠菜单'}
            aria-expanded={!sidebarCollapsed}
            title={sidebarCollapsed ? '展开菜单' : '折叠菜单'}
            data-testid="workspace_sidebar_collapse_button"
            onClick={onToggleSidebarCollapsed}
          >
            <IconCozSideExpand className="text-[14px]" />
          </button>
        </div>

        {dropdownOpen ? (
          <div className="coze-prototype-space-dropdown" role="menu">
            <div
              className="coze-prototype-space-current-row"
              aria-label="当前工作空间"
            >
              <span className="coze-prototype-space-check" aria-hidden="true">
                ✓
              </span>
              <span className="coze-prototype-space-row-main">
                <span className="coze-prototype-space-row-name">
                  {currentSpaceName}
                </span>
                {shouldShowSpaceTypeLabel(currentSpace, fallbackName) ? (
                  <span className="coze-prototype-space-row-type">
                    {getSpaceTypeLabel(currentSpace)}
                  </span>
                ) : null}
              </span>
            </div>
            {showSpaceSearch ? (
              <div className="coze-prototype-space-search-row">
                <Input
                  value={spaceSearchKeyword}
                  placeholder="搜索空间"
                  onChange={(value: string) => setSpaceSearchKeyword(value)}
                />
              </div>
            ) : null}
            <div className="coze-prototype-space-dropdown-section">
              {visibleOtherSpaces.length ? (
                visibleOtherSpaces.map(renderSpaceRow)
              ) : (
                <div className="coze-prototype-space-empty-row">
                  没有匹配的空间
                </div>
              )}
            </div>
            <button
              type="button"
              className="coze-prototype-space-create-row"
              onClick={openCreateModal}
            >
              <IconCozPlus className="text-[14px]" />
              <span>创建团队空间</span>
            </button>
          </div>
        ) : null}
      </div>

      <Modal
        title="创建团队空间"
        visible={createModalOpen}
        okText="创建"
        cancelText="取消"
        maskClosable={false}
        onOk={() => void handleCreateTeamSpace()}
        onCancel={closeCreateModal}
      >
        <div className="coze-prototype-create-space-form">
          <div className="coze-prototype-create-space-intro">
            <span className="coze-prototype-create-space-avatar" aria-hidden>
              {teamName.trim().slice(0, 1) || '团'}
            </span>
            <p>
              通过创建团队空间，将支持智能体、插件、工作流、大模型和知识库在团队内进行协作和分享。
            </p>
          </div>
          <label className="coze-prototype-create-space-field">
            <span>团队空间名称</span>
            <Input
              value={teamName}
              maxLength={50}
              placeholder="请输入团队空间名称"
              onChange={(value: string) => {
                setTeamName(value);
                setFormError('');
              }}
            />
          </label>
          <label className="coze-prototype-create-space-field">
            <span>团队空间描述</span>
            <textarea
              value={teamDescription}
              maxLength={2000}
              placeholder="描述这个团队空间的协作目标"
              onChange={event => setTeamDescription(event.target.value)}
            />
          </label>
          {formError ? (
            <div className="coze-prototype-create-space-error">{formError}</div>
          ) : null}
          {creating ? (
            <div className="coze-prototype-create-space-hint">正在创建...</div>
          ) : null}
        </div>
      </Modal>
    </>
  );
};

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

  const menus = getVisibleWorkspaceMenuMeta(
    currentSpace as WorkspaceMenuPolicySpace | undefined,
  ).map(item => ({
    ...item,
    ...MENU_ICONS[item.path],
    suffix:
      item.path === SPACE_SUB_MODULE.TASKS ? (
        <IconCozMore className="text-[16px]" />
      ) : undefined,
    title: () => item.label,
  }));
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
      <WorkspaceSwitcher
        currentSpace={currentSpace}
        fallbackName={workspaceDisplayName}
        sidebarCollapsed={sidebarCollapsed}
        onToggleSidebarCollapsed={toggleSidebarCollapsed}
      />
      <div className="coze-prototype-sidebar-section">
        <div className="coze-prototype-assistant-card">
          <span className="coze-prototype-assistant-dot" aria-hidden="true" />
          <span
            className="min-w-0 flex-1 truncate text-[13px] leading-[20px]"
            style={{ color: 'var(--newx-color-text-primary)' }}
          >
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
        <WorkspaceAccountDropdown />
        <Typography.Text
          ellipsis={{ showTooltip: true, rows: 1 }}
          className="min-w-0 flex-1 text-[13px] leading-[20px] font-[500]"
          style={{ color: 'var(--newx-color-text-primary)' }}
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
