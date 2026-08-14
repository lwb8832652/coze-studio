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

import type { WorkbenchJournalStreamMessage } from '../types';
import {
  CanonicalThreadCoreClient,
  type CanonicalFetchStream,
} from '../canonical-thread-client';
import type { CanonicalFetch } from '../canonical-fetch';
import { adaptCanonicalJournalEvent } from '../adapters/canonical-thread-adapter';

const spaceID = '9001';
const threadID = '1001';
const runID = '3001';

const actionEvent = {
  event_id: '101',
  thread_id: threadID,
  run_id: runID,
  event_type: 'action.started',
  payload: {
    type: 'generic',
    data: {
      action_id: 'action-1',
      operation: 'read',
      target: '需求文档',
      display_verb_running: '正在读取',
      display_verb_completed: '已读取',
    },
  },
  created_at: '2026-07-30T06:32:10.000Z',
  schema_version: '1.1',
  attempt_id: 'att-1',
  sequence: 1,
  status: 'running',
  occurred_at: '2026-07-30T06:32:09.900Z',
  visibility: 'user',
  payload_version: '1.0',
};

const attempt = {
  attempt_id: 'att-1',
  run_id: runID,
  status: 'running',
  projection_state: 'healthy',
  latest_sequence: 1,
  created_at: '2026-07-30T06:32:09.000Z',
  started_at: '2026-07-30T06:32:09.100Z',
  recovery_capability: {
    allowed: false,
    requires_confirmation: false,
    allowed_actions: [],
  },
};

const bootstrap = {
  attempts: [attempt],
  default_attempt_id: attempt.attempt_id,
  default_attempt: attempt,
  projection_state: 'healthy',
  latest_sequence: 1,
  events: {
    data: [actionEvent],
    has_more: false,
    attempt_id: attempt.attempt_id,
    latest_sequence: 1,
    next_after_sequence: 1,
    next_after_event_id: '101',
  },
  content_types: ['document', 'terminal', 'code', 'skill', 'browser'],
  enrollment: {
    enrolled: true,
    schema_version: '1.1',
    payload_version: '1.0',
    journal_protocol_version: '1.1',
    journal_enabled: true,
    snapshots_enabled: true,
  },
  submit_at: '2026-07-30T06:32:08.000Z',
  server_time: '2026-07-30T06:32:11.000Z',
  recovery_capability: attempt.recovery_capability,
};

const jsonResponse = (body: unknown, init: ResponseInit = {}) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'content-type': 'application/json' },
    ...init,
  });

type StreamConfig = FetchSteamConfig<WorkbenchJournalStreamMessage>;

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
    config: () => {
      if (!config) {
        throw new Error('Missing Journal stream configuration');
      }
      return config;
    },
  };
};

const emitFrame = (config: StreamConfig, frame: unknown) => {
  const message = config.streamParser?.(frame as never, {
    terminate: vi.fn(),
    onParseError: vi.fn(),
  });
  if (message !== undefined) {
    config.onMessage?.({ message });
  }
};

