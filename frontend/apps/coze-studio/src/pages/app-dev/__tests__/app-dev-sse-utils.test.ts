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

import { describe, expect, it } from 'vitest';

import { projectAppDevEvent, sanitizeToolPayload } from '../utils/sse-utils';

describe('AppDev SSE projection', () => {
  it('merges streamed messages and closes streaming state on prompt end', () => {
    const started = projectAppDevEvent([], {
      event: 'prompt_start',
      data: { requestId: 'request-1' },
    });
    const firstChunk = projectAppDevEvent(started, {
      event: 'agent_message_chunk',
      data: { id: 'assistant-1', content: '页面' },
    });
    const secondChunk = projectAppDevEvent(firstChunk, {
      event: 'agent_message_chunk',
      data: { id: 'assistant-1', content: '已更新' },
    });
    const completed = projectAppDevEvent(secondChunk, {
      event: 'prompt_end',
      data: {},
    });

    expect(completed).toEqual([
      expect.objectContaining({ type: 'section', title: '任务开始' }),
      expect.objectContaining({
        id: 'assistant-1',
        content: '页面已更新',
        isStreaming: false,
      }),
    ]);
  });

  it('merges thought chunks without exposing sensitive values', () => {
    const first = projectAppDevEvent([], {
      event: 'agent_thought_chunk',
      data: { id: 'thought-1', content: '读取 token=secret-token ' },
    });
    const second = projectAppDevEvent(first, {
      event: 'agent_thought_chunk',
      data: { id: 'thought-1', content: '继续处理' },
    });

    expect(second).toHaveLength(1);
    expect(second[0]).toEqual(
      expect.objectContaining({
        type: 'thinking',
        content: '读取 [已隐藏敏感信息] 继续处理',
      }),
    );
    expect(second[0].content).not.toContain('secret-token');
  });

  it('sanitizes tool summaries and error messages', () => {
    const tool = sanitizeToolPayload({
      title: '读取 s3://private-bucket/object',
      summary: 'Bearer private-token',
      rawProviderBody: 'must not be projected',
    });
    const errors = projectAppDevEvent([], {
      event: 'error',
      data: { message: 'api_key=private-key request failed' },
    });

    expect(tool.title).not.toContain('private-bucket');
    expect(tool.content).not.toContain('private-token');
    expect(JSON.stringify(tool)).not.toContain('rawProviderBody');
    expect(errors[0].content).toContain('[已隐藏敏感信息]');
    expect(errors[0].content).not.toContain('private-key');
  });

  it('ignores unknown and heartbeat events', () => {
    const messages = [
      {
        id: 'existing',
        type: 'assistant' as const,
        content: '已有消息',
      },
    ];

    expect(projectAppDevEvent(messages, { event: 'heartbeat', data: {} })).toBe(
      messages,
    );
    expect(projectAppDevEvent(messages, { event: 'unknown', data: {} })).toBe(
      messages,
    );
  });
});
