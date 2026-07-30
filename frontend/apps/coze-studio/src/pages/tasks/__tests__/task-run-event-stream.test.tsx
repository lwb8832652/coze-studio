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

import { useLayoutEffect, useState } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import type { TaskThreadDetailEvent } from '../task-thread-detail-model';
import { useTaskThreadRunEventStream } from '../task-run-event-stream';
import type { WorkbenchRunEvent } from '../../workbench/thread-client';

const mockSubscribeRunEvents = vi.hoisted(() => vi.fn());

vi.mock(
  '../../workbench/thread-client/canonical-thread-client-singleton',
  () => ({
    canonicalThreadClient: {
      subscribeRunEvents: mockSubscribeRunEvents,
    },
  }),
);

Reflect.set(globalThis, 'IS_REACT_ACT_ENVIRONMENT', true);

interface StreamRequest {
  space_id: string;
  thread_id: string;
  run_id: string;
  signal: AbortSignal;
  onEvent: (event: WorkbenchRunEvent) => void;
  onEnd: () => void;
  onError: (error: Error) => void;
}

interface CapturedSource {
  close: ReturnType<typeof vi.fn>;
  open: boolean;
  request: StreamRequest;
}

const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];
let capturedSources: CapturedSource[] = [];
let maxOpenSources = 0;
let renderedEvents: TaskThreadDetailEvent[] = [];

const runEvent = ({
  eventId,
  eventType = 'step.completed',
  runId,
  threadId,
}: {
  eventId: string;
  eventType?: string;
  runId: string;
  threadId: string;
}): WorkbenchRunEvent => ({
  event_id: eventId,
  thread_id: threadId,
  run_id: runId,
  event_type: eventType,
  payload: '{}',
  created_at: 1,
});

const StreamHarness = ({
  enabled = true,
  onLayoutCommit,
  runId,
  spaceId,
  threadId,
}: {
  enabled?: boolean;
  onLayoutCommit?: () => void;
  runId: string;
  spaceId: string;
  threadId: string;
}) => {
  const [events, setEvents] = useState<TaskThreadDetailEvent[]>([]);
  renderedEvents = events;
  useTaskThreadRunEventStream({
    enabled,
    runId,
    setEvents,
    spaceId,
    threadId,
  });
  useLayoutEffect(() => {
    onLayoutCommit?.();
  }, [onLayoutCommit]);

  return null;
};

const renderHarness = ({
  runId = 'run-1',
  spaceId = 'space-1',
  threadId = 'thread-1',
}: {
  runId?: string;
  spaceId?: string;
  threadId?: string;
} = {}) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => {
    root.render(
      <StreamHarness runId={runId} spaceId={spaceId} threadId={threadId} />,
    );
  });

  return {
    root,
    rerender: (scope: {
      onLayoutCommit?: () => void;
      runId: string;
      spaceId: string;
      threadId: string;
    }) =>
      act(() => {
        root.render(<StreamHarness {...scope} />);
      }),
  };
};

beforeEach(() => {
  vi.clearAllMocks();
  capturedSources = [];
  maxOpenSources = 0;
  renderedEvents = [];
  mockSubscribeRunEvents.mockImplementation((request: StreamRequest) => {
    const source: CapturedSource = {
      close: vi.fn(() => {
        source.open = false;
      }),
      open: true,
      request,
    };
    capturedSources.push(source);
    maxOpenSources = Math.max(
      maxOpenSources,
      capturedSources.filter(item => item.open).length,
    );

    return {
      close: source.close,
      closed: Promise.resolve(),
    };
  });
});

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => root.unmount());
    container.remove();
  });
});

