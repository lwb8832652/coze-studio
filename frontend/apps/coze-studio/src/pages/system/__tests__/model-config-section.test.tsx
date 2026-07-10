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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { ModelConfigSection } from '../model-config-section';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('ModelConfigSection', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onCreateModel = vi.fn();
  const onDeleteModel = vi.fn();
  const onRefresh = vi.fn();

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
    vi.clearAllMocks();
  });

  it('renders provider groups and model detail rows', () => {
    act(() => {
      root.render(
        <ModelConfigSection
          modelProviders={[
            {
              provider: {
                model_class: 1,
                name: {
                  zh_cn: 'OpenAI 模型',
                },
              },
              model_list: [
                {
                  id: 12,
                  display_info: {
                    name: 'GPT 4o',
                  },
                  connection: {
                    base_conn_info: {
                      api_key: 'sk***test',
                      base_url: 'https://api.openai.com/v1',
                      model: 'gpt-4o',
                    },
                  },
                  enable_base64_url: true,
                },
              ],
            },
          ]}
          onCreateModel={onCreateModel}
          onDeleteModel={onDeleteModel}
        />,
      );
    });

    expect(container.textContent).toContain('模型配置');
    expect(container.textContent).not.toMatch(/nuwax|一期|参考/i);
    expect(container.textContent).toContain('OpenAI 模型');
    expect(container.textContent).toContain('模型列表');
    expect(container.textContent).toContain('供应商');
    expect(container.textContent).toContain('密钥状态');
    expect(container.textContent).toContain('1 个模型');
    expect(container.textContent).toContain('GPT 4o');
    expect(container.textContent).toContain('gpt-4o');
    expect(container.textContent).toContain('密钥已配置');
    expect(container.textContent).not.toContain('sk***test');
    expect(container.textContent).toContain('Base64 URL');
  });

  it('submits new model config, clears the form, and deletes existing model config', async () => {
    onCreateModel.mockResolvedValueOnce(undefined);

    act(() => {
      root.render(
        <ModelConfigSection
          modelProviders={[
            {
              provider: {
                model_class: 1,
                name: {
                  zh_cn: 'OpenAI 模型',
                },
              },
              model_list: [
                {
                  id: 12,
                  display_info: {
                    name: 'GPT 4o',
                  },
                },
              ],
            },
          ]}
          onCreateModel={onCreateModel}
          onDeleteModel={onDeleteModel}
        />,
      );
    });

    const nameInput = container.querySelector(
      'input[aria-label="模型展示名称"]',
    ) as HTMLInputElement;
    const modelInput = container.querySelector(
      'input[aria-label="模型标识"]',
    ) as HTMLInputElement;
    const baseURLInput = container.querySelector(
      'input[aria-label="模型 Base URL"]',
    ) as HTMLInputElement;
    const apiKeyInput = container.querySelector(
      'input[aria-label="模型 API Key"]',
    ) as HTMLInputElement;
    const base64Input = container.querySelector(
      'input[aria-label="启用 Base64 URL"]',
    ) as HTMLInputElement;
    const submitButton = container.querySelector(
      'button[aria-label="新增模型配置"]',
    ) as HTMLButtonElement;
    const deleteButton = container.querySelector(
      'button[aria-label="删除模型-12"]',
    ) as HTMLButtonElement;

    act(() => {
      Simulate.change(nameInput, {
        target: {
          value: 'GPT 4o',
        },
      } as unknown as Event);
      Simulate.change(modelInput, {
        target: {
          value: 'gpt-4o',
        },
      } as unknown as Event);
      Simulate.change(baseURLInput, {
        target: {
          value: 'https://api.openai.com/v1',
        },
      } as unknown as Event);
      Simulate.change(apiKeyInput, {
        target: {
          value: 'sk-test',
        },
      } as unknown as Event);
      Simulate.change(base64Input, {
        target: {
          checked: true,
        },
      } as unknown as Event);
    });

    await act(async () => {
      Simulate.click(submitButton);
      await Promise.resolve();
    });

    act(() => {
      Simulate.click(deleteButton);
    });

    expect(onCreateModel).toHaveBeenCalledWith({
      model_class: 1,
      model_name: 'GPT 4o',
      enable_base64_url: true,
      connection: {
        base_conn_info: {
          api_key: 'sk-test',
          base_url: 'https://api.openai.com/v1',
          model: 'gpt-4o',
        },
      },
    });
    expect(onDeleteModel).toHaveBeenCalledWith(12);
    expect(nameInput.value).toBe('');
    expect(modelInput.value).toBe('');
    expect(baseURLInput.value).toBe('');
    expect(apiKeyInput.value).toBe('');
    expect(base64Input.checked).toBe(false);
  });

  it('filters model cards by keyword and refreshes model providers', () => {
    act(() => {
      root.render(
        <ModelConfigSection
          modelProviders={[
            {
              provider: {
                model_class: 1,
                name: {
                  zh_cn: 'OpenAI 模型',
                },
              },
              model_list: [
                {
                  id: 12,
                  display_info: {
                    name: 'GPT 4o',
                  },
                  connection: {
                    base_conn_info: {
                      model: 'gpt-4o',
                    },
                  },
                },
              ],
            },
            {
              provider: {
                model_class: 20,
                name: {
                  zh_cn: 'Ollama',
                },
              },
              model_list: [
                {
                  id: 20,
                  display_info: {
                    name: 'Local Llama',
                  },
                  connection: {
                    base_conn_info: {
                      model: 'llama3',
                    },
                  },
                },
              ],
            },
          ]}
          onRefresh={onRefresh}
        />,
      );
    });

    expect(container.textContent).toContain('GPT 4o');
    expect(container.textContent).toContain('Local Llama');

    const keywordInput = container.querySelector(
      'input[aria-label="搜索模型配置"]',
    ) as HTMLInputElement;
    const refreshButton = container.querySelector(
      'button[aria-label="刷新模型配置"]',
    ) as HTMLButtonElement;

    act(() => {
      Simulate.change(keywordInput, {
        target: {
          value: 'local',
        },
      } as unknown as Event);
    });

    expect(container.textContent).not.toContain('GPT 4o');
    expect(container.textContent).toContain('Local Llama');

    act(() => {
      Simulate.click(refreshButton);
    });

    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it('allows Ollama model config without api key', async () => {
    act(() => {
      root.render(
        <ModelConfigSection
          modelProviders={[
            {
              provider: {
                model_class: 20,
                name: {
                  zh_cn: 'Ollama',
                },
              },
              model_list: [],
            },
          ]}
          onCreateModel={onCreateModel}
        />,
      );
    });

    const nameInput = container.querySelector(
      'input[aria-label="模型展示名称"]',
    ) as HTMLInputElement;
    const modelInput = container.querySelector(
      'input[aria-label="模型标识"]',
    ) as HTMLInputElement;
    const submitButton = container.querySelector(
      'button[aria-label="新增模型配置"]',
    ) as HTMLButtonElement;

    act(() => {
      Simulate.change(nameInput, {
        target: {
          value: 'Local Llama',
        },
      } as unknown as Event);
      Simulate.change(modelInput, {
        target: {
          value: 'llama3',
        },
      } as unknown as Event);
    });

    await act(async () => {
      Simulate.click(submitButton);
      await Promise.resolve();
    });

    expect(onCreateModel).toHaveBeenCalledWith({
      model_class: 20,
      model_name: 'Local Llama',
      enable_base64_url: false,
      connection: {
        base_conn_info: {
          model: 'llama3',
        },
      },
    });
  });
});
