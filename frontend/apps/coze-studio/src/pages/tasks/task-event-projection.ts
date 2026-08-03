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

import type { TaskThreadDetailEvent } from './task-thread-detail-model';
import {
  getTaskThreadEventDisplay,
  type TaskThreadEventDisplay,
} from './helpers';

interface ProjectedTaskExecutionEvent {
  event: TaskThreadDetailEvent;
  display: TaskThreadEventDisplay;
}

interface ParsedToolCall {
  id?: string;
  name: string;
  args: Record<string, unknown>;
}

interface TaskExecutionProjectionContext {
  fallbackSkillName?: string;
}

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

const getString = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
};

const getObject = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
};

const parseToolArguments = (value: unknown): Record<string, unknown> => {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }

  if (typeof value !== 'string') {
    return {};
  }

  return parseJSONObject(value) ?? {};
};

const getFirstString = (
  payload: Record<string, unknown> | undefined,
  keys: string[],
) => {
  for (const key of keys) {
    const value = getString(payload, key);

    if (value) {
      return value;
    }
  }

  return undefined;
};

const getStringArray = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return Array.isArray(value)
    ? value
        .map(item => (typeof item === 'string' ? item.trim() : ''))
        .filter(Boolean)
    : [];
};

const getToolPath = (args: Record<string, unknown>) =>
  getFirstString(args, [
    'path',
    'file_path',
    'filepath',
    'virtual_path',
    'output_path',
  ]);

const getToolQuery = (args: Record<string, unknown>) =>
  getFirstString(args, ['query', 'search_query', 'q', 'keyword']);

const getSkillName = (args: Record<string, unknown>) =>
  getFirstString(args, ['skill', 'skill_name']);

const getLoadedSkillFallbackName = (events: TaskThreadDetailEvent[]) => {
  const names = new Set<string>();

  events.forEach(event => {
    if (event.event_type !== 'skills.loaded') {
      return;
    }

    const parsed = parseJSONObject(event.payload);
    const skillNames = getStringArray(parsed, 'skill_names');
    const singleSkillName = getString(parsed, 'skill_name');

    [...skillNames, singleSkillName].forEach(name => {
      if (name) {
        names.add(name);
      }
    });
  });

  return names.size === 1 ? [...names][0] : undefined;
};

const getToolCalls = (
  parsed: Record<string, unknown> | undefined,
): ParsedToolCall[] => {
  const rawToolCalls = parsed?.tool_calls;

  if (!Array.isArray(rawToolCalls)) {
    return [];
  }

  return rawToolCalls
    .map((rawToolCall): ParsedToolCall | undefined => {
      if (
        !rawToolCall ||
        typeof rawToolCall !== 'object' ||
        Array.isArray(rawToolCall)
      ) {
        return undefined;
      }

      const toolCall = rawToolCall as Record<string, unknown>;
      const functionCall = getObject(toolCall, 'function');
      const name =
        getString(toolCall, 'name') ?? getString(functionCall, 'name');

      if (!name) {
        return undefined;
      }

      return {
        id: getString(toolCall, 'id'),
        name,
        args: parseToolArguments(
          toolCall.args ?? toolCall.arguments ?? functionCall?.arguments,
        ),
      };
    })
    .filter((item): item is ParsedToolCall => Boolean(item));
};

const getToolResultByCallID = (events: TaskThreadDetailEvent[]) => {
  const resultByCallID = new Map<string, TaskThreadDetailEvent>();
  const terminalEventTypes = new Set([
    'tool.completed',
    'tool.succeeded',
    'tool.failed',
    'tool.canceled',
    'tool.cancelled',
    'tool.timed_out',
    'tool.result',
  ]);

  events.forEach(event => {
    if (!event.event_type || !terminalEventTypes.has(event.event_type)) {
      return;
    }

    const parsed = parseJSONObject(event.payload);
    const toolCallID = getString(parsed, 'tool_call_id');

    if (toolCallID) {
      resultByCallID.set(toolCallID, event);
    }
  });

  return resultByCallID;
};

