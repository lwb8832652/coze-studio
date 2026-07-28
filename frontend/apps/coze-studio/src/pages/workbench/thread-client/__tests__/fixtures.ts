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
const createdAtISO = '2026-01-01T00:00:00.000Z';
const updatedAtISO = '2026-01-01T00:01:00.000Z';

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
      thread_id: 'thread-1',
      space_id: 'space-1',
      creator_id: 'user-1',
      title: 'Prepare launch brief',
      status: 'running',
      source: 'agent',
      progress: 40,
      last_user_message: 'Prepare the launch brief',
      last_agent_message: 'Drafting the brief',
      created_at: createdAt,
      updated_at: updatedAt,
      values: {
        todos: [{ id: 'todo-1', title: 'Draft outline', status: 'completed' }],
      },
    },
  },
  canonical: {
    thread_id: 'thread-1',
    created_at: createdAtISO,
    updated_at: updatedAtISO,
    metadata: {},
    status: 'running',
    values: {
      todos: [{ id: 'todo-1', title: 'Draft outline', status: 'completed' }],
    },
    interrupts: [],
    coze: {
      creator_id: 'user-1',
      title: 'Prepare launch brief',
      source: 'agent',
      progress: 40,
      last_user_message: 'Prepare the launch brief',
      last_agent_message: 'Drafting the brief',
    },
  },
  visible: {
    thread_id: 'thread-1',
    space_id: 'space-1',
    creator_id: 'user-1',
    title: 'Prepare launch brief',
    status: 'running',
    source: 'agent',
    progress: 40,
    last_user_message: 'Prepare the launch brief',
    last_agent_message: 'Drafting the brief',
    created_at: createdAt,
    updated_at: updatedAt,
    values: {
      todos: [{ id: 'todo-1', title: 'Draft outline', status: 'completed' }],
    },
  } satisfies WorkbenchThread,
});

export const todoTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: { id: 'todo-1', title: 'Draft outline', status: 'completed' },
  },
  canonical: {
    values: {
      todos: [{ id: 'todo-1', title: 'Draft outline', status: 'completed' }],
    },
  },
  visible: {
    id: 'todo-1',
    title: 'Draft outline',
    status: 'completed',
  } satisfies WorkbenchTodo,
});

