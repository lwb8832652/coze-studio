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

import { useState, type ComponentProps } from 'react';

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';

import { createRoot, type Root } from './task-test-root-registry';

vi.mock('lottie-web', () => ({
  destroy: vi.fn(),
  loadAnimation: vi.fn(),
  default: {
    destroy: vi.fn(),
    loadAnimation: vi.fn(),
  },
}));

import { TaskUsageDetailDrawer } from '../task-usage-detail-drawer';
import {
  loadTaskTokenUsageViewMode,
  saveTaskTokenUsageViewMode,
} from '../task-message-token-usage';
import type { TaskDetailTokenUsage } from '../task-detail-loader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const createUsage = (
  overrides: Partial<TaskDetailTokenUsage> = {},
): TaskDetailTokenUsage => ({
  inputTokens: 1200,
  outputTokens: 340,
  totalTokens: 1540,
  costMicros: 2000,
  currency: 'USD',
  callCount: 2,
  leadAgentTokens: 1400,
  subagentTokens: 0,
  middlewareTokens: 0,
  toolTokens: 140,
  modelAttributions: ['openai / gpt-4.1'],
  ...overrides,
});

const renderDrawer = (
  props: Partial<ComponentProps<typeof TaskUsageDetailDrawer>> = {},
) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  const requestedVisible = props.visible ?? true;
  const renderWithVisibility = (visible: boolean) => {
    root.render(
      <TaskUsageDetailDrawer
        tokenUsage={createUsage()}
        detailItems={[
          {
            runID: 'run-auditable-1',
            createdAt: 1_717_000_200_000,
            usage: createUsage(),
          },
        ]}
        viewMode="debug"
        onCancel={vi.fn()}
        onViewModeChange={vi.fn()}
        {...props}
        visible={visible}
      />,
    );
  };

  act(() => {
    renderWithVisibility(false);
  });
  if (requestedVisible) {
    act(() => {
      renderWithVisibility(true);
    });
  }

  return { container, root };
};

const cleanup = (root: Root, container: HTMLDivElement) => {
  act(() => root.unmount());
  container.remove();
};

afterEach(() => {
  vi.restoreAllMocks();
  document
    .querySelectorAll('.semi-portal')
    .forEach(portal => portal.parentElement?.removeChild(portal));
});

