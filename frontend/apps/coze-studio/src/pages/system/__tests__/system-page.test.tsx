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

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockNavigate = vi.hoisted(() => vi.fn());
const mockUseParams = vi.hoisted(() => vi.fn(() => ({ section: 'overview' })));
const mockListAdminWorkspaces = vi.hoisted(() => vi.fn());
const mockListAdminUsers = vi.hoisted(() => vi.fn());
const mockListAdminUserSpaces = vi.hoisted(() => vi.fn());
const mockListAdminWorkspaceMembers = vi.hoisted(() => vi.fn());
const mockGetAdminBasicConfig = vi.hoisted(() => vi.fn());
const mockGetAdminModelList = vi.hoisted(() => vi.fn());
const mockGetAdminKnowledgeConfig = vi.hoisted(() => vi.fn());
const mockGetSystemAdminStatus = vi.hoisted(() => vi.fn());
const mockCreateAdminModel = vi.hoisted(() => vi.fn());
const mockCreateAdminUser = vi.hoisted(() => vi.fn());
const mockDeleteAdminModel = vi.hoisted(() => vi.fn());
const mockSaveAdminBasicConfig = vi.hoisted(() => vi.fn());
const mockResetAdminUserPassword = vi.hoisted(() => vi.fn());
const mockUpdateAdminUser = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  createAdminModel: mockCreateAdminModel,
  createAdminUser: mockCreateAdminUser,
  deleteAdminModel: mockDeleteAdminModel,
  getAdminBasicConfig: mockGetAdminBasicConfig,
  getAdminKnowledgeConfig: mockGetAdminKnowledgeConfig,
  getAdminModelList: mockGetAdminModelList,
  getSystemAdminStatus: mockGetSystemAdminStatus,
  listAdminUserSpaces: mockListAdminUserSpaces,
  listAdminWorkspaceMembers: mockListAdminWorkspaceMembers,
  listAdminUsers: mockListAdminUsers,
  listAdminWorkspaces: mockListAdminWorkspaces,
  resetAdminUserPassword: mockResetAdminUserPassword,
  saveAdminBasicConfig: mockSaveAdminBasicConfig,
  updateAdminUser: mockUpdateAdminUser,
}));

import SystemManagementPage from '../index';

