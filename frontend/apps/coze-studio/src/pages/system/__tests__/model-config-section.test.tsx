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

/* eslint-disable @typescript-eslint/consistent-type-imports -- Test-only lazy package types avoid loading the production module graph. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

const serviceMocks = vi.hoisted(() => ({
  createAdminManagedModel: vi.fn(),
  deleteAdminModel: vi.fn(),
  getAdminManagedModelDetail: vi.fn(),
  getAdminManagedModelGrants: vi.fn(),
  listAdminManagedModels: vi.fn(),
  listAdminModelProviders: vi.fn(),
  listAdminUsers: vi.fn(),
  listAdminWorkspaces: vi.fn(),
  saveAdminManagedModelGrants: vi.fn(),
  sortAdminManagedModels: vi.fn(),
  testAdminManagedModelEndpoint: vi.fn(),
  updateAdminManagedModel: vi.fn(),
  updateAdminManagedModelStatus: vi.fn(),
}));

vi.mock('../service', async importOriginal => ({
  ...(await importOriginal<typeof import('../service')>()),
  ...serviceMocks,
}));

import { ModelConfigSection } from '../model-config-section';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const managedModel = {
  access_mode: 2,
  capability_types: ['text', 'reasoning'],
  creator_id: '9',
  enabled: true,
  id: '12',
  model_class: 3,
  model_identifier: 'deepseek-v4-pro',
  name: 'DeepSeek V4 Pro',
  provider_key: 'deepseek',
  sort_order: 1,
  updated_at_ms: 1784707200000,
};

describe('ModelConfigSection', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    serviceMocks.listAdminManagedModels.mockResolvedValue({
      models: [managedModel],
      total: 1,
    });
    serviceMocks.listAdminModelProviders.mockResolvedValue({
      providers: [
        {
          default_base_url: 'https://api.deepseek.com/v1',
          model_class: 3,
          name: { zh_cn: 'Deepseek 模型' },
          protocol: 'openai-compatible',
          provider_key: 'deepseek',
          supports_custom_base_url: true,
          supports_function_call: true,
          supports_multimodal: false,
        },
      ],
    });
    serviceMocks.listAdminUsers.mockResolvedValue({ users: [], total: 0 });
    serviceMocks.listAdminWorkspaces.mockResolvedValue({
      workspaces: [],
      total: 0,
    });
    serviceMocks.getAdminManagedModelGrants.mockResolvedValue({
      access_mode: 2,
      grants: [],
    });
    serviceMocks.getAdminManagedModelDetail.mockResolvedValue({
      model: {
        enable_base64_url: false,
        endpoints: [
          {
            base_url: 'https://api.deepseek.com/v1',
            enabled: true,
            has_api_key: true,
            id: '22',
            sort_order: 0,
            weight: 1,
          },
        ],
        function_call_mode: 'native',
        max_context_tokens: 128000,
        max_output_tokens: 8192,
        protocol: 'openai-compatible',
        reasoning_mode: 'enabled',
        routing_strategy: 1,
        summary: managedModel,
        usage_scenarios: ['chat', 'agent'],
      },
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  const renderSection = async () => {
    await act(async () => {
      root.render(<ModelConfigSection />);
      await new Promise(resolve => setTimeout(resolve, 0));
    });
  };

  it('renders the Nuwax-aligned model management table and filters', async () => {
    await renderSection();

    expect(container.textContent).toContain('添加模型');
    expect(container.textContent).toContain('模型名称');
    expect(container.textContent).toContain('模型标识');
    expect(container.textContent).toContain('模型介绍');
    expect(container.textContent).toContain('创建者');
    expect(container.textContent).toContain('管控');
    expect(container.textContent).toContain('DeepSeek V4 Pro');
    expect(container.textContent).toContain('文本生成');
    expect(container.textContent).toContain('深度思考');
    expect(container.querySelector('input[aria-label="搜索模型"]')).toBeNull();
    expect(
      container.querySelector('select[aria-label="模型类型筛选"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('select[aria-label="模型状态筛选"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('select[aria-label="模型管控筛选"]'),
    ).not.toBeNull();
    expect(
      container.querySelectorAll(
        '.coze-prototype-system-model-filters > select',
      ),
    ).toHaveLength(3);
    const dragButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="调整DeepSeek V4 Pro排序"]',
    );
    expect(dragButton).not.toBeNull();
    expect(dragButton?.textContent).toBe('');
  });

  it('opens edit dialog with a write-only credential field', async () => {
    await renderSection();
    const editButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '编辑',
    ) as HTMLButtonElement;

    await act(async () => {
      Simulate.click(editButton);
      await new Promise(resolve => setTimeout(resolve, 0));
    });

    expect(container.textContent).toContain('编辑模型');
    expect(container.textContent).toContain('模型连通性测试');
    const secretInput = container.querySelector(
      'input[aria-label="Endpoint 1 API Key"]',
    ) as HTMLInputElement;
    expect(secretInput.type).toBe('password');
    expect(secretInput.value).toBe('');
    expect(secretInput.placeholder).toContain('留空则保持不变');
    expect(container.textContent).not.toContain('system-secret');
  });

  it('opens the complete add model form', async () => {
    await renderSection();
    const addButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '添加模型',
    ) as HTMLButtonElement;

    act(() => Simulate.click(addButton));

    expect(container.textContent).toContain('添加模型');
    expect(container.textContent).toContain('基础信息');
    expect(container.textContent).toContain('能力与范围');
    expect(container.textContent).toContain('模型参数');
    expect(container.textContent).toContain('Endpoint 配置');
    expect(
      container.querySelector('select[aria-label="供应商"]'),
    ).not.toBeNull();
  });

  it('hydrates and submits OpenAI provider options', async () => {
    const openAIModel = {
      ...managedModel,
      model_class: 1,
      model_identifier: 'gpt-4.1',
      name: 'GPT 4.1',
      provider_key: 'openai',
    };
    serviceMocks.listAdminManagedModels.mockResolvedValue({
      models: [openAIModel],
      total: 1,
    });
    serviceMocks.listAdminModelProviders.mockResolvedValue({
      providers: [
        {
          default_base_url: 'https://api.openai.com/v1',
          model_class: 1,
          name: { zh_cn: 'OpenAI 模型' },
          protocol: 'openai-compatible',
          provider_key: 'openai',
          supports_custom_base_url: true,
          supports_function_call: true,
          supports_multimodal: true,
        },
      ],
    });
    serviceMocks.getAdminManagedModelDetail.mockResolvedValue({
      model: {
        enable_base64_url: false,
        endpoints: [
          {
            base_url: 'https://api.openai.com/v1',
            enabled: true,
            has_api_key: true,
            id: '22',
            sort_order: 0,
            weight: 1,
          },
        ],
        function_call_mode: 'native',
        max_context_tokens: 128000,
        max_output_tokens: 8192,
        protocol: 'openai-compatible',
        provider_options: {
          openai_api_version: '2025-04-01-preview',
          openai_by_azure: true,
        },
        reasoning_mode: 'enabled',
        routing_strategy: 1,
        summary: openAIModel,
        usage_scenarios: ['chat', 'agent'],
      },
    });

    await renderSection();
    const editButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '编辑',
    ) as HTMLButtonElement;
    await act(async () => {
      Simulate.click(editButton);
      await new Promise(resolve => setTimeout(resolve, 0));
    });

    const azureInput = container.querySelector(
      'input[aria-label="使用 Azure OpenAI"]',
    ) as HTMLInputElement;
    const versionInput = container.querySelector(
      'input[aria-label="OpenAI API Version"]',
    ) as HTMLInputElement;
    expect(azureInput.checked).toBe(true);
    expect(versionInput.value).toBe('2025-04-01-preview');

    act(() => {
      Simulate.change(azureInput, { target: { checked: false } });
      Simulate.change(versionInput, { target: { value: '2025-06-01' } });
    });
    const confirmButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '确认',
    ) as HTMLButtonElement;
    await act(async () => {
      Simulate.click(confirmButton);
      await new Promise(resolve => setTimeout(resolve, 0));
    });

    expect(serviceMocks.updateAdminManagedModel).toHaveBeenCalledWith(
      expect.objectContaining({
        management: expect.objectContaining({
          provider_options: {
            openai_api_version: '2025-06-01',
            openai_by_azure: false,
          },
        }),
      }),
    );
  });

  it('requires Vertex AI project and location before creating a Gemini model', async () => {
    serviceMocks.listAdminModelProviders.mockResolvedValue({
      providers: [
        {
          default_base_url: 'https://generativelanguage.googleapis.com/v1beta',
          model_class: 11,
          name: { zh_cn: 'Gemini 模型' },
          protocol: 'gemini',
          provider_key: 'gemini',
          supports_custom_base_url: true,
          supports_function_call: true,
          supports_multimodal: true,
        },
      ],
    });
    await renderSection();
    const addButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '添加模型',
    ) as HTMLButtonElement;
    act(() => Simulate.click(addButton));

    const changeInput = (label: string, value: string) => {
      const input = container.querySelector(
        `[aria-label="${label}"]`,
      ) as HTMLInputElement;
      Simulate.change(input, { target: { value } });
    };
    act(() => {
      changeInput('模型名称', 'Gemini 2.5 Pro');
      changeInput('模型标识', 'gemini-2.5-pro');
      changeInput('Endpoint 1 API Key', 'write-only-secret');
      const backend = container.querySelector(
        'select[aria-label="Gemini Backend"]',
      ) as HTMLSelectElement;
      Simulate.change(backend, { target: { value: '2' } });
    });

    const confirmButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '确认',
    ) as HTMLButtonElement;
    act(() => Simulate.click(confirmButton));
    expect(container.textContent).toContain(
      '使用 Vertex AI 时请填写 Project 和 Location',
    );
    expect(serviceMocks.createAdminManagedModel).not.toHaveBeenCalled();

    act(() => {
      changeInput('Vertex AI Project', 'newx-dev');
      changeInput('Vertex AI Location', 'us-central1');
    });
    await act(async () => {
      Simulate.click(confirmButton);
      await new Promise(resolve => setTimeout(resolve, 0));
    });
    expect(serviceMocks.createAdminManagedModel).toHaveBeenCalledWith(
      expect.objectContaining({
        management: expect.objectContaining({
          provider_options: {
            gemini_backend: 2,
            gemini_location: 'us-central1',
            gemini_project: 'newx-dev',
          },
        }),
      }),
    );
  });
});
