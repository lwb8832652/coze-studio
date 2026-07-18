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

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { normalizeAppDevPreviewUrl } from '../utils/preview-url';
import { PreviewPanel } from '../components/preview-panel';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('AppDev preview security', () => {
  let root: Root | undefined;
  let container: HTMLDivElement | undefined;

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
      root = undefined;
    }
    container = undefined;
  });

  it('accepts HTTPS and local debug URLs only', () => {
    expect(normalizeAppDevPreviewUrl('https://preview.example.com/app')).toBe(
      'https://preview.example.com/app',
    );
    expect(normalizeAppDevPreviewUrl('http://127.0.0.1:3000/')).toBe(
      'http://127.0.0.1:3000/',
    );
    expect(
      normalizeAppDevPreviewUrl('http://attacker.example.com/app'),
    ).toBeUndefined();
    expect(normalizeAppDevPreviewUrl('javascript:alert(1)')).toBeUndefined();
    expect(
      normalizeAppDevPreviewUrl(
        'https://app.example.com/preview',
        'https://app.example.com',
      ),
    ).toBeUndefined();
  });

  it('does not render an unsafe preview URL', async () => {
    container = document.createElement('div');
    root = createRoot(container);

    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            generation: 1,
            status: 'running',
            canStart: false,
            recovering: false,
            stopping: false,
          }}
          onRefresh={vi.fn()}
        />,
      );
    });

    expect(container.querySelector('iframe')).toBeNull();
    expect(container.textContent).toContain('预览地址尚未就绪');
  });

  it('sandboxes the embedded preview', async () => {
    container = document.createElement('div');
    root = createRoot(container);

    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            generation: 1,
            status: 'running',
            canStart: false,
            recovering: false,
            stopping: false,
          }}
          trustedPreviewUrl={normalizeAppDevPreviewUrl(
            'https://preview.example.com/app',
          )}
          onRefresh={vi.fn()}
        />,
      );
    });

    const iframe = container.querySelector('iframe');
    expect(iframe?.getAttribute('sandbox')).toBe(
      'allow-scripts allow-same-origin allow-forms allow-modals allow-popups allow-downloads',
    );
    expect(iframe?.getAttribute('referrerpolicy')).toBe('no-referrer');
  });

  it('traps fullscreen focus, closes on Escape, and restores its trigger', async () => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            generation: 1,
            status: 'running',
            canStart: false,
            recovering: false,
            stopping: false,
          }}
          trustedPreviewUrl={normalizeAppDevPreviewUrl(
            'https://preview.example.com/app',
          )}
          onRefresh={vi.fn()}
        />,
      );
    });
    const trigger = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '全屏',
    )!;
    trigger.focus();
    await act(async () => trigger.click());
    const dialog = container.querySelector<HTMLElement>('[role="dialog"]')!;
    const close = Array.from(dialog.querySelectorAll('button')).find(
      button => button.textContent === '关闭',
    )!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(document.activeElement).toBe(close);

    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    });
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    container.remove();
  });

  it('exposes device and design mode toggles with aria-pressed', async () => {
    container = document.createElement('div');
    root = createRoot(container);
    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            generation: 1,
            status: 'running',
            canStart: false,
            recovering: false,
            stopping: false,
          }}
          trustedPreviewUrl={normalizeAppDevPreviewUrl(
            'https://preview.example.com/app',
          )}
          onRefresh={vi.fn()}
          onOpenDesignMode={vi.fn()}
        />,
      );
    });
    const desktop = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '桌面',
    );
    const mobile = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '手机',
    );
    const design = container.querySelector<HTMLButtonElement>(
      '.app-dev-preview-panel__design-status',
    );
    expect(desktop?.getAttribute('aria-pressed')).toBe('true');
    expect(mobile?.getAttribute('aria-pressed')).toBe('false');
    expect(design?.getAttribute('aria-pressed')).toBe('false');
  });

  it('shows a safe empty state when a running projection has no preview URL', async () => {
    container = document.createElement('div');
    root = createRoot(container);
    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            generation: 2,
            status: 'running',
            canStart: false,
            recovering: false,
            stopping: false,
          }}
          onRefresh={vi.fn()}
        />,
      );
    });
    expect(container.querySelector('iframe')).toBeNull();
    expect(container.textContent).toContain('预览地址尚未就绪');
  });

  it.each([
    ['stopped', '开发环境未启动'],
    ['starting', '正在启动开发环境'],
    ['recovering', '正在恢复开发环境'],
    ['stopping', '正在停止开发环境'],
    ['cleanup_pending', '正在清理运行资源'],
    ['error', '预览异常'],
  ] as const)(
    'renders the %s runtime empty state safely',
    async (status, title) => {
      if (root) {
        act(() => root?.unmount());
      }
      container = document.createElement('div');
      root = createRoot(container);
      await act(async () => {
        root?.render(
          <PreviewPanel
            runtime={{
              generation: 3,
              status,
              canStart: status === 'stopped' || status === 'error',
              recovering: status === 'recovering',
              stopping: status === 'stopping' || status === 'cleanup_pending',
            }}
            onRefresh={vi.fn()}
          />,
        );
      });
      expect(container.querySelector('iframe')).toBeNull();
      expect(container.textContent).toContain(title);
    },
  );

  it('shows explicit recovery and cleanup empty states without an iframe', async () => {
    container = document.createElement('div');
    root = createRoot(container);
    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            generation: 3,
            status: 'recovering',
            canStart: false,
            recovering: true,
            stopping: false,
          }}
          onRefresh={vi.fn()}
        />,
      );
    });
    expect(container.querySelector('iframe')).toBeNull();
    expect(container.textContent).toContain('正在恢复');
  });
});
