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

/* eslint-disable @typescript-eslint/naming-convention -- Test doubles preserve external PascalCase component exports. */

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockNavigate = vi.hoisted(() => vi.fn());
const mockListTaskThreads = vi.hoisted(() => vi.fn());

vi.hoisted(() => {
  (
    globalThis as typeof globalThis & {
      IS_BOE?: boolean;
      IS_DEV_MODE?: boolean;
      IS_OVERSEA?: boolean;
      REGION?: string;
    }
  ).IS_BOE = false;
  (globalThis as typeof globalThis & { IS_DEV_MODE?: boolean }).IS_DEV_MODE =
    false;
  (globalThis as typeof globalThis & { IS_OVERSEA?: boolean }).IS_OVERSEA =
    false;
  (globalThis as typeof globalThis & { REGION?: string }).REGION = 'cn';
});

vi.mock('lottie-web', () => ({
  destroy: vi.fn(),
  loadAnimation: vi.fn(),
  default: {
    destroy: vi.fn(),
    loadAnimation: vi.fn(),
  },
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('../../../components/workspace-page-top-bar', () => ({
  WorkspacePageTopBar: () => (
    <div data-testid="workspace-page-top-bar">
      <span>NewX AI 专属助理准备好</span>
      <button type="button">去聊天专属助理</button>
    </div>
  ),
}));

vi.mock('../service', () => ({
  listTaskThreads: mockListTaskThreads,
}));

import { TaskThreadDetailStatus } from '../task-thread-detail-model';
import { getPendingHumanInteraction } from '../task-human-interaction';
import { projectTaskExecutionEvents } from '../task-event-projection';
import TasksPage from '../index';
import {
  canCancelTask,
  filterTasks,
  formatUpdatedTime,
  getTaskThreadEventDisplay,
  getTaskInputText,
  getTaskStatusText,
} from '../helpers';

describe('TasksPage helpers', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockNavigate.mockReset();
    mockListTaskThreads.mockReset();
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-1',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '生成周报',
            status: 'running',
            last_user_message: '整理项目进展',
            last_agent_message: '',
            metadata: '{}',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
  });

  it('renders task threads as the all tasks source', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TasksPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('全部任务');
    expect(container.textContent).toContain('NewX AI 专属助理准备好');
    expect(container.textContent).toContain('去聊天专属助理');
    expect(container.textContent).toContain(
      '这里收纳您当前工作空间内的全部任务',
    );
    expect(
      container.querySelector('input[placeholder="搜索会话"]'),
    ).toBeTruthy();
    expect(container.textContent).toContain('已收藏');
    expect(container.textContent).toContain('批量操作');
    expect(container.textContent).toContain('生成周报');
    expect(container.textContent).toContain('整理项目进展');
    expect(container.textContent).toContain('运行中');
    expect(container.textContent).not.toContain('{"message":"整理项目进展"}');
    expect(mockListTaskThreads).toHaveBeenCalledWith({ space_id: 'space-1' });

    const openButton = container.querySelector(
      'button[aria-label="打开任务 生成周报"]',
    ) as HTMLButtonElement;
    act(() => {
      openButton.click();
    });
    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/tasks/thread-1');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('opens the canonical task thread id', async () => {
    mockListTaskThreads.mockResolvedValueOnce({
      data: {
        threads: [
          {
            thread_id: 'thread-canonical',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: 'Canonical 新建任务',
            status: 'idle',
            last_user_message: '请用一句话回复 smoke OK',
            last_agent_message: '',
            metadata: '{}',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TasksPage />);
      await Promise.resolve();
    });

    const openButton = container.querySelector(
      'button[aria-label="打开任务 Canonical 新建任务"]',
    ) as HTMLButtonElement;
    act(() => {
      openButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/tasks/thread-canonical',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('uses compact task display titles across all task rows and actions', async () => {
    mockListTaskThreads.mockResolvedValueOnce({
      data: {
        threads: [
          {
            thread_id: 'thread-travel',
            space_id: 'space-1',
            creator_id: 'user-1',
            title:
              '请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项，并生成一个可在产物面板预览和下载的 Markdown 或 PDF 文档。',
            status: 'completed',
            last_user_message:
              '请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项',
            last_agent_message: '',
            metadata: '{}',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TasksPage />);
      await Promise.resolve();
    });

    expect(
      container.querySelector('.coze-prototype-row-title')?.textContent,
    ).toBe('武汉3日游攻略');
    expect(
      container.querySelector('button[aria-label="打开任务 武汉3日游攻略"]'),
    ).toBeTruthy();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('patches existing task thread titles from workspace update events', async () => {
    mockListTaskThreads.mockResolvedValueOnce({
      data: {
        threads: [
          {
            thread_id: 'thread-travel',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '请帮我制定一份武汉3日游攻略，包含预算表和注意事项',
            status: 'running',
            last_user_message: '请帮我制定一份武汉3日游攻略',
            last_agent_message: '',
            metadata: '{}',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TasksPage />);
      await Promise.resolve();
    });

    act(() => {
      window.dispatchEvent(
        new CustomEvent('coze:workspace-task-thread-upsert', {
          detail: {
            mode: 'patch',
            space_id: 'space-1',
            thread: {
              thread_id: 'thread-travel',
              title: '武汉3日游攻略',
            },
          },
        }),
      );
      window.dispatchEvent(
        new CustomEvent('coze:workspace-task-thread-upsert', {
          detail: {
            mode: 'patch',
            space_id: 'space-1',
            thread: {
              thread_id: 'missing-thread',
              title: '不应插入',
            },
          },
        }),
      );
    });

    const rowTitles = Array.from(
      container.querySelectorAll('.coze-prototype-row-title'),
    ).map(item => item.textContent);

    expect(rowTitles).toEqual(['武汉3日游攻略']);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders task list loading, empty, and error states', async () => {
    const loadingContainer = document.createElement('div');
    document.body.appendChild(loadingContainer);
    let loadingRoot: Root | undefined;
    let resolveThreads: (
      value: Awaited<ReturnType<typeof mockListTaskThreads>>,
    ) => void = () => undefined;
    const pendingThreads = new Promise<
      Awaited<ReturnType<typeof mockListTaskThreads>>
    >(resolve => {
      resolveThreads = resolve;
    });
    mockListTaskThreads.mockReturnValueOnce(pendingThreads);

    act(() => {
      loadingRoot = createRoot(loadingContainer);
      loadingRoot.render(<TasksPage />);
    });

    expect(loadingContainer.textContent).toContain('加载中...');

    await act(async () => {
      resolveThreads({
        data: {
          threads: [],
          total: 0,
        },
        code: 0,
        msg: '',
      });
      await pendingThreads;
      await Promise.resolve();
    });

    expect(loadingContainer.textContent).toContain('暂无任务');

    act(() => {
      loadingRoot?.unmount();
    });
    loadingContainer.remove();

    const errorContainer = document.createElement('div');
    document.body.appendChild(errorContainer);
    let errorRoot: Root | undefined;
    mockListTaskThreads.mockRejectedValueOnce(new Error('任务列表服务异常'));

    await act(async () => {
      errorRoot = createRoot(errorContainer);
      errorRoot.render(<TasksPage />);
      await Promise.resolve();
    });

    expect(errorContainer.textContent).toContain('任务列表服务异常');

    act(() => {
      errorRoot?.unmount();
    });
    errorContainer.remove();
  });

  it('formats backend millisecond timestamps without converting from seconds', () => {
    const timestamp = 1717000300123;

    expect(formatUpdatedTime(timestamp)).toBe(
      new Date(timestamp).toLocaleString(),
    );
  });

  it('allows canceling created tasks', () => {
    expect(canCancelTask(TaskThreadDetailStatus.Created)).toBe(true);
    expect(canCancelTask(TaskThreadDetailStatus.Queued)).toBe(true);
    expect(canCancelTask(TaskThreadDetailStatus.Running)).toBe(true);
  });

  it('filters tasks by title and status', () => {
    const tasks = [
      {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成周报',
        status: TaskThreadDetailStatus.Running,
        progress: 40,
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      {
        id: 'task-2',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '排查错误',
        status: TaskThreadDetailStatus.Failed,
        progress: 100,
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
    ];

    expect(filterTasks(tasks, '周报', 'all')).toHaveLength(1);
    expect(filterTasks(tasks, '', 'failed')).toHaveLength(1);
    expect(getTaskStatusText(TaskThreadDetailStatus.Succeeded)).toBe('已完成');
  });

  it('formats JSON task input as readable text', () => {
    expect(getTaskInputText(JSON.stringify({ message: '请生成报告' }))).toBe(
      '请生成报告',
    );
    expect(getTaskInputText('普通输入')).toBe('普通输入');
  });

  it('formats agent run step events as execution steps', () => {
    const started = getTaskThreadEventDisplay(
      'step.started',
      JSON.stringify({
        step_name: 'generate_answer',
        step_index: 0,
      }),
    );
    const completed = getTaskThreadEventDisplay(
      'step.completed',
      JSON.stringify({
        step_name: 'generate_answer',
        step_index: 0,
        final: true,
      }),
    );

    expect(started.title).toBe('开始执行 generate_answer');
    expect(started.status).toBe('running');
    expect(started.structured).toBe(true);
    expect(completed.title).toBe('完成 generate_answer');
    expect(completed.status).toBe('completed');
    expect(completed.structured).toBe(true);
  });

  it('formats agent tool events as execution steps', () => {
    const started = getTaskThreadEventDisplay(
      'tool.started',
      JSON.stringify({
        step_name: 'search_web',
        tool_name: 'search_web',
        arguments_present: true,
      }),
    );
    const completed = getTaskThreadEventDisplay(
      'tool.completed',
      JSON.stringify({
        step_name: 'search_web',
        tool_name: 'search_web',
        result_present: true,
      }),
    );
    const failed = getTaskThreadEventDisplay(
      'tool.failed',
      JSON.stringify({
        step_name: 'search_web',
        tool_name: 'search_web',
        error_message: 'unsupported agent tool: search_web',
      }),
    );

    expect(started.title).toBe('调用工具 search_web');
    expect(started.status).toBe('running');
    expect(started.structured).toBe(true);
    expect(completed.title).toBe('使用 “search_web” 工具');
    expect(completed.status).toBe('completed');
    expect(failed.title).toBe('工具 search_web 调用失败');
    expect(failed.status).toBe('failed');
  });

  it('hides unsafe tool event details from execution cards', () => {
    const started = getTaskThreadEventDisplay(
      'tool.started',
      JSON.stringify({
        tool_name: 'api_key=tool-secret',
        arguments_present: true,
        tool_arguments: '{"url":"https://private.example.test/a"}',
        detail: 'tool_arguments include bearer=secret-token',
      }),
    );
    const completed = getTaskThreadEventDisplay(
      'tool.completed',
      JSON.stringify({
        tool_name: 'safe_search',
        result_present: true,
        result: '{"object_key":"bucket/private/result.json"}',
        detail: 'provider_raw response https://private.example.test/result',
      }),
    );
    const failed = getTaskThreadEventDisplay(
      'tool.failed',
      JSON.stringify({
        tool_name: 'safe_search',
        error_message: 'tool failed with access_token=secret-token',
      }),
    );

    const renderedText = [
      started.title,
      started.detail,
      completed.title,
      completed.detail,
      failed.title,
      failed.detail,
    ].join(' ');

    expect(started.title).toBe('调用工具 工具');
    expect(started.detail).toBe('参数已准备');
    expect(completed.detail).toBe('');
    expect(failed.detail).toBe('工具调用失败，详情已隐藏');
    expect(renderedText).not.toContain('tool_arguments');
    expect(renderedText).not.toContain('provider_raw');
    expect(renderedText).not.toContain('private.example.test');
    expect(renderedText).not.toContain('secret-token');
    expect(renderedText).not.toContain('api_key');
    expect(renderedText).not.toContain('object_key');
  });

  it('formats plan task events and keeps only the latest task state', () => {
    const events = [
      {
        id: 'event-1',
        task_id: 'task-1',
        event_type: 'plan.task.created',
        payload: JSON.stringify({
          plan_task_id: '1',
          subject: '运行测试',
          status: 'pending',
        }),
        created_at: 1,
      },
      {
        id: 'event-2',
        task_id: 'task-1',
        event_type: 'tool.completed',
        payload: JSON.stringify({ tool_name: 'search_web' }),
        created_at: 2,
      },
      {
        id: 'event-3',
        task_id: 'task-1',
        event_type: 'plan.task.updated',
        payload: JSON.stringify({
          plan_task_id: '1',
          subject: '运行测试',
          status: 'in_progress',
          active_form: '正在运行测试',
        }),
        created_at: 3,
      },
      {
        id: 'event-4',
        task_id: 'task-1',
        event_type: 'plan.task.completed',
        payload: JSON.stringify({
          plan_task_id: '1',
          subject: '运行测试',
          status: 'completed',
        }),
        created_at: 4,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected).toHaveLength(2);
    expect(projected[0].event.id).toBe('event-2');
    expect(projected[1].event.id).toBe('event-4');
    expect(projected[1].display.title).toBe('完成计划：运行测试');
    expect(projected[1].display.status).toBe('completed');

    expect(
      getTaskThreadEventDisplay(
        'plan.task.deleted',
        JSON.stringify({
          plan_task_id: '2',
          subject: '过期计划',
          status: 'deleted',
        }),
      ),
    ).toMatchObject({
      title: '移除计划：过期计划',
      status: 'neutral',
      structured: true,
    });
  });

  it('projects assistant tool_calls into DeerFlow-style execution steps', () => {
    const events = [
      {
        id: 'event-assistant-tool-call',
        task_id: 'task-1',
        event_type: 'message.completed',
        payload: JSON.stringify({
          role: 'assistant',
          reasoning_content: 'Let me create the document in the workspace.',
          tool_calls: [
            {
              id: 'call-write-file',
              type: 'function',
              function: {
                name: 'write_file',
                arguments: JSON.stringify({
                  description: '创建武汉3日游攻略 Markdown 文档',
                  path: '/mnt/user-data/workspace/武汉3日游攻略.md',
                }),
              },
            },
          ],
        }),
        created_at: 5,
      },
      {
        id: 'event-tool-result',
        task_id: 'task-1',
        event_type: 'tool.completed',
        payload: JSON.stringify({
          role: 'tool',
          tool_name: 'write_file',
          tool_call_id: 'call-write-file',
          content: '{"ok":true}',
        }),
        created_at: 6,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected).toHaveLength(2);
    expect(projected[0].display.kind).toBe('thought');
    expect(projected[0].display.thought).toBe(
      'Let me create the document in the workspace.',
    );
    expect(projected[1].event.id).toBe('event-assistant-tool-call:tool-call-0');
    expect(projected[1].display.title).toBe('创建武汉3日游攻略 Markdown 文档');
    expect(projected[1].display.detail).toBe(
      '/mnt/user-data/workspace/武汉3日游攻略.md',
    );
    expect(projected[1].display.status).toBe('completed');
  });

  it('projects web search tool calls with DeerFlow-style readable query labels', () => {
    const events = [
      {
        id: 'event-assistant-search-call',
        task_id: 'task-1',
        event_type: 'message.completed',
        payload: JSON.stringify({
          role: 'assistant',
          tool_calls: [
            {
              id: 'call-web-search',
              type: 'function',
              function: {
                name: 'web_search',
                arguments: JSON.stringify({
                  query: '青岛最佳旅游时间',
                  max_results: 5,
                }),
              },
            },
          ],
        }),
        created_at: 5,
      },
      {
        id: 'event-tool-result',
        task_id: 'task-1',
        event_type: 'tool.completed',
        payload: JSON.stringify({
          role: 'tool',
          tool_name: 'web_search',
          tool_call_id: 'call-web-search',
          content: '{"results":[]}',
        }),
        created_at: 6,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected).toHaveLength(1);
    expect(projected[0].display.title).toBe('搜索网页：“青岛最佳旅游时间”');
    expect(projected[0].display.status).toBe('completed');
  });

  it('projects failed run lifecycle events when no assistant turn exists', () => {
    const projected = projectTaskExecutionEvents([
      {
        id: 'event-failed-run',
        task_id: 'thread-1',
        run_id: 'run-1',
        event_type: 'run.failed',
        payload: JSON.stringify({
          error_code: 'executor_error',
          error_message: 'bounded failure',
        }),
        created_at: 1,
      },
    ]);

    expect(projected).toHaveLength(1);
    expect(projected[0].display.title).toBe('任务执行失败');
    expect(projected[0].display.detail).toBe('bounded failure');
    expect(projected[0].display.status).toBe('failed');
  });

  it('shows selected skill name for skill tool-call execution steps', () => {
    const events = [
      {
        id: 'event-assistant-skill-call',
        task_id: 'task-1',
        event_type: 'message.completed',
        payload: JSON.stringify({
          role: 'assistant',
          tool_calls: [
            {
              id: 'call-skill',
              type: 'function',
              function: {
                name: 'skill',
                arguments: JSON.stringify({
                  skill: 'skill-creator',
                }),
              },
            },
          ],
        }),
        created_at: 5,
      },
      {
        id: 'event-tool-result',
        task_id: 'task-1',
        event_type: 'tool.completed',
        payload: JSON.stringify({
          role: 'tool',
          tool_name: 'skill',
          tool_call_id: 'call-skill',
          content: 'loaded',
        }),
        created_at: 6,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected).toHaveLength(1);
    expect(projected[0].display.title).toBe('使用 “skill-creator” 技能');
    expect(projected[0].display.status).toBe('completed');
  });

  it('keeps skill tool-call failures visible with selected skill names', () => {
    const events = [
      {
        id: 'event-assistant-skill-call',
        task_id: 'task-1',
        event_type: 'message.completed',
        payload: JSON.stringify({
          role: 'assistant',
          tool_calls: [
            {
              id: 'call-skill',
              type: 'function',
              function: {
                name: 'skill',
                arguments: JSON.stringify({
                  skill: 'skill-creator',
                }),
              },
            },
          ],
        }),
        created_at: 5,
      },
      {
        id: 'event-tool-failed',
        task_id: 'task-1',
        event_type: 'tool.failed',
        payload: JSON.stringify({
          role: 'tool',
          tool_name: 'skill',
          skill_name: 'skill-creator',
          tool_call_id: 'call-skill',
          error_message: 'guardrail blocked load',
        }),
        created_at: 6,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected).toHaveLength(1);
    expect(projected[0].display.title).toBe('“skill-creator” 技能加载失败');
    expect(projected[0].display.status).toBe('failed');
    expect(projected[0].display.detail).toBe('工具调用失败，详情已隐藏');
  });

  it('hides internal transcript and memory lifecycle events from the flow', () => {
    const events = [
      {
        id: 'event-transcript',
        task_id: 'task-1',
        event_type: 'context.transcript_persisted',
        payload: JSON.stringify({
          kind: 'terminal',
          digest: 'd3fd',
          snapshot_id: 1,
          message_count: 7,
        }),
        created_at: 8,
      },
      {
        id: 'event-memory',
        task_id: 'task-1',
        event_type: 'memory.update_queued',
        payload: JSON.stringify({
          kind: 'terminal',
          digest: 'd3fd',
          snapshot_id: 1,
          message_count: 7,
        }),
        created_at: 9,
      },
    ];

    expect(projectTaskExecutionEvents(events)).toHaveLength(0);
  });

  it('falls back to loaded skill name for empty aggregate skill tool calls', () => {
    const events = [
      {
        id: 'event-skills-loaded',
        task_id: 'task-1',
        event_type: 'skills.loaded',
        payload: JSON.stringify({
          skill_count: 1,
          skill_names: ['skill-creator'],
        }),
        created_at: 4,
      },
      {
        id: 'event-assistant-skill-call',
        task_id: 'task-1',
        event_type: 'message.completed',
        payload: JSON.stringify({
          role: 'assistant',
          tool_calls: [
            {
              id: 'call-skill',
              type: 'function',
              function: {
                name: 'skill',
                arguments: JSON.stringify({}),
              },
            },
          ],
        }),
        created_at: 5,
      },
      {
        id: 'event-tool-result',
        task_id: 'task-1',
        event_type: 'tool.completed',
        payload: JSON.stringify({
          role: 'tool',
          tool_name: 'skill',
          tool_call_id: 'call-skill',
          content: 'loaded',
        }),
        created_at: 6,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected.some(item => item.display.structured)).toBe(true);
    expect(projected.at(-1)?.display.title).toBe('使用 “skill-creator” 技能');
  });

  it('renders skill catalog events as available-skill directory steps', () => {
    const events = [
      {
        id: 'event-skills-loaded',
        task_id: 'task-1',
        event_type: 'skills.loaded',
        payload: JSON.stringify({
          skill_count: 22,
          skill_ids: ['skill-1', 'skill-2', 'skill-3'],
          skill_names: ['skill-creator', 'report-writer', 'slides-maker'],
        }),
        created_at: 4,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected[0].display.title).toBe('可用技能目录 22 个');
    expect(projected[0].display.detail).toBe(
      'skill-creator、report-writer、slides-maker',
    );
    expect(projected[0].display.title).not.toContain('加载');
  });

  it('renders single skill catalog events without implying full content preload', () => {
    const events = [
      {
        id: 'event-skills-loaded',
        task_id: 'task-1',
        event_type: 'skills.loaded',
        payload: JSON.stringify({
          skill_count: 1,
          skill_ids: ['skill-1'],
          skill_names: ['skill-creator'],
        }),
        created_at: 4,
      },
      {
        id: 'event-assistant-completed',
        task_id: 'task-1',
        event_type: 'message.completed',
        payload: JSON.stringify({
          role: 'assistant',
          content: '好的，让我们开始创建技能。',
        }),
        created_at: 5,
      },
    ];

    const projected = projectTaskExecutionEvents(events);

    expect(projected[0].display.title).toBe('可用技能 “skill-creator”');
    expect(projected[0].display.status).toBe('completed');
    expect(projected[0].display.kind).toBe('step');
    expect(projected[0].display.title).not.toContain('skill_names');
    expect(projected[0].display.title).not.toContain('加载');
  });

  it('extracts latest pending clarification prompt', () => {
    const pending = getPendingHumanInteraction([
      {
        id: 'event-1',
        task_id: 'thread-1',
        event_type: 'run.interrupted',
        payload: JSON.stringify({
          interrupts: {
            items: [
              {
                id: 'interrupt-1',
                is_root_cause: true,
                info: {
                  schema: 'coze.human_interaction.v1',
                  interaction_id: 'hi_1',
                  kind: 'clarification',
                  question: '请选择时间范围',
                  allow_free_text: true,
                  required: true,
                },
              },
            ],
          },
          human_interaction: {
            schema: 'coze.human_interaction.v1',
            interaction_id: 'hi_1',
            kind: 'clarification',
            question: '请选择时间范围',
            allow_free_text: true,
            required: true,
          },
        }),
        created_at: 1,
      },
    ]);

    expect(pending?.interruptId).toBe('interrupt-1');
    expect(pending?.interactionId).toBe('hi_1');
    expect(pending?.kind).toBe('clarification');
    expect(pending?.prompt.question).toBe('请选择时间范围');
  });

  it('hides prompt after matching resolved event', () => {
    const pending = getPendingHumanInteraction([
      {
        id: 'event-1',
        task_id: 'thread-1',
        event_type: 'run.interrupted',
        payload: JSON.stringify({
          interrupts: {
            items: [
              {
                id: 'interrupt-1',
                is_root_cause: true,
                info: {
                  schema: 'coze.human_interaction.v1',
                  interaction_id: 'hi_1',
                  kind: 'clarification',
                  question: '请选择时间范围',
                  allow_free_text: true,
                  required: true,
                },
              },
            ],
          },
          human_interaction: {
            schema: 'coze.human_interaction.v1',
            interaction_id: 'hi_1',
            kind: 'clarification',
            question: '请选择时间范围',
            allow_free_text: true,
            required: true,
          },
        }),
        created_at: 1,
      },
      {
        id: 'event-2',
        task_id: 'thread-1',
        event_type: 'human.interaction.resolved',
        payload: JSON.stringify({
          schema: 'coze.human_interaction_resolved.v1',
          interrupt_id: 'interrupt-1',
          interaction_id: 'hi_1',
          kind: 'clarification',
          decision: 'answered',
        }),
        created_at: 2,
      },
    ]);

    expect(pending).toBeUndefined();
  });

  it('extracts confirmation decision metadata', () => {
    const pending = getPendingHumanInteraction([
      {
        id: 'event-1',
        task_id: 'thread-1',
        event_type: 'run.interrupted',
        payload: JSON.stringify({
          interrupts: {
            items: [
              {
                id: 'interrupt-1',
                is_root_cause: true,
                info: {
                  schema: 'coze.human_interaction.v1',
                  interaction_id: 'hi_1',
                  kind: 'confirmation',
                  title: '确认执行删除',
                  summary: '将删除 3 条记录',
                  action: 'delete_records',
                  risk_level: 'high',
                  default_decision: 'rejected',
                  rejection_guidance: '可以改为导出后人工确认',
                  consequences: ['记录将不可见'],
                  affected_resources: ['customer:1'],
                  required: true,
                },
              },
            ],
          },
          human_interactions: [
            {
              schema: 'coze.human_interaction.v1',
              interaction_id: 'hi_1',
              kind: 'confirmation',
              title: '确认执行删除',
              summary: '将删除 3 条记录',
              action: 'delete_records',
              risk_level: 'high',
              default_decision: 'rejected',
              rejection_guidance: '可以改为导出后人工确认',
              consequences: ['记录将不可见'],
              affected_resources: ['customer:1'],
              required: true,
            },
          ],
        }),
        created_at: 1,
      },
    ]);

    expect(pending?.kind).toBe('confirmation');
    expect(pending?.prompt.risk_level).toBe('high');
    expect(pending?.prompt.action).toBe('delete_records');
    expect(pending?.prompt.default_decision).toBe('rejected');
    expect(pending?.prompt.consequences).toEqual(['记录将不可见']);
  });
});
