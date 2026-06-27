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

import { getTaskEventDisplay } from './helpers';

type TaskEvent = workbenchTask.TaskEvent;

const getPlanTaskId = (event: TaskEvent) => {
  if (!event.event_type?.startsWith('plan.task.')) {
    return undefined;
  }

  try {
    const payload: unknown = JSON.parse(event.payload ?? '');

    if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
      const planTaskId = (payload as Record<string, unknown>).plan_task_id;

      if (typeof planTaskId === 'string' && planTaskId.trim()) {
        return planTaskId.trim();
      }
    }
  } catch (error) {
    if (error instanceof SyntaxError) {
      return undefined;
    }

    throw error;
  }

  return undefined;
};

export const projectTaskExecutionEvents = (events: TaskEvent[]) => {
  const latestPlanEventIndex = new Map<string, number>();

  events.forEach((event, index) => {
    const planTaskId = getPlanTaskId(event);

    if (planTaskId) {
      latestPlanEventIndex.set(planTaskId, index);
    }
  });

  return events
    .filter((event, index) => {
      const planTaskId = getPlanTaskId(event);

      return !planTaskId || latestPlanEventIndex.get(planTaskId) === index;
    })
    .map(event => ({
      event,
      display: getTaskEventDisplay(event.event_type, event.payload),
    }));
};
