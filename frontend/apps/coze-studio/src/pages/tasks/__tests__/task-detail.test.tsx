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
const mockUseUserInfo = vi.hoisted(() =>
  vi.fn(() => ({ user_id_str: 'user-1' })),
);
const mockGetTask = vi.hoisted(() => vi.fn());
const mockGetTaskThread = vi.hoisted(() => vi.fn());
const mockListTaskThreadMessages = vi.hoisted(() => vi.fn());
const mockListTaskThreadRuns = vi.hoisted(() => vi.fn());
const mockListTaskThreadRunEvents = vi.hoisted(() => vi.fn());
const mockGetTaskThreadTokenUsage = vi.hoisted(() => vi.fn());
const mockGetWorkbenchRuntimeDoctor = vi.hoisted(() => vi.fn());
const mockListTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockListTaskThreadGuardrailAuditEvents = vi.hoisted(() => vi.fn());
const mockExportTaskThreadGuardrailAuditEvents = vi.hoisted(() => vi.fn());
const mockListTaskThreadArtifacts = vi.hoisted(() => vi.fn());
const mockListTaskThreadArtifactScanJobs = vi.hoisted(() => vi.fn());
const mockRetryTaskThreadArtifactScanJob = vi.hoisted(() => vi.fn());
const mockReviewTaskThreadArtifactScan = vi.hoisted(() => vi.fn());
const mockUpdateTaskThreadMemory = vi.hoisted(() => vi.fn());
const mockDeleteTaskThreadMemory = vi.hoisted(() => vi.fn());
const mockClearTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockRestoreTaskThreadMemory = vi.hoisted(() => vi.fn());
const mockExportTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockImportTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockFetchTaskThreadArtifactContent = vi.hoisted(() => vi.fn());
const mockGetTaskThreadArtifactSignedURL = vi.hoisted(() => vi.fn());
const mockDeleteTaskThreadArtifact = vi.hoisted(() => vi.fn());
const mockRestoreTaskThreadArtifact = vi.hoisted(() => vi.fn());
const mockGetTaskThreadRunEventsStreamURL = vi.hoisted(() =>
  vi.fn(
    ({ threadId }: { threadId: string }) =>
      `/api/workbench/task_threads/${threadId}/run_events/stream`,
  ),
);
const mockAppendTaskThreadMessage = vi.hoisted(() => vi.fn());
const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockResumeTaskThreadRun = vi.hoisted(() => vi.fn());
const mockCancelTaskThreadRun = vi.hoisted(() => vi.fn());
const mockRetryTaskThreadSubagentRun = vi.hoisted(() => vi.fn());
const mockListTaskEvents = vi.hoisted(() => vi.fn());
const mockSendWorkbenchChat = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: mockUseUserInfo,
}));

