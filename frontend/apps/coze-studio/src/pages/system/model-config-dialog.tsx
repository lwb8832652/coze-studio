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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-lines -- Production model form. */

import { useEffect, useMemo, useState } from 'react';

import { getI18nText } from './view-model';
import type {
  AdminManagedModel,
  AdminModelDetail,
  AdminModelEndpointInput,
  AdminModelEndpointTestPayload,
  AdminModelManagementInput,
  AdminModelProviderOption,
} from './service';

const CAPABILITY_OPTIONS = [
  { label: '文本生成', value: 'text' },
  { label: '图像理解', value: 'image' },
  { label: '语音理解', value: 'audio' },
  { label: '视频理解', value: 'video' },
  { label: '深度思考', value: 'reasoning' },
];

const USAGE_OPTIONS = [
  { label: '对话', value: 'chat' },
  { label: '智能体', value: 'agent' },
  { label: '工作流', value: 'workflow' },
  { label: '网页应用', value: 'appdev' },
];

const newEndpoint = (baseURL = ''): AdminModelEndpointInput => ({
  api_key: '',
  base_url: baseURL,
  enabled: true,
  sort_order: 0,
  weight: 1,
});

const createInitialValue = (
  providers: AdminModelProviderOption[],
): AdminModelManagementInput => {
  const provider = providers[0];
  return {
    access_mode: 1,
    capability_types: ['text'],
    description: '',
    enabled: true,
    enable_base64_url: false,
    endpoints: [newEndpoint(provider?.default_base_url)],
    function_call_mode: 'auto',
    max_context_tokens: 128000,
    max_output_tokens: 4096,
    model_identifier: '',
    name: '',
    protocol: provider?.protocol || 'openai-compatible',
    provider_key: provider?.provider_key || '',
    reasoning_mode: 'default',
    routing_strategy: 1,
    usage_scenarios: USAGE_OPTIONS.map(item => item.value),
  };
};

const detailToValue = (
  detail: AdminModelDetail,
): AdminModelManagementInput => ({
  access_mode: detail.summary.access_mode,
  capability_types: detail.summary.capability_types,
  description: detail.summary.description || '',
  enabled: detail.summary.enabled,
  enable_base64_url: detail.enable_base64_url,
  endpoints: detail.endpoints.map((endpoint, index) => ({
    api_key: '',
    base_url: endpoint.base_url,
    enabled: endpoint.enabled,
    id: endpoint.id || undefined,
    sort_order: index,
    weight: endpoint.weight,
  })),
  function_call_mode: detail.function_call_mode,
  max_context_tokens: detail.max_context_tokens,
  max_output_tokens: detail.max_output_tokens,
  model_identifier: detail.summary.model_identifier,
  name: detail.summary.name,
  protocol: detail.protocol,
  provider_key: detail.summary.provider_key,
  reasoning_mode: detail.reasoning_mode,
  routing_strategy: detail.routing_strategy,
  usage_scenarios: detail.usage_scenarios,
});

interface ModelConfigDialogProps {
  detail: AdminModelDetail | null;
  open: boolean;
  providers: AdminModelProviderOption[];
  saving: boolean;
  onCancel: () => void;
  onSave: (
    id: AdminManagedModel['id'] | undefined,
    modelClass: number,
    input: AdminModelManagementInput,
  ) => void | Promise<void>;
  onTest: (payload: AdminModelEndpointTestPayload) => Promise<{
    success: boolean;
    latency_ms: number;
    error_message?: string;
  }>;
}

