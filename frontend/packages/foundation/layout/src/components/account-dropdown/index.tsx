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

import {
  Fragment,
  type FC,
  isValidElement,
  type ReactNode,
  type PropsWithChildren,
} from 'react';

import classNames from 'classnames';
import { useUserInfo } from '@coze-foundation/account-adapter';
import { IconCozPeopleFill } from '@coze-arch/coze-design/icons';
import { Badge, CozAvatar, Dropdown } from '@coze-arch/coze-design';

import { reportNavClick } from '../global-layout/utils';
import { type LayoutAccountMenuItem } from '../global-layout/types';

import style from './index.module.less';

function isReactNode(value: unknown): value is ReactNode {
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'number' ||
    typeof value === 'boolean' ||
    isValidElement(value) ||
    Array.isArray(value)
  ) {
    return true;
  }
  return false;
}

export const GlobalLayoutAccountDropdown: FC<
  PropsWithChildren<{
    menus?: LayoutAccountMenuItem[];
    userBadge?: ReactNode;
    userTips?: ReactNode;
    disableVisibleChange?: boolean;
    visible?: boolean;
    onVisibleChange?: (visible: boolean) => void;
  }>
> = ({
  menus,
  userBadge = null,
  userTips = null,
  children,
  disableVisibleChange,
  visible,
  onVisibleChange,
}) => {
  const userInfo = useUserInfo();

  if (!userInfo) {
    return null;
  }
  return (
    <>
      <Dropdown
        trigger={disableVisibleChange ? 'custom' : 'click'}
        clickToHide={!disableVisibleChange}
        position={'rightBottom'}
        visible={disableVisibleChange ? visible : undefined}
        onVisibleChange={nextVisible => {
          if (!disableVisibleChange) {
            queueMicrotask(() => onVisibleChange?.(nextVisible));
          }
        }}
        render={
          <Dropdown.Menu
            className={classNames(style.menu, 'w-[250px]')}
            mode="menu"
          >
            {menus?.map((item, index) =>
              isReactNode(item) ? (
                <Fragment key={`custom-menu-${index}`}>{item}</Fragment>
              ) : (
                <Dropdown.Item
                  key={item.title}
                  onClick={() => {
                    reportNavClick(item.title);
                    queueMicrotask(() => {
                      item.onClick();
                    });
                  }}
                  data-testid={item.dataTestId}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center">
                      <div className="mr-[8px] flex items-center">
                        {item.prefixIcon}
                      </div>
                      <div>{item.title}</div>
                    </div>
                    <div className="flex items-center">{item.extra}</div>
                  </div>
                </Dropdown.Item>
              ),
            )}
          </Dropdown.Menu>
        }
      >
        <button
          type="button"
          className={classNames(
            'relative',
            'p-[4px] rounded-[8px] transition-colors hover:coz-mg-secondary-hovered',
            'leading-none',
            'border-0 bg-transparent cursor-pointer',
            visible && 'coz-mg-secondary-hovered',
          )}
          aria-label="账号菜单"
          aria-haspopup="menu"
          aria-expanded={visible}
          data-testid="layout_avatar-menu-button"
        >
          <Badge
            position="rightBottom"
            countStyle={{
              right: 6,
              bottom: 6,
            }}
            count={userBadge}
          >
            <CozAvatar
              src={userInfo.avatar_url}
              className={classNames('w-[32px] h-[32px] rounded-full')}
              type="person"
              style={{
                color: '#fff',
                backgroundColor: 'var(--newx-color-accent, #14804a)',
              }}
            >
              <IconCozPeopleFill />
            </CozAvatar>
          </Badge>
          {userTips}
        </button>
      </Dropdown>
      {children}
    </>
  );
};
