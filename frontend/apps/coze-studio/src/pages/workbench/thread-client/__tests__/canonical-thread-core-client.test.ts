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

import { CanonicalThreadCoreClient } from '../canonical-thread-client';
import {
  fetchCanonicalJSON,
  WorkbenchClientError,
  type CanonicalFetch,
} from '../canonical-fetch';
import {
  humanInteractionTransportFixture,
  messageTransportFixture,
  runEventTransportFixture,
  runTransportFixture,
  threadTransportFixture,
} from './fixtures';

const canonicalSpaceID = '9001';
const canonicalThreadID = '1001';
const canonicalRunID = '3001';
const canonicalFileID = '6001';
const forbiddenRoutePrefixes = [
  '/api/workbench/task_threads',
  '/api/threads',
  '/api/runs',
] as const;

interface ResponseOptions {
  status?: number;
  headers?: Record<string, string>;
}

const jsonResponse = (
  body: unknown,
  { status = 200, headers = {} }: ResponseOptions = {},
) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json', ...headers },
  });

const textResponse = (
  body: string,
  { status = 200, headers = {} }: ResponseOptions = {},
) => new Response(body, { status, headers });

type FetchResult = Response | { error: unknown };

const recordingFetch = (...results: FetchResult[]) => {
  const queue = [...results];
  return vi.fn(
    (_input: RequestInfo | URL, _init?: RequestInit): Promise<Response> => {
      const next = queue.shift();
      if (!next) {
        throw new Error('Missing mocked canonical response');
      }
      if ('error' in next) {
        return Promise.reject(next.error);
      }
      return Promise.resolve(next);
    },
  );
};

interface RequestSnapshot {
  url: string;
  method: string | undefined;
  credentials: RequestCredentials | undefined;
  signal: AbortSignal | null | undefined;
  headers: Record<string, string>;
  body?: unknown;
}

