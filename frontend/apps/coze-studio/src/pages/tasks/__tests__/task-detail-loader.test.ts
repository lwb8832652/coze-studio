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

import {
  isTaskThreadDetailReadOnly,
  TaskThreadDetailStatus,
} from '../task-thread-detail-model';
import {
  fetchTaskDetail,
  mergeJournalTaskThreadEvents,
} from '../task-detail-loader';

const mockGetTaskThread = vi.hoisted(() => vi.fn());
const mockGetTaskThreadTokenUsage = vi.hoisted(() => vi.fn());
const mockListTaskThreadArtifacts = vi.hoisted(() => vi.fn());
const mockListTaskThreadMessages = vi.hoisted(() => vi.fn());
const mockListTaskThreadRunEvents = vi.hoisted(() => vi.fn());
const mockListTaskThreadRuns = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getTaskThread: mockGetTaskThread,
  getTaskThreadTokenUsage: mockGetTaskThreadTokenUsage,
  listTaskThreadArtifacts: mockListTaskThreadArtifacts,
  listTaskThreadMessages: mockListTaskThreadMessages,
  listTaskThreadRunEvents: mockListTaskThreadRunEvents,
  listTaskThreadRuns: mockListTaskThreadRuns,
}));

describe('fetchTaskDetail', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetTaskThread.mockReset();
    mockGetTaskThreadTokenUsage.mockResolvedValue({
      code: 0,
      msg: 'success',
      data: {
        usage: [],
        total: 0,
        aggregate: undefined,
        run_aggregates: [],
      },
    });

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
    expect(mockListTaskThreadRunEvents).not.toHaveBeenCalled();
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
          page_size: 20,
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

  it('loads events from the primary top-level Run instead of a subagent retry worker', async () => {
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
        can_edit: true,
        created_at: 1767225600000,
        updated_at: 1767225660000,
      },
    });
    mockListTaskThreadRuns.mockImplementation(request =>
      Promise.resolve({
        code: 0,
        msg: 'success',
        data: {
          runs:
            request.parent_run_id === '0'
              ? [
                  {
                    run_id: '3001',
                    thread_id: '1001',
                    space_id: '9001',
                    status: 'pending',
                    metadata:
                      '{"source":"subagent_retry","source_run_id":"2101"}',
                    run_kind: 'task',
                    created_at: 1767225700000,
                    updated_at: 1767225700000,
                  },
                  {
                    run_id: '2001',
                    thread_id: '1001',
                    space_id: '9001',
                    status: 'running',
                    metadata: '{}',
                    run_kind: 'task',
                    adaptive_execution: {
                      schema: 'coze.adaptive_execution_public.v1',
                      enabled: true,
                      mode: 'multi_step',
                      safe_summary: '任务需要分步执行。',
                      clarification_question: null,
                    },
                    created_at: 1767225600000,
                    updated_at: 1767225660000,
                  },
                ]
              : [],
          total: request.parent_run_id === '0' ? 2 : 0,
        },
      }),
    );

    const detail = await fetchTaskDetail({ id: '1001', spaceId: '9001' });

    expect(detail.latestTaskRunID).toBe('2001');
    expect(detail.task?.adaptive_execution).toEqual({
      schema: 'coze.adaptive_execution_public.v1',
      enabled: true,
      mode: 'multi_step',
      safe_summary: '任务需要分步执行。',
      clarification_question: null,
    });
    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith({
      thread_id: '1001',
      run_id: '2001',
      space_id: '9001',
      page: 1,
      page_size: 100,
    });
  });

  it('paginates past a full page of subagent retry workers to find the primary Run', async () => {
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
        can_edit: true,
        created_at: 1767225600000,
        updated_at: 1767225660000,
      },
    });
    const retryWorkers = Array.from({ length: 20 }, (_, index) => ({
      run_id: String(4000 + index),
      thread_id: '1001',
      space_id: '9001',
      status: 'pending',
      metadata: JSON.stringify({
        source: 'subagent_retry',
        source_run_id: '2101',
      }),
      run_kind: 'task',
      created_at: 1767225800000 - index,
      updated_at: 1767225800000 - index,
    }));
    mockListTaskThreadRuns.mockImplementation(request => {
      if (request.parent_run_id !== '0') {
        return Promise.resolve({
          code: 0,
          msg: 'success',
          data: { runs: [], total: 0 },
        });
      }
      return Promise.resolve({
        code: 0,
        msg: 'success',
        data: {
          runs:
            request.page === 1
              ? retryWorkers
              : [
                  {
                    run_id: '2001',
                    thread_id: '1001',
                    space_id: '9001',
                    status: 'running',
                    metadata: '{}',
                    run_kind: 'task',
                    created_at: 1767225600000,
                    updated_at: 1767225660000,
                  },
                ],
          total: 21,
        },
      });
    });

    const detail = await fetchTaskDetail({ id: '1001', spaceId: '9001' });

    expect(detail.latestTaskRunID).toBe('2001');
    expect(mockListTaskThreadRuns).toHaveBeenCalledWith({
      thread_id: '1001',
      space_id: '9001',
      parent_run_id: '0',
      page: 2,
      page_size: 20,
    });
    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith({
      thread_id: '1001',
      run_id: '2001',
      space_id: '9001',
      page: 1,
      page_size: 100,
    });
  });

  it('restores historical subagent and retry failures from public Run events', async () => {
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
        can_edit: true,
        created_at: 1767225600000,
        updated_at: 1767225660000,
      },
    });
    const topNew = {
      run_id: '3001',
      thread_id: '1001',
      space_id: '9001',
      assistant_id: 'agent',
      status: 'running',
      metadata: '{}',
      run_kind: 'task',
      created_at: 30,
      updated_at: 30,
    };
    const topOld = {
      ...topNew,
      run_id: '2001',
      status: 'error',
      created_at: 20,
      updated_at: 20,
    };
    const retryRun = {
      ...topNew,
      run_id: '2501',
      status: 'error',
      metadata:
        '{"source":"subagent_retry","source_run_id":"2101","requested_at":25}',
      created_at: 25,
      updated_at: 26,
    };
    const childRun = {
      ...topOld,
      run_id: '2101',
      parent_run_id: '2001',
      run_kind: 'subagent',
      status: 'error',
    };
    mockListTaskThreadRuns.mockImplementation(request => {
      let runs: unknown[] = [];
      if (request.parent_run_id === '0') {
        runs = [topNew];
      } else if (request.parent_run_id === '2001') {
        runs = [childRun];
      } else if (!request.parent_run_id && request.page_size === 20) {
        runs = [topNew, retryRun, topOld];
      }

      return Promise.resolve({
        code: 0,
        msg: 'success',
        data: { runs, total: runs.length },
      });
    });
    mockListTaskThreadRunEvents.mockImplementation(request => {
      const events =
        request.run_id === '2001'
          ? [
              {
                event_id: 'event-child-failed',
                thread_id: '1001',
                run_id: '2001',
                event_type: 'subagent.run.failed',
                payload:
                  '{"subagent_run_id":"2101","status":"failed","error_code":"subagent_timeout"}',
                created_at: 21,
              },
            ]
          : request.run_id === '2501'
            ? [
                {
                  event_id: 'event-retry-failed',
                  thread_id: '1001',
                  run_id: '2501',
                  event_type: 'run.failed',
                  payload: '{"status":"failed","error_code":"retry_failed"}',
                  created_at: 26,
                },
              ]
            : [];

      return Promise.resolve({
        code: 0,
        msg: 'success',
        data: { events, total: events.length },
      });
    });

    const detail = await fetchTaskDetail({ id: '1001', spaceId: '9001' });

    expect(detail.subagentRuns?.[0]).toMatchObject({
      errorCode: 'subagent_timeout',
      runId: '2101',
      statusText: '超时',
      retryAttempts: [
        expect.objectContaining({
          errorCode: 'retry_failed',
          retryRunId: '2501',
        }),
      ],
    });
    expect(detail.subagentRuns?.[0]?.timeline?.[0]).toMatchObject({
      eventType: 'subagent.run.failed',
      status: 'failed',
    });
    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith(
      expect.objectContaining({ run_id: '2001', space_id: '9001' }),
    );
    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith(
      expect.objectContaining({ run_id: '2501', space_id: '9001' }),
    );
  });

  it.each([
    ['success', undefined, TaskThreadDetailStatus.Succeeded],
    ['error', undefined, TaskThreadDetailStatus.Failed],
    ['interrupted', 'canceled', TaskThreadDetailStatus.Canceled],
  ])(
    'maps canonical Run status %s to the existing detail status',
    async (status, terminalReason, expectedStatus) => {
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
          can_edit: true,
          created_at: 1767225600000,
          updated_at: 1767225660000,
        },
      });
      mockListTaskThreadRuns.mockImplementation(request => {
        const parentRunID = request.parent_run_id;

        return Promise.resolve({
          code: 0,
          msg: 'success',
          data: {
            runs:
              parentRunID === '0'
                ? [
                    {
                      run_id: '2001',
                      thread_id: '1001',
                      space_id: '9001',
                      assistant_id: 'agent',
                      status,
                      metadata: '{}',
                      multitask_strategy: 'reject',
                      attempt_kind: 'turn',
                      run_kind: 'task',
                      stream_modes: ['events'],
                      on_disconnect: 'continue',
                      durability: 'async',
                      terminal_reason: terminalReason,
                      created_at: 1767225600000,
                      updated_at: 1767225660000,
                    },
                  ]
                : [],
            total: parentRunID === '0' ? 1 : 0,
          },
        });
      });

      const detail = await fetchTaskDetail({ id: '1001', spaceId: '9001' });

      expect(detail.task?.status).toBe(expectedStatus);
    },
  );
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
