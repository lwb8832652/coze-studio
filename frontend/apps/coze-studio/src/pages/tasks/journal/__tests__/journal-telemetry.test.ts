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

import {
  createJournalClock,
  createJournalTelemetry,
} from '../journal-telemetry';

describe('Journal telemetry', () => {
  it('uses the metadata clock offset for end-to-end visibility latency', () => {
    const send = vi.fn();
    const telemetry = createJournalTelemetry({ send });
    const clock = createJournalClock({
      client_received_at: 2_000,
      server_time: 1_900,
    });

    telemetry.eventVisible({
      event_id: '101',
      attempt_id: 'att-1',
      run_id: '3001',
      occurred_at: 1_500,
      client_visible_at: 2_100,
      clock,
      protocol_version: '1.1',
      transport: 'sse',
    });

    expect(send).toHaveBeenCalledWith({
      name: 'journal_event_visible',
      metrics: { latency_ms: 500 },
      categories: {
        client_version: 'canonical_v1',
        protocol_version: '1.1',
        result: 'success',
        transport: 'sse',
      },
    });
  });

  it('reports one visible event only once across rerenders', () => {
    const send = vi.fn();
    const telemetry = createJournalTelemetry({ send });
    const input = {
      event_id: '101',
      attempt_id: 'att-1',
      run_id: '3001',
      occurred_at: 1_000,
      client_visible_at: 1_100,
      clock: createJournalClock({
        client_received_at: 1_050,
        server_time: 1_050,
      }),
      protocol_version: '1.1',
      transport: 'sse' as const,
    };

    telemetry.eventVisible(input);
    telemetry.eventVisible(input);

    expect(send).toHaveBeenCalledTimes(1);
  });

  it('reports the first visible step once per attempt with submit-time latency', () => {
    const send = vi.fn();
    const telemetry = createJournalTelemetry({ send });
    const input = {
      event_id: '101',
      attempt_id: 'att-1',
      run_id: '3001',
      occurred_at: 1_100,
      submit_at: 1_000,
      client_visible_at: 1_300,
      clock: createJournalClock({
        client_received_at: 1_250,
        server_time: 1_200,
      }),
      protocol_version: '1.1',
      transport: 'sse' as const,
    };

    telemetry.firstVisible(input);
    telemetry.firstVisible({ ...input, event_id: '102' });

    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith({
      name: 'journal_first_visible',
      metrics: { latency_ms: 250 },
      categories: {
        client_version: 'canonical_v1',
        protocol_version: '1.1',
        result: 'success',
        transport: 'sse',
      },
    });
  });

  it('rejects sensitive or free-form telemetry fields', () => {
    const send = vi.fn();
    const telemetry = createJournalTelemetry({ send });

    expect(() =>
      telemetry.report('journal_snapshot_load', {
        metrics: { latency_ms: 12 },
        categories: {
          result: 'success',
          url: 'https://example.test/private',
        },
      }),
    ).toThrow(/unsafe Journal telemetry category/i);
    expect(() =>
      telemetry.report('journal_stream_gap', {
        metrics: { latency_ms: 12 },
        categories: {
          result: 'token_sk-secret',
        },
      }),
    ).toThrow(/unsafe Journal telemetry category/i);
    expect(send).not.toHaveBeenCalled();
  });

  it('never lets an observability failure alter product behavior', () => {
    const telemetry = createJournalTelemetry({
      send: () => {
        throw new Error('collector unavailable');
      },
    });

    expect(() =>
      telemetry.report('journal_follow_change', {
        metrics: {},
        categories: { result: 'live_follow' },
      }),
    ).not.toThrow();
  });
});
