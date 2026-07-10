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

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const getProfile = vi.hoisted(() => vi.fn());
const updateProfile = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getProfile,
  updateProfile,
}));

import ProfilePage from '..';

const baseProfile = {
  user_id_str: '1001',
  name: '畅享AI',
  screen_name: '畅享AI',
  email: 'admin@example.com',
  description: '负责系统管理',
  avatar_url: '',
  app_user_info: {
    user_unique_name: 'admin',
  },
};

describe('ProfilePage', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    vi.clearAllMocks();
    getProfile.mockResolvedValue(baseProfile);
    updateProfile.mockResolvedValue({
      ...baseProfile,
      name: '新昵称',
      description: '新的个人简介',
    });
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  const renderPage = async () => {
    await act(async () => {
      root.render(<ProfilePage />);
    });
    await act(async () => {
      await Promise.resolve();
    });
  };

  const flushEffects = async () => {
    await act(async () => {
      await Promise.resolve();
    });
  };

  it('renders current profile after loading', async () => {
    await renderPage();

    expect(container.textContent).toContain('个人中心');
    expect(
      container.querySelector<HTMLInputElement>('input[aria-label="个人昵称"]')
        ?.value,
    ).toBe('畅享AI');
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="个人用户名"]',
      )?.value,
    ).toBe('admin');
    expect(container.textContent).toContain('admin@example.com');
    expect(container.textContent).toContain('admin');
  });

  it('saves nickname and description changes', async () => {
    await renderPage();

    const nameInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="个人昵称"]',
    );
    const descriptionInput = container.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="个人简介"]',
    );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存个人资料"]',
    );
    const form = saveButton?.closest('form');

    await act(async () => {
      Simulate.change(nameInput!, {
        target: { value: '新昵称' },
      } as never);
      Simulate.change(descriptionInput!, {
        target: { value: '新的个人简介' },
      } as never);
    });

    await act(async () => {
      Simulate.submit(form!);
    });
    await flushEffects();

    expect(updateProfile).toHaveBeenCalledWith({
      name: '新昵称',
      description: '新的个人简介',
    });
    expect(container.textContent).toContain('保存成功');
  });

  it('saves username changes', async () => {
    await renderPage();

    const usernameInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="个人用户名"]',
    );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存个人资料"]',
    );
    const form = saveButton?.closest('form');

    await act(async () => {
      Simulate.change(usernameInput!, {
        target: { value: 'new_admin' },
      } as never);
    });

    await act(async () => {
      Simulate.submit(form!);
    });
    await flushEffects();

    expect(updateProfile).toHaveBeenCalledWith({
      name: '畅享AI',
      description: '负责系统管理',
      user_unique_name: 'new_admin',
    });
  });

  it('blocks saving empty nickname', async () => {
    await renderPage();

    const nameInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="个人昵称"]',
    );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存个人资料"]',
    );
    const form = saveButton?.closest('form');

    await act(async () => {
      Simulate.change(nameInput!, {
        target: { value: '   ' },
      } as never);
    });

    await act(async () => {
      Simulate.submit(form!);
    });

    expect(container.textContent).toContain('请输入昵称');
    expect(updateProfile).not.toHaveBeenCalled();
  });

  it('blocks saving empty username', async () => {
    await renderPage();

    const usernameInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="个人用户名"]',
    );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存个人资料"]',
    );
    const form = saveButton?.closest('form');

    await act(async () => {
      Simulate.change(usernameInput!, {
        target: { value: '   ' },
      } as never);
    });

    await act(async () => {
      Simulate.submit(form!);
    });

    expect(container.textContent).toContain('请输入用户名');
    expect(updateProfile).not.toHaveBeenCalled();
  });
});
