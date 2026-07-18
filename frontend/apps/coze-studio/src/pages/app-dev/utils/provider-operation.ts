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

/* eslint-disable no-control-regex -- Persisted operation identifiers reject control characters. */

import type { AppDevBuildInfo, AppDevProjectIdentity } from '../types';
import { normalizeAppDevSnapshotID } from './snapshot-id';

export type AppDevPendingOperationKind =
  | 'runtime-auto-start'
  | 'runtime-manual-start'
  | 'runtime-stop'
  | 'runtime-restart'
  | 'snapshot-restore'
  | 'build';

export type AppDevOperationPhase = 'pending' | 'requesting' | 'observed';

export interface AppDevOperationScope extends AppDevProjectIdentity {
  principalId?: string;
}

export interface AppDevPendingOperation {
  intent: AppDevPendingOperationKind;
  operationId: string;
  phase: AppDevOperationPhase;
  createdAt: number;
  generation?: number;
  snapshotId?: string;
}

type PendingOperationInput = Omit<
  AppDevPendingOperation,
  'intent' | 'createdAt' | 'phase'
> & {
  phase?: AppDevOperationPhase;
  createdAt?: number;
};

export interface AppDevTerminalBuild {
  operationId: string;
  generation: number;
  recordedAt: number;
  projection: AppDevBuildInfo;
}

export interface AppDevOperationStorageResult<T> {
  value: T | undefined;
  persistent: boolean;
  warning?: string;
}

interface StoredOperationIdentity {
  schemaVersion: number;
  principalId: string;
  spaceId: string;
  projectId: string;
}

type StoredPendingOperation = AppDevPendingOperation & StoredOperationIdentity;
type StoredTerminalBuild = AppDevTerminalBuild &
  StoredOperationIdentity & { intent: 'build-terminal' };

export const APP_DEV_OPERATION_SCHEMA_VERSION = 2;
export const APP_DEV_OPERATION_STORAGE_WARNING =
  '浏览器无法安全保存操作恢复状态，本次操作仅在当前页面有效';
export const APP_DEV_OPERATION_INVALID_WARNING =
  '已忽略无效或过期的操作恢复状态';
export const APP_DEV_OPERATION_TTLS_MS = {
  'runtime-auto-start': 15 * 60 * 1000,
  'runtime-manual-start': 15 * 60 * 1000,
  'runtime-stop': 15 * 60 * 1000,
  'runtime-restart': 15 * 60 * 1000,
  'snapshot-restore': 30 * 60 * 1000,
  build: 2 * 60 * 60 * 1000,
  'build-terminal': 24 * 60 * 60 * 1000,
} as const;

const FUTURE_CLOCK_TOLERANCE_MS = 5 * 60 * 1000;
const MAX_STORED_RECORD_BYTES = 16 * 1024;
const MAX_MEMORY_RECORDS = 64;
const memoryStorage = new Map<string, string>();
const memoryOnlyKeys = new Set<string>();
const suppressedStorageKeys = new Set<string>();

const validOperationID = (value: unknown): value is string =>
  typeof value === 'string' && /^[A-Za-z0-9._:-]{8,128}$/u.test(value);

const validPrincipalID = (value: unknown): value is string =>
  typeof value === 'string' && /^[A-Za-z0-9._:-]{1,128}$/u.test(value);

const validScopeValue = (value: unknown): value is string =>
  typeof value === 'string' &&
  value.length > 0 &&
  value.length <= 256 &&
  !/[\u0000-\u001f\u007f]/u.test(value);

const validGeneration = (value: unknown) =>
  value === undefined || (Number.isSafeInteger(value) && Number(value) >= 0);

const validTimestamp = (
  value: unknown,
  ttl: number,
  now = Date.now(),
): value is number =>
  typeof value === 'number' &&
  Number.isFinite(value) &&
  value >= now - ttl &&
  value <= now + FUTURE_CLOCK_TOLERANCE_MS;

