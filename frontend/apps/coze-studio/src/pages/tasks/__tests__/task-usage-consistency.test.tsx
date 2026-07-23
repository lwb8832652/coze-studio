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
import { act } from 'react-dom/test-utils';
import type { workbenchTask } from '@coze-studio/api-schema';

import { loadTaskThreadUsage, useTaskUsageData } from '../task-usage-loader';
import type { TaskTokenUsageSnapshot } from '../task-detail-token-usage';
import { createRoot, type Root } from './task-test-root-registry';

const { mockGetTaskThreadTokenUsage } = vi.hoisted(() => ({
  mockGetTaskThreadTokenUsage: vi.fn(),
}));

vi.mock('../service', () => ({
  getTaskThreadTokenUsage: mockGetTaskThreadTokenUsage,
}));

type UsageRow = workbenchTask.TaskThreadTokenUsage;
type UsageAggregate = workbenchTask.TaskThreadTokenUsageAggregate;

const createAggregate = (totalTokens: number): UsageAggregate => ({
  input_tokens: totalTokens,
  output_tokens: 0,
  total_tokens: totalTokens,
  cost_micros: 0,
  call_count: totalTokens > 0 ? 1 : 0,
  lead_agent_tokens: totalTokens,
  subagent_tokens: 0,
  middleware_tokens: 0,
  tool_tokens: 0,
});

const createRow = ({
  id,
  runID = `run-${id}`,
  totalTokens = 1,
}: {
  id: string;
  runID?: string;
  totalTokens?: number;
}): UsageRow => ({
  usage_id: id,
  thread_id: 'thread-usage',
  run_id: runID,
  space_id: 'space-1',
  source: 'lead_agent',
  step_id: '',
  step_index: 0,
  step_name: '',
  model_name: 'safe-model',
  provider: 'safe-provider',
  input_tokens: totalTokens,
  output_tokens: 0,
  total_tokens: totalTokens,
  cost_micros: 0,
  currency: '',
  estimated: false,
  raw_usage: 'private',
  metadata: 'private',
  created_at: 1717000000000,
});

const createResponse = ({
  aggregate,
  rows,
  total,
}: {
  aggregate: UsageAggregate;
  rows: UsageRow[];
  total: number;
}) => ({
  data: {
    aggregate,
    total,
    usage: rows,
  },
  code: 0,
  msg: '',
});

const createSnapshot = ({
  id,
  runID,
  totalTokens,
}: {
  id: string;
  runID: string;
  totalTokens: number;
}): TaskTokenUsageSnapshot => ({
  usageID: id,
  runID,
  source: 'lead_agent',
  stepID: '',
  stepName: '',
  modelName: 'safe-model',
  provider: 'safe-provider',
  inputTokens: totalTokens,
  outputTokens: 0,
  totalTokens,
  costMicros: 0,
  currency: '',
  estimated: false,
  createdAt: 1717000000000,
});

const flushPromises = async () => {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
};

const createDeferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });

  return { promise, reject, resolve };
};

