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

import type { ReactNode } from 'react';

import { vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { workbench, workbenchTask } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() =>
  vi.fn(() => ({ space_id: 'space-1', task_id: 'task-1' })),
);
const mockNavigate = vi.hoisted(() => vi.fn());
const mockGetTask = vi.hoisted(() => vi.fn());
const mockGetTaskThread = vi.hoisted(() => vi.fn());
const mockListTaskThreadMessages = vi.hoisted(() => vi.fn());
const mockAppendTaskThreadMessage = vi.hoisted(() => vi.fn());
const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockListTaskEvents = vi.hoisted(() => vi.fn());
const mockSendWorkbenchChat = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  getTask: mockGetTask,
  getTaskThread: mockGetTaskThread,
  listTaskThreadMessages: mockListTaskThreadMessages,
  appendTaskThreadMessage: mockAppendTaskThreadMessage,
  createTaskThreadRun: mockCreateTaskThreadRun,
  listTaskEvents: mockListTaskEvents,
  sendWorkbenchChat: mockSendWorkbenchChat,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    'aria-label': ariaLabel,
    children,
    disabled,
    icon,
    loading,
    onClick,
  }: {
    'aria-label'?: string;
    children: ReactNode;
    disabled?: boolean;
    icon?: ReactNode;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      disabled={disabled}
      data-loading={loading}
      onClick={onClick}
    >
      {icon}
      {children}
    </button>
  ),
  TextArea: ({
    'aria-label': ariaLabel,
    className,
    onChange,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    className?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      className={className}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozArrowDown: () => <span />,
  IconCozAsynchronousTask: () => <span />,
  IconCozBell: () => <span />,
  IconCozImage: () => <span />,
  IconCozLink: () => <span />,
  IconCozMicrophone: () => <span />,
  IconCozPlus: () => <span />,
  IconCozSendFill: () => <span />,
  IconCozUpload: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import TaskDetailPage from '../detail';

describe('TaskDetailPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1', task_id: 'task-1' });
    mockGetTask.mockReset();
    mockGetTaskThread.mockReset();
    mockListTaskThreadMessages.mockReset();
    mockAppendTaskThreadMessage.mockReset();
    mockCreateTaskThreadRun.mockReset();
    mockListTaskEvents.mockReset();
    mockNavigate.mockReset();
    mockSendWorkbenchChat.mockReset();
    mockGetTask.mockResolvedValue({
      data: {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成周报',
        status: workbenchTask.TaskStatus.Running,
        progress: 65,
        input: JSON.stringify({
          message: '请总结本周项目进展',
          execution_type: 'Agent',
        }),
        result: JSON.stringify({
          message: '本周完成了 UI 改造方案。',
          result_type: 'agent_trace',
          execution_type: 'Agent',
        }),
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockCreateTaskThreadRun.mockResolvedValue({
      data: {
        run_id: 'run-pending-1',
        thread_id: 'thread-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        assistant_id: 'default',
        status: 'pending',
        command: '{}',
        input: '{"messages":[]}',
        config: '{}',
        context: '{}',
        metadata: '{}',
        stream_mode: '["messages","updates"]',
        multitask_strategy: 'enqueue',
        on_disconnect: 'continue',
        durability: 'async',
        idempotency_key: 'run-key',
        worker_id: '',
        error_code: '',
        error_message: '',
        started_at: 0,
        ended_at: 0,
        created_at: 1717000400000,
        updated_at: 1717000400000,
      },
      code: 0,
      msg: '',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-1',
        legacy_task_id: 'task-legacy-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成周报',
        status: 'running',
        source: 'task',
        progress: 65,
        last_user_message: '请总结本周项目进展',
        last_agent_message: '本周完成了 UI 改造方案。',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadMessages.mockResolvedValue({
      data: {
        messages: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockAppendTaskThreadMessage.mockResolvedValue({
      data: {
        message_id: 'msg-appended-1',
        thread_id: 'thread-1',
        run_id: '',
        role: 'user',
        content: '请继续追问',
        metadata: '',
        created_at: 1717000400000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskEvents.mockResolvedValue({
      data: {
        events: [
          {
            id: 'event-1',
            task_id: 'task-1',
            event_type: 'task.step',
            payload: JSON.stringify({
              title: '理解任务意图',
              detail: '解析用户输入并确定执行路径',
              status: 'completed',
              runtime: 'Agent',
            }),
            created_at: 1717000100000,
          },
          {
            id: 'event-2',
            task_id: 'task-1',
            event_type: 'task.thought',
            payload: JSON.stringify({
              title: '思考过程',
              thought: '我会先拆解目标，再组织报告结构。',
              status: 'completed',
              runtime: 'Agent',
            }),
            created_at: 1717000200000,
          },
        ],
      },
      code: 0,
      msg: '',
    });
  });

  it('renders task data and execution events', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(mockGetTask).toHaveBeenCalledWith({ task_id: 'task-1' });
    expect(mockListTaskEvents).toHaveBeenCalledWith({ task_id: 'task-1' });
    expect(container.textContent).toContain('生成周报');
    expect(container.textContent).toContain('Aime · Agent 已为你启动工作流');
    expect(container.textContent).toContain('请总结本周项目进展');
    expect(container.textContent).toContain('本周完成了 UI 改造方案。');
    expect(container.textContent).toContain('Agent 最终结果');
    expect(container.textContent).toContain('理解任务意图');
    expect(container.textContent).toContain('解析用户输入并确定执行路径');
    expect(container.textContent).toContain('我会先拆解目标，再组织报告结构。');
    expect(container.textContent).toContain('执行流程');
    expect(container.textContent).not.toContain(
      '{"message":"请总结本周项目进展"}',
    );
    expect(
      container.querySelector('textarea[aria-label="任务描述"]'),
    ).toBeTruthy();
    expect(container.textContent).toContain('65%');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('resolves canonical thread route params through legacy task detail during compatibility', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-1',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(mockGetTaskThread).toHaveBeenCalledWith({ thread_id: 'thread-1' });
    expect(mockGetTask).toHaveBeenCalledWith({ task_id: 'task-legacy-1' });
    expect(mockListTaskEvents).toHaveBeenCalledWith({
      task_id: 'task-legacy-1',
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders canonical thread summary when no legacy task exists', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-only-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-only-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '独立智能体任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '摘要里的旧用户消息',
        last_agent_message: '摘要里的旧助手消息',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadMessages.mockResolvedValue({
      data: {
        messages: [
          {
            message_id: 'msg-1',
            thread_id: 'thread-only-1',
            run_id: 'run-1',
            role: 'user',
            content: '请基于真实消息分析客户反馈',
            metadata: '',
            created_at: 1717000100000,
          },
          {
            message_id: 'msg-2',
            thread_id: 'thread-only-1',
            run_id: 'run-1',
            role: 'assistant',
            content: '真实消息显示响应速度最重要',
            metadata: '',
            created_at: 1717000200000,
          },
        ],
        total: 2,
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(mockGetTaskThread).toHaveBeenCalledWith({
      thread_id: 'thread-only-1',
    });
    expect(mockListTaskThreadMessages).toHaveBeenCalledWith({
      thread_id: 'thread-only-1',
      page: 1,
      page_size: 50,
    });
    expect(mockGetTask).not.toHaveBeenCalled();
    expect(mockListTaskEvents).not.toHaveBeenCalled();
    expect(container.textContent).toContain('独立智能体任务');
    expect(container.textContent).toContain('请基于真实消息分析客户反馈');
    expect(container.textContent).toContain('真实消息显示响应速度最重要');
    expect(container.textContent).not.toContain('摘要里的旧用户消息');
    expect(container.textContent).not.toContain('摘要里的旧助手消息');
    expect(container.textContent).not.toContain('未找到任务');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('sends canonical thread follow-up messages through the message API', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-only-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-only-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '独立智能体任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '摘要里的旧用户消息',
        last_agent_message: '摘要里的旧助手消息',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadMessages
      .mockResolvedValueOnce({
        data: {
          messages: [
            {
              message_id: 'msg-1',
              thread_id: 'thread-only-1',
              run_id: 'run-1',
              role: 'user',
              content: '请基于真实消息分析客户反馈',
              metadata: '',
              created_at: 1717000100000,
            },
            {
              message_id: 'msg-2',
              thread_id: 'thread-only-1',
              run_id: 'run-1',
              role: 'assistant',
              content: '真实消息显示响应速度最重要',
              metadata: '',
              created_at: 1717000200000,
            },
          ],
          total: 2,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          messages: [
            {
              message_id: 'msg-1',
              thread_id: 'thread-only-1',
              run_id: 'run-1',
              role: 'user',
              content: '请基于真实消息分析客户反馈',
              metadata: '',
              created_at: 1717000100000,
            },
            {
              message_id: 'msg-2',
              thread_id: 'thread-only-1',
              run_id: 'run-1',
              role: 'assistant',
              content: '真实消息显示响应速度最重要',
              metadata: '',
              created_at: 1717000200000,
            },
            {
              message_id: 'msg-3',
              thread_id: 'thread-only-1',
              run_id: '',
              role: 'user',
              content: '请追加行动建议',
              metadata: '',
              created_at: 1717000400000,
            },
          ],
          total: 3,
        },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '请追加行动建议' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockAppendTaskThreadMessage).toHaveBeenCalledWith({
      thread_id: 'thread-only-1',
      role: 'user',
      content: '请追加行动建议',
      metadata: expect.any(String),
    });
    expect(mockCreateTaskThreadRun).toHaveBeenCalledWith({
      thread_id: 'thread-only-1',
      input: expect.any(String),
      config: expect.any(String),
      metadata: expect.any(String),
      idempotency_key: expect.any(String),
    });
    expect(mockSendWorkbenchChat).not.toHaveBeenCalled();

    const appendRequest = mockAppendTaskThreadMessage.mock.calls[0]?.[0];
    expect(JSON.parse(appendRequest.metadata)).toMatchObject({
      mode: 'Auto',
      enable_skills: expect.arrayContaining(['meego-guidelines']),
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });
    const runRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(JSON.parse(runRequest.input)).toMatchObject({
      messages: [
        {
          role: 'user',
          content: '请追加行动建议',
        },
      ],
    });
    expect(JSON.parse(runRequest.config)).toMatchObject({
      mode: 'Auto',
      enable_skills: expect.arrayContaining(['meego-guidelines']),
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });
    expect(JSON.parse(runRequest.metadata)).toMatchObject({
      source: 'workbench_detail_followup',
      appended_message_id: 'msg-appended-1',
    });
    expect(runRequest.idempotency_key).toBe(
      'thread-only-1:msg-appended-1:followup',
    );
    expect(mockListTaskThreadMessages).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('请追加行动建议');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders answer results as ordinary conversation content', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockGetTask.mockResolvedValue({
      data: {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '解释 Ark 模式',
        status: workbenchTask.TaskStatus.Succeeded,
        progress: 100,
        input: JSON.stringify({
          message: 'Ark 模式是什么',
          execution_type: 'Ark',
        }),
        result: JSON.stringify({
          message: 'Ark 模式会直接使用模型生成普通回答。',
          result_type: 'answer',
          execution_type: 'Ark',
        }),
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskEvents.mockResolvedValue({
      data: {
        events: [],
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain(
      'Ark 模式会直接使用模型生成普通回答。',
    );
    expect(container.textContent).toContain('普通回答');
    expect(container.textContent).not.toContain('一、任务输入');
    expect(container.textContent).not.toContain('解释 Ark 模式报告');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders streaming answer events before the final result is persisted', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockGetTask.mockResolvedValue({
      data: {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '解释流式输出',
        status: workbenchTask.TaskStatus.Running,
        progress: 45,
        input: JSON.stringify({
          message: '为什么要流式输出',
          execution_type: 'Ark',
        }),
        result: JSON.stringify({
          message: '',
          result_type: 'answer',
          execution_type: 'Ark',
        }),
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskEvents.mockResolvedValue({
      data: {
        events: [
          {
            id: 'event-answer-1',
            task_id: 'task-1',
            event_type: 'answer.delta',
            payload: JSON.stringify({
              title: '生成回答',
              message: '任务创建后立即进入详情页，回答内容持续写入。',
              status: 'running',
              runtime: 'Ark',
            }),
            created_at: 1717000200000,
          },
        ],
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain(
      '任务创建后立即进入详情页，回答内容持续写入。',
    );
    expect(container.textContent).not.toContain('结果生成中');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders report results with the report template branch', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockGetTask.mockResolvedValue({
      data: {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成调研报告',
        status: workbenchTask.TaskStatus.Succeeded,
        progress: 100,
        input: JSON.stringify({ message: '请生成调研报告' }),
        result: JSON.stringify({
          message: '这是一份调研报告正文。',
          result_type: 'report',
          execution_type: 'Ark',
        }),
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskEvents.mockResolvedValue({
      data: {
        events: [],
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('生成调研报告报告');
    expect(container.textContent).toContain('这是一份调研报告正文。');
    expect(container.textContent).toContain('一、任务输入');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('sends follow-up messages through WorkbenchChat and refreshes in place', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockGetTask
      .mockResolvedValueOnce({
        data: {
          id: 'task-1',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '生成周报',
          status: workbenchTask.TaskStatus.Succeeded,
          progress: 100,
          input: JSON.stringify({
            message: '请总结本周项目进展',
            execution_type: 'Agent',
          }),
          result: JSON.stringify({
            message: '本周完成了 UI 改造方案。',
            result_type: 'answer',
            execution_type: 'Ark',
          }),
          created_at: 1717000000000,
          updated_at: 1717000300000,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          id: 'task-1',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '生成周报',
          status: workbenchTask.TaskStatus.Succeeded,
          progress: 100,
          input: JSON.stringify({
            message: '请总结本周项目进展',
            execution_type: 'Agent',
          }),
          result: JSON.stringify({
            message: '已补充风险项。',
            result_type: 'answer',
            execution_type: 'Ark',
          }),
          created_at: 1717000000000,
          updated_at: 1717000400000,
        },
        code: 0,
        msg: '',
      });
    mockListTaskEvents.mockResolvedValue({
      data: {
        events: [],
      },
      code: 0,
      msg: '',
    });
    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        task: {
          id: 'task-1',
        },
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '请补充风险项' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockSendWorkbenchChat).toHaveBeenCalledWith({
      space_id: 'space-1',
      task_id: 'task-1',
      message: '请补充风险项',
      mode: workbench.ChatMode.Auto,
      enable_skills: expect.arrayContaining(['meego-guidelines']),
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });
    expect(mockGetTask).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('已补充风险项。');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
