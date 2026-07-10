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
    const codeRunnerSelect = container.querySelector<HTMLSelectElement>(
      'select[aria-label="代码执行环境"]',
    );

    expect(container.textContent).toContain('配置总览');
    expect(container.textContent).toContain('服务地址');
    expect(serverHostInput?.value).toBe('http://localhost:8888');
    expect(adminEmailsInput?.value).toBe('admin@example.test');
    expect(allowRegistrationInput?.value).toBe('example.test');
    expect(codeRunnerSelect?.value).toBe('1');
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
    const codeRunnerSelect = container.querySelector<HTMLSelectElement>(
      'select[aria-label="代码执行环境"]',
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
      codeRunnerSelect!.value = '1';
      Simulate.change(codeRunnerSelect!);
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
      code_runner_type: 1,
      disable_user_registration: true,
      server_host: 'https://agent.example.test',
    });
  });
});
