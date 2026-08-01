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

import type {
  WorkbenchJournalAttempt,
  WorkbenchJournalBootstrap,
  WorkbenchJournalContentStatus,
  WorkbenchJournalControl,
  WorkbenchJournalEvent,
  WorkbenchJournalExecutionStatus,
  WorkbenchJournalSnapshot,
} from '../../workbench/thread-client';

export type JournalTransportStatus =
  | 'idle'
  | 'bootstrapping'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'polling'
  | 'ended'
  | 'disabled'
  | 'degraded'
  | 'error';

export type JournalViewMode = 'live_follow' | 'live_paused' | 'historical';

export interface JournalGap {
  expected_sequence: number;
  received_sequence: number;
  reason: 'sequence_missing' | 'parent_missing' | 'buffer_overflow';
}

export interface JournalTransportState {
  status: JournalTransportStatus;
  reconnect_attempt: number;
  last_activity_at?: number;
  error_code?: string;
}

export interface JournalExecutionState {
  attempts: WorkbenchJournalAttempt[];
  selected_attempt_id?: string;
  status: WorkbenchJournalExecutionStatus;
  events: WorkbenchJournalEvent[];
  consistent_sequence: number;
  pending_events: WorkbenchJournalEvent[];
  gap?: JournalGap;
  detail_available: boolean;
}

export interface JournalContentState {
  status: WorkbenchJournalContentStatus;
  selected_snapshot_id?: string;
  snapshot?: WorkbenchJournalSnapshot;
  error_code?: string;
}

export interface JournalState {
  transport: JournalTransportState;
  execution: JournalExecutionState;
  content: JournalContentState;
  view_mode: JournalViewMode;
  has_displayable_journal_content: boolean;
  submit_at?: number;
  server_time?: number;
  gap_buffer_limit: number;
}

export type JournalAction =
  | { type: 'bootstrap_started' }
  | { type: 'bootstrap_succeeded'; bootstrap: WorkbenchJournalBootstrap }
  | { type: 'events_received'; events: WorkbenchJournalEvent[] }
  | {
      type: 'transport_changed';
      status: JournalTransportStatus;
      reconnect_attempt?: number;
      error_code?: string;
      activity_at?: number;
    }
  | { type: 'control_received'; control: WorkbenchJournalControl }
  | { type: 'attempt_selected'; attempt_id: string }
  | { type: 'view_mode_changed'; mode: JournalViewMode }
  | { type: 'snapshot_loading'; snapshot_id: string }
  | { type: 'snapshot_loaded'; snapshot: WorkbenchJournalSnapshot }
  | { type: 'snapshot_failed'; error_code: string; no_permission?: boolean }
  | { type: 'inactivity_timeout'; observed_at: number }
  | { type: 'reset' };

const defaultGapBufferLimit = 256;
const terminalStatuses = new Set<WorkbenchJournalExecutionStatus>([
  'completed',
  'failed',
  'cancelled',
  'timed_out',
]);

export const createInitialJournalState = (
  options: { gap_buffer_limit?: number } = {},
): JournalState => ({
  transport: {
    status: 'idle',
    reconnect_attempt: 0,
  },
  execution: {
    attempts: [],
    status: 'pending',
    events: [],
    consistent_sequence: 0,
    pending_events: [],
    detail_available: true,
  },
  content: { status: 'empty' },
  view_mode: 'live_follow',
  has_displayable_journal_content: false,
  gap_buffer_limit: options.gap_buffer_limit ?? defaultGapBufferLimit,
});

const isDisplayableEvent = (event: WorkbenchJournalEvent): boolean =>
  ['milestone.', 'action.', 'artifact.', 'verification.', 'confirmation.'].some(
    prefix => event.event_type.startsWith(prefix),
  );

const eventSequence = (event: WorkbenchJournalEvent): number | undefined =>
  Number.isSafeInteger(event.sequence) && (event.sequence ?? -1) >= 0
    ? event.sequence
    : undefined;

const eventOrder = (
  left: WorkbenchJournalEvent,
  right: WorkbenchJournalEvent,
): number => {
  const leftSequence = eventSequence(left);
  const rightSequence = eventSequence(right);
  if (leftSequence !== undefined && rightSequence !== undefined) {
    return leftSequence - rightSequence;
  }
  if (left.created_at !== right.created_at) {
    return left.created_at - right.created_at;
  }
  return left.event_id.localeCompare(right.event_id);
};

