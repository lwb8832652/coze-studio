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

import { type ReactElement } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { useTaskDetailActions } from '../task-detail-hooks';
import type { WorkbenchComposerSubmitPayload } from '../../workbench/components/types';
import type {
  HumanInteractionResponse,
  WorkbenchRun,
} from '../../workbench/thread-client';
import type { PendingHumanInteraction } from '../task-human-interaction';

const mockSendFollowUpMessage = vi.hoisted(() => vi.fn());
const mockCreateFollowUpIdempotencyKey = vi.hoisted(() => vi.fn());
const mockFetchTaskDetail = vi.hoisted(() => vi.fn());
const mockCancelTaskThreadRun = vi.hoisted(() => vi.fn());
const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockResumeTaskThreadRun = vi.hoisted(() => vi.fn());
const mockRetryTaskThreadSubagentRun = vi.hoisted(() => vi.fn());

vi.mock('../task-follow-up', () => ({
  createFollowUpIdempotencyKey: mockCreateFollowUpIdempotencyKey,
  sendFollowUpMessage: mockSendFollowUpMessage,
}));

vi.mock('../task-detail-loader', () => ({
  fetchTaskDetail: mockFetchTaskDetail,
}));

vi.mock('../service', () => ({
  cancelTaskThreadRun: mockCancelTaskThreadRun,
  createTaskThreadRun: mockCreateTaskThreadRun,
  listTaskThreadArtifacts: vi.fn(),
  resumeTaskThreadRun: mockResumeTaskThreadRun,
  retryTaskThreadSubagentRun: mockRetryTaskThreadSubagentRun,
}));

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

type FollowUpActions = ReturnType<typeof useTaskDetailActions>;

let currentActions: FollowUpActions;
const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const submitPayload = {
  message: '旧任务消息',
  mode: 'flash',
  files: [new File(['context'], 'context.txt')],
} as WorkbenchComposerSubmitPayload;

const createDeferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return { promise, reject, resolve };
};

const createRun = (runId: string): WorkbenchRun => ({
  run_id: runId,
  thread_id: 'task-1',
  space_id: 'space-1',
  assistant_id: 'agent',
  status: 'running',
  metadata: '',
  multitask_strategy: '',
  attempt_kind: 'resume',
  run_kind: 'task',
  stream_modes: ['events'],
  on_disconnect: 'continue',
  durability: 'async',
  created_at: 1,
  updated_at: 1,
});

const pendingHumanInteraction = {
  interruptId: 'interrupt-1',
  interactionId: 'interaction-1',
  sourceRunId: 'run-interrupted',
  kind: 'confirmation',
  prompt: {
    schema: 'coze.human_interaction.v1',
    interaction_id: 'interaction-1',
    kind: 'confirmation',
  },
  event: {},
} as PendingHumanInteraction;

const humanResponse: HumanInteractionResponse = {
  schema: 'coze.human_interaction_response.v1',
  interaction_id: 'interaction-1',
  kind: 'confirmation',
  decision: 'approve',
};

const HookHarness = ({
  applyTaskDetail,
  commitTopLevelRun = vi.fn(),
  pendingInteraction,
  taskDetailId,
}: {
  applyTaskDetail: ReturnType<typeof vi.fn>;
  commitTopLevelRun?: ReturnType<typeof vi.fn>;
  pendingInteraction?: PendingHumanInteraction;
  taskDetailId: string;
}) => {
  currentActions = useTaskDetailActions({
    applyTaskDetail,
    artifacts: [],
    events: [],
    messages: [],
    commitTopLevelRun,
    pendingHumanInteraction: pendingInteraction,
    spaceID: 'space-1',
    subagentRuns: [],
    taskDetailId,
    todos: [],
    tokenUsageByRunID: {},
  });

  return null;
};

const renderHookHarness = (
  element: ReactElement,
): { container: HTMLDivElement; root: Root } => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => {
    root.render(element);
  });

  return { container, root };
};

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });
});

beforeEach(() => {
  vi.clearAllMocks();
  let keyIndex = 0;
  mockCreateFollowUpIdempotencyKey.mockImplementation(
    (threadId: string) => `${threadId}:key-${++keyIndex}:followup`,
  );
  mockFetchTaskDetail.mockResolvedValue({});
  mockSendFollowUpMessage.mockResolvedValue({});
  mockCancelTaskThreadRun.mockResolvedValue({});
  mockCreateTaskThreadRun.mockResolvedValue({});
  mockResumeTaskThreadRun.mockResolvedValue({});
  mockRetryTaskThreadSubagentRun.mockResolvedValue({});
});

