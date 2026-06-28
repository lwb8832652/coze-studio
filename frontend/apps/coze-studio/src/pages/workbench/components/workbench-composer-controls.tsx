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
  type ReactNode,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react';

import {
  IconCozLink,
  IconCozLightbulb,
  IconCozSendFill,
  IconCozUpload,
} from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';

import { ExtensionsPopover } from '../extensions-popover';
import { WorkbenchRuntimeSettingsControl } from './workbench-runtime-settings-control';
import {
  findSelectedWorkbenchModel,
  getWorkbenchModelFallbackName,
  WorkbenchModelSelector,
} from './workbench-model-selector';
import {
  DeerFlowArrowUpIcon,
  DeerFlowGraduationIcon,
  DeerFlowRocketIcon,
  DeerFlowZapIcon,
} from './workbench-deerflow-mode-icons';
import type { WorkbenchComposerOverlayPlacement } from './workbench-composer-at-menu';
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

export type WorkbenchComposerPresentation = 'default' | 'deerflow';
type WorkbenchRuntimeSettingsChange = Dispatch<
  SetStateAction<WorkbenchRuntimeSettings>
>;
type DeerFlowInputMode = 'flash' | 'thinking' | 'pro' | 'ultra';
interface WorkbenchComposerToolbarProps {
  atMenuOpen: boolean;
  canSend: boolean;
  extensionsOpen: boolean;
  loading: boolean;
  modelLoader?: (spaceId: string) => Promise<WorkbenchLLMModel[]>;
  modelMenuOpen: boolean;
  models: WorkbenchLLMModel[];
  modelsLoading: boolean;
  mode: WorkbenchMode;
  modeMenuOpen: boolean;
  overlayPlacement: WorkbenchComposerOverlayPlacement;
  presentation: WorkbenchComposerPresentation;
  resourceSelection: WorkbenchResourceSelection;
  runtimeSettings: WorkbenchRuntimeSettings;
  selectedModelType?: number;
  failoverCandidateCount: number;
  spaceId?: string;
  onAtMenuOpenChange: (open: boolean) => void;
  onExtensionsOpenChange: (open: boolean) => void;
  onModelMenuOpenChange: (open: boolean) => void;
  onModeMenuOpenChange: (open: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onResourceSelectionChange: (selection: WorkbenchResourceSelection) => void;
  onRuntimeSettingsChange: WorkbenchRuntimeSettingsChange;
  onSelectedModelTypeChange: (modelType: number) => void;
  onSubmit: () => void;
}

const DEERFLOW_MODE_OPTIONS: Array<{
  description: string;
  icon: ReactNode;
  label: string;
  value: DeerFlowInputMode;
  workbenchMode: WorkbenchMode;
}> = [
  {
    value: 'flash',
    label: '闪速',
    icon: <DeerFlowZapIcon />,
    description: '快速且高效的完成任务，但可能不够精准',
    workbenchMode: 'Auto',
  },
  {
    value: 'thinking',
    label: '思考',
    icon: <IconCozLightbulb />,
    description: '思考后再行动，在时间与准确性之间取得平衡',
    workbenchMode: 'Ask',
  },
  {
    value: 'pro',
    label: 'Pro',
    icon: <DeerFlowGraduationIcon />,
    description: '思考、计划再执行，获得更精准的结果，可能需要更多时间',
    workbenchMode: 'Agent',
  },
  {
    value: 'ultra',
    label: 'Ultra',
    icon: <DeerFlowRocketIcon />,
    description: '继承自 Pro 模式，可调用子代理分工协作，适合复杂多步骤任务',
    workbenchMode: 'Agent',
  },
];

const defaultDeerFlowMode = 'pro';

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
  open,
  placement,
  onOpenChange,
  onModeChange,
}: {
  open: boolean;
  placement: WorkbenchComposerOverlayPlacement;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
}) => {
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
        onClick={() => onOpenChange(!open)}
      >
        <span aria-hidden="true">{activeOption.icon}</span>
        <span>{activeOption.label}</span>
      </button>
      {open ? (
        <div
          className="chat-workbench-deerflow-mode-menu"
          data-placement={placement}
          role="menu"
        >
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
                onOpenChange(false);
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

const WorkbenchComposerSendButton = ({
  canSend,
  isDeerFlow,
  loading,
  onSubmit,
}: {
  canSend: boolean;
  isDeerFlow: boolean;
  loading: boolean;
  onSubmit: () => void;
}) =>
  isDeerFlow ? (
    <button
      type="button"
      aria-label="发送任务"
      className="chat-workbench-send chat-workbench-send-deerflow"
      disabled={!canSend || loading}
      onClick={onSubmit}
    >
      <DeerFlowArrowUpIcon />
    </button>
  ) : (
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
  );

export const WorkbenchComposerToolbar = ({
  atMenuOpen,
  canSend,
  extensionsOpen,
  loading,
  modelLoader,
  modelMenuOpen,
  models,
  modelsLoading,
  mode,
  modeMenuOpen,
  overlayPlacement,
  presentation,
  resourceSelection,
  runtimeSettings,
  selectedModelType,
  failoverCandidateCount,
  spaceId,
  onAtMenuOpenChange,
  onExtensionsOpenChange,
  onModelMenuOpenChange,
  onModeMenuOpenChange,
  onModeChange,
  onResourceSelectionChange,
  onRuntimeSettingsChange,
  onSelectedModelTypeChange,
  onSubmit,
}: WorkbenchComposerToolbarProps) => {
  const isDeerFlow = presentation === 'deerflow';
  const modelLabel = modelsLoading
    ? '模型加载中'
    : getWorkbenchModelFallbackName(
        findSelectedWorkbenchModel(models, selectedModelType),
      );
  const modelSelector =
    spaceId && modelLoader ? (
      <WorkbenchModelSelector
        loading={modelsLoading}
        models={models}
        value={selectedModelType}
        open={modelMenuOpen}
        placement={overlayPlacement}
        onOpenChange={onModelMenuOpenChange}
        onChange={model =>
          onSelectedModelTypeChange(workbenchModelTypeToNumber(model))
        }
      />
    ) : (
      <span className="chat-workbench-model-readout">{modelLabel}</span>
    );
  const sendButton = (
    <WorkbenchComposerSendButton
      canSend={canSend}
      isDeerFlow={isDeerFlow}
      loading={loading}
      onSubmit={onSubmit}
    />
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
          <WorkbenchDeerFlowModeSelector
            open={modeMenuOpen}
            placement={overlayPlacement}
            onOpenChange={onModeMenuOpenChange}
            onModeChange={onModeChange}
          />
          <ExtensionsPopover
            open={extensionsOpen}
            placement={overlayPlacement}
            renderMask={false}
            showSelectedCount={false}
            value={resourceSelection}
            onChange={onResourceSelectionChange}
            onOpenChange={onExtensionsOpenChange}
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
          placement={overlayPlacement}
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
