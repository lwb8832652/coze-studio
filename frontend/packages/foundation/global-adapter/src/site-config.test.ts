// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  DEFAULT_SITE_CONFIG,
  useCommonConfigStore,
} from '@coze-foundation/global-store';
import { I18n } from '@coze-arch/i18n';

const setHtmlTitleSiteName = vi.hoisted(() => vi.fn());

vi.mock('@coze-arch/bot-utils', () => ({ setHtmlTitleSiteName }));

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
    setHtmlTitleSiteName.mockReset();
    useCommonConfigStore.getState().updateSiteConfig(DEFAULT_SITE_CONFIG);
    document.head
      .querySelectorAll('[data-coze-site-config]')
      .forEach(node => node.remove());
    document.head
      .querySelectorAll('link[rel="icon"]')
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
    expect(setHtmlTitleSiteName).toHaveBeenCalledWith('Acme AI');
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

  it('owns one favicon link and restores the default when unset', () => {
    const staticIcon = document.createElement('link');
    staticIcon.rel = 'icon';
    staticIcon.href = '/favicon.png';
    document.head.appendChild(staticIcon);
    const customConfig = {
      ...DEFAULT_SITE_CONFIG,
      faviconUrl: 'https://assets.example.com/favicon.png',
    };

    applySiteConfigToDocument(customConfig);

    let icons =
      document.head.querySelectorAll<HTMLLinkElement>('link[rel="icon"]');
    expect(icons).toHaveLength(1);
    expect(icons[0]?.href).toBe('https://assets.example.com/favicon.png');
    expect(icons[0]?.dataset.cozeSiteConfig).toBe('true');

    applySiteConfigToDocument({ ...customConfig, faviconUrl: '' });

    icons = document.head.querySelectorAll<HTMLLinkElement>('link[rel="icon"]');
    expect(icons).toHaveLength(1);
    expect(icons[0]?.getAttribute('href')).toBe('/newx-favicon.png');
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

  it('preserves the last successful configuration after a network error', async () => {
    const configuredSite = {
      ...DEFAULT_SITE_CONFIG,
      siteName: 'Acme AI',
      siteDescription: 'Acme intelligent workspace',
      siteLogoUrl: 'https://assets.example.com/logo.png',
      faviconUrl: 'https://assets.example.com/favicon.png',
      revision: 'revision-1',
    };
    useCommonConfigStore.getState().updateSiteConfig(configuredSite);
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    await expect(refreshSiteConfig()).resolves.toEqual(configuredSite);
    expect(useCommonConfigStore.getState().siteConfig).toEqual(configuredSite);
    expect(document.title).toBe(configuredSite.siteName);
  });

  it('keeps a newer refresh result when an older request resolves last', async () => {
    type SiteConfigResponse = Pick<Response, 'ok' | 'json'>;
    const responseFor = (siteName: string): SiteConfigResponse => ({
      ok: true,
      json: () => Promise.resolve({ site_name: siteName }),
    });
    let resolveFirst: (response: SiteConfigResponse) => void = () => undefined;
    let resolveSecond: (response: SiteConfigResponse) => void = () => undefined;
    const firstResponse = new Promise<SiteConfigResponse>(resolve => {
      resolveFirst = resolve;
    });
    const secondResponse = new Promise<SiteConfigResponse>(resolve => {
      resolveSecond = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockImplementationOnce(() => firstResponse)
        .mockImplementationOnce(() => secondResponse),
    );

    const firstRefresh = refreshSiteConfig();
    const secondRefresh = refreshSiteConfig();
    resolveSecond(responseFor('Newest AI'));
    await expect(secondRefresh).resolves.toMatchObject({
      siteName: 'Newest AI',
    });
    resolveFirst(responseFor('Older AI'));
    await expect(firstRefresh).resolves.toMatchObject({
      siteName: 'Newest AI',
    });
    expect(useCommonConfigStore.getState().siteConfig.siteName).toBe(
      'Newest AI',
    );
  });
});