const createToolCallProjection = ({
  context,
  event,
  index,
  resultByCallID,
  toolCall,
}: {
  context: TaskExecutionProjectionContext;
  event: TaskThreadDetailEvent;
  index: number;
  resultByCallID: Map<string, TaskThreadDetailEvent>;
  toolCall: ParsedToolCall;
}): ProjectedTaskExecutionEvent => {
  const resultEvent = toolCall.id ? resultByCallID.get(toolCall.id) : undefined;
  const resultPayload = parseJSONObject(resultEvent?.payload);
  const failed = Boolean(
    resultEvent?.event_type &&
      [
        'tool.failed',
        'tool.canceled',
        'tool.cancelled',
        'tool.timed_out',
      ].includes(resultEvent.event_type),
  );
  const terminalFailureDetail =
    resultEvent?.event_type === 'tool.timed_out'
      ? '工具调用超时'
      : resultEvent?.event_type === 'tool.canceled' ||
          resultEvent?.event_type === 'tool.cancelled'
        ? '工具调用已取消'
        : undefined;
  const completed = Boolean(resultEvent && !failed);
  const title = getString(toolCall.args, 'description');
  const detail = getToolPath(toolCall.args);
  const query =
    toolCall.name === 'web_search' ? getToolQuery(toolCall.args) : undefined;
  const skillName =
    toolCall.name === 'skill'
      ? (getSkillName(toolCall.args) ??
        getSkillName(resultPayload ?? {}) ??
        context.fallbackSkillName)
      : undefined;
  const payload = JSON.stringify({
    tool_name: toolCall.name,
    tool_call_id: toolCall.id,
    skill_name: skillName,
    query,
    title,
    detail,
    error_message: terminalFailureDetail,
    status: failed ? 'failed' : completed ? 'completed' : 'running',
    runtime: 'Agent',
  });
  const projectedEvent: TaskThreadDetailEvent = {
    ...event,
    id: `${event.id}:tool-call-${index}`,
    event_type: failed
      ? 'tool.failed'
      : completed
        ? 'tool.completed'
        : 'tool.started',
    payload,
  };

  return {
    event: projectedEvent,
    display: getTaskThreadEventDisplay(
      projectedEvent.event_type,
      projectedEvent.payload,
    ),
  };
};

const projectAssistantMessageEvent = (
  event: TaskThreadDetailEvent,
  resultByCallID: Map<string, TaskThreadDetailEvent>,
  context: TaskExecutionProjectionContext,
): ProjectedTaskExecutionEvent[] => {
  if (event.event_type !== 'message.completed') {
    return [];
  }

  const parsed = parseJSONObject(event.payload);

  if (getString(parsed, 'role') !== 'assistant') {
    return [];
  }

  const projections: ProjectedTaskExecutionEvent[] = [];
  const messageDisplay = getTaskThreadEventDisplay(
    event.event_type,
    event.payload,
  );

  if (messageDisplay.visibleInFlow !== false) {
    projections.push({ event, display: messageDisplay });
  }

  getToolCalls(parsed).forEach((toolCall, index) => {
    projections.push(
      createToolCallProjection({
        context,
        event,
        index,
        resultByCallID,
        toolCall,
      }),
    );
  });

  return projections;
};

const hasProjectedToolCallSource = (
  event: TaskThreadDetailEvent,
  projectedToolCallIDs: Set<string>,
) => {
  if (!event.event_type?.startsWith('tool.')) {
    return false;
  }

  const parsed = parseJSONObject(event.payload);
  const toolCallID = getString(parsed, 'tool_call_id');

  return Boolean(toolCallID && projectedToolCallIDs.has(toolCallID));
};

const getPlanTaskId = (event: TaskThreadDetailEvent) => {
  if (!event.event_type?.startsWith('plan.task.')) {
    return undefined;
  }

  try {
    const payload = parseJSONObject(event.payload);

    if (payload && typeof payload === 'object' && !Array.isArray(payload)) {
      const planTaskId = payload.plan_task_id;

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

export const projectTaskExecutionEvents = (events: TaskThreadDetailEvent[]) => {
  const context: TaskExecutionProjectionContext = {
    fallbackSkillName: getLoadedSkillFallbackName(events),
  };
  const latestPlanEventIndex = new Map<string, number>();
  const resultByCallID = getToolResultByCallID(events);
  const projectedToolCallIDs = new Set<string>();

  events.forEach((event, index) => {
    const planTaskId = getPlanTaskId(event);

    if (planTaskId) {
      latestPlanEventIndex.set(planTaskId, index);
    }
    const parsed = parseJSONObject(event.payload);

    getToolCalls(parsed).forEach(toolCall => {
      if (toolCall.id) {
        projectedToolCallIDs.add(toolCall.id);
      }
    });
  });

  return events
    .filter((event, index) => {
      const planTaskId = getPlanTaskId(event);

      return (
        (!planTaskId || latestPlanEventIndex.get(planTaskId) === index) &&
        !hasProjectedToolCallSource(event, projectedToolCallIDs)
      );
    })
    .flatMap(event => {
      const assistantMessageProjections = projectAssistantMessageEvent(
        event,
        resultByCallID,
        context,
      );

      if (assistantMessageProjections.length) {
        return assistantMessageProjections;
      }

      return [
        {
          event,
          display: getTaskThreadEventDisplay(event.event_type, event.payload),
        },
      ];
    })
    .filter(({ display }) => display.visibleInFlow !== false);
};
