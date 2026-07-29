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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import type { WorkbenchRun } from '../../workbench/thread-client';
import { useTaskDetailActions, useTaskDetailData } from '../task-detail-hooks';
import type { WorkbenchComposerSubmitPayload } from '../../workbench/components/types';
import { TaskThreadDetailStatus } from '../task-thread-detail-model';

const mockFetchTaskDetail = vi.hoisted(() => vi.fn());
const mockListTaskThreadArtifacts = vi.hoisted(() => vi.fn());
const mockSendFollowUpMessage = vi.hoisted(() => vi.fn());
const mockStreamProps = vi.hoisted(() => ({
  current: undefined as Record<string, unknown> | undefined,
}));

vi.mock('../task-detail-loader', async importOriginal => {
  const actual = await importOriginal<typeof import('../task-detail-loader')>();

  return {
    ...actual,
    fetchTaskDetail: mockFetchTaskDetail,
  };
});

vi.mock('../task-follow-up', () => ({
  createFollowUpIdempotencyKey: (threadId: string) =>
    `${threadId}:test-key:followup`,
  sendFollowUpMessage: mockSendFollowUpMessage,
}));

vi.mock('../task-run-event-stream', () => ({
  useTaskThreadRunEventStream: (props: Record<string, unknown>) => {
    mockStreamProps.current = props;
  },
}));

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

vi.mock('../service', () => ({
  cancelTaskThreadRun: vi.fn(),
  createTaskThreadRun: vi.fn(),
  listTaskThreadArtifacts: mockListTaskThreadArtifacts,
  resumeTaskThreadRun: vi.fn(),
  retryTaskThreadSubagentRun: vi.fn(),
}));

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type DetailData = ReturnType<typeof useTaskDetailData>;
type DetailActions = ReturnType<typeof useTaskDetailActions>;

let currentData: DetailData;
let currentActions: DetailActions;
const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const createDeferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise;
  });

  return { promise, resolve };
};

const createDetail = (id: string, spaceID = 'space-1') => {
  const scopedID = spaceID === 'space-1' ? id : `${id}-${spaceID}`;

  return (
  ({
    threadId: id,
    task: {
      id,
      space_id: spaceID,
      title: `任务 ${id}`,
      input: '{}',
      status: 3,
      progress: 100,
    },
    events: [{ id: `event-${scopedID}`, event_type: 'start', payload: '{}' }],
    messages: [
      {
        message_id: `message-${scopedID}`,
        thread_id: id,
        run_id: `run-${id}`,
        role: 'assistant',
        content: `消息 ${id}`,
        metadata: '',
        created_at: 1,
      },
    ],
    artifacts: [
      {
        artifact_id: `artifact-${scopedID}`,
        thread_id: id,
        run_id: `run-${id}`,
        name: `产物 ${id}`,
        created_at: 1,
        updated_at: 1,
      },
    ],
    subagentRuns: [],
    todos: [],
    tokenUsageByRunID: {},
  }) as never
  );
};

const createRun = (runID: string, createdAt: number): WorkbenchRun => ({
  run_id: runID,
  thread_id: 'thread-1',
  space_id: 'space-1',
  assistant_id: 'agent',
  status: 'running',
  metadata: '',
  multitask_strategy: '',
  attempt_kind: 'turn',
  run_kind: 'task',
  stream_modes: ['events'],
  on_disconnect: 'continue',
  durability: 'async',
  created_at: createdAt,
  updated_at: createdAt,
});

const submitPayload = {
  message: '并发追问',
  mode: 'pro',
  runtimeSettings: {
    runtime: 'eino_adk',
  },
} as WorkbenchComposerSubmitPayload;

const HookHarness = ({
  spaceID,
  taskDetailId,
}: {
  spaceID: string;
  taskDetailId: string;
}) => {
  currentData = useTaskDetailData({
    spaceID,
    taskDetailId,
  });
  currentActions = useTaskDetailActions({
    applyTaskDetail: currentData.applyTaskDetail,
    applyOptimisticFollowUp: currentData.applyOptimisticFollowUp,
    artifacts: currentData.artifacts,
    events: currentData.events,
    messages: currentData.messages,
    pendingHumanInteraction: undefined,
    spaceID,
    subagentRuns: currentData.subagentRuns,
    task: currentData.task,
    taskDetailId,
    todos: currentData.todos,
    tokenUsage: currentData.tokenUsage,
    tokenUsageByRunID: currentData.tokenUsageByRunID,
  });

  return null;
};

