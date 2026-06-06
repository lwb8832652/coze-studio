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

import { useNavigate } from 'react-router-dom';
import { type ReactNode, type FC } from 'react';

import { useShallow } from 'zustand/react/shallow';
import classNames from 'classnames';
import { useSpaceStore } from '@coze-foundation/space-store';
import { localStorageService } from '@coze-foundation/local-storage';
import { EVENT_NAMES, sendTeaEvent } from '@coze-arch/bot-tea';

export interface IWorkspaceListItem {
  icon?: ReactNode;
  activeIcon?: ReactNode;
  suffix?: ReactNode;
  title?: () => string;
  path?: string;
  dataTestId?: string;
  variant?: 'default' | 'primary';
}

interface IWorkspaceListItemProps extends IWorkspaceListItem {
  currentSubMenu?: string;
}

export const WorkspaceListItem: FC<IWorkspaceListItemProps> = ({
  icon,
  activeIcon,
  suffix,
  title,
  path,
  currentSubMenu,
  dataTestId,
  variant = 'default',
}) => {
  const navigate = useNavigate();
  const { spaceId } = useSpaceStore(
    useShallow(store => ({
      spaceId: store.space.id,
    })),
  );
  return spaceId ? (
    <div
      onClick={() => {
        sendTeaEvent(EVENT_NAMES.coze_space_sidenavi_ck, {
          item: title?.() || 'unknown-workspace-submenu',
          navi_type: 'second',
          need_login: true,
          have_access: true,
        });
        localStorageService.setValue('workspace-subMenu', path);
        navigate(`/space/${spaceId}/${path}`);
      }}
      className={classNames('coze-prototype-nav-item', {
        'coze-prototype-nav-item-primary': variant === 'primary',
        'coze-prototype-nav-item-active':
          path === currentSubMenu && variant !== 'primary',
      })}
      id={`workspace-submenu-${path}`}
      data-testid={dataTestId}
    >
      <div className="coze-prototype-nav-icon">
        {path === currentSubMenu ? activeIcon : icon}
      </div>
      <div
        className={classNames('coze-prototype-nav-title', {
          'coze-prototype-nav-title-primary': variant === 'primary',
        })}
      >
        {title?.()}
      </div>
      {suffix && variant !== 'primary' ? (
        <div className="coze-prototype-nav-suffix">{suffix}</div>
      ) : null}
    </div>
  ) : null;
};
