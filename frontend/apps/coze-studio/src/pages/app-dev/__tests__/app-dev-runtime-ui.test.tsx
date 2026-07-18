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

/* eslint-disable @typescript-eslint/require-await, max-params -- Runtime test doubles preserve async callback contracts. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { RuntimeToolbar } from '../components/runtime-toolbar';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('AppDev runtime toolbar states', () => {
  let root: Root | undefined;
  let container: HTMLDivElement | undefined;

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
    }
    root = undefined;
    container = undefined;
  });

  const render = async (
    status:
      | 'starting'
      | 'running'
      | 'recovering'
      | 'stopping'
      | 'cleanup_pending'
      | 'stopped'
      | 'error',
    canStart: boolean,
  ) => {
    container = document.createElement('div');
    root = createRoot(container);
    await act(async () => {
      root?.render(
        <RuntimeToolbar
          runtime={{
            generation: 1,
            status,
            canStart,
            recovering: status === 'recovering',
            stopping: status === 'cleanup_pending',
          }}
          build={{
            generation: 1,
            state: 'idle',
            releaseAvailable: false,
            size: 0,
            stale: false,
          }}
          onStart={vi.fn()}
          onRestart={vi.fn()}
          onStop={vi.fn()}
          onRefresh={vi.fn()}
          onBuild={vi.fn()}
          onDownloadRelease={vi.fn()}
          onExport={vi.fn()}
        />,
      );
    });
  };

  it('renders recovery and cleanup as readonly lifecycle states', async () => {
    await render('recovering', false);
    expect(container?.textContent).toContain('恢复中');
    expect(
      Array.from(container?.querySelectorAll('button') || []).every(
        button => button.textContent === '刷新' || button.disabled,
      ),
    ).toBe(true);

    act(() => root?.unmount());
    root = undefined;
    await render('cleanup_pending', false);
    expect(container?.textContent).toContain('清理中');
  });

  it.each([
    ['starting', false, '启动中...', true],
    ['running', false, '重启', false],
    ['recovering', false, '恢复中...', true],
    ['stopping', false, '停止中...', true],
    ['cleanup_pending', false, '清理中...', true],
    ['stopped', true, '启动环境', false],
    ['error', true, '启动环境', false],
  ] as const)(
    'renders %s controls as a complete lifecycle state',
    async (status, canStart, actionText, disabled) => {
      await render(status, canStart);
      const action = Array.from(
        container?.querySelectorAll('button') || [],
      ).find(button => button.textContent === actionText);
      expect(action).toBeDefined();
      expect(action?.disabled).toBe(disabled);
    },
  );

  it('enables start only when the backend projection allows it', async () => {
    await render('stopped', false);
    expect(
      Array.from(container?.querySelectorAll('button') || []).find(button =>
        button.textContent?.includes('启动环境'),
      )?.disabled,
    ).toBe(true);
  });
});
