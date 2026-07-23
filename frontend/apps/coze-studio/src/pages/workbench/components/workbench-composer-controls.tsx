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

/* eslint-disable @coze-arch/max-line-per-function -- Composer body and toolbar retain shared accessibility and interaction state. */

/* eslint-disable max-lines -- P0 composer controls stay colocated until phase 2 split. */

import {
  useEffect,
  useRef,
  type CompositionEventHandler,
  type KeyboardEventHandler,
  type ReactNode,
  type Dispatch,
  type SetStateAction,
} from 'react';

import {
  IconCozArrowDown,
  IconCozAt,
  IconCozCheckMark,
  IconCozCross,
  IconCozLink,
  IconCozLightbulbFill,
  IconCozDiamondFill,
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
  DeerFlowFlashIcon,
  DeerFlowSendIcon,
  DeerFlowStopIcon,
  DeerFlowUltraIcon,
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
  WORKBENCH_MODES,
  workbenchModelTypeToNumber,
  type WorkbenchComposerVariant,
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
  attachmentsEnabled?: boolean;
  attachmentDisabledReason?: string;
  atMenuOpen: boolean;
  canSend: boolean;
  disabled?: boolean;
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
  onReloadModels: () => void | Promise<void>;
  onModeMenuOpenChange: (open: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onResourceSelectionChange: (selection: WorkbenchResourceSelection) => void;
  onRuntimeSettingsChange: WorkbenchRuntimeSettingsChange;
  onSelectedModelTypeChange: (modelType: number) => void;
  onAttachClick?: () => void;
  onSubmit: () => void;
}

const DEERFLOW_MODE_OPTIONS: Array<{
  icon: ReactNode;
  value: WorkbenchMode;
}> = [
  {
    value: 'flash',
    icon: <DeerFlowFlashIcon />,
  },
  {
    value: 'thinking',
    icon: <IconCozLightbulbFill />,
  },
  {
    value: 'pro',
    icon: <IconCozDiamondFill />,
  },
  {
    value: 'ultra',
    icon: <DeerFlowUltraIcon />,
  },
];

const WorkbenchComposerAttachments = ({
  disabled,
  files,
  onFileRemove,
}: {
  disabled?: boolean;
  files?: File[];
  onFileRemove?: (file: File) => void;
}) => {
  if (!files?.length) {
    return null;
  }

  return (
    <div className="chat-workbench-attachments" aria-label="附件列表">
      {files.map((file, index) => (
        <span
          className="chat-workbench-attachment-chip"
          key={`${file.name}-${file.size}-${file.lastModified}-${index}`}
        >
          <IconCozUpload />
          <span>{file.name}</span>
          <button
            type="button"
            aria-label={`移除附件 ${file.name}`}
            disabled={disabled}
            onClick={() => onFileRemove?.(file)}
          >
            <IconCozCross />
          </button>
        </span>
      ))}
    </div>
  );
};

export const WorkbenchComposerBody = ({
  atDraft,
  atSegments,
  files,
  variant = 'home',
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
  onFileRemove,
  onTextareaBlur,
  onTextareaFocus,
  disabled = false,
  readOnly = false,
  onCompositionEnd,
  onCompositionStart,
}: {
  atDraft?: WorkbenchAtDraft | null;
  atSegments?: WorkbenchAtSegment[];
  files?: File[];
  variant?: WorkbenchComposerVariant;
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
  onFileRemove?: (file: File) => void;
  onTextareaBlur?: () => void;
  onTextareaFocus?: () => void;
  disabled?: boolean;
  readOnly?: boolean;
  onCompositionEnd?: CompositionEventHandler<HTMLTextAreaElement>;
  onCompositionStart?: CompositionEventHandler<HTMLTextAreaElement>;
}) => {
  const isDeerFlow = presentation === 'deerflow';
  const hasRichContent = Boolean(
    atSegments?.length || atDraft || files?.length,
  );
  const richInputRef = useRef<HTMLDivElement>(null);
  const hadDraftRef = useRef(Boolean(atDraft));
  const autosize =
    variant === 'detail'
      ? { minRows: 1, maxRows: 6 }
      : { minRows: 3, maxRows: 8 };

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
                disabled={disabled || readOnly}
                onClick={() => {
                  if (!disabled && !readOnly) {
                    onSkillSuggestionApply?.(skill);
                  }
                }}
                onMouseDown={event => event.preventDefault()}
                onMouseEnter={() => {
                  if (!disabled && !readOnly) {
                    onSkillSuggestionIndexChange?.(index);
                  }
                }}
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
      <div
        aria-disabled={disabled || undefined}
        className="chat-workbench-rich-input"
        data-disabled={disabled || readOnly || undefined}
        ref={richInputRef}
      >
        <WorkbenchComposerAttachments
          disabled={disabled || readOnly}
          files={files}
          onFileRemove={onFileRemove}
        />
        <WorkbenchAtSegments
          disabled={disabled || readOnly}
          segments={atSegments}
          onSegmentRemove={onAtSegmentRemove}
        />
        <WorkbenchAtInline
          disabled={disabled || readOnly}
          draft={atDraft}
          onAnchorRectChange={onAtAnchorRectChange}
          onDraftCancel={onAtDraftCancel}
          onDraftQueryChange={onAtDraftQueryChange}
        />
        <TextArea
          aria-label="任务描述"
          autosize={autosize}
          disabled={disabled}
          readOnly={readOnly}
          value={value}
          onBlur={onTextareaBlur}
          onChange={onChange}
          onCompositionEnd={onCompositionEnd}
          onCompositionStart={onCompositionStart}
          onFocus={onTextareaFocus}
          onKeyDown={
            disabled || readOnly
              ? event => event.preventDefault()
              : onSkillSuggestionKeyDown
          }
          placeholder={isDeerFlow && !hasRichContent ? '今天想做什么？' : ''}
          className="chat-workbench-input"
        />
      </div>
      {!value && !isDeerFlow && !hasRichContent ? (
        <div
          className="chat-workbench-composer-prompt"
          aria-label="当前模式提示"
        >
          <span aria-hidden="true">
            {DEERFLOW_MODE_OPTIONS.find(option => option.value === mode)?.icon}
          </span>
          <span>{WORKBENCH_MODE_PROMPTS[mode]}</span>
        </div>
      ) : null}
    </div>
  );
};

