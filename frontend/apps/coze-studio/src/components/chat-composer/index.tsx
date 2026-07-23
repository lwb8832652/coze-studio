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

/* eslint-disable @typescript-eslint/naming-convention, @coze-arch/max-line-per-function -- Composer orchestration keeps external editor attributes and interaction state together. */

import {
  useRef,
  type CompositionEventHandler,
  type KeyboardEventHandler,
  type ReactNode,
  type Ref,
} from 'react';

import {
  IconCozSendFill,
  IconCozStopCircle,
} from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';

import './index.less';

export type ChatComposerVariant = 'hero' | 'docked';

export interface ChatComposerEditorProps {
  'aria-label': string;
  disabled: boolean;
  readOnly: boolean;
  placeholder: string;
  rows: number;
  value: string;
  onChange: (value: string) => void;
  onCompositionEnd: CompositionEventHandler<HTMLTextAreaElement>;
  onCompositionStart: CompositionEventHandler<HTMLTextAreaElement>;
  onKeyDown: KeyboardEventHandler<HTMLTextAreaElement>;
}

export interface ChatComposerProps {
  value: string;
  variant: ChatComposerVariant;
  onChange: (value: string) => void;
  onSubmit: (value: string) => void | Promise<void>;
  placeholder?: string;
  disabled?: boolean;
  readOnly?: boolean;
  submitting?: boolean;
  streaming?: boolean;
  stopping?: boolean;
  onStop?: () => void | Promise<void>;
  modelControl?: ReactNode;
  skillControl?: ReactNode;
  toolControl?: ReactNode;
  attachments?: ReactNode;
  footerStart?: ReactNode;
  footerEnd?: ReactNode;
  overlay?: ReactNode;
  auxiliary?: ReactNode;
  error?: ReactNode;
  className?: string;
  ariaLabel?: string;
  inputAriaLabel?: string;
  composerStyle?: string;
  rootRef?: Ref<HTMLElement>;
  showDefaultAction?: boolean;
  renderEditor?: (props: ChatComposerEditorProps) => ReactNode;
  onEditorKeyDown?: KeyboardEventHandler<HTMLTextAreaElement>;
}

export const ChatComposer = ({
  value,
  variant,
  onChange,
  onSubmit,
  placeholder = '',
  disabled = false,
  readOnly = false,
  submitting = false,
  streaming = false,
  stopping = false,
  onStop,
  modelControl,
  skillControl,
  toolControl,
  attachments,
  footerStart,
  footerEnd,
  overlay,
  auxiliary,
  error,
  className,
  ariaLabel = '消息输入',
  inputAriaLabel = '消息',
  composerStyle,
  rootRef,
  showDefaultAction = true,
  renderEditor,
  onEditorKeyDown,
}: ChatComposerProps) => {
  const composingRef = useRef(false);
  const editorDisabled = disabled || submitting || streaming;
  const canSubmit = Boolean(value.trim()) && !editorDisabled && !readOnly;
  const actionDisabled = streaming
    ? disabled || stopping || !onStop
    : !canSubmit;
  const rootClassName = ['chat-composer', className].filter(Boolean).join(' ');
  const hasFooterContent = Boolean(
    footerStart ||
      footerEnd ||
      modelControl ||
      skillControl ||
      toolControl ||
      showDefaultAction,
  );

  const handlePrimaryAction = () => {
    if (streaming) {
      if (!actionDisabled) {
        void onStop?.();
      }
      return;
    }

    if (canSubmit) {
      void onSubmit(value);
    }
  };

  const handleEditorKeyDown: KeyboardEventHandler<
    HTMLTextAreaElement
  > = event => {
    onEditorKeyDown?.(event);

    if (
      event.defaultPrevented ||
      event.key !== 'Enter' ||
      event.shiftKey ||
      event.nativeEvent.isComposing ||
      composingRef.current
    ) {
      return;
    }

    event.preventDefault();
    handlePrimaryAction();
  };
  const handleCompositionEnd: CompositionEventHandler<
    HTMLTextAreaElement
  > = () => {
    composingRef.current = false;
  };
  const handleCompositionStart: CompositionEventHandler<
    HTMLTextAreaElement
  > = () => {
    composingRef.current = true;
  };

  const editorProps: ChatComposerEditorProps = {
    'aria-label': inputAriaLabel,
    disabled: editorDisabled,
    readOnly,
    placeholder,
    rows: variant === 'hero' ? 3 : 1,
    value,
    onChange,
    onCompositionEnd: handleCompositionEnd,
    onCompositionStart: handleCompositionStart,
    onKeyDown: handleEditorKeyDown,
  };

  return (
    <>
      <section
        ref={rootRef}
        aria-busy={submitting || stopping || undefined}
        aria-label={ariaLabel}
        className={rootClassName}
        data-composer-style={composerStyle}
        data-variant={variant}
        role="group"
        onCompositionEndCapture={() => {
          composingRef.current = false;
        }}
        onCompositionStartCapture={() => {
          composingRef.current = true;
        }}
      >
        {overlay}
        {attachments ? (
          <div className="chat-composer__attachments">{attachments}</div>
        ) : null}
        <div className="chat-composer__editor">
          {renderEditor ? (
            renderEditor(editorProps)
          ) : (
            <TextArea
              aria-label={editorProps['aria-label']}
              autosize={{
                minRows: editorProps.rows,
                maxRows: variant === 'hero' ? 8 : 6,
              }}
              className="chat-composer__textarea"
              disabled={editorProps.disabled}
              placeholder={editorProps.placeholder}
              readOnly={editorProps.readOnly}
              value={editorProps.value}
              onChange={editorProps.onChange}
              onCompositionEnd={editorProps.onCompositionEnd}
              onCompositionStart={editorProps.onCompositionStart}
              onKeyDown={editorProps.onKeyDown}
            />
          )}
        </div>
        {auxiliary}
        {hasFooterContent ? (
          <footer
            className="chat-composer__footer"
            data-custom-layout={!showDefaultAction || undefined}
          >
            <div className="chat-composer__footer-start">
              {footerStart}
              {skillControl}
              {toolControl}
            </div>
            <div className="chat-composer__footer-end">
              {modelControl}
              {footerEnd}
              {showDefaultAction ? (
                <Button
                  aria-label={streaming ? '停止生成' : '发送消息'}
                  className="chat-composer__primary-action"
                  data-state={streaming ? 'streaming' : 'ready'}
                  disabled={actionDisabled}
                  icon={streaming ? <IconCozStopCircle /> : <IconCozSendFill />}
                  loading={!streaming && submitting}
                  type="button"
                  onClick={handlePrimaryAction}
                />
              ) : null}
            </div>
          </footer>
        ) : null}
      </section>
      {error ? (
        <div className="chat-composer__error" role="alert">
          {error}
        </div>
      ) : null}
    </>
  );
};
