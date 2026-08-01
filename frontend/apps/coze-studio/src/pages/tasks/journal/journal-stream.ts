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

/* eslint-disable max-lines -- Stream recovery, cursor fencing, and terminal ordering form one state machine. */

import { WorkbenchClientError } from '../../workbench/thread-client/canonical-fetch';
import type {
  JournalEventSubscription,
  WorkbenchJournalEventPage,
  WorkbenchJournalMetadata,
  WorkbenchJournalStreamMessage,
} from '../../workbench/thread-client';
import { eventAtSequence, journalRetryDelay } from './journal-stream-utils';
import type {
  JournalStreamController,
  JournalStreamControllerOptions,
} from './journal-stream-contract';
import type { JournalAction, JournalState } from './journal-reducer';
import {
  createJournalCursorStore,
  type JournalCursor,
  type JournalCursorStore,
} from './journal-cursor';

export { createJournalCursorStore } from './journal-cursor';
export type {
  JournalCursor,
  JournalCursorStorage,
  JournalCursorStore,
  JournalStreamScope,
} from './journal-cursor';
export type {
  JournalStreamClient,
  JournalStreamController,
  JournalStreamControllerOptions,
} from './journal-stream-contract';

const protocolVersion = '1.1';
const pageSize = 200;
const maximumBootstrapPages = 100;
const reconnectDelays = [1_000, 2_000, 4_000] as const;
const pollingInterval = 2_000;
const inactivityTimeout = 45_000;
const terminalStatuses = new Set([
  'completed',
  'failed',
  'cancelled',
  'timed_out',
]);

class DefaultJournalStreamController implements JournalStreamController {
  private readonly cursorStore: JournalCursorStore;
  private readonly now: () => number;
  private currentState: JournalState | undefined;
  private subscription: JournalEventSubscription | undefined;
  private reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  private pollingTimer: ReturnType<typeof setTimeout> | undefined;
  private inactivityTimer: ReturnType<typeof setTimeout> | undefined;
  private cursor: JournalCursor | undefined;
  private reconnectAttempt = 0;
  private streamGeneration = 0;
  private stopped = false;
  private terminal = false;
  private polling = false;
  private suspended = false;
  private capabilityConfirmed = false;
  private backfillInFlight = false;
  private endAfterBackfillRequested = false;

  constructor(private readonly options: JournalStreamControllerOptions) {
    this.cursorStore = options.cursorStore ?? createJournalCursorStore();
    this.now = options.now ?? Date.now;
  }

  async start(): Promise<void> {
    this.stopped = false;
    this.terminal = false;
    this.polling = false;
    this.suspended = false;
    this.endAfterBackfillRequested = false;
    this.reconnectAttempt = 0;
    this.streamGeneration += 1;
    this.clearTimers();
    this.closeSubscription();
    await this.loadBootstrap();
  }

  stop(): void {
    this.stopped = true;
    this.streamGeneration += 1;
    this.clearTimers();
    this.closeSubscription();
  }

  resumeStream(): void {
    if (this.stopped || this.terminal || this.suspended || !this.cursor) {
      return;
    }
    this.polling = false;
    this.clearPollingTimer();
    this.reconnectAttempt = 0;
    this.connectStream(false);
  }

  private reduce(action: JournalAction): JournalState {
    this.currentState = this.options.reduce(action);
    return this.currentState;
  }