export const ModelConfigDialog = ({
  detail,
  open,
  providers,
  saving,
  onCancel,
  onSave,
  onTest,
}: ModelConfigDialogProps) => {
  const [value, setValue] = useState<AdminModelManagementInput>(() =>
    createInitialValue(providers),
  );
  const [message, setMessage] = useState('');
  const [testing, setTesting] = useState(false);
  const selectedProvider = useMemo(
    () =>
      providers.find(provider => provider.provider_key === value.provider_key),
    [providers, value.provider_key],
  );

  useEffect(() => {
    if (!open) {
      return;
    }
    setValue(detail ? detailToValue(detail) : createInitialValue(providers));
    setMessage('');
  }, [detail, open, providers]);

  if (!open) {
    return null;
  }

  const updateEndpoint = (
    index: number,
    patch: Partial<AdminModelEndpointInput>,
  ) => {
    setValue(current => ({
      ...current,
      endpoints: current.endpoints.map((endpoint, endpointIndex) =>
        endpointIndex === index ? { ...endpoint, ...patch } : endpoint,
      ),
    }));
  };

  const toggleListValue = (
    field: 'capability_types' | 'usage_scenarios',
    item: string,
  ) => {
    setValue(current => {
      const list = current[field];
      return {
        ...current,
        [field]: list.includes(item)
          ? list.filter(valueItem => valueItem !== item)
          : [...list, item],
      };
    });
  };

  const validate = () => {
    if (
      !value.provider_key ||
      !value.name.trim() ||
      !value.model_identifier.trim()
    ) {
      return '请填写供应商、模型名称和模型标识';
    }
    if (value.capability_types.length === 0) {
      return '请至少选择一种模型类型';
    }
    if (
      value.max_context_tokens <= 0 ||
      value.max_output_tokens <= 0 ||
      value.max_output_tokens > value.max_context_tokens
    ) {
      return 'Token 上限配置不正确';
    }
    if (
      value.endpoints.length === 0 ||
      value.endpoints.some(endpoint => !endpoint.base_url.trim())
    ) {
      return '请至少配置一个有效的接口地址';
    }
    if (
      !detail &&
      selectedProvider?.provider_key !== 'ollama' &&
      value.endpoints.some(endpoint => !endpoint.api_key?.trim())
    ) {
      return '新建模型时需要填写 API Key';
    }
    return '';
  };

  const normalizeValue = (): AdminModelManagementInput => ({
    ...value,
    description: value.description?.trim(),
    endpoints: value.endpoints.map((endpoint, index) => ({
      ...endpoint,
      api_key: endpoint.api_key?.trim() || undefined,
      base_url: endpoint.base_url.trim(),
      id: endpoint.id || undefined,
      sort_order: index,
    })),
    model_identifier: value.model_identifier.trim(),
    name: value.name.trim(),
  });

  const submit = async () => {
    const validationMessage = validate();
    if (validationMessage) {
      setMessage(validationMessage);
      return;
    }
    if (!selectedProvider) {
      setMessage('供应商配置不可用，请刷新后重试');
      return;
    }
    setMessage('');
    await onSave(
      detail?.summary.id,
      selectedProvider.model_class,
      normalizeValue(),
    );
  };

  const testConnection = async () => {
    const validationMessage = validate();
    if (validationMessage) {
      setMessage(validationMessage);
      return;
    }
    const endpoint =
      value.endpoints.find(item => item.enabled) || value.endpoints[0];
    setTesting(true);
    setMessage('');
    try {
      const result = await onTest({
        endpoint: {
          ...endpoint,
          api_key: endpoint.api_key?.trim() || undefined,
        },
        model_id: detail?.summary.id,
        model_identifier: value.model_identifier.trim(),
        protocol: value.protocol,
        provider_key: value.provider_key,
      });
      setMessage(
        result.success
          ? `连接成功，耗时 ${result.latency_ms} ms`
          : result.error_message || '连接失败，请检查接口地址和凭证',
      );
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="coze-prototype-model-dialog-backdrop">
      <section
        aria-labelledby="system-model-dialog-title"
        aria-modal="true"
        className="coze-prototype-model-dialog"
        role="dialog"
      >
        <header className="coze-prototype-model-dialog-header">
          <div>
            <h2 id="system-model-dialog-title">
              {detail ? '编辑模型' : '添加模型'}
            </h2>
            <p>配置公共模型能力、路由策略和安全接入端点。</p>
          </div>
          <button
            aria-label="关闭模型配置弹窗"
            type="button"
            onClick={onCancel}
          >
            关闭
          </button>
        </header>

        <div className="coze-prototype-model-dialog-body">
          <section className="coze-prototype-model-form-section">
            <h3>基础信息</h3>
            <div className="coze-prototype-model-form-grid">
              <label>
                <span>供应商</span>
                <select
                  aria-label="供应商"
                  disabled={Boolean(detail)}
                  value={value.provider_key}
                  onChange={event => {
                    const provider = providers.find(
                      item => item.provider_key === event.target.value,
                    );
                    setValue(current => ({
                      ...current,
                      endpoints: current.endpoints.map((endpoint, index) =>
                        index === 0 && !endpoint.base_url
                          ? {
                              ...endpoint,
                              base_url: provider?.default_base_url || '',
                            }
                          : endpoint,
                      ),
                      protocol: provider?.protocol || current.protocol,
                      provider_key: event.target.value,
                    }));
                  }}
                >
                  <option value="">请选择供应商</option>
                  {providers.map(provider => (
                    <option
                      key={provider.provider_key}
                      value={provider.provider_key}
                    >
                      {getI18nText(provider.name)}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>模型名称</span>
                <input
                  aria-label="模型名称"
                  maxLength={128}
                  placeholder="例如 DeepSeek V4 Pro"
                  value={value.name}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      name: event.target.value,
                    }))
                  }
                />
              </label>
              <label>
                <span>模型标识</span>
                <input
                  aria-label="模型标识"
                  maxLength={256}
                  placeholder="例如 deepseek-v4-pro"
                  value={value.model_identifier}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      model_identifier: event.target.value,
                    }))
                  }
                />
              </label>
              <label className="coze-prototype-model-field-wide">
                <span>模型介绍</span>
                <textarea
                  aria-label="模型介绍"
                  maxLength={1024}
                  placeholder="说明模型定位、能力和适用场景"
                  rows={3}
                  value={value.description}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      description: event.target.value,
                    }))
                  }
                />
              </label>
            </div>
          </section>

          <section className="coze-prototype-model-form-section">
            <h3>能力与范围</h3>
            <div className="coze-prototype-model-option-group">
              <span>模型类型</span>
              <div>
                {CAPABILITY_OPTIONS.map(option => (
                  <label key={option.value}>
                    <input
                      checked={value.capability_types.includes(option.value)}
                      type="checkbox"
                      onChange={() =>
                        toggleListValue('capability_types', option.value)
                      }
                    />
                    {option.label}
                  </label>
                ))}
              </div>
            </div>
            <div className="coze-prototype-model-option-group">
              <span>可用范围</span>
              <div>
                {USAGE_OPTIONS.map(option => (
                  <label key={option.value}>
                    <input
                      checked={value.usage_scenarios.includes(option.value)}
                      type="checkbox"
                      onChange={() =>
                        toggleListValue('usage_scenarios', option.value)
                      }
                    />
                    {option.label}
                  </label>
                ))}
              </div>
            </div>
            <div className="coze-prototype-model-form-grid">
              <label>
                <span>状态</span>
                <select
                  aria-label="模型状态"
                  value={value.enabled ? 'enabled' : 'disabled'}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      enabled: event.target.value === 'enabled',
                    }))
                  }
                >
                  <option value="enabled">启用</option>
                  <option value="disabled">停用</option>
                </select>
              </label>
              <label>
                <span>管控</span>
                <select
                  aria-label="模型管控"
                  value={value.access_mode}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      access_mode: Number(event.target.value),
                    }))
                  }
                >
                  <option value={1}>全部用户可用</option>
                  <option value={2}>按授权范围可用</option>
                </select>
              </label>
            </div>
          </section>

          <section className="coze-prototype-model-form-section">
            <h3>模型参数</h3>
            <div className="coze-prototype-model-form-grid">
              <label>
                <span>接口协议</span>
                <select
                  aria-label="接口协议"
                  value={value.protocol}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      protocol: event.target.value,
                    }))
                  }
                >
                  <option value="openai-compatible">OpenAI Compatible</option>
                  <option value="anthropic">Anthropic</option>
                  <option value="gemini">Gemini</option>
                  <option value="ollama">Ollama</option>
                </select>
              </label>
              <label>
                <span>开启思考模式</span>
                <select
                  aria-label="思考模式"
                  value={value.reasoning_mode}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      reasoning_mode: event.target.value,
                    }))
                  }
                >
                  <option value="default">跟随模型默认</option>
                  <option value="enabled">开启</option>
                  <option value="disabled">关闭</option>
                  <option value="auto">自动</option>
                </select>
              </label>
              <label>
                <span>最大输出 Token</span>
                <input
                  aria-label="最大输出 Token"
                  min={1}
                  type="number"
                  value={value.max_output_tokens}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      max_output_tokens: Number(event.target.value),
                    }))
                  }
                />
              </label>
              <label>
                <span>最大上下文长度</span>
                <input
                  aria-label="最大上下文长度"
                  min={1}
                  type="number"
                  value={value.max_context_tokens}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      max_context_tokens: Number(event.target.value),
                    }))
                  }
                />
              </label>
              <label>
                <span>函数调用支持</span>
                <select
                  aria-label="函数调用支持"
                  value={value.function_call_mode}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      function_call_mode: event.target.value,
                    }))
                  }
                >
                  <option value="auto">自动检测</option>
                  <option value="native">原生支持</option>
                  <option value="none">不支持</option>
                </select>
              </label>
              <label>
                <span>接口调用策略</span>
                <select
                  aria-label="接口调用策略"
                  value={value.routing_strategy}
                  onChange={event =>
                    setValue(current => ({
                      ...current,
                      routing_strategy: Number(event.target.value),
                    }))
                  }
                >
                  <option value={1}>轮询</option>
                  <option value={2}>加权轮询</option>
                </select>
              </label>
            </div>
          </section>

          <section className="coze-prototype-model-form-section">
            <div className="coze-prototype-model-section-title">
              <div>
                <h3>Endpoint 配置</h3>
                <p>密钥只写入并加密保存，编辑时不会回显。</p>
              </div>
              <button
                type="button"
                onClick={() =>
                  setValue(current => ({
                    ...current,
                    endpoints: [...current.endpoints, newEndpoint()],
                  }))
                }
              >
                新增 Endpoint
              </button>
            </div>
            <div className="coze-prototype-model-endpoints">
              {value.endpoints.map((endpoint, index) => {
                const savedEndpoint = detail?.endpoints.find(
                  item => String(item.id) === String(endpoint.id),
                );
                return (
                  <article key={`${endpoint.id || 'new'}-${index}`}>
                    <div className="coze-prototype-model-endpoint-head">
                      <strong>Endpoint {index + 1}</strong>
                      {value.endpoints.length > 1 ? (
                        <button
                          type="button"
                          onClick={() =>
                            setValue(current => ({
                              ...current,
                              endpoints: current.endpoints.filter(
                                (_, endpointIndex) => endpointIndex !== index,
                              ),
                            }))
                          }
                        >
                          删除
                        </button>
                      ) : null}
                    </div>
                    <div className="coze-prototype-model-form-grid">
                      <label className="coze-prototype-model-field-wide">
                        <span>URL</span>
                        <input
                          aria-label={`Endpoint ${index + 1} URL`}
                          placeholder="https://api.example.com/v1"
                          value={endpoint.base_url}
                          onChange={event =>
                            updateEndpoint(index, {
                              base_url: event.target.value,
                            })
                          }
                        />
                      </label>
                      <label>
                        <span>API Key</span>
                        <input
                          aria-label={`Endpoint ${index + 1} API Key`}
                          autoComplete="new-password"
                          placeholder={
                            savedEndpoint?.has_api_key
                              ? '已配置，留空则保持不变'
                              : '请输入 API Key'
                          }
                          type="password"
                          value={endpoint.api_key || ''}
                          onChange={event =>
                            updateEndpoint(index, {
                              api_key: event.target.value,
                            })
                          }
                        />
                      </label>
                      <label>
                        <span>权重</span>
                        <input
                          aria-label={`Endpoint ${index + 1} 权重`}
                          min={1}
                          type="number"
                          value={endpoint.weight}
                          onChange={event =>
                            updateEndpoint(index, {
                              weight: Number(event.target.value),
                            })
                          }
                        />
                      </label>
                    </div>
                    <label className="coze-prototype-model-inline-check">
                      <input
                        checked={endpoint.enabled}
                        type="checkbox"
                        onChange={event =>
                          updateEndpoint(index, {
                            enabled: event.target.checked,
                          })
                        }
                      />
                      启用该 Endpoint
                    </label>
                  </article>
                );
              })}
            </div>
            <label className="coze-prototype-model-inline-check">
              <input
                checked={Boolean(value.enable_base64_url)}
                type="checkbox"
                onChange={event =>
                  setValue(current => ({
                    ...current,
                    enable_base64_url: event.target.checked,
                  }))
                }
              />
              启用 Base64 URL
            </label>
          </section>
        </div>

        <footer className="coze-prototype-model-dialog-footer">
          <p role="status">{message}</p>
          <div>
            <button
              disabled={saving || testing}
              type="button"
              onClick={() => void testConnection()}
            >
              {testing ? '测试中...' : '模型连通性测试'}
            </button>
            <button disabled={saving} type="button" onClick={onCancel}>
              取消
            </button>
            <button
              className="is-primary"
              disabled={saving || testing}
              type="button"
              onClick={() => void submit()}
            >
              {saving ? '保存中...' : '确认'}
            </button>
          </div>
        </footer>
      </section>
    </div>
  );
};
