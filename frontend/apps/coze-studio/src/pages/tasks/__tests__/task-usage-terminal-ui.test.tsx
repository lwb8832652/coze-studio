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

import { flushSync } from 'react-dom';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';

import { TaskUsagePopover } from '../task-usage-popover';
import { TaskUsageDetailDrawer } from '../task-usage-detail-drawer';
import type { TaskDetailTokenUsage } from '../task-detail-token-usage';
import { createRoot } from './task-test-root-registry';

const originalMatchMediaDescriptor = Object.getOwnPropertyDescriptor(
  window,
  'matchMedia',
);

const usage: TaskDetailTokenUsage = {
  inputTokens: 80,
  outputTokens: 20,
  totalTokens: 100,
  costMicros: 0,
  currency: '',
  callCount: 1,
  leadAgentTokens: 100,
  subagentTokens: 0,
  middlewareTokens: 0,
  toolTokens: 0,
  modelAttributions: [],
};

const createDetailItems = (count: number) =>
  Array.from({ length: count }, (_, index) => ({
    canLocate: false,
    runID: `private-run-${index}`,
    createdAt: 1_720_000_000_000 + index,
    usage,
  }));

const createMatchMedia = () =>
  vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));

beforeEach(() => {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: createMatchMedia(),
    writable: true,
  });
});

afterEach(() => {
  if (originalMatchMediaDescriptor) {
    Object.defineProperty(window, 'matchMedia', originalMatchMediaDescriptor);
  } else {
    Reflect.deleteProperty(window, 'matchMedia');
  }
  document
    .querySelectorAll('.semi-portal')
    .forEach(portal => portal.parentElement?.removeChild(portal));
});

describe('task usage terminal UI contracts', () => {
  it('reopens with at most one 100-row batch before passive effects run', () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    const detailItems = createDetailItems(1000);
    const renderDrawer = (visible: boolean) =>
      root.render(
        <TaskUsageDetailDrawer
          visible={visible}
          tokenUsage={usage}
          detailItems={detailItems}
          viewMode="per_turn"
          onCancel={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );

    act(() => {
      renderDrawer(true);
    });
    for (let index = 0; index < 9; index += 1) {
      const showMore = Array.from(
        document.body.querySelectorAll<HTMLButtonElement>('button'),
      ).find(button => button.textContent?.includes('显示更多'));
      act(() => {
        showMore?.click();
      });
    }
    expect(document.body.querySelectorAll('time')).toHaveLength(1000);

    act(() => {
      renderDrawer(false);
    });
    act(() => {
      flushSync(() => {
        renderDrawer(true);
      });
      expect(document.body.querySelectorAll('time').length).toBeLessThanOrEqual(
        100,
      );
    });
  });

  it('does not derive detail rows until the hidden drawer is requested', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);
    const deriveDetailItems = vi.fn(() => createDetailItems(1));

    act(() => {
      root.render(
        <TaskUsagePopover
          scopeKey="thread-lazy"
          tokenUsage={usage}
          currentReplyUsage={usage}
          detailItems={[]}
          detailItemsFactory={deriveDetailItems}
          viewMode="per_turn"
          hasAssistantReply
        />,
      );
    });
    expect(deriveDetailItems).not.toHaveBeenCalled();

    await act(async () => {
      container
        .querySelector<HTMLButtonElement>('button.coze-prototype-token-usage')
        ?.click();
      await Promise.resolve();
    });
    expect(deriveDetailItems).not.toHaveBeenCalled();

    const detailButton = Array.from(
      document.body.querySelectorAll<HTMLButtonElement>('button'),
    ).find(button => button.textContent?.includes('查看用量明细'));
    await act(async () => {
      detailButton?.click();
      await Promise.resolve();
    });

    expect(deriveDetailItems).toHaveBeenCalledTimes(1);
    expect(document.body.querySelectorAll('time')).toHaveLength(1);
  });
});
