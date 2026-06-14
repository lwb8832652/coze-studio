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

import { useEffect, type Dispatch, type SetStateAction } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';

import { mapTaskThreadRunEventToTaskEvent } from './task-detail-loader';
import { getTaskThreadRunEventsStreamURL } from './service';

type TaskEvent = workbenchTask.TaskEvent;
type TaskThreadRunEvent = workbenchTask.TaskThreadRunEvent;

const mergeTaskEvents = (current: TaskEvent[], incoming: TaskEvent[]) => {
  const eventsByID = new Map<string, TaskEvent>();

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

const parseTaskThreadRunEvent = (
  event: MessageEvent,
): TaskThreadRunEvent | undefined => {
  try {
    const parsed: unknown = JSON.parse(event.data);

    if (
      parsed &&
      typeof parsed === 'object' &&
      'event_id' in parsed &&
      'thread_id' in parsed &&
      'run_id' in parsed &&
      'event_type' in parsed
    ) {
      return parsed as TaskThreadRunEvent;
    }
  } catch (error) {
    console.warn('Failed to parse task thread run event stream payload', error);
  }

  return undefined;
};

export const useTaskThreadRunEventStream = ({
  enabled,
  setEvents,
  threadId,
}: {
  enabled: boolean;
  setEvents: Dispatch<SetStateAction<TaskEvent[]>>;
  threadId?: string;
}) => {
  useEffect(() => {
    if (!enabled || !threadId || typeof EventSource === 'undefined') {
      return;
    }

    const eventSource = new EventSource(
      getTaskThreadRunEventsStreamURL({ threadId }),
    );
    const handleRunEvent = (event: MessageEvent) => {
      const runEvent = parseTaskThreadRunEvent(event);
      if (!runEvent) {
        return;
      }

      setEvents(current =>
        mergeTaskEvents(current, [mapTaskThreadRunEventToTaskEvent(runEvent)]),
      );
    };
    const handleDone = () => {
      eventSource.close();
    };

    eventSource.addEventListener('run.event', handleRunEvent);
    eventSource.addEventListener('done', handleDone);

    return () => {
      eventSource.removeEventListener('run.event', handleRunEvent);
      eventSource.removeEventListener('done', handleDone);
      eventSource.close();
    };
  }, [enabled, setEvents, threadId]);
};
