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

import { vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: (selector: (state: unknown) => unknown) =>
    selector({
      inited: true,
      loading: false,
      spaceList: [{ id: 'space-1' }],
    }),
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => {
  const Skeleton = ({ children }: { children: ReactNode }) => <>{children}</>;
  Skeleton.Paragraph = () => <div />;

  return {
    Skeleton,
    Space: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  };
});

vi.mock('../components/favorites-list', () => ({
  FavoritesList: () => <div>Favourites fallback</div>,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import { WorkspaceSubMenu } from '../index';

describe('WorkspaceSubMenu', () => {
  it('renders a custom bottom panel when provided', () => {
    const markup = renderToStaticMarkup(
      <WorkspaceSubMenu
        header={<div>Header</div>}
        menus={[]}
        bottomPanel={<div>我的任务</div>}
      />,
    );

    expect(markup).toContain('我的任务');
    expect(markup).not.toContain('Favourites fallback');
  });
});
