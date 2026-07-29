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

import { reporter } from '@coze-arch/logger';

import { WorkbenchClientError } from './canonical-fetch';

export const workbenchClientOperations = [
  'searchThreads',
  'createThread',
  'getThread',
  'listMessages',
  'appendMessage',
  'generateSuggestions',
  'listRuns',
  'createRun',
  'getRun',
  'cancelRun',
  'resumeRun',
  'retrySubagentRun',
  'listRunEvents',
  'subscribeRunEvents',
  'listUploads',
  'uploadFiles',
  'deleteUpload',
  'listArtifacts',
  'getArtifactContent',
  'getArtifactSignedURL',
  'deleteArtifact',
  'restoreArtifact',
  'reviewArtifactScan',
  'listArtifactScanJobs',
  'retryArtifactScanJob',
  'getTokenUsage',
  'listMemories',
  'updateMemory',
  'deleteMemory',
  'clearMemories',
  'restoreMemory',
  'importMemories',
  'exportMemories',
  'listMemoryAuditEvents',
  'listGuardrailAuditEvents',
  'exportGuardrailAuditEvents',
  'listMCPRuntimeAuditEvents',
] as const;

export type WorkbenchClientOperation =
  (typeof workbenchClientOperations)[number];

export type WorkbenchClientOperationOutcome =
  | 'success'
  | 'rejected'
  | 'failed'
  | 'canceled'
  | 'disconnected';

const identifierKeys = [
  'thread_id',
  'run_id',
  'source_run_id',
  'message_id',
  'event_id',
  'file_id',
  'artifact_id',
  'memory_id',
  'job_id',
] as const;

type WorkbenchClientIdentifierKey = (typeof identifierKeys)[number];

export type WorkbenchClientIdentifiers = Partial<
  Record<WorkbenchClientIdentifierKey, string>
>;

export interface WorkbenchClientTelemetryMeta
  extends WorkbenchClientIdentifiers {
  client_contract: 'canonical_v1';
  operation: WorkbenchClientOperation;
  duration_ms: number;
  outcome: WorkbenchClientOperationOutcome;
  error_code?: string;
  trace_id?: string;
  error_class?: string;
}

export interface WorkbenchClientTelemetryEvent {
  eventName: 'workbench_thread_client_operation';
  meta: WorkbenchClientTelemetryMeta;
}

export type WorkbenchClientTelemetry = (
  event: WorkbenchClientTelemetryEvent,
) => void;

interface ObserveWorkbenchClientOperationOptions<T> {
  telemetry?: WorkbenchClientTelemetry;
  operation: WorkbenchClientOperation;
  identifiers?: WorkbenchClientIdentifiers;
  now?: () => number;
  execute: () => Promise<T>;
}

export interface WorkbenchClientOperationTracker {
  success: () => void;
  error: (error: unknown) => void;
  canceled: () => void;
  disconnected: () => void;
}

interface WorkbenchClientOperationTrackerOptions {
  telemetry?: WorkbenchClientTelemetry;
  operation: WorkbenchClientOperation;
  identifiers?: WorkbenchClientIdentifiers;
  now?: () => number;
}

const positiveDecimal = /^[1-9]\d*$/;
const stableErrorCode = /^[a-z][a-z0-9_]{0,63}$/;
const stableErrorClass = /^[A-Za-z][A-Za-z0-9]{0,63}$/;
const stableTraceReference = /^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$/;
const sensitiveTelemetryMarker =
  /authorization|bearer|api[_-]?key|access[_-]?token|credential|password/i;
const credentialPrefixMarker =
  /(?:^|[_-])(?:sk|ghp|github_pat|xox[baprs])[_-]/i;

const isSafeTelemetryValue = (value: string, pattern: RegExp): boolean =>
  pattern.test(value) &&
  !sensitiveTelemetryMarker.test(value) &&
  !credentialPrefixMarker.test(value);

