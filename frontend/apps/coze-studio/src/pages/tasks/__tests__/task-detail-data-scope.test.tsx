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

import { useTaskDetailActions, useTaskDetailData } from '../task-detail-hooks';
import type { WorkbenchComposerSubmitPayload } from '../../workbench/components/types';

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

const createDetail = (id: string) =>
  ({
    source: 'thread',
    threadId: id,
    task: {
      id,
      title: `任务 ${id}`,
      input: '{}',
      status: 3,
      progress: 100,
    },
    events: [{ id: `event-${id}`, event_type: 'start', payload: '{}' }],
    messages: [
      {
        message_id: `message-${id}`,
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
        artifact_id: `artifact-${id}`,
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
  }) as never;

const submitPayload = {
  message: '并发追问',
  mode: 'pro',
  runtimeSettings: {
    runtime: 'eino_adk',
  },
} as WorkbenchComposerSubmitPayload;

const HookHarness = ({ taskDetailId }: { taskDetailId: string }) => {
  currentData = useTaskDetailData({
    spaceID: 'space-1',
    taskDetailId,
    taskDetailSource: 'thread',
  });
  currentActions = useTaskDetailActions({
    applyTaskDetail: currentData.applyTaskDetail,
    applyOptimisticFollowUp: currentData.applyOptimisticFollowUp,
    artifacts: currentData.artifacts,
    events: currentData.events,
    messages: currentData.messages,
    pendingHumanInteraction: undefined,
    spaceID: 'space-1',
    subagentRuns: currentData.subagentRuns,
    task: currentData.task,
    taskDetailId,
    taskDetailSource: 'thread',
    todos: currentData.todos,
    tokenUsage: currentData.tokenUsage,
    tokenUsageByRunID: currentData.tokenUsageByRunID,
  });

  return null;
};

const renderHarness = (taskDetailId = 'thread-old') => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => root.render(<HookHarness taskDetailId={taskDetailId} />));

  return {
    root,
    rerender: (nextTaskDetailId: string) =>
      act(() => root.render(<HookHarness taskDetailId={nextTaskDetailId} />)),
  };
};

beforeEach(() => {
  vi.clearAllMocks();
  mockStreamProps.current = undefined;
  mockFetchTaskDetail.mockImplementation(({ id }: { id: string }) =>
    Promise.resolve(createDetail(id)),
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
        run: { run_id: 'run-follow-up', status: 'running' },
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
});