const WorkbenchModeSelector = ({
  disabled,
  mode,
  onModeChange,
}: {
  disabled?: boolean;
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
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            onModeChange(item);
          }
        }}
      >
        {item}
      </button>
    ))}
  </div>
);

const WorkbenchDeerFlowModeSelector = ({
  disabled,
  mode,
  open,
  placement,
  onOpenChange,
  onModeChange,
}: {
  disabled?: boolean;
  mode: WorkbenchMode;
  open: boolean;
  placement: WorkbenchComposerOverlayPlacement;
  onOpenChange: (open: boolean) => void;
  onModeChange: (mode: WorkbenchMode) => void;
}) => {
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const activeOption =
    DEERFLOW_MODE_OPTIONS.find(option => option.value === mode) ??
    DEERFLOW_MODE_OPTIONS.find(
      option => option.value === DEFAULT_WORKBENCH_MODE,
    ) ??
    DEERFLOW_MODE_OPTIONS[0];

  useEffect(() => {
    if (!open || disabled) {
      return;
    }

    const closeAndRestoreFocus = () => {
      onOpenChange(false);
      queueMicrotask(() => triggerRef.current?.focus());
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeAndRestoreFocus();
      }
    };
    const handleMouseDown = (event: MouseEvent) => {
      if (
        event.target instanceof Node &&
        !rootRef.current?.contains(event.target)
      ) {
        closeAndRestoreFocus();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('mousedown', handleMouseDown);

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('mousedown', handleMouseDown);
    };
  }, [disabled, onOpenChange, open]);

  return (
    <div ref={rootRef} className="chat-workbench-deerflow-mode">
      <button
        ref={triggerRef}
        type="button"
        className="chat-workbench-deerflow-mode-trigger"
        aria-label="选择模式"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            onOpenChange(!open);
          }
        }}
      >
        <span aria-hidden="true">{activeOption.icon}</span>
        <span className="chat-workbench-deerflow-mode-label">
          {WORKBENCH_MODE_LABELS[activeOption.value]}
        </span>
        <IconCozArrowDown className="chat-workbench-deerflow-mode-arrow" />
      </button>
      {open ? (
        <div
          className="chat-workbench-deerflow-mode-menu"
          data-placement={placement}
          role="menu"
          onKeyDown={event => {
            if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') {
              return;
            }
            event.preventDefault();
            const items = Array.from(
              event.currentTarget.querySelectorAll<HTMLButtonElement>(
                '[role="menuitemradio"]:not(:disabled)',
              ),
            );
            if (!items.length) {
              return;
            }
            const currentIndex = items.indexOf(
              document.activeElement as HTMLButtonElement,
            );
            const step = event.key === 'ArrowDown' ? 1 : -1;
            items[(currentIndex + step + items.length) % items.length]?.focus();
          }}
        >
          <div className="chat-workbench-deerflow-mode-title">选择模式</div>
          {DEERFLOW_MODE_OPTIONS.map(option => (
            <button
              key={option.value}
              type="button"
              role="menuitemradio"
              aria-checked={option.value === mode}
              disabled={disabled}
              className="chat-workbench-deerflow-mode-option"
              data-active={option.value === mode}
              onClick={() => {
                if (!disabled) {
                  onModeChange(option.value);
                  onOpenChange(false);
                }
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
                {option.value === mode ? <IconCozCheckMark /> : null}
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
      {stopMode ? <DeerFlowStopIcon /> : <DeerFlowSendIcon />}
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
  attachmentsEnabled = true,
  attachmentDisabledReason,
  atMenuOpen,
  canSend,
  disabled = false,
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
  onReloadModels,
  onModeMenuOpenChange,
  onModeChange,
  onResourceSelectionChange,
  onRuntimeSettingsChange,
  onSelectedModelTypeChange,
  onAttachClick,
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
        disabled={disabled}
        loading={modelsLoading}
        models={models}
        value={selectedModelType}
        open={modelMenuOpen}
        placement={overlayPlacement}
        spaceId={spaceId}
        onOpenChange={open => {
          if (!disabled) {
            onModelMenuOpenChange(open);
          }
        }}
        onChange={model =>
          !disabled &&
          onSelectedModelTypeChange(workbenchModelTypeToNumber(model))
        }
        onModelsChanged={onReloadModels}
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
      <div
        className="chat-workbench-toolbar"
        data-disabled={disabled || undefined}
        data-presentation="deerflow"
      >
        <div className="chat-workbench-toolbar-left">
          <button
            type="button"
            className="chat-workbench-icon-action"
            aria-label="添加附件"
            disabled={disabled || !attachmentsEnabled}
            title={attachmentsEnabled ? undefined : attachmentDisabledReason}
            onClick={
              disabled || !attachmentsEnabled ? undefined : onAttachClick
            }
          >
            <IconCozUpload />
          </button>
          <WorkbenchDeerFlowModeSelector
            disabled={disabled}
            mode={mode}
            open={modeMenuOpen}
            placement={overlayPlacement}
            onOpenChange={open => {
              if (!disabled) {
                onModeMenuOpenChange(open);
              }
            }}
            onModeChange={nextMode => {
              if (!disabled) {
                onModeChange(nextMode);
              }
            }}
          />
          <ExtensionsPopover
            disabled={disabled}
            open={extensionsOpen}
            placement={overlayPlacement}
            renderMask={false}
            showSelectedCount={false}
            value={resourceSelection}
            settings={runtimeSettings}
            onChange={selection => {
              if (!disabled) {
                onResourceSelectionChange(selection);
              }
            }}
            onSettingsChange={settings => {
              if (!disabled) {
                onRuntimeSettingsChange(settings);
              }
            }}
            onOpenChange={open => {
              if (!disabled) {
                onExtensionsOpenChange(open);
              }
            }}
          />
        </div>

        <div className="chat-workbench-toolbar-actions">
          {modelSelector}
          <button
            type="button"
            aria-label="添加上下文"
            aria-expanded={atMenuOpen}
            disabled={disabled}
            onClick={() => {
              if (!disabled) {
                onAtMenuOpenChange(!atMenuOpen);
              }
            }}
          >
            <IconCozAt />
          </button>
          {sendButton}
        </div>
      </div>
    );
  }

  return (
    <div
      className="chat-workbench-toolbar"
      data-disabled={disabled || undefined}
    >
      <div className="chat-workbench-toolbar-left">
        <WorkbenchModeSelector
          disabled={disabled}
          mode={mode}
          onModeChange={onModeChange}
        />

        <ExtensionsPopover
          disabled={disabled}
          placement={overlayPlacement}
          value={resourceSelection}
          settings={runtimeSettings}
          onChange={selection => {
            if (!disabled) {
              onResourceSelectionChange(selection);
            }
          }}
          onSettingsChange={settings => {
            if (!disabled) {
              onRuntimeSettingsChange(settings);
            }
          }}
        />

        {spaceId && modelLoader ? modelSelector : null}

        <WorkbenchRuntimeSettingsControl
          disabled={disabled}
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
          disabled={disabled}
          onClick={() => {
            if (!disabled) {
              onAtMenuOpenChange(!atMenuOpen);
            }
          }}
        >
          <IconCozAt />
        </button>
        <button
          type="button"
          aria-label="添加附件"
          disabled={disabled || !attachmentsEnabled}
          title={attachmentsEnabled ? undefined : attachmentDisabledReason}
          onClick={disabled || !attachmentsEnabled ? undefined : onAttachClick}
        >
          <IconCozLink />
        </button>
      </div>
      {sendButton}
    </div>
  );
};
