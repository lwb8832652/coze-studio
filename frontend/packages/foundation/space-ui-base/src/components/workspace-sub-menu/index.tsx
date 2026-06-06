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

import { type ReactNode } from 'react';

import { useShallow } from 'zustand/react/shallow';
import { useSpaceStore } from '@coze-foundation/space-store';
import { Skeleton } from '@coze-arch/coze-design';

import { type IWorkspaceListItem } from './components/workspace-list-item';
import { WorkspaceList } from './components/workspace-list';
import { FavoritesList } from './components/favorites-list';

import './components/list.css';

interface IWorkspaceSubMenuProps {
  header: ReactNode;
  menus: Array<IWorkspaceListItem>;
  currentSubMenu?: string;
  bottomPanel?: ReactNode;
  footer?: ReactNode;
}

export const WorkspaceSubMenu = ({
  header,
  menus,
  currentSubMenu,
  bottomPanel,
  footer,
}: IWorkspaceSubMenuProps) => {
  const { spaceList, loading } = useSpaceStore(
    useShallow(state => ({
      currentSpace: state.space,
      spaceList: state.spaceList,
      loading: !!state.loading || !state.inited,
    })),
  );

  const hasSpace = spaceList.length > 0;
  const lowerPanel = bottomPanel ?? <FavoritesList />;

  return (
    <Skeleton loading={loading} active placeholder={<Skeleton.Paragraph />}>
      <div className="coze-prototype-sidebar">
        <div className="w-full flex-none">{header}</div>
        {hasSpace ? (
          <>
            <div className="w-full flex-none">
              <WorkspaceList menus={menus} currentSubMenu={currentSubMenu} />
            </div>
            <div className="mt-[16px] min-h-0 w-full flex-1 overflow-y-auto">
              {lowerPanel}
            </div>
            {footer ? <div className="w-full flex-none">{footer}</div> : null}
          </>
        ) : null}
      </div>
    </Skeleton>
  );
};

export { IWorkspaceListItem };
