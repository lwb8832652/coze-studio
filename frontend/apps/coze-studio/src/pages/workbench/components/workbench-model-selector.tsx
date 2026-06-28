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

import { IconCozArrowDown } from '@coze-arch/coze-design/icons';

import { workbenchModelTypeToNumber, type WorkbenchLLMModel } from './types';

export const findSelectedWorkbenchModel = (
  models: WorkbenchLLMModel[],
  value?: number,
) => models.find(model => workbenchModelTypeToNumber(model) === value);

const knownDeerFlowModelDisplayNames: Record<string, string> = {
  'deepseek-v4-pro': 'DeepSeek V4 Pro (Thinking)',
};

const modelIdentifierTokenLabels: Record<string, string> = {
  ai: 'AI',
  api: 'API',
  claude: 'Claude',
  deepseek: 'DeepSeek',
  doubao: 'Doubao',
  gemini: 'Gemini',
  gpt: 'GPT',
  kimi: 'Kimi',
  openai: 'OpenAI',
  pro: 'Pro',
  qwen: 'Qwen',
  vl: 'VL',
};

const trimModelText = (value?: string) => value?.trim() ?? '';

const isModelIdentifierLike = (value: string) =>
  /^[a-z0-9][a-z0-9._:/-]*$/.test(value);

const getKnownModelDisplayName = (model?: WorkbenchLLMModel) => {
  const candidates = [
    model?.display_name,
    model?.name,
    model?.model_name,
    model?.model,
  ].map(trimModelText);

  for (const candidate of candidates) {
    const knownDisplayName = knownDeerFlowModelDisplayNames[candidate];

    if (knownDisplayName) {
      return knownDisplayName;
    }
  }

  return '';
};

const humanizeWorkbenchModelIdentifier = (identifier: string) =>
  identifier
    .replace(/[/:_-]+/g, ' ')
    .split(' ')
    .filter(Boolean)
    .map(part => {
      const normalizedPart = part.toLowerCase();
      const knownLabel = modelIdentifierTokenLabels[normalizedPart];

      if (knownLabel) {
        return knownLabel;
      }

      if (/^v\d/i.test(part)) {
        return part.toUpperCase();
      }

      if (/^[\d.]+$/.test(part)) {
        return part;
      }

      return `${part.charAt(0).toUpperCase()}${part.slice(1)}`;
    })
    .join(' ');

const getWorkbenchModelDisplayName = (model?: WorkbenchLLMModel) => {
  const directDisplayName = trimModelText(model?.display_name);

  if (directDisplayName) {
    return directDisplayName;
  }

  const knownDisplayName = getKnownModelDisplayName(model);

  if (knownDisplayName) {
    return knownDisplayName;
  }

  const name = trimModelText(model?.name);

  if (name && !isModelIdentifierLike(name)) {
    return name;
  }

  const modelName = trimModelText(model?.model_name);

  if (modelName && !isModelIdentifierLike(modelName)) {
    return modelName;
  }

  return humanizeWorkbenchModelIdentifier(
    name || modelName || trimModelText(model?.model),
  );
};

const appendWorkbenchModelEndpointName = (
  displayName: string,
  endpointName?: string,
) => {
  const normalizedEndpointName = endpointName?.trim();

  if (!displayName || !normalizedEndpointName) {
    return displayName;
  }

  if (displayName.includes(normalizedEndpointName)) {
    return displayName;
  }

  return `${displayName} (${normalizedEndpointName})`;
};

const getWorkbenchModelIdentifier = (model?: WorkbenchLLMModel) =>
  model?.model_name || model?.name || model?.model || '';

const getWorkbenchModelGroupName = (model: WorkbenchLLMModel) => {
  const groupName = trimModelText(model.model_class_name);

  if (groupName.toLowerCase() === 'deekseek') {
    return 'DeepSeek';
  }

  return groupName || '其他模型';
};

export const getWorkbenchModelFallbackName = (model?: WorkbenchLLMModel) => {
  const displayName = getWorkbenchModelDisplayName(model);

  if (displayName) {
    return appendWorkbenchModelEndpointName(displayName, model?.endpoint_name);
  }

  return model ? `模型 ${workbenchModelTypeToNumber(model)}` : '默认模型';
};

