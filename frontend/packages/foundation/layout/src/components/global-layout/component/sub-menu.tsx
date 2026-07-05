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

import { type FC, Suspense, useState, useCallback, useEffect } from 'react';

import { useRouteConfig } from '@coze-arch/bot-hooks';

import styles from '../side-sheet.module.less';

const STORAGE_KEY = 'submenu-width';
const MIN_WIDTH = 200;
const MAX_WIDTH = 380;
const DEFAULT_WIDTH = MIN_WIDTH;
const COLLAPSED_WIDTH = 56;
const SUBMENU_COLLAPSE_EVENT = 'coze-workspace-submenu-collapse-change';

interface SubMenuCollapseEventDetail {
  storageKey?: string;
  collapsed?: boolean;
}

const readStoredCollapsed = (storageKey: string) => {
  if (typeof window === 'undefined') {
    return false;
  }

  try {
    return localStorage.getItem(`${storageKey}:collapsed`) === 'true';
  } catch (error) {
    console.warn('Failed to read submenu collapsed state.', error);

    return false;
  }
};

const writeStoredCollapsed = (storageKey: string, collapsed: boolean) => {
  try {
    localStorage.setItem(`${storageKey}:collapsed`, String(collapsed));
  } catch (error) {
    console.warn('Failed to persist submenu collapsed state.', error);
  }
};

interface SubMenuProps {
  defaultWidth?: number;
  resizable?: boolean;
  storageKey?: string;
}

export const SubMenu: FC<SubMenuProps> = ({
  defaultWidth = DEFAULT_WIDTH,
  resizable = true,
  storageKey = STORAGE_KEY,
}) => {
  const config = useRouteConfig();
  const { subMenu: SubMenuComponent } = config;
  const [width, setWidth] = useState(() => {
    if (!resizable) {
      return defaultWidth;
    }

    const savedWidth = localStorage.getItem(storageKey);
    return savedWidth
      ? Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, Number(savedWidth)))
      : defaultWidth;
  });
  const [collapsed, setCollapsed] = useState(() =>
    readStoredCollapsed(storageKey),
  );
  const effectiveWidth = collapsed ? COLLAPSED_WIDTH : width;

  useEffect(() => {
    const handleCollapseChange = (event: Event) => {
      const { detail } = event as CustomEvent<SubMenuCollapseEventDetail>;

      if (detail.storageKey && detail.storageKey !== storageKey) {
        return;
      }

      const nextCollapsed = Boolean(detail.collapsed);
      setCollapsed(nextCollapsed);
      writeStoredCollapsed(storageKey, nextCollapsed);
    };

    window.addEventListener(SUBMENU_COLLAPSE_EVENT, handleCollapseChange);

    return () => {
      window.removeEventListener(SUBMENU_COLLAPSE_EVENT, handleCollapseChange);
    };
  }, [storageKey]);

  const handleMouseDown = useCallback(
    (event: React.MouseEvent) => {
      if (!resizable || collapsed) {
        return;
      }

      event.preventDefault();
      const startX = event.pageX;
      const startWidth = width;

      const handleMouseMove = (e: MouseEvent) => {
        const newWidth = Math.min(
          MAX_WIDTH,
          Math.max(MIN_WIDTH, startWidth + e.pageX - startX),
        );
        setWidth(newWidth);
        localStorage.setItem(storageKey, String(newWidth));
      };

      const handleMouseUp = () => {
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
      };

      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    },
    [collapsed, resizable, storageKey, width],
  );

  if (!SubMenuComponent) {
    return null;
  }

  return (
    <div className="relative flex h-full shrink-0 flex-row">
      <div
        className="box-border flex h-full flex-col overflow-auto px-[6px] py-[12px] transition-[width] duration-200 ease-linear"
        data-testid="global-layout-sub-menu-panel"
        data-collapsed={String(collapsed)}
        style={{ width: `${effectiveWidth}px` }}
      >
        <Suspense>
          <SubMenuComponent />
        </Suspense>
      </div>
      {resizable ? (
        <div
          className={styles['sub-menu-resize']}
          onMouseDown={handleMouseDown}
        >
          <div className={styles['sub-menu-resize-line']}></div>
        </div>
      ) : null}
    </div>
  );
};