  private async loadBootstrap(): Promise<void> {
    this.reduce({ type: 'bootstrap_started' });
    try {
      const bootstrap = await this.options.client.getRunJournal({
        ...this.options.scope,
        ...(this.options.attemptId
          ? { attempt_id: this.options.attemptId }
          : {}),
        journal_protocol_version: protocolVersion,
        limit: pageSize,
      });
      if (this.stopped) {
        return;
      }
      const selected = this.options.attemptId
        ? bootstrap.attempts.find(
            candidate => candidate.attempt_id === this.options.attemptId,
          )
        : bootstrap.default_attempt;
      if (this.options.attemptId && !selected) {
        this.reduce({
          type: 'transport_changed',
          status: 'error',
          error_code: 'RESOURCE_NOT_FOUND',
        });
        return;
      }
      const scopedBootstrap = selected
        ? {
            ...bootstrap,
            default_attempt_id: selected.attempt_id,
            default_attempt: selected,
          }
        : bootstrap;
      this.reduce({ type: 'bootstrap_succeeded', bootstrap: scopedBootstrap });
      if (this.options.attemptId) {
        this.reduce({ type: 'view_mode_changed', mode: 'historical' });
      }
      if (
        !bootstrap.enrollment.journal_enabled ||
        bootstrap.projection_state === 'disabled'
      ) {
        this.suspended = true;
        this.reduce({ type: 'transport_changed', status: 'disabled' });
        return;
      }
      if (bootstrap.enrollment.journal_protocol_version !== protocolVersion) {
        this.markDetailUnavailable(
          'protocol_incompatible',
          'SCHEMA_INCOMPATIBLE',
          bootstrap.server_time,
        );
        return;
      }
      if (!selected) {
        this.reduce({ type: 'transport_changed', status: 'ended' });
        return;
      }
      this.cursor = {
        attempt_id: selected.attempt_id,
        sequence: this.currentState?.execution.consistent_sequence ?? 0,
        ...this.lastConsistentEventID(),
      };
      await this.loadRemainingPages(bootstrap.events);
      this.persistConsistentCursor();
      this.terminal = terminalStatuses.has(selected.status);
      if (this.terminal) {
        this.reduce({ type: 'transport_changed', status: 'ended' });
        return;
      }
      if (bootstrap.projection_state === 'degraded') {
        this.suspended = true;
        this.reduce({ type: 'transport_changed', status: 'degraded' });
        return;
      }
      this.connectStream(false);
    } catch (error) {
      if (!this.stopped) {
        this.reduce({
          type: 'transport_changed',
          status: 'error',
          error_code:
            error instanceof WorkbenchClientError
              ? error.code
              : 'journal_bootstrap_failed',
        });
      }
    }
  }

  private async loadRemainingPages(
    initialPage: WorkbenchJournalEventPage,
  ): Promise<void> {
    let page = initialPage;
    let pages = 0;
    while (
      page.has_more &&
      page.attempt_id &&
      page.next_after_sequence !== undefined &&
      pages < maximumBootstrapPages &&
      !this.stopped
    ) {
      page = await this.options.client.listJournalEvents({
        ...this.options.scope,
        attempt_id: page.attempt_id,
        after_sequence: page.next_after_sequence,
        after_event_id: page.next_after_event_id,
        limit: pageSize,
      });
      this.applyPage(page);
      pages += 1;
    }
  }

  private applyPage(page: WorkbenchJournalEventPage): JournalState {
    const state = this.reduce({
      type: 'events_received',
      events: page.items,
    });
    this.persistConsistentCursor();
    return state;
  }

  private lastConsistentEventID(): { event_id?: string } {
    const state = this.currentState;
    if (!state || state.execution.consistent_sequence <= 0) {
      return {};
    }
    const event = eventAtSequence(
      state.execution.events,
      state.execution.consistent_sequence,
    );
    return event ? { event_id: event.event_id } : {};
  }

  private persistConsistentCursor(): void {
    const state = this.currentState;
    const attemptID = state?.execution.selected_attempt_id;
    if (!state || !attemptID) {
      return;
    }
    this.cursor = {
      attempt_id: attemptID,
      sequence: state.execution.consistent_sequence,
      ...this.lastConsistentEventID(),
    };
    this.cursorStore.write(this.options.scope, this.cursor);
  }

  private connectStream(reconnecting: boolean): void {
    if (this.stopped || this.terminal || this.suspended || !this.cursor) {
      return;
    }
    this.polling = false;
    this.clearPollingTimer();
    this.capabilityConfirmed = false;
    const generation = ++this.streamGeneration;
    const controller = new AbortController();
    this.reduce({
      type: 'transport_changed',
      status: reconnecting ? 'reconnecting' : 'connecting',
      reconnect_attempt: this.reconnectAttempt,
    });
    this.closeSubscription();
    this.subscription = this.options.client.subscribeJournalEvents({
      ...this.options.scope,
      attempt_id: this.cursor.attempt_id,
      after_sequence: this.cursor.sequence,
      after_event_id: this.cursor.event_id,
      journal_protocol_version: protocolVersion,
      signal: controller.signal,
      onMessage: message => {
        if (generation !== this.streamGeneration || this.stopped) {
          return;
        }
        this.handleMessage(message);
      },
      onError: error => {
        if (generation !== this.streamGeneration || this.stopped) {
          return;
        }
        controller.abort();
        this.handleStreamFailure(error);
      },
    });
  }

