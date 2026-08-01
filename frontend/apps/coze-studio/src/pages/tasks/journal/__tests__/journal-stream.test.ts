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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  createJournalCursorStore,
  createJournalStreamController,
  type JournalStreamClient,
} from '../journal-stream';
import {
  createInitialJournalState,
  journalReducer,
  type JournalAction,
  type JournalState,
} from '../journal-reducer';
import { WorkbenchClientError } from '../../../workbench/thread-client/canonical-fetch';
import type {
  WorkbenchJournalBootstrap,
  WorkbenchJournalEvent,
  WorkbenchJournalStreamMessage,
} from '../../../workbench/thread-client';

const scope = {
  space_id: '9001',
  thread_id: '1001',
  run_id: '3001',
};

const attempt = {
  attempt_id: 'att-1',
  run_id: scope.run_id,
  status: 'running' as const,
  projection_state: 'healthy' as const,
  latest_sequence: 0,
  created_at: 1_000,
  recovery_capability: {
    allowed: false,
    requires_confirmation: false,
    allowed_actions: [],
  },
};

const bootstrap = (
  enabled = true,
  projectionState: 'healthy' | 'degraded' | 'disabled' = 'healthy',
): WorkbenchJournalBootstrap => ({
  attempts: [attempt],
  default_attempt_id: attempt.attempt_id,
  default_attempt: attempt,
  projection_state: projectionState,
  latest_sequence: 0,
  events: {
    items: [],
    has_more: false,
    attempt_id: attempt.attempt_id,
    latest_sequence: 0,
    next_after_sequence: 0,
  },
  content_types: [],
  enrollment: {
    enrolled: enabled,
    schema_version: '1.1',
    payload_version: '1.0',
    journal_protocol_version: '1.1',
    journal_enabled: enabled,
    snapshots_enabled: enabled,
  },
  submit_at: 900,
  server_time: 1_100,
  recovery_capability: attempt.recovery_capability,
});

interface CapturedSubscription {
  request: Parameters<JournalStreamClient['subscribeJournalEvents']>[0];
  close: ReturnType<typeof vi.fn>;
}

const createClient = ({
  journal = bootstrap(),
}: {
  journal?: WorkbenchJournalBootstrap;
} = {}) => {
  const subscriptions: CapturedSubscription[] = [];
  const client: JournalStreamClient = {
    getRunJournal: vi.fn(() => Promise.resolve(journal)),
    listJournalEvents: vi.fn(() =>
      Promise.resolve({
        items: [],
        has_more: false,
        attempt_id: attempt.attempt_id,
        latest_sequence: 0,
        next_after_sequence: 0,
      }),
    ),
    subscribeJournalEvents: vi.fn(request => {
      const captured = { request, close: vi.fn() };
      subscriptions.push(captured);
      return { close: captured.close, closed: Promise.resolve() };
    }),
  };
  return { client, subscriptions };
};

const createStateOwner = () => {
  let state = createInitialJournalState();
  return {
    get state() {
      return state;
    },
    reduce(action: JournalAction): JournalState {
      state = journalReducer(state, action);
      return state;
    },
  };
};

const metadata = (): WorkbenchJournalStreamMessage => ({
  kind: 'metadata',
  metadata: {
    thread_id: scope.thread_id,
    run_id: scope.run_id,
    attempt_id: attempt.attempt_id,
    latest_sequence: 0,
    submit_at: 900,
    server_time: 1_100,
    journal_enabled: true,
    snapshots_enabled: true,
    journal_protocol_version: '1.1',
  },
});