export const messageTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      message_id: 'message-1',
      thread_id: 'thread-1',
      run_id: 'run-1',
      role: 'assistant',
      content: 'Drafting the brief',
      metadata: '{"channel":"workbench"}',
      created_at: updatedAt,
    },
  },
  canonical: {
    message_id: 'message-1',
    thread_id: 'thread-1',
    run_id: 'run-1',
    role: 'assistant',
    content: 'Drafting the brief',
    metadata: { channel: 'workbench' },
    created_at: updatedAtISO,
    seq: '2',
  },
  visible: {
    message_id: 'message-1',
    thread_id: 'thread-1',
    run_id: 'run-1',
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
      run_id: 'run-1',
      thread_id: 'thread-1',
      parent_run_id: '',
      space_id: 'space-1',
      creator_id: 'user-1',
      assistant_id: 'assistant-1',
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
      worker_id: 'worker-safe-1',
      error_code: '',
      error_message: '',
      started_at: createdAt,
      ended_at: 0,
      created_at: createdAt,
      updated_at: updatedAt,
    },
  },
  canonical: {
    run_id: 'run-1',
    thread_id: 'thread-1',
    assistant_id: 'assistant-1',
    status: 'running',
    created_at: createdAtISO,
    updated_at: updatedAtISO,
    metadata: { mode: 'agent' },
    multitask_strategy: 'reject',
    coze: {
      parent_run_id: '',
      creator_id: 'user-1',
      run_kind: 'agent',
      command: '',
      input: { message: 'Prepare the launch brief' },
      config: { model_name: 'gpt-test' },
      context: {},
      stream_mode: 'events',
      on_disconnect: 'continue',
      durability: 'async',
      worker_ref: 'worker-safe-1',
      error_code: '',
      error_message: '',
      started_at: createdAtISO,
      ended_at: null,
    },
  },
  visible: {
    run_id: 'run-1',
    thread_id: 'thread-1',
    parent_run_id: '',
    space_id: 'space-1',
    creator_id: 'user-1',
    assistant_id: 'assistant-1',
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
    worker_id: 'worker-safe-1',
    error_code: '',
    error_message: '',
    started_at: createdAt,
    ended_at: 0,
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
          event_id: 'event-1',
          thread_id: 'thread-1',
          run_id: 'run-1',
          event_type: 'run.started',
          payload: '{"phase":"started"}',
          created_at: createdAt,
        },
      ],
      total: 1,
      journal_messages: [
        {
          id: 'private-journal-1',
          tool_calls: [{ arguments: '{"must":"be dropped"}' }],
          usage: '{"must":"be dropped"}',
        },
      ],
    },
  },
  canonical: {
    data: [
      {
        event_id: 'event-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        event_type: 'run.started',
        payload: { phase: 'started' },
        created_at: createdAtISO,
      },
    ],
    has_more: false,
  },
  visible: {
    event_id: 'event-1',
    thread_id: 'thread-1',
    run_id: 'run-1',
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
          file_id: 'file-1',
          filename: 'brief.md',
          path: '/uploads/file-1',
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
        file_id: 'file-1',
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
    file_id: 'file-1',
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
          artifact_id: 'artifact-1',
          thread_id: 'thread-1',
          run_id: 'run-1',
          file_id: 'file-1',
          title: 'Launch brief',
          artifact_type: 'document',
          virtual_path: '/brief.md',
          content_type: 'text/markdown',
          size_bytes: 128,
          preview_mode: 'text',
          metadata: '{"scan_status":"clean"}',
          created_at: createdAt,
          updated_at: updatedAt,
          deleted_at: 0,
        },
      ],
      total: 1,
    },
  },
  canonical: {
    artifacts: [
      {
        artifact_id: 'artifact-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        file_id: 'file-1',
        title: 'Launch brief',
        artifact_type: 'document',
        virtual_path: '/brief.md',
        content_type: 'text/markdown',
        size_bytes: 128,
        preview_mode: 'text',
        metadata: { scan_status: 'clean' },
        created_at: createdAtISO,
        updated_at: updatedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: {
    artifact_id: 'artifact-1',
    thread_id: 'thread-1',
    run_id: 'run-1',
    file_id: 'file-1',
    title: 'Launch brief',
    artifact_type: 'document',
    virtual_path: '/brief.md',
    content_type: 'text/markdown',
    size_bytes: 128,
    preview_mode: 'text',
    metadata: '{"scan_status":"clean"}',
    created_at: createdAt,
    updated_at: updatedAt,
    deleted_at: 0,
  } satisfies WorkbenchArtifact,
});

export const artifactScanJobTransportFixture = pairFixture({
  v1: {
    code: 0,
    msg: 'success',
    data: {
      jobs: [
        {
          job_id: 'scan-1',
          thread_id: 'thread-1',
          run_id: 'run-1',
          space_id: 'space-1',
          artifact_id: 'artifact-1',
          file_id: 'file-1',
          scanner: 'default',
          status: 'succeeded',
          worker_id: 'worker-safe-1',
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
        job_id: 'scan-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        artifact_id: 'artifact-1',
        file_id: 'file-1',
        scanner: 'default',
        status: 'succeeded',
        worker_ref: 'worker-safe-1',
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
    job_id: 'scan-1',
    thread_id: 'thread-1',
    run_id: 'run-1',
    space_id: 'space-1',
    artifact_id: 'artifact-1',
    file_id: 'file-1',
    scanner: 'default',
    status: 'succeeded',
    worker_id: 'worker-safe-1',
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
  usage_id: 'usage-1',
  thread_id: 'thread-1',
  run_id: 'run-1',
  space_id: 'space-1',
  source: 'lead_agent',
  step_id: 'step-1',
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
      run_aggregates: [{ run_id: 'run-1', aggregate: tokenAggregate }],
    },
  },
  canonical: {
    usage: [
      {
        usage_id: 'usage-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        source: 'lead_agent',
        step_id: 'step-1',
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
    run_aggregates: [{ run_id: 'run-1', aggregate: tokenAggregate }],
  },
  visible: {
    items: [visibleTokenUsage],
    total: 1,
    has_more: false,
    aggregate: tokenAggregate,
    run_aggregates: [{ run_id: 'run-1', aggregate: tokenAggregate }],
  } satisfies WorkbenchTokenUsageResult,
});

const visibleMemory = {
  memory_id: 'memory-1',
  thread_id: 'thread-1',
  run_id: 'run-1',
  space_id: 'space-1',
  scope: 'thread',
  content: 'Launch date is Friday',
  metadata: '{"kind":"fact"}',
  score: 0.9,
  confidence: 0.95,
  source_type: 'message',
  source_id: 'message-1',
  correction_of_memory_id: '',
  corrected_at: 0,
  expires_at: 0,
  created_at: createdAt,
  updated_at: updatedAt,
  deleted_at: 0,
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
        memory_id: 'memory-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        scope: 'thread',
        content: 'Launch date is Friday',
        metadata: { kind: 'fact' },
        score: 0.9,
        confidence: 0.95,
        source_type: 'message',
        source_id: 'message-1',
        created_at: createdAtISO,
        updated_at: updatedAtISO,
      },
    ],
    total: 1,
    has_more: false,
  },
  visible: visibleMemory,
});

const visibleMemoryAudit = {
  event_id: 'memory-audit-1',
  thread_id: 'thread-1',
  run_id: 'run-1',
  space_id: 'space-1',
  memory_id: 'memory-1',
  actor_id: 'user-1',
  event_type: 'memory.updated',
  scope: 'thread',
  source_type: 'message',
  source_id: 'message-1',
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
        event_id: 'memory-audit-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        memory_id: 'memory-1',
        actor_id: 'user-1',
        event_type: 'memory.updated',
        scope: 'thread',
        source_type: 'message',
        source_id: 'message-1',
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
  event_id: 'guardrail-audit-1',
  thread_id: 'thread-1',
  run_id: 'run-1',
  space_id: 'space-1',
  actor_id: 'user-1',
  event_type: 'guardrail.evaluated',
  target_type: 'run',
  target_id: 'run-1',
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
        event_id: 'guardrail-audit-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        actor_id: 'user-1',
        event_type: 'guardrail.evaluated',
        target_type: 'run',
        target_id: 'run-1',
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
  event_id: 'mcp-audit-1',
  space_id: 'space-1',
  thread_id: 'thread-1',
  run_id: 'run-1',
  server_id: 'server-1',
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
        event_id: 'mcp-audit-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        server_id: 'server-1',
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
    interaction_id: 'interaction-1',
    kind: 'confirmation',
    decision: 'approve',
    comment: 'Proceed',
    submitted_by: 'user-1',
    submitted_at: updatedAt,
    source: 'workbench',
  },
  canonical: {
    schema: 'human_interaction_v1',
    interaction_id: 'interaction-1',
    kind: 'confirmation',
    decision: 'approve',
    comment: 'Proceed',
    submitted_by: 'user-1',
    submitted_at: updatedAtISO,
    source: 'workbench',
  },
  visible: {
    schema: 'human_interaction_v1',
    interaction_id: 'interaction-1',
    kind: 'confirmation',
    decision: 'approve',
    comment: 'Proceed',
    submitted_by: 'user-1',
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
}
type FieldRule = ReadRule | { constant: unknown };
type FieldMap = Record<string, FieldRule>;
type TransportFixtureProjector = (wire: unknown) => unknown;

const fixtureSpaceID = 'space-1';
const identity: WireDecoder = value => value;

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
  if (!['string', 'number', 'bigint'].includes(typeof value)) {
    throw new TypeError(`${label} must be an ID`);
  }
  return String(value);
};

