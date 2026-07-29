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
import type { FetchSteamConfig } from '@coze-arch/fetch-stream';

import type { WorkbenchRunEvent } from '../types';
import {
  createRunEventCursorStore,
  type RunEventCursorScope,
  type RunEventCursorStorage,
} from '../run-event-cursor';
import {
  CanonicalThreadCoreClient,
  type CanonicalFetchStream,
} from '../canonical-thread-client';
import { WorkbenchClientError } from '../canonical-fetch';
import { runEventTransportFixture } from './fixtures';

const spaceID = '9001';
const threadID = '1001';
const runID = '3001';
const scope: RunEventCursorScope = {
  contract: 'canonical_v1',
  spaceId: spaceID,
  threadId: threadID,
  runId: runID,
};

type StreamMessage = unknown;
type StreamConfig = FetchSteamConfig<StreamMessage>;

const streamHarness = () => {
  let config: StreamConfig | undefined;
  const stream = vi.fn(
    (_input: RequestInfo, nextConfig: StreamConfig): Promise<void> => {
      config = nextConfig;
      return Promise.resolve();
    },
  );
  return {
    stream: stream as unknown as CanonicalFetchStream,
    config: () => {
      if (!config) {
        throw new Error('Missing stream configuration');
      }
      return config;
    },
  };
};

const emitFrame = (
  config: StreamConfig,
  frame: { event: string; id?: string; data: string },
) => {
  const message = config.streamParser?.(frame as never, {
    terminate: vi.fn(),
    onParseError: vi.fn(),
  });
  if (message !== undefined) {
    config.onMessage?.({ message });
  }
};

const eventFrame = (
  eventID = runEventTransportFixture.visible.event_id,
  overrides: Record<string, unknown> = {},
) => ({
  event: 'events',
  id: eventID,
  data: JSON.stringify({
    ...runEventTransportFixture.canonical.data[0],
    event_id: eventID,
    ...overrides,
  }),
});

const createCallbacks = () => ({
  onEvent: vi.fn<(event: WorkbenchRunEvent) => void>(),
  onEnd: vi.fn<() => void>(),
  onError: vi.fn<(error: Error) => void>(),
});

