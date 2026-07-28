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

/* eslint-disable max-lines -- Core resource adapters share one strict canonical decoder. */

import type {
  WorkbenchCursorPage,
  WorkbenchMessageCursorPage,
  WorkbenchPage,
} from '../workbench-thread-client';
import type {
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchRunCreation,
  WorkbenchRunEvent,
  WorkbenchThread,
  WorkbenchThreadCreation,
  WorkbenchTodo,
} from '../types';
import type { CanonicalPagination } from '../canonical-fetch';
import {
  invalidCanonicalRequest,
  invalidCanonicalResponse,
} from '../canonical-fetch';

type CanonicalRecord = Record<string, unknown>;

const positiveDecimalID = /^[1-9]\d*$/;
const rfc3339 =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/;

const responseFailure = (label: string, expected: string): never => {
  throw invalidCanonicalResponse(`${label} must be ${expected}`);
};

const asRecord = (value: unknown, label: string): CanonicalRecord => {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    return responseFailure(label, 'an object');
  }
  return value as CanonicalRecord;
};

const asArray = (value: unknown, label: string): unknown[] => {
  if (!Array.isArray(value)) {
    return responseFailure(label, 'an array');
  }
  return value;
};

const required = (
  object: CanonicalRecord,
  key: string,
  label: string,
): unknown => {
  if (!Object.prototype.hasOwnProperty.call(object, key)) {
    return responseFailure(`${label}.${key}`, 'present');
  }
  return object[key];
};

const asString = (value: unknown, label: string): string => {
  if (typeof value !== 'string') {
    return responseFailure(label, 'a string');
  }
  return value;
};

const asNonEmptyString = (value: unknown, label: string): string => {
  const decoded = asString(value, label);
  if (decoded.length === 0) {
    return responseFailure(label, 'a non-empty string');
  }
  return decoded;
};

const asBoolean = (value: unknown, label: string): boolean => {
  if (typeof value !== 'boolean') {
    return responseFailure(label, 'a boolean');
  }
  return value;
};

const asResourceID = (value: unknown, label: string): string => {
  if (typeof value !== 'string' || !positiveDecimalID.test(value)) {
    return responseFailure(label, 'a positive decimal string ID');
  }
  return value;
};

const asSafeNonNegativeInteger = (value: unknown, label: string): number => {
  if (!Number.isSafeInteger(value) || (value as number) < 0) {
    return responseFailure(label, 'a non-negative safe integer');
  }
  return value as number;
};

interface JSONValidation {
  label: string;
  fail: (message: string) => never;
  seen?: WeakSet<object>;
}

const assertJSONValue = (value: unknown, validation: JSONValidation): void => {
  const { label, fail } = validation;
  const seen = validation.seen ?? new WeakSet<object>();
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'boolean'
  ) {
    return;
  }
  if (typeof value === 'number') {
    if (
      !Number.isFinite(value) ||
      (Number.isInteger(value) && !Number.isSafeInteger(value))
    ) {
      return fail(`${label} contains an unsafe number`);
    }
    return;
  }
  if (typeof value !== 'object') {
    return fail(`${label} contains a non-JSON value`);
  }
  if (seen.has(value)) {
    return fail(`${label} contains a cycle`);
  }
  seen.add(value);
  if (Array.isArray(value)) {
    value.forEach((item, index) =>
      assertJSONValue(item, {
        label: `${label}[${index}]`,
        fail,
        seen,
      }),
    );
  } else {
    Object.entries(value).forEach(([key, item]) =>
      assertJSONValue(item, { label: `${label}.${key}`, fail, seen }),
    );
  }
  seen.delete(value);
};

const asJSONObject = (value: unknown, label: string): CanonicalRecord => {
  const object = asRecord(value, label);
  assertJSONValue(object, {
    label,
    fail: message => {
      throw invalidCanonicalResponse(message);
    },
  });
  return object;
};

const stringifyJSONObject = (value: unknown, label: string): string => {
  const object = asJSONObject(value, label);
  try {
    return JSON.stringify(object);
  } catch (error) {
    return responseFailure(label, `JSON serializable: ${String(error)}`);
  }
};