const rfc3339 =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

const asEpoch = (value: unknown, label: string): number => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  const milliseconds =
    typeof value === 'string' && rfc3339.test(value)
      ? Date.parse(value)
      : Number.NaN;
  if (Number.isFinite(milliseconds)) {
    return milliseconds;
  }
  throw new TypeError(`${label} must be finite epoch milliseconds or RFC3339`);
};

const asJSON = (value: unknown, label: string): string => {
  if (typeof value === 'string') {
    return value;
  }
  const serialized = JSON.stringify(value);
  if (serialized === undefined) {
    throw new TypeError(`${label} must be JSON serializable`);
  }
  return serialized;
};

const exact =
  (expected: unknown): WireDecoder =>
  (value, label) => {
    if (value !== expected) {
      throw new TypeError(`${label} must equal ${String(expected)}`);
    }
    return value;
  };

const nullish =
  (decode: WireDecoder, fallback: unknown): WireDecoder =>
  (value, label) =>
    value === null || value === undefined ? fallback : decode(value, label);

const read = (from: string, decode: WireDecoder = identity): ReadRule => ({
  from,
  decode,
});
const fixed = (constant: unknown): FieldRule => ({ constant });

const fieldMap = (declaration: string, explicit: FieldMap = {}): FieldMap => {
  const fields = { ...explicit };
  const decoders: Record<string, WireDecoder> = {
    ids: asID,
    plain: identity,
    epochs: asEpoch,
    json: asJSON,
  };
  declaration.split(';').forEach(group => {
    const [kind, paths] = group.trim().split(':');
    paths.split(/\s+/).forEach(path => {
      const [name, from = name] = path.split('=');
      const decode = decoders[kind];
      if (!decode) {
        throw new TypeError(`unknown field decoder ${kind}`);
      }
      fields[name] = read(from, decode);
    });
  });
  return fields;
};