const operationKey = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind | 'build-terminal',
) =>
  `app-dev:provider-operation:v${APP_DEV_OPERATION_SCHEMA_VERSION}:${encodeURIComponent(
    validPrincipalID(scope.principalId) ? scope.principalId : 'anonymous',
  )}:${kind}:${encodeURIComponent(scope.spaceId)}:${encodeURIComponent(
    scope.projectId,
  )}`;

const persistentScope = (scope: AppDevOperationScope) =>
  validPrincipalID(scope.principalId);

const setMemory = (key: string, raw: string) => {
  memoryStorage.delete(key);
  memoryStorage.set(key, raw);
  while (memoryStorage.size > MAX_MEMORY_RECORDS) {
    const oldest = memoryStorage.keys().next().value as string | undefined;
    if (!oldest) {
      break;
    }
    memoryStorage.delete(oldest);
  }
};

const readRaw = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind | 'build-terminal',
) => {
  const key = operationKey(scope, kind);
  if (!persistentScope(scope)) {
    return {
      key,
      raw: memoryStorage.get(key),
      persistent: false,
      warning: APP_DEV_OPERATION_STORAGE_WARNING,
    };
  }
  if (suppressedStorageKeys.has(key)) {
    try {
      globalThis.sessionStorage.removeItem(key);
      suppressedStorageKeys.delete(key);
      return { key, raw: undefined, persistent: true };
    } catch {
      return {
        key,
        raw: undefined,
        persistent: false,
        warning: APP_DEV_OPERATION_STORAGE_WARNING,
      };
    }
  }
  try {
    const raw = globalThis.sessionStorage.getItem(key);
    if (raw !== null) {
      setMemory(key, raw);
      memoryOnlyKeys.delete(key);
      return { key, raw, persistent: true };
    }
    if (memoryOnlyKeys.has(key)) {
      return { key, raw: memoryStorage.get(key), persistent: false };
    }
    memoryStorage.delete(key);
    return { key, raw: undefined, persistent: true };
  } catch {
    return {
      key,
      raw: memoryStorage.get(key),
      persistent: false,
      warning: APP_DEV_OPERATION_STORAGE_WARNING,
    };
  }
};

const writeRaw = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind | 'build-terminal',
  raw: string,
) => {
  const key = operationKey(scope, kind);
  setMemory(key, raw);
  if (!persistentScope(scope)) {
    return {
      persistent: false,
      warning: APP_DEV_OPERATION_STORAGE_WARNING,
    };
  }
  try {
    globalThis.sessionStorage.setItem(key, raw);
    memoryOnlyKeys.delete(key);
    suppressedStorageKeys.delete(key);
    return { persistent: true };
  } catch {
    memoryOnlyKeys.add(key);
    return {
      persistent: false,
      warning: APP_DEV_OPERATION_STORAGE_WARNING,
    };
  }
};

const removeRaw = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind | 'build-terminal',
) => {
  const key = operationKey(scope, kind);
  memoryStorage.delete(key);
  memoryOnlyKeys.delete(key);
  if (!persistentScope(scope)) {
    return {
      persistent: false,
      warning: APP_DEV_OPERATION_STORAGE_WARNING,
    };
  }
  try {
    globalThis.sessionStorage.removeItem(key);
    suppressedStorageKeys.delete(key);
    return { persistent: true };
  } catch {
    suppressedStorageKeys.add(key);
    return {
      persistent: false,
      warning: APP_DEV_OPERATION_STORAGE_WARNING,
    };
  }
};

const parseStoredRecord = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind | 'build-terminal',
  raw: string | undefined,
): { value?: Record<string, unknown>; invalid: boolean } => {
  if (!raw) {
    return { invalid: false };
  }
  if (raw.length > MAX_STORED_RECORD_BYTES) {
    return { invalid: true };
  }
  try {
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      return { invalid: true };
    }
    const record = value as Record<string, unknown>;
    const expectedPrincipal = validPrincipalID(scope.principalId)
      ? scope.principalId
      : 'anonymous';
    if (
      record.schemaVersion !== APP_DEV_OPERATION_SCHEMA_VERSION ||
      record.principalId !== expectedPrincipal ||
      record.spaceId !== scope.spaceId ||
      record.projectId !== scope.projectId ||
      record.intent !== kind
    ) {
      return { invalid: true };
    }
    return { value: record, invalid: false };
  } catch {
    return { invalid: true };
  }
};