/* eslint-disable complexity, @typescript-eslint/no-magic-numbers -- RFC3339 calendar bounds are protocol constants. */
const isValidRFC3339Calendar = (value: string): boolean => {
  const match = rfc3339.exec(value);
  if (!match) {
    return false;
  }
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const offsetHour = Number(match[7] ?? 0);
  const offsetMinute = Number(match[8] ?? 0);
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const daysInMonth = [
    31,
    leapYear ? 29 : 28,
    31,
    30,
    31,
    30,
    31,
    31,
    30,
    31,
    30,
    31,
  ];

  return (
    month >= 1 &&
    month <= 12 &&
    day >= 1 &&
    day <= (daysInMonth[month - 1] ?? 0) &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59 &&
    offsetHour <= 23 &&
    offsetMinute <= 59
  );
};
/* eslint-enable complexity, @typescript-eslint/no-magic-numbers -- RFC3339 calendar validation ends here. */

const asEpochMilliseconds = (value: unknown, label: string): number => {
  const milliseconds =
    typeof value === 'string' && isValidRFC3339Calendar(value)
      ? Date.parse(value)
      : Number.NaN;
  if (!Number.isSafeInteger(milliseconds) || milliseconds < 0) {
    return responseFailure(label, 'a valid RFC3339 timestamp');
  }
  return milliseconds;
};

const nullableResourceID = (
  value: unknown,
  label: string,
): string | undefined =>
  value === null ? undefined : asResourceID(value, label);

const nullableString = (value: unknown, label: string): string | undefined =>
  value === null ? undefined : asString(value, label);

const nullableEpoch = (value: unknown, label: string): number | undefined =>
  value === null ? undefined : asEpochMilliseconds(value, label);

const asStringArray = (value: unknown, label: string): string[] =>
  asArray(value, label).map((item, index) =>
    asString(item, `${label}[${index}]`),
  );

const assertExpectedID = (
  actual: string,
  expected: string | undefined,
  label: string,
): void => {
  if (expected !== undefined && actual !== expected) {
    responseFailure(label, `the addressed ID ${expected}`);
  }
};

const adaptTodo = (value: unknown, label: string): WorkbenchTodo => {
  const todo = asRecord(value, label);
  return {
    id: asString(required(todo, 'id', label), `${label}.id`),
    title: asString(required(todo, 'title', label), `${label}.title`),
    status: asString(required(todo, 'status', label), `${label}.status`),
  };
};

interface AdapterScope {
  spaceId: string;
  threadId?: string;
  runId?: string;
}

