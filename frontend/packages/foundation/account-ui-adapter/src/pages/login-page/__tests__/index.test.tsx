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

import type { ButtonHTMLAttributes, InputHTMLAttributes } from 'react';

import '@testing-library/jest-dom/vitest';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

const login = vi.hoisted(() => vi.fn());
const register = vi.hoisted(() => vi.fn());
const setLang = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  useLoginService: () => ({
    login,
    register,
    loginLoading: false,
    registerLoading: false,
  }),
}));

vi.mock('@coze-studio/components/coze-brand', () => ({
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the package component name.
  CozeBrand: () => <div>Coze</div>,
}));

vi.mock('@coze-arch/i18n', () => ({
  I18n: { setLang },
}));

vi.mock('@coze-arch/coze-design', () => {
  type MockButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
    htmlType?: 'button' | 'submit' | 'reset';
    loading?: boolean;
    color?: string;
  };
  type MockInputProps = Omit<
    InputHTMLAttributes<HTMLInputElement>,
    'onChange' | 'size'
  > & {
    mode?: string;
    validateStatus?: string;
    onChange?: (value: string) => void;
  };

  return {
    // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the package component name.
    Button: ({
      children,
      htmlType,
      loading: _loading,
      color: _color,
      ...props
    }: MockButtonProps) => (
      <button type={htmlType ?? 'button'} {...props}>
        {children}
      </button>
    ),
    // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the package component name.
    Input: ({
      mode,
      validateStatus: _validateStatus,
      onChange,
      type,
      ...props
    }: MockInputProps) => (
      <input
        {...props}
        type={mode === 'password' ? 'password' : type}
        onChange={event => onChange?.(event.target.value)}
      />
    ),
  };
});

vi.mock('@coze-arch/coze-design/icons', () => ({
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Mock export mirrors the package component name.
  IconCozEarth: () => <span aria-hidden="true" />,
}));

import { LoginPage } from '..';

const fillRegistrationForm = (confirmPassword = 'secret123') => {
  fireEvent.change(screen.getByTestId('login.input.email'), {
    target: { value: 'owner@example.com' },
  });
  fireEvent.change(screen.getByTestId('login.input.password'), {
    target: { value: 'secret123' },
  });
  fireEvent.change(screen.getByTestId('login.input.confirm-password'), {
    target: { value: confirmPassword },
  });
};

describe('LoginPage', () => {
  beforeEach(() => {
    localStorage.clear();
    login.mockReset();
    register.mockReset();
    setLang.mockReset();
  });

  it('switches between password login and email registration', () => {
    render(<LoginPage />);

    expect(screen.getByRole('tab', { name: '密码登录' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(
      screen.queryByTestId('login.input.confirm-password'),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('tab', { name: '邮箱注册' }));

    expect(screen.getByRole('tab', { name: '邮箱注册' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(
      screen.getByTestId('login.input.confirm-password'),
    ).toBeInTheDocument();
  });

  it('submits registration only when both passwords match', () => {
    render(<LoginPage />);
    fireEvent.click(screen.getByRole('tab', { name: '邮箱注册' }));
    fillRegistrationForm('different-password');

    fireEvent.click(screen.getByTestId('login.button.signup'));

    expect(register).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent('两次输入的密码不一致');

    fireEvent.change(screen.getByTestId('login.input.confirm-password'), {
      target: { value: 'secret123' },
    });
    fireEvent.click(screen.getByTestId('login.button.signup'));

    expect(register).toHaveBeenCalledTimes(1);
  });

  it('requires accepting the agreement before login', () => {
    render(<LoginPage />);
    fireEvent.change(screen.getByTestId('login.input.email'), {
      target: { value: 'owner@example.com' },
    });
    fireEvent.change(screen.getByTestId('login.input.password'), {
      target: { value: 'secret123' },
    });
    fireEvent.click(screen.getByRole('checkbox'));

    fireEvent.click(screen.getByTestId('login.button.login'));

    expect(login).not.toHaveBeenCalled();
    expect(screen.getByRole('alert')).toHaveTextContent(
      '请先阅读并同意开源协议',
    );
  });

  it('switches the login copy to English', () => {
    render(<LoginPage />);

    fireEvent.click(screen.getByRole('button', { name: '切换至 English' }));

    expect(screen.getByRole('tab', { name: 'Password' })).toBeInTheDocument();
    expect(setLang).toHaveBeenCalledWith('en');
    expect(localStorage.getItem('i18next')).toBe('en');
  });
});
