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

/* eslint-disable max-lines -- Canonical resource adapters share one strict public decoder. */

import type {
  WorkbenchCursorPage,
  WorkbenchMessageCursorPage,
  WorkbenchPage,
  WorkbenchTokenUsageResult,
} from '../workbench-thread-client';
import type {
  WorkbenchArtifact,
  WorkbenchArtifactContent,
  WorkbenchArtifactRestoreResult,
  WorkbenchArtifactScanDecision,
  WorkbenchArtifactScanJob,
  WorkbenchArtifactScanRetryResult,
  WorkbenchArtifactScanReviewResult,
  WorkbenchArtifactSignedURL,
  WorkbenchGuardrailAuditEvent,
  WorkbenchGuardrailAuditExport,
  WorkbenchMCPRuntimeAuditEvent,
  WorkbenchMemory,
  WorkbenchMemoryAuditEvent,
  WorkbenchMemoryClearResult,
  WorkbenchMemoryExport,
  WorkbenchMemoryImportResult,
  WorkbenchMemoryRestoreResult,
  WorkbenchMemoryUpdateResult,
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchRunCreation,
  WorkbenchRunEvent,
  WorkbenchRunTokenUsageAggregate,
  WorkbenchThread,
  WorkbenchThreadCreation,
  WorkbenchTodo,
  WorkbenchTokenUsage,
  WorkbenchTokenUsageAggregate,
  WorkbenchUpload,
  WorkbenchUploadCreation,
} from '../types';
import type {
  CanonicalBlobResult,
  CanonicalPagination,
} from '../canonical-fetch';
import {
  invalidCanonicalRequest,
  invalidCanonicalResponse,
} from '../canonical-fetch';

type CanonicalRecord = Record<string, unknown>;

const positiveDecimalID = /^[1-9]\d*$/;
const nonNegativeDecimalID = /^(0|[1-9]\d*)$/;
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