export const adaptCanonicalMessage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchMessage => {
  const label = 'canonical Message';
  const message = asRecord(value, label);
  const messageID = asResourceID(
    required(message, 'message_id', label),
    `${label}.message_id`,
  );
  const threadID = asResourceID(
    required(message, 'thread_id', label),
    `${label}.thread_id`,
  );
  const runID = asResourceID(
    required(message, 'run_id', label),
    `${label}.run_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  assertExpectedID(runID, scope.runId, `${label}.run_id`);
  if (Object.prototype.hasOwnProperty.call(message, 'seq')) {
    asResourceID(message.seq, `${label}.seq`);
  }
  return {
    message_id: messageID,
    thread_id: threadID,
    run_id: runID,
    role: asString(required(message, 'role', label), `${label}.role`),
    content: asString(required(message, 'content', label), `${label}.content`),
    metadata: stringifyJSONObject(
      required(message, 'metadata', label),
      `${label}.metadata`,
    ),
    created_at: asEpochMilliseconds(
      required(message, 'created_at', label),
      `${label}.created_at`,
    ),
  };
};

export const adaptCanonicalRun = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchRun => {
  const label = 'canonical Run';
  const run = asRecord(value, label);
  const coze = asRecord(required(run, 'coze', label), `${label}.coze`);
  const runID = asResourceID(required(run, 'run_id', label), `${label}.run_id`);
  const threadID = asResourceID(
    required(run, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  assertExpectedID(runID, scope.runId, `${label}.run_id`);

  const assistantID = asString(
    required(run, 'assistant_id', label),
    `${label}.assistant_id`,
  );
  if (assistantID !== 'agent') {
    responseFailure(`${label}.assistant_id`, 'the public alias agent');
  }
  const messageID = nullableResourceID(
    required(coze, 'message_id', `${label}.coze`),
    `${label}.coze.message_id`,
  );
  const sourceRunID = nullableResourceID(
    required(coze, 'source_run_id', `${label}.coze`),
    `${label}.coze.source_run_id`,
  );
  const parentRunID = nullableResourceID(
    required(coze, 'parent_run_id', `${label}.coze`),
    `${label}.coze.parent_run_id`,
  );
  const terminalReason = nullableString(
    required(coze, 'terminal_reason', `${label}.coze`),
    `${label}.coze.terminal_reason`,
  );
  const startedAt = nullableEpoch(
    required(coze, 'started_at', `${label}.coze`),
    `${label}.coze.started_at`,
  );
  const endedAt = nullableEpoch(
    required(coze, 'ended_at', `${label}.coze`),
    `${label}.coze.ended_at`,
  );

  if (Object.prototype.hasOwnProperty.call(coze, 'submission_message')) {
    adaptCanonicalMessage(coze.submission_message, {
      spaceId: scope.spaceId,
      threadId: threadID,
      runId: runID,
    });
  }

  return {
    run_id: runID,
    thread_id: threadID,
    space_id: scope.spaceId,
    assistant_id: assistantID,
    status: asString(required(run, 'status', label), `${label}.status`),
    metadata: stringifyJSONObject(
      required(run, 'metadata', label),
      `${label}.metadata`,
    ),
    multitask_strategy: asString(
      required(run, 'multitask_strategy', label),
      `${label}.multitask_strategy`,
    ),
    ...(messageID === undefined ? {} : { message_id: messageID }),
    attempt_kind: asString(
      required(coze, 'attempt_kind', `${label}.coze`),
      `${label}.coze.attempt_kind`,
    ),
    ...(sourceRunID === undefined ? {} : { source_run_id: sourceRunID }),
    ...(parentRunID === undefined ? {} : { parent_run_id: parentRunID }),
    run_kind: asString(
      required(coze, 'run_kind', `${label}.coze`),
      `${label}.coze.run_kind`,
    ),
    stream_modes: asStringArray(
      required(coze, 'stream_modes', `${label}.coze`),
      `${label}.coze.stream_modes`,
    ),
    on_disconnect: asString(
      required(coze, 'on_disconnect', `${label}.coze`),
      `${label}.coze.on_disconnect`,
    ),
    durability: asString(
      required(coze, 'durability', `${label}.coze`),
      `${label}.coze.durability`,
    ),
    ...(terminalReason === undefined
      ? {}
      : { terminal_reason: terminalReason }),
    ...(startedAt === undefined ? {} : { started_at: startedAt }),
    ...(endedAt === undefined ? {} : { ended_at: endedAt }),
    created_at: asEpochMilliseconds(
      required(run, 'created_at', label),
      `${label}.created_at`,
    ),
    updated_at: asEpochMilliseconds(
      required(run, 'updated_at', label),
      `${label}.updated_at`,
    ),
  };
};

interface CanonicalInitialSubmission {
  message: WorkbenchMessage;
  run: WorkbenchRun;
}

const adaptInitialSubmission = (
  value: unknown,
  scope: AdapterScope,
): CanonicalInitialSubmission | undefined => {
  if (value === null) {
    return undefined;
  }
  const label = 'canonical Thread.coze.initial_submission';
  const submission = asRecord(value, label);
  const run = adaptCanonicalRun(required(submission, 'run', label), scope);
  const message = adaptCanonicalMessage(
    required(submission, 'message', label),
    {
      spaceId: scope.spaceId,
      threadId: run.thread_id,
      runId: run.run_id,
    },
  );
  return { message, run };
};

export const adaptCanonicalThread = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchThread => {
  const label = 'canonical Thread';
  const thread = asRecord(value, label);
  const metadata = asJSONObject(
    required(thread, 'metadata', label),
    `${label}.metadata`,
  );
  const values = asJSONObject(
    required(thread, 'values', label),
    `${label}.values`,
  );
  asJSONObject(required(thread, 'interrupts', label), `${label}.interrupts`);
  const coze = asRecord(required(thread, 'coze', label), `${label}.coze`);
  const threadID = asResourceID(
    required(thread, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const canonicalStatus = asString(
    required(thread, 'status', label),
    `${label}.status`,
  );
  if (!['idle', 'busy', 'interrupted', 'error'].includes(canonicalStatus)) {
    responseFailure(`${label}.status`, 'a reviewed canonical Thread status');
  }
  const source = asString(
    required(coze, 'source', `${label}.coze`),
    `${label}.coze.source`,
  );
  if (!['', 'web', 'im', 'api'].includes(source)) {
    responseFailure(`${label}.coze.source`, 'a reviewed public source');
  }
  adaptInitialSubmission(
    required(coze, 'initial_submission', `${label}.coze`),
    { spaceId: scope.spaceId, threadId: threadID },
  );

  let todos: WorkbenchTodo[] | undefined;
  if (Object.prototype.hasOwnProperty.call(values, 'todos')) {
    todos = asArray(values.todos, `${label}.values.todos`).map((todo, index) =>
      adaptTodo(todo, `${label}.values.todos[${index}]`),
    );
  }
  const title = Object.prototype.hasOwnProperty.call(metadata, 'title')
    ? asString(metadata.title, `${label}.metadata.title`)
    : '';

  return {
    thread_id: threadID,
    space_id: scope.spaceId,
    title,
    status: asString(
      required(coze, 'product_status', `${label}.coze`),
      `${label}.coze.product_status`,
    ),
    source,
    progress: asSafeNonNegativeInteger(
      required(coze, 'progress', `${label}.coze`),
      `${label}.coze.progress`,
    ),
    last_user_message: asString(
      required(coze, 'last_user_message', `${label}.coze`),
      `${label}.coze.last_user_message`,
    ),
    last_agent_message: asString(
      required(coze, 'last_agent_message', `${label}.coze`),
      `${label}.coze.last_agent_message`,
    ),
    can_edit: asBoolean(
      required(coze, 'can_edit', `${label}.coze`),
      `${label}.coze.can_edit`,
    ),
    created_at: asEpochMilliseconds(
      required(thread, 'created_at', label),
      `${label}.created_at`,
    ),
    updated_at: asEpochMilliseconds(
      required(thread, 'updated_at', label),
      `${label}.updated_at`,
    ),
    ...(todos === undefined ? {} : { values: { todos } }),
  };
};

export const adaptCanonicalThreadCreation = (
  value: unknown,
  spaceId: string,
): WorkbenchThreadCreation => {
  const thread = adaptCanonicalThread(value, { spaceId });
  const wire = asRecord(value, 'canonical Thread');
  const coze = asRecord(wire.coze, 'canonical Thread.coze');
  const submission = adaptInitialSubmission(coze.initial_submission, {
    spaceId,
    threadId: thread.thread_id,
  });
  return submission === undefined
    ? { thread }
    : { thread, message: submission.message, run: submission.run };
};

export const adaptCanonicalRunCreation = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchRunCreation => {
  const run = adaptCanonicalRun(value, scope);
  const wire = asRecord(value, 'canonical Run');
  const coze = asRecord(wire.coze, 'canonical Run.coze');
  if (!Object.prototype.hasOwnProperty.call(coze, 'submission_message')) {
    return responseFailure(
      'canonical Run.coze.submission_message',
      'present for Run creation',
    );
  }
  const message = adaptCanonicalMessage(coze.submission_message, {
    spaceId: scope.spaceId,
    threadId: run.thread_id,
    runId: run.run_id,
  });
  if (run.message_id !== message.message_id) {
    return responseFailure(
      'canonical Run.coze.message_id',
      'the submission Message ID',
    );
  }
  return { run, message };
};

export const adaptCanonicalRunEvent = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchRunEvent => {
  const label = 'canonical Run event';
  const event = asRecord(value, label);
  const threadID = asResourceID(
    required(event, 'thread_id', label),
    `${label}.thread_id`,
  );
  const runID = asResourceID(
    required(event, 'run_id', label),
    `${label}.run_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  assertExpectedID(runID, scope.runId, `${label}.run_id`);
  return {
    event_id: asResourceID(
      required(event, 'event_id', label),
      `${label}.event_id`,
    ),
    thread_id: threadID,
    run_id: runID,
    event_type: asNonEmptyString(
      required(event, 'event_type', label),
      `${label}.event_type`,
    ),
    payload: stringifyJSONObject(
      required(event, 'payload', label),
      `${label}.payload`,
    ),
    created_at: asEpochMilliseconds(
      required(event, 'created_at', label),
      `${label}.created_at`,
    ),
  };
};

const requirePaginationTotal = (pagination: CanonicalPagination): number => {
  if (pagination.total === undefined) {
    return responseFailure('X-Pagination-Total', 'present');
  }
  return asSafeNonNegativeInteger(pagination.total, 'X-Pagination-Total');
};

export const adaptCanonicalThreadList = (
  value: unknown,
  spaceId: string,
  pagination: CanonicalPagination,
): WorkbenchPage<WorkbenchThread> => {
  const items = asArray(value, 'canonical Thread list').map(item =>
    adaptCanonicalThread(item, { spaceId }),
  );
  const { next } = pagination;
  return {
    items,
    total: requirePaginationTotal(pagination),
    has_more: next !== undefined,
    ...(next === undefined ? {} : { next_cursor: next }),
  };
};

export const adaptCanonicalRunList = (
  value: unknown,
  scope: AdapterScope,
  pagination: CanonicalPagination,
): WorkbenchPage<WorkbenchRun> => {
  const items = asArray(value, 'canonical Run list').map(item =>
    adaptCanonicalRun(item, scope),
  );
  const { next } = pagination;
  return {
    items,
    total: requirePaginationTotal(pagination),
    has_more: next !== undefined,
    ...(next === undefined ? {} : { next_cursor: next }),
  };
};

export const adaptCanonicalMessagePage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchMessageCursorPage => {
  const label = 'canonical Message page';
  const page = asRecord(value, label);
  const nextBefore = Object.prototype.hasOwnProperty.call(
    page,
    'next_before_seq',
  )
    ? asResourceID(page.next_before_seq, `${label}.next_before_seq`)
    : undefined;
  const nextAfter = Object.prototype.hasOwnProperty.call(page, 'next_after_seq')
    ? asResourceID(page.next_after_seq, `${label}.next_after_seq`)
    : undefined;
  return {
    items: asArray(required(page, 'data', label), `${label}.data`).map(item =>
      adaptCanonicalMessage(item, scope),
    ),
    has_more: asBoolean(required(page, 'has_more', label), `${label}.has_more`),
    ...(nextBefore === undefined ? {} : { next_before_seq: nextBefore }),
    ...(nextAfter === undefined ? {} : { next_after_seq: nextAfter }),
  };
};

export const adaptCanonicalRunEventPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchCursorPage<WorkbenchRunEvent> => {
  const label = 'canonical Run event page';
  const page = asRecord(value, label);
  const next = Object.prototype.hasOwnProperty.call(page, 'next_after_event_id')
    ? asResourceID(page.next_after_event_id, `${label}.next_after_event_id`)
    : undefined;
  return {
    items: asArray(required(page, 'data', label), `${label}.data`).map(item =>
      adaptCanonicalRunEvent(item, scope),
    ),
    has_more: asBoolean(required(page, 'has_more', label), `${label}.has_more`),
    ...(next === undefined ? {} : { next_cursor: next }),
  };
};

export const assertCanonicalRequestResourceID = (
  value: unknown,
  label: string,
): string => {
  if (typeof value !== 'string' || !positiveDecimalID.test(value)) {
    throw invalidCanonicalRequest(
      'invalid_resource_id',
      `${label} must be a positive decimal string ID`,
    );
  }
  return value;
};

export const parseCanonicalWriteJSON = (
  value: string | undefined,
  label: string,
): unknown => {
  if (value === undefined || value.trim() === '') {
    return undefined;
  }
  let parsed: unknown;
  try {
    parsed = JSON.parse(value) as unknown;
  } catch (error) {
    throw invalidCanonicalRequest(
      'invalid_request_json',
      `${label} must contain valid JSON: ${String(error)}`,
    );
  }
  assertJSONValue(parsed, {
    label,
    fail: message => {
      throw invalidCanonicalRequest('invalid_request_json', message);
    },
  });
  return parsed;
};

export const parseCanonicalWriteJSONObject = (
  value: string | undefined,
  label: string,
): CanonicalRecord | undefined => {
  const parsed = parseCanonicalWriteJSON(value, label);
  if (parsed === undefined || parsed === null) {
    return undefined;
  }
  if (typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      `${label} must contain a JSON object`,
    );
  }
  return parsed as CanonicalRecord;
};

export const parseRequiredCanonicalWriteJSONObject = (
  value: string,
  label: string,
): CanonicalRecord => {
  const parsed = parseCanonicalWriteJSONObject(value, label);
  if (parsed === undefined) {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      `${label} must contain a JSON object`,
    );
  }
  return parsed;
};
