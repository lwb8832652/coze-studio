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
import {
  freezeLegacyTaskThreadInput,
  type DeepReadonly,
} from './legacy-task-thread-reference';

const createdAt = 1767225600000;
const updatedAt = 1767225660000;
const deletedAt = 1767225720000;
const createdAtISO = '2026-01-01T00:00:00.000Z';
const updatedAtISO = '2026-01-01T00:01:00.000Z';
const deletedAtISO = '2026-01-01T00:02:00.000Z';
const ids = {
  thread: '1001',
  todo: '1101',
  message: '2001',
  run: '3001',
  assistant: '4001',
  runEvent: '5001',
  file: '6001',
  artifact: '7001',
  scanJob: '8001',
  worker: '8101',
  usage: '8201',
  step: '8301',
  memory: '8401',
  correctedMemory: '8400',
  memoryAudit: '8501',
  actor: '8601',
  guardrailAudit: '8701',
  mcpAudit: '8901',
  space: '9001',
  server: '9002',
  interaction: '9101',
  journal: '9201',
} as const;

export interface PairedTransportFixture<Legacy, Canonical, Visible> {
  v1: Legacy;
  canonical: Canonical;
  visible: Visible;
}

const pairFixture = <Legacy, Canonical, Visible>(
  fixture: PairedTransportFixture<Legacy, Canonical, Visible>,
): DeepReadonly<PairedTransportFixture<Legacy, Canonical, Visible>> =>
  freezeLegacyTaskThreadInput(fixture);

export const threadTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      thread_id: ids.thread,
      space_id: ids.space,
      creator_id: ids.actor,
      title: 'Prepare launch brief',
      status: 'running',
      source: 'agent',
      progress: 40,
      last_user_message: 'Prepare the launch brief',
      last_agent_message: 'Drafting the brief',
      created_at: createdAt,
      updated_at: updatedAt,
      values: {
        todos: [{ id: ids.todo, title: 'Draft outline', status: 'completed' }],
      },
    },
  },
  canonical: {
    thread_id: ids.thread,
    created_at: createdAtISO,
    updated_at: updatedAtISO,
    metadata: { title: 'Prepare launch brief' },
    status: 'running',
    values: {
      todos: [{ id: ids.todo, title: 'Draft outline', status: 'completed' }],
    },
    interrupts: [],
    coze: {
      product_status: 'active',
      initial_submission: 'Prepare the launch brief',
      source: 'agent',
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
    source: 'agent',
    progress: 40,
    last_user_message: 'Prepare the launch brief',
    last_agent_message: 'Drafting the brief',
    can_edit: true,
    created_at: createdAt,
    updated_at: updatedAt,
    values: {
      todos: [{ id: ids.todo, title: 'Draft outline', status: 'completed' }],
    },
  } satisfies WorkbenchThread,
});

export const todoTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: { id: ids.todo, title: 'Draft outline', status: 'completed' },
  },
  canonical: {
    values: {
      todos: [{ id: ids.todo, title: 'Draft outline', status: 'completed' }],
    },
  },
  visible: {
    id: ids.todo,
    title: 'Draft outline',
    status: 'completed',
  } satisfies WorkbenchTodo,
});

export const messageTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      message_id: ids.message,
      thread_id: ids.thread,
      run_id: ids.run,
      role: 'assistant',
      content: 'Drafting the brief',
      metadata: '{"channel":"workbench"}',
      created_at: updatedAt,
    },
  },
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

export const runTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      run_id: ids.run,
      thread_id: ids.thread,
      parent_run_id: '',
      space_id: ids.space,
      creator_id: ids.actor,
      assistant_id: ids.assistant,
      run_kind: 'agent',
      status: 'running',
      command: '',
      input: '{"message":"Prepare the launch brief"}',
      config: '{"model_name":"gpt-test"}',
      context: '{}',
      metadata: '{"mode":"agent"}',
      stream_mode: 'events',
      multitask_strategy: 'reject',
      on_disconnect: 'continue',
      durability: 'async',
      worker_id: ids.worker,
      error_code: '',
      error_message: '',
      started_at: createdAt,
      ended_at: 0,
      created_at: createdAt,
      updated_at: updatedAt,
    },
  },
  canonical: {
    run_id: ids.run,
    thread_id: ids.thread,
    assistant_id: ids.assistant,
    status: 'running',
    created_at: createdAtISO,
    updated_at: updatedAtISO,
    metadata: { mode: 'agent' },
    multitask_strategy: 'reject',
    coze: {
      attempt_kind: 'initial',
      run_kind: 'agent',
      stream_modes: ['events'],
      on_disconnect: 'continue',
      durability: 'async',
      started_at: createdAtISO,
    },
  },
  visible: {
    run_id: ids.run,
    thread_id: ids.thread,
    space_id: ids.space,
    assistant_id: ids.assistant,
    status: 'running',
    metadata: '{"mode":"agent"}',
    multitask_strategy: 'reject',
    attempt_kind: 'initial',
    run_kind: 'agent',
    stream_modes: ['events'],
    on_disconnect: 'continue',
    durability: 'async',
    started_at: createdAt,
    created_at: createdAt,
    updated_at: updatedAt,
  } satisfies WorkbenchRun,
});

