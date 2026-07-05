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

import { WorkspaceManagementSection } from '../workspace-management-section';
import type { AdminWorkspace, AdminWorkspaceMember } from '../service';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const workspaces: AdminWorkspace[] = [
  {
    id: '101',
    name: '畅享 AI',
    description: '团队协作空间',
    owner_user_id: '9',
    owner_name: 'Owner',
    total_member_num: 3,
    created_at: 1710000000,
  },
];

const workspaceMembers: AdminWorkspaceMember[] = [
  {
    user_id: '9',
    name: 'Owner',
    email: 'owner@example.test',
    user_unique_name: 'owner',
    role_type: 2,
    joined_at: 1710000000,
  },
];

describe('WorkspaceManagementSection', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onKeywordChange = vi.fn();
  const onSearch = vi.fn();
  const onShowWorkspaceMembers = vi.fn();
  const onTurnPage = vi.fn();

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
        <WorkspaceManagementSection
          pageSize={20}
          selectedWorkspace={workspaces[0]}
          workspaceKeyword="畅享"
          workspaceMembers={workspaceMembers}
          workspaceMembersError=""
          workspaceMembersLoading={false}
          workspacePage={1}
          workspaceTotal={25}
          workspaces={workspaces}
          onKeywordChange={onKeywordChange}
          onSearch={onSearch}
          onShowWorkspaceMembers={onShowWorkspaceMembers}
          onTurnPage={onTurnPage}
          {...overrides}
        />,
      );
    });
  };

  it('renders workspaces and selected workspace members', () => {
    renderSection();

    expect(container.textContent).toContain('工作空间总览');
    expect(container.textContent).toContain('共 25 个空间');
    expect(container.textContent).toContain('本页 1 个空间');
    expect(container.textContent).toContain('已选 畅享 AI');
    expect(container.textContent).toContain('已展开 1 位成员');
    expect(container.textContent).toContain('筛选工作空间');
    expect(container.textContent).toContain('工作空间列表');
    expect(container.textContent).toContain('创建时间');
    expect(container.textContent).toContain('畅享 AI');
    expect(container.textContent).toContain('团队协作空间');
    expect(container.textContent).toContain('成员详情');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('管理员');
    expect(container.textContent).toContain('第 1 页，共 25 个工作空间。');
  });

  it('emits search and row actions', () => {
    renderSection({
      selectedWorkspace: null,
      workspaceMembers: [],
    });

    const input = container.querySelector(
      'input[aria-label="搜索工作空间"]',
    ) as HTMLInputElement;
    const searchButton = container.querySelector(
      'button[aria-label="执行工作空间搜索"]',
    ) as HTMLButtonElement;
    const viewButton = container.querySelector(
      'button[aria-label="查看工作空间成员-101"]',
    ) as HTMLButtonElement;
    const nextButton = container.querySelector(
      'button[aria-label="下一页工作空间"]',
    ) as HTMLButtonElement;

    act(() => {
      Simulate.change(input, {
        target: {
          value: '团队协作',
        },
      } as unknown as Event);
      Simulate.click(searchButton);
      Simulate.click(viewButton);
      Simulate.click(nextButton);
    });

    expect(onKeywordChange).toHaveBeenCalledWith('团队协作');
    expect(onSearch).toHaveBeenCalled();
    expect(onShowWorkspaceMembers).toHaveBeenCalledWith(workspaces[0]);
    expect(onTurnPage).toHaveBeenCalledWith(2);
  });
});
