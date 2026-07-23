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

import { type ReactNode } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import type { workbenchTask } from '@coze-studio/api-schema';

import { TaskUsagePopover } from '../task-usage-popover';
import { TaskUsageDetailDrawer } from '../task-usage-detail-drawer';
import {
  buildTaskUsageDetailItems,
  loadTaskTokenUsageViewMode,
  saveTaskTokenUsageViewMode,
  type TaskUsageDetailItem,
} from '../task-message-token-usage';
import type { TaskDetailTokenUsage } from '../task-detail-loader';
import { createRoot } from './task-test-root-registry';

const originalMatchMediaDescriptor = Object.getOwnPropertyDescriptor(
  window,
  'matchMedia',
);

vi.mock('lottie-web', () => ({
  destroy: vi.fn(),
  loadAnimation: vi.fn(),
  default: {
    destroy: vi.fn(),
    loadAnimation: vi.fn(),
  },
}));

const createUsage = (totalTokens = 100): TaskDetailTokenUsage => ({
  inputTokens: totalTokens,
  outputTokens: 0,
  totalTokens,
  costMicros: 0,
  currency: '',
  callCount: 1,
  leadAgentTokens: totalTokens,
  subagentTokens: 0,
  middlewareTokens: 0,
  toolTokens: 0,
  modelAttributions: ['safe-provider / safe-model'],
});

const renderNode = async (node: ReactNode) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  await act(async () => {
    root.render(node);
    await Promise.resolve();
    await Promise.resolve();
  });

  return { container, root };
};

const setMobileViewport = (matches: boolean) => {
  const listeners = new Set<(event: MediaQueryListEvent) => void>();
  const matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addEventListener: (
      _type: string,
      listener: (event: MediaQueryListEvent) => void,
    ) => listeners.add(listener),
    removeEventListener: (
      _type: string,
      listener: (event: MediaQueryListEvent) => void,
    ) => listeners.delete(listener),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: matchMedia,
  });

  return matchMedia;
};

const findButton = (label: string) =>
  Array.from(document.body.querySelectorAll<HTMLButtonElement>('button')).find(
    button => button.textContent?.includes(label),
  );

