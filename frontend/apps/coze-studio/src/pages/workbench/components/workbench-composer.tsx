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

import {
  type Dispatch,
  type SetStateAction,
  useEffect,
  useMemo,
  useState,
} from 'react';

import {
  IconCozArrowDown,
  IconCozLink,
  IconCozSendFill,
} from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';

import { ExtensionsPopover } from '../extensions-popover';
import { WorkbenchRuntimeSettingsControl } from './workbench-runtime-settings-control';
import {
  createWorkbenchSubmitPayload,
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
  getWorkbenchFailoverCandidateModelIds,
  WORKBENCH_MODE_PROMPTS,
  WORKBENCH_MODE_SYMBOLS,
  WORKBENCH_MODES,
  workbenchModelTypeToNumber,
  type WorkbenchLLMModel,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchComposerVariant,
  type WorkbenchMode,
  type WorkbenchResourceSelection,
  type WorkbenchRuntimeSettings,
} from './types';

const AT_RESOURCES = [
  '技能',
  '代码仓库',
  '仓库分支',
  '代码文件夹',
  '代码文件',
  '用户',
  '风神数据',
  'Meego 工作项',
  '空间文档库',
] as const;
type WorkbenchRuntimeSettingsChange = Dispatch<
  SetStateAction<WorkbenchRuntimeSettings>
>;

export interface WorkbenchComposerProps {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  variant?: WorkbenchComposerVariant;
  spaceId?: string;
  taskId?: string;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
}

const AtMenu = ({ onClose }: { onClose: () => void }) => (
  <div
    className="chat-workbench-at-menu"
    role="menu"
    aria-label="@ 选择资源类型"
  >
    <div className="chat-workbench-at-menu-title">
      <span>@</span>
      选择资源类型
    </div>
    <div className="chat-workbench-at-menu-list">
      {AT_RESOURCES.map((item, index) => (
        <button key={item} type="button" role="menuitem" onClick={onClose}>
          <span className="chat-workbench-at-menu-icon">
            {String(index + 1).padStart(2, '0')}
          </span>
          <span>{item}</span>
          <span aria-hidden="true">›</span>
        </button>
      ))}
    </div>
    <div className="chat-workbench-at-menu-footer">
      <span>↑↓ 移动光标</span>
      <span>↵ 选择条目</span>
      <span>Esc 退出</span>
    </div>
  </div>
);

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

