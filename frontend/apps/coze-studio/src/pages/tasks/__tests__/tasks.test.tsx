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

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { workbenchTask } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockNavigate = vi.hoisted(() => vi.fn());
const mockListTasks = vi.hoisted(() => vi.fn());
const mockListTaskThreads = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  cancelTask: vi.fn(),
  listTasks: mockListTasks,
  listTaskThreads: mockListTaskThreads,
  retryTask: vi.fn(),
}));

import { getPendingHumanInteraction } from '../task-human-interaction';
import { projectTaskExecutionEvents } from '../task-event-projection';
import TasksPage from '../index';
import {
  canCancelTask,
  filterTasks,
  formatUpdatedTime,
  getTaskEventDisplay,
  getTaskInputText,
  getTaskStatusText,
} from '../helpers';

describe('TasksPage helpers', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockNavigate.mockReset();
    mockListTasks.mockResolvedValue({
      data: {
        tasks: [
          {
            id: 'task-1',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '生成周报',
            status: workbenchTask.TaskStatus.Running,
            progress: 40,
            input: JSON.stringify({ message: '整理项目进展' }),
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
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
            legacy_task_id: 'task-legacy-1',
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
    expect(container.textContent).toContain('Aime 专属助理准备好');
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
    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/chats/task-legacy-1',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('opens canonical thread id when legacy task id is zero string', async () => {
    mockListTaskThreads.mockResolvedValueOnce({
      data: {
        threads: [
          {
            thread_id: 'thread-zero-legacy',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: 'Canonical 新建任务',
            status: 'idle',
            last_user_message: '请用一句话回复 smoke OK',
            last_agent_message: '',
            legacy_task_id: '0',
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
      '/space/space-1/chats/thread-zero-legacy',
    );

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
    expect(canCancelTask(workbenchTask.TaskStatus.Created)).toBe(true);
    expect(canCancelTask(workbenchTask.TaskStatus.Queued)).toBe(true);
    expect(canCancelTask(workbenchTask.TaskStatus.Running)).toBe(true);
  });

  it('filters tasks by title and status', () => {
    const tasks = [
      {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成周报',
        status: workbenchTask.TaskStatus.Running,
        progress: 40,
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      {
        id: 'task-2',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '排查错误',
        status: workbenchTask.TaskStatus.Failed,
        progress: 100,
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
    ];

    expect(filterTasks(tasks, '周报', 'all')).toHaveLength(1);
    expect(filterTasks(tasks, '', 'failed')).toHaveLength(1);
    expect(getTaskStatusText(workbenchTask.TaskStatus.Succeeded)).toBe(
      '已完成',
    );
  });

  it('formats JSON task input as readable text', () => {
    expect(getTaskInputText(JSON.stringify({ message: '请生成报告' }))).toBe(
      '请生成报告',
    );
    expect(getTaskInputText('普通输入')).toBe('普通输入');
  });

  it('formats agent run step events as execution steps', () => {
    const started = getTaskEventDisplay(
      'step.started',
      JSON.stringify({
        step_name: 'generate_answer',
        step_index: 0,
      }),
    );
    const completed = getTaskEventDisplay(
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
    const started = getTaskEventDisplay(
      'tool.started',
      JSON.stringify({
        step_name: 'search_web',
        tool_name: 'search_web',
        arguments_present: true,
      }),
    );
    const completed = getTaskEventDisplay(
      'tool.completed',
      JSON.stringify({
        step_name: 'search_web',
        tool_name: 'search_web',
        result_present: true,
      }),
    );
    const failed = getTaskEventDisplay(
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
    expect(completed.title).toBe('工具 search_web 调用完成');
    expect(completed.status).toBe('completed');
    expect(failed.title).toBe('工具 search_web 调用失败');
    expect(failed.status).toBe('failed');
  });

  it('hides unsafe tool event details from execution cards', () => {
    const started = getTaskEventDisplay(
      'tool.started',
      JSON.stringify({
        tool_name: 'api_key=tool-secret',
        arguments_present: true,
        tool_arguments: '{"url":"https://private.example.test/a"}',
        detail: 'tool_arguments include bearer=secret-token',
      }),
    );
    const completed = getTaskEventDisplay(
      'tool.completed',
      JSON.stringify({
        tool_name: 'safe_search',
        result_present: true,
        result: '{"object_key":"bucket/private/result.json"}',
        detail: 'provider_raw response https://private.example.test/result',
      }),
    );
    const failed = getTaskEventDisplay(
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
    expect(completed.detail).toBe('已返回结果');
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
      getTaskEventDisplay(
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
