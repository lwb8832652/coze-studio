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

/* eslint-disable max-params -- Fetch test doubles preserve the browser callback signature. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  readAppDevPendingOperation,
  writeAppDevPendingOperation,
} from '../utils/provider-operation';
import {
  getAppDevRuntimeStatus,
  keepAliveAppDevRuntime,
  listAppDevRuntimeLogs,
  restartAppDevRuntime,
  startAppDevRuntime,
  stopAppDevRuntime,
} from '../service';
import { useAppDevRuntime } from '../hooks/use-app-dev-runtime';

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

const flushPromises = async () => {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
};

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => (resolve = next));
  return { promise, resolve };
};

describe('useAppDevRuntime provider lifecycle', () => {
  let root: Root | undefined;
  let runtime: ReturnType<typeof useAppDevRuntime> | undefined;
  const operationScope = {
    spaceId: 'space-1',
    projectId: 'project-1',
    principalId: 'user-1',
  };

  const render = async (spaceId = 'space-1', projectId = 'project-1') => {
    const Harness = () => {
      runtime = useAppDevRuntime(spaceId, projectId, 'user-1');
      return null;
    };
    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness />);
      await flushPromises();
    });
  };

  beforeEach(() => {
    vi.useFakeTimers();
    sessionStorage.clear();
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible');
    vi.mocked(listAppDevRuntimeLogs).mockResolvedValue({ items: [] });
  });

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
        await flushPromises();
      });
      root = undefined;
    }
    runtime = undefined;
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.clearAllMocks();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('makes the auto-start decision once and explicit stop suppresses it', async () => {
    vi.mocked(getAppDevRuntimeStatus)
      .mockResolvedValueOnce({
        generation: 0,
        status: 'stopped',
        canStart: true,
        recovering: false,
        stopping: false,
      })
      .mockResolvedValue({
        generation: 1,
        status: 'stopped',
        canStart: true,
        recovering: false,
        stopping: false,
      });
    vi.mocked(startAppDevRuntime).mockResolvedValue({
      generation: 1,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    vi.mocked(stopAppDevRuntime).mockResolvedValue({
      generation: 1,
      status: 'stopped',
      canStart: true,
      recovering: false,
      stopping: false,
    });
    await render();
    await act(flushPromises);
    expect(startAppDevRuntime).toHaveBeenCalledTimes(1);

    await act(async () => runtime?.stop());
    await act(flushPromises);
    expect(runtime?.runtime.status).toBe('stopped');
    expect(startAppDevRuntime).toHaveBeenCalledTimes(1);
    expect(
      readAppDevPendingOperation(operationScope, 'runtime-auto-start'),
    ).toBeUndefined();

    await act(async () => runtime?.refreshStatus());
    expect(startAppDevRuntime).toHaveBeenCalledTimes(1);
  });

  it('recovers a persisted manual start with the same operation after reload', async () => {
    writeAppDevPendingOperation(operationScope, 'runtime-manual-start', {
      operationId: 'manual-start-operation',
      phase: 'requesting',
    });
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 0,
      status: 'stopped',
      canStart: true,
      recovering: false,
      stopping: false,
    });
    vi.mocked(startAppDevRuntime).mockResolvedValue({
      generation: 1,
      status: 'starting',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    await render();
    await act(flushPromises);
    expect(startAppDevRuntime).toHaveBeenCalledWith(
      expect.objectContaining({ operationId: 'manual-start-operation' }),
    );
  });

  it('invalidates an older same-project status before committing stop', async () => {
    let resolveOld!: (
      value: Awaited<ReturnType<typeof getAppDevRuntimeStatus>>,
    ) => void;
    const oldStatus = new Promise<
      Awaited<ReturnType<typeof getAppDevRuntimeStatus>>
    >(resolve => (resolveOld = resolve));
    vi.mocked(getAppDevRuntimeStatus)
      .mockResolvedValueOnce({
        generation: 4,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      })
      .mockImplementationOnce(() => oldStatus)
      .mockResolvedValue({
        generation: 4,
        status: 'stopping',
        canStart: false,
        recovering: false,
        stopping: true,
      });
    vi.mocked(stopAppDevRuntime).mockResolvedValue({
      generation: 4,
      status: 'stopping',
      canStart: false,
      recovering: false,
      stopping: true,
    });
    await render();
    act(() => void runtime?.refreshStatus());
    await act(flushPromises);
    await act(async () => runtime?.stop());
    await act(async () => {
      resolveOld({
        generation: 3,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
      await flushPromises();
    });
    expect(runtime?.runtime.status).toBe('stopping');
    expect(runtime?.runtime.generation).toBe(4);
  });

  it('does not let a late keepalive overwrite a stop transition', async () => {
    let resolveKeepAlive!: (
      value: Awaited<ReturnType<typeof keepAliveAppDevRuntime>>,
    ) => void;
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 6,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    vi.mocked(keepAliveAppDevRuntime).mockImplementation(
      () => new Promise(resolve => (resolveKeepAlive = resolve)),
    );
    vi.mocked(stopAppDevRuntime).mockResolvedValue({
      generation: 6,
      status: 'stopping',
      canStart: false,
      recovering: false,
      stopping: true,
    });
    await render();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
      await flushPromises();
    });
    expect(keepAliveAppDevRuntime).toHaveBeenCalledTimes(1);
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 6,
      status: 'stopping',
      canStart: false,
      recovering: false,
      stopping: true,
    });
    await act(async () => runtime?.stop());
    await act(async () => {
      resolveKeepAlive({
        generation: 6,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
      await flushPromises();
    });
    expect(runtime?.runtime.status).toBe('stopping');
  });

  it('rejects a lower-generation mutation before journal or terminal side effects', async () => {
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 8,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    vi.mocked(stopAppDevRuntime).mockResolvedValue({
      generation: 7,
      status: 'stopped',
      canStart: true,
      recovering: false,
      stopping: false,
    });
    await render();
    await act(async () => runtime?.stop());
    const pending = readAppDevPendingOperation(operationScope, 'runtime-stop');
    expect(runtime?.runtime.generation).toBe(8);
    expect(runtime?.runtime.status).toBe('stopping');
    expect(pending).toMatchObject({ phase: 'requesting' });
    expect(pending?.generation).toBeUndefined();
    expect(getAppDevRuntimeStatus).toHaveBeenCalledTimes(1);
  });

  it('rejects a lower-generation keepalive without updating its timestamp', async () => {
    let resolveKeepAlive!: (
      value: Awaited<ReturnType<typeof keepAliveAppDevRuntime>>,
    ) => void;
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 9,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    vi.mocked(keepAliveAppDevRuntime).mockImplementation(
      () => new Promise(resolve => (resolveKeepAlive = resolve)),
    );
    await render();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
      await flushPromises();
    });
    expect(keepAliveAppDevRuntime).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolveKeepAlive({
        generation: 8,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
      await flushPromises();
    });
    expect(runtime?.runtime.generation).toBe(9);
    expect(runtime?.runtime.lastKeepAliveAt).toBeUndefined();
  });

  it.each([
    [
      'runtime-stop',
      'persisted-stop-operation',
      stopAppDevRuntime,
      { status: 'stopping', stopping: true },
      { status: 'stopped', stopping: false, canStart: true },
    ],
    [
      'runtime-restart',
      'persisted-restart-operation',
      restartAppDevRuntime,
      { status: 'starting', stopping: false },
      { status: 'running', stopping: false, canStart: false },
    ],
  ] as const)(
    'recovers persisted %s with the same key and clears it at authority terminal',
    async (intent, operationId, mutation, mutationState, terminalState) => {
      writeAppDevPendingOperation(operationScope, intent, {
        operationId,
        phase: 'requesting',
      });
      vi.mocked(getAppDevRuntimeStatus)
        .mockResolvedValueOnce({
          generation: 10,
          status: 'running',
          canStart: false,
          recovering: false,
          stopping: false,
        })
        .mockResolvedValue({
          generation: 10,
          recovering: false,
          ...terminalState,
        });
      vi.mocked(mutation).mockResolvedValue({
        generation: 10,
        canStart: false,
        recovering: false,
        ...mutationState,
      });
      await render();
      await act(flushPromises);
      expect(mutation).toHaveBeenCalledWith(
        expect.objectContaining({ operationId }),
      );
      expect(
        readAppDevPendingOperation(operationScope, intent),
      ).toBeUndefined();
    },
  );

  it('reuses the persisted stop key after a retryable failure', async () => {
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 11,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    vi.mocked(stopAppDevRuntime)
      .mockRejectedValueOnce(new Error('raw network failure'))
      .mockResolvedValue({
        generation: 11,
        status: 'stopping',
        canStart: false,
        recovering: false,
        stopping: true,
      });
    await render();
    await act(async () => runtime?.stop());
    const { operationId } = vi.mocked(stopAppDevRuntime).mock.calls[0][0];
    await act(async () => runtime?.stop());
    expect(vi.mocked(stopAppDevRuntime).mock.calls[1][0].operationId).toBe(
      operationId,
    );
  });

  it('aborts and rejects logs that return after a runtime mutation', async () => {
    const oldLogs =
      deferred<Awaited<ReturnType<typeof listAppDevRuntimeLogs>>>();
    vi.mocked(getAppDevRuntimeStatus)
      .mockResolvedValueOnce({
        generation: 12,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      })
      .mockResolvedValue({
        generation: 12,
        status: 'stopping',
        canStart: false,
        recovering: false,
        stopping: true,
      });
    vi.mocked(listAppDevRuntimeLogs)
      .mockImplementationOnce(() => oldLogs.promise)
      .mockResolvedValue({
        items: [
          { id: 'new', level: 'info', message: 'new logs', timestamp: '' },
        ],
      });
    vi.mocked(stopAppDevRuntime).mockResolvedValue({
      generation: 12,
      status: 'stopping',
      canStart: false,
      recovering: false,
      stopping: true,
    });
    await render();
    await act(async () => runtime?.stop());
    await act(async () => {
      oldLogs.resolve({
        items: [
          { id: 'old', level: 'error', message: 'old logs', timestamp: '' },
        ],
      });
      await flushPromises();
    });
    expect(runtime?.logs.map(log => log.message)).toEqual(['new logs']);
    expect(runtime?.error).not.toContain('old logs');
  });

  it('rejects logs captured before the authoritative generation changes', async () => {
    const oldLogs =
      deferred<Awaited<ReturnType<typeof listAppDevRuntimeLogs>>>();
    const newLogs =
      deferred<Awaited<ReturnType<typeof listAppDevRuntimeLogs>>>();
    vi.mocked(getAppDevRuntimeStatus)
      .mockResolvedValueOnce({
        generation: 13,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      })
      .mockResolvedValue({
        generation: 14,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
    vi.mocked(listAppDevRuntimeLogs)
      .mockImplementationOnce(() => oldLogs.promise)
      .mockImplementationOnce(() => newLogs.promise);
    await render();
    await act(async () => {
      await runtime?.refreshStatus();
      await flushPromises();
    });
    expect(listAppDevRuntimeLogs).toHaveBeenCalledTimes(2);
    await act(async () => {
      oldLogs.resolve({
        items: [
          {
            id: 'old',
            level: 'error',
            message: 'generation 13',
            timestamp: '',
          },
        ],
      });
      await flushPromises();
    });
    expect(runtime?.runtime.generation).toBe(14);
    expect(runtime?.logs).toEqual([]);
    expect(runtime?.logsLoading).toBe(true);
    await act(async () => {
      newLogs.resolve({
        items: [
          { id: 'new', level: 'info', message: 'generation 14', timestamp: '' },
        ],
      });
      await flushPromises();
    });
    expect(runtime?.logs.map(item => item.message)).toEqual(['generation 14']);
    expect(runtime?.logsLoading).toBe(false);
  });

  it('only auto-starts after an authoritative can_start=true status', async () => {
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 1,
      status: 'stopped',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    await render();
    expect(startAppDevRuntime).not.toHaveBeenCalled();

    act(() => root?.unmount());
    root = undefined;
    vi.mocked(getAppDevRuntimeStatus).mockRejectedValueOnce(
      new Error('network raw body'),
    );
    await render('space-1', 'project-2');
    expect(startAppDevRuntime).not.toHaveBeenCalled();
  });

  it('reuses the persisted auto-start operation after a short refresh', async () => {
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 2,
      status: 'stopped',
      canStart: true,
      recovering: false,
      stopping: false,
    });
    vi.mocked(startAppDevRuntime).mockRejectedValue(new Error('network'));

    await render();
    await act(flushPromises);
    expect(startAppDevRuntime).toHaveBeenCalledTimes(1);
    const firstOperation =
      vi.mocked(startAppDevRuntime).mock.calls[0][0].operationId;

    act(() => root?.unmount());
    root = undefined;
    await render();
    await act(flushPromises);
    expect(startAppDevRuntime).toHaveBeenCalledTimes(2);
    expect(vi.mocked(startAppDevRuntime).mock.calls[1][0].operationId).toBe(
      firstOperation,
    );
  });

  it('deduplicates repeated stop clicks while the same intent is in flight', async () => {
    vi.mocked(getAppDevRuntimeStatus).mockResolvedValue({
      generation: 3,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    let resolveStop!: (
      value: Awaited<ReturnType<typeof stopAppDevRuntime>>,
    ) => void;
    vi.mocked(stopAppDevRuntime).mockImplementation(
      () => new Promise(resolve => (resolveStop = resolve)),
    );
    await render();

    let first!: Promise<void>;
    let second!: Promise<void>;
    act(() => {
      first = runtime?.stop() as Promise<void>;
      second = runtime?.stop() as Promise<void>;
    });
    expect(stopAppDevRuntime).toHaveBeenCalledTimes(1);
    expect(second).toBe(first);
    await act(async () => {
      resolveStop({
        generation: 3,
        status: 'stopping',
        canStart: false,
        recovering: false,
        stopping: true,
      });
      await first;
    });
  });
});
