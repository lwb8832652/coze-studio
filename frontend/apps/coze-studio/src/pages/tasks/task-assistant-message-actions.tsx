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

import { useState } from 'react';

import {
  IconCozCopy,
  IconCozThumbdown,
  IconCozThumbdownFill,
  IconCozThumbsup,
  IconCozThumbsupFill,
} from '@coze-arch/coze-design/icons';

import { copyTextToClipboard } from './task-clipboard';

type TaskAssistantFeedbackValue = 'positive' | 'negative' | undefined;

export const TaskAssistantMessageActions = ({
  copyText,
}: {
  copyText: string;
}) => {
  const [feedback, setFeedback] = useState<TaskAssistantFeedbackValue>();
  const [copied, setCopied] = useState(false);
  const visibleCopyText = copyText.trim();

  const toggleFeedback = (
    nextFeedback: Exclude<TaskAssistantFeedbackValue, undefined>,
  ) => {
    setFeedback(currentFeedback =>
      currentFeedback === nextFeedback ? undefined : nextFeedback,
    );
  };

  const handleCopy = () => {
    void copyTextToClipboard(visibleCopyText).then(didCopy => {
      if (!didCopy) {
        return;
      }

      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    });
  };

  return (
    <div
      className="coze-prototype-assistant-actions"
      data-testid="task-assistant-message-actions"
    >
      <button
        type="button"
        aria-label="复制回复"
        className="coze-prototype-assistant-action"
        disabled={!visibleCopyText}
        onClick={handleCopy}
      >
        <IconCozCopy className="text-[14px]" />
        <span>{copied ? '已复制' : '复制'}</span>
      </button>
      <button
        type="button"
        aria-label="赞"
        className="coze-prototype-assistant-action"
        data-selected={feedback === 'positive'}
        onClick={() => toggleFeedback('positive')}
      >
        {feedback === 'positive' ? (
          <IconCozThumbsupFill className="text-[14px]" />
        ) : (
          <IconCozThumbsup className="text-[14px]" />
        )}
      </button>
      <button
        type="button"
        aria-label="踩"
        className="coze-prototype-assistant-action"
        data-selected={feedback === 'negative'}
        onClick={() => toggleFeedback('negative')}
      >
        {feedback === 'negative' ? (
          <IconCozThumbdownFill className="text-[14px]" />
        ) : (
          <IconCozThumbdown className="text-[14px]" />
        )}
      </button>
    </div>
  );
};
