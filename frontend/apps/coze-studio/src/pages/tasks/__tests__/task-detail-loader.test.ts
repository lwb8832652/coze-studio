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

import { beforeEach, describe, expect, it, vi } from 'vitest';

import { isTaskThreadDetailReadOnly } from '../task-thread-detail-model';
import {
  fetchTaskDetail,
  mergeJournalTaskThreadEvents,
} from '../task-detail-loader';

const mockGetTaskThread = vi.hoisted(() => vi.fn());
const mockListTaskThreadArtifacts = vi.hoisted(() => vi.fn());
const mockListTaskThreadMessages = vi.hoisted(() => vi.fn());
const mockListTaskThreadRunEvents = vi.hoisted(() => vi.fn());
const mockListTaskThreadRuns = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getTaskThread: mockGetTaskThread,
  getTaskThreadTokenUsage: vi.fn(),
  listTaskThreadArtifacts: mockListTaskThreadArtifacts,
  listTaskThreadMessages: mockListTaskThreadMessages,
  listTaskThreadRunEvents: mockListTaskThreadRunEvents,
  listTaskThreadRuns: mockListTaskThreadRuns,
}));

describe('fetchTaskDetail', () => {
  beforeEach(() => {
    mockGetTaskThread.mockReset();

    mockGetTaskThread.mockResolvedValue({
      data: undefined,
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { artifacts: [], total: 0 },
    });
    mockListTaskThreadMessages.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { messages: [], total: 0 },
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { events: [], total: 0 },
    });
    mockListTaskThreadRuns.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: { runs: [], total: 0 },
    });
  });

  it('returns an empty canonical detail when the task thread is missing', async () => {
    await expect(
      fetchTaskDetail({ id: 'missing-thread', spaceId: 'space-1' }),
    ).resolves.toEqual({
      threadId: 'missing-thread',
      task: undefined,
      events: [],
    });
    expect(mockGetTaskThread).toHaveBeenCalledWith({
      thread_id: 'missing-thread',
      space_id: 'space-1',
    });
  });

  it('maps canonical can_edit without requiring creator_id', async () => {
    mockGetTaskThread.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        thread_id: '1001',
        space_id: '9001',
        title: 'Canonical task',
        status: 'running',
        source: 'web',
        progress: 40,
        last_user_message: 'Prepare the brief',
        last_agent_message: 'Working',
        can_edit: false,
        created_at: 1767225600000,
        updated_at: 1767225660000,
      },
    });

    const detail = await fetchTaskDetail({ id: '1001', spaceId: '9001' });

    expect(detail.task).toMatchObject({
      id: '1001',
      can_edit: false,
    });
    expect(detail.task).not.toHaveProperty('creator_id');
    expect(mockGetTaskThread).toHaveBeenCalledWith({
      thread_id: '1001',
      space_id: '9001',
    });
    expect(mockListTaskThreadMessages).toHaveBeenCalledWith({
      thread_id: '1001',
      space_id: '9001',
      page: 1,
      page_size: 50,
    });
    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith({
      thread_id: '1001',
      space_id: '9001',
      page: 1,
      page_size: 100,
    });
    expect(mockListTaskThreadArtifacts).toHaveBeenCalledWith({
      thread_id: '1001',
      space_id: '9001',
      page: 1,
      page_size: 50,
    });
    expect(mockListTaskThreadRuns.mock.calls).toEqual([
      [
        {
          thread_id: '1001',
          space_id: '9001',
          parent_run_id: '0',
          page: 1,
          page_size: 1,
        },
      ],
      [
        {
          thread_id: '1001',
          space_id: '9001',
          page: 1,
          page_size: 20,
        },
      ],
    ]);
  });
});

describe('isTaskThreadDetailReadOnly', () => {
  it('uses canonical can_edit before legacy creator inference', () => {
    const canonicalTask = {
      id: '1001',
      space_id: '9001',
      creator_id: 'current-user',
      can_edit: false,
      title: 'Canonical task',
      status: 3,
      progress: 40,
      created_at: 1767225600000,
      updated_at: 1767225660000,
    };

    expect(
      isTaskThreadDetailReadOnly({
        task: canonicalTask,
        userID: 'current-user',
      }),
    ).toBe(true);
    expect(
      isTaskThreadDetailReadOnly({
        task: { ...canonicalTask, can_edit: true, creator_id: 'other-user' },
        userID: 'current-user',
      }),
    ).toBe(false);
  });
});

describe('mergeJournalTaskThreadEvents', () => {
  it('prefers journal-backed message and tool steps while keeping non-message events', () => {
    const events = mergeJournalTaskThreadEvents({
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