export const runEventTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      events: [
        {
          event_id: ids.runEvent,
          thread_id: ids.thread,
          run_id: ids.run,
          event_type: 'run.started',
          payload: '{"phase":"started"}',
          created_at: createdAt,
        },
      ],
      total: 1,
      journal_messages: [
        {
          id: ids.journal,
          tool_calls: [{ arguments: '{"must":"be dropped"}' }],
          usage: '{"must":"be dropped"}',
        },
      ],
    },
  },
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

export const uploadTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      success: true,
      files: [
        {
          file_id: ids.file,
          filename: 'brief.md',
          path: `/uploads/${ids.file}`,
          virtual_path: '/brief.md',
          content_type: 'text/markdown',
          size: 128,
          created_at: createdAt,
        },
      ],
      message: 'uploaded',
      skipped_files: [],
    },
  },
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

export const artifactTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
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
          metadata: '{"scan_status":"clean"}',
          created_at: createdAt,
          updated_at: updatedAt,
          deleted_at: deletedAt,
        },
      ],
      total: 1,
    },
  },
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

export const artifactScanJobTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      jobs: [
        {
          job_id: ids.scanJob,
          thread_id: ids.thread,
          run_id: ids.run,
          space_id: ids.space,
          artifact_id: ids.artifact,
          file_id: ids.file,
          scanner: 'default',
          status: 'succeeded',
          worker_id: ids.worker,
          attempt_count: 1,
          last_error: '',
          available_at: createdAt,
          started_at: createdAt,
          ended_at: updatedAt,
          created_at: createdAt,
          updated_at: updatedAt,
        },
      ],
      total: 1,
    },
  },
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
        worker_ref: ids.worker,
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
    worker_id: ids.worker,
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
  step_id: ids.step,
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

export const tokenUsageTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      usage: [
        {
          ...visibleTokenUsage,
          raw_usage: '{"must":"be dropped"}',
          metadata: '{"must":"be dropped"}',
        },
      ],
      total: 1,
      aggregate: tokenAggregate,
      run_aggregates: [{ run_id: ids.run, aggregate: tokenAggregate }],
    },
  },
  canonical: {
    usage: [
      {
        usage_id: ids.usage,
        thread_id: ids.thread,
        run_id: ids.run,
        source: 'lead_agent',
        step_id: ids.step,
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
  source_id: ids.message,
  correction_of_memory_id: ids.correctedMemory,
  corrected_at: createdAt,
  expires_at: updatedAt,
  created_at: createdAt,
  updated_at: updatedAt,
  deleted_at: deletedAt,
} satisfies WorkbenchMemory;

export const memoryTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: { memories: [visibleMemory], total: 1 },
  },
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
        source_id: ids.message,
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
  source_id: ids.message,
  affected_count: 1,
  created_at: updatedAt,
} satisfies WorkbenchMemoryAuditEvent;

export const memoryAuditTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: { events: [visibleMemoryAudit], total: 1 },
  },
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
        source_id: ids.message,
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
  target_id: ids.run,
  operation: 'output',
  source: 'runtime',
  action: 'allow',
  fail_mode: 'closed',
  provider: 'builtin',
  reason_code: '',
  rule_ids: '["rule-1"]',
  created_at: updatedAt,
} satisfies WorkbenchGuardrailAuditEvent;

