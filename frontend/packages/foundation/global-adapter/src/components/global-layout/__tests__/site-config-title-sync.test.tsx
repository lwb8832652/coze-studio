// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment jsdom
/* eslint-disable @typescript-eslint/naming-convention -- mocked React exports preserve their public PascalCase component names. */

import { MemoryRouter, useNavigate } from 'react-router-dom';
import { type PropsWithChildren } from 'react';

import { afterAll, beforeAll } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useCommonConfigStore } from '@coze-foundation/global-store';

import { GlobalLayout } from '../index';

vi.mock('ahooks', () => ({
  useUpdate: () => vi.fn(),
}));

vi.mock('@coze-foundation/browser-upgrade-banner', () => ({
  BrowserUpgradeWrap: ({ children }: PropsWithChildren) => children,
}));

vi.mock('@coze-foundation/layout', () => ({
  GlobalLayout: ({ children }: PropsWithChildren) => children,
}));

vi.mock('@coze-arch/i18n/i18n-provider', () => ({
  I18nProvider: ({ children }: PropsWithChildren) => children,
}));

vi.mock('@coze-arch/i18n', () => ({
  I18n: {
    language: 'zh-CN',
    addResourceBundle: vi.fn(),
    setLang: vi.fn(),
  },
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => null,
}));

vi.mock('@coze-arch/coze-design/locales', () => ({
  en_US: {},
  zh_CN: {},
}));

vi.mock('@coze-arch/coze-design', () => ({
  CDLocaleProvider: ({ children }: PropsWithChildren) => children,
  ThemeProvider: ({ children }: PropsWithChildren) => children,
  enUS: {},
  zhCN: {},
}));

vi.mock('@coze-arch/bot-semi', () => ({
  LocaleProvider: ({ children }: PropsWithChildren) => children,
  UIDocumentTitle: ({ title }: { title: string }) => (
    <span data-testid="global-document-title" data-title-order="title">
      {title}
    </span>
  ),
}));

vi.mock('@/components/global-layout-composed', () => ({
  GlobalLayoutComposed: ({ children }: PropsWithChildren) => (
    <>
      <span data-title-order="content">业务内容</span>
      {children}
    </>
  ),
}));

const RouteHarness = () => {
  const navigate = useNavigate();
  return (
    <button type="button" onClick={() => navigate('/library')}>
      切换路由
    </button>
  );
};

describe('site configuration title synchronization', () => {
  beforeAll(() => {
    vi.stubGlobal('IS_BOE', false);
  });

  afterAll(() => {
    vi.unstubAllGlobals();
  });

  it('restores the configured site name after a route transition', async () => {
    useCommonConfigStore.getState().updateSiteConfig({
      siteName: 'Acme AI',
      siteDescription: 'Acme workspace',
      siteLogoUrl: '',
      faviconUrl: '',
      revision: 'revision-1',
    });
    document.title = '扣子';

    render(
      <MemoryRouter initialEntries={['/chats/new']}>
        <GlobalLayout />
        <RouteHarness />
      </MemoryRouter>,
    );

    expect(screen.getByTestId('global-document-title').textContent).toBe(
      'Acme AI',
    );
    expect(
      Array.from(document.querySelectorAll('[data-title-order]')).map(node =>
        node.getAttribute('data-title-order'),
      ),
    ).toEqual(['content', 'title']);
    await waitFor(() => expect(document.title).toBe('Acme AI'));

    document.title = '扣子';
    fireEvent.click(screen.getByRole('button', { name: '切换路由' }));

    await waitFor(() => expect(document.title).toBe('Acme AI'));
  });
});
