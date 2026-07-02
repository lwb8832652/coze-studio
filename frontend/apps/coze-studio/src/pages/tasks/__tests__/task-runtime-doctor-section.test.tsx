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

import type { ReactNode } from 'react';

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockGetWorkbenchRuntimeDoctor = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getWorkbenchRuntimeDoctor: mockGetWorkbenchRuntimeDoctor,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    'aria-label': ariaLabel,
    children,
    icon,
    loading,
    onClick,
  }: {
    'aria-label'?: string;
    children?: ReactNode;
    icon?: ReactNode;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      data-loading={loading}
      onClick={onClick}
    >
      {icon}
      {children}
    </button>
  ),
  Spin: ({
    children,
    spinning,
  }: {
    children?: ReactNode;
    spinning?: boolean;
  }) => <div data-spinning={spinning}>{children}</div>,
  Tag: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design icon names. */
vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozRefresh: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import { TaskRuntimeDoctorSection } from '../task-runtime-doctor-section';

const renderRuntimeDoctorSection = async (spaceId = 'space-1') => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;

  await act(async () => {
    root = createRoot(container);
    root.render(<TaskRuntimeDoctorSection spaceId={spaceId} />);
    await Promise.resolve();
  });

  return {
    container,
    unmount: () => {
      act(() => {
        root?.unmount();
      });
      container.remove();
    },
  };
};

describe('TaskRuntimeDoctorSection', () => {
  beforeEach(() => {
    mockGetWorkbenchRuntimeDoctor.mockReset();
    mockGetWorkbenchRuntimeDoctor.mockResolvedValue({
      data: {
        status: 'warning',
        runtime: {
          default_mode: 'eino_adk',
          eino_adk_enabled: true,
        },
        model: {
          status: 'ready',
          configured: true,
          live_probe: 'ready',
          capabilities: {
            native_tool_search: true,
            thinking: true,
            reasoning: false,
            vision: true,
            pdf: false,
            file: false,
            audio: false,
            video: false,
          },
        },
        sandbox: {
          status: 'ready',
          runner_type: 'sandbox',
          network: 'configured',
          process: 'restricted',
          ffi: 'restricted',
          node_modules: 'configured',
          message: 'sandbox code runner policy is configured',
        },
        web_tools: {
          web_fetch: {
            status: 'ready',
            configured: true,
            message: 'web_fetch is available',
          },
          web_search: {
            status: 'disabled',
            configured: false,
            message: 'web_search backend is disabled',
          },
        },
        mcp_tools: {
          status: 'warning',
          total_servers: 3,
          enabled_servers: 2,
          healthy_servers: 1,
          unhealthy_servers: 0,
          unknown_servers: 2,
        },
        checks: [
          {
            name: 'runtime.eino_adk',
            category: 'runtime',
            status: 'ready',
            message: 'Eino ADK runtime is enabled',
          },
          {
            name: 'model.default',
            category: 'model',
            status: 'ready',
            message: 'Workbench default chat model is configured',
          },
          {
            name: 'model.capabilities',
            category: 'model',
            status: 'ready',
            message:
              'Detected provider capabilities: native_tool_search, thinking, vision',
          },
          {
            name: 'model.live_connectivity',
            category: 'model',
            status: 'ready',
            message: 'Live model probe succeeded',
          },
          {
            name: 'sandbox.runner_policy',
            category: 'sandbox',
            status: 'ready',
            message:
              'sandbox code runner policy is configured; network configured; process restricted; ffi restricted; node modules configured',
          },
          {
            name: 'skills.runtime_catalog',
            category: 'skills',
            status: 'ready',
            message: '2 enabled / 3 total skills: research, writer',
          },
          {
            name: 'mcp_tools.runtime_health',
            category: 'mcp_tools',
            status: 'warning',
            message: '1 healthy, 0 unhealthy, 2 unknown / 3 total',
          },
        ],
      },
      code: 0,
      msg: '',
    });
  });

  it('renders runtime, web, mcp, and deferred P0 diagnostic cards safely', async () => {
    const { container, unmount } = await renderRuntimeDoctorSection();

    expect(mockGetWorkbenchRuntimeDoctor).toHaveBeenCalledWith({
      space_id: 'space-1',
    });
    expect(container.textContent).toContain('运行诊断');
    expect(container.textContent).toContain('需要关注');
    expect(container.textContent).toContain('Eino ADK');
    expect(container.textContent).toContain('已启用');
    expect(container.textContent).toContain('默认模式 eino_adk');
    expect(container.textContent).toContain('Web Fetch');
    expect(container.textContent).toContain('web_fetch is available');
    expect(container.textContent).toContain('Web Search');
    expect(container.textContent).toContain('未配置');
    expect(container.textContent).toContain('MCP 工具');
    expect(container.textContent).toContain('总计 3');
    expect(container.textContent).toContain('健康 1');
    expect(container.textContent).toContain('未知 2');
    expect(container.textContent).toContain('模型连通');
    expect(container.textContent).toContain('默认模型已配置');
    expect(container.textContent).toContain('Live Probe 正常');
    expect(container.textContent).toContain('模型能力');
    expect(container.textContent).toContain('原生工具搜索、思考、视觉');
    expect(container.textContent).toContain(
      'Workbench default chat model is configured',
    );
    expect(container.textContent).toContain('Sandbox');
    expect(container.textContent).toContain('Runner sandbox');
    expect(container.textContent).toContain('网络 configured');
    expect(container.textContent).toContain('Skill 检查');
    expect(container.textContent).toContain(
      '2 enabled / 3 total skills: research, writer',
    );
    expect(container.textContent).toContain('记忆检查');
    expect(container.textContent).toContain('后端深度检查待接入');
    expect(container.textContent).not.toContain('api_key');
    expect(container.textContent).not.toContain('secret');
    expect(container.textContent).not.toContain('tool_arguments');

    unmount();
  });

  it('refreshes runtime doctor data from the panel action', async () => {
    const { container, unmount } = await renderRuntimeDoctorSection();
    const refreshButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('刷新'),
    );
    expect(refreshButton).toBeTruthy();

    await act(async () => {
      Simulate.click(refreshButton as HTMLButtonElement);
      await Promise.resolve();
    });

    expect(mockGetWorkbenchRuntimeDoctor).toHaveBeenCalledTimes(2);
    expect(mockGetWorkbenchRuntimeDoctor).toHaveBeenLastCalledWith({
      space_id: 'space-1',
    });

    unmount();
  });

  it('shows bounded error state when runtime doctor loading fails', async () => {
    mockGetWorkbenchRuntimeDoctor.mockRejectedValueOnce(
      new Error('runtime doctor unavailable'),
    );

    const { container, unmount } = await renderRuntimeDoctorSection();

    expect(container.textContent).toContain('runtime doctor unavailable');
    expect(container.querySelector('[role="alert"]')).toBeTruthy();

    unmount();
  });
});