const storedIdentity = (
  scope: AppDevOperationScope,
): StoredOperationIdentity => ({
  schemaVersion: APP_DEV_OPERATION_SCHEMA_VERSION,
  principalId: validPrincipalID(scope.principalId)
    ? scope.principalId
    : 'anonymous',
  spaceId: scope.spaceId,
  projectId: scope.projectId,
});

const assertScope = (scope: AppDevOperationScope) => {
  if (!validScopeValue(scope.spaceId) || !validScopeValue(scope.projectId)) {
    throw new Error('操作作用域无效');
  }
};

export const createAppDevOperationID = () => {
  const source = globalThis.crypto;
  if (typeof source?.randomUUID === 'function') {
    return source.randomUUID();
  }
  if (typeof source?.getRandomValues !== 'function') {
    throw new Error('当前浏览器无法安全生成操作标识');
  }
  const bytes = source.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0'));
  return `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex
    .slice(6, 8)
    .join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10).join('')}`;
};

export const readAppDevPendingOperationResult = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind,
): AppDevOperationStorageResult<AppDevPendingOperation | undefined> => {
  const stored = readRaw(scope, kind);
  const parsed = parseStoredRecord(scope, kind, stored.raw);
  const { value } = parsed;
  if (
    parsed.invalid ||
    (value &&
      (!validOperationID(value.operationId) ||
        !validGeneration(value.generation) ||
        !['pending', 'requesting', 'observed'].includes(String(value.phase)) ||
        !validTimestamp(value.createdAt, APP_DEV_OPERATION_TTLS_MS[kind]) ||
        (kind === 'snapshot-restore' &&
          !normalizeAppDevSnapshotID(value.snapshotId))))
  ) {
    removeRaw(scope, kind);
    return {
      value: undefined,
      persistent: stored.persistent,
      warning: stored.warning ?? APP_DEV_OPERATION_INVALID_WARNING,
    };
  }
  return {
    value: value
      ? {
          intent: kind,
          operationId: value.operationId as string,
          phase: value.phase as AppDevOperationPhase,
          createdAt: value.createdAt as number,
          generation:
            value.generation === undefined
              ? undefined
              : Number(value.generation),
          snapshotId:
            kind === 'snapshot-restore'
              ? normalizeAppDevSnapshotID(value.snapshotId)
              : undefined,
        }
      : undefined,
    persistent: stored.persistent,
    warning: stored.warning,
  };
};

export const readAppDevPendingOperation = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind,
) => readAppDevPendingOperationResult(scope, kind).value;

export const writeAppDevPendingOperation = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind,
  operation: PendingOperationInput,
): AppDevOperationStorageResult<AppDevPendingOperation> => {
  assertScope(scope);
  const record: AppDevPendingOperation = {
    ...operation,
    intent: kind,
    phase: operation.phase ?? 'pending',
    createdAt: operation.createdAt ?? Date.now(),
  };
  if (
    kind === 'snapshot-restore' &&
    !normalizeAppDevSnapshotID(record.snapshotId)
  ) {
    return {
      value: undefined,
      persistent: persistentScope(scope),
      warning: APP_DEV_OPERATION_INVALID_WARNING,
    };
  }
  if (
    !validOperationID(record.operationId) ||
    !validGeneration(record.generation) ||
    !validTimestamp(record.createdAt, APP_DEV_OPERATION_TTLS_MS[kind]) ||
    (kind === 'snapshot-restore' &&
      !normalizeAppDevSnapshotID(record.snapshotId))
  ) {
    throw new Error('操作标识无效');
  }
  const stored: StoredPendingOperation = {
    ...record,
    ...storedIdentity(scope),
  };
  const outcome = writeRaw(scope, kind, JSON.stringify(stored));
  return { value: record, ...outcome };
};

