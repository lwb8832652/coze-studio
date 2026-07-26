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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

const mockCancelTaskThreadRun = vi.hoisted(() => vi.fn());
const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockFetchTaskDetail = vi.hoisted(() => vi.fn());
const mockRetryTaskThreadSubagentRun = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  cancelTaskThreadRun: mockCancelTaskThreadRun,
  createTaskThreadRun: mockCreateTaskThreadRun,
  retryTaskThreadSubagentRun: mockRetryTaskThreadSubagentRun,
}));

vi.mock('../task-detail-loader', () => ({
  fetchTaskDetail: mockFetchTaskDetail,
}));

import { useTaskRunActions } from '../task-run-actions-hook';

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type TaskRunActions = ReturnType<typeof useTaskRunActions>;

let currentActions: TaskRunActions;
const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const createDeferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise;
  });

  return { promise, resolve };
};

const createTask = (id: string) =>
  ({
    id,
    input: JSON.stringify({ message: `重试 ${id}` }),
    title: `任务 ${id}`,
  }) as NonNullable<Parameters<typeof useTaskRunActions>[0]['task']>;

const HookHarness = ({
  applyTaskDetail,
  taskDetailId,
}: {
  applyTaskDetail: ReturnType<typeof vi.fn>;
  taskDetailId: string;
}) => {
  currentActions = useTaskRunActions({
    applyTaskDetail,
    spaceID: 'space-1',
    task: createTask(taskDetailId),
    taskDetailId,
  });

  return null;
};

const renderHarness = (
  applyTaskDetail: ReturnType<typeof vi.fn>,
  taskDetailId = 'task-old',
) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => {
    root.render(
      <HookHarness
        applyTaskDetail={applyTaskDetail}
        taskDetailId={taskDetailId}
      />,
    );
  });

  return {
    container,
    root,
    switchTask: (nextTaskDetailId: string) =>
      act(() => {
        root.render(
          <HookHarness
            applyTaskDetail={applyTaskDetail}
            taskDetailId={nextTaskDetailId}
          />,
        );
      }),
  };
};

beforeEach(() => {
  vi.clearAllMocks();
  mockFetchTaskDetail.mockReset();
  mockFetchTaskDetail.mockResolvedValue({});
  mockCancelTaskThreadRun.mockResolvedValue({});
  mockCreateTaskThreadRun.mockResolvedValue({});
  mockRetryTaskThreadSubagentRun.mockResolvedValue({});
});

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => root.unmount());
    container.remove();
  });
});

describe('useTaskRunActions task request generation', () => {
  it('synchronously excludes conflicting mutations while one is active', async () => {
    const cancelDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockCancelTaskThreadRun.mockReturnValue(cancelDeferred.promise);
    renderHarness(applyTaskDetail);

    let cancelAction!: Promise<void>;
    let retryAction!: Promise<void>;
    await act(async () => {
      cancelAction = currentActions.handleCancelTaskRun('run-old');
      retryAction = currentActions.handleRetryTaskRun('run-old');
      await Promise.resolve();
    });

    expect(mockCancelTaskThreadRun).toHaveBeenCalledTimes(1);
    expect(mockCreateTaskThreadRun).not.toHaveBeenCalled();
    expect(currentActions.taskRunActionsDisabled).toBe(true);

    await act(async () => {
      cancelDeferred.resolve({});
      await cancelAction;
      await retryAction;
    });
  });

  it('separates mutation success from refresh failure and retries only refresh', async () => {
    const applyTaskDetail = vi.fn();
    mockFetchTaskDetail
      .mockRejectedValueOnce(new Error('刷新超时'))
      .mockResolvedValueOnce({ task: { id: 'task-old' } });
    renderHarness(applyTaskDetail);

    await act(async () => {
      await currentActions.handleCancelTaskRun('run-old');
    });
    expect(currentActions.taskRunActionError).toBe(
      '操作已成功但刷新失败，可重试刷新',
    );

    await act(async () => {
      await currentActions.handleCancelTaskRun('run-old');
    });
    expect(mockCancelTaskThreadRun).toHaveBeenCalledTimes(1);
    expect(mockFetchTaskDetail).toHaveBeenCalledTimes(2);
    expect(applyTaskDetail).toHaveBeenCalledTimes(1);
  });

  it('ignores a late cancel refresh after the route task changes', async () => {
    const refreshDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockFetchTaskDetail.mockReturnValue(refreshDeferred.promise);
    const { switchTask } = renderHarness(applyTaskDetail);

    let pendingAction!: Promise<void>;
    await act(async () => {
      pendingAction = currentActions.handleCancelTaskRun('run-old');
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockFetchTaskDetail).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'task-old' }),
    );

    switchTask('task-new');
    expect(currentActions.taskRunActionLoading).toBe('');
    await act(async () => {
      refreshDeferred.resolve({ task: { id: 'task-old' } });
      await pendingAction;
    });

    expect(applyTaskDetail).not.toHaveBeenCalled();
    expect(currentActions.taskRunActionError).toBe('');
  });

  it('ignores a late task retry refresh after the route task changes', async () => {
    const refreshDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockFetchTaskDetail.mockReturnValue(refreshDeferred.promise);
    const { switchTask } = renderHarness(applyTaskDetail);

    let pendingAction!: Promise<void>;
    await act(async () => {
      pendingAction = currentActions.handleRetryTaskRun('run-old');
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockFetchTaskDetail).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'task-old' }),
    );

    switchTask('task-new');
    expect(currentActions.taskRunActionLoading).toBe('');
    await act(async () => {
      refreshDeferred.resolve({ task: { id: 'task-old' } });
      await pendingAction;
    });

    expect(applyTaskDetail).not.toHaveBeenCalled();
    expect(currentActions.taskRunActionError).toBe('');
  });

  it('ignores a late subagent retry refresh after the route task changes', async () => {
    const refreshDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockFetchTaskDetail.mockReturnValue(refreshDeferred.promise);
    const { switchTask } = renderHarness(applyTaskDetail);

    let pendingAction!: Promise<void>;
    await act(async () => {
      pendingAction = currentActions.handleRetrySubagentRun('subagent-old');
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(mockFetchTaskDetail).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'task-old' }),
    );

    switchTask('task-new');
    expect(currentActions.retryingSubagentRunId).toBe('');
    await act(async () => {
      refreshDeferred.resolve({ task: { id: 'task-old' } });
      await pendingAction;
    });

    expect(applyTaskDetail).not.toHaveBeenCalled();
    expect(currentActions.subagentRetryError).toBe('');
  });

  it('does not refresh or update after unmounting a pending action', async () => {
    const cancelDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockCancelTaskThreadRun.mockReturnValue(cancelDeferred.promise);
    const { container, root } = renderHarness(applyTaskDetail);

    let pendingAction!: Promise<void>;
    await act(async () => {
      pendingAction = currentActions.handleCancelTaskRun('run-old');
      await Promise.resolve();
    });
    act(() => root.unmount());
    mountedRoots.pop();
    container.remove();

    await act(async () => {
      cancelDeferred.resolve({});
      await pendingAction;
    });

    expect(mockFetchTaskDetail).not.toHaveBeenCalled();
    expect(applyTaskDetail).not.toHaveBeenCalled();
  });
});
