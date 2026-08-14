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

import { describe, expect, it, vi } from 'vitest';
import type { FetchSteamConfig } from '@coze-arch/fetch-stream';

import type {
  WorkbenchPage,
  WorkbenchTokenUsageResult,
} from '../workbench-thread-client';
import type {
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
  WorkbenchUpload,
} from '../types';
import {
  presentTaskThreadArtifactListResponse,
  presentTaskThreadArtifactScanJobListResponse,
  presentTaskThreadGetResponse,
  presentTaskThreadGuardrailAuditListResponse,
  presentTaskThreadMCPRuntimeAuditListResponse,
  presentTaskThreadMemoryAuditListResponse,
  presentTaskThreadMemoryListResponse,
  presentTaskThreadMessageResponse,
  presentTaskThreadRunEventListResponse,
  presentTaskThreadRunResponse,
  presentTaskThreadListResponse,
  presentTaskThreadTokenUsageResponse,
  presentTaskThreadUploadResponse,
} from '../legacy-page-response';
import {
  CanonicalThreadCoreClient,
  type CanonicalFetchStream,
} from '../canonical-thread-client';
import type { CanonicalFetch } from '../canonical-fetch';
import {
  adaptCanonicalArtifactPage,
  adaptCanonicalArtifactScanJobPage,
  adaptCanonicalGuardrailAuditPage,
  adaptCanonicalMCPRuntimeAuditPage,
  adaptCanonicalMemoryAuditPage,
  adaptCanonicalMemoryPage,
  adaptCanonicalMessage,
  adaptCanonicalRun,
  adaptCanonicalRunEvent,
  adaptCanonicalThread,
  adaptCanonicalThreadList,
  adaptCanonicalTokenUsageResult,
  adaptCanonicalUploadCreation,
} from '../adapters/canonical-thread-adapter';
import {
  artifactScanJobTransportFixture,
  artifactTransportFixture,
  canonicalTransportFixtures,
  guardrailAuditTransportFixture,
  humanInteractionTransportFixture,
  mcpRuntimeAuditTransportFixture,
  memoryAuditTransportFixture,
  memoryTransportFixture,
  messageTransportFixture,
  projectCanonicalTransportFixture,
  runEventTransportFixture,
  runTransportFixture,
  threadTransportFixture,
  todoTransportFixture,
  tokenUsageTransportFixture,
  uploadTransportFixture,
  type TransportFixtureFamily,
} from './fixtures';

const spaceID = '9001';
const threadID = '1001';
const runID = '3001';
const artifactID = '7001';
const memoryID = '8401';
const scope = { spaceId: spaceID, threadId: threadID, runId: runID };

const page = <T>(item: T): WorkbenchPage<T> => ({
  items: [item],
  total: 1,
  has_more: false,
});

const adaptCanonicalVisibleFixture = (
  family: TransportFixtureFamily,
): unknown => {
  switch (family) {
    case 'thread':
      return adaptCanonicalThread(threadTransportFixture.canonical, scope);
    case 'todo':
      // Todo is nested Thread state and Human Interaction is request-only, so
      // their reviewed canonical projectors are the applicable contract edge.
      return projectCanonicalTransportFixture(
        'todo',
        todoTransportFixture.canonical,
      );
    case 'message':
      return adaptCanonicalMessage(messageTransportFixture.canonical, scope);
    case 'run':
      return adaptCanonicalRun(runTransportFixture.canonical, scope);
    case 'run_event':
      return adaptCanonicalRunEvent(
        runEventTransportFixture.canonical.data[0],
        scope,
      );
    case 'upload':
      return adaptCanonicalUploadCreation(uploadTransportFixture.canonical)
        .uploads[0];
    case 'artifact':
      return adaptCanonicalArtifactPage(
        artifactTransportFixture.canonical,
        scope,
      ).items[0];
    case 'artifact_scan_job':
      return adaptCanonicalArtifactScanJobPage(
        artifactScanJobTransportFixture.canonical,
        scope,
      ).items[0];
    case 'token_usage':
      return adaptCanonicalTokenUsageResult(
        tokenUsageTransportFixture.canonical,
        scope,
      );
    case 'memory':
      return adaptCanonicalMemoryPage(memoryTransportFixture.canonical, scope)
        .items[0];
    case 'memory_audit':
      return adaptCanonicalMemoryAuditPage(
        memoryAuditTransportFixture.canonical,
        scope,
      ).items[0];
    case 'guardrail_audit':
      return adaptCanonicalGuardrailAuditPage(
        guardrailAuditTransportFixture.canonical,
        scope,
      ).items[0];
    case 'mcp_runtime_audit':
      return adaptCanonicalMCPRuntimeAuditPage(
        mcpRuntimeAuditTransportFixture.canonical,
        scope,
      ).items[0];
    case 'human_interaction':
      return projectCanonicalTransportFixture(
        'human_interaction',
        humanInteractionTransportFixture.canonical,
      );
    default:
      throw new Error(`Unsupported fixture family: ${String(family)}`);
  }
};