const readPath = (source: unknown, path: string, label: string): unknown[] =>
  path
    .split('.')
    .filter(Boolean)
    .reduce<unknown[]>(
      (values, segment) => {
        if (segment === '*') {
          return values.flatMap((value, index) =>
            asArray(value, `${label}[${index}]`),
          );
        }
        return values.map(value => {
          if (Array.isArray(value) && /^\d+$/.test(segment)) {
            const item = value[Number(segment)];
            if (item === undefined) {
              throw new TypeError(`${label}.${segment} is required`);
            }
            return item;
          }
          const record = asRecord(value, label);
          if (!(segment in record)) {
            throw new TypeError(`${label}.${segment} is required`);
          }
          return record[segment];
        });
      },
      [source],
    );

const projectFields = (
  source: WireRecord,
  schema: FieldMap,
  label: string,
): WireRecord =>
  Object.fromEntries(
    Object.entries(schema).map(([name, rule]) => {
      if ('constant' in rule) {
        return [name, rule.constant];
      }
      const values = readPath(source, rule.from, label);
      if (values.length !== 1) {
        throw new TypeError(`${label}.${rule.from} must resolve once`);
      }
      return [name, rule.decode(values[0], `${label}.${rule.from}`)];
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
      const values = readPath(wire, check.from, label);
      if (!values.length) {
        throw new TypeError(`${label}.${check.from} must contain a value`);
      }
      values.forEach((value, index) =>
        check.decode(value, `${label}.${check.from}[${index}]`),
      );
    });
    const roots = readPath(wire, root, label);
    if (roots.length !== 1) {
      throw new TypeError(`${label}.${root} must resolve once`);
    }
    return projectFields(asRecord(roots[0], label), schema, label);
  };

const v1Projector = (...[family, root, schema, checks = []]: ProjectorArgs) =>
  createProjector(`v1.${family}`, root, schema, [
    read('code', exact(0)),
    read('msg'),
    ...checks,
  ]);

const canonicalProjector = (
  ...[family, root, schema, checks = []]: ProjectorArgs
) => createProjector(`canonical.${family}`, root, schema, checks);

const v1TodoFields = fieldMap('ids:id; plain:title status');
const canonicalTodoFields = fieldMap('ids:id; plain:title status');

const v1ThreadFields = fieldMap(
  'ids:thread_id space_id creator_id; ' +
    'plain:title status source last_user_message last_agent_message progress; ' +
    'epochs:created_at updated_at',
  {
    values: read(
      'values',
      objectOf({ todos: read('todos', arrayOf(v1TodoFields)) }),
    ),
  },
);
const canonicalThreadFields = fieldMap(
  'ids:thread_id creator_id=coze.creator_id; plain:status ' +
    'title=coze.title source=coze.source ' +
    'last_user_message=coze.last_user_message ' +
    'last_agent_message=coze.last_agent_message progress=coze.progress; ' +
    'epochs:created_at updated_at',
  {
    space_id: fixed(fixtureSpaceID),
    values: read(
      'values',
      objectOf({ todos: read('todos', arrayOf(canonicalTodoFields)) }),
    ),
  },
);

