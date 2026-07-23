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

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror package names. */

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */

import type { ReactNode } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockGetWorkspaceDetail = vi.hoisted(() => vi.fn());
const mockListWorkspaceMembers = vi.hoisted(() => vi.fn());
const mockSearchWorkspaceUsers = vi.hoisted(() => vi.fn());
const mockAddWorkspaceMembers = vi.hoisted(() => vi.fn());
const mockDeleteWorkspace = vi.hoisted(() => vi.fn());
const mockUpdateWorkspace = vi.hoisted(() => vi.fn());
const mockUpdateWorkspaceMemberRole = vi.hoisted(() => vi.fn());
const mockRemoveWorkspaceMember = vi.hoisted(() => vi.fn());
const mockTransferWorkspace = vi.hoisted(() => vi.fn());
const mockToastError = vi.hoisted(() => vi.fn());
const mockToastSuccess = vi.hoisted(() => vi.fn());
const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: '101' })));
const mockUseSpaceStore = vi.hoisted(() =>
  vi.fn((selector: (state: unknown) => unknown) =>
    selector({
      space: {
        id: '101',
        name: '本地空间',
        space_type: 2,
      },
      spaceList: [
        {
          id: '101',
          name: '本地空间',
          space_type: 2,
        },
      ],
    }),
  ),
);

vi.mock('react-router-dom', () => ({
  useParams: mockUseParams,
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: mockUseSpaceStore,
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({
    name: '测试用户',
    screen_name: '测试用户',
  }),
}));

vi.mock('@coze-arch/bot-api/developer_api', () => ({
  SpaceType: {
    Personal: 1,
    Team: 2,
  },
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozPeopleFill: () => (
    <span data-testid="default-person-avatar-icon">person-avatar-icon</span>
  ),
}));

