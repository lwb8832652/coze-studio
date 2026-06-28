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

import type { WorkbenchComposerOverlayPlacement } from './workbench-composer-at-menu';
import { workbenchModelTypeToNumber, type WorkbenchLLMModel } from './types';

export const findSelectedWorkbenchModel = (
  models: WorkbenchLLMModel[],
  value?: number,
) => models.find(model => workbenchModelTypeToNumber(model) === value);

const getWorkbenchModelDisplayName = (model?: WorkbenchLLMModel) =>
  model?.model_name || model?.name || '';

const getWorkbenchModelIdentifier = (model?: WorkbenchLLMModel) =>
  model?.name || model?.model_name || '';

export const getWorkbenchModelFallbackName = (model?: WorkbenchLLMModel) => {
  const displayName = getWorkbenchModelDisplayName(model);

  if (displayName) {
    return displayName;
  }

  return model ? `模型 ${workbenchModelTypeToNumber(model)}` : '默认模型';
};

const groupModelsByClass = (models: WorkbenchLLMModel[]) => {
  const groups: Array<{ name: string; models: WorkbenchLLMModel[] }> = [];
  const groupIndexes = new Map<string, number>();

  models.forEach(model => {
    const groupName = model.model_class_name || '其他模型';
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
  placement,
  onOpenChange,
  onChange,
}: {
  loading: boolean;
  models: WorkbenchLLMModel[];
  value?: number;
  open: boolean;
  placement: WorkbenchComposerOverlayPlacement;
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
        <div className="chat-workbench-model-menu" data-placement={placement}>
          <label className="chat-workbench-model-search">
            <span aria-hidden="true">⌕</span>
            <input
              aria-label="搜索模型"
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
      ) : null}
    </div>
  );
};
