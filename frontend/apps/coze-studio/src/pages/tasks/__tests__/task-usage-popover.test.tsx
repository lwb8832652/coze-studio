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

import type { ComponentProps } from 'react';

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

import { TaskUsagePopover } from '../task-usage-popover';
import {
  getCurrentAssistantRunID,
  loadTaskTokenUsageViewMode,
} from '../task-message-token-usage';
import type { TaskDetailTokenUsage } from '../task-detail-loader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const createUsage = (
  overrides: Partial<TaskDetailTokenUsage> = {},
): TaskDetailTokenUsage => ({
  inputTokens: 26_000,
  outputTokens: 3_900,
  totalTokens: 29_900,
  costMicros: 0,
  currency: '',
  callCount: 2,
  leadAgentTokens: 29_900,
  subagentTokens: 0,
  middlewareTokens: 0,
  toolTokens: 0,
  modelAttributions: ['openai / gpt-4.1'],
  ...overrides,
});

const renderPopover = (
  props: Partial<ComponentProps<typeof TaskUsagePopover>> = {},
) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  act(() => {
    root.render(
      <TaskUsagePopover
        scopeKey="thread-1"
        tokenUsage={createUsage()}
        currentReplyUsage={createUsage({
          inputTokens: 3000,
          outputTokens: 636,
          totalTokens: 3636,
        })}
        detailItems={[
          {
            runID: 'run-current-1',
            createdAt: 1_717_000_200_000,
            usage: createUsage({
              inputTokens: 3000,
              outputTokens: 636,
              totalTokens: 3636,
            }),
          },
        ]}
        viewMode="per_turn"
        {...props}
      />,
    );
  });

  return { container, root };
};

const cleanup = (root: Root, container: HTMLDivElement) => {
  act(() => root.unmount());
  container.remove();
};

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
  document
    .querySelectorAll('.semi-portal')
    .forEach(portal => portal.parentElement?.removeChild(portal));
});