  private handleMessage(message: WorkbenchJournalStreamMessage): void {
    if (message.kind === 'metadata') {
      this.handleMetadata(message.metadata);
      return;
    }
    if (!this.capabilityConfirmed) {
      this.suspendStream();
      this.markDetailUnavailable('capability_unavailable');
      return;
    }
    if (message.kind === 'event') {
      const state = this.reduce({
        type: 'events_received',
        events: [message.event],
      });
      this.persistConsistentCursor();
      this.markActivity();
      if (state.execution.gap) {
        void this.backfill();
      }
      if (terminalStatuses.has(state.execution.status)) {
        this.finishTerminal();
      }
      return;
    }
    if (message.kind === 'heartbeat') {
      this.reconnectAttempt = 0;
      this.markActivity();
      return;
    }
    if (message.kind === 'control') {
      this.reduce({ type: 'control_received', control: message.control });
      if (message.control.type === 'journal_degraded') {
        this.suspendStream();
      } else {
        this.stop();
      }
      return;
    }
    const state = this.currentState;
    if (
      state?.execution.gap ||
      state?.execution.consistent_sequence !== message.latest_sequence
    ) {
      void this.backfill(true);
      return;
    }
    this.finishTerminal();
  }

  private handleMetadata(metadata: WorkbenchJournalMetadata): void {
    if (
      metadata.journal_enabled !== true ||
      metadata.journal_protocol_version !== protocolVersion ||
      metadata.attempt_id !== this.cursor?.attempt_id
    ) {
      this.suspendStream();
      this.markDetailUnavailable(
        'capability_unavailable',
        undefined,
        metadata.server_time,
      );
      return;
    }
    this.capabilityConfirmed = true;
    this.reduce({
      type: 'transport_changed',
      status: 'connected',
      reconnect_attempt: this.reconnectAttempt,
      activity_at: this.now(),
    });
    this.options.onMetadata?.(metadata, this.now());
    this.markActivity();
  }

  private markDetailUnavailable(
    type: 'capability_unavailable' | 'protocol_incompatible',
    errorCode?: 'SCHEMA_INCOMPATIBLE',
    serverTime = this.now(),
  ): void {
    this.reduce({
      type: 'control_received',
      control: {
        type,
        schema_version: protocolVersion,
        journal_protocol_version: protocolVersion,
        server_time: serverTime,
        ...(errorCode ? { error_code: errorCode } : {}),
      },
    });
  }

  private markActivity(): void {
    this.clearInactivityTimer();
    this.inactivityTimer = setTimeout(() => {
      if (this.stopped || this.terminal) {
        return;
      }
      this.reduce({ type: 'inactivity_timeout', observed_at: this.now() });
      this.handleStreamFailure(
        new WorkbenchClientError({
          message: 'Journal stream inactive',
          code: 'journal_stream_inactive',
          retryable: true,
          outcome: 'failed',
        }),
      );
    }, inactivityTimeout);
  }

  private handleStreamFailure(error: unknown): void {
    this.clearInactivityTimer();
    this.closeSubscription();
    if (this.stopped || this.terminal) {
      return;
    }
    if (this.reconnectAttempt < reconnectDelays.length) {
      const fallback = reconnectDelays[this.reconnectAttempt];
      this.reconnectAttempt += 1;
      const delay = journalRetryDelay(error, fallback);
      this.reduce({
        type: 'transport_changed',
        status: 'reconnecting',
        reconnect_attempt: this.reconnectAttempt,
        error_code:
          error instanceof WorkbenchClientError ? error.code : undefined,
      });
      this.reconnectTimer = setTimeout(() => this.connectStream(true), delay);
      return;
    }
    this.startPolling(error);
  }

