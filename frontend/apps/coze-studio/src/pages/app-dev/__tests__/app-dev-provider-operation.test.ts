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

import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  clearAppDevPendingOperation,
  clearAppDevTerminalBuild,
  createAppDevOperationID,
  APP_DEV_OPERATION_SCHEMA_VERSION,
  APP_DEV_OPERATION_INVALID_WARNING,
  APP_DEV_OPERATION_STORAGE_WARNING,
  APP_DEV_OPERATION_TTLS_MS,
  readAppDevTerminalBuild,
  readAppDevPendingOperation,
  writeAppDevTerminalBuild,
  writeAppDevPendingOperation,
} from '../utils/provider-operation';

describe('AppDev provider operation persistence', () => {
  const touchedScopes = new Map<
    string,
    { spaceId: string; projectId: string; principalId?: string }
  >();
  afterEach(() => {
    vi.restoreAllMocks();
    for (const identity of touchedScopes.values()) {
      for (const intent of [
        'runtime-auto-start',
        'runtime-manual-start',
        'runtime-stop',
        'runtime-restart',
        'snapshot-restore',
        'build',
      ] as const) {
        clearAppDevPendingOperation(identity, intent);
      }
      clearAppDevTerminalBuild(identity);
    }
    touchedScopes.clear();
    sessionStorage.clear();
  });

  const scope = (
    projectId = 'project-1',
    principalId: string | undefined = 'user-1',
  ) => {
    const identity = { spaceId: 'space-1', projectId, principalId };
    touchedScopes.set(`${principalId ?? 'anonymous'}:${projectId}`, identity);
    return identity;
  };

  it('isolates pending operations by space and project', () => {
    const first = scope('project-1');
    const second = scope('project-2');
    writeAppDevPendingOperation(first, 'build', {
      operationId: 'build-operation-one',
      generation: 1,
      phase: 'observed',
    });
    writeAppDevPendingOperation(second, 'build', {
      operationId: 'build-operation-two',
      generation: 2,
      phase: 'observed',
    });

    expect(readAppDevPendingOperation(first, 'build')).toMatchObject({
      intent: 'build',
      operationId: 'build-operation-one',
      generation: 1,
      phase: 'observed',
    });
    expect(readAppDevPendingOperation(second, 'build')).toMatchObject({
      intent: 'build',
      operationId: 'build-operation-two',
      generation: 2,
      phase: 'observed',
    });
    clearAppDevPendingOperation(first, 'build', 'build-operation-one');
    expect(readAppDevPendingOperation(first, 'build')).toBeUndefined();
    expect(readAppDevPendingOperation(second, 'build')).toBeDefined();
  });

  it('generates backend-safe unpredictable operation identifiers', () => {
    const first = createAppDevOperationID();
    const second = createAppDevOperationID();
    expect(first).not.toBe(second);
    expect(first).toMatch(/^[A-Za-z0-9._:-]{8,128}$/u);
  });

  it('isolates every runtime intent and binds snapshot parameters', () => {
    const identity = scope();
    const intents = [
      'runtime-auto-start',
      'runtime-manual-start',
      'runtime-stop',
      'runtime-restart',
    ] as const;
    intents.forEach((intent, index) =>
      writeAppDevPendingOperation(identity, intent, {
        operationId: `${intent}-operation-${index}`,
        phase: 'requesting',
      }),
    );
    writeAppDevPendingOperation(identity, 'snapshot-restore', {
      operationId: 'snapshot-restore-operation',
      snapshotId: 'snapshot-1',
      phase: 'requesting',
    });

    expect(
      intents.map(
        intent => readAppDevPendingOperation(identity, intent)?.intent,
      ),
    ).toEqual(intents);
    expect(
      readAppDevPendingOperation(identity, 'snapshot-restore'),
    ).toMatchObject({
      intent: 'snapshot-restore',
      snapshotId: 'snapshot-1',
      phase: 'requesting',
    });
  });

  it('persists only a safe terminal build projection for reload reconciliation', () => {
    const identity = scope();
    writeAppDevTerminalBuild(identity, {
      operationId: 'build-terminal-operation',
      generation: 7,
      projection: {
        generation: 7,
        state: 'ready',
        releaseAvailable: true,
        size: 42,
        stale: false,
      },
    });
    expect(readAppDevTerminalBuild(identity)).toMatchObject({
      operationId: 'build-terminal-operation',
      generation: 7,
      projection: { state: 'ready', releaseAvailable: true },
    });
    expect(JSON.stringify(readAppDevTerminalBuild(identity))).not.toMatch(
      /token|object.?key|digest|provider/iu,
    );
  });

  it('binds journal keys and records to the authenticated principal', () => {
    const first = scope('project-1', 'user-1');
    const second = scope('project-1', 'user-2');
    writeAppDevPendingOperation(first, 'runtime-stop', {
      operationId: 'stop-operation-user-one',
    });
    writeAppDevPendingOperation(second, 'runtime-stop', {
      operationId: 'stop-operation-user-two',
    });

    expect(readAppDevPendingOperation(first, 'runtime-stop')?.operationId).toBe(
      'stop-operation-user-one',
    );
    expect(
      readAppDevPendingOperation(second, 'runtime-stop')?.operationId,
    ).toBe('stop-operation-user-two');
    const records = Array.from({ length: sessionStorage.length }, (_, index) =>
      JSON.parse(sessionStorage.getItem(sessionStorage.key(index)!) || '{}'),
    );
    expect(records).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          schemaVersion: APP_DEV_OPERATION_SCHEMA_VERSION,
          principalId: 'user-1',
          spaceId: 'space-1',
          projectId: 'project-1',
          intent: 'runtime-stop',
        }),
        expect.objectContaining({ principalId: 'user-2' }),
      ]),
    );
    expect(
      Array.from({ length: sessionStorage.length }, (_, index) =>
        sessionStorage.key(index),
      ).join('\n'),
    ).not.toMatch(/email|token/iu);
  });

  it.each([
    ['expired', APP_DEV_OPERATION_TTLS_MS['runtime-stop'] + 1],
    ['future', -(5 * 60 * 1000 + 1)],
  ])('deletes and ignores a %s operation record', (_, age) => {
    const now = 2_000_000_000_000;
    vi.spyOn(Date, 'now').mockReturnValue(now);
    writeAppDevPendingOperation(scope(), 'runtime-stop', {
      operationId: 'bounded-stop-operation',
      createdAt: now,
    });
    const key = sessionStorage.key(0)!;
    const record = JSON.parse(sessionStorage.getItem(key)!) as Record<
      string,
      unknown
    >;
    record.createdAt = now - age;
    sessionStorage.setItem(key, JSON.stringify(record));

    expect(readAppDevPendingOperation(scope(), 'runtime-stop')).toBeUndefined();
    expect(sessionStorage.length).toBe(0);
  });

  it.each([
    [
      'missing timestamp',
      (record: Record<string, unknown>) => delete record.createdAt,
    ],
    [
      'wrong schema',
      (record: Record<string, unknown>) => {
        record.schemaVersion = 99;
      },
    ],
    [
      'wrong principal',
      (record: Record<string, unknown>) => {
        record.principalId = 'user-2';
      },
    ],
    [
      'wrong intent',
      (record: Record<string, unknown>) => {
        record.intent = 'runtime-restart';
      },
    ],
  ])('deletes and ignores %s', (_, mutate) => {
    writeAppDevPendingOperation(scope(), 'runtime-stop', {
      operationId: 'validated-stop-operation',
    });
    const key = sessionStorage.key(0)!;
    const record = JSON.parse(sessionStorage.getItem(key)!);
    mutate(record);
    sessionStorage.setItem(key, JSON.stringify(record));

    expect(readAppDevPendingOperation(scope(), 'runtime-stop')).toBeUndefined();
    expect(sessionStorage.getItem(key)).toBeNull();
  });

  it('uses only bounded memory without a principal and reports a safe warning', () => {
    const anonymous = {
      spaceId: 'space-1',
      projectId: 'anonymous-project',
    };
    const result = writeAppDevPendingOperation(anonymous, 'runtime-stop', {
      operationId: 'anonymous-stop-operation',
    });

    expect(result.warning).toBe(APP_DEV_OPERATION_STORAGE_WARNING);
    expect(sessionStorage.length).toBe(0);
    expect(readAppDevPendingOperation(anonymous, 'runtime-stop')).toMatchObject(
      {
        operationId: 'anonymous-stop-operation',
      },
    );
    clearAppDevPendingOperation(anonymous, 'runtime-stop');
  });

  it('falls back to memory when browser storage throws', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('raw quota path', 'QuotaExceededError');
    });
    const result = writeAppDevPendingOperation(
      scope('quota-project'),
      'build',
      {
        operationId: 'quota-build-operation',
      },
    );

    expect(result.warning).toBe(APP_DEV_OPERATION_STORAGE_WARNING);
    expect(
      readAppDevPendingOperation(scope('quota-project'), 'build'),
    ).toMatchObject({ operationId: 'quota-build-operation' });
    expect(JSON.stringify(result)).not.toContain('raw quota path');
  });

  it('falls back safely when browser storage read and removal throw', () => {
    const identity = scope('storage-error-project');
    writeAppDevPendingOperation(identity, 'runtime-stop', {
      operationId: 'storage-error-stop-operation',
    });
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('blocked read', 'SecurityError');
    });
    expect(readAppDevPendingOperation(identity, 'runtime-stop')).toMatchObject({
      operationId: 'storage-error-stop-operation',
    });
    vi.restoreAllMocks();
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new DOMException('blocked remove', 'SecurityError');
    });
    const result = clearAppDevPendingOperation(
      identity,
      'runtime-stop',
      'storage-error-stop-operation',
    );
    expect(result.warning).toBe(APP_DEV_OPERATION_STORAGE_WARNING);
    expect(
      readAppDevPendingOperation(identity, 'runtime-stop'),
    ).toBeUndefined();
  });

  it.each([
    undefined,
    42,
    '',
    ' snapshot ',
    'snapshot\u0000id',
    'x'.repeat(257),
    'snapshot/1',
    'snapshot\\1',
    'snapshot?1',
  ])(
    'rejects invalid snapshot identity %j without writing storage',
    snapshotId => {
      const result = writeAppDevPendingOperation(
        scope('invalid-snapshot-project'),
        'snapshot-restore',
        {
          operationId: 'invalid-snapshot-operation',
          snapshotId: snapshotId as string,
        },
      );
      expect(result.value).toBeUndefined();
      expect(result.warning).toBe(APP_DEV_OPERATION_INVALID_WARNING);
      expect(sessionStorage.length).toBe(0);
      expect(
        readAppDevPendingOperation(
          scope('invalid-snapshot-project'),
          'snapshot-restore',
        ),
      ).toBeUndefined();
    },
  );

  it.each(['corrupt', 'future'])(
    'clears a %s terminal build journal',
    corruption => {
      const identity = scope(`terminal-${corruption}`);
      writeAppDevTerminalBuild(identity, {
        operationId: `terminal-${corruption}-operation`,
        generation: 8,
        projection: {
          generation: 8,
          state: 'ready',
          releaseAvailable: true,
          size: 12,
          stale: false,
        },
      });
      const key = sessionStorage.key(0)!;
      if (corruption === 'corrupt') {
        sessionStorage.setItem(key, '{');
      } else {
        const record = JSON.parse(sessionStorage.getItem(key)!);
        record.recordedAt =
          Date.now() + APP_DEV_OPERATION_TTLS_MS['build-terminal'] + 1;
        sessionStorage.setItem(key, JSON.stringify(record));
      }
      expect(readAppDevTerminalBuild(identity)).toBeUndefined();
      expect(sessionStorage.getItem(key)).toBeNull();
    },
  );
});
