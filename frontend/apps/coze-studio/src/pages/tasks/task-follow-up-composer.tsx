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

import type { ReactNode } from 'react';

import { IconCozCross } from '@coze-arch/coze-design/icons';

import { getWorkbenchLLMModels } from '../workbench/service';
import {
  WorkbenchComposer,
  type WorkbenchComposerCapabilities,
} from '../workbench/components/workbench-composer';
import {
  type WorkbenchComposerSubmitPayload,
  type WorkbenchMode,
} from '../workbench/components/types';

export const TaskFollowUpComposer = ({
  value,
  mode,
  loading,
  error,
  spaceId,
  taskId,
  resetKey,
  capabilities,
  footerEnd,
  todoDock,
  suggestions = [],
  suggestionsHidden = false,
  suggestionsLoading = false,
  stopLoading,
  stopMode,
  onDismissSuggestions,
  onSuggestionClick,
  onValueChange,
  onModeChange,
  onStop,
  onSubmit,
}: {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  spaceId?: string;
  taskId?: string;
  resetKey?: string | number;
  capabilities?: Partial<WorkbenchComposerCapabilities>;
  footerEnd?: ReactNode;
  todoDock?: ReactNode;
  suggestions?: string[];
  suggestionsHidden?: boolean;
  suggestionsLoading?: boolean;
  stopLoading?: boolean;
  stopMode?: boolean;
  onDismissSuggestions?: () => void;
  onSuggestionClick?: (suggestion: string) => void;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onStop?: () => void | Promise<void>;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
}) => {
  const showSuggestions =
    !suggestionsHidden && (suggestionsLoading || suggestions.length > 0);
  const interactionDisabled = loading || Boolean(stopMode);

  return (
    <section className="coze-prototype-followup">
      <div className="coze-prototype-followup-stack">
        {todoDock}
        {showSuggestions ? (
          <div className="coze-prototype-followup-suggestions">
            {suggestionsLoading ? (
              <span className="coze-prototype-followup-suggestion-loading">
                正在生成可能的后续问题...
              </span>
            ) : (
              <>
                {suggestions.map(suggestion => (
                  <button
                    key={suggestion}
                    type="button"
                    className="coze-prototype-followup-suggestion"
                    disabled={interactionDisabled}
                    onClick={() => {
                      if (!interactionDisabled) {
                        onSuggestionClick?.(suggestion);
                      }
                    }}
                  >
                    {suggestion}
                  </button>
                ))}
                <button
                  type="button"
                  aria-label="关闭推荐追问"
                  className="coze-prototype-followup-suggestion-close"
                  disabled={interactionDisabled}
                  onClick={
                    interactionDisabled ? undefined : onDismissSuggestions
                  }
                >
                  <IconCozCross />
                </button>
              </>
            )}
          </div>
        ) : null}
        <WorkbenchComposer
          value={value}
          mode={mode}
          loading={loading}
          error={error}
          variant="detail"
          presentation="deerflow"
          spaceId={spaceId}
          taskId={taskId}
          resetKey={resetKey}
          capabilities={capabilities}
          footerEnd={footerEnd}
          stopLoading={stopLoading}
          stopMode={stopMode}
          modelLoader={getWorkbenchLLMModels}
          onValueChange={onValueChange}
          onModeChange={onModeChange}
          onStop={onStop}
          onSubmit={onSubmit}
        />
      </div>
    </section>
  );
};
