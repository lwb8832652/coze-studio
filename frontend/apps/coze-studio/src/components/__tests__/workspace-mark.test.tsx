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

import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import {
  DEFAULT_SITE_CONFIG,
  useCommonConfigStore,
} from '@coze-foundation/global-store';

import { WorkspaceMark } from '../workspace-mark';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('WorkspaceMark', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    useCommonConfigStore.getState().updateSiteConfig({
      ...DEFAULT_SITE_CONFIG,
      siteLogoUrl: 'https://assets.example.com/missing-logo.png',
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    useCommonConfigStore.getState().updateSiteConfig(DEFAULT_SITE_CONFIG);
  });

  it('falls back after a logo load error and retries a changed URL', () => {
    act(() => {
      root.render(<WorkspaceMark />);
    });

    act(() => {
      Simulate.error(container.querySelector<HTMLImageElement>('img')!);
    });

    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('svg')).not.toBeNull();

    act(() => {
      useCommonConfigStore.getState().updateSiteConfig({
        ...DEFAULT_SITE_CONFIG,
        siteLogoUrl: 'https://assets.example.com/recovered-logo.png',
      });
    });

    expect(container.querySelector<HTMLImageElement>('img')?.src).toBe(
      'https://assets.example.com/recovered-logo.png',
    );
  });
});