describe('task usage pagination consistency and cancellation', () => {
  beforeEach(() => {
    mockGetTaskThreadTokenUsage.mockReset();
  });

  it('continues past a full page when total is zero and marks the untrusted total partial', async () => {
    const firstPage = Array.from({ length: 50 }, (_, index) =>
      createRow({ id: `usage-${index + 1}` }),
    );
    const secondPage = [createRow({ id: 'usage-51' })];
    mockGetTaskThreadTokenUsage
      .mockResolvedValueOnce(
        createResponse({
          aggregate: createAggregate(50),
          rows: firstPage,
          total: 0,
        }),
      )
      .mockResolvedValueOnce(
        createResponse({
          aggregate: createAggregate(51),
          rows: secondPage,
          total: 0,
        }),
      )
      .mockResolvedValueOnce(
        createResponse({
          aggregate: createAggregate(51),
          rows: firstPage,
          total: 0,
        }),
      );

    const result = await loadTaskThreadUsage('thread-usage');

    expect(
      mockGetTaskThreadTokenUsage.mock.calls.map(([request]) => request.page),
    ).toEqual([1, 2, 1]);
    expect(result.loadedCount).toBe(51);
    expect(result.totalCount).toBe(51);
    expect(result.isPartial).toBe(true);
    expect(result.tokenUsage?.totalTokens).toBe(51);
  });

  it('checks abort and scope after every page so a cancelled request never advances pagination', async () => {
    const deferred = createDeferred<ReturnType<typeof createResponse>>();
    let requestSignal: AbortSignal | undefined;
    mockGetTaskThreadTokenUsage.mockImplementation(
      (
        _request: unknown,
        options?: {
          signal?: AbortSignal;
        },
      ) => {
        requestSignal = options?.signal;
        return deferred.promise;
      },
    );
    const controller = new AbortController();
    const resultPromise = loadTaskThreadUsage('thread-usage', {
      isCurrentScope: () => true,
      signal: controller.signal,
    });

    controller.abort();
    deferred.resolve(
      createResponse({
        aggregate: createAggregate(50),
        rows: Array.from({ length: 50 }, (_, index) =>
          createRow({ id: `usage-${index + 1}` }),
        ),
        total: 1000,
      }),
    );

    await expect(resultPromise).rejects.toMatchObject({
      name: 'AbortError',
    });
    expect(requestSignal?.aborted).toBe(true);
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(1);
  });

  it('aborts task switches and unmounts while queuing an explicit retry behind the active request', async () => {
    const requests: Array<{
      deferred: ReturnType<
        typeof createDeferred<ReturnType<typeof createResponse>>
      >;
      signal?: AbortSignal;
      threadID: string;
    }> = [];
    mockGetTaskThreadTokenUsage.mockImplementation(
      (
        request: { thread_id: string },
        options?: {
          signal?: AbortSignal;
        },
      ) => {
        const deferred = createDeferred<ReturnType<typeof createResponse>>();
        requests.push({
          deferred,
          signal: options?.signal,
          threadID: request.thread_id,
        });
        return deferred.promise;
      },
    );

    let usageState: ReturnType<typeof useTaskUsageData> | undefined;
    const Harness = ({ threadID }: { threadID: string }) => {
      usageState = useTaskUsageData({
        enabled: true,
        threadID,
      });
      return null;
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<Harness threadID="thread-a" />);
      await flushPromises();
    });
    expect(requests).toHaveLength(1);

    await act(async () => {
      root.render(<Harness threadID="thread-b" />);
      await flushPromises();
    });
    expect(requests[0].signal?.aborted).toBe(true);
    expect(requests).toHaveLength(2);

    await act(async () => {
      void usageState?.retry();
      await flushPromises();
    });
    expect(requests[1].signal?.aborted).toBe(false);
    expect(requests).toHaveLength(2);

    await act(async () => {
      requests[1].deferred.resolve(
        createResponse({
          aggregate: createAggregate(1),
          rows: [
            createRow({
              id: 'thread-b-usage-1',
              totalTokens: 1,
            }),
          ],
          total: 1,
        }),
      );
      await flushPromises();
    });
    expect(requests).toHaveLength(3);

    await act(async () => {
      requests[2].deferred.resolve(
        createResponse({
          aggregate: createAggregate(1),
          rows: [
            createRow({
              id: 'thread-b-usage-1',
              totalTokens: 1,
            }),
          ],
          total: 1,
        }),
      );
      await flushPromises();
    });
    expect(requests).toHaveLength(4);

    act(() => {
      root.unmount();
    });
    expect(requests[3].signal?.aborted).toBe(true);

    requests[0].deferred.resolve(
      createResponse({
        aggregate: createAggregate(0),
        rows: [],
        total: 0,
      }),
    );
    requests[3].deferred.resolve(
      createResponse({
        aggregate: createAggregate(0),
        rows: [],
        total: 0,
      }),
    );
    await flushPromises();
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(4);
  });

  it('uses confirmed REST snapshots, treats SSE as coalesced invalidation, and never regresses on retry', async () => {
    const firstPage = Array.from({ length: 50 }, (_, index) =>
      createRow({
        id: `usage-${index + 1}`,
        runID: `run-${index + 1}`,
      }),
    );
    const pageTwo = createDeferred<ReturnType<typeof createResponse>>();
    let retrying = false;
    mockGetTaskThreadTokenUsage.mockImplementation(
      ({ page }: { page?: number }) => {
        if (retrying) {
          return Promise.resolve(
            createResponse({
              aggregate: createAggregate(90),
              rows: [createRow({ id: 'usage-1', runID: 'run-1' })],
              total: 1,
            }),
          );
        }
        const callIndex = mockGetTaskThreadTokenUsage.mock.calls.length;
        if (callIndex === 1) {
          return Promise.resolve(
            createResponse({
              aggregate: createAggregate(100),
              rows: firstPage,
              total: 51,
            }),
          );
        }
        if (page === 2) {
          return pageTwo.promise;
        }

        return Promise.resolve(
          createResponse({
            aggregate: createAggregate(120),
            rows: [
              createRow({
                id: 'usage-1',
                runID: 'run-1',
                totalTokens: 5,
              }),
              ...firstPage.slice(1),
            ],
            total: 51,
          }),
        );
      },
    );

    let usageState: ReturnType<typeof useTaskUsageData> | undefined;
    const Harness = () => {
      usageState = useTaskUsageData({
        enabled: true,
        threadID: 'thread-usage',
      });
      return null;
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root: Root = createRoot(container);

    await act(async () => {
      root.render(<Harness />);
      await flushPromises();
    });
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(2);

    act(() => {
      usageState?.handleTokenUsageSnapshot(
        createSnapshot({
          id: 'usage-1',
          runID: 'run-1',
          totalTokens: 5,
        }),
      );
      usageState?.handleTokenUsageSnapshot(
        createSnapshot({
          id: 'usage-live',
          runID: 'run-live',
          totalTokens: 7,
        }),
      );
    });
    await act(async () => {
      pageTwo.resolve(
        createResponse({
          aggregate: createAggregate(107),
          rows: [createRow({ id: 'usage-51', runID: 'run-51' })],
          total: 51,
        }),
      );
      await flushPromises();
      await flushPromises();
    });

    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(6);
    expect(usageState?.tokenUsage?.totalTokens).toBe(120);
    expect(usageState?.tokenUsageByRunID['run-1']?.totalTokens).toBe(5);
    expect(usageState?.tokenUsageByRunID['run-live']).toBeUndefined();
    expect(usageState?.loadedCount).toBe(51);

    act(() => {
      usageState?.handleTokenUsageSnapshot(
        createSnapshot({
          id: 'usage-1',
          runID: 'run-1',
          totalTokens: 9,
        }),
      );
      usageState?.handleTokenUsageSnapshot(
        createSnapshot({
          id: 'usage-live-2',
          runID: 'run-live-2',
          totalTokens: 6,
        }),
      );
    });
    await act(async () => {
      await flushPromises();
      await flushPromises();
    });
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(9);
    expect(usageState?.tokenUsage?.totalTokens).toBe(120);
    expect(usageState?.tokenUsageByRunID['run-1']?.totalTokens).toBe(5);
    expect(usageState?.loadedCount).toBe(51);

    retrying = true;
    await act(async () => {
      await usageState?.retry();
      await flushPromises();
    });
    expect(usageState?.tokenUsage?.totalTokens).toBe(120);
    expect(usageState?.tokenUsageByRunID['run-1']?.totalTokens).toBe(5);
    expect(usageState?.loadedCount).toBe(51);
  });
});