const actionEvent = (
  sequence: number,
  status: 'running' | 'completed' = 'running',
): WorkbenchJournalEvent => ({
  event_id: String(100 + sequence),
  thread_id: scope.thread_id,
  run_id: scope.run_id,
  event_type: status === 'completed' ? 'action.completed' : 'action.started',
  payload: {
    type: 'terminal',
    data: {
      action_id: `action-${sequence}`,
      operation: 'terminal',
      target: `command-${sequence}`,
      display_verb_running: '正在执行',
      display_verb_completed: '已执行',
      content_type: 'terminal',
    },
  },
  created_at: 1_000 + sequence,
  occurred_at: 1_000 + sequence,
  attempt_id: attempt.attempt_id,
  sequence,
  status,
  visibility: 'user',
  payload_version: '1.0',
});

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('Journal stream controller', () => {
  it('loads the explicitly selected historical Attempt without opening a live stream', async () => {
    const historicalAttempt = {
      ...attempt,
      attempt_id: 'att-0',
      status: 'failed' as const,
      latest_sequence: 1,
      created_at: 900,
    };
    const historicalEvent = {
      ...actionEvent(1, 'completed'),
      attempt_id: historicalAttempt.attempt_id,
    };
    const { client } = createClient({
      journal: {
        ...bootstrap(),
        attempts: [historicalAttempt, attempt],
        events: {
          items: [historicalEvent],
          has_more: false,
          attempt_id: historicalAttempt.attempt_id,
          latest_sequence: 1,
        },
      },
    });
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      attemptId: historicalAttempt.attempt_id,
      reduce: action => owner.reduce(action),
    });

    await controller.start();

    expect(client.getRunJournal).toHaveBeenCalledWith(
      expect.objectContaining({ attempt_id: historicalAttempt.attempt_id }),
    );
    expect(owner.state.execution.selected_attempt_id).toBe(
      historicalAttempt.attempt_id,
    );
    expect(owner.state.execution.events).toEqual([historicalEvent]);
    expect(client.subscribeJournalEvents).not.toHaveBeenCalled();
  });

  it('reconnects after 1/2/4 seconds and then falls back to 2-second polling', async () => {
    const { client, subscriptions } = createClient();
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
      cursorStore: createJournalCursorStore({
        storage: null,
        memory: new Map(),
      }),
    });

    await controller.start();
    expect(subscriptions).toHaveLength(1);
    subscriptions[0]?.request.onMessage(metadata());

    subscriptions[0]?.request.onError(new Error('disconnect-0'));
    await vi.advanceTimersByTimeAsync(999);
    expect(subscriptions).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(subscriptions).toHaveLength(2);

    subscriptions[1]?.request.onError(new Error('disconnect-1'));
    await vi.advanceTimersByTimeAsync(2_000);
    expect(subscriptions).toHaveLength(3);

    subscriptions[2]?.request.onError(new Error('disconnect-2'));
    await vi.advanceTimersByTimeAsync(4_000);
    expect(subscriptions).toHaveLength(4);

    subscriptions[3]?.request.onError(new Error('disconnect-3'));
    await Promise.resolve();
    expect(client.listJournalEvents).toHaveBeenCalledTimes(1);
    expect(owner.state.transport.status).toBe('polling');

    await vi.advanceTimersByTimeAsync(2_000);
    expect(client.listJournalEvents).toHaveBeenCalledTimes(2);
    controller.stop();
  });

  it('does not open an empty stream when enrollment is disabled', async () => {
    const { client } = createClient({ journal: bootstrap(false, 'disabled') });
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
    });

    await controller.start();

    expect(client.subscribeJournalEvents).not.toHaveBeenCalled();
    expect(owner.state.transport.status).toBe('disabled');
    expect(owner.state.has_displayable_journal_content).toBe(false);
  });

  it('drops bootstrapped detail when stream metadata revokes capability', async () => {
    const visibleEvent = actionEvent(1);
    const visibleAttempt = { ...attempt, latest_sequence: 1 };
    const { client, subscriptions } = createClient({
      journal: {
        ...bootstrap(),
        attempts: [visibleAttempt],
        default_attempt: visibleAttempt,
        latest_sequence: 1,
        events: {
          items: [visibleEvent],
          has_more: false,
          attempt_id: attempt.attempt_id,
          latest_sequence: 1,
          next_after_sequence: 1,
        },
      },
    });
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
    });

    await controller.start();
    expect(owner.state.has_displayable_journal_content).toBe(true);
    subscriptions[0]?.request.onMessage({
      kind: 'metadata',
      metadata: {
        ...metadata().metadata,
        journal_enabled: false,
      },
    });

    expect(owner.state.transport.status).toBe('disabled');
    expect(owner.state.execution.events).toEqual([]);
    expect(owner.state.has_displayable_journal_content).toBe(false);
  });

  it('reloads from bootstrap when a polling cursor expires', async () => {
    const { client, subscriptions } = createClient();
    vi.mocked(client.listJournalEvents).mockRejectedValueOnce(
      new WorkbenchClientError({
        message: 'expired',
        status: 410,
        code: 'JOURNAL_CURSOR_EXPIRED',
        retryable: true,
        outcome: 'rejected',
      }),
    );
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
    });

    await controller.start();
    subscriptions[0]?.request.onError(new Error('disconnect-0'));
    await vi.advanceTimersByTimeAsync(1_000);
    subscriptions[1]?.request.onError(new Error('disconnect-1'));
    await vi.advanceTimersByTimeAsync(2_000);
    subscriptions[2]?.request.onError(new Error('disconnect-2'));
    await vi.advanceTimersByTimeAsync(4_000);
    subscriptions[3]?.request.onError(new Error('disconnect-3'));
    await Promise.resolve();
    await Promise.resolve();

    expect(client.getRunJournal).toHaveBeenCalledTimes(2);
    expect(owner.state.execution.selected_attempt_id).toBe('att-1');
    controller.stop();
  });

  it('persists event-id and attempt sequence together without touching the main Run cursor', () => {
    const memory = new Map<string, string>();
    const store = createJournalCursorStore({ storage: null, memory });
    store.write(scope, {
      attempt_id: 'att-1',
      sequence: 7,
      event_id: '900719925474099312345',
    });

    expect(store.read(scope)).toEqual({
      attempt_id: 'att-1',
      sequence: 7,
      event_id: '900719925474099312345',
    });
    expect(Array.from(memory.keys())).toEqual([
      'coze:workbench:journal-cursor:canonical_v1:9001:1001:3001',
    ]);
  });

  it('waits for 45 seconds of inactivity before starting the reconnect ladder', async () => {
    const { client, subscriptions } = createClient();
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
    });

    await controller.start();
    subscriptions[0]?.request.onMessage(metadata());

    await vi.advanceTimersByTimeAsync(44_999);
    expect(subscriptions[0]?.close).not.toHaveBeenCalled();
    expect(subscriptions).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1);
    expect(subscriptions[0]?.close).toHaveBeenCalledTimes(1);
    expect(owner.state.transport.status).toBe('reconnecting');

    await vi.advanceTimersByTimeAsync(999);
    expect(subscriptions).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(subscriptions).toHaveLength(2);
    controller.stop();
  });

  it('respects Retry-After before reconnecting a rate-limited stream', async () => {
    const rateLimited = createClient();
    const rateOwner = createStateOwner();
    const rateController = createJournalStreamController({
      client: rateLimited.client,
      scope,
      reduce: action => rateOwner.reduce(action),
    });
    await rateController.start();
    rateLimited.subscriptions[0]?.request.onMessage(metadata());
    rateLimited.subscriptions[0]?.request.onError(
      new WorkbenchClientError({
        message: 'limited',
        status: 429,
        code: 'JOURNAL_RATE_LIMITED',
        retryable: true,
        retryAfterMs: 5_000,
        outcome: 'rejected',
      }),
    );

    await vi.advanceTimersByTimeAsync(4_999);
    expect(rateLimited.subscriptions).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(rateLimited.subscriptions).toHaveLength(2);
    rateController.stop();
  });

  it('stops all retries after the server degrades Journal detail', async () => {
    const { client, subscriptions } = createClient();
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
    });

    await controller.start();
    subscriptions[0]?.request.onMessage(metadata());
    subscriptions[0]?.request.onMessage({
      kind: 'control',
      control: {
        type: 'journal_degraded',
        schema_version: '1.1',
        journal_protocol_version: '1.1',
        server_time: 1_200,
      },
    });

    expect(owner.state.transport.status).toBe('degraded');
    expect(subscriptions[0]?.close).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(subscriptions).toHaveLength(1);
    expect(client.listJournalEvents).not.toHaveBeenCalled();
  });

  it('backfills a sequence gap before accepting a terminal event', async () => {
    const memory = new Map<string, string>();
    const { client, subscriptions } = createClient();
    vi.mocked(client.listJournalEvents).mockResolvedValueOnce({
      items: [actionEvent(1)],
      has_more: false,
      attempt_id: attempt.attempt_id,
      latest_sequence: 2,
      next_after_sequence: 1,
    });
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
      cursorStore: createJournalCursorStore({ storage: null, memory }),
    });

    await controller.start();
    subscriptions[0]?.request.onMessage(metadata());
    subscriptions[0]?.request.onMessage({
      kind: 'event',
      event: actionEvent(2, 'completed'),
    });
    subscriptions[0]?.request.onMessage({
      kind: 'end',
      attempt_id: attempt.attempt_id,
      status: 'completed',
      latest_sequence: 2,
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(client.listJournalEvents).toHaveBeenCalledTimes(1);
    expect(owner.state.execution.gap).toBeUndefined();
    expect(owner.state.execution.consistent_sequence).toBe(2);
    expect(owner.state.transport.status).toBe('ended');
    expect(
      createJournalCursorStore({ storage: null, memory }).read(scope),
    ).toEqual({
      attempt_id: attempt.attempt_id,
      sequence: 2,
      event_id: '102',
    });
  });

  it('closes the live stream before a failed gap backfill falls back to polling', async () => {
    const { client, subscriptions } = createClient();
    vi.mocked(client.listJournalEvents).mockRejectedValueOnce(
      new Error('backfill unavailable'),
    );
    const owner = createStateOwner();
    const controller = createJournalStreamController({
      client,
      scope,
      reduce: action => owner.reduce(action),
    });

    await controller.start();
    subscriptions[0]?.request.onMessage(metadata());
    subscriptions[0]?.request.onMessage({
      kind: 'event',
      event: actionEvent(2),
    });
    await Promise.resolve();
    await Promise.resolve();

    expect(owner.state.transport.status).toBe('polling');
    expect(subscriptions[0]?.close).toHaveBeenCalledTimes(1);
    controller.stop();
  });
});
