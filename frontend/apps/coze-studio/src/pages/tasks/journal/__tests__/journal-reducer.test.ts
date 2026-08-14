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

import { describe, expect, it } from 'vitest';

import { createInitialJournalState, journalReducer } from '../journal-reducer';
import type {
  WorkbenchJournalBootstrap,
  WorkbenchJournalEvent,
  WorkbenchJournalSnapshot,
} from '../../../workbench/thread-client';

const attempt = {
  attempt_id: 'att-1',
  run_id: '3001',
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
  events: WorkbenchJournalEvent[] = [],
): WorkbenchJournalBootstrap => ({
  attempts: [{ ...attempt, latest_sequence: events.length }],
  default_attempt_id: attempt.attempt_id,
  default_attempt: { ...attempt, latest_sequence: events.length },
  projection_state: 'healthy',
  latest_sequence: events.length,
  events: {
    items: events,
    has_more: false,
    attempt_id: attempt.attempt_id,
    latest_sequence: events.length,
    next_after_sequence: events.length,
  },
  content_types: [],
  enrollment: {
    enrolled: true,
    schema_version: '1.1',
    payload_version: '1.0',
    journal_protocol_version: '1.1',
    journal_enabled: true,
    snapshots_enabled: true,
  },
  submit_at: 900,
  server_time: 1_100,
  recovery_capability: attempt.recovery_capability,
});

const event = ({
  eventId,
  eventType = 'action.started',
  parentEventId,
  sequence,
  status = 'running',
}: {
  eventId: string;
  eventType?: string;
  parentEventId?: string;
  sequence: number;
  status?: WorkbenchJournalEvent['status'];
}): WorkbenchJournalEvent => ({
  event_id: eventId,
  thread_id: '1001',
  run_id: '3001',
  event_type: eventType,
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
  created_at: 1_000 + sequence,
  schema_version: '1.1',
  attempt_id: attempt.attempt_id,
  sequence,
  status,
  payload_version: '1.0',
  ...(parentEventId ? { parent_event_id: parentEventId } : {}),
});