const requestSnapshot = (
  fetchMock: ReturnType<typeof recordingFetch>,
  index = 0,
): RequestSnapshot => {
  const call = fetchMock.mock.calls[index];
  if (!call) {
    throw new Error(`Missing fetch call ${index}`);
  }
  const [input, init] = call;
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

const expectNoFallback = (fetchMock: ReturnType<typeof recordingFetch>) => {
  for (const [input] of fetchMock.mock.calls) {
    const url = String(input);
    expect(forbiddenRoutePrefixes.some(prefix => url.startsWith(prefix))).toBe(
      false,
    );
  }
};

const expectClientError = async (
  promise: Promise<unknown>,
): Promise<WorkbenchClientError> => {
  const error = await promise.then(
    () => undefined,
    reason => reason as unknown,
  );
  expect(error).toBeInstanceOf(WorkbenchClientError);
  return error as WorkbenchClientError;
};

const cloneThreadWire = () =>
  structuredClone(threadTransportFixture.canonical) as unknown as Record<
    string,
    unknown
  >;

const cloneRunWire = () =>
  structuredClone(runTransportFixture.canonical) as unknown as Record<
    string,
    unknown
  >;

const cloneMessageWire = () =>
  structuredClone(messageTransportFixture.canonical) as unknown as Record<
    string,
    unknown
  >;

const cloneEventPageWire = () =>
  structuredClone(runEventTransportFixture.canonical) as unknown as Record<
    string,
    unknown
  >;

const childRecord = (owner: Record<string, unknown>, key: string) => {
  const child = owner[key];
  if (typeof child !== 'object' || child === null || Array.isArray(child)) {
    throw new TypeError(`${key} is not an object`);
  }
  return child as Record<string, unknown>;
};

const makeThreadCreationWire = ({ deferred = false } = {}) => {
  const thread = cloneThreadWire();
  const coze = childRecord(thread, 'coze');
  if (deferred) {
    thread.status = 'idle';
    coze.product_status = 'idle';
    coze.initial_submission = null;
    return thread;
  }
  const message = cloneMessageWire();
  message.role = 'user';
  message.content = 'Prepare the launch brief';
  coze.initial_submission = {
    message,
    run: cloneRunWire(),
  };
  return thread;
};

const makeRunCreationWire = (content: string) => {
  const run = cloneRunWire();
  const coze = childRecord(run, 'coze');
  const message = cloneMessageWire();
  message.role = 'user';
  message.content = content;
  coze.message_id = '2001';
  coze.submission_message = message;
  return run;
};

const makeRetryRunCreationWire = () => {
  const run = cloneRunWire();
  run.run_id = '3002';
  const coze = childRecord(run, 'coze');
  coze.message_id = null;
  coze.attempt_kind = 'retry';
  coze.source_run_id = canonicalRunID;
  delete coze.submission_message;
  return run;
};

const coreClient = (fetchMock: ReturnType<typeof recordingFetch>) =>
  new CanonicalThreadCoreClient({ fetch: fetchMock as CanonicalFetch });

describe('fetchCanonicalJSON', () => {
  it('returns direct JSON and safe pagination headers through one scoped request', async () => {
    const { signal } = new AbortController();
    const fetchMock = recordingFetch(
      jsonResponse(
        { accepted: true },
        {
          headers: {
            'X-Pagination-Total': '42',
            'X-Pagination-Next': '20',
          },
        },
      ),
    );

    await expect(
      fetchCanonicalJSON('/api/workbench/threads/search', {
        fetch: fetchMock as CanonicalFetch,
        method: 'POST',
        spaceId: canonicalSpaceID,
        json: { limit: 20, offset: 0 },
        idempotencyKey: 'request-key',
        signal,
      }),
    ).resolves.toEqual({
      body: { accepted: true },
      pagination: { total: 42, next: '20' },
    });

    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/search',
      method: 'POST',
      credentials: 'same-origin',
      signal,
      headers: {
        'content-type': 'application/json',
        'idempotency-key': 'request-key',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: { limit: 20, offset: 0 },
    });
  });

  it('accepts canonical 204 without parsing a body', async () => {
    const fetchMock = recordingFetch(new Response(null, { status: 204 }));

    await expect(
      fetchCanonicalJSON('/api/workbench/threads/1001/runs/3001/cancel', {
        fetch: fetchMock as CanonicalFetch,
        method: 'POST',
        spaceId: canonicalSpaceID,
      }),
    ).resolves.toEqual({ body: undefined, pagination: {} });
    expect(requestSnapshot(fetchMock).headers).toEqual({
      'x-coze-space-id': canonicalSpaceID,
      'x-requested-with': 'XMLHttpRequest',
    });
  });

  it('maps a canonical error without losing reviewed fields', async () => {
    const fetchMock = recordingFetch(
      jsonResponse(
        {
          detail: 'Run is active',
          code: 'run_conflict',
          retryable: false,
          trace_id: 'trace-409',
        },
        { status: 409 },
      ),
    );

    const error = await expectClientError(
      fetchCanonicalJSON('/api/workbench/threads/1001/runs', {
        fetch: fetchMock as CanonicalFetch,
        method: 'POST',
        spaceId: canonicalSpaceID,
        json: { assistant_id: 'agent' },
      }),
    );

    expect(error).toMatchObject({
      message: 'Run is active',
      status: 409,
      code: 'run_conflict',
      traceId: 'trace-409',
      retryable: false,
      outcome: 'rejected',
    });
  });

  it.each([
    ['malformed success', textResponse('{'), 200],
    ['unknown success shape', textResponse('"legacy-success"'), 200],
    ['unsafe success integer', textResponse('{"count":9007199254740992}'), 200],
    ['malformed error', textResponse('{', { status: 400 }), 400],
    [
      'unknown error shape',
      jsonResponse(
        { code: 'run_conflict', message: 'legacy envelope' },
        { status: 409 },
      ),
      409,
    ],
    [
      'unsafe pagination total',
      jsonResponse([], {
        headers: { 'X-Pagination-Total': '9007199254740992' },
      }),
      200,
    ],
    [
      'invalid pagination next',
      jsonResponse([], { headers: { 'X-Pagination-Next': '-1' } }),
      200,
    ],
  ])(
    'classifies %s as an unknown protocol outcome',
    async (_name, response, status) => {
      const fetchMock = recordingFetch(response);

      const error = await expectClientError(
        fetchCanonicalJSON('/api/workbench/threads/search', {
          fetch: fetchMock as CanonicalFetch,
          method: 'POST',
          spaceId: canonicalSpaceID,
          json: { limit: 20, offset: 0 },
        }),
      );

      expect(error).toMatchObject({
        status,
        code: 'invalid_response',
        retryable: false,
        outcome: 'unknown',
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expectNoFallback(fetchMock);
    },
  );
});

describe('CanonicalThreadCoreClient request contract', () => {
  it('searches direct Thread resources with exact offset pagination', async () => {
    const fetchMock = recordingFetch(
      jsonResponse([threadTransportFixture.canonical], {
        headers: {
          'X-Pagination-Total': '51',
          'X-Pagination-Next': '50',
        },
      }),
    );
    const client = coreClient(fetchMock);
    expect(client.contract).toBe('canonical_v1');

    await expect(
      client.searchThreads({
        space_id: canonicalSpaceID,
        page: 3,
        page_size: 25,
        status: 'busy',
      }),
    ).resolves.toEqual({
      items: [threadTransportFixture.visible],
      total: 51,
      has_more: true,
      next_cursor: '50',
    });
    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/search',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'content-type': 'application/json',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: { limit: 25, offset: 50, status: 'busy' },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('caps core page size before calculating the next page offset', async () => {
    const fetchMock = recordingFetch(
      jsonResponse([threadTransportFixture.canonical], {
        headers: { 'X-Pagination-Total': '200' },
      }),
    );
    const client = coreClient(fetchMock);

    await client.searchThreads({
      space_id: canonicalSpaceID,
      page: 2,
      page_size: 1000,
    });

    expect(requestSnapshot(fetchMock).body).toEqual({
      limit: 100,
      offset: 100,
    });
  });

  it('creates one initial user submission without unsupported create options', async () => {
    const fetchMock = recordingFetch(jsonResponse(makeThreadCreationWire()));
    const client = coreClient(fetchMock);

    await expect(
      client.createThread({
        space_id: canonicalSpaceID,
        message: 'Prepare the launch brief',
        title: 'Prepare launch brief',
        config: '{"runtime":"eino_adk","mode":"pro"}',
        context: '{"locale":"en-US"}',
        metadata: '{"source":"workbench_new_task"}',
        idempotency_key: 'initial-key',
      }),
    ).resolves.toEqual({
      thread: threadTransportFixture.visible,
      message: {
        ...messageTransportFixture.visible,
        role: 'user',
        content: 'Prepare the launch brief',
      },
      run: runTransportFixture.visible,
    });

    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'content-type': 'application/json',
        'idempotency-key': 'initial-key',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: {
        metadata: { title: 'Prepare launch brief', source: 'web' },
        coze: {
          initial_run: {
            assistant_id: 'agent',
            input: {
              messages: [{ role: 'user', content: 'Prepare the launch brief' }],
            },
            config: { runtime: 'eino_adk', mode: 'pro' },
            context: { locale: 'en-US' },
            metadata: { source: 'workbench_new_task' },
          },
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('creates one deferred Thread and does not invent an initial Run', async () => {
    const fetchMock = recordingFetch(
      jsonResponse(makeThreadCreationWire({ deferred: true })),
    );
    const client = coreClient(fetchMock);

    await expect(
      client.createThread({
        space_id: canonicalSpaceID,
        message: 'Inspect the uploaded brief',
        config: '{"runtime":"eino_adk"}',
        defer_start: true,
        idempotency_key: 'deferred-key',
      }),
    ).resolves.toEqual({
      thread: {
        ...threadTransportFixture.visible,
        status: 'idle',
      },
    });

    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'content-type': 'application/json',
        'idempotency-key': 'deferred-key',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: {
        metadata: { source: 'web' },
        coze: {
          deferred_initial_run: {
            assistant_id: 'agent',
            input: {
              messages: [
                { role: 'user', content: 'Inspect the uploaded brief' },
              ],
            },
            config: { runtime: 'eino_adk' },
          },
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('gets one Thread and injects request space instead of response space', async () => {
    const responseThread = cloneThreadWire();
    responseThread.space_id = '9999';
    const fetchMock = recordingFetch(jsonResponse(responseThread));
    const client = coreClient(fetchMock);

    await expect(
      client.getThread({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
      }),
    ).resolves.toEqual(threadTransportFixture.visible);
    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/1001',
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
    });
  });

  it.each([
    ['before_seq', '19'],
    ['after_seq', '20'],
  ] as const)(
    'lists the canonical Message page using %s',
    async (cursorName, cursorValue) => {
      const fetchMock = recordingFetch(
        jsonResponse({
          data: [messageTransportFixture.canonical],
          has_more: true,
          next_before_seq: '1',
          next_after_seq: '2',
        }),
      );
      const client = coreClient(fetchMock);

      await expect(
        client.listMessages({
          space_id: canonicalSpaceID,
          thread_id: canonicalThreadID,
          limit: 10,
          [cursorName]: cursorValue,
        }),
      ).resolves.toEqual({
        items: [messageTransportFixture.visible],
        has_more: true,
        next_before_seq: '1',
        next_after_seq: '2',
      });
      expect(requestSnapshot(fetchMock)).toEqual({
        url: `/api/workbench/threads/1001/messages?limit=10&${cursorName}=${cursorValue}`,
        method: 'GET',
        credentials: 'same-origin',
        signal: undefined,
        headers: {
          'x-coze-space-id': canonicalSpaceID,
          'x-requested-with': 'XMLHttpRequest',
        },
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    },
  );

  it('lists direct Run resources with filters and pagination headers', async () => {
    const fetchMock = recordingFetch(
      jsonResponse([runTransportFixture.canonical], {
        headers: {
          'X-Pagination-Total': '21',
          'X-Pagination-Next': '20',
        },
      }),
    );
    const client = coreClient(fetchMock);

    await expect(
      client.listRuns({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
        parent_run_id: '3000',
        status: 'running',
        page: 2,
        page_size: 10,
      }),
    ).resolves.toEqual({
      items: [runTransportFixture.visible],
      total: 21,
      has_more: true,
      next_cursor: '20',
    });
    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/1001/runs?parent_run_id=3000&status=running&limit=10&offset=10',
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
    });
  });

  it('creates one atomic follow-up and keeps only uploaded file_id references', async () => {
    const content = 'Use the uploaded brief for the answer';
    const fetchMock = recordingFetch(
      jsonResponse(makeRunCreationWire(content)),
    );
    const client = coreClient(fetchMock);

    await expect(
      client.createRun({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
        input: JSON.stringify({
          messages: [{ role: 'user', content: 'stale duplicate' }],
          uploaded_files: [
            {
              file_id: canonicalFileID,
              file_name: 'brief.md',
              virtual_path: '/brief.md',
            },
          ],
        }),
        message_content: content,
        message_metadata: '{"source":"composer"}',
        command: '{}',
        config: '{"runtime":"eino_adk","mode":"pro"}',
        context: '{"locale":"en-US"}',
        metadata: '{"source":"workbench_detail_followup"}',
        stream_mode: '["messages-tuple","updates"]',
        multitask_strategy: 'reject',
        on_disconnect: 'continue',
        durability: 'async',
        idempotency_key: 'follow-up-key',
      }),
    ).resolves.toEqual({
      run: {
        ...runTransportFixture.visible,
        message_id: '2001',
      },
      message: {
        ...messageTransportFixture.visible,
        role: 'user',
        content,
      },
    });

    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/1001/runs',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'content-type': 'application/json',
        'idempotency-key': 'follow-up-key',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: {
        assistant_id: 'agent',
        input: {
          messages: [{ role: 'user', content }],
          uploaded_files: [{ file_id: canonicalFileID }],
        },
        command: {},
        config: { runtime: 'eino_adk', mode: 'pro' },
        context: { locale: 'en-US' },
        metadata: { source: 'workbench_detail_followup' },
        coze: {
          message_metadata: { source: 'composer' },
        },
        stream_mode: ['messages-tuple', 'updates'],
        multitask_strategy: 'reject',
        on_disconnect: 'continue',
        durability: 'async',
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('creates an attachment-free Run exactly once', async () => {
    const content = 'Continue without files';
    const fetchMock = recordingFetch(
      jsonResponse(makeRunCreationWire(content)),
    );
    const client = coreClient(fetchMock);

    await client.createRun({
      space_id: canonicalSpaceID,
      thread_id: canonicalThreadID,
      input: '{"messages":[],"uploaded_files":[]}',
      message_content: content,
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(requestSnapshot(fetchMock).body).toEqual({
      assistant_id: 'agent',
      input: {
        messages: [{ role: 'user', content }],
        uploaded_files: [],
      },
    });
  });

  it('creates a message-less top-level retry with the reviewed source relation', async () => {
    const content = 'Retry the current task';
    const fetchMock = recordingFetch(jsonResponse(makeRetryRunCreationWire()));
    const client = coreClient(fetchMock);

    await expect(
      client.createRun({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
        input: '{"uploaded_files":[]}',
        message_content: content,
        metadata: '{"source":"task_retry"}',
        attempt_kind: 'retry',
        source_run_id: canonicalRunID,
        idempotency_key: 'retry-key',
      }),
    ).resolves.toEqual({
      run: {
        ...runTransportFixture.visible,
        run_id: '3002',
        attempt_kind: 'retry',
        source_run_id: canonicalRunID,
      },
    });

    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/1001/runs',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'content-type': 'application/json',
        'idempotency-key': 'retry-key',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: {
        assistant_id: 'agent',
        input: {
          messages: [{ role: 'user', content }],
          uploaded_files: [],
        },
        metadata: { source: 'task_retry' },
        coze: {
          attempt_kind: 'retry',
          source_run_id: canonicalRunID,
        },
      },
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('gets and cancels the addressed Run on canonical routes', async () => {
    const fetchMock = recordingFetch(
      jsonResponse(runTransportFixture.canonical),
      new Response(null, { status: 204 }),
    );
    const client = coreClient(fetchMock);
    const request = {
      space_id: canonicalSpaceID,
      thread_id: canonicalThreadID,
      run_id: canonicalRunID,
    };

    await expect(client.getRun(request)).resolves.toEqual(
      runTransportFixture.visible,
    );
    await expect(client.cancelRun(request)).resolves.toBeUndefined();
    expect(requestSnapshot(fetchMock, 0)).toEqual({
      url: '/api/workbench/threads/1001/runs/3001',
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
    });
    expect(requestSnapshot(fetchMock, 1)).toEqual({
      url: '/api/workbench/threads/1001/runs/3001/cancel',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
    });
  });

  it('resumes with only canonical response fields and server-owned attribution omitted', async () => {
    const resumedRun = cloneRunWire();
    resumedRun.run_id = '3002';
    const fetchMock = recordingFetch(jsonResponse(resumedRun));
    const client = coreClient(fetchMock);
    const response = {
      ...humanInteractionTransportFixture.canonical,
      submitted_by: 'forged-user',
      submitted_at: 1767225600000,
      source: 'forged-source',
    };

    await expect(
      client.resumeRun({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
        run_id: canonicalRunID,
        interrupt_id: 'interrupt-1',
        response,
        idempotency_key: 'resume-key',
      }),
    ).resolves.toEqual({ ...runTransportFixture.visible, run_id: '3002' });
    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/1001/runs/3001/resume',
      method: 'POST',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'content-type': 'application/json',
        'idempotency-key': 'resume-key',
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      body: {
        interrupt_id: 'interrupt-1',
        response: humanInteractionTransportFixture.canonical,
      },
    });
  });

  it('maps the app cursor to after_event_id and encodes repeated event_types', async () => {
    const page = cloneEventPageWire();
    page.has_more = true;
    page.next_after_event_id = '5001';
    const fetchMock = recordingFetch(jsonResponse(page));
    const client = coreClient(fetchMock);

    await expect(
      client.listRunEvents({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
        run_id: canonicalRunID,
        cursor: '5000',
        limit: 10,
        event_types: ['tool.completed', 'run.completed'],
      }),
    ).resolves.toEqual({
      items: [runEventTransportFixture.visible],
      has_more: true,
      next_cursor: '5001',
    });
    expect(requestSnapshot(fetchMock)).toEqual({
      url: '/api/workbench/threads/1001/runs/3001/events?after_event_id=5000&limit=10&event_types=tool.completed&event_types=run.completed',
      method: 'GET',
      credentials: 'same-origin',
      signal: undefined,
      headers: {
        'x-coze-space-id': canonicalSpaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
    });
  });
});

describe('CanonicalThreadCoreClient strict local and response validation', () => {
  it.each([
    ['command', '{}'],
    ['stream_mode', 'events'],
    ['multitask_strategy', 'reject'],
    ['on_disconnect', 'continue'],
    ['durability', 'async'],
  ] as const)(
    'fails closed for unsupported createThread option %s',
    async (field, value) => {
      const fetchMock = recordingFetch(jsonResponse(makeThreadCreationWire()));
      const client = coreClient(fetchMock);

      const error = await expectClientError(
        client.createThread({
          space_id: canonicalSpaceID,
          message: 'Create safely',
          [field]: value,
        }),
      );

      expect(error).toMatchObject({
        status: undefined,
        code: 'unsupported_create_option',
        retryable: false,
        outcome: 'unknown',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it('uses the stable unsupported error for malformed createThread command JSON', async () => {
    const fetchMock = recordingFetch(jsonResponse(makeThreadCreationWire()));
    const client = coreClient(fetchMock);

    const error = await expectClientError(
      client.createThread({
        space_id: canonicalSpaceID,
        message: 'Create safely',
        command: '{invalid',
      }),
    );

    expect(error).toMatchObject({
      status: undefined,
      code: 'unsupported_create_option',
      retryable: false,
      outcome: 'unknown',
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([
    ['create config', 'createThread', 'config'],
    ['create context', 'createThread', 'context'],
    ['create metadata', 'createThread', 'metadata'],
    ['run input', 'createRun', 'input'],
    ['run config', 'createRun', 'config'],
    ['run context', 'createRun', 'context'],
    ['run metadata', 'createRun', 'metadata'],
    ['run command', 'createRun', 'command'],
    ['run message metadata', 'createRun', 'message_metadata'],
  ] as const)(
    'rejects invalid JSON in %s before fetch',
    async (_name, operation, field) => {
      const fetchMock = recordingFetch(jsonResponse(makeThreadCreationWire()));
      const client = coreClient(fetchMock);
      const promise =
        operation === 'createThread'
          ? client.createThread({
              space_id: canonicalSpaceID,
              message: 'Create safely',
              [field]: '{invalid',
            })
          : client.createRun({
              space_id: canonicalSpaceID,
              thread_id: canonicalThreadID,
              input: '{"uploaded_files":[]}',
              message_content: 'Continue safely',
              [field]: '{invalid',
            });

      const error = await expectClientError(promise);
      expect(error).toMatchObject({
        status: undefined,
        code: 'invalid_request_json',
        retryable: false,
        outcome: 'unknown',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each(['createThread', 'createRun'] as const)(
    'rejects a non-public assistant alias in %s before fetch',
    async operation => {
      const fetchMock = recordingFetch(jsonResponse(makeThreadCreationWire()));
      const client = coreClient(fetchMock);
      const promise =
        operation === 'createThread'
          ? client.createThread({
              space_id: canonicalSpaceID,
              message: 'Create safely',
              assistant_id: 'assistant-a',
            })
          : client.createRun({
              space_id: canonicalSpaceID,
              thread_id: canonicalThreadID,
              input: '{"uploaded_files":[]}',
              message_content: 'Continue safely',
              assistant_id: 'assistant-a',
            });

      const error = await expectClientError(promise);
      expect(error).toMatchObject({
        status: undefined,
        code: 'invalid_assistant_id',
        retryable: false,
        outcome: 'unknown',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each([
    [
      'retry without a source',
      { attempt_kind: 'retry' as const },
      'invalid_retry',
    ],
    [
      'turn with a retry source',
      { attempt_kind: 'turn' as const, source_run_id: canonicalRunID },
      'invalid_retry',
    ],
    [
      'retry with Message metadata',
      {
        attempt_kind: 'retry' as const,
        source_run_id: canonicalRunID,
        message_metadata: '{"source":"composer"}',
      },
      'invalid_retry',
    ],
    [
      'retry with a malformed source',
      { attempt_kind: 'retry' as const, source_run_id: 'source-opaque' },
      'invalid_resource_id',
    ],
    [
      'unknown attempt kind',
      { attempt_kind: 'resume' as 'turn' },
      'invalid_retry',
    ],
  ] as const)(
    'rejects invalid top-level retry form: %s',
    async (_name, extension, code) => {
      const fetchMock = recordingFetch(jsonResponse(makeRunCreationWire('x')));
      const client = coreClient(fetchMock);

      const error = await expectClientError(
        client.createRun({
          space_id: canonicalSpaceID,
          thread_id: canonicalThreadID,
          input: '{"uploaded_files":[]}',
          message_content: 'Continue safely',
          ...extension,
        }),
      );

      expect(error).toMatchObject({ code, outcome: 'unknown' });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it('rejects non-object Message metadata before fetch', async () => {
    const fetchMock = recordingFetch(jsonResponse(makeRunCreationWire('x')));
    const client = coreClient(fetchMock);

    const error = await expectClientError(
      client.createRun({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
        input: '{"uploaded_files":[]}',
        message_content: 'Continue safely',
        message_metadata: '[]',
      }),
    );

    expect(error).toMatchObject({
      code: 'invalid_request_shape',
      outcome: 'unknown',
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each([
    [
      'non-task retry',
      (run: Record<string, unknown>) => {
        childRecord(run, 'coze').run_kind = 'subagent';
      },
    ],
    [
      'child retry',
      (run: Record<string, unknown>) => {
        childRecord(run, 'coze').parent_run_id = '2999';
      },
    ],
    [
      'retry with a Message',
      (run: Record<string, unknown>) => {
        const coze = childRecord(run, 'coze');
        coze.message_id = '2001';
        coze.submission_message = cloneMessageWire();
      },
    ],
    [
      'retry without source',
      (run: Record<string, unknown>) => {
        childRecord(run, 'coze').source_run_id = null;
      },
    ],
  ] as const)(
    'rejects malformed message-less creation response: %s',
    async (_name, mutate) => {
      const run = makeRetryRunCreationWire();
      mutate(run);
      const fetchMock = recordingFetch(jsonResponse(run));
      const client = coreClient(fetchMock);

      const error = await expectClientError(
        client.createRun({
          space_id: canonicalSpaceID,
          thread_id: canonicalThreadID,
          input: '{"uploaded_files":[]}',
          message_content: 'Retry safely',
          attempt_kind: 'retry',
          source_run_id: canonicalRunID,
        }),
      );

      expect(error).toMatchObject({
        status: 200,
        code: 'invalid_response',
        outcome: 'unknown',
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    },
  );

  it.each([
    ['space_id', { space_id: 'space-opaque', thread_id: canonicalThreadID }],
    ['thread_id', { space_id: canonicalSpaceID, thread_id: 'thread-opaque' }],
  ] as const)(
    'rejects a non-decimal %s before path construction',
    async (_field, request) => {
      const fetchMock = recordingFetch(
        jsonResponse(threadTransportFixture.canonical),
      );
      const client = coreClient(fetchMock);

      const error = await expectClientError(client.getThread(request));
      expect(error).toMatchObject({
        code: 'invalid_resource_id',
        outcome: 'unknown',
      });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it('normalizes absent title and todos without creating presenter values', async () => {
    const thread = cloneThreadWire();
    delete childRecord(thread, 'metadata').title;
    delete childRecord(thread, 'values').todos;
    const fetchMock = recordingFetch(jsonResponse(thread));
    const client = coreClient(fetchMock);

    const visible = await client.getThread({
      space_id: canonicalSpaceID,
      thread_id: canonicalThreadID,
    });

    expect(visible).toMatchObject({ title: '' });
    expect(visible).not.toHaveProperty('values');
  });

  it.each([
    ['opaque resource id', ['thread_id'], 'thread-opaque'],
    ['unsafe integer', ['coze', 'progress'], 9007199254740992],
    ['nonexistent date', ['created_at'], '2026-02-30T00:00:00Z'],
    ['non-leap date', ['updated_at'], '2025-02-29T12:00:00+08:00'],
    ['non-finite date', ['created_at'], 'not-a-time'],
  ] as const)(
    'rejects a malformed Thread as a whole: %s',
    async (_name, path, value) => {
      const thread = cloneThreadWire();
      let owner = thread;
      for (const segment of path.slice(0, -1)) {
        owner = childRecord(owner, segment);
      }
      owner[path.at(-1) ?? ''] = value;
      const fetchMock = recordingFetch(jsonResponse(thread));
      const client = coreClient(fetchMock);

      const error = await expectClientError(
        client.getThread({
          space_id: canonicalSpaceID,
          thread_id: canonicalThreadID,
        }),
      );
      expect(error).toMatchObject({
        status: 200,
        code: 'invalid_response',
        outcome: 'unknown',
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
    },
  );

  it('keeps a valid RFC3339 leap day and offset as finite epoch milliseconds', async () => {
    const message = cloneMessageWire();
    message.created_at = '2024-02-29T12:34:56+08:00';
    const fetchMock = recordingFetch(
      jsonResponse({ data: [message], has_more: false }),
    );
    const client = coreClient(fetchMock);

    const page = await client.listMessages({
      space_id: canonicalSpaceID,
      thread_id: canonicalThreadID,
    });

    expect(page.items[0]?.created_at).toBe(Date.UTC(2024, 1, 29, 4, 34, 56));
  });

  it('rejects a legacy success envelope instead of returning a partial resource', async () => {
    const fetchMock = recordingFetch(
      jsonResponse({
        code: 0,
        msg: 'success',
        data: threadTransportFixture.canonical,
      }),
    );
    const client = coreClient(fetchMock);

    const error = await expectClientError(
      client.getThread({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
      }),
    );
    expect(error).toMatchObject({
      status: 200,
      code: 'invalid_response',
      outcome: 'unknown',
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expectNoFallback(fetchMock);
  });
});

describe('CanonicalThreadCoreClient no-fallback errors', () => {
  it.each([400, 401, 403, 404, 409, 413, 422, 429, 500])(
    'issues one canonical request for HTTP %s',
    async status => {
      const fetchMock = recordingFetch(
        jsonResponse(
          {
            detail: `HTTP ${status}`,
            code: `http_${status}`,
            retryable: status >= 500,
            trace_id: `trace-${status}`,
          },
          { status },
        ),
      );
      const client = coreClient(fetchMock);

      const error = await expectClientError(
        client.getThread({
          space_id: canonicalSpaceID,
          thread_id: canonicalThreadID,
        }),
      );

      expect(error).toMatchObject({
        status,
        code: `http_${status}`,
        traceId: `trace-${status}`,
        retryable: status >= 500,
        outcome: status >= 500 ? 'failed' : 'rejected',
      });
      expect(fetchMock).toHaveBeenCalledTimes(1);
      expect(requestSnapshot(fetchMock).url).toBe(
        '/api/workbench/threads/1001',
      );
      expectNoFallback(fetchMock);
    },
  );

  it('classifies a network reset as unknown without retry or fallback', async () => {
    const reset = new TypeError('connection reset');
    const fetchMock = recordingFetch({ error: reset });
    const client = coreClient(fetchMock);

    const error = await expectClientError(
      client.getThread({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
      }),
    );

    expect(error).toMatchObject({
      status: undefined,
      code: 'network_error',
      retryable: false,
      outcome: 'unknown',
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expectNoFallback(fetchMock);
  });

  it('does not trust a transport error classification without an HTTP response', async () => {
    const transportError = new WorkbenchClientError({
      message: 'transport supplied a misleading classification',
      status: 409,
      code: 'misleading_transport_error',
      retryable: false,
      outcome: 'rejected',
    });
    const fetchMock = recordingFetch({ error: transportError });
    const client = coreClient(fetchMock);

    const error = await expectClientError(
      client.getThread({
        space_id: canonicalSpaceID,
        thread_id: canonicalThreadID,
      }),
    );

    expect(error).not.toBe(transportError);
    expect(error).toMatchObject({
      status: undefined,
      code: 'network_error',
      retryable: false,
      outcome: 'unknown',
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expectNoFallback(fetchMock);
  });

  it('preserves the original AbortError name, code, and identity', async () => {
    const abort = Object.assign(new Error('aborted'), {
      name: 'AbortError',
      code: 'ABORT_ERR',
    });
    const fetchMock = recordingFetch({ error: abort });
    const client = coreClient(fetchMock);

    const promise = client.getThread({
      space_id: canonicalSpaceID,
      thread_id: canonicalThreadID,
    });

    await expect(promise).rejects.toBe(abort);
    expect(abort).toMatchObject({ name: 'AbortError', code: 'ABORT_ERR' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expectNoFallback(fetchMock);
  });
});