const v1MessageFields = fieldMap(
  'ids:message_id thread_id run_id; plain:role content; ' +
    'json:metadata; epochs:created_at',
);
const canonicalMessageFields = fieldMap(
  'ids:message_id thread_id run_id; plain:role content; ' +
    'json:metadata; epochs:created_at',
);

const v1RunFields = fieldMap(
  'ids:run_id thread_id parent_run_id space_id creator_id assistant_id ' +
    'worker_id; plain:run_kind status command stream_mode multitask_strategy ' +
    'on_disconnect durability error_code error_message; ' +
    'json:input config context metadata; ' +
    'epochs:started_at ended_at created_at updated_at',
);
const canonicalRunFields = fieldMap(
  'ids:run_id thread_id assistant_id parent_run_id=coze.parent_run_id ' +
    'creator_id=coze.creator_id worker_id=coze.worker_ref; plain:status ' +
    'multitask_strategy run_kind=coze.run_kind command=coze.command ' +
    'stream_mode=coze.stream_mode on_disconnect=coze.on_disconnect ' +
    'durability=coze.durability error_code=coze.error_code ' +
    'error_message=coze.error_message; json:metadata input=coze.input ' +
    'config=coze.config context=coze.context; ' +
    'epochs:created_at updated_at started_at=coze.started_at',
  {
    space_id: fixed(fixtureSpaceID),
    ended_at: read('coze.ended_at', nullish(asEpoch, 0)),
  },
);

const v1RunEventFields = fieldMap(
  'ids:event_id thread_id run_id; plain:event_type; ' +
    'json:payload; epochs:created_at',
);
const canonicalRunEventFields = fieldMap(
  'ids:event_id thread_id run_id; plain:event_type; ' +
    'json:payload; epochs:created_at',
);

const v1UploadFields = fieldMap(
  'ids:file_id; plain:file_name=filename virtual_path content_type ' +
    'size_bytes=size; epochs:created_at',
);
const canonicalUploadFields = fieldMap(
  'ids:file_id; plain:file_name virtual_path content_type size_bytes; ' +
    'epochs:created_at',
);

const v1ArtifactFields = fieldMap(
  'ids:artifact_id thread_id run_id file_id; ' +
    'plain:title artifact_type virtual_path content_type preview_mode ' +
    'size_bytes; json:metadata; epochs:created_at updated_at deleted_at',
);
const canonicalArtifactFields = fieldMap(
  'ids:artifact_id thread_id run_id file_id; ' +
    'plain:title artifact_type virtual_path content_type preview_mode ' +
    'size_bytes; json:metadata; epochs:created_at updated_at',
  {
    deleted_at: fixed(0),
  },
);

const v1ScanFields = fieldMap(
  'ids:job_id thread_id run_id space_id artifact_id file_id worker_id; ' +
    'plain:scanner status error_code=last_error attempt_count; ' +
    'epochs:available_at started_at ended_at created_at updated_at',
);
const canonicalScanFields = fieldMap(
  'ids:job_id thread_id run_id artifact_id file_id worker_id=worker_ref; ' +
    'plain:scanner status error_code attempt_count; ' +
    'epochs:available_at started_at ended_at created_at updated_at',
  {
    space_id: fixed(fixtureSpaceID),
  },
);

const tokenAggregateFields = (): FieldMap =>
  fieldMap(
    'plain:input_tokens output_tokens total_tokens cost_micros call_count ' +
      'lead_agent_tokens subagent_tokens middleware_tokens tool_tokens',
  );
const runAggregateFields = (): FieldMap => ({
  run_id: read('run_id', asID),
  aggregate: read('aggregate', objectOf(tokenAggregateFields())),
});