vi.mock('../service', () => ({
  getTask: mockGetTask,
  getTaskThread: mockGetTaskThread,
  getTaskThreadRunEventsStreamURL: mockGetTaskThreadRunEventsStreamURL,
  listTaskThreadMessages: mockListTaskThreadMessages,
  listTaskThreadRuns: mockListTaskThreadRuns,
  listTaskThreadRunEvents: mockListTaskThreadRunEvents,
  getTaskThreadTokenUsage: mockGetTaskThreadTokenUsage,
  getWorkbenchRuntimeDoctor: mockGetWorkbenchRuntimeDoctor,
  listTaskThreadMemories: mockListTaskThreadMemories,
  listTaskThreadGuardrailAuditEvents: mockListTaskThreadGuardrailAuditEvents,
  exportTaskThreadGuardrailAuditEvents:
    mockExportTaskThreadGuardrailAuditEvents,
  updateTaskThreadMemory: mockUpdateTaskThreadMemory,
  deleteTaskThreadMemory: mockDeleteTaskThreadMemory,
  clearTaskThreadMemories: mockClearTaskThreadMemories,
  restoreTaskThreadMemory: mockRestoreTaskThreadMemory,
  exportTaskThreadMemories: mockExportTaskThreadMemories,
  importTaskThreadMemories: mockImportTaskThreadMemories,
  listTaskThreadArtifacts: mockListTaskThreadArtifacts,
  listTaskThreadArtifactScanJobs: mockListTaskThreadArtifactScanJobs,
  retryTaskThreadArtifactScanJob: mockRetryTaskThreadArtifactScanJob,
  reviewTaskThreadArtifactScan: mockReviewTaskThreadArtifactScan,
  fetchTaskThreadArtifactContent: mockFetchTaskThreadArtifactContent,
  getTaskThreadArtifactSignedURL: mockGetTaskThreadArtifactSignedURL,
  deleteTaskThreadArtifact: mockDeleteTaskThreadArtifact,
  restoreTaskThreadArtifact: mockRestoreTaskThreadArtifact,
  appendTaskThreadMessage: mockAppendTaskThreadMessage,
  createTaskThreadRun: mockCreateTaskThreadRun,
  resumeTaskThreadRun: mockResumeTaskThreadRun,
  cancelTaskThreadRun: mockCancelTaskThreadRun,
  retryTaskThreadSubagentRun: mockRetryTaskThreadSubagentRun,
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
  Empty: ({
    description,
    title,
  }: {
    description?: ReactNode;
    title?: ReactNode;
  }) => (
    <div>
      {title}
      {description}
    </div>
  ),
  Input: ({
    'aria-label': ariaLabel,
    onChange,
    onEnterPress,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    onChange?: (value: string) => void;
    onEnterPress?: () => void;
    placeholder?: string;
    value?: string;
  }) => (
    <input
      aria-label={ariaLabel}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
      onKeyDown={event => {
        if (event.key === 'Enter') {
          onEnterPress?.();
        }
      }}
    />
  ),
  List: ({
    dataSource = [],
    emptyContent,
    loading,
    renderItem,
  }: {
    dataSource?: unknown[];
    emptyContent?: ReactNode;
    loading?: boolean;
    renderItem?: (item: unknown, index: number) => ReactNode;
  }) => (
    <div data-loading={loading} role="list">
      {dataSource.length
        ? dataSource.map((item, index) => (
            <div key={index} role="listitem">
              {renderItem?.(item, index)}
            </div>
          ))
        : emptyContent}
    </div>
  ),
  Table: ({
    columns = [],
    dataSource = [],
  }: {
    columns?: Array<{ dataIndex?: string; title?: ReactNode }>;
    dataSource?: Array<Record<string, ReactNode>>;
  }) => (
    <table>
      <thead>
        <tr>
          {columns.map((column, index) => (
            <th key={String(column.dataIndex ?? index)}>{column.title}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {dataSource.map((row, rowIndex) => (
          <tr key={String(row.key ?? rowIndex)}>
            {columns.map((column, columnIndex) => (
              <td key={String(column.dataIndex ?? columnIndex)}>
                {column.dataIndex ? row[column.dataIndex] : null}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  ),
  Popconfirm: ({
    children,
    onConfirm,
  }: {
    children?: ReactNode;
    onConfirm?: () => void | Promise<void>;
  }) => (
    <span>
      {children}
      <button type="button" onClick={() => void onConfirm?.()}>
        确认删除
      </button>
    </span>
  ),
  SideSheet: ({
    children,
    onCancel,
    title,
    visible,
  }: {
    children?: ReactNode;
    onCancel?: () => void;
    title?: ReactNode;
    visible?: boolean;
  }) =>
    visible ? (
      <section aria-label={String(title)} role="dialog">
        <button type="button" onClick={onCancel}>
          关闭
        </button>
        <div>{title}</div>
        {children}
      </section>
    ) : null,
  Spin: ({
    children,
    spinning,
  }: {
    children?: ReactNode;
    spinning?: boolean;
  }) => <div data-spinning={spinning}>{children}</div>,
  Tag: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
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
  IconCozCross: () => <span />,
  IconCozDocument: () => <span />,
  IconCozDownload: () => <span />,
  IconCozEdit: () => <span />,
  IconCozEye: () => <span />,
  IconCozImport: () => <span />,
  IconCozImage: () => <span />,
  IconCozLink: () => <span />,
  IconCozMagnifier: () => <span />,
  IconCozMicrophone: () => <span />,
  IconCozPlus: () => <span />,
  IconCozRefresh: () => <span />,
  IconCozSendFill: () => <span />,
  IconCozSetting: () => <span />,
  IconCozTrashCan: () => <span />,
  IconCozUpload: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import TaskDetailPage from '../detail';

class MockEventSource {
  static instances: MockEventSource[] = [];

  readonly url: string;

  readonly close = vi.fn();

  private listeners = new Map<string, Array<(event: MessageEvent) => void>>();

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  removeEventListener(type: string, listener: (event: MessageEvent) => void) {
    this.listeners.set(
      type,
      (this.listeners.get(type) ?? []).filter(item => item !== listener),
    );
  }

  emit(type: string, data: string) {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data } as MessageEvent);
    }
  }
}

describe('TaskDetailPage', () => {
  beforeEach(() => {
    Object.defineProperty(globalThis, 'EventSource', {
      configurable: true,
      value: MockEventSource,
    });
    MockEventSource.instances = [];
    mockUseParams.mockReturnValue({ space_id: 'space-1', task_id: 'task-1' });
    mockUseUserInfo.mockReturnValue({ user_id_str: 'user-1' });
    mockGetTask.mockReset();
    mockGetTaskThread.mockReset();
    mockGetTaskThreadRunEventsStreamURL.mockClear();
    mockListTaskThreadMessages.mockReset();
    mockListTaskThreadRuns.mockReset();
    mockListTaskThreadRunEvents.mockReset();
    mockGetTaskThreadTokenUsage.mockReset();
    mockGetWorkbenchRuntimeDoctor.mockReset();
    mockListTaskThreadMemories.mockReset();
    mockListTaskThreadGuardrailAuditEvents.mockReset();
    mockExportTaskThreadGuardrailAuditEvents.mockReset();
    mockListTaskThreadArtifacts.mockReset();
    mockListTaskThreadArtifactScanJobs.mockReset();
    mockRetryTaskThreadArtifactScanJob.mockReset();
    mockReviewTaskThreadArtifactScan.mockReset();
    mockUpdateTaskThreadMemory.mockReset();
    mockDeleteTaskThreadMemory.mockReset();
    mockClearTaskThreadMemories.mockReset();
    mockRestoreTaskThreadMemory.mockReset();
    mockExportTaskThreadMemories.mockReset();
    mockImportTaskThreadMemories.mockReset();
    mockFetchTaskThreadArtifactContent.mockReset();
    mockGetTaskThreadArtifactSignedURL.mockReset();
    mockDeleteTaskThreadArtifact.mockReset();
    mockRestoreTaskThreadArtifact.mockReset();
    mockAppendTaskThreadMessage.mockReset();
    mockCreateTaskThreadRun.mockReset();
    mockResumeTaskThreadRun.mockReset();
    mockCancelTaskThreadRun.mockReset();
    mockRetryTaskThreadSubagentRun.mockReset();
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
    mockListTaskThreadMemories.mockResolvedValue({
      data: {
        memories: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadGuardrailAuditEvents.mockResolvedValue({
      data: {
        events: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockExportTaskThreadGuardrailAuditEvents.mockResolvedValue({
      data: {
        schema: 'coze.task_thread_guardrail_audit.export.v1',
        thread_id: 'thread-1',
        exported_at: 1717000400000,
        page: 1,
        page_size: 1000,
        total: 0,
        events: [],
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
    mockResumeTaskThreadRun.mockResolvedValue({
      data: {
        run_id: 'run-resume-1',
        thread_id: 'thread-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        assistant_id: 'default',
        status: 'queued',
        command: '{}',
        input: '{"messages":[]}',
        config: '{}',
        context: '{}',
        metadata: '{}',
        stream_mode: '["messages","updates"]',
        multitask_strategy: 'enqueue',
        on_disconnect: 'continue',
        durability: 'async',
        idempotency_key: 'resume-key',
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
    mockCancelTaskThreadRun.mockResolvedValue({
      data: {
        run_id: 'run-cancel-1',
        thread_id: 'thread-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        assistant_id: 'default',
        parent_run_id: '0',
        run_kind: 'task',
        status: 'canceled',
        command: '{}',
        input: '{"messages":[]}',
        config: '{}',
        context: '{}',
        metadata: '{}',
        stream_mode: '["messages","updates"]',
        multitask_strategy: 'enqueue',
        on_disconnect: 'continue',
        durability: 'async',
        idempotency_key: 'cancel-key',
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
    mockRetryTaskThreadSubagentRun.mockResolvedValue({
      data: {
        run_id: 'run-retry-1',
        thread_id: 'thread-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        assistant_id: 'default',
        status: 'queued',
        command: '{}',
        input: '{"messages":[]}',
        config: '{}',
        context: '{}',
        metadata: '{}',
        stream_mode: '["messages","updates"]',
        multitask_strategy: 'enqueue',
        on_disconnect: 'continue',
        durability: 'async',
        idempotency_key: 'retry-key',
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
    mockGetTaskThread.mockImplementation(
      ({ thread_id }: { thread_id: string }) =>
        Promise.resolve({
          data:
            thread_id === 'thread-1'
              ? {
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
                }
              : undefined,
          code: 0,
          msg: '',
        }),
    );
    mockListTaskThreadMessages.mockResolvedValue({
      data: {
        messages: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRuns.mockResolvedValue({
      data: {
        runs: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockGetTaskThreadTokenUsage.mockResolvedValue({
      data: {
        usage: [],
        total: 0,
        aggregate: {
          input_tokens: 0,
          output_tokens: 0,
          total_tokens: 0,
          cost_micros: 0,
          call_count: 0,
          lead_agent_tokens: 0,
          subagent_tokens: 0,
          middleware_tokens: 0,
          tool_tokens: 0,
        },
      },
      code: 0,
      msg: '',
    });
    mockGetWorkbenchRuntimeDoctor.mockResolvedValue({
      data: {
        status: 'ready',
        runtime: {
          default_mode: 'eino_adk',
          eino_adk_enabled: true,
        },
        web_tools: {
          web_fetch: {
            status: 'ready',
            configured: true,
            message: 'web_fetch is available',
          },
          web_search: {
            status: 'disabled',
            configured: false,
            message: 'web_search backend is disabled',
          },
        },
        mcp_tools: {
          status: 'ready',
          total_servers: 1,
          enabled_servers: 1,
          healthy_servers: 1,
          unhealthy_servers: 0,
          unknown_servers: 0,
        },
        checks: [
          {
            name: 'runtime.eino_adk',
            category: 'runtime',
            status: 'ready',
            message: 'Eino ADK runtime is enabled',
          },
        ],
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts.mockResolvedValue({
      data: {
        artifacts: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifactScanJobs.mockResolvedValue({
      data: {
        jobs: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
    mockRetryTaskThreadArtifactScanJob.mockResolvedValue({
      data: {
        job: undefined,
        retried: true,
      },
      code: 0,
      msg: '',
    });
    mockReviewTaskThreadArtifactScan.mockResolvedValue({
      data: {
        artifact_id: 'artifact-review-1',
        decision: 'release',
        reviewed: true,
        scan_status: 'clean',
      },
      code: 0,
      msg: '',
    });
    mockFetchTaskThreadArtifactContent.mockResolvedValue({
      blob: new Blob(['artifact body'], { type: 'text/plain' }),
      contentDisposition: "inline; filename*=UTF-8''artifact.txt",
      contentType: 'text/plain',
    });
    mockDeleteTaskThreadArtifact.mockResolvedValue({
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

  it('resolves canonical chat route params through task thread detail', async () => {
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
    expect(mockGetTask).not.toHaveBeenCalled();
    expect(mockListTaskEvents).not.toHaveBeenCalled();
    expect(mockListTaskThreadMessages).toHaveBeenCalledWith({
      thread_id: 'thread-1',
      page: 1,
      page_size: 50,
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('resolves task route params through task thread detail before legacy fallback', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      task_id: 'thread-1',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(mockGetTaskThread).toHaveBeenCalledWith({ thread_id: 'thread-1' });
    expect(mockGetTask).not.toHaveBeenCalled();
    expect(mockListTaskEvents).not.toHaveBeenCalled();
    expect(mockListTaskThreadMessages).toHaveBeenCalledWith({
      thread_id: 'thread-1',
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('生成周报');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders read-only task memory controls for non-owner thread viewers', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-1',
    });
    mockUseUserInfo.mockReturnValue({ user_id_str: 'user-2' });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    const findButton = (label: string) =>
      Array.from(container.querySelectorAll('button')).find(button =>
        button.textContent?.includes(label),
      ) as HTMLButtonElement | undefined;

    expect(container.textContent).toContain('任务记忆');
    expect(container.textContent).toContain('只读');
    expect(findButton('导出记忆')?.disabled).toBe(false);
    expect(findButton('导入记忆')?.disabled).toBe(true);
    expect(findButton('清空')?.disabled).toBe(true);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders canonical thread guardrail audit records with metadata-only fields', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-guardrail-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-guardrail-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '安全审计任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请调用受控工具',
        last_agent_message: '已完成安全审计',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadGuardrailAuditEvents.mockResolvedValue({
      data: {
        events: [
          {
            action: 'confirm',
            actor_id: 'user-1',
            created_at: 1717000200000,
            event_id: 'guardrail-event-1',
            event_type: 'guardrail.decision.confirm',
            fail_mode: 'fail_closed',
            operation: 'invoke',
            provider: 'http_scanner',
            reason_code: 'network_review',
            rule_ids: '["url_review","external_policy"]',
            run_id: 'run-guardrail-1',
            source: 'adk_runtime_tool',
            space_id: 'space-1',
            target_id: 'web_fetch',
            target_type: 'network',
            thread_id: 'thread-guardrail-1',
          },
          {
            action: 'deny',
            actor_id: 'user-1',
            created_at: 1717000100000,
            event_id: 'guardrail-event-2',
            event_type: 'guardrail.decision.deny',
            fail_mode: 'fail_closed',
            operation: 'load',
            provider: 'pattern_scanner',
            reason_code: 'skill_load_review',
            rule_ids: '["skill_load_review","raw_prompt_secret"]',
            run_id: 'run-guardrail-1',
            source: 'adk_skill_backend',
            space_id: 'space-1',
            target_id: 'shell_helper',
            target_type: 'skill',
            thread_id: 'thread-guardrail-1',
          },
        ],
        total: 2,
      },
      code: 0,
      msg: '',
    });
    mockExportTaskThreadGuardrailAuditEvents
      .mockResolvedValueOnce({
        data: {
          schema: 'coze.task_thread_guardrail_audit.export.v1',
          thread_id: 'thread-guardrail-1',
          exported_at: 1717000400000,
          page: 1,
          page_size: 1000,
          total: 3,
          events: [
            {
              action: 'confirm',
              actor_id: 'user-1',
              created_at: 1717000200000,
              event_id: 'guardrail-export-event-1',
              event_type: 'guardrail.decision.confirm',
              fail_mode: 'fail_closed',
              operation: 'invoke',
              provider: 'http_scanner',
              reason_code: 'network_review',
              rule_ids: 'url_review,external_policy',
              run_id: 'run-guardrail-1',
              source: 'adk_runtime_tool',
              space_id: 'space-1',
              target_id: 'web_fetch',
              target_type: 'network',
              thread_id: 'thread-guardrail-1',
            },
            {
              action: 'deny',
              actor_id: 'user-1',
              created_at: 1717000100000,
              event_id: 'guardrail-export-event-2',
              event_type: 'guardrail.decision.deny',
              fail_mode: 'fail_closed',
              operation: 'load',
              provider: 'pattern_scanner',
              reason_code: 'skill_load_review',
              rule_ids: 'skill_load_review,raw_prompt_secret',
              run_id: 'run-guardrail-1',
              source: 'adk_skill_backend',
              space_id: 'space-1',
              target_id: 'shell_helper',
              target_type: 'skill',
              thread_id: 'thread-guardrail-1',
            },
          ],
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          schema: 'coze.task_thread_guardrail_audit.export.v1',
          thread_id: 'thread-guardrail-1',
          exported_at: 1717000400000,
          page: 2,
          page_size: 1000,
          total: 3,
          events: [
            {
              action: 'warn',
              actor_id: 'user-1',
              created_at: 1717000000000,
              event_id: 'guardrail-export-event-3',
              event_type: 'guardrail.decision.warn',
              fail_mode: 'fail_open',
              operation: 'invoke',
              provider: 'http_scanner',
              reason_code: 'warn_review',
              rule_ids: 'warn_review',
              run_id: 'run-guardrail-1',
              source: 'adk_runtime_tool',
              space_id: 'space-1',
              target_id: 'safe_tool',
              target_type: 'tool_call',
              thread_id: 'thread-guardrail-1',
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
      await Promise.resolve();
    });

    expect(mockListTaskThreadGuardrailAuditEvents).toHaveBeenCalledWith({
      thread_id: 'thread-guardrail-1',
      page: 1,
      page_size: 20,
    });
    expect(container.textContent).toContain('安全审计');
    expect(container.textContent).toContain('2 条');
    expect(container.textContent).toContain('需要确认');
    expect(container.textContent).toContain('已拒绝');
    expect(container.textContent).toContain('network_review');
    expect(container.textContent).toContain('skill_load_review');
    expect(container.textContent).toContain('network · web_fetch');
    expect(container.textContent).toContain('skill · shell_helper');
    expect(container.textContent).toContain('http_scanner');
    expect(container.textContent).toContain('url_review');
    expect(container.textContent).toContain('external_policy');
    expect(container.textContent).not.toContain('raw prompt');
    expect(container.textContent).not.toContain('secret prompt');
    expect(container.textContent).not.toContain('tool_arguments');
    expect(container.textContent).not.toContain('checkpoint');

    const previousCreateObjectURL = URL.createObjectURL;
    const previousRevokeObjectURL = URL.revokeObjectURL;
    let exportedBlob: Blob | undefined;
    URL.createObjectURL = vi.fn((blob: Blob) => {
      exportedBlob = blob;

      return 'blob:guardrail-audit-export';
    });
    URL.revokeObjectURL = vi.fn();
    const anchorClick = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined);

    try {
      const exportButton = Array.from(
        container.querySelectorAll('button'),
      ).find(button => button.textContent?.includes('导出审计'));
      expect(exportButton).toBeTruthy();

      await act(async () => {
        Simulate.click(exportButton as HTMLButtonElement);
        await Promise.resolve();
      });

      expect(mockExportTaskThreadGuardrailAuditEvents).toHaveBeenNthCalledWith(
        1,
        {
          thread_id: 'thread-guardrail-1',
          page: 1,
          page_size: 1000,
        },
      );
      expect(mockExportTaskThreadGuardrailAuditEvents).toHaveBeenNthCalledWith(
        2,
        {
          thread_id: 'thread-guardrail-1',
          page: 2,
          page_size: 1000,
        },
      );
      expect(anchorClick).toHaveBeenCalled();
      expect(exportedBlob).toBeTruthy();
      const exportedText = await exportedBlob?.text();
      const exported = JSON.parse(exportedText ?? '{}') as {
        events: Array<Record<string, unknown>>;
        page: number;
        page_size: number;
        schema: string;
        thread_id: string;
        total: number;
      };
      expect(exported.schema).toBe(
        'coze.task_thread_guardrail_audit.export.v1',
      );
      expect(exported.thread_id).toBe('thread-guardrail-1');
      expect(exported.page).toBe(1);
      expect(exported.page_size).toBe(1000);
      expect(exported.total).toBe(3);
      expect(exported.events).toHaveLength(3);
      expect(exported.events[0]).toMatchObject({
        action: 'confirm',
        event_id: 'guardrail-export-event-1',
        provider: 'http_scanner',
        reason_code: 'network_review',
        rule_ids: ['url_review', 'external_policy'],
        target_id: 'web_fetch',
        target_type: 'network',
      });
      expect(exported.events[2]).toMatchObject({
        action: 'warn',
        event_id: 'guardrail-export-event-3',
        rule_ids: ['warn_review'],
        target_id: 'safe_tool',
        target_type: 'tool_call',
      });
      const serializedExport = JSON.stringify(exported);
      expect(serializedExport).not.toContain('secret prompt');
      expect(serializedExport).not.toContain('tool_arguments');
      expect(serializedExport).not.toContain('checkpoint');
      expect(serializedExport).not.toContain('provider_raw');
      expect(serializedExport).not.toContain('/mnt/');
    } finally {
      URL.createObjectURL = previousCreateObjectURL;
      URL.revokeObjectURL = previousRevokeObjectURL;
      anchorClick.mockRestore();
    }

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
    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith({
      thread_id: 'thread-only-1',
      page: 1,
      page_size: 100,
    });
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

  it('renders canonical thread summary when legacy task id is zero string', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-zero-legacy',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-zero-legacy',
        legacy_task_id: '0',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: 'Canonical 新建任务',
        status: 'running',
        source: 'web',
        progress: 10,
        last_user_message: '',
        last_agent_message: '',
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
            message_id: 'msg-zero-1',
            thread_id: 'thread-zero-legacy',
            run_id: 'run-zero-1',
            role: 'user',
            content: '请用一句话回复 smoke OK',
            metadata: '',
            created_at: 1717000100000,
          },
        ],
        total: 1,
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
      thread_id: 'thread-zero-legacy',
    });
    expect(mockGetTask).not.toHaveBeenCalled();
    expect(mockListTaskEvents).not.toHaveBeenCalled();
    expect(mockListTaskThreadMessages).toHaveBeenCalledWith({
      thread_id: 'thread-zero-legacy',
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('Canonical 新建任务');
    expect(container.textContent).toContain('请用一句话回复 smoke OK');
    expect(container.textContent).not.toContain('Request failed');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders runtime doctor panel for canonical thread details', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-runtime-1',
      thread_id: 'thread-runtime-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-runtime-1',
        legacy_task_id: '',
        space_id: 'space-runtime-1',
        creator_id: 'user-1',
        title: '运行诊断任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '检查运行配置',
        last_agent_message: '运行配置检查完成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(mockGetWorkbenchRuntimeDoctor).toHaveBeenCalledWith({
      space_id: 'space-runtime-1',
    });
    expect(container.textContent).toContain('运行诊断');
    expect(container.textContent).toContain('Eino ADK');
    expect(container.textContent).toContain('MCP 工具');
    expect(container.textContent).toContain('Skill 检查');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders canonical thread token usage in task detail top bar', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-token-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-token-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: 'Token 统计任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请统计模型用量',
        last_agent_message: '统计完成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockGetTaskThreadTokenUsage.mockResolvedValue({
      data: {
        usage: [],
        total: 2,
        aggregate: {
          input_tokens: 1234,
          output_tokens: 567,
          total_tokens: 1801,
          cost_micros: 0,
          call_count: 2,
          lead_agent_tokens: 1500,
          subagent_tokens: 0,
          middleware_tokens: 0,
          tool_tokens: 301,
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

    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledWith({
      thread_id: 'thread-token-1',
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('Token 1,801');
    expect(container.textContent).toContain('In 1,234 / Out 567');
    expect(container.textContent).toContain('Agent 1,500');
    expect(container.textContent).toContain('Tool 301');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders canonical thread artifacts, previews safe text inline, and downloads via signed URL', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-artifacts-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-artifacts-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '产物任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请生成报告',
        last_agent_message: '报告已生成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts.mockResolvedValue({
      data: {
        artifacts: [
          {
            artifact_id: 'artifact-1',
            artifact_type: 'report',
            content_type: 'text/plain; charset=utf-8',
            created_at: 1717000300000,
            file_id: 'file-1',
            metadata: '{}',
            preview_mode: 'text',
            run_id: 'run-1',
            size_bytes: 42,
            thread_id: 'thread-artifacts-1',
            title: 'report.txt',
            updated_at: 1717000300000,
            virtual_path: '/mnt/user-data/outputs/report.txt',
          },
          {
            artifact_id: 'artifact-2',
            artifact_type: 'html',
            content_type: 'text/html; charset=utf-8',
            created_at: 1717000300000,
            file_id: 'file-2',
            metadata: '{}',
            preview_mode: 'download',
            run_id: 'run-1',
            size_bytes: 128,
            thread_id: 'thread-artifacts-1',
            title: 'page.html',
            updated_at: 1717000300000,
            virtual_path: '/mnt/user-data/outputs/page.html',
          },
          {
            artifact_id: 'artifact-4',
            artifact_type: 'image',
            content_type: 'image/png',
            created_at: 1717000300000,
            file_id: 'file-4',
            metadata: '{}',
            preview_mode: 'image',
            run_id: 'run-1',
            size_bytes: 512,
            thread_id: 'thread-artifacts-1',
            title: 'chart.png',
            updated_at: 1717000300000,
            virtual_path: '/mnt/user-data/outputs/chart.png',
          },
          {
            artifact_id: 'artifact-5',
            artifact_type: 'report',
            content_type: 'application/pdf',
            created_at: 1717000300000,
            file_id: 'file-5',
            metadata: '{}',
            preview_mode: 'pdf',
            run_id: 'run-1',
            size_bytes: 1024,
            thread_id: 'thread-artifacts-1',
            title: 'report.pdf',
            updated_at: 1717000300000,
            virtual_path: '/mnt/user-data/outputs/report.pdf',
          },
          {
            artifact_id: 'artifact-3',
            artifact_type: 'html',
            content_type: 'text/html; charset=utf-8',
            created_at: 1717000300000,
            file_id: 'file-3',
            metadata: '{}',
            preview_mode: 'text',
            run_id: 'run-1',
            size_bytes: 128,
            thread_id: 'thread-artifacts-1',
            title: 'unsafe.txt',
            updated_at: 1717000300000,
            virtual_path: '/mnt/user-data/outputs/unsafe.txt',
          },
        ],
        total: 5,
      },
      code: 0,
      msg: '',
    });
    const previewBlob = new Blob(['Hello <world>\nline two'], {
      type: 'text/plain',
    });
    mockGetTaskThreadArtifactSignedURL.mockImplementation(
      ({ artifact_id: artifactID, mode }) =>
        Promise.resolve({
          data: {
            artifact_id: artifactID,
            content_type:
              artifactID === 'artifact-2'
                ? 'text/html; charset=utf-8'
                : artifactID === 'artifact-4'
                  ? 'image/png'
                  : artifactID === 'artifact-5'
                    ? 'application/pdf'
                    : 'text/plain; charset=utf-8',
            expires_in_seconds: 300,
            preview_mode:
              mode === 'download'
                ? 'download'
                : artifactID === 'artifact-4'
                  ? 'image'
                  : artifactID === 'artifact-5'
                    ? 'pdf'
                    : 'text',
            url:
              artifactID === 'artifact-2'
                ? 'https://storage.example.test/signed/page.html?token=download'
                : artifactID === 'artifact-4'
                  ? 'https://storage.example.test/signed/chart.png?token=preview'
                  : artifactID === 'artifact-5'
                    ? 'https://storage.example.test/signed/report.pdf?token=preview'
                    : 'https://storage.example.test/signed/report.txt?token=preview',
          },
          code: 0,
          msg: 'success',
        }),
    );
    mockFetchTaskThreadArtifactContent.mockImplementation(
      ({ artifact_id, mode }) =>
        Promise.resolve({
          blob: previewBlob,
          contentDisposition:
            mode === 'download'
              ? "attachment; filename*=UTF-8''page.html"
              : "inline; filename*=UTF-8''report.txt",
          contentType:
            artifact_id === 'artifact-2'
              ? 'text/html; charset=utf-8'
              : 'text/plain; charset=utf-8',
        }),
    );
    const previousCreateObjectURL = URL.createObjectURL;
    const previousRevokeObjectURL = URL.revokeObjectURL;
    const previousOpen = window.open;
    const createObjectURL = vi.fn().mockReturnValueOnce('blob:download');
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = vi.fn();
    window.open = vi.fn();
    const anchorClick = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined);

    try {
      await act(async () => {
        root = createRoot(container);
        root.render(<TaskDetailPage />);
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(mockListTaskThreadArtifacts).toHaveBeenCalledWith({
        thread_id: 'thread-artifacts-1',
        page: 1,
        page_size: 50,
      });
      expect(container.textContent).toContain('产物 5');
      expect(
        container.querySelector('[data-testid="task-artifacts-open"]'),
      ).toBeTruthy();
      const artifactsButton = Array.from(
        container.querySelectorAll('button'),
      ).find(
        button => button.textContent?.trim() === '产物 5',
      ) as HTMLButtonElement;
      expect(artifactsButton).toBeTruthy();

      await act(async () => {
        Simulate.click(artifactsButton);
        await Promise.resolve();
      });

      expect(container.textContent).toContain('任务产物');
      expect(container.textContent).toContain('report.txt');
      expect(container.textContent).toContain('page.html');
      expect(container.textContent).toContain('chart.png');
      expect(container.textContent).toContain('report.pdf');
      expect(container.textContent).toContain('unsafe.txt');
      expect(container.textContent).toContain('text');
      expect(container.textContent).toContain('download');
      expect(
        container.querySelector(
          '[data-testid="task-artifact-item"][data-artifact-id="artifact-1"]',
        ),
      ).toBeTruthy();
      expect(
        container.querySelector('button[aria-label="预览 unsafe.txt"]'),
      ).toBeNull();
      const previewButton = container.querySelector(
        'button[aria-label="预览 report.txt"]',
      ) as HTMLButtonElement;
      const downloadUnsafeButton = container.querySelector(
        'button[aria-label="下载 unsafe.txt"]',
      ) as HTMLButtonElement;
      const downloadPageButton = container.querySelector(
        'button[aria-label="下载 page.html"]',
      ) as HTMLButtonElement;
      const imagePreviewButton = container.querySelector(
        'button[aria-label="预览 chart.png"]',
      ) as HTMLButtonElement;
      const pdfPreviewButton = container.querySelector(
        'button[aria-label="预览 report.pdf"]',
      ) as HTMLButtonElement;
      expect(downloadUnsafeButton).toBeTruthy();
      expect(downloadPageButton).toBeTruthy();
      expect(imagePreviewButton).toBeTruthy();
      expect(pdfPreviewButton).toBeTruthy();

      await act(async () => {
        Simulate.click(previewButton);
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(mockFetchTaskThreadArtifactContent).toHaveBeenCalledWith({
        artifact_id: 'artifact-1',
        mode: 'preview',
        thread_id: 'thread-artifacts-1',
      });
      expect(mockGetTaskThreadArtifactSignedURL).not.toHaveBeenCalled();
      expect(window.open).not.toHaveBeenCalled();
      expect(
        container.querySelector('[data-testid="task-artifact-inline-preview"]'),
      ).toBeTruthy();
      expect(
        container.querySelector(
          '[data-testid="task-artifact-inline-preview-text"]',
        )?.textContent,
      ).toContain('Hello <world>');
      expect(container.textContent).toContain('Hello <world>');
      expect(container.textContent).toContain('line two');

      await act(async () => {
        Simulate.click(imagePreviewButton);
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(mockGetTaskThreadArtifactSignedURL).toHaveBeenCalledWith({
        artifact_id: 'artifact-4',
        mode: 'preview',
        thread_id: 'thread-artifacts-1',
        ttl_seconds: 300,
      });
      expect(window.open).not.toHaveBeenCalled();
      expect(
        container
          .querySelector(
            'img[data-testid="task-artifact-inline-preview-image"]',
          )
          ?.getAttribute('src'),
      ).toBe('https://storage.example.test/signed/chart.png?token=preview');

      mockFetchTaskThreadArtifactContent.mockRejectedValueOnce(
        new Error('preview failed'),
      );
      await act(async () => {
        Simulate.click(previewButton);
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(container.textContent).toContain('preview failed');
      expect(
        container.querySelector(
          'img[data-testid="task-artifact-inline-preview-image"]',
        ),
      ).toBeNull();

      await act(async () => {
        Simulate.click(pdfPreviewButton);
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(mockGetTaskThreadArtifactSignedURL).toHaveBeenCalledWith({
        artifact_id: 'artifact-5',
        mode: 'preview',
        thread_id: 'thread-artifacts-1',
        ttl_seconds: 300,
      });
      expect(window.open).toHaveBeenCalledWith(
        'https://storage.example.test/signed/report.pdf?token=preview',
        '_blank',
        'noopener,noreferrer',
      );
      expect(
        container.querySelector(
          'img[data-testid="task-artifact-inline-preview-image"]',
        ),
      ).toBeNull();

      await act(async () => {
        Simulate.click(downloadPageButton);
        await Promise.resolve();
      });

      expect(mockGetTaskThreadArtifactSignedURL).toHaveBeenCalledWith({
        artifact_id: 'artifact-2',
        mode: 'download',
        thread_id: 'thread-artifacts-1',
        ttl_seconds: 300,
      });
      expect(anchorClick).toHaveBeenCalled();
      expect(createObjectURL).not.toHaveBeenCalled();
    } finally {
      URL.createObjectURL = previousCreateObjectURL;
      URL.revokeObjectURL = previousRevokeObjectURL;
      window.open = previousOpen;
      anchorClick.mockRestore();
      act(() => {
        root?.unmount();
      });
      container.remove();
    }
  });

  it('deletes a canonical thread artifact and refreshes the drawer list', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-artifact-delete-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-artifact-delete-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '产物删除任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请生成可以删除的报告',
        last_agent_message: '报告已生成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts
      .mockResolvedValueOnce({
        data: {
          artifacts: [
            {
              artifact_id: 'artifact-delete-1',
              artifact_type: 'report',
              content_type: 'text/plain; charset=utf-8',
              created_at: 1717000300000,
              file_id: 'file-delete-1',
              metadata: '{}',
              preview_mode: 'text',
              run_id: 'run-1',
              size_bytes: 42,
              thread_id: 'thread-artifact-delete-1',
              title: 'obsolete.txt',
              updated_at: 1717000300000,
              virtual_path: '/mnt/user-data/outputs/obsolete.txt',
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [],
          total: 0,
        },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const artifactsButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '产物 1',
    ) as HTMLButtonElement;
    expect(artifactsButton).toBeTruthy();

    await act(async () => {
      Simulate.click(artifactsButton);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('obsolete.txt');
    const deleteButton = container.querySelector(
      'button[aria-label="删除 obsolete.txt"]',
    ) as HTMLButtonElement;
    expect(deleteButton).toBeTruthy();

    await act(async () => {
      Simulate.click(deleteButton);
      const confirmButton = Array.from(
        container.querySelectorAll('button'),
      ).find(
        button => button.textContent?.trim() === '确认删除',
      ) as HTMLButtonElement;
      Simulate.click(confirmButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockDeleteTaskThreadArtifact).toHaveBeenCalledWith({
      artifact_id: 'artifact-delete-1',
      thread_id: 'thread-artifact-delete-1',
    });
    expect(mockListTaskThreadArtifacts).toHaveBeenLastCalledWith({
      thread_id: 'thread-artifact-delete-1',
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('产物 0');
    expect(container.textContent).toContain('暂无任务产物');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('restores the last removed thread artifact from the drawer undo action', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    const artifact = {
      artifact_id: 'artifact-restore-1',
      artifact_type: 'report',
      content_type: 'text/plain; charset=utf-8',
      created_at: 1717000300000,
      file_id: 'file-restore-1',
      metadata: '{}',
      preview_mode: 'text',
      run_id: 'run-1',
      size_bytes: 42,
      thread_id: 'thread-artifact-restore-1',
      title: 'restore-me.txt',
      updated_at: 1717000300000,
      virtual_path: '/mnt/user-data/outputs/restore-me.txt',
    };

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-artifact-restore-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-artifact-restore-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '产物恢复任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请生成可以恢复的报告',
        last_agent_message: '报告已生成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts
      .mockResolvedValueOnce({
        data: {
          artifacts: [artifact],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [],
          total: 0,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [artifact],
          total: 1,
        },
        code: 0,
        msg: '',
      });
    mockRestoreTaskThreadArtifact.mockResolvedValue({
      data: {
        artifact_id: 'artifact-restore-1',
        restored: true,
      },
      code: 0,
      msg: 'success',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const artifactsButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '产物 1',
    ) as HTMLButtonElement;
    expect(artifactsButton).toBeTruthy();

    await act(async () => {
      Simulate.click(artifactsButton);
      await Promise.resolve();
    });

    const deleteButton = container.querySelector(
      'button[aria-label="删除 restore-me.txt"]',
    ) as HTMLButtonElement;
    expect(deleteButton).toBeTruthy();

    await act(async () => {
      Simulate.click(deleteButton);
      const confirmButton = Array.from(
        container.querySelectorAll('button'),
      ).find(
        button => button.textContent?.trim() === '确认删除',
      ) as HTMLButtonElement;
      Simulate.click(confirmButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('已移除 restore-me.txt');
    const restoreButton = container.querySelector(
      'button[aria-label="撤销移除 restore-me.txt"]',
    ) as HTMLButtonElement;
    expect(restoreButton).toBeTruthy();

    await act(async () => {
      Simulate.click(restoreButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockRestoreTaskThreadArtifact).toHaveBeenCalledWith({
      artifact_id: 'artifact-restore-1',
      thread_id: 'thread-artifact-restore-1',
    });
    expect(mockListTaskThreadArtifacts).toHaveBeenLastCalledWith({
      thread_id: 'thread-artifact-restore-1',
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('restore-me.txt');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('lists deleted thread artifacts and restores one from the drawer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    const deletedArtifact = {
      artifact_id: 'artifact-deleted-1',
      artifact_type: 'report',
      content_type: 'text/plain; charset=utf-8',
      created_at: 1717000300000,
      deleted_at: 1717000400000,
      file_id: 'file-deleted-1',
      metadata: '{}',
      preview_mode: 'text',
      run_id: 'run-1',
      size_bytes: 42,
      thread_id: 'thread-artifact-deleted-1',
      title: 'deleted-report.txt',
      updated_at: 1717000400000,
      virtual_path: '/mnt/user-data/outputs/deleted-report.txt',
    };

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-artifact-deleted-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-artifact-deleted-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '已移除产物任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请生成报告',
        last_agent_message: '报告已生成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts
      .mockResolvedValueOnce({
        data: {
          artifacts: [],
          total: 0,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [deletedArtifact],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [],
          total: 0,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [],
          total: 0,
        },
        code: 0,
        msg: '',
      });
    mockRestoreTaskThreadArtifact.mockResolvedValue({
      data: {
        artifact_id: 'artifact-deleted-1',
        restored: true,
      },
      code: 0,
      msg: 'success',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const artifactsButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '产物 0',
    ) as HTMLButtonElement;
    expect(artifactsButton).toBeTruthy();

    await act(async () => {
      Simulate.click(artifactsButton);
      await Promise.resolve();
    });

    const deletedTab = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '已移除',
    ) as HTMLButtonElement;
    expect(deletedTab).toBeTruthy();

    await act(async () => {
      Simulate.click(deletedTab);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListTaskThreadArtifacts).toHaveBeenCalledWith({
      thread_id: 'thread-artifact-deleted-1',
      deleted_only: true,
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('deleted-report.txt');

    const restoreButton = container.querySelector(
      'button[aria-label="恢复 deleted-report.txt"]',
    ) as HTMLButtonElement;
    expect(restoreButton).toBeTruthy();

    await act(async () => {
      Simulate.click(restoreButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockRestoreTaskThreadArtifact).toHaveBeenCalledWith({
      artifact_id: 'artifact-deleted-1',
      thread_id: 'thread-artifact-deleted-1',
    });
    expect(mockListTaskThreadArtifacts).toHaveBeenLastCalledWith({
      thread_id: 'thread-artifact-deleted-1',
      deleted_only: true,
      page: 1,
      page_size: 50,
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('reviews a blocked artifact scan status and refreshes the drawer list', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-artifact-review-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-artifact-review-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '产物审核任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请生成需要审核的报告',
        last_agent_message: '报告已生成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifacts
      .mockResolvedValueOnce({
        data: {
          artifacts: [
            {
              artifact_id: 'artifact-review-1',
              artifact_type: 'report',
              content_type: 'text/plain; charset=utf-8',
              created_at: 1717000300000,
              file_id: 'file-review-1',
              metadata: '{"scan_status":"blocked"}',
              preview_mode: 'text',
              run_id: 'run-1',
              size_bytes: 42,
              thread_id: 'thread-artifact-review-1',
              title: 'blocked.txt',
              updated_at: 1717000300000,
              virtual_path: '/mnt/user-data/outputs/blocked.txt',
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          artifacts: [
            {
              artifact_id: 'artifact-review-1',
              artifact_type: 'report',
              content_type: 'text/plain; charset=utf-8',
              created_at: 1717000300000,
              file_id: 'file-review-1',
              metadata: '{"scan_status":"clean"}',
              preview_mode: 'text',
              run_id: 'run-1',
              size_bytes: 42,
              thread_id: 'thread-artifact-review-1',
              title: 'blocked.txt',
              updated_at: 1717000400000,
              virtual_path: '/mnt/user-data/outputs/blocked.txt',
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const artifactsButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '产物 1',
    ) as HTMLButtonElement;
    expect(artifactsButton).toBeTruthy();

    await act(async () => {
      Simulate.click(artifactsButton);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('blocked.txt');
    expect(container.textContent).toContain('blocked');
    const releaseButton = container.querySelector(
      'button[aria-label="放行产物 blocked.txt"]',
    ) as HTMLButtonElement;
    expect(releaseButton).toBeTruthy();

    await act(async () => {
      Simulate.click(releaseButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockReviewTaskThreadArtifactScan).toHaveBeenCalledWith({
      artifact_id: 'artifact-review-1',
      decision: 'release',
      thread_id: 'thread-artifact-review-1',
    });
    expect(mockListTaskThreadArtifacts).toHaveBeenLastCalledWith({
      thread_id: 'thread-artifact-review-1',
      page: 1,
      page_size: 50,
    });
    expect(container.textContent).toContain('clean');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders artifact scan jobs in the task artifact drawer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-scan-jobs-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-scan-jobs-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '产物扫描任务',
        status: 'running',
        source: 'agent',
        progress: 40,
        last_user_message: '请生成报告',
        last_agent_message: '',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadArtifactScanJobs
      .mockResolvedValueOnce({
        data: {
          jobs: [
            {
              artifact_id: 'artifact-private-1',
              attempt_count: 3,
              available_at: 1717000400000,
              created_at: 1717000200000,
              ended_at: 1717000350000,
              file_id: 'file-private-1',
              job_id: 'scan-job-1',
              last_error: 'scanner unavailable',
              lease_expires_at: 0,
              run_id: 'run-1',
              scanner: 'clamav',
              space_id: 'space-1',
              started_at: 1717000300000,
              status: 'failed',
              thread_id: 'thread-scan-jobs-1',
              updated_at: 1717000350000,
              user_id: 'user-1',
              worker_id: 'scan-worker-a',
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          jobs: [
            {
              artifact_id: 'artifact-private-1',
              attempt_count: 3,
              available_at: 1717000400000,
              created_at: 1717000200000,
              ended_at: 0,
              file_id: 'file-private-1',
              job_id: 'scan-job-1',
              last_error: 'manual retry requested',
              lease_expires_at: 0,
              run_id: 'run-1',
              scanner: 'clamav',
              space_id: 'space-1',
              started_at: 0,
              status: 'pending',
              thread_id: 'thread-scan-jobs-1',
              updated_at: 1717000400000,
              user_id: 'user-1',
              worker_id: '',
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const artifactsButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '产物 0',
    ) as HTMLButtonElement;
    expect(artifactsButton).toBeTruthy();

    await act(async () => {
      Simulate.click(artifactsButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListTaskThreadArtifactScanJobs).toHaveBeenCalledWith({
      thread_id: 'thread-scan-jobs-1',
      page: 1,
      page_size: 20,
    });
    expect(container.textContent).toContain('扫描队列 1');
    expect(container.textContent).toContain('failed');
    expect(container.textContent).toContain('clamav');
    expect(container.textContent).toContain('尝试 3');
    expect(container.textContent).toContain('scanner unavailable');
    expect(container.textContent).not.toContain('agent-runtime/');
    expect(container.textContent).not.toContain('/mnt/user-data');

    const retryButton = container.querySelector(
      'button[aria-label="重试扫描任务 scan-job-1"]',
    ) as HTMLButtonElement;
    expect(retryButton).toBeTruthy();

    await act(async () => {
      Simulate.click(retryButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockRetryTaskThreadArtifactScanJob).toHaveBeenCalledWith({
      job_id: 'scan-job-1',
      thread_id: 'thread-scan-jobs-1',
    });
    expect(mockListTaskThreadArtifactScanJobs).toHaveBeenLastCalledWith({
      thread_id: 'thread-scan-jobs-1',
      page: 1,
      page_size: 20,
    });
    expect(container.textContent).toContain('pending');
    expect(container.textContent).toContain('manual retry requested');
    expect(container.textContent).not.toContain('agent-runtime/');
    expect(container.textContent).not.toContain('/mnt/user-data');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders canonical thread subagent run cards grouped under parent runs', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-subagent-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-subagent-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '多智能体研究任务',
        status: 'running',
        source: 'agent',
        progress: 40,
        last_user_message: '请调研竞品并写摘要',
        last_agent_message: '',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRuns.mockImplementation(({ parent_run_id }) => {
      if (parent_run_id === 'run-parent-1') {
        return Promise.resolve({
          data: {
            runs: [
              {
                run_id: 'run-child-1',
                thread_id: 'thread-subagent-1',
                parent_run_id: 'run-parent-1',
                space_id: 'space-1',
                creator_id: 'user-1',
                assistant_id: 'singleagent:1001',
                run_kind: 'subagent',
                status: 'running',
                command: '{}',
                input: '{"messages":[]}',
                config: '{}',
                context: '{}',
                metadata: JSON.stringify({
                  subagent: { name: 'researcher' },
                }),
                stream_mode: '["messages","updates"]',
                multitask_strategy: 'enqueue',
                on_disconnect: 'continue',
                durability: 'async',
                idempotency_key: '',
                worker_id: '',
                error_code: '',
                error_message: '',
                started_at: 1717000200000,
                ended_at: 0,
                created_at: 1717000200000,
                updated_at: 1717000300000,
              },
              {
                run_id: 'run-child-2',
                thread_id: 'thread-subagent-1',
                parent_run_id: 'run-parent-1',
                space_id: 'space-1',
                creator_id: 'user-1',
                assistant_id: 'singleagent:1002',
                run_kind: 'subagent',
                status: 'failed',
                command: '{}',
                input: '{"messages":[]}',
                config: '{}',
                context: '{}',
                metadata: JSON.stringify({
                  subagent: { name: 'writer' },
                }),
                stream_mode: '["messages","updates"]',
                multitask_strategy: 'enqueue',
                on_disconnect: 'continue',
                durability: 'async',
                idempotency_key: '',
                worker_id: '',
                error_code: 'subagent_timeout',
                error_message: 'context deadline exceeded',
                started_at: 1717000200000,
                ended_at: 1717000300000,
                created_at: 1717000200000,
                updated_at: 1717000300000,
              },
            ],
            total: 2,
          },
          code: 0,
          msg: '',
        });
      }

      return Promise.resolve({
        data: {
          runs: [
            {
              run_id: 'run-parent-1',
              thread_id: 'thread-subagent-1',
              parent_run_id: '',
              space_id: 'space-1',
              creator_id: 'user-1',
              assistant_id: 'default',
              run_kind: 'task',
              status: 'running',
              command: '{}',
              input: '{"messages":[]}',
              config: '{}',
              context: '{}',
              metadata: '{}',
              stream_mode: '["messages","updates"]',
              multitask_strategy: 'enqueue',
              on_disconnect: 'continue',
              durability: 'async',
              idempotency_key: '',
              worker_id: '',
              error_code: '',
              error_message: '',
              started_at: 1717000100000,
              ended_at: 0,
              created_at: 1717000100000,
              updated_at: 1717000200000,
            },
            {
              run_id: 'run-retry-1',
              thread_id: 'thread-subagent-1',
              parent_run_id: '',
              space_id: 'space-1',
              creator_id: 'user-1',
              assistant_id: 'default',
              run_kind: 'task',
              status: 'queued',
              command: '',
              input: '',
              config: '',
              context: '',
              metadata: JSON.stringify({
                source: 'subagent_retry',
                source_run_id: 'run-child-2',
                parent_run_id: 'run-parent-1',
                requested_at: 1717000350000,
              }),
              stream_mode: '["messages","updates"]',
              multitask_strategy: 'enqueue',
              on_disconnect: 'continue',
              durability: 'async',
              idempotency_key: 'subagent-retry-1',
              worker_id: '',
              error_code: '',
              error_message: '',
              started_at: 0,
              ended_at: 0,
              created_at: 1717000350000,
              updated_at: 1717000350000,
            },
          ],
          total: 2,
        },
        code: 0,
        msg: '',
      });
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [
          {
            event_id: 'event-subagent-0',
            thread_id: 'thread-subagent-1',
            run_id: 'run-parent-1',
            event_type: 'subagent.run.started',
            payload: JSON.stringify({
              child_run_id: 'run-child-2',
              status: 'running',
              elapsed_ms: 0,
              subagent: { name: 'writer' },
            }),
            created_at: 1717000200000,
          },
          {
            event_id: 'event-subagent-1',
            thread_id: 'thread-subagent-1',
            run_id: 'run-parent-1',
            event_type: 'subagent.run.failed',
            payload: JSON.stringify({
              child_run_id: 'run-child-2',
              status: 'failed',
              elapsed_ms: 1400,
              error_code: 'subagent_timeout',
              terminal_classification: 'timeout',
              error_message: 'context deadline exceeded',
              subagent: { name: 'writer' },
            }),
            created_at: 1717000300000,
          },
        ],
        total: 2,
      },
      code: 0,
      msg: '',
    });
    mockGetTaskThreadTokenUsage.mockImplementation(({ run_id }) => {
      if (run_id === 'run-parent-1') {
        return Promise.resolve({
          data: {
            usage: [
              {
                usage_id: 'usage-child-1-a',
                thread_id: 'thread-subagent-1',
                run_id: 'run-child-1',
                space_id: 'space-1',
                source: 'subagent',
                step_id: 'model-call-1',
                step_index: 0,
                step_name: 'research',
                model_name: 'gpt-4.1',
                provider: 'openai',
                input_tokens: 120,
                output_tokens: 50,
                total_tokens: 170,
                cost_micros: 140,
                currency: 'USD',
                estimated: false,
                raw_usage: '',
                metadata: '',
                created_at: 1717000250000,
              },
              {
                usage_id: 'usage-child-1-b',
                thread_id: 'thread-subagent-1',
                run_id: 'run-child-1',
                space_id: 'space-1',
                source: 'subagent',
                step_id: 'model-call-1b',
                step_index: 1,
                step_name: 'research_verify',
                model_name: 'doubao-pro',
                provider: 'volcengine',
                input_tokens: 90,
                output_tokens: 40,
                total_tokens: 130,
                cost_micros: 110,
                currency: 'USD',
                estimated: false,
                raw_usage: '',
                metadata: '',
                created_at: 1717000260000,
              },
              {
                usage_id: 'usage-child-2-a',
                thread_id: 'thread-subagent-1',
                run_id: 'run-child-2',
                space_id: 'space-1',
                source: 'subagent',
                step_id: 'model-call-2',
                step_index: 0,
                step_name: 'write',
                model_name: 'doubao-lite',
                provider: 'volcengine',
                input_tokens: 42,
                output_tokens: 0,
                total_tokens: 42,
                cost_micros: 0,
                currency: '',
                estimated: false,
                raw_usage: '',
                metadata: '',
                created_at: 1717000290000,
              },
            ],
            total: 3,
            aggregate: {
              input_tokens: 252,
              output_tokens: 90,
              total_tokens: 342,
              cost_micros: 0,
              call_count: 3,
              lead_agent_tokens: 0,
              subagent_tokens: 342,
              middleware_tokens: 0,
              tool_tokens: 0,
            },
            run_aggregates: [
              {
                run_id: 'run-child-1',
                aggregate: {
                  input_tokens: 210,
                  output_tokens: 90,
                  total_tokens: 300,
                  cost_micros: 250,
                  call_count: 2,
                  lead_agent_tokens: 0,
                  subagent_tokens: 300,
                  middleware_tokens: 0,
                  tool_tokens: 0,
                },
              },
              {
                run_id: 'run-child-2',
                aggregate: {
                  input_tokens: 42,
                  output_tokens: 0,
                  total_tokens: 42,
                  cost_micros: 0,
                  call_count: 1,
                  lead_agent_tokens: 0,
                  subagent_tokens: 42,
                  middleware_tokens: 0,
                  tool_tokens: 0,
                },
              },
            ],
          },
          code: 0,
          msg: '',
        });
      }

      return Promise.resolve({
        data: {
          usage: [],
          total: 0,
          aggregate: {
            input_tokens: 0,
            output_tokens: 0,
            total_tokens: 0,
            cost_micros: 0,
            call_count: 0,
            lead_agent_tokens: 0,
            subagent_tokens: 0,
            middleware_tokens: 0,
            tool_tokens: 0,
          },
        },
        code: 0,
        msg: '',
      });
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListTaskThreadRuns).toHaveBeenCalledWith({
      thread_id: 'thread-subagent-1',
      page: 1,
      page_size: 20,
    });
    expect(mockListTaskThreadRuns).toHaveBeenCalledWith({
      thread_id: 'thread-subagent-1',
      parent_run_id: 'run-parent-1',
      page: 1,
      page_size: 20,
    });
    expect(mockListTaskThreadRuns).not.toHaveBeenCalledWith(
      expect.objectContaining({ parent_run_id: 'run-retry-1' }),
    );
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledWith({
      thread_id: 'thread-subagent-1',
      run_id: 'run-parent-1',
      include_child_runs: true,
      page: 1,
      page_size: 100,
    });
    expect(mockGetTaskThreadTokenUsage).not.toHaveBeenCalledWith(
      expect.objectContaining({ run_id: 'run-child-1' }),
    );
    expect(mockGetTaskThreadTokenUsage).not.toHaveBeenCalledWith(
      expect.objectContaining({ run_id: 'run-child-2' }),
    );
    expect(container.textContent).toContain('子智能体执行');
    expect(container.textContent).toContain('researcher');
    expect(container.textContent).toContain('运行中');
    expect(container.textContent).toContain('openai / gpt-4.1 +1');
    expect(container.textContent).toContain('Token 300');
    expect(container.textContent).toContain('Cost USD 0.000250');
    expect(container.textContent).toContain('2 calls');
    expect(container.textContent).toContain('writer');
    expect(container.textContent).toContain('超时');
    expect(container.textContent).toContain('volcengine / doubao-lite');
    expect(container.textContent).toContain('Token 42');
    expect(container.textContent).toContain('耗时 1.4s');
    expect(container.textContent).toContain('timeout');
    expect(container.textContent).toContain('重试 1 次');
    expect(container.textContent).toContain('最近重试：排队中');
    expect(container.textContent).toContain('context deadline exceeded');
    expect(container.textContent).toContain('执行明细');
    expect(container.textContent).toContain('子智能体已启动');
    expect(container.textContent).toContain('子智能体失败');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('retries failed canonical thread subagent runs from the subagent card', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-subagent-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-subagent-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '多智能体研究任务',
        status: 'completed',
        source: 'agent',
        progress: 100,
        last_user_message: '请调研竞品并写摘要',
        last_agent_message: '任务已完成',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRuns.mockImplementation(({ parent_run_id }) => {
      if (parent_run_id === 'run-parent-1') {
        return Promise.resolve({
          data: {
            runs: [
              {
                run_id: 'run-child-2',
                thread_id: 'thread-subagent-1',
                parent_run_id: 'run-parent-1',
                space_id: 'space-1',
                creator_id: 'user-1',
                assistant_id: 'singleagent:1002',
                run_kind: 'subagent',
                status: 'failed',
                command: '',
                input: '',
                config: '',
                context: '',
                metadata: JSON.stringify({
                  subagent: { name: 'writer' },
                }),
                stream_mode: '["messages","updates"]',
                multitask_strategy: 'enqueue',
                on_disconnect: 'continue',
                durability: 'async',
                idempotency_key: '',
                worker_id: '',
                error_code: 'subagent_timeout',
                error_message: 'context deadline exceeded',
                started_at: 1717000200000,
                ended_at: 1717000300000,
                created_at: 1717000200000,
                updated_at: 1717000300000,
              },
            ],
            total: 1,
          },
          code: 0,
          msg: '',
        });
      }

      return Promise.resolve({
        data: {
          runs: [
            {
              run_id: 'run-parent-1',
              thread_id: 'thread-subagent-1',
              parent_run_id: '',
              space_id: 'space-1',
              creator_id: 'user-1',
              assistant_id: 'default',
              run_kind: 'task',
              status: 'completed',
              command: '{}',
              input: '{"messages":[]}',
              config: '{}',
              context: '{}',
              metadata: '{}',
              stream_mode: '["messages","updates"]',
              multitask_strategy: 'enqueue',
              on_disconnect: 'continue',
              durability: 'async',
              idempotency_key: '',
              worker_id: '',
              error_code: '',
              error_message: '',
              started_at: 1717000100000,
              ended_at: 1717000300000,
              created_at: 1717000100000,
              updated_at: 1717000300000,
            },
          ],
          total: 1,
        },
        code: 0,
        msg: '',
      });
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('writer');
    expect(container.textContent).toContain('超时');
    const retryButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('重试'),
    ) as HTMLButtonElement;

    await act(async () => {
      retryButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockRetryTaskThreadSubagentRun).toHaveBeenCalledWith({
      thread_id: 'thread-subagent-1',
      run_id: 'run-child-2',
    });
    expect(mockGetTaskThread).toHaveBeenCalledTimes(2);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('streams canonical thread run events into the execution flow', async () => {
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
        status: 'running',
        source: 'agent',
        progress: 35,
        last_user_message: '请分析客户反馈',
        last_agent_message: '',
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
            content: '请分析客户反馈',
            metadata: '',
            created_at: 1717000100000,
          },
        ],
        total: 2,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [
          {
            event_id: 'event-run-1',
            thread_id: 'thread-only-1',
            run_id: 'run-1',
            event_type: 'step.started',
            payload: JSON.stringify({
              step_name: 'generate_answer',
              step_index: 0,
            }),
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

    expect(mockListTaskThreadRunEvents).toHaveBeenCalledWith({
      thread_id: 'thread-only-1',
      page: 1,
      page_size: 100,
    });
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toContain(
      '/api/workbench/task_threads/thread-only-1/run_events/stream',
    );
    expect(container.textContent).toContain('执行流程');
    expect(container.textContent).toContain('开始执行 generate_answer');

    act(() => {
      MockEventSource.instances[0].emit(
        'run.event',
        JSON.stringify({
          event_id: 'event-run-2',
          thread_id: 'thread-only-1',
          run_id: 'run-1',
          event_type: 'step.completed',
          payload: JSON.stringify({
            step_name: 'generate_answer',
            step_index: 0,
            final: true,
          }),
          created_at: 1717000300000,
        }),
      );
    });

    expect(container.textContent).toContain('完成 generate_answer');

    act(() => {
      root?.unmount();
    });
    expect(MockEventSource.instances[0].close).toHaveBeenCalled();
    container.remove();
  });

  it('cancels the latest running canonical thread run from task detail', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-cancel-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-cancel-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '可取消任务',
        status: 'running',
        source: 'agent',
        progress: 35,
        last_user_message: '请分析客户反馈',
        last_agent_message: '',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRuns.mockResolvedValue({
      data: {
        runs: [
          {
            run_id: 'run-cancel-1',
            thread_id: 'thread-cancel-1',
            parent_run_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            assistant_id: 'default',
            run_kind: 'task',
            status: 'running',
            command: '{}',
            input: '{"messages":[]}',
            config: '{}',
            context: '{}',
            metadata: '{}',
            stream_mode: '["messages","updates"]',
            multitask_strategy: 'enqueue',
            on_disconnect: 'continue',
            durability: 'async',
            idempotency_key: '',
            worker_id: '',
            error_code: '',
            error_message: '',
            started_at: 1717000100000,
            ended_at: 0,
            created_at: 1717000100000,
            updated_at: 1717000200000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [
          {
            event_id: 'event-cancel-1',
            thread_id: 'thread-cancel-1',
            run_id: 'run-cancel-1',
            event_type: 'step.started',
            payload: JSON.stringify({
              step_name: 'generate_answer',
            }),
            created_at: 1717000200000,
          },
          {
            event_id: 'event-cancel-child-1',
            thread_id: 'thread-cancel-1',
            run_id: 'run-child-later-1',
            event_type: 'step.completed',
            payload: JSON.stringify({
              step_name: 'child_work',
            }),
            created_at: 1717000250000,
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

    const cancelButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('取消任务'),
    ) as HTMLButtonElement;

    await act(async () => {
      cancelButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockCancelTaskThreadRun).toHaveBeenCalledWith({
      thread_id: 'thread-cancel-1',
      run_id: 'run-cancel-1',
    });
    expect(mockGetTaskThread).toHaveBeenCalledTimes(2);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('retries failed canonical thread runs without mutating historical runs', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-retry-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-retry-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '失败任务',
        status: 'failed',
        source: 'agent',
        progress: 100,
        last_user_message: '请分析客户反馈',
        last_agent_message: '执行失败',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRuns.mockResolvedValue({
      data: {
        runs: [
          {
            run_id: 'run-failed-1',
            thread_id: 'thread-retry-1',
            parent_run_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            assistant_id: 'default',
            run_kind: 'task',
            status: 'failed',
            command: '{}',
            input: '{"messages":[]}',
            config: '{}',
            context: '{}',
            metadata: '{}',
            stream_mode: '["messages","updates"]',
            multitask_strategy: 'enqueue',
            on_disconnect: 'continue',
            durability: 'async',
            idempotency_key: '',
            worker_id: '',
            error_code: 'executor_error',
            error_message: 'bounded failure',
            started_at: 1717000100000,
            ended_at: 1717000300000,
            created_at: 1717000100000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadMessages.mockResolvedValue({
      data: {
        messages: [
          {
            message_id: 'msg-user-1',
            thread_id: 'thread-retry-1',
            run_id: 'run-failed-1',
            role: 'user',
            content: '请分析客户反馈',
            metadata: '',
            created_at: 1717000100000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [
          {
            event_id: 'event-retry-1',
            thread_id: 'thread-retry-1',
            run_id: 'run-failed-1',
            event_type: 'run.failed',
            payload: JSON.stringify({
              error_code: 'executor_error',
              error_message: 'bounded failure',
            }),
            created_at: 1717000200000,
          },
          {
            event_id: 'event-retry-child-1',
            thread_id: 'thread-retry-1',
            run_id: 'run-child-later-2',
            event_type: 'step.failed',
            payload: JSON.stringify({
              error_message: 'child bounded failure',
            }),
            created_at: 1717000250000,
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

    const retryButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('重试任务'),
    ) as HTMLButtonElement;

    await act(async () => {
      retryButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockCreateTaskThreadRun).toHaveBeenCalledWith({
      thread_id: 'thread-retry-1',
      input: expect.any(String),
      config: expect.any(String),
      metadata: expect.any(String),
      idempotency_key: expect.any(String),
    });
    const retryRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(JSON.parse(retryRequest.input)).toMatchObject({
      messages: [
        {
          role: 'user',
          content: '请分析客户反馈',
        },
      ],
    });
    expect(JSON.parse(retryRequest.config)).toMatchObject({
      runtime: 'eino_adk',
      mode: 'Auto',
      token_usage: {
        enabled: true,
      },
    });
    expect(JSON.parse(retryRequest.metadata)).toMatchObject({
      source: 'task_retry',
      source_run_id: 'run-failed-1',
    });
    expect(mockGetTaskThread).toHaveBeenCalledTimes(2);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders clarification card and submits answer through resume API', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-human-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-human-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '需要补充信息的任务',
        status: 'interrupted',
        source: 'agent',
        progress: 45,
        last_user_message: '请分析销售数据',
        last_agent_message: '',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [
          {
            event_id: 'event-human-1',
            thread_id: 'thread-human-1',
            run_id: 'run-source-1',
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
            created_at: 1717000200000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('请选择时间范围');
    const answerInput = container.querySelector(
      'textarea[aria-label="补充信息"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(answerInput, {
        target: { value: '最近 7 天' },
      } as unknown as Event);
    });
    const submitButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('提交回答'),
    ) as HTMLButtonElement;

    await act(async () => {
      submitButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockResumeTaskThreadRun).toHaveBeenCalledWith({
      thread_id: 'thread-human-1',
      run_id: 'run-source-1',
      interrupt_id: 'interrupt-1',
      response: {
        schema: 'coze.human_interaction_response.v1',
        interaction_id: 'hi_1',
        kind: 'clarification',
        decision: 'answered',
        answer: '最近 7 天',
        source: 'task_detail',
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders confirmation card and submits rejection through resume API', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockUseParams.mockReturnValue({
      space_id: 'space-1',
      thread_id: 'thread-confirm-1',
    });
    mockGetTaskThread.mockResolvedValue({
      data: {
        thread_id: 'thread-confirm-1',
        legacy_task_id: '',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '需要确认的任务',
        status: 'interrupted',
        source: 'agent',
        progress: 60,
        last_user_message: '请清理测试记录',
        last_agent_message: '',
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskThreadRunEvents.mockResolvedValue({
      data: {
        events: [
          {
            event_id: 'event-confirm-1',
            thread_id: 'thread-confirm-1',
            run_id: 'run-source-2',
            event_type: 'run.interrupted',
            payload: JSON.stringify({
              interrupts: {
                items: [
                  {
                    id: 'interrupt-2',
                    is_root_cause: true,
                    info: {
                      schema: 'coze.human_interaction.v1',
                      interaction_id: 'hi_2',
                      kind: 'confirmation',
                      title: '确认删除记录',
                      summary: '将删除 3 条测试记录',
                      action: 'delete_records',
                      risk_level: 'high',
                      required: true,
                    },
                  },
                ],
              },
              human_interaction: {
                schema: 'coze.human_interaction.v1',
                interaction_id: 'hi_2',
                kind: 'confirmation',
                title: '确认删除记录',
                summary: '将删除 3 条测试记录',
                action: 'delete_records',
                risk_level: 'high',
                required: true,
              },
            }),
            created_at: 1717000200000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('确认删除记录');
    expect(container.textContent).toContain('将删除 3 条测试记录');
    const rejectReason = container.querySelector(
      'textarea[aria-label="拒绝原因"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(rejectReason, {
        target: { value: '先保留数据' },
      } as unknown as Event);
    });
    const rejectButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('拒绝执行'),
    ) as HTMLButtonElement;

    await act(async () => {
      rejectButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockResumeTaskThreadRun).toHaveBeenCalledWith({
      thread_id: 'thread-confirm-1',
      run_id: 'run-source-2',
      interrupt_id: 'interrupt-2',
      response: {
        schema: 'coze.human_interaction_response.v1',
        interaction_id: 'hi_2',
        kind: 'confirmation',
        decision: 'rejected',
        comment: '先保留数据',
        source: 'task_detail',
      },
    });

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
      enable_skills: [],
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
      runtime: 'eino_adk',
      mode: 'Auto',
      enable_skills: [],
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
      memory_retrieval: {
        limit: 5,
        candidate_limit: 20,
        scopes: ['thread', 'long_term'],
        min_confidence: 0.2,
      },
      web_tools: {
        enabled: false,
      },
      token_usage: {
        enabled: true,
      },
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
      runtime_settings: expect.any(String),
      enable_skills: [],
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });
    expect(
      JSON.parse(mockSendWorkbenchChat.mock.calls[0]?.[0].runtime_settings),
    ).toMatchObject({
      runtime: 'eino_adk',
      memory_retrieval: {
        limit: 5,
        candidate_limit: 20,
        scopes: ['thread', 'long_term'],
        min_confidence: 0.2,
      },
      web_tools: {
        enabled: false,
      },
      token_usage: {
        enabled: true,
      },
    });
    expect(mockGetTask).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('已补充风险项。');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
