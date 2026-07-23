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
import {
  DEFAULT_SITE_CONFIG,
  useCommonConfigStore,
} from '@coze-foundation/global-store';

const uploadAdminSiteAsset = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  uploadAdminSiteAsset,
}));

import { SystemSettingsSection } from '../system-settings-section';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('SystemSettingsSection', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    uploadAdminSiteAsset.mockReset();
    useCommonConfigStore.getState().updateSiteConfig(DEFAULT_SITE_CONFIG);
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
      'input[aria-label="站点访问地址"]',
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
      container.querySelector('input[aria-label="站点访问地址"]'),
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
      container.querySelector('input[aria-label="站点访问地址"]'),
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

  it('saves editable system settings', async () => {
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

  it('submits only fields changed in the site configuration card', async () => {
    const saveConfig = vi.fn().mockResolvedValue(undefined);
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            admin_emails: 'admin@example.test',
            allow_registration_email: 'example.test',
            disable_user_registration: false,
            server_host: 'https://old.example.test',
            site_name: 'Old Site',
            site_description: 'Old description',
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    const serverHost = container.querySelector<HTMLInputElement>(
      'input[aria-label="站点访问地址"]',
    )!;
    const siteName = container.querySelector<HTMLInputElement>(
      'input[aria-label="站点名称"]',
    )!;
    const siteDescription = container.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="站点介绍"]',
    )!;
    await act(async () => {
      serverHost.value = 'https://new.example.test';
      Simulate.change(serverHost);
      siteName.value = 'NewX AI';
      Simulate.change(siteName);
      siteDescription.value = 'NewX AI workspace';
      Simulate.change(siteDescription);
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存站点配置"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(saveConfig).toHaveBeenCalledWith({
      server_host: 'https://new.example.test',
      site_name: 'NewX AI',
      site_description: 'NewX AI workspace',
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

  it('uploads a site logo, previews it and saves only the returned URI', async () => {
    const saveConfig = vi.fn().mockResolvedValue(undefined);
    uploadAdminSiteAsset.mockResolvedValue({
      uri: 'site-brand/logo/content-hash.png',
      url: 'https://assets.example.com/logo.png',
      width: 128,
      height: 64,
      mime_type: 'image/png',
    });
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            server_host: 'http://localhost:8888',
            site_name: 'NewX AI',
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    const input = container.querySelector<HTMLInputElement>(
      'input[aria-label="上传站点 Logo"]',
    )!;
    const file = new File(['logo'], 'logo.png', { type: 'image/png' });
    Object.defineProperty(input, 'files', {
      configurable: true,
      value: [file],
    });
    await act(async () => {
      Simulate.change(input);
      await Promise.resolve();
    });

    expect(uploadAdminSiteAsset).toHaveBeenCalledWith('logo', file);
    expect(
      container.querySelector<HTMLImageElement>('img[alt="站点 Logo"]')?.src,
    ).toBe('https://assets.example.com/logo.png');

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存站点配置"]',
        )!,
      );
      await Promise.resolve();
    });
    expect(saveConfig).toHaveBeenCalledWith({
      site_logo_uri: 'site-brand/logo/content-hash.png',
    });
  });

  it('supports removing an existing site logo with an explicit empty URI', async () => {
    const saveConfig = vi.fn().mockResolvedValue(undefined);
    useCommonConfigStore.getState().updateSiteConfig({
      ...DEFAULT_SITE_CONFIG,
      siteLogoUrl: 'https://assets.example.com/logo.png',
    });
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            server_host: 'http://localhost:8888',
            site_name: 'NewX AI',
            site_logo_uri: 'site-brand/logo/content-hash.png',
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    await act(async () => {
      Simulate.click(
        Array.from(
          container.querySelectorAll<HTMLButtonElement>('button'),
        ).find(button => button.textContent === '移除')!,
      );
    });
    expect(
      container.querySelector<HTMLImageElement>('img[alt="站点 Logo"]'),
    ).toBeNull();

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存站点配置"]',
        )!,
      );
      await Promise.resolve();
    });
    expect(saveConfig).toHaveBeenCalledWith({ site_logo_uri: '' });
  });

  it('keeps invalid site text local and does not issue a save', async () => {
    const saveConfig = vi.fn();
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            server_host: 'http://localhost:8888',
            site_name: 'NewX AI',
          }}
          knowledgeConfig={{}}
          onSaveBasicConfig={saveConfig}
        />,
      );
    });

    const siteName = container.querySelector<HTMLInputElement>(
      'input[aria-label="站点名称"]',
    )!;
    await act(async () => {
      siteName.value = '   ';
      Simulate.change(siteName);
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存站点配置"]',
        )!,
      );
    });

    expect(container.textContent).toContain('站点名称不能为空');
    expect(saveConfig).not.toHaveBeenCalled();
  });
});
