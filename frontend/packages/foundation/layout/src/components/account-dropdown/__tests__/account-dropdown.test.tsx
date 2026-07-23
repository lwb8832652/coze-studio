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

import { GlobalLayoutAccountDropdown } from '../index';

vi.mock('@coze-foundation/account-adapter', () => ({
  useUserInfo: () => ({
    avatar_url: '',
    name: '测试用户',
  }),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the component name.
  IconCozPeopleFill: () => (
    <span data-testid="default-person-avatar-icon">person-avatar-icon</span>
  ),
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror component names. */
vi.mock('@coze-arch/coze-design', () => {
  const Dropdown = ({
    children,
    render,
  }: {
    children: ReactNode;
    render?: ReactNode | (() => ReactNode);
  }) => (
    <div>
      {children}
      {typeof render === 'function' ? render() : render}
    </div>
  );

  Dropdown.Menu = ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  );
  Dropdown.Item = ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  );

  return {
    Avatar: () => <span>avatar</span>,
    Badge: ({ children }: { children: ReactNode }) => <>{children}</>,
    CozAvatar: ({
      children,
      src,
      type,
    }: {
      children?: ReactNode;
      src?: string;
      type?: string;
    }) => (
      <span data-testid="account-person-avatar" data-src={src} data-type={type}>
        {children}
      </span>
    ),
    Dropdown,
  };
});
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

describe('GlobalLayoutAccountDropdown', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('does not emit React key warnings when menus include custom React nodes', () => {
    const consoleErrorSpy = vi
      .spyOn(console, 'error')
      .mockImplementation(() => undefined);

    renderToStaticMarkup(
      <GlobalLayoutAccountDropdown
        menus={[
          <div>Custom profile</div>,
          <div>Custom divider</div>,
          {
            title: 'Logout',
            onClick: vi.fn(),
          },
        ]}
      />,
    );

    const hasReactKeyWarning = consoleErrorSpy.mock.calls.some(call =>
      call.some(
        arg =>
          typeof arg === 'string' &&
          arg.includes('Each child in a list should have a unique "key" prop'),
      ),
    );

    expect(hasReactKeyWarning).toBe(false);
  });

  it('uses the shared person avatar when no custom avatar is configured', () => {
    const markup = renderToStaticMarkup(<GlobalLayoutAccountDropdown />);

    expect(markup).toContain('data-testid="account-person-avatar"');
    expect(markup).toContain('data-type="person"');
    expect(markup).toContain('person-avatar-icon');
    expect(markup).not.toContain('>avatar<');
  });
});
