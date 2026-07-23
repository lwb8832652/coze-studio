// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment jsdom

import React from 'react';

import { describe, expect, it } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot } from 'react-dom/client';

import { UILayout } from '../index';

vi.mock('@coze-arch/i18n/i18n-provider', async () => {
  const { createContext } = await import('react');
  return {
    i18nContext: createContext({
      i18n: {
        t: () => '扣子',
      },
    }),
  };
});

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const flushHelmet = async () => {
  await act(
    () =>
      new Promise(resolve => {
        setTimeout(resolve, 30);
      }),
  );
};

describe('UILayout browser title', () => {
  it('preserves the globally configured title when no page title is supplied', async () => {
    document.title = 'NewX AI';
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(() => {
      root.render(<UILayout>页面内容</UILayout>);
    });
    await flushHelmet();

    expect(document.title).toBe('NewX AI');

    await act(() => root.unmount());
    container.remove();
  });

  it('applies an explicit page title', async () => {
    document.title = 'NewX AI';
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(() => {
      root.render(<UILayout title="资源库 - NewX AI">页面内容</UILayout>);
    });
    await flushHelmet();

    expect(document.title).toBe('资源库 - NewX AI');

    await act(() => root.unmount());
    container.remove();
  });
});
