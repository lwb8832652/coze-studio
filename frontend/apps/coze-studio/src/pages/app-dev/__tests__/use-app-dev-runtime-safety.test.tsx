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

import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  getAppDevRuntimeStatus,
  listAppDevRuntimeLogs,
  startAppDevRuntime,
  stopAppDevRuntime,
} from '../service';
import { useAppDevRuntime } from '../hooks/use-app-dev-runtime';
import { readAppDevPendingOperation } from '../utils/provider-operation';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', () => ({
  getAppDevRuntimeStatus: vi.fn(),
  isAppDevDeterministicError: vi.fn(() => false),
  keepAliveAppDevRuntime: vi.fn(),
  listAppDevRuntimeLogs: vi.fn(),
  normalizeAppDevError: vi.fn(() => '安全错误'),
  restartAppDevRuntime: vi.fn(),
  startAppDevRuntime: vi.fn(),
  stopAppDevRuntime: vi.fn(),
}));

const flush = async () => {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
};

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => {
    resolve = next;
  });
  return { promise, resolve };
};

describe('useAppDevRuntime safety boundaries', () => {
  const scope = {
    spaceId: 'space-1',
    projectId: 'project-1',
    principalId: 'user-1',
  };
  let root: Root | undefined;
  let hook: ReturnType<typeof useAppDevRuntime> | undefined;

  const render = async (strict = false) => {
    const Harness = () => {
      hook = useAppDevRuntime(
        scope.spaceId,
        scope.projectId,
        scope.principalId,
      );
      return null;
    };
    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(
        strict ? (
          <StrictMode>
            <Harness />
          </StrictMode>
        ) : (
          <Harness />
        ),
      );
      await flush();
    });
  };

  beforeEach(() => {
    vi.useFakeTimers();
    sessionStorage.clear();
    vi.mocked(listAppDevRuntimeLogs).mockResolvedValue({ items: [] });
  });

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
        await flush();
      });
    }
    root = undefined;
    hook = undefined;
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('lets stop supersede an in-flight start and persists only the winner', async () => {
    vi.mocked(getAppDevRuntimeStatus)
      .mockResolvedValueOnce({
        generation: 3,
        status: 'stopped',
        canStart: false,
        recovering: false,
        stopping: false,
      })
      .mockResolvedValue({
        generation: 3,
        status: 'stopping',
        canStart: false,
        recovering: false,
        stopping: true,
      });
    const startResult =
      deferred<Awaited<ReturnType<typeof startAppDevRuntime>>>();
    let startSignal: AbortSignal | undefined;
    vi.mocked(startAppDevRuntime).mockImplementation(input => {
      startSignal = input.signal;
      return startResult.promise;
    });
    vi.mocked(stopAppDevRuntime).mockResolvedValue({
      generation: 3,
      status: 'stopping',
      canStart: false,
      recovering: false,
      stopping: true,
    });
    await render();

    act(() => void hook?.start());
    await act(flush);
    act(() => void hook?.stop());
    await act(flush);

    expect(startSignal?.aborted).toBe(true);
    expect(
      readAppDevPendingOperation(scope, 'runtime-manual-start'),
    ).toBeUndefined();
    expect(readAppDevPendingOperation(scope, 'runtime-stop')).toBeDefined();
    await act(async () => {
      startResult.resolve({
        generation: 3,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
      await flush();
    });
    expect(hook?.runtime.status).toBe('stopping');
  });

  it('refreshes logs for a new running generation and releases loading', async () => {
    vi.mocked(getAppDevRuntimeStatus)
      .mockResolvedValueOnce({
        generation: 31,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      })
      .mockResolvedValueOnce({
        generation: 32,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
    const oldLogs =
      deferred<Awaited<ReturnType<typeof listAppDevRuntimeLogs>>>();
    const signals: AbortSignal[] = [];
    vi.mocked(listAppDevRuntimeLogs)
      .mockImplementationOnce(input => {
        signals.push(input.signal!);
        return oldLogs.promise;
      })
      .mockImplementationOnce(input => {
        signals.push(input.signal!);
        return Promise.resolve({
          items: [
            { id: 'new', level: 'info', message: 'gen32', timestamp: '' },
          ],
        });
      });
    await render();
    await act(async () => hook?.refreshStatus());
    await act(flush);

    expect(signals[0]?.aborted).toBe(true);
    expect(listAppDevRuntimeLogs).toHaveBeenCalledTimes(2);
    expect(hook?.logs.map(item => item.message)).toEqual(['gen32']);
    expect(hook?.logsLoading).toBe(false);
  });

  it('reuses one auto-start operation under StrictMode and aborts replayed work', async () => {
    const statusSignals: AbortSignal[] = [];
    vi.mocked(getAppDevRuntimeStatus).mockImplementation(input => {
      statusSignals.push(input.signal!);
      return Promise.resolve({
        generation: 0,
        status: 'stopped',
        canStart: true,
        recovering: false,
        stopping: false,
      });
    });
    vi.mocked(startAppDevRuntime).mockImplementation(
      () => new Promise(() => undefined),
    );

    await render(true);
    await act(flush);

    expect(
      new Set(
        vi
          .mocked(startAppDevRuntime)
          .mock.calls.map(call => call[0].operationId),
      ).size,
    ).toBeLessThanOrEqual(1);
    expect(statusSignals[0]?.aborted).toBe(true);
  });
});