describe('useTaskDetailActions follow-up request scope', () => {
  it('reuses an idempotency key after an ambiguous timeout', async () => {
    mockSendFollowUpMessage
      .mockRejectedValueOnce(
        Object.assign(new Error('请求超时'), { name: 'TimeoutError' }),
      )
      .mockResolvedValueOnce({});
    renderHookHarness(
      <HookHarness applyTaskDetail={vi.fn()} taskDetailId="task-1" />,
    );

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
      await currentActions.handleFollowUpSubmit(submitPayload);
    });

    const firstKey = mockSendFollowUpMessage.mock.calls[0]?.[0].idempotencyKey;
    const secondKey = mockSendFollowUpMessage.mock.calls[1]?.[0].idempotencyKey;
    expect(firstKey).toEqual(expect.any(String));
    expect(secondKey).toBe(firstKey);
  });

  it('rotates the idempotency key when the payload changes', async () => {
    mockSendFollowUpMessage
      .mockRejectedValueOnce(
        Object.assign(new Error('请求超时'), { name: 'TimeoutError' }),
      )
      .mockResolvedValueOnce({});
    renderHookHarness(
      <HookHarness applyTaskDetail={vi.fn()} taskDetailId="task-1" />,
    );

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
      await currentActions.handleFollowUpSubmit({
        ...submitPayload,
        message: '编辑后的消息',
      });
    });

    expect(mockSendFollowUpMessage.mock.calls[0]?.[0].idempotencyKey).not.toBe(
      mockSendFollowUpMessage.mock.calls[1]?.[0].idempotencyKey,
    );
  });

  it('rotates the idempotency key after confirmed success', async () => {
    renderHookHarness(
      <HookHarness applyTaskDetail={vi.fn()} taskDetailId="task-1" />,
    );

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
      await currentActions.handleFollowUpSubmit(submitPayload);
    });

    expect(mockSendFollowUpMessage.mock.calls[0]?.[0].idempotencyKey).not.toBe(
      mockSendFollowUpMessage.mock.calls[1]?.[0].idempotencyKey,
    );
  });

  it('rotates the idempotency key after explicit cancellation', async () => {
    mockSendFollowUpMessage
      .mockRejectedValueOnce(
        Object.assign(new Error('用户取消'), { name: 'AbortError' }),
      )
      .mockResolvedValueOnce({});
    renderHookHarness(
      <HookHarness applyTaskDetail={vi.fn()} taskDetailId="task-1" />,
    );

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
      await currentActions.handleFollowUpSubmit(submitPayload);
    });

    expect(mockSendFollowUpMessage.mock.calls[0]?.[0].idempotencyKey).not.toBe(
      mockSendFollowUpMessage.mock.calls[1]?.[0].idempotencyKey,
    );
  });

  it('resets the draft on task switch and ignores an old submit response', async () => {
    const submitDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockSendFollowUpMessage.mockReturnValue(submitDeferred.promise);
    const { root } = renderHookHarness(
      <HookHarness applyTaskDetail={applyTaskDetail} taskDetailId="task-old" />,
    );

    act(() => currentActions.setFollowUpValue('旧任务草稿'));
    let oldSubmit!: Promise<void>;
    await act(async () => {
      oldSubmit = currentActions.handleFollowUpSubmit(submitPayload);
      await Promise.resolve();
    });
    expect(currentActions.followUpLoading).toBe(true);

    act(() => {
      root.render(
        <HookHarness
          applyTaskDetail={applyTaskDetail}
          taskDetailId="task-new"
        />,
      );
    });
    expect(currentActions.followUpValue).toBe('');
    expect(currentActions.followUpLoading).toBe(false);
    expect(currentActions.followUpResetKey).toBe(0);
    act(() => currentActions.setFollowUpValue('新任务草稿'));

    await act(async () => {
      submitDeferred.resolve({});
      await oldSubmit;
    });

    expect(currentActions.followUpValue).toBe('新任务草稿');
    expect(currentActions.followUpResetKey).toBe(0);
    expect(mockFetchTaskDetail).not.toHaveBeenCalled();
    expect(applyTaskDetail).not.toHaveBeenCalled();
  });

  it('ignores an old refresh response after switching tasks', async () => {
    const refreshDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockFetchTaskDetail.mockReturnValue(refreshDeferred.promise);
    const { root } = renderHookHarness(
      <HookHarness applyTaskDetail={applyTaskDetail} taskDetailId="task-old" />,
    );

    act(() => currentActions.setFollowUpValue('旧任务草稿'));
    let oldSubmit!: Promise<void>;
    await act(async () => {
      oldSubmit = currentActions.handleFollowUpSubmit(submitPayload);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(currentActions.followUpValue).toBe('');
    expect(currentActions.followUpResetKey).toBe(1);

    act(() => {
      root.render(
        <HookHarness
          applyTaskDetail={applyTaskDetail}
          taskDetailId="task-new"
        />,
      );
    });
    act(() => currentActions.setFollowUpValue('新任务草稿'));

    await act(async () => {
      refreshDeferred.resolve({ task: { id: 'task-old' } });
      await oldSubmit;
    });

    expect(currentActions.followUpValue).toBe('新任务草稿');
    expect(currentActions.followUpResetKey).toBe(0);
    expect(applyTaskDetail).not.toHaveBeenCalled();
  });

  it('preserves the draft and reset scope when submit fails', async () => {
    mockSendFollowUpMessage.mockRejectedValue(new Error('发送失败'));
    const applyTaskDetail = vi.fn();
    renderHookHarness(
      <HookHarness applyTaskDetail={applyTaskDetail} taskDetailId="task-1" />,
    );
    act(() => currentActions.setFollowUpValue('保留的草稿'));

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
    });

    expect(currentActions.followUpValue).toBe('保留的草稿');
    expect(currentActions.followUpResetKey).toBe(0);
    expect(currentActions.followUpError).toBe('发送失败');
    expect(mockFetchTaskDetail).not.toHaveBeenCalled();
  });

  it('preserves the draft when submit is canceled', async () => {
    mockSendFollowUpMessage.mockRejectedValue(
      Object.assign(new Error('用户取消'), { name: 'AbortError' }),
    );
    const applyTaskDetail = vi.fn();
    renderHookHarness(
      <HookHarness applyTaskDetail={applyTaskDetail} taskDetailId="task-1" />,
    );
    act(() => currentActions.setFollowUpValue('取消后仍保留'));

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
    });

    expect(currentActions.followUpValue).toBe('取消后仍保留');
    expect(currentActions.followUpResetKey).toBe(0);
    expect(currentActions.followUpError).toBe('发送已取消');
    expect(mockFetchTaskDetail).not.toHaveBeenCalled();
  });

  it('invalidates a pending follow-up generation when unmounted', async () => {
    const submitDeferred = createDeferred<unknown>();
    const applyTaskDetail = vi.fn();
    mockSendFollowUpMessage.mockReturnValue(submitDeferred.promise);
    const { container, root } = renderHookHarness(
      <HookHarness applyTaskDetail={applyTaskDetail} taskDetailId="task-1" />,
    );

    let pendingSubmit!: Promise<void>;
    await act(async () => {
      pendingSubmit = currentActions.handleFollowUpSubmit(submitPayload);
      await Promise.resolve();
    });
    act(() => root.unmount());
    mountedRoots.pop();
    container.remove();

    await act(async () => {
      submitDeferred.resolve({});
      await pendingSubmit;
    });

    expect(mockFetchTaskDetail).not.toHaveBeenCalled();
    expect(applyTaskDetail).not.toHaveBeenCalled();
  });

  it('keeps the successful reset when detail refresh fails', async () => {
    const committedRun = createRun('run-follow-up');
    const commitTopLevelRun = vi.fn();
    mockSendFollowUpMessage.mockResolvedValue({
      kind: 'thread',
      run: committedRun,
    });
    mockFetchTaskDetail.mockRejectedValue(new Error('刷新失败'));
    const applyTaskDetail = vi.fn();
    renderHookHarness(
      <HookHarness
        applyTaskDetail={applyTaskDetail}
        commitTopLevelRun={commitTopLevelRun}
        taskDetailId="task-1"
      />,
    );
    act(() => currentActions.setFollowUpValue('已经发送的消息'));

    await act(async () => {
      await currentActions.handleFollowUpSubmit(submitPayload);
    });

    expect(currentActions.followUpValue).toBe('');
    expect(currentActions.followUpResetKey).toBe(1);
    expect(currentActions.followUpError).toBe(
      '消息已发送但刷新失败，可重试刷新',
    );
    expect(currentActions.followUpLoading).toBe(false);
    expect(mockSendFollowUpMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        spaceId: 'space-1',
        threadId: 'task-1',
      }),
    );
    expect(commitTopLevelRun).toHaveBeenCalledWith(committedRun);
    expect(commitTopLevelRun.mock.invocationCallOrder[0]).toBeLessThan(
      mockFetchTaskDetail.mock.invocationCallOrder[0],
    );
  });

  it('commits a resumed top-level Run before refresh and does not repeat the successful write', async () => {
    const committedRun = createRun('run-resume-new');
    const commitTopLevelRun = vi.fn();
    mockResumeTaskThreadRun.mockResolvedValue({ data: committedRun });
    mockFetchTaskDetail.mockRejectedValue(new Error('刷新失败'));
    renderHookHarness(
      <HookHarness
        applyTaskDetail={vi.fn()}
        commitTopLevelRun={commitTopLevelRun}
        pendingInteraction={pendingHumanInteraction}
        taskDetailId="task-1"
      />,
    );

    await act(async () => {
      await currentActions.handleHumanInteractionSubmit(humanResponse);
    });

    expect(mockResumeTaskThreadRun).toHaveBeenCalledWith({
      interrupt_id: 'interrupt-1',
      response: humanResponse,
      run_id: 'run-interrupted',
      space_id: 'space-1',
      thread_id: 'task-1',
    });
    expect(commitTopLevelRun).toHaveBeenCalledTimes(1);
    expect(commitTopLevelRun).toHaveBeenCalledWith(committedRun);
    expect(commitTopLevelRun.mock.invocationCallOrder[0]).toBeLessThan(
      mockFetchTaskDetail.mock.invocationCallOrder[0],
    );

    await act(async () => {
      await currentActions.handleHumanInteractionSubmit(humanResponse);
    });

    expect(mockResumeTaskThreadRun).toHaveBeenCalledTimes(1);
    expect(mockFetchTaskDetail).toHaveBeenCalledTimes(2);
    expect(commitTopLevelRun).toHaveBeenCalledTimes(1);
  });
});
