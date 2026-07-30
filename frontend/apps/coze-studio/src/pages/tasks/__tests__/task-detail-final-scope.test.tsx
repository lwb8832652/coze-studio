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

/* eslint-disable @typescript-eslint/consistent-type-imports, @typescript-eslint/naming-convention -- Test doubles mirror lazy package types and external exports. */

import {
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  useState,
} from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({}),
}));

vi.mock('@coze-foundation/global-adapter/account-settings', () => ({
  useAccountSettings: () => ({
    node: null,
    open: vi.fn(),
  }),
}));

vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    icon,
    iconPosition: _iconPosition,
    loading: _loading,
    size: _size,
    theme: _theme,
    type: _designType,
    ...props
  }: ButtonHTMLAttributes<HTMLButtonElement> & {
    icon?: ReactNode;
    iconPosition?: string;
    loading?: boolean;
    size?: string;
    theme?: string;
    type?: string;
  }) => (
    <button type="button" {...props}>
      {icon}
      {children}
    </button>
  ),
  Input: ({
    onChange,
    prefix,
    showClear: _showClear,
    size: _size,
    ...props
  }: Omit<InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'prefix'> & {
    onChange?: (value: string) => void;
    prefix?: ReactNode;
    showClear?: boolean;
    size?: string;
  }) => (
    <label>
      {prefix}
      <input {...props} onChange={event => onChange?.(event.target.value)} />
    </label>
  ),
  Spin: () => <span>加载中</span>,
  Tabs: () => null,
  TextArea: ({
    autosize: _autosize,
    onChange,
    ...props
  }: Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
    autosize?: boolean | { minRows: number; maxRows: number };
    onChange?: (value: string) => void;
  }) => (
    <textarea {...props} onChange={event => onChange?.(event.target.value)} />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => {
  const MockIcon = () => <span aria-hidden="true" />;

  return {
    IconCozArrowBack: MockIcon,
    IconCozArrowDown: MockIcon,
    IconCozArrowRight: MockIcon,
    IconCozArrowUp: MockIcon,
    IconCozAt: MockIcon,
    IconCozCheckMark: MockIcon,
    IconCozCode: MockIcon,
    IconCozCross: MockIcon,
    IconCozDatabase: MockIcon,
    IconCozDiamondFill: MockIcon,
    IconCozKnowledge: MockIcon,
    IconCozLightbulb: MockIcon,
    IconCozLightbulbFill: MockIcon,
    IconCozLightningFill: MockIcon,
    IconCozLink: MockIcon,
    IconCozListDisorder: MockIcon,
    IconCozMagnifier: MockIcon,
    IconCozPlugin: MockIcon,
    IconCozRocketFill: MockIcon,
    IconCozSendFill: MockIcon,
    IconCozSetting: MockIcon,
    IconCozSkill: MockIcon,
    IconCozStopCircle: MockIcon,
    IconCozUpload: MockIcon,
    IconCozWorkflow: MockIcon,
  };
});

vi.mock(
  '../../workbench/components/workbench-runtime-settings-control',
  () => ({
    WorkbenchRuntimeSettingsControl: () => null,
  }),
);

vi.mock('../../skill/service', () => ({
  listSkills: vi.fn().mockResolvedValue({ data: { skills: [] } }),
}));

vi.mock('../../tools/service', () => ({
  listMCPToolRegistryEntries: vi.fn().mockResolvedValue({
    data: { entries: [] },
  }),
}));

vi.mock('../../workbench/service', () => ({
  getWorkbenchLLMModels: vi.fn().mockResolvedValue([]),
  listWorkbenchDatabaseResources: vi.fn(),
  listWorkbenchKnowledgeResources: vi.fn(),
  listWorkbenchWorkflowResources: vi.fn(),
}));

import { TaskThreadDetailStatus } from '../task-thread-detail-model';
import { TaskFollowUpComposer } from '../task-follow-up-composer';
import { fetchTaskDetail, type TaskDetail } from '../task-detail-loader';
import { useTaskDetailData } from '../task-detail-hooks';
import { listTaskThreadArtifacts } from '../service';
import type { WorkbenchRunEvent } from '../../workbench/thread-client';
import { DEFAULT_WORKBENCH_MODE } from '../../workbench/components/types';

const mockSubscribeRunEvents = vi.hoisted(() => vi.fn());