describe('journalReducer', () => {
  it('deduplicates events and drains an out-of-order gap only after continuity is restored', () => {
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap(),
    });

    state = journalReducer(state, {
      type: 'events_received',
      events: [event({ eventId: '102', sequence: 2 })],
    });
    expect(state.execution.events).toEqual([]);
    expect(state.execution.gap).toMatchObject({
      expected_sequence: 1,
      received_sequence: 2,
    });

    const first = event({ eventId: '101', sequence: 1 });
    state = journalReducer(state, {
      type: 'events_received',
      events: [first, first],
    });

    expect(state.execution.events.map(item => item.sequence)).toEqual([1, 2]);
    expect(state.execution.consistent_sequence).toBe(2);
    expect(state.execution.gap).toBeUndefined();
  });

  it('freezes a child whose parent is missing and never exposes the inconsistent event', () => {
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap(),
    });
    state = journalReducer(state, {
      type: 'events_received',
      events: [
        event({
          eventId: '101',
          parentEventId: '100',
          sequence: 1,
        }),
      ],
    });

    expect(state.execution.events).toEqual([]);
    expect(state.execution.gap?.reason).toBe('parent_missing');
  });

  it('does not publish an empty Journal for lifecycle-only direct tasks', () => {
    const lifecycle = event({
      eventId: '101',
      eventType: 'run.completed',
      sequence: 1,
      status: 'completed',
    });
    const state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([lifecycle]),
    });

    expect(state.execution.status).toBe('completed');
    expect(state.has_displayable_journal_content).toBe(false);
  });

  it('treats an interrupted lifecycle fact as terminal and non-displayable', () => {
    const lifecycle = event({
      eventId: '101',
      eventType: 'run.lifecycle',
      sequence: 1,
      status: 'interrupted',
    });
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([lifecycle]),
    });
    const transportStatus = state.transport.status;

    state = journalReducer(state, {
      type: 'inactivity_timeout',
      observed_at: 46_000,
    });

    expect(state.execution.status).toBe('interrupted');
    expect(state.has_displayable_journal_content).toBe(false);
    expect(state.transport.status).toBe(transportStatus);
  });

  it('publishes an intro-only Journal before the first execution step arrives', () => {
    const intro = event({
      eventId: '101',
      eventType: 'journal.intro',
      sequence: 1,
    });
    const state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([intro]),
    });

    expect(state.execution.events).toEqual([intro]);
    expect(state.has_displayable_journal_content).toBe(true);
  });

  it('keeps the Attempt running when a child action completes', () => {
    const completedAction = event({
      eventId: '101',
      eventType: 'action.terminal',
      sequence: 1,
      status: 'completed',
    });
    const state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([completedAction]),
    });

    expect(state.execution.status).toBe('running');
    expect(state.has_displayable_journal_content).toBe(true);
  });

  it('keeps a legacy public action visible without inventing a sequence cursor', () => {
    const legacy: WorkbenchJournalEvent = {
      event_id: 'legacy-101',
      thread_id: '1001',
      run_id: '3001',
      event_type: 'action.completed',
      payload: {
        operation: 'read',
        target: '需求文档',
      },
      created_at: 1_001,
    };
    const state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([legacy]),
    });

    expect(state.execution.events).toEqual([legacy]);
    expect(state.execution.consistent_sequence).toBe(0);
    expect(state.has_displayable_journal_content).toBe(true);
  });

  it('keeps execution, content, and view mode independent', () => {
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap(),
    });
    state = journalReducer(state, {
      type: 'view_mode_changed',
      mode: 'historical',
    });
    state = journalReducer(state, {
      type: 'snapshot_failed',
      error_code: 'SNAPSHOT_UNAVAILABLE',
    });
    state = journalReducer(state, {
      type: 'transport_changed',
      status: 'reconnecting',
    });

    expect(state.execution.status).toBe('running');
    expect(state.content.status).toBe('error');
    expect(state.transport.status).toBe('reconnecting');
    expect(state.view_mode).toBe('historical');

    state = journalReducer(state, {
      type: 'events_received',
      events: [event({ eventId: '101', sequence: 1 })],
    });
    expect(state.view_mode).toBe('historical');
  });

  it('does not apply a terminal event while an earlier sequence is missing', () => {
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap(),
    });
    state = journalReducer(state, {
      type: 'events_received',
      events: [
        event({
          eventId: '102',
          eventType: 'milestone.terminal',
          sequence: 2,
          status: 'completed',
        }),
      ],
    });

    expect(state.execution.status).toBe('running');
    expect(state.execution.gap).toBeDefined();
  });

  it('resets the immutable projection when the selected attempt changes', () => {
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([event({ eventId: '101', sequence: 1 })]),
    });
    state = journalReducer(state, {
      type: 'attempt_selected',
      attempt_id: 'att-2',
    });

    expect(state.execution.selected_attempt_id).toBe('att-2');
    expect(state.execution.events).toEqual([]);
    expect(state.execution.consistent_sequence).toBe(0);
    expect(state.view_mode).toBe('historical');
  });

  it('marks detail unavailable when the bounded gap buffer overflows', () => {
    let state = journalReducer(
      createInitialJournalState({
        gap_buffer_limit: 2,
      }),
      {
        type: 'bootstrap_succeeded',
        bootstrap: bootstrap(),
      },
    );
    state = journalReducer(state, {
      type: 'events_received',
      events: [
        event({ eventId: '103', sequence: 3 }),
        event({ eventId: '104', sequence: 4 }),
        event({ eventId: '105', sequence: 5 }),
      ],
    });

    expect(state.execution.detail_available).toBe(false);
    expect(state.execution.gap?.reason).toBe('buffer_overflow');
  });

  it('makes a valid snapshot displayable without changing execution status', () => {
    const snapshot: WorkbenchJournalSnapshot = {
      content_type: 'document',
      snapshot_id: 'snap-1',
      event_id: '101',
      attempt_id: attempt.attempt_id,
      is_fragmented: false,
      status: 'ready',
      created_at: 1_200,
      visibility: 'user',
      fragments: [],
      has_more: false,
      content: {
        document: {
          title: '验收报告',
          content: '# 结果',
        },
      },
    };
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap(),
    });
    state = journalReducer(state, {
      type: 'snapshot_loaded',
      snapshot,
    });

    expect(state.execution.status).toBe('running');
    expect(state.content.status).toBe('ready');
    expect(state.has_displayable_journal_content).toBe(true);
  });

  it('drops Journal detail when a runtime control disables or degrades it', () => {
    const visible = event({ eventId: '101', sequence: 1 });
    let state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: bootstrap([visible]),
    });

    state = journalReducer(state, {
      type: 'control_received',
      control: {
        type: 'journal_degraded',
        schema_version: '1.1',
        journal_protocol_version: '1.1',
        server_time: 1_200,
      },
    });

    expect(state.transport.status).toBe('degraded');
    expect(state.execution.detail_available).toBe(false);
    expect(state.execution.events).toEqual([]);
    expect(state.content.status).toBe('empty');
    expect(state.has_displayable_journal_content).toBe(false);

    state = journalReducer(state, {
      type: 'snapshot_loaded',
      snapshot: {
        snapshot_id: 'stale-snapshot',
        content_type: 'document',
        event_id: visible.event_id,
        attempt_id: attempt.attempt_id,
        is_fragmented: false,
        status: 'ready',
        created_at: 1_250,
        visibility: 'user',
        fragments: [],
        has_more: false,
        content: {
          document: { title: '过期详情', content: '不可重新显示' },
        },
      },
    });
    expect(state.content.status).toBe('empty');
    expect(state.has_displayable_journal_content).toBe(false);

    state = journalReducer(state, {
      type: 'control_received',
      control: {
        type: 'journal_disabled',
        schema_version: '1.1',
        journal_protocol_version: '1.1',
        server_time: 1_300,
      },
    });

    expect(state.transport.status).toBe('disabled');
    expect(state.has_displayable_journal_content).toBe(false);
  });

  it('does not expose bootstrap events from a degraded projection', () => {
    const degraded = {
      ...bootstrap([event({ eventId: '101', sequence: 1 })]),
      projection_state: 'degraded' as const,
    };
    const state = journalReducer(createInitialJournalState(), {
      type: 'bootstrap_succeeded',
      bootstrap: degraded,
    });

    expect(state.transport.status).toBe('degraded');
    expect(state.execution.detail_available).toBe(false);
    expect(state.execution.events).toEqual([]);
    expect(state.has_displayable_journal_content).toBe(false);
  });
});