const asFiniteNumber = (value: unknown, label: string): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    return responseFailure(label, 'a finite number');
  }
  return value;
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
  const hasSubmissionMessage = Object.prototype.hasOwnProperty.call(
    coze,
    'submission_message',
  );
  if (run.run_kind !== 'task') {
    return responseFailure(
      'canonical Run.coze.run_kind',
      'task for Run creation',
    );
  }
  if (run.parent_run_id !== undefined) {
    return responseFailure(
      'canonical Run.coze.parent_run_id',
      'null for Run creation',
    );
  }
  if (run.attempt_kind === 'retry') {
    if (hasSubmissionMessage) {
      return responseFailure(
        'canonical Run.coze.submission_message',
        'omitted for top-level retry creation',
      );
    }
    if (run.message_id !== undefined) {
      return responseFailure(
        'canonical Run.coze.message_id',
        'null for top-level retry creation',
      );
    }
    if (run.source_run_id === undefined) {
      return responseFailure(
        'canonical Run.coze.source_run_id',
        'present for top-level retry creation',
      );
    }
    return { run };
  }
  if (run.attempt_kind !== 'turn') {
    return responseFailure(
      'canonical Run.coze.attempt_kind',
      'turn or retry for Run creation',
    );
  }
  if (run.source_run_id !== undefined) {
    return responseFailure(
      'canonical Run.coze.source_run_id',
      'null for turn creation',
    );
  }
  if (!hasSubmissionMessage) {
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

const optionalResourceIDField = (
  object: CanonicalRecord,
  key: string,
  label: string,
): string | undefined =>
  Object.prototype.hasOwnProperty.call(object, key)
    ? asResourceID(object[key], `${label}.${key}`)
    : undefined;

const optionalStringField = (
  object: CanonicalRecord,
  key: string,
  label: string,
): string | undefined =>
  Object.prototype.hasOwnProperty.call(object, key)
    ? asString(object[key], `${label}.${key}`)
    : undefined;

const optionalEpochField = (
  object: CanonicalRecord,
  key: string,
  label: string,
): number | undefined =>
  Object.prototype.hasOwnProperty.call(object, key)
    ? asEpochMilliseconds(object[key], `${label}.${key}`)
    : undefined;

const optionalPageCursor = (
  object: CanonicalRecord,
  label: string,
): string | undefined => {
  if (!Object.prototype.hasOwnProperty.call(object, 'next_cursor')) {
    return undefined;
  }
  const cursor = asString(object.next_cursor, `${label}.next_cursor`);
  if (!nonNegativeDecimalID.test(cursor)) {
    return responseFailure(
      `${label}.next_cursor`,
      'a non-negative decimal cursor',
    );
  }
  return cursor;
};

const adaptCanonicalProductPage = <T>(
  value: unknown,
  options: {
    label: string;
    itemsKey: string;
    adapt: (item: unknown, index: number) => T;
  },
): WorkbenchPage<T> => {
  const { label, itemsKey, adapt } = options;
  const page = asRecord(value, label);
  const next = optionalPageCursor(page, label);
  return {
    items: asArray(required(page, itemsKey, label), `${label}.${itemsKey}`).map(
      adapt,
    ),
    total: asSafeNonNegativeInteger(
      required(page, 'total', label),
      `${label}.total`,
    ),
    has_more: asBoolean(required(page, 'has_more', label), `${label}.has_more`),
    ...(next === undefined ? {} : { next_cursor: next }),
  };
};

const adaptCanonicalUpload = (
  value: unknown,
  indexLabel = 'canonical Upload',
): WorkbenchUpload => {
  const upload = asRecord(value, indexLabel);
  return {
    file_id: asResourceID(
      required(upload, 'file_id', indexLabel),
      `${indexLabel}.file_id`,
    ),
    file_name: asString(
      required(upload, 'file_name', indexLabel),
      `${indexLabel}.file_name`,
    ),
    virtual_path: asString(
      required(upload, 'virtual_path', indexLabel),
      `${indexLabel}.virtual_path`,
    ),
    content_type: asString(
      required(upload, 'content_type', indexLabel),
      `${indexLabel}.content_type`,
    ),
    size_bytes: asSafeNonNegativeInteger(
      required(upload, 'size_bytes', indexLabel),
      `${indexLabel}.size_bytes`,
    ),
    created_at: asEpochMilliseconds(
      required(upload, 'created_at', indexLabel),
      `${indexLabel}.created_at`,
    ),
  };
};

export const adaptCanonicalUploadPage = (
  value: unknown,
): WorkbenchPage<WorkbenchUpload> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical Upload page',
    itemsKey: 'uploads',
    adapt: (item, index) =>
      adaptCanonicalUpload(item, `canonical Upload page.uploads[${index}]`),
  });

export const adaptCanonicalUploadCreation = (
  value: unknown,
): WorkbenchUploadCreation => {
  const label = 'canonical Upload creation';
  const response = asRecord(value, label);
  return {
    uploads: asArray(
      required(response, 'uploads', label),
      `${label}.uploads`,
    ).map((item, index) =>
      adaptCanonicalUpload(item, `${label}.uploads[${index}]`),
    ),
    skipped_files: asStringArray(
      required(response, 'skipped_files', label),
      `${label}.skipped_files`,
    ),
  };
};