const WorkbenchModelSelector = ({
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
  const selectedModel = models.find(
    model => workbenchModelTypeToNumber(model) === value,
  );
  const modelGroups = useMemo(() => groupModelsByClass(models), [models]);
  const label = loading ? '模型加载中' : selectedModel?.name || '默认模型';

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
        <div className="chat-workbench-model-menu" role="listbox">
          {modelGroups.map(group => (
            <div key={group.name} className="chat-workbench-model-group">
              <div className="chat-workbench-model-group-title">
                {group.name}
              </div>
              {group.models.map(model => {
                const modelType = workbenchModelTypeToNumber(model);

                return (
                  <button
                    key={`${modelType}-${model.name || 'model'}`}
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
                    <span>{model.name || `模型 ${modelType}`}</span>
                    {model.endpoint_name ? (
                      <span>{model.endpoint_name}</span>
                    ) : null}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
};

const useWorkbenchModelSelection = ({
  spaceId,
  modelLoader,
}: {
  spaceId?: string;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
}) => {
  const [models, setModels] = useState<WorkbenchLLMModel[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [selectedModelType, setSelectedModelType] = useState<
    number | undefined
  >();
  const selectedModel = models.find(
    model => workbenchModelTypeToNumber(model) === selectedModelType,
  );

  useEffect(() => {
    if (!spaceId || !modelLoader) {
      setModels([]);
      setSelectedModelType(undefined);
      return;
    }

    let canceled = false;
    setModelsLoading(true);

    void modelLoader(spaceId)
      .then(nextModels => {
        if (canceled) {
          return;
        }

        setModels(nextModels);
        setSelectedModelType(prevModelType => {
          if (
            prevModelType &&
            nextModels.some(
              model => workbenchModelTypeToNumber(model) === prevModelType,
            )
          ) {
            return prevModelType;
          }

          return nextModels[0]
            ? workbenchModelTypeToNumber(nextModels[0])
            : undefined;
        });
      })
      .catch(() => {
        if (!canceled) {
          setModels([]);
          setSelectedModelType(undefined);
        }
      })
      .finally(() => {
        if (!canceled) {
          setModelsLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [modelLoader, spaceId]);

  return {
    models,
    modelsLoading,
    selectedModel,
    selectedModelType,
    setSelectedModelType,
  };
};

const WorkbenchComposerBody = ({
  value,
  mode,
  onChange,
}: {
  value: string;
  mode: WorkbenchMode;
  onChange: (value: string) => void;
}) => (
  <div className="chat-workbench-composer-body">
    <TextArea
      aria-label="任务描述"
      autosize={false}
      rows={3}
      value={value}
      onChange={onChange}
      placeholder=""
      className="chat-workbench-input"
    />
    {!value ? (
      <div className="chat-workbench-composer-prompt" aria-label="当前模式提示">
        <span aria-hidden="true">{WORKBENCH_MODE_SYMBOLS[mode]}</span>
        <span>{WORKBENCH_MODE_PROMPTS[mode]}</span>
      </div>
    ) : null}
  </div>
);

const WorkbenchModeSelector = ({
  mode,
  onModeChange,
}: {
  mode: WorkbenchMode;
  onModeChange: (mode: WorkbenchMode) => void;
}) => (
  <div className="chat-workbench-mode" aria-label="模式选择">
    {WORKBENCH_MODES.map(item => (
      <button
        key={item}
        type="button"
        className="chat-workbench-mode-button"
        data-active={mode === item}
        aria-pressed={mode === item}
        onClick={() => onModeChange(item)}
      >
        {item}
      </button>
    ))}
  </div>
);

const WorkbenchComposerToolbar = ({
  atMenuOpen,
  canSend,
  loading,
  modelLoader,
  modelMenuOpen,
  models,
  modelsLoading,
  mode,
  resourceSelection,
  runtimeSettings,
  selectedModelType,
  failoverCandidateCount,
  spaceId,
  onAtMenuOpenChange,
  onModelMenuOpenChange,
  onModeChange,
  onResourceSelectionChange,
  onRuntimeSettingsChange,
  onSelectedModelTypeChange,
  onSubmit,
}: {
  atMenuOpen: boolean;
  canSend: boolean;
  loading: boolean;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  modelMenuOpen: boolean;
  models: WorkbenchLLMModel[];
  modelsLoading: boolean;
  mode: WorkbenchMode;
  resourceSelection: WorkbenchResourceSelection;
  runtimeSettings: WorkbenchRuntimeSettings;
  selectedModelType?: number;
  failoverCandidateCount: number;
  spaceId?: string;
  onAtMenuOpenChange: (open: boolean) => void;
  onModelMenuOpenChange: (open: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onResourceSelectionChange: (selection: WorkbenchResourceSelection) => void;
  onRuntimeSettingsChange: WorkbenchRuntimeSettingsChange;
  onSelectedModelTypeChange: (modelType: number) => void;
  onSubmit: () => void;
}) => (
  <div className="chat-workbench-toolbar">
    <div className="chat-workbench-toolbar-left">
      <WorkbenchModeSelector mode={mode} onModeChange={onModeChange} />

      <ExtensionsPopover
        value={resourceSelection}
        onChange={onResourceSelectionChange}
      />

      {spaceId && modelLoader ? (
        <WorkbenchModelSelector
          loading={modelsLoading}
          models={models}
          value={selectedModelType}
          open={modelMenuOpen}
          onOpenChange={onModelMenuOpenChange}
          onChange={model =>
            onSelectedModelTypeChange(workbenchModelTypeToNumber(model))
          }
        />
      ) : null}

      <WorkbenchRuntimeSettingsControl
        failoverCandidateCount={failoverCandidateCount}
        settings={runtimeSettings}
        onChange={onRuntimeSettingsChange}
      />
    </div>

    <div className="chat-workbench-toolbar-actions">
      <button
        type="button"
        aria-label="添加上下文"
        aria-expanded={atMenuOpen}
        onClick={() => onAtMenuOpenChange(!atMenuOpen)}
      >
        @
      </button>
      <button type="button" aria-label="添加附件">
        <IconCozLink />
      </button>
    </div>
    <Button
      aria-label="发送任务"
      color="primary"
      disabled={!canSend}
      icon={<IconCozSendFill />}
      loading={loading}
      onClick={onSubmit}
      className="chat-workbench-send"
    >
      <span className="chat-workbench-send-label">
        {loading ? '发送中' : '发送'}
      </span>
    </Button>
  </div>
);

export const WorkbenchComposer = ({
  value,
  mode,
  loading,
  error,
  variant = 'home',
  spaceId,
  taskId,
  modelLoader,
  onValueChange,
  onModeChange,
  onSubmit,
}: WorkbenchComposerProps) => {
  const [atMenuOpen, setAtMenuOpen] = useState(false);
  const [resourceSelection, setResourceSelection] = useState(
    createDefaultWorkbenchResourceSelection,
  );
  const [runtimeSettings, setRuntimeSettings] = useState(
    createDefaultWorkbenchRuntimeSettings,
  );
  const [modelMenuOpen, setModelMenuOpen] = useState(false);
  const canSend = Boolean(value.trim()) && !loading;
  const {
    models,
    modelsLoading,
    selectedModel,
    selectedModelType,
    setSelectedModelType,
  } = useWorkbenchModelSelection({ spaceId, modelLoader });
  const failoverCandidateCount = getWorkbenchFailoverCandidateModelIds(
    models,
    selectedModelType,
  ).length;

  useEffect(() => {
    setRuntimeSettings(prevSettings => ({
      ...prevSettings,
      mcp_tools: {
        ...prevSettings.mcp_tools,
        enabled: resourceSelection.enable_mcp.length > 0,
        allowed_tools: [...resourceSelection.enable_mcp],
      },
    }));
  }, [resourceSelection.enable_mcp]);

  const handleValueChange = (nextValue: string) => {
    onValueChange(nextValue);
    setAtMenuOpen(nextValue.endsWith('@'));
  };

  const handleSubmit = () => {
    const message = value.trim();

    if (!message || loading) {
      return;
    }

    onSubmit(
      createWorkbenchSubmitPayload({
        message,
        mode,
        taskId,
        selectedModel,
        models,
        resourceSelection,
        runtimeSettings,
      }),
    );
  };

  return (
    <>
      <section
        className="chat-workbench-composer"
        data-variant={variant}
        aria-label="任务输入"
      >
        {atMenuOpen ? <AtMenu onClose={() => setAtMenuOpen(false)} /> : null}
        <WorkbenchComposerBody
          value={value}
          mode={mode}
          onChange={handleValueChange}
        />

        <WorkbenchComposerToolbar
          atMenuOpen={atMenuOpen}
          canSend={canSend}
          loading={loading}
          modelLoader={modelLoader}
          modelMenuOpen={modelMenuOpen}
          models={models}
          modelsLoading={modelsLoading}
          mode={mode}
          resourceSelection={resourceSelection}
          runtimeSettings={runtimeSettings}
          selectedModelType={selectedModelType}
          failoverCandidateCount={failoverCandidateCount}
          spaceId={spaceId}
          onAtMenuOpenChange={setAtMenuOpen}
          onModelMenuOpenChange={setModelMenuOpen}
          onModeChange={onModeChange}
          onResourceSelectionChange={setResourceSelection}
          onRuntimeSettingsChange={setRuntimeSettings}
          onSelectedModelTypeChange={setSelectedModelType}
          onSubmit={handleSubmit}
        />
      </section>

      {error ? (
        <div className="chat-workbench-error" role="alert">
          {error}
        </div>
      ) : null}
    </>
  );
};
