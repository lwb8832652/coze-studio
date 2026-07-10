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
          runtime={{ status: 'running', previewUrl: 'javascript:alert(1)' }}
          onRefresh={vi.fn()}
        />,
      );
    });

    expect(container.querySelector('iframe')).toBeNull();
    expect(container.textContent).toContain('预览地址不安全');
  });

  it('sandboxes the embedded preview', async () => {
    container = document.createElement('div');
    root = createRoot(container);

    await act(async () => {
      root?.render(
        <PreviewPanel
          runtime={{
            status: 'running',
            previewUrl: 'https://preview.example.com/app',
          }}
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
});