const adaptCanonicalArtifact = (
  value: unknown,
  scope: AdapterScope,
  label = 'canonical Artifact',
): WorkbenchArtifact => {
  const artifact = asRecord(value, label);
  const threadID = asResourceID(
    required(artifact, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const deletedAt = optionalEpochField(artifact, 'deleted_at', label);
  return {
    artifact_id: asResourceID(
      required(artifact, 'artifact_id', label),
      `${label}.artifact_id`,
    ),
    thread_id: threadID,
    run_id: asResourceID(
      required(artifact, 'run_id', label),
      `${label}.run_id`,
    ),
    file_id: asResourceID(
      required(artifact, 'file_id', label),
      `${label}.file_id`,
    ),
    title: asString(required(artifact, 'title', label), `${label}.title`),
    artifact_type: asString(
      required(artifact, 'artifact_type', label),
      `${label}.artifact_type`,
    ),
    virtual_path: asString(
      required(artifact, 'virtual_path', label),
      `${label}.virtual_path`,
    ),
    content_type: asString(
      required(artifact, 'content_type', label),
      `${label}.content_type`,
    ),
    size_bytes: asSafeNonNegativeInteger(
      required(artifact, 'size_bytes', label),
      `${label}.size_bytes`,
    ),
    preview_mode: asString(
      required(artifact, 'preview_mode', label),
      `${label}.preview_mode`,
    ),
    metadata: stringifyJSONObject(
      required(artifact, 'metadata', label),
      `${label}.metadata`,
    ),
    created_at: asEpochMilliseconds(
      required(artifact, 'created_at', label),
      `${label}.created_at`,
    ),
    updated_at: asEpochMilliseconds(
      required(artifact, 'updated_at', label),
      `${label}.updated_at`,
    ),
    ...(deletedAt === undefined ? {} : { deleted_at: deletedAt }),
  };
};

export const adaptCanonicalArtifactPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchPage<WorkbenchArtifact> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical Artifact page',
    itemsKey: 'artifacts',
    adapt: (item, index) =>
      adaptCanonicalArtifact(
        item,
        scope,
        `canonical Artifact page.artifacts[${index}]`,
      ),
  });

export const adaptCanonicalArtifactContent = (
  value: CanonicalBlobResult,
): WorkbenchArtifactContent => {
  if (!(value.blob instanceof Blob)) {
    return responseFailure('canonical Artifact content body', 'a Blob');
  }
  if (!value.contentType) {
    return responseFailure(
      'canonical Artifact content Content-Type',
      'present',
    );
  }
  if (!value.contentDisposition) {
    return responseFailure(
      'canonical Artifact content Content-Disposition',
      'present',
    );
  }
  return {
    blob: value.blob,
    content_type: value.contentType,
    content_disposition: value.contentDisposition,
  };
};

export const adaptCanonicalArtifactSignedURL = (
  value: unknown,
  artifactId: string,
): WorkbenchArtifactSignedURL => {
  const label = 'canonical Artifact signed URL';
  const response = asRecord(value, label);
  const responseArtifactID = asResourceID(
    required(response, 'artifact_id', label),
    `${label}.artifact_id`,
  );
  assertExpectedID(responseArtifactID, artifactId, `${label}.artifact_id`);
  return {
    artifact_id: responseArtifactID,
    url: asNonEmptyString(required(response, 'url', label), `${label}.url`),
    expires_in_seconds: asSafeNonNegativeInteger(
      required(response, 'expires_in_seconds', label),
      `${label}.expires_in_seconds`,
    ),
    content_type: asString(
      required(response, 'content_type', label),
      `${label}.content_type`,
    ),
    preview_mode: asString(
      required(response, 'preview_mode', label),
      `${label}.preview_mode`,
    ),
  };
};

export const adaptCanonicalArtifactRestore = (
  value: unknown,
  scope: AdapterScope,
  artifactId: string,
): WorkbenchArtifactRestoreResult => {
  const label = 'canonical Artifact restore';
  const response = asRecord(value, label);
  const artifact = adaptCanonicalArtifact(
    required(response, 'artifact', label),
    scope,
    `${label}.artifact`,
  );
  assertExpectedID(artifact.artifact_id, artifactId, `${label}.artifact_id`);
  return {
    artifact,
    restored: asBoolean(
      required(response, 'restored', label),
      `${label}.restored`,
    ),
  };
};

