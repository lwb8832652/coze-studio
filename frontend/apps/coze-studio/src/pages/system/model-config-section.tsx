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

import { useEffect, useMemo, useState } from 'react';

import type {
  AdminCreateModelPayload,
  AdminModelInfo,
  AdminProviderModelListItem,
} from './service';
import { getI18nText } from './view-model';

interface ModelConfigSectionProps {
  modelProviders: AdminProviderModelListItem[];
  modelSaving?: boolean;
  modelRefreshing?: boolean;
  modelMessage?: string;
  onCreateModel?: (payload: AdminCreateModelPayload) => void | Promise<void>;
  onDeleteModel?: (id: number | string) => void | Promise<void>;
  onRefresh?: () => void | Promise<void>;
}

const getModelName = (model: AdminModelInfo) =>
  model.display_info?.name || model.connection?.base_conn_info?.model || model.id;

const getModelIdentity = (model: AdminModelInfo) =>
  model.connection?.base_conn_info?.model || '未配置标识';

const getProviderName = (provider: AdminProviderModelListItem) =>
  getI18nText(provider.provider?.name);

const getModelSearchText = (
  provider: AdminProviderModelListItem,
  model?: AdminModelInfo,
) =>
  [
    getProviderName(provider),
    provider.provider?.model_class,
    model ? getModelName(model) : '',
    model?.connection?.base_conn_info?.model,
    model?.connection?.base_conn_info?.base_url,
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();

export const ModelConfigSection = ({
  modelProviders,
  modelSaving = false,
  modelRefreshing = false,
  modelMessage = '',
  onCreateModel,
  onDeleteModel,
  onRefresh,
}: ModelConfigSectionProps) => {
  const firstProviderClass = modelProviders.find(
    provider => typeof provider.provider?.model_class === 'number',
  )?.provider?.model_class;
  const [modelClass, setModelClass] = useState(
    firstProviderClass ? String(firstProviderClass) : '',
  );
  const [modelName, setModelName] = useState('');
  const [modelID, setModelID] = useState('');
  const [baseURL, setBaseURL] = useState('');
  const [apiKey, setAPIKey] = useState('');
  const [enableBase64URL, setEnableBase64URL] = useState(false);
  const [validationMessage, setValidationMessage] = useState('');
  const [keyword, setKeyword] = useState('');

  const modelCount = useMemo(
    () =>
      modelProviders.reduce(
        (sum, provider) => sum + (provider.model_list?.length ?? 0),
        0,
      ),
    [modelProviders],
  );

  const filteredProviders = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    if (!normalizedKeyword) {
      return modelProviders;
    }

    return modelProviders
      .map(provider => {
        if (getModelSearchText(provider).includes(normalizedKeyword)) {
          return provider;
        }

        const modelList = (provider.model_list ?? []).filter(model =>
          getModelSearchText(provider, model).includes(normalizedKeyword),
        );

        return {
          ...provider,
          model_list: modelList,
        };
      })
      .filter(
        provider =>
          getModelSearchText(provider).includes(normalizedKeyword) ||
          (provider.model_list?.length ?? 0) > 0,
      );
  }, [keyword, modelProviders]);

  const filteredModelCount = useMemo(
    () =>
      filteredProviders.reduce(
        (sum, provider) => sum + (provider.model_list?.length ?? 0),
        0,
      ),
    [filteredProviders],
  );
  const filteredModelRows = useMemo(
    () =>
      filteredProviders.flatMap(provider =>
        (provider.model_list ?? []).map(model => ({
          provider,
          model,
        })),
      ),
    [filteredProviders],
  );

  useEffect(() => {
    if (!modelClass && firstProviderClass) {
      setModelClass(String(firstProviderClass));
    }
  }, [firstProviderClass, modelClass]);

  const submitModel = async () => {
    const selectedModelClass = Number(modelClass);
    const requireAPIKey = selectedModelClass !== 20;
    if (
      !modelClass ||
      Number.isNaN(selectedModelClass) ||
      !modelName.trim() ||
      !modelID.trim() ||
      (requireAPIKey && !apiKey.trim())
    ) {
      setValidationMessage(
        requireAPIKey
          ? '请填写供应商、展示名称、模型标识和 API Key'
          : '请填写供应商、展示名称和模型标识',
      );
      return;
    }

    const baseConnInfo = {
      model: modelID.trim(),
      ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
      ...(baseURL.trim() ? { base_url: baseURL.trim() } : {}),
    };

    try {
      await onCreateModel?.({
        model_class: selectedModelClass,
        model_name: modelName.trim(),
        enable_base64_url: enableBase64URL,
        connection: {
          base_conn_info: baseConnInfo,
        },
      });
      setModelName('');
      setModelID('');
      setBaseURL('');
      setAPIKey('');
      setEnableBase64URL(false);
      setValidationMessage('');
    } catch (error) {
      setValidationMessage('');
    }
  };

  return (
    <section className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>模型配置</h2>
          <p>
            集中管理公共模型供应商、模型搜索、新增、删除和接入信息安全展示。
          </p>
          {modelMessage ? <p>{modelMessage}</p> : null}
          {validationMessage ? <p>{validationMessage}</p> : null}
          <div className="mt-[12px] grid gap-[10px] md:grid-cols-3">
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">供应商</p>
              <strong className="text-[20px] text-[#1d2333]">
                {modelProviders.length}
              </strong>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">模型总数</p>
              <strong className="text-[20px] text-[#1d2333]">
                {modelCount}
              </strong>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">当前筛选</p>
              <strong className="text-[20px] text-[#1d2333]">
                {filteredModelCount}
              </strong>
            </div>
          </div>
          <div className="mt-[12px] flex flex-wrap items-center gap-[8px]">
            <input
              aria-label="搜索模型配置"
              className="h-[34px] min-w-[240px] flex-1 rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
              placeholder="搜索供应商、模型名称或模型标识"
              value={keyword}
              onChange={event => setKeyword(event.target.value)}
            />
            <button
              aria-label="刷新模型配置"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] text-[#1d2333] disabled:cursor-not-allowed disabled:opacity-60"
              disabled={modelRefreshing || !onRefresh}
              type="button"
              onClick={() => void onRefresh?.()}
            >
              {modelRefreshing ? '刷新中...' : '刷新'}
            </button>
          </div>
          <div className="mt-[12px] grid gap-[8px] md:grid-cols-2">
            <select
              aria-label="模型供应商"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
              value={modelClass}
              onChange={event => setModelClass(event.target.value)}
            >
              <option value="">选择供应商</option>
              {modelProviders.map((provider, index) => (
                <option
                  key={`${provider.provider?.model_class ?? index}`}
                  value={provider.provider?.model_class ?? ''}
                >
                  {getI18nText(provider.provider?.name)}
                </option>
              ))}
            </select>
            <input
              aria-label="模型展示名称"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
              placeholder="展示名称，例如 GPT 4o"
              value={modelName}
              onChange={event => setModelName(event.target.value)}
            />
            <input
              aria-label="模型标识"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
              placeholder="模型标识，例如 gpt-4o"
              value={modelID}
              onChange={event => setModelID(event.target.value)}
            />
            <input
              aria-label="模型 Base URL"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
              placeholder="Base URL，可选"
              value={baseURL}
              onChange={event => setBaseURL(event.target.value)}
            />
            <input
              aria-label="模型 API Key"
              className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
              placeholder="API Key"
              type="password"
              value={apiKey}
              onChange={event => setAPIKey(event.target.value)}
            />
            <label className="flex h-[34px] items-center gap-[8px] text-[13px] text-[#4d566a]">
              <input
                aria-label="启用 Base64 URL"
                checked={enableBase64URL}
                type="checkbox"
                onChange={event => setEnableBase64URL(event.target.checked)}
              />
              启用 Base64 URL
            </label>
          </div>
          <button
            aria-label="新增模型配置"
            className="mt-[12px] h-[34px] rounded-[8px] bg-[#1d2333] px-[14px] text-[13px] text-white disabled:cursor-not-allowed disabled:opacity-60"
            disabled={modelSaving || !onCreateModel}
            type="button"
            onClick={() => void submitModel()}
          >
            {modelSaving ? '保存中...' : '新增模型配置'}
          </button>
        </div>
        <span>{modelCount} 个模型</span>
      </article>
      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <h2>模型列表</h2>
          {modelProviders.length === 0 ? <p>暂无模型供应商配置</p> : null}
          {modelProviders.length > 0 && filteredProviders.length === 0 ? (
            <p>没有匹配的模型配置</p>
          ) : null}
          {filteredProviders.length > 0 ? (
            <div className="mt-[10px] flex flex-wrap gap-[8px]">
              {filteredProviders.map((provider, index) => (
                <span
                  className="rounded-full bg-[#eef3ff] px-[10px] py-[4px] text-[12px] text-[#2b55d4]"
                  key={`${getProviderName(provider)}-${index}`}
                >
                  {getProviderName(provider)} ·{' '}
                  {provider.model_list?.length ?? 0} 个模型
                </span>
              ))}
            </div>
          ) : null}
          <div className="mt-[12px] overflow-x-auto rounded-[12px] border border-[#e7ebf3]">
            <table className="w-full min-w-[860px] border-collapse text-left text-[13px]">
              <thead className="bg-[#f8fafc] text-[#687385]">
                <tr>
                  <th className="px-[14px] py-[10px] font-medium">模型名称</th>
                  <th className="px-[14px] py-[10px] font-medium">供应商</th>
                  <th className="px-[14px] py-[10px] font-medium">模型标识</th>
                  <th className="px-[14px] py-[10px] font-medium">Base URL</th>
                  <th className="px-[14px] py-[10px] font-medium">密钥状态</th>
                  <th className="px-[14px] py-[10px] font-medium">
                    Base64 URL
                  </th>
                  <th className="px-[14px] py-[10px] font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredModelRows.length === 0 ? (
                  <tr>
                    <td className="px-[14px] py-[18px] text-[#687385]" colSpan={7}>
                      暂无模型配置
                    </td>
                  </tr>
                ) : null}
                {filteredModelRows.map(({ provider, model }) => (
                  <tr
                    className="border-t border-[#edf0f5]"
                    key={`${provider.provider?.model_class ?? 'provider'}-${
                      model.id ?? getModelIdentity(model)
                    }`}
                  >
                    <td className="px-[14px] py-[12px] text-[#1d2333]">
                      <strong>{getModelName(model)}</strong>
                      {model.id ? (
                        <span className="block text-[12px] text-[#8a94a6]">
                          配置 ID：{model.id}
                        </span>
                      ) : null}
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {getProviderName(provider)}
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {getModelIdentity(model)}
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {model.connection?.base_conn_info?.base_url ||
                        '默认服务地址'}
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {model.connection?.base_conn_info?.api_key
                        ? '密钥已配置'
                        : '未配置密钥'}
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {model.enable_base64_url ? '已启用' : '未启用'}
                    </td>
                    <td className="px-[14px] py-[12px]">
                      {model.id ? (
                        <button
                          aria-label={`删除模型-${model.id}`}
                          className="h-[28px] rounded-[8px] border border-[#f1b4b4] px-[10px] text-[12px] text-[#b42318]"
                          type="button"
                          onClick={() => void onDeleteModel?.(model.id!)}
                        >
                          删除
                        </button>
                      ) : (
                        <span className="text-[12px] text-[#8a94a6]">-</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <span>{filteredModelRows.length} 个模型</span>
      </article>
    </section>
  );
};
