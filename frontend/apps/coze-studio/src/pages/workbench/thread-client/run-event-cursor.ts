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

export interface RunEventCursorScope {
  contract: 'canonical_v1';
  spaceId: string;
  threadId: string;
  runId: string;
}

export interface RunEventCursorStorage {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
}

export interface RunEventCursorStore {
  read: (scope: RunEventCursorScope) => string | undefined;
  write: (scope: RunEventCursorScope, cursor: string) => void;
  clear: (scope: RunEventCursorScope) => void;
}

interface RunEventCursorStoreOptions {
  storage?: RunEventCursorStorage | null;
  memory?: Map<string, string>;
}

const positiveDecimal = /^[1-9]\d*$/;
const nonNegativeDecimal = /^(0|[1-9]\d*)$/;
const defaultMemory = new Map<string, string>();
const keyPrefix = 'coze:workbench:run-event-cursor';

const sessionCursorStorage = (): RunEventCursorStorage | undefined => {
  try {
    return globalThis.sessionStorage;
  } catch (error) {
    void error;
    return undefined;
  }
};

const cursorKey = (scope: RunEventCursorScope): string =>
  [keyPrefix, scope.contract, scope.spaceId, scope.threadId, scope.runId].join(
    ':',
  );

const normalizePositiveCursor = (value: unknown): string | undefined =>
  typeof value === 'string' && positiveDecimal.test(value) ? value : undefined;

export const greatestRunEventCursor = (
  left: string | undefined,
  right: string | undefined,
): string | undefined => {
  const validLeft =
    left !== undefined && nonNegativeDecimal.test(left) ? left : undefined;
  const validRight =
    right !== undefined && nonNegativeDecimal.test(right) ? right : undefined;
  if (validLeft === undefined) {
    return validRight;
  }
  if (validRight === undefined) {
    return validLeft;
  }
  if (validLeft.length !== validRight.length) {
    return validLeft.length > validRight.length ? validLeft : validRight;
  }
  return validLeft >= validRight ? validLeft : validRight;
};

export const createRunEventCursorStore = (
  options: RunEventCursorStoreOptions = {},
): RunEventCursorStore => {
  const memory = options.memory ?? defaultMemory;
  const clearedKeys = new Set<string>();
  const storage =
    options.storage === undefined ? sessionCursorStorage() : options.storage;

  const readCursor = (key: string): string | undefined => {
    if (clearedKeys.has(key)) {
      return undefined;
    }
    let persisted: string | undefined;
    if (storage) {
      try {
        persisted = normalizePositiveCursor(storage.getItem(key));
      } catch (error) {
        void error;
        // In-memory state remains available when browser storage is blocked.
      }
    }
    const latest = greatestRunEventCursor(
      normalizePositiveCursor(memory.get(key)),
      persisted,
    );
    if (latest !== undefined) {
      memory.set(key, latest);
    }
    return normalizePositiveCursor(latest);
  };

  return {
    read(scope) {
      return readCursor(cursorKey(scope));
    },
    write(scope, cursor) {
      const normalized = normalizePositiveCursor(cursor);
      if (normalized === undefined) {
        return;
      }
      const key = cursorKey(scope);
      clearedKeys.delete(key);
      const latest = greatestRunEventCursor(readCursor(key), normalized);
      if (latest === undefined) {
        return;
      }
      memory.set(key, latest);
      if (storage) {
        try {
          storage.setItem(key, latest);
        } catch (error) {
          void error;
          // The confirmed cursor is still retained in memory.
        }
      }
    },
    clear(scope) {
      const key = cursorKey(scope);
      clearedKeys.add(key);
      memory.delete(key);
      if (storage) {
        try {
          storage.removeItem(key);
        } catch (error) {
          void error;
          // Clearing the in-memory fallback is sufficient for this session.
        }
      }
    },
  };
};