vi.mock(
  '../../workbench/thread-client/canonical-thread-client-singleton',
  () => ({
    canonicalThreadClient: {
      subscribeRunEvents: mockSubscribeRunEvents,
    },
  }),
);

vi.mock('../task-detail-loader', async importOriginal => {
  const actual = await importOriginal<typeof import('../task-detail-loader')>();

  return {
    ...actual,
    fetchTaskDetail: vi.fn(),
  };
});

vi.mock('../task-title-sync', () => ({
  useTaskThreadTitleSync: ({
    setTask,
  }: {
    setTask: (task: unknown) => void;
  }) => ({
    handleThreadTitleUpdated: vi.fn(),
    setCurrentTask: setTask,
  }),
}));

vi.mock('../service', async importOriginal => {
  const actual = await importOriginal<typeof import('../service')>();

  return {
    ...actual,
    getTaskThreadTokenUsage: vi.fn().mockResolvedValue({
      data: {
        total: 0,
        usage: [],
      },
      code: 0,
      msg: '',
    }),
    getWorkbenchLLMModels: vi.fn().mockResolvedValue([]),
    listTaskThreadArtifacts: vi.fn().mockResolvedValue({
      data: { artifacts: [] },
      code: 0,
      msg: '',
    }),
  };
});

interface ScopedRunEventStreamRequest {
  space_id: string;
  thread_id: string;
  run_id: string;
  signal: AbortSignal;
  onEvent: (event: WorkbenchRunEvent) => void;
  onEnd: () => void;
  onError: (error: Error) => void;
}

class ScopedRunEventSubscription {
  static instances: ScopedRunEventSubscription[] = [];

  readonly close = vi.fn();
  readonly closed = Promise.resolve();

  constructor(readonly request: ScopedRunEventStreamRequest) {
    ScopedRunEventSubscription.instances.push(this);
  }

  emit(event: WorkbenchRunEvent) {
    this.request.onEvent(event);
  }
}

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(nextResolve => {
    resolve = nextResolve;
  });
  return { promise, resolve };
};

const message = (id: string, threadId: string) => ({
  message_id: id,
  thread_id: threadId,
  run_id: 'run-1',
  role: 'assistant',
  content: id,
  metadata: '',
  created_at: 1,
});

const artifact = (id: string) => ({
  artifact_id: id,
  file_name: `${id}.txt`,
  content_type: 'text/plain',
  created_at: 1,
});

const detail = (
  threadId: string,
  options?: {
    artifacts?: ReturnType<typeof artifact>[];
    latestTaskRunStatus?: string;
    messages?: ReturnType<typeof message>[];
  },
) =>
  ({
    threadId,
    task: {
      id: threadId,
      space_id: 'space-1',
      status: TaskThreadDetailStatus.Succeeded,
      progress: 100,
    },
    events: [
      {
        id: `${threadId}-event-1`,
        event_type: 'message',
        created_at: 1,
      },
    ],
    messages: options?.messages ?? [message(`${threadId}-message-1`, threadId)],
    todos: [],
    artifacts: options?.artifacts ?? [artifact(`${threadId}-artifact-1`)],
    subagentRuns: [],
    latestTaskRunID: 'run-1',
    latestTaskRunCreatedAt: 1,
    latestTaskRunStatus: options?.latestTaskRunStatus ?? 'succeeded',
    tokenUsageByRunID: {},
  }) as unknown as TaskDetail;

const RouteComposerHarness = ({
  onSubmit,
  taskDetailId,
}: {
  onSubmit: ReturnType<typeof vi.fn>;
  taskDetailId: string;
}) => {
  const data = useTaskDetailData({
    spaceID: 'space-1',
    taskDetailId,
  });
  const [value, setValue] = useState('不得提交到旧任务');

  return (
    <>
      <div data-testid="loaded-task">{data.task?.id ?? 'none'}</div>
      <TaskFollowUpComposer
        value={value}
        mode={DEFAULT_WORKBENCH_MODE}
        loading={!data.loadedTaskDetailCurrent}
        taskId={taskDetailId}
        onModeChange={vi.fn()}
        onSubmit={onSubmit}
        onValueChange={setValue}
      />
    </>
  );
};

let currentData: ReturnType<typeof useTaskDetailData>;

const TaskDetailDataHarness = ({ taskDetailId }: { taskDetailId: string }) => {
  currentData = useTaskDetailData({
    spaceID: 'space-1',
    taskDetailId,
  });

  return null;
};