vi.mock('@coze-arch/coze-design', () => ({
  CozAvatar: ({
    children,
    className,
    src,
    type,
  }: {
    children?: ReactNode;
    className?: string;
    src?: string;
    type?: string;
  }) => (
    <span
      className={className}
      data-testid="workspace-person-avatar"
      data-src={src}
      data-type={type}
    >
      {children}
    </span>
  ),
  Input: ({
    value,
    maxLength,
    placeholder,
    onChange,
  }: {
    value: string;
    maxLength?: number;
    placeholder?: string;
    onChange?: (value: string) => void;
  }) => (
    <input
      value={value}
      maxLength={maxLength}
      placeholder={placeholder}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
  Loading: ({ loading }: { loading: boolean }) =>
    loading ? <span data-testid="workspace-loading" /> : null,
  Modal: ({
    visible,
    title,
    children,
    okText,
    cancelText,
    onOk,
    onCancel,
  }: {
    visible: boolean;
    title: string;
    children: ReactNode;
    okText?: string;
    cancelText?: string;
    onOk?: () => void;
    onCancel?: () => void;
  }) =>
    visible ? (
      <div role="dialog">
        <h2>{title}</h2>
        {children}
        <button type="button" onClick={onCancel}>
          {cancelText}
        </button>
        <button type="button" onClick={onOk}>
          {okText}
        </button>
      </div>
    ) : null,
  Toast: {
    error: mockToastError,
    success: mockToastSuccess,
  },
}));

vi.mock('../../../components/workspace-page-top-bar', () => ({
  WorkspacePageTopBar: () => <div data-testid="workspace-top-bar" />,
}));

vi.mock('../service', () => ({
  addWorkspaceMembers: mockAddWorkspaceMembers,
  deleteWorkspace: mockDeleteWorkspace,
  getWorkspaceDetail: mockGetWorkspaceDetail,
  listWorkspaceMembers: mockListWorkspaceMembers,
  removeWorkspaceMember: mockRemoveWorkspaceMember,
  searchWorkspaceUsers: mockSearchWorkspaceUsers,
  transferWorkspace: mockTransferWorkspace,
  updateWorkspace: mockUpdateWorkspace,
  updateWorkspaceMemberRole: mockUpdateWorkspaceMemberRole,
}));

import WorkspacePage from '../index';

describe('WorkspacePage', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    mockGetWorkspaceDetail.mockResolvedValue({
      data: {
        id: '101',
        name: '畅享 AI',
        description: '团队协作空间',
        space_type: 2,
        owner_user_id: '9',
        current_user_role: 1,
        total_member_num: 2,
        allow_develop: true,
        receive_publish: false,
      },
    });
    mockListWorkspaceMembers.mockResolvedValue({
      members: [
        {
          user_id: '9',
          name: 'Owner',
          user_unique_name: 'owner',
          email: 'owner@example.test',
          role_type: 1,
          joined_at: 1717000000,
        },
        {
          user_id: '10',
          name: 'Member',
          user_unique_name: 'member',
          email: 'member@example.test',
          role_type: 3,
          joined_at: 1717000100,
        },
      ],
    });
    mockSearchWorkspaceUsers.mockResolvedValue({
      users: [
        {
          user_id: '11',
          name: 'New User',
          email: 'new@example.test',
          role_type: 0,
        },
      ],
      total: 1,
    });
    mockAddWorkspaceMembers.mockResolvedValue({ code: 0 });
    mockDeleteWorkspace.mockResolvedValue({ code: 0 });
    mockUpdateWorkspace.mockResolvedValue({ code: 0 });
    mockUpdateWorkspaceMemberRole.mockResolvedValue({ code: 0 });
    mockRemoveWorkspaceMember.mockResolvedValue({ code: 0 });
    mockTransferWorkspace.mockResolvedValue({ code: 0 });
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
    vi.clearAllMocks();
  });

  const renderPage = async () => {
    await act(async () => {
      root.render(<WorkspacePage />);
    });
    await act(async () => {
      await Promise.resolve();
    });
  };

  it('renders workspace detail and members from service data', async () => {
    await renderPage();

    expect(mockGetWorkspaceDetail).toHaveBeenCalledWith('101');
    expect(mockListWorkspaceMembers).toHaveBeenCalledWith({
      space_id: '101',
    });
    expect(container.textContent).toContain('畅享 AI');
    expect(container.textContent).toContain('成员管理');
    expect(container.textContent).toContain('添加成员');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('owner@example.test');
    expect(container.textContent).toContain('Member');
  });

  it('uses the shared person avatar for members without custom avatars', async () => {
    await renderPage();

    const avatars = Array.from(
      container.querySelectorAll('[data-testid="workspace-person-avatar"]'),
    );
    expect(avatars).toHaveLength(2);
    expect(
      avatars.every(avatar => avatar.getAttribute('data-type') === 'person'),
    ).toBe(true);
    expect(
      container.querySelectorAll('[data-testid="default-person-avatar-icon"]'),
    ).toHaveLength(2);
  });

  it('filters members by keyword and role', async () => {
    await renderPage();

    const keywordInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="搜索工作空间成员"]',
    );
    expect(keywordInput).not.toBeNull();

    await act(async () => {
      Simulate.change(keywordInput!, {
        target: {
          value: 'owner',
        },
      } as never);
    });

    expect(container.textContent).toContain('Owner');
    expect(container.textContent).not.toContain('Member');

    const roleSelect = container.querySelector<HTMLSelectElement>(
      'select[aria-label="筛选成员角色"]',
    );
    expect(roleSelect).not.toBeNull();

    await act(async () => {
      Simulate.change(roleSelect!, {
        target: {
          value: '3',
        },
      } as never);
    });

    expect(container.textContent).toContain('没有匹配的成员');
  });

  it('adds selected users from the add member modal', async () => {
    await renderPage();

    const addButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('添加成员'),
    ) as HTMLButtonElement;
    expect(addButton).toBeTruthy();

    await act(async () => {
      addButton.click();
    });

    const searchInput = Array.from(container.querySelectorAll('input')).find(
      input => input.placeholder === '搜索用户昵称、邮箱或用户名',
    ) as HTMLInputElement;
    expect(searchInput).toBeTruthy();

    await act(async () => {
      Simulate.change(searchInput, {
        target: {
          value: 'new',
        },
      } as never);
    });

    const searchButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '搜索',
    ) as HTMLButtonElement;

    await act(async () => {
      searchButton.click();
      await Promise.resolve();
    });

    const candidateButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button =>
      button.textContent?.includes('New User'),
    ) as HTMLButtonElement;
    expect(candidateButton).toBeTruthy();

    await act(async () => {
      candidateButton.click();
    });

    const confirmButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '确认添加',
    ) as HTMLButtonElement;

    await act(async () => {
      confirmButton.click();
      await Promise.resolve();
    });

    expect(mockAddWorkspaceMembers).toHaveBeenCalledWith({
      space_id: '101',
      members: [
        {
          user_id: '11',
          role_type: 3,
        },
      ],
    });
    expect(mockToastSuccess).toHaveBeenCalledWith({
      content: '成员添加成功',
    });
  });

  it('updates workspace setting switches from the space setting tab', async () => {
    await renderPage();

    const settingTab = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '空间设置',
    ) as HTMLButtonElement;
    expect(settingTab).toBeTruthy();

    await act(async () => {
      settingTab.click();
    });

    expect(container.textContent).toContain('开发者功能');
    expect(container.textContent).toContain('接受来自外部空间的发布');
    expect(container.textContent).toContain('开发配置');
    expect(container.textContent).toContain('资源配置');
    expect(container.textContent).toContain('技能配置');
    expect(container.textContent).not.toContain('智能体开发');
    expect(container.textContent).not.toContain('组件库');
    expect(container.textContent).not.toContain('广场');
    expect(
      container.querySelectorAll('.coze-prototype-space-setting-section')
        .length,
    ).toBeGreaterThanOrEqual(4);

    const allowDevelopSwitch = container.querySelector<HTMLInputElement>(
      'input[aria-label="开发者功能"]',
    );
    expect(allowDevelopSwitch).toBeTruthy();
    expect(allowDevelopSwitch?.checked).toBe(true);

    await act(async () => {
      Simulate.change(allowDevelopSwitch!, {
        target: {
          checked: false,
        },
      } as never);
      await Promise.resolve();
    });

    expect(mockUpdateWorkspace).toHaveBeenCalledWith({
      space_id: '101',
      allow_develop: false,
    });
    expect(mockToastSuccess).toHaveBeenCalledWith({
      content: '空间设置已更新',
    });
  });

  it('paginates workspace members like a management table', async () => {
    mockListWorkspaceMembers.mockResolvedValueOnce({
      members: Array.from({ length: 12 }, (_, index) => ({
        user_id: `${index + 1}`,
        name: `Member ${index + 1}`,
        user_unique_name: `member-${index + 1}`,
        email: `member-${index + 1}@example.test`,
        role_type: index === 0 ? 1 : 3,
        joined_at: 1717000000 + index,
      })),
    });

    await renderPage();

    expect(container.textContent).toContain('共 12 位成员');
    expect(container.textContent).toContain('第 1 / 2 页');
    expect(container.textContent).toContain('Member 10');
    expect(container.textContent).not.toContain('Member 11');

    const nextButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="下一页成员"]',
    );
    expect(nextButton).toBeTruthy();

    await act(async () => {
      Simulate.click(nextButton!);
    });

    expect(container.textContent).toContain('第 2 / 2 页');
    expect(container.textContent).toContain('Member 11');
    expect(container.textContent).toContain('Member 12');
    expect(container.textContent).not.toContain('Member 10');
  });

  it('shows an empty member state', async () => {
    mockListWorkspaceMembers.mockResolvedValueOnce({
      members: [],
    });

    await renderPage();

    expect(container.textContent).toContain('暂无成员');
  });

  it('hides team-only settings for personal workspace', async () => {
    mockGetWorkspaceDetail.mockResolvedValueOnce({
      data: {
        id: '101',
        name: 'Personal Space',
        description: 'This is your personal space',
        space_type: 1,
        owner_user_id: '9',
        current_user_role: 1,
        total_member_num: 1,
      },
    });

    await renderPage();

    expect(container.textContent).toContain('成员管理');
    expect(container.textContent).toContain('个人空间');
    expect(container.textContent).toContain(
      '个人空间暂不支持成员邀请、角色调整和移除操作',
    );
    expect(container.textContent).not.toContain('Personal Space');
    expect(container.textContent).not.toContain('This is your personal space');
    expect(container.textContent).not.toContain('空间设置');
    expect(container.textContent).not.toContain('添加成员');
  });

  it('shows an error state when workspace request fails', async () => {
    mockGetWorkspaceDetail.mockRejectedValueOnce(new Error('network failed'));

    await renderPage();

    expect(container.textContent).toContain('加载工作空间失败');
    expect(mockToastError).toHaveBeenCalledWith({
      content: '加载工作空间失败',
    });
  });
});
