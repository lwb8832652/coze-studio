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

import { describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';

const mockGetTaskThreadTokenUsage = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getTaskThreadTokenUsage: mockGetTaskThreadTokenUsage,
}));

import { useTaskUsageData } from '../task-usage-loader';
import { createRoot } from './task-test-root-registry';

const createAggregate = (totalTokens: number, callCount: number) => ({
  input_tokens: totalTokens,
  output_tokens: 0,
  total_tokens: totalTokens,
  cost_micros: 0,
  currency: '',
  call_count: callCount,
  lead_agent_tokens: totalTokens,
  subagent_tokens: 0,
  middleware_tokens: 0,
  tool_tokens: 0,
});

const createRows = (start: number, count: number) =>
  Array.from({ length: count }, (_, offset) => {
    const index = start + offset;
    return {
      usage_id: `usage-${index}`,
      thread_id: 'thread-polling',
      run_id: `run-${index}`,
      source: 'lead_agent',
      input_tokens: 2,
      output_tokens: 0,
      total_tokens: 2,
      cost_micros: 0,
      currency: '',
      created_at: 1_720_000_000_000 + index,
    };
  });

const response = (
  rows: ReturnType<typeof createRows>,
  total: number,
  aggregateTokens = total * 2,
) => ({
  data: {
    aggregate: createAggregate(aggregateTokens, total),
    total,
    usage: rows,
  },
  code: 0,
  msg: '',
});

describe('task usage polling and retry monotonicity', () => {
  it('keeps accepted usage visible and queues one follow-up for terminal refresh churn', async () => {
    const slowRefresh = Promise.withResolvers<ReturnType<typeof response>>();
    mockGetTaskThreadTokenUsage.mockReset();
    mockGetTaskThreadTokenUsage
      .mockResolvedValueOnce(response(createRows(0, 1), 1))
      .mockResolvedValueOnce(response(createRows(0, 1), 1))
      .mockReturnValueOnce(slowRefresh.promise)
      .mockResolvedValueOnce(response(createRows(0, 1), 1, 4))
      .mockResolvedValueOnce(response(createRows(0, 1), 1, 6))
      .mockResolvedValueOnce(response(createRows(0, 1), 1, 6));

    let usageState: ReturnType<typeof useTaskUsageData> | undefined;
    const Harness = ({ refreshKey }: { refreshKey: number }) => {
      usageState = useTaskUsageData({
        enabled: true,
        refreshKey,
        threadID: 'thread-polling',
      });
      return null;
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<Harness refreshKey={0} />);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(usageState?.tokenUsage?.totalTokens).toBe(2);

    await act(async () => {
      root.render(<Harness refreshKey={1} />);
      await Promise.resolve();
    });
    const slowSignal = mockGetTaskThreadTokenUsage.mock.calls[2]?.[1]
      ?.signal as AbortSignal | undefined;

    await act(async () => {
      root.render(<Harness refreshKey={2} />);
      root.render(<Harness refreshKey={3} />);
      await Promise.resolve();
    });

    expect(usageState?.tokenUsage?.totalTokens).toBe(2);
    expect(slowSignal?.aborted).toBe(false);
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(3);

    await act(async () => {
      slowRefresh.resolve(response(createRows(0, 1), 1, 4));
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(6);
    expect(usageState?.tokenUsage?.totalTokens).toBe(6);
  });

  it('does not roll back 51 accepted rows when retry returns one stale row', async () => {
    mockGetTaskThreadTokenUsage.mockReset();
    mockGetTaskThreadTokenUsage
      .mockResolvedValueOnce(response(createRows(0, 50), 51, 102))
      .mockResolvedValueOnce(response(createRows(50, 1), 51, 102))
      .mockResolvedValueOnce(response(createRows(0, 50), 51, 102))
      .mockResolvedValueOnce(response(createRows(0, 1), 1, 2))
      .mockResolvedValueOnce(response(createRows(0, 1), 1, 2));

    let usageState: ReturnType<typeof useTaskUsageData> | undefined;
    const Harness = () => {
      usageState = useTaskUsageData({
        enabled: true,
        threadID: 'thread-polling',
      });
      return null;
    };
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<Harness />);
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(usageState?.loadedCount).toBe(51);
    expect(usageState?.tokenUsage?.callCount).toBe(51);
    expect(usageState?.tokenUsageByRunID['run-50']?.totalTokens).toBe(2);

    await act(async () => {
      await usageState?.retry();
      await Promise.resolve();
    });

    expect(usageState?.tokenUsage?.totalTokens).toBe(102);
    expect(usageState?.tokenUsage?.callCount).toBe(51);
    expect(usageState?.loadedCount).toBe(51);
    expect(usageState?.totalCount).toBeGreaterThanOrEqual(51);
    expect(usageState?.tokenUsageByRunID['run-50']?.totalTokens).toBe(2);
  });
});
