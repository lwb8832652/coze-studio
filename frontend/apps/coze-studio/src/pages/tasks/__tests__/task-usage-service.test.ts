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

import { getTaskThreadTokenUsage } from '../task-usage-service';
import { tokenUsageTransportFixture } from '../../workbench/thread-client/__tests__/fixtures';

const canonicalGetTokenUsage = vi.hoisted(() => vi.fn());
const generatedAbort = vi.hoisted(() => vi.fn());
const generatedTokenUsage = vi.hoisted(() =>
  Object.assign(vi.fn(), {
    abort: generatedAbort,
  }),
);
const spaceStore = vi.hoisted(() => ({
  getSpaceId: vi.fn(() => 'store-space'),
}));

vi.mock('../../workbench/thread-client/canonical-thread-client', () => ({
  CanonicalThreadClient: vi.fn(function recordingCanonicalThreadClient() {
    return {
      contract: 'canonical_v1',
      getTokenUsage: canonicalGetTokenUsage,
    };
  }),
  CanonicalThreadCoreClient: vi.fn(
    function recordingCanonicalThreadCoreClient() {
      return {
        contract: 'canonical_v1',
        getTokenUsage: canonicalGetTokenUsage,
      };
    },
  ),
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: Object.assign(vi.fn(), {
    getState: () => ({ getSpaceId: spaceStore.getSpaceId }),
  }),
}));

vi.mock('@coze-studio/api-schema', () => ({
  workbenchTask: {
    GetTaskThreadTokenUsage: {
      withAbort: () => generatedTokenUsage,
    },
  },
}));

const usage = tokenUsageTransportFixture.visible;
const successResponse = {
  code: 0,
  msg: 'success',
  data: {
    usage: usage.items,
    total: usage.total,
    aggregate: usage.aggregate,
    run_aggregates: usage.run_aggregates,
  },
};

beforeEach(() => {
  vi.clearAllMocks();
  canonicalGetTokenUsage.mockResolvedValue(usage);
  generatedTokenUsage.mockResolvedValue(successResponse);
});

describe('task usage canonical client cancellation boundary', () => {
  it('rejects a request without an explicit route workspace', async () => {
    await expect(
      getTaskThreadTokenUsage({
        thread_id: 'thread-missing-space',
      } as Parameters<typeof getTaskThreadTokenUsage>[0]),
    ).rejects.toThrow('workspace');

    expect(canonicalGetTokenUsage).not.toHaveBeenCalled();
    expect(spaceStore.getSpaceId).not.toHaveBeenCalled();
  });

  it('passes an external AbortSignal and explicit space to the canonical client', async () => {
    canonicalGetTokenUsage.mockImplementation(
      ({ signal }: { signal?: AbortSignal }) =>
        new Promise((resolve, reject) => {
          signal?.addEventListener(
            'abort',
            () =>
              reject(
                new DOMException('Task usage request aborted', 'AbortError'),
              ),
            { once: true },
          );
          void resolve;
        }),
    );
    const controller = new AbortController();
    const request = getTaskThreadTokenUsage(
      {
        space_id: 'space-route',
        thread_id: 'thread-abort',
        page: 1,
        page_size: 50,
      },
      { signal: controller.signal },
    );

    controller.abort();

    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
    expect(canonicalGetTokenUsage).toHaveBeenCalledWith({
      space_id: 'space-route',
      thread_id: 'thread-abort',
      page: 1,
      page_size: 50,
      signal: controller.signal,
    });
    expect(generatedAbort).not.toHaveBeenCalled();
  });

  it('prefers an explicit request space and preserves the usage envelope', async () => {
    const response = await getTaskThreadTokenUsage({
      thread_id: 'thread-explicit',
      space_id: 'explicit-space',
      run_id: 'run-1',
      include_child_runs: false,
      page: 2,
      page_size: 10,
    } as Parameters<typeof getTaskThreadTokenUsage>[0]);

    expect(response).toEqual(successResponse);
    expect(canonicalGetTokenUsage).toHaveBeenCalledWith({
      space_id: 'explicit-space',
      thread_id: 'thread-explicit',
      run_id: 'run-1',
      include_child_runs: false,
      page: 2,
      page_size: 10,
    });
    expect(spaceStore.getSpaceId).not.toHaveBeenCalled();
  });

  it('keeps AbortError naming for an already-aborted signal', async () => {
    const controller = new AbortController();
    controller.abort();

    await expect(
      getTaskThreadTokenUsage(
        { space_id: 'space-route', thread_id: 'thread-pre-abort' },
        { signal: controller.signal },
      ),
    ).rejects.toMatchObject({ name: 'AbortError' });
    expect(canonicalGetTokenUsage).not.toHaveBeenCalled();
  });
});
