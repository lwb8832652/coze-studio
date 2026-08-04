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

import { afterEach, describe, expect, it, vi } from 'vitest';

import { verifySiteAssetPreview } from '../site-asset-preview';

describe('verifySiteAssetPreview', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('resolves only after the image URL loads', async () => {
    const assignedURLs: string[] = [];
    class LoadableImage {
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;

      set src(value: string) {
        assignedURLs.push(value);
        queueMicrotask(() => this.onload?.());
      }
    }
    vi.stubGlobal('Image', LoadableImage);

    await expect(
      verifySiteAssetPreview('https://assets.example.com/logo.png'),
    ).resolves.toBeUndefined();
    expect(assignedURLs).toEqual(['https://assets.example.com/logo.png']);
  });

  it('rejects when the image URL cannot be loaded', async () => {
    class MissingImage {
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;

      set src(_value: string) {
        queueMicrotask(() => this.onerror?.());
      }
    }
    vi.stubGlobal('Image', MissingImage);

    await expect(
      verifySiteAssetPreview('https://assets.example.com/missing.png'),
    ).rejects.toThrow('site asset preview is unavailable');
  });

  it('rejects when the image URL does not settle before the timeout', async () => {
    vi.useFakeTimers();
    class StalledImage {
      onload: (() => void) | null = null;
      onerror: (() => void) | null = null;

      set src(_value: string) {}
    }
    vi.stubGlobal('Image', StalledImage);

    const preview = verifySiteAssetPreview(
      'https://assets.example.com/stalled.png',
    );
    const rejection = expect(preview).rejects.toThrow(
      'site asset preview timed out',
    );
    await vi.advanceTimersByTimeAsync(5_000);
    await rejection;
  });
});