export const adaptCanonicalArtifactScanReview = (
  value: unknown,
  artifactId: string,
): WorkbenchArtifactScanReviewResult => {
  const label = 'canonical Artifact scan review';
  const response = asRecord(value, label);
  const responseArtifactID = asResourceID(
    required(response, 'artifact_id', label),
    `${label}.artifact_id`,
  );
  assertExpectedID(responseArtifactID, artifactId, `${label}.artifact_id`);
  const decision = asString(
    required(response, 'decision', label),
    `${label}.decision`,
  );
  if (!['release', 'quarantine', 'block'].includes(decision)) {
    return responseFailure(`${label}.decision`, 'a reviewed scan decision');
  }
  return {
    artifact_id: responseArtifactID,
    decision: decision as WorkbenchArtifactScanDecision,
    scan_status: asString(
      required(response, 'scan_status', label),
      `${label}.scan_status`,
    ),
    reviewed: asBoolean(
      required(response, 'reviewed', label),
      `${label}.reviewed`,
    ),
  };
};

const adaptCanonicalArtifactScanJob = (
  value: unknown,
  scope: AdapterScope,
  label = 'canonical Artifact scan job',
): WorkbenchArtifactScanJob => {
  const job = asRecord(value, label);
  const threadID = asResourceID(
    required(job, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const availableAt = optionalEpochField(job, 'available_at', label);
  const startedAt = optionalEpochField(job, 'started_at', label);
  const endedAt = optionalEpochField(job, 'ended_at', label);
  return {
    job_id: asResourceID(required(job, 'job_id', label), `${label}.job_id`),
    thread_id: threadID,
    run_id: asResourceID(required(job, 'run_id', label), `${label}.run_id`),
    space_id: scope.spaceId,
    artifact_id: asResourceID(
      required(job, 'artifact_id', label),
      `${label}.artifact_id`,
    ),
    file_id: asResourceID(required(job, 'file_id', label), `${label}.file_id`),
    scanner: asString(required(job, 'scanner', label), `${label}.scanner`),
    status: asString(required(job, 'status', label), `${label}.status`),
    worker_id: asString(
      required(job, 'worker_ref', label),
      `${label}.worker_ref`,
    ),
    attempt_count: asSafeNonNegativeInteger(
      required(job, 'attempt_count', label),
      `${label}.attempt_count`,
    ),
    error_code: asString(
      required(job, 'error_code', label),
      `${label}.error_code`,
    ),
    ...(availableAt === undefined ? {} : { available_at: availableAt }),
    ...(startedAt === undefined ? {} : { started_at: startedAt }),
    ...(endedAt === undefined ? {} : { ended_at: endedAt }),
    created_at: asEpochMilliseconds(
      required(job, 'created_at', label),
      `${label}.created_at`,
    ),
    updated_at: asEpochMilliseconds(
      required(job, 'updated_at', label),
      `${label}.updated_at`,
    ),
  };
};

export const adaptCanonicalArtifactScanJobPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchPage<WorkbenchArtifactScanJob> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical Artifact scan job page',
    itemsKey: 'jobs',
    adapt: (item, index) =>
      adaptCanonicalArtifactScanJob(
        item,
        scope,
        `canonical Artifact scan job page.jobs[${index}]`,
      ),
  });

export const adaptCanonicalArtifactScanRetry = (
  value: unknown,
  scope: AdapterScope,
  jobId: string,
): WorkbenchArtifactScanRetryResult => {
  const label = 'canonical Artifact scan retry';
  const response = asRecord(value, label);
  const job = adaptCanonicalArtifactScanJob(
    required(response, 'job', label),
    scope,
    `${label}.job`,
  );
  assertExpectedID(job.job_id, jobId, `${label}.job.job_id`);
  return {
    job,
    retried: asBoolean(
      required(response, 'retried', label),
      `${label}.retried`,
    ),
  };
};

const adaptCanonicalTokenUsageAggregate = (
  value: unknown,
  label: string,
): WorkbenchTokenUsageAggregate => {
  const aggregate = asRecord(value, label);
  const number = (key: string) =>
    asSafeNonNegativeInteger(
      required(aggregate, key, label),
      `${label}.${key}`,
    );
  return {
    input_tokens: number('input_tokens'),
    output_tokens: number('output_tokens'),
    total_tokens: number('total_tokens'),
    cost_micros: number('cost_micros'),
    call_count: number('call_count'),
    lead_agent_tokens: number('lead_agent_tokens'),
    subagent_tokens: number('subagent_tokens'),
    middleware_tokens: number('middleware_tokens'),
    tool_tokens: number('tool_tokens'),
  };
};

