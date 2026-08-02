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
const mockListObjectStorageConfigs = vi.hoisted(() => vi.fn());
const mockCreateObjectStorageConfig = vi.hoisted(() => vi.fn());
const mockUpdateObjectStorageConfig = vi.hoisted(() => vi.fn());
const mockTestObjectStorageConfig = vi.hoisted(() => vi.fn());
const mockActivateObjectStorageConfig = vi.hoisted(() => vi.fn());
const mockDeleteObjectStorageConfig = vi.hoisted(() => vi.fn());
const mockSaveAdminBasicConfig = vi.hoisted(() => vi.fn());
const mockIsAdminBasicConfigConflict = vi.hoisted(() => vi.fn());
const mockResetAdminUserPassword = vi.hoisted(() => vi.fn());
const mockUpdateAdminUser = vi.hoisted(() => vi.fn());
const mockObjectStorageProviderType = vi.hoisted(() => ({
  ALIYUN_OSS: 2,
  AWS_S3: 5,
  HUAWEI_OBS: 4,
  MINIO: 6,
  QINIU: 1,
  TENCENT_COS: 3,
  TOS: 7,
}));
const mockObjectStorageHealthStatus = vi.hoisted(() => ({
  HEALTHY: 2,
  UNKNOWN: 1,
  UNHEALTHY: 3,
}));
const mockObjectStorageRuntimeSource = vi.hoisted(() => ({
  DATABASE: 1,
  ENV_RESCUE: 2,
}));
const mockAdminAnnouncementRouteType = vi.hoisted(() => ({
  None: 0,
  SystemAnnouncements: 2,
  WorkspaceHome: 1,
}));
const MockAdminAPIError = vi.hoisted(
  () =>
    class AdminAPIError extends Error {
      readonly status: number;
      readonly errorCode?: string;

      constructor(status: number, message: string, errorCode?: string) {
        super(message);
        this.status = status;
        this.errorCode = errorCode;
      }
    },
);

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('@coze-foundation/global-adapter', () => ({
  refreshSiteConfig: vi.fn(),
}));

