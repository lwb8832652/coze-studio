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

const mockGetTaskThreadTokenUsage = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  getTaskThreadTokenUsage: mockGetTaskThreadTokenUsage,
}));

import { loadTaskThreadUsage } from '../task-usage-loader';

const createServerAggregate = (totalTokens: number) => ({
  input_tokens: totalTokens,
  output_tokens: 0,
  total_tokens: totalTokens,
  cost_micros: 0,
  currency: '',
  call_count: totalTokens > 0 ? 1 : 0,
  lead_agent_tokens: totalTokens,
  subagent_tokens: 0,
  middleware_tokens: 0,
  tool_tokens: 0,
});

const createRows = (page: number, count: number) =>
  Array.from({ length: count }, (_, offset) => {
    const index = (page - 1) * 50 + offset;
    return {
      usage_id: `usage-${index}`,
      thread_id: 'thread-total',
      run_id: `run-${index}`,
      source: 'lead_agent',
      input_tokens: 1,
      output_tokens: 1,
      total_tokens: 2,
      cost_micros: 0,
      currency: '',
      created_at: 1_720_000_000_000 + index,
    };
  });

describe('task usage terminal data contracts', () => {
  it.each([1, 0.5])(
    'treats contradictory total=%s as unknown and continues until a short page',
    async total => {
      mockGetTaskThreadTokenUsage.mockReset();
      mockGetTaskThreadTokenUsage.mockImplementation(
        ({ page }: { page: number }) =>
          Promise.resolve({
            data: {
              usage: createRows(page, page === 1 ? 50 : 1),
              total,
              aggregate: createServerAggregate(102),
            },
            code: 0,
            msg: '',
          }),
      );

      const result = await loadTaskThreadUsage('thread-total', {
        spaceID: 'space-1',
      });

      expect(
        mockGetTaskThreadTokenUsage.mock.calls.map(([request]) => request.page),
      ).toEqual([1, 2, 1]);
      expect(result.loadedCount).toBe(51);
      expect(result.isPartial).toBe(true);
    },
  );

  it('marks a contradictory total partial when full pages reach the cap', async () => {
    mockGetTaskThreadTokenUsage.mockReset();
    mockGetTaskThreadTokenUsage.mockImplementation(
      ({ page }: { page: number }) =>
        Promise.resolve({
          data: {
            usage: createRows(page, 50),
            total: 0.5,
            aggregate: createServerAggregate(2000),
          },
          code: 0,
          msg: '',
        }),
    );

    const result = await loadTaskThreadUsage('thread-total', {
      spaceID: 'space-1',
    });

    expect(result.loadedCount).toBe(1000);
    expect(result.isPartial).toBe(true);
    expect(mockGetTaskThreadTokenUsage).toHaveBeenCalledTimes(21);
  });
});
