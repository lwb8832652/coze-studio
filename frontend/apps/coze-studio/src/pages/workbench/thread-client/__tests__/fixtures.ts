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
      files: [
        {
          file_id: 'file-1',
          file_name: 'brief.md',
          virtual_path: '/brief.md',
          content_type: 'text/markdown',
          size_bytes: 128,
          created_at: createdAt,
        },
      ],
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
