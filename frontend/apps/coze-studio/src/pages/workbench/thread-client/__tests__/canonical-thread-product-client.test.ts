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

import type { WorkbenchThreadClient } from '../workbench-thread-client';
import { CanonicalThreadCoreClient } from '../canonical-thread-client';
import type { CanonicalFetch } from '../canonical-fetch';
import { WorkbenchClientError } from '../canonical-fetch';
import {
  artifactScanJobTransportFixture,
  artifactTransportFixture,
  guardrailAuditTransportFixture,
  mcpRuntimeAuditTransportFixture,
  memoryAuditTransportFixture,
  memoryTransportFixture,
  messageTransportFixture,
  runTransportFixture,
  tokenUsageTransportFixture,
  uploadTransportFixture,
} from './fixtures';

const spaceID = '9001';
const threadID = '1001';
const runID = '3001';
const retryRunID = '3002';
const fileID = '6001';
const artifactID = '7001';
const scanJobID = '8001';
const memoryID = '8401';
const correctedMemoryID = '8400';

const jsonResponse = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });

const emptyResponse = (): Response => new Response(null, { status: 204 });

const queuedFetch = (...responses: Response[]) =>
  vi.fn(() => {
    const response = responses.shift();
    if (!response) {
      throw new Error('Missing queued response');
    }
    return Promise.resolve(response);
  });

interface RequestSnapshot {
  url: string;
  method?: string;
  credentials?: RequestCredentials;
  signal?: AbortSignal | null;
  headers: Record<string, string>;
  body?: unknown;
}

const requestSnapshot = (
  fetchMock: ReturnType<typeof queuedFetch>,
  index: number,
): RequestSnapshot => {
  const call = fetchMock.mock.calls[index];
  if (!call) {
    throw new Error(`Missing fetch call ${index}`);
  }
  const [input, init] = call as unknown as [RequestInfo | URL, RequestInit];
  const headers: Record<string, string> = {};
  new Headers(init?.headers).forEach((value, key) => {
    headers[key.toLowerCase()] = value;
  });
  const snapshot: RequestSnapshot = {
    url: String(input),
    method: init?.method,
    credentials: init?.credentials,
    signal: init?.signal,
    headers,
  };
  if (typeof init?.body === 'string') {
    snapshot.body = JSON.parse(init.body) as unknown;
  } else if (init?.body !== undefined && init.body !== null) {
    snapshot.body = init.body;
  }
  return snapshot;
};

const scopedHeaders = {
  'x-coze-space-id': spaceID,
  'x-requested-with': 'XMLHttpRequest',
};
const jsonHeaders = {
  ...scopedHeaders,
  'content-type': 'application/json',
};

const productClient = (fetchMock: ReturnType<typeof queuedFetch>) =>
  new CanonicalThreadCoreClient({
    fetch: fetchMock as unknown as CanonicalFetch,
  }) as unknown as WorkbenchThreadClient;

const retryRunWire = () => {
  const run = structuredClone(
    runTransportFixture.canonical,
  ) as unknown as Record<string, unknown>;
  run.run_id = retryRunID;
  const coze = run.coze as Record<string, unknown>;
  coze.attempt_kind = 'retry';
  coze.source_run_id = runID;
  return run;
};

const expectClientError = async (promise: Promise<unknown>) => {
  const error = await promise.then(
    () => undefined,
    reason => reason as unknown,
  );
  expect(error).toBeInstanceOf(WorkbenchClientError);
  return error as WorkbenchClientError;
};

