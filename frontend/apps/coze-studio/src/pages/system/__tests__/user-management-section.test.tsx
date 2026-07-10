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

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { UserManagementSection } from '../user-management-section';
import type { AdminUser, AdminUserSpace } from '../service';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const users: AdminUser[] = [
  {
    user_id: '9',
    name: 'Owner',
    email: 'owner@example.test',
    user_unique_name: 'owner',
    created_at: 1710000000,
  },
];

const userSpaces: AdminUserSpace[] = [
  {
    id: '101',
    name: '畅享 AI',
    owner_user_id: '9',
    owner_name: 'Owner',
    role_type: 2,
    total_member_num: 3,
  },
  {
    id: '102',
    name: 'Personal Space',
    description: 'This is your personal space',
    owner_user_id: '10',
    owner_name: 'Member',
    role_type: 1,
    total_member_num: 1,
  },
];

describe('UserManagementSection', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onKeywordChange = vi.fn();
  const onOpenWorkspace = vi.fn();
  const onRefresh = vi.fn();
  const onCreateUser = vi.fn();
  const onUpdateUser = vi.fn();
  const onResetUserPassword = vi.fn();
  const onSearch = vi.fn();
  const onTurnPage = vi.fn();
  const onShowUserSpaces = vi.fn();

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
    vi.clearAllMocks();
  });

  const renderSection = (overrides = {}) => {
    act(() => {
      root.render(
        <UserManagementSection
          pageSize={20}
          selectedUser={users[0]}
          userKeyword="own"
          userPage={1}
          userSpaces={userSpaces}
          userSpacesError=""
          userSpacesLoading={false}
          userTotal={25}
          users={users}
          isLoading={false}
          onKeywordChange={onKeywordChange}
          onCreateUser={onCreateUser}
          onOpenWorkspace={onOpenWorkspace}
          onRefresh={onRefresh}
          onResetUserPassword={onResetUserPassword}
          onSearch={onSearch}
          onShowUserSpaces={onShowUserSpaces}
          onTurnPage={onTurnPage}
          onUpdateUser={onUpdateUser}
          {...overrides}
        />,
      );
    });
  };

  it('renders users and selected user spaces', () => {
    renderSection();

    expect(container.textContent).toContain('用户总览');
    expect(container.textContent).toContain('共 25 个用户');
    expect(container.textContent).toContain('本页 1 个账号');
    expect(container.textContent).toContain('已选 Owner');
    expect(container.textContent).toContain('团队 1 个 / 个人 1 个');
    expect(container.textContent).toContain('已展开 2 个空间');
    expect(container.textContent).toContain('筛选用户');
    expect(container.textContent).toContain('新增用户');
    expect(container.textContent).toContain('用户列表');
    expect(container.textContent).toContain('创建时间');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('owner@example.test');
    expect(container.textContent).toContain('所属空间详情');
    expect(container.textContent).toContain('畅享 AI');
    expect(container.textContent).toContain('团队空间');
    expect(container.textContent).toContain('个人空间');
    expect(container.textContent).not.toContain('Personal Space');
    expect(container.textContent).toContain('管理员');
    expect(container.textContent).toContain('进入空间');
    expect(container.textContent).toContain('编辑');
    expect(container.textContent).toContain('密码重置');
    expect(container.textContent).toContain('第 1 页，共 25 个用户。');
  });

  it('emits search and row actions', () => {
    renderSection();

    const input = container.querySelector(
      'input[aria-label="搜索用户"]',
    ) as HTMLInputElement;
    const searchButton = container.querySelector(
      'button[aria-label="执行用户搜索"]',
    ) as HTMLButtonElement;
    const refreshButton = container.querySelector(
      'button[aria-label="刷新用户列表"]',
    ) as HTMLButtonElement;
    const viewButton = container.querySelector(
      'button[aria-label="查看用户空间-9"]',
    ) as HTMLButtonElement;
    const openButton = container.querySelector(
      'button[aria-label="进入用户所属空间-101"]',
    ) as HTMLButtonElement;
    const nextButton = container.querySelector(
      'button[aria-label="下一页用户"]',
    ) as HTMLButtonElement;

    act(() => {
      Simulate.change(input, {
        target: {
          value: 'owner@example.test',
        },
      } as unknown as Event);
      Simulate.click(searchButton);
      Simulate.click(refreshButton);
      Simulate.click(viewButton);
      Simulate.click(openButton);
      Simulate.click(nextButton);
    });

    expect(onKeywordChange).toHaveBeenCalledWith('owner@example.test');
    expect(onSearch).toHaveBeenCalled();
    expect(onRefresh).toHaveBeenCalled();
    expect(onShowUserSpaces).toHaveBeenCalledWith(users[0]);
    expect(onOpenWorkspace).toHaveBeenCalledWith('101');
    expect(onTurnPage).toHaveBeenCalledWith(2);
  });

  it('submits add, edit and reset password actions', async () => {
    onCreateUser.mockResolvedValue(undefined);
    onUpdateUser.mockResolvedValue(undefined);
    onResetUserPassword.mockResolvedValue(undefined);
    renderSection();

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="新增用户"]',
        )!,
      );
    });

    await act(async () => {
      const emailInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="新增用户邮箱"]',
      )!;
      emailInput.value = 'new@example.test';
      Simulate.change(emailInput);
      const passwordInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="新增用户密码"]',
      )!;
      passwordInput.value = 'secret1';
      Simulate.change(passwordInput);
      const nameInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="新增用户昵称"]',
      )!;
      nameInput.value = 'New User';
      Simulate.change(nameInput);
      const uniqueNameInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="新增用户名"]',
      )!;
      uniqueNameInput.value = 'new-user';
      Simulate.change(uniqueNameInput);
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="提交新增用户"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(onCreateUser).toHaveBeenCalledWith({
      email: 'new@example.test',
      locale: 'zh-CN',
      name: 'New User',
      password: 'secret1',
      user_unique_name: 'new-user',
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="编辑用户-9"]',
        )!,
      );
    });

    await act(async () => {
      const nameInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="编辑用户昵称"]',
      )!;
      nameInput.value = 'Owner Edited';
      Simulate.change(nameInput);
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="提交编辑用户"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(onUpdateUser).toHaveBeenCalledWith({
      locale: 'zh-CN',
      name: 'Owner Edited',
      user_id: '9',
      user_unique_name: 'owner',
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="重置用户密码-9"]',
        )!,
      );
    });

    await act(async () => {
      const passwordInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="重置用户密码"]',
      )!;
      passwordInput.value = 'secret2';
      Simulate.change(passwordInput);
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="提交重置密码"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(onResetUserPassword).toHaveBeenCalledWith({
      password: 'secret2',
      user_id: '9',
    });
  });

  it('disables user list actions while loading', () => {
    renderSection({
      isLoading: true,
      selectedUser: null,
      userSpaces: [],
    });

    expect(container.textContent).toContain('正在加载用户...');
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="执行用户搜索"]',
      )?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="刷新用户列表"]',
      )?.disabled,
    ).toBe(true);
  });
});