const uniqueEvents = (
  current: WorkbenchJournalEvent[],
  incoming: WorkbenchJournalEvent[],
): WorkbenchJournalEvent[] => {
  const known = new Set(current.map(event => event.event_id));
  return incoming.filter(event => {
    if (known.has(event.event_id)) {
      return false;
    }
    known.add(event.event_id);
    return true;
  });
};

const pendingWithIncoming = (
  state: JournalExecutionState,
  incoming: WorkbenchJournalEvent[],
): WorkbenchJournalEvent[] => {
  const pending = new Map<number, WorkbenchJournalEvent>();
  state.pending_events.forEach(event => {
    const sequence = eventSequence(event);
    if (sequence !== undefined) {
      pending.set(sequence, event);
    }
  });
  incoming.forEach(event => {
    const sequence = eventSequence(event);
    if (
      sequence !== undefined &&
      sequence > state.consistent_sequence &&
      !pending.has(sequence)
    ) {
      pending.set(sequence, event);
    }
  });
  return Array.from(pending.values()).sort(eventOrder);
};

const deriveGap = (
  expected: number,
  pending: WorkbenchJournalEvent[],
  knownEventIDs: Set<string>,
): JournalGap | undefined => {
  const next = pending[0];
  const received = next ? eventSequence(next) : undefined;
  if (next && received === expected && next.parent_event_id) {
    if (!knownEventIDs.has(next.parent_event_id)) {
      return {
        expected_sequence: expected,
        received_sequence: received,
        reason: 'parent_missing',
      };
    }
  }
  return received !== undefined && received > expected
    ? {
        expected_sequence: expected,
        received_sequence: received,
        reason: 'sequence_missing',
      }
    : undefined;
};

const applySequencedEvents = (
  state: JournalExecutionState,
  incoming: WorkbenchJournalEvent[],
  gapBufferLimit: number,
): JournalExecutionState => {
  const legacy = uniqueEvents(
    [...state.events, ...state.pending_events],
    incoming.filter(event => eventSequence(event) === undefined),
  );
  const events = [...state.events, ...legacy].sort(eventOrder);
  const pending = pendingWithIncoming(
    state,
    uniqueEvents(
      [...events, ...state.pending_events],
      incoming.filter(event => eventSequence(event) !== undefined),
    ),
  );
  if (pending.length > gapBufferLimit) {
    return {
      ...state,
      events,
      pending_events: pending.slice(0, gapBufferLimit),
      gap: {
        expected_sequence: state.consistent_sequence + 1,
        received_sequence:
          eventSequence(pending[0]) ?? state.consistent_sequence + 1,
        reason: 'buffer_overflow',
      },
      detail_available: false,
    };
  }

  let consistentSequence = state.consistent_sequence;
  let { status } = state;
  const applied = [...events];
  const remaining = [...pending];
  const knownEventIDs = new Set(applied.map(event => event.event_id));
  while (remaining.length > 0) {
    const next = remaining[0];
    const sequence = eventSequence(next);
    if (sequence !== consistentSequence + 1) {
      break;
    }
    if (next.parent_event_id && !knownEventIDs.has(next.parent_event_id)) {
      break;
    }
    remaining.shift();
    applied.push(next);
    knownEventIDs.add(next.event_id);
    consistentSequence = sequence;
    if (next.status) {
      status = next.status;
    }
  }

  return {
    ...state,
    status,
    events: applied.sort(eventOrder),
    consistent_sequence: consistentSequence,
    pending_events: remaining,
    gap: deriveGap(consistentSequence + 1, remaining, knownEventIDs),
  };
};

const stateWithEvents = (
  state: JournalState,
  incoming: WorkbenchJournalEvent[],
): JournalState => {
  const matching = state.execution.selected_attempt_id
    ? incoming.filter(
        event =>
          !event.attempt_id ||
          event.attempt_id === state.execution.selected_attempt_id,
      )
    : incoming;
  const execution = applySequencedEvents(
    state.execution,
    matching,
    state.gap_buffer_limit,
  );
  return {
    ...state,
    execution,
    has_displayable_journal_content:
      state.has_displayable_journal_content ||
      execution.events.some(isDisplayableEvent),
  };
};