describe('TaskUsageDetailDrawer', () => {
  it('shows per-run totals and only bounded audit metadata', () => {
    const unsafeUsage = {
      ...createUsage(),
      prompt: 'forbidden-prompt',
      completion: 'forbidden-completion',
      toolArguments: 'forbidden-tool-arguments',
      providerRawBody: 'forbidden-provider-body',
      credential: 'forbidden-credential',
      objectURI: 'agent-runtime://forbidden-object',
      checkpoint: 'forbidden-checkpoint',
    } as TaskDetailTokenUsage;
    const { container, root } = renderDrawer({
      detailItems: [
        {
          runID: 'run-auditable-1',
          createdAt: 1_717_000_200_000,
          usage: unsafeUsage,
        },
      ],
    });

    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );
    expect(drawer?.textContent).toContain('会话总量');
    expect(drawer?.textContent).toContain('每次 Agent 回复');
    expect(drawer?.textContent).not.toContain('run-auditable-1');
    expect(drawer?.textContent).toContain('输入');
    expect(drawer?.textContent).toContain('1,200');
    expect(drawer?.textContent).toContain('输出');
    expect(drawer?.textContent).toContain('340');
    expect(drawer?.textContent).toContain('合计');
    expect(drawer?.textContent).toContain('1,540');
    expect(drawer?.textContent).toContain('调用次数');
    expect(drawer?.textContent).toContain('openai / gpt-4.1');
    expect(drawer?.querySelector('time')?.getAttribute('datetime')).toBe(
      new Date(1_717_000_200_000).toISOString(),
    );
    expect(drawer?.textContent).not.toContain('forbidden-prompt');
    expect(drawer?.textContent).not.toContain('forbidden-completion');
    expect(drawer?.textContent).not.toContain('forbidden-tool-arguments');
    expect(drawer?.textContent).not.toContain('forbidden-provider-body');
    expect(drawer?.textContent).not.toContain('forbidden-credential');
    expect(drawer?.textContent).not.toContain('agent-runtime://');
    expect(drawer?.textContent).not.toContain('forbidden-checkpoint');
    expect(drawer?.textContent).not.toContain('USD 0.002000');

    cleanup(root, container);
  });

  it('handles partial rows without fabricating a timestamp or metadata', () => {
    const { container, root } = renderDrawer({
      detailItems: [
        {
          runID: 'run-partial',
          usage: createUsage({
            callCount: 0,
            modelAttributions: [],
          }),
        },
      ],
      viewMode: 'per_turn',
    });

    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );
    expect(drawer?.textContent).not.toContain('run-partial');
    expect(drawer?.textContent).toContain('时间暂不可用');
    const auditMetadata = drawer?.querySelector('[aria-label="可审计元数据"]');
    expect(auditMetadata).toBeNull();

    cleanup(root, container);
  });

  it('renders loading, empty, and retryable error states inside the drawer', () => {
    const loadingRender = renderDrawer({
      detailItems: [],
      loading: true,
      tokenUsage: undefined,
    });
    expect(document.body.textContent).toContain('正在加载用量明细');
    cleanup(loadingRender.root, loadingRender.container);

    const emptyRender = renderDrawer({
      detailItems: [],
      tokenUsage: undefined,
    });
    expect(document.body.textContent).toContain('暂无逐次用量明细');
    cleanup(emptyRender.root, emptyRender.container);

    const onRetry = vi.fn();
    const errorRender = renderDrawer({
      detailItems: [],
      error: '用量明细加载失败',
      tokenUsage: undefined,
      onRetry,
    });
    expect(
      document.body.querySelector('[role="alert"]')?.textContent,
    ).toContain('用量明细加载失败');
    const retry = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>('button'),
    ).find(button => button.textContent?.includes('重试'));
    Simulate.click(retry!);
    expect(onRetry).toHaveBeenCalledTimes(1);
    cleanup(errorRender.root, errorRender.container);
  });

  it('delegates Escape and mask close through the real SideSheet', async () => {
    const onCancel = vi.fn();
    const { container, root } = renderDrawer({ onCancel });

    await act(async () => {
      const escapeEvent = new KeyboardEvent('keydown', {
        bubbles: true,
        key: 'Escape',
      });
      Object.defineProperty(escapeEvent, 'keyCode', { value: 27 });
      window.dispatchEvent(escapeEvent);
      await Promise.resolve();
    });
    expect(onCancel).toHaveBeenCalledTimes(1);

    cleanup(root, container);

    const maskOnCancel = vi.fn();
    const maskRender = renderDrawer({ onCancel: maskOnCancel });
    const mask = document.body.querySelector<HTMLElement>(
      '.semi-sidesheet-mask',
    );
    expect(mask).toBeTruthy();
    await act(async () => {
      Simulate.click(mask!);
      await Promise.resolve();
    });
    expect(maskOnCancel).toHaveBeenCalledTimes(1);
    cleanup(maskRender.root, maskRender.container);
  });

  it('uses roving tabIndex and supports all radio navigation keys', async () => {
    const Harness = () => {
      const [viewMode, setViewMode] =
        useState<ComponentProps<typeof TaskUsageDetailDrawer>['viewMode']>(
          'off',
        );

      return (
        <TaskUsageDetailDrawer
          visible
          detailItems={[]}
          viewMode={viewMode}
          onCancel={vi.fn()}
          onViewModeChange={setViewMode}
        />
      );
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() => root.render(<Harness />));
    const radios = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>(
        '[aria-label="Token 用量显示偏好"] [role="radio"]',
      ),
    );

    expect(radios).toHaveLength(4);
    expect(radios.map(radio => radio.tabIndex)).toEqual([0, -1, -1, -1]);
    expect(radios[0].getAttribute('aria-checked')).toBe('true');

    radios[0].focus();
    await act(async () => {
      Simulate.keyDown(radios[0], { key: 'ArrowRight' });
      await Promise.resolve();
    });
    expect(document.activeElement?.textContent).toContain('会话总览');
    expect(radios[1].getAttribute('aria-checked')).toBe('true');

    await act(async () => {
      Simulate.keyDown(radios[1], { key: 'End' });
      await Promise.resolve();
    });
    expect(document.activeElement?.textContent).toContain('审计信息');

    await act(async () => {
      Simulate.keyDown(radios[3], { key: 'ArrowDown' });
      await Promise.resolve();
    });
    expect(document.activeElement?.textContent).toContain('按需查看');

    await act(async () => {
      Simulate.keyDown(radios[0], { key: 'ArrowLeft' });
      await Promise.resolve();
      Simulate.keyDown(radios[3], { key: 'Home' });
      await Promise.resolve();
      Simulate.keyDown(radios[0], { key: 'ArrowUp' });
      await Promise.resolve();
    });
    expect(document.activeElement?.textContent).toContain('审计信息');

    act(() => root.unmount());
    container.remove();
  });

  it('labels every loaded run total as partial when pagination is incomplete', () => {
    const { container, root } = renderDrawer({
      isPartial: true,
      loadedCount: 50,
      totalCount: 75,
    });
    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );

    expect(drawer?.textContent).toContain('部分数据');
    expect(drawer?.textContent).toContain('已加载 50 / 75 条明细');
    expect(drawer?.textContent).toContain('已加载');
    expect(drawer?.textContent).not.toContain('合计');

    cleanup(root, container);
  });

  it('keeps per-turn rows free of debug audit metadata', () => {
    const { container, root } = renderDrawer({ viewMode: 'per_turn' });
    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );
    const runItem = drawer?.querySelector('.coze-prototype-usage-run-item');

    expect(drawer?.textContent).toContain('Agent 回复');
    expect(runItem?.textContent).toContain('输入');
    expect(runItem?.textContent).toContain('输出');
    expect(runItem?.textContent).toContain('合计');
    expect(runItem?.textContent).not.toContain('调用次数');
    expect(runItem?.textContent).not.toContain('Lead Agent');
    expect(runItem?.textContent).not.toContain('Tool');
    expect(runItem?.textContent).not.toContain('模型');
    expect(runItem?.textContent).not.toContain('openai / gpt-4.1');
    expect(drawer?.querySelector('[aria-label="可审计元数据"]')).toBeNull();

    cleanup(root, container);
  });

  it('keeps summary mode aggregate-only without a per-run list', () => {
    const { container, root } = renderDrawer({ viewMode: 'summary' });

    expect(document.body.textContent).toContain('会话总量');
    expect(
      document.body.querySelector('[aria-label="每次 Agent 回复"]'),
    ).toBeNull();

    cleanup(root, container);
  });

  it('explains that off mode can be re-enabled from the settings entry', () => {
    const { container, root } = renderDrawer({ viewMode: 'off' });
    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );

    expect(drawer?.textContent).toContain(
      '隐藏数值和明细，仍可通过设置入口重新开启。',
    );

    cleanup(root, container);
  });

  it('updates audit visibility immediately and persists mode changes', async () => {
    saveTaskTokenUsageViewMode('per_turn');

    const Harness = () => {
      const [viewMode, setViewMode] = useState(loadTaskTokenUsageViewMode());
      const handleViewModeChange = (
        mode: ComponentProps<typeof TaskUsageDetailDrawer>['viewMode'],
      ) => {
        saveTaskTokenUsageViewMode(mode);
        setViewMode(mode);
      };

      return (
        <TaskUsageDetailDrawer
          visible
          tokenUsage={createUsage()}
          detailItems={[
            {
              runID: 'run-mode-switch',
              createdAt: 1_717_000_200_000,
              usage: createUsage(),
            },
          ]}
          viewMode={viewMode}
          onCancel={vi.fn()}
          onViewModeChange={handleViewModeChange}
        />
      );
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    act(() => {
      root.render(<Harness />);
    });

    const getRadio = (label: string) =>
      Array.from(
        document.body.querySelectorAll<HTMLButtonElement>('[role="radio"]'),
      ).find(button => button.textContent?.includes(label));
    expect(document.body.textContent).not.toContain('调用次数');

    await act(async () => {
      Simulate.click(getRadio('审计信息')!);
      await Promise.resolve();
    });
    expect(getRadio('审计信息')?.getAttribute('aria-checked')).toBe('true');
    expect(document.body.textContent).toContain('调用次数');
    expect(loadTaskTokenUsageViewMode()).toBe('debug');

    await act(async () => {
      Simulate.click(getRadio('逐回复')!);
      await Promise.resolve();
    });
    expect(getRadio('逐回复')?.getAttribute('aria-checked')).toBe('true');
    expect(document.body.textContent).not.toContain('调用次数');
    expect(loadTaskTokenUsageViewMode()).toBe('per_turn');

    cleanup(root, container);
  });
});
