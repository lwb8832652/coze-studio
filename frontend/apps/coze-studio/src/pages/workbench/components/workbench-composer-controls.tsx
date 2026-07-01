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

/* eslint-disable max-lines -- P0 composer controls stay colocated until phase 2 split. */

import {
  useEffect,
  useRef,
  type KeyboardEventHandler,
  type ReactNode,
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
  DeerFlowStopIcon,
  DeerFlowZapIcon,
} from './workbench-deerflow-mode-icons';
import { WorkbenchAtSegments } from './workbench-composer-at-segments';
import type {
  WorkbenchAtDraft,
  WorkbenchAtSegment,
  WorkbenchComposerOverlayPlacement,
} from './workbench-composer-at-menu';
import { WorkbenchAtInline } from './workbench-composer-at-inline';
import {
  DEFAULT_WORKBENCH_MODE,
  WORKBENCH_MODE_DESCRIPTIONS,
  WORKBENCH_MODE_LABELS,
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
  stopLoading?: boolean;
  stopMode?: boolean;
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
  icon: ReactNode;
  value: WorkbenchMode;
}> = [
  {
    value: 'flash',
    icon: <DeerFlowZapIcon />,
  },
  {
    value: 'thinking',
    icon: <IconCozLightbulb />,
  },
  {
    value: 'pro',
    icon: <DeerFlowGraduationIcon />,
  },
  {
    value: 'ultra',
    icon: <DeerFlowRocketIcon />,
  },
];

