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

import {
  useEffect,
  useLayoutEffect,
  useRef,
  type Dispatch,
  type SetStateAction,
} from 'react';

import { canonicalThreadClient } from '../workbench/thread-client/canonical-thread-client-singleton';
import type { WorkbenchRunEvent } from '../workbench/thread-client';
import type { TaskThreadDetailEvent } from './task-thread-detail-model';
import {
  parseTaskTokenUsageSnapshotEvent,
  type TaskTokenUsageSnapshot,
} from './task-detail-token-usage';
import { mapTaskThreadRunEventToDetailEvent } from './task-detail-loader';

type TaskThreadRunEvent = WorkbenchRunEvent;

interface ThreadTitleUpdate {
  threadId: string;
  title: string;
}

const mergeTaskThreadDetailEvents = (
  current: TaskThreadDetailEvent[],
  incoming: TaskThreadDetailEvent[],
) => {
  const eventsByID = new Map<string, TaskThreadDetailEvent>();

  for (const event of [...current, ...incoming]) {
    eventsByID.set(event.id, event);
  }

  return Array.from(eventsByID.values()).sort((left, right) => {
    if (left.created_at !== right.created_at) {
      return left.created_at - right.created_at;
    }

    return left.id.localeCompare(right.id);
  });
};

const parseEventPayload = (payload?: string): Record<string, unknown> => {
  if (!payload?.trim()) {
    return {};
  }

  try {
    const parsed: unknown = JSON.parse(payload);

    return parsed && typeof parsed === 'object'
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
};

const getThreadTitleUpdate = (
  event: TaskThreadRunEvent,
): ThreadTitleUpdate | undefined => {
  if (event.event_type !== 'context.thread_title_updated') {
    return undefined;
  }

  const payload = parseEventPayload(event.payload);
  const title = String(payload.thread_title ?? payload.title ?? '').trim();
  if (!title) {
    return undefined;
  }

  return {
    threadId: String(event.thread_id ?? ''),
    title,
  };
};

const TERMINAL_RUN_EVENT_TYPES = new Set([
  'run.completed',
  'run.failed',
  'run.canceled',
  'run.cancelled',
  'run.interrupted',
]);

export const useTaskThreadRunEventStream = ({
  enabled,
  onTokenUsageSnapshot,
  onThreadTitleUpdated,
  runId,
  setEvents,
  spaceId,
  threadId,
}: {
  enabled: boolean;
  onTokenUsageSnapshot?: (
    snapshot: TaskTokenUsageSnapshot,
    event: TaskThreadRunEvent,
  ) => void;
  onThreadTitleUpdated?: (update: ThreadTitleUpdate) => void;
  runId?: string;
  setEvents: Dispatch<SetStateAction<TaskThreadDetailEvent[]>>;
  spaceId?: string;
  threadId?: string;
}) => {
  const generationRef = useRef(0);
  const streamScopeKey = JSON.stringify([
    enabled,
    spaceId ?? '',
    threadId ?? '',
    runId ?? '',
  ]);
  const committedScopeKeyRef = useRef(streamScopeKey);

  useLayoutEffect(() => {
    if (committedScopeKeyRef.current === streamScopeKey) {
      return;
    }

    committedScopeKeyRef.current = streamScopeKey;
    generationRef.current += 1;
  }, [streamScopeKey]);

  useEffect(() => {
    if (!enabled || !spaceId || !threadId || !runId) {
      return;
    }

    const generation = ++generationRef.current;
    const capturedScope = { generation, runId, spaceId, threadId };
    const controller = new AbortController();
    const subscriptionRef: {
      current?: ReturnType<typeof canonicalThreadClient.subscribeRunEvents>;
    } = {};
    let closed = false;
    let terminalEventSeen = false;
    const isCurrentScope = () =>
      !closed &&
      !controller.signal.aborted &&
      generationRef.current === capturedScope.generation;
    const close = () => {
      if (closed) {
        return;
      }

      closed = true;
      controller.abort();
      subscriptionRef.current?.close();
    };
    const handleRunEvent = (runEvent: TaskThreadRunEvent) => {
      if (
        !isCurrentScope() ||
        terminalEventSeen ||
        runEvent.thread_id !== capturedScope.threadId ||
        runEvent.run_id !== capturedScope.runId
      ) {
        return;
      }

      const titleUpdate = getThreadTitleUpdate(runEvent);
      if (titleUpdate) {
        onThreadTitleUpdated?.(titleUpdate);
      }

      const tokenUsageSnapshot = parseTaskTokenUsageSnapshotEvent(runEvent);
      if (tokenUsageSnapshot) {
        onTokenUsageSnapshot?.(tokenUsageSnapshot, runEvent);
      } else {
        setEvents(current =>
          mergeTaskThreadDetailEvents(current, [
            mapTaskThreadRunEventToDetailEvent(runEvent),
          ]),
        );
      }

      if (TERMINAL_RUN_EVENT_TYPES.has(runEvent.event_type)) {
        terminalEventSeen = true;
        close();
      }
    };
    subscriptionRef.current = canonicalThreadClient.subscribeRunEvents({
      space_id: capturedScope.spaceId,
      thread_id: capturedScope.threadId,
      run_id: capturedScope.runId,
      signal: controller.signal,
      onEvent: handleRunEvent,
      onEnd: () => {
        if (isCurrentScope()) {
          close();
        }
      },
      onError: () => {
        if (isCurrentScope()) {
          close();
        }
      },
    });
    if (closed) {
      subscriptionRef.current.close();
    }

    return close;
  }, [
    enabled,
    onThreadTitleUpdated,
    onTokenUsageSnapshot,
    runId,
    setEvents,
    spaceId,
    threadId,
  ]);
};