describe('canonical Journal client', () => {
  it('strictly adapts a public Journal action event', () => {
    expect(
      adaptCanonicalJournalEvent(actionEvent, {
        spaceId: spaceID,
        threadId: threadID,
        runId: runID,
      }),
    ).toMatchObject({ event_id: '101', sequence: 1 });
  });

  it('accepts the additive interrupted Journal execution status', () => {
    expect(
      adaptCanonicalJournalEvent(
        {
          ...actionEvent,
          event_type: 'run.lifecycle',
          status: 'interrupted',
        },
        {
          spaceId: spaceID,
          threadId: threadID,
          runId: runID,
        },
      ),
    ).toMatchObject({ event_type: 'run.lifecycle', status: 'interrupted' });
  });

  it('loads a strict v1.1 bootstrap with independent event and sequence cursors', async () => {
    const fetch = vi.fn<CanonicalFetch>(() =>
      Promise.resolve(jsonResponse(bootstrap)),
    );
    const client = new CanonicalThreadCoreClient({ fetch });

    const result = await client.getRunJournal({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      attempt_id: 'att-1',
      after_event_id: '99',
      after_sequence: 0,
      limit: 200,
    });

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(String(fetch.mock.calls[0]?.[0])).toBe(
      `/api/workbench/threads/${threadID}/runs/${runID}/journal?` +
        'attempt_id=att-1&after_sequence=0&after_event_id=99&limit=200&' +
        'journal_protocol_version=1.1',
    );
    expect(result.events.items[0]).toMatchObject({
      event_id: '101',
      sequence: 1,
      payload: actionEvent.payload,
    });
    expect(result.server_time).toBe(Date.parse(bootstrap.server_time));
  });

  it('sends only identifiers, frozen action, and a fresh idempotency key for snapshot audit', async () => {
    const fetch = vi.fn<CanonicalFetch>(() =>
      Promise.resolve(
        jsonResponse({
          snapshot_id: 'snap-1',
          action: 'copy_output',
          allowed: true,
          audited_at: '2026-07-30T06:32:12.000Z',
          copy_text: 'public output',
        }),
      ),
    );
    const client = new CanonicalThreadCoreClient({ fetch });

    await client.auditJournalSnapshotAction({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      snapshot_id: 'snap-1',
      action: 'copy_output',
      fragment_id: 'fragment-1',
      idempotency_key: 'journal-action-1',
    });

    const request = fetch.mock.calls[0]?.[1];
    expect(request?.headers).toMatchObject({
      'Idempotency-Key': 'journal-action-1',
      'X-Coze-Space-ID': spaceID,
    });
    const body = JSON.parse(String(request?.body)) as Record<string, unknown>;
    expect(body).toEqual({
      action: 'copy_output',
      fragment_id: 'fragment-1',
    });
    expect(body).not.toHaveProperty('content');
    expect(body).not.toHaveProperty('url');
    expect(body).not.toHaveProperty('path');
  });

  it('accepts the Journal compatibility error field and preserves Retry-After', async () => {
    const fetch = vi.fn<CanonicalFetch>(() =>
      Promise.resolve(
        jsonResponse(
          {
            detail: 'Journal request rate limited',
            error_code: 'JOURNAL_RATE_LIMITED',
            code: 'JOURNAL_RATE_LIMITED',
            retryable: true,
            trace_id: 'trace-1',
          },
          {
            status: 429,
            headers: {
              'content-type': 'application/json',
              'Retry-After': '3',
            },
          },
        ),
      ),
    );

    await expect(
      new CanonicalThreadCoreClient({ fetch }).getRunJournal({
        space_id: spaceID,
        thread_id: threadID,
        run_id: runID,
      }),
    ).rejects.toMatchObject({
      code: 'JOURNAL_RATE_LIMITED',
      retryable: true,
      retryAfterMs: 3_000,
      traceId: 'trace-1',
    });
  });

  it('parses metadata, event, control, and terminal frames on the dedicated subscription', async () => {
    const harness = streamHarness();
    const onMessage = vi.fn();
    const onError = vi.fn();
    const subscription = new CanonicalThreadCoreClient({
      stream: harness.stream,
    }).subscribeJournalEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      attempt_id: 'att-1',
      after_event_id: '99',
      after_sequence: 0,
      signal: new AbortController().signal,
      onMessage,
      onError,
    });

    expect(String(vi.mocked(harness.stream).mock.calls[0]?.[0])).toBe(
      `/api/workbench/threads/${threadID}/runs/${runID}/stream?` +
        'journal_protocol_version=1.1&attempt_id=att-1&after_sequence=0&' +
        'after_event_id=99&cancel_on_disconnect=false',
    );

    emitFrame(harness.config(), {
      type: 'event',
      event: 'metadata',
      data: JSON.stringify({
        thread_id: threadID,
        run_id: runID,
        attempt_id: 'att-1',
        latest_sequence: 1,
        submit_at: bootstrap.submit_at,
        server_time: bootstrap.server_time,
        journal_enabled: true,
        snapshots_enabled: true,
        journal_protocol_version: '1.1',
      }),
    });
    emitFrame(harness.config(), {
      type: 'event',
      event: 'events',
      id: '101',
      data: JSON.stringify({ kind: 'event', event: actionEvent }),
    });
    expect(onError.mock.calls).toEqual([]);
    expect(onMessage.mock.calls.map(call => call[0].kind)).toEqual([
      'metadata',
      'event',
    ]);
    emitFrame(harness.config(), {
      type: 'event',
      event: 'control',
      data: JSON.stringify({
        kind: 'control',
        control: {
          type: 'journal_degraded',
          schema_version: '1.1',
          journal_protocol_version: '1.1',
          server_time: bootstrap.server_time,
          attempt_id: 'att-1',
          latest_sequence: 1,
          error_code: 'JOURNAL_RATE_LIMITED',
          retryable: true,
        },
      }),
    });
    emitFrame(harness.config(), {
      type: 'event',
      event: 'end',
      data: JSON.stringify({
        attempt_id: 'att-1',
        status: 'completed',
        latest_sequence: 1,
        reason: 'terminal_attempt',
      }),
    });

    await subscription.closed;
    expect(onMessage.mock.calls.map(call => call[0].kind)).toEqual([
      'metadata',
      'event',
      'control',
      'end',
    ]);
    expect(onError).not.toHaveBeenCalled();
    expect(harness.config().signal?.aborted).toBe(true);
  });
});