describe('canonical Run event stream', () => {
  it('uses one scoped fetch stream, ignores metadata, and stores only a parsed event cursor', () => {
    const harness = streamHarness();
    const storage = new Map<string, string>();
    const cursorStore = createRunEventCursorStore({
      storage: null,
      memory: storage,
    });
    const callbacks = createCallbacks();
    const controller = new AbortController();
    const originalEventSource = globalThis.EventSource;
    const eventSource = vi.fn();
    Object.defineProperty(globalThis, 'EventSource', {
      configurable: true,
      value: eventSource,
    });

    try {
      const client = new CanonicalThreadCoreClient({
        stream: harness.stream,
        cursorStore,
      });
      client.subscribeRunEvents({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        cursor: '7',
        signal: controller.signal,
        ...callbacks,
      });

      expect(harness.stream).toHaveBeenCalledTimes(1);
      const [url, config] = vi.mocked(harness.stream).mock.calls[0];
      const expectedURL =
        `/api/workbench/threads/${threadID}/runs/${runID}/stream?` +
        'after_event_id=7&cancel_on_disconnect=false&stream_mode=events';
      expect(String(url)).toBe(expectedURL);
      expect(config).toMatchObject({
        method: 'GET',
        credentials: 'same-origin',
        headers: {
          Accept: 'text/event-stream',
          'X-Coze-Space-ID': spaceID,
          'x-requested-with': 'XMLHttpRequest',
        },
      });
      expect(config.signal).not.toBe(controller.signal);

      emitFrame(harness.config(), {
        event: 'metadata',
        data: JSON.stringify({
          run_id: runID,
          hidden: 'must not be projected or persisted',
        }),
      });
      expect(callbacks.onEvent).not.toHaveBeenCalled();
      expect(cursorStore.read(scope)).toBeUndefined();

      emitFrame(harness.config(), eventFrame('11'));
      expect(callbacks.onEvent).toHaveBeenCalledWith({
        ...runEventTransportFixture.visible,
        event_id: '11',
      });
      expect(cursorStore.read(scope)).toBe('11');
      expect(eventSource).not.toHaveBeenCalled();
      expect(String(url)).not.toContain('/api/workbench/task_threads');
      expect(String(url)).not.toMatch(/^\/api\/(threads|runs)(?:\/|$)/);
    } finally {
      Object.defineProperty(globalThis, 'EventSource', {
        configurable: true,
        value: originalEventSource,
      });
    }
  });

  it('reconnects with the greatest confirmed decimal cursor in a new subscription', () => {
    const storage = new Map<string, string>();
    const cursorStore = createRunEventCursorStore({
      storage: null,
      memory: storage,
    });
    cursorStore.write(scope, '900719925474099312345');
    const first = streamHarness();
    const second = streamHarness();
    const callbacks = createCallbacks();

    new CanonicalThreadCoreClient({
      stream: first.stream,
      cursorStore,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      cursor: '8',
      signal: new AbortController().signal,
      ...callbacks,
    });
    new CanonicalThreadCoreClient({
      stream: second.stream,
      cursorStore,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      cursor: '900719925474099312346',
      signal: new AbortController().signal,
      ...callbacks,
    });

    expect(String(vi.mocked(first.stream).mock.calls[0][0])).toContain(
      'after_event_id=900719925474099312345',
    );
    expect(String(vi.mocked(second.stream).mock.calls[0][0])).toContain(
      'after_event_id=900719925474099312346',
    );
    expect(first.stream).toHaveBeenCalledTimes(1);
    expect(second.stream).toHaveBeenCalledTimes(1);
  });

  it('does not advance the cursor when a public event is malformed', async () => {
    const harness = streamHarness();
    const cursorStore = createRunEventCursorStore({
      storage: null,
      memory: new Map(),
    });
    const callbacks = createCallbacks();
    const subscription = new CanonicalThreadCoreClient({
      stream: harness.stream,
      cursorStore,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      signal: new AbortController().signal,
      ...callbacks,
    });

    emitFrame(
      harness.config(),
      eventFrame('12', { thread_id: 'different-thread' }),
    );

    await subscription.closed;
    expect(cursorStore.read(scope)).toBeUndefined();
    expect(callbacks.onEvent).not.toHaveBeenCalled();
    expect(callbacks.onError).toHaveBeenCalledTimes(1);
    expect(callbacks.onError.mock.calls[0][0]).toMatchObject({
      name: 'WorkbenchClientError',
      code: 'invalid_response',
    });
    expect(harness.config().signal?.aborted).toBe(true);
  });

  it('keeps the confirmed cursor when a terminal status is invalid', async () => {
    const harness = streamHarness();
    const cursorStore = createRunEventCursorStore({
      storage: null,
      memory: new Map(),
    });
    cursorStore.write(scope, '16');
    const callbacks = createCallbacks();
    const subscription = new CanonicalThreadCoreClient({
      stream: harness.stream,
      cursorStore,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      signal: new AbortController().signal,
      ...callbacks,
    });

    emitFrame(harness.config(), {
      event: 'end',
      data: JSON.stringify({
        thread_id: threadID,
        run_id: runID,
        status: 'succeeded',
        reason: 'terminal',
      }),
    });

    await subscription.closed;
    expect(callbacks.onEnd).not.toHaveBeenCalled();
    expect(callbacks.onError).toHaveBeenCalledWith(
      expect.objectContaining({ code: 'invalid_response' }),
    );
    expect(cursorStore.read(scope)).toBe('16');
  });

  it('clears the cursor and ends exactly once on a terminal frame', async () => {
    const harness = streamHarness();
    const cursorStore = createRunEventCursorStore({
      storage: null,
      memory: new Map(),
    });
    cursorStore.write(scope, '17');
    const callbacks = createCallbacks();
    const subscription = new CanonicalThreadCoreClient({
      stream: harness.stream,
      cursorStore,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      signal: new AbortController().signal,
      ...callbacks,
    });

    emitFrame(harness.config(), {
      event: 'end',
      data: JSON.stringify({
        thread_id: threadID,
        run_id: runID,
        status: 'success',
        reason: 'terminal',
      }),
    });
    emitFrame(harness.config(), {
      event: 'end',
      data: JSON.stringify({ thread_id: threadID, run_id: runID }),
    });

    await subscription.closed;
    expect(callbacks.onEnd).toHaveBeenCalledTimes(1);
    expect(callbacks.onError).not.toHaveBeenCalled();
    expect(cursorStore.read(scope)).toBeUndefined();
    expect(harness.config().signal?.aborted).toBe(true);
  });

  it('projects a canonical HTTP failure and never opens a fallback stream', async () => {
    const harness = streamHarness();
    const callbacks = createCallbacks();
    const subscription = new CanonicalThreadCoreClient({
      stream: harness.stream,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      signal: new AbortController().signal,
      ...callbacks,
    });
    const response = new Response(
      JSON.stringify({
        detail: 'Run access denied',
        code: 'permission_denied',
        retryable: false,
        trace_id: 'trace-stream-403',
      }),
      {
        status: 403,
        headers: { 'content-type': 'application/json' },
      },
    );
    const error = await harness
      .config()
      .onStart?.(response)
      .then(
        () => undefined,
        reason => reason as unknown,
      );
    expect(error).toBeInstanceOf(WorkbenchClientError);
    harness.config().onError?.({
      fetchStreamError: {
        code: 10001,
        msg: 'must not be exposed',
        error,
      },
    });

    await subscription.closed;
    expect(callbacks.onError).toHaveBeenCalledWith(
      expect.objectContaining({
        code: 'permission_denied',
        status: 403,
        traceId: 'trace-stream-403',
      }),
    );
    expect(harness.stream).toHaveBeenCalledTimes(1);
    expect(harness.config().signal?.aborted).toBe(true);
  });

  it('rejects a response whose media type only prefixes text/event-stream', async () => {
    const harness = streamHarness();
    const callbacks = createCallbacks();
    const subscription = new CanonicalThreadCoreClient({
      stream: harness.stream,
    }).subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      signal: new AbortController().signal,
      ...callbacks,
    });

    const error = await harness
      .config()
      .onStart?.(
        new Response('invalid stream', {
          headers: { 'content-type': 'text/event-stream-invalid' },
        }),
      )
      .then(
        () => undefined,
        reason => reason as unknown,
      );

    expect(error).toMatchObject({ code: 'invalid_response' });
    subscription.close();
    await subscription.closed;
  });

  it.each(['external abort', 'explicit close'] as const)(
    'resolves closed without reporting an error on %s',
    async closeKind => {
      const harness = streamHarness();
      const callbacks = createCallbacks();
      const controller = new AbortController();
      const subscription = new CanonicalThreadCoreClient({
        stream: harness.stream,
      }).subscribeRunEvents({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        signal: controller.signal,
        ...callbacks,
      });

      if (closeKind === 'external abort') {
        controller.abort();
      } else {
        subscription.close();
      }

      await subscription.closed;
      expect(harness.config().signal?.aborted).toBe(true);
      expect(callbacks.onEnd).not.toHaveBeenCalled();
      expect(callbacks.onError).not.toHaveBeenCalled();
      expect(harness.stream).toHaveBeenCalledTimes(1);
    },
  );
});