const bootstrapState = (
  state: JournalState,
  bootstrap: WorkbenchJournalBootstrap,
): JournalState => {
  const selected =
    bootstrap.default_attempt ??
    bootstrap.attempts.find(
      attempt => attempt.attempt_id === bootstrap.default_attempt_id,
    );
  const reset: JournalState = {
    ...state,
    transport: {
      status:
        bootstrap.projection_state === 'disabled'
          ? 'disabled'
          : bootstrap.projection_state === 'degraded'
            ? 'degraded'
            : 'connecting',
      reconnect_attempt: 0,
    },
    execution: {
      attempts: bootstrap.attempts,
      selected_attempt_id: selected?.attempt_id,
      status: selected?.status ?? 'pending',
      events: [],
      consistent_sequence: 0,
      pending_events: [],
      detail_available: true,
    },
    content: { status: 'empty' },
    view_mode: 'live_follow',
    has_displayable_journal_content: false,
    submit_at: bootstrap.submit_at,
    server_time: bootstrap.server_time,
  };
  return stateWithEvents(reset, bootstrap.events.items);
};

const controlState = (
  state: JournalState,
  control: WorkbenchJournalControl,
): JournalState => {
  if (control.type === 'journal_disabled') {
    return {
      ...state,
      transport: {
        ...state.transport,
        status: 'disabled',
        error_code: control.error_code,
      },
    };
  }
  if (control.type === 'journal_degraded') {
    return {
      ...state,
      transport: {
        ...state.transport,
        status: 'degraded',
        error_code: control.error_code,
      },
    };
  }
  return {
    ...state,
    transport: {
      ...state.transport,
      status: 'error',
      error_code: control.error_code ?? control.type,
    },
  };
};

export const journalReducer = (
  state: JournalState,
  action: JournalAction,
): JournalState => {
  switch (action.type) {
    case 'bootstrap_started':
      return {
        ...state,
        transport: { status: 'bootstrapping', reconnect_attempt: 0 },
      };
    case 'bootstrap_succeeded':
      return bootstrapState(state, action.bootstrap);
    case 'events_received':
      return stateWithEvents(state, action.events);
    case 'transport_changed':
      return {
        ...state,
        transport: {
          status: action.status,
          reconnect_attempt:
            action.reconnect_attempt ?? state.transport.reconnect_attempt,
          ...(action.error_code ? { error_code: action.error_code } : {}),
          ...(action.activity_at
            ? { last_activity_at: action.activity_at }
            : {}),
        },
      };
    case 'control_received':
      return controlState(state, action.control);
    case 'attempt_selected': {
      const selected = state.execution.attempts.find(
        attempt => attempt.attempt_id === action.attempt_id,
      );
      return {
        ...state,
        execution: {
          ...state.execution,
          selected_attempt_id: action.attempt_id,
          status: selected?.status ?? 'pending',
          events: [],
          consistent_sequence: 0,
          pending_events: [],
          gap: undefined,
          detail_available: true,
        },
        content: { status: 'empty' },
        view_mode: 'historical',
        has_displayable_journal_content: false,
      };
    }
    case 'view_mode_changed':
      return { ...state, view_mode: action.mode };
    case 'snapshot_loading':
      return {
        ...state,
        content: {
          status: 'loading',
          selected_snapshot_id: action.snapshot_id,
        },
      };
    case 'snapshot_loaded':
      return {
        ...state,
        content: {
          status: action.snapshot.status,
          selected_snapshot_id: action.snapshot.snapshot_id,
          snapshot: action.snapshot,
        },
        has_displayable_journal_content: true,
      };
    case 'snapshot_failed':
      return {
        ...state,
        content: {
          ...state.content,
          status: action.no_permission ? 'no_permission' : 'error',
          error_code: action.error_code,
          snapshot: undefined,
        },
      };
    case 'inactivity_timeout':
      return terminalStatuses.has(state.execution.status)
        ? state
        : {
            ...state,
            transport: {
              ...state.transport,
              status: 'reconnecting',
              last_activity_at: action.observed_at,
            },
          };
    case 'reset':
      return createInitialJournalState({
        gap_buffer_limit: state.gap_buffer_limit,
      });
    default:
      return state;
  }
};
