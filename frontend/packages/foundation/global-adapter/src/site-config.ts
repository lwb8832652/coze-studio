// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import {
  DEFAULT_SITE_CONFIG,
  type ISiteConfig,
  useCommonConfigStore,
} from '@coze-foundation/global-store';
import { I18n } from '@coze-arch/i18n';
import { setHtmlTitleSiteName } from '@coze-arch/bot-utils';

export { DEFAULT_SITE_CONFIG };

interface PublicSiteConfigResponse {
  site_name?: unknown;
  site_description?: unknown;
  site_logo_url?: unknown;
  favicon_url?: unknown;
  revision?: unknown;
}

const SITE_CONFIG_REQUEST_TIMEOUT_MS = 5_000;
const DEFAULT_FAVICON_URL = '/newx-favicon.png';

let siteConfigRefreshSequence = 0;

const readString = (value: unknown): string =>
  typeof value === 'string' ? value.trim() : '';

export const normalizeSiteConfig = (
  value?: PublicSiteConfigResponse | null,
): ISiteConfig => {
  const siteName = readString(value?.site_name);
  const siteDescription = readString(value?.site_description);
  const siteLogoUrl = readString(value?.site_logo_url);
  const faviconUrl = readString(value?.favicon_url);
  const revision = readString(value?.revision);

  if (!siteName) {
    return DEFAULT_SITE_CONFIG;
  }

  return {
    siteName,
    siteDescription: siteDescription || DEFAULT_SITE_CONFIG.siteDescription,
    siteLogoUrl,
    faviconUrl,
    revision,
  };
};

const upsertManagedMeta = (
  selector: string,
  create: () => HTMLElement,
): HTMLElement => {
  const existing = document.head.querySelector<HTMLElement>(selector);
  if (existing) {
    return existing;
  }
  const element = create();
  element.dataset.cozeSiteConfig = 'true';
  document.head.appendChild(element);
  return element;
};

const applyFaviconToDocument = (faviconUrl: string): void => {
  const iconLinks = Array.from(
    document.head.querySelectorAll<HTMLLinkElement>('link[rel="icon"]'),
  );
  const managedIcon = iconLinks.find(
    icon => icon.dataset.cozeSiteConfig === 'true',
  );
  const favicon = managedIcon ?? iconLinks[0] ?? document.createElement('link');
  favicon.rel = 'icon';
  favicon.dataset.cozeSiteConfig = 'true';
  favicon.removeAttribute('type');
  favicon.setAttribute('href', faviconUrl || DEFAULT_FAVICON_URL);
  if (!favicon.isConnected) {
    document.head.appendChild(favicon);
  }
  iconLinks.forEach(icon => {
    if (icon !== favicon) {
      icon.remove();
    }
  });
};

export const applySiteConfigToDocument = (config: ISiteConfig): void => {
  if (typeof document === 'undefined') {
    return;
  }

  const configuredLanguages = new Set(
    ['zh-CN', 'en', I18n.language].filter(Boolean),
  );
  configuredLanguages.forEach(language => {
    I18n.addResourceBundle(
      language,
      'translation',
      { platform_name: config.siteName },
      true,
      true,
    );
  });

  setHtmlTitleSiteName(config.siteName);
  document.title = config.siteName;
  const description = upsertManagedMeta(
    'meta[name="description"][data-coze-site-config]',
    () => {
      const element = document.createElement('meta');
      element.setAttribute('name', 'description');
      return element;
    },
  );
  description.setAttribute('content', config.siteDescription);

  applyFaviconToDocument(config.faviconUrl);
};

export const fetchSiteConfig = async (
  signal?: AbortSignal,
): Promise<ISiteConfig> => {
  const requestController = new AbortController();
  const abortFromCaller = () => requestController.abort(signal?.reason);
  if (signal?.aborted) {
    abortFromCaller();
  } else {
    signal?.addEventListener('abort', abortFromCaller, { once: true });
  }
  const timeout = setTimeout(
    () => requestController.abort(),
    SITE_CONFIG_REQUEST_TIMEOUT_MS,
  );

  try {
    const response = await fetch('/api/site/config', {
      credentials: 'include',
      signal: requestController.signal,
    });
    if (!response.ok) {
      throw new Error(`site configuration request failed: ${response.status}`);
    }
    return normalizeSiteConfig(
      (await response.json()) as PublicSiteConfigResponse,
    );
  } finally {
    clearTimeout(timeout);
    signal?.removeEventListener('abort', abortFromCaller);
  }
};

export const refreshSiteConfig = async (
  signal?: AbortSignal,
): Promise<ISiteConfig> => {
  const refreshSequence = ++siteConfigRefreshSequence;
  let config = useCommonConfigStore.getState().siteConfig;
  try {
    config = await fetchSiteConfig(signal);
  } catch (error) {
    if (signal?.aborted) {
      throw error;
    }
  }
  if (refreshSequence !== siteConfigRefreshSequence) {
    return useCommonConfigStore.getState().siteConfig;
  }
  useCommonConfigStore.getState().updateSiteConfig(config);
  applySiteConfigToDocument(config);
  return config;
};
