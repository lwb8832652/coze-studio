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

import { mergeJournalTaskEvents } from '../task-detail-journal-events';

describe('mergeJournalTaskEvents', () => {
  it('prefers journal-backed message and tool steps while keeping non-message events', () => {
    const events = mergeJournalTaskEvents({
      runEvents: [
        {
          event_id: '1',
          thread_id: '7',
          run_id: '42',
          event_type: 'plan.task.updated',
          payload: '{"subject":"生成文档","status":"running"}',
          created_at: 1000,
        },
        {
          event_id: '2',
          thread_id: '7',
          run_id: '42',
          event_type: 'message.completed',
          payload: '{"role":"assistant","content":"raw event"}',
          created_at: 1001,
        },
        {
          event_id: '3',
          thread_id: '7',
          run_id: '42',
          event_type: 'tool.completed',
          payload: '{"tool_call_id":"call_search","content":"raw result"}',
          created_at: 1002,
        },
      ],
      journalMessages: [
        {
          id: 'event-2',
          thread_id: '7',
          run_id: '42',
          type: 'ai',
          role: 'assistant',
          content: '',
          name: '',
          tool_call_id: '',
          tool_calls: [
            {
              id: 'call_search',
              name: 'web_search',
              type: 'function',
              arguments: '{"query":"青岛最佳旅游时间"}',
            },
          ],
          additional_kwargs: '{"reasoning_content":"先搜索资料"}',
          usage: '{}',
          created_at: 1001,
          source_event_id: '2',
        },
        {
          id: 'event-3',
          thread_id: '7',
          run_id: '42',
          type: 'tool',
          role: 'tool',
          content: '',
          name: 'web_search',
          tool_call_id: 'call_search',
          tool_calls: [],
          additional_kwargs: '{}',
          usage: '{}',
          created_at: 1002,
          source_event_id: '3',
        },
      ],
    });

    expect(events.map(event => event.event_type)).toEqual([
      'plan.task.updated',
      'message.completed',
      'tool.completed',
    ]);
    expect(events[1].payload).toContain('先搜索资料');
    expect(events[1].payload).toContain('青岛最佳旅游时间');
    expect(events[1].payload).not.toContain('raw event');
    expect(events[2].payload).not.toContain('raw result');
  });
});
