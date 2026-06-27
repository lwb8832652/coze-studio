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

  it('renders MCP tool servers from the backend', async () => {
    const { container, root } = await renderToolsPage();

    expect(mockListMCPToolServers).toHaveBeenCalledWith({
      space_id: 'space-1',
    });
    expect(container.textContent).toContain('工具');
    expect(container.textContent).toContain('MCP 工具配置');
    expect(container.textContent).toContain('browser-tools');
    expect(container.textContent).toContain('Browser automation tools');
    expect(container.textContent).toContain('stdio');
    expect(container.textContent).toContain('search');
    expect(container.textContent).toContain('已启用');
    expect(container.textContent).toContain('健康 12ms');

    cleanup(container, root);
  });

  it('creates a default MCP server config for the current space', async () => {
    const { container, root } = await renderToolsPage();

    const createButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('创建工具配置'),
    ) as HTMLButtonElement;

    await act(async () => {
      createButton.click();
      await Promise.resolve();
    });

    expect(mockUpsertMCPToolServer).toHaveBeenCalledWith(
      expect.objectContaining({
        space_id: 'space-1',
        name: 'browser-tools',
        server_type: 'stdio',
        enabled: true,
      }),
    );

    cleanup(container, root);
  });

  it('runs a test call against the first configured tool', async () => {
    const { container, root } = await renderToolsPage();

    const testButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('测试 search'),
    ) as HTMLButtonElement;

    await act(async () => {
      testButton.click();
      await Promise.resolve();
    });

    expect(mockTestMCPToolCall).toHaveBeenCalledWith({
      server_id: '100',
      tool_name: 'search',
      arguments: '{"query":"coze studio"}',
    });
    expect(container.textContent).toContain('browser-tools');
    expect(container.textContent).toContain('latency_ms');

    cleanup(container, root);
  });

  it('deletes an MCP server after confirmation', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const { container, root } = await renderToolsPage();

    const deleteButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '删除',
    ) as HTMLButtonElement;

    await act(async () => {
      deleteButton.click();
      await Promise.resolve();
    });

    expect(mockDeleteMCPToolServer).toHaveBeenCalledWith({
      server_id: '100',
    });
    expect(mockListMCPToolServers).toHaveBeenCalledTimes(2);

    confirmSpy.mockRestore();
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