export const guardrailAuditTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: { events: [visibleGuardrailAudit], total: 1 },
  },
  canonical: {
    events: [
      {
        event_id: ids.guardrailAudit,
        thread_id: ids.thread,
        run_id: ids.run,
        actor_id: ids.actor,
        event_type: 'guardrail.evaluated',
        target_type: 'run',
        target_id: ids.run,
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
  server_id: ids.server,
  runtime_tool_name: 'search',
  event_type: 'tool.completed',
  error_code: '',
  elapsed_millis: 25,
  output_bytes: 64,
  created_at: updatedAt,
} satisfies WorkbenchMCPRuntimeAuditEvent;

export const mcpRuntimeAuditTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: { events: [visibleMCPAudit], total: 1 },
  },
  canonical: {
    events: [
      {
        event_id: ids.mcpAudit,
        thread_id: ids.thread,
        run_id: ids.run,
        server_id: ids.server,
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

export const humanInteractionTransportFixture = pairFixture({
  v1: {
    schema: 'human_interaction_v1',
    interaction_id: ids.interaction,
    kind: 'confirmation',
    decision: 'approve',
    comment: 'Proceed',
    submitted_by: ids.actor,
    submitted_at: updatedAt,
    source: 'workbench',
  },
  canonical: {
    schema: 'human_interaction_v1',
    interaction_id: ids.interaction,
    kind: 'confirmation',
    decision: 'approve',
    comment: 'Proceed',
    submitted_by: ids.actor,
    submitted_at: updatedAtISO,
    source: 'workbench',
  },
  visible: {
    schema: 'human_interaction_v1',
    interaction_id: ids.interaction,
    kind: 'confirmation',
    decision: 'approve',
    comment: 'Proceed',
    submitted_by: ids.actor,
    submitted_at: updatedAt,
    source: 'workbench',
  } satisfies HumanInteractionResponse,
});

export const pairedTransportFixtures = freezeLegacyTaskThreadInput({
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

export type TransportFixtureFamily = keyof typeof pairedTransportFixtures;

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

const asID = (value: unknown, label: string): string => {
  if (typeof value === 'string' && /^(?:0|[1-9]\d*)$/.test(value)) {
    return value;
  }
  if (typeof value === 'number' && Number.isSafeInteger(value) && value >= 0) {
    return String(value);
  }
  throw new TypeError(`${label} must be a decimal string or safe integer ID`);
};

const asString = (value: unknown, label: string): string => {
  if (typeof value !== 'string') {
    throw new TypeError(`${label} must be a string`);
  }
  return value;
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
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

const asEpoch = (value: unknown, label: string): number => {
  if (typeof value === 'number' && Number.isSafeInteger(value) && value >= 0) {
    return value;
  }
  const milliseconds =
    typeof value === 'string' && rfc3339.test(value)
      ? Date.parse(value)
      : Number.NaN;
  if (Number.isSafeInteger(milliseconds) && milliseconds >= 0) {
    return milliseconds;
  }
  throw new TypeError(
    `${label} must be safe epoch milliseconds or valid RFC3339`,
  );
};

const parseJSON = (value: unknown, label: string): unknown => {
  if (typeof value !== 'string') {
    return value;
  }
  try {
    return JSON.parse(value) as unknown;
  } catch (error) {
    throw new TypeError(`${label} must contain valid JSON: ${String(error)}`);
  }
};

const serializeJSON = (value: unknown, label: string): string => {
  const serialized = JSON.stringify(value);
  if (typeof serialized !== 'string') {
    throw new TypeError(`${label} must be JSON serializable`);
  }
  return serialized;
};

const asJSONObject = (value: unknown, label: string): string =>
  serializeJSON(asRecord(parseJSON(value, label), label), label);

const asJSONArray = (value: unknown, label: string): string =>
  serializeJSON(asArray(parseJSON(value, label), label), label);

const asStringArray = (value: unknown, label: string): string[] =>
  asArray(value, label).map((item, index) =>
    asString(item, `${label}[${index}]`),
  );

const asSingletonStringArray = (value: unknown, label: string): string[] => [
  asString(value, label),
];

const exact =
  (expected: unknown): WireDecoder =>
  (value, label) => {
    if (value !== expected) {
      throw new TypeError(`${label} must equal ${String(expected)}`);
    }
    return value;
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
  omit: value =>
    value === null || value === undefined || !!options.omit?.(value),
});
const fixed = (constant: unknown): FieldRule => ({ constant });
const fields = <Schema extends FieldMap>(schema: Schema): Schema => schema;

const id = (from: string): ReadRule => read(from, asID);
const string = (from: string): ReadRule => read(from, asString);
const boolean = (from: string): ReadRule => read(from, asBoolean);
const number = (from: string): ReadRule => read(from, asFiniteNumber);
const integer = (from: string): ReadRule => read(from, asSafeInteger);
const epoch = (from: string): ReadRule => read(from, asEpoch);
const jsonObject = (from: string): ReadRule => read(from, asJSONObject);
const jsonArray = (from: string): ReadRule => read(from, asJSONArray);
const optionalID = (from: string): ReadRule => optionalRead(from, asID);
const optionalLegacyID = (from: string): ReadRule =>
  optionalRead(from, asID, { omit: value => value === '' });
const optionalString = (from: string): ReadRule => optionalRead(from, asString);
const optionalEpoch = (from: string): ReadRule => optionalRead(from, asEpoch);
const optionalLegacyEpoch = (from: string): ReadRule =>
  optionalRead(from, asEpoch, { omit: value => value === 0 });

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

const v1Projector = (...[family, root, schema, checks = []]: ProjectorArgs) =>
  createProjector(`v1.${family}`, root, schema, [
    read('code', exact(0)),
    string('msg'),
    ...checks,
  ]);

const canonicalProjector = (
  ...[family, root, schema, checks = []]: ProjectorArgs
) => createProjector(`canonical.${family}`, root, schema, checks);

const v1TodoFields = fields({
  id: id('id'),
  title: string('title'),
  status: string('status'),
});
const canonicalTodoFields = fields({
  id: id('id'),
  title: string('title'),
  status: string('status'),
});

const v1ThreadFields = fields({
  thread_id: id('thread_id'),
  space_id: id('space_id'),
  title: string('title'),
  status: string('status'),
  source: string('source'),
  progress: number('progress'),
  last_user_message: string('last_user_message'),
  last_agent_message: string('last_agent_message'),
  can_edit: fixed(true),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
  values: read(
    'values',
    objectOf({ todos: read('todos', arrayOf(v1TodoFields)) }),
  ),
});
const canonicalThreadFields = fields({
  thread_id: id('thread_id'),
  space_id: fixed(fixtureSpaceID),
  title: string('metadata.title'),
  status: string('status'),
  source: string('coze.source'),
  progress: number('coze.progress'),
  last_user_message: string('coze.last_user_message'),
  last_agent_message: string('coze.last_agent_message'),
  can_edit: boolean('coze.can_edit'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
  values: read(
    'values',
    objectOf({ todos: read('todos', arrayOf(canonicalTodoFields)) }),
  ),
});

const v1MessageFields = fields({
  message_id: id('message_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  role: string('role'),
  content: string('content'),
  metadata: jsonObject('metadata'),
  created_at: epoch('created_at'),
});
const canonicalMessageFields = fields({
  message_id: id('message_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  role: string('role'),
  content: string('content'),
  metadata: jsonObject('metadata'),
  created_at: epoch('created_at'),
});

const v1RunFields = fields({
  run_id: id('run_id'),
  thread_id: id('thread_id'),
  space_id: id('space_id'),
  assistant_id: id('assistant_id'),
  status: string('status'),
  metadata: jsonObject('metadata'),
  multitask_strategy: string('multitask_strategy'),
  attempt_kind: fixed('initial'),
  parent_run_id: optionalLegacyID('parent_run_id'),
  run_kind: string('run_kind'),
  stream_modes: read('stream_mode', asSingletonStringArray),
  on_disconnect: string('on_disconnect'),
  durability: string('durability'),
  started_at: optionalLegacyEpoch('started_at'),
  ended_at: optionalLegacyEpoch('ended_at'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
});
const canonicalRunFields = fields({
  run_id: id('run_id'),
  thread_id: id('thread_id'),
  space_id: fixed(fixtureSpaceID),
  assistant_id: id('assistant_id'),
  status: string('status'),
  metadata: jsonObject('metadata'),
  multitask_strategy: string('multitask_strategy'),
  message_id: optionalID('coze.message_id'),
  attempt_kind: string('coze.attempt_kind'),
  source_run_id: optionalID('coze.source_run_id'),
  parent_run_id: optionalID('coze.parent_run_id'),
  run_kind: string('coze.run_kind'),
  stream_modes: read('coze.stream_modes', asStringArray),
  on_disconnect: string('coze.on_disconnect'),
  durability: string('coze.durability'),
  terminal_reason: optionalString('coze.terminal_reason'),
  started_at: optionalEpoch('coze.started_at'),
  ended_at: optionalEpoch('coze.ended_at'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
});

const v1RunEventFields = fields({
  event_id: id('event_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  event_type: string('event_type'),
  payload: jsonObject('payload'),
  created_at: epoch('created_at'),
});
const canonicalRunEventFields = fields({
  event_id: id('event_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  event_type: string('event_type'),
  payload: jsonObject('payload'),
  created_at: epoch('created_at'),
});

const v1UploadFields = fields({
  file_id: id('file_id'),
  file_name: string('filename'),
  virtual_path: string('virtual_path'),
  content_type: string('content_type'),
  size_bytes: integer('size'),
  created_at: epoch('created_at'),
});
const canonicalUploadFields = fields({
  file_id: id('file_id'),
  file_name: string('file_name'),
  virtual_path: string('virtual_path'),
  content_type: string('content_type'),
  size_bytes: integer('size_bytes'),
  created_at: epoch('created_at'),
});

const v1ArtifactFields = fields({
  artifact_id: id('artifact_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  file_id: id('file_id'),
  title: string('title'),
  artifact_type: string('artifact_type'),
  virtual_path: string('virtual_path'),
  content_type: string('content_type'),
  size_bytes: integer('size_bytes'),
  preview_mode: string('preview_mode'),
  metadata: jsonObject('metadata'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
  deleted_at: optionalLegacyEpoch('deleted_at'),
});
const canonicalArtifactFields = fields({
  artifact_id: id('artifact_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  file_id: id('file_id'),
  title: string('title'),
  artifact_type: string('artifact_type'),
  virtual_path: string('virtual_path'),
  content_type: string('content_type'),
  size_bytes: integer('size_bytes'),
  preview_mode: string('preview_mode'),
  metadata: jsonObject('metadata'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
  deleted_at: optionalEpoch('deleted_at'),
});

const v1ScanFields = fields({
  job_id: id('job_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  space_id: id('space_id'),
  artifact_id: id('artifact_id'),
  file_id: id('file_id'),
  scanner: string('scanner'),
  status: string('status'),
  worker_id: id('worker_id'),
  attempt_count: integer('attempt_count'),
  error_code: string('last_error'),
  available_at: optionalLegacyEpoch('available_at'),
  started_at: optionalLegacyEpoch('started_at'),
  ended_at: optionalLegacyEpoch('ended_at'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
});
const canonicalScanFields = fields({
  job_id: id('job_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  space_id: fixed(fixtureSpaceID),
  artifact_id: id('artifact_id'),
  file_id: id('file_id'),
  scanner: string('scanner'),
  status: string('status'),
  worker_id: id('worker_ref'),
  attempt_count: integer('attempt_count'),
  error_code: string('error_code'),
  available_at: optionalEpoch('available_at'),
  started_at: optionalEpoch('started_at'),
  ended_at: optionalEpoch('ended_at'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
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
const runAggregateFields = (): FieldMap =>
  fields({
    run_id: id('run_id'),
    aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  });

const v1TokenFields = fields({
  usage_id: id('usage_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  space_id: id('space_id'),
  source: string('source'),
  step_id: id('step_id'),
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
  created_at: epoch('created_at'),
});
const canonicalTokenFields = fields({
  usage_id: id('usage_id'),
  thread_id: id('thread_id'),
  run_id: id('run_id'),
  space_id: fixed(fixtureSpaceID),
  source: string('source'),
  step_id: id('step_id'),
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
  created_at: epoch('created_at'),
});
const v1TokenPageFields: FieldMap = {
  items: read('usage', arrayOf(v1TokenFields)),
  total: integer('total'),
  has_more: fixed(false),
  aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  run_aggregates: read('run_aggregates', arrayOf(runAggregateFields())),
};
const canonicalTokenPageFields: FieldMap = {
  items: read('usage', arrayOf(canonicalTokenFields)),
  total: integer('total'),
  has_more: boolean('has_more'),
  aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  run_aggregates: read('run_aggregates', arrayOf(runAggregateFields())),
};

const v1MemoryFields = fields({
  memory_id: id('memory_id'),
  thread_id: id('thread_id'),
  run_id: optionalID('run_id'),
  space_id: id('space_id'),
  scope: string('scope'),
  content: string('content'),
  metadata: jsonObject('metadata'),
  score: number('score'),
  confidence: number('confidence'),
  source_type: string('source_type'),
  source_id: id('source_id'),
  correction_of_memory_id: optionalLegacyID('correction_of_memory_id'),
  corrected_at: optionalLegacyEpoch('corrected_at'),
  expires_at: optionalLegacyEpoch('expires_at'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
  deleted_at: optionalLegacyEpoch('deleted_at'),
});
const canonicalMemoryFields = fields({
  memory_id: id('memory_id'),
  thread_id: id('thread_id'),
  run_id: optionalID('run_id'),
  space_id: fixed(fixtureSpaceID),
  scope: string('scope'),
  content: string('content'),
  metadata: jsonObject('metadata'),
  score: number('score'),
  confidence: number('confidence'),
  source_type: string('source_type'),
  source_id: id('source_id'),
  correction_of_memory_id: optionalID('correction_of_memory_id'),
  corrected_at: optionalEpoch('corrected_at'),
  expires_at: optionalEpoch('expires_at'),
  created_at: epoch('created_at'),
  updated_at: epoch('updated_at'),
  deleted_at: optionalEpoch('deleted_at'),
});

const auditBaseFields = (space: FieldRule): FieldMap =>
  fields({
    event_id: id('event_id'),
    thread_id: id('thread_id'),
    run_id: optionalID('run_id'),
    space_id: space,
    created_at: epoch('created_at'),
  });
const memoryAuditFields = (space: FieldRule): FieldMap =>
  fields({
    ...auditBaseFields(space),
    memory_id: optionalID('memory_id'),
    actor_id: optionalID('actor_id'),
    event_type: string('event_type'),
    scope: string('scope'),
    source_type: string('source_type'),
    source_id: id('source_id'),
    affected_count: integer('affected_count'),
  });
const guardrailAuditFields = (space: FieldRule): FieldMap =>
  fields({
    ...auditBaseFields(space),
    actor_id: optionalID('actor_id'),
    event_type: string('event_type'),
    target_type: string('target_type'),
    target_id: id('target_id'),
    operation: string('operation'),
    source: string('source'),
    action: string('action'),
    fail_mode: string('fail_mode'),
    provider: string('provider'),
    reason_code: string('reason_code'),
    rule_ids: jsonArray('rule_ids'),
  });
const mcpAuditFields = (space: FieldRule): FieldMap =>
  fields({
    ...auditBaseFields(space),
    server_id: optionalID('server_id'),
    runtime_tool_name: string('runtime_tool_name'),
    event_type: string('event_type'),
    error_code: string('error_code'),
    elapsed_millis: integer('elapsed_millis'),
    output_bytes: integer('output_bytes'),
  });

const v1HumanInteractionFields = fields({
  schema: string('schema'),
  interaction_id: id('interaction_id'),
  kind: string('kind'),
  decision: string('decision'),
  answer: optionalString('answer'),
  choice_id: optionalID('choice_id'),
  comment: optionalString('comment'),
  submitted_by: optionalID('submitted_by'),
  submitted_at: optionalEpoch('submitted_at'),
  source: optionalString('source'),
});
const canonicalHumanInteractionFields = fields({
  schema: string('schema'),
  interaction_id: id('interaction_id'),
  kind: string('kind'),
  decision: string('decision'),
  answer: optionalString('answer'),
  choice_id: optionalID('choice_id'),
  comment: optionalString('comment'),
  submitted_by: optionalID('submitted_by'),
  submitted_at: optionalEpoch('submitted_at'),
  source: optionalString('source'),
});

const v1PrivateDropChecks = [
  jsonObject('data.usage.*.raw_usage'),
  jsonObject('data.usage.*.metadata'),
];
const v1RunEventDropChecks = [
  id('data.journal_messages.*.id'),
  jsonObject('data.journal_messages.*.tool_calls.*.arguments'),
  jsonObject('data.journal_messages.*.usage'),
];
const v1ThreadPrivateDropChecks = [id('data.creator_id')];
const v1RunPrivateDropChecks = [
  id('data.creator_id'),
  string('data.command'),
  jsonObject('data.input'),
  jsonObject('data.config'),
  jsonObject('data.context'),
  id('data.worker_id'),
  string('data.error_code'),
  string('data.error_message'),
];
const canonicalThreadShapeChecks = [
  string('coze.product_status'),
  string('coze.initial_submission'),
  read('interrupts', asArray),
];

type ProjectorPair = readonly [
  v1: TransportFixtureProjector,
  canonical: TransportFixtureProjector,
];

const v1TotalCheck = [integer('data.total')];
const canonicalPageChecks = [integer('total'), boolean('has_more')];
type PagedPairArgs = [
  family: string,
  v1Root: string,
  canonicalRoot: string,
  v1Fields: FieldMap,
  canonicalFields: FieldMap,
];
const pagedPair = (
  ...[family, v1Root, canonicalRoot, v1Fields, canonicalFields]: PagedPairArgs
): ProjectorPair => [
  v1Projector(family, v1Root, v1Fields, v1TotalCheck),
  canonicalProjector(
    family,
    canonicalRoot,
    canonicalFields,
    canonicalPageChecks,
  ),
];

const transportProjectors: Record<TransportFixtureFamily, ProjectorPair> = {
  thread: [
    v1Projector('thread', 'data', v1ThreadFields, v1ThreadPrivateDropChecks),
    canonicalProjector(
      'thread',
      '',
      canonicalThreadFields,
      canonicalThreadShapeChecks,
    ),
  ],
  todo: [
    v1Projector('todo', 'data', v1TodoFields),
    canonicalProjector('todo', 'values.todos.0', canonicalTodoFields),
  ],
  message: [
    v1Projector('message', 'data', v1MessageFields),
    canonicalProjector('message', '', canonicalMessageFields),
  ],
  run: [
    v1Projector('run', 'data', v1RunFields, v1RunPrivateDropChecks),
    canonicalProjector('run', '', canonicalRunFields),
  ],
  run_event: [
    v1Projector('run_event', 'data.events.0', v1RunEventFields, [
      ...v1TotalCheck,
      ...v1RunEventDropChecks,
    ]),
    canonicalProjector('run_event', 'data.0', canonicalRunEventFields, [
      boolean('has_more'),
    ]),
  ],
  upload: [
    v1Projector('upload', 'data.files.0', v1UploadFields, [
      read('data.success', exact(true)),
      string('data.message'),
      read('data.skipped_files', asArray),
      string('data.files.0.path'),
    ]),
    canonicalProjector('upload', 'uploads.0', canonicalUploadFields, [
      read('skipped_files', asArray),
    ]),
  ],
  artifact: pagedPair(
    'artifact',
    'data.artifacts.0',
    'artifacts.0',
    v1ArtifactFields,
    canonicalArtifactFields,
  ),
  artifact_scan_job: pagedPair(
    'artifact_scan_job',
    'data.jobs.0',
    'jobs.0',
    v1ScanFields,
    canonicalScanFields,
  ),
  token_usage: [
    v1Projector('token_usage', 'data', v1TokenPageFields, v1PrivateDropChecks),
    canonicalProjector('token_usage', '', canonicalTokenPageFields),
  ],
  memory: pagedPair(
    'memory',
    'data.memories.0',
    'memories.0',
    v1MemoryFields,
    canonicalMemoryFields,
  ),
  memory_audit: pagedPair(
    'memory_audit',
    'data.events.0',
    'events.0',
    memoryAuditFields(read('space_id', asID)),
    memoryAuditFields(fixed(fixtureSpaceID)),
  ),
  guardrail_audit: pagedPair(
    'guardrail_audit',
    'data.events.0',
    'events.0',
    guardrailAuditFields(read('space_id', asID)),
    guardrailAuditFields(fixed(fixtureSpaceID)),
  ),
  mcp_runtime_audit: pagedPair(
    'mcp_runtime_audit',
    'data.events.0',
    'events.0',
    mcpAuditFields(read('space_id', asID)),
    mcpAuditFields(fixed(fixtureSpaceID)),
  ),
  human_interaction: [
    createProjector('v1.human_interaction', '', v1HumanInteractionFields),
    canonicalProjector(
      'human_interaction',
      '',
      canonicalHumanInteractionFields,
    ),
  ],
};

export const projectV1TransportFixture = (
  family: TransportFixtureFamily,
  wire: unknown,
): unknown => transportProjectors[family][0](wire);

export const projectCanonicalTransportFixture = (
  family: TransportFixtureFamily,
  wire: unknown,
): unknown => transportProjectors[family][1](wire);