  private startPolling(error?: unknown): void {
    if (this.stopped || this.terminal || this.suspended || !this.cursor) {
      return;
    }
    this.streamGeneration += 1;
    this.clearInactivityTimer();
    this.closeSubscription();
    this.polling = true;
    this.reduce({
      type: 'transport_changed',
      status: 'polling',
      reconnect_attempt: this.reconnectAttempt,
      error_code:
        error instanceof WorkbenchClientError ? error.code : undefined,
    });
    void this.pollOnce();
  }

  private async pollOnce(): Promise<void> {
    if (this.stopped || this.terminal || !this.polling || !this.cursor) {
      return;
    }
    let nextDelay = pollingInterval;
    try {
      const page = await this.options.client.listJournalEvents({
        ...this.options.scope,
        attempt_id: this.cursor.attempt_id,
        after_sequence: this.cursor.sequence,
        after_event_id: this.cursor.event_id,
        limit: pageSize,
      });
      const state = this.applyPage(page);
      if (page.has_more) {
        void this.pollOnce();
        return;
      }
      if (
        this.endAfterBackfillRequested ||
        terminalStatuses.has(state.execution.status)
      ) {
        this.finishTerminal();
        return;
      }
    } catch (error) {
      if (
        error instanceof WorkbenchClientError &&
        error.code === 'JOURNAL_CURSOR_EXPIRED'
      ) {
        await this.loadBootstrap();
        return;
      }
      nextDelay = journalRetryDelay(error, pollingInterval);
    }
    if (!this.stopped && !this.terminal && this.polling) {
      this.pollingTimer = setTimeout(() => void this.pollOnce(), nextDelay);
    }
  }

  private async backfill(endAfterBackfill = false): Promise<void> {
    if (endAfterBackfill) {
      this.endAfterBackfillRequested = true;
    }
    if (this.backfillInFlight || this.stopped || !this.cursor) {
      return;
    }
    this.backfillInFlight = true;
    try {
      const page = await this.options.client.listJournalEvents({
        ...this.options.scope,
        attempt_id: this.cursor.attempt_id,
        after_sequence: this.cursor.sequence,
        after_event_id: this.cursor.event_id,
        limit: pageSize,
      });
      const state = this.applyPage(page);
      if (page.has_more || state.execution.gap) {
        this.backfillInFlight = false;
        void this.backfill(endAfterBackfill);
        return;
      }
      if (
        this.endAfterBackfillRequested ||
        terminalStatuses.has(state.execution.status)
      ) {
        this.finishTerminal();
      }
    } catch (error) {
      if (
        error instanceof WorkbenchClientError &&
        error.code === 'JOURNAL_CURSOR_EXPIRED'
      ) {
        await this.loadBootstrap();
      } else {
        this.startPolling(error);
      }
    } finally {
      this.backfillInFlight = false;
    }
  }

  private finishTerminal(): void {
    this.terminal = true;
    this.endAfterBackfillRequested = false;
    this.polling = false;
    this.clearTimers();
    this.closeSubscription();
    this.reduce({ type: 'transport_changed', status: 'ended' });
  }

  private suspendStream(): void {
    this.suspended = true;
    this.polling = false;
    this.streamGeneration += 1;
    this.clearTimers();
    this.closeSubscription();
  }

  private closeSubscription(): void {
    const { subscription } = this;
    this.subscription = undefined;
    subscription?.close();
  }

  private clearTimers(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = undefined;
    }
    this.clearPollingTimer();
    this.clearInactivityTimer();
  }

  private clearPollingTimer(): void {
    if (this.pollingTimer) {
      clearTimeout(this.pollingTimer);
      this.pollingTimer = undefined;
    }
  }

  private clearInactivityTimer(): void {
    if (this.inactivityTimer) {
      clearTimeout(this.inactivityTimer);
      this.inactivityTimer = undefined;
    }
  }
}

export const createJournalStreamController = (
  options: JournalStreamControllerOptions,
): JournalStreamController => new DefaultJournalStreamController(options);