const presentVisibleFixture = (
  family: TransportFixtureFamily,
  value: unknown,
): unknown => {
  switch (family) {
    case 'thread':
      return presentTaskThreadGetResponse(value as WorkbenchThread);
    case 'todo':
      return presentTaskThreadGetResponse({
        ...threadTransportFixture.visible,
        values: { todos: [value as WorkbenchTodo] },
      });
    case 'message':
      return presentTaskThreadMessageResponse(value as WorkbenchMessage);
    case 'run':
      return presentTaskThreadRunResponse(value as WorkbenchRun);
    case 'run_event':
      return presentTaskThreadRunEventListResponse(
        page(value as WorkbenchRunEvent),
      );
    case 'upload':
      return presentTaskThreadUploadResponse({
        uploads: [value as WorkbenchUpload],
        skipped_files: [],
      });
    case 'artifact':
      return presentTaskThreadArtifactListResponse(
        page(value as WorkbenchArtifact),
      );
    case 'artifact_scan_job':
      return presentTaskThreadArtifactScanJobListResponse(
        page(value as WorkbenchArtifactScanJob),
      );
    case 'token_usage':
      return presentTaskThreadTokenUsageResponse(
        value as WorkbenchTokenUsageResult,
      );
    case 'memory':
      return presentTaskThreadMemoryListResponse(
        page(value as WorkbenchMemory),
      );
    case 'memory_audit':
      return presentTaskThreadMemoryAuditListResponse(
        page(value as WorkbenchMemoryAuditEvent),
      );
    case 'guardrail_audit':
      return presentTaskThreadGuardrailAuditListResponse(
        page(value as WorkbenchGuardrailAuditEvent),
      );
    case 'mcp_runtime_audit':
      return presentTaskThreadMCPRuntimeAuditListResponse(
        page(value as WorkbenchMCPRuntimeAuditEvent),
      );
    case 'human_interaction':
      return value;
    default:
      throw new Error(`Unsupported fixture family: ${String(family)}`);
  }
};

const jsonResponse = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });

const cloneRecord = (value: unknown): Record<string, unknown> =>
  structuredClone(value) as Record<string, unknown>;

const atomicThreadWire = (deferred: boolean) => {
  const thread = cloneRecord(threadTransportFixture.canonical);
  const coze = thread.coze as Record<string, unknown>;
  if (deferred) {
    thread.status = 'idle';
    coze.product_status = 'idle';
    coze.initial_submission = null;
  } else {
    coze.initial_submission = {
      message: messageTransportFixture.canonical,
      run: runTransportFixture.canonical,
    };
  }
  return thread;
};

const runCreationWire = (retry = false) => {
  const run = cloneRecord(runTransportFixture.canonical);
  const coze = run.coze as Record<string, unknown>;
  if (retry) {
    coze.attempt_kind = 'retry';
    coze.source_run_id = runID;
  } else {
    coze.message_id = messageTransportFixture.visible.message_id;
    coze.submission_message = messageTransportFixture.canonical;
  }
  return run;
};

type StreamConfig = FetchSteamConfig<unknown>;

const streamHarness = () => {
  let config: StreamConfig | undefined;
  const stream = vi.fn(
    (_input: RequestInfo, nextConfig: StreamConfig): Promise<void> => {
      config = nextConfig;
      return Promise.resolve();
    },
  );
  return {
    stream: stream as unknown as CanonicalFetchStream,
    mock: stream,
    config: () => {
      if (!config) {
        throw new Error('Missing stream configuration');
      }
      return config;
    },
  };
};

const emitStreamFrame = (config: StreamConfig, frame: unknown) => {
  const message = config.streamParser?.(frame as never, {
    terminate: vi.fn(),
    onParseError: vi.fn(),
  });
  if (message !== undefined) {
    config.onMessage?.({ message });
  }
};

