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

import slardar from '@coze-studio/default-slardar';

export type JournalTelemetryName =
  | 'journal_event_visible'
  | 'journal_first_visible'
  | 'journal_stream_gap'
  | 'journal_snapshot_load'
  | 'journal_follow_change'
  | 'journal_artifact_action';

export interface JournalClock {
  offset_ms: number;
}

export interface JournalTelemetryPayload {
  metrics: Record<string, number>;
  categories: Record<string, string>;
}

export interface JournalTelemetryEvent extends JournalTelemetryPayload {
  name: JournalTelemetryName;
}

export type JournalTelemetrySend = (event: JournalTelemetryEvent) => void;

interface JournalVisibleInput {
  event_id: string;
  attempt_id: string;
  run_id: string;
  occurred_at: number;
  client_visible_at: number;
  clock: JournalClock;
  protocol_version: string;
  transport: 'sse' | 'polling';
}

interface JournalFirstVisibleInput extends JournalVisibleInput {
  submit_at: number;
}

interface JournalTelemetryOptions {
  send?: JournalTelemetrySend;
}

const safeCategoryKeys = new Set([
  'client_version',
  'protocol_version',
  'rollout_group',
  'task_type',
  'transport',
  'result',
  'content_type',
  'view_mode',
  'action',
]);
const safeMetricKeys = new Set([
  'latency_ms',
  'gap_size',
  'reconnect_attempt',
  'fragment_count',
  'size_bytes',
]);
const stableCategoryValue = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$/;
const sensitiveValue =
  /authorization|bearer|cookie|password|secret|token|api[_-]?key|https?:\/\//i;

const finiteTimestamp = (value: number, label: string): number => {
  if (!Number.isFinite(value) || value < 0) {
    throw new TypeError(`${label} must be a non-negative finite timestamp`);
  }
  return value;
};

export const createJournalClock = (input: {
  client_received_at: number;
  server_time: number;
}): JournalClock => ({
  offset_ms:
    finiteTimestamp(input.server_time, 'server_time') -
    finiteTimestamp(input.client_received_at, 'client_received_at'),
});

const visibleLatency = ({
  clientVisibleAt,
  clock,
  serverOccurredAt,
}: {
  clientVisibleAt: number;
  clock: JournalClock;
  serverOccurredAt: number;
}): number => {
  const correctedVisibleAt =
    finiteTimestamp(clientVisibleAt, 'client_visible_at') + clock.offset_ms;
  const latency = Math.round(
    correctedVisibleAt - finiteTimestamp(serverOccurredAt, 'occurred_at'),
  );
  return Number.isSafeInteger(latency) && latency > 0 ? latency : 0;
};

const assertSafePayload = (payload: JournalTelemetryPayload): void => {
  Object.entries(payload.metrics).forEach(([key, value]) => {
    if (!safeMetricKeys.has(key) || !Number.isFinite(value) || value < 0) {
      throw new TypeError(`Unsafe Journal telemetry metric: ${key}`);
    }
  });
  Object.entries(payload.categories).forEach(([key, value]) => {
    if (
      !safeCategoryKeys.has(key) ||
      !stableCategoryValue.test(value) ||
      sensitiveValue.test(value)
    ) {
      throw new TypeError(`Unsafe Journal telemetry category: ${key}`);
    }
  });
};

const defaultSend: JournalTelemetrySend = event => {
  slardar('sendEvent', {
    name: event.name,
    metrics: event.metrics,
    categories: event.categories,
  });
};

export interface JournalTelemetry {
  report: (
    name: JournalTelemetryName,
    payload: JournalTelemetryPayload,
  ) => void;
  eventVisible: (input: JournalVisibleInput) => void;
  firstVisible: (input: JournalFirstVisibleInput) => void;
}

export const createJournalTelemetry = ({
  send = defaultSend,
}: JournalTelemetryOptions = {}): JournalTelemetry => {
  const visibleEventKeys = new Set<string>();
  const firstVisibleRunKeys = new Set<string>();
  const report = (
    name: JournalTelemetryName,
    payload: JournalTelemetryPayload,
  ): void => {
    assertSafePayload(payload);
    try {
      send({ name, ...payload });
    } catch (error) {
      void error;
      // Product behavior cannot depend on observability availability.
    }
  };
  const visibilityCategories = (
    input: JournalVisibleInput,
  ): Record<string, string> => ({
    client_version: 'canonical_v1',
    protocol_version: input.protocol_version,
    result: 'success',
    transport: input.transport,
  });

  return {
    report,
    eventVisible: input => {
      const key = `${input.run_id}:${input.attempt_id}:${input.event_id}`;
      if (visibleEventKeys.has(key)) {
        return;
      }
      visibleEventKeys.add(key);
      report('journal_event_visible', {
        metrics: {
          latency_ms: visibleLatency({
            clientVisibleAt: input.client_visible_at,
            clock: input.clock,
            serverOccurredAt: input.occurred_at,
          }),
        },
        categories: visibilityCategories(input),
      });
    },
    firstVisible: input => {
      const key = `${input.run_id}:${input.attempt_id}`;
      if (firstVisibleRunKeys.has(key)) {
        return;
      }
      firstVisibleRunKeys.add(key);
      report('journal_first_visible', {
        metrics: {
          latency_ms: visibleLatency({
            clientVisibleAt: input.client_visible_at,
            clock: input.clock,
            serverOccurredAt: input.submit_at,
          }),
        },
        categories: visibilityCategories(input),
      });
    },
  };
};
