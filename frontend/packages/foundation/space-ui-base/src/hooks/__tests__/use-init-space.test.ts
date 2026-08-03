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

import { vi } from 'vitest';

const { mockGetValueSync } = vi.hoisted(() => ({
  mockGetValueSync: vi.fn(),
}));

vi.mock('@coze-foundation/local-storage', () => ({
  localStorageService: {
    getValueSync: mockGetValueSync,
  },
}));

import { getFallbackWorkspaceURL } from '../space-landing';

describe('getFallbackWorkspaceURL', () => {
  beforeEach(() => {
    mockGetValueSync.mockReset();
    mockGetValueSync.mockImplementation((key: string) => {
      if (key === 'workspace-spaceId') {
        return 'space-2';
      }
      if (key === 'workspace-subMenu') {
        return 'develop';
      }
      return undefined;
    });
  });

  it('keeps the last valid workspace but uses the configured homepage', async () => {
    await expect(
      getFallbackWorkspaceURL({
        fallbackSpaceID: 'space-1',
        fallbackSpaceMenu: 'chats/new',
        checkSpaceID: id => id === 'space-1' || id === 'space-2',
        restoreLastSubMenu: false,
      }),
    ).resolves.toBe('/space/space-2/chats/new');
  });

  it('preserves the shared initializer default of restoring the last submenu', async () => {
    await expect(
      getFallbackWorkspaceURL({
        fallbackSpaceID: 'space-1',
        fallbackSpaceMenu: 'develop',
        checkSpaceID: id => id === 'space-1' || id === 'space-2',
      }),
    ).resolves.toBe('/space/space-2/develop');
  });
});