const adaptCanonicalTokenUsage = (
  value: unknown,
  scope: AdapterScope,
  label: string,
): WorkbenchTokenUsage => {
  const usage = asRecord(value, label);
  const threadID = asResourceID(
    required(usage, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  return {
    usage_id: asResourceID(
      required(usage, 'usage_id', label),
      `${label}.usage_id`,
    ),
    thread_id: threadID,
    run_id: asResourceID(required(usage, 'run_id', label), `${label}.run_id`),
    space_id: scope.spaceId,
    source: asString(required(usage, 'source', label), `${label}.source`),
    step_id: asString(required(usage, 'step_id', label), `${label}.step_id`),
    step_index: asSafeNonNegativeInteger(
      required(usage, 'step_index', label),
      `${label}.step_index`,
    ),
    step_name: asString(
      required(usage, 'step_name', label),
      `${label}.step_name`,
    ),
    model_name: asString(
      required(usage, 'model_name', label),
      `${label}.model_name`,
    ),
    provider: asString(required(usage, 'provider', label), `${label}.provider`),
    input_tokens: asSafeNonNegativeInteger(
      required(usage, 'input_tokens', label),
      `${label}.input_tokens`,
    ),
    output_tokens: asSafeNonNegativeInteger(
      required(usage, 'output_tokens', label),
      `${label}.output_tokens`,
    ),
    total_tokens: asSafeNonNegativeInteger(
      required(usage, 'total_tokens', label),
      `${label}.total_tokens`,
    ),
    cost_micros: asSafeNonNegativeInteger(
      required(usage, 'cost_micros', label),
      `${label}.cost_micros`,
    ),
    currency: asString(required(usage, 'currency', label), `${label}.currency`),
    estimated: asBoolean(
      required(usage, 'estimated', label),
      `${label}.estimated`,
    ),
    created_at: asEpochMilliseconds(
      required(usage, 'created_at', label),
      `${label}.created_at`,
    ),
  };
};

export const adaptCanonicalTokenUsageResult = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchTokenUsageResult => {
  const label = 'canonical Token usage response';
  const response = asRecord(value, label);
  const next = optionalPageCursor(response, label);
  const runAggregates = asArray(
    required(response, 'run_aggregates', label),
    `${label}.run_aggregates`,
  ).map((item, index): WorkbenchRunTokenUsageAggregate => {
    const itemLabel = `${label}.run_aggregates[${index}]`;
    const aggregate = asRecord(item, itemLabel);
    return {
      run_id: asResourceID(
        required(aggregate, 'run_id', itemLabel),
        `${itemLabel}.run_id`,
      ),
      aggregate: adaptCanonicalTokenUsageAggregate(
        required(aggregate, 'aggregate', itemLabel),
        `${itemLabel}.aggregate`,
      ),
    };
  });
  return {
    items: asArray(required(response, 'usage', label), `${label}.usage`).map(
      (item, index) =>
        adaptCanonicalTokenUsage(item, scope, `${label}.usage[${index}]`),
    ),
    total: asSafeNonNegativeInteger(
      required(response, 'total', label),
      `${label}.total`,
    ),
    has_more: asBoolean(
      required(response, 'has_more', label),
      `${label}.has_more`,
    ),
    ...(next === undefined ? {} : { next_cursor: next }),
    aggregate: adaptCanonicalTokenUsageAggregate(
      required(response, 'aggregate', label),
      `${label}.aggregate`,
    ),
    run_aggregates: runAggregates,
  };
};

const adaptCanonicalMemory = (
  value: unknown,
  scope: AdapterScope,
  label = 'canonical Memory',
): WorkbenchMemory => {
  const memory = asRecord(value, label);
  const threadID = asResourceID(
    required(memory, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const runID = optionalResourceIDField(memory, 'run_id', label);
  const correctionID = optionalResourceIDField(
    memory,
    'correction_of_memory_id',
    label,
  );
  const correctedAt = optionalEpochField(memory, 'corrected_at', label);
  const expiresAt = optionalEpochField(memory, 'expires_at', label);
  const deletedAt = optionalEpochField(memory, 'deleted_at', label);
  return {
    memory_id: asResourceID(
      required(memory, 'memory_id', label),
      `${label}.memory_id`,
    ),
    thread_id: threadID,
    ...(runID === undefined ? {} : { run_id: runID }),
    space_id: scope.spaceId,
    scope: asString(required(memory, 'scope', label), `${label}.scope`),
    content: asString(required(memory, 'content', label), `${label}.content`),
    metadata: stringifyJSONObject(
      required(memory, 'metadata', label),
      `${label}.metadata`,
    ),
    score: asFiniteNumber(required(memory, 'score', label), `${label}.score`),
    confidence: asFiniteNumber(
      required(memory, 'confidence', label),
      `${label}.confidence`,
    ),
    source_type: asString(
      required(memory, 'source_type', label),
      `${label}.source_type`,
    ),
    source_id: asString(
      required(memory, 'source_id', label),
      `${label}.source_id`,
    ),
    ...(correctionID === undefined
      ? {}
      : { correction_of_memory_id: correctionID }),
    ...(correctedAt === undefined ? {} : { corrected_at: correctedAt }),
    ...(expiresAt === undefined ? {} : { expires_at: expiresAt }),
    created_at: asEpochMilliseconds(
      required(memory, 'created_at', label),
      `${label}.created_at`,
    ),
    updated_at: asEpochMilliseconds(
      required(memory, 'updated_at', label),
      `${label}.updated_at`,
    ),
    ...(deletedAt === undefined ? {} : { deleted_at: deletedAt }),
  };
};

export const adaptCanonicalMemoryPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchPage<WorkbenchMemory> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical Memory page',
    itemsKey: 'memories',
    adapt: (item, index) =>
      adaptCanonicalMemory(
        item,
        scope,
        `canonical Memory page.memories[${index}]`,
      ),
  });

export const adaptCanonicalMemoryUpdate = (
  value: unknown,
  scope: AdapterScope,
  memoryId: string,
): WorkbenchMemoryUpdateResult => {
  const label = 'canonical Memory update';
  const response = asRecord(value, label);
  const memory = adaptCanonicalMemory(
    required(response, 'memory', label),
    scope,
    `${label}.memory`,
  );
  assertExpectedID(memory.memory_id, memoryId, `${label}.memory.memory_id`);
  return {
    memory,
    updated: asBoolean(
      required(response, 'updated', label),
      `${label}.updated`,
    ),
  };
};

export const adaptCanonicalMemoryRestore = (
  value: unknown,
  scope: AdapterScope,
  memoryId: string,
): WorkbenchMemoryRestoreResult => {
  const label = 'canonical Memory restore';
  const response = asRecord(value, label);
  const memory = adaptCanonicalMemory(
    required(response, 'memory', label),
    scope,
    `${label}.memory`,
  );
  assertExpectedID(memory.memory_id, memoryId, `${label}.memory.memory_id`);
  return {
    memory,
    restored: asBoolean(
      required(response, 'restored', label),
      `${label}.restored`,
    ),
  };
};

export const adaptCanonicalMemoryClear = (
  value: unknown,
): WorkbenchMemoryClearResult => {
  const label = 'canonical Memory clear';
  const response = asRecord(value, label);
  return {
    deleted: asSafeNonNegativeInteger(
      required(response, 'deleted', label),
      `${label}.deleted`,
    ),
  };
};

export const adaptCanonicalMemoryImport = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchMemoryImportResult => {
  const label = 'canonical Memory import';
  const response = asRecord(value, label);
  return {
    imported: asSafeNonNegativeInteger(
      required(response, 'imported', label),
      `${label}.imported`,
    ),
    skipped: asSafeNonNegativeInteger(
      required(response, 'skipped', label),
      `${label}.skipped`,
    ),
    memories: asArray(
      required(response, 'memories', label),
      `${label}.memories`,
    ).map((item, index) =>
      adaptCanonicalMemory(item, scope, `${label}.memories[${index}]`),
    ),
  };
};

export const adaptCanonicalMemoryExport = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchMemoryExport => {
  const label = 'canonical Memory export';
  const response = asRecord(value, label);
  const threadID = asResourceID(
    required(response, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  return {
    schema: asNonEmptyString(
      required(response, 'schema', label),
      `${label}.schema`,
    ),
    thread_id: threadID,
    exported_at: asEpochMilliseconds(
      required(response, 'exported_at', label),
      `${label}.exported_at`,
    ),
    total: asSafeNonNegativeInteger(
      required(response, 'total', label),
      `${label}.total`,
    ),
    memories: asArray(
      required(response, 'memories', label),
      `${label}.memories`,
    ).map((item, index) =>
      adaptCanonicalMemory(item, scope, `${label}.memories[${index}]`),
    ),
  };
};

const adaptCanonicalMemoryAudit = (
  value: unknown,
  scope: AdapterScope,
  label: string,
): WorkbenchMemoryAuditEvent => {
  const event = asRecord(value, label);
  const threadID = asResourceID(
    required(event, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const runID = optionalResourceIDField(event, 'run_id', label);
  const memoryID = optionalResourceIDField(event, 'memory_id', label);
  const actorID = optionalResourceIDField(event, 'actor_id', label);
  return {
    event_id: asResourceID(
      required(event, 'event_id', label),
      `${label}.event_id`,
    ),
    thread_id: threadID,
    ...(runID === undefined ? {} : { run_id: runID }),
    space_id: scope.spaceId,
    ...(memoryID === undefined ? {} : { memory_id: memoryID }),
    ...(actorID === undefined ? {} : { actor_id: actorID }),
    event_type: asString(
      required(event, 'event_type', label),
      `${label}.event_type`,
    ),
    scope: asString(required(event, 'scope', label), `${label}.scope`),
    source_type: asString(
      required(event, 'source_type', label),
      `${label}.source_type`,
    ),
    source_id: asString(
      required(event, 'source_id', label),
      `${label}.source_id`,
    ),
    affected_count: asSafeNonNegativeInteger(
      required(event, 'affected_count', label),
      `${label}.affected_count`,
    ),
    created_at: asEpochMilliseconds(
      required(event, 'created_at', label),
      `${label}.created_at`,
    ),
  };
};

export const adaptCanonicalMemoryAuditPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchPage<WorkbenchMemoryAuditEvent> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical Memory audit page',
    itemsKey: 'events',
    adapt: (item, index) =>
      adaptCanonicalMemoryAudit(
        item,
        scope,
        `canonical Memory audit page.events[${index}]`,
      ),
  });

const adaptCanonicalGuardrailAudit = (
  value: unknown,
  scope: AdapterScope,
  label: string,
): WorkbenchGuardrailAuditEvent => {
  const event = asRecord(value, label);
  const threadID = asResourceID(
    required(event, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const runID = optionalResourceIDField(event, 'run_id', label);
  const actorID = optionalResourceIDField(event, 'actor_id', label);
  return {
    event_id: asResourceID(
      required(event, 'event_id', label),
      `${label}.event_id`,
    ),
    thread_id: threadID,
    ...(runID === undefined ? {} : { run_id: runID }),
    space_id: scope.spaceId,
    ...(actorID === undefined ? {} : { actor_id: actorID }),
    event_type: asString(
      required(event, 'event_type', label),
      `${label}.event_type`,
    ),
    target_type: asString(
      required(event, 'target_type', label),
      `${label}.target_type`,
    ),
    target_id: asString(
      required(event, 'target_id', label),
      `${label}.target_id`,
    ),
    operation: asString(
      required(event, 'operation', label),
      `${label}.operation`,
    ),
    source: asString(required(event, 'source', label), `${label}.source`),
    action: asString(required(event, 'action', label), `${label}.action`),
    fail_mode: asString(
      required(event, 'fail_mode', label),
      `${label}.fail_mode`,
    ),
    provider: asString(required(event, 'provider', label), `${label}.provider`),
    reason_code: asString(
      required(event, 'reason_code', label),
      `${label}.reason_code`,
    ),
    rule_ids: JSON.stringify(
      asStringArray(required(event, 'rule_ids', label), `${label}.rule_ids`),
    ),
    created_at: asEpochMilliseconds(
      required(event, 'created_at', label),
      `${label}.created_at`,
    ),
  };
};

export const adaptCanonicalGuardrailAuditPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchPage<WorkbenchGuardrailAuditEvent> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical Guardrail audit page',
    itemsKey: 'events',
    adapt: (item, index) =>
      adaptCanonicalGuardrailAudit(
        item,
        scope,
        `canonical Guardrail audit page.events[${index}]`,
      ),
  });

export const adaptCanonicalGuardrailAuditExport = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchGuardrailAuditExport => {
  const label = 'canonical Guardrail audit export';
  const response = asRecord(value, label);
  const threadID = asResourceID(
    required(response, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  return {
    schema: asNonEmptyString(
      required(response, 'schema', label),
      `${label}.schema`,
    ),
    thread_id: threadID,
    exported_at: asEpochMilliseconds(
      required(response, 'exported_at', label),
      `${label}.exported_at`,
    ),
    total: asSafeNonNegativeInteger(
      required(response, 'total', label),
      `${label}.total`,
    ),
    events: asArray(required(response, 'events', label), `${label}.events`).map(
      (item, index) =>
        adaptCanonicalGuardrailAudit(item, scope, `${label}.events[${index}]`),
    ),
  };
};

const adaptCanonicalMCPRuntimeAudit = (
  value: unknown,
  scope: AdapterScope,
  label: string,
): WorkbenchMCPRuntimeAuditEvent => {
  const event = asRecord(value, label);
  const threadID = asResourceID(
    required(event, 'thread_id', label),
    `${label}.thread_id`,
  );
  assertExpectedID(threadID, scope.threadId, `${label}.thread_id`);
  const runID = optionalResourceIDField(event, 'run_id', label);
  const serverID = optionalStringField(event, 'server_id', label);
  return {
    event_id: asResourceID(
      required(event, 'event_id', label),
      `${label}.event_id`,
    ),
    space_id: scope.spaceId,
    thread_id: threadID,
    ...(runID === undefined ? {} : { run_id: runID }),
    ...(serverID === undefined ? {} : { server_id: serverID }),
    runtime_tool_name: asString(
      required(event, 'runtime_tool_name', label),
      `${label}.runtime_tool_name`,
    ),
    event_type: asString(
      required(event, 'event_type', label),
      `${label}.event_type`,
    ),
    error_code: asString(
      required(event, 'error_code', label),
      `${label}.error_code`,
    ),
    elapsed_millis: asSafeNonNegativeInteger(
      required(event, 'elapsed_millis', label),
      `${label}.elapsed_millis`,
    ),
    output_bytes: asSafeNonNegativeInteger(
      required(event, 'output_bytes', label),
      `${label}.output_bytes`,
    ),
    created_at: asEpochMilliseconds(
      required(event, 'created_at', label),
      `${label}.created_at`,
    ),
  };
};

export const adaptCanonicalMCPRuntimeAuditPage = (
  value: unknown,
  scope: AdapterScope,
): WorkbenchPage<WorkbenchMCPRuntimeAuditEvent> =>
  adaptCanonicalProductPage(value, {
    label: 'canonical MCP runtime audit page',
    itemsKey: 'events',
    adapt: (item, index) =>
      adaptCanonicalMCPRuntimeAudit(
        item,
        scope,
        `canonical MCP runtime audit page.events[${index}]`,
      ),
  });

export const adaptCanonicalSuggestions = (value: unknown): string[] => {
  const label = 'canonical Suggestion response';
  const response = asRecord(value, label);
  return asStringArray(
    required(response, 'suggestions', label),
    `${label}.suggestions`,
  );
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