describe('Run event cursor storage', () => {
  it('stores only positive decimal IDs and falls back when session storage throws', () => {
    const persisted = new Map<string, string>();
    const storage: RunEventCursorStorage = {
      getItem: key => persisted.get(key) ?? null,
      setItem: (key, value) => {
        persisted.set(key, value);
      },
      removeItem: key => {
        persisted.delete(key);
      },
    };
    const store = createRunEventCursorStore({ storage, memory: new Map() });

    store.write(scope, '0');
    store.write(scope, '-1');
    store.write(scope, '1e3');
    expect(persisted.size).toBe(0);
    store.write(scope, '21');
    expect(store.read(scope)).toBe('21');
    store.write(scope, '19');
    expect(store.read(scope)).toBe('21');
    const [persistedKey, persistedValue] = [...persisted.entries()][0];
    expect(persistedKey).toContain('canonical_v1');
    expect(persistedKey).toContain(`${spaceID}:${threadID}:${runID}`);
    expect(persistedValue).toBe('21');

    const throwingStorage: RunEventCursorStorage = {
      getItem: () => {
        throw new DOMException('blocked', 'SecurityError');
      },
      setItem: () => {
        throw new DOMException('blocked', 'SecurityError');
      },
      removeItem: () => {
        throw new DOMException('blocked', 'SecurityError');
      },
    };
    const fallback = createRunEventCursorStore({
      storage: throwingStorage,
      memory: new Map(),
    });
    fallback.write(scope, '22');
    expect(fallback.read(scope)).toBe('22');
    fallback.clear(scope);
    expect(fallback.read(scope)).toBeUndefined();

    const stalePersisted = new Map<string, string>();
    const staleStorage: RunEventCursorStorage = {
      getItem: key => stalePersisted.get(key) ?? null,
      setItem: (key, value) => {
        stalePersisted.set(key, value);
      },
      removeItem: () => {
        throw new DOMException('remove blocked', 'SecurityError');
      },
    };
    const staleStore = createRunEventCursorStore({
      storage: staleStorage,
      memory: new Map(),
    });
    staleStore.write(scope, '23');
    staleStore.clear(scope);
    expect(staleStore.read(scope)).toBeUndefined();
  });
});
