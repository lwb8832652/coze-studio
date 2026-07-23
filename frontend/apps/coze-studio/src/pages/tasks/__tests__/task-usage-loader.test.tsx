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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';

import { createRoot, type Root } from './task-test-root-registry';
import type { TaskTokenUsageSnapshot } from '../task-detail-token-usage';

const mockGetTaskThreadTokenUsage = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getTaskThreadTokenUsage: mockGetTaskThreadTokenUsage,
}));

import { loadTaskThreadUsage, useTaskUsageData } from '../task-usage-loader';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const createAggregate = (totalTokens = 7500) => ({
  input_tokens: Math.max(0, totalTokens - 1500),
  output_tokens: Math.min(1500, totalTokens),
  total_tokens: totalTokens,
  cost_micros: 0,
  call_count: totalTokens > 0 ? 75 : 0,
  lead_agent_tokens: totalTokens,
  subagent_tokens: 0,
  middleware_tokens: 0,
  tool_tokens: 0,
});

const createUsageRow = (index: number, threadID = 'thread-1') => ({
  usage_id: `usage-${threadID}-${index}`,
  thread_id: threadID,
  run_id: `run-${threadID}-${index}`,
  space_id: 'space-1',
  source: 'lead_agent',
  step_id: `step-${index}`,
  step_index: index,
  step_name: 'answer',
  model_name: 'gpt-4.1',
  provider: 'openai',
  input_tokens: 80,
  output_tokens: 20,
  total_tokens: 100,
  cost_micros: 0,
  currency: '',
  estimated: false,
  raw_usage: '{"prompt":"must-not-render"}',
  metadata: '{"provider_raw_body":"must-not-render"}',
  created_at: 1_717_000_000_000 + index,
});

const createUsageResponse = ({
  page,
  pageSize = 50,
  threadID = 'thread-1',
  total = 75,
}: {
  page: number;
  pageSize?: number;
  threadID?: string;
  total?: number;
}) => {
  const start = (page - 1) * pageSize;
  const end = Math.min(start + pageSize, total);

  return {
    data: {
      usage: Array.from({ length: Math.max(0, end - start) }, (_, offset) =>
        createUsageRow(start + offset, threadID),
      ),
      total,
      aggregate: createAggregate(total * 100),
    },
    code: 0,
    msg: '',
  };
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

describe('loadTaskThreadUsage', () => {
  beforeEach(() => {
    mockGetTaskThreadTokenUsage.mockReset();
  });

  it('loads every usage row beyond the first 50-item page', async () => {
    mockGetTaskThreadTokenUsage.mockImplementation(
      ({ page }: { page: number }) =>
        Promise.resolve(createUsageResponse({ page })),
    );

    const result = await loadTaskThreadUsage('thread-1');

    expect(mockGetTaskThreadTokenUsage).toHaveBeenNthCalledWith(
      1,
      {
        thread_id: 'thread-1',
        page: 1,
        page_size: 50,
      },
      { signal: undefined },
    );
    expect(mockGetTaskThreadTokenUsage).toHaveBeenNthCalledWith(
      2,
      {
        thread_id: 'thread-1',
        page: 2,
        page_size: 50,
      },
      { signal: undefined },
    );
    expect(mockGetTaskThreadTokenUsage).toHaveBeenNthCalledWith(
      3,
      {
        thread_id: 'thread-1',
        page: 1,
        page_size: 50,
      },
      { signal: undefined },
    );
    expect(result.loadedCount).toBe(75);
    expect(result.totalCount).toBe(75);
    expect(result.isPartial).toBe(false);
    expect(result.tokenUsage?.totalTokens).toBe(7500);
    expect(result.tokenUsageByRunID['run-thread-1-74']?.totalTokens).toBe(100);
  });

  it('keeps the server aggregate and marks rows partial when a later page fails', async () => {
    mockGetTaskThreadTokenUsage.mockImplementation(
      ({ page }: { page: number }) =>
        page === 1
          ? Promise.resolve(createUsageResponse({ page }))
          : Promise.reject(new Error('raw provider response must stay hidden')),
    );

    const result = await loadTaskThreadUsage('thread-1');

    expect(result.tokenUsage?.totalTokens).toBe(7500);
    expect(result.loadedCount).toBe(50);
    expect(result.totalCount).toBe(75);
    expect(result.isPartial).toBe(true);
    expect(result.error).toBe('Token 用量明细加载失败，请重试');
    expect(result.error).not.toContain('provider');
    expect(result.tokenUsageByRunID['run-thread-1-49']?.totalTokens).toBe(100);
    expect(result.tokenUsageByRunID['run-thread-1-74']).toBeUndefined();
  });
});

type UsageState = ReturnType<typeof useTaskUsageData>;
let currentUsageState: UsageState;
const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const UsageHarness = ({ threadID }: { threadID: string }) => {
  currentUsageState = useTaskUsageData({
    enabled: true,
    threadID,
  });

  return null;
};

const renderUsageHarness = (threadID: string) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => root.render(<UsageHarness threadID={threadID} />));

  return {
    rerender: (nextThreadID: string) =>
      act(() => root.render(<UsageHarness threadID={nextThreadID} />)),
  };
};

