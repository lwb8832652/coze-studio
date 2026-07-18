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

import { SystemSettingsSection } from '../system-settings-section';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('SystemSettingsSection', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it('renders configured system settings', () => {
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            admin_emails: 'admin@example.test',
            allow_registration_email: 'example.test',
            code_runner_type: 1,
            disable_user_registration: true,
            server_host: 'http://localhost:8888',
          }}
          knowledgeConfig={{
            builtin_model_id: 42,
            embedding_config: {
              type: 1,
            },
            ocr_config: {
              type: 1,
            },
          }}
        />,
      );
    });

    const serverHostInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统服务地址"]',
    );
    const adminEmailsInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统管理员邮箱"]',
    );
    const allowRegistrationInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="允许注册邮箱"]',
    );
    const sandboxMigrationLink = container.querySelector<HTMLAnchorElement>(
      'a[href="/system/sandbox"]',
    );

    expect(container.textContent).toContain('配置总览');
    expect(container.textContent).toContain('服务地址');
    expect(serverHostInput?.value).toBe('http://localhost:8888');
    expect(adminEmailsInput?.value).toBe('admin@example.test');
    expect(allowRegistrationInput?.value).toBe('example.test');
    expect(container.querySelector('[aria-label="代码执行环境"]')).toBeNull();
    expect(container.textContent).toContain('配置已迁移到 Sandbox 管理');
    expect(sandboxMigrationLink?.textContent).toContain('前往 Sandbox 管理');
    expect(container.textContent).toContain('已关闭');
    expect(container.textContent).toContain('基础配置');
    expect(container.textContent).toContain('保存基础配置');
    expect(container.textContent).toContain('知识库配置');
    expect(container.textContent).toContain('内置模型 ID：42');
    expect(container.textContent).toContain('Embedding：已配置');
    expect(container.textContent).toContain('OCR：已配置');
  });

  it('renders empty setting hints', () => {
    act(() => {
      root.render(
        <SystemSettingsSection basicConfig={{}} knowledgeConfig={{}} />,
      );
    });

    expect(container.textContent).toContain('服务地址');
    expect(container.textContent).toContain('未配置');
    expect(container.textContent).toContain('允许注册');
    expect(container.textContent).toContain('内置模型 ID：未配置');
    expect(container.textContent).toContain('Embedding：未配置');
    expect(container.textContent).toContain('Rerank：未配置');
  });

  it('renders loading without a writable empty form', () => {
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={null}
          basicConfigLoading
          knowledgeConfig={{}}
        />,
      );
    });

    expect(container.textContent).toContain('正在加载系统基础配置');
    expect(
      container.querySelector('input[aria-label="系统服务地址"]'),
    ).toBeNull();
    expect(
      container.querySelector('button[aria-label="保存系统基础配置"]'),
    ).toBeNull();
  });

  it('renders load error and retries without exposing the form', async () => {
    const retry = vi.fn();
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={null}
          basicConfigLoadError="加载系统基础配置失败"
          knowledgeConfig={{}}
          onReloadBasicConfig={retry}
        />,
      );
    });

    expect(container.textContent).toContain('加载系统基础配置失败');
    expect(
      container.querySelector('input[aria-label="系统服务地址"]'),
    ).toBeNull();
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="重试加载系统基础配置"]',
        )!,
      );
    });
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it('saves editable basic settings', async () => {
    const saveConfig = vi.fn().mockResolvedValue(undefined);
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            admin_emails: 'admin@example.test',
            allow_registration_email: '',
            code_runner_type: 0,
            disable_user_registration: false,
            server_host: 'http://localhost:8888',
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    const serverHostInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统服务地址"]',
    );
    const adminEmailsInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统管理员邮箱"]',
    );
    const allowRegistrationInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="允许注册邮箱"]',
    );
    const disableRegistrationCheckbox =
      container.querySelector<HTMLInputElement>(
        'input[aria-label="关闭用户注册"]',
      );
    const saveButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="保存系统基础配置"]',
    );

    await act(async () => {
      serverHostInput!.value = ' https://agent.example.test ';
      Simulate.change(serverHostInput!);
      adminEmailsInput!.value = ' admin@example.test,ops@example.test ';
      Simulate.change(adminEmailsInput!);
      allowRegistrationInput!.value = ' example.test ';
      Simulate.change(allowRegistrationInput!);
      disableRegistrationCheckbox!.checked = true;
      Simulate.change(disableRegistrationCheckbox!);
    });

    await act(async () => {
      Simulate.click(saveButton!);
      await Promise.resolve();
    });

    expect(saveConfig).toHaveBeenCalledWith({
      admin_emails: 'admin@example.test,ops@example.test',
      allow_registration_email: 'example.test',
      disable_user_registration: true,
      server_host: 'https://agent.example.test',
    });
  });

  it('keeps legacy sandbox configuration readonly without issuing a save', () => {
    const saveConfig = vi.fn();
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            code_runner_type: 1,
            sandbox_config: {
              allow_env: 'SECRET_ENV_NAME',
            },
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    expect(container.textContent).toContain('配置已迁移到 Sandbox 管理');
    expect(container.querySelector('[aria-label="代码执行环境"]')).toBeNull();
    expect(container.querySelector('a[href="/system/sandbox"]')).not.toBeNull();
    expect(container.textContent).not.toContain('SECRET_ENV_NAME');
    expect(saveConfig).not.toHaveBeenCalled();
  });

  it('submits only fields changed by the administrator', async () => {
    const saveConfig = vi.fn().mockResolvedValue(undefined);
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            admin_emails: 'admin@example.test',
            allow_registration_email: 'example.test',
            disable_user_registration: false,
            server_host: 'https://old.example.test',
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    const serverHost = container.querySelector<HTMLInputElement>(
      'input[aria-label="系统服务地址"]',
    )!;
    await act(async () => {
      serverHost.value = 'https://new.example.test';
      Simulate.change(serverHost);
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存系统基础配置"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(saveConfig).toHaveBeenCalledWith({
      server_host: 'https://new.example.test',
    });
  });

  it('offers refresh after a version conflict', async () => {
    const reload = vi.fn();
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{ server_host: 'https://old.example.test' }}
          basicConfigMessage="配置已被其他管理员更新，请刷新后重试"
          basicConfigRefreshRequired
          knowledgeConfig={{}}
          onReloadBasicConfig={reload}
          onSaveBasicConfig={vi.fn()}
        />,
      );
    });

    expect(container.textContent).toContain(
      '配置已被其他管理员更新，请刷新后重试',
    );
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="刷新系统基础配置"]',
        )!,
      );
    });
    expect(reload).toHaveBeenCalledTimes(1);
  });
});
