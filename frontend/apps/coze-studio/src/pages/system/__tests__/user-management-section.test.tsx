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
];

describe('UserManagementSection', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onKeywordChange = vi.fn();
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
          onKeywordChange={onKeywordChange}
          onSearch={onSearch}
          onShowUserSpaces={onShowUserSpaces}
          onTurnPage={onTurnPage}
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
    expect(container.textContent).toContain('已展开 1 个空间');
    expect(container.textContent).toContain('筛选用户');
    expect(container.textContent).toContain('用户列表');
    expect(container.textContent).toContain('创建时间');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('owner@example.test');
    expect(container.textContent).toContain('所属空间详情');
    expect(container.textContent).toContain('畅享 AI');
    expect(container.textContent).toContain('管理员');
    expect(container.textContent).toContain('第 1 页，共 25 个用户。');
  });

  it('emits search and row actions', () => {
    renderSection({
      selectedUser: null,
      userSpaces: [],
    });

    const input = container.querySelector(
      'input[aria-label="搜索用户"]',
    ) as HTMLInputElement;
    const searchButton = container.querySelector(
      'button[aria-label="执行用户搜索"]',
    ) as HTMLButtonElement;
    const viewButton = container.querySelector(
      'button[aria-label="查看用户空间-9"]',
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
      Simulate.click(viewButton);
      Simulate.click(nextButton);
    });

    expect(onKeywordChange).toHaveBeenCalledWith('owner@example.test');
    expect(onSearch).toHaveBeenCalled();
    expect(onShowUserSpaces).toHaveBeenCalledWith(users[0]);
    expect(onTurnPage).toHaveBeenCalledWith(2);
  });
});
