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

import type { ReactNode } from 'react';

import { act } from 'react-dom/test-utils';
import { renderToStaticMarkup } from 'react-dom/server';
import { createRoot, type Root } from 'react-dom/client';

import { GlobalLayoutSider } from '../sider';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('@coze-arch/bot-hooks', () => ({
  useRouteConfig: () => ({
    subMenu: () => <div>Secondary workspace menu</div>,
  }),
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror component names. */
vi.mock('@coze-arch/bot-icons', () => ({
  IconMenuLogo: () => <div>Main logo</div>,
}));

vi.mock('@coze-arch/coze-design', () => ({
  Divider: () => <hr />,
  Space: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

describe('GlobalLayoutSider', () => {
  beforeEach(() => {
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      value: {
        getItem: vi.fn(() => null),
        setItem: vi.fn(),
      },
    });
  });

  it('can hide the primary navigation while keeping the secondary menu', () => {
    const markup = renderToStaticMarkup(
      <GlobalLayoutSider hidePrimarySider={true} />,
    );

    expect(markup).toContain('Secondary workspace menu');
    expect(markup).not.toContain('Main logo');
  });

  it('collapses the secondary workspace menu width from a DeerFlow-style sidebar event', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <GlobalLayoutSider hidePrimarySider={true} subMenuDefaultWidth={300} />,
      );
      await Promise.resolve();
    });

    const subMenuPanel = container.querySelector(
      '[data-testid="global-layout-sub-menu-panel"]',
    ) as HTMLElement | null;

    expect(subMenuPanel).not.toBeNull();
    expect(subMenuPanel?.style.width).toBe('300px');
    expect(subMenuPanel?.getAttribute('data-collapsed')).toBe('false');

    act(() => {
      window.dispatchEvent(
        new CustomEvent('coze-workspace-submenu-collapse-change', {
          detail: {
            storageKey: 'workspace-submenu-width',
            collapsed: true,
          },
        }),
      );
    });

    expect(subMenuPanel?.style.width).toBe('56px');
    expect(subMenuPanel?.getAttribute('data-collapsed')).toBe('true');
    expect(localStorage.setItem).toHaveBeenCalledWith(
      'workspace-submenu-width:collapsed',
      'true',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
