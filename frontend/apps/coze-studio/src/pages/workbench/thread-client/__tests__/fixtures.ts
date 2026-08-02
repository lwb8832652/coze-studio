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

import type { WorkbenchTokenUsageResult } from '../workbench-thread-client';
import type {
  HumanInteractionResponse,
  WorkbenchArtifact,
  WorkbenchArtifactScanJob,
  WorkbenchGuardrailAuditEvent,
  WorkbenchMCPRuntimeAuditEvent,
  WorkbenchMemory,
  WorkbenchMemoryAuditEvent,
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchRunEvent,
  WorkbenchThread,
  WorkbenchTodo,
  WorkbenchTokenUsage,
  WorkbenchTokenUsageAggregate,
  WorkbenchUpload,
} from '../types';

export type DeepReadonly<T> = T extends (...args: never[]) => unknown
  ? T
  : T extends readonly (infer Item)[]
    ? readonly DeepReadonly<Item>[]
    : T extends object
      ? { readonly [Key in keyof T]: DeepReadonly<T[Key]> }
      : T;

const deepFreeze = (value: unknown): void => {
  if (!value || typeof value !== 'object' || Object.isFrozen(value)) {
    return;
  }

  for (const child of Object.values(value)) {
    deepFreeze(child);
  }

  Object.freeze(value);
};

const freezeFixture = <T>(value: T): DeepReadonly<T> => {
  deepFreeze(value);
  return value as DeepReadonly<T>;
};

const createdAt = 1767225600000;
const updatedAt = 1767225660000;
const deletedAt = 1767225720000;
const createdAtISO = '2026-01-01T00:00:00.000Z';
const updatedAtISO = '2026-01-01T00:01:00.000Z';
const deletedAtISO = '2026-01-01T00:02:00.000Z';
const ids = {
  thread: '1001',
  message: '2001',
  run: '3001',
  runEvent: '5001',
  file: '6001',
  artifact: '7001',
  scanJob: '8001',
  usage: '8201',
  memory: '8401',
  correctedMemory: '8400',
  memoryAudit: '8501',
  actor: '8601',
  guardrailAudit: '8701',
  mcpAudit: '8901',
  space: '9001',
} as const;
const opaque = {
  todo: 'todo-1',
  assistant: 'agent',
  worker: 'worker-safe-1',
  step: 'step-1',
  source: 'message:3001',
  target: 'artifact:5001',
  server: 'server-main',
  interaction: 'hi_1',
  choice: 'a',
} as const;

export const canonicalRunSubmissionExtensions = freezeFixture({
  turn: {
    message_metadata: { source: 'workbench_detail_followup' },
  },
  retry: {
    attempt_kind: 'retry',
    source_run_id: '3001',
  },
});

export interface CanonicalTransportFixture<Canonical, Visible> {
  canonical: Canonical;
  visible: Visible;
}

const canonicalFixture = <Canonical, Visible>(
  fixture: CanonicalTransportFixture<Canonical, Visible>,
): DeepReadonly<CanonicalTransportFixture<Canonical, Visible>> =>
  freezeFixture(fixture);

export const threadTransportFixture = canonicalFixture({
  canonical: {
    thread_id: ids.thread,
    created_at: createdAtISO,
    updated_at: updatedAtISO,
    metadata: { title: 'Prepare launch brief' },
    status: 'busy',
    values: {
      todos: [{ id: opaque.todo, title: 'Draft outline', status: 'completed' }],
    },
    interrupts: {},
    coze: {
      product_status: 'running',
      initial_submission: null,
      source: 'web',
      progress: 40,
      last_user_message: 'Prepare the launch brief',
      last_agent_message: 'Drafting the brief',
      can_edit: true,
    },
  },
  visible: {
    thread_id: ids.thread,
    space_id: ids.space,
    title: 'Prepare launch brief',
    status: 'running',
    source: 'web',
    progress: 40,
    last_user_message: 'Prepare the launch brief',
    last_agent_message: 'Drafting the brief',
    can_edit: true,
    created_at: createdAt,
    updated_at: updatedAt,
    values: {
      todos: [{ id: opaque.todo, title: 'Draft outline', status: 'completed' }],
    },
  } satisfies WorkbenchThread,
});

export const todoTransportFixture = canonicalFixture({
  canonical: {
    values: {
      todos: [{ id: opaque.todo, title: 'Draft outline', status: 'completed' }],
    },
  },
  visible: {
    id: opaque.todo,
    title: 'Draft outline',
    status: 'completed',
  } satisfies WorkbenchTodo,
});

export const messageTransportFixture = canonicalFixture({
  canonical: {
    message_id: ids.message,
    thread_id: ids.thread,
    run_id: ids.run,
    role: 'assistant',
    content: 'Drafting the brief',
    metadata: { channel: 'workbench' },
    created_at: updatedAtISO,
    seq: '2',
  },
  visible: {
    message_id: ids.message,
    thread_id: ids.thread,
    run_id: ids.run,
    role: 'assistant',
    content: 'Drafting the brief',
    metadata: '{"channel":"workbench"}',
    created_at: updatedAt,
  } satisfies WorkbenchMessage,
});