const renderHarness = (taskDetailId = 'thread-old', spaceID = 'space-1') => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => root.render(<HookHarness spaceID={spaceID} taskDetailId={taskDetailId} />));

  return {
    root,
    rerender: (nextTaskDetailId: string) =>
      act(() =>
        root.render(
          <HookHarness spaceID={spaceID} taskDetailId={nextTaskDetailId} />,
        ),
      ),
    rerenderScope: (nextTaskDetailId: string, nextSpaceID: string) =>
      act(() =>
        root.render(
          <HookHarness
            spaceID={nextSpaceID}
            taskDetailId={nextTaskDetailId}
          />,
        ),
      ),
  };
};

beforeEach(() => {
  vi.clearAllMocks();
  mockStreamProps.current = undefined;
  mockFetchTaskDetail.mockImplementation(
    ({ id, spaceId }: { id: string; spaceId?: string }) =>
      Promise.resolve(createDetail(id, spaceId)),
  );
  mockListTaskThreadArtifacts.mockResolvedValue({ data: { artifacts: [] } });
  mockSendFollowUpMessage.mockResolvedValue({});
});

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => root.unmount());
    container.remove();
  });
});

describe('task detail scoped writes', () => {
  it('keeps a newly committed Run when an older captured detail response arrives', async () => {
    mockFetchTaskDetail.mockResolvedValueOnce({
      ...createDetail('thread-1'),
      task: {
        ...createDetail('thread-1').task,
        error: 'previous failure',
        status: TaskThreadDetailStatus.Failed,
      },
      latestTaskRunCreatedAt: 10,
      latestTaskRunID: 'run-old',
    });
    renderHarness('thread-1');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const requestToken = currentData.captureTaskDetailRequestToken();
    act(() => currentData.commitTopLevelRun(createRun('run-new', 20)));
    act(() => {
      currentData.applyTaskDetail(
        {
          ...createDetail('thread-1'),
          events: [
            {
              id: 'event-stale',
              event_type: 'run.failed',
              payload: '{}',
            },
          ],
          task: {
            ...createDetail('thread-1').task,
            error: 'stale failure',
            status: TaskThreadDetailStatus.Failed,
          },
          latestTaskRunCreatedAt: 10,
          latestTaskRunID: 'run-old',
        },
        'thread-1',
        requestToken,
      );
    });

    expect(currentData.latestTaskRunID).toBe('run-new');
    expect(currentData.task?.status).toBe(TaskThreadDetailStatus.Running);
    expect(currentData.task?.error).toBe('');
    expect(currentData.events.map(event => event.id)).not.toContain(
      'event-stale',
    );
  });

  it('keeps the larger numeric Run ID when creation timestamps tie', async () => {
    mockFetchTaskDetail.mockResolvedValueOnce({
      ...createDetail('thread-1'),
      latestTaskRunCreatedAt: 20,
      latestTaskRunID: '200',
    });
    renderHarness('thread-1');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const requestToken = currentData.captureTaskDetailRequestToken();
    act(() => {
      currentData.applyTaskDetail(
        {
          ...createDetail('thread-1'),
          events: [
            {
              id: 'event-older-tie',
              event_type: 'run.failed',
              payload: '{}',
            },
          ],
          task: {
            ...createDetail('thread-1').task,
            error: 'older tied Run',
            status: TaskThreadDetailStatus.Failed,
          },
          latestTaskRunCreatedAt: 20,
          latestTaskRunID: '100',
        },
        'thread-1',
        requestToken,
      );
    });

    expect(currentData.latestTaskRunID).toBe('200');
    expect(currentData.task?.status).toBe(TaskThreadDetailStatus.Running);
    expect(currentData.events.map(event => event.id)).not.toContain(
      'event-older-tie',
    );
  });

  it('preserves concurrent SSE fields during optimistic follow-up merge', async () => {
    const sendDeferred = createDeferred<unknown>();
    const refreshDeferred = createDeferred<unknown>();
    mockSendFollowUpMessage.mockReturnValue(sendDeferred.promise);
    mockFetchTaskDetail
      .mockResolvedValueOnce(createDetail('thread-1'))
      .mockReturnValueOnce(refreshDeferred.promise);
    renderHarness('thread-1');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    let pendingSubmit!: Promise<void>;
    await act(async () => {
      pendingSubmit = currentActions.handleFollowUpSubmit(submitPayload);
      await Promise.resolve();
    });
    act(() => {
      const setEvents = mockStreamProps.current?.setEvents as
        | ((updater: (events: unknown[]) => unknown[]) => void)
        | undefined;
      setEvents?.(events => [
        ...events,
        { id: 'event-sse', event_type: 'answer.delta', payload: '{}' },
      ]);
    });
    await act(async () => {
      sendDeferred.resolve({
        kind: 'thread',
        run: {
          run_id: 'run-follow-up',
          space_id: 'space-1',
          status: 'running',
          thread_id: 'thread-1',
        },
        message: {
          message_id: 'message-follow-up',
          thread_id: 'thread-1',
          run_id: 'run-follow-up',
          role: 'user',
          content: '并发追问',
          metadata: '',
          created_at: 2,
        },
      });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(currentData.events.map(event => event.id)).toContain('event-sse');
    expect(currentData.artifacts.map(item => item.artifact_id)).toContain(
      'artifact-thread-1',
    );
    expect(currentData.messages.map(item => item.message_id)).toContain(
      'message-follow-up',
    );

    await act(async () => {
      refreshDeferred.resolve(createDetail('thread-1'));
      await pendingSubmit;
    });
  });

  it('ignores a late artifact refresh after the route task changes', async () => {
    const artifactDeferred = createDeferred<{
      data: { artifacts: Array<{ artifact_id: string }> };
    }>();
    mockListTaskThreadArtifacts.mockReturnValue(artifactDeferred.promise);
    const { rerender } = renderHarness('thread-old');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    let pendingRefresh!: Promise<void>;
    await act(async () => {
      pendingRefresh = currentData.refreshArtifacts();
      await Promise.resolve();
    });
    rerender('thread-new');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      artifactDeferred.resolve({
        data: { artifacts: [{ artifact_id: 'artifact-late-old' }] },
      });
      await pendingRefresh;
    });

    expect(currentData.artifacts.map(item => item.artifact_id)).not.toContain(
      'artifact-late-old',
    );
    expect(currentData.artifacts.map(item => item.artifact_id)).toContain(
      'artifact-thread-new',
    );
  });

  it('ignores a late artifact refresh after the workspace changes for the same thread', async () => {
    const artifactDeferred = createDeferred<{
      data: { artifacts: Array<{ artifact_id: string }> };
    }>();
    mockListTaskThreadArtifacts.mockReturnValue(artifactDeferred.promise);
    const { rerenderScope } = renderHarness('thread-shared', 'space-1');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    let pendingRefresh!: Promise<void>;
    await act(async () => {
      pendingRefresh = currentData.refreshArtifacts();
      await Promise.resolve();
    });
    rerenderScope('thread-shared', 'space-2');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    await act(async () => {
      artifactDeferred.resolve({
        data: { artifacts: [{ artifact_id: 'artifact-late-space-1' }] },
      });
      await pendingRefresh;
    });

    expect(currentData.artifacts.map(item => item.artifact_id)).not.toContain(
      'artifact-late-space-1',
    );
    expect(currentData.artifacts.map(item => item.artifact_id)).toContain(
      'artifact-thread-shared-space-2',
    );
  });

  it('ignores an empty detail snapshot captured in the previous workspace', async () => {
    const { rerenderScope } = renderHarness('thread-shared', 'space-1');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    const staleRequestToken = currentData.captureTaskDetailRequestToken();

    rerenderScope('thread-shared', 'space-2');
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => {
      currentData.applyTaskDetail(
        {
          threadId: 'thread-shared',
          task: undefined,
          events: [],
        },
        'thread-shared',
        staleRequestToken,
      );
    });

    expect(currentData.task?.space_id).toBe('space-2');
    expect(currentData.events.map(event => event.id)).toContain(
      'event-thread-shared-space-2',
    );
  });
});
