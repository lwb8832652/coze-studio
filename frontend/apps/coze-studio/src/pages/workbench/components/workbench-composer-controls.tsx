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

import { useMemo, useState, type Dispatch, type SetStateAction } from 'react';

import {
  IconCozArrowDown,
  IconCozLink,
  IconCozSendFill,
  IconCozUpload,
} from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';

import { ExtensionsPopover } from '../extensions-popover';
import { WorkbenchRuntimeSettingsControl } from './workbench-runtime-settings-control';
import {
  WORKBENCH_MODE_PROMPTS,
  WORKBENCH_MODE_SYMBOLS,
  WORKBENCH_MODES,
  workbenchModelTypeToNumber,
  type WorkbenchLLMModel,
  type WorkbenchMode,
  type WorkbenchResourceSelection,
  type WorkbenchRuntimeSettings,
} from './types';

const AT_RESOURCE_NUMBER_WIDTH = 2;
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

export type WorkbenchComposerPresentation = 'default' | 'deerflow';
type WorkbenchRuntimeSettingsChange = Dispatch<
  SetStateAction<WorkbenchRuntimeSettings>
>;
type DeerFlowInputMode = 'flash' | 'thinking' | 'pro' | 'ultra';
interface WorkbenchComposerToolbarProps {
  atMenuOpen: boolean;
  canSend: boolean;
  loading: boolean;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  modelMenuOpen: boolean;
  models: WorkbenchLLMModel[];
  modelsLoading: boolean;
  mode: WorkbenchMode;
  presentation: WorkbenchComposerPresentation;
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
}

const DEERFLOW_MODE_OPTIONS: Array<{
  description: string;
  icon: string;
  label: string;
  value: DeerFlowInputMode;
  workbenchMode: WorkbenchMode;
}> = [
  {
    value: 'flash',
    label: '闪速',
    icon: '⚡',
    description: '快速且高效的完成任务，但可能不够精准',
    workbenchMode: 'Auto',
  },
  {
    value: 'thinking',
    label: '思考',
    icon: '◌',
    description: '思考后再行动，在时间与准确性之间取得平衡',
    workbenchMode: 'Ask',
  },
  {
    value: 'pro',
    label: 'Pro',
    icon: '◇',
    description: '思考、计划再执行，获得更精准的结果，可能需要更多时间',
    workbenchMode: 'Agent',
  },
  {
    value: 'ultra',
    label: 'Ultra',
    icon: '🚀',
    description: '继承自 Pro 模式，可调用子代理分工协作，适合复杂多步骤任务',
    workbenchMode: 'Agent',
  },
];

const defaultDeerFlowMode = 'ultra';

export const findSelectedWorkbenchModel = (
  models: WorkbenchLLMModel[],
  value?: number,
) => models.find(model => workbenchModelTypeToNumber(model) === value);