const groupModelsByClass = (models: WorkbenchLLMModel[]) => {
  const groups: Array<{ name: string; models: WorkbenchLLMModel[] }> = [];
  const groupIndexes = new Map<string, number>();

  models.forEach(model => {
    const groupName = getWorkbenchModelGroupName(model);
    const existingIndex = groupIndexes.get(groupName);

    if (existingIndex === undefined) {
      groupIndexes.set(groupName, groups.length);
      groups.push({ name: groupName, models: [model] });

      return;
    }

    groups[existingIndex].models.push(model);
  });

  return groups;
};

export const WorkbenchModelSelector = ({
  loading,
  models,
  value,
  open,
  onOpenChange,
  onChange,
}: {
  loading: boolean;
  models: WorkbenchLLMModel[];
  value?: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChange: (model: WorkbenchLLMModel) => void;
}) => {
  const [keyword, setKeyword] = useState('');
  const selectedModel = findSelectedWorkbenchModel(models, value);
  const normalizedKeyword = keyword.trim().toLowerCase();
  const visibleModels = useMemo(() => {
    if (!normalizedKeyword) {
      return models;
    }

    return models.filter(model =>
      [
        getWorkbenchModelDisplayName(model),
        getWorkbenchModelIdentifier(model),
        model.model_class_name,
        model.endpoint_name,
      ].some(text => text?.toLowerCase().includes(normalizedKeyword)),
    );
  }, [models, normalizedKeyword]);
  const modelGroups = useMemo(
    () => groupModelsByClass(visibleModels),
    [visibleModels],
  );
  const label = loading
    ? '模型加载中'
    : getWorkbenchModelFallbackName(selectedModel);

  useEffect(() => {
    if (!open) {
      setKeyword('');
    }
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        onOpenChange(false);
      }
    };

    document.addEventListener('keydown', handleKeyDown);

    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onOpenChange, open]);

  return (
    <div className="chat-workbench-model">
      <button
        type="button"
        className="chat-workbench-model-trigger"
        aria-label="选择模型"
        aria-expanded={open}
        disabled={loading || models.length === 0}
        onClick={() => onOpenChange(!open)}
      >
        <span>{label}</span>
        <IconCozArrowDown />
      </button>

      {open && models.length ? (
        <div
          className="chat-workbench-model-dialog-mask"
          data-open="true"
          onMouseDown={event => {
            if (event.target === event.currentTarget) {
              onOpenChange(false);
            }
          }}
        >
          <div
            className="chat-workbench-model-menu"
            role="dialog"
            aria-modal="true"
            aria-label="选择模型"
          >
            <label className="chat-workbench-model-search">
              <span aria-hidden="true">⌕</span>
              <input
                aria-label="搜索模型"
                autoFocus
                value={keyword}
                placeholder="搜索模型..."
                onChange={event => setKeyword(event.target.value)}
              />
            </label>
            <div className="chat-workbench-model-list" role="listbox">
              {modelGroups.map(group => (
                <div key={group.name} className="chat-workbench-model-group">
                  <div className="chat-workbench-model-group-title">
                    {group.name}
                  </div>
                  {group.models.map(model => {
                    const modelType = workbenchModelTypeToNumber(model);
                    const displayName = getWorkbenchModelFallbackName(model);
                    const identifier = getWorkbenchModelIdentifier(model);

                    return (
                      <button
                        key={`${modelType}-${identifier || 'model'}`}
                        type="button"
                        role="option"
                        aria-selected={modelType === value}
                        className="chat-workbench-model-option"
                        data-active={modelType === value}
                        onClick={() => {
                          onChange(model);
                          onOpenChange(false);
                        }}
                      >
                        <span className="chat-workbench-model-option-copy">
                          <span>{displayName}</span>
                          {identifier && identifier !== displayName ? (
                            <span>{identifier}</span>
                          ) : null}
                        </span>
                        <span className="chat-workbench-model-option-check">
                          {modelType === value ? '✓' : ''}
                        </span>
                      </button>
                    );
                  })}
                </div>
              ))}
              {visibleModels.length === 0 ? (
                <div className="chat-workbench-model-empty">暂无匹配模型</div>
              ) : null}
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
};
