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

import { renderToStaticMarkup } from 'react-dom/server';

import { GlobalLayoutSider } from '../sider';

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
});