describe('SystemManagementPage', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    mockUseParams.mockReturnValue({ section: 'overview' });
    mockGetSystemAdminStatus.mockResolvedValue({
      is_admin: true,
    });
    mockListAdminWorkspaces.mockResolvedValue({
      workspaces: [
        {
          id: '101',
          name: '畅享 AI',
          owner_name: 'Owner',
          total_member_num: 2,
          created_at: 1717000000,
        },
      ],
      total: 1,
    });
    mockListAdminUsers.mockResolvedValue({
      users: [
        {
          user_id: '9',
          name: 'Owner',
          email: 'owner@example.test',
          user_unique_name: 'owner',
          created_at: 1717000000,
        },
      ],
      total: 1,
    });
    mockListAdminWorkspaceMembers.mockResolvedValue({
      members: [
        {
          user_id: '9',
          name: 'Owner',
          email: 'owner@example.test',
          user_unique_name: 'owner',
          role_type: 2,
        },
      ],
    });
    mockListAdminUserSpaces.mockResolvedValue({
      spaces: [
        {
          id: '101',
          name: '畅享 AI',
          description: '团队协作空间',
          owner_name: 'Owner',
          role_type: 2,
          total_member_num: 3,
        },
      ],
    });
    mockGetAdminBasicConfig.mockResolvedValue({
      configuration: {
        admin_emails: 'owner@example.test',
        allow_registration_email: 'example.test',
        code_runner_type: 1,
        disable_user_registration: true,
        plugin_configuration: {
          mode: 'kept',
        },
        server_host: 'http://localhost:8888',
      },
    });
    mockGetAdminModelList.mockResolvedValue({
      provider_model_list: [
        {
          provider: {
            model_class: 1,
            name: {
              zh_cn: 'OpenAI 模型',
              en_us: 'OpenAI Model',
            },
          },
          model_list: [
            {
              id: 1,
              display_info: {
                name: 'gpt-4o',
              },
            },
          ],
        },
      ],
    });
    mockGetAdminKnowledgeConfig.mockResolvedValue({
      knowledge_config: {
        builtin_model_id: 42,
        embedding_config: {
          type: 1,
        },
      },
    });
    mockSaveAdminBasicConfig.mockResolvedValue({});
    mockCreateAdminUser.mockResolvedValue({
      user: {
        user_id: '99',
      },
    });
    mockUpdateAdminUser.mockResolvedValue({
      msg: 'success',
    });
    mockResetAdminUserPassword.mockResolvedValue({
      msg: 'success',
    });
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
      root.render(<SystemManagementPage />);
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  it('renders overview totals from system management APIs', async () => {
    await renderPage();

    expect(mockGetSystemAdminStatus).toHaveBeenCalled();
    expect(mockListAdminWorkspaces).toHaveBeenCalledWith({
      page: 1,
      size: 20,
    });
    expect(mockListAdminUsers).toHaveBeenCalledWith({
      page: 1,
      size: 20,
    });
    expect(mockGetAdminBasicConfig).toHaveBeenCalled();
    expect(container.textContent).toContain('用户总数');
    expect(container.textContent).toContain('工作空间总数');
    expect(container.textContent).toContain('owner@example.test');
    expect(container.textContent).toContain('最近用户');
    expect(container.textContent).toContain('最近工作空间');
    expect(container.textContent).toContain('畅享 AI');
  });

  it('blocks direct system access for non-admin users', async () => {
    mockGetSystemAdminStatus.mockResolvedValue({
      is_admin: false,
    });

    await renderPage();

    expect(container.textContent).toContain('无权访问系统管理');
    expect(container.textContent).toContain('请联系系统管理员开通后台管理权限');
    expect(mockListAdminWorkspaces).not.toHaveBeenCalled();
    expect(mockListAdminUsers).not.toHaveBeenCalled();
    expect(mockGetAdminBasicConfig).not.toHaveBeenCalled();
    expect(mockGetAdminModelList).not.toHaveBeenCalled();
    expect(mockGetAdminKnowledgeConfig).not.toHaveBeenCalled();
  });

  it('renders users section rows', async () => {
    mockUseParams.mockReturnValue({ section: 'users' });

    await renderPage();

    expect(container.textContent).toContain('用户管理');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('owner@example.test');
  });

  it('searches and paginates users', async () => {
    mockUseParams.mockReturnValue({ section: 'users' });
    mockListAdminUsers.mockResolvedValue({
      users: [
        {
          user_id: '9',
          name: 'Owner',
          email: 'owner@example.test',
          user_unique_name: 'owner',
        },
      ],
      total: 21,
    });

    await renderPage();

    const searchInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="搜索用户"]',
    );
    expect(searchInput).not.toBeNull();

    await act(async () => {
      Simulate.change(searchInput!, {
        target: {
          value: 'owner',
        },
      } as never);
    });

    const searchButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="执行用户搜索"]',
    );
    expect(searchButton).not.toBeNull();

    await act(async () => {
      Simulate.click(searchButton!);
    });

    expect(mockListAdminUsers).toHaveBeenLastCalledWith({
      keyword: 'owner',
      page: 1,
      size: 20,
    });

    const nextButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="下一页用户"]',
    );
    expect(nextButton).not.toBeNull();

    await act(async () => {
      Simulate.click(nextButton!);
    });

    expect(mockListAdminUsers).toHaveBeenLastCalledWith({
      keyword: 'owner',
      page: 2,
      size: 20,
    });

    const refreshButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="刷新用户列表"]',
    );
    expect(refreshButton).not.toBeNull();

    await act(async () => {
      Simulate.click(refreshButton!);
    });

    expect(mockListAdminUsers).toHaveBeenLastCalledWith({
      keyword: 'owner',
      page: 2,
      size: 20,
    });
  });

  it('loads user workspaces from the users section', async () => {
    mockUseParams.mockReturnValue({ section: 'users' });

    await renderPage();

    const spacesButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="查看用户空间-9"]',
    );
    expect(spacesButton).not.toBeNull();

    await act(async () => {
      Simulate.click(spacesButton!);
    });

    expect(mockListAdminUserSpaces).toHaveBeenCalledWith({
      user_id: '9',
    });
    expect(container.textContent).toContain('所属空间详情');
    expect(container.textContent).toContain('畅享 AI');
    expect(container.textContent).toContain('管理员');

    const openSpaceButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="进入用户所属空间-101"]',
    );
    expect(openSpaceButton).not.toBeNull();

    await act(async () => {
      Simulate.click(openSpaceButton!);
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/101/workspace');
  });

  it('creates, updates and resets users from the users section', async () => {
    mockUseParams.mockReturnValue({ section: 'users' });

    await renderPage();

    const addButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="新增用户"]',
    );
    expect(addButton).not.toBeNull();

    await act(async () => {
      Simulate.click(addButton!);
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
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="提交新增用户"]',
        )!,
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockCreateAdminUser).toHaveBeenCalledWith({
      email: 'new@example.test',
      locale: 'zh-CN',
      name: 'New User',
      password: 'secret1',
      user_unique_name: '',
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
      await Promise.resolve();
    });

    expect(mockUpdateAdminUser).toHaveBeenCalledWith({
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

    expect(mockResetAdminUserPassword).toHaveBeenCalledWith({
      password: 'secret2',
      user_id: '9',
    });
  });

  it('renders workspaces section rows', async () => {
    mockUseParams.mockReturnValue({ section: 'workspaces' });

    await renderPage();

    expect(container.textContent).toContain('工作空间管理');
    expect(container.textContent).toContain('畅享 AI');
    expect(container.textContent).toContain('Owner');
  });

  it('loads workspace members from the workspace section', async () => {
    mockUseParams.mockReturnValue({ section: 'workspaces' });

    await renderPage();

    const membersButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="查看工作空间成员-101"]',
    );
    expect(membersButton).not.toBeNull();

    await act(async () => {
      Simulate.click(membersButton!);
    });

    expect(mockListAdminWorkspaceMembers).toHaveBeenCalledWith({
      space_id: '101',
    });
    expect(container.textContent).toContain('成员详情');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('owner@example.test');
    expect(container.textContent).toContain('管理员');

    const openSpaceButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="进入工作空间-101"]',
    );
    expect(openSpaceButton).not.toBeNull();

    await act(async () => {
      Simulate.click(openSpaceButton!);
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/101/workspace');
  });

  it('searches and paginates workspaces', async () => {
    mockUseParams.mockReturnValue({ section: 'workspaces' });
    mockListAdminWorkspaces.mockResolvedValue({
      workspaces: [
        {
          id: '101',
          name: '畅享 AI',
          owner_name: 'Owner',
          total_member_num: 2,
        },
      ],
      total: 21,
    });

    await renderPage();

    const searchInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="搜索工作空间"]',
    );
    expect(searchInput).not.toBeNull();

    await act(async () => {
      Simulate.change(searchInput!, {
        target: {
          value: '畅享',
        },
      } as never);
    });

    const searchButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="执行工作空间搜索"]',
    );
    expect(searchButton).not.toBeNull();

    await act(async () => {
      Simulate.click(searchButton!);
    });

    expect(mockListAdminWorkspaces).toHaveBeenLastCalledWith({
      keyword: '畅享',
      page: 1,
      size: 20,
    });

    const nextButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="下一页工作空间"]',
    );
    expect(nextButton).not.toBeNull();

    await act(async () => {
      Simulate.click(nextButton!);
    });

    expect(mockListAdminWorkspaces).toHaveBeenLastCalledWith({
      keyword: '畅享',
      page: 2,
      size: 20,
    });

    const refreshButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="刷新工作空间列表"]',
    );
    expect(refreshButton).not.toBeNull();

    await act(async () => {
      Simulate.click(refreshButton!);
    });

    expect(mockListAdminWorkspaces).toHaveBeenLastCalledWith({
      keyword: '畅享',
      page: 2,
      size: 20,
    });
  });

  it('renders settings section from basic config', async () => {
    mockUseParams.mockReturnValue({ section: 'settings' });

    await renderPage();

    expect(container.textContent).toContain('系统配置');
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="系统服务地址"]',
      )?.value,
    ).toBe('http://localhost:8888');
    expect(container.textContent).toContain('已关闭');
    expect(container.textContent).toContain('基础配置');
    expect(container.textContent).toContain('保存基础配置');
    expect(container.textContent).toContain('知识库配置');
    expect(container.textContent).toContain('内置模型 ID');
    expect(container.textContent).toContain('42');
  });

  it('saves basic config from settings section without dropping hidden fields', async () => {
    mockUseParams.mockReturnValue({ section: 'settings' });

    await renderPage();

    const serverHostInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统服务地址"]',
    );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存系统基础配置"]',
    );

    await act(async () => {
      serverHostInput!.value = 'https://agent.example.test';
      Simulate.change(serverHostInput!);
    });

    await act(async () => {
      Simulate.click(saveButton!);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockSaveAdminBasicConfig).toHaveBeenCalledWith({
      admin_emails: 'owner@example.test',
      allow_registration_email: 'example.test',
      code_runner_type: 1,
      disable_user_registration: true,
      plugin_configuration: {
        mode: 'kept',
      },
      server_host: 'https://agent.example.test',
    });
    expect(container.textContent).toContain('系统基础配置已保存');
  });

  it('renders model config section from model APIs', async () => {
    mockUseParams.mockReturnValue({ section: 'models' });

    await renderPage();

    expect(container.textContent).toContain('模型配置');
    expect(container.textContent).toContain('OpenAI 模型');
    expect(container.textContent).toContain('gpt-4o');
    expect(container.textContent).toContain('新增模型配置');
  });
});
