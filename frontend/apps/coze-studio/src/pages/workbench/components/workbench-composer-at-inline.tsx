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

import { useEffect, useRef, type KeyboardEvent } from 'react';

import type { WorkbenchAtDraft } from './workbench-composer-at-menu';

export const WorkbenchAtInline = ({
  draft,
  disabled = false,
  onAnchorRectChange,
  onDraftCancel,
  onDraftQueryChange,
}: {
  draft?: WorkbenchAtDraft | null;
  disabled?: boolean;
  onAnchorRectChange?: (rect: DOMRect) => void;
  onDraftCancel?: () => void;
  onDraftQueryChange?: (query: string) => void;
}) => {
  const inputRef = useRef<HTMLInputElement>(null);
  const prefixRef = useRef<HTMLSpanElement>(null);
  const draftStage = draft?.stage;
  const draftResourceType =
    draft?.stage === 'resource-search' ? draft.resourceType : '';

  useEffect(() => {
    if (!draftStage || !prefixRef.current) {
      return;
    }

    onAnchorRectChange?.(prefixRef.current.getBoundingClientRect());
  }, [draftStage, draftResourceType, onAnchorRectChange]);

  useEffect(() => {
    if (draftStage && !disabled) {
      inputRef.current?.focus();
    }
  }, [disabled, draftStage, draftResourceType]);

  if (draft) {
    const handleKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
      if (event.key !== 'Backspace' || draft.query) {
        return;
      }

      event.preventDefault();
      onDraftCancel?.();
    };

    const isResourceSearch = draft.stage === 'resource-search';
    const resourceType = isResourceSearch ? draft.resourceType : undefined;
    const placeholder = resourceType
      ? `请输入搜索${resourceType}`
      : '选择资源类型';
    const ariaLabel = resourceType ? `@${resourceType}搜索` : '@资源搜索';

    return (
      <div className="chat-workbench-at-inline" aria-label="@资源引用">
        <span className="chat-workbench-at-prefix" ref={prefixRef}>
          {resourceType ? `@${resourceType}：` : '@'}
        </span>
        <label className="chat-workbench-at-placeholder">
          <input
            ref={inputRef}
            aria-label={ariaLabel}
            disabled={disabled}
            value={draft.query}
            placeholder={placeholder}
            onChange={event => {
              if (!disabled) {
                onDraftQueryChange?.(event.target.value);
              }
            }}
            onKeyDown={handleKeyDown}
          />
        </label>
      </div>
    );
  }
  return null;
};
