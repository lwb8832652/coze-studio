/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

import {
  Children,
  cloneElement,
  isValidElement,
  type ReactElement,
  type ReactNode,
} from 'react';

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockList = vi.hoisted(() => vi.fn());
const mockUpsert = vi.hoisted(() => vi.fn());
const mockDiscover = vi.hoisted(() => vi.fn());
const mockTest = vi.hoisted(() => vi.fn());
const mockDelete = vi.hoisted(() => vi.fn());
const mockExport = vi.hoisted(() => vi.fn());
const mockAudit = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({ useParams: mockUseParams }));
vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({ user_id_str: '7' }),
}));
vi.mock('../service', () => ({
  deleteMCPToolServer: mockDelete,
  discoverMCPToolServer: mockDiscover,
  exportMCPToolServer: mockExport,
  getMCPToolServer: vi.fn(),
  listMCPToolAuditEvents: mockAudit,
  listMCPToolRegistryEntries: vi.fn(),
  listMCPToolServers: mockList,
  testMCPToolCall: mockTest,
  upsertMCPToolServer: mockUpsert,
}));

vi.mock('@coze-arch/coze-design', () => {
  const Button = ({
    children,
    disabled,
    loading,
    onClick,
  }: {
    children?: ReactNode;
    disabled?: boolean;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      disabled={disabled || loading}
      onClick={() => onClick?.()}
    >
      {children}
    </button>
  );
  const Input = ({
    value,
    onChange,
    ...props
  }: {
    value?: string;
    onChange?: (value: string) => void;
  } & Record<string, unknown>) => (
    <input
      aria-label={props['aria-label'] as string | undefined}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  );
  const TextArea = ({
    value,
    onChange,
    ...props
  }: {
    value?: string;
    onChange?: (value: string) => void;
  } & Record<string, unknown>) => (
    <textarea
      aria-label={props['aria-label'] as string | undefined}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  );
  const Switch = ({
    checked,
    disabled,
    loading,
    onChange,
    ...props
  }: {
    checked?: boolean;
    disabled?: boolean;
    loading?: boolean;
    onChange?: (checked: boolean) => void;
  } & Record<string, unknown>) => (
    <button
      type="button"
      role="switch"
      aria-label={props['aria-label'] as string | undefined}
      aria-checked={checked}
      disabled={disabled || loading}
      onClick={() => onChange?.(!checked)}
    />
  );
  const SideSheet = ({
    children,
    title,
    visible,
  }: {
    children?: ReactNode;
    title?: ReactNode;
    visible?: boolean;
  }) =>
    visible ? (
      <section role="dialog">
        <h2>{title}</h2>
        {children}
      </section>
    ) : null;
  const Modal = ({
    cancelText,
    children,
    okText,
    onCancel,
    onOk,
    title,
    visible,
  }: {
    cancelText?: string;
    children?: ReactNode;
    okText?: string;
    onCancel?: () => void;
    onOk?: () => void;
    title?: ReactNode;
    visible?: boolean;
  }) =>
    visible ? (
      <section role="dialog">
        <h2>{title}</h2>
        {children}
        <button type="button" onClick={onCancel}>
          {cancelText}
        </button>
        <button type="button" onClick={onOk}>
          {okText}
        </button>
      </section>
    ) : null;
  const Tabs = ({
    children,
    onChange,
  }: {
    children?: ReactNode;
    onChange?: (itemKey: string) => void;
  }) => (
    <div>
      {Children.map(children, child =>
        isValidElement(child)
          ? cloneElement(
              child as ReactElement<{
                onSelect?: (itemKey: string) => void;
              }>,
              { onSelect: onChange },
            )
          : child,
      )}
    </div>
  );
  const TabPane = ({
    children,
    itemKey,
    onSelect,
    tab,
  }: {
    children?: ReactNode;
    itemKey?: string;
    onSelect?: (itemKey: string) => void;
    tab?: ReactNode;
  }) => (
    <section>
      <button type="button" onClick={() => itemKey && onSelect?.(itemKey)}>
        {tab}
      </button>
      {children}
    </section>
  );
  return {
    Button,
    Input,
    Modal,
    SideSheet,
    ['Spin']: () => <span>loading</span>,
    Switch,
    TabPane,
    Tabs,
    TextArea,
  };
});

import ToolsPage from '../index';
import { MCPToolSettingsPanel } from '../mcp-settings-panel';

const customServer = {
  server_id: '100',
  space_id: 'space-1',
  creator_id: '7',
  source_type: 'custom' as const,
  name: 'browser-tools',
  description: 'Browser automation tools',
  server_type: 'stdio',
  enabled: true,
  config: '{"command":"npx"}',
  auth: '{"configured":true}',
  tools: [
    {
      name: 'search',
      description: 'Search the web',
      input_schema:
        '{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}',
    },
  ],
  resources: [
    {
      resource_id: 'mcp_resource_safe',
      uri: 'file:///private/credential.txt',
      name: '',
      description: '',
      mime_type: 'text/plain',
    },
  ],
  prompts: [],
  health_status: 'healthy',
  health_checked_at: 1717000200000,
  health_latency_ms: 12,
  health_error: '',
  created_at: 1717000000000,
  updated_at: 1717000300000,
};

const officialServer = {
  ...customServer,
  server_id: '200',
  creator_id: '0',
  source_type: 'official' as const,
  name: 'official-search',
};

describe('ToolsPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockList.mockReset();
    mockList.mockResolvedValue({
      data: {
        servers: [customServer, officialServer],
        total: 2,
        can_manage: true,
      },
      code: 0,
      msg: '',
    });
    mockUpsert.mockReset();
    mockUpsert.mockResolvedValue({ data: customServer, code: 0, msg: '' });
    mockDiscover.mockReset();
    mockDiscover.mockResolvedValue({
      data: {
        tools: customServer.tools,
        resources: customServer.resources,
        prompts: [],
      },
      code: 0,
      msg: '',
    });
    mockTest.mockReset();
    mockTest.mockResolvedValue({
      data: {
        status: 'success',
        output: '{"result":"completed"}',
        latency_ms: 12,
      },
      code: 0,
      msg: '',
    });
    mockDelete.mockReset();
    mockDelete.mockResolvedValue({ data: customServer, code: 0, msg: '' });
    mockExport.mockReset();
    mockExport.mockResolvedValue({
      data: {
        name: customServer.name,
        description: customServer.description,
        server_type: customServer.server_type,
        config: customServer.config,
        tools: customServer.tools,
        resources: [],
        prompts: [],
      },
      code: 0,
      msg: '',
    });
    mockAudit.mockReset();
    mockAudit.mockResolvedValue({
      data: {
        events: [
          {
            event_id: '1',
            actor_id: '7',
            tool_name: 'search',
            status: 'success',
            latency_ms: 12,
            created_at: 1717000300000,
          },
        ],
        next_cursor: '',
      },
      code: 0,
      msg: '',
    });
  });

  it('renders the Nuwax-aligned MCP management workspace on the existing page', async () => {
    const { container, root } = await renderToolsPage();

    expect(mockList).toHaveBeenCalledWith({ space_id: 'space-1' });
    expect(container.textContent).toContain('MCP 管理');
    expect(container.textContent).toContain('自定义服务');
    expect(container.textContent).toContain('官方服务');
    expect(container.textContent).toContain('创建者');
    expect(container.textContent).toContain('部署状态');
    expect(container.textContent).toContain('新建 MCP 服务');
    expect(container.textContent).toContain('browser-tools');
    expect(container.textContent).toContain('Browser automation tools');
    expect(container.textContent).toContain('运行正常');
    expect(
      container.querySelector('[aria-label="搜索 MCP 服务"]'),
    ).toBeTruthy();

    cleanup(container, root);
  });

  it('creates a custom server through the atomic backend operation without leaving /tools', async () => {
    const { container, root } = await renderToolsPage();

    clickButton(container, '+ 新建 MCP 服务');
    changeValue(container, '服务名称', 'docs-mcp');
    changeValue(container, '服务描述', 'Documentation search');
    changeValue(container, '服务配置 JSON', '{"command":"npx"}');

    await act(async () => {
      clickButton(container, '保存并发现能力');
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockUpsert).toHaveBeenCalledWith(
      expect.objectContaining({
        space_id: 'space-1',
        name: 'docs-mcp',
        server_type: 'stdio',
        tools: [],
      }),
    );
    expect(mockDiscover).not.toHaveBeenCalled();

    cleanup(container, root);
  });

  it('keeps existing authentication write-only while editing', async () => {
    const { container, root } = await renderToolsPage();

    clickButton(container, '编辑配置');
    const authInput = container.querySelector(
      '[aria-label="认证配置 JSON"]',
    ) as HTMLTextAreaElement;
    expect(authInput.value).toBe('');

    await act(async () => {
      clickButton(container, '保存并发现能力');
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockUpsert).toHaveBeenCalledWith(
      expect.objectContaining({
        server_id: '100',
        auth: '{"configured":true}',
      }),
    );

    cleanup(container, root);
  });

  it('opens capabilities, runs a tool and loads safe audit metadata', async () => {
    const { container, root } = await renderToolsPage();

    clickButton(container, '查看能力');
    expect(container.textContent).not.toContain(
      'file:///private/credential.txt',
    );
    clickButton(container, '调用日志');
    expect(mockAudit).toHaveBeenCalledWith({
      server_id: '100',
      limit: 20,
      cursor: undefined,
    });
    expect(container.textContent).toContain('工具 1');
    expect(container.textContent).toContain('调用日志');

    clickButton(container, '试运行');
    changeValue(container, '参数 query', 'coze');
    await act(async () => {
      clickButton(container, '运行');
      await Promise.resolve();
    });

    expect(mockTest).toHaveBeenCalledWith({
      server_id: '100',
      tool_name: 'search',
      arguments: expect.stringContaining('"query": "coze"'),
    });

    cleanup(container, root);
  });

  it('keeps official servers read-only apart from enable and test actions', async () => {
    const { container, root } = await renderToolsPage();
    clickButton(container, '官方服务');

    expect(container.textContent).toContain('official-search');
    expect(container.textContent).toContain('官方只读');
    expect(container.textContent).not.toContain('编辑配置');
    expect(container.textContent).not.toContain('服务导出');
    expect(
      container.querySelector('[aria-label="关闭 official-search"]'),
    ).toBeTruthy();

    cleanup(container, root);
  });

  it('exposes the complete MCP workflow in account settings', async () => {
    const { container, root } = await renderMcpSettingsPanel();

    expect(container.textContent).toContain('browser-tools');
    expect(container.textContent).toContain('新建 MCP 服务');
    expect(container.textContent).toContain('查看能力');
    expect(container.textContent).toContain('编辑配置');
    expect(container.textContent).toContain('服务导出');
    expect(
      container.querySelector('[aria-label="关闭 browser-tools"]'),
    ).toBeTruthy();

    cleanup(container, root);
  });
});