export const WorkbenchComposerBody = ({
  atDraft,
  atSegments,
  value,
  mode,
  presentation,
  showSkillSuggestions,
  skillSuggestionPlacement = 'top',
  skillSuggestionIndex,
  skillSuggestions,
  onChange,
  onSkillSuggestionApply,
  onSkillSuggestionIndexChange,
  onSkillSuggestionKeyDown,
  onAtDraftQueryChange,
  onAtDraftCancel,
  onAtAnchorRectChange,
  onAtSegmentRemove,
  onTextareaBlur,
  onTextareaFocus,
}: {
  atDraft?: WorkbenchAtDraft | null;
  atSegments?: WorkbenchAtSegment[];
  value: string;
  mode: WorkbenchMode;
  presentation: WorkbenchComposerPresentation;
  showSkillSuggestions?: boolean;
  skillSuggestionPlacement?: WorkbenchComposerOverlayPlacement;
  skillSuggestionIndex?: number;
  skillSuggestions?: Array<{ description?: string; id: string; name: string }>;
  onChange: (value: string) => void;
  onSkillSuggestionApply?: (skill: { id: string; name: string }) => void;
  onSkillSuggestionIndexChange?: (index: number) => void;
  onSkillSuggestionKeyDown?: KeyboardEventHandler<HTMLTextAreaElement>;
  onAtDraftQueryChange?: (query: string) => void;
  onAtDraftCancel?: () => void;
  onAtAnchorRectChange?: (rect: DOMRect) => void;
  onAtSegmentRemove?: (segment: WorkbenchAtSegment) => void;
  onTextareaBlur?: () => void;
  onTextareaFocus?: () => void;
}) => {
  const isDeerFlow = presentation === 'deerflow';
  const hasRichContent = Boolean(atSegments?.length || atDraft);
  const richInputRef = useRef<HTMLDivElement>(null);
  const hadDraftRef = useRef(Boolean(atDraft));

  useEffect(() => {
    const shouldRestoreFocus = hadDraftRef.current && !atDraft;
    hadDraftRef.current = Boolean(atDraft);

    if (!shouldRestoreFocus) {
      return;
    }

    richInputRef.current
      ?.querySelector<HTMLTextAreaElement>('textarea[aria-label="任务描述"]')
      ?.focus();
  }, [atDraft]);

  return (
    <div className="chat-workbench-composer-body">
      {showSkillSuggestions && skillSuggestions?.length ? (
        <div
          aria-label="Skill suggestions"
          className="chat-workbench-skill-suggestions"
          data-placement={skillSuggestionPlacement}
          role="listbox"
        >
          {skillSuggestions.map((skill, index) => {
            const selected = index === skillSuggestionIndex;

            return (
              <button
                aria-selected={selected}
                className="chat-workbench-skill-suggestion"
                data-selected={selected}
                key={skill.id || skill.name}
                role="option"
                type="button"
                onClick={() => onSkillSuggestionApply?.(skill)}
                onMouseDown={event => event.preventDefault()}
                onMouseEnter={() => onSkillSuggestionIndexChange?.(index)}
              >
                <span className="chat-workbench-skill-suggestion-name">
                  /{skill.name}
                </span>
                {skill.description ? (
                  <span className="chat-workbench-skill-suggestion-desc">
                    {skill.description}
                  </span>
                ) : null}
              </button>
            );
          })}
        </div>
      ) : null}
      <div className="chat-workbench-rich-input" ref={richInputRef}>
        <WorkbenchAtSegments
          segments={atSegments}
          onSegmentRemove={onAtSegmentRemove}
        />
        <WorkbenchAtInline
          draft={atDraft}
          onAnchorRectChange={onAtAnchorRectChange}
          onDraftCancel={onAtDraftCancel}
          onDraftQueryChange={onAtDraftQueryChange}
        />
        <TextArea
          aria-label="任务描述"
          autosize={false}
          rows={hasRichContent ? 1 : 3}
          value={value}
          onBlur={onTextareaBlur}
          onChange={onChange}
          onFocus={onTextareaFocus}
          onKeyDown={onSkillSuggestionKeyDown}
          placeholder={isDeerFlow && !hasRichContent ? '今天想做什么？' : ''}
          className="chat-workbench-input"
        />
      </div>
      {!value && !isDeerFlow && !hasRichContent ? (
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
  mode,
  open,
  placement,
  onOpenChange,
  onModeChange,
}: {
  mode: WorkbenchMode;
  open: boolean;
  placement: WorkbenchComposerOverlayPlacement;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
}) => {
  const activeOption =
    DEERFLOW_MODE_OPTIONS.find(option => option.value === mode) ??
    DEERFLOW_MODE_OPTIONS.find(
      option => option.value === DEFAULT_WORKBENCH_MODE,
    ) ??
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
        <span>{WORKBENCH_MODE_LABELS[activeOption.value]}</span>
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
              aria-checked={option.value === mode}
              className="chat-workbench-deerflow-mode-option"
              data-active={option.value === mode}
              onClick={() => {
                onModeChange(option.value);
                onOpenChange(false);
              }}
            >
              <span className="chat-workbench-deerflow-mode-option-icon">
                {option.icon}
              </span>
              <span className="chat-workbench-deerflow-mode-option-copy">
                <span>{WORKBENCH_MODE_LABELS[option.value]}</span>
                <span>{WORKBENCH_MODE_DESCRIPTIONS[option.value]}</span>
              </span>
              <span className="chat-workbench-deerflow-mode-option-check">
                {option.value === mode ? '✓' : ''}
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
  stopLoading,
  stopMode,
  onSubmit,
}: {
  canSend: boolean;
  isDeerFlow: boolean;
  loading: boolean;
  stopLoading?: boolean;
  stopMode?: boolean;
  onSubmit: () => void;
}) =>
  isDeerFlow ? (
    <button
      type="button"
      aria-label={stopMode ? '停止任务' : '发送任务'}
      className="chat-workbench-send chat-workbench-send-deerflow"
      data-status={stopMode ? 'streaming' : 'ready'}
      disabled={stopMode ? stopLoading : !canSend || loading}
      onClick={onSubmit}
    >
      {stopMode ? <DeerFlowStopIcon /> : <DeerFlowArrowUpIcon />}
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
  stopLoading,
  stopMode,
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
      stopLoading={stopLoading}
      stopMode={stopMode}
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
            mode={mode}
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
            settings={runtimeSettings}
            onChange={onResourceSelectionChange}
            onSettingsChange={onRuntimeSettingsChange}
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
          settings={runtimeSettings}
          onChange={onResourceSelectionChange}
          onSettingsChange={onRuntimeSettingsChange}
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
