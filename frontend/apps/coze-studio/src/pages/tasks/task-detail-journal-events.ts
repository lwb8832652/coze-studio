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

import type { workbenchTask } from '@coze-studio/api-schema';

type TaskEvent = workbenchTask.TaskEvent;
type TaskThreadRunEvent = workbenchTask.TaskThreadRunEvent;
type TaskThreadRunJournalMessage = workbenchTask.TaskThreadRunJournalMessage;

const parseJSONObject = (
  value?: string,
): Record<string, unknown> | undefined => {
  const trimmed = value?.trim();

  if (!trimmed) {
    return undefined;
  }

  try {
    const parsed: unknown = JSON.parse(trimmed);

    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return undefined;
  }

  return undefined;
};

const normalizeJSONString = (value?: string) =>
  JSON.stringify(parseJSONObject(value) ?? {});

export const mapTaskThreadRunEventToTaskEvent = (
  event: TaskThreadRunEvent,
): TaskEvent => ({
  id: event.event_id,
  task_id: event.thread_id,
  run_id: event.run_id,
  event_type: event.event_type,
  payload: event.payload,
  created_at: event.created_at,
});

const mapTaskThreadRunJournalMessageToTaskEvent = (
  message: TaskThreadRunJournalMessage,
): TaskEvent | undefined => {
  if (message.type === 'human') {
    return undefined;
  }

  if (message.type === 'tool') {
    return {
      id: message.source_event_id || `journal-${message.id}`,
      task_id: message.thread_id,
      run_id: message.run_id,
      event_type: 'tool.completed',
      payload: JSON.stringify({
        role: 'tool',
        tool_name: message.name,
        tool_call_id: message.tool_call_id,
        content: message.content,
      }),
      created_at: message.created_at,
    };
  }

  if (message.type !== 'ai') {
    return undefined;
  }

  const additionalKwargs = parseJSONObject(message.additional_kwargs);
  const payload: Record<string, unknown> = {
    ...additionalKwargs,
    role: 'assistant',
  };
  if (message.content) {
    payload.content = message.content;
  }
  if (message.tool_calls?.length) {
    payload.tool_calls = message.tool_calls.map(toolCall => ({
      id: toolCall.id,
      type: toolCall.type || 'function',
      function: {
        name: toolCall.name,
        arguments: normalizeJSONString(toolCall.arguments),
      },
    }));
  }

  return {
    id: message.source_event_id || `journal-${message.id}`,
    task_id: message.thread_id,
    run_id: message.run_id,
    event_type: 'message.completed',
    payload: JSON.stringify(payload),
    created_at: message.created_at,
  };
};

const isJournalBackedEventType = (eventType?: string) =>
  eventType === 'message.completed' || eventType?.startsWith('tool.');

export const mergeJournalTaskEvents = ({
  journalMessages,
  runEvents,
}: {
  journalMessages?: TaskThreadRunJournalMessage[];
  runEvents: TaskThreadRunEvent[];
}) => {
  const journalEvents = (journalMessages ?? [])
    .map(mapTaskThreadRunJournalMessageToTaskEvent)
    .filter((event): event is TaskEvent => Boolean(event));
  const baseEvents = runEvents
    .map(mapTaskThreadRunEventToTaskEvent)
    .filter(event =>
      journalEvents.length ? !isJournalBackedEventType(event.event_type) : true,
    );

  return [...baseEvents, ...journalEvents].sort((left, right) => {
    if (left.created_at !== right.created_at) {
      return left.created_at - right.created_at;
    }

    return String(left.id).localeCompare(String(right.id));
  });
};