export const AtMenu = ({ onClose }: { onClose: () => void }) => (
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
            {String(index + 1).padStart(AT_RESOURCE_NUMBER_WIDTH, '0')}
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
  const selectedModel = findSelectedWorkbenchModel(models, value);
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

export const WorkbenchComposerBody = ({
  value,
  mode,
  presentation,
  onChange,
}: {
  value: string;
  mode: WorkbenchMode;
  presentation: WorkbenchComposerPresentation;
  onChange: (value: string) => void;
}) => {
  const isDeerFlow = presentation === 'deerflow';

  return (
    <div className="chat-workbench-composer-body">
      <TextArea
        aria-label="任务描述"
        autosize={false}
        rows={3}
        value={value}
        onChange={onChange}
        placeholder={isDeerFlow ? '今天想做什么？' : ''}
        className="chat-workbench-input"
      />
      {!value && !isDeerFlow ? (
        <div
          className="chat-workbench-composer-prompt"
          aria-label="当前模式提示"
        >
          <span aria-hidden="true">{WORKBENCH_MODE_SYMBOLS[mode]}</span>
          <span>{WORKBENCH_MODE_PROMPTS[mode]}</span>
        </div>
      ) : null}
    </div>
  );
};

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

const WorkbenchDeerFlowModeSelector = ({
  onModeChange,
}: {
  onModeChange: (mode: WorkbenchMode) => void;
}) => {
  const [open, setOpen] = useState(false);
  const [deerflowMode, setDeerflowMode] =
    useState<DeerFlowInputMode>(defaultDeerFlowMode);
  const activeOption =
    DEERFLOW_MODE_OPTIONS.find(option => option.value === deerflowMode) ??
    DEERFLOW_MODE_OPTIONS[0];

  return (
    <div className="chat-workbench-deerflow-mode">
      <button
        type="button"
        className="chat-workbench-deerflow-mode-trigger"
        aria-label="选择模式"
        aria-expanded={open}
        onClick={() => setOpen(value => !value)}
      >
        <span aria-hidden="true">{activeOption.icon}</span>
        <span>{activeOption.label}</span>
      </button>
      {open ? (
        <div className="chat-workbench-deerflow-mode-menu" role="menu">
          <div className="chat-workbench-deerflow-mode-title">模式</div>
          {DEERFLOW_MODE_OPTIONS.map(option => (
            <button
              key={option.value}
              type="button"
              role="menuitemradio"
              aria-checked={option.value === deerflowMode}
              className="chat-workbench-deerflow-mode-option"
              data-active={option.value === deerflowMode}
              onClick={() => {
                setDeerflowMode(option.value);
                onModeChange(option.workbenchMode);
                setOpen(false);
              }}
            >
              <span className="chat-workbench-deerflow-mode-option-icon">
                {option.icon}
              </span>
              <span className="chat-workbench-deerflow-mode-option-copy">
                <span>{option.label}</span>
                <span>{option.description}</span>
              </span>
              <span className="chat-workbench-deerflow-mode-option-check">
                {option.value === deerflowMode ? '✓' : ''}
              </span>
            </button>
          ))}
        </div>
      ) : null}
    </div>
  );
};

export const WorkbenchComposerToolbar = ({
  atMenuOpen,
  canSend,
  loading,
  modelLoader,
  modelMenuOpen,
  models,
  modelsLoading,
  mode,
  presentation,
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
}: WorkbenchComposerToolbarProps) => {
  const isDeerFlow = presentation === 'deerflow';
  const modelLabel = modelsLoading
    ? '模型加载中'
    : findSelectedWorkbenchModel(models, selectedModelType)?.name || '默认模型';
  const modelSelector =
    spaceId && modelLoader ? (
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
    ) : (
      <span className="chat-workbench-model-readout">{modelLabel}</span>
    );
  const sendButton = (
    <Button
      aria-label="发送任务"
      color="primary"
      disabled={!canSend}
      icon={<IconCozSendFill />}
      loading={loading}
      onClick={onSubmit}
      className={`chat-workbench-send${
        isDeerFlow ? ' chat-workbench-send-deerflow' : ''
      }`}
    >
      <span className="chat-workbench-send-label">
        {loading ? '发送中' : '发送'}
      </span>
    </Button>
  );

  if (isDeerFlow) {
    return (
      <div className="chat-workbench-toolbar" data-presentation="deerflow">
        <div className="chat-workbench-toolbar-left">
          <button
            type="button"
            className="chat-workbench-icon-action"
            aria-label="添加附件"
          >
            <IconCozUpload />
          </button>
          <WorkbenchDeerFlowModeSelector onModeChange={onModeChange} />
          <ExtensionsPopover
            value={resourceSelection}
            onChange={onResourceSelectionChange}
          />
        </div>

        <div className="chat-workbench-toolbar-actions">
          {modelSelector}
          <button
            type="button"
            aria-label="添加上下文"
            aria-expanded={atMenuOpen}
            onClick={() => onAtMenuOpenChange(!atMenuOpen)}
          >
            @
          </button>
          <button type="button" aria-label="添加链接">
            <IconCozLink />
          </button>
          {sendButton}
        </div>
      </div>
    );
  }

  return (
    <div className="chat-workbench-toolbar">
      <div className="chat-workbench-toolbar-left">
        <WorkbenchModeSelector mode={mode} onModeChange={onModeChange} />

        <ExtensionsPopover
          value={resourceSelection}
          onChange={onResourceSelectionChange}
        />

        {spaceId && modelLoader ? modelSelector : null}

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
      {sendButton}
    </div>
  );
};