export const runTransportFixture = canonicalFixture({
  canonical: {
    run_id: ids.run,
    thread_id: ids.thread,
    assistant_id: opaque.assistant,
    status: 'running',
    created_at: createdAtISO,
    updated_at: updatedAtISO,
    metadata: { mode: 'agent' },
    multitask_strategy: 'reject',
    coze: {
      message_id: null,
      attempt_kind: 'turn',
      source_run_id: null,
      parent_run_id: null,
      run_kind: 'task',
      stream_modes: ['events'],
      on_disconnect: 'continue',
      durability: 'async',
      terminal_reason: null,
      started_at: createdAtISO,
      ended_at: null,
    },
  },
  visible: {
    run_id: ids.run,
    thread_id: ids.thread,
    space_id: ids.space,
    assistant_id: opaque.assistant,
    status: 'running',
    metadata: '{"mode":"agent"}',
    multitask_strategy: 'reject',
    attempt_kind: 'turn',
    run_kind: 'task',
    stream_modes: ['events'],
    on_disconnect: 'continue',
    durability: 'async',
    started_at: createdAt,
    created_at: createdAt,
    updated_at: updatedAt,
  } satisfies WorkbenchRun,
});

export const runEventTransportFixture = canonicalFixture({
  canonical: {
    data: [
      {
        event_id: ids.runEvent,
        thread_id: ids.thread,
        run_id: ids.run,
        event_type: 'run.started',
        payload: { phase: 'started' },
        created_at: createdAtISO,
      },
    ],
    has_more: false,
  },
  visible: {
    event_id: ids.runEvent,
    thread_id: ids.thread,
    run_id: ids.run,
    event_type: 'run.started',
    payload: '{"phase":"started"}',
    created_at: createdAt,
  } satisfies WorkbenchRunEvent,
});

export const uploadTransportFixture = canonicalFixture({
  canonical: {
    uploads: [
      {
        file_id: ids.file,
        file_name: 'brief.md',
        virtual_path: '/brief.md',
        content_type: 'text/markdown',
        size_bytes: 128,
        created_at: createdAtISO,
      },
    ],
    skipped_files: [],
  },
  visible: {
    file_id: ids.file,
    file_name: 'brief.md',
    virtual_path: '/brief.md',
    content_type: 'text/markdown',
    size_bytes: 128,
    created_at: createdAt,
  } satisfies WorkbenchUpload,
});