describe('CanonicalThread product routes', () => {
  it('uses canonical compatibility append, persisted suggestions, and subagent retry', async () => {
    const fetchMock = queuedFetch(
      jsonResponse(messageTransportFixture.canonical),
      jsonResponse({ suggestions: ['What should happen next?'] }),
      jsonResponse(retryRunWire()),
    );
    const client = productClient(fetchMock);

    await expect(
      client.appendMessage({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        role: 'assistant',
        content: 'Drafting the brief',
        metadata: '{"channel":"workbench"}',
      }),
    ).resolves.toEqual(messageTransportFixture.visible);
    await expect(
      client.generateSuggestions({
        space_id: spaceID,
        thread_id: threadID,
        messages: [{ role: 'user', content: 'must not be sent' }],
        n: 2,
        model_name: 'gpt-test',
        model_type: '42',
      }),
    ).resolves.toEqual(['What should happen next?']);
    await expect(
      client.retrySubagentRun({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        idempotency_key: 'retry-subagent-key',
      }),
    ).resolves.toEqual({
      ...runTransportFixture.visible,
      run_id: retryRunID,
      attempt_kind: 'retry',
      source_run_id: runID,
    });

    expect(requestSnapshot(fetchMock, 0)).toEqual({
      url: `/api/workbench/threads/${threadID}/messages`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: jsonHeaders,
      body: {
        run_id: runID,
        role: 'assistant',
        content: 'Drafting the brief',
        metadata: { channel: 'workbench' },
        append_mode: 'internal_compat',
      },
    });
    expect(requestSnapshot(fetchMock, 1)).toEqual({
      url: `/api/workbench/threads/${threadID}/suggestions`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: jsonHeaders,
      body: { n: 2, model_name: 'gpt-test', model_type: '42' },
    });
    expect(requestSnapshot(fetchMock, 2)).toEqual({
      url: `/api/workbench/threads/${threadID}/runs/${runID}/retry`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        ...scopedHeaders,
        'idempotency-key': 'retry-subagent-key',
      },
    });
  });

  it.each(['user', 'human'])(
    'rejects compatibility append role %s locally',
    async role => {
      const fetchMock = queuedFetch(
        jsonResponse(messageTransportFixture.canonical),
      );
      const client = productClient(fetchMock);

      const error = await expectClientError(
        client.appendMessage({
          space_id: spaceID,
          thread_id: threadID,
          run_id: runID,
          role,
          content: 'must stay atomic',
        }),
      );

      expect(error).toMatchObject({
        code: 'atomic_run_submission_required',
        retryable: false,
        outcome: 'unknown',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it('passes zero suggestion count through for server-side defaulting', async () => {
    const fetchMock = queuedFetch(jsonResponse({ suggestions: [] }));
    const client = productClient(fetchMock);

    await expect(
      client.generateSuggestions({
        space_id: spaceID,
        thread_id: threadID,
        n: 0,
      }),
    ).resolves.toEqual([]);
    expect(requestSnapshot(fetchMock, 0).body).toEqual({ n: 0 });
  });

  it('lists, creates, and deletes uploads by stable file ID', async () => {
    const fetchMock = queuedFetch(
      jsonResponse({
        uploads: uploadTransportFixture.canonical.uploads,
        total: 1,
        has_more: false,
      }),
      jsonResponse(uploadTransportFixture.canonical),
      emptyResponse(),
    );
    const client = productClient(fetchMock);
    const file = new File(['brief'], 'brief.md', { type: 'text/markdown' });

    await expect(
      client.listUploads({
        space_id: spaceID,
        thread_id: threadID,
        page: 3,
        page_size: 5,
      }),
    ).resolves.toEqual({
      items: [uploadTransportFixture.visible],
      total: 1,
      has_more: false,
    });
    await expect(
      client.uploadFiles({
        space_id: spaceID,
        thread_id: threadID,
        files: [file],
      }),
    ).resolves.toEqual({
      uploads: [uploadTransportFixture.visible],
      skipped_files: [],
    });
    await expect(
      client.deleteUpload({
        space_id: spaceID,
        thread_id: threadID,
        file_id: fileID,
      }),
    ).resolves.toBeUndefined();

    expect(requestSnapshot(fetchMock, 0)).toEqual({
      url: `/api/workbench/threads/${threadID}/uploads`,
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    const uploadRequest = requestSnapshot(fetchMock, 1);
    expect(uploadRequest).toMatchObject({
      url: `/api/workbench/threads/${threadID}/uploads`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(uploadRequest.body).toBeInstanceOf(FormData);
    expect((uploadRequest.body as FormData).getAll('files')).toEqual([file]);
    expect(requestSnapshot(fetchMock, 2)).toEqual({
      url: `/api/workbench/threads/${threadID}/uploads/${fileID}`,
      method: 'DELETE',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
  });

  it('covers all Artifact and scan routes including Blob content', async () => {
    const contentResponse = new Response('artifact bytes', {
      status: 200,
      headers: {
        'content-type': 'text/plain; charset=utf-8',
        'content-disposition': 'inline; filename="brief.txt"',
      },
    });
    const signedURL = {
      artifact_id: artifactID,
      url: 'https://objects.example.test/signed',
      expires_in_seconds: 60,
      content_type: 'text/plain',
      preview_mode: 'text',
    };
    const scanReview = {
      artifact_id: artifactID,
      decision: 'release',
      scan_status: 'clean',
      reviewed: true,
    };
    const fetchMock = queuedFetch(
      jsonResponse(artifactTransportFixture.canonical),
      contentResponse,
      jsonResponse(signedURL),
      emptyResponse(),
      jsonResponse({
        artifact: artifactTransportFixture.canonical.artifacts[0],
        restored: true,
      }),
      jsonResponse(scanReview),
      jsonResponse(artifactScanJobTransportFixture.canonical),
      jsonResponse({
        job: artifactScanJobTransportFixture.canonical.jobs[0],
        retried: true,
      }),
    );
    const client = productClient(fetchMock);

    await expect(
      client.listArtifacts({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        deleted_only: true,
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [artifactTransportFixture.visible],
      total: 1,
      has_more: false,
    });
    const content = await client.getArtifactContent({
      space_id: spaceID,
      thread_id: threadID,
      artifact_id: artifactID,
      mode: 'preview',
    });
    expect(await content.blob.text()).toBe('artifact bytes');
    expect(content).toMatchObject({
      content_type: 'text/plain; charset=utf-8',
      content_disposition: 'inline; filename="brief.txt"',
    });
    await expect(
      client.getArtifactSignedURL({
        space_id: spaceID,
        thread_id: threadID,
        artifact_id: artifactID,
        mode: 'download',
        ttl_seconds: 60,
      }),
    ).resolves.toEqual(signedURL);
    await expect(
      client.deleteArtifact({
        space_id: spaceID,
        thread_id: threadID,
        artifact_id: artifactID,
      }),
    ).resolves.toBeUndefined();
    await expect(
      client.restoreArtifact({
        space_id: spaceID,
        thread_id: threadID,
        artifact_id: artifactID,
      }),
    ).resolves.toEqual({
      artifact: artifactTransportFixture.visible,
      restored: true,
    });
    await expect(
      client.reviewArtifactScan({
        space_id: spaceID,
        thread_id: threadID,
        artifact_id: artifactID,
        decision: 'release',
        reason: 'Reviewed by operator',
      }),
    ).resolves.toEqual(scanReview);
    await expect(
      client.listArtifactScanJobs({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        artifact_id: artifactID,
        status: 'succeeded',
        scanner: 'default',
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [artifactScanJobTransportFixture.visible],
      total: 1,
      has_more: false,
    });
    await expect(
      client.retryArtifactScanJob({
        space_id: spaceID,
        thread_id: threadID,
        job_id: scanJobID,
      }),
    ).resolves.toEqual({
      job: artifactScanJobTransportFixture.visible,
      retried: true,
    });

    expect(requestSnapshot(fetchMock, 0)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifacts?run_id=${runID}&deleted_only=true&limit=10&offset=10`,
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 1)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifacts/${artifactID}/content?mode=preview`,
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 2)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifacts/${artifactID}/signed_url?mode=download&ttl_seconds=60`,
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 3)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifacts/${artifactID}`,
      method: 'DELETE',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 4)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifacts/${artifactID}/restore`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 5)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifacts/${artifactID}/scan_review`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: jsonHeaders,
      body: { decision: 'release', reason: 'Reviewed by operator' },
    });
    expect(requestSnapshot(fetchMock, 6)).toEqual({
      url: [
        `/api/workbench/threads/${threadID}/artifact_scan_jobs`,
        `?run_id=${runID}&artifact_id=${artifactID}`,
        '&status=succeeded&scanner=default&limit=10&offset=10',
      ].join(''),
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 7)).toEqual({
      url: `/api/workbench/threads/${threadID}/artifact_scan_jobs/${scanJobID}/retry`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
  });

  it('passes zero signed URL TTL through for server-side defaulting', async () => {
    const response = {
      artifact_id: artifactID,
      url: 'https://objects.example.test/default-ttl',
      expires_in_seconds: 300,
      content_type: 'text/plain',
      preview_mode: 'text',
    };
    const fetchMock = queuedFetch(jsonResponse(response));
    const client = productClient(fetchMock);

    await expect(
      client.getArtifactSignedURL({
        space_id: spaceID,
        thread_id: threadID,
        artifact_id: artifactID,
        mode: 'download',
        ttl_seconds: 0,
      }),
    ).resolves.toEqual(response);
    expect(requestSnapshot(fetchMock, 0).url).toBe(
      `/api/workbench/threads/${threadID}/artifacts/${artifactID}/signed_url?mode=download&ttl_seconds=0`,
    );
  });

  it('maps token usage filters and preserves abort ownership', async () => {
    const fetchMock = queuedFetch(
      jsonResponse(tokenUsageTransportFixture.canonical),
    );
    const client = productClient(fetchMock);
    const { signal } = new AbortController();

    await expect(
      client.getTokenUsage({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        include_child_runs: true,
        source: 'tool',
        page: 2,
        page_size: 5,
        signal,
      }),
    ).resolves.toEqual(tokenUsageTransportFixture.visible);
    expect(requestSnapshot(fetchMock, 0)).toEqual({
      url: [
        `/api/workbench/threads/${threadID}/token_usage`,
        `?run_id=${runID}&include_child_runs=true`,
        '&source=tool&limit=5&offset=5',
      ].join(''),
      method: 'GET',
      credentials: 'same-origin',
      signal,
      headers: scopedHeaders,
    });
  });

  it('covers every Memory route with object metadata and RFC3339 writes', async () => {
    const exportResponse = {
      schema: 'coze.memory_export.v1',
      thread_id: threadID,
      exported_at: '2026-01-01T00:01:00.000Z',
      total: 1,
      memories: memoryTransportFixture.canonical.memories,
    };
    const fetchMock = queuedFetch(
      jsonResponse(memoryTransportFixture.canonical),
      jsonResponse({
        memory: memoryTransportFixture.canonical.memories[0],
        updated: true,
      }),
      emptyResponse(),
      jsonResponse({
        memory: memoryTransportFixture.canonical.memories[0],
        restored: true,
      }),
      jsonResponse({ deleted: 2 }),
      jsonResponse(exportResponse),
      jsonResponse({
        imported: 1,
        skipped: 0,
        memories: memoryTransportFixture.canonical.memories,
      }),
    );
    const client = productClient(fetchMock);

    await expect(
      client.listMemories({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        scope: 'thread',
        scopes: ['run', 'long_term'],
        q: 'launch',
        include_expired: true,
        include_deleted: false,
        page: 3,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [memoryTransportFixture.visible],
      total: 1,
      has_more: false,
    });
    await expect(
      client.updateMemory({
        space_id: spaceID,
        thread_id: threadID,
        memory_id: memoryID,
        run_id: runID,
        scope: 'thread',
        content: 'Launch date is Friday',
        metadata: '{"kind":"fact"}',
        score: 0.9,
        confidence: 0.95,
        source_type: 'message',
        source_id: 'message:3001',
        correction_of_memory_id: correctedMemoryID,
        corrected_at: 1767225600000,
        expires_at: 1767225660000,
      }),
    ).resolves.toEqual({
      memory: memoryTransportFixture.visible,
      updated: true,
    });
    await expect(
      client.deleteMemory({
        space_id: spaceID,
        thread_id: threadID,
        memory_id: memoryID,
      }),
    ).resolves.toBeUndefined();
    await expect(
      client.restoreMemory({
        space_id: spaceID,
        thread_id: threadID,
        memory_id: memoryID,
      }),
    ).resolves.toEqual({
      memory: memoryTransportFixture.visible,
      restored: true,
    });
    await expect(
      client.clearMemories({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        scopes: ['thread', 'run'],
      }),
    ).resolves.toEqual({ deleted: 2 });
    await expect(
      client.exportMemories({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        scope: 'thread',
        scopes: ['run'],
        q: 'launch',
        include_expired: true,
        include_deleted: false,
        page: 1,
        page_size: 50,
      }),
    ).resolves.toEqual({
      schema: 'coze.memory_export.v1',
      thread_id: threadID,
      exported_at: 1767225660000,
      total: 1,
      memories: [memoryTransportFixture.visible],
    });
    await expect(
      client.importMemories({
        space_id: spaceID,
        thread_id: threadID,
        memories: [
          {
            run_id: runID,
            scope: 'thread',
            content: 'Launch date is Friday',
            metadata: '{"kind":"fact"}',
            score: 0.9,
            confidence: 0.95,
            source_type: 'message',
            source_id: 'message:3001',
            correction_of_memory_id: correctedMemoryID,
            corrected_at: 1767225600000,
            expires_at: 1767225660000,
          },
        ],
      }),
    ).resolves.toEqual({
      imported: 1,
      skipped: 0,
      memories: [memoryTransportFixture.visible],
    });

    expect(requestSnapshot(fetchMock, 0)).toEqual({
      url: [
        `/api/workbench/threads/${threadID}/memories`,
        `?run_id=${runID}&scope=thread&scopes=run&scopes=long_term`,
        '&q=launch&include_expired=true&include_deleted=false',
        '&limit=10&offset=20',
      ].join(''),
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    const memoryWrite = {
      run_id: runID,
      scope: 'thread',
      content: 'Launch date is Friday',
      metadata: { kind: 'fact' },
      score: 0.9,
      confidence: 0.95,
      source_type: 'message',
      source_id: 'message:3001',
      correction_of_memory_id: correctedMemoryID,
      corrected_at: '2026-01-01T00:00:00.000Z',
      expires_at: '2026-01-01T00:01:00.000Z',
    };
    expect(requestSnapshot(fetchMock, 1)).toEqual({
      url: `/api/workbench/threads/${threadID}/memories/${memoryID}`,
      method: 'PUT',
      credentials: 'same-origin',
      signal: undefined,
      headers: jsonHeaders,
      body: memoryWrite,
    });
    expect(requestSnapshot(fetchMock, 2)).toEqual({
      url: `/api/workbench/threads/${threadID}/memories/${memoryID}`,
      method: 'DELETE',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 3)).toEqual({
      url: `/api/workbench/threads/${threadID}/memories/${memoryID}/restore`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 4)).toEqual({
      url: `/api/workbench/threads/${threadID}/memories/clear`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: jsonHeaders,
      body: { run_id: runID, scopes: ['thread', 'run'] },
    });
    expect(requestSnapshot(fetchMock, 5)).toEqual({
      url: [
        `/api/workbench/threads/${threadID}/memories/export`,
        `?run_id=${runID}&scope=thread&scopes=run&q=launch`,
        '&include_expired=true&include_deleted=false&limit=50',
      ].join(''),
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: scopedHeaders,
    });
    expect(requestSnapshot(fetchMock, 6)).toEqual({
      url: `/api/workbench/threads/${threadID}/memories/import`,
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: jsonHeaders,
      body: { memories: [memoryWrite] },
    });
  });

  it('lists and exports only reviewed audit fields', async () => {
    const guardrailExport = {
      schema: 'coze.guardrail_audit_export.v1',
      thread_id: threadID,
      exported_at: '2026-01-01T00:01:00.000Z',
      total: 1,
      events: guardrailAuditTransportFixture.canonical.events,
    };
    const fetchMock = queuedFetch(
      jsonResponse(memoryAuditTransportFixture.canonical),
      jsonResponse(guardrailAuditTransportFixture.canonical),
      jsonResponse(guardrailExport),
      jsonResponse(mcpRuntimeAuditTransportFixture.canonical),
    );
    const client = productClient(fetchMock);

    await expect(
      client.listMemoryAuditEvents({
        space_id: spaceID,
        thread_id: threadID,
        memory_id: memoryID,
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [memoryAuditTransportFixture.visible],
      total: 1,
      has_more: false,
    });
    await expect(
      client.listGuardrailAuditEvents({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [guardrailAuditTransportFixture.visible],
      total: 1,
      has_more: false,
    });
    await expect(
      client.exportGuardrailAuditEvents({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      schema: 'coze.guardrail_audit_export.v1',
      thread_id: threadID,
      exported_at: 1767225660000,
      total: 1,
      events: [guardrailAuditTransportFixture.visible],
    });
    await expect(
      client.listMCPRuntimeAuditEvents({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [mcpRuntimeAuditTransportFixture.visible],
      total: 1,
      has_more: false,
    });

    const routes = [
      `memories/audit_events?memory_id=${memoryID}&limit=10&offset=10`,
      `guardrail_audit_events?run_id=${runID}&limit=10&offset=10`,
      `guardrail_audit_events/export?run_id=${runID}&limit=10&offset=10`,
      `mcp_runtime_audit_events?run_id=${runID}&limit=10&offset=10`,
    ];
    routes.forEach((route, index) => {
      expect(requestSnapshot(fetchMock, index)).toEqual({
        url: `/api/workbench/threads/${threadID}/${route}`,
        method: 'GET',
        credentials: 'same-origin',
        signal: undefined,
        headers: scopedHeaders,
      });
    });
  });

  it('caps product page size before calculating export offsets', async () => {
    const response = {
      schema: 'coze.guardrail_audit_export.v1',
      thread_id: threadID,
      exported_at: '2026-01-01T00:01:00.000Z',
      total: 400,
      events: guardrailAuditTransportFixture.canonical.events,
    };
    const fetchMock = queuedFetch(
      jsonResponse(response),
      jsonResponse(response),
    );
    const client = productClient(fetchMock);

    await client.exportGuardrailAuditEvents({
      space_id: spaceID,
      thread_id: threadID,
      page: 1,
      page_size: 1000,
    });
    await client.exportGuardrailAuditEvents({
      space_id: spaceID,
      thread_id: threadID,
      page: 2,
      page_size: 1000,
    });

    expect(requestSnapshot(fetchMock, 0).url).toBe(
      `/api/workbench/threads/${threadID}/guardrail_audit_events/export?limit=200&offset=0`,
    );
    expect(requestSnapshot(fetchMock, 1).url).toBe(
      `/api/workbench/threads/${threadID}/guardrail_audit_events/export?limit=200&offset=200`,
    );
  });

  it('rejects invalid pagination and Memory metadata before fetch', async () => {
    const fetchMock = queuedFetch(
      jsonResponse(memoryTransportFixture.canonical),
    );
    const client = productClient(fetchMock);

    const pagination = await expectClientError(
      client.listArtifacts({
        space_id: spaceID,
        thread_id: threadID,
        page: 1.5,
      }),
    );
    expect(pagination.code).toBe('invalid_pagination');

    const metadata = await expectClientError(
      client.updateMemory({
        space_id: spaceID,
        thread_id: threadID,
        memory_id: memoryID,
        scope: 'thread',
        content: 'x',
        metadata: '[]',
      }),
    );
    expect(metadata.code).toBe('invalid_request_shape');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