const renderToolsPage = async () => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;
  await act(async () => {
    root = createRoot(container);
    root.render(<ToolsPage />);
    await Promise.resolve();
  });
  return { container, root };
};

const renderMcpSettingsPanel = async () => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;
  await act(async () => {
    root = createRoot(container);
    root.render(<MCPToolSettingsPanel spaceId="123" />);
    await Promise.resolve();
  });
  return { container, root };
};

const clickButton = (container: HTMLElement, text: string) => {
  const button = Array.from(container.querySelectorAll('button')).find(
    item => item.textContent?.trim() === text,
  );
  if (!button) {
    throw new Error(`button not found: ${text}`);
  }
  act(() => button.dispatchEvent(new MouseEvent('click', { bubbles: true })));
};

const changeValue = (
  container: HTMLElement,
  ariaLabel: string,
  value: string,
) => {
  const input = container.querySelector(`[aria-label="${ariaLabel}"]`) as
    | HTMLInputElement
    | HTMLTextAreaElement
    | null;
  if (!input) {
    throw new Error(`field not found: ${ariaLabel}`);
  }
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(
      input instanceof HTMLTextAreaElement
        ? HTMLTextAreaElement.prototype
        : HTMLInputElement.prototype,
      'value',
    )?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
};

const cleanup = (container: HTMLElement, root?: Root) => {
  act(() => root?.unmount());
  container.remove();
};