describe('task detail final route and revision scope', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.clearAllMocks();
    vi.useRealTimers();
    ScopedRunEventSubscription.instances = [];
    mockSubscribeRunEvents.mockReset();
    mockSubscribeRunEvents.mockImplementation(
      request => new ScopedRunEventSubscription(request),
    );
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.useRealTimers();
  });

  it('closes the old run subscription and blocks click and Enter during a route load window', async () => {
    const taskA = deferred<TaskDetail>();
    const taskB = deferred<TaskDetail>();
    vi.mocked(fetchTaskDetail).mockImplementation(({ id }) =>
      id === 'thread-a' ? taskA.promise : taskB.promise,
    );
    const onSubmit = vi.fn();
    act(() =>
      root.render(
        <RouteComposerHarness taskDetailId="thread-a" onSubmit={onSubmit} />,
      ),
    );

    await act(async () => {
      taskA.resolve(detail('thread-a'));
      await taskA.promise;
    });
    expect(
      container.querySelector('[data-testid="loaded-task"]')?.textContent,
    ).toBe('thread-a');
    expect(ScopedRunEventSubscription.instances).toHaveLength(1);

    act(() =>
      root.render(
        <RouteComposerHarness taskDetailId="thread-b" onSubmit={onSubmit} />,
      ),
    );
    expect(
      container.querySelector('[data-testid="loaded-task"]')?.textContent,
    ).toBe('none');
    expect(ScopedRunEventSubscription.instances[0].close).toHaveBeenCalled();

    const sendButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="发送任务"]',
    )!;
    const textarea = container.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="任务描述"]',
    )!;
    act(() => {
      sendButton.click();
      textarea.dispatchEvent(
        new KeyboardEvent('keydown', { bubbles: true, key: 'Enter' }),
      );
    });
    expect(onSubmit).not.toHaveBeenCalled();

    await act(async () => {
      taskB.resolve(detail('thread-b'));
      await taskB.promise;
    });
    expect(
      container.querySelector('[data-testid="loaded-task"]')?.textContent,
    ).toBe('thread-b');
  });

  it('preserves SSE, optimistic messages, and artifacts added during a stale refresh', async () => {
    vi.useFakeTimers();
    const initial = detail('thread-a', { latestTaskRunStatus: 'running' });
    const staleRefresh = deferred<TaskDetail>();
    vi.mocked(fetchTaskDetail)
      .mockResolvedValueOnce(initial)
      .mockReturnValueOnce(staleRefresh.promise);
    vi.mocked(listTaskThreadArtifacts).mockResolvedValue({
      data: {
        artifacts: [artifact('thread-a-artifact-1'), artifact('artifact-new')],
      },
      code: 0,
      msg: '',
    } as never);
    act(() => root.render(<TaskDetailDataHarness taskDetailId="thread-a" />));

    await act(async () => {
      await Promise.resolve();
    });
    await act(async () => {
      vi.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(vi.mocked(fetchTaskDetail)).toHaveBeenCalledTimes(2);

    act(() => {
      ScopedRunEventSubscription.instances[0].emit({
        event_id: 'event-new',
        thread_id: 'thread-a',
        run_id: 'run-1',
        event_type: 'run.message',
        payload: '{}',
        created_at: 2,
      });
      currentData.applyOptimisticFollowUp({
        followUpResult: {
          run: {
            run_id: 'run-new',
            thread_id: 'thread-a',
            space_id: 'space-1',
            status: 'running',
            created_at: 2,
          },
          message: message('message-new', 'thread-a'),
        } as never,
        payload: {
          message: 'message-new',
          mode: DEFAULT_WORKBENCH_MODE,
        } as never,
        threadId: 'thread-a',
      });
    });
    await act(async () => {
      await currentData.refreshArtifacts();
    });

    await act(async () => {
      staleRefresh.resolve(initial);
      await staleRefresh.promise;
    });
    expect(currentData.events.map(event => event.id)).toContain('event-new');
    expect(currentData.messages.map(item => item.message_id)).toContain(
      'message-new',
    );
    expect(currentData.artifacts.map(item => item.artifact_id)).toContain(
      'artifact-new',
    );
  });

  it('keeps an explicit artifact removal made while refresh is pending', async () => {
    vi.useFakeTimers();
    const initial = detail('thread-a', { latestTaskRunStatus: 'running' });
    const staleRefresh = deferred<TaskDetail>();
    vi.mocked(fetchTaskDetail)
      .mockResolvedValueOnce(initial)
      .mockReturnValueOnce(staleRefresh.promise);
    vi.mocked(listTaskThreadArtifacts).mockResolvedValue({
      data: { artifacts: [] },
      code: 0,
      msg: '',
    } as never);
    act(() => root.render(<TaskDetailDataHarness taskDetailId="thread-a" />));

    await act(async () => {
      await Promise.resolve();
    });
    await act(async () => {
      vi.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(vi.mocked(fetchTaskDetail)).toHaveBeenCalledTimes(2);
    await act(async () => {
      await currentData.refreshArtifacts();
    });
    expect(currentData.artifacts).toHaveLength(0);

    await act(async () => {
      staleRefresh.resolve(initial);
      await staleRefresh.promise;
    });
    expect(currentData.artifacts).toHaveLength(0);
  });

  it.each([
    {
      label: 'inverse response order',
      order: ['second', 'first'] as const,
    },
    {
      label: 'request order',
      order: ['first', 'second'] as const,
    },
  ])(
    'preserves concurrent collection updates for two stale refreshes in $label',
    async ({ order }) => {
      const initial = detail('thread-a');
      const firstRefresh = deferred<TaskDetail>();
      const secondRefresh = deferred<TaskDetail>();
      vi.mocked(fetchTaskDetail)
        .mockResolvedValueOnce(initial)
        .mockReturnValueOnce(firstRefresh.promise)
        .mockReturnValueOnce(secondRefresh.promise);
      vi.mocked(listTaskThreadArtifacts).mockResolvedValue({
        data: {
          artifacts: [
            artifact('thread-a-artifact-1'),
            artifact('artifact-concurrent'),
          ],
        },
        code: 0,
        msg: '',
      } as never);
      act(() => root.render(<TaskDetailDataHarness taskDetailId="thread-a" />));
      await act(async () => {
        await Promise.resolve();
      });

      interface RevisionAwareTaskDetailData {
        applyTaskDetail: (
          detail: TaskDetail,
          taskDetailId?: string,
          requestToken?: object,
        ) => void;
        captureTaskDetailRequestToken?: () => object;
      }
      const startRefresh = () => {
        const revisionData =
          currentData as unknown as RevisionAwareTaskDetailData;
        const requestToken = revisionData.captureTaskDetailRequestToken?.();

        return fetchTaskDetail({
          id: 'thread-a',
          spaceId: 'space-1',
        }).then(nextDetail =>
          revisionData.applyTaskDetail(nextDetail, 'thread-a', requestToken),
        );
      };
      const firstPending = startRefresh();
      const secondPending = startRefresh();

      act(() => {
        ScopedRunEventSubscription.instances[0].emit({
          event_id: 'event-concurrent',
          thread_id: 'thread-a',
          run_id: 'run-1',
          event_type: 'run.message',
          payload: '{}',
          created_at: 2,
        });
        currentData.applyOptimisticFollowUp({
          followUpResult: {
            run: {
              run_id: 'run-concurrent',
              thread_id: 'thread-a',
              space_id: 'space-1',
              status: 'running',
              created_at: 2,
            },
            message: message('message-concurrent', 'thread-a'),
          } as never,
          payload: {
            message: 'message-concurrent',
            mode: DEFAULT_WORKBENCH_MODE,
          } as never,
          threadId: 'thread-a',
        });
      });
      await act(async () => {
        await currentData.refreshArtifacts();
      });

      const requests = {
        first: { deferred: firstRefresh, pending: firstPending },
        second: { deferred: secondRefresh, pending: secondPending },
      };
      const expectConcurrentCollections = () => {
        expect(currentData.events.map(event => event.id)).toContain(
          'event-concurrent',
        );
        expect(currentData.messages.map(item => item.message_id)).toContain(
          'message-concurrent',
        );
        expect(currentData.artifacts.map(item => item.artifact_id)).toContain(
          'artifact-concurrent',
        );
      };

      for (const requestName of order) {
        const request = requests[requestName];
        await act(async () => {
          request.deferred.resolve(initial);
          await request.pending;
        });
        expectConcurrentCollections();
      }
    },
  );
});
