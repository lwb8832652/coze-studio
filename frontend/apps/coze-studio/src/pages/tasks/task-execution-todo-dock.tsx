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

import { useState } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import {
  IconCozArrowDown,
  IconCozListDisorder,
} from '@coze-arch/coze-design/icons';

import type {
  TaskThreadDetailEvent,
  TaskThreadDetailModel,
} from './task-thread-detail-model';
import { projectTaskExecutionEvents } from './task-event-projection';
import { isTaskTerminalStatus } from './helpers';

type TaskThreadTodo = workbenchTask.TaskThreadTodo;

const parseTaskThreadEventPayload = (
  payload?: string,
): Record<string, unknown> => {
  try {
    const parsed: unknown = JSON.parse(payload || '{}');

    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
};

const isTodoExecutionEvent = (event: TaskThreadDetailEvent) => {
  const eventType = String(event.event_type ?? '').toLowerCase();
  const payload = parseTaskThreadEventPayload(event.payload);
  const toolName =
    typeof payload.tool_name === 'string' ? payload.tool_name : undefined;
  const title = typeof payload.title === 'string' ? payload.title : undefined;

  return (
    eventType.includes('todo') ||
    toolName === 'write_todos' ||
    Boolean(title?.toLowerCase().includes('to-do'))
  );
};

export const TaskExecutionTodoDock = ({
  events,
  task,
  todos,
}: {
  events: TaskThreadDetailEvent[];
  task: TaskThreadDetailModel;
  todos?: TaskThreadTodo[];
}) => {
  const [collapsed, setCollapsed] = useState(true);
  const eventItems = projectTaskExecutionEvents(events);
  const hasStructuredItems = eventItems.some(item => item.display.structured);
  const persistedTodoItems = (todos ?? [])
    .map((todo, index) => ({
      id: todo.id || `todo-${index + 1}`,
      status: todo.status || 'pending',
      title: todo.title,
    }))
    .filter(todo => todo.title);
  const eventTodoItems = (
    hasStructuredItems
      ? eventItems.filter(item => item.display.structured)
      : eventItems
  )
    .filter(item => isTodoExecutionEvent(item.event))
    .map(item => {
      const taskIsTerminal = isTaskTerminalStatus(task.status);

      return {
        id: item.event.id,
        status:
          taskIsTerminal && item.display.status === 'running'
            ? 'completed'
            : item.display.status,
        title: item.display.title,
      };
    });
  const todoItems = persistedTodoItems.length
    ? persistedTodoItems
    : eventTodoItems;

  if (!todoItems.length) {
    return null;
  }

  return (
    <section
      aria-label="任务 To-dos"
      className="coze-prototype-todo-dock"
      data-collapsed={collapsed}
      data-testid="task-todo-dock"
    >
      <button
        type="button"
        className="coze-prototype-todo-dock-header"
        aria-expanded={!collapsed}
        onClick={() => setCollapsed(value => !value)}
      >
        <span className="coze-prototype-todo-dock-title">
          <IconCozListDisorder aria-hidden="true" />
          <span>To-dos</span>
        </span>
        <span
          className="coze-prototype-todo-dock-chevron"
          data-collapsed={collapsed}
          aria-hidden="true"
        >
          <IconCozArrowDown />
        </span>
      </button>
      <div className="coze-prototype-todo-dock-body">
        <ol className="coze-prototype-todo-dock-list">
          {todoItems.map(item => (
            <li
              key={item.id}
              className="coze-prototype-todo-dock-item"
              data-status={item.status}
            >
              <span className="coze-prototype-todo-dock-indicator" />
              <span>{item.title}</span>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
};
