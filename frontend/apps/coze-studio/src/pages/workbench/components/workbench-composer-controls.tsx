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
  type Dispatch,
  type SetStateAction,
} from 'react';

import {
  IconCozAt,
  IconCozCross,
  IconCozLink,
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
  DeerFlowSendIcon,
  DeerFlowStopIcon,
} from './workbench-deerflow-mode-icons';
import { WorkbenchAtSegments } from './workbench-composer-at-segments';
import type {
  WorkbenchAtDraft,
  WorkbenchAtSegment,
  WorkbenchComposerOverlayPlacement,
} from './workbench-composer-at-menu';
import { WorkbenchAtInline } from './workbench-composer-at-inline';
import {
  workbenchModelTypeToNumber,
  type WorkbenchComposerVariant,
  type WorkbenchLLMModel,
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
  onResourceSelectionChange: (selection: WorkbenchResourceSelection) => void;
  onRuntimeSettingsChange: WorkbenchRuntimeSettingsChange;
  onSelectedModelTypeChange: (modelType: number) => void;
  onAttachClick?: () => void;
  onSubmit: () => void;
}

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