const v1TokenFields = fieldMap(
  'ids:usage_id thread_id run_id space_id step_id; ' +
    'plain:source step_name model_name provider currency step_index ' +
    'input_tokens output_tokens total_tokens cost_micros estimated; ' +
    'epochs:created_at',
);
const canonicalTokenFields = fieldMap(
  'ids:usage_id thread_id run_id step_id; ' +
    'plain:source step_name model_name provider currency step_index ' +
    'input_tokens output_tokens total_tokens cost_micros estimated; ' +
    'epochs:created_at',
  {
    space_id: fixed(fixtureSpaceID),
  },
);
const v1TokenPageFields: FieldMap = {
  items: read('usage', arrayOf(v1TokenFields)),
  total: read('total'),
  has_more: fixed(false),
  aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  run_aggregates: read('run_aggregates', arrayOf(runAggregateFields())),
};
const canonicalTokenPageFields: FieldMap = {
  items: read('usage', arrayOf(canonicalTokenFields)),
  total: read('total'),
  has_more: read('has_more'),
  aggregate: read('aggregate', objectOf(tokenAggregateFields())),
  run_aggregates: read('run_aggregates', arrayOf(runAggregateFields())),
};

const v1MemoryFields = fieldMap(
  'ids:memory_id thread_id run_id space_id source_id ' +
    'correction_of_memory_id; plain:scope content source_type score ' +
    'confidence; json:metadata; ' +
    'epochs:corrected_at expires_at created_at updated_at deleted_at',
);
const canonicalMemoryFields = fieldMap(
  'ids:memory_id thread_id run_id source_id; ' +
    'plain:scope content source_type score confidence; json:metadata; ' +
    'epochs:created_at updated_at',
  {
    space_id: fixed(fixtureSpaceID),
    correction_of_memory_id: fixed(''),
    corrected_at: fixed(0),
    expires_at: fixed(0),
    deleted_at: fixed(0),
  },
);

const auditBaseFields = (space: FieldRule): FieldMap =>
  fieldMap('ids:event_id thread_id run_id; epochs:created_at', {
    space_id: space,
  });
const memoryAuditFields = (space: FieldRule): FieldMap =>
  fieldMap(
    'ids:memory_id actor_id source_id; ' +
      'plain:event_type scope source_type affected_count',
    auditBaseFields(space),
  );
const guardrailAuditFields = (space: FieldRule): FieldMap =>
  fieldMap(
    'ids:actor_id target_id; plain:event_type target_type operation source ' +
      'action fail_mode provider reason_code; json:rule_ids',
    auditBaseFields(space),
  );
const mcpAuditFields = (space: FieldRule): FieldMap =>
  fieldMap(
    'ids:server_id; plain:runtime_tool_name event_type error_code ' +
      'elapsed_millis output_bytes',
    auditBaseFields(space),
  );

const v1HumanInteractionFields = fieldMap(
  'ids:interaction_id submitted_by; ' +
    'plain:schema kind decision comment source; epochs:submitted_at',
);
const canonicalHumanInteractionFields = fieldMap(
  'ids:interaction_id submitted_by; ' +
    'plain:schema kind decision comment source; epochs:submitted_at',
);

const v1PrivateDropChecks = [
  read('data.usage.*.raw_usage', asJSON),
  read('data.usage.*.metadata', asJSON),
];
const v1RunEventDropChecks = [
  read('data.journal_messages.*.id', asID),
  read('data.journal_messages.*.tool_calls.*.arguments', asJSON),
  read('data.journal_messages.*.usage', asJSON),
];

type ProjectorPair = readonly [
  v1: TransportFixtureProjector,
  canonical: TransportFixtureProjector,
];

const v1TotalCheck = [read('data.total')];
const canonicalPageChecks = [read('total'), read('has_more')];
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
    v1Projector('thread', 'data', v1ThreadFields),
    canonicalProjector('thread', '', canonicalThreadFields),
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
    v1Projector('run', 'data', v1RunFields),
    canonicalProjector('run', '', canonicalRunFields),
  ],
  run_event: [
    v1Projector('run_event', 'data.events.0', v1RunEventFields, [
      ...v1TotalCheck,
      ...v1RunEventDropChecks,
    ]),
    canonicalProjector('run_event', 'data.0', canonicalRunEventFields, [
      read('has_more'),
    ]),
  ],
  upload: [
    v1Projector('upload', 'data.files.0', v1UploadFields, [
      read('data.success', exact(true)),
      read('data.message'),
      read('data.skipped_files', asArray),
      read('data.files.0.path'),
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
