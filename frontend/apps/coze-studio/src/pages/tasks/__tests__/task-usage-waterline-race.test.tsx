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

const createAggregate = (totalTokens: number) => ({
  input_tokens: totalTokens,
  output_tokens: 0,
  total_tokens: totalTokens,
  cost_micros: 0,
  currency: '',
  call_count: 1,
  lead_agent_tokens: totalTokens,
  subagent_tokens: 0,
  middleware_tokens: 0,
  tool_tokens: 0,
});

const createRow = (usageID: string, totalTokens: number) => ({
  usage_id: usageID,
  thread_id: 'thread-waterline',
  run_id: `run-${usageID}`,
  source: 'lead_agent',
  input_tokens: totalTokens,
  output_tokens: 0,
  total_tokens: totalTokens,
  cost_micros: 0,
  currency: '',
  created_at: 1_720_000_000_000,
});

const createSnapshot = (usageID: string, totalTokens: number) => ({
  usageID,
  runID: `run-${usageID}`,
  source: 'lead_agent',
  stepID: '',
  stepName: '',
  modelName: '',
  provider: '',
  createdAt: 1_720_000_000_000,
  inputTokens: totalTokens,
  outputTokens: 0,
  totalTokens,
  costMicros: 0,
  currency: '',
  estimated: false,
});

describe('task usage SSE invalidation queue', () => {
  it('keeps REST values authoritative and coalesces SSE during a request into one pending refresh', async () => {
    const slowRefresh = Promise.withResolvers<{
      code: number;
      data: {
        aggregate: ReturnType<typeof createAggregate>;
        total: number;
        usage: Array<ReturnType<typeof createRow>>;
      };
      msg: string;
    }>();
    mockGetTaskThreadTokenUsage.mockReset();
    mockGetTaskThreadTokenUsage
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(100),
          total: 1,
          usage: [createRow('existing', 100)],
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(100),
          total: 1,
          usage: [createRow('existing', 100)],
        },
        code: 0,
        msg: '',
      })
      .mockReturnValueOnce(slowRefresh.promise)
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(120),
          total: 1,
          usage: [createRow('existing', 120)],
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(150),
          total: 2,
          usage: [createRow('existing', 130), createRow('added', 20)],
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(150),
          total: 2,
          usage: [createRow('existing', 130), createRow('added', 20)],
        },
        code: 0,
        msg: '',
      });

    let usageState: ReturnType<typeof useTaskUsageData> | undefined;
    const Harness = () => {
      usageState = useTaskUsageData({
        enabled: true,
        spaceID: 'space-1',
        threadID: 'thread-waterline',
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
    });
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(2);

    act(() => {
      usageState?.handleTokenUsageSnapshot(createSnapshot('existing', 130));
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(usageState?.tokenUsage?.totalTokens).toBe(100);
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(3);

    act(() => {
      usageState?.handleTokenUsageSnapshot(createSnapshot('added', 20));
      usageState?.handleTokenUsageSnapshot(createSnapshot('added', 25));
    });
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(3);

    await act(async () => {
      slowRefresh.resolve({
        data: {
          aggregate: createAggregate(120),
          total: 1,
          usage: [createRow('existing', 120)],
        },
        code: 0,
        msg: '',
      });
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(6);
    expect(usageState?.tokenUsage?.totalTokens).toBe(150);
  });

  it('retains the last confirmed snapshot when invalidation refresh fails and explicit retry recovers', async () => {
    mockGetTaskThreadTokenUsage.mockReset();
    mockGetTaskThreadTokenUsage
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(100),
          total: 1,
          usage: [createRow('existing', 100)],
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(100),
          total: 1,
          usage: [createRow('existing', 100)],
        },
        code: 0,
        msg: '',
      })
      .mockRejectedValueOnce(new Error('usage unavailable'))
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(130),
          total: 1,
          usage: [createRow('existing', 130)],
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          aggregate: createAggregate(130),
          total: 1,
          usage: [createRow('existing', 130)],
        },
        code: 0,
        msg: '',
      });

    let usageState: ReturnType<typeof useTaskUsageData> | undefined;
    const Harness = () => {
      usageState = useTaskUsageData({
        enabled: true,
        spaceID: 'space-1',
        threadID: 'thread-waterline',
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
    });
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(2);

    act(() => {
      usageState?.handleTokenUsageSnapshot(createSnapshot('existing', 130));
    });
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(usageState?.tokenUsage?.totalTokens).toBe(100);
    expect(usageState?.error).toBeTruthy();

    await act(async () => {
      await usageState?.retry();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(usageState?.error).toBe('');
    expect(usageState?.tokenUsage?.totalTokens).toBe(130);
  });
});