export const artifactTransportFixture = canonicalFixture({
  canonical: {
    artifacts: [
      {
        artifact_id: ids.artifact,
        thread_id: ids.thread,
        run_id: ids.run,
        file_id: ids.file,
        title: 'Launch brief',
        artifact_type: 'document',
        virtual_path: '/brief.md',
        content_type: 'text/markdown',
        size_bytes: 128,
        preview_mode: 'text',
        metadata: { scan_status: 'clean' },
        created_at: createdAtISO,
        updated_at: updatedAtISO,
        deleted_at: deletedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: {
    artifact_id: ids.artifact,
    thread_id: ids.thread,
    run_id: ids.run,
    file_id: ids.file,
    title: 'Launch brief',
    artifact_type: 'document',
    virtual_path: '/brief.md',
    content_type: 'text/markdown',
    size_bytes: 128,
    preview_mode: 'text',
    metadata: '{"scan_status":"clean"}',
    created_at: createdAt,
    updated_at: updatedAt,
    deleted_at: deletedAt,
  } satisfies WorkbenchArtifact,
});

export const artifactScanJobTransportFixture = canonicalFixture({
  canonical: {
    jobs: [
      {
        job_id: ids.scanJob,
        thread_id: ids.thread,
        run_id: ids.run,
        artifact_id: ids.artifact,
        file_id: ids.file,
        scanner: 'default',
        status: 'succeeded',
        worker_ref: opaque.worker,
        attempt_count: 1,
        error_code: '',
        available_at: createdAtISO,
        started_at: createdAtISO,
        ended_at: updatedAtISO,
        created_at: createdAtISO,
        updated_at: updatedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: {
    job_id: ids.scanJob,
    thread_id: ids.thread,
    run_id: ids.run,
    space_id: ids.space,
    artifact_id: ids.artifact,
    file_id: ids.file,
    scanner: 'default',
    status: 'succeeded',
    worker_id: opaque.worker,
    attempt_count: 1,
    error_code: '',
    available_at: createdAt,
    started_at: createdAt,
    ended_at: updatedAt,
    created_at: createdAt,
    updated_at: updatedAt,
  } satisfies WorkbenchArtifactScanJob,
});

const tokenAggregate = {
  input_tokens: 10,
  output_tokens: 5,
  total_tokens: 15,
  cost_micros: 25,
  call_count: 1,
  lead_agent_tokens: 15,
  subagent_tokens: 0,
  middleware_tokens: 0,
  tool_tokens: 0,
} satisfies WorkbenchTokenUsageAggregate;

const visibleTokenUsage = {
  usage_id: ids.usage,
  thread_id: ids.thread,
  run_id: ids.run,
  space_id: ids.space,
  source: 'lead_agent',
  step_id: opaque.step,
  step_index: 0,
  step_name: 'answer',
  model_name: 'gpt-test',
  provider: 'openai-compatible',
  input_tokens: 10,
  output_tokens: 5,
  total_tokens: 15,
  cost_micros: 25,
  currency: 'USD',
  estimated: false,
  created_at: createdAt,
} satisfies WorkbenchTokenUsage;

export const tokenUsageTransportFixture = canonicalFixture({
  canonical: {
    usage: [
      {
        usage_id: ids.usage,
        thread_id: ids.thread,
        run_id: ids.run,
        source: 'lead_agent',
        step_id: opaque.step,
        step_index: 0,
        step_name: 'answer',
        model_name: 'gpt-test',
        provider: 'openai-compatible',
        input_tokens: 10,
        output_tokens: 5,
        total_tokens: 15,
        cost_micros: 25,
        currency: 'USD',
        estimated: false,
        created_at: createdAtISO,
      },
    ],
    total: 1,
    has_more: false,
    aggregate: tokenAggregate,
    run_aggregates: [{ run_id: ids.run, aggregate: tokenAggregate }],
  },
  visible: {
    items: [visibleTokenUsage],
    total: 1,
    has_more: false,
    aggregate: tokenAggregate,
    run_aggregates: [{ run_id: ids.run, aggregate: tokenAggregate }],
  } satisfies WorkbenchTokenUsageResult,
});

const visibleMemory = {
  memory_id: ids.memory,
  thread_id: ids.thread,
  run_id: ids.run,
  space_id: ids.space,
  scope: 'thread',
  content: 'Launch date is Friday',
  metadata: '{"kind":"fact"}',
  score: 0.9,
  confidence: 0.95,
  source_type: 'message',
  source_id: opaque.source,
  correction_of_memory_id: ids.correctedMemory,
  corrected_at: createdAt,
  expires_at: updatedAt,
  created_at: createdAt,
  updated_at: updatedAt,
  deleted_at: deletedAt,
} satisfies WorkbenchMemory;

export const memoryTransportFixture = canonicalFixture({
  canonical: {
    memories: [
      {
        memory_id: ids.memory,
        thread_id: ids.thread,
        run_id: ids.run,
        scope: 'thread',
        content: 'Launch date is Friday',
        metadata: { kind: 'fact' },
        score: 0.9,
        confidence: 0.95,
        source_type: 'message',
        source_id: opaque.source,
        correction_of_memory_id: ids.correctedMemory,
        corrected_at: createdAtISO,
        expires_at: updatedAtISO,
        created_at: createdAtISO,
        updated_at: updatedAtISO,
        deleted_at: deletedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: visibleMemory,
});

const visibleMemoryAudit = {
  event_id: ids.memoryAudit,
  thread_id: ids.thread,
  run_id: ids.run,
  space_id: ids.space,
  memory_id: ids.memory,
  actor_id: ids.actor,
  event_type: 'memory.updated',
  scope: 'thread',
  source_type: 'message',
  source_id: opaque.source,
  affected_count: 1,
  created_at: updatedAt,
} satisfies WorkbenchMemoryAuditEvent;

export const memoryAuditTransportFixture = canonicalFixture({
  canonical: {
    events: [
      {
        event_id: ids.memoryAudit,
        thread_id: ids.thread,
        run_id: ids.run,
        memory_id: ids.memory,
        actor_id: ids.actor,
        event_type: 'memory.updated',
        scope: 'thread',
        source_type: 'message',
        source_id: opaque.source,
        affected_count: 1,
        created_at: updatedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: visibleMemoryAudit,
});

const visibleGuardrailAudit = {
  event_id: ids.guardrailAudit,
  thread_id: ids.thread,
  run_id: ids.run,
  space_id: ids.space,
  actor_id: ids.actor,
  event_type: 'guardrail.evaluated',
  target_type: 'run',
  target_id: opaque.target,
  operation: 'output',
  source: 'runtime',
  action: 'allow',
  fail_mode: 'closed',
  provider: 'builtin',
  reason_code: '',
  rule_ids: '["rule-1"]',
  created_at: updatedAt,
} satisfies WorkbenchGuardrailAuditEvent;

export const guardrailAuditTransportFixture = canonicalFixture({
  canonical: {
    events: [
      {
        event_id: ids.guardrailAudit,
        thread_id: ids.thread,
        run_id: ids.run,
        actor_id: ids.actor,
        event_type: 'guardrail.evaluated',
        target_type: 'run',
        target_id: opaque.target,
        operation: 'output',
        source: 'runtime',
        action: 'allow',
        fail_mode: 'closed',
        provider: 'builtin',
        reason_code: '',
        rule_ids: ['rule-1'],
        created_at: updatedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: visibleGuardrailAudit,
});

const visibleMCPAudit = {
  event_id: ids.mcpAudit,
  space_id: ids.space,
  thread_id: ids.thread,
  run_id: ids.run,
  server_id: opaque.server,
  runtime_tool_name: 'search',
  event_type: 'tool.completed',
  error_code: '',
  elapsed_millis: 25,
  output_bytes: 64,
  created_at: updatedAt,
} satisfies WorkbenchMCPRuntimeAuditEvent;

export const mcpRuntimeAuditTransportFixture = canonicalFixture({
  canonical: {
    events: [
      {
        event_id: ids.mcpAudit,
        thread_id: ids.thread,
        run_id: ids.run,
        server_id: opaque.server,
        runtime_tool_name: 'search',
        event_type: 'tool.completed',
        error_code: '',
        elapsed_millis: 25,
        output_bytes: 64,
        created_at: updatedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: visibleMCPAudit,
});

export const humanInteractionTransportFixture = canonicalFixture({
  canonical: {
    schema: 'coze.human_interaction_response.v1',
    interaction_id: opaque.interaction,
    kind: 'confirmation',
    decision: 'approved',
    comment: 'Proceed',
    choice_id: opaque.choice,
  },
  visible: {
    schema: 'coze.human_interaction_response.v1',
    interaction_id: opaque.interaction,
    kind: 'confirmation',
    decision: 'approved',
    comment: 'Proceed',
    choice_id: opaque.choice,
  } satisfies HumanInteractionResponse,
});

export const canonicalTransportFixtures = freezeFixture({
  thread: threadTransportFixture,
  todo: todoTransportFixture,
  message: messageTransportFixture,
  run: runTransportFixture,
  run_event: runEventTransportFixture,
  upload: uploadTransportFixture,
  artifact: artifactTransportFixture,
  artifact_scan_job: artifactScanJobTransportFixture,
  token_usage: tokenUsageTransportFixture,
  memory: memoryTransportFixture,
  memory_audit: memoryAuditTransportFixture,
  guardrail_audit: guardrailAuditTransportFixture,
  mcp_runtime_audit: mcpRuntimeAuditTransportFixture,
  human_interaction: humanInteractionTransportFixture,
});

export type TransportFixtureFamily = keyof typeof canonicalTransportFixtures;

type WireRecord = Record<string, unknown>;
type WireDecoder = (value: unknown, label: string) => unknown;
interface ReadRule {
  from: string;
  decode: WireDecoder;
  optional?: boolean;
  omit?: (value: unknown) => boolean;
}
type FieldRule = ReadRule | { constant: unknown };
type FieldMap = Record<string, FieldRule>;
type TransportFixtureProjector = (wire: unknown) => unknown;

const fixtureSpaceID = ids.space;

const asRecord = (value: unknown, label: string): WireRecord => {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new TypeError(`${label} must be an object`);
  }
  return value as WireRecord;
};

const asArray = (value: unknown, label: string): unknown[] => {
  if (!Array.isArray(value)) {
    throw new TypeError(`${label} must be an array`);
  }
  return value;
};

const asResourceID = (value: unknown, label: string): string => {
  if (typeof value === 'string' && /^[1-9]\d*$/.test(value)) {
    return value;
  }
  throw new TypeError(`${label} must be a positive decimal string ID`);
};

const asString = (value: unknown, label: string): string => {
  if (typeof value !== 'string') {
    throw new TypeError(`${label} must be a string`);
  }
  return value;
};

const asNonEmptyString = (value: unknown, label: string): string => {
  const decoded = asString(value, label);
  if (!decoded) {
    throw new TypeError(`${label} must be a non-empty string`);
  }
  return decoded;
};

const asBoolean = (value: unknown, label: string): boolean => {
  if (typeof value !== 'boolean') {
    throw new TypeError(`${label} must be a boolean`);
  }
  return value;
};

const asFiniteNumber = (value: unknown, label: string): number => {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new TypeError(`${label} must be a finite number`);
  }
  return value;
};

const asSafeInteger = (value: unknown, label: string): number => {
  if (!Number.isSafeInteger(value) || (value as number) < 0) {
    throw new TypeError(`${label} must be a non-negative safe integer`);
  }
  return value as number;
};

const rfc3339 =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/;

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
    day <= daysInMonth[month - 1] &&
    hour <= 23 &&
    minute <= 59 &&
    second <= 59 &&
    offsetHour <= 23 &&
    offsetMinute <= 59
  );
};

const asCanonicalEpoch = (value: unknown, label: string): number => {
  const milliseconds =
    typeof value === 'string' && isValidRFC3339Calendar(value)
      ? Date.parse(value)
      : Number.NaN;
  if (Number.isSafeInteger(milliseconds) && milliseconds >= 0) {
    return milliseconds;
  }
  throw new TypeError(`${label} must be a valid RFC3339 string`);
};

const assertJSONValue = (
  value: unknown,
  label: string,
  seen = new WeakSet<object>(),
): void => {
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'boolean'
  ) {
    return;
  }
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) {
      throw new TypeError(`${label} must not contain non-finite numbers`);
    }
    return;
  }
  if (typeof value !== 'object') {
    throw new TypeError(`${label} must contain JSON values only`);
  }
  if (seen.has(value)) {
    throw new TypeError(`${label} must not contain cycles`);
  }
  seen.add(value);
  const entries = Array.isArray(value)
    ? value.entries()
    : Object.entries(asRecord(value, label));
  for (const [key, child] of entries) {
    assertJSONValue(child, `${label}.${String(key)}`, seen);
  }
  seen.delete(value);
};

const serializeJSON = (value: unknown, label: string): string => {
  assertJSONValue(value, label);
  const serialized = JSON.stringify(value);
  if (typeof serialized !== 'string') {
    throw new TypeError(`${label} must be JSON serializable`);
  }
  return serialized;
};

const asCanonicalJSONObject = (value: unknown, label: string): string =>
  serializeJSON(asRecord(value, label), label);

const asCanonicalJSONArray = (value: unknown, label: string): string =>
  serializeJSON(asArray(value, label), label);

const asCanonicalObject = (value: unknown, label: string): WireRecord => {
  const object = asRecord(value, label);
  assertJSONValue(object, label);
  return object;
};

const asCanonicalInitialSubmission = (
  value: unknown,
  label: string,
): unknown => {
  if (value === null) {
    return value;
  }
  return asCanonicalObject(value, label);
};

const asStringArray = (value: unknown, label: string): string[] =>
  asArray(value, label).map((item, index) =>
    asString(item, `${label}[${index}]`),
  );

const exact =
  (expected: unknown): WireDecoder =>
  (value, label) => {
    if (value !== expected) {
      throw new TypeError(`${label} must equal ${String(expected)}`);
    }
    return value;
  };

const exactObjectKeys =
  (allowed: readonly string[]): WireDecoder =>
  (value, label) => {
    const record = asRecord(value, label);
    const extra = Object.keys(record).find(key => !allowed.includes(key));
    if (extra) {
      throw new TypeError(`${label}.${extra} is not allowed`);
    }
    return record;
  };

const oneOfStrings =
  (allowed: readonly string[]): WireDecoder =>
  (value, label) => {
    const decoded = asString(value, label);
    if (!allowed.includes(decoded)) {
      throw new TypeError(`${label} must be one of ${allowed.join(', ')}`);
    }
    return decoded;
  };

const read = (from: string, decode: WireDecoder): ReadRule => ({
  from,
  decode,
});
const optionalRead = (
  from: string,
  decode: WireDecoder,
  options: { omit?: (value: unknown) => boolean } = {},
): ReadRule => ({
  from,
  decode,
  optional: true,
  omit: options.omit,
});
const fixed = (constant: unknown): FieldRule => ({ constant });
const fields = <Schema extends FieldMap>(schema: Schema): Schema => schema;

const canonicalID = (from: string): ReadRule => read(from, asResourceID);
const string = (from: string): ReadRule => read(from, asString);
const nonEmptyString = (from: string): ReadRule => read(from, asNonEmptyString);
const boolean = (from: string): ReadRule => read(from, asBoolean);
const number = (from: string): ReadRule => read(from, asFiniteNumber);
const integer = (from: string): ReadRule => read(from, asSafeInteger);
const canonicalEpoch = (from: string): ReadRule => read(from, asCanonicalEpoch);
const canonicalJSONObject = (from: string): ReadRule =>
  read(from, asCanonicalJSONObject);
const canonicalJSONArray = (from: string): ReadRule =>
  read(from, asCanonicalJSONArray);
const optionalCanonicalID = (from: string): ReadRule =>
  optionalRead(from, asResourceID, { omit: value => value === null });
const optionalCanonicalString = (from: string): ReadRule =>
  optionalRead(from, asString, { omit: value => value === null });
const optionalCanonicalEpoch = (from: string): ReadRule =>
  optionalRead(from, asCanonicalEpoch, { omit: value => value === null });

const readPath = (
  source: unknown,
  path: string,
  options: { label: string; required?: boolean },
): unknown[] => {
  const { label, required = true } = options;
  return path
    .split('.')
    .filter(Boolean)
    .reduce<unknown[]>(
      (values, segment) => {
        if (segment === '*') {
          return values.flatMap((value, index) =>
            asArray(value, `${label}[${index}]`),
          );
        }
        return values
          .map(value => {
            if (Array.isArray(value) && /^\d+$/.test(segment)) {
              const item = value[Number(segment)];
              if (item === undefined) {
                if (!required) {
                  return undefined;
                }
                throw new TypeError(`${label}.${segment} is required`);
              }
              return item;
            }
            const record = asRecord(value, label);
            if (!(segment in record)) {
              if (!required) {
                return undefined;
              }
              throw new TypeError(`${label}.${segment} is required`);
            }
            return record[segment];
          })
          .filter(value => value !== undefined);
      },
      [source],
    );
};

const projectFields = (
  source: WireRecord,
  schema: FieldMap,
  label: string,
): WireRecord =>
  Object.fromEntries(
    Object.entries(schema).flatMap(([name, rule]) => {
      if ('constant' in rule) {
        return [[name, rule.constant]] as const;
      }
      const values = readPath(source, rule.from, {
        label,
        required: !rule.optional,
      });
      if (!values.length && rule.optional) {
        return [];
      }
      if (values.length !== 1) {
        throw new TypeError(`${label}.${rule.from} must resolve once`);
      }
      if (rule.optional && rule.omit?.(values[0])) {
        return [];
      }
      return [[name, rule.decode(values[0], `${label}.${rule.from}`)]] as const;
    }),
  );

const objectOf =
  (schema: FieldMap): WireDecoder =>
  (value, label) =>
    projectFields(asRecord(value, label), schema, label);

const arrayOf =
  (schema: FieldMap): WireDecoder =>
  (value, label) =>
    asArray(value, label).map((item, index) =>
      projectFields(
        asRecord(item, `${label}[${index}]`),
        schema,
        `${label}[${index}]`,
      ),
    );

type ProjectorArgs = [
  name: string,
  root: string,
  schema: FieldMap,
  checks?: readonly ReadRule[],
];

const createProjector =
  (
    ...[label, root, schema, checks = []]: ProjectorArgs
  ): TransportFixtureProjector =>
  wire => {
    checks.forEach(check => {
      const values = readPath(wire, check.from, { label });
      if (!values.length) {
        throw new TypeError(`${label}.${check.from} must contain a value`);
      }
      values.forEach((value, index) =>
        check.decode(value, `${label}.${check.from}[${index}]`),
      );
    });
    const roots = readPath(wire, root, { label });
    if (roots.length !== 1) {
      throw new TypeError(`${label}.${root} must resolve once`);
    }
    return projectFields(asRecord(roots[0], label), schema, label);
  };

const canonicalProjector = (
  ...[family, root, schema, checks = []]: ProjectorArgs
) => createProjector(`canonical.${family}`, root, schema, checks);

const canonicalTodoFields = fields({
  id: string('id'),
  title: string('title'),
  status: string('status'),
});

const threadTitleFromMetadata: WireDecoder = (value, label) => {
  const metadata = asRecord(value, label);
  return 'title' in metadata ? asString(metadata.title, `${label}.title`) : '';
};

const threadValuesFromTodos =
  (todoFields: FieldMap): WireDecoder =>
  (value, label) => ({ todos: arrayOf(todoFields)(value, label) });

const canonicalThreadFields = fields({
  thread_id: canonicalID('thread_id'),
  space_id: fixed(fixtureSpaceID),
  title: read('metadata', threadTitleFromMetadata),
  status: string('coze.product_status'),
  source: read('coze.source', oneOfStrings(['', 'web', 'im', 'api'])),
  progress: number('coze.progress'),
  last_user_message: string('coze.last_user_message'),
  last_agent_message: string('coze.last_agent_message'),
  can_edit: boolean('coze.can_edit'),
  created_at: canonicalEpoch('created_at'),
  updated_at: canonicalEpoch('updated_at'),
  values: optionalRead(
    'values.todos',
    threadValuesFromTodos(canonicalTodoFields),
  ),
});

const canonicalMessageFields = fields({
  message_id: canonicalID('message_id'),
  thread_id: canonicalID('thread_id'),
  run_id: canonicalID('run_id'),
  role: string('role'),
  content: string('content'),
  metadata: canonicalJSONObject('metadata'),
  created_at: canonicalEpoch('created_at'),
});

const canonicalRunFields = fields({
  run_id: canonicalID('run_id'),
  thread_id: canonicalID('thread_id'),
  space_id: fixed(fixtureSpaceID),
  assistant_id: read('assistant_id', exact('agent')),
  status: string('status'),
  metadata: canonicalJSONObject('metadata'),
  multitask_strategy: string('multitask_strategy'),
  message_id: optionalCanonicalID('coze.message_id'),
  attempt_kind: string('coze.attempt_kind'),
  source_run_id: optionalCanonicalID('coze.source_run_id'),
  parent_run_id: optionalCanonicalID('coze.parent_run_id'),
  run_kind: string('coze.run_kind'),
  stream_modes: read('coze.stream_modes', asStringArray),
  on_disconnect: string('coze.on_disconnect'),
  durability: string('coze.durability'),
  terminal_reason: optionalCanonicalString('coze.terminal_reason'),
  started_at: optionalCanonicalEpoch('coze.started_at'),
  ended_at: optionalCanonicalEpoch('coze.ended_at'),
  created_at: canonicalEpoch('created_at'),
  updated_at: canonicalEpoch('updated_at'),
});

const canonicalRunEventFields = fields({
  event_id: canonicalID('event_id'),
  thread_id: canonicalID('thread_id'),
  run_id: canonicalID('run_id'),
  event_type: string('event_type'),
  payload: canonicalJSONObject('payload'),
  created_at: canonicalEpoch('created_at'),
});

const canonicalUploadFields = fields({
  file_id: canonicalID('file_id'),
  file_name: string('file_name'),
  virtual_path: string('virtual_path'),
  content_type: string('content_type'),
  size_bytes: integer('size_bytes'),
  created_at: canonicalEpoch('created_at'),
});

const canonicalArtifactFields = fields({
  artifact_id: canonicalID('artifact_id'),
  thread_id: canonicalID('thread_id'),
  run_id: canonicalID('run_id'),
  file_id: canonicalID('file_id'),
  title: string('title'),
  artifact_type: string('artifact_type'),
  virtual_path: string('virtual_path'),
  content_type: string('content_type'),
  size_bytes: integer('size_bytes'),
  preview_mode: string('preview_mode'),
  metadata: canonicalJSONObject('metadata'),
  created_at: canonicalEpoch('created_at'),
  updated_at: canonicalEpoch('updated_at'),
  deleted_at: optionalCanonicalEpoch('deleted_at'),
});

const canonicalScanFields = fields({
  job_id: canonicalID('job_id'),
  thread_id: canonicalID('thread_id'),
  run_id: canonicalID('run_id'),
  space_id: fixed(fixtureSpaceID),
  artifact_id: canonicalID('artifact_id'),
  file_id: canonicalID('file_id'),
  scanner: string('scanner'),
  status: string('status'),
  worker_id: string('worker_ref'),
  attempt_count: integer('attempt_count'),
  error_code: string('error_code'),
  available_at: optionalCanonicalEpoch('available_at'),
  started_at: optionalCanonicalEpoch('started_at'),
  ended_at: optionalCanonicalEpoch('ended_at'),
  created_at: canonicalEpoch('created_at'),
  updated_at: canonicalEpoch('updated_at'),
});

const tokenAggregateFields = (): FieldMap =>
  fields({
    input_tokens: integer('input_tokens'),
    output_tokens: integer('output_tokens'),
    total_tokens: integer('total_tokens'),
    cost_micros: integer('cost_micros'),
    call_count: integer('call_count'),
    lead_agent_tokens: integer('lead_agent_tokens'),
    subagent_tokens: integer('subagent_tokens'),
    middleware_tokens: integer('middleware_tokens'),
    tool_tokens: integer('tool_tokens'),
  });
const runAggregateFields = (resourceID: (from: string) => ReadRule): FieldMap =>
  fields({
    run_id: resourceID('run_id'),
    aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  });

const canonicalTokenFields = fields({
  usage_id: canonicalID('usage_id'),
  thread_id: canonicalID('thread_id'),
  run_id: canonicalID('run_id'),
  space_id: fixed(fixtureSpaceID),
  source: string('source'),
  step_id: string('step_id'),
  step_index: integer('step_index'),
  step_name: string('step_name'),
  model_name: string('model_name'),
  provider: string('provider'),
  input_tokens: integer('input_tokens'),
  output_tokens: integer('output_tokens'),
  total_tokens: integer('total_tokens'),
  cost_micros: integer('cost_micros'),
  currency: string('currency'),
  estimated: boolean('estimated'),
  created_at: canonicalEpoch('created_at'),
});
const canonicalTokenPageFields: FieldMap = {
  items: read('usage', arrayOf(canonicalTokenFields)),
  total: integer('total'),
  has_more: boolean('has_more'),
  aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  run_aggregates: read(
    'run_aggregates',
    arrayOf(runAggregateFields(canonicalID)),
  ),
};

const canonicalMemoryFields = fields({
  memory_id: canonicalID('memory_id'),
  thread_id: canonicalID('thread_id'),
  run_id: optionalCanonicalID('run_id'),
  space_id: fixed(fixtureSpaceID),
  scope: string('scope'),
  content: string('content'),
  metadata: canonicalJSONObject('metadata'),
  score: number('score'),
  confidence: number('confidence'),
  source_type: string('source_type'),
  source_id: string('source_id'),
  correction_of_memory_id: optionalCanonicalID('correction_of_memory_id'),
  corrected_at: optionalCanonicalEpoch('corrected_at'),
  expires_at: optionalCanonicalEpoch('expires_at'),
  created_at: canonicalEpoch('created_at'),
  updated_at: canonicalEpoch('updated_at'),
  deleted_at: optionalCanonicalEpoch('deleted_at'),
});

interface AuditWireRules {
  id: (from: string) => ReadRule;
  optionalID: (from: string) => ReadRule;
  epoch: (from: string) => ReadRule;
  jsonArray: (from: string) => ReadRule;
  optionalString: (from: string) => ReadRule;
}
const canonicalAuditRules: AuditWireRules = {
  id: canonicalID,
  optionalID: optionalCanonicalID,
  epoch: canonicalEpoch,
  jsonArray: canonicalJSONArray,
  optionalString: optionalCanonicalString,
};
const auditBaseFields = (rules: AuditWireRules, space: FieldRule): FieldMap =>
  fields({
    event_id: rules.id('event_id'),
    thread_id: rules.id('thread_id'),
    run_id: rules.optionalID('run_id'),
    space_id: space,
    created_at: rules.epoch('created_at'),
  });
const memoryAuditFields = (rules: AuditWireRules, space: FieldRule): FieldMap =>
  fields({
    ...auditBaseFields(rules, space),
    memory_id: rules.optionalID('memory_id'),
    actor_id: rules.optionalID('actor_id'),
    event_type: string('event_type'),
    scope: string('scope'),
    source_type: string('source_type'),
    source_id: string('source_id'),
    affected_count: integer('affected_count'),
  });
const guardrailAuditFields = (
  rules: AuditWireRules,
  space: FieldRule,
): FieldMap =>
  fields({
    ...auditBaseFields(rules, space),
    actor_id: rules.optionalID('actor_id'),
    event_type: string('event_type'),
    target_type: string('target_type'),
    target_id: string('target_id'),
    operation: string('operation'),
    source: string('source'),
    action: string('action'),
    fail_mode: string('fail_mode'),
    provider: string('provider'),
    reason_code: string('reason_code'),
    rule_ids: rules.jsonArray('rule_ids'),
  });
const mcpAuditFields = (rules: AuditWireRules, space: FieldRule): FieldMap =>
  fields({
    ...auditBaseFields(rules, space),
    server_id: rules.optionalString('server_id'),
    runtime_tool_name: string('runtime_tool_name'),
    event_type: string('event_type'),
    error_code: string('error_code'),
    elapsed_millis: integer('elapsed_millis'),
    output_bytes: integer('output_bytes'),
  });

const canonicalHumanInteractionFields = fields({
  schema: read('schema', exact('coze.human_interaction_response.v1')),
  interaction_id: nonEmptyString('interaction_id'),
  kind: read('kind', exact('confirmation')),
  decision: read('decision', exact('approved')),
  answer: optionalCanonicalString('answer'),
  choice_id: optionalCanonicalString('choice_id'),
  comment: optionalCanonicalString('comment'),
});
const canonicalHumanInteractionKeys = [
  'schema',
  'interaction_id',
  'kind',
  'decision',
  'answer',
  'choice_id',
  'comment',
] as const;

const canonicalThreadShapeChecks = [
  read('status', exact('busy')),
  read('coze.initial_submission', asCanonicalInitialSubmission),
  read('interrupts', asCanonicalObject),
];

const canonicalPageChecks = [integer('total'), boolean('has_more')];
const pagedCanonicalProjector = (
  family: string,
  root: string,
  schema: FieldMap,
): TransportFixtureProjector =>
  canonicalProjector(family, root, schema, canonicalPageChecks);

const transportProjectors: Record<
  TransportFixtureFamily,
  TransportFixtureProjector
> = {
  thread: canonicalProjector(
    'thread',
    '',
    canonicalThreadFields,
    canonicalThreadShapeChecks,
  ),
  todo: canonicalProjector('todo', 'values.todos.0', canonicalTodoFields),
  message: canonicalProjector('message', '', canonicalMessageFields),
  run: canonicalProjector('run', '', canonicalRunFields),
  run_event: canonicalProjector(
    'run_event',
    'data.0',
    canonicalRunEventFields,
    [boolean('has_more')],
  ),
  upload: canonicalProjector('upload', 'uploads.0', canonicalUploadFields, [
    read('skipped_files', asArray),
  ]),
  artifact: pagedCanonicalProjector(
    'artifact',
    'artifacts.0',
    canonicalArtifactFields,
  ),
  artifact_scan_job: pagedCanonicalProjector(
    'artifact_scan_job',
    'jobs.0',
    canonicalScanFields,
  ),
  token_usage: canonicalProjector('token_usage', '', canonicalTokenPageFields),
  memory: pagedCanonicalProjector(
    'memory',
    'memories.0',
    canonicalMemoryFields,
  ),
  memory_audit: pagedCanonicalProjector(
    'memory_audit',
    'events.0',
    memoryAuditFields(canonicalAuditRules, fixed(fixtureSpaceID)),
  ),
  guardrail_audit: pagedCanonicalProjector(
    'guardrail_audit',
    'events.0',
    guardrailAuditFields(canonicalAuditRules, fixed(fixtureSpaceID)),
  ),
  mcp_runtime_audit: pagedCanonicalProjector(
    'mcp_runtime_audit',
    'events.0',
    mcpAuditFields(canonicalAuditRules, fixed(fixtureSpaceID)),
  ),
  human_interaction: canonicalProjector(
    'human_interaction',
    '',
    canonicalHumanInteractionFields,
    [read('', exactObjectKeys(canonicalHumanInteractionKeys))],
  ),
};

export const projectCanonicalTransportFixture = (
  family: TransportFixtureFamily,
  wire: unknown,
): unknown => transportProjectors[family](wire);