const ownDataProperty = (
  record: Record<string, unknown>,
  key: string,
): unknown => {
  try {
    const descriptor = Object.getOwnPropertyDescriptor(record, key);
    return descriptor && 'value' in descriptor ? descriptor.value : undefined;
  } catch (error) {
    void error;
    return undefined;
  }
};

export const workbenchClientIdentifiers = (
  request: unknown,
): WorkbenchClientIdentifiers => {
  if (
    typeof request !== 'object' ||
    request === null ||
    Array.isArray(request)
  ) {
    return {};
  }
  const record = request as Record<string, unknown>;
  const identifiers: WorkbenchClientIdentifiers = {};
  identifierKeys.forEach(key => {
    const value = ownDataProperty(record, key);
    if (typeof value === 'string' && positiveDecimal.test(value)) {
      identifiers[key] = value;
    }
  });
  return identifiers;
};

const defaultTelemetry: WorkbenchClientTelemetry = event => {
  reporter.event(event);
};

const safeDuration = (startedAt: number, now: () => number): number => {
  const duration = Math.round(now() - startedAt);
  return Number.isSafeInteger(duration) && duration >= 0 ? duration : 0;
};

const emitTelemetry = (
  telemetry: WorkbenchClientTelemetry | undefined,
  event: WorkbenchClientTelemetryEvent,
): void => {
  try {
    (telemetry ?? defaultTelemetry)(event);
  } catch (error) {
    void error;
    // Observability must never alter an already determined business result.
  }
};

const failureFields = (
  error: unknown,
): Pick<
  WorkbenchClientTelemetryMeta,
  'outcome' | 'error_code' | 'trace_id' | 'error_class'
> => {
  if (error instanceof WorkbenchClientError) {
    return {
      outcome: error.outcome === 'rejected' ? 'rejected' : 'failed',
      error_code: isSafeTelemetryValue(error.code, stableErrorCode)
        ? error.code
        : 'invalid_error_code',
      ...(error.traceId &&
      isSafeTelemetryValue(error.traceId, stableTraceReference)
        ? { trace_id: error.traceId }
        : {}),
    };
  }
  if (
    typeof error === 'object' &&
    error !== null &&
    'name' in error &&
    error.name === 'AbortError'
  ) {
    return {
      outcome: 'canceled',
      error_code: 'aborted',
    };
  }
  return {
    outcome: 'failed',
    error_code: 'unexpected_error',
    error_class:
      error instanceof Error &&
      isSafeTelemetryValue(error.name, stableErrorClass)
        ? error.name
        : error instanceof Error
          ? 'Error'
          : 'UnknownError',
  };
};

export const createWorkbenchClientOperationTracker = ({
  telemetry,
  operation,
  identifiers = {},
  now = Date.now,
}: WorkbenchClientOperationTrackerOptions): WorkbenchClientOperationTracker => {
  const startedAt = now();
  let emitted = false;
  const emit = (
    outcome: WorkbenchClientOperationOutcome,
    failure: Partial<WorkbenchClientTelemetryMeta> = {},
  ) => {
    if (emitted) {
      return;
    }
    emitted = true;
    emitTelemetry(telemetry, {
      eventName: 'workbench_thread_client_operation',
      meta: {
        client_contract: 'canonical_v1',
        operation,
        ...identifiers,
        duration_ms: safeDuration(startedAt, now),
        outcome,
        ...failure,
      },
    });
  };
  return {
    success: () => emit('success'),
    error: error => {
      const fields = failureFields(error);
      emit(fields.outcome, fields);
    },
    canceled: () => emit('canceled', { error_code: 'aborted' }),
    disconnected: () =>
      emit('disconnected', { error_code: 'stream_disconnected' }),
  };
};

export const observeWorkbenchClientOperation = async <T>({
  telemetry,
  operation,
  identifiers,
  now,
  execute,
}: ObserveWorkbenchClientOperationOptions<T>): Promise<T> => {
  const tracker = createWorkbenchClientOperationTracker({
    telemetry,
    operation,
    identifiers,
    now,
  });
  try {
    const result = await execute();
    tracker.success();
    return result;
  } catch (error) {
    tracker.error(error);
    throw error;
  }
};
