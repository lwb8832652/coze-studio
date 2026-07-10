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

import { formatFileSize } from '../utils/file-utils';
import type { AppDevChatAttachment, AppDevChatMessage } from '../types';

type MessageContentSegment =
  | {
      type: 'text';
      value: string;
    }
  | {
      type: 'code';
      language: string;
      value: string;
    };

const parseMessageContent = (value: string): MessageContentSegment[] => {
  const segments: MessageContentSegment[] = [];
  const codeBlockPattern = /```([a-zA-Z0-9_-]*)\n?([\s\S]*?)```/gu;
  let cursor = 0;
  let match = codeBlockPattern.exec(value);

  while (match) {
    const [raw, language, code] = match;
    if (match.index > cursor) {
      segments.push({
        type: 'text',
        value: value.slice(cursor, match.index),
      });
    }
    segments.push({
      type: 'code',
      language,
      value: code.trimEnd(),
    });
    cursor = match.index + raw.length;
    match = codeBlockPattern.exec(value);
  }

  if (cursor < value.length) {
    segments.push({
      type: 'text',
      value: value.slice(cursor),
    });
  }

  return segments.length ? segments : [{ type: 'text', value }];
};

const renderTextBlock = (value: string) =>
  value
    .split(/\n{2,}/u)
    .filter(Boolean)
    .map((block, index) => {
      const trimmed = block.trim();
      if (/^#{1,4}\s/u.test(trimmed)) {
        return (
          <h4 key={`${index}-${trimmed}`}>
            {trimmed.replace(/^#{1,4}\s/u, '')}
          </h4>
        );
      }

      const lines = trimmed.split('\n').filter(Boolean);
      if (lines.length && lines.every(line => /^[-*]\s+/u.test(line.trim()))) {
        return (
          <ul key={`${index}-${trimmed}`}>
            {lines.map(line => (
              <li key={line}>{line.trim().replace(/^[-*]\s+/u, '')}</li>
            ))}
          </ul>
        );
      }

      return <p key={`${index}-${trimmed}`}>{block}</p>;
    });

const MessageContent = ({ value }: { value: string }) => (
  <div className="app-dev-chat-message__content">
    {parseMessageContent(value).map((segment, index) =>
      segment.type === 'code' ? (
        <pre key={`${index}-${segment.language}`}>
          <span>
            <em>{segment.language || 'code'}</em>
            <button
              type="button"
              onClick={() => void navigator.clipboard?.writeText(segment.value)}
            >
              复制
            </button>
          </span>
          <code>{segment.value}</code>
        </pre>
      ) : (
        renderTextBlock(segment.value)
      ),
    )}
  </div>
);

const MESSAGE_LABELS: Record<AppDevChatMessage['type'], string> = {
  user: '用户需求',
  assistant: 'AI 回复',
  thinking: '思考过程',
  tool_call: '工具调用',
  tool_call_update: '工具进度',
  error: '异常',
  section: '阶段',
};

const MESSAGE_AVATARS: Record<AppDevChatMessage['type'], string> = {
  user: 'You',
  assistant: 'AI',
  thinking: '…',
  tool_call: '{}',
  tool_call_update: '↻',
  error: '!',
  section: '#',
};

const formatMessageTime = (value?: string) => {
  if (!value) {
    return '';
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
};

const attachmentTypeLabel = (type: AppDevChatAttachment['type']) => {
  if (type === 'prototype_image') {
    return '原型图';
  }
  if (type === 'image') {
    return '图片';
  }

  return '文件';
};

export const ChatMessage = ({ message }: { message: AppDevChatMessage }) => {
  const messageTime = formatMessageTime(message.createdAt);

  return (
    <article className="app-dev-chat-message" data-type={message.type}>
      <header className="app-dev-chat-message__header">
        <span className="app-dev-chat-message__avatar" aria-hidden="true">
          {MESSAGE_AVATARS[message.type]}
        </span>
        <div>
          <strong>{message.title || MESSAGE_LABELS[message.type]}</strong>
          <span>
            {MESSAGE_LABELS[message.type]}
            {messageTime ? ` · ${messageTime}` : ''}
          </span>
        </div>
        {message.isStreaming ? (
          <em className="app-dev-chat-message__streaming">生成中</em>
        ) : null}
      </header>
      <MessageContent value={message.content} />
      {message.attachments?.length ? (
        <div className="app-dev-chat-message__attachments">
          {message.attachments.map(attachment => (
            <span key={attachment.id || attachment.path}>
              <strong>{attachmentTypeLabel(attachment.type)}</strong>
              <em>{attachment.name}</em>
              {attachment.size ? (
                <small>{formatFileSize(attachment.size)}</small>
              ) : null}
              <small>{attachment.path}</small>
            </span>
          ))}
        </div>
      ) : null}
    </article>
  );
};
