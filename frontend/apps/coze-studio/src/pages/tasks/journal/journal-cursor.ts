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

export interface JournalStreamScope {
  space_id: string;
  thread_id: string;
  run_id: string;
}

export interface JournalCursor {
  attempt_id: string;
  sequence: number;
  event_id?: string;
}

export interface JournalCursorStorage {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
}

export interface JournalCursorStore {
  read: (scope: JournalStreamScope) => JournalCursor | undefined;
  write: (scope: JournalStreamScope, cursor: JournalCursor) => void;
  clear: (scope: JournalStreamScope) => void;
}

interface JournalCursorStoreOptions {
  storage?: JournalCursorStorage | null;
  memory?: Map<string, string>;
}

const positiveDecimal = /^[1-9]\d*$/;
const keyPrefix = 'coze:workbench:journal-cursor:canonical_v1';
const defaultMemory = new Map<string, string>();

const sessionCursorStorage = (): JournalCursorStorage | undefined => {
  try {
    return globalThis.sessionStorage;
  } catch (error) {
    void error;
    return undefined;
  }
};

const cursorKey = (scope: JournalStreamScope): string =>
  [keyPrefix, scope.space_id, scope.thread_id, scope.run_id].join(':');

const parseCursor = (
  value: string | null | undefined,
): JournalCursor | undefined => {
  if (!value) {
    return undefined;
  }
  try {
    const parsed = JSON.parse(value) as unknown;
    if (
      typeof parsed !== 'object' ||
      parsed === null ||
      Array.isArray(parsed)
    ) {
      return undefined;
    }
    const cursor = parsed as Record<string, unknown>;
    if (
      typeof cursor.attempt_id !== 'string' ||
      cursor.attempt_id.trim() === '' ||
      !Number.isSafeInteger(cursor.sequence) ||
      (cursor.sequence as number) < 0 ||
      (cursor.event_id !== undefined &&
        (typeof cursor.event_id !== 'string' ||
          !positiveDecimal.test(cursor.event_id)))
    ) {
      return undefined;
    }
    return {
      attempt_id: cursor.attempt_id,
      sequence: cursor.sequence as number,
      ...(typeof cursor.event_id === 'string'
        ? { event_id: cursor.event_id }
        : {}),
    };
  } catch (error) {
    void error;
    return undefined;
  }
};

const serializeCursor = (cursor: JournalCursor): string | undefined => {
  if (
    !cursor.attempt_id.trim() ||
    !Number.isSafeInteger(cursor.sequence) ||
    cursor.sequence < 0 ||
    (cursor.event_id !== undefined && !positiveDecimal.test(cursor.event_id))
  ) {
    return undefined;
  }
  return JSON.stringify(cursor);
};

const newerCursor = (
  current: JournalCursor | undefined,
  incoming: JournalCursor,
): JournalCursor => {
  if (
    current === undefined ||
    current.attempt_id !== incoming.attempt_id ||
    incoming.sequence >= current.sequence
  ) {
    return incoming;
  }
  return current;
};

export const createJournalCursorStore = (
  options: JournalCursorStoreOptions = {},
): JournalCursorStore => {
  const memory = options.memory ?? defaultMemory;
  const storage =
    options.storage === undefined ? sessionCursorStorage() : options.storage;
  const read = (scope: JournalStreamScope): JournalCursor | undefined => {
    const key = cursorKey(scope);
    const inMemory = parseCursor(memory.get(key));
    let persisted: JournalCursor | undefined;
    if (storage) {
      try {
        persisted = parseCursor(storage.getItem(key));
      } catch (error) {
        void error;
      }
    }
    const selected =
      persisted &&
      (!inMemory ||
        persisted.attempt_id !== inMemory.attempt_id ||
        persisted.sequence >= inMemory.sequence)
        ? persisted
        : inMemory;
    const encoded = selected ? serializeCursor(selected) : undefined;
    if (encoded) {
      memory.set(key, encoded);
    }
    return selected;
  };
  return {
    read,
    write(scope, cursor) {
      const encoded = serializeCursor(cursor);
      if (!encoded) {
        return;
      }
      const key = cursorKey(scope);
      const selected = newerCursor(read(scope), cursor);
      const selectedEncoded = serializeCursor(selected);
      if (!selectedEncoded) {
        return;
      }
      memory.set(key, selectedEncoded);
      if (storage) {
        try {
          storage.setItem(key, selectedEncoded);
        } catch (error) {
          void error;
        }
      }
    },
    clear(scope) {
      const key = cursorKey(scope);
      memory.delete(key);
      if (storage) {
        try {
          storage.removeItem(key);
        } catch (error) {
          void error;
        }
      }
    },
  };
};