describe('useTaskThreadRunEventStream canonical lifecycle', () => {
  it('subscribes the committed active Run with explicit workspace scope', () => {
    renderHarness();

    expect(mockSubscribeRunEvents).toHaveBeenCalledTimes(1);
    expect(capturedSources[0]?.request).toMatchObject({
      run_id: 'run-1',
      space_id: 'space-1',
      thread_id: 'thread-1',
    });
    expect(capturedSources[0]?.request.signal.aborted).toBe(false);
    expect(maxOpenSources).toBe(1);
  });

  it('closes on the first terminal event and ignores duplicate terminal callbacks', () => {
    renderHarness();
    const source = capturedSources[0];

    act(() => {
      source.request.onEvent(
        runEvent({
          eventId: 'event-terminal',
          eventType: 'run.completed',
          runId: 'run-1',
          threadId: 'thread-1',
        }),
      );
    });

    expect(source.close).toHaveBeenCalledTimes(1);
    expect(source.request.signal.aborted).toBe(true);
    expect(renderedEvents.map(event => event.id)).toEqual(['event-terminal']);

    act(() => {
      source.request.onEvent(
        runEvent({
          eventId: 'event-terminal-duplicate',
          eventType: 'run.completed',
          runId: 'run-1',
          threadId: 'thread-1',
        }),
      );
    });

    expect(source.close).toHaveBeenCalledTimes(1);
    expect(source.request.signal.aborted).toBe(true);
    expect(renderedEvents.map(event => event.id)).toEqual(['event-terminal']);

    act(() => source.request.onEnd());

    expect(source.close).toHaveBeenCalledTimes(1);
    expect(source.request.signal.aborted).toBe(true);
    expect(renderedEvents.map(event => event.id)).toEqual(['event-terminal']);
  });

  it('closes the old source before space, Thread, or Run scope changes', () => {
    const { rerender } = renderHarness();
    rerender({ runId: 'run-2', spaceId: 'space-1', threadId: 'thread-1' });
    expect(capturedSources[0]?.close).toHaveBeenCalledTimes(1);
    rerender({ runId: 'run-3', spaceId: 'space-1', threadId: 'thread-2' });
    expect(capturedSources[1]?.close).toHaveBeenCalledTimes(1);
    rerender({ runId: 'run-4', spaceId: 'space-2', threadId: 'thread-2' });
    expect(capturedSources[2]?.close).toHaveBeenCalledTimes(1);

    expect(capturedSources).toHaveLength(4);
    expect(capturedSources.filter(source => source.open)).toHaveLength(1);
    expect(maxOpenSources).toBe(1);
  });

  it('ignores late event, end, and error callbacks from an old Run', () => {
    const { rerender } = renderHarness();
    const oldSource = capturedSources[0];
    rerender({ runId: 'run-2', spaceId: 'space-1', threadId: 'thread-1' });
    const currentSource = capturedSources[1];

    act(() => {
      oldSource.request.onEvent(
        runEvent({
          eventId: 'event-late',
          runId: 'run-1',
          threadId: 'thread-1',
        }),
      );
      oldSource.request.onError(new Error('late source failed'));
      oldSource.request.onEnd();
    });

    expect(renderedEvents).toEqual([]);
    expect(currentSource.close).not.toHaveBeenCalled();
    expect(currentSource.request.signal.aborted).toBe(false);
  });

  it('invalidates the old Run before layout callbacks can publish stale events', () => {
    const { rerender } = renderHarness();
    const oldSource = capturedSources[0];

    rerender({
      runId: 'run-2',
      spaceId: 'space-1',
      threadId: 'thread-1',
      onLayoutCommit: () => {
        oldSource.request.onEvent(
          runEvent({
            eventId: 'event-during-layout',
            runId: 'run-1',
            threadId: 'thread-1',
          }),
        );
      },
    });

    expect(renderedEvents).toEqual([]);
    expect(capturedSources).toHaveLength(2);
  });

  it('closes the only source on canonical onEnd and unmount', () => {
    const { root } = renderHarness();
    const source = capturedSources[0];

    act(() => source.request.onEnd());
    expect(source.close).toHaveBeenCalledTimes(1);

    act(() => root.unmount());
    mountedRoots.pop();
    expect(source.close).toHaveBeenCalledTimes(1);
    expect(capturedSources.filter(item => item.open)).toEqual([]);
  });
});