describe('TaskUsagePopover', () => {
  it('opens upward from the compact composer trigger and launches run details', async () => {
    const { container, root } = renderPopover();
    const trigger = container.querySelector<HTMLButtonElement>(
      'button.coze-prototype-token-usage',
    );

    expect(trigger).toBeTruthy();
    expect(trigger?.textContent).toContain('用量');
    expect(trigger?.textContent).toContain('29.9K');
    expect(trigger?.getAttribute('aria-haspopup')).toBe('dialog');

    await act(async () => {
      trigger?.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    const summary = document.body.querySelector(
      '[data-testid="task-token-usage-popover"]',
    );
    expect(summary?.textContent).toContain('Token 用量');
    expect(summary?.textContent).toContain('本次对话');
    expect(summary?.textContent).toContain('29.9K');
    expect(summary?.textContent).toContain('输入');
    expect(summary?.textContent).toContain('26.0K');
    expect(summary?.textContent).toContain('输出');
    expect(summary?.textContent).toContain('3,900');
    expect(summary?.textContent).toContain('当前回复');
    expect(summary?.textContent).toContain('3,636');

    const detailButton = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>('button'),
    ).find(button => button.textContent?.includes('查看用量明细'));
    expect(detailButton).toBeTruthy();

    await act(async () => {
      Simulate.click(detailButton!);
      await Promise.resolve();
      await Promise.resolve();
    });

    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );
    expect(drawer).toBeTruthy();
    expect(document.body.textContent).toContain('用量明细');
    expect(drawer?.textContent).not.toContain('run-current-1');
    expect(drawer?.textContent).toContain('3,636');

    cleanup(root, container);
  });

  it('keeps error and retry local to usage without disabling the trigger', async () => {
    const onRetry = vi.fn();
    const { container, root } = renderPopover({
      currentReplyUsage: undefined,
      detailItems: [],
      error: 'Token 统计暂不可用',
      tokenUsage: undefined,
      onRetry,
    });
    const trigger = container.querySelector<HTMLButtonElement>(
      'button.coze-prototype-token-usage',
    );

    expect(trigger?.disabled).toBe(false);
    expect(trigger?.textContent).toContain('用量');

    await act(async () => {
      trigger?.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(
      document.body.querySelector('[role="alert"]')?.textContent,
    ).toContain('Token 统计暂不可用');
    const retry = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>('button'),
    ).find(button => button.textContent?.includes('重试'));
    Simulate.click(retry!);
    expect(onRetry).toHaveBeenCalledTimes(1);

    cleanup(root, container);
  });

  it('renders loading and empty states without inventing aggregate totals', async () => {
    const { container, root } = renderPopover({
      currentReplyUsage: undefined,
      detailItems: [],
      loading: true,
      tokenUsage: undefined,
    });
    const trigger = container.querySelector<HTMLButtonElement>(
      'button.coze-prototype-token-usage',
    );

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(document.body.textContent).toContain('正在加载 Token 用量');
    expect(document.body.textContent).not.toContain('总计 0');

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
    });
    cleanup(root, container);

    const emptyRender = renderPopover({
      currentReplyUsage: undefined,
      detailItems: [],
      tokenUsage: undefined,
    });
    const emptyTrigger = emptyRender.container.querySelector<HTMLButtonElement>(
      'button.coze-prototype-token-usage',
    );

    await act(async () => {
      Simulate.click(emptyTrigger!);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(document.body.textContent).toContain('暂无 Token 用量');
    await act(async () => {
      Simulate.click(emptyTrigger!);
      await Promise.resolve();
    });
    cleanup(emptyRender.root, emptyRender.container);
  });

  it('returns focus to the trigger when the popover closes', async () => {
    const { container, root } = renderPopover();
    const trigger = container.querySelector<HTMLButtonElement>(
      'button.coze-prototype-token-usage',
    );
    trigger?.focus();

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
      await Promise.resolve();
    });

    const popoverID = trigger?.getAttribute('aria-controls');
    const currentPopover = popoverID
      ? document.getElementById(popoverID)
      : null;
    const closeButton = currentPopover?.querySelector<HTMLButtonElement>(
      'button[aria-label="关闭 Token 用量"]',
    );
    await act(async () => {
      Simulate.click(closeButton!);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(document.activeElement).toBe(trigger);
    cleanup(root, container);
  });

  it('focuses the popover, then closes on Escape and outside click with trigger focus restored', async () => {
    const { container, root } = renderPopover();
    const trigger = container.querySelector<HTMLButtonElement>(
      '[data-testid="task-usage-trigger"]',
    );
    trigger?.focus();

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
      await Promise.resolve();
    });
    const popoverID = trigger?.getAttribute('aria-controls');
    const getCurrentPopover = () =>
      popoverID ? document.getElementById(popoverID) : null;
    expect(document.activeElement).toBe(
      getCurrentPopover()?.querySelector(
        'button[aria-label="关闭 Token 用量"]',
      ),
    );

    await act(async () => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', {
          bubbles: true,
          key: 'Escape',
          keyCode: 27,
        }),
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(
      getCurrentPopover()?.querySelector(
        '[data-testid="task-token-usage-popover"]',
      ) ?? null,
    ).toBeNull();
    expect(document.activeElement).toBe(trigger);

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      document.body.dispatchEvent(
        new MouseEvent('mousedown', { bubbles: true }),
      );
      document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(
      getCurrentPopover()?.querySelector(
        '[data-testid="task-token-usage-popover"]',
      ) ?? null,
    ).toBeNull();
    expect(document.activeElement).toBe(trigger);

    cleanup(root, container);
  });

  it('marks the current reply unknown when its run is outside loaded partial pages', async () => {
    const { container, root } = renderPopover({
      currentReplyIncomplete: true,
      currentReplyUsage: undefined,
      isPartial: true,
      loadedCount: 50,
      totalCount: 75,
    });
    const trigger = container.querySelector<HTMLButtonElement>(
      '[data-testid="task-usage-trigger"]',
    );

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
      await Promise.resolve();
    });

    const summary = document.body.querySelector(
      '[data-testid="task-token-usage-popover"]',
    );
    expect(summary?.textContent).toContain('当前回复');
    expect(summary?.textContent).toContain('未完整加载');
    expect(summary?.textContent).toContain('部分数据');
    expect(summary?.textContent).toContain('已加载 50 / 75 条明细');
    expect(summary?.textContent).not.toContain('当前回复0');

    cleanup(root, container);
  });

  it('keeps a settings-only recovery entry when the saved mode is off', async () => {
    const { container, root } = renderPopover({ viewMode: 'off' });

    expect(
      container.querySelector('[data-testid="task-usage-trigger"]'),
    ).toBeNull();
    const settingsTrigger = container.querySelector<HTMLButtonElement>(
      '[aria-label="用量显示设置"]',
    );
    expect(settingsTrigger).not.toBeNull();
    expect(settingsTrigger?.textContent).not.toContain('29.9K');
    expect(container.textContent).not.toContain('会话总量');

    await act(async () => {
      settingsTrigger?.click();
      await Promise.resolve();
    });

    const drawer = document.body.querySelector(
      '[data-testid="task-usage-detail-drawer"]',
    );
    expect(drawer).not.toBeNull();
    expect(document.body.textContent).toContain('用量显示设置');
    expect(drawer?.textContent).toContain('会话总览');
    expect(drawer?.textContent).not.toContain('会话总量');
    expect(drawer?.textContent).not.toContain('每次 Agent 回复');
    expect(drawer?.textContent).not.toContain('29,900');

    cleanup(root, container);
  });

  it('keeps summary mode aggregate-only without current reply usage', async () => {
    const { container, root } = renderPopover({ viewMode: 'summary' });
    const trigger = container.querySelector<HTMLButtonElement>(
      '[data-testid="task-usage-trigger"]',
    );

    await act(async () => {
      Simulate.click(trigger!);
      await Promise.resolve();
      await Promise.resolve();
    });

    const popoverID = trigger?.getAttribute('aria-controls');
    const popover = popoverID ? document.getElementById(popoverID) : null;
    expect(popover?.textContent).toContain('会话总量');
    expect(popover?.textContent).not.toContain('当前回复');

    cleanup(root, container);
  });
});

describe('token usage compatibility and current-turn scope', () => {
  it.each(['off', 'summary', 'per_turn', 'debug'] as const)(
    'loads the existing %s preference without changing its meaning',
    mode => {
      window.localStorage.setItem(
        'coze.task-detail.token-usage-view-mode',
        mode,
      );

      expect(loadTaskTokenUsageViewMode()).toBe(mode);
    },
  );

  it('selects only the assistant run belonging to the latest user turn', () => {
    const messages = [
      {
        message_id: 'user-old',
        thread_id: 'thread-1',
        run_id: 'run-old',
        role: 'user',
        content: 'old',
        metadata: '',
        created_at: 100,
      },
      {
        message_id: 'assistant-old',
        thread_id: 'thread-1',
        run_id: 'run-old',
        role: 'assistant',
        content: 'old answer',
        metadata: '',
        created_at: 200,
      },
      {
        message_id: 'user-current',
        thread_id: 'thread-1',
        run_id: 'run-current',
        role: 'user',
        content: 'current',
        metadata: '',
        created_at: 300,
      },
    ];

    expect(getCurrentAssistantRunID(messages, 'run-current')).toBe(
      'run-current',
    );
    expect(getCurrentAssistantRunID(messages, 'run-late-old')).toBe('');
  });
});
