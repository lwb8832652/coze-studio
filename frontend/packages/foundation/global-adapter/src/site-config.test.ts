// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  DEFAULT_SITE_CONFIG,
  useCommonConfigStore,
} from '@coze-foundation/global-store';
import { I18n } from '@coze-arch/i18n';

import {
  applySiteConfigToDocument,
  fetchSiteConfig,
  normalizeSiteConfig,
  refreshSiteConfig,
} from './site-config';

describe('site configuration bootstrap', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    useCommonConfigStore.getState().updateSiteConfig(DEFAULT_SITE_CONFIG);
    document.head
      .querySelectorAll('[data-coze-site-config]')
      .forEach(node => node.remove());
  });

  it('falls back to product defaults for an incomplete public response', () => {
    expect(normalizeSiteConfig({ site_name: '  ' })).toEqual(
      DEFAULT_SITE_CONFIG,
    );
  });

  it('updates title, description and favicon without creating duplicates', () => {
    const addResourceBundleSpy = vi
      .spyOn(I18n, 'addResourceBundle')
      .mockImplementation(() => undefined);
    const config = normalizeSiteConfig({
      site_name: 'Acme AI',
      site_description: 'Acme intelligent workspace',
      site_logo_url: 'https://assets.example.com/logo.png',
      favicon_url: 'https://assets.example.com/favicon.png',
      revision: 'revision-1',
    });

    applySiteConfigToDocument(config);
    applySiteConfigToDocument(config);

    expect(document.title).toBe('Acme AI');
    expect(
      document.head.querySelectorAll(
        'meta[name="description"][data-coze-site-config]',
      ),
    ).toHaveLength(1);
    expect(
      document.head.querySelector<HTMLLinkElement>(
        'link[rel="icon"][data-coze-site-config]',
      )?.href,
    ).toBe('https://assets.example.com/favicon.png');
    expect(addResourceBundleSpy).toHaveBeenCalledWith(
      'zh-CN',
      'translation',
      { platform_name: 'Acme AI' },
      true,
      true,
    );
    expect(addResourceBundleSpy).toHaveBeenCalledWith(
      'en',
      'translation',
      { platform_name: 'Acme AI' },
      true,
      true,
    );
  });

  it('fetches and normalizes the public site configuration', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () =>
        Promise.resolve({
          site_name: ' Acme AI ',
          site_description: ' Acme workspace ',
          site_logo_url: ' https://assets.example.com/logo.png ',
          favicon_url: ' https://assets.example.com/favicon.png ',
          revision: ' revision-1 ',
        }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(fetchSiteConfig()).resolves.toEqual({
      siteName: 'Acme AI',
      siteDescription: 'Acme workspace',
      siteLogoUrl: 'https://assets.example.com/logo.png',
      faviconUrl: 'https://assets.example.com/favicon.png',
      revision: 'revision-1',
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/site/config', {
      credentials: 'include',
      signal: expect.any(AbortSignal),
    });
  });

  it('bounds a stalled public configuration request', async () => {
    vi.useFakeTimers();
    let requestSignal: AbortSignal | undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        requestSignal = init?.signal as AbortSignal | undefined;
        return new Promise((_resolve, reject) => {
          requestSignal?.addEventListener(
            'abort',
            () => reject(new DOMException('Aborted', 'AbortError')),
            { once: true },
          );
        });
      }),
    );

    const request = fetchSiteConfig();
    const rejection = expect(request).rejects.toMatchObject({
      name: 'AbortError',
    });
    expect(requestSignal).toBeInstanceOf(AbortSignal);
    expect(requestSignal?.aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(5_000);
    expect(requestSignal?.aborted).toBe(true);
    await rejection;
  });

  it('falls back safely and updates the shared store after a network error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    await expect(refreshSiteConfig()).resolves.toEqual(DEFAULT_SITE_CONFIG);
    expect(useCommonConfigStore.getState().siteConfig).toEqual(
      DEFAULT_SITE_CONFIG,
    );
    expect(document.title).toBe(DEFAULT_SITE_CONFIG.siteName);
  });
});