const flushUsage = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

const createSnapshot = (
  usageID: string,
  runID: string,
): TaskTokenUsageSnapshot => ({
  usageID,
  runID,
  source: 'lead_agent',
  stepID: 'answer',
  stepName: 'answer',
  modelName: 'gpt-4.1',
  provider: 'openai',
  inputTokens: 12,
  outputTokens: 8,
  totalTokens: 20,
  costMicros: 0,
  currency: '',
  estimated: false,
  createdAt: 1_717_000_000_000,
});

describe('useTaskUsageData scope', () => {
  beforeEach(() => {
    mockGetTaskThreadTokenUsage.mockReset();
  });

  afterEach(() => {
    mountedRoots.splice(0).forEach(({ container, root }) => {
      act(() => root.unmount());
      container.remove();
    });
  });

  it('ignores late REST data and loading/error from the previous route', async () => {
    const oldRequest = createDeferred<ReturnType<typeof createUsageResponse>>();
    mockGetTaskThreadTokenUsage.mockImplementation(
      ({ thread_id }: { thread_id: string }) =>
        thread_id === 'thread-old'
          ? oldRequest.promise
          : Promise.resolve(
              createUsageResponse({
                page: 1,
                threadID: 'thread-new',
                total: 1,
              }),
            ),
    );
    const { rerender } = renderUsageHarness('thread-old');
    expect(currentUsageState.loading).toBe(true);

    rerender('thread-new');
    expect(currentUsageState.error).toBe('');
    expect(currentUsageState.tokenUsage).toBeUndefined();
    await flushUsage();
    expect(currentUsageState.tokenUsage?.totalTokens).toBe(100);

    await act(async () => {
      oldRequest.resolve(
        createUsageResponse({
          page: 1,
          threadID: 'thread-old',
          total: 1,
        }),
      );
      await oldRequest.promise;
    });

    expect(currentUsageState.loading).toBe(false);
    expect(currentUsageState.error).toBe('');
    expect(currentUsageState.tokenUsage?.totalTokens).toBe(100);
    expect(
      currentUsageState.tokenUsageByRunID['run-thread-old-0'],
    ).toBeUndefined();
  });

  it('ignores an old SSE snapshot after the route changes', async () => {
    mockGetTaskThreadTokenUsage.mockImplementation(
      ({ thread_id }: { thread_id: string }) =>
        Promise.resolve(
          createUsageResponse({
            page: 1,
            threadID: thread_id,
            total: 1,
          }),
        ),
    );
    const { rerender } = renderUsageHarness('thread-old');
    await flushUsage();
    const oldSnapshotHandler = currentUsageState.handleTokenUsageSnapshot;

    rerender('thread-new');
    await flushUsage();
    act(() => {
      oldSnapshotHandler(createSnapshot('usage-old-late', 'run-old-late'));
    });

    expect(currentUsageState.tokenUsageByRunID['run-old-late']).toBeUndefined();
    expect(
      currentUsageState.tokenUsageByRunID['run-thread-new-0']?.totalTokens,
    ).toBe(100);
  });

  it('ignores completion of an old retry after a new route has loaded', async () => {
    const oldRetry = createDeferred<ReturnType<typeof createUsageResponse>>();
    mockGetTaskThreadTokenUsage
      .mockResolvedValueOnce({
        data: undefined,
        code: 500,
        msg: 'old failure',
      })
      .mockReturnValueOnce(oldRetry.promise)
      .mockImplementation(({ thread_id }: { thread_id: string }) =>
        Promise.resolve(
          createUsageResponse({
            page: 1,
            threadID: thread_id,
            total: 1,
          }),
        ),
      );
    const { rerender } = renderUsageHarness('thread-old');
    await flushUsage();
    expect(currentUsageState.error).toBeTruthy();

    let pendingRetry!: Promise<void>;
    await act(async () => {
      pendingRetry = currentUsageState.retry();
      await Promise.resolve();
    });
    rerender('thread-new');
    await flushUsage();

    await act(async () => {
      oldRetry.resolve(
        createUsageResponse({
          page: 1,
          threadID: 'thread-old',
          total: 1,
        }),
      );
      await pendingRetry;
    });

    expect(currentUsageState.error).toBe('');
    expect(currentUsageState.loading).toBe(false);
    expect(
      currentUsageState.tokenUsageByRunID['run-thread-old-0'],
    ).toBeUndefined();
    expect(
      currentUsageState.tokenUsageByRunID['run-thread-new-0']?.totalTokens,
    ).toBe(100);
  });
});