export const clearAppDevPendingOperation = (
  scope: AppDevOperationScope,
  kind: AppDevPendingOperationKind,
  expectedOperationId?: string,
) => {
  if (
    expectedOperationId &&
    readAppDevPendingOperation(scope, kind)?.operationId !== expectedOperationId
  ) {
    return { value: undefined, persistent: persistentScope(scope) };
  }
  const outcome = removeRaw(scope, kind);
  return { value: undefined, ...outcome };
};

const validBuildProjection = (value: unknown): value is AppDevBuildInfo => {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const projection = value as Partial<AppDevBuildInfo>;
  return (
    validGeneration(projection.generation) &&
    projection.generation !== undefined &&
    ['ready', 'failed'].includes(String(projection.state)) &&
    typeof projection.releaseAvailable === 'boolean' &&
    Number.isSafeInteger(projection.size) &&
    Number(projection.size) >= 0 &&
    typeof projection.stale === 'boolean' &&
    (projection.updatedAt === undefined ||
      typeof projection.updatedAt === 'string') &&
    (projection.safeErrorCode === undefined ||
      typeof projection.safeErrorCode === 'string') &&
    (projection.safeMessage === undefined ||
      (typeof projection.safeMessage === 'string' &&
        projection.safeMessage.length <= 256))
  );
};

export const readAppDevTerminalBuildResult = (
  scope: AppDevOperationScope,
): AppDevOperationStorageResult<AppDevTerminalBuild | undefined> => {
  const stored = readRaw(scope, 'build-terminal');
  const parsed = parseStoredRecord(scope, 'build-terminal', stored.raw);
  const { value } = parsed;
  if (
    parsed.invalid ||
    (value &&
      (!validOperationID(value.operationId) ||
        !Number.isSafeInteger(value.generation) ||
        Number(value.generation) < 0 ||
        !validTimestamp(
          value.recordedAt,
          APP_DEV_OPERATION_TTLS_MS['build-terminal'],
        ) ||
        !validBuildProjection(value.projection)))
  ) {
    removeRaw(scope, 'build-terminal');
    return {
      value: undefined,
      persistent: stored.persistent,
      warning: stored.warning ?? APP_DEV_OPERATION_INVALID_WARNING,
    };
  }
  return {
    value: value
      ? {
          operationId: value.operationId as string,
          generation: Number(value.generation),
          recordedAt: value.recordedAt as number,
          projection: value.projection as AppDevBuildInfo,
        }
      : undefined,
    persistent: stored.persistent,
    warning: stored.warning,
  };
};

export const readAppDevTerminalBuild = (scope: AppDevOperationScope) =>
  readAppDevTerminalBuildResult(scope).value;

export const writeAppDevTerminalBuild = (
  scope: AppDevOperationScope,
  input: Omit<AppDevTerminalBuild, 'recordedAt'> & { recordedAt?: number },
): AppDevOperationStorageResult<AppDevTerminalBuild> => {
  assertScope(scope);
  const record: AppDevTerminalBuild = {
    ...input,
    recordedAt: input.recordedAt ?? Date.now(),
  };
  if (
    !validOperationID(record.operationId) ||
    record.generation !== record.projection.generation ||
    !validTimestamp(
      record.recordedAt,
      APP_DEV_OPERATION_TTLS_MS['build-terminal'],
    ) ||
    !validBuildProjection(record.projection)
  ) {
    throw new Error('构建状态无效');
  }
  const stored: StoredTerminalBuild = {
    ...record,
    ...storedIdentity(scope),
    intent: 'build-terminal',
  };
  const outcome = writeRaw(scope, 'build-terminal', JSON.stringify(stored));
  return { value: record, ...outcome };
};

export const clearAppDevTerminalBuild = (
  scope: AppDevOperationScope,
  expectedOperationId?: string,
) => {
  if (
    expectedOperationId &&
    readAppDevTerminalBuild(scope)?.operationId !== expectedOperationId
  ) {
    return { value: undefined, persistent: persistentScope(scope) };
  }
  const outcome = removeRaw(scope, 'build-terminal');
  return { value: undefined, ...outcome };
};