vi.mock('../service', () => ({
  AdminAPIError: MockAdminAPIError,
  AdminAnnouncementRouteType: mockAdminAnnouncementRouteType,
  ObjectStorageHealthStatus: mockObjectStorageHealthStatus,
  ObjectStorageProviderType: mockObjectStorageProviderType,
  ObjectStorageRuntimeSource: mockObjectStorageRuntimeSource,
  activateObjectStorageConfig: mockActivateObjectStorageConfig,
  cancelAdminAnnouncement: vi.fn(),
  createAdminManagedModel: vi.fn(),
  createAdminModel: mockCreateAdminModel,
  createAdminAnnouncement: vi.fn(),
  createAdminUser: mockCreateAdminUser,
  createObjectStorageConfig: mockCreateObjectStorageConfig,
  deleteAdminModel: mockDeleteAdminModel,
  deleteObjectStorageConfig: mockDeleteObjectStorageConfig,
  getAdminBasicConfig: mockGetAdminBasicConfig,
  getAdminKnowledgeConfig: mockGetAdminKnowledgeConfig,
  getAdminManagedModelDetail: vi.fn(),
  getAdminManagedModelGrants: vi.fn().mockResolvedValue({ grants: [] }),
  getAdminModelList: mockGetAdminModelList,
  getSystemAdminStatus: mockGetSystemAdminStatus,
  isAdminBasicConfigConflict: mockIsAdminBasicConfigConflict,
  listAdminManagedModels: vi.fn().mockResolvedValue({
    models: [
      {
        access_mode: 2,
        capability_types: ['text', 'reasoning'],
        creator_id: '9',
        enabled: true,
        id: '12',
        model_class: 3,
        model_identifier: 'deepseek-v4-pro',
        name: 'DeepSeek V4 Pro',
        provider_key: 'deepseek',
        sort_order: 1,
        updated_at_ms: 1784707200000,
      },
    ],
    total: 1,
  }),
  listAdminModelProviders: vi.fn().mockResolvedValue({
    providers: [
      {
        default_base_url: 'https://api.deepseek.com/v1',
        model_class: 3,
        name: { zh_cn: 'Deepseek 模型' },
        protocol: 'openai-compatible',
        provider_key: 'deepseek',
        supports_custom_base_url: true,
        supports_function_call: true,
        supports_multimodal: false,
      },
    ],
  }),
  listAdminAnnouncementAuditEvents: vi.fn(),
  listAdminAnnouncements: vi.fn(),
  listObjectStorageConfigs: mockListObjectStorageConfigs,
  listAdminUserSpaces: mockListAdminUserSpaces,
  listAdminWorkspaceMembers: mockListAdminWorkspaceMembers,
  listAdminUsers: mockListAdminUsers,
  listAdminWorkspaces: mockListAdminWorkspaces,
  resetAdminUserPassword: mockResetAdminUserPassword,
  publishAdminAnnouncement: vi.fn(),
  replayAdminAnnouncements: vi.fn(),
  scheduleAdminAnnouncement: vi.fn(),
  saveAdminBasicConfig: mockSaveAdminBasicConfig,
  saveAdminManagedModelGrants: vi.fn(),
  sortAdminManagedModels: vi.fn(),
  testAdminManagedModelEndpoint: vi.fn(),
  testObjectStorageConfig: mockTestObjectStorageConfig,
  updateAdminManagedModel: vi.fn(),
  updateAdminManagedModelStatus: vi.fn(),
  updateAdminAnnouncement: vi.fn(),
  updateAdminUser: mockUpdateAdminUser,
  updateObjectStorageConfig: mockUpdateObjectStorageConfig,
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
      revision: 'rev-7',
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
    mockListObjectStorageConfigs.mockResolvedValue({
      configs: [
        {
          id: '7',
          name: '七牛主存储',
          provider_type: mockObjectStorageProviderType.QINIU,
          config: {
            bucket: 'coze-assets',
            download_domain: 'assets.example.test',
            region: 'z0',
          },
          credential_configured: true,
          health: {
            status: mockObjectStorageHealthStatus.HEALTHY,
          },
          desired_active: false,
          runtime_active: false,
          restart_required: false,
          version: '3',
          runtime_revision: '',
          created_at: '2026-07-28T10:00:00Z',
          updated_at: '2026-07-28T10:10:00Z',
        },
      ],
      runtime_source: mockObjectStorageRuntimeSource.DATABASE,
      restart_required: false,
      code: 0,
      msg: '',
    });
    mockCreateObjectStorageConfig.mockResolvedValue({
      config: {
        id: '8',
      },
      code: 0,
      msg: '',
    });
    mockUpdateObjectStorageConfig.mockResolvedValue({
      config: {
        id: '7',
      },
      code: 0,
      msg: '',
    });
    mockTestObjectStorageConfig.mockResolvedValue({
      success: true,
      health: {
        status: mockObjectStorageHealthStatus.HEALTHY,
        latency_ms: 18,
      },
      code: 0,
      msg: '',
    });
    mockActivateObjectStorageConfig.mockResolvedValue({
      config: {
        id: '7',
      },
      code: 0,
      msg: '',
    });
    mockDeleteObjectStorageConfig.mockResolvedValue({
      code: 0,
      msg: '',
    });
    mockSaveAdminBasicConfig.mockResolvedValue({ revision: 'rev-8' });
    mockIsAdminBasicConfigConflict.mockImplementation(
      error => error?.errorCode === 'BASE_CONFIG_VERSION_CONFLICT',
    );
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
        'input[aria-label="站点访问地址"]',
      )?.value,
    ).toBe('http://localhost:8888');
    expect(container.textContent).toContain('已关闭');
    expect(container.textContent).toContain('基础配置');
    expect(container.textContent).toContain('保存基础配置');
    expect(container.textContent).toContain('知识库配置');
    expect(container.textContent).toContain('内置模型 ID');
    expect(container.textContent).toContain('42');
  });

  it('saves only changed basic config fields with its revision', async () => {
    mockUseParams.mockReturnValue({ section: 'settings' });

    await renderPage();

    const adminEmailsInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统管理员邮箱"]',
    );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存系统基础配置"]',
    );

    await act(async () => {
      adminEmailsInput!.value = 'owner@example.test,ops@example.test';
      Simulate.change(adminEmailsInput!);
    });

    await act(async () => {
      Simulate.click(saveButton!);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockSaveAdminBasicConfig).toHaveBeenCalledWith(
      {
        admin_emails: 'owner@example.test,ops@example.test',
      },
      'rev-7',
    );
    expect(container.textContent).toContain('系统基础配置已保存');
  });

  it('shows a refresh action after a concurrent basic config conflict', async () => {
    mockUseParams.mockReturnValue({ section: 'settings' });
    mockSaveAdminBasicConfig.mockRejectedValue({
      errorCode: 'BASE_CONFIG_VERSION_CONFLICT',
    });
    await renderPage();

    const adminEmailsInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统管理员邮箱"]',
    )!;
    await act(async () => {
      adminEmailsInput.value = 'owner@example.test,ops@example.test';
      Simulate.change(adminEmailsInput);
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存系统基础配置"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(container.textContent).toContain(
      '配置已被其他管理员更新，请刷新后重试',
    );
    expect(
      container.querySelector('button[aria-label="刷新系统基础配置"]'),
    ).not.toBeNull();
  });

  it('renders model config section from model APIs', async () => {
    mockUseParams.mockReturnValue({ section: 'models' });

    await renderPage();

    expect(container.textContent).toContain('模型配置');
    expect(container.textContent).toContain('DeepSeek V4 Pro');
    expect(container.textContent).toContain('deepseek-v4-pro');
    expect(container.textContent).toContain('添加模型');
  });

  it('guides qiniu download domains without URL schemes', async () => {
    mockUseParams.mockReturnValue({ section: 'object-storage' });

    await renderPage();

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="新增对象存储配置"]',
        )!,
      );
    });

    const downloadDomainInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="对象存储下载域名"]',
    );
    expect(downloadDomainInput?.placeholder).toBe('assets.example.com');
    expect(downloadDomainInput?.placeholder).not.toContain('://');
  });

  it('creates and activates object storage configs from the system section', async () => {
    mockUseParams.mockReturnValue({ section: 'object-storage' });

    await renderPage();
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListObjectStorageConfigs).toHaveBeenCalled();
    expect(container.textContent).toContain('对象存储');
    expect(container.textContent).toContain('七牛主存储');
    expect(container.textContent).toContain('新增配置');

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="新增对象存储配置"]',
        )!,
      );
    });

    await act(async () => {
      const providerSelect = container.querySelector<HTMLSelectElement>(
        'select[aria-label="对象存储云厂商"]',
      )!;
      providerSelect.value = String(mockObjectStorageProviderType.MINIO);
      Simulate.change(providerSelect);

      const nameInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="对象存储配置名称"]',
      )!;
      nameInput.value = 'MinIO 备用';
      Simulate.change(nameInput);

      const bucketInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="对象存储 Bucket"]',
      )!;
      bucketInput.value = 'coze';
      Simulate.change(bucketInput);

      const endpointInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="对象存储 Endpoint"]',
      )!;
      endpointInput.value = 'http://minio:9000';
      Simulate.change(endpointInput);

      const accessKeyInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="对象存储 Access Key ID"]',
      )!;
      accessKeyInput.value = 'minio-ak';
      Simulate.change(accessKeyInput);

      const secretInput = container.querySelector<HTMLInputElement>(
        'input[aria-label="对象存储 Secret Access Key"]',
      )!;
      secretInput.value = 'minio-sk';
      Simulate.change(secretInput);
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存对象存储配置"]',
        )!,
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockCreateObjectStorageConfig).toHaveBeenCalledWith(
      expect.objectContaining({
        credential: {
          access_key_id: 'minio-ak',
          secret_access_key: 'minio-sk',
        },
        name: 'MinIO 备用',
        provider_type: mockObjectStorageProviderType.MINIO,
      }),
    );
    expect(
      mockCreateObjectStorageConfig.mock.calls[0]?.[0].config,
    ).toMatchObject({
      bucket: 'coze',
      endpoint: 'http://minio:9000',
    });

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="激活对象存储配置-7"]',
        )!,
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockActivateObjectStorageConfig).toHaveBeenCalledWith({
      expected_version: '3',
      id: '7',
      migration_confirmed: false,
    });
  });
});
