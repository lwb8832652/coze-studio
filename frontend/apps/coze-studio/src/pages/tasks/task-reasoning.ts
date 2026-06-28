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

const THINKING_TAG_PATTERN = /<think>[\s\S]*?<\/think>/gi;

const getString = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
};

const extractThinkingTagContent = (value: string) => {
  const contents: string[] = [];

  for (const match of value.matchAll(THINKING_TAG_PATTERN)) {
    const content = match[0]
      .replace(/^<think>/i, '')
      .replace(/<\/think>$/i, '')
      .trim();

    if (content) {
      contents.push(content);
    }
  }

  return contents.join('\n\n');
};

const getReasoningPartsText = (
  payload: Record<string, unknown> | undefined,
) => {
  const value = payload?.reasoning_parts;

  if (!Array.isArray(value)) {
    return '';
  }

  return value
    .map(part => {
      if (typeof part === 'string') {
        return part.trim();
      }

      if (part && typeof part === 'object' && !Array.isArray(part)) {
        const { text } = part as Record<string, unknown>;

        return typeof text === 'string' ? text.trim() : '';
      }

      return '';
    })
    .filter(Boolean)
    .join('\n\n');
};

export const stripTaskThinkingTags = (value: string) =>
  value.replace(THINKING_TAG_PATTERN, '').trim();

export const getTaskReasoningContent = (
  payload: Record<string, unknown> | undefined,
  rawMessage = '',
) =>
  [
    getString(payload, 'reasoning_content'),
    getString(payload, 'reasoning'),
    getString(payload, 'thinking'),
    getString(payload, 'thought'),
    getReasoningPartsText(payload),
    extractThinkingTagContent(rawMessage),
  ]
    .filter(Boolean)
    .join('\n\n')
    .trim();
