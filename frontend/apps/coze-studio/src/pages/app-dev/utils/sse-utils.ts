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

import type { AppDevChatMessage, AppDevSSEEvent } from '../types';

const safeString = (value: unknown) => (typeof value === 'string' ? value : '');
const sensitivePatterns = [
  /AKIA[0-9A-Z]{16}/g,
  /sk-[A-Za-z0-9_-]{16,}/g,
  /Bearer\s+[A-Za-z0-9._-]+/gi,
  /(password|secret|token|credential|api[_-]?key)\s*[:=]\s*[^,\s}]+/gi,
  /(s3|oss|cos):\/\/[^\s]+/gi,
];

const redactSensitiveText = (value: string) =>
  sensitivePatterns.reduce(
    (content, pattern) => content.replace(pattern, '[已隐藏敏感信息]'),
    value.slice(0, 2000),
  );

const upsertStreamingMessage = (
  messages: AppDevChatMessage[],
  nextMessage: AppDevChatMessage,
) => {
  const index = messages.findIndex(message => message.id === nextMessage.id);
  if (index < 0) {
    return [...messages, nextMessage];
  }

  return messages.map((message, messageIndex) =>
    messageIndex === index
      ? {
          ...message,
          content: `${message.content}${nextMessage.content}`,
          isStreaming: nextMessage.isStreaming,
        }
      : message,
  );
};

export const sanitizeToolPayload = (payload: Record<string, unknown>) => {
  const title = redactSensitiveText(safeString(payload.title)) || '工具调用';
  const summary =
    redactSensitiveText(safeString(payload.summary)) || '正在执行工具调用';

  return {
    title,
    content: summary,
  };
};

export const projectAppDevEvent = (
  messages: AppDevChatMessage[],
  event: AppDevSSEEvent,
) => {
  const now = new Date().toISOString();

  switch (event.event) {
    case 'prompt_start':
      return [
        ...messages,
        {
          id: `section_${safeString(event.data.requestId) || now}`,
          type: 'section' as const,
          title: '任务开始',
          content: 'AI 开始处理网页应用开发任务',
          createdAt: now,
        },
      ];
    case 'agent_thought_chunk':
      return upsertStreamingMessage(messages, {
        id: safeString(event.data.id) || 'thinking',
        type: 'thinking',
        title: '思考过程',
        content: redactSensitiveText(safeString(event.data.content)),
        isStreaming: true,
        createdAt: now,
      });
    case 'tool_call': {
      const tool = sanitizeToolPayload(event.data);
      return [
        ...messages,
        {
          id: safeString(event.data.id) || `tool_${now}`,
          type: 'tool_call',
          title: tool.title,
          content: tool.content,
          createdAt: now,
        },
      ];
    }
    case 'tool_call_update': {
      const tool = sanitizeToolPayload(event.data);
      return [
        ...messages,
        {
          id: safeString(event.data.id) || `tool_update_${now}`,
          type: 'tool_call_update',
          title: tool.title,
          content: tool.content,
          createdAt: now,
        },
      ];
    }
    case 'agent_message_chunk':
      return upsertStreamingMessage(messages, {
        id: safeString(event.data.id) || 'assistant',
        type: 'assistant',
        role: 'assistant',
        content: redactSensitiveText(safeString(event.data.content)),
        isStreaming: true,
        createdAt: now,
      });
    case 'prompt_end':
      return messages.map(message =>
        message.isStreaming
          ? {
              ...message,
              isStreaming: false,
            }
          : message,
      );
    case 'error':
      return [
        ...messages,
        {
          id: `error_${now}`,
          type: 'error',
          title: '执行异常',
          content:
            redactSensitiveText(safeString(event.data.message)) ||
            'AI 任务执行失败',
          createdAt: now,
        },
      ];
    default:
      return messages;
  }
};