describe('task usage resilient entry, drawer performance, and accessibility', () => {
  beforeEach(() => {
    setMobileViewport(false);
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
    if (originalMatchMediaDescriptor) {
      Object.defineProperty(window, 'matchMedia', originalMatchMediaDescriptor);
    } else {
      Reflect.deleteProperty(window, 'matchMedia');
    }
  });

  it('handles localStorage read and write failures without changing the default mode', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    expect(loadTaskTokenUsageViewMode()).toBe('per_turn');

    vi.restoreAllMocks();
    const setItem = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new DOMException('full', 'QuotaExceededError');
      });
    expect(() => saveTaskTokenUsageViewMode('debug')).not.toThrow();
    expect(setItem).toHaveBeenCalledTimes(1);
  });

  it('hides a non-off entry before the first reply, but keeps stats, retry, and off recovery states distinct', async () => {
    const { container, root } = await renderNode(
      <TaskUsagePopover
        detailItems={[]}
        hasAssistantReply={false}
        loading
        scopeKey="thread-three-state"
        viewMode="summary"
      />,
    );
    expect(
      container.querySelector('[data-testid="task-usage-trigger"]'),
    ).toBeNull();
    expect(container.querySelector('[aria-label="用量显示设置"]')).toBeNull();

    await act(async () => {
      root.render(
        <TaskUsagePopover
          detailItems={[]}
          hasAssistantReply={false}
          scopeKey="thread-three-state"
          tokenUsage={createUsage(80)}
          viewMode="summary"
        />,
      );
      await Promise.resolve();
    });
    expect(
      container.querySelector('[data-testid="task-usage-trigger"]'),
    ).toBeNull();

    await act(async () => {
      root.render(
        <TaskUsagePopover
          detailItems={[]}
          error="usage failed"
          hasAssistantReply
          scopeKey="thread-three-state"
          viewMode="summary"
          onRetry={vi.fn()}
        />,
      );
      await Promise.resolve();
    });
    const retryTrigger = container.querySelector<HTMLButtonElement>(
      '[data-testid="task-usage-trigger"]',
    );
    expect(retryTrigger).not.toBeNull();
    await act(async () => {
      retryTrigger?.click();
      await Promise.resolve();
    });
    expect(document.body.textContent).toContain('usage failed');
    expect(findButton('重试')).toBeDefined();

    await act(async () => {
      root.render(
        <TaskUsagePopover
          detailItems={[]}
          hasAssistantReply={false}
          scopeKey="thread-three-state"
          viewMode="off"
        />,
      );
      await Promise.resolve();
    });
    expect(
      container.querySelector('[data-testid="task-usage-trigger"]'),
    ).toBeNull();
    expect(
      container.querySelector('[aria-label="用量显示设置"]'),
    ).not.toBeNull();
  });

  it('uses a real bottom SideSheet on mobile and restores focus after Escape', async () => {
    const matchMedia = setMobileViewport(true);
    const { container } = await renderNode(
      <TaskUsagePopover
        detailItems={[]}
        hasAssistantReply
        scopeKey="thread-mobile"
        tokenUsage={createUsage(120)}
        viewMode="per_turn"
      />,
    );
    const trigger = container.querySelector<HTMLButtonElement>(
      '[data-testid="task-usage-trigger"]',
    );
    await act(async () => {
      trigger?.click();
      await Promise.resolve();
    });
    await act(async () => {
      findButton('查看用量明细')?.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    const drawer = document.body.querySelector<HTMLElement>(
      '[data-testid="task-usage-detail-drawer"]',
    );
    expect(matchMedia).toHaveBeenCalled();
    expect(drawer?.dataset.placement).toBe('bottom');
    expect(
      document.body.querySelector('.semi-sidesheet-bottom'),
    ).not.toBeNull();

    await act(async () => {
      const escapeEvent = new KeyboardEvent('keydown', {
        bubbles: true,
        key: 'Escape',
      });
      Object.defineProperty(escapeEvent, 'keyCode', {
        configurable: true,
        value: 27,
      });
      window.dispatchEvent(escapeEvent);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(document.activeElement).toBe(trigger);
  });

  it('does not consume detail rows while hidden and mounts visible rows in keyboard-reachable batches', async () => {
    const hiddenRows = new Proxy(
      [
        {
          canLocate: true,
          runID: 'sensitive-hidden-run',
          usage: createUsage(),
        },
      ],
      {
        get(target, property, receiver) {
          if (property === Symbol.iterator) {
            throw new Error('hidden rows must not be consumed');
          }
          return Reflect.get(target, property, receiver);
        },
      },
    );
    await expect(
      renderNode(
        <TaskUsageDetailDrawer
          detailItems={hiddenRows}
          viewMode="per_turn"
          visible={false}
          onCancel={vi.fn()}
        />,
      ),
    ).resolves.toBeDefined();

    const detailItems: TaskUsageDetailItem[] = Array.from(
      { length: 1000 },
      (_, index) => ({
        canLocate: index === 999,
        createdAt: 1717000000000 + index,
        runID: `sensitive-run-${String(index).padStart(4, '0')}`,
        usage: createUsage(index + 1),
      }),
    );
    const onLocateReply = vi.fn();
    await renderNode(
      <TaskUsageDetailDrawer
        detailItems={detailItems}
        viewMode="per_turn"
        visible
        onCancel={vi.fn()}
        onLocateReply={onLocateReply}
      />,
    );

    expect(
      document.body.querySelectorAll('.coze-prototype-usage-run-item'),
    ).toHaveLength(100);
    expect(document.body.textContent).not.toContain('sensitive-run-0000');
    const locateButton = findButton('定位回复');
    expect(locateButton).toBeDefined();
    await act(async () => {
      locateButton?.click();
      await Promise.resolve();
    });
    expect(onLocateReply).toHaveBeenCalledWith('sensitive-run-0999');

    const showMore = findButton('显示更多');
    expect(showMore).toBeDefined();
    showMore?.focus();
    expect(document.activeElement).toBe(showMore);
    await act(async () => {
      showMore?.click();
      await Promise.resolve();
    });
    expect(
      document.body.querySelectorAll('.coze-prototype-usage-run-item'),
    ).toHaveLength(200);
  });

  it('marks only message-backed runs locatable and disables locating while rows are partial', async () => {
    const runUsage = createUsage(42);
    const items = buildTaskUsageDetailItems(
      [
        {
          role: 'assistant',
          run_id: 'sensitive-matched-run',
          content: 'safe answer',
          created_at: 1717000000000,
        } as workbenchTask.TaskThreadMessage,
      ],
      {
        'sensitive-matched-run': runUsage,
        'sensitive-unmatched-run': createUsage(10),
      },
    );
    expect(items).toEqual([
      expect.objectContaining({
        canLocate: true,
        runID: 'sensitive-matched-run',
      }),
      expect.objectContaining({
        canLocate: false,
        runID: 'sensitive-unmatched-run',
      }),
    ]);

    await renderNode(
      <TaskUsageDetailDrawer
        detailItems={items}
        isPartial
        loadedCount={2}
        totalCount={3}
        viewMode="per_turn"
        visible
        onCancel={vi.fn()}
        onLocateReply={vi.fn()}
      />,
    );
    expect(findButton('定位回复')).toBeUndefined();
    expect(document.body.textContent).toContain('明细不完整，暂不能定位');
    expect(document.body.textContent).not.toContain('sensitive-matched-run');
  });
});
