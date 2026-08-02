/* Copyright 2025 coze-dev Authors */

/* eslint-disable @typescript-eslint/naming-convention, @typescript-eslint/require-await -- Component and request mocks preserve imported contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import type * as SystemService from '../service';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockNavigate = vi.hoisted(() => vi.fn());
const mockAdminStatus = vi.hoisted(() => vi.fn());
const sandboxRender = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ section: 'sandbox' }),
}));

vi.mock('@coze-foundation/global-adapter', () => ({
  refreshSiteConfig: vi.fn(),
}));

vi.mock('../sandbox-management-section', () => ({
  SandboxManagementSection: () => {
    sandboxRender();
    return <div data-testid="sandbox-management">Sandbox 管理内容</div>;
  },
}));

vi.mock('../service', async importOriginal => {
  const actual = await importOriginal<typeof SystemService>();
  return {
    ...actual,
    createAdminModel: vi.fn(),
    createAdminUser: vi.fn(),
    deleteAdminModel: vi.fn(),
    getAdminBasicConfig: vi.fn().mockResolvedValue({
      revision: 'rev-1',
      configuration: {},
    }),
    getAdminKnowledgeConfig: vi
      .fn()
      .mockResolvedValue({ knowledge_config: {} }),
    getAdminModelList: vi.fn().mockResolvedValue({ provider_model_list: [] }),
    getSystemAdminStatus: mockAdminStatus,
    isAdminBasicConfigConflict: vi.fn().mockReturnValue(false),
    listAdminUserSpaces: vi.fn(),
    listAdminUsers: vi.fn().mockResolvedValue({ users: [], total: 0 }),
    listAdminWorkspaceMembers: vi.fn(),
    listAdminWorkspaces: vi
      .fn()
      .mockResolvedValue({ workspaces: [], total: 0 }),
    resetAdminUserPassword: vi.fn(),
    saveAdminBasicConfig: vi.fn(),
    updateAdminUser: vi.fn(),
  };
});

import SystemManagementPage from '../index';

describe('/system/sandbox', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  const render = async () => {
    await act(async () => {
      root.render(<SystemManagementPage />);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  it('renders the real secondary page only after the existing admin guard passes', async () => {
    mockAdminStatus.mockResolvedValue({ is_admin: true });
    await render();

    expect(container.textContent).toContain('沙箱管理');
    expect(
      container.querySelector('[data-testid="sandbox-management"]'),
    ).not.toBeNull();
    expect(sandboxRender).toHaveBeenCalledTimes(1);
  });

  it('does not expose the menu or page to a non-admin', async () => {
    mockAdminStatus.mockResolvedValue({ is_admin: false });
    await render();

    expect(container.textContent).toContain('无权访问系统管理');
    expect(container.textContent).not.toContain('沙箱管理');
    expect(
      container.querySelector('[data-testid="sandbox-management"]'),
    ).toBeNull();
    expect(sandboxRender).not.toHaveBeenCalled();
  });

  it('does not flash sandbox content while administrator status is unresolved', async () => {
    mockAdminStatus.mockReturnValue(new Promise(() => undefined));

    act(() => {
      root.render(<SystemManagementPage />);
    });

    expect(container.textContent).not.toContain('沙箱管理');
    expect(container.textContent).toContain('正在验证系统管理权限');
    expect(sandboxRender).not.toHaveBeenCalled();
  });
});