describe('canonical client equivalence', () => {
  it('presents every canonical resource as its reviewed visible fixture', () => {
    for (const [family, fixture] of Object.entries(
      canonicalTransportFixtures,
    )) {
      const fixtureFamily = family as TransportFixtureFamily;
      const expected = presentVisibleFixture(fixtureFamily, fixture.visible);

      expect(
        presentVisibleFixture(
          fixtureFamily,
          adaptCanonicalVisibleFixture(fixtureFamily),
        ),
      ).toEqual(expected);
    }
  });

  it('preserves list ordering, pagination totals, and terminal Run states', () => {
    const firstCanonical = cloneRecord(threadTransportFixture.canonical);
    const secondCanonical = cloneRecord(threadTransportFixture.canonical);
    secondCanonical.thread_id = '1002';
    const secondCoze = secondCanonical.coze as Record<string, unknown>;
    secondCoze.last_user_message = 'Second task';

    const canonicalPage = adaptCanonicalThreadList(
      [secondCanonical, firstCanonical],
      spaceID,
      { total: 42, next: '2' },
    );
    const firstVisible = threadTransportFixture.visible;
    const secondVisible = {
      ...firstVisible,
      thread_id: '1002',
      last_user_message: 'Second task',
    };

    expect(presentTaskThreadListResponse(canonicalPage)).toEqual(
      presentTaskThreadListResponse({
        items: [secondVisible, firstVisible],
        total: 42,
        has_more: true,
        next_cursor: '2',
      }),
    );

    for (const status of ['succeeded', 'failed', 'canceled', 'interrupted']) {
      const canonical = cloneRecord(runTransportFixture.canonical);
      canonical.status = status;
      expect(
        presentTaskThreadRunResponse(adaptCanonicalRun(canonical, scope)),
      ).toEqual(
        presentTaskThreadRunResponse({
          ...runTransportFixture.visible,
          status,
        }),
      );
    }
  });

  it('strictly decodes the optional public adaptive execution decision', () => {
    const canonical = cloneRecord(runTransportFixture.canonical);
    const coze = canonical.coze as Record<string, unknown>;
    coze.adaptive_execution = {
      schema: 'coze.adaptive_execution_public.v1',
      enabled: true,
      mode: 'direct',
      safe_summary: '直接回答，无需调用工具。',
      clarification_question: null,
      internal_decision_id: 'must-not-be-visible',
    };

    expect(adaptCanonicalRun(canonical, scope).adaptive_execution).toEqual({
      schema: 'coze.adaptive_execution_public.v1',
      enabled: true,
      mode: 'direct',
      safe_summary: '直接回答，无需调用工具。',
      clarification_question: null,
    });

    for (const invalidDecision of [
      {
        schema: 'coze.adaptive_execution_public.v2',
        enabled: true,
        mode: 'direct',
        safe_summary: '',
        clarification_question: null,
      },
      {
        schema: 'coze.adaptive_execution_public.v1',
        enabled: true,
        mode: 'unreviewed-mode',
        safe_summary: '',
        clarification_question: null,
      },
      {
        schema: 'coze.adaptive_execution_public.v1',
        enabled: 'yes',
        mode: 'single_step',
        safe_summary: '',
        clarification_question: null,
      },
    ]) {
      const invalidCanonical = cloneRecord(runTransportFixture.canonical);
      (invalidCanonical.coze as Record<string, unknown>).adaptive_execution =
        invalidDecision;

      expect(() => adaptCanonicalRun(invalidCanonical, scope)).toThrow(
        'adaptive_execution',
      );
    }
  });

  it('keeps visible errors and AbortError ownership stable', async () => {
    const errorMessage = 'Run is active';
    const canonicalError = new CanonicalThreadCoreClient({
      fetch: vi.fn(() =>
        Promise.resolve(
          jsonResponse(
            {
              detail: errorMessage,
              code: 'run_conflict',
              retryable: false,
              trace_id: 'trace-gate-a',
            },
            409,
          ),
        ),
      ) as CanonicalFetch,
    }).getThread({ space_id: spaceID, thread_id: threadID });

    await expect(canonicalError).rejects.toThrow(errorMessage);

    const abortError = new DOMException('Request aborted', 'AbortError');
    const canonicalAbort = new CanonicalThreadCoreClient({
      fetch: vi.fn(() => Promise.reject(abortError)) as CanonicalFetch,
    }).getThread({
      space_id: spaceID,
      thread_id: threadID,
      signal: new AbortController().signal,
    });

    await expect(canonicalAbort).rejects.toBe(abortError);
  });

  it('uses one canonical request source for every page workflow', async () => {
    const responses = [
      jsonResponse(atomicThreadWire(false)),
      jsonResponse(atomicThreadWire(true)),
      jsonResponse(uploadTransportFixture.canonical),
      jsonResponse(runCreationWire()),
      jsonResponse(runCreationWire()),
      new Response(null, { status: 204 }),
      jsonResponse(runTransportFixture.canonical),
      jsonResponse(runCreationWire(true)),
      jsonResponse(artifactTransportFixture.canonical),
      jsonResponse({
        artifact: artifactTransportFixture.canonical.artifacts[0],
        restored: true,
      }),
      jsonResponse(memoryTransportFixture.canonical),
      jsonResponse({
        memory: memoryTransportFixture.canonical.memories[0],
        updated: true,
      }),
    ];
    const fetchMock = vi.fn(() => {
      const response = responses.shift();
      if (!response) {
        throw new Error('Missing queued response');
      }
      return Promise.resolve(response);
    });
    const stream = streamHarness();
    const client = new CanonicalThreadCoreClient({
      fetch: fetchMock as CanonicalFetch,
      stream: stream.stream,
    });

    await client.createThread({
      space_id: spaceID,
      message: 'Create without files',
    });
    await client.createThread({
      space_id: spaceID,
      message: 'Create with files',
      defer_start: true,
    });
    const uploads = await client.uploadFiles({
      space_id: spaceID,
      thread_id: threadID,
      files: [new File(['brief'], 'brief.md')],
    });
    await client.createRun({
      space_id: spaceID,
      thread_id: threadID,
      input: JSON.stringify({ uploaded_files: uploads.uploads }),
      message_content: 'Create with files',
    });
    await client.createRun({
      space_id: spaceID,
      thread_id: threadID,
      input: '{"message":"Follow up"}',
      message_content: 'Follow up',
    });
    await client.cancelRun({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
    });
    await client.resumeRun({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      interrupt_id: '10001',
      response: humanInteractionTransportFixture.visible,
    });
    await client.retrySubagentRun({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
    });
    await client.listArtifacts({
      space_id: spaceID,
      thread_id: threadID,
    });
    await client.restoreArtifact({
      space_id: spaceID,
      thread_id: threadID,
      artifact_id: artifactID,
    });
    await client.listMemories({
      space_id: spaceID,
      thread_id: threadID,
    });
    await client.updateMemory({
      space_id: spaceID,
      thread_id: threadID,
      memory_id: memoryID,
      scope: 'thread',
      content: 'Launch date is Friday',
    });

    const callbacks = {
      onEvent: vi.fn(),
      onEnd: vi.fn(),
      onError: vi.fn(),
    };
    const subscription = client.subscribeRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      signal: new AbortController().signal,
      ...callbacks,
    });
    emitStreamFrame(stream.config(), {
      type: 'event',
      event: 'events',
      id: runEventTransportFixture.visible.event_id,
      data: JSON.stringify(runEventTransportFixture.canonical.data[0]),
    });
    emitStreamFrame(stream.config(), {
      type: 'event',
      event: 'end',
      data: JSON.stringify({
        thread_id: threadID,
        run_id: runID,
        status: 'success',
        reason: 'terminal',
      }),
    });
    await subscription.closed;

    const requests = fetchMock.mock.calls.map(([input, init]) => ({
      url: String(input),
      method: (init as RequestInit | undefined)?.method ?? 'GET',
    }));
    const streamURL = String(stream.mock.mock.calls[0][0]);
    expect(
      [...requests.map(request => request.url), streamURL].every(url =>
        url.startsWith('/api/workbench/threads'),
      ),
    ).toBe(true);
    expect(requests.map(request => request.url).join('\n')).not.toMatch(
      /\/api\/workbench\/task_threads|^\/api\/(threads|runs)(?:\/|$)/m,
    );
    expect(
      requests
        .filter(request => request.method !== 'GET')
        .map(request => ({
          method: request.method,
          path: request.url.split('?')[0],
        })),
    ).toEqual([
      { method: 'POST', path: '/api/workbench/threads' },
      { method: 'POST', path: '/api/workbench/threads' },
      { method: 'POST', path: `/api/workbench/threads/${threadID}/uploads` },
      { method: 'POST', path: `/api/workbench/threads/${threadID}/runs` },
      { method: 'POST', path: `/api/workbench/threads/${threadID}/runs` },
      {
        method: 'POST',
        path: `/api/workbench/threads/${threadID}/runs/${runID}/cancel`,
      },
      {
        method: 'POST',
        path: `/api/workbench/threads/${threadID}/runs/${runID}/resume`,
      },
      {
        method: 'POST',
        path: `/api/workbench/threads/${threadID}/runs/${runID}/retry`,
      },
      {
        method: 'POST',
        path: `/api/workbench/threads/${threadID}/artifacts/${artifactID}/restore`,
      },
      {
        method: 'PUT',
        path: `/api/workbench/threads/${threadID}/memories/${memoryID}`,
      },
    ]);
    expect(streamURL).toContain(
      `/api/workbench/threads/${threadID}/runs/${runID}/stream`,
    );
    expect(callbacks.onEvent).toHaveBeenCalledWith(
      runEventTransportFixture.visible,
    );
    expect(callbacks.onEnd).toHaveBeenCalledTimes(1);
    expect(callbacks.onError).not.toHaveBeenCalled();
    expect(responses).toHaveLength(0);
  });

  it('keeps canonical transport fixtures immutable', () => {
    expect(Object.isFrozen(canonicalTransportFixtures)).toBe(true);
  });
});
