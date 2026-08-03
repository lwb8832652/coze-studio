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

import { localStorageService } from '@coze-foundation/local-storage';

interface FallbackWorkspaceURLOptions {
  fallbackSpaceID: string;
  fallbackSpaceMenu: string;
  checkSpaceID: (id: string) => boolean;
  restoreLastSubMenu?: boolean;
}

export const getFallbackWorkspaceURL = async ({
  fallbackSpaceID,
  fallbackSpaceMenu,
  checkSpaceID,
  restoreLastSubMenu = true,
}: FallbackWorkspaceURLOptions) => {
  const storedSpaceID =
    await localStorageService.getValueSync('workspace-spaceId');
  const targetSpaceID = storedSpaceID ?? fallbackSpaceID;
  const storedSpaceSubMenu = restoreLastSubMenu
    ? await localStorageService.getValueSync('workspace-subMenu')
    : undefined;
  const targetSpaceSubMenu = storedSpaceSubMenu ?? fallbackSpaceMenu;
  const validSpaceID =
    targetSpaceID && checkSpaceID(targetSpaceID)
      ? targetSpaceID
      : fallbackSpaceID;

  return `/space/${validSpaceID}/${targetSpaceSubMenu}`;
};
