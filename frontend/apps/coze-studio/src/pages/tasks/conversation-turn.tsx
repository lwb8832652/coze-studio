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

import { forwardRef, type HTMLAttributes, type ReactNode } from 'react';

import { useCommonConfigStore } from '@coze-foundation/global-store';
import { IconCozCheckMark } from '@coze-arch/coze-design/icons';

import { WorkspaceMark } from '../../components/workspace-mark';

const normalizeTimestamp = (value?: number) => {
  if (!value) {
    return undefined;
  }

  return value < 1_000_000_000_000 ? value * 1000 : value;
};

const formatTimestamp = (value?: number) => {
  const timestamp = normalizeTimestamp(value);

  if (!timestamp) {
    return undefined;
  }

  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  }).format(timestamp);
};

const formatDateTime = (value?: number) => {
  const timestamp = normalizeTimestamp(value);

  return timestamp ? new Date(timestamp).toISOString() : undefined;
};

export const TaskConversationColumn = ({
  children,
}: {
  children: ReactNode;
}) => <div className="coze-prototype-conversation-column">{children}</div>;

export const TaskUserTurn = ({
  children,
  createdAt,
}: {
  children: ReactNode;
  createdAt?: number;
}) => {
  const timestamp = formatTimestamp(createdAt);
  const dateTime = formatDateTime(createdAt);

  return (
    <article className="coze-prototype-user-turn">
      <div className="coze-prototype-user-bubble">
        <div className="coze-prototype-user-bubble-content">{children}</div>
        {timestamp ? (
          <time className="coze-prototype-turn-time" dateTime={dateTime}>
            {timestamp}
          </time>
        ) : null}
        <span className="coze-prototype-user-delivery" aria-label="已发送">
          <IconCozCheckMark />
        </span>
      </div>
    </article>
  );
};

interface TaskAssistantTurnShellProps extends HTMLAttributes<HTMLElement> {
  children?: ReactNode;
  createdAt?: number;
  hideHeader?: boolean;
  journalMode?: boolean;
  subtitle: string;
}

export const TaskAssistantTurnShell = forwardRef<
  HTMLElement,
  TaskAssistantTurnShellProps
>(
  (
    {
      children,
      className = '',
      createdAt,
      hideHeader = false,
      journalMode = false,
      subtitle,
      ...rest
    },
    ref,
  ) => {
    const siteName = useCommonConfigStore(state => state.siteConfig.siteName);
    const timestamp = formatTimestamp(createdAt);
    const dateTime = formatDateTime(createdAt);

    return (
      <article
        {...rest}
        ref={ref}
        className={`coze-prototype-assistant-turn coze-prototype-assistant-turn-shell ${
          journalMode ? 'coze-prototype-assistant-turn-journal' : ''
        } ${className}`.trim()}
        tabIndex={-1}
      >
        {hideHeader ? null : (
          <header className="coze-prototype-assistant-turn-header">
            <WorkspaceMark variant="assistant" />
            <span className="coze-prototype-assistant-identity">
              <span>
                <strong>{siteName}</strong>
                {journalMode ? null : <span> · {subtitle}</span>}
              </span>
              {!journalMode && timestamp ? (
                <time dateTime={dateTime}>{timestamp}</time>
              ) : null}
            </span>
          </header>
        )}
        {children ? (
          <div className="coze-prototype-assistant-turn-body">{children}</div>
        ) : null}
      </article>
    );
  },
);

TaskAssistantTurnShell.displayName = 'TaskAssistantTurnShell';
