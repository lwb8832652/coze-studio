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

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockListMCPToolServers = vi.hoisted(() => vi.fn());
const mockUpsertMCPToolServer = vi.hoisted(() => vi.fn());
const mockTestMCPToolCall = vi.hoisted(() => vi.fn());
const mockDeleteMCPToolServer = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  deleteMCPToolServer: mockDeleteMCPToolServer,
  getMCPToolServer: vi.fn(),
  listMCPToolServers: mockListMCPToolServers,
  testMCPToolCall: mockTestMCPToolCall,
  upsertMCPToolServer: mockUpsertMCPToolServer,
}));

vi.mock('@coze-arch/coze-design', () => {
  const mockSwitchComponent = (
    props: {
      checked?: boolean;
      disabled?: boolean;
      loading?: boolean;
      onChange?: (checked: boolean) => void;
    } & Record<string, unknown>,
  ) => {
    const { checked, disabled, loading, onChange } = props;
    const ariaLabel =
      typeof props['aria-label'] === 'string' ? props['aria-label'] : undefined;

    return (
      <button
        type="button"
        aria-label={ariaLabel}
        aria-checked={checked}
        disabled={disabled || loading}
        role="switch"
        onClick={() => onChange?.(!checked)}
      />
    );
  };

  return {
    ['Switch']: mockSwitchComponent,
  };
});

import ToolsPage from '../index';

const server = {
  server_id: '100',
  space_id: 'space-1',
  name: 'browser-tools',
  description: 'Browser automation tools',
  server_type: 'stdio',
  enabled: true,
  config: '{"command":"npx"}',
  auth: '{"type":"none"}',
  tools: [
    {
      name: 'search',
      description: 'Search the web',
      input_schema: '{"type":"object"}',
    },
  ],
  health_status: 'healthy',
  health_checked_at: 1717000200000,
  health_latency_ms: 12,
  health_error: '',
  created_at: 1717000000000,
  updated_at: 1717000300000,
};

describe('ToolsPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockListMCPToolServers.mockReset();
    mockListMCPToolServers.mockResolvedValue({
      data: { servers: [server], total: 1 },
      code: 0,
      msg: '',
    });
    mockUpsertMCPToolServer.mockReset();
    mockUpsertMCPToolServer.mockResolvedValue({
      data: server,
      code: 0,
      msg: '',
    });
    mockTestMCPToolCall.mockReset();
    mockTestMCPToolCall.mockResolvedValue({
      data: {
        status: 'success',
        output: '{"server_name":"browser-tools","tool_name":"search"}',
        latency_ms: 12,
      },
      code: 0,
      msg: '',
    });
    mockDeleteMCPToolServer.mockReset();
    mockDeleteMCPToolServer.mockResolvedValue({
      data: server,
      code: 0,
      msg: '',
    });
  });

  it('renders DeerFlow-style MCP server settings from the backend', async () => {
    const { container, root } = await renderToolsPage();

    expect(mockListMCPToolServers).toHaveBeenCalledWith({
      space_id: 'space-1',
    });
    expect(container.textContent).toContain('工具');
    expect(container.textContent).toContain('管理 MCP 工具的配置和启用状态。');
    expect(container.textContent).toContain('browser-tools');
    expect(container.textContent).toContain('Browser automation tools');
    expect(container.textContent).not.toContain('全部工具');
    expect(container.textContent).not.toContain('创建工具配置');
    expect(container.textContent).not.toContain('测试 search');
    expect(container.textContent).not.toContain('删除');
    expect(container.querySelector('[role="switch"]')).toBeTruthy();

    cleanup(container, root);
  });

  it('toggles an MCP server enabled state through the existing config API', async () => {
    const { container, root } = await renderToolsPage();

    const switchButton = container.querySelector(
      'button[role="switch"][aria-label="关闭 browser-tools"]',
    ) as HTMLButtonElement;

    await act(async () => {
      switchButton.click();
      await Promise.resolve();
    });

    expect(mockUpsertMCPToolServer).toHaveBeenCalledWith(
      expect.objectContaining({
        server_id: '100',
        space_id: 'space-1',
        name: 'browser-tools',
        server_type: 'stdio',
        enabled: false,
        config: '{"command":"npx"}',
        auth: '{"type":"none"}',
        tools: server.tools,
      }),
    );
    expect(mockListMCPToolServers).toHaveBeenCalledTimes(2);

    cleanup(container, root);
  });

  it('shows backend business errors instead of a false empty state', async () => {
    mockListMCPToolServers.mockResolvedValueOnce({
      code: 401,
      msg: 'missing session_key in cookie',
    });

    const { container, root } = await renderToolsPage();

    expect(container.textContent).toContain('missing session_key in cookie');
    expect(container.textContent).not.toContain('暂无 MCP 工具。');

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

const cleanup = (container: HTMLElement, root?: Root) => {
  act(() => {
    root?.unmount();
  });
  container.remove();
};
